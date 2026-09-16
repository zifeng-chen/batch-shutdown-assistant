import json
import uuid
from datetime import datetime, timedelta
from config import now_bjt

from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, update

from database import get_db
from models.models import Device, Task, TaskResult, TaskType, TaskStatus, DeviceStatus
from api.websocket_manager import manager
from api.auth import get_current_user
from api.audit import log_audit

UPGRADE_TIMEOUT_SECONDS = 300

router = APIRouter(prefix="/api/tasks", tags=["tasks"])


class TaskCreate(BaseModel):
    task_type: str
    device_ids: list[str]
    params: dict | None = None


class TaskResponse(BaseModel):
    id: str
    task_type: str
    status: str
    params: str | None
    created_at: datetime

    class Config:
        from_attributes = True


@router.post("", response_model=TaskResponse)
async def create_task(
    payload: TaskCreate,
    db: AsyncSession = Depends(get_db),
    current_user: dict = Depends(get_current_user),
):
    task = Task(
        id=str(uuid.uuid4()),
        task_type=TaskType(payload.task_type),
        status=TaskStatus.PENDING,
        params=json.dumps(payload.params) if payload.params else None,
    )
    db.add(task)

    task_results = []
    for device_id in payload.device_ids:
        tr = TaskResult(
            id=str(uuid.uuid4()),
            task_id=task.id,
            device_id=device_id,
            status="pending",
        )
        db.add(tr)
        task_results.append(tr)

    await db.commit()

    task.status = TaskStatus.RUNNING
    await db.commit()

    sent_count = 0
    for tr in task_results:
        command = {
            "id": tr.id,
            "action": payload.task_type,
            "params": payload.params or {},
        }
        success = await manager.send_command(tr.device_id, command)
        if success:
            tr.status = "sent"
            tr.started_at = now_bjt()
            sent_count += 1
        else:
            tr.status = "failed"
            tr.output = "Agent offline"
            tr.finished_at = now_bjt()

    await db.commit()

    if sent_count == 0:
        task.status = TaskStatus.FAILED
    await db.commit()

    await log_audit(db, "task_create", current_user["username"],
                    target=f"{payload.task_type}:{len(payload.device_ids)} devices",
                    detail=json.dumps(payload.params) if payload.params else None)

    return task


@router.get("")
async def list_tasks(
    db: AsyncSession = Depends(get_db),
    _: dict = Depends(get_current_user),
):
    result = await db.execute(
        select(Task).order_by(Task.created_at.desc()).limit(100)
    )
    tasks = result.scalars().all()

    task_list = []
    for t in tasks:
        results = await db.execute(
            select(TaskResult).where(TaskResult.task_id == t.id)
        )
        task_results = results.scalars().all()

        device_ips = []
        for tr in task_results:
            dev_result = await db.execute(
                select(Device.ip).where(Device.id == tr.device_id)
            )
            dev = dev_result.scalar_one_or_none()
            if dev:
                device_ips.append(dev)

        # 构造操作描述
        desc = ""
        params = json.loads(t.params) if t.params else {}
        if t.task_type == TaskType.UPGRADE:
            ver = params.get("version", "?")
            desc = f"升级到 v{ver}"
        elif t.task_type == TaskType.SYSPREP:
            desc = "Sysprep 关机"
        elif t.task_type == TaskType.SYSPREP_REBOOT:
            desc = "Sysprep 重启"
        else:
            desc = str(t.task_type)

        task_list.append({
            "id": t.id,
            "task_type": t.task_type.value if hasattr(t.task_type, 'value') else str(t.task_type),
            "status": t.status.value if hasattr(t.status, 'value') else str(t.status),
            "description": desc,
            "device_ips": device_ips,
            "created_at": t.created_at.isoformat() if t.created_at else None,
        })

    return task_list


@router.get("/{task_id}")
async def get_task(
    task_id: str,
    db: AsyncSession = Depends(get_db),
    _: dict = Depends(get_current_user),
):
    result = await db.execute(select(Task).where(Task.id == task_id))
    task = result.scalar_one_or_none()
    if not task:
        raise HTTPException(status_code=404, detail="Task not found")

    results = await db.execute(
        select(TaskResult).where(TaskResult.task_id == task_id)
    )
    task_results = results.scalars().all()

    # upgrade 任务超时兜底：超过 5 分钟仍在 running/sent 则标记为 timeout
    if task.task_type == TaskType.UPGRADE and task.status == TaskStatus.RUNNING:
        cutoff = now_bjt() - timedelta(seconds=UPGRADE_TIMEOUT_SECONDS)
        timed_out = False
        for tr in task_results:
            if tr.status in ("sent", "pending") and tr.started_at and tr.started_at < cutoff:
                tr.status = "timeout"
                tr.output = "upgrade timed out (agent did not report back)"
                tr.finished_at = now_bjt()
                timed_out = True
        if timed_out:
            task.status = TaskStatus.FAILED
            await db.commit()

    # 查询每个结果对应的设备IP
    results_with_ip = []
    for r in task_results:
        dev_result = await db.execute(
            select(Device.ip).where(Device.id == r.device_id)
        )
        dev_ip = dev_result.scalar_one_or_none()
        results_with_ip.append({
            "id": r.id,
            "device_id": r.device_id,
            "device_ip": dev_ip or r.device_id,
            "status": r.status,
            "output": r.output,
            "started_at": r.started_at,
            "finished_at": r.finished_at,
        })

    return {
        "id": task.id,
        "task_type": task.task_type,
        "status": task.status,
        "params": task.params,
        "created_at": task.created_at,
        "results": results_with_ip,
    }


@router.post("/{task_id}/cancel")
async def cancel_task(
    task_id: str,
    db: AsyncSession = Depends(get_db),
    current_user: dict = Depends(get_current_user),
):
    result = await db.execute(select(Task).where(Task.id == task_id))
    task = result.scalar_one_or_none()
    if not task:
        raise HTTPException(status_code=404, detail="Task not found")

    if task.status not in (TaskStatus.PENDING, TaskStatus.RUNNING):
        raise HTTPException(status_code=400, detail="Task cannot be cancelled")

    task.status = TaskStatus.CANCELLED
    pending = await db.execute(
        select(TaskResult).where(
            TaskResult.task_id == task_id,
            TaskResult.status.in_(["pending", "sent"]),
        )
    )
    for tr in pending.scalars().all():
        tr.status = "cancelled"
        tr.finished_at = now_bjt()

    await db.commit()

    await log_audit(db, "task_cancel", current_user["username"], target=task_id)
    return {"status": "cancelled"}
