"""Deterministic driver observations only; not native lifecycle evidence."""
import copy
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import test_run
import test_collect
import test_windows_job

m = test_run.m
c = test_collect.m


class Clock:
    def __init__(self):
        self.now = 100.0

    def monotonic(self):
        return self.now


class Child:
    pid = 1234
    returncode = None

    def poll(self):
        return self.returncode


class FakeOwned:
    """No OS child, signals, time sleeps, or external state."""
    def __init__(self, clock, mode):
        self.clock, self.mode = clock, mode
        self.children, self.streams, self.diagnostic_times = [], {}, {}
        self.failed = self.cleanup_failed = None
        self.cleanup_called = False
        self.child = Child()

    def fail(self, error):
        if self.failed is None:
            self.failed = str(error)

    def start(self, argv, root, env, out, err, marker):
        self.root, self.marker = root, marker
        self.report = Path(argv[-1])
        self.report.write_bytes(b'')
        self.children = [dict(process=self.child, pid=1234, pgid=1234, reaped=False, group_empty=False, exit=None)]
        self.diagnostic_times = {1234: dict(spawn_observed=self.clock.now, term_sent=None, kill_sent=None)}
        self.streams = {i: dict(pid=1234, kind=kind, size=0, eof=False) for i, kind in enumerate(('stdout', 'stderr'))}
        return self.child

    def ready(self):
        self.streams[0]['size'] += len(m.MARKER) + 1
        self.marker.feed(m.MARKER+b'\n', self.clock.now)

    def pump(self, deadline):
        if self.mode in ('no_ready', 'late_ready') or self.marker.observed is not None:
            self.clock.now = deadline
            m.check_deadline(deadline)
        self.clock.now += 2
        self.ready()
        if self.mode == 'duplicate':
            self.ready()
        if self.mode not in ('ready_hang', 'cleanup_duplicate', 'hang_cleanup_throws'):
            self.child.returncode = 0 if self.mode != 'nonzero' else 7
            self.report.write_text(json.dumps(test_run.report()))
            for stream in self.streams.values():
                stream['eof'] = True

    def pipes_finished(self, child):
        return all(s['eof'] for s in self.streams.values())

    def cleanup(self, deadline):
        self.cleanup_called = True
        self.clock.now += .1
        self.diagnostic_times[1234]['term_sent'] = self.clock.now
        if self.mode == 'late_ready':
            self.ready()
            self.report.write_text(json.dumps(test_run.report()))
        if self.mode == 'cleanup_duplicate':
            try:
                self.ready()
            except ValueError as error:
                self.cleanup_failed = str(error)
                self.fail(error)
        self.clock.now += .5
        self.diagnostic_times[1234]['kill_sent'] = self.clock.now
        self.child.returncode = 0 if self.mode == 'late_ready' else self.child.returncode if self.child.returncode is not None else -9
        self.children[0].update(exit=self.child.returncode, reaped=True, group_empty=True)
        for stream in self.streams.values():
            stream['eof'] = True
        if self.mode in ('cleanup_throws', 'hang_cleanup_throws'):
            raise OSError('cleanup failed ' + 'x'*3000)
        if self.mode == 'cleanup_failure':
            self.cleanup_failed = 'cleanup-incomplete: pipes'
            self.fail(self.cleanup_failed)
        if self.mode == 'cleanup_deadline':
            self.clock.now = deadline
        return [{k: v for k, v in e.items() if k != 'process'} for e in self.children]


class DiagnosticStageTests(unittest.TestCase):
    def execute(self, mode, failure=True, patch=None, clock=None):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        root = Path(temp.name)
        clock = Clock() if clock is None else clock
        owner = FakeOwned(clock, mode)
        with mock.patch.object(m.time, 'monotonic', clock.monotonic), mock.patch.object(m, 'Owned', return_value=owner), \
                mock.patch.object(m, 'snapshot', return_value={'fixed': 'a'*64}):
            with patch if patch else mock.patch.object(m, 'MAX_REPORT', m.MAX_REPORT):
                if failure:
                    with self.assertRaises((ValueError, OSError)):
                        m.stage(Path('/never-executed'), root, 1, 'darwin/amd64', 220)
                else:
                    m.stage(Path('/never-executed'), root, 1, 'darwin/amd64', 220)
        self.assertTrue(owner.cleanup_called)
        record = json.loads((root/'stage1-driver.json').read_text())
        self.assertEqual(record['passed'], not failure)
        self.assertLess(len(json.dumps(record['diagnostics']).encode()), 16384)
        return record

    def test_nan_deadline_preserves_original_fail_closed_predicate(self):
        with self.assertRaises(m.DeadlineExceeded):
            m.check_deadline(float('nan'))

    def test_report_os_timeout_does_not_claim_completion_watchdog(self):
        clock = Clock()
        def read_report(*args):
            clock.now = 115
            raise TimeoutError('synthetic filesystem ETIMEDOUT, not a watchdog')
        r = self.execute('success', clock=clock, patch=mock.patch.object(m, 'read_report', read_report))
        self.assertEqual(r['diagnostics']['failure']['code'], 'report_validation')
        self.assertIsNone(r['diagnostics']['failure']['deadline_seconds'])
        self.assertEqual(r['failure'], 'synthetic filesystem ETIMEDOUT, not a watchdog')

    def test_trusted_watchdog_preserves_its_actual_deadline(self):
        with mock.patch.object(m.time, 'monotonic', return_value=160):
            with self.assertRaises(m.DeadlineExceeded) as caught:
                m.check_deadline(155)
        self.assertEqual(caught.exception.deadline, 155)
        self.assertEqual(str(caught.exception), 'software deadline exceeded')

    def test_real_windows_setup_deadline_is_unknown_not_guessed_ready(self):
        clock = Clock()
        class SlowAPI(test_windows_job.FakeAPI):
            def create_job(self):
                job = super().create_job()
                clock.now = 160.0
                return job
        api = SlowAPI()
        adapter = test_windows_job.adapter()
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(m.time, 'monotonic', clock.monotonic), \
                mock.patch.object(m, 'windows_owner', side_effect=lambda deadline: adapter.WindowsOwned(api=api, deadline=deadline)):
            root = Path(tmp)
            with self.assertRaisesRegex(ValueError, 'Windows software deadline exceeded'):
                m.stage(Path('/never-executed'), root, 1, 'windows/amd64', 220)
            r = json.loads((root/'stage1-driver.json').read_text())
        # Actual WindowsOwned.start/check used its own 60s stage deadline.
        # The unchanged adapter raises an untyped TimeoutError, so the driver
        # cannot attribute it to READY45 or any other watchdog from its text.
        self.assertFalse(r['passed'])
        self.assertEqual(r['diagnostics']['failure'], dict(code='setup', phase='setup', at_seconds=60, deadline_seconds=None))
        self.assertEqual(r['failure'], 'Windows software deadline exceeded')
        self.assertFalse(api.live)
        self.assertEqual(api.foreign, {'unrelated-process', 'unrelated-job'})

    def test_windows_os_errors_after_ready_budget_are_not_watchdogs(self):
        for error in (TimeoutError('OS timed out'), TimeoutError('Windows software deadline exceeded'), OSError('software deadline exceeded')):
            with self.subTest(error=repr(error)):
                clock = Clock()
                class ErrorAPI(test_windows_job.FakeAPI):
                    def create_job(self):
                        clock.now = 146.0
                        raise error
                api = ErrorAPI(); adapter = test_windows_job.adapter()
                with tempfile.TemporaryDirectory() as tmp, mock.patch.object(m.time, 'monotonic', clock.monotonic), \
                        mock.patch.object(m, 'windows_owner', side_effect=lambda deadline: adapter.WindowsOwned(api=api, deadline=deadline)):
                    root = Path(tmp)
                    with self.assertRaisesRegex(ValueError, str(error)):
                        m.stage(Path('/never-executed'), root, 1, 'windows/amd64', 220)
                    r = json.loads((root/'stage1-driver.json').read_text())
                self.assertFalse(r['passed'])
                self.assertEqual(r['diagnostics']['failure'], dict(code='setup', phase='setup', at_seconds=46, deadline_seconds=None))
                self.assertFalse(api.live)

    def test_general_completion_timeout_does_not_claim_watchdog(self):
        original = FakeOwned.pump
        for error in (TimeoutError('software deadline exceeded'), OSError('OS timed out')):
            with self.subTest(error=repr(error)):
                def pump(owner, deadline):
                    if owner.marker.observed is None:
                        return original(owner, deadline)
                    owner.clock.now = deadline
                    raise error
                r = self.execute('ready_hang', patch=mock.patch.object(FakeOwned, 'pump', pump))
                self.assertEqual(r['failure'], str(error))
                self.assertEqual(r['diagnostics']['failure']['code'], 'completion_wait')
                self.assertIsNone(r['diagnostics']['failure']['deadline_seconds'])

    def test_no_ready_hang_original_deadline_and_unknowns(self):
        r = self.execute('no_ready'); d = r['diagnostics']; f = d['failure_snapshot']
        self.assertEqual(d['deadlines_seconds'], dict(ready=45, completion_limit=55, cleanup=60))
        self.assertEqual(d['failure'], dict(code='ready_watchdog', phase='ready_wait', at_seconds=45, deadline_seconds=45))
        self.assertEqual((f['marker_count'], f['marker_first_seconds']), (0, None))
        self.assertEqual(f['processes'][0]['poll_source'], 'live')
        self.assertIsNone(f['processes'][0]['poll_exit'])
        self.assertEqual(d['after_cleanup_snapshot']['processes'][0]['poll_exit'], -9)
        self.assertIsNone(r['marker_observed_seconds'])
        self.assertEqual(f['report'], dict(state='plain', bytes=0))
        self.assertFalse(f['pipes'][0]['eof'])
        self.assertTrue(d['after_cleanup_snapshot']['pipes'][0]['eof'])

    def test_ready_hang_keeps_first_observation_and_completion_deadline(self):
        r = self.execute('ready_hang'); d = r['diagnostics']
        self.assertEqual(r['marker_observed_seconds'], 2)
        self.assertEqual(d['failure']['code'], 'completion_watchdog')
        self.assertEqual(d['failure']['deadline_seconds'], 12)
        self.assertEqual(d['failure_snapshot']['marker_first_seconds'], 2)
        self.assertEqual(d['failure_snapshot']['marker_count'], 1)
        self.assertLess(d['failure']['at_seconds'], d['process_times'][0]['term_sent_seconds'])
        self.assertLess(d['process_times'][0]['term_sent_seconds'], d['process_times'][0]['kill_sent_seconds'])

    def test_late_ready_and_complete_report_do_not_undo_failure(self):
        r = self.execute('late_ready'); d = r['diagnostics']
        self.assertEqual(r['failure'], 'software deadline exceeded')
        self.assertEqual(d['failure_snapshot']['marker_count'], 0)
        self.assertIsNone(d['failure_snapshot']['marker_first_seconds'])
        self.assertEqual(d['failure_snapshot']['report']['bytes'], 0)
        after = d['after_cleanup_snapshot']
        self.assertEqual(after['marker_count'], 1)
        self.assertGreater(after['marker_first_seconds'], d['failure']['at_seconds'])
        self.assertGreater(after['report']['bytes'], 0)
        self.assertEqual(after['processes'][0]['poll_exit'], 0)
        self.assertEqual(r['marker_count'], 1)

    def test_duplicate_is_original_failure_and_keeps_first_time(self):
        r = self.execute('duplicate'); d = r['diagnostics']
        self.assertEqual(r['failure'], 'duplicate READY marker')
        self.assertEqual(d['failure_snapshot']['marker_count'], 2)
        self.assertEqual(d['failure_snapshot']['marker_first_seconds'], 2)
        self.assertIsNone(d['failure']['deadline_seconds'])

    def test_cleanup_duplicate_cannot_replace_original_watchdog(self):
        r = self.execute('cleanup_duplicate'); d = r['diagnostics']
        self.assertEqual(r['failure'], 'software deadline exceeded')
        self.assertEqual(d['failure']['code'], 'completion_watchdog')
        self.assertEqual(d['cleanup_error'], 'duplicate READY marker')
        self.assertEqual(d['failure_snapshot']['marker_count'], 1)
        self.assertEqual(d['after_cleanup_snapshot']['marker_count'], 2)

    def test_cleanup_failure_and_exception_are_red_and_bounded(self):
        for mode in ('cleanup_failure', 'cleanup_throws', 'cleanup_deadline'):
            with self.subTest(mode=mode):
                d = self.execute(mode)['diagnostics']
                self.assertEqual(d['failure']['code'], 'cleanup')
                self.assertLessEqual(len(d['cleanup_error'] or ''), 512)
                self.assertIsNotNone(d['after_cleanup_snapshot'])

    def test_cleanup_exception_never_overwrites_original_timeout(self):
        r = self.execute('hang_cleanup_throws'); d = r['diagnostics']
        self.assertEqual(r['failure'], 'software deadline exceeded')
        self.assertEqual(d['failure']['code'], 'completion_watchdog')
        self.assertIsNone(d['failure_snapshot']['processes'][0]['poll_exit'])
        self.assertEqual(d['after_cleanup_snapshot']['processes'][0]['poll_exit'], -9)
        self.assertEqual(len(d['cleanup_error']), 512)

    def test_observation_error_preserves_original_failure_and_cleanup(self):
        patch = mock.patch.object(m.StageDiagnostics, 'snapshot', side_effect=RuntimeError('diagnostic exception'))
        r = self.execute('ready_hang', patch=patch)
        self.assertEqual(r['failure'], 'software deadline exceeded')
        self.assertEqual(r['diagnostics']['failure']['code'], 'completion_watchdog')
        self.assertIsNone(r['diagnostics']['failure_snapshot'])
        self.assertEqual(r['diagnostics']['observation_errors'], ['snapshot'])

    def test_diagnostic_error_after_success_fails_closed(self):
        patch = mock.patch.object(m.StageDiagnostics, 'snapshot', side_effect=RuntimeError('diagnostic exception'))
        r = self.execute('success', patch=patch)
        self.assertEqual(r['diagnostics']['failure']['code'], 'diagnostic')

    def test_report_session_nonzero_stable_codes(self):
        for mode, patch, code in (
            ('nonzero', None, 'completion_validation'),
            ('success', mock.patch.object(m, 'read_report', side_effect=ValueError('bad report')), 'report_validation'),
            ('success', mock.patch.object(m, 'snapshot', side_effect=ValueError('bad session')), 'session_snapshot')):
            with self.subTest(code=code):
                self.assertEqual(self.execute(mode, patch=patch)['diagnostics']['failure']['code'], code)

    def test_success_records_no_failure(self):
        r = self.execute('success', failure=False)
        self.assertIsNone(r['diagnostics']['failure'])
        self.assertIsNone(r['diagnostics']['failure_snapshot'])
        self.assertEqual(r['diagnostics']['observation_errors'], [])
        c.validate_diagnostics(r['diagnostics'], r, 'darwin/amd64',
                               r['diagnostics']['after_cleanup_snapshot']['report']['bytes'])
        self.assertEqual(r['diagnostics']['after_cleanup_snapshot']['processes'][0]['poll_source'], 'cached')
        self.assertIsNone(r['diagnostics']['after_cleanup_snapshot']['processes'][0]['poll_seconds'])

    def test_poll_failure_stays_unknown_not_final_exit(self):
        clock = Clock(); owner = FakeOwned(clock, 'ready_hang')
        owner.children = [dict(process=Child(), pid=1234, pgid=1234)]
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(Child, 'poll', side_effect=OSError('poll')):
            d = m.StageDiagnostics(100, 145, 155, 160)
            snap = d.snapshot(owner, owner.children[0]['process'], m.Marker(), Path(tmp)/'missing')
        self.assertEqual(snap['processes'][0]['poll_source'], 'unknown')
        self.assertIsNone(snap['processes'][0]['poll_exit'])
        self.assertEqual(d.data['observation_errors'], ['process_poll'])
        self.assertEqual(snap['report'], dict(state='missing', bytes=None))

    def test_report_stat_never_reads_link_fifo_or_large_contents(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); regular = root/'report'; regular.write_bytes(b'x'*70000)
            paths = [(regular, 'plain', 70000), (root/'missing', 'missing', None)]
            link = root/'link'; link.symlink_to(regular); paths.append((link, 'unsafe', None))
            hard = root/'hard'; os.link(regular, hard); paths += [(hard, 'unsafe', None), (regular, 'unsafe', None)]
            # Remove hard link before checking the initially safe regular file.
            d = m.StageDiagnostics(0, 45, 55, 60)
            self.assertEqual(d.snapshot(None, None, m.Marker(), hard)['report'], dict(state='unsafe', bytes=None))
            hard.unlink(); paths = paths[:3]
            if hasattr(os, 'mkfifo'):
                fifo = root/'fifo'; os.mkfifo(fifo); paths.append((fifo, 'unsafe', None))
            d = m.StageDiagnostics(0, 45, 55, 60)
            with mock.patch.object(Path, 'read_bytes', side_effect=AssertionError('no content read')):
                for path, state, size in paths:
                    self.assertEqual(d.snapshot(None, None, m.Marker(), path)['report'], dict(state=state, bytes=size))


class DiagnosticCollectorTests(unittest.TestCase):
    def setUp(self):
        self.fixture = test_collect.SyntheticCollectionTests(methodName='runTest')
        self.fixture.setUp()
        self.addCleanup(self.fixture.doCleanups)
        for n in (1, 2):
            r = self.fixture.read(f'stage{n}-driver.json')
            processes = r['processes']
            r['diagnostics'] = dict(version=1, deadlines_seconds=dict(ready=45, completion_limit=55, cleanup=60),
                failure=None, failure_snapshot=None, cleanup_started_seconds=.3, cleanup_ended_seconds=.4,
                cleanup_error=None, observation_errors=[], process_times=[dict(pid=p['pid'], spawn_observed_seconds=.01,
                term_sent_seconds=None, kill_sent_seconds=None) for p in processes],
                after_cleanup_snapshot=dict(at_seconds=.45, marker_count=1, marker_first_seconds=.1,
                report=dict(state='plain', bytes=(self.fixture.root/f'stage{n}.json').stat().st_size),
                processes=[dict(pid=p['pid'], pgid=p['pgid'], is_app=i==2, poll_source='cached', poll_exit=0, poll_seconds=None)
                           for i, p in enumerate(processes)],
                pipes=[dict(pid=p['pid'], kind=kind, bytes=10, eof=True) for p in processes for kind in ('stdout','stderr')]
                      + [dict(pid=None, kind='helper_ready', bytes=5, eof=True)]))
            self.fixture.write(f'stage{n}-driver.json', r)

    def test_extension_passes_only_with_original_gate(self):
        self.fixture.collect()
        c.gate(self.fixture.output, self.fixture.expected, self.fixture.artifact)

    def test_windows_extension_with_unknown_signal_times(self):
        self.fixture.expected['target'] = 'windows/amd64'
        self.fixture.fixture()
        import shutil
        for n in (1, 2):
            shutil.rmtree(self.fixture.root/f'infra-stage{n}')
            r = self.fixture.read(f'stage{n}-driver.json')
            pid = 1000+n
            r['processes'] = [dict(pid=pid, assigned_to_job=True, resumed=True, reaped=True,
                                  job_empty=True, exit=0, cleanup_incomplete=False)]
            r['diagnostics'] = dict(version=1, deadlines_seconds=dict(ready=45, completion_limit=55, cleanup=60),
                failure=None, failure_snapshot=None, cleanup_started_seconds=.3, cleanup_ended_seconds=.4,
                cleanup_error=None, observation_errors=[], process_times=[dict(pid=pid, spawn_observed_seconds=.01,
                term_sent_seconds=None, kill_sent_seconds=None)],
                after_cleanup_snapshot=dict(at_seconds=.45, marker_count=1, marker_first_seconds=.1,
                report=dict(state='plain', bytes=(self.fixture.root/f'stage{n}.json').stat().st_size),
                processes=[dict(pid=pid, pgid=None, is_app=True, poll_source='cached', poll_exit=0, poll_seconds=None)],
                pipes=[dict(pid=pid, kind=kind, bytes=10, eof=True) for kind in ('stdout','stderr')]))
            self.fixture.write(f'stage{n}-driver.json', r)
        self.fixture.stages.pop('libraries')
        self.fixture.collect()
        c.gate(self.fixture.output, self.fixture.expected, self.fixture.artifact)

    def test_all_nested_unknown_or_missing_fields_rejected(self):
        original = self.fixture.read('stage1-driver.json')
        paths = [(), ('deadlines_seconds',), ('process_times',0), ('after_cleanup_snapshot',),
                 ('after_cleanup_snapshot','processes',0), ('after_cleanup_snapshot','pipes',0), ('after_cleanup_snapshot','report')]
        for index, path in enumerate(paths):
            for kind in ('extra', 'missing'):
                changed = copy.deepcopy(original); target = changed['diagnostics']
                for key in path: target = target[key]
                if kind == 'extra': target['surprise'] = 1
                else: del target[next(iter(target))]
                self.fixture.write('stage1-driver.json', changed)
                self.fixture.output = self.fixture.base / f'bad-fields-{index}-{kind}'
                self.fixture.reject()

    def test_failure_late_marker_nonfinite_and_bounds_rejected(self):
        original = self.fixture.read('stage1-driver.json')
        changes = [(('version',), True), (('version',), 2), (('failure',), {'code':'completion_watchdog'}),
                   (('failure_snapshot',), {}), (('cleanup_error',), 'failed'), (('observation_errors',), ['snapshot']),
                   (('cleanup_ended_seconds',), float('inf')), (('cleanup_started_seconds',), -.1),
                   (('deadlines_seconds','ready'), 46), (('process_times',0,'spawn_observed_seconds'), None),
                   (('process_times',0,'term_sent_seconds'), .1), (('after_cleanup_snapshot','marker_first_seconds'), .49),
                   (('after_cleanup_snapshot','processes',0,'poll_source'), 'live'),
                   (('after_cleanup_snapshot','pipes',0,'bytes'), m.MAX_LOG+1),
                   (('after_cleanup_snapshot','pipes',0,'eof'), False), (('after_cleanup_snapshot','report','bytes'), 0)]
        for index, (path, value) in enumerate(changes):
            changed = copy.deepcopy(original); target = changed['diagnostics']
            for key in path[:-1]: target = target[key]
            target[path[-1]] = value
            self.fixture.write('stage1-driver.json', changed)
            self.fixture.output = self.fixture.base / f'bad-value-{index}'
            self.fixture.reject()

    def test_diagnostics_never_bypass_failed_original_report_or_provenance(self):
        for index, (name, changes) in enumerate((('stage1-driver.json',dict(passed=False)),
                     ('result.json',dict(source='b'*40)), ('stage1.json',dict(complete=False)))):
            old = self.fixture.read(name)
            self.fixture.change(name, **changes)
            self.fixture.output = self.fixture.base / f'original-failure-{index}'
            self.fixture.reject()
            self.fixture.write(name, old)


if __name__ == '__main__':
    unittest.main()
