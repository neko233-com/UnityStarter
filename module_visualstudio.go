package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

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
		m.Selected, m.PresenceUnverified, m.ToolStatus, m.ToolPath = false, false, "", ""
		var candidates []string
		for _, instance := range all {
			if instance.InstanceID == "" || instance.ProductID != "Microsoft.VisualStudio.Product.Community" || !strings.HasPrefix(instance.InstallationVersion, major) || !instance.IsComplete || !instance.IsLaunchable {
				continue
			}
			if readyIDs[instance.InstanceID] {
				m.Selected, m.ToolStatus = true, "ready"
				break
			}
			if filepath.IsAbs(instance.InstallationPath) {
				candidates = append(candidates, filepath.Clean(instance.InstallationPath))
			}
		}
		if !m.Selected {
			switch len(candidates) {
			case 1:
				m.ToolStatus, m.ToolPath = "incomplete", candidates[0]
			default:
				if len(candidates) > 1 {
					m.ToolStatus = "ambiguous"
				}
			}
		}
	}
}

func visualStudioModifyArgs(m editorModule) ([]string, error) {
	if visualStudioModuleMajor(m) == "" || m.ToolStatus != "incomplete" || !filepath.IsAbs(m.ToolPath) {
		return nil, fmt.Errorf("visual studio installation is unavailable or ambiguous")
	}
	args := []string{"modify", "--installPath", filepath.Clean(m.ToolPath)}
	for _, component := range visualStudioComponents {
		args = append(args, "--add", component)
	}
	return args, nil
}

func (a *app) openVisualStudioModifier(e editorInstall, module editorModule) {
	if a.win == nil || a.moduleLoading || a.moduleBusy || len(a.moduleQueue) != 0 {
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
		current.ToolStatus, current.ToolPath = "", ""
		modules := []editorModule{current}
		checkVisualStudioModules(ctx, modules)
		current = modules[0]
		win.Update(func() {
			a.moduleLoading = false
			if !a.manageOpen || !a.manageModules || a.manageEditor.Exe != e.Exe {
				return
			}
			if current.ToolStatus != "incomplete" || !strings.EqualFold(filepath.Clean(current.ToolPath), filepath.Clean(module.ToolPath)) {
				a.moduleStatus = locales.VisualStudioInstanceChanged()
				return
			}
			if err := startVisualStudioModifier(current); err != nil {
				a.moduleStatus = locales.OpenFailed(err.Error())
				return
			}
			a.moduleStatus = locales.VisualStudioInstallerOpened()
		})
	}()
}
