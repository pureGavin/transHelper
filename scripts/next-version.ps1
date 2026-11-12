# 根据今年已有的发布 tag 计算下一个版本号 YYYY.MM.x。
# x 只在年份变化时从 1 重新计数，月份变化时继续累加。
# 用法：powershell -NoProfile -ExecutionPolicy Bypass -File scripts\next-version.ps1

$ErrorActionPreference = 'Stop'

$tz = [TimeZoneInfo]::FindSystemTimeZoneById('China Standard Time')
$now = [TimeZoneInfo]::ConvertTime([DateTime]::UtcNow, $tz)
$year = $now.Year
$month = $now.Month

$root = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root

$max = 0
$tags = & git tag -l "v$year.*"
if ($LASTEXITCODE -ne 0) {
    throw "git tag 失败，退出码 $LASTEXITCODE"
}
if ($tags) {
    foreach ($tag in @($tags)) {
        if ($tag -match '^v(\d{4})\.(\d{2})\.(\d+)$') {
            $n = [int]$Matches[3]
            if ($n -gt $max) {
                $max = $n
            }
        }
    }
}

Write-Output ('{0}.{1:D2}.{2}' -f $year, $month, ($max + 1))
