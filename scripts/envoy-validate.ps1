$ErrorActionPreference = "Stop"

$envoy = Get-Command envoy -ErrorAction SilentlyContinue
if (-not $envoy) {
    Write-Host "SKIP: envoy binary not installed. Install Envoy or run T8 in Docker."
    exit 0
}

$config = Join-Path $PSScriptRoot "..\deploy\envoy\bootstrap.yaml"
Write-Host "==> envoy --mode validate -c $config"
& $envoy.Source --mode validate -c $config
exit $LASTEXITCODE