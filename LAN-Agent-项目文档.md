# LAN Agent 局域网管理系统 - 项目文档

## 1. 项目概述

LAN Agent 是一套面向 Windows 11 家庭版的局域网批量管理系统，用于对局域网内多台 PC 远程执行 Sysprep OOBE 关机/重启操作。系统采用 Agent + Web 管理端架构，被管理设备无需安装任何额外运行时，Agent 以 Windows 服务形式常驻运行。

### 核心能力

- 远程 Sysprep OOBE 关机 / 重启（不消耗 Rearm 次数）
- 设备自动注册、心跳监控、在线状态实时显示
- 批量任务下发与结果回传
- 设备批量删除（在线设备自动推送卸载指令）
- 同一个安装包可部署到任意多台设备，每台自动生成唯一 Token
- 完整的操作审计日志

---

## 2. 系统架构

```
┌─────────────────────────────────┐
│       Web 管理端 (Vue3)          │
│  仪表盘 | 设备管理 | 任务中心     │
│  审计日志 | 修改密码              │
└──────────────┬──────────────────┘
               │ REST API / WebSocket
┌──────────────▼──────────────────┐
│      后端服务 (FastAPI)          │
│  ┌──────────┐ ┌───────────────┐ │
│  │ 设备管理  │ │  任务调度引擎   │ │
│  │ CRUD/导入 │ │ 并发/状态追踪   │ │
│  └──────────┘ └───────┬───────┘ │
│  ┌──────────┐ ┌───────┴───────┐ │
│  │ JWT 认证  │ │ WebSocket 管理 │ │
│  │ 审计日志  │ │ Agent 连接池   │ │
│  └──────────┘ └───────────────┘ │
│  ┌─────────────────────────────┐ │
│  │     SQLite 持久化存储        │ │
│  └─────────────────────────────┘ │
└──────────────┬──────────────────┘
               │ HTTPS + WebSocket (Agent 主动连接)
    ┌──────────▼──────────┐
    │  Win11 家庭版 ×N     │
    │  LanAgent.exe       │
    │  · 5秒心跳上报       │
    │  · WebSocket 指令通道 │
    │  · 执行 sysprep     │
    │  · 开机自启+崩溃恢复  │
    └─────────────────────┘
```

### 技术选型

| 层级 | 技术 | 说明 |
|------|------|------|
| Agent | Go 1.21 | 编译为单个 exe，无运行时依赖 |
| 后端 | Python FastAPI + SQLAlchemy | 原生 async，SQLite 存储 |
| 前端 | Vue3 + Element Plus | 管理类 UI 组件库 |
| 通信 | WebSocket + REST API | Agent 主动连接管理端 |
| 认证 | JWT (python-jose + bcrypt) | 管理端用户认证 |
| 部署 | systemd | 管理端进程守护 |

---

## 3. 项目结构

```
lan-agent/
├── agent/                      # Go Agent 源码
│   ├── main.go                 # 入口：服务注册、安装/卸载、WebSocket
│   ├── config.go               # 配置文件读写
│   ├── heartbeat.go            # 5秒心跳 + recover 检测
│   ├── handler.go              # 指令处理：sysprep/shutdown/reboot/uninstall
│   ├── sysprep.go              # sysprep 执行 + rearm 计数读取
│   ├── netutil.go              # 网络接口枚举
│   └── go.mod / go.sum         # Go 依赖
├── server/                     # Python 后端
│   ├── main.py                 # FastAPI 应用入口 + SPA 服务
│   ├── config.py               # 配置 + 北京时间工具函数
│   ├── database.py             # SQLAlchemy 异步引擎
│   ├── requirements.txt        # Python 依赖
│   ├── lanagent.db             # SQLite 数据库（自动生成）
│   ├── static/                 # 前端构建产物（自动生成）
│   ├── models/
│   │   └── models.py           # Device / Task / TaskResult / User / AuditLog
│   └── api/
│       ├── auth.py             # JWT + bcrypt 认证
│       ├── auth_routes.py      # 登录 / 修改密码
│       ├── agent_routes.py     # Agent 心跳 / recover
│       ├── device_routes.py    # 设备 CRUD / 批量删除
│       ├── task_routes.py      # 任务创建 / 查询 / 取消
│       ├── ws_routes.py        # WebSocket Agent + Task 通道
│       ├── websocket_manager.py # WebSocket 连接池管理
│       ├── audit.py            # 审计日志写入工具
│       ├── audit_routes.py     # 审计日志查询
│       ├── import_routes.py    # CSV / IP段批量导入
│       └── deploy_routes.py    # 部署包生成（动态IP检测）
├── web/                        # Vue3 前端源码
│   ├── src/
│   │   ├── views/              # Dashboard / Devices / Tasks / Audit / Login
│   │   ├── api/index.js        # Axios 封装 + Token 拦截器
│   │   ├── router/index.js     # 路由 + 守卫
│   │   └── utils/time.js       # 时间格式化
│   ├── vite.config.js
│   └── package.json
├── installer/                  # 独立安装脚本（备用）
└── deploy/
    └── README.md               # 快速启动指南
```

---

## 4. 核心流程

### 4.1 设备注册

```
Agent 启动 → 读取 deploy.conf(server地址) → 自动生成唯一 UUID Token
    → 发送心跳 POST /api/agents/heartbeat
    → 服务端按 IP 匹配或自动创建设备记录
    → 返回 device_id → Agent 保存到本地 config.json
    → 后续每 5 秒心跳一次，同步主机名/IP/版本等信息
```

### 4.2 任务下发

```
管理端选择设备 → 创建任务 POST /api/tasks
    → 服务端通过 WebSocket 向 Agent 发送指令
    → Agent 执行 sysprep/shutdown/reboot
    → Agent 通过 WebSocket 回传执行结果
    → 管理端实时更新任务状态
```

### 4.3 覆盖安装

```
新 LanAgent.exe 运行 → 检测到 deploy.conf → 读取 server 地址
    → 生成新 UUID Token → 停止旧服务 → 替换 exe + config
    → 重启服务 → 立即发送心跳同步新 Token → 管理端更新设备记录
```

### 4.4 设备删除

```
管理端点击删除 → 检查设备是否在线
    → 在线：通过 WebSocket 推送 uninstall 指令 → Agent 停止服务并退出
    → 清理 task_results 关联记录 → 删除 devices 记录
    → 写入审计日志
```

---

## 5. 部署指南

### 5.1 管理端部署

**环境要求：** Linux 服务器，Python 3.10+

```bash
cd lan-agent/server
pip install -r requirements.txt

# 首次启动自动创建数据库和默认管理员(admin/admin123)
uvicorn main:app --host 0.0.0.0 --port 8080
```

**生产部署（systemd 守护）：**

```ini
# /etc/systemd/system/lanagent-server.service
[Unit]
Description=LAN Agent Manager Server
After=network.target

[Service]
Type=simple
WorkingDirectory=/path/to/lan-agent/server
ExecStart=/usr/bin/python3 -m uvicorn main:app --host 0.0.0.0 --port 8080
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload
systemctl enable --now lanagent-server
```

访问 `http://服务器IP:8080`，默认账号 `admin / admin123`，登录后请立即修改密码。

### 5.2 被管理端部署

**环境要求：** Windows 11 家庭版 x86_64，无需安装任何运行时

1. 在管理端「设备管理」页面点击「生成U盘安装包」
2. 下载 `LanAgent-Deploy.zip`，解压到 U 盘
3. 在目标电脑上：右键 `LanAgent.exe` → **以管理员身份运行**
4. 等待显示「[成功]LAN Agent 已安装并启动」，按回车关闭

**同一个安装包可部署到任意多台设备**，每台自动生成唯一 Token。

### 5.3 卸载

右键 `uninstall.bat` → 以管理员身份运行。或在管理端删除在线设备时自动推送卸载指令。

---

## 6. 功能清单

### 管理端

| 功能 | 说明 |
|------|------|
| 登录认证 | JWT Token，支持修改密码 |
| 仪表盘 | 设备总数/在线/离线统计，最近任务列表 |
| 设备管理 | 列表/搜索/添加/编辑主机名/单个删除/批量删除 |
| 批量操作 | 勾选多台设备 → 批量 Sysprep 关机/重启/删除 |
| 任务中心 | 创建任务/查看详情/取消任务 |
| 批量导入 | CSV 文件上传 / IP 段范围导入 |
| 审计日志 | 所有关键操作记录，按类型筛选，分页查看 |
| 部署包下载 | 一键生成含动态 IP 的安装包 |

### Agent 端

| 功能 | 说明 |
|------|------|
| 自动注册 | 首次心跳自动注册到管理端 |
| 5秒心跳 | 持续上报状态、主机名、IP、版本 |
| Sysprep 关机 | `sysprep /quiet /oobe /shutdown`（不消耗 Rearm） |
| Sysprep 重启 | `sysprep /quiet /oobe /quit` + `shutdown /r` |
| 远程卸载 | 接收 uninstall 指令后停止服务并退出 |
| 开机自启 | Windows 服务 StartAutomatic |
| 崩溃恢复 | SCM Recovery：5s/10s/30s 三级自动重启 |
| 覆盖安装 | 停旧服务 → 替换文件 → 重启 → 同步 Token |
| Token 自生成 | 每台设备独立 UUID，同包多机安全 |

---

## 7. API 接口

### 认证

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/auth/login | 登录获取 JWT |
| GET | /api/auth/me | 获取当前用户 |
| POST | /api/auth/change-password | 修改密码 |

### 设备

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/devices | 设备列表（分页/搜索） |
| GET | /api/devices/stats | 设备统计 |
| POST | /api/devices | 添加设备 |
| PUT | /api/devices/{id} | 更新设备 |
| DELETE | /api/devices/{id} | 删除设备（在线则推送卸载） |
| POST | /api/devices/batch-delete | 批量删除 |
| POST | /api/devices/import/csv | CSV 导入 |
| POST | /api/devices/import/ip-range | IP 段导入 |

### Agent

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/agents/heartbeat | Agent 心跳注册/更新 |
| POST | /api/agents/recover | Sysprep 后恢复上报 |

### 任务

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/tasks | 创建并下发任务 |
| GET | /api/tasks | 任务列表 |
| GET | /api/tasks/{id} | 任务详情+结果 |
| POST | /api/tasks/{id}/cancel | 取消任务 |

### WebSocket

| 路径 | 说明 |
|------|------|
| /ws/agents/{device_id}?token=xxx | Agent 指令通道（需 Token 验证） |
| /ws/tasks/{task_id} | 任务进度推送 |

### 其他

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/audit | 审计日志（分页/筛选） |
| POST | /api/deploy/package | 下载部署包（动态 IP） |
| GET | /api/health | 健康检查 |

---

## 8. 数据模型

### Device

| 字段 | 类型 | 说明 |
|------|------|------|
| id | UUID | 主键 |
| hostname | String | 主机名 |
| ip | String | IP 地址 |
| status | Enum | online / offline |
| token | String | Agent 认证令牌（自动生成） |
| rearm_count | Int | Rearm 剩余次数 |
| agent_version | String | Agent 版本号 |
| last_heartbeat | DateTime | 最后心跳时间（北京时间） |

### Task / TaskResult

| 字段 | 说明 |
|------|------|
| task_type | sysprep / sysprep_reboot |
| status | pending / running / completed / failed / cancelled |
| results | 每台设备的执行结果 |

### AuditLog

| 字段 | 说明 |
|------|------|
| action | device_create / device_delete / task_create / deploy_package 等 |
| username | 操作人 |
| target | 操作目标 |
| detail | 详细信息 |

---

## 9. 安全设计

- **JWT 认证**：所有管理端 API 需 Bearer Token，未认证返回 401
- **WebSocket 认证**：Agent 连接需携带 Token query 参数，服务端校验 device.token
- **Token 隔离**：每台设备独立 UUID Token，无共享凭证
- **密码加密**：bcrypt 哈希存储
- **SQL 注入防护**：SQLAlchemy ORM 参数化查询
- **XML 注入防护**：unattend.xml 内容转义
- **动态 IP 检测**：部署包自动检测宿主机局域网 IP，不硬编码
- **No-Cache 头**：前端静态资源禁止浏览器缓存

---

## 10. 已知限制

- Windows 11 家庭版不支持 WinRM，必须使用 Agent 模式
- Sysprep `/oobe` 不带 `/generalize`，不清除 SID，不消耗 Rearm
- Agent 仅支持 Windows x86_64
- 管理端为单进程部署，适合中小规模（百台级别）
- SQLite 不适合高并发写入场景，大规模建议迁移 PostgreSQL
