package main

import (
	"github.com/egoist/mygo"

	"github.com/CairnPack/codei18n-desktop/internal/app"
	"github.com/CairnPack/codei18n-desktop/locales"
)

// shell 实现 views.Shell：系统目录对话框与 UI 语言切换。
type shell struct {
	win *mygo.Window
	app *app.App
}

func (s *shell) PickFolders(onPicked func(paths []string)) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: s.win, Title: locales.AddProject(), Directory: true, Multiple: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		s.win.Update(func() { onPicked(paths) })
	}()
}

func (s *shell) SetLocale(tag string) {
	mygo.App.SetLocale(tag)
	st := s.app.Settings
	st.Locale = tag
	s.app.SetSettings(st)
}

func (s *shell) Locale() string { return mygo.App.Locale() }
