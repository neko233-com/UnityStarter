//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This exercises the built executable with a separate fake editor process and
// isolated user data. The real UnityStarter window must be closed beforehand.
func TestInstalledCLICreationAndGUIRecovery(t *testing.T) {
	appExe := os.Getenv("UNITYSTARTER_TEST_APP")
	if appExe == "" {
		t.Skip("set UNITYSTARTER_TEST_APP to a built UnityStarter executable")
	}
	base := t.TempDir()
	appData := filepath.Join(base, "roaming")
	localData := filepath.Join(base, "local")
	t.Setenv("APPDATA", appData)
	t.Setenv("LOCALAPPDATA", localData)
	t.Setenv("USERPROFILE", base)
	t.Setenv("ProgramFiles", "")
	t.Setenv("ProgramFiles(x86)", "")
	allow := filepath.Join(base, "allow")
	t.Setenv("UNITYSTARTER_E2E_ALLOW", allow)
	source := filepath.Join(base, "fake_editor.go")
	code := `package main
import ("os"; "path/filepath"; "time")
func main() {
 if len(os.Args) != 3 || os.Args[1] != "-createProject" { os.Exit(2) }
 for i:=0; i<300; i++ { if _,err:=os.Stat(os.Getenv("UNITYSTARTER_E2E_ALLOW")); err==nil { break }; time.Sleep(100*time.Millisecond) }
 if _,err:=os.Stat(os.Getenv("UNITYSTARTER_E2E_ALLOW")); err!=nil { os.Exit(3) }
 p:=os.Args[2]
 for _,d:=range []string{"ProjectSettings","Assets","Packages"} { if os.MkdirAll(filepath.Join(p,d),0755)!=nil { os.Exit(4) } }
 if os.WriteFile(filepath.Join(p,"ProjectSettings","ProjectVersion.txt"),[]byte("m_EditorVersion: 6000.0.2f1\n"),0644)!=nil { os.Exit(5) }
 if os.WriteFile(filepath.Join(p,"Packages","manifest.json"),[]byte("{\"dependencies\":{}}"),0644)!=nil { os.Exit(6) }
}`
	if err := os.WriteFile(source, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "Editors")
	editor := filepath.Join(root, "6000.0.2f1", "Editor", "Unity.exe")
	if err := os.MkdirAll(filepath.Dir(editor), 0o755); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", editor, source)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake editor: %v: %s", err, output)
	}
	if err := savePrefs(prefs{EditorRoots: []string{root}}); err != nil {
		t.Fatal(err)
	}
	hubDir := filepath.Join(appData, "UnityHub")
	if err := os.MkdirAll(hubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	h := hub{Kind: kindUnity, Registry: filepath.Join(hubDir, "projects-v1.json")}
	if err := os.WriteFile(h.Registry, []byte(`{"schema_version":"v1","data":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(base, "Project")
	cli := exec.Command(appExe, "new", project, "--editor", "6000.0.2f1")
	if output, err := cli.CombinedOutput(); err != nil || !strings.Contains(string(output), project) {
		t.Fatalf("CLI creation: %v: %s", err, output)
	}
	if len(pendingProjects()) != 1 || h.lists(project) {
		t.Fatal("CLI prematurely registered or lost the pending project")
	}
	gui := exec.Command(appExe)
	if err := gui.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gui.Process.Kill(); _ = gui.Wait() }()
	if err := os.WriteFile(allow, []byte("continue"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if h.lists(project) && len(pendingProjects()) == 0 {
			settings := loadPrefs()
			for _, path := range settings.PinnedProjects {
				if strings.EqualFold(path, project) {
					t.Log("CLI launched the fake editor; GUI recovered and registered the project")
					return
				}
			}
			t.Fatal("Hub registration succeeded without persisting the project")
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("GUI did not finish the pending project: pending=%d hub=%v", len(pendingProjects()), h.lists(project))
}
