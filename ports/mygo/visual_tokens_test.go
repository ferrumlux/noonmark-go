// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceVisualPalette(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  ui.Color
		hex  string
	}{
		{"primary", sourceVisual.text1, "#1c1c1f"},
		{"secondary", sourceVisual.text2, "#6b6b75"},
		{"accent", sourceVisual.accent, "#2961c7"},
		{"sidebar", sourceVisual.sidebar, "#fafaf9"},
		{"background", sourceVisual.background, "#fdfdfd"},
		{"panel", sourceVisual.panel, "#ffffff"},
		{"selected", sourceVisual.accentSoft, "#edf4ff"},
		{"terminal", sourceVisual.terminal, "#a0a0a8"},
		{"line", sourceVisual.line, "#e6e6e9"},
	} {
		if tc.got != ui.Hex(tc.hex) {
			t.Errorf("%s: got %+v want %s", tc.name, tc.got, tc.hex)
		}
	}
	if sourceVisual.background == sourceVisual.panel {
		t.Fatal("background must not become the white panel")
	}
	if sourceVisual.terminal == sourceVisual.text2 {
		t.Fatal("terminal and secondary text have distinct source roles")
	}
}
func TestSourceVisualComponentRecipes(t *testing.T) {
	if navRadius != 8 || classificationRadius != 3 || completionDiameter/2 != 9 {
		t.Fatal("component radii must remain distinct")
	}
	if navRowHeight != 34 || navInset != 9 || navCompactInset != 7 || navIconSize != 14 || navIconFrame != 19 || navLeading != 14 || navTextGap != 10 {
		t.Fatal("sidebar source geometry drift")
	}
	if navFontSize != 14.5 || pageHeadingSize != 20 || dayTitleSize != 13 {
		t.Fatal("source typography drift")
	}
	for _, active := range []bool{false, true} {
		color, weight := navLabelStyle(active)
		wantColor, wantWeight := sourceVisual.text2, 500
		if active {
			wantColor, wantWeight = sourceVisual.text1, 600
		}
		if color != wantColor || weight != wantWeight {
			t.Fatalf("nav active=%v", active)
		}
	}
	for _, done := range []bool{false, true} {
		color, weight := taskTitleStyle(done)
		wantColor, wantWeight := sourceVisual.text1, 500
		if done {
			wantColor, wantWeight = sourceVisual.terminal, 400
		}
		if color != wantColor || weight != wantWeight {
			t.Fatalf("task completed=%v", done)
		}
	}
}

// Headless state regression, not a claim about OS pointer/focus behavior.
func TestSourceNavigationHoverSurface(t *testing.T) {
	s := newApp()
	tt := ui.NewTester(s.view, 1200, 768)
	// The blank trailing interior is away from labels, glyphs and corner antialiasing.
	before := tt.Image().RGBAAt(175, 120)
	tt.Move(175, 120)
	hover := tt.Image().RGBAAt(175, 120)
	if before == hover {
		t.Fatal("idle navigation did not receive its hover fill")
	}
	tt.Move(900, 700)
	if got := tt.Image().RGBAAt(175, 120); got != before {
		t.Fatal("navigation hover fill persisted after exit")
	}
	// Selected navigation retains its source selection rather than hover whitening.
	before = tt.Image().RGBAAt(175, 83)
	tt.Move(175, 83)
	if got := tt.Image().RGBAAt(175, 83); got != before {
		t.Fatal("selected navigation must not acquire idle hover fill")
	}
}

// Optional externally rooted evidence uses the production view and synthetic
// domain commands, never screenshot fixture tasks in the runtime graph.
func TestSourceVisualEvidenceFrames(t *testing.T) {
	dir := os.Getenv("NOONMARK_UI_EVIDENCE_DIR")
	if dir == "" {
		return
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("absolute evidence path required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, tt, svc, _ := liveTest(t)
	for _, title := range []string{"合成：整理评审材料", "合成：确认明天议程", "合成：准备发布说明"} {
		liveType(t, tt, title)
		liveSubmit(t, tt)
	}
	liveClick(t, tt, "切换完成 "+string(svc.Snapshot().Traces[0].ID))
	save := func(name string) {
		t.Helper()
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, ui.Render(s.view, 1200, 768, 2))
		closeErr := f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	save("live-day-2x.png")
	liveClick(t, tt, "收起侧栏")
	save("live-day-compact-2x.png")
	liveClick(t, tt, "收起侧栏")
	if s.compact {
		t.Fatal("sidebar did not restore")
	}
}
