# 批量关机助手 · LAN Agent

> 当前版本 **v1.7.6** · [项目文档](docs/项目文档.md) · [MIT License](LICENSE)

面向 **Windows 11 家庭版** 的局域网批量管理系统。在一台 Linux/Windows 服务器上运行管理端，在门店的每一台样机上安装轻量 Agent，即可远程执行 Sysprep 出样、关机、重启、弹窗通知、硬件信息采集、文件分发与版本升级。

系统按 **管理端 + 被管理端** 两条线组织：管理端是唯一的控制入口，被管理端只做一件事——连回管理端、领指令、执行、回报结果。

---

## 目录

- [核心能力](#核心能力)
- [系统架构](#系统架构)
- [快速开始](#快速开始)
- [被管理端安装](#被管理端安装)
- [指令清单](#指令清单)
- [Agent 与管理端通信契约](#agent-与管理端通信契约)
- [本地状态与日志](#本地状态与日志)
- [API 概览](#api-概览)
- [安全设计](#安全设计)
- [开发与构建](#开发与构建)
- [排障索引](#排障索引)
- [文档地图](#文档地图)

---

## 核心能力

| 能力 | 说明 |
| --- | --- |
| Sysprep 出样 | 执行系统准备工具并关机/重启，不消耗 Rearm 次数 |
| 远程关机 / 重启 | 单台或批量，结果逐台回报 |
| 弹窗通知 | 向目标机桌面弹出提示窗口，可设标题、正文与超时 |
| 硬件信息采集 | CPU、内存、磁盘、系统版本、序列号、MAC、厂商型号 |
| 文件分发 | 推送文件到被管理端指定目录 |
| 密码修复 | `reset_pwd` 清空当前登录账户密码，修复 Sysprep 后被改写的凭据 |
| 推送升级 | 管理端一键推送新版 Agent，无需逐台操作 |
| 设备自动注册 | 安装后自动上线，双向心跳；重装后自动接管原记录 |
| 离线检测 | 15 秒未心跳标记离线，列表在线在前、离线置底 |
| IP 自动纠正 | 设备未联网时保留原 IP，恢复联网后自动回推正确地址 |
| 操作审计 | 所有关键操作完整留痕 |
| 远程卸载 | 删除设备时推送卸载指令，清理服务、进程与安装目录 |
| 离线自毁 | 可选能力，默认关闭（详见下文） |

---

## 系统架构

```
┌─────────────────────────────────────────┐
│  管理端 (FastAPI + SQLite + Vue3 SPA)   │
│  单端口对外，REST + WebSocket            │
└──────────────┬──────────────────────────┘
               │
      ① HTTP  心跳 POST /api/agents/heartbeat（15s 间隔）
      ② WS    长连接 /ws/agents/{device_id}
               │
┌──────────────┴──────────────────────────┐
│  被管理端 (Go 单文件 Agent，Windows 服务) │
│  Windows 11 家庭版 ×N                    │
└─────────────────────────────────────────┘
```

两条通道分工明确：

- **心跳（HTTP）**：证明存活、上报主机名/IP/版本/Rearm 次数、领取管理端下发的令牌与离线自毁阈值。
- **长连接（WebSocket）**：实时接收指令、回传执行结果。设备"在线"指心跳通；能否收到指令看长连接。

**两条通道都显式禁用系统代理。** 设备上若配置了 `HTTP_PROXY` / `HTTPS_PROXY`，心跳会经代理、长连接走直连，现象就是"设备显示在线，但指令永远没有响应"。Agent 内置的 HTTP 客户端与 WebSocket 拨号器均设为 `Proxy: nil`，保证两条通道走同一条路径。

---

## 快速开始

### 1. 部署管理端

```bash
git clone https://github.com/zifeng-chen/batch-shutdown-assistant.git
cd batch-shutdown-assistant

# 后端依赖
cd server
python3 -m venv .venv && . .venv/bin/activate
pip install -r requirements.txt

# 前端构建（需要 Node.js 18+）
cd ../web && npm install && npm run build   # 产物输出到 server/static/

# 启动
cd ../server
uvicorn main:app --host 0.0.0.0 --port 8900
```

访问 `http://服务器IP:8900`。默认账号 `admin / admin123`。

> ⚠️ `admin123` 与默认 `change-me-in-production` 只是占位值，**严禁用于生产环境**。部署前请完成下方《部署安全清单》。

### 2. 部署安全清单

生产环境启动前逐项确认：

| 项 | 做法 |
| --- | --- |
| 管理员密码 | 登录后立即修改，或首次启动前清库后重建 |
| `SECRET_KEY` | 设置环境变量 `LANAGENT_SECRET`（未改时启动日志会打警告） |
| 注册密钥 | 设置 `LANAGENT_ENROLL_SECRET`；未设置时由 `SECRET_KEY` 派生 |
| 数据目录 | 设置 `LANAGENT_DATA_DIR` 指向代码目录之外，避免升级覆盖数据 |
| 网络暴露 | 仅在内网提供；不要直接暴露到公网 |
| 安装包 | `deploy.conf` 内含注册密钥，禁止外传 |

### 3. 生产运行（systemd）

```ini
# /etc/systemd/system/lanagent-server.service
[Unit]
Description=LAN Agent Manager
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/lanagent/server
Environment=LANAGENT_DATA_DIR=/var/lib/lanagent
Environment=LANAGENT_SECRET=请替换为随机长字符串
ExecStart=/opt/lanagent/server/.venv/bin/python -m uvicorn main:app --host 0.0.0.0 --port 8900
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now lanagent-server
sudo systemctl status lanagent-server
curl http://127.0.0.1:8900/api/health    # {"status":"ok"}
```

---

## 被管理端安装

安装包由管理端**现场生成**，不是仓库里的静态文件：登录管理端 → 设备页 → 下载安装包，或直接请求 `/api/deploy/package`。

包内包含：

| 文件 | 作用 |
| --- | --- |
| `LanAgent.exe` | Agent 主程序 |
| `deploy.conf` | 服务器地址 + 令牌 + 注册密钥 |
| `reinstall.bat` | **安装/覆盖安装入口**，右键以管理员身份运行 |
| `uninstall.bat` | 卸载入口 |
| `reset-password.bat` | 清空当前账户密码（Sysprep 后登录失败的修复手段） |
| 安装说明.txt | 同内容离线说明 |

### 安装步骤

1. 解压到目标电脑任意位置（`deploy.conf` 必须与 `bat` 同目录）；
2. 右键 `reinstall.bat` → 以管理员身份运行；
3. 脚本依次输出：管理员权限 → 安装包文件 → 停止旧服务 → 清理旧文件 → 复制文件 → 安装启动；
4. 看到「安装成功！服务正在运行」即完成；
5. 管理端约 10 秒内可见设备上线。

> **不要直接双击 `LanAgent.exe`。** 它需要 `config.json`/`deploy.conf` 才能启动，缺少配置会立刻退出，表现为窗口一闪而过。

### 安装路径

```
C:\Program Files\LanAgent\          ← 安装目录（1.5.0 起）
├── LanAgent.exe
├── config.json                      ← 运行期配置（device_id / token）
├── deploy.conf                      ← 安装包带入的初始配置
├── logs\agent.log                   ← 运行日志
└── data\                            ← 设备本地数据
    └── files\                       ← 文件分发落地目录
```

**旧路径 `C:\Windows\LanAgent`**（1.4.9 及更早）由安装脚本与 Agent 自动识别清理：先迁移 `logs` 与 `data`（目标已存在则保留不动），再删除旧目录；更早的 `C:\LanAgent` 数据目录由 `migrateOldData()` 处理。

### 覆盖安装

已装旧版本时无需手动卸载，直接右键 `reinstall.bat` 管理员运行即可。脚本会：

- 停止并删除旧的服务注册；
- 结束残留的 `LanAgent.exe` 进程（**按完整路径匹配**，不会误杀解压目录里正在运行的同名程序）；
- 删除旧目录下的 `LanAgent.exe` / `config.json` / `deploy.conf`；
- 复制新版本、重建并启动服务；
- 保留 `logs` 与 `data`，历史日志与设备数据不丢；
- 顺带清理更早版本的遗留路径。

### 安装日志

脚本输出的每一行都同步写入安装包所在目录下的 `安装日志.txt`。终端窗口意外关闭也不影响，可以直接打开查看卡在哪一步。若该目录不可写，自动回退到 `%TEMP%`。

### 卸载

右键 `uninstall.bat` → 管理员运行。或在管理端删除设备，会自动推送 `uninstall` 指令。

---

## 指令清单

**指令名一律小写。** 管理端下发时统一转小写，Agent 侧 `handleCommand` 按小写名匹配；大写形式会命中默认分支并回报 `unknown action`。

| 指令 | 作用 | 主要参数 |
| --- | --- | --- |
| `sysprep` | 执行 Sysprep 并关机 | — |
| `sysprep_reboot` | 执行 Sysprep 并重启 | — |
| `shutdown` | 关机 | — |
| `reboot` | 重启 | — |
| `reset_pwd` | 清空当前登录账户密码 | — |
| `status` | 回传主机名、IP、Rearm 次数、Agent 版本 | — |
| `sysinfo` | 采集并回传硬件/系统信息 | — |
| `hostmsg` | 桌面弹窗 | `title` `body` `timeout` |
| `file_push` | 推送文件到指定目录 | `url` `dest` |
| `upgrade` | 拉取并替换为新版 Agent | `url` `version` |
| `uninstall` | 自卸载 | — |

`status` 的返回值用 `json.Marshal` 构造，不是手工拼字符串——主机名可能含引号或反斜杠，手拼会让管理端解析失败，表现为"设备无响应"。

---

## Agent 与管理端通信契约

### 心跳

```
POST /api/agents/heartbeat
Authorization: Bearer <agent_token>
X-Enroll-Secret: <enroll_secret>      ← 首次注册/重装接管时必需

{
  "device_id": "…或 null",
  "hostname": "SAMPLE-PC-01",
  "os": "Windows 11 家庭版 中文版",
  "ip": "192.168.2.98",
  "rearm_count": 3,
  "status": "online",
  "agent_version": "1.7.6",
  "timestamp": 1763000000,
  "sysinfo": { … }
}
```

响应：

```json
{
  "device_id": "设备主键",
  "status": "created | updated | re-enrolled | deleted",
  "offline_selfdestruct_hours": 0
}
```

### 设备识别与令牌接管顺序

1. 带 `device_id` → 精确匹配记录；
2. 未命中且带 `ip` → 按 IP 匹配（取最后心跳最新的一条，覆盖"重装但没带 device_id"的情况）；
3. 令牌一致 → 正常刷新；
4. 令牌不一致，但记录已软删除 **或** `X-Enroll-Secret` 正确 → 接管并轮换令牌，返回 `re-enrolled`；
5. 已软删除且身份无法验证 → 返回 `deleted`；
6. 其余令牌不一致 → `401 Invalid agent token`；
7. 无匹配记录 → 必须 `X-Enroll-Secret` 正确，否则 `401 Enrollment secret required`；通过则新建记录，返回 `created`。

注册临界区加了锁：Agent 启动瞬间并发心跳不会产生重复设备记录。

### 长连接

```
ws://<server>/ws/agents/<device_id>?token=<agent_token>
Authorization: Bearer <agent_token>
```

服务端下发指令，Agent 执行后以 JSON 写回同一条连接。写操作带 10 秒写超时，避免对端不读时把整个读循环阻塞。

### 令牌与 device_id 的持久化

心跳回填的 `device_id` 与轮换后的 `token` 会同时写入两处：

- **磁盘** `config.json`；
- **内存** `liveCfg`（受 `RWMutex` 保护，读写均为副本）。

磁盘写失败时（安装目录不可写）长连接改用内存值继续，不会卡在等 `device_id` 的状态。长连接每轮重连都重新读配置并用最新 token 重建 Authorization 头——re-enroll 之后沿用旧 token 会让长连接一直 401。

---

## 本地状态与日志

### 日志

运行日志固定在 `C:\Program Files\LanAgent\logs\agent.log`。Windows 服务模式下没有控制台，不落盘就等于没有任何现场证据，因此日志文件在服务启动的第一步就打开，早于任何其它初始化。

日志策略：

| 规则 | 行为 |
| --- | --- |
| 保留窗口 | 只保留最近 **10 分钟**的日志 |
| 断开后 | 停止写入业务日志，只在断开那一刻写一条带时间的断开记录 |
| 恢复后 | 重新开始记录 |
| 单行上限 | 8 KB，超长截断，避免异常输出撑爆文件 |

滚动与截断由 `logmgr.go` 负责。这个策略是为了让现场回传的日志始终是"最近发生了什么"，而不是一个需要翻很久的历史文件。

### 离线与自毁

- 离线判定：管理端每 10 秒巡检一次，`last_heartbeat` 超过 15 秒未更新即标记离线。
- 离线自毁：`OFFLINE_SELF_DESTRUCT_HOURS`，**默认 0 = 关闭**。开启后，Agent 连续无法连接管理端超过该时长会自行卸载。默认关闭是为了避免管理端宕机或网络中断时误伤整网设备。

### IP 上报规则

Agent 枚举本机所有 IPv4 地址并按业务优先级排序：

```
192.168.x.x  >  10.x.x.x  >  172.16–31.x.x  >  其它
回环 / 169.254 链路本地 / 0.x  →  视为无效（不参与上报）
```

多网卡机器（有线 + 无线 + 虚拟网卡 + 蓝牙）的枚举顺序不稳定，必须优先选真正代表本机局域网位置的地址。若当前**没有任何有效地址**（典型场景：机器未联网），Agent 上报空 IP，管理端保留上一次的有效 IP，等设备恢复联网后自动回推正确地址。

---

## API 概览

| 分组 | 路径 | 说明 |
| --- | --- | --- |
| 认证 | `POST /api/auth/login`、`GET /api/auth/me`、`POST /api/auth/change-password` | 登录与改密 |
| 设备 | `GET/POST /api/devices`、`PUT/DELETE /api/devices/{id}`、`POST /api/devices/batch-delete`、`GET /api/devices/stats` | 设备增删改查与统计 |
| 导入 | `POST /api/devices/import/csv`、`POST /api/devices/import/ip-range` | CSV / IP 段批量导入 |
| 任务 | `POST /api/tasks`、`GET /api/tasks`、`GET /api/tasks/{id}`、`POST /api/tasks/{id}/cancel` | 指令下发与查询 |
| Agent | `POST /api/agents/heartbeat`、`POST /api/agents/recover` | 心跳与 Sysprep 后恢复 |
| 部署 | `GET /api/deploy/server-url`、`POST /api/deploy/generate-token`、`POST /api/deploy/package` | 安装包生成 |
| 升级 | `POST /api/upgrade/upload`、`GET /api/upgrade/latest`、`GET /api/upgrade/latest-download`、`POST /api/upgrade/file-upload` | Agent 版本分发 |
| 审计 | `GET /api/audit` | 操作审计 |
| 健康 | `GET /api/health` | 返回 `{"status":"ok"}` |
| WebSocket | `/ws/agents/{device_id}`、`/ws/tasks/{task_id}` | 指令通道与任务进度 |

设备列表按 IP 数值排序（跨网段正确，`192.168.2.9` 排在 `192.168.2.10` 之前），非法或缺失 IP 排到末尾并按主机名稳定次级排序；界面上在线设备在上、离线设备置底。

---

## 安全设计

| 层面 | 措施 |
| --- | --- |
| 管理端认证 | JWT，密码 bcrypt 哈希 |
| Agent 身份 | 每设备独立 token；首次注册需预共享 `enroll_secret` |
| 重装接管 | 令牌不一致时只有持有正确注册密钥、或记录已软删除，才允许接管 |
| 命令通道 | 长连接校验 Bearer token，失败关闭码 4001 |
| 文件落地 | `safeJoinUnder` 限制目标路径不得逃出允许的根目录 |
| 静态资源 | SPA 兜底路由校验 `commonpath`，阻止 `../` 逃逸 |
| 进程操作 | 只结束可执行文件位于指定目录下的进程，不按映像名全局匹配 |
| 审计 | 关键操作写入审计表，超期自动清理 |

---

## 开发与构建

### 仓库结构

```
batch-shutdown-assistant/
├── agent/                  Go Agent（Windows amd64 单文件）
│   ├── main.go             服务框架、长连接、覆盖安装、迁移
│   ├── heartbeat.go        心跳、版本号、IP 取值
│   ├── handler.go          指令分发
│   ├── logmgr.go           日志窗口与离线静默
│   ├── netutil.go          网卡枚举与 IP 优先级
│   ├── upgrade.go          自升级
│   ├── selfdestruct.go     离线自毁
│   ├── sysprep.go          Sysprep 执行
│   ├── sysinfo.go          硬件信息采集
│   ├── hostmsg.go          桌面弹窗
│   ├── password.go         密码清空
│   └── config.go           路径常量与配置读写
├── server/                 FastAPI 管理端
│   ├── main.py             应用装配、离线巡检、维护循环
│   ├── config.py           版本号唯一来源
│   ├── api/                各业务路由
│   ├── models/models.py    设备 / 任务 / 审计
│   └── static/             前端构建产物
├── web/                    Vue3 + Element Plus 前端
├── installer/              安装/卸载 bat 脚本模板
├── docs/                   项目文档与更新日志
└── build.sh                一键构建
```

### 一键构建

```bash
./build.sh                  # 全量：同步版本号 + 跑测试 + 编译 Agent + 构建前端
./build.sh --agent-only     # 只编译 Agent
./build.sh --web-only       # 只构建前端
./build.sh --skip-tests     # 跳过测试
./build.sh --version 1.7.7  # 指定版本并回写 config.py
```

**版本号只有一个来源：`server/config.py` 的 `APP_VERSION`。** 构建脚本会把它同步到 `web/package.json`、`agent/heartbeat.go` 的 `buildVersion`、以及 `Devices.vue` 的版本兜底值，请不要在别处手改版本号。

### 只编译 Agent

```bash
cd agent
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags "-s -w -X main.buildVersion=1.7.6" -o LanAgent.exe .
```

### 测试

```bash
cd server && python -m pytest -q        # 后端
cd web && npm run test                  # 前端
cd agent && go test ./...               # Agent
```

### 发版流程

1. 改 `server/config.py` 的 `APP_VERSION`；
2. 更新 [docs/更新日志.md](docs/更新日志.md) 与相关文档；
3. `./build.sh` 全量构建并确认测试通过；
4. 提交并推送，打 tag `vX.Y.Z`；
5. 管理端上传新 `LanAgent.exe`，在设备页推送升级。

---

## 排障索引

| 现象 | 先看这里 |
| --- | --- |
| 设备在线但指令无响应 | 长连接是否建立：日志查 `[ws] connected`；是否有系统代理 |
| `waiting for device_id` 一直循环 | 心跳是否回填成功：日志查 `[heartbeat] saved device_id=`；安装目录是否可写 |
| 指令回报 `unknown action` | 指令名是否被下成大写 |
| 安装窗口闪退 | 是否直接双击了 `LanAgent.exe`；是否右键管理员运行 `reinstall.bat` |
| 覆盖安装失败、提示文件被占用 | 杀毒软件锁定；重启后立即运行脚本 |
| Sysprep 后登录提示密码不正确 | 用 `reset-password.bat` 清空密码 |
| 设备 IP 显示为空或不更新 | 机器是否联网；是否存在多个虚拟网卡 |

详细排查步骤见 [docs/排障手册.md](docs/排障手册.md)。

---

## 文档地图

| 文档 | 内容 |
| --- | --- |
| [docs/项目文档.md](docs/项目文档.md) | 完整设计与实现说明：架构、流程、数据模型、接口 |
| [docs/Agent协议.md](docs/Agent协议.md) | 心跳与长连接的逐字段契约、状态机、错误码 |
| [docs/部署指南.md](docs/部署指南.md) | 管理端部署、Agent 分发、升级与回滚 |
| [docs/排障手册.md](docs/排障手册.md) | 按现象索引的排查流程与日志判读 |
| [docs/安全说明.md](docs/安全说明.md) | 安全模型、威胁边界、加固清单 |
| [docs/更新日志.md](docs/更新日志.md) | 版本更新记录 |

---

## License

[MIT](LICENSE)
