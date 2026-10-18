package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CairnPack/codei18n"
)

// ProjectState 是侧栏显示的项目状态。
type ProjectState string

const (
	StateLoading        ProjectState = "loading"        // 正在快速载入
	StateNotInitialized ProjectState = "notInitialized" // 未初始化
	StateReady          ProjectState = "ready"          // 已初始化，没有待翻译条目
	StatePending        ProjectState = "pending"        // 有待翻译条目
	StateScanning       ProjectState = "scanning"       // 扫描中
	StateTranslating    ProjectState = "translating"    // 翻译中
	StateError          ProjectState = "error"          // 出错
)

// ProjectEntry 是工作区中的一个项目。
type ProjectEntry struct {
	Path   string
	Name   string
	Status *codei18n.Status // 快速载入结果；nil 表示尚未载入
	Err    string           // 最近一次载入或任务的错误（已脱敏）
	// Checked 表示在批量操作中被勾选。
	Checked bool

	project Project
	running *Job // 正在运行的任务
}

// State 返回项目当前状态。
func (p *ProjectEntry) State() ProjectState {
	switch {
	case p.running != nil && p.running.Kind == JobTranslate:
		return StateTranslating
	case p.running != nil:
		return StateScanning
	case p.Err != "":
		return StateError
	case p.Status == nil:
		return StateLoading
	case !p.Status.Initialized:
		return StateNotInitialized
	case p.Status.Pending > 0:
		return StatePending
	default:
		return StateReady
	}
}

// Running 返回正在运行的任务（可能为 nil）。
func (p *ProjectEntry) Running() *Job { return p.running }

// Project 返回已打开的 core 项目。
func (p *ProjectEntry) Project() Project { return p.project }

// recentFile 是最近项目列表与 UI 设置的持久化格式。
type recentFile struct {
	Version  int        `json:"version"`
	Projects []string   `json:"projects"`
	Settings UISettings `json:"settings"`
}

// UISettings 是桌面端自己的设置（与项目配置无关）。
type UISettings struct {
	// Locale 是 UI 语言（BCP 47），空表示跟随系统。注意这与项目的翻译目标语言是两个独立的设置。
	Locale string `json:"locale"`
	// Watch 开启后项目文件变化时在后台做增量扫描（不自动翻译）。
	Watch bool `json:"watch"`
	// MaxParallel 是多项目批量任务的全局并发上限（1 表示顺序执行）。
	MaxParallel int `json:"maxParallel"`
	// Concurrency 是单个项目翻译的并发批次数（0 表示使用项目配置）。
	Concurrency int `json:"concurrency"`
}

func defaultSettings() UISettings { return UISettings{MaxParallel: 1} }

const recentFileName = "workspace.json"

func (a *App) recentPath() string { return filepath.Join(a.deps.DataDir, recentFileName) }

// load 读取最近项目列表与设置；文件损坏时使用默认值。
func (a *App) load() []string {
	a.Settings = defaultSettings()
	data, err := os.ReadFile(a.recentPath())
	if err != nil {
		return nil
	}
	var f recentFile
	if json.Unmarshal(data, &f) != nil {
		return nil
	}
	a.Settings = f.Settings
	if a.Settings.MaxParallel <= 0 {
		a.Settings.MaxParallel = 1
	}
	return f.Projects
}

// save 原子写出最近项目列表与设置。
func (a *App) save() {
	f := recentFile{Version: 1, Settings: a.Settings}
	for _, p := range a.Projects {
		f.Projects = append(f.Projects, p.Path)
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(a.deps.DataDir, 0o755); err != nil {
		a.notify(err.Error())
		return
	}
	tmp := a.recentPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err == nil {
		if err := os.Rename(tmp, a.recentPath()); err != nil {
			a.notify(err.Error())
		}
	}
}

// AddPaths 把目录加入工作区（来自目录对话框或拖放）。不是目录的路径被忽略，已存在的项目不重复添加。
// 返回新加入的项目数。
func (a *App) AddPaths(paths []string) int {
	added := 0
	for _, raw := range paths {
		abs, err := filepath.Abs(raw)
		if err != nil {
			continue
		}
		st, err := os.Stat(abs)
		if err != nil || !st.IsDir() {
			continue
		}
		if a.find(abs) != nil {
			continue
		}
		entry := &ProjectEntry{Path: abs, Name: filepath.Base(abs)}
		a.Projects = append(a.Projects, entry)
		a.quickLoad(entry)
		added++
		if a.Selected == "" {
			a.Selected = abs
		}
	}
	if added > 0 {
		a.sortProjects()
		a.save()
	}
	return added
}

// RemoveProject 从工作区移除项目（不删除任何文件）。运行中的任务会被取消。
func (a *App) RemoveProject(path string) {
	for i, p := range a.Projects {
		if p.Path != path {
			continue
		}
		if p.running != nil {
			a.Cancel(p.running.ID)
		}
		a.stopWatch(p.Path)
		a.Projects = append(a.Projects[:i], a.Projects[i+1:]...)
		if a.Selected == path {
			a.Selected = ""
			if len(a.Projects) > 0 {
				a.Selected = a.Projects[0].Path
			}
		}
		a.save()
		return
	}
}

func (a *App) find(path string) *ProjectEntry {
	for _, p := range a.Projects {
		if strings.EqualFold(p.Path, path) {
			return p
		}
	}
	return nil
}

// SelectedProject 返回当前选中的项目（可能为 nil）。
func (a *App) SelectedProject() *ProjectEntry { return a.find(a.Selected) }

func (a *App) sortProjects() {
	sort.SliceStable(a.Projects, func(i, j int) bool {
		return strings.ToLower(a.Projects[i].Name) < strings.ToLower(a.Projects[j].Name)
	})
}

// quickLoad 在后台打开项目并读取状态（只读 mappings、index 与缓存，不触发扫描）。
func (a *App) quickLoad(entry *ProjectEntry) {
	path := entry.Path
	go func() {
		p, err := a.deps.Core.Open(path)
		var st *codei18n.Status
		if err == nil {
			st, err = p.Status()
		}
		a.deps.Update(func() {
			e := a.find(path)
			if e == nil {
				return
			}
			if p != nil {
				e.project = p
			}
			if err != nil {
				e.Err = err.Error()
				return
			}
			e.Status, e.Err = st, ""
		})
	}()
}

// Refresh 重新快速载入一个项目的状态。
func (a *App) Refresh(path string) {
	if e := a.find(path); e != nil && e.running == nil {
		a.quickLoad(e)
	}
}

// Summary 返回状态栏用的汇总文本数据：项目数、运行中任务数。
func (a *App) Summary() (projects, running int) {
	for _, p := range a.Projects {
		if p.running != nil {
			running++
		}
	}
	return len(a.Projects), running
}

// notify 记录一条给用户看的消息（视图层以 toast 显示后清空）。
func (a *App) notify(msg string) {
	a.Notices = append(a.Notices, Notice{Text: msg, At: time.Now()})
}

// Notice 是一条待显示的提示。
type Notice struct {
	Text string
	At   time.Time
}

// TakeNotices 取出并清空待显示的提示（视图每帧调用）。
func (a *App) TakeNotices() []Notice {
	n := a.Notices
	a.Notices = nil
	return n
}

// errorText 把错误转成显示文本；core 返回的错误已经脱敏。
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprint(err)
}
