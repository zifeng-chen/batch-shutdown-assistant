package main

import (
	"strings"
	"testing"
	"time"
)

func line(ts time.Time, msg string) string {
	return ts.Format("2006/01/02 15:04:05") + " main.go:70: " + msg + "\n"
}

func TestTrimLogContentKeepsWindow(t *testing.T) {
	now := time.Now()
	content := line(now.Add(-30*time.Minute), "old") +
		line(now.Add(-11*time.Minute), "just-outside") +
		line(now.Add(-9*time.Minute), "inside-1") +
		line(now.Add(-1*time.Minute), "inside-2")

	got := string(trimLogContent([]byte(content), now.Add(-logRetention)))

	if strings.Contains(got, "old") || strings.Contains(got, "just-outside") {
		t.Fatalf("窗口外的行没有被裁掉:\n%s", got)
	}
	if !strings.Contains(got, "inside-1") || !strings.Contains(got, "inside-2") {
		t.Fatalf("窗口内的行被误删:\n%s", got)
	}
	if strings.HasPrefix(got, "\n") {
		t.Fatalf("裁剪后残留空行: %q", got)
	}
}

func TestTrimLogContentAllExpired(t *testing.T) {
	now := time.Now()
	content := line(now.Add(-20*time.Minute), "a") + line(now.Add(-15*time.Minute), "b")
	if got := trimLogContent([]byte(content), now.Add(-logRetention)); got != nil && len(got) != 0 {
		t.Fatalf("全部过期时应清空，实际: %q", string(got))
	}
}

func TestTrimLogContentKeepsContinuation(t *testing.T) {
	now := time.Now()
	content := line(now.Add(-20*time.Minute), "header") + "  续行内容\n"
	got := string(trimLogContent([]byte(content), now.Add(-logRetention)))
	if !strings.Contains(got, "续行内容") {
		t.Fatalf("无时间前缀的续行应保留: %q", got)
	}
	if strings.Contains(got, "header") {
		t.Fatalf("过期的时间行应被删除: %q", got)
	}
}

func TestOfflineStopsWriting(t *testing.T) {
	agentLog.off = true
	if _, err := (agentLogWriter{}).Write([]byte("2026/11/17 16:00:00 suppressed\n")); err != nil {
		t.Fatal(err)
	}
	agentLog.off = false
}
