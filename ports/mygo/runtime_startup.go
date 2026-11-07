// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
)

const legacyDevelopment = "legacy-development"
const ordinaryDevelopment = "ordinary-development"

type runtimeStartup struct {
	profile, session, initSession, screen string
	values                                map[string]string
}

// Pure argument validation. Never open storage or create a report here. Unlike
// flag.Parse this rejects trailing arguments and duplicate flags on every route.
func parseRuntimeStartup(args []string) (runtimeStartup, error) {
	o := runtimeStartup{profile: legacyDevelopment, screen: "day", values: map[string]string{}}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "-help" || args[0] == "--help") {
		return o, nil
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" || !strings.HasPrefix(arg, "-") {
			return o, errors.New("positional arguments and -- are not supported")
		}
		name, value, equal := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-"), "=")
		boolean := name == "list" || name == "prototype"
		switch name {
		case "runtime-profile", "session", "init-session", "screen", "render", "render-one", "native-window-smoke", "list", "prototype":
		default:
			return o, fmt.Errorf("unknown flag %q", name)
		}
		if _, ok := o.values[name]; ok {
			return o, fmt.Errorf("duplicate flag %q", name)
		}
		if boolean && !equal {
			value = "true"
		} else if !equal {
			i++
			if i == len(args) || strings.HasPrefix(args[i], "-") {
				return o, fmt.Errorf("missing value for %s", name)
			}
			value = args[i]
		}
		if value == "" {
			return o, fmt.Errorf("empty value for %s", name)
		}
		if boolean {
			if _, err := strconv.ParseBool(value); err != nil {
				return o, fmt.Errorf("invalid boolean for %s", name)
			}
		}
		o.values[name] = value
	}
	if p, ok := o.values["runtime-profile"]; ok {
		o.profile = p
	}
	if o.profile != legacyDevelopment && o.profile != ordinaryDevelopment {
		return o, errors.New("runtime-profile must be legacy-development or ordinary-development")
	}
	o.session, o.initSession = o.values["session"], o.values["init-session"]
	if s, ok := o.values["screen"]; ok {
		o.screen = s
	}
	if _, ok := findScreen(o.screen); !ok {
		return o, fmt.Errorf("unknown screen %q", o.screen)
	}
	hasSession := o.session != "" || o.initSession != ""
	if o.session != "" && o.initSession != "" {
		return o, errors.New("choose only one session action")
	}
	if o.values["render"] != "" && o.values["render-one"] != "" {
		return o, errors.New("choose only one render action")
	}
	if hasSession && (o.present("prototype") || o.present("list") || o.present("render")) {
		return o, errors.New("session mode cannot mix with prototype, list or all-screen rendering")
	}
	if o.present("list") && (o.present("prototype") || o.present("render") || o.present("render-one")) {
		return o, errors.New("list cannot mix with rendering or prototype")
	}
	if o.present("runtime-profile") && !hasSession {
		return o, errors.New("runtime-profile requires a session action")
	}
	if o.profile == ordinaryDevelopment && o.present("render-one") {
		return o, errors.New("ordinary-development does not support render-one")
	}
	if o.present("native-window-smoke") {
		for name := range o.values {
			switch name {
			case "native-window-smoke", "init-session", "session", "screen", "runtime-profile":
			default:
				return o, fmt.Errorf("native window smoke rejects flag %q", name)
			}
		}
		if !hasSession || o.screen != "day" {
			return o, errors.New("native window smoke requires one session action and day")
		}
	}
	if !hasSession && !o.present("prototype") && !o.present("list") && !o.present("render") && !o.present("render-one") {
		return o, errors.New("use -init-session or -session for development storage; -prototype for static preview")
	}
	return o, nil
}
func (o runtimeStartup) present(name string) bool { _, ok := o.values[name]; return ok }
func (o runtimeStartup) path() string {
	if o.initSession != "" {
		return o.initSession
	}
	return o.session
}

// Read-only filesystem checks precede all report/session/render writes.
func (o runtimeStartup) checkPaths() error {
	p := o.path()
	if p == "" {
		return nil
	}
	if !filepath.IsAbs(p) || filepath.Clean(p) != p || strings.ContainsRune(p, 0) {
		return errors.New("session requires a clean absolute path")
	}
	if err := nativeWindowCheckParents(filepath.Dir(p)); err != nil {
		return err
	}
	info, err := os.Lstat(p)
	if o.initSession != "" {
		if !errors.Is(err, os.ErrNotExist) {
			return errors.New("init-session requires an absent directory")
		}
	} else if err != nil || !info.IsDir() || nativeWindowLink(info) {
		return errors.New("session requires an existing plain directory")
	}
	return nil
}

func (o runtimeStartup) openService(today core.LocalDate) (*taskapp.Service, error) {
	if o.profile == ordinaryDevelopment {
		if o.initSession != "" {
			return taskapp.CreateOrdinaryDevelopment(o.initSession, today)
		}
		return taskapp.OpenOrdinaryDevelopment(o.session, today)
	}
	if o.initSession != "" {
		return taskapp.Create(o.initSession, today)
	}
	return taskapp.Open(o.session, today)
}
func (o runtimeStartup) startupFailure(stage string, err error) error {
	return fmt.Errorf("%s未完成（profile=%s）；创建或启动日结可能已写入，目录已保留，请检查后再操作，不会自动重试或降级：%w", stage, o.profile, err)
}
func runtimeSourceVersion() string {
	if b, ok := debug.ReadBuildInfo(); ok {
		for _, s := range b.Settings {
			if s.Key == "vcs.revision" {
				return "ordinary-entry-v1 / " + s.Value
			}
		}
	}
	return "ordinary-entry-v1 / source-revision-not-embedded (experimental)"
}
