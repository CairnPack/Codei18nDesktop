package app

import (
	"context"
	"errors"
	"time"

	"github.com/CairnPack/codei18n"
)

// JobKind 是任务类型。
type JobKind string

const (
	JobScan        JobKind = "scan"        // 扫描 / 提取注释（map update，增量）
	JobTranslate   JobKind = "translate"   // 翻译缺失条目
	JobPrune       JobKind = "prune"       // 清理孤儿条目（map update --prune）
	JobRetryFailed JobKind = "retryFailed" // 重试上次失败的条目
)

// JobState 是任务状态。
type JobState string

const (
	JobQueued   JobState = "queued"
	JobRunning  JobState = "running"
	JobDone     JobState = "done"
	JobFailed   JobState = "failed"
	JobCanceled JobState = "canceled"
)

// Job 是一个后台任务。
type Job struct {
	ID       int
	Project  string
	Kind     JobKind
	State    JobState
	Progress codei18n.Event
	Err      string
	// IDs 只用于 JobRetryFailed：要重试的注释 ID。
	IDs []string
	// Map / Translate 是完成后的结果（按任务类型其一非 nil）。
	Map       *codei18n.MapUpdateResult
	Translate *codei18n.TranslateResult
	Queued    time.Time
	Started   time.Time
	Finished  time.Time

	cancel context.CancelFunc
}

// Active 表示任务排队中或运行中。
func (j *Job) Active() bool { return j.State == JobQueued || j.State == JobRunning }

// Fraction 返回 0..1 的进度；未知时返回 -1（显示为不确定进度条）。
func (j *Job) Fraction() float64 {
	if j.State == JobDone {
		return 1
	}
	if j.Progress.Total <= 0 {
		return -1
	}
	return float64(j.Progress.Done) / float64(j.Progress.Total)
}

// progressInterval 是进度事件送回主线程的最小间隔（扫描 2000 个文件会产生上千个事件）。
const progressInterval = 50 * time.Millisecond

// Enqueue 为项目加入一个任务。同一项目同类型的任务已在排队或运行时返回它，不重复加入。
func (a *App) Enqueue(path string, kind JobKind) *Job {
	entry := a.find(path)
	if entry == nil {
		return nil
	}
	for _, j := range a.Jobs {
		if j.Project == path && j.Kind == kind && j.Active() {
			return j
		}
	}
	a.nextJob++
	job := &Job{ID: a.nextJob, Project: path, Kind: kind, State: JobQueued, Queued: time.Now()}
	a.Jobs = append(a.Jobs, job)
	a.schedule()
	return job
}

// EnqueueChecked 为所有勾选的项目加入同一类任务（多项目批量），返回加入的任务。
func (a *App) EnqueueChecked(kind JobKind) []*Job {
	var out []*Job
	for _, p := range a.Projects {
		if p.Checked {
			if j := a.Enqueue(p.Path, kind); j != nil {
				out = append(out, j)
			}
		}
	}
	return out
}

// Cancel 取消任务：排队中的直接移出队列，运行中的取消其 context（core 在 1 个批次内停下，已完成结果已落盘）。
func (a *App) Cancel(id int) {
	for _, j := range a.Jobs {
		if j.ID != id {
			continue
		}
		switch j.State {
		case JobQueued:
			j.State, j.Finished = JobCanceled, time.Now()
		case JobRunning:
			if j.cancel != nil {
				j.cancel()
			}
		}
		return
	}
}

// CancelAll 取消所有排队与运行中的任务。
func (a *App) CancelAll() {
	for _, j := range a.Jobs {
		if j.Active() {
			a.Cancel(j.ID)
		}
	}
}

// ClearFinished 从任务列表中移除已结束的任务。
func (a *App) ClearFinished() {
	kept := a.Jobs[:0]
	for _, j := range a.Jobs {
		if j.Active() {
			kept = append(kept, j)
		}
	}
	a.Jobs = kept
}

// ActiveJobs 返回排队与运行中的任务。
func (a *App) ActiveJobs() []*Job {
	var out []*Job
	for _, j := range a.Jobs {
		if j.Active() {
			out = append(out, j)
		}
	}
	return out
}

// schedule 按全局并发上限启动排队中的任务；同一项目同一时间只运行一个任务（core 对同一项目的写任务互斥）。
func (a *App) schedule() {
	running := 0
	for _, j := range a.Jobs {
		if j.State == JobRunning {
			running++
		}
	}
	for _, j := range a.Jobs {
		if running >= a.Settings.MaxParallel {
			return
		}
		if j.State != JobQueued {
			continue
		}
		entry := a.find(j.Project)
		if entry == nil {
			j.State, j.Err, j.Finished = JobFailed, "project removed", time.Now()
			continue
		}
		if entry.running != nil {
			continue
		}
		a.start(entry, j)
		running++
	}
}

func (a *App) start(entry *ProjectEntry, j *Job) {
	ctx, cancel := context.WithCancel(context.Background())
	j.cancel, j.State, j.Started = cancel, JobRunning, time.Now()
	entry.running = j
	entry.Err = ""
	concurrency := a.Settings.Concurrency
	path, kind, id := entry.Path, j.Kind, j.ID
	go func() {
		p, err := a.deps.Core.Open(path)
		var mapRes *codei18n.MapUpdateResult
		var trRes *codei18n.TranslateResult
		if err == nil {
			progress := a.throttledProgress(id)
			switch kind {
			case JobScan, JobPrune:
				mapRes, err = p.UpdateMappings(ctx, codei18n.MapOptions{Prune: kind == JobPrune, Progress: progress})
			case JobTranslate:
				trRes, err = p.Translate(ctx, codei18n.TranslateOptions{Concurrency: concurrency, Progress: progress})
			case JobRetryFailed:
				var failures []codei18n.Failure
				if failures, err = p.Failures(); err == nil {
					ids := make([]string, 0, len(failures))
					for _, f := range failures {
						ids = append(ids, f.ID)
					}
					if len(ids) > 0 {
						trRes, err = p.Translate(ctx, codei18n.TranslateOptions{Concurrency: concurrency, OnlyIDs: ids, Progress: progress})
					} else {
						trRes = &codei18n.TranslateResult{}
					}
				}
			}
		}
		a.deps.Update(func() { a.finish(id, p, mapRes, trRes, err) })
	}()
}

// throttledProgress 把进度事件按 progressInterval 节流后送回主线程；阶段变化与完成事件总是立即送达。
func (a *App) throttledProgress(id int) codei18n.ProgressFunc {
	var last time.Time
	var lastPhase codei18n.Phase
	return func(e codei18n.Event) {
		now := time.Now()
		if e.Phase == lastPhase && e.Done < e.Total && now.Sub(last) < progressInterval {
			return
		}
		last, lastPhase = now, e.Phase
		a.deps.Update(func() {
			if j := a.job(id); j != nil && j.State == JobRunning {
				j.Progress = e
			}
		})
	}
}

func (a *App) finish(id int, p Project, mapRes *codei18n.MapUpdateResult, trRes *codei18n.TranslateResult, err error) {
	j := a.job(id)
	if j == nil {
		return
	}
	j.Finished, j.Map, j.Translate, j.cancel = time.Now(), mapRes, trRes, nil
	switch {
	case errors.Is(err, context.Canceled):
		j.State = JobCanceled
	case err != nil:
		j.State, j.Err = JobFailed, errorText(err)
	default:
		j.State = JobDone
	}
	if entry := a.find(j.Project); entry != nil {
		entry.running = nil
		if p != nil {
			entry.project = p
		}
		if j.State == JobFailed {
			entry.Err = j.Err
		}
		a.quickLoad(entry)
		if a.Browser != nil && a.Browser.Project == entry.Path && j.State != JobFailed {
			a.Browser.Reload()
		}
	}
	a.schedule()
}

func (a *App) job(id int) *Job {
	for _, j := range a.Jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}
