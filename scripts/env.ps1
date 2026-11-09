# 用点号加载，变量才会留在当前会话：
#   . .\scripts\env.ps1
# 用 -File 运行只影响那个进程，脚本结束后环境就恢复了。
# 这里不写注册表，也不改用户配置。关掉终端后，环境变量回到原来的值。

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

# 遥测目录跟着 APPDATA 走。GOTELEMETRYDIR 不能单独设置。
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
