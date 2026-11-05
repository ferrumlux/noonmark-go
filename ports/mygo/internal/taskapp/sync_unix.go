//go:build !windows

// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import "os"

func syncDirectory(root *os.Root, path string) error {
	f, e := root.Open(path)
	if e != nil {
		return ErrIncomplete
	}
	defer f.Close()
	if e = f.Sync(); e != nil {
		return ErrIncomplete
	}
	return nil
}
