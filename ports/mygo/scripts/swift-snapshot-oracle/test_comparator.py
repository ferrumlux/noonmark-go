"""Synthetic contract tests only: never Swift interoperability evidence."""
import copy
import json
from pathlib import Path
import unittest
from comparator import MappingError, ReportError, build_bindings, compare_views, compare_raw, summarize, STAGES, OBSERVATIONS, LAYERS, M2_REQUIRED, comparison

SPEC = Path(__file__).resolve().parent / 'spec'
if not SPEC.exists():
    SPEC = Path(__file__).resolve().parents[5] / 'snapshot-representation-oracle-design'

class ComparatorContracts(unittest.TestCase):
    def setUp(self):
        self.schema = json.loads((SPEC/'FieldSchema-R2.json').read_text())
        self.seed = json.loads((SPEC/'r2-seeds/multi-interleaved.json').read_text())
        self.bindings = build_bindings(self.seed, self.seed, self.seed)
    def test_schedule_bindings_follow_causal_plan_across_hash_seeds(self):
        import subprocess,sys,os
        script = """
import json,copy
from pathlib import Path
from comparator import build_bindings
current=json.loads((Path('spec/r2-seeds/multi-interleaved.json')).read_text())
trace=current['snapshot']['traces'][0];tid=trace['id']['rawValue']
seed=copy.deepcopy(current)
seed['snapshot']['traces']=[t for t in seed['snapshot']['traces'] if t['id']['rawValue']!=tid]
seed['snapshot']['subtasks']=[t for t in seed['snapshot']['subtasks'] if t['traceID']['rawValue']!=tid]
command={'commandKey':'test-schedule','op':'schedule','chainRawID':trace['chainID']['rawValue'],'date':trace['date']}
rows=build_bindings(seed,current,current,command,{'swift':{'trace':tid},'go':{'trace':tid}})
actual=[r['lineageKey'] for r in rows if r['rule']=='trace-lineage']
definition=next(d for d in current['snapshot']['definitions'] if d['id']==trace['definitionID'])
expected=['lineage-'+p['lineageID']['rawValue'].lower() for p in definition['plannedSubtasks']]
assert actual==expected,(actual,expected)
print(json.dumps(rows,sort_keys=True))
"""
        outputs=[subprocess.check_output([sys.executable,'-c',script],cwd=SPEC.parent,env=dict(os.environ,PYTHONHASHSEED=str(i))) for i in range(8)]
        self.assertEqual(len(set(outputs)),1)
    def test_trace_children_planned_permutations(self):
        for section in ('traces', 'subtasks', 'planned'):
            b=copy.deepcopy(self.seed)
            a = b['snapshot']['definitions'][0]['plannedSubtasks'] if section=='planned' else b['snapshot'][section]
            a.reverse()
            entity, arrays=compare_views(self.seed,b,self.bindings,self.schema)
            self.assertEqual(entity['verdict'],'equal',section)
            self.assertEqual(arrays['verdict'],'different',section)
    def test_scalar_changes_not_hidden(self):
        for key,value in [('position',9),('title','changed'),('createdAt',4636737291354636289)]:
            b=copy.deepcopy(self.seed);b['snapshot']['subtasks'][0][key]=value
            self.assertEqual(compare_views(self.seed,b,self.bindings,self.schema)[0]['verdict'],'different')
    def test_owner_reference_difference(self):
        b=copy.deepcopy(self.seed)
        b['snapshot']['definitions'][0]['chainID']=copy.deepcopy(b['snapshot']['chains'][1]['id'])
        self.assertEqual(compare_views(self.seed,b,self.bindings,self.schema)[0]['verdict'],'different')
    def test_optional_utf8_and_bits(self):
        for field,value in [('descriptionText',''),('descriptionText','e\u0301'),('createdAt',4636737291354636289)]:
            b=copy.deepcopy(self.seed);b['snapshot']['definitions'][0][field]=value
            self.assertEqual(compare_views(self.seed,b,self.bindings,self.schema)[0]['verdict'],'different')
    def test_duplicate_identity_rejected(self):
        b=copy.deepcopy(self.seed);b['snapshot']['traces'].append(b['snapshot']['traces'][0])
        with self.assertRaises(MappingError): build_bindings(self.seed,self.seed,b)
    def test_missing_entity_rejected(self):
        b=copy.deepcopy(self.seed);b['snapshot']['traces'].pop()
        with self.assertRaises(MappingError): build_bindings(self.seed,self.seed,b)
    def test_binding_tampering_rejected(self):
        for mutation in ('duplicate','owner','missing'):
            bindings=copy.deepcopy(self.bindings)
            if mutation=='duplicate': bindings.append(bindings[0])
            elif mutation=='owner': next(x for x in bindings if x['kind']=='definition')['ownerKey']='chain-wrong'
            else: bindings.pop()
            with self.assertRaises(MappingError):compare_views(self.seed,self.seed,bindings,self.schema)
    def test_causal_create_random_ids(self):
        empty=json.loads((SPEC/'r2-seeds/empty.json').read_text())
        registry=json.loads((SPEC/'SeedRegistry-R2.json').read_text())
        cmd=next(c for c in registry['cases'] if c['id']=='create-nil')['steps'][0]
        left=copy.deepcopy(empty);right=copy.deepcopy(empty)
        chain=copy.deepcopy(self.seed['snapshot']['chains'][0]);chain['noteEntries']=[]
        definition=copy.deepcopy(self.seed['snapshot']['definitions'][0]);definition['plannedSubtasks']=[]
        for env,cid,did in [(left,'AAAAAAAA-0000-4000-8000-000000000001','AAAAAAAA-0000-4000-8000-000000000002'),(right,cmd['chainGoID'],cmd['definitionGoID'])]:
            c=copy.deepcopy(chain);c['id']['rawValue']=cid
            d=copy.deepcopy(definition);d['id']['rawValue']=did;d['chainID']['rawValue']=cid
            env['snapshot']['chains']=[c];env['snapshot']['definitions']=[d]
        returned={'swift':{'chain':left['snapshot']['chains'][0]['id']['rawValue']},'go':{'chain':cmd['chainGoID']}}
        bindings=build_bindings(empty,left,right,cmd,returned)
        self.assertEqual(compare_views(left,right,bindings,self.schema)[0]['verdict'],'equal')
        self.assertEqual(compare_raw('raw-package-bytes',json.dumps(left).encode(),json.dumps(right).encode())['verdict'],'different')
        with self.assertRaises(MappingError):build_bindings(empty,left,right,cmd,{})
        extra=copy.deepcopy(left['snapshot']['definitions'][0]);extra['id']['rawValue']='AAAAAAAA-0000-4000-8000-000000000003'
        left['snapshot']['definitions'].append(extra)
        with self.assertRaises(MappingError):build_bindings(empty,left,right,cmd,returned)
    def test_raw_never_canonicalized(self):
        result=compare_raw('raw-package-bytes',b'{"a":1}',b'{ "a":1}')
        self.assertEqual(result['verdict'],'different');self.assertEqual(result['firstOffset'],1)
    def synthetic_reports(self):
        registry=json.loads((SPEC/'SeedRegistry-R2.json').read_text())
        cases=[]
        for c in registry['cases']:
            points=[]
            for key in ['seed']+[x['commandKey'] for x in c['steps']]:
                stages=[dict(side='controller',name='preflight',outcome='success',reason='none')]
                for side in ('swift','go'):
                    for name in STAGES:
                        na=side=='swift' and name in ('from-domain','cross-inspect') or (name=='command' if key=='seed' else name in ('seed-prepare','seed-decode','seed-core'))
                        stages.append(dict(side=side,name=name,outcome='not-applicable' if na else 'success',reason='not-applicable' if na else 'none'))
                observations=[dict(layer=layer,artifact=dict(name=c['id']+'/'+key+'/'+layer+'.json',bytes=2,sha256='0'*64),rawState='schema-valid',reason='none') for layer in OBSERVATIONS]
                points.append(dict(key=key,stages=stages,observations=observations,bindings=[],comparisons=[comparison(layer,'equal') for layer in LAYERS]))
            cases.append(dict(caseID=c['id'],fixtureValidity='valid',points=points))
        return cases,registry,{'V%02d'%i:'pass' for i in range(1,10)},dict.fromkeys(M2_REQUIRED,'not-run')
    def test_unverified_green_claims_count_zero(self):
        result=summarize(*self.synthetic_reports())
        self.assertEqual(result['effectivePointCount'],0)
        self.assertFalse(result['observationAccepted']);self.assertFalse(result['interopAccepted'])
    def test_report_mutations_rejected(self):
        for mutation in ('missing-stage','missing-stage-reason','duplicate-comparison','missing-observation-field','artifact-path'):
            cases,r,g,m=self.synthetic_reports();p=cases[0]['points'][0]
            if mutation=='missing-stage':p['stages'].pop()
            elif mutation=='missing-stage-reason':del p['stages'][0]['reason']
            elif mutation=='duplicate-comparison':p['comparisons'][1]=copy.deepcopy(p['comparisons'][0])
            elif mutation=='missing-observation-field':del p['observations'][0]['reason']
            else:p['observations'][0]['artifact']['name']='../escape'
            with self.assertRaises(ReportError,msg=mutation):summarize(cases,r,g,m)
    def test_invalid_fixture_unsupported_notrun_never_effective(self):
        for status in ('invalid-fixture','unsupported','harness-error','pending'):
            cases,r,g,m=self.synthetic_reports()
            for c in cases:c['fixtureValidity']=status
            all_points={(c['caseID'],p['key']) for c in cases for p in c['points']}
            result=summarize(cases,r,g,m,verified_points=all_points)
            self.assertEqual(result['effectivePointCount'],0)
        cases,r,g,m=self.synthetic_reports()
        for c in cases:
            for p in c['points']:p['comparisons'][0]['verdict']='not-run'
        result=summarize(cases,r,g,m,verified_points={(c['caseID'],p['key']) for c in cases for p in c['points']})
        self.assertEqual(result['effectivePointCount'],0)
    def test_terminal_go_inspect_rejection_counts_observation_only(self):
        cases,r,g,m=self.synthetic_reports();p=cases[0]['points'][0]
        inspect=next(s for s in p['stages'] if s['side']=='go' and s['name']=='cross-inspect')
        decode=next(s for s in p['stages'] if s['side']=='go' and s['name']=='cross-decode')
        cross=next(c for c in p['comparisons'] if c['layer']=='cross-decode')
        inspect.update(outcome='rejected',reason='codec-rejected')
        decode.update(outcome='not-run',reason='dependency-failed');cross.update(verdict='rejected',reason='codec-rejected')
        verified={(cases[0]['caseID'],p['key'])}
        result=summarize(cases,r,g,m,verified_points=verified)
        self.assertEqual(result['effectivePointCount'],1);self.assertEqual(result['interopPointCount'],0)
        self.assertFalse(result['interopAccepted'])
        for which,field,value in [('inspect','outcome','not-run'),('inspect','outcome','unsupported'),('inspect','reason','profile-violation'),('decode','reason','none'),('comparison','verdict','equal')]:
            changed=copy.deepcopy(cases);q=changed[0]['points'][0]
            target=next(c for c in q['comparisons'] if c['layer']=='cross-decode') if which=='comparison' else next(s for s in q['stages'] if s['side']=='go' and s['name']==('cross-inspect' if which=='inspect' else 'cross-decode'))
            target[field]=value
            self.assertEqual(summarize(changed,r,g,m,verified_points=verified)['effectivePointCount'],0,(which,field,value))
        inspect['reason']='profile-violation';decode.update(outcome='rejected',reason='profile-violation')
        self.assertEqual(summarize(cases,r,g,m,verified_points=verified)['effectivePointCount'],0)
    def test_arbitrary_cross_decode_notrun_never_effective(self):
        for side in ('swift','go'):
            cases,r,g,m=self.synthetic_reports();p=cases[0]['points'][0]
            next(s for s in p['stages'] if s['side']==side and s['name']=='cross-decode').update(outcome='not-run',reason='dependency-failed')
            next(c for c in p['comparisons'] if c['layer']=='cross-decode')['verdict']='rejected'
            self.assertEqual(summarize(cases,r,g,m,verified_points={(cases[0]['caseID'],p['key'])})['effectivePointCount'],0)
    def test_incomplete_reports_cannot_pass(self):
        registry=json.loads((SPEC/'SeedRegistry-R2.json').read_text())
        with self.assertRaises(ReportError):summarize([],registry,{}, {})

if __name__=='__main__':unittest.main()
