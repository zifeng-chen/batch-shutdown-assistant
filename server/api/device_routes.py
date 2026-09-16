import uuid
from datetime import datetime

from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import BaseModel
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func, delete as sql_delete

from database import get_db
from models.models import Device, DeviceStatus, TaskResult
from api.auth import get_current_user
from api.audit import log_audit
from api.websocket_manager import manager
from config import now_bjt

router = APIRouter(prefix="/api/devices", tags=["devices"])


class DeviceCreate(BaseModel):
    hostname: str | None = None
    ip: str | None = None
    group_name: str | None = None


class DeviceUpdate(BaseModel):
    hostname: str | None = None
    ip: str | None = None
    group_name: str | None = None


class DeviceResponse(BaseModel):
    id: str
    hostname: str | None
    ip: str | None
    os_info: str | None
    status: str
    group_name: str | None
    rearm_count: int
    agent_version: str | None
    last_heartbeat: datetime | None
    created_at: datetime

    class Config:
        from_attributes = True


@router.get("", response_model=list[DeviceResponse])
async def list_devices(
    status: str | None = None,
    group: str | None = None,
    search: str | None = None,
    page: int = Query(1, ge=1),
    size: int = Query(50, ge=1, le=200),
    db: AsyncSession = Depends(get_db),
    _: dict = Depends(get_current_user),
):
    query = select(Device).where(Device.deleted_at.is_(None))

    if status:
        query = query.where(Device.status == status)
    if group:
        query = query.where(Device.group_name == group)
    if search:
        query = query.where(
            (Device.hostname.ilike(f"%{search}%")) | (Device.ip.ilike(f"%{search}%"))
        )

    query = query.offset((page - 1) * size).limit(size)

    result = await db.execute(query)
    devices_list = result.scalars().all()

    # 按IP地址数值排序
    import ipaddress as _ip
    def ip_sort_key(d):
        try:
            return int(_ip.IPv4Address(d.ip)) if d.ip else 0
        except Exception:
            return 0
    devices_list.sort(key=ip_sort_key)

    return devices_list


@router.post("", response_model=DeviceResponse)
async def create_device(
    payload: DeviceCreate,
    db: AsyncSession = Depends(get_db),
    current_user: dict = Depends(get_current_user),
):
    device = Device(
        id=str(uuid.uuid4()),
        hostname=payload.hostname,
        ip=payload.ip,
        group_name=payload.group_name,
        token=str(uuid.uuid4()),
        status=DeviceStatus.OFFLINE,
    )
    db.add(device)
    await db.commit()
    await db.refresh(device)

    await log_audit(db, "device_create", current_user["username"],
                    target=f"{payload.hostname or ''}({payload.ip or ''})")
    return device


@router.get("/stats")
async def device_stats(
    db: AsyncSession = Depends(get_db),
    _: dict = Depends(get_current_user),
):
    active = Device.deleted_at.is_(None)
    total = await db.execute(select(func.count(Device.id)).where(active))
    online = await db.execute(
        select(func.count(Device.id)).where(active, Device.status == DeviceStatus.ONLINE)
    )
    offline = await db.execute(
        select(func.count(Device.id)).where(active, Device.status == DeviceStatus.OFFLINE)
    )
    return {
        "total": total.scalar(),
        "online": online.scalar(),
        "offline": offline.scalar(),
    }


@router.put("/{device_id}", response_model=DeviceResponse)
async def update_device(
    device_id: str,
    payload: DeviceUpdate,
    db: AsyncSession = Depends(get_db),
    current_user: dict = Depends(get_current_user),
):
    result = await db.execute(select(Device).where(Device.id == device_id))
    device = result.scalar_one_or_none()
    if not device:
        raise HTTPException(status_code=404, detail="Device not found")

    if payload.hostname is not None:
        device.hostname = payload.hostname
    if payload.ip is not None:
        device.ip = payload.ip
    if payload.group_name is not None:
        device.group_name = payload.group_name

    await db.commit()
    await db.refresh(device)

    await log_audit(db, "device_update", current_user["username"], target=device_id)
    return device


@router.delete("/{device_id}")
async def delete_device(
    device_id: str,
    db: AsyncSession = Depends(get_db),
    current_user: dict = Depends(get_current_user),
):
    result = await db.execute(select(Device).where(Device.id == device_id))
    device = result.scalar_one_or_none()
    if not device:
        raise HTTPException(status_code=404, detail="Device not found")

    # 如果设备在线，先推送卸载指令
    uninstalled = False
    if device.status == DeviceStatus.ONLINE and manager.is_agent_online(device_id):
        import uuid as _uuid
        cmd = {"id": str(_uuid.uuid4()), "action": "uninstall", "params": {}}
        uninstalled = await manager.send_command(device_id, cmd)

    device.deleted_at = now_bjt()
    device.status = DeviceStatus.OFFLINE
    await db.commit()

    await log_audit(db, "device_delete", current_user["username"],
                    target=device_id,
                    detail=f"uninstall_sent={uninstalled}" if uninstalled else None)
    return {"status": "deleted", "uninstall_sent": uninstalled}


class BatchDeleteRequest(BaseModel):
    device_ids: list[str]


@router.post("/batch-delete")
async def batch_delete_devices(
    payload: BatchDeleteRequest,
    db: AsyncSession = Depends(get_db),
    current_user: dict = Depends(get_current_user),
):
    import logging
    import uuid as _uuid
    logging.getLogger(__name__).info(f"batch-delete received {len(payload.device_ids)} ids: {payload.device_ids[:3]}")

    # 对在线设备推送卸载指令
    result = await db.execute(
        select(Device).where(Device.id.in_(payload.device_ids))
    )
    devices = result.scalars().all()
    uninstall_count = 0
    for device in devices:
        if device.status == DeviceStatus.ONLINE and manager.is_agent_online(device.id):
            cmd = {"id": str(_uuid.uuid4()), "action": "uninstall", "params": {}}
            if await manager.send_command(device.id, cmd):
                uninstall_count += 1

    deleted = 0
    for device in devices:
        device.deleted_at = now_bjt()
        device.status = DeviceStatus.OFFLINE
        deleted += 1
    await db.commit()

    await log_audit(db, "device_batch_delete", current_user["username"],
                    detail=f"deleted={deleted}/{len(payload.device_ids)}, uninstall_sent={uninstall_count}")
    return {"deleted": deleted, "requested": len(payload.device_ids), "uninstall_sent": uninstall_count}
