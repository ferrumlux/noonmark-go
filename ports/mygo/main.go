// Noonmark UI study implemented with MyGo's native Go UI toolkit.
// This is a UI-only prototype. No HTML, JavaScript, database, network or AI logic.
package main

import (
	"flag"
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

var (
	white  = ui.Hex("#ffffff")
	side   = ui.Hex("#fafafa")
	ink    = ui.Hex("#202124")
	muted  = ui.Hex("#777782")
	faint  = ui.Hex("#bfbfc3")
	line   = ui.Hex("#e7e7eb")
	blue   = ui.Hex("#2b61cf")
	soft   = ui.Hex("#eff4ff")
	teal   = ui.Hex("#009f95")
	orange = ui.Hex("#e98616")
	purple = ui.Hex("#8656ff")
	green  = ui.Hex("#087b57")
	pink   = ui.Hex("#df4181")
)

type app struct {
	page              string
	draft             string
	selectedDay       int
	done              map[int]bool
	compact           bool
	detail            bool
	openUtility       func(string)
	closeWindow       func()
	runtime           taskService
	runtimeError      string
	runtimeNotice     string
	runtimeSelection  string
	runtimeDate       core.LocalDate
	runtimeAppliedDay core.LocalDate
	refreshDay        func() error
}

func newApp() *app { return &app{page: "day", selectedDay: 24, done: map[int]bool{}, detail: true} }
func txt(c *ui.Context, x, y, size float32, s string, col ui.Color, weight int) ui.Element {
	return ui.Text(c, s).Absolute().Left(x).Top(y).FontSize(size).FontWeight(weight).TextColor(col)
}
func box(c *ui.Context, x, y, w, h float32, col ui.Color, r float32) ui.Element {
	return ui.Box(c).Absolute().Left(x).Top(y).Size(w, h).Background(col).Radius(r)
}
func rule(c *ui.Context, x, y, w float32) { box(c, x, y, w, 0.7, line, 0) }
func labelButton(c *ui.Context, key string, x, y, w, h float32, label string, fn func()) {
	b := ui.ButtonBase(c.Key(key)).Absolute().Left(x).Top(y).Size(w, h).Label(label).Radius(7)
	if b.Hovered() {
		b.Border(0.7, blue.Mix(white, 0.65))
	}
	if b.Clicked() && fn != nil {
		fn()
	}
}
func pill(c *ui.Context, x, y, w float32, s string, col, bg ui.Color) {
	box(c, x, y, w, 22, bg, 12)
	txt(c, x+8, y+2, 11, s, col, 600)
}
func badge(c *ui.Context, x, y float32, s string, col ui.Color) {
	switch s {
	case "发布":
		col = orange
	case "阻断":
		col = pink
	case "体验":
		col = purple
	case "交接", "同步":
		col = blue
	case "洞察":
		col = teal
	}
	bg := col.Mix(white, 0.92)
	w := float32(21 + len([]rune(s))*10)
	box(c, x, y, w, 21, bg, 3).Border(0.7, line)
	txt(c, x+5, y+2, 10.5, "#", col, 700)
	txt(c, x+16, y+2, 10.5, s, muted, 600)
}
func group(c *ui.Context, x, y float32, title string, count int, col ui.Color) {
	icon(c, x, y+2, 14, "folder", col)
	txt(c, x+24, y, 12, title, col, 600)
	txt(c, x+56, y+1, 10.5, fmt.Sprint(count), muted, 400)
}

var paths = map[string]string{
	"dotted":   `<circle cx="12" cy="12" r="10.5" stroke-dasharray="2.5 2.5"/>`,
	"clock":    `<circle cx="12" cy="12" r="9"/><path d="M12 6v7H7"/>`,
	"tray":     `<path d="M3 10l5-6h8l5 6v9H3zM3 10h5l2 4h4l2-4h5"/>`,
	"calendar": `<rect x="4" y="4" width="16" height="17" rx="2"/><path d="M4 9h16M8 3v4m8-4v4M8 12h1m3 0h1m3 0h1M8 16h1m3 0h1m3 0h1"/>`,
	"repeat":   `<path d="M3 9V6h16l-3-3m3 3l-3 3M21 15v3H5l3 3m-3-3l3-3"/>`,
	"alert":    `<circle cx="12" cy="12" r="9"/><path d="M12 6v7m0 3v1"/>`,
	"check":    `<path d="M12 2l3 3h4v4l3 3-3 3v4h-4l-3 3-3-3H5v-4l-3-3 3-3V5h4zM7 12l3 3 6-7"/>`,
	"spark":    `<path d="M12 3l2 6 6 3-6 2-2 7-2-7-6-2 6-3zM4 2v5m-2-2h5"/>`,
	"note":     `<rect x="4" y="4" width="16" height="16" rx="2"/><path d="M7 8h10M7 12h10M7 16h6"/>`,
	"bulb":     `<path d="M8 17c0-5-3-6-3-9a7 7 0 0114 0c0 3-3 5-3 9zM8 20h8m-6 3h4"/>`,
	"folder":   `<path d="M2 5h7l3 3h10v13H2z" fill="currentColor" stroke="none"/><path d="M2 10h20" stroke="white" stroke-width="1"/>`,
	"search":   `<circle cx="10" cy="10" r="6"/><path d="M15 15l5 5"/>`,
	"pin":      `<path d="M9 3h7l-1 6 4 4-5 1-6 7 2-8-5-2 5-3z"/>`,
}
var svg = map[string]*ui.SVG{}

func init() {
	for k, p := range paths {
		svg[k] = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + p + `</svg>`))
	}
}
func icon(c *ui.Context, x, y, size float32, name string, col ui.Color) {
	if im := svg[name]; im != nil {
		ui.Icon(c, im).Absolute().Left(x).Top(y).Size(size, size).TextColor(col)
	}
}

type navitem struct {
	title, page, icon, count string
	y                        float32
	color                    ui.Color
}

var nav = []navitem{
	{"Day Todo", "day", "clock", "22", 66, blue}, {"任务池", "pool", "tray", "32", 103, teal}, {"未来计划", "upcoming", "calendar", "23", 140, purple}, {"重复计划", "recurring", "repeat", "12", 177, orange},
	{"未完成", "unfinished", "alert", "15", 245, orange}, {"已完成", "completed", "check", "460", 282, green}, {"日历", "calendar", "calendar", "", 319, pink}, {"烛龙", "ai", "spark", "", 356, purple},
	{"Sticky Note", "sticky", "note", "", 424, ui.Hex("#c74006")}, {"飞光", "flylight", "bulb", "", 461, ui.Hex("#cb8900")},
}

func (s *app) sidebar(c *ui.Context, h, sw float32) {
	box(c, 0, 0, sw, h, side, 0)
	box(c, sw, 0, .7, h, ui.Hex("#d6d6d9"), 0)
	if sw > 100 {
		txt(c, 20, 36, 11, "计划", muted, 600)
		txt(c, 20, 224, 11, "轨迹", muted, 600)
		txt(c, 20, 403, 11, "札记", muted, 600)
	}
	txt(c, sw-30, 37, 16, "‹‹", muted, 400)
	labelButton(c, "collapse", sw-35, 32, 28, 28, "收起侧栏", func() { s.compact = !s.compact })
	for _, n := range nav {
		n := n
		if navPage(s.page) == n.page {
			box(c, 9, n.y, sw-18, 34, soft, 9)
		}
		icon(c, 24, n.y+10, 16, n.icon, n.color)
		if sw > 100 {
			col := muted
			if navPage(s.page) == n.page {
				col = ink
			}
			txt(c, 52, n.y+7, 15, n.title, col, 600)
			if n.count != "" {
				txt(c, sw-35, n.y+9, 11, n.count, muted, 400)
			}
		}
		labelButton(c, "nav-"+n.page, 9, n.y, sw-18, 34, "切换到"+n.title, func() { s.page = n.page; s.draft = "" })
	}
}
func (s *app) dismissUtility(fallback string, clearDraft bool) {
	if clearDraft {
		s.draft = ""
	}
	if s.closeWindow != nil {
		s.closeWindow()
		return
	}
	s.page = fallback
}
func (s *app) view(c *ui.Context) {
	if c.Shortcut(ui.Cmd, ui.KeyComma) && s.openUtility != nil {
		s.openUtility("settings-general")
	}
	if c.Shortcut(ui.Cmd, ui.KeyN) && s.openUtility != nil {
		s.openUtility("quick-entry")
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyI) && s.openUtility != nil {
		s.openUtility("flylight-quick-capture")
	}
	if c.Shortcut(0, ui.KeyEscape) && s.closeWindow != nil && (strings.HasPrefix(s.page, "settings-") || s.page == "quick-entry" || s.page == "flylight-quick-capture") {
		s.closeWindow()
		return
	}
	t := ui.LightTheme()
	t.Background = white
	t.Text = ink
	t.TextMuted = muted
	t.Accent = blue
	t.FontSize = 12
	if runtime.GOOS == "linux" {
		t.Font = "Noto Sans CJK SC"
	}
	c.SetTheme(t)
	if s.runtime != nil {
		s.runtimeView(c)
		return
	}
	if strings.HasPrefix(s.page, "settings-") {
		s.settingsView(c)
		return
	}
	if s.page == "quick-entry" || s.page == "flylight-quick-capture" {
		s.captureView(c)
		return
	}
	w, h := c.Size()
	sw := float32(220)
	if s.compact {
		sw = 72
	}
	rw := float32(280)
	if s.page == "calendar" || s.page == "sticky" || s.page == "sticky-stream" || s.page == "ai" || !s.detail {
		rw = 0
	}
	ui.Box(c).Fill().Background(white).Clip().Children(func() {
		s.sidebar(c, h, sw)
		mw := w - sw - rw
		ui.Box(c).Absolute().Left(sw).Top(0).Size(mw, h).Clip().Children(func() {
			switch s.page {
			case "day":
				s.day(c, mw, h)
			case "pool":
				s.pool(c, mw, h)
			case "calendar":
				s.calendar(c, mw, h)
			case "upcoming":
				s.upcoming(c, mw, h)
			case "recurring":
				s.recurring(c, mw, h)
			case "unfinished":
				s.unfinished(c, mw, h)
			case "completed":
				s.completed(c, mw, h)
			case "flylight", "flylight-edit":
				s.flylight(c, mw, h)
			case "sticky", "sticky-stream":
				s.sticky(c, mw, h)
			case "ai":
				s.ai(c, mw, h)
			default:
				s.stub(c, mw, h)
			}
		})
		if rw > 0 {
			ui.Box(c).Absolute().Left(w-rw).Top(0).Size(rw, h).Clip().Children(func() {
				switch s.page {
				case "upcoming", "recurring", "unfinished", "completed":
					s.planRail(c, rw, h)
				case "flylight", "flylight-edit":
					s.memoRail(c, rw, h)
				default:
					s.rail(c, rw, h)
				}
			})
		}
	})
}
func (s *app) input(c *ui.Context, x, y, w float32, placeholder string) {
	ui.TextInputBase(c.Key("quick-input"), &s.draft).Absolute().Left(x).Top(y).Size(w, 32).Padding(7, 12).FontSize(12).Background(ui.Hex("#fbfbfc")).Radius(8).Border(1, line).Placeholder(placeholder).PlaceholderColor(faint).Label("快速输入")
}
func toolbar(c *ui.Context, w, y float32, calendar bool) {
	if calendar {
		for i, text := range []string{"‹", "今天", "›"} {
			x := w - 170 + float32(i)*44
			bw := float32(28)
			if i == 1 {
				bw = 44
			}
			if i == 2 {
				x += 16
			}
			box(c, x, y, bw, 28, ui.Hex("#fbfbfc"), 8)
			tx := x + 11
			if i == 1 {
				tx = x + 10
			}
			txt(c, tx, y+5, 13, text, muted, 500)
		}
		return
	}
	txt(c, w-198, y+7, 13, "⇅ 视图⌄", ink, 500)
	for i, text := range []string{"‹", "›"} {
		x := w - 148 + float32(i)*36
		box(c, x, y, 28, 28, ui.Hex("#fbfbfc"), 8)
		txt(c, x+11, y+5, 15, text, muted, 500)
	}
	box(c, w-76, y, 56, 28, ui.Hex("#fbfbfc"), 8)
	txt(c, w-66, y+6, 12, "选日期", muted, 500)
}

func (s *app) day(c *ui.Context, w, h float32) {
	txt(c, 20, 15, 21, "2026年7月24日", ink, 700)
	txt(c, 173, 23, 11, "周五", muted, 500)
	box(c, 205, 25, 37, 18, blue, 10)
	txt(c, 213, 25, 10.5, "今日", white, 600)
	toolbar(c, w, 16, false)
	weekdays := []string{"六", "日", "一", "二", "三", "四", "五", "六", "日", "一", "二", "三", "四", "五"}
	gap := (w - 40) / 14
	for i := 0; i < 14; i++ {
		x := 20 + float32(i)*gap
		d := 18 + i
		txt(c, x+18, 57, 10.5, weekdays[i], muted, 400)
		if d == s.selectedDay {
			box(c, x+11, 74, 25, 25, blue, 13)
		}
		col := ink
		if d == s.selectedDay {
			col = white
		}
		txt(c, x+15, 78, 12, fmt.Sprint(d), col, 600)
		if d > 24 {
			box(c, x+22, 106, 3, 3, blue, 2)
		}
		labelButton(c, fmt.Sprintf("date-%d", d), x, 72, gap, 29, fmt.Sprintf("7月%d日", d), func() { s.selectedDay = d })
	}
	rule(c, 20, 121, w-40)
	s.input(c, 20, 146, w-150, "添加今日任务，可用 #标签、@分组、/重复")
	box(c, w-122, 146, 102, 29, ui.Hex("#fbfbfc"), 8)
	txt(c, w-112, 153, 12, "从任务池排期…", muted, 500)
	icon(c, 20, 203, 15, "pin", ui.Hex("#c72b30"))
	txt(c, 44, 203, 10.5, "2", muted, 400)
	s.task(c, 0, 56, 228, w, "修复登录阻断问题", "工程", "阻断", teal, 0, false)
	s.task(c, 1, 56, 286, w, "审阅发布说明", "产品", "发布", blue, 0, false)
	group(c, 20, 349, "复盘", 1, orange)
	s.task(c, 2, 56, 376, w, "每日产品复盘", "", "发布", orange, 1, false)
	txt(c, 157, 378, 10.5, "重复 · 每天", muted, 500)
	group(c, 20, 439, "工程", 2, teal)
	s.task(c, 3, 56, 465, w, "完善设置页交互", "", "体验", purple, 3, false)
	box(c, 72, 515, 150, 4, ui.Hex("#f0f0f3"), 3)
	box(c, 72, 515, 25, 4, blue, 3)
	txt(c, 227, 509, 10.5, "17%", blue, 600)
	s.task(c, 4, 56, 541, w, "完成晨间同步", "", "同步", blue, 3, true)
	txt(c, 20, 602, 12, "未分组", ink, 600)
	txt(c, 64, 604, 10.5, "19", muted, 400)
	s.task(c, 5, 56, 636, w, "年度工作日整理", "", "", blue, 1, false)
	txt(c, 170, 638, 10.5, "重复 · 工作日", muted, 500)
	s.task(c, 6, 56, 680, w, "准备发布演示（已确认范围）", "", "", blue, 3, false)
	s.task(c, 7, 56, 724, w, "准备发布演示（已确认范围）", "", "", blue, 3, false)
}
func (s *app) task(c *ui.Context, id int, x, y, w float32, title, grp, tag string, col ui.Color, sub int, complete bool) {
	checked := complete || s.done[id]
	cy := y + 12
	if tag == "" {
		cy = y + 1
	}
	if id == 3 {
		cy = y + 20
	}
	circle := box(c, x-19, cy, 18, 18, white, 9).Border(1.5, ui.Hex("#cdced6"))
	if checked {
		circle.Background(green).Border(0, green)
		txt(c, x-15, cy-1, 14, "✓", white, 700)
	}
	labelButton(c, fmt.Sprintf("check-%d", id), x-24, cy-5, 28, 28, "完成 "+title, func() { s.done[id] = !s.done[id] })
	tc := ink
	if checked {
		tc = ui.Hex("#a3a3ad")
	}
	tt := txt(c, x+16, y, 12.5, title, tc, 600)
	if checked {
		tt.Strikethrough()
	}
	bx := x + 16
	if grp != "" {
		box(c, bx, y+21, 48, 20, col.Mix(white, .91), 3)
		icon(c, bx+5, y+25, 11, "folder", col)
		txt(c, bx+21, y+23, 10.5, grp, col, 600)
		bx += 54
	}
	if tag != "" {
		badge(c, bx, y+21, tag, col)
	}
	if sub > 0 {
		sx := x + 16 + float32(len([]rune(title)))*12 + 8
		if sx > w-130 {
			sx = w - 130
		}
		if tag != "" {
			sx += 30
		}
		sy := y + 10
		switch id {
		case 2:
			sx = 222
		case 3:
			sx = 262
			sy = y + 19
		case 4:
			sx = 163
		case 5:
			sx = 245
			sy = y - 2
		case 6, 7:
			sx = 252
			sy = y - 2
		}
		pill(c, sx, sy, 69, fmt.Sprintf("› %d 子任务", sub), blue, soft)
	}
	ry := y + 49
	if id == 3 {
		ry = y + 66
	}
	if tag == "" {
		ry = y + 28
	}
	rule(c, x+8, ry, w-x-28)
}
func (s *app) pool(c *ui.Context, w, h float32) {
	txt(c, 20, 14, 21, "任务池", ink, 700)
	txt(c, 20, 44, 12, "尚未安排日期的任务。排期后会出现在对应的 Day Todo。", muted, 500)
	txt(c, w-82, 27, 13, "⇅ 视图⌄", ink, 500)
	s.input(c, 20, 83, w-40, "新建到任务池，可用 #标签、@分组、/重复")
	type g struct {
		name, tag string
		col       ui.Color
		nums      []int
	}
	gs := []g{{"产品", "发布", blue, []int{2, 6, 10}}, {"协作", "交接", orange, []int{4, 8, 12}}, {"复盘", "发布", orange, []int{0}}, {"工程", "体验", teal, []int{3, 7, 11}}, {"研究", "洞察", purple, []int{1, 5, 9}}}
	y := float32(139)
	for _, g := range gs {
		group(c, 20, y, g.name, len(g.nums), g.col)
		y += 32
		for _, n := range g.nums {
			title := fmt.Sprintf("年度第 %d 轮：整理完整月度输入", n)
			if n == 0 {
				title = "规划发布后复盘"
			}
			icon(c, 32, y-3, 18, "dotted", ui.Hex("#cdced6"))
			txt(c, 60, y-3, 12.5, title, ink, 600)
			bx := float32(253)
			if n >= 10 {
				bx += 6
			}
			if n == 0 {
				bx = 160
			}
			badge(c, bx, y-5, g.tag, g.col)
			rule(c, 60, y+24, w-80)
			y += 38
		}
		y -= 1
	}
}
func (s *app) rail(c *ui.Context, w, h float32) {
	box(c, 0, 0, .7, h, ui.Hex("#dddddf"), 0)
	txt(c, 17, 6, 16, "››", muted, 400)
	icon(c, 45, 12, 14, "search", muted)
	txt(c, 63, 10, 12, "搜索", ui.Hex("#999999"), 500)
	if s.page == "pool" {
		s.poolRail(c, w)
		return
	}
	if s.page != "day" {
		txt(c, 14, 53, 12, "详情", muted, 600)
		txt(c, 14, 83, 12, "请选择一个项目", faint, 400)
		return
	}
	txt(c, 14, 50, 11, "每日复盘", muted, 600)
	txt(c, 197, 50, 11, "7月24日 周五", muted, 400)
	icon(c, 16, 86, 13, "spark", blue)
	txt(c, 38, 84, 12, "让烛龙分析今日", ink, 600)
	txt(c, 259, 82, 16, "›", muted, 400)
	txt(c, 14, 130, 11, "完成率", muted, 600)
	txt(c, 232, 118, 24, "4%", blue, 700)
	txt(c, 14, 162, 11, "近 7 天完成率", muted, 600)
	for i, d := range []string{"六", "日", "一", "二", "三", "四", "五"} {
		x := float32(31 + i*36)
		bh := float32(3)
		if i == 1 || i == 5 {
			bh = 13
		}
		if i == 3 {
			bh = 9
		}
		col := ui.Hex("#b4b4bb")
		if i == 6 {
			col = blue
		}
		box(c, x, 210-bh, 3, bh, col, 2)
		txt(c, x-2, 214, 9, d, muted, 400)
	}
	rule(c, 14, 241, 252)
	txt(c, 14, 257, 11, "总任务", muted, 600)
	labels := []string{"待完成", "已完成", "未完成", "已延期", "已废弃"}
	vals := []string{"22", "1", "0", "1", "0"}
	for i, l := range labels {
		y := float32(279 + i*20)
		txt(c, 14, y, 11, l, muted, 500)
		col := ink
		if i == 0 {
			col = blue
		}
		if i == 1 {
			col = green
		}
		txt(c, 251, y, 11, vals[i], col, 600)
	}
	rule(c, 14, 390, 252)
	txt(c, 14, 407, 11, "信号与建议", muted, 600)
	txt(c, 14, 429, 12, "•", blue, 700)
	txt(c, 26, 428, 11.5, "明天已排 2 项任务。", ink, 400)
	txt(c, 26, 447, 11.5, "查看明天", blue, 600)
	rule(c, 14, 477, 252)
	txt(c, 14, 493, 11, "今日总结", muted, 600)
	box(c, 14, 511, 252, 92, ui.Hex("#fbfbfc"), 8)
	txt(c, 24, 519, 11.5, "今天完成了演示准备，并保留置顶、延期和\n进行中任务供继续体验。", ink, 400).Width(232)
	txt(c, 14, 618, 11, "未完成原因", muted, 600)
	box(c, 14, 637, 252, 92, ui.Hex("#fbfbfc"), 8)
	txt(c, 26, 644, 11, "哪些没完成，真实原因是什么？", faint, 400)
	txt(c, 14, 745, 11, "明日注意事项", muted, 600)
}
func (s *app) poolRail(c *ui.Context, w float32) {
	txt(c, 14, 50, 11, "任务池统计", muted, 600)
	txt(c, 14, 68, 11.5, "当前任务池的本地事实，不依赖 Provider。", muted, 500)
	txt(c, 14, 105, 11, "任务总量", muted, 600)
	txt(c, 238, 93, 24, "32", blue, 700)
	vals := []string{"5", "4", "19", "32", "13", "14"}
	names := []string{"池内分组", "池内标签", "未分组", "有说明", "已拆分", "重新回池"}
	for i, v := range vals {
		x := float32(14 + (i%2)*130)
		y := float32(130 + (i/2)*42)
		txt(c, x, y, 16, v, ink, 600)
		txt(c, x, y+21, 10.5, names[i], muted, 400)
	}
	txt(c, 14, 258, 10.5, "停留最久  年度第 1 轮：整理完整月…", muted, 500)
	txt(c, 229, 258, 11, "375 天", ink, 500)
	rule(c, 14, 289, 252)
	txt(c, 14, 305, 11, "烛龙分析", muted, 600)
	txt(c, 14, 323, 11.5, "基于任务内容与已授权轨迹，生成可审查\n的判断。", muted, 500).Width(252)
	txt(c, 14, 365, 10.5, "2026年7月24日 下午4:30", muted, 400)
	txt(c, 14, 386, 11, "「年度第 1 轮：整理完整月度输入」的完成边界\n仍可更具体。", ink, 600).Width(252).FixedLineHeight(16)
	txt(c, 14, 422, 10.5, "证据：年度第 1 轮：整理完整月度输入", muted, 400)
	txt(c, 14, 439, 10, "置信度：中 · 不确定性：当前判断只依据任务池里已有\n的标题、说明、附言与计划子任务。", muted, 400).Width(252).FixedLineHeight(14)
	txt(c, 14, 471, 10.5, "补充可观察的交付物或完成标准，再决定是否安\n排日期。", blue, 500).Width(252).FixedLineHeight(14)
	txt(c, 14, 509, 11, "重新分析", blue, 600)
	txt(c, 72, 509, 11, "查看会话", muted, 600)
}
func (s *app) calendar(c *ui.Context, w, h float32) {
	txt(c, 18, 15, 21, "2026年7月", ink, 700)
	txt(c, 18, 46, 12, "整月轨迹总览。点选任一天，右侧查看当天详情。", muted, 500)
	toolbar(c, w, 16, true)
	txt(c, w-37, 24, 16, "‹‹", muted, 400)
	cw := (w - 18) / 7
	rh := (h - 111) / 5
	week := []string{"一", "二", "三", "四", "五", "六", "日"}
	for i, d := range week {
		txt(c, 18+float32(i)*cw+cw/2-5, 75, 11, d, muted, 600)
	}
	for i := 0; i < 35; i++ {
		col, row := i%7, i/7
		x := 18 + float32(col)*cw
		y := 111 + float32(row)*rh
		d := i - 1
		if col > 0 {
			box(c, x, y, .7, rh, line, 0)
		}
		if row > 0 {
			rule(c, x, y, cw)
		}
		if d < 1 || d > 31 {
			continue
		}
		if d == s.selectedDay {
			box(c, x+1, y+1, cw-2, rh-1, ui.Hex("#edf3ff"), 0)
			box(c, x+1, y, 2, rh, blue, 0)
		}
		tc := ink
		if d == s.selectedDay {
			tc = blue
		} else if d == 18 || d >= 27 {
			tc = muted
		}
		txt(c, x+6, y+3, 13, fmt.Sprint(d), tc, 600)
		if d == 24 {
			box(c, x+27, y+11, 4, 4, blue, 2)
		}
		items := []string{}
		if d < 15 {
			items = []string{fmt.Sprintf("年度第 %d 天：完成日…", 341+d)}
		}
		switch d {
		case 15:
			items = []string{"整理用户研究简报", "跟进访谈行动项"}
		case 16:
			items = []string{"跟进访谈行动项"}
		case 17:
			items = []string{"交付 onboarding 三屏…"}
		case 19:
			items = []string{"验证旧版增长实验", "恢复发布前检查清单"}
		case 20, 21:
			items = []string{"交付首页交互稿"}
		case 22, 23:
			items = []string{"整理跨团队交接材料"}
		case 24:
			items = []string{"修复登录阻断问题", "审阅发布说明", "年度撤回延期样例 1", "年度撤回延期样例 2", "年度撤回延期样例 3", "年度撤回延期样例 4", "+15"}
		case 25:
			items = []string{"等待法务确认隐私文案"}
		case 26:
			items = []string{"主持路线图工作坊", "安排发布后复盘会议", "安排发布后复盘会议", "安排发布后复盘会议", "安排发布后复盘会议"}
		case 28:
			items = []string{"提交采购申请"}
		}
		if len(items) > 0 {
			box(c, x+cw-15, y+7, 10, 10, green.Mix(white, .73), 3)
		}
		for j, s := range items {
			iy := y + 22 + float32(j)*14
			lc := muted
			dot := green
			if d >= 24 {
				lc = ink
				dot = blue
			}
			if s == "+15" {
				txt(c, x+12, iy, 9.5, s, muted, 400)
				continue
			}
			box(c, x+8, iy+5, 4, 4, dot, 2)
			t := txt(c, x+18, iy, 9.7, s, lc, 500).Width(cw - 24).SingleLine()
			if d < 18 || d == 23 {
				t.Strikethrough()
			}
		}
		dd := d
		labelButton(c, fmt.Sprintf("cal-%d", d), x+1, y+1, cw-2, rh-2, fmt.Sprintf("选择7月%d日", d), func() { s.selectedDay = dd })
	}
}
func (s *app) stub(c *ui.Context, w, h float32) {
	title := s.page
	for _, n := range nav {
		if n.page == s.page {
			title = n.title
		}
	}
	txt(c, 20, 16, 21, title, ink, 700)
	txt(c, 20, 58, 12, "此页面尚未实现。本轮原型已完成 Day Todo、任务池、日历。", muted, 400).Width(w - 40)
	txt(c, 20, 86, 12, "通过左侧导航切换已完成页面。", muted, 400)
}

type screenSpec struct {
	key           string
	width, height int
}

var screens = []screenSpec{{"day", 1200, 768}, {"pool", 1200, 768}, {"calendar", 1200, 768}, {"upcoming", 1200, 768}, {"recurring", 1200, 768}, {"unfinished", 1200, 768}, {"completed", 1200, 768}, {"flylight", 1200, 768}, {"flylight-edit", 1200, 768}, {"sticky", 1200, 768}, {"sticky-stream", 1200, 768}, {"ai", 1200, 768}, {"settings-general", 860, 660}, {"settings-groups", 860, 660}, {"settings-data", 860, 660}, {"settings-privacy", 860, 660}, {"settings-sync", 860, 660}, {"settings-provider", 860, 660}, {"quick-entry", 520, 168}, {"flylight-quick-capture", 520, 262}}

func navPage(page string) string {
	switch page {
	case "flylight-edit":
		return "flylight"
	case "sticky-stream":
		return "sticky"
	}
	return page
}
func findScreen(key string) (screenSpec, bool) {
	for _, spec := range screens {
		if spec.key == key {
			return spec, true
		}
	}
	return screenSpec{}, false
}
func renderScreen(spec screenSpec, dir string) error {
	s := newApp()
	s.page = spec.key
	return renderApp(s, spec, dir)
}
func renderApp(s *app, spec screenSpec, dir string) error {
	img := ui.Render(s.view, spec.width, spec.height, 2)
	f, err := os.Create(filepath.Join(dir, "noonmark-"+spec.key+"-native.png"))
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	fmt.Printf("Rendered %s: %dx%d pixels via MyGo ui.Render\n", spec.key, img.Bounds().Dx(), img.Bounds().Dy())
	return nil
}
func main() {
	render := flag.String("render", "", "Export all implemented native UI frames to this directory")
	one := flag.String("render-one", "", "Export only -screen to this directory")
	page := flag.String("screen", "day", "Initial screen; use -list to see available screens")
	list := flag.Bool("list", false, "List native prototype screen keys and logical dimensions")
	initSession := flag.String("init-session", "", "Exclusively create a new isolated development session directory")
	session := flag.String("session", "", "Open an existing isolated development session")
	prototype := flag.Bool("prototype", false, "Run the static UI prototype without storage")
	flag.Parse()
	if *list {
		for _, spec := range screens {
			fmt.Printf("%s %dx%d\n", spec.key, spec.width, spec.height)
		}
		return
	}
	spec, ok := findScreen(*page)
	if !ok {
		log.Fatalf("Unknown screen %q; use -list", *page)
	}
	if *initSession != "" && *session != "" {
		log.Fatal("Choose only one session action")
	}
	var service *taskapp.Service
	if *initSession != "" || *session != "" {
		if *prototype || *render != "" {
			log.Fatal("Session mode cannot mix with prototype or all-screen rendering")
		}
		now := time.Now()
		today := core.LocalDate{Year: now.Year(), Month: int(now.Month()), Day: now.Day()}
		var err error
		if *initSession != "" {
			service, err = taskapp.Create(*initSession, today)
		} else {
			service, err = taskapp.Open(*session, today)
		}
		if err != nil {
			log.Fatal("Cannot open isolated development session: ", err)
		}
		defer service.Close()
	}
	if service == nil && !*prototype && *render == "" && *one == "" {
		log.Fatal("Use -init-session or -session for development storage; -prototype for the static visual study")
	}
	if *render != "" || *one != "" {
		dir := *render
		if *one != "" {
			dir = *one
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatal(err)
		}
		targets := screens
		if *one != "" {
			targets = []screenSpec{spec}
		}
		for _, target := range targets {
			var err error
			if service != nil {
				state := newRuntimeApp(service)
				state.page = target.key
				err = renderApp(state, target, dir)
			} else {
				err = renderScreen(target, dir)
			}
			if err != nil {
				log.Fatal(err)
			}
		}
		return
	}
	var open func(string)
	open = func(key string) {
		spec, ok := findScreen(key)
		if !ok {
			return
		}
		s := newApp()
		if service != nil {
			s = newRuntimeApp(service)
			s.observeRuntimeDay(time.Now)
		}
		s.page = key
		s.openUtility = open
		mw, mh := 960, 720
		if strings.HasPrefix(key, "settings-") {
			mw, mh = 700, 520
		} else if spec.width < 960 {
			mw, mh = spec.width, spec.height
		}
		title := "Noonmark · MyGo UI Prototype"
		if service != nil {
			title = "Noonmark · MyGo Development"
		}
		win := mygo.NewWindow(mygo.WindowOptions{Title: title, Width: spec.width, Height: spec.height, MinWidth: mw, MinHeight: mh, Content: ui.View(s.view)})
		s.closeWindow = win.Close
	}
	mygo.App.WhenReady(func() { open(*page) })
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
