# 从提交说明收集 Release 页面要写的更新、修复和删除。
# 调用 git 前把输出编码设成 UTF-8：runner 上的 Windows PowerShell 默认按系统代码页解码，中文提交说明会乱码。
# 用法：powershell -NoProfile -ExecutionPolicy Bypass -File scripts/release-notes.ps1 -OutFile notes.md [-From v2026.11.1]

param(
    [Parameter(Mandatory = $true)]
    [string]$OutFile,

    [string]$From = ''
)

$ErrorActionPreference = 'Stop'

if (Test-Path -LiteralPath $OutFile) {
    Remove-Item -LiteralPath $OutFile -Force
}

$utf8 = New-Object System.Text.UTF8Encoding $false
$OutputEncoding = $utf8
[Console]::OutputEncoding = $utf8

function Get-PreviousReleaseTag {
    $tags = @(& git tag -l "v*")
    if ($LASTEXITCODE -ne 0) {
        throw "git tag 失败，退出码 $LASTEXITCODE"
    }
    $best = $null
    foreach ($tag in $tags) {
        if ($tag -match '^v(\d{4})\.(0[1-9]|1[0-2])\.(\d+)$') {
            $year = [int]$Matches[1]
            $seq = [int]$Matches[3]
            $newer = $false
            if (-not $best) {
                $newer = $true
            } elseif ($year -gt $best.Year -or ($year -eq $best.Year -and $seq -gt $best.Seq)) {
                $newer = $true
            }
            if ($newer) {
                $best = @{ Year = $year; Seq = $seq; Tag = $tag }
            }
        }
    }
    if ($best) {
        return $best.Tag
    }
    return ''
}

$base = ''
if ($From) {
    $base = $From
    $range = "$From..HEAD"
} else {
    $base = Get-PreviousReleaseTag
    if ($base) {
        $range = "$base..HEAD"
    } else {
        $range = 'HEAD'
    }
}

$raw = @(& git -c core.quotepath=false -c i18n.logOutputEncoding=utf-8 log $range --no-merges --format="---END---%n%B")
if ($LASTEXITCODE -ne 0) {
    throw "git log 失败，退出码 $LASTEXITCODE"
}
if ($raw.Count -eq 0) {
    throw '没有新提交，不能发布'
}

$text = $raw -join "`n"
$groups = @{
    '更新' = New-Object System.Collections.Generic.List[string]
    '修复' = New-Object System.Collections.Generic.List[string]
    '删除' = New-Object System.Collections.Generic.List[string]
}
$seen = @{}
foreach ($line in ($text -split "`n")) {
    $trim = $line.Trim()
    if ($trim -match '^(更新|修复|删除)\s*[:：]\s*(.+)$') {
        $kind = $Matches[1]
        $item = $Matches[2].Trim()
        if (-not $item) {
            continue
        }
        $key = $kind + '|' + $item
        if ($seen.ContainsKey($key)) {
            continue
        }
        $seen[$key] = $true
        $groups[$kind].Add($item)
    }
}

$total = $groups['更新'].Count + $groups['修复'].Count + $groups['删除'].Count
if ($total -eq 0) {
    throw '提交说明里没有「更新：」「修复：」或「删除：」，不能发布。请补上用户能感受到的变化后再推送。'
}

$lines = New-Object System.Collections.Generic.List[string]
foreach ($kind in @('更新', '修复', '删除')) {
    if ($groups[$kind].Count -eq 0) {
        continue
    }
    $lines.Add("## $kind")
    foreach ($item in $groups[$kind]) {
        $lines.Add("- $item")
    }
    $lines.Add('')
}

$sha = (& git rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0) {
    throw "git rev-parse 失败，退出码 $LASTEXITCODE"
}
if ($base) {
    $lines.Add("https://github.com/pureGavin/transHelper/compare/${base}...$sha")
} else {
    $lines.Add("https://github.com/pureGavin/transHelper/commit/$sha")
}
$lines.Add('')

$outDir = Split-Path -Parent $OutFile
if ($outDir -and -not (Test-Path -LiteralPath $outDir)) {
    New-Item -ItemType Directory -Path $outDir | Out-Null
}
[System.IO.File]::WriteAllText($OutFile, (($lines -join "`n")), $utf8)
Write-Output $OutFile
