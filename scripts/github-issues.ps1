# 提交和拉取 pureGavin/transHelper 的 GitHub Issues。
# 用法：
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/github-issues.ps1 save-token
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/github-issues.ps1 list [-State open|closed|all] [-Label bug]
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/github-issues.ps1 get -Number 1
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/github-issues.ps1 create -Title "标题" -BodyFile body.md -Label bug
# token 放在被忽略的 .tools/secrets/github-token。读取公开 issue 可以没有 token。

param(
    [Parameter(Mandatory = $true, Position = 0)]
    [ValidateSet('save-token', 'list', 'get', 'create')]
    [string]$Command,

    [ValidateSet('open', 'closed', 'all')]
    [string]$State = 'open',

    [string]$Label = '',

    [int]$Number = 0,

    [string]$Title = '',

    [string]$BodyFile = ''
)

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$root = Split-Path -Parent $PSScriptRoot
$tokenPath = Join-Path $root '.tools\secrets\github-token'
$apiRoot = 'https://api.github.com/repos/pureGavin/transHelper'

function Get-SavedToken {
    if (-not (Test-Path -LiteralPath $tokenPath)) {
        return ''
    }
    $text = [System.IO.File]::ReadAllText($tokenPath)
    if ($null -eq $text) {
        return ''
    }
    return $text.Trim()
}

function Save-GitHubToken {
    $secure = Read-Host -AsSecureString -Prompt 'GitHub token'
    $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
        $plain = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr)
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
    }
    if ([string]::IsNullOrWhiteSpace($plain)) {
        throw '没有读到 token'
    }
    $dir = Split-Path -Parent $tokenPath
    if (-not (Test-Path -LiteralPath $dir)) {
        New-Item -ItemType Directory -Path $dir | Out-Null
    }
    $utf8 = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText($tokenPath, $plain.Trim(), $utf8)
    $plain = $null
    & icacls.exe $tokenPath /inheritance:r /grant:r "${env:USERNAME}:(R,W)" | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "icacls 失败，退出码 $LASTEXITCODE"
    }
    Write-Output "已写入 $tokenPath ，只保留当前用户读写"
}

function Invoke-GitHub {
    param(
        [string]$Method,
        [string]$Uri,
        [string]$JsonBody,
        [string]$Token,
        [switch]$Creating
    )

    $request = [System.Net.HttpWebRequest]::Create($Uri)
    $request.Method = $Method
    $request.Accept = 'application/vnd.github+json'
    $request.UserAgent = 'transhelper-issues'
    $request.Headers.Add('X-GitHub-Api-Version', '2022-11-28')
    $request.AutomaticDecompression = [System.Net.DecompressionMethods]::GZip -bor [System.Net.DecompressionMethods]::Deflate
    if ($Token) {
        $request.Headers.Add('Authorization', "Bearer $Token")
    }
    if ($JsonBody) {
        $bytes = [System.Text.Encoding]::UTF8.GetBytes($JsonBody)
        $request.ContentType = 'application/json; charset=utf-8'
        $request.ContentLength = $bytes.Length
        $stream = $request.GetRequestStream()
        $stream.Write($bytes, 0, $bytes.Length)
        $stream.Close()
    }

    $status = 0
    $link = ''
    $text = ''
    try {
        $response = $request.GetResponse()
        $status = [int]$response.StatusCode
        $link = $response.Headers['Link']
        $reader = New-Object System.IO.StreamReader($response.GetResponseStream(), [System.Text.Encoding]::UTF8)
        $text = $reader.ReadToEnd()
        $reader.Close()
        $response.Close()
    } catch [System.Net.WebException] {
        $errResponse = $_.Exception.Response
        if (-not $errResponse) {
            throw
        }
        $status = [int]$errResponse.StatusCode
        $reader = New-Object System.IO.StreamReader($errResponse.GetResponseStream(), [System.Text.Encoding]::UTF8)
        $text = $reader.ReadToEnd()
        $reader.Close()
        $errResponse.Close()
        if ($Creating -and ($status -eq 401 -or $status -eq 403 -or $status -eq 404)) {
            throw 'token 缺失、已过期，或没有本仓库的 Issues 读写权限'
        }
        throw "GitHub 接口返回 $status"
    }

    $data = $null
    if ($text) {
        $data = $text | ConvertFrom-Json
    }
    return @{ Status = $status; Data = $data; Link = $link }
}

function Get-NextUri([string]$LinkHeader) {
    if (-not $LinkHeader) {
        return ''
    }
    foreach ($part in $LinkHeader.Split(',')) {
        if ($part -match '<([^>]+)>;\s*rel="next"') {
            return $Matches[1]
        }
    }
    return ''
}

function Format-IssueLine($Issue) {
    $names = @()
    foreach ($label in @($Issue.labels)) {
        if ($label.name) {
            $names += $label.name
        }
    }
    $labelText = ''
    if ($names.Count -gt 0) {
        $labelText = '[' + ($names -join ', ') + '] '
    }
    Write-Output ("#{0} {1}{2}" -f $Issue.number, $labelText, $Issue.title)
    Write-Output ("    评论 {0}  更新 {1}" -f $Issue.comments, $Issue.updated_at)
    Write-Output ("    {0}" -f $Issue.html_url)
}

function Get-AllIssues {
    param([string]$Token)

    $query = "state=$State&per_page=100"
    if ($Label) {
        $query += '&labels=' + [Uri]::EscapeDataString($Label)
    }
    $uri = "$apiRoot/issues?$query"
    $found = @()
    while ($uri) {
        $page = Invoke-GitHub -Method 'GET' -Uri $uri -Token $Token
        foreach ($item in @($page.Data)) {
            if ($item.PSObject.Properties.Name -contains 'pull_request') {
                continue
            }
            if (-not $item.number) {
                continue
            }
            $found += $item
        }
        $uri = Get-NextUri $page.Link
    }
    return $found
}

function Show-IssueList {
    param([string]$Token)
    $issues = @(Get-AllIssues -Token $Token)
    if ($issues.Count -eq 0) {
        Write-Output '没有 issue'
        return
    }
    foreach ($issue in $issues) {
        Format-IssueLine $issue
    }
}

function Show-Issue {
    param([string]$Token)
    if ($Number -le 0) {
        throw 'get 需要 -Number'
    }
    $issue = Invoke-GitHub -Method 'GET' -Uri "$apiRoot/issues/$Number" -Token $Token
    $item = $issue.Data
    if ($item.PSObject.Properties.Name -contains 'pull_request') {
        throw "#$Number 是 pull request，不是 issue"
    }
    Format-IssueLine $item
    Write-Output ("状态：" + $item.state)
    Write-Output '----'
    if ([string]::IsNullOrWhiteSpace($item.body)) {
        Write-Output '（没有正文）'
    } else {
        Write-Output $item.body
    }
    Write-Output '----'
    $uri = "$apiRoot/issues/$Number/comments?per_page=100"
    $count = 0
    while ($uri) {
        $page = Invoke-GitHub -Method 'GET' -Uri $uri -Token $Token
        foreach ($comment in @($page.Data)) {
            if (-not $comment.id) {
                continue
            }
            $count++
            $user = ''
            if ($comment.user) {
                $user = $comment.user.login
            }
            Write-Output ("评论 {0}  {1}  {2}" -f $count, $user, $comment.created_at)
            Write-Output $comment.body
            Write-Output '----'
        }
        $uri = Get-NextUri $page.Link
    }
    if ($count -eq 0) {
        Write-Output '没有评论'
    }
}

function Test-IssueSection([string]$Body, [string]$Name) {
    $match = [regex]::Match($Body, "(?ms)^##\s*$([regex]::Escape($Name))\s*\r?\n(.*?)(?=^##\s|\z)")
    if (-not $match.Success) {
        throw "正文缺少「## $Name」"
    }
    if ([string]::IsNullOrWhiteSpace($match.Groups[1].Value)) {
        throw "「## $Name」不能为空"
    }
}

function Test-PublicText([string]$Text, [string]$Token) {
    if ($Token -and $Text.Contains($Token)) {
        throw '标题或正文里有 token，issue 是公开的，已拒绝'
    }
    $blocked = @(
        'github_pat_[0-9A-Za-z_]+',
        'ghp_[0-9A-Za-z]+',
        'gho_[0-9A-Za-z]+',
        '(?<![0-9])192\.168\.\d{1,3}\.\d{1,3}',
        '(?<![0-9])10\.\d{1,3}\.\d{1,3}\.\d{1,3}',
        '(?<![0-9])172\.(1[6-9]|2[0-9]|3[01])\.\d{1,3}\.\d{1,3}'
    )
    foreach ($pattern in $blocked) {
        if ($Text -match $pattern) {
            throw '标题或正文里有 token、密钥或内网地址，issue 是公开的，已拒绝'
        }
    }
    $secretLine = [regex]::Match($Text, '(?i)(?:api_key|密钥)\s*[=:：]\s*(\S+)')
    if ($secretLine.Success) {
        $value = $secretLine.Groups[1].Value.Trim('"').Trim("'")
        if ($value -and $value -notmatch '^(留空|空|无|none|null)$' -and $value.Length -ge 6) {
            throw '标题或正文里有 token、密钥或内网地址，issue 是公开的，已拒绝'
        }
    }
}

function New-Issue {
    param([string]$Token)
    if (-not $Token) {
        throw 'token 缺失、已过期，或没有本仓库的 Issues 读写权限'
    }
    if ([string]::IsNullOrWhiteSpace($Title)) {
        throw 'create 需要 -Title'
    }
    if (-not $Label) {
        throw 'create 需要 -Label'
    }
    if (-not (Test-Path -LiteralPath $BodyFile)) {
        throw "找不到正文文件 $BodyFile"
    }
    $body = [System.IO.File]::ReadAllText((Resolve-Path -LiteralPath $BodyFile))
    foreach ($name in @('问题', '复现步骤', '期望结果')) {
        Test-IssueSection -Body $body -Name $name
    }
    Test-PublicText -Text ($Title + "`n" + $body) -Token $Token

    Add-Type -AssemblyName System.Web.Extensions
    $serializer = New-Object System.Web.Script.Serialization.JavaScriptSerializer
    $serializer.MaxJsonLength = 67108864
    $payload = @{
        title  = $Title
        body   = $body
        labels = @($Label)
    }
    $json = $serializer.Serialize($payload)
    $created = Invoke-GitHub -Method 'POST' -Uri "$apiRoot/issues" -JsonBody $json -Token $Token -Creating
    $item = $created.Data
    Write-Output ("已创建 #{0}" -f $item.number)
    Write-Output $item.title
    Write-Output $item.html_url
}

$token = Get-SavedToken
switch ($Command) {
    'save-token' { Save-GitHubToken }
    'list' { Show-IssueList -Token $token }
    'get' { Show-Issue -Token $token }
    'create' { New-Issue -Token $token }
}
