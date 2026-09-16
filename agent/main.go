package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "LanAgent"

type AgentService struct {
	cfg    *Config
	stopCh chan struct{}
	wg     sync.WaitGroup
}

func (s *AgentService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}

	migrateOldData()

	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("[service] load config failed: %v", err)
		return true, 1
	}
	s.cfg = cfg

	s.stopCh = make(chan struct{})
	s.wg.Add(2)
	go s.runHeartbeat()
	go func() {
		time.Sleep(3 * time.Second)
		s.runWebSocket()
	}()

	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	log.Printf("[service] %s started", serviceName)

	for {
		select {
		case cr := <-r:
			switch cr.Cmd {
			case svc.Interrogate:
				changes <- cr.CurrentStatus
			case svc.Stop, svc.Shutdown:
				log.Printf("[service] received stop/shutdown")
				close(s.stopCh)
				s.wg.Wait()
				changes <- svc.Status{State: svc.StopPending}
				return false, 0
			}
		}
	}
}

func (s *AgentService) runHeartbeat() {
	defer s.wg.Done()
	startHeartbeat(s.cfg, s.stopCh)
}

func (s *AgentService) runWebSocket() {
	defer s.wg.Done()

	header := http.Header{}
	header.Set("Authorization", "Bearer "+s.cfg.Token)

	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		// 每次重连都重新读取 device_id（心跳可能刚更新了它）
		cfg, err := LoadConfig()
		if err != nil || cfg.DeviceID == "" {
			log.Printf("[ws] waiting for device_id from heartbeat...")
			time.Sleep(3 * time.Second)
			continue
		}

		wsURL := strings.Replace(cfg.ServerURL, "http", "ws", 1) + "/ws/agents/" + cfg.DeviceID + "?token=" + cfg.Token
		log.Printf("[ws] connecting to %s", wsURL)

		conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
		if err != nil {
			log.Printf("[ws] connect failed: %v, retrying in 5s", err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("[ws] connected")
		s.handleConnection(conn)
		conn.Close()

		select {
		case <-s.stopCh:
			return
		default:
			log.Printf("[ws] disconnected, reconnecting in 3s")
			time.Sleep(3 * time.Second)
		}
	}
}

func (s *AgentService) handleConnection(conn *websocket.Conn) {
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("[ws] read error: %v", err)
				return
			}

			var cmd Command
			if err := json.Unmarshal(message, &cmd); err != nil {
				log.Printf("[ws] invalid command: %v", err)
				continue
			}

			result := handleCommand(s.cfg, cmd)

			resp, _ := json.Marshal(result)
			if err := conn.WriteMessage(websocket.TextMessage, resp); err != nil {
				log.Printf("[ws] write result failed: %v", err)
				return
			}

			log.Printf("[ws] command %s completed: %s", cmd.ID, result.Status)
		}
	}()

	select {
	case <-done:
	case <-s.stopCh:
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}
}

func runInteractive() {
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "usage: LanAgent.exe /install /server=<url> /token=<token>")
		os.Exit(1)
	}

	s := &AgentService{cfg: cfg, stopCh: make(chan struct{})}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		startHeartbeat(cfg, s.stopCh)
	}()
	time.Sleep(2 * time.Second)
	go func() {
		defer wg.Done()
		s.runWebSocket()
	}()

	<-sigCh
	log.Println("[interactive] shutting down")
	close(s.stopCh)
	wg.Wait()
}

func pauseAndExit(code int) {
	fmt.Println()
	fmt.Println("按回车键退出...")
	fmt.Scanln()
	os.Exit(code)
}

func installService(serverURL, token string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取程序路径失败: %w", err)
	}

	_ = os.MkdirAll(installDir, 0755)
	destExe := filepath.Join(installDir, "LanAgent.exe")
	destCfg := filepath.Join(installDir, configFileName)

	cfg := &Config{
		ServerURL: serverURL,
		Token:     token,
	}
	cfgData, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(destCfg, cfgData, 0644); err != nil {
		return fmt.Errorf("保存配置文件失败: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("连接服务管理器失败（请以管理员身份运行）: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err == nil {
		fmt.Println("检测到已有 LAN Agent 服务，正在覆盖安装...")
		s.Control(svc.Stop)
		time.Sleep(2 * time.Second)

		if err := copyFile(exe, destExe); err != nil {
			s.Close()
			return fmt.Errorf("替换程序文件失败: %w", err)
		}

		if err := s.Start(); err != nil {
			s.Close()
			return fmt.Errorf("重启服务失败: %w", err)
		}
		s.Close()
		return nil
	}

	if err := copyFile(exe, destExe); err != nil {
		return fmt.Errorf("复制程序文件失败: %w", err)
	}

	s, err = m.CreateService(serviceName, destExe, mgr.Config{
		StartType:   mgr.StartAutomatic,
		DisplayName: "LAN Agent Service",
		Description: "LAN management agent for remote sysprep and shutdown",
	})
	if err != nil {
		return fmt.Errorf("创建服务失败: %w", err)
	}
	defer s.Close()

	if err := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		log.Printf("[install] event log install warning: %v", err)
	}

	// 不配置 recovery 自动重启：Agent 正常退出（如升级/卸载）时，
	// 由独立的升级/卸载脚本控制重启时机，避免 recovery 抢先加载旧 exe 导致替换失败
	s.SetRecoveryActions([]mgr.RecoveryAction{}, 86400)

	if err := s.Start(); err != nil {
		return fmt.Errorf("启动服务失败: %w", err)
	}

	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

func uninstallService() error {
	// 检查管理员权限
	if !isAdmin() {
		return fmt.Errorf("请以管理员身份运行此程序")
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("连接服务管理器失败: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		// 服务不存在，尝试直接清理目录
		fmt.Println("服务未找到，直接清理安装目录...")
		if e := os.RemoveAll(installDir); e != nil {
			fmt.Printf("删除目录失败: %v\n", e)
		}
		os.RemoveAll(oldDataDir)
		return nil
	}
	defer s.Close()

	fmt.Println("正在停止服务...")
	s.Control(svc.Stop)
	time.Sleep(3 * time.Second)

	fmt.Println("正在删除服务注册...")
	if err := s.Delete(); err != nil {
		return fmt.Errorf("删除服务失败: %w", err)
	}

	_ = eventlog.Remove(serviceName)

	// 用独立 cmd 脚本删除目录：当前进程退出后由脚本完成删除，避免 exe 被自身占用
	fmt.Printf("正在清理安装目录: %s\n", installDir)
	batPath := filepath.Join(os.TempDir(), "lanagent_uninstall.bat")
	batContent := fmt.Sprintf(
		`@echo off`+"\r\n"+
			`ping 127.0.0.1 -n 3 >nul`+"\r\n"+ // 等当前进程退出
			`taskkill /f /im LanAgent.exe >nul 2>&1`+"\r\n"+
			`ping 127.0.0.1 -n 2 >nul`+"\r\n"+
			`rmdir /s /q "%s" >nul 2>&1`+"\r\n"+
			`if exist "%s" (`+"\r\n"+
			`  ping 127.0.0.1 -n 3 >nul`+"\r\n"+
			`  taskkill /f /im LanAgent.exe >nul 2>&1`+"\r\n"+
			`  rmdir /s /q "%s" >nul 2>&1`+"\r\n"+
			`)`+"\r\n"+
			`rmdir /s /q "%s" >nul 2>&1`+"\r\n"+ // 清理旧数据目录
			`del "%s" >nul 2>&1`+"\r\n", // 删除自身 bat
		installDir, installDir, installDir, oldDataDir, batPath,
	)
	if err := os.WriteFile(batPath, []byte(batContent), 0644); err != nil {
		return fmt.Errorf("创建卸载脚本失败: %w", err)
	}

	cmd := exec.Command("cmd", "/c", batPath)
	cmd.Start()

	fmt.Println("LAN Agent 已卸载成功。")
	return nil
}

func isAdmin() bool {
	// 尝试连接服务管理器，非管理员会失败
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	m.Disconnect()
	return true
}

func parseInstallArgs() (serverURL, token string) {
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "/server=") {
			serverURL = strings.TrimPrefix(arg, "/server=")
		}
		if strings.HasPrefix(arg, "/token=") {
			token = strings.TrimPrefix(arg, "/token=")
		}
	}
	return
}

func loadDeployConf() (serverURL string, err error) {
	var candidates []string

	if exe, e := os.Executable(); e == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "deploy.conf"))
	}
	if wd, e := os.Getwd(); e == nil {
		candidates = append(candidates, filepath.Join(wd, "deploy.conf"))
	}

	var data []byte
	found := false
	for _, path := range candidates {
		d, e := os.ReadFile(path)
		if e == nil {
			data = d
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("deploy.conf not found in exe directory or working directory")
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key == "server" {
			serverURL = val
		}
	}
	if serverURL == "" {
		return "", fmt.Errorf("deploy.conf missing server")
	}
	return serverURL, nil
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "/install":
			serverURL, token := parseInstallArgs()
			if serverURL == "" || token == "" {
				fmt.Fprintln(os.Stderr, "用法: LanAgent.exe /install /server=<地址> /token=<令牌>")
				os.Exit(1)
			}
			if err := installService(serverURL, token); err != nil {
				fmt.Fprintf(os.Stderr, "安装失败: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("LAN Agent 服务安装并启动成功。")
			return

		case "/silent":
			serverURL, err := loadDeployConf()
			if err != nil {
				fmt.Fprintf(os.Stderr, "静默安装失败: %v\n", err)
				os.Exit(1)
			}
			token := generateToken()
			if err := installService(serverURL, token); err != nil {
				fmt.Fprintf(os.Stderr, "静默安装失败: %v\n", err)
				os.Exit(1)
			}
			cfg := &Config{ServerURL: serverURL, Token: token}
			_ = sendHeartbeat(cfg, "online")
			return

		case "/uninstall":
			if err := uninstallService(); err != nil {
				fmt.Fprintf(os.Stderr, "卸载失败: %v\n", err)
				pauseAndExit(1)
			}
			fmt.Println("LAN Agent 已卸载成功。")
			pauseAndExit(0)
			return
		}
	}

	// 无参数运行
	localIP := getLocalIP()

	// 有 deploy.conf → 安装模式
	if serverURL, err := loadDeployConf(); err == nil {
		token := generateToken()
		fmt.Println("============================================")
		fmt.Println("  LAN Agent 局域网管理系统 - 安装程序")
		fmt.Println("============================================")
		fmt.Println()
		fmt.Printf("本机IP地址: %s\n", localIP)
		fmt.Printf("服务器地址: %s\n", serverURL)
		fmt.Println()

		fmt.Println("正在安装 LAN Agent 服务...")
		if err := installService(serverURL, token); err != nil {
			fmt.Println()
			fmt.Printf("[安装失败] %v\n", err)
			fmt.Println()
			fmt.Println("常见原因:")
			fmt.Println("  1. 未以管理员身份运行（请右键→以管理员身份运行）")
			fmt.Println("  2. 杀毒软件拦截了服务注册")
			fmt.Println("  3. deploy.conf 配置有误")
			pauseAndExit(1)
		}
		fmt.Println()
		fmt.Println("============================================")
		fmt.Printf("  [成功] LAN Agent 已安装并启动\n")
		fmt.Printf("  本机IP: %s\n", localIP)
		fmt.Println("============================================")

		// 立即发送心跳通知管理端更新token
		cfg := &Config{ServerURL: serverURL, Token: token}
		if err := sendHeartbeat(cfg, "online"); err != nil {
			fmt.Printf("  (心跳通知失败: %v，服务启动后会自动重试)\n", err)
		} else {
			fmt.Println("  已向管理端注册")
		}

		pauseAndExit(0)
		return
	}

	// 没有 deploy.conf → 检查是否作为 Windows 服务运行
	isService, err := svc.IsWindowsService()
	if err != nil {
		// IsWindowsService 失败时，尝试直接以服务方式运行
		log.Printf("IsWindowsService error: %v, trying service mode", err)
		svc.Run(serviceName, &AgentService{})
		return
	}

	if isService {
		elog, err := eventlog.Open(serviceName)
		if err == nil {
			defer elog.Close()
			elog.Info(1, "LanAgent service starting")
		}
		svc.Run(serviceName, &AgentService{})
	} else {
		fmt.Println("未找到 deploy.conf 配置文件。")
		fmt.Println("请将 deploy.conf 放在与本程序相同的目录下。")
		fmt.Println()
		fmt.Println("用法:")
		fmt.Println("  安装: LanAgent.exe          (需要 deploy.conf)")
		fmt.Println("  卸载: LanAgent.exe /uninstall")
		pauseAndExit(1)
	}
}
