// Package views 是桌面端的 mygo 原生 UI 视图函数。视图只读写 app.App 的状态并调用其方法，
// 不直接做 I/O；需要操作系统的动作（目录对话框、切换 UI 语言）通过 Shell 注入，便于无窗口视图测试。
// 所有用户可见文本都来自 locales 目录（代码里不硬编码）。
package views

import (
	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n-desktop/internal/app"
	"github.com/CairnPack/codei18n-desktop/locales"
)

// Shell 是视图需要的操作系统能力。
type Shell interface {
	// PickFolders 打开系统目录对话框（异步），用户选择后在主线程回调。
	PickFolders(onPicked func(paths []string))
	// SetLocale 切换 UI 语言（空串表示跟随系统），立即生效。
	SetLocale(tag string)
	// Locale 返回当前 UI 语言。
	Locale() string
}

// UILanguages 是提供的 UI 语言（ARB 目录）。注意这与项目的翻译目标语言是两个独立的设置。
var UILanguages = []string{"en", "zh-Hans", "zh-Hant", "ja"}

// Root 是窗口的根视图。
type Root struct {
	App   *app.App
	Shell Shell

	sidebarWidth float32
	// 各页面的局部 UI 状态（列表滚动、对话框开关等），不属于应用状态。
	ui pageState
}

// pageState 是视图层的局部状态（不持久化）。
type pageState struct {
	confirmRemove   bool
	confirmReset    bool
	browserList     ui.ListState
	jobsList        ui.ListState
	newProfileName  string
	languageChoice  string
	browserLangEdit string
}

// View 构建整个窗口：左侧项目侧栏，右侧当前页面；整个窗口接受文件夹拖放。
func (r *Root) View(c *ui.Context) {
	a := r.App
	if a == nil {
		return
	}
	if r.sidebarWidth == 0 {
		r.sidebarWidth = 260
	}
	for _, n := range a.TakeNotices() {
		c.Toast(n.Text)
	}
	root := ui.Row(c).Fill().AlignItems(ui.Stretch)
	if files := root.DroppedFiles(); len(files) > 0 {
		a.AddPaths(files)
	}
	root.Children(func() {
		r.sidebar(c)
		ui.Divider(c)
		ui.Column(c).Grow(1).Padding(16).Gap(12).Children(func() {
			r.page(c)
		})
	})
	if a.Wizard != nil {
		wizardDialog(c, r)
	}
}

// sidebar 显示项目列表与各项目状态，以及全局入口。
func (r *Root) sidebar(c *ui.Context) {
	a := r.App
	selected := a.Selected
	switch a.Page {
	case app.PageGlobalSettings:
		selected = "page:global"
	case app.PageTasks:
		selected = "page:tasks"
	}
	before := selected
	ui.Column(c).Width(r.sidebarWidth).Children(func() {
		ui.Sidebar(c, &selected, func() {
			ui.SidebarSection(c, locales.SidebarProjects(), nil, func() {
				for _, p := range a.Projects {
					ui.SidebarItem(c.Key(p.Path), p.Path, nil, p.Name).Tooltip(p.Path).Children(func() {
						stateBadge(c, p.State())
					})
				}
			})
			ui.SidebarSection(c, locales.SidebarApp(), nil, func() {
				_, running := a.Summary()
				item := ui.SidebarItem(c, "page:tasks", nil, locales.SidebarTasks())
				if running > 0 {
					item.Children(func() { ui.Badge(c, locales.RunningCount(running)) })
				}
				ui.SidebarItem(c, "page:global", nil, locales.SidebarGlobalSettings())
			})
		}).Grow(1)
		ui.Row(c).Padding(8).Gap(8).Children(func() {
			if ui.PrimaryButton(c, locales.AddProject()).Clicked() {
				r.Shell.PickFolders(func(paths []string) { a.AddPaths(paths) })
			}
		})
		ui.Text(c, locales.DropHint()).TextColor(c.Theme().TextMuted).Padding(0, 8, 8, 8).FontSize(12)
	})
	if selected != before {
		switch selected {
		case "page:global":
			a.OpenGlobalSettings()
		case "page:tasks":
			a.Page = app.PageTasks
		default:
			a.Select(selected)
			a.Page = app.PageOverview
		}
	}
}

// stateBadge 显示项目状态（未初始化 / 已初始化 / 有待翻译 / 扫描中 / 翻译中 / 出错）。
func stateBadge(c *ui.Context, s app.ProjectState) {
	ui.Badge(c, StateLabel(s))
}

// StateLabel 返回项目状态的显示文本。
func StateLabel(s app.ProjectState) string {
	switch s {
	case app.StateLoading:
		return locales.StateLoading()
	case app.StateNotInitialized:
		return locales.StateNotInitialized()
	case app.StateReady:
		return locales.StateReady()
	case app.StatePending:
		return locales.StatePending()
	case app.StateScanning:
		return locales.StateScanning()
	case app.StateTranslating:
		return locales.StateTranslating()
	case app.StateError:
		return locales.StateError()
	}
	return string(s)
}

// page 按 app.Page 显示当前页面。
func (r *Root) page(c *ui.Context) {
	a := r.App
	switch a.Page {
	case app.PageGlobalSettings:
		globalSettingsPage(c, r)
		return
	case app.PageTasks:
		tasksPage(c, r)
		return
	}
	if a.SelectedProject() == nil {
		emptyState(c, r)
		return
	}
	switch a.Page {
	case app.PageBrowser:
		browserPage(c, r)
	case app.PageProjectSettings:
		projectSettingsPage(c, r)
	default:
		overviewPage(c, r)
	}
}

// emptyState 是没有项目时的引导。
func emptyState(c *ui.Context, r *Root) {
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Text(c, locales.EmptyTitle()).FontSize(22).Bold()
		ui.Text(c, locales.EmptyHint()).TextColor(c.Theme().TextMuted)
		if ui.PrimaryButton(c, locales.AddProject()).Clicked() {
			r.Shell.PickFolders(func(paths []string) { r.App.AddPaths(paths) })
		}
	})
}
