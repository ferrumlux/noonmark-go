#!/usr/bin/env python3
"""Deterministic, external-only unsigned packaging and fail-closed release gates."""
import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import subprocess
import sys
import tarfile
import zipfile

ROOT = Path(__file__).resolve().parents[4]
APP = ROOT / 'ports/mygo'
APP_REL = 'ports/mygo'
APPROVAL = APP_REL + '/release/approval.json'
EVIDENCE = APP_REL + '/release/acceptance/'
TARGETS = tuple(f'{system}/{arch}' for system in ('linux', 'windows', 'darwin') for arch in ('amd64', 'arm64'))
REQUIREMENTS = ('feature-parity', 'storage-compatibility', 'privacy-security') + tuple('native-' + x.replace('/', '-') for x in TARGETS)
GO_VERSION = 'go1.27.3'
REPOSITORY = 'ferrumlux/noonmark-go'
UPSTREAM = '43e6eefaae3a04be89cab3e234d470111afd80ef'
STABLE = re.compile(r'v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\Z')
VERSION = re.compile(r'v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-dev\.[0-9a-f]{12})?\Z')


class Refused(ValueError):
    pass


def require(condition, message):
    if not condition:
        raise Refused(message)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def canonical(value):
    return (json.dumps(value, sort_keys=True, ensure_ascii=False, indent=2) + '\n').encode()


def run(args, cwd=ROOT, env=None):
    return subprocess.check_output([str(x) for x in args], cwd=cwd, env=env)


def decode_jsons(data):
    decoder = json.JSONDecoder()
    text = data.decode()
    while text.strip():
        text = text.lstrip()
        value, offset = decoder.raw_decode(text)
        yield value
        text = text[offset:]


def safe_name(name):
    p = PurePosixPath(name)
    require(bool(name) and not p.is_absolute() and str(p) == name and '..' not in p.parts
            and '\\' not in name and '\x00' not in name, 'unsafe archive path')
    return name


def physical(path):
    p = Path(path)
    require(p.is_absolute() and str(p) == os.path.normpath(str(p)), 'absolute normalized path required')
    cursor = Path(p.anchor)
    for part in p.parts[1:]:
        cursor /= part
        require(not cursor.is_symlink(), 'symlink path refused')
    return p


def external(path, root=ROOT):
    p = physical(path)
    require(p != root and root not in p.parents, 'output must be outside source repository')
    return p


def git_files(root=ROOT):
    require(not run(['git', 'status', '--porcelain', '--untracked-files=all'], root).strip(),
            'packaging requires clean committed source, including untracked files')
    entries = {}
    for record in run(['git', 'ls-tree', '-rz', '--full-tree', 'HEAD'], root).split(b'\0'):
        if not record:
            continue
        meta, name = record.split(b'\t', 1)
        mode, kind, oid = meta.decode().split()
        name = safe_name(name.decode())
        require(kind == 'blob' and mode in ('100644', '100755'), 'symlink/submodule source refused')
        entries[name] = (run(['git', 'cat-file', 'blob', oid], root), int(mode, 8) & 0o777)
    return entries


def source_digest(files):
    # Approval and acceptance reports are added after testing this precise input tree.
    h = hashlib.sha256()
    for name, (data, mode) in sorted(files.items()):
        if name == APPROVAL or name.startswith(EVIDENCE):
            continue
        h.update(canonical([name, mode, sha(data)]))
    return h.hexdigest()


def release_gate(files, version):
    require(bool(STABLE.fullmatch(version)), 'stable release tag must be vMAJOR.MINOR.PATCH')
    require(APPROVAL in files, 'missing release approval')
    approval = json.loads(files[APPROVAL][0])
    require(set(approval) == {'schema', 'approved', 'version', 'source_sha256', 'evidence', 'reason'}, 'invalid approval schema')
    require(approval['schema'] == 1 and approval['approved'] is True, 'stable release is not approved')
    require(approval['version'] == version, 'approved version does not match tag')
    digest = source_digest(files)
    require(approval['source_sha256'] == digest, 'approval is stale for this source')
    require(set(approval['evidence']) == set(REQUIREMENTS), 'complete feature, privacy, compatibility and six-target evidence required')
    for name, entry in approval['evidence'].items():
        require(set(entry) == {'path', 'sha256'}, 'invalid evidence reference')
        path = safe_name(entry['path'])
        require(path.startswith(EVIDENCE) and path in files, 'acceptance evidence must be tracked in release/acceptance')
        payload = files[path][0]
        require(sha(payload) == entry['sha256'], 'acceptance evidence hash mismatch')
        report = json.loads(payload)
        require(report.get('schema') == 1 and report.get('requirement') == name and report.get('result') == 'passed', 'acceptance report is not a pass')
        require(report.get('source_sha256') == digest, 'acceptance report tests a different source')
        require(isinstance(report.get('commands'), list) and report['commands']
                and all(isinstance(x, str) and x.strip() for x in report['commands']), 'acceptance report lacks executed commands')
        require(isinstance(report.get('observations'), str) and report['observations'].strip(), 'acceptance report lacks observations')
        if name.startswith('native-'):
            require(report.get('execution') == 'native-desktop', 'compile/headless results cannot approve native desktop acceptance')
    return approval


def tar_bytes(entries):
    raw = io.BytesIO()
    with gzip.GzipFile(fileobj=raw, mode='wb', filename='', mtime=0, compresslevel=9) as gz:
        with tarfile.open(fileobj=gz, mode='w', format=tarfile.PAX_FORMAT) as tar:
            for name, (data, mode) in sorted(entries.items()):
                info = tarfile.TarInfo(safe_name(name))
                info.size, info.mode, info.mtime = len(data), mode, 0
                info.uid = info.gid = 0
                info.uname = info.gname = ''
                tar.addfile(info, io.BytesIO(data))
    return raw.getvalue()


def zip_bytes(entries):
    out = io.BytesIO()
    with zipfile.ZipFile(out, 'w', zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        for name, (data, mode) in sorted(entries.items()):
            info = zipfile.ZipInfo(safe_name(name), date_time=(1980, 1, 1, 0, 0, 0))
            info.create_system = 3
            info.external_attr = (stat.S_IFREG | mode) << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            z.writestr(info, data, compresslevel=9)
    return out.getvalue()


def tree_files(path):
    out = {}
    for p in sorted(path.rglob('*')):
        require(not p.is_symlink(), 'dependency symlink refused')
        if p.is_dir():
            continue
        require(p.is_file(), 'non-regular dependency entry refused')
        out[p.relative_to(path).as_posix()] = (p.read_bytes(), 0o755 if p.stat().st_mode & 0o111 else 0o644)
    return out


def license_name(name):
    base = PurePosixPath(name).name
    if base.lower().endswith(('.go', '.c', '.h', '.py', '.js', '.ts')):
        return False
    return re.search(r'(?i)(^|[-_.])(licen[cs]e|copying|notice|patents|authors|copyright|ofl)(\b|[._-])', base) is not None


def dependency_sources(go, env):
    modules = {}
    for target in TARGETS:
        system, arch = target.split('/')
        target_env = dict(env, GOOS=system, GOARCH=arch)
        for pkg in decode_jsons(run([go, 'list', '-deps', '-json', './...'], APP, target_env)):
            mod = pkg.get('Module')
            if not mod:
                continue
            selected = mod.get('Replace', mod)
            path = Path(selected['Dir'])
            if path == ROOT or ROOT in path.parents:
                continue
            identity = mod['Path']
            record = {'path': identity, 'version': mod.get('Version', '(workspace)'), 'sum': mod.get('Sum'),
                      'source': path, 'patched': identity == 'github.com/egoist/mygo'}
            if identity in modules:
                require(modules[identity] == record, 'dependency selection changed across targets')
            modules[identity] = record
    require('github.com/egoist/mygo' in modules, 'patched MyGo dependency missing')
    source, licenses, metadata = {}, {}, []
    for index, (name, record) in enumerate(sorted(modules.items())):
        directory = f'dependencies/{index:02d}-{name.rsplit("/", 1)[-1]}'
        contents = tree_files(record.pop('source'))
        notices = {n: entry for n, entry in contents.items() if license_name(n)}
        require(any(PurePosixPath(n).name.upper().startswith(('LICENSE', 'LICENCE', 'COPYING')) for n in notices), 'missing dependency license: ' + name)
        source.update({directory + '/' + n: entry for n, entry in contents.items()})
        licenses.update({'licenses/' + directory + '/' + n: entry for n, entry in notices.items()})
        record['directory'] = directory
        record['source_sha256'] = sha(canonical([[n, mode, sha(data)] for n, (data, mode) in sorted(contents.items())]))
        record['notices'] = {n: sha(entry[0]) for n, entry in sorted(notices.items())}
        metadata.append(record)
    return source, licenses, metadata


def write_new(path, payload):
    with path.open('xb') as f:
        f.write(payload)


def validate_binary(data, target):
    system, arch = target.split('/')
    if system == 'linux':
        require(data[:5] == b'\x7fELF\x02' and data[5] == 1, 'not a 64-bit little endian ELF')
        require(int.from_bytes(data[18:20], 'little') == {'amd64': 62, 'arm64': 183}[arch], 'wrong ELF architecture')
    elif system == 'windows':
        require(data[:2] == b'MZ' and len(data) > 64, 'not a PE executable')
        pos = int.from_bytes(data[60:64], 'little')
        require(data[pos:pos+4] == b'PE\0\0', 'missing PE header')
        require(int.from_bytes(data[pos+4:pos+6], 'little') == {'amd64': 0x8664, 'arm64': 0xaa64}[arch], 'wrong PE architecture')
    else:
        require(data[:4] == b'\xcf\xfa\xed\xfe', 'not a 64-bit Mach-O executable')
        require(int.from_bytes(data[4:8], 'little') == {'amd64': 0x01000007, 'arm64': 0x0100000c}[arch], 'wrong Mach-O architecture')


def package(args):
    commit = run(['git', 'rev-parse', 'HEAD']).decode().strip()
    files = git_files()
    require(run(['git', 'rev-parse', 'HEAD']).decode().strip() == commit, 'source commit moved during snapshot')
    version = args.version or 'v0.0.0-dev.' + commit[:12]
    require(bool(VERSION.fullmatch(version)), 'invalid version')
    stable = bool(STABLE.fullmatch(version))
    if stable:
        release_gate(files, version)
    else:
        require(version == 'v0.0.0-dev.' + commit[:12], 'candidate version must identify this exact commit')
    base = external(os.environ['NOONMARK_MYGO_BUILD_ROOT'])
    require(base.is_dir(), 'build root must already exist')
    output = external(args.output) if args.output else base / ('package-' + version)
    require(base in output.parents, 'package output must be within external build root')
    output.mkdir()  # No replacement/cleanup of a previous package.
    go = os.environ.get('NOONMARK_GO', 'go')
    env = dict(os.environ, CGO_ENABLED='0', GOAMD64='v1', GOARM64='v8.0', GOFLAGS='-mod=readonly')
    require(run([go, 'env', 'GOVERSION'], APP, env).decode().strip() == GO_VERSION, 'wrong Go toolchain')
    run([go, 'mod', 'verify'], APP, env)
    source, licenses, dependencies = dependency_sources(go, env)
    for name, entry in files.items():
        if license_name(name):
            licenses['licenses/project/' + name] = entry
    require('LICENSE' in files and APP_REL + '/LICENSE' in files, 'AGPL license missing')
    patch = json.loads(files[APP_REL + '/patches/mygo/native-finalize/manifest.json'][0])
    for dep in dependencies:
        if dep['patched']:
            dep['sum'] = patch['module']['sum']
    metadata = {'schema': 1, 'name': 'Noonmark Go', 'version': version, 'channel': 'stable' if stable else 'candidate',
                'commit': commit, 'source_sha256': source_digest(files), 'repository': 'https://github.com/' + REPOSITORY,
                'upstream': {'repository': 'https://github.com/quboliu/noonmark', 'commit': UPSTREAM},
                'license': 'AGPL-3.0-only', 'project_notices': {n: sha(entry[0]) for n, entry in files.items() if license_name(n)},
                'go': GO_VERSION, 'targets': list(TARGETS),
                'build_flags': ['-p=1', '-trimpath', '-buildvcs=false', '-ldflags=-buildid='],
                'cgo_enabled': False, 'publisher_signed': False, 'native_runtime_verified_by_packaging': False,
                'dependencies': dependencies, 'mygo_patch_manifest': patch}
    source.update({'repository/' + n: entry for n, entry in files.items()})
    source['SOURCE.json'] = canonical(metadata), 0o644
    source['BUILD.md'] = files[APP_REL + '/docs/release/SOURCE-BUILD.md']
    source['build.py'] = files[APP_REL + '/scripts/release/rebuild_source.py']
    uses = ['.', 'domain-core', 'domain-codec', 'domain-extensions']
    work = 'go 1.27.3\n\nuse (\n' + ''.join('  ./repository/ports/mygo/' + x + '\n' for x in uses) + ')\n\nreplace (\n'
    work += ''.join('  ' + d['path'] + ' => ./' + d['directory'] + '\n' for d in dependencies) + ')\n'
    source['go.work'] = work.encode(), 0o644
    source_name = f'noonmark-go-{version}-source.tar.gz'
    write_new(output / source_name, tar_bytes(source))
    common = dict(licenses)
    common['LICENSE'] = files['LICENSE']
    common['SOURCE.json'] = canonical(dict(metadata, corresponding_source=source_name)), 0o644
    common['README.md'] = files[APP_REL + '/docs/release/PACKAGE.md']
    binary_records = {}
    staging = output / 'source'
    staging.mkdir()
    for name, (data, mode) in source.items():
        path = staging / name
        path.parent.mkdir(parents=True, exist_ok=True)
        write_new(path, data)
        path.chmod(mode)
    builddir = output / 'build'
    subprocess.run([sys.executable, '-B', str(staging / 'build.py'), '--output', str(builddir), '--go', go], check=True)
    for target in TARGETS:
        system, arch = target.split('/')
        binary = 'noonmark-mygo' + ('.exe' if system == 'windows' else '')
        path = builddir / ('noonmark-mygo-' + target.replace('/', '-') + ('.exe' if system == 'windows' else ''))
        data = path.read_bytes()
        validate_binary(data, target)
        binary_records[target] = {'sha256': sha(data), 'size': len(data)}
        entries = dict(common, **{binary: (data, 0o755)})
        entries['TARGET.json'] = canonical({'target': target, **binary_records[target]}), 0o644
        name = f'noonmark-go-{version}-{system}-{arch}'
        writer, extension = (zip_bytes, '.zip') if system == 'windows' else (tar_bytes, '.tar.gz')
        write_new(output / (name + extension), writer(entries))
    artifacts = {p.name: {'sha256': sha(p.read_bytes()), 'size': p.stat().st_size} for p in sorted(output.iterdir()) if p.is_file()}
    manifest = dict(metadata, binaries=binary_records, artifacts=artifacts)
    write_new(output / 'manifest.json', canonical(manifest))
    checksums = ''.join(sha(p.read_bytes()) + '  ' + p.name + '\n' for p in sorted(output.iterdir()) if p.is_file())
    write_new(output / 'SHA256SUMS', checksums.encode())
    verify(output)
    require(not run(['git', 'status', '--porcelain', '--untracked-files=all']).strip(), 'build modified source')
    require(run(['git', 'rev-parse', 'HEAD']).decode().strip() == commit, 'source commit moved during packaging')
    print('PACKAGED ' + str(output), flush=True)


def archive_entries(path):
    if path.suffix == '.zip':
        with zipfile.ZipFile(path) as z:
            items = [(x.filename, z.read(x), x.external_attr >> 16) for x in z.infolist()]
    else:
        with tarfile.open(path, 'r:gz') as tar:
            items = []
            for x in tar:
                require(x.isfile(), 'archive has non-file entry')
                items.append((x.name, tar.extractfile(x).read(), x.mode))
    result = {}
    for name, data, mode in items:
        safe_name(name)
        require(name not in result and not stat.S_ISLNK(mode), 'duplicate or symlink archive member')
        result[name] = (data, mode & 0o777)
    return result


def verify(path):
    output = physical(Path(path).absolute())
    manifest = json.loads((output / 'manifest.json').read_bytes())
    require(manifest['schema'] == 1 and manifest['targets'] == list(TARGETS), 'invalid target manifest')
    require(re.fullmatch('[0-9a-f]{40}', manifest['commit']) is not None, 'invalid source commit')
    require(manifest['publisher_signed'] is False and manifest['native_runtime_verified_by_packaging'] is False, 'unsupported signing/runtime claim')
    base_metadata = {k: v for k, v in manifest.items() if k not in ('artifacts', 'binaries')}
    require(manifest['license'] == 'AGPL-3.0-only' and manifest['go'] == GO_VERSION, 'wrong license or toolchain')
    version = manifest['version']
    expected = {'manifest.json'} | set(manifest['artifacts'])
    sums = {}
    for line in (output / 'SHA256SUMS').read_text().splitlines():
        digest, name = line.split('  ')
        safe_name(name)
        require('/' not in name and name not in sums and re.fullmatch('[0-9a-f]{64}', digest), 'invalid checksum entry')
        sums[name] = digest
    require(set(sums) == expected, 'incomplete or extra checksums')
    actual = {p.name for p in output.iterdir() if p.is_file()}
    require(actual == expected | {'SHA256SUMS'}, 'unexpected or missing package files')
    for name, digest in sums.items():
        require(sha((output / name).read_bytes()) == digest, 'checksum mismatch: ' + name)
    source_name = f'noonmark-go-{version}-source.tar.gz'
    required_artifacts = {source_name}
    for target in TARGETS:
        system, arch = target.split('/')
        name = f'noonmark-go-{version}-{system}-{arch}' + ('.zip' if system == 'windows' else '.tar.gz')
        required_artifacts.add(name)
        entries = archive_entries(output / name)
        binary = 'noonmark-mygo' + ('.exe' if system == 'windows' else '')
        require(all(n in entries for n in (binary, 'LICENSE', 'SOURCE.json', 'TARGET.json', 'README.md')), 'binary package missing required metadata')
        data, mode = entries[binary]
        validate_binary(data, target)
        require(mode == 0o755 and sha(data) == manifest['binaries'][target]['sha256'], 'binary identity mismatch')
        require(json.loads(entries['SOURCE.json'][0]) == dict(base_metadata, corresponding_source=source_name), 'source metadata mismatch')
        require(json.loads(entries['TARGET.json'][0]) == dict(manifest['binaries'][target], target=target), 'target metadata mismatch')
        require(len(data) == manifest['binaries'][target]['size'], 'binary size mismatch')
        require(sha(entries['LICENSE'][0]) == manifest['project_notices']['LICENSE'], 'root license mismatch')
        for notice, digest in manifest['project_notices'].items():
            name = 'licenses/project/' + safe_name(notice)
            require(name in entries and sha(entries[name][0]) == digest, 'project notice missing or changed')
        for dep in manifest['dependencies']:
            for notice, digest in dep['notices'].items():
                name = 'licenses/' + safe_name(dep['directory']) + '/' + safe_name(notice)
                require(name in entries and sha(entries[name][0]) == digest, 'binary dependency notice missing or changed')
    require(set(manifest['artifacts']) == required_artifacts, 'expected exactly six binaries and corresponding source')
    for name, item in manifest['artifacts'].items():
        data = (output / name).read_bytes()
        require(item == {'sha256': sha(data), 'size': len(data)}, 'manifest artifact mismatch')
    source = archive_entries(output / source_name)
    require('repository/LICENSE' in source and 'build.py' in source and 'SOURCE.json' in source, 'corresponding source incomplete')
    require(json.loads(source['SOURCE.json'][0]) == base_metadata, 'corresponding source metadata mismatch')
    repository = {n[len('repository/'):]: entry for n, entry in source.items() if n.startswith('repository/')}
    require(source_digest(repository) == manifest['source_sha256'], 'corresponding source digest mismatch')
    for notice, digest in manifest['project_notices'].items():
        require(notice in repository and sha(repository[notice][0]) == digest, 'corresponding project notice missing or changed')
    if manifest['channel'] == 'stable':
        release_gate(repository, version)
    else:
        require(manifest['channel'] == 'candidate' and version == 'v0.0.0-dev.' + manifest['commit'][:12], 'invalid candidate identity')
    require(any(d['path'] == 'github.com/egoist/mygo' and d['patched'] for d in manifest['dependencies']), 'patched MyGo source missing')
    for dep in manifest['dependencies']:
        directory = safe_name(dep['directory'])
        require(directory + '/go.mod' in source, 'dependency source missing')
        contents = {n[len(directory)+1:]: entry for n, entry in source.items() if n.startswith(directory + '/')}
        digest = sha(canonical([[n, mode, sha(data)] for n, (data, mode) in sorted(contents.items())]))
        require(digest == dep['source_sha256'], 'dependency source digest mismatch')
        for notice, digest in dep['notices'].items():
            require(notice in contents and sha(contents[notice][0]) == digest, 'dependency license missing or changed')
    print('VERIFIED six binary archives, corresponding source, licenses and checksums', flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    p = sub.add_parser('package'); p.add_argument('--version'); p.add_argument('--output', type=Path)
    p = sub.add_parser('verify'); p.add_argument('directory', type=Path)
    p = sub.add_parser('gate'); p.add_argument('version')
    sub.add_parser('source-digest')
    args = parser.parse_args()
    try:
        if args.command == 'package': package(args)
        elif args.command == 'verify': verify(args.directory)
        elif args.command == 'gate': release_gate(git_files(), args.version); print('Stable release approval passed')
        else: print(source_digest(git_files()))
    except (Refused, OSError, KeyError, json.JSONDecodeError, subprocess.CalledProcessError) as error:
        print('REFUSED: ' + str(error), file=sys.stderr)
        return 2
    return 0


if __name__ == '__main__':
    sys.exit(main())
