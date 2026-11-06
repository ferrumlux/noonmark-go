// SPDX-License-Identifier: AGPL-3.0-only
package canonical16

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func readRecords(t *testing.T, path string, visit func([]string, int)) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "ucd", path))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(strings.SplitN(scanner.Text(), "#", 2)[0])
		if text == "" || strings.HasPrefix(text, "@") {
			continue
		}
		parts := strings.Split(text, ";")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		visit(parts, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}
func fromHex(t *testing.T, s string) string {
	t.Helper()
	var out []rune
	for _, field := range strings.Fields(s) {
		cp, err := strconv.ParseInt(field, 16, 32)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, rune(cp))
	}
	return string(out)
}
func nfd(s string) string {
	var out []rune
	for _, cp := range s {
		out = decompose(out, cp)
	}
	return string(order(out))
}

func TestFullNormalizationConformance(t *testing.T) {
	rows := 0
	readRecords(t, "NormalizationTest.txt", func(parts []string, line int) {
		if len(parts) < 5 {
			t.Fatalf("line %d: missing columns", line)
		}
		var columns [5]string
		for i := range columns {
			columns[i] = fromHex(t, parts[i])
		}
		for i, input := range columns {
			nfcWant, nfdWant := columns[1], columns[2]
			if i >= 3 {
				nfcWant, nfdWant = columns[3], columns[4]
			}
			actual, err := NFC(input)
			if err != nil || actual != nfcWant {
				t.Fatalf("line %d col %d NFC(%U) = %U, want %U: %v", line, i+1, []rune(input), []rune(actual), []rune(nfcWant), err)
			}
			if actual := nfd(input); actual != nfdWant {
				t.Fatalf("line %d col %d NFD = %U, want %U", line, i+1, []rune(actual), []rune(nfdWant))
			}
		}
		rows++
	})
	if rows != 19965 {
		t.Fatalf("normalization row count changed: %d", rows)
	}
	t.Logf("Unicode16 full NormalizationTest: %d rows, %d NFC and %d NFD checks", rows, rows*5, rows*5)
}

func TestFullGraphemeBreakConformance(t *testing.T) {
	rows := 0
	readRecords(t, "auxiliary/GraphemeBreakTest.txt", func(parts []string, line int) {
		fields := strings.Fields(parts[0])
		var input strings.Builder
		var expected []int
		for _, field := range fields {
			switch field {
			case "÷":
				expected = append(expected, input.Len())
			case "×":
			default:
				input.WriteString(fromHex(t, field))
			}
		}
		actual := []int{0}
		visitClusters(input.String(), func(_, end int) { actual = append(actual, end) })
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("line %d (%U) got %v want %v", line, []rune(input.String()), actual, expected)
		}
		rows++
	})
	if rows != 1093 {
		t.Fatalf("grapheme row count changed: %d", rows)
	}
	t.Logf("Unicode16 full GraphemeBreakTest: %d rows", rows)
}

func TestAllScalarPublishedNFKCCasefold(t *testing.T) {
	mappings := make(map[rune]string)
	readRecords(t, "DerivedNormalizationProps.txt", func(parts []string, line int) {
		if parts[1] != "NFKC_CF" {
			return
		}
		span := strings.Split(parts[0], "..")
		lo, err := strconv.ParseInt(span[0], 16, 32)
		if err != nil {
			t.Fatal(err)
		}
		hi, err := strconv.ParseInt(span[len(span)-1], 16, 32)
		if err != nil {
			t.Fatal(err)
		}
		for cp := rune(lo); cp <= rune(hi); cp++ {
			mappings[cp] = fromHex(t, parts[2])
		}
	})
	count := 0
	for cp := rune(0); cp <= 0x10ffff; cp++ {
		if cp >= 0xd800 && cp <= 0xdfff {
			continue
		}
		expected, found := mappings[cp]
		if !found {
			expected = string(cp)
		}
		actual, err := NFKCCasefold(string(cp))
		if err != nil || actual != expected {
			t.Fatalf("U+%04X got %U want %U: %v", cp, []rune(actual), []rune(expected), err)
		}
		count++
	}
	if count != 1112064 {
		t.Fatal(count)
	}
	t.Logf("Unicode16 published NFKC_CF: all %d valid scalars", count)
}

func TestPinnedSourceDigests(t *testing.T) {
	data, err := os.ReadFile("testdata/ucd/SOURCE.json")
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := sha256.Sum256(data)
	if hex.EncodeToString(manifestDigest[:]) != "b4e2c156009050d413808ae1dee3c6ffa324ef65e4aeb78961d54a8ce68836c9" {
		t.Fatal("SOURCE.json differs from pinned official manifest")
	}
	var source struct {
		UnicodeVersion string `json:"unicode_version"`
		Files          []struct {
			Path   string
			SHA256 string
		} `json:"files"`
		License struct {
			Path   string
			SHA256 string
		} `json:"license"`
	}
	if err := json.Unmarshal(data, &source); err != nil {
		t.Fatal(err)
	}
	if source.UnicodeVersion != UnicodeVersion {
		t.Fatal(source.UnicodeVersion)
	}
	for _, file := range append(source.Files, source.License) {
		data, err := os.ReadFile(filepath.Join("testdata/ucd", file.Path))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != file.SHA256 {
			t.Fatalf("changed pinned source %s", file.Path)
		}
	}
}

// UAX15 also requires unchanged scalars outside the listed decomposition cases.
// Published NFC/NFD Quick_Check=N properties provide that complete scalar gate.
func TestAllScalarNormalizationQuickCheck(t *testing.T) {
	notNFC, notNFD := map[rune]bool{}, map[rune]bool{}
	readRecords(t, "DerivedNormalizationProps.txt", func(parts []string, _ int) {
		if len(parts) < 3 || parts[2] != "N" || (parts[1] != "NFC_QC" && parts[1] != "NFD_QC") {
			return
		}
		bounds := strings.Split(parts[0], "..")
		lo, err := strconv.ParseInt(bounds[0], 16, 32)
		if err != nil {
			t.Fatal(err)
		}
		hi, err := strconv.ParseInt(bounds[len(bounds)-1], 16, 32)
		if err != nil {
			t.Fatal(err)
		}
		target := notNFC
		if parts[1] == "NFD_QC" {
			target = notNFD
		}
		for cp := rune(lo); cp <= rune(hi); cp++ {
			target[cp] = true
		}
	})
	for cp := rune(0); cp <= 0x10ffff; cp++ {
		if cp >= 0xd800 && cp <= 0xdfff {
			continue
		}
		text := string(cp)
		if got, _ := NFC(text); (got != text) != notNFC[cp] {
			t.Fatalf("NFC scalar quick-check U+%04X", cp)
		}
		if got := nfd(text); (got != text) != notNFD[cp] {
			t.Fatalf("NFD scalar quick-check U+%04X", cp)
		}
	}
}

// The regular Go test gate catches generated-table edits even before the
// separate offline Python generation/cmp gate runs. Pin changes are reviewed
// version-contract changes, not a way to absorb changed generated output.
func TestPinnedGeneratedTablesAndGenerator(t *testing.T) {
	table, err := os.ReadFile("tables_generated.go")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(table)
	if hex.EncodeToString(digest[:]) != "39c0bba7e56b499ab71457dfe0c5e3ed81d76211ae2f9009c2d82f689bf912dd" {
		t.Fatal("generated Unicode16 table differs from pinned output")
	}
	generator, err := os.ReadFile("internal/generate/generate.py")
	if err != nil {
		t.Fatal(err)
	}
	digest = sha256.Sum256(generator)
	if !strings.Contains(string(table), "// Generator SHA-256: "+hex.EncodeToString(digest[:])+"\n") {
		t.Fatal("generator changed without regeneration")
	}
}
