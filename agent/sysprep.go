package main

import (
	"fmt"
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

func executeSysprep(unattendPath string) error {
	args := []string{"/quiet", "/oobe", "/shutdown"}
	if unattendPath != "" {
		args = append(args, "/unattend:"+unattendPath)
	}

	cmd := exec.Command(`C:\Windows\System32\Sysprep\sysprep.exe`, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sysprep failed: %v, output: %s", err, string(output))
	}
	return nil
}

func executeSysprepReboot(unattendPath string) error {
	args := []string{"/quiet", "/oobe", "/quit"}
	if unattendPath != "" {
		args = append(args, "/unattend:"+unattendPath)
	}

	cmd := exec.Command(`C:\Windows\System32\Sysprep\sysprep.exe`, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sysprep failed: %v, output: %s", err, string(output))
	}
	return nil
}

func executeShutdown() error {
	cmd := exec.Command("shutdown", "/s", "/f", "/t", "0")
	return cmd.Run()
}

func executeReboot() error {
	cmd := exec.Command("shutdown", "/r", "/f", "/t", "0")
	return cmd.Run()
}
