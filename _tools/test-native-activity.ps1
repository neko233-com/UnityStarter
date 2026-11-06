$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repo = Split-Path -Parent $PSScriptRoot
$tempBase = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$fixture = Join-Path $tempBase ('TestUnityStarterActivity-' + [Guid]::NewGuid().ToString('N'))
$resolvedFixture = [IO.Path]::GetFullPath($fixture)
if (-not $resolvedFixture.StartsWith($tempBase, [StringComparison]::OrdinalIgnoreCase) -or
    -not [IO.Path]::GetFileName($resolvedFixture).StartsWith('TestUnityStarterActivity-', [StringComparison]::Ordinal)) {
    throw 'Unsafe fixture directory'
}
$process = $null
try {
    New-Item -ItemType Directory -Path $fixture | Out-Null
    $source = Join-Path $fixture 'source'
    New-Item -ItemType Directory -Path $source | Out-Null
    Get-ChildItem -LiteralPath $repo -Filter '*.go' -File | Where-Object { $_.Name -ne 'main.go' -and -not $_.Name.EndsWith('_test.go') } | Copy-Item -Destination $source
    Copy-Item -LiteralPath (Join-Path $repo 'go.mod'), (Join-Path $repo 'go.sum') -Destination $source
    Copy-Item -LiteralPath (Join-Path $repo 'locales') -Destination $source -Recurse
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'activity-fixture.go.txt') -Destination (Join-Path $source 'main.go')
    $archive = Join-Path $fixture 'node.zip'
    Invoke-WebRequest -Uri 'https://mirrors.cloud.tencent.com/nodejs-release/v18.14.1/node-v18.14.1-win-x64.zip' -OutFile $archive -TimeoutSec 120
    if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne 'B0EC709961AB9B2E46D616D33F26D0FF1548B02045C7650F018E809E7A6CBBDA') { throw 'Official Node.js archive digest mismatch' }
    $exe = Join-Path $fixture 'activity-fixture.exe'
    Push-Location $source
    try { & go build -ldflags='-H=windowsgui' -o $exe .; if ($LASTEXITCODE -ne 0) { throw 'Fixture build failed' } } finally { Pop-Location }
    New-Item -ItemType Directory -Path (Join-Path $fixture 'appdata'), (Join-Path $fixture 'localappdata') | Out-Null
    $oldAppData = $env:APPDATA; $oldLocalAppData = $env:LOCALAPPDATA; $oldFixture = $env:UNITYSTARTER_ACTIVITY_FIXTURE
    try {
        $env:APPDATA = Join-Path $fixture 'appdata'
        $env:LOCALAPPDATA = Join-Path $fixture 'localappdata'
        $env:UNITYSTARTER_ACTIVITY_FIXTURE = $fixture
        # The fixture is an interactive UI test and must have a visible window.
        $process = Start-Process -FilePath $exe -WindowStyle Normal -PassThru -RedirectStandardError (Join-Path $fixture 'stderr.log')
    } finally {
        $env:APPDATA = $oldAppData; $env:LOCALAPPDATA = $oldLocalAppData; $env:UNITYSTARTER_ACTIVITY_FIXTURE = $oldFixture
    }
    Add-Type -AssemblyName UIAutomationClient
    Add-Type -AssemblyName UIAutomationTypes
    $deadline = [DateTime]::UtcNow.AddSeconds(30)
    do { $process.Refresh(); if ($process.HasExited) { throw "Fixture exited early: $($process.ExitCode)" }; if ($process.MainWindowHandle -ne 0) { break }; Start-Sleep -Milliseconds 200 } while ([DateTime]::UtcNow -lt $deadline)
    if ($process.MainWindowHandle -eq 0) { throw 'Fixture window did not open' }
    $root = [System.Windows.Automation.AutomationElement]::FromHandle($process.MainWindowHandle)
    function Find-Element([string]$name) {
        $condition = [System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty, $name)
        return $root.FindFirst([System.Windows.Automation.TreeScope]::Descendants, $condition)
    }
    function Invoke-Element([string]$name) {
        $element = Find-Element $name
        if (-not $element) { throw "Missing control: $name" }
        $element.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
    }
    function Wait-Text([string]$pattern, [int]$seconds = 30) {
        $deadline = [DateTime]::UtcNow.AddSeconds($seconds)
        do {
            $process.Refresh(); if ($process.HasExited) { throw "Fixture exited early: $($process.ExitCode)" }
            try { $all = $root.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition) }
            catch { Start-Sleep -Milliseconds 200; continue }
            foreach ($item in $all) { if ($item.Current.Name -match $pattern) { return $item.Current.Name } }
            Start-Sleep -Milliseconds 200
        } while ([DateTime]::UtcNow -lt $deadline)
        throw "Timed out waiting for: $pattern"
    }
    [void](Wait-Text '^Nodejs$' 30)
    $node = Find-Element 'Nodejs'
    if (-not $node -or -not $node.Current.IsEnabled) { throw 'Node.js selection missing' }
    $node.GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern).Toggle()
    Start-Sleep -Milliseconds 200
    Invoke-Element '安装所选（1）'
    [void](Wait-Text '下载.*%' 30)
    Invoke-Element '后台运行'
    $progress = Wait-Text '[0-9.]+ [KM]B / [0-9.]+ [KM]B' 30
    Write-Output "Background download progress: $progress"
    $cancel = Find-Element '取消'
    if (-not $cancel) { throw 'Background cancellation missing' }
    $cancel.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
    [void](Wait-Text '已停止|已取消' 30)
    $nodeDir = Join-Path $fixture 'editor\Editor\Data\PlaybackEngines\OpenHarmonyPlayer\Nodejs'
    $nodeExe = Join-Path $nodeDir 'node-v18.14.1-win-x64\node.exe'
    $stateDir = Join-Path $fixture 'appdata\UnityStarter\modules'
    if (Test-Path -LiteralPath $nodeDir) { throw 'Canceled download left a partial module installation' }
    if ((Test-Path -LiteralPath $stateDir) -and @(Get-ChildItem -LiteralPath $stateDir -File).Count -gt 0) { throw 'Canceled download recorded an installed module' }
    Write-Output 'Cancellation left no module directory or installed record'
    Invoke-Element '添加模块'
    Invoke-Element '安装所选（1）'
    [void](Wait-Text '下载.*%' 30)
    Invoke-Element '后台运行'
    [void](Wait-Text '安装所选模块完成|所选模块已安装|安装完成' 180)
    if (-not (Test-Path -LiteralPath $nodeExe)) { throw 'Completed module missing' }
    $version = & $nodeExe --version
    if ($version -ne 'v18.14.1') { throw "Unexpected Node.js version: $version" }
    Write-Output "Background retry installed Node.js: $version"
    $root.GetCurrentPattern([System.Windows.Automation.WindowPattern]::Pattern).Close()
    if (-not $process.WaitForExit(30000)) { throw 'Fixture did not exit' }
    if ($process.ExitCode -ne 0) { throw "Fixture verification failed: $($process.ExitCode)" }
} catch {
    $log = Join-Path $fixture 'stderr.log'
    if (Test-Path -LiteralPath $log) { $message = Get-Content -LiteralPath $log -Raw; if ($message) { Write-Output $message } }
    throw
} finally {
    if ($process) { $process.Refresh(); if (-not $process.HasExited) { Stop-Process -Id $process.Id -Force; [void]$process.WaitForExit(10000) } }
    if (Test-Path -LiteralPath $resolvedFixture) { Remove-Item -LiteralPath $resolvedFixture -Recurse -Force }
}
