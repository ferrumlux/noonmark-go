// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func dayDetailGateEvidenceScope(data string) error {
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		columns := strings.Split(line, "\t")
		if len(columns) != 5 {
			return fmt.Errorf("invalid gate row")
		}
		if columns[0] != "GO-UI-DETAIL-001" && strings.Contains(columns[4], "ff4901") {
			return fmt.Errorf("day detail evidence assigned to another case: %s", columns[0])
		}
	}
	return nil
}

func TestDayDetailGateEvidenceScope(t *testing.T) {
	data, err := os.ReadFile("docs/engineering/failure-cases/gates.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if err := dayDetailGateEvidenceScope(string(data)); err != nil {
		t.Fatal(err)
	}
}

func TestDayDetailGateEvidenceScopeRejectsBroadStatusReplacement(t *testing.T) {
	for _, id := range []string{"GO-TITLE-001", "GO-SERVICE-HEALTH-001", "GO-HISTORY-REMAINING-001"} {
		if err := dayDetailGateEvidenceScope(id + "\tfast\tsymptom\tscripts/check\t处理中；ff4901ac专项通过"); err == nil {
			t.Fatal("unrelated status accepted", id)
		}
	}
	if err := dayDetailGateEvidenceScope("GO-UI-DETAIL-001\tfast\tsymptom\tscripts/check\t已修复；ff4901ac本地验证通过"); err != nil {
		t.Fatal(err)
	}
}
