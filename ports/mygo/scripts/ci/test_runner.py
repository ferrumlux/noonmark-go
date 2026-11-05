#!/usr/bin/env python3
"""Synthetic runner control-flow tests; never substitutes for real build evidence."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time

HERE = Path(__file__).resolve().parent
STAGES = ('check', 'cross-build', 'test-entrypoints')


def fixture(mode):
    with tempfile.TemporaryDirectory(prefix='noonmark-ci-runner-') as tmp:
        base = Path(tmp)
        repo = base / 'source'
        scripts = repo / 'ports/mygo/scripts'
        (scripts / 'ci').mkdir(parents=True)
        shutil.copyfile(HERE / 'run', scripts / 'ci/run')
        capture = (HERE / 'capture.py').read_text()
        if mode == 'delay':
            capture = capture.replace('Path(sys.argv[1]).write_bytes(remaining)',
                                      'import time; time.sleep(0.2)\nPath(sys.argv[1]).write_bytes(remaining)')
        elif mode == 'writer-failure':
            capture = capture.replace('Path(sys.argv[1]).write_bytes(remaining)',
                                      'raise OSError("synthetic storage write failure")')
        (scripts / 'ci/capture.py').write_text(capture)
        for stage in STAGES:
            body = f'#!/bin/bash\nset -eu\nprintf "%s\\n" {stage}\n'
            if stage == mode:
                body += 'exit 37\n'
            if stage == 'cross-build':
                body += 'mkdir -p "$NOONMARK_MYGO_BUILD_ROOT/bin"; printf synthetic > "$NOONMARK_MYGO_BUILD_ROOT/bin/noonmark-mygo-linux-amd64"\n'
            (scripts / stage).write_text(body)
        binaries = base / 'fake-bin'
        binaries.mkdir()
        for name, body in {
            'go': '#!/bin/bash\ncase "$1" in env) echo go1.27.3;; version) echo synthetic-go;; esac\n',
            'fc-match': '#!/bin/bash\necho "Noto Sans CJK SC"\n',
        }.items():
            (binaries / name).write_text(body)
            (binaries / name).chmod(0o755)
        build = (repo if mode == 'inside-source' else base) / 'build'
        build.mkdir()
        if mode == 'nonempty':
            (build / 'existing').write_text('synthetic')
        env = dict(os.environ, PATH=str(binaries) + ':' + os.environ['PATH'],
                   NOONMARK_GO=str(binaries / 'go'), NOONMARK_MYGO_BUILD_ROOT=str(build))
        start = time.monotonic()
        result = subprocess.run(['bash', str(scripts / 'ci/run')], env=env, capture_output=True)
        elapsed = time.monotonic() - start
        expected = 37 if mode in STAGES else (1 if mode == 'writer-failure' else (2 if mode in ('nonempty', 'inside-source') else 0))
        assert result.returncode == expected, (mode, result.returncode, result.stderr)
        assert (build / 'ci-evidence/result.txt').exists() == (expected == 0), mode
        if mode == 'delay':
            assert elapsed >= 0.6, elapsed
            for name in ('checks', 'cross-build', 'entrypoints'):
                assert (build / f'ci-evidence/{name}.txt').stat().st_size > 0, name


for mode in (*STAGES, 'success', 'delay', 'writer-failure', 'nonempty', 'inside-source'):
    fixture(mode)
print('8 synthetic runner fixtures passed: stage failures, success, synchronous logs, writer failure and root guards.')
