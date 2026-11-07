// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"errors"
	"sort"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	coordinator "github.com/ferrumlux/noonmark/ports/mygo/internal/titlecoordinator"
)

// Candidate only: no production startup calls enableCategoryCandidate.
// The Service, not this view, owns admission, natural-day preparation and writes.
type categoryStore interface {
	ReadView() (taskapp.OrdinaryReadView, error)
	ReplaceCategorySelectionOn(core.LocalDate, core.TaskChainID, ordinary.Choice, ordinary.UUID) taskapp.CategoryOutcome
}
type runtimeCategoryHub struct {
	store categoryStore
	view  taskapp.OrdinaryReadView
}
type runtimeCategoryEditor struct {
	chain             core.TaskChainID
	generation        uint64
	native            titleNative
	creating, pending bool
	failure           error
	savedUntil        time.Time
}

const categoryFailureMessage = "分组未保存，草稿已保留。"
const categoryNewColor = "#D87831"

// Enable only before attaching windows, with a real ordinary development Service.
func (h *titleHub) enableCategoryCandidate() error {
	if len(h.windows) != 0 {
		return taskapp.ErrSession
	}
	store, ok := h.store.(categoryStore)
	if !ok {
		return taskapp.ErrSession
	}
	view, err := store.ReadView()
	if err != nil {
		return err
	}
	h.category = &runtimeCategoryHub{store: store, view: view}
	h.publish(view.Graph, view.Today)
	return nil
}
func categoryCatalog(view taskapp.OrdinaryReadView) []ordinary.Category {
	out := make([]ordinary.Category, 0, len(view.Classification.Categories))
	for _, v := range view.Classification.Categories {
		if v.Lifecycle == "active" {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CanonicalKey != out[j].CanonicalKey {
			return out[i].CanonicalKey < out[j].CanonicalKey
		}
		return out[i].ID < out[j].ID
	})
	return out
}
func categoryCurrent(view taskapp.OrdinaryReadView, chain core.TaskChainID) ordinary.CategoryID {
	if r := view.Classification.CurrentByChainID[chain].Category; r != nil {
		return r.CategoryID
	}
	return ""
}
func (h *titleHub) categoryCanClose(s *app) bool {
	w := h.windows[s.titleWindowID]
	if w == nil || w.category == nil {
		return true
	}
	e := w.category
	if e.failure != nil {
		return false
	}
	if e.creating && !e.pending {
		snap, ok := e.native.Snapshot()
		return ok && !snap.Composing && snap.Text == ""
	}
	return true // admitted commands are protected by w.commands
}
func (h *titleHub) categoryCanNavigate(s *app) bool {
	w := h.windows[s.titleWindowID]
	if w == nil || w.category == nil {
		return true
	}
	e := w.category
	if e.pending || e.failure != nil {
		return false
	}
	if e.creating {
		snap, ok := e.native.Snapshot()
		return ok && !snap.Composing && snap.Text == ""
	}
	return true
}
func (s *app) categoryEditor(chain core.TaskChainID) *runtimeCategoryEditor {
	if s.titles == nil || s.titles.category == nil {
		return nil
	}
	w := s.titles.windows[s.titleWindowID]
	if w == nil {
		return nil
	}
	if w.category == nil || w.category.chain != chain {
		s.titles.nextEditor++
		w.category = &runtimeCategoryEditor{chain: chain, generation: s.titles.nextEditor}
	}
	return w.category
}

// categoryDisplayName bounds only the UI's single source D preprocessing pass.
// ordinary.budget.display prepays 3*len(raw) and checks D output <= input.
// Use division before D to avoid an overflowing multiplication. Native input
// backing storage and the Service's full transaction budget are separate.
func categoryDisplayName(raw string) (string, error) {
	if len(raw) > ordinary.DefaultLimits().MaxBytes/3 {
		return "", ordinary.ErrResourceLimit
	}
	name, err := canonical16.DisplayName(raw)
	if err != nil {
		return "", err
	}
	if len(name) > len(raw) {
		return "", ordinary.ErrResourceLimit
	}
	return name, nil
}

func (s *app) submitCategory(e *runtimeCategoryEditor, choice ordinary.Choice) {
	h := s.titles
	if h == nil || h.category == nil {
		return
	}
	w := h.windows[s.titleWindowID]
	if w == nil || w.category != e || e.pending {
		return
	}
	submitted := ""
	if choice.Kind == ordinary.ChoiceNew {
		// Never finish composition as a side effect of Enter or Create.
		snap, ok := e.native.Snapshot()
		if !ok || snap.Composing {
			return
		}
		name, err := categoryDisplayName(snap.Text)
		if err != nil || name == "" {
			e.savedUntil = time.Time{}
			e.failure = err
			if e.failure == nil {
				e.failure = errors.New("请输入分组名称。")
			}
			w.invalidate()
			return
		}
		submitted = snap.Text
		choice.Name, choice.ColorHex = name, categoryNewColor
	}
	id, err := core.NewIdentity()
	if err != nil {
		e.failure = err
		w.invalidate()
		return
	}
	generation := e.generation
	e.pending, e.failure, e.savedUntil = true, nil, time.Time{}
	w.commands++
	settled := false
	settle := func() bool {
		if settled {
			return false
		}
		settled = true
		w.commands--
		e.pending = false
		return true
	}
	notAdmitted := func() {
		if !settle() {
			return
		}
		if h.windows[s.titleWindowID] == w && w.category == e && e.generation == generation {
			e.failure = coordinator.ErrNotAdmitted
			w.closing = false
			h.quitting = false
			w.invalidate()
		}
	}
	if !h.tryPostProducer(func() {
		if !h.trySubmit(func(post func(func()) bool) {
			var out taskapp.CategoryOutcome
			if h.stopped {
				out.Err = taskapp.ErrSession
			} else {
				out = h.category.store.ReplaceCategorySelectionOn(localTitleDate(h.now()), e.chain, choice, ordinary.UUID(id))
			}
			h.stopOnError(out.Err)
			// Read once on the same lane; retry receipts never restore old chip values.
			view, readErr := h.category.store.ReadView()
			post(func() {
				if !settle() {
					return
				}
				if readErr == nil {
					h.category.view = view
					h.publish(view.Graph, view.Today)
				}
				if h.windows[s.titleWindowID] != w || w.category != e || e.generation != generation {
					return
				}
				e.pending = false
				confirmed := out.Err == nil && out.Result == taskapp.ResultConfirmed && out.Receipt != nil
				if !confirmed || readErr != nil {
					e.failure = out.Err
					if e.failure == nil {
						e.failure = readErr
					}
					if e.failure == nil {
						e.failure = errors.New(categoryFailureMessage)
					}
					w.closing = false
					h.quitting = false
				} else {
					e.failure = nil
					if choice.Kind == ordinary.ChoiceNew {
						if snap, ok := e.native.Snapshot(); ok && !snap.Composing && snap.Text == submitted {
							// Retire only this acknowledged, non-composing edit.
							// Replace alone leaves a live native lease until unmount.
							if terminal, finished := e.native.Finish(); finished && !terminal.Composing && terminal.Text == submitted && e.native.Replace("") {
								e.creating = false
								e.generation++
							}
						}
					}
					// A confirmed old operation does not acknowledge a newer
					// visible draft (or an existing choice made beside a draft).
					if !e.creating {
						e.savedUntil = h.now().Add(900 * time.Millisecond)
						savedGeneration := e.generation
						h.categoryClock.After(900*time.Millisecond, func() {
							h.tryPostProducer(func() {
								if h.windows[s.titleWindowID] == w && w.category == e && e.generation == savedGeneration {
									w.invalidate()
								}
							})
						})
					}

				}
				w.invalidate()
				if confirmed && readErr == nil && w.closing && w.commands == 0 {
					w.closing = false
					w.close()
				}
				if confirmed && readErr == nil && h.quitting {
					h.quitting = false
					h.quit()
				}
			})
		}) {
			notAdmitted()
		}
	}) {
		notAdmitted()
	}
}

// Source TaskClassificationEditor.swift:95–149,188–264,434–449.
// Labels/manager are not exposed because those services are outside this slice.
func (s *app) liveCategorySection(c *ui.Context, width float32, chain core.TaskChainID) bool {
	e := s.categoryEditor(chain)
	if e == nil {
		return false
	}
	h := s.titles
	w := h.windows[s.titleWindowID]
	catalog := categoryCatalog(h.category.view)
	selected := categoryCurrent(h.category.view, chain)
	name := "无分组"
	color := sourceVisual.text3
	fill := ui.Transparent
	for _, v := range catalog {
		if v.ID == selected {
			name = v.Name
			color = ink
			fill = sourceVisual.controlFill
			if v.PresentationApproval == "userApproved" {
				color = ui.Hex(v.ColorHex)
				fill = color.Alpha(.09)
			}
		}
	}
	weight := 400
	if selected != "" {
		weight = 600
	}
	measured, _ := dayMeasure(c, "category-chip-width", 0, 11, weight, name)
	chipWidth := measured + 26
	if selected != "" {
		chipWidth = min(float32(92), chipWidth+13.5)
	}
	ui.Box(c).Size(width, 28).Children(func() {
		txt(c, 0, 6, 10.5, "分组", sourceVisual.text3, 600).Width(36).Height(24)
		menu := ui.ButtonBase(c.Key(struct {
			Window     *titleWindow
			Chain      core.TaskChainID
			Generation uint64
			Navigation uint64
		}{w, chain, e.generation, w.navigation})).Absolute().Left(44).Size(chipWidth, 28).Radius(6).Background(fill).Label("任务分组 " + string(chain)).Value(name).Disabled(e.pending || h.category.view.Health.Session != taskapp.SessionWritable)
		menu.Children(func() {
			x := float32(7)
			available := chipWidth - 26
			if selected != "" {
				icon(c, x, 9.25, 9.5, "folder", color)
				x += 13.5
				available -= 13.5
			}
			txt(c, x, 6, 11, name, color, weight).Width(available).SingleLine().Ellipsis("…").Tooltip(name)
			icon(c, chipWidth-15, 10, 8, "chevron-down", sourceVisual.text3)
		})
		menu.Menu(func(m *ui.Menu) {
			if m.Item("不加入分组").Checked(selected == "").Disabled(e.pending).Chosen() && selected != "" {
				s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNone})
			}
			if len(catalog) > 0 {
				m.Separator()
			}
			for _, v := range catalog {
				if m.Item(v.Name).Checked(selected == v.ID).Disabled(e.pending).Chosen() && selected != v.ID {
					s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceExisting, CategoryID: v.ID})
				}
			}
			m.Separator()
			if m.Item("新建分组…").Disabled(e.pending).Chosen() {
				e.creating = true
				e.failure = nil
				e.savedUntil = time.Time{}
			}
		})
		if !e.savedUntil.IsZero() && h.now().Before(e.savedUntil) {
			box(c, width-12, 9, 10, 10, green, 5).Label("分组已保存")
			txt(c, width-10.5, 8, 8, "✓", white, 600)
		}
	})
	if e.creating {
		createWidth, _ := dayMeasure(c, "category-create-width", 0, 11, 500, "创建并加入")
		cancelWidth, _ := dayMeasure(c, "category-cancel-width", 0, 11, 400, "取消")
		createWidth = max(float32(28), createWidth)
		cancelWidth = max(float32(28), cancelWidth)
		createX := width - createWidth
		cancelX := createX - 8 - cancelWidth
		inputWidth := max(float32(32), cancelX-8-60)
		ui.Box(c).Height(8)
		ui.Box(c).Size(width, 28).Children(func() {
			box(c, 44, 10, 8, 8, ui.Hex(categoryNewColor), 4)
			e.native.context = c
			input := ui.TextInputBase(c.Key(e), &e.native.draft).Bind(&e.native.handle).Absolute().Left(60).Size(inputWidth, 28).Padding(0, 8).FontSize(11.5).FontWeight(500).Background(sourceVisual.controlFill).Border(1, line).Radius(6).Label("新分组名称").Placeholder("分组名称").OnSubmit(func() { h.tryPostProducer(func() { s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew}) }) })
			_ = input
			cancel := ui.ButtonBase(c.Key(struct {
				E      *runtimeCategoryEditor
				Action string
			}{e, "cancel"})).Absolute().Left(cancelX).Size(cancelWidth, 28).Label("取消新建分组").Disabled(e.pending)
			cancel.Children(func() { txt(c, 0, 6, 11, "取消", sourceVisual.text3, 400).SingleLine() })
			if cancel.Clicked() && !e.pending {
				h.tryPostProducer(func() {
					if h.windows[s.titleWindowID] != w || w.category != e || e.pending {
						return
					}
					if snap, ok := e.native.Finish(); ok && !snap.Composing && e.native.Replace("") {
						e.creating = false
						e.failure = nil
						e.generation++
						w.invalidate()
					}
				})
			}
			create := ui.ButtonBase(c.Key(struct {
				E      *runtimeCategoryEditor
				Action string
			}{e, "create"})).Absolute().Left(createX).Size(createWidth, 28).Label("创建并加入").Disabled(e.pending)
			create.Children(func() { txt(c, 0, 6, 11, "创建并加入", blue, 500).SingleLine() })
			if create.Clicked() {
				h.tryPostProducer(func() { s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew}) })
			}
		})
	}
	if e.failure != nil {
		ui.Box(c).Height(8)
		ui.Text(c, categoryFailureMessage).FontSize(10.5).TextColor(ui.Hex("#C33E39")).Width(width)
	}
	return true
}

func (s *app) categoryScopeCopy(rest string) string {
	if s.titles != nil && s.titles.category != nil {
		return rest
	}
	return "分类、" + rest
}
