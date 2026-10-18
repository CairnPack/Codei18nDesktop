package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n-desktop/internal/app"
	"github.com/CairnPack/codei18n-desktop/internal/views"
	"github.com/CairnPack/codei18n-desktop/locales"
)

type fakeShell struct{ picked []string }

func (s *fakeShell) PickFolders(onPicked func([]string)) { onPicked(s.picked) }
func (s *fakeShell) SetLocale(string)                     {}
func (s *fakeShell) Locale() string                       { return "" }

// 冒烟：空工作区显示引导，点击添加后项目出现在侧栏。
func TestRootSmoke(t *testing.T) {
	dir := t.TempDir()
	a := app.New(app.Deps{Core: app.NewCore(), Update: func(fn func()) { fn() }, DataDir: t.TempDir()})
	root := &views.Root{App: a, Shell: &fakeShell{picked: []string{dir}}}
	tt := ui.NewTester(root.View, 1200, 780)
	if !tt.HasText(locales.EmptyTitle()) {
		t.Fatalf("应显示空工作区引导: %q", tt.Texts())
	}
	if err := tt.Click(locales.AddProject()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.Projects) != 1 {
		t.Fatalf("应加入项目: %d", len(a.Projects))
	}
}
