; UnityStarter Windows installer hooks (mygo windows.installerInclude).

; MyGo's uninstaller removes $INSTDIR recursively. Older releases placed
; editors there, so stop before any uninstall changes when that folder exists.
Function un.onInit
  IfFileExists "$INSTDIR\Editors\*.*" 0 safe
  IfSilent +2
  MessageBox MB_OK|MB_ICONEXCLAMATION "UnityStarter found editors in $INSTDIR\Editors. Move them outside the app folder before uninstalling."
  Abort
  safe:
FunctionEnd

!macro RunRegisterCliScript
  StrCpy $R9 "$WINDIR\Sysnative\WindowsPowerShell\v1.0\powershell.exe"
  IfFileExists $R9 +2
  StrCpy $R9 "$SYSDIR\WindowsPowerShell\v1.0\powershell.exe"
  ExecWait '"$R9" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\register-cli.ps1" -InstallDir "$INSTDIR"' $R8
  StrCmp $R8 "0" +2
  Abort "UnityStarter CLI registration failed."
!macroend

!macro customInstall
  !insertmacro RunRegisterCliScript
!macroend

!macro customUnInstall
  StrCpy $R9 "$WINDIR\Sysnative\WindowsPowerShell\v1.0\powershell.exe"
  IfFileExists $R9 +2
  StrCpy $R9 "$SYSDIR\WindowsPowerShell\v1.0\powershell.exe"
  ExecWait '"$R9" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\register-cli.ps1" -InstallDir "$INSTDIR" -Unregister' $R8
!macroend
