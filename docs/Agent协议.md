# Agent 协议

> 版本 **v1.7.6** · 相关文档：[项目文档](项目文档.md) · [安全说明](安全说明.md) · [排障手册](排障手册.md)

本文档是管理端与 Agent 之间的**逐字段契约**。改任何一侧之前，先读完这篇。

---

## 1. 两条通道

| 通道 | 协议 | 方向 | 用途 |
| --- | --- | --- | --- |
| 心跳 | HTTP POST | Agent → 管理端 | 证明存活、上报状态、领取令牌与阈值 |
| 长连接 | WebSocket | 双向 | 接收指令、回传结果 |

分工必须分清，否则排障会走错方向：

- **"在线"** 由心跳决定。心跳停 15 秒，管理端标记离线。
- **"能收指令"** 由长连接决定。心跳正常但长连接未建立时，设备看起来在线，指令永远没有响应。

两者都显式禁用系统代理（`Proxy: nil`）。这一点是硬约束，不是风格问题。

---

## 2. 心跳

### 请求

```
POST /api/agents/heartbeat
Content-Type: application/json
Authorization: Bearer <agent_token>
X-Enroll-Secret: <enroll_secret>
```

`Authorization` 与 `X-Enroll-Secret` 都可缺省，但缺哪个就限制了哪些能力（见第 2.3 节）。

### 请求体

```json
{
  "device_id": "c1f0…-uuid 或 null",
  "hostname": "SAMPLE-PC-01",
  "os": "Windows 11 家庭版 中文版",
  "ip": "192.168.2.98",
  "rearm_count": 3,
  "status": "online",
  "agent_version": "1.7.6",
  "timestamp": 1763000000,
  "sysinfo": {
    "cpu_model": "12th Gen Intel(R) Core(TM) i5-12400",
    "cpu_cores": 12,
    "memory_total_mb": 16384,
    "disk_total_gb": 512,
    "disk_free_gb": 301,
    "os_version": "10.0.22631",
    "serial_number": "PF3XXXXX",
    "mac_address": "AA:BB:CC:DD:EE:FF",
    "manufacturer": "LENOVO",
    "model": "GeekPro-17IRB"
  }
}
```

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `device_id` | 否 | 首次心跳为空；之后由响应回填并持久化 |
| `hostname` | 是 | 主机名；管理端可锁定（`hostname_locked`），锁定后不被覆盖 |
| `os` | 是 | 系统描述 |
| `ip` | 是 | 本机局域网 IP，**可能为空字符串**（未联网，见第 5 节） |
| `rearm_count` | 是 | Sysprep 剩余次数，`-1` 表示取不到 |
| `status` | 是 | `online` / 其它 |
| `agent_version` | 是 | 编译注入的 `buildVersion` |
| `timestamp` | 是 | Unix 秒 |
| `sysinfo` | 否 | 硬件信息，缺省时不清空已有记录 |

### 响应

```json
{
  "device_id": "26",
  "status": "updated",
  "offline_selfdestruct_hours": 0
}
```

| 字段 | 说明 |
| --- | --- |
| `device_id` | 当前设备主键。**JSON 里可能是数字也可能是字符串**，Agent 必须两种都接 |
| `status` | `created` / `updated` / `re-enrolled` / `deleted` |
| `offline_selfdestruct_hours` | 离线自毁阈值，`0` = 关闭；**字段可能缺席** |

### 2.1 状态含义

| status | 含义 | Agent 应做 |
| --- | --- | --- |
| `created` | 新设备首次注册成功 | 持久化 `device_id` |
| `updated` | 已有设备正常刷新 | 无特殊动作 |
| `re-enrolled` | 令牌轮换后接管成功（重装场景） | **同步新 token**，用于重建长连接认证头 |
| `deleted` | 记录处于软删除态且身份未验证 | 通常伴随无 token，不要继续使用旧凭据 |

### 2.2 device_id 的类型陷阱

这是历史上最隐蔽的一个 bug，值得单独说明。

服务端返回的 `device_id` 是数据库自增主键，在 JSON 里是**数字**。早期 Agent 用 `string` 接收：

```go
var result struct {
    DeviceID string `json:"device_id"`   // ← 收到数字，Unmarshal 报错
}
```

`json.Unmarshal` 遇到类型不匹配会返回错误，而当时的代码是 `if err == nil { ... }`，错误被静默吞掉，`DeviceID` 一直是空字符串。表现就是：**心跳 HTTP 200、看起来完全正常，但长连接一直 `waiting for device_id`，指令永远收不到。**

正确做法是用 `interface{}` 接收再归一化：

```go
func normalizeDeviceID(value interface{}) string {
    switch v := value.(type) {
    case nil:     return ""
    case string:  return strings.TrimSpace(v)
    case float64: // JSON 数字统一解析为 float64
        if v == float64(int64(v)) { return strconv.FormatInt(int64(v), 10) }
        return strconv.FormatFloat(v, 'f', -1, 64)
    case json.Number: return v.String()
    default:      return ""
    }
}
```

同时 `offline_selfdestruct_hours` 用 `*int` 接收：旧代码用 `int` 配 `if result.OfflineHours >= 0`，字段缺席时零值 0 也满足条件，会把阈值错误地重置为 0。用指针加 `!= nil` 判断才能区分"没给"和"给了 0"。

### 2.3 设备识别顺序

```
① 有 device_id ？
     └ 精确匹配 Device.id ──命中→ 进入身份校验
② 未命中且有 ip ？
     └ 按 ip 匹配，取 last_heartbeat 最新的一条 ──命中→ 进入身份校验
③ 都没命中
     └ X-Enroll-Secret 正确？─是→ 新建记录，返回 created
                              └否→ 401 Enrollment secret required
```

第 ② 步是为了覆盖"设备重装后没带 device_id"的场景——旧记录还在，靠 IP 找回。

### 2.4 身份校验与接管

命中记录后按令牌判定：

| 条件 | 结果 |
| --- | --- |
| 记录无 token，或 token 一致 | 正常刷新 → `updated` |
| token 不一致，且（记录已软删除 **或** `X-Enroll-Secret` 正确） | 接管并轮换 token → `re-enrolled` |
| token 不一致，且记录已软删除，且无有效密钥 | `deleted` |
| 其余 token 不一致 | `401 Invalid agent token` |

"软删除"指 `deleted_at` 非空。这样做的目的是：被管理者删除的设备，重装后能自动恢复上线，不需要人工干预；同时不会被不知密钥的人抢占。

### 2.5 并发保护

Agent 启动瞬间可能并发发出多个心跳。注册/接管过程串行化（按事件循环持有的 `asyncio.Lock`），避免"都查不到记录 → 都新建"的竞态，否则同一台设备会产生多条记录。

锁按事件循环惰性创建，不放在模块级别——模块级 `asyncio.Lock` 会绑定到首次使用的事件循环，在测试或多循环环境抛 `bound to a different event loop`。

### 2.6 Sysprep 后的恢复

```
POST /api/agents/recover
Authorization: Bearer <agent_token>

{ "device_id": "…", "hostname": "SAMPLE-PC-01" }
```

Sysprep 会重置系统状态，Agent 用它把设备重新置为在线。必须持有该设备令牌，否则 `401`；设备不存在则 `404`。

---

## 3. 长连接

### 连接

```
ws://<server>/ws/agents/<device_id>?token=<agent_token>
Authorization: Bearer <agent_token>
```

握手校验令牌，失败以关闭码 `4001` 断开。

### 指令下发

管理端在同一条连接上推送：

```json
{ "id": "uuid", "action": "shutdown", "params": {} }
```

### 结果回传

Agent 执行后写回同一条连接：

```json
{ "id": "uuid", "status": "success", "output": "…" }
```

- 写操作带 **10 秒写超时**，防止对端不读时永久阻塞，把整个读循环卡死；
- `json.Marshal` 失败会记日志并跳过该条，不会中断循环。

### 3.1 action 必须小写

管理端下发时统一转小写，Agent 侧 `handleCommand` 按小写名匹配：

```go
switch action {
case "sysprep":        …
case "sysprep_reboot": …
case "reset_pwd":      …
case "shutdown":       …
case "reboot":         …
case "status":         …
case "upgrade":        …
case "file_push":      …
case "hostmsg":        …
case "sysinfo":        …
case "uninstall":      …
default:  // 回报 unknown action
}
```

历史上出现过下发大写 `SHUTDOWN` 导致全部命中 `default`，现象是"指令下发成功但设备毫无反应"。

### 3.2 令牌与 device_id 的重建

长连接每轮重连都**重新读配置并用最新 token 重建 Authorization 头**，不能沿用启动时的值。原因：re-enroll 之后 token 已轮换，旧 token 会让长连接持续 401。

### 3.3 内存配置副本

心跳回填 `device_id` 与轮换 `token` 时，同时写磁盘 `config.json` 与内存 `liveCfg`。

安装目录不可写时（`C:\Program Files` 权限异常、杀软拦截），磁盘写会失败。此时长连接回退到内存值继续，不会卡死。`liveCfg` 的读写都用副本：

```go
func setLiveConfig(cfg *Config) {
    snapshot := *cfg
    liveCfgMu.Lock(); liveCfg = &snapshot; liveCfgMu.Unlock()
}
func getLiveConfig() *Config {
    liveCfgMu.Lock(); defer liveCfgMu.Unlock()
    if liveCfg == nil { return nil }
    snapshot := *liveCfg; return &snapshot
}
```

存指针会让调用方在无锁状态下改动共享对象，产生数据竞争。

---

## 4. 指令参数

| action | params | 说明 |
| --- | --- | --- |
| `sysprep` | — | Sysprep 并关机 |
| `sysprep_reboot` | — | Sysprep 并重启 |
| `shutdown` / `reboot` | — | 直接关机 / 重启 |
| `reset_pwd` | — | 清空当前登录账户密码 |
| `status` | — | 回传主机名、IP、Rearm 次数、版本 |
| `sysinfo` | — | 采集硬件信息回传 |
| `hostmsg` | `title` `body` `timeout` | 桌面弹窗，`timeout` 秒后自动关闭 |
| `file_push` | `url` `filename` `dest` | 下载文件到指定目录 |
| `upgrade` | `url` `version` | 拉取并替换 Agent |
| `uninstall` | — | 自卸载 |

### 4.1 status 的返回值

用 `json.Marshal` 构造，不手工拼字符串：

```go
statusPayload, err := json.Marshal(map[string]interface{}{
    "hostname":      getHostname(),
    "ip":            getLocalIP(),
    "rearm_count":   rearm,
    "agent_version": getAgentVersion(),
})
```

主机名可能含引号或反斜杠，手工 `fmt.Sprintf` 拼出来的是非法 JSON，管理端解析失败，看起来像设备无响应。

### 4.2 下载来源校验

`upgrade` 与 `file_push` 都必须先过 `validateServerURL(cfg, url)`：地址必须指向当前管理服务器（同主机同端口、`http`/`https`）。这条校验挡住"冒充服务器诱导下载任意程序"的主路径。

### 4.3 路径约束

`file_push` 的目标路径经 `safeJoinUnder(destDir, filename)` 处理，拼接结果必须仍在 `destDir` 之下。含 `..` 或绝对路径的 `filename` 直接拒绝。

---

## 5. IP 上报

Agent 枚举所有 IPv4 地址后按业务优先级排序：

| 网段 | 优先级 | 说明 |
| --- | --- | --- |
| `192.168.x.x` | 10 | 门店最常见，最优 |
| `10.x.x.x` | 20 | 次优 |
| 其它（含公网） | 30 | 可用 |
| `172.16–31.x.x` | 50 | 常见虚拟网卡（Docker/WSL/Hyper-V） |
| 回环 / 169.254 / 0.x | ≥90 | 无效 |

最终取排序后的第一个；若第一个的优先级 ≥90（或一个都没有），**上报空字符串**，不写 `127.0.0.1`。

管理端收到空 IP 时保留上一次的有效 IP，等设备恢复联网后自动回推正确地址。多网卡机器（有线 + 无线 + 虚拟网卡 + 蓝牙）的枚举顺序不稳定，不排序会随机上报虚拟网卡地址，列表里就会看到设备 IP 跳来跳去。

---

## 6. 本地日志

日志文件：`C:\Program Files\LanAgent\logs\agent.log`

策略：

| 规则 | 值 |
| --- | --- |
| 保留窗口 | 最近 10 分钟 |
| 断开后 | 停写业务日志，只记一条断开时间 |
| 恢复后 | 重新开始记录 |
| 单行上限 | 8 KB |

时间解析取每行前 **19 个字符**（`2006/01/02 15:04:05`）。不要取 20——第 20 个字符是分隔空格，按 20 切会永远解析失败，日志窗口永不生效。

断开标记必须在**释放锁之后**写。持锁调用 `log.Output` 会重新进入 `logWrite`，形成死锁——这是真实发生过的线上问题：

```go
func logMarkDisconnected(reason string) {
    agentLog.mu.Lock()
    if agentLog.off { agentLog.mu.Unlock(); return }
    agentLog.off = true
    if agentLog.f != nil {
        line := logLinePrefix() + "[log] 与管理端断开 " + time.Now().Format("2006/01/02 15:04:05")
        if reason = strings.TrimSpace(reason); reason != "" { line += "，原因: " + reason }
        appendLogLocked([]byte(line + "\n"))
        _ = agentLog.f.Sync()
    }
    agentLog.mu.Unlock()          // 先解锁
}
```

---

## 7. 现场判读

安装后按顺序看到这两行，说明链路完全通了：

```
[heartbeat] saved device_id=26 (action=ok code=0)
[ws] connected
```

| 日志 | 含义 | 下一步 |
| --- | --- | --- |
| `[heartbeat] saved device_id=…` | 心跳注册成功，主键已回填 | 等长连接 |
| `[ws] connected` | 长连接建立，可收指令 | 正常 |
| `[ws] waiting for device_id from heartbeat...` | 心跳没回填成功 | 看心跳日志与安装目录权限 |
| `[heartbeat] 解析响应失败` | 响应结构与预期不符 | 核对版本是否匹配 |
| `[ws] connect failed` | 长连接握手失败 | 查 token、端口、代理 |
| 日志只有一行断开时间 | 已与管理端断开，后续静默 | 网络或管理端异常 |

更完整的现象对照见 [排障手册](排障手册.md)。
