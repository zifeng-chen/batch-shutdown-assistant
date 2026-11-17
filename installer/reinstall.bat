@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion
title LAN Agent 一键覆盖安装

set "SERVICE=LanAgent"
set "INSTALL_DIR=C:\Program Files\LanAgent"
set "DEST_EXE=%INSTALL_DIR%\LanAgent.exe"

echo ============================================
echo   LAN Agent 一键覆盖安装（无需先卸载）
echo ============================================
echo.

rem ---------- 1. 管理员权限检查 ----------
net session >nul 2>&1
if not "%ERRORLEVEL%"=="0" (
    echo [错误] 需要管理员权限。
    echo        请右键本文件 -^> 以管理员身份运行。
    echo.
    pause
    exit /b 1
)
echo [1/5] 管理员权限检查         OK

rem ---------- 2. 定位安装包文件 ----------
set "SRC_DIR=%~dp0"
if not exist "%SRC_DIR%LanAgent.exe" (
    echo [错误] 未找到 LanAgent.exe。
    echo        请确保本脚本与 LanAgent.exe 在同一目录。
    echo.
    pause
    exit /b 1
)
echo [2/5] 找到 LanAgent.exe       OK

if not exist "%SRC_DIR%deploy.conf" (
    echo [警告] 未找到 deploy.conf，将以无配置模式安装。
)

rem ---------- 3. 停止现有服务并等待真正停止 ----------
sc query "%SERVICE%" >nul 2>&1
if not "%ERRORLEVEL%"=="0" (
    echo [3/5] 未检测到已有服务     跳过停止
    goto :wait_file
)

echo [3/5] 正在停止现有服务...
sc stop "%SERVICE%" >nul 2>&1

set /a WAIT=0
:wait_stop
sc query "%SERVICE%" | findstr /C:"STOPPED" >nul 2>&1
if not errorlevel 1 goto :stopped
set /a WAIT+=1
if !WAIT! GEQ 60 (
    echo [警告] 等待停止超时^>60秒，尝试强制结束进程...
    taskkill /f /im LanAgent.exe >nul 2>&1
    ping -n 3 127.0.0.1 >nul 2>&1
    goto :wait_file
)
<nul set /p =.
ping -n 2 127.0.0.1 >nul 2>&1
goto :wait_stop

:stopped
echo.
echo         服务已停止（等待 !WAIT! 秒）

rem ---------- 4. 复制新版本（文件被占用时退避重试）----------
:wait_file
set /a WAIT=0
:copy_retry
if not exist "%INSTALL_DIR%" mkdir "%INSTALL_DIR%" >nul 2>&1
copy /Y "%SRC_DIR%LanAgent.exe" "%DEST_EXE%" >nul 2>&1
if not errorlevel 1 goto :copy_ok
set /a WAIT+=1
if !WAIT! GEQ 20 (
    echo [错误] 无法替换文件（被占用）：%DEST_EXE%
    echo        请关闭占用该文件的程序（杀毒软件/残留进程），稍后重试。
    echo.
    pause
    exit /b 1
)
<nul set /p =.
ping -n 2 127.0.0.1 >nul 2>&1
goto :copy_retry

:copy_ok
if exist "%SRC_DIR%deploy.conf" (
    copy /Y "%SRC_DIR%deploy.conf" "%INSTALL_DIR%\deploy.conf" >nul
)
echo.
echo [4/5] 复制新版本文件          OK

rem ---------- 5. 运行安装并校验 ----------
rem 从安装目录运行：exe 会识别到"程序已在安装目录"，跳过自我复制，
rem 只负责创建/启动服务，避免自我覆盖失败。
echo [5/5] 正在安装/启动服务...
echo.
pushd "%INSTALL_DIR%"
"%DEST_EXE%"
set "RC=%ERRORLEVEL%"
popd
echo.

sc query "%SERVICE%" | findstr /C:"RUNNING" >nul 2>&1
if errorlevel 1 (
    echo ============================================
    echo   安装未确认成功，服务当前未处于 RUNNING。
    echo   请查看上方输出中的具体错误信息。
    echo ============================================
    echo.
    pause
    exit /b 1
)

echo ============================================
echo   安装成功！服务正在运行
echo   服务名:   %SERVICE%
echo   安装路径: %INSTALL_DIR%
echo ============================================
echo.
pause
exit /b 0
