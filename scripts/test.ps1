param(
    [switch]$Race
)

$ErrorActionPreference = "Stop"
# `go test` must run for the host OS/arch; a stale cross-compile env makes
# Windows try to execute Linux test binaries.
$hostOS = (go env GOHOSTOS)
if ($env:GOOS -and $env:GOOS -ne $hostOS) {
    Write-Warning "Clearing GOOS=$env:GOOS for host test run ($hostOS)."
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
}
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

Write-Host "==> helm lint (optional)"
& "$PSScriptRoot\helm-lint.ps1"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "==> envoy validate (optional)"
& "$PSScriptRoot\envoy-validate.ps1"
exit $LASTEXITCODE