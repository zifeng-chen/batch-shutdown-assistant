import asyncio
import logging
import os
import uuid
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request
from fastapi.staticfiles import StaticFiles
from fastapi.responses import FileResponse
from sqlalchemy import select, update

from database import init_db, async_session
from models.models import User, Device, DeviceStatus
from api.auth import hash_password
from api.audit import purge_old_audits
from api.upgrade_routes import _cleanup_uploads
from config import now_bjt, HEARTBEAT_TIMEOUT_SECONDS, SECRET_KEY, APP_VERSION
from api.agent_routes import router as agent_router
from api.device_routes import router as device_router
from api.task_routes import router as task_router
from api.ws_routes import router as ws_router
from api.auth_routes import router as auth_router
from api.audit_routes import router as audit_router
from api.import_routes import router as import_router
from api.deploy_routes import router as deploy_router
from api.upgrade_routes import router as upgrade_router

logging.basicConfig(level=logging.INFO)

STATIC_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "static")


async def ensure_default_admin():
    async with async_session() as db:
        result = await db.execute(select(User).where(User.username == "admin"))
        if not result.scalar_one_or_none():
            admin = User(
                id=str(uuid.uuid4()),
                username="admin",
                password_hash=hash_password("admin123"),
                role="admin",
            )
            db.add(admin)
            await db.commit()
            logging.info("Default admin created: admin / admin123")


async def heartbeat_timeout_checker():
    """每10秒检查一次，将超时设备标记为离线"""
    while True:
        await asyncio.sleep(10)
        try:
            from datetime import timedelta
            from sqlalchemy import text
            cutoff = (now_bjt() - timedelta(seconds=HEARTBEAT_TIMEOUT_SECONDS)).strftime('%Y-%m-%d %H:%M:%S')
            async with async_session() as db:
                # 原生 SQL 不走 Enum 类型处理器，须用 .name 匹配库中存储的枚举成员名（大写）
                result = await db.execute(
                    text("UPDATE devices SET status = :offline WHERE status = :online AND last_heartbeat < :cutoff"),
                    {"offline": DeviceStatus.OFFLINE.name, "online": DeviceStatus.ONLINE.name, "cutoff": cutoff}
                )
                if result.rowcount > 0:
                    await db.commit()
                    logging.info(f"Heartbeat timeout: {result.rowcount} device(s) marked offline")
        except Exception as e:
            logging.error(f"Heartbeat checker error: {e}")


async def maintenance_loop():
    """每日一次：清理超期审计日志与过期上传文件。"""
    while True:
        await asyncio.sleep(24 * 3600)
        try:
            async with async_session() as db:
                deleted = await purge_old_audits(db)
            if deleted:
                logging.info(f"Audit purge: removed {deleted} old entries")
            _cleanup_uploads()
        except Exception as e:
            logging.error(f"Maintenance loop error: {e}")


@asynccontextmanager
async def lifespan(app: FastAPI):
    if SECRET_KEY == "change-me-in-production":
        logging.warning(
            "⚠️ 正在使用默认 SECRET_KEY，存在安全风险！请设置环境变量 LANAGENT_SECRET 后重启。"
        )
    await init_db()
    await ensure_default_admin()
    task = asyncio.create_task(heartbeat_timeout_checker())
    maintenance = asyncio.create_task(maintenance_loop())
    logging.info("Server ready")
    yield
    task.cancel()
    maintenance.cancel()


app = FastAPI(title="LAN Agent Manager", version=APP_VERSION, lifespan=lifespan)

app.include_router(auth_router)
app.include_router(agent_router)
app.include_router(device_router)
app.include_router(task_router)
app.include_router(ws_router)
app.include_router(audit_router)
app.include_router(import_router)
app.include_router(deploy_router)
app.include_router(upgrade_router)

try:
    app.mount("/assets", StaticFiles(directory=os.path.join(STATIC_DIR, "assets")), name="assets")
except Exception:
    pass


@app.get("/api/health")
async def health():
    return {"status": "ok"}


@app.get("/{full_path:path}")
async def serve_spa(request: Request, full_path: str):
    static_root = os.path.realpath(STATIC_DIR)
    if full_path:
        candidate = os.path.realpath(os.path.join(static_root, full_path))
        # 防止 ../ 或编码变体逃逸出静态目录
        if os.path.commonpath([candidate, static_root]) == static_root and os.path.isfile(candidate):
            resp = FileResponse(candidate)
            resp.headers["Cache-Control"] = "no-cache"
            return resp
    index_path = os.path.join(static_root, "index.html")
    if os.path.isfile(index_path):
        resp = FileResponse(index_path)
        resp.headers["Cache-Control"] = "no-cache"
        return resp
    return {"detail": "Not found"}
