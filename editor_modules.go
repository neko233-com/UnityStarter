package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

type editorModule struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	URL          string `json:"downloadUrl"`
	Checksum     string `json:"checksum"`
	Destination  string `json:"destination"`
	Command      string `json:"cmd"`
	Parent       string `json:"parent"`
	EULA         string `json:"eulaUrl1"`
	EULA2        string `json:"eulaUrl2"`
	Visible      bool   `json:"visible"`
	Selected     bool   `json:"selected"`
	DownloadSize int64  `json:"downloadSize"`
}

func readEditorModules(root string) []editorModule {
	var modules []editorModule
	data, err := os.ReadFile(filepath.Join(root, "modules.json"))
	if err != nil || json.Unmarshal(data, &modules) != nil {
		return nil
	}
	return modules
}

func installedModuleNames(modules []editorModule) []string {
	var names []string
	for _, m := range modules {
		if m.Selected && m.Name != "" {
			names = append(names, m.Name)
		}
	}
	return names
}

// The Hub metadata also describes ZIPs, dependent components, external tools
// and licenses. Only standalone, checksummed official NSIS modules can be
// safely handled by this installer.
var moduleIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func moduleOutput(m editorModule) string {
	switch m.ID {
	case "android":
		return "AndroidPlayer"
	case "ios":
		return "iOSSupport"
	case "webgl":
		return "WebGLSupport"
	case "windows-il2cpp":
		return filepath.Join("windowsstandalonesupport", "Variations", "win64_player_development_il2cpp")
	case "weixinminigame":
		return "WeixinMiniGameSupport"
	case "playableads":
		return "PlayableAdsSupport"
	default:
		return ""
	}
}

func standaloneModule(m editorModule) bool {
	u, err := url.Parse(m.URL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Host != u.Hostname() {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host != "download.unity3d.com" && host != "download.tuanjie.cn" {
		return false
	}
	digest, err := hex.DecodeString(m.Checksum)
	return err == nil && len(digest) == 16 && strings.HasSuffix(strings.ToLower(u.Path), ".exe") && m.Command == "" && m.Parent == "" && m.EULA == "" && m.EULA2 == "" && m.Destination == "{UNITY_PATH}" && moduleIDPattern.MatchString(m.ID) && moduleOutput(m) != ""
}

// Update only the selected field and retain all metadata the official Hub
// uses. Re-read after the installer exits so unrelated edits are not lost.
func markModuleInstalled(root string, module editorModule) error {
	path := filepath.Join(root, "modules.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return markOwnedModuleInstalled(root, module.ID)
	}
	if err != nil {
		return err
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(data, &rows); err != nil {
		return err
	}
	found := false
	for _, row := range rows {
		var key string
		_ = json.Unmarshal(row["id"], &key)
		if key == module.ID {
			var current editorModule
			encoded, err := json.Marshal(row)
			if err != nil || json.Unmarshal(encoded, &current) != nil || current.URL != module.URL || current.Checksum != module.Checksum {
				return fmt.Errorf("module metadata changed during installation")
			}
			row["selected"] = json.RawMessage("true")
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("module no longer in metadata")
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, encoded)
}

func (a *app) installEditorModule(e editorInstall, m editorModule) {
	if a.win == nil || a.moduleBusy || e.Simulated || m.Selected || !standaloneModule(m) {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.moduleBusy, a.moduleInstalling, a.moduleCancel, a.moduleStatus = true, false, cancel, locales.InstallModuleProgress()
	win := a.win
	go func() {
		defer cancel()
		digest, _ := hex.DecodeString(m.Checksum)
		path := filepath.Join(downloadsDir(), "module-"+e.Version+"-"+m.ID+".exe")
		last := time.Now()
		err := downloadFileVerified(ctx, m.URL, path, "md5-"+base64.StdEncoding.EncodeToString(digest), func(done, total int64) {
			if time.Since(last) < 200*time.Millisecond {
				return
			}
			last = time.Now()
			win.Update(func() {
				if total > 0 {
					a.moduleStatus = fmt.Sprintf("%s %d%%", locales.InstallModuleProgress(), done*100/total)
				}
			})
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			err = fmt.Errorf("module download failed")
		}
		if err == nil {
			ready := make(chan bool, 1)
			win.Update(func() {
				if ctx.Err() != nil {
					ready <- false
				} else {
					a.moduleInstalling = true
					ready <- true
				}
			})
			if !<-ready {
				err = context.Canceled
			} else {
				cmd := exec.Command(path, installerArgs(e.Kind, e.Root)...)
				configureInstallerCmd(cmd, e.Kind, e.Root)
				err = cmd.Run()
			}
		}
		if err == nil {
			output := filepath.Join(e.Root, "Editor", "Data", "PlaybackEngines", moduleOutput(m))
			if info, statErr := os.Stat(output); statErr != nil || !info.IsDir() {
				err = fmt.Errorf("module installation completed but expected files were not found")
			}
		}
		if err == nil {
			err = markModuleInstalled(e.Root, m)
		}
		win.Update(func() {
			a.moduleBusy = false
			a.moduleInstalling = false
			a.moduleCancel = nil
			if errors.Is(err, context.Canceled) {
				a.moduleStatus = ""
				return
			}
			if err != nil {
				a.moduleStatus = locales.OpenFailed(err.Error())
				return
			}
			a.moduleStatus = locales.InstallModuleDone()
			modules := readEditorModules(e.Root)
			if len(modules) == 0 {
				modules = append([]editorModule(nil), e.Modules...)
				for i := range modules {
					if modules[i].ID == m.ID {
						modules[i].Selected = true
					}
				}
			}
			a.manageEditor.Modules = modules
			a.refresh()
		})
	}()
}

func editorUninstaller(e editorInstall) string {
	path := filepath.Join(e.Root, "Editor", "Uninstall.exe")
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path
	}
	return ""
}

func (a *app) editorManageDialog(c *ui.Context) {
	if !a.manageOpen {
		return
	}
	t := c.Theme()
	e := a.manageEditor
	ui.Modal(c, &a.manageOpen, func() {
		ui.Column(c).Width(560).Gap(12).Children(func() {
			ui.Text(c, e.Version+" · "+channelName(e.Kind)).FontSize(18).Bold()
			ui.Text(c, e.Exe).FontSize(12).TextColor(t.TextMuted).Clip()
			if a.manageUninstall {
				ui.Text(c, locales.InstallUninstallWarning()).FontSize(12).TextColor(t.TextMuted)
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					ui.Button(c, locales.ActionCancel()).OnClick(func() { a.manageUninstall = false })
					ui.PrimaryButton(c, locales.InstallUninstall()).OnClick(func() {
						if p := editorUninstaller(e); p != "" {
							if err := mygo.Shell.OpenPath(p); err != nil {
								a.alert(locales.OpenFailed(err.Error()))
								return
							}
							a.manageOpen = false
						}
					})
				})
				return
			}
			if a.manageModules {
				ui.Text(c, locales.InstallModuleHint()).FontSize(12).TextColor(t.TextMuted)
				if a.moduleLoading {
					ui.Text(c, locales.InstallModuleLoading()).FontSize(12).TextColor(t.TextMuted)
				}
				if a.moduleStatus != "" {
					ui.Text(c, a.moduleStatus).FontSize(12).TextColor(t.TextMuted)
				}
				ui.Scroll(c).Height(300).Children(func() {
					ui.Column(c).Gap(5).Children(func() {
						for _, module := range e.Modules {
							m := module
							if !m.Visible {
								continue
							}
							c.Key(m.ID)
							ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
								ui.Text(c, m.Name).Grow(1).FontSize(12).Clip()
								if m.Selected {
									ui.Badge(c, locales.DownloadsInstalled())
								} else {
									button := ui.Button(c, locales.ActionInstall()).Disabled(a.moduleBusy || !standaloneModule(m)).OnClick(func() { a.installEditorModule(e, m) })
									if !standaloneModule(m) {
										button.Tooltip(locales.InstallModuleUnavailable())
									}
								}
							})
						}
					})
				})
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if a.moduleBusy && !a.moduleInstalling {
						ui.Button(c, locales.ActionCancel()).OnClick(func() {
							if a.moduleCancel != nil {
								a.moduleCancel()
							}
						})
					} else if !a.moduleBusy {
						ui.Button(c, locales.DialogOK()).OnClick(func() { a.manageOpen = false })
					}
				})
				return
			}
			ui.Button(c, locales.InstallModules()).OnClick(func() {
				a.manageModules = true
				if len(e.Modules) == 0 {
					a.loadEditorModules(e)
				}
			})
			ui.Button(c, locales.ActionFolder()).OnClick(func() { mygo.Shell.ShowItemInFolder(e.Exe) })
			uninstaller := editorUninstaller(e)
			button := ui.Button(c, locales.InstallUninstall()).Disabled(uninstaller == "").OnClick(func() { a.manageUninstall = true })
			if uninstaller == "" {
				button.Tooltip(locales.InstallUninstallMissing())
			}
			ui.Row(c).Justify(ui.End).Children(func() { ui.Button(c, locales.DialogOK()).OnClick(func() { a.manageOpen = false }) })
		})
	})
}
