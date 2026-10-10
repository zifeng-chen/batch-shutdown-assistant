package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// 升级相关路径
const (
	upgradeLogName  = "upgrade.log"
	newExeName      = "LanAgent_new.exe"
	backupSuffix    = ".old"
	upgradeBatName  = "lanagent_upgrade.bat"
	minExeSizeBytes = 1024 * 1024 // 新包至少 1MB，防止下到错误页面
)

// upgradeLogPath 返回升级日志文件路径
func upgradeLogPath() string {
	return filepath.Join(installDir, "logs", upgradeLogName)
}

// appendUpgradeLog 追加一行升级日志；写日志失败不应影响主流程
func appendUpgradeLog(format string, args ...interface{}) {
	path := upgradeLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	line := fmt.Sprintf("[%s] %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		fmt.Sprintf(format, args...))
	_, _ = f.WriteString(line)
}

// downloadUpgrade 下载新版本 exe 到临时目录，并做基本校验。
// 返回可执行文件路径与错误。
func downloadUpgrade(cfg *Config, url string) (string, error) {
	if err := validateServerURL(cfg, url); err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}

	tmpPath := filepath.Join(os.TempDir(), newExeName)
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}

	written, err := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("写入失败: %w", err)
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("关闭文件失败: %w", closeErr)
	}

	// 校验：避免把错误页面或截断的包当成程序写入
	if written < minExeSizeBytes {
		os.Remove(tmpPath)
		return "", fmt.Errorf("升级包异常：仅 %d 字节（应大于 %d）", written, minExeSizeBytes)
	}
	if !isPEExecutable(tmpPath) {
		os.Remove(tmpPath)
		return "", fmt.Errorf("升级包不是有效的 Windows 可执行文件")
	}

	appendUpgradeLog("download ok: %s (%d bytes)", tmpPath, written)
	return tmpPath, nil
}

// isPEExecutable 校验文件头是否为 PE（MZ 开头）
func isPEExecutable(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	head := make([]byte, 2)
	if _, err := io.ReadFull(f, head); err != nil {
		return false
	}
	return head[0] == 'M' && head[1] == 'Z'
}

// buildUpgradeBat 生成独立的升级批处理脚本。
//
// 设计要点（对应旧实现的缺陷）：
//   - 不依赖 schtasks / timeout：这些在服务会话（Session 0）下不可靠
//   - 全程写日志，不再静默失败
//   - 替换前备份旧版；新服务启动失败时自动回滚
//   - 结束时自删除，避免残留
func buildUpgradeBat(destExe, backupExe, newExe, batPath, logPath string) string {
	return fmt.Sprintf(`@echo off
setlocal
set "LOG=%s"
echo [%%date%% %%time%%] ===== upgrade started ===== >> "%%LOG%%"

rem ---- 1. 停止服务，释放 exe 占用 ----
sc stop LanAgent >> "%%LOG%%" 2>&1
ping -n 4 127.0.0.1 > nul 2>&1

rem ---- 2. 备份旧版本 ----
if exist "%s" del /f /q "%s" > nul 2>&1
move /Y "%s" "%s" >> "%%LOG%%" 2>&1
if not exist "%s" (
  echo [%%date%% %%time%%] ERROR: backup old exe failed >> "%%LOG%%"
  goto :rollback_failed
)

rem ---- 3. 写入新版本 ----
copy /Y "%s" "%s" >> "%%LOG%%" 2>&1
if not exist "%s" (
  echo [%%date%% %%time%%] ERROR: copy new exe failed >> "%%LOG%%"
  goto :rollback
)

rem ---- 4. 启动服务并校验 ----
sc start LanAgent >> "%%LOG%%" 2>&1
ping -n 6 127.0.0.1 > nul 2>&1
sc query LanAgent | findstr /C:"RUNNING" > nul 2>&1
if errorlevel 1 (
  echo [%%date%% %%time%%] ERROR: service not RUNNING, rolling back >> "%%LOG%%"
  goto :rollback
)

echo [%%date%% %%time%%] upgrade SUCCESS >> "%%LOG%%"
del /f /q "%s" > nul 2>&1
goto :done

:rollback
sc stop LanAgent >> "%%LOG%%" 2>&1
ping -n 4 127.0.0.1 > nul 2>&1
copy /Y "%s" "%s" >> "%%LOG%%" 2>&1
sc start LanAgent >> "%%LOG%%" 2>&1
echo [%%date%% %%time%%] rolled back to previous version >> "%%LOG%%"
goto :done

:rollback_failed
echo [%%date%% %%time%%] no backup available, manual intervention required >> "%%LOG%%"

:done
echo [%%date%% %%time%%] ===== upgrade finished ===== >> "%%LOG%%"
del /f /q "%s" > nul 2>&1
exit /b 0
`,
		// 1) 日志路径
		logPath,
		// 2) 清理旧备份 + 备份当前 exe
		backupExe, backupExe,
		destExe, backupExe, backupExe,
		// 3) 写入新 exe
		newExe, destExe, destExe,
		// 4) 成功时删除临时新包
		newExe,
		// 5) 回滚：备份拷回目标
		backupExe, destExe,
		// 6) 脚本自删除
		batPath,
	)
}

// startUpgrade 执行升级：下载 → 生成脚本 → 脱离父进程启动脚本 → 返回。
//
// 关键：用 DETACHED_PROCESS 让脚本脱离当前进程树运行，
// 这样 Agent 稍后退出（或被 SCM 终止）不会连带杀死升级脚本。
func startUpgrade(cfg *Config, url, version string) error {
	appendUpgradeLog("upgrade requested: version=%s url=%s", version, url)

	newExe, err := downloadUpgrade(cfg, url)
	if err != nil {
		appendUpgradeLog("upgrade aborted: %v", err)
		return err
	}

	selfExe, err := os.Executable()
	if err != nil {
		appendUpgradeLog("cannot resolve current exe: %v", err)
		return fmt.Errorf("无法获取当前程序路径: %w", err)
	}
	selfExe, _ = filepath.EvalSymlinks(selfExe)

	destExe := filepath.Join(installDir, "LanAgent.exe")
	backupExe := destExe + backupSuffix
	batPath := filepath.Join(os.TempDir(), upgradeBatName)
	logPath := upgradeLogPath()

	bat := buildUpgradeBat(destExe, backupExe, newExe, batPath, logPath)
	if err := os.WriteFile(batPath, []byte(bat), 0644); err != nil {
		appendUpgradeLog("write upgrade script failed: %v", err)
		return fmt.Errorf("写入升级脚本失败: %w", err)
	}
	appendUpgradeLog("upgrade script written: %s", batPath)

	// 直接以脱离进程树的方式启动 bat。
	//
	// 旧实现用 PowerShell 的 Start-Process -ArgumentList '/c','"path"'，嵌套引号在
	// cmd.exe 的 /c 解析规则下会失效，导致脚本静默不执行（升级"成功"但版本不变）。
	// 这里改为直接 exec cmd /c，由 Go 负责参数传递，并用 DETACHED_PROCESS 让脚本
	// 脱离本进程，避免 Agent 退出时连带杀死脚本。
	c := exec.Command("cmd", "/c", batPath)
	c.Dir = filepath.Dir(batPath)
	c.SysProcAttr = detachedUpgradeAttr()

	if err := c.Start(); err != nil {
		appendUpgradeLog("launch upgrade script failed: %v", err)
		return fmt.Errorf("启动升级脚本失败: %w", err)
	}
	// 不 Wait：脚本需在 Agent 退出后继续运行；交由 DETACHED_PROCESS 独立存活。
	_ = c.Process.Release()

	appendUpgradeLog("upgrade script launched, agent will exit")
	return nil
}

// cleanupUpgradeArtifacts 清理可能残留的升级临时文件（服务启动时调用）。
// 只清理"较旧"的残留（>5 分钟），避免误删刚启动、尚未执行完的升级脚本。
func cleanupUpgradeArtifacts() {
	tmpNew := filepath.Join(os.TempDir(), newExeName)
	tmpBat := filepath.Join(os.TempDir(), upgradeBatName)
	cutoff := time.Now().Add(-5 * time.Minute)
	for _, p := range []string{tmpNew, tmpBat} {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		if fi.ModTime().After(cutoff) {
			// 近期产生的文件可能是正在执行的升级脚本，跳过
			continue
		}
		if err := os.Remove(p); err == nil {
			log.Printf("[upgrade] cleaned leftover %s", p)
		}
	}
}

// upgradeInProgress 标记是否正在升级，避免重复触发
var upgradeRunning bool

func isUpgradeRunning() bool {
	return upgradeRunning
}

func markUpgradeRunning() {
	upgradeRunning = true
}

// ensureServiceRecovery 确保服务配置了自动恢复策略。
//
// 旧版升级流程曾用 `sc failure ... actions= ""` 清空恢复配置，导致服务一旦
// 异常退出就再也不会被拉起。这里在每次服务启动时把策略恢复为
// “失败后 5 秒/10 秒/30 秒各重启一次”，保证意外挂掉能自愈。
func ensureServiceRecovery() {
	if err := exec.Command("cmd", "/c",
		`sc failure LanAgent reset= 86400 actions= restart/5000/restart/10000/restart/30000`,
	).Run(); err != nil {
		log.Printf("[service] set recovery policy failed: %v", err)
	}
}
