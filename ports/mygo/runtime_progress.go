// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"reflect"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskview"
)

const (
	progressFailureMessage         = "完成进度未通过验证，暂不显示；任务资料没有改写。"
	weightedProgressLabel          = "按子任务自动计算的完成进度"
	manualProgressLabel            = "手动完成进度"
	rowProgressWidth       float32 = 150
	rowProgressHeight      float32 = 4
	rowProgressGap         float32 = 6
	rowProgressTop         float32 = 5
	rowProgressFont        float32 = 10
	detailProgressHeight   float32 = 6
	detailProgressGap      float32 = 8
	detailProgressFont     float32 = 13
)

// UI-thread owned, shared by every window of a titleHub. Construction is only
// at initialization/confirmed publication, never per row or frame. Keep graph
// and projection together so a rejected graph cannot expose stale percentages
// as if they described newer facts. An unchanged snapshot reuses the instance.
type runtimeProgressCache struct {
	graph      core.TaskGraph
	projection *core.ProgressProjection
	navigation *runtimeNavigationProjection
	err        error
}

func newRuntimeProgressCache(g core.TaskGraph) *runtimeProgressCache {
	p := &runtimeProgressCache{}
	p.publish(g)
	return p
}
func (p *runtimeProgressCache) publish(g core.TaskGraph) bool {
	if p.projection != nil && reflect.DeepEqual(p.graph, g) {
		p.err = nil
		return true
	}
	next, err := core.NewProgressProjection(g)
	if err != nil {
		p.err = err
		return false
	}
	p.graph, p.projection, p.navigation, p.err = g.Clone(), next, newRuntimeNavigationProjection(g), nil
	return true
}
func (s *app) traceProgress(id core.DayTraceID) (core.TraceProgress, bool) {
	if s.progress == nil || s.progress.projection == nil || s.progress.err != nil {
		return core.TraceProgress{}, false
	}
	p := s.progress.projection.TraceProgress(id)
	// The core's missing-ID sentinel is 0/0; a known manual trace is 0/100.
	return p, p.TotalWeight > 0
}
func showsRowProgress(status core.TraceStatus, p core.TraceProgress) bool {
	if p.Percent <= 0 || p.Percent >= 100 {
		return false
	}
	return status == core.TracePending || status == core.TraceDeferred || status == core.TraceUnfinished
}
func progressLabel(p core.TraceProgress) string {
	if p.Mode == core.ProgressWeightedSubtasks {
		return weightedProgressLabel
	}
	return manualProgressLabel
}
func progressColor(percent int) ui.Color {
	if percent >= 100 {
		return sourceVisual.success
	}
	return sourceVisual.accent
}

type rowProgressLayout struct {
	progress                                           core.TraceProgress
	visible                                            bool
	headerHeight, titleTop, progressTop, height, width float32
}

func (s *app) measureRowProgress(c *ui.Context, row taskview.DayRow, textH float32) rowProgressLayout {
	p, available := s.traceProgress(row.TraceID)
	m := rowProgressLayout{progress: p, visible: available && showsRowProgress(row.Status, p)}
	contentHeight := textH
	if m.visible {
		pw, ph := dayMeasure(c, "progress-measure-"+string(row.TraceID), 0, rowProgressFont, 600, fmt.Sprintf("%d%%", p.Percent))
		m.height = max(rowProgressHeight, ph)
		m.width = rowProgressWidth + rowProgressGap + pw
		contentHeight += rowProgressTop + m.height
	}
	m.headerHeight = max(float32(28), contentHeight) + 16
	m.titleTop = (m.headerHeight - contentHeight) / 2
	m.progressTop = m.titleTop + textH + rowProgressTop
	return m
}

// Capsule geometry is independent of input controls; even manual values are
// readonly until the dedicated domain/Service setter is actually connected.
func staticRuntimeProgress(c *ui.Context, width, height float32, percent int) {
	box(c, 0, 0, width, height, sourceVisual.chip, height/2)
	fill := width * float32(min(100, max(0, percent))) / 100
	if fill > 0 {
		box(c, 0, 0, fill, height, progressColor(percent), height/2)
	}
}
func liveRowProgress(c *ui.Context, id string, m rowProgressLayout, inset float32) {
	if !m.visible {
		return
	}
	value := fmt.Sprintf("%d%%", m.progress.Percent)
	ui.Box(c.Key("day-row-progress-"+id)).Absolute().Left(52+inset).Top(m.progressTop+inset).Size(m.width, m.height).IgnoreInput(true).
		Role(ui.RoleProgress).Label(progressLabel(m.progress)).Value(value).Range(0, 100, float64(m.progress.Percent)).Children(func() {
		ui.Box(c.Key("track")).Absolute().Top((m.height-rowProgressHeight)/2).Size(rowProgressWidth, rowProgressHeight).Children(func() {
			staticRuntimeProgress(c, rowProgressWidth, rowProgressHeight, m.progress.Percent)
		})
		txt(c, rowProgressWidth+rowProgressGap, 0, rowProgressFont, value, progressColor(m.progress.Percent), 600)
	})
}
func (s *app) liveDetailProgress(c *ui.Context, width float32, id core.DayTraceID) {
	p, ok := s.traceProgress(id)
	if !ok {
		ui.Text(c, progressFailureMessage).FontSize(10.5).TextColor(sourceVisual.warning).Width(width)
		return
	}
	value := fmt.Sprintf("%d%%", p.Percent)
	var pw, ph float32
	theme := *c.Theme()
	theme.FontSize = detailProgressFont
	ui.Themed(c.Key("detail-progress-measure-"+string(id)), &theme).Absolute().Size(0, 0).IgnoreInput(true).Children(func() {
		pw, ph = c.MeasureText(0, ui.Span{Text: value, Size: detailProgressFont, Weight: 700, Features: "tnum"})
	})
	ui.Box(c.Key("day-detail-progress-"+string(id))).Size(width, ph+detailProgressGap+detailProgressHeight).
		Role(ui.RoleProgress).Label(progressLabel(p)).Value(value).Range(0, 100, float64(p.Percent)).Children(func() {
		txt(c, width-pw, 0, detailProgressFont, value, progressColor(p.Percent), 700).FontFeatures("tnum")
		ui.Box(c.Key("track")).Absolute().Top(ph+detailProgressGap).Size(width, detailProgressHeight).Children(func() {
			staticRuntimeProgress(c, width, detailProgressHeight, p.Percent)
		})
	})
}
