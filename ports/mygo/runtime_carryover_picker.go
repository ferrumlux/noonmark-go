// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"strings"
	"time"
)

type carryoverPickerKind uint8

const (
	carryoverDefer carryoverPickerKind = iota + 1
	carryoverContinue
)

// Each opening must receive a fresh nonzero Presentation. The coordinator owns
// identity and must recheck it, editor barriers and source eligibility on submit.
type carryoverPickerIdentity struct {
	Kind                           carryoverPickerKind
	Window, Session, SourceContext string
	Chain                          core.TaskChainID
	Trace                          core.DayTraceID
	Presentation                   uint64
}

func (p carryoverPickerIdentity) valid() bool {
	return (p.Kind == carryoverDefer || p.Kind == carryoverContinue) && p.Window != "" && p.Session != "" && p.SourceContext != "" && p.Chain != "" && p.Trace != "" && p.Presentation != 0
}
func (p carryoverPickerIdentity) action() string {
	if p.Kind == carryoverDefer {
		return "延期"
	}
	return "延续"
}
func (p carryoverPickerIdentity) hint() string {
	if p.Kind == carryoverDefer {
		return "请选择今天之后的任意未来日期。"
	}
	return "可选择今天或任意未来日期。"
}
func (p carryoverPickerIdentity) allows(d, today core.LocalDate) bool {
	return d.Valid() && today.Valid() && (d.Compare(today) > 0 || p.Kind == carryoverContinue && d == today)
}
func pickerMonth(d core.LocalDate) core.LocalDate { d.Day = 1; return d }

type carryoverPickerIntent struct {
	Identity  carryoverPickerIdentity
	Date      core.LocalDate
	Cancelled bool
}
type carryoverPicker struct {
	identity          carryoverPickerIdentity
	selected, month   core.LocalDate
	open              bool
	focus, validation string
}

func newCarryoverPicker(id carryoverPickerIdentity, today core.LocalDate) (*carryoverPicker, error) {
	if !id.valid() || !today.Valid() {
		return nil, fmt.Errorf("invalid picker context")
	}
	d := today
	if id.Kind == carryoverDefer {
		var err error
		d, err = today.AddDays(1)
		if err != nil {
			return nil, err
		}
	}
	return &carryoverPicker{identity: id, selected: d, month: pickerMonth(d), open: true, focus: "calendar"}, nil
}
func (p *carryoverPicker) active(id carryoverPickerIdentity) bool {
	return p != nil && p.open && id == p.identity
}
func (p *carryoverPicker) selectDate(id carryoverPickerIdentity, d, today core.LocalDate) {
	if !p.active(id) || !id.allows(d, today) {
		return
	}
	p.selected = d
	p.month = pickerMonth(d)
	p.focus = "calendar"
	p.validation = ""
}
func (p *carryoverPicker) shiftMonth(delta int, today core.LocalDate) (core.LocalDate, bool) {
	t := time.Date(p.month.Year, time.Month(p.month.Month), 1, 12, 0, 0, 0, time.UTC).AddDate(0, delta, 0)
	d := core.LocalDate{Year: t.Year(), Month: int(t.Month()), Day: 1}
	min := today
	if p.identity.Kind == carryoverDefer {
		var e error
		min, e = today.AddDays(1)
		if e != nil {
			return d, false
		}
	}
	return d, d.Valid() && today.Valid() && d.Compare(pickerMonth(min)) >= 0
}
func (p *carryoverPicker) confirm(id carryoverPickerIdentity, today core.LocalDate) *carryoverPickerIntent {
	if !p.active(id) {
		return nil
	}
	if !id.allows(p.selected, today) {
		p.validation = id.hint()
		return nil
	}
	p.open = false
	return &carryoverPickerIntent{Identity: id, Date: p.selected}
}
func (p *carryoverPicker) cancel(id carryoverPickerIdentity) *carryoverPickerIntent {
	if !p.active(id) {
		return nil
	}
	p.open = false
	return &carryoverPickerIntent{Identity: id, Cancelled: true}
}

// Grid always reserves 42 slots. Zero dates are unrepresentable Go boundary
// slots, not a Swift business date restriction.
func (p *carryoverPicker) grid() [42]core.LocalDate {
	var g [42]core.LocalDate
	t := time.Date(p.month.Year, time.Month(p.month.Month), 1, 12, 0, 0, 0, time.UTC)
	lead := (int(t.Weekday()) + 6) % 7
	for i := range g {
		g[i], _ = p.month.AddDays(i - lead)
	}
	return g
}
func pickerWeekday(d core.LocalDate) string {
	return []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}[time.Date(d.Year, time.Month(d.Month), d.Day, 12, 0, 0, 0, time.UTC).Weekday()]
}

// draw is a component boundary only. It neither opens itself from live menus nor
// invokes Service. today is supplied freshly on every frame, including Return.
func (p *carryoverPicker) draw(c *ui.Context, id carryoverPickerIdentity, today core.LocalDate) *carryoverPickerIntent {
	return p.drawPending(c, id, today, false)
}

// drawPending is the binding variant: an admitted asynchronous command has an
// immutable target. Cancel dismisses only its visual purpose, never the write.
func (p *carryoverPicker) drawPending(c *ui.Context, id carryoverPickerIdentity, today core.LocalDate, pending bool) (intent *carryoverPickerIntent) {
	if !p.active(id) {
		return nil
	}
	key := fmt.Sprintf("carryover-%q-%q-%q-%q-%q-%d-%d", id.Window, id.Session, id.SourceContext, id.Chain, id.Trace, id.Kind, id.Presentation)
	c = c.Key(key)
	ui.Overlay(c, func() {
		back := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Center().Modal()
		back.Children(func() {
			panel := ui.Box(c).Role(ui.RoleDialog)
			panel.Children(func() {
				back.Background(ink.Alpha(.18))
				panel.Size(340, 395).Padding(18).Background(white).Radius(12).Shadow(0, 10, 30, 0, ink.Alpha(.2)).Label(id.action() + "日期对话框")
				title, hint := id.action()+"到哪一天？", id.hint()
				if pending {
					title = "正在保存" + id.action() + "…"
					hint = "关闭不会撤销已提交的操作。"
					panel.Description(title + " " + hint)
				}
				ui.Box(c).Size(304, 359).Children(func() {
					txt(c, 0, 0, 16, title, sourceVisual.text1, 600).Role(ui.RoleHeading)
					txt(c, 0, 24, 11, hint, sourceVisual.text3, 400)
					txt(c, 0, 57, 13, fmt.Sprintf("%04d年%d月", p.month.Year, p.month.Month), sourceVisual.text1, 600).FontFeatures("tnum")
					button := func(k string, x, y, w float32, title, label string, enabled bool, accent bool, fn func()) ui.Element {
						b := ui.ButtonBase(c.Key(k)).Absolute().Left(x).Top(y).Size(w, 28).Radius(7).Background(sourceVisual.panel2).Disabled(!enabled).Label(label).FocusBind(&p.focus, k).FocusRing(true)
						color := sourceVisual.text2
						if accent {
							b.Background(sourceVisual.accent)
							color = white
						}
						if !enabled {
							color = color.Alpha(.42)
						}
						if b.Hovered() && enabled && !accent {
							b.Background(sourceVisual.chip)
						}
						b.Children(func() {
							tw, _ := c.MeasureText(0, ui.Span{Text: title, Size: 12, Weight: 400})
							txt(c, (w-tw)/2, 5, 12, title, color, 400)
						})
						if b.Clicked() {
							fn()
						}
						return b
					}
					if id.allows(today, today) {
						button("today", 190, 52, 46, "今天", "日期选择回到今天", !pending, false, func() { p.selectDate(id, today, today) })
					}
					for _, v := range []struct {
						k, title, label string
						x               float32
						delta           int
					}{{"previous", "‹", "日期选择上个月", 242, -1}, {"next", "›", "日期选择下个月", 276, 1}} {
						d, ok := p.shiftMonth(v.delta, today)
						button(v.k, v.x, 52, 28, v.title, v.label, ok && !pending, false, func() { p.month = d; p.focus = "calendar" })
					}
					for i, label := range []string{"一", "二", "三", "四", "五", "六", "日"} {
						txt(c, float32(i)*44+14, 88, 11, label, sourceVisual.text3, 600).LetterSpacing(.6).AccessHidden(true)
					}
					ui.Box(c.Key("calendar")).Absolute().Left(0).Top(114).Size(304, 190).Label("日期选择日历").FocusBind(&p.focus, "calendar").Children(func() {
						for i, d := range p.grid() {
							x, y := float32(i%7)*44, float32(i/7)*32
							if !d.Valid() {
								ui.Box(c.Key(fmt.Sprintf("blank-%d", i))).Absolute().Left(x).Top(y).Size(40, 30)
								continue
							}
							allowed := id.allows(d, today)
							enabled := allowed && !pending
							selected := d == p.selected
							label := "选择日期 " + d.String()
							var states []string
							if d == today {
								states = append(states, "今天")
							}
							if selected {
								states = append(states, "已选择")
							}
							if !enabled {
								states = append(states, "不可选择")
							}
							description := fmt.Sprintf("%04d年%d月%d日，%s", d.Year, d.Month, d.Day, pickerWeekday(d))
							if pending {
								description += "，保存期间不可更改目标日期"
								if selected {
									description += "，已提交的目标日期"
								}
							}
							b := ui.ButtonBase(c.Key(d.String())).Absolute().Left(x).Top(y).Size(40, 30).Radius(6).Label(label).Value(strings.Join(states, ", ")).Selected(selected).Background(ui.Transparent).Description(description).Disabled(!enabled).FocusBind(&p.focus, d.String()).FocusRing(true)
							if pending && (allowed || selected) {
								// Pending freezes the target; it does not invalidate the date.
								// Keep it readable without re-enabling input/accessibility.
								b.DimDisabled(false)
							}
							if b.Hovered() && enabled {
								b.Background(sourceVisual.hover)
							}
							b.Children(func() {
								color := sourceVisual.text1
								weight := 400
								if d.Month != p.month.Month {
									color = sourceVisual.text3
								}
								circle := ui.Box(c).Absolute().Left(8).Top(3).Size(24, 24).Radius(12)
								target := float32(0)
								if selected {
									target = 1
									weight = 600
								}
								// AnimateTo starts at its first target and honours ReduceMotion.
								// AnimateColor intentionally does not honour ReduceMotion in MyGo.
								progress := circle.AnimateTo("selected", target, ui.Animation{Duration: 120 * time.Millisecond, Ease: ui.EaseOut})
								circle.Background(sourceVisual.accent.Alpha(progress))
								if d == today {
									circle.Border(1, sourceVisual.accent.Alpha(.78*(1-progress)))
									color = sourceVisual.accent
								}
								color = color.Mix(white, progress)
								if !enabled && !(pending && (allowed || selected)) {
									color = sourceVisual.line2.Alpha(.72)
								}

								title := fmt.Sprint(d.Day)
								tw, th := c.MeasureText(0, ui.Span{Text: title, Size: 11.5, Weight: weight})
								txt(c, (40-tw)/2, (30-th)/2, 11.5, title, color, weight).FontFeatures("tnum")
							})
							if b.Clicked() {
								p.selectDate(id, d, today)
							}
						}
					})
					ui.Box(c).Absolute().Left(0).Top(318).Size(304, 1).Background(sourceVisual.line)
					summary := fmt.Sprintf("%04d年%d月%d日 · %s", p.selected.Year, p.selected.Month, p.selected.Day, pickerWeekday(p.selected))
					color := sourceVisual.text2
					if p.validation != "" {
						summary = p.validation
						color = sourceVisual.warning
					}
					txt(c, 0, 336, 11.5, summary, color, 500).Width(184).Height(18).SingleLine().FontFeatures("tnum").Label("日期选择摘要")
					cancelTitle, cancelLabel := "取消", "取消日期选择"
					confirmTitle, confirmLabel := id.action(), "确认"+id.action()
					if pending {
						cancelTitle, cancelLabel = "关闭", "关闭日期对话框"
						confirmTitle, confirmLabel = "保存中", "正在保存"+id.action()
					}
					cancel := button("cancel", 192, 331, 52, cancelTitle, cancelLabel, true, false, func() { intent = p.cancel(id) })
					confirm := button("confirm", 252, 331, 52, confirmTitle, confirmLabel, !pending, true, func() { intent = p.confirm(id, today) })
					if pending {
						cancel.Description("仅关闭日期对话框，已提交的操作会继续保存。")
						confirm.Description("目标日期已提交，请等待保存结果。")
					}
				})
				// Scope arrows to the calendar draft, never text editors behind the modal.
				if ui.FocusedValue(c, &p.focus) == "calendar" {
					for _, v := range []struct {
						k ui.Key
						n int
					}{{ui.KeyLeft, -1}, {ui.KeyRight, 1}, {ui.KeyUp, -7}, {ui.KeyDown, 7}} {
						if back.OverlayShortcut(0, v.k) || back.OverlayShortcut(ui.Shift, v.k) {
							if d, e := p.selected.AddDays(v.n); e == nil && !pending {
								p.selectDate(id, d, today)
							}
						}
					}
				}
				if panel.Shortcut(0, ui.KeyEnter) && intent == nil && !pending {
					intent = p.confirm(id, today)
				}
				if back.OverlayShortcut(0, ui.KeyEscape) && intent == nil {
					intent = p.cancel(id)
				}
			})
		})
	})
	return intent
}
