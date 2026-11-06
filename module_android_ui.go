package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"unitystarter/locales"
)

func (a *app) prepareAndroidSDK(e editorInstall, m editorModule) {
	if a.win == nil || a.moduleLoading || a.moduleBusy || !androidSDKCandidate(e, m) {
		return
	}
	a.moduleLoading = true
	a.moduleStatus = locales.AndroidPrepare()
	a.moduleCurrentName = m.Name
	win := a.win
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		parent, err := androidPlayerDir(e.Root)
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
			if err == nil {
				plan.RecoveryPending, err = androidJournalPending(filepath.Join(downloadsDir(), "android"), e.Root, plan)
			}
			if err == nil && !plan.RecoveryPending {
				_, err = emptyAndroidDestination(parent, "SDK")
			}
			if err == nil && !plan.RecoveryPending {
				_, err = emptyAndroidDestination(parent, "NDK")
			}
			if err == nil {
				plan.CachedBytes = androidCachedBytes(filepath.Join(downloadsDir(), "android"), e.Root, plan)
			}
		}
		win.Update(func() {
			a.moduleLoading = false
			if !a.manageModules || a.manageEditor.Exe != e.Exe {
				return
			}
			if err != nil {
				a.moduleStatus = locales.OpenFailed(err.Error())
				a.finishModuleQueueItem(m.ID, false)
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
	a.moduleCurrentName, a.moduleProgressDone, a.moduleProgressTotal = plan.Module.Name, 0, 0
	a.moduleJobSequence++
	sequence := a.moduleJobSequence
	win := a.win
	go func() {
		defer cancel()
		cache, err := openAndroidCache(filepath.Join(downloadsDir(), "android"), e.Root, plan)
		if cache != nil {
			defer cache.close()
		}
		var archives []string
		downloadFailed := false
		last := time.Now()
		recovered := false
		var releaseEditor func()
		defer func() {
			if releaseEditor != nil {
				releaseEditor()
			}
		}()
		lockEditor := func() error {
			if releaseEditor != nil {
				return nil
			}
			var lockErr error
			releaseEditor, lockErr = acquireEditorLock(e.Exe)
			return lockErr
		}
		if err == nil {
			_, pending, journalErr := readAndroidJournal(cache)
			if journalErr != nil {
				err = journalErr
			} else if pending {
				if err = lockEditor(); err == nil {
					ready := make(chan bool, 1)
					win.Update(func() {
						if ctx.Err() != nil {
							ready <- false
						} else {
							a.moduleInstalling = true
							a.moduleStatus = locales.AndroidRecovering()
							ready <- true
						}
					})
					if !<-ready {
						err = context.Canceled
					} else {
						recovered, err = recoverAndroidJournal(cache, e.Root)
					}
				}
			}
		}
		if err == nil && !recovered {
			archives, err = downloadAndroidPackages(ctx, plan, cache.dir, func(done, total int64) {
				if time.Since(last) < 200*time.Millisecond {
					return
				}
				last = time.Now()
				win.Update(func() {
					if a.moduleBusy && !a.moduleInstalling && total > 0 {
						if a.moduleJobSequence != sequence {
							return
						}
						a.moduleProgressDone, a.moduleProgressTotal = done, total
						a.moduleStatus = fmt.Sprintf("%s %d%%", locales.InstallModuleProgress(), done*100/total)
					}
				})
			})
			if err != nil && !errors.Is(err, context.Canceled) {
				downloadFailed = true
				err = fmt.Errorf("Android SDK download failed")
			}
		}
		if err == nil && !recovered {
			err = lockEditor()
		}
		if err == nil && !recovered {
			ready := make(chan bool, 1)
			win.Update(func() {
				if ctx.Err() != nil {
					ready <- false
				} else {
					a.moduleInstalling = true
					a.moduleStatus = locales.DownloadsInstalling()
					ready <- true
				}
			})
			if !<-ready {
				err = context.Canceled
			} else {
				err = installAndroidArchivesJournal(e.Root, plan, archives, cache, nil)
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
		var journalErr error
		if err == nil && physicalDone {
			journalErr = finalizeAndroidJournal(cache, e.Root)
		}
		var cacheErr error
		var retained int64
		retained, cacheErr = finishAndroidCache(cache, e.Root, plan, physicalDone && err == nil && journalErr == nil, ctx.Err())
		if releaseEditor != nil {
			releaseEditor()
			releaseEditor = nil
		}
		win.Update(func() {
			a.moduleBusy, a.moduleInstalling, a.moduleCancel = false, false, nil
			a.moduleProgressDone, a.moduleProgressTotal = 0, 0
			if errors.Is(err, context.Canceled) {
				a.moduleStatus = ""
				if cacheErr != nil {
					a.moduleStatus = locales.AndroidCacheCleanupFailed() + ": " + cacheErr.Error()
				}
				a.finishModuleQueueItem(plan.Module.ID, false)
				return
			}
			if err != nil {
				if physicalDone {
					a.moduleStatus = locales.InstallModuleRecordFailed() + ": " + err.Error()
				} else {
					a.moduleStatus = locales.OpenFailed(err.Error())
				}
				if downloadFailed && retained > 0 {
					a.moduleStatus = locales.AndroidDownloadRetained(humanSize(retained))
				}
				if cacheErr != nil {
					a.moduleStatus += " · " + locales.AndroidCacheCleanupFailed() + ": " + cacheErr.Error()
				}
				a.finishModuleQueueItem(plan.Module.ID, false)
				return
			}
			a.moduleStatus = locales.InstallModuleDone()
			if localOnly {
				a.moduleStatus = locales.InstallModuleLocalOnly()
			}
			if journalErr != nil {
				a.moduleStatus += " · " + locales.AndroidRecoveryCleanupFailed() + ": " + journalErr.Error()
			}
			if cacheErr != nil {
				a.moduleStatus += " · " + locales.AndroidCacheCleanupFailed() + ": " + cacheErr.Error()
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
			a.finishModuleQueueItem(plan.Module.ID, true)
		})
	}()
}

func (a *app) discardAndroidCache(e editorInstall, plan *androidInstallPlan) {
	if a.win == nil || a.moduleLoading || a.moduleBusy || plan == nil {
		return
	}
	a.moduleLoading = true
	a.moduleStatus = locales.AndroidCacheClearing()
	win := a.win
	go func() {
		cache, err := openAndroidCache(filepath.Join(downloadsDir(), "android"), e.Root, *plan)
		if err == nil {
			err = cache.clear()
			cache.close()
		}
		win.Update(func() {
			a.moduleLoading = false
			if a.androidPlan != plan || a.manageEditor.Exe != e.Exe {
				return
			}
			if err != nil {
				a.moduleStatus = locales.AndroidCacheCleanupFailed() + ": " + err.Error()
				return
			}
			plan.CachedBytes = 0
			a.moduleStatus = ""
		})
	}()
}
