// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func dayHostedCIEvidenceScope(data string) error {
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		columns := strings.Split(line, "\t")
		if len(columns) != 5 {
			return fmt.Errorf("invalid gate row")
		}
		allowed := columns[0] == "GO-PERF-DAY-001" || columns[0] == "GO-MYGO-LAYOUT-001"
		for _, marker := range []string{"42240698683", "bff7414", "0b33571", "HOSTED-CI-42240698683.json"} {
			if !allowed && strings.Contains(line, marker) {
				return fmt.Errorf("hosted day evidence assigned to another case: %s", columns[0])
			}
		}
		if allowed && (!strings.Contains(columns[4], "42240698683") || !strings.Contains(columns[4], "bff7414")) {
			return fmt.Errorf("missing exact hosted identity: %s", columns[0])
		}
	}
	return nil
}

func TestDayHostedCIEvidenceScope(t *testing.T) {
	data, err := os.ReadFile("docs/engineering/failure-cases/gates.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if err := dayHostedCIEvidenceScope(string(data)); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile("docs/tasks/diagnose-root-test-timeout/HOSTED-CI-42240698683.json")
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Head      string   `json:"head"`
		Tree      string   `json:"tree"`
		Candidate string   `json:"candidate_commit"`
		Run       int64    `json:"run_id"`
		IDs       []string `json:"scope_failure_ids"`
	}
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Head != "bff7414d71c10f31f45c679393e035aa6386b989" || receipt.Tree != "04482b01e72640b808d7d49c65b7acf728fadfc3" || receipt.Candidate != "0b33571a1d3892201d9c457051db25c6a45eaabb" || receipt.Run != 42240698683 || strings.Join(receipt.IDs, ",") != "GO-PERF-DAY-001,GO-MYGO-LAYOUT-001" {
		t.Fatalf("hosted receipt source or case scope changed: %+v", receipt)
	}
}

func TestDayHostedCIEvidenceScopeRejectsLeaks(t *testing.T) {
	data, err := os.ReadFile("docs/engineering/failure-cases/gates.tsv")
	if err != nil {
		t.Fatal(err)
	}
	rejected := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		columns := strings.Split(line, "\t")
		if columns[0] == "GO-PERF-DAY-001" || columns[0] == "GO-MYGO-LAYOUT-001" {
			continue
		}
		for _, marker := range []string{"42240698683", "bff7414", "0b33571", "HOSTED-CI-42240698683.json"} {
			for col := 1; col < 5; col++ {
				mutated := append([]string(nil), columns...)
				mutated[col] += ";" + marker
				if err := dayHostedCIEvidenceScope(strings.Join(mutated, "\t")); err == nil {
					t.Fatalf("accepted leaked evidence for %s column %d", columns[0], col)
				}
				rejected++
			}
		}
	}
	if rejected == 0 {
		t.Fatal("no unrelated rows tested")
	}
	for _, invalid := range []string{"bad row", "GO-PERF-DAY-001\tfast\tsymptom\tscripts/check\tpassed"} {
		if err := dayHostedCIEvidenceScope(invalid); err == nil {
			t.Fatalf("accepted invalid scope: %s", invalid)
		}
	}
}
