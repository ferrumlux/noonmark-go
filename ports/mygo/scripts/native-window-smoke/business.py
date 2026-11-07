"""Explicit single-host Service-command evidence. Never UI/input/pixel proof."""
import argparse
import datetime
import hashlib
import json
import math
import re
import struct

SCENARIO = 'ordinary-category-v1'
SCOPE = 'native ordinary Service-command business cold-open'
MAX_REPORT = 64 << 10


class ScenarioAction(argparse.Action):
    def __call__(self, parser, namespace, values, option_string=None):
        if getattr(namespace, self.dest, None) is not None:
            parser.error('duplicate business scenario')
        setattr(namespace, self.dest, values)

FIELDS = set('version scenario action_level stage profile actual_today app_pid window_id window_binding_confirmed ui_read_view_acknowledged ui_category_callback_tested os_input_tested display_pixels_verified ime_tested date_samples before_store_revision store_revision before_classification_revision classification_revision operations pre_close_health persistent_facts_json persistent_facts_sha256 complete errors'.split())


def require(ok, message):
    if not ok:
        raise ValueError(message)


def mode(scenario, target):
    require(scenario == SCENARIO and target == 'linux/amd64', 'unsupported business scenario/host')


def keys(value, expected, label):
    require(type(value) is dict and set(value) == set(expected.split()), label + ' fields')


def unique(pairs):
    result = {}
    for k, v in pairs:
        require(k not in result, 'duplicate JSON key')
        result[k] = v
    return result


def strict(data):
    return json.loads(data, object_pairs_hook=unique, parse_float=lambda _: (_ for _ in ()).throw(ValueError('floating JSON forbidden')), parse_constant=lambda _: (_ for _ in ()).throw(ValueError('nonfinite JSON')))


def integer(value, lo=0, hi=(1 << 64)-1):
    require(type(value) is int and lo <= value <= hi, 'integer range/type')


def instant(value):
    integer(value)
    seconds = struct.unpack('>d', struct.pack('>Q', value))[0]
    require(math.isfinite(seconds), 'nonfinite Instant bits')
    return seconds


def uuid(value):
    require(type(value) is str and re.fullmatch('[0-9A-F]{8}(?:-[0-9A-F]{4}){3}-[0-9A-F]{12}', value), 'UUID')


def digest(value):
    require(type(value) is str and re.fullmatch('[0-9a-f]{64}', value), 'digest')


def read(path, stage):
    # Import late to avoid driver/business import cycles.
    import run
    require(0 < run.smoke.plain(path).st_size <= MAX_REPORT, 'business report size')
    with path.open('rb') as stream:
        data = stream.read(MAX_REPORT + 1)
    require(len(data) <= MAX_REPORT, 'business report grew')
    return validate(strict(data), stage)


def validate(r, stage):
    require(type(r) is dict and set(r) == FIELDS, 'business report fields')
    require(type(r['version']) is int and r['version'] == 1 and r['scenario'] == SCENARIO
            and r['action_level'] == 'service-command' and r['stage'] == stage
            and r['profile'] == 'ordinary-development', 'business identity')
    for key in ('app_pid', 'window_id'):
        integer(r[key], 1)
    for key in ('window_binding_confirmed', 'ui_read_view_acknowledged', 'complete'):
        require(r[key] is True, 'business acknowledgement missing')
    for key in ('ui_category_callback_tested', 'os_input_tested', 'display_pixels_verified', 'ime_tested'):
        require(r[key] is False, 'unsupported business claim')
    require(r['errors'] == [], 'business failure')
    require(type(r['actual_today']) is str and re.fullmatch(r'\d{4}-\d{2}-\d{2}', r['actual_today']), 'business date')
    datetime.date.fromisoformat(r['actual_today'])
    phases = ['startup', 'after_open'] + (['before_add', 'after_add', 'before_category', 'after_category'] if stage == 'create' else []) + ['publish_ack', 'finalize']
    samples = r['date_samples']
    require(type(samples) is list and len(samples) == len(phases), 'date samples')
    for sample, phase in zip(samples, phases):
        keys(sample, 'phase date zone offset_seconds', 'date sample')
        require(sample['phase'] == phase and sample['date'] == r['actual_today'], 'date-boundary-not-covered')
        require(type(sample['zone']) is str and 0 < len(sample['zone']) <= 128, 'timezone')
        integer(sample['offset_seconds'], -86400, 86400)
        require((sample['zone'], sample['offset_seconds']) == (samples[0]['zone'], samples[0]['offset_seconds']), 'date-boundary-not-covered')
    health = r['pre_close_health']
    keys(health, 'Session Reason FailureStage', 'health')
    require(all(type(v) is int and v == 1 for v in health.values()), 'unhealthy business')
    for key in ('before_store_revision', 'store_revision', 'before_classification_revision', 'classification_revision'):
        integer(r[key])
    require(r['classification_revision'] == 1, 'classification revision')
    require(r['before_classification_revision'] == (0 if stage == 'create' else 1), 'initial classification revision')
    require(r['store_revision'] == r['before_store_revision'] + (2 if stage == 'create' else 0), 'store revision delta')
    payload = r['persistent_facts_json']
    require(type(payload) is str, 'facts JSON type')
    raw = payload.encode('utf-8')
    require(len(raw) <= MAX_REPORT and hashlib.sha256(raw).hexdigest() == r['persistent_facts_sha256'], 'facts raw digest')
    f = strict(raw)
    facts(f, r['actual_today'])
    operations(r['operations'], stage, f)
    return r


def operations(ops, stage, facts):
    require(type(ops) is list and len(ops) == (2 if stage == 'create' else 0), 'business operations')
    if not ops:
        return
    for op, name in zip(ops, ['AddToday', 'ReplaceCategorySelectionOn']):
        keys(op, 'name error outcome preparation interaction_id plan_id receipt', 'operation')
        require(op['name'] == name and op['error'] == '', 'operation failed/order')
    add, category = ops
    require(all(add[k] is None for k in ('outcome', 'preparation', 'receipt')) and add['interaction_id'] == add['plan_id'] == '', 'fabricated AddToday outcome')
    for key, extent in (('outcome', 5), ('preparation', 1)):
        o = category[key]
        keys(o, 'Result WriteExtent Session FailureStage Error', 'typed outcome')
        require(o == {'Result': 1, 'WriteExtent': extent, 'Session': 1, 'FailureStage': 1, 'Error': ''}
                and all(type(o[k]) is int for k in ('Result', 'WriteExtent', 'Session', 'FailureStage')), 'category outcome')
    record = facts['Classification']['ChangeRecords'][0]
    require(category['interaction_id'] == record['InteractionID'] and category['plan_id'] == record['PlanID'], 'operation identity linkage')
    receipt = category['receipt']
    keys(receipt, 'PlanID Revision Notices ChangeRecordID DecisionID ChangeRecordIntegrityDigest', 'command receipt')
    # The command returns nil Notices; the wire-decoded read view is []. These
    # are separate observations, not normalization of persistent DTO bytes.
    require(receipt['Notices'] is None, 'command receipt notices')
    persistent = facts['Classification']['CommittedReceiptsByInteractionID'][record['InteractionID']]
    exact(receipt, dict(persistent, Notices=None))


def binding(b, lifecycle, pid):
    require(b['actual_today'] == lifecycle['today'] and b['window_id'] == lifecycle['window_id'] and b['app_pid'] == pid, 'business window/process binding')


def compare(first, second):
    require(first['stage'] == 'create' and second['stage'] == 'open', 'business stage order')
    for key in ('actual_today', 'store_revision', 'classification_revision', 'persistent_facts_json', 'persistent_facts_sha256'):
        require(first[key] == second[key], 'business cold-open mismatch: ' + key)
    require(strict(first['persistent_facts_json']) == strict(second['persistent_facts_json']), 'business facts mismatch')
    require(first['date_samples'][0]['zone'] == second['date_samples'][0]['zone']
            and first['date_samples'][0]['offset_seconds'] == second['date_samples'][0]['offset_seconds'], 'date-boundary-not-covered')


def exact(actual, expected):
    """Equality retaining JSON types (not Python's True == 1 equivalence)."""
    require(type(actual) is type(expected), 'facts field type')
    if type(expected) is dict:
        require(set(actual) == set(expected), 'facts object fields')
        for k in expected:
            exact(actual[k], expected[k])
    elif type(expected) is list:
        require(len(actual) == len(expected), 'facts collection cardinality')
        for a, b in zip(actual, expected):
            exact(a, b)
    else:
        require(actual == expected, 'facts field value')


def facts(f, today):
    keys(f, 'Version Graph Classification', 'persistent facts')
    require(type(f['Version']) is int and f['Version'] == 1, 'facts version')
    g, a = f['Graph'], f['Classification']
    keys(g, 'Days Chains Definitions Traces Subtasks', 'graph')
    for k in ('Days', 'Chains', 'Definitions', 'Traces'):
        require(type(g[k]) is list and len(g[k]) == 1 and type(g[k][0]) is dict, 'task cardinality')
    chain, definition, trace, day = [g[k][0] for k in ('Chains', 'Definitions', 'Traces', 'Days')]
    ids = [x.get('ID') for x in (chain, definition, trace, day)]
    for id_ in ids:
        uuid(id_)
    require(len(set(ids)) == len(ids), 'duplicate entity identity')
    now = chain.get('CreatedAt')
    instant(now)
    year, month, d = map(int, today.split('-'))
    date = {'Year': year, 'Month': month, 'Day': d}
    want_graph = {
        'Days': [dict(ID=ids[3], Date=date, LockedAt=None, ReviewSummary=None, ReviewUnfinishedReason=None, ReviewTomorrowNote=None, CreatedAt=now, UpdatedAt=now)],
        'Chains': [dict(ID=ids[0], State='active', CycleMembership=None, NoteEntries=[], CreatedAt=now, UpdatedAt=now)],
        'Definitions': [dict(ID=ids[1], ChainID=ids[0], Sequence=1, Title='Native smoke task v1', DescriptionText=None, PlannedSubtasks=[], CreatedAt=now, ContentUpdatedAt=now, SupersededAt=None, SupersededByDefinitionID=None)],
        'Traces': [dict(ID=ids[2], ChainID=ids[0], DefinitionID=ids[1], Date=date, Status='pending', Priority=1, PinOrder=None, ContinuationSeq=0, DescriptionText=None, NoteEntries=[], ManualProgressPercent=None, CarriedFromTraceID=None, ChangedToTraceID=None, CreatedAt=now, ContentUpdatedAt=now, CompletedAt=None, SettledAt=None, DraftCancellationID=None, DraftCancelledOn=None)],
        'Subtasks': []}
    exact(g, want_graph)
    keys(a, 'FormatVersion Revision Categories CurrentByChainID RelationHistory ChangeRecords CommittedReceiptsByInteractionID SnapshotsByTraceID SnapshotEventsByTraceID', 'classification')
    require(type(a['Categories']) is dict and len(a['Categories']) == 1 and type(a['ChangeRecords']) is list and len(a['ChangeRecords']) == 1, 'category/record cardinality')
    catid, cat = next(iter(a['Categories'].items()))
    uuid(catid)
    keys(cat, 'ID Name CanonicalKey CanonicalKeyVersion ColorHex PresentationApproval Lifecycle NameVersions CreatedAt UpdatedAt', 'category')
    require(type(cat['NameVersions']) is list and len(cat['NameVersions']) == 1, 'name version count')
    nv = cat['NameVersions'][0]
    keys(nv, 'ID Name CanonicalKey CanonicalKeyVersion ValidFrom ValidUntil', 'name version')
    uuid(nv['ID'])
    record = a['ChangeRecords'][0]
    keys(record, 'ID PlanID InteractionID Source DecisionID Changes Notices CommittedAt Revision PlanDigest IntegrityDigest', 'change record')
    for k in ('ID', 'PlanID', 'InteractionID', 'DecisionID'):
        uuid(record[k])
    for k in ('PlanDigest', 'IntegrityDigest'):
        digest(record[k])
    category_now = record['CommittedAt']
    require(instant(category_now) > instant(now), 'category clock ordering')
    all_ids = ids + [catid, nv['ID'], record['ID'], record['PlanID'], record['InteractionID'], record['DecisionID']]
    require(len(set(all_ids)) == len(all_ids), 'duplicate category identity')
    common = dict(Name='Native smoke category v1', CanonicalKey='native smoke category v1', CanonicalKeyVersion='unicode-16.0.0.0-nfkc-casefold-whitespace-v1')
    changes = [dict(Kind='setCurrent', ChainID=ids[0], Before=dict(Category=None, Labels=None), After=dict(Category=dict(ID=catid, Kind='category', Name=common['Name'], ColorHex='#D87831', Resolution='new'), Labels=None))]
    want_record = dict(record, Source='userDirect', Changes=changes, Notices=[], Revision=1)
    receipt = dict(PlanID=record['PlanID'], Revision=1, Notices=[], ChangeRecordID=record['ID'], DecisionID=record['DecisionID'], ChangeRecordIntegrityDigest=record['IntegrityDigest'])
    want = dict(FormatVersion=1, Revision=1,
                Categories={catid: dict(ID=catid, **common, ColorHex='#D87831', PresentationApproval='userApproved', Lifecycle='active', NameVersions=[dict(ID=nv['ID'], **common, ValidFrom=category_now, ValidUntil=None)], CreatedAt=category_now, UpdatedAt=category_now)},
                CurrentByChainID={ids[0]: dict(Category=dict(CategoryID=catid, Source='userDirect', DecisionID=record['DecisionID'], CreatedAt=category_now, UpdatedAt=category_now, Revision=1))},
                RelationHistory=[], ChangeRecords=[want_record], CommittedReceiptsByInteractionID={record['InteractionID']: receipt}, SnapshotsByTraceID={}, SnapshotEventsByTraceID={})
    exact(a, want)
    plan_digest, record_digest = fixed_digests(record)
    require(record['PlanDigest'] == plan_digest and record['IntegrityDigest'] == record_digest, 'category canonical digest mismatch')


def fixed_digests(record):
    """Source-derived ordinary fixed ASCII scenario, not a general Swift codec.

    Matches ordinary/digest.go planPayload and aggregate_digest.go. Optional
    CreatedItemID is absent for ChoiceNew; the planned category has a UUID
    wrapper. Integer bit patterns stay integers throughout.
    """
    change = record['Changes'][0]
    item = change['After']['Category']
    after = dict(category={k[0].lower()+k[1:]: v for k, v in item.items()}, labels=[])
    # Acronym ID becomes id, not iD.
    after['category']['id'] = after['category'].pop('iD')
    changes = [dict(setCurrent=dict(after=after, before=dict(labels=[]), chainID=dict(rawValue=change['ChainID'])))]
    plan = dict(baseRevision=0, blockers=[], changes=changes, formatVersion=1,
                interactionID=record['InteractionID'], notices=[], planID=record['PlanID'],
                plannedCategoryID=dict(rawValue=item['ID']), plannedLabelIDs=[], source=dict(userDirect={}))
    def canonical_hash(value):
        raw = json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False)
        require(raw.isascii() and '/' not in raw, 'non-fixed canonical scenario')
        return hashlib.sha256(raw.encode('ascii')).hexdigest()
    plan_hash = canonical_hash(plan)
    committed = dict(changes=changes, committedAtBitPattern=record['CommittedAt'], decisionID=record['DecisionID'],
                     formatVersion=1, id=record['ID'], interactionID=record['InteractionID'], notices=[],
                     planDigest=plan_hash, planID=record['PlanID'], revision=1, source=dict(userDirect={}))
    return plan_hash, canonical_hash(committed)
