import uuid
from datetime import datetime, timedelta
from config import now_bjt

from sqlalchemy import delete, select
from sqlalchemy.ext.asyncio import AsyncSession

from models.models import AuditLog

# 审计日志保留天数
AUDIT_RETENTION_DAYS = 180
# 审计日志最大条数（超出时删除最旧的）
AUDIT_MAX_ROWS = 200_000


async def purge_old_audits(db: AsyncSession) -> int:
    """删除超期或超量的审计日志，返回删除条数。"""
    cutoff = now_bjt() - timedelta(days=AUDIT_RETENTION_DAYS)
    result = await db.execute(delete(AuditLog).where(AuditLog.created_at < cutoff))
    deleted = result.rowcount or 0

    count = (await db.execute(select(AuditLog.id))).scalars().all()
    if len(count) > AUDIT_MAX_ROWS:
        excess = len(count) - AUDIT_MAX_ROWS
        oldest = (
            await db.execute(select(AuditLog.id).order_by(AuditLog.created_at).limit(excess))
        ).scalars().all()
        if oldest:
            await db.execute(delete(AuditLog).where(AuditLog.id.in_(oldest)))
            deleted += len(oldest)

    await db.commit()
    return deleted


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
