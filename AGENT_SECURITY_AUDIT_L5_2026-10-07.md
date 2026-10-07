# LAN Agent —— Agent 侧专项安全走查（L5）

**范围**：`agent/`（Go 客户端，10 个文件，约 1,600 行）
**方法**：全量通读 `main.go / handler.go / sysprep.go / upgrade.go / config.go / heartbeat.go / sysinfo.go / proc.go / netutil.go / hostmsg.go`，结合 Server 侧命令构造链路分析
**日期**：2026-10-07

---

## 一、总体结论

Agent 运行在**被管控主机**上、以 **SYSTEM 权限**常驻，是整条攻击链的**终点**。整体实现比预期规范——**命令分发是严格的白名单 switch**，没有把参数拼进 shell 执行。但发现 **2 个高危**问题，均与"**Agent 无条件信任服务端下发的 URL**"有关。

**核心风险模型**：Agent 对"服务器"有绝对信任。任何能**冒充服务器**或**篡改 WS 消息**的人，都能让所有被管控主机执行任意代码。因此 Agent 侧的 URL 校验和传输加密是重中之重。

---

## 二、高危问题

### A-H1｜升级/推文件 URL 无来源校验，可被诱导下载任意文件
**位置**：`agent/handler.go:145-178`（upgrade）、`handler.go:180-225`（file_push）、`agent/upgrade.go:47-88`

```go
// upgrade: 直接用服务端给的 URL 下载并执行
if err := startUpgrade(cfg, params.URL, params.Version); err != nil { ... }

// file_push: 直接把服务端给的 URL 内容写到服务端指定的目录
resp, err := http.Get(params.URL)
...
f, err := os.Create(destPath)
```

**问题**：
1. `params.URL` **完全不校验来源**——没有校验"该 URL 是否指向本管理服务器"。`upgrade.go:81` 只校验了下载文件的 PE 头（`MZ`），**任何人只要让 Agent 下载一个 PE 文件就能执行**。
2. `file_push` 可写入**任意目录**（`params.DestDir` 直接 `MkdirAll` + `Create`），例如写入 `C:\Windows\System32` 或启动目录。
3. 全程 **HTTP 明文**（`http.Get`，`cfg.ServerURL` 用 `http://`）——局域网内可被 ARP 欺骗/中间人篡改 WS 消息或替换下载内容。

**攻击路径**：内网攻击者 ARP 欺骗 → 劫持 Server↔Agent 连接 → 下发 `upgrade` 指令指向恶意 exe → **全部 14 台主机以 SYSTEM 权限执行恶意代码**。

**建议**（按性价比排序）：
1. **校验下载源域名**：`upgrade`/`file_push` 的 URL 必须与 `cfg.ServerURL` 同源（host 一致），否则拒绝。**改动小、收益大**。
2. **限制 file_push 目标目录**：`DestDir` 白名单（仅允许 `filesDir` 及子目录），拒绝绝对路径与 `..`。
3. **升级包哈希/签名校验**：Server 下发 `sha256`，Agent 下载后比对；条件允许则做数字签名验证。

### A-H2｜token 经 URL query 传输 + 固定 AES 密钥
**位置**：`agent/main.go:103`、`agent/config.go:76`

```go
// main.go: token 拼在 URL query 里
wsURL := ... + "/ws/agents/" + cfg.DeviceID + "?token=" + cfg.Token

// config.go: 硬编码的 16 字节 AES 密钥
var encryptionKey = []byte("LanAgent2024Secr")
```

**问题**：
1. token 出现在 **WS URL 的 query string**，会被中间代理、服务器访问日志、抓包记录。
2. `EncryptToken/DecryptToken` 使用**硬编码的 AES 密钥**（源码可见）——等于没有加密，任何拿到源码的人都能解出 token。
3. 且经排查，这两个函数**当前只有定义、未见调用**（`config.json` 里 token 是明文存储的），属于"看似加密实则误导"的死代码。

**建议**：
1. token 传输改 `Sec-WebSocket-Protocol` 或**首帧认证**（该改动涉及三端，归入 H2-c 一并做）。
2. **删除或修正** `EncryptToken/DecryptToken`——若要用，密钥必须来自运行时派生（如 DPAPI / 机器绑定），不能用硬编码常量。
3. 评估用 Windows **DPAPI** 加密本地 `config.json` 中的 token。

---

## 三、中危问题

### A-M1｜`file_push` 路径穿越
**位置**：`agent/handler.go:195-200`

```go
destDir := params.DestDir
if destDir == "" { destDir = filesDir }
_ = os.MkdirAll(destDir, 0755)
destPath := filepath.Join(destDir, params.Filename)
```

`Filename` 若含 `..\..\` 或为绝对路径，`filepath.Join` 可逃逸出目标目录；`DestDir` 本身也任意。**与 A-H1 第 2 条同源**，一并修复。

### A-M2｜sysprep 默认口令硬编码、密码明文写入 unattend
**位置**：`agent/handler.go:331-334`、`:360-363`

```go
if adminPass == "" { adminPass = "ChangeMe@2024" }
...
<Password><Value>%s</Value><PlainText>true</PlainText></Password>
```

- 未传密码时用**硬编码默认口令** `ChangeMe@2024`——所有机器一个弱口令，且写入 `unattend.xml`（明文，位于安装目录）。
- **建议**：去掉默认口令（未提供则报错），强制由管理端随机生成；unattend.xml 用后即删。

### A-M3｜升级脚本路径未加引号转义（潜在注入）
**位置**：`agent/upgrade.go:215-218`

```go
psCmd := fmt.Sprintf(
    `Start-Process -FilePath "cmd.exe" -ArgumentList '/c','"%s"' -WindowStyle Hidden`,
    batPath,
)
```

`batPath` 由固定路径拼接（`os.TempDir()` + 固定名），**当前不可控**，所以暂不可利用。但该模式（把变量拼进 PowerShell 命令串）很脆弱——一旦 `batPath` 来源变化即成注入点。**建议**改用 `exec.Command` 参数数组，不拼字符串。

---

## 四、低危 / 规范问题

| 编号 | 位置 | 问题 | 建议 |
|------|------|------|------|
| A-L1 | `handler.go:284-299` | 卸载 bat 里 `installDir` 直接拼进命令（当前为常量，不可控） | 常量入 bat 前做转义，防未来变更 |
| A-L2 | `handler.go:77,118` | 强制关机用 `exec.Command("cmd","/c","shutdown...")`，参数是常量，安全 | 可保留 |
| A-L3 | `config.go:73` | `config.json` 权限 `0644`，token 明文 | 收紧为 `0600`；配合 DPAPI |
| A-L4 | `main.go:106` | WS 用 `websocket.DefaultDialer`，未设 `Origin`/超时 | 显式设置握手超时与协议头 |
| A-L5 | `upgrade.go:91-103` | `isPEExecutable` 只查 `MZ` 两字节，比 Server 侧的完整 PE 头校验弱 | 统一为完整 PE 头校验 |

---

## 五、做得好的地方（确认无问题）

- ✅ **命令分发白名单**：`handleCommand` 用 `switch cmd.Action` 严格枚举（sysprep/reboot/shutdown/status/upgrade/file_push/hostmsg/sysinfo/uninstall），`default` 返回失败。**没有任何把 `params` 拼进 `cmd /c` 的字符串型命令执行**。
- ✅ **sysprep 参数做了 XML 转义**：`xmlEscape` 对 `& < > " '` 全部转义，防 XML 注入（`handler.go:317-324`）。
- ✅ **升级有备份与自动回滚**：`buildUpgradeBat` 先备份旧 exe，新服务起不来自动回滚，写日志。
- ✅ **升级文件有基本校验**：下载后校验大小（>1MB）与 PE 头。
- ✅ **计划任务自愈**：`ensureServiceRecovery` 保证意外退出后服务被拉起。
- ✅ **`exec.Command` 传参数组**：绝大多数调用用参数数组传递，天然免疫命令注入。

---

## 六、修复优先级

| 优先级 | 条目 | 理由 |
|--------|------|------|
| **P0** | A-H1 校验下载源同源 + 限制 file_push 目录 | 一改即堵住"诱导执行任意代码"的主路径 |
| **P0** | A-H2 删除/修正硬编码 AES 死代码 | 消除误导性加密 |
| **P1** | A-M2 去掉 sysprep 默认口令 | 弱口令 + 明文密码 |
| **P1** | A-M1 路径穿越 | 与 A-H1 同修 |
| **P2** | A-M3/A-L* | 规范加固，防未来回归 |
| **P2** | A-H2 token 传输方式（=H2-c） | 需三端协同，单独排期 |

---

## 七、落地方式（符合"不手动重装"约束）

所有 Agent 侧修复都可**通过现有推送升级通道**下发：

1. Server 端 `upgrade` 接口在推送时**附带 `sha256`**（配合 A-H1 第 3 条）。
2. 修改 `agent/*.go` → 编译新版本 → 通过管理页"批量升级"推送。
3. 升级脚本自带回滚，即使新版本有问题也会自动退回旧版。

**唯一注意**：A-H2 若改 token 传输方式（H2-c），需 **Server 先双模兼容 → 推送升级全部 Agent → 再关旧口子**，确保升级过程中设备不掉线。

---

*本报告基于静态代码走查。建议对 A-H1 的攻击路径（ARP 欺骗 + 恶意升级包）在测试环境做一次验证。*