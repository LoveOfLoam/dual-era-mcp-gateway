param()
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
} finally { Pop-Location }

function Invoke-JsonPost($Uri, $Body, $Headers = @{}) {
    $bodyFile = [IO.Path]::GetTempFileName()
    $headerFile = [IO.Path]::GetTempFileName()
    $outFile = [IO.Path]::GetTempFileName()
    try {
        [IO.File]::WriteAllText($bodyFile, $Body, [Text.UTF8Encoding]::new($false))
        $args = @("-sS", "-k", "--max-time", "10", "-D", $headerFile, "-o", $outFile, "-w", "%{http_code}", "-X", "POST", $Uri,
            "-H", "Content-Type: application/json", "-H", "Accept: application/json, text/event-stream",
            "-H", "Authorization: Bearer test", "--data-binary", "@$bodyFile")
        foreach ($k in $Headers.Keys) { $args += @("-H", "$k`: $($Headers[$k])") }
        $status = 0
        [void][int]::TryParse(((& curl.exe @args 2>&1) | Out-String).Trim(), [ref]$status)
        $text = if (Test-Path $outFile) { [IO.File]::ReadAllText($outFile) } else { "" }
        $session = $null
        foreach ($line in Get-Content $headerFile -ErrorAction SilentlyContinue) {
            if ($line -match '^(?i)mcp-session-id:\s*(.+)$') { $session = $Matches[1].Trim() }
        }
        $json = $null; try { $json = $text | ConvertFrom-Json } catch { }
        return @{ Status = $status; Session = $session; Json = $json; Body = $text }
    } finally { Remove-Item $bodyFile, $headerFile, $outFile -Force -ErrorAction SilentlyContinue }
}

$up = $false
try {
    docker compose -f $compose up -d --build
    if ($LASTEXITCODE -ne 0) { throw "compose up failed" }
    $up = $true
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $r = Invoke-WebRequest -Uri "http://127.0.0.1:9901/ready" -UseBasicParsing -TimeoutSec 2
            if ($r.StatusCode -eq 200) { break }
        } catch { Start-Sleep -Seconds 2 }
    }
    Start-Sleep -Seconds 3

    $initBody = '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"chaos","version":"0"}}}'
    $init = Invoke-JsonPost "http://127.0.0.1:8080/mcp" $initBody
    if ($init.Status -ne 200 -or -not $init.Session) { throw "baseline initialize failed: $($init.Status) $($init.Body)" }
    $sid = $init.Session
    $listBody = '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
    $list = Invoke-JsonPost "http://127.0.0.1:8080/mcp" $listBody @{"Mcp-Session-Id" = $sid}
    $baseNames = @($list.Json.result.tools | ForEach-Object { $_.name })
    Write-Host "baseline tools: $($baseNames -join ',')"

    Write-Host "==> stop crm (partial backend failure)"
    $ErrorActionPreference = "Continue"
    docker compose -f $compose stop crm *> $null
    $ErrorActionPreference = "Stop"
    Start-Sleep -Seconds 2
    $partial = Invoke-JsonPost "http://127.0.0.1:8080/mcp" $listBody @{"Mcp-Session-Id" = $sid}
    $partialNames = @($partial.Json.result.tools | ForEach-Object { $_.name })
    Write-Host "partial status=$($partial.Status) tools=$($partialNames -join ',')"
    if ($partialNames -notcontains "weather__get_weather" -or $partialNames -notcontains "rest__get_customer") {
        throw "partial catalog lost healthy tools"
    }
    $crmCall = Invoke-JsonPost "http://127.0.0.1:8080/mcp" '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"crm__get_customer","arguments":{"customer_id":"1"}}}' @{"Mcp-Session-Id" = $sid; "Mcp-Name" = "crm__get_customer"}
    Write-Host "crm call while stopped: status=$($crmCall.Status) body=$($crmCall.Body)"
    if ($crmCall.Status -eq 200 -and $crmCall.Body -notmatch '"error"') {
        throw "crm call unexpectedly succeeded while backend was stopped"
    }

    Write-Host "==> start crm and re-check catalog"
    $ErrorActionPreference = "Continue"
    docker compose -f $compose start crm *> $null
    $ErrorActionPreference = "Stop"
    Start-Sleep -Seconds 2
    $recovered = Invoke-JsonPost "http://127.0.0.1:8080/mcp" $listBody @{"Mcp-Session-Id" = $sid}
    $recoveredNames = @($recovered.Json.result.tools | ForEach-Object { $_.name })
    Write-Host "recovered status=$($recovered.Status) tools=$($recoveredNames -join ',')"
    if ($recoveredNames -notcontains "crm__get_customer") { throw "crm did not recover" }

    Write-Host "==> stop policy (ext_authz fail-closed)"
    $ErrorActionPreference = "Continue"
    docker compose -f $compose stop policy *> $null
    $ErrorActionPreference = "Stop"
    Start-Sleep -Seconds 2
    $denied = Invoke-JsonPost "http://127.0.0.1:8080/mcp" $initBody
    Write-Host "while policy stopped: status=$($denied.Status) body=$($denied.Body)"
    if ($denied.Status -eq 200) { throw "requests passed while ext_authz was down" }

    Write-Host "T8 CHAOS PASS: partial backend failure, recovery, ext_authz fail-closed"
} catch {
    Write-Host "T8 CHAOS FAIL: $($_.Exception.Message)"
    exit 1
} finally {
    if ($up) {
        $ErrorActionPreference = "Continue"
        docker compose -f $compose down -v *> $null
    }
}
exit 0