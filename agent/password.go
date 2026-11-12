package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// 清空密码后的账户状态必须与工厂模式（Ctrl+Shift+F3 审核模式）一致：
// 空密码。OOBE 阶段一旦写入带密码的账户配置，重启就会提示「密码不正确」。

// execQuiet 执行命令并返回去掉首尾空白的合并输出。
func execQuiet(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = hiddenWindowAttr()
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// activeConsoleUser 取当前登录在控制台的用户名。
// exe 以管理员身份手动运行时进程本身就是该用户；作为服务（SYSTEM）运行时
// %USERNAME% 是机器名$，必须回退到 Win32_ComputerSystem.UserName。
func activeConsoleUser() string {
	if u := strings.TrimSpace(os.Getenv("USERNAME")); u != "" &&
		!strings.EqualFold(u, "SYSTEM") &&
		!strings.HasSuffix(u, "$") {
		return u
	}

	out, err := execQuiet("wmic", "computersystem", "get", "username", "/value")
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if v, ok := strings.CutPrefix(line, "UserName="); ok && v != "" {
				// 形如 DOMAIN\user，去掉域前缀
				if i := strings.LastIndex(v, `\`); i >= 0 {
					v = v[i+1:]
				}
				if v != "" {
					return v
				}
			}
		}
	}

	// 最后回退：查询最近登录的本地账户
	out, err = execQuiet("quser")
	if err == nil {
		lines := strings.Split(out, "\n")
		for i, line := range lines {
			if i == 0 || strings.TrimSpace(line) == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) > 1 {
				return strings.TrimPrefix(fields[0], ">")
			}
		}
	}
	return ""
}

// isLocalAdmin 判断账户是否属于本机 Administrators 组。
func isLocalAdmin(name string) bool {
	out, err := execQuiet("net", "localgroup", "Administrators")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.EqualFold(strings.TrimSpace(line), name) {
			return true
		}
	}
	return false
}

// resetPassword 把指定本地账户的密码清空。
// 只清当前控制台用户：批量清空所有管理员会把备用账户也一起废掉，
// 一旦清错账户就没有任何可用凭据了。
func resetPassword() error {
	name := activeConsoleUser()
	if name == "" {
		return fmt.Errorf("无法确定当前登录账户，请在桌面登录状态下以管理员身份运行")
	}

	if !isLocalAdmin(name) {
		return fmt.Errorf("账户 %q 不是本机管理员，拒绝清空密码（需管理员权限才能恢复）", name)
	}

	fmt.Printf("目标账户: %s\n", name)
	fmt.Println("正在清空密码...")

	out, err := execQuiet("net", "user", name, "")
	if err != nil {
		return fmt.Errorf("清空密码失败: %v\n输出: %s", err, out)
	}

	// 关闭该账户的密码过期要求，避免空密码被策略强制改回
	_, _ = execQuiet("wmic", "useraccount", "where",
		fmt.Sprintf("name='%s'", strings.ReplaceAll(name, "'", "''")),
		"set", "PasswordExpires=false")

	fmt.Println()
	fmt.Println("============================================")
	fmt.Printf("  [成功] 账户 %s 的密码已清空\n", name)
	fmt.Println("  重启后可直接点击该账户进入桌面")
	fmt.Println("============================================")
	return nil
}
