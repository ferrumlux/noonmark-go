# SPDX-License-Identifier: AGPL-3.0-only
"""R3 M1 comparison contracts. Inputs must first pass bounded/schema preflight.

No serialization, product sorting, decoder invocation or implicit ID matching occurs
here. Entity maps are keyed by causal identities; array order is a separate view.
Synthetic tests of these routines are not runtime interoperability evidence.
"""
import hashlib
import re

class MappingError(ValueError):
    pass

class ReportError(ValueError):
    pass

KINDS = {'days':'day','chains':'chain','definitions':'definition','traces':'trace','subtasks':'subtask','plannedSubtasks':'planned','noteEntries':'note'}
TYPE_KIND = {'Day':'day','Chain':'chain','Definition':'definition','Trace':'trace','Subtask':'subtask','PlannedSubtask':'planned','Note':'note'}
REFERENCES = {'chainID':'chain','definitionID':'definition','traceID':'trace','lineageID':'lineage'}
LAYERS = ('entity','array-sequence','raw-package-bytes','raw-export-bytes','swift-self-roundtrip','go-self-roundtrip','cross-decode')
OBSERVATIONS = ('swift-engine-projection','go-kernel-projection','go-from-domain-projection','swift-package','go-package','swift-export','go-export','swift-self-reencoded','go-self-reencoded')
STAGES = ('preflight','seed-prepare','seed-decode','seed-core','command','snapshot','from-domain','encode-package','encode-export','self-decode','self-encode','cross-preflight','cross-inspect','cross-decode')
M2_REQUIRED = ('A01-addPlanned','A05-id-creation','A06-old-definition','A07-multiple-deleted-notes','A07-note-permutation','A08-dictionary-order','B01-nonmonotonic-now','B02-date','B03-negative-priority-x-date','B04-controlled-tie','B05-random-tie','C01-review-locked','C02-clock-bits','C03-date-boundary','N02-description-pairs','N03-blank-note-domain','H01','H02','H03','H04')
UUID = re.compile(r'[0-9A-F]{8}(?:-[0-9A-F]{4}){3}-[0-9A-F]{12}\Z')
SHA = re.compile(r'[0-9a-f]{64}\Z')
KEY = re.compile(r'[a-z][a-z0-9-]{0,79}\Z')


def _id(value):
    if not isinstance(value,dict) or set(value)!={'rawValue'} or not isinstance(value['rawValue'],str) or not UUID.fullmatch(value['rawValue']):
        raise MappingError('schema-invalid: identity')
    return value['rawValue']


def _inventory(envelope):
    """Return typed identities and structural ownership, without array positions."""
    entities={kind:{} for kind in (*TYPE_KIND.values(),'lineage')}
    owners={}; lineages={}
    def add(kind,obj,owner=None):
        raw=_id(obj['id'])
        if raw in entities[kind]:
            # A note is intentionally repeated in its chain and trace projection.
            if kind!='note' or entities[kind][raw]!=obj or owners[(kind,raw)]!=owner:
                raise MappingError('mapping-ambiguous: duplicate identity')
        else:entities[kind][raw]=obj;owners[(kind,raw)]=owner
        if kind in ('planned','subtask'):
            lineage=_id(obj['lineageID']);entities['lineage'].setdefault(lineage,{'id':{'rawValue':lineage}})
            lineages[(kind,raw)]=lineage
        return raw
    s=envelope['snapshot']
    for section in ('days','chains','definitions','traces','subtasks'):
        kind=KINDS[section]
        for obj in s[section]:
            owner=None
            if kind in ('definition','trace'):owner=('chain',_id(obj['chainID']))
            if kind=='subtask':owner=('trace',_id(obj['traceID']))
            raw=add(kind,obj,owner)
            if kind=='definition':
                seen=set()
                for child in obj['plannedSubtasks']:
                    lineage=_id(child['lineageID'])
                    if lineage in seen:raise MappingError('mapping-ambiguous: duplicate planned lineage')
                    seen.add(lineage);add('planned',child,('definition',raw))
            if kind in ('chain','trace'):
                note_owner=('chain',raw if kind=='chain' else _id(obj['chainID']))
                local=set()
                for note in obj['noteEntries']:
                    nr=_id(note['id'])
                    if nr in local:raise MappingError('mapping-ambiguous: duplicate note occurrence')
                    local.add(nr);add('note',note,note_owner)
    for (kind,raw),owner in owners.items():
        if owner and owner[1] not in entities[owner[0]]:raise MappingError('mapping-ambiguous: dangling owner')
        obj=entities[kind][raw]
        if kind=='trace' and _id(obj['definitionID']) not in entities['definition']:raise MappingError('mapping-ambiguous: dangling definition')
        if kind=='subtask':
            trace=entities['trace'][owner[1]]
            definition=entities['definition'][_id(trace['definitionID'])]
            if not any(_id(p['lineageID'])==lineages[(kind,raw)] for p in definition['plannedSubtasks']):raise MappingError('mapping-ambiguous: missing trace lineage')
    return entities,owners,lineages


def _side_mapping(seed,current,command,returned):
    base,_,_=_inventory(seed);entities,owners,lineages=_inventory(current)
    mapping={};rules={};origins={}
    for kind,items in base.items():
        if not set(items)<=set(entities[kind]):raise MappingError('mapping-ambiguous: missing seed entity')
        for raw in items:
            mapping[(kind,raw)]=kind+'-'+raw.lower();rules[(kind,raw)]='seed-id';origins[(kind,raw)]='seed'
    added={k:set(v)-set(base[k]) for k,v in entities.items()}
    def bind(kind,raw,key,rule):
        if raw not in added[kind]:raise MappingError('mapping-ambiguous: returned identity not uniquely new')
        mapping[(kind,raw)]=key;rules[(kind,raw)]=rule;origins[(kind,raw)]=command['commandKey'];added[kind].remove(raw)
    if command:
        key=command['commandKey'];op=command['op']
        if op=='create':
            chain=returned.get('chain')
            if not chain or len(added['chain'])!=1:raise MappingError('mapping-ambiguous: returned chain')
            bind('chain',chain,'chain-'+key,'returned-chain')
            ds=[raw for raw in added['definition'] if owners[('definition',raw)]==('chain',chain) and entities['definition'][raw]['sequence']==1]
            if len(ds)!=1:raise MappingError('mapping-ambiguous: new owner definition')
            bind('definition',ds[0],'definition-'+key,'new-owner-definition')
            ns=[raw for raw in added['note'] if owners[('note',raw)]==('chain',chain)]
            if len(ns)>1:raise MappingError('mapping-ambiguous: new owner note')
            if ns:bind('note',ns[0],'note-'+key,'new-owner-note')
        elif op=='schedule':
            trace=returned.get('trace')
            if not trace or len(added['trace'])!=1:raise MappingError('mapping-ambiguous: returned trace')
            if owners[('trace',trace)]!=('chain',command['chainRawID']):raise MappingError('mapping-ambiguous: returned trace owner')
            bind('trace',trace,'trace-'+key,'returned-trace')
            dates=[raw for raw,obj in entities['day'].items() if obj['date']==command['date']]
            if len(dates)!=1:raise MappingError('mapping-ambiguous: date day')
            if dates[0] in added['day']:bind('day',dates[0],'day-'+key,'date-day')
            definition=entities['definition'][_id(entities['trace'][trace]['definitionID'])]
            expected={_id(p['lineageID']) for p in definition['plannedSubtasks']}
            found={}
            for raw in tuple(added['subtask']):
                if owners[('subtask',raw)]!=('trace',trace):raise MappingError('mapping-ambiguous: actual owner')
                lineage=lineages[('subtask',raw)]
                if lineage not in expected or lineage in found:raise MappingError('mapping-ambiguous: actual lineage')
                found[lineage]=raw
            if set(found)!=expected:raise MappingError('mapping-ambiguous: missing actual')
            # 因果计划只决定映射收据次序；身份仍按 owner 与 lineage 验证。
            for planned in definition['plannedSubtasks']:
                lineage=_id(planned['lineageID']);bind('subtask',found[lineage],'subtask-'+lineage.lower(),'trace-lineage')
        else:raise MappingError('unsupported command')
    if any(added.values()):raise MappingError('mapping-ambiguous: extra entity')
    if len(set(mapping.values()))!=len(mapping):raise MappingError('mapping-ambiguous: duplicate logical key')
    result={}
    for ident,key in mapping.items():
        owner=owners.get(ident);lineage=lineages.get(ident)
        result[key]={'kind':ident[0],'id':ident[1],'key':key,'origin':origins[ident],'rule':rules[ident],
                     'ownerKey':mapping[owner] if owner else '', 'lineageKey':mapping[('lineage',lineage)] if lineage else ''}
    return result


def build_bindings(seed_envelope,swift_envelope,go_envelope,command=None,returned=None):
    returned=returned or {}
    left=_side_mapping(seed_envelope,swift_envelope,command,returned.get('swift',{}))
    right=_side_mapping(seed_envelope,go_envelope,command,returned.get('go',{}))
    if set(left)!=set(right):raise MappingError('mapping-ambiguous: unmatched causal entities')
    bindings=[]
    for key,a in left.items():
        b=right[key]
        if {k:v for k,v in a.items() if k!='id'}!={k:v for k,v in b.items() if k!='id'}:raise MappingError('mapping-ambiguous: causal relation mismatch')
        bindings.append({k:v for k,v in a.items() if k!='id'}|{'swiftID':a['id'],'goID':b['id']})
    return bindings


def _validated_map(bindings,envelope,side,check_relations=True):
    if len(bindings)>64:raise MappingError('resource-limit: bindings')
    entities,owners,lineages=_inventory(envelope);mapping={};keys=set()
    for b in bindings:
        if set(b)!={'kind','key','swiftID','goID','origin','rule','ownerKey','lineageKey'}:raise MappingError('schema-invalid: binding')
        ident=(b['kind'],b[side+'ID'])
        if ident in mapping or b['key'] in keys or not KEY.fullmatch(b['key']):raise MappingError('mapping-ambiguous: duplicate binding')
        if ident[0] not in entities or ident[1] not in entities[ident[0]]:raise MappingError('mapping-ambiguous: extra binding')
        mapping[ident]=b['key'];keys.add(b['key'])
    if set(mapping)!={(kind,raw) for kind,items in entities.items() for raw in items}:raise MappingError('mapping-ambiguous: incomplete binding')
    for b in bindings:
        ident=(b['kind'],b[side+'ID']);owner=owners.get(ident);lineage=lineages.get(ident)
        if check_relations and (b['ownerKey']!=(mapping[owner] if owner else '') or b['lineageKey']!=(mapping[('lineage',lineage)] if lineage else '')):raise MappingError('mapping-ambiguous: incorrect ownership')
    return mapping


def _views(envelope,schema,mapping):
    fields={};arrays={}
    def walk(value,typ,path,owner_kind=None,field=None):
        if typ in schema.get('aliases',{}):typ=schema['aliases'][typ]
        if typ.startswith('[]') or typ.startswith('pairs:'):
            if not isinstance(value,list):raise MappingError('schema-invalid: array')
            subtype=typ[2:] if typ.startswith('[]') else None
            if subtype in TYPE_KIND:
                kind=TYPE_KIND[subtype];seq=[]
                for obj in value:
                    key=mapping[(kind,_id(obj['id']))];seq.append(key)
                    walk(obj,subtype,path+'['+key+']')
                arrays[path]=seq
            else:
                # R3 M1 excludes every other nonempty array, including dictionary pairs.
                if value:raise MappingError('unsupported-profile: array')
                arrays[path]=[]
            return
        if typ=='ID':
            kind=owner_kind if field=='id' else REFERENCES.get(field)
            if not kind:raise MappingError('unsupported-profile: ID field')
            try:fields[path]=mapping[(kind,_id(value))]
            except KeyError:raise MappingError('mapping-ambiguous: unmapped reference') from None
            return
        if typ in schema['objects']:
            if not isinstance(value,dict):raise MappingError('schema-invalid: object')
            specs=schema['objects'][typ];allowed={f['wire'] for f in specs}
            if set(value)-allowed:raise MappingError('schema-invalid: unknown field')
            for spec in specs:
                name=spec['wire'];p=path+'.'+name
                if name not in value:
                    if not spec['optional']:raise MappingError('schema-invalid: missing field')
                    fields[p]=('absent',);continue
                if value[name] is None:raise MappingError('schema-invalid: null')
                walk(value[name],spec['type'],p,TYPE_KIND.get(typ),name)
            return
        if typ not in ('string','bool','int','uint','uuid','stamp') and not typ.startswith(('enum:','enumint:')):raise MappingError('unsupported-profile: missing type')
        fields[path]=(type(value).__name__,value)
    walk(envelope,'Envelope','$')
    return fields,arrays


def comparison(layer,verdict,reason='none',path='',offset=-1,left='',right=''):
    return dict(layer=layer,verdict=verdict,reason=reason,firstPath=path[:256],firstOffset=offset,leftSHA256=left,rightSHA256=right)


def _diff(layer,a,b):
    # Sorting only diagnostic schema paths, never product entities/arrays.
    for key in sorted(a.keys()|b.keys()):
        if key not in a or key not in b or a[key]!=b[key]:return comparison(layer,'different',path=key)
    return comparison(layer,'equal')


def compare_views(left,right,bindings,schema,left_side='swift',right_side='go'):
    from bounded import operation
    operation('compare')
    lm=_validated_map(bindings,left,left_side,False);rm=_validated_map(bindings,right,right_side,False)
    # Binding claims must be witnessed by at least one side. A valid reference
    # changing on the other side is then a field difference, not silently aligned.
    inventories=[_inventory(left),_inventory(right)]
    for b in bindings:
        witnessed=False
        for side,mapping,(_,owners,lineages) in zip((left_side,right_side),(lm,rm),inventories):
            ident=(b['kind'],b[side+'ID']);owner=owners.get(ident);lineage=lineages.get(ident)
            witnessed |= b['ownerKey']==(mapping[owner] if owner else '') and b['lineageKey']==(mapping[('lineage',lineage)] if lineage else '')
        if not witnessed:raise MappingError('mapping-ambiguous: incorrect ownership')
    lf,la=_views(left,schema,lm);rf,ra=_views(right,schema,rm)
    return _diff('entity',lf,rf),_diff('array-sequence',la,ra)


def compare_raw(layer,left,right):
    if layer not in LAYERS:raise ReportError('unknown layer')
    if not isinstance(left,bytes) or not isinstance(right,bytes):raise ReportError('raw bytes required')
    if max(len(left),len(right))>262144:raise ReportError('resource-limit')
    offset=next((i for i,(a,b) in enumerate(zip(left,right)) if a!=b),min(len(left),len(right))) if left!=right else -1
    return comparison(layer,'equal' if left==right else 'different',offset=offset,left=hashlib.sha256(left).hexdigest(),right=hashlib.sha256(right).hexdigest())


def summarize(cases,registry,negative_gates,m2,*,verified_points=()):
    """Recompute status; caller must verify every artifact and comparison first.

    This strict structural gate never treats child-reported counters as evidence.
    Full negative/runtime evidence verification is the controller's responsibility.
    It must pass verified_points only after re-reading archived bytes, checking
    digests/schema, deriving bindings and recomputing every comparison. Without
    this separate verification authority, effective counts stay zero.
    """
    verified_points=set(verified_points)
    expected={c['id']:c for c in registry['cases']}
    if len(expected)!=18 or sum(1+len(c['steps']) for c in expected.values())!=32:raise ReportError('registry-mismatch')
    if len(cases)!=18 or len({c['caseID'] for c in cases})!=18 or {c['caseID'] for c in cases}!=set(expected):raise ReportError('missing/duplicate case')
    effective=interop=0
    validities=[]
    for case in cases:
        validity=case['fixtureValidity'];validities.append(validity)
        if validity not in ('pending','valid','invalid-fixture','unsupported','harness-error'):raise ReportError('unknown validity')
        points=case['points'];point_keys=['seed']+[s['commandKey'] for s in expected[case['caseID']]['steps']]
        if len(points)!=len(point_keys) or {p['key'] for p in points}!=set(point_keys):raise ReportError('missing/duplicate point')
        for p in points:
            if set(p)!={'key','stages','bindings','observations','comparisons'}:raise ReportError('point schema')
            stages=p['stages']
            if any(set(s)!={'side','name','outcome','reason'} for s in stages):raise ReportError('stage schema')
            pairs={(s['side'],s['name']) for s in stages}
            required={(side,name) for side in ('swift','go') for name in STAGES}|{('controller','preflight')}
            if len(stages)!=29 or pairs!=required:raise ReportError('missing/duplicate stage')
            stage_ok=True
            cross_rejected=any(c.get('layer')=='cross-decode' and c.get('verdict')=='rejected' for c in p['comparisons'])
            go_inspect_rejected=any(s['side']=='go' and s['name']=='cross-inspect' and s['outcome']=='rejected' and s['reason']=='codec-rejected' for s in stages)
            for s in stages:
                side,name,outcome=s['side'],s['name'],s['outcome']
                na=side=='swift' and name in ('from-domain','cross-inspect') or side!='controller' and (name=='command' if p['key']=='seed' else name in ('seed-prepare','seed-decode','seed-core'))
                if outcome not in ('success','rejected','unsupported','not-run','harness-error','not-applicable'):raise ReportError('unknown outcome')
                if outcome=='not-applicable' and not na:raise ReportError('invalid not-applicable')
                if na and outcome!='not-applicable':stage_ok=False
                observed_rejection=name in ('cross-inspect','cross-decode') and outcome=='rejected' and s['reason']=='codec-rejected' and cross_rejected
                # Go Inspect is a real terminal codec observation. Its guarded
                # Decode was never invoked and must remain not-run, not rejected.
                terminal_inspect=side=='go' and name=='cross-decode' and outcome=='not-run' and s['reason']=='dependency-failed' and go_inspect_rejected and cross_rejected
                if not na and outcome!='success' and not observed_rejection and not terminal_inspect:stage_ok=False
            comparisons=p['comparisons']
            if len(comparisons)!=7 or {c['layer'] for c in comparisons}!=set(LAYERS):raise ReportError('missing/duplicate comparison')
            for c in comparisons:
                if set(c)!={'layer','verdict','reason','firstPath','firstOffset','leftSHA256','rightSHA256'}:raise ReportError('comparison schema')
                if c['verdict'] not in ('equal','different','rejected','not-comparable','unsupported','not-run','harness-error'):raise ReportError('unknown verdict')
            observations=p['observations']
            if len(observations)>9 or len({o['layer'] for o in observations})!=len(observations):raise ReportError('duplicate observation')
            for o in observations:
                if set(o)!={'layer','artifact','rawState','reason'}:raise ReportError('observation schema')
                if o['layer'] not in OBSERVATIONS or o['rawState'] not in ('archived-unchecked','schema-valid','malformed-raw'):raise ReportError('observation schema')
                a=o['artifact']
                if a.get('name')!=case['caseID']+'/'+p['key']+'/'+o['layer']+'.json':raise ReportError('artifact name')
                if set(a)!={'name','bytes','sha256'} or not SHA.fullmatch(a['sha256']) or type(a['bytes'])!=int or not 0<=a['bytes']<=262144:raise ReportError('artifact schema')
            bindings=p['bindings']
            if any(set(b)!={'kind','key','swiftID','goID','origin','rule','ownerKey','lineageKey'} for b in bindings):raise ReportError('binding schema')
            if any(len({(b['kind'],b[side+'ID']) for b in bindings})!=len(bindings) for side in ('swift','go')):raise ReportError('duplicate binding ID')
            if len(bindings)>64 or len({b['key'] for b in bindings})!=len(bindings):raise ReportError('duplicate binding')
            observed=len(observations)==9 and {o['layer'] for o in observations}==set(OBSERVATIONS) and all(o['rawState']=='schema-valid' for o in observations)
            actual=all(c['verdict'] in ('equal','different','rejected') for c in comparisons)
            if (case['caseID'],p['key']) in verified_points and validity=='valid' and stage_ok and observed and actual:
                effective+=1
                if all(c['verdict']=='equal' for c in comparisons):interop+=1
    gates=set(negative_gates)=={'V%02d'%i for i in range(1,10)} and all(v=='pass' for v in negative_gates.values())
    # M2 must remain explicit, not disappear from a short green report.
    report_complete=set(m2)==set(M2_REQUIRED) and all(v=='not-run' for v in m2.values()) and all(v in ('pass','fail','not-run') for v in negative_gates.values()) and set(negative_gates)=={'V%02d'%i for i in range(1,10)}
    accepted=report_complete and effective==32 and gates
    return dict(fixtureValidity='valid' if all(v=='valid' for v in validities) else 'harness-error',mapping='complete' if effective==32 else 'not-run',effectivePointCount=effective,interopPointCount=interop,negativeGatePassCount=sum(v=='pass' for v in negative_gates.values()),reportComplete=report_complete,observationAccepted=accepted,interopAccepted=accepted and interop==32)
