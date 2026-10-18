package views_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/secrets"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n-desktop/internal/app"
	"github.com/CairnPack/codei18n-desktop/internal/views"
	"github.com/CairnPack/codei18n-desktop/locales"
)

// fakeShell 记录视图请求的 UI 语言切换。
type fakeShell struct {
	locales []string
	cur     string
}

func (s *fakeShell) PickFolders(func([]string)) {}
func (s *fakeShell) SetLocale(tag string)       { s.locales = append(s.locales, tag); s.cur = tag }
func (s *fakeShell) Locale() string             { return s.cur }

// env 是视图测试的环境：真实 core（临时全局目录 + 内存凭据库），后台投递的函数在测试 goroutine 中执行。
type env struct {
	t      *testing.T
	app    *app.App
	shell  *fakeShell
	tt     *ui.Tester
	q      chan func()
	global string
	store  *secrets.Memory
}

func newEnv(t *testing.T, globalConfig string) *env {
	t.Helper()
	e := &env{t: t, q: make(chan func(), 100000), global: t.TempDir(), store: secrets.NewMemory(), shell: &fakeShell{}}
	if globalConfig != "" {
		if err := os.WriteFile(filepath.Join(e.global, "config.json"), []byte(globalConfig), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	core := app.NewCore(codei18n.WithGlobalDir(e.global), codei18n.WithSecretStore(e.store),
		codei18n.WithEnv(func(string) string { return "" }))
	e.app = app.New(app.Deps{Core: core, Update: func(fn func()) { e.q <- fn }, DataDir: t.TempDir()})
	root := &views.Root{App: e.app, Shell: e.shell}
	e.tt = ui.NewTester(root.View, 1400, 2400)
	return e
}

// until 执行后台投递的函数直到 cond 成立，然后重绘一帧。
func (e *env) until(what string, cond func() bool) {
	e.t.Helper()
	deadline := time.After(30 * time.Second)
	for !cond() {
		select {
		case fn := <-e.q:
			fn()
		case <-time.After(10 * time.Millisecond):
		case <-deadline:
			e.t.Fatalf("超时等待: %s", what)
		}
	}
	e.tt.Frame()
}

func (e *env) click(s string) {
	e.t.Helper()
	if err := e.tt.Click(s); err != nil {
		e.t.Fatalf("%v; texts %q", err, e.tt.Texts())
	}
}

// noSecret 断言界面上任何文本都不包含令牌明文。
func (e *env) noSecret(secret string) {
	e.t.Helper()
	for _, s := range e.tt.Texts() {
		if strings.Contains(s, secret) || strings.Contains(s, secret[:8]) {
			e.t.Fatalf("界面显示了令牌明文: %q", s)
		}
	}
}

// clickInRow 点击与 rowLabel 同一行（纵向重叠）的 button 文本。
func clickInRow(e *env, rowLabel, button string) {
	e.t.Helper()
	row, ok := e.tt.Find(rowLabel)
	if !ok {
		e.t.Fatalf("找不到行 %q", rowLabel)
	}
	// Tester 只提供第一个匹配的 Find；用坐标点击：取按钮的横坐标与行标签的纵坐标。
	btn, ok := e.tt.Find(button)
	if !ok {
		e.t.Fatalf("找不到按钮 %q", button)
	}
	e.tt.ClickAt(btn.X+btn.W/2, row.Y+row.H/2)
}

func count(texts []string, s string) int {
	n := 0
	for _, t := range texts {
		if t == s {
			n++
		}
	}
	return n
}

// openProject 加入一个项目目录并打开它的设置页。
func (e *env) openProject() *app.ProjectForm {
	e.t.Helper()
	dir := e.t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\n// Hello world comment\nfunc main() {}\n"), 0o644)
	e.app.AddPaths([]string{dir})
	e.until("快速载入", func() bool {
		p := e.app.SelectedProject()
		return p != nil && (p.Status != nil || p.Err != "")
	})
	// 项目令牌需要已初始化的项目：走向导流程初始化，并等待首次扫描结束。
	e.app.OpenWizard(dir)
	e.app.SubmitWizard()
	if e.app.Wizard != nil {
		e.t.Fatalf("向导提交失败: %s", e.app.Wizard.Err)
	}
	e.until("首次扫描完成", func() bool {
		p := e.app.SelectedProject()
		return len(e.app.ActiveJobs()) == 0 && p.Status != nil && p.Status.Initialized
	})
	e.app.Page = app.PageProjectSettings
	e.tt.Frame()
	f := e.app.ProjectForm
	if f == nil || f.Err != "" {
		e.t.Fatalf("项目设置未打开: %+v; texts %q", f, e.tt.Texts())
	}
	return f
}

const globalWithProfile = `{"profile":"p","profiles":{"p":{"provider":"openai","model":"gpt-x","baseUrl":"https://api.example.com/v1"}}}`

func TestProjectSettingsBadgesOverrideReset(t *testing.T) {
	e := newEnv(t, globalWithProfile)
	f := e.openProject()
	texts := e.tt.Texts()
	if !slices.Contains(texts, locales.PsTitle()) || count(texts, locales.PsSourceGlobal()) < 3 || count(texts, locales.PsSourceDefault()) < 3 {
		t.Fatalf("应显示来源徽章: %q", texts)
	}
	baseProject := count(texts, locales.PsSourceProject()) // 图例中的一个
	// 向导写入了项目层字段（如排除规则），这些字段显示恢复按钮；未编辑时没有覆盖按钮
	baseReset := count(texts, locales.PsReset())
	if slices.Contains(texts, locales.PsOverride()) || baseReset == 0 || f.Row(app.FieldExcludePatterns).Source != config.LayerProject {
		t.Fatalf("初始按钮状态: %q", texts)
	}

	f.Row(app.FieldModel).Edit = "gpt-y"
	e.tt.Frame()
	e.click(locales.PsOverride())
	if r := f.Row(app.FieldModel); r.Value != "gpt-y" || r.Source != config.LayerProject {
		t.Fatalf("覆盖后应来自项目层: %+v", r)
	}
	if n := count(e.tt.Texts(), locales.PsSourceProject()); n != baseProject+1 {
		t.Fatalf("应多一个项目徽章: %d", n)
	}
	if n := count(e.tt.Texts(), locales.PsReset()); n != baseReset+1 {
		t.Fatalf("覆盖后应显示恢复按钮: %d", n)
	}
	// 点击模型字段那一行的恢复按钮（Find 返回第一个匹配，按位置找同一行的）
	clickInRow(e, locales.PsFieldModel(), locales.PsReset())
	if r := f.Row(app.FieldModel); r.Value != "gpt-x" || r.Source != config.LayerGlobal {
		t.Fatalf("恢复继承: %+v", r)
	}
	if n := count(e.tt.Texts(), locales.PsReset()); n != baseReset {
		t.Fatalf("恢复后该字段不应再显示恢复按钮: %d", n)
	}

	e.click(locales.PsBack())
	if e.app.Page != app.PageOverview {
		t.Fatalf("返回按钮: %s", e.app.Page)
	}
}

func TestProjectTokenAndRestoreAll(t *testing.T) {
	e := newEnv(t, globalWithProfile)
	f := e.openProject()
	const secret = "sk-project-token-ABCD"

	// 遮罩输入 → 保存 → 只显示末 4 位，输入框清空，明文从不出现在界面上
	e.click(locales.TokInput())
	e.tt.Type(secret)
	if f.Tokens.Input != secret {
		t.Fatalf("输入应绑定到面板: %q", f.Tokens.Input)
	}
	e.noSecret(secret)
	e.click(locales.TokSave())
	if f.Tokens.Input != "" || !f.Tokens.Saved {
		t.Fatalf("%+v", f.Tokens)
	}
	if !e.tt.HasText(locales.TokSaved("ABCD")) || !e.tt.HasText(locales.TokInUse(locales.TokSourceProject())) {
		t.Fatalf("应显示已保存与来源: %q", e.tt.Texts())
	}
	e.noSecret(secret)

	// 恢复全部继承：对话框询问是否一并删除项目令牌
	f.Row(app.FieldLocalLanguage).Edit = "ja"
	e.tt.Frame()
	e.click(locales.PsOverride())
	if f.Row(app.FieldLocalLanguage).Source != config.LayerProject {
		t.Fatal("覆盖失败")
	}
	e.click(locales.PsResetAll())
	if !e.tt.HasText(locales.PsResetAllTokenMessage()) {
		t.Fatalf("应询问是否删除令牌: %q", e.tt.Texts())
	}
	e.click(locales.PsDeleteToken())
	if f.Tokens.Saved || f.Row(app.FieldLocalLanguage).Source == config.LayerProject {
		t.Fatalf("应恢复继承并删除项目令牌: %+v", f.Tokens)
	}
	if !e.tt.HasText(locales.TokNotSaved()) {
		t.Fatalf("%q", e.tt.Texts())
	}
}

func TestTokenDeleteNeedsConfirmation(t *testing.T) {
	e := newEnv(t, "")
	e.app.Page = app.PageGlobalSettings
	e.tt.Frame()
	e.click(locales.GsNewProfileName())
	e.tt.Type("main")
	e.click(locales.GsNew())
	g := e.app.GlobalForm
	e.click(locales.TokInput())
	e.tt.Type("sk-delete-me-1234")
	e.click(locales.TokSave())
	if !g.Tokens.Saved {
		t.Fatal("应已保存")
	}

	e.click(locales.TokDelete())
	if !e.tt.HasText(locales.TokDeleteTitle()) || !g.Tokens.Saved {
		t.Fatal("删除应先弹出确认")
	}
	e.click(locales.Cancel())
	if !g.Tokens.Saved || g.Tokens.ConfirmDelete {
		t.Fatal("取消后令牌应保留")
	}
	e.click(locales.TokDelete())
	e.click(locales.TokDeleteConfirm())
	if g.Tokens.Saved {
		t.Fatal("确认后应删除")
	}
	if _, err := e.store.Get("profile/main"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("凭据库中应已删除: %v", err)
	}
	if !e.tt.HasText(locales.TokNotSaved()) {
		t.Fatalf("%q", e.tt.Texts())
	}
}

func TestGlobalSettingsProfiles(t *testing.T) {
	e := newEnv(t, "")
	e.app.Page = app.PageGlobalSettings
	e.tt.Frame()
	if !e.tt.HasText(locales.GsNoProfiles()) {
		t.Fatalf("%q", e.tt.Texts())
	}

	// 新建
	e.click(locales.GsNewProfileName())
	e.tt.Type("deepseek")
	e.click(locales.GsNew())
	g := e.app.GlobalForm
	if g.Selected != "deepseek" || !slices.Contains(e.tt.Texts(), "deepseek") {
		t.Fatalf("应新建并选中: %+v", g)
	}

	// 编辑并保存
	e.click(locales.GsModel())
	e.tt.Type("deepseek-chat")
	e.click(locales.GsBaseUrl())
	e.tt.Type("https://api.deepseek.com")
	e.click(locales.GsSave())
	if g.Err != "" || g.Edit.Model != "deepseek-chat" || g.Edit.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("保存后应从文件重新载入: %+v", g)
	}

	// 令牌
	const secret = "sk-global-token-WXYZ"
	e.click(locales.TokInput())
	e.tt.Type(secret)
	e.click(locales.TokSave())
	if s, err := e.store.Get("profile/deepseek"); err != nil || s.Last4() != "WXYZ" {
		t.Fatalf("令牌应保存到凭据库: %v", err)
	}
	if !e.tt.HasText(locales.TokSaved("WXYZ")) {
		t.Fatalf("%q", e.tt.Texts())
	}
	e.noSecret(secret)

	// 重命名保留令牌
	g.Edit.Name = "ds"
	e.tt.Frame()
	e.click(locales.GsSave())
	if g.Selected != "ds" || !slices.Contains(e.tt.Texts(), "ds") {
		t.Fatalf("重命名: %+v", g)
	}
	if s, err := e.store.Get("profile/ds"); err != nil || s.Last4() != "WXYZ" || !e.tt.HasText(locales.TokSaved("WXYZ")) {
		t.Fatalf("重命名应保留令牌: %v", err)
	}

	// 设为默认
	e.click(locales.GsUseDefault())
	if g.Default != "ds" || !e.tt.HasText(locales.GsDefaultBadge()) {
		t.Fatalf("设为默认: %+v", g)
	}

	// 复制（不复制令牌），然后删除原配置档（确认对话框，令牌一并删除）
	e.click(locales.GsCopy())
	if g.Selected != "ds-copy" || g.Tokens.Saved {
		t.Fatalf("复制: %+v", g)
	}
	e.click("ds")
	e.click(locales.GsDelete())
	if !e.tt.HasText(locales.GsDeleteTitle("ds")) || !e.tt.HasText(locales.GsDeleteMessage()) {
		t.Fatalf("删除应确认: %q", e.tt.Texts())
	}
	e.click(locales.GsDeleteConfirm())
	if slices.Contains(g.Profiles, "ds") {
		t.Fatalf("应已删除: %v", g.Profiles)
	}
	if _, err := e.store.Get("profile/ds"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatal("删除配置档应删除令牌")
	}
}

func TestGlobalSettingsTestConnection(t *testing.T) {
	e := newEnv(t, `{"profiles":{"m":{"provider":"mock"}}}`)
	e.app.Page = app.PageGlobalSettings
	e.tt.Frame()
	g := e.app.GlobalForm
	if g.Selected != "m" {
		t.Fatalf("%+v", g)
	}
	e.click(locales.TokTest())
	if !g.Tokens.Testing || !e.tt.HasText(locales.TokTesting()) {
		t.Fatalf("测试中应显示进度: %q", e.tt.Texts())
	}
	e.until("测试完成", func() bool { return g.Tokens.TestDone })
	if g.Tokens.TestErr != "" {
		t.Fatalf("mock 应连接成功: %s", g.Tokens.TestErr)
	}
	if !e.tt.HasText(locales.TokTestOK(int(g.Tokens.TestLatency.Milliseconds()))) {
		t.Fatalf("应显示延迟: %q", e.tt.Texts())
	}
}

func TestGlobalSettingsAppSection(t *testing.T) {
	e := newEnv(t, "")
	e.app.Page = app.PageGlobalSettings
	e.tt.Frame()
	if !e.tt.HasText(locales.GsLanguageHint()) {
		t.Fatal("应说明 UI 语言与翻译目标语言无关")
	}
	e.click(i18n.Autonym("ja"))
	if !slices.Equal(e.shell.locales, []string{"ja"}) {
		t.Fatalf("应立即切换 UI 语言: %q", e.shell.locales)
	}
	e.click(locales.GsLanguageSystem())
	if !slices.Equal(e.shell.locales, []string{"ja", ""}) {
		t.Fatalf("跟随系统应传空串: %q", e.shell.locales)
	}

	e.click(locales.GsWatch())
	if !e.app.Settings.Watch {
		t.Fatal("文件监视开关应写入设置")
	}
	e.click(locales.GsMaxParallelLabel())
	e.tt.Key(0, ui.KeyUp)
	if e.app.Settings.MaxParallel != 2 || !e.tt.HasText(locales.GsMaxParallel(2)) {
		t.Fatalf("并行上限: %d", e.app.Settings.MaxParallel)
	}
}
