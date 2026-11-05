#!/usr/bin/env python3
"""Static workflow contract, with negative fixtures. Does not emulate Actions."""
from pathlib import Path
import subprocess
import yaml

ROOT = Path(__file__).resolve().parents[4]


class UniqueLoader(yaml.BaseLoader):
    pass


def unique(loader, node):
    values = {}
    for key, value in node.value:
        key = loader.construct_object(key)
        if key in values:
            raise ValueError('duplicate key: ' + key)
        values[key] = loader.construct_object(value)
    return values


UniqueLoader.add_constructor('tag:yaml.org,2002:map', unique)
PINS = {
    'actions/checkout': 'd23441a48e516b6c34aea4fa41551a30e30af803',
    'actions/setup-go': '924ae3a1cded613372ab5595356fb5720e22ba16',
    'actions/upload-artifact': 'ea165f8d65b6e75b540449e92b4886f43607fa02',
}


def validate(text):
    doc = yaml.load(text, Loader=UniqueLoader)
    assert set(doc['on']) == {'push', 'workflow_dispatch'}
    assert doc['on']['push'] == {'branches': ['main'], 'tags': ['v*'], 'paths': [
        '.github/workflows/release.yml', 'ports/mygo/scripts/release/**', 'ports/mygo/release/**',
        'ports/mygo/docs/release/**', 'ports/mygo/scripts/ci/requirements.txt']}
    assert not doc['on']['workflow_dispatch']  # no manual publication/approval override
    assert doc['permissions'] == {'contents': 'read'}
    assert doc['concurrency']['cancel-in-progress'] == 'false'
    assert set(doc['jobs']) == {'verify', 'publish'}
    verify, publish = doc['jobs']['verify'], doc['jobs']['publish']
    assert verify['permissions'] == {'contents': 'read', 'actions': 'read'}
    assert publish['permissions'] == {'contents': 'write', 'actions': 'read'}
    assert publish['needs'] == 'verify'
    assert publish['if'] == "github.event_name == 'push' && startsWith(github.ref, 'refs/tags/') && github.repository == 'ferrumlux/noonmark-go'"
    assert 'secrets.' not in text and 'self-hosted' not in text and 'continue-on-error' not in text
    for job in (verify, publish):
        assert job['runs-on'] == 'ubuntu-24.04'
        assert not any(k in job for k in ('container', 'services'))
        for step in job['steps']:
            assert 'continue-on-error' not in step
            if 'uses' in step:
                name, pin = step['uses'].split('@')
                assert PINS[name] == pin
                opts = step['with']
                if name == 'actions/checkout': assert opts['persist-credentials'] == 'false'
                if name == 'actions/setup-go': assert opts['go-version'] == '1.27.3' and opts['cache'] == 'false'
                if name == 'actions/upload-artifact':
                    paths = opts['path'].splitlines()
                    base = '${{ runner.temp }}/noonmark-release-check/first/package/'
                    assert paths == [base + x for x in ('*.tar.gz', '*.zip', 'manifest.json', 'SHA256SUMS')]
                    assert opts['if-no-files-found'] == 'error'
                    assert 'if' not in step
            if 'run' in step:
                assert '${{' not in step['run']  # untrusted contexts go through quoted env vars
                subprocess.run(['bash', '-n'], input=step['run'], text=True, check=True)
    gates = [s for s in verify['steps'] if 'check_hosted_ci.py' in s.get('run', '')]
    assert len(gates) == 1 and gates[0]['if'] == "github.event_name == 'push' && startsWith(github.ref, 'refs/tags/')"
    assert verify['env']['RELEASE_VERSION'] == "${{ github.event_name == 'push' && startsWith(github.ref, 'refs/tags/') && github.ref_name || '' }}"
    assert 'release.py gate "$GITHUB_REF_NAME"' in gates[0]['run']
    assert 'git merge-base --is-ancestor "$GITHUB_SHA" origin/main' in gates[0]['run']
    assert sum('scripts/release/check ' in s.get('run', '') for s in verify['steps']) == 1
    assert sum('scripts/release/publish.py ' in s.get('run', '') for s in publish['steps']) == 1


def main():
    text = (ROOT / '.github/workflows/release.yml').read_text()
    validate(text)
    negatives = (
        text.replace('contents: read', 'contents: write', 1),
        text.replace('needs: verify', 'needs: []'),
        text.replace("tags: ['v*']", "tags: ['*']"),
        text.replace("branches: ['main']", "branches: ['*']"),
        text.replace(" && startsWith(github.ref, 'refs/tags/')", ""),
        text.replace("if: github.event_name == 'push' && startsWith(github.ref, 'refs/tags/') && github.repository", "if: github.event_name == 'push' && github.repository"),
        text.replace("workflow_dispatch:\n", "workflow_dispatch:\n    inputs:\n      publish:\n        type: boolean\n"),
        text.replace("if: github.event_name == 'push' &&", "if: always() ||"),
        text.replace('*.tar.gz', '**/*'),
        text.replace('if-no-files-found: error', 'if-no-files-found: warn'),
        text.replace('@d23441a48e516b6c34aea4fa41551a30e30af803', '@v6'),
        text.replace('cache: false', 'cache: true'),
        text.replace('persist-credentials: false', 'persist-credentials: true'),
        text.replace('ubuntu-24.04', 'self-hosted'),
        text.replace('permissions:\n', 'permissions: {}\npermissions:\n', 1),
        text.replace('release.py gate "$GITHUB_REF_NAME"', 'echo skipped'),
        text.replace('check_hosted_ci.py', 'unchecked.py'),
    )
    for bad in negatives:
        try:
            validate(bad)
        except (AssertionError, ValueError, KeyError, TypeError):
            continue
        raise AssertionError('unsafe workflow mutation accepted')
    for name in ('package', 'check'):
        subprocess.run(['bash', '-n', str(ROOT / 'ports/mygo/scripts/release' / name)], check=True)
    print(f'PASS: release workflow contract, {len(negatives)} negative fixtures and shell syntax')


if __name__ == '__main__':
    main()
