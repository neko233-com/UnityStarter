Unicode true
RequestExecutionLevel user
Name "UnityStarter editor fixture"
!ifndef OUTPUT
  !error "Pass /DOUTPUT=<absolute output path>"
!endif
!ifndef EDITOR_EXE
  !error "Pass /DEDITOR_EXE=<versioned Unity.exe fixture path>"
!endif
OutFile "${OUTPUT}"
InstallDir "$TEMP\unitystarter-editor-fixture"
SilentInstall silent
Section
  SetOutPath "$INSTDIR\Editor"
  File /oname=Unity.exe "${EDITOR_EXE}"
  SetOutPath "$INSTDIR\Editor\Data\Managed"
  FileOpen $0 "$OUTDIR\UnityEngine.dll" w
  FileWrite $0 "fixture"
  FileClose $0
  FileOpen $0 "$OUTDIR\UnityEditor.dll" w
  FileWrite $0 "fixture"
  FileClose $0
  CreateDirectory "$INSTDIR\Editor\Data\Resources"
  CreateDirectory "$INSTDIR\Editor\Data\PlaybackEngines"
SectionEnd
