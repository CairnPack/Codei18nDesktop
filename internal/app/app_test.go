package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/secrets"
)

// harness 模拟主线程：后台 goroutine 通过 Update 投递的函数在测试 goroutine 中按顺序执行。
type harness struct {
	t      *testing.T
	app    *App
	q      chan func()
	global string
	store  *secrets.Memory
	data   string
	extra  []codei18n.Option
	watch  *fakeWatcher
}

func newHarness(t *testing.T, extra ...codei18n.Option) *harness {
	h := &harness{t: t, q: make(chan func(), 100000), global: t.TempDir(), store: secrets.NewMemory(), data: t.TempDir(), extra: extra, watch: &fakeWatcher{}}
	h.app = New(h.deps())
	return h
}

func (h *harness) deps() Deps {
	opts := append([]codei18n.Option{
		codei18n.WithGlobalDir(h.global), codei18n.WithSecretStore(h.store),
		codei18n.WithEnv(func(string) string { return "" }),
	}, h.extra...)
	return Deps{Core: NewCore(opts...), Update: func(fn func()) { h.q <- fn }, DataDir: h.data, Watcher: h.watch}
}

// until 执行投递的函数直到 cond 成立（最多 30 秒）。
func (h *harness) until(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.After(30 * time.Second)
	for !cond() {
		select {
		case fn := <-h.q:
			fn()
		case <-time.After(10 * time.Millisecond): // 后台没有投递时也要重新检查条件
		case <-deadline:
			h.t.Fatalf("超时等待: %s", what)
		}
	}
}

// drain 执行当前已投递的所有函数。
func (h *harness) drain() {
	for {
		select {
		case fn := <-h.q:
			fn()
		case <-time.After(50 * time.Millisecond):
			return
		}
	}
}

func project(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func goFiles(n int) map[string]string {
	files := map[string]string{}
	for i := 0; i < n; i++ {
		files[fmt.Sprintf("f%02d.go", i)] = fmt.Sprintf("package f\n\n// Comment number %d explains the code\nfunc F%d() {}\n", i, i)
	}
	return files
}

// initProject 用 mock provider 初始化项目（向导流程）并等待首次扫描完成。
func (h *harness) initProject(path string) *ProjectEntry {
	h.t.Helper()
	h.app.AddPaths([]string{path})
	e := h.app.find(path)
	h.until("快速载入", func() bool { return e.Status != nil || e.Err != "" })
	h.app.OpenWizard(path)
	if h.app.Wizard == nil {
		h.t.Fatal("向导未打开")
	}
	h.app.SubmitWizard()
	if h.app.Wizard != nil {
		h.t.Fatalf("向导提交失败: %s", h.app.Wizard.Err)
	}
	p, _ := h.app.ensureProject(e)
	p.UpdateConfig(func(c *config.File) error { c.TranslationProvider = config.Ptr("mock"); return nil })
	h.until("首次扫描完成", func() bool { return len(h.app.ActiveJobs()) == 0 && e.Status != nil && e.Status.HasIndex })
	return e
}

func TestWorkspaceAddRemovePersist(t *testing.T) {
	h := newHarness(t)
	a, b := project(t, nil), project(t, nil)
	file := filepath.Join(a, "file.txt")
	os.WriteFile(file, []byte("x"), 0o644)
	if n := h.app.AddPaths([]string{a, b, a, file, filepath.Join(a, "missing")}); n != 2 {
		t.Fatalf("应只加入 2 个目录: %d", n)
	}
	h.until("快速载入", func() bool { return h.app.Projects[0].Status != nil && h.app.Projects[1].Status != nil })
	if h.app.Projects[0].State() != StateNotInitialized {
		t.Fatalf("%s", h.app.Projects[0].State())
	}
	// 最近项目列表持久化在用户配置目录
	again := New(h.deps())
	if len(again.Projects) != 2 {
		t.Fatalf("重启后应恢复 2 个项目: %d", len(again.Projects))
	}
	h.app.RemoveProject(a)
	if len(New(h.deps()).Projects) != 1 {
		t.Fatal("移除应持久化")
	}
}

func TestWizardScanTranslateAndBrowse(t *testing.T) {
	h := newHarness(t)
	path := project(t, goFiles(5))
	e := h.initProject(path)
	if e.State() != StatePending || e.Status.Comments != 5 || e.Status.Pending != 5 {
		t.Fatalf("扫描后应有 5 条待翻译: %s %+v", e.State(), e.Status)
	}
	cfg, _ := e.project.Config()
	if !strings.Contains(strings.Join(cfg.ExcludePatterns.Value, ","), "node_modules/**") {
		t.Fatal("向导应写入预填的排除规则")
	}
	h.app.Enqueue(path, JobTranslate)
	h.until("翻译完成", func() bool { return len(h.app.ActiveJobs()) == 0 && e.Status.Pending == 0 })
	if e.State() != StateReady {
		t.Fatalf("%s", e.State())
	}

	// 注释浏览器：统计、筛选、搜索、编辑、纠正原语言
	h.app.Select(path)
	h.app.OpenBrowser()
	b := h.app.Browser
	h.until("载入注释", func() bool { return !b.Loading })
	if st := b.Stats(); st.Total != 5 || st.Translated != 5 || st.Coverage() != 1 {
		t.Fatalf("%+v", st)
	}
	b.Filter.Query = "number 3"
	if f := b.Filtered(); len(f) != 1 {
		t.Fatalf("全文搜索: %v", f)
	}
	b.Filter = BrowserFilter{Translated: TriNo}
	if len(b.Filtered()) != 0 {
		t.Fatal("全部已翻译")
	}
	b.Filter = BrowserFilter{}
	b.Select(b.Filtered()[0])
	if !strings.Contains(b.Edit, "MOCK") {
		t.Fatalf("编辑框应载入译文: %q", b.Edit)
	}
	b.Edit = "// 手动译文"
	b.SaveEdit()
	if b.Err != "" {
		t.Fatal(b.Err)
	}
	recs, _ := e.project.Comments()
	found := false
	for _, r := range recs {
		if r.Translation("zh-CN") == "// 手动译文" {
			found = true
		}
	}
	if !found {
		t.Fatal("手动译文应写回 mappings")
	}
	id := b.Current().ID
	b.CorrectLanguage("ja")
	h.until("重新载入", func() bool { return !b.Loading })
	for _, r := range b.Records {
		if r.ID == id && (r.Lang != "ja" || r.LangSource != "override") {
			t.Fatalf("纠正应持久化: %+v", r)
		}
	}
}

// blocking 每批翻译都等待放行信号，用于测试并发与取消。
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

func TestBatchQueueSequentialParallelAndCancel(t *testing.T) {
	bl := &blocking{release: make(chan struct{})}
	h := newHarness(t, codei18n.WithTranslator(bl))
	p1, p2 := project(t, goFiles(3)), project(t, goFiles(3))
	e1, e2 := h.initProject(p1), h.initProject(p2)
	e1.Checked, e2.Checked = true, true

	// 顺序执行（并发上限 1）：同一时间只有一个项目在翻译
	jobs := h.app.EnqueueChecked(JobTranslate)
	if len(jobs) != 2 {
		t.Fatalf("%d", len(jobs))
	}
	h.until("第一个任务开始", func() bool { if jobs[0].State == JobFailed || jobs[0].State == JobDone { t.Fatalf("job0 %s err=%s res=%+v", jobs[0].State, jobs[0].Err, jobs[0].Translate) }; return bl.started.Load() == 1 })
	h.drain()
	if jobs[0].State != JobRunning || jobs[1].State != JobQueued {
		t.Fatalf("顺序执行: %s %s", jobs[0].State, jobs[1].State)
	}
	if e1.State() != StateTranslating {
		t.Fatalf("%s", e1.State())
	}
	// 取消排队中的任务：直接移出；取消运行中的任务：context 取消
	h.app.Cancel(jobs[1].ID)
	h.app.Cancel(jobs[0].ID)
	h.until("取消完成", func() bool { return len(h.app.ActiveJobs()) == 0 })
	if jobs[0].State != JobCanceled || jobs[1].State != JobCanceled {
		t.Fatalf("%s %s", jobs[0].State, jobs[1].State)
	}

	// 并行（上限 2）：两个项目同时翻译
	h.app.SetSettings(UISettings{MaxParallel: 2})
	bl.started.Store(0)
	jobs = h.app.EnqueueChecked(JobTranslate)
	h.until("两个任务同时开始", func() bool { return bl.started.Load() == 2 })
	h.drain()
	if jobs[0].State != JobRunning || jobs[1].State != JobRunning {
		t.Fatal("应并行运行")
	}
	close(bl.release)
	h.until("全部完成", func() bool { return len(h.app.ActiveJobs()) == 0 })
	if jobs[0].State != JobDone || jobs[0].Translate.SuccessCount != 3 {
		t.Fatalf("%+v %+v", jobs[0], jobs[0].Translate)
	}
	h.until("状态刷新", func() bool { return e1.Status.Pending == 0 && e2.Status.Pending == 0 })
	// 同一项目同类任务排队中时不重复加入
	h.app.SetSettings(UISettings{MaxParallel: 1})
}

func TestProjectSettingsOverrideAndInheritance(t *testing.T) {
	h := newHarness(t)
	os.WriteFile(filepath.Join(h.global, "config.json"),
		[]byte(`{"profile":"p","profiles":{"p":{"provider":"openai","model":"gpt-x","baseUrl":"https://api.example.com/v1"}}}`), 0o644)
	path := project(t, goFiles(1))
	h.initProject(path)
	h.app.Select(path)
	h.app.OpenProjectSettings()
	f := h.app.ProjectForm
	if f == nil || f.Err != "" {
		t.Fatalf("%+v", f)
	}
	if r := f.Row(FieldModel); r.Value != "gpt-x" || r.Source != config.LayerGlobal || r.Overridden() {
		t.Fatalf("%+v", r)
	}
	f.Row(FieldModel).Edit = "gpt-y"
	f.Override(FieldModel)
	if r := f.Row(FieldModel); r.Value != "gpt-y" || r.Source != config.LayerProject {
		t.Fatalf("%+v", r)
	}
	f.Row(FieldBatchSize).Edit = "abc"
	f.Override(FieldBatchSize)
	if f.Err == "" {
		t.Fatal("非法数字应报错")
	}
	f.Reset(FieldModel)
	if r := f.Row(FieldModel); r.Value != "gpt-x" || r.Source != config.LayerGlobal {
		t.Fatalf("恢复继承: %+v", r)
	}

	// 项目专用令牌：遮罩输入、保存后只显示末 4 位与来源；恢复继承时可一并删除
	tp := f.Tokens
	tp.Input = "sk-project-token-ABCD"
	tp.Save()
	if tp.Input != "" || !tp.Saved || tp.Last4 != "ABCD" || tp.Source != codei18n.TokenSourceProject {
		t.Fatalf("%+v", tp)
	}
	f.Row(FieldLanguagesProbe()).Edit = "ja"
	f.Override(FieldLocalLanguage)
	f.ResetAll(true)
	if tp.Saved || f.Row(FieldLocalLanguage).Source == config.LayerProject {
		t.Fatalf("ResetAll(true) 应删除项目令牌并恢复继承: %+v", tp)
	}
}

// FieldLanguagesProbe 让测试读起来清楚：编辑本地语言字段。
func FieldLanguagesProbe() FieldKey { return FieldLocalLanguage }

func TestGlobalSettingsProfilesAndTokens(t *testing.T) {
	h := newHarness(t)
	h.app.OpenGlobalSettings()
	g := h.app.GlobalForm
	g.NewProfile("deepseek")
	if g.Selected != "deepseek" || len(g.Profiles) != 1 {
		t.Fatalf("%+v", g)
	}
	g.Edit.BaseURL, g.Edit.Model, g.Edit.Concurrency = "https://api.deepseek.com", "deepseek-chat", "8"
	g.SaveSelected()
	if g.Err != "" {
		t.Fatal(g.Err)
	}
	g.Tokens.Input = "sk-global-token-WXYZ"
	g.Tokens.Save()
	if !g.Tokens.Saved || g.Tokens.Last4 != "WXYZ" {
		t.Fatalf("%+v", g.Tokens)
	}
	if s, err := h.store.Get("profile/deepseek"); err != nil || s.Last4() != "WXYZ" {
		t.Fatal("令牌应保存到凭据库（CLI 与 hook 共用）")
	}
	g.CopySelected()
	if g.Selected != "deepseek-copy" || g.Tokens.Saved {
		t.Fatal("复制不复制令牌")
	}
	g.SelectProfile("deepseek")
	g.Edit.Name = "ds"
	g.SaveSelected()
	if s, err := h.store.Get("profile/ds"); err != nil || s.Last4() != "WXYZ" {
		t.Fatal("重命名应迁移令牌")
	}
	g.Tokens.AskDelete()
	if !g.Tokens.ConfirmDelete {
		t.Fatal("删除需要二次确认")
	}
	g.Tokens.CancelDelete()
	g.DeleteSelected()
	if _, err := h.store.Get("profile/ds"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatal("删除配置档应删除令牌")
	}
}

type fakeWatcher struct {
	callbacks map[string]func()
}

func (w *fakeWatcher) Watch(root string, onChange func()) (func(), error) {
	if w.callbacks == nil {
		w.callbacks = map[string]func(){}
	}
	w.callbacks[root] = onChange
	return func() { delete(w.callbacks, root) }, nil
}

func TestWatchTriggersIncrementalScanOnly(t *testing.T) {
	h := newHarness(t)
	path := project(t, goFiles(2))
	e := h.initProject(path)
	h.app.SetSettings(UISettings{Watch: true, MaxParallel: 1})
	cb := h.watch.callbacks[path]
	if cb == nil {
		t.Fatal("应开始监视")
	}
	os.WriteFile(filepath.Join(path, "new.go"), []byte("package f\n\n// Brand new comment here\nfunc N() {}\n"), 0o644)
	cb()
	h.until("增量扫描完成", func() bool { return len(h.app.ActiveJobs()) == 0 && e.Status.Comments == 3 })
	for _, j := range h.app.Jobs {
		if j.Kind == JobTranslate {
			t.Fatal("文件变化不应自动翻译")
		}
	}
	h.app.SetSettings(UISettings{Watch: false, MaxParallel: 1})
	if len(h.watch.callbacks) != 0 {
		t.Fatal("关闭后应停止监视")
	}
}
