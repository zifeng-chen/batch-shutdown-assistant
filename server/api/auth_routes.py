from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from database import get_db
from models.models import User
from api.auth import verify_password, hash_password, create_access_token, get_current_user

router = APIRouter(prefix="/api/auth", tags=["auth"])


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
async def login(payload: LoginRequest, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(User).where(User.username == payload.username))
    user = result.scalar_one_or_none()

    if not user or not verify_password(payload.password, user.password_hash):
        raise HTTPException(status_code=401, detail="用户名或密码错误")

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
