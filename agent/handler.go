package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Command struct {
	ID     string          `json:"id"`
	Action string          `json:"action"` // sysprep, shutdown, reboot, status
	Params json.RawMessage `json:"params,omitempty"`
}

type CommandResult struct {
	CommandID string `json:"command_id"`
	DeviceID  string `json:"device_id"`
	Status    string `json:"status"` // success, failed
	Output    string `json:"output"`
	Timestamp int64  `json:"timestamp"`
}

type SysprepParams struct {
	SkipOOBE      bool   `json:"skip_oobe"`
	LocalAdmin    string `json:"local_admin"`
	LocalPassword string `json:"local_password"`
}

func handleCommand(cfg *Config, cmd Command) CommandResult {
	result := CommandResult{
		CommandID: cmd.ID,
		DeviceID:  cfg.DeviceID,
		Timestamp: time.Now().Unix(),
	}

	log.Printf("[handler] executing command %s: %s", cmd.ID, cmd.Action)

	switch cmd.Action {
	case "sysprep":
		var params SysprepParams
		if cmd.Params != nil {
			_ = json.Unmarshal(cmd.Params, &params)
		}

		unattendPath, err := generateUnattend(cfg, params)
		if err != nil {
			result.Status = "failed"
			result.Output = fmt.Sprintf("generate unattend failed: %v", err)
			return result
		}

		if err := writeSetupCompleteScript(cfg); err != nil {
			result.Status = "failed"
			result.Output = fmt.Sprintf("write setup complete failed: %v", err)
			return result
		}

		if err := runSysprep(false, unattendPath); err != nil {
			result.Status = "failed"
			result.Output = err.Error()
			return result
		}

		result.Status = "success"
		result.Output = "sysprep initiated, machine will shutdown and enter OOBE"

		go func() {
			time.Sleep(60 * time.Second)
			log.Printf("[sysprep] fallback: forcing shutdown after 60s")
			exec.Command("cmd", "/c", "shutdown /s /t 0 /f").Run()
		}()

	case "sysprep_reboot":
		var params SysprepParams
		if cmd.Params != nil {
			_ = json.Unmarshal(cmd.Params, &params)
		}

		unattendPath, err := generateUnattend(cfg, params)
		if err != nil {
			result.Status = "failed"
			result.Output = fmt.Sprintf("generate unattend failed: %v", err)
			return result
		}

		if err := writeSetupCompleteScript(cfg); err != nil {
			result.Status = "failed"
			result.Output = fmt.Sprintf("write setup complete failed: %v", err)
			return result
		}

		if err := runSysprep(true, unattendPath); err != nil {
			result.Status = "failed"
			result.Output = err.Error()
			return result
		}

		// /reboot 已由 sysprep 自身发起重启，不再额外调用 executeReboot
		result.Status = "success"
		result.Output = "sysprep initiated, machine will reboot and enter OOBE"

		go func() {
			time.Sleep(60 * time.Second)
			log.Printf("[sysprep_reboot] fallback: forcing reboot after 60s")
			exec.Command("cmd", "/c", "shutdown /r /t 0 /f").Run()
		}()

	case "reset_pwd":
		if err := resetPassword(); err != nil {
			result.Status = "failed"
			result.Output = err.Error()
			return result
		}
		result.Status = "success"
		result.Output = fmt.Sprintf("password cleared for account: %s", activeConsoleUser())

	case "shutdown":
		if err := executeShutdown(); err != nil {
			result.Status = "failed"
			result.Output = err.Error()
			return result
		}
		result.Status = "success"
		result.Output = "shutdown initiated"

	case "reboot":
		if err := executeReboot(); err != nil {
			result.Status = "failed"
			result.Output = err.Error()
			return result
		}
		result.Status = "success"
		result.Output = "reboot initiated"

	case "status":
		rearm := getRearmCount()
		result.Status = "success"
		result.Output = fmt.Sprintf(`{"hostname":"%s","ip":"%s","rearm_count":%d,"agent_version":"%s"}`,
			getHostname(), getLocalIP(), rearm, getAgentVersion())

	case "upgrade":
		var params struct {
			URL     string `json:"url"`
			Version string `json:"version"`
		}
		if cmd.Params != nil {
			_ = json.Unmarshal(cmd.Params, &params)
		}
		if params.URL == "" {
			result.Status = "failed"
			result.Output = "missing upgrade url"
			return result
		}
		if isUpgradeRunning() {
			result.Status = "failed"
			result.Output = "upgrade already in progress"
			return result
		}

		if err := startUpgrade(cfg, params.URL, params.Version); err != nil {
			result.Status = "failed"
			result.Output = fmt.Sprintf("upgrade failed: %v", err)
			return result
		}

		markUpgradeRunning()
		result.Status = "success"
		result.Output = fmt.Sprintf("upgrade to %s staged, service will restart (结果见 %s)",
			params.Version, upgradeLogPath())

		// 升级脚本已独立启动。等待其完成"禁用恢复策略 + 停止服务"这一步再退出：
		// bat 启动后需要几秒才会执行到 sc failure / sc stop，过早退出会让 SCM
		// 在替换未完成时拉起旧版。30 秒足够覆盖下载校验与脚本初始化。
		go func() {
			time.Sleep(30 * time.Second)
			os.Exit(0)
		}()

	case "file_push":
		var params struct {
			URL      string `json:"url"`
			Filename string `json:"filename"`
			DestDir  string `json:"dest_dir"`
		}
		if cmd.Params != nil {
			_ = json.Unmarshal(cmd.Params, &params)
		}
		if params.URL == "" || params.Filename == "" {
			result.Status = "failed"
			result.Output = "missing url or filename"
			return result
		}

		// 只允许从本管理服务器下载
		if err := validateServerURL(cfg, params.URL); err != nil {
			result.Status = "failed"
			result.Output = err.Error()
			return result
		}

		// 目标目录限定在 filesDir 之下，防止写入任意路径
		destDir := params.DestDir
		if destDir == "" {
			destDir = filesDir
		}
		baseClean := filepath.Clean(filesDir)
		dirClean := filepath.Clean(destDir)
		if dirClean != baseClean && !strings.HasPrefix(dirClean, baseClean+string(filepath.Separator)) {
			result.Status = "failed"
			result.Output = fmt.Sprintf("目标目录越界，仅允许 %s 及子目录", filesDir)
			return result
		}

		// 文件名不得穿越或为绝对路径
		destPath, err := safeJoinUnder(destDir, params.Filename)
		if err != nil {
			result.Status = "failed"
			result.Output = err.Error()
			return result
		}

		_ = os.MkdirAll(destDir, 0755)

		resp, err := http.Get(params.URL)
		if err != nil {
			result.Status = "failed"
			result.Output = fmt.Sprintf("download failed: %v", err)
			return result
		}
		defer resp.Body.Close()

		f, err := os.Create(destPath)
		if err != nil {
			result.Status = "failed"
			result.Output = fmt.Sprintf("create file failed: %v", err)
			return result
		}
		written, err := io.Copy(f, resp.Body)
		f.Close()
		if err != nil {
			result.Status = "failed"
			result.Output = fmt.Sprintf("write file failed: %v", err)
			return result
		}

		result.Status = "success"
		result.Output = fmt.Sprintf("file saved to %s (%d bytes)", destPath, written)

	case "hostmsg":
		var params struct {
			Title   string `json:"title"`
			Message string `json:"message"`
			Timeout int    `json:"timeout"`
		}
		if cmd.Params != nil {
			_ = json.Unmarshal(cmd.Params, &params)
		}

		timeout := params.Timeout
		if timeout <= 0 {
			timeout = 15 // 默认 15 秒自动关闭
		}

		title := params.Title
		message := params.Message
		if message == "" {
			// 未指定内容时，弹出本机自身信息（优先用缓存，无缓存则即时采集）
			info := sysInfoCache.info
			if info == nil {
				info = getSysInfo(true)
			}
			title, message = buildHostMessage(info, getLocalIP(), getHostname(), getAgentVersion())
		}
		if title == "" {
			title = "设备信息提示"
		}

		if err := showHostMessage(title, message, uint32(timeout)); err != nil {
			result.Status = "failed"
			result.Output = fmt.Sprintf("show message failed: %v", err)
			return result
		}
		result.Status = "success"
		result.Output = fmt.Sprintf("message displayed on session desktop (auto close in %ds)", timeout)

	case "sysinfo":
		info := getSysInfo(true)
		payload, err := json.Marshal(info)
		if err != nil {
			result.Status = "failed"
			result.Output = fmt.Sprintf("marshal sysinfo failed: %v", err)
			return result
		}
		result.Status = "success"
		result.Output = string(payload)

	case "uninstall":
		result.Status = "success"
		result.Output = "uninstall initiated"
		go func() {
			time.Sleep(2 * time.Second)

			// 生成独立卸载bat到临时目录，当前进程退出后由bat完成清理
			tmpDir := os.TempDir()
			batPath := filepath.Join(tmpDir, "lanagent_uninstall.bat")
			batContent := fmt.Sprintf(
				"@echo off\r\n"+
					"ping 127.0.0.1 -n 4 >nul\r\n"+
					"net stop LanAgent >nul 2>&1\r\n"+
					"sc delete LanAgent >nul 2>&1\r\n"+
					"taskkill /f /im LanAgent.exe >nul 2>&1\r\n"+
					"ping 127.0.0.1 -n 3 >nul\r\n"+
					"rmdir /s /q \"%s\" >nul 2>&1\r\n"+
					"if exist \"%s\" (\r\n"+
					"  ping 127.0.0.1 -n 3 >nul\r\n"+
					"  taskkill /f /im LanAgent.exe >nul 2>&1\r\n"+
					"  rmdir /s /q \"%s\" >nul 2>&1\r\n"+
					")\r\n"+
					"del \"%%~f0\" >nul 2>&1\r\n",
				installDir, installDir, installDir,
			)
			if err := os.WriteFile(batPath, []byte(batContent), 0644); err != nil {
				os.Exit(0)
				return
			}

			exec.Command("cmd", "/c", "start", "", "/b", batPath).Start()
			os.Exit(0)
		}()

	default:
		result.Status = "failed"
		result.Output = fmt.Sprintf("unknown action: %s", cmd.Action)
	}

	return result
}

// generateUnattend 生成 OOBE 阶段应答文件。
//
// 刻意不包含 UserAccounts / AutoLogon：
// 工厂模式（Ctrl+Shift+F3 审核模式）下账户已存在且为空密码，原样保留即可自动进桌面。
// 一旦写入 LocalAccounts 或带 Password 的 AutoLogon，OOBE 会改写账户凭据，
// 导致重启后提示「密码不正确」。
func generateUnattend(cfg *Config, params SysprepParams) (string, error) {
	xml := `<?xml version="1.0" encoding="utf-8"?>
<unattend xmlns="urn:schemas-microsoft-com:unattend">
  <settings pass="oobeSystem">
    <component name="Microsoft-Windows-Shell-Setup"
               processorArchitecture="amd64"
               publicKeyToken="31bf3856ad364e35"
               language="neutral"
               versionScope="nonSxS"
               xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">
      <OOBE>
        <HideEULAPage>true</HideEULAPage>
        <HideOEMRegistrationScreen>true</HideOEMRegistrationScreen>
        <HideOnlineAccountScreens>true</HideOnlineAccountScreens>
        <HideWirelessSetupInOOBE>true</HideWirelessSetupInOOBE>
        <ProtectYourPC>3</ProtectYourPC>
      </OOBE>
    </component>
  </settings>
</unattend>`

	// 放在 sysprep 自己的目录下：C:\Program Files 在 OOBE 阶段不可靠
	const path = `C:\Windows\System32\Sysprep\unattend.xml`
	if err := os.WriteFile(path, []byte(xml), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func writeSetupCompleteScript(cfg *Config) error {
	scriptDir := `C:\Windows\Setup\Scripts`
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		return err
	}

	// 只恢复服务，不触碰 Winlogon 自动登录设置：
	// 账户保持 OOBE 前的空密码状态即可自动进桌面，写 AutoAdminLogon=0 会反而把人拦在登录界面。
	script := "@echo off\r\n" +
		"net start LanAgent\r\n"

	recoverDir := dataDir
	_ = os.MkdirAll(recoverDir, 0755)

	recoverFlag := filepath.Join(recoverDir, "need_recover")
	_ = os.WriteFile(recoverFlag, []byte("1"), 0644)

	return os.WriteFile(filepath.Join(scriptDir, "SetupComplete.cmd"), []byte(script), 0755)
}
