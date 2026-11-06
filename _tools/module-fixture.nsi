Unicode true
RequestExecutionLevel user
Name "UnityStarter module fixture"
!ifndef OUTPUT
  !error "Pass /DOUTPUT=<absolute output path>"
!endif
OutFile "${OUTPUT}"
InstallDir "$TEMP\unitystarter-fixture"
SilentInstall silent
Section
  SetOutPath "$INSTDIR\Editor\Data\PlaybackEngines\WebGLSupport"
  FileOpen $0 "$OUTDIR\fixture.txt" w
  FileWrite $0 "fixture"
  FileClose $0
SectionEnd
