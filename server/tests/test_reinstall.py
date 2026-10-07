"""设备重装（token 轮换）自愈逻辑测试

覆盖此前的死锁缺陷：设备重装后带新 token + 正确注册密钥，应能接管自身记录，
而不是被 401 永久拒绝。
"""
import os
import sys

import pytest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))


def _hb(**over):
    body = {
        "hostname": "host", "os": "Windows 10", "ip": "10.0.0.1",
        "rearm_count": 0, "status": "online", "agent_version": "1.4.2",
        "timestamp": 0,
    }
    body.update(over)
    return body


# ---------- 正常心跳 ----------

async def test_normal_heartbeat_ok(client, make_device):
    did = await make_device(ip="10.50.0.1", hostname="normal", token="tok-good")
    res = await client.post(
        "/api/agents/heartbeat",
        json=_hb(device_id=did, hostname="normal", ip="10.50.0.1"),
        headers={"Authorization": "Bearer tok-good"},
    )
    assert res.status_code == 200
    assert res.json()["status"] == "updated"


async def test_heartbeat_refreshes_runtime_fields(client, make_device):
    from sqlalchemy import select
    from models.models import Device

    did = await make_device(ip="10.50.0.2", hostname="old-name", token="t")
    await client.post(
        "/api/agents/heartbeat",
        json=_hb(device_id=did, hostname="new-name", ip="10.50.0.2",
                 agent_version="1.4.2", rearm_count=3),
        headers={"Authorization": "Bearer t"},
    )
    res = await client.get("/api/devices")
    # 直接查库更可靠
    from tests.conftest import TEST_DB  # noqa
    import sqlite3
    conn = sqlite3.connect(TEST_DB)
    row = conn.execute(
        "SELECT hostname, rearm_count, agent_version FROM devices WHERE id=?", (did,)
    ).fetchone()
    conn.close()
    assert row[0] == "new-name"
    assert row[1] == 3
    assert row[2] == "1.4.2"


# ---------- 重装接管（核心修复）----------

async def test_reinstall_with_enroll_secret_recovers(client, make_device):
    """重装后 device_id 变了、token 也变了，但带正确密钥 → 应按 IP 找到旧记录并接管"""
    did = await make_device(ip="10.51.0.1", hostname="reinstall", token="old-token")

    # 重装后的 Agent：新 device_id（服务端不认识）、新 token、带正确密钥
    res = await client.post(
        "/api/agents/heartbeat",
        json=_hb(device_id="brand-new-uuid", hostname="reinstall",
                 ip="10.51.0.1", agent_version="1.4.2"),
        headers={
            "Authorization": "Bearer new-token",
            "X-Enroll-Secret": "test-enroll",
        },
    )
    assert res.status_code == 200, res.text
    assert res.json()["device_id"] == did, "应复用原记录而非新建"
    assert res.json()["status"] == "re-enrolled"


async def test_reinstall_updates_token(client, make_device):
    """接管后 token 应被更新为新值"""
    import sqlite3
    from tests.conftest import TEST_DB

    did = await make_device(ip="10.51.0.2", hostname="r2", token="old")
    await client.post(
        "/api/agents/heartbeat",
        json=_hb(device_id="other-uuid", ip="10.51.0.2"),
        headers={"Authorization": "Bearer fresh", "X-Enroll-Secret": "test-enroll"},
    )
    conn = sqlite3.connect(TEST_DB)
    token = conn.execute("SELECT token FROM devices WHERE id=?", (did,)).fetchone()[0]
    conn.close()
    assert token == "fresh"

    # 后续用新 token 心跳应正常
    res = await client.post(
        "/api/agents/heartbeat",
        json=_hb(device_id=did, ip="10.51.0.2"),
        headers={"Authorization": "Bearer fresh"},
    )
    assert res.status_code == 200
    assert res.json()["status"] == "updated"


async def test_reinstall_without_secret_rejected(client, make_device):
    """无正确密钥的 token 轮换必须被拒（防伪造）"""
    did = await make_device(ip="10.51.0.3", hostname="victim", token="real-token")
    res = await client.post(
        "/api/agents/heartbeat",
        json=_hb(device_id=did, ip="10.51.0.3", hostname="attacker"),
        headers={"Authorization": "Bearer forged-token"},
    )
    assert res.status_code == 401


async def test_reinstall_with_wrong_secret_rejected(client, make_device):
    did = await make_device(ip="10.51.0.4", hostname="victim2", token="real")
    res = await client.post(
        "/api/agents/heartbeat",
        json=_hb(device_id=did, ip="10.51.0.4"),
        headers={"Authorization": "Bearer forged", "X-Enroll-Secret": "wrong-secret"},
    )
    assert res.status_code == 401


# ---------- 软删除恢复 ----------

async def test_soft_deleted_device_recovers_with_secret(client, make_device):
    did = await make_device(ip="10.52.0.1", hostname="deleted", token="old", deleted=True)
    res = await client.post(
        "/api/agents/heartbeat",
        json=_hb(device_id=did, ip="10.52.0.1", hostname="deleted"),
        headers={"Authorization": "Bearer new-token", "X-Enroll-Secret": "test-enroll"},
    )
    assert res.status_code == 200
    assert res.json()["status"] == "re-enrolled"


async def test_soft_deleted_restored_appears_in_list(auth_client, make_device):
    did = await make_device(ip="10.52.0.2", token="old", deleted=True)
    await auth_client.post(
        "/api/agents/heartbeat",
        json=_hb(device_id=did, ip="10.52.0.2"),
        headers={"Authorization": "Bearer new", "X-Enroll-Secret": "test-enroll"},
    )
    ids = [d["id"] for d in (await auth_client.get("/api/devices")).json()]
    assert did in ids, "恢复后应重新出现在设备列表"


async def test_unregistered_new_device_needs_secret(client):
    """全新设备不带密钥应被拒"""
    res = await client.post(
        "/api/agents/heartbeat",
        json=_hb(ip="10.53.0.1", hostname="fresh"),
        headers={"Authorization": "Bearer any"},
    )
    assert res.status_code == 401


async def test_unregistered_new_device_with_secret_registers(client):
    res = await client.post(
        "/api/agents/heartbeat",
        json=_hb(ip="10.53.0.2", hostname="fresh2"),
        headers={"Authorization": "Bearer any", "X-Enroll-Secret": "test-enroll"},
    )
    assert res.status_code == 200
    assert res.json()["device_id"]


async def test_same_ip_reinstall_does_not_create_duplicate(client, make_device):
    """重装接管后，同 IP 不应出现两条记录"""
    import sqlite3
    from tests.conftest import TEST_DB

    await make_device(ip="10.54.0.1", hostname="dup", token="old")
    await client.post(
        "/api/agents/heartbeat",
        json=_hb(device_id="unknown-uuid", ip="10.54.0.1"),
        headers={"Authorization": "Bearer new", "X-Enroll-Secret": "test-enroll"},
    )
    conn = sqlite3.connect(TEST_DB)
    n = conn.execute("SELECT COUNT(*) FROM devices WHERE ip='10.54.0.1'").fetchone()[0]
    conn.close()
    assert n == 1, f"同 IP 应只有 1 条记录，实际 {n} 条"