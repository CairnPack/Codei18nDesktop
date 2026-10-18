// Codei18n Desktop：基于 mygo 原生 UI 的 CodeI18n 桌面端，进程内调用 core 库。
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n-desktop/internal/app"
	"github.com/CairnPack/codei18n-desktop/internal/views"
	"github.com/CairnPack/codei18n-desktop/locales"
)

func main() {
	// 固定应用名：配置目录、单实例锁都依赖它（go run / mygo dev / 安装版一致）。
	mygo.App.SetName("Codei18n")
	root := &views.Root{}
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     locales.AppTitle(),
			Width:     1200,
			Height:    780,
			MinWidth:  860,
			MinHeight: 560,
			StateKey:  "main",
			Content:   ui.View(root.View),
		})
		dataDir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Printf("无法获取配置目录: %v", err)
		}
		a := app.New(app.Deps{
			Core:    app.NewCore(),
			Update:  win.Update,
			DataDir: dataDir,
			Watcher: watcher{},
		})
		if a.Settings.Locale != "" {
			mygo.App.SetLocale(a.Settings.Locale)
		}
		mygo.App.OnLocaleChanged(func(string) { win.SetTitle(locales.AppTitle()) })
		win.Update(func() {
			root.App = a
			root.Shell = &shell{win: win, app: a}
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
