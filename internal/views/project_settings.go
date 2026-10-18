package views

import (
	"slices"

	"github.com/CairnPack/codei18n/core/config"
	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n-desktop/internal/app"
	"github.com/CairnPack/codei18n-desktop/locales"
)

// providers 是可选的翻译 provider。
var providers = []string{"openai", "ollama", "mock"}

// projectSettingsPage 是项目设置页：每个字段显示生效值、来源层徽章，可单独覆盖或恢复继承；
// 底部是项目专用令牌。
func projectSettingsPage(c *ui.Context, r *Root) {
	a := r.App
	if a.ProjectForm == nil && r.ui.psTried != a.Selected {
		r.ui.psTried = a.Selected
		a.OpenProjectSettings()
	}
	f := a.ProjectForm
	th := c.Theme()
	ui.Row(c).Gap(12).Children(func() {
		if ui.Button(c, locales.PsBack()).Clicked() {
			a.Page = app.PageOverview
		}
		ui.Text(c, locales.PsTitle()).FontSize(22).Bold()
	})
	if f == nil {
		return
	}
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Gap(12).Padding(0, 4, 16, 0).Children(func() {
			ui.Text(c, locales.PsIntro()).TextColor(th.TextMuted)
			ui.Row(c).Gap(6).Children(func() {
				ui.Text(c, locales.PsLegend()).TextColor(th.TextMuted).FontSize(12)
				for _, l := range app.Layers() {
					layerBadge(c.Key(string(l)), l)
				}
			})
			if f.Err != "" {
				ui.Text(c, f.Err).TextColor(th.Danger)
			}
			ui.Form(c, func() {
				for i := range f.Rows {
					fieldRow(c.Key(string(f.Rows[i].Key)), f, &f.Rows[i])
				}
			})
			ui.Row(c).Children(func() {
				if ui.Button(c, locales.PsResetAll()).Clicked() {
					r.ui.confirmReset = true
				}
			})
			ui.Divider(c)
			ui.Text(c, locales.PsTokenTitle()).FontSize(16).Bold()
			ui.Text(c, locales.PsTokenHint()).TextColor(th.TextMuted).FontSize(12)
			tokenBox(c, f.Tokens)
		})
	})

	// 恢复全部继承：已保存项目令牌时询问是否一并删除（默认按钮为保留令牌）。
	if f.Tokens != nil && f.Tokens.Saved {
		switch ui.AlertDialog(c, &r.ui.confirmReset, locales.PsResetAllTitle(), locales.PsResetAllTokenMessage(),
			locales.Cancel(), locales.PsDeleteToken(), locales.PsKeepToken()) {
		case 1:
			f.ResetAll(true)
		case 2:
			f.ResetAll(false)
		}
	} else if ui.AlertDialog(c, &r.ui.confirmReset, locales.PsResetAllTitle(), locales.PsResetAllMessage(),
		locales.Cancel(), locales.PsResetAllConfirm()) == 1 {
		f.ResetAll(false)
	}
}

// fieldRow 显示一个字段：编辑器、来源徽章、覆盖 / 恢复继承按钮。
func fieldRow(c *ui.Context, f *app.ProjectForm, row *app.FieldRow) {
	key := row.Key
	ui.Field(c, fieldLabel(key), func() {
		ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
			fieldEditor(c, f, row)
			layerBadge(c, row.Source)
			if row.Edit != row.Value && ui.PrimaryButton(c, locales.PsOverride()).Clicked() {
				f.Override(key)
			}
			if row.Overridden() && ui.Button(c, locales.PsReset()).Tooltip(locales.PsResetHint()).Clicked() {
				f.Reset(key)
			}
		})
	})
}

// fieldEditor 按字段类型选择编辑器：列表用多行文本，配置档与 provider 用下拉选择。
func fieldEditor(c *ui.Context, f *app.ProjectForm, row *app.FieldRow) {
	label := fieldLabel(row.Key)
	switch {
	case row.Multiline():
		ui.TextArea(c, &row.Edit).Lines(2, 6).Label(label).Grow(1)
	case row.Key == app.FieldProfile && len(f.Profiles) > 0:
		ui.Select(c, &row.Edit, withCurrent(f.Profiles, row.Edit)).Label(label).Grow(1)
	case row.Key == app.FieldProvider:
		ui.Select(c, &row.Edit, withCurrent(providers, row.Edit)).Label(label).Grow(1)
	default:
		ui.TextInput(c, &row.Edit).Label(label).Grow(1)
	}
}

// withCurrent 返回选项列表；当前值不在其中时追加到末尾，避免下拉框丢失已有的值。
func withCurrent(options []string, current string) []string {
	if current == "" || slices.Contains(options, current) {
		return options
	}
	return append(slices.Clip(options), current)
}

// layerBadge 显示来源层徽章；项目层用强调色。
func layerBadge(c *ui.Context, l config.Layer) {
	th := c.Theme()
	b := ui.Badge(c, layerLabel(l))
	switch l {
	case config.LayerProject:
		b.Background(th.Accent).TextColor(th.AccentText)
	case config.LayerOverride:
		b.Background(th.Warning.Alpha(0.3))
	}
}

// layerLabel 返回来源层的显示文本。
func layerLabel(l config.Layer) string {
	switch l {
	case config.LayerGlobal:
		return locales.PsSourceGlobal()
	case config.LayerProject:
		return locales.PsSourceProject()
	case config.LayerOverride:
		return locales.PsSourceOverride()
	}
	return locales.PsSourceDefault()
}

// fieldLabel 返回字段的显示名称。
func fieldLabel(k app.FieldKey) string {
	switch k {
	case app.FieldSourceLanguage:
		return locales.PsFieldSourceLanguage()
	case app.FieldLocalLanguage:
		return locales.PsFieldLocalLanguage()
	case app.FieldProfile:
		return locales.PsFieldProfile()
	case app.FieldProvider:
		return locales.PsFieldProvider()
	case app.FieldBaseURL:
		return locales.PsFieldBaseUrl()
	case app.FieldModel:
		return locales.PsFieldModel()
	case app.FieldEndpoint:
		return locales.PsFieldEndpoint()
	case app.FieldBatchSize:
		return locales.PsFieldBatchSize()
	case app.FieldConcurrency:
		return locales.PsFieldConcurrency()
	case app.FieldExcludePatterns:
		return locales.PsFieldExcludePatterns()
	case app.FieldDetectLanguages:
		return locales.PsFieldDetectLanguages()
	}
	return string(k)
}
