import os
import uuid

from fastapi import APIRouter, Depends, UploadFile, File, HTTPException
from fastapi.responses import FileResponse

from api.auth import get_current_user
from config import BASE_DIR

router = APIRouter(prefix="/api/upgrade", tags=["upgrade"])

UPLOAD_DIR = os.path.join(BASE_DIR, "uploads")


@router.post("/upload")
async def upload_agent_exe(
    file: UploadFile = File(...),
    _: dict = Depends(get_current_user),
):
    if not file.filename.lower().endswith(".exe"):
        raise HTTPException(status_code=400, detail="仅支持 .exe 文件")

    os.makedirs(UPLOAD_DIR, exist_ok=True)
    filename = f"agent-{uuid.uuid4().hex[:8]}.exe"
    save_path = os.path.join(UPLOAD_DIR, filename)

    content = await file.read()
    with open(save_path, "wb") as f:
        f.write(content)

    return {"filename": filename, "size": len(content)}


@router.get("/file/{filename}")
async def download_agent_exe(filename: str):
    path = os.path.join(UPLOAD_DIR, filename)
    if not os.path.isfile(path):
        raise HTTPException(status_code=404, detail="File not found")
    return FileResponse(path, media_type="application/octet-stream")


@router.get("/latest")
async def get_latest_agent(_: dict = Depends(get_current_user)):
    """返回服务器上最新的 Agent exe（即 agent/ 目录下编译好的版本）"""
    agent_path = os.path.join(os.path.dirname(BASE_DIR), "agent", "LanAgent.exe")
    if not os.path.isfile(agent_path):
        raise HTTPException(status_code=404, detail="Agent exe not found on server")
    size = os.path.getsize(agent_path)
    version_path = os.path.join(os.path.dirname(BASE_DIR), "agent", "version.txt")
    version = "1.1.0"
    if os.path.isfile(version_path):
        with open(version_path) as f:
            v = f.read().strip()
            if v:
                version = v
    return {"version": version, "size": size}


@router.get("/latest-download")
async def download_latest_agent():
    """直接下载服务器上编译好的 Agent exe"""
    agent_path = os.path.join(os.path.dirname(BASE_DIR), "agent", "LanAgent.exe")
    if not os.path.isfile(agent_path):
        raise HTTPException(status_code=404, detail="Agent exe not found on server")
    return FileResponse(agent_path, media_type="application/octet-stream", filename="LanAgent.exe")


@router.post("/file-upload")
async def upload_file(
    file: UploadFile = File(...),
    _: dict = Depends(get_current_user),
):
    os.makedirs(UPLOAD_DIR, exist_ok=True)
    safe_name = file.filename.replace("/", "_").replace("\\", "_")
    save_name = f"file-{uuid.uuid4().hex[:8]}-{safe_name}"
    save_path = os.path.join(UPLOAD_DIR, save_name)

    content = await file.read()
    with open(save_path, "wb") as f:
        f.write(content)

    return {"filename": save_name, "original_name": safe_name, "size": len(content)}
