$ErrorActionPreference = 'Stop'
if (-not [Environment]::Is64BitProcess) {
    $native = Get-Command pwsh.exe -All | Where-Object { $_.Source -notmatch '\(x86\)' } | Select-Object -First 1 -ExpandProperty Source
    if (-not $native) { throw 'A 64-bit PowerShell 7 is required.' }
    & $native -NoProfile -File $PSCommandPath
    exit $LASTEXITCODE
}
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    $version = (Get-Content 'mygo.json' -Raw | ConvertFrom-Json).version
    $tag = "v$version"
    if ($env:RELEASE_TAG -and $env:RELEASE_TAG -cne $tag) { throw 'Release tag does not match mygo.json.' }
    $tagCommit = git rev-parse "refs/tags/$tag^{}"
    if ($LASTEXITCODE -ne 0) { throw 'Release tag does not exist.' }
    $head = git rev-parse HEAD
    if ($LASTEXITCODE -ne 0 -or $tagCommit -ne $head) { throw 'Release tag does not point at HEAD.' }
    $dir = Join-Path $root 'build/windows-amd64'
    $sumFile = Join-Path $dir 'SHA256SUMS.txt'
    $names = @('UnityStarter.exe', "unitystarter-setup-$version-amd64.exe", "unitystarter-$version-amd64.msi")
    $sums = @{}
    foreach ($line in Get-Content -LiteralPath $sumFile) {
        if ($line -notmatch '^([0-9a-f]{64})  ([A-Za-z0-9._-]+)$') { throw 'Invalid checksum manifest.' }
        $sums[$Matches[2]] = $Matches[1]
    }
    $assets = foreach ($name in $names) {
        $path = Join-Path $dir $name
        if (-not (Test-Path -LiteralPath $path) -or -not $sums.ContainsKey($name)) { throw "Missing release asset: $name" }
        if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -cne $sums[$name]) { throw "Checksum mismatch: $name" }
        $path
    }
    $notes = Join-Path $dir 'release-notes.txt'
    @'
UnityStarter 使用原生界面和一体化标题栏；左侧切换 Unity 与团结引擎。项目列表读取 Editor 的最近项目，优先显示并在后台补充版本控制状态；没有 Hub 时也可本地收藏。项目可以选择已安装的同类编辑器版本，并在跨版本打开前确认。安装页支持搜索、筛选、模块清单和编辑器自带的卸载程序。MSI 版本支持校验下载的自动更新。

模块安装支持有官方校验值且无需额外许可或依赖处理的受支持独立模块，以及 SHA-256 校验的官方 OpenJDK；SDK、NDK 等其余模块请使用官方 Hub。独立安装程序需要管理员权限时由 Windows 提示授权；OpenJDK ZIP 需要当前用户有编辑器目录写入权限。

Unity Personal 首次激活需要登录 Unity Hub；请查看 README 中的已知限制。发布包未经代码签名，请使用 SHA256SUMS.txt 校验文件。
'@ | Set-Content -LiteralPath $notes -Encoding utf8
    gh release create $tag @assets $sumFile --verify-tag --title "UnityStarter $version" --notes-file $notes --generate-notes
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally { Pop-Location }
