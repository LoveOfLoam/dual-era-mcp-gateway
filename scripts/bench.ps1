$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$env:GOCACHE = Join-Path $env:TEMP "gocache-dual-era"
$env:GOMODCACHE = Join-Path $env:TEMP "gomodcache-dual-era"
New-Item -ItemType Directory -Force -Path $env:GOCACHE, $env:GOMODCACHE | Out-Null
Set-Location $root
go test ./internal/facade -run '^$' -bench . -benchmem -benchtime=1s