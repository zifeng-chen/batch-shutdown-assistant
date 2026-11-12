@echo off
chcp 65001 >nul
title LAN Agent - 清空当前账户密码

echo ============================================
echo   清空当前管理员账户密码
echo ============================================
echo.
echo 说明：把当前登录的管理员账户密码清空，
echo       恢复到工厂模式（Ctrl+Shift+F3）下的状态，
echo       重启后可直接点击账户进入桌面。
echo.

net session >nul 2>&1
if %ERRORLEVEL% neq 0 (
    echo [错误] 请以管理员身份运行此脚本！
    echo        右键点击此文件 -^> 以管理员身份运行
    echo.
    pause
    exit /b 1
)

set "EXE=%~dp0LanAgent.exe"
if not exist "%EXE%" (
    echo [错误] 未找到 LanAgent.exe，请确保与本脚本在同一目录。
    echo.
    pause
    exit /b 1
)

echo 即将清空当前账户密码，请确认：
set /p CONFIRM=继续？(Y/N):
if /i not "%CONFIRM%"=="Y" (
    echo 已取消
    pause
    exit /b 0
)

echo.
"%EXE%" /resetpwd
echo.

pause
