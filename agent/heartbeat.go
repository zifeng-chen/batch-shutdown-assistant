package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"
)

type HeartbeatPayload struct {
	DeviceID   string `json:"device_id"`
	Hostname   string `json:"hostname"`
	OS         string `json:"os"`
	IP         string `json:"ip"`
	RearmCount int    `json:"rearm_count"`
	Status     string `json:"status"`
	AgentVer   string `json:"agent_version"`
	Timestamp  int64  `json:"timestamp"`

	// 硬件信息：仅在采集后随心跳携带（非每次都发），服务端收到即更新
	SysInfo *SysInfo `json:"sysinfo,omitempty"`
}

// sysInfoCache 缓存最近一次采集结果，避免每次心跳都执行耗时的 WMI 查询
var sysInfoCache struct {
	info      *SysInfo
	collected time.Time
}

const sysInfoRefreshInterval = 6 * time.Hour

// getSysInfo 按需刷新并返回缓存的硬件信息。
// force 为 true 时强制重新采集（供 sysinfo 任务使用）。
func getSysInfo(force bool) *SysInfo {
	if !force && sysInfoCache.info != nil &&
		time.Since(sysInfoCache.collected) < sysInfoRefreshInterval {
		return nil // 缓存仍有效，本次心跳不重复上报
	}

	info := collectSysInfo()
	sysInfoCache.info = info
	sysInfoCache.collected = time.Now()
	return info
}

// buildVersion 由编译时 -ldflags "-X main.buildVersion=x.y.z" 注入，是版本号的唯一来源
var buildVersion = "1.4.3"

func getAgentVersion() string {
	return buildVersion
}

func getHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

func getLocalIP() string {
	// simplified: return first non-loopback IPv4
	// in production, enumerate interfaces
	addrs, err := getInterfaceAddrs()
	if err != nil || len(addrs) == 0 {
		return "127.0.0.1"
	}
	return addrs[0]
}

func sendHeartbeat(cfg *Config, status string) error {
	rearm := getRearmCount()

	payload := HeartbeatPayload{
		DeviceID:   cfg.DeviceID,
		Hostname:   getHostname(),
		OS:         fmt.Sprintf("Windows %s", runtime.GOARCH),
		IP:         getLocalIP(),
		RearmCount: rearm,
		Status:     status,
		AgentVer:   getAgentVersion(),
		Timestamp:  time.Now().Unix(),
		SysInfo:    getSysInfo(false),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := cfg.ServerURL + "/api/agents/heartbeat"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	// 始终携带注册密钥：服务端在设备重装（token 轮换）时用它确认合法性。
	// 未配置密钥的旧版本安装包不受影响，仅少了这条 header。
	if cfg.EnrollSecret != "" {
		req.Header.Set("X-Enroll-Secret", cfg.EnrollSecret)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("heartbeat failed: HTTP %d", resp.StatusCode)
	}

	var result struct {
		DeviceID string `json:"device_id"`
		Status   string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		if result.DeviceID != "" && cfg.DeviceID != result.DeviceID {
			cfg.DeviceID = result.DeviceID
			_ = SaveConfig(cfg)
			log.Printf("[heartbeat] saved device_id=%s (status=%s)", result.DeviceID, result.Status)
		}
	}

	return nil
}

func startHeartbeat(cfg *Config, stopCh <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	checkAndRecover(cfg)

	if err := sendHeartbeat(cfg, "online"); err != nil {
		log.Printf("[heartbeat] initial heartbeat failed: %v", err)
	} else {
		log.Printf("[heartbeat] registered with server, device_id=%s", cfg.DeviceID)
	}

	for {
		select {
		case <-stopCh:
			_ = sendHeartbeat(cfg, "offline")
			return
		case <-ticker.C:
			if err := sendHeartbeat(cfg, "online"); err != nil {
				log.Printf("[heartbeat] failed: %v", err)
			}
		}
	}
}

func checkAndRecover(cfg *Config) {
	recoverFlag := dataDir + `\need_recover`
	if _, err := os.Stat(recoverFlag); os.IsNotExist(err) {
		return
	}

	log.Printf("[recover] detected post-sysprep recovery flag, reporting to server")

	payload := map[string]string{
		"device_id": cfg.DeviceID,
		"hostname":  getHostname(),
	}
	body, _ := json.Marshal(payload)

	url := cfg.ServerURL + "/api/agents/recover"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		log.Printf("[recover] build request failed: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[recover] report failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 400 {
		log.Printf("[recover] reported successfully")
		_ = os.Remove(recoverFlag)
	} else {
		log.Printf("[recover] server returned HTTP %d", resp.StatusCode)
	}
}
