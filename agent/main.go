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
	"unsafe"

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

	fmt.Println("[安装] 创建安装目录...")
	if err := os.MkdirAll(installDir, 0755); err != nil {
		fmt.Printf("[安装] os.MkdirAll 失败(%v)，尝试 cmd mkdir...\n", err)
		if out, err2 := exec.Command("cmd", "/c", "mkdir", installDir).CombinedOutput(); err2 != nil {
			return fmt.Errorf("创建安装目录失败(%s): %w (cmd: %s)", installDir, err, strings.TrimSpace(string(out)))
		}
	}
	destExe := filepath.Join(installDir, "LanAgent.exe")
	destCfg := filepath.Join(installDir, configFileName)

	cfg := &Config{
		ServerURL: serverURL,
		Token:     token,
	}
	cfgData, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println("[安装] 写入配置文件...")
	if err := os.WriteFile(destCfg, cfgData, 0644); err != nil {
		return fmt.Errorf("保存配置文件失败: %w", err)
	}

	fmt.Println("[安装] 连接服务管理器...")
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("连接服务管理器失败（请以管理员身份运行）: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err == nil {
		fmt.Println("[安装] 检测到已有 LAN Agent 服务，正在覆盖安装...")
		s.Control(svc.Stop)
		time.Sleep(2 * time.Second)

		fmt.Println("[安装] 替换程序文件...")
		if err := copyFile(exe, destExe); err != nil {
			s.Close()
			return fmt.Errorf("替换程序文件失败: %w", err)
		}

		fmt.Println("[安装] 重启服务...")
		if err := s.Start(); err != nil {
			s.Close()
			return fmt.Errorf("重启服务失败: %w", err)
		}
		s.Close()
		fmt.Println("[安装] 覆盖安装完成")
		return nil
	}

	fmt.Println("[安装] 复制程序文件到安装目录...")
	if err := copyFile(exe, destExe); err != nil {
		return fmt.Errorf("复制程序文件失败: %w", err)
	}

	// 等待文件写入完成，避免杀毒软件锁定导致后续操作失败
	fmt.Println("[安装] 等待文件就绪...")
	time.Sleep(1 * time.Second)

	fmt.Println("[安装] 创建 Windows 服务...")
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

	s.SetRecoveryActions([]mgr.RecoveryAction{}, 86400)

	fmt.Println("[安装] 启动服务...")
	if err := s.Start(); err != nil {
		// 首次启动可能因文件锁定失败，等待后重试
		fmt.Printf("[安装] 首次启动失败(%v)，等待2秒后重试...\n", err)
		time.Sleep(2 * time.Second)
		if err2 := s.Start(); err2 != nil {
			return fmt.Errorf("启动服务失败(重试后仍失败): %w (首次: %v)", err2, err)
		}
		fmt.Println("[安装] 重试启动成功")
	}

	fmt.Println("[安装] 服务安装并启动成功")
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

// uninstallLog 负责把卸载过程逐步写入 U 盘项目 logs 目录，并同步输出到控制台。
type uninstallLog struct {
	file *os.File
	path string // 日志文件完整路径
}

// logf 输出一条带时间戳的日志到控制台与日志文件。
func (l *uninstallLog) logf(format string, args ...interface{}) {
	line := fmt.Sprintf("[%s] %s", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
	fmt.Println(line)
	if l != nil && l.file != nil {
		fmt.Fprintln(l.file, line)
		l.file.Sync()
	}
}

func (l *uninstallLog) close() {
	if l != nil && l.file != nil {
		l.file.Close()
	}
}

// findRemovableDrive 查找包含项目目录的U盘/移动磁盘。
// 优先匹配 DRIVE_REMOVABLE(2)，其次匹配含目标路径的非系统盘(DRIVE_FIXED=3)。
func findRemovableDrive() string {
	const projectSub = `应用软件\LanAgent-Deploy`
	var fixedCandidate string

	for letter := 'A'; letter <= 'Z'; letter++ {
		root := fmt.Sprintf("%c:\\", letter)
		rootPtr, err := syscall.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		dll := syscall.NewLazyDLL("kernel32.dll")
		proc := dll.NewProc("GetDriveTypeW")
		driveType, _, _ := proc.Call(uintptr(unsafe.Pointer(rootPtr)))

		switch driveType {
		case 2: // DRIVE_REMOVABLE — U盘/SD卡
			return fmt.Sprintf("%c:", letter)
		case 3: // DRIVE_FIXED — 可能是被识别为固定磁盘的U盘/移动硬盘
			if letter == 'C' {
				continue // 跳过系统盘
			}
			targetDir := fmt.Sprintf("%c:\\%s", letter, projectSub)
			if info, err := os.Stat(targetDir); err == nil && info.IsDir() {
				fixedCandidate = fmt.Sprintf("%c:", letter)
			}
		}
	}
	return fixedCandidate
}

// initUninstallLog 定位 U 盘项目路径并创建 logs 目录与日志文件，命名规则：uninstall-时间.log。
func initUninstallLog() (*uninstallLog, string) {
	const projectSub = `应用软件\LanAgent-Deploy`
	drive := findRemovableDrive()
	if drive == "" {
		return nil, "未检测到可移动磁盘(U盘)，日志将只输出到控制台"
	}
	logDir := drive + `\` + projectSub + `\logs`
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Sprintf("创建日志目录失败 %s: %v", logDir, err)
	}
	logFile := filepath.Join(logDir, fmt.Sprintf("uninstall-%s.log", time.Now().Format("20060102_150405")))
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Sprintf("创建日志文件失败 %s: %v", logFile, err)
	}
	return &uninstallLog{file: f, path: logFile}, ""
}

// execCmdOutput 执行命令行并返回合并输出，用于逐步探测权限/命令结果。
func execCmdOutput(cmdline string) string {
	out, err := exec.Command("cmd", "/c", cmdline).CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)) + " (err: " + err.Error() + ")"
	}
	return strings.TrimSpace(string(out))
}

func uninstallService() error {
	// 如果当前exe位于安装目录内，先复制到临时目录再重新执行，避免删除时文件被自身占用
	selfExe, _ := os.Executable()
	selfExe, _ = filepath.EvalSymlinks(selfExe)
	if strings.HasPrefix(strings.ToLower(selfExe), strings.ToLower(installDir)) {
		tmpExe := filepath.Join(os.TempDir(), "LanAgent_uninstall.exe")
		data, err := os.ReadFile(selfExe)
		if err == nil {
			if err2 := os.WriteFile(tmpExe, data, 0755); err2 == nil {
				fmt.Printf("当前exe在安装目录内，已复制到 %s 重新执行卸载...\n", tmpExe)
				cmd := exec.Command(tmpExe, "/uninstall")
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				cmd.Stdin = os.Stdin
				cmd.Run()
				os.Remove(tmpExe)
				return nil
			}
		}
		fmt.Printf("警告: 无法复制到临时目录(%v)，继续尝试直接卸载\n", err)
	}

	// 初始化 U 盘日志
	log, logNote := initUninstallLog()
	defer log.close()
	fmt.Println("=== LAN Agent 卸载开始 ===")
	fmt.Printf("日志文件: %s\n", logNote)

	// 步骤 0：权限检查
	log.logf("[权限检查] 尝试连接服务管理器以判定管理员/SYSTEM 权限...")
	isAdminUser := isAdmin()
	log.logf("[权限检查] isAdmin 结果: %v（false 表示非管理员，通常因未以管理员身份运行或服务管理器拒绝访问）", isAdminUser)
	if !isAdminUser {
		log.logf("[权限检查] 当前进程无管理员权限，可能无法停止/删除服务。请以管理员身份重新运行。")
		return fmt.Errorf("请以管理员身份运行此程序")
	}
	log.logf("[权限检查] 已获得管理员/SYSTEM 权限，可继续执行服务操作")

	// 步骤 1：连接服务管理器
	log.logf("[步骤1] 连接服务管理器(Services Control Manager)...")
	m, err := mgr.Connect()
	if err != nil {
		log.logf("[步骤1] 连接服务管理器失败（权限或系统问题）: %v", err)
		return fmt.Errorf("连接服务管理器失败: %w", err)
	}
	defer m.Disconnect()
	log.logf("[步骤1] 服务管理器连接成功")

	// 步骤 2：打开服务
	log.logf("[步骤2] 打开服务 %s ...", serviceName)
	s, err := m.OpenService(serviceName)
	if err != nil {
		log.logf("[步骤2] 服务 %s 不存在（可能已卸载或从未安装），跳过服务操作，直接清理目录", serviceName)
		cleanInstallDirs(log)
		log.logf("=== LAN Agent 卸载完成（服务不存在，仅清理目录）===")
		return nil
	}
	defer s.Close()
	log.logf("[步骤2] 服务 %s 存在", serviceName)

	// 步骤 3：停止服务
	log.logf("[步骤3] 停止服务 %s ...", serviceName)
	if _, err := s.Control(svc.Stop); err != nil {
		log.logf("[步骤3] 停止服务失败: %v（服务可能已停止或需要更高权限）", err)
	} else {
		log.logf("[步骤3] 停止服务指令已发送，等待服务完全停止...")
		time.Sleep(3 * time.Second)
		log.logf("[步骤3] 服务已停止")
	}

	// 步骤 4：删除服务注册
	log.logf("[步骤4] 删除服务注册 %s ...", serviceName)
	if err := s.Delete(); err != nil {
		log.logf("[步骤4] 删除服务注册失败: %v", err)
		return fmt.Errorf("删除服务失败: %w", err)
	}
	log.logf("[步骤4] 服务注册已删除")

	// 步骤 5：删除事件日志源
	log.logf("[步骤5] 删除事件日志源 %s ...", serviceName)
	if err := eventlog.Remove(serviceName); err != nil {
		log.logf("[步骤5] 删除事件日志源失败(可忽略): %v", err)
	} else {
		log.logf("[步骤5] 事件日志源已删除")
	}

	// 步骤 6：清理安装目录
	log.logf("[步骤6] 清理安装目录...")
	log.logf("[步骤6] 先强制结束残留的 LanAgent.exe 进程")
	killOut := execCmdOutput("taskkill /f /im LanAgent.exe")
	log.logf("[步骤6] taskkill 输出: %s", killOut)
	time.Sleep(2 * time.Second)
	cleanInstallDirs(log)

	log.logf("=== LAN Agent 卸载完成 ===")
	return nil
}

// cleanInstallDirs 删除安装目录与旧数据目录，逐目录记录结果。
func cleanInstallDirs(log *uninstallLog) {
	for _, dir := range []string{installDir, oldDataDir} {
		log.logf("[清理] 删除目录: %s ...", dir)
		if err := os.RemoveAll(dir); err != nil {
			log.logf("[清理] 删除目录失败: %v", err)
			continue
		}
		if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
			log.logf("[清理] 目录已删除: %s", dir)
		} else {
			log.logf("[清理] 目录仍存在（可能文件被占用），需手动检查: %s", dir)
		}
	}
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
