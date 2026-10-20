#!/usr/bin/env bash
set -euo pipefail

# 覆盖率门槛脚本。
#
# 用法:
#   check-coverage.sh [统一阈值] [输出文件] [包...]
#
#   不带包列表：按 scripts/coverage-thresholds.txt 里的分档阈值逐包判定，
#               合计口径也按表里的 total 判定。
#   带包列表：所有包统一按「统一阈值」判定（用于单包排查或临时收紧）。
#
# 不覆盖 GOMODCACHE / GOCACHE：CI 里 actions/setup-go 已经把它们指向带缓存的
# 目录，脚本再改成工作区内的目录会让 cache: true 失效，并把整个模块集
# 重新下载一遍。
# GOSUMDB 保持 off 是为了让离线/沙箱环境可用；go.sum 仍然逐个校验哈希。
export GOSUMDB="${GOSUMDB:-off}"

THRESHOLD="${1:-80}"
PROFILE="${2:-coverage.out}"
THRESHOLD_FILE="${NULLIFIER_COVERAGE_THRESHOLDS:-scripts/coverage-thresholds.txt}"

MODULE="$(go list -m)"

declare -a PACKAGES=()
declare -A FLOOR=()
TOTAL_FLOOR=""

if [[ "$#" -gt 2 ]]; then
    PACKAGES=("${@:3}")
else
    if [[ ! -f "${THRESHOLD_FILE}" ]]; then
        echo "找不到覆盖率阈值文件: ${THRESHOLD_FILE}" >&2
        exit 1
    fi
    while read -r name floor _; do
        if [[ -z "${name}" || "${name}" == \#* ]]; then
            continue
        fi
        if [[ "${name}" == "total" ]]; then
            TOTAL_FLOOR="${floor}"
            continue
        fi
        PACKAGES+=("./${name}")
        FLOOR["${name}"]="${floor}"
    done < "${THRESHOLD_FILE}"
fi

go test -coverprofile="${PROFILE}" "${PACKAGES[@]}"

# 逐包统计：coverprofile 每行是
#   <import path>/<file>.go:<start>,<end> <语句数> <命中次数>
# 命中次数大于 0 的语句才算被覆盖。
COVERED_FILE="$(mktemp)"
FLOOR_FILE="$(mktemp)"
trap 'rm -f "${COVERED_FILE}" "${FLOOR_FILE}"' EXIT

awk -v mod="${MODULE}" '
    NR == 1 && $1 == "mode:" { next }
    NF < 3 { next }
    {
        path = $1
        if (index(path, mod "/") == 1) {
            path = substr(path, length(mod) + 2)
        }
        n = split(path, parts, "/")
        dir = parts[1]
        for (i = 2; i < n; i++) {
            dir = dir "/" parts[i]
        }
        total[dir] += $2
        if ($3 + 0 > 0) {
            covered[dir] += $2
        }
    }
    END {
        for (d in total) {
            printf "%s %d %d\n", d, covered[d], total[d]
        }
    }
' "${PROFILE}" > "${COVERED_FILE}"

for name in "${!FLOOR[@]}"; do
    echo "${name} ${FLOOR[${name}]}" >> "${FLOOR_FILE}"
done

if ! awk -v floorfile="${FLOOR_FILE}" -v uniform="${THRESHOLD}" -v totalfloor="${TOTAL_FLOOR}" '
    BEGIN {
        while ((getline line < floorfile) > 0) {
            split(line, parts, " ")
            floor[parts[1]] = parts[2] + 0
        }
        close(floorfile)
        tf = (totalfloor != "") ? totalfloor + 0 : uniform + 0
    }
    {
        dir = $1
        covered = $2 + 0
        total = $3 + 0
        f = (dir in floor) ? floor[dir] : uniform + 0
        pct = (total > 0) ? covered * 100.0 / total : 0
        bad = (total > 0 && pct + 0.0001 < f)
        printf "  %-44s %7.2f%%  门槛 %6.2f%%  %s\n", dir, pct, f, (bad ? "未达标" : "达标")
        if (bad) { failed = 1 }
        gc += covered
        gt += total
    }
    END {
        gpct = (gt > 0) ? gc * 100.0 / gt : 0
        bad = (gt > 0 && gpct + 0.0001 < tf)
        printf "  %-44s %7.2f%%  门槛 %6.2f%%  %s\n", "合计", gpct, tf, (bad ? "未达标" : "达标")
        if (bad) { failed = 1 }
        exit (failed ? 1 : 0)
    }
' "${COVERED_FILE}"; then
    echo "覆盖率门槛未达标" >&2
    exit 1
fi

echo "覆盖率门槛达标"
