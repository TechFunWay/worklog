#!/usr/bin/env bash
# check-sync.sh — SmallGo 框架文件同步校验
#
# 背景：techfunway 家族的应用（bill、reminders 等）是 smallgo 框架的实例，
# go.mod 模块名同为 smallgo/server。框架层代码（auth/audit/logger/scheduler/
# sysconfig/upload/...）应与 smallgo 基线保持一致；业务包（如 bill/、reminder/）
# 及少量实例化文件（端口、网关前缀、AppName）允许分叉。
#
# 本脚本将 smallgo 的框架基线文件与实例仓库逐一 diff，输出三类结果：
#   MISSING  —— smallgo 有而实例缺失（多为新增框架能力，需要回填）
#   DRIFTED  —— 两边都有但内容不同（需要人工评估合并方向）
#   OK       —— 完全一致
#
# 用法:
#   ./scripts/check-sync.sh <实例仓库根目录>...
# 示例:
#   ./scripts/check-sync.sh ../bill ../reminders
set -uo pipefail

SMALLGO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# 框架基线文件（相对 server/）。新增框架包/文件时维护此清单。
FRAMEWORK_FILES=(
  apps/registry.go
  audit/audit.go
  auth/jwt.go
  config/dotenv.go
  database/database.go
  database/upgrade.go
  logger/logger.go
  middleware/middleware.go
  middleware/ratelimit.go
  realtime/realtime.go
  response/response.go
  scheduler/scheduler.go
  security/security.go
  sysconfig/handlers.go
  sysconfig/registry.go
  sysconfig/sysconfig.go
  upload/upload.go
  utils/export.go
  utils/pagination.go
)

# 实例化文件：必然不同（端口/网关前缀/AppName/业务 import），单独列出仅供参考。
INSTANCE_FILES=(
  config/config.go
  main.go
  server/server.go
  version/version.go
  database/models.go
  user/handler.go
  user/service.go
  sysconfig/sysconfig_test.go
)

if [[ $# -lt 1 ]]; then
  echo "用法: $0 <实例仓库根目录>...   例: $0 ../bill ../reminders" >&2
  exit 1
fi

status=0
for repo in "$@"; do
  repo_abs="$(cd "$repo" 2>/dev/null && pwd)" || { echo "跳过：$repo 不是目录" >&2; continue; }
  name="$(basename "$repo_abs")"
  server_dir="$repo_abs/server"
  if [[ ! -d "$server_dir" ]]; then
    echo "[$name] 未找到 $server_dir（不是 smallgo/server 实例？）"
    echo
    continue
  fi

  echo "=== $name ==="
  missing=0; drifted=0
  for f in "${FRAMEWORK_FILES[@]}"; do
    if [[ ! -f "$server_dir/$f" ]]; then
      echo "  MISSING  server/$f"
      missing=$((missing+1)); continue
    fi
    if ! diff -q "$SMALLGO_ROOT/server/$f" "$server_dir/$f" >/dev/null 2>&1; then
      echo "  DRIFTED  server/$f"
      drifted=$((drifted+1))
    fi
  done
  echo "  小结: $((${#FRAMEWORK_FILES[@]} - missing - drifted)) 一致 / $drifted 漂移 / $missing 缺失"

  # 实例化文件仅提示存在差异的数量（必然不同，不判失败）
  instance_diff=0
  for f in "${INSTANCE_FILES[@]}"; do
    if [[ -f "$server_dir/$f" ]] && ! diff -q "$SMALLGO_ROOT/server/$f" "$server_dir/$f" >/dev/null 2>&1; then
      instance_diff=$((instance_diff+1))
    fi
  done
  echo "  实例化文件（预期分叉，人工把关）: $instance_diff/${#INSTANCE_FILES[@]} 处不同"
  echo

  if (( missing > 0 || drifted > 0 )); then
    status=1
  fi
done

if (( status != 0 )); then
  echo "结论：存在需要回流/评估的框架分叉（同步方向：以 smallgo 基线为准，实例差异回流为扩展点）。"
else
  echo "结论：所有实例的框架层文件与 smallgo 基线一致。"
fi
exit $status
