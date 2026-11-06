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

新建项目可区分同版本的多个编辑器安装目录，并校验项目名称；版本清单刷新失败时保留当前已获取的列表。

编辑器扫描保留同版本的不同安装目录，支持选择版本父目录、版本目录或 Editor 文件夹，并在 Windows 读取程序的实际版本。

Unity 模块清单按版本缓存；离线时可查看保存的清单和经文件核对的本地安装状态，并可在管理窗口刷新清单。

下载进度与单连接临时文件使用可识别的独立名称，异常退出后可清理本程序的残留，同时保留不属于安装计划的文件。

支持校验、隔离解压并安装团结引擎官方离线文档；文档页可打开本机手册和脚本 API，已有文档不会被覆盖。

下载页可按版本号查询 Unity 官方历史版本，CLI 安装也会查询历史清单；兼容旧版官方清单的十六进制 MD5 编码和近似大小，仍会校验完整下载。

两个引擎的官方版本清单独立获取并缓存；团结引擎无需安装官方 Hub 即可获取版本和模块列表，网络不可用时使用已缓存清单。

支持具备校验值和服务器标识的分段下载在断网或异常退出后续传，已校验的完整文件可复用；下载页显示速度并可丢弃已保存进度。Android 多包任务可保留中断缓存，重新确认许可后继续；提交中断可校验并恢复，解压中异常退出可能留下隐藏暂存目录。

模块安装支持受支持的独立模块、官方 OpenJDK，以及可完整核对依赖的 Unity Android SDK／NDK；安装 Android 工具前需阅读并接受 Google 许可。团结旧版 Android 依赖及其他不符合条件的模块请使用官方 Hub。独立安装程序需要管理员权限时由 Windows 提示授权；ZIP 模块需要当前用户有编辑器目录写入权限。

Unity 7 Android 多包安装已验证官方清单、隔离安装和失败清理；真实大包安装与 Android 构建尚待接受许可后验收。

Unity Personal 首次激活需要登录 Unity Hub；请查看 README 中的已知限制。发布包未经代码签名，请使用 SHA256SUMS.txt 校验文件。
'@ | Set-Content -LiteralPath $notes -Encoding utf8
    gh release create $tag @assets $sumFile --verify-tag --title "UnityStarter $version" --notes-file $notes --generate-notes
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally { Pop-Location }
