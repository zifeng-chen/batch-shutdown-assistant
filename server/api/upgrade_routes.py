import os
import re
import uuid

from fastapi import APIRouter, Depends, UploadFile, File, HTTPException
from fastapi.responses import FileResponse, JSONResponse

from api.auth import get_current_user
from config import BASE_DIR

router = APIRouter(prefix="/api/upgrade", tags=["upgrade"])

UPLOAD_DIR = os.path.join(BASE_DIR, "uploads")


def _extract_version_from_exe(exe_path: str) -> str:
    """从编译好的 exe 中提取 buildVersion 字符串"""
    try:
        with open(exe_path, "rb") as f:
            data = f.read()
        # Go ldflags -X 注入的变量在二进制中以 "build\t-ldflags=\"-X main.buildVersion=X.Y.Z\"" 形式存在
        m = re.search(rb'buildVersion=(\d+\.\d+\.\d+)', data)
        if m:
            return m.group(1).decode("ascii")
    except Exception:
        pass
    return ""


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
    """返回服务器上最新的 Agent exe 版本号（从 exe 二进制中提取）"""
    agent_path = os.path.join(os.path.dirname(BASE_DIR), "agent", "LanAgent.exe")
    if not os.path.isfile(agent_path):
        raise HTTPException(status_code=404, detail="Agent exe not found on server")
    size = os.path.getsize(agent_path)

    version = os.environ.get("LANAGENT_VERSION", "")
    if not version:
        version = _extract_version_from_exe(agent_path)
    if not version:
        version = "unknown"

    return JSONResponse({"version": version, "size": size}, headers={"Cache-Control": "no-store"})


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
