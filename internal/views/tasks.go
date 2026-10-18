package views

import (
	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n-desktop/internal/app"
	"github.com/CairnPack/codei18n-desktop/locales"
)

// tasksPage 是多项目批量任务页：勾选项目、批量加入扫描/翻译、全局并发上限与全部任务列表。
func tasksPage(c *ui.Context, r *Root) {
	a := r.App
	t := c.Theme()
	ui.Text(c, locales.TaskTitle()).FontSize(22).Bold()
	ui.Text(c, locales.TaskHint()).TextColor(t.TextMuted)

	checked, finished := 0, 0
	for _, p := range a.Projects {
		if p.Checked {
			checked++
		}
	}
	for _, j := range a.Jobs {
		if !j.Active() {
			finished++
		}
	}
	active := len(a.Jobs) - finished

	if len(a.Projects) == 0 {
		ui.Text(c, locales.TaskNoProjects()).TextColor(t.TextMuted)
	} else {
		taskProjects(c, r, checked)
	}

	ui.Row(c).Gap(8).Children(func() {
		if ui.PrimaryButton(c, locales.TaskScanSelected()).Disabled(checked == 0).Clicked() {
			a.EnqueueChecked(app.JobScan)
		}
		if ui.Button(c, locales.TaskTranslateSelected()).Disabled(checked == 0).Clicked() {
			a.EnqueueChecked(app.JobTranslate)
		}
		ui.Spacer(c)
		ui.Button(c, locales.TaskCancelAll()).Disabled(active == 0).OnClick(a.CancelAll)
		// 清理会改动正在渲染的任务列表，放到构建结束后执行
		ui.Button(c, locales.TaskClearFinished()).Disabled(finished == 0).OnClick(a.ClearFinished)
	})
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, locales.TaskParallel())
		v := float64(a.Settings.MaxParallel)
		if ui.NumberInput(c, &v, 1, 16, 1).Label(locales.TaskParallel()).Width(170).Changed() {
			s := a.Settings
			s.MaxParallel = int(v)
			a.SetSettings(s)
		}
		ui.Text(c, locales.TaskParallelHint()).TextColor(t.TextMuted).FontSize(12)
	})

	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, locales.TaskJobs()).Bold()
		ui.Text(c, locales.TaskSummary(active, len(a.Jobs))).TextColor(t.TextMuted)
	})
	jobs := a.Jobs
	r.ui.jobsList.Key = func(i int) any { return jobs[i].ID }
	ui.List(c, &r.ui.jobsList, len(jobs), func(i int) {
		jobRow(c, a, jobs[i])
	}).Grow(1).Gap(4).Border(1, t.Border).Radius(8).Padding(4).Children(func() {
		if len(jobs) == 0 {
			ui.Text(c, locales.TaskNoJobs()).TextColor(t.TextMuted).Padding(12)
		}
	})
}

// taskProjects 是带勾选框的项目列表（勾选状态即 ProjectEntry.Checked）。
func taskProjects(c *ui.Context, r *Root, checked int) {
	a := r.App
	t := c.Theme()
	all := checked == len(a.Projects)
	if ui.Checkbox(c, &all, locales.TaskSelectAll()).Changed() {
		for _, p := range a.Projects {
			p.Checked = all
		}
	}
	ui.Scroll(c).MaxHeight(220).Border(1, t.Border).Radius(8).Children(func() {
		ui.Column(c).Padding(4, 8).Gap(2).Children(func() {
			for _, p := range a.Projects {
				ui.Row(c).Key(p.Path).Gap(10).Padding(4, 0).Children(func() {
					ui.Checkbox(c.Key("check"), &p.Checked, p.Name).Label(locales.TaskSelectProject(p.Name))
					stateBadge(c, p.State())
					ui.Text(c, p.Path).TextColor(t.TextMuted).FontSize(12).SingleLine().Grow(1)
				})
			}
		})
	})
}

// jobRow 是任务列表中的一行：项目、类型、状态、进度、当前文件、错误与单个取消按钮。
func jobRow(c *ui.Context, a *app.App, j *app.Job) {
	t := c.Theme()
	ui.Column(c).Gap(4).Padding(8, 10).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, projectName(a, j.Project)).Bold().SingleLine()
			ui.Text(c, jobKindLabel(j.Kind)).TextColor(t.TextMuted)
			ui.Badge(c, jobStateLabel(j.State))
			ui.Spacer(c)
			if j.Active() {
				id := j.ID
				ui.Button(c, locales.TaskCancelJob()).OnClick(func() { a.Cancel(id) })
			}
		})
		switch {
		case j.State == app.JobRunning:
			jobProgress(c, j)
		case j.State == app.JobFailed:
			ui.Text(c, j.Err).TextColor(t.Danger).Selectable()
		case !j.Active():
			text, bad := jobResultText(j)
			color := t.TextMuted
			if bad {
				color = t.Danger
			}
			ui.Text(c, text).TextColor(color).FontSize(12)
		}
	})
}
