package main

import (
	"encoding/json"
	"strings"
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
		m.Selected, m.PresenceUnverified, m.ToolStatus = false, false, ""
		for _, instance := range all {
			if instance.InstanceID == "" || instance.ProductID != "Microsoft.VisualStudio.Product.Community" || !strings.HasPrefix(instance.InstallationVersion, major) || !instance.IsComplete || !instance.IsLaunchable {
				continue
			}
			if readyIDs[instance.InstanceID] {
				m.Selected, m.ToolStatus = true, "ready"
				break
			}
			m.ToolStatus = "incomplete"
		}
	}
}
