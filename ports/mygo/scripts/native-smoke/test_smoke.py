"""Small synthetic fixtures only. These do not prove any native runtime."""
import ctypes
import errno
import json
from pathlib import Path
import struct
import sys
import tempfile
import unittest
import zlib
from unittest.mock import patch
import smoke

SHA = 'a' * 40


def elf(arch=62):
    b = bytearray(64)
    b[:6] = b'\x7fELF\x02\x01'
    struct.pack_into('<H', b, 18, arch)
    return bytes(b)


def chunk(kind, data):
    return struct.pack('>I', len(data)) + kind + data + struct.pack('>I', zlib.crc32(kind+data) & 0xffffffff)


def image(raw=b'\0\x01\x02\x03', dimensions=(1, 1)):
    return b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>IIBBBBB', *dimensions, 8, 2, 0, 0, 0)) + chunk(b'IDAT', zlib.compress(raw)) + chunk(b'IEND', b'')


class Fixtures(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.addCleanup(self.tmp.cleanup)

    def test_pack_verify_tamper_allowlist_provenance(self):
        binary = self.root / 'input'
        binary.write_bytes(elf())
        artifact = self.root / 'artifact'
        smoke.pack(binary, artifact, 'linux/amd64', SHA, '123', '1')
        exe = smoke.verify(artifact, 'linux/amd64', SHA, '123', '1')
        for source, run, attempt in [('b'*40, '123', '1'), (SHA, '124', '1'), (SHA, '123', '2')]:
            with self.assertRaises(ValueError):
                smoke.verify(artifact, 'linux/amd64', source, run, attempt)
        (artifact / 'extra').write_bytes(b'no')
        with self.assertRaises(ValueError):
            smoke.verify(artifact, 'linux/amd64', SHA, '123', '1')
        (artifact / 'extra').unlink()
        exe.write_bytes(elf(183))
        with self.assertRaises(ValueError):
            smoke.verify(artifact, 'linux/amd64', SHA, '123', '1')

    def test_six_headers(self):
        for osname in ('linux', 'darwin', 'windows'):
            for arch in ('amd64', 'arm64'):
                if osname == 'linux':
                    b = elf(62 if arch == 'amd64' else 183)
                elif osname == 'darwin':
                    b = b'\xcf\xfa\xed\xfe' + struct.pack('<I', 0x1000007 if arch == 'amd64' else 0x100000c) + bytes(24)
                else:
                    b = bytearray(128)
                    b[:2] = b'MZ'
                    struct.pack_into('<I', b, 60, 64)
                    b[64:68] = b'PE\0\0'
                    struct.pack_into('<H', b, 68, 0x8664 if arch == 'amd64' else 0xaa64)
                    b[88:90] = b'\x0b\x02'
                path = self.root / 'exe'
                path.write_bytes(b)
                self.assertEqual(smoke.binary_target(path), osname + '/' + arch)

    def test_png_full_decode_and_negatives(self):
        path = self.root / 'frame.png'
        for filter_type in range(5):
            path.write_bytes(image(bytes([filter_type, 1, 2, 3])))
            smoke.png(path, (1, 1))
        for bad in [b'', image()[:-1], image()+b'x', image(b'\x05\x01\x02\x03'), image(b'\0\x01'), image(b'\0\x01\x02\x03\0'), image(dimensions=(2, 1))]:
            path.write_bytes(bad)
            with self.assertRaises(ValueError):
                smoke.png(path, (1, 1))
        corrupt = bytearray(image())
        corrupt[45] ^= 1
        path.write_bytes(corrupt)
        with self.assertRaises(ValueError):
            smoke.png(path, (1, 1))

    def test_command_rejection_crash_timeout(self):
        py = Path(sys.executable)
        smoke.command(py, ['-c', 'print("ok")'], self.root, 'ok')
        smoke.command(py, ['-c', 'import sys; sys.stderr.write("2026/11/06 12:00:00 expected\\n"); sys.exit(1)'], self.root, 'guard', diagnostic='expected')
        for code in ['raise SystemExit(1)', 'raise SystemExit(2)', 'print("expected")']:
            with self.assertRaises(ValueError):
                smoke.command(py, ['-c', code], self.root, 'bad', diagnostic='expected')
        with self.assertRaises(ValueError):
            smoke.command(py, ['-c', 'import time; time.sleep(5)'], self.root, 'timeout', timeout=0.05)

    def test_startup_rejections_require_exact_diagnostic(self):
        # Exercise the real subprocess validator, not a mocked return code.
        diagnostics = (
            'init-session requires an absent directory',
            'use -init-session or -session for development storage; -prototype for static preview',
        )
        old = (
            'Cannot open isolated development session: invalid isolated development session',
            'Use -init-session or -session for development storage; -prototype for the static visual study',
        )
        for i, diagnostic in enumerate(diagnostics):
            expected = '2026/11/07 15:31:51 ' + diagnostic + '\n'
            cases = [('correct', 1, '', expected, True),
                     ('old', 1, '', '2026/11/07 15:31:51 ' + old[i] + '\n', False),
                     ('other', 1, '', '2026/11/07 15:31:51 unrelated failure\n', False),
                     ('stdout', 1, 'unexpected\n', expected, False),
                     ('prefix', 1, '', 'extra\n' + expected, False),
                     ('suffix', 1, '', expected + 'extra\n', False),
                     ('no-timestamp', 1, '', diagnostic + '\n', False),
                     ('bad-timestamp', 1, '', '2026-11-07 15:31:51 ' + diagnostic + '\n', False),
                     ('no-newline', 1, '', expected.rstrip('\n'), False),
                     ('detail', 1, '', expected.rstrip('\n') + ': detail\n', False),
                     ('zero', 0, '', expected, False),
                     ('two', 2, '', expected, False)]
            for name, status, out, err, accepted in cases:
                with self.subTest(diagnostic=diagnostic, case=name):
                    label = 'diagnostic-' + str(i) + '-' + name
                    code = f'import sys; sys.stdout.write({out!r}); sys.stderr.write({err!r}); sys.exit({status})'
                    args = [Path(sys.executable), ['-c', code], self.root, label]
                    if accepted:
                        self.assertEqual(smoke.command(*args, diagnostic=diagnostic), '')
                    else:
                        with self.assertRaisesRegex(ValueError, label + ': wrong rejection'):
                            smoke.command(*args, diagnostic=diagnostic)
                    receipt = json.loads((self.root / (label + '.json')).read_text())
                    self.assertTrue(receipt['started'])
                    self.assertEqual(receipt['exit'], status)
                    self.assertEqual('failure' in receipt, not accepted)

    def test_startup_orchestration_exact_rejections_and_preservation(self):
        listing = 'day 1200x768\npool 1200x768\n' + ''.join('screen-' + chr(97+i) + ' 1x1\n' for i in range(18))
        for fault in ('none', 'reuse-mutation', 'reuse-frames', 'implicit-mutation', 'implicit-home'):
            with self.subTest(fault=fault):
                root = self.root / fault
                root.mkdir()
                calls = []
                def fake(exe, args, folder, label, **kwargs):
                    calls.append(label)
                    if label == 'list':
                        return listing
                    if label in ('create', 'reopen'):
                        session = root / 'session'
                        (session / 'rev').mkdir(parents=True, exist_ok=True)
                        (session / 'SESSION_IDENTITY').write_bytes(smoke.IDENTITY)
                        (session / 'rev' / 'COMMITTED').write_bytes(b'fixture')
                        (session / 'rev' / 'state.json').write_bytes(b'{}')
                        frames = root / label
                        frames.mkdir()
                        (frames / ('noonmark-' + ('day' if label == 'create' else 'pool') + '-native.png')).write_bytes(b'fixture')
                    if label in ('reuse', 'implicit'):
                        self.assertEqual(kwargs['diagnostic'], {
                            'reuse': 'init-session requires an absent directory',
                            'implicit': 'use -init-session or -session for development storage; -prototype for static preview',
                        }[label])
                        if fault == 'implicit-home' and label == 'implicit':
                            (root / '.unexpected-config').mkdir()
                        if fault == label + '-mutation':
                            (root / 'session' / 'rev' / 'state.json').write_bytes(b'mutation')
                        if fault == 'reuse-frames' and label == 'reuse':
                            (root / 'reuse-frames').mkdir()
                    return ''
                with patch.object(smoke, 'command', fake), patch.object(smoke, 'png'):
                    if fault == 'none':
                        smoke.smoke(Path('not-an-executable'), root)
                        self.assertEqual(calls, ['list', 'create', 'reopen', 'reuse', 'implicit'])
                    else:
                        with self.assertRaisesRegex(ValueError, 'implicit created or removed home data' if fault == 'implicit-home' else ('implicit' if fault.startswith('implicit') else 'reuse') + ' modified session'):
                            smoke.smoke(Path('not-an-executable'), root)

    def test_session_snapshot_identity_and_mutation(self):
        s = self.root / 'session'
        (s / 'revision').mkdir(parents=True)
        (s / 'SESSION_IDENTITY').write_bytes(smoke.IDENTITY)
        (s / 'revision' / 'COMMITTED').write_bytes(b'fixture')
        (s / 'revision' / 'state.json').write_bytes(b'{}')
        before = smoke.snapshot(s)
        (s / 'revision' / 'state.json').write_bytes(b'{"changed":true}')
        self.assertNotEqual(before, smoke.snapshot(s))
        (s / 'SESSION_IDENTITY').write_bytes(b'wrong\n')
        with self.assertRaises(ValueError):
            smoke.snapshot(s)

    def test_fake_cli_orchestration_missing_bad_png_guard_mutation(self):
        # No executable is run: fake responses exercise orchestration fail-closed.
        listing = 'day 1200x768\npool 1200x768\n' + ''.join('screen-' + chr(97+i) + ' 1x1\n' for i in range(18))
        for fault in ('missing-png', 'bad-png', 'wrong-identity', 'mutating-guard'):
            root = self.root / fault
            root.mkdir()
            def fake(exe, args, folder, label, **kwargs):
                if label == 'list':
                    return listing
                if label in ('create', 'reopen'):
                    s = root / 'session'
                    (s / 'rev').mkdir(parents=True, exist_ok=True)
                    (s / 'SESSION_IDENTITY').write_bytes(b'bad' if fault == 'wrong-identity' else smoke.IDENTITY)
                    (s / 'rev' / 'COMMITTED').write_bytes(b'fixture')
                    (s / 'rev' / 'state.json').write_bytes(b'{}')
                    frames = root / label
                    frames.mkdir()
                    if fault != 'missing-png':
                        (frames / ('noonmark-' + ('day' if label == 'create' else 'pool') + '-native.png')).write_bytes(b'bad')
                if label == 'reuse':
                    (root / 'session' / 'rev' / 'state.json').write_bytes(b'mutation')
                return ''
            with patch.object(smoke, 'command', fake):
                if fault in ('wrong-identity', 'mutating-guard'):
                    with patch.object(smoke, 'png'), self.assertRaises(ValueError):
                        smoke.smoke(Path('not-an-executable'), root)
                else:
                    with self.assertRaises(ValueError):
                        smoke.smoke(Path('not-an-executable'), root)

    def test_review_identity_gate_before_read(self):
        import os
        s = self.root / 'session'
        s.mkdir()
        identity = s / 'SESSION_IDENTITY'
        if hasattr(os, 'mkfifo'):
            os.mkfifo(identity)
            with patch.object(Path, 'read_bytes', side_effect=AssertionError('must not read')), self.assertRaises(ValueError):
                smoke.snapshot(s)
            identity.unlink()
        target = self.root / 'target'
        target.write_bytes(smoke.IDENTITY)
        identity.symlink_to(target)
        with patch.object(Path, 'read_bytes', side_effect=AssertionError('must not read')), self.assertRaises(ValueError):
            smoke.snapshot(s)
        identity.unlink()
        identity.write_bytes(smoke.IDENTITY + b'X')
        with patch.object(Path, 'read_bytes', side_effect=AssertionError('must not read')), self.assertRaises(ValueError):
            smoke.snapshot(s)
        identity.unlink()
        import os
        os.link(target, identity)
        with patch.object(Path, 'read_bytes', side_effect=AssertionError('must not read')), self.assertRaises(ValueError):
            smoke.snapshot(s)

    def test_review_directory_gates(self):
        from types import SimpleNamespace
        target = self.root / 'real'
        target.mkdir()
        linked = self.root / 'link'
        linked.symlink_to(target, target_is_directory=True)
        for call in (lambda: smoke.directory(linked), lambda: smoke.snapshot(linked), lambda: smoke.verify(linked, 'linux/amd64', SHA, '123', '1')):
            with self.assertRaises(ValueError):
                call()
        original = Path.lstat
        def reparse(path, *args, **kwargs):
            st = original(path, *args, **kwargs)
            if path == target:
                return SimpleNamespace(st_mode=st.st_mode, st_file_attributes=0x400)
            return st
        with patch.object(Path, 'lstat', reparse):
            with self.assertRaises(ValueError):
                smoke.directory(target)
            with self.assertRaises(ValueError):
                smoke.verify(target, 'linux/amd64', SHA, '123', '1')
            with self.assertRaises(ValueError):
                smoke.snapshot(target)

    def test_review_frames_root_rejected_before_decode(self):
        target = self.root / 'real-frames'
        target.mkdir()
        listing = 'day 1200x768\npool 1200x768\n' + ''.join('screen-' + chr(97+i) + ' 1x1\n' for i in range(18))
        def fake(exe, args, root, label, **kwargs):
            if label == 'list':
                return listing
            (root / 'create').symlink_to(target, target_is_directory=True)
        with patch.object(smoke, 'command', fake), patch.object(smoke, 'png', side_effect=AssertionError('must not decode')), self.assertRaises(ValueError):
            smoke.smoke(Path('unused'), self.root)

    def test_review_png_type_and_idat_state(self):
        path = self.root / 'png'
        b = image()
        bad = [b[:33] + chunk(k, b'') + b[33:] for k in (b'te1t', b'text', b'\0abc')]
        bad.append(b[:33] + chunk(b'IDAT', b'') + chunk(b'tEXt', b'k\0v') + b[33:])
        bad.append(image(b'\0' + bytes(1 << 20)))
        for data in bad:
            path.write_bytes(data)
            with self.assertRaises(ValueError):
                smoke.png(path, (1, 1))

    def test_review_mac_query_fail_closed(self):
        # Error, malformed, empty/short and unexpected-success values stay closed.
        for answer in [(-1, errno.EACCES, 4, 1), (-1, errno.EIO, 4, 0),
                       (-1, 0, 4, 1), (1, 0, 4, 1), (0, 0, 0, 0),
                       (0, 0, 8, 1), (0, 0, 4, 2), (0, 0, 4, -1)]:
            with self.subTest(answer=answer), self.assertRaises(ValueError):
                self.mac_host('x86_64', {'hw.optional.arm64': answer})

    def mac_host(self, machine, answers, facts=None):
        from types import SimpleNamespace
        if facts is None:
            facts = {}
        calls = []
        def query(name, old, size, new, newlen):
            self.assertIsNone(new)
            self.assertEqual(newlen, 0)
            self.assertEqual(ctypes.get_errno(), 0)
            self.assertEqual(ctypes.cast(size, ctypes.POINTER(ctypes.c_size_t)).contents.value, 4)
            name = name.decode('ascii')
            calls.append(name)
            self.assertIn(name, answers, 'unexpected/unbound capability query')
            rc, error, count, value = answers[name]
            ctypes.cast(old, ctypes.POINTER(ctypes.c_int)).contents.value = value
            ctypes.cast(size, ctypes.POINTER(ctypes.c_size_t)).contents.value = count
            ctypes.set_errno(error)
            return rc
        with patch.object(smoke.platform, 'system', return_value='Darwin'), \
                patch.object(smoke.platform, 'machine', return_value=machine), \
                patch.object(smoke.ctypes, 'CDLL', return_value=SimpleNamespace(sysctlbyname=query)) as loader:
            result = smoke.native_host(facts)
        loader.assert_called_once_with('/usr/lib/libSystem.B.dylib', use_errno=True)
        self.assertEqual(query.argtypes, [ctypes.c_char_p, ctypes.c_void_p,
                                        ctypes.POINTER(ctypes.c_size_t), ctypes.c_void_p, ctypes.c_size_t])
        self.assertIs(query.restype, ctypes.c_int)
        self.assertEqual(calls, [q['name'] for q in facts['queries']])
        return result

    def test_mac_intel_requires_positive_capability(self):
        missing = (-1, errno.ENOENT, 4, 0)
        yes, no = (0, 0, 4, 1), (0, 0, 4, 0)
        for arm in (missing, no):
            for translated in (missing, no):
                facts = {}
                self.assertEqual(self.mac_host('x86_64', {'hw.optional.arm64': arm,
                    'sysctl.proc_translated': translated, 'hw.optional.x86_64': yes}, facts), 'darwin/amd64')
                self.assertEqual(facts['method'], 'libSystem.sysctlbyname')
                self.assertEqual(facts['queries'][0]['errno'], arm[1])
                self.assertEqual(len(facts['queries']), 3)
        for x86 in (missing, no, (-1, errno.EPERM, 4, 1)):
            with self.assertRaises(ValueError):
                self.mac_host('x86_64', {'hw.optional.arm64': missing,
                    'sysctl.proc_translated': missing, 'hw.optional.x86_64': x86})

    def test_mac_arm_and_rosetta_not_mislabelled_as_intel(self):
        missing = (-1, errno.ENOENT, 4, 0)
        yes, no = (0, 0, 4, 1), (0, 0, 4, 0)
        for machine, translated in [('arm64', no), ('arm64', missing), ('x86_64', yes)]:
            facts = {}
            self.assertEqual(self.mac_host(machine, {'hw.optional.arm64': yes,
                'sysctl.proc_translated': translated}, facts), 'darwin/arm64')
            self.assertEqual(len(facts['queries']), 2)
        for machine, arm, translated in [('x86_64', yes, no), ('x86_64', yes, missing),
            ('arm64', yes, yes), ('unknown', yes, no), ('arm64', missing, no),
            ('x86_64', missing, yes), ('x86_64', no, yes)]:
            with self.subTest(machine=machine, arm=arm, translated=translated), self.assertRaises(ValueError):
                self.mac_host(machine, {'hw.optional.arm64': arm, 'sysctl.proc_translated': translated})

    def test_mac_query_failure_keeps_errno_receipt(self):
        facts = {}
        with self.assertRaisesRegex(ValueError, 'errno=' + str(errno.EACCES)):
            self.mac_host('x86_64', {'hw.optional.arm64': (-1, errno.EACCES, 4, 9)}, facts)
        self.assertEqual(facts['queries'], [{'name': 'hw.optional.arm64', 'returncode': -1,
                         'errno': errno.EACCES, 'size': 4, 'value': None}])

    def test_review_phase_failure_records(self):
        for label, exe, args, kwargs in [
            ('spawn', self.root / 'missing', [], {}),
            ('timeout', Path(sys.executable), ['-c', 'import time;time.sleep(3)'], {'timeout': 0.05}),
            ('wrong', Path(sys.executable), ['-c', 'raise SystemExit(2)'], {'diagnostic': 'expected'})]:
            with self.assertRaises((ValueError, OSError)):
                smoke.command(exe, args, self.root, label, **kwargs)
            record = json.loads((self.root / (label + '.json')).read_bytes())
            self.assertEqual(record['argv'], [str(exe), *args])
            self.assertIn('failure', record)
            self.assertGreaterEqual(record['duration'], 0)
            if label == 'spawn':
                self.assertIsNone(record['exit'])
                self.assertFalse(record['started'])
            else:
                self.assertIs(type(record['exit']), int)
                self.assertTrue(record['started'])

    def test_review_manifest_strict_types(self):
        binary = self.root / 'binary'
        binary.write_bytes(elf())
        artifact = self.root / 'artifact'
        smoke.pack(binary, artifact, 'linux/amd64', SHA, '123', '1')
        path = artifact / 'manifest.json'
        original = json.loads(path.read_bytes())
        for key, value in [('schema', True), ('schema', 1.0), ('size', True), ('size', 64.0), ('sha256', 'A' * 64), ('sha256', 42)]:
            altered = dict(original)
            altered[key] = value
            path.write_text(json.dumps(altered))
            with self.assertRaises(ValueError):
                smoke.verify(artifact, 'linux/amd64', SHA, '123', '1')


if __name__ == '__main__':
    unittest.main()
