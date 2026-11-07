# SPDX-License-Identifier: AGPL-3.0-only
"""Python/static-only integration tests. All execution receipts below are mock data.
No Swift, compiler, network, product, or hosted CI execution; no durable fake capture.
"""
import ast
import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import zipfile

HERE = Path(__file__).resolve().parent

def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value

e = load('ordinary_ci_export', HERE / 'export_evidence.py')
f = load('ordinary_ci_synthetic_protocol', e.ORACLE / 'test_contracts.py')


class WorkflowSafety(unittest.TestCase):
    def test_exact_workflow_contract(self):
        e.check_workflow(e.WORKFLOW.read_bytes())

    def assert_runner_context_contract(self, raw):
        # runner is unavailable in jobs.<job_id>.env, but available in step env.
        # Keep this regression independent of the exact-byte workflow hash.
        job, steps = raw.split('    steps:\n', 1)
        self.assertNotIn('${{ runner.', job)
        work = 'NOONMARK_MYGO_BUILD_ROOT: ${{ runner.temp }}/noonmark-ordinary-category-${{ github.run_id }}-${{ github.run_attempt }}'
        evidence = 'EVIDENCE_ROOT: ${{ runner.temp }}/noonmark-ordinary-category-evidence-${{ github.run_id }}-${{ github.run_attempt }}'
        capture = steps.split('        id: capture\n', 1)[1].split('      - name:', 1)[0]
        export = steps.split('      - name: Export only bounded allowlisted synthetic evidence\n', 1)[1].split('      - name:', 1)[0]
        upload = steps.split('      - name: Upload explicit evidence files only\n', 1)[1]
        self.assertIn('        env:\n          ' + work + '\n', capture)
        self.assertIn('        env:\n          ' + work + '\n          ' + evidence + '\n', export)
        self.assertIn('        env:\n          ' + evidence + '\n', upload)
        self.assertEqual(raw.count(work), 2)
        self.assertEqual(raw.count(evidence), 2)

    def test_runner_paths_are_bound_at_step_scope(self):
        self.assert_runner_context_contract(e.WORKFLOW.read_text())

    def test_reject_runner_context_at_job_scope_without_hash_lock(self):
        raw = e.WORKFLOW.read_text()
        broken = raw.replace('    env:\n', '    env:\n      BROKEN: ${{ runner.temp }}\n', 1)
        with self.assertRaises(AssertionError):
            self.assert_runner_context_contract(broken)

    def test_reject_inconsistent_step_paths_without_hash_lock(self):
        raw = e.WORKFLOW.read_text()
        broken = raw.replace('/noonmark-ordinary-category-${{ github.run_id }}', '/different-${{ github.run_id }}', 1)
        with self.assertRaises(AssertionError):
            self.assert_runner_context_contract(broken)

    def test_frozen_source_lock(self):
        _, identity = e.r.verify(e.REVIEWED)
        self.assertEqual(identity, e.REVIEWED)

    def test_no_export_execution_or_network_api(self):
        tree = ast.parse((HERE / 'export_evidence.py').read_text())
        forbidden = {'subprocess', 'socket', 'requests', 'urllib', 'shutil', 'tarfile'}
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                self.assertFalse({n.name.split('.')[0] for n in node.names} & forbidden)
            if isinstance(node, ast.ImportFrom):
                self.assertNotIn((node.module or '').split('.')[0], forbidden)
            if isinstance(node, ast.Call) and isinstance(node.func, ast.Attribute):
                self.assertNotIn(node.func.attr, ('capture', 'Popen', 'system', 'rglob', 'glob'))


# The lock deliberately rejects every unreviewed YAML change, including semantic aliases.
MUTATIONS = {
    'branch_widening': ('branches: [main]', 'branches: [main, dev]'),
    'path_widening': ("'ports/mygo/scripts/ordinary-category-oracle/**'", "'ports/mygo/**'"),
    'pull_request_trigger': ('  workflow_dispatch:', '  pull_request:\n  workflow_dispatch:'),
    'pull_request_target': ('  workflow_dispatch:', '  pull_request_target:\n  workflow_dispatch:'),
    'schedule': ('  workflow_dispatch:', '  schedule:\n    - cron: "* * * * *"\n  workflow_dispatch:'),
    'write_permission': ('contents: read', 'contents: write'),
    'id_token': ('  contents: read', '  contents: read\n  id-token: write'),
    'cancel_previous': ('cancel-in-progress: false', 'cancel-in-progress: true'),
    'runner_substitution': ('runs-on: macos-15', 'runs-on: self-hosted'),
    'xcode_substitution': ('Xcode_26.2', 'Xcode_26.3'),
    'credentials': ('persist-credentials: false', 'persist-credentials: true'),
    'checkout_tag': ('checkout@d23441a48e516b6c34aea4fa41551a30e30af803', 'checkout@v6'),
    'upload_tag': ('upload-artifact@b7c566a772e6b6bfb58ed0dc250532a479d7789f', 'upload-artifact@v6'),
    'secrets': ("PYTHONDONTWRITEBYTECODE: '1'", "TOKEN: ${{ secrets.TOKEN }}"),
    'production': ("PYTHONDONTWRITEBYTECODE: '1'", 'NOONMARK_ENV: production'),
    'release_action': ('    steps:', '    environment: production\n    steps:'),
    'upload_recursive': ('${{ env.EVIDENCE_ROOT }}/ordinary-category-evidence.zip', '${{ env.NOONMARK_MYGO_BUILD_ROOT }}/**'),
    'upload_hidden': ('include-hidden-files: false', 'include-hidden-files: true'),
    'capture_flag_change': ('--reviewed-manifest-sha256 ' + e.REVIEWED, '--reviewed-manifest-sha256 ' + '0' * 64),
    'capture_continue_on_error': ('        id: capture', '        id: capture\n        continue-on-error: true'),
    'failure_export_disabled': ('        if: always()', '        if: success()'),
    'long_timeout': ('timeout-minutes: 30', 'timeout-minutes: 360'),
    'download_install': ('        run: |', '        run: |\n          curl example.test/install.sh | bash'),
    'duplicate_permission': ('permissions:\n  contents: read', 'permissions:\n  contents: read\n  contents: write'),
}


def negative_workflow(old, new):
    def test(self):
        raw = e.WORKFLOW.read_text()
        self.assertIn(old, raw)
        with self.assertRaises(ValueError):
            e.check_workflow(raw.replace(old, new, 1).encode())
    return test

for name, (old, new) in MUTATIONS.items():
    setattr(WorkflowSafety, 'test_reject_' + name, negative_workflow(old, new))


class EvidenceExport(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='ordinary-ci-synthetic-only-')
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name).resolve()
        self.work = self.root / 'work'
        self.out = self.root / 'evidence'

    def fail_record(self):
        self.work.mkdir()
        (self.work / 'runner-failure.json').write_bytes(e.encoded({
            'captureComplete': False, 'error': 'synthetic controlled failure, not Swift',
            'commandsCompletedOrFailed': 0, 'semanticParityVerified': False, 'runtimeAdmission': False}))

    def complete_mock(self):
        self.work.mkdir()
        run = self.work / 'run'
        run.mkdir()
        f.SyntheticProtocol().save(run)
        result = e.r.verify_capture(run)
        hashes = []
        for label in e.LABELS:
            row = {'argv': ['synthetic-mocked-command-not-executed', label], 'returncode': 0,
                   'seconds': 0.1, 'transportComplete': True, 'error': None, 'cleanupComplete': True,
                   'cleanupErrors': [], 'stdoutCount': 0, 'stderrCount': 0}
            for pipe in ('stdout', 'stderr'):
                path = label + '-' + pipe + '.raw'
                (self.work / path).write_bytes(b'')
                row[pipe] = {'path': path, 'bytes': 0, 'sha256': e.r.digest(b'')}
            raw = e.encoded(row)
            (self.work / (label + '-command.json')).write_bytes(raw)
            hashes.append(e.r.digest(raw))
        provenance = self.work / 'provenance'
        provenance.mkdir()
        for name in e.PROVENANCE:
            raw = e.encoded({'syntheticMockOnly': True}) if name == 'staged-identity.json' else e.r.checked(e.ORACLE / ('upstream/LICENSE' if name == 'LICENSE' else name))
            (provenance / name).write_bytes(raw)
        result.update(sourceBoundExecution=True, reviewedToolManifestSHA256=e.REVIEWED,
                      stagedIdentitySHA256=e.r.digest((provenance / 'staged-identity.json').read_bytes()),
                      commandReceiptSHA256s=hashes)
        (self.work / 'capture-receipt.json').write_bytes(e.encoded(result))
        (self.work / 'platform.json').write_bytes(e.encoded({
            'xcode': 'synthetic', 'swift': 'synthetic', 'os': 'synthetic', 'sdk': 'synthetic', 'arch': 'synthetic',
            'imageOS': None, 'imageVersion': None, 'runnerOS': None, 'officialHostedImageVerified': False,
            'reviewedToolManifestSHA256': e.REVIEWED, 'sourceClosureSHA256': e.r.SOURCE_SHA}))

    def summary(self):
        return json.loads((self.out / 'export-summary.json').read_bytes())

    def assert_rejected(self, outcome='success'):
        self.assertFalse(e.export(self.work, self.out, outcome))
        s = self.summary()
        self.assertFalse(s['captureComplete'])
        self.assertFalse(s['evidenceValidated'])
        self.assertEqual(s['reasonCode'], 'evidence-validation-failed')
        with zipfile.ZipFile(self.out / 'ordinary-category-evidence.zip') as z:
            self.assertEqual(z.namelist(), ['export-summary.json'])

    def test_missing_work_preserves_bounded_failure(self):
        self.assertTrue(e.export(self.work, self.out, 'failure'))
        self.assertEqual(self.summary()['reasonCode'], 'capture-not-started')
        self.assertFalse(self.summary()['captureComplete'])

    def test_missing_success_work_rejected(self):
        self.assert_rejected()

    def test_skipped_capture_never_claims_complete(self):
        self.assertTrue(e.export(self.work, self.out, 'skipped'))
        self.assertFalse(self.summary()['captureComplete'])

    def test_failure_evidence_preserved_without_private_files(self):
        self.fail_record()
        for directory in ('home', 'build', 'cache', 'tmp', 'input'):
            (self.work / directory).mkdir()
            (self.work / directory / 'DO-NOT-UPLOAD').write_text('not evidence')
        (self.work / 'arbitrary-private.json').write_text('not evidence')
        (self.work / 'run').mkdir()
        (self.work / 'run' / 'unverified-partial.json').write_text('not evidence')
        self.assertTrue(e.export(self.work, self.out, 'failure'))
        with zipfile.ZipFile(self.out / 'ordinary-category-evidence.zip') as z:
            self.assertEqual(set(z.namelist()), {'runner-failure.json', 'export-summary.json'})

    def test_complete_synthetic_protocol_export(self):
        self.complete_mock()
        self.assertTrue(e.export(self.work, self.out, 'success'))
        s = self.summary()
        self.assertTrue(s['captureComplete'])
        self.assertFalse(s['semanticParityVerified'])
        self.assertFalse(s['runtimeAdmission'])
        with zipfile.ZipFile(self.out / 'ordinary-category-evidence.zip') as z:
            self.assertNotIn('home', z.namelist())
            self.assertIn('run/clock-negativeZero.json', z.namelist())
            self.assertIn('run/main-new-plan-payload.json', z.namelist())
            for ref in s['files']:
                raw = z.read(ref['path'])
                self.assertEqual(len(raw), ref['bytes'])
                self.assertEqual(e.r.digest(raw), ref['sha256'])

    def test_failed_command_diagnostic_prefix_preserved(self):
        self.fail_record()
        row = {'argv': ['synthetic-mocked-command-not-executed'], 'returncode': -9,
               'seconds': 20.1, 'transportComplete': False, 'error': 'process deadline',
               'cleanupComplete': True, 'cleanupErrors': [], 'stdoutCount': 73728, 'stderrCount': 0}
        for pipe, content in (('stdout', b'X' * 65536), ('stderr', b'')):
            path = 'xcode-version-' + pipe + '.diagnostic-prefix'
            (self.work / path).write_bytes(content)
            row[pipe] = {'path': path, 'bytes': len(content), 'sha256': e.r.digest(content)}
        (self.work / 'xcode-version-command.json').write_bytes(e.encoded(row))
        self.assertTrue(e.export(self.work, self.out, 'failure'))
        self.assertFalse(self.summary()['captureComplete'])
        with zipfile.ZipFile(self.out / 'ordinary-category-evidence.zip') as z:
            self.assertEqual(len(z.read('commands/xcode-version-stdout.diagnostic-prefix')), 65536)
            self.assertNotIn('commands/xcode-version-stdout.raw', z.namelist())

    def test_harness_bounded_failure_preserved(self):
        self.fail_record()
        (self.work / 'run').mkdir()
        raw = e.encoded({'schema': 'ordinary-category-failure-v1', 'captureComplete': False,
                         'semanticParityVerified': False, 'error': {'status': 'error',
                         'errorType': 'SyntheticOnly', 'message': 'not Swift'}})
        (self.work / 'run/failure.json').write_bytes(raw)
        self.assertTrue(e.export(self.work, self.out, 'failure'))
        with zipfile.ZipFile(self.out / 'ordinary-category-evidence.zip') as z:
            self.assertEqual(z.read('run/failure.json'), raw)

    def test_command_pipe_symlink_rejected(self):
        self.complete_mock()
        path = self.work / 'xcode-version-stdout.raw'
        path.unlink()
        path.symlink_to(e.ORACLE / 'INPUT.json')
        self.assert_rejected()

    def test_command_pipe_oversize_rejected(self):
        self.complete_mock()
        (self.work / 'xcode-version-stdout.raw').write_bytes(b'X' * 65537)
        self.assert_rejected()

    def test_provenance_link_rejected(self):
        self.complete_mock()
        path = self.work / 'provenance/INPUT.json'
        path.unlink()
        path.symlink_to(e.ORACLE / 'INPUT.json')
        self.assert_rejected()

    def test_unlisted_run_file_rejected(self):
        self.complete_mock()
        (self.work / 'run/extra.json').write_text('{}')
        self.assert_rejected()

    def test_failed_outcome_cannot_export_success_as_golden(self):
        self.complete_mock()
        self.assertTrue(e.export(self.work, self.out, 'failure'))
        self.assertFalse(self.summary()['captureComplete'])
        with zipfile.ZipFile(self.out / 'ordinary-category-evidence.zip') as z:
            self.assertNotIn('capture-receipt.json', z.namelist())
            self.assertFalse(any(n.startswith('run/') for n in z.namelist()))

    def test_forged_capture_receipt_without_originals(self):
        self.fail_record()
        (self.work / 'capture-receipt.json').write_text('{"captureComplete":true}')
        self.assert_rejected()

    def test_failure_symlink_rejected(self):
        self.work.mkdir()
        (self.work / 'runner-failure.json').symlink_to(e.ORACLE / 'INPUT.json')
        self.assert_rejected('failure')

    def test_failure_hardlink_rejected(self):
        import os
        self.fail_record()
        os.link(self.work / 'runner-failure.json', self.root / 'hardlink')
        self.assert_rejected('failure')

    def test_failure_oversize_rejected(self):
        self.fail_record()
        (self.work / 'runner-failure.json').write_bytes(b'X' * 16385)
        self.assert_rejected('failure')

    def test_work_symlink_rejected(self):
        real = self.root / 'real'
        real.mkdir()
        self.work.symlink_to(real, target_is_directory=True)
        self.assert_rejected('failure')

    def test_work_symlink_parent_rejected(self):
        real = self.root / 'real'
        real.mkdir()
        (real / 'work').mkdir()
        link = self.root / 'linked'
        link.symlink_to(real, target_is_directory=True)
        self.work = link / 'work'
        self.assert_rejected('failure')

    def test_summary_does_not_echo_unverified_error(self):
        self.work.mkdir()
        with patch.object(e, 'collect', side_effect=ValueError('private-string-do-not-upload')):
            self.assert_rejected('failure')
        self.assertNotIn(b'private-string', (self.out / 'export-summary.json').read_bytes())

    def test_preexisting_evidence_not_overwritten(self):
        self.out.mkdir()
        marker = self.out / 'marker'
        marker.write_text('keep')
        with self.assertRaises(ValueError):
            e.export(self.work, self.out, 'failure')
        self.assertEqual(marker.read_text(), 'keep')

    def test_overlapping_evidence_rejected(self):
        with self.assertRaises(ValueError):
            e.export(self.work, self.work / 'evidence', 'failure')
        self.assertFalse(self.work.exists())

    def test_repo_output_rejected(self):
        with self.assertRaises(ValueError):
            e.export(self.work, e.REPO / 'do-not-create', 'failure')
        self.assertFalse((e.REPO / 'do-not-create').exists())

    def test_binary_payload_rejected(self):
        with self.assertRaises(UnicodeDecodeError):
            e.add({}, 'binary.json', b'\xff')

    def test_path_traversal_rejected(self):
        for name in ('../secret', 'run/../secret', '/private', 'home/private', '.hidden'):
            with self.subTest(name=name), self.assertRaises(ValueError):
                e.add({}, name, b'{}')

    def test_export_total_budget(self):
        with patch.object(e, 'MAX_EXPORT', 3), self.assertRaises(ValueError):
            e.add({}, 'file.json', b'1234')

    def test_export_count_budget(self):
        with patch.object(e, 'MAX_FILES', 1), self.assertRaises(ValueError):
            e.add({'one.json': b'{}'}, 'two.json', b'{}')

    def test_export_single_file_budget(self):
        with self.assertRaises(ValueError):
            e.add({}, 'file.json', b'12345', 4)

    def test_invalid_outcome_rejected_and_not_echoed(self):
        self.assert_rejected('sensitive-invalid-outcome')
        self.assertEqual(self.summary()['captureOutcome'], 'invalid')


def bad_complete(relative, change):
    def test(self):
        self.complete_mock()
        path = self.work / relative
        if change is None:
            path.unlink()
        else:
            obj = json.loads(path.read_bytes())
            change(obj)
            path.write_bytes(e.encoded(obj))
        self.assert_rejected()
    return test

for name, relative, change in (
    ('missing_manifest', 'run/manifest.json', None),
    ('missing_receipt', 'capture-receipt.json', None),
    ('source_bound_false', 'capture-receipt.json', lambda v: v.update(sourceBoundExecution=False)),
    ('parity_true', 'capture-receipt.json', lambda v: v.update(semanticParityVerified=True)),
    ('admission_true', 'capture-receipt.json', lambda v: v.update(runtimeAdmission=True)),
    ('wrong_tool_identity', 'capture-receipt.json', lambda v: v.update(reviewedToolManifestSHA256='0' * 64)),
    ('missing_command', 'compile-core-command.json', None),
    ('command_incomplete', 'compile-core-command.json', lambda v: v.update(cleanupComplete=False)),
    ('pipe_traversal', 'xcode-version-command.json', lambda v: v['stdout'].update(path='../secret')),
    ('wrong_pipe_hash', 'xcode-version-command.json', lambda v: v['stdout'].update(sha256='0' * 64)),
    ('bad_stage_hash', 'capture-receipt.json', lambda v: v.update(stagedIdentitySHA256='0' * 64)),
    ('changed_provenance', 'provenance/INPUT.json', lambda v: v.update(names=[])),
    ('wrong_command_hash', 'capture-receipt.json', lambda v: v.update(commandReceiptSHA256s=[])),
    ('missing_platform', 'platform.json', None),
    ('self_claim_official_image', 'platform.json', lambda v: v.update(officialHostedImageVerified=True)),
):
    setattr(EvidenceExport, 'test_reject_' + name, bad_complete(relative, change))


if __name__ == '__main__':
    unittest.main()
