package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// 离线自毁：客户端连续无法连接管理端超过阈值后自动卸载并清理程序目录。
//
// 设计约束：
//   - 只有"连不上"才计入离线时长；服务端返回 4xx/5xx 说明网络是通的，不计入。
//     否则管理端升级重启期间会把全网设备误伤。
//   - 阈值由服务端下发（heartbeat 响应字段），出厂默认关闭（0 表示不启用）。
//   - 起始时间持久化到磁盘：设备断电重启后仍能累计，避免重启绕过计时。
//   - 每次成功心跳都会清空计时文件。

const offlineSinceFile = "offline_since"

func offlineSincePath() string {
	return filepath.Join(dataDir, offlineSinceFile)
}

// recordOfflineStart 在首次连接失败时记录时间戳；已存在则保持不变。
func recordOfflineStart() {
	p := offlineSincePath()
	if _, err := os.Stat(p); err == nil {
		return
	}
	_ = os.MkdirAll(dataDir, 0755)
	ts := time.Now().Format(time.RFC3339)
	if err := os.WriteFile(p, []byte(ts), 0644); err != nil {
		log.Printf("[selfdestruct] write offline marker failed: %v", err)
		return
	}
	log.Printf("[selfdestruct] offline timer started at %s", ts)
}

// clearOfflineStart 心跳成功后清空计时，表示设备已恢复在线。
func clearOfflineStart() {
	p := offlineSincePath()
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return
	}
	_ = os.Remove(p)
	log.Printf("[selfdestruct] offline timer cleared (back online)")
}

// checkSelfDestruct 判断离线是否已超过允许时长，超过则执行卸载。
// 返回 true 表示已触发自毁，调用方应停止后续操作。
func checkSelfDestruct(timeoutHours int) bool {
	if timeoutHours <= 0 {
		return false
	}

	p := offlineSincePath()
	data, err := os.ReadFile(p)
	if err != nil {
		return false
	}

	since, err := time.Parse(time.RFC3339, string(data))
	if err != nil {
		// 时间戳损坏，清掉重新计时，避免误删设备
		_ = os.Remove(p)
		return false
	}

	elapsed := time.Since(since)
	limit := time.Duration(timeoutHours) * time.Hour
	if elapsed < limit {
		return false
	}

	log.Printf("[selfdestruct] offline for %v (limit %v), initiating removal", elapsed.Truncate(time.Minute), limit)
	selfDestruct()
	return true
}

// selfDestruct 停服务、删除服务注册、清理程序目录。
//
// 不能直接删除正在运行的 exe，因此生成一个独立 bat：
// 等待本进程退出后再执行清理，最后自删除。
func selfDestruct() {
	batPath := filepath.Join(os.TempDir(), "lanagent_selfdestruct.bat")
	script := "@echo off\r\n" +
		"ping 127.0.0.1 -n 6 >nul\r\n" +
		"net stop LanAgent >nul 2>&1\r\n" +
		"sc delete LanAgent >nul 2>&1\r\n" +
		"taskkill /f /im LanAgent.exe >nul 2>&1\r\n" +
		"ping 127.0.0.1 -n 4 >nul\r\n" +
		`rmdir /s /q "` + installDir + `" >nul 2>&1` + "\r\n" +
		`rmdir /s /q "` + legacyInstallDir + `" >nul 2>&1` + "\r\n" +
		`del "%~f0" >nul 2>&1` + "\r\n"

	if err := os.WriteFile(batPath, []byte(script), 0644); err != nil {
		log.Printf("[selfdestruct] write script failed: %v", err)
		return
	}

	c := exec.Command("cmd", "/c", batPath)
	c.SysProcAttr = detachedUpgradeAttr()
	if err := c.Start(); err != nil {
		log.Printf("[selfdestruct] launch script failed: %v", err)
		return
	}
	_ = c.Process.Release()

	log.Printf("[selfdestruct] removal script launched, agent will exit")
	time.Sleep(2 * time.Second)
	os.Exit(0)
}
