// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"github.com/egoist/mygo/ui"
	"math"
)

// Source: App/NoonmarkMacApp/Theme.swift:71–94,184–209 (coolGray,
// normal contrast). Keep Swift's normalized sRGB expressions as authority;
// MyGo Color stores 8-bit channels, so quantize once at the renderer boundary.
func sourceRGB(r, g, b float64) ui.Color {
	return ui.RGB(uint8(math.Round(r*255)), uint8(math.Round(g*255)), uint8(math.Round(b*255)))
}

var sourceVisual = struct {
	background, panel, panel2, sidebar, text1, text2, text3, placeholder, terminal ui.Color
	line, line2, chip, controlFill, hover, accent, accentSoft, success, warning    ui.Color
}{
	background: sourceRGB(.994, .993, .991), panel: sourceRGB(1, 1, 1),
	panel2: sourceRGB(.984, .984, .987), sidebar: sourceRGB(.982, .981, .978),
	text1: sourceRGB(.11, .11, .12), text2: sourceRGB(.42, .42, .46),
	text3: sourceRGB(.455, .455, .495), placeholder: sourceRGB(.60, .60, .64),
	// MacUIDesignContract.swift:274–278, not placeholder or secondary text.
	terminal: sourceRGB(.627451, .627451, .658824),
	line:     sourceRGB(.90, .902, .914), line2: sourceRGB(.80, .805, .835),
	chip: sourceRGB(.951, .951, .958), controlFill: sourceRGB(.996, .996, .997),
	hover: sourceRGB(.977, .979, .984), accent: sourceRGB(.16, .38, .78),
	accentSoft: sourceRGB(.93, .955, 1), success: sourceRGB(.04, .45, .30),
	warning: sourceRGB(.706, .302, .204),
}

// WorkspaceShellViews.swift:145–186 and MacUIIconMetrics. DIP, not pixels.
const (
	// AccessibilityPresentationPolicy.swift:35–37, default transparency.
	navLayeredOpacity         float32 = .78
	navRowHeight              float32 = 34
	navRadius                 float32 = 8
	navInset                  float32 = 9
	navCompactInset           float32 = 7
	navLeading                float32 = 14
	navIconFrame              float32 = 19 // WorkspaceShellViews.swift:206
	navIconSize               float32 = 14
	navTextGap                float32 = 10
	navFontSize               float32 = 14.5
	pageHeadingSize           float32 = 20  // PageHeader.swift
	dayTitleSize              float32 = 13  // DayTodoPage.swift
	classificationRadius      float32 = 3   // TaskClassificationBadges.swift
	classificationFillOpacity float32 = .10 // MacUITaskLabelPatchLayout
	completionDiameter        float32 = 18
)

func navLabelStyle(active bool) (ui.Color, int) {
	if active {
		return sourceVisual.text1, 600
	}
	return sourceVisual.text2, 500
}
func taskTitleStyle(completed bool) (ui.Color, int) {
	if completed {
		return sourceVisual.terminal, 400
	}
	return sourceVisual.text1, 500
}

// Keep selection independent of focus. This recipe applies only to navigation,
// not task selection, utility buttons, date circles or composer surfaces.
func navButton(c *ui.Context, key string, x, y, w float32, active bool, label string, fn func(), content func()) {
	b := ui.ButtonBase(c.Key(key)).Absolute().Left(x).Top(y).Size(w, navRowHeight).Label(label).Radius(navRadius)
	if active {
		b.Background(sourceVisual.accentSoft.Alpha(navLayeredOpacity))
	} else if b.Hovered() {
		b.Background(sourceVisual.panel.Alpha(.72)).Border(1, sourceVisual.line.Alpha(.45))
	}
	b.Children(content)
	if b.Clicked() && fn != nil {
		fn()
	}
}
