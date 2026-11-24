param(
    [int]$Concurrency = 20,
    [int]$Seconds = 5,
    [string]$Out = "docs/benchmarks/t8-load.json"
)

$ErrorActionPreference = "Stop"
if (-not $env:GOCACHE) { $env:GOCACHE = Join-Path $env:TEMP "gocache-dual-era" }
if (-not $env:GOMODCACHE) { $env:GOMODCACHE = Join-Path $env:TEMP "gomodcache-dual-era" }

$root = Split-Path -Parent $PSScriptRoot
$compose = Join-Path $root "deploy\t8\docker-compose.yml"
$bin = Join-Path $root "deploy\t8\bin"
New-Item -ItemType Directory -Force -Path $bin | Out-Null

Push-Location $root
try {
    $oldGOOS = $env:GOOS; $oldGOARCH = $env:GOARCH; $oldCGO = $env:CGO_ENABLED
    $env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
    go build -trimpath -o (Join-Path $bin "gateway") ./cmd/gateway
    go build -trimpath -o (Join-Path $bin "legacy-mock") ./cmd/legacy-mock
    go build -trimpath -o (Join-Path $bin "policy") ./cmd/policy
    go build -trimpath -o (Join-Path $bin "rest-mock") ./cmd/rest-mock
    $env:GOOS = $oldGOOS; $env:GOARCH = $oldGOARCH; $env:CGO_ENABLED = $oldCGO
    if (-not $oldGOOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue }
    if (-not $oldGOARCH) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue }
    if (-not $oldCGO) { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue }
    $loadgen = Join-Path $env:TEMP ("loadgen-" + $PID + ".exe")
    go build -o $loadgen ./cmd/loadgen
} finally { Pop-Location }

$up = $false
try {
    docker compose -f $compose up -d --build
    if ($LASTEXITCODE -ne 0) { throw "compose up failed" }
    $up = $true

    for ($i = 0; $i -lt 30; $i++) {
        try {
            $ready = Invoke-WebRequest -Uri "http://127.0.0.1:9901/ready" -UseBasicParsing -TimeoutSec 2
            if ($ready.StatusCode -eq 200) { break }
        } catch { Start-Sleep -Seconds 2 }
    }
    Start-Sleep -Seconds 3

    $bodyFile = Join-Path $env:TEMP ("t8-load-body-" + $PID + ".json")
    $body = '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}'
    [IO.File]::WriteAllText($bodyFile, $body, [Text.UTF8Encoding]::new($false))
    try {
        & $loadgen `
            -url "http://127.0.0.1:8080/modern/mcp" `
            -method POST `
            -body-file $bodyFile `
            -header "Content-Type: application/json" `
            -header "Accept: application/json" `
            -header "Authorization: Bearer test" `
            -header "MCP-Protocol-Version: 2026-07-28" `
            -header "Mcp-Method: tools/list" `
            -concurrency $Concurrency `
            -duration "${Seconds}s" `
            -out (Join-Path $root $Out)
        if ($LASTEXITCODE -ne 0) { throw "loadgen failed" }
    } finally {
        Remove-Item $bodyFile -Force -ErrorAction SilentlyContinue
        Remove-Item $loadgen -Force -ErrorAction SilentlyContinue
    }
} finally {
    if ($up) {
        $ErrorActionPreference = "Continue"
        docker compose -f $compose down -v *> $null
    }
}
exit 0