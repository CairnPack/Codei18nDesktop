package app

import (
	"context"
	"errors"
	"time"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n/core/secrets"
)

// TokenScope 是令牌所在层。
type TokenScope string

const (
	ScopeGlobal  TokenScope = "global"  // 配置档的全局令牌
	ScopeProject TokenScope = "project" // 项目专用令牌
)

// TokenPanel 是令牌输入框的模型：默认遮罩、保存后不回显明文，只显示"已保存 ••••abcd"与来源层。
// 所有读写都经由 core 的令牌 API（系统凭据库），保存后 CLI 与 git hook 立即可用。
type TokenPanel struct {
	Scope   TokenScope
	Profile string

	// Input 绑定到密码输入框；保存或取消后立即清空。
	Input string
	// Reveal 为 true 时临时显示输入内容（只影响正在输入的内容，已保存的令牌永远不会回显）。
	Reveal bool
	// ConfirmDelete 为 true 时视图显示二次确认。
	ConfirmDelete bool

	Saved bool
	Last4 string
	// Source 是项目页显示的最终生效来源（env / project / global / config-file / none）。
	Source codei18n.TokenSource
	EnvVar string
	// Blocked 表示项目配置把地址改到了全局配置之外，全局令牌不会被使用。
	Blocked bool
	// Plaintext 是配置文件中发现的明文令牌位置（显示警告条与迁移按钮）。
	Plaintext []string

	Testing     bool
	TestLatency time.Duration
	TestErr     string
	TestDone    bool

	Err string

	update      func(func())
	project     Project
	global      Global
	testProfile func(ctx context.Context, profile string) error
}

func (a *App) newProjectTokenPanel(path string, p Project) *TokenPanel {
	t := &TokenPanel{Scope: ScopeProject, update: a.deps.Update, project: p}
	t.refresh()
	return t
}

func newGlobalTokenPanel(g Global, profile string, update func(func()), test func(context.Context, string) error) *TokenPanel {
	t := &TokenPanel{Scope: ScopeGlobal, Profile: profile, global: g, update: update, testProfile: test}
	t.refresh()
	return t
}

func (t *TokenPanel) refresh() {
	t.Err = ""
	switch t.Scope {
	case ScopeGlobal:
		saved, last4, err := t.global.ProfileTokenStatus(t.Profile)
		if err != nil {
			t.Err = errorText(err)
		}
		t.Saved, t.Last4 = saved, last4
	case ScopeProject:
		st, err := t.project.TokenStatus()
		if err != nil {
			t.Err = errorText(err)
			return
		}
		t.Profile = st.Profile
		t.Saved, t.Last4 = st.ProjectSaved, st.ProjectLast4
		t.Source, t.EnvVar, t.Blocked = st.Source, st.EnvVar, st.GlobalBlocked
		t.Plaintext = t.Plaintext[:0]
		for _, p := range st.Plaintext {
			t.Plaintext = append(t.Plaintext, p.Location)
		}
		if st.StoreError != "" {
			t.Err = st.StoreError
		}
	}
}

// Save 保存（或替换）输入的令牌，然后清空输入框。
func (t *TokenPanel) Save() {
	s := codei18n.NewSecret([]byte(t.Input))
	t.Input, t.Reveal = "", false
	if s.IsZero() {
		t.Err = errorText(secrets.ErrEmpty)
		return
	}
	var err error
	switch t.Scope {
	case ScopeGlobal:
		err = t.global.SetProfileToken(t.Profile, s)
	case ScopeProject:
		err = t.project.SetProjectToken(t.Profile, s)
	}
	if err != nil {
		t.Err = errorText(err)
		return
	}
	t.refresh()
}

// AskDelete 进入删除的二次确认。
func (t *TokenPanel) AskDelete() { t.ConfirmDelete = true }

// CancelDelete 取消删除。
func (t *TokenPanel) CancelDelete() { t.ConfirmDelete = false }

// ConfirmDeleteNow 确认后删除令牌。
func (t *TokenPanel) ConfirmDeleteNow() {
	t.ConfirmDelete = false
	t.deleteNow()
}

func (t *TokenPanel) deleteNow() {
	var err error
	switch t.Scope {
	case ScopeGlobal:
		err = t.global.DeleteProfileToken(t.Profile)
	case ScopeProject:
		err = t.project.DeleteProjectToken(t.Profile)
	}
	if err != nil && !errors.Is(err, secrets.ErrNotFound) {
		t.Err = errorText(err)
		return
	}
	t.refresh()
}

// MigratePlaintext 把项目配置中的明文令牌迁入系统凭据库。
func (t *TokenPanel) MigratePlaintext() {
	if t.project == nil {
		return
	}
	if _, err := t.project.MigratePlaintextTokens(); err != nil {
		t.Err = errorText(err)
		return
	}
	t.refresh()
}

// testTimeout 是"测试连接"的超时。
const testTimeout = 30 * time.Second

// Test 用当前生效的令牌发一次短请求，显示延迟或错误（错误已由 core 脱敏，不含令牌）。
func (t *TokenPanel) Test() {
	if t.Testing || (t.project == nil && t.testProfile == nil) {
		return
	}
	t.Testing, t.TestDone, t.TestErr = true, false, ""
	p, test, profile := t.project, t.testProfile, t.Profile
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()
		start := time.Now()
		var err error
		if p != nil {
			_, err = p.TranslateText(ctx, "Hello", "", "")
		} else {
			err = test(ctx, profile)
		}
		elapsed := time.Since(start)
		t.update(func() {
			t.Testing, t.TestDone, t.TestLatency, t.TestErr = false, true, elapsed, errorText(err)
		})
	}()
}
