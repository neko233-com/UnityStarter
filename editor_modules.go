package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

type editorModule struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	URL                   string `json:"downloadUrl"`
	Checksum              string `json:"checksum"`
	Integrity             string `json:"integrity,omitempty"`
	Destination           string `json:"destination"`
	Command               string `json:"cmd"`
	Parent                string `json:"parent"`
	Sync                  string `json:"sync,omitempty"`
	Required              bool   `json:"required,omitempty"`
	RenameFrom            string `json:"renameFrom,omitempty"`
	RenameTo              string `json:"renameTo,omitempty"`
	EULA                  string `json:"eulaUrl1"`
	EULA2                 string `json:"eulaUrl2"`
	Visible               bool   `json:"visible"`
	Selected              bool   `json:"selected"`
	PresenceUnverified    bool   `json:"-"`
	DownloadSize          int64  `json:"downloadSize"`
	DependencyFingerprint string `json:"-"`
}

func readEditorModules(root string) []editorModule {
	var modules []editorModule
	data, err := os.ReadFile(filepath.Join(root, "modules.json"))
	if err != nil || json.Unmarshal(data, &modules) != nil {
		return nil
	}
	annotateSDKDependencies(modules)
	state := readModuleState(root)
	for i := range modules {
		if moduleFilesVerifiable(modules[i]) {
			modules[i].Selected = (modules[i].Selected || moduleStateMatches(state, modules[i])) && moduleFilesPresent(root, modules[i])
		} else if moduleStateMatches(state, modules[i]) && moduleFilesPresent(root, modules[i]) {
			modules[i].Selected = true
		}
		modules[i].PresenceUnverified = modules[i].Selected && !moduleFilesVerifiable(modules[i])
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

// The Hub metadata also describes dependent components, external tools and
// licenses. Only explicitly validated installers and OpenJDK archives are handled.
var moduleIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+\-]*$`)

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
	case "linux-il2cpp":
		return filepath.Join("LinuxStandaloneSupport", "Variations", "linux64_player_development_il2cpp")
	case "linux-mono":
		return filepath.Join("LinuxStandaloneSupport", "Variations", "linux64_player_development_mono")
	case "linux-server":
		return filepath.Join("LinuxStandaloneSupport", "Variations", "linux64_server_development_mono")
	case "windows-server":
		return filepath.Join("WindowsStandaloneSupport", "Variations", "win64_server_development_mono")
	case "mac-coreclr":
		return filepath.Join("MacStandaloneSupport", "Variations", "macos_arm64_player_development_coreclr")
	case "mac-server-coreclr":
		return filepath.Join("MacStandaloneSupport", "Variations", "macos_arm64_server_development_coreclr")
	case "mac-mono":
		return filepath.Join("MacStandaloneSupport", "Variations", "macos_arm64_player_development_mono")
	case "mac-server":
		return filepath.Join("MacStandaloneSupport", "Variations", "macos_arm64_server_development_mono")
	case "appletv":
		return "AppleTVSupport"
	case "visionos":
		return "VisionOSPlayer"
	case "universal-windows-platform":
		return "MetroSupport"
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
	_, valid := standaloneModuleMD5(m)
	return valid && strings.HasSuffix(strings.ToLower(u.Path), ".exe") && m.Command == "" && m.Parent == "" && m.EULA == "" && m.EULA2 == "" && m.Destination == "{UNITY_PATH}" && moduleIDPattern.MatchString(m.ID) && moduleOutput(m) != ""
}

// Unity Hub's modules.json may have only integrity, while the Tuanjie Hub
// stores hexadecimal checksum. When both exist, they must agree.
func standaloneModuleMD5(m editorModule) ([]byte, bool) {
	var checksum []byte
	if m.Checksum != "" {
		var err error
		checksum, err = hex.DecodeString(m.Checksum)
		if err != nil || len(checksum) != 16 {
			return nil, false
		}
	}
	if m.Integrity != "" {
		if !strings.HasPrefix(m.Integrity, "md5-") {
			return nil, false
		}
		_, digest, err := integrityHasher(m.Integrity)
		if err != nil || len(digest) != 16 || (len(checksum) != 0 && !bytes.Equal(checksum, digest)) {
			return nil, false
		}
		checksum = digest
	}
	return checksum, len(checksum) == 16
}

func moduleInstallSpec(e editorInstall, m editorModule) (integrity, player, extension string, ok bool) {
	if _, kind, valid := editorLanguageModule(m); valid && e.Kind == kind {
		return "", languageModule, ".po", true
	}
	if standaloneModule(m) {
		digest, _ := standaloneModuleMD5(m)
		return "md5-" + base64.StdEncoding.EncodeToString(digest), "", ".exe", true
	}
	if sum, player, valid := openJDKArchive(m); valid && parentModuleSelected(e.Modules, m.Parent) {
		return sum, player, ".zip", true
	}
	if sum, version, valid := tuanjieDocumentationArchive(m); valid && e.Kind == kindTuanjie && version == e.Version {
		return sum, documentationModule, ".zip", true
	}
	if version, valid := unityDocumentationArchive(m); valid && e.Kind == kindUnity {
		parts := editorVersionPattern.FindStringSubmatch(e.Version)
		if len(parts) > 2 && version == parts[1]+"."+parts[2] {
			return "", unityDocumentationModule, ".zip", true
		}
	}
	return "", "", "", false
}

// Update only the selected field and retain all metadata the official Hub
// uses. Re-read after the installer exits so unrelated edits are not lost.
var errModuleMetadataChanged = errors.New("module metadata changed during installation")

func markModuleInstalled(root string, module editorModule) error {
	path := filepath.Join(root, "modules.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return markOwnedModuleInstalled(root, module)
	}
	if err != nil {
		return err
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(data, &rows); err != nil {
		return err
	}
	var currentModules []editorModule
	if err := json.Unmarshal(data, &currentModules); err != nil {
		return err
	}
	annotateSDKDependencies(currentModules)
	found := false
	for i, row := range rows {
		var key string
		_ = json.Unmarshal(row["id"], &key)
		if key == module.ID {
			var current editorModule
			encoded, err := json.Marshal(row)
			if err != nil || json.Unmarshal(encoded, &current) != nil || current.URL != module.URL || current.Checksum != module.Checksum || current.Integrity != module.Integrity || current.Destination != module.Destination || current.Parent != module.Parent || current.Sync != module.Sync || current.EULA != module.EULA || current.EULA2 != module.EULA2 || current.Command != module.Command || current.RenameFrom != module.RenameFrom || current.RenameTo != module.RenameTo || currentModules[i].DependencyFingerprint != module.DependencyFingerprint {
				return errModuleMetadataChanged
			}
			row["selected"] = json.RawMessage("true")
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("module no longer in metadata")
	}
	if legacyAndroidSDKMetadata(module) {
		for i, current := range currentModules {
			if current.Sync == module.ID {
				rows[i]["selected"] = json.RawMessage("true")
			}
		}
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, encoded)
}

func (a *app) installEditorModule(e editorInstall, m editorModule) {
	_, _, _, supported := moduleInstallSpec(e, m)
	if a.win == nil || a.moduleBusy || e.Simulated || m.Selected || !supported {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.moduleBusy, a.moduleInstalling, a.moduleCancel, a.moduleStatus = true, false, cancel, locales.InstallModuleProgress()
	win := a.win
	go func() {
		defer cancel()
		last := time.Now()
		progress := func(done, total int64) {
			if time.Since(last) < 200*time.Millisecond {
				return
			}
			last = time.Now()
			win.Update(func() {
				if total > 0 {
					a.moduleStatus = fmt.Sprintf("%s %d%%", locales.InstallModuleProgress(), done*100/total)
				}
			})
		}
		beforeInstall := func() error {
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
				return context.Canceled
			}
			return nil
		}
		result := executeEditorModule(ctx, e, m, progress, beforeInstall)
		win.Update(func() {
			a.finishEditorModuleInstall(e, m, result)
		})
	}()
}

type moduleInstallResult struct {
	err          error
	physicalDone bool
	localOnly    bool
}

// executeEditorModule performs the same verified download and install for an
// individual action or a queue item. The editor lock is released before the
// caller may start the next item.
func executeEditorModule(ctx context.Context, e editorInstall, m editorModule, progress func(int64, int64), beforeInstall func() error) moduleInstallResult {
	integrity, player, extension, supported := moduleInstallSpec(e, m)
	if e.Simulated || m.Selected || !supported {
		return moduleInstallResult{err: fmt.Errorf("unsupported module")}
	}
	path := filepath.Join(downloadsDir(), "module-"+e.Version+"-"+m.ID+extension)
	expectedSize := int64(0)
	if player == documentationModule {
		expectedSize = m.DownloadSize
	}
	var language []byte
	var err error
	if player == unityDocumentationModule {
		integrity, expectedSize, err = unityDocumentationRemote(ctx, m)
		if err == nil {
			err = downloadFileVerifiedSize(ctx, m.URL, path, integrity, expectedSize, progress)
		}
	} else if player == languageModule {
		language, err = downloadEditorLanguage(ctx, m, progress)
	} else {
		err = downloadFileVerifiedSize(ctx, m.URL, path, integrity, expectedSize, progress)
	}
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			err = fmt.Errorf("module download failed")
		}
		return moduleInstallResult{err: err}
	}
	releaseEditor, err := acquireEditorLock(e.Exe)
	if err != nil {
		return moduleInstallResult{err: err}
	}
	defer releaseEditor()
	if err = beforeInstall(); err != nil {
		return moduleInstallResult{err: err}
	}
	if player == languageModule {
		err = installEditorLanguage(e.Root, m, language)
	} else if player == documentationModule {
		err = installTuanjieDocumentation(e.Root, path, integrity)
	} else if player == unityDocumentationModule {
		err = installDocumentation(e.Root, path, integrity, "en")
	} else if player != "" {
		err = installOpenJDK(e.Root, path, player, integrity)
	} else {
		err = runEditorInstaller(path, e.Kind, e.Root)
	}
	if err == nil && player == "" && !moduleFilesPresent(e.Root, m) {
		err = fmt.Errorf("module installation completed but expected files were not found")
	}
	result := moduleInstallResult{err: err, physicalDone: err == nil}
	if result.err == nil {
		result.err = markModuleInstalled(e.Root, m)
		if result.err != nil && !errors.Is(result.err, errModuleMetadataChanged) {
			if localErr := markOwnedModuleInstalled(e.Root, m); localErr == nil {
				result.localOnly, result.err = true, nil
			}
		}
	}
	return result
}

func (a *app) finishEditorModuleInstall(e editorInstall, m editorModule, result moduleInstallResult) {
	a.moduleBusy, a.moduleInstalling, a.moduleCancel = false, false, nil
	if errors.Is(result.err, context.Canceled) {
		a.moduleStatus = ""
		a.finishModuleQueueItem(m.ID, false)
		return
	}
	if result.err != nil {
		if result.physicalDone {
			a.moduleStatus = locales.InstallModuleRecordFailed() + ": " + result.err.Error()
		} else {
			a.moduleStatus = locales.OpenFailed(result.err.Error())
		}
		a.finishModuleQueueItem(m.ID, false)
		return
	}
	a.moduleStatus = locales.InstallModuleDone()
	if result.localOnly {
		a.moduleStatus = locales.InstallModuleLocalOnly()
	}
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
	a.finishModuleQueueItem(m.ID, true)
}

func editorUninstaller(e editorInstall) string {
	path := filepath.Join(e.Root, "Editor", "Uninstall.exe")
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
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
				if a.uninstallStatus != "" {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						if a.uninstallBusy {
							ui.Spinner(c)
						}
						ui.Text(c, a.uninstallStatus).FontSize(12).TextColor(t.TextMuted).Grow(1)
					})
				}
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if a.uninstallBusy {
						ui.Button(c, locales.InstallUninstallHide()).OnClick(func() { a.manageOpen = false })
						return
					}
					ui.Button(c, locales.DialogOK()).OnClick(func() {
						a.manageOpen, a.manageUninstall = false, false
						a.uninstallStatus = ""
					})
					ui.Button(c, locales.InstallUninstallOfficial()).Disabled(a.editorWorkActive()).OnClick(func() {
						if p := editorUninstaller(e); p != "" {
							if err := mygo.Shell.OpenPath(p); err != nil {
								a.alert(locales.OpenFailed(err.Error()))
								return
							}
							a.manageOpen = false
						}
					})
					button := ui.PrimaryButton(c, locales.InstallUninstall()).Disabled(a.editorWorkActive() || editorUninstaller(e) == "").OnClick(func() { a.startEditorUninstall(e) })
					if a.editorWorkActive() {
						button.Tooltip(locales.InstallUninstallWaitWork())
					}
				})
				return
			}
			if a.manageModules {
				if a.androidPlan != nil {
					plan := *a.androidPlan
					var total int64
					for _, pkg := range plan.Packages {
						total += pkg.ExpectedSize
					}
					ui.Text(c, locales.AndroidLicenseTitle()).FontSize(15).Bold()
					if plan.RecoveryPending {
						ui.Text(c, locales.AndroidRecoveryLicenseHint()).FontSize(12).TextColor(t.TextMuted)
					} else {
						ui.Text(c, locales.AndroidLicenseHint(len(plan.Packages), humanSize(total))).FontSize(12).TextColor(t.TextMuted)
					}
					if plan.LegacyTuanjie {
						ui.Text(c, locales.AndroidLegacyBaseline()).FontSize(12).TextColor(t.TextMuted)
					}
					if plan.CachedBytes > 0 {
						ui.Text(c, locales.AndroidCachedBytes(humanSize(plan.CachedBytes))).FontSize(12).TextColor(t.TextMuted)
					}
					if plan.RecoveryPending {
						ui.Text(c, locales.AndroidRecoveryPending()).FontSize(12).TextColor(t.TextMuted)
					}
					if a.moduleStatus != "" {
						ui.Text(c, a.moduleStatus).FontSize(12).TextColor(t.TextMuted)
					}
					ui.Scroll(c).Height(240).Children(func() { ui.Text(c, plan.License).Width(510).FontSize(12) })
					ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
						if plan.CachedBytes > 0 && !plan.RecoveryPending {
							ui.Button(c, locales.AndroidDiscardCache()).Disabled(a.moduleLoading).OnClick(func() { a.discardAndroidCache(e, a.androidPlan) })
						}
						ui.Button(c, locales.ActionCancel()).Disabled(a.moduleLoading).OnClick(func() {
							a.androidPlan = nil
							queued := len(a.moduleQueue) > 0
							a.moduleStatus = ""
							a.finishModuleQueueItem(plan.Module.ID, false)
							if !queued {
								a.moduleStatus = ""
							}
						})
						label := locales.AndroidLicenseAccept()
						if plan.RecoveryPending {
							label = locales.AndroidLicenseRecover()
						}
						ui.PrimaryButton(c, label).Disabled(a.moduleLoading).OnClick(func() { a.androidPlan = nil; a.installAndroidSDK(e, plan) })
					})
					return
				}
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, locales.InstallModuleHint()).Grow(1).FontSize(12).TextColor(t.TextMuted)
					ui.Button(c, locales.InstallModuleRefresh()).Disabled(a.moduleLoading || a.moduleBusy).OnClick(func() { a.loadEditorModules(e) })
				})
				if a.moduleLoading {
					ui.Text(c, locales.InstallModuleLoading()).FontSize(12).TextColor(t.TextMuted)
				}
				if a.moduleStatus != "" {
					ui.Text(c, a.moduleStatus).FontSize(12).TextColor(t.TextMuted)
				}
				if a.moduleSelectionRestored && len(a.moduleSelection) > 0 {
					ui.Text(c, locales.ModuleSelectionRestored()).FontSize(12).TextColor(t.TextMuted)
				}
				if len(a.moduleQueue) > 0 {
					ui.Text(c, locales.ModuleBatchProgress(a.moduleQueueTotal-len(a.moduleQueue)+1, a.moduleQueueTotal)).FontSize(12).TextColor(t.TextMuted)
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
								if m.Selected {
									ui.Text(c, m.Name).Grow(1).FontSize(12).Clip()
									label := locales.DownloadsInstalled()
									if m.PresenceUnverified {
										label = locales.InstallModuleReported()
									}
									ui.Badge(c, label)
								} else {
									checked := a.moduleSelection[m.ID]
									canSelect := moduleSelectable(e, m, a.moduleSelection)
									checkbox := ui.Checkbox(c, &checked, m.Name).Grow(1).Disabled(a.moduleBusy || a.moduleLoading || len(a.moduleQueue) > 0 || (!canSelect && !checked)).OnClick(func() { a.setModuleSelection(e, m.ID, checked) })
									if !canSelect && !checked {
										checkbox.Tooltip(locales.InstallModuleUnavailable())
									}
									_, _, _, supported := moduleInstallSpec(e, m)
									android := androidSDKCandidate(e, m)
									button := ui.Button(c, locales.ActionInstall()).Disabled(a.moduleBusy || a.moduleLoading || len(a.moduleQueue) > 0 || (!supported && !android)).OnClick(func() {
										if android {
											a.prepareAndroidSDK(e, m)
										} else {
											a.installEditorModule(e, m)
										}
									})
									if !supported && !android {
										_, _, archive := openJDKArchive(m)
										if (archive || (e.Kind == kindUnity && androidSDKMetadata(m)) || (e.Kind == kindTuanjie && legacyAndroidSDKMetadata(m))) && !parentModuleSelected(e.Modules, m.Parent) {
											button.Tooltip(locales.InstallModuleNeedsParent())
										} else {
											button.Tooltip(locales.InstallModuleUnavailable())
										}
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
						ui.Button(c, locales.DialogOK()).Disabled(a.moduleLoading || len(a.moduleQueue) > 0).OnClick(func() {
							a.manageOpen, a.manageModules = false, false
							a.moduleStatus = ""
						})
						ui.PrimaryButton(c, locales.ModuleBatchInstall(len(a.moduleSelection))).Disabled(a.uninstallBusy || a.moduleLoading || len(a.moduleSelection) == 0 || len(a.moduleQueue) > 0).OnClick(func() { a.startModuleQueue(e) })
					}
				})
				return
			}
			ui.Button(c, locales.InstallModules()).OnClick(func() {
				a.manageModules = true
				if len(e.Modules) == 0 || e.ModulesCached {
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
