param()
$ErrorActionPreference = "Stop"
if (-not $env:GOCACHE) { $env:GOCACHE = Join-Path $env:TEMP "gocache-dual-era" }
if (-not $env:GOMODCACHE) { $env:GOMODCACHE = Join-Path $env:TEMP "gomodcache-dual-era" }

$root = Split-Path -Parent $PSScriptRoot
$compose = Join-Path $root "deploy\t8\docker-compose.yml"
$mtlsCompose = Join-Path $root "deploy\t8\docker-compose.mtls.yml"
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
    go build -trimpath -o (Join-Path $bin "sse-shim") ./cmd/sse-shim
    $env:GOOS = $oldGOOS; $env:GOARCH = $oldGOARCH; $env:CGO_ENABLED = $oldCGO
    if (-not $oldGOOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue }
    if (-not $oldGOARCH) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue }
    if (-not $oldCGO) { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue }
    $mtlsCheck = Join-Path $env:TEMP ("mtlscheck-" + $PID + ".exe")
    go build -o $mtlsCheck ./cmd/mtlscheck
} finally { Pop-Location }

& (Join-Path $PSScriptRoot "gen-mtls-certs.ps1") | Out-Null
if ($LASTEXITCODE -ne 0) { throw "mTLS cert generation failed" }

$up = $false
try {
    docker compose -f $compose -f $mtlsCompose up -d --build
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

    $certs = Join-Path $root "deploy\t8\certs\mtls"
    $bodyFile = Join-Path $env:TEMP ("mtls-body-" + $PID + ".json")
    [IO.File]::WriteAllText($bodyFile, '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"mtls","version":"0"}}}', [Text.UTF8Encoding]::new($false))
    $outFile = Join-Path $env:TEMP ("mtls-out-" + $PID + ".json")
    try {
        $withCert = & $mtlsCheck -url "https://127.0.0.1:8444/mcp" `
            -cert (Join-Path $certs "client.crt") -key (Join-Path $certs "client.key") `
            -ca (Join-Path $certs "ca.crt") -body-file $bodyFile -token test 2>&1
        Write-Host "with client cert: $($withCert -join ' ')"
        if ($LASTEXITCODE -ne 0 -or ($withCert -join ' ') -notmatch 'status=200') {
            throw "mTLS request failed: $($withCert -join ' ')"
        }

        $ErrorActionPreference = "Continue"
        $noCert = & $mtlsCheck -url "https://127.0.0.1:8444/mcp" `
            -ca (Join-Path $certs "ca.crt") -body-file $bodyFile -token test 2>&1
        $noCertExit = $LASTEXITCODE
        $ErrorActionPreference = "Stop"
        Write-Host "without client cert: exit=$noCertExit $($noCert -join ' ')"
        if ($noCertExit -eq 0) {
            throw "request without a client certificate was accepted"
        }

        Write-Host "T8 MTLS PASS: client-cert request 200; no-cert request rejected"
    } finally {
        Remove-Item $bodyFile -Force -ErrorAction SilentlyContinue
        Remove-Item $mtlsCheck -Force -ErrorAction SilentlyContinue
    }
} catch {
    Write-Host "T8 MTLS FAIL: $($_.Exception.Message)"
    exit 1
} finally {
    if ($up) {
        $ErrorActionPreference = "Continue"
        docker compose -f $compose -f $mtlsCompose down -v *> $null
    }
}
exit 0