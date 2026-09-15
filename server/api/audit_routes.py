from fastapi import APIRouter, Depends, Query
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func

from database import get_db
from models.models import AuditLog
from api.auth import get_current_user

router = APIRouter(prefix="/api/audit", tags=["audit"])


@router.get("")
async def list_audit_logs(
    page: int = Query(1, ge=1),
    size: int = Query(50, ge=1, le=200),
    action: str | None = None,
    username: str | None = None,
    db: AsyncSession = Depends(get_db),
    _: dict = Depends(get_current_user),
):
    query = select(AuditLog)
    count_query = select(func.count(AuditLog.id))

    if action:
        query = query.where(AuditLog.action == action)
        count_query = count_query.where(AuditLog.action == action)
    if username:
        query = query.where(AuditLog.username == username)
        count_query = count_query.where(AuditLog.username == username)

    total_result = await db.execute(count_query)
    total = total_result.scalar()

    query = query.order_by(AuditLog.created_at.desc()).offset((page - 1) * size).limit(size)
    result = await db.execute(query)
    logs = result.scalars().all()

    return {
        "total": total,
        "page": page,
        "size": size,
        "items": [
            {
                "id": l.id,
                "username": l.username,
                "action": l.action,
                "target": l.target,
                "detail": l.detail,
                "ip_address": l.ip_address,
                "created_at": l.created_at,
            }
            for l in logs
        ],
    }
