import logging
import uuid
from datetime import datetime, timedelta
from config import now_bjt

from fastapi import APIRouter, Depends, HTTPException, Request
from pydantic import BaseModel
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, update

from database import get_db
from models.models import Device, DeviceStatus

logger = logging.getLogger(__name__)
router = APIRouter(prefix="/api/agents", tags=["agents"])


class HeartbeatRequest(BaseModel):
    device_id: str | None = None
    hostname: str
    os: str
    ip: str
    rearm_count: int
    status: str
    agent_version: str
    timestamp: int


class RecoverRequest(BaseModel):
    device_id: str
    hostname: str


@router.post("/heartbeat")
async def agent_heartbeat(payload: HeartbeatRequest, request: Request, db: AsyncSession = Depends(get_db)):
    # 从 Authorization header 提取 Agent token
    auth_header = request.headers.get("authorization", "")
    agent_token = ""
    if auth_header.startswith("Bearer "):
        agent_token = auth_header[7:]

    device = None

    if payload.device_id:
        result = await db.execute(
            select(Device).where(Device.id == payload.device_id)
        )
        device = result.scalar_one_or_none()

    if not device and payload.ip:
        result = await db.execute(
            select(Device).where(Device.ip == payload.ip)
        )
        ip_device = result.scalar_one_or_none()
        # 如果该IP对应的设备已被软删除，不自动重新注册
        if ip_device and ip_device.deleted_at is not None:
            return {"device_id": ip_device.id, "status": "deleted"}
        device = ip_device

    if device:
        # 已软删除的设备不接受心跳，不自动恢复
        if device.deleted_at is not None:
            return {"device_id": device.id, "status": "deleted"}

        device.hostname = payload.hostname
        device.os_info = payload.os
        device.ip = payload.ip
        device.rearm_count = payload.rearm_count
        device.agent_version = payload.agent_version
        device.last_heartbeat = now_bjt()
        if agent_token and device.token != agent_token:
            device.token = agent_token
        if payload.status == "online":
            device.status = DeviceStatus.ONLINE
        await db.commit()
        return {"device_id": device.id, "status": "updated"}

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
    db.add(device)
    await db.commit()

    logger.info(f"Auto-registered new device: {new_id} ({payload.hostname} / {payload.ip})")
    return {"device_id": new_id, "status": "created"}


@router.post("/recover")
async def agent_recover(payload: RecoverRequest, db: AsyncSession = Depends(get_db)):
    result = await db.execute(
        select(Device).where(Device.id == payload.device_id)
    )
    device = result.scalar_one_or_none()
    if not device:
        raise HTTPException(status_code=404, detail="Device not found")

    device.hostname = payload.hostname
    device.status = DeviceStatus.ONLINE
    device.last_heartbeat = now_bjt()
    await db.commit()

    logger.info(f"Device {payload.device_id} recovered after sysprep")
    return {"status": "recovered", "device_id": payload.device_id}
