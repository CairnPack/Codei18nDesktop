package app

import (
	"strings"

	"github.com/CairnPack/codei18n"
)

// DefaultExcludes 是向导中预填的排除规则（常见的依赖与构建目录）。
var DefaultExcludes = []string{
	".git/**", "vendor/**", ".codei18n/**", "node_modules/**", "build/**", ".gradle/**",
	"bin/**", "obj/**", ".dart_tool/**", "dist/**", "target/**", ".idea/**",
}

// Wizard 是未初始化项目的初始化向导。字段直接绑定到视图控件。
type Wizard struct {
	Project        string
	SourceLanguage string
	LocalLanguage  string
	// Excludes 每行一条规则。
	Excludes string
	// Profile 选用的 provider 配置档；空表示不选用（使用全局默认）。
	Profile      string
	Profiles     []string
	InstallHooks bool
	Gitignore    bool
	IsGitRepo    bool
	// ScanAfter 初始化后立即加入一次扫描任务。
	ScanAfter bool
	Err       string
}

// OpenWizard 为项目打开初始化向导。
func (a *App) OpenWizard(path string) {
	e := a.find(path)
	if e == nil {
		return
	}
	w := &Wizard{
		Project: path, SourceLanguage: "en", LocalLanguage: "zh-CN",
		Excludes: strings.Join(DefaultExcludes, "\n"), Gitignore: true, ScanAfter: true,
	}
	if g, err := a.deps.Core.Global(); err == nil {
		if names, err := g.Profiles(); err == nil {
			w.Profiles = names
		}
		if f, err := g.File(); err == nil && f.Profile != nil {
			w.Profile = *f.Profile
		}
	}
	if e.project != nil {
		w.IsGitRepo = e.project.IsGitRepo()
		w.InstallHooks = w.IsGitRepo
	}
	a.Wizard = w
}

// CancelWizard 关闭向导。
func (a *App) CancelWizard() { a.Wizard = nil }

// SubmitWizard 写入项目配置并按选择安装 hook、更新 .gitignore、加入首次扫描。成功时关闭向导。
func (a *App) SubmitWizard() {
	w := a.Wizard
	if w == nil {
		return
	}
	e := a.find(w.Project)
	if e == nil {
		a.Wizard = nil
		return
	}
	p, err := a.ensureProject(e)
	if err != nil {
		w.Err = errorText(err)
		return
	}
	var excludes []string
	for _, line := range strings.Split(w.Excludes, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			excludes = append(excludes, line)
		}
	}
	opts := codei18n.InitOptions{
		SourceLanguage:  nonEmpty(w.SourceLanguage),
		LocalLanguage:   nonEmpty(w.LocalLanguage),
		ExcludePatterns: excludes,
		Profile:         nonEmpty(w.Profile),
	}
	if err := p.Init(opts); err != nil {
		w.Err = errorText(err)
		return
	}
	if w.IsGitRepo && w.Gitignore {
		if _, err := p.EnsureGitignore(); err != nil {
			a.notify(errorText(err))
		}
	}
	if w.IsGitRepo && w.InstallHooks {
		if err := p.InstallHooks(); err != nil {
			a.notify(errorText(err))
		}
	}
	scan := w.ScanAfter
	a.Wizard = nil
	if scan {
		a.Enqueue(e.Path, JobScan)
	} else {
		a.quickLoad(e)
	}
}

// ensureProject 返回已打开的项目，必要时同步打开（打开项目只做路径检查，开销很小）。
func (a *App) ensureProject(e *ProjectEntry) (Project, error) {
	if e.project != nil {
		return e.project, nil
	}
	p, err := a.deps.Core.Open(e.Path)
	if err != nil {
		return nil, err
	}
	e.project = p
	return p, nil
}

func nonEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}
