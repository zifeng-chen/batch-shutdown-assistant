import json
import logging

from fastapi import APIRouter, WebSocket, WebSocketDisconnect, Query
from sqlalchemy import select, update

from database import async_session
from models.models import Device, DeviceStatus, Task, TaskResult, TaskStatus
from api.websocket_manager import manager
from config import now_bjt

logger = logging.getLogger(__name__)
router = APIRouter()

TERMINAL_STATUSES = {"success", "failed", "cancelled", "timeout"}


async def _sync_parent_task(db, task_id: str):
    """当所有子 TaskResult 都到达终态时，同步更新父 Task 状态。"""
    results = await db.execute(
        select(TaskResult).where(TaskResult.task_id == task_id)
    )
    all_results = results.scalars().all()
    if not all_results:
        return

    statuses = {r.status for r in all_results}
    if not statuses.issubset(TERMINAL_STATUSES):
        return

    if statuses == {"success"}:
        final = TaskStatus.COMPLETED
    elif "cancelled" in statuses and statuses <= {"cancelled", "success"}:
        final = TaskStatus.CANCELLED
    else:
        final = TaskStatus.FAILED

    await db.execute(
        update(Task).where(Task.id == task_id).values(status=final)
    )
    await db.commit()


@router.websocket("/ws/agents/{device_id}")
async def agent_websocket(websocket: WebSocket, device_id: str, token: str = Query(default="")):
    # 验证 Agent token
    async with async_session() as db:
        result = await db.execute(
            select(Device).where(Device.id == device_id)
        )
        device = result.scalar_one_or_none()
        if not device or device.token != token:
            await websocket.close(code=4001, reason="Unauthorized")
            return

    await manager.connect_agent(device_id, websocket)

    async with async_session() as db:
        await db.execute(
            update(Device)
            .where(Device.id == device_id)
            .values(status=DeviceStatus.ONLINE, last_heartbeat=now_bjt())
        )
        await db.commit()

    try:
        while True:
            data = await websocket.receive_text()
            msg = json.loads(data)

            command_id = msg.get("command_id")
            if command_id:
                async with async_session() as db:
                    tr = await db.execute(
                        select(TaskResult).where(TaskResult.id == command_id)
                    )
                    task_result = tr.scalar_one_or_none()
                    if task_result:
                        task_result.status = msg.get("status", "unknown")
                        task_result.output = msg.get("output", "")
                        task_result.finished_at = now_bjt()
                        await db.commit()

                        await _sync_parent_task(db, str(task_result.task_id))

                        await manager.broadcast_task_update(
                            str(task_result.task_id),
                            {
                                "type": "result",
                                "command_id": command_id,
                                "device_id": device_id,
                                "status": msg.get("status"),
                                "output": msg.get("output"),
                            }
                        )

    except WebSocketDisconnect:
        pass
    finally:
        await manager.disconnect_agent(device_id)
        async with async_session() as db:
            await db.execute(
                update(Device)
                .where(Device.id == device_id)
                .values(status=DeviceStatus.OFFLINE)
            )
            await db.commit()


@router.websocket("/ws/tasks/{task_id}")
async def task_websocket(websocket: WebSocket, task_id: str):
    await websocket.accept()
    await manager.subscribe_task(task_id, websocket)
    try:
        while True:
            await websocket.receive_text()
    except WebSocketDisconnect:
        pass
    finally:
        await manager.unsubscribe_task(task_id, websocket)
