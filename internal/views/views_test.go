package views_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/secrets"

	"github.com/CairnPack/codei18n-desktop/internal/app"
	"github.com/CairnPack/codei18n-desktop/internal/views"
	"github.com/CairnPack/codei18n-desktop/locales"
)

type noopShell struct{}

func (noopShell) PickFolders(func([]string)) {}
func (noopShell) SetLocale(string)           {}
func (noopShell) Locale() string             { return "" }

// pageEnv 是无窗口视图测试的环境：后台 goroutine 通过 Update 投递的函数排队，
// 由测试 goroutine（模拟主线程）执行后再绘制一帧。
type pageEnv struct {
	t    *testing.T
	app  *app.App
	root *views.Root
	tt   *ui.Tester
	q    chan func()
}

func newPageEnv(t *testing.T, extra ...codei18n.Option) *pageEnv {
	t.Helper()
	e := &pageEnv{t: t, q: make(chan func(), 100000)}
	opts := append([]codei18n.Option{
		codei18n.WithGlobalDir(t.TempDir()), codei18n.WithSecretStore(secrets.NewMemory()),
		codei18n.WithEnv(func(string) string { return "" }),
	}, extra...)
	e.app = app.New(app.Deps{Core: app.NewCore(opts...), Update: func(fn func()) { e.q <- fn }, DataDir: t.TempDir()})
	e.root = &views.Root{App: e.app, Shell: noopShell{}}
	e.tt = ui.NewTester(e.root.View, 1280, 860)
	return e
}

// until 执行投递的函数并绘制帧，直到 cond 成立（最多 30 秒）。
func (e *pageEnv) until(what string, cond func() bool) {
	e.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		e.pump()
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("超时等待: %s; 文本 %q", what, e.tt.Texts())
		}
		select {
		case fn := <-e.q:
			fn()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// pump 执行当前已投递的函数，然后绘制一帧。
func (e *pageEnv) pump() {
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

func (e *pageEnv) click(s string) {
	e.t.Helper()
	if err := e.tt.Click(s); err != nil {
		e.t.Fatalf("点击 %q: %v; 文本 %q", s, err, e.tt.Texts())
	}
}

func (e *pageEnv) mustHave(s string) {
	e.t.Helper()
	if !e.tt.HasText(s) {
		e.t.Fatalf("应显示 %q; 文本 %q", s, e.tt.Texts())
	}
}

func project(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < n; i++ {
		src := fmt.Sprintf("package f\n\n// Comment number %d explains the code\nfunc F%d() {}\n", i, i)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%02d.go", i)), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func (e *pageEnv) entry(path string) *app.ProjectEntry {
	for _, p := range e.app.Projects {
		if p.Path == path {
			return p
		}
	}
	e.t.Fatalf("项目不在工作区: %s", path)
	return nil
}

// initProject 加入项目、走初始化向导（应用层）、把 provider 设为 mock，并等待首次扫描完成。
func (e *pageEnv) initProject(path string) *app.ProjectEntry {
	e.t.Helper()
	e.app.AddPaths([]string{path})
	p := e.entry(path)
	e.until("快速载入", func() bool { return p.Status != nil || p.Err != "" })
	e.app.OpenWizard(path)
	e.app.SubmitWizard()
	if e.app.Wizard != nil {
		e.t.Fatalf("向导提交失败: %s", e.app.Wizard.Err)
	}
	if err := p.Project().UpdateConfig(func(c *config.File) error { c.TranslationProvider = config.Ptr("mock"); return nil }); err != nil {
		e.t.Fatal(err)
	}
	e.until("首次扫描完成", func() bool { return len(e.app.ActiveJobs()) == 0 && p.Status != nil && p.Status.HasIndex })
	return p
}

// blocking 每批翻译都等待放行信号，用于测试取消。
type blocking struct {
	release chan struct{}
	started atomic.Int32
}

func (b *blocking) Translate(ctx context.Context, text, from, to string) (string, error) {
	r, err := b.TranslateBatch(ctx, []string{text}, from, to)
	if err != nil {
		return "", err
	}
	return r[0], nil
}

func (b *blocking) TranslateBatch(ctx context.Context, texts []string, from, to string) ([]string, error) {
	b.started.Add(1)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.release:
	}
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i] = "[T] " + t
	}
	return out, nil
}

func TestOverviewCountsAndTranslate(t *testing.T) {
	e := newPageEnv(t)
	path := project(t, 5)
	p := e.initProject(path)
	e.app.Select(path)
	e.pump()
	e.mustHave(locales.OvStatComments())
	e.mustHave(locales.OvStatPending())
	e.mustHave(locales.OvLastResult())
	e.mustHave(locales.OvScanResult(5, 5, 0))
	e.mustHave(locales.StatePending())
	if p.Status.Comments != 5 || p.Status.Pending != 5 {
		t.Fatalf("%+v", p.Status)
	}
	e.mustHave("5")

	e.click(locales.OvTranslate())
	if len(e.app.Jobs) != 2 || e.app.Jobs[1].Kind != app.JobTranslate {
		t.Fatalf("点击翻译应加入翻译任务: %+v", e.app.Jobs)
	}
	job := e.app.Jobs[1]
	e.until("翻译完成", func() bool { return job.State == app.JobDone && p.Status.Pending == 0 })
	e.pump()
	e.mustHave(locales.OvTranslateResult(5, 0, 5))
	e.mustHave(locales.StateReady())

	// 移除需要确认（AlertDialog）
	e.click(locales.OvRemove())
	e.mustHave(locales.OvRemoveTitle(p.Name))
	e.click(locales.OvRemoveConfirm())
	if len(e.app.Projects) != 0 {
		t.Fatal("确认后应从工作区移除")
	}
}

func TestOverviewCancelRunningJob(t *testing.T) {
	bl := &blocking{release: make(chan struct{})}
	defer close(bl.release)
	e := newPageEnv(t, codei18n.WithTranslator(bl))
	path := project(t, 3)
	e.initProject(path)
	e.app.Select(path)
	e.pump()
	e.click(locales.OvTranslate())
	job := e.app.Jobs[len(e.app.Jobs)-1]
	e.until("翻译开始", func() bool { return job.State == app.JobRunning && bl.started.Load() > 0 })
	e.mustHave(locales.OvProgress())
	e.mustHave(locales.StateTranslating())
	e.click(locales.OvCancelJob())
	e.until("取消完成", func() bool { return job.State == app.JobCanceled })
	e.pump()
	e.mustHave(locales.OvJobCanceled(locales.TaskKindTranslate()))
	if e.tt.HasText(locales.OvCancelJob()) {
		t.Fatal("任务结束后不应再显示取消按钮")
	}
}

func TestTasksBatchTwoProjects(t *testing.T) {
	e := newPageEnv(t)
	p1, p2 := project(t, 2), project(t, 3)
	e1, e2 := e.initProject(p1), e.initProject(p2)
	e.app.Page = app.PageTasks
	e.pump()
	e.mustHave(locales.TaskTitle())
	e.click(locales.TaskSelectProject(e1.Name))
	e.click(locales.TaskSelectProject(e2.Name))
	if !e1.Checked || !e2.Checked {
		t.Fatal("勾选框应绑定 ProjectEntry.Checked")
	}
	before := len(e.app.Jobs)
	e.click(locales.TaskTranslateSelected())
	jobs := e.app.Jobs[before:]
	if len(jobs) != 2 {
		t.Fatalf("应为两个项目各加入一个翻译任务: %d", len(jobs))
	}
	e.until("批量完成", func() bool { return jobs[0].State == app.JobDone && jobs[1].State == app.JobDone })
	e.pump()
	e.mustHave(locales.OvTranslateResult(2, 0, 2))
	e.mustHave(locales.OvTranslateResult(3, 0, 3))
	e.mustHave(locales.TaskSummary(0, len(e.app.Jobs)))
	e.click(locales.TaskClearFinished())
	e.pump()
	if len(e.app.Jobs) != 0 {
		t.Fatalf("清除已结束任务: %d", len(e.app.Jobs))
	}
	e.mustHave(locales.TaskNoJobs())
}

func TestWizardInitializesProject(t *testing.T) {
	e := newPageEnv(t)
	path := project(t, 2)
	e.app.AddPaths([]string{path})
	p := e.entry(path)
	e.until("快速载入", func() bool { return p.Status != nil })
	e.mustHave(locales.OvNotInitTitle())
	e.click(locales.OvInitialize())
	if e.app.Wizard == nil {
		t.Fatal("应打开向导")
	}
	e.mustHave(locales.WizTitle())
	e.mustHave(locales.WizNotGit())
	if !strings.Contains(e.app.Wizard.Excludes, "node_modules/**") {
		t.Fatal("排除规则应预填")
	}
	e.click(locales.WizInitialize())
	if e.app.Wizard != nil {
		t.Fatalf("提交后应关闭向导: %s", e.app.Wizard.Err)
	}
	e.until("初始化并扫描", func() bool {
		return len(e.app.ActiveJobs()) == 0 && p.Status != nil && p.Status.Initialized && p.Status.Comments == 2
	})
	e.pump()
	e.mustHave(locales.OvStatComments())
	e.mustHave(locales.OvScan())
}

func TestBrowserFilterSelectEditSave(t *testing.T) {
	e := newPageEnv(t)
	path := project(t, 6)
	p := e.initProject(path)
	e.app.Select(path)
	e.pump()
	e.click(locales.OvBrowse())
	if e.app.Page != app.PageBrowser || e.app.Browser == nil {
		t.Fatal("应打开注释浏览器")
	}
	b := e.app.Browser
	e.until("载入注释", func() bool { return !b.Loading && len(b.Records) == 6 })
	e.mustHave(locales.BrShown(6, 6))
	e.mustHave(locales.BrSummary(6, 0, 0))
	e.mustHave(locales.BrNoSelection())

	// 全文搜索缩小列表
	e.click(locales.BrSearch())
	e.tt.Type("number 3")
	e.pump()
	if len(b.Filtered()) != 1 {
		t.Fatalf("搜索应只剩 1 条: %d", len(b.Filtered()))
	}
	e.mustHave(locales.BrShown(1, 6))
	e.mustHave("f03.go:3")
	if e.tt.HasText("f01.go:3") {
		t.Fatal("不匹配的行不应显示")
	}

	// 三态筛选：全部未翻译，选"已翻译"后为空
	e.click(locales.BrTranslatedYes())
	e.mustHave(locales.BrNoResults())
	e.click(locales.BrTriAll())
	e.mustHave(locales.BrShown(1, 6))

	// 原语言下拉（值与显示文本不同）：可选值来自 FacetValues
	e.click(locales.BrAllLangs())
	if m := e.tt.Menu(); len(m) == 0 || m[0] != locales.BrAllLangs() {
		t.Fatalf("菜单: %q", m)
	}
	if err := e.tt.ChooseMenuItem("en"); err != nil {
		t.Fatal(err)
	}
	e.pump()
	if b.Filter.Lang != "en" || len(b.Filtered()) != 1 {
		t.Fatalf("按原语言筛选: %q %d", b.Filter.Lang, len(b.Filtered()))
	}

	// 选中行填入编辑框，输入并保存写回 mappings
	e.click("f03.go:3")
	if b.Current() == nil || !strings.Contains(b.Current().Original, "number 3") {
		t.Fatalf("应选中第 3 条: %+v", b.Current())
	}
	e.mustHave(locales.BrOriginal())
	e.click(locales.BrTranslationLabel())
	e.tt.Type("手动译文")
	if b.Edit != "手动译文" {
		t.Fatalf("编辑框应绑定 Browser.Edit: %q", b.Edit)
	}
	e.click(locales.BrSave())
	if b.Err != "" {
		t.Fatal(b.Err)
	}
	recs, err := p.Project().Comments()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range recs {
		if r.Translation("zh-CN") == "手动译文" && strings.Contains(r.Original, "number 3") {
			found = true
		}
	}
	if !found {
		t.Fatal("保存应把译文写回 mappings")
	}
	e.pump()
	e.mustHave(locales.BrSummary(6, 1, 17))
	e.mustHave(locales.BrMarkTranslated())
}

// 2 万条记录：列表只构建可见行，筛选结果被记忆，帧时间与记录数基本无关。
func TestBrowserLargeListIsVirtualized(t *testing.T) {
	e := newPageEnv(t)
	path := project(t, 0)
	e.app.AddPaths([]string{path})
	e.until("快速载入", func() bool { return e.entry(path).Status != nil })
	const n = 20000
	recs := make([]codei18n.CommentRecord, n)
	for i := range recs {
		recs[i] = codei18n.CommentRecord{
			ID: fmt.Sprintf("id%05d", i), File: fmt.Sprintf("file%05d.go", i), Language: "go", Lang: "en",
			Original: fmt.Sprintf("// comment %d", i), Texts: map[string]string{"en": fmt.Sprintf("// comment %d", i)},
		}
	}
	// 直接放入记录（不经后台载入），Project 与选中项目一致，页面不会重新打开
	e.app.Browser = &app.Browser{Project: path, Display: "zh-CN", Records: recs, Selected: -1}
	e.app.Page = app.PageBrowser
	e.pump()
	e.mustHave(locales.BrShown(n, n))
	e.mustHave("file00000.go:0")
	if e.tt.HasText("file19999.go:0") {
		t.Fatal("不可见的行不应被构建")
	}
	if got := len(e.tt.Texts()); got > 600 {
		t.Fatalf("只应构建可见行，文本数 %d", got)
	}
	start := time.Now()
	for i := 0; i < 10; i++ {
		e.tt.Frame()
	}
	per := time.Since(start) / 10
	t.Logf("2 万条记录每帧 %v", per)
	if per > time.Second {
		t.Fatalf("帧太慢: %v", per)
	}
	e.click(locales.BrSearch())
	e.tt.Type("file1999")
	e.pump()
	e.mustHave(locales.BrShown(10, n))
	e.click("file19999.go:0")
	if b := e.app.Browser; b.Current() == nil || b.Current().ID != "id19999" {
		t.Fatalf("应选中 id19999: %+v", b.Current())
	}
}
