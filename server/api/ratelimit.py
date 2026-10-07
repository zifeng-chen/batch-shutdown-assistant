"""登录失败限流：进程内按 (IP, username) 计数，超阈值锁定一段时间并指数退避。

无外部依赖（不引入 Redis），适用于单进程部署。多进程/多副本部署时应改用共享存储。
"""

import time
import threading

# 连续失败多少次后锁定
MAX_FAILURES = 5
# 首次锁定时长（秒）
BASE_LOCK_SECONDS = 300
# 锁定上限（秒）
MAX_LOCK_SECONDS = 3600
# 失败计数窗口（秒）：超过该时间无新失败则计数归零
FAILURE_WINDOW_SECONDS = 900

_lock = threading.Lock()
# key -> {"count": int, "first_at": float, "last_at": float, "locked_until": float, "lock_level": int}
_state: dict[str, dict] = {}


def _key(ip: str, username: str) -> str:
    return f"{ip}|{username or ''}"


def check_allowed(ip: str, username: str) -> int:
    """返回允许登录前的剩余等待秒数；0 表示未被锁定、可以尝试。"""
    now = time.time()
    with _lock:
        st = _state.get(_key(ip, username))
        if not st:
            return 0
        if st["locked_until"] > now:
            return int(st["locked_until"] - now)
        return 0


def record_failure(ip: str, username: str) -> int:
    """记录一次失败。达到阈值则锁定并返回锁定时长（秒），否则返回 0。"""
    now = time.time()
    k = _key(ip, username)
    with _lock:
        st = _state.get(k)
        if not st or now - st["last_at"] > FAILURE_WINDOW_SECONDS:
            st = {"count": 0, "first_at": now, "last_at": now, "locked_until": 0, "lock_level": 0}
            _state[k] = st

        st["count"] += 1
        st["last_at"] = now

        if st["count"] >= MAX_FAILURES:
            st["lock_level"] += 1
            lock_seconds = min(BASE_LOCK_SECONDS * (2 ** (st["lock_level"] - 1)), MAX_LOCK_SECONDS)
            st["locked_until"] = now + lock_seconds
            # 锁定后重置计数，解锁后重新累计
            st["count"] = 0
            return lock_seconds
        return 0


def record_success(ip: str, username: str) -> None:
    """登录成功：清除该键的失败状态。"""
    with _lock:
        _state.pop(_key(ip, username), None)