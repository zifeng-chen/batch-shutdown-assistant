package main

import "syscall"

// hiddenWindowAttr 返回隐藏控制台窗口的进程属性，
// 避免服务执行外部命令时在用户桌面闪出黑窗口。
func hiddenWindowAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true}
}
