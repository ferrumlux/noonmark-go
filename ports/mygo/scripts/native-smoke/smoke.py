#!/usr/bin/env python3
"""Bounded native-runtime smoke. No builds, downloads, shell or user-data discovery."""
import argparse
import ctypes
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import stat
import struct
import subprocess
import sys
import tempfile
import time
import zlib

TARGETS = {f'{o}/{a}' for o in ('linux', 'darwin', 'windows') for a in ('amd64', 'arm64')}
IDENTITY = b'noonmark-mygo-development-session-v2\n'
MAX_BINARY = 256 << 20
MAX_PNG = 32 << 20


def require(ok, reason):
    if not ok:
        raise ValueError(reason)


def plain(path):
    st = path.lstat()
    require(stat.S_ISREG(st.st_mode) and not path.is_symlink() and st.st_nlink == 1
            and not getattr(st, 'st_file_attributes', 0) & 0x400, 'non-regular artifact/file')
    return st


def directory(path):
    st = path.lstat()
    require(stat.S_ISDIR(st.st_mode) and not stat.S_ISLNK(st.st_mode)
            and not getattr(st, 'st_file_attributes', 0) & 0x400, 'non-regular directory')
    return path


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for b in iter(lambda: f.read(1 << 20), b''):
            h.update(b)
    return h.hexdigest()


def filename(target):
    require(target in TARGETS, 'unsupported target')
    return 'noonmark-mygo-' + target.replace('/', '-') + ('.exe' if target.startswith('windows/') else '')


def binary_target(path):
    plain(path)
    with path.open('rb') as f:
        b = f.read(4096)
    if b[:4] == b'\x7fELF':
        require(len(b) >= 64 and b[4:6] == b'\x02\x01', 'expected ELF64 little endian')
        arch = {62: 'amd64', 183: 'arm64'}.get(struct.unpack_from('<H', b, 18)[0])
        osname = 'linux'
    elif b[:4] == b'\xcf\xfa\xed\xfe':
        require(len(b) >= 32, 'short Mach-O')
        arch = {0x1000007: 'amd64', 0x100000c: 'arm64'}.get(struct.unpack_from('<I', b, 4)[0])
        osname = 'darwin'
    elif b[:2] == b'MZ':
        require(len(b) >= 64, 'short PE')
        pos = struct.unpack_from('<I', b, 60)[0]
        require(64 <= pos <= 1 << 20, 'invalid PE offset')
        with path.open('rb') as f:
            f.seek(pos)
            pe = f.read(26)
        require(len(pe) == 26 and pe[:4] == b'PE\0\0' and pe[24:26] == b'\x0b\x02', 'expected PE32+')
        arch = {0x8664: 'amd64', 0xaa64: 'arm64'}.get(struct.unpack_from('<H', pe, 4)[0])
        osname = 'windows'
    else:
        raise ValueError('unknown executable header')
    require(arch is not None, 'unsupported executable architecture')
    return osname + '/' + arch


def native_host():
    system = platform.system().lower()
    require(system in ('linux', 'darwin', 'windows'), 'unsupported host OS')
    if system == 'windows':
        # GetNativeSystemInfo reports OS architecture even under WOW/x64 emulation.
        buf = ctypes.create_string_buffer(128)
        ctypes.windll.kernel32.GetNativeSystemInfo(ctypes.byref(buf))
        arch = {9: 'amd64', 12: 'arm64'}.get(struct.unpack_from('<H', buf.raw)[0])
    elif system == 'darwin':
        r = subprocess.run(['/usr/sbin/sysctl', '-n', 'hw.optional.arm64'], capture_output=True, timeout=10)
        require(r.returncode == 0, 'native macOS architecture query failed')
        require(r.stdout.strip() in (b'0', b'1'), 'unknown native macOS architecture')
        arch = 'arm64' if r.stdout.strip() == b'1' else 'amd64'
    else:
        require('microsoft' not in platform.release().lower(), 'WSL is not a native host target')
        arch = {'x86_64': 'amd64', 'aarch64': 'arm64'}.get(os.uname().machine)
    require(arch is not None, 'unknown native architecture')
    return system + '/' + arch


def provenance(source, run, attempt):
    require(re.fullmatch('[0-9a-f]{40}', source) is not None, 'source must be full SHA')
    require(re.fullmatch('[1-9][0-9]*', run) is not None and re.fullmatch('[1-9][0-9]*', attempt) is not None, 'invalid run/attempt')
    return {'source_sha': source, 'run_id': run, 'run_attempt': attempt}


def pack(binary, output, target, source, run, attempt):
    meta = provenance(source, run, attempt)
    require(plain(binary).st_size <= MAX_BINARY and binary_target(binary) == target, 'binary target/size mismatch')
    output.mkdir()  # exclusive new directory, never overwrite
    name = filename(target)
    shutil.copyfile(binary, output / name)
    meta.update(schema=1, target=target, filename=name, size=(output / name).stat().st_size, sha256=digest(output / name))
    (output / 'manifest.json').write_bytes((json.dumps(meta, sort_keys=True) + '\n').encode())


def verify(artifact, target, source, run, attempt):
    expected = provenance(source, run, attempt)
    directory(artifact)
    name = filename(target)
    require({p.name for p in artifact.iterdir()} == {name, 'manifest.json'}, 'artifact allowlist mismatch')
    mp = artifact / 'manifest.json'
    require(plain(mp).st_size < 4096, 'manifest too large')
    def unique(pairs):
        d = {}
        for k, v in pairs:
            require(k not in d, 'duplicate manifest field')
            d[k] = v
        return d
    m = json.loads(mp.read_bytes(), object_pairs_hook=unique)
    require(set(m) == {'schema', 'target', 'filename', 'size', 'sha256', *expected}, 'manifest fields mismatch')
    require(type(m['schema']) is int and m['schema'] == 1 and type(m['size']) is int
            and type(m['sha256']) is str and re.fullmatch('[0-9a-f]{64}', m['sha256']) is not None, 'manifest field types')
    require(m['target'] == target and m['filename'] == name, 'manifest target mismatch')
    require(all(m[k] == v for k, v in expected.items()), 'artifact source/run mismatch')
    exe = artifact / name
    require(0 < plain(exe).st_size == m['size'] <= MAX_BINARY, 'artifact size mismatch')
    require(digest(exe) == m['sha256'] and binary_target(exe) == target, 'artifact digest/header mismatch')
    return exe


def png(path, dimensions):
    require(0 < plain(path).st_size <= MAX_PNG, 'PNG size invalid')
    b = path.read_bytes()
    require(b[:8] == b'\x89PNG\r\n\x1a\n', 'PNG signature')
    pos, compressed, header, ended, idat_ended = 8, bytearray(), None, False, False
    saw_idat = False
    while pos < len(b):
        require(pos + 12 <= len(b), 'truncated PNG chunk')
        n = struct.unpack_from('>I', b, pos)[0]
        kind = b[pos+4:pos+8]
        require(re.fullmatch(b'[A-Za-z]{2}[A-Z][A-Za-z]', kind) is not None, 'invalid PNG chunk type')
        require(pos + n + 12 <= len(b), 'truncated PNG payload')
        data = b[pos+8:pos+8+n]
        crc = struct.unpack_from('>I', b, pos+8+n)[0]
        require(zlib.crc32(kind + data) & 0xffffffff == crc, 'PNG CRC')
        require(header is not None or kind == b'IHDR', 'IHDR must be first')
        if kind == b'IHDR':
            require(header is None and n == 13, 'invalid IHDR')
            header = struct.unpack('>IIBBBBB', data)
            w, h, depth, color, comp, filt, interlace = header
            require((w, h) == dimensions and depth == 8 and color in (2, 6) and (comp, filt, interlace) == (0, 0, 0), 'unexpected PNG encoding/dimensions')
        elif kind == b'IDAT':
            require(not idat_ended, 'non-contiguous IDAT')
            saw_idat = True
            compressed.extend(data)
        elif kind == b'IEND':
            require(n == 0 and compressed and pos+n+12 == len(b), 'invalid IEND/trailing data')
            ended = True
        else:
            require(kind[0] & 32, 'unsupported critical PNG chunk')
            if saw_idat:
                idat_ended = True
        pos += n + 12
    require(ended, 'missing IEND')
    w, h, _, color, *_ = header
    channels = 3 if color == 2 else 4
    stride = w * channels
    limit = (stride + 1) * h
    dec = zlib.decompressobj()
    raw = dec.decompress(compressed, limit + 1)
    require(len(raw) == limit and dec.eof and not dec.unused_data and not dec.unconsumed_tail, 'invalid PNG compressed raster')
    # Decode every scanline (including all five PNG filters), not just IHDR/zlib.
    previous = bytearray(stride)
    for row in range(h):
        offset = row * (stride + 1)
        filter_type = raw[offset]
        require(filter_type <= 4, 'invalid PNG filter')
        current = bytearray(raw[offset+1:offset+1+stride])
        for x in range(stride):
            a = current[x-channels] if x >= channels else 0
            up = previous[x]
            c = previous[x-channels] if x >= channels else 0
            if filter_type == 1:
                predictor = a
            elif filter_type == 2:
                predictor = up
            elif filter_type == 3:
                predictor = (a + up) // 2
            elif filter_type == 4:
                p = a + up - c
                distances = (abs(p-a), abs(p-up), abs(p-c))
                predictor = (a, up, c)[distances.index(min(distances))]
            else:
                predictor = 0
            current[x] = (current[x] + predictor) & 255
        previous = current


def snapshot(root):
    directory(root)
    identity = root / 'SESSION_IDENTITY'
    require(plain(identity).st_size == len(IDENTITY), 'wrong session identity size')
    require(identity.read_bytes() == IDENTITY, 'wrong session identity')
    result = {}
    for p in sorted(root.rglob('*')):
        require(not p.is_symlink() and not getattr(p.lstat(), 'st_file_attributes', 0) & 0x400, 'session link')
        if p.is_file():
            require(plain(p).st_size <= 24 << 20, 'session file too large')
            result[p.relative_to(root).as_posix()] = digest(p)
        else:
            require(p.is_dir(), 'special session file')
            result[p.relative_to(root).as_posix() + '/'] = 'directory'
    require(any(k.endswith('/COMMITTED') for k in result) and any(k.endswith('/state.json') for k in result), 'missing committed session')
    return result


def command(exe, argv, root, label, timeout=60, diagnostic=None):
    start = time.monotonic()
    env = {k: v for k, v in os.environ.items() if not any(s in k.upper() for s in ('TOKEN', 'SECRET', 'PASSWORD', 'API_KEY'))}
    env.update(TMP=str(root), TEMP=str(root), TMPDIR=str(root), HOME=str(root), USERPROFILE=str(root), APPDATA=str(root), LOCALAPPDATA=str(root), XDG_CONFIG_HOME=str(root), XDG_DATA_HOME=str(root), XDG_CACHE_HOME=str(root))
    out, err = root / (label + '.stdout.txt'), root / (label + '.stderr.txt')
    # Capture into isolated files; enforce the evidence size limit before reading.
    record = {'argv': [str(exe), *argv], 'exit': None, 'started': False}
    process = None
    try:
        with out.open('wb') as stdout, err.open('wb') as stderr:
            process = subprocess.Popen([str(exe), *argv], cwd=root, env=env, stdout=stdout, stderr=stderr)
            record['started'] = True
            while process.poll() is None:
                require(time.monotonic() - start < timeout, label + ': timeout')
                require(out.stat().st_size <= 1 << 20 and err.stat().st_size <= 1 << 20, label + ': excessive output')
                time.sleep(0.02)
            record['exit'] = process.returncode
        require(out.stat().st_size <= 1 << 20 and err.stat().st_size <= 1 << 20, label + ': excessive output')
        stdout, stderr = out.read_text(errors='replace'), err.read_text(errors='replace')
        if diagnostic is None:
            require(record['exit'] == 0, label + ': failed')
        else:
            require(record['exit'] == 1 and stdout == '' and re.fullmatch(r'\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} ' + re.escape(diagnostic) + r'\n', stderr) is not None, label + ': wrong rejection')
        return stdout
    except Exception as error:
        if process is not None:
            if process.poll() is None:
                process.kill()
            record['exit'] = process.wait()
        record['failure'] = str(error)
        raise
    finally:
        record['duration'] = time.monotonic() - start
        (root / (label + '.json')).write_text(json.dumps(record))


def smoke(exe, root):
    listing = command(exe, ['-list'], root, 'list')
    rows = listing.splitlines()
    require(len(rows) == 20 and all(re.fullmatch(r'[a-z-]+ [1-9][0-9]*x[1-9][0-9]*', r) for r in rows), 'invalid screen list')
    screens = dict(row.split() for row in rows)
    require(len(screens) == 20 and screens.get('day') == '1200x768' and screens.get('pool') == '1200x768', 'screen dimensions changed')
    session = root / 'session'
    for label, flag, screen in [('create', '-init-session', 'day'), ('reopen', '-session', 'pool')]:
        frames = root / label
        command(exe, [flag, str(session), '-render-one', str(frames), '-screen', screen], root, label)
        directory(frames)
        require({p.name for p in frames.iterdir()} == {'noonmark-' + screen + '-native.png'}, 'unexpected frame files')
        png(frames / ('noonmark-' + screen + '-native.png'), (2400, 1536))
        snapshot(session)
    before = snapshot(session)
    command(exe, ['-init-session', str(session), '-render-one', str(root / 'reuse-frames'), '-screen', 'day'], root, 'reuse', diagnostic='Cannot open isolated development session: invalid isolated development session')
    require(snapshot(session) == before and not (root / 'reuse-frames').exists(), 'reuse modified session')
    command(exe, [], root, 'implicit', diagnostic='Use -init-session or -session for development storage; -prototype for the static visual study')
    require(snapshot(session) == before, 'implicit modified session')


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('action', choices=['pack', 'verify', 'run'])
    p.add_argument('--target', required=True, choices=sorted(TARGETS))
    p.add_argument('--source', required=True)
    p.add_argument('--run-id', required=True)
    p.add_argument('--run-attempt', required=True)
    p.add_argument('--artifact', required=True, type=Path)
    p.add_argument('--binary', type=Path)
    p.add_argument('--work-parent', type=Path)
    a = p.parse_args()
    if a.action == 'pack':
        require(a.binary is not None, '--binary required')
        pack(a.binary, a.artifact, a.target, a.source, a.run_id, a.run_attempt)
        return
    require(a.artifact.is_absolute(), 'absolute artifact path required')
    exe = verify(a.artifact, a.target, a.source, a.run_id, a.run_attempt)
    if a.action == 'verify':
        return
    require(native_host() == a.target, 'host/target mismatch')
    require(a.work_parent is not None and a.work_parent.is_absolute() and a.work_parent.is_dir(), 'existing absolute external work parent required')
    parent = a.work_parent.resolve()
    repo = Path(__file__).resolve().parents[4]
    require(parent != repo and repo not in parent.parents, 'work parent inside source')
    root = Path(tempfile.mkdtemp(prefix='native-smoke-', dir=parent))
    result = {'target': a.target, 'source': a.source, 'run_id': a.run_id, 'run_attempt': a.run_attempt, 'host': platform.platform(), 'native_host': native_host(), 'binary_sha256': digest(exe), 'desktop_window_tested': False, 'ime_tested': False, 'passed': False}
    try:
        if os.name != 'nt':
            exe.chmod(0o700)
        smoke(exe, root)
        result['passed'] = True
    except Exception as e:
        result['failure'] = str(e)
        raise
    finally:
        (root / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
        print(str(root))


if __name__ == '__main__':
    main()
