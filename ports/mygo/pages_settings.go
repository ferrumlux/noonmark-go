package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
)

// These independent utility windows follow the six public 2x screenshots.
// Sync and Provider are source-derived, intentionally inert previews.
func (s *app) settingsView(c *ui.Context) {
	w, h := c.Size()
	k := w / 860
	ui.Box(c).Absolute().Size(860, h/k).TransformOrigin(0, 0).Scale(k, k).Clip().Background(ui.Hex("#fdfdfd")).Children(func() {
		box(c, 0, 0, 240, h/k, ui.Hex("#e5e5e5"), 0)
		box(c, 240, 0, .6, h/k, ui.Hex("#777777"), 0)
		titles := []string{"偏好", "组织", "数据", "同步", "烛龙与自动化", "写入与隐私边界"}
		pages := []string{"settings-general", "settings-groups", "settings-data", "settings-sync", "settings-provider", "settings-privacy"}
		glyphs := []string{"☷", "⊞", "♧", "⟳", "✦", "♙"}
		for i, p := range pages {
			y := float32(10 + i*28)
			col := ink
			if s.page == p {
				bg := ui.Hex("#1681f7")
				if i == 0 {
					bg = ui.Hex("#cbcbcb")
				} else {
					col = white
				}
				box(c, 10, y, 220, 28, bg, 5)
			}
			txt(c, 19, y+4, 15, glyphs[i], col, 400)
			txt(c, 40, y+5, 13, titles[i], col, 400)
			page := p
			labelButton(c, "settings-nav-"+p, 10, y, 220, 28, "设置："+titles[i], func() { s.page = page })
		}
		title := "偏好"
		for i, p := range pages {
			if p == s.page {
				title = titles[i]
			}
		}
		txt(c, 275, 29, 20, title, ink, 700)
		switch s.page {
		case "settings-groups":
			txt(c, 275, 74, 12, "用一个分组建立结构，再用标签补充横向线索。", muted, 500)
			txt(c, 275, 102, 15, "6", ink, 700)
			txt(c, 275, 121, 10, "分组", muted, 400)
			box(c, 343, 103, .7, 30, line, 0)
			txt(c, 358, 102, 15, "11", ink, 700)
			txt(c, 358, 121, 10, "标签", muted, 400)
			box(c, 700, 103, 126, 30, soft, 7).Border(.7, blue.Mix(white, .65))
			txt(c, 711, 111, 10.5, "☷  管理分组与标签  ›", blue, 600)
			labelButton(c, "groups-preview", 700, 103, 126, 30, "打开分组与标签预览", func() { s.done[910] = !s.done[910] })
			if s.done[910] {
				box(c, 275, 164, 551, 218, white, 8).Border(1, line)
				txt(c, 292, 180, 14, "分组与标签", ink, 600)
				for i, t := range []string{"工程", "产品", "设计", "运营", "个人", "学习"} {
					group(c, 292, float32(215+i*25), t, 0, blue)
				}
				txt(c, 528, 214, 12, "# 发布   # 阻断   # 体验", muted, 400)
				txt(c, 528, 244, 12, "# 交接   # 同步   # 洞察", muted, 400)
			}
		case "settings-data":
			txt(c, 275, 74, 12, "导出经过校验的恢复包，或事务性导入。", muted, 500)
			txt(c, 284, 109, 12, "导出数据 (JSON)", blue, 500)
			txt(c, 400, 109, 12, "导入数据…", muted, 500)
		case "settings-privacy":
			s.settingsPrivacy(c)
		case "settings-sync":
			s.settingsSync(c)
		case "settings-provider":
			s.settingsProvider(c)
		default:
			s.settingsGeneral(c)
		}
	})
}
func (s *app) settingPair(c *ui.Context, key int, y float32, title, left, right string) {
	txt(c, 275, y, 11.5, title, muted, 600)
	y += 26
	box(c, 275, y, 320, 21, ui.Hex("#efefef"), 5).Border(.6, line)
	x := float32(275)
	if s.done[key] {
		x += 160
	}
	box(c, x, y, 160, 21, white, 5).Border(.6, ui.Hex("#d1d1d1"))
	txt(c, 341, y+1, 13, left, ink, 400)
	txt(c, 489, y+1, 13, right, ink, 400)
	labelButton(c, "pair-left-"+title, 275, y, 160, 21, "选择"+left, func() { s.done[key] = false })
	labelButton(c, "pair-right-"+title, 435, y, 160, 21, "选择"+right, func() { s.done[key] = true })
}
func (s *app) settingCheck(c *ui.Context, key int, x, y float32, label string, initial bool) {
	checked := s.done[key]
	if initial {
		checked = !checked
	}
	bg := white
	if checked {
		bg = ui.Hex("#1681ff")
	}
	box(c, x, y+2, 13, 13, bg, 3).Border(.7, line)
	if checked {
		txt(c, x+1, y-1, 13, "✓", white, 700)
	}
	txt(c, x+19, y, 12.5, label, ink, 400)
	labelButton(c, fmt.Sprintf("setting-check-%d", key), x, y, 230, 20, label, func() { s.done[key] = !s.done[key] })
}
func (s *app) settingsGeneral(c *ui.Context) {
	txt(c, 275, 74, 12, "只影响本机偏好，不改变任务事实。", muted, 500)
	s.settingPair(c, 900, 103, "外观", "冷灰", "微暖纸感")
	s.settingPair(c, 901, 166, "语言", "中文", "English")
	s.settingPair(c, 902, 229, "页面滑动方向", "翻书方向", "反向")
	txt(c, 275, 285, 11.5, "同时用于 Day Todo 与日历；翻书方向为左滑到下一天或下个月。", muted, 400)
	for i, title := range []string{"全局快速记录", "全局飞光速记"} {
		y := float32(316 + i*181)
		txt(c, 275, y, 11.5, title, muted, 600)
		s.settingCheck(c, 903+i, 275, y+32, "启用全局快捷键", true)
		shortcut := "⌃⇧N"
		if i == 1 {
			shortcut = "⌃⇧I"
		}
		box(c, 668, y+28, 159, 21, white, 5).Border(.7, line)
		txt(c, 729, y+29, 13, shortcut, ink, 500)
		status := "已启用 · 未发现系统冲突"
		if s.done[903+i] {
			status = "已停用"
		}
		txt(c, 275, y+62, 11.5, status, green, 600)
		txt(c, 275, y+87, 12, "恢复默认  "+shortcut, ui.Hex("#007aff"), 500)
		txt(c, 275, y+112, 11.5, "晷迹运行时，即使主窗口关闭也可使用；完全退出晷迹后失效。", muted, 400)
		txt(c, 275, y+136, 11.5, "macOS 无法列出所有 App 的自定义快捷键。晷迹不会改写其他 App 的快捷键设置；如果组合重复，可能两", muted, 400)
		txt(c, 275, y+151, 11.5, "个 App 同时响应，也可能只有其中一个响应。保存后请在常用 App 中试按。", muted, 400)
	}
}
func (s *app) settingsPrivacy(c *ui.Context) {
	txt(c, 275, 74, 12, "Provider 请求和 AI 建议必须 fail-closed。", muted, 500)
	rows := []string{"发送远程请求前必须展示授权范围。", "不发送数据库文件、内部 ID、Keychain 值或同步端点配置。", "创建、排期、变更、延续、废弃或标签写入都必须由用户确认。", "Provider 失败不影响 Day Todo、任务池、日历和数据包。"}
	cols := []ui.Color{blue, ui.Hex("#b54b2d"), green, muted}
	for i, t := range rows {
		y := float32(103 + i*25)
		box(c, 275, y+5, 6, 6, cols[i], 3)
		txt(c, 289, y, 12, t, muted, 400)
	}
	rule(c, 275, 207, 551)
	txt(c, 275, 222, 12, "本机诊断记录", ink, 600)
	txt(c, 275, 247, 11.5, "仅保留白名单化的阶段、计数、耗时和错误码；不含任务正文、凭证或同步路径，不会自动上传。最多占用 4", muted, 400)
	txt(c, 275, 263, 11.5, "MiB，最长保留 7 天。", muted, 400)
	txt(c, 275, 289, 11, "已保存 4 条证据，占用 16 KB；丢弃 0 条，损坏排除 0 条，超限排除 0 条。", muted, 400)
	txt(c, 371, 319, 12, "导出诊断资料…", blue, 500)
	txt(c, 645, 319, 12, "清除本机诊断记录", ui.Hex("#bd4a2a"), 500)
}
func (s *app) settingsSync(c *ui.Context) {
	txt(c, 275, 74, 12, "本机优先，按需同步。", muted, 500)
	s.settingPair(c, 920, 104, "数据模式", "本地优先", "在线优先")
	txt(c, 275, 165, 11.5, "同步端点只用于数据副本；本机任务仍可独立使用。", muted, 400)
	txt(c, 275, 202, 12, "本地优先云同步", muted, 600)
	s.settingCheck(c, 921, 275, 231, "启用云同步", false)
	pill(c, 460, 230, 62, "未启用", muted, side)
	s.settingPair(c, 922, 269, "同步端点", "iCloud", "本地文件夹")
	s.settingPair(c, 923, 337, "同步方式", "手动", "自动")
	txt(c, 285, 404, 12, "立即同步", faint, 500)
	txt(c, 385, 404, 11.5, "云同步未启用", muted, 400)
	for i, r := range []string{"iCloud Drive 位置       未配置", "最近同步时间             尚未同步", "最近有效同步时间       尚未同步"} {
		y := float32(447 + i*37)
		txt(c, 275, y, 12, r, muted, 400)
		rule(c, 275, y+28, 551)
	}
	txt(c, 275, 599, 11, "源码推导的界面预览 · 无公开截图 · 不执行同步", muted, 400)
}
func (s *app) settingsProvider(c *ui.Context) {
	txt(c, 275, 74, 12, "配置 Provider、权限边界与自动化。", muted, 500)
	txt(c, 275, 108, 12, "Provider 配置", muted, 600)
	s.settingCheck(c, 930, 275, 138, "启用 Provider", false)
	pill(c, 440, 138, 62, "未启用", muted, side)
	txt(c, 275, 179, 11, "Provider 类型", muted, 600)
	box(c, 275, 202, 320, 25, white, 5).Border(.7, line)
	txt(c, 286, 206, 12, "OpenAI Compatible                          ⌄", ink, 400)
	labels := []string{"名称", "Base URL", "模型", "API Key"}
	values := []string{"自定义 Provider", "https://api.example.com/v1", "gpt-4.1-mini / llama3.1", "未配置"}
	for i, l := range labels {
		y := float32(245 + i*35)
		txt(c, 275, y+5, 12, l, muted, 500)
		box(c, 366, y, 460, 27, white, 5).Border(.7, line)
		txt(c, 377, y+5, 12, values[i], faint, 400)
	}
	txt(c, 285, 398, 12, "保存", blue, 500)
	txt(c, 342, 398, 12, "测试连接", muted, 500)
	txt(c, 426, 398, 12, "清除", ui.Hex("#bd4a2a"), 500)
	txt(c, 275, 440, 12, "权限上限", muted, 600)
	for i, l := range []string{"读取数据", "远程发送"} {
		y := float32(470 + i*34)
		txt(c, 275, y, 12, l, muted, 400)
		box(c, 367, y-3, 320, 25, side, 5).Border(.7, line)
		txt(c, 384, y, 11.5, "每次询问          允许          禁止", ink, 400)
	}
	s.settingCheck(c, 931, 275, 548, "启用烛龙页面", false)
	s.settingCheck(c, 932, 275, 579, "启用自动分类", false)
	txt(c, 275, 625, 11, "源码推导的界面预览 · 无公开截图 · 不保存凭证或调用 API", muted, 400)
}
func (s *app) captureView(c *ui.Context) {
	w, h := c.Size()
	k := w / 520
	fly := s.page == "flylight-quick-capture"
	ui.Box(c).Absolute().Size(520, h/k).TransformOrigin(0, 0).Scale(k, k).Clip().Background(ui.Hex("#000000")).Children(func() {
		bottom := float32(165)
		if fly {
			bottom = 262
		}
		box(c, 0, 22, 520, bottom-22, ui.Hex("#fdfdfd"), 0)
		title := "快速记录"
		sub := "任务会加入今天的 Day Todo；当前浏览日期不会改变。"
		if fly {
			title = "飞光速记"
			sub = "内容会进入「飞光」时间线；支持 Markdown，回车换行，⌘回车保存，Esc 关闭。"
		}
		txt(c, 20, 38, 16, title, ink, 700)
		txt(c, 20, 63, 12, sub, muted, 500)
		if !fly {
			ui.TextInputBase(c.Key("capture-task"), &s.draft).Absolute().Left(20).Top(93).Size(480, 21).Padding(1, 10).FontSize(13).Background(white).Radius(5).Border(.7, line).Placeholder("输入今天要完成的任务，可附加 #标签、@分组").PlaceholderColor(faint).Label("快速记录输入")
			box(c, 382, 128, 43, 21, white, 6).Border(.7, line)
			txt(c, 390, 129, 13, "取消", ink, 500)
			labelButton(c, "cancel-capture", 382, 128, 43, 21, "取消快速记录", func() { s.dismissUtility("day", true) })
			bg := ui.Hex("#8ab5ff")
			if s.draft != "" {
				bg = blue
			}
			box(c, 432, 128, 68, 21, bg, 6)
			txt(c, 440, 129, 13, "添加任务", ui.Hex("#6995cf"), 500)
		} else {
			box(c, 20, 91, 480, 113, ui.Hex("#fafafb"), 13).Border(1.5, blue)
			ui.TextAreaBase(c.Key("capture-note"), &s.draft).Absolute().Left(30).Top(99).Size(458, 56).Padding(6, 6).FontSize(14).Background(ui.Hex("#fafafb")).Placeholder("@工").PlaceholderColor(ink).Label("飞光速记输入")
			rule(c, 21, 161, 478)
			txt(c, 36, 175, 13, "#     @     Aa", muted, 700)
			txt(c, 255, 175, 11, "⌘↵ 发布", muted, 500)
			box(c, 304, 167, 77, 31, ui.Hex("#fdfdfd"), 8).Border(.7, line)
			txt(c, 317, 175, 12, "稍后继续", muted, 600)
			labelButton(c, "capture-later", 304, 167, 77, 31, "稍后继续", func() { s.dismissUtility("flylight", false) })
			box(c, 390, 167, 102, 31, blue, 8)
			txt(c, 420, 175, 12, "发布 ↑", white, 700)
			box(c, 20, 208, 480, 36, white, 7).Border(.8, line)
			box(c, 29, 217, 18, 18, teal.Mix(white, .9), 3)
			txt(c, 33, 219, 11, "@", teal, 600)
			txt(c, 55, 218, 12, "@ 工程", ink, 600)
		}
	})
}
