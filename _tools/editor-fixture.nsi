Unicode true
RequestExecutionLevel user
Name "UnityStarter editor fixture"
!ifndef OUTPUT
  !error "Pass /DOUTPUT=<absolute output path>"
!endif
OutFile "${OUTPUT}"
InstallDir "$TEMP\unitystarter-editor-fixture"
SilentInstall silent
Section
  SetOutPath "$INSTDIR\Editor"
  FileOpen $0 "$OUTDIR\Unity.exe" w
  FileWrite $0 "editor fixture"
  FileClose $0
SectionEnd
