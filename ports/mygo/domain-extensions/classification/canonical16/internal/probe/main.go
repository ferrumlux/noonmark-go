// SPDX-License-Identifier: AGPL-3.0-only
// Synthetic-data differential probe, not an application entry point.
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	c "github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
	"os"
)

func main() {
	all := flag.Bool("all-scalars", false, "emit cp/byte-count/UTF-8 NFKC_CF binary records")
	flag.Parse()
	if err := run(*all); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(all bool) error {
	output := bufio.NewWriter(os.Stdout)
	defer output.Flush()
	if all {
		for cp := rune(0); cp <= 0x10ffff; cp++ {
			if cp >= 0xd800 && cp <= 0xdfff {
				continue
			}
			folded, err := c.NFKCCasefold(string(cp))
			if err != nil {
				return err
			}
			var header [8]byte
			binary.LittleEndian.PutUint32(header[:4], uint32(cp))
			binary.LittleEndian.PutUint32(header[4:], uint32(len(folded)))
			if _, err = output.Write(header[:]); err != nil {
				return err
			}
			if _, err = output.WriteString(folded); err != nil {
				return err
			}
		}
		return output.Flush()
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 65536), 16*1024*1024)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		var request struct {
			Input string `json:"input"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return err
		}
		nfc, err := c.NFC(request.Input)
		if err != nil {
			return err
		}
		fold, err := c.NFKCCasefold(request.Input)
		if err != nil {
			return err
		}
		display, err := c.DisplayName(request.Input)
		if err != nil {
			return err
		}
		publicKey, err := c.CanonicalKey(request.Input)
		if err != nil {
			return err
		}
		d, k, e := (c.Canonicalizer{}).Canonicalize(request.Input)
		errorText := ""
		if e != nil {
			errorText = e.Error()
		}
		response := struct {
			NFC         string `json:"nfc"`
			Fold        string `json:"fold"`
			Display     string `json:"display"`
			PublicKey   string `json:"public_key"`
			NameDisplay string `json:"name_display"`
			Key         string `json:"key"`
			Error       string `json:"error"`
		}{nfc, fold, display, publicKey, d, k, errorText}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return output.Flush()
}
