@echo off
:: LAN Agent 批量部署脚本
:: 用法: deploy.bat <目标IP列表文件> <管理端URL> <Token>
:: 示例: deploy.bat ips.txt http://192.168.1.100:8080 my-token

if "%~3"=="" (
    echo Usage: deploy.bat ^<ip_list_file^> ^<server_url^> ^<token^>
    exit /b 1
)

set IP_FILE=%~1
set SERVER_URL=%~2
set TOKEN=%~3

for /f "tokens=* delims=" %%i in (%IP_FILE%) do (
    echo Deploying to %%i ...
    psexec \\%%i -accepteula -h cmd /c "copy \\%COMPUTERNAME%\share\LanAgent.exe C:\Program Files\LanAgent\ /Y && C:\Program Files\LanAgent\LanAgent.exe /install /server=%SERVER_URL% /token=%TOKEN%"
    if %ERRORLEVEL% equ 0 (
        echo [OK] %%i
    ) else (
        echo [FAIL] %%i
    )
)

echo Done.
