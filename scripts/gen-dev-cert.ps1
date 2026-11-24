param(
    [string]$OutDir = "$PSScriptRoot/../deploy/t8/certs"
)

$ErrorActionPreference = "Stop"
$openssl = Get-Command openssl -ErrorAction SilentlyContinue
if (-not $openssl) {
    Write-Host "SKIP: openssl is not installed; cannot generate a dev certificate."
    exit 0
}
$resolved = [System.IO.Path]::GetFullPath($OutDir)
New-Item -ItemType Directory -Force -Path $resolved | Out-Null

# Some Windows OpenSSL builds ship without a usable default openssl.cnf.
if (-not $env:OPENSSL_CONF -or -not (Test-Path $env:OPENSSL_CONF)) {
    $candidate = Join-Path (Split-Path (Split-Path $openssl.Source)) "ssl/openssl.cnf"
    if (Test-Path $candidate) { $env:OPENSSL_CONF = $candidate }
}

& $openssl.Source req -x509 -newkey rsa:2048 -nodes -days 2 `
    -keyout (Join-Path $resolved "server.key") `
    -out (Join-Path $resolved "server.crt") `
    -subj "/CN=localhost" `
    -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
if ($LASTEXITCODE -ne 0) { throw "openssl failed with exit $LASTEXITCODE" }
Write-Host "Generated dev cert in $resolved (2 days; do not use in production)."