package app

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/CairnPack/codei18n/core/config"
)

// ConfigView 是项目配置的读写接口（*codei18n.Project 实现）。
type ConfigView interface {
	Config() (*config.Effective, error)
	Files() (global, project *config.File, err error)
	UpdateConfig(fn func(*config.File) error) error
}

// GlobalConfigView 是全局配置的读写接口（*codei18n.Global 实现）。
type GlobalConfigView interface {
	File() (*config.File, error)
	Update(fn func(*config.File) error) error
	SaveProfile(name string, f config.ProviderFields) error
}

// FieldKey 标识设置页上的一个字段。
type FieldKey string

const (
	FieldSourceLanguage  FieldKey = "sourceLanguage"
	FieldLocalLanguage   FieldKey = "localLanguage"
	FieldProfile         FieldKey = "profile"
	FieldProvider        FieldKey = "provider"
	FieldBaseURL         FieldKey = "baseUrl"
	FieldModel           FieldKey = "model"
	FieldEndpoint        FieldKey = "endpoint"
	FieldBatchSize       FieldKey = "batchSize"
	FieldConcurrency     FieldKey = "concurrency"
	FieldExcludePatterns FieldKey = "excludePatterns"
	FieldDetectLanguages FieldKey = "detectLanguages"
)

// ProjectFields 是项目设置页按顺序显示的字段。
var ProjectFields = []FieldKey{
	FieldSourceLanguage, FieldLocalLanguage, FieldProfile, FieldProvider, FieldBaseURL, FieldModel,
	FieldEndpoint, FieldBatchSize, FieldConcurrency, FieldExcludePatterns, FieldDetectLanguages,
}

// FieldRow 是一个字段的显示数据：生效值、来源层与编辑缓冲。
type FieldRow struct {
	Key    FieldKey
	Value  string       // 生效值（列表字段以换行分隔）
	Source config.Layer // default / global / project / override
	// Edit 绑定到输入框；与 Value 不同且点击"覆盖"时写入项目配置。
	Edit string
}

// Overridden 表示该字段的值来自项目配置。
func (r FieldRow) Overridden() bool { return r.Source == config.LayerProject }

// Multiline 表示该字段是列表（每行一项）。
func (r FieldRow) Multiline() bool {
	return r.Key == FieldExcludePatterns || r.Key == FieldDetectLanguages
}

// ProjectForm 是项目设置页的模型。
type ProjectForm struct {
	Project  string
	Rows     []FieldRow
	Profiles []string
	Tokens   *TokenPanel
	Err      string

	view ConfigView
}

// OpenProjectSettings 打开选中项目的设置页。
func (a *App) OpenProjectSettings() {
	e := a.SelectedProject()
	if e == nil {
		return
	}
	p, err := a.ensureProject(e)
	if err != nil {
		a.notify(errorText(err))
		return
	}
	f := &ProjectForm{Project: e.Path, view: p}
	if g, err := a.deps.Core.Global(); err == nil {
		f.Profiles, _ = g.Profiles()
	}
	f.reload()
	f.Tokens = a.newProjectTokenPanel(e.Path, p)
	a.ProjectForm, a.Page = f, PageProjectSettings
}

func (f *ProjectForm) reload() {
	eff, err := f.view.Config()
	if err != nil {
		f.Err = errorText(err)
		return
	}
	f.Err = ""
	f.Rows = f.Rows[:0]
	for _, k := range ProjectFields {
		v, src := fieldValue(eff, k)
		f.Rows = append(f.Rows, FieldRow{Key: k, Value: v, Source: src, Edit: v})
	}
}

// Row 返回字段的行（不存在时返回零值）。
func (f *ProjectForm) Row(k FieldKey) *FieldRow {
	for i := range f.Rows {
		if f.Rows[i].Key == k {
			return &f.Rows[i]
		}
	}
	return nil
}

// Override 把字段的编辑值写入项目配置（只覆盖这一个字段，原子写）。
func (f *ProjectForm) Override(k FieldKey) {
	row := f.Row(k)
	if row == nil {
		return
	}
	value := strings.TrimSpace(row.Edit)
	err := f.view.UpdateConfig(func(c *config.File) error { return setField(c, k, value) })
	if err != nil {
		f.Err = errorText(err)
		return
	}
	f.reload()
}

// Reset 恢复字段的继承（从项目配置中删除该字段）。
func (f *ProjectForm) Reset(k FieldKey) {
	if err := f.view.UpdateConfig(func(c *config.File) error { clearField(c, k); return nil }); err != nil {
		f.Err = errorText(err)
		return
	}
	f.reload()
}

// ResetAll 恢复全部字段的继承；deleteToken 为 true 时同时删除项目专用令牌。
func (f *ProjectForm) ResetAll(deleteToken bool) {
	err := f.view.UpdateConfig(func(c *config.File) error {
		for _, k := range ProjectFields {
			clearField(c, k)
		}
		return nil
	})
	if err != nil {
		f.Err = errorText(err)
		return
	}
	if deleteToken && f.Tokens != nil {
		f.Tokens.deleteNow()
	}
	f.reload()
}

// fieldValue 取生效值与来源。
func fieldValue(e *config.Effective, k FieldKey) (string, config.Layer) {
	p := e.Provider
	switch k {
	case FieldSourceLanguage:
		return e.SourceLanguage.Value, e.SourceLanguage.Source
	case FieldLocalLanguage:
		return e.LocalLanguage.Value, e.LocalLanguage.Source
	case FieldProfile:
		return e.Profile.Value, e.Profile.Source
	case FieldProvider:
		return p.Name.Value, p.Name.Source
	case FieldBaseURL:
		return p.BaseURL.Value, p.BaseURL.Source
	case FieldModel:
		return p.Model.Value, p.Model.Source
	case FieldEndpoint:
		return p.Endpoint.Value, p.Endpoint.Source
	case FieldBatchSize:
		return strconv.Itoa(p.BatchSize.Value), p.BatchSize.Source
	case FieldConcurrency:
		return strconv.Itoa(p.Concurrency.Value), p.Concurrency.Source
	case FieldExcludePatterns:
		return strings.Join(e.ExcludePatterns.Value, "\n"), e.ExcludePatterns.Source
	case FieldDetectLanguages:
		return strings.Join(e.DetectLanguages.Value, "\n"), e.DetectLanguages.Source
	}
	return "", config.LayerDefault
}

// ErrInvalidNumber 表示数字字段不是正整数。
var ErrInvalidNumber = errors.New("invalid number")

// setField 把字段写入项目配置层。provider 相关字段写入 profileOverrides（项目对所选配置档的单字段覆盖）。
func setField(c *config.File, k FieldKey, value string) error {
	po := func() *config.ProviderFields {
		if c.ProfileOverrides == nil {
			c.ProfileOverrides = &config.ProviderFields{}
		}
		return c.ProfileOverrides
	}
	num := func() (*int, error) {
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return nil, ErrInvalidNumber
		}
		return &n, nil
	}
	switch k {
	case FieldSourceLanguage:
		c.SourceLanguage = nonEmpty(value)
	case FieldLocalLanguage:
		c.LocalLanguage = nonEmpty(value)
	case FieldProfile:
		c.Profile = nonEmpty(value)
	case FieldProvider:
		po().Provider = nonEmpty(value)
	case FieldBaseURL:
		po().BaseURL = nonEmpty(value)
	case FieldModel:
		po().Model = nonEmpty(value)
	case FieldEndpoint:
		po().Endpoint = nonEmpty(value)
	case FieldBatchSize:
		n, err := num()
		if err != nil {
			return err
		}
		po().BatchSize = n
	case FieldConcurrency:
		n, err := num()
		if err != nil {
			return err
		}
		po().Concurrency = n
	case FieldExcludePatterns:
		c.ExcludePatterns = lines(value)
	case FieldDetectLanguages:
		c.DetectLanguages = lines(value)
	}
	return nil
}

// clearField 从项目配置中删除字段，恢复继承。provider 字段同时清除旧版 init 写入的快照字段。
func clearField(c *config.File, k FieldKey) {
	po := c.ProfileOverrides
	switch k {
	case FieldSourceLanguage:
		c.SourceLanguage = nil
	case FieldLocalLanguage:
		c.LocalLanguage = nil
	case FieldProfile:
		c.Profile = nil
	case FieldProvider:
		c.TranslationProvider = nil
		if po != nil {
			po.Provider = nil
		}
	case FieldBaseURL:
		deleteLegacy(c, "baseurl", "base_url")
		if po != nil {
			po.BaseURL = nil
		}
	case FieldModel:
		deleteLegacy(c, "model")
		if po != nil {
			po.Model = nil
		}
	case FieldEndpoint:
		deleteLegacy(c, "endpoint")
		if po != nil {
			po.Endpoint = nil
		}
	case FieldBatchSize:
		c.BatchSize = nil
		if po != nil {
			po.BatchSize = nil
		}
	case FieldConcurrency:
		c.Concurrency = nil
		if po != nil {
			po.Concurrency = nil
		}
	case FieldExcludePatterns:
		c.ExcludePatterns = nil
	case FieldDetectLanguages:
		c.DetectLanguages = nil
	}
}

func deleteLegacy(c *config.File, keys ...string) {
	for k := range c.TranslationConfig {
		for _, want := range keys {
			if strings.EqualFold(k, want) {
				delete(c.TranslationConfig, k)
			}
		}
	}
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// ---- 全局设置 ----

// ProfileEdit 是全局配置档的编辑缓冲。
type ProfileEdit struct {
	Name, Provider, BaseURL, Model, Endpoint, BatchSize, Concurrency string
}

// GlobalForm 是全局设置页的模型。
type GlobalForm struct {
	Profiles []string
	// Default 是全局默认选用的配置档。
	Default string
	// Selected 是正在编辑的配置档。
	Selected string
	Edit     ProfileEdit
	Tokens   *TokenPanel
	// Plaintext 是全局配置中发现的明文令牌字段位置（显示警告条）。
	Plaintext []string
	Err       string

	g      Global
	update func(func())
	test   func(context.Context, string) error
}

// OpenGlobalSettings 打开全局设置页。
func (a *App) OpenGlobalSettings() {
	g, err := a.deps.Core.Global()
	if err != nil {
		a.notify(errorText(err))
		return
	}
	f := &GlobalForm{g: g, update: a.deps.Update, test: a.deps.Core.TestProfile}
	f.reload()
	if len(f.Profiles) > 0 {
		f.SelectProfile(f.Profiles[0])
	}
	a.GlobalForm, a.Page = f, PageGlobalSettings
}

func (f *GlobalForm) reload() {
	f.Profiles, _ = f.g.Profiles()
	if file, err := f.g.File(); err == nil && file.Profile != nil {
		f.Default = *file.Profile
	} else {
		f.Default = ""
	}
	f.Plaintext = f.Plaintext[:0]
	if pts, err := f.g.PlaintextTokens(); err == nil {
		for _, p := range pts {
			f.Plaintext = append(f.Plaintext, p.Location)
		}
	}
}

// SelectProfile 选择要编辑的配置档。
func (f *GlobalForm) SelectProfile(name string) {
	file, err := f.g.File()
	if err != nil {
		f.Err = errorText(err)
		return
	}
	pf := file.Profiles[name]
	f.Selected = name
	f.Edit = ProfileEdit{Name: name}
	if pf != nil {
		f.Edit.Provider, f.Edit.BaseURL, f.Edit.Model, f.Edit.Endpoint = deref(pf.Provider), deref(pf.BaseURL), deref(pf.Model), deref(pf.Endpoint)
		f.Edit.BatchSize, f.Edit.Concurrency = itoa(pf.BatchSize), itoa(pf.Concurrency)
	}
	f.Tokens = newGlobalTokenPanel(f.g, name, f.update, f.test)
}

// NewProfile 新建一个配置档（名称自动编号）并选中它。
func (f *GlobalForm) NewProfile(base string) {
	name := uniqueName(base, f.Profiles)
	if err := f.g.SaveProfile(name, config.ProviderFields{Provider: config.Ptr("openai")}); err != nil {
		f.Err = errorText(err)
		return
	}
	f.reload()
	f.SelectProfile(name)
}

// SaveSelected 保存正在编辑的配置档字段；名称变化时先重命名（core 会迁移令牌）。
func (f *GlobalForm) SaveSelected() {
	if f.Selected == "" {
		return
	}
	name := strings.TrimSpace(f.Edit.Name)
	if name != f.Selected {
		if err := f.g.RenameProfile(f.Selected, name); err != nil {
			f.Err = errorText(err)
			return
		}
		f.Selected = name
	}
	pf := config.ProviderFields{Provider: nonEmpty(f.Edit.Provider), BaseURL: nonEmpty(f.Edit.BaseURL),
		Model: nonEmpty(f.Edit.Model), Endpoint: nonEmpty(f.Edit.Endpoint)}
	var err error
	if pf.BatchSize, err = optionalNumber(f.Edit.BatchSize); err != nil {
		f.Err = errorText(err)
		return
	}
	if pf.Concurrency, err = optionalNumber(f.Edit.Concurrency); err != nil {
		f.Err = errorText(err)
		return
	}
	// 保留原有的未识别字段与 apiKeyRef：在现有对象上修改。
	err = f.g.Update(func(c *config.File) error {
		if c.Profiles == nil {
			c.Profiles = map[string]*config.ProviderFields{}
		}
		cur := c.Profiles[name]
		if cur == nil {
			cur = &config.ProviderFields{}
			c.Profiles[name] = cur
		}
		cur.Provider, cur.BaseURL, cur.Model, cur.Endpoint = pf.Provider, pf.BaseURL, pf.Model, pf.Endpoint
		cur.BatchSize, cur.Concurrency = pf.BatchSize, pf.Concurrency
		return nil
	})
	if err != nil {
		f.Err = errorText(err)
		return
	}
	f.Err = ""
	f.reload()
	f.SelectProfile(name)
}

// CopySelected 复制正在编辑的配置档（令牌不复制）。
func (f *GlobalForm) CopySelected() {
	if f.Selected == "" {
		return
	}
	name := uniqueName(f.Selected+"-copy", f.Profiles)
	if err := f.g.CopyProfile(f.Selected, name); err != nil {
		f.Err = errorText(err)
		return
	}
	f.reload()
	f.SelectProfile(name)
}

// DeleteSelected 删除正在编辑的配置档，并一并删除它在凭据库中的令牌（由 core 完成）。
func (f *GlobalForm) DeleteSelected() {
	if f.Selected == "" {
		return
	}
	if err := f.g.DeleteProfile(f.Selected); err != nil {
		f.Err = errorText(err)
		return
	}
	f.Selected, f.Tokens = "", nil
	f.reload()
	if len(f.Profiles) > 0 {
		f.SelectProfile(f.Profiles[0])
	}
}

// SetDefault 设置全局默认选用的配置档（空串表示不选用）。
func (f *GlobalForm) SetDefault(name string) {
	if err := f.g.Update(func(c *config.File) error { c.Profile = nonEmpty(name); return nil }); err != nil {
		f.Err = errorText(err)
		return
	}
	f.reload()
}

// MigratePlaintext 把全局配置中的明文令牌迁入系统凭据库。
func (f *GlobalForm) MigratePlaintext() {
	if _, err := f.g.MigratePlaintextTokens(); err != nil {
		f.Err = errorText(err)
		return
	}
	f.reload()
	if f.Selected != "" {
		f.SelectProfile(f.Selected)
	}
}

func uniqueName(base string, existing []string) string {
	taken := map[string]bool{}
	for _, n := range existing {
		taken[n] = true
	}
	base = strings.Join(strings.Fields(base), "-")
	if base == "" {
		base = "profile"
	}
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		if n := base + "-" + strconv.Itoa(i); !taken[n] {
			return n
		}
	}
}

func optionalNumber(s string) (*int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return nil, ErrInvalidNumber
	}
	return &n, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func itoa(n *int) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(*n)
}

// Layers 返回来源层的显示顺序（视图用于图例）。
func Layers() []config.Layer {
	return []config.Layer{config.LayerDefault, config.LayerGlobal, config.LayerProject, config.LayerOverride}
}
