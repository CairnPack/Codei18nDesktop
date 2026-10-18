package main

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// watcher 用 mygo.WatchFiles 监视项目目录（递归），忽略依赖、构建目录与 .codei18n。
type watcher struct{}

var ignoredDirs = map[string]bool{
	".git": true, ".codei18n": true, "node_modules": true, "build": true, ".gradle": true,
	"bin": true, "obj": true, ".dart_tool": true, "dist": true, "target": true, ".idea": true, "vendor": true,
}

func (watcher) Watch(root string, onChange func()) (func(), error) {
	ctx, cancel := context.WithCancel(context.Background())
	w, err := mygo.WatchFiles(ctx, root, mygo.FileWatchOptions{
		Recursive: true,
		Debounce:  500 * time.Millisecond,
		MaxDelay:  3 * time.Second,
		HintsOnly: true, // 大仓库只登记根目录，避免 Windows 上 8192 条目上限
		Ignore: func(path string, isDir bool) bool {
			for _, part := range strings.Split(path, "/") {
				if ignoredDirs[part] {
					return true
				}
			}
			return false
		},
	})
	if err != nil {
		cancel()
		return nil, err
	}
	go func() {
		for {
			if _, err := w.Next(ctx); err != nil {
				return
			}
			onChange()
		}
	}()
	return func() { cancel(); w.Close() }, nil
}
