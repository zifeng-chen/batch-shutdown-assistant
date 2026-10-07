import asyncio
import logging
import uuid
from datetime import datetime, timedelta
from config import now_bjt

from fastapi import APIRouter, Depends, HTTPException, Request
from pydantic import BaseModel
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, update

from database import get_db
from config import ENROLL_SECRET, SECRET_KEY
from models.models import Device, DeviceStatus

logger = logging.getLogger(__name__)
router = APIRouter(prefix="/api/agents", tags=["agents"])


def _enroll_secret() -> str:
    """首次注册所需的预共享密钥，未显式配置时由 SECRET_KEY 派生"""
    return ENROLL_SECRET or f"enroll-{SECRET_KEY}"


class SysInfoPayload(BaseModel):
    cpu_model: str | None = None
    cpu_cores: int | None = None
    memory_total_mb: int | None = None
    disk_total_gb: int | None = None
    disk_free_gb: int | None = None
    os_version: str | None = None
    serial_number: str | None = None
    mac_address: str | None = None
    manufacturer: str | None = None
    model: str | None = None


class HeartbeatRequest(BaseModel):
    device_id: str | None = None
    hostname: str
    os: str
    ip: str
    rearm_count: int
    status: str
    agent_version: str
    timestamp: int
    sysinfo: SysInfoPayload | None = None


def _apply_sysinfo(device: Device, info: SysInfoPayload | None) -> None:
    """把 Agent 上报的硬件信息写入设备记录"""
    if info is None:
        return
    device.cpu_model = info.cpu_model
    device.cpu_cores = info.cpu_cores
    device.memory_total_mb = info.memory_total_mb
    device.disk_total_gb = info.disk_total_gb
    device.disk_free_gb = info.disk_free_gb
    device.os_version = info.os_version
    device.serial_number = info.serial_number
    device.mac_address = info.mac_address
    device.manufacturer = info.manufacturer
    device.model = info.model
    device.sysinfo_updated_at = now_bjt()


class RecoverRequest(BaseModel):
    device_id: str
    hostname: str


def _enroll_secret_valid(request: Request) -> bool:
    """请求是否携带了正确的注册密钥"""
    return request.headers.get("x-enroll-secret", "") == _enroll_secret()


# 设备注册/接管的临界区锁。
# Agent 启动瞬间可能并发发出多个心跳，若不串行化会出现"都查不到记录 → 都新建"
# 的竞态，导致同一台设备产生多条记录。
#
# 锁按事件循环惰性创建：模块级 asyncio.Lock 会绑定到首次使用的事件循环，
# 在测试或多循环环境下会抛 "bound to a different event loop"。
_registration_locks = None


def _get_registration_lock() -> asyncio.Lock:
    global _registration_locks, _lock_guard
    import weakref

    if _registration_locks is None:
        _registration_locks = weakref.WeakKeyDictionary()
    loop = asyncio.get_running_loop()
    if loop not in _registration_locks:
        _registration_locks[loop] = asyncio.Lock()
    return _registration_locks[loop]


def _adopt_device(device: Device, payload: HeartbeatRequest, agent_token: str) -> None:
    """把设备记录更新为上报方的身份信息（用于首次注册 / 重装接管 / 软删除恢复）"""
    device.hostname = payload.hostname
    device.os_info = payload.os
    if payload.ip:
        device.ip = payload.ip
    device.rearm_count = payload.rearm_count
    device.agent_version = payload.agent_version
    device.last_heartbeat = now_bjt()
    device.token = agent_token
    device.deleted_at = None
    device.status = DeviceStatus.ONLINE if payload.status == "online" else DeviceStatus.OFFLINE
    _apply_sysinfo(device, payload.sysinfo)


@router.post("/heartbeat")
async def agent_heartbeat(payload: HeartbeatRequest, request: Request, db: AsyncSession = Depends(get_db)):
    # 从 Authorization header 提取 Agent token
    auth_header = request.headers.get("authorization", "")
    agent_token = ""
    if auth_header.startswith("Bearer "):
        agent_token = auth_header[7:]

    # 串行化注册/接管过程，避免并发心跳各自新建记录
    async with _get_registration_lock():
        return await _handle_heartbeat(payload, request, db, agent_token)


async def _handle_heartbeat(
    payload: HeartbeatRequest, request: Request, db: AsyncSession, agent_token: str
):
    device = None

    # 优先按 device_id 精确匹配
    if payload.device_id:
        result = await db.execute(
            select(Device).where(Device.id == payload.device_id)
        )
        device = result.scalar_one_or_none()

    # 未命中则按 IP 匹配（覆盖：设备未带 device_id 的重装场景）
    if device is None and payload.ip:
        result = await db.execute(
            select(Device).where(Device.ip == payload.ip).order_by(Device.last_heartbeat.desc())
        )
        device = result.scalars().first()

    if device is not None:
        # 令牌一致 → 正常心跳
        if not device.token or agent_token == device.token:
            pass

        # 令牌不一致：可能是合法的重装（Agent 重新生成 token），也可能是伪造。
        # 仅当携带正确注册密钥（或记录已软删除）时才接受接管，否则拒绝。
        elif device.deleted_at is not None or _enroll_secret_valid(request):
            logger.info(
                "Device %s (%s / %s) re-enrolled: token rotated%s",
                device.id, payload.hostname, payload.ip,
                " (restored from deleted)" if device.deleted_at is not None else "",
            )
            _adopt_device(device, payload, agent_token)
            await db.commit()
            return {"device_id": device.id, "status": "re-enrolled"}

        # 已软删除且无法验证身份 → 告知其仍处于删除态
        elif device.deleted_at is not None:
            return {"device_id": device.id, "status": "deleted"}

        else:
            logger.warning(
                "Rejected heartbeat for device %s: token mismatch (%s / %s)",
                device.id, payload.hostname, payload.ip,
            )
            raise HTTPException(status_code=401, detail="Invalid agent token")

        # 令牌有效：刷新运行时信息
        if not device.hostname_locked:
            device.hostname = payload.hostname
        device.os_info = payload.os
        device.rearm_count = payload.rearm_count
        device.agent_version = payload.agent_version
        device.last_heartbeat = now_bjt()
        if payload.status == "online":
            device.status = DeviceStatus.ONLINE
        _apply_sysinfo(device, payload.sysinfo)
        await db.commit()
        return {"device_id": device.id, "status": "updated"}

    # 未知设备首次注册：需携带预共享 enroll secret，防止匿名伪造设备
    if not _enroll_secret_valid(request):
        logger.warning(
            "Rejected unregistered device enrollment: %s / %s",
            payload.hostname, payload.ip,
        )
        raise HTTPException(status_code=401, detail="Enrollment secret required")

    new_id = str(uuid.uuid4())
    device = Device(
        id=new_id,
        hostname=payload.hostname,
        ip=payload.ip,
        os_info=payload.os,
        rearm_count=payload.rearm_count,
        agent_version=payload.agent_version,
        token=agent_token or str(uuid.uuid4()),
        status=DeviceStatus.ONLINE if payload.status == "online" else DeviceStatus.OFFLINE,
        last_heartbeat=now_bjt(),
    )
    _apply_sysinfo(device, payload.sysinfo)
    db.add(device)
    await db.commit()

    logger.info("Auto-registered new device: %s (%s / %s)", new_id, payload.hostname, payload.ip)
    return {"device_id": new_id, "status": "created"}


@router.post("/recover")
async def agent_recover(payload: RecoverRequest, request: Request, db: AsyncSession = Depends(get_db)):
    auth_header = request.headers.get("authorization", "")
    agent_token = auth_header[7:] if auth_header.startswith("Bearer ") else ""

    result = await db.execute(
        select(Device).where(Device.id == payload.device_id)
    )
    device = result.scalar_one_or_none()
    if not device:
        raise HTTPException(status_code=404, detail="Device not found")

    # 必须持有该设备的令牌才能恢复，防止他人抢占设备
    if device.token and agent_token != device.token:
        logger.warning(f"Rejected recover for device {payload.device_id}: token mismatch")
        raise HTTPException(status_code=401, detail="Invalid agent token")

    if not device.hostname_locked:
        device.hostname = payload.hostname
    device.status = DeviceStatus.ONLINE
    device.last_heartbeat = now_bjt()
    await db.commit()

    logger.info(f"Device {payload.device_id} recovered after sysprep")
    return {"status": "recovered", "device_id": payload.device_id}
