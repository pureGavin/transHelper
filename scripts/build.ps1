# 构建发布包：无控制台窗口的 exe 和说明，再打成 zip。
# 用法：powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build.ps1 [-Version 2026.11.1]
# 不传 -Version 时用当前北京时间的年月，x 固定为 0，用来和正式发布区分。
# 不用 UPX。压缩过的 exe 更容易被杀毒软件误报。

param(
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"

if (-not $Version) {
    $tz = [TimeZoneInfo]::FindSystemTimeZoneById('China Standard Time')
    $now = [TimeZoneInfo]::ConvertTime([DateTime]::UtcNow, $tz)
    $Version = '{0}.{1:D2}.0' -f $now.Year, $now.Month
}

if ($Version -notmatch '^\d{4}\.(0[1-9]|1[0-2])\.\d+$') {
    throw "版本号必须是 YYYY.MM.x，例如 2026.11.1"
}

$root = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root
. (Join-Path $PSScriptRoot "env.ps1")

$outDir = Join-Path $root "dist\transhelper"
$exe = Join-Path $outDir "transhelper.exe"

$running = Get-Process -Name transhelper -ErrorAction SilentlyContinue | Where-Object {
    $_.Path -and ($_.Path -eq $exe)
}
if ($running) {
    throw "dist\transhelper\transhelper.exe 正在运行。先在托盘里退出，再重新构建。"
}

# 阶段 2 的控制台版直接放在 dist 根目录。正式包改到 dist\transhelper。
foreach ($old in @(
    (Join-Path $root "dist\transhelper.exe"),
    (Join-Path $root "dist\transhelper.log")
)) {
    if (Test-Path -LiteralPath $old) {
        Remove-Item -LiteralPath $old -Force
    }
}

# 不检查 .tools。那里是 Go 工具链，gofmt 扫进去又慢又会报一堆无关文件。
$fmt = & gofmt -l cmd internal
if ($LASTEXITCODE -ne 0) {
    throw "gofmt 失败，退出码 $LASTEXITCODE"
}
if ($fmt) {
    throw ("这些文件还没格式化:`n" + ($fmt -join "`n"))
}

& go vet ./...
if ($LASTEXITCODE -ne 0) {
    throw "go vet 失败，退出码 $LASTEXITCODE"
}

& go test ./...
if ($LASTEXITCODE -ne 0) {
    throw "go test 失败，退出码 $LASTEXITCODE"
}

& go tool go-winres make --arch amd64 --in cmd\transhelper\winres\winres.json --out cmd\transhelper\rsrc --file-version $Version --product-version $Version
if ($LASTEXITCODE -ne 0) {
    throw "go-winres 失败，退出码 $LASTEXITCODE"
}

if (-not (Test-Path -LiteralPath $outDir)) {
    New-Item -ItemType Directory -Path $outDir | Out-Null
}

# -buildmode=pie 打开 ASLR 和高熵 ASLR。-s -w 去掉符号表和 DWARF。
# 编完再检查 PE 头，避免链接器默认值变了还照样发出去。
$ldflags = "-H=windowsgui -s -w -X main.version=$Version"
& go build -trimpath -buildmode=pie -ldflags $ldflags -o $exe .\cmd\transhelper
if ($LASTEXITCODE -ne 0) {
    throw "go build 失败，退出码 $LASTEXITCODE"
}

& go run ./cmd/pecheck $exe
if ($LASTEXITCODE -ne 0) {
    throw "pecheck 失败，退出码 $LASTEXITCODE"
}

Copy-Item -LiteralPath (Join-Path $root "README.md") -Destination (Join-Path $outDir "README.md") -Force

$zip = Join-Path $root ("dist\transHelper-{0}-windows-amd64.zip" -f $Version)
if (Test-Path -LiteralPath $zip) {
    Remove-Item -LiteralPath $zip -Force
}
# 按文件清单打包，不把目录里可能存在的 transhelper.toml（含密钥）放进去。
Compress-Archive -LiteralPath @(
    $exe,
    (Join-Path $outDir "README.md")
) -DestinationPath $zip

Write-Host "发布包："
Get-FileHash -Algorithm SHA256 -LiteralPath $exe, $zip | Format-Table -AutoSize Hash, Path
