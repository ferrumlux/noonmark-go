# SPDX-License-Identifier: AGPL-3.0-only
"""Executed V02/V03/V04/V05/V08 gates over authenticated real observations.

Unit tests may exercise this code with synthetic inputs, but the runtime caller
must supply its actual three completed rounds. No additional business points are
created or counted. Every named pass is produced only after its assertion ran.
"""
import copy
import json
from bounded import load_json, validate_envelope, validate_capture, sha, Rejected
from comparator import (build_bindings, compare_views, compare_raw, summarize,
                        MappingError, ReportError, M2_REQUIRED, OBSERVATIONS)


def _require(ok):
 if not ok:raise ReportError('negative-gate-failed')
def _json(value):return json.dumps(value,ensure_ascii=False,separators=(',',':')).encode()
def _reject(call):
 try:call()
 except (MappingError,ReportError,Rejected,KeyError,ValueError,OSError):return
 raise ReportError('expected-rejection')
def verify_artifact(claim,data):
 _require(type(data) is bytes and len(data)==claim['bytes'] and sha(data)==claim['sha256'])
 return data

def verify_raw_claim(claim,left,right):
 expected=compare_raw(claim['layer'],left,right)
 _require(claim==expected)
 return expected

def validate_cross_receipt(receiver,raw,result):
 """Inspect rejection is terminal in Go; never force Decode past that guard."""
 _require(receiver in ('swift','go'))
 _require(type(result) is dict and type(result.get('accepted')) is bool and result.get('inputSHA256')==sha(raw) and result.get('preflightSucceeded') is True)
 if result.get('rejectedAt')=='inspect':
  _require(receiver=='go' and result['accepted'] is False and result.get('inspectInvoked') is True and result.get('decoderInvoked') is False)
 else:
  _require(result.get('decoderInvoked') is True)
 return result

def raw_mutations(raw):
 """Only adversarial inputs are re-encoded. Original bytes remain untouched."""
 value=load_json(raw)
 _require(type(value) is dict and 'formatVersion' in value)
 escape=raw.replace(b'"formatVersion"',b'"format\\u0056ersion"',1)
 _require(escape!=raw)
 key_order=_json(dict(reversed(tuple(value.items()))))
 if key_order==raw:key_order=b' '+key_order
 changed=copy.deepcopy(value);s=changed['snapshot'];array=None
 for section in ('traces','subtasks'):
  if len(s.get(section,[]))>=2:array=s[section];break
 if array is None:
  for definition in s.get('definitions',[]):
   if len(definition['plannedSubtasks'])>=2:array=definition['plannedSubtasks'];break
 _require(array is not None);array.reverse()
 return {'whitespace':raw+b' ','escape':escape,'key-order':key_order,'array-order':_json(changed)}


def execute_comparison_gates(spec,round_cases,artifact_reader,*,cross_decode=None):
 """Return rows, aggregate gates, and independent actual decoder receipts.

 artifact_reader(round_index,caseID,pointKey,layer) returns archived raw bytes.
 cross_decode(receiver,raw,check) must execute the real isolated decoder on raw,
 account/archive all evidence, and return accepted/inputSHA256/preflightSucceeded/
 decoderInvoked. A terminal Go Inspect rejection instead requires accepted=False,
 inspectInvoked=True, decoderInvoked=False, rejectedAt='inspect'. Swift cannot use
 this exception. Absence of runtime evidence fails its gate, never skips it green.
 """
 rows=[];receipts=[];groups={g:[] for g in ('V02','V03','V04','V05','V08')}
 def check(group,name,call):
  import gate_receipts as gr,time
  start=time.monotonic_ns();outcome='pass'
  try:call()
  except (MappingError,ReportError,Rejected,KeyError,ValueError,TypeError,IndexError,StopIteration,OSError):outcome='fail'
  ident=group.lower()+'-'+name
  gr.record_result(ident,outcome,start,time.monotonic_ns())
  rows.append({'check':ident,'outcome':outcome});groups[group].append(outcome)
 def point(round_index,case_id,key):
  c=next(c for c in round_cases[round_index] if c['caseID']==case_id)
  _require(c['fixtureValidity']=='valid')
  return c,next(p for p in c['points'] if p['key']==key)
 def raw(round_index,c,p,layer):
  observation=next(o for o in p['observations'] if o['layer']==layer)
  _require(observation['rawState']=='schema-valid')
  data=artifact_reader(round_index,c['caseID'],p['key'],layer)
  verify_artifact(observation['artifact'],data)
  validate_envelope(load_json(data),spec['schema'],spec['preferences'],export=layer.endswith('-export'))
  return data
 def envelope(case_id='observe-multi-interleaved',key='seed',side='swift'):
  c,p=point(0,case_id,key);layer=side+('-engine-projection' if side=='swift' else '-kernel-projection')
  return load_json(raw(0,c,p,layer))
 def baseline():
  v=envelope();return v,build_bindings(spec['seeds']['multi-interleaved'],v,v)
 def binding_mutation(name):
  value,bindings=baseline();changed=copy.deepcopy(bindings)
  if name=='duplicate-key':changed[1]['key']=changed[0]['key']
  elif name=='duplicate-id':
   target=next(b for b in changed if b['kind']=='trace');other=next(b for b in changed if b['kind']=='trace' and b is not target);other['swiftID']=target['swiftID']
  elif name=='wrong-owner':next(b for b in changed if b['kind']=='definition')['ownerKey']='chain-wrong'
  elif name=='missing-binding':changed.pop()
  _reject(lambda:compare_views(value,value,changed,spec['schema']))
 for name in ('duplicate-key','duplicate-id','wrong-owner','missing-binding'):
  check('V02',name,lambda n=name:binding_mutation(n))
 def entity_mutation(name):
  value,_=baseline();changed=copy.deepcopy(value)
  if name=='missing-trace-lineage':changed['snapshot']['subtasks'][0]['lineageID']['rawValue']='FFFFFFFF-0000-4000-8000-000000000001'
  elif name=='extra-entity':
   obj=copy.deepcopy(changed['snapshot']['chains'][0]);obj['id']['rawValue']='FFFFFFFF-0000-4000-8000-000000000002';changed['snapshot']['chains'].append(obj)
  elif name=='duplicate-entity':changed['snapshot']['traces'].append(copy.deepcopy(changed['snapshot']['traces'][0]))
  _reject(lambda:build_bindings(spec['seeds']['multi-interleaved'],value,changed))
 for name in ('missing-trace-lineage','extra-entity','duplicate-entity'):check('V02',name,lambda n=name:entity_mutation(n))
 def two_definitions():
  v=envelope('create-nil','create-one');changed=copy.deepcopy(v)
  extra=copy.deepcopy(changed['snapshot']['definitions'][0]);extra['id']['rawValue']='FFFFFFFF-0000-4000-8000-000000000003';changed['snapshot']['definitions'].append(extra)
  cmd=next(c for c in spec['registry']['cases'] if c['id']=='create-nil')['steps'][0]
  returned={'swift':{'chain':v['snapshot']['chains'][0]['id']['rawValue']},'go':{'chain':v['snapshot']['chains'][0]['id']['rawValue']}}
  _reject(lambda:build_bindings(spec['seeds']['empty'],v,changed,cmd,returned))
 check('V02','two-definition-candidates',two_definitions)
 def permutation(section):
  v,b=baseline();changed=copy.deepcopy(v)
  a=changed['snapshot']['definitions'][0]['plannedSubtasks'] if section=='planned' else changed['snapshot'][section]
  _require(len(a)>=2);a.reverse()
  e,seq=compare_views(v,changed,b,spec['schema']);_require(e['verdict']=='equal' and seq['verdict']=='different')
 for name in ('traces','subtasks','planned'):check('V03','permutation-'+name,lambda n=name:permutation(n))
 def field_mutation(name):
  v,b=baseline();changed=copy.deepcopy(v);s=changed['snapshot'];obj=s['subtasks'][0]
  if name=='position':obj['position']+=1
  elif name=='owner':s['definitions'][0]['chainID']=copy.deepcopy(s['chains'][1]['id'])
  elif name=='lineage':
   owner=obj['traceID']['rawValue'];trace=next(t for t in s['traces'] if t['id']['rawValue']==owner)
   definition=next(d for d in s['definitions'] if d['id']==trace['definitionID'])
   obj['lineageID']=copy.deepcopy(next(p['lineageID'] for p in definition['plannedSubtasks'] if p['lineageID']!=obj['lineageID']))
  elif name=='clock':obj['createdAt']+=1
  elif name=='optional':s['definitions'][0]['descriptionText']=''
  elif name=='utf8':obj['title']+='e\u0301'
  _require(compare_views(v,changed,b,spec['schema'])[0]['verdict']=='different')
 for name in ('position','owner','lineage','clock','optional','utf8'):check('V03',name,lambda n=name:field_mutation(n))
 for sender,receiver in (('swift','go'),('go','swift')):
  for mutation in ('whitespace','escape','key-order','array-order'):
   def cross(sender=sender,receiver=receiver,mutation=mutation):
    c,p=point(0,'observe-multi-interleaved','seed');original=raw(0,c,p,sender+'-package');changed=raw_mutations(original)[mutation]
    _require(compare_raw('raw-package-bytes',original,changed)['verdict']=='different')
    validate_envelope(load_json(changed),spec['schema'],spec['preferences'])
    _require(cross_decode is not None)
    name='v04-'+sender+'-'+mutation
    result=cross_decode(receiver,changed,name)
    validate_cross_receipt(receiver,changed,result)
    receipts.append({'check':name,'receiver':receiver,'accepted':result['accepted'],'inputSHA256':sha(changed),'inspectInvoked':result.get('inspectInvoked',False),'decoderInvoked':result['decoderInvoked'],'rejectedAt':result.get('rejectedAt','')})
   check('V04',sender+'-'+mutation,cross)
 all_gates={f'V{i:02}':'pass' for i in range(1,10)};m2=dict.fromkeys(M2_REQUIRED,'not-run')
 def summary(cases,verified=()):return summarize(cases,spec['registry'],all_gates,m2,verified_points=verified)
 def all_verified(cases):return {(c['caseID'],p['key']) for c in cases for p in c['points']}
 def structural_mutation(name):
  cases=copy.deepcopy(round_cases[0]);c=next(c for c in cases if c['caseID']=='observe-multi-interleaved');p=c['points'][0]
  if name=='missing-case':cases.pop()
  elif name=='missing-point':c['points'].pop()
  elif name=='missing-stage':p['stages'].pop()
  elif name=='missing-binding':
   p['bindings'].pop();v=envelope();return _reject(lambda:compare_views(v,v,p['bindings'],spec['schema']))
  elif name=='missing-observation':p['observations'].pop()
  elif name=='missing-comparison':p['comparisons'].pop()
  elif name=='duplicate-comparison':p['comparisons'][1]=copy.deepcopy(p['comparisons'][0])
  elif name=='unknown-case':c['caseID']='unknown-case'
  elif name=='unknown-stage':p['stages'][0]['name']='unknown-stage'
  elif name=='unknown-layer':p['comparisons'][0]['layer']='unknown-layer'
  if name=='missing-observation':
   _require(summary(cases,all_verified(cases))['effectivePointCount']<32)
  else:_reject(lambda:summary(cases,all_verified(cases)))
 for name in ('missing-case','missing-point','missing-stage','missing-binding','missing-observation','missing-comparison','duplicate-comparison','unknown-case','unknown-stage','unknown-layer'):
  check('V05',name,lambda n=name:structural_mutation(n))
 def remove_fields(section):
  c,p=point(0,'observe-multi-interleaved','seed')
  base=c if section=='case' else p if section=='point' else p[section][0]
  for field in tuple(base):
   changed=copy.deepcopy(c)
   target=changed if section=='case' else changed['points'][0] if section=='point' else changed['points'][0][section][0]
   del target[field]
   _reject(lambda:validate_capture(_json(changed)))
  _require(bool(base))
 for name in ('case','point','stages','bindings','observations','comparisons'):check('V05','missing-fields-'+name,lambda n=name:remove_fields(n))
 def artifact_mutation(name):
  c,p=point(0,'observe-multi-interleaved','seed');data=raw(0,c,p,'swift-package')
  claim=copy.deepcopy(next(o for o in p['observations'] if o['layer']=='swift-package')['artifact'])
  if name=='hash':claim['sha256']='0'*64
  elif name=='bytes':claim['bytes']+=1
  _reject(lambda:verify_artifact(claim,data))
 for name in ('hash','bytes'):check('V05','artifact-'+name,lambda n=name:artifact_mutation(n))
 def missing_artifact():
  c,p=point(0,'observe-multi-interleaved','seed');changed=copy.deepcopy(p)
  changed['observations']=[o for o in p['observations'] if o['layer']!='swift-package']
  try:raw(0,c,changed,'swift-package')
  except StopIteration:return
  raise ReportError('missing-artifact-accepted')
 check('V05','missing-artifact',missing_artifact)
 def unverified():
  result=summary(round_cases[0]);_require(result['effectivePointCount']==result['interopPointCount']==0 and not result['observationAccepted'])
 check('V05','negative-not-effective',unverified)
 def invalid_state(state):
  cases=copy.deepcopy(round_cases[0])
  for c in cases:c['fixtureValidity']=state
  result=summary(cases,all_verified(cases));_require(result['effectivePointCount']==result['interopPointCount']==0 and not result['observationAccepted'])
 for state in ('unsupported','invalid-fixture','harness-error','pending'):check('V05',state+'-not-effective',lambda s=state:invalid_state(s))
 def repetitions():
  _require(len(round_cases)==3)
  expected={c['id'] for c in spec['registry']['cases']}
  for ri,cases in enumerate(round_cases):
   _require(len(cases)==18 and {c['caseID'] for c in cases}==expected)
   points_seen=0
   for c in cases:
    _require(c['fixtureValidity']=='valid');validate_capture(_json(c))
    for p in c['points']:
     points_seen+=1
     for side in ('swift','go'):
      for stage in ('snapshot','encode-package','encode-export','self-decode','self-encode'):
       _require(next(s for s in p['stages'] if s['side']==side and s['name']==stage)['outcome']=='success')
     values={layer:load_json(raw(ri,c,p,layer)) for layer in OBSERVATIONS}
     registered=next(item for item in spec['registry']['cases'] if item['id']==c['caseID'])
     command=registered['steps'][0] if p['key']!='seed' else None
     returned={side:{b['kind']:b[side+'ID'] for b in p['bindings'] if b['rule'] in ('returned-chain','returned-trace')} for side in ('swift','go')}
     derived=build_bindings(spec['seeds'][registered['seed']],values['swift-engine-projection'],values['go-kernel-projection'],command,returned)
     _require({b['key']:b for b in derived}=={b['key']:b for b in p['bindings']})
     for side,layer in (('swift','swift-engine-projection'),('go','go-kernel-projection')):
      compare_views(values[layer],values[layer],p['bindings'],spec['schema'],side,side)
     for suffix,layer in (('package','raw-package-bytes'),('export','raw-export-bytes')):
      claim=next(x for x in p['comparisons'] if x['layer']==layer)
      verify_raw_claim(claim,raw(ri,c,p,'swift-'+suffix),raw(ri,c,p,'go-'+suffix))
   _require(points_seen==32)
  _require(summary(round_cases[0],all_verified(round_cases[0]))['effectivePointCount']<=32)
 check('V08','three-real-rounds',repetitions)
 def forged_equal():
  c,p=point(0,'observe-multi-interleaved','seed');a=raw(0,c,p,'swift-package');b=a+b' '
  claim=compare_raw('raw-package-bytes',a,b);claim['verdict']='equal';claim['rightSHA256']=claim['leftSHA256']
  _reject(lambda:verify_raw_claim(claim,a,b))
 check('V08','forged-equal-hash',forged_equal)
 return {'rows':rows,'gates':{g:'pass' if outcomes and all(v=='pass' for v in outcomes) else 'fail' for g,outcomes in groups.items()},'crossDecodeRecords':receipts}
