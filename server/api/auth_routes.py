from fastapi import APIRouter, Depends, HTTPException, Request
from pydantic import BaseModel
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from database import get_db
from models.models import User
from api.auth import verify_password, hash_password, create_access_token, get_current_user
from api.audit import log_audit
from api import ratelimit

router = APIRouter(prefix="/api/auth", tags=["auth"])


def _client_ip(request: Request) -> str:
    """取客户端真实 IP；若经反向代理，优先使用 X-Forwarded-For 首个地址。"""
    fwd = request.headers.get("x-forwarded-for", "")
    if fwd:
        return fwd.split(",")[0].strip()
    return request.client.host if request.client else "unknown"


def validate_password_strength(password: str) -> None:
    """密码强度校验：至少 8 位，且同时包含字母和数字"""
    if len(password) < 8:
        raise HTTPException(status_code=400, detail="密码长度至少 8 位")
    if len(password) > 128:
        raise HTTPException(status_code=400, detail="密码长度不能超过 128 位")
    has_alpha = any(c.isalpha() for c in password)
    has_digit = any(c.isdigit() for c in password)
    if not (has_alpha and has_digit):
        raise HTTPException(status_code=400, detail="密码需同时包含字母和数字")


class LoginRequest(BaseModel):
    username: str
    password: str


class LoginResponse(BaseModel):
    access_token: str
    token_type: str = "bearer"
    username: str


@router.post("/login", response_model=LoginResponse)
async def login(payload: LoginRequest, request: Request, db: AsyncSession = Depends(get_db)):
    ip = _client_ip(request)

    # 已在锁定状态：直接拒绝，不消耗查询
    wait = ratelimit.check_allowed(ip, payload.username)
    if wait > 0:
        await log_audit(db, "login_locked", payload.username, detail=f"locked {wait}s", ip_address=ip)
        raise HTTPException(
            status_code=429,
            detail=f"登录失败次数过多，请 {wait} 秒后重试",
        )

    result = await db.execute(select(User).where(User.username == payload.username))
    user = result.scalar_one_or_none()

    if not user or not verify_password(payload.password, user.password_hash):
        locked_seconds = ratelimit.record_failure(ip, payload.username)
        if locked_seconds:
            await log_audit(db, "login_locked", payload.username,
                            detail=f"locked {locked_seconds}s after repeated failures", ip_address=ip)
            raise HTTPException(
                status_code=429,
                detail=f"登录失败次数过多，请 {locked_seconds} 秒后重试",
            )
        await log_audit(db, "login_failed", payload.username, ip_address=ip)
        raise HTTPException(status_code=401, detail="用户名或密码错误")

    ratelimit.record_success(ip, payload.username)
    await log_audit(db, "login_success", user.username, ip_address=ip)

    token = create_access_token({"sub": user.username, "role": user.role})
    return LoginResponse(access_token=token, username=user.username)


@router.get("/me")
async def get_me(current_user: dict = Depends(get_current_user)):
    return current_user


class ChangePasswordRequest(BaseModel):
    old_password: str
    new_password: str


@router.post("/change-password")
async def change_password(
    payload: ChangePasswordRequest,
    current_user: dict = Depends(get_current_user),
    db: AsyncSession = Depends(get_db),
):
    result = await db.execute(select(User).where(User.username == current_user["username"]))
    user = result.scalar_one_or_none()
    if not user or not verify_password(payload.old_password, user.password_hash):
        raise HTTPException(status_code=400, detail="原密码错误")

    validate_password_strength(payload.new_password)
    if payload.new_password == payload.old_password:
        raise HTTPException(status_code=400, detail="新密码不能与原密码相同")

    user.password_hash = hash_password(payload.new_password)
    await db.commit()
    return {"status": "ok"}
