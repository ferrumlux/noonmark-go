// SPDX-License-Identifier: AGPL-3.0-only
package titlecoordinator

import (
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestTryEnqueueDrainOwnsAcceptedClosuresOnly(t *testing.T) {
	l := NewSerialLane()
	started, release := make(chan struct{}), make(chan struct{})
	var got []int
	if !l.TryEnqueue(func() { close(started); <-release; got = append(got, 1) }) {
		t.Fatal("initial rejection")
	}
	<-started
	if !l.TryEnqueue(func() { got = append(got, 2) }) {
		t.Fatal("queued rejection")
	}
	l.StopAfterDrain()
	l.StopAfterDrain()
	if l.TryEnqueue(func() { got = append(got, 3) }) {
		t.Fatal("accepted after stop")
	}
	select {
	case <-l.Done():
		t.Fatal("Done closed before accepted worker returned")
	default:
	}
	close(release)
	<-l.Done()
	if !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatal(got)
	}
}

func TestTryEnqueueRacesStopWithoutLosingAcceptedWork(t *testing.T) {
	l := NewSerialLane()
	started, release, race := make(chan struct{}), make(chan struct{}), make(chan struct{})
	if !l.TryEnqueue(func() { close(started); <-release }) {
		t.Fatal("initial rejection")
	}
	<-started
	const count = 64
	var wg sync.WaitGroup
	var accepted [count]bool
	var executed [count]atomic.Int32
	wg.Add(count + 1)
	for i := 0; i < count; i++ {
		go func(i int) {
			defer wg.Done()
			<-race
			accepted[i] = l.TryEnqueue(func() { executed[i].Add(1) })
		}(i)
	}
	go func() { defer wg.Done(); <-race; l.StopAfterDrain() }()
	close(race)
	wg.Wait()
	close(release)
	<-l.Done()
	for i := 0; i < count; i++ {
		want := int32(0)
		if accepted[i] {
			want = 1
		}
		if got := executed[i].Load(); got != want {
			t.Fatalf("task %d accepted=%v executed=%d", i, accepted[i], got)
		}
	}
	if l.TryEnqueue(func() { t.Error("late task ran") }) {
		t.Fatal("accepted after drain")
	}
}
