# SPDX-License-Identifier: AGPL-3.0-only
"""Python-only protocol/failure tests. Synthetic fixtures are NOT Swift output/goldens.
Never executes capture(), Swift, compiler, product, hosted CI or network.
"""
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import stat
import tempfile
import unittest
from types import SimpleNamespace
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('ordinary_reference_runner', Path(__file__).with_name('runner.py'))
r = importlib.util.module_from_spec(SPEC); SPEC.loader.exec_module(r)
FROZEN = (Path(__file__).with_name('INPUT.json')).read_bytes()
ALGORITHM = 'unicode-16.0.0-nfkc-casefold-whitespace-v1'
I = '10000000-0000-0000-0000-000000000001'
P = '20000000-0000-0000-0000-000000000001'
R = '30000000-0000-0000-0000-000000000001'
OK = {'status': 'ok'}
ERROR = {'status': 'error', 'errorType': 'SyntheticProtocolTestOnly', 'message': 'synthetic controlled refusal, not Swift'}

def data(v):
    return json.dumps(v, sort_keys=True, separators=(',', ':')).encode()

def text(v):
    import base64
    return {'text': v, 'utf8Base64': base64.b64encode(v.encode()).decode()}

def projection():
    return {'revision': '0', 'categories': [], 'currentKeys': [], 'emptyCurrentKeys': [],
            'recordIDsInOriginalOrder': [], 'removalIDsInOriginalOrder': [], 'receiptInteractionIDs': [], 'dateBitsHex': {}}

def state():
    return dict.fromkeys(r.STATE_FIELDS - {'formatVersion', 'revision'}, []) | {'formatVersion': 1, 'revision': 0}

class SyntheticProtocol:
    def __init__(self):
        self.raw = {}; self.refs = {}; self.observations = {}
        self.add('input-original.json', FROZEN, 'frozen-original-input')
        for name in ['main'] + ['name-' + n['id'] for n in json.loads(FROZEN)['names']] + ['clock-negativeZero', 'clock-zero', 'clock-submillisecond']:
            self.add(name + '-graph.json', data({'chains': [{}, {}], 'definitions': [{}, {}]}), 'actual-synthetic-ordinary-graph')
        for case in r.expected_cases(FROZEN):
            if case.endswith('-direct'):
                v = self.direct(case)
            elif case in r.STATE_MUTATIONS:
                v = {'kind': 'single-field-state-mutation', 'controlRefs': ['main-none'], 'positiveControlReference': 'main-none',
                     'prepare': {'status': 'not-run', 'reason': 'state-only mutation'}, 'commit': {'status': 'not-run', 'reason': 'state-only mutation'},
                     'before': projection(), 'afterStages': self.after(case)}
            else:
                v = self.command(case)
            v.update(schema='ordinary-category-observation-v1', caseID=case)
            self.observations[case] = v

    def add(self, name, raw, role):
        self.raw[name] = raw
        ref = {'path': name, 'bytes': len(raw), 'sha256': r.digest(raw), 'role': role}
        self.refs[name] = ref
        return copy.deepcopy(ref)

    def after(self, case):
        return {'state': projection(), 'integrity': OK.copy(),
                'encode': {'status': 'ok', 'dateStrategy': 'deferredToDate', 'originalBytes': self.add(case + '-state.json', data(state()), 'actual-Foundation-state-Codable-bytes')},
                'decode': {'status': 'ok', 'stateEqual': True, 'allDateBitsEqual': True, 'decodedDateBitsHex': {}}}

    def direct(self, case):
        category = {'id': {'rawValue': P}, 'name': 'synthetic', 'canonicalKey': 'synthetic', 'canonicalKeyVersion': ALGORITHM,
                    'colorHex': '#2A6FDB', 'presentationApproval': 'userApproved', 'lifecycle': 'active', 'nameVersions': [], 'createdAt': 0, 'updatedAt': 0}
        return {'kind': 'normalization-reference', 'entry': 'direct-initializer', 'raw': text('synthetic'),
                'DAndKStages': [{'stage': 'D' + str(i), 'name': text('synthetic'), 'KOfThisStage': text('synthetic')} for i in range(6)],
                'name': text('synthetic'), 'canonicalKey': text('synthetic'), 'originalCategoryID': P, 'originalNameVersionIDs': [],
                'nowBitsHex': '0000000000000000',
                'encode': {'status': 'ok', 'originalBytes': self.add(case + '-category.json', data(category), 'actual-category-Codable-bytes')},
                'decode': {'status': 'ok', 'name': text('synthetic'), 'createdAtBitsHex': '0000000000000000'}}

    def command(self, case):
        v = {'kind': 'command', 'controlRefs': r.CONTROL_REFS.get(case, []), 'requiredPositiveControl': case in r.POSITIVE_CONTROLS,
             'request': {'intent': {'kind': 'setCurrent', 'chainID': I, 'category': {'kind': 'none'}, 'labels': []}, 'interactionID': I, 'prepareNowBitsHex': '0000000000000000'}, 'logicalAndDateBitsUnchanged': True, 'afterStages': self.after(case)}
        if case.startswith('name-'):
            return v | {'prepare': ERROR.copy(), 'commit': {'status': 'not-run', 'reason': 'synthetic prepare refusal'}}
        retained = {'main-retry-original', 'main-retry-changed-now', 'retry-changed-decision', 'preview-tamper-old-digest', 'preview-tamper-rehashed', 'digest-tamper'}
        v.update(prepare={'status': 'not-run', 'reason': 'retained or mutated plan'} if case in retained else OK.copy(),
                 nowBitsHex='0000000000000000', before=projection(),
                 originalBeforeBytes=self.add(case + '-before.json', data(state()), 'actual-before-state-bytes'))
        material = {'formatVersion': 1, 'planID': P, 'baseRevision': 0, 'interactionID': I, 'source': {'userDirect': {}},
                    'changes': [{'setCurrent': {'chainID': {'rawValue': I}, 'before': {'labels': []}, 'after': {'labels': []}}}],
                    'plannedLabelIDs': [], 'notices': [], 'blockers': []}
        raw = data(material); phash = r.digest(raw); actual = '0' * 64 if case in r.TAMPERS else phash
        v['plan'] = {'planID': P, 'interactionID': I, 'preparedIntent': {'kind': 'setCurrent', 'chainID': I, 'category': {'kind': 'none'}, 'labels': []}, 'baseRevision': '0',
                     'digest': actual, 'reconstructedSHA256': phash, 'matchesActualDigest': actual == phash,
                     'originalHasValidDigest': actual == phash, 'intentionallyCorrupted': case in r.TAMPERS,
                     'reconstructedPayloadBytes': self.add(case + '-plan-payload.json', raw, 'reconstructed-plan-payload-not-intercepted')}
        v['confirmation'] = {'authority': 'bound-user', 'decisionID': I, 'planID': P, 'interactionID': I, 'digest': actual, 'originalAuthorizes': True}
        if case not in r.POSITIVE_CONTROLS:
            v['commit'] = ERROR.copy(); return v
        v['commit'] = OK.copy()
        record = {'formatVersion': 1, 'id': R, 'planID': P, 'interactionID': I, 'source': material['source'], 'decisionID': I,
                  'changes': material['changes'], 'notices': [], 'committedAtBitPattern': 0, 'revision': 0, 'planDigest': actual}
        rhash = r.digest(data(record))
        original = {k: val for k, val in record.items() if k not in ('formatVersion', 'committedAtBitPattern')} | {'committedAt': 0, 'integrityDigest': rhash}
        receipt = {'planID': P, 'revision': 0, 'notices': [], 'changeRecordID': R, 'decisionID': I, 'changeRecordIntegrityDigest': rhash}
        v['receiptBytes'] = self.add(case + '-receipt.json', data(receipt), 'actual-Foundation-receipt-bytes')
        v['record'] = {'recordID': R, 'integrityDigest': rhash, 'committedAtBitsHex': '0000000000000000', 'matchesActualDigest': True,
                       'reconstructedPayloadBytes': self.add(case + '-record-payload.json', data(record), 'reconstructed-record-payload-not-intercepted'),
                       'originalRecordBytes': self.add(case + '-record.json', data(original), 'actual-Foundation-record-Codable-bytes')}
        v['exactReceiptAlreadyPresent'] = False
        return v

    def save(self, root, input_override=None, ids=None):
        if input_override is not None:
            self.add('input-original.json', input_override, 'frozen-original-input')
        for case, value in self.observations.items():
            self.add(case + '.json', data(value), 'observation')
        for name, raw in self.raw.items():
            (root / name).write_bytes(raw)
        manifest = {'schema': 'ordinary-category-artifacts-v1', 'suite': 'ordinary-category-only-v1', 'upstreamCommit': r.COMMIT,
                    'inputSHA256': r.INPUT_SHA, 'captureComplete': True, 'semanticParityVerified': False, 'runtimeAdmission': False,
                    'fullSnapshotExecuted': False, 'causalIDNormalizationApplied': False, 'algorithmVersion': ALGORITHM,
                    'observationIDs': ids if ids is not None else r.expected_cases(FROZEN), 'artifactBytes': sum(map(len, self.raw.values())), 'files': list(self.refs.values())}
        (root / 'manifest.json').write_bytes(data(manifest))

class CaptureContracts(unittest.TestCase):
    def verify(self, fixture, **kwargs):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve(); fixture.save(root, **kwargs)
            return r.verify_capture(root, FROZEN)

    def rejects(self, mutate, pattern=None, **kwargs):
        fixture = SyntheticProtocol(); mutate(fixture)
        with self.assertRaisesRegex((ValueError, OSError), pattern or '.'):
            self.verify(fixture, **kwargs)

    def test_complete_synthetic_protocol_is_not_source_bound_execution(self):
        result = self.verify(SyntheticProtocol())
        self.assertTrue(result['captureComplete']); self.assertFalse(result['sourceBoundExecution'])
        self.assertFalse(result['semanticParityVerified']); self.assertEqual(result['observationCount'], 65)
        self.assertRegex(result['outputManifestSHA256'], '^[0-9a-f]{64}$')

    def test_empty65_observations_rejected(self):
        self.rejects(lambda f: f.observations.update({c: {'schema': 'ordinary-category-observation-v1', 'caseID': c} for c in f.observations}))

    def test_substituted_input_with65_rejected(self):
        self.rejects(lambda f: None, 'actual archived input', input_override=b'{"names":[]}')

    def test_substituted_input_and_reduced25_rejected(self):
        self.rejects(lambda f: None, 'observation', input_override=b'{"names":[]}', ids=r.expected_cases(FROZEN)[:22] + r.expected_cases(FROZEN)[-3:])

    def test_oversized_observation_rejected(self):
        self.rejects(lambda f: f.observations['main-new'].update(padding='x' * 66000), 'oversized')

    def test_missing_success_receipt_rejected(self):
        self.rejects(lambda f: f.observations['main-new'].pop('receiptBytes'), 'receipt')

    def test_missing_success_record_rejected(self):
        self.rejects(lambda f: f.observations['main-new'].pop('record'), 'receipt/record')

    def test_missing_encode_bytes_rejected(self):
        self.rejects(lambda f: f.observations['main-new']['afterStages']['encode'].pop('originalBytes'))

    def test_missing_stage_error_details_rejected(self):
        self.rejects(lambda f: f.observations['relation-clock-backward'].update(commit={'status': 'error'}))

    def test_missing_not_run_reason_rejected(self):
        self.rejects(lambda f: f.observations['main-retry-original'].update(prepare={'status': 'not-run'}))

    def test_missing_normalization_DK_material_rejected(self):
        self.rejects(lambda f: f.observations['name-chinese-direct']['DAndKStages'][0].pop('KOfThisStage'))

    def test_missing_prepared_intent_material_rejected(self):
        self.rejects(lambda f: f.observations['main-new']['plan'].update(preparedIntent={}))

    def test_wrong_reference_metadata_rejected(self):
        self.rejects(lambda f: f.observations['main-new']['receiptBytes'].update(role='observation'), 'reference')

    def test_wrong_digest_report_rejected(self):
        self.rejects(lambda f: f.observations['main-new']['plan'].update(matchesActualDigest=False), 'digest')

    def test_wrong_intentional_tamper_label_rejected(self):
        self.rejects(lambda f: f.observations['digest-tamper']['plan'].update(intentionallyCorrupted=False), 'digest')

    def test_positive_control_integrity_failure_rejected(self):
        self.rejects(lambda f: f.observations['same-base-first-positive']['afterStages'].update(integrity=ERROR.copy()), 'positive control')

    def test_positive_control_commit_failure_rejected(self):
        def mutate(f):
            v = f.observations['tamper-positive-control']; v['commit'] = ERROR.copy()
            for k in ('record', 'receiptBytes', 'exactReceiptAlreadyPresent'): v.pop(k)
        self.rejects(mutate, 'positive control')

    def test_missing_control_reference_rejected(self):
        self.rejects(lambda f: f.observations['same-base-stale-second'].update(controlRefs=[]), 'control')

    def test_failed_negative_integrity_is_collected(self):
        f = SyntheticProtocol(); f.observations['missing-removal']['afterStages']['integrity'] = ERROR.copy()
        self.assertTrue(self.verify(f)['captureComplete'])

    def test_missing_algorithm_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve(); SyntheticProtocol().save(root)
            m = json.loads((root / 'manifest.json').read_bytes()); m.pop('algorithmVersion')
            (root / 'manifest.json').write_bytes(data(m))
            with self.assertRaises(ValueError): r.verify_capture(root, FROZEN)

    def test_output_identity_change_during_verification_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve(); SyntheticProtocol().save(root)
            original_checked = r.checked; changed = False
            def changing(path, cap=r.MAX_RAW):
                nonlocal changed
                raw = original_checked(path, cap)
                if path.name == 'main-new.json' and not changed:
                    changed = True
                    path.write_bytes(raw + b' ')
                return raw
            with patch.object(r, 'checked', side_effect=changing):
                with self.assertRaisesRegex(ValueError, 'output identity changed'):
                    r.verify_capture(root, FROZEN)

    def test_hardlinked_output_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve(); SyntheticProtocol().save(root)
            os.link(root / 'main-new.json', root / 'hardlink')
            with self.assertRaisesRegex(ValueError, 'linked'): r.verify_capture(root, FROZEN)

class ContainmentContracts(unittest.TestCase):
    def test_symlink_leaf_parent_and_hardlink_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve(); (root / 'real').mkdir(); (root / 'real' / 'f').write_bytes(b'x')
            (root / 'parent').symlink_to(root / 'real', target_is_directory=True)
            (root / 'leaf').symlink_to(root / 'real' / 'f')
            for path in (root / 'parent' / 'f', root / 'leaf'):
                with self.assertRaises(OSError): r.checked(path)
            os.link(root / 'real' / 'f', root / 'hard')
            with self.assertRaisesRegex(ValueError, 'linked'): r.checked(root / 'hard')

    def test_staged_identity_detects_content_and_inode_change(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve() / 'frozen'; root.mkdir(); (root / 'f').write_bytes(b'a'); (root / 'f').chmod(0o444); root.chmod(0o555)
            before = r.frozen_identity(root)
            (root / 'f').chmod(0o644); (root / 'f').write_bytes(b'b'); (root / 'f').chmod(0o444)
            self.assertNotEqual(r.frozen_identity(root), before)
            root.chmod(0o755); (root / 'f').unlink(); (root / 'f').write_bytes(b'a'); (root / 'f').chmod(0o444); root.chmod(0o555)
            self.assertNotEqual(r.frozen_identity(root), before)
            root.chmod(0o755)

    def test_full_staged_closure_is_independent_and_finally_rechecked(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve(); live = root / 'live'; work = root / 'work'; work.mkdir(mode=0o700)
            shutil.copytree(r.HERE, live)
            reviewed = r.digest((live / 'TOOL-MANIFEST.json').read_bytes())
            staged = None
            try:
                with patch.object(r, 'HERE', live):
                    staged, identity = r.stage_inputs(work, reviewed)
                    self.assertEqual(len([p for p in staged.rglob('*') if p.is_file()]), 15 + len(json.loads((live / 'TOOL-MANIFEST.json').read_bytes())['files']) + 1)
                    original = (staged / 'OrdinaryCategoryOracle.swift').read_bytes()
                    (live / 'OrdinaryCategoryOracle.swift').write_bytes(b'unreviewed live alteration')
                    self.assertEqual((staged / 'OrdinaryCategoryOracle.swift').read_bytes(), original)
                    r.assert_frozen(staged, identity, reviewed)
                    target = staged / 'OrdinaryCategoryOracle.swift'; target.chmod(0o644); target.write_bytes(b'changed'); target.chmod(0o444)
                    with self.assertRaisesRegex(ValueError, 'identity changed'):
                        r.assert_frozen(staged, identity, reviewed)
            finally:
                if staged:
                    for directory in [staged, *[p for p in staged.rglob('*') if p.is_dir()]]:
                        directory.chmod(0o755)

    def test_sandbox_does_not_read_live_candidate(self):
        policy = r.sandbox(Path('/private/tmp/isolated'), Path('/Applications/Xcode.app/Contents/Developer'), True)
        self.assertNotIn(str(r.HERE), policy); self.assertIn('(deny network*)', policy)

class Stream:
    def fileno(self): return 123
    def close(self): pass

class Process:
    def __init__(self, status): self.pid = 43210; self.returncode = status; self.stdout = Stream(); self.stderr = Stream(); self.reaps = 0
    def wait(self, timeout=None): self.reaps += 1; return self.returncode

class Selector:
    def register(self, *args): pass
    def get_map(self): return {}
    def close(self): pass

class ProcessContracts(unittest.TestCase):
    def probe(self, status=0, selector=None, set_blocking=None, cleanup_uncertain=False, read=None):
        p = Process(status); kills = []
        def killpg(pid, sig):
            kills.append((pid, sig))
            if sig == 0:
                if cleanup_uncertain: raise PermissionError('synthetic uncertainty')
                raise ProcessLookupError()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve(); ledger = []
            with patch.object(r.subprocess, 'Popen', return_value=p), patch.object(r.selectors, 'DefaultSelector', side_effect=selector or (lambda: Selector())), patch.object(r.os, 'set_blocking', side_effect=set_blocking or (lambda *args: None)), patch.object(r.os, 'killpg', side_effect=killpg), patch.object(r.os, 'read', side_effect=read or (lambda *args: b'')):
                try:
                    r.bounded_process(['MOCK-ONLY-NO-PROCESS'], root, {}, 1, 'mock', ledger)
                    success = True
                except ValueError:
                    success = False
            self.assertIn((p.pid, r.signal.SIGKILL), kills); self.assertGreaterEqual(p.reaps, 1)
            return success, ledger[0]

    def test_nonzero_exit_cleans_group(self):
        success, row = self.probe(status=1); self.assertFalse(success); self.assertTrue(row['cleanupComplete']); self.assertTrue(row['transportComplete'])

    def test_success_cleans_residual_group(self):
        success, row = self.probe(); self.assertTrue(success); self.assertTrue(row['cleanupComplete'])

    def test_selector_creation_failure_cleans_group(self):
        success, row = self.probe(selector=lambda: (_ for _ in ()).throw(RuntimeError('setup failure')))
        self.assertFalse(success); self.assertFalse(row['transportComplete'])

    def test_selector_registration_failure_cleans_group(self):
        class BadSelector(Selector):
            def register(self, *args): raise RuntimeError('register failure')
        success, row = self.probe(selector=lambda: BadSelector()); self.assertFalse(success); self.assertFalse(row['transportComplete'])

    def test_set_blocking_failure_cleans_group(self):
        success, row = self.probe(set_blocking=lambda *args: (_ for _ in ()).throw(RuntimeError('fd setup failure')))
        self.assertFalse(success)

    def test_timeout_failure_cleans_group(self):
        class TimeoutSelector(Selector):
            def get_map(self): return {1: 1}
            def select(self, *args): raise TimeoutError('synthetic timeout')
        success, row = self.probe(selector=lambda: TimeoutSelector())
        self.assertFalse(success); self.assertFalse(row['transportComplete'])

    def test_output_overrun_cleans_group_and_keeps_diagnostic_prefix(self):
        class OutputSelector(Selector):
            def get_map(self): return {1: 1}
            def select(self, *args): return [(SimpleNamespace(fd=123, data='stdout'), None)]
        success, row = self.probe(selector=lambda: OutputSelector(), read=lambda *args: b'x' * 8192)
        self.assertFalse(success); self.assertFalse(row['transportComplete'])
        self.assertEqual(row['stdout']['bytes'], 65536)
        self.assertTrue(row['stdout']['path'].endswith('.diagnostic-prefix'))

    def test_cleanup_uncertainty_prevents_success(self):
        success, row = self.probe(cleanup_uncertain=True); self.assertFalse(success); self.assertFalse(row['cleanupComplete'])

if __name__ == '__main__':
    unittest.main()
