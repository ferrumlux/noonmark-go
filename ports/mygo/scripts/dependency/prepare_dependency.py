#!/usr/bin/env python3
"""Offline, fail-closed candidate. Prints one absolute GOWORK path on success."""
import argparse
import base64
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import sys
import uuid
import zipfile

SCHEMA = 1
NAMESPACE = 'noonmark-patched-dependencies-v1'
OWNER = b'noonmark-patched-dependencies-v1\n'
MODULE_DIRS = ('.', 'domain-core', 'domain-extensions', 'domain-codec')
PROTECTED = {'go.mod', 'go.sum', 'LICENSE'}
MAX_FILE = 64 * 1024 * 1024
MAX_TREE = 128 * 1024 * 1024


class Refused(Exception):
    pass


def require(condition, message):
    if not condition:
        raise Refused(message)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def canonical(value):
    return (json.dumps(value, ensure_ascii=True, sort_keys=True, separators=(',', ':')) + '\n').encode()


def physical(path, directory=None):
    p = Path(path)
    require(p.is_absolute() and str(p) == os.path.normpath(str(p)), 'path must be absolute and normalized')
    require('..' not in p.parts, 'parent path component refused')
    cursor = Path(p.anchor)
    for part in p.parts[1:]:
        cursor /= part
        st = cursor.lstat()
        require(not stat.S_ISLNK(st.st_mode), 'symlink path refused: ' + str(cursor))
    st = p.lstat()
    if directory is not None:
        require(stat.S_ISDIR(st.st_mode) if directory else stat.S_ISREG(st.st_mode), 'unexpected path type: ' + str(p))
    return p


def read_file(path, limit=MAX_FILE):
    p = physical(path, False)
    before = p.lstat()
    require(before.st_nlink == 1, 'hardlinked input refused: ' + str(p))
    fd = os.open(p, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd, 'rb') as f:
        st = os.fstat(f.fileno())
        require((before.st_dev, before.st_ino) == (st.st_dev, st.st_ino), 'input replaced during read')
        require(stat.S_ISREG(st.st_mode) and st.st_nlink == 1 and st.st_size <= limit, 'unsafe input file')
        data = f.read(limit + 1)
        require(len(data) <= limit, 'input size limit exceeded')
        after = os.fstat(f.fileno())
        require((st.st_size, st.st_mtime_ns) == (after.st_size, after.st_mtime_ns), 'input changed during read')
        return data


def relative(name):
    require(isinstance(name, str) and bool(name) and '\\' not in name and '\x00' not in name, 'unsafe relative path')
    p = PurePosixPath(name)
    require(not p.is_absolute() and str(p) == name and all(x not in ('', '.', '..') for x in p.parts), 'unsafe relative path: ' + name)
    require(not any(c.isspace() for c in name), 'whitespace in archive/patch path refused')
    return name


def overlap(a, b):
    return a == b or a in b.parents or b in a.parents


def hash1(files):
    h = hashlib.sha256()
    for name in sorted(files):
        h.update((sha(files[name]) + '  ' + name + '\n').encode())
    return 'h1:' + base64.b64encode(h.digest()).decode()


def digest(value):
    require(isinstance(value, str) and re.fullmatch('[a-f0-9]{64}', value), 'invalid SHA256')
    return value


def checksum(value):
    require(isinstance(value, str) and re.fullmatch(r'h1:[A-Za-z0-9+/]{43}=', value), 'invalid Go h1')
    return value


def load_json(data):
    def pairs(items):
        out = {}
        for key, value in items:
            require(key not in out, 'duplicate JSON key')
            out[key] = value
        return out
    return json.loads(data, object_pairs_hook=pairs)


def read_manifest(path):
    data = read_file(path, 1024 * 1024)
    m = load_json(data)
    require(set(m) == {'schema', 'module', 'go', 'patches'}, 'unknown or missing manifest fields')
    require(m['schema'] == SCHEMA, 'unsupported manifest schema')
    require(re.fullmatch(r'1\.\d+\.\d+', m['go']), 'Go directive must be exact')
    mod = m['module']
    require(set(mod) == {'path', 'version', 'sum', 'gomod_sum', 'zip_sha256', 'license_sha256'}, 'invalid module fields')
    require(re.fullmatch(r'[a-z0-9][a-z0-9./_-]*', mod['path']) and '/' in mod['path'], 'unsupported module path (use lowercase canonical path)')
    require(re.fullmatch(r'v\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?', mod['version']), 'invalid pinned version')
    for key in ('zip_sha256', 'license_sha256'):
        digest(mod[key])
    for key in ('sum', 'gomod_sum'):
        checksum(mod[key])
    require(isinstance(m['patches'], list) and 0 < len(m['patches']) <= 100, 'nonempty bounded patch list required')
    patches = []
    seen = set()
    for entry in m['patches']:
        require(set(entry) == {'file', 'sha256', 'files'}, 'invalid patch entry')
        name = relative(entry['file'])
        require(name not in seen, 'duplicate patch name')
        seen.add(name)
        content = read_file(path.parent / name, 4 * 1024 * 1024)
        require(sha(content) == digest(entry['sha256']), 'patch SHA256 mismatch: ' + name)
        require(isinstance(entry['files'], dict) and entry['files'], 'patch file hashes required')
        for target, pair in entry['files'].items():
            relative(target)
            require(target not in PROTECTED, 'protected upstream file: ' + target)
            require(set(pair) == {'before', 'after'}, 'invalid patch target hash fields')
            require(pair['before'] is not None or pair['after'] is not None, 'empty target operation')
            for value in pair.values():
                if value is not None:
                    digest(value)
        patches.append((entry, content))
    return m, patches


def verify_go_directive(data, version):
    values = re.findall(rb'^go\s+(1\.\d+(?:\.\d+)?)\s*$', data, re.M)
    require(len(values) == 1, 'exactly one Go directive required')
    actual = tuple(int(x) for x in values[0].decode().split('.'))
    actual = actual + (0,) * (3 - len(actual))
    require(actual <= tuple(int(x) for x in version.split('.')), 'workspace Go version is too old for module')


def read_archive(path, m):
    data = read_file(path)
    mod = m['module']
    require(sha(data) == mod['zip_sha256'], 'archive SHA256 mismatch')
    prefix = mod['path'] + '@' + mod['version'] + '/'
    files = {}
    hash_files = {}
    total = 0
    with zipfile.ZipFile(io.BytesIO(data)) as z:
        require(len(z.infolist()) <= 20000, 'too many archive entries')
        for info in z.infolist():
            require(info.filename.startswith(prefix), 'archive module/version prefix mismatch')
            name = info.filename[len(prefix):]
            if info.is_dir():
                if name:
                    relative(name.rstrip('/'))
                continue
            relative(name)
            require(name not in files, 'duplicate archive path')
            mode = info.external_attr >> 16
            require(stat.S_IFMT(mode) in (0, stat.S_IFREG), 'non-regular archive entry')
            require(not (mode & 0o111), 'executable archive entry unsupported')
            total += info.file_size
            require(info.file_size <= MAX_FILE and total <= MAX_TREE, 'archive size limit exceeded')
            payload = z.read(info)
            files[name] = payload
            hash_files[info.filename] = payload
    require(hash1(hash_files) == mod['sum'], 'official module h1 mismatch')
    require('go.mod' in files and hash1({'go.mod': files['go.mod']}) == mod['gomod_sum'], 'go.mod h1 mismatch')
    require('LICENSE' in files and sha(files['LICENSE']) == mod['license_sha256'], 'LICENSE SHA256 mismatch')
    require(files['LICENSE'].startswith(b'MIT License\n'), 'MIT license expected')
    require(re.search(rb'^module\s+' + re.escape(mod['path'].encode()) + rb'\s*$', files['go.mod'], re.M), 'archive go.mod identity mismatch')
    for name in files:
        require(not any(str(parent) in files for parent in PurePosixPath(name).parents if str(parent) != '.'), 'archive file/directory collision')
    verify_go_directive(files['go.mod'], m['go'])
    return files


HUNK = re.compile(rb'@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@[^\r\n]*\n$')


def apply_patch(files, entry, data):
    require(b'\x00' not in data and data.endswith(b'\n'), 'binary or unterminated patch refused')
    lines = data.splitlines(keepends=True)
    i = 0
    updates = {}
    while i < len(lines):
        # Only standard textual git metadata; reject rename/mode-change/binary extensions.
        while i < len(lines) and not lines[i].startswith(b'--- '):
            line = lines[i]
            allowed = (line.startswith(b'diff --git a/') or re.fullmatch(rb'index [a-f0-9]+\.\.[a-f0-9]+(?: 100644)?\n', line) or line in (b'new file mode 100644\n', b'deleted file mode 100644\n'))
            require(bool(allowed), 'unsupported patch metadata')
            i += 1
        require(i + 1 < len(lines) and lines[i + 1].startswith(b'+++ '), 'missing diff headers')
        old = lines[i][4:-1].decode('utf-8')
        new = lines[i + 1][4:-1].decode('utf-8')
        require(old == '/dev/null' or old.startswith('a/'), 'invalid old patch path')
        require(new == '/dev/null' or new.startswith('b/'), 'invalid new patch path')
        old_name = None if old == '/dev/null' else relative(old[2:])
        new_name = None if new == '/dev/null' else relative(new[2:])
        target = new_name or old_name
        require(target and (not old_name or not new_name or old_name == new_name), 'rename refused')
        require(target in entry['files'] and target not in updates and target not in PROTECTED, 'undeclared/duplicate/protected patch target')
        pair = entry['files'][target]
        original = files.get(target)
        require((original is None) == (old_name is None), 'patch source existence mismatch')
        require((None if original is None else sha(original)) == pair['before'], 'source file SHA256 mismatch: ' + target)
        original_lines = (original or b'').splitlines(keepends=True)
        output = []
        cursor = 0
        i += 2
        hunk_count = 0
        while i < len(lines) and lines[i].startswith(b'@@ '):
            match = HUNK.fullmatch(lines[i])
            require(match is not None, 'malformed hunk')
            old_start, old_count, new_start, new_count = (int(match.group(n) or b'1') for n in range(1, 5))
            old_index = old_start if old_count == 0 else old_start - 1
            new_index = new_start if new_count == 0 else new_start - 1
            require(0 <= old_index <= len(original_lines) and old_index >= cursor, 'hunk source position invalid')
            output.extend(original_lines[cursor:old_index])
            require(len(output) == new_index, 'hunk new position mismatch (offset refused)')
            cursor = old_index
            i += 1
            operations = []
            while i < len(lines) and lines[i][:1] in (b' ', b'+', b'-'):
                # Stop at next file header after this hunk's declared line counts.
                if sum(op != b'+' for op, _ in operations) == old_count and sum(op != b'-' for op, _ in operations) == new_count:
                    break
                op, payload = lines[i][:1], lines[i][1:]
                i += 1
                if i < len(lines) and lines[i] == b'\\ No newline at end of file\n':
                    require(payload.endswith(b'\n'), 'invalid no-newline marker')
                    payload = payload[:-1]
                    i += 1
                operations.append((op, payload))
            require(sum(op != b'+' for op, _ in operations) == old_count and sum(op != b'-' for op, _ in operations) == new_count, 'hunk count mismatch')
            for op, payload in operations:
                if op != b'+':
                    require(cursor < len(original_lines) and original_lines[cursor] == payload, 'hunk content mismatch (fuzz refused): ' + target)
                    cursor += 1
                if op != b'-':
                    output.append(payload)
            hunk_count += 1
        require(hunk_count > 0, 'patch needs at least one hunk')
        output.extend(original_lines[cursor:])
        result = b''.join(output)
        if new_name is None:
            require(not result and pair['after'] is None, 'deletion must remove complete file')
            result = None
        else:
            require(sha(result) == pair['after'], 'patched file SHA256 mismatch: ' + target)
        updates[target] = result
    require(set(updates) == set(entry['files']), 'declared patch targets mismatch')
    for target, content in updates.items():
        if content is None:
            del files[target]
        else:
            files[target] = content


def parse_sums(data):
    sums = {}
    for line in data.decode().splitlines():
        parts = line.split()
        require(len(parts) == 3, 'malformed go.sum line')
        name, version, value = parts
        checksum(value)
        key = (name, version)
        require(key not in sums or sums[key] == value, 'conflicting go.sum values')
        sums[key] = value
    return sums


def workspace(source, repo, m, files):
    require(source != repo and repo in source.parents, 'app must be strictly inside explicit repo root')
    source_inputs = {}
    sums = parse_sums(files.get('go.sum', b''))
    module_paths = []
    for name in MODULE_DIRS:
        path = physical(source if name == '.' else source / name, True)
        mod = read_file(path / 'go.mod', 1024 * 1024)
        source_inputs[name + '/go.mod'] = sha(mod)
        verify_go_directive(mod, m['go'])
        parsed = re.findall(rb'^module\s+(\S+)\s*$', mod, re.M)
        require(len(parsed) == 1 and parsed[0] not in module_paths, 'missing or duplicate workspace module')
        module_paths.append(parsed[0])
        if name == '.':
            pin = re.escape((m['module']['path'] + ' ' + m['module']['version']).encode()).replace(b'\\ ', rb'\s+')
            require(re.search(rb'^\s*(?:require\s+)?' + pin + rb'(?:\s*//[^\n]*)?\s*$', mod, re.M), 'root go.mod official pin mismatch')
            require(not re.search(rb'^\s*(?:replace\s+)?' + re.escape(m['module']['path'].encode()) + rb'(?:\s|$).*=>', mod, re.M), 'root MyGo replace refused')
        sum_path = path / 'go.sum'
        if sum_path.exists() or sum_path.is_symlink():
            data = read_file(sum_path, 4 * 1024 * 1024)
            source_inputs[name + '/go.sum'] = sha(data)
            local = parse_sums(data)
            if name == '.':
                require(local.get((m['module']['path'], m['module']['version'])) == m['module']['sum'], 'root official module checksum mismatch')
                require(local.get((m['module']['path'], m['module']['version'] + '/go.mod')) == m['module']['gomod_sum'], 'root official go.mod checksum mismatch')
            for key, value in local.items():
                require(key not in sums or sums[key] == value, 'workspace checksum conflict')
                sums[key] = value
        else:
            require(name != '.', 'root go.sum required')
            source_inputs[name + '/go.sum'] = None
    sum_data = ''.join(f'{name} {version} {value}\n' for (name, version), value in sorted(sums.items())).encode()
    return source_inputs, sum_data


def write_exclusive(path, data, mode=0o400):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, mode)
    with os.fdopen(fd, 'wb') as f:
        f.write(data)
        f.flush()
        os.fsync(f.fileno())


def expected_dirs(paths):
    result = {'.'}
    for name in paths:
        result.update(str(p) for p in PurePosixPath(name).parents)
    return result


def verify_output(path, expected):
    physical(path, True)
    found = {}
    dirs = set()
    for root, children, names in os.walk(path, followlinks=False):
        root_path = Path(root)
        dirs.add(str(root_path.relative_to(path)))
        require(stat.S_IMODE(root_path.lstat().st_mode) == 0o500, 'output directory permissions changed')
        for name in children:
            physical(root_path / name, True)
        for name in names:
            p = root_path / name
            require(stat.S_IMODE(p.lstat().st_mode) == 0o400, 'output file permissions changed')
            found[str(p.relative_to(path))] = sha(read_file(p))
    require(found == {name: sha(data) for name, data in expected.items()}, 'output inventory/content mismatch')
    require(dirs == expected_dirs(expected), 'output directory inventory mismatch')


def prepare(manifest, archive, source_root, repo_root, build_root):
    manifest = physical(manifest, False)
    archive = physical(archive, False)
    source = physical(source_root, True)
    repo = physical(repo_root, True)
    build = physical(build_root, True)
    require(build != Path(build.anchor), 'filesystem root cannot be a build root')
    for other in (repo, manifest.parent):
        require(not overlap(build, other), 'build root overlaps source/bundle')
    require(build.stat().st_uid == os.getuid() and not build.stat().st_mode & 0o022, 'build root must be user-owned and not group/world writable')
    m, patches = read_manifest(manifest)
    namespace = build / NAMESPACE
    # Recognize the canonical Go download cache suffix; for custom archives,
    # conservatively treat the explicitly supplied parent as the cache boundary.
    suffix = ('cache', 'download', *m['module']['path'].split('/'), '@v', m['module']['version'] + '.zip')
    if archive.parts[-len(suffix):] == suffix:
        cache_root = Path(*archive.parts[:-len(suffix)])
    else:
        cache_root = archive.parent
    require(build != cache_root and cache_root not in build.parents, 'build root must not be inside the module cache')
    require(not overlap(namespace, cache_root), 'private namespace overlaps module cache')
    files = read_archive(archive, m)
    inputs, sum_data = workspace(source, repo, m, files)
    for entry, data in patches:
        apply_patch(files, entry, data)
    require(sha(files['LICENSE']) == m['module']['license_sha256'], 'license changed')
    recipe = {'schema': SCHEMA, 'manifest': m, 'helper_sha256': sha(read_file(Path(__file__).absolute())), 'source_root': str(source), 'repo_root': str(repo), 'source_inputs': inputs}
    recipe_id = sha(canonical(recipe))
    if not namespace.exists():
        namespace.mkdir(mode=0o700)
        write_exclusive(namespace / 'OWNER', OWNER)
    physical(namespace, True)
    require(namespace.stat().st_uid == os.getuid() and stat.S_IMODE(namespace.stat().st_mode) == 0o700, 'unsafe namespace ownership/mode')
    require(read_file(namespace / 'OWNER') == OWNER, 'unknown build namespace')
    output = namespace / recipe_id
    paths = [str(source if n == '.' else source / n) for n in MODULE_DIRS]
    replacement = 'replace ' + m['module']['path'] + ' ' + m['module']['version'] + ' => ' + json.dumps(str(output / 'mygo'))
    work = ('go ' + m['go'] + '\n\nuse (\n' + ''.join('\t' + json.dumps(p) + '\n' for p in paths) + ')\n\n' + replacement + '\n').encode()
    expected = {'mygo/' + name: data for name, data in files.items()}
    expected['go.work'] = work
    expected['go.work.sum'] = sum_data
    receipt = {'recipe': recipe, 'recipe_id': recipe_id, 'files': {name: sha(data) for name, data in expected.items()}}
    expected['receipt.json'] = canonical(receipt)
    lock = namespace / (recipe_id + '.lock')
    try:
        lock.mkdir(mode=0o700)
    except FileExistsError:
        raise Refused('recipe lock already exists; preparation is busy or previously failed; no automatic recovery')
    try:
        if output.exists() or output.is_symlink():
            verify_output(output, expected)
        else:
            stage = namespace / (recipe_id + '.partial-' + uuid.uuid4().hex)
            stage.mkdir(mode=0o700)
            for name in sorted(expected):
                destination = stage / name
                destination.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
                write_exclusive(destination, expected[name])
            for root, dirs, names in os.walk(stage, topdown=False):
                os.chmod(root, 0o500)
            verify_output(stage, expected)
            require(not output.exists() and not output.is_symlink(), 'output appeared unexpectedly')
            os.rename(stage, output)
            verify_output(output, expected)
    except Exception:
        # Preserve exact failed attempt and lock. Never repair or remove unknown data.
        raise
    else:
        lock.rmdir()  # Only this invocation's newly-created, empty coordination lock.
    return output / 'go.work'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for flag in ('manifest', 'archive', 'source-root', 'repo-root', 'build-root'):
        parser.add_argument('--' + flag, required=True)
    args = parser.parse_args()
    try:
        print(prepare(args.manifest, args.archive, args.source_root, args.repo_root, args.build_root))
    except (Refused, OSError, ValueError, KeyError, TypeError, zipfile.BadZipFile, UnicodeError) as error:
        print('prepare_dependency: refused: ' + str(error), file=sys.stderr)
        return 2
    return 0


if __name__ == '__main__':
    sys.exit(main())
