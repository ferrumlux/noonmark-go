// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskview"
)

// CalendarPage.swift defines square slots. Component-specific geometry stays
// separate from navigation/card radii. These are logical points, not pixels.
const calendarGridTop float32 = 111

func (s *app) selectCalendarDate(date core.LocalDate, focus bool) {
	if !date.Valid() {
		return
	}
	s.runtimeCalendarDate = date
	if focus {
		s.runtimeCalendarFocus = date.String()
	} else {
		s.runtimeCalendarFocus = ""
	}
}

func (s *app) liveCalendarView(c *ui.Context, w, h float32) {
	if focus := ui.FocusedValue(c, &s.runtimeCalendarFocus); focus != "" {
		focusedDate, focusErr := core.ParseLocalDate(focus)
		for _, key := range []struct {
			key  ui.Key
			days int
		}{{ui.KeyLeft, -1}, {ui.KeyRight, 1}, {ui.KeyUp, -7}, {ui.KeyDown, 7}} {
			if focusErr == nil && c.Shortcut(0, key.key) {
				if d, e := taskview.MoveCalendarSelection(focusedDate, key.days); e == nil {
					s.selectCalendarDate(d, true)
				}
			}
		}
	}
	g := s.runtime.Snapshot()
	month, err := taskview.ProjectCalendar(g, s.runtimeCalendarDate, s.runtime.Today())
	sw := float32(220)
	if s.compact {
		sw = 72
	}
	rw := float32(0)
	if s.runtimeCalendarRail {
		rw = 248
	}
	mw := w - sw - rw
	s.liveSidebar(c, h, sw, g)
	if err != nil {
		txt(c, sw+18, 20, 20, "日历无法安全显示", ink, 600)
		txt(c, sw+18, 60, 12, "任务事实或日期未通过验证，未修改任何资料。", sourceVisual.warning, 400)
		return
	}
	ui.Box(c).Absolute().Left(sw).Top(0).Size(mw, h).Clip().Children(func() { s.liveCalendarMonth(c, mw, h, month) })
	if rw > 0 {
		ui.Box(c).Absolute().Left(w-rw).Top(0).Size(rw, h).Clip().Children(func() { s.liveCalendarRail(c, rw, h, month.SelectedDay) })
	}
}

func calendarHeaderButton(c *ui.Context, key string, x, y, w float32, title, label string, enabled bool, fn func()) {
	b := ui.ButtonBase(c.Key(key)).Absolute().Left(x).Top(y).Size(w, 28).Label(label).Disabled(!enabled).Radius(7).Background(sourceVisual.panel2)
	if b.Hovered() && enabled {
		b.Background(sourceVisual.chip)
	}
	b.Children(func() {
		color := sourceVisual.text2
		if !enabled {
			color = sourceVisual.placeholder
		}
		tw, _ := c.MeasureText(0, ui.Span{Text: title, Size: 12, Weight: 400})
		txt(c, (w-tw)/2, 5, 12, title, color, 400)
	})
	if enabled && b.Clicked() {
		fn()
	}
}

// PaneBoundaryToggle uses its own radius and transparent idle surface.
func calendarPaneButton(c *ui.Context, key string, x, y float32, title, label string, fn func()) {
	b := ui.ButtonBase(c.Key(key)).Absolute().Left(x).Top(y).Size(28, 28).Label(label).Radius(6)
	if b.Hovered() {
		b.Background(sourceVisual.hover).Border(1, line.Alpha(.5))
	}
	b.Children(func() { txt(c, 7, 4, 12, title, sourceVisual.text3, 600) })
	if b.Clicked() {
		fn()
	}
}

func (s *app) liveCalendarMonth(c *ui.Context, w, h float32, m taskview.CalendarMonth) {
	txt(c, 18, 16, 20, fmt.Sprintf("%d年%d月", m.Month.Year, m.Month.Month), ink, 600).FontFeatures("tnum")
	end := w - 18
	if !s.runtimeCalendarRail {
		calendarPaneButton(c, "calendar-open-rail", end-28, 16, "‹‹", "展开日历详情", func() { s.runtimeCalendarRail = true })
		end -= 36
	}
	calendarHeaderButton(c, "calendar-next", end-28, 16, 28, "›", "下个月", m.CanNext, func() {
		if d, e := taskview.ShiftCalendarMonth(s.runtimeCalendarDate, 1); e == nil {
			s.selectCalendarDate(d, false)
		}
	})
	calendarHeaderButton(c, "calendar-today", end-80, 16, 44, "今天", "日历回到今天", true, func() { s.selectCalendarDate(s.runtime.Today(), false) })
	calendarHeaderButton(c, "calendar-prev", end-116, 16, 28, "‹", "上个月", m.CanPrevious, func() {
		if d, e := taskview.ShiftCalendarMonth(s.runtimeCalendarDate, -1); e == nil {
			s.selectCalendarDate(d, false)
		}
	})
	extraTop := float32(0)
	if s.runtimeError != "" {
		txt(c, 18, 48, 11, s.runtimeError, sourceVisual.warning, 500).Width(w - 36).MaxLines(2)
		extraTop = 20
	} else {
		txt(c, 18, 48, 12, "整月轨迹总览。点选任一天，右侧查看当天详情。", muted, 400).Width(w - 36).SingleLine()
	}
	cw := (w - 18) / 7
	for i, label := range []string{"一", "二", "三", "四", "五", "六", "日"} {
		tw, _ := c.MeasureText(0, ui.Span{Text: label, Size: 11, Weight: 600})
		txt(c, 18+float32(i)*cw+(cw-tw)/2, 78+extraTop, 11, label, sourceVisual.text3, 600)
	}
	gridTop := calendarGridTop + extraTop
	rh := max(float32(88), (h-gridTop)/float32(m.RowCount))
	for _, slot := range m.Slots {
		x, y := 18+float32(slot.Column)*cw, gridTop+float32(slot.Row)*rh
		if slot.Day != nil {
			day := *slot.Day
			b := ui.ButtonBase(c.Key("calendar-date-"+day.Date.String())).Absolute().Left(x).Top(y).Size(cw, rh).Label("选择日期 "+day.Date.String()).FocusBind(&s.runtimeCalendarFocus, day.Date.String()).FocusRing(true).Clip()
			fill := sourceVisual.panel
			if day.IsSelected {
				fill = sourceVisual.accentSoft
			} else if b.Hovered() {
				fill = sourceVisual.panel2
			}
			b.Background(fill).Children(func() { calendarCellContents(c, cw, rh, day) })
			if b.Clicked() {
				s.selectCalendarDate(day.Date, true)
			}
		}
		// Empty slots retain the same topology and do not become date buttons.
		if slot.BottomBoundary {
			box(c, x, y+rh-1, cw, 1, line, 0).PassThrough()
		}
		if slot.TrailingBoundary {
			box(c, x+cw-1, y, 1, rh, line, 0).PassThrough()
		}
	}
}
func calendarStatusColors(status core.TraceStatus) (ui.Color, ui.Color, bool) {
	switch status {
	case core.TracePending:
		return blue, ink, false
	case core.TraceCompleted:
		return sourceVisual.success, sourceVisual.text3, true
	case core.TraceUnfinished:
		return sourceVisual.warning, sourceVisual.text2, false
	case core.TraceAbandoned:
		return sourceVisual.text3, sourceVisual.text3, true
	default:
		return sourceVisual.text3, sourceVisual.text2, false
	}
}
func calendarHeatColor(level int) ui.Color {
	switch level {
	case 1:
		return sourceVisual.success.Alpha(.28)
	case 2:
		return sourceVisual.success.Alpha(.46)
	case 3:
		return sourceVisual.success.Alpha(.68)
	case 4:
		return sourceVisual.success
	default:
		return sourceVisual.line2
	}
}

// Preserve the source's maximum six previews, but reserve a visible count in
// short six-row windows. This explicit height adaptation avoids hiding tasks
// behind clipping; the rail always contains every projected row.
func calendarPreviewFit(height float32, total int, rowHeight, footerHeight float32) (visible, hidden int) {
	pitch := max(float32(14), rowHeight)
	withoutMore := min(6, max(0, int((height-27+pitch-rowHeight)/pitch)))
	if total <= withoutMore {
		return total, 0
	}
	visible = min(total, min(6, max(0, int((height-27-footerHeight)/pitch))))
	return visible, total - visible
}

// MyGo MeasureText lays spans over the theme's base-font strut. Use a scoped
// matching base size so a 9.5-point span is measured like Text.FontSize(9.5),
// without mutating the window theme or painting a measurement widget.
func calendarMeasureHeight(c *ui.Context, key string, size float32, texts []string) float32 {
	theme := *c.Theme()
	theme.FontSize = size
	height := float32(0)
	ui.Themed(c.Key(key), &theme).Absolute().Size(0, 0).Background(ui.Transparent).IgnoreInput(true).Children(func() {
		for _, text := range texts {
			_, h := c.MeasureText(0, ui.Span{Text: text, Size: size, Weight: 400})
			height = max(height, h)
		}
	})
	return height
}

func calendarCellContents(c *ui.Context, w, h float32, day taskview.CalendarDay) {
	if day.IsSelected {
		box(c, 0, 0, 2, h, blue, 0)
	}
	color, weight := ink, 500
	if day.Total == 0 {
		color = sourceVisual.text3
	}
	if day.IsToday || day.IsSelected {
		weight = 700
	}
	if day.IsSelected {
		color = blue
	}
	dayText := fmt.Sprint(day.Date.Day)
	txt(c, 6, 4, 12.5, dayText, color, weight).FixedLineHeight(15).FontFeatures("tnum")
	if day.IsToday {
		dw, _ := c.MeasureText(0, ui.Span{Text: dayText, Size: 12.5, Weight: weight})
		box(c, 6+dw+4, 10, 4, 4, blue, 2)
	}
	if day.HasHeat {
		box(c, w-16, 6, 10, 10, calendarHeatColor(day.HeatLevel), 2.5)
	}
	titles := make([]string, 0, len(day.PreviewRows))
	for _, row := range day.PreviewRows {
		titles = append(titles, strings.NewReplacer("\n", " ", "\r", " ").Replace(row.Title))
	}
	rowHeight := max(float32(12), calendarMeasureHeight(c, "calendar-measure-preview", 9.5, titles))
	footerHeight := max(float32(11), calendarMeasureHeight(c, "calendar-measure-remainder", 9, []string{fmt.Sprintf("+%d", day.Total)}))
	pitch := max(float32(14), rowHeight)
	visible, hidden := calendarPreviewFit(h, day.Total, rowHeight, footerHeight)
	for i, row := range day.Rows[:visible] {
		y := float32(23) + pitch*float32(i)
		dot, title, strike := calendarStatusColors(row.Status)
		box(c, 8, y+4, 4, 4, dot, 2)
		text := txt(c, 18, y, 9.5, row.Title, title, 400).Width(w - 24).SingleLine()
		if strike {
			text.Strikethrough()
		}
	}
	if hidden > 0 {
		txt(c, 13, 23+pitch*float32(visible), 9, fmt.Sprintf("+%d", hidden), sourceVisual.text3, 400)
	}
}
func (s *app) liveCalendarRail(c *ui.Context, w, h float32, day taskview.CalendarDay) {
	box(c, 0, 0, 1, h, line, 0)
	calendarPaneButton(c, "calendar-close-rail", w-36, 14, "››", "收起日历详情", func() { s.runtimeCalendarRail = false })
	txt(c, 16, 18, 15, fmt.Sprintf("%d年%d月%d日", day.Date.Year, day.Date.Month, day.Date.Day), ink, 700).FontFeatures("tnum")
	weekday := int(time.Date(day.Date.Year, time.Month(day.Date.Month), day.Date.Day, 12, 0, 0, 0, time.UTC).Weekday())
	scope, bg, fg := "今天", blue, white
	if day.Position == taskview.CalendarHistory {
		scope, bg, fg = "历史", sourceVisual.chip, muted
	} else if day.Position == taskview.CalendarFuture {
		scope, bg, fg = "未来", soft, blue
	}
	box(c, 16, 47, 42, 20, bg, 10)
	txt(c, 25, 49, 11, scope, fg, 600)
	txt(c, 68, 49, 11, fmt.Sprintf("周%s · %d 项任务", []string{"日", "一", "二", "三", "四", "五", "六"}[weekday], day.Total), sourceVisual.text3, 400)
	open := ui.ButtonBase(c.Key("calendar-open-day")).Absolute().Left(16).Top(76).Size(w-32, 28).Label("在 Day Todo 打开这一天")
	open.Children(func() { txt(c, 0, 4, 11.5, "在 Day Todo 打开这一天 →", blue, 500) })
	if open.Clicked() {
		s.runtimeDate = day.Date
		s.page = "day"
		s.runtimeSelection = ""
	}

	box(c, 0, 116, w, 1, line.Alpha(.62), 0)
	ui.Scroll(c.Key("calendar-detail-scroll-"+day.Date.String())).Absolute().Left(16).Top(128).Size(w-32, h-140).Children(func() {
		ui.Box(c).Width(w - 32).Height(64).Children(func() {
			txt(c, 0, 0, 11, fmt.Sprintf("待完成 %d · 已完成 %d · 未完成 %d", day.Pending, day.Completed, day.Unfinished), muted, 400).Width(w - 32)
			txt(c, 0, 29, 10.5, "普通任务只读详情；完整洞察尚未接通。", sourceVisual.text3, 400).Width(w - 32)
		})
		if day.Total == 0 {
			ui.Text(c, "这一天没有留下任务。").FontSize(12).TextColor(muted)
		}
		for _, row := range day.Rows {
			row := row
			dot, color, strike := calendarStatusColors(row.Status)
			status := statusText(row.Status)
			chipTextWidth, _ := c.MeasureText(0, ui.Span{Text: status, Size: 10.5, Weight: 500})
			chipWidth := 21 + chipTextWidth
			titleWidth := w - 32 - 20 - 8 - chipWidth
			_, titleHeight := c.MeasureText(titleWidth, ui.Span{Text: row.Title, Size: 12.5, Weight: 500})
			rowHeight := max(float32(18), min(titleHeight, float32(36))) + 16
			ui.Box(c.Key("calendar-detail-" + string(row.TraceID))).Height(rowHeight + 10).Width(w - 32).Children(func() {
				t := txt(c, 10, 8, 12.5, row.Title, color, 500).Width(titleWidth).MaxLines(2)
				if strike {
					t.Strikethrough()
				}
				bg := sourceVisual.chip
				switch row.Status {
				case core.TracePending:
					bg = soft
				case core.TraceCompleted:
					bg = sourceRGB(.91, .98, .95)
				case core.TraceUnfinished:
					bg = sourceRGB(1, .937, .922)
				}
				x := w - 42 - chipWidth
				box(c, x, 8, chipWidth, 18, bg, 9)
				box(c, x+6, 15, 4, 4, dot, 2)
				txt(c, x+14, 9, 10.5, status, dot, 500)
				box(c, 10, rowHeight-1, w-52, 1, line.Alpha(.62), 0)
			})
		}
	})
}
