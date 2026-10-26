#!/usr/bin/env bash
#
# 门户 UI 自动化脚本（T28）—— 用**真浏览器**把门户的两条路径走一遍。
#
# 与 browser-acceptance.sh 的区别：那个脚本把「在真浏览器里点一遍」这件事
# **留给人**（中间那一下点击仍然要人来做）；这个脚本用 Playwright 把点击也做掉，
# 于是「门户的浏览器端路径」第一次有了自动化回归。
#
# 它跑的是**真实链路**：
#   - 真 WASM prover（浏览器里下载并编译电路）
#   - 真盲签名（两段式：预留 → 盲签名 → 解盲 → attest）
#   - 真资格证明（一人一票，重复领取被拒）
#   - release 模式的服务端（真 verifying key 验签、强制一人一票）
#
# 用法：
#   bash ./scripts/web-e2e.sh run     一条龙：备环境 → 跑用例 → 收尾（CI 用这个）
#   bash ./scripts/web-e2e.sh setup   只备好环境并把参数写进 state.env
#   bash ./scripts/web-e2e.sh test    在备好的环境上跑用例
#   bash ./scripts/web-e2e.sh stop    停掉 setup 起的后台进程
#
# 可调参数（都是环境变量）：
#   WORK_DIR=.e2e            工作目录：密钥、名单、库、日志、份额
#   API_PORT=18095           服务端端口
#   PORTAL_PORT=3100         门户端口
#   ADMIN_TOKEN=...          管理端 Bearer token
#   POOL_SIZE=5              凭证池备几个槽位
#   OPTIONS=alpha,beta       提案选项
#   ACCEPT_OPTION=beta       本次要投的选项（用例要核对计票还原出它）
#   MEMBERS="alice bob"      名单成员（每人一份资格包）
#   PROPOSAL_TITLE=e2e proposal  提案标题（用 ASCII：Git Bash 会把中文参数转成乱码）
#   WINDOW_OPEN_DELAY=45     投票窗口距现在多少秒打开（秒）
#   WINDOW_LENGTH=90         投票窗口开多久（秒）
#
#   DB_DRIVER=sqlite         数据库驱动：sqlite（默认）或 postgres
#   PG_HOST=127.0.0.1        postgres 主机（DB_DRIVER=postgres 时用）
#   PG_PORT=5432             postgres 端口
#   PG_USER=postgres         postgres 用户
#   PG_PASSWORD=...          postgres 密码
#   PG_DATABASE=anonvote_e2e postgres 库名（**必须已经存在**，脚本不建库）
#   PG_SSLMODE=disable       postgres sslmode
#
# ⚠️ **窗口的时长是有讲究的**：门户的「计票」要求提案**已关闭**（服务端只认
#    ends_at 过去了），而「投票」要求窗口**开着**。所以窗口必须先开、后关，
#    中间那段要装得下「浏览器端出证明」（十几秒）。窗口太短会偶发失败，
#    太长则整轮 e2e 变慢 —— 90 秒是在本机实测过的稳妥值。
#
# ⚠️ **限流额度要拉高**：一整轮 e2e 的请求全来自 127.0.0.1，共享一个来源额度。
#    用默认额度（凭证 10 rps / 40 burst）时，领取链路连着几个请求就可能被
#    自己挡下 —— 那测到的是限流器，不是门户。限流本身另有 HTTP 级用例。

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

WORK_DIR="${WORK_DIR:-.e2e}"
API_PORT="${API_PORT:-18095}"
PORTAL_PORT="${PORTAL_PORT:-3100}"
ADMIN_TOKEN="${ADMIN_TOKEN:-e2e-admin-token}"
POOL_SIZE="${POOL_SIZE:-5}"
OPTIONS="${OPTIONS:-alpha,beta}"
ACCEPT_OPTION="${ACCEPT_OPTION:-beta}"
MEMBERS="${MEMBERS:-alice bob}"
PROPOSAL_TITLE="${PROPOSAL_TITLE:-e2e proposal}"
WINDOW_OPEN_DELAY="${WINDOW_OPEN_DELAY:-45}"
WINDOW_LENGTH="${WINDOW_LENGTH:-90}"

DB_DRIVER="${DB_DRIVER:-sqlite}"
PG_HOST="${PG_HOST:-127.0.0.1}"
PG_PORT="${PG_PORT:-5432}"
PG_USER="${PG_USER:-postgres}"
PG_PASSWORD="${PG_PASSWORD:-}"
PG_DATABASE="${PG_DATABASE:-anonvote_e2e}"
PG_SSLMODE="${PG_SSLMODE:-disable}"

# 门户一律用 127.0.0.1：localhost 在很多机器上先命中 IPv6，会打到别的服务上。
API_BASE="http://127.0.0.1:${API_PORT}"
PORTAL_URL="http://127.0.0.1:${PORTAL_PORT}"

# SQLite 驱动依赖 cgo。Windows 上 Go 默认关掉 cgo —— 服务端会直接报
# "Binary was compiled with 'CGO_ENABLED=0'" 起不来（与 Makefile 同一口径）。
case "$(uname -s 2>/dev/null || echo unknown)" in
  MINGW* | MSYS* | CYGWIN*) export CGO_ENABLED=1 ;;
esac

# native_path 把路径转成本机原生写法：Git Bash 下 /c/x → C:/x。
#
# 必须转：脚本在 Git Bash 里跑，但服务端 / CLI 都是 Windows 原生二进制，
# 它们会把 /c/Users/... 当成 C:\c\Users\... —— 表现为「数据库文件打不开」。
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

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "缺少命令: $1"
}

# port_in_use 检查端口有没有被占。
#
# **必须查**：端口被占时健康检查会打到**别人**的服务上 —— 于是用例跑在一个
# 名单根不同、配置不同的服务端上，结论全是错的（而且没人会发现）。
port_in_use() {
  local port="$1"
  if command -v netstat >/dev/null 2>&1; then
    netstat -an 2>/dev/null | grep -qE "[:.]${port}[[:space:]].*LISTEN" && return 0
  fi
  return 1
}

# rfc3339_at 把 epoch 秒转成 RFC3339。GNU date 与 BSD date 都能用。
rfc3339_at() {
  local ts="$1"
  if date -u -d "@${ts}" +%Y-%m-%dT%H:%M:%SZ >/dev/null 2>&1; then
    date -u -d "@${ts}" +%Y-%m-%dT%H:%M:%SZ
  else
    date -u -r "@${ts}" +%Y-%m-%dT%H:%M:%SZ
  fi
}

# json_string 从一段 JSON 里取某个字符串字段的值。
json_string() {
  printf '%s' "$1" | sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" | head -n1
}

# options_json 把逗号分隔的选项变成 JSON 数组。
options_json() {
  local out="["
  local sep=""
  local opt
  for opt in ${1//,/ }; do
    out="${out}${sep}\"${opt}\""
    sep=","
  done
  printf '%s]' "${out}"
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

# build_wasm 构建浏览器端 prover 并把 Go 官方胶水拷进静态目录。
# 等价于 `make web-wasm`，这里自己拼是为了不依赖 make（Windows 上是 mingw32-make）。
build_wasm() {
  mkdir -p web/public
  GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o web/public/ballot-prover.wasm ./cmd/ballot-wasm
  cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/public/wasm_exec.js
}

cmd_setup() {
  require_cmd go
  require_cmd curl
  require_cmd pnpm

  if port_in_use "$API_PORT"; then
    die "服务端端口 ${API_PORT} 已被占用 —— 换个 API_PORT 再跑"
  fi
  if port_in_use "$PORTAL_PORT"; then
    die "门户端口 ${PORTAL_PORT} 已被占用 —— 换个 PORTAL_PORT 再跑"
  fi

  if [ -e "$WORK_DIR" ] && [ -n "$(ls -A "$WORK_DIR" 2>/dev/null)" ]; then
    die "工作目录已存在且非空: $WORK_DIR —— 先跑 stop 并删掉它，或换个 WORK_DIR"
  fi

  mkdir -p "$WORK_DIR/keys" "$WORK_DIR/eligibility" "$WORK_DIR/bin"
  WORK_DIR_ABS="$(cd "$WORK_DIR" && pwd)"
  WORK_DIR_NATIVE="$(native_path "$WORK_DIR_ABS")"

  log "1/8 构建浏览器端 prover（WASM）"
  build_wasm
  info "web/public/ballot-prover.wasm + wasm_exec.js"

  log "2/8 构建二进制"
  go build -o "$WORK_DIR_NATIVE/bin/keygen" ./cmd/keygen
  go build -o "$WORK_DIR_NATIVE/bin/server" ./cmd/server
  go build -o "$WORK_DIR_NATIVE/bin/cli" ./cmd/cli
  go build -o "$WORK_DIR_NATIVE/bin/eligibility-list" ./cmd/eligibility-list
  info "keygen / server / cli / eligibility-list"

  log "3/8 生成两套密钥（选票 / 资格）"
  "$WORK_DIR_NATIVE/bin/keygen" -dir "$WORK_DIR_NATIVE/keys" -force >/dev/null
  "$WORK_DIR_NATIVE/bin/keygen" -circuit eligibility -dir "$WORK_DIR_NATIVE/keys" -force >/dev/null
  # proving key 只在客户端：门户要现下载它来出证明。
  cp "$WORK_DIR_ABS/keys/proving.key" web/public/proving.key
  cp "$WORK_DIR_ABS/keys/eligibility-proving.key" web/public/eligibility-proving.key
  info "proving key 只在客户端：web/public/proving.key、web/public/eligibility-proving.key"

  log "4/8 生成资格名单（离线工具，名单根与成员包分开保管）"
  printf '%s\n' $MEMBERS >"$WORK_DIR_ABS/members.txt"
  "$WORK_DIR_NATIVE/bin/eligibility-list" -members "$WORK_DIR_NATIVE/members.txt" \
    -out "$WORK_DIR_NATIVE/eligibility" -force >/dev/null
  local root_hex
  root_hex="$(json_string "$(cat "$WORK_DIR_ABS/eligibility/root.json")" root)"
  [ -n "$root_hex" ] || die "读不出名单根：$WORK_DIR/eligibility/root.json"
  info "名单根: $root_hex"

  log "5/8 写 release 模式的配置并启动服务端"
  # release 模式：**强制**一人一票（资格根与资格验证密钥成对）、强制 admin
  # 令牌、强制限流。门户 e2e 要的就是这个形态 —— 开发模式（不验签、
  # 不校验资格）测出来的通过不能说明生产能过。
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
  sslmode: \"${PG_SSLMODE}\""
    info "数据库: PostgreSQL ${PG_HOST}:${PG_PORT}/${PG_DATABASE}"
  else
    db_block="db:
  driver: \"sqlite\"
  path: \"${WORK_DIR_NATIVE}/e2e.db\""
    info "数据库: SQLite ${WORK_DIR_NATIVE}/e2e.db"
  fi
  cat >"$WORK_DIR_ABS/config.yaml" <<EOF
server:
  address: ":${API_PORT}"
  mode: "release"
  admin_bearer_token: "${ADMIN_TOKEN}"
  cors_allowed_origins:
    - "${PORTAL_URL}"
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
    rps: 1000
    burst: 2000
  votes:
    rps: 1000
    burst: 2000
  read:
    rps: 1000
    burst: 2000
${db_block}
EOF
  # 迁移先跑：release 模式不在启动时改表（多副本会争 DDL 锁）。
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

  log "6/8 启动门户 dev server"
  # 用 exec 形式而不是 `pnpm run dev`：package.json 里的脚本带 --open，
  # 在 CI 上会试图弹浏览器。
  pnpm --dir web exec rsbuild dev --host 127.0.0.1 --port "$PORTAL_PORT" \
    >"$WORK_DIR_ABS/portal.log" 2>&1 &
  echo $! >"$WORK_DIR_ABS/portal.pid"
  if ! wait_for_http "$PORTAL_URL" 120; then
    tail -n 30 "$WORK_DIR_ABS/portal.log" >&2 || true
    die "门户没起来，看 $WORK_DIR_ABS/portal.log"
  fi
  info "门户在 $PORTAL_URL"

  log "7/8 建提案、备凭证池、准备门限解密"
  local now starts_at ends_at proposal proposal_id
  now="$(date +%s)"
  starts_at="$(rfc3339_at $((now + WINDOW_OPEN_DELAY)))"
  ends_at="$(rfc3339_at $((now + WINDOW_OPEN_DELAY + WINDOW_LENGTH)))"
  proposal="$(curl -fsS -X POST "$API_BASE/api/v1/proposals" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H 'Content-Type: application/json' \
    -d "{\"title\":\"${PROPOSAL_TITLE}\",\"description\":\"portal e2e\",\"options\":$(options_json "$OPTIONS"),\"starts_at\":\"${starts_at}\",\"ends_at\":\"${ends_at}\"}")"
  proposal_id="$(json_string "$proposal" id)"
  [ -n "$proposal_id" ] || die "建提案失败: $proposal"
  info "提案 $proposal_id：${starts_at} → ${ends_at}"

  curl -fsS -X POST "$API_BASE/api/v1/proposals/$proposal_id/credentials/pool" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H 'Content-Type: application/json' \
    -d "{\"count\":${POOL_SIZE}}" >/dev/null
  info "凭证池备了 ${POOL_SIZE} 个槽位"

  # 门限解密必须在投票窗口开始之前准备好，否则服务端会拒绝。
  "$WORK_DIR_NATIVE/bin/cli" -base-url "$API_BASE" -admin-token "$ADMIN_TOKEN" \
    decrypt-setup -proposal "$proposal_id" -out "$WORK_DIR_NATIVE/shares.json" >/dev/null
  info "解密份额已存到 $WORK_DIR_ABS/shares.json"

  log "8/8 环境就绪"
  local first_member
  first_member="$(printf '%s\n' $MEMBERS | head -n1)"
  cat >"$WORK_DIR_ABS/state.env" <<EOF
E2E_PORTAL_URL="${PORTAL_URL}"
E2E_API_BASE="${API_BASE}"
E2E_ADMIN_TOKEN="${ADMIN_TOKEN}"
E2E_PROPOSAL_ID="${proposal_id}"
E2E_PROPOSAL_TITLE="${PROPOSAL_TITLE}"
E2E_STARTS_AT="${starts_at}"
E2E_ENDS_AT="${ends_at}"
E2E_ELIGIBILITY_FILE="${WORK_DIR_NATIVE}/eligibility/eligibility-${first_member}.json"
E2E_SHARES_FILE="${WORK_DIR_NATIVE}/shares.json"
E2E_ACCEPT_OPTION="${ACCEPT_OPTION}"
EOF
  info "跑用例：bash ./scripts/web-e2e.sh test"
  info "收尾：  bash ./scripts/web-e2e.sh stop"
}

cmd_test() {
  [ -f "$WORK_DIR/state.env" ] || die "找不到 $WORK_DIR/state.env —— 先跑 setup"
  # shellcheck disable=SC1090
  . "$WORK_DIR/state.env"
  export E2E_PORTAL_URL E2E_API_BASE E2E_ADMIN_TOKEN E2E_PROPOSAL_ID \
    E2E_PROPOSAL_TITLE E2E_STARTS_AT E2E_ENDS_AT E2E_ELIGIBILITY_FILE \
    E2E_SHARES_FILE E2E_ACCEPT_OPTION

  log "跑 Playwright 用例"
  pnpm --dir web exec playwright test "$@"

  # 服务端侧的证据：用例断言的是页面上的票数，服务端日志是另一份独立证据。
  if grep -q "vote submitted" "$WORK_DIR/server.log"; then
    info "✅ 服务端日志里有 vote submitted"
  else
    die "服务端日志里没有 vote submitted —— 看 $WORK_DIR/server.log"
  fi
}

cmd_stop() {
  log "停掉后台进程"
  local pid_file pid
  for pid_file in "$WORK_DIR/server.pid" "$WORK_DIR/portal.pid"; do
    [ -f "$pid_file" ] || continue
    pid="$(cat "$pid_file")"
    # ⚠️ **必须用 taskkill**：在 MSYS（Git Bash）里 `kill` 只对 MSYS 进程有效，
    #    对 Windows 原生进程**杀不掉** —— 服务端会继续占着端口，下一次跑
    #    就撞上「端口已被占用」。
    if taskkill //PID "$pid" //F >/dev/null 2>&1; then
      info "已停掉 pid $pid（$(basename "$pid_file")）"
    elif kill "$pid" 2>/dev/null; then
      info "已停掉 pid $pid（$(basename "$pid_file")）"
    else
      info "pid $pid 已经不在了"
    fi
    rm -f "$pid_file"
  done
}

# cmd_run 一条龙：备环境 → 跑用例 → 收尾。
cmd_run() {
  # 收尾一定要跑：跑崩了留着的服务端会占着端口，下一次就撞端口。
  trap cmd_stop EXIT
  cmd_setup
  cmd_test "$@"
}

case "${1:-run}" in
  setup) cmd_setup ;;
  test) shift || true; cmd_test "$@" ;;
  stop) cmd_stop ;;
  run) shift || true; cmd_run "$@" ;;
  *)
    cat <<EOF
用法: bash ./scripts/web-e2e.sh <run|setup|test|stop>

  run    一条龙：备环境 → 跑用例 → 收尾（CI 用这个）
  setup  只备好环境（密钥 / 名单 / 服务端 / 门户 / 提案 / 凭证池 / 解密会话）
  test   在备好的环境上跑 Playwright 用例
  stop   停掉 setup 起的后台进程
EOF
    exit 2
    ;;
esac
