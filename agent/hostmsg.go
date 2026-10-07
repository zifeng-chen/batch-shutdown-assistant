package main

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

// 消息框按钮/图标常量
const (
	mbOK              = 0x00000000
	mbIconInformation = 0x00000040
	mbIconWarning     = 0x00000030
	mbSetForeground   = 0x00010000
	mbTopmost         = 0x00040000
)

// WTS 会话信息类
const wtsCurrentServerHandle = 0

var (
	wtsapi32              = syscall.NewLazyDLL("wtsapi32.dll")
	procWTSQueryUserToken = wtsapi32.NewProc("WTSQueryUserToken")
	procWTSSendMessageW   = wtsapi32.NewProc("WTSSendMessageW")

	kernel32                         = syscall.NewLazyDLL("kernel32.dll")
	procWTSGetActiveConsoleSessionId = kernel32.NewProc("WTSGetActiveConsoleSessionId")
	procProcessIdToSessionId         = kernel32.NewProc("ProcessIdToSessionId")
)

// activeSessionID 返回当前活动控制台会话 ID（即已登录用户的会话）。
// Agent 作为服务运行在 Session 0，必须显式定位交互式会话才能把窗口弹到用户桌面。
func activeSessionID() uint32 {
	// 优先取活动控制台会话
	ret, _, _ := procWTSGetActiveConsoleSessionId.Call()
	sid := uint32(ret)
	if sid != 0xFFFFFFFF {
		return sid
	}

	// 退化方案：取 explorer.exe 所在会话
	script := "(Get-Process explorer -ErrorAction SilentlyContinue | Select-Object -First 1).SessionId"
	if v := runPowerShell(script); v != "" {
		var n uint32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}

	// 再退化：取 winlogon 会话
	script = "(Get-Process winlogon -ErrorAction SilentlyContinue | Select-Object -First 1).SessionId"
	if v := runPowerShell(script); v != "" {
		var n uint32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}

	return 0
}

// showHostMessage 向用户桌面弹出提示窗口。
// title/body 为窗口标题与正文，timeoutSec 为自动关闭秒数（0 表示不自动关闭）。
// 用户在弹窗上点击「确定」可立即关闭。
func showHostMessage(title, body string, timeoutSec uint32) error {
	sessionID := activeSessionID()
	if sessionID == 0 {
		return fmt.Errorf("未找到可用的交互式登录会话（可能无用户登录）")
	}

	titlePtr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return err
	}
	bodyPtr, err := syscall.UTF16PtrFromString(body)
	if err != nil {
		return err
	}

	var resp uint32
	var bytesWritten uint32

	style := uint32(mbOK | mbIconInformation | mbSetForeground | mbTopmost)

	ret, _, callErr := procWTSSendMessageW.Call(
		wtsCurrentServerHandle,
		uintptr(sessionID),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(len(title)*2),
		uintptr(unsafe.Pointer(bodyPtr)),
		uintptr(len(body)*2),
		uintptr(style),
		uintptr(timeoutSec),
		uintptr(unsafe.Pointer(&resp)),
		uintptr(unsafe.Pointer(&bytesWritten)),
		0,
	)

	if ret == 0 {
		return fmt.Errorf("WTSSendMessageW 调用失败: %v", callErr)
	}
	return nil
}

// buildHostMessage 依据设备自身信息组装弹窗正文
func buildHostMessage(info *SysInfo, ip, hostname, agentVersion string) (string, string) {
	title := "设备信息提示"

	var b strings.Builder
	b.WriteString("本机配置信息如下：\r\n\r\n")
	b.WriteString(fmt.Sprintf("主机名：%s\r\n", orDash(hostname)))
	b.WriteString(fmt.Sprintf("IP 地址：%s\r\n", orDash(ip)))
	if info != nil {
		b.WriteString(fmt.Sprintf("操作系统：%s\r\n", orDash(info.OSVersion)))
		b.WriteString(fmt.Sprintf("CPU：%s\r\n", orDash(info.CPUModel)))
		if info.CPUCores > 0 {
			b.WriteString(fmt.Sprintf("CPU 核心数：%d\r\n", info.CPUCores))
		}
		if info.MemoryTotalMB > 0 {
			b.WriteString(fmt.Sprintf("内存：%d MB\r\n", info.MemoryTotalMB))
		}
		if info.DiskTotalGB > 0 {
			b.WriteString(fmt.Sprintf("C 盘容量：%d GB（剩余 %d GB）\r\n",
				info.DiskTotalGB, info.DiskFreeGB))
		}
		b.WriteString(fmt.Sprintf("序列号：%s\r\n", orDash(info.SerialNumber)))
		b.WriteString(fmt.Sprintf("MAC 地址：%s\r\n", orDash(info.MACAddress)))
	}
	b.WriteString(fmt.Sprintf("Agent 版本：%s\r\n", orDash(agentVersion)))
	b.WriteString("\r\n本窗口将在 15 秒后自动关闭。")

	return title, b.String()
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
