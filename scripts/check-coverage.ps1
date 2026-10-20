param(
    [double]$Threshold = 80.0,
    [string]$Profile = "coverage.out",
    [string[]]$Packages = @(
        "./internal/usecase",
        "./internal/config",
        "./internal/di",
        "./internal/infrastructure/http",
        "./internal/infrastructure/http/middleware"
    )
)

$ErrorActionPreference = "Stop"

# 不覆盖 GOMODCACHE / GOCACHE：CI 里 actions/setup-go 已经把它们指向带缓存的
# 目录，脚本再改会让 cache 失效，并把整个模块集重新下载一遍。
# GOSUMDB 保持 off 是为了让离线/沙箱环境可用；go.sum 仍然逐个校验哈希。
if (-not $env:GOSUMDB) {
    $env:GOSUMDB = "off"
}

go test -coverprofile=$Profile $Packages
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

$summary = go tool cover -func=$Profile | Select-String "total:"
if (-not $summary) {
    Write-Error "Cannot parse coverage summary."
    exit 1
}

$line = $summary.Line
if ($line -notmatch '([0-9]+(\.[0-9]+)?)%') {
    Write-Error "Unexpected coverage format: $line"
    exit 1
}

$coverage = [double]$matches[1]
if ($coverage -lt $Threshold) {
    Write-Error ("Coverage gate failed: {0}% < {1}%" -f $coverage, $Threshold)
    exit 1
}

Write-Host ("Coverage gate passed: {0}% >= {1}%" -f $coverage, $Threshold)
