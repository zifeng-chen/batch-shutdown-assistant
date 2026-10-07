@echo off
:: LAN Agent 批量部署脚本
:: 用法: deploy.bat <目标IP列表文件> <管理端URL> <Token> [注册密钥]
:: 示例: deploy.bat ips.txt http://192.168.1.100:8080 my-token enroll-xxxxx
::
:: 说明: 注册密钥用于设备首次接入校验。管理端未配置 LANAGENT_ENROLL_SECRET 时，
::       密钥为 enroll-<LANAGENT_SECRET>，可向管理员索取。

if "%~3"=="" (
    echo Usage: deploy.bat ^<ip_list_file^> ^<server_url^> ^<token^> [enroll_secret]
    exit /b 1
)

set IP_FILE=%~1
set SERVER_URL=%~2
set TOKEN=%~3
set ENROLL=%~4

set ENROLL_ARG=
if not "%ENROLL%"=="" set ENROLL_ARG=/enroll=%ENROLL%

for /f "tokens=* delims=" %%i in (%IP_FILE%) do (
    echo Deploying to %%i ...
    psexec \\%%i -accepteula -h cmd /c "copy \\%COMPUTERNAME%\share\LanAgent.exe C:\Program Files\LanAgent\ /Y && C:\Program Files\LanAgent\LanAgent.exe /install /server=%SERVER_URL% /token=%TOKEN% %ENROLL_ARG%"
    if %ERRORLEVEL% equ 0 (
        echo [OK] %%i
    ) else (
        echo [FAIL] %%i
    )
)

echo Done.
