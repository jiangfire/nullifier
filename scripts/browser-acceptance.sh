#!/usr/bin/env bash
#
# 浏览器验收脚本 —— 固化 docs/deployment-checklist.md §8 / §8.1 的手工流程。
#
# 仓库里没有浏览器自动化（docs/roadmap.md §4.3 盲区 2），所以「在真浏览器里点一遍」
# 这一步仍然要人来做。本脚本负责它前后的全部环节，避免每次上线都从零手搓：
#
#   setup   备好环境（密钥 / 名单 / 提案 / 凭证池 / 门户）并打印人工步骤
#   verify  人工点完之后核对服务端证据：日志里的关键事件 + 计票还原出所选选项
#   stop    停掉 setup 起的后台进程
#
# 用法：
#   bash ./scripts/browser-acceptance.sh setup
#   bash ./scripts/browser-acceptance.sh verify
#   bash ./scripts/browser-acceptance.sh stop
#
# 可调参数（都是环境变量）：
#   WORK_DIR=.acceptance        工作目录：密钥、名单、库、日志都在这里
#   API_PORT=18080              服务端端口
#   PORTAL_PORT=3000            门户端口
#   ADMIN_TOKEN=...             管理端 Bearer token
#   WINDOW_START_DELAY=180      多少秒后开窗（门限解密必须在开窗之前准备好）
#   WINDOW_MINUTES=15           投票窗口长度
#   POOL_SIZE=10                凭证池备几个槽位
#   OPTIONS=alpha,beta          提案选项
#   ACCEPT_OPTION=beta          verify 时期望被计票还原出来的那个选项
#   MEMBERS="alice bob carol"   名单成员（每人一份资格包）
#   PROPOSAL_TITLE=...          提案标题（用 ASCII：Git Bash 会把中文参数转成乱码）
#
# 这个脚本只在本地开发机上跑：它会生成密钥、名单和数据库，全部落在 WORK_DIR 里。

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

WORK_DIR="${WORK_DIR:-.acceptance}"
API_PORT="${API_PORT:-18080}"
PORTAL_PORT="${PORTAL_PORT:-3000}"
ADMIN_TOKEN="${ADMIN_TOKEN:-acceptance-admin-token}"
WINDOW_START_DELAY="${WINDOW_START_DELAY:-180}"
WINDOW_MINUTES="${WINDOW_MINUTES:-15}"
POOL_SIZE="${POOL_SIZE:-10}"
OPTIONS="${OPTIONS:-alpha,beta}"
ACCEPT_OPTION="${ACCEPT_OPTION:-beta}"
MEMBERS="${MEMBERS:-alice bob carol}"
# 提案标题用 ASCII：Git Bash 会把命令行参数按本地代码页转码，中文标题会变成乱码
# （msys 的 argv 转换）—— 那是本机 shell 的问题，不是服务端的。
PROPOSAL_TITLE="${PROPOSAL_TITLE:-browser acceptance}"

# 门户一律用 127.0.0.1：`localhost` 在很多机器上先命中 IPv6，会打到别的服务上。
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

# rfc3339_at 把 epoch 秒转成 RFC3339。GNU date 与 BSD date 都能用。
rfc3339_at() {
  local ts="$1"
  if date -u -d "@${ts}" +%Y-%m-%dT%H:%M:%SZ >/dev/null 2>&1; then
    date -u -d "@${ts}" +%Y-%m-%dT%H:%M:%SZ
  else
    date -u -r "${ts}" +%Y-%m-%dT%H:%M:%SZ
  fi
}

# json_string 从一段 JSON 里取某个字符串字段的值。
json_string() {
  printf '%s' "$1" | sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" | head -n1
}

# json_int 从一段 JSON 里取某个整数字段的值。
json_int() {
  printf '%s' "$1" | sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\([0-9]\{1,\}\).*/\1/p" | head -n1
}

# tally_of 从计票结果里取某个选项的票数。
tally_of() {
  printf '%s' "$1" | grep -o "\"$2\"[[:space:]]*:[[:space:]]*[0-9]\{1,\}" | head -n1 | grep -o '[0-9]\{1,\}$'
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
#
# 每次 curl 都要单独限时：不加 --max-time 的话，一次连不上的探测可能挂好几秒，
# 「等 30 秒」就会变成「等好几分钟」。
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

  if [ -e "$WORK_DIR" ]; then
    die "工作目录已存在: $WORK_DIR —— 先跑 stop 并删掉它，或换个 WORK_DIR"
  fi

  # 门户端口被占的话 rsbuild 会另起一个端口，脚本却还在等原来的那个 ——
  # 先说清楚，别让人对着「门户没起来」猜。
  if curl -fsS -o /dev/null --max-time 2 "http://127.0.0.1:${PORTAL_PORT}" 2>/dev/null; then
    die "门户端口 ${PORTAL_PORT} 已被占用 —— 换个 PORTAL_PORT 再跑（比如 PORTAL_PORT=3100）"
  fi

  mkdir -p "$WORK_DIR/keys" "$WORK_DIR/eligibility" "$WORK_DIR/bin"
  # 两套路径：bash 自己用的（MSYS 写法）与交给 Go 二进制用的（原生写法）。
  WORK_DIR_ABS="$(cd "$WORK_DIR" && pwd)"
  WORK_DIR_NATIVE="$(native_path "$WORK_DIR_ABS")"

  log "1/8 构建浏览器端 prover（WASM）"
  build_wasm
  info "web/public/ballot-prover.wasm + wasm_exec.js"

  log "2/8 生成两套密钥（选票 / 资格）"
  go build -o "$WORK_DIR_NATIVE/bin/keygen" ./cmd/keygen
  go build -o "$WORK_DIR_NATIVE/bin/server" ./cmd/server
  go build -o "$WORK_DIR_NATIVE/bin/cli" ./cmd/cli
  go build -o "$WORK_DIR_NATIVE/bin/eligibility-list" ./cmd/eligibility-list
  "$WORK_DIR_NATIVE/bin/keygen" -dir "$WORK_DIR_NATIVE/keys" -force >/dev/null
  "$WORK_DIR_NATIVE/bin/keygen" -circuit eligibility -dir "$WORK_DIR_NATIVE/keys" -force >/dev/null
  cp "$WORK_DIR_ABS/keys/proving.key" web/public/proving.key
  cp "$WORK_DIR_ABS/keys/eligibility-proving.key" web/public/eligibility-proving.key
  info "proving key 只在客户端：web/public/proving.key、web/public/eligibility-proving.key"

  log "3/8 生成资格名单（离线工具，名单根与成员包分开保管）"
  printf '%s\n' $MEMBERS >"$WORK_DIR_ABS/members.txt"
  "$WORK_DIR_NATIVE/bin/eligibility-list" -members "$WORK_DIR_NATIVE/members.txt" \
    -out "$WORK_DIR_NATIVE/eligibility" -force
  local root_hex
  root_hex="$(json_string "$(cat "$WORK_DIR_ABS/eligibility/root.json")" root)"
  [ -n "$root_hex" ] || die "读不出名单根：$WORK_DIR/eligibility/root.json"
  info "名单根: $root_hex"

  log "4/8 写验收用配置并启动服务端"
  cat >"$WORK_DIR_ABS/config.yaml" <<EOF
server:
  address: ":${API_PORT}"
  mode: "debug"
  admin_bearer_token: "${ADMIN_TOKEN}"
  cors_allowed_origins:
    - "http://127.0.0.1:${PORTAL_PORT}"
    - "http://localhost:${PORTAL_PORT}"
crypto:
  vk_path: "${WORK_DIR_NATIVE}/keys/verifying.key"
  issuer_key_path: "${WORK_DIR_NATIVE}/issuer_keys.json"
  eligibility_root: "${root_hex}"
  eligibility_vk_path: "${WORK_DIR_NATIVE}/keys/eligibility-verifying.key"
db:
  driver: "sqlite"
  path: "${WORK_DIR_NATIVE}/acceptance.db"
EOF
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

  log "5/8 启动门户 dev server"
  pnpm --dir web run dev --port "$PORTAL_PORT" >"$WORK_DIR_ABS/portal.log" 2>&1 &
  echo $! >"$WORK_DIR_ABS/portal.pid"
  if ! wait_for_http "$PORTAL_URL" 90; then
    tail -n 30 "$WORK_DIR_ABS/portal.log" >&2 || true
    die "门户没起来，看 $WORK_DIR_ABS/portal.log"
  fi
  info "门户在 $PORTAL_URL"

  # 提案放在最后建：窗口从建完那一刻开始算，前面每一步都不占用人工操作时间。
  log "6/8 建提案（先不开窗）、备凭证池、准备门限解密"
  local now starts_at ends_at proposal proposal_id
  now="$(date +%s)"
  starts_at="$(rfc3339_at $((now + WINDOW_START_DELAY)))"
  ends_at="$(rfc3339_at $((now + WINDOW_START_DELAY + WINDOW_MINUTES * 60)))"
  proposal="$(curl -fsS -X POST "$API_BASE/api/v1/proposals" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H 'Content-Type: application/json' \
    -d "{\"title\":\"${PROPOSAL_TITLE}\",\"description\":\"browser acceptance\",\"options\":$(options_json "$OPTIONS"),\"starts_at\":\"${starts_at}\",\"ends_at\":\"${ends_at}\"}")"
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

  cat >"$WORK_DIR_ABS/state.env" <<EOF
PROPOSAL_ID=${proposal_id}
STARTS_AT=${starts_at}
ENDS_AT=${ends_at}
ENDS_AT_EPOCH=$((now + WINDOW_START_DELAY + WINDOW_MINUTES * 60))
API_BASE=${API_BASE}
PORTAL_URL=${PORTAL_URL}
ACCEPT_OPTION=${ACCEPT_OPTION}
CLI_BIN=${WORK_DIR_NATIVE}/bin/cli
SHARES_FILE=${WORK_DIR_NATIVE}/shares.json
SERVER_LOG=${WORK_DIR_ABS}/server.log
EOF

  log "7/8 打印人工步骤"
  local first_member
  first_member="$(printf '%s\n' $MEMBERS | head -n1)"
  cat <<EOF
  门户：${PORTAL_URL}
  后端：${API_BASE}   （门户「连接配置」里的「后端地址」填这个，注意用 127.0.0.1）

  1. 打开门户 → 左侧「连接配置」把「后端地址」改成 ${API_BASE}
  2. 左侧「投票大厅」→ 右上角点「刷新」（提案是刚建的，不刷新列表是空的）
     → 在「${PROPOSAL_TITLE}」那张卡片上点「去投票」
     （这一步把提案带进各个面板，后面不用手填 proposal_id）
  3. 左侧「我的身份」→「导入资格文件」→ 选这个文件：
       ${WORK_DIR_NATIVE}/eligibility/eligibility-${first_member}.json
  4. 点「匿名领取」→ 等十几秒（浏览器要下载并初始化 prover）→ 徽标变成「已就绪」
  5. 再点一次「匿名领取」→ 应提示「这份资格已经领过票了 —— 一人一票」
  6. 左侧「参与投票」→ 选 ${ACCEPT_OPTION} →「在浏览器本地生成选票」→ 等十几秒
     →「提交匿名选票」
  7. 服务端日志出现 credential attested 与 vote submitted

  窗口 ${starts_at} → ${ends_at} 关闭。窗口关掉之后回来跑：

       bash ./scripts/browser-acceptance.sh verify

  出现 "Failed to execute 'postMessage' on 'Worker'" 就是门户的 bug，不要上线。
EOF

  log "8/8 验收环境就绪"
  info "人工点完之后跑 verify；跑完用 stop 收尾"
}

cmd_verify() {
  [ -f "$WORK_DIR/state.env" ] || die "找不到 $WORK_DIR/state.env —— 先跑 setup"
  WORK_DIR_ABS="$(cd "$WORK_DIR" && pwd)"
  # shellcheck disable=SC1090
  . "$WORK_DIR/state.env"

  log "1/4 服务端状态"
  local health
  health="$(curl -fsS "$API_BASE/healthz")" || die "连不上服务端 $API_BASE"
  info "healthz: $health"
  case "$health" in
    *'"eligibility":true'*) : ;;
    *) die "healthz 里 eligibility 不是 true" ;;
  esac

  log "2/4 服务端日志里的关键事件"
  local failed=0
  check_log() {
    if grep -q "$1" "$SERVER_LOG"; then
      info "✅ $1"
    else
      info "❌ $1 —— 服务端日志里没有"
      failed=1
    fi
  }
  check_log "credential attested"
  check_log "vote submitted"
  check_log "eligibility already used"
  [ "$failed" -eq 0 ] || die "日志里的证据不齐（上面打了 ❌ 的那些）—— 先看 $SERVER_LOG"

  log "3/4 等投票窗口关闭"
  local now
  now="$(date +%s)"
  if [ "$now" -lt "$ENDS_AT_EPOCH" ]; then
    local remain
    remain=$((ENDS_AT_EPOCH - now))
    info "还要等 ${remain} 秒（窗口 ${ENDS_AT} 关闭）"
    sleep "$remain"
  fi

  log "4/4 发布结果并核对计票"
  # 幂等：verify 跑第二次时结果已经发布过了，发布会被拒（RESULT_ALREADY_PUBLISHED），
  # 这时改成读已发布的结果来核对。
  if ! "$CLI_BIN" -base-url "$API_BASE" -admin-token "$ADMIN_TOKEN" \
    result-publish -proposal "$PROPOSAL_ID" -shares-file "$SHARES_FILE" \
    >"$WORK_DIR_ABS/result.json" 2>"$WORK_DIR_ABS/publish.err"; then
    info "发布结果失败：$(tail -n 1 "$WORK_DIR_ABS/publish.err")"
    info "改读已发布的结果"
    "$CLI_BIN" -base-url "$API_BASE" \
      result-get -proposal "$PROPOSAL_ID" >"$WORK_DIR_ABS/result.json"
  fi
  local result total accepted
  result="$(cat "$WORK_DIR_ABS/result.json")"
  total="$(json_int "$result" total_votes)"
  accepted="$(tally_of "$result" "$ACCEPT_OPTION")"
  info "result: $result"
  [ "$total" = "1" ] || die "total_votes 不是 1（实际 ${total:-?}）"
  [ "$accepted" = "1" ] || die "选项 ${ACCEPT_OPTION} 的票数不是 1（实际 ${accepted:-?}）"
  info "✅ 计票还原出 ${ACCEPT_OPTION}"
  info "浏览器验收通过。跑 stop 收尾。"
}

cmd_stop() {
  log "停掉后台进程"
  local pid_file pid
  for pid_file in "$WORK_DIR/server.pid" "$WORK_DIR/portal.pid"; do
    [ -f "$pid_file" ] || continue
    pid="$(cat "$pid_file")"
    if kill "$pid" 2>/dev/null; then
      info "已停掉 pid $pid（$(basename "$pid_file")）"
    else
      info "pid $pid 已经不在了"
    fi
    rm -f "$pid_file"
  done
}

case "${1:-}" in
  setup) cmd_setup ;;
  verify) cmd_verify ;;
  stop) cmd_stop ;;
  *)
    cat <<EOF
用法: bash ./scripts/browser-acceptance.sh <setup|verify|stop>

  setup   备好验收环境并打印人工步骤
  verify  人工点完之后核对服务端证据与计票结果
  stop    停掉 setup 起的后台进程
EOF
    exit 2
    ;;
esac
