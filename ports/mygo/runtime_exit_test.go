// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"errors"
	"testing"
)

func TestRuntimeExitStatusCleansBeforeReturningFailure(t *testing.T) {
	for _, failed := range []bool{false, true} {
		var err error
		want := 0
		if failed {
			err = errors.New("synthetic event-loop startup failure")
			want = 1
		}
		order := []string{}
		code := runtimeExitStatus(err, func() error { order = append(order, "drain writer", "drain dispatcher", "close service"); return nil })
		if code != want || len(order) != 3 || order[2] != "close service" {
			t.Fatal("exit status or cleanup lost", code, order)
		}
	}
}
