package main

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"unitystarter/locales"
)

// Several jobs share one status strip; their details stay in the fixed task page.
func (a *app) workspaceActivityBanner(c *ui.Context) {
	if a.page == pageActivity {
		return
	}
	switch {
	case a.uninstallBusy && !a.manageOpen:
		a.uninstallActivityBanner(c)
	case !a.manageOpen && (a.moduleBusy || a.moduleLoading || len(a.moduleQueue) > 0 || a.androidPlan != nil):
		a.moduleActivityBanner(c)
	case a.visualStudioWatching():
		a.visualStudioActivityBanner(c)
	case a.templateJob != nil && a.templateJob.Busy && !a.newOpen:
		a.templateActivityBanner(c)
	case !a.manageOpen && a.uninstallStatus != "":
		a.uninstallActivityBanner(c)
	case !a.manageOpen && a.manageModules && a.moduleStatus != "":
		a.moduleActivityBanner(c)
	case len(a.activityJobs(true)) > 0:
		a.editorActivityBanner(c)
	case a.templateJob != nil && !a.newOpen:
		a.templateActivityBanner(c)
	}
}

func (a *app) workspaceTaskCount() int {
	n := len(a.activityJobs(true))
	if a.moduleBusy || a.moduleLoading || len(a.moduleQueue) > 0 || a.androidPlan != nil {
		n++
	}
	if a.uninstallBusy {
		n++
	}
	if a.visualStudioWatching() {
		n++
	}
	if a.templateJob != nil && a.templateJob.Busy {
		n++
	}
	return n
}

func (a *app) statusMessage(c *ui.Context, message string) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, message).FontSize(12).TextColor(c.Theme().TextMuted).Grow(1).MinWidth(0).SingleLine().Clip().Tooltip(message)
		if len(message) > 120 || strings.Contains(message, "\n") {
			ui.Button(c, locales.StatusDetails()).OnClick(func() { a.alert(message) })
		}
	})
}

func sidebarSection(c *ui.Context, title string, open *bool, children func()) ui.Element {
	_, height := c.Size()
	if height < 700 {
		children()
		return ui.Element{}
	}
	return ui.SidebarSection(c, title, open, children)
}

func sidebarItem(c *ui.Context, id string, icon *ui.SVG, label string) ui.Element {
	item := ui.SidebarItem(c, id, icon, label)
	_, height := c.Size()
	if height < 700 {
		item.Height(28).Padding(4, 8)
	}
	return item
}

func (a *app) uninstallActivityCard(c *ui.Context) {
	if !a.uninstallBusy && strings.TrimSpace(a.uninstallStatus) == "" {
		return
	}
	panel(c, func() {
		ui.Text(c, locales.InstallUninstall()+" · "+displayVersion(a.manageEditor.Version, a.manageEditor.DisplayVersion)).FontSize(14).Bold()
		ui.Text(c, a.uninstallStatus).FontSize(12).TextColor(c.Theme().TextMuted)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if a.uninstallBusy {
				ui.Progress(c, -1).Grow(1)
			}
			ui.Button(c, locales.InstallManage()).OnClick(func() { a.openEditorManagement(a.manageEditor) })
		})
	})
}
