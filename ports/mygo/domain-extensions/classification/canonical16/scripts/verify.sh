#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# 离线规范、race、生成器、六目标纯Go编译；ICU差分显式启用，不静默跳过。
set -euo pipefail
package_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
module_root=$(cd "$package_root/../.." && pwd -P)
: "${GO:?Set GO to the Go 1.27.3 executable}"
: "${NOONMARK_MYGO_BUILD_ROOT:?Set an explicit external build root}"
: "${GOPATH:?Set an external GOPATH}"
: "${GOCACHE:?Set an external GOCACHE}"
export GOTOOLCHAIN=local GOWORK=off GOPROXY=off GOSUMDB=off
export PYTHONDONTWRITEBYTECODE=1 GOTELEMETRY=off GOENV=off GOFLAGS=-mod=readonly
# This is the first filesystem-related operation after read-only source lookup.
# The guard performs no mkdir/resolve-through-symlink or Go invocation.
python3 -B "$package_root/scripts/guard_paths.py" --module-root "$module_root"
# Preserve the already-validated environment bytes. In particular, never pipe a
# path through shell command substitution, which strips trailing newlines.
build_base="$NOONMARK_MYGO_BUILD_ROOT"
# This substitution contains only newly generated ASCII hex, never a path.
run_id=$(python3 -B -c 'import uuid; print(uuid.uuid4().hex)')
[[ $run_id =~ ^[0-9a-f]{32}$ ]] || { echo 'Invalid generated run identifier' >&2; exit 2; }
build_root="$build_base/run-$run_id"
# Only now may this entry create directories or write logs/cache files.
mkdir -p -- "$build_base" "$GOPATH" "$GOCACHE"
# Exclusive creation: never reuse or truncate any previous run's output files.
mkdir -m 700 -- "$build_root"
printf 'Canonical16 evidence run: %s\n' "$build_root"
export GOMODCACHE="$GOPATH/pkg/mod"
unset GOCACHEPROG
"$GO" version | tee "$build_root/go-version.txt"
"$GO" version | grep -q '^go version go1\.27\.3 ' || { echo 'Expected pinned Go 1.27.3' >&2; exit 1; }
mkdir -p "$build_root/tmp"
export TMPDIR="$build_root/tmp" GOTMPDIR="$build_root/tmp"
python3 -B "$package_root/scripts/test_guard_paths.py" --scratch "$build_root/tmp" | tee "$build_root/path-guard.log"
python3 "$package_root/internal/generate/generate.py" --check | tee "$build_root/generate-check.log"
python3 "$package_root/internal/generate/generate.py" --output "$build_root/tables-regenerated.go" | tee "$build_root/generate-rerun.log"
cmp "$package_root/tables_generated.go" "$build_root/tables-regenerated.go"
# Prove pinned-input tampering fails before generation, without touching source.
python3 - "$package_root" "$build_root" <<'PY'
import shutil, subprocess, sys, tempfile
from pathlib import Path
root, build = map(Path, sys.argv[1:])
with tempfile.TemporaryDirectory(prefix='ucd-check-', dir=build/'tmp') as temp:
    ucd = Path(temp)/'ucd'
    shutil.copytree(root/'testdata/ucd', ucd)
    for filename, expected_error in [('DerivedCoreProperties.txt', 'source checksum mismatch'), ('SOURCE.json', 'differs from the pinned manifest')]:
        original = (ucd/filename).read_bytes()
        with (ucd/filename).open('ab') as out:
            out.write(b'\n# deliberately corrupted synthetic source copy\n')
        result = subprocess.run([sys.executable, str(root/'internal/generate/generate.py'), '--ucd', str(ucd), '--output', str(Path(temp)/'bad.go')], capture_output=True, text=True)
        if result.returncode == 0 or expected_error not in result.stderr or (Path(temp)/'bad.go').exists():
            raise SystemExit('generator did not fail closed on altered ' + filename)
        (ucd/filename).write_bytes(original)
print('Pinned-source and manifest tamper rejection passed')
PY
cd "$module_root"
"$GO" test -count=1 -race -v ./classification/canonical16/... | tee "$build_root/race.log"
"$GO" test -count=1 ./... | tee "$build_root/module.log"
"$GO" vet ./classification/canonical16/... 2>&1 | tee "$build_root/vet.log"
"$GO" test ./classification/canonical16 -run '^$' -bench BenchmarkReverseNonstarterRun -benchtime=200ms | tee "$build_root/bench.log"
"$GO" build -buildvcs=false -trimpath -o "$build_root/canonical16-probe" ./classification/canonical16/internal/probe
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
    os=${target%/*}; arch=${target#*/}
    suffix=''; if [[ $os = windows ]]; then suffix=.exe; fi
    CGO_ENABLED=0 GOOS=$os GOARCH=$arch "$GO" build -buildvcs=false -trimpath -o "$build_root/canonical16-$os-$arch$suffix" ./classification/canonical16/internal/probe
    printf '%s pure-Go build passed (not target runtime execution)\n' "$target"
done | tee "$build_root/cross-build.log"
if [[ ${RUN_ICU76:-0} = 1 ]]; then
    python3 "$package_root/internal/generate/differential_icu76.py" --probe "$build_root/canonical16-probe" --output "$build_root/icu76" | tee "$build_root/icu76.log"
fi
