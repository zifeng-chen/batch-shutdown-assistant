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
from api.agent_routes import _enroll_secret
from api.audit import log_audit
from config import BASE_DIR
import os

router = APIRouter(prefix="/api/deploy", tags=["deploy"])


def _detect_lan_ip() -> str | None:
    """探测本机局域网 IP，供设备下载地址使用（localhost/127.0.0.1 时替换）。"""
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
    return local_ip


def _resolve_server_url(request: Request) -> str:
    """返回设备应访问的服务器基地址；浏览器用 localhost 访问时替换为局域网 IP。"""
    host = request.headers.get("host", "localhost:8900")
    hostname_part = host.split(":")[0]
    if hostname_part in ("localhost", "127.0.0.1"):
        local_ip = _detect_lan_ip()
        if not local_ip:
            raise HTTPException(status_code=500, detail="无法检测宿主机局域网IP，请用局域网IP访问管理端后重试")
        port = host.split(":")[1] if ":" in host else "8900"
        host = f"{local_ip}:{port}"
    return f"http://{host}"


class DeployPackageRequest(BaseModel):
    device_ids: list[str] | None = None
    group_name: str | None = None


class GenerateTokenRequest(BaseModel):
    hostname: str = ""
    ip: str = ""
    group_name: str = ""


@router.get("/server-url")
async def get_server_url(request: Request, current_user: dict = Depends(get_current_user)):
    """返回设备应访问的服务器基地址。前端推送升级/文件时用它构造下载 URL，
    避免浏览器用 localhost 访问时把 localhost 推给远端设备。"""
    return {"server_url": _resolve_server_url(request)}


@router.post("/generate-token")
async def generate_device_token(
    payload: GenerateTokenRequest,
    db: AsyncSession = Depends(get_db),
    current_user: dict = Depends(get_current_user),
):
    hostname = payload.hostname
    ip = payload.ip
    group_name = payload.group_name
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
    server_url = _resolve_server_url(request)

    agent_exe_path = os.path.join(os.path.dirname(BASE_DIR), "agent", "LanAgent.exe")
    if not os.path.isfile(agent_exe_path):
        raise HTTPException(status_code=500, detail="LanAgent.exe not found on server")

    # 安装包需携带注册密钥，否则新设备无法完成首次注册
    enroll_secret = _enroll_secret()
    deploy_conf = (
        f"# LAN Agent deploy config\n"
        f"server={server_url}\n"
        f"enroll={enroll_secret}\n"
    )

    readme_txt = (
        "========================================\n"
        "  LAN Agent 安装使用指南（联想门店运营系统 · 样机设备管理）\n"
        "========================================\n"
        "\n"
        "【安装步骤】\n"
        "\n"
        "1. 解压本压缩包到目标电脑的任意位置（deploy.conf 必须与 bat 在同一目录）\n"
        "\n"
        "2. 右键点击 reinstall.bat → 以管理员身份运行\n"
        "\n"
        "3. 脚本依次显示：管理员权限 / 安装包文件 / 停止旧服务 / 清理旧文件 / 复制文件 / 安装启动\n"
        "\n"
        "4. 看到「安装成功！服务正在运行」即完成\n"
        "\n"
        "5. 管理端约 10 秒内可见该设备上线\n"
        "\n"
        "注意：请勿直接双击 LanAgent.exe。它需要 config.json 才能启动，\n"
        "没有配置文件时会立刻退出，表现为窗口一闪而过。\n"
        "\n"
        "【已安装旧版本时如何覆盖安装】\n"
        "\n"
        "直接右键 reinstall.bat → 以管理员身份运行，无需手动卸载。\n"
        "\n"
        "脚本会自动完成以下动作，已处理好旧版本残留：\n"
        "  - 停止并删除旧的 LanAgent 服务注册\n"
        "  - 结束残留的 LanAgent.exe 进程\n"
        "  - 删除安装目录下的旧 LanAgent.exe / config.json / deploy.conf\n"
        "  - 复制新版本并重新创建、启动服务\n"
        "  - 保留 logs 与 data 目录，历史日志和数据不会丢失\n"
        "  - 顺带清理更早版本遗留的 C:\\Windows\\LanAgent 与 C:\\LanAgent\n"
        "\n"
        "如果仍提示「无法写入」或「文件被占用」，通常是杀毒软件锁定了文件，\n"
        "重启电脑后立即运行本脚本即可。\n"
        "\n"
        "【安装日志】\n"
        "\n"
        "脚本运行中显示的每一行，都会同步写入安装包所在目录下的：\n"
        "\n"
        "  安装日志.txt\n"
        "\n"
        "即与 reinstall.bat 同一个文件夹，安装完成后可以直接打开查看或回传。\n"
        "\n"
        "即使终端窗口意外关闭，日志也已保留，能看到停在哪一步、失败原因是什么。\n"
        "\n"
        "【卸载步骤】\n"
        "\n"
        "1. 右键点击 uninstall.bat → 以管理员身份运行\n"
        "\n"
        "2. 等待显示卸载完成\n"
        "\n"
        "【密码被改、登录提示「密码不正确」的修复】\n"
        "\n"
        "若机器执行过旧版 Sysprep 后无法登录，用已知密码进入桌面后：\n"
        "\n"
        "1. 右键点击 reset-password.bat → 以管理员身份运行\n"
        "\n"
        "2. 输入 Y 确认\n"
        "\n"
        "3. 显示「密码已清空」后重启，即可空密码直接进桌面\n"
        "\n"
        "该脚本只清空当前登录账户的密码，不影响其他账户。\n"
        "\n"
        "【注意事项】\n"
        "\n"
        "- 必须以管理员身份运行，否则安装会失败\n"
        "- 同一个安装包可以安装到任意多台设备\n"
        "- 安装后服务开机自启，无需手动操作\n"
        "- deploy.conf 文件不要删除，服务运行需要读取该文件\n"
        "- deploy.conf 内含注册密钥，请勿外传；管理端更换密钥后需重新生成安装包\n"
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
        # 一键覆盖安装脚本：停服务→等待真正停止→替换文件→安装校验，
        # 已装过旧版的设备可直接运行它完成升级，无需先卸载。
        reinstall_src = os.path.join(os.path.dirname(BASE_DIR), "installer", "reinstall.bat")
        if os.path.isfile(reinstall_src):
            with open(reinstall_src, "rb") as f:
                data = f.read()
            # 统一为 CRLF，保证 Windows 下双击运行正常
            data = data.replace(b"\r\n", b"\n").replace(b"\n", b"\r\n")
            zf.writestr("reinstall.bat", data)

        # 密码修复脚本：旧版 Agent 会改写账户密码导致登录失败，此脚本用于恢复空密码
        resetpwd_src = os.path.join(os.path.dirname(BASE_DIR), "installer", "reset-password.bat")
        if os.path.isfile(resetpwd_src):
            with open(resetpwd_src, "rb") as f:
                data = f.read()
            data = data.replace(b"\r\n", b"\n").replace(b"\n", b"\r\n")
            zf.writestr("reset-password.bat", data)

    buf.seek(0)

    await log_audit(db, "deploy_package", current_user["username"],
                    detail=f"server={server_url}")

    return StreamingResponse(
        buf,
        media_type="application/zip",
        headers={"Content-Disposition": "attachment; filename=LanAgent-Deploy.zip"},
    )
