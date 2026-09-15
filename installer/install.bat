@echo off
chcp 65001 >nul
title LAN Agent 安装程序

echo ============================================
echo   LAN Agent 局域网管理系统 - 客户端安装
echo ============================================
echo.

:: 检查管理员权限
net session >nul 2>&1
if %ERRORLEVEL% neq 0 (
    echo [错误] 请以管理员身份运行此脚本！
    echo 右键点击此文件 → 以管理员身份运行
    pause
    exit /b 1
)

:: 获取当前目录
set INSTALL_DIR=%~dp0

:: 检查 LanAgent.exe 是否存在
if not exist "%INSTALL_DIR%LanAgent.exe" (
    echo [错误] 未找到 LanAgent.exe，请确保本脚本与 LanAgent.exe 在同一目录
    pause
    exit /b 1
)

echo.
echo 请输入管理端服务器地址（例如 http://192.168.1.100:8080）
set /p SERVER_URL=服务器地址: 

echo.
echo 请输入设备Token（在管理端添加设备时获取）
set /p TOKEN=设备Token: 

if "%SERVER_URL%"=="" (
    echo [错误] 服务器地址不能为空
    pause
    exit /b 1
)
if "%TOKEN%"=="" (
    echo [错误] Token不能为空
    pause
    exit /b 1
)

echo.
echo -------------------------------------------
echo 即将安装 LAN Agent 服务：
echo   安装路径: C:\Program Files\LanAgent\
echo   服务器:   %SERVER_URL%
echo   Token:    %TOKEN%
echo -------------------------------------------
echo.
set /p CONFIRM=确认安装？(Y/N): 
if /i not "%CONFIRM%"=="Y" (
    echo 已取消安装
    pause
    exit /b 0
)

:: 创建安装目录
mkdir "C:\Program Files\LanAgent" >nul 2>&1
mkdir "C:\LanAgent" >nul 2>&1

:: 复制文件
copy /Y "%INSTALL_DIR%LanAgent.exe" "C:\Program Files\LanAgent\LanAgent.exe" >nul
if %ERRORLEVEL% neq 0 (
    echo [错误] 复制文件失败
    pause
    exit /b 1
)

:: 安装并启动服务
echo.
echo 正在安装服务...
"C:\Program Files\LanAgent\LanAgent.exe" /install /server=%SERVER_URL% /token=%TOKEN%
if %ERRORLEVEL% neq 0 (
    echo [错误] 服务安装失败
    pause
    exit /b 1
)

echo.
echo ============================================
echo   安装成功！LAN Agent 服务已启动
echo ============================================
echo.
echo 卸载命令: "C:\Program Files\LanAgent\LanAgent.exe" /uninstall
echo.
pause
