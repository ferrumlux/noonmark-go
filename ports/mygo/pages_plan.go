package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
)

var planCheckIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 6l3 3 5-7"/></svg>`))

// These views intentionally render fixture content only, without planning logic.
func planHeader(c *ui.Context, w float32, title, subtitle string, control string) {
	txt(c, 20, 13, 19, title, ink, 700)
	txt(c, 20, 44, 12, subtitle, muted, 600)
	if control != "" {
		x := w - 82
		if title == "未来计划" {
			x = w - 115
		}
		txt(c, x, 28, 13, control, ink, 500)
	}
}
func planChip(c *ui.Context, x, y float32, title string, col ui.Color) {
	box(c, x, y, 47, 20, col.Mix(white, .92), 3)
	icon(c, x+6, y+5, 11, "folder", col)
	txt(c, x+21, y+2, 10.5, title, col, 600)
}
func planCircle(c *ui.Context, x, y float32, name string, col ui.Color) {
	box(c, x, y, 20, 20, col.Mix(white, .89), 10)
	icon(c, x+4, y+4, 12, name, col)
}
func planDate(c *ui.Context, y float32, date string) {
	txt(c, 20, y-2, 13, date, ink, 600)
	txt(c, 120, y-2, 11.5, "查看当天 →", blue, 600)
}
func planFutureRow(c *ui.Context, w, y float32, title, grp, tag, note string, repeat bool) {
	col, name := purple, "calendar"
	if repeat {
		col, name = ui.Hex("#ba690d"), "repeat"
	}
	cy := y + 11
	if grp == "" && note == "" {
		cy = y + 1
	}
	planCircle(c, 32, cy, name, col)
	txt(c, 62, y, 13, title, ink, 600)
	bottom := y + 27
	if grp != "" {
		gc := orange
		if grp == "产品" {
			gc = blue
		}
		planChip(c, 62, y+21, grp, gc)
		badge(c, 115, y+21, tag, col)
		bottom = y + 49
	}
	if note != "" {
		txt(c, 62, y+20, 11, note, muted, 500)
		bottom = y + 39
	}
	rule(c, 60, bottom, w-80)
}
func (s *app) upcoming(c *ui.Context, w, h float32) {
	planHeader(c, w, "未来计划", "按日期查看普通任务与可见范围内的重复实例。", "⇄ 未来 15 天⌄")
	planDate(c, 82, "7月25日 · 周六")
	planFutureRow(c, w, 110, "每日产品复盘", "复盘", "发布", "", true)
	planFutureRow(c, w, 168, "等待法务确认隐私文案", "", "", "由 7月24日 延期 →", false)
	planDate(c, 224, "7月26日 · 周日")
	planFutureRow(c, w, 252, "准备下周工作回顾", "复盘", "发布", "", true)
	planFutureRow(c, w, 310, "主持路线图工作坊", "产品", "路线图", "", false)
	for i := 0; i < 4; i++ {
		planFutureRow(c, w, 370+float32(i)*36, "安排发布后复盘会议", "", "", "", false)
	}
	planDate(c, 520, "7月27日 · 周一")
	planFutureRow(c, w, 548, "准备下周工作回顾", "复盘", "发布", "", true)
	planDate(c, 614, "7月28日 · 周二")
	planFutureRow(c, w, 642, "每日产品复盘", "复盘", "发布", "", true)
	planFutureRow(c, w, 700, "准备下周工作回顾", "复盘", "发布", "", true)
}
func (s *app) recurring(c *ui.Context, w, h float32) {
	planHeader(c, w, "重复计划", "集中管理重复规则、完整轨迹与已结束计划。", "")
	type row struct {
		title, freq, status string
		y                   float32
		state               int
	}
	rows := []row{
		{"年度每周回顾", "每 6 天", "进行中 · 暂无后续排期", 92, 0},
		{"年度工作日整理", "工作日", "进行中 · 下次 7月24日", 128, 0},
		{"每日产品复盘", "每天", "进行中 · 下次 7月24日", 164, 0},
		{"准备下周工作回顾", "每天", "即将开始 · 7月26日开始", 249, 1},
		{"下月工作日复盘", "工作日", "即将开始 · 8月13日开始", 285, 1},
		{"下月隔日训练", "每 2 天", "即将开始 · 8月18日开始", 321, 1},
		{"完成首次晨间回顾", "每天", "已结束 · 完成 1/1 次", 357, 2},
		{"首月每日整理", "每天", "已结束 · 完成 0/30 次", 393, 2},
		{"首月每周盘点", "每 7 天", "已结束 · 完成 0/5 次", 429, 2},
		{"暂停周报打磨", "每天", "已停止 · 停止于 7月24日", 465, 3},
		{"停止年度周报", "每天", "已停止 · 停止于 7月25日", 501, 3},
		{"停止年度工作日记录", "工作日", "已停止 · 停止于 7月25日", 537, 3},
	}
	box(c, 20, 155, w-40, 84, ui.Hex("#fbf4ee"), 7).Border(.8, ui.Hex("#ecd1b8"))
	for i, r := range rows {
		col := ui.Hex("#ba690d")
		tc := ink
		if r.state == 1 {
			col = purple
		}
		if r.state >= 2 {
			tc = ui.Hex("#a3a3ad")
			col = green
		}
		if r.state == 3 {
			col = ui.Hex("#bd4b32")
		}
		planCircle(c, 32, r.y-1, "repeat", col)
		if r.state >= 2 {
			box(c, 36, r.y+3, 12, 12, col, 2)
			if r.state == 2 {
				txt(c, 36, r.y-1, 12, "⚑", white, 600)
			}
		}
		txt(c, 62, r.y, 13, r.title, tc, 600)
		fx := 62 + float32(len([]rune(r.title)))*13 + 12
		if i == 2 {
			planChip(c, 150, r.y, "复盘", orange)
			badge(c, 203, r.y, "发布", orange)
			fx = 258
		}
		if i == 3 {
			planChip(c, 176, r.y, "复盘", orange)
			badge(c, 229, r.y, "发布", orange)
			fx = 284
		}
		txt(c, fx, r.y+1, 11, r.freq, muted, 600)
		sc := col
		if r.state >= 2 {
			sc = tc
		}
		txt(c, w-300, r.y+1, 10.8, r.status, sc, 500).Width(225).TextAlign(ui.End)
		txt(c, w-65, r.y+1, 10.5, "轨迹", muted, 600)
		arrow := "›"
		if i == 2 {
			arrow = "⌄"
		}
		txt(c, w-40, r.y-2, 17, arrow, muted, 400)
		if i != 2 {
			rule(c, 60, r.y+26, w-80)
		}
	}
	days := []string{"三", "四", "五", "六", "日", "一", "二", "三", "四", "五", "六", "日", "一"}
	for i, d := range days {
		x := 68 + float32(i)*32
		if i == 12 {
			x += 10
		}
		col := muted
		if i == 9 {
			box(c, x-7, 193, 28, 37, soft, 7)
			col = blue
		}
		txt(c, x+2, 191, 9, d, muted, 500)
		txt(c, x-1, 205, 11.5, fmt.Sprint(15+i), col, 600)
		symbol := "✓"
		sc := green
		if i%2 == 1 {
			symbol = "×"
			sc = ui.Hex("#bd4b32")
		}
		if i >= 9 {
			symbol = "●"
			sc = purple
		}
		if i == 9 {
			sc = blue
		}
		if i == 11 {
			symbol = "▷"
			sc = faint
		}
		txt(c, x+1, 220, 11, symbol, sc, 600)
		if i == 12 {
			txt(c, x+11, 223, 9, "→7/28", purple, 600)
		}
	}
}
func (s *app) unfinished(c *ui.Context, w, h float32) {
	planHeader(c, w, "未完成", "按任务链去重的未完成轨迹。延续它，或明确废弃。", "⇅ 视图⌄")
	for i, title := range []string{"整理跨团队交接材料", "完成可用性走查"} {
		y := float32(92 + i*84)
		planCircle(c, 32, y+1, "", ui.Hex("#c34c30"))
		txt(c, 36, y, 14, "×", ui.Hex("#c34c30"), 400)
		txt(c, 60, y, 13, title, ink, 600)
		gc, grp, tag := orange, "协作", "交接"
		if i == 1 {
			gc, grp, tag = purple, "研究", "走查"
		}
		planChip(c, 60, y+22, grp, gc)
		badge(c, 113, y+22, tag, teal)
		txt(c, 60, y+52, 11, "1 次未完成", ui.Hex("#c34c30"), 500)
		txt(c, 122, y+52, 11, fmt.Sprintf("·  0 次延续  ·  最近未完成 · 7月%d日", 22-i), muted, 500)
		txt(c, w-62, y+52, 10.5, "▸ 明细", muted, 600)
		rule(c, 60, y+74, w-80)
	}
	titles := []string{"验证旧版增长实验", "年度第 12 轮：停止失效尝试", "年度第 11 轮：停止失效尝试", "年度第 10 轮：停止失效尝试", "年度第 9 轮：停止失效尝试", "年度第 8 轮：停止失效尝试", "年度第 7 轮：停止失效尝试", "年度第 6 轮：停止失效尝试", "年度第 5 轮：停止失效尝试"}
	dates := []string{"7月19日", "6月9日", "5月11日", "4月12日", "3月14日", "2月13日", "1月15日", "12月17日", "11月18日"}
	for i, title := range titles {
		y := float32(260 + i*60)
		box(c, 32, y+1, 18, 18, ui.Hex("#f4f4f6"), 9)
		txt(c, 36, y-2, 17, "−", muted, 600)
		txt(c, 60, y, 13, title, muted, 600)
		txt(c, 60, y+26, 11, "已废弃  ·  0 次延续  ·  最近未完成 · "+dates[i], muted, 500)
		txt(c, w-62, y+26, 10.5, "▸ 明细", muted, 600)
		rule(c, 60, y+50, w-80)
	}
}
func (s *app) completed(c *ui.Context, w, h float32) {
	planHeader(c, w, "已完成", "按完成记录逐条展示，并附带每条的任务轨迹。", "⇅ 视图⌄")
	group(c, 20, 94, "产品", 30, blue)
	dates := []string{"7月17日 17:00", "7月5日 18:00", "6月23日 18:00", "6月11日 18:00", "5月30日 18:00", "5月18日 18:00", "5月6日 18:00", "4月24日 18:00", "4月12日 18:03", "3月31日 18:00"}
	for i, date := range dates {
		y := float32(126 + i*68)
		title := fmt.Sprintf("年度第 %d 天：完成日常推进", 358-i*12)
		tag := "发布"
		col := orange
		if i == 0 {
			title = "交付 onboarding 三屏文案"
			tag = "文案"
			col = pink
		}
		box(c, 32, y+13, 18, 18, green, 9)
		ui.Icon(c, planCheckIcon).Absolute().Left(36).Top(y+17).Size(11, 11).TextColor(white)
		txt(c, 60, y, 13, title, muted, 600).Strikethrough()
		badge(c, 60, y+21, tag, col)
		txt(c, w-122, y+14, 11, date, muted, 500).Width(90).TextAlign(ui.End)
		rule(c, 60, y+55, w-80)
	}
}

func (s *app) planRail(c *ui.Context, w, h float32) {
	box(c, 0, 0, .7, h, ui.Hex("#dddddf"), 0)
	txt(c, 17, 6, 16, "››", muted, 400)
	icon(c, 45, 12, 14, "search", muted)
	txt(c, 63, 10, 12, "搜索", ui.Hex("#999999"), 500)
	if s.page == "recurring" {
		s.recurringRail(c, w)
		return
	}
	title, desc := "未来计划分析", "按日期观察未来任务负载与延续压力。"
	labels := []string{"计划任务", "覆盖日期", "延续任务"}
	vals := []string{"23", "15", "0"}
	cols := []ui.Color{blue, muted, green}
	signals := []string{"最近计划：7月25日 · 每日产品复盘", "7月26日 已排 6 项，负载最重。"}
	advice := []string{"优先检查最近日期，避免未来任务堆到同一天。", "暂无延续压力，可以按优先级安排新任务。"}
	if s.page == "unfinished" {
		title = "未完成风险"
		desc = "按任务链汇总历史未完成与当前延续状态。"
		labels = []string{"任务链", "未完成次", "延续任务"}
		vals = []string{"15", "15", "0"}
		cols = []ui.Color{ui.Hex("#bd4b32"), ui.Hex("#bd4b32"), blue}
		signals = []string{"没有重复未完成链。", "暂无已延续到当前或未来的未完成链。"}
		advice = []string{"优先处理最近未完成且尚未延续的任务链。", "对单次未完成任务，直接延续或明确废弃。"}
	}
	if s.page == "completed" {
		title = "完成轨迹"
		desc = "汇总完成记录、子任务完成和跨日轨迹。"
		labels = []string{"完成任务", "子任务", "覆盖日期"}
		vals = []string{"458", "29", "362"}
		cols = []ui.Color{green, blue, muted}
		signals = []string{"最近完成：7月24日 · 完成晨间同步", "25 条完成记录经历过延续。"}
		advice = []string{"复盘最近完成记录，提取可复制的推进方式。", "子任务完成已单独记录，可用来判断复杂任务的\n真实推进。"}
	}
	txt(c, 14, 50, 11, title, muted, 600)
	txt(c, 14, 68, 11.5, desc, muted, 500)
	for i, l := range labels {
		y := float32(98 + i*20)
		txt(c, 14, y, 11, l, muted, 500)
		txt(c, w-14-float32(len(vals[i]))*7, y, 11.5, vals[i], cols[i], 600)
	}
	rule(c, 14, 169, w-28)
	txt(c, 14, 186, 11, "本地信号", muted, 600)
	for i, line := range signals {
		y := float32(208 + i*22)
		txt(c, 14, y, 11, "•", muted, 600)
		txt(c, 26, y, 11.5, line, ink, 500)
	}
	ay := float32(260)
	if s.page == "upcoming" {
		txt(c, 26, 248, 11.5, "查看这一天", blue, 600)
		ay = 278
	}
	txt(c, 14, ay, 11, "算法建议", muted, 600)
	for i, line := range advice {
		y := ay + 22 + float32(i)*22
		txt(c, 14, y, 11, "•", muted, 600)
		txt(c, 26, y, 11.5, line, ink, 500).FixedLineHeight(17)
	}
	end := ay + 74
	if s.page == "completed" {
		end += 17
	}
	rule(c, 14, end, w-28)
	icon(c, 16, end+38, 13, "spark", blue)
	ct, cd := "交给烛龙", "建立加密会话，先确认本次范围；没有\nProvider 时仍可使用本地证据。"
	if s.page == "unfinished" {
		ct = "继续最近会话"
		cd = "复盘一下最近任务经常延期的模式。 · 草稿\n待审"
	}
	txt(c, 38, end+21, 11.5, ct, ink, 600)
	txt(c, 38, end+37, 10.5, cd, muted, 500).FixedLineHeight(16)
	txt(c, w-21, end+34, 17, "›", muted, 400)
}
func (s *app) recurringRail(c *ui.Context, w float32) {
	txt(c, 14, 57, 11, "重复任务", muted, 600)
	txt(c, w-34, 53, 17, "×", muted, 400)
	txt(c, 14, 94, 14, "每日产品复盘", ink, 600)
	txt(c, 14, 123, 12, "每天用十分钟记录完成、遗漏与下一步。", ink, 500)
	txt(c, 14, 173, 11, "分组", muted, 600)
	icon(c, 62, 175, 14, "folder", ink)
	txt(c, 83, 173, 11, "复盘", orange, 600)
	txt(c, 124, 173, 11, "☷ 管理", muted, 600)
	rule(c, 14, 201, w-28)
	txt(c, 14, 216, 11, "标签", muted, 600)
	box(c, 58, 211, 72, 24, ui.Hex("#fcf3e9"), 6).Border(.7, ui.Hex("#f3d8b7"))
	txt(c, 66, 216, 11, "# 发布  ×", muted, 600)
	box(c, 136, 210, 112, 28, white, 6).Border(.7, line)
	txt(c, 144, 217, 11, "输入 # 添加标签", ui.Hex("#999999"), 500)
	txt(c, 24, 262, 11.5, "记录完成、遗\n漏与下一步", ink, 600).FixedLineHeight(15)
	box(c, 144, 267, 105, 21, white, 6).Border(.8, line)
	txt(c, 152, 270, 10, "简", ink, 500)
	box(c, 231, 269, 16, 16, ui.Hex("#1889ff"), 4)
	txt(c, 234, 267, 14, "⌄", white, 700)
	txt(c, 257, 267, 14, "×", ink, 500)
	box(c, 14, 307, w-28, 32, ui.Hex("#fafafa"), 7)
	txt(c, 26, 315, 12, "添加子任务，回车确认", faint, 500)
	txt(c, 14, 353, 11, "重复设置", muted, 600)
	for i, p := range [][2]string{{"开始日期", "7月15日"}, {"重复方式", "每天"}, {"结束条件", "截至 2026-07-27"}} {
		y := float32(375 + i*22)
		txt(c, 14, y, 11, p[0], muted, 600)
		txt(c, 82, y, 12, p[1], ink, 600)
	}
	txt(c, 14, 440, 10.5, "尚未开始且没有执行记录时可调整开始日期；开始后，\n频率和结束条件的修改从明天起生效，历史不会改写。", muted, 500).FixedLineHeight(13)
	txt(c, 14, 474, 11.5, "修改重复设置...", blue, 600)
	txt(c, 14, 503, 11, "轨迹", muted, 600)
	txt(c, 14, 524, 11.5, "进行中 · 下次 7月24日", muted, 500)
	txt(c, 14, 552, 11.5, "停止重复...", ui.Hex("#c34c30"), 600)
}
