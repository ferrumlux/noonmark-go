#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""实际启动 verify.sh 的负例；所有拒绝必须发生在首次写入和 Go 调用之前。"""
import argparse
import os
from pathlib import Path
import runpy
import subprocess
import stat
import tempfile

SCRIPT_ROOT = Path(__file__).resolve().parent
GUARD = runpy.run_path(str(SCRIPT_ROOT/'guard_paths.py'), run_name='path_guard_test_import')

def snapshot(root):
    entries = []
    for path in root.rglob('*'):
        info = path.lstat()
        if stat.S_ISLNK(info.st_mode):
            value = path.readlink().as_posix()
        elif stat.S_ISREG(info.st_mode):
            value = path.read_bytes()
        else:
            # Never open FIFO/socket/device fixtures while taking snapshots.
            value = b''
        entries.append((str(path.relative_to(root)), info.st_mode, info.st_nlink, value))
    return sorted(entries)

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--scratch', type=Path, required=True)
    args = parser.parse_args()
    module = SCRIPT_ROOT.parents[2]
    roots = GUARD['source_roots'](module)
    aggregate = module.parent
    repositories = [root for root in roots if GUARD['is_repository'](root)]
    if not repositories:
        # A standalone non-Git distribution still gets an explicit repo-layout
        # function-level test below, while the actual entry protects aggregate.
        repositories = [aggregate]
    with tempfile.TemporaryDirectory(prefix='guard-negative-', dir=args.scratch) as temp:
        temporary = Path(temp)
        link = temporary/'link'
        (temporary/'target').mkdir()
        link.symlink_to(temporary/'target', target_is_directory=True)
        for dirname in ['nested-build', 'nested-cache', 'nested-gopath']:
            (temporary/dirname/'inside').mkdir(parents=True)
            (temporary/dirname/'inside/link').symlink_to(temporary/'target', target_is_directory=True)
        sentinel = temporary/'unchanged-synthetic-sentinel.txt'
        sentinel.write_bytes(b'Immutable synthetic canary\n')
        for dirname in ['hardlink-build', 'hardlink-gopath', 'hardlink-gocache']:
            (temporary/dirname).mkdir()
            os.link(sentinel, temporary/dirname/'go-version.txt')
        (temporary/'fifo-build').mkdir()
        os.mkfifo(temporary/'fifo-build/go-version.txt')
        name = 'must-not-create-' + temporary.name
        defaults = {'NOONMARK_MYGO_BUILD_ROOT':str(temporary/'build'),
                    'GOPATH':str(temporary/'gopath'), 'GOCACHE':str(temporary/'gocache')}
        cases = [
            ('module child', {'NOONMARK_MYGO_BUILD_ROOT':str(module/name/'child')}),
            ('app sibling', {'NOONMARK_MYGO_BUILD_ROOT':str(aggregate/name/'app-child')}),
            ('repository child', {'NOONMARK_MYGO_BUILD_ROOT':str(repositories[0]/name/'child')}),
            ('source-containing ancestor', {'NOONMARK_MYGO_BUILD_ROOT':str(aggregate.parent)}),
            ('build symlink', {'NOONMARK_MYGO_BUILD_ROOT':str(link/'child')}),
            ('GOPATH symlink', {'GOPATH':str(link/'child')}),
            ('GOCACHE symlink', {'GOCACHE':str(link/'child')}),
            ('nested build symlink', {'NOONMARK_MYGO_BUILD_ROOT':str(temporary/'nested-build')}),
            ('nested GOCACHE symlink', {'GOCACHE':str(temporary/'nested-cache')}),
            ('nested GOPATH symlink', {'GOPATH':str(temporary/'nested-gopath')}),
            ('GOPATH inside source', {'GOPATH':str(aggregate/name/'gopath')}),
            ('GOCACHE inside repo', {'GOCACHE':str(repositories[0]/name/'gocache')}),
            ('cache overlap', {'GOCACHE':str(temporary/'gopath/child')}),
            ('build/cache overlap', {'GOCACHE':str(temporary/'build/child')}),
            ('cache/build overlap', {'NOONMARK_MYGO_BUILD_ROOT':str(temporary/'gopath/child')}),
            ('trailing-newline path', {'NOONMARK_MYGO_BUILD_ROOT':str(temporary/'newline-target')+'\n'}),
            ('embedded-control path', {'NOONMARK_MYGO_BUILD_ROOT':str(temporary/'tab\tpath')}),
            ('double-leading slash', {'NOONMARK_MYGO_BUILD_ROOT':'/'+str(aggregate/name/'child')}),
            ('duplicate slash', {'NOONMARK_MYGO_BUILD_ROOT':str(temporary)+'//build'}),
            ('dot path component', {'NOONMARK_MYGO_BUILD_ROOT':str(temporary)+'/./build'}),
            ('hardlinked build output', {'NOONMARK_MYGO_BUILD_ROOT':str(temporary/'hardlink-build')}),
            ('hardlinked GOPATH output', {'GOPATH':str(temporary/'hardlink-gopath')}),
            ('hardlinked GOCACHE output', {'GOCACHE':str(temporary/'hardlink-gocache')}),
            ('special output FIFO', {'NOONMARK_MYGO_BUILD_ROOT':str(temporary/'fifo-build')}),
            ('relative build root', {'NOONMARK_MYGO_BUILD_ROOT':'relative-build'}),
        ]
        for title, overrides in cases:
            env = {**os.environ, **defaults, **overrides}
            env.update(GO='/guard-must-reject-before-running-go', PYTHONDONTWRITEBYTECODE='1')
            before = snapshot(temporary)
            forbidden = [module/name, aggregate/name, repositories[0]/name]
            if any(p.exists() or p.is_symlink() for p in forbidden):
                raise AssertionError('negative path unexpectedly exists')
            result = subprocess.run(['bash', str(SCRIPT_ROOT/'verify.sh')], env=env, capture_output=True, text=True)
            if result.returncode != 2 or 'Path guard rejected:' not in result.stderr:
                raise AssertionError((title, result.returncode, result.stdout, result.stderr))
            if snapshot(temporary) != before or any(p.exists() or p.is_symlink() for p in forbidden):
                raise AssertionError(title + ' created or changed files before rejection')
            print(title + ': rejected before writes')
        # Positive allocation regression: a deliberately failing benign shell
        # builtin stops before recursive test execution. Each invocation must
        # still allocate a fresh 0700 run without changing earlier run bytes.
        positive = temporary/'positive-runs'
        env = {**os.environ, **defaults, 'NOONMARK_MYGO_BUILD_ROOT':str(positive),
               'GO':'false', 'PYTHONDONTWRITEBYTECODE':'1'}
        first_run = None
        first_snapshot = None
        for invocation in range(2):
            result = subprocess.run(['bash', str(SCRIPT_ROOT/'verify.sh')], env=env, capture_output=True, text=True)
            if result.returncode != 1 or 'Path guard rejected:' in result.stderr:
                raise AssertionError('normal external paths were not accepted')
            runs = list(positive.glob('run-*'))
            if len(runs) != invocation+1 or any(stat.S_IMODE(path.stat().st_mode)!=0o700 for path in runs):
                raise AssertionError('run directories were reused or have unsafe mode')
            if invocation == 0:
                first_run = runs[0]
                first_snapshot = snapshot(first_run)
            elif snapshot(first_run) != first_snapshot:
                raise AssertionError('a later run changed existing evidence')
        print('Two normal external invocations created distinct 0700 runs; old bytes unchanged')
        # Minimal synthetic path markers, not a copy of repository source.
        repo = temporary/'synthetic-repo'
        fake_module = repo/'ports/mygo/domain-extensions'
        fake_module.mkdir(parents=True)
        (repo/'.git').mkdir()
        (repo/'.git/HEAD').write_text('ref: refs/heads/synthetic\n')
        (fake_module.parent/'domain-core').mkdir()
        cases = [repo/'other-app'/'output', fake_module.parent/'app'/'output']
        for target in cases:
            before = snapshot(temporary)
            env = dict(defaults, NOONMARK_MYGO_BUILD_ROOT=str(target))
            try:
                GUARD['validate'](fake_module, env)
            except GUARD['UnsafePath']:
                pass
            else:
                raise AssertionError('synthetic whole-repo boundary accepted')
            if snapshot(temporary) != before:
                raise AssertionError('guard function mutated filesystem')
        print('Whole-repository and sibling-domain layout guards: passed')
    print('All 27 path-negative cases passed; no pre-rejection writes')

if __name__ == '__main__':
    main()
