"""Single-host business workflow source contract. No workflow/product execution."""
import copy
import hashlib
from pathlib import Path
import unittest
import test_workflow as legacy

ROOT = legacy.ROOT
WORKFLOW = ROOT / '.github/workflows/native-category-business.yml'
PINS = legacy.PINS
PATHS = ['.github/workflows/native-category-business.yml', 'ports/mygo/**/*.go', 'ports/mygo/**/go.mod', 'ports/mygo/**/go.sum', 'ports/mygo/patches/**', 'ports/mygo/scripts/native-window-smoke/**', 'ports/mygo/scripts/native-smoke/**', 'ports/mygo/scripts/build', 'ports/mygo/scripts/environment', 'ports/mygo/scripts/dependency/**', 'ports/mygo/scripts/ci/requirements.txt']
FILES = legacy.FILES | {'stage1-business.json', 'stage2-business.json'}


def contract(text):
    w = legacy.load(text)
    assert w['permissions'] == {'contents': 'read'}
    assert set(w['on']) == {'push', 'workflow_dispatch'}
    assert w['on']['push'] == {'branches': ['dev/ordinary-category', 'main'], 'paths': PATHS}
    assert set(w['jobs']) == {'build', 'runtime'}
    build, runtime = w['jobs']['build'], w['jobs']['runtime']
    assert build['runs-on'] == runtime['runs-on'] == 'ubuntu-24.04'
    assert build['timeout-minutes'] == '60' and runtime['timeout-minutes'] == '20'
    assert runtime['needs'] == 'build' and 'strategy' not in runtime
    assert build['outputs'] == {'source': '${{ steps.identity.outputs.source }}', 'build_attempt': '${{ steps.identity.outputs.build_attempt }}', 'run_id': '${{ steps.identity.outputs.run_id }}', 'binary_sha256': '${{ steps.cross.outputs.binary_sha256 }}', 'linux_amd64': '${{ steps.upload_linux_amd64.outputs.artifact-id }}'}
    for forbidden in ('secrets.', 'self-hosted', 'workflow_run:', 'pull_request_target:', 'continue-on-error', 'matrix.', 'scripts/cross-build', 'go test', '-tags', 'EGL_PLATFORM', 'breakaway'):
        assert forbidden not in text
    for job in (build, runtime):
        assert 'permissions' not in job and 'if' not in job
        for step in job['steps']:
            if 'uses' in step:
                action, pin = step['uses'].split('@')
                assert PINS[action] == pin
                if action == 'actions/checkout':
                    assert step['with'] == {'ref': '${{ github.sha }}', 'persist-credentials': 'false'}
                if action == 'actions/setup-go':
                    assert step['with'] == {'go-version': '1.27.3', 'cache': 'false'}
                if action == 'actions/upload-artifact':
                    assert step['with']['retention-days'] == '7' and step['with']['if-no-files-found'] == 'error'
                    assert not any(c in step['with']['path'] for c in '*?!')
            if step.get('shell') == 'python {0}': compile(step['run'], '<business-workflow>', 'exec')
    b, r = legacy.ids(build), legacy.ids(runtime)
    baseline = legacy.load(legacy.WORKFLOW.read_text())
    old_b, old_r = legacy.ids(baseline['jobs']['build']), legacy.ids(baseline['jobs']['runtime'])
    assert b['identity']['run'] == old_b['identity']['run']
    assert b['fixtures']['run'] == old_b['fixtures']['run']
    assert build['steps'].index(b['fixtures']) < build['steps'].index(b['cross'])
    assert 'if' not in b['fixtures'] and 'if' not in b['cross']
    assert "['bash', 'ports/mygo/scripts/build']" in b['cross']['run']
    assert "smoke.native_host() != 'linux/amd64'" in b['cross']['run']
    assert "smoke.pack(root / 'bin' / 'noonmark-mygo'" in b['cross']['run']
    assert "'binary_sha256=' + smoke.digest(root / 'bin' / 'noonmark-mygo')" in b['cross']['run']
    assert b['cross']['run'].count('subprocess.run(') == 1 and 'timeout=3000' in b['cross']['run']
    assert b['upload_linux_amd64']['with']['path'].splitlines() == ['${{ runner.temp }}/native-artifacts/linux-amd64/noonmark-mygo-linux-amd64', '${{ runner.temp }}/native-artifacts/linux-amd64/manifest.json']
    assert [s.get('id') for s in build['steps'] if s.get('id', '').startswith('upload_')] == ['upload_linux_amd64']
    assert r['libraries'] == old_r['libraries']
    assert r['download']['with'] == {'artifact-ids': '${{ needs.build.outputs.linux_amd64 }}', 'path': '${{ runner.temp }}/native-artifact', 'merge-multiple': 'true'}
    identity = {'TARGET': 'linux/amd64', 'BUILD_SOURCE': '${{ needs.build.outputs.source }}', 'BUILD_RUN_ID': '${{ needs.build.outputs.run_id }}', 'BUILD_ATTEMPT': '${{ needs.build.outputs.build_attempt }}', 'ARTIFACT_ID': '${{ needs.build.outputs.linux_amd64 }}', 'BUILD_BINARY_SHA256': '${{ needs.build.outputs.binary_sha256 }}'}
    assert r['provenance']['env'] == {k: v for k, v in identity.items() if k != 'TARGET'}
    assert "os.environ['BUILD_SOURCE'] != os.environ['GITHUB_SHA']" in r['provenance']['run']
    assert "os.environ['BUILD_RUN_ID'] != os.environ['GITHUB_RUN_ID']" in r['provenance']['run']
    assert "re.fullmatch('[0-9a-f]{64}', os.environ['BUILD_BINARY_SHA256'])" in r['provenance']['run']
    for key in ('checkout', 'provenance', 'download', 'smoke'): assert 'if' not in r[key]
    for name in ('smoke', 'collect', 'gate'):
        step = r[name]
        assert {k: step['env'][k] for k in identity} == identity
        assert "'--business-scenario', 'ordinary-category-v1'" in step['run']
        for argument, variable in (('source', 'BUILD_SOURCE'), ('run-id', 'BUILD_RUN_ID'), ('consumer-attempt', 'GITHUB_RUN_ATTEMPT'), ('target', 'TARGET'), ('artifact-id', 'ARTIFACT_ID')):
            assert "'--"+argument+"', os.environ['"+variable+"']" in step['run']
        assert 'check=True' in step['run'] and step['run'].count('subprocess.run(') == 1
    assert "'--run-attempt', os.environ['BUILD_ATTEMPT']" in r['smoke']['run']
    assert 'parent.mkdir(mode=0o700)' in r['smoke']['run'] and 'exist_ok' not in r['smoke']['run']
    for name in ('smoke', 'gate'):
        assert "smoke.require(smoke.digest(binary) == os.environ['BUILD_BINARY_SHA256'], 'trusted build binary hash mismatch')" in r[name]['run']
        assert "binary = smoke.verify(" in r[name]['run']
    for name in ('collect', 'gate'):
        step = r[name]
        assert step['if'] == 'always()' and step['env']['STAGES'] == '${{ toJSON(steps) }}'
        assert "v.get('outcome', 'unknown')" in step['run']
        assert "'--build-attempt', os.environ['BUILD_ATTEMPT']" in step['run']
        assert "'--stages', json.dumps(outcomes)" in step['run']
    assert "'--gate'" in r['gate']['run'] and "'--gate'" not in r['collect']['run']
    assert r['upload']['if'] == "always() && steps.collect.outcome == 'success'"
    assert set(r['upload']['with']['path'].splitlines()) == {'${{ runner.temp }}/native-window-evidence/'+f for f in FILES}
    assert [s.get('id') for s in runtime['steps']][-3:] == ['collect', 'upload', 'gate']
    return w


class BusinessWorkflowContracts(unittest.TestCase):
    def test_current_workflow(self):
        contract(WORKFLOW.read_text())

    def test_old_six_host_workflow_bytes_unchanged(self):
        self.assertEqual(hashlib.sha256(legacy.WORKFLOW.read_bytes()).hexdigest(), '16caad6ba077a61dce9a275742946eadab806cad69a9d282599349bb4f8c2448')

    def test_negative_contract_mutations(self):
        original = WORKFLOW.read_text()
        mutations = [('contents: read', 'contents: write'), ('ubuntu-24.04', 'ubuntu-latest'), ('branches: [dev/ordinary-category, main]', 'branches: ["*"]'), ('persist-credentials: false', 'persist-credentials: true'), ('artifact-ids:', 'name:'), ("'--business-scenario', 'ordinary-category-v1'", "'--business-scenario', 'other'"), ('native-window-evidence/stage2-business.json', 'native-window-evidence/*'), ("'--run-attempt', os.environ['BUILD_ATTEMPT']", "'--run-attempt', os.environ['GITHUB_RUN_ATTEMPT']"), ("'--consumer-attempt', os.environ['GITHUB_RUN_ATTEMPT']", "'--consumer-attempt', os.environ['BUILD_ATTEMPT']"), ('parent.mkdir(mode=0o700)', 'parent.mkdir(mode=0o700, exist_ok=True)'), ("v.get('outcome', 'unknown')", "v.get('conclusion', 'unknown')"), ("'--gate'", "'--skip-gate'"), ("smoke.digest(binary) == os.environ['BUILD_BINARY_SHA256']", 'True'), ('ports/mygo/scripts/build', 'ports/mygo/scripts/cross-build'), ('xvfb xauth dbus', 'dbus'), ('id: smoke\n', 'id: smoke\n        continue-on-error: true\n'), ('id: gate\n        if: always()', 'id: gate\n        if: success()')]
        for old, new in mutations:
            self.assertIn(old, original)
            with self.subTest(old=old), self.assertRaises((AssertionError, KeyError, SyntaxError)):
                contract(original.replace(old, new))


if __name__ == '__main__': unittest.main()
