#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""Fixed, bounded platform diagnostics only. Never invoke Swift or oracle capture."""
import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import re
import stat
import sys
import time
import types
import zipfile

HERE = Path(__file__).absolute().parent
REPO = HERE.parents[3]
ORACLE = HERE.parent / 'ordinary-category-oracle'
WORKFLOW = REPO / '.github/workflows/ordinary-category-sandbox-diagnostic.yml'
DEVELOPER = Path('/Applications/Xcode_26.2.app/Contents/Developer')
RUNNER_SHA = '3464a455677d7e45b77d622aa05feb7674d0ba65bf6382ab7e8e2a2f99bc715c'
MANIFEST_SHA = '4d81f1784ef074bba86895dda6127fcce4b293ad1f6a6ac3ed666a28519a0fe8'
SOURCE_SHA = 'ad948c7383b6af6b7cc3a82e0d7752982a9cadd5d38101bd683ccdde6cb0e31d'
INPUT_SHA = '2a3f43ffb5a78165ce9d32958089e0e3973280829fa5f3ab33f69fa44ea70aae'
SCHEMA = 'ordinary-category-sandbox-diagnostic-v1'
PURPOSE = 'platform-preflight-diagnostic-only'
LABELS = ('basic-true', 'system-xcode-version', 'developer-xcode-version')
PROGRAMS = (Path('/usr/bin/true'), Path('/usr/bin/xcodebuild'), DEVELOPER / 'usr/bin/xcodebuild')
SANDBOX = Path('/usr/bin/sandbox-exec')
FALSE_FIELDS = ('captureComplete', 'semanticParityVerified', 'runtimeAdmission', 'swiftExecuted')
REASONS = {'not-started', 'aggregate-budget', 'prior-incomplete', 'setup-integrity', 'identity-change',
           'missing', 'symlink', 'nonregular', 'not-executable', 'writable', 'unreadable', 'receipt-uncertain',
           'transport-incomplete', 'cleanup-uncertain', 'invalid-utf8', 'unexpected-exception', 'redaction-required'}
CAPS = {'summary.json': 32768, 'compile.sb': 8192}
MAX_TOTAL = 524288


def require(ok, code='invalid-contract'):
    if not ok:
        raise ValueError(code)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def identity(s):
    return [s.st_dev, s.st_ino, s.st_mode, s.st_uid, s.st_nlink, s.st_size, s.st_mtime_ns, s.st_ctime_ns]


def root_identity(path):
    s = directory(path)
    return [s.st_dev, s.st_ino, s.st_mode, s.st_uid]


def safe_path(path):
    p = Path(path)
    require(p.is_absolute() and '..' not in p.parts and all(c not in str(p) for c in '\\"\n\r\0'), 'unsafe-path')
    require(str(p) == str(path) and len(str(p)) <= 1024, 'noncanonical-path')
    return p


def parent_fd(path):
    p = safe_path(path)
    fd = os.open('/', os.O_RDONLY | os.O_DIRECTORY)
    try:
        for name in p.parts[1:-1]:
            new = os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd); fd = new
        return fd
    except BaseException:
        os.close(fd)
        raise


def directory(path):
    p = safe_path(path)
    if p == Path('/'):
        return os.stat('/')
    fd = parent_fd(p)
    leaf = None
    try:
        leaf = os.open(p.name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
        s = os.fstat(leaf)
        require(stat.S_ISDIR(s.st_mode), 'non-directory')
        return s
    finally:
        if leaf is not None: os.close(leaf)
        os.close(fd)


def read(path, cap):
    p = safe_path(path); fd = parent_fd(p); leaf = None
    try:
        leaf = os.open(p.name, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW, dir_fd=fd)
        before = os.fstat(leaf)
        require(stat.S_ISREG(before.st_mode) and before.st_nlink == 1 and before.st_size <= cap, 'unsafe-file')
        chunks = []; size = 0
        while True:
            chunk = os.read(leaf, min(65536, cap + 1 - size))
            if not chunk: break
            chunks.append(chunk); size += len(chunk)
            require(size <= cap, 'oversize-file')
        require(identity(before) == identity(os.fstat(leaf)) == identity(os.stat(p.name, dir_fd=fd, follow_symlinks=False)), 'file-identity-change')
        return b''.join(chunks)
    finally:
        if leaf is not None: os.close(leaf)
        os.close(fd)


def encoded(value):
    return (json.dumps(value, sort_keys=True, ensure_ascii=False, separators=(',', ':'), allow_nan=False) + '\n').encode()


def unique(pairs):
    out = {}
    for k, v in pairs:
        require(k not in out, 'duplicate-json-key'); out[k] = v
    return out


def decode(raw):
    return json.loads(raw.decode('utf-8'), object_pairs_hook=unique,
                      parse_constant=lambda _: (_ for _ in ()).throw(ValueError('nonfinite-json')))


def fields(obj, keys):
    require(type(obj) is dict and set(obj) == set(keys), 'object-fields')


def integer(value, minimum=0):
    require(type(value) is int and value >= minimum, 'integer-type')


def finite(value):
    require(type(value) in (int, float) and math.isfinite(value) and value >= 0, 'finite-seconds')


def hash_value(value):
    require(type(value) is str and re.fullmatch('[0-9a-f]{64}', value), 'hash-format')


def write(path, raw):
    p = safe_path(path); fd = parent_fd(p)
    try:
        leaf = os.open(p.name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600, dir_fd=fd)
        with os.fdopen(leaf, 'wb') as f: f.write(raw)
    finally:
        os.close(fd)


def load_runner():
    # Hash the actual bytes before compile/exec; never import then trust self-verification.
    path = ORACLE / 'runner.py'; raw = read(path, 131072)
    require(sha(raw) == RUNNER_SHA, 'runner-hash')
    m = types.ModuleType('verified_ordinary_diagnostic_dependency')
    m.__file__ = str(path)
    exec(compile(raw, str(path), 'exec'), m.__dict__)
    return m


def verify(reviewed_driver):
    hash_value(reviewed_driver)
    raw = read(HERE / 'driver.py', 131072)
    require(sha(raw) == reviewed_driver, 'driver-hash')
    lock = decode(read(HERE / 'CLOSURE.json', 8192))
    fields(lock, {'schema', 'files', 'dependencies'})
    require(lock['schema'] == 'ordinary-sandbox-diagnostic-lock-v1', 'lock-schema')
    expected = {'runnerSHA256': RUNNER_SHA, 'toolManifestSHA256': MANIFEST_SHA,
                'sourceClosureSHA256': SOURCE_SHA, 'inputSHA256': INPUT_SHA}
    require(lock['dependencies'] == expected, 'dependency-lock')
    paths = {'driver.py': HERE / 'driver.py', 'test_diagnostic.py': HERE / 'test_diagnostic.py',
             'CONTRACT.md': HERE / 'CONTRACT.md', 'workflow': WORKFLOW}
    require(type(lock['files']) is dict and set(lock['files']) == set(paths), 'lock-closure')
    for name, path in paths.items():
        hash_value(lock['files'][name]); require(sha(read(path, 131072)) == lock['files'][name], 'closure-hash')
    require(lock['files']['driver.py'] == reviewed_driver, 'self-lock')
    require({p.name for p in HERE.iterdir()} == {'CLOSURE.json', 'driver.py', 'test_diagnostic.py', 'CONTRACT.md'}, 'extra-diagnostic-file')
    r = load_runner()
    for name, expected_sha in [('TOOL-MANIFEST.json', MANIFEST_SHA), ('SOURCE-CLOSURE.json', SOURCE_SHA), ('INPUT.json', INPUT_SHA)]:
        require(sha(read(ORACLE / name, 65536)) == expected_sha, 'dependency-hash')
    r.verify(MANIFEST_SHA)
    return r


def inputs(work, evidence, run_id, attempt, head):
    work = safe_path(work); evidence = safe_path(evidence)
    require(type(run_id) is str and re.fullmatch('[1-9][0-9]{0,19}', run_id), 'run-id')
    require(type(attempt) is str and re.fullmatch('[1-9][0-9]{0,5}', attempt), 'attempt')
    require(type(head) is str and re.fullmatch('[0-9a-f]{40}', head), 'head-sha')
    for a, b in ((work, REPO), (evidence, REPO), (work, evidence)):
        require(a != b and a not in b.parents and b not in a.parents, 'overlap')
    directory(work.parent); directory(evidence.parent)
    return work, evidence


def fresh(path):
    require(not os.path.lexists(path), 'preexisting-root')
    fd = parent_fd(path)
    try:
        os.mkdir(Path(path).name, 0o700, dir_fd=fd)
    finally:
        os.close(fd)
    s = directory(path)
    require(s.st_uid == os.getuid() and stat.S_IMODE(s.st_mode) == 0o700, 'root-owner-mode')


def executable(path):
    # System Mach-O files have no oracle-size/nlink==1 limitation.
    p = safe_path(path); fd = None; leaf = None
    try:
        fd = parent_fd(p)
        before = os.stat(p.name, dir_fd=fd, follow_symlinks=False)
        if stat.S_ISLNK(before.st_mode): return None, 'symlink'
        if not stat.S_ISREG(before.st_mode): return None, 'nonregular'
        if not before.st_mode & 0o111: return None, 'not-executable'
        if before.st_mode & 0o022: return None, 'writable'
        leaf = os.open(p.name, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW, dir_fd=fd)
        require(identity(before) == identity(os.fstat(leaf)) == identity(os.stat(p.name, dir_fd=fd, follow_symlinks=False)), 'executable-identity-change')
        return identity(before), None
    except FileNotFoundError:
        return None, 'missing'
    except OSError as exc:
        # ELOOP or ENOTDIR from a nofollow ancestor is never resolved/followed.
        return None, 'symlink' if exc.errno in (20, 40, 62) else 'unreadable'
    finally:
        if leaf is not None: os.close(leaf)
        if fd is not None: os.close(fd)


def child_env(work):
    return {'PATH': '/usr/bin:/bin', 'HOME': str(work / 'home'), 'TMPDIR': str(work / 'tmp'),
            'LANG': 'en_US.UTF-8', 'LC_ALL': 'en_US.UTF-8', 'TZ': 'UTC', 'DEVELOPER_DIR': str(DEVELOPER),
            'CLANG_MODULE_CACHE_PATH': str(work / 'cache')}


def argv(work, index):
    return [str(SANDBOX), '-f', str(work / 'compile.sb'), str(PROGRAMS[index])] + ([] if index == 0 else ['-version'])


def metadata():
    out = {}
    for k, v in [('imageOS', os.environ.get('ImageOS', 'unknown')), ('imageVersion', os.environ.get('ImageVersion', 'unknown')),
                 ('runnerOS', os.environ.get('RUNNER_OS', 'unknown')), ('arch', platform.machine() or 'unknown')]:
        require(type(v) is str and re.fullmatch('[A-Za-z0-9_.-]{1,80}', v), 'invalid-image-field')
        out[k] = v
    return out


def base_summary(work, run_id, attempt, head, driver):
    return {'schema': SCHEMA, 'purpose': PURPOSE, **{k: False for k in FALSE_FIELDS},
            'runID': run_id, 'attempt': attempt, 'headSHA': head, 'workRoot': str(work),
            'driverSHA256': driver, 'runnerSHA256': RUNNER_SHA, 'toolManifestSHA256': MANIFEST_SHA,
            'sourceClosureSHA256': SOURCE_SHA, 'inputSHA256': INPUT_SHA,
            'profileSHA256': None, 'rootIdentity': None, 'metadata': metadata(),
            'commandPhaseSeconds': 0.0, 'diagnosticComplete': False, 'status': 'setup-failure',
            'probes': [{'label': label, 'state': 'not-run', 'reason': 'not-started'} for label in LABELS]}


def privacy(raw):
    # Fail closed on obvious credentials/private records. This is not a universal DLP claim.
    text = raw.decode('utf-8', errors='strict')
    require(not re.search(r'(?i)(-----BEGIN .*PRIVATE KEY|gh[pousr]_[A-Za-z0-9]{16}|github_pat_|Bearer\s+[A-Za-z0-9._-]{16}|(?:password|secret|access_token)\s*[=:]|/Users/(?!runner(?:/|\b))[^/\s]+/)', text), 'redaction-required')
    return text


def receipt(work, index, raw):
    row = decode(raw)
    fields(row, {'argv', 'returncode', 'seconds', 'transportComplete', 'error', 'cleanupComplete', 'cleanupErrors', 'stdoutCount', 'stderrCount', 'stdout', 'stderr'})
    require(row['argv'] == argv(work, index), 'command-argv')
    require(row['returncode'] is None or type(row['returncode']) is int, 'returncode')
    finite(row['seconds'])
    for k in ('transportComplete', 'cleanupComplete'): require(type(row[k]) is bool, 'bool-type')
    require(type(row['cleanupErrors']) is list and len(row['cleanupErrors']) <= 8, 'cleanup-errors')
    for v in row['cleanupErrors']:
        require(type(v) is str and len(v) <= 256, 'cleanup-text'); privacy(v.encode())
    require(row['cleanupComplete'] == (not row['cleanupErrors']), 'cleanup-conflict')
    error = row['error']
    require(error is None or type(error) is str and len(error) <= 256, 'error-text')
    if error is not None: privacy(error.encode())
    if error is None:
        require(row['returncode'] == 0 and row['transportComplete'], 'false-success')
    if type(error) is str and error.startswith('nonzero exit: '):
        require(row['returncode'] not in (0, None) and error == 'nonzero exit: ' + str(row['returncode']), 'nonzero-error')
    streams = {}; members = {}
    suffix = '.raw' if row['transportComplete'] else '.diagnostic-prefix'
    for stream in ('stdout', 'stderr'):
        ref = row[stream]; fields(ref, {'path', 'bytes', 'sha256'})
        name = LABELS[index] + '-' + stream + suffix
        require(ref['path'] == name, 'stream-path'); integer(ref['bytes']); hash_value(ref['sha256'])
        integer(row[stream + 'Count']); require(ref['bytes'] <= 65536 and row[stream + 'Count'] >= ref['bytes'], 'stream-count')
        if row['transportComplete']: require(row[stream + 'Count'] == ref['bytes'], 'raw-count')
        other = work / (LABELS[index] + '-' + stream + ('.diagnostic-prefix' if suffix == '.raw' else '.raw'))
        require(not os.path.lexists(other), 'ambiguous-stream')
        data = read(work / name, 65536)
        require(len(data) == ref['bytes'] and sha(data) == ref['sha256'], 'stream-hash')
        # Decode checked below for continuation, and again here for export privacy.
        streams[stream] = data; members[name] = data
    return row, streams, members


def continuation(row, streams, caught):
    if not row['cleanupComplete'] or row['cleanupErrors']: return 'cleanup-uncertain'
    if not row['transportComplete'] or row['returncode'] is None: return 'transport-incomplete'
    try:
        for raw in streams.values(): raw.decode('utf-8', errors='strict')
    except UnicodeError: return 'invalid-utf8'
    try:
        for raw in streams.values(): privacy(raw)
    except ValueError: return 'redaction-required'
    if row['returncode'] == 0:
        return 'unexpected-exception' if caught or row['error'] is not None else None
    # Only the original helper's ordinary nonzero exit is safe to continue after.
    if caught is ValueError and row['error'] == 'nonzero exit: ' + str(row['returncode']): return None
    return 'unexpected-exception'


def probe(r, work, run_id, attempt, head, driver):
    require(platform.system() == 'Darwin', 'macos-only')
    directory(DEVELOPER)
    require(not os.path.lexists(work), 'preexisting-root')
    summary = base_summary(work, run_id, attempt, head, driver)
    fresh(work); summary['rootIdentity'] = root_identity(work)
    started = None; frozen = None; stage_id = None
    try:
        for p in ('build', 'cache', 'tmp', 'home', 'run', 'provenance'):
            os.mkdir(work / p, 0o700)
        frozen, stage_id = r.stage_inputs(work, MANIFEST_SHA)
        profile = r.sandbox(work, DEVELOPER, True).encode()
        require(len(profile) <= 8192, 'profile-budget')
        write(work / 'compile.sb', profile)
        profile_id = identity(os.lstat(work / 'compile.sb'))
        summary['profileSHA256'] = sha(profile)
        def guard():
            require(root_identity(work) == summary['rootIdentity'], 'work-identity-change')
            r.assert_frozen(frozen, stage_id, MANIFEST_SHA)
            require(identity(os.lstat(work / 'compile.sb')) == profile_id, 'profile-identity-change')
            require(read(work / 'compile.sb', 8192) == profile == r.sandbox(work, DEVELOPER, True).encode(), 'profile-change')
        ledger = []; started = time.monotonic(); deadline = started + 120.0
        stopped = None; failed = False
        for i, label in enumerate(LABELS):
            if stopped:
                summary['probes'][i] = {'label': label, 'state': 'not-run', 'reason': stopped}
                continue
            guard()
            if deadline - time.monotonic() < 31.0:
                stopped = 'aggregate-budget'; summary['probes'][i] = {'label': label, 'state': 'not-run', 'reason': stopped}; continue
            sid, sreason = executable(SANDBOX); eid, reason = executable(PROGRAMS[i])
            reason = 'setup-integrity' if sreason else reason
            if reason:
                summary['probes'][i] = {'label': label, 'state': 'not-run', 'reason': reason}
                failed = True
                if sreason or i != 2: stopped = 'setup-integrity'
                continue
            # Repeat all input checks immediately before launch, then test aggregate again.
            guard()
            require(executable(SANDBOX) == (sid, None) and executable(PROGRAMS[i]) == (eid, None), 'executable-identity-change')
            if deadline - time.monotonic() < 31.0:
                stopped = 'aggregate-budget'; summary['probes'][i] = {'label': label, 'state': 'not-run', 'reason': stopped}; continue
            before = len(ledger); caught = None
            try:
                r.bounded_process(argv(work, i), work, child_env(work), 20, label, ledger)
            except BaseException as exc:
                caught = type(exc)
            expired = time.monotonic() > deadline
            # Unknown receipt/identity results stop later execution, never fabricate success.
            try:
                require(len(ledger) == before + 1, 'ledger-count')
                raw = read(work / (label + '-command.json'), 8192)
                row, streams, _ = receipt(work, i, raw)
                require(row == ledger[-1], 'ledger-disk-mismatch')
                guard()
                require(executable(SANDBOX) == (sid, None) and executable(PROGRAMS[i]) == (eid, None), 'executable-identity-change')
            except BaseException:
                summary['probes'][i] = {'label': label, 'state': 'unknown', 'reason': 'receipt-uncertain'}
                stopped = 'prior-incomplete'; failed = True; continue
            stop = continuation(row, streams, caught)
            version = None if i == 0 else bool(row['returncode'] == 0 and stop is None and streams['stdout'].decode('utf-8').splitlines()[:1] == ['Xcode 26.2'])
            summary['probes'][i] = {'label': label, 'state': 'attempted', 'receiptSHA256': sha(raw),
                                    'sandboxIdentity': sid, 'executableIdentity': eid, 'versionMatches': version,
                                    'stopReason': stop}
            failed |= row['returncode'] != 0 or version is False or stop is not None
            if stop: stopped = 'prior-incomplete'
            if expired or time.monotonic() > deadline: stopped = 'aggregate-budget'
        guard()
        summary['commandPhaseSeconds'] = time.monotonic() - started
        complete = not stopped and all(x['state'] == 'attempted' or (i == 2 and x['state'] == 'not-run') for i, x in enumerate(summary['probes']))
        summary['diagnosticComplete'] = complete
        summary['status'] = ('probe-failure' if failed else 'completed') if complete else 'incomplete'
    except BaseException:
        if started is not None: summary['commandPhaseSeconds'] = time.monotonic() - started
        summary['status'] = 'setup-failure' if started is None else 'incomplete'
        for i, slot in enumerate(summary['probes']):
            if slot['state'] == 'not-run' and slot['reason'] == 'not-started':
                summary['probes'][i] = {'label': LABELS[i], 'state': 'not-run', 'reason': 'setup-integrity'}
    raw = encoded(summary); require(len(raw) <= 32768, 'summary-budget')
    require(root_identity(work) == summary['rootIdentity'], 'work-identity-change')
    write(work / 'summary.json', raw)
    return 0 if summary['status'] == 'completed' else 1


def validate_summary(r, work, raw, run_id, attempt, head, driver):
    s = decode(raw)
    expected = base_summary(work, run_id, attempt, head, driver)
    fields(s, expected.keys())
    for k in ('schema', 'purpose', *FALSE_FIELDS, 'runID', 'attempt', 'headSHA', 'workRoot',
              'driverSHA256', 'runnerSHA256', 'toolManifestSHA256', 'sourceClosureSHA256', 'inputSHA256'):
        require(type(s[k]) is type(expected[k]) and s[k] == expected[k], 'summary-binding')
    require(type(s['rootIdentity']) is list and len(s['rootIdentity']) == 4, 'root-identity')
    for value in s['rootIdentity']: integer(value)
    require(s['rootIdentity'] == root_identity(work), 'root-identity')
    require(s['rootIdentity'][3] == os.getuid() and stat.S_IMODE(s['rootIdentity'][2]) == 0o700, 'root-owner-mode')
    fields(s['metadata'], {'imageOS', 'imageVersion', 'runnerOS', 'arch'})
    for v in s['metadata'].values(): require(type(v) is str and re.fullmatch('[A-Za-z0-9_.-]{1,80}', v), 'metadata')
    finite(s['commandPhaseSeconds'])
    require(type(s['diagnosticComplete']) is bool and s['status'] in ('completed', 'probe-failure', 'setup-failure', 'incomplete'), 'diagnostic-status')
    profile = read(work / 'compile.sb', 8192)
    require(profile == r.sandbox(work, DEVELOPER, True).encode() and s['profileSHA256'] == sha(profile), 'profile-binding')
    require(type(s['probes']) is list and len(s['probes']) == 3, 'probe-count')
    members = {'summary.json': raw, 'compile.sb': profile}; failed = False; interrupted = False; total_seconds = 0.0
    for i, slot in enumerate(s['probes']):
        require(type(slot) is dict and slot.get('label') == LABELS[i], 'probe-order')
        state = slot.get('state')
        if state in ('not-run', 'unknown'):
            fields(slot, {'label', 'state', 'reason'})
            require(slot['reason'] in REASONS, 'not-run-reason')
            # Only an unavailable C can explain a complete plan with a non-attempt.
            if state == 'unknown' or i != 2 or slot['reason'] not in {'missing', 'symlink', 'nonregular', 'not-executable', 'writable', 'unreadable'}:
                interrupted = True
            failed = True
            continue
        require(state == 'attempted' and not interrupted, 'attempt-after-stop')
        fields(slot, {'label', 'state', 'receiptSHA256', 'sandboxIdentity', 'executableIdentity', 'versionMatches', 'stopReason'})
        for key in ('sandboxIdentity', 'executableIdentity'):
            ident = slot[key]
            require(type(ident) is list and len(ident) == 8, 'executable-identity')
            for value in ident: integer(value)
            require(stat.S_ISREG(ident[2]) and ident[2] & 0o111 and not ident[2] & 0o022, 'executable-mode')
        hash_value(slot['receiptSHA256'])
        name = LABELS[i] + '-command.json'; command = read(work / name, 8192)
        require(sha(command) == slot['receiptSHA256'], 'receipt-binding')
        row, streams, pipes = receipt(work, i, command)
        for data in streams.values(): privacy(data)
        # Strings in original receipts remain unmodified, but never export suspected secrets.
        privacy(command)
        members[name] = command; members.update(pipes); total_seconds += row['seconds']
        safe = continuation(row, streams, None if row['returncode'] == 0 else ValueError)
        stop = slot['stopReason']
        require(stop == safe or (safe is None and stop == 'unexpected-exception'), 'stop-reason')
        if stop: interrupted = True
        version = None if i == 0 else bool(row['returncode'] == 0 and stop is None and streams['stdout'].decode('utf-8').splitlines()[:1] == ['Xcode 26.2'])
        require(type(slot['versionMatches']) is type(version) and slot['versionMatches'] == version, 'version-result')
        failed |= row['returncode'] != 0 or version is False or stop is not None
    require(s['commandPhaseSeconds'] + 0.000001 >= total_seconds, 'phase-duration')
    if s['commandPhaseSeconds'] > 120: interrupted = True
    complete = not interrupted
    require(s['diagnosticComplete'] == complete, 'complete-result')
    if complete:
        require(s['status'] == ('probe-failure' if failed else 'completed'), 'status-result')
    else:
        require(s['status'] in ('incomplete', 'setup-failure'), 'status-result')
    require(len(members) <= 11 and sum(map(len, members.values())) <= MAX_TOTAL, 'archive-budget')
    return s, members


def export(r, work, evidence, run_id, attempt, head, driver):
    import io
    fresh(evidence)
    evidence_id = root_identity(evidence)
    try:
        root = root_identity(work)
        raw = read(work / 'summary.json', 32768)
        summary, members = validate_summary(r, work, raw, run_id, attempt, head, driver)
        # Validate the complete selected closure again, including identity, before packaging.
        identities = {name: identity(os.lstat(work / name)) for name in members}
        for name, data in members.items():
            cap = CAPS.get(name, 8192 if name.endswith('-command.json') else 65536)
            require(read(work / name, cap) == data and identity(os.lstat(work / name)) == identities[name], 'export-drift')
        require(root_identity(work) == root, 'export-root-drift')
        out = io.BytesIO()
        with zipfile.ZipFile(out, 'w', compression=zipfile.ZIP_DEFLATED) as z:
            for name, data in sorted(members.items()):
                require(re.fullmatch('[A-Za-z0-9_.-]+', name), 'member-name')
                info = zipfile.ZipInfo(name, (1980, 1, 1, 0, 0, 0)); info.create_system = 3
                info.external_attr = (stat.S_IFREG | 0o444) << 16; info.compress_type = zipfile.ZIP_DEFLATED
                z.writestr(info, data)
        archive = out.getvalue(); require(len(archive) <= MAX_TOTAL, 'zip-budget')
        public = {'schema': SCHEMA, 'purpose': PURPOSE, **{k: False for k in FALSE_FIELDS},
                  'evidenceValidated': True, 'diagnosticComplete': summary['diagnosticComplete'], 'status': summary['status'],
                  'runID': run_id, 'attempt': attempt, 'headSHA': head,
                  'zip': {'path': 'sandbox-diagnostic-evidence.zip', 'bytes': len(archive), 'sha256': sha(archive)},
                  'files': [{'path': k, 'bytes': len(v), 'sha256': sha(v)} for k, v in sorted(members.items())]}
        report = encoded(public); require(len(report) <= 32768, 'export-summary-budget')
    except BaseException as exc:
        # Do not serialize arbitrary OS errors, paths, or exception strings.
        reason = 'redaction-required' if type(exc) is ValueError and str(exc) == 'redaction-required' else 'evidence-invalid-or-unavailable'
        require(root_identity(evidence) == evidence_id, 'evidence-identity-change')
        write(evidence / 'export-summary.json', encoded({'schema': SCHEMA, 'purpose': PURPOSE,
              **{k: False for k in FALSE_FIELDS}, 'evidenceValidated': False, 'diagnosticComplete': False,
              'status': 'incomplete', 'reason': reason, 'zip': None}))
        return 1
    require(root_identity(evidence) == evidence_id, 'evidence-identity-change')
    write(evidence / 'sandbox-diagnostic-evidence.zip', archive)
    write(evidence / 'export-summary.json', report)
    return 0


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('action', choices=('verify', 'probe', 'export'))
    p.add_argument('--reviewed-driver-sha256', required=True)
    p.add_argument('--reviewed-manifest-sha256', choices=(MANIFEST_SHA,), required=True)
    p.add_argument('--work-root'); p.add_argument('--evidence-root')
    p.add_argument('--run-id'); p.add_argument('--attempt'); p.add_argument('--head-sha')
    args = p.parse_args()
    r = verify(args.reviewed_driver_sha256)
    if args.action == 'verify':
        require(all(v is None for v in (args.work_root, args.evidence_root, args.run_id, args.attempt, args.head_sha)), 'verify-inputs')
        print(json.dumps({'staticVerified': True, **{k: False for k in FALSE_FIELDS}}, sort_keys=True)); return 0
    work, evidence = inputs(args.work_root, args.evidence_root, args.run_id, args.attempt, args.head_sha)
    if args.action == 'probe':
        require(not os.path.lexists(evidence), 'preexisting-evidence')
        return probe(r, work, args.run_id, args.attempt, args.head_sha, args.reviewed_driver_sha256)
    return export(r, work, evidence, args.run_id, args.attempt, args.head_sha, args.reviewed_driver_sha256)


if __name__ == '__main__':
    try:
        sys.exit(main())
    except Exception:
        print('sandbox diagnostic failed closed; no capture or admission claim', file=sys.stderr)
        sys.exit(1)
