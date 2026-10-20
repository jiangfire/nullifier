#!/usr/bin/env bash
set -euo pipefail

THRESHOLD="${1:-80}"
PROFILE="${2:-coverage.out}"
if [[ "$#" -gt 2 ]]; then
  PACKAGES=("${@:3}")
else
  PACKAGES=(
    "./internal/usecase"
    "./internal/config"
    "./internal/di"
    "./internal/infrastructure/http"
    "./internal/infrastructure/http/middleware"
  )
fi

# 不覆盖 GOMODCACHE / GOCACHE：CI 里 actions/setup-go 已经把它们指向带缓存的
# 目录，脚本再改成工作区内的目录会让 cache: true 失效，并把整个模块集
# 重新下载一遍。
# GOSUMDB 保持 off 是为了让离线/沙箱环境可用；go.sum 仍然逐个校验哈希。
export GOSUMDB="${GOSUMDB:-off}"

go test -coverprofile="${PROFILE}" "${PACKAGES[@]}"

TOTAL_LINE="$(go tool cover -func="${PROFILE}" | grep "total:" || true)"
if [[ -z "${TOTAL_LINE}" ]]; then
  echo "无法解析覆盖率输出"
  exit 1
fi

# Extract the first percentage on the total line without depending on the
# whitespace `go tool cover` happens to pad with: Go 1.25 separates the
# percentage with a tab while Go 1.26 uses spaces, and a parser anchored on a
# literal space silently yields 0 on the other version.
COVERAGE="$(echo "${TOTAL_LINE}" | grep -oE '[0-9]+(\.[0-9]+)?%' | head -1 | tr -d '%')"
if [[ -z "${COVERAGE}" ]]; then
  echo "覆盖率格式不符合预期: ${TOTAL_LINE}"
  exit 1
fi

awk -v coverage="${COVERAGE}" -v threshold="${THRESHOLD}" '
BEGIN {
  if (coverage + 0 < threshold + 0) {
    printf("覆盖率未达标: %.2f%% < %.2f%%\n", coverage, threshold);
    exit 1;
  }
  printf("覆盖率达标: %.2f%% >= %.2f%%\n", coverage, threshold);
}'
