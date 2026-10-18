// Package app 是桌面端的应用状态与业务逻辑：工作区、项目状态、任务队列、配置表单、令牌、注释浏览器。
//
// 线程模型：App 的所有字段只在主线程（mygo 视图所在线程）读写。长任务在后台 goroutine 中运行，
// 结果一律通过注入的 Update 函数送回主线程再修改状态，因此本包不需要锁。视图层只调用 App 的方法，
// 不直接做 I/O；所有外部依赖（core、对话框、文件监视、持久化目录）都通过 Deps 注入，便于测试。
package app

import (
	"context"

	"github.com/CairnPack/codei18n"
)

// Core 抽象 core 库的入口，测试中可以注入假实现或使用临时目录中的真实项目。
type Core interface {
	// Open 打开项目（项目不必已初始化）。
	Open(root string) (Project, error)
	// Global 打开全局配置（provider 配置档与全局令牌）。
	Global() (Global, error)
	// TestProfile 用配置档的有效设置与令牌发一次短请求（全局设置页的"测试连接"）。
	TestProfile(ctx context.Context, profile string) error
}

// Project 是 *codei18n.Project 中桌面端用到的方法。
type Project interface {
	Root() string
	Initialized() bool
	Init(opts codei18n.InitOptions) error
	Status() (*codei18n.Status, error)
	UpdateMappings(ctx context.Context, opts codei18n.MapOptions) (*codei18n.MapUpdateResult, error)
	Translate(ctx context.Context, opts codei18n.TranslateOptions) (*codei18n.TranslateResult, error)
	TranslateText(ctx context.Context, text, from, to string) (string, error)
	Comments() ([]codei18n.CommentRecord, error)
	SetTranslation(id, lang, text string) error
	SetCommentLanguage(id, lang string) error
	Failures() ([]codei18n.Failure, error)
	TokenStatus() (*codei18n.TokenStatus, error)
	SetProjectToken(profile string, s codei18n.Secret) error
	DeleteProjectToken(profile string) error
	MigratePlaintextTokens() ([]codei18n.MigratedToken, error)
	IsGitRepo() bool
	InstallHooks() error
	HooksInstalled() bool
	EnsureGitignore() (bool, error)
	ConfigView
}

// Global 是 *codei18n.Global 中桌面端用到的方法。
type Global interface {
	GlobalConfigView
	Profiles() ([]string, error)
	SetProfileToken(profile string, s codei18n.Secret) error
	DeleteProfileToken(profile string) error
	ProfileTokenStatus(profile string) (saved bool, last4 string, err error)
	CopyProfile(from, to string) error
	RenameProfile(from, to string) error
	DeleteProfile(name string) error
	PlaintextTokens() ([]codei18n.PlaintextToken, error)
	MigratePlaintextTokens() ([]codei18n.MigratedToken, error)
}

// Watcher 监视项目目录的文件变化；Start 返回停止函数。回调可能在任意 goroutine 上执行。
type Watcher interface {
	Watch(root string, onChange func()) (stop func(), err error)
}

// Deps 是 App 的外部依赖。
type Deps struct {
	Core Core
	// Update 把 fn 送到主线程执行并重绘（生产环境为 mygo Window.Update；测试中直接调用）。
	Update func(fn func())
	// DataDir 是桌面端自己的配置目录（最近项目列表、UI 设置），生产环境为 mygo 的 PathUserData。
	DataDir string
	// Watcher 可以为 nil（不支持文件监视）。
	Watcher Watcher
}
