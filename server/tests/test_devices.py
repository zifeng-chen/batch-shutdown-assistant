"""设备管理接口测试：CRUD、分页、搜索、IP 数值排序"""
import os
import sys

import pytest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from api.device_routes import ip_sort_key


# ---------- IP 排序键（本次修复项）----------

class _Fake:
    def __init__(self, ip, hostname=""):
        self.ip = ip
        self.hostname = hostname


def test_ip_sort_cross_subnet():
    """跨网段排序必须按数值，而非字符串长度 —— 这是修复前的 bug"""
    raw = ["192.168.1.10", "192.168.1.2", "192.168.1.100",
           "10.0.0.1", "192.168.1.9", "172.16.5.4",
           "192.168.10.1", "192.168.9.1"]
    got = [d.ip for d in sorted([_Fake(x) for x in raw], key=ip_sort_key)]
    assert got == ["10.0.0.1", "172.16.5.4", "192.168.1.2", "192.168.1.9",
                   "192.168.1.10", "192.168.1.100", "192.168.9.1", "192.168.10.1"]


def test_ip_sort_invalid_last():
    devs = [_Fake(None), _Fake("192.168.1.1"), _Fake("bad-ip"), _Fake("10.0.0.1")]
    got = [d.ip for d in sorted(devs, key=ip_sort_key)]
    assert got[:2] == ["10.0.0.1", "192.168.1.1"]
    assert set(got[2:]) == {None, "bad-ip"}


def test_ip_sort_stable_by_hostname():
    a = _Fake("10.0.0.1", "b-host")
    b = _Fake("10.0.0.1", "a-host")
    assert [d.hostname for d in sorted([a, b], key=ip_sort_key)] == ["a-host", "b-host"]


# ---------- 鉴权 ----------

async def test_list_devices_requires_auth(client):
    assert (await client.get("/api/devices")).status_code == 401


async def test_stats_requires_auth(client):
    assert (await client.get("/api/devices/stats")).status_code == 401


# ---------- CRUD ----------

async def test_create_and_list(auth_client):
    res = await auth_client.post("/api/devices", json={"hostname": "pc1", "ip": "192.168.1.5"})
    assert res.status_code == 200
    body = res.json()
    assert body["hostname"] == "pc1"
    assert body["status"] == "offline"

    lst = await auth_client.get("/api/devices")
    assert lst.status_code == 200
    assert any(d["id"] == body["id"] for d in lst.json())


async def test_update_device(auth_client):
    dev = (await auth_client.post("/api/devices", json={"hostname": "old", "ip": "10.0.0.1"})).json()
    res = await auth_client.put(f"/api/devices/{dev['id']}", json={"hostname": "new"})
    assert res.status_code == 200
    assert res.json()["hostname"] == "new"


async def test_update_missing_device_404(auth_client):
    res = await auth_client.put("/api/devices/does-not-exist", json={"hostname": "x"})
    assert res.status_code == 404


async def test_delete_is_soft(auth_client):
    dev = (await auth_client.post("/api/devices", json={"ip": "10.0.0.2"})).json()
    assert (await auth_client.delete(f"/api/devices/{dev['id']}")).status_code == 200

    lst = (await auth_client.get("/api/devices")).json()
    assert all(d["id"] != dev["id"] for d in lst), "软删除后不应出现在列表中"


async def test_delete_missing_404(auth_client):
    assert (await auth_client.delete("/api/devices/nope")).status_code == 404


async def test_batch_delete(auth_client):
    ids = []
    for i in range(3):
        d = (await auth_client.post("/api/devices", json={"ip": f"10.2.0.{i}"})).json()
        ids.append(d["id"])
    res = await auth_client.post("/api/devices/batch-delete", json={"device_ids": ids})
    assert res.status_code == 200
    assert res.json()["deleted"] == 3

    remaining = (await auth_client.get("/api/devices")).json()
    assert not any(d["id"] in ids for d in remaining)


# ---------- 列表能力 ----------

async def test_list_sorted_by_ip(auth_client):
    for ip in ["192.168.1.100", "192.168.1.2", "10.0.0.1"]:
        await auth_client.post("/api/devices", json={"ip": ip})
    got = [d["ip"] for d in (await auth_client.get("/api/devices")).json()]
    assert got == sorted(got, key=lambda x: tuple(int(p) for p in x.split(".")))


async def test_search_filter(auth_client):
    await auth_client.post("/api/devices", json={"hostname": "alpha-pc", "ip": "10.5.0.1"})
    await auth_client.post("/api/devices", json={"hostname": "beta-pc", "ip": "10.5.0.2"})
    got = (await auth_client.get("/api/devices", params={"search": "alpha"})).json()
    assert len(got) == 1 and got[0]["hostname"] == "alpha-pc"


async def test_pagination(auth_client):
    for i in range(5):
        await auth_client.post("/api/devices", json={"ip": f"10.6.0.{i}"})
    page1 = (await auth_client.get("/api/devices", params={"page": 1, "size": 2})).json()
    page2 = (await auth_client.get("/api/devices", params={"page": 2, "size": 2})).json()
    assert len(page1) == 2 and len(page2) == 2
    assert {d["id"] for d in page1}.isdisjoint({d["id"] for d in page2})


async def test_size_upper_bound(auth_client):
    assert (await auth_client.get("/api/devices", params={"size": 99999})).status_code == 422
    assert (await auth_client.get("/api/devices", params={"size": 5000})).status_code == 200


async def test_stats_counts(auth_client, make_device):
    from models.models import DeviceStatus
    await make_device(ip="10.7.0.1", status=DeviceStatus.ONLINE)
    await make_device(ip="10.7.0.2", status=DeviceStatus.OFFLINE)
    stats = (await auth_client.get("/api/devices/stats")).json()
    assert stats["total"] == 2
    assert stats["online"] == 1
    assert stats["offline"] == 1