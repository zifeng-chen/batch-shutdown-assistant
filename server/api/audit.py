import uuid
from datetime import datetime, timedelta
from config import now_bjt

from sqlalchemy.ext.asyncio import AsyncSession

from models.models import AuditLog


async def log_audit(
    db: AsyncSession,
    action: str,
    username: str | None = None,
    target: str | None = None,
    detail: str | None = None,
    ip_address: str | None = None,
):
    entry = AuditLog(
        id=str(uuid.uuid4()),
        username=username,
        action=action,
        target=target,
        detail=detail,
        ip_address=ip_address,
        created_at=now_bjt(),
    )
    db.add(entry)
    await db.commit()
