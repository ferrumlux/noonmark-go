"""Small workflow contracts/collector fixtures, never executes Go or workflow jobs."""
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import collect
import consume
import smoke
from test_smoke import elf, SHA
import yaml  # already-pinned CI validation dependency; never installed by consumer

ROOT = Path(__file__).resolve().parents[4]
WORKFLOW = ROOT / '.github/workflows/native-smoke.yml'


def workflow_contract(text):
    # BaseLoader keeps YAML's `on` key and scalar strings intact.
    w = yaml.load(text, Loader=yaml.BaseLoader)
    assert w['permissions'] == {'contents': 'read'}
    assert set(w['on']) == {'push', 'workflow_dispatch'}
    assert w['on']['push']['branches'] == ['main']
    assert w['on']['push']['paths'] == ['.github/workflows/native-smoke.yml', 'ports/mygo/**', '!ports/mygo/docs/**', '!ports/mygo/**/*.md']
    jobs = w['jobs']
    assert set(jobs) == {'build', 'runtime'}
    build, runtime = jobs['build'], jobs['runtime']
    assert build['runs-on'] == 'ubuntu-24.04'
    assert runtime['needs'] == 'build' and runtime['strategy']['fail-fast'] == 'false'
    rows = runtime['strategy']['matrix']['include']
    assert {(r['target'], r['runner'], r['output']) for r in rows} == {
        ('linux/amd64', 'ubuntu-24.04', 'linux_amd64'), ('linux/arm64', 'ubuntu-24.04-arm', 'linux_arm64'),
        ('windows/amd64', 'windows-2025', 'windows_amd64'), ('windows/arm64', 'windows-11-arm', 'windows_arm64'),
        ('darwin/amd64', 'macos-15-intel', 'darwin_amd64'), ('darwin/arm64', 'macos-15', 'darwin_arm64')}
    pinned = {'actions/checkout': 'd23441a48e516b6c34aea4fa41551a30e30af803', 'actions/setup-go': '924ae3a1cded613372ab5595356fb5720e22ba16', 'actions/upload-artifact': 'ea165f8d65b6e75b540449e92b4886f43607fa02', 'actions/download-artifact': 'd3f86a106a0bac45b974a628896c90dbdf5c8093'}
    for job in jobs.values():
        for step in job['steps']:
            if 'uses' in step:
                action, sha = step['uses'].split('@')
                assert pinned[action] == sha
                if action == 'actions/checkout':
                    assert step['with']['ref'] == '${{ github.sha }}' and step['with']['persist-credentials'] == 'false'
            if step.get('shell') == 'python {0}':
                compile(step['run'], '<workflow-python>', 'exec')
            assert 'continue-on-error' not in step
    build_ids = [step.get('id') for step in build['steps']]
    assert build_ids.index('fixtures') < build_ids.index('cross')
    fixture = next(step for step in build['steps'] if step.get('id') == 'fixtures')
    assert 'if' not in fixture
    for required in ('venv.EnvBuilder(with_pip=True)', 'ports/mygo/scripts/ci/requirements.txt', "'unittest', 'discover'", "'test_*.py'", 'check=True'):
        assert required in fixture['run']
    assert fixture['run'].count('check=True') == 2
    assert "Path(os.environ['RUNNER_TEMP'], 'native-smoke-python')" in fixture['run']
    assert 'secrets.' not in text and 'self-hosted' not in text
    assert 'scripts/check' not in text and 'scripts/test-entrypoints' not in text
    assert text.count("['bash', 'ports/mygo/scripts/cross-build']") == 1
    rt = {s['id']: s for s in runtime['steps'] if 'id' in s}
    assert rt['provenance']['env']['ARTIFACT_ID'] == '${{ needs.build.outputs[matrix.output] }}'
    assert "re.fullmatch('[1-9][0-9]*'" in rt['provenance']['run']
    assert rt['download']['with'] == {'artifact-ids': '${{ needs.build.outputs[matrix.output] }}', 'path': '${{ runner.temp }}/native-artifact', 'merge-multiple': 'true'}
    assert rt['smoke']['env']['BUILD_ATTEMPT'] == '${{ needs.build.outputs.build_attempt }}'
    assert "os.environ['GITHUB_RUN_ATTEMPT']" in rt['smoke']['run']
    runtime_text = str(runtime)
    assert 'pip' not in runtime_text and 'venv' not in runtime_text
    assert 'setup-go' not in runtime_text and 'prepare_dependency' not in runtime_text and 'scripts/environment' not in runtime_text
    for row in rows:
        key = row['output']
        assert build['outputs'][key] == '${{ steps.upload_' + key + '.outputs.artifact-id }}'
    for job in jobs.values():
        collectors = [s for s in job['steps'] if s.get('id') == 'collect']
        assert collectors[0]['if'] == 'always()' and 'collect.py' in collectors[0]['run']
    for role, job, product in [('build', build, 'cross'), ('runtime', runtime, 'smoke')]:
        gates = [step for step in job['steps'] if '--gate-role' in step.get('run', '')]
        assert len(gates) == 1
        assert gates[0]['if'] == "always() && steps." + product + ".outcome == 'success'"
        assert "'--gate-role', '" + role + "'" in gates[0]['run'] and 'check=True' in gates[0]['run']
        assert job['steps'].index(gates[0]) > max(i for i, step in enumerate(job['steps']) if step.get('uses', '').startswith('actions/upload-artifact@'))
    return w


class Contracts(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.addCleanup(self.tmp.cleanup)

    def test_workflow_and_mutations(self):
        text = WORKFLOW.read_text()
        workflow_contract(text)
        for old, new in [('fail-fast: false', 'fail-fast: true'), ('artifact-ids:', 'name:'), ('persist-credentials: false', 'persist-credentials: true'), ('contents: read', 'contents: write'), ('macos-15-intel', 'macos-latest')]:
            with self.assertRaises((AssertionError, KeyError, ValueError)):
                workflow_contract(text.replace(old, new))

    def test_collector_bounded_allowlist_and_failure_preserved(self):
        work = self.root / 'work'
        work.mkdir()
        (work / 'session').mkdir()
        (work / 'session' / 'state.json').write_bytes(b'PRIVATE FIXTURE')
        (work / 'extra-secret.txt').write_bytes(b'PRIVATE FIXTURE')
        (work / 'stage.json').write_text(json.dumps({'passed': False, 'failure': 'too much output'}))
        (work / 'list.stdout.txt').write_bytes(b'x' * (collect.LOG_LIMIT * 3))
        (work / 'list.stderr.txt').write_bytes(b'')
        output = self.root / 'export'
        report = collect.collect(work, output, {'smoke': 'failure'})
        self.assertEqual(json.loads((output / 'stage.json').read_bytes())['passed'], False)
        self.assertEqual((output / 'list.stdout.txt').stat().st_size, collect.LOG_LIMIT)
        self.assertIn(b'TRUNCATED', (output / 'list.stdout.txt').read_bytes())
        self.assertEqual(report['truncated'][0]['original_bytes'], collect.LOG_LIMIT * 3)
        self.assertFalse((output / 'session').exists())
        self.assertFalse((output / 'extra-secret.txt').exists())
        self.assertEqual(report['workflow_stages'], {'smoke': 'failure'})

    def test_collector_links_bad_json_png_missing_root(self):
        work = self.root / 'work'
        work.mkdir()
        target = self.root / 'outside'
        target.write_bytes(b'private')
        (work / 'list.stdout.txt').symlink_to(target)
        (work / 'stage.json').write_bytes(b'{bad')
        (work / 'create').mkdir()
        (work / 'create/noonmark-day-native.png').write_bytes(b'bad')
        report = collect.collect(work, self.root / 'export', {})
        self.assertEqual({r['name'] for r in report['rejected']}, {'list.stdout.txt', 'stage.json', 'create/noonmark-day-native.png'})
        report = collect.collect(self.root / 'missing', self.root / 'export-missing', {'libraries': 'failure'})
        self.assertIn('source_unavailable', report)
        self.assertEqual(report['workflow_stages']['libraries'], 'failure')

    def test_consumer_attempt_source_stage_failure(self):
        binary = self.root / 'binary'
        binary.write_bytes(elf())
        artifact = self.root / 'artifact'
        smoke.pack(binary, artifact, 'linux/amd64', SHA, '123', '1')
        work = self.root / 'work'
        with patch.dict(os.environ, {'NATIVE_TEST_SECRET': 'must-not-be-exported'}), patch.object(smoke, 'native_host', return_value='linux/amd64'), patch.object(smoke, 'smoke'):
            consume.consume(artifact, work, 'linux/amd64', SHA, SHA, '123', '1', '2')
        result = json.loads((work / 'stage.json').read_bytes())
        self.assertTrue(result['passed'])
        for key in ('host_platform', 'python_version', 'python_implementation', 'python_process_machine'):
            self.assertIs(type(result[key]), str)
            self.assertTrue(result[key])
        self.assertNotIn('environment', result)
        self.assertNotIn('NATIVE_TEST_SECRET', json.dumps(result))
        self.assertNotIn('must-not-be-exported', json.dumps(result))
        self.assertEqual((result['build_attempt'], result['consumer_attempt']), ('1', '2'))
        work = self.root / 'mismatch'
        with self.assertRaises(ValueError):
            consume.consume(artifact, work, 'linux/amd64', SHA, 'b'*40, '123', '1', '2')
        result = json.loads((work / 'stage.json').read_bytes())
        self.assertFalse(result['passed'])
        self.assertEqual(result['stage'], 'provenance')
        work = self.root / 'host-mismatch'
        with patch.object(smoke, 'native_host', return_value='linux/arm64'), self.assertRaises(ValueError):
            consume.consume(artifact, work, 'linux/amd64', SHA, SHA, '123', '1', '2')
        self.assertEqual(json.loads((work / 'stage.json').read_bytes())['stage'], 'native-host')

    def test_required_fixture_and_gate_steps_cannot_disappear(self):
        text = WORKFLOW.read_text()
        start = text.index('      - name: Test native smoke contracts before build')
        end = text.index('      - name: Reviewed six-target cross-build and package')
        for changed in [text[:start] + text[end:],
                        text.replace('id: fixtures', 'id: fixtures\n        continue-on-error: true'),
                        text.replace("'-p', 'test_*.py'", "'-p', 'nothing_*.py'"),
                        text.replace('ports/mygo/**', 'ports/mygo/scripts/native-smoke/**'),
                        text.replace("--gate-role", "--unused-option"),
                        text.replace("steps.smoke.outcome == 'success'", "steps.smoke.outcome == 'failure'")]:
            with self.assertRaises((AssertionError, KeyError, ValueError)):
                workflow_contract(changed)

    def complete_evidence(self, work, role):
        work.mkdir()
        (work / 'stage.json').write_text(json.dumps({'stage': 'completed', 'passed': True}))
        if role == 'build':
            names = ['cross.stdout.txt', 'cross.stderr.txt']
        else:
            (work / 'manifest.json').write_text('{}')
            names = []
            for label in ('list', 'create', 'reopen', 'reuse', 'implicit'):
                (work / (label + '.json')).write_text('{}')
                names.extend([label + '.stdout.txt', label + '.stderr.txt'])
            for name in collect.FRAMES:
                p = work / name
                p.parent.mkdir()
                p.write_bytes(b'synthetic checked raster')
        for name in names:
            (work / name).write_bytes(b'')

    def test_success_evidence_gate_positive_optional_and_negative(self):
        for role in ('build', 'runtime'):
            work = self.root / role
            self.complete_evidence(work, role)
            with patch.object(smoke, 'png'):
                collect.collect(work, self.root / (role + '-export'), {'smoke': 'success'})
            collect.gate(self.root / (role + '-export'), role)
        # Optional runtime items such as cross/result are absent and do not block.
        for label, mutate in [
            ('bad-stage', lambda work: (work / 'stage.json').write_bytes(b'{bad')),
            ('bad-pass', lambda work: (work / 'stage.json').write_bytes(b'{"stage":"completed","passed":1}')),
            ('missing-frame', lambda work: (work / 'create/noonmark-day-native.png').unlink()),
            ('truncated-log', lambda work: (work / 'list.stdout.txt').write_bytes(b'x' * (collect.LOG_LIMIT + 1))),
            ('missing-stage', lambda work: (work / 'stage.json').unlink())]:
            work = self.root / label
            self.complete_evidence(work, 'runtime')
            mutate(work)
            output = self.root / (label + '-export')
            with patch.object(smoke, 'png'):
                collect.collect(work, output, {'smoke': 'success'})
            self.assertTrue((output / 'collection.json').is_file())
            with self.assertRaises(ValueError):
                collect.gate(output, 'runtime')
        output = self.root / 'unavailable-export'
        collect.collect(self.root / 'absent', output, {'smoke': 'success'})
        with self.assertRaises(ValueError):
            collect.gate(output, 'runtime')

    def test_trigger_product_inputs_and_markdown_exclusions(self):
        # Fixed include/exclude contract above is the authority; these examples
        # document intended GitHub glob semantics, not a replacement glob engine.
        patterns = workflow_contract(WORKFLOW.read_text())['on']['push']['paths']
        self.assertIn('ports/mygo/**', patterns)
        for source in ('main.go', 'go.mod', 'go.sum', 'patches/mygo/native-finalize/manifest.json', 'patches/mygo/native-finalize/0001-native-finalize.patch', 'scripts/environment', 'internal/taskapp/service.go'):
            self.assertFalse(source.startswith('docs/') or source.endswith('.md'))
        self.assertEqual(patterns[-2:], ['!ports/mygo/docs/**', '!ports/mygo/**/*.md'])


if __name__ == '__main__':
    unittest.main()
