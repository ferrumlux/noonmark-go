//go:build windows

// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import "os"

// File contents are flushed and read back. Directory durability on Windows
// requires a separately validated native backend before production use.
func syncDirectory(_ *os.Root, _ string) error { return nil }
