// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"reflect"
	"strings"
	"testing"
)

func TestTitleStreamSafeCollisionPersistsExactBytes(t *testing.T) {
	a := strings.Repeat("\u0301", 31)
	b := strings.Repeat("\u0301", 30) + "\u034f\u0301"
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		s, path := fresh(t)
		must(t, s.AddToday(pair[0]))
		trace := s.Snapshot().Traces[0].ID
		rev := s.disk.revision
		actual, err := s.SaveTraceTitleInputAcknowledged(trace, pair[1])
		must(t, err)
		if actual != pair[1] || s.Snapshot().Definitions[0].Title != pair[1] || s.disk.revision <= rev {
			t.Fatal("distinct CGJ edit was treated as a no-op or acknowledged wrong bytes")
		}
		must(t, s.Close())
		reopened, err := Open(path, testDay)
		must(t, err)
		if reopened.Snapshot().Definitions[0].Title != pair[1] {
			t.Fatal("reopen changed CGJ title bytes")
		}
		must(t, reopened.Close())
	}
}

func TestTitleInvalidUTF8UpdateRejectsWithoutPersistence(t *testing.T) {
	for _, input := range []string{string([]byte{0xff}), "new" + string([]byte{0xc3}), "\xe2\x82", "\xed\xa0\x80"} {
		for _, mode := range []string{"input", "rename", "trace-ack"} {
			t.Run(mode, func(t *testing.T) {
				s, path := fresh(t)
				must(t, s.AddToday("original"))
				before, rev := s.Snapshot(), s.disk.revision
				chain, trace := before.Chains[0].ID, before.Traces[0].ID
				var err error
				switch mode {
				case "input":
					err = s.SaveTaskTitleInput(chain, input)
				case "rename":
					err = s.RenameTaskTitle(chain, input)
				case "trace-ack":
					var actual string
					actual, err = s.SaveTraceTitleInputAcknowledged(trace, input)
					if actual != "" {
						t.Error("malformed title returned acknowledgment")
					}
				}
				if err == nil {
					t.Error("invalid UTF-8 title update accepted")
				} else {
					t.Logf("rejected malformed title: %T: %v", err, err)
				}
				if !reflect.DeepEqual(before, s.Snapshot()) || s.disk.revision != rev {
					t.Error("invalid UTF-8 changed graph or disk revision")
				}
				must(t, s.Close())
				reopened, err := Open(path, testDay)
				must(t, err)
				defer reopened.Close()
				if !reflect.DeepEqual(before, reopened.Snapshot()) {
					t.Error("malformed title persisted to disk")
				}
			})
		}
	}
}
