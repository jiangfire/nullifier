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

if (-not $env:GOMODCACHE) {
    $env:GOMODCACHE = Join-Path (Get-Location) ".gomodcache"
}
if (-not $env:GOCACHE) {
    $env:GOCACHE = Join-Path (Get-Location) ".gocache"
}
if (-not $env:GOSUMDB) {
    $env:GOSUMDB = "off"
}
New-Item -ItemType Directory -Force -Path $env:GOMODCACHE | Out-Null
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null

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
