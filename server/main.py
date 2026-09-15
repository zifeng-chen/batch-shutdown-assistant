import asyncio
import logging
import os
import uuid
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request
from fastapi.staticfiles import StaticFiles
from fastapi.responses import FileResponse
from fastapi.middleware.cors import CORSMiddleware
from sqlalchemy import select, update

from database import init_db, async_session
from models.models import User, Device, DeviceStatus
from api.auth import hash_password
from config import now_bjt, HEARTBEAT_TIMEOUT_SECONDS
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
            from datetime import timedelta, datetime as dt
            from sqlalchemy import text
            cutoff = (dt.now() - timedelta(seconds=HEARTBEAT_TIMEOUT_SECONDS)).strftime('%Y-%m-%d %H:%M:%S')
            async with async_session() as db:
                result = await db.execute(
                    text("UPDATE devices SET status = 'OFFLINE' WHERE status = 'ONLINE' AND last_heartbeat < :cutoff"),
                    {"cutoff": cutoff}
                )
                if result.rowcount > 0:
                    await db.commit()
                    logging.info(f"Heartbeat timeout: {result.rowcount} device(s) marked offline")
        except Exception as e:
            logging.error(f"Heartbeat checker error: {e}")


@asynccontextmanager
async def lifespan(app: FastAPI):
    await init_db()
    await ensure_default_admin()
    task = asyncio.create_task(heartbeat_timeout_checker())
    logging.info("Server ready")
    yield
    task.cancel()


app = FastAPI(title="LAN Agent Manager", version="1.0.0", lifespan=lifespan)

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
    file_path = os.path.join(STATIC_DIR, full_path)
    if full_path and os.path.isfile(file_path):
        resp = FileResponse(file_path)
        resp.headers["Cache-Control"] = "no-cache"
        return resp
    index_path = os.path.join(STATIC_DIR, "index.html")
    if os.path.isfile(index_path):
        resp = FileResponse(index_path)
        resp.headers["Cache-Control"] = "no-cache"
        return resp
    return {"detail": "Not found"}
