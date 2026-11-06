#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""只捕获真实 macOS 上游 Swift 行为，不产生 Go 互通结论。"""
from pathlib import Path
import hashlib
import json
import os
import platform
import re
import subprocess
import sys

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
COMMIT = '43e6eefaae3a04be89cab3e234d470111afd80ef'
UPSTREAM = 'https://github.com/quboliu/noonmark'
DEVELOPER = '/Applications/Xcode_26.2.app/Contents/Developer'
MAX_INPUT = 2 * 1024 * 1024
MAX_EVIDENCE = 16 * 1024 * 1024
WORKFLOW_SHA = '61f45789c162e12d0836d79b87fa8481c9472ebd9566beb07b0fdd457615d04b'
MANIFEST_SHA = 'a62e1974c004a9509f5cd48c9a3f1cc326fbab089719126d20c84cf608e2860d'
VECTORS_SHA = '564fe5cde3f4d30b946d8153ce48d899380e41d79be43cb2a2fd6cbee8ca687d'
TEXT_FIELDS = ('input', 'displayName', 'canonicalKey', 'displayNameOfDisplayName', 'canonicalKeyOfDisplayName')

class GuardError(Exception):
    pass

def require(condition, message):
    if not condition:
        raise GuardError(message)

def sha(data):
    return hashlib.sha256(data).hexdigest()

def read_checked(path, digest, limit):
    require(path.is_file() and not path.is_symlink(), 'input_not_regular')
    require(path.stat().st_size <= limit, 'input_over_limit')
    data = path.read_bytes()
    require(sha(data) == digest, 'input_digest_mismatch')
    return data

def validate_manifest(value):
    require(set(value) == {'schemaVersion', 'repository', 'commit', 'rootTree', 'coreTree', 'files'}, 'manifest_shape')
    require(value['schemaVersion'] == 1 and value['commit'] == COMMIT and value['repository'] == UPSTREAM, 'manifest_source')
    require(value['rootTree'] == 'eca47c1c210b8b0d53556c955754710f5f2188c0', 'manifest_root_tree')
    require(value['coreTree'] == 'a1aa0252c8839d01dc69e47e05f2e69eceebc97d', 'manifest_core_tree')
    paths = [entry['path'] for entry in value['files']]
    require(len(paths) == len(set(paths)) == 16, 'manifest_files')
    require(paths[:2] == ['Package.swift', 'LICENSE'], 'manifest_metadata')
    require(all(re.fullmatch(r'Sources/NoonmarkCore/[A-Za-z]+\.swift', p) for p in paths[2:]), 'manifest_path')
    for entry in value['files']:
        require(set(entry) == {'path', 'sha256', 'bytes', 'gitBlob'}, 'manifest_entry')
        require(re.fullmatch('[0-9a-f]{64}', entry['sha256']) is not None, 'manifest_sha')
        require(re.fullmatch('[0-9a-f]{40}', entry['gitBlob']) is not None, 'manifest_blob')
        require(type(entry['bytes']) is int and 0 < entry['bytes'] < 4 * 1024 * 1024, 'manifest_size')
    return value

def validate_vectors(value):
    require(set(value) == {'schemaVersion', 'provenance', 'cases', 'pairs'}, 'vectors_shape')
    require(value['schemaVersion'] == 1 and value['provenance'] == 'fixed-synthetic-only', 'vectors_provenance')
    require(1 <= len(value['cases']) <= 128 and len(value['pairs']) <= 128, 'vectors_count')
    ids = set()
    for case in value['cases']:
        require(set(case) == {'id', 'input'} and re.fullmatch('[a-z0-9-]{1,80}', case['id']) is not None, 'vector_id')
        require(case['id'] not in ids, 'vector_duplicate')
        ids.add(case['id'])
        require(isinstance(case['input'], str) and len(case['input'].encode('utf-8')) <= 1024 * 1024, 'vector_length')
    pair_ids = set()
    for pair in value['pairs']:
        require(set(pair) == {'id', 'left', 'right'}, 'pair_shape')
        require(re.fullmatch('[a-z0-9-]{1,160}', pair['id']) is not None and pair['id'] not in pair_ids, 'pair_id')
        require(pair['left'] in ids and pair['right'] in ids, 'pair_reference')
        pair_ids.add(pair['id'])
    return value

def paths_overlap(a, b):
    return a == b or a in b.parents or b in a.parents

def prepare_root(env, repository):
    # macOS 的 /var -> /private/var 是 OS 既有链接；先解析系统父目录，再拒绝目标链接。
    raw = env.get('RUNNER_TEMP', '')
    require(raw and Path(raw).is_absolute(), 'runner_temp_required')
    base = Path(raw).resolve(strict=True)
    require(base.is_dir() and not paths_overlap(base, repository.resolve()), 'runner_temp_overlaps_source')
    work = base / 'noonmark-swift-name-oracle'
    require(not work.exists() and not work.is_symlink(), 'work_root_must_be_new')
    work.mkdir(mode=0o700)
    for child in ('evidence', 'logs', 'home', 'tmp', 'cache', 'build', 'source'):
        (work / child).mkdir(mode=0o700)
    return work

def clean_environment(work):
    return {
        'PATH': '/usr/bin:/bin:/usr/sbin:/sbin', 'HOME': str(work / 'home'),
        'TMPDIR': str(work / 'tmp'), 'XDG_CACHE_HOME': str(work / 'cache'),
        'CLANG_MODULE_CACHE_PATH': str(work / 'cache' / 'clang'),
        'SWIFT_MODULECACHE_PATH': str(work / 'cache' / 'swift'),
        'CFFIXED_USER_HOME': str(work / 'home'), 'SQLITE_TMPDIR': str(work / 'tmp'),
        'XCRUN_CACHE_PATH': str(work / 'cache' / 'xcrun'),
        'DEVELOPER_DIR': DEVELOPER, 'LANG': 'en_US.UTF-8', 'LC_ALL': 'en_US.UTF-8',
        'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': '/dev/null',
        'GIT_TERMINAL_PROMPT': '0', 'GIT_ATTR_NOSYSTEM': '1',
        'GIT_CONFIG_COUNT': '0', 'GIT_CEILING_DIRECTORIES': str(work),
        'PYTHONDONTWRITEBYTECODE': '1',
    }

def validate_text(record):
    require(set(record) == {'text', 'utf8Hex', 'unicodeScalars', 'characterCount', 'characters'}, 'text_shape')
    text = record['text']
    require(isinstance(text, str), 'text_type')
    require(text.encode('utf-8').hex() == record['utf8Hex'], 'utf8_mismatch')
    require([ord(c) for c in text] == record['unicodeScalars'], 'scalars_mismatch')
    characters = record['characters']
    require(type(record['characterCount']) is int and len(characters) == record['characterCount'], 'character_count')
    cursor = 0
    for character in characters:
        require(set(character) == {'startScalar', 'endScalar', 'isWhitespace'}, 'character_shape')
        require(type(character['startScalar']) is int and type(character['endScalar']) is int, 'character_type')
        require(character['startScalar'] == cursor and cursor < character['endScalar'] <= len(text), 'character_range')
        require(type(character['isWhitespace']) is bool, 'whitespace_type')
        cursor = character['endScalar']
    require(cursor == len(text), 'character_coverage')

def validate_capture(capture, vectors):
    require(set(capture) == {'schemaVersion', 'algorithmVersion', 'icuVersion', 'icuUnicodeVersion', 'cases', 'pairs', 'entityRoundTrips'}, 'capture_shape')
    require(capture['schemaVersion'] == 1, 'capture_schema')
    for field in ('icuVersion', 'icuUnicodeVersion'):
        require(re.fullmatch(r'\d+\.\d+\.\d+\.\d+', capture[field]) is not None, 'version_format')
    require(capture['algorithmVersion'] == 'unicode-' + capture['icuUnicodeVersion'] + '-nfkc-casefold-whitespace-v1', 'algorithm_version')
    require([x['id'] for x in capture['cases']] == [x['id'] for x in vectors['cases']], 'capture_cases')
    records = {}
    for case, vector in zip(capture['cases'], vectors['cases']):
        require(set(case) == {'id', *TEXT_FIELDS}, 'case_shape')
        for field in TEXT_FIELDS:
            validate_text(case[field])
        require(case['input']['text'] == vector['input'], 'capture_input_changed')
        records[case['id']] = case
    require(len(capture['pairs']) == len(vectors['pairs']), 'capture_pairs')
    for record, vector in zip(capture['pairs'], vectors['pairs']):
        require(set(record) == {'id', 'left', 'right', 'input', 'displayName', 'canonicalKey'}, 'pair_capture_shape')
        require(all(record[key] == vector[key] for key in ('id', 'left', 'right')), 'pair_identity')
        for field in ('input', 'displayName', 'canonicalKey'):
            result = record[field]
            require(set(result) == {'swiftEqual', 'utf8Equal'} and all(type(x) is bool for x in result.values()), 'equality_shape')
            expected = records[record['left']][field]['utf8Hex'] == records[record['right']][field]['utf8Hex']
            require(result['utf8Equal'] == expected, 'pair_bytes_mismatch')
    expected_entities = [(v['id'], kind) for v in vectors['cases'] if v['id'] in {'ascii', 'empty', 'only-di', 'display-non-idempotent', 'space-mark', 'compat-diaeresis'} for kind in ('TaskCategory', 'TaskLabel')]
    require([(r['caseID'], r['entity']) for r in capture['entityRoundTrips']] == expected_entities, 'entity_cases')
    for entity in capture['entityRoundTrips']:
        validate_text(entity['name']); validate_text(entity['canonicalKey'])
        require(entity['canonicalKeyVersion'] == capture['algorithmVersion'], 'entity_version')
        require(type(entity['encodedByteCount']) is int and 0 < entity['encodedByteCount'] < 65536, 'entity_size')
        require(entity['decodeStatus'] in ('accepted', 'rejected'), 'entity_status')
        if entity['decodeStatus'] == 'accepted':
            require(all(type(entity[k]) is bool for k in ('roundTripEqual', 'nameUTF8Equal', 'keyUTF8Equal')), 'entity_equality')
            require(set(entity) == {'caseID', 'entity', 'name', 'canonicalKey', 'canonicalKeyVersion', 'encodedByteCount', 'decodeStatus', 'roundTripEqual', 'nameUTF8Equal', 'keyUTF8Equal'}, 'entity_shape')
        else:
            require(entity['decodeErrorKind'] in ('dataCorrupted', 'keyNotFound', 'typeMismatch', 'valueNotFound', 'other'), 'entity_error')
            require(set(entity) <= {'caseID', 'entity', 'name', 'canonicalKey', 'canonicalKeyVersion', 'encodedByteCount', 'decodeStatus', 'decodeErrorKind', 'decodeErrorDescription'}, 'entity_shape')
            require(len(entity.get('decodeErrorDescription', '')) <= 256, 'entity_error_length')
    return capture

def comparison_status(unicode_version):
    # 版本一致也不等于分段版本一致，更不等于已完成 Go 比对。
    return 'not_performed' if unicode_version == '16.0.0.0' else 'unsupported'

def write_json(path, value, limit):
    data = (json.dumps(value, ensure_ascii=False, separators=(',', ':')) + '\n').encode('utf-8')
    require(len(data) <= limit, 'evidence_over_limit')
    with path.open('xb') as handle:
        handle.write(data)

def main():
    require(len(sys.argv) == 1, 'no_runtime_arguments')
    manifest_bytes = read_checked(HERE / 'source-manifest.json', MANIFEST_SHA, 16384)
    vectors_bytes = read_checked(HERE / 'vectors.json', VECTORS_SHA, MAX_INPUT)
    read_checked(ROOT / '.github/workflows/swift-name-oracle.yml', WORKFLOW_SHA, 8192)
    manifest = validate_manifest(json.loads(manifest_bytes))
    vectors = validate_vectors(json.loads(vectors_bytes))
    require(platform.system() == 'Darwin', 'macos_required_not_swift_verified')
    require(os.environ.get('GITHUB_ACTIONS') == 'true' and os.environ.get('RUNNER_ENVIRONMENT') == 'github-hosted', 'github_hosted_required')
    require(os.environ.get('DEVELOPER_DIR') == DEVELOPER, 'xcode_selection_mismatch')
    work = prepare_root(os.environ, ROOT)
    env = clean_environment(work)
    stage = 'toolchain'
    summary = {'schemaVersion': 1, 'oracle_capture': 'failed', 'unicode16_comparison': 'not_performed', 'stage': stage}
    def run(label, command, limit=262144, timeout=180, input_bytes=None):
        result = subprocess.run(command, cwd=work, env=env, input=input_bytes,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout)
        (work / 'logs' / (label + '.log')).write_bytes(result.stderr[-262144:])
        if result.returncode != 0:
            # 仅固定公开源／合成输入的有界失败尾段；不上传目录或全部编译日志。
            tail = result.stderr[-2048:].decode('utf-8', errors='replace')[-1024:]
            summary['failureLog'] = tail.replace(str(work), '$RUN_ROOT').replace(str(ROOT), '$REPOSITORY')
            print(label + ': ' + summary['failureLog'], file=sys.stderr)
            raise GuardError(label + '_failed')
        require(len(result.stdout) <= limit, label + '_output_over_limit')
        return result.stdout
    def git(label, *arguments, limit=262144):
        return run(label, ['/usr/bin/git', '-c', 'core.hooksPath=/dev/null', '-c', 'protocol.file.allow=never',
                          '--git-dir=' + str(work / 'upstream.git'), *arguments], limit=limit)
    try:
        require(Path(DEVELOPER).is_dir(), 'xcode_26_2_unavailable')
        xcode = run('xcode', ['/usr/bin/xcodebuild', '-version']).decode().strip()
        require(xcode.splitlines()[0] == 'Xcode 26.2', 'xcode_version_mismatch')
        swift = run('swift-version', ['/usr/bin/xcrun', '--no-cache', 'swiftc', '--version']).decode().strip()
        os_version = run('os-version', ['/usr/bin/sw_vers']).decode().strip()
        sdk_version = run('sdk-version', ['/usr/bin/xcrun', '--no-cache', '--sdk', 'macosx', '--show-sdk-version']).decode().strip()
        architecture = platform.machine()
        require(architecture in ('arm64', 'x86_64'), 'unsupported_architecture')
        stage = 'source_verification'
        git('git-init', 'init', '--bare')
        git('git-fetch', 'fetch', '--no-tags', '--depth=1', UPSTREAM, COMMIT)
        for ref, expected in [('FETCH_HEAD', COMMIT), (COMMIT + '^{tree}', manifest['rootTree']),
                              (COMMIT + ':Sources/NoonmarkCore', manifest['coreTree'])]:
            require(git('git-identity', 'rev-parse', ref).decode().strip() == expected, 'git_identity_mismatch')
        listing = git('git-core-tree', 'ls-tree', COMMIT + ':Sources/NoonmarkCore').decode().splitlines()
        expected_listing = sorted('100644 blob ' + x['gitBlob'] + '\t' + Path(x['path']).name for x in manifest['files'][2:])
        require(sorted(listing) == expected_listing, 'core_tree_files_mismatch')
        sources = []
        for entry in manifest['files']:
            data = git('git-blob', 'cat-file', 'blob', COMMIT + ':' + entry['path'], limit=4*1024*1024)
            require(len(data) == entry['bytes'] and sha(data) == entry['sha256'], 'upstream_file_hash_mismatch')
            target = work / 'source' / entry['path']
            target.parent.mkdir(parents=True, exist_ok=True)
            with target.open('xb') as handle:
                handle.write(data)
            target.chmod(0o444)
            if entry['path'].endswith('.swift') and entry['path'].startswith('Sources/'):
                sources.append(str(target))
        stage = 'compile'
        build = work / 'build'
        target = architecture + '-apple-macosx14.0'
        flags = ['-swift-version', '6', '-target', target, '-module-cache-path', str(work / 'cache' / 'swift')]
        run('compile-core', ['/usr/bin/xcrun', '--no-cache', 'swiftc', *flags, '-parse-as-library', '-module-name', 'NoonmarkCore',
            '-emit-library', '-emit-module', '-emit-module-path', str(build / 'NoonmarkCore.swiftmodule'),
            '-o', str(build / 'libNoonmarkCore.dylib'), *sources], timeout=900)
        harness_bytes = (HERE / 'NameOracle.swift').read_bytes()
        harness = work / 'source' / 'NameOracle.swift'
        harness.write_bytes(harness_bytes)
        run('compile-harness', ['/usr/bin/xcrun', '--no-cache', 'swiftc', *flags, '-I', str(build), '-L', str(build),
            '-lNoonmarkCore', '-Xlinker', '-rpath', '-Xlinker', str(build), str(harness), '-o', str(build / 'name-oracle')], timeout=300)
        stage = 'capture'
        # deny network 与其他路径写入；无 App/runtime/storage、路径输入或用户账户环境。
        sandbox = work / 'oracle.sb'
        escaped = str(work).replace('\\', '\\\\').replace('"', '\\"')
        sandbox.write_text('(version 1)\n(allow default)\n(deny network*)\n(deny file-write*)\n'
                           '(allow file-write* (subpath "' + escaped + '"))\n'
                           '(deny file-read* (subpath "/Volumes") (regex "^/Users/[^/]+/(Library|Documents|Desktop|Downloads|Pictures|Movies|Music)(/|$)"))\n'
                           '(allow file-read* (subpath "' + escaped + '"))\n')
        capture = json.loads(run('oracle', ['/usr/bin/sandbox-exec', '-f', str(sandbox), str(build / 'name-oracle')],
                                 limit=MAX_EVIDENCE, timeout=300, input_bytes=vectors_bytes))
        validate_capture(capture, vectors)
        stage = 'evidence'
        metadata = {
            'captureKind': 'actual-upstream-swift-public-api', 'source': manifest,
            'sourceManifestSHA256': sha(manifest_bytes), 'vectorsSHA256': sha(vectors_bytes),
            'harnessSHA256': sha(harness_bytes), 'runnerSHA256': sha(Path(__file__).read_bytes()),
            'workflowSHA256': WORKFLOW_SHA, 'workflowCommit': os.environ.get('GITHUB_SHA', ''),
            'runID': os.environ.get('GITHUB_RUN_ID', ''), 'runAttempt': os.environ.get('GITHUB_RUN_ATTEMPT', ''),
            'swiftVersion': swift, 'xcodeVersion': xcode, 'osVersion': os_version,
            'architecture': architecture, 'sdkVersion': sdk_version, 'deploymentTarget': target,
            'runnerImageOS': os.environ.get('ImageOS', ''), 'runnerImageVersion': os.environ.get('ImageVersion', ''),
            'swiftGraphemeUnicodeVersion': 'not_exposed_independently',
            'entityFixture': {'id': '00000000-0000-4000-8000-000000000001', 'unixSeconds': 1700000000,
                              'colorHex': '#336699', 'historyUUID': 'generated_by_upstream_not_recorded'},
            'compiledTargets': ['NoonmarkCore', 'NameOracle'],
            'productionDataAccess': False, 'iCloudAccess': False, 'keychainAccess': False, 'aiNetworkAccess': False,
        }
        capture['metadata'] = metadata
        capture['oracle_capture'] = 'succeeded'
        capture['unicode16_comparison'] = comparison_status(capture['icuUnicodeVersion'])
        write_json(work / 'evidence' / 'capture.json', capture, MAX_EVIDENCE)
        summary = {'schemaVersion': 1, 'oracle_capture': 'succeeded', 'unicode16_comparison': capture['unicode16_comparison'],
                   'algorithmVersion': capture['algorithmVersion'], 'icuVersion': capture['icuVersion'],
                   'icuUnicodeVersion': capture['icuUnicodeVersion'], 'caseCount': len(capture['cases']),
                   'pairCount': len(capture['pairs']), 'entityRoundTripCount': len(capture['entityRoundTrips']),
                   'captureSHA256': sha((work / 'evidence' / 'capture.json').read_bytes()), 'stage': 'complete'}
        print(json.dumps(summary, sort_keys=True))
    except (GuardError, subprocess.TimeoutExpired, OSError, ValueError, KeyError) as error:
        summary['stage'] = stage
        summary['failure'] = str(error) if isinstance(error, GuardError) else type(error).__name__
        raise
    finally:
        write_json(work / 'evidence' / 'summary.json', summary, 16384)

if __name__ == '__main__':
    try:
        main()
    except (GuardError, subprocess.TimeoutExpired, OSError, ValueError, KeyError) as error:
        print('Swift oracle failed: ' + (str(error) if isinstance(error, GuardError) else type(error).__name__), file=sys.stderr)
        sys.exit(1)
