package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func templateFixture(t *testing.T, kind, id, version string) projectTemplate {
	t.Helper()
	file := filepath.Join(t.TempDir(), id+"-"+version+".tgz")
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	meta := map[string]any{"name": id, "displayName": "Universal 3D", "version": version, "type": "template", "dependencies": map[string]string{"com.unity.render-pipelines.universal": "14.0.11"}}
	data, _ := json.Marshal(meta)
	for name, data := range map[string][]byte{"package/package.json": data, "package/ProjectData~/Assets/Settings/UniversalRP.asset": []byte("URP fixture"), "package/ProjectData~/Packages/manifest.json": []byte(`{"dependencies":{"com.unity.render-pipelines.universal":"14.0.11"}}`)} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	for _, closer := range []io.Closer{tw, gz, f} {
		if err := closer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	item, err := readTemplateArchive(file, kind)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func TestProjectTemplateCreationPassesFullArchiveAndRejectsChangedTemplate(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	template := templateFixture(t, kindUnity, "com.unity.template.urp-blank", "14.0.0")
	e := editorInstall{Kind: kindUnity, Version: "2022.3.51f1", Exe: "editor"}
	dir := filepath.Join(t.TempDir(), "URP Game")
	var args []string
	if err := startProjectCreationFromTemplate(dir, e, template, func(_ string, argv []string, _ string) error { args = argv; return nil }); err != nil {
		t.Fatal(err)
	}
	if len(args) != 4 || args[0] != "-createProject" || args[1] != dir || args[2] != "-cloneFromTemplate" || args[3] != template.Archive {
		t.Fatalf("template not passed to Editor: %q", args)
	}
	if len(pendingProjects()) != 1 || pendingProjectReady(pendingProjects()[0]) {
		t.Fatal("creation was falsely reported ready before Editor materialized the template")
	}
	template.ID = "com.unity.template.hdrp-blank"
	other := filepath.Join(t.TempDir(), "must-not-exist")
	if err := startProjectCreationFromTemplate(other, e, template, func(string, []string, string) error { t.Fatal("wrong template launched"); return nil }); err == nil {
		t.Fatal("changed template accepted")
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatal("invalid template changed project directory")
	}
}

func TestTemplateCacheRequiresDigestAndEngineIdentity(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	item := templateFixture(t, kindUnity, "com.unity.template.urp-blank", "14.0.0")
	data, err := os.ReadFile(item.Archive)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha1.Sum(data)
	item.Integrity = "sha1-" + base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(sum[:])))
	item.Archive = templateCachePath(kindUnity, item)
	if err := os.MkdirAll(filepath.Dir(item.Archive), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(item.Archive, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyProjectTemplate(item, kindUnity); err != nil {
		t.Fatal(err)
	}
	if err := verifyProjectTemplate(item, kindTuanjie); err == nil {
		t.Fatal("Unity template entered Tuanjie group")
	}
	f, _ := os.OpenFile(item.Archive, os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.Write([]byte("tampered"))
	_ = f.Close()
	if err := verifyProjectTemplate(item, kindUnity); err == nil {
		t.Fatal("modified cached archive accepted")
	}
}

func TestNativeTemplateCatalogCategoriesAndVersionIsolation(t *testing.T) {
	oldChina, oldGraph := templateChinaBase, templateGraphURL
	t.Cleanup(func() { templateChinaBase, templateGraphURL = oldChina, oldGraph })
	branch := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Variables struct {
				Versions []string `json:"supportedUnityEditorVersions"`
			} `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Variables.Versions) == 1 {
			branch = body.Variables.Versions[0]
		}
		fmt.Fprint(w, `{"data":{"getTemplates":{"edges":[{"node":{"name":"Universal 3D","packageName":"com.unity.template.urp-blank","type":"CORE","renderPipeline":"URP","versions":[{"name":"14.0.0","size":{"value":10},"tarball":{"url":"https://download.packages.unity.com/a.tgz","integrity":"sha1-AAAAAAAAAAAAAAAAAAAAAAAAAAA="}}]}},{"node":{"name":"Universal sample","packageName":"com.unity.template.urp-sample","type":"SAMPLE","versions":[{"name":"14.0.0","tarball":{"url":"https://download.packages.unity.com/b.tgz","integrity":"sha1-AAAAAAAAAAAAAAAAAAAAAAAAAAA="}}]}}]}}}`)
	}))
	defer server.Close()
	templateChinaBase = server.URL + "/"
	templateGraphURL = server.URL
	items, err := fetchProjectTemplates(context.Background(), editorInstall{Kind: kindUnity, Version: "2022.3.51f1"})
	if err != nil || len(items) != 2 || branch != "2022.3" {
		t.Fatalf("catalog: %+v %q %v", items, branch, err)
	}
	a := newApp()
	a.newTemplates = items
	a.newTemplateCategory = "SAMPLE"
	if got := a.filteredProjectTemplates(); len(got) != 1 || got[0].ID != "com.unity.template.urp-sample" {
		t.Fatal("sample templates unavailable")
	}
	a.newTemplateCategory = ""
	a.newTemplateQuery = "URP"
	if len(a.filteredProjectTemplates()) != 2 {
		t.Fatal("pipeline/package search unavailable")
	}
	a.newTemplateQuery = ""
	a.chooseDefaultProjectTemplate()
	if a.newTemplateID != items[0].key() {
		t.Fatal("URP was not the default")
	}
	a.loadProjectTemplates(editorInstall{Kind: kindTuanjie, Version: "2022.3.62t17", Exe: "Tuanjie.exe"}, false)
	if len(a.newTemplates) != 0 || a.newTemplateID != "" {
		t.Fatal("templates leaked across editor change")
	}
}

func newTemplateUIFixture(t *testing.T) *app {
	a := fixture()
	a.jobs = nil
	a.editors = []editorInstall{{Kind: kindUnity, Version: "2022.3.51f1", Exe: "Editor.exe"}}
	a.openNewProject()
	a.newDir, a.newName = t.TempDir(), "URP Game"
	item := templateFixture(t, kindUnity, "com.unity.template.urp-blank", "14.0.0")
	a.newTemplates = []projectTemplate{item}
	for _, extra := range []struct{ id, name, pipeline, category string }{{"universal-2d", "Universal 2D", "URP", "CORE"}, {"hdrp-blank", "High Definition 3D", "HDRP", "CORE"}, {"urp-sample", "Universal 3D sample", "URP", "SAMPLE"}, {"fps", "FPS Microgame", "BUILT_IN", "LEARNING"}} {
		copy := item
		copy.ID = "com.unity.template." + extra.id
		copy.Name, copy.Pipeline, copy.Category, copy.Archive = extra.name, extra.pipeline, extra.category, ""
		copy.Size = 64 << 20
		copy.URL = "https://download.packages.unity.com/a.tgz"
		a.newTemplates = append(a.newTemplates, copy)
	}
	a.chooseDefaultProjectTemplate()
	return a
}

func TestNativeTemplatePickerPreservesSelectionAndLaunchesURP(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	for _, width := range []int{900, 1400} {
		a := newTemplateUIFixture(t)
		launched := false
		a.launchEditor = func(_ string, args []string, _ string) error {
			launched = len(args) == 4 && args[2] == "-cloneFromTemplate"
			return nil
		}
		tt := ui.NewTester(a.view, width, 760)
		if err := tt.Click(locales.TemplateSample()); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click("Universal 3D sample"); err != nil {
			t.Fatal(err)
		}
		if !tt.HasText(locales.TemplateDownload()) {
			t.Fatal("remote template has no native download action")
		}
		if err := tt.Click(locales.TemplateLocal()); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click("Universal 3D"); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			tt.Frame()
		}
		if err := tt.Click(locales.ActionCreate()); err != nil {
			t.Fatal(err)
		}
		if !launched || a.newOpen {
			t.Fatalf("URP creation did not launch: %s", a.newError)
		}
	}
}

func TestTemplateDownloadBackgroundKeepsTaskAndCancellation(t *testing.T) {
	a := newTemplateUIFixture(t)
	cancelled := false
	item, _ := a.selectedProjectTemplate()
	a.templateJob = &templateDownloadJob{Template: item, Editor: a.editors[0], Busy: true, Done: 25, Total: 100, cancel: func() { cancelled = true }}
	if !a.editorWorkActive() {
		t.Fatal("updating/restarting must wait for template downloads")
	}
	tt := ui.NewTester(a.view, 900, 560)
	if err := tt.Click(locales.ActionBackground()); err != nil {
		t.Fatal(err)
	}
	if a.newOpen || a.page != pageActivity || !a.templateJob.Busy {
		t.Fatal("backgrounding lost template download")
	}
	if err := tt.Click(locales.ActionCancel()); err != nil {
		t.Fatal(err)
	}
	if !cancelled {
		t.Fatal("template cancellation was not sent")
	}
	a.templateJob.Busy = false
	if a.editorWorkActive() {
		t.Fatal("completed template download still blocks updates")
	}
	tt.Frame()
	if err := tt.Click(locales.TemplateView()); err != nil {
		t.Fatal(err)
	}
	if !a.newOpen || a.newTemplateID != item.key() || a.newEditor != a.templateJob.Editor.Exe {
		t.Fatal("resuming template changed selection or Editor")
	}
}

func TestProjectTemplatesLive(t *testing.T) {
	if os.Getenv("PROJECT_TEMPLATES_LIVE") != "1" {
		t.Skip("set PROJECT_TEMPLATES_LIVE=1 for official catalogs and installed Editor template archives")
	}
	for _, e := range []editorInstall{
		{Kind: kindUnity, Version: "7000.0.0a16", Exe: filepath.Join(os.Getenv("LOCALAPPDATA"), `UnityStarter\Editors\7000.0.0a16\Editor\Unity.exe`)},
		{Kind: kindTuanjie, Version: "2022.3.62t17", Exe: `D:\IDE\TuanJieEditor\2022.3.62t17\Editor\Tuanjie.exe`},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		remote, err := fetchProjectTemplates(ctx, e)
		cancel()
		if err != nil || len(remote) == 0 {
			t.Fatalf("%s catalog: %v (%d)", e.Kind, err, len(remote))
		}
		local := localProjectTemplates(e)
		if len(local) == 0 {
			t.Fatalf("%s bundled templates missing", e.Kind)
		}
		items := mergeProjectTemplates(e, local, remote)
		urp := false
		for _, item := range items {
			if item.Pipeline == "URP" {
				urp = true
			}
			if item.Archive != "" {
				if err := verifyProjectTemplate(item, e.Kind); err != nil {
					t.Fatal(err)
				}
			}
		}
		if !urp {
			t.Fatalf("%s URP missing", e.Kind)
		}
		t.Logf("%s %s: remote=%d local=%d merged=%d", e.Kind, e.Version, len(remote), len(local), len(items))
		if dir := os.Getenv("TEMPLATES_SNAPSHOT"); dir != "" {
			old := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return "zh-Hans" })
			for _, size := range []struct{ w, h int }{{1400, 820}, {900, 560}} {
				a := newApp()
				a.editors = []editorInstall{e}
				a.activeEngine = e.Kind
				a.newEngine = e.Kind
				a.newEditor = e.Exe
				a.newOpen = true
				a.newName = "My URP Game"
				a.newDir = `D:\Code\Games`
				a.newTemplates = items
				a.newTemplateSource = templateEditorKey(e)
				a.chooseDefaultProjectTemplate()
				tt := ui.NewTester(a.view, size.w, size.h)
				if err := writePNG(filepath.Join(dir, fmt.Sprintf("new-project-%s-%d.png", e.Kind, size.w)), tt.Image()); err != nil {
					t.Fatal(err)
				}
			}
			i18n.SetLocaleFunc(func() string { return old })
		}
	}
}

func TestProjectTemplateEditorCreationLive(t *testing.T) {
	if os.Getenv("PROJECT_TEMPLATE_CREATE_LIVE") != "1" {
		t.Skip("set PROJECT_TEMPLATE_CREATE_LIVE=1 to create an isolated URP project with the installed Tuanjie Editor")
	}
	t.Setenv("APPDATA", t.TempDir())
	e := editorInstall{Kind: kindTuanjie, Version: "2022.3.62t17", Exe: `D:\IDE\TuanJieEditor\2022.3.62t17\Editor\Tuanjie.exe`}
	items := localProjectTemplates(e)
	var chosen projectTemplate
	for _, item := range items {
		if item.Pipeline == "URP" {
			chosen = item
			break
		}
	}
	if os.Getenv("PROJECT_TEMPLATE_3D_LIVE") == "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		remote, err := fetchProjectTemplates(ctx, e)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range remote {
			if strings.HasSuffix(item.ID, ".urp-blank") {
				chosen = item
				break
			}
		}
		chosen.Archive = templateCachePath(e.Kind, chosen)
		ctx, cancel = context.WithTimeout(context.Background(), 3*time.Minute)
		err = downloadFileVerified(ctx, chosen.URL, chosen.Archive, chosen.Integrity, func(int64, int64) {})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("official URP 3D template downloaded and verified: %s", chosen.key())
	}
	if chosen.ID == "" {
		t.Fatal("no bundled URP template")
	}
	base := t.TempDir()
	dir := filepath.Join(base, "URPProject")
	log := filepath.Join(base, "Editor.log")
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	err := startProjectCreationFromTemplate(dir, e, chosen, func(exe string, args []string, _ string) error {
		args = append(args, "-batchmode", "-quit", "-buildTarget", "Win64", "-logFile", log)
		return exec.CommandContext(ctx, exe, args...).Run()
	})
	if err != nil {
		data, _ := os.ReadFile(log)
		tail := string(data)
		if len(tail) > 4500 {
			tail = tail[len(tail)-4500:]
		}
		t.Fatalf("Editor create: %v\n%s", err, tail)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "Packages", "manifest.json"))
	if err != nil || !strings.Contains(string(manifest), "com.unity.render-pipelines.universal") {
		t.Fatalf("URP dependency missing: %v", err)
	}
	graphics, err := os.ReadFile(filepath.Join(dir, "ProjectSettings", "GraphicsSettings.asset"))
	if err != nil || !strings.Contains(string(graphics), "m_CustomRenderPipeline:") || strings.Contains(string(graphics), "m_CustomRenderPipeline: {fileID: 0}") {
		t.Fatalf("URP render pipeline not assigned: %v", err)
	}
	t.Log("Actual Editor created and imported the isolated URP template; manifest and assigned pipeline verified")
}
