# 编译并运行 uidrive，用窗口消息操作托盘和设置窗口。
# 例：
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts\uidrive.ps1 start
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts\uidrive.ps1 menu settings
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts\uidrive.ps1 state
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts\uidrive.ps1 stop

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root
. (Join-Path $PSScriptRoot 'env.ps1')

$outDir = Join-Path $root '.tools\tmp\uidrive'
if (-not (Test-Path -LiteralPath $outDir)) {
    New-Item -ItemType Directory -Path $outDir | Out-Null
}
$exe = Join-Path $outDir 'uidrive.exe'
& go build -tags uidrive -o $exe .\cmd\uidrive
if ($LASTEXITCODE -ne 0) {
    throw "编译 uidrive 失败，退出码 $LASTEXITCODE"
}
& $exe @args
exit $LASTEXITCODE
