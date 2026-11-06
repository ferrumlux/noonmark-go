# SPDX-License-Identifier: AGPL-3.0-only
"""Downloaded evidence verifier. Replays receipts, never executes Swift or Go.

The replay is a deterministic recomputation, not another observation. Every
return code and byte comes from an authenticated reference in the original run.
"""
from pathlib import Path
import json,tempfile
from unittest.mock import patch
from bounded import need,Rejected,Limits,load_json,read_file,no_symlink,sha,validate_capture,validate_envelope
from process import validate_exit,execution_witness,VerifiedReplay


def reference(root,ref,limit=262144):
 need(type(ref) is dict and set(ref)=={'name','bytes','sha256'},'missing-evidence')
 name=ref['name'];need(type(name) is str and not Path(name).is_absolute() and '..' not in Path(name).parts and len(name)<=160,'missing-evidence')
 data=read_file(no_symlink(root/name),limit)
 need(type(ref['bytes']) is int and len(data)==ref['bytes'] and sha(data)==ref['sha256'],'missing-evidence');return data

def read_execution_index(root):
 from gate_receipts import binding_digest
 index=load_json(read_file(root/'execution-receipts.json',131072),Limits(bytes=131072,depth=8,nodes=8192,string=256,array=1024))
 need(set(index)=={'schemaVersion','bindingSHA256','receiptParts','directionParts'} and index['schemaVersion']==3 and index['bindingSHA256']==binding_digest(root),'missing-evidence')
 expanded={}
 for field,group,maximum in (('receiptParts','receipts',3),('directionParts','directions',2)):
  need(type(index[field]) is list and len(index[field])<=maximum,'missing-evidence');items=[]
  for i,ref in enumerate(index[field]):
   need(ref['name']=='execution-'+group+'-part-'+str(i)+'.json','missing-evidence')
   part=load_json(reference(root,ref,131072),Limits(bytes=131072,depth=8,nodes=8192,string=256,array=128))
   need(set(part)=={'schemaVersion','bindingSHA256','items'} and part['schemaVersion']==3 and part['bindingSHA256']==index['bindingSHA256'] and 1<=len(part['items'])<=128,'missing-evidence');items.extend(part['items'])
  expanded[group]=items
 return expanded

def read_receipts(root,spec):
 index=read_execution_index(root)
 by_id={};by_output={}
 case_map={c['id']:c for c in spec['registry']['cases']}
 for ref in index['receipts']:
  r=load_json(reference(root,ref,131072),Limits(bytes=131072,depth=12,nodes=8192,string=256,array=64))
  need(set(r)=={'schemaVersion','bindingSHA256','resourceWitness','requestedTimeoutNs','artifactManifests','id','mode','side','metadata','inputs','returncode','transportError','argv','executionWitness','outcome','reason','stdout','stderr','stderrBytes','artifacts','schemaValidWitness'},'schema-invalid')
  from gate_receipts import binding_digest
  need(r['bindingSHA256']==binding_digest(root),'missing-evidence')
  ident=r['id'];need(ref['name']=='receipts/'+ident+'.json' and ident not in by_id and r['schemaVersion']==3 and r['side'] in ('swift','go'),'missing-evidence')
  m=r['metadata'];side=r['side'];mode=r['mode']
  if mode in ('case','cross'):
   round_no=m.get('round');case=m.get('caseID');need(type(round_no) is int and 1<=round_no<=3 and case in case_map,'missing-evidence')
   base=f'round-{round_no}/work-{case}/'
   if mode=='case':
    need(set(m)=={'round','caseID'} and ident==f'r{round_no}-{case}-{side}-case','missing-evidence');output=base+side
    need(len(r['inputs'])==1 and r['inputs'][0]['name']==base+'case.json','missing-evidence')
    need(load_json(reference(root,r['inputs'][0]))==case_map[case],'registry-mismatch')
    allowed={'result.json'}|{key+'/'+layer+'.json' for key in ['seed']+[c['commandKey'] for c in case_map[case]['steps']] for layer in ('swift-engine-projection','swift-package','swift-export','swift-self-reencoded','go-kernel-projection','go-from-domain-projection','go-package','go-export','go-self-reencoded') if layer.startswith(side+'-')}
   else:
    point=m.get('pointKey');need(set(m)=={'round','caseID','pointKey'} and point in ['seed']+[c['commandKey'] for c in case_map[case]['steps']] and ident==f'r{round_no}-{case}-{point}-{side}-cross','missing-evidence');output=base+point+'-cross-'+side
    sender='go' if side=='swift' else 'swift';need(len(r['inputs'])==1 and r['inputs'][0]['name']==f'round-{round_no}/{case}/{point}/{sender}-package.json','missing-evidence');allowed={'result.json','decoded.json'}
  elif mode=='mutation':
   from gates import MUTATIONS
   mutation=m.get('mutationID');need(set(m)=={'mutationID'} and mutation in (*MUTATIONS,'control-valid') and ident=='mutation-'+side+'-'+mutation,'missing-evidence');output=ident;allowed={'result.json'}
  elif mode=='ingress':
   need(set(m)=={'faultMode'} and m['faultMode'] in ('normal','bypass','late') and ident=='ingress-'+side+'-'+m['faultMode'] and not r['inputs'],'missing-evidence');output=ident
   from process import ingress_paths
   allowed=set(ingress_paths(side))
  elif mode=='cross-negative':
   check=m.get('check');sender='go' if side=='swift' else 'swift';need(set(m)=={'check'} and check in {'v04-'+sender+'-'+x for x in ('whitespace','escape','key-order','array-order')} and ident=='cross-negative-'+side+'-'+check,'missing-evidence');output=ident;allowed={'result.json','decoded.json'}
   need(len(r['inputs'])==1 and r['inputs'][0]['name']==ident+'.json','missing-evidence')
  else:raise Rejected('schema-invalid')
  for item in r['inputs']:reference(root,item)
  reference(root,r['stdout'],131072);reference(root,r['stderr'],65536)
  need(r['stdout']['name']=='receipts/'+ident+'-stdout.bin' and r['stderr']['name']=='receipts/'+ident+'-stderr.bin','missing-evidence')
  need(type(r['artifactManifests']) is list and len(r['artifactManifests'])<=7 and (mode=='ingress' or not r['artifactManifests']),'missing-evidence')
  expanded=list(r['artifacts'])
  for i,ref in enumerate(r['artifactManifests']):
   need(ref['name']=='receipts/'+ident+'-artifacts-'+str(i)+'.json','missing-evidence')
   shard=load_json(reference(root,ref,131072),Limits(bytes=131072,depth=8,nodes=4096,string=256,strings=131072,array=256))
   need(set(shard)=={'schemaVersion','files'} and shard['schemaVersion']==2 and type(shard['files']) is list and 1<=len(shard['files'])<=256,'schema-invalid');expanded.extend(shard['files'])
  r['artifacts']=expanded
  artifacts={a['name']:a for a in r['artifacts']};need(len(artifacts)==len(r['artifacts']),'missing-evidence')
  for name,a in artifacts.items():need(name.startswith(output+'/') and name[len(output)+1:] in allowed,'missing-evidence');reference(root,a)
  protocol=None;outcome='harness-error'
  if output+'/result.json' in artifacts:
   try:
    protocol=load_json(reference(root,artifacts[output+'/result.json'],131072),Limits(bytes=131072,depth=16,nodes=16384,string=4096,strings=131072,array=512))
    need(not r['transportError'],'missing-evidence')
    w=r['executionWitness'];need(type(w) is dict and set(w)=={'pid','startedNs','endedNs','argvSHA256'} and type(w['pid']) is int and w['pid']>0 and type(w['startedNs']) is int and type(w['endedNs']) is int and 0<w['startedNs']<=w['endedNs'],'missing-evidence')
    need(w['argvSHA256']==execution_witness(r['argv'],w['pid'],w['startedNs'],w['endedNs'])['argvSHA256'],'missing-evidence')
    rw=r['resourceWitness'];requested=r['requestedTimeoutNs']
    need(type(requested) is int and 0<requested<=120000000000 and type(rw) is dict and set(rw)=={'stdoutBytes','stderrBytes','timeoutNs','deadlineNs','terminationNs','terminationReason'} and all(type(rw[k]) is int for k in ('stdoutBytes','stderrBytes','timeoutNs','deadlineNs','terminationNs')),'missing-evidence')
    need(rw['timeoutNs']==requested and rw['deadlineNs']==w['startedNs']+requested and rw['stdoutBytes']==r['stdout']['bytes'] and rw['stderrBytes']==r['stderrBytes'] and rw['terminationReason']=='normal' and rw['terminationNs']==0,'missing-evidence')
    outcome=validate_exit(mode,side,r['returncode'],protocol)
    result_raw=reference(root,artifacts[output+'/result.json'],131072)
    need(reference(root,r['stdout'],131072)==(result_raw+b'\n' if side=='go' else b''),'missing-evidence')
    if mode=='mutation':need(protocol['id']==m['mutationID'],'missing-evidence')
    if mode=='ingress':need(protocol['mode']==m['faultMode'],'missing-evidence')
   except Rejected:pass
  need(r['outcome']==outcome and r['schemaValidWitness']==(outcome=='success' and protocol is not None),'missing-evidence')
  if outcome=='success' and mode in ('cross','cross-negative'):need(output+'/decoded.json' in artifacts,'missing-evidence')
  need(output not in by_output,'missing-evidence');by_output[output]=(r,protocol);by_id[ident]=(r,protocol)
 directions={}
 for ref in index['directions']:
  direction=load_json(reference(root,ref,131072),Limits(bytes=131072,depth=12,nodes=2048,string=256,array=64))
  need(set(direction)=={'schemaVersion','bindingSHA256','id','processReceiptSHA256','input','decoded','verdict','comparisons'} and direction['schemaVersion']==3 and direction['bindingSHA256']==binding_digest(root),'schema-invalid')
  ident=direction['id'];need(ident in by_id and ident not in directions and by_id[ident][0]['mode']=='cross' and ref['name']=='receipts/'+ident+'-direction.json','missing-evidence')
  process_ref=next(x for x in index['receipts'] if x['name']=='receipts/'+ident+'.json')
  need(direction['processReceiptSHA256']==process_ref['sha256'] and direction['input']==by_id[ident][0]['inputs'][0],'missing-evidence')
  reference(root,direction['input'])
  if direction['decoded'] is not None:reference(root,direction['decoded'])
  directions[ident]=direction
 need(set(directions)=={i for i,(r,p) in by_id.items() if r['mode']=='cross'},'missing-evidence')
 return by_id,by_output


def verify_run(root,spec):
 """Recompute all 3 rounds from process receipts; validate recorded gate evidence."""
 import controller,runner
 runner.validate_tool_manifest(runner.HERE/'tool-manifest.json')
 from comparator import summarize,M2_REQUIRED
 root=no_symlink(root);by_id,by_output=read_receipts(root,spec)
 context=load_json(read_file(root/'run-context.json',131072),Limits(bytes=131072,depth=8,nodes=4096,string=256,array=64))
 need(set(context)=={'schemaVersion','toolchain','scope'} and context['schemaVersion']==2,'schema-invalid')
 guard=read_file(root/'guard-evidence.json',65536);controller.validate_scope(context['scope'],guard)
 with tempfile.TemporaryDirectory(prefix='offline-recompute-',dir=root.parent) as temp:
  target=Path(temp)/'run';target.mkdir()
  (target/'run-binding.json').write_bytes(read_file(root/'run-binding.json',4096))
  for name in ('source-gates.json','compile.sb','runtime.sb','go-compile.sb'):
   original=root.parent/name
   if original.exists():(target.parent/name).write_bytes(read_file(original,131072))
  source_inputs=root.parent/'gate-source'
  if source_inputs.exists():
   (target.parent/'gate-source').mkdir()
   for original in source_inputs.iterdir():(target.parent/'gate-source'/original.name).write_bytes(read_file(original,131072))
  def replay(argv,**kwargs):
   destination=Path(argv[argv.index('--output')+1]);name=str(destination.relative_to(target))
   need(name in by_output,'missing-evidence');receipt,protocol=by_output[name]
   if '--cross' in argv:
    incoming=read_file(Path(argv[argv.index('--cross')+1]),262144)
    need(len(receipt['inputs'])==1 and sha(incoming)==receipt['inputs'][0]['sha256'],'missing-evidence')
   destination.mkdir()
   for ref in receipt['artifacts']:
    suffix=ref['name'][len(name)+1:];path=destination/suffix;path.parent.mkdir(parents=True,exist_ok=True)
    with open(path,'xb') as f:f.write(reference(root,ref))
   return VerifiedReplay({'resourceWitness':receipt['resourceWitness'],'originalRequestedTimeoutNs':receipt['requestedTimeoutNs'],'executionWitness':receipt['executionWitness'],'originalArgv':receipt['argv'],'returncode':receipt['returncode'],'transportError':receipt['transportError'],'stdout':reference(root,receipt['stdout'],131072),'stderrTail':reference(root,receipt['stderr'],65536),'stderrBytes':receipt['stderrBytes']})
  with patch.object(controller,'execute',replay):
   recomputed=controller.run_suite(['recorded-swift'],['recorded-go'],target,context['toolchain'],context['scope'],guard_raw=guard,run_negative_gates=False)
  rounds=[];verified=set()
  for round_no in range(1,4):
   cases=[]
   for definition in spec['registry']['cases']:
    name=f'round-{round_no}/{definition["id"]}.capture.json'
    actual=validate_capture(read_file(root/name,131072));expected=validate_capture(read_file(target/name,131072))
    need(actual==expected,'missing-evidence');cases.append(expected)
    if round_no==1:
     for p in expected['points']:
      if expected['fixtureValidity']=='valid' and all(c['verdict'] in ('equal','different','rejected') for c in p['comparisons']):verified.add((definition['id'],p['key']))
   rounds.append(cases)
  old_index=read_execution_index(root)
  new_index=read_execution_index(target)
  # Process receipt hashes can differ in the replay's metadata, so compare the
  # direction payload except that digest after validating the original link.
  def direction_payload(where,ref):
   d=load_json(reference(where,ref,131072),Limits(bytes=131072,depth=12,nodes=2048,string=256,array=64));d.pop('processReceiptSHA256');return d
  expected_dirs={r['name']:direction_payload(target,r) for r in new_index['directions']}
  need({r['name']:direction_payload(root,r) for r in old_index['directions']}==expected_dirs,'missing-evidence')
  from gates import verify_recorded_gates
  gates=verify_recorded_gates(root,spec,rounds,by_id)
  result=summarize(rounds[0],spec['registry'],gates,dict.fromkeys(M2_REQUIRED,'not-run'),verified_points=verified)
  return {'schemaVersion':2,'summary':result,'negativeGates':gates,'receiptCount':len(by_id)}

if __name__=='__main__':
 import sys,json
 from bounded import load_spec
 try:
  need(len(sys.argv)==2,'schema-invalid')
  result=verify_run(Path(sys.argv[1]),load_spec(Path(__file__).resolve().parent/'spec'))
  print(json.dumps(result,sort_keys=True))
  from runner import observation_ready
  sys.exit(0 if observation_ready(result) else 1)
 except (Rejected,OSError,ValueError,KeyError,TypeError) as error:
  print('offline verification failed: '+type(error).__name__,file=sys.stderr);sys.exit(1)
