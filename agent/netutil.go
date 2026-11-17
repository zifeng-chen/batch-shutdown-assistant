package main

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
)

// validateServerURL 校验下载地址是否指向本管理服务器。
// 防止被诱导从任意第三方地址下载可执行文件。
func validateServerURL(cfg *Config, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("非法下载地址: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("不支持的下载协议: %s", u.Scheme)
	}
	srv, err := url.Parse(cfg.ServerURL)
	if err != nil {
		return fmt.Errorf("服务器地址配置无效: %w", err)
	}
	if !strings.EqualFold(u.Host, srv.Host) {
		return fmt.Errorf("拒绝非管理服务器来源: %s (期望 %s)", u.Host, srv.Host)
	}
	return nil
}

// safeJoinUnder 将 name 安全地拼接到 baseDir 之下，
// 拒绝绝对路径与 ".." 逃逸。返回最终路径。
func safeJoinUnder(baseDir, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("文件名为空")
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("拒绝绝对路径: %s", name)
	}
	clean := filepath.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("拒绝路径穿越: %s", name)
	}
	final := filepath.Join(baseDir, clean)
	baseClean := filepath.Clean(baseDir)
	if final != baseClean && !strings.HasPrefix(final, baseClean+string(filepath.Separator)) {
		return "", fmt.Errorf("目标路径越界: %s", final)
	}
	return final, nil
}

// ipPriority 给候选地址打分，数值越小越优先。
// 多网卡机器（有线/无线/虚拟网卡/蓝牙）枚举顺序不稳定，
// 必须优先选真正能代表本机局域网位置的地址。
func ipPriority(ip net.IP) int {
	ip4 := ip.To4()
	if ip4 == nil {
		return 99
	}
	// 回环、链路本地(169.254)、0.x 都不是可路由的业务地址
	if ip4.IsLoopback() || ip4.IsLinkLocalUnicast() || ip4.IsUnspecified() {
		return 90
	}
	// 常见虚拟网卡网段，优先级最低
	if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
		return 50
	}
	if ip4[0] == 10 {
		return 20
	}
	if ip4[0] == 192 && ip4[1] == 168 {
		return 10
	}
	return 30
}

func getInterfaceAddrs() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	type candidate struct {
		ip       string
		priority int
	}
	var candidates []candidate
	seen := map[string]bool{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		ipAddrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range ipAddrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			if ipNet.IP.To4() == nil || ipNet.IP.IsLoopback() {
				continue
			}
			text := ipNet.IP.String()
			if seen[text] {
				continue
			}
			seen[text] = true
			candidates = append(candidates, candidate{ip: text, priority: ipPriority(ipNet.IP)})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].priority < candidates[j].priority
	})

	addrs := make([]string, 0, len(candidates))
	for _, item := range candidates {
		addrs = append(addrs, item.ip)
	}
	return addrs, nil
}
