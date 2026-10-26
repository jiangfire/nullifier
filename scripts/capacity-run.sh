#!/usr/bin/env bash
#
# 容量压测脚本 —— 跑出「这台服务能扛多少票」的数字（T27）。
#
# 与 browser-acceptance.sh 的区别：那个脚本测的是「一条链路能不能走通」，
# 这个脚本测的是**容量**：吞吐、延迟分位、错误率。
#
# 它跑的是**真实投票路径**：
#   - 真 Groth16 证明（本地出证明，服务端真验签）
#   - 资格证明领取（一人一票，与 release 模式同一条链路）
#   - release 模式的服务端 + **真 PostgreSQL**（生产用的就是 PostgreSQL）
#
# 测出来的数字回答的是「多少投票规模配多少资源」这个问题，
# 结论写在 docs/capacity-report-v0.3.0.md。
#
# 用法：
#   bash ./scripts/capacity-run.sh setup     备好环境（密钥 / 名单 / 库 / 服务端）
#   bash ./scripts/capacity-run.sh run       跑一轮压测，输出报告
#   bash ./scripts/capacity-run.sh sweep     按 CONCURRENCIES 跑一轮并发扫描
#   bash ./scripts/capacity-run.sh stop      收尾（停服务端）
#
# 可调参数（都是环境变量）：
#   WORK_DIR=.capacity          工作目录：密钥、名单、配置、日志、报告
#   API_PORT=18090              服务端端口
#   ADMIN_TOKEN=...             管理端 Bearer token
#   VOTES=200                   一轮压测提交多少张票
#   CONCURRENCY=20              并发度
#   CONCURRENCIES="1 5 20 50"   sweep 时依次试的并发度
#   DUPLICATES=5                防重投复核的并发请求数
#   MEMBERS=256                  资格名单的成员数（每人一份资格包）
#                               ⚠️ 必须 ≥ VOTES + DUPLICATES：同一提案里一个资格
#                                  只能领一次票（一人一票）
#   OPEN_DELAY=5s               投票窗口距现在多少秒打开（**带单位**，如 5s）
#
#   DB_DRIVER=postgres          数据库驱动：postgres（默认）或 sqlite
#   PG_HOST=127.0.0.1           postgres 主机
#   PG_PORT=5432                postgres 端口
#   PG_USER=postgres            postgres 用户
#   PG_PASSWORD=...             postgres 密码
#   PG_DATABASE=anonvote_capacity  postgres 库名（**必须已经存在**，脚本不建库）
#   PG_SSLMODE=disable          postgres sslmode
#
#   RATE_LIMIT_RPS=100000       压测期间把限流额度拉高（否则量到的是限流器）
#   RATE_LIMIT_BURST=200000     同上
#
#   DB_MAX_OPEN_CONNS=16         服务端数据库连接池上限
#   DB_MAX_IDLE_CONNS=16         空闲连接数
#   DB_CONN_MAX_LIFETIME_SECONDS=3600  连接最大存活时间
#
# ⚠️ **连接池别用 database/sql 的默认值**：默认只保留 2 条空闲连接，高并发下
#    每个请求都要重开一条数据库连接 —— 量到的是连接开销，不是服务端能力。
#
# ⚠️ **也别开太大**：实测把池从 16 开到 64，第一轮的吞吐从 2134/s 掉到 **307/s**
#    （64 条连接要现场建 64 个 PostgreSQL 后端进程，每个约 38 MB），
#    连跑几十轮后吞吐还会从 2300/s 回落到 1400/s。投票写路径每请求只占连接
#    约 0.5 ms，16 条连接的理论上限已是 3 万+/s —— 远高于实测的 2200/s。
#    结论见 docs/capacity-report-v0.3.0.md。
#
# ⚠️ **限流额度必须拉高**：压测的请求全来自同一台机器，共享一份额度 ——
#    用默认额度（votes 10 rps / 40 burst）的话，几百个请求会被限流挡下，
#    量出来的就是限流阈值而不是服务端的容量。
#
# ⚠️ **压测客户端与服务端在同一台机器上**：测量窗口里客户端只发 HTTP 请求
#    （出证明在窗口之前就跑完了），但两边的 CPU 仍然共享。这会**低估**服务端
#    的能力 —— 真实部署里客户端在别的机器上。结论里写明了这一点。

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

WORK_DIR="${WORK_DIR:-.capacity}"
API_PORT="${API_PORT:-18090}"
ADMIN_TOKEN="${ADMIN_TOKEN:-capacity-admin-token}"
VOTES="${VOTES:-200}"
CONCURRENCY="${CONCURRENCY:-20}"
CONCURRENCIES="${CONCURRENCIES:-1 5 20 50}"
DUPLICATES="${DUPLICATES:-5}"
# PROVE_WORKERS 是客户端并行出证明 / 并行领取的 worker 数（0 = CPU 核数）。
#
# ⚠️ 调成 1 会让领取**串行** —— 那正是用来对照的：服务端取槽位不占位，
#    并发领取会抢同一个槽位。串行能过、并行过不去，就说明问题在槽位分配上。
PROVE_WORKERS="${PROVE_WORKERS:-0}"
MEMBERS="${MEMBERS:-256}"
OPEN_DELAY="${OPEN_DELAY:-5s}"
RATE_LIMIT_RPS="${RATE_LIMIT_RPS:-100000}"
RATE_LIMIT_BURST="${RATE_LIMIT_BURST:-200000}"
DB_MAX_OPEN_CONNS="${DB_MAX_OPEN_CONNS:-16}"
DB_MAX_IDLE_CONNS="${DB_MAX_IDLE_CONNS:-16}"
DB_CONN_MAX_LIFETIME_SECONDS="${DB_CONN_MAX_LIFETIME_SECONDS:-3600}"

DB_DRIVER="${DB_DRIVER:-postgres}"
PG_HOST="${PG_HOST:-127.0.0.1}"
PG_PORT="${PG_PORT:-5432}"
PG_USER="${PG_USER:-postgres}"
PG_PASSWORD="${PG_PASSWORD:-}"
PG_DATABASE="${PG_DATABASE:-anonvote_capacity}"
PG_SSLMODE="${PG_SSLMODE:-disable}"

API_BASE="http://127.0.0.1:${API_PORT}"

case "$(uname -s 2>/dev/null || echo unknown)" in
  MINGW* | MSYS* | CYGWIN*) export CGO_ENABLED=1 ;;
esac

native_path() {
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -m "$1"
  else
    printf '%s' "$1"
  fi
}

log() { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
info() { printf '    %s\n' "$*"; }
die() { printf '\n\033[31m错误: %s\033[0m\n' "$*" >&2; exit 1; }

# require_cmd 检查命令存在。
require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "缺少命令: $1"
}

# port_in_use 检查端口有没有被占。
#
# **必须查**：端口被占时，服务端起不来，但健康检查会打到**别人**的服务上
# —— 于是压测跑在一个名单根不同、配置不同的服务端上，数字全是错的
# （而且没人会发现）。
port_in_use() {
  local port="$1"
  if command -v netstat >/dev/null 2>&1; then
    netstat -an 2>/dev/null | grep -qE "[:.]${port}[[:space:]].*LISTEN" && return 0
  fi
  return 1
}

# json_string 从一份 JSON 报告文件里取某个字符串字段的值。
json_string() {
  cat "$1" | sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" | head -n1
}

# json_number 从一份 JSON 报告文件里取某个数值字段的值。
json_number() {
  cat "$1" | sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\([0-9.]\{1,\}\).*/\1/p" | head -n1
}

# json_statuses 把 status_counts 摊平成 "201×12 409×3" 这样的一行。
#
# 报告里的 JSON 是**多行**排版的（status_counts 的 `{` 与 `}` 不在同一行），
# 所以先压成一行再匹配 —— 否则 sed 一行都匹配不到，状态分布那一列永远是空的。
json_statuses() {
  local captured
  # 报告里的 JSON 是**多行**排版的（status_counts 的 `{` 与 `}` 不在同一行），
  # 所以先压成一行再匹配 —— 否则 sed 一行都匹配不到，状态分布那一列永远是空的。
  #
  # 压平后的内容**没有结尾换行**：直接喂给 `read` 会在 EOF 处返回非零，
  # 循环体一次都不执行（状态分布又变成空的）。所以先捕获，再用 printf 补上换行。
  captured="$(tr -d '\n' <"$1" \
    | sed -n 's/.*"status_counts"[[:space:]]*:[[:space:]]*{\([^}]*\)}.*/\1/p' \
    | head -n1)"
  printf '%s\n' "$captured" | tr ',' '\n' | while read -r entry; do
        [ -n "$entry" ] || continue
        key="$(printf '%s' "$entry" | sed -n 's/.*"\([^"]*\)"[[:space:]]*:[[:space:]]*\([0-9]\{1,\}\).*/\1/p')"
        value="$(printf '%s' "$entry" | sed -n 's/.*"\([^"]*\)"[[:space:]]*:[[:space:]]*\([0-9]\{1,\}\).*/\2/p')"
        [ -n "$key" ] && printf '%s×%s ' "$key" "$value"
      done | sed 's/[[:space:]]*$//'
}

# wait_for_http 等一个 URL 返回 2xx，最多等 $2 秒。
wait_for_http() {
  local url="$1"
  local timeout="${2:-30}"
  local deadline
  deadline=$(( $(date +%s) + timeout ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    if curl -fsS -o /dev/null --max-time 3 "$url" 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  return 1
}

cmd_setup() {
  require_cmd go
  require_cmd curl

  # 端口被占时早失败：否则健康检查会打到别人的服务端上，
  # 压测跑在错误的配置上（名单根、额度、数据库全不一样）。
  if port_in_use "$API_PORT"; then
    die "端口 ${API_PORT} 已被占用 —— 换个 API_PORT 再跑（或者先停掉占用的进程）"
  fi

  # 工作目录已存在且**非空**时才拒绝：空目录留着不影响（上一次跑崩了留下的），
  # 直接复用，省得手工去删。
  if [ -e "$WORK_DIR" ] && [ -n "$(ls -A "$WORK_DIR" 2>/dev/null)" ]; then
    die "工作目录已存在且非空: $WORK_DIR —— 先跑 stop 并删掉它，或换个 WORK_DIR"
  fi

  mkdir -p "$WORK_DIR/keys" "$WORK_DIR/eligibility" "$WORK_DIR/bin" "$WORK_DIR/reports"
  WORK_DIR_ABS="$(cd "$WORK_DIR" && pwd)"
  WORK_DIR_NATIVE="$(native_path "$WORK_DIR_ABS")"

  log "1/6 构建二进制"
  go build -o "$WORK_DIR_NATIVE/bin/keygen" ./cmd/keygen
  go build -o "$WORK_DIR_NATIVE/bin/server" ./cmd/server
  go build -o "$WORK_DIR_NATIVE/bin/loadtest" ./cmd/loadtest
  go build -o "$WORK_DIR_NATIVE/bin/eligibility-list" ./cmd/eligibility-list
  info "keygen / server / loadtest / eligibility-list"

  log "2/6 生成两套密钥（选票 / 资格）"
  "$WORK_DIR_NATIVE/bin/keygen" -dir "$WORK_DIR_NATIVE/keys" -force >/dev/null
  "$WORK_DIR_NATIVE/bin/keygen" -circuit eligibility -dir "$WORK_DIR_NATIVE/keys" -force >/dev/null
  info "keys/verifying.key、keys/eligibility-verifying.key（服务端用）"
  info "keys/proving.key、keys/eligibility-proving.key（**只在客户端**，不进服务端）"

  log "3/6 生成资格名单（离线工具，名单根与成员包分开保管）"
  : >"$WORK_DIR_ABS/members.txt"
  local index
  for index in $(seq 1 "$MEMBERS"); do
    printf 'voter-%03d\n' "$index" >>"$WORK_DIR_ABS/members.txt"
  done
  "$WORK_DIR_NATIVE/bin/eligibility-list" -members "$WORK_DIR_NATIVE/members.txt" \
    -out "$WORK_DIR_NATIVE/eligibility" -force >/dev/null
  local root_hex
  root_hex="$(json_string "$WORK_DIR_ABS/eligibility/root.json" root)"
  [ -n "$root_hex" ] || die "读不出名单根：$WORK_DIR/eligibility/root.json"
  info "$MEMBERS 名成员，名单根 $root_hex"

  log "4/6 写 release 模式的配置"
  # release 模式：**强制**一人一票（资格根与资格验证密钥成对）、强制 admin 令牌、
  # 强制限流。压测要的就是这个形态 —— 开发模式（不验签、不校验资格）测出来的
  # 数字跟生产没有可比性。
  local db_block
  if [ "$DB_DRIVER" = "postgres" ]; then
    if ! (exec 3<>"/dev/tcp/${PG_HOST}/${PG_PORT}") 2>/dev/null; then
      die "连不上 PostgreSQL ${PG_HOST}:${PG_PORT} —— 先起库（或改用 DB_DRIVER=sqlite）"
    fi
    db_block="db:
  driver: \"postgres\"
  host: \"${PG_HOST}\"
  port: ${PG_PORT}
  name: \"${PG_DATABASE}\"
  user: \"${PG_USER}\"
  password: \"${PG_PASSWORD}\"
  sslmode: \"${PG_SSLMODE}\"
  max_open_conns: ${DB_MAX_OPEN_CONNS}
  max_idle_conns: ${DB_MAX_IDLE_CONNS}
  conn_max_lifetime_seconds: ${DB_CONN_MAX_LIFETIME_SECONDS}"
    info "数据库: PostgreSQL ${PG_HOST}:${PG_PORT}/${PG_DATABASE}（**必须已存在**）"
  else
    db_block="db:
  driver: \"sqlite\"
  path: \"${WORK_DIR_NATIVE}/capacity.db\""
    info "数据库: SQLite ${WORK_DIR_NATIVE}/capacity.db"
  fi
  cat >"$WORK_DIR_ABS/config.yaml" <<EOF
server:
  address: ":${API_PORT}"
  mode: "release"
  admin_bearer_token: "${ADMIN_TOKEN}"
  cors_allowed_origins: []
  trusted_proxies: []
  auto_migrate: false
crypto:
  vk_path: "${WORK_DIR_NATIVE}/keys/verifying.key"
  issuer_key_path: "${WORK_DIR_NATIVE}/issuer_keys.json"
  eligibility_root: "${root_hex}"
  eligibility_vk_path: "${WORK_DIR_NATIVE}/keys/eligibility-verifying.key"
rate_limit:
  enabled: true
  credentials:
    rps: ${RATE_LIMIT_RPS}
    burst: ${RATE_LIMIT_BURST}
  votes:
    rps: ${RATE_LIMIT_RPS}
    burst: ${RATE_LIMIT_BURST}
  read:
    rps: ${RATE_LIMIT_RPS}
    burst: ${RATE_LIMIT_BURST}
${db_block}
EOF
  info "配置: $WORK_DIR_ABS/config.yaml"

  log "5/6 迁移并启动服务端"
  CONFIG_FILE="$WORK_DIR_NATIVE/config.yaml" \
    "$WORK_DIR_NATIVE/bin/server" -migrate >"$WORK_DIR_ABS/migrate.log" 2>&1 || {
      tail -n 20 "$WORK_DIR_ABS/migrate.log" >&2 || true
      die "迁移失败，看 $WORK_DIR_ABS/migrate.log"
    }
  CONFIG_FILE="$WORK_DIR_NATIVE/config.yaml" \
    "$WORK_DIR_NATIVE/bin/server" >"$WORK_DIR_ABS/server.log" 2>&1 &
  echo $! >"$WORK_DIR_ABS/server.pid"
  if ! wait_for_http "$API_BASE/healthz" 30; then
    tail -n 30 "$WORK_DIR_ABS/server.log" >&2 || true
    die "服务端没起来，看 $WORK_DIR_ABS/server.log"
  fi
  local health
  health="$(curl -fsS "$API_BASE/healthz")"
  info "healthz: $health"
  case "$health" in
    *'"eligibility":true'*) : ;;
    *) die "healthz 里 eligibility 不是 true —— 这台实例没在强制一人一票" ;;
  esac

  log "6/6 环境就绪"
  cat >"$WORK_DIR_ABS/state.env" <<EOF
API_BASE=${API_BASE}
ADMIN_TOKEN=${ADMIN_TOKEN}
WORK_DIR_NATIVE=${WORK_DIR_NATIVE}
LOADTEST_BIN=${WORK_DIR_NATIVE}/bin/loadtest
SERVER_LOG=${WORK_DIR_ABS}/server.log
EOF
  info "跑压测：bash ./scripts/capacity-run.sh run"
  info "跑扫描：bash ./scripts/capacity-run.sh sweep"
  info "收尾：  bash ./scripts/capacity-run.sh stop"
}

# run_loadtest 跑一轮压测，返回报告 JSON 的路径。
run_loadtest() {
  local concurrency="$1"
  local out="$WORK_DIR_ABS/reports/report-c${concurrency}.json"
  # shellcheck disable=SC1090
  . "$WORK_DIR_ABS/state.env"
  "$LOADTEST_BIN" \
    -base-url "$API_BASE" \
    -admin-token "$ADMIN_TOKEN" \
    -votes "$VOTES" \
    -concurrency "$concurrency" \
    -duplicates "$DUPLICATES" \
    -open-delay "$OPEN_DELAY" \
    -proving-key "$WORK_DIR_NATIVE/keys/proving.key" \
    -eligibility-dir "$WORK_DIR_NATIVE/eligibility" \
    -eligibility-proving-key "$WORK_DIR_NATIVE/keys/eligibility-proving.key" \
    -prove-workers "$PROVE_WORKERS" \
    -json >"$out" 2>"$WORK_DIR_ABS/reports/last.err"
  printf '%s' "$out"
}

cmd_run() {
  [ -f "$WORK_DIR/state.env" ] || die "找不到 $WORK_DIR/state.env —— 先跑 setup"
  WORK_DIR_ABS="$(cd "$WORK_DIR" && pwd)"
  log "跑一轮：$VOTES 张票，并发度 $CONCURRENCY"
  local out
  out="$(run_loadtest "$CONCURRENCY")"
  cat "$out"
  info "报告: $out"
}

cmd_sweep() {
  [ -f "$WORK_DIR/state.env" ] || die "找不到 $WORK_DIR/state.env —— 先跑 setup"
  WORK_DIR_ABS="$(cd "$WORK_DIR" && pwd)"
  log "并发扫描：$VOTES 张票，依次试并发度 $CONCURRENCIES"
  printf '%-12s %-10s %-12s %-12s %-12s %s\n' "并发度" "吞吐/s" "p50" "p99" "最大" "状态分布"
  local concurrency out throughput p50 p99 max statuses
  for concurrency in $CONCURRENCIES; do
    if ! out="$(run_loadtest "$concurrency")"; then
      info "并发度 $concurrency：压测失败，看 $WORK_DIR_ABS/reports/last.err"
      continue
    fi
    throughput="$(json_number "$out" throughput_per_second)"
    p50="$(json_string "$out" latency_p50)"
    p99="$(json_string "$out" latency_p99)"
    max="$(json_string "$out" latency_max)"
    statuses="$(json_statuses "$out")"
    printf '%-12s %-10s %-12s %-12s %-12s %s\n' \
      "$concurrency" "$throughput" "$p50" "$p99" "$max" "$statuses"
  done
  info "报告在 $WORK_DIR_ABS/reports/"
}

cmd_stop() {
  log "停掉后台进程"
  local pid_file pid
  for pid_file in "$WORK_DIR/server.pid"; do
    [ -f "$pid_file" ] || continue
    pid="$(cat "$pid_file")"
    # ⚠️ **必须用 taskkill**：在 MSYS（Git Bash）里 `kill` 只对 MSYS 进程有效，
    #    对 Windows 原生进程**杀不掉** —— 服务端会继续占着端口，下一次 start
    #    就撞上「端口已被占用」，或者更糟：新进程起不来，旧的还在跑，
    #    压测打到一个配置不对的服务端上，数字全是错的。
    if taskkill //PID "$pid" //F >/dev/null 2>&1; then
      info "已停掉 pid $pid（$(basename "$pid_file")）"
    elif kill "$pid" 2>/dev/null; then
      info "已停掉 pid $pid（$(basename "$pid_file")）"
    else
      info "pid $pid 已经不在了"
    fi
    rm -f "$pid_file"
  done
  # 端口没释放就早失败：残留的服务端进程会让这一轮的数字毫无意义。
  local waited=0
  while port_in_use "$API_PORT"; do
    if [ "$waited" -ge 10 ]; then
      die "端口 ${API_PORT} 仍被占用 —— 有别的进程还在监听，先把它停掉"
    fi
    sleep 1
    waited=$((waited + 1))
  done
}

# cmd_start 只启动服务端（环境已经备好时用）。
#
# setup 之后改了二进制或配置，用它重启服务端，不必把整个环境推倒重来。
cmd_start() {
  [ -f "$WORK_DIR/state.env" ] || die "找不到 $WORK_DIR/state.env —— 先跑 setup"
  WORK_DIR_ABS="$(cd "$WORK_DIR" && pwd)"
  WORK_DIR_NATIVE="$(native_path "$WORK_DIR_ABS")"
  if port_in_use "$API_PORT"; then
    die "端口 ${API_PORT} 已被占用 —— 换个 API_PORT，或者先停掉占用的进程"
  fi
  log "启动服务端"
  # 迁移先跑：release 模式不在启动时改表（多副本会争 DDL 锁）。
  CONFIG_FILE="$WORK_DIR_NATIVE/config.yaml" \
    "$WORK_DIR_NATIVE/bin/server" -migrate >"$WORK_DIR_ABS/migrate.log" 2>&1 || {
      tail -n 20 "$WORK_DIR_ABS/migrate.log" >&2 || true
      die "迁移失败，看 $WORK_DIR_ABS/migrate.log"
    }
  CONFIG_FILE="$WORK_DIR_NATIVE/config.yaml" \
    "$WORK_DIR_NATIVE/bin/server" >>"$WORK_DIR_ABS/server.log" 2>&1 &
  echo $! >"$WORK_DIR_ABS/server.pid"
  if ! wait_for_http "$API_BASE/healthz" 30; then
    tail -n 30 "$WORK_DIR_ABS/server.log" >&2 || true
    die "服务端没起来，看 $WORK_DIR_ABS/server.log"
  fi
  info "healthz: $(curl -fsS "$API_BASE/healthz")"
}

case "${1:-}" in
  setup) cmd_setup ;;
  run) cmd_run ;;
  sweep) cmd_sweep ;;
  start) cmd_start ;;
  stop) cmd_stop ;;
  *)
    cat <<EOF
用法: bash ./scripts/capacity-run.sh <setup|run|sweep|start|stop>

  setup   备好容量压测环境（密钥 / 名单 / 库 / release 模式服务端）
  run     跑一轮压测（VOTES 张票，CONCURRENCY 并发），输出 JSON 报告
  sweep   按 CONCURRENCIES 依次跑并发扫描，打印对比表
  start   只启动服务端（改了二进制或配置后重启用）
  stop    停掉 setup 起的后台进程
EOF
    exit 2
    ;;
esac
