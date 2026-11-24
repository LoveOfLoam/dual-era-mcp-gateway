param()
$ErrorActionPreference = "Stop"
if (-not $env:GOCACHE) { $env:GOCACHE = Join-Path $env:TEMP "gocache-dual-era" }
if (-not $env:GOMODCACHE) { $env:GOMODCACHE = Join-Path $env:TEMP "gomodcache-dual-era" }

$root = Split-Path -Parent $PSScriptRoot
$compose = Join-Path $root "deploy\t8\docker-compose.yml"
$haCompose = Join-Path $root "deploy\t8\docker-compose.redis-ha.yml"
$bin = Join-Path $root "deploy\t8\bin"
New-Item -ItemType Directory -Force -Path $bin | Out-Null

Push-Location $root
try {
    $oldGOOS = $env:GOOS; $oldGOARCH = $env:GOARCH; $oldCGO = $env:CGO_ENABLED
    $env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
    foreach ($cmd in @("gateway", "legacy-mock", "policy", "rest-mock", "sse-shim", "controlplane", "crd-controller")) {
        go build -trimpath -o (Join-Path $bin $cmd) "./cmd/$cmd"
        if ($LASTEXITCODE -ne 0) { throw "build $cmd failed" }
    }
    $env:GOOS = $oldGOOS; $env:GOARCH = $oldGOARCH; $env:CGO_ENABLED = $oldCGO
    if (-not $oldGOOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue }
    if (-not $oldGOARCH) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue }
    if (-not $oldCGO) { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue }
} finally { Pop-Location }

function PostJson($uri, $body, $headers = @{}) {
    $bodyFile = [IO.Path]::GetTempFileName()
    $hdr = [IO.Path]::GetTempFileName()
    $out = [IO.Path]::GetTempFileName()
    try {
        [IO.File]::WriteAllText($bodyFile, $body, [Text.UTF8Encoding]::new($false))
        $args = @("-sS", "-D", $hdr, "-o", $out, "-w", "%{http_code}", "-X", "POST", $uri,
            "-H", "Content-Type: application/json", "-H", "Accept: application/json, text/event-stream",
            "-H", "Authorization: Bearer test", "--data-binary", "@$bodyFile")
        foreach ($k in $headers.Keys) { $args += @("-H", "$k`: $($headers[$k])") }
        $code = & curl.exe @args 2>&1
        $text = if (Test-Path $out) { [IO.File]::ReadAllText($out) } else { "" }
        $sid = $null
        foreach ($line in Get-Content $hdr -ErrorAction SilentlyContinue) {
            if ($line -match '^(?i)mcp-session-id:\s*(.+)$') { $sid = $Matches[1].Trim() }
        }
        return @{ Status = [int]$code; Session = $sid; Body = $text }
    } finally { Remove-Item $bodyFile, $hdr, $out -Force -ErrorAction SilentlyContinue }
}

$up = $false
try {
    docker compose -f $compose -f $haCompose up -d --build
    if ($LASTEXITCODE -ne 0) { throw "compose up failed" }
    $up = $true
    for ($i = 0; $i -lt 40; $i++) {
        try {
            $r = Invoke-WebRequest -Uri "http://127.0.0.1:9901/ready" -UseBasicParsing -TimeoutSec 2
            if ($r.StatusCode -eq 200) { break }
        } catch { Start-Sleep -Seconds 2 }
    }
    Start-Sleep -Seconds 4

    $init = PostJson "http://127.0.0.1:8080/mcp" '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"redis-ha","version":"0"}}}'
    if ($init.Status -ne 200 -or -not $init.Session) { throw "initialize failed: $($init.Status) $($init.Body)" }
    $sid = $init.Session
    $call = '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"weather__get_weather","arguments":{"q":"Paris"}}}'
    $before = PostJson "http://127.0.0.1:8080/mcp" $call @{"Mcp-Session-Id" = $sid; "Mcp-Name" = "weather__get_weather"}
    if ($before.Status -ne 200) { throw "pre-failover call failed: $($before.Status)" }

    Write-Host "==> stopping redis-master"
    $ErrorActionPreference = "Continue"
    docker stop t8-redis-master-1 *> $null
    $ErrorActionPreference = "Stop"
    Start-Sleep -Seconds 15

    $master = (docker compose -f $compose -f $haCompose exec -T redis-sentinel redis-cli -p 26379 sentinel get-master-addr-by-name mymaster 2>&1 | Out-String).Trim()
    Write-Host "sentinel master: $master"
    if (-not $master) { throw "sentinel did not report a master" }
    if ($master -match 'redis-master') { throw "sentinel did not fail over away from redis-master" }
    $keys = (docker compose -f $compose -f $haCompose exec -T redis-replica redis-cli DBSIZE 2>&1 | Out-String).Trim()
    Write-Host "promoted replica keys: $keys"
    if (-not $keys -or [int]$keys -lt 1) { throw "promoted replica has no limiter keys" }

    $after = PostJson "http://127.0.0.1:8080/mcp" $call @{"Mcp-Session-Id" = $sid; "Mcp-Name" = "weather__get_weather"}
    Write-Host "post-failover call: status=$($after.Status)"
    if ($after.Status -ne 200) { throw "post-failover call failed: $($after.Status)" }

    Write-Host "T8 REDIS HA PASS: sentinel master=$master, limiter still serving"
} catch {
    Write-Host "T8 REDIS HA FAIL: $($_.Exception.Message)"
    exit 1
} finally {
    if ($up) {
        cmd /c "docker compose -f `"$compose`" -f `"$haCompose`" down -v >NUL 2>&1"
    }
}
exit 0