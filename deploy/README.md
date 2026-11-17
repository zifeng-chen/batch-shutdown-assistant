# deploy 目录说明

> 版本 **v1.7.6**

本目录保留的是**早期命令行部署方式**的参考材料，面向开发调试与历史兼容。

> **生产部署请以 [docs/部署指南.md](../docs/部署指南.md) 为准。** 现行流程是：管理端界面生成安装包 → 解压 → 右键管理员运行 `reinstall.bat`。不再需要手工执行 `/install` 命令。

---

## 架构

| 组件 | 说明 |
| --- | --- |
| Agent (Go) | 装在每台 Win11 家庭版 PC 上，注册为 Windows 服务，通过 WebSocket 主动连管理端 |
| Server (FastAPI) | 设备管理、任务调度、WebSocket、JWT 认证、审计 |
| Web (Vue3 + Element Plus) | 管理端前端 |

---

## 开发环境启动

```bash
cd server
pip install -r requirements.txt
uvicorn main:app --host 0.0.0.0 --port 8900
```

首次启动自动创建默认管理员 `admin / admin123`，登录后立即修改。

```bash
cd web
npm install
npm run build      # 产物输出到 server/static/
```

---

## 手工安装 Agent（调试用）

```powershell
.\LanAgent.exe /install /server=http://管理端IP:8900 /token=你的token
```

进行此操作前需已存在 `C:\Program Files\LanAgent\deploy.conf`。生产环境不要用这种方式——`reinstall.bat` 会处理权限、旧版本清理、服务注册等全部细节。

卸载：

```powershell
.\LanAgent.exe /uninstall
```

---

## 编译 Agent

需要 Go 1.21+：

```bash
cd agent
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags "-s -w -X main.buildVersion=1.7.6" -o LanAgent.exe .
```

`CGO_ENABLED=0` 是必须的，否则产物依赖外部 C 运行库，无法单文件分发。

日常构建直接跑仓库根目录的 `./build.sh` 即可，它会自动同步版本号并跑测试。

---

## 功能清单

### 核心功能
- 设备自动注册、心跳上报、在线状态监控
- 批量 Sysprep 出样（含 unattend.xml 生成与 `SetupComplete.cmd` 恢复机制）
- 批量远程关机 / 重启
- 实时任务进度（WebSocket）
- Rearm 次数预检与追踪
- 硬件信息采集
- 弹窗通知、文件分发、远程清空密码
- 推送升级

### 用户认证
- JWT 认证
- 默认管理员（`admin / admin123`）
- 修改密码
- 前端路由守卫，未登录自动跳转
- 登录限流：5 次失败锁定 5 分钟

### 批量导入
- CSV 上传（`hostname,ip,group`）
- IP 段范围导入（最大 1024 个地址）
- 自动去重，跳过已存在 IP

### 审计日志
- 记录登录、设备增删改、批量导入、任务创建/取消等关键操作
- 按操作类型筛选、分页查看
- 记录操作人、目标、详情、来源 IP

---

## 相关文档

| 文档 | 内容 |
| --- | --- |
| [docs/部署指南.md](../docs/部署指南.md) | 生产部署、分发、升级、回滚 |
| [docs/Agent协议.md](../docs/Agent协议.md) | 心跳与长连接契约 |
| [docs/排障手册.md](../docs/排障手册.md) | 按现象排查 |
| [docs/安全说明.md](../docs/安全说明.md) | 安全模型与加固清单 |
