#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""Run every synthetic comparator test in a fresh, standalone source layout."""
import os
import subprocess
import sys
import compare as c


def main():
    base = os.environ.get('NOONMARK_SWIFT_COMPARE_TEST_ROOT', '')
    if not os.environ.get('GO'):
        raise RuntimeError('Explicit Go 1.27.3 GO is required')
    inputs = c.load_inputs()
    # Validate the original source boundary before creating any fixtures.
    run, _ = c.prepare_run(base, inputs)
    workspace = run / 'SYNTHETIC-STANDALONE-WORKSPACE'
    root = workspace / 'checkout'
    blobs = dict(inputs['verifiedBlobs'])
    owned = ['compare.py', 'pins.json', 'probe.go', 'README.md', 'FAILURE_CASES.md', 'run-tests.py']
    owned += sorted(p.name for p in c.HERE.glob('test*.py'))
    c.require('test_compare.py' in owned and 'test_review_regressions.py' in owned,
              'both permanent regression modules are required')
    for name in owned:
        blobs['ports/mygo/scripts/swift-name-compare/' + name] = c.read_file(c.HERE / name, c.MAX_INPUT)
    for name, blob in blobs.items():
        target = root / name
        target.parent.mkdir(parents=True, exist_ok=True)
        with target.open('xb') as out:
            out.write(blob)
    c.require(sorted(p.name for p in workspace.iterdir()) == ['checkout'],
              'standalone source workspace must have no sibling checkout')
    manifest = {'syntheticOnly': True, 'swiftVerified': False,
                'sourceFilesSHA256': {name: c.sha(blob) for name, blob in sorted(blobs.items())},
                'workspaceEntriesBefore': ['checkout'], 'noNoonmarkGoSiblingBefore': True}
    (run / 'standalone-source-manifest.json').write_bytes(c.encoded(manifest))
    env = dict(os.environ, PYTHONDONTWRITEBYTECODE='1',
               NOONMARK_SWIFT_COMPARE_TEST_ROOT=str(run / 'test-output'))
    # Never inherit an import path that could mask the copied comparator.
    env.pop('PYTHONPATH', None)
    command = [sys.executable, '-B', '-m', 'unittest', 'discover', '-s',
               'ports/mygo/scripts/swift-name-compare', '-p', 'test*.py', '-v']
    print('Synthetic standalone regression evidence: ' + str(run), flush=True)
    result = subprocess.run(command, cwd=root, env=env, capture_output=True)
    (run / 'tests.stdout').write_bytes(result.stdout)
    (run / 'tests.stderr').write_bytes(result.stderr)
    after = {name: c.sha((root / name).read_bytes()) for name in blobs}
    actual_files = {str(p.relative_to(root)) for p in root.rglob('*') if p.is_file()}
    source_unchanged = (after == manifest['sourceFilesSHA256'] and actual_files == set(blobs))
    no_sibling = not (workspace / 'noonmark-go').exists()
    report = {'exitCode': result.returncode, 'sourceUnchanged': source_unchanged,
              'noNoonmarkGoSiblingAfter': no_sibling, 'syntheticOnly': True,
              'swiftVerified': False, 'command': command,
              'stdoutSHA256': c.sha(result.stdout), 'stderrSHA256': c.sha(result.stderr)}
    (run / 'standalone-result.json').write_bytes(c.encoded(report))
    sys.stdout.buffer.write(result.stdout)
    sys.stderr.buffer.write(result.stderr)
    c.require(source_unchanged and no_sibling, 'standalone tests wrote into the source layout')
    return result.returncode


if __name__ == '__main__':
    raise SystemExit(main())
