import os
from datetime import datetime, timezone, timedelta

BASE_DIR = os.path.dirname(os.path.abspath(__file__))
# 全局版本号唯一来源；可通过环境变量覆盖
APP_VERSION = os.environ.get("LANAGENT_VERSION", "1.4.3")
DATABASE_URL = f"sqlite+aiosqlite:///{os.path.join(BASE_DIR, 'lanagent.db')}"
SECRET_KEY = os.environ.get("LANAGENT_SECRET", "change-me-in-production")
# Agent 首次注册（enroll）所需的预共享密钥；未配置时按 SECRET_KEY 派生
ENROLL_SECRET = os.environ.get("LANAGENT_ENROLL_SECRET", "")
TOKEN_EXPIRE_MINUTES = 60 * 24 * 30
MAX_CONCURRENT_TASKS = 10
HEARTBEAT_TIMEOUT_SECONDS = 15

BJT = timezone(timedelta(hours=8))


def now_bjt() -> datetime:
    """返回北京时间的 naive datetime（墙钟时间），与 DateTime 列保持一致。"""
    return datetime.now(BJT).replace(tzinfo=None)
