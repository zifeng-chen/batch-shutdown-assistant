#!/usr/bin/env bash
#
# LAN Agent 一键构建脚本
#
#   版本号唯一来源：server/config.py 的 APP_VERSION 常量
#   脚本将其同步到前后端并编译产物：
#     1. Agent (Go)      -> agent/LanAgent.exe   (Windows amd64)
#     2. Web   (Vue)     -> server/static/       (vite 输出)
#     3. 版本同步        -> web/package.json, agent/heartbeat.go
#
# 用法:
#   ./build.sh                    # 全量构建（含测试，版本取 config.py 的 APP_VERSION）
#   ./build.sh --version 1.4.2    # 指定版本构建，并回写 config.py
#   ./build.sh --web-only         # 仅构建前端
#   ./build.sh --agent-only       # 仅编译 Agent
#   ./build.sh --skip-tests       # 跳过测试
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

CONFIG_PY="server/config.py"

# ---------- 解析参数 ----------
TARGET="all"
NEW_VERSION=""
SKIP_TESTS=0
while [ $# -gt 0 ]; do
  case "$1" in
    --web-only)   TARGET="web" ;;
    --agent-only) TARGET="agent" ;;
    --skip-tests) SKIP_TESTS=1 ;;
    --version)    shift; NEW_VERSION="${1:-}" ;;
    -h|--help)
      sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//'
      exit 0 ;;
    *)
      echo "未知参数: $1" >&2; exit 1 ;;
  esac
  shift
done

# ---------- 版本号解析（唯一来源: config.py）----------
read_app_version() {
  grep -oP '^APP_VERSION\s*=\s*os\.environ\.get\("LANAGENT_VERSION",\s*"\K[^"]+' "$CONFIG_PY"
}

if [ -n "$NEW_VERSION" ]; then
  if ! [[ "$NEW_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "错误: 版本号 '$NEW_VERSION' 非法（应为 x.y.z）" >&2; exit 1
  fi
  sed -i "s/^APP_VERSION = os\.environ\.get(\"LANAGENT_VERSION\", \"[^\"]*\")/APP_VERSION = os.environ.get(\"LANAGENT_VERSION\", \"$NEW_VERSION\")/" "$CONFIG_PY"
fi

VERSION="$(read_app_version)"
if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "错误: 无法从 $CONFIG_PY 解析出合法版本号（得到 '$VERSION'）" >&2
  exit 1
fi
echo "==> 构建版本: $VERSION  (来源: $CONFIG_PY)"

# ---------- 同步版本号到其他端 ----------
sync_version() {
  echo "==> 同步版本号到各端"

  # web/package.json
  if command -v node >/dev/null 2>&1; then
    node -e '
      const fs=require("fs"),p="web/package.json";
      const j=JSON.parse(fs.readFileSync(p,"utf8"));
      j.version=process.argv[1];
      fs.writeFileSync(p, JSON.stringify(j,null,2)+"\n");
    ' "$VERSION"
  else
    sed -i "s/\"version\": \"[^\"]*\"/\"version\": \"$VERSION\"/" web/package.json
  fi

  # agent/heartbeat.go 的 buildVersion 默认值
  sed -i "s/^var buildVersion = \"[^\"]*\"/var buildVersion = \"$VERSION\"/" agent/heartbeat.go

  # 前端升级对话框的版本兜底值（服务端不可达时显示）
  sed -i "s/upgradeVersion\.value = '[0-9.]*'/upgradeVersion.value = '$VERSION'/" web/src/views/Devices.vue

  echo "    完成"
}

# ---------- 构建 Agent (Go) ----------
build_agent() {
  echo "==> 编译 Agent (Windows amd64)"
  : "${HOME:=/root}"
  export HOME
  export GOPATH="${GOPATH:-$HOME/go}"
  export GOMODCACHE="${GOMODCACHE:-$GOPATH/pkg/mod}"
  export GOCACHE="${GOCACHE:-$HOME/.cache/go-build}"
  export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
  export GOOS=windows GOARCH=amd64 CGO_ENABLED=0

  ( cd agent && go build \
      -ldflags "-s -w -X main.buildVersion=$VERSION" \
      -o LanAgent.exe . )

  local size
  size=$(du -h agent/LanAgent.exe | cut -f1)
  echo "    完成 -> agent/LanAgent.exe ($size)"
}

# ---------- 构建 Web (Vue) ----------
build_web() {
  echo "==> 构建前端 (vite -> server/static)"
  ( cd web
    if [ ! -d node_modules ]; then
      echo "    安装依赖中..."
      npm install --no-audit --no-fund
    fi
    npm run build )
  echo "    完成 -> server/static/"
}

# ---------- 运行测试 ----------
run_tests() {
  echo "==> 运行测试"

  if [ -d web/node_modules ]; then
    ( cd web && npm run test --silent ) || { echo "前端测试失败，构建中止" >&2; exit 1; }
  else
    echo "    跳过前端测试（未安装 node_modules）"
  fi

  local py="python3"
  for cand in "$ROOT/server/.venv/bin/python" "$ROOT/.venv/bin/python"; do
    [ -x "$cand" ] && py="$cand" && break
  done

  if "$py" -c "import pytest" >/dev/null 2>&1; then
    ( cd server && "$py" -m pytest -q ) || { echo "后端测试失败，构建中止" >&2; exit 1; }
  else
    echo "    跳过后端测试（未安装 pytest，见 server/requirements-dev.txt）"
  fi
}

# ---------- 执行 ----------
sync_version

if [ "$SKIP_TESTS" -eq 1 ]; then
  echo "==> 已跳过测试 (--skip-tests)"
else
  run_tests
fi

case "$TARGET" in
  all)   build_agent; build_web ;;
  agent) build_agent ;;
  web)   build_web ;;
esac

echo
echo "==> 构建完成 (v$VERSION)"