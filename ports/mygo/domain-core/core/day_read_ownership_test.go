// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

// This inventory intentionally includes values as well as references. Any new
// field or changed nested shape requires new ownership fixtures and assertions.
func TestDayReadOwnershipFieldInventory(t *testing.T) {
	inventories := []struct {
		value  any
		fields string
	}{
		{DayTrace{}, "ID:core.DayTraceID;ChainID:core.TaskChainID;DefinitionID:core.TaskDefinitionID;Date:core.LocalDate;Status:core.TraceStatus;Priority:int;PinOrder:*int;ContinuationSeq:int;DescriptionText:*string;NoteEntries:[]core.TaskNoteEntry;ManualProgressPercent:*int;CarriedFromTraceID:*core.DayTraceID;ChangedToTraceID:*core.DayTraceID;CreatedAt:core.Instant;ContentUpdatedAt:core.Instant;CompletedAt:*core.Instant;SettledAt:*core.Instant;DraftCancellationID:*core.CancellationID;DraftCancelledOn:*core.LocalDate;"},
		{TaskNoteEntry{}, "ID:core.TaskNoteEntryID;Body:string;CreatedAt:core.Instant;UpdatedAt:core.Instant;DeletedAt:*core.Instant;"},
		{LocalDate{}, "Year:int;Month:int;Day:int;"},
	}
	for _, x := range inventories {
		typ := reflect.TypeOf(x.value)
		got := ""
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			got += f.Name + ":" + f.Type.String() + ";"
		}
		if got != x.fields {
			t.Fatalf("update ownership inventory and fixtures for %s: %s", typ, got)
		}
	}
	var shape func(reflect.Type) string
	shape = func(typ reflect.Type) string {
		switch typ.Kind() {
		case reflect.Pointer, reflect.Slice:
			return typ.Kind().String() + "(" + shape(typ.Elem()) + ")"
		case reflect.Struct:
			s := "struct("
			for i := 0; i < typ.NumField(); i++ {
				s += shape(typ.Field(i).Type) + ","
			}
			return s + ")"
		default:
			return typ.Kind().String()
		}
	}
	want := "struct(string,string,string,struct(int,int,int,),string,int,ptr(int),int,ptr(string),slice(struct(string,string,uint64,uint64,ptr(uint64),)),ptr(int),ptr(string),ptr(string),uint64,uint64,ptr(uint64),ptr(uint64),ptr(string),ptr(struct(int,int,int,)),)"
	if got := shape(reflect.TypeOf(DayTrace{})); got != want {
		t.Fatalf("nested reference shape changed: %s", got)
	}
}

// Independently walk mutable storage, without consulting cloneDayTrace's list.
// Slice element addresses cover backing arrays, including spare capacity.
func dayReadAddresses(v reflect.Value, out map[uintptr]bool) {
	switch v.Kind() {
	case reflect.Interface:
		if !v.IsNil() {
			dayReadAddresses(v.Elem(), out)
		}
	case reflect.Pointer:
		if !v.IsNil() {
			out[v.Pointer()] = true
			dayReadAddresses(v.Elem(), out)
		}
	case reflect.Slice:
		if v.IsNil() {
			return
		}
		full := v.Slice(0, v.Cap())
		for i := 0; i < full.Len(); i++ {
			e := full.Index(i)
			if e.CanAddr() {
				out[e.Addr().Pointer()] = true
			}
			dayReadAddresses(e, out)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			dayReadAddresses(v.Field(i), out)
		}
	}
}
func dayReadDisjoint(t *testing.T, a, b any) {
	t.Helper()
	aa, bb := map[uintptr]bool{}, map[uintptr]bool{}
	dayReadAddresses(reflect.ValueOf(a), aa)
	dayReadAddresses(reflect.ValueOf(b), bb)
	for p := range aa {
		if bb[p] {
			t.Fatal("mutable storage aliases across ownership boundary")
		}
	}
}

// Pure clone specimen, deliberately not a valid domain lifecycle: cancellation,
// completion and movement pointers are all present to exercise the copy surface.
func dayReadFullSpecimen() DayTrace {
	return DayTrace{ID: DayTraceID(identity(70)), ChainID: TaskChainID(identity(71)), DefinitionID: TaskDefinitionID(identity(72)), Date: date("2026-11-05"), Status: TracePending, Priority: 7, PinOrder: ptr(4), ContinuationSeq: 3, DescriptionText: ptr("description"), NoteEntries: []TaskNoteEntry{{ID: TaskNoteEntryID(identity(73)), Body: "active", CreatedAt: instant(-2), UpdatedAt: instant(0)}, {ID: TaskNoteEntryID(identity(74)), Body: "deleted", CreatedAt: instant(-1), UpdatedAt: instant(2), DeletedAt: ptr(instant(2))}}, ManualProgressPercent: ptr(33), CarriedFromTraceID: ptr(DayTraceID(identity(75))), ChangedToTraceID: ptr(DayTraceID(identity(76))), CreatedAt: Instant(math.Float64bits(math.Copysign(0, -1))), ContentUpdatedAt: instant(0), CompletedAt: ptr(Instant(0x7ff8000000000001)), SettledAt: ptr(Instant(0xfff0000000000000)), DraftCancellationID: ptr(CancellationID(identity(77))), DraftCancelledOn: ptr(date("2026-11-06"))}
}
func dayReadMutate(t *DayTrace) {
	v := reflect.ValueOf(t).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() == reflect.Pointer && !f.IsNil() {
			f.Elem().Set(reflect.Zero(f.Type().Elem()))
		}
	}
	for i := range t.NoteEntries {
		n := &t.NoteEntries[i]
		n.ID = "changed"
		n.Body = "changed"
		n.CreatedAt = instant(400)
		n.UpdatedAt = instant(401)
		if n.DeletedAt != nil {
			*n.DeletedAt = instant(402)
		}
	}
	if len(t.NoteEntries) > 0 {
		t.NoteEntries[0] = TaskNoteEntry{Body: "replacement"}
	}
	t.NoteEntries = append(t.NoteEntries, TaskNoteEntry{Body: "append"})
	t.NoteEntries = t.NoteEntries[:0]
}
func TestDayReadOwnershipCloneAllFields(t *testing.T) {
	for _, notes := range []string{"nil", "empty", "populated"} {
		t.Run(notes, func(t *testing.T) {
			src := dayReadFullSpecimen()
			if notes == "nil" {
				src.NoteEntries = nil
			}
			if notes == "empty" {
				src.NoteEntries = []TaskNoteEntry{}
			}
			if notes == "populated" {
				ns := make([]TaskNoteEntry, len(src.NoteEntries), 8)
				copy(ns, src.NoteEntries)
				src.NoteEntries = ns
			}
			clone := cloneDayTrace(src)
			graphClone := (TaskGraph{Traces: []DayTrace{src, src}}).Clone()
			if !reflect.DeepEqual(src, clone) || !reflect.DeepEqual(src, graphClone.Traces[0]) {
				t.Fatal("clone loses values, nil shape or exact Instant bits")
			}
			dayReadDisjoint(t, src, clone)
			dayReadDisjoint(t, src, graphClone)
			dayReadDisjoint(t, clone, graphClone)
			dayReadDisjoint(t, graphClone.Traces[0], graphClone.Traces[1])
			dayReadMutate(&clone)
			if !reflect.DeepEqual(src, graphClone.Traces[0]) {
				t.Fatal("clone mutation escaped")
			}
		})
	}
}
func dayReadOwnedGraph() TaskGraph {
	g := baseGraph()
	g.Traces[0].PinOrder = ptr(4)
	g.Traces[0].DescriptionText = ptr("synthetic description")
	g.Traces[0].ManualProgressPercent = ptr(25)
	notes := make([]TaskNoteEntry, 2, 7)
	notes[0] = TaskNoteEntry{ID: TaskNoteEntryID(identity(80)), Body: "active", CreatedAt: instant(100), UpdatedAt: instant(100)}
	notes[1] = TaskNoteEntry{ID: TaskNoteEntryID(identity(81)), Body: "", CreatedAt: instant(100), UpdatedAt: instant(100), DeletedAt: ptr(instant(100))}
	g.Traces[0].NoteEntries = notes
	return g
}
func TestDayReadOwnershipPublicBoundaries(t *testing.T) {
	input := dayReadOwnedGraph()
	k := kernel(t, input)
	d := input.Traces[0].Date
	baseline := k.Snapshot()
	first, second := k.DayTodo(d), k.DayTodo(d)
	for _, pair := range [][2]any{{input, first}, {baseline, first}, {first, second}, {input, baseline}} {
		dayReadDisjoint(t, pair[0], pair[1])
	}
	dayReadMutate(&first[0])
	if !reflect.DeepEqual(baseline, k.Snapshot()) || !reflect.DeepEqual(second, k.DayTodo(d)) {
		t.Fatal("returned mutation reached kernel or another result")
	}
	dayReadMutate(&input.Traces[0])
	snap := k.Snapshot()
	dayReadMutate(&snap.Traces[0])
	if !reflect.DeepEqual(baseline, k.Snapshot()) || !reflect.DeepEqual(second, k.DayTodo(d)) {
		t.Fatal("input or snapshot mutation reached retained result")
	}
}
func TestDayReadOwnershipRetainedAcrossCommands(t *testing.T) {
	k := kernel(t, dayReadOwnedGraph())
	d := date("2026-11-05")
	id := DayTraceID(identity(4))
	old := k.DayTodo(d)
	want := k.DayTodo(d)
	commands := []func() error{
		func() error {
			return k.RenameTaskTitle(EditTaskTitleInput{ChainID: TaskChainID(identity(2)), Title: "新合成标题", Today: d, Now: instant(100), NewDefinitionID: TaskDefinitionID(identity(160))})
		},
		func() error { return k.MarkCompleted(id, d, instant(101)) },
		func() error { return k.UndoCompleted(id, d, instant(102)) },
		func() error {
			return k.SetManualProgress(SetManualProgressInput{ChainID: TaskChainID(identity(2)), TraceID: id, Today: d, Now: instant(103), Percent: 50})
		},
		func() error {
			return k.DeferCurrentTrace(TraceCarryoverInput{ChainID: TaskChainID(identity(2)), TraceID: id, NewTraceID: DayTraceID(identity(90)), DayID: DayID(identity(91)), TargetDate: date("2026-11-06"), Today: d, Now: instant(104)})
		},
	}
	for i, command := range commands {
		if err := command(); err != nil {
			t.Fatalf("command %d: %v", i, err)
		}
		if !reflect.DeepEqual(old, want) {
			t.Fatalf("command %d changed retained result", i)
		}
	}
	if err := k.MarkCompleted(id, d, instant(105)); err == nil {
		t.Fatal("expected rejected completion of deferred source")
	}
	if !reflect.DeepEqual(old, want) {
		t.Fatal("rejected command changed retained result")
	}
	next := k.DayTodo(date("2026-11-06"))
	if len(next) != 1 || next[0].CarriedFromTraceID == nil {
		t.Fatal("missing public carryover pointer")
	}
	dayReadDisjoint(t, next, k.Snapshot())
	dayReadMutate(&next[0])
	assertValid(t, k.Snapshot())
}

func TestDayReadOwnershipConcurrentCommands(t *testing.T) {
	k := kernel(t, dayReadOwnedGraph())
	d := date("2026-11-05")
	id := DayTraceID(identity(4))
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 8)
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 150; i++ {
				rows := k.DayTodo(d)
				for j := range rows {
					dayReadMutate(&rows[j])
				}
				if err := ValidateTaskGraph(k.Snapshot()); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 100; i++ {
			if err := k.MarkCompleted(id, d, instant(float64(110+2*i))); err != nil {
				errs <- err
				return
			}
			if err := k.UndoCompleted(id, d, instant(float64(111+2*i))); err != nil {
				errs <- err
				return
			}
		}
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	assertValid(t, k.Snapshot())
}

// Completing atomically clears the pin and sets status/time. Readers must see a
// full old or new object, never a mixture. Each iteration uses a fresh kernel so
// expected states are fixed independently of subsequent reads.
func TestDayReadOwnershipAtomicBeforeAfter(t *testing.T) {
	for n := 0; n < 40; n++ {
		g := dayReadOwnedGraph()
		k := kernel(t, g)
		d := g.Traces[0].Date
		before := k.DayTodo(d)
		reference := kernel(t, g)
		if err := reference.MarkCompleted(g.Traces[0].ID, d, instant(101)); err != nil {
			t.Fatal(err)
		}
		after := reference.DayTodo(d)
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make(chan error, 4)
		for r := 0; r < 3; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for i := 0; i < 30; i++ {
					got := k.DayTodo(d)
					if !reflect.DeepEqual(got, before) && !reflect.DeepEqual(got, after) {
						errs <- fmt.Errorf("mixed atomic completion state")
						return
					}
				}
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := k.MarkCompleted(g.Traces[0].ID, d, instant(101)); err != nil {
				errs <- err
			}
		}()
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
	}
}

func TestDayReadOwnershipSharedInputRows(t *testing.T) {
	g := dayReadOwnedGraph()
	c := g.Chains[0]
	c.ID = TaskChainID(identity(120))
	def := g.Definitions[0]
	def.ID = TaskDefinitionID(identity(121))
	def.ChainID = c.ID
	row := g.Traces[0]
	row.ID = DayTraceID(identity(122))
	row.ChainID = c.ID
	row.DefinitionID = def.ID
	row.Priority = 2
	// Input rows intentionally share all pointers and note backing storage.
	g.Chains = append(g.Chains, c)
	g.Definitions = append(g.Definitions, def)
	g.Traces = append(g.Traces, row)
	if err := ValidateTaskGraph(g); err != nil {
		t.Fatalf("%#v", err)
	}
	k := kernel(t, g)
	rows := k.DayTodo(row.Date)
	if len(rows) != 2 {
		t.Fatal("missing row")
	}
	want := k.DayTodo(row.Date)
	dayReadDisjoint(t, rows[0], rows[1])
	dayReadMutate(&rows[0])
	if !reflect.DeepEqual(rows[1], want[1]) || !reflect.DeepEqual(k.DayTodo(row.Date), want) {
		t.Fatal("row mutation reached another row or kernel")
	}
}

func TestDayReadOwnershipLifecyclePointers(t *testing.T) {
	for _, completed := range []bool{false, true} {
		t.Run(fmt.Sprint(completed), func(t *testing.T) {
			k := kernel(t, dayReadOwnedGraph())
			d := date("2026-11-05")
			id := DayTraceID(identity(4))
			if completed {
				if err := k.MarkCompleted(id, d, instant(101)); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := k.SettleTaskDays(date("2026-11-06"), instant(101)); err != nil {
					t.Fatal(err)
				}
			}
			rows := k.DayTodo(d)
			want := k.DayTodo(d)
			if completed && rows[0].CompletedAt == nil {
				t.Fatal("missing completion pointer")
			}
			if !completed && rows[0].SettledAt == nil {
				t.Fatal("missing settlement pointer")
			}
			dayReadDisjoint(t, rows, k.Snapshot())
			dayReadMutate(&rows[0])
			if !reflect.DeepEqual(want, k.DayTodo(d)) {
				t.Fatal("terminal pointer mutation reached kernel")
			}
		})
	}
}

// This observation reports command end-to-end latency under reader contention,
// not pure mutex wait time: it includes the unchanged mutation clone/validation.
// There is deliberately no wall-clock pass threshold or runtime/GC adjustment.
func TestDayReadOwnershipWriterObservation(t *testing.T) {
	for sample := 0; sample < 3; sample++ {
		for _, mode := range []string{"old", "new"} {
			g := dayReadLoad(100, 4, 2)
			g.Subtasks = g.Subtasks[1:]
			k := kernel(t, g)
			d := g.Traces[0].Date
			trace := g.Traces[0]
			read := k.DayTodo
			if mode == "old" {
				read = func(d LocalDate) []DayTrace { return dayReadOldTodo(k, d) }
			}
			var wg sync.WaitGroup
			ready := make(chan struct{}, 2)
			done := make(chan struct{})
			for r := 0; r < 2; r++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					read(d)
					ready <- struct{}{}
					for {
						select {
						case <-done:
							return
						default:
							read(d)
						}
					}
				}()
			}
			<-ready
			<-ready
			timings := make([]time.Duration, 0, 60)
			var failure error
			for i := 0; i < 60; i++ {
				start := time.Now()
				err := k.SetManualProgress(SetManualProgressInput{ChainID: trace.ChainID, TraceID: trace.ID, Percent: i % 2, Today: d, Now: instant(float64(200 + i))})
				timings = append(timings, time.Since(start))
				if err != nil {
					failure = err
					break
				}
			}
			close(done)
			wg.Wait()
			if failure != nil {
				t.Fatal(failure)
			}
			sort.Slice(timings, func(i, j int) bool { return timings[i] < timings[j] })
			var total time.Duration
			for _, v := range timings {
				total += v
			}
			t.Logf("synthetic sample=%d mode=%s readers=2 unrelated=100 writes=%d command-latency mean=%s median=%s p95=%s max=%s (not isolated mutex wait)", sample, mode, len(timings), total/time.Duration(len(timings)), timings[len(timings)/2], timings[len(timings)*95/100], timings[len(timings)-1])
		}
	}
}

// Cancelling a future pending draft removes the newest history row and unmasks
// an older same-day deferred row. A split-lock old first pass/new second pass
// would incorrectly return nil, which is neither allowed complete state.
func TestDayReadOwnershipAtomicHistoryUnmask(t *testing.T) {
	for n := 0; n < 40; n++ {
		g := baseGraph()
		old := g.Traces[0]
		old.ID = DayTraceID(identity(150))
		old.Status = TraceDeferred
		old.Priority = 2
		old.CreatedAt = instant(90)
		old.SettledAt = ptr(instant(95))
		g.Traces = append(g.Traces, old)
		target := g.Traces[0]
		target.ID = DayTraceID(identity(153))
		target.Date = date("2026-11-06")
		target.Status = TraceCompleted
		target.CompletedAt = ptr(instant(100))
		target.CarriedFromTraceID = ptr(old.ID)
		g.Traces = append(g.Traces, target)
		g.Days = append(g.Days, Day{ID: DayID(identity(154)), Date: target.Date, CreatedAt: instant(100), UpdatedAt: instant(100)})
		if err := ValidateTaskGraph(g); err != nil {
			t.Fatalf("%#v", err)
		}
		k := kernel(t, g)
		d := g.Traces[0].Date
		before := k.DayTodo(d)
		cancel := func(k *TaskKernel) error {
			return k.ReturnToPool(ReturnToPoolInput{TraceID: g.Traces[0].ID, ChainID: g.Traces[0].ChainID, NewDefinitionID: TaskDefinitionID(identity(151)), CancellationID: CancellationID(identity(152)), Today: date("2026-11-04"), Now: instant(101)})
		}
		reference := kernel(t, g)
		if err := cancel(reference); err != nil {
			t.Fatal(err)
		}
		after := reference.DayTodo(d)
		if len(before) != 1 || before[0].ID != g.Traces[0].ID || len(after) != 1 || after[0].ID != old.ID {
			t.Fatal("independent expected history IDs failed")
		}
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make(chan error, 4)
		for r := 0; r < 3; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for i := 0; i < 40; i++ {
					got := k.DayTodo(d)
					if !reflect.DeepEqual(got, before) && !reflect.DeepEqual(got, after) {
						errs <- fmt.Errorf("mixed history passes: %+v", got)
						return
					}
				}
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := cancel(k); err != nil {
				errs <- err
			}
		}()
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
	}
}
