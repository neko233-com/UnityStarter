Unicode true
RequestExecutionLevel user
Name "UnityStarter uninstall fixture"
!ifndef OUTPUT
  !error "Pass /DOUTPUT=<absolute output path>"
!endif
OutFile "${OUTPUT}"
InstallDir "$TEMP\unitystarter-uninstall-fixture"
SilentInstall silent
Section
  SetOutPath "$INSTDIR\Editor"
  FileOpen $0 "$OUTDIR\Unity.exe" w
  FileWrite $0 "fixture"
  FileClose $0
  WriteUninstaller "$OUTDIR\Uninstall.exe"
SectionEnd
Section "Uninstall"
  Sleep 1500
  Delete "$INSTDIR\Unity.exe"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
