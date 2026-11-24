$ErrorActionPreference = "Stop"

$helm = Get-Command helm -ErrorAction SilentlyContinue
if (-not $helm) {
    Write-Host "SKIP: helm not installed; install Helm to lint/template the chart."
    exit 0
}

$root = Split-Path -Parent $PSScriptRoot
$chart = Join-Path $root "deploy\helm\dual-era"
Write-Host "==> helm lint $chart"
helm lint $chart
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "==> helm template"
helm template dual-era $chart *> $null
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "Helm chart OK"