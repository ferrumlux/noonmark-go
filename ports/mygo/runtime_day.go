// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskview"
)

func weekdayNarrow(d core.LocalDate) string {
	return []string{"日", "一", "二", "三", "四", "五", "六"}[int(time.Date(d.Year, time.Month(d.Month), d.Day, 12, 0, 0, 0, time.UTC).Weekday())]
}
func dayMeasure(c *ui.Context, key string, width, size float32, weight int, text string) (w, h float32) {
	theme := *c.Theme()
	theme.FontSize = size
	ui.Themed(c.Key(key), &theme).Absolute().Size(0, 0).Background(ui.Transparent).IgnoreInput(true).Children(func() { w, h = c.MeasureText(width, ui.Span{Text: text, Size: size, Weight: weight}) })
	return
}
func (s *app) selectRuntimeDate(date core.LocalDate, focus bool) {
	if !date.Valid() {
		return
	}
	s.titleTransition(func() {
		s.runtimeDate = date
		s.runtimeSelection = ""
		if focus {
			s.runtimeDayFocus = date.String()
		} else {
			s.runtimeDayFocus = ""
		}
	})
}
func (s *app) liveDayView(c *ui.Context, w, h float32) {
	if focus := ui.FocusedValue(c, &s.runtimeDayFocus); focus != "" && !s.runtimePoolPicker {
		if d, e := core.ParseLocalDate(focus); e == nil {
			for _, key := range []struct {
				key  ui.Key
				days int
			}{{ui.KeyLeft, -1}, {ui.KeyRight, 1}, {ui.KeyUp, -7}, {ui.KeyDown, 7}} {
				if c.Shortcut(0, key.key) {
					if next, e := d.AddDays(key.days); e == nil {
						s.selectRuntimeDate(next, true)
					}
				}
			}
		}
	}
	g := s.runtime.Snapshot()
	day, e := taskview.ProjectDay(g, s.runtimeDate, s.runtime.Today())
	day.Rows = s.retainSubtaskParents(day.Rows, g)
	sw := float32(220)
	if s.compact {
		sw = 72
	}
	rw := float32(0)
	if s.runtimeDayRail {
		rw = 280
	}
	mw := w - sw - rw
	s.liveSidebar(c, h, sw, g)
	if e != nil {
		txt(c, sw+20, 20, 20, "日待办无法安全显示", ink, 600)
		txt(c, sw+20, 60, 12, "任务事实或日期未通过验证，未修改任何资料。", sourceVisual.warning, 400)
		return
	}
	ui.Box(c).Absolute().Left(sw).Top(0).Size(mw, h).Clip().Children(func() { s.liveDayMain(c, mw, h, day, g) })
	if rw > 0 {
		ui.Box(c).Absolute().Left(w-rw).Top(0).Size(rw, h).Clip().Children(func() { s.liveDayDetail(c, rw, h, day) })
	}
}
func (s *app) liveDayMain(c *ui.Context, w, h float32, day taskview.DayPage, graph core.TaskGraph) {
	title := fmt.Sprintf("%d年%d月%d日", day.Selected.Year, day.Selected.Month, day.Selected.Day)
	dateWidth, _ := dayMeasure(c, "day-date-width", 0, 20, 600, title)
	week := "周" + weekdayNarrow(day.Selected)
	weekWidth, _ := dayMeasure(c, "day-week-width", 0, 12, 400, week)
	badge := "今日"
	badgeColor := blue
	if day.IsHistory {
		badge = "历史已锁定"
		badgeColor = muted
	} else if day.IsFuture {
		badge = "未来"
	}
	badgeWidth, _ := dayMeasure(c, "day-badge-width", 0, 10.5, 600, badge)
	badgeWidth += 16
	controls := float32(64)
	if day.Selected != day.Today {
		controls += 52
	}
	if !s.runtimeDayRail {
		controls += 36
	}
	headerHeight := float32(28)
	if dateWidth+weekWidth+badgeWidth+24+controls > w-40 {
		headerHeight = 64
	}
	txt(c, 20, 16, 20, title, ink, 600).FontFeatures("tnum")
	txt(c, 20+dateWidth+8, 24, 12, week, sourceVisual.text3, 400)
	bg, fg := badgeColor.Alpha(.12), badgeColor
	if day.Selected == day.Today {
		bg, fg = blue, white
	}
	x := 20 + dateWidth + 8 + weekWidth + 8
	box(c, x, 23, badgeWidth, 20, bg, 10)
	txt(c, x+8, 24, 10.5, badge, fg, 600)
	end := w - 20
	actionsY := float32(16)
	if headerHeight > 28 {
		actionsY += 36
	}
	if !s.runtimeDayRail {
		calendarPaneButton(c, "day-expand-rail", end-28, actionsY, "‹‹", "展开日待办详情", func() { s.runtimeDayRail = true })
		end -= 36
	}
	_, nextErr := day.Selected.AddDays(1)
	_, prevErr := day.Selected.AddDays(-1)
	calendarHeaderButton(c, "day-next", end-28, actionsY, 28, "›", "后一天", nextErr == nil, func() {
		if d, e := s.runtimeDate.AddDays(1); e == nil {
			s.selectRuntimeDate(d, false)
		}
	})
	end -= 36
	if day.Selected != day.Today {
		calendarHeaderButton(c, "day-today", end-44, actionsY, 44, "今天", "回到今天", true, func() { s.selectRuntimeDate(s.runtime.Today(), false) })
		end -= 52
	}
	calendarHeaderButton(c, "day-prev", end-28, actionsY, 28, "‹", "前一天", prevErr == nil, func() {
		if d, e := s.runtimeDate.AddDays(-1); e == nil {
			s.selectRuntimeDate(d, false)
		}
	})
	stripY := 16 + headerHeight + 12
	ui.Box(c.Key("day-date-strip")).Absolute().Left(20).Top(stripY).Size(w-40, 66).Children(func() { s.liveDateStrip(c, w-40, day.Strip) })
	top := stripY + 66 + 10
	notice := ""
	if day.IsHistory {
		notice = "历史日已锁定：任务事实不可改写。复盘、延续与废弃尚未接通。"
	} else if day.IsFuture {
		notice = "未来日：可以新增排期，不能提前完成或生成复盘。"
	}
	if notice != "" {
		_, nh := dayMeasure(c, "day-notice-height", w-40, 11, 400, notice)
		txt(c, 20, top, 11, notice, sourceVisual.text2, 400).Width(w - 40)
		top += nh + 10
	}
	// Boundary and persistence messages remain visible even when there is no editor.
	if s.runtimeError != "" {
		_, eh := dayMeasure(c, "day-error-height", w-40, 11, 500, s.runtimeError)
		txt(c, 20, top, 11, s.runtimeError, sourceVisual.warning, 500).Width(w - 40)
		top += eh + 10
	}
	ui.Scroll(c.Key("live-scroll-day")).Absolute().Left(20).Top(top).Size(w-40, h-top-20).Children(func() {
		listTop := float32(14)
		if day.IsHistory {
			listTop = 12
		}
		ui.Box(c).Height(listTop).Width(w - 40)
		if !day.IsHistory {
			ui.Box(c.Key("day-quick-add")).Height(49).Width(w - 40).Children(func() {
				placeholder := "添加今日任务"
				if day.IsFuture {
					placeholder = "为这一天排期"
				}
				pickerText := "从任务池排期…"
				pickerWidth, _ := dayMeasure(c, "day-picker-width", 0, 12, 400, pickerText)
				pickerWidth += 20
				ui.TextInputBase(c.Key("live-task-input"), &s.draft).Absolute().Size(w-40-pickerWidth-8, 37).FontSize(12.5).Padding(8.5, 10).Background(sourceVisual.controlFill).Border(1, line.Alpha(.72)).Radius(8).Label("开发任务标题").Placeholder(placeholder).OnSubmit(func() { s.submitLiveDraft(false) })
				calendarHeaderButton(c, "day-from-pool", w-40-pickerWidth, 0, pickerWidth, pickerText, "从任务池排期", true, s.openPoolPicker)
			})
		}
		if s.runtimeNotice != "" && s.runtimeError == "" {
			ui.Text(c, s.runtimeNotice).FontSize(10.5).TextColor(muted)
			ui.Box(c).Height(8)
		}
		pinCount := 0
		for _, r := range day.Rows {
			if r.Pinned {
				pinCount++
			}
		}
		if pinCount > 0 {
			ui.Text(c, fmt.Sprintf("📌  %d", pinCount)).FontSize(11).FontWeight(600).TextColor(muted)
		}
		for _, r := range day.Rows {
			r := r
			_, weight := taskTitleStyle(r.Status == core.TraceCompleted)
			parent := runtimeSubtaskTarget{chain: r.ChainID, trace: r.TraceID}
			children := s.childRows("day-list", parent)
			accessory := float32(0)
			if len(children) > 0 {
				accessory = subtaskDisclosureWidth(c, string(r.TraceID), len(actualSubtaskRows(s.runtime.Snapshot(), r.ChainID, r.TraceID, s.runtime.Today()))) + 12
			}
			borderInset := float32(0)
			if s.runtimeSelection == string(r.TraceID) {
				borderInset = 1
			}
			_, textH := dayMeasure(c, "day-row-measure-"+string(r.TraceID), w-40-64-accessory-borderInset, 13, weight, r.Title)
			headerHeight := max(float32(28), textH) + 16
			rowHeight := headerHeight
			if s.subtaskExpanded[r.TraceID] && len(children) > 0 {
				rowHeight += 11 + float32(len(children)-1)*subtaskRowSpacing
				for _, child := range children {
					rowHeight += s.subtaskRowHeight(c, w-40-52, "day-list", child)
				}
			}
			canComplete, _ := taskMenuCapabilities(graph, r, day.Today)
			ui.Box(c.Key("live-row-" + string(r.TraceID))).Height(rowHeight).Width(w - 40).Children(func() {
				s.liveDayRow(c, w-40, rowHeight, textH, r, canComplete)
				if s.subtaskExpanded[r.TraceID] && len(children) > 0 {
					ui.Box(c).Absolute().Left(40).Top(headerHeight + 1).Width(w - 40 - 52).Children(func() { s.liveSubtaskSection(c, w-40-52, "day-list", parent, false) })
				}
			})
		}
		if len(day.Rows) == 0 {
			ui.Box(c).Height(40)
			ui.Text(c, "这一天没有留下任务。").FontSize(12).TextColor(muted)
		}
	})
}
func (s *app) liveDateStrip(c *ui.Context, w float32, strip taskview.DateStrip) {
	cw := w / 14
	for i, cell := range strip.Cells {
		if cell.Date == nil {
			continue
		}
		cell := cell
		d := *cell.Date
		x := float32(i) * cw
		b := ui.ButtonBase(c.Key("day-strip-"+d.String())).Absolute().Left(x).Size(cw, 57).Label("日条选择日期 "+d.String()).FocusBind(&s.runtimeDayFocus, d.String()).FocusRing(true)
		b.Children(func() {
			ww, _ := dayMeasure(c, "strip-week", 0, 10, 400, weekdayNarrow(d))
			txt(c, (cw-ww)/2, 2, 10, weekdayNarrow(d), sourceVisual.text3, 400)
			col := ink
			if cell.IsSelected {
				box(c, (cw-24)/2, 18, 24, 24, blue, 12).Shadow(0, 2, 6, 0, blue.Alpha(.34))
				col = white
			} else if cell.IsToday {
				box(c, (cw-24)/2, 18, 24, 24, ui.Transparent, 12).Border(1.5, blue)
				col = blue
			}
			digit := fmt.Sprint(d.Day)
			tw, th := dayMeasure(c, "strip-number", 0, 12, 500, digit)
			txt(c, (cw-tw)/2, 18+(24-th)/2, 12, digit, col, 500).FontFeatures("tnum")
			if cell.Pending > 0 {
				dot := blue
				if cell.IsSelected {
					dot = white.Alpha(.9)
				}
				box(c, (cw-3)/2, 51, 3, 3, dot, 1.5)
			}
		})
		if b.Clicked() {
			s.selectRuntimeDate(d, true)
		}
	}
	color, width := line.Alpha(.62), float32(1)
	if ui.FocusedValue(c, &s.runtimeDayFocus) != "" {
		color, width = blue.Alpha(.72), 1.5
	}
	box(c, 0, 65, w, width, color, 0)
}
func (s *app) liveDayRow(c *ui.Context, w, h, textH float32, r taskview.DayRow, enabled bool) {
	id := string(r.TraceID)
	headerHeight := max(float32(28), textH) + 16
	children := s.childRows("day-list", runtimeSubtaskTarget{chain: r.ChainID, trace: r.TraceID})
	accessory := float32(0)
	if len(children) > 0 {
		accessory = subtaskDisclosureWidth(c, string(r.TraceID), len(actualSubtaskRows(s.runtime.Snapshot(), r.ChainID, r.TraceID, s.runtime.Today()))) + 12
	}
	borderInset := float32(0)
	if s.runtimeSelection == id {
		borderInset = 1
	}
	_, titleWeight := taskTitleStyle(r.Status == core.TraceCompleted)
	intrinsic, _ := dayMeasure(c, "day-row-intrinsic-"+id, 0, 13, titleWeight, r.Title)
	contentWidth := min(intrinsic, max(float32(1), w-64-accessory-borderInset))
	selected := s.runtimeSelection == id
	// Native menu choices are delivered on a later frame. Re-key the host on
	// navigation so a menu opened before leaving cannot act after returning to
	// the same trace. Labels remain stable for accessibility and testing.
	navigation := uint64(0)
	var window *titleWindow
	if s.titles != nil {
		window = s.titles.windows[s.titleWindowID]
		if window != nil {
			navigation = window.navigation
		}
	}
	b := ui.ButtonBase(c.Key(struct {
		Trace      string
		Navigation uint64
		Window     *titleWindow
	}{"day-select-" + id, navigation, window})).Absolute().Size(w, h).Label("查看任务 " + id).Radius(7).Background(white)
	if s.titles != nil {
		b.ContextMenu(func(m *ui.Menu) { s.liveTaskContextMenu(m, r) })
	}
	if selected {
		b.Background(blue.Alpha(.08)).Border(1, blue.Alpha(.22))
	} else if b.Hovered() {
		b.Background(sourceVisual.hover)
	}
	b.Children(func() {
		col, weight := taskTitleStyle(r.Status == core.TraceCompleted)
		if r.Status == core.TraceUnfinished {
			col = muted
		}
		t := txt(c, 52, (headerHeight-textH)/2, 13, r.Title, col, weight).Width(contentWidth)
		if r.Status == core.TraceCompleted || r.Status == core.TraceAbandoned {
			t.Strikethrough()
		}
	})
	if b.Clicked() {
		s.titleTransition(func() { s.runtimeSelection = id; s.runtimeDayRail = true })
	}
	check := ui.ButtonBase(c.Key("live-check-"+id)).Absolute().Left(12).Top((headerHeight-28)/2).Size(28, 28).Label("切换完成 " + id).Disabled(!enabled)
	check.Children(func() {
		circle := box(c, 5, 5, 18, 18, white, 9).Border(1.5, sourceVisual.line2)
		glyph := ""
		fg := white
		if r.Status == core.TraceCompleted {
			circle.Background(green).Border(0, green)
			glyph = "✓"
		} else if r.Status == core.TraceUnfinished {
			circle.Background(sourceRGB(1, .937, .922)).Border(0, ui.Transparent)
			glyph = "✕"
			fg = sourceVisual.warning
		}
		if glyph != "" {
			gw, gh := dayMeasure(c, "day-status-glyph", 0, 11, 700, glyph)
			txt(c, (28-gw)/2, (28-gh)/2, 11, glyph, fg, 700)
		}
	})
	if enabled && check.Clicked() {
		s.titleTransition(func() {
			s.runAction(fmt.Sprintf("complete:%s:%t", r.TraceID, r.Status != core.TraceCompleted), func() error { return s.runtime.SetCompleted(r.TraceID, r.Status != core.TraceCompleted) })
		})
	}
	if len(children) > 0 {
		disclosureWidth := accessory - 12
		b := ui.ButtonBase(c.Key("subtask-disclosure-"+id)).Absolute().Left(52+borderInset+contentWidth+12).Top((headerHeight-22)/2).Size(disclosureWidth, 22).Radius(11).Background(sourceVisual.accentSoft).Label("展开子任务 " + id)
		b.Children(func() {
			glyph := "chevron-right"
			if s.subtaskExpanded[r.TraceID] {
				glyph = "chevron-down"
			}
			icon(c, 7, 6.5, 9, glyph, blue)
			txt(c, 20, 3, 11, subtaskDisclosureText(len(actualSubtaskRows(s.runtime.Snapshot(), r.ChainID, r.TraceID, s.runtime.Today()))), blue, 600).FontFeatures("tnum")
		})
		if b.Clicked() {
			expanded := !s.subtaskExpanded[r.TraceID]
			s.titleTransition(func() {
				if s.subtaskExpanded == nil {
					s.subtaskExpanded = map[core.DayTraceID]bool{}
				}
				s.subtaskExpanded[r.TraceID] = expanded
			})
		}
	}
	if !selected {
		box(c, 44, h-1, w-44, 1, line.Alpha(.62), 0).PassThrough()
	}
}
func (s *app) liveDayDetail(c *ui.Context, w, h float32, day taskview.DayPage) {
	box(c, 0, 0, 1, h, line, 0)
	calendarPaneButton(c, "day-close-rail", w-38, 14, "››", "收起日待办详情", func() { s.titleTransition(func() { s.runtimeDayRail = false }) })
	var selected *taskview.DayRow
	for i := range day.Rows {
		if string(day.Rows[i].TraceID) == s.runtimeSelection {
			selected = &day.Rows[i]
			break
		}
	}
	if s.titleEditor != nil {
		selected = &s.titleEditor.row
	}
	if selected != nil {
		txt(c, 14, 18, 12, "任务详情", muted, 600)
		ui.Scroll(c.Key("day-task-detail-scroll")).Absolute().Left(14).Top(56).Size(w-28, h-70).Children(func() {
			s.liveTitle(c, w-28, *selected)
			ui.Box(c).Height(16)
			ui.Text(c, statusText(selected.Status)).FontSize(11.5).TextColor(sourceVisual.text3).Label("日任务详情状态")
			ui.Box(c).Height(10)
			ui.Text(c, day.Selected.String()).FontSize(11).TextColor(muted)
			ui.Box(c).Height(18)
			ui.Box(c).Size(w-28, 28).Children(func() {
				calendarHeaderButton(c, "day-show-review", 0, 0, 100, "返回日统计", "返回日统计", true, func() { s.titleTransition(func() { s.runtimeSelection = "" }) })
			})
			ui.Box(c).Height(14)
			parent := runtimeSubtaskTarget{chain: selected.ChainID, trace: selected.TraceID}
			s.liveSubtaskSection(c, w-28, "day", parent, s.canAddSubtask(parent))
			ui.Box(c).Height(24)
			ui.Text(c, "分类、完整 Markdown 与撤销恢复尚未接通；加权进度待对齐。").FontSize(10.5).TextColor(muted).Width(w - 28)
		})
		return
	}
	// ReviewRail uses a vertical flow, baseline-style hero row and five status
	// rows. Omit unavailable AI/editors, not factual zero-valued statuses.
	ui.Scroll(c.Key("day-review-scroll")).Absolute().Left(14).Top(48).Size(w-28, h-62).Children(func() {
		width := w - 28
		date := fmt.Sprintf("%d月%d日 周%s", day.Selected.Month, day.Selected.Day, weekdayNarrow(day.Selected))
		dw, dh := dayMeasure(c, "review-date-size", 0, 11, 400, date)
		_, hh := dayMeasure(c, "review-heading-size", 0, 11, 600, "每日复盘")
		ui.Box(c).Size(width, max(dh, hh)).Children(func() {
			txt(c, 0, 0, 11, "每日复盘", sourceVisual.text3, 600)
			txt(c, width-dw, 0, 11, date, sourceVisual.text3, 400)
		})
		if day.Review == nil {
			ui.Box(c).Height(24)
			ui.Text(c, "未来日尚不能生成复盘。").FontSize(12).TextColor(muted).Width(width)
			return
		}
		ui.Box(c).Height(16)
		col := blue
		if day.Stats.Total > 0 && day.Stats.CompletionRate == 100 {
			col = green
		}
		value := fmt.Sprintf("%d%%", day.Stats.CompletionRate)
		vw, vh := dayMeasure(c, "review-hero-size", 0, 22, 600, value)
		_, lh := dayMeasure(c, "review-hero-label-size", 0, 11, 600, "完成率")
		ui.Box(c).Size(width, vh).Children(func() {
			txt(c, 0, vh-lh, 11, "完成率", sourceVisual.text3, 600)
			txt(c, width-vw, 0, 22, value, col, 600).FontFeatures("tnum")
		})
		ui.Box(c).Height(16)
		ui.Text(c, "近 7 天完成率").FontSize(11).FontWeight(600).TextColor(sourceVisual.text3)
		ui.Box(c).Height(8)
		_, axisH := dayMeasure(c, "review-axis-size", 0, 8.5, 400, "一")
		ui.Box(c).Size(width, 26+4+axisH).Children(func() {
			cw := width / 7
			for i, p := range day.Review.Trend {
				x := float32(i) * cw
				barH, color := float32(3), sourceVisual.line2
				if p.Ratio != nil {
					barH = max(float32(3), 26*float32(*p.Ratio))
					color = sourceVisual.text3.Alpha(.55)
					if p.IsSelected {
						color = blue
					}
				}
				box(c, x+(cw-3)/2, 26-barH, 3, barH, color, 1.5)
				if p.Date != nil {
					label := weekdayNarrow(*p.Date)
					tw, _ := dayMeasure(c, "trend-week-"+fmt.Sprint(i), 0, 8.5, 400, label)
					axisColor := sourceVisual.text3
					if p.IsSelected {
						axisColor = blue
					}
					txt(c, x+(cw-tw)/2, 30, 8.5, label, axisColor, 400)
				}
			}
		})
		ui.Box(c).Height(16)
		ui.Box(c).Size(width, 1).Background(line)
		ui.Box(c).Height(16)
		ui.Text(c, "总任务").FontSize(11).FontWeight(600).TextColor(sourceVisual.text3).Label(fmt.Sprintf("日复盘总任务 %d", day.Stats.Total))
		ui.Box(c).Height(8)
		for i, item := range []struct {
			label string
			count int
			color ui.Color
		}{
			{"待完成", day.Stats.Pending, blue}, {"已完成", day.Stats.Completed, green},
			{"未完成", day.Stats.Unfinished, sourceVisual.warning}, {"已延期", day.Stats.Deferred, ink}, {"已废弃", day.Stats.Abandoned, ink},
		} {
			if i > 0 {
				ui.Box(c).Height(6)
			}
			value := fmt.Sprint(item.count)
			vw, vh := dayMeasure(c, "review-stat-size-"+fmt.Sprint(i), 0, 11.5, 500, value)
			_, lh := dayMeasure(c, "review-stat-label-"+fmt.Sprint(i), 0, 11.5, 400, item.label)
			color := item.color
			if item.count == 0 && i < 3 {
				color = sourceVisual.text3
			}
			ui.Box(c).Size(width, max(vh, lh)).Children(func() {
				txt(c, 0, 0, 11.5, item.label, sourceVisual.text3, 400)
				txt(c, width-vw, 0, 11.5, value, color, 500).FontFeatures("tnum")
			})
		}
		if day.Stats.Unfinished > 0 || (day.Selected == day.Today && day.Review.TomorrowTotal > 0) {
			ui.Box(c).Height(16)
			ui.Box(c).Size(width, 1).Background(line)
			ui.Box(c).Height(16)
			ui.Text(c, "信号与建议").FontSize(11).FontWeight(600).TextColor(sourceVisual.text3)
			ui.Box(c).Height(8)
			if day.Stats.Unfinished > 0 {
				ui.Text(c, fmt.Sprintf("有 %d 项未完成任务。", day.Stats.Unfinished)).FontSize(11.5).TextColor(sourceVisual.warning).Width(width)
				ui.Box(c).Height(8)
			}
			if day.Selected == day.Today && day.Review.TomorrowTotal > 0 {
				ui.Text(c, fmt.Sprintf("明日有 %d 项任务。", day.Review.TomorrowTotal)).FontSize(11.5).TextColor(ink).Width(width)
				ui.Box(c).Height(4)
				b := ui.ButtonBase(c.Key("day-view-tomorrow")).Size(90, 22).Label("查看明天")
				b.Children(func() { txt(c, 0, 0, 11.5, "查看明天", blue, 500) })
				if b.Clicked() {
					if d, e := s.runtime.Today().AddDays(1); e == nil {
						s.selectRuntimeDate(d, false)
					}
				}
			}
		}
		ui.Box(c).Height(16)
		ui.Box(c).Size(width, 1).Background(line)
		ui.Box(c).Height(16)
		ui.Text(c, "只读日统计。复盘编辑与 AI 尚未接通。").FontSize(10.5).TextColor(muted).Width(width)
	})
}
