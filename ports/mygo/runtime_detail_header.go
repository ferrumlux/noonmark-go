// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskview"
)

const detailHeaderSize float32 = 11

// WorkspaceUIPrimitives.swift:870–958, standard StatusChip (984–1011).
// Internal cancelled drafts and unknown values have no visible presentation.
type detailStatusColors struct{ foreground, background, dot ui.Color }

func detailStatusStyle(status core.TraceStatus) (detailStatusColors, bool) {
	switch status {
	case core.TracePending:
		return detailStatusColors{sourceVisual.accent, sourceVisual.accentSoft, sourceVisual.accent}, true
	case core.TraceCompleted:
		return detailStatusColors{sourceVisual.success, sourceVisual.okSoft, sourceVisual.success}, true
	case core.TraceUnfinished:
		return detailStatusColors{sourceVisual.warning, sourceVisual.warnSoft, sourceVisual.warning}, true
	case core.TraceDeferred, core.TraceChanged, core.TraceReturnedToPool:
		return detailStatusColors{sourceVisual.text2, sourceVisual.chip, sourceVisual.text3}, true
	case core.TraceAbandoned:
		return detailStatusColors{sourceVisual.text3, sourceVisual.chip, sourceVisual.text3}, true
	default:
		return detailStatusColors{}, false
	}
}

type detailMetadataGeometry struct{ chipW, chipH, radius, chipY, dateX, dateY, height float32 }

func detailMetadataLayout(width, tw, th, dw, dh float32) detailMetadataGeometry {
	g := detailMetadataGeometry{chipW: 16 + 5 + 5 + tw, chipH: max(float32(5), th) + 4}
	g.radius = g.chipH / 2
	g.height = max(g.chipH, dh)
	g.chipY = (g.height - g.chipH) / 2
	g.dateX, g.dateY = g.chipW+8, (g.height-dh)/2
	// 8 DIP is the port's explicit adaptation of the default Swift HStack gap.
	if g.dateX+dw > width {
		g.chipY = 0
		g.dateX = 0
		g.dateY = g.chipH + 8
		g.height = g.dateY + dh
	}
	return g
}

// displayDate(.monthDay) + weekday(.weekday), zh_Hans_SG; fixed upstream
// AppDateTimeFormatter.swift:157–164 uses M月d日 and EEE, not fullDate.
func detailDateText(d core.LocalDate) string {
	return fmt.Sprintf("%d月%d日 周%s", d.Month, d.Day, weekdayNarrow(d))
}
func liveDetailMetadata(c *ui.Context, width float32, row taskview.DayRow, dateValue core.LocalDate) {
	style, visible := detailStatusStyle(row.Status)
	if !visible {
		return
	}
	text, date := statusText(row.Status), detailDateText(dateValue)
	tw, th := dayMeasure(c, "detail-status-measure", 0, 11, 500, text)
	dw, dh := dayMeasure(c, "detail-date-measure", 0, 11, 400, date)
	g := detailMetadataLayout(width, tw, th, dw, dh)
	ui.Box(c.Key("detail-metadata")).Size(width, g.height).Children(func() {
		box(c, 0, g.chipY, g.chipW, g.chipH, style.background, g.radius).PassThrough()
		box(c, 8, g.chipY+(g.chipH-5)/2, 5, 5, style.dot, 2.5).PassThrough()
		txt(c, 18, g.chipY+(g.chipH-th)/2, 11, text, style.foreground, 500).Label("日任务详情状态").Value(text)
		txt(c, g.dateX, g.dateY, 11, date, sourceVisual.text3, 400).Label("日任务详情日期").Value(date)
	})
}

// Native SF Symbols are unavailable on this renderer. A centred vector xmark
// preserves the 11 DIP glyph box without depending on fallback font metrics.
var detailXmark = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 11 11" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M1 1L10 10M10 1L1 10"/></svg>`))

func (s *app) detailCloseButton(c *ui.Context, x, y float32) {
	b := ui.ButtonBase(c.Key("day-detail-close")).Absolute().Left(x).Top(y).Size(28, 28).Radius(6).Label("关闭任务详情")
	b.Children(func() {
		ui.Icon(c, detailXmark).Absolute().Left(8.5).Top(8.5).Size(11, 11).TextColor(sourceVisual.text3)
	})
	if b.Clicked() {
		s.titleTransition(func() { s.runtimeSelection = "" })
	}
}
