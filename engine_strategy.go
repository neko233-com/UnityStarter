// engine_strategy.go —— 每个 engine 独立的品牌 + 行为策略。
//
// Unity 和 Tuanjie 不是同构的两个 tab：它们各有自己的品牌色、
// 自己的图标、自己的下载源、自己的模板来源、自己的授权机制。
// engineStrategy 把这些差异收敛在一处，view 层只通过
// a.engineStrategy(kind) 拿到策略，不再散落 if/else。

package main

import "github.com/egoist/mygo/ui"

// engineBranding 描述一个 engine 的品牌标识（视觉层）。
type engineBranding struct {
	Kind      string   // kindUnity / kindTuanjie
	Name      string   // "Unity" / "团结引擎"
	ShortName string   // "U" / "T"
	Icon      *ui.SVG  // iconUnity / iconTuanjie
	Accent    ui.Color // 品牌色，选中态高亮 / 品牌图标 tint
	BgActive  ui.Color // 选中态背景
	BgHover   ui.Color // 悬停态背景
}

// engineRoutePages 描述一个 engine 的页面路由映射。
type engineRoutePages struct {
	Projects  string
	Installs  string
	Downloads string
	Favorites string
}

// engineStrategy 描述一个 engine 的完整策略（视觉 + 路由 + 行为）。
type engineStrategy struct {
	engineBranding
	Pages engineRoutePages
	// 行为策略 —— 每个 engine 各有一套
	NeedsLicenseFlow bool // Unity 需要授权流程，Tuanjie 不需要
	DownloadChannel  string
	TemplateSource   string
	AssetStoreURL    string
	DocsURL          string
	LicenseURL       string
}

// engineStrategyFor 返回指定 engine 的策略。
func engineStrategyFor(kind string) engineStrategy {
	if kind == kindTuanjie {
		return tuanjieStrategy()
	}
	return unityStrategy()
}

// engineBrandingFor 返回指定 engine 的品牌标识（不含行为）。
func engineBrandingFor(kind string) engineBranding {
	return engineStrategyFor(kind).engineBranding
}

// engineValid 判断 kind 是否是已知 engine。
func engineValid(kind string) bool {
	return kind == kindUnity || kind == kindTuanjie
}

func unityStrategy() engineStrategy {
	return engineStrategy{
		engineBranding: engineBranding{
			Kind:      kindUnity,
			Name:      "Unity",
			ShortName: "U",
			Icon:      iconUnity,
			Accent:    ui.Hex("#E4002B"),
			BgActive:  ui.Hex("#E4002B"),
			BgHover:   ui.Hex("#3a2a2d"),
		},
		Pages: engineRoutePages{
			Projects:  pageProjects,
			Installs:  pageInstalls,
			Downloads: pageDownloads,
			Favorites: pageFavorites,
		},
		NeedsLicenseFlow: true,
		DownloadChannel:   "unity-releases",
		TemplateSource:    "unity-hub",
		AssetStoreURL:     urlAssetStoreUnity,
		DocsURL:           urlDocsUnity,
		LicenseURL:        urlLicenseUnity,
	}
}

func tuanjieStrategy() engineStrategy {
	return engineStrategy{
		engineBranding: engineBranding{
			Kind:      kindTuanjie,
			Name:      "团结引擎",
			ShortName: "T",
			Icon:      iconTuanjie,
			Accent:    ui.Hex("#2196F3"),
			BgActive:  ui.Hex("#2196F3"),
			BgHover:   ui.Hex("#223240"),
		},
		Pages: engineRoutePages{
			Projects:  pageTuanjieProjects,
			Installs:  pageTuanjieInstalls,
			Downloads: pageTuanjieDownloads,
			Favorites: pageTuanjieFavorites,
		},
		NeedsLicenseFlow: false,
		DownloadChannel:   "tuanjie-cdn",
		TemplateSource:    "tuanjie-hub",
		AssetStoreURL:     urlAssetStoreTuanjie,
		DocsURL:           urlDocsTuanjie,
		LicenseURL:        urlLicenseTuanjie,
	}
}

// engineStrategy 返回当前 activeEngine 的策略。
func (a *app) engineStrategy() engineStrategy {
	return engineStrategyFor(a.activeEngine)
}
