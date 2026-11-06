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

func nativeWindowPreflight(args []string) (*nativeWindowReportFile, error) {
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
	values := map[string]string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" || !strings.HasPrefix(arg, "-") {
			return nil, fmt.Errorf("native window smoke rejects positional arguments and --")
		}
		name, value, equal := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-"), "=")
		switch name {
		case "native-window-smoke", "init-session", "session", "screen":
		default:
			return nil, fmt.Errorf("native window smoke rejects flag %q", name)
		}
		if _, exists := values[name]; exists {
			return nil, fmt.Errorf("duplicate native window smoke flag %q", name)
		}
		if !equal {
			i++
			if i == len(args) || strings.HasPrefix(args[i], "-") {
				return nil, fmt.Errorf("missing value for %s", name)
			}
			value = args[i]
		}
		if value == "" {
			return nil, fmt.Errorf("empty native window smoke flag %s", name)
		}
		values[name] = value
	}
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
	root, err := os.OpenRoot(parent)
	if err != nil {
		return nil, err
	}
	file, err := root.OpenFile(reportBase, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	return &nativeWindowReportFile{file: file, root: root, stage: stage}, nil
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
