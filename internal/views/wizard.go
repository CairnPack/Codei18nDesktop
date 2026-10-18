package views

import (
	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n-desktop/locales"
)

// wizardDialog 是未初始化项目的初始化向导（模态框）。控件直接绑定 app.Wizard 的字段。
func wizardDialog(c *ui.Context, r *Root) {
	a := r.App
	w := a.Wizard
	if w == nil {
		return
	}
	t := c.Theme()
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(560).Gap(12).Padding(20).Children(func() {
			ui.Text(c, locales.WizTitle()).FontSize(20).Bold()
			ui.Text(c, w.Project).TextColor(t.TextMuted).FontSize(12).Selectable()
			ui.Text(c, locales.WizIntro()).TextColor(t.TextMuted)
			ui.Form(c, func() {
				ui.Field(c, locales.WizSourceLanguage(), func() {
					ui.TextInput(c, &w.SourceLanguage).Label(locales.WizSourceLanguage()).Width(200)
				}).Description(locales.WizSourceLanguageHint())
				ui.Field(c, locales.WizLocalLanguage(), func() {
					ui.TextInput(c, &w.LocalLanguage).Label(locales.WizLocalLanguage()).Width(200)
				}).Description(locales.WizLocalLanguageHint())
				ui.Field(c, locales.WizExcludes(), func() {
					ui.TextArea(c, &w.Excludes).Lines(4, 8).Label(locales.WizExcludes())
				}).Description(locales.WizExcludesHint())
				ui.Field(c, locales.WizProfile(), func() {
					// 配置档的值（名称或空）与显示文本不同，用菜单按钮而不是 Select
					label := w.Profile
					if label == "" {
						label = locales.WizProfileNone()
					}
					ui.Row(c).Children(func() {
						ui.MenuButton(c, label, func(m *ui.Menu) {
							if m.Item(locales.WizProfileNone()).Checked(w.Profile == "").Chosen() {
								w.Profile = ""
							}
							if len(w.Profiles) > 0 {
								m.Separator()
							}
							for _, name := range w.Profiles {
								if m.Item(name).Checked(w.Profile == name).Chosen() {
									w.Profile = name
								}
							}
						}).Label(locales.WizProfile())
					})
				}).Description(locales.WizProfileHint())
			})
			if w.IsGitRepo {
				ui.Checkbox(c, &w.InstallHooks, locales.WizInstallHooks())
				ui.Checkbox(c, &w.Gitignore, locales.WizGitignore())
			} else {
				ui.Text(c, locales.WizNotGit()).TextColor(t.TextMuted).FontSize(12)
			}
			ui.Checkbox(c, &w.ScanAfter, locales.WizScanAfter())
			if w.Err != "" {
				ui.Text(c, w.Err).TextColor(t.Danger).Selectable()
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, locales.Cancel()).OnClick(a.CancelWizard)
				// 提交会关闭向导（a.Wizard 置空），放到构建结束后执行
				ui.PrimaryButton(c, locales.WizInitialize()).OnClick(a.SubmitWizard)
			})
		})
	})
	if !open {
		a.CancelWizard()
	}
}
