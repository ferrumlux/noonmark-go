// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskview"
)

func (s *app) openPoolPicker() {
	s.runtimePoolPicker = true
	s.runtimePoolFocus = "cancel"
	if rows, e := taskview.ProjectPool(s.runtime.Snapshot()); e == nil && len(rows) > 0 && s.runtimeDate.Compare(s.runtime.Today()) >= 0 {
		s.runtimePoolFocus = string(rows[0].ChainID)
	}
}
func (s *app) schedulePoolSelection(chain core.TaskChainID) {
	s.runCommand(func() error { return s.runtime.Schedule(chain, s.runtimeDate) }, false)
	if s.runtimeError != "" {
		return
	}
	// Schedule succeeds only after full aggregate persistence/readback. Resolve
	// the resulting trace from that published graph, never from a guessed ID.
	for _, t := range s.runtime.Snapshot().Traces {
		if t.ChainID == chain && t.Date == s.runtimeDate && t.Status == core.TracePending {
			s.runtimeSelection = string(t.ID)
			s.runtimeDayRail = true
			break
		}
	}
	s.runtimePoolPicker = false
	s.runtimePoolFocus = ""
}
func (s *app) livePoolPicker(c *ui.Context) {
	if !s.runtimePoolPicker {
		return
	}
	rows, err := taskview.ProjectPool(s.runtime.Snapshot())
	history := s.runtimeDate.Compare(s.runtime.Today()) < 0
	ui.DialogBase(c.Key("live-pool-picker"), &s.runtimePoolPicker, func(back, panel ui.Element) {
		// Swift supplies its native sheet chrome. This is explicitly MyGo's dialog
		// shell, while the source-owned content widths/padding/row radii match.
		back.Background(ink.Alpha(.18))
		panel.Width(400).Padding(16).Gap(12).Background(white).Radius(12).Shadow(0, 10, 30, 0, ink.Alpha(.2)).Label("从任务池排期对话框")
		ui.Text(c, "从任务池排期到这一天").FontSize(14).FontWeight(600).TextColor(ink).Width(368)
		switch {
		case err != nil:
			ui.Text(c, "任务池资料未通过验证，不能安全排期。").FontSize(11).TextColor(sourceVisual.warning).Width(368)
		case history:
			ui.Text(c, "任务池不能排期到过去日期。请先切到今天或未来日期。").FontSize(11).TextColor(muted).Width(368)
		case len(rows) == 0:
			ui.Box(c).Size(368, 58).Children(func() {
				icon(c, 172, 0, 24, "tray", teal)
				tw, _ := dayMeasure(c, "pool-empty-size", 0, 12.5, 400, "任务池是空的。")
				txt(c, (368-tw)/2, 34, 12.5, "任务池是空的。", sourceVisual.text3, 400)
			})
		default:
			heights := make([]float32, len(rows))
			total := float32(0)
			for i, r := range rows {
				_, th := dayMeasure(c, fmt.Sprintf("pool-title-size-%s", r.ChainID), 314, 13, 400, r.Title)
				heights[i] = max(float32(20), th) + 20
				if i > 0 {
					total += 6
				}
				total += heights[i]
			}
			ui.Scroll(c.Key("pool-picker-list")).Size(368, min(float32(320), total)).Label("任务池排期列表").Children(func() {
				for i, r := range rows {
					if i > 0 {
						ui.Box(c).Height(6)
					}
					r := r
					h := heights[i]
					b := ui.ButtonBase(c.Key("pool-picker-row-"+string(r.ChainID))).Size(368, h).Radius(7).Background(white).Label("排期任务 "+string(r.ChainID)).FocusBind(&s.runtimePoolFocus, string(r.ChainID)).FocusRing(true)
					b.Children(func() {
						txt(c, 10, 10, 13, r.Title, ink, 400).Width(314)
						_, sh := dayMeasure(c, "pool-schedule-height", 0, 13, 400, "排期")
						txt(c, 332, (h-sh)/2, 13, "排期", blue, 400)
					})
					if b.Clicked() {
						s.schedulePoolSelection(r.ChainID)
					}
				}
			})
		}
		if s.runtimeError != "" {
			ui.Text(c, s.runtimeError).FontSize(11).FontWeight(500).TextColor(sourceVisual.warning).Width(368).Label("任务池排期错误")
		}
		cancel := ui.ButtonBase(c.Key("pool-picker-cancel")).Size(368, 28).Radius(7).Background(sourceVisual.panel2).Label("取消从任务池排期").FocusBind(&s.runtimePoolFocus, "cancel").FocusRing(true)
		cancel.Children(func() {
			tw, _ := dayMeasure(c, "pool-cancel-width", 0, 12, 400, "取消")
			txt(c, (368-tw)/2, 5, 12, "取消", muted, 400)
		})
		if cancel.Clicked() {
			s.runtimePoolPicker = false
			s.runtimePoolFocus = ""
		}
	})
	if !s.runtimePoolPicker {
		s.runtimePoolFocus = ""
	}
}
