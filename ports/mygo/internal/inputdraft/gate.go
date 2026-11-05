// SPDX-License-Identifier: AGPL-3.0-only
// 改编自 quboliu/noonmark，commit 43e6eefaae3a04be89cab3e234d470111afd80ef。
// 原文件：Sources/NoonmarkMacRuntime/IMETextDraftAutosaveGate.swift。

// Package inputdraft 实现不含定时器、文本或持久化副作用的 IME 草稿自动保存状态机。
// 调用方必须串行处理每个 Gate 的事件，并在启动持久化前重新验证 schedule。
package inputdraft

import (
	"errors"

	"golang.org/x/text/unicode/norm"
)

// Schedule 描述一次候选保存。DelayMilliseconds 不参与身份验证。
// 返回 schedule 并不保证此刻允许保存；BeginPersistence 会重新检查状态。
type Schedule struct {
	OwnerID           string
	Generation        uint64
	Revision          uint64
	Sequence          uint64
	DelayMilliseconds uint64
}

// PersistenceRequest 是需要原样交回完成回调的保存身份。
type PersistenceRequest struct {
	OwnerID    string
	Generation uint64
	Revision   uint64
	RequestID  uint64
}

// Gate 持有一个 owner 的纯状态机；可按值复制，但不支持并发访问同一实例。
// 零值和 nil receiver 均 fail-closed，不会产生保存许可。
type Gate struct {
	initialized          bool
	ownerID              string
	generation           uint64
	isComposing          bool
	revision             uint64
	acknowledgedRevision uint64
	scheduleSequence     uint64
	requestSequence      uint64
	inFlight             PersistenceRequest
	hasInFlight          bool
	explicitFlushPending bool
	retryAttempt         uint64
}

// New 创建初始 generation 为 1 的 gate。
func New(ownerID string) (*Gate, error) {
	return NewWithGeneration(ownerID, 1)
}

// NewWithGeneration 拒绝空 owner 或初始 generation 为 0；不 trim owner。
func NewWithGeneration(ownerID string, generation uint64) (*Gate, error) {
	if ownerID == "" {
		return nil, errors.New("inputdraft: owner ID must not be empty")
	}
	if generation == 0 {
		return nil, errors.New("inputdraft: initial generation must be greater than zero")
	}
	return &Gate{initialized: true, ownerID: ownerID, generation: generation}, nil
}

func (g *Gate) valid() bool { return g != nil && g.initialized }

// OwnerID 返回调用方提供的 owner 标识。
func (g *Gate) OwnerID() string {
	if !g.valid() {
		return ""
	}
	return g.ownerID
}

// Generation 返回当前 generation；discard 可按 uint64 wrapping 推进到 0。
func (g *Gate) Generation() uint64 {
	if !g.valid() {
		return 0
	}
	return g.generation
}

// IsComposing 报告 native 输入是否处于 composing。
func (g *Gate) IsComposing() bool { return g.valid() && g.isComposing }

// HasUnflushedChanges 按上游的直接 revision 不等关系报告 dirty 状态。
func (g *Gate) HasUnflushedChanges() bool {
	return g.valid() && g.acknowledgedRevision != g.revision
}

// HasPersistenceInFlight 报告是否已有未完成的保存。
func (g *Gate) HasPersistenceInFlight() bool { return g.valid() && g.hasInFlight }

// DraftDidChange 记录文本有变化，保留现有 composing 状态。
func (g *Gate) DraftDidChange() *Schedule {
	if !g.valid() {
		return nil
	}
	return g.NativeSnapshotDidChange(true, g.isComposing)
}

// CompositionDidChange 只改变 composing 状态，不推进 revision。
func (g *Gate) CompositionDidChange(isActive bool) *Schedule {
	return g.NativeSnapshotDidChange(false, isActive)
}

// NativeSnapshotDidChange 以一次原子事件处理文本变化与 composing 快照。
func (g *Gate) NativeSnapshotDidChange(textChanged, nextCompositionState bool) *Schedule {
	if !g.valid() {
		return nil
	}
	compositionChanged := g.isComposing != nextCompositionState
	if !textChanged && !compositionChanged {
		return nil
	}
	if textChanged {
		g.revision++ // 对应 Swift &+=，保留 uint64 wrapping。
		g.retryAttempt = 0
	}
	g.isComposing = nextCompositionState
	g.invalidateSchedule()
	if g.isComposing || !g.HasUnflushedChanges() {
		return nil
	}
	return g.makeSchedule(g.nextDraftDelay())
}

// RequestFlush 记住显式 flush，即使当前 clean、composing 或保存中。
func (g *Gate) RequestFlush() *Schedule {
	if !g.valid() {
		return nil
	}
	g.explicitFlushPending = true
	if g.isComposing || !g.HasUnflushedChanges() || g.hasInFlight {
		return nil
	}
	return g.makeSchedule(0)
}

// PermitsAutosave 拒绝 nil、过期或当前不适用的 schedule。
func (g *Gate) PermitsAutosave(schedule *Schedule) bool {
	return g.valid() && schedule != nil &&
		sameOwner(schedule.OwnerID, g.ownerID) &&
		schedule.Generation == g.generation &&
		schedule.Revision == g.revision &&
		schedule.Sequence == g.scheduleSequence &&
		!g.isComposing && g.HasUnflushedChanges() && !g.hasInFlight
}

// BeginPersistence 验证 schedule，最多发出一个 in-flight request。
func (g *Gate) BeginPersistence(schedule Schedule) *PersistenceRequest {
	if !g.PermitsAutosave(&schedule) {
		return nil
	}
	g.requestSequence++
	request := PersistenceRequest{
		OwnerID: g.ownerID, Generation: g.generation,
		Revision: g.revision, RequestID: g.requestSequence,
	}
	g.inFlight, g.hasInFlight = request, true
	return &request
}

// PersistenceDidSucceed 仅确认精确匹配的保存，不清除其后产生的编辑。
// 仍 dirty 时，即使 composing 也会返回 schedule，保持上游行为。
func (g *Gate) PersistenceDidSucceed(request PersistenceRequest) *Schedule {
	if !g.matchesRequest(request) {
		return nil
	}
	g.clearInFlight()
	if request.Revision > g.acknowledgedRevision {
		g.acknowledgedRevision = request.Revision
	}
	g.retryAttempt = 0
	if !g.HasUnflushedChanges() {
		g.explicitFlushPending = false
		g.invalidateSchedule()
		return nil
	}
	return g.makeSchedule(g.nextDraftDelay())
}

// PersistenceDidFail 保持 dirty，并返回 1/2/4/8/10 秒重试 schedule。
// 与 Swift 一致：retry counter wrap 到 0 后进入减法分支会 fail-closed panic。
func (g *Gate) PersistenceDidFail(request PersistenceRequest) *Schedule {
	if !g.matchesRequest(request) {
		return nil
	}
	g.clearInFlight()
	g.retryAttempt++
	if g.isComposing || !g.HasUnflushedChanges() {
		return nil
	}
	if g.retryAttempt == 0 {
		panic("inputdraft: retry attempt underflow (Swift UInt64 subtraction trap)")
	}
	exponent := g.retryAttempt - 1
	if exponent > 4 {
		exponent = 4
	}
	delay := uint64(1000) << exponent
	if delay > 10000 {
		delay = 10000
	}
	return g.makeSchedule(delay)
}

// AcknowledgeWithoutPersistence 确认当前 revision，但保留 composing 状态。
func (g *Gate) AcknowledgeWithoutPersistence() {
	if !g.valid() {
		return
	}
	g.acknowledgedRevision = g.revision
	g.explicitFlushPending = false
	g.retryAttempt = 0
	g.clearInFlight()
	g.invalidateSchedule()
}

// DiscardLocalChanges 推进 generation 并清除草稿状态；不重置 sequence。
func (g *Gate) DiscardLocalChanges() {
	if !g.valid() {
		return
	}
	g.generation++
	g.revision = 0
	g.acknowledgedRevision = 0
	g.explicitFlushPending = false
	g.retryAttempt = 0
	g.isComposing = false
	g.clearInFlight()
	g.invalidateSchedule()
}

func (g *Gate) matchesRequest(request PersistenceRequest) bool {
	return g.valid() && g.hasInFlight && sameRequest(request, g.inFlight) &&
		sameOwner(request.OwnerID, g.ownerID) && request.Generation == g.generation
}

func (g *Gate) clearInFlight() {
	g.inFlight = PersistenceRequest{}
	g.hasInFlight = false
}

func (g *Gate) nextDraftDelay() uint64 {
	if g.explicitFlushPending {
		return 0
	}
	return 700
}

func (g *Gate) makeSchedule(delay uint64) *Schedule {
	g.scheduleSequence++
	return &Schedule{
		OwnerID: g.ownerID, Generation: g.generation, Revision: g.revision,
		Sequence: g.scheduleSequence, DelayMilliseconds: delay,
	}
}

func (g *Gate) invalidateSchedule() { g.scheduleSequence++ }

// 仅比较时使用 NFC，保留 Swift String canonical equality，不改原 owner 字节。
func sameOwner(a, b string) bool {
	return a == b || norm.NFC.String(a) == norm.NFC.String(b)
}

func sameRequest(a, b PersistenceRequest) bool {
	return sameOwner(a.OwnerID, b.OwnerID) && a.Generation == b.Generation &&
		a.Revision == b.Revision && a.RequestID == b.RequestID
}
