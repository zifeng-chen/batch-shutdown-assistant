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