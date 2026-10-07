package main

import (
	"os/exec"
	"strconv"
	"strings"
)

// SysInfo 采集到的设备硬件/系统配置信息
type SysInfo struct {
	CPUModel      string `json:"cpu_model,omitempty"`
	CPUCores      int    `json:"cpu_cores,omitempty"`
	MemoryTotalMB int    `json:"memory_total_mb,omitempty"`
	DiskTotalGB   int    `json:"disk_total_gb,omitempty"`
	DiskFreeGB    int    `json:"disk_free_gb,omitempty"`
	OSVersion     string `json:"os_version,omitempty"`
	SerialNumber  string `json:"serial_number,omitempty"`
	MACAddress    string `json:"mac_address,omitempty"`
	Manufacturer  string `json:"manufacturer,omitempty"`
	Model         string `json:"model,omitempty"`
}

// runPowerShell 执行 PowerShell 命令并返回去除首尾空白的输出
func runPowerShell(script string) string {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive",
		"-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = hiddenWindowAttr()
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// collectSysInfo 汇总采集设备信息；任何单项失败都不影响其余项
func collectSysInfo() *SysInfo {
	info := &SysInfo{}

	if v := runPowerShell(
		"(Get-CimInstance Win32_Processor | Select-Object -First 1).Name",
	); v != "" {
		info.CPUModel = v
	}

	if v := runPowerShell(
		"(Get-CimInstance Win32_Processor | Measure-Object -Property NumberOfLogicalProcessors -Sum).Sum",
	); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			info.CPUCores = n
		}
	}

	if v := runPowerShell(
		"[math]::Round((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory/1MB)",
	); v != "" {
		if n, err := strconv.Atoi(strings.Split(v, ".")[0]); err == nil {
			info.MemoryTotalMB = n
		}
	}

	// 系统盘（C:）总容量与剩余空间
	if v := runPowerShell(
		"[math]::Round((Get-CimInstance Win32_LogicalDisk -Filter \"DeviceID='C:'\").Size/1GB)",
	); v != "" {
		if n, err := strconv.Atoi(strings.Split(v, ".")[0]); err == nil {
			info.DiskTotalGB = n
		}
	}
	if v := runPowerShell(
		"[math]::Round((Get-CimInstance Win32_LogicalDisk -Filter \"DeviceID='C:'\").FreeSpace/1GB)",
	); v != "" {
		if n, err := strconv.Atoi(strings.Split(v, ".")[0]); err == nil {
			info.DiskFreeGB = n
		}
	}

	if v := runPowerShell(
		"(Get-CimInstance Win32_OperatingSystem).Caption + ' ' + (Get-CimInstance Win32_OperatingSystem).Version",
	); v != "" {
		info.OSVersion = v
	}

	if v := runPowerShell(
		"(Get-CimInstance Win32_BIOS).SerialNumber",
	); v != "" {
		info.SerialNumber = v
	}

	// 主板厂商与型号（更准确地反映整机品牌机型）
	if v := runPowerShell(
		"(Get-CimInstance Win32_ComputerSystem).Manufacturer",
	); v != "" {
		info.Manufacturer = v
	}
	if v := runPowerShell(
		"(Get-CimInstance Win32_ComputerSystem).Model",
	); v != "" {
		info.Model = v
	}

	// 取第一块物理网卡 MAC（优先有线）
	if v := runPowerShell(
		"(Get-CimInstance Win32_NetworkAdapter -Filter \"PhysicalAdapter=True AND MACAddress IS NOT NULL\" | " +
			"Select-Object -First 1).MACAddress",
	); v != "" {
		info.MACAddress = v
	}

	return info
}

// coreHardwareReady 判断关键字段是否已采集到，用于决定是否需要重试
func (s *SysInfo) coreHardwareReady() bool {
	return s != nil && s.CPUModel != "" && s.MemoryTotalMB > 0
}
