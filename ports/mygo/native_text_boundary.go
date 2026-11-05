// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
)

// snapshotTitleNative is called on the owning UI thread between build frames.
// It deliberately never substitutes the application draft if the native owner
// is missing. Finalization commits already-delivered marked text, not future
// OS events. The caller still owns session identity, persistence and close.
func snapshotTitleNative(handle ui.Handle, context *ui.Context, finalize bool) (titlesession.NativeSnapshot, bool) {
	var snapshot ui.TextInputSnapshot
	var ok bool
	if finalize {
		snapshot, ok = handle.FinishTextEditing(context)
	} else {
		snapshot, ok = handle.TextInputSnapshot(context)
	}
	if !ok {
		return titlesession.NativeSnapshot{}, false
	}
	return titlesession.NativeSnapshot{Text: snapshot.Text, Composing: snapshot.Composing}, true
}
