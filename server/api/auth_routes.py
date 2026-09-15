from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from database import get_db
from models.models import User
from api.auth import verify_password, hash_password, create_access_token, get_current_user

router = APIRouter(prefix="/api/auth", tags=["auth"])


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


@router.post("/change-password")
async def change_password(
    old_password: str,
    new_password: str,
    current_user: dict = Depends(get_current_user),
    db: AsyncSession = Depends(get_db),
):
    result = await db.execute(select(User).where(User.username == current_user["username"]))
    user = result.scalar_one_or_none()
    if not user or not verify_password(old_password, user.password_hash):
        raise HTTPException(status_code=400, detail="原密码错误")

    user.password_hash = hash_password(new_password)
    await db.commit()
    return {"status": "ok"}
