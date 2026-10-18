package app

// applyWatch 按 Settings.Watch 启停所有项目的文件监视。
func (a *App) applyWatch() {
	if !a.Settings.Watch || a.deps.Watcher == nil {
		for path := range a.watches {
			a.stopWatch(path)
		}
		return
	}
	for _, p := range a.Projects {
		a.startWatch(p)
	}
}

func (a *App) startWatch(p *ProjectEntry) {
	if _, ok := a.watches[p.Path]; ok || a.deps.Watcher == nil {
		return
	}
	path := p.Path
	stop, err := a.deps.Watcher.Watch(path, func() {
		// 文件变化：只做增量扫描并刷新统计，不自动翻译（翻译会产生费用）。
		a.deps.Update(func() { a.onFilesChanged(path) })
	})
	if err != nil {
		a.notify(errorText(err))
		return
	}
	a.watches[path] = stop
}

func (a *App) stopWatch(path string) {
	if stop, ok := a.watches[path]; ok {
		stop()
		delete(a.watches, path)
	}
}

func (a *App) onFilesChanged(path string) {
	e := a.find(path)
	if e == nil || e.Status == nil || !e.Status.Initialized {
		return
	}
	a.Enqueue(path, JobScan)
}
