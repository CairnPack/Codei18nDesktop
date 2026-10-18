package app

import (
	"context"
	"os"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n/core/config"
)

// coreImpl 是基于 core 库的 Core 实现。
type coreImpl struct {
	opts []codei18n.Option
}

// NewCore 返回使用 core 库的 Core；opts 用于注入日志等（测试可注入全局目录与内存凭据库）。
func NewCore(opts ...codei18n.Option) Core { return coreImpl{opts: opts} }

func (c coreImpl) Open(root string) (Project, error) {
	p, err := codei18n.Open(root, c.opts...)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (c coreImpl) Global() (Global, error) {
	g, err := codei18n.OpenGlobal(c.opts...)
	if err != nil {
		return nil, err
	}
	return g, nil
}

// TestProfile 在临时目录中以该配置档打开一个空项目并翻译一句短文本，用于全局设置页的"测试连接"。
func (c coreImpl) TestProfile(ctx context.Context, profile string) error {
	dir, err := os.MkdirTemp("", "codei18n-test-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	opts := append(append([]codei18n.Option(nil), c.opts...), codei18n.WithOverrides(config.Overrides{Profile: &profile}))
	p, err := codei18n.Open(dir, opts...)
	if err != nil {
		return err
	}
	_, err = p.TranslateText(ctx, "Hello", "", "")
	return err
}
