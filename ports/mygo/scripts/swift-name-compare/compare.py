#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""严格离线 capture 验证及有限公开 API 对账；不能证明 capture 的远端真实性。"""
import argparse
import hashlib
import types
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
MODULE = ROOT / 'ports/mygo/domain-extensions'
ORACLE = ROOT / 'ports/mygo/scripts/swift-name-oracle'
PINS_SHA256 = 'ce1c96fc2770e1b5fdf1c84a87810a899f6f89564404c22d55e511316922bb4a'
MAX_CAPTURE = 16 * 1024 * 1024
MAX_INPUT = 2 * 1024 * 1024
VERSION = 'unicode-16.0.0.0-nfkc-casefold-whitespace-v1'
SNAPSHOT_MODULE = b'module github.com/ferrumlux/noonmark/ports/mygo/domain-extensions\n\ngo 1.27.3\n'
FIELDS = ('input', 'displayName', 'canonicalKey', 'displayNameOfDisplayName', 'canonicalKeyOfDisplayName')
ENTITY_IDS = {'ascii', 'empty', 'only-di', 'display-non-idempotent', 'space-mark', 'compat-diaeresis'}
ACTUAL = 'actual-upstream-swift-public-api'
SYNTHETIC = 'synthetic-test-fixture-not-swift'
UNCOMPARED = {
    'swiftCharacterBoundaries': 'Go canonical16 has no public boundary API; only contiguous scalar coverage is validated.',
    'swiftCharacterWhitespace': 'No public per-Character Go API; captured booleans are type-checked only.',
    'swiftCharacterCount': 'Only consistency with captured characters is validated, not equality to Go segmentation.',
    'swiftGraphemeUnicodeVersion': 'not_exposed_independently; ICU Unicode version is not evidence of Swift segmentation version.',
    'entityDecodeAndWire': 'No Go entity decoder or encoder invoked; decodeStatus, errors, encoded size and round-trip flags are validated structurally only.',
    'classificationWorkflows': 'No engine, history, ownership, catalog, runtime, migration or production integration is tested.',
    'captureAuthenticity': 'Hashes and caller-supplied run identity bind bytes but cannot authenticate GitHub provenance or prove execution; verify artifact origin separately.',
    'otherVersionsAndInputs': 'Results apply only to these fixed vectors and the recorded runtime; no general Swift or cross-platform equivalence claim.',
}

class Rejected(ValueError):
    pass

def require(ok, message):
    if not ok:
        raise Rejected(message)

def sha(data):
    return hashlib.sha256(data).hexdigest()

def encoded(value):
    return json.dumps(value, ensure_ascii=False, separators=(',', ':'), sort_keys=True).encode('utf-8')

def shape(value, keys, path):
    require(type(value) is dict and set(value) == set(keys), path + ': shape')

def text(value, path, limit=4096):
    require(type(value) is str, path + ': string required')
    try:
        data = value.encode('utf-8')
    except UnicodeError:
        raise Rejected(path + ': invalid Unicode scalar') from None
    require(len(data) <= limit, path + ': text size')
    return value

def integer(value, path, minimum=0, maximum=2**31-1):
    require(type(value) is int and minimum <= value <= maximum, path + ': integer range')

def regex(value, pattern, path):
    text(value, path)
    require(re.fullmatch(pattern, value) is not None, path + ': format')

def strict_json(data, label):
    def pairs(items):
        obj = {}
        for key, value in items:
            require(key not in obj, label + ': duplicate JSON member ' + key[:80])
            obj[key] = value
        return obj
    def bad_constant(value):
        raise Rejected(label + ': nonfinite JSON number')
    try:
        return json.loads(data.decode('utf-8'), object_pairs_hook=pairs, parse_constant=bad_constant)
    except (UnicodeError, json.JSONDecodeError, RecursionError, ValueError) as error:
        if isinstance(error, Rejected):
            raise
        raise Rejected(label + ': invalid JSON/UTF-8') from None

def read_file(path, limit):
    path = Path(path)
    require(path.is_absolute(), 'input path must be absolute')
    for part in [*reversed(path.parents), path]:
        info = part.lstat()
        require(not stat.S_ISLNK(info.st_mode), 'input path contains symlink')
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, 'rb') as stream:
        info = os.fstat(stream.fileno())
        require(stat.S_ISREG(info.st_mode) and info.st_nlink == 1, 'input must be single-link regular file')
        require(info.st_size <= limit, 'input size exceeds limit')
        data = stream.read(limit + 1)
    require(len(data) <= limit, 'input grew beyond limit')
    return data

def validate_package_inventory(names):
    # Reject unpinned compile inputs even when their names are platform-specific.
    build_extensions = {'.go', '.c', '.cc', '.cpp', '.cxx', '.m', '.h', '.hh', '.hpp', '.hxx', '.f', '.F', '.for', '.f90', '.s', '.S', '.swig', '.swigcxx', '.syso'}
    actual = {name for name in names if Path(name).suffix in build_extensions and not name.endswith('_test.go')}
    require(actual == {'canonical.go', 'normalize.go', 'grapheme.go', 'tables_generated.go'}, 'unpinned canonical16 compile input inventory')

def load_inputs():
    pins_bytes = read_file(HERE / 'pins.json', 16384)
    require(sha(pins_bytes) == PINS_SHA256, 'pinned source manifest changed')
    pins = strict_json(pins_bytes, 'pins')
    shape(pins, ('schemaVersion', 'files'), 'pins')
    require(type(pins['schemaVersion']) is int and pins['schemaVersion'] == 1, 'pins version')
    names = ['.github/workflows/swift-name-oracle.yml'] + [f'ports/mygo/scripts/swift-name-oracle/{n}' for n in ('run.py', 'NameOracle.swift', 'source-manifest.json', 'vectors.json')] + [f'ports/mygo/domain-extensions/classification/canonical16/{n}' for n in ('canonical.go', 'normalize.go', 'grapheme.go', 'tables_generated.go')]
    names += ['ports/mygo/domain-extensions/go.mod', 'ports/mygo/domain-extensions/classification/canonical16/scripts/guard_paths.py', 'ports/mygo/scripts/swift-name-compare/probe.go']
    shape(pins['files'], names, 'pinned files')
    validate_package_inventory([p.name for p in (MODULE / 'classification/canonical16').iterdir()])
    blobs = {}
    for name, expected in pins['files'].items():
        regex(expected, '[0-9a-f]{64}', 'pin')
        blob = read_file(ROOT / name, MAX_INPUT)
        require(sha(blob) == expected, 'source hash mismatch: ' + name)
        blobs[name] = blob
    manifest = strict_json(blobs['ports/mygo/scripts/swift-name-oracle/source-manifest.json'], 'manifest')
    vectors_bytes = blobs['ports/mygo/scripts/swift-name-oracle/vectors.json']
    vectors = strict_json(vectors_bytes, 'vectors')
    require(len(vectors['cases']) == 108 and len(vectors['pairs']) == 17, 'fixed input counts')
    return {'pins': pins['files'], 'pinsSHA256': sha(pins_bytes), 'manifest': manifest, 'vectors': vectors, 'vectorsBytes': vectors_bytes, 'verifiedBlobs': blobs, 'comparatorBytes': read_file(HERE / 'compare.py', 65536)}

def validate_text(value, path):
    shape(value, ('text', 'utf8Hex', 'unicodeScalars', 'characterCount', 'characters'), path)
    string = text(value['text'], path + '.text', MAX_INPUT)
    require(type(value['utf8Hex']) is str and value['utf8Hex'] == string.encode('utf-8').hex(), path + ': utf8Hex mismatch')
    scalars = value['unicodeScalars']
    require(type(scalars) is list and all(type(x) is int for x in scalars) and scalars == list(map(ord, string)), path + ': unicodeScalars mismatch')
    integer(value['characterCount'], path + '.characterCount', maximum=len(string))
    chars = value['characters']
    require(type(chars) is list and len(chars) == value['characterCount'], path + ': characters count')
    cursor = 0
    for i, char in enumerate(chars):
        shape(char, ('startScalar', 'endScalar', 'isWhitespace'), path + '.characters')
        integer(char['startScalar'], path + '.startScalar', maximum=len(string))
        integer(char['endScalar'], path + '.endScalar', maximum=len(string))
        require(char['startScalar'] == cursor and cursor < char['endScalar'], path + ': character range/coverage')
        require(type(char['isWhitespace']) is bool, path + ': whitespace boolean')
        cursor = char['endScalar']
    require(cursor == len(string), path + ': character coverage')

def validate_metadata(meta, inputs, identity, *, synthetic=False):
    keys = ('captureKind source sourceManifestSHA256 vectorsSHA256 harnessSHA256 runnerSHA256 workflowSHA256 workflowCommit runID runAttempt swiftVersion xcodeVersion osVersion architecture sdkVersion deploymentTarget runnerImageOS runnerImageVersion swiftGraphemeUnicodeVersion entityFixture compiledTargets productionDataAccess iCloudAccess keychainAccess aiNetworkAccess').split()
    shape(meta, keys, 'metadata')
    require(meta['captureKind'] == (SYNTHETIC if synthetic else ACTUAL), 'metadata captureKind')
    require(encoded(meta['source']) == encoded(inputs['manifest']), 'metadata source manifest')
    for field, filename in [('sourceManifestSHA256', 'source-manifest.json'), ('vectorsSHA256', 'vectors.json'), ('harnessSHA256', 'NameOracle.swift'), ('runnerSHA256', 'run.py')]:
        require(meta[field] == inputs['pins']['ports/mygo/scripts/swift-name-oracle/' + filename], 'metadata ' + field)
    require(meta['workflowSHA256'] == inputs['pins']['.github/workflows/swift-name-oracle.yml'], 'metadata workflowSHA256')
    for key, pattern in [('workflowCommit', '[0-9a-f]{40}'), ('runID', '[1-9][0-9]{0,19}'), ('runAttempt', '[1-9][0-9]{0,9}')]:
        regex(meta[key], pattern, 'metadata.' + key)
        require(meta[key] == identity[key], 'metadata run identity: ' + key)
    swift = text(meta['swiftVersion'], 'metadata.swiftVersion')
    require(re.search(r'^Apple Swift version [0-9]+\.[0-9]+[^\n]*', swift, re.M) is not None, 'metadata Swift compiler version missing')
    regex(meta['xcodeVersion'], r'Xcode 26\.2\nBuild version [A-Za-z0-9]+', 'metadata.xcodeVersion')
    regex(meta['osVersion'], r'ProductName:\s+macOS\nProductVersion:\s+15\.[0-9]+(?:\.[0-9]+)?\nBuildVersion:\s+[A-Za-z0-9]+', 'metadata.osVersion')
    require(meta['architecture'] in ('arm64', 'x86_64'), 'metadata architecture')
    regex(meta['sdkVersion'], r'[0-9]+\.[0-9]+(?:\.[0-9]+)?', 'metadata.sdkVersion')
    require(meta['deploymentTarget'] == meta['architecture'] + '-apple-macosx14.0', 'metadata deploymentTarget')
    require(meta['runnerImageOS'] == 'macos15', 'metadata runnerImageOS')
    regex(meta['runnerImageVersion'], r'[0-9]{8}\.[0-9]+(?:\.[0-9]+)?', 'metadata.runnerImageVersion')
    require(meta['swiftGraphemeUnicodeVersion'] == 'not_exposed_independently', 'metadata must not invent Swift grapheme version')
    require(encoded(meta['entityFixture']) == encoded({'id': '00000000-0000-4000-8000-000000000001', 'unixSeconds': 1700000000, 'colorHex': '#336699', 'historyUUID': 'generated_by_upstream_not_recorded'}), 'metadata entityFixture')
    require(meta['compiledTargets'] == ['NoonmarkCore', 'NameOracle'], 'metadata compiledTargets')
    for key in ('productionDataAccess', 'iCloudAccess', 'keychainAccess', 'aiNetworkAccess'):
        require(meta[key] is False, 'metadata forbidden access: ' + key)

def validate_capture(capture, inputs, identity, *, synthetic=False):
    shape(capture, ('schemaVersion', 'algorithmVersion', 'icuVersion', 'icuUnicodeVersion', 'cases', 'pairs', 'entityRoundTrips', 'metadata', 'oracle_capture', 'unicode16_comparison'), 'capture')
    require(type(capture['schemaVersion']) is int and capture['schemaVersion'] == 1, 'capture schemaVersion')
    validate_metadata(capture['metadata'], inputs, identity, synthetic=synthetic)
    require(capture['oracle_capture'] == 'succeeded', 'capture not succeeded')
    for field in ('icuVersion', 'icuUnicodeVersion'):
        regex(capture[field], r'[0-9]{1,3}(?:\.[0-9]{1,3}){3}', field)
        require(all(int(v) <= 255 for v in capture[field].split('.')), field + ': byte range')
    require(capture['algorithmVersion'] == 'unicode-' + capture['icuUnicodeVersion'] + '-nfkc-casefold-whitespace-v1', 'algorithm version does not match recorded ICU Unicode')
    supported = capture['icuUnicodeVersion'] == '16.0.0.0'
    require(capture['unicode16_comparison'] == ('not_performed' if supported else 'unsupported'), 'capture comparison status must not claim a comparison')
    vectors = inputs['vectors']
    require(type(capture['cases']) is list and len(capture['cases']) == 108, 'capture case count')
    index = {}
    for row, vector in zip(capture['cases'], vectors['cases']):
        shape(row, ('id', *FIELDS), 'case')
        require(row['id'] == vector['id'], 'case identity/order/missing/duplicate')
        for field in FIELDS:
            validate_text(row[field], 'cases.' + vector['id'] + '.' + field)
        require(row['input']['text'] == vector['input'], 'original input changed: ' + vector['id'])
        index[row['id']] = row
    require(type(capture['pairs']) is list and len(capture['pairs']) == 17, 'pair count')
    for row, pair in zip(capture['pairs'], vectors['pairs']):
        shape(row, ('id', 'left', 'right', 'input', 'displayName', 'canonicalKey'), 'pair')
        require(all(row[k] == pair[k] for k in ('id', 'left', 'right')), 'pair identity/order/missing/duplicate')
        for field in ('input', 'displayName', 'canonicalKey'):
            value = row[field]
            shape(value, ('swiftEqual', 'utf8Equal'), 'pair equality')
            require(all(type(v) is bool for v in value.values()), 'pair equality type')
            require(value['utf8Equal'] == (index[pair['left']][field]['utf8Hex'] == index[pair['right']][field]['utf8Hex']), 'pair utf8Equal inconsistency')
            require(not value['utf8Equal'] or value['swiftEqual'], 'pair equal bytes cannot be Swift-unequal')
    expected = [(v['id'], kind) for v in vectors['cases'] if v['id'] in ENTITY_IDS for kind in ('TaskCategory', 'TaskLabel')]
    rows = capture['entityRoundTrips']
    require(type(rows) is list and len(rows) == 12, 'entity count')
    base = {'caseID', 'entity', 'name', 'canonicalKey', 'canonicalKeyVersion', 'encodedByteCount', 'decodeStatus'}
    for row, (case_id, entity) in zip(rows, expected):
        require(type(row) is dict and base <= set(row), 'entity missing required field')
        require(row['caseID'] == case_id and row['entity'] == entity, 'entity identity/order/missing/duplicate')
        validate_text(row['name'], 'entity.' + case_id + '.' + entity + '.name')
        validate_text(row['canonicalKey'], 'entity.' + case_id + '.' + entity + '.canonicalKey')
        require(row['canonicalKeyVersion'] == capture['algorithmVersion'], 'entity version mismatch')
        integer(row['encodedByteCount'], 'entity encodedByteCount', 1, 65535)
        if row['decodeStatus'] == 'accepted':
            shape(row, base | {'roundTripEqual', 'nameUTF8Equal', 'keyUTF8Equal'}, 'accepted entity')
            require(all(type(row[k]) is bool for k in ('roundTripEqual', 'nameUTF8Equal', 'keyUTF8Equal')), 'entity equality type')
        else:
            require(row['decodeStatus'] == 'rejected', 'entity decodeStatus')
            require(set(row) in (base | {'decodeErrorKind'}, base | {'decodeErrorKind', 'decodeErrorDescription'}), 'rejected entity shape')
            require(row['decodeErrorKind'] in ('dataCorrupted', 'keyNotFound', 'typeMismatch', 'valueNotFound', 'other'), 'entity decodeErrorKind')
            if 'decodeErrorDescription' in row:
                text(row['decodeErrorDescription'], 'entity decodeErrorDescription', 4096)
                # Swift prefix(256) counts Characters, not Unicode scalars. Preserve that distinction.
                require(len(row['decodeErrorDescription']) <= 4096, 'entity description resource limit')
    return supported

def validate_summary(summary, capture, capture_bytes):
    shape(summary, ('schemaVersion', 'oracle_capture', 'unicode16_comparison', 'algorithmVersion', 'icuVersion', 'icuUnicodeVersion', 'caseCount', 'pairCount', 'entityRoundTripCount', 'captureSHA256', 'stage'), 'summary')
    for key, expected in [('schemaVersion', 1), ('caseCount', 108), ('pairCount', 17), ('entityRoundTripCount', 12)]:
        require(type(summary[key]) is int and summary[key] == expected, 'summary ' + key)
    for key in ('oracle_capture', 'unicode16_comparison', 'algorithmVersion', 'icuVersion', 'icuUnicodeVersion'):
        require(summary[key] == capture[key], 'summary ' + key)
    require(summary['stage'] == 'complete' and summary['captureSHA256'] == sha(capture_bytes), 'summary completion/capture hash')

def compare(capture, probe):
    shape(probe, ('schemaVersion', 'kind', 'goVersion', 'unicodeVersion', 'algorithmVersion', 'cases', 'pairs', 'entityFields'), 'Go probe')
    require(type(probe['schemaVersion']) is int and probe['schemaVersion'] == 1 and probe['kind'] == 'go-canonical16-public-api-probe' and probe['goVersion'] == 'go1.27.3' and probe['unicodeVersion'] == '16.0.0', 'Go probe identity/version')
    require(all(type(probe[key]) is list for key in ('cases', 'pairs', 'entityFields')), 'Go probe array types')
    require(len(probe['cases']) == 108 and len(probe['pairs']) == 17 and len(probe['entityFields']) == 12, 'Go probe counts')
    text(probe['algorithmVersion'], 'Go algorithmVersion')
    checks = []
    def check(path, swift, go):
        a, b = encoded(swift), encoded(go)
        checks.append({'path': path, 'equal': a == b, 'captureValueSHA256': sha(a), 'goValueSHA256': sha(b)})
    check('algorithmVersion', capture['algorithmVersion'], probe['algorithmVersion'])
    def check_text(path, a, b):
        shape(b, ('text', 'utf8Hex', 'unicodeScalars'), 'Go text')
        string = text(b['text'], 'Go text', MAX_INPUT)
        require(type(b['utf8Hex']) is str and b['utf8Hex'] == string.encode('utf-8').hex(), 'Go utf8Hex inconsistency')
        require(type(b['unicodeScalars']) is list and all(type(value) is int for value in b['unicodeScalars']) and b['unicodeScalars'] == list(map(ord, string)), 'Go unicodeScalars type/value inconsistency')
        for field in ('text', 'utf8Hex', 'unicodeScalars'):
            check(path + '.' + field, a[field], b[field])
    for a, b in zip(capture['cases'], probe['cases']):
        shape(b, ('id', *FIELDS), 'Go case')
        require(a['id'] == b['id'], 'Go case identity')
        for field in FIELDS:
            check_text('cases.' + a['id'] + '.' + field, a[field], b[field])
    probe_index = {row['id']: row for row in probe['cases']}
    for a, b in zip(capture['pairs'], probe['pairs']):
        shape(b, ('id', 'left', 'right', 'input', 'displayName', 'canonicalKey'), 'Go pair')
        require(all(a[k] == b[k] for k in ('id', 'left', 'right')), 'Go pair identity')
        for field in ('input', 'displayName', 'canonicalKey'):
            shape(b[field], ('swiftEqual', 'utf8Equal'), 'Go equality')
            require(all(type(v) is bool for v in b[field].values()), 'Go equality type')
            byte_equal = probe_index[b['left']][field]['utf8Hex'] == probe_index[b['right']][field]['utf8Hex']
            require(b[field]['utf8Equal'] == byte_equal, 'Go pair utf8Equal inconsistency')
            require(not byte_equal or b[field]['swiftEqual'], 'Go equal bytes cannot be canonically unequal')
            for key in ('swiftEqual', 'utf8Equal'):
                check('pairs.' + a['id'] + '.' + field + '.' + key, a[field][key], b[field][key])
    for a, b in zip(capture['entityRoundTrips'], probe['entityFields']):
        shape(b, ('caseID', 'entity', 'name', 'canonicalKey', 'canonicalKeyVersion'), 'Go entity fields')
        require(a['caseID'] == b['caseID'] and a['entity'] == b['entity'], 'Go entity identity')
        prefix = 'entityFields.' + a['caseID'] + '.' + a['entity']
        for field in ('name', 'canonicalKey'):
            check_text(prefix + '.' + field, a[field], b[field])
        text(b['canonicalKeyVersion'], 'Go entity canonicalKeyVersion')
        check(prefix + '.canonicalKeyVersion', a['canonicalKeyVersion'], b['canonicalKeyVersion'])
    mismatches = sum(not row['equal'] for row in checks)
    synthetic = capture['metadata']['captureKind'] == SYNTHETIC
    return {'schemaVersion': 1, 'status': ('synthetic_fixture_' if synthetic else 'supported_fields_') + ('mismatch' if mismatches else 'match'), 'evidenceKind': capture['metadata']['captureKind'], 'swiftVerified': False, 'coverage': {'caseCount': 108, 'pairCount': 17, 'entityFieldRecordCount': 12, 'textRecordCount': 564, 'textRepresentationChecks': 1692, 'pairBooleanChecks': 102, 'entityVersionChecks': 12, 'algorithmVersionChecks': 1, 'totalChecks': len(checks), 'mismatchCount': mismatches}, 'notCompared': UNCOMPARED, 'checks': checks}

def prepare_run(build_root, inputs=None):
    inputs = load_inputs() if inputs is None else inputs
    # Execute the same verified bytes, never reopen mutable helper source.
    guard = types.ModuleType('canonical16_paths')
    source = inputs['verifiedBlobs']['ports/mygo/domain-extensions/classification/canonical16/scripts/guard_paths.py']
    exec(compile(source, '<verified-canonical16-path-guard>', 'exec'), guard.__dict__)
    base = guard.checked_absolute(build_root, 'build root')
    env = {'NOONMARK_MYGO_BUILD_ROOT': str(base / 'runs'), 'GOPATH': str(base / 'gopath'), 'GOCACHE': str(base / 'gocache')}
    protected = [ROOT]
    for sibling in ROOT.parent.iterdir():
        if sibling.is_dir() and (guard.is_repository(sibling) or (sibling / 'ports/mygo/AGENTS.md').is_file()):
            protected.append(sibling.resolve())
    for source in protected:
        require(not guard.overlaps(base, source), 'build root overlaps source')
    guard.reject_unsafe_entries(base)
    guard.validate(MODULE, env)
    base.mkdir(mode=0o700, parents=True, exist_ok=True)
    for path in env.values():
        Path(path).mkdir(mode=0o700, parents=True, exist_ok=True)
    run = Path(tempfile.mkdtemp(prefix='run-', dir=env['NOONMARK_MYGO_BUILD_ROOT']))
    (run / 'tmp').mkdir(mode=0o700)
    return run, env

def freeze_build_sources(run, inputs):
    """Snapshot verified bytes, not source paths. This is not a same-UID sandbox."""
    files = {'go.mod': SNAPSHOT_MODULE}
    origins = {'go.mod': 'generated minimal offline module; original module remains independently pinned'}
    prefix = 'ports/mygo/domain-extensions/'
    for name in ('canonical.go', 'normalize.go', 'grapheme.go', 'tables_generated.go'):
        relative = 'classification/canonical16/' + name
        files[relative] = inputs['verifiedBlobs'][prefix + relative]
        origins[relative] = prefix + relative
    probe_source = 'ports/mygo/scripts/swift-name-compare/probe.go'
    files['cmd/name-probe/main.go'] = inputs['verifiedBlobs'][probe_source]
    origins['cmd/name-probe/main.go'] = probe_source
    snapshot = run / 'source-snapshot'
    snapshot.mkdir(mode=0o700)
    for name, data in files.items():
        target = snapshot / name
        target.parent.mkdir(parents=True, exist_ok=True)
        with target.open('xb') as handle:
            handle.write(data)
        target.chmod(0o400)
    manifest = {'schemaVersion': 1, 'kind': 'verified-bytes-offline-build-snapshot',
                'files': {name: {'sha256': sha(data), 'bytes': len(data), 'origin': origins[name]} for name, data in sorted(files.items())},
                'scope': 'All application/probe/module compile inputs. Standard-library/compiler inputs belong to the recorded Go toolchain, not this source snapshot.',
                'boundary': 'Prevents normal source edits from changing build inputs after validation; not a security boundary against a malicious same-UID process.'}
    manifest_bytes = encoded(manifest) + b'\n'
    with (run / 'source-snapshot-manifest.json').open('xb') as handle:
        handle.write(manifest_bytes)
    (run / 'source-snapshot-manifest.json').chmod(0o400)
    for directory in sorted((p for p in snapshot.rglob('*') if p.is_dir()), key=lambda p: len(p.parts), reverse=True):
        directory.chmod(0o500)
    snapshot.chmod(0o500)
    verify_build_snapshot(snapshot, manifest)
    return snapshot, manifest, sha(manifest_bytes)

def verify_build_snapshot(snapshot, manifest):
    found = set()
    for parent, directories, files in os.walk(snapshot, followlinks=False):
        for name in directories + files:
            path = Path(parent) / name
            info = path.lstat()
            require(not stat.S_ISLNK(info.st_mode), 'snapshot symlink')
            require(stat.S_ISDIR(info.st_mode) or (stat.S_ISREG(info.st_mode) and info.st_nlink == 1), 'snapshot special file or hardlink')
            require(info.st_mode & 0o222 == 0, 'snapshot unexpectedly writable')
        for name in files:
            path = Path(parent) / name
            relative = str(path.relative_to(snapshot))
            found.add(relative)
            require(relative in manifest['files'], 'snapshot extra file')
            data = read_file(path, MAX_INPUT)
            require(sha(data) == manifest['files'][relative]['sha256'] and len(data) == manifest['files'][relative]['bytes'], 'snapshot bytes changed')
    require(found == set(manifest['files']), 'snapshot missing input')

def build_probe(go, run, paths, inputs):
    require(Path(go).is_absolute() and Path(go).is_file(), 'absolute Go executable required')
    snapshot, snapshot_manifest, snapshot_hash = freeze_build_sources(run, inputs)
    env = {'PATH': '/usr/bin:/bin', 'HOME': str(run), 'TMPDIR': str(run / 'tmp'), 'GOTMPDIR': str(run / 'tmp'), 'GOPATH': paths['GOPATH'], 'GOCACHE': paths['GOCACHE'], 'GOMODCACHE': str(Path(paths['GOPATH']) / 'pkg/mod'), 'GOPROXY': 'off', 'GOSUMDB': 'off', 'GOTOOLCHAIN': 'local', 'GOWORK': 'off', 'GOENV': 'off', 'CGO_ENABLED': '0', 'GOFLAGS': '-mod=readonly', 'LANG': 'C.UTF-8'}
    def command(args, name, stdin=None, limit=MAX_CAPTURE, timeout=180):
        with (run / (name + '.stderr')).open('xb') as errors, (run / (name + '.stdout')).open('xb') as output:
            completed = subprocess.run(args, cwd=snapshot, env=env, input=stdin, stdout=output, stderr=errors, timeout=timeout)
        require(completed.returncode == 0, name + ' failed; inspect external run log')
        return read_file(run / (name + '.stdout'), limit)
    version = command([go, 'version'], 'go-version', limit=4096).decode().strip()
    require(re.fullmatch(r'go version go1\.27\.3 [a-z0-9]+/[a-z0-9]+', version) is not None, 'Go 1.27.3 required')
    verify_build_snapshot(snapshot, snapshot_manifest)
    command([go, 'build', '-trimpath', '-buildvcs=false', '-o', str(run / 'probe'), './cmd/name-probe'], 'go-build')
    verify_build_snapshot(snapshot, snapshot_manifest)
    result_bytes = command([str(run / 'probe')], 'go-probe', inputs['vectorsBytes'])
    verify_build_snapshot(snapshot, snapshot_manifest)
    return strict_json(result_bytes, 'Go probe'), {'goVersion': version,
        'goExecutableSHA256': sha(read_file(Path(go), 64 * 1024 * 1024)),
        'probeSourceSHA256': sha(inputs['verifiedBlobs']['ports/mygo/scripts/swift-name-compare/probe.go']),
        'probeBinarySHA256': sha(read_file(run / 'probe', 32 * 1024 * 1024)), 'probeOutputSHA256': sha(result_bytes),
        'buildSourceSnapshotSHA256': snapshot_hash, 'buildSourceSnapshot': snapshot_manifest,
        'buildFlags': ['-trimpath', '-buildvcs=false', '-mod=readonly', 'CGO_ENABLED=0', 'GOWORK=off', 'GOENV=off', 'GOPROXY=off', 'GOSUMDB=off', 'GOTOOLCHAIN=local']}

def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('capture', 'summary', 'capture-sha256', 'workflow-commit', 'run-id', 'run-attempt', 'go', 'build-root'):
        parser.add_argument('--' + name, required=True)
    args = parser.parse_args(argv)
    inputs = load_inputs()
    regex(args.capture_sha256, '[0-9a-f]{64}', 'expected capture SHA256')
    capture_bytes = read_file(Path(args.capture), MAX_CAPTURE)
    require(sha(capture_bytes) == args.capture_sha256, 'caller expected capture hash mismatch')
    summary_bytes = read_file(Path(args.summary), 16384)
    capture = strict_json(capture_bytes, 'capture')
    summary = strict_json(summary_bytes, 'summary')
    identity = {'workflowCommit': args.workflow_commit, 'runID': args.run_id, 'runAttempt': args.run_attempt}
    supported = validate_capture(capture, inputs, identity)
    validate_summary(summary, capture, capture_bytes)
    provenance = {'captureSHA256': sha(capture_bytes), 'summarySHA256': sha(summary_bytes), 'sourceFilesSHA256': inputs['pins'], 'pinsSHA256': inputs['pinsSHA256'], 'comparisonSourceSHA256': sha(inputs['comparatorBytes']), 'captureIdentity': identity, 'swiftRuntime': {k: capture['metadata'][k] for k in ('swiftVersion', 'xcodeVersion', 'osVersion', 'sdkVersion', 'architecture', 'deploymentTarget', 'runnerImageOS', 'runnerImageVersion', 'swiftGraphemeUnicodeVersion')}, 'icuVersion': capture['icuVersion'], 'icuUnicodeVersion': capture['icuUnicodeVersion']}
    if not supported:
        print(json.dumps({'schemaVersion': 1, 'status': 'unsupported', 'reason': 'Only ICU Unicode 16.0.0.0 is supported; no Go comparison executed.', 'totalChecks': 0, 'swiftVerified': False, 'provenance': provenance, 'notCompared': UNCOMPARED}, ensure_ascii=False, sort_keys=True))
        return 3
    run, paths = prepare_run(args.build_root, inputs)
    for name, data in [('capture.json', capture_bytes), ('summary.json', summary_bytes), ('vectors.json', inputs['vectorsBytes'])]:
        with (run / name).open('xb') as output:
            output.write(data)
    probe, probe_hashes = build_probe(args.go, run, paths, inputs)
    report = compare(capture, probe)
    report['provenance'] = {**provenance, **probe_hashes}
    report['replay'] = {'command': [sys.executable, '-B', str(HERE / 'compare.py'), '--capture', str(run / 'capture.json'), '--summary', str(run / 'summary.json'), '--capture-sha256', args.capture_sha256, '--workflow-commit', args.workflow_commit, '--run-id', args.run_id, '--run-attempt', args.run_attempt, '--go', args.go, '--build-root', args.build_root], 'requires': 'Unchanged pinned repository files and Go 1.27.3. Re-execution creates a fresh run directory.'}
    with (run / 'comparison.json').open('xb') as output:
        output.write(encoded(report) + b'\n')
    print(json.dumps({'status': report['status'], 'coverage': report['coverage'], 'report': str(run / 'comparison.json'), 'reportSHA256': sha((run / 'comparison.json').read_bytes()), 'swiftVerified': False}, sort_keys=True))
    return 1 if report['coverage']['mismatchCount'] else 0

if __name__ == '__main__':
    try:
        sys.exit(main())
    except (Rejected, OSError, ValueError, KeyError, TypeError, subprocess.TimeoutExpired, RecursionError) as error:
        print(json.dumps({'schemaVersion': 1, 'status': 'rejected', 'reason': str(error), 'swiftVerified': False, 'comparisonCompleted': False}), file=sys.stderr)
        sys.exit(2)
