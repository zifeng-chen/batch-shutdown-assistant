"""鉴权、密码策略与 Agent 注册校验测试"""
import os
import sys

import pytest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from api.auth import create_access_token, hash_password, verify_password
from api.auth_routes import validate_password_strength
from fastapi import HTTPException


# ---------- 密码哈希 ----------

def test_password_hash_roundtrip():
    h = hash_password("Secret123")
    assert h != "Secret123"
    assert verify_password("Secret123", h)
    assert not verify_password("wrong", h)


def test_hash_is_salted():
    assert hash_password("same") != hash_password("same")


# ---------- 密码强度 ----------

@pytest.mark.parametrize("pw,ok", [
    ("1234567", False),      # 太短
    ("12345678", False),     # 纯数字
    ("abcdefgh", False),     # 纯字母
    ("abcd1234", True),      # 合法
    ("A1b2C3d4", True),      # 合法
    ("a" * 129, False),      # 超长
])
def test_password_strength(pw, ok):
    if ok:
        validate_password_strength(pw)
    else:
        with pytest.raises(HTTPException):
            validate_password_strength(pw)


def test_password_strength_boundary_8():
    validate_password_strength("abcdefg1")  # 恰好 8 位且含数字 → 通过


# ---------- 登录 ----------

async def test_login_success(client, admin_user):
    res = await client.post("/api/auth/login", json=admin_user)
    assert res.status_code == 200
    assert res.json()["access_token"]
    assert res.json()["username"] == "admin"


async def test_login_wrong_password(client, admin_user):
    res = await client.post(
        "/api/auth/login", json={"username": "admin", "password": "bad"}
    )
    assert res.status_code == 401


async def test_login_unknown_user(client):
    res = await client.post(
        "/api/auth/login", json={"username": "nobody", "password": "x"}
    )
    assert res.status_code == 401


async def test_me_requires_token(client):
    assert (await client.get("/api/auth/me")).status_code == 401


async def test_me_with_token(auth_client):
    res = await auth_client.get("/api/auth/me")
    assert res.status_code == 200
    assert res.json()["username"] == "admin"


async def test_change_password_requires_strength(auth_client, admin_user):
    res = await auth_client.post("/api/auth/change-password", json={
        "old_password": admin_user["password"], "new_password": "weak",
    })
    assert res.status_code == 400


async def test_change_password_same_as_old(auth_client, admin_user):
    res = await auth_client.post("/api/auth/change-password", json={
        "old_password": admin_user["password"],
        "new_password": admin_user["password"],
    })
    assert res.status_code == 400


# ---------- Token ----------

def test_token_decode_roundtrip():
    from api.auth import decode_token
    tok = create_access_token({"sub": "alice", "role": "admin"})
    assert decode_token(tok)["sub"] == "alice"


def test_invalid_token_rejected():
    from api.auth import decode_token
    with pytest.raises(HTTPException):
        decode_token("not-a-jwt")


# ---------- Agent 心跳鉴权（本次修复项）----------

def _hb(**over):
    """心跳请求体，所有必填字段都有默认值"""
    body = {
        "hostname": "host", "os": "Windows 10", "ip": "10.0.0.1",
        "rearm_count": 0, "status": "online", "agent_version": "1.4.1",
        "timestamp": 0,
    }
    body.update(over)
    return body


async def test_heartbeat_token_mismatch_rejected(client, make_device):
    """存量设备的 token 不匹配时必须 401，防止伪造心跳"""
    await make_device(ip="10.0.0.5", hostname="victim", token="real-token")
    res = await client.post(
        "/api/agents/heartbeat",
        json=_hb(hostname="attacker", ip="10.0.0.5"),
        headers={"Authorization": "Bearer wrong-token"},
    )
    assert res.status_code == 401


async def test_heartbeat_correct_token_accepted(client, make_device):
    did = await make_device(ip="10.0.0.6", hostname="good", token="right-token")
    res = await client.post(
        "/api/agents/heartbeat",
        json=_hb(hostname="good", ip="10.0.0.6"),
        headers={"Authorization": "Bearer right-token"},
    )
    assert res.status_code == 200
    assert res.json()["device_id"] == did


async def test_new_device_requires_enroll_secret(client):
    """未知设备不带注册密钥必须被拒绝"""
    res = await client.post(
        "/api/agents/heartbeat",
        json=_hb(hostname="newbie", ip="10.9.9.9"),
        headers={"Authorization": "Bearer some-token"},
    )
    assert res.status_code == 401


async def test_new_device_with_enroll_secret_accepted(client):
    res = await client.post(
        "/api/agents/heartbeat",
        json=_hb(hostname="newbie", ip="10.9.9.10"),
        headers={
            "Authorization": "Bearer some-token",
            "X-Enroll-Secret": "test-enroll",
        },
    )
    assert res.status_code == 200
    assert res.json()["device_id"]


async def test_recover_requires_matching_token(client, make_device):
    did = await make_device(ip="10.1.1.1", hostname="h", token="dev-token")
    res = await client.post(
        "/api/agents/recover",
        json={"device_id": did, "hostname": "h", "ip": "10.1.1.1"},
        headers={"Authorization": "Bearer wrong"},
    )
    assert res.status_code == 401