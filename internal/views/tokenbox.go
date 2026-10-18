package views

import (
	"strings"

	"github.com/CairnPack/codei18n"
	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n-desktop/internal/app"
	"github.com/CairnPack/codei18n-desktop/locales"
)

// tokenBox 是可复用的令牌输入框：默认遮罩输入，保存后只显示"已保存 ••••abcd"与来源层，
// 支持替换、删除（二次确认）、测试连接，以及明文令牌的迁移提示。已保存的令牌永远不会回显。
func tokenBox(c *ui.Context, t *app.TokenPanel) {
	if t == nil {
		return
	}
	th := c.Theme()
	ui.Column(c).Gap(8).Children(func() {
		// 保存状态：只显示末 4 位。
		ui.Row(c).Gap(8).Children(func() {
			if t.Saved {
				ui.Badge(c, locales.TokSaved(t.Last4)).Background(th.Success.Alpha(0.18))
			} else {
				ui.Badge(c, locales.TokNotSaved())
			}
			if t.Scope == app.ScopeProject {
				ui.Text(c, locales.TokInUse(tokenSourceText(t))).TextColor(th.TextMuted)
			}
		})
		if t.Scope == app.ScopeGlobal {
			ui.Text(c, locales.TokGlobalHint()).TextColor(th.TextMuted).FontSize(12)
		}
		if t.Blocked {
			noticeBar(c, th.Warning, locales.TokBlocked(), "", nil)
		}
		if len(t.Plaintext) > 0 {
			noticeBar(c, th.Warning, locales.TokPlaintext(strings.Join(t.Plaintext, ", ")), locales.TokMigrate(), t.MigratePlaintext)
		}

		// 输入行：Reveal 只影响正在输入的内容（不调用 Password 即显示）。
		ui.Row(c).Gap(8).Children(func() {
			in := ui.TextInput(c, &t.Input).Label(locales.TokInput()).Grow(1)
			if t.Saved {
				in.Placeholder(locales.TokPlaceholderReplace())
			} else {
				in.Placeholder(locales.TokPlaceholderNew())
			}
			if !t.Reveal {
				in.Password()
			}
			submitted := in.Submitted()
			ui.Toggle(c, &t.Reveal, locales.TokShow())
			if ui.PrimaryButton(c, locales.TokSave()).Disabled(t.Input == "").Clicked() || (submitted && t.Input != "") {
				t.Save()
			}
			if t.Saved && ui.Button(c, locales.TokDelete()).Clicked() {
				t.AskDelete()
			}
		})

		// 测试连接：进行中显示转圈，完成后显示延迟或（已脱敏的）错误。
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, locales.TokTest()).Disabled(t.Testing).Clicked() {
				t.Test()
			}
			switch {
			case t.Testing:
				ui.Spinner(c)
				ui.Text(c, locales.TokTesting()).TextColor(th.TextMuted)
			case t.TestDone && t.TestErr != "":
				ui.Text(c, locales.TokTestFailed(t.TestErr)).TextColor(th.Danger)
			case t.TestDone:
				ui.Text(c, locales.TokTestOK(int(t.TestLatency.Milliseconds()))).TextColor(th.Success)
			}
		})
		if t.Err != "" {
			ui.Text(c, t.Err).TextColor(th.Danger)
		}
	})

	// 删除的二次确认。
	switch ui.AlertDialog(c, &t.ConfirmDelete, locales.TokDeleteTitle(), locales.TokDeleteMessage(),
		locales.Cancel(), locales.TokDeleteConfirm()) {
	case 1:
		t.ConfirmDeleteNow()
	case 0, ui.AlertDismissed:
		t.CancelDelete()
	}
}

// tokenSourceText 返回项目最终生效令牌的来源层文本。
func tokenSourceText(t *app.TokenPanel) string {
	switch t.Source {
	case codei18n.TokenSourceEnv:
		return locales.TokSourceEnv(t.EnvVar)
	case codei18n.TokenSourceProject:
		return locales.TokSourceProject()
	case codei18n.TokenSourceGlobal:
		return locales.TokSourceGlobal()
	case codei18n.TokenSourceFile:
		return locales.TokSourceFile()
	}
	return locales.TokSourceNone()
}

// noticeBar 是带颜色的提示条；action 非空时右侧显示一个按钮。
func noticeBar(c *ui.Context, color ui.Color, text, actionLabel string, action func()) {
	ui.Row(c).Gap(12).Padding(8, 12).Radius(8).Background(color.Alpha(0.15)).Border(1, color).Children(func() {
		ui.Text(c, text).Grow(1)
		if action != nil && ui.Button(c, actionLabel).Clicked() {
			action()
		}
	})
}
