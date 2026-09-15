import os
from datetime import datetime, timezone, timedelta

BASE_DIR = os.path.dirname(os.path.abspath(__file__))
DATABASE_URL = f"sqlite+aiosqlite:///{os.path.join(BASE_DIR, 'lanagent.db')}"
SECRET_KEY = os.environ.get("LANAGENT_SECRET", "change-me-in-production")
TOKEN_EXPIRE_MINUTES = 60 * 24 * 30
MAX_CONCURRENT_TASKS = 10
HEARTBEAT_TIMEOUT_SECONDS = 15

BJT = timezone(timedelta(hours=8))


def now_bjt() -> datetime:
    return datetime.now(BJT)
