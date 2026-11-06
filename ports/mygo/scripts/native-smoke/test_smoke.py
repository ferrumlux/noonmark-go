"""Small synthetic fixtures only. These do not prove any native runtime."""
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
        from types import SimpleNamespace
        for code, out in [(0, b''), (0, b'unexpected\n'), (1, b'1\n')]:
            with patch.object(smoke.platform, 'system', return_value='Darwin'), patch.object(smoke.subprocess, 'run', return_value=SimpleNamespace(returncode=code, stdout=out)), self.assertRaises(ValueError):
                smoke.native_host()
        for out, expected in [(b'0\n', 'darwin/amd64'), (b'1\n', 'darwin/arm64')]:
            with patch.object(smoke.platform, 'system', return_value='Darwin'), patch.object(smoke.subprocess, 'run', return_value=SimpleNamespace(returncode=0, stdout=out)):
                self.assertEqual(smoke.native_host(), expected)

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
