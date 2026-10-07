"""并发注册与去重测试

覆盖缺陷：Agent 启动瞬间并发心跳导致同一设备产生多条记录。
"""
import asyncio
import os
import sqlite3
import sys

import pytest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from tests.conftest import TEST_DB


def _hb(**over):
    body = {
        "hostname": "host", "os": "Windows 10", "ip": "10.0.0.1",
        "rearm_count": 0, "status": "online", "agent_version": "1.4.3",
        "timestamp": 0,
    }
    body.update(over)
    return body


def _count_by_ip(ip):
    conn = sqlite3.connect(TEST_DB)
    n = conn.execute(
        "SELECT COUNT(*) FROM devices WHERE ip=? AND deleted_at IS NULL", (ip,)
    ).fetchone()[0]
    conn.close()
    return n


async def test_concurrent_registration_no_duplicates(client):
    """10 个并发注册请求只应产生 1 条记录"""
    ip = "10.60.0.1"
    headers = {"Authorization": "Bearer tok", "X-Enroll-Secret": "test-enroll"}

    results = await asyncio.gather(*[
        client.post("/api/agents/heartbeat", json=_hb(ip=ip), headers=headers)
        for _ in range(10)
    ])

    # 所有请求都应成功
    assert all(r.status_code == 200 for r in results)

    # 但只应有一条设备记录
    assert _count_by_ip(ip) == 1, f"并发注册产生了 {_count_by_ip(ip)} 条记录"


async def test_concurrent_returns_same_device_id(client):
    """并发注册应返回同一个 device_id"""
    ip = "10.60.0.2"
    headers = {"Authorization": "Bearer tok2", "X-Enroll-Secret": "test-enroll"}

    results = await asyncio.gather(*[
        client.post("/api/agents/heartbeat", json=_hb(ip=ip), headers=headers)
        for _ in range(8)
    ])
    ids = {r.json()["device_id"] for r in results}
    assert len(ids) == 1, f"同一设备返回了多个 ID: {ids}"


async def test_concurrent_heartbeat_existing_device(client, make_device):
    """已存在设备的并发心跳不应新增记录"""
    ip = "10.60.0.3"
    did = await make_device(ip=ip, hostname="existing", token="known")

    results = await asyncio.gather(*[
        client.post("/api/agents/heartbeat",
                    json=_hb(device_id=did, ip=ip),
                    headers={"Authorization": "Bearer known"})
        for _ in range(10)
    ])
    assert all(r.status_code == 200 for r in results)
    assert _count_by_ip(ip) == 1


async def test_concurrent_reinstall_no_duplicates(client, make_device):
    """重装场景下的并发心跳也应只保留一条记录"""
    ip = "10.60.0.4"
    await make_device(ip=ip, hostname="reinst", token="old-token")

    results = await asyncio.gather(*[
        client.post("/api/agents/heartbeat",
                    json=_hb(device_id="new-uuid", ip=ip),
                    headers={"Authorization": "Bearer new-token",
                             "X-Enroll-Secret": "test-enroll"})
        for _ in range(10)
    ])
    assert all(r.status_code == 200 for r in results)
    assert _count_by_ip(ip) == 1, f"重装并发产生了 {_count_by_ip(ip)} 条记录"


# ---------- 启动时自动去重 ----------

def test_migrate_dedupes_existing_records(tmp_path):
    """迁移时应清理历史遗留的重复记录"""
    from sqlalchemy import create_engine, text
    from database import _dedupe_devices

    db_file = tmp_path / "dup.db"
    eng = create_engine(f"sqlite:///{db_file}")
    with eng.begin() as conn:
        conn.execute(text("""
            CREATE TABLE devices (
                id VARCHAR(36) PRIMARY KEY, ip VARCHAR(45), hostname VARCHAR(255),
                status VARCHAR(20), last_heartbeat DATETIME, deleted_at DATETIME
            )
        """))
        # 同 IP 三条记录，心跳时间不同
        conn.execute(text("INSERT INTO devices VALUES ('a','10.1.1.1','h','online','2026-01-01 10:00:00',NULL)"))
        conn.execute(text("INSERT INTO devices VALUES ('b','10.1.1.1','h','online','2026-01-01 12:00:00',NULL)"))
        conn.execute(text("INSERT INTO devices VALUES ('c','10.1.1.1','h','online','2026-01-01 11:00:00',NULL)"))
        # 其他 IP 不受影响
        conn.execute(text("INSERT INTO devices VALUES ('d','10.2.2.2','h2','online','2026-01-01 10:00:00',NULL)"))

        _dedupe_devices(conn)

        rows = conn.execute(text(
            "SELECT id, last_heartbeat FROM devices ORDER BY id"
        )).fetchall()

    ids = [r[0] for r in rows]
    assert "b" in ids, "应保留心跳最新的记录"
    assert "d" in ids, "其他 IP 的记录不应被误删"
    assert len([i for i in ids if i in ("a", "b", "c")]) == 1
    assert len(ids) == 2


def test_dedupe_keeps_deleted_separate(tmp_path):
    """已软删除的记录不参与去重"""
    from sqlalchemy import create_engine, text
    from database import _dedupe_devices

    db_file = tmp_path / "dup2.db"
    eng = create_engine(f"sqlite:///{db_file}")
    with eng.begin() as conn:
        conn.execute(text("""
            CREATE TABLE devices (
                id VARCHAR(36) PRIMARY KEY, ip VARCHAR(45), hostname VARCHAR(255),
                status VARCHAR(20), last_heartbeat DATETIME, deleted_at DATETIME
            )
        """))
        conn.execute(text("INSERT INTO devices VALUES ('live','10.3.3.3','h','online','2026-01-01 10:00:00',NULL)"))
        conn.execute(text("INSERT INTO devices VALUES ('dead','10.3.3.3','h','offline','2026-01-01 09:00:00','2026-01-01 09:30:00')"))

        _dedupe_devices(conn)

        n = conn.execute(text("SELECT COUNT(*) FROM devices")).scalar()
    assert n == 2, "软删除记录不应被去重删除"


def test_dedupe_handles_no_duplicates(tmp_path):
    """无重复时不应有任何改动"""
    from sqlalchemy import create_engine, text
    from database import _dedupe_devices

    db_file = tmp_path / "clean.db"
    eng = create_engine(f"sqlite:///{db_file}")
    with eng.begin() as conn:
        conn.execute(text("""
            CREATE TABLE devices (
                id VARCHAR(36) PRIMARY KEY, ip VARCHAR(45), hostname VARCHAR(255),
                status VARCHAR(20), last_heartbeat DATETIME, deleted_at DATETIME
            )
        """))
        conn.execute(text("INSERT INTO devices VALUES ('x','10.4.4.4','h','online','2026-01-01 10:00:00',NULL)"))
        _dedupe_devices(conn)
        n = conn.execute(text("SELECT COUNT(*) FROM devices")).scalar()
    assert n == 1