param(
    [int]$Concurrency = 20,
    [int]$Seconds = 5,
    [string]$Out = "docs/benchmarks/local-load.json"
)

$ErrorActionPreference = "Stop"
if (-not $env:GOCACHE) { $env:GOCACHE = Join-Path $env:TEMP "gocache-dual-era" }
if (-not $env:GOMODCACHE) { $env:GOMODCACHE = Join-Path $env:TEMP "gomodcache-dual-era" }
$repo = Split-Path $PSScriptRoot -Parent
Push-Location $repo
$tmp = Join-Path $env:TEMP ("dual-era-load-" + $PID)
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

function Build($name, $pkg) {
    $exe = Join-Path $tmp ($name + ".exe")
    & go build -o $exe (Join-Path $repo $pkg)
    if ($LASTEXITCODE -ne 0) { throw "build $name failed" }
    return $exe
}

$mock = Build "legacy-mock" "cmd/legacy-mock"
$gateway = Build "gateway" "cmd/gateway"
$loadgen = Build "loadgen" "cmd/loadgen"

$mockProc = $null
$gwProc = $null
try {
    $mockProc = Start-Process -FilePath $mock -ArgumentList @("-addr", ":9201", "-name", "weather", "-tools", "get_weather,list_cities") -PassThru -WindowStyle Hidden
    $gwProc = Start-Process -FilePath $gateway -ArgumentList @("-addr", ":8281", "-legacy", "weather=http://127.0.0.1:9201/mcp") -PassThru -WindowStyle Hidden
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $r = Invoke-WebRequest -Uri "http://127.0.0.1:8281/healthz" -UseBasicParsing -TimeoutSec 1
            if ($r.StatusCode -eq 200) { break }
        } catch { Start-Sleep -Milliseconds 500 }
    }
    $meta = '"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}'
    $body = '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{' + $meta + '}}'
    $bodyFile = Join-Path $tmp "body.json"
    [System.IO.File]::WriteAllText($bodyFile, $body, [System.Text.UTF8Encoding]::new($false))
    & $loadgen `
        -url "http://127.0.0.1:8281/modern/mcp" `
        -method POST `
        -body-file $bodyFile `
        -header "Content-Type: application/json" `
        -header "MCP-Protocol-Version: 2026-07-28" `
        -header "Mcp-Method: tools/list" `
        -concurrency $Concurrency `
        -duration "${Seconds}s" `
        -out (Join-Path $repo $Out)
    if ($LASTEXITCODE -ne 0) { throw "loadgen failed" }
} finally {
    if ($gwProc -and -not $gwProc.HasExited) { Stop-Process -Id $gwProc.Id -Force -ErrorAction SilentlyContinue }
    if ($mockProc -and -not $mockProc.HasExited) { Stop-Process -Id $mockProc.Id -Force -ErrorAction SilentlyContinue }
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    Pop-Location
}