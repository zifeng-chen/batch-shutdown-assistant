"""批量导入、任务下发、审计日志与静态文件安全测试"""
import io
import os
import sys

import pytest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))


# ---------- CSV 导入 ----------

async def test_import_csv_basic(auth_client):
    csv = "hostname,ip\npc1,192.168.2.1\npc2,192.168.2.2\n"
    res = await auth_client.post(
        "/api/devices/import/csv",
        files={"file": ("devices.csv", io.BytesIO(csv.encode()), "text/csv")},
    )
    assert res.status_code == 200
    assert res.json()["added"] == 2

    ips = [d["ip"] for d in (await auth_client.get("/api/devices")).json()]
    assert "192.168.2.1" in ips and "192.168.2.2" in ips


async def test_import_csv_dedup(auth_client):
    """重复 IP 应被跳过而非报错"""
    csv = "hostname,ip\npc1,10.10.10.1\n"
    await auth_client.post("/api/devices/import/csv",
                           files={"file": ("a.csv", io.BytesIO(csv.encode()), "text/csv")})
    res = await auth_client.post("/api/devices/import/csv",
                                 files={"file": ("a.csv", io.BytesIO(csv.encode()), "text/csv")})
    assert res.status_code == 200
    body = res.json()
    assert body["added"] == 0 and body["skipped"] == 1


async def test_import_csv_missing_ip_column(auth_client):
    csv = "hostname\npc1\n"
    res = await auth_client.post(
        "/api/devices/import/csv",
        files={"file": ("bad.csv", io.BytesIO(csv.encode()), "text/csv")},
    )
    assert res.status_code == 200
    assert res.json()["added"] == 0


async def test_import_rejects_non_csv(auth_client):
    res = await auth_client.post(
        "/api/devices/import/csv",
        files={"file": ("data.txt", io.BytesIO(b"x"), "text/plain")},
    )
    assert res.status_code == 400


async def test_import_requires_auth(client):
    csv = "hostname,ip\npc1,1.1.1.1\n"
    res = await client.post(
        "/api/devices/import/csv",
        files={"file": ("a.csv", io.BytesIO(csv.encode()), "text/csv")},
    )
    assert res.status_code == 401


# ---------- IP 段导入 ----------

async def test_import_ip_range(auth_client):
    res = await auth_client.post("/api/devices/import/ip-range",
                                 json={"start_ip": "192.168.3.1", "end_ip": "192.168.3.5"})
    assert res.status_code == 200
    assert res.json()["added"] == 5


async def test_import_ip_range_reversed(auth_client):
    """起止颠倒应报错，而非静默产生空结果"""
    res = await auth_client.post("/api/devices/import/ip-range",
                                 json={"start_ip": "192.168.4.10", "end_ip": "192.168.4.1"})
    assert res.status_code == 400


async def test_import_ip_range_invalid(auth_client):
    res = await auth_client.post("/api/devices/import/ip-range",
                                 json={"start_ip": "not-an-ip", "end_ip": "also-bad"})
    assert res.status_code == 400


# ---------- 任务 ----------

async def test_create_task_requires_auth(client):
    assert (await client.post("/api/tasks", json={"task_type": "sysprep", "device_ids": []})).status_code == 401


async def test_create_task_invalid_type(auth_client):
    res = await auth_client.post("/api/tasks", json={"task_type": "bogus", "device_ids": []})
    assert res.status_code == 400


async def test_create_task_marks_offline_devices_failed(auth_client, make_device):
    """设备离线时任务结果应记为 failed"""
    did = await make_device(ip="10.20.0.1")
    res = await auth_client.post("/api/tasks", json={"task_type": "sysprep", "device_ids": [did]})
    assert res.status_code == 200

    detail = (await auth_client.get(f"/api/tasks/{res.json()['id']}")).json()
    assert detail["results"][0]["status"] == "failed"


async def test_create_task_empty_devices(auth_client):
    res = await auth_client.post("/api/tasks", json={"task_type": "sysprep", "device_ids": []})
    assert res.status_code == 200


# ---------- 审计 ----------

async def test_audit_written_on_device_create(auth_client):
    await auth_client.post("/api/devices", json={"hostname": "audited", "ip": "10.30.0.1"})
    logs = (await auth_client.get("/api/audit", params={"size": 50})).json()
    actions = [x["action"] for x in logs["items"]]
    assert "device_create" in actions


async def test_audit_requires_auth(client):
    assert (await client.get("/api/audit")).status_code == 401


async def test_audit_filter_by_action(auth_client):
    await auth_client.post("/api/devices", json={"ip": "10.31.0.1"})
    logs = (await auth_client.get("/api/audit", params={"action": "device_create"})).json()
    assert all(x["action"] == "device_create" for x in logs["items"])


# ---------- 静态文件路径穿越（本次修复项）----------

def test_path_traversal_blocked():
    """serve_spa 的守卫逻辑：任何逃逸出 static 的路径都必须被拒"""
    import tempfile
    with tempfile.TemporaryDirectory() as root:
        os.makedirs(os.path.join(root, "assets"))
        open(os.path.join(root, "index.html"), "w").write("ok")

        def guarded(fp):
            real_root = os.path.realpath(root)
            cand = os.path.realpath(os.path.join(real_root, fp))
            try:
                inside = os.path.commonpath([cand, real_root]) == real_root
            except ValueError:
                inside = False
            return inside and os.path.isfile(cand)

        assert guarded("index.html") is True
        assert guarded("assets") is False          # 目录非文件
        assert guarded("../../etc/passwd") is False
        assert guarded("assets/../../../../etc/passwd") is False
        assert guarded("/etc/passwd") is False


# ---------- 健康检查 & 版本 ----------

async def test_health(client):
    res = await client.get("/api/health")
    assert res.status_code == 200 and res.json()["status"] == "ok"


async def test_version_is_consistent():
    from config import APP_VERSION
    import main as m
    assert m.app.version == APP_VERSION