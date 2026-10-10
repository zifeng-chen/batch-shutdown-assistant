package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- 升级脚本生成 ----------

func TestBuildUpgradeBat_AllPathsSubstituted(t *testing.T) {
	bat := buildUpgradeBat(
		`C:\Program Files\LanAgent\LanAgent.exe`,
		`C:\Program Files\LanAgent\LanAgent.exe.old`,
		`C:\Temp\LanAgent_new.exe`,
		`C:\Temp\lanagent_upgrade.bat`,
		`C:\Program Files\LanAgent\logs\upgrade.log`,
	)

	// 不应残留未替换的占位符（%date% 等 cmd 变量除外）
	if strings.Contains(bat, "%s") {
		t.Errorf("脚本中存在未替换的 %%s 占位符")
	}

	// 关键路径必须出现
	for _, want := range []string{
		`C:\Program Files\LanAgent\LanAgent.exe`,
		`C:\Program Files\LanAgent\LanAgent.exe.old`,
		`C:\Temp\LanAgent_new.exe`,
		`C:\Temp\lanagent_upgrade.bat`,
		`C:\Program Files\LanAgent\logs\upgrade.log`,
	} {
		if !strings.Contains(bat, want) {
			t.Errorf("脚本缺少路径: %s", want)
		}
	}
}

func TestBuildUpgradeBat_HasSafetyGuards(t *testing.T) {
	bat := buildUpgradeBat("d", "b", "n", "bat", "log")

	// 必须包含失败判断与回滚分支
	for name, needle := range map[string]string{
		"停止服务":   "sc stop LanAgent",
		"启动服务":   "sc start LanAgent",
		"备份旧版":   "move /Y",
		"检查备份":   "if not exist",
		"启动校验":   `findstr /C:"RUNNING"`,
		"回滚标签":   ":rollback",
		"备份缺失分支": ":rollback_failed",
		"自删除":    "del /f /q",
	} {
		if !strings.Contains(bat, needle) {
			t.Errorf("脚本缺少「%s」相关语句 (%q)", name, needle)
		}
	}
}

func TestBuildUpgradeBat_NoUnreliableCommands(t *testing.T) {
	bat := buildUpgradeBat("d", "b", "n", "bat", "log")

	// 旧实现依赖的 schtasks / timeout 在服务会话下不可靠，必须不再出现
	for _, banned := range []string{"schtasks", "timeout /t"} {
		if strings.Contains(bat, banned) {
			t.Errorf("脚本不应再使用不可靠的命令: %s", banned)
		}
	}
}

func TestBuildUpgradeBat_LogsEverywhere(t *testing.T) {
	bat := buildUpgradeBat("d", "b", "n", "bat", "log")
	// 错误路径必须都写日志，而不是静默失败
	if strings.Count(bat, "ERROR:") < 2 {
		t.Error("脚本的错误分支缺少日志输出")
	}
	if !strings.Contains(bat, "upgrade SUCCESS") {
		t.Error("脚本缺少成功标记")
	}
}

// 回归防护：LOG 变量必须用 set "LOG=..." 的标准写法。
// 旧写法 set LOG="..." 会把引号存进变量，导致 >> "%LOG%" 展开异常。
func TestBuildUpgradeBat_LogVarQuotedCorrectly(t *testing.T) {
	bat := buildUpgradeBat("d", "b", "n", "bat", "log")
	if !strings.Contains(bat, `set "LOG=`) {
		t.Error(`LOG 变量应使用 set "LOG=..." 写法`)
	}
	if strings.Contains(bat, `set LOG="`) {
		t.Error(`不应使用会引入多余引号的 set LOG="..." 写法`)
	}
}

// 回归防护：升级脚本必须以脱离进程树的方式直接启动，
// 不能再依赖嵌套引号的 PowerShell Start-Process（曾导致脚本静默不执行）。
func TestDetachedUpgradeAttr_UsesNativeFlags(t *testing.T) {
	a := detachedUpgradeAttr()
	if a == nil {
		t.Fatal("detachedUpgradeAttr 不应返回 nil")
	}
	// DETACHED_PROCESS(0x8) 必须置位，脚本才能脱离 Agent 进程树
	if a.CreationFlags&0x00000008 == 0 {
		t.Error("缺少 DETACHED_PROCESS 标志，脚本可能随 Agent 退出被杀")
	}
	if !a.HideWindow {
		t.Error("应隐藏窗口避免弹窗")
	}
}

// ---------- PE 校验 ----------

func TestIsPEExecutable_ValidMZ(t *testing.T) {
	p := filepath.Join(t.TempDir(), "fake.exe")
	os.WriteFile(p, []byte{'M', 'Z', 0x90, 0x00, 0x03}, 0644)
	if !isPEExecutable(p) {
		t.Error("合法的 MZ 头应被识别为可执行文件")
	}
}

func TestIsPEExecutable_RejectsHTML(t *testing.T) {
	// 常见故障：下载到错误页面而非 exe
	p := filepath.Join(t.TempDir(), "page.html")
	os.WriteFile(p, []byte("<!DOCTYPE html><html>"), 0644)
	if isPEExecutable(p) {
		t.Error("HTML 不应被识别为可执行文件")
	}
}

func TestIsPEExecutable_RejectsEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "empty")
	os.WriteFile(p, []byte{}, 0644)
	if isPEExecutable(p) {
		t.Error("空文件不应被识别为可执行文件")
	}
}

func TestIsPEExecutable_MissingFile(t *testing.T) {
	if isPEExecutable(filepath.Join(t.TempDir(), "nope.exe")) {
		t.Error("不存在的文件不应被识别为可执行文件")
	}
}

// ---------- 升级状态标记 ----------

func TestUpgradeRunningFlag(t *testing.T) {
	upgradeRunning = false
	if isUpgradeRunning() {
		t.Fatal("初始状态应为未升级")
	}
	markUpgradeRunning()
	if !isUpgradeRunning() {
		t.Error("标记后应处于升级中")
	}
	upgradeRunning = false // 复位，避免影响其他测试
}

// ---------- 上传包大小阈值 ----------

func TestMinExeSizeThresholdSane(t *testing.T) {
	// 阈值应大于 1MB，避免把错误页面当成升级包
	if minExeSizeBytes < 1024*1024 {
		t.Errorf("最小包大小阈值过小: %d", minExeSizeBytes)
	}
}

// ---------- 下载地址来源校验 ----------

func TestValidateServerURL_SameOrigin(t *testing.T) {
	cfg := &Config{ServerURL: "http://192.168.2.57:8080"}
	for _, u := range []string{
		"http://192.168.2.57:8080/api/upgrade/latest-download",
		"http://192.168.2.57:8080/api/upgrade/file/abc.exe",
	} {
		if err := validateServerURL(cfg, u); err != nil {
			t.Errorf("同源地址应放行: %s (%v)", u, err)
		}
	}
}

func TestValidateServerURL_RejectsForeignHost(t *testing.T) {
	cfg := &Config{ServerURL: "http://192.168.2.57:8080"}
	for _, u := range []string{
		"http://evil.example.com/payload.exe",
		"http://192.168.2.99:8080/payload.exe", // 同端口不同 IP
		"http://192.168.2.57:9999/payload.exe", // 同 IP 不同端口
	} {
		if err := validateServerURL(cfg, u); err == nil {
			t.Errorf("非本服务器地址应被拒绝: %s", u)
		}
	}
}

func TestValidateServerURL_RejectsBadScheme(t *testing.T) {
	cfg := &Config{ServerURL: "http://192.168.2.57:8080"}
	if err := validateServerURL(cfg, "ftp://192.168.2.57:8080/x.exe"); err == nil {
		t.Error("非 http(s) 协议应被拒绝")
	}
}

// ---------- 路径穿越防护 ----------

func TestSafeJoinUnder_Normal(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "data", "files")
	got, err := safeJoinUnder(base, "report.txt")
	if err != nil {
		t.Fatalf("正常文件名不应报错: %v", err)
	}
	if got != filepath.Join(base, "report.txt") {
		t.Errorf("拼接结果异常: %s", got)
	}
}

func TestSafeJoinUnder_RejectsTraversal(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "data", "files")
	for _, name := range []string{
		"../evil.exe",
		"..\\evil.exe",
		"sub/../../evil.exe",
	} {
		if _, err := safeJoinUnder(base, name); err == nil {
			t.Errorf("穿越路径应被拒绝: %s", name)
		}
	}
}

func TestSafeJoinUnder_RejectsAbsolute(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "data", "files")
	abs := filepath.Join(string(filepath.Separator), "Windows", "System32", "evil.dll")
	if _, err := safeJoinUnder(base, abs); err == nil {
		t.Error("绝对路径应被拒绝")
	}
}

func TestSafeJoinUnder_RejectsEmpty(t *testing.T) {
	if _, err := safeJoinUnder("/tmp", ""); err == nil {
		t.Error("空文件名应被拒绝")
	}
}
