$ErrorActionPreference = 'Stop'
if (-not [Environment]::Is64BitProcess) {
    $native = Get-Command pwsh.exe -All | Where-Object { $_.Source -notmatch '\(x86\)' } | Select-Object -First 1 -ExpandProperty Source
    if (-not $native) { throw 'A 64-bit PowerShell 7 is required.' }
    & $native -NoProfile -File $PSCommandPath
    exit $LASTEXITCODE
}
$msi = $env:MSI_PATH
$msiHash = $env:MSI_SHA256
$exeHash = $env:EXE_SHA256
if (-not $msi -or $msiHash -notmatch '^[0-9a-fA-F]{64}$' -or $exeHash -notmatch '^[0-9a-fA-F]{64}$') { throw 'Set MSI_PATH, MSI_SHA256 and EXE_SHA256.' }
$msi = (Resolve-Path -LiteralPath $msi).Path
if ((Get-FileHash -LiteralPath $msi -Algorithm SHA256).Hash -ine $msiHash) { throw 'MSI checksum mismatch.' }
$managed = Join-Path $env:LOCALAPPDATA 'Programs\UnityStarterMSI\UnityStarter.exe'
$legacy = Join-Path $env:LOCALAPPDATA 'Programs\UnityStarter\UnityStarter.exe'
foreach ($proc in Get-CimInstance Win32_Process -Filter "Name='UnityStarter.exe'") {
    if ($proc.ExecutablePath -and ($proc.ExecutablePath -ieq $managed -or $proc.ExecutablePath -ieq $legacy)) {
        $app = Get-Process -Id $proc.ProcessId -ErrorAction SilentlyContinue
        if ($app -and -not $app.HasExited) {
            $closed = $app.CloseMainWindow() -and $app.WaitForExit(5000)
            if (-not $closed) {
                if ($env:UNITYSTARTER_FORCE_CLOSE -ne '1') { throw "UnityStarter did not close: $($proc.ProcessId)." }
                Stop-Process -Id $proc.ProcessId -Force -ErrorAction Stop
                $app.WaitForExit()
            }
        }
    }
}
$log = Join-Path $env:TEMP 'UnityStarterMSI-install.log'
$msiexec = Join-Path $env:WINDIR 'System32\msiexec.exe'
$p = Start-Process -FilePath $msiexec -ArgumentList @('/i', "`"$msi`"", '/qn', '/norestart', '/L*v', "`"$log`"") -Wait -PassThru -WindowStyle Hidden
if ($p.ExitCode -ne 0) { throw "MSI installation returned $($p.ExitCode); see $log" }
if (-not (Test-Path -LiteralPath $managed) -or (Get-FileHash -LiteralPath $managed -Algorithm SHA256).Hash -ine $exeHash) { throw 'Installed executable checksum mismatch.' }
Start-Process -FilePath $managed -WorkingDirectory (Split-Path $managed) -WindowStyle Normal
Write-Host "Installed and opened: $managed"
