package app

import (
	"errors"
	"sort"
	"strings"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n/core/langcode"
)

// Tri 是三态筛选：全部 / 是 / 否。
type Tri int

const (
	TriAll Tri = iota
	TriYes
	TriNo
)

// BrowserFilter 是注释浏览器的筛选条件。空字符串表示不筛选该项。
type BrowserFilter struct {
	File       string // 文件路径包含该子串
	Language   string // 编程语言标识（index.json 的 language）
	Lang       string // 识别出的原语言（规范码）
	Translated Tri
	Failed     Tri
	Query      string // 全文搜索（原文与所有译文，不区分大小写）
}

// LangShare 是一种原语言的注释数。
type LangShare struct {
	Lang  string
	Count int
}

// BrowserStats 是顶部统计。
type BrowserStats struct {
	Total      int
	Translated int
	Langs      []LangShare // 按数量降序
}

// Coverage 返回翻译覆盖率（0..1）。
func (s BrowserStats) Coverage() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Translated) / float64(s.Total)
}

// Browser 是注释浏览器的模型。Records 可能有数万条；视图用虚拟列表只渲染可见行。
type Browser struct {
	Project string
	// Display 是显示语言（项目的本地语言）。
	Display string
	Records []codei18n.CommentRecord
	Filter  BrowserFilter
	// Selected 是选中记录在 Records 中的下标（-1 表示未选中）。
	Selected int
	// Edit 绑定到译文编辑框。
	Edit    string
	Loading bool
	Err     string

	filtered []int
	key      BrowserFilter
	dirty    bool
	stats    BrowserStats
	app      *App
}

// OpenBrowser 打开选中项目的注释浏览器并在后台载入记录。
func (a *App) OpenBrowser() {
	e := a.SelectedProject()
	if e == nil {
		return
	}
	if a.Browser == nil || a.Browser.Project != e.Path {
		a.Browser = &Browser{Project: e.Path, Selected: -1, app: a}
		a.Browser.Reload()
	}
	a.Page = PageBrowser
}

// Reload 在后台重新载入记录（保留筛选条件与选中项的 ID）。
func (b *Browser) Reload() {
	a := b.app
	e := a.find(b.Project)
	if e == nil {
		return
	}
	b.Loading = true
	path := b.Project
	selectedID := ""
	if b.Selected >= 0 && b.Selected < len(b.Records) {
		selectedID = b.Records[b.Selected].ID
	}
	go func() {
		p, err := a.deps.Core.Open(path)
		var recs []codei18n.CommentRecord
		display := ""
		if err == nil {
			recs, err = p.Comments()
		}
		if err == nil {
			if cfg, cerr := p.Config(); cerr == nil {
				display = cfg.LocalLanguage.Value
			}
		}
		a.deps.Update(func() {
			if a.Browser != b {
				return
			}
			b.Loading = false
			if err != nil {
				b.Err = errorText(err)
				return
			}
			b.Err, b.Records, b.Display, b.dirty = "", recs, display, true
			b.Selected = -1
			for i, r := range recs {
				if r.ID == selectedID {
					b.Select(i)
					break
				}
			}
			b.computeStats()
		})
	}()
}

// Filtered 返回符合筛选条件的记录下标（筛选条件不变时复用上次结果）。
func (b *Browser) Filtered() []int {
	if !b.dirty && b.key == b.Filter && b.filtered != nil {
		return b.filtered
	}
	q := strings.ToLower(strings.TrimSpace(b.Filter.Query))
	b.filtered = b.filtered[:0]
	for i := range b.Records {
		if b.match(&b.Records[i], q) {
			b.filtered = append(b.filtered, i)
		}
	}
	b.key, b.dirty = b.Filter, false
	return b.filtered
}

func (b *Browser) match(r *codei18n.CommentRecord, q string) bool {
	f := b.Filter
	if f.File != "" && !strings.Contains(strings.ToLower(r.File), strings.ToLower(f.File)) {
		return false
	}
	if f.Language != "" && r.Language != f.Language {
		return false
	}
	if f.Lang != "" && !langcode.Equivalent(r.Lang, f.Lang) && r.Lang != f.Lang {
		return false
	}
	if f.Translated != TriAll && (b.IsTranslated(r) != (f.Translated == TriYes)) {
		return false
	}
	if f.Failed != TriAll && ((r.Error != "") != (f.Failed == TriYes)) {
		return false
	}
	if q != "" {
		hit := false
		for _, t := range r.Texts {
			if strings.Contains(strings.ToLower(t), q) {
				hit = true
				break
			}
		}
		if !hit && !strings.Contains(strings.ToLower(r.File), q) {
			return false
		}
	}
	return true
}

// IsTranslated 表示该条已有显示语言的文本（原文本身就是显示语言时也算）。
func (b *Browser) IsTranslated(r *codei18n.CommentRecord) bool {
	if langcode.Equivalent(r.SourceKey, b.Display) || langcode.Equivalent(r.Lang, b.Display) {
		return true
	}
	return r.Translation(b.Display) != ""
}

func (b *Browser) computeStats() {
	counts := map[string]int{}
	s := BrowserStats{Total: len(b.Records)}
	for i := range b.Records {
		r := &b.Records[i]
		counts[r.Lang]++
		if b.IsTranslated(r) {
			s.Translated++
		}
	}
	for lang, n := range counts {
		s.Langs = append(s.Langs, LangShare{Lang: lang, Count: n})
	}
	sort.Slice(s.Langs, func(i, j int) bool {
		if s.Langs[i].Count != s.Langs[j].Count {
			return s.Langs[i].Count > s.Langs[j].Count
		}
		return s.Langs[i].Lang < s.Langs[j].Lang
	})
	b.stats = s
}

// Stats 返回统计。
func (b *Browser) Stats() BrowserStats { return b.stats }

// FacetValues 返回筛选下拉框的可选值（编程语言、原语言、文件）。
func (b *Browser) FacetValues() (languages, langs []string) {
	ls, gs := map[string]bool{}, map[string]bool{}
	for _, r := range b.Records {
		if r.Language != "" {
			ls[r.Language] = true
		}
		gs[r.Lang] = true
	}
	for k := range ls {
		languages = append(languages, k)
	}
	for k := range gs {
		langs = append(langs, k)
	}
	sort.Strings(languages)
	sort.Strings(langs)
	return languages, langs
}

// Select 选中一条记录，并把它的显示语言译文放入编辑框。
func (b *Browser) Select(i int) {
	if i < 0 || i >= len(b.Records) {
		b.Selected, b.Edit = -1, ""
		return
	}
	b.Selected = i
	b.Edit = b.Records[i].Translation(b.Display)
}

// Current 返回选中的记录（可能为 nil）。
func (b *Browser) Current() *codei18n.CommentRecord {
	if b.Selected < 0 || b.Selected >= len(b.Records) {
		return nil
	}
	return &b.Records[b.Selected]
}

// ErrProjectBusy 表示项目正在运行任务，暂时不能编辑。
var ErrProjectBusy = errors.New("project busy")

// SaveEdit 把编辑框中的译文写回 mappings.json（原子写）。项目有任务运行时拒绝（core 返回 ErrBusy）。
func (b *Browser) SaveEdit() {
	r := b.Current()
	if r == nil {
		return
	}
	p, err := b.project()
	if err == nil {
		err = p.SetTranslation(r.ID, b.Display, b.Edit)
	}
	if err != nil {
		if errors.Is(err, codei18n.ErrBusy) {
			err = ErrProjectBusy
		}
		b.Err = errorText(err)
		return
	}
	b.Err = ""
	key := b.Display
	if k := langcode.FindKey(r.Texts, b.Display); k != "" {
		key = k
	}
	texts := make(map[string]string, len(r.Texts)+1)
	for k, v := range r.Texts {
		texts[k] = v
	}
	if b.Edit == "" {
		delete(texts, key)
	} else {
		texts[key] = b.Edit
	}
	r.Texts, b.dirty = texts, true
	b.computeStats()
}

// CorrectLanguage 纠正选中记录识别出的原语言（持久化到项目，下次扫描不覆盖）。lang 为空表示撤销纠正。
func (b *Browser) CorrectLanguage(lang string) {
	r := b.Current()
	if r == nil {
		return
	}
	p, err := b.project()
	if err == nil {
		err = p.SetCommentLanguage(r.ID, lang)
	}
	if err != nil {
		if errors.Is(err, codei18n.ErrBusy) {
			err = ErrProjectBusy
		}
		b.Err = errorText(err)
		return
	}
	b.Reload()
}

func (b *Browser) project() (Project, error) {
	e := b.app.find(b.Project)
	if e == nil {
		return nil, errors.New("project removed")
	}
	if e.running != nil {
		return nil, ErrProjectBusy
	}
	return b.app.ensureProject(e)
}
