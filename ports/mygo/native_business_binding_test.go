package main

import (
	"testing"
)

// Pure binding/publication tests. No native window, pixels, or input claim.
func TestNativeBusinessActualHubReadViewAcknowledgement(t *testing.T) {
	for _, kind := range []string{"success", "wrong-app", "wrong-service-view", "runtime-error", "publish-rejected"} {
		t.Run(kind, func(t *testing.T) {
			b, l := nativeBusinessTestProbe(t)
			h := newTestTitleHub(b.service, l, func(f func()) { f() }, wallTitleClock{}, b.now, func() {})
			if err := h.enableCategoryCandidate(); err != nil {
				t.Fatal(err)
			}
			s := newRuntimeApp(h.readView())
			invalidated := 0
			h.attach(s, func() { invalidated++ }, func() {})
			b.bind(h, s, 12)
			if err := b.service.AddToday(nativeBusinessTaskTitle); err != nil {
				t.Fatal(err)
			}
			v, err := b.service.ReadView()
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "wrong-app":
				h.windows[s.titleWindowID].app = newApp()
			case "wrong-service-view":
				s.runtime = b.service
			case "runtime-error":
				s.runtimeError = "unacknowledged failure"
			case "publish-rejected":
				v.Graph.Traces[0].ChainID = "bad-chain"
			}
			err = b.publish(v)
			if kind == "success" {
				if err != nil || !b.report.WindowBindingConfirmed || invalidated == 0 {
					t.Fatalf("failed actual memory publish: %v", err)
				}
			} else if err == nil || b.report.WindowBindingConfirmed {
				t.Fatal("false acknowledgement")
			}
		})
	}
}
func TestNativeBusinessReceiptBindingFields(t *testing.T) {
	_, v, _ := nativeBusinessFixture(t)
	for _, r := range v.Classification.CommittedReceiptsByInteractionID {
		live := r
		live.Notices = nil
		if !nativeBusinessReceiptMatches(r, live) {
			t.Fatal("actual empty notices shape")
		}
		live.Revision++
		if nativeBusinessReceiptMatches(r, live) {
			t.Fatal("wrong receipt revision")
		}
	}
}
func TestNativeBusinessStartupSamplesRequired(t *testing.T) {
	b, _ := nativeBusinessTestProbe(t)
	if err := b.sample("startup"); err == nil {
		t.Fatal("duplicate startup")
	}
	empty := newNativeBusinessProbe("create", b.now)
	if err := empty.sample("after_open"); err == nil {
		t.Fatal("missing startup")
	}
}
