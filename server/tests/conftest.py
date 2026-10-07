"""pytest 全局夹具：独立测试数据库 + 已鉴权客户端"""
import asyncio
import os
import sys
import uuid

import pytest
import pytest_asyncio
from httpx import ASGITransport, AsyncClient
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine

# 让测试能 import server 下的模块
SERVER_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, SERVER_DIR)

os.environ.setdefault("LANAGENT_SECRET", "test-secret")
os.environ.setdefault("LANAGENT_ENROLL_SECRET", "test-enroll")

from database import Base, get_db  # noqa: E402
from models.models import User  # noqa: E402
from api.auth import hash_password, create_access_token  # noqa: E402
import main as app_module  # noqa: E402

TEST_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "test_lanagent.db")


@pytest.fixture(scope="session")
def event_loop():
    loop = asyncio.new_event_loop()
    yield loop
    loop.close()


@pytest_asyncio.fixture
async def engine():
    if os.path.exists(TEST_DB):
        os.remove(TEST_DB)
    eng = create_async_engine(f"sqlite+aiosqlite:///{TEST_DB}", echo=False)
    async with eng.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    yield eng
    await eng.dispose()
    if os.path.exists(TEST_DB):
        os.remove(TEST_DB)


@pytest_asyncio.fixture
async def session_factory(engine):
    return async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)


@pytest_asyncio.fixture
async def db(session_factory):
    async with session_factory() as s:
        yield s


@pytest_asyncio.fixture
async def admin_user(session_factory):
    async with session_factory() as s:
        user = User(
            id=str(uuid.uuid4()),
            username="admin",
            password_hash=hash_password("admin123"),
            role="admin",
        )
        s.add(user)
        await s.commit()
    return {"username": "admin", "password": "admin123"}


@pytest_asyncio.fixture
async def client(session_factory, admin_user):
    """覆盖 get_db 依赖，指向测试库，并禁用 lifespan 中的后台任务与静态目录"""
    async def _override_get_db():
        async with session_factory() as s:
            yield s

    app_module.app.dependency_overrides[get_db] = _override_get_db

    transport = ASGITransport(app=app_module.app)
    async with AsyncClient(transport=transport, base_url="http://test") as c:
        yield c

    app_module.app.dependency_overrides.clear()


@pytest_asyncio.fixture
async def auth_client(client, admin_user):
    """已登录的客户端，自动携带 Bearer token"""
    res = await client.post(
        "/api/auth/login",
        json={"username": admin_user["username"], "password": admin_user["password"]},
    )
    assert res.status_code == 200, res.text
    token = res.json()["access_token"]
    client.headers.update({"Authorization": f"Bearer {token}"})
    return client


@pytest.fixture
def auth_token(admin_user):
    return create_access_token({"sub": admin_user["username"], "role": "admin"})


@pytest_asyncio.fixture
async def make_device(session_factory):
    """快速插入设备的工厂夹具"""
    from models.models import Device, DeviceStatus

    async def _make(ip="192.168.1.1", hostname="host", status=DeviceStatus.OFFLINE,
                    version=None, token=None, deleted=False):
        async with session_factory() as s:
            d = Device(
                id=str(uuid.uuid4()),
                ip=ip,
                hostname=hostname,
                status=status,
                agent_version=version,
                token=token or str(uuid.uuid4()),
                deleted_at=None,
            )
            if deleted:
                from config import now_bjt
                d.deleted_at = now_bjt()
            s.add(d)
            await s.commit()
            return d.id

    return _make