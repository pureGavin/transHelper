# 把 Go 工具链下载到 .tools，校验后打印版本。
# 可以重复运行：哈希匹配的压缩包和已安装的版本会原样保留。
# 不运行 MSI，不用 setx，也不执行 go env -w。

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
$tools = Join-Path $root '.tools'
$downloads = Join-Path $tools 'downloads'
if (-not (Test-Path -LiteralPath $downloads)) {
    New-Item -ItemType Directory -Path $downloads | Out-Null
}

$version = 'go1.27.3'
$fileName = 'go1.27.3.windows-amd64.zip'
$url = "https://golang.google.cn/dl/$fileName"
$expectedSha = 'f26081f438319757f8a1b46d7d0b2c5ec7583d4038e485e0aa725cfde7885fff'
$zipPath = Join-Path $downloads $fileName
$goExe = Join-Path $tools 'go\bin\go.exe'
$stamp = Join-Path $tools 'go\.transhelper-toolchain'

function Get-Sha256Lower([string]$Path) {
    return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}

$zipOk = $false
if (Test-Path -LiteralPath $zipPath) {
    $actual = Get-Sha256Lower $zipPath
    if ($actual -eq $expectedSha) {
        $zipOk = $true
        Write-Host "zip already present and hash matches"
    } else {
        Write-Host "zip hash mismatch, downloading again"
        Remove-Item -LiteralPath $zipPath -Force
    }
}

if (-not $zipOk) {
    Write-Host "downloading $url"
    & curl.exe -fL --retry 3 --retry-delay 2 -o $zipPath $url
    if ($LASTEXITCODE -ne 0) {
        throw "download failed with exit $LASTEXITCODE"
    }
    $actual = Get-Sha256Lower $zipPath
    if ($actual -ne $expectedSha) {
        Remove-Item -LiteralPath $zipPath -Force
        throw "sha256 mismatch: expected $expectedSha got $actual"
    }
    Write-Host "sha256 ok"
}

$installed = $false
if ((Test-Path -LiteralPath $goExe) -and (Test-Path -LiteralPath $stamp)) {
    $stampText = (Get-Content -LiteralPath $stamp -Raw).Trim()
    if ($stampText -eq $version) {
        $installed = $true
        Write-Host "toolchain $version already installed"
    }
}

if (-not $installed) {
    $goDir = Join-Path $tools 'go'
    if (Test-Path -LiteralPath $goDir) {
        Remove-Item -LiteralPath $goDir -Recurse -Force
    }
    Write-Host "extracting to $tools"
    & tar.exe -xf $zipPath -C $tools
    if ($LASTEXITCODE -ne 0) {
        throw "extract failed with exit $LASTEXITCODE"
    }
    if (-not (Test-Path -LiteralPath $goExe)) {
        throw "go.exe missing after extract"
    }
    Set-Content -LiteralPath $stamp -Value $version -Encoding ascii -NoNewline
}

. (Join-Path $PSScriptRoot 'env.ps1')
& go version
if ($LASTEXITCODE -ne 0) {
    throw "go version failed with exit $LASTEXITCODE"
}
