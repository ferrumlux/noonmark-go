import copy
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location('window_run', Path(__file__).with_name('run.py'))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


def report():
    names = m.BEFORE_CLOSE + ['close_attempt'] * 2 + m.AFTER_CLOSE
    return dict(version=1, scope='lifecycle-only', stage='create', today='2026-11-06', window_id=1,
                window_count=1, events=[dict(seq=i+1, name=n, elapsed_ns=i*100) for i, n in enumerate(names)],
                visible=True, native_handle_present=True, content_width=1200, content_height=768,
                native_frame_submitted=True, display_pixels_verified=False, run_error='',
                writer_drained=True, dispatcher_drained=True, service_close_error='',
                controller_stopped=True, errors=[], complete=True)


class SchemaTests(unittest.TestCase):
    def test_valid_multiple_guard_attempts(self):
        m.validate_report(report(), 'create')

    def test_every_missing_or_unknown_field(self):
        for key in m.REPORT_FIELDS:
            with self.subTest(key=key):
                data = report()
                del data[key]
                with self.assertRaises(ValueError):
                    m.validate_report(data, 'create')
        data = report()
        data['other'] = True
        with self.assertRaises(ValueError):
            m.validate_report(data, 'create')

    def test_cleanup_pixels_and_errors(self):
        changes = dict(complete=False, writer_drained=False, dispatcher_drained=False,
                       controller_stopped=False, run_error='forceStop', service_close_error='first close failed',
                       errors=['failure'], display_pixels_verified=True, window_count=2,
                       native_frame_submitted=False, visible=False, content_width=0, today='2026-02-30')
        for key, value in changes.items():
            with self.subTest(key=key):
                data = report()
                data[key] = value
                with self.assertRaises(ValueError):
                    m.validate_report(data, 'create')

    def test_event_missing_out_of_order_unknown_or_overflow(self):
        for i in range(len(report()['events'])):
            data = report()
            del data['events'][i]
            for j, event in enumerate(data['events']):
                event['seq'] = j+1
            # Deleting either of two close attempts is intentionally still valid.
            if i in (4, 5):
                m.validate_report(data, 'create')
            else:
                with self.assertRaises(ValueError):
                    m.validate_report(data, 'create')
        for value in (-1, float('nan'), float('inf'), True, 10.1):
            data = report()
            data['events'][-1]['elapsed_ns'] = value
            with self.assertRaises(ValueError):
                m.validate_report(data, 'create')
        data = report()
        data['events'][-1]['elapsed_ns'] = 10_000_001_000
        with self.assertRaisesRegex(ValueError, 'internal close'):
            m.validate_report(data, 'create')
        data = report()
        data['events'] = data['events'] * 8
        with self.assertRaises(ValueError):
            m.validate_report(data, 'create')

    def test_strict_json_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'report'
            valid = json.dumps(report())
            for text in ('', '{', valid + '{}', valid.replace('"version": 1', '"version": 1, "version": 1'),
                         valid.replace('"version": 1', '"version": NaN'), ' ' * (m.MAX_REPORT+1)):
                path.write_text(text)
                with self.assertRaises(ValueError):
                    m.read_report(path, 'create')
            path.write_text(valid)
            m.read_report(path, 'create')
            link = Path(tmp) / 'link'
            link.symlink_to(path)
            with self.assertRaises(ValueError):
                m.read_report(link, 'create')


class MarkerTests(unittest.TestCase):
    def test_split_bytes_and_crlf(self):
        for line in (m.MARKER+b'\n', m.MARKER+b'\r\n'):
            marker = m.Marker()
            for b in line:
                marker.feed(bytes([b]), 5)
            marker.finish()
            self.assertEqual(marker.observed, 5)

    def test_exact_line_not_substring(self):
        for line in (m.MARKER, b'x'+m.MARKER+b'\n', m.MARKER+b'x\n', b' '+m.MARKER+b'\n', m.MARKER+b'\r\r\n'):
            marker = m.Marker()
            marker.feed(line, 0)
            with self.assertRaises(ValueError):
                marker.finish()

    def test_duplicate_after_exit_buffer_fails(self):
        marker = m.Marker()
        marker.feed(m.MARKER+b'\n', 0)
        with self.assertRaisesRegex(ValueError, 'duplicate'):
            marker.feed(b'other log\n'+m.MARKER+b'\n', 1)


class IsolationTests(unittest.TestCase):
    def test_environment(self):
        with mock.patch.dict(os.environ, dict(MYGO_AUTOMATION='1', MYGO_GENERATE='1', MYGO_GPU='1',
                                             API_KEY='secret', DISPLAY=':0', EGL_PLATFORM='surfaceless',
                                             DBUS_SESSION_BUS_ADDRESS='existing')):
            env = m.isolated_env(Path('/private'))
        self.assertFalse(any(k.startswith('MYGO_') for k in env))
        self.assertFalse(any(k in env for k in ('API_KEY', 'DISPLAY', 'EGL_PLATFORM', 'DBUS_SESSION_BUS_ADDRESS')))
        self.assertEqual(env['HOME'], '/private')

    def test_no_external_session_or_report_arguments(self):
        for flag in ('--session', '--report', '--work-root', '--command', '--shell'):
            with self.assertRaises(SystemExit):
                m.parser().parse_args(['--artifact', '/a', '--target', 'linux/amd64', '--source', 'a'*40,
                                      '--run-id', '1', '--run-attempt', '1', '--consumer-attempt', '2', '--artifact-id', '456', '--work-parent', '/p', flag, '/x'])

    def test_links_and_inside_artifact(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            child = root / 'child'
            child.mkdir()
            with self.assertRaises(ValueError):
                m.work_parent(child, root)
            link = root / 'link'
            link.symlink_to(child, target_is_directory=True)
            with self.assertRaises(ValueError):
                m.clean_directory(link)

    def test_windows_fails_without_any_spawn(self):
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(m.subprocess, 'Popen') as spawn, mock.patch.object(m, 'windows_owner', side_effect=ValueError('Windows blocked: native APIs unavailable')):
            with self.assertRaisesRegex(ValueError, 'Windows blocked'):
                m.stage(Path('/not-executed'), Path(tmp), 1, 'windows/amd64', time.monotonic()+60)
            spawn.assert_not_called()

    def test_xvfb_missing_fails_without_spawn(self):
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(m.shutil, 'which', return_value=None), mock.patch.object(m.subprocess, 'Popen') as spawn:
            with self.assertRaisesRegex(ValueError, 'unavailable'):
                m.stage(Path('/not-executed'), Path(tmp), 1, 'linux/amd64', time.monotonic()+60)
            spawn.assert_not_called()

    def test_xauthority_private_no_cookie_in_argv(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'authority'
            m.xauthority(path, 21000)
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            self.assertIn(b'MIT-MAGIC-COOKIE-1', path.read_bytes())
            with self.assertRaises(FileExistsError):
                m.xauthority(path, 21000)


@unittest.skipIf(os.name == 'nt', 'POSIX ownership tests')
class ProcessTests(unittest.TestCase):
    def run_child(self, code, marker=None):
        owned = m.Owned()
        self.addCleanup(owned.cleanup, time.monotonic()+5)
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        child = owned.start([sys.executable, '-c', code], root, m.isolated_env(root), root/'out', root/'err', marker)
        return owned, child

    def test_exit_zero_without_marker_fails(self):
        marker = m.Marker()
        owned, child = self.run_child('pass', marker)
        while child.poll() is None or not owned.pipes_finished(child):
            owned.pump(time.monotonic()+2)
        with self.assertRaises(ValueError):
            marker.finish()

    def test_stderr_does_not_count(self):
        marker = m.Marker()
        owned, child = self.run_child('import sys; print("NOONMARK_NATIVE_WINDOW_READY_V1", file=sys.stderr)', marker)
        while child.poll() is None or not owned.pipes_finished(child):
            owned.pump(time.monotonic()+2)
        with self.assertRaises(ValueError):
            marker.finish()

    def test_timeout_is_irreversible_even_after_clean_exit(self):
        owned, child = self.run_child('import time; time.sleep(10)')
        with self.assertRaises(ValueError) as error:
            owned.pump(time.monotonic()-1)
        owned.fail(error.exception)
        children = owned.cleanup(time.monotonic()+2)
        self.assertTrue(children[0]['reaped'])
        self.assertTrue(children[0]['group_empty'])
        self.assertIsNotNone(owned.failed)
        # Remove cleanup registered for already-closed descriptor owner.
        self._cleanups = [c for c in self._cleanups if c[0] != owned.cleanup]

    def test_cleanup_does_not_touch_unrelated_process(self):
        unrelated = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(10)'], start_new_session=True)
        try:
            owned, child = self.run_child('import time; time.sleep(10)')
            children = owned.cleanup(time.monotonic()+2)
            self._cleanups = [c for c in self._cleanups if c[0] != owned.cleanup]
            self.assertTrue(children[0]['reaped'])
            self.assertIsNone(unrelated.poll())
        finally:
            unrelated.terminate()
            unrelated.wait(timeout=2)

    def test_excess_output_fails(self):
        owned, child = self.run_child('import sys; sys.stdout.write("x"*1100000)')
        with self.assertRaisesRegex(ValueError, 'excessive|line limit'):
            while True:
                owned.pump(time.monotonic()+2)


class WorkflowTests(unittest.TestCase):
    def test_paths_must_be_lexically_clean(self):
        for raw in ('relative', '/tmp/../tmp', '/tmp//x', '/tmp/x/'):
            with self.assertRaises(ValueError):
                m.absolute_argument(raw)

    def test_negative_first_event_time(self):
        data = report()
        data['events'][0]['elapsed_ns'] = -1
        with self.assertRaises(ValueError):
            m.validate_report(data, 'create')

    def test_linux_fixed_argv_ownership_and_no_inherited_bus(self):
        with tempfile.TemporaryDirectory() as tmp:
            owned = mock.Mock()
            x = mock.Mock()
            x.poll.return_value = None
            bus = mock.Mock()
            bus.poll.return_value = None
            bus.stdout.fileno.return_value = 88
            owned.start.side_effect = [x, bus]
            owned.line.side_effect = [b'21000', b'unix:path=/tmp/owned-bus,guid=' + b'a'*32]
            env = {}
            with mock.patch.object(m.shutil, 'which', side_effect=['/usr/bin/Xvfb', '/usr/bin/dbus-daemon']), mock.patch.object(m.secrets, 'randbelow', return_value=1000):
                m.linux_infra(owned, Path(tmp), env, time.monotonic()+5)
            args = owned.start.call_args_list[0].args[0]
            self.assertEqual(args[:2], ['/usr/bin/Xvfb', ':21000'])
            self.assertIn('-displayfd', args)
            self.assertIn('-auth', args)
            self.assertNotIn('-ac', args)
            self.assertEqual(owned.start.call_args_list[1].args[0], ['/usr/bin/dbus-daemon', '--session', '--nofork', '--nopidfile', '--print-address=1'])
            self.assertEqual(env['DISPLAY'], ':21000')
            self.assertTrue(env['DBUS_SESSION_BUS_ADDRESS'].startswith('unix:'))
            # Mock owner does not hold the real readiness read fd; close it here.
            os.close(owned.watch_fd.call_args.args[0])

    def test_reopen_same_path_and_no_second_run_after_failure(self):
        with tempfile.TemporaryDirectory() as tmp:
            parent = Path(tmp)
            artifact = parent/'artifact'
            artifact.mkdir()
            (artifact/'manifest.json').write_text('{}')
            exe = artifact/'binary'
            exe.write_text('synthetic')
            work = parent/'work'
            work.mkdir()
            args = m.parser().parse_args(['--artifact', str(artifact), '--target', 'linux/amd64',
                                         '--source', 'a'*40, '--run-id', '1', '--run-attempt', '1', '--consumer-attempt', '2', '--artifact-id', '456', '--work-parent', str(work)])
            def synthetic_stage(exe, root, number, target, total, first_start=None):
                return dict(today='2026-11-06', session_path=str(root/'session'), session_files={'fixed': 'hash'})
            with mock.patch.object(m.smoke, 'verify', return_value=exe), mock.patch.object(m.smoke, 'native_host', return_value='linux/amd64'), mock.patch.object(m, 'stage', side_effect=synthetic_stage) as stages:
                self.assertEqual(m.run(args), 0)
                self.assertEqual(stages.call_count, 2)
                result = json.loads((next(work.iterdir()) / 'result.json').read_text())
                self.assertEqual((result['run_attempt'], result['build_attempt'], result['consumer_attempt'], result['artifact_id']), ('1', '1', '2', '456'))
                self.assertEqual(stages.call_args_list[0].args[1], stages.call_args_list[1].args[1])
                self.assertEqual([call.args[2] for call in stages.call_args_list], [1, 2])
            with mock.patch.object(m.smoke, 'verify', return_value=exe), mock.patch.object(m.smoke, 'native_host', return_value='linux/amd64'), mock.patch.object(m, 'stage', side_effect=ValueError('first failure')) as stages:
                self.assertEqual(m.run(args), 1)
                self.assertEqual(stages.call_count, 1)

    def test_date_boundary_and_session_mutation_fail(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            artifact = root/'artifact'
            artifact.mkdir()
            (artifact/'manifest.json').write_text('{}')
            exe = artifact/'binary'
            exe.write_text('synthetic')
            work = root/'work'
            work.mkdir()
            args = m.parser().parse_args(['--artifact', str(artifact), '--target', 'linux/amd64', '--source', 'a'*40,
                                         '--run-id', '1', '--run-attempt', '1', '--consumer-attempt', '2', '--artifact-id', '456', '--work-parent', str(work)])
            first = dict(today='2026-11-06', session_path='/same/session', session_files={'fixed':'hash'})
            for second in (dict(first, today='2026-11-07'), dict(first, session_files={'fixed':'changed'}), dict(first, session_path='/other/session')):
                with mock.patch.object(m.smoke, 'verify', return_value=exe), mock.patch.object(m.smoke, 'native_host', return_value='linux/amd64'), mock.patch.object(m, 'stage', side_effect=[first, second]):
                    self.assertEqual(m.run(args), 1)

class ConsumerIdentityTests(unittest.TestCase):
    def test_synthetic_invalid_identity_stops_before_filesystem_or_execution(self):
        args = m.parser().parse_args(['--artifact', '/synthetic/artifact', '--work-parent', '/synthetic/parent',
                                     '--target', 'linux/amd64', '--source', 'a'*40, '--run-id', '123',
                                     '--run-attempt', '2', '--consumer-attempt', '3', '--artifact-id', '456'])
        for key, bad in (('consumer_attempt', '1'), ('consumer_attempt', '0'), ('consumer_attempt', 'x'),
                         ('artifact_id', ''), ('artifact_id', '0'), ('artifact_id', 'name-not-ID')):
            with self.subTest(key=key, bad=bad):
                altered = copy.copy(args)
                setattr(altered, key, bad)
                with mock.patch.object(m, 'clean_directory') as filesystem, mock.patch.object(m, 'stage') as stage:
                    with self.assertRaises(ValueError):
                        m.run(altered)
                    filesystem.assert_not_called()
                    stage.assert_not_called()


if __name__ == '__main__':
    unittest.main()
