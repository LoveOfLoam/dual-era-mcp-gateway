param(
    [int]$Concurrency = 10,
    [int]$Seconds = 6
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
    foreach ($cmd in @("gateway", "legacy-mock", "policy", "rest-mock", "sse-shim", "controlplane")) {
        go build -trimpath -o (Join-Path $bin $cmd) "./cmd/$cmd"
        if ($LASTEXITCODE -ne 0) { throw "build $cmd failed" }
    }
    $env:GOOS = $oldGOOS; $env:GOARCH = $oldGOARCH; $env:CGO_ENABLED = $oldCGO
    if (-not $oldGOOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue }
    if (-not $oldGOARCH) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue }
    if (-not $oldCGO) { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue }
    $loadgen = Join-Path $env:TEMP ("loadgen-ha-" + $PID + ".exe")
    go build -o $loadgen ./cmd/loadgen
} finally { Pop-Location }

$up = $false
try {
    docker compose -f $compose up -d --build
    if ($LASTEXITCODE -ne 0) { throw "compose up failed" }
    $up = $true
    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $r = Invoke-WebRequest -Uri "http://127.0.0.1:9901/ready" -UseBasicParsing -TimeoutSec 2
            if ($r.StatusCode -eq 200) { $ready = $true; break }
        } catch { Start-Sleep -Seconds 2 }
    }
    if (-not $ready) { throw "public Envoy did not become ready" }
    Start-Sleep -Seconds 4

    $bodyFile = Join-Path $env:TEMP ("ha-body-" + $PID + ".json")
    [IO.File]::WriteAllText($bodyFile, '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}', [Text.UTF8Encoding]::new($false))
    $outFile = Join-Path $root "docs/benchmarks/t8-ha-load.json"
    $job = Start-Job -ScriptBlock {
        param($exe, $body, $out)
        & $exe -url "http://127.0.0.1:8080/modern/mcp" -method POST -body-file $body `
            -header "Content-Type: application/json" -header "Accept: application/json" `
            -header "Authorization: Bearer test" `
            -header "MCP-Protocol-Version: 2026-07-28" -header "Mcp-Method: tools/list" `
            -concurrency 10 -duration "6s" -out $out 2>&1
    } -ArgumentList $loadgen, $bodyFile, $outFile

    Start-Sleep -Seconds 2
    Write-Host "==> stopping one gateway replica (t8-gateway-b-1)"
    $ErrorActionPreference = "Continue"
    docker stop t8-gateway-b-1 *> $null
    $stopExit = $LASTEXITCODE
    $ErrorActionPreference = "Stop"
    if ($stopExit -ne 0) { throw "failed to stop gateway replica" }

    Wait-Job $job -Timeout 20 | Out-Null
    $loadOut = Receive-Job $job
    Remove-Job $job -Force -ErrorAction SilentlyContinue
    Write-Host "HA load: $($loadOut -join ' ')"
    if ($loadOut -match 'errors=[1-9]') { throw "load saw errors after replica stop" }

    Write-Host "T8 HA PASS: one gateway replica stopped under load, 0 errors"
} catch {
    Write-Host "T8 HA FAIL: $($_.Exception.Message)"
    exit 1
} finally {
    if ($up) {
        cmd /c "docker compose -f `"$compose`" down -v >NUL 2>&1"
    }
    Remove-Item $loadgen, $bodyFile, $outFile -Force -ErrorAction SilentlyContinue
}
exit 0