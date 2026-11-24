param(
    [int]$GatewayPort = 8080
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

Write-Host "Starting two legacy mock MCP servers..."
$weather = Start-Process -FilePath "go" -ArgumentList "run","./cmd/legacy-mock","-addr",":9001","-name","weather","-tools","get_weather,list_cities" -WorkingDirectory $root -PassThru -WindowStyle Hidden
$crm = Start-Process -FilePath "go" -ArgumentList "run","./cmd/legacy-mock","-addr",":9002","-name","crm","-tools","get_customer" -WorkingDirectory $root -PassThru -WindowStyle Hidden

Start-Sleep -Seconds 2
Write-Host "Starting dual-era gateway on :$GatewayPort..."
try {
    go run ./cmd/gateway -addr ":$GatewayPort" -legacy "weather=http://127.0.0.1:9001/mcp,crm=http://127.0.0.1:9002/mcp"
} finally {
    Stop-Process -Id $weather.Id -Force -ErrorAction SilentlyContinue
    Stop-Process -Id $crm.Id -Force -ErrorAction SilentlyContinue
}