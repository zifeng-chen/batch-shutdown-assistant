package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// 日志策略：
//
//  1. logs\agent.log 只保留最近 logRetention 内的内容，旧记录在每次写入时被裁掉，
//     文件天然有上限，长期运行不会把客户磁盘写满。
//  2. 与管理端断开后不再记录业务日志，只在断开那一刻写一条断开时间，
//     断开期间的重连噪音全部丢弃；连接恢复后再重新开始记录。
const (
	logRetention = 10 * time.Minute
	// 单条上限，避免异常超长内容把整个窗口挤掉
	logMaxLineBytes = 8 * 1024
)

var agentLog struct {
	mu  sync.Mutex
	f   *os.File
	off bool // true 表示与管理端断开，业务日志一律丢弃
}

// initFileLog 把日志输出接到安装目录下的 logs\agent.log。
// 窗口裁剪由 writer 完成，这里不再按大小滚动。
func initFileLog() {
	agentLog.mu.Lock()
	defer agentLog.mu.Unlock()
	if agentLog.f != nil {
		return
	}
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		log.Printf("[log] 创建日志目录失败: %v", err)
		return
	}
	path := filepath.Join(logsDir, "agent.log")

	// 每次服务启动都重开文件：上次运行的内容已超出关注范围，直接清掉
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		log.Printf("[log] 打开日志文件失败(%s): %v", path, err)
		return
	}
	agentLog.f = f
	log.SetOutput(io.MultiWriter(agentLogWriter{}, os.Stderr))
}

type agentLogWriter struct{}

// Write 让标准 log 包写出的内容经过 10 分钟窗口裁剪后落盘。
func (agentLogWriter) Write(p []byte) (int, error) {
	agentLog.mu.Lock()
	defer agentLog.mu.Unlock()

	if agentLog.f == nil {
		return len(p), nil
	}
	if agentLog.off {
		return len(p), nil
	}
	appendLogLocked(p)
	return len(p), nil
}

// appendLogLocked 裁剪窗口后追加内容，调用方必须已持有 agentLog.mu。
func appendLogLocked(p []byte) {
	if len(p) > logMaxLineBytes {
		p = append(p[:logMaxLineBytes:logMaxLineBytes], '\n')
	}
	path := agentLog.f.Name()
	content, _ := os.ReadFile(path)
	content = trimLogContent(content, time.Now().Add(-logRetention))
	content = append(content, p...)
	_ = os.WriteFile(path, content, 0644)
}

// parseLogLineTime 解析日志开头的 "2006/01/02 15:04:05" 时间戳（19 字节）。
func parseLogLineTime(b []byte) (time.Time, bool) {
	if len(b) < 19 {
		return time.Time{}, false
	}
	head := string(b[:19])
	for _, layout := range []string{"2006/01/02 15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, head, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// trimLogContent 去掉所有早于 cutoff 的记录，返回处理后的内容。
//
// 以「行首带时间戳」作为一条记录（含其续行）的起点，时间戳过期的整段一起丢弃；
// 末尾没有时间戳的续行属于最新记录，需要保留。
func trimLogContent(content []byte, cutoff time.Time) []byte {
	if len(content) == 0 {
		return content
	}
	lines := bytes.Split(content, []byte("\n"))

	start := -1
	for i, line := range lines {
		t, ok := parseLogLineTime(line)
		if ok && !t.Before(cutoff) {
			start = i
			break
		}
	}
	if start < 0 {
		start = len(lines)
		for i := len(lines) - 1; i >= 0; i-- {
			if _, ok := parseLogLineTime(lines[i]); ok {
				start = i + 1
				break
			}
		}
	}
	out := bytes.Join(lines[start:], []byte("\n"))
	for len(out) > 0 && out[0] == '\n' {
		out = out[1:]
	}
	return out
}

// logIsOffline 返回当前是否处于「与管理端断开」的静默状态。
func logIsOffline() bool {
	agentLog.mu.Lock()
	defer agentLog.mu.Unlock()
	return agentLog.off
}

// logMarkConnected 记录与管理端恢复连通。
// 上一段是断线状态时清空文件，让日志只反映当前这段会话。
func logMarkConnected() {
	agentLog.mu.Lock()
	wasOffline := agentLog.off
	agentLog.off = false
	if wasOffline && agentLog.f != nil {
		_ = agentLog.f.Truncate(0)
		_, _ = agentLog.f.Seek(0, io.SeekStart)
	}
	agentLog.mu.Unlock()

	// 必须在锁外写日志：log 包会回调 Write 重入同一把锁
	if wasOffline {
		log.Printf("[log] 与管理端连接恢复，重新开始记录")
	}
}

// logMarkDisconnected 记录断开时间并停止后续写入。
// 重复调用只在第一次生效，避免每次重连失败都追加一条。
func logMarkDisconnected(reason string) {
	agentLog.mu.Lock()
	if agentLog.off {
		agentLog.mu.Unlock()
		return
	}
	agentLog.off = true

	// 这条是断开前最后一条记录，必须绕过静默判断直接落盘
	if agentLog.f != nil {
		line := logLinePrefix() + "[log] 与管理端断开 " + time.Now().Format("2006/01/02 15:04:05")
		if reason = strings.TrimSpace(reason); reason != "" {
			line += "，原因: " + reason
		}
		appendLogLocked([]byte(line + "\n"))
		_ = agentLog.f.Sync()
	}
	agentLog.mu.Unlock()
}

// logLinePrefix 复现标准 log 包的时间戳前缀（LstdFlags），供绕过 log 输出时使用。
func logLinePrefix() string {
	return time.Now().Format("2006/01/02 15:04:05") + " "
}
