# LAN Agent 局域网管理系统

## 架构

- **Agent (Go)**: 安装在每台 Win11 家庭版 PC 上，注册为 Windows 服务，通过 WebSocket 主动连接管理端
- **Server (FastAPI)**: 后端服务，设备管理、任务调度、WebSocket 通信、JWT 认证、审计日志
- **Web (Vue3 + Element Plus)**: 管理端前端

## 快速开始

### 1. 启动管理端

```bash
cd server
pip install -r requirements.txt
uvicorn main:app --host 0.0.0.0 --port 8080
```

首次启动自动创建默认管理员：`admin / admin123`，请登录后立即修改密码。

### 2. 构建前端（可选，开发时用 vite dev）

```bash
cd web
npm install
npm run build   # 输出到 server/static/
```

### 3. 在目标机器上安装 Agent

以管理员身份运行 PowerShell：

```powershell
.\LanAgent.exe /install /server=http://管理端IP:8080 /token=你的token
```

Token 在管理端添加设备时自动生成。

### 4. 卸载 Agent

```powershell
.\LanAgent.exe /uninstall
```

## 编译 Agent

需要 Go 1.21+，交叉编译 x86_64 Windows：

```bash
cd agent
GOOS=windows GOARCH=amd64 go build -o LanAgent.exe .
```

## 功能清单

### 核心功能
- 设备自动注册、心跳上报、在线状态监控
- 批量 Sysprep 通用化关机（含 unattend.xml 自动生成 + SetupComplete.cmd 恢复机制）
- 批量远程关机/重启
- 实时任务进度（WebSocket）
- Rearm 次数预检和追踪
- 设备分组管理

### 用户认证
- JWT Token 登录认证
- 默认管理员账户（admin/admin123）
- 修改密码
- 前端路由守卫，未登录自动跳转

### 批量导入
- CSV 文件上传导入（格式：hostname,ip,group）
- IP 段范围导入（最大 1024 个地址）
- 自动去重，跳过已存在的 IP

### 审计日志
- 记录所有关键操作：登录、设备增删改、批量导入、任务创建/取消
- 按操作类型筛选
- 分页查看
- 记录操作人、目标、详情、来源 IP
