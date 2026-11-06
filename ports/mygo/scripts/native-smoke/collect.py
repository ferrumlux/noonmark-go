#!/usr/bin/env python3
"""Export an exact, bounded evidence allowlist. Never traverse session data."""
import argparse
import json
from pathlib import Path
import shutil
import smoke

LOG_LIMIT = 64 << 10
SUMMARY_LIMIT = 32 << 10
PNG_LIMIT = 32 << 20
LABELS = ('list', 'create', 'reopen', 'reuse', 'implicit', 'cross')
FILES = {'stage.json': SUMMARY_LIMIT, 'result.json': SUMMARY_LIMIT, 'manifest.json': 4096}
for label in LABELS:
    FILES[label + '.json'] = SUMMARY_LIMIT
    FILES[label + '.stdout.txt'] = LOG_LIMIT
    FILES[label + '.stderr.txt'] = LOG_LIMIT
FRAMES = {'create/noonmark-day-native.png': (2400, 1536), 'reopen/noonmark-pool-native.png': (2400, 1536)}


def collect(work, output, stage):
    output.mkdir()  # exclusive destination
    report = {'workflow_stages': stage, 'exported': [], 'truncated': [], 'rejected': [], 'missing': []}
    try:
        smoke.directory(work)
    except (ValueError, OSError) as error:
        report['source_unavailable'] = str(error)
    else:
        for name, limit in FILES.items():
            path = work / name
            try:
                st = smoke.plain(path)
                if name.endswith('.txt'):
                    with path.open('rb') as f:
                        data = f.read(limit)
                    if st.st_size > limit:
                        suffix = b'\n[TRUNCATED: evidence excerpt; see collection.json; validation status unchanged]\n'
                        data = data[:limit-len(suffix)] + suffix
                        report['truncated'].append({'name': name, 'original_bytes': st.st_size})
                    (output / name).write_bytes(data)
                else:
                    smoke.require(st.st_size <= limit, 'summary over limit')
                    # Reject malformed summary instead of reinterpreting it as success.
                    data = path.read_bytes()
                    json.loads(data)
                    (output / name).write_bytes(data)
                report['exported'].append(name)
            except FileNotFoundError:
                report['missing'].append(name)
            except (ValueError, OSError) as error:
                report['rejected'].append({'name': name, 'reason': str(error)})
        for name, dimensions in FRAMES.items():
            path = work / name
            try:
                smoke.directory(path.parent)
                smoke.require(smoke.plain(path).st_size <= PNG_LIMIT, 'PNG over limit')
                smoke.png(path, dimensions)
                dest = output / path.name
                shutil.copyfile(path, dest)
                report['exported'].append(name)
            except FileNotFoundError:
                report['missing'].append(name)
            except (ValueError, OSError) as error:
                report['rejected'].append({'name': name, 'reason': str(error)})
    (output / 'collection.json').write_text(json.dumps(report, indent=2) + '\n')
    return report


def gate(output, role):
    """After export/upload, fail a successful product stage with incomplete evidence."""
    smoke.directory(output)
    report_path = output / 'collection.json'
    smoke.require(smoke.plain(report_path).st_size <= SUMMARY_LIMIT, 'collection report over limit')
    report = json.loads(report_path.read_bytes())
    smoke.require('source_unavailable' not in report, 'successful stage evidence source unavailable')
    required = {'stage.json'}
    if role == 'build':
        required.update({'cross.stdout.txt', 'cross.stderr.txt'})
    elif role == 'runtime':
        required.add('manifest.json')
        for label in ('list', 'create', 'reopen', 'reuse', 'implicit'):
            required.update({label + '.json', label + '.stdout.txt', label + '.stderr.txt'})
        required.update(FRAMES)
    else:
        raise ValueError('unknown evidence role')
    smoke.require(required <= set(report['exported']), 'successful stage missing required evidence')
    for field in ('rejected', 'truncated'):
        smoke.require(not required.intersection(item['name'] for item in report[field]), 'successful stage rejected/truncated required evidence')
    for name in required:
        exported_name = Path(name).name if name in FRAMES else name
        smoke.plain(output / exported_name)
    status = json.loads((output / 'stage.json').read_bytes())
    smoke.require(status.get('passed') is True and status.get('stage') == 'completed', 'successful stage summary not completed/passed')
    return required


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--work', type=Path)
    p.add_argument('--gate-role', choices=['build', 'runtime'])
    p.add_argument('--output', type=Path, required=True)
    p.add_argument('--stages', default='{}', help='JSON of workflow step outcomes, never a replacement for smoke status')
    a = p.parse_args()
    if a.gate_role:
        gate(a.output, a.gate_role)
    else:
        smoke.require(a.work is not None, '--work required for collection')
        collect(a.work, a.output, json.loads(a.stages))
