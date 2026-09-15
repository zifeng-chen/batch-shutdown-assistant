@echo off
chcp 65001 >nul
title LAN Agent 卸载程序

echo ============================================
echo   LAN Agent 局域网管理系统 - 卸载
echo ============================================
echo.

net session >nul 2>&1
if %ERRORLEVEL% neq 0 (
    echo [错误] 请以管理员身份运行此脚本！
    pause
    exit /b 1
)

set /p CONFIRM=确认卸载 LAN Agent？(Y/N): 
if /i not "%CONFIRM%"=="Y" (
    echo 已取消
    pause
    exit /b 0
)

"C:\Program Files\LanAgent\LanAgent.exe" /uninstall
if %ERRORLEVEL% neq 0 (
    echo [警告] 服务卸载可能未完全成功，请手动检查
)

echo.
echo 卸载完成
pause
