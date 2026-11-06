# Dot-source this file so the variables stay in the current session:
#   . .\scripts\env.ps1
# Running it with -File only affects that process, then exits.
# Nothing here writes the registry or the user profile. Close the
# session and the process environment returns to normal.

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
$tools = Join-Path $root '.tools'

$env:GOROOT = Join-Path $tools 'go'
$env:GOPATH = Join-Path $tools 'gopath'
$env:GOMODCACHE = Join-Path $tools 'gopath\pkg\mod'
$env:GOCACHE = Join-Path $tools 'gocache'
$env:GOTMPDIR = Join-Path $tools 'tmp'
$env:GOENV = Join-Path $tools 'go.env'
$env:GOFLAGS = '-modcacherw'
$env:GOTOOLCHAIN = 'local'
$env:CGO_ENABLED = '0'
$env:GOPROXY = 'https://goproxy.cn|https://proxy.golang.org|direct'

# Telemetry follows APPDATA. GOTELEMETRYDIR is not settable.
$env:APPDATA = Join-Path $tools 'home\Roaming'
$env:LOCALAPPDATA = Join-Path $tools 'home\Local'
$env:TEMP = Join-Path $tools 'tmp'
$env:TMP = $env:TEMP

$goBin = Join-Path $env:GOROOT 'bin'
$goPathBin = Join-Path $env:GOPATH 'bin'
$prefix = "$goBin;$goPathBin;"
if (-not $env:Path.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
    $env:Path = "$prefix$env:Path"
}

$dirs = @(
    $env:GOPATH,
    $env:GOMODCACHE,
    $env:GOCACHE,
    $env:GOTMPDIR,
    $env:APPDATA,
    $env:LOCALAPPDATA
)
foreach ($dir in $dirs) {
    if (-not (Test-Path -LiteralPath $dir)) {
        New-Item -ItemType Directory -Path $dir | Out-Null
    }
}
