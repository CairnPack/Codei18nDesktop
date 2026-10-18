package views

import (
	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n-desktop/internal/app"
	"github.com/CairnPack/codei18n-desktop/locales"
)

// 概览页与任务页共用的任务显示辅助函数。

// jobKindLabel 返回任务类型的显示文本。
func jobKindLabel(k app.JobKind) string {
	switch k {
	case app.JobScan:
		return locales.TaskKindScan()
	case app.JobTranslate:
		return locales.TaskKindTranslate()
	case app.JobPrune:
		return locales.TaskKindPrune()
	case app.JobRetryFailed:
		return locales.TaskKindRetry()
	}
	return string(k)
}

// jobStateLabel 返回任务状态的显示文本。
func jobStateLabel(s app.JobState) string {
	switch s {
	case app.JobQueued:
		return locales.TaskStateQueued()
	case app.JobRunning:
		return locales.TaskStateRunning()
	case app.JobDone:
		return locales.TaskStateDone()
	case app.JobFailed:
		return locales.TaskStateFailed()
	case app.JobCanceled:
		return locales.TaskStateCanceled()
	}
	return string(s)
}

// phaseLabel 返回进度阶段的显示文本；还没有收到进度事件时显示"正在开始"。
func phaseLabel(p codei18n.Phase) string {
	switch p {
	case "":
		return locales.TaskPhaseStarting()
	case codei18n.PhaseDiscover:
		return locales.TaskPhaseDiscover()
	case codei18n.PhaseScan:
		return locales.TaskPhaseScan()
	case codei18n.PhaseMap:
		return locales.TaskPhaseMap()
	case codei18n.PhaseIndex:
		return locales.TaskPhaseIndex()
	case codei18n.PhaseTranslate:
		return locales.TaskPhaseTranslate()
	case codei18n.PhaseSave:
		return locales.TaskPhaseSave()
	case codei18n.PhaseConvert:
		return locales.TaskPhaseConvert()
	case codei18n.PhaseDone:
		return locales.TaskPhaseDone()
	}
	return string(p)
}

// jobProgress 显示运行中任务的阶段、计数、当前文件与进度条（未知总数时为不确定进度条）。
func jobProgress(c *ui.Context, j *app.Job) {
	t := c.Theme()
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, phaseLabel(j.Progress.Phase))
		if j.Progress.Total > 0 {
			ui.Text(c, locales.OvProgressCount(j.Progress.Done, j.Progress.Total)).TextColor(t.TextMuted)
		}
	})
	ui.Progress(c, j.Fraction()).FillWidth()
	if j.Progress.File != "" {
		// 文件路径是数据，不是界面文本
		ui.Text(c, j.Progress.File).TextColor(t.TextMuted).FontSize(12).SingleLine()
	}
}

// jobResultText 返回已结束任务的结果摘要（扫描：新增/迁移/孤儿；翻译：成功/失败；取消；错误）。
// 第二个返回值表示是否为错误（用危险色显示）。
func jobResultText(j *app.Job) (string, bool) {
	kind := jobKindLabel(j.Kind)
	switch j.State {
	case app.JobCanceled:
		return locales.OvJobCanceled(kind), false
	case app.JobFailed:
		return locales.OvJobFailed(kind, j.Err), true
	case app.JobDone:
	default:
		return jobStateLabel(j.State), false
	}
	if m := j.Map; m != nil {
		var s string
		switch {
		case j.Kind == app.JobPrune:
			s = locales.OvPruneResult(m.Pruned)
		case m.Unchanged:
			s = locales.OvUnchanged()
		default:
			s = locales.OvScanResult(m.TotalComments, m.AddedCount, m.MigratedCount)
		}
		if j.Kind == app.JobScan && m.Orphans > 0 {
			s += " " + locales.OvOrphans(m.Orphans)
		}
		if len(m.FileErrors) > 0 {
			s += " " + locales.OvFileErrors(len(m.FileErrors))
		}
		return s, false
	}
	if tr := j.Translate; tr != nil {
		if tr.Canceled {
			return locales.OvJobCanceled(kind), false
		}
		return locales.OvTranslateResult(tr.SuccessCount, tr.FailCount, tr.TotalTasks), tr.FailCount > 0
	}
	return jobStateLabel(j.State), false
}

// projectName 返回任务所属项目的显示名（项目已移除时退回路径）。
func projectName(a *app.App, path string) string {
	for _, p := range a.Projects {
		if p.Path == path {
			return p.Name
		}
	}
	return path
}
