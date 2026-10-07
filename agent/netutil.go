package main

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
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

func getInterfaceAddrs() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var addrs []string
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
			if ipNet.IP.To4() != nil && !ipNet.IP.IsLoopback() {
				addrs = append(addrs, ipNet.IP.String())
			}
		}
	}
	return addrs, nil
}
