package main

import "syscall"

// hiddenWindowAttr 返回隐藏控制台窗口的进程属性，
// 避免服务执行外部命令时在用户桌面闪出黑窗口。
func hiddenWindowAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true}
}

// detachedUpgradeAttr 让升级脚本脱离本进程（及其作业对象），
// 使 Agent 退出后脚本仍能独立完成 exe 替换与服务重启。
// CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS | CREATE_NO_WINDOW
func detachedUpgradeAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x00000200 | 0x00000008 | 0x08000000,
	}
}
