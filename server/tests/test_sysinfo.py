"""设备信息采集与弹窗任务测试"""
import json
import os
import sys

import pytest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))


def _hb(**over):
    body = {
        "hostname": "host", "os": "Windows 10", "ip": "10.0.0.1",
        "rearm_count": 0, "status": "online", "agent_version": "1.4.1",
        "timestamp": 0,
    }
    body.update(over)
    return body


SAMPLE_SYSINFO = {
    "cpu_model": "Intel Core i7-12700",
    "cpu_cores": 12,
    "memory_total_mb": 16384,
    "disk_total_gb": 512,
    "disk_free_gb": 320,
    "os_version": "Windows 10 Pro 10.0.19045",
    "serial_number": "SN-ABCD1234",
    "mac_address": "00:1A:2B:3C:4D:5E",
    "manufacturer": "Lenovo",
    "model": "ThinkCentre M720",
}


# ---------- 心跳携带硬件信息 ----------

async def test_heartbeat_persists_sysinfo(client):
    res = await client.post(
        "/api/agents/heartbeat",
        json=_hb(hostname="info-pc", ip="10.40.0.1", sysinfo=SAMPLE_SYSINFO),
        headers={"Authorization": "Bearer t1", "X-Enroll-Secret": "test-enroll"},
    )
    assert res.status_code == 200

    # 通过设备接口读取，确认字段已落库
    login = await client.post("/api/auth/login",
                              json={"username": "admin", "password": "admin123"})
    client.headers.update({"Authorization": f"Bearer {login.json()['access_token']}"})
    devs = (await client.get("/api/devices")).json()
    dev = next(d for d in devs if d["ip"] == "10.40.0.1")

    assert dev["cpu_model"] == "Intel Core i7-12700"
    assert dev["cpu_cores"] == 12
    assert dev["memory_total_mb"] == 16384
    assert dev["disk_total_gb"] == 512
    assert dev["disk_free_gb"] == 320
    assert dev["serial_number"] == "SN-ABCD1234"
    assert dev["mac_address"] == "00:1A:2B:3C:4D:5E"
    assert dev["manufacturer"] == "Lenovo"
    assert dev["model"] == "ThinkCentre M720"
    assert dev["sysinfo_updated_at"] is not None


async def test_heartbeat_without_sysinfo_keeps_existing(auth_client, make_device):
    """不带 sysinfo 的心跳不应清空已采集的信息"""
    from models.models import Device
    from sqlalchemy import select

    did = await make_device(ip="10.40.0.2", hostname="keep", token="tok2")
    await auth_client.post(
        "/api/agents/heartbeat",
        json=_hb(hostname="keep", ip="10.40.0.2", sysinfo=SAMPLE_SYSINFO),
        headers={"Authorization": "Bearer tok2"},
    )
    # 再发一次不带 sysinfo 的心跳
    await auth_client.post(
        "/api/agents/heartbeat",
        json=_hb(hostname="keep", ip="10.40.0.2"),
        headers={"Authorization": "Bearer tok2"},
    )
    devs = (await auth_client.get("/api/devices")).json()
    dev = next(d for d in devs if d["id"] == did)
    assert dev["cpu_model"] == "Intel Core i7-12700", "已有信息不应被清空"


async def test_partial_sysinfo_accepted(client):
    """只上报部分字段也应接受"""
    res = await client.post(
        "/api/agents/heartbeat",
        json=_hb(ip="10.40.0.3", sysinfo={"cpu_model": "AMD Ryzen 5"}),
        headers={"Authorization": "Bearer t3", "X-Enroll-Secret": "test-enroll"},
    )
    assert res.status_code == 200


# ---------- 新任务类型 ----------

async def test_hostmsg_task_accepted(auth_client, make_device):
    from models.models import DeviceStatus
    did = await make_device(ip="10.41.0.1", status=DeviceStatus.ONLINE)
    res = await auth_client.post("/api/tasks", json={
        "task_type": "hostmsg",
        "device_ids": [did],
        "params": {"timeout": 15, "title": "提示", "message": "内容"},
    })
    assert res.status_code == 200
    detail = (await auth_client.get(f"/api/tasks/{res.json()['id']}")).json()
    assert detail["task_type"] == "hostmsg"


async def test_sysinfo_task_accepted(auth_client, make_device):
    from models.models import DeviceStatus
    did = await make_device(ip="10.41.0.2", status=DeviceStatus.ONLINE)
    res = await auth_client.post("/api/tasks", json={
        "task_type": "sysinfo", "device_ids": [did],
    })
    assert res.status_code == 200


async def test_hostmsg_task_description(auth_client, make_device):
    from models.models import DeviceStatus
    did = await make_device(ip="10.41.0.3", status=DeviceStatus.ONLINE)
    res = await auth_client.post("/api/tasks", json={
        "task_type": "hostmsg", "device_ids": [did],
    })
    detail = (await auth_client.get(f"/api/tasks/{res.json()['id']}")).json()
    assert detail["description"] == "弹出信息提示"


async def test_sysinfo_task_description(auth_client, make_device):
    from models.models import DeviceStatus
    did = await make_device(ip="10.41.0.4", status=DeviceStatus.ONLINE)
    res = await auth_client.post("/api/tasks", json={
        "task_type": "sysinfo", "device_ids": [did],
    })
    detail = (await auth_client.get(f"/api/tasks/{res.json()['id']}")).json()
    assert detail["description"] == "采集设备信息"


# ---------- sysinfo 结果回写 ----------

async def test_store_sysinfo_output_writes_device(session_factory, make_device):
    """Agent 通过 WebSocket 回传的 sysinfo JSON 应写回设备记录"""
    from api.ws_routes import _store_sysinfo_output

    did = await make_device(ip="10.42.0.1", hostname="ws-pc")
    async with session_factory() as db:
        await _store_sysinfo_output(db, did, json.dumps(SAMPLE_SYSINFO))

    from sqlalchemy import select
    from models.models import Device
    async with session_factory() as db:
        dev = (await db.execute(select(Device).where(Device.id == did))).scalar_one()
        assert dev.cpu_model == "Intel Core i7-12700"
        assert dev.memory_total_mb == 16384
        assert dev.sysinfo_updated_at is not None


async def test_store_sysinfo_ignores_bad_json(session_factory, make_device):
    """非 JSON 输出不应导致异常"""
    from api.ws_routes import _store_sysinfo_output
    did = await make_device(ip="10.42.0.2")
    async with session_factory() as db:
        await _store_sysinfo_output(db, did, "not json at all")
        await _store_sysinfo_output(db, did, "[1,2,3]")   # 非 dict
        await _store_sysinfo_output(db, did, "{}")        # 空对象


async def test_store_sysinfo_ignores_unknown_fields(session_factory, make_device):
    """未知字段应被忽略，不污染模型"""
    from api.ws_routes import _store_sysinfo_output
    from sqlalchemy import select
    from models.models import Device

    did = await make_device(ip="10.42.0.3")
    async with session_factory() as db:
        await _store_sysinfo_output(
            db, did, json.dumps({"cpu_model": "X", "evil_field": "boom"})
        )
    async with session_factory() as db:
        dev = (await db.execute(select(Device).where(Device.id == did))).scalar_one()
        assert dev.cpu_model == "X"
        assert not hasattr(dev, "evil_field")


# ---------- 数据模型迁移 ----------

def test_migration_adds_missing_columns(tmp_path):
    """旧库缺列时，_migrate 应自动补齐新列"""
    from sqlalchemy import create_engine, inspect, text
    from database import _migrate

    db_file = tmp_path / "legacy.db"
    eng = create_engine(f"sqlite:///{db_file}")
    with eng.begin() as conn:
        # 模拟旧表结构：只有基础列
        conn.execute(text("""
            CREATE TABLE devices (
                id VARCHAR(36) PRIMARY KEY,
                ip VARCHAR(45),
                hostname VARCHAR(255),
                status VARCHAR(20)
            )
        """))
        _migrate(conn)

        cols = {c["name"] for c in inspect(conn).get_columns("devices")}
        for expected in ["cpu_model", "cpu_cores", "memory_total_mb",
                         "disk_total_gb", "disk_free_gb", "os_version",
                         "serial_number", "mac_address", "manufacturer",
                         "model", "sysinfo_updated_at"]:
            assert expected in cols, f"缺少迁移列 {expected}"


def test_migration_idempotent(tmp_path):
    """重复执行迁移不应报错"""
    from sqlalchemy import create_engine, inspect, text
    from database import _migrate

    db_file = tmp_path / "idem.db"
    eng = create_engine(f"sqlite:///{db_file}")
    with eng.begin() as conn:
        conn.execute(text("CREATE TABLE devices (id VARCHAR(36) PRIMARY KEY)"))
        _migrate(conn)
        _migrate(conn)
        cols = {c["name"] for c in inspect(conn).get_columns("devices")}
        assert "cpu_model" in cols


def test_migration_skips_when_no_table(tmp_path):
    """完全没有 devices 表时不应报错"""
    from sqlalchemy import create_engine
    from database import _migrate

    eng = create_engine(f"sqlite:///{tmp_path/'empty.db'}")
    with eng.begin() as conn:
        _migrate(conn)  # 不应抛出