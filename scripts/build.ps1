$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $projectRoot
go run .\scripts
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
