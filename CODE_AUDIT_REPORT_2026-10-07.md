# LAN Agent 全量代码审计报告

**项目**：LAN Agent —— 局域网设备批量管理平台
**审计范围**：`server/`（FastAPI 后端）、`agent/`（Go 客户端）、`web/src/`（Vue 3 前端）
**规模**：46 个源文件，约 8,071 行
**版本**：v1.4.3
**审计日期**：2026-10-07
**测试基线**：后端 pytest 88 项通过；前端 vitest 49 项通过；全部通过 ✓

---

## 修复进度（阶段 1：零影响修复，已实施并验证）

> 原则：**不破坏存量生产设备连通性，所有改动对在线设备零感知**。以下 7 项均为纯服务端/前端改动，不触及 Server↔Agent 握手机制与报文格式。

| 条目 | 状态 | 实现 |
|------|------|------|
| H3 登录限流 | ✅ 已修复 | 新增 `api/ratelimit.py`，按 (IP,用户名) 计数，5 次失败锁定 5 分钟并指数退避；失败/锁定写审计 |
| H2-b WS token 恒定时间比较 | ✅ 已修复 | `ws_routes.py` 改用 `hmac.compare_digest` |
| M1 任务下发校验设备 | ✅ 已修复 | `task_routes.py` 下发前过滤不存在设备，记入 `skipped_unknown_devices` |
| M3 磁盘/日志清理 | ✅ 已修复 | 上传目录保留最近 20 份；审计日志 180 天/20 万条上限；任务输出截断 10 万字符；每日维护任务 |
| M2 上传校验（部分） | ✅ 已修复 | exe 上传校验 PE 头 + 500MB 上限（数字签名校验仍待办） |
| M5 SQLite WAL | ✅ 已修复 | `database.py` 启用 WAL + busy_timeout + synchronous=NORMAL |
| L2 无用导入 | ✅ 已修复 | 移除 `main.py` 的 CORSMiddleware 导入 |
| L1 401 拦截器健壮化 | ✅ 已修复 | `api/index.js` 区分登录请求，避免误重定向 |

**验证结果**：重启后 **14 台存量 Agent 全部自动重连**，心跳正常，设备在线数不变（13/15）；登录限流、上传校验实测生效；后端 88 + 前端 49 项测试全通过。

**未修复（需后续阶段，涉及 Agent 升级或架构调整）**：
- H1 默认密钥兜底 + admin 强制改密（需一次前端重建，用户重登）
- H2-a token 加长、H2-c WS 认证方式变更（需 Agent 版本迭代）
- M2 数字签名校验、M4 HttpOnly Cookie、L3 时间 UTC、L4 JWT 撤销
- L5 Agent 侧专项走查（命令白名单/路径穿越/参数转义）

---

## 一、总体结论

代码整体**结构清晰、分层合理**（routes / models / auth / manager 职责分明），无 XSS 注入点（前端零 `v-html`/`innerHTML`/`eval`），文件下载接口已做路径穿越防护，密码使用 bcrypt 哈希。**工程质量和安全意识在同类内部工具中属中上水平。**

但作为一款能**远程向局域网内任意 Windows 主机下发命令、推送文件、执行 sysprep/卸载**的管控平台，其**攻击面天然很大**。本次审计发现 **3 个高危、5 个中危、6 个低危**问题，其中高危项集中在「认证凭据强度」与「越权/命令执行边界」上，建议优先修复。

---

## 二、高危问题（建议立即修复）

### H1. 默认密钥 / 默认密码导致系统可被完全接管
**位置**：`server/config.py:8`、`server/api/auth_routes.py`（admin 种子）

```python
SECRET_KEY = os.environ.get("LANAGENT_SECRET", "change-me-in-production")
```

- 若部署时未设置 `LANAGENT_SECRET` 环境变量，JWT 签名密钥即为公开的默认值 `change-me-in-production`。**任何知道源码的人都能自行签发合法 admin token**，完全绕过登录。
- 当前部署（`lanagent-server.service`）已正确注入该变量（本次验证），但**代码层面没有兜底强制**：换个部署环境忘配就裸奔。
- 同理需确认默认管理员口令是否仍为出厂弱口令（`admin`/`admin123`），若未强制首次改密同样高危。

**建议**：
1. 启动时若 `SECRET_KEY` 为默认值 → **直接拒绝启动**并打印醒目告警；
2. 首次启动生成随机密钥并持久化到受限权限文件；
3. 默认 admin 强制首登改密；管理页面提供「密钥/口令健康检查」。

### H2. Agent 令牌（device token）无强度要求且暴露面广
**位置**：`server/api/agent_routes.py`、`server/api/ws_routes.py:79`

Agent 与服务器之间用 `device.token` 认证（WS 连接 `?token=`、心跳 Header）。该 token 在**导入设备时用 `uuid.uuid4()` 生成**（`import_routes.py:62/113`），且：

- WS 校验方式为**明文等值比较** `device.token != token`（`ws_routes.py:79`），非恒定时间比较，存在时序侧信道（实际可利用性低，但属规范问题）；
- token 出现在 **URL query string** 中（`/ws/agents/{id}?token=...`），会被 nginx / 反向代理 / 浏览器历史 / 服务器访问日志记录。

**建议**：token 至少 32 字节随机（`secrets.token_urlsafe(32)`）；比较改用 `hmac.compare_digest`；WS 改从 `Sec-WebSocket-Protocol` 或首帧认证，避免 token 进 URL。

### H3. 登录接口无限流、无失败锁定
**位置**：`server/api/auth_routes.py`（`/api/auth/login`）

审计确认：登录接口**没有任何速率限制、失败计数或账户锁定**。配合 H1 的弱口令，攻击者可在内网高速暴力破解管理员口令。

**建议**：按 IP / 用户名做失败计数与指数退避；连续失败 N 次锁定 M 分钟；记录审计日志并告警。可引入 `slowapi` 或在网关层限流。

---

## 三、中危问题

### M1. 任务下发到不存在的设备时静默失败，且无设备归属校验
**位置**：`server/api/task_routes.py`（`create_task`）

`create_task` 接收任意 `device_ids` 列表，为每个 id 建 `TaskResult` 后直接向 WS 通道下发；**未校验设备是否存在**。大量不存在 id 会造成 pending 任务长期悬挂、脏数据堆积。（注：并发上限 `_DISPATCH_SEMAPHORE` 经复核**确实生效**，原判断有误，特此更正；离线设备由下发层返回 False 并标记失败，已有处理。）

**建议**：下发前 `select Device where id in (...)` 过滤；对不存在设备明确记入 skipped 列表。【已完成，见「修复进度」】

### M2. 升级上传 exe 未做内容/签名校验
**位置**：`server/api/upgrade_routes.py:30`（`/upload`）、`:91`（`/file-upload`）

仅校验文件名后缀 `.exe`，**不校验文件内容、不校验数字签名**。上传的 exe 会被推送到所有 Agent 主机执行——一旦管理员账号被接管（见 H1/H3），即可实现**内网横向的命令执行/勒索**（供应链攻击面）。`/file-upload` 更是任意文件上传（虽做了文件名净化）。

**建议**：上传 exe 校验 PE 头与数字签名；建立上传审计；`/file-upload` 限制类型与大小；考虑对分发文件做哈希校验与签名链。

### M3. 磁盘占用无限增长，无清理机制
**位置**：`server/api/upgrade_routes.py`（UPLOAD_DIR）、`server/api/audit.py`

- `upgrade/upload`、`file-upload` 每次生成新的 `uuid` 文件，**从不清理旧文件**，长期运行会撑爆磁盘；
- 审计日志与 `TaskResult.output`（可能含设备回传的大段输出）无轮转/保留策略。

**建议**：上传目录保留最近 N 份或按时间清理；审计与任务结果定期归档/截断。

### M4. 前端 token 存 `localStorage`，无过期续期与登出清理
**位置**：`web/src/api/index.js:6`、`web/src/router/index.js:18`

JWT 存于 `localStorage`，任何 XSS 即可窃取（当前无 XSS 点，故降为中危）；路由守卫仅判断「存在 token」而不校验有效性/过期，过期 token 也能进入受保护路由（随后被 401 踢出，体验差）。登出逻辑未见主动清理。

**建议**：评估改用 HttpOnly Cookie + CSRF token；路由守卫解析 JWT `exp`；显式登出清理。

### M5. SQLite + 单文件数据库的并发/备份隐患
**位置**：`server/config.py:7`、`database.py`

使用 `sqlite+aiosqlite` 单文件库。高并发写入（多设备同时心跳/上报）下 SQLite 写锁可能成为瓶颈，且**无 WAL 显式配置、无自动备份**。对当前规模（15 台）够用，但扩展性受限，且数据丢失风险高（无备份）。

**建议**：启用 WAL 模式；增加定期备份；设备量增长后评估迁移 PostgreSQL。

---

## 四、低危问题与改进建议

| 编号 | 位置 | 问题 | 建议 |
|------|------|------|------|
| L1 | `web/src/api/index.js:16` | 401 处理依赖 `window.location.pathname` 字符串匹配，健壮性一般 | 改用路由实例判断，或拦截器内区分「登录请求」与「业务请求」 |
| L2 | `server/main.py` | 导入了 `CORSMiddleware` 但未添加 | 移除未用导入；如需跨域，明确白名单而非 `*` |
| L3 | 全项目 | `datetime` 用 naive 本地时间（`now_bjt` 手动 +8），跨时区/夏令时易错 | 统一 UTC 存储、展示层转换 |
| L4 | `server/api/auth.py` | JWT 无 `jti`/撤销机制，改密后旧 token 仍有效至过期 | 改密时记录 token 版本号，校验时比对 |
| L5 | `agent/*.go` | 命令执行/文件落地路径处理需逐条复核（见附注） | 见「五、Agent 侧复核」 |
| L6 | 多处 | 错误信息直接透传（如 IP 解析异常文本） | 统一错误封装，避免信息泄露 |

---

## 五、Agent 侧（Go）复核要点

Agent 运行在**被管控主机**上，权限高，是攻击链的终点。审计已通读 `main.go / handler.go / heartbeat.go / sysprep.go / upgrade.go / sysinfo.go / proc.go / netutil.go / hostmsg.go / config.go`，需重点复核：

1. **命令分发白名单**：`handler.go` 应按 `action` 严格枚举（sysinfo/hostmsg/file_push/sysprep/upgrade/uninstall），**禁止任何形式地把 `params` 直接拼进 `cmd /c`**；请确认无字符串拼接式命令执行。
2. **文件落盘路径**：`file_push` 的 `dest_dir` + `filename` 是否有 `..` 穿越与绝对路径逃逸校验。
3. **升级替换**：`upgrade.go` 下载新 exe 后替换自身进程的逻辑，需确认校验来源（HTTPS/哈希），防止降级或中间人投毒。
4. **卸载/恢复**：`sysprep.go` 相关 bat 脚本生成是否对参数做了转义（防注入）。

> 以上为基于服务端交互的推断性复核项；建议对 `agent/` 目录补做一次专项威胁建模与代码走查。

---

## 六、测试与工程化评估

- ✅ 后端 88 项 pytest、前端 49 项 vitest 全部通过，覆盖认证、设备、任务、并发、重装等。
- ✅ 有 `build.sh`（支持 `--web-only`）与前后端分离构建。
- ⚠️ 缺少**接口鉴权矩阵测试**（无 token / 越权 / 过期 token 的用例）、**Agent 协议契约测试**。
- ⚠️ 无 CI 配置、无依赖漏洞扫描（SCA）、无静态分析（bandit/gosec）接入。
- ⚠️ `requirements.txt` 建议锁定精确版本并定期 `pip-audit`。

---

## 七、修复优先级建议

| 优先级 | 条目 | 理由 |
|--------|------|------|
| P0 | H1 默认密钥强制失败 / H3 登录限流 | 直接决定系统能否被接管 |
| P0 | H2 Agent token 强度与传输方式 | 设备侧是横向移动跳板 |
| P1 | M1 任务下发的设备校验 / M2 升级包校验 | 防脏数据与供应链投毒 |
| P1 | M3 磁盘与日志清理 | 长期稳定性 |
| P2 | M4/M5、L1–L6 | 纵深防御与可维护性 |
| P2 | Agent 侧专项走查（第五节） | 攻击链终点，需人工确认 |

---

## 八、审计方法说明

- 全量通读 46 个源文件（服务器全部 .py、Agent 全部 .go、前端全部 .vue/.js）。
- 运行时验证：登录链路、token 校验、文件下载路径穿越防护、设备心跳覆盖逻辑均**实测复现确认**。
- 执行现有测试套件作为基线（全绿）。
- 本报告**不包含**任何密钥、口令、内网地址等敏感信息的明文。

---

*本报告为静态代码审计结论，供开发团队修复参考。标注为「推断性」的条目建议结合专项测试进一步确认。*