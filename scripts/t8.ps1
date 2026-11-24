$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$compose = Join-Path $root "deploy\t8\docker-compose.yml"

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
    $all = @{
        "Content-Type"  = "application/json"
        "Accept"        = "application/json"
        "Authorization" = "Bearer test"
    }
    foreach ($k in $Headers.Keys) { $all[$k] = $Headers[$k] }
    $resp = Invoke-WebRequest -Uri $Uri -Method Post -Body $Body -Headers $all -UseBasicParsing
    return @{
        Status  = [int]$resp.StatusCode
        Headers = $resp.Headers
        Json    = ($resp.Content | ConvertFrom-Json)
    }
}

$up = $false
try {
    Write-Host "==> docker compose up --build"
    docker compose -f $compose up -d --build
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

    Write-Host "==> legacy /mcp initialize + tools/list"
    $init = Invoke-JsonPost "http://127.0.0.1:8080/mcp" '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
    $session = $init.Headers["Mcp-Session-Id"]
    if (-not $session) { throw "legacy initialize did not return Mcp-Session-Id" }
    $listBody = '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
    $list = Invoke-JsonPost "http://127.0.0.1:8080/mcp" $listBody @{"Mcp-Session-Id" = $session}
    $names = @($list.Json.result.tools | ForEach-Object { $_.name })
    if (-not ($names -contains "weather__get_weather") -or -not ($names -contains "crm__get_customer")) {
        throw "legacy catalog missing expected tools: $($names -join ',')"
    }

    Write-Host "==> modern /modern/mcp discover + tools/list"
    $meta = '"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}'
    $discover = Invoke-JsonPost "http://127.0.0.1:8080/modern/mcp" ('{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{' + $meta + '}}') @{
        "MCP-Protocol-Version" = "2026-07-28"
        "Mcp-Method"           = "server/discover"
    }
    if ($discover.Status -ne 200) { throw "modern discover status $($discover.Status)" }
    $modernList = Invoke-JsonPost "http://127.0.0.1:8080/modern/mcp" ('{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{' + $meta + '}}') @{
        "MCP-Protocol-Version" = "2026-07-28"
        "Mcp-Method"           = "tools/list"
    }
    $modernNames = @($modernList.Json.result.tools | ForEach-Object { $_.name })
    if (-not ($modernNames -contains "weather__get_weather") -or -not ($modernNames -contains "crm__get_customer")) {
        throw "modern catalog missing expected tools: $($modernNames -join ',')"
    }

    Write-Host "T8 PASS: public path split + internal mcp_router + modern facade"
} catch {
    Write-Host "T8 FAIL: $($_.Exception.Message)"
    if ($up) { docker compose -f $compose logs --no-color | Select-Object -Last 120 }
    exit 1
} finally {
    if ($up) { docker compose -f $compose down -v *> $null }
}