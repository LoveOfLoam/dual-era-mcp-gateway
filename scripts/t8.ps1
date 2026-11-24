param([switch]$TLS)
$ErrorActionPreference = "Stop"
if (-not $env:GOCACHE) { $env:GOCACHE = Join-Path $env:TEMP "gocache-dual-era" }
if (-not $env:GOMODCACHE) { $env:GOMODCACHE = Join-Path $env:TEMP "gomodcache-dual-era" }

$root = Split-Path -Parent $PSScriptRoot
$compose = Join-Path $root "deploy\t8\docker-compose.yml"
$composeArgs = @("-f", $compose)
$baseUrl = "http://127.0.0.1:8080"

function Build-T8Binaries {
    $bin = Join-Path $root "deploy\t8\bin"
    New-Item -ItemType Directory -Force -Path $bin | Out-Null
    $oldGOOS = $env:GOOS
    $oldGOARCH = $env:GOARCH
    $oldCGO = $env:CGO_ENABLED
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"
    Push-Location $root
    try {
        go build -trimpath -o (Join-Path $bin "gateway") ./cmd/gateway
        if ($LASTEXITCODE -ne 0) { throw "build gateway failed" }
        go build -trimpath -o (Join-Path $bin "legacy-mock") ./cmd/legacy-mock
        if ($LASTEXITCODE -ne 0) { throw "build legacy-mock failed" }
        go build -trimpath -o (Join-Path $bin "policy") ./cmd/policy
        if ($LASTEXITCODE -ne 0) { throw "build policy failed" }
        go build -trimpath -o (Join-Path $bin "rest-mock") ./cmd/rest-mock
        if ($LASTEXITCODE -ne 0) { throw "build rest-mock failed" }
        go build -trimpath -o (Join-Path $bin "sse-shim") ./cmd/sse-shim
        if ($LASTEXITCODE -ne 0) { throw "build sse-shim failed" }
        go build -trimpath -o (Join-Path $bin "controlplane") ./cmd/controlplane
        if ($LASTEXITCODE -ne 0) { throw "build controlplane failed" }
        go build -trimpath -o (Join-Path $bin "crd-controller") ./cmd/crd-controller
        if ($LASTEXITCODE -ne 0) { throw "build crd-controller failed" }
    } finally {
        Pop-Location
        if ($oldGOOS) { $env:GOOS = $oldGOOS } else { Remove-Item Env:GOOS -ErrorAction SilentlyContinue }
        if ($oldGOARCH) { $env:GOARCH = $oldGOARCH } else { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue }
        if ($oldCGO) { $env:CGO_ENABLED = $oldCGO } else { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue }
    }
}

Build-T8Binaries
$sseExe = Join-Path $env:TEMP ("ssecheck-" + $PID + ".exe")
Push-Location $root
try {
    go build -o $sseExe ./cmd/ssecheck
    if ($LASTEXITCODE -ne 0) { throw "ssecheck build failed" }
} finally { Pop-Location }
if ($TLS) {
    & (Join-Path $PSScriptRoot "gen-dev-cert.ps1") | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "dev certificate generation failed" }
    $composeArgs += @("-f", (Join-Path $root "deploy\t8\docker-compose.tls.yml"))
    $baseUrl = "https://127.0.0.1:8443"
    Push-Location $root
    try {
        $h2Exe = Join-Path $env:TEMP ("h2check-" + $PID + ".exe")
        go build -o $h2Exe ./cmd/h2check
        if ($LASTEXITCODE -ne 0) { throw "h2check build failed" }
    } finally { Pop-Location }
}

$docker = Get-Command docker -ErrorAction SilentlyContinue
if (-not $docker) {
    Write-Host "SKIP: docker CLI not installed; T8 needs Docker + Envoy image."
    exit 0
}

try {
    docker info *> $null
    if ($LASTEXITCODE -ne 0) { throw "docker daemon unavailable" }
    docker compose version *> $null
    if ($LASTEXITCODE -ne 0) { throw "docker compose plugin unavailable" }
} catch {
    Write-Host "SKIP: Docker daemon/Compose unavailable: $($_.Exception.Message)"
    exit 0
}

function Invoke-JsonPost($Uri, $Body, $Headers = @{}) {
    # curl + a body file avoids Windows PowerShell's JSON quoting/BOM problems.
    $bodyFile = [IO.Path]::GetTempFileName()
    $headerFile = [IO.Path]::GetTempFileName()
    $outFile = [IO.Path]::GetTempFileName()
    try {
        [IO.File]::WriteAllText($bodyFile, $Body, [Text.UTF8Encoding]::new($false))
        $curlArgs = @(
            "-sS", "-k", "--max-time", "15",
            "-D", $headerFile, "-o", $outFile, "-w", "%{http_code}",
            "-X", "POST", $Uri,
            "-H", "Content-Type: application/json",
            "-H", "Accept: application/json, text/event-stream",
            "-H", "Authorization: Bearer test",
            "--data-binary", "@$bodyFile"
        )
        foreach ($k in $Headers.Keys) { $curlArgs += @("-H", "$k`: $($Headers[$k])") }
        $statusText = (& curl.exe @curlArgs 2>&1 | Out-String).Trim()
        $status = 0
        [void][int]::TryParse($statusText, [ref]$status)
        $text = if (Test-Path $outFile) { [IO.File]::ReadAllText($outFile) } else { "" }
        $session = $null
        foreach ($line in Get-Content $headerFile -ErrorAction SilentlyContinue) {
            if ($line -match '^(?i)mcp-session-id:\s*(.+)$') { $session = $Matches[1].Trim() }
        }
        $json = $null
        try { $json = $text | ConvertFrom-Json } catch { }
        return @{ Status = $status; Session = $session; Json = $json; Body = $text }
    } finally {
        Remove-Item $bodyFile, $headerFile, $outFile -Force -ErrorAction SilentlyContinue
    }
}

$up = $false
try {
    Write-Host "==> docker compose up --build"
    docker compose @composeArgs up -d --build
    if ($LASTEXITCODE -ne 0) { throw "compose up failed" }
    $up = $true

    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        try {
            # Use Envoy admin /ready; public /healthz is behind ext_authz.
            $r = Invoke-WebRequest -Uri "http://127.0.0.1:9901/ready" -UseBasicParsing -TimeoutSec 2
            if ($r.StatusCode -eq 200) { $ready = $true; break }
        } catch { Start-Sleep -Seconds 2 }
    }
    if (-not $ready) { throw "public Envoy did not become ready" }

    $prom = (Invoke-WebRequest -Uri "http://127.0.0.1:9901/stats/prometheus" -UseBasicParsing -TimeoutSec 5).Content
    if ($prom -notmatch 'envoy_http_downstream_rq_total') {
        throw "Envoy Prometheus stats missing downstream request metric"
    }
    $rqLine = ($prom -split "`n" | Where-Object { $_ -match '^envoy_http_downstream_rq_total' } | Select-Object -First 1)
    Write-Host "==> Envoy prometheus: $rqLine"

    if ($TLS) {
        $h2Body = Join-Path $env:TEMP ("h2-body-" + $PID + ".json")
        [IO.File]::WriteAllText($h2Body, '{"jsonrpc":"2.0","id":99,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"h2check","version":"0"}}}', [Text.UTF8Encoding]::new($false))
        try {
            $h2Out = & $h2Exe -url "$baseUrl/mcp" -body-file $h2Body -token test 2>&1
            if ($LASTEXITCODE -ne 0) { throw "actual HTTP/2 request failed: $($h2Out -join ' ')" }
            Write-Host "==> actual HTTP/2: $($h2Out[0])"
        } finally {
            Remove-Item $h2Body -Force -ErrorAction SilentlyContinue
            Remove-Item $h2Exe -Force -ErrorAction SilentlyContinue
        }
    }

    Write-Host "==> legacy /mcp initialize + tools/list"
    $init = $null
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $init = Invoke-JsonPost "$baseUrl/mcp" '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t8","version":"0"}}}'
            if ($init.Status -eq 200) { break }
            Start-Sleep -Seconds 1
        } catch {
            # policy/ext_authz can still be starting; public Envoy returns 503.
            Start-Sleep -Seconds 1
        }
    }
    if (-not $init) { throw "legacy initialize failed after retries" }
    $session = $init.Session
    if (-not $session) { throw "legacy initialize did not return Mcp-Session-Id" }
    $listBody = '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
    $names = @()
    for ($i = 0; $i -lt 30; $i++) {
        $list = Invoke-JsonPost "$baseUrl/mcp" $listBody @{"Mcp-Session-Id" = $session}
        if ($list.Status -eq 200) {
            $names = @($list.Json.result.tools | ForEach-Object { $_.name })
            if (($names -contains "weather__get_weather") -and ($names -contains "crm__get_customer") -and ($names -contains "rest__get_customer")) { break }
        }
        Start-Sleep -Seconds 1
    }
    if (-not ($names -contains "weather__get_weather") -or -not ($names -contains "crm__get_customer") -or -not ($names -contains "rest__get_customer")) {
        throw "legacy catalog missing expected tools: $($names -join ',')"
    }

    Write-Host "==> legacy SSE transport"
    $sseArgs = @("-url", "$baseUrl/mcp", "-token", "test")
    if ($TLS) { $sseArgs += "-insecure" }
    $sseOK = $false
    for ($i = 0; $i -lt 30; $i++) {
        $sseOut = & $sseExe @sseArgs 2>&1
        if ($LASTEXITCODE -eq 0 -and ($sseOut -join ' ') -match 'sse_ok=true') { $sseOK = $true; break }
        Start-Sleep -Seconds 1
    }
    if (-not $sseOK) { throw "SSE check failed: $($sseOut -join ' ')" }
    Write-Host "==> $($sseOut -join ' ')"

    Write-Host "==> legacy tools/call + ratelimit 429"
    # Let the policy/ratelimit gRPC service and Redis connection settle; while it
    # is down the filter is fail-open and those calls are not counted.
    Start-Sleep -Seconds 3
    $callBody = '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"weather__get_weather","arguments":{"q":"Paris"}}}'
    $okCalls = 0
    $limitedCalls = 0
    for ($i = 0; $i -lt 70; $i++) {
        $call = Invoke-JsonPost "$baseUrl/mcp" $callBody @{
            "Mcp-Session-Id" = $session
            "Mcp-Name"       = "weather__get_weather"
        }
        if ($call.Status -eq 200) { $okCalls++ }
        elseif ($call.Status -eq 429) { $limitedCalls++ }
    }
    if ($limitedCalls -eq 0) {
        throw "ratelimit did not trigger (200=$okCalls, 429=$limitedCalls)"
    }
    $restCall = Invoke-JsonPost "$baseUrl/mcp" '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"rest__get_customer","arguments":{"customer_id":"42"}}}' @{
        "Mcp-Session-Id" = $session
        "Mcp-Name"       = "rest__get_customer"
    }
    if ($restCall.Status -ne 200 -or $restCall.Body -notmatch 'Ada Lovelace') {
        throw "REST bridge call failed: status=$($restCall.Status) body=$($restCall.Body)"
    }
    $redisKeys = (& docker compose @composeArgs exec -T redis redis-cli DBSIZE 2>&1 | Out-String).Trim()
    if (-not $redisKeys -or [int]$redisKeys -lt 1) {
        throw "Redis limiter did not write keys (DBSIZE=$redisKeys)"
    }
    Write-Host "==> ratelimit observed: 200=$okCalls 429=$limitedCalls redis_keys=$redisKeys"

    Write-Host "==> modern /modern/mcp discover + tools/list"
    $meta = '"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}'
    $discover = $null
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $discover = Invoke-JsonPost "$baseUrl/modern/mcp" ('{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{' + $meta + '}}') @{
                "MCP-Protocol-Version" = "2026-07-28"
                "Mcp-Method"           = "server/discover"
            }
            if ($discover.Status -eq 200) { break }
            Start-Sleep -Seconds 1
        } catch { Start-Sleep -Seconds 1 }
    }
    if (-not $discover) { throw "modern discover failed after retries" }
    if ($discover.Status -ne 200) { throw "modern discover status $($discover.Status)" }
    $modernList = Invoke-JsonPost "$baseUrl/modern/mcp" ('{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{' + $meta + '}}') @{
        "MCP-Protocol-Version" = "2026-07-28"
        "Mcp-Method"           = "tools/list"
    }
    $modernNames = @()
    for ($i = 0; $i -lt 30; $i++) {
        $modernList = Invoke-JsonPost "$baseUrl/modern/mcp" ('{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{' + $meta + '}}') @{
            "MCP-Protocol-Version" = "2026-07-28"
            "Mcp-Method"           = "tools/list"
        }
        if ($modernList.Status -eq 200) {
            $modernNames = @($modernList.Json.result.tools | ForEach-Object { $_.name })
            if (($modernNames -contains "weather__get_weather") -and ($modernNames -contains "crm__get_customer")) { break }
        }
        Start-Sleep -Seconds 1
    }
    if (-not ($modernNames -contains "weather__get_weather") -or -not ($modernNames -contains "crm__get_customer")) {
        throw "modern catalog missing expected tools: $($modernNames -join ',')"
    }

    $mode = if ($TLS) { "TLS" } else { "HTTP" }
    Write-Host "T8 PASS ($mode): public path split + internal mcp_router + modern facade + ratelimit"
} catch {
    Write-Host "T8 FAIL: $($_.Exception.Message)"
    if ($up) { docker compose @composeArgs logs --no-color | Select-Object -Last 120 }
    exit 1
} finally {
    if ($up) {
        # Teardown progress is written to stderr; do not turn it into a script failure.
        $ErrorActionPreference = "Continue"
        docker compose @composeArgs down -v *> $null
    }
    Remove-Item $sseExe -Force -ErrorAction SilentlyContinue
}