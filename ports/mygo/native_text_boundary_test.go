// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestTitleNativeBoundaryRequiresLiveOwner(t *testing.T) {
	for _, finish := range []bool{false, true} {
		if snapshot, ok := snapshotTitleNative(ui.Handle{}, nil, finish); ok || snapshot.Text != "" || snapshot.Composing {
			t.Fatal("invented native input")
		}
	}
}
func TestTitleNativeBoundaryBetweenFrames(t *testing.T) {
	var context *ui.Context
	var handle ui.Handle
	draft := "原题"
	duringFrameSucceeded := false
	tt := ui.NewTester(func(c *ui.Context) {
		context = c
		ui.TextArea(c.Key("title"), &draft).Bind(&handle).Label("native-title")
		_, ok := snapshotTitleNative(handle, c, true)
		duringFrameSucceeded = duringFrameSucceeded || ok
	}, 320, 200)
	if duringFrameSucceeded {
		t.Fatal("finished editor during build")
	}
	b, ok := tt.Find("native-title")
	if !ok {
		t.Fatal("missing text control")
	}
	tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("新标题")
	snapshot, ok := snapshotTitleNative(handle, context, true)
	if !ok || snapshot.Composing || snapshot.Text != draft || snapshot.Text != "新标题" {
		t.Fatalf("native snapshot=%+v draft=%q ok=%v", snapshot, draft, ok)
	}
}
