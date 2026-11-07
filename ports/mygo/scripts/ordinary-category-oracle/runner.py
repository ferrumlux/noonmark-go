#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""Independent ordinary-category reference only. Default: static verify, never compile.
Capture requires a reviewed tool-manifest hash and a separate empty external work root.
No downloads, hosted dispatch, product invocation or existing oracle imports.
"""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import selectors
import signal
import stat
import subprocess
import sys
import time

HERE = Path(__file__).resolve().parent
COMMIT = '43e6eefaae3a04be89cab3e234d470111afd80ef'
INPUT_SHA = '2a3f43ffb5a78165ce9d32958089e0e3973280829fa5f3ab33f69fa44ea70aae'
SOURCE_SHA = 'ad948c7383b6af6b7cc3a82e0d7752982a9cadd5d38101bd683ccdde6cb0e31d'
MAX_RAW = 262144


def require(value, reason):
    if not value:
        raise ValueError(reason)


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def file_identity(st):
    return (st.st_dev, st.st_ino, st.st_mode, st.st_nlink, st.st_uid, st.st_size, st.st_mtime_ns, st.st_ctime_ns)


def real_directory(path):
    path = Path(path)
    require(path.is_absolute() and '..' not in path.parts, 'directory must be absolute')
    fd = os.open('/', os.O_RDONLY | os.O_DIRECTORY)
    try:
        for name in path.parts[1:]:
            new = os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd); fd = new
        st = os.fstat(fd)
        require(stat.S_ISDIR(st.st_mode), 'not a real directory')
        return file_identity(st)
    finally:
        os.close(fd)


def checked(path, cap=MAX_RAW):
    # Walk/open via directory descriptors: no linked ancestor, leaf or hardlink.
    path = Path(path)
    require(path.is_absolute() and '..' not in path.parts, 'input absolute path')
    fd = os.open('/', os.O_RDONLY | os.O_DIRECTORY)
    leaf = None
    try:
        for part in path.parts[1:-1]:
            new = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd); fd = new
        leaf = os.open(path.name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=fd)
        before = os.fstat(leaf)
        require(stat.S_ISREG(before.st_mode) and before.st_nlink == 1 and before.st_size <= cap, 'nonregular, linked or oversized input: ' + str(path))
        chunks = []; total = 0
        while True:
            raw = os.read(leaf, min(65536, cap + 1 - total))
            if not raw: break
            chunks.append(raw); total += len(raw)
            require(total <= cap, 'input grew beyond cap')
        require(file_identity(before) == file_identity(os.fstat(leaf)) == file_identity(os.stat(path.name, dir_fd=fd, follow_symlinks=False)), 'input identity changed during read')
        return b''.join(chunks)
    finally:
        if leaf is not None: os.close(leaf)
        os.close(fd)


def json_read(path, cap=MAX_RAW):
    return json.loads(checked(path, cap))


def write(path, raw):
    with path.open('xb') as f:
        f.write(raw)


def dump(path, value):
    write(path, (json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':')) + '\n').encode())


def verify(reviewed=None, root=None):
    root = HERE if root is None else root
    real_directory(root)
    source_raw = checked(root / 'SOURCE-CLOSURE.json', 65536)
    require(digest(source_raw) == SOURCE_SHA, 'source closure lock mismatch')
    source = json.loads(source_raw)
    require(source['commit'] == COMMIT and len(source['files']) == 15, 'closure scope')
    names = set()
    for row in source['files']:
        p = row['path']
        require(p not in names and not Path(p).is_absolute() and '..' not in Path(p).parts, 'source path')
        names.add(p)
        raw = checked(root / 'upstream' / p)
        require((len(raw), digest(raw), hashlib.sha1(b'blob ' + str(len(raw)).encode() + b'\0' + raw).hexdigest()) == (row['bytes'], row['sha256'], row['gitBlob']), 'source bytes mismatch: ' + p)
    actual = {str(p.relative_to(root / 'upstream')) for p in (root / 'upstream').rglob('*') if p.is_file() or p.is_symlink()}
    require(actual == names, 'extra or missing upstream input')
    require(digest(checked(root / 'INPUT.json', 16384)) == INPUT_SHA, 'frozen input hash')
    toolraw = checked(root / 'TOOL-MANIFEST.json', 65536)
    if reviewed is not None:
        require(digest(toolraw) == reviewed, 'reviewed tool manifest mismatch')
    tools = json.loads(toolraw)
    require(tools['scope'] == 'ordinary-category-only-v1', 'tool scope')
    seen = set()
    for row in tools['files']:
        p = row['path']
        require(p not in seen and '/' not in p and p not in ('TOOL-MANIFEST.json', ''), 'tool path')
        seen.add(p)
        raw = checked(root / p)
        require(len(raw) == row['bytes'] and digest(raw) == row['sha256'], 'tool hash: ' + p)
    require({'runner.py', 'OrdinaryCategoryOracle.swift', 'INPUT.json', 'SOURCE-CLOSURE.json', 'SCHEMA.md', 'PLAN.md'} <= seen, 'missing mandatory tool')
    actual_tools = {p.name for p in root.iterdir() if p.name != 'upstream'}
    require(actual_tools == seen | {'TOOL-MANIFEST.json'}, 'extra or missing tool input')
    return source, digest(toolraw)


def cleanup_group(p):
    errors = []
    try:
        os.killpg(p.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    except BaseException as exc:
        errors.append('killpg: ' + str(exc))
    try:
        p.wait(timeout=5)
    except BaseException as exc:
        errors.append('reap: ' + str(exc))
    deadline = time.monotonic() + 5
    while True:
        try:
            os.killpg(p.pid, 0)
        except ProcessLookupError:
            break
        except BaseException as exc:
            errors.append('group status: ' + str(exc)); break
        if time.monotonic() >= deadline:
            errors.append('residual process group after cleanup'); break
        time.sleep(0.01)
    return errors


def bounded_process(argv, work, env, seconds, label, ledger):
    started = time.monotonic()
    buffers = {'stdout': bytearray(), 'stderr': bytearray()}
    counts = {'stdout': 0, 'stderr': 0}
    sel = None; p = None; error = None; pipes_complete = False; cleanup_errors = []
    try:
        p = subprocess.Popen(argv, cwd=work, env=env, stdin=subprocess.DEVNULL,
                             stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
        # Everything after Popen, including selector setup, is inside lifetime guard.
        sel = selectors.DefaultSelector()
        for name, stream in [('stdout', p.stdout), ('stderr', p.stderr)]:
            os.set_blocking(stream.fileno(), False); sel.register(stream, selectors.EVENT_READ, name)
        while sel.get_map():
            if time.monotonic() - started > seconds:
                raise TimeoutError('process deadline')
            for key, _ in sel.select(min(0.1, max(0.001, seconds - (time.monotonic() - started)))):
                data = os.read(key.fd, 8192)
                if not data:
                    sel.unregister(key.fileobj); continue
                counts[key.data] += len(data)
                if counts[key.data] > 65536:
                    raise ValueError(key.data + ' exceeded 64KiB; prefix is diagnostic, not original')
                buffers[key.data].extend(data)
        pipes_complete = True
        p.wait(timeout=max(0.01, seconds - (time.monotonic() - started)))
        if p.returncode != 0: error = 'nonzero exit: ' + str(p.returncode)
    except BaseException as exc:
        error = str(exc)
    finally:
        # Kill residual descendants even after normal success/nonzero EOF.
        if p is not None:
            cleanup_errors = cleanup_group(p)
        if sel is not None:
            try: sel.close()
            except BaseException as exc: cleanup_errors.append('selector close: ' + str(exc))
        if p is not None:
            for stream in (p.stdout, p.stderr):
                try: stream.close()
                except BaseException as exc: cleanup_errors.append('pipe close: ' + str(exc))
    row = {'argv': argv, 'returncode': p.returncode if p is not None else None,
           'seconds': time.monotonic() - started, 'transportComplete': pipes_complete,
           'error': error, 'cleanupComplete': not cleanup_errors, 'cleanupErrors': cleanup_errors,
           'stdoutCount': counts['stdout'], 'stderrCount': counts['stderr']}
    for name in buffers:
        suffix = '.raw' if pipes_complete else '.diagnostic-prefix'
        path = work / (label + '-' + name + suffix)
        write(path, buffers[name]); row[name] = {'path': path.name, 'bytes': len(buffers[name]), 'sha256': digest(buffers[name])}
    ledger.append(row); dump(work / (label + '-command.json'), row)
    require(error is None and p is not None and p.returncode == 0 and not cleanup_errors, label + ' did not complete successfully')
    return bytes(buffers['stdout']).decode('utf-8', errors='strict').strip()


def safe(path):
    s = str(path)
    require(s.startswith('/') and not any(c in s for c in '\\"\n\r\0'), 'sandbox path')
    return s


def sandbox(work, developer, compiling):
    reads = [Path('/System/Library'), Path('/usr/lib'), Path('/usr/bin'), Path('/usr/share'), Path('/bin'), developer, work]
    ancestors = sorted({p for r in reads for p in r.parents})
    lines = ['(version 1)', '(deny default)', '(deny network*)',
             '(allow process-exec process-fork signal sysctl-read)',
             '(allow mach-lookup (global-name "com.apple.system.logger") (global-name "com.apple.system.notification_center"))',
             '(allow file-read-metadata ' + ' '.join('(literal "' + safe(p) + '")' for p in ancestors) + ')',
             '(allow file-read* ' + ' '.join('(subpath "' + safe(p) + '")' for p in reads) + ')',
             '(allow file-read* (literal "/dev/null") (literal "/dev/random") (literal "/dev/urandom"))',
             '(allow file-write* (literal "/dev/null"))']
    writable = ['build', 'cache', 'tmp', 'home'] if compiling else ['run', 'tmp', 'home']
    lines.append('(allow file-write* ' + ' '.join('(subpath "' + safe(work / p) + '")' for p in writable) + ')')
    return '\n'.join(lines) + '\n'


POSITIVE_CONTROLS = {
    'main-new', 'main-repeat', 'main-retry-original', 'main-retry-changed-now', 'main-none', 'main-none-again',
    'chain-b-first-none-earlier-global-time', 'chain-b-existing', 'chain-b-new-name-reuse',
    'same-base-first-positive', 'tamper-positive-control',
}
CONTROL_REFS = {
    'main-repeat': ['main-new'], 'relation-clock-backward': ['main-new'],
    'main-retry-original': ['main-new', 'main-repeat'], 'main-retry-changed-now': ['main-new', 'main-repeat'],
    'retry-changed-decision': ['main-new', 'main-repeat'], 'main-none': ['main-repeat'], 'main-none-again': ['main-none'],
    'chain-b-first-none-earlier-global-time': ['main-none-again'],
    'chain-b-existing': ['main-new', 'chain-b-first-none-earlier-global-time'], 'chain-b-new-name-reuse': ['chain-b-existing'],
    'same-base-first-positive': ['chain-b-new-name-reuse'], 'tamper-positive-control': ['chain-b-new-name-reuse'],
    'same-base-stale-second': ['same-base-first-positive'],
    **{c: ['tamper-positive-control'] for c in ('preview-tamper-old-digest', 'preview-tamper-rehashed', 'digest-tamper')},
    **{c: ['main-none'] for c in ('missing-removal', 'removal-origin-decision', 'removal-time-backward', 'removal-revision-not-later', 'audit-order-reversed')},
}
STATE_MUTATIONS = {'missing-removal', 'removal-origin-decision', 'removal-time-backward', 'removal-revision-not-later', 'audit-order-reversed'}
TAMPERS = {'preview-tamper-old-digest', 'digest-tamper'}
STATE_FIELDS = {'formatVersion', 'revision', 'categories', 'labels', 'currentByChainID', 'relationHistory', 'categoryMerges',
                'labelMerges', 'categoryDeletionTombstones', 'labelDeletionTombstones', 'snapshotsByTraceID',
                'snapshotEventsByTraceID', 'committedReceiptsByInteractionID', 'changeRecords'}
ROLES = {'frozen-original-input', 'observation', 'actual-synthetic-ordinary-graph', 'actual-before-state-bytes',
         'actual-Foundation-state-Codable-bytes', 'reconstructed-plan-payload-not-intercepted',
         'reconstructed-record-payload-not-intercepted', 'actual-Foundation-record-Codable-bytes',
         'actual-Foundation-receipt-bytes', 'actual-category-Codable-bytes'}


def expected_cases(frozen):
    require(digest(frozen) == INPUT_SHA, 'trusted frozen input bytes differ')
    inp = json.loads(frozen)
    fixed = ['main-new', 'main-repeat', 'main-retry-original', 'main-retry-changed-now', 'main-none', 'main-none-again',
             'chain-b-first-none-earlier-global-time', 'chain-b-existing', 'chain-b-new-name-reuse', 'relation-clock-backward',
             'retry-changed-decision', 'same-base-first-positive', 'same-base-stale-second', 'tamper-positive-control',
             'preview-tamper-old-digest', 'preview-tamper-rehashed', 'digest-tamper', 'missing-removal',
             'removal-origin-decision', 'removal-time-backward', 'removal-revision-not-later', 'audit-order-reversed']
    for n in inp['names']:
        fixed += ['name-' + n['id'] + s for s in ['-direct', '-prepared-create-reference', '-core-new', '-ui-reference-input']]
    fixed += ['clock-negativeZero', 'clock-zero', 'clock-submillisecond']
    require(len(fixed) == 65 and len(set(fixed)) == 65, 'frozen observation count')
    return fixed


def object_fields(value, required, optional=()):
    require(isinstance(value, dict) and set(required) <= set(value) <= set(required) | set(optional), 'missing/unknown object fields')


def meaningful(value):
    return isinstance(value, str) and 0 < len(value) <= 4096


def stage(value, statuses=('ok', 'error', 'not-run'), extras=()):
    require(isinstance(value, dict) and value.get('status') in statuses, 'missing/invalid stage status')
    status = value['status']
    if status == 'error':
        object_fields(value, {'status', 'errorType', 'message'})
        require(meaningful(value['errorType']) and meaningful(value['message']), 'missing error details')
    elif status == 'not-run':
        object_fields(value, {'status', 'reason'}); require(meaningful(value['reason']), 'missing not-run reason')
    else:
        object_fields(value, {'status'} | set(extras))
    return status


def text_view(value):
    object_fields(value, {'text', 'utf8Base64'})
    require(isinstance(value['text'], str) and isinstance(value['utf8Base64'], str), 'text/UTF8 evidence')
    require(base64.b64decode(value['utf8Base64'], validate=True) == value['text'].encode('utf-8'), 'text/UTF8 bytes disagree')


def intent_view(value):
    require(isinstance(value, dict), 'prepared/request intent missing')
    if value.get('kind') == 'createCategory':
        object_fields(value, {'kind', 'name', 'colorHex', 'referenceOnly'})
        require(value['referenceOnly'] is True, 'standalone create must be reference-only')
        text_view(value['name']); text_view(value['colorHex'])
    else:
        object_fields(value, {'kind', 'chainID', 'category', 'labels'})
        require(value['kind'] == 'setCurrent' and meaningful(value['chainID']) and value['labels'] == [], 'ordinary intent scope')
        c = value['category']; require(isinstance(c, dict), 'category choice missing')
        if c.get('kind') == 'none': object_fields(c, {'kind'})
        elif c.get('kind') == 'existing':
            object_fields(c, {'kind', 'categoryID'}); require(meaningful(c['categoryID']), 'existing ID')
        else:
            object_fields(c, {'kind', 'name', 'colorHex'}); require(c['kind'] == 'new', 'choice kind')
            text_view(c['name']); text_view(c['colorHex'])


def state_projection(value):
    object_fields(value, {'revision', 'categories', 'currentKeys', 'emptyCurrentKeys', 'recordIDsInOriginalOrder',
                         'removalIDsInOriginalOrder', 'receiptInteractionIDs', 'dateBitsHex'})
    require(isinstance(value['revision'], str) and value['revision'].isdigit(), 'state revision string')
    for k in ('categories', 'currentKeys', 'emptyCurrentKeys', 'recordIDsInOriginalOrder', 'removalIDsInOriginalOrder', 'receiptInteractionIDs'):
        require(isinstance(value[k], list), 'state array missing')
    require(isinstance(value['dateBitsHex'], dict), 'state date bits')
    for k, v in value['dateBitsHex'].items():
        require(meaningful(k) and isinstance(v, str) and re.fullmatch('[0-9a-f]{16}', v), 'state date bit shape')


def verify_capture(run, frozen=None):
    real_directory(run)
    frozen = checked(HERE / 'INPUT.json', 16384) if frozen is None else frozen
    fixed = expected_cases(frozen)
    initial_output_identity = {p.name: file_identity(p.lstat()) for p in [run, *run.iterdir()]}
    manifest_raw = checked(run / 'manifest.json', 131072)
    m = json.loads(manifest_raw)
    object_fields(m, {'schema', 'suite', 'upstreamCommit', 'inputSHA256', 'captureComplete', 'semanticParityVerified',
                     'runtimeAdmission', 'fullSnapshotExecuted', 'causalIDNormalizationApplied', 'algorithmVersion',
                     'observationIDs', 'artifactBytes', 'files'})
    require(m['schema'] == 'ordinary-category-artifacts-v1' and m['suite'] == 'ordinary-category-only-v1', 'capture schema')
    require(m['captureComplete'] is True and all(m[k] is False for k in ('semanticParityVerified', 'runtimeAdmission', 'fullSnapshotExecuted', 'causalIDNormalizationApplied')), 'capture boundary')
    require(m['inputSHA256'] == INPUT_SHA and m['upstreamCommit'] == COMMIT, 'capture identity')
    require(isinstance(m['algorithmVersion'], str) and re.fullmatch(r'unicode-[0-9]+(?:\.[0-9]+){1,3}-nfkc-casefold-whitespace-v1', m['algorithmVersion']), 'missing canonicalizer version')
    require(m['observationIDs'] == fixed, 'missing, extra or reordered observation')
    require(isinstance(m['files'], list) and len(m['files']) <= 512, 'artifact count')
    total = 0; refs = {}; rawfiles = {}
    for ref in m['files']:
        object_fields(ref, {'path', 'bytes', 'sha256', 'role'})
        name = ref['path']
        require(isinstance(name, str) and re.fullmatch('[A-Za-z0-9][A-Za-z0-9._-]{0,119}', name) and name not in refs, 'artifact path/duplicate')
        require(ref['role'] in ROLES, 'artifact role')
        cap = 65536 if ref['role'] == 'observation' else 16384 if ref['role'] == 'frozen-original-input' else MAX_RAW
        raw = checked(run / name, cap)
        require(type(ref['bytes']) is int and len(raw) == ref['bytes'] and digest(raw) == ref['sha256'], 'artifact hash: ' + name)
        total += len(raw); require(total <= 16777216, 'capture total budget')
        refs[name] = ref; rawfiles[name] = raw
    require(type(m['artifactBytes']) is int and total == m['artifactBytes'], 'capture total mismatch')
    require({p.name for p in run.iterdir()} == set(refs) | {'manifest.json'}, 'unlisted output')
    require('input-original.json' in refs and refs['input-original.json']['role'] == 'frozen-original-input' and rawfiles['input-original.json'] == frozen and digest(rawfiles['input-original.json']) == INPUT_SHA, 'actual archived input differs from frozen input')
    used = {'input-original.json'}

    def reference(ref, name, role):
        require(isinstance(ref, dict) and ref == refs.get(name) and ref['role'] == role, 'raw reference metadata/path/role mismatch: ' + name)
        used.add(name); return rawfiles[name]

    def state_raw(ref, name, role, projection):
        raw = reference(ref, name, role); v = json.loads(raw)
        object_fields(v, STATE_FIELDS)
        require(v['formatVersion'] == 1 and type(v['revision']) is int and str(v['revision']) == projection['revision'], 'raw/projection state identity')
        for k in STATE_FIELDS - {'formatVersion', 'revision'}:
            require(isinstance(v[k], (list, dict)), 'raw state collection missing')
        return v

    def after_stages(v, case):
        object_fields(v, {'state', 'integrity', 'encode', 'decode'})
        state_projection(v['state']); integrity = stage(v['integrity'], ('ok', 'error'))
        enc = stage(v['encode'], ('ok', 'error'), ('originalBytes', 'dateStrategy'))
        if enc == 'error':
            stage(v['decode'], ('not-run',)); return False
        require(v['encode']['dateStrategy'] == 'deferredToDate', 'state date strategy')
        state_raw(v['encode']['originalBytes'], case + '-state.json', 'actual-Foundation-state-Codable-bytes', v['state'])
        dec = stage(v['decode'], ('ok', 'error'), ('stateEqual', 'allDateBitsEqual', 'decodedDateBitsHex'))
        if dec == 'ok':
            require(type(v['decode']['stateEqual']) is bool and type(v['decode']['allDateBitsEqual']) is bool and isinstance(v['decode']['decodedDateBitsHex'], dict), 'decode evidence types')
            require(v['decode']['allDateBitsEqual'] == (v['decode']['decodedDateBitsHex'] == v['state']['dateBitsHex']), 'decoded bit comparison')
        return integrity == enc == dec == 'ok'

    passed = set()
    for case in fixed:
        observation_ref = refs.get(case + '.json')
        require(observation_ref is not None, 'missing observation artifact')
        v = json.loads(reference(observation_ref, case + '.json', 'observation'))
        require(v.get('schema') == 'ordinary-category-observation-v1' and v.get('caseID') == case, 'observation identity')
        if case.endswith('-direct'):
            object_fields(v, {'schema', 'caseID', 'kind', 'entry', 'raw', 'DAndKStages', 'name', 'canonicalKey',
                              'originalCategoryID', 'originalNameVersionIDs', 'nowBitsHex', 'encode', 'decode'})
            require(v['kind'] == 'normalization-reference' and v['entry'] == 'direct-initializer', 'direct variant')
            for name in ('raw', 'name', 'canonicalKey'): text_view(v[name])
            require(isinstance(v['DAndKStages'], list) and [r.get('stage') for r in v['DAndKStages']] == ['D' + str(i) for i in range(6)], 'normalization stage evidence')
            for row in v['DAndKStages']:
                object_fields(row, {'stage', 'name', 'KOfThisStage'}); text_view(row['name']); text_view(row['KOfThisStage'])
            stage(v['encode'], ('ok',), ('originalBytes',))
            raw = json.loads(reference(v['encode']['originalBytes'], case + '-category.json', 'actual-category-Codable-bytes'))
            object_fields(raw, {'id', 'name', 'canonicalKey', 'canonicalKeyVersion', 'colorHex', 'presentationApproval', 'lifecycle', 'nameVersions', 'createdAt', 'updatedAt'})
            require(raw['canonicalKeyVersion'] == m['algorithmVersion'], 'category algorithm version')
            if stage(v['decode'], ('ok', 'error'), ('name', 'createdAtBitsHex')) == 'ok':
                text_view(v['decode']['name']); require(re.fullmatch('[0-9a-f]{16}', v['decode']['createdAtBitsHex']), 'category decode date bits')
            continue
        expected_refs = CONTROL_REFS.get(case, [])
        require(v.get('controlRefs') == expected_refs and all(c in passed for c in expected_refs), 'missing/invalid successful control reference')
        if case in STATE_MUTATIONS:
            object_fields(v, {'schema', 'caseID', 'kind', 'prepare', 'commit', 'positiveControlReference', 'controlRefs', 'before', 'afterStages'})
            require(v['kind'] == 'single-field-state-mutation' and v['positiveControlReference'] == 'main-none', 'state mutation variant')
            stage(v['prepare'], ('not-run',)); stage(v['commit'], ('not-run',)); state_projection(v['before'])
            after_stages(v['afterStages'], case); continue
        require(v.get('kind') == 'command' and v.get('requiredPositiveControl') is (case in POSITIVE_CONTROLS), 'command/control variant')
        require(type(v.get('logicalAndDateBitsUnchanged')) is bool, 'missing logical/date unchanged evidence')
        require(isinstance(v.get('request'), dict), 'missing original request')
        prep = stage(v.get('prepare'))
        if prep != 'not-run':
            object_fields(v['request'], {'intent', 'interactionID', 'prepareNowBitsHex'})
            intent_view(v['request']['intent'])
            require(meaningful(v['request']['interactionID']) and re.fullmatch('[0-9a-f]{16}', v['request']['prepareNowBitsHex']), 'raw request identity/clock missing')
        if prep == 'error':
            require(case.startswith('name-'), 'required command prepare evidence missing')
            object_fields(v, {'schema', 'caseID', 'kind', 'request', 'prepare', 'commit', 'logicalAndDateBitsUnchanged', 'controlRefs', 'requiredPositiveControl', 'afterStages'})
            stage(v['commit'], ('not-run',)); require(case not in POSITIVE_CONTROLS, 'required positive prepare failed')
            after_stages(v['afterStages'], case); continue
        retained = {'main-retry-original', 'main-retry-changed-now', 'retry-changed-decision', 'preview-tamper-old-digest', 'preview-tamper-rehashed', 'digest-tamper'}
        require(prep == ('not-run' if case in retained else 'ok'), 'prepare status does not match case operation')
        object_fields(v, {'schema', 'caseID', 'kind', 'request', 'prepare', 'commit', 'controlRefs', 'requiredPositiveControl',
                          'nowBitsHex', 'before', 'originalBeforeBytes', 'plan', 'confirmation', 'logicalAndDateBitsUnchanged', 'afterStages'},
                      {'receiptBytes', 'record', 'exactReceiptAlreadyPresent'})
        require(isinstance(v['nowBitsHex'], str) and re.fullmatch('[0-9a-f]{16}', v['nowBitsHex']), 'now bit shape')
        state_projection(v['before']); state_raw(v['originalBeforeBytes'], case + '-before.json', 'actual-before-state-bytes', v['before'])
        p = v['plan']
        object_fields(p, {'planID', 'interactionID', 'preparedIntent', 'baseRevision', 'digest', 'reconstructedSHA256',
                          'matchesActualDigest', 'originalHasValidDigest', 'intentionallyCorrupted', 'reconstructedPayloadBytes'})
        intent_view(p['preparedIntent'])
        raw = reference(p['reconstructedPayloadBytes'], case + '-plan-payload.json', 'reconstructed-plan-payload-not-intercepted')
        material = json.loads(raw)
        object_fields(material, {'formatVersion', 'planID', 'baseRevision', 'interactionID', 'source', 'changes', 'plannedLabelIDs', 'notices', 'blockers'}, {'createdItemID', 'plannedCategoryID'})
        require(material['formatVersion'] == 1 and material['plannedLabelIDs'] == [] and isinstance(material['changes'], list) and len(material['changes']) == 1, 'plan payload scope')
        require(material['planID'] == p['planID'] and material['interactionID'] == p['interactionID'] and str(material['baseRevision']) == p['baseRevision'], 'plan payload identity')
        require(isinstance(p['digest'], str) and re.fullmatch('[0-9a-f]{64}', p['digest']), 'plan digest shape')
        match = digest(raw) == p['digest']
        require(p['reconstructedSHA256'] == digest(raw) and p['matchesActualDigest'] is match and p['originalHasValidDigest'] is match and p['intentionallyCorrupted'] is (case in TAMPERS), 'plan digest evidence mismatch')
        require(match is (case not in TAMPERS), 'original or intentional-tamper digest distinction')
        confirmation = v['confirmation']
        object_fields(confirmation, {'authority', 'decisionID', 'planID', 'interactionID', 'digest', 'originalAuthorizes'})
        require(confirmation['authority'] == 'bound-user' and confirmation['originalAuthorizes'] is True and all(confirmation[k] == p[k] for k in ('planID', 'interactionID', 'digest')), 'confirmation binding')
        committed = stage(v['commit'], ('ok', 'error')) == 'ok'
        if committed:
            require({'receiptBytes', 'record', 'exactReceiptAlreadyPresent'} <= set(v) and type(v['exactReceiptAlreadyPresent']) is bool, 'successful commit missing raw receipt/record')
            receipt = json.loads(reference(v['receiptBytes'], case + '-receipt.json', 'actual-Foundation-receipt-bytes'))
            object_fields(receipt, {'planID', 'revision', 'notices', 'changeRecordID', 'changeRecordIntegrityDigest'}, {'decisionID'})
            r = v['record']; object_fields(r, {'recordID', 'integrityDigest', 'committedAtBitsHex', 'matchesActualDigest', 'reconstructedPayloadBytes', 'originalRecordBytes'})
            rr = reference(r['reconstructedPayloadBytes'], case + '-record-payload.json', 'reconstructed-record-payload-not-intercepted')
            rm = json.loads(rr)
            object_fields(rm, {'formatVersion', 'id', 'planID', 'interactionID', 'source', 'changes', 'notices', 'committedAtBitPattern', 'revision', 'planDigest'}, {'decisionID'})
            original = json.loads(reference(r['originalRecordBytes'], case + '-record.json', 'actual-Foundation-record-Codable-bytes'))
            object_fields(original, {'id', 'planID', 'interactionID', 'source', 'changes', 'notices', 'committedAt', 'revision', 'planDigest', 'integrityDigest'}, {'decisionID'})
            require(digest(rr) == r['integrityDigest'] == original['integrityDigest'] == receipt['changeRecordIntegrityDigest'] and r['matchesActualDigest'] is True, 'record digest evidence')
            require(rm['formatVersion'] == 1 and type(rm['committedAtBitPattern']) is int and format(rm['committedAtBitPattern'], '016x') == r['committedAtBitsHex'], 'record date-bit evidence')
            require(rm['id'] == original['id'] == r['recordID'] == receipt['changeRecordID'] and rm['planID'] == original['planID'] == receipt['planID'] == p['planID'] and rm['interactionID'] == original['interactionID'] == p['interactionID'] and rm['planDigest'] == original['planDigest'] == p['digest'], 'receipt-record-plan binding')
            require(all(rm[k] == original[k] for k in ('source', 'changes', 'notices', 'revision')) and rm.get('decisionID') == original.get('decisionID') == receipt.get('decisionID') == confirmation['decisionID'] and receipt['revision'] == rm['revision'], 'receipt-record material fields')
        else:
            require(not ({'receiptBytes', 'record', 'exactReceiptAlreadyPresent'} & set(v)), 'failed commit has success-only evidence')
        positive_stages = after_stages(v['afterStages'], case)
        if case in POSITIVE_CONTROLS:
            require(committed and positive_stages, 'required positive control did not pass actual stages')
            passed.add(case)
    # Only the fixed graph DTO artifacts may be otherwise unreferenced.
    graph_names = {'main-graph.json'} | {'name-' + n['id'] + '-graph.json' for n in json.loads(frozen)['names']} | {'clock-' + c + '-graph.json' for c in ('negativeZero', 'zero', 'submillisecond')}
    require(set(refs) - used == graph_names, 'missing or orphan raw artifacts')
    for name in graph_names:
        g = json.loads(reference(refs[name], name, 'actual-synthetic-ordinary-graph'))
        object_fields(g, {'chains', 'definitions'}); require(len(g['chains']) == len(g['definitions']) == 2, 'graph fixture shape')
    require(initial_output_identity == {p.name: file_identity(p.lstat()) for p in [run, *run.iterdir()]}, 'output identity changed during verification')
    return {'captureComplete': True, 'observationCount': len(fixed), 'rawBytesVerified': True,
            'sourceBoundExecution': False, 'outputManifestSHA256': digest(manifest_raw),
            'inputSHA256': INPUT_SHA, 'sourceClosureSHA256': SOURCE_SHA,
            'semanticParityVerified': False, 'runtimeAdmission': False}


def frozen_identity(root):
    real_directory(root)
    result = {}
    for p in [root, *sorted(root.rglob('*'))]:
        st = p.lstat()
        require(not stat.S_ISLNK(st.st_mode), 'linked staged input')
        require(not (st.st_mode & 0o222), 'staged input is writable')
        if p.is_dir():
            real_directory(p)
        else:
            checked(p); require(st.st_nlink == 1, 'hardlinked staged input')
        result[str(p.relative_to(root))] = file_identity(st)
    return result


def stage_inputs(work, reviewed):
    # Copy exact checked bytes before any subprocess. Compiler never reads HERE.
    source, actual = verify(reviewed)
    require(actual == reviewed, 'reviewed manifest identity')
    manifest = json_read(HERE / 'TOOL-MANIFEST.json', 65536)
    paths = ['TOOL-MANIFEST.json'] + [r['path'] for r in manifest['files']] + ['upstream/' + r['path'] for r in source['files']]
    root = work / 'input'; root.mkdir(mode=0o700)
    for name in paths:
        raw = checked(HERE / name)
        target = root / name; target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        write(target, raw); target.chmod(0o444)
    verify(reviewed, root=root)
    for directory in sorted([root, *[p for p in root.rglob('*') if p.is_dir()]], key=lambda x: len(x.parts), reverse=True):
        directory.chmod(0o555)
    return root, frozen_identity(root)


def assert_frozen(root, identity, reviewed):
    require(frozen_identity(root) == identity, 'staged source identity changed')
    verify(reviewed, root=root)


def capture(args, source, reviewed):
    require(platform.system() == 'Darwin', 'capture needs reviewed macOS runner; no Linux emulation')
    require(args.work_root and args.developer_dir and args.reviewed_manifest_sha256, 'capture needs work root, developer dir and independent reviewed manifest hash')
    work = Path(args.work_root)
    require(work.is_absolute() and not work.exists(), 'work root must be new absolute path')
    real_directory(work.parent)
    require(HERE not in work.parents and work not in HERE.parents, 'work/source roots overlap')
    developer = Path(args.developer_dir)
    real_directory(developer)
    require(developer.parts[:2] == ('/', 'Applications'), 'developer directory must be installed official Xcode in /Applications')
    work.mkdir(mode=0o700)
    ledger = []
    try:
        for p in ('build', 'cache', 'tmp', 'home', 'run', 'provenance'):
            (work / p).mkdir(mode=0o700)
        frozen, identity = stage_inputs(work, reviewed)
        for name in ('SOURCE-CLOSURE.json', 'TOOL-MANIFEST.json', 'INPUT.json'):
            write(work / 'provenance' / name, checked(frozen / name))
        write(work / 'provenance' / 'LICENSE', checked(frozen / 'upstream' / 'LICENSE'))
        dump(work / 'provenance' / 'staged-identity.json', identity)
        for mode in ('compile', 'runtime'):
            write(work / (mode + '.sb'), sandbox(work, developer, mode == 'compile').encode())
        cp = ['/usr/bin/sandbox-exec', '-f', str(work / 'compile.sb')]
        rp = ['/usr/bin/sandbox-exec', '-f', str(work / 'runtime.sb')]
        env = {'PATH': '/usr/bin:/bin', 'HOME': str(work / 'home'), 'TMPDIR': str(work / 'tmp'), 'LANG': 'en_US.UTF-8',
               'LC_ALL': 'en_US.UTF-8', 'TZ': 'UTC', 'DEVELOPER_DIR': str(developer), 'CLANG_MODULE_CACHE_PATH': str(work / 'cache')}
        def run(argv, seconds, label, policy=cp):
            assert_frozen(frozen, identity, reviewed)
            return bounded_process(policy + argv, work, env, seconds, label, ledger)
        # Every preflight program is inside the same deny-network/private-root policy.
        xcode = run(['/usr/bin/xcodebuild', '-version'], 20, 'xcode-version')
        require(xcode.splitlines()[0] == 'Xcode 26.2', 'Xcode 26.2 not installed; do not silently substitute')
        swift = run(['/usr/bin/xcrun', '--no-cache', '--find', 'swiftc'], 20, 'swiftc-path')
        require(Path(swift).resolve().is_relative_to(developer), 'swiftc outside selected official Xcode')
        version = run([swift, '--version'], 20, 'swift-version')
        require('Swift version 6.' in version, 'Swift version mismatch')
        osversion = run(['/usr/bin/sw_vers'], 20, 'macos-version')
        sdk = run(['/usr/bin/xcrun', '--sdk', 'macosx', '--show-sdk-version'], 20, 'sdk-version')
        arch = platform.machine(); require(arch in ('arm64', 'x86_64'), 'architecture')
        dump(work / 'platform.json', {'xcode': xcode, 'swift': version, 'os': osversion, 'sdk': sdk, 'arch': arch,
              'imageOS': os.environ.get('ImageOS'), 'imageVersion': os.environ.get('ImageVersion'),
              'runnerOS': os.environ.get('RUNNER_OS'), 'officialHostedImageVerified': False,
              'reviewedToolManifestSHA256': reviewed, 'sourceClosureSHA256': SOURCE_SHA})
        flags = ['-swift-version', '6', '-target', arch + '-apple-macosx14.0', '-module-cache-path', str(work / 'cache'), '-package-name', 'Noonmark']
        build = work / 'build'; deadline = time.monotonic() + 900
        core = [str(frozen / 'upstream' / r['path']) for r in source['files'] if r['role'] == 'compiled-reference-only']
        run([swift, *flags, '-enable-testing', '-parse-as-library', '-module-name', 'NoonmarkCore',
             '-emit-library', '-emit-module', '-emit-module-path', str(build / 'NoonmarkCore.swiftmodule'),
             '-o', str(build / 'libNoonmarkCore.dylib'), *core], max(0.01, deadline - time.monotonic()), 'compile-core')
        run([swift, *flags, '-parse-as-library', '-I', str(build), '-L', str(build), '-lNoonmarkCore',
             '-Xlinker', '-rpath', '-Xlinker', str(build), str(frozen / 'OrdinaryCategoryOracle.swift'),
             '-o', str(build / 'ordinary-category-oracle')], max(0.01, deadline - time.monotonic()), 'compile-oracle')
        run([str(build / 'ordinary-category-oracle'), str(frozen / 'INPUT.json'), str(work / 'run')], 60, 'capture', policy=rp)
        result = verify_capture(work / 'run', checked(frozen / 'INPUT.json', 16384))
        assert_frozen(frozen, identity, reviewed)
        require(ledger and all(r['returncode'] == 0 and r['transportComplete'] and r['cleanupComplete'] for r in ledger), 'execution/cleanup witness incomplete')
        result.update(sourceBoundExecution=True, reviewedToolManifestSHA256=reviewed,
                      stagedIdentitySHA256=digest(checked(work / 'provenance' / 'staged-identity.json')),
                      commandReceiptSHA256s=[digest(checked(work / (label + '-command.json'))) for label in
                        ('xcode-version', 'swiftc-path', 'swift-version', 'macos-version', 'sdk-version', 'compile-core', 'compile-oracle', 'capture')])
        dump(work / 'capture-receipt.json', result)
        print(json.dumps(result, sort_keys=True))
    except BaseException as exc:
        dump(work / 'runner-failure.json', {'captureComplete': False, 'error': str(exc)[:2048],
             'commandsCompletedOrFailed': len(ledger), 'semanticParityVerified': False, 'runtimeAdmission': False})
        raise


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('action', nargs='?', choices=['verify', 'capture'], default='verify')
    p.add_argument('--reviewed-manifest-sha256')
    p.add_argument('--work-root'); p.add_argument('--developer-dir')
    args = p.parse_args()
    source, reviewed = verify(args.reviewed_manifest_sha256)
    if args.action == 'verify':
        print(json.dumps({'staticSourcesAndToolsVerified': True, 'toolManifestSHA256': reviewed,
                          'swiftExecuted': False, 'semanticParityVerified': False}, sort_keys=True))
    else:
        capture(args, source, reviewed)


if __name__ == '__main__':
    try:
        main()
    except Exception as exc:
        print('ordinary-category runner failure: ' + str(exc)[:2048], file=sys.stderr)
        sys.exit(1)
