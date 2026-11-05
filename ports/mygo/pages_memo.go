package main

import "github.com/egoist/mygo/ui"

// Public Noonmark demo copy only. These views do not persist, publish or call AI.
const memoFirst = "连续用了十天，Sticky Note 比长清单更能压住注意力，写进复盘。"
const memoHandoff = "交接材料里的接口清单可以抽成模板，下个项目直接复用。"
const memoExpense = "差旅报销能不能攒到月底批量处理？每次都打断半个下午。"

func memoText(c *ui.Context, x, y, w float32, text string) {
	txt(c, x, y, 13.5, text, ink, 500).Width(w).FixedLineHeight(20)
}
func memoButton(c *ui.Context, key string, x, y, w float32, text string, primary bool, fn func()) {
	bg, fg := white, muted
	if primary {
		bg, fg = blue, white
	}
	box(c, x, y, w, 32, bg, 9).Border(.6, line)
	txt(c, x, y+7, 12, text, fg, 600).Width(w).TextAlign(ui.Center)
	labelButton(c, key, x, y, w, 32, "飞光操作："+text, fn)
}
func (s *app) flylight(c *ui.Context, w, h float32) {
	editing := s.page == "flylight-edit"
	txt(c, 20, 14, 21, "飞光", ink, 700)
	txt(c, 20, 44, 12, "先记下来，再决定它要去哪里。", muted, 600)
	icon(c, w-117, 30, 15, "search", muted)
	txt(c, w-95, 29, 12, "搜索", muted, 600)
	txt(c, w-65, 28, 15, "↻", muted, 500)
	txt(c, w-43, 29, 12, "回看", muted, 600)
	ch := float32(65)
	if editing {
		ch = 113
	}
	border := line
	bw := float32(1)
	if editing {
		border = blue
		bw = 1.5
	}
	box(c, 20, 71, w-40, ch, ui.Hex("#fbfbfc"), 13).Border(bw, border)
	iy := float32(71)
	if editing {
		iy = 82
	}
	ui.TextAreaBase(c.Key("flylight-draft"), &s.draft).Absolute().Left(38).Top(iy).Size(w-76, ch-43).FontSize(13.5).TextColor(ink).Placeholder("你在想什么？支持 Markdown、#标签与 @分组；⌘回车保存，草稿自动保留").PlaceholderColor(faint).Label("飞光草稿，仅本次演示")
	fy := 71 + ch - 43
	rule(c, 20, fy, w-40)
	txt(c, 38, fy+11, 13, "#     @    Aa", muted, 700)
	toggle := "展开"
	if editing {
		toggle = "收起"
	}
	memoButton(c, "flylight-expand", w-216, fy+6, 78, toggle, false, func() {
		if editing {
			s.page = "flylight"
		} else {
			s.page = "flylight-edit"
		}
	})
	box(c, w-130, fy+6, 102, 32, ui.Hex("#f4f4f5"), 9)
	txt(c, w-100, fy+13, 12, "发布 ↑", muted, 600)
	y := 71 + ch + 14
	txt(c, 20, y, 11.5, "最近 · 8", muted, 500)
	y += 31
	txt(c, 20, y, 12, "2026年7月24日   2", ui.Hex("#9999a3"), 600)
	y += 20
	if editing {
		box(c, 20, y, w-40, 133, ui.Hex("#fbfdff"), 9)
		box(c, 32, y+10, w-64, 113, ui.Hex("#fbfbfc"), 13).Border(1, line)
		memoText(c, 48, y+22, w-96, memoFirst)
		box(c, w-239, y+23, 1.5, 17, blue, 1)
		rule(c, 32, y+81, w-64)
		txt(c, 48, y+94, 13, "#     @    Aa", muted, 700)
		memoButton(c, "flylight-cancel", w-228, y+87, 78, "取消", false, func() { s.page = "flylight" })
		box(c, w-142, y+87, 102, 32, ui.Hex("#f4f4f5"), 9)
		txt(c, w-125, y+94, 12, "保存修改 ↑", muted, 600)
		y += 133
	} else {
		box(c, 20, y, w-40, 56, ui.Hex("#f5f8ff"), 9).Border(1, ui.Hex("#c4d5fb"))
		memoText(c, 32, y+8, w-64, memoFirst)
		txt(c, 32, y+31, 11, "15:10", ui.Hex("#9999a3"), 500)
		icon(c, 72, y+33, 12, "note", ui.Hex("#c74006"))
		txt(c, 93, y+31, 11, "Sticky Note", ui.Hex("#c74006"), 600)
		txt(c, w-50, y+28, 15, "…", ink, 600)
		labelButton(c, "flylight-inline-edit", 20, y, w-40, 56, "在列表中编辑飞光", func() { s.page = "flylight-edit" })
		y += 56
	}
	rule(c, 32, y, w-52)
	memoText(c, 32, y+9, w-64, "审阅发布说明时想到：已知限制应该附上临时绕过方法，减少支持成本。")
	txt(c, 32, y+31, 11, "09:20  #发布", ui.Hex("#9999a3"), 500)
	txt(c, w-50, y+29, 15, "…", ui.Hex("#cccccf"), 600)
	y += 75
	entries := []struct {
		date, body, meta string
		sticky           bool
	}{
		{"2026年7月23日   1", memoHandoff, "13:30  @协作  #交接", true},
		{"2026年7月22日   1", "数据口径注释确认由周报模板统一维护，恢复这条留作月底跟进提醒。", "18:05", false},
		{"2026年7月21日   1", "可用性走查的意外发现：空标题占位比想象中更容易误触，值得单独跟进。", "19:45  #走查", false},
		{"2026年7月18日   1", "首页交互稿的悬停反馈有点保守；和设计对齐后决定保留大字号方案，交付稿里注明。", "16:20", false},
		{"2026年7月16日   1", memoExpense, "12:40", true},
	}
	for _, e := range entries {
		txt(c, 20, y, 12, e.date, ui.Hex("#9999a3"), 600)
		memoText(c, 32, y+27, w-64, e.body)
		txt(c, 32, y+50, 11, e.meta, ui.Hex("#9999a3"), 500)
		if e.sticky {
			sx := float32(150)
			if e.meta == "12:40" {
				sx = 72
			}
			icon(c, sx, y+52, 12, "note", ui.Hex("#c74006"))
			txt(c, sx+21, y+50, 11, "Sticky Note", ui.Hex("#c74006"), 600)
		}
		txt(c, w-50, y+48, 15, "…", ui.Hex("#cccccf"), 600)
		y += 93
	}
}
func (s *app) memoRail(c *ui.Context, w, h float32) {
	box(c, 0, 0, .7, h, ui.Hex("#dddddf"), 0)
	txt(c, 17, 6, 16, "››", muted, 400)
	icon(c, 45, 12, 14, "search", muted)
	txt(c, 63, 10, 12, "搜索", ui.Hex("#999999"), 500)
	txt(c, 14, 49, 11, "飞光详情", muted, 600)
	memoText(c, 14, 78, w-28, memoFirst)
	rule(c, 14, 118, w-28)
	txt(c, 14, 135, 11, "记录时间", muted, 500)
	txt(c, 14, 155, 12, "2026-07-24 15:10", muted, 500)
	rule(c, 14, 190, w-28)
	txt(c, 14, 207, 11, "操作", muted, 500)
	memoButton(c, "rail-edit-flylight", 14, 229, 102, "编辑飞光", true, func() { s.page = "flylight-edit" })
	txt(c, w-31, 230, 15, "…", ink, 600)
	memoButton(c, "rail-remove-sticky", 14, 269, 140, "从 Sticky Note 移除", false, nil)
}
func (s *app) sticky(c *ui.Context, w, h float32) {
	stream := s.page == "sticky-stream"
	txt(c, 20, 14, 21, "Sticky Note", ink, 700)
	txt(c, 20, 44, 12, "把值得反复看见的飞光留在这里。", muted, 600)
	view := "▦ 便签墙⌄"
	if stream {
		view = "☷ 清单流⌄"
	}
	txt(c, w-94, 29, 13, view, ink, 500)
	labelButton(c, "sticky-view-toggle", w-100, 22, 80, 30, "切换便签墙或清单流", func() {
		if stream {
			s.page = "sticky"
		} else {
			s.page = "sticky-stream"
		}
	})
	bodies := []string{memoFirst, memoHandoff, memoExpense}
	metas := []string{"2026-07-24 15:10", "2026-07-23 13:30  @协作  #交接", "2026-07-16 12:40"}
	if stream {
		cw := w - 220
		if cw < 480 {
			cw = w - 40
		}
		x := (w - cw) / 2
		for i, body := range bodies {
			y := float32(88 + i*71)
			memoText(c, x+14, y, cw-28, body)
			txt(c, x+14, y+30, 11, metas[i], muted, 500)
			txt(c, x+cw-31, y+28, 15, "…", ink, 600)
			if i < 2 {
				rule(c, x, y+59, cw)
			}
		}
	} else {
		cw := (w - 72) / 3
		for i, body := range bodies {
			x := 20 + float32(i)*(cw+16)
			box(c, x, 77, cw, 99, ui.Hex("#fffbef"), 4).Border(1, line).Shadow(0, 3, 12, 0, ui.Hex("#0000000b"))
			memoText(c, x+18, 94, cw-36, body)
			txt(c, x+18, 143, 11, metas[i], muted, 500)
			txt(c, x+cw-35, 140, 15, "…", ink, 600)
		}
	}
}
func (s *app) ai(c *ui.Context, w, h float32) {
	box(c, 0, 0, w, h, ui.Hex("#fdfdfd"), 0)
	txt(c, 20, 18, 18, "‹", muted, 400)
	txt(c, 37, 19, 12, "全部会话", muted, 600)
	txt(c, w/2-21, 14, 21, "烛龙", ink, 700)
	txt(c, w-135, 20, 12, "⌄ 视图 · 对话", ink, 600)
	txt(c, w-40, 19, 16, "‹‹", muted, 500)
	cw := float32(680)
	if cw > w-80 {
		cw = w - 80
	}
	x := (w - cw) / 2
	box(c, x+cw-230, 84, 230, 32, ui.Hex("#edf3ff"), 13)
	txt(c, x+cw-218, 91, 12.5, "复盘一下最近任务经常延期的模式。", ink, 500)
	txt(c, x, 135, 12.5, "最近的延期主要发生在依赖未确认和任务范围过大的场景。建议把等待依赖与可独立推进的部分分开。", ink, 500).Width(cw).FixedLineHeight(20)
	box(c, x, h-106, cw, 90, ui.Hex("#fbfbfc"), 14).Shadow(0, 2, 24, 0, ui.Hex("#00000009"))
	ui.TextAreaBase(c.Key("ai-demo-input"), &s.draft).Absolute().Left(x+18).Top(h-97).Size(cw-55, 65).FontSize(13.5).TextColor(ink).Placeholder("向烛龙发消息……").PlaceholderColor(faint).Label("烛龙静态演示输入，不会发送")
	box(c, x+cw-41, h-52, 28, 28, ui.Hex("#fdfdfd"), 14)
	txt(c, x+cw-33, h-50, 19, "↑", ui.Hex("#bcbcc4"), 500)
}
