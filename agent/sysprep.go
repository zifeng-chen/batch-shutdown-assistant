package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func getRearmCount() int {
	cmd := exec.Command("cscript", "//nologo", `C:\Windows\System32\slmgr.vbs`, "/dlv")
	out, err := cmd.Output()
	if err != nil {
		return -1
	}
	for _, line := range strings.Split(string(out), "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "remaining windows rearm count") ||
			strings.Contains(lower, "remaining rearm") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				n := 0
				for _, c := range val {
					if c >= '0' && c <= '9' {
						n = n*10 + int(c-'0')
					} else if n > 0 {
						break
					}
				}
				return n
			}
		}
	}
	return -1
}

const sysprepExe = `C:\Windows\System32\Sysprep\sysprep.exe`

// sysprepSuccessTag 是 sysprep 成功完成的标志文件，每次执行前需清除。
const sysprepSuccessTag = `C:\Windows\System32\Sysprep\sysprep_succeeded.tag`

// runSysprep 以 OOBE 模式执行 sysprep。
// reboot 为 true 用 /reboot（重启回 OOBE），false 用 /shutdown（关机，下次开机进 OOBE）。
// 不加 /generalize：需要反复执行，而每次 /generalize 会消耗一次 rearm（默认仅 3 次）。
func runSysprep(reboot bool, unattendPath string) error {
	args := []string{"/quiet", "/oobe"}
	if reboot {
		args = append(args, "/reboot")
	} else {
		args = append(args, "/shutdown")
	}
	if unattendPath != "" {
		args = append(args, "/unattend:"+unattendPath)
	}

	// 清除上次的成功标记，否则无法判断本次是否成功
	_ = os.Remove(sysprepSuccessTag)

	cmd := exec.Command(sysprepExe, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sysprep 执行失败(exit=%v): %s", err, strings.TrimSpace(string(output)))
	}

	// /quiet 下 stdout 为空，只能靠标志文件判定；缺失说明 sysprep 提前退出
	if _, statErr := os.Stat(sysprepSuccessTag); statErr != nil {
		return fmt.Errorf("sysprep 未生成成功标记，可能执行失败（详见 %s）", sysprepErrLog)
	}
	return nil
}

const sysprepErrLog = `C:\Windows\System32\Sysprep\Panther\setuperr.log`

func executeShutdown() error {
	cmd := exec.Command("shutdown", "/s", "/f", "/t", "0")
	return cmd.Run()
}

func executeReboot() error {
	cmd := exec.Command("shutdown", "/r", "/f", "/t", "0")
	return cmd.Run()
}
