param()
$ErrorActionPreference = "Continue"

Write-Host "==> govulncheck"
$govuln = Get-Command govulncheck -ErrorAction SilentlyContinue
if ($govuln) {
    & $govuln.Source ./...
    if ($LASTEXITCODE -ne 0) { Write-Warning "govulncheck reported findings (see above)." }
} else {
    Write-Host "SKIP: govulncheck not installed (go install golang.org/x/vuln/cmd/govulncheck@latest)."
}

Write-Host "==> trivy fs"
$trivy = Get-Command trivy -ErrorAction SilentlyContinue
if ($trivy) {
    & $trivy.Source fs --severity HIGH,CRITICAL --exit-code 1 .
    if ($LASTEXITCODE -ne 0) { Write-Warning "trivy reported findings (see above)." }
} else {
    Write-Host "SKIP: trivy not installed."
}