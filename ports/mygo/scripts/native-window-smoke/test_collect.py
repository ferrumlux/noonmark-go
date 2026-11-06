"""Synthetic collector/gate tests only: these do not provide native-platform evidence."""
import hashlib
import importlib.util
import json
import shutil
import struct
import subprocess
import sys
from pathlib import Path
import tempfile
import unittest


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


m = load('window_collect_tests_subject', 'collect.py')
r = load('window_collect_tests_reports', 'test_run.py')


class SyntheticCollectionTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.parent = self.base / 'work'
        self.root = self.parent / 'native-window-synthetic'
        self.root.mkdir(parents=True)
        self.output = self.base / 'evidence'
        self.expected = dict(source='a' * 40, run_id='123', build_attempt='1',
                             consumer_attempt='2', target='linux/amd64', artifact_id='456')
        self.stages = dict(provenance='success', download='success', libraries='success', smoke='success')
        self.fixture()

    def write(self, name, value):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(value) + '\n')

    def read(self, name):
        return json.loads((self.root / name).read_text())

    def change(self, name, **changes):
        value = self.read(name)
        value.update(changes)
        self.write(name, value)

    def fixture(self):
        self.artifact = self.base / 'artifact'
        if self.artifact.exists():
            shutil.rmtree(self.artifact)
        binary = self.base / 'synthetic-header-never-executed'
        if self.expected['target'].startswith('windows/'):
            header = bytearray(128)
            header[:2] = b'MZ'
            struct.pack_into('<I', header, 60, 64)
            header[64:68] = b'PE\0\0'
            struct.pack_into('<H', header, 68, 0x8664)
            header[88:90] = b'\x0b\x02'
        else:
            header = bytearray(64)
            header[:6] = b'\x7fELF\x02\x01'
            struct.pack_into('<H', header, 18, 62)
        binary.write_bytes(header)
        r.m.smoke.pack(binary, self.artifact, self.expected['target'], self.expected['source'],
                       self.expected['run_id'], self.expected['build_attempt'])
        manifest = json.loads((self.artifact / 'manifest.json').read_text())
        digest = manifest['sha256']
        self.write('manifest.json', manifest)
        self.write('result.json', dict(version=1, scope='lifecycle-only', passed=True,
                   display_pixels_verified=False, ime_tested=False, **self.expected,
                   run_attempt=self.expected['build_attempt'], budget_kind='software-watchdog',
                   work_root=str(self.root), binary_sha256=digest, duration_seconds=2.0))
        for number, stage in ((1, 'create'), (2, 'open')):
            report = r.report()
            report['stage'] = stage
            self.write(f'stage{number}.json', report)
            self.write(f'stage{number}-driver.json', dict(stage=stage, passed=True, marker_count=1,
                       marker_observed_seconds=0.1, exit=0, duration_seconds=0.5,
                       processes=[dict(pid=1234+number*10+i, pgid=1234+number*10+i, reaped=True, exit=0, group_empty=True) for i in range(3)],
                       budget_kind='software-watchdog', today=report['today'],
                       report_sha256=hashlib.sha256((self.root / f'stage{number}.json').read_bytes()).hexdigest(),
                       session_path=str(self.root / 'session'), session_identity=r.m.smoke.IDENTITY.decode().rstrip('\n'),
                       session_files={'SESSION_IDENTITY': hashlib.sha256(r.m.smoke.IDENTITY).hexdigest(), 'synthetic/COMMITTED': 'b'*64, 'synthetic/state.json': 'c'*64}))
            (self.root / f'stage{number}.stdout.txt').write_bytes(r.m.MARKER+b'\n')
            (self.root / f'stage{number}.stderr.txt').write_text('synthetic stderr\n')
            infra = self.root / f'infra-stage{number}'
            infra.mkdir(exist_ok=True)
            for service in ('xvfb', 'dbus'):
                for stream in ('stdout', 'stderr'):
                    (infra / f'{service}.{stream}.txt').write_text('synthetic infrastructure log\n')

    def collect(self):
        record = m.collect(self.parent, self.output, self.expected, self.stages, self.artifact)
        self.assertIsInstance(record, dict)
        self.assertTrue((self.output / 'collection.json').is_file())
        return record

    def reject(self):
        self.collect()
        with self.assertRaises(ValueError):
            m.gate(self.output, self.expected, self.artifact)

    def test_synthetic_required_cli_gate_checks_current_upload_outcome(self):
        self.collect()
        args = [sys.executable, '-B', str(Path(__file__).with_name('collect.py')), '--gate',
                '--output', str(self.output), '--artifact', str(self.artifact)]
        for key, value in self.expected.items():
            args.extend(['--' + key.replace('_', '-'), value])
        for outcome, code in (('success', 0), ('failure', 1), ('skipped', 1)):
            stages = dict(self.stages, collect='success', upload=outcome)
            result = subprocess.run(args + ['--stages', json.dumps(stages)], capture_output=True, timeout=10)
            self.assertEqual(result.returncode, code, result.stderr.decode())

    def test_synthetic_valid_earlier_build_consumed_by_later_attempt(self):
        self.collect()
        m.gate(self.output, self.expected, self.artifact)
        self.assertEqual(self.read('result.json')['run_attempt'], '1')
        self.assertEqual(self.read('result.json')['consumer_attempt'], '2')

    def test_synthetic_windows_job_evidence_without_native_execution(self):
        self.expected['target'] = 'windows/amd64'
        self.fixture()
        for number in (1, 2):
            shutil.rmtree(self.root / f'infra-stage{number}')
            self.change(f'stage{number}-driver.json', processes=[dict(
                pid=number+1000, assigned_to_job=True, resumed=True, reaped=True,
                job_empty=True, exit=0, cleanup_incomplete=False)])
        self.stages.pop('libraries')
        self.collect()
        m.gate(self.output, self.expected, self.artifact)

    def test_synthetic_exact_ready_boundary_is_rejected(self):
        self.change('stage1-driver.json', marker_observed_seconds=45, duration_seconds=46)
        self.reject()

    def test_synthetic_identity_mismatch_each_field(self):
        for key, bad in dict(source='b'*40, run_id='999', build_attempt='2', consumer_attempt='3',
                             target='windows/amd64', artifact_id='999', run_attempt='2').items():
            with self.subTest(key=key):
                original = self.read('result.json')
                self.change('result.json', **{key: bad})
                self.output = self.base / ('mismatch-' + key)
                self.reject()
                self.write('result.json', original)

    def test_synthetic_manifest_provenance_mismatch(self):
        for key, bad in dict(source_sha='b'*40, run_id='999', run_attempt='2', target='windows/amd64', sha256='d'*64).items():
            with self.subTest(key=key):
                original = self.read('manifest.json')
                self.change('manifest.json', **{key: bad})
                self.output = self.base / ('manifest-' + key)
                self.reject()
                self.write('manifest.json', original)

    def test_synthetic_missing_evidence(self):
        (self.root / 'stage2.json').unlink()
        self.reject()

    def test_synthetic_unlisted_private_file_never_copied(self):
        (self.root / 'private-secret.txt').write_text('not evidence')
        self.collect()
        self.assertFalse((self.output / 'private-secret.txt').exists())
        self.assertNotIn('not evidence', (self.output / 'collection.json').read_text())

    def test_synthetic_extra_work_root_rejected(self):
        (self.parent / 'native-window-other').mkdir()
        self.reject()

    def test_synthetic_link_rejected_without_copying_target(self):
        external = self.base / 'outside'
        external.write_text('external secret must not be copied')
        path = self.root / 'stage1.stdout.txt'
        path.unlink()
        try:
            path.symlink_to(external)
        except OSError:
            self.skipTest('symlinks unavailable')
        self.reject()
        copied = self.output / 'stage1.stdout.txt'
        self.assertFalse(copied.exists())

    def test_synthetic_duplicate_and_truncated_json_raw_retained(self):
        original = (self.root / 'result.json').read_bytes()
        for index, raw in enumerate((b'{"passed":true,"passed":true}', original[:-4], original+b'{}')):
            with self.subTest(index=index):
                (self.root / 'result.json').write_bytes(raw)
                self.output = self.base / f'invalid-json-{index}'
                self.reject()
                self.assertEqual((self.output / 'result.json').read_bytes(), raw)

    def test_synthetic_false_success_and_deadlines(self):
        for index, changes in enumerate((dict(passed=False), dict(duration_seconds=120),
                                         dict(display_pixels_verified=True), dict(ime_tested=True),
                                         dict(scope='capture-only'))):
            with self.subTest(changes=changes):
                original = self.read('result.json')
                self.change('result.json', **changes)
                self.output = self.base / f'result-failure-{index}'
                self.reject()
                self.write('result.json', original)

    def test_synthetic_stage_false_success_cleanup_and_deadlines(self):
        changes = (dict(passed=False), dict(exit=1), dict(marker_count=0), dict(marker_count=2),
                   dict(marker_observed_seconds=45.01), dict(duration_seconds=60),
                   dict(cleanup_incomplete=True), dict(report_sha256='f'*64),
                   dict(processes=[]), dict(processes=[dict(pid=1, reaped=False, group_empty=True)]),
                   dict(processes=[dict(pid=1, reaped=True, group_empty=False)]),
                   dict(session_path='/different/session'), dict(session_identity='wrong'),
                   dict(session_files={'different': 'a'*64}), dict(today='2026-11-07'))
        for index, change in enumerate(changes):
            with self.subTest(change=change):
                original = self.read('stage2-driver.json')
                self.change('stage2-driver.json', **change)
                self.output = self.base / f'stage-failure-{index}'
                self.reject()
                self.write('stage2-driver.json', original)

    def test_synthetic_exit_zero_force_stop_or_missing_run_returned(self):
        for index in range(3):
            report = r.report()
            if index == 0:
                report['run_error'] = 'forceStop'
            elif index == 1:
                report['events'] = [event for event in report['events'] if event['name'] != 'run_returned']
                for sequence, event in enumerate(report['events'], 1):
                    event['seq'] = sequence
            else:
                report['writer_drained'] = False
            self.write('stage1.json', report)
            self.change('stage1-driver.json', report_sha256=hashlib.sha256((self.root/'stage1.json').read_bytes()).hexdigest())
            self.output = self.base / f'report-failure-{index}'
            self.reject()

    def test_synthetic_capture_only_or_duplicate_stdout_marker(self):
        for index, raw in enumerate((b'captured frame\n', r.m.MARKER+b'\n'+r.m.MARKER+b'\n', b'prefix '+r.m.MARKER+b'\n')):
            (self.root / 'stage1.stdout.txt').write_bytes(raw)
            self.output = self.base / f'marker-failure-{index}'
            self.reject()

    def test_synthetic_failed_workflow_stages(self):
        for name in self.stages:
            for state in ('failure', 'cancelled', 'skipped'):
                with self.subTest(name=name, state=state):
                    self.stages[name] = state
                    self.output = self.base / f'workflow-{name}-{state}'
                    self.reject()
            self.stages[name] = 'success'

    def test_synthetic_early_failure_retains_raw_logs_without_passing(self):
        for path in list(self.root.iterdir()):
            if path.is_file() and path.suffix == '.json':
                path.unlink()
        self.stages['smoke'] = 'failure'
        self.reject()
        self.assertEqual((self.output/'stage1.stderr.txt').read_text(), 'synthetic stderr\n')
        self.assertEqual((self.output/'infra-stage1/dbus.stderr.txt').read_text(), 'synthetic infrastructure log\n')

    def test_synthetic_overbudget_raw_prefix_retained_and_gate_rejects(self):
        name = 'stage1.stderr.txt'
        limit = m.FILES[name]
        (self.root/name).write_bytes(b'x' * (limit + 1))
        record = self.collect()
        self.assertIn(name, record['truncated'])
        self.assertEqual((self.output/name).read_bytes(), b'x' * limit)
        with self.assertRaises(ValueError):
            m.gate(self.output, self.expected, self.artifact)

    def test_synthetic_colluding_manifest_and_result_cannot_replace_binary_digest(self):
        self.change('manifest.json', sha256='d'*64)
        self.change('result.json', binary_sha256='d'*64)
        self.reject()

    def test_synthetic_artifact_tampering_preserves_logs_and_fails(self):
        binary = self.artifact / r.m.smoke.filename(self.expected['target'])
        binary.write_bytes(binary.read_bytes() + b'tampered')
        self.reject()
        self.assertEqual((self.output/'stage1.stderr.txt').read_text(), 'synthetic stderr\n')

    def test_synthetic_post_collection_tampering_fails_independent_gate(self):
        self.collect()
        (self.output/'stage1.stdout.txt').write_text('tampered\n')
        with self.assertRaises(ValueError):
            m.gate(self.output, self.expected, self.artifact)


if __name__ == '__main__':
    unittest.main()
