package main

import (
	"fmt"
	"net/url"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

// The official pages this app opens in the browser. They are the ones the
// hubs themselves link to.
const (
	urlAssetStoreTuanjie = "https://assetstore.u3d.cn"
	urlAssetStoreUnity   = "https://assetstore.unity.com"
	urlCommunity         = "https://developer.unity.cn/unityHub"
	urlLearn             = "https://learn.u3d.cn/"
	urlGamePublishing    = "https://unity.cn/gamepublishing"
	urlMiniGameCenter    = "https://minihost.tuanjie.cn/hub"
	urlUOS               = "https://uos.u3dcloud.cn/hub"
	urlCloudRendering    = "https://cloudrendering.unity.cn/hub-home"
	urlPartner           = "https://partner.u3d.cn/hub"
	urlLicenseTuanjie    = "https://license.tuanjie.cn"
	urlLicenseUnity      = "https://license.unity.cn"
	urlUnityID           = "https://id.unity.cn"
	urlDocsTuanjie       = "https://docs.unity.cn/cn/tuanjiemanual/Manual/index.html"
	urlDocsUnity         = "https://docs.unity3d.com/Manual/index.html"
	urlScriptingAPI      = "https://docs.unity3d.com/ScriptReference/"
	urlRepo              = "https://github.com/neko233-com/UnityStarter"
	urlIssues            = "https://github.com/neko233-com/UnityStarter/issues/new"
)

// link is an entry of a page of links.
type link struct {
	Title  string
	Detail string
	URL    string
}

// linkGroup is a titled group of links.
type linkGroup struct {
	Title string
	Links []link
}

// linksPage shows titled groups of links, with the app's header above them.
func (a *app) linksPage(c *ui.Context, title, hint string, groups func(), extra func()) {
	a.header(c, title, hint, func() {})
	ui.Divider(c)
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(24, 26).Gap(22).MaxWidth(820).Children(func() {
			if extra != nil {
				extra()
			}
			groups()
		})
	})
}

func (a *app) linkGroup(c *ui.Context, title string, links []link) {
	t := c.Theme()
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, title).FontSize(14).Bold()
		for _, l := range links {
			l := l
			ui.Row(c).Gap(12).AlignItems(ui.Center).Padding(6, 0).Children(func() {
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, l.Title).FontSize(13)
					ui.Text(c, l.Detail).FontSize(11).TextColor(t.TextMuted)
				})
				if ui.Button(c, locales.ActionOpen()).Clicked() {
					_ = mygo.Shell.OpenExternal(l.URL)
				}
			})
		}
	})
}

func (a *app) storePage(c *ui.Context) {
	a.linksPage(c, locales.PageStore(), locales.PageStoreHint(), func() {
		a.linkGroup(c, locales.GroupStores(), []link{
			{locales.LinkAssetStoreTuanjie(), locales.LinkAssetStoreTuanjieHint(), urlAssetStoreTuanjie},
			{locales.LinkAssetStoreUnity(), locales.LinkAssetStoreUnityHint(), urlAssetStoreUnity},
		})
		a.linkGroup(c, locales.GroupLearning(), []link{
			{"Unity 中文课堂", locales.LinkLearnHint(), urlLearn},
			{locales.LinkCommunity(), locales.LinkCommunityHint(), urlCommunity},
		})
	}, nil)
}

func (a *app) publishPage(c *ui.Context) {
	a.linksPage(c, locales.PagePublish(), locales.PagePublishHint(), func() {
		a.linkGroup(c, locales.GroupChannels(), []link{
			{locales.LinkGamePublishing(), locales.LinkGamePublishingHint(), urlGamePublishing},
			{locales.LinkMiniGame(), locales.LinkMiniGameHint(), urlMiniGameCenter},
		})
		a.linkGroup(c, locales.GroupServices(), []link{
			{locales.LinkUOS(), locales.LinkUOSHint(), urlUOS},
			{locales.LinkCloudRender(), locales.LinkCloudRenderHint(), urlCloudRendering},
			{locales.LinkPartner(), locales.LinkPartnerHint(), urlPartner},
		})
	}, nil)
}

func (a *app) licensePage(c *ui.Context) {
	t := c.Theme()
	found := licenseLocations()
	a.linksPage(c, locales.PageLicense(), locales.PageLicenseHint(), func() {
		a.linkGroup(c, locales.GroupLicenses(), []link{
			{locales.LinkLicenseTuanjie(), locales.LinkLicenseTuanjieHint(), urlLicenseTuanjie},
			{locales.LinkLicenseUnity(), locales.LinkLicenseUnityHint(), urlLicenseUnity},
			{locales.LinkAccount(), locales.LinkAccountHint(), urlUnityID},
		})
	}, func() {
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, locales.LicensesFound()).FontSize(14).Bold()
			if len(found) == 0 {
				ui.Text(c, locales.LicensesNone()).FontSize(12).TextColor(t.TextMuted)
			}
			for _, loc := range found {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, loc.Path).FontSize(12).Grow(1).Clip()
					if loc.IsDir {
						ui.Text(c, locales.LicensesFiles(loc.Files)).FontSize(11).TextColor(t.TextMuted)
					}
					ui.Text(c, humanSize(loc.Size)).FontSize(11).TextColor(t.TextMuted).Width(76)
					ui.Text(c, a.formatTime(loc.Modified)).FontSize(11).TextColor(t.TextMuted).Width(96)
					if ui.Button(c, locales.ActionFolder()).Clicked() {
						mygo.Shell.ShowItemInFolder(loc.Path)
					}
				})
			}
		})
	})
}

func (a *app) docsPage(c *ui.Context) {
	a.linksPage(c, locales.PageDocs(), locales.PageDocsHint(), func() {
		a.linkGroup(c, locales.GroupEngineDocs(), []link{
			{locales.LinkDocsTuanjie(), locales.LinkDocsTuanjieHint(), urlDocsTuanjie},
			{locales.LinkDocsUnity(), locales.LinkDocsUnityHint(), urlDocsUnity},
			{locales.LinkScripting(), locales.LinkScriptingHint(), urlScriptingAPI},
		})
		a.linkGroup(c, locales.GroupProjectDocs(), []link{
			{locales.LinkRepo(), locales.LinkRepoHint(), urlRepo},
		})
	}, nil)
}

func (a *app) feedbackPage(c *ui.Context) {
	t := c.Theme()
	a.linksPage(c, locales.PageFeedback(), locales.PageFeedbackHint(), func() {}, func() {
		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, locales.Diagnostics()).FontSize(14).Bold()
			ui.Text(c, a.diagnostics()).FontSize(11).TextColor(t.TextMuted)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.PrimaryButton(c, locales.ActionReportIssue()).Clicked() {
					_ = mygo.Shell.OpenExternal(a.issueURL())
				}
				if ui.Button(c, locales.ActionCopy()).Clicked() {
					mygo.Clipboard.WriteText(a.diagnostics())
					a.diagCopied = true
				}
				if a.diagCopied {
					ui.Text(c, locales.Copied()).FontSize(11).TextColor(t.TextMuted)
				}
			})
		})
	})
}

// diagnostics is what a bug report needs to say about this computer, without
// the paths of the projects.
func (a *app) diagnostics() string {
	var b strings.Builder
	fmt.Fprintf(&b, "UnityStarter %s (%s/%s)\n", appVersion, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "Locale: %s\n", i18n.Locale())
	fmt.Fprintf(&b, "Editors: %d\n", len(a.editors))
	for _, e := range a.editors {
		fmt.Fprintf(&b, "  %s %s\n", e.Version, e.Kind)
	}
	fmt.Fprintf(&b, "Projects: %d\n", len(a.projects))
	for _, p := range a.projects {
		fmt.Fprintf(&b, "  %s %s hub=%s\n", p.Name, p.Version, p.Hub)
	}
	fmt.Fprintf(&b, "Hubs: %d\n", len(a.hubs))
	for _, h := range a.hubs {
		fmt.Fprintf(&b, "  %s %s %s\n", h.Kind, h.Version, hubMode(h))
	}
	return b.String()
}

func (a *app) issueURL() string {
	q := url.Values{}
	q.Set("title", locales.IssueTitle())
	q.Set("body", locales.IssueBody(a.diagnostics()))
	return urlIssues + "?" + q.Encode()
}
