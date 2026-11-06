#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""入口／输出验证器的离线测试；这些 fixture 不是 Swift 执行证据。"""
from copy import deepcopy
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('oracle_runner', HERE / 'run.py')
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)

class ContractTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='oracle-contract-', dir=os.environ.get('RUNNER_TEMP'))
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / 'repo'; self.repo.mkdir()
        self.runner_temp = self.root / 'temp'; self.runner_temp.mkdir()
        self.vectors = {'schemaVersion': 1, 'provenance': 'fixed-synthetic-only',
                        'cases': [{'id': 'fixture-only', 'input': 'A'}],
                        'pairs': [{'id': 'same', 'left': 'fixture-only', 'right': 'fixture-only'}]}

    def assertRejected(self, function, *args):
        with self.assertRaises((runner.GuardError, UnicodeEncodeError, KeyError, TypeError)):
            function(*args)

    def test_pinned_inputs(self):
        for path, digest, limit in [
            (HERE / 'source-manifest.json', runner.MANIFEST_SHA, 16384),
            (HERE / 'vectors.json', runner.VECTORS_SHA, runner.MAX_INPUT),
            (runner.ROOT / '.github/workflows/swift-name-oracle.yml', runner.WORKFLOW_SHA, 8192)]:
            runner.read_checked(path, digest, limit)
        runner.validate_manifest(json.loads((HERE / 'source-manifest.json').read_bytes()))
        value = runner.validate_vectors(json.loads((HERE / 'vectors.json').read_bytes()))
        self.assertEqual(len(value['cases']), 108)
        self.assertEqual(len(value['pairs']), 17)
        self.assertEqual(next(x['input'] for x in value['cases'] if x['id'] == 'display-non-idempotent'), 'a\n\u0301b')
        self.assertEqual(len(next(x['input'] for x in value['cases'] if x['id'] == 'nonstarters-65536')), 131073)

    def test_workflow_safety_mutations_rejected(self):
        text = (runner.ROOT / '.github/workflows/swift-name-oracle.yml').read_text()
        changes = [('contents: read', 'contents: write'), ('macos-15', 'self-hosted'),
                   ('branches: [main]', 'branches: ["*"]'), ('  workflow_dispatch:', '  pull_request_target:'),
                   ('persist-credentials: false', 'persist-credentials: true'),
                   ('include-hidden-files: false', 'include-hidden-files: true'),
                   ('evidence/*.json', '**/*'), ('retention-days: 7', 'retention-days: 90'),
                   ('Xcode_26.2', 'Xcode_16.4'), ('timeout-minutes: 30', 'timeout-minutes: 360'),
                   ('d23441a48e516b6c34aea4fa41551a30e30af803', 'v6'),
                   ('b7c566a772e6b6bfb58ed0dc250532a479d7789f', 'v6'),
                   ("      - 'ports/mygo/scripts/swift-name-oracle/**'", "      - '**'")]
        path = self.root / 'workflow.yml'
        for old, new in changes:
            with self.subTest(mutation=old):
                self.assertIn(old, text)
                path.write_text(text.replace(old, new))
                self.assertRejected(runner.read_checked, path, runner.WORKFLOW_SHA, 8192)

    def test_source_manifest_rejects_substitution_and_paths(self):
        original = json.loads((HERE / 'source-manifest.json').read_text())
        for field, value in [('commit', 'a' * 40), ('repository', 'https://example.invalid/evil'),
                             ('coreTree', 'b' * 40), ('rootTree', 'c' * 40)]:
            data = deepcopy(original); data[field] = value
            self.assertRejected(runner.validate_manifest, data)
        for path in ('../evil.swift', '/tmp/evil.swift', 'Sources/NoonmarkStorage/Store.swift',
                     'Sources/NoonmarkCore/../evil.swift'):
            data = deepcopy(original); data['files'][2]['path'] = path
            self.assertRejected(runner.validate_manifest, data)
        data = deepcopy(original); data['files'].append(data['files'][2])
        self.assertRejected(runner.validate_manifest, data)
        data = deepcopy(original); data['files'][2]['bytes'] = 10**10
        self.assertRejected(runner.validate_manifest, data)

    def test_digest_symlink_and_size_guards(self):
        path = self.root / 'input'; path.write_bytes(b'fixed')
        runner.read_checked(path, runner.sha(b'fixed'), 5)
        self.assertRejected(runner.read_checked, path, runner.sha(b'evil'), 5)
        self.assertRejected(runner.read_checked, path, runner.sha(b'fixed'), 4)
        link = self.root / 'link'; link.symlink_to(path)
        self.assertRejected(runner.read_checked, link, runner.sha(b'fixed'), 5)

    def test_fresh_root_and_source_overlap(self):
        work = runner.prepare_root({'RUNNER_TEMP': str(self.runner_temp)}, self.repo)
        self.assertEqual(work.parent, self.runner_temp)
        self.assertEqual(set(x.name for x in work.iterdir()), {'evidence','logs','home','tmp','cache','build','source'})
        self.assertRejected(runner.prepare_root, {'RUNNER_TEMP': str(self.runner_temp)}, self.repo)
        for invalid in ({}, {'RUNNER_TEMP': 'relative'}, {'RUNNER_TEMP': str(self.repo)},
                        {'RUNNER_TEMP': str(self.root)}, {'RUNNER_TEMP': str(self.repo/'missing')}):
            with self.subTest(environment=invalid):
                with self.assertRaises((runner.GuardError, FileNotFoundError)):
                    runner.prepare_root(invalid, self.repo)

    def test_existing_or_dangling_work_symlink_rejected(self):
        work = self.runner_temp / 'noonmark-swift-name-oracle'
        work.symlink_to(self.root / 'missing')
        self.assertRejected(runner.prepare_root, {'RUNNER_TEMP': str(self.runner_temp)}, self.repo)
        self.assertFalse((self.root / 'missing').exists())

    def test_clean_environment_contains_no_credentials_or_user_home(self):
        work = runner.prepare_root({'RUNNER_TEMP': str(self.runner_temp)}, self.repo)
        with unittest.mock.patch.dict(os.environ, {'SECRET': 'not-forwarded', 'HOME': '/Users/production',
             'GIT_CONFIG_PARAMETERS': 'malicious', 'DYLD_INSERT_LIBRARIES': '/tmp/malicious'}):
            env = runner.clean_environment(work)
        self.assertEqual(env['HOME'], str(work/'home'))
        self.assertEqual(env['TMPDIR'], str(work/'tmp'))
        for forbidden in ('SECRET', 'GIT_CONFIG_PARAMETERS', 'DYLD_INSERT_LIBRARIES'):
            self.assertNotIn(forbidden, env)
        self.assertFalse(any('/Users/production' in value for value in env.values()))

    def test_vectors_guards(self):
        runner.validate_vectors(self.vectors)
        data = deepcopy(self.vectors); data['provenance'] = 'production'
        self.assertRejected(runner.validate_vectors, data)
        data = deepcopy(self.vectors); data['cases'] *= 129
        self.assertRejected(runner.validate_vectors, data)
        data = deepcopy(self.vectors); data['cases'] *= 2
        self.assertRejected(runner.validate_vectors, data)
        data = deepcopy(self.vectors); data['cases'][0]['input'] = 'A' * (1024*1024+1)
        self.assertRejected(runner.validate_vectors, data)
        data = deepcopy(self.vectors); data['cases'][0]['input'] = '\ud800'
        self.assertRejected(runner.validate_vectors, data)
        data = deepcopy(self.vectors); data['pairs'][0]['right'] = 'missing'
        self.assertRejected(runner.validate_vectors, data)
        data = deepcopy(self.vectors); data['pairs'] *= 2
        self.assertRejected(runner.validate_vectors, data)

    def fixture_capture_not_swift_execution(self):
        text = {'text':'A', 'utf8Hex':'41', 'unicodeScalars':[65], 'characterCount':1,
                'characters':[{'startScalar':0, 'endScalar':1, 'isWhitespace':False}]}
        case = {'id':'fixture-only', **{field:deepcopy(text) for field in runner.TEXT_FIELDS}}
        pair = {**self.vectors['pairs'][0], **{field:{'swiftEqual':True,'utf8Equal':True}
                                                for field in ('input','displayName','canonicalKey')}}
        return {'schemaVersion':1, 'algorithmVersion':'unicode-16.0.0.0-nfkc-casefold-whitespace-v1',
                'icuVersion':'76.1.0.0', 'icuUnicodeVersion':'16.0.0.0',
                'cases':[case], 'pairs':[pair], 'entityRoundTrips':[]}

    def test_output_schema_fixture_is_not_swift_verification(self):
        runner.validate_capture(self.fixture_capture_not_swift_execution(), self.vectors)

    def test_output_bytes_scalars_ranges_and_status_guards(self):
        for field, value in [('utf8Hex','42'), ('unicodeScalars',[66]), ('characterCount',2),
                             ('characters',[{'startScalar':1,'endScalar':2,'isWhitespace':False}])]:
            data = self.fixture_capture_not_swift_execution(); data['cases'][0]['input'][field] = value
            self.assertRejected(runner.validate_capture, data, self.vectors)
        for field, value in [('icuVersion','unknown'), ('algorithmVersion','unicode-15.0.0.0-nfkc-casefold-whitespace-v1')]:
            data = self.fixture_capture_not_swift_execution(); data[field] = value
            self.assertRejected(runner.validate_capture, data, self.vectors)
        data = self.fixture_capture_not_swift_execution(); data['cases'][0]['extra'] = 'unexpected'
        self.assertRejected(runner.validate_capture, data, self.vectors)
        data = self.fixture_capture_not_swift_execution(); data['pairs'][0]['input']['utf8Equal'] = False
        self.assertRejected(runner.validate_capture, data, self.vectors)
        data = self.fixture_capture_not_swift_execution(); data['pairs'][0]['input']['swiftEqual'] = 'true'
        self.assertRejected(runner.validate_capture, data, self.vectors)

    def test_comparison_never_claimed_by_capture_success(self):
        self.assertEqual(runner.comparison_status('16.0.0.0'), 'not_performed')
        for version in ('15.1.0.0','17.0.0.0','16.1.0.0','unknown'):
            self.assertEqual(runner.comparison_status(version), 'unsupported')

    def test_evidence_bounds_and_exclusive_create(self):
        output = self.root/'evidence.json'
        self.assertRejected(runner.write_json, output, {'payload':'A'*32}, 8)
        self.assertFalse(output.exists())
        runner.write_json(output, {'ok':True}, 32)
        with self.assertRaises(FileExistsError):
            runner.write_json(output, {'overwrite':True}, 64)

    def test_missing_xcode_writes_failure_evidence_without_fallback(self):
        environment = {'RUNNER_TEMP':str(self.runner_temp), 'GITHUB_ACTIONS':'true',
                       'RUNNER_ENVIRONMENT':'github-hosted', 'DEVELOPER_DIR':'/missing/xcode'}
        with mock.patch.dict(os.environ, environment, clear=True), \
             mock.patch.object(runner.platform, 'system', return_value='Darwin'), \
             mock.patch.object(runner, 'DEVELOPER', '/missing/xcode'), \
             mock.patch.object(sys, 'argv', ['run.py']), \
             mock.patch.object(runner.subprocess, 'run') as process:
            with self.assertRaisesRegex(runner.GuardError, 'xcode_26_2_unavailable'):
                runner.main()
            process.assert_not_called()
        evidence = self.runner_temp/'noonmark-swift-name-oracle'/'evidence'
        self.assertEqual(set(x.name for x in evidence.iterdir()), {'summary.json'})
        summary = json.loads((evidence/'summary.json').read_text())
        self.assertEqual(summary['oracle_capture'], 'failed')
        self.assertEqual(summary['failure'], 'xcode_26_2_unavailable')

    def test_wrong_xcode_version_never_falls_back(self):
        developer = self.root/'developer'; developer.mkdir()
        environment = {'RUNNER_TEMP':str(self.runner_temp), 'GITHUB_ACTIONS':'true',
                       'RUNNER_ENVIRONMENT':'github-hosted', 'DEVELOPER_DIR':str(developer)}
        outcome = subprocess.CompletedProcess(['xcodebuild'], 0, b'Xcode 16.4\nBuild version fixture\n', b'')
        with mock.patch.dict(os.environ, environment, clear=True), \
             mock.patch.object(runner.platform, 'system', return_value='Darwin'), \
             mock.patch.object(runner, 'DEVELOPER', str(developer)), \
             mock.patch.object(sys, 'argv', ['run.py']), \
             mock.patch.object(runner.subprocess, 'run', return_value=outcome) as process:
            with self.assertRaisesRegex(runner.GuardError, 'xcode_version_mismatch'):
                runner.main()
            self.assertEqual(process.call_count, 1)
        summary = json.loads((self.runner_temp/'noonmark-swift-name-oracle/evidence/summary.json').read_text())
        self.assertEqual(summary['oracle_capture'], 'failed')
        self.assertEqual(summary['failure'], 'xcode_version_mismatch')

    def test_self_hosted_and_linux_reject_before_artifacts(self):
        for system, kind, expected in [('Linux','github-hosted','macos_required'),
                                      ('Darwin','self-hosted','github_hosted_required')]:
            environment = {'RUNNER_TEMP':str(self.runner_temp), 'GITHUB_ACTIONS':'true',
                           'RUNNER_ENVIRONMENT':kind, 'DEVELOPER_DIR':runner.DEVELOPER}
            with self.subTest(system=system, kind=kind), \
                 mock.patch.dict(os.environ, environment, clear=True), \
                 mock.patch.object(runner.platform, 'system', return_value=system), \
                 mock.patch.object(sys, 'argv', ['run.py']), \
                 mock.patch.object(runner.subprocess, 'run') as process:
                with self.assertRaisesRegex(runner.GuardError, expected):
                    runner.main()
                process.assert_not_called()
                self.assertEqual(list(self.runner_temp.iterdir()), [])

    def exercise_compile_commands_with_synthetic_source(self, architecture='arm64', fail_core=False):
        # Deliberately synthetic Git/source responses. No Swift compiler runs,
        # no upstream source is replaced, and no successful capture is emitted.
        manifest = json.loads((HERE / 'source-manifest.json').read_text())
        blobs = {}
        for entry in manifest['files']:
            blob = ('SYNTHETIC-COMMAND-CONTRACT-ONLY:' + entry['path'] + '\n').encode()
            blobs[entry['path']] = blob
            entry['bytes'] = len(blob)
            entry['sha256'] = runner.sha(blob)
        work = self.runner_temp / 'noonmark-swift-name-oracle'
        developer = self.root / 'developer'; developer.mkdir()
        environment = {'RUNNER_TEMP': str(self.runner_temp), 'GITHUB_ACTIONS': 'true',
                       'RUNNER_ENVIRONMENT': 'github-hosted', 'DEVELOPER_DIR': str(developer),
                       'SWIFT_PACKAGE_NAME': 'untrusted-caller-package', 'SWIFTFLAGS': '-package-name evil'}
        commands = []

        def fake_process(command, **kwargs):
            commands.append(command)
            self.assertEqual(kwargs['cwd'], work)
            self.assertNotIn('SWIFT_PACKAGE_NAME', kwargs['env'])
            self.assertNotIn('SWIFTFLAGS', kwargs['env'])
            output = b''
            code = 0
            error = b''
            if command == ['/usr/bin/xcodebuild', '-version']:
                output = b'Xcode 26.2\nBuild version SYNTHETIC\n'
            elif command == ['/usr/bin/xcrun', '--no-cache', 'swiftc', '--version']:
                output = b'Apple Swift version SYNTHETIC-NOT-EXECUTED\n'
            elif command == ['/usr/bin/sw_vers']:
                output = b'ProductName: macOS SYNTHETIC-NOT-EXECUTED\n'
            elif command == ['/usr/bin/xcrun', '--no-cache', '--sdk', 'macosx', '--show-sdk-version']:
                output = b'26.2\n'
            elif command[0] == '/usr/bin/git':
                args = command[6:]
                if args[0] in ('init', 'fetch'):
                    pass
                elif args[0] == 'rev-parse':
                    output = { 'FETCH_HEAD': runner.COMMIT,
                               runner.COMMIT + '^{tree}': manifest['rootTree'],
                               runner.COMMIT + ':Sources/NoonmarkCore': manifest['coreTree']}[args[1]].encode()
                elif args[0] == 'ls-tree':
                    output = '\n'.join('100644 blob ' + x['gitBlob'] + '\t' + Path(x['path']).name
                                       for x in manifest['files'][2:]).encode()
                elif args[:2] == ['cat-file', 'blob']:
                    output = blobs[args[2].split(':', 1)[1]]
                else:
                    self.fail('unexpected synthetic Git command: ' + repr(command))
            elif command[:3] == ['/usr/bin/xcrun', '--no-cache', 'swiftc']:
                # Stop at core failure, or at harness failure after inspecting
                # both commands. Never manufacture Swift output or capture.json.
                if fail_core or '-module-name' not in command:
                    code = 17
                    error = b'SYNTHETIC compiler failure, not Swift execution\n'
            else:
                self.fail('unexpected process after compiler failure: ' + repr(command))
            return subprocess.CompletedProcess(command, code, output, error)

        failure = 'compile-core_failed' if fail_core else 'compile-harness_failed'
        with mock.patch.dict(os.environ, environment, clear=True), \
             mock.patch.object(runner.platform, 'system', return_value='Darwin'), \
             mock.patch.object(runner.platform, 'machine', return_value=architecture), \
             mock.patch.object(runner, 'DEVELOPER', str(developer)), \
             mock.patch.object(runner, 'validate_manifest', return_value=manifest), \
             mock.patch.object(sys, 'argv', ['run.py']), \
             mock.patch.object(runner.subprocess, 'run', side_effect=fake_process):
            with self.assertRaisesRegex(runner.GuardError, failure):
                runner.main()
        evidence = work / 'evidence'
        self.assertEqual([p.name for p in evidence.iterdir()], ['summary.json'])
        summary = json.loads((evidence / 'summary.json').read_text())
        self.assertEqual(summary['stage'], 'compile')
        self.assertEqual(summary['oracle_capture'], 'failed')
        self.assertEqual(summary['unicode16_comparison'], 'not_performed')
        self.assertEqual(summary['failure'], failure)
        for name, blob in blobs.items():
            source = work / 'source' / name
            self.assertEqual(source.read_bytes(), blob)
            self.assertEqual(source.stat().st_mode & 0o222, 0)
        return [c for c in commands if c[:3] == ['/usr/bin/xcrun', '--no-cache', 'swiftc'] and '--version' not in c]

    def test_main_core_compiler_has_package_context_harness_stays_external(self):
        commands = self.exercise_compile_commands_with_synthetic_source()
        self.assertEqual(len(commands), 2)
        core, harness = commands
        self.assertEqual(core.count('-package-name'), 1)
        self.assertEqual(core[core.index('-package-name') + 1], 'Noonmark')
        self.assertEqual(core[core.index('-module-name') + 1], 'NoonmarkCore')
        self.assertIn('-parse-as-library', core)
        self.assertIn('-emit-library', core)
        self.assertIn('-emit-module', core)
        work = self.runner_temp / 'noonmark-swift-name-oracle'
        manifest = json.loads((HERE / 'source-manifest.json').read_text())
        self.assertEqual([x for x in core if x.endswith('.swift')],
                         [str(work / 'source' / x['path']) for x in manifest['files'][2:]])
        self.assertNotIn('-package-name', harness)
        self.assertNotIn('-enable-testing', harness)
        self.assertIn('-lNoonmarkCore', harness)
        for command in commands:
            self.assertEqual(command[command.index('-swift-version') + 1], '6')
            self.assertEqual(command[command.index('-target') + 1], 'arm64-apple-macosx14.0')
            self.assertEqual(command[command.index('-module-cache-path') + 1], str(work / 'cache' / 'swift'))

    def test_main_core_compile_failure_stops_before_harness_and_capture(self):
        commands = self.exercise_compile_commands_with_synthetic_source(architecture='x86_64', fail_core=True)
        self.assertEqual(len(commands), 1)
        self.assertEqual(commands[0][commands[0].index('-target') + 1], 'x86_64-apple-macosx14.0')

    def test_failure_gate_mapping_is_enforced_by_capture_workflow(self):
        registry = runner.ROOT / 'ports/mygo/docs/engineering/failure-cases/swift-oracle-gates.tsv'
        lines = registry.read_text().splitlines()
        self.assertEqual(lines[0], 'case_id\tfast_gate\tsymptom_gate\taggregate\tstatus')
        self.assertEqual(len(lines), 2)
        case, fast, symptom, aggregate, status = lines[1].split('\t')
        self.assertEqual(case, 'GO-SWIFT-ORACLE-001')
        self.assertTrue((registry.parent / (case + '.md')).is_file())
        self.assertEqual(fast, 'TestSwiftNameOracleCompilationContract')
        main_registry = (registry.parent / 'gates.tsv').read_text().splitlines()
        registered = [line.split('\t') for line in main_registry[1:] if line.split('\t')[0] == case]
        self.assertEqual(registered, [[case, fast, fast, 'scripts/check',
                                     'in_progress_offline_contract_only_real_macos_symptom_in_swift_oracle_gates']])
        self.assertEqual(symptom, 'python3 -B ports/mygo/scripts/swift-name-oracle/run.py')
        self.assertEqual(aggregate, 'scripts/check;.github/workflows/swift-name-oracle.yml#jobs.capture')
        self.assertIn(status, ('in_progress_macos_ci_pending', 'fixed_hosted_run_verified'))
        workflow_path = aggregate.split(';')[1].split('#')[0]
        workflow = runner.read_checked(runner.ROOT / workflow_path, runner.WORKFLOW_SHA, 8192).decode()
        fast_step = '      - name: Test isolated capture contract\n        run: python3 -B ports/mygo/scripts/swift-name-oracle/test_oracle.py'
        symptom_step = '      - name: Capture actual upstream Swift behavior\n        run: ' + symptom
        self.assertIn(fast_step, workflow)
        self.assertIn(symptom_step, workflow)
        self.assertLess(workflow.index(fast_step), workflow.index(symptom_step))

    def test_cli_does_not_accept_external_inputs(self):
        outcome = subprocess.run([sys.executable, '-B', str(HERE/'run.py'), '/Users/production'],
                                 capture_output=True, text=True)
        self.assertNotEqual(outcome.returncode, 0)
        self.assertIn('no_runtime_arguments', outcome.stderr)

if __name__ == '__main__':
    unittest.main(verbosity=2)
