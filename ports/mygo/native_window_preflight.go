package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
)

// The supported driver owns a private external parent. This preflight prevents
// accidental reuse and links; it is not a sandbox against a hostile same-user process.
type nativeWindowReportFile struct {
	file  *os.File
	root  *os.Root
	stage string
}

type nativeWindowReportPlan struct{ parent, base, stage string }

// Compatibility wrapper for the original smoke driver; main uses the split
// read-only plan and exclusive creation phases below.
func nativeWindowPreflight(args []string) (*nativeWindowReportFile, error) {
	plan, err := planNativeWindowReport(args)
	if err != nil {
		return nil, err
	}
	return createNativeWindowReport(plan)
}

// Read-only checks: no directory, file or Service can be created here.
func planNativeWindowReport(args []string) (*nativeWindowReportPlan, error) {
	// The standalone legacy helper is a no-op outside smoke mode. main always
	// validates the entire argv before calling it.
	enabled := false
	for _, arg := range args {
		name, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-"), "=")
		if strings.HasPrefix(arg, "-") && name == "native-window-smoke" {
			enabled = true
		}
	}
	if !enabled {
		return nil, nil
	}
	options, err := parseRuntimeStartup(args)
	if err != nil {
		return nil, err
	}
	if !options.present("native-window-smoke") {
		return nil, nil
	}
	if err := options.checkPaths(); err != nil {
		return nil, err
	}
	values := options.values
	if page, exists := values["screen"]; exists && page != "day" {
		return nil, errors.New("native window smoke requires day")
	}
	create, hasCreate := values["init-session"]
	open, hasOpen := values["session"]
	if hasCreate == hasOpen {
		return nil, errors.New("native window smoke requires exactly one session action")
	}
	session, stage, reportBase := create, "create", "stage1.json"
	if hasOpen {
		session, stage, reportBase = open, "open", "stage2.json"
	}
	report := values["native-window-smoke"]
	for _, path := range []string{session, report} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsRune(path, 0) {
			return nil, errors.New("native window smoke requires clean absolute paths")
		}
	}
	parent := filepath.Dir(session)
	if filepath.Base(session) != "session" || filepath.Base(report) != reportBase || filepath.Dir(report) != parent {
		return nil, errors.New("native window smoke requires fixed sibling session and stage report paths")
	}
	if err := nativeWindowCheckParents(parent); err != nil {
		return nil, err
	}
	info, err := os.Lstat(session)
	if hasCreate {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("native window smoke create requires an absent session")
		}
	} else {
		if err != nil {
			return nil, fmt.Errorf("native window smoke session: %w", err)
		}
		if !info.IsDir() || nativeWindowLink(info) {
			return nil, errors.New("native window smoke open requires a plain session directory")
		}
	}
	if _, err := os.Lstat(report); !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("native window smoke report already exists or cannot be inspected")
	}
	return &nativeWindowReportPlan{parent: parent, base: reportBase, stage: stage}, nil
}

func createNativeWindowReport(plan *nativeWindowReportPlan) (*nativeWindowReportFile, error) {
	if plan == nil {
		return nil, nil
	}
	// Recheck plain parents immediately before opening; exclusivity remains the
	// authority if a report appears after the read-only checks.
	if err := nativeWindowCheckParents(plan.parent); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(plan.parent)
	if err != nil {
		return nil, err
	}
	file, err := root.OpenFile(plan.base, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	return &nativeWindowReportFile{file: file, root: root, stage: plan.stage}, nil
}

func nativeWindowCheckParents(path string) error {
	for {
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("native window smoke parent: %w", err)
		}
		if !info.IsDir() || nativeWindowLink(info) {
			return errors.New("native window smoke requires plain directory parents")
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}

func nativeWindowLink(info os.FileInfo) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	if runtime.GOOS != "windows" {
		return false
	}
	// os.FileInfo.Sys on Windows exposes Win32FileAttributeData. Inspect the
	// attribute instead of only ModeSymlink: non-symlink reparse points also fail.
	data := reflect.ValueOf(info.Sys())
	if data.Kind() != reflect.Pointer || data.IsNil() {
		return true
	}
	data = data.Elem()
	if data.Kind() != reflect.Struct {
		return true
	}
	attr := data.FieldByName("FileAttributes")
	return !attr.IsValid() || attr.Kind() != reflect.Uint32 || attr.Uint()&0x400 != 0
}

type nativeWindowReportSink interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

func nativeWindowWriteReport(file nativeWindowReportSink, payload []byte) error {
	var err error
	if len(payload) == 0 || len(payload) > 64*1024 {
		err = errors.New("native window smoke report size invalid")
	} else {
		var n int
		n, err = file.Write(payload)
		if err == nil && n != len(payload) {
			err = io.ErrShortWrite
		}
		if err == nil {
			err = file.Sync()
		}
	}
	return errors.Join(err, file.Close())
}

func (r *nativeWindowReportFile) WriteFinal(payload []byte) error {
	if r.file == nil {
		return errors.New("native window smoke report already finalized")
	}
	file := r.file
	r.file = nil
	err := nativeWindowWriteReport(file, payload)
	return errors.Join(err, r.root.Close())
}
