param(
    [string]$OutDir = "$PSScriptRoot/../deploy/t8/certs/mtls"
)

$ErrorActionPreference = "Stop"
$openssl = Get-Command openssl -ErrorAction SilentlyContinue
if (-not $openssl) {
    Write-Host "SKIP: openssl is not installed; cannot generate mTLS certs."
    exit 0
}
$resolved = [System.IO.Path]::GetFullPath($OutDir)
New-Item -ItemType Directory -Force -Path $resolved | Out-Null

if (-not $env:OPENSSL_CONF -or -not (Test-Path $env:OPENSSL_CONF)) {
    $candidate = Join-Path (Split-Path (Split-Path $openssl.Source)) "ssl/openssl.cnf"
    if (Test-Path $candidate) { $env:OPENSSL_CONF = $candidate }
}

function Invoke-OpenSSL([string[]]$OpenSSLArgs) {
    & $openssl.Source @OpenSSLArgs
    if ($LASTEXITCODE -ne 0) { throw "openssl failed: $($OpenSSLArgs -join ' ')" }
}

$caKey = Join-Path $resolved "ca.key"
$caCrt = Join-Path $resolved "ca.crt"
Invoke-OpenSSL @("req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "2",
    "-keyout", $caKey, "-out", $caCrt, "-subj", "/CN=dual-era-dev-ca",
    "-addext", "basicConstraints=critical,CA:TRUE",
    "-addext", "keyUsage=critical,keyCertSign,cRLSign")

function New-Leaf([string]$Name, [string]$Subject, [string]$ExtText) {
    $key = Join-Path $resolved "$Name.key"
    $csr = Join-Path $resolved "$Name.csr"
    $crt = Join-Path $resolved "$Name.crt"
    $ext = Join-Path $resolved "$Name.ext"
    [IO.File]::WriteAllText($ext, $ExtText)
    Invoke-OpenSSL @("req", "-new", "-newkey", "rsa:2048", "-nodes",
        "-keyout", $key, "-out", $csr, "-subj", $Subject)
    Invoke-OpenSSL @("x509", "-req", "-in", $csr, "-CA", $caCrt, "-CAkey", $caKey,
        "-CAcreateserial", "-days", "2", "-out", $crt, "-extfile", $ext)
    Remove-Item $csr, $ext -Force -ErrorAction SilentlyContinue
}

New-Leaf "server" "/CN=localhost" @"
subjectAltName=DNS:localhost,IP:127.0.0.1
basicConstraints=CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
"@
New-Leaf "client" "/CN=t8-client" @"
basicConstraints=CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=clientAuth
"@

# Windows curl uses schannel and can only import a PKCS#12 client identity.
Invoke-OpenSSL @("pkcs12", "-export", "-out", (Join-Path $resolved "client.p12"),
    "-inkey", (Join-Path $resolved "client.key"),
    "-in", (Join-Path $resolved "client.crt"),
    "-certfile", (Join-Path $resolved "ca.crt"),
    "-passout", "pass:changeit")

Write-Host "Generated mTLS CA/server/client certs in $resolved (2 days; dev only)."