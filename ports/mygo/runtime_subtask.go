// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskview"
)

// Optional capability keeps the ordinary task boundary and its test doubles
// unchanged. All calls occur on the existing shared writer lane.
type subtaskStore interface {
	AddPlannedSubtask(core.TaskChainID, core.TaskDefinitionID, string, core.Difficulty) (core.PlannedSubtaskID, error)
	SavePlannedSubtaskTitleAcknowledged(core.TaskChainID, core.TaskDefinitionID, core.PlannedSubtaskID, string) (string, error)
	SetPlannedSubtaskDifficulty(core.TaskChainID, core.TaskDefinitionID, core.PlannedSubtaskID, core.Difficulty) error
	RemovePlannedSubtask(core.TaskChainID, core.TaskDefinitionID, core.PlannedSubtaskID) error
	AddSubtask(core.TaskChainID, core.DayTraceID, string, core.Difficulty) (core.SubtaskID, error)
	SaveSubtaskTitleAcknowledged(core.TaskChainID, core.DayTraceID, core.SubtaskID, string) (string, error)
	SetSubtaskDifficulty(core.TaskChainID, core.DayTraceID, core.SubtaskID, core.Difficulty) error
	DeleteSubtask(core.TaskChainID, core.DayTraceID, core.SubtaskID) error
	SetSubtaskCompleted(core.TaskChainID, core.DayTraceID, core.SubtaskID, bool) error
}

type runtimeSubtaskTarget struct {
	chain      core.TaskChainID
	trace      core.DayTraceID
	definition core.TaskDefinitionID
	child      string
}

func (t runtimeSubtaskTarget) pool() bool { return t.trace == "" }
func (t runtimeSubtaskTarget) route() string {
	return strings.Join([]string{"subtask", string(t.chain), string(t.trace), string(t.definition), t.child}, "|")
}
func parseSubtaskRoute(route string) (runtimeSubtaskTarget, bool) {
	p := strings.Split(route, "|")
	if len(p) != 5 || p[0] != "subtask" || p[1] == "" || p[4] == "" || (p[2] == "") == (p[3] == "") {
		return runtimeSubtaskTarget{}, false
	}
	return runtimeSubtaskTarget{core.TaskChainID(p[1]), core.DayTraceID(p[2]), core.TaskDefinitionID(p[3]), p[4]}, true
}
func (h *titleHub) saveTitleAcknowledged(route, title string) (string, error) {
	if !strings.HasPrefix(route, "subtask|") {
		return h.store.SaveTraceTitleInputAcknowledged(core.DayTraceID(route), title)
	}
	target, ok := parseSubtaskRoute(route)
	store, available := h.store.(subtaskStore)
	if !ok || !available {
		return "", core.ErrInvalidInput
	}
	if target.pool() {
		return store.SavePlannedSubtaskTitleAcknowledged(target.chain, target.definition, core.PlannedSubtaskID(target.child), title)
	}
	return store.SaveSubtaskTitleAcknowledged(target.chain, target.trace, core.SubtaskID(target.child), title)
}
func (s *app) subtaskStore() (subtaskStore, bool) {
	if s.titles == nil {
		return nil, false
	}
	store, ok := s.titles.store.(subtaskStore)
	return store, ok
}

type runtimeSubtaskRow struct {
	target                      runtimeSubtaskTarget
	title                       string
	difficulty                  core.Difficulty
	status                      core.SubtaskStatus
	position                    int
	completed, editable, toggle bool
	retained                    bool
}

func actualSubtaskRows(g core.TaskGraph, chain core.TaskChainID, trace core.DayTraceID, today core.LocalDate) []runtimeSubtaskRow {
	var parent *core.DayTrace
	for i := range g.Traces {
		if g.Traces[i].ID == trace && g.Traces[i].ChainID == chain {
			parent = &g.Traces[i]
			break
		}
	}
	if parent == nil {
		return nil
	}
	editable := parent.Status == core.TracePending && parent.Date.Compare(today) >= 0
	for _, day := range g.Days {
		if day.Date == parent.Date && day.LockedAt != nil {
			editable = false
		}
	}
	var rows []runtimeSubtaskRow
	for _, child := range g.Subtasks {
		if child.TraceID != trace || child.Status == core.SubtaskCancelledDraft {
			continue
		}
		rows = append(rows, runtimeSubtaskRow{target: runtimeSubtaskTarget{chain: chain, trace: trace, child: string(child.ID)}, title: child.Title, difficulty: child.Difficulty, status: child.Status, position: child.Position, completed: child.CompletedAt != nil, editable: editable, toggle: editable && parent.Date == today && (child.Status == core.SubtaskPending || child.Status == core.SubtaskCompleted)})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].position < rows[j].position })
	return rows
}
func plannedSubtaskRows(g core.TaskGraph, chain core.TaskChainID, definition core.TaskDefinitionID, today core.LocalDate) []runtimeSubtaskRow {
	editable := false
	for _, r := range rowsFor(g, "pool", today, today) {
		if r.chain == chain {
			editable = true
		}
	}
	var rows []runtimeSubtaskRow
	for _, d := range g.Definitions {
		if d.ChainID != chain || d.ID != definition {
			continue
		}
		editable = editable && d.SupersededAt == nil
		for _, child := range d.PlannedSubtasks {
			rows = append(rows, runtimeSubtaskRow{target: runtimeSubtaskTarget{chain: chain, definition: definition, child: string(child.ID)}, title: child.Title, difficulty: child.Difficulty, position: child.Position, editable: editable})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].position < rows[j].position })
	return rows
}
func subtaskPrefix(surface, child string) string { return surface + ".subtask." + child }
func subtaskOwner(surface, child string) string {
	return "subtask:" + subtaskPrefix(surface, child) + ".title"
}
func (s *app) childRows(surface string, parent runtimeSubtaskTarget) []runtimeSubtaskRow {
	g, today := s.runtime.Snapshot(), s.runtime.Today()
	var rows []runtimeSubtaskRow
	if parent.pool() {
		rows = plannedSubtaskRows(g, parent.chain, parent.definition, today)
	} else {
		rows = actualSubtaskRows(g, parent.chain, parent.trace, today)
	}
	present := map[string]bool{}
	for i := range rows {
		r := &rows[i]
		e := s.subtaskEditors[subtaskOwner(surface, r.target.child)]
		retained := e != nil && e.subtask != nil && e.subtask.target != r.target
		if e != nil && e.locked && e.registered && s.titles != nil {
			if state, ok := s.titles.coordinator.State(e.reg); ok && (state.Dirty || state.LastError != nil) {
				retained = true
			}
		}
		if retained {
			// A definition fork can preserve child ID while changing its owner.
			// Render the old object's draft explicitly, never as the new saved row.
			*r = *e.subtask
			r.retained, r.editable, r.toggle = true, false, false
		}
		present[r.target.child] = true
	}
	// A remotely deleted or returned object cannot destroy its native draft.
	// Keep its original read-only surface until lifecycle disposal has proof.
	for _, e := range s.titleEditors() {
		if e.subtask == nil || e.owner != subtaskOwner(surface, e.subtask.target.child) {
			continue
		}
		t := e.subtask.target
		if t.chain != parent.chain || t.trace != parent.trace || t.definition != parent.definition || present[t.child] {
			continue
		}
		r := *e.subtask
		r.editable = false
		r.toggle = false
		r.retained = true
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].position < rows[j].position })
	return rows
}
func (h *titleHub) publishSubtaskEditors(s *app, g core.TaskGraph, today core.LocalDate) {
	needsDisposal := false
	for _, e := range s.titleEditors() {
		if e.subtask == nil || !e.registered {
			continue
		}
		target := e.subtask.target
		var rows []runtimeSubtaskRow
		if target.pool() {
			rows = plannedSubtaskRows(g, target.chain, target.definition, today)
		} else {
			rows = actualSubtaskRows(g, target.chain, target.trace, today)
		}
		found, editable := false, false
		for _, r := range rows {
			if r.target != target {
				continue
			}
			found, editable = true, r.editable
			*e.subtask = r
			_ = h.coordinator.SynchronizePersistedText(e.reg, e.identity, r.title)
			break
		}
		if !found || !editable {
			if !e.locked {
				e.locked = true
				if err := h.coordinator.LockTrace(e.reg); err != nil {
					e.failure = err
					s.runtimeError = titleFailureMessage
				}
			}
			state, ok := h.coordinator.State(e.reg)
			if ok && !state.Dirty && !state.InFlight && !state.AwaitingVerification && state.LastError == nil {
				needsDisposal = true
			}
		}
	}
	if needsDisposal {
		s.disposeTitleEditors()
	}
}

// Source: DayTodoPage.swift:1080–1238; SubtaskDifficultyDotMenu.swift:4–83.
const (
	subtaskControlSpacing  float32 = 4
	subtaskRowSpacing      float32 = 6
	subtaskCheckSize       float32 = 15
	subtaskCheckRadius     float32 = 5
	subtaskHitSize         float32 = 28
	subtaskDotSize         float32 = 10
	subtaskDifficultyWidth float32 = 12
	subtaskComposerRadius  float32 = 7
	subtaskNativeSize      float32 = 12.5 // MarkdownEditing.style.subtask native font, wrapper readonly font is 12.
	subtaskNativeInset     float32 = 8.5
)

func difficultyTitle(d core.Difficulty) string {
	switch d {
	case core.DifficultySimple:
		return "简单"
	case core.DifficultyMedium:
		return "中等"
	case core.DifficultyHard:
		return "困难"
	}
	return ""
}
func difficultyColor(d core.Difficulty) ui.Color {
	switch d {
	case core.DifficultyMedium:
		return blue.Alpha(.72)
	case core.DifficultyHard:
		return sourceVisual.warning.Alpha(.92)
	}
	return sourceVisual.text3.Alpha(.58)
}
func (s *app) subtaskRowHeight(c *ui.Context, width float32, surface string, r runtimeSubtaskRow) float32 {
	title := r.title
	if e := s.subtaskEditors[subtaskOwner(surface, r.target.child)]; e != nil {
		title = e.native.draft
	}
	right := float32(0)
	if r.editable || r.target.pool() {
		right = 32
	} else if r.completed {
		right = 13
	}
	_, height := dayMeasure(c, "subtask-height-"+surface+r.target.child, max(float32(20), width-48-right), subtaskNativeSize, 500, title)
	result := min(float32(120), max(float32(32), height+17))
	if r.retained {
		result += 14
	}
	return result
}
func (s *app) liveSubtaskSection(c *ui.Context, width float32, surface string, parent runtimeSubtaskTarget, add bool) {
	rows := s.childRows(surface, parent)
	for i, r := range rows {
		if i > 0 {
			ui.Box(c).Height(subtaskRowSpacing)
		}
		height := s.subtaskRowHeight(c, width, surface, r)
		ui.Box(c.Key(subtaskPrefix(surface, r.target.child) + ".row")).Width(width).Height(height).Children(func() { s.liveSubtaskRow(c, width, height, surface, r) })
	}
	// Failed explicit submissions stay bound to their original target, even if
	// another window has returned/completed the parent or forked its definition.
	// This recovery-only field is not a new normal Swift composer surface.
	if surface != "day-list" {
		failed := s.failedSubtaskComposers(parent.chain)
		if len(failed) > 0 {
			for _, composer := range failed {
				ui.Box(c).Height(subtaskRowSpacing)
				ui.Text(c, s.subtaskRecoveryLabel(composer)).FontSize(10.5).TextColor(sourceVisual.warning).Width(width)
				ui.Box(c).Height(4)
				s.liveSubtaskComposer(c, width, composer.surface, composer.target)
			}
			return
		}
	}
	if add {
		if len(rows) > 0 {
			ui.Box(c).Height(subtaskRowSpacing)
		}
		s.liveSubtaskComposer(c, width, surface, parent)
	} else if len(rows) == 0 && surface != "day-list" {
		ui.Text(c, "暂无子任务").FontSize(11).TextColor(muted)
	}
}
func (s *app) liveSubtaskRow(c *ui.Context, width, height float32, surface string, r runtimeSubtaskRow) {
	prefix := subtaskPrefix(surface, r.target.child)
	if r.retained {
		txt(c, 48, height-12, 9, "原对象的未保存草稿", sourceVisual.warning, 400)
		height -= 14
	}
	if e := s.subtaskEditors[subtaskOwner(surface, r.target.child)]; e != nil && (e.locked || e.subtask.target != r.target) {
		r.editable, r.toggle = false, false
	}
	store, available := s.subtaskStore()
	drawCheck := func() {
		color, fill := sourceVisual.line2, white
		if r.status == core.SubtaskCompleted {
			color, fill = green, green
		}
		box(c, 6.5, 6.5, subtaskCheckSize, subtaskCheckSize, fill, subtaskCheckRadius).Border(1.5, color)
		if r.status == core.SubtaskCompleted {
			txt(c, 10, 7.5, 9, "✓", white, 400)
		}
	}
	if r.target.pool() {
		ui.Box(c).Absolute().Size(28, 28).IgnoreInput(true).Children(drawCheck)
	} else {
		check := ui.ButtonBase(c.Key(prefix+".completion")).Absolute().Size(28, 28).Label(prefix + ".completion").Description(map[bool]string{false: "标记完成", true: "撤销完成"}[r.status == core.SubtaskCompleted]).Disabled(!r.toggle || !available)
		check.Children(drawCheck)
		if check.Clicked() && r.toggle && available {
			completed := r.status != core.SubtaskCompleted
			s.subtaskAction(fmt.Sprintf("%s:complete:%t", r.target.route(), completed), func() error {
				return store.SetSubtaskCompleted(r.target.chain, r.target.trace, core.SubtaskID(r.target.child), completed)
			})
		}
	}
	navigation := uint64(0)
	var window *titleWindow
	if s.titles != nil {
		window = s.titles.windows[s.titleWindowID]
		if window != nil {
			navigation = window.navigation
		}
	}
	// The opaque window instance, not its reusable display ID/navigation count,
	// separates a queued native menu choice from any later Service attachment.
	menu := ui.ButtonBase(c.Key(struct {
		Owner      string
		Navigation uint64
		Window     *titleWindow
	}{prefix + ".difficulty", navigation, window})).Absolute().Left(32).Size(12, 28).Label(prefix + ".difficulty").Description("修改子任务难度").Value(difficultyTitle(r.difficulty)).Disabled(!r.editable || !available)
	menu.Children(func() {
		box(c, 1, 9, 10, 10, difficultyColor(r.difficulty), 5).Border(.75, sourceVisual.line2.Alpha(.65)).IgnoreInput(true)
	})
	if available {
		menu.Menu(func(m *ui.Menu) {
			for _, d := range []core.Difficulty{core.DifficultySimple, core.DifficultyMedium, core.DifficultyHard} {
				if m.Item(difficultyTitle(d)).Checked(d == r.difficulty).Disabled(!r.editable).Chosen() && r.editable {
					d := d
					s.subtaskAction(r.target.route()+":difficulty:"+fmt.Sprint(d), func() error {
						if r.target.pool() {
							return store.SetPlannedSubtaskDifficulty(r.target.chain, r.target.definition, core.PlannedSubtaskID(r.target.child), d)
						}
						return store.SetSubtaskDifficulty(r.target.chain, r.target.trace, core.SubtaskID(r.target.child), d)
					})
				}
			}
		})
	}
	right := float32(0)
	if r.editable || r.target.pool() {
		right = 32
	} else if r.completed {
		right = 13
	}
	ui.Box(c).Absolute().Left(48).Size(max(float32(20), width-48-right), height).Children(func() { s.liveSubtaskTitle(c, max(float32(20), width-48-right), height, surface, r) })
	if r.completed && !r.editable && !r.target.pool() {
		icon(c, width-9, 9, 9, "lock", sourceVisual.text3)
	}
	if r.editable && available {
		b := ui.ButtonBase(c.Key(prefix+".delete")).Absolute().Left(width-28).Size(28, 28).Label(prefix + ".delete").Description("移除子任务")
		b.Children(func() {
			if r.target.pool() {
				icon(c, 9, 9, 10, "x", sourceVisual.text3)
			} else {
				icon(c, 9, 9, 10, "trash", sourceVisual.text3)
			}
		})
		if b.Clicked() {
			s.subtaskAction(r.target.route()+":delete", func() error {
				if r.target.pool() {
					return store.RemovePlannedSubtask(r.target.chain, r.target.definition, core.PlannedSubtaskID(r.target.child))
				}
				return store.DeleteSubtask(r.target.chain, r.target.trace, core.SubtaskID(r.target.child))
			})
		}
	}
}
func (s *app) liveSubtaskTitle(c *ui.Context, width, height float32, surface string, r runtimeSubtaskRow) {
	owner := subtaskOwner(surface, r.target.child)
	prefix := subtaskPrefix(surface, r.target.child)
	e := s.subtaskEditors[owner]
	h := s.titles
	_, available := s.subtaskStore()
	canCreate := r.editable && available && h != nil && h.windows[s.titleWindowID] != nil && h.windows[s.titleWindowID].commands == 0 && !s.livePickerOpen()
	if e == nil && !canCreate {
		color := ink
		if r.status == core.SubtaskCompleted {
			color = sourceVisual.terminal
		}
		text := ui.Text(c, r.title).Width(width).FontSize(12).TextColor(color).Label(prefix + ".title")
		if r.status == core.SubtaskCompleted {
			text.Strikethrough()
		}
		return
	}
	if e == nil {
		if s.subtaskEditors == nil {
			s.subtaskEditors = map[string]*runtimeTitleEditor{}
		}
		h.nextEditor++
		captured := r
		e = &runtimeTitleEditor{subtask: &captured, owner: owner, native: titleNative{draft: r.title}, seenText: r.title, key: titleEditorKey{owner, h.nextEditor, 1}}
		for _, trace := range h.graph.Traces {
			if trace.ID == r.target.trace {
				e.row = taskview.DayRow{CalendarRow: taskview.CalendarRow{ChainID: trace.ChainID, TraceID: trace.ID, Title: currentTitle(h.graph, trace.ChainID, trace.DefinitionID), Status: trace.Status}}
				break
			}
		}
		s.subtaskEditors[owner] = e
	}
	e.native.context = c
	color := ink
	if r.status == core.SubtaskCompleted {
		color = sourceVisual.terminal
	}
	control := ui.TextAreaBase(c.Key(e.key), &e.native.draft).Bind(&e.native.handle).Width(width).Height(height).FontSize(subtaskNativeSize).FontWeight(500).TextColor(color).Padding(subtaskNativeInset, 0).FixedLineHeight(15).Background(ui.Transparent).Border(0, ui.Transparent).Label(prefix + ".title").Placeholder("子任务").ReadOnly(e.locked)
	if r.status == core.SubtaskCompleted {
		control.Strikethrough()
	}
	control.OnTextKey(func(k ui.TextKey) bool {
		if k.Key != ui.KeyEnter || k.Mods != 0 {
			return false
		}
		reg, id := e.reg, e.identity
		h.post(func() {
			if s.subtaskEditors[owner] == e && e.registered {
				if current, ok := h.coordinator.Identity(reg); ok && current == id {
					_ = h.coordinator.EndEditing(reg)
				}
			}
		})
		return true
	})
	if !e.registered && !e.queued {
		e.queued = true
		h.post(func() {
			if s.subtaskEditors[owner] == e {
				h.registerEditor(s, e)
			}
		})
	}
	text, compose := e.native.draft, control.Composing()
	if e.registered && (text != e.seenText || compose != e.seenCompose) {
		reg, id := e.reg, e.identity
		h.post(func() { _ = h.coordinator.Observe(reg, id) })
	}
	e.seenText, e.seenCompose = text, compose
	if e.registered && control.FocusLeft() {
		reg, id := e.reg, e.identity
		h.post(func() {
			if current, ok := h.coordinator.Identity(reg); ok && current == id {
				_ = h.coordinator.EndEditing(reg)
			}
		})
	}
}
func (s *app) subtaskAction(key string, fn func() error) {
	s.titleTransition(func() { s.runAction(key, fn) })
}

const subtaskComposerFailureMessage = "子任务添加尚未确认，草稿已保留。请重试添加，或明确清空原对象草稿后再关闭。"

type runtimeSubtaskComposer struct {
	failure                      error
	finalizedText                string
	finalizedValid               bool
	carryoverRecovery            carryoverRecoveryReason
	surface                      string
	target                       runtimeSubtaskTarget
	native                       titleNative
	pending                      bool
	mounted                      bool
	acknowledged                 string
	hasAcknowledged, clearQueued bool
}

func (s *app) liveSubtaskComposer(c *ui.Context, width float32, surface string, parent runtimeSubtaskTarget) {
	_, available := s.subtaskStore()
	if !available {
		return
	}
	id := string(parent.trace)
	if parent.pool() {
		id = string(parent.chain)
	}
	key := subtaskPrefix(surface, id) + ".new|" + parent.route()
	if s.subtaskComposers == nil {
		s.subtaskComposers = map[string]*runtimeSubtaskComposer{}
	}
	composer := s.subtaskComposers[key]
	if composer == nil {
		composer = &runtimeSubtaskComposer{target: parent, surface: surface}
		s.subtaskComposers[key] = composer
	}
	composer.native.context = c
	composer.mounted = true
	composer.finalizedValid = false
	if composer.hasAcknowledged && !composer.clearQueued {
		composer.clearQueued = true
		s.titles.post(func() { composer.clearQueued = false; s.clearAcknowledgedComposer(composer) })
	}
	ui.TextAreaBase(c.Key(key), &composer.native.draft).Bind(&composer.native.handle).Width(width).Height(32).FontSize(subtaskNativeSize).FontWeight(500).Padding(8.5, 10).FixedLineHeight(15).Background(sourceVisual.panel2).Border(0, ui.Transparent).Radius(subtaskComposerRadius).Label(subtaskPrefix(surface, id) + ".new").Placeholder("添加子任务").OnTextKey(func(k ui.TextKey) bool {
		if k.Key != ui.KeyEnter || k.Mods != 0 {
			return false
		}
		s.titles.post(func() { s.submitSubtaskComposer(composer) })
		return true
	})
}
func (s *app) finalizeSubtaskComposers() bool {
	return s.finalizeComposerOwners(true)
}

func (s *app) finalizeComposerOwners(leaving bool) bool {
	for _, composer := range s.subtaskComposers {
		if !composer.mounted || (!leaving && composer.carryoverRecovery != carryoverRecoveryNone) {
			continue
		}
		composer.finalizedValid = false
		if snapshot, ok := composer.native.Finish(); !ok {
			s.runtimeError = "子任务输入尚未能安全读取，草稿已保留。"
			return false
		} else {
			composer.finalizedText, composer.finalizedValid = snapshot.Text, !snapshot.Composing
			if !snapshot.Composing && strings.TrimSpace(snapshot.Text) == "" {
				composer.failure = nil
				if !composer.pending && !composer.hasAcknowledged {
					composer.carryoverRecovery = carryoverRecoveryNone
				}
			}
			if s.titles != nil && s.titles.carryoverInFlight[carryoverSourceKey{composer.target.chain, composer.target.trace}] > 0 {
				s.retainCarryoverComposer(composer, snapshot, true, carryoverRecoveryUnconfirmed)
			}
		}
	}
	return true
}
func (s *app) submitSubtaskComposer(composer *runtimeSubtaskComposer) {
	if composer.pending || !composer.mounted {
		return
	}
	if composer.hasAcknowledged {
		s.clearAcknowledgedComposer(composer)
		if composer.hasAcknowledged {
			return
		}
		// If the old acknowledgment belonged to different text, this same Return
		// still submits the user's newly committed text. Do not swallow the gesture.
	}
	snapshot, ok := composer.native.Finish()
	if !ok || snapshot.Composing {
		s.runtimeError = "子任务输入尚未能安全读取，草稿已保留。"
		return
	}
	raw := snapshot.Text
	title := strings.TrimSpace(raw)
	if title == "" {
		return
	}
	store, available := s.subtaskStore()
	if !available {
		return
	}
	h := s.titles
	target := composer.target
	composer.pending = true
	accepted := h.commandOnce(s, target.route()+":add:"+title, func() error {
		if target.pool() {
			_, err := store.AddPlannedSubtask(target.chain, target.definition, title, core.DifficultySimple)
			return err
		}
		_, err := store.AddSubtask(target.chain, target.trace, title, core.DifficultySimple)
		return err
	}, func(err error) {
		composer.pending = false
		composer.failure = err
		if err == nil {
			composer.acknowledged, composer.hasAcknowledged = raw, true
			s.clearAcknowledgedComposer(composer)
			if !s.hasTitleFailure() && !s.hasSubtaskComposerFailure() {
				s.runtimeError = ""
				s.runtimeNotice = "已保存到隔离开发会话"
			}
		} else {
			s.runtimeError = subtaskComposerFailureMessage
			s.runtimeNotice = ""
		}
	})
	if !accepted {
		composer.pending = false
	}
}

func (s *app) canAddSubtask(target runtimeSubtaskTarget) bool {
	if _, available := s.subtaskStore(); !available {
		return false
	}
	g := s.runtime.Snapshot()
	if target.pool() {
		for _, row := range rowsFor(g, "pool", s.runtimeDate, s.runtime.Today()) {
			if row.chain != target.chain {
				continue
			}
			for _, d := range g.Definitions {
				if d.ID == target.definition && d.ChainID == target.chain && d.SupersededAt == nil {
					return true
				}
			}
		}
		return false
	}
	return actualSubtaskTargetEditable(g, s.runtime.Today(), target)
}

// This exact predicate also controls guarded native-owner retirement.
func actualSubtaskTargetEditable(g core.TaskGraph, today core.LocalDate, target runtimeSubtaskTarget) bool {
	for _, t := range g.Traces {
		if t.ID != target.trace || t.ChainID != target.chain || t.Status != core.TracePending || t.Date.Compare(today) < 0 {
			continue
		}
		for _, day := range g.Days {
			if day.Date == t.Date && day.LockedAt != nil {
				return false
			}
		}
		return true
	}
	return false
}
func (s *app) retainSubtaskParents(rows []taskview.DayRow, g core.TaskGraph) []taskview.DayRow {
	seen := map[core.DayTraceID]bool{}
	for _, r := range rows {
		seen[r.TraceID] = true
	}
	for _, e := range s.titleEditors() {
		if e.subtask == nil || e.owner != subtaskOwner("day-list", e.subtask.target.child) || seen[e.subtask.target.trace] {
			continue
		}
		for _, t := range g.Traces {
			if t.ID == e.subtask.target.trace && t.Date == s.runtimeDate {
				r := e.row
				r.Status = t.Status
				rows = append(rows, r)
				seen[t.ID] = true
				break
			}
		}
	}
	for _, composer := range s.failedSubtaskComposers("") {
		if composer.target.trace == "" || seen[composer.target.trace] {
			continue
		}
		for _, t := range g.Traces {
			if t.ID == composer.target.trace && t.Date == s.runtimeDate {
				rows = append(rows, taskview.DayRow{CalendarRow: taskview.CalendarRow{ChainID: t.ChainID, TraceID: t.ID, Title: currentTitle(g, t.ChainID, t.DefinitionID), Status: t.Status}})
				seen[t.ID] = true
				break
			}
		}
	}
	return rows
}
func (s *app) livePoolSubtaskDetail(c *ui.Context, w, h float32, g core.TaskGraph) bool {
	var target runtimeSubtaskTarget
	title := ""
	for _, r := range rowsFor(g, "pool", s.runtimeDate, s.runtime.Today()) {
		if r.key != s.runtimeSelection {
			continue
		}
		for _, d := range g.Definitions {
			if d.ChainID == r.chain && d.SupersededAt == nil {
				target = runtimeSubtaskTarget{chain: r.chain, definition: d.ID}
				title = d.Title
				break
			}
		}
	}
	if target.chain == "" {
		for _, e := range s.titleEditors() {
			if e.subtask != nil && e.subtask.target.pool() && string(e.subtask.target.chain) == s.runtimeSelection {
				target = e.subtask.target
				target.child = ""
				title = currentTitle(g, target.chain, target.definition)
				break
			}
		}
	}
	if target.chain == "" {
		for _, composer := range s.failedSubtaskComposers(core.TaskChainID(s.runtimeSelection)) {
			if string(composer.target.chain) == s.runtimeSelection {
				target = composer.target
				title = currentTitle(g, target.chain, target.definition)
				break
			}
		}
	}
	if target.chain == "" {
		return false
	}
	box(c, 0, 0, 1, h, line, 0)
	txt(c, 14, 18, 12, "任务池", muted, 600)
	calendarPaneButton(c, "pool-close-detail", w-38, 14, "››", "关闭任务池详情", func() { s.titleTransition(func() { s.runtimeSelection = "" }) })
	ui.Scroll(c.Key("pool-task-detail-scroll")).Absolute().Left(14).Top(56).Size(w-28, h-70).Children(func() {
		ui.Text(c, title).FontSize(14).FontWeight(600).TextColor(ink).Width(w - 28)
		ui.Box(c).Height(14)
		ui.Text(c, "任务池不形成 Day Todo 轨迹。").FontSize(11).TextColor(sourceVisual.text3).Width(w - 28)
		ui.Box(c).Height(14)
		s.liveSubtaskSection(c, w-28, "pool", target, s.canAddSubtask(target))
		ui.Box(c).Height(24)
		ui.Text(c, "分类、完整 Markdown 与撤销恢复尚未接通。").FontSize(10.5).TextColor(muted).Width(w - 28)
	})
	return true
}

// Offscreen successful adds are cleared only after remounting and a fresh
// native proof. Newer text and marked input are never cleared by an old ack.
func (s *app) clearAcknowledgedComposer(composer *runtimeSubtaskComposer) {
	if !composer.hasAcknowledged || !composer.mounted {
		return
	}
	current, valid := composer.native.Snapshot()
	if !valid || current.Composing {
		return
	}
	if current.Text != composer.acknowledged || composer.native.Replace("") {
		composer.hasAcknowledged = false
		fresh, ok := composer.native.Snapshot()
		if ok && !fresh.Composing && strings.TrimSpace(fresh.Text) == "" && !composer.pending && composer.failure == nil {
			composer.carryoverRecovery = carryoverRecoveryNone
		}
	}
}

// Source disclosure is intrinsic: chevron9 + spacing4 + horizontal padding14.
func subtaskDisclosureWidth(c *ui.Context, key string, count int) float32 {
	textWidth, _ := dayMeasure(c, "subtask-disclosure-width-"+key, 0, 11, 600, subtaskDisclosureText(count))
	return textWidth + 27
}

func (s *app) hasSubtaskComposerFailure() bool {
	for _, composer := range s.subtaskComposers {
		if composer.failure != nil || composer.carryoverRecovery != carryoverRecoveryNone {
			return true
		}
	}
	return false
}

// A failed explicit Add is not an unsubmitted transient draft. Ordinary close
// is not consent to discard it. Navigation retains object-keyed composers so
// the user can return to retry; only close/quit need this additional barrier.
func (s *app) subtaskComposersCanClose() bool {
	for _, composer := range s.subtaskComposers {
		if composer.failure == nil && composer.carryoverRecovery == carryoverRecoveryNone {
			continue
		}
		if composer.mounted {
			snapshot, ok := composer.native.Snapshot()
			if ok && !snapshot.Composing && strings.TrimSpace(snapshot.Text) == "" && !composer.pending && !composer.hasAcknowledged {
				// An explicit user edit to blank is a native-proven discard, never an
				// assistant-side timeout, stale binding or failed-save acknowledgment.
				composer.failure = nil
				composer.carryoverRecovery = carryoverRecoveryNone
				continue
			}
		}
		s.runtimeError = subtaskComposerFailureMessage
		s.runtimeNotice = ""
		return false
	}
	return true
}

// Recovery is discoverable from either current parent detail. The original
// surface and object target never change; Return still invokes that target.
func (s *app) failedSubtaskComposers(chain core.TaskChainID) []*runtimeSubtaskComposer {
	keys := make([]string, 0, len(s.subtaskComposers))
	for key, c := range s.subtaskComposers {
		if (c.failure != nil || c.carryoverRecovery != carryoverRecoveryNone) && (chain == "" || c.target.chain == chain) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := make([]*runtimeSubtaskComposer, 0, len(keys))
	for _, key := range keys {
		result = append(result, s.subtaskComposers[key])
	}
	return result
}
func (s *app) subtaskRecoveryLabel(c *runtimeSubtaskComposer) string {
	origin := "原任务池版本"
	if !c.target.pool() {
		origin = "原日任务"
		for _, t := range s.runtime.Snapshot().Traces {
			if t.ID == c.target.trace {
				origin = "原日期 " + t.Date.String()
				break
			}
		}
	}
	if c.carryoverRecovery != carryoverRecoveryNone {
		return origin + "的未提交输入已保留；可复制或清空。"
	}
	return origin + "的新增草稿尚未确认；可重试或清空。"
}
func subtaskDisclosureText(count int) string {
	if count == 0 {
		return "未保存草稿"
	}
	return fmt.Sprintf("%d 子任务", count)
}
