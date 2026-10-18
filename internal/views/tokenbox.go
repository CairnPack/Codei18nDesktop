package views

import (
	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n-desktop/internal/app"
	"github.com/CairnPack/codei18n-desktop/locales"
)

// tokenBox 待实现（占位）：令牌输入框组件（遮罩、保存、替换、删除确认、测试连接、明文迁移）。
func tokenBox(c *ui.Context, t *app.TokenPanel) {
	if t == nil {
		return
	}
	ui.Text(c, locales.NotImplemented())
}
