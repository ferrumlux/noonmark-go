// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"errors"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	"sort"
	"strings"
	"time"
)

// The UI only sees a transaction boundary. A successful command means that its
// complete candidate graph has been verified and persisted, not merely queued.
const naturalDayFailureMessage = "日期变化未能安全保存，任务操作已暂停。请保留草稿并检查会话。"

type taskService interface {
	Snapshot() core.TaskGraph
	Today() core.LocalDate
	AddPool(string) error
	AddToday(string) error
	Schedule(core.TaskChainID, core.LocalDate) error
	SetCompleted(core.DayTraceID, bool) error
}

func newRuntimeApp(service taskService) *app {
	s := newApp()
	s.runtime = service
	s.runtimeDate = service.Today()
	s.runtimeCalendarDate = service.Today()
	s.runtimeAppliedDay = service.Today()
	return s
}

type liveRow struct {
	key, title string
	chain      core.TaskChainID
	trace      core.DayTraceID
	date       core.LocalDate
	status     core.TraceStatus
}

func currentTitle(g core.TaskGraph, chain core.TaskChainID, definition core.TaskDefinitionID) string {
	for _, d := range g.Definitions {
		if definition != "" && d.ID == definition || definition == "" && d.ChainID == chain && d.SupersededAt == nil {
			return d.Title
		}
	}
	return ""
}
func rowsFor(g core.TaskGraph, page string, date, today core.LocalDate) []liveRow {
	var rows []liveRow
	if page == "pool" {
		for _, c := range g.Chains {
			if c.State != core.ChainActive || c.CycleMembership != nil {
				continue
			}
			var latest *core.DayTrace
			pending := false
			for i := range g.Traces {
				t := &g.Traces[i]
				if t.ChainID != c.ID {
					continue
				}
				if t.Status == core.TracePending {
					pending = true
				}
				if latest == nil || t.ContentUpdatedAt.After(latest.ContentUpdatedAt) || t.ContentUpdatedAt.Equal(latest.ContentUpdatedAt) && (t.CreatedAt.After(latest.CreatedAt) || t.CreatedAt.Equal(latest.CreatedAt) && t.ID > latest.ID) {
					latest = t
				}
			}
			if !pending && (latest == nil || latest.Status == core.TraceReturnedToPool || latest.Status == core.TraceCancelledDraft) {
				rows = append(rows, liveRow{key: string(c.ID), chain: c.ID, title: currentTitle(g, c.ID, "")})
			}
		}
		return rows
	}
	traces := g.Traces
	if page == "day" {
		if k, e := core.NewTaskKernel(g); e == nil {
			traces = k.DayTodo(date)
		} else {
			return nil
		}
	}
	for _, t := range traces {
		include := page == "day" || page == "upcoming" && t.Date.Compare(today) > 0 && t.Status == core.TracePending || page == "completed" && t.Status == core.TraceCompleted || page == "unfinished" && t.Status == core.TraceUnfinished || page == "calendar" && t.Status.FormsDayHistory()
		if include {
			rows = append(rows, liveRow{key: string(t.ID), chain: t.ChainID, trace: t.ID, title: currentTitle(g, t.ChainID, t.DefinitionID), date: t.Date, status: t.Status})
		}
	}
	if page != "day" {
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].date != rows[j].date {
				return rows[i].date.Compare(rows[j].date) < 0
			}
			return rows[i].key < rows[j].key
		})
	}
	return rows
}
func (s *app) runCommand(fn func() error, clear bool) {
	if s.refreshDay != nil {
		if e := s.refreshDay(); e != nil {
			s.runtimeError = naturalDayFailureMessage
			s.runtimeNotice = ""
			return
		}
	}
	if err := fn(); err != nil {
		s.runtimeError = "操作未保存，界面仍保留原资料与草稿。"
		switch {
		case errors.Is(err, taskapp.ErrConflict):
			s.runtimeError = "会话已被另一窗口进程修改。请关闭并重新打开会话。"
		case errors.Is(err, taskapp.ErrIncomplete):
			s.runtimeError = "写入未完整确认。已停止修改，请保留会话供检查。"
		case errors.Is(err, taskapp.ErrLimit):
			s.runtimeError = "已到达开发会话容量限制，未写入新资料。"
		case errors.Is(err, core.ErrImmutableHistory):
			s.runtimeError = "这个日期的历史已锁定，不能改写。"
		case errors.Is(err, core.ErrFuturePlanCannotComplete):
			s.runtimeError = "只能完成今天的任务，历史与未来计划保持不变。"
		}
		s.runtimeNotice = ""
		return
	}
	s.runtimeError = ""
	s.runtimeNotice = "已保存到隔离开发会话"
	for _, w := range mygo.Windows() {
		w.Invalidate()
	}
	if clear {
		s.draft = ""
	}
}
func (s *app) submitLiveDraft(pool bool) {
	title := strings.TrimSpace(s.draft)
	if title == "" {
		s.runtimeError = "请输入任务标题。"
		return
	}
	if pool {
		s.runCommand(func() error { return s.runtime.AddPool(title) }, true)
	} else {
		s.runCommand(func() error { return s.runtime.AddToday(title) }, true)
	}
}
func (s *app) liveSidebar(c *ui.Context, h, sw float32, g core.TaskGraph) {
	box(c, 0, 0, sw, h, side, 0)
	box(c, sw, 0, .7, h, line, 0)
	if sw > 100 {
		txt(c, 20, 36, 11, "计划", muted, 600)
		txt(c, 20, 224, 11, "轨迹", muted, 600)
		txt(c, 20, 403, 11, "札记", muted, 600)
	}
	txt(c, sw-30, 37, 16, "‹‹", muted, 400)
	labelButton(c, "live-collapse", sw-35, 32, 28, 28, "收起侧栏", func() { s.compact = !s.compact })
	for _, n := range nav {
		n := n
		active := s.page == n.page
		inset, iconX := navInset, navInset+navLeading+(navIconFrame-navIconSize)/2
		if sw <= 100 {
			inset, iconX = navCompactInset, (sw-navIconSize)/2
		}
		navButton(c, "live-nav-"+n.page, inset, n.y, sw-2*inset, active, "切换到"+n.title, func() { s.page = n.page; s.runtimeSelection = ""; s.runtimeError = ""; s.runtimeNotice = "" }, func() {
			icon(c, iconX-inset, (navRowHeight-navIconSize)/2, navIconSize, n.icon, n.color)
			if sw > 100 {
				col, weight := navLabelStyle(active)
				txt(c, navLeading+navIconFrame+navTextGap, 7, navFontSize, n.title, col, weight)
				switch n.page {
				case "day", "pool", "upcoming", "completed", "unfinished":
					txt(c, sw-35-inset, 9, 11, fmt.Sprint(len(rowsFor(g, n.page, s.runtime.Today(), s.runtime.Today()))), sourceVisual.text3, 400).FontFeatures("tnum")
				}
			}
		})
	}
	if sw > 100 {
		txt(c, 20, h-48, 10.5, "隔离开发会话", blue, 600)
		txt(c, 20, h-29, 10, "不接入现有 Noonmark 数据", muted, 400)
	}
}
func liveAction(c *ui.Context, key string, x, y, w float32, title, label string, enabled bool, fn func()) {
	col := blue
	if !enabled {
		col = faint
	}
	txt(c, x, y+5, 11.5, title, col, 600)
	b := ui.ButtonBase(c.Key(key)).Absolute().Left(x-4).Top(y).Size(w, 28).Label(label).Disabled(!enabled)
	if enabled && b.Clicked() {
		fn()
	}
}
func (s *app) runtimeView(c *ui.Context) {
	if s.refreshDay != nil {
		if e := s.refreshDay(); e != nil {
			s.runtimeError = naturalDayFailureMessage
			s.runtimeNotice = ""
		} else if s.runtimeError == naturalDayFailureMessage {
			s.runtimeError = ""
		}
		c.After(time.Minute)
	}
	w, h := c.Size()
	if s.page == "quick-entry" {
		txt(c, 20, 15, 18, "快速记录", ink, 700)
		ui.TextInput(c.Key("live-capture-input"), &s.draft).Absolute().Left(20).Top(53).Size(w-40, 32).Label("开发任务标题").Placeholder("写入隔离会话的任务池").OnSubmit(func() { s.submitLiveDraft(true) })
		liveAction(c, "capture-save", w-112, 94, 94, "保存", "保存到任务池", true, func() { s.submitLiveDraft(true) })
		liveAction(c, "capture-cancel", 20, 94, 54, "取消", "取消快速记录", true, func() { s.dismissUtility("pool", true) })
		s.liveFeedback(c, 20, 130, w-40)
		return
	}
	if s.page == "calendar" {
		s.liveCalendarView(c, w, h)
		return
	}
	sw := float32(220)
	if s.compact {
		sw = 72
	}
	rw := float32(280)
	if w-sw < 650 {
		rw = 0
	}
	mw := w - sw - rw
	g := s.runtime.Snapshot()
	s.liveSidebar(c, h, sw, g)
	ui.Box(c).Absolute().Left(sw).Top(0).Size(mw, h).Clip().Children(func() { s.liveMain(c, mw, h, g) })
	if rw > 0 {
		ui.Box(c).Absolute().Left(w-rw).Top(0).Size(rw, h).Clip().Children(func() { s.liveRail(c, rw, h, g) })
	}
}
func (s *app) liveFeedback(c *ui.Context, x, y, w float32) {
	message := s.runtimeNotice
	col := muted
	if s.runtimeError != "" {
		message = s.runtimeError
		col = orange
	}
	if message != "" {
		txt(c, x, y, 11, message, col, 500).Width(w)
	}
}
func (s *app) liveMain(c *ui.Context, w, h float32, g core.TaskGraph) {
	title := s.page
	for _, n := range nav {
		if n.page == s.page {
			title = n.title
		}
	}
	if s.page == "day" {
		title = fmt.Sprintf("%d年%d月%d日", s.runtimeDate.Year, s.runtimeDate.Month, s.runtimeDate.Day)
	}
	txt(c, 20, 16, pageHeadingSize, title, ink, 600)
	supported := false
	switch s.page {
	case "day", "pool", "upcoming", "completed", "unfinished", "calendar":
		supported = true
	}
	if !supported {
		txt(c, 20, 60, 12, "此页面尚未连接领域与存储。", muted, 500)
		txt(c, 20, 86, 11.5, "当前可用：新增任务、任务池排期、今日完成与撤销。", muted, 400).Width(w - 40)
		return
	}
	top := float32(94)
	if s.page == "day" {
		liveAction(c, "prev-day", w-146, 17, 30, "‹", "前一天", true, func() {
			if d, e := s.runtimeDate.AddDays(-1); e == nil {
				s.runtimeDate = d
			}
		})
		liveAction(c, "today", w-107, 17, 55, "今天", "回到今天", true, func() { s.runtimeDate = s.runtime.Today() })
		liveAction(c, "next-day", w-45, 17, 25, "›", "后一天", true, func() {
			if d, e := s.runtimeDate.AddDays(1); e == nil {
				s.runtimeDate = d
			}
		})
	}
	canAdd := s.page == "pool" || s.page == "day" && s.runtimeDate == s.runtime.Today()
	if canAdd {
		placeholder := "添加今日任务"
		if s.page == "pool" {
			placeholder = "新建到任务池"
		}
		ui.TextInput(c.Key("live-task-input"), &s.draft).Absolute().Left(20).Top(59).Size(w-110, 32).Label("开发任务标题").Placeholder(placeholder).OnSubmit(func() { s.submitLiveDraft(s.page == "pool") })
		liveAction(c, "live-add", w-73, 61, 55, "添加", "添加任务", true, func() { s.submitLiveDraft(s.page == "pool") })
		top = 126
	} else {
		txt(c, 20, 54, 11.5, "依据当前隔离会话的真实任务事实显示。", muted, 400)
	}
	s.liveFeedback(c, 20, top-25, w-40)
	rows := rowsFor(g, s.page, s.runtimeDate, s.runtime.Today())
	if len(rows) == 0 {
		txt(c, 20, top+18, 12, "暂无任务", muted, 400)
		return
	}
	ui.Scroll(c.Key("live-scroll-"+s.page)).Absolute().Left(20).Top(top).Size(w-40, h-top-20).Children(func() {
		for _, r := range rows {
			r := r
			ui.Box(c.Key("live-row-" + r.key)).Height(65).Width(w - 40).Children(func() { s.liveTaskRow(c, w-40, r) })
		}
	})
}
func (s *app) liveTaskRow(c *ui.Context, w float32, r liveRow) {
	if s.runtimeSelection == r.key {
		box(c, 0, 0, w, 63, soft, 5)
	}
	complete := r.status == core.TraceCompleted
	if r.trace != "" {
		col := sourceVisual.line2
		if complete {
			col = green
		}
		circle := box(c, 9, 13, completionDiameter, completionDiameter, white, completionDiameter/2).Border(1.5, col)
		if complete {
			circle.Background(green)
			txt(c, 12, 11, 14, "✓", white, 700)
		}
		enabled := r.date == s.runtime.Today() && (r.status == core.TracePending || complete)
		b := ui.ButtonBase(c.Key("live-check-"+r.key)).Absolute().Left(4).Top(7).Size(28, 30).Label("切换完成 " + r.key).Disabled(!enabled)
		if enabled && b.Clicked() {
			s.runCommand(func() error { return s.runtime.SetCompleted(r.trace, !complete) }, false)
		}
	} else {
		icon(c, 10, 14, 16, "dotted", muted)
	}
	right := float32(35)
	if r.trace == "" {
		right = 175
	}
	titleColor, titleWeight := taskTitleStyle(complete)
	t := txt(c, 41, 12, dayTitleSize, r.title, titleColor, titleWeight).Width(w - 41 - right).SingleLine()
	if complete {
		t.Strikethrough()
	}
	status := "任务池"
	if r.trace != "" {
		status = r.date.String() + " · " + statusText(r.status)
	}
	txt(c, 41, 35, 10.5, status, muted, 400)
	labelButton(c, "live-select-"+r.key, 39, 5, w-41-right, 48, "查看任务 "+r.key, func() { s.runtimeSelection = r.key })
	if r.trace == "" {
		liveAction(c, "live-schedule-today-"+r.key, w-150, 15, 66, "排到今天", "排到今天 "+r.key, true, func() { s.runCommand(func() error { return s.runtime.Schedule(r.chain, s.runtime.Today()) }, false) })
		liveAction(c, "live-schedule-tomorrow-"+r.key, w-75, 15, 66, "排到明天", "排到明天 "+r.key, true, func() {
			d, e := s.runtime.Today().AddDays(1)
			if e == nil {
				s.runCommand(func() error { return s.runtime.Schedule(r.chain, d) }, false)
			}
		})
	}
	rule(c, 40, 64, w-40)
}
func statusText(s core.TraceStatus) string {
	switch s {
	case core.TracePending:
		return "待完成"
	case core.TraceCompleted:
		return "已完成"
	case core.TraceUnfinished:
		return "未完成"
	case core.TraceDeferred:
		return "已延期"
	case core.TraceChanged:
		return "已改期"
	case core.TraceReturnedToPool:
		return "已回池"
	case core.TraceCancelledDraft:
		return "取消草稿"
	case core.TraceAbandoned:
		return "已废弃"
	}
	return string(s)
}
func (s *app) liveRail(c *ui.Context, w, h float32, g core.TaskGraph) {
	box(c, 0, 0, .7, h, line, 0)
	txt(c, 17, 20, 12, "当前会话", muted, 600)
	for i, p := range []struct{ k, n string }{{"day", "今日任务"}, {"pool", "任务池"}, {"upcoming", "未来计划"}, {"completed", "已完成"}} {
		y := float32(61 + i*32)
		txt(c, 17, y, 12, p.n, muted, 500)
		txt(c, w-42, y, 13, fmt.Sprint(len(rowsFor(g, p.k, s.runtime.Today(), s.runtime.Today()))), ink, 600)
	}
	rule(c, 17, 203, w-34)
	txt(c, 17, 222, 12, "任务详情", muted, 600)
	var selected *liveRow
	for _, p := range []string{"pool", "calendar"} {
		for _, r := range rowsFor(g, p, s.runtimeDate, s.runtime.Today()) {
			if r.key == s.runtimeSelection {
				r := r
				selected = &r
			}
		}
	}
	if selected == nil {
		txt(c, 17, 253, 11.5, "选择任务查看其真实状态", faint, 400)
	} else {
		txt(c, 17, 253, 13, selected.title, ink, 600).Width(w - 34)
		if selected.trace != "" {
			txt(c, 17, 309, 11.5, selected.date.String()+" · "+statusText(selected.status), muted, 400)
		} else {
			txt(c, 17, 309, 11.5, "尚未排期", muted, 400)
		}
	}
	txt(c, 17, h-77, 10.5, "开发存储，不是 Swift 数据导入器。", muted, 400).Width(w - 34)
	txt(c, 17, h-48, 10.5, "未接通的页面不展示伪造任务。", muted, 400).Width(w - 34)
}

func (s *app) observeRuntimeDay(now func() time.Time) {
	boundary, ok := s.runtime.(interface{ AdvanceTo(core.LocalDate) error })
	if !ok {
		return
	}
	s.refreshDay = func() error {
		at := now()
		today := core.LocalDate{Year: at.Year(), Month: int(at.Month()), Day: at.Day()}
		if e := boundary.AdvanceTo(today); e != nil {
			return e
		}
		applied := s.runtime.Today()
		if applied != s.runtimeAppliedDay {
			if s.runtimeDate == s.runtimeAppliedDay {
				s.runtimeDate = applied
				s.runtimeSelection = ""
			}
			if s.runtimeCalendarDate == s.runtimeAppliedDay {
				s.runtimeCalendarDate = applied
				s.runtimeCalendarFocus = ""
			}
			s.runtimeAppliedDay = applied
			for _, w := range mygo.Windows() {
				w.Invalidate()
			}
		}
		return nil
	}
}
