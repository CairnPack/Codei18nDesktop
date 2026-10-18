package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n-desktop/internal/app"
	"github.com/CairnPack/codei18n-desktop/internal/views"
	"github.com/CairnPack/codei18n-desktop/locales"
	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/secrets"
)

// e2e 驱动真实的根视图，只通过界面完成完整流程（§9）：
// 添加 2 个项目 → 初始化 → 全局配置档与项目覆写 → 扫描 → 批量翻译（mock）→ 注释浏览器查看与编辑。
type e2e struct {
	t  *testing.T
	q  chan func()
	tt *ui.Tester
	a  *app.App
}

func (e *e2e) pump() {
	for {
		select {
		case fn := <-e.q:
			fn()
		default:
			e.tt.Frame()
			return
		}
	}
}

func (e *e2e) until(what string, cond func() bool) {
	e.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			e.t.Fatalf("超时等待: %s; 界面文本 %q", what, e.tt.Texts())
		}
		e.pump()
		time.Sleep(5 * time.Millisecond)
	}
	e.pump()
}

func (e *e2e) click(label string) {
	e.t.Helper()
	e.pump()
	if err := e.tt.Click(label); err != nil {
		e.t.Fatalf("点击 %q 失败: %v; 界面文本 %q", label, err, e.tt.Texts())
	}
	e.pump()
}

func sourceProject(t *testing.T, name string, n int) string {
	dir := filepath.Join(t.TempDir(), name)
	os.MkdirAll(dir, 0o755)
	for i := 0; i < n; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.go", i)),
			[]byte(fmt.Sprintf("package p\n\n// Reads the %s settings number %d\nfunc F%d() {}\n", name, i, i)), 0o644)
	}
	return dir
}

func TestEndToEndTwoProjects(t *testing.T) {
	global := t.TempDir()
	store := secrets.NewMemory()
	core := app.NewCore(codei18n.WithGlobalDir(global), codei18n.WithSecretStore(store),
		codei18n.WithEnv(func(string) string { return "" }))
	e := &e2e{t: t, q: make(chan func(), 100000)}
	e.a = app.New(app.Deps{Core: core, Update: func(fn func()) { e.q <- fn }, DataDir: t.TempDir()})
	alpha, beta := sourceProject(t, "alpha", 3), sourceProject(t, "beta", 2)
	root := &views.Root{App: e.a, Shell: &fakeShell{picked: []string{alpha, beta}}}
	e.tt = ui.NewTester(root.View, 1400, 900)

	// 1. 添加两个项目（系统目录对话框由 Shell 注入）
	e.click(locales.AddProject())
	e.until("两个项目快速载入完成", func() bool {
		return len(e.a.Projects) == 2 && e.a.Projects[0].Status != nil && e.a.Projects[1].Status != nil
	})

	// 2. 全局设置：新建 mock 配置档并设为默认
	e.click(locales.SidebarGlobalSettings())
	if err := e.tt.Click(locales.GsNewProfileName()); err != nil {
		t.Fatal(err)
	}
	e.tt.Type("local-mock")
	e.click(locales.GsNew())
	g := e.a.GlobalForm
	if g == nil || g.Selected != "local-mock" {
		t.Fatalf("应新建配置档: %+v", g)
	}
	g.Edit.Provider = "mock" // provider 下拉框的值，直接设置绑定字段
	e.click(locales.GsSave())
	e.click(locales.GsUseDefault())
	if g.Default != "local-mock" || g.Err != "" {
		t.Fatalf("默认配置档: %+v", g)
	}

	// 3. 逐个项目：初始化向导 → 首次扫描
	for _, p := range []string{alpha, beta} {
		e.click(filepath.Base(p))
		e.click(locales.OvInitialize())
		if e.a.Wizard == nil {
			t.Fatal("向导未打开")
		}
		e.click(locales.WizInitialize())
		entry := e.a.SelectedProject()
		e.until("首次扫描完成 "+p, func() bool {
			return entry.Running() == nil && entry.Status != nil && entry.Status.HasIndex && entry.Status.Pending > 0
		})
	}

	// 4. 项目覆写：alpha 覆盖本地语言字段以外的模型字段，来源变为"项目"
	e.click("alpha")
	e.click(locales.OvSettings())
	f := e.a.ProjectForm
	if f == nil {
		t.Fatal("项目设置未打开")
	}
	row := f.Row(app.FieldModel)
	row.Edit = "mock-model"
	e.pump()
	e.click(locales.PsOverride())
	if r := f.Row(app.FieldModel); r.Value != "mock-model" || r.Source != config.LayerProject {
		t.Fatalf("项目覆写: %+v", r)
	}
	if r := f.Row(app.FieldProvider); r.Value != "mock" || r.Source != config.LayerGlobal {
		t.Fatalf("provider 应继承全局配置档: %+v", r)
	}
	e.click(locales.PsBack())

	// 5. 多项目批量翻译（mock）
	e.click(locales.SidebarTasks())
	e.click(locales.TaskSelectAll())
	e.click(locales.TaskTranslateSelected())
	e.until("批量翻译完成", func() bool {
		if len(e.a.ActiveJobs()) > 0 {
			return false
		}
		for _, p := range e.a.Projects {
			if p.Status == nil || p.Status.Pending != 0 {
				return false
			}
		}
		return true
	})
	for _, j := range e.a.Jobs {
		if j.Kind == app.JobTranslate && j.State != app.JobDone {
			t.Fatalf("翻译任务应完成: %+v", j)
		}
	}

	// 6. 注释浏览器：查看并编辑一条译文
	e.click("alpha")
	e.click(locales.OvBrowse())
	b := e.a.Browser
	e.until("注释载入", func() bool { return b != nil && !b.Loading && len(b.Records) == 3 })
	if b.Stats().Coverage() != 1 {
		t.Fatalf("覆盖率应为 100%%: %+v", b.Stats())
	}
	b.Select(b.Filtered()[0])
	e.pump()
	if !strings.Contains(b.Edit, "MOCK") {
		t.Fatalf("应显示 mock 译文: %q", b.Edit)
	}
	b.Edit = "// 读取 alpha 设置"
	e.click(locales.BrSave())
	if b.Err != "" {
		t.Fatal(b.Err)
	}
	p, _ := codei18n.Open(alpha, codei18n.WithGlobalDir(global), codei18n.WithSecretStore(store))
	recs, _ := p.Comments()
	edited := false
	for _, r := range recs {
		if r.Translation("zh-CN") == "// 读取 alpha 设置" {
			edited = true
		}
	}
	if !edited {
		t.Fatal("编辑应写回 mappings.json")
	}
}
