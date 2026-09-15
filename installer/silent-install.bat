@echo off
chcp 65001 >nul
title LAN Agent 静默安装

:: 检查管理员权限
net session >nul 2>&1
if %ERRORLEVEL% neq 0 (
    echo [错误] 请以管理员身份运行！
    pause
    exit /b 1
)

set INSTALL_DIR=%~dp0

if not exist "%INSTALL_DIR%LanAgent.exe" (
    echo [错误] 未找到 LanAgent.exe
    pause
    exit /b 1
)

if not exist "%INSTALL_DIR%deploy.conf" (
    echo [错误] 未找到 deploy.conf，请先用管理端生成配置文件
    pause
    exit /b 1
)

echo 正在静默安装 LAN Agent...
"%INSTALL_DIR%LanAgent.exe" /silent
if %ERRORLEVEL% equ 0 (
    echo.
    echo ============================================
    echo   [成功] LAN Agent 已安装并启动
    echo ============================================
) else (
    echo.
    echo [失败] 安装出错，请检查 deploy.conf 配置
)
echo.
pause
