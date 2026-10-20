param(
    [double]$Threshold = 80.0,
    [string]$Profile = "coverage.out",
    [string[]]$Packages = @(),
    [string]$ThresholdFile = ""
)

$ErrorActionPreference = "Stop"

# 覆盖率门槛脚本（PowerShell 版）。
#
# 用法:
#   check-coverage.ps1 [-Threshold 80] [-Profile coverage.out] [-Packages ...]
#
#   不带 -Packages：按 scripts/coverage-thresholds.txt 里的分档阈值逐包判定，
#                   合计口径也按表里的 total 判定。
#   带 -Packages：所有包统一按 -Threshold 判定（用于单包排查或临时收紧）。
#
# 不覆盖 GOMODCACHE / GOCACHE：CI 里 actions/setup-go 已经把它们指向带缓存的
# 目录，脚本再改会让 cache 失效，并把整个模块集重新下载一遍。
# GOSUMDB 保持 off 是为了让离线/沙箱环境可用；go.sum 仍然逐个校验哈希。
if (-not $env:GOSUMDB) {
    $env:GOSUMDB = "off"
}

if (-not $ThresholdFile) {
    $ThresholdFile = if ($env:NULLIFIER_COVERAGE_THRESHOLDS) {
        $env:NULLIFIER_COVERAGE_THRESHOLDS
    } else {
        "scripts/coverage-thresholds.txt"
    }
}

$module = (go list -m).Trim()

$floors = @{}
$totalFloor = $null
$selected = @()

if ($Packages.Count -gt 0) {
    $selected = $Packages
} else {
    if (-not (Test-Path $ThresholdFile)) {
        Write-Error "Coverage threshold file not found: $ThresholdFile"
        exit 1
    }
    foreach ($line in Get-Content $ThresholdFile) {
        $trimmed = $line.Trim()
        if ($trimmed -eq "" -or $trimmed.StartsWith("#")) {
            continue
        }
        $parts = $trimmed -split '\s+'
        if ($parts[0] -eq "total") {
            $totalFloor = [double]$parts[1]
            continue
        }
        $floors[$parts[0]] = [double]$parts[1]
        $selected += "./$($parts[0])"
    }
}

go test "-coverprofile=$Profile" @selected
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

# 逐包统计：coverprofile 每行是
#   <import path>/<file>.go:<start>,<end> <语句数> <命中次数>
# 命中次数大于 0 的语句才算被覆盖。
$covered = @{}
$totals = @{}

foreach ($line in Get-Content $Profile) {
    if ($line.StartsWith("mode:")) {
        continue
    }
    $parts = $line -split ' '
    if ($parts.Count -lt 3) {
        continue
    }
    $path = $parts[0]
    if ($path.StartsWith("$module/")) {
        $path = $path.Substring($module.Length + 1)
    }
    $segments = $path -split '/'
    if ($segments.Length -lt 2) {
        continue
    }
    $dir = ($segments[0..($segments.Length - 2)]) -join '/'
    $stmt = [int]$parts[1]
    $count = [int]$parts[2]
    if (-not $totals.ContainsKey($dir)) {
        $totals[$dir] = 0
        $covered[$dir] = 0
    }
    $totals[$dir] += $stmt
    if ($count -gt 0) {
        $covered[$dir] += $stmt
    }
}

$failed = $false
$grandCovered = 0
$grandTotal = 0

foreach ($dir in ($totals.Keys | Sort-Object)) {
    $cov = $covered[$dir]
    $tot = $totals[$dir]
    $grandCovered += $cov
    $grandTotal += $tot

    $floor = if ($floors.ContainsKey($dir)) { $floors[$dir] } else { $Threshold }
    $pct = if ($tot -gt 0) { $cov * 100.0 / $tot } else { 0.0 }
    $bad = $tot -gt 0 -and $pct -lt $floor
    $status = if ($bad) { "未达标" } else { "达标" }
    Write-Host ("  {0,-44} {1,7:N2}%  门槛 {2,6:N2}%  {3}" -f $dir, $pct, $floor, $status)
    if ($bad) {
        $failed = $true
    }
}

$grandPct = if ($grandTotal -gt 0) { $grandCovered * 100.0 / $grandTotal } else { 0.0 }
$grandFloor = if ($null -ne $totalFloor) { $totalFloor } else { $Threshold }
$grandBad = $grandTotal -gt 0 -and $grandPct -lt $grandFloor
$grandStatus = if ($grandBad) { "未达标" } else { "达标" }
Write-Host ("  {0,-44} {1,7:N2}%  门槛 {2,6:N2}%  {3}" -f "合计", $grandPct, $grandFloor, $grandStatus)
if ($grandBad) {
    $failed = $true
}

if ($failed) {
    Write-Error "覆盖率门槛未达标"
    exit 1
}

Write-Host "覆盖率门槛达标"
