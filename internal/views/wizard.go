package views

import (
	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n-desktop/locales"
)

// wizardDialog 待实现（占位）：未初始化项目的初始化向导。
func wizardDialog(c *ui.Context, r *Root) {
	open := true
	ui.Modal(c, &open, func() {
		ui.Text(c, locales.NotImplemented())
		if ui.Button(c, locales.Cancel()).Clicked() {
			r.App.CancelWizard()
		}
	})
	if !open {
		r.App.CancelWizard()
	}
}
