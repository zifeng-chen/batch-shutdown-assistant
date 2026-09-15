import io
import uuid
import zipfile

from fastapi import APIRouter, Depends, HTTPException, Request
from fastapi.responses import StreamingResponse
from pydantic import BaseModel
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from database import get_db
from models.models import Device, DeviceStatus
from api.auth import get_current_user
from api.audit import log_audit
from config import BASE_DIR
import os

router = APIRouter(prefix="/api/deploy", tags=["deploy"])


class DeployPackageRequest(BaseModel):
    device_ids: list[str] | None = None
    group_name: str | None = None


@router.post("/generate-token")
async def generate_device_token(
    hostname: str = "",
    ip: str = "",
    group_name: str = "",
    db: AsyncSession = Depends(get_db),
    current_user: dict = Depends(get_current_user),
):
    device = Device(
        id=str(uuid.uuid4()),
        hostname=hostname or None,
        ip=ip or None,
        group_name=group_name or None,
        token=str(uuid.uuid4()),
        status=DeviceStatus.OFFLINE,
    )
    db.add(device)
    await db.commit()
    await db.refresh(device)

    await log_audit(db, "device_create", current_user["username"],
                    target=f"{hostname}({ip})", detail="via deploy package")

    return {
        "device_id": device.id,
        "token": device.token,
        "hostname": device.hostname,
        "ip": device.ip,
    }


@router.post("/package")
async def download_deploy_package(
    payload: DeployPackageRequest,
    request: Request,
    db: AsyncSession = Depends(get_db),
    current_user: dict = Depends(get_current_user),
):
    host = request.headers.get("host", "localhost:8080")
    hostname_part = host.split(":")[0]
    if hostname_part in ("localhost", "127.0.0.1"):
        import socket
        local_ip = None
        try:
            s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            s.connect(("8.8.8.8", 80))
            local_ip = s.getsockname()[0]
            s.close()
        except Exception:
            pass
        if not local_ip:
            try:
                for addr in socket.getaddrinfo(socket.gethostname(), None, socket.AF_INET):
                    ip = addr[4][0]
                    if not ip.startswith("127."):
                        local_ip = ip
                        break
            except Exception:
                pass
        if not local_ip:
            raise HTTPException(status_code=500, detail="无法检测宿主机局域网IP，请用局域网IP访问管理端后重新下载")
        port = host.split(":")[1] if ":" in host else "8080"
        host = f"{local_ip}:{port}"
    server_url = f"http://{host}"

    agent_exe_path = os.path.join(os.path.dirname(BASE_DIR), "agent", "LanAgent.exe")
    if not os.path.isfile(agent_exe_path):
        raise HTTPException(status_code=500, detail="LanAgent.exe not found on server")

    deploy_conf = f"# LAN Agent deploy config\nserver={server_url}\n"

    readme_txt = (
        "========================================\n"
        "  LAN Agent 安装使用指南\n"
        "========================================\n"
        "\n"
        "【安装步骤】\n"
        "\n"
        "1. 解压本压缩包到 U 盘或目标电脑的任意位置\n"
        "\n"
        "2. 右键点击 LanAgent.exe → 以管理员身份运行\n"
        "\n"
        "3. 出现安装界面，显示本机 IP 和服务器地址\n"
        "\n"
        "4. 等待显示「[成功]LAN Agent 已安装并启动」\n"
        "\n"
        "5. 按回车键关闭窗口，安装完成\n"
        "\n"
        "【卸载步骤】\n"
        "\n"
        "1. 右键点击 uninstall.bat → 以管理员身份运行\n"
        "\n"
        "2. 等待显示卸载完成\n"
        "\n"
        "【注意事项】\n"
        "\n"
        "- 必须以管理员身份运行，否则安装会失败\n"
        "- 同一个安装包可以安装到任意多台设备\n"
        "- 安装后服务开机自启，无需手动操作\n"
        "- 覆盖安装会自动停止旧服务、替换文件、重启服务\n"
        "- deploy.conf 文件不要删除，服务运行需要读取该文件\n"
        "\n"
        "========================================\n"
    )

    uninstall_bat = (
        "@echo off\r\n"
        "title LAN Agent Uninstall\r\n"
        "net session >nul 2>&1\r\n"
        "if %ERRORLEVEL% neq 0 (\r\n"
        "    echo [ERROR] Please run as Administrator!\r\n"
        "    pause\r\n"
        "    exit /b 1\r\n"
        ")\r\n"
        '"C:\\Program Files\\LanAgent\\LanAgent.exe" /uninstall\r\n'
        "echo Uninstall complete\r\n"
        "pause\r\n"
    )

    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as zf:
        zf.write(agent_exe_path, "LanAgent.exe")
        zf.writestr("deploy.conf", deploy_conf.encode("ascii"))
        # UTF-8 BOM + UTF-8 content for Windows Notepad compatibility
        readme_bytes = b"\xef\xbb\xbf" + readme_txt.encode("utf-8")
        zf.writestr("安装请阅读此文档.txt", readme_bytes)
        zf.writestr("uninstall.bat", uninstall_bat.encode("ascii"))

    buf.seek(0)

    await log_audit(db, "deploy_package", current_user["username"],
                    detail=f"server={server_url}")

    return StreamingResponse(
        buf,
        media_type="application/zip",
        headers={"Content-Disposition": "attachment; filename=LanAgent-Deploy.zip"},
    )
