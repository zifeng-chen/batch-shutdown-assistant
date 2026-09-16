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

		if err := executeSysprep(unattendPath); err != nil {
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

		if err := executeSysprepReboot(unattendPath); err != nil {
			result.Status = "failed"
			result.Output = err.Error()
			return result
		}

		// sysprep completed, now reboot
		if err := executeReboot(); err != nil {
			result.Status = "failed"
			result.Output = fmt.Sprintf("sysprep ok but reboot failed: %v", err)
			return result
		}

		result.Status = "success"
		result.Output = "sysprep initiated, machine will reboot and enter OOBE"

		go func() {
			time.Sleep(60 * time.Second)
			log.Printf("[sysprep_reboot] fallback: forcing reboot after 60s")
			exec.Command("cmd", "/c", "shutdown /r /t 0 /f").Run()
		}()

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

		result.Status = "success"
		result.Output = fmt.Sprintf("upgrade to %s initiated", params.Version)

		go func() {
			time.Sleep(2 * time.Second)

			resp, err := http.Get(params.URL)
			if err != nil {
				log.Printf("[upgrade] download failed: %v", err)
				return
			}
			defer resp.Body.Close()

			tmpPath := filepath.Join(os.TempDir(), "LanAgent_new.exe")
			f, err := os.Create(tmpPath)
			if err != nil {
				log.Printf("[upgrade] create temp file failed: %v", err)
				return
			}
			_, err = io.Copy(f, resp.Body)
			f.Close()
			if err != nil {
				log.Printf("[upgrade] write temp file failed: %v", err)
				return
			}

			destExe := installDir + `\LanAgent.exe`
			oldExe := installDir + `\LanAgent.exe.old`

			// rename 旧 exe → .old（Windows 允许 rename 运行中的 exe），再 copy 新 exe 到原路径，最后重启服务
			script := fmt.Sprintf(
				`timeout /t 2 /nobreak >nul & move /Y "%s" "%s" >nul 2>&1 & copy /Y "%s" "%s" >nul 2>&1 & net stop LanAgent >nul 2>&1 & timeout /t 1 /nobreak >nul & net start LanAgent >nul 2>&1 & del "%s" >nul 2>&1`,
				destExe, oldExe, tmpPath, destExe, oldExe,
			)

			c := exec.Command("cmd", "/c", script)
			c.Start()

			log.Printf("[upgrade] handoff to external process, exiting")
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

		destDir := params.DestDir
		if destDir == "" {
			destDir = filesDir
		}
		_ = os.MkdirAll(destDir, 0755)
		destPath := filepath.Join(destDir, params.Filename)

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

	case "uninstall":
		result.Status = "success"
		result.Output = "uninstall initiated"
		go func() {
			time.Sleep(2 * time.Second)
			exec.Command("cmd", "/c", `"`+installDir+`\LanAgent.exe"`, "/uninstall").Run()
			os.Exit(0)
		}()

	default:
		result.Status = "failed"
		result.Output = fmt.Sprintf("unknown action: %s", cmd.Action)
	}

	return result
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

func generateUnattend(cfg *Config, params SysprepParams) (string, error) {
	adminName := params.LocalAdmin
	if adminName == "" {
		adminName = "LabAdmin"
	}
	adminPass := params.LocalPassword
	if adminPass == "" {
		adminPass = "ChangeMe@2024"
	}

	safeName := xmlEscape(adminName)
	safePass := xmlEscape(adminPass)

	xml := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
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
      <UserAccounts>
        <LocalAccounts>
          <LocalAccount wcm:action="add">
            <Name>%s</Name>
            <Group>Administrators</Group>
            <Password>
              <Value>%s</Value>
              <PlainText>true</PlainText>
            </Password>
          </LocalAccount>
        </LocalAccounts>
      </UserAccounts>
      <AutoLogon>
        <Username>%s</Username>
        <Enabled>true</Enabled>
        <LogonCount>1</LogonCount>
      </AutoLogon>
    </component>
  </settings>
</unattend>`, safeName, safePass, safeName)

	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	path := filepath.Join(dir, "unattend.xml")
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

	script := "@echo off\r\n" +
		"net start LanAgent\r\n" +
		`reg add "HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" /v AutoAdminLogon /t REG_SZ /d 0 /f` + "\r\n"

	logDir := logsDir
	_ = os.MkdirAll(logDir, 0755)

	recoverFlag := filepath.Join(logDir, "need_recover")
	_ = os.WriteFile(recoverFlag, []byte("1"), 0644)

	return os.WriteFile(filepath.Join(scriptDir, "SetupComplete.cmd"), []byte(script), 0755)
}
