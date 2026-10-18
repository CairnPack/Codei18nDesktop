package app

// Page 是主区域显示的页面。
type Page string

const (
	PageOverview        Page = "overview"        // 项目概览：状态、操作、任务进度
	PageBrowser         Page = "browser"         // 注释浏览器
	PageProjectSettings Page = "projectSettings" // 项目设置
	PageGlobalSettings  Page = "globalSettings"  // 全局设置（provider 配置档、UI 语言、并发）
	PageTasks           Page = "tasks"           // 全部任务（多项目批量队列）
)

// App 是桌面端的全部应用状态。只在主线程访问（见包文档）。
type App struct {
	deps Deps

	Projects []*ProjectEntry
	Selected string
	Page     Page
	Settings UISettings
	Jobs     []*Job
	Notices  []Notice

	// Wizard 是当前打开的初始化向导（nil 表示未打开）。
	Wizard *Wizard
	// Browser 是注释浏览器的模型（按项目懒加载）。
	Browser *Browser
	// ProjectForm / GlobalForm 是设置页的模型（进入页面时创建）。
	ProjectForm *ProjectForm
	GlobalForm  *GlobalForm

	nextJob int
	watches map[string]func()
}

// New 创建 App，读取最近项目列表并在后台快速载入各项目状态。
func New(deps Deps) *App {
	a := &App{deps: deps, Page: PageOverview, watches: map[string]func(){}}
	paths := a.load()
	for _, p := range paths {
		entry := &ProjectEntry{Path: p, Name: baseName(p)}
		a.Projects = append(a.Projects, entry)
		a.quickLoad(entry)
	}
	a.sortProjects()
	if len(a.Projects) > 0 {
		a.Selected = a.Projects[0].Path
	}
	if a.Settings.Watch {
		a.applyWatch()
	}
	return a
}

// Select 选中项目并回到概览页。
func (a *App) Select(path string) {
	if a.find(path) == nil {
		return
	}
	a.Selected = path
	if a.Page == PageBrowser || a.Page == PageProjectSettings {
		a.Page = PageOverview
	}
	a.Browser, a.ProjectForm = nil, nil
}

// SetSettings 保存 UI 设置；并发上限变大时立即调度排队任务，监视开关变化时启停监视。
func (a *App) SetSettings(s UISettings) {
	if s.MaxParallel <= 0 {
		s.MaxParallel = 1
	}
	watchChanged := s.Watch != a.Settings.Watch
	a.Settings = s
	a.save()
	if watchChanged {
		a.applyWatch()
	}
	a.schedule()
}

func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}
