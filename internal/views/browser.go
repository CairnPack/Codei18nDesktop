package views

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n-desktop/internal/app"
	"github.com/CairnPack/codei18n-desktop/locales"
)

// correctionLangs 是"纠正原语言"菜单中的常用语言码（语言码是数据，不是界面文本）。
var correctionLangs = []string{"en", "zh-Hans", "zh-Hant", "ja", "ko", "de", "fr", "es", "ru"}

// browserPage 是注释浏览器：顶部统计、筛选行、虚拟列表与详情编辑窗格。
// 记录可能有数万条：列表只构建可见行，筛选结果由 Browser.Filtered 记忆，
// 统计由 Browser.Stats 缓存，筛选下拉的可选值只在菜单打开时计算。
func browserPage(c *ui.Context, r *Root) {
	a := r.App
	p := a.SelectedProject()
	if a.Browser == nil || p == nil || a.Browser.Project != p.Path {
		a.OpenBrowser()
	}
	b := a.Browser
	if b == nil {
		return
	}
	t := c.Theme()

	ui.Row(c).Gap(10).Children(func() {
		ui.Button(c, locales.BrBack()).OnClick(func() { a.Page = app.PageOverview })
		ui.Text(c, locales.BrTitle(p.Name)).FontSize(20).Bold().SingleLine().Grow(1)
		if b.Loading {
			ui.Spinner(c).Label(locales.BrLoading())
		}
		ui.Button(c, locales.BrReload()).Disabled(b.Loading).OnClick(b.Reload)
	})
	browserStats(c, b)
	if b.Err != "" {
		msg := b.Err
		if msg == app.ErrProjectBusy.Error() {
			msg = locales.BrBusy()
		}
		ui.Text(c, msg).TextColor(t.Danger).Selectable()
	}
	browserFilters(c, b)

	rows := b.Filtered()
	ui.Text(c, locales.BrShown(len(rows), len(b.Records))).TextColor(t.TextMuted).FontSize(12)
	if r.ui.browserSplit == 0 {
		r.ui.browserSplit = 50
	}
	ui.SplitPercent(c, &r.ui.browserSplit, func() {
		browserList(c, r, b, rows)
	}, func() {
		ui.Scroll(c).Fill().Children(func() {
			browserDetail(c, b)
		})
	}).Grow(1)
}

// browserStats 显示总数、覆盖率与各原语言占比。
func browserStats(c *ui.Context, b *app.Browser) {
	t := c.Theme()
	st := b.Stats()
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, locales.BrSummary(st.Total, st.Translated, percent(st.Translated, st.Total)))
		ui.Progress(c, st.Coverage()).Width(120)
	})
	if len(st.Langs) == 0 {
		return
	}
	ui.Row(c).Gap(6).Children(func() {
		for i, l := range st.Langs {
			if i == 8 { // 只显示最多的几种
				break
			}
			name := l.Lang
			if name == "" {
				name = locales.BrLangUnknown()
			}
			ui.Badge(c, locales.BrLangShare(name, percent(l.Count, st.Total))).TextColor(t.Text)
		}
	})
}

// percent 返回 n/total 的整数百分比。
func percent(n, total int) int {
	if total == 0 {
		return 0
	}
	return int(math.Round(float64(n) * 100 / float64(total)))
}

// browserFilters 是筛选行：文件、编程语言、原语言、已翻译、失败与全文搜索，直接绑定到 Browser.Filter。
func browserFilters(c *ui.Context, b *app.Browser) {
	f := &b.Filter
	ui.Row(c).Gap(8).Children(func() {
		ui.TextInput(c, &f.File).Placeholder(locales.BrFilterFile()).Label(locales.BrFilterFile()).Width(170)
		label := f.Language
		if label == "" {
			label = locales.BrAllLanguages()
		}
		ui.MenuButton(c, label, func(m *ui.Menu) {
			// 菜单只在打开时构建，FacetValues 遍历全部记录的开销只在这时发生
			languages, _ := b.FacetValues()
			if m.Item(locales.BrAllLanguages()).Checked(f.Language == "").Chosen() {
				f.Language = ""
			}
			m.Separator()
			for _, l := range languages {
				if m.Item(l).Checked(f.Language == l).Chosen() {
					f.Language = l
				}
			}
		}).Label(locales.BrLanguage())
		label = f.Lang
		if label == "" {
			label = locales.BrAllLangs()
		}
		ui.MenuButton(c, label, func(m *ui.Menu) {
			_, langs := b.FacetValues()
			if m.Item(locales.BrAllLangs()).Checked(f.Lang == "").Chosen() {
				f.Lang = ""
			}
			m.Separator()
			for _, l := range langs {
				if l == "" {
					continue // 未识别的原语言无法按空串筛选（空串表示不筛选）
				}
				if m.Item(l).Checked(f.Lang == l).Chosen() {
					f.Lang = l
				}
			}
		}).Label(locales.BrLang())
		ui.SearchField(c, &f.Query).Label(locales.BrSearch()).Grow(1).MinWidth(160)
	})
	ui.Row(c).Gap(12).Children(func() {
		tr := int(f.Translated)
		if ui.Segmented(c, &tr, locales.BrTriAll(), locales.BrTranslatedYes(), locales.BrTranslatedNo()).
			Label(locales.BrTranslatedFilter()).Changed() {
			f.Translated = app.Tri(tr)
		}
		fl := int(f.Failed)
		if ui.Segmented(c, &fl, locales.BrTriAll(), locales.BrFailedYes(), locales.BrFailedNo()).
			Label(locales.BrFailedFilter()).Changed() {
			f.Failed = app.Tri(fl)
		}
	})
}

// browserList 是筛选结果的虚拟列表（Key 为注释 ID），只构建可见行。
func browserList(c *ui.Context, r *Root, b *app.Browser, rows []int) {
	t := c.Theme()
	// 选中行由模型推出：rows 是升序的记录下标，二分查找即可（每帧 O(log n)）
	r.ui.browserRow = -1
	if b.Selected >= 0 {
		if i := sort.SearchInts(rows, b.Selected); i < len(rows) && rows[i] == b.Selected {
			r.ui.browserRow = i
		}
	}
	recs := b.Records
	ls := &r.ui.browserList
	ls.Key = func(i int) any { return recs[rows[i]].ID }
	ls.Label = func(i int) string { return firstLine(recs[rows[i]].Original) }
	ls.Selected = &r.ui.browserRow
	list := ui.List(c, ls, len(rows)).Fill().Border(1, t.Border).Radius(8).Padding(4).Children(func() {
		if len(rows) == 0 && !b.Loading {
			ui.Text(c, locales.BrNoResults()).TextColor(t.TextMuted).Padding(12)
		}
	})
	list.Rows(func(row ui.ListRow) {
		browserRow(row.Context, b, &recs[rows[row.Index]], row.Selected())
	})
	if list.Changed() && r.ui.browserRow >= 0 && r.ui.browserRow < len(rows) {
		b.Select(rows[r.ui.browserRow])
	}
}

// browserRow 是列表中的一行：文件:行、编程语言、原语言徽标、原文首行、已翻译 / 失败标记。
// selected 时行背景为强调色，次要文字与标记改用强调色上的文字颜色以保证对比度。
func browserRow(c *ui.Context, b *app.Browser, rec *codei18n.CommentRecord, selected bool) {
	t := c.Theme()
	muted, ok, bad := t.TextMuted, t.Success, t.Danger
	if selected {
		muted, ok, bad = t.AccentText, t.AccentText, t.AccentText
	}
	ui.Column(c).Gap(2).Padding(6, 10).Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			ui.Text(c, location(rec)).FontSize(12).TextColor(muted).SingleLine().Grow(1)
			if rec.Language != "" {
				ui.Text(c, rec.Language).FontSize(12).TextColor(muted)
			}
			lang := rec.Lang
			if lang == "" {
				lang = locales.BrLangUnknown()
			}
			ui.Badge(c, lang)
			if b.IsTranslated(rec) {
				ui.Text(c, locales.BrMarkTranslated()).TextColor(ok).FontSize(12)
			}
			if rec.Error != "" {
				ui.Text(c, locales.BrMarkFailed()).TextColor(bad).FontSize(12)
			}
		})
		ui.Text(c, firstLine(rec.Original)).SingleLine()
	})
}

// location 返回"文件:行"（没有位置索引时退回注释 ID）。这是数据格式，不是界面文本。
func location(rec *codei18n.CommentRecord) string {
	if rec.File == "" {
		return rec.ID
	}
	return fmt.Sprintf("%s:%d", rec.File, rec.Range.StartLine)
}

// firstLine 返回文本的第一行（去掉首尾空白）。
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

// browserDetail 是详情窗格：元数据、纠正原语言、原文（只读可选中）与译文编辑框。
func browserDetail(c *ui.Context, b *app.Browser) {
	t := c.Theme()
	rec := b.Current()
	ui.Column(c).Gap(12).Padding(4, 4, 4, 12).Children(func() {
		if rec == nil {
			ui.Text(c, locales.BrNoSelection()).TextColor(t.TextMuted)
			return
		}
		ui.Column(c).Gap(4).Children(func() {
			meta(c, locales.BrMetaFile(), location(rec))
			meta(c, locales.BrMetaSymbol(), rec.Symbol)
			meta(c, locales.BrMetaType(), rec.Type)
			meta(c, locales.BrMetaParts(), strconv.Itoa(rec.Parts))
			meta(c, locales.BrMetaLang(), rec.Lang)
			meta(c, locales.BrMetaLangSource(), rec.LangSource)
			if rec.Error != "" {
				meta(c, locales.BrMetaError(), rec.Error)
			}
		})
		ui.Row(c).Children(func() {
			ui.MenuButton(c, locales.BrCorrectLanguage(), func(m *ui.Menu) {
				for _, l := range correctionLangs {
					if m.Item(l).Checked(rec.Lang == l).Chosen() {
						b.CorrectLanguage(l)
					}
				}
				m.Separator()
				if m.Item(locales.BrUndoCorrection()).Disabled(rec.LangSource != "override").Chosen() {
					b.CorrectLanguage("")
				}
			}).Label(locales.BrCorrectLanguage())
		})
		ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(1).Gap(6).Children(func() {
				ui.Text(c, locales.BrOriginal()).Bold()
				ui.Text(c, rec.Original).Selectable().Padding(8).Border(1, t.Border).Radius(6).FillWidth()
			})
			ui.Column(c).Grow(1).Gap(6).Children(func() {
				ui.Text(c, locales.BrTranslation(b.Display)).Bold()
				// 以注释 ID 为键：切换记录时编辑框状态（光标、撤销）不串到另一条
				ui.TextArea(c.Key(rec.ID), &b.Edit).Lines(3, 14).Label(locales.BrTranslationLabel())
				ui.Row(c).Justify(ui.End).Children(func() {
					ui.PrimaryButton(c, locales.BrSave()).Disabled(b.Loading).OnClick(b.SaveEdit)
				})
			})
		})
	})
}

// meta 显示一行元数据：标签与值（值为空时显示占位符）。
func meta(c *ui.Context, label, value string) {
	t := c.Theme()
	if value == "" {
		value = locales.BrEmptyValue()
	}
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, label).TextColor(t.TextMuted).Width(130)
		ui.Text(c, value).Selectable().Grow(1)
	})
}
