#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""All captures in these tests are conspicuously SYNTHETIC; no Swift is run."""
import contextlib
import copy
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import compare as c

BUILD = os.environ.get('NOONMARK_SWIFT_COMPARE_TEST_ROOT', '')
GO = os.environ.get('GO', '')
IDENTITY = {'workflowCommit': '1' * 40, 'runID': '100000000001', 'runAttempt': '1'}


def synthetic_text(value):
    result = dict(value)
    n = len(result['text'])
    # Deliberately NOT grapheme segmentation. The comparator must not certify it.
    result['characterCount'] = int(n > 0)
    result['characters'] = [{'startScalar': 0, 'endScalar': n, 'isWhitespace': False}] if n else []
    return result


def synthetic_capture(inputs, probe):
    pins = inputs['pins']
    meta = {'captureKind': c.SYNTHETIC, 'source': inputs['manifest'], **IDENTITY,
            'sourceManifestSHA256': pins['ports/mygo/scripts/swift-name-oracle/source-manifest.json'],
            'vectorsSHA256': pins['ports/mygo/scripts/swift-name-oracle/vectors.json'],
            'harnessSHA256': pins['ports/mygo/scripts/swift-name-oracle/NameOracle.swift'],
            'runnerSHA256': pins['ports/mygo/scripts/swift-name-oracle/run.py'],
            'workflowSHA256': pins['.github/workflows/swift-name-oracle.yml'],
            'swiftVersion': 'Apple Swift version 6.2.3 (SYNTHETIC TEST FIXTURE, NOT EXECUTED)',
            'xcodeVersion': 'Xcode 26.2\nBuild version SYNTHETIC',
            'osVersion': 'ProductName:\t\tmacOS\nProductVersion:\t\t15.0\nBuildVersion:\t\tSYNTHETIC',
            'architecture': 'arm64', 'sdkVersion': '26.2', 'deploymentTarget': 'arm64-apple-macosx14.0',
            'runnerImageOS': 'macos15', 'runnerImageVersion': '20000101.1.1',
            'swiftGraphemeUnicodeVersion': 'not_exposed_independently',
            'entityFixture': {'id': '00000000-0000-4000-8000-000000000001', 'unixSeconds': 1700000000,
                              'colorHex': '#336699', 'historyUUID': 'generated_by_upstream_not_recorded'},
            'compiledTargets': ['NoonmarkCore', 'NameOracle'],
            'productionDataAccess': False, 'iCloudAccess': False, 'keychainAccess': False, 'aiNetworkAccess': False}
    cases = [{'id': row['id'], **{field: synthetic_text(row[field]) for field in c.FIELDS}} for row in probe['cases']]
    entities = []
    for row in probe['entityFields']:
        entities.append({**row, 'name': synthetic_text(row['name']), 'canonicalKey': synthetic_text(row['canonicalKey']),
                         'encodedByteCount': 100, 'decodeStatus': 'accepted', 'roundTripEqual': True,
                         'nameUTF8Equal': True, 'keyUTF8Equal': True})
    return {'schemaVersion': 1, 'algorithmVersion': c.VERSION, 'icuVersion': '76.1.0.0',
            'icuUnicodeVersion': '16.0.0.0', 'cases': cases, 'pairs': copy.deepcopy(probe['pairs']),
            'entityRoundTrips': entities, 'metadata': meta, 'oracle_capture': 'succeeded', 'unicode16_comparison': 'not_performed'}


def summary(capture, blob):
    return {'schemaVersion': 1, 'oracle_capture': 'succeeded', 'unicode16_comparison': capture['unicode16_comparison'],
            'algorithmVersion': capture['algorithmVersion'], 'icuVersion': capture['icuVersion'],
            'icuUnicodeVersion': capture['icuUnicodeVersion'], 'caseCount': 108, 'pairCount': 17,
            'entityRoundTripCount': 12, 'captureSHA256': c.sha(blob), 'stage': 'complete'}


class ComparisonTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if not BUILD or not GO:
            raise RuntimeError('Explicit external NOONMARK_SWIFT_COMPARE_TEST_ROOT and Go 1.27.3 GO are required')
        cls.inputs = c.load_inputs()
        cls.work, paths = c.prepare_run(BUILD, cls.inputs)
        cls.probe, cls.probe_hashes = c.build_probe(GO, cls.work, paths, cls.inputs)
        cls.capture = synthetic_capture(cls.inputs, cls.probe)
        cls.blob = c.encoded(cls.capture) + b'\n'
        if len(cls.blob) > c.MAX_CAPTURE:
            raise RuntimeError('synthetic fixture exceeds actual capture cap')
        (cls.work / 'SYNTHETIC-NOT-SWIFT.capture.json').write_bytes(cls.blob)
        (cls.work / 'SYNTHETIC-NOT-SWIFT.summary.json').write_bytes(c.encoded(summary(cls.capture, cls.blob)))
        report = c.compare(cls.capture, cls.probe)
        report['provenance'] = {'fixtureOrigin': 'Go probe self-comparison for comparator tests only, never real Swift evidence', 'captureSHA256': c.sha(cls.blob), 'sourceFilesSHA256': cls.inputs['pins'], **cls.probe_hashes}
        (cls.work / 'SYNTHETIC-NOT-SWIFT.comparison.json').write_bytes(c.encoded(report))
        print('Synthetic-only test evidence: ' + str(cls.work), flush=True)

    def validate(self, value):
        return c.validate_capture(value, self.inputs, IDENTITY, synthetic=True)

    def mutated(self, edit):
        capture = copy.deepcopy(self.capture)
        edit(capture)
        return capture

    def rejected(self, edit):
        with self.assertRaises(c.Rejected):
            self.validate(self.mutated(edit))

    def test_all_fixed_vectors_and_coverage(self):
        self.assertTrue(self.validate(self.capture))
        c.validate_summary(summary(self.capture, self.blob), self.capture, self.blob)
        result = c.compare(self.capture, self.probe)
        self.assertEqual(result['status'], 'synthetic_fixture_match')
        self.assertFalse(result['swiftVerified'])
        self.assertEqual(result['coverage']['totalChecks'], 1807)
        self.assertEqual(result['coverage']['mismatchCount'], 0)
        self.assertEqual(result['coverage']['textRecordCount'], 564)
        self.assertIn('swiftCharacterBoundaries', result['notCompared'])
        self.assertIn('entityDecodeAndWire', result['notCompared'])

    def test_production_validator_rejects_synthetic_fixture(self):
        with self.assertRaises(c.Rejected):
            c.validate_capture(self.capture, self.inputs, IDENTITY)

    def test_metadata_tampering(self):
        edits = {
            'source commit': lambda x: x['metadata']['source'].__setitem__('commit', '0' * 40),
            'source byte count bool': lambda x: x['metadata']['source']['files'][0].__setitem__('bytes', True),
            'manifest hash': lambda x: x['metadata'].__setitem__('sourceManifestSHA256', '0' * 64),
            'vectors hash': lambda x: x['metadata'].__setitem__('vectorsSHA256', '0' * 64),
            'harness hash': lambda x: x['metadata'].__setitem__('harnessSHA256', '0' * 64),
            'runner hash': lambda x: x['metadata'].__setitem__('runnerSHA256', '0' * 64),
            'workflow hash': lambda x: x['metadata'].__setitem__('workflowSHA256', '0' * 64),
            'workflow commit': lambda x: x['metadata'].__setitem__('workflowCommit', '0' * 40),
            'run id': lambda x: x['metadata'].__setitem__('runID', '2'),
            'run attempt': lambda x: x['metadata'].__setitem__('runAttempt', 1),
            'compiler absent': lambda x: x['metadata'].__setitem__('swiftVersion', ''),
            'xcode changed': lambda x: x['metadata'].__setitem__('xcodeVersion', 'Xcode 26.3\nBuild version 1'),
            'macos version': lambda x: x['metadata'].__setitem__('osVersion', 'Linux'),
            'architecture': lambda x: x['metadata'].__setitem__('architecture', 'amd64'),
            'deployment': lambda x: x['metadata'].__setitem__('deploymentTarget', 'arm64-apple-macosx15.0'),
            'runner': lambda x: x['metadata'].__setitem__('runnerImageOS', ''),
            'image version': lambda x: x['metadata'].__setitem__('runnerImageVersion', ''),
            'inferred grapheme': lambda x: x['metadata'].__setitem__('swiftGraphemeUnicodeVersion', '16.0.0'),
            'entity fixture': lambda x: x['metadata']['entityFixture'].__setitem__('unixSeconds', 0),
            'target': lambda x: x['metadata'].__setitem__('compiledTargets', ['NoonmarkApp']),
            'production': lambda x: x['metadata'].__setitem__('productionDataAccess', True),
            'false int': lambda x: x['metadata'].__setitem__('keychainAccess', 0),
            'missing': lambda x: x['metadata'].pop('sdkVersion'),
            'extra': lambda x: x['metadata'].__setitem__('unknown', True),
        }
        for name, edit in edits.items():
            with self.subTest(name=name):
                self.rejected(edit)

    def test_shape_counts_and_record_tampering(self):
        edits = {
            'schema bool': lambda x: x.__setitem__('schemaVersion', True),
            'schema future': lambda x: x.__setitem__('schemaVersion', 2),
            'extra root': lambda x: x.__setitem__('unexpected', 1),
            'missing case': lambda x: x['cases'].pop(),
            'duplicate case': lambda x: x['cases'].__setitem__(1, x['cases'][0]),
            'extra case': lambda x: x['cases'].append(x['cases'][0]),
            'case order': lambda x: x['cases'].reverse(),
            'extra case field': lambda x: x['cases'][1].__setitem__('unexpected', 1),
            'input changed': lambda x: x['cases'][1].__setitem__('input', synthetic_text({'text': 'X', 'utf8Hex': '58', 'unicodeScalars': [88]})),
            'hex changed': lambda x: x['cases'][1]['input'].__setitem__('utf8Hex', '00'),
            'scalars changed': lambda x: x['cases'][1]['input']['unicodeScalars'].__setitem__(0, 0),
            'scalar bool': lambda x: x['cases'][4]['input'].__setitem__('unicodeScalars', [True]),
            'surrogate': lambda x: x['cases'][1]['input'].__setitem__('text', '\ud800'),
            'character count bool': lambda x: x['cases'][1]['input'].__setitem__('characterCount', True),
            'character gap': lambda x: x['cases'][1]['input']['characters'][0].__setitem__('startScalar', 1),
            'character overflow': lambda x: x['cases'][1]['input']['characters'][0].__setitem__('endScalar', 999),
            'character empty': lambda x: x['cases'][1]['input']['characters'][0].__setitem__('endScalar', 0),
            'whitespace int': lambda x: x['cases'][1]['input']['characters'][0].__setitem__('isWhitespace', 0),
            'missing pair': lambda x: x['pairs'].pop(),
            'duplicate pair': lambda x: x['pairs'].__setitem__(1, x['pairs'][0]),
            'extra pair': lambda x: x['pairs'].append(x['pairs'][0]),
            'pair reference': lambda x: x['pairs'][0].__setitem__('left', 'unknown'),
            'pair bytes inconsistent': lambda x: x['pairs'][0]['input'].__setitem__('utf8Equal', not x['pairs'][0]['input']['utf8Equal']),
            'equality type': lambda x: x['pairs'][0]['input'].__setitem__('swiftEqual', 1),
            'missing entity': lambda x: x['entityRoundTrips'].pop(),
            'duplicate entity': lambda x: x['entityRoundTrips'].__setitem__(1, x['entityRoundTrips'][0]),
            'extra entity': lambda x: x['entityRoundTrips'].append(x['entityRoundTrips'][0]),
            'entity version': lambda x: x['entityRoundTrips'][0].__setitem__('canonicalKeyVersion', 'unknown'),
            'entity size': lambda x: x['entityRoundTrips'][0].__setitem__('encodedByteCount', 65536),
            'decode status': lambda x: x['entityRoundTrips'][0].__setitem__('decodeStatus', 'unknown'),
            'decode flag': lambda x: x['entityRoundTrips'][0].__setitem__('roundTripEqual', 1),
            'claim success': lambda x: x.__setitem__('unicode16_comparison', 'passed'),
            'capture failed': lambda x: x.__setitem__('oracle_capture', 'failed'),
        }
        for name, edit in edits.items():
            with self.subTest(name=name):
                self.rejected(edit)

    def test_rejected_entity_shape_and_uncompared_flags(self):
        value = copy.deepcopy(self.capture)
        row = value['entityRoundTrips'][0]
        for field in ('roundTripEqual', 'nameUTF8Equal', 'keyUTF8Equal'):
            row.pop(field)
        row.update(decodeStatus='rejected', decodeErrorKind='dataCorrupted', decodeErrorDescription='SYNTHETIC rejection')
        self.assertTrue(self.validate(value))
        self.assertEqual(c.compare(value, self.probe)['coverage']['mismatchCount'], 0)
        row['unexpected'] = True
        with self.assertRaises(c.Rejected):
            self.validate(value)
        row.pop('unexpected')
        row.pop('decodeErrorKind')
        with self.assertRaises(c.Rejected):
            self.validate(value)

    def test_version_gate(self):
        for version in ('15.1.0.0', '17.0.0.0', '255.0.0.0'):
            value = copy.deepcopy(self.capture)
            value.update(icuUnicodeVersion=version, algorithmVersion='unicode-' + version + '-nfkc-casefold-whitespace-v1', unicode16_comparison='unsupported')
            for row in value['entityRoundTrips']:
                row['canonicalKeyVersion'] = value['algorithmVersion']
            self.assertFalse(self.validate(value))
        self.rejected(lambda x: x.__setitem__('icuUnicodeVersion', '16.0'))
        self.rejected(lambda x: x.__setitem__('icuUnicodeVersion', '256.0.0.0'))
        self.rejected(lambda x: x.__setitem__('algorithmVersion', 'unicode-17.0.0.0-nfkc-casefold-whitespace-v1'))
        value = copy.deepcopy(self.capture)
        value['icuVersion'] = '99.0.0.0'
        self.assertTrue(self.validate(value))  # ICU library major is NOT the Unicode or Swift version.

    def test_detects_all_comparable_field_tampering(self):
        for field in c.FIELDS[1:]:
            value = copy.deepcopy(self.capture)
            value['cases'][1][field] = synthetic_text({'text': 'TAMPERED', 'utf8Hex': 'TAMPERED'.encode().hex(), 'unicodeScalars': list(map(ord, 'TAMPERED'))})
            result = c.compare(value, self.probe)
            self.assertEqual(result['coverage']['mismatchCount'], 3)
        value = copy.deepcopy(self.capture)
        value['pairs'][0]['input']['swiftEqual'] = not value['pairs'][0]['input']['swiftEqual']
        self.assertTrue(self.validate(value))
        self.assertEqual(c.compare(value, self.probe)['coverage']['mismatchCount'], 1)
        for field in ('name', 'canonicalKey'):
            value = copy.deepcopy(self.capture)
            value['entityRoundTrips'][0][field] = synthetic_text({'text': 'X', 'utf8Hex': '58', 'unicodeScalars': [88]})
            self.assertEqual(c.compare(value, self.probe)['coverage']['mismatchCount'], 3)

    def test_nonidempotent_display_and_empty_entity(self):
        rows = {row['id']: row for row in self.probe['cases']}
        row = rows['display-non-idempotent']
        self.assertEqual(row['displayName']['text'], 'a \u0301b')
        self.assertEqual(row['displayNameOfDisplayName']['text'], 'a b')
        self.assertNotEqual(row['canonicalKey']['text'], row['canonicalKeyOfDisplayName']['text'])
        entity = next(e for e in self.probe['entityFields'] if e['caseID'] == 'display-non-idempotent')
        self.assertEqual(entity['canonicalKey'], row['canonicalKeyOfDisplayName'])
        empty = next(e for e in self.probe['entityFields'] if e['caseID'] == 'empty')
        self.assertEqual(empty['name']['text'], '')
        self.assertEqual(empty['canonicalKey']['text'], '')
        only_di = next(e for e in self.probe['entityFields'] if e['caseID'] == 'only-di')
        self.assertEqual(only_di['canonicalKey']['text'], '')

    def test_strict_json_and_size(self):
        for raw in (b'{"x":1,"x":2}', b'{"x":NaN}', b'{"x":Infinity}', b'\xff', b'{} {}', b'[' * 10000):
            with self.assertRaises(c.Rejected):
                c.strict_json(raw, 'test')
        path = self.work / 'oversize.json'
        with path.open('wb') as out:
            out.truncate(c.MAX_CAPTURE + 1)
        with self.assertRaises(c.Rejected):
            c.read_file(path, c.MAX_CAPTURE)
        link = self.work / 'linked.json'
        link.symlink_to(self.work / 'SYNTHETIC-NOT-SWIFT.capture.json')
        with self.assertRaises(c.Rejected):
            c.read_file(link, c.MAX_CAPTURE)
        link.unlink()

    def test_build_uses_verified_snapshot_after_source_changes(self):
        run, paths = c.prepare_run(BUILD, self.inputs)
        # Only this synthetic fixture is changed, never the formal/candidate tree.
        fixture = run / 'SYNTHETIC-MUTABLE-SOURCE'
        package = fixture / 'classification/canonical16'
        package.mkdir(parents=True)
        (fixture / 'go.mod').write_text('module invalid.example/changed\n\ngo 1.27.3\n')
        (package / 'canonical.go').write_text('THIS IS DELIBERATELY INVALID SYNTHETIC SOURCE\n')
        (package / 'untracked.go').write_text('package canonical16\nimport "os"\nfunc init(){os.Stderr.WriteString("SYNTHETIC_UNPINNED_SENTINEL")}\n')
        (fixture / 'probe.go').write_text('THIS IS A CHANGED SYNTHETIC PROBE\n')
        with patch.object(c, 'MODULE', fixture), patch.object(c, 'HERE', fixture):
            probe, provenance = c.build_probe(GO, run, paths, self.inputs)
        self.assertEqual(probe, self.probe)
        self.assertNotIn(b'SYNTHETIC_UNPINNED_SENTINEL', (run / 'go-probe.stderr').read_bytes())
        manifest = provenance['buildSourceSnapshot']
        self.assertEqual(len(manifest['files']), 6)
        self.assertEqual(manifest['files']['cmd/name-probe/main.go']['sha256'], self.inputs['pins']['ports/mygo/scripts/swift-name-compare/probe.go'])
        self.assertNotIn('untracked.go', str(manifest))
        self.assertEqual(provenance['buildSourceSnapshotSHA256'], c.sha((run / 'source-snapshot-manifest.json').read_bytes()))
        for path in (run / 'source-snapshot').rglob('*'):
            self.assertEqual(path.stat().st_mode & 0o222, 0)
        result = c.compare(self.capture, probe)
        self.assertEqual(result['status'], 'synthetic_fixture_match')

    def test_go_scalar_types_for_every_text_field(self):
        for field in c.FIELDS:
            probe = copy.deepcopy(self.probe)
            row = next(row for row in probe['cases'] if row['id'] == 'ascii')
            row[field]['unicodeScalars'] = [float(x) for x in row[field]['unicodeScalars']]
            with self.subTest(case_field=field), self.assertRaises(c.Rejected):
                c.compare(self.capture, probe)
        for field in ('name', 'canonicalKey'):
            probe = copy.deepcopy(self.probe)
            row = next(row for row in probe['entityFields'] if row['caseID'] == 'ascii')
            row[field]['unicodeScalars'] = [float(x) for x in row[field]['unicodeScalars']]
            with self.subTest(entity_field=field), self.assertRaises(c.Rejected):
                c.compare(self.capture, probe)
        for field, value in [('algorithmVersion', True), ('cases', {}), ('pairs', {}), ('entityFields', {})]:
            probe = copy.deepcopy(self.probe)
            probe[field] = value
            with self.subTest(field=field), self.assertRaises(c.Rejected):
                c.compare(self.capture, probe)

    def test_snapshot_bytes_and_inventory_rejected_if_changed(self):
        run, _ = c.prepare_run(BUILD, self.inputs)
        snapshot, manifest, _ = c.freeze_build_sources(run, self.inputs)
        # Only an external synthetic snapshot is altered to exercise its verifier.
        path = snapshot / 'go.mod'
        path.chmod(0o600)
        path.write_bytes(path.read_bytes() + b'// SYNTHETIC MUTATION\n')
        path.chmod(0o400)
        with self.assertRaises(c.Rejected):
            c.verify_build_snapshot(snapshot, manifest)
        path.chmod(0o600)
        path.write_bytes(c.SNAPSHOT_MODULE)
        path.chmod(0o400)
        c.verify_build_snapshot(snapshot, manifest)
        # Verify inventory independently of the changed-byte failure above.
        snapshot.chmod(0o700)
        extra = snapshot / 'SYNTHETIC-extra.go'
        extra.write_text('package synthetic\n')
        extra.chmod(0o400)
        snapshot.chmod(0o500)
        with self.assertRaises(c.Rejected):
            c.verify_build_snapshot(snapshot, manifest)
        snapshot.chmod(0o700)
        extra.unlink()
        path.unlink()
        snapshot.chmod(0o500)
        with self.assertRaises(c.Rejected):
            c.verify_build_snapshot(snapshot, manifest)

    def test_pinned_sources_are_fail_closed(self):
        real_read = c.read_file
        for target in ('pins.json', 'vectors.json', 'NameOracle.swift', 'canonical.go', 'go.mod', 'guard_paths.py', 'probe.go'):
            def changed(path, limit, target=target):
                result = real_read(path, limit)
                return result + b' ' if Path(path).name == target else result
            with self.subTest(source=target), patch.object(c, 'read_file', side_effect=changed), self.assertRaises(c.Rejected):
                c.load_inputs()

    def test_unpinned_compile_inputs_rejected(self):
        fixed = ['canonical.go', 'normalize.go', 'grapheme.go', 'tables_generated.go']
        c.validate_package_inventory(fixed + ['ignored_test.go', 'README.md'])
        for extra in ('extra.go', 'extra_linux.go', 'extra.s', 'extra.S', 'extra.syso', 'extra.c'):
            with self.subTest(extra=extra), self.assertRaises(c.Rejected):
                c.validate_package_inventory(fixed + [extra])
        with self.assertRaises(c.Rejected):
            c.validate_package_inventory(fixed[:-1])

    def test_file_hardlink_fifo_and_output_symlink(self):
        original = self.work / 'regular-canary.txt'
        original.write_bytes(b'unchanged')
        link = self.work / 'hardlink-canary.txt'
        os.link(original, link)
        try:
            with self.assertRaises(c.Rejected):
                c.read_file(original, 100)
        finally:
            link.unlink()
        fifo = self.work / 'fifo-canary'
        os.mkfifo(fifo)
        try:
            with self.assertRaises(c.Rejected):
                c.read_file(fifo, 100)
        finally:
            fifo.unlink()
        link.symlink_to(c.ROOT, target_is_directory=True)
        try:
            with self.assertRaises(ValueError):
                c.prepare_run(str(link / 'forbidden'))
            with self.assertRaises(ValueError):
                c.prepare_run(BUILD)
        finally:
            link.unlink()
        self.assertEqual(original.read_bytes(), b'unchanged')

    def test_summary_binding(self):
        original = summary(self.capture, self.blob)
        for field, value in [('captureSHA256', '0' * 64), ('caseCount', 107), ('schemaVersion', True), ('stage', 'capture'), ('unicode16_comparison', 'passed')]:
            changed = dict(original)
            changed[field] = value
            with self.assertRaises(c.Rejected):
                c.validate_summary(changed, self.capture, self.blob)

    def synthetic_path_layout(self):
        # Never infer fixture repositories from the developer's sibling checkout.
        layout = Path(tempfile.mkdtemp(prefix='SYNTHETIC-PATH-LAYOUT-', dir=self.work))
        root = layout / 'candidate'
        module = root / 'ports/mygo/domain-extensions'
        module.mkdir(parents=True)
        git_repo = layout / 'arbitrary-git-source'
        (git_repo / '.git').mkdir(parents=True)
        (git_repo / '.git/HEAD').write_text('ref: refs/heads/synthetic-only\n')
        port_repo = layout / 'arbitrary-port-source'
        (port_repo / 'ports/mygo').mkdir(parents=True)
        (port_repo / 'ports/mygo/AGENTS.md').write_text('SYNTHETIC source marker only\n')
        return layout, root, module, git_repo, port_repo

    def test_bad_paths_no_write(self):
        layout, root, module, git_repo, port_repo = self.synthetic_path_layout()
        def inventory():
            return {str(p.relative_to(layout)): None if p.is_dir() else p.read_bytes()
                    for p in layout.rglob('*')}
        before = inventory()
        bad = (str(root / 'build'), str(git_repo / 'outside-mygo'),
               str(port_repo / 'outside-mygo'), str(layout), 'relative',
               str(layout / 'external') + '/x/../bad', str(layout / 'newline') + '\n')
        with patch.object(c, 'ROOT', root), patch.object(c, 'MODULE', module):
            for base in bad:
                with self.subTest(path=base):
                    # A rejection must precede even an attempted output creation.
                    with patch.object(Path, 'mkdir', side_effect=AssertionError('unexpected mkdir')) as mkdir, \
                         patch.object(c.tempfile, 'mkdtemp', side_effect=AssertionError('unexpected mkdtemp')) as mkdtemp:
                        with self.assertRaises(ValueError):
                            c.prepare_run(base, self.inputs)
                        mkdir.assert_not_called()
                        mkdtemp.assert_not_called()
                    self.assertEqual(inventory(), before)

    def test_unmarked_sibling_name_is_valid_external_output(self):
        layout, root, module, _, _ = self.synthetic_path_layout()
        # The name alone is not a repository marker; this was the old false red.
        base = layout / 'noonmark-go/outside-mygo'
        self.assertFalse(base.parent.exists())
        with patch.object(c, 'ROOT', root), patch.object(c, 'MODULE', module):
            first, paths = c.prepare_run(str(base), self.inputs)
            second, _ = c.prepare_run(str(base), self.inputs)
        self.assertNotEqual(first, second)
        self.assertTrue((first / 'tmp').is_dir())
        self.assertTrue((second / 'tmp').is_dir())
        self.assertEqual(first.parent, base / 'runs')
        self.assertEqual(set(paths.values()), {str(base / x) for x in ('runs', 'gopath', 'gocache')})

    def test_prepare_run_uses_verified_helper_without_source_reread(self):
        # Absorb the independent review's helper-binding check permanently.
        with patch.object(c, 'read_file', side_effect=AssertionError('unexpected source reread')) as read:
            run, _ = c.prepare_run(BUILD, self.inputs)
        read.assert_not_called()
        self.assertTrue((run / 'tmp').is_dir())

    def args(self, capture_name='SYNTHETIC-NOT-SWIFT.capture.json', summary_name='SYNTHETIC-NOT-SWIFT.summary.json', digest=None):
        return ['--capture', str(self.work / capture_name), '--summary', str(self.work / summary_name), '--capture-sha256', digest or c.sha(self.blob), '--workflow-commit', IDENTITY['workflowCommit'], '--run-id', IDENTITY['runID'], '--run-attempt', IDENTITY['runAttempt'], '--go', GO, '--build-root', BUILD]

    def test_cli_rejects_fixture_before_build(self):
        result = subprocess.run([os.sys.executable, '-B', str(c.HERE / 'compare.py'), *self.args()], capture_output=True)
        self.assertEqual(result.returncode, 2)
        self.assertEqual(json.loads(result.stderr)['status'], 'rejected')
        self.assertIn('captureKind', json.loads(result.stderr)['reason'])

    def test_cli_unsupported_never_builds(self):
        capture = copy.deepcopy(self.capture)
        capture.update(icuUnicodeVersion='17.0.0.0', algorithmVersion='unicode-17.0.0.0-nfkc-casefold-whitespace-v1', unicode16_comparison='unsupported')
        for entity in capture['entityRoundTrips']:
            entity['canonicalKeyVersion'] = capture['algorithmVersion']
        blob = c.encoded(capture)
        (self.work / 'SYNTHETIC-UNSUPPORTED.capture.json').write_bytes(blob)
        (self.work / 'SYNTHETIC-UNSUPPORTED.summary.json').write_bytes(c.encoded(summary(capture, blob)))
        with patch.object(c, 'ACTUAL', c.SYNTHETIC), patch.object(c, 'prepare_run', side_effect=AssertionError('must not build')), contextlib.redirect_stdout(io.StringIO()) as output:
            result = c.main(self.args('SYNTHETIC-UNSUPPORTED.capture.json', 'SYNTHETIC-UNSUPPORTED.summary.json', c.sha(blob)))
        self.assertEqual(result, 3)
        report = json.loads(output.getvalue())
        self.assertEqual(report['status'], 'unsupported')
        self.assertEqual(report['totalChecks'], 0)

    def test_cli_synthetic_mismatch_is_nonzero(self):
        capture = copy.deepcopy(self.capture)
        row = next(x for x in capture['cases'] if x['id'] == 'nonstarters-29')
        row['displayName'] = synthetic_text({'text': 'X', 'utf8Hex': '58', 'unicodeScalars': [88]})
        blob = c.encoded(capture)
        (self.work / 'SYNTHETIC-MISMATCH.capture.json').write_bytes(blob)
        (self.work / 'SYNTHETIC-MISMATCH.summary.json').write_bytes(c.encoded(summary(capture, blob)))
        with patch.object(c, 'ACTUAL', c.SYNTHETIC), contextlib.redirect_stdout(io.StringIO()) as output:
            code = c.main(self.args('SYNTHETIC-MISMATCH.capture.json', 'SYNTHETIC-MISMATCH.summary.json', c.sha(blob)))
        result = json.loads(output.getvalue())
        self.assertEqual(code, 1)
        self.assertEqual(result['status'], 'synthetic_fixture_mismatch')
        self.assertEqual(result['coverage']['mismatchCount'], 3)

    def test_two_complete_synthetic_replays_are_fresh(self):
        paths = []
        for _ in range(2):
            # Test-only injection: production CLI offers NO switch accepting synthetic captures.
            with patch.object(c, 'ACTUAL', c.SYNTHETIC), contextlib.redirect_stdout(io.StringIO()) as output:
                code = c.main(self.args())
            self.assertEqual(code, 0)
            result = json.loads(output.getvalue())
            self.assertEqual(result['status'], 'synthetic_fixture_match')
            report = Path(result['report'])
            self.assertEqual(c.sha(report.read_bytes()), result['reportSHA256'])
            paths.append(report)
        self.assertNotEqual(paths[0], paths[1])
        first = c.strict_json(paths[0].read_bytes(), 'report')
        second = c.strict_json(paths[1].read_bytes(), 'report')
        self.assertEqual(first['checks'], second['checks'])
        self.assertEqual(first['provenance'], second['provenance'])
        self.assertIn('captureAuthenticity', first['notCompared'])


if __name__ == '__main__':
    unittest.main(verbosity=2)
