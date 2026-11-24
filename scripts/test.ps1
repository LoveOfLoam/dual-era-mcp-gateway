param(
    [switch]$Race
)

$ErrorActionPreference = "Stop"
if (-not $env:GOCACHE) {
    $env:GOCACHE = Join-Path $env:TEMP "gocache-dual-era"
    New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
}
if (-not $env:GOMODCACHE) {
    # Keep module downloads writable in constrained sandboxes. On a normal
    # workstation this can be omitted to reuse the default Go module cache.
    $env:GOMODCACHE = Join-Path $env:TEMP "gomodcache-dual-era"
    New-Item -ItemType Directory -Force -Path $env:GOMODCACHE | Out-Null
}

Write-Host "==> go test ./..."
if ($Race) {
    $gcc = Get-Command gcc -ErrorAction SilentlyContinue
    if (-not $gcc) {
        Write-Warning "Race detector requested but gcc is not installed; running without -race."
        go test ./...
    } else {
        $env:CGO_ENABLED = "1"
        go test -race ./...
    }
} else {
    go test ./...
}
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "==> go vet ./..."
go vet ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "==> envoy validate (optional)"
& "$PSScriptRoot\envoy-validate.ps1"
exit $LASTEXITCODE