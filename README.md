# 批量关机助手

面向 Windows 11 家庭版的局域网批量管理系统，远程执行 Sysprep OOBE 关机/重启操作。

## 功能特性

- **Sysprep OOBE 关机/重启** — 等同于系统准备工具，不消耗 Rearm 次数
- **设备自动注册** — 安装后 5 秒内出现在管理端
- **批量操作** — 勾选多台设备一键下发指令
- **推送升级** — 管理端一键推送新版 Agent，无需逐台操作
- **文件分发** — 向被管理端推送文件到指定目录
- **离线检测** — 15 秒未心跳自动标记离线，红色高亮
- **操作审计** — 所有关键操作完整记录
- **同包多机** — 同一个安装包部署到任意多台设备

## 系统架构

```
Web 管理端 (Vue3 + Element Plus)
        ↕ REST API / WebSocket
后端服务 (FastAPI + SQLite)
        ↕ WebSocket (Agent 主动连接)
Win11 家庭版 ×N (Go Agent, Windows 服务)
```

## 快速开始

### 1. 部署管理端

```bash
cd server
pip install -r requirements.txt
uvicorn main:app --host 0.0.0.0 --port 8080
```

访问 `http://服务器IP:8080`，默认账号 `admin / admin123`。

生产环境建议使用 systemd 守护：

```ini
# /etc/systemd/system/lanagent-server.service
[Unit]
Description=LAN Agent Manager
After=network.target

[Service]
Type=simple
WorkingDirectory=/path/to/server
ExecStart=/usr/bin/python3 -m uvicorn main:app --host 0.0.0.0 --port 8080
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### 2. 安装被管理端

1. 在管理端「设备管理」→「生成U盘安装包」
2. 解压到 U 盘
3. 目标电脑上右键 `LanAgent.exe` → **以管理员身份运行**
4. 等待显示成功后按回车关闭

> 同一个安装包可安装到任意多台设备，每台自动生成唯一标识。

### 3. 编译 Agent

需要 Go 1.21+：

```bash
cd agent
VERSION=$(cat version.txt | tr -d '[:space:]')
GOOS=windows GOARCH=amd64 go build -ldflags "-X lan-agent.agentVersion=$VERSION" -o LanAgent.exe .
```

版本号通过 `-ldflags` 嵌入 exe，不依赖外部文件。发版时修改 `agent/version.txt` 再重新编译即可。

## 项目结构

```
├── agent/              # Go Agent（被管理端）
│   ├── main.go         # 入口、服务注册、安装/卸载
│   ├── heartbeat.go    # 5秒心跳、版本上报
│   ├── handler.go      # 指令处理(sysprep/shutdown/upgrade/file_push/uninstall)
│   ├── sysprep.go      # Sysprep 执行、Rearm 计数读取
│   └── config.go       # 配置读写
├── server/             # Python FastAPI 后端
│   ├── main.py         # 应用入口、SPA 服务、心跳超时检测
│   ├── api/            # REST + WebSocket 路由
│   ├── models/         # SQLAlchemy 数据模型
│   └── static/         # 前端构建产物
├── web/                # Vue3 前端源码
│   └── src/views/      # Dashboard/Devices/Tasks/Audit/Help/Login
└── deploy/             # 部署文档
```

## API 概览

| 模块 | 接口 | 说明 |
|------|------|------|
| 认证 | POST /api/auth/login | JWT 登录 |
| 设备 | GET/POST/PUT/DELETE /api/devices | 设备 CRUD |
| 设备 | POST /api/devices/batch-delete | 批量删除（在线设备自动卸载） |
| Agent | POST /api/agents/heartbeat | 心跳注册/更新 |
| 任务 | POST /api/tasks | 创建并下发任务 |
| 升级 | GET /api/upgrade/latest | 获取服务端最新版本 |
| 升级 | POST /api/upgrade/file-upload | 上传分发文件 |
| 部署 | POST /api/deploy/package | 下载安装包（动态IP） |
| 审计 | GET /api/audit | 操作日志查询 |
| WS | /ws/agents/{id}?token=xxx | Agent 指令通道 |
| WS | /ws/tasks/{id} | 任务进度实时推送 |

## 支持的指令

| 指令 | 说明 | 消耗 Rearm |
|------|------|-----------|
| sysprep | `sysprep /quiet /oobe /shutdown` | ❌ |
| sysprep_reboot | `sysprep /quiet /oobe /quit` + reboot | ❌ |
| shutdown | `shutdown /s /f /t 0` | ❌ |
| reboot | `shutdown /r /f /t 0` | ❌ |
| file_push | 下载文件到指定目录 | ❌ |
| upgrade | 下载新 exe → 停服务 → 替换 → 重启 | ❌ |
| uninstall | 停服务 → 删除注册 → 清理文件 | ❌ |

## 安全设计

- JWT 认证保护所有管理端 API
- WebSocket Agent 连接需 Token 验证
- 每台设备独立 UUID Token
- 密码 bcrypt 哈希存储
- SQL 注入防护（ORM 参数化查询）
- XML 注入防护（unattend.xml 转义）
- 部署包动态检测宿主机 IP，无硬编码

## 环境要求

- **管理端：** Linux/macOS，Python 3.10+
- **被管理端：** Windows 11 家庭版 x86_64，无需额外运行时
- **网络：** 局域网互通，Agent 主动连接管理端（无需开放入站端口）

## License

MIT
