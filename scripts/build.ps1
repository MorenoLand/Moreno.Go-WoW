$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $projectRoot
$running = @(Get-Process -Name MorenoWoW -ErrorAction SilentlyContinue)
if ($running.Count -gt 0) {
	$running | Stop-Process -Force
	Wait-Process -Id $running.Id -ErrorAction SilentlyContinue
}
$binPath = Join-Path $projectRoot "bin"
New-Item -ItemType Directory -Force -Path $binPath | Out-Null
go build -o (Join-Path $binPath "MorenoWoW.exe") .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
$moduleCache = go env GOMODCACHE
$g3nDllPath = Join-Path $moduleCache "github.com\g3n\engine@v0.2.0\audio\windows\bin"
if (Test-Path -LiteralPath $g3nDllPath) { Get-ChildItem -LiteralPath $g3nDllPath -Filter "*.dll" | Copy-Item -Destination $binPath -Force }
