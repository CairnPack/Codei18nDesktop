package views

import (
	"strings"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n-desktop/internal/app"
	"github.com/CairnPack/codei18n-desktop/locales"
)

// globalSettingsPage 是全局设置页：左侧 provider 配置档列表，右侧所选配置档的编辑表单与令牌，
// 下方是桌面端自己的设置（UI 语言、文件监视、并行项目上限）。
func globalSettingsPage(c *ui.Context, r *Root) {
	a := r.App
	if a.GlobalForm == nil && !r.ui.gsTried {
		r.ui.gsTried = true
		a.OpenGlobalSettings()
	}
	f := a.GlobalForm
	th := c.Theme()
	ui.Text(c, locales.GsTitle()).FontSize(22).Bold()
	if f == nil {
		return
	}
	if r.ui.gsForm != f {
		// 换了表单（重新进入页面）：从应用状态重新同步页面局部状态。
		r.ui.gsForm = f
		r.ui.languageChoice = a.Settings.Locale
		r.ui.languageSet = a.Settings.Locale
		r.ui.watch = a.Settings.Watch
		r.ui.maxParallel = float64(max(a.Settings.MaxParallel, 1))
		r.ui.newProfileName = ""
	}
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Gap(16).Padding(0, 4, 16, 0).Children(func() {
			if len(f.Plaintext) > 0 {
				noticeBar(c, th.Warning, locales.TokPlaintext(strings.Join(f.Plaintext, ", ")), locales.TokMigrate(), f.MigratePlaintext)
			}
			if f.Err != "" {
				ui.Text(c, f.Err).TextColor(th.Danger)
			}
			ui.Row(c).Gap(24).AlignItems(ui.Start).Children(func() {
				profileList(c, r, f)
				ui.Column(c).Grow(1).Gap(12).Children(func() {
					if f.Selected != "" {
						profileEditor(c, f)
					}
				})
			})
			ui.Divider(c)
			appSettings(c, r)
		})
	})

	// 删除配置档的二次确认：说明它保存的令牌也会被删除。
	if ui.AlertDialog(c, &r.ui.confirmDelete, locales.GsDeleteTitle(f.Selected), locales.GsDeleteMessage(),
		locales.Cancel(), locales.GsDeleteConfirm()) == 1 {
		f.DeleteSelected()
	}
}

// profileList 是配置档列表与新建 / 复制 / 删除 / 设为默认按钮。
func profileList(c *ui.Context, r *Root, f *app.GlobalForm) {
	th := c.Theme()
	ui.Column(c).Width(260).Gap(8).Children(func() {
		ui.Text(c, locales.GsProfiles()).FontSize(16).Bold()
		if len(f.Profiles) == 0 {
			ui.Text(c, locales.GsNoProfiles()).TextColor(th.TextMuted)
		}
		for _, name := range f.Profiles {
			ui.Row(c.Key(name)).Gap(6).Children(func() {
				var b ui.Element
				if name == f.Selected {
					b = ui.PrimaryButton(c, name)
				} else {
					b = ui.Button(c, name)
				}
				if b.Grow(1).Clicked() && name != f.Selected {
					f.SelectProfile(name)
				}
				if name == f.Default {
					ui.Badge(c, locales.GsDefaultBadge())
				}
			})
		}
		ui.Row(c).Gap(6).Children(func() {
			in := ui.TextInput(c, &r.ui.newProfileName).Label(locales.GsNewProfileName()).
				Placeholder(locales.GsNewProfileName()).Grow(1)
			if ui.Button(c, locales.GsNew()).Clicked() || in.Submitted() {
				f.NewProfile(strings.TrimSpace(r.ui.newProfileName))
				r.ui.newProfileName = ""
			}
		})
		if f.Selected != "" {
			ui.Row(c).Gap(6).Children(func() {
				if ui.Button(c, locales.GsCopy()).Clicked() {
					f.CopySelected()
				}
				if ui.Button(c, locales.GsDelete()).Clicked() {
					r.ui.confirmDelete = true
				}
			})
			if f.Selected != f.Default && ui.Button(c, locales.GsUseDefault()).Clicked() {
				f.SetDefault(f.Selected)
			}
		}
	})
}

// profileEditor 是所选配置档的编辑表单（改名即重命名，core 会迁移令牌）与它的令牌。
func profileEditor(c *ui.Context, f *app.GlobalForm) {
	e := &f.Edit
	ui.Form(c, func() {
		ui.Field(c, locales.GsName(), func() { ui.TextInput(c, &e.Name) })
		ui.Field(c, locales.GsProvider(), func() { ui.Select(c, &e.Provider, withCurrent(providers, e.Provider)) })
		ui.Field(c, locales.GsBaseUrl(), func() { ui.TextInput(c, &e.BaseURL) })
		ui.Field(c, locales.GsModel(), func() { ui.TextInput(c, &e.Model) })
		if e.Provider == "ollama" || e.Endpoint != "" {
			ui.Field(c, locales.GsEndpoint(), func() { ui.TextInput(c, &e.Endpoint) })
		}
		ui.Field(c, locales.GsBatchSize(), func() { ui.TextInput(c, &e.BatchSize).Width(120) })
		ui.Field(c, locales.GsConcurrency(), func() { ui.TextInput(c, &e.Concurrency).Width(120) })
	})
	ui.Row(c).Children(func() {
		if ui.PrimaryButton(c, locales.GsSave()).Disabled(strings.TrimSpace(e.Name) == "").Clicked() {
			f.SaveSelected()
		}
	})
	ui.Text(c, locales.GsTokenTitle()).FontSize(16).Bold()
	// 按配置档区分令牌框的控件状态，切换配置档时输入框不串用。
	tokenBox(c.Key("token:"+f.Selected), f.Tokens)
}

// appSettings 是桌面端自己的设置：UI 语言（立即生效）、文件监视、并行项目上限。
func appSettings(c *ui.Context, r *Root) {
	a := r.App
	th := c.Theme()
	ui.Text(c, locales.GsApp()).FontSize(16).Bold()
	ui.Field(c, locales.GsLanguage(), func() {
		ui.RadioGroup(c, func() {
			ui.Radio(c, &r.ui.languageChoice, "", locales.GsLanguageSystem())
			for _, tag := range UILanguages {
				ui.Radio(c.Key(tag), &r.ui.languageChoice, tag, i18n.Autonym(tag))
			}
		})
	}).Description(locales.GsLanguageHint())
	ui.Column(c).Gap(4).Children(func() {
		ui.Checkbox(c, &r.ui.watch, locales.GsWatch())
		ui.Text(c, locales.GsWatchHint()).TextColor(th.TextMuted).FontSize(12)
	})
	ui.Column(c).Gap(4).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, locales.GsMaxParallel(int(r.ui.maxParallel)))
			ui.Stepper(c, &r.ui.maxParallel, 1, 16, 1).Label(locales.GsMaxParallelLabel())
		})
		ui.Text(c, locales.GsMaxParallelHint()).TextColor(th.TextMuted).FontSize(12)
	})

	// 绑定值在构建之后才更新，因此在下一帧比较编辑值与已应用的值。
	if r.ui.languageChoice != r.ui.languageSet {
		r.ui.languageSet = r.ui.languageChoice
		r.Shell.SetLocale(r.ui.languageChoice)
	}
	if mp := int(r.ui.maxParallel); r.ui.watch != a.Settings.Watch || mp != a.Settings.MaxParallel {
		s := a.Settings
		s.Watch, s.MaxParallel = r.ui.watch, mp
		a.SetSettings(s)
	}
}
