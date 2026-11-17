package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
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
var buildVersion = "1.7.6"

// normalizeDeviceID 把服务端返回的 device_id 统一成字符串。
//
// 服务端返回的是数据库自增主键（JSON 数字），旧版服务端返回的是 UUID 字符串，
// 两种都要能接住。用 interface{} 接收后在这里归一，避免类型不匹配导致
// 整个响应解析失败——那会让 device_id 永远是空值，WebSocket 也就永远连不上。
func normalizeDeviceID(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		// JSON 数字统一解析为 float64；主键是整数，去掉小数部分避免出现 26.000000
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	case bool:
		return ""
	default:
		return ""
	}
}

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
	// getInterfaceAddrs 已按业务优先级排序（192.168 > 10 > 172.16-31 > 其他），
	// 取第一个即可，避免多网卡时误报虚拟网卡或链路本地地址。
	addrs, err := getInterfaceAddrs()
	if err != nil || len(addrs) == 0 {
		return ""
	}
	first := addrs[0]
	// 全是无效地址（未联网/只有虚拟网卡）时返回空，由管理端保留上一次的有效 IP
	if ip := net.ParseIP(first); ip == nil || ipPriority(ip) >= 90 {
		return ""
	}
	return first
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

	resp, err := agentHTTPClient.Do(req)
	if err != nil {
		// 连不上管理端：开始/继续计时，供离线自毁判定使用
		recordOfflineStart()
		// 此时 WebSocket 也连不上，直接进入静默，只留一条断开记录
		if !logIsOffline() {
			logMarkDisconnected(err.Error())
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// 网络是通的，仅服务端异常（如升级重启），不计入离线时长
		return fmt.Errorf("heartbeat failed: HTTP %d", resp.StatusCode)
	}

	// 连接成功即视为在线，清空离线计时
	clearOfflineStart()
	logMarkConnected()

	// 服务端统一包一层信封：{code, data, msg}，device_id / token / commands 全在 data 里。
	// 直接按顶层字段解析不会报错，只会静默拿到空值——历史上就是这么丢的 device_id。
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
		Msg  string          `json:"msg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		log.Printf("[heartbeat] 解析响应失败: %v", err)
	} else {
		var result struct {
			// device_id 是数据库自增主键，JSON 里是数字，用泛型接收后再统一转字符串
			DeviceID     interface{} `json:"device_id"`
			Token        string      `json:"token"`
			Action       string      `json:"action"`
			OfflineHours *int        `json:"offline_selfdestruct_hours"`
		}
		if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
			if err := json.Unmarshal(envelope.Data, &result); err != nil {
				log.Printf("[heartbeat] 解析 data 失败: %v，原始片段: %.200s", err, envelope.Data)
			}
		}
		// 服务端轮换/下发令牌时同步，否则重装后的设备会一直用旧令牌被拒
		if result.Token != "" && cfg.Token != result.Token {
			cfg.Token = result.Token
			setLiveConfig(cfg)
			if err := SaveConfig(cfg); err != nil {
				log.Printf("[heartbeat] 保存 token 失败: %v（WebSocket 改用内存值继续）", err)
			}
		}
		deviceID := normalizeDeviceID(result.DeviceID)
		if deviceID != "" && cfg.DeviceID != deviceID {
			cfg.DeviceID = deviceID
			// 先同步到内存，保证 WebSocket 立即能拿到，不依赖磁盘写入是否成功
			setLiveConfig(cfg)
			// 写入失败必须显式记录：config.json 落在安装目录，
			// 若该目录不可写，device_id 每次心跳后都会丢失，
			// WebSocket 会一直卡在 waiting for device_id，指令永远收不到。
			if err := SaveConfig(cfg); err != nil {
				log.Printf("[heartbeat] 保存 device_id 失败: %v（WebSocket 改用内存值继续）", err)
			} else {
				log.Printf("[heartbeat] saved device_id=%s (action=%s code=%d)", deviceID, result.Action, envelope.Code)
			}
		}
		// 阈值随心跳下发，允许管理端调整；0 表示关闭。缺省时不动现值。
		if result.OfflineHours != nil && *result.OfflineHours >= 0 {
			offlineSelfDestructHours = *result.OfflineHours
		}
	}

	return nil
}

// offlineSelfDestructHours 是离线自毁阈值，由服务端心跳下发。
// 0 表示不启用（默认），避免管理端故障或网络中断时误伤设备。
var offlineSelfDestructHours int

func startHeartbeat(cfg *Config, stopCh <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	checkAndRecover(cfg)

	// 启动时先检查：设备可能关机期间已超期
	if checkSelfDestruct(offlineSelfDestructHours) {
		return
	}

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
			// 每轮心跳后检查离线时长，超阈值则自行卸载
			if checkSelfDestruct(offlineSelfDestructHours) {
				return
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

	resp, err := agentHTTPClient.Do(req)
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
