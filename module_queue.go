package main

import (
	"fmt"
	"strings"

	"unitystarter/locales"
)

// Only these module formats actually require an installed parent. Other
// catalog parent labels may be presentation groups rather than dependencies.
func moduleRequiredParent(m editorModule) string {
	if _, _, ok := openJDKArchive(m); ok || androidSDKMetadata(m) || legacyAndroidSDKMetadata(m) {
		return m.Parent
	}
	return ""
}

func moduleSelectable(e editorInstall, m editorModule, selected map[string]bool) bool {
	if e.Simulated || !m.Visible || m.Selected {
		return false
	}
	planned := e
	planned.Modules = append([]editorModule(nil), e.Modules...)
	for i := range planned.Modules {
		if selected[planned.Modules[i].ID] {
			planned.Modules[i].Selected = true
		}
	}
	_, _, _, supported := moduleInstallSpec(planned, m)
	return supported || androidSDKCandidate(planned, m)
}

func planModuleQueue(e editorInstall, selected map[string]bool) ([]string, error) {
	byID := make(map[string]editorModule, len(e.Modules))
	for _, m := range e.Modules {
		if _, exists := byID[m.ID]; exists && selected[m.ID] {
			return nil, fmt.Errorf("ambiguous module ID")
		}
		byID[m.ID] = m
	}
	var queue []string
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if done[id] {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("cyclic module dependency")
		}
		m, ok := byID[id]
		if !ok || !selected[id] || !moduleSelectable(e, m, selected) {
			return fmt.Errorf("module or dependency is unavailable")
		}
		visiting[id] = true
		if parent := moduleRequiredParent(m); parent != "" {
			p, exists := byID[parent]
			if !exists {
				return fmt.Errorf("module dependency is missing")
			}
			if !p.Selected {
				if !selected[parent] {
					return fmt.Errorf("module dependency is not selected")
				}
				if err := visit(parent); err != nil {
					return err
				}
			}
		}
		delete(visiting, id)
		done[id] = true
		queue = append(queue, id)
		return nil
	}
	// Preserve catalog order for independent modules; visit dependencies first.
	for _, m := range e.Modules {
		if selected[m.ID] {
			if err := visit(m.ID); err != nil {
				return nil, err
			}
		}
	}
	for id, on := range selected {
		if on && !done[id] {
			return nil, fmt.Errorf("selected module is missing")
		}
	}
	return queue, nil
}

func (a *app) setModuleSelection(e editorInstall, id string, on bool) {
	if a.moduleSelection == nil {
		a.moduleSelection = map[string]bool{}
	}
	if on {
		a.moduleSelection[id] = true
		if err := a.persistModuleSelection(e); err != nil {
			a.moduleStatus = locales.ModuleSelectionSaveFailed()
		} else if a.moduleStatus == locales.ModuleSelectionSaveFailed() {
			a.moduleStatus = ""
		}
		return
	}
	delete(a.moduleSelection, id)
	// A selected child cannot remain queued after its required parent is removed.
	for changed := true; changed; {
		changed = false
		for _, m := range e.Modules {
			if a.moduleSelection[m.ID] && !moduleSelectable(e, m, a.moduleSelection) {
				delete(a.moduleSelection, m.ID)
				changed = true
			}
		}
	}
	if err := a.persistModuleSelection(e); err != nil {
		a.moduleStatus = locales.ModuleSelectionSaveFailed()
	} else if a.moduleStatus == locales.ModuleSelectionSaveFailed() {
		a.moduleStatus = ""
	}
}

func (a *app) reconcileModuleSelection(e editorInstall) (bool, error) {
	remaining := map[string]bool{}
	for _, m := range e.Modules {
		if a.moduleSelection[m.ID] && !m.Selected {
			remaining[m.ID] = true
		}
	}
	changed := len(remaining) != len(a.moduleSelection)
	a.moduleSelection = remaining
	for again := true; again; {
		again = false
		for _, m := range e.Modules {
			if remaining[m.ID] && !moduleSelectable(e, m, remaining) {
				delete(remaining, m.ID)
				again, changed = true, true
			}
		}
	}
	if changed {
		return true, a.persistModuleSelection(e)
	}
	return false, nil
}

func (a *app) startModuleQueue(e editorInstall) {
	if (a.win == nil && a.moduleQueueRunner == nil) || a.uninstallBusy || a.moduleBusy || a.moduleLoading || len(a.moduleQueue) > 0 || a.androidPlan != nil {
		return
	}
	queue, err := planModuleQueue(e, a.moduleSelection)
	if err != nil || len(queue) == 0 {
		a.moduleStatus = locales.ModuleBatchUnavailable()
		return
	}
	if err := a.persistModuleSelection(e); err != nil {
		a.moduleStatus = locales.ModuleSelectionSaveFailed()
		return
	}
	a.moduleSelectionRestored = false
	a.moduleQueue, a.moduleQueueTotal = queue, len(queue)
	a.moduleQueueWarnings = nil
	a.advanceModuleQueue()
}

func (a *app) advanceModuleQueue() {
	if len(a.moduleQueue) == 0 {
		return
	}
	e := a.manageEditor
	for _, m := range e.Modules {
		if m.ID != a.moduleQueue[0] {
			continue
		}
		if m.Selected {
			a.finishModuleQueueItem(m.ID, true)
			return
		}
		_, _, _, supported := moduleInstallSpec(e, m)
		android := androidSDKCandidate(e, m)
		if !supported && !android {
			a.moduleStatus = locales.ModuleBatchUnavailable()
			a.finishModuleQueueItem(m.ID, false)
			return
		}
		if a.moduleQueueRunner != nil {
			a.moduleQueueRunner(e, m, android)
		} else if android {
			a.prepareAndroidSDK(e, m)
		} else {
			a.installEditorModule(e, m)
		}
		return
	}
	a.moduleStatus = locales.ModuleBatchUnavailable()
	a.finishModuleQueueItem(a.moduleQueue[0], false)
}

func (a *app) finishModuleQueueItem(id string, success bool) {
	if success {
		delete(a.moduleSelection, id)
		if err := a.persistModuleSelection(a.manageEditor); err != nil {
			a.moduleStatus = locales.ModuleSelectionSaveFailed()
			a.moduleQueue, a.moduleQueueTotal = nil, 0
			a.moduleQueueWarnings = nil
			return
		}
	}
	if len(a.moduleQueue) == 0 || a.moduleQueue[0] != id {
		return
	}
	if success && a.moduleStatus != "" && a.moduleStatus != locales.InstallModuleDone() {
		a.moduleQueueWarnings = append(a.moduleQueueWarnings, a.moduleStatus)
	}
	if !success {
		if a.moduleStatus == "" {
			a.moduleStatus = locales.ModuleBatchStopped()
		}
		if len(a.moduleQueueWarnings) > 0 {
			a.moduleStatus += " · " + strings.Join(a.moduleQueueWarnings, " · ")
		}
		a.moduleQueue, a.moduleQueueTotal = nil, 0
		a.moduleQueueWarnings = nil
		return
	}
	a.moduleQueue = a.moduleQueue[1:]
	if len(a.moduleQueue) == 0 {
		a.moduleQueueTotal = 0
		if len(a.moduleQueueWarnings) > 0 {
			a.moduleStatus = locales.ModuleBatchDone() + " · " + strings.Join(a.moduleQueueWarnings, " · ")
		} else {
			a.moduleStatus = locales.ModuleBatchDone()
		}
		a.moduleQueueWarnings = nil
		return
	}
	a.advanceModuleQueue()
}
