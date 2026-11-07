// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Builds the positive control through real task and category commands and a
// complete classification replay. No product admission callback is replaced.
func ordinaryScopeFixture(t *testing.T) (canonical.Value, canonical.Value) {
	t.Helper()
	id := func(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012X", n) }
	kernel, err := core.NewTaskKernel(core.TaskGraph{})
	must(t, err)
	now, err := core.InstantFromSeconds(1)
	must(t, err)
	must(t, kernel.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(id(1)), DefinitionID: core.TaskDefinitionID(id(2)), Title: "合成普通分组任务", Now: now}))
	ctx, err := ordinary.NewEmptyContext(kernel.Snapshot())
	must(t, err)
	empty, err := v7.ExportOrdinaryCategoryCandidate(ctx, ordinary.EmptyState())
	must(t, err)
	request := ordinary.Request{ChainID: core.TaskChainID(id(1)), Choice: ordinary.Choice{Kind: ordinary.ChoiceNew, Name: "工程", ColorHex: "#123456"}, InteractionID: ordinary.UUID(id(3))}
	ids := ordinary.PrepareIDs{PlanID: ordinary.UUID(id(4)), CategoryID: ordinary.CategoryID(id(5))}
	plan, err := ordinary.Prepare(ctx, ordinary.EmptyState(), request, ids)
	must(t, err)
	data, err := plan.Data()
	must(t, err)
	confirmation, err := ordinary.ConfirmUser(plan, request.InteractionID)
	must(t, err)
	commit := ordinary.CommitIDs{RecordID: ordinary.UUID(id(6)), NameVersionID: ordinary.UUID(id(7))}
	state, receipt, err := ordinary.Apply(ctx, ordinary.EmptyState(), plan, confirmation, now, commit)
	must(t, err)
	proof, err := ordinary.Replay(ctx, []ordinary.CommittedStep{{Request: request, PrepareIDs: ids, Plan: data, DecisionID: request.InteractionID, Now: now, CommitIDs: commit, Receipt: receipt}})
	must(t, err)
	replayed, err := proof.State()
	must(t, err)
	if !reflect.DeepEqual(state, replayed) {
		t.Fatal("scope fixture lacks causal replay")
	}
	nonempty, err := v7.ExportOrdinaryCategoryCandidate(ctx, state)
	must(t, err)
	imported, err := v7.ImportOrdinaryCategoryCandidate(nonempty, canonical.Limits{})
	must(t, err)
	if len(imported.State.Categories) != 1 || imported.State.Revision != 1 {
		t.Fatal("not a valid nonempty control")
	}
	return empty, nonempty
}
func ordinaryScopeEnvelope(t *testing.T, s canonical.Value) []byte {
	t.Helper()
	b, err := canonical.Marshal(historyObject(map[string]canonical.Value{"formatVersion": historyNumber(7), "snapshot": s}))
	must(t, err)
	return b
}

func TestOrdinaryCategoryCandidateDoesNotExpandProductAdmission(t *testing.T) {
	empty, nonempty := ordinaryScopeFixture(t)
	// Freeze the duplicated offline neutral profile against the actual product.
	neutral, err := emptySupplement()
	must(t, err)
	if !historyEqual(empty.Object["preferences"], neutral.Preferences) || !historyEqual(empty.Object["ideas"], neutral.Ideas) {
		t.Fatal("offline neutral profile drift")
	}
	emptyBytes, err := encodeSnapshot(empty)
	must(t, err)
	if _, err = decodeSnapshot(emptyBytes); err != nil {
		t.Fatal("empty production control", err)
	}
	if _, err = inspectForwardClockOrdinaryHistoryVNext(empty); err != nil {
		t.Fatal("empty vnext control", err)
	}
	before := ordinaryScopeEnvelope(t, nonempty)
	inspection, err := v7.New(v7.Options{}).Inspect(before)
	must(t, err)
	if !inspection.RequiresClassificationAudit {
		t.Fatal("valid nonempty control did not require audit")
	}
	// The direct validator's error identifies the actual unchanged scope fence.
	if err = ValidateEmptyHistory(nonempty); err == nil || !strings.Contains(err.Error(), "unsupported ") {
		t.Fatal("not an empty-catalog scope rejection", err)
	}
	if _, err = encodeSnapshot(nonempty); err == nil {
		t.Fatal("production encoder admitted nonempty catalog")
	}
	if _, err = decodeSnapshot(before); err == nil {
		t.Fatal("production decoder admitted nonempty catalog")
	}
	if _, err = v7.New(v7.Options{}).Decode(before); !errors.Is(err, v7.ErrDomainValidationRequired) {
		t.Fatal("default codec gate changed", err)
	}
	if _, err = inspectForwardClockOrdinaryHistoryVNext(nonempty); err == nil {
		t.Fatal("old vnext expanded")
	} else {
		var scoped *historyVNextError
		if !errors.As(err, &scoped) || scoped.Reason != historyScope {
			t.Fatal("vnext did not reach scope boundary", err)
		}
	}
	if !bytes.Equal(before, ordinaryScopeEnvelope(t, nonempty)) {
		t.Fatal("rejection mutated source")
	}
}

// This writer is test-only. It constructs a complete synthetic v2 container from
// raw canonical payload bytes because the normal writer MUST reject the fixture.
// It does not add an alternate write path to the product.
func ordinaryScopeStore(t *testing.T, payload []byte) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "session")
	must(t, os.Mkdir(root, 0700))
	must(t, os.WriteFile(filepath.Join(root, markerName), []byte(marker), 0600))
	name := filepath.Join(root, "rev-0000000001")
	must(t, os.Mkdir(name, 0700))
	encoded, err := json.Marshal(record{Format: "noonmark-mygo-development-v2", Revision: 1, ParentSHA256: "", Payload: payload})
	must(t, err)
	must(t, os.WriteFile(filepath.Join(name, "state.json"), encoded, 0600))
	sum := sha256.Sum256(encoded)
	hash := hex.EncodeToString(sum[:])
	must(t, os.WriteFile(filepath.Join(name, "COMMITTED"), []byte(hash+"\n"), 0600))
	// Verify the exact outer container independently before Open reaches payload.
	diskRoot, err := os.OpenRoot(root)
	must(t, err)
	defer diskRoot.Close()
	disk := diskStore{root: diskRoot, path: root, revision: 1, hash: hash}
	if _, err = disk.verifyPrefix(1, hash, false); err != nil {
		t.Fatal("scope fixture has invalid identity/hash/COMMITTED", err)
	}
	actual, err := os.ReadFile(filepath.Join(root, markerName))
	must(t, err)
	if string(actual) != marker {
		t.Fatal("wrong session identity")
	}
	return root
}
func TestOrdinaryCategoryCandidateOpenReachesPayloadScope(t *testing.T) {
	empty, nonempty := ordinaryScopeFixture(t)
	// Same valid graph and exact container construction, only classifications vary.
	neutralPayload, err := encodeSnapshot(empty)
	must(t, err)
	neutralPath := ordinaryScopeStore(t, neutralPayload)
	service, err := Open(neutralPath, testDay)
	must(t, err)
	must(t, service.Close())
	payload := ordinaryScopeEnvelope(t, nonempty)
	if _, err = v7.ImportOrdinaryCategoryCandidate(nonempty, canonical.Limits{}); err != nil {
		t.Fatal(err)
	}
	if err = ValidateEmptyHistory(nonempty); err == nil {
		t.Fatal("nonempty scope control was not rejected")
	}
	path := ordinaryScopeStore(t, payload)
	statePath := filepath.Join(path, "rev-0000000001", "state.json")
	before, err := os.ReadFile(statePath)
	must(t, err)
	service, err = Open(path, testDay)
	if service != nil {
		service.Close()
		t.Fatal("Open returned a nonempty-catalog service")
	}
	if !errors.Is(err, ErrCorrupt) {
		t.Fatal("Open did not fail at validated payload boundary", err)
	}
	after, readErr := os.ReadFile(statePath)
	must(t, readErr)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected fixture was rewritten")
	}
	entries, readErr := os.ReadDir(path)
	must(t, readErr)
	if len(entries) != 2 {
		t.Fatal("rejection created a new revision")
	}
}
