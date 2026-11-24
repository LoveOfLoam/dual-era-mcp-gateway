param(
    [int]$Changes = 20,
    [int]$Concurrency = 10,
    [int]$Seconds = 3
)
$ErrorActionPreference = "Stop"
if (-not $env:GOCACHE) { $env:GOCACHE = Join-Path $env:TEMP "gocache-dual-era" }
if (-not $env:GOMODCACHE) { $env:GOMODCACHE = Join-Path $env:TEMP "gomodcache-dual-era" }

$root = Split-Path -Parent $PSScriptRoot
$compose = Join-Path $root "deploy\t8\docker-compose.yml"
$xdsCompose = Join-Path $root "deploy\t8\docker-compose.xds.yml"
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
    $loadgen = Join-Path $env:TEMP ("loadgen-xds-" + $PID + ".exe")
    go build -o $loadgen ./cmd/loadgen
    $sseCheck = Join-Path $env:TEMP ("ssecheck-xds-" + $PID + ".exe")
    go build -o $sseCheck ./cmd/ssecheck
} finally { Pop-Location }

$up = $false
try {
    docker compose -f $compose -f $xdsCompose up -d --build
    if ($LASTEXITCODE -ne 0) { throw "compose up failed" }
    $up = $true
    $ready = $false
    for ($i = 0; $i -lt 40; $i++) {
        try {
            $r = Invoke-WebRequest -Uri "http://127.0.0.1:9902/ready" -UseBasicParsing -TimeoutSec 2
            if ($r.StatusCode -eq 200) { $ready = $true; break }
        } catch { Start-Sleep -Seconds 1 }
    }
    if (-not $ready) { throw "xDS Envoy did not become ready" }

    $initBody = '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"xds","version":"0"}}}'
    $listBody = '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
    $initFile = Join-Path $env:TEMP ("xds-init-" + $PID + ".json")
    [IO.File]::WriteAllText($initFile, $initBody, [Text.UTF8Encoding]::new($false))
    $hdr = Join-Path $env:TEMP ("xds-hdr-" + $PID + ".txt")
    $ok = $false
    for ($i = 0; $i -lt 30; $i++) {
        $code = & curl.exe -sS -o NUL -D $hdr -w "%{http_code}" -X POST "http://127.0.0.1:8443/mcp" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" --data-binary "@$initFile" 2>&1
        if ([int]$code -eq 200) { $ok = $true; break }
        Start-Sleep -Seconds 1
    }
    if (-not $ok) { throw "xDS public listener did not become ready: $code" }
    $sid = (Get-Content $hdr | Where-Object { $_ -match '^(?i)mcp-session-id:' } | Select-Object -First 1) -replace '^(?i)mcp-session-id:\s*', ''
    Write-Host "xDS initialize: 200 session=$($sid.Length) chars"
    $sse = & $sseCheck -url "http://127.0.0.1:8443/mcp" -token test 2>&1
    Write-Host "xDS SSE: $($sse -join ' ')"
    if ($LASTEXITCODE -ne 0) { throw "xDS SSE transport failed" }

    $before = (Invoke-RestMethod "http://127.0.0.1:18080/metrics" -UseBasicParsing) | Select-String 'mcp_controlplane_xds_ack_total (\d+)'
    $ackBefore = if ($before.Matches.Count -gt 0) { [int]$before.Matches[0].Groups[1].Value } else { 0 }

    $bodyFile = Join-Path $env:TEMP ("xds-load-" + $PID + ".json")
    [IO.File]::WriteAllText($bodyFile, '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}', [Text.UTF8Encoding]::new($false))
    $outFile = Join-Path $root "docs/benchmarks/t8-xds-load.json"
    $job = Start-Job -ScriptBlock {
        param($exe, $body, $out)
        & $exe -url "http://127.0.0.1:8443/modern/mcp" -method POST -body-file $body `
            -header "Content-Type: application/json" -header "Accept: application/json" `
            -header "MCP-Protocol-Version: 2026-07-28" -header "Mcp-Method: tools/list" `
            -concurrency 10 -duration "3s" -out $out 2>&1
    } -ArgumentList $loadgen, $bodyFile, $outFile

    $sseHold = Start-Job -ScriptBlock {
        param($exe)
        & $exe -url "http://127.0.0.1:8443/mcp" -token test -hold 3s 2>&1
    } -ArgumentList $sseCheck

    $base = Get-Content (Join-Path $root "deploy\t8\xds-config.json") -Raw | ConvertFrom-Json
    $a = $base.PSObject.Copy()
    $a | Add-Member -NotePropertyName listenerHeader -NotePropertyValue "v1" -Force
    $b = $base.PSObject.Copy()
    $b | Add-Member -NotePropertyName listenerHeader -NotePropertyValue "v2" -Force
    $jsonA = $a | ConvertTo-Json -Depth 8
    $jsonB = $b | ConvertTo-Json -Depth 8
    for ($i = 0; $i -lt $Changes; $i++) {
        $json = if ($i % 2 -eq 0) { $jsonA } else { $jsonB }
        $resp = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18080/api/v1/apply" -ContentType "application/json" -Body $json
        if (-not $resp.version) { throw "apply $i failed" }
        if ($i % 10 -eq 0) {
            $sse = & $sseCheck -url "http://127.0.0.1:8443/mcp" -token test 2>&1
            if ($LASTEXITCODE -ne 0) { throw "SSE failed after apply $i" }
        }
    }
    Wait-Job $sseHold -Timeout 20 | Out-Null
    $sseHoldOut = Receive-Job $sseHold
    $sseHoldExit = $sseHold.State
    Remove-Job $sseHold -Force -ErrorAction SilentlyContinue
    Write-Host "xDS long-lived SSE: $($sseHoldOut -join ' ')"
    if ($sseHoldExit -ne "Completed" -or ($sseHoldOut -join ' ') -notmatch 'sse_ok=true') {
        throw "long-lived SSE failed during churn"
    }

    Wait-Job $job -Timeout 20 | Out-Null
    $loadOut = Receive-Job $job
    Remove-Job $job -Force -ErrorAction SilentlyContinue
    Write-Host "xDS load: $($loadOut -join ' ')"
    if ($loadOut -match 'errors=[1-9]') { throw "load saw errors during xDS churn" }

    $after = (Invoke-RestMethod "http://127.0.0.1:18080/metrics" -UseBasicParsing) | Select-String 'mcp_controlplane_xds_ack_total (\d+)'
    $ackAfter = if ($after.Matches.Count -gt 0) { [int]$after.Matches[0].Groups[1].Value } else { 0 }
    Write-Host "xDS ACKs before=$ackBefore after=$ackAfter changes=$Changes"
    if ($ackAfter -le $ackBefore) { throw "xDS did not ACK the updates" }

    $nackBefore = (Invoke-RestMethod "http://127.0.0.1:18080/metrics" -UseBasicParsing) | Select-String 'mcp_controlplane_xds_nack_total (\d+)'
    $nackBefore = if ($nackBefore.Matches.Count -gt 0) { [int]$nackBefore.Matches[0].Groups[1].Value } else { 0 }
    Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18080/api/v1/inject-invalid" | Out-Null
    Start-Sleep -Seconds 2
    $nackAfter = (Invoke-RestMethod "http://127.0.0.1:18080/metrics" -UseBasicParsing) | Select-String 'mcp_controlplane_xds_nack_total (\d+)'
    $nackAfter = if ($nackAfter.Matches.Count -gt 0) { [int]$nackAfter.Matches[0].Groups[1].Value } else { 0 }
    Write-Host "xDS NACKs before=$nackBefore after=$nackAfter"
    if ($nackAfter -le $nackBefore) { throw "Envoy did not NACK the invalid listener" }

    Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18080/api/v1/apply" -ContentType "application/json" -Body $jsonA | Out-Null
    Start-Sleep -Seconds 2
    $code = & curl.exe -sS -o NUL -w "%{http_code}" -X POST "http://127.0.0.1:8443/mcp" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" --data-binary "@$initFile" 2>&1
    Write-Host "xDS recovery initialize: status=$code"
    if ([int]$code -ne 200) { throw "listener did not recover after NACK" }

    Write-Host "T8 XDS PASS: LDS/CDS/RDS churn, ACK, NACK and recovery"
} catch {
    Write-Host "T8 XDS FAIL: $($_.Exception.Message)"
    exit 1
} finally {
    if ($up) {
        $ErrorActionPreference = "Continue"
        docker compose -f $compose -f $xdsCompose down -v *> $null
    }
    Remove-Item $loadgen, $sseCheck -Force -ErrorAction SilentlyContinue
}
exit 0