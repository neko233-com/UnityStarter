package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

// The Hub's Visual Studio module represents an IDE plus its required Unity
// workloads, not merely an installed devenv.exe.
var visualStudioComponents = []string{
	"Microsoft.VisualStudio.Workload.ManagedGame",
	"Microsoft.VisualStudio.Workload.NativeDesktop",
	"Microsoft.VisualStudio.Component.VC.Tools.x86.x64",
	"Microsoft.VisualStudio.Component.Windows10SDK.16299",
}

type visualStudioInstance struct {
	InstanceID          string `json:"instanceId"`
	ProductID           string `json:"productId"`
	InstallationVersion string `json:"installationVersion"`
	InstallationPath    string `json:"installationPath"`
	IsComplete          bool   `json:"isComplete"`
	IsLaunchable        bool   `json:"isLaunchable"`
}

type visualStudioTask struct {
	Editor editorInstall
	Module editorModule
	Status string
	Cancel context.CancelFunc
}

func (a *app) visualStudioWatching() bool {
	return a.visualStudioTask != nil && a.visualStudioTask.Status == "waiting"
}

func checkVisualStudioModules(ctx context.Context, modules []editorModule) {
	for _, m := range modules {
		if visualStudioModuleMajor(m) != "" {
			all, ready, err := queryVisualStudioInstances(ctx)
			if err == nil {
				applyVisualStudioState(modules, all, ready)
			}
			return
		}
	}
}

func visualStudioModuleMajor(m editorModule) string {
	if m.ID != "visualstudio" || !strings.HasPrefix(m.Name, "Microsoft Visual Studio Community ") || !strings.Contains(m.Command, "Microsoft.VisualStudio.Product.Community") {
		return ""
	}
	for _, component := range visualStudioComponents {
		if !strings.Contains(m.Command, component) {
			return ""
		}
	}
	switch strings.TrimPrefix(m.Name, "Microsoft Visual Studio Community ") {
	case "2019":
		return "16."
	case "2022":
		return "17."
	case "2026":
		return "18."
	}
	return ""
}

func applyVisualStudioState(modules []editorModule, allJSON, readyJSON []byte) {
	var all, ready []visualStudioInstance
	if json.Unmarshal(allJSON, &all) != nil || json.Unmarshal(readyJSON, &ready) != nil || all == nil || ready == nil {
		return
	}
	readyIDs := make(map[string]bool, len(ready))
	for _, instance := range ready {
		if instance.InstanceID != "" && instance.IsComplete && instance.IsLaunchable && instance.ProductID == "Microsoft.VisualStudio.Product.Community" {
			readyIDs[instance.InstanceID] = true
		}
	}
	for i := range modules {
		m := &modules[i]
		major := visualStudioModuleMajor(*m)
		if major == "" {
			continue
		}
		m.Selected, m.PresenceUnverified, m.ToolStatus, m.ToolPath, m.ToolInstanceID = false, false, "", "", ""
		var candidates []visualStudioInstance
		for _, instance := range all {
			if instance.InstanceID == "" || instance.ProductID != "Microsoft.VisualStudio.Product.Community" || !strings.HasPrefix(instance.InstallationVersion, major) || !instance.IsComplete || !instance.IsLaunchable {
				continue
			}
			if readyIDs[instance.InstanceID] {
				m.Selected, m.ToolStatus, m.ToolInstanceID = true, "ready", instance.InstanceID
				m.ToolPath = filepath.Clean(instance.InstallationPath)
				break
			}
			if filepath.IsAbs(instance.InstallationPath) {
				candidates = append(candidates, instance)
			}
		}
		if !m.Selected {
			switch len(candidates) {
			case 1:
				m.ToolStatus, m.ToolPath, m.ToolInstanceID = "incomplete", filepath.Clean(candidates[0].InstallationPath), candidates[0].InstanceID
			default:
				if len(candidates) > 1 {
					m.ToolStatus = "ambiguous"
				}
			}
		}
	}
}

func visualStudioModifyArgs(m editorModule) ([]string, error) {
	if visualStudioModuleMajor(m) == "" || m.ToolStatus != "incomplete" || m.ToolInstanceID == "" || !filepath.IsAbs(m.ToolPath) {
		return nil, fmt.Errorf("visual studio installation is unavailable or ambiguous")
	}
	args := []string{"modify", "--installPath", filepath.Clean(m.ToolPath)}
	for _, component := range visualStudioComponents {
		args = append(args, "--add", component)
	}
	return args, nil
}

func (a *app) openVisualStudioModifier(e editorInstall, module editorModule) {
	if a.win == nil || a.moduleLoading || a.moduleBusy || len(a.moduleQueue) != 0 || a.visualStudioWatching() {
		return
	}
	if _, err := visualStudioModifyArgs(module); err != nil {
		a.moduleStatus = locales.VisualStudioInstanceChanged()
		return
	}
	a.moduleLoading = true
	a.moduleStatus = ""
	win := a.win
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		current := module
		current.ToolStatus, current.ToolPath, current.ToolInstanceID = "", "", ""
		modules := []editorModule{current}
		checkVisualStudioModules(ctx, modules)
		current = modules[0]
		win.Update(func() {
			a.moduleLoading = false
			if !a.manageOpen || !a.manageModules || a.manageEditor.Exe != e.Exe {
				return
			}
			if current.ToolStatus != "incomplete" || current.ToolInstanceID != module.ToolInstanceID || !strings.EqualFold(filepath.Clean(current.ToolPath), filepath.Clean(module.ToolPath)) {
				a.moduleStatus = locales.VisualStudioInstanceChanged()
				return
			}
			if err := startVisualStudioModifier(current); err != nil {
				a.moduleStatus = locales.OpenFailed(err.Error())
				return
			}
			a.moduleStatus = locales.VisualStudioInstallerOpened()
			a.startVisualStudioWatch(e, current)
		})
	}()
}

// visualStudioTargetReady accepts only the exact instance selected before
// launching the installer; another ready copy is not proof for this job.
func visualStudioTargetReady(module editorModule, allJSON, readyJSON []byte) bool {
	var all, ready []visualStudioInstance
	if json.Unmarshal(allJSON, &all) != nil || json.Unmarshal(readyJSON, &ready) != nil || module.ToolInstanceID == "" || !filepath.IsAbs(module.ToolPath) {
		return false
	}
	matched := false
	for _, instance := range all {
		if instance.InstanceID == module.ToolInstanceID && instance.ProductID == "Microsoft.VisualStudio.Product.Community" && strings.HasPrefix(instance.InstallationVersion, visualStudioModuleMajor(module)) && instance.IsComplete && instance.IsLaunchable && strings.EqualFold(filepath.Clean(instance.InstallationPath), filepath.Clean(module.ToolPath)) {
			matched = true
			break
		}
	}
	if !matched || visualStudioModuleMajor(module) == "" {
		return false
	}
	for _, instance := range ready {
		if instance.InstanceID == module.ToolInstanceID && instance.IsComplete && instance.IsLaunchable && instance.ProductID == "Microsoft.VisualStudio.Product.Community" {
			return true
		}
	}
	return false
}

func waitVisualStudioReady(ctx context.Context, module editorModule, interval time.Duration, query func(context.Context) ([]byte, []byte, error)) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		probe, cancel := context.WithTimeout(ctx, 15*time.Second)
		all, ready, err := query(probe)
		cancel()
		if err == nil && visualStudioTargetReady(module, all, ready) {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (a *app) startVisualStudioWatch(e editorInstall, module editorModule) {
	if a.visualStudioWatching() {
		return
	}
	parent := a.updateCtx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, time.Hour)
	task := &visualStudioTask{Editor: e, Module: module, Status: "waiting", Cancel: cancel}
	a.visualStudioTask = task
	win := a.win
	go func() {
		err := waitVisualStudioReady(ctx, module, 5*time.Second, queryVisualStudioInstances)
		cancel()
		if parent.Err() != nil {
			return
		}
		win.Update(func() {
			if a.visualStudioTask != task || task.Status != "waiting" {
				return
			}
			if err != nil {
				task.Status = "unverified"
				if a.moduleStatus == locales.VisualStudioInstallerOpened() {
					a.moduleStatus = locales.VisualStudioUnverified()
				}
				return
			}
			task.Status = "ready"
			if a.moduleStatus == locales.VisualStudioInstallerOpened() {
				a.moduleStatus = locales.VisualStudioVerified()
			}
			if a.manageEditor.Exe == e.Exe {
				for i := range a.manageEditor.Modules {
					m := &a.manageEditor.Modules[i]
					if m.ID == module.ID && m.Command == module.Command && m.Name == module.Name {
						m.Selected, m.PresenceUnverified, m.ToolStatus = true, false, "ready"
					}
				}
			}
		})
	}()
}

func (a *app) dismissVisualStudioTask() {
	if task := a.visualStudioTask; task != nil {
		if task.Cancel != nil {
			task.Cancel()
		}
		a.visualStudioTask = nil
	}
}

func (a *app) visualStudioActivityBanner(c *ui.Context) {
	if !a.visualStudioWatching() || a.page == pageActivity {
		return
	}
	ui.Row(c).Padding(8, 22).Gap(10).AlignItems(ui.Center).Background(c.Theme().Surface).Children(func() {
		ui.Text(c, locales.VisualStudioWaiting()).FontSize(12).Grow(1).Clip()
		ui.Progress(c, -1).Width(130)
		ui.Button(c, locales.ActivityViewAll()).OnClick(func() { a.page = pageActivity })
	})
}
