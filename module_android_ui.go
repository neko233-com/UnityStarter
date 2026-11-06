package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"unitystarter/locales"
)

func (a *app) prepareAndroidSDK(e editorInstall, m editorModule) {
	if a.win == nil || a.moduleLoading || a.moduleBusy || !androidSDKCandidate(e, m) {
		return
	}
	a.moduleLoading = true
	a.moduleStatus = locales.AndroidPrepare()
	win := a.win
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		parent, err := androidPlayerDir(e.Root)
		if err == nil {
			_, err = emptyAndroidDestination(parent, "SDK")
		}
		if err == nil {
			_, err = emptyAndroidDestination(parent, "NDK")
		}
		if err == nil {
			var probe string
			probe, err = os.MkdirTemp(parent, ".unitystarter-permission-")
			if err == nil {
				err = os.Remove(probe)
			}
		}
		var plan androidInstallPlan
		if err == nil {
			plan, err = prepareAndroidPlan(ctx, e, m)
		}
		win.Update(func() {
			a.moduleLoading = false
			if !a.manageOpen || !a.manageModules || a.manageEditor.Exe != e.Exe {
				return
			}
			if err != nil {
				a.moduleStatus = locales.OpenFailed(err.Error())
				return
			}
			a.moduleStatus = ""
			a.androidPlan = &plan
		})
	}()
}

// Called only by the explicit license acceptance action in the native dialog.
func (a *app) installAndroidSDK(e editorInstall, plan androidInstallPlan) {
	if a.win == nil || a.moduleBusy || !androidSDKCandidate(e, plan.Module) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	a.moduleBusy, a.moduleInstalling, a.moduleCancel, a.moduleStatus = true, false, cancel, locales.InstallModuleProgress()
	win := a.win
	go func() {
		defer cancel()
		cache := downloadsDir()
		err := os.MkdirAll(cache, 0o755)
		var dir string
		if err == nil {
			dir, err = os.MkdirTemp(cache, ".unitystarter-android-download-")
		}
		if dir != "" {
			defer removeAndroidStage(cache, dir, ".unitystarter-android-download-")
		}
		var archives []string
		last := time.Now()
		if err == nil {
			archives, err = downloadAndroidPackages(ctx, plan, dir, func(done, total int64) {
				if time.Since(last) < 200*time.Millisecond {
					return
				}
				last = time.Now()
				win.Update(func() {
					if a.moduleBusy && !a.moduleInstalling && total > 0 {
						a.moduleStatus = fmt.Sprintf("%s %d%%", locales.InstallModuleProgress(), done*100/total)
					}
				})
			})
			if err != nil && !errors.Is(err, context.Canceled) {
				err = fmt.Errorf("Android SDK download failed")
			}
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
				err = installAndroidArchives(e.Root, plan, archives)
			}
		}
		physicalDone := err == nil
		localOnly := false
		if err == nil {
			err = markModuleInstalled(e.Root, plan.Module)
			if err != nil && !errors.Is(err, errModuleMetadataChanged) {
				if localErr := markOwnedModuleInstalled(e.Root, plan.Module); localErr == nil {
					localOnly = true
					err = nil
				}
			}
		}
		win.Update(func() {
			a.moduleBusy, a.moduleInstalling, a.moduleCancel = false, false, nil
			if errors.Is(err, context.Canceled) {
				a.moduleStatus = ""
				return
			}
			if err != nil {
				if physicalDone {
					a.moduleStatus = locales.InstallModuleRecordFailed() + ": " + err.Error()
				} else {
					a.moduleStatus = locales.OpenFailed(err.Error())
				}
				return
			}
			a.moduleStatus = locales.InstallModuleDone()
			if localOnly {
				a.moduleStatus = locales.InstallModuleLocalOnly()
			}
			modules := readEditorModules(e.Root)
			if len(modules) == 0 {
				modules = append([]editorModule(nil), e.Modules...)
				for i := range modules {
					if modules[i].ID == plan.Module.ID {
						modules[i].Selected = true
					}
				}
			}
			a.manageEditor.Modules = modules
			a.refresh()
		})
	}()
}
