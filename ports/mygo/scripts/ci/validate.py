#!/usr/bin/env python3
"""Offline workflow safety/entrypoint contract; not a GitHub runner emulator."""
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import yaml

ROOT = Path(__file__).resolve().parents[4]
path = ROOT / '.github/workflows/mygo.yml'

class UniqueLoader(yaml.BaseLoader):
    pass

def unique_mapping(loader, node):
    result = {}
    for key, value in node.value:
        key = loader.construct_object(key)
        if key in result:
            raise ValueError('duplicate YAML key: ' + key)
        result[key] = loader.construct_object(value)
    return result

UniqueLoader.add_constructor('tag:yaml.org,2002:map', unique_mapping)

def validate(text):
    doc = yaml.load(text, Loader=UniqueLoader)
    assert set(doc['on']) == {'push', 'pull_request', 'workflow_dispatch'}
    assert doc['permissions'] == {'contents': 'read'}
    assert set(doc['jobs']) == {'development'}
    job = doc['jobs']['development']
    assert job['runs-on'] == 'ubuntu-24.04'
    assert job['timeout-minutes'] == '60'
    assert 'runner.' not in str(job.get('env', {}))
    assert not any(x in text for x in ('secrets.', 'pull_request_target', 'self-hosted'))
    assert not any(k in job for k in ('permissions', 'container', 'services', 'environment'))
    pins = {
        'actions/checkout': 'd23441a48e516b6c34aea4fa41551a30e30af803',
        'actions/setup-go': '924ae3a1cded613372ab5595356fb5720e22ba16',
        'actions/upload-artifact': 'ea165f8d65b6e75b540449e92b4886f43607fa02',
    }
    seen = set()
    for step in job['steps']:
        assert 'continue-on-error' not in step
        if 'uses' in step:
            action, sha = step['uses'].split('@')
            assert re.fullmatch('[0-9a-f]{40}', sha) and pins[action] == sha
            seen.add(action)
            opts = step['with']
            if action.endswith('checkout'):
                assert opts['persist-credentials'] == 'false'
            elif action.endswith('setup-go'):
                assert opts['go-version'] == '1.27.3' and opts['cache'] == 'false'
            else:
                assert opts['path'] == '${{ runner.temp }}/noonmark-mygo/ci-evidence/*.txt'
                assert opts['retention-days'] == '7'
        elif 'run' in step:
            subprocess.run(['bash', '-n'], input=step['run'], text=True, check=True)
    assert seen == set(pins)
    assert any(s.get('run') == 'bash ports/mygo/scripts/ci/run' for s in job['steps'])

text = path.read_text()
validate(text)
for bad in (
    text.replace('    env:\n', '    env:\n      BAD_ROOT: ${{ runner.temp }}/bad\n'),
    text.replace('pull_request:', 'pull_request_target:'),
    text.replace('contents: read', 'contents: write'),
    text.replace('cache: false', 'cache: true'),
    text.replace('ci-evidence/*.txt', '**/*'),
    text.replace('ubuntu-24.04', 'self-hosted'),
    text.replace('go-version: \'1.27.3\'', 'go-version: \'stable\''),
    text.replace('@d23441a48e516b6c34aea4fa41551a30e30af803', '@v4'),
    text.replace('permissions:\n', 'permissions: {}\npermissions:\n'),
):
    try:
        validate(bad)
    except (AssertionError, ValueError, KeyError):
        continue
    raise AssertionError('unsafe mutation accepted')
subprocess.run(['bash', '-n', str(ROOT / 'ports/mygo/scripts/ci/run')], check=True)
print('Workflow contract and 9 negative safety fixtures passed; shell syntax passed.')

# Capture is a foreground pipeline member: completion includes the bounded write.
with tempfile.TemporaryDirectory(prefix='noonmark-ci-capture-') as tmp:
    output = Path(tmp) / 'log.txt'
    payload = bytes(range(256)) * 2049
    capture = ROOT / 'ports/mygo/scripts/ci/capture.py'
    result = subprocess.run([sys.executable, '-B', str(capture), str(output)], input=payload, capture_output=True, check=True)
    assert result.stdout == payload and output.read_bytes() == payload[-262144:]
    failed = subprocess.run([sys.executable, '-B', str(capture), tmp], input=b'failure', capture_output=True)
    assert failed.returncode != 0
print('Bounded synchronous capture and write-failure fixtures passed.')
subprocess.run([sys.executable, '-B', str(ROOT / 'ports/mygo/scripts/ci/test_runner.py')], check=True)
