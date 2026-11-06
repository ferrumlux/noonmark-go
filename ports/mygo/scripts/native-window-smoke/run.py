#!/usr/bin/env python3
"""Isolated two-process native Day lifecycle gate. No pixels, input or builds.

Budgets are software watchdogs, not hard OS quotas. Synchronous OS/file calls
can stall; an outer job timeout must treat a missing result as failure/unknown.
Windows uses a suspended Job/handle-list adapter; actual Windows host verification remains pending.
"""
import argparse
import datetime
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import secrets
import selectors
import shutil
import signal
import socket
import stat
import struct
import subprocess
import sys
import tempfile
import time

_spec = importlib.util.spec_from_file_location('native_smoke', Path(__file__).resolve().parents[1] / 'native-smoke' / 'smoke.py')
smoke = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(smoke)
require = smoke.require
MARKER = b'NOONMARK_NATIVE_WINDOW_READY_V1'
MAX_LOG = 1 << 20
MAX_REPORT = 64 << 10
REPORT_FIELDS = {'version', 'scope', 'stage', 'today', 'window_id', 'window_count', 'events',
                 'visible', 'native_handle_present', 'content_width', 'content_height',
                 'native_frame_submitted', 'display_pixels_verified', 'run_error',
                 'writer_drained', 'dispatcher_drained', 'service_close_error',
                 'controller_stopped', 'errors', 'complete'}
BEFORE_CLOSE = ['created', 'ready', 'visibility_checked', 'probe_close_requested']
AFTER_CLOSE = ['closed', 'run_returned', 'writer_drained', 'dispatcher_drained', 'service_close_result', 'finalized']


def check_deadline(deadline):
    require(time.monotonic() < deadline, 'software deadline exceeded')


def clean_directory(path):
    require(path.is_absolute() and str(path) == os.path.normpath(str(path)), 'clean absolute directory required')
    for part in [path, *path.parents]:
        smoke.directory(part)
    return path


def work_parent(path, artifact):
    clean_directory(path)
    repo = Path(__file__).resolve().parents[4]
    for forbidden in (repo, artifact):
        require(path != forbidden and forbidden not in path.parents, 'work parent inside source/artifact')
    # Caller declares an external build/runtime root. Do not discover other trees.
    return path


def unique(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, 'duplicate JSON key')
        result[key] = value
    return result


def read_report(path, stage):
    require(0 < smoke.plain(path).st_size <= MAX_REPORT, 'report size')
    report = json.loads(path.read_bytes(), object_pairs_hook=unique,
                        parse_constant=lambda _: (_ for _ in ()).throw(ValueError('nonfinite JSON')))
    return validate_report(report, stage)


def validate_report(r, stage):
    require(type(r) is dict and set(r) == REPORT_FIELDS, 'report fields')
    require(type(r['version']) is int and r['version'] == 1 and r['scope'] == 'lifecycle-only' and r['stage'] == stage, 'report identity')
    require(type(r['today']) is str and re.fullmatch(r'\d{4}-\d{2}-\d{2}', r['today']), 'today format')
    datetime.date.fromisoformat(r['today'])
    for key in ('window_id', 'window_count', 'content_width', 'content_height'):
        require(type(r[key]) is int and r[key] > 0, key + ' type/value')
    require(r['window_count'] == 1, 'multiple windows')
    for key in ('visible', 'native_handle_present', 'native_frame_submitted', 'writer_drained',
                'dispatcher_drained', 'controller_stopped', 'complete'):
        require(r[key] is True, key + ' missing')
    require(r['display_pixels_verified'] is False, 'unsupported pixels claim')
    require(r['run_error'] == '' and r['service_close_error'] == '' and r['errors'] == [], 'application failure')
    events = r['events']
    require(type(events) is list and 11 <= len(events) <= 64, 'event count')
    last = -1
    for i, event in enumerate(events):
        require(type(event) is dict and set(event) == {'seq', 'name', 'elapsed_ns'}, 'event fields')
        require(type(event['seq']) is int and event['seq'] == i + 1, 'event sequence')
        require(type(event['name']) is str, 'event name')
        require(type(event['elapsed_ns']) is int and 0 <= event['elapsed_ns'] <= 60_000_000_000 and last <= event['elapsed_ns'], 'event duration')
        last = event['elapsed_ns']
    names = [event['name'] for event in events]
    attempts = len(names) - len(BEFORE_CLOSE) - len(AFTER_CLOSE)
    require(1 <= attempts <= 32 and names == BEFORE_CLOSE + ['close_attempt'] * attempts + AFTER_CLOSE, 'event order/cardinality')
    require(events[-1]['elapsed_ns'] - events[3]['elapsed_ns'] <= 10_000_000_000, 'internal close budget')
    return r


class Marker:
    """Byte-incremental, exact stdout lines, LF or a single CRLF only."""
    def __init__(self):
        self.buffer = b''
        self.count = 0
        self.observed = None

    def feed(self, data, now):
        self.buffer += data
        require(len(self.buffer) <= MAX_LOG, 'stdout line limit')
        while b'\n' in self.buffer:
            line, self.buffer = self.buffer.split(b'\n', 1)
            if line.endswith(b'\r'):
                line = line[:-1]
            if line == MARKER:
                self.count += 1
                require(self.count == 1, 'duplicate READY marker')
                self.observed = now

    def finish(self):
        # An unterminated final line cannot become a marker.
        require(self.count == 1, 'missing complete READY marker')


def isolated_env(root):
    env = {k: v for k, v in os.environ.items()
           if not any(s in k.upper() for s in ('TOKEN', 'SECRET', 'PASSWORD', 'API_KEY', 'CREDENTIAL'))
           and not k.upper().startswith('MYGO_')
           and k not in ('DISPLAY', 'XAUTHORITY', 'DBUS_SESSION_BUS_ADDRESS', 'EGL_PLATFORM')}
    env.update({k: str(root) for k in ('TMP', 'TEMP', 'TMPDIR', 'HOME', 'USERPROFILE', 'APPDATA',
                                     'LOCALAPPDATA', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_CACHE_HOME', 'XDG_RUNTIME_DIR')})
    return env


class Owned:
    """Foreground POSIX children, exact PIDs/PGIDs, nonblocking bounded pipes."""
    def __init__(self):
        self.selector = selectors.DefaultSelector()
        self.children = []
        self.streams = {}
        self.failed = None

    def fail(self, error):
        if self.failed is None:
            self.failed = str(error)

    def start(self, argv, cwd, env, stdout_path, stderr_path, marker=None, pass_fds=()):
        require(os.name != 'nt', 'Windows blocked: verified suspended Job/handle-list adapter unavailable')
        child = subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.DEVNULL,
                                 stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                 start_new_session=True, close_fds=True, pass_fds=pass_fds)
        entry = {'process': child, 'pid': child.pid, 'pgid': child.pid, 'reaped': False,
                 'group_empty': False, 'exit': None}
        self.children.append(entry)
        try:
            for stream, path, parse in ((child.stdout, stdout_path, marker), (child.stderr, stderr_path, None)):
                os.set_blocking(stream.fileno(), False)
                output = path.open('xb')
                self.streams[stream.fileno()] = {'pipe': stream, 'output': output, 'size': 0, 'marker': parse, 'data': bytearray()}
                self.selector.register(stream, selectors.EVENT_READ)
        except BaseException:
            self.fail('pipe/log setup failure')
            raise
        return child

    def watch_fd(self, fd):
        os.set_blocking(fd, False)
        self.streams[fd] = {'pipe': fd, 'output': None, 'size': 0, 'marker': None, 'data': bytearray()}
        self.selector.register(fd, selectors.EVENT_READ)

    def pump(self, deadline):
        check_deadline(deadline)
        for key, _ in self.selector.select(min(.02, max(0, deadline-time.monotonic()))):
            fd = key.fd
            stream = self.streams[fd]
            data = os.read(fd, 65536)
            if not data:
                self.selector.unregister(key.fileobj)
                stream['eof'] = True
                continue
            stream['size'] += len(data)
            require(stream['size'] <= MAX_LOG, 'excessive output')
            if stream['output'] is not None:
                require(stream['output'].write(data) == len(data), 'short log write')
            stream['data'].extend(data)
            if stream['marker'] is not None:
                stream['marker'].feed(data, time.monotonic())
        check_deadline(deadline)

    def line(self, fd, deadline):
        while b'\n' not in self.streams[fd]['data']:
            require(not self.streams[fd].get('eof'), 'helper exited without readiness')
            self.pump(deadline)
        data = bytes(self.streams[fd]['data'])
        require(len(data) <= 4096, 'helper readiness too large')
        return data.split(b'\n', 1)[0]

    def pipes_finished(self, child):
        return all(self.streams[pipe.fileno()].get('eof', False) for pipe in (child.stdout, child.stderr))

    @staticmethod
    def group_exists(pgid):
        try:
            os.killpg(pgid, 0)
            return True
        except ProcessLookupError:
            return False

    def cleanup(self, deadline):
        # TERM/KILL are only sent to the exact new sessions this object created.
        for entry in self.children:
            try:
                os.killpg(entry['pgid'], signal.SIGTERM)
            except ProcessLookupError:
                pass
        grace = min(deadline, time.monotonic() + .5)
        while time.monotonic() < grace and any(e['process'].poll() is None for e in self.children):
            try:
                self.pump(grace)
            except Exception as error:
                self.fail(error)
        for entry in self.children:
            try:
                os.killpg(entry['pgid'], signal.SIGKILL)
            except ProcessLookupError:
                pass
        for entry in self.children:
            child = entry['process']
            try:
                child.wait(timeout=max(0, deadline-time.monotonic()))
                entry.update(reaped=True, exit=child.returncode)
            except subprocess.TimeoutExpired:
                self.fail('cleanup-incomplete: PID ' + str(entry['pid']))
            entry['group_empty'] = not self.group_exists(entry['pgid'])
            if not entry['group_empty']:
                self.fail('cleanup-incomplete: process group ' + str(entry['pgid']))
        while self.selector.get_map() and time.monotonic() < deadline:
            try:
                self.pump(deadline)
            except Exception as error:
                self.fail(error)
                break
        if self.selector.get_map():
            self.fail('cleanup-incomplete: pipes did not reach EOF')
        self.selector.close()
        for stream in self.streams.values():
            if stream['output'] is not None:
                stream['output'].close()
            if isinstance(stream['pipe'], int):
                os.close(stream['pipe'])
            else:
                stream['pipe'].close()
        return [{k: v for k, v in entry.items() if k != 'process'} for entry in self.children]


def xauthority(path, number):
    # FamilyLocal entry; cookie exists only in this private file, never argv/log.
    parts = (socket.gethostname().encode(), str(number).encode(), b'MIT-MAGIC-COOKIE-1', secrets.token_bytes(16))
    record = struct.pack('>H', 256) + b''.join(struct.pack('>H', len(p)) + p for p in parts)
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'wb') as output:
        output.write(record)


def linux_infra(owned, root, env, deadline):
    xvfb, dbus = shutil.which('Xvfb'), shutil.which('dbus-daemon')
    require(xvfb is not None and dbus is not None, 'Linux blocked: Xvfb/dbus-daemon unavailable')
    number = 20000 + secrets.randbelow(20000)
    require(not Path('/tmp/.X11-unix/X' + str(number)).exists()
            and not Path('/tmp/.X' + str(number) + '-lock').exists(), 'display collision')
    auth = root / 'Xauthority'
    xauthority(auth, number)
    read_fd, write_fd = os.pipe()
    try:
        owned.watch_fd(read_fd)
        server = owned.start([xvfb, ':' + str(number), '-displayfd', str(write_fd), '-screen', '0',
                              '1600x1000x24', '-nolisten', 'tcp', '-auth', str(auth)], root, env,
                             root / 'xvfb.stdout.txt', root / 'xvfb.stderr.txt', pass_fds=(write_fd,))
    finally:
        os.close(write_fd)
    ready = owned.line(read_fd, deadline)
    require(ready == str(number).encode() and server.poll() is None, 'Xvfb display readiness mismatch')
    bus = owned.start([dbus, '--session', '--nofork', '--nopidfile', '--print-address=1'], root, env,
                      root / 'dbus.stdout.txt', root / 'dbus.stderr.txt')
    address = owned.line(bus.stdout.fileno(), deadline)
    require(re.fullmatch(rb'unix:(?:path|abstract)=[A-Za-z0-9_./%=-]+(?:,guid=[0-9a-f]{32})?', address)
            and bus.poll() is None, 'invalid owned dbus address')
    env.update(DISPLAY=':' + str(number), XAUTHORITY=str(auth), DBUS_SESSION_BUS_ADDRESS=address.decode(),
               GDK_BACKEND='x11', LIBGL_ALWAYS_SOFTWARE='1')


def snapshot(root, deadline):
    """Bounded fixed synthetic session evidence, no recursive arbitrary scan."""
    smoke.directory(root)
    result = {}
    count, total = 0, 0
    for base, dirs, files in os.walk(root, followlinks=False):
        check_deadline(deadline)
        require(len(dirs) + len(files) <= 128, 'session entry limit')
        for name in sorted(dirs + files):
            count += 1
            require(count <= 128, 'session entry limit')
            path = Path(base) / name
            info = path.lstat()
            require(not stat.S_ISLNK(info.st_mode) and not getattr(info, 'st_file_attributes', 0) & 0x400, 'session link')
            key = path.relative_to(root).as_posix()
            if name in dirs:
                smoke.directory(path)
                result[key + '/'] = 'directory'
                continue
            size = smoke.plain(path).st_size
            total += size
            require(size <= 24 << 20 and total <= 32 << 20, 'session byte limit')
            digest = hashlib.sha256()
            with path.open('rb') as source:
                while data := source.read(65536):
                    check_deadline(deadline)
                    digest.update(data)
            result[key] = digest.hexdigest()
    identity = root / 'SESSION_IDENTITY'
    require(smoke.plain(identity).st_size == len(smoke.IDENTITY) and identity.read_bytes() == smoke.IDENTITY, 'wrong session identity')
    require(any(k.endswith('/COMMITTED') for k in result) and any(k.endswith('/state.json') for k in result), 'missing committed session')
    return result


def windows_owner(deadline):
    # Load only for the verified Windows target. The adapter has no POSIX
    # fallback and owns suspended creation, Job membership and pipe handles.
    spec = importlib.util.spec_from_file_location('native_window_windows_job', Path(__file__).with_name('windows_job.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module.WindowsOwned(deadline=deadline)


def stage(exe, root, number, target, total_deadline, first_start=None):
    t0 = time.monotonic() if first_start is None else first_start
    deadline = min(t0 + 60, total_deadline)
    ready_deadline = min(t0 + 45, deadline - 5)
    completion_deadline = min(t0 + 55, deadline - 5)
    label = 'stage' + str(number)
    record = {'stage': 'create' if number == 1 else 'open', 'passed': False,
              'marker_count': 0, 'processes': [], 'budget_kind': 'software-watchdog'}
    owned, marker = None, Marker()
    try:
        check_deadline(ready_deadline)
        owned = windows_owner(deadline) if target.startswith('windows/') else Owned()
        infra = root / ('infra-' + label)
        infra.mkdir(mode=0o700)
        env = isolated_env(infra)
        if target.startswith('linux/'):
            linux_infra(owned, infra, env, ready_deadline)
        argv = [str(exe), '-init-session' if number == 1 else '-session', str(root / 'session'),
                '-screen', 'day', '-native-window-smoke', str(root / (label + '.json'))]
        app = owned.start(argv, root, env, root / (label + '.stdout.txt'), root / (label + '.stderr.txt'), marker)
        while True:
            require(all(entry['process'] is app or entry['process'].poll() is None for entry in owned.children), 'owned infrastructure exited early')
            current = ready_deadline if marker.observed is None else min(completion_deadline, marker.observed + 10)
            owned.pump(current)
            if app.poll() is not None and owned.pipes_finished(app):
                break
        marker.finish()
        record.update(marker_count=marker.count, marker_observed_seconds=marker.observed-t0, exit=app.returncode)
        check_deadline(min(completion_deadline, marker.observed + 10))
        require(app.returncode == 0, 'application nonzero exit')
        report = read_report(root / (label + '.json'), record['stage'])
        record['today'] = report['today']
        record['report_sha256'] = smoke.digest(root / (label + '.json'))
        record['session_path'] = str(root / 'session')
        record['session_identity'] = smoke.IDENTITY.decode().rstrip('\n')
        record['session_files'] = snapshot(root / 'session', min(completion_deadline, marker.observed + 10))
        check_deadline(min(completion_deadline, marker.observed + 10))
        record['passed'] = True
    except BaseException as error:
        record['failure'] = str(error)
        if owned is not None:
            owned.fail(error)
    finally:
        record['marker_count'] = marker.count
        record['processes'] = owned.cleanup(deadline) if owned is not None else []
        if owned is not None and owned.failed is not None:
            record.update(passed=False, failure=owned.failed)
        if time.monotonic() >= deadline:
            record.update(passed=False, failure=record.get('failure', 'software deadline exceeded during cleanup'))
        record['duration_seconds'] = time.monotonic() - t0
        (root / (label + '-driver.json')).write_text(json.dumps(record, indent=2) + '\n')
    require(record['passed'], record.get('failure', 'stage failure'))
    return record


def run(args):
    smoke.provenance(args.source, args.run_id, args.run_attempt)
    require(re.fullmatch('[1-9][0-9]*', args.consumer_attempt) is not None, 'invalid consumer attempt')
    require(re.fullmatch('[1-9][0-9]*', args.artifact_id) is not None, 'invalid artifact ID')
    require(int(args.consumer_attempt) >= int(args.run_attempt), 'consumer precedes build')
    start = time.monotonic()
    total_deadline = start + 120
    clean_directory(args.artifact)
    parent = work_parent(args.work_parent, args.artifact)
    root = Path(tempfile.mkdtemp(prefix='native-window-', dir=parent))
    result = {'version': 1, 'scope': 'lifecycle-only', 'passed': False,
              'display_pixels_verified': False, 'ime_tested': False,
              'target': args.target, 'source': args.source, 'run_id': args.run_id, 'run_attempt': args.run_attempt,
              'build_attempt': args.run_attempt, 'consumer_attempt': args.consumer_attempt,
              'artifact_id': args.artifact_id,
              'budget_kind': 'software-watchdog', 'work_root': str(root)}
    try:
        require(os.name == 'nt' or stat.S_IMODE(root.stat().st_mode) == 0o700, 'private root permissions')
        exe = smoke.verify(args.artifact, args.target, args.source, args.run_id, args.run_attempt)
        check_deadline(start + 45)
        require(smoke.native_host() == args.target, 'native host/target mismatch')
        result['binary_sha256'] = smoke.digest(exe)
        shutil.copyfile(args.artifact / 'manifest.json', root / 'manifest.json')
        if os.name != 'nt':
            exe.chmod(0o700)
        first = stage(exe, root, 1, args.target, total_deadline, start)
        second = stage(exe, root, 2, args.target, total_deadline)
        require(first['today'] == second['today'], 'date-boundary-not-covered')
        require(first['session_path'] == second['session_path'] and first['session_files'] == second['session_files'], 'session changed across same-day reopen')
        check_deadline(total_deadline)
        result['passed'] = True
    except Exception as error:
        result['failure'] = str(error)
    finally:
        result['duration_seconds'] = time.monotonic() - start
        (root / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
        print(root)
    return 0 if result['passed'] else 1


def absolute_argument(raw):
    require(os.path.isabs(raw) and os.path.normpath(raw) == raw, 'clean absolute argument required')
    return Path(raw)


def parser():
    p = argparse.ArgumentParser(description=__doc__, allow_abbrev=False)
    p.add_argument('--artifact', type=absolute_argument, required=True)
    p.add_argument('--target', choices=sorted(smoke.TARGETS), required=True)
    for key in ('source', 'run-id', 'run-attempt', 'consumer-attempt', 'artifact-id'):
        p.add_argument('--' + key, required=True)
    p.add_argument('--work-parent', type=absolute_argument, required=True)
    return p


if __name__ == '__main__':
    sys.exit(run(parser().parse_args()))
