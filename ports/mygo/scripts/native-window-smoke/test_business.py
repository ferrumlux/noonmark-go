"""Synthetic business evidence only. Never executes native app or Go code."""
import copy
import hashlib
import json
from pathlib import Path
import struct
import unittest
from unittest import mock
import tempfile
import test_run
import business as b
import test_collect


def bits(seconds):
    return struct.unpack('>Q', struct.pack('>d', seconds))[0]


def fixture_facts():
    ids = [f'{i:08X}-0000-4000-8000-000000000000' for i in range(1, 11)]
    chain, definition, trace, day, cat, nv, record, plan, interaction, decision = ids
    now, later = bits(1234.123456789), bits(1234.234567891)
    date = dict(Year=2026, Month=11, Day=6)
    common = dict(Name='Native smoke category v1', CanonicalKey='native smoke category v1', CanonicalKeyVersion='unicode-16.0.0.0-nfkc-casefold-whitespace-v1')
    receipt = dict(PlanID=plan, Revision=1, Notices=[], ChangeRecordID=record, DecisionID=decision, ChangeRecordIntegrityDigest='c'*64)
    change = dict(ID=record, PlanID=plan, InteractionID=interaction, Source='userDirect', DecisionID=decision, Changes=[dict(Kind='setCurrent', ChainID=chain, Before=dict(Category=None, Labels=None), After=dict(Category=dict(ID=cat, Kind='category', Name=common['Name'], ColorHex='#D87831', Resolution='new'), Labels=None))], Notices=[], CommittedAt=later, Revision=1, PlanDigest='b'*64, IntegrityDigest='c'*64)
    change['PlanDigest'], change['IntegrityDigest'] = b.fixed_digests(change)
    receipt['ChangeRecordIntegrityDigest'] = change['IntegrityDigest']
    return dict(Version=1, Graph=dict(
        Days=[dict(ID=day, Date=date, LockedAt=None, ReviewSummary=None, ReviewUnfinishedReason=None, ReviewTomorrowNote=None, CreatedAt=now, UpdatedAt=now)],
        Chains=[dict(ID=chain, State='active', CycleMembership=None, NoteEntries=[], CreatedAt=now, UpdatedAt=now)],
        Definitions=[dict(ID=definition, ChainID=chain, Sequence=1, Title='Native smoke task v1', DescriptionText=None, PlannedSubtasks=[], CreatedAt=now, ContentUpdatedAt=now, SupersededAt=None, SupersededByDefinitionID=None)],
        Traces=[dict(ID=trace, ChainID=chain, DefinitionID=definition, Date=date, Status='pending', Priority=1, PinOrder=None, ContinuationSeq=0, DescriptionText=None, NoteEntries=[], ManualProgressPercent=None, CarriedFromTraceID=None, ChangedToTraceID=None, CreatedAt=now, ContentUpdatedAt=now, CompletedAt=None, SettledAt=None, DraftCancellationID=None, DraftCancelledOn=None)], Subtasks=[]),
        Classification=dict(FormatVersion=1, Revision=1, Categories={cat: dict(ID=cat, **common, ColorHex='#D87831', PresentationApproval='userApproved', Lifecycle='active', NameVersions=[dict(ID=nv, **common, ValidFrom=later, ValidUntil=None)], CreatedAt=later, UpdatedAt=later)}, CurrentByChainID={chain: dict(Category=dict(CategoryID=cat, Source='userDirect', DecisionID=decision, CreatedAt=later, UpdatedAt=later, Revision=1))}, RelationHistory=[], ChangeRecords=[change], CommittedReceiptsByInteractionID={interaction: receipt}, SnapshotsByTraceID={}, SnapshotEventsByTraceID={}))


def fixture(stage='create'):
    f = fixture_facts()
    payload = json.dumps(f, separators=(',', ':'))
    change = f['Classification']['ChangeRecords'][0]
    outcome = dict(Result=1, WriteExtent=5, Session=1, FailureStage=1, Error='')
    ops = [dict(name='AddToday', error='', outcome=None, preparation=None, interaction_id='', plan_id='', receipt=None), dict(name='ReplaceCategorySelectionOn', error='', outcome=outcome, preparation=dict(outcome, WriteExtent=1), interaction_id=change['InteractionID'], plan_id=change['PlanID'], receipt=dict(f['Classification']['CommittedReceiptsByInteractionID'][change['InteractionID']], Notices=None))]
    phases = ['startup', 'after_open'] + (['before_add', 'after_add', 'before_category', 'after_category'] if stage == 'create' else []) + ['publish_ack', 'finalize']
    return dict(version=1, scenario=b.SCENARIO, action_level='service-command', stage=stage, profile='ordinary-development', actual_today='2026-11-06', app_pid=1246 if stage == 'create' else 1256, window_id=1, window_binding_confirmed=True, ui_read_view_acknowledged=True, ui_category_callback_tested=False, os_input_tested=False, display_pixels_verified=False, ime_tested=False, date_samples=[dict(phase=p, date='2026-11-06', zone='UTC', offset_seconds=0) for p in phases], before_store_revision=0 if stage == 'create' else 2, store_revision=2, before_classification_revision=0 if stage == 'create' else 1, classification_revision=1, operations=ops if stage == 'create' else [], pre_close_health=dict(Session=1, Reason=1, FailureStage=1), persistent_facts_json=payload, persistent_facts_sha256=hashlib.sha256(payload.encode()).hexdigest(), complete=True, errors=[])


class BusinessSchemaTests(unittest.TestCase):
    def test_valid_complete_synthetic_pair_and_bit_precision(self):
        a, z = fixture(), fixture('open')
        b.validate(a, 'create'); b.validate(z, 'open'); b.compare(a, z)
        self.assertGreater(fixture_facts()['Graph']['Chains'][0]['CreatedAt'], 2**53)
        self.assertNotEqual(bits(1234.123456789), int(float(bits(1234.123456789))))

    def test_mode_is_explicit_single_host(self):
        b.mode(b.SCENARIO, 'linux/amd64')
        for mode, host in [(None, 'linux/amd64'), ('other', 'linux/amd64'), (b.SCENARIO, 'linux/arm64'), (b.SCENARIO, 'darwin/amd64')]:
            with self.assertRaises(ValueError):
                b.mode(mode, host)

    def test_every_missing_and_unknown_report_field(self):
        for key in b.FIELDS:
            r = fixture(); del r[key]
            with self.subTest(key=key), self.assertRaises(ValueError):
                b.validate(r, 'create')
        r = fixture(); r['unknown'] = None
        with self.assertRaises(ValueError): b.validate(r, 'create')

    def test_strict_payload_duplicate_trailing_and_nonfinite(self):
        for payload in ['{"Version":1,"Version":1}', '{}{}', '{"x":NaN}', '{"x":Infinity}', '{"x":1e999}', '{"x":1.0}']:
            r = fixture(); r['persistent_facts_json'] = payload; r['persistent_facts_sha256'] = hashlib.sha256(payload.encode()).hexdigest()
            with self.assertRaises(ValueError): b.validate(r, 'create')

    def test_every_nested_field_required_and_unknown_rejected(self):
        f = fixture_facts()
        def paths(value, prefix=()):
            if type(value) is dict:
                yield prefix
                for key, child in value.items(): yield from paths(child, prefix+(key,))
            elif type(value) is list:
                for index, child in enumerate(value): yield from paths(child, prefix+(index,))
        for path in paths(f):
            original = f
            for part in path: original = original[part]
            for field in [None, *original]:
                mutated = copy.deepcopy(f); node = mutated
                for part in path: node = node[part]
                if field is None: node['unsupported'] = None
                else: del node[field]
                with self.subTest(path=path, missing_field=field), self.assertRaises((ValueError, KeyError)):
                    b.facts(mutated, '2026-11-06')

    def test_fact_values_nil_arrays_wrong_ids_and_instants(self):
        changes = [lambda f: f['Graph'].update(Subtasks=None), lambda f: f['Graph']['Traces'][0].update(Priority=True), lambda f: f['Graph']['Traces'][0].update(ChainID='bad'), lambda f: f['Classification'].update(RelationHistory=None), lambda f: f['Classification']['ChangeRecords'][0].update(CommittedAt=bits(float('inf'))), lambda f: f['Graph']['Chains'][0].update(CreatedAt=float(bits(1234.123456789))), lambda f: f['Classification']['ChangeRecords'][0]['Changes'][0].update(Kind='other')]
        for change in changes:
            f = fixture_facts(); change(f)
            with self.assertRaises(ValueError): b.facts(f, '2026-11-06')
        for value in (-1, 2**64, True, 1.0, bits(float('nan'))):
            with self.assertRaises(ValueError): b.instant(value)
        self.assertLess(b.instant(bits(-2.0)), b.instant(bits(-1.0)))

    def test_outcome_and_dates_fail_closed(self):
        changes = [lambda r: r['operations'][0].update(outcome={}), lambda r: r['operations'][1]['outcome'].update(WriteExtent=4), lambda r: r['operations'][1].update(receipt=None), lambda r: r['date_samples'][3].update(date='2026-11-07'), lambda r: r['date_samples'][3].update(offset_seconds=3600), lambda r: r.update(store_revision=3), lambda r: r.update(ui_read_view_acknowledged=False), lambda r: r.update(complete=False)]
        for change in changes:
            r = fixture(); change(r)
            with self.assertRaises(ValueError): b.validate(r, 'create')
        r = fixture('open'); r['operations'] = fixture()['operations']
        with self.assertRaises(ValueError): b.validate(r, 'open')

    def test_coherently_forged_digest_receipt_rejected(self):
        f = fixture_facts(); a = f['Classification']; r = a['ChangeRecords'][0]
        r['PlanDigest'] = 'a'*64; r['IntegrityDigest'] = 'b'*64
        a['CommittedReceiptsByInteractionID'][r['InteractionID']]['ChangeRecordIntegrityDigest'] = r['IntegrityDigest']
        with self.assertRaises(ValueError): b.facts(f, '2026-11-06')

    def test_raw_payload_byte_equality_not_reencoding(self):
        a, z = fixture(), fixture('open')
        z['persistent_facts_json'] = json.dumps(json.loads(z['persistent_facts_json']), indent=1)
        z['persistent_facts_sha256'] = hashlib.sha256(z['persistent_facts_json'].encode()).hexdigest()
        b.validate(z, 'open')
        with self.assertRaises(ValueError): b.compare(a, z)


class BusinessCollectorTests(test_collect.SyntheticCollectionTests):
    # Reuse only fixture helpers, not parent's tests (which intentionally require legacy mode).
    def business_fixture(self):
        self.change('result.json', scope=b.SCOPE, business_scenario=b.SCENARIO)
        for n, stage in ((1, 'create'), (2, 'open')):
            self.write(f'stage{n}-business.json', fixture(stage))
            self.change(f'stage{n}-driver.json', business_scenario=b.SCENARIO, spawn_sequence=n, business_report_sha256=hashlib.sha256((self.root/f'stage{n}-business.json').read_bytes()).hexdigest())

        self.change('result.json', business_report_sha256={f'stage{n}-business.json': hashlib.sha256((self.root/f'stage{n}-business.json').read_bytes()).hexdigest() for n in (1, 2)})

    def test_business_positive_and_legacy_cross_rejection(self):
        self.business_fixture()
        m = test_collect.m
        m.collect(self.parent, self.output, self.expected, self.stages, self.artifact, b.SCENARIO)
        m.gate(self.output, self.expected, self.artifact, b.SCENARIO)
        with self.assertRaises(ValueError): m.gate(self.output, self.expected, self.artifact)

    def test_legacy_evidence_rejected_in_business_mode(self):
        m = test_collect.m
        m.collect(self.parent, self.output, self.expected, self.stages, self.artifact, b.SCENARIO)
        with self.assertRaises(ValueError): m.gate(self.output, self.expected, self.artifact, b.SCENARIO)

    def test_business_wrong_spawn_sequence(self):
        self.business_fixture()
        m = test_collect.m
        self.change('stage2-driver.json', spawn_sequence=1)
        m.collect(self.parent, self.output, self.expected, self.stages, self.artifact, b.SCENARIO)
        with self.assertRaises(ValueError): m.gate(self.output, self.expected, self.artifact, b.SCENARIO)

    def test_business_missing_report_oversize_and_symlink(self):
        self.business_fixture()
        m = test_collect.m
        path = self.root / 'stage2-business.json'
        original = path.read_bytes()
        for index, mode in enumerate(('missing', 'oversize', 'symlink')):
            path.unlink(missing_ok=True)
            if mode == 'oversize': path.write_bytes(b'x'*(b.MAX_REPORT+1))
            if mode == 'symlink': path.symlink_to(self.root / 'stage1-business.json')
            self.output = self.base / ('failure-' + str(index))
            m.collect(self.parent, self.output, self.expected, self.stages, self.artifact, b.SCENARIO)
            with self.assertRaises(ValueError): m.gate(self.output, self.expected, self.artifact, b.SCENARIO)
            path.unlink(missing_ok=True); path.write_bytes(original)

    def test_business_wrong_pid_window_hash_and_result_scenario(self):
        self.business_fixture()
        m = test_collect.m
        original = self.read('stage2-business.json')
        for index, patch in enumerate((dict(app_pid=999), dict(window_id=999), dict(ui_read_view_acknowledged=False))):
            self.write('stage2-business.json', dict(original, **patch))
            digest = hashlib.sha256((self.root/'stage2-business.json').read_bytes()).hexdigest()
            self.change('stage2-driver.json', business_report_sha256=digest)
            result = self.read('result.json'); result['business_report_sha256']['stage2-business.json'] = digest
            self.write('result.json', result)
            self.output = self.base / ('binding-' + str(index))
            m.collect(self.parent, self.output, self.expected, self.stages, self.artifact, b.SCENARIO)
            with self.assertRaises(ValueError): m.gate(self.output, self.expected, self.artifact, b.SCENARIO)

    def test_legacy_collection_rejects_business_files_before_export(self):
        self.business_fixture()
        record = self.collect()
        self.assertIn('source_unavailable', record)
        self.assertEqual(record['exported'], {})


class BusinessDriverTests(unittest.TestCase):
    def test_duplicate_or_empty_scenario_rejected(self):
        argv = ['--artifact', '/synthetic/artifact', '--target', 'linux/amd64', '--source', 'a'*40,
                '--run-id', '1', '--run-attempt', '1', '--consumer-attempt', '1', '--artifact-id', '1', '--work-parent', '/synthetic/work']
        for extra in (['--business-scenario', ''], ['--business-scenario', b.SCENARIO]*2):
            with self.assertRaises(SystemExit): test_run.m.parser().parse_args(argv+extra)

    def test_date_boundary_does_not_start_second_process(self):
        m = test_run.m
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); artifact = root/'artifact'; artifact.mkdir()
            work = root/'work'; work.mkdir()
            exe = artifact/'binary'; exe.write_text('synthetic, never executed')
            (artifact/'manifest.json').write_text('{}')
            args = m.parser().parse_args(['--artifact', str(artifact), '--target', 'linux/amd64', '--source', 'a'*40,
                '--run-id', '1', '--run-attempt', '1', '--consumer-attempt', '1', '--artifact-id', '1', '--work-parent', str(work), '--business-scenario', b.SCENARIO])
            first = dict(today='1900-01-01', session_path='synthetic', session_files={}, business_report_sha256='a'*64)
            with mock.patch.object(m.smoke, 'verify', return_value=exe), mock.patch.object(m.smoke, 'native_host', return_value='linux/amd64'), mock.patch.object(m, 'stage', return_value=first) as stages:
                self.assertEqual(m.run(args), 1)
                self.assertEqual(stages.call_count, 1)
                self.assertEqual(stages.call_args.args[-1], b.SCENARIO)
                result = json.loads((next(work.iterdir())/'result.json').read_text())
                self.assertEqual(result['failure'], 'date-boundary-not-covered')


if __name__ == '__main__':
    unittest.main()
