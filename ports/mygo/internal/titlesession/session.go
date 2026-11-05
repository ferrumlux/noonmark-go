// SPDX-License-Identifier: AGPL-3.0-only
// 文本生命周期改编自 quboliu/noonmark 43e6eef 的
// App/NoonmarkMacApp/AutosavingMarkdownEditor.swift。

// Package titlesession 提供串行调用、无 IO 的标题草稿 session。
// 输入必须来自真实 native committed-text/composing 快照，不能由应用 Draft 冒充。
// Session 必须通过指针使用，不可复制，且不能被并发调用。
package titlesession

import (
	"errors"
	"strings"

	"github.com/ferrumlux/noonmark/ports/mygo/internal/inputdraft"

	"golang.org/x/text/unicode/norm"
)

var (
	ErrInvalidConfig        = errors.New("titlesession: owner, trace, session and generation are required")
	ErrStaleEvent           = errors.New("titlesession: stale or inactive session event")
	ErrNotReady             = errors.New("titlesession: job is not currently permitted")
	ErrInvalidResult        = errors.New("titlesession: failed result must not contain acknowledged text")
	ErrVerificationMismatch = errors.New("titlesession: verified text does not match the requested title")
	ErrUnsafeDispose        = errors.New("titlesession: session snapshot is not flushed for lifecycle disposal")
)

// Config 的 PersistedText 必须是已核实的初始标题，可以为空；不得推测字节。
// SessionID 与 Generation 由调用方分配，不读取时钟，也不生成随机身份。
type Config struct {
	OwnerID       string
	SessionID     uint64
	Generation    uint64
	TraceID       string
	PersistedText string
}

// NativeSnapshot 只描述原生 committed text 与 composition 状态。
// Text 不包含尚未提交的 marked text。此类型不能自行证明其数据来源。
type NativeSnapshot struct {
	Text      string
	Composing bool
}

// OptionalText 使用值语义，避免暴露内部 string 指针。
type OptionalText struct {
	Text    string
	Present bool
}

// PersistenceResult 的成功可暂不携带 VerifiedText；此时必须随后用同一
// SaveRequest 提交已核实的实际文本。在此之前不会确认 Gate 或返回 FlushClean。
// Err 非 nil 时不得同时声明已确认文本。
type PersistenceResult struct {
	Err          error
	VerifiedText OptionalText
}

// instanceToken 非零大小，确保两个仍存活的独立实例具有不同地址。
type instanceToken struct{ marker byte }

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// Identity 是不可修改的事件身份；discard 后必须重新读取 Identity()。
// 在事件来源处捕获身份，不能把旧事件重新标记成当前新身份。
type Identity struct {
	instance   *instanceToken
	owner      string
	session    uint64
	generation uint64
}

func (i Identity) OwnerID() string    { return i.owner }
func (i Identity) SessionID() uint64  { return i.session }
func (i Identity) Generation() uint64 { return i.generation }

// Job 是不可修改的候选，不代表此刻获准保存。
// 相同 Job 再次返回不能重启调用方的 deadline；0 ms 必须显式执行。
type Job struct {
	identity Identity
	gate     inputdraft.Schedule
}

func (j Job) Identity() Identity                { return j.identity }
func (j Job) GateSchedule() inputdraft.Schedule { return j.gate }
func (j Job) DelayMilliseconds() uint64         { return j.gate.DelayMilliseconds }

// SaveRequest 是传给异步保存调用的不可修改值，不能事后读取 UI selection。
// 完成时将原值交还 PersistenceDidComplete，不要自行重建 gate token。
type SaveRequest struct {
	identity   Identity
	gate       inputdraft.PersistenceRequest
	trace      string
	raw        string
	normalized string
}

func (r SaveRequest) Identity() Identity                         { return r.identity }
func (r SaveRequest) GateRequest() inputdraft.PersistenceRequest { return r.gate }
func (r SaveRequest) TraceID() string                            { return r.trace }
func (r SaveRequest) RawDraft() string                           { return r.raw }
func (r SaveRequest) NormalizedText() string                     { return r.normalized }

// LifecycleStatus 仅描述调用方已提供的快照，不证明 native 最新状态。
type LifecycleStatus uint8

const (
	LifecycleInvalid LifecycleStatus = iota
	LifecycleNotRequested
	LifecycleNeedsNativeFinalization
	LifecycleWaitingPersistence
	LifecycleWaitingChanges
	LifecycleFailed
	LifecycleFlushClean
	LifecycleDisposed
)

type LifecycleResult struct {
	Status LifecycleStatus
	Err    error
}

func (r LifecycleResult) SnapshotIsFlushed() bool {
	return r.Status == LifecycleFlushClean && r.Err == nil
}

// State 是只读值快照；修改它不会改变 Session。
type State struct {
	Identity                  Identity
	TraceID                   string
	Draft                     string
	LastNativeText            string
	LastAcknowledgedText      string
	PendingPersistedText      OptionalText
	CanonicalizesAfterSuccess bool
	Composing                 bool
	Dirty                     bool
	InFlight                  bool
	AwaitingVerification      bool
	Disposed                  bool
	LastError                 error
	Lifecycle                 LifecycleResult
}

// Update 总是带当前状态和当前候选作业。Job=nil 表示没有要保留的候选。
// DraftReplacement 是模型/控件应应用的文字，不是已捕获的 native snapshot。
// Accepted=false 时本事件没有修改状态；Err 解释拒绝或核实失败。
type Update struct {
	Accepted         bool
	Job              *Job
	DraftReplacement OptionalText
	State            State
	Err              error
}

// SnapshotIsFlushed 同时要求本次事件被接受且没有错误。拒绝的旧生命周期
// 事件可能仍携带先前 clean 的只读 State，不能将那个旧快照当作本次成功。
func (u Update) SnapshotIsFlushed() bool {
	return u.Accepted && u.Err == nil && u.State.Lifecycle.SnapshotIsFlushed()
}

type BeginResult struct {
	Update  Update
	Request *SaveRequest
}

// Session 中 Gate 是唯一 dirty/schedule/in-flight/retry 状态机。
type Session struct {
	noCopy               noCopy
	instance             *instanceToken
	gate                 *inputdraft.Gate
	sessionID            uint64
	traceID              string
	draft                string
	lastNative           string
	lastAcknowledged     string
	pendingPersisted     OptionalText
	canonicalizes        bool
	job                  *Job
	inFlight             *SaveRequest
	awaitingVerification bool
	lastError            error
	lifecycleRequested   bool
	nativeFinalized      bool
	lifecycleFailure     error
	disposed             bool
}

func New(config Config) (*Session, error) {
	if config.OwnerID == "" || config.TraceID == "" || config.SessionID == 0 || config.Generation == 0 {
		return nil, ErrInvalidConfig
	}
	gate, err := inputdraft.NewWithGeneration(config.OwnerID, config.Generation)
	if err != nil {
		return nil, err
	}
	return &Session{
		instance: &instanceToken{marker: 1}, gate: gate,
		sessionID: config.SessionID, traceID: config.TraceID,
		draft: config.PersistedText, lastNative: config.PersistedText,
		lastAcknowledged: config.PersistedText,
	}, nil
}

func (s *Session) initialized() bool { return s != nil && s.instance != nil && s.gate != nil }
func (s *Session) active() bool      { return s.initialized() && !s.disposed }

func (s *Session) Identity() Identity {
	if !s.initialized() {
		return Identity{}
	}
	return Identity{s.instance, s.gate.OwnerID(), s.sessionID, s.gate.Generation()}
}

func (s *Session) State() State {
	if !s.initialized() {
		return State{Lifecycle: LifecycleResult{Status: LifecycleInvalid}}
	}
	return State{
		Identity: s.Identity(), TraceID: s.traceID, Draft: s.draft,
		LastNativeText: s.lastNative, LastAcknowledgedText: s.lastAcknowledged,
		PendingPersistedText: s.pendingPersisted, CanonicalizesAfterSuccess: s.canonicalizes,
		Composing: s.gate.IsComposing(), Dirty: s.gate.HasUnflushedChanges(),
		InFlight: s.gate.HasPersistenceInFlight(), AwaitingVerification: s.awaitingVerification,
		Disposed: s.disposed, LastError: s.lastError, Lifecycle: s.lifecycleResult(),
	}
}

func (s *Session) ScheduledJob() *Job {
	if !s.active() || s.job == nil {
		return nil
	}
	value := *s.job
	return &value
}

func (s *Session) validIdentity(id Identity) bool {
	return s.active() && id.instance == s.instance && id.session == s.sessionID &&
		id.generation == s.gate.Generation() && sameText(id.owner, s.gate.OwnerID())
}

func (s *Session) update(accepted bool, err error, replacement OptionalText) Update {
	return Update{Accepted: accepted, Job: s.ScheduledJob(), DraftReplacement: replacement, State: s.State(), Err: err}
}

func (s *Session) replaceJob(schedule *inputdraft.Schedule) {
	s.job = nil
	if schedule != nil {
		s.job = &Job{identity: s.Identity(), gate: *schedule}
	}
}

// NativeSnapshotDidChange 允许重复观察；没有语义/合成变化时保留原 deadline。
func (s *Session) NativeSnapshotDidChange(id Identity, snapshot NativeSnapshot) Update {
	if !s.validIdentity(id) {
		return s.update(false, ErrStaleEvent, OptionalText{})
	}
	s.observe(snapshot)
	return s.update(true, nil, OptionalText{})
}

func (s *Session) observe(snapshot NativeSnapshot) {
	textChanged := !sameText(s.lastNative, snapshot.Text)
	compositionChanged := s.gate.IsComposing() != snapshot.Composing
	s.draft, s.lastNative = snapshot.Text, snapshot.Text
	if s.pendingPersisted.Present && !sameText(s.pendingPersisted.Text, snapshot.Text) {
		s.pendingPersisted = OptionalText{}
	}
	if !textChanged && !compositionChanged {
		return
	}
	// 新 native 事件使上一次生命周期最终化证据过期；不可自行补证。
	s.nativeFinalized = false
	s.replaceJob(s.gate.NativeSnapshotDidChange(textChanged, snapshot.Composing))
}

// RequestFlush 对应结束编辑；本方法不宣称 native 已最终化。
func (s *Session) RequestFlush(id Identity) Update {
	if !s.validIdentity(id) {
		return s.update(false, ErrStaleEvent, OptionalText{})
	}
	s.canonicalizes = true
	s.replaceJob(s.gate.RequestFlush())
	return s.update(true, nil, OptionalText{})
}

// RequestLifecycleFlush 必须由调用方送最新真实 native 快照。
// nativeFinalized 只有在原生最终化已成功时才为 true；本方法不执行最终化。
// 输入再次变化或失败后，需要新的显式调用；FlushClean 仅用于调用方已提供的快照。
func (s *Session) RequestLifecycleFlush(id Identity, snapshot NativeSnapshot, nativeFinalized bool) Update {
	if !s.validIdentity(id) {
		return s.update(false, ErrStaleEvent, OptionalText{})
	}
	s.observe(snapshot)
	s.lifecycleRequested = true
	s.lifecycleFailure = nil
	s.nativeFinalized = nativeFinalized && !snapshot.Composing
	s.canonicalizes = true
	s.job = nil
	if s.nativeFinalized {
		s.replaceJob(s.gate.RequestFlush())
	}
	return s.update(true, nil, OptionalText{})
}

// BeginPersistence 再验证当前候选；空白普通输入只消费候选并保留 dirty。
// 即使 normalized 与上次 ack 相同，也返回实际 Service 验证请求。
func (s *Session) BeginPersistence(job Job) BeginResult {
	if !s.validIdentity(job.identity) || s.job == nil || job != *s.job {
		return BeginResult{Update: s.update(false, ErrStaleEvent, OptionalText{})}
	}
	if !s.gate.PermitsAutosave(&job.gate) {
		return BeginResult{Update: s.update(false, ErrNotReady, OptionalText{})}
	}
	normalized, nonempty := NormalizeTitle(s.draft)
	if !nonempty {
		s.job = nil
		if !s.canonicalizes || s.lifecycleRequested && (!s.nativeFinalized || s.lifecycleFailure != nil) {
			// 失败或未完成 native 最终化不能借普通空白 retry 丢掉草稿。
			return BeginResult{Update: s.update(true, nil, OptionalText{})}
		}
		s.draft, s.lastNative = s.lastAcknowledged, s.lastAcknowledged
		s.gate.DiscardLocalChanges()
		s.canonicalizes = false
		s.lastError = nil
		return BeginResult{Update: s.update(true, nil, OptionalText{Text: s.draft, Present: true})}
	}
	gateRequest := s.gate.BeginPersistence(job.gate)
	if gateRequest == nil {
		return BeginResult{Update: s.update(false, ErrNotReady, OptionalText{})}
	}
	request := SaveRequest{s.Identity(), *gateRequest, s.traceID, s.draft, normalized}
	s.inFlight = &request
	s.job = nil
	returned := request
	return BeginResult{Update: s.update(true, nil, OptionalText{}), Request: &returned}
}

// PersistenceDidComplete 只接受本实例当前不可变请求。verified 字节必须来自
// 原子 Service ack 或等强度核实，不能简单把请求 normalized 回填冒充事实。
func (s *Session) PersistenceDidComplete(request SaveRequest, result PersistenceResult) Update {
	if !s.validIdentity(request.identity) || s.inFlight == nil || request != *s.inFlight {
		return s.update(false, ErrStaleEvent, OptionalText{})
	}
	if result.Err != nil && result.VerifiedText.Present {
		return s.update(false, ErrInvalidResult, OptionalText{})
	}
	if result.Err != nil {
		s.inFlight = nil
		s.awaitingVerification = false
		s.lastError = result.Err
		if s.lifecycleRequested {
			s.lifecycleFailure = result.Err
		}
		s.replaceJob(s.gate.PersistenceDidFail(request.gate))
		return s.update(true, result.Err, OptionalText{})
	}
	if !result.VerifiedText.Present {
		s.awaitingVerification = true
		return s.update(true, nil, OptionalText{})
	}
	if !sameText(result.VerifiedText.Text, request.normalized) {
		s.awaitingVerification = true
		s.lastError = ErrVerificationMismatch
		return s.update(true, ErrVerificationMismatch, OptionalText{})
	}
	verified := result.VerifiedText.Text
	s.inFlight = nil
	s.awaitingVerification = false
	s.lastError = nil
	s.pendingPersisted = OptionalText{Text: verified, Present: true}
	s.lastAcknowledged = verified
	s.replaceJob(s.gate.PersistenceDidSucceed(request.gate))
	replacement := OptionalText{}
	if s.canonicalizes && !s.gate.IsComposing() && sameText(s.draft, request.raw) && !sameText(s.draft, verified) {
		s.draft, s.lastNative = verified, verified
		replacement = OptionalText{Text: verified, Present: true}
	}
	// 对应 Swift lifecycle 的递归 flush：新草稿仍需要结束编辑规范化。
	s.canonicalizes = s.lifecycleRequested && s.gate.HasUnflushedChanges()
	return s.update(true, nil, replacement)
}

// SynchronizePersistedText 只接收调用方核实的最新有序发布，不接受过期读回。
// 返回 Accepted=true 但未应用时，代表与 Swift 一样保留本地编辑。
func (s *Session) SynchronizePersistedText(id Identity, text string) Update {
	if !s.validIdentity(id) {
		return s.update(false, ErrStaleEvent, OptionalText{})
	}
	if s.pendingPersisted.Present && sameText(s.pendingPersisted.Text, text) {
		s.pendingPersisted = OptionalText{}
		s.lastAcknowledged = text
		return s.update(true, nil, OptionalText{})
	}
	if s.gate.IsComposing() || s.gate.HasUnflushedChanges() || s.gate.HasPersistenceInFlight() || s.awaitingVerification {
		return s.update(true, nil, OptionalText{})
	}
	s.gate.AcknowledgeWithoutPersistence()
	s.lastAcknowledged = text
	s.job = nil
	replacement := OptionalText{}
	if !sameText(s.draft, text) {
		s.draft, s.lastNative = text, text
		replacement = OptionalText{Text: text, Present: true}
	}
	return s.update(true, nil, replacement)
}

// Dispose 不丢弃未保存草稿；必须已取得所提供快照的 lifecycle FlushClean。
// 调用方还须停止对应 native 输入来源并取得最新快照，本方法不执行这些动作。
func (s *Session) Dispose(id Identity) Update {
	if !s.validIdentity(id) {
		return s.update(false, ErrStaleEvent, OptionalText{})
	}
	if !s.lifecycleResult().SnapshotIsFlushed() {
		return s.update(false, ErrUnsafeDispose, OptionalText{})
	}
	s.disposed = true
	s.job = nil
	return s.update(true, nil, OptionalText{})
}

func (s *Session) lifecycleResult() LifecycleResult {
	if !s.initialized() {
		return LifecycleResult{Status: LifecycleInvalid}
	}
	if s.disposed {
		return LifecycleResult{Status: LifecycleDisposed}
	}
	if !s.lifecycleRequested {
		return LifecycleResult{Status: LifecycleNotRequested}
	}
	if s.lifecycleFailure != nil {
		return LifecycleResult{Status: LifecycleFailed, Err: s.lifecycleFailure}
	}
	if s.lastError != nil {
		return LifecycleResult{Status: LifecycleFailed, Err: s.lastError}
	}
	if !s.nativeFinalized || s.gate.IsComposing() {
		return LifecycleResult{Status: LifecycleNeedsNativeFinalization}
	}
	if s.gate.HasPersistenceInFlight() || s.awaitingVerification {
		return LifecycleResult{Status: LifecycleWaitingPersistence}
	}
	if s.gate.HasUnflushedChanges() || s.job != nil {
		return LifecycleResult{Status: LifecycleWaitingChanges}
	}
	return LifecycleResult{Status: LifecycleFlushClean}
}

// NormalizeTitle 与正式 Go domain 标题命令相同。Foundation whitespace
// 精确对等仍待 Apple oracle；不做 NFC 字节改写，保留内部换行。
func NormalizeTitle(text string) (string, bool) {
	text = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"))
	return text, text != ""
}

func sameText(a, b string) bool { return a == b || norm.NFC.String(a) == norm.NFC.String(b) }
