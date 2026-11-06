#!/usr/bin/env python3
"""Closed lifecycle evidence export and independent required gate.

Collection preserves bounded raw failures, including malformed reports; only the
separate gate can pass. No session, HOME, cookies, binaries or screen capture is
read or exported. This checks trusted workflow evidence, not hostile-user proof.
"""
import argparse
import hashlib
import json
import math
from pathlib import Path
import re
import run as driver

smoke = driver.smoke
require = smoke.require
MAX_LOG = driver.MAX_LOG
MAX_REPORT = driver.MAX_REPORT
FILES = {'manifest.json': 4096, 'result.json': MAX_REPORT}
for n in (1, 2):
    for name in (f'stage{n}.json', f'stage{n}-driver.json'):
        FILES[name] = MAX_REPORT
    for stream in ('stdout', 'stderr'):
        FILES[f'stage{n}.{stream}.txt'] = MAX_LOG
        for helper in ('xvfb', 'dbus'):
            FILES[f'infra-stage{n}/{helper}.{stream}.txt'] = MAX_LOG
IDENTITY_KEYS = {'source', 'run_id', 'build_attempt', 'consumer_attempt', 'target', 'artifact_id'}


def identity(expected):
    require(set(expected) == IDENTITY_KEYS, 'expected identity fields')
    smoke.provenance(expected['source'], expected['run_id'], expected['build_attempt'])
    for key in ('consumer_attempt', 'artifact_id'):
        require(type(expected[key]) is str and re.fullmatch('[1-9][0-9]*', expected[key]), 'invalid ' + key)
    require(expected['target'] in smoke.TARGETS, 'invalid target')
    require(int(expected['consumer_attempt']) >= int(expected['build_attempt']), 'consumer precedes build')


def checked_path(root, name):
    path = root / name
    for parent in path.parents:
        smoke.directory(parent)
        if parent == root:
            break
    smoke.plain(path)
    return path


def raw(root, name, limit):
    path = checked_path(root, name)
    require(path.stat().st_size <= limit, 'file over budget: ' + name)
    with path.open('rb') as stream:
        data = stream.read(limit + 1)
    require(len(data) <= limit, 'file grew over budget: ' + name)
    return data


def strict(data):
    return json.loads(data, object_pairs_hook=driver.unique,
                      parse_constant=lambda _: (_ for _ in ()).throw(ValueError('nonfinite JSON')))


def collect(work_parent, output, expected, stages, artifact):
    identity(expected)
    driver.clean_directory(output.parent)
    output.mkdir(mode=0o700)
    record = {'version': 1, 'expected': expected, 'workflow_stages': stages,
              'exported': {}, 'missing': [], 'rejected': [], 'truncated': []}
    try:
        verified_binary(artifact, expected)
    except (ValueError, OSError) as error:
        record['artifact_failure'] = str(error)
    try:
        driver.clean_directory(work_parent)
        entries = list(work_parent.iterdir())
        require(len(entries) == 1 and entries[0].name.startswith('native-window-'), 'exactly one owned work root required')
        work = driver.clean_directory(entries[0])
        record['work_root'] = str(work)
        # Inspect names at the root only; never traverse private session/HOME.
        allowed = {Path(n).parts[0] for n in FILES} | {'session'}
        require(all(p.name in allowed for p in work.iterdir()), 'unexpected work-root entry')
    except (ValueError, OSError) as error:
        record['source_unavailable'] = str(error)
    else:
        for name, limit in FILES.items():
            try:
                path = checked_path(work, name)
                size = path.stat().st_size
                with path.open('rb') as stream:
                    data = stream.read(limit + 1)
                if size > limit or len(data) > limit:
                    record['truncated'].append(name)
                    data = data[:limit]  # exact raw prefix, never a replacement success JSON
                dest = output / name
                dest.parent.mkdir(exist_ok=True)
                dest.write_bytes(data)
                record['exported'][name] = hashlib.sha256(data).hexdigest()
            except FileNotFoundError:
                record['missing'].append(name)
            except (ValueError, OSError) as error:
                record['rejected'].append({'name': name, 'reason': str(error)})
    (output / 'collection.json').write_text(json.dumps(record, indent=2) + '\n')
    return record


def finite(value, limit, label):
    require(type(value) in (int, float) and math.isfinite(value) and 0 <= value < limit, label)


def verified_binary(artifact, expected):
    driver.clean_directory(artifact)
    return smoke.verify(artifact, expected['target'], expected['source'], expected['run_id'], expected['build_attempt'])


def gate(output, expected, artifact):
    identity(expected)
    binary_hash = smoke.digest(verified_binary(artifact, expected))
    driver.clean_directory(output)
    collection = strict(raw(output, 'collection.json', MAX_REPORT))
    require(set(collection) == {'version', 'expected', 'workflow_stages', 'exported', 'missing', 'rejected', 'truncated', 'work_root'}, 'collection fields/source unavailable')
    require(collection['version'] == 1 and collection['expected'] == expected, 'collection identity mismatch')
    require(not collection['rejected'] and not collection['truncated'], 'rejected/truncated evidence')
    required = {name for name in FILES if not name.startswith('infra-') or expected['target'].startswith('linux/')}
    exported = collection['exported']
    require(type(exported) is dict and set(exported) == required, 'missing/extra evidence')
    require(set(collection['missing']) == set(FILES) - required, 'missing evidence accounting')
    # Exact finite directory inspection, never recursive glob or traversal.
    allowed_top = {Path(n).parts[0] for n in required} | {'collection.json'}
    require({p.name for p in output.iterdir()} == allowed_top, 'extra output entry')
    for directory in ('infra-stage1', 'infra-stage2'):
        if directory in allowed_top:
            smoke.directory(output / directory)
            require({p.name for p in (output / directory).iterdir()} == {Path(n).name for n in required if n.startswith(directory + '/')}, 'extra helper evidence')
    data = {}
    for name in required:
        data[name] = raw(output, name, FILES[name])
        require(hashlib.sha256(data[name]).hexdigest() == exported[name], 'evidence digest mismatch')
    stages = collection['workflow_stages']
    for key in ('provenance', 'download', 'smoke') + (('libraries',) if expected['target'].startswith('linux/') else ()):
        require(stages.get(key) == 'success', 'workflow step failed: ' + key)
    result = strict(data['result.json'])
    result_keys = {'version', 'scope', 'passed', 'display_pixels_verified', 'ime_tested', 'target', 'source', 'run_id', 'run_attempt', 'build_attempt', 'consumer_attempt', 'artifact_id', 'budget_kind', 'work_root', 'binary_sha256', 'duration_seconds'}
    require(type(result) is dict and set(result) == result_keys, 'result fields/failure')
    require(all(result[k] == v for k, v in expected.items()), 'result identity mismatch')
    require(result['run_attempt'] == expected['build_attempt'], 'builder attempt mismatch')
    require(result['version'] == 1 and result['scope'] == 'lifecycle-only' and result['passed'] is True and result['display_pixels_verified'] is False and result['ime_tested'] is False and result['budget_kind'] == 'software-watchdog', 'runtime not passed')
    finite(result['duration_seconds'], 120, 'runtime duration')
    require(result['work_root'] == collection['work_root'], 'work root mismatch')
    manifest = strict(data['manifest.json'])
    provenance = smoke.provenance(expected['source'], expected['run_id'], expected['build_attempt'])
    require(type(manifest) is dict and set(manifest) == {'schema', 'target', 'filename', 'size', 'sha256', *provenance}, 'manifest fields')
    require(all(manifest[k] == v for k, v in provenance.items()), 'manifest provenance')
    require(type(manifest['schema']) is int and manifest['schema'] == 1 and manifest['target'] == expected['target'] and manifest['filename'] == smoke.filename(expected['target']), 'manifest target')
    require(type(manifest['size']) is int and 0 < manifest['size'] <= smoke.MAX_BINARY, 'manifest size')
    require(type(manifest['sha256']) is str and re.fullmatch('[0-9a-f]{64}', manifest['sha256']) and result['binary_sha256'] == manifest['sha256'] == binary_hash, 'binary hash mismatch')
    records = []
    for n, label in ((1, 'create'), (2, 'open')):
        report = driver.validate_report(strict(data[f'stage{n}.json']), label)
        record = strict(data[f'stage{n}-driver.json'])
        keys = {'stage', 'passed', 'marker_count', 'processes', 'budget_kind', 'marker_observed_seconds', 'exit', 'today', 'report_sha256', 'session_path', 'session_identity', 'session_files', 'duration_seconds'}
        require(type(record) is dict and set(record) == keys, 'stage fields/failure')
        require(record['stage'] == label and record['passed'] is True and type(record['exit']) is int and record['exit'] == 0 and type(record['marker_count']) is int and record['marker_count'] == 1 and record['budget_kind'] == 'software-watchdog', 'stage failed')
        finite(record['marker_observed_seconds'], 45, 'ready duration')
        finite(record['duration_seconds'], 60, 'stage duration')
        require(record['marker_observed_seconds'] <= record['duration_seconds'], 'marker after completion')
        require(record['today'] == report['today'] and record['report_sha256'] == hashlib.sha256(data[f'stage{n}.json']).hexdigest(), 'report identity/hash mismatch')
        require(record['session_identity'] == smoke.IDENTITY.decode().rstrip('\n') and record['session_path'] == str(Path(result['work_root']) / 'session'), 'session identity/path mismatch')
        require(type(record['session_files']) is dict and 0 < len(record['session_files']) <= 128, 'session snapshot missing')
        files = record['session_files']
        require('SESSION_IDENTITY' in files and any(k.endswith('/COMMITTED') for k in files) and any(k.endswith('/state.json') for k in files), 'session snapshot incomplete')
        require(files['SESSION_IDENTITY'] == hashlib.sha256(smoke.IDENTITY).hexdigest(), 'session identity digest')
        for name, value in files.items():
            require(type(name) is str and name and not name.startswith('/') and '..' not in name.split('/') and '\\' not in name, 'session snapshot name')
            require(value == 'directory' if name.endswith('/') else type(value) is str and re.fullmatch('[0-9a-f]{64}', value), 'session snapshot digest')
        marker = driver.Marker()
        marker.feed(data[f'stage{n}.stdout.txt'], 0)
        marker.finish()
        processes = record['processes']
        require(type(processes) is list and len(processes) == (3 if expected['target'].startswith('linux/') else 1), 'owned process count')
        pids = set()
        for process in processes:
            require(type(process) is dict and type(process.get('pid')) is int and process['pid'] > 0 and process['pid'] not in pids and process.get('reaped') is True and not process.get('cleanup_incomplete', False), 'process not reaped')
            pids.add(process['pid'])
            require(process.get('job_empty' if expected['target'].startswith('windows/') else 'group_empty') is True, 'process tree not empty')
            require(type(process.get('exit')) is int, 'missing process exit')
            if expected['target'].startswith('windows/'):
                require(set(process) == {'pid', 'assigned_to_job', 'resumed', 'reaped', 'job_empty', 'exit', 'cleanup_incomplete'} and process['assigned_to_job'] is True and process['resumed'] is True and process['cleanup_incomplete'] is False, 'Windows ownership incomplete')
            else:
                require(set(process) == {'pid', 'pgid', 'reaped', 'group_empty', 'exit'} and type(process['pgid']) is int and process['pgid'] == process['pid'], 'POSIX ownership incomplete')
        require(processes[-1]['exit'] == 0, 'app exit nonzero')
        records.append(record)
    require(sum(r['duration_seconds'] for r in records) <= result['duration_seconds'], 'stage durations exceed total')
    for key in ('session_path', 'session_identity', 'session_files', 'today'):
        require(records[0][key] == records[1][key], 'reopen mismatch: ' + key)


def parser():
    p = argparse.ArgumentParser(description=__doc__, allow_abbrev=False)
    p.add_argument('--work-parent', type=driver.absolute_argument)
    p.add_argument('--output', type=driver.absolute_argument, required=True)
    p.add_argument('--gate', action='store_true')
    p.add_argument('--artifact', type=driver.absolute_argument, required=True)
    for name in sorted(IDENTITY_KEYS):
        p.add_argument('--' + name.replace('_', '-'), required=True)
    p.add_argument('--stages', default='{}')
    return p


if __name__ == '__main__':
    args = parser().parse_args()
    expected = {k: getattr(args, k) for k in IDENTITY_KEYS}
    if args.gate:
        stages = strict(args.stages)
        require(all(stages.get(k) == 'success' for k in ('provenance', 'download', 'smoke', 'collect', 'upload')), 'current workflow stage failed')
        gate(args.output, expected, args.artifact)
    else:
        require(args.work_parent is not None, '--work-parent required')
        collect(args.work_parent, args.output, expected, strict(args.stages), args.artifact)
