package views

import (
	"sort"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n-desktop/internal/app"
	"github.com/CairnPack/codei18n-desktop/locales"
)

// overviewPage 是选中项目的概览：状态、统计、操作、运行中任务的进度与上次任务结果。
func overviewPage(c *ui.Context, r *Root) {
	a := r.App
	p := a.SelectedProject()
	if p == nil {
		return
	}
	t := c.Theme()
	overviewHeader(c, p)
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Gap(14).Padding(0, 4, 4, 0).Children(func() {
			if p.Err != "" {
				ui.Text(c, locales.OvProjectError(p.Err)).TextColor(t.Danger).Selectable()
			}
			switch {
			case p.Status == nil:
				ui.Row(c).Gap(8).Children(func() {
					ui.Spinner(c)
					ui.Text(c, locales.OvLoading()).TextColor(t.TextMuted)
				})
			case !p.Status.Initialized:
				notInitialized(c, r, p)
			default:
				overviewStats(c, p)
				overviewActions(c, r, p)
			}
			overviewJobs(c, r, p)
		})
	})
	path := p.Path
	switch ui.AlertDialog(c, &r.ui.confirmRemove, locales.OvRemoveTitle(p.Name), locales.OvRemoveMessage(),
		locales.Cancel(), locales.OvRemoveConfirm()) {
	case 1:
		a.RemoveProject(path)
	}
}

// overviewHeader 显示项目名、路径与状态徽标。
func overviewHeader(c *ui.Context, p *app.ProjectEntry) {
	t := c.Theme()
	ui.Row(c).Gap(12).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, p.Name).FontSize(22).Bold().SingleLine()
			// 项目路径是数据，可选中复制
			ui.Text(c, p.Path).TextColor(t.TextMuted).FontSize(12).Selectable()
		})
		stateBadge(c, p.State())
	})
}

// notInitialized 解释未初始化状态并提供初始化入口。
func notInitialized(c *ui.Context, r *Root, p *app.ProjectEntry) {
	t := c.Theme()
	ui.Column(c).Gap(10).Padding(16).Border(1, t.Border).Radius(8).Children(func() {
		ui.Text(c, locales.OvNotInitTitle()).FontSize(17).Bold()
		ui.Text(c, locales.OvNotInitHint()).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, locales.OvInitialize()).Clicked() {
				r.App.OpenWizard(p.Path)
			}
			if ui.Button(c, locales.OvRemove()).Clicked() {
				r.ui.confirmRemove = true
			}
		})
	})
}

// overviewStats 显示注释数、已翻译、待翻译、失败与原语言分布。
func overviewStats(c *ui.Context, p *app.ProjectEntry) {
	st := p.Status
	t := c.Theme()
	ui.Row(c).Gap(10).Children(func() {
		statCard(c, locales.OvStatComments(), st.Comments, t.Text)
		statCard(c, locales.OvStatTranslated(), st.Translated, t.Success)
		statCard(c, locales.OvStatPending(), st.Pending, t.Warning)
		failed := t.Text
		if st.Failed > 0 {
			failed = t.Danger
		}
		statCard(c, locales.OvStatFailed(), st.Failed, failed)
	})
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, locales.OvLanguages()).Bold()
		if len(st.Languages) == 0 {
			ui.Text(c, locales.OvNoLanguages()).TextColor(t.TextMuted)
			return
		}
		// 语言分布很小（几种语言），每帧排序没有开销问题
		langs := make([]string, 0, len(st.Languages))
		for l := range st.Languages {
			langs = append(langs, l)
		}
		sort.Slice(langs, func(i, j int) bool {
			if st.Languages[langs[i]] != st.Languages[langs[j]] {
				return st.Languages[langs[i]] > st.Languages[langs[j]]
			}
			return langs[i] < langs[j]
		})
		ui.Row(c).Gap(6).Children(func() {
			for _, l := range langs {
				name := l
				if name == "" {
					name = locales.BrLangUnknown()
				}
				ui.Badge(c, locales.OvLangCount(name, st.Languages[l]))
			}
		})
	})
}

// statCard 是一个数字统计卡片。
func statCard(c *ui.Context, label string, n int, color ui.Color) {
	t := c.Theme()
	ui.Column(c).Grow(1).Gap(2).Padding(10, 14).Border(1, t.Border).Radius(8).Children(func() {
		ui.Text(c, strconv.Itoa(n)).FontSize(24).Bold().TextColor(color)
		ui.Text(c, label).TextColor(t.TextMuted).FontSize(12)
	})
}

// overviewActions 是项目操作按钮与并发设置。
func overviewActions(c *ui.Context, r *Root, p *app.ProjectEntry) {
	a := r.App
	t := c.Theme()
	st := p.Status
	path := p.Path
	orphans := 0
	if j := lastFinished(a, path, func(j *app.Job) bool { return j.Map != nil }); j != nil && j.Kind == app.JobScan {
		orphans = j.Map.Orphans
	}
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, locales.OvActions()).Bold()
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, locales.OvScan()).Clicked() {
				a.Enqueue(path, app.JobScan)
			}
			if ui.Button(c, locales.OvTranslate()).Clicked() {
				a.Enqueue(path, app.JobTranslate)
			}
			prune := locales.OvPrune()
			if orphans > 0 {
				prune = locales.OvPruneCount(orphans)
			}
			if ui.Button(c, prune).Clicked() {
				a.Enqueue(path, app.JobPrune)
			}
			if ui.Button(c, locales.OvRetryFailed()).Disabled(st.Failed == 0).Clicked() {
				a.Enqueue(path, app.JobRetryFailed)
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			// 打开页面会切换 a.Page，放到构建结束后执行
			ui.Button(c, locales.OvBrowse()).OnClick(a.OpenBrowser)
			ui.Button(c, locales.OvSettings()).OnClick(a.OpenProjectSettings)
			ui.Spacer(c)
			if ui.Button(c, locales.OvRemove()).Clicked() {
				r.ui.confirmRemove = true
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, locales.OvConcurrency())
			// 局部副本 + Changed：控件在构造函数返回前写回，变化时立即保存设置
			v := float64(a.Settings.Concurrency)
			if ui.NumberInput(c, &v, 0, 64, 1).Label(locales.OvConcurrency()).Width(170).Changed() {
				s := a.Settings
				s.Concurrency = int(v)
				a.SetSettings(s)
			}
			ui.Text(c, locales.OvConcurrencyHint()).TextColor(t.TextMuted).FontSize(12)
		})
	})
}

// overviewJobs 显示本项目运行中任务的实时进度、排队数与上次结束任务的结果。
func overviewJobs(c *ui.Context, r *Root, p *app.ProjectEntry) {
	a := r.App
	t := c.Theme()
	queued := 0
	for _, j := range a.Jobs {
		if j.Project == p.Path && j.State == app.JobQueued {
			queued++
		}
	}
	if j := p.Running(); j != nil || queued > 0 {
		ui.Column(c).Gap(6).Padding(12).Border(1, t.Border).Radius(8).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, locales.OvProgress()).Bold()
				if j != nil {
					ui.Badge(c, jobKindLabel(j.Kind))
				}
				ui.Spacer(c)
				if j != nil {
					id := j.ID
					ui.Button(c, locales.OvCancelJob()).OnClick(func() { a.Cancel(id) })
				}
			})
			if j != nil {
				jobProgress(c, j)
			}
			if queued > 0 {
				ui.Text(c, locales.OvQueued(queued)).TextColor(t.TextMuted)
			}
		})
	}
	if j := lastFinished(a, p.Path, nil); j != nil {
		text, bad := jobResultText(j)
		color := t.Text
		if bad {
			color = t.Danger
		}
		ui.Column(c).Gap(4).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, locales.OvLastResult()).Bold()
				ui.Badge(c, jobKindLabel(j.Kind))
			})
			ui.Text(c, text).TextColor(color).Selectable()
		})
	}
}

// lastFinished 返回项目最近结束（且满足 match）的任务；没有时返回 nil。
func lastFinished(a *app.App, path string, match func(*app.Job) bool) *app.Job {
	var best *app.Job
	for _, j := range a.Jobs {
		if j.Project != path || j.Active() || (match != nil && !match(j)) {
			continue
		}
		if best == nil || !j.Finished.Before(best.Finished) {
			best = j
		}
	}
	return best
}
