param(
    [ValidateRange(1024, 65535)][int]$Port = 8081,
    [switch]$Build
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$previewData = Join-Path $projectRoot "data/preview-$Port"
$previewBinary = Join-Path $projectRoot 'dist/local/assetloop.exe'
$previewLog = Join-Path $projectRoot ".cache/logs/preview-$Port.log"
$env:GOCACHE = Join-Path $projectRoot '.cache/go-build/preview'

foreach ($directory in @($previewData, (Split-Path $previewBinary), (Split-Path $previewLog), $env:GOCACHE)) {
    New-Item -ItemType Directory -Path $directory -Force | Out-Null
}

Push-Location -LiteralPath $projectRoot
try {
    if ($Build -or -not (Test-Path -LiteralPath $previewBinary)) {
        & go build -o $previewBinary ./cmd/assetloop
        if ($LASTEXITCODE -ne 0) { throw 'Preview build failed.' }
    }

    $env:HTTP_ADDR = "127.0.0.1:$Port"
    $env:AUTH_MODE = 'local'
    $env:DB_DRIVER = 'sqlite'
    $env:DB_DSN = Join-Path $previewData 'assetloop.db'
    $env:ATTACHMENT_DEFAULT_STORE = 'local'
    $env:ATTACHMENT_LOCAL_ROOT = Join-Path $previewData 'blobs'
    $env:MCP_ENABLED = 'true'
    $env:MCP_ISSUER = "http://127.0.0.1:$Port"
    $env:MCP_CLIENT_ID = 'codex-local'
    $env:MCP_REDIRECT_URI = 'http://127.0.0.1/callback'

    $providerFile = Join-Path $projectRoot '.env.zhuanzhuan.local'
    if (Test-Path -LiteralPath $providerFile) {
        foreach ($providerLine in Get-Content -LiteralPath $providerFile) {
            if ($providerLine -match '^\s*ZHUANZHUAN_MCP_TOKEN\s*=\s*(.*)$') {
                $env:ZHUANZHUAN_MCP_TOKEN = $matches[1].Trim().Trim('"').Trim("'")
            }
        }
    }

    Write-Host "Preview: http://127.0.0.1:$Port (log: $previewLog)"
    & $previewBinary serve >> $previewLog 2>&1
    if ($LASTEXITCODE -ne 0) { throw "Preview exited with code $LASTEXITCODE. See $previewLog" }
} finally {
    Pop-Location
}
