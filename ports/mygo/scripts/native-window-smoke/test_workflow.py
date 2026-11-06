"""Static workflow contracts and mutations; never builds or executes native code."""
import copy
from pathlib import Path
import unittest

import yaml  # Existing pinned CI validation dependency; never installed by runtime.

ROOT = Path(__file__).resolve().parents[4]
WORKFLOW = ROOT / '.github/workflows/native-window-smoke.yml'
BASE = ROOT / '.github/workflows/native-smoke.yml'
RUNNERS = {
    ('linux/amd64', 'ubuntu-24.04', 'linux_amd64'),
    ('linux/arm64', 'ubuntu-24.04-arm', 'linux_arm64'),
    ('windows/amd64', 'windows-2025', 'windows_amd64'),
    ('windows/arm64', 'windows-11-arm', 'windows_arm64'),
    ('darwin/amd64', 'macos-15-intel', 'darwin_amd64'),
    ('darwin/arm64', 'macos-15', 'darwin_arm64'),
}
PINS = {
    'actions/checkout': 'd23441a48e516b6c34aea4fa41551a30e30af803',
    'actions/setup-go': '924ae3a1cded613372ab5595356fb5720e22ba16',
    'actions/upload-artifact': 'ea165f8d65b6e75b540449e92b4886f43607fa02',
    'actions/download-artifact': 'd3f86a106a0bac45b974a628896c90dbdf5c8093',
}
FILES = {'collection.json', 'result.json', 'manifest.json'}
for label in ('stage1', 'stage2'):
    FILES.update({label + '.json', label + '-driver.json', label + '.stdout.txt', label + '.stderr.txt'})
    FILES.update('infra-' + label + '/' + helper + '.' + stream + '.txt'
                 for helper in ('xvfb', 'dbus') for stream in ('stdout', 'stderr'))


def load(text):
    return yaml.load(text, Loader=yaml.BaseLoader)


def ids(job):
    return {s['id']: s for s in job['steps'] if 'id' in s}


def workflow_contract(text):
    w = load(text)
    assert w['permissions'] == {'contents': 'read'}
    assert set(w['on']) == {'push', 'workflow_dispatch'}
    assert w['on']['push'] == {'branches': ['main'], 'paths': ['.github/workflows/native-window-smoke.yml', 'ports/mygo/**/*.go', 'ports/mygo/**/go.mod', 'ports/mygo/**/go.sum', 'ports/mygo/patches/**', 'ports/mygo/scripts/native-window-smoke/**', 'ports/mygo/scripts/native-smoke/**', 'ports/mygo/scripts/cross-build', 'ports/mygo/scripts/environment', 'ports/mygo/scripts/dependency/**', 'ports/mygo/scripts/ci/requirements.txt']}
    assert set(w['jobs']) == {'build', 'runtime'}
    build, runtime = w['jobs']['build'], w['jobs']['runtime']
    assert build['runs-on'] == 'ubuntu-24.04' and build['timeout-minutes'] == '60'
    assert runtime['runs-on'] == '${{ matrix.runner }}' and runtime['timeout-minutes'] == '20'
    assert runtime['needs'] == 'build' and runtime['strategy']['fail-fast'] == 'false'
    rows = runtime['strategy']['matrix']['include']
    assert len(rows) == 6 and {(r['target'], r['runner'], r['output']) for r in rows} == RUNNERS
    for forbidden in ('secrets.', 'self-hosted', 'workflow_run:', 'pull_request_target:', 'continue-on-error',
                      'scripts/check', 'scripts/test-entrypoints', 'release@'):
        assert forbidden not in text
    for job in (build, runtime):
        assert 'permissions' not in job and 'if' not in job and 'continue-on-error' not in job
        for step in job['steps']:
            if 'uses' in step:
                action, sha = step['uses'].split('@')
                assert PINS[action] == sha
                if action == 'actions/checkout':
                    assert step['with'] == {'ref': '${{ github.sha }}', 'persist-credentials': 'false'}
                if action == 'actions/upload-artifact':
                    assert step['with']['retention-days'] == '7'
                    assert step['with']['if-no-files-found'] == 'error'
                    assert not any(c in step['with']['path'] for c in '*?!')
            if step.get('shell') == 'python {0}':
                compile(step['run'], '<workflow-python>', 'exec')
    b, r = ids(build), ids(runtime)
    # Reuse reviewed routines without a second build/pack implementation.
    baseline = ids(load(BASE.read_text())['jobs']['build'])
    assert b['cross']['run'] == baseline['cross']['run']
    assert b['identity']['run'] == baseline['identity']['run']
    assert build['outputs'] == load(BASE.read_text())['jobs']['build']['outputs']
    assert build['steps'].index(b['fixtures']) < build['steps'].index(b['cross'])
    assert 'if' not in b['fixtures'] and 'if' not in b['cross']
    for part in ("('native-smoke', 'native-window-smoke')", "'unittest', 'discover'", "'test_*.py'",
                 'ports/mygo/scripts/ci/requirements.txt', 'venv.EnvBuilder(with_pip=True)', 'check=True'):
        assert part in b['fixtures']['run']
    for target, _, key in RUNNERS:
        dirname = target.replace('/', '-')
        filename = 'noonmark-mygo-' + dirname + ('.exe' if target.startswith('windows/') else '')
        assert b['upload_' + key]['with']['path'].splitlines() == [
            '${{ runner.temp }}/native-artifacts/' + dirname + '/' + filename,
            '${{ runner.temp }}/native-artifacts/' + dirname + '/manifest.json']
    assert b['collect']['if'] == 'always()'
    assert r['libraries']['if'] == "runner.os == 'Linux'"
    assert 'xvfb xauth dbus' in r['libraries']['run']
    assert r['provenance']['run'] == ids(load(BASE.read_text())['jobs']['runtime'])['provenance']['run']
    assert r['provenance']['env'] == ids(load(BASE.read_text())['jobs']['runtime'])['provenance']['env']
    assert r['download']['with'] == {'artifact-ids': '${{ needs.build.outputs[matrix.output] }}',
        'path': '${{ runner.temp }}/native-artifact', 'merge-multiple': 'true'}
    for stage in ('checkout', 'provenance', 'download', 'smoke'):
        assert 'if' not in r[stage]
    for part in ('setup-go', 'pip', 'venv', 'scripts/cross-build', 'prepare_dependency', 'scripts/environment',
                 'native-smoke/consume.py', 'EGL_PLATFORM'):
        assert part not in str(runtime)
    identity = {'TARGET': '${{ matrix.target }}', 'BUILD_SOURCE': '${{ needs.build.outputs.source }}',
        'BUILD_RUN_ID': '${{ needs.build.outputs.run_id }}', 'BUILD_ATTEMPT': '${{ needs.build.outputs.build_attempt }}',
        'ARTIFACT_ID': '${{ needs.build.outputs[matrix.output] }}'}
    for name in ('smoke', 'collect', 'gate'):
        step = r[name]
        assert {k: step['env'][k] for k in identity} == identity
        for argument, variable in (('source', 'BUILD_SOURCE'), ('run-id', 'BUILD_RUN_ID'),
                                   ('consumer-attempt', 'GITHUB_RUN_ATTEMPT'), ('target', 'TARGET'), ('artifact-id', 'ARTIFACT_ID')):
            assert "'--" + argument + "', os.environ['" + variable + "']" in step['run']
        assert 'check=True' in step['run']
    assert "'--run-attempt', os.environ['BUILD_ATTEMPT']" in r['smoke']['run']
    assert "parent.mkdir(mode=0o700)" in r['smoke']['run'] and 'exist_ok' not in r['smoke']['run']
    assert "'--work-parent', str(parent)" in r['smoke']['run']
    assert "Path(os.environ['RUNNER_TEMP'], 'native-window-parent')" in r['smoke']['run']
    assert 'ports/mygo/scripts/native-window-smoke/run.py' in r['smoke']['run']
    assert r['smoke']['run'].count('subprocess.run(') == 1
    for name in ('collect', 'gate'):
        step = r[name]
        assert step['if'] == 'always()'
        assert step['env']['STAGES'] == '${{ toJSON(steps) }}'
        assert "v.get('outcome', 'unknown')" in step['run']
        assert "'--stages', json.dumps(outcomes)" in step['run']
        assert "'--build-attempt', os.environ['BUILD_ATTEMPT']" in step['run']
        assert 'ports/mygo/scripts/native-window-smoke/collect.py' in step['run']
        assert "'--artifact', str(Path(os.environ['RUNNER_TEMP'], 'native-artifact'))" in step['run']
        assert "'--output', str(Path(os.environ['RUNNER_TEMP'], 'native-window-evidence'))" in step['run']
    assert "'--gate'" in r['gate']['run'] and "'--gate'" not in r['collect']['run']
    assert r['upload']['if'] == "always() && steps.collect.outcome == 'success'"
    assert set(r['upload']['with']['path'].splitlines()) == {'${{ runner.temp }}/native-window-evidence/' + f for f in FILES}
    assert [s.get('id') for s in runtime['steps']][-3:] == ['collect', 'upload', 'gate']
    return w


class WorkflowContracts(unittest.TestCase):
    def test_workflow_contract(self):
        workflow_contract(WORKFLOW.read_text())

    def test_security_provenance_and_evidence_mutations(self):
        original = WORKFLOW.read_text()
        mutations = [
            ('fail-fast: false', 'fail-fast: true'), ('contents: read', 'contents: write'),
            ('persist-credentials: false', 'persist-credentials: true'), ('macos-15-intel', 'macos-latest'),
            ('artifact-ids:', 'name:'), ('native-window-evidence/result.json', 'native-window-evidence/*'),
            ("'--run-attempt', os.environ['BUILD_ATTEMPT']", "'--run-attempt', os.environ['GITHUB_RUN_ATTEMPT']"),
            ("'--consumer-attempt', os.environ['GITHUB_RUN_ATTEMPT']", "'--consumer-attempt', os.environ['BUILD_ATTEMPT']"),
            ("parent.mkdir(mode=0o700)", "parent.mkdir(mode=0o700, exist_ok=True)"),
            ("v.get('outcome', 'unknown')", "v.get('conclusion', 'unknown')"),
            ("always() && steps.collect.outcome == 'success'", "always() && steps.smoke.outcome == 'success'"),
            ("'--gate'", "'--not-a-gate'"), ('xvfb xauth dbus', 'dbus'),
            ("('native-smoke', 'native-window-smoke')", "('native-smoke',)"),
            ('ports/mygo/**/*.go', 'ports/mygo/**'),
            ('id: smoke\n', 'id: smoke\n        continue-on-error: true\n'),
            ('id: gate\n        if: always()', "id: gate\n        if: steps.smoke.outcome == 'success'"),
        ]
        for old, new in mutations:
            with self.subTest(old=old):
                self.assertIn(old, original)
                with self.assertRaises((AssertionError, KeyError, ValueError, SyntaxError)):
                    workflow_contract(original.replace(old, new))

    def test_required_stages_cannot_disappear_or_be_conditional(self):
        original = load(WORKFLOW.read_text())
        for name in ('provenance', 'download', 'smoke', 'collect', 'upload', 'gate'):
            for mutation in ('remove', 'conditional'):
                w = copy.deepcopy(original)
                steps = w['jobs']['runtime']['steps']
                step = next(s for s in steps if s.get('id') == name)
                if mutation == 'remove':
                    steps.remove(step)
                else:
                    step['if'] = 'false'
                with self.subTest(name=name, mutation=mutation):
                    with self.assertRaises((AssertionError, KeyError, ValueError)):
                        workflow_contract(yaml.safe_dump(w))


if __name__ == '__main__':
    unittest.main()
