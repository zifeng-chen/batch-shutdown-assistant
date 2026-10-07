from sqlalchemy.ext.asyncio import create_async_engine, async_sessionmaker, AsyncSession
from sqlalchemy.orm import DeclarativeBase

from config import DATABASE_URL

engine = create_async_engine(DATABASE_URL, echo=False)
async_session = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)


class Base(DeclarativeBase):
    pass


async def get_db() -> AsyncSession:
    async with async_session() as session:
        yield session


async def init_db():
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
        await conn.run_sync(_migrate)


def _migrate(conn):
    """轻量迁移：为已存在的表补齐新增列，并消除历史重复记录"""
    from sqlalchemy import inspect, text

    inspector = inspect(conn)
    if "devices" not in inspector.get_table_names():
        return

    existing = {c["name"] for c in inspector.get_columns("devices")}
    additions = {
        "cpu_model": "VARCHAR(255)",
        "cpu_cores": "INTEGER",
        "memory_total_mb": "INTEGER",
        "disk_total_gb": "INTEGER",
        "disk_free_gb": "INTEGER",
        "os_version": "VARCHAR(255)",
        "serial_number": "VARCHAR(255)",
        "mac_address": "VARCHAR(255)",
        "manufacturer": "VARCHAR(255)",
        "model": "VARCHAR(255)",
        "sysinfo_updated_at": "DATETIME",
        "hostname_locked": "BOOLEAN DEFAULT 0",
    }
    for col, coltype in additions.items():
        if col not in existing:
            conn.execute(text(f"ALTER TABLE devices ADD COLUMN {col} {coltype}"))

    _dedupe_devices(conn)


def _dedupe_devices(conn):
    """清理同一 IP 的历史重复记录，保留心跳最新的一条。

    早期版本存在并发注册竞态，会产生重复设备记录。
    """
    from sqlalchemy import text

    try:
        rows = conn.execute(text(
            "SELECT ip FROM devices WHERE ip IS NOT NULL AND deleted_at IS NULL "
            "GROUP BY ip HAVING COUNT(*) > 1"
        )).fetchall()
    except Exception:
        return

    for (ip,) in rows:
        ids = conn.execute(text(
            "SELECT id FROM devices WHERE ip = :ip AND deleted_at IS NULL "
            "ORDER BY last_heartbeat DESC"
        ), {"ip": ip}).fetchall()
        for (dup_id,) in ids[1:]:
            conn.execute(text("DELETE FROM devices WHERE id = :id"), {"id": dup_id})
