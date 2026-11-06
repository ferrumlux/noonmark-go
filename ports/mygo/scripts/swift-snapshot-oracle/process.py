# SPDX-License-Identifier: AGPL-3.0-only
"""Bounded pipe consumer. Never subprocess.communicate/capture_output on a probe."""
import os, selectors, signal, subprocess, time
from bounded import Rejected, operation

def execute(argv, *, timeout=120, cwd=None, env=None, stdin=b''):
 if len(stdin)>1024:raise Rejected('resource-limit')
 operation('process')
 started=time.monotonic_ns()
 p=subprocess.Popen(argv,cwd=cwd,env=env,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,start_new_session=True)
 sel=selectors.DefaultSelector();out=bytearray();tail=bytearray();stderr_read=0;stdout_read=0;timeout_ns=int(timeout*1000000000);deadline_ns=started+timeout_ns;deadline=deadline_ns/1000000000;stop_reason='normal';termination_ns=0
 try:
  p.stdin.write(stdin);p.stdin.close()
  for stream in (p.stdout,p.stderr):os.set_blocking(stream.fileno(),False);sel.register(stream,selectors.EVENT_READ)
  while sel.get_map():
   remain=deadline-time.monotonic()
   if remain<=0:stop_reason='timeout';raise Rejected('timeout')
   for key,_ in sel.select(min(.1,remain)):
    data=os.read(key.fd,8192)
    if not data:sel.unregister(key.fileobj);continue
    if key.fileobj is p.stdout:
     stdout_read+=len(data)
     if len(out)+len(data)>131072:stop_reason='stdout-limit';raise Rejected('resource-limit')
     out.extend(data)
    else:
     stderr_read+=len(data)
     if stderr_read>1048576:stop_reason='stderr-limit';raise Rejected('resource-limit')
     tail.extend(data);del tail[:-65536]
  p.wait(timeout=max(.001,deadline-time.monotonic()))
  return {'returncode':p.returncode,'stdout':bytes(out),'stderrTail':bytes(tail),'stderrBytes':stderr_read,'executionWitness':execution_witness(argv,p.pid,started,time.monotonic_ns()),'resourceWitness':{'stdoutBytes':stdout_read,'stderrBytes':stderr_read,'timeoutNs':timeout_ns,'deadlineNs':deadline_ns,'terminationNs':termination_ns,'terminationReason':stop_reason}}
 except (Rejected,subprocess.TimeoutExpired) as error:
  termination_ns=time.monotonic_ns()
  if isinstance(error,subprocess.TimeoutExpired):stop_reason='timeout'
  try:os.killpg(p.pid,signal.SIGKILL)
  except ProcessLookupError:pass
  p.wait()
  failure=error if isinstance(error,Rejected) else Rejected('timeout')
  failure.process_result={'returncode':p.returncode,'stdout':bytes(out),'stderrTail':bytes(tail),'stderrBytes':stderr_read,'transportError':failure.reason,'executionWitness':execution_witness(argv,p.pid,started,time.monotonic_ns()),'resourceWitness':{'stdoutBytes':stdout_read,'stderrBytes':stderr_read,'timeoutNs':timeout_ns,'deadlineNs':deadline_ns,'terminationNs':termination_ns,'terminationReason':stop_reason}}
  raise failure
 finally:
  sel.close();p.stdout.close();p.stderr.close()


def execution_witness(argv,pid,started,ended):
 import json
 from bounded import sha
 return {'pid':pid,'startedNs':started,'endedNs':ended,'argvSHA256':sha(json.dumps(argv,separators=(',',':')).encode())}

class VerifiedReplay(dict):
 """Only offline's validated original receipt replay; never an OS execution."""
 pass


def validate_protocol(mode,side,value):
 from bounded import need
 import re
 need(type(value) is dict and type(value.get('schemaVersion')) is int and value['schemaVersion']==2,'schema-invalid')
 if mode=='case':
  need(set(value)=={'schemaVersion','caseID','fixtureValidity','points','interopAccepted'} and value['interopAccepted'] is False,'schema-invalid')
  need(type(value['caseID']) is str and re.fullmatch('[a-z][a-z0-9-]{0,63}',value['caseID']) and type(value['points']) is list and len(value['points'])<=2,'schema-invalid')
  names={'preflight','seed-prepare','seed-decode','seed-core','command','snapshot','from-domain','encode-package','encode-export','self-decode','self-encode','cross-preflight','cross-inspect','cross-decode'}
  keys=[]
  for point in value['points']:
   need(set(point)=={'key','returned','stages','artifacts','selfEqual'} and point['key'] in ('seed','create-one','schedule-one') and point['key'] not in keys and type(point['selfEqual']) is bool,'schema-invalid');keys.append(point['key'])
   need(type(point['stages']) is dict and set(point['stages'])==names and all(v in ('success','rejected','unsupported','not-run','harness-error','not-applicable') for v in point['stages'].values()),'schema-invalid')
   returned=point['returned'];need(type(returned) is dict and set(returned)<= {'chain','trace'} and all(type(v) is str and re.fullmatch('[0-9A-F]{8}(?:-[0-9A-F]{4}){3}-[0-9A-F]{12}',v) for v in returned.values()),'schema-invalid')
   need(type(point['artifacts']) is list and len(point['artifacts'])<=5,'schema-invalid')
   for a in point['artifacts']:
    need(set(a)=={'name','bytes','sha256','rawState'} and type(a['name']) is str and a['name'].startswith(point['key']+'/'+side+'-') and type(a['bytes']) is int and 0<=a['bytes']<=262144 and re.fullmatch('[0-9a-f]{64}',a['sha256']) and a['rawState'] in ('archived-unchecked','schema-valid','malformed-raw'),'schema-invalid')
 elif mode in ('cross','cross-negative'):
  base={'schemaVersion','accepted','requiresClassificationAudit','decodeCalls'}
  need(base<=set(value) and set(value)<=(base|{'status','inspectCalls','reason','decodedSHA256'} if side=='go' else base),'schema-invalid')
 elif mode=='mutation':
  need(set(value)=={'schemaVersion','id','accepted','reason','counters'} and type(value['id']) is str and re.fullmatch('[a-z0-9-]{1,80}',value['id']) and type(value['accepted']) is bool and type(value['reason']) is str and len(value['reason'].encode())<=256,'schema-invalid')
  need(type(value['counters']) is dict and set(value['counters'])=={'decode','clone','bridge','command','serializer'} and all(type(v) is int and v>=0 for v in value['counters'].values()),'schema-invalid')
 elif mode=='ingress':
  need(set(value)=={'schemaVersion','mode','pass','rows','expectedRows'} and value['mode'] in ('normal','bypass','late') and type(value['pass']) is bool and type(value['rows']) is list and type(value['expectedRows']) is list,'schema-invalid')
 else:raise Rejected('schema-invalid')
 return value


def validate_exit(mode,side,code,protocol):
 """Closed protocol/OS termination agreement. JSON cannot override a signal."""
 from bounded import need
 need(type(code) is int and code>=0,'missing-evidence')
 validate_protocol(mode,side,protocol)
 if mode=='case':
  validity=protocol.get('fixtureValidity')
  if validity=='valid':need(code==0,'missing-evidence');return 'success'
  need(validity in ('invalid-fixture','unsupported','harness-error') and code==(2 if side=='go' else 0),'missing-evidence')
  return 'controlled-rejection' if validity!='harness-error' else 'harness-error'
 if mode in ('cross','cross-negative'):
  accepted=protocol.get('accepted');need(type(accepted) is bool and protocol.get('requiresClassificationAudit') is False,'missing-evidence')
  decode=protocol.get('decodeCalls');need(type(decode) is int and decode in (0,1),'missing-evidence')
  if side=='go':
   inspect=protocol.get('inspectCalls');status=protocol.get('status')
   need(type(inspect) is int and inspect==1,'missing-evidence')
   if accepted:need(code==0 and decode==1 and status=='accepted','missing-evidence')
   else:need(code==2 and ((decode==0 and status=='inspect-rejected') or (decode==1 and status=='decode-rejected')),'missing-evidence')
  else:need(code==0 and decode==1,'missing-evidence')
  return 'success' if accepted else 'controlled-rejection'
 if mode=='ingress':
  need(code==0,'missing-evidence');return 'success'
 if mode=='mutation':
  need(code==0 and type(protocol.get('accepted')) is bool,'missing-evidence');return 'success'
 raise Rejected('schema-invalid')


def ingress_paths(side):
 """Fixed matrix-produced file paths, not a recursive upload permission."""
 from gates import PROBE_ENTRIES
 entries=PROBE_ENTRIES[side];paths=['result.json'];ordinal=0
 layers=([side+'-'+v for v in ('kernel-projection','from-domain-projection','package','export','self-reencoded')] if side=='go' else [side+'-'+v for v in ('engine-projection','package','export','self-reencoded')])
 for entry,dimensions in entries.items():
  if entry in ('create','schedule'):continue
  for dimension in dimensions:
   ordinal+=1;base='work-'+str(ordinal)+'/'
   if side=='swift':paths.extend([base+'case.json',base+'capture/result.json']);base+='capture/'
   paths.extend(base+'seed/'+layer+'.json' for layer in layers)
   if side=='swift' and entry=='cross':paths.extend('work-'+str(ordinal)+'/cross-result/'+n for n in ('result.json','decoded.json'))
 for op in ('create','schedule'):
  base='control-'+op+'/'
  if side=='swift':paths.extend([base+'case.json',base+'capture/result.json']);base+='capture/'
  paths.extend(base+key+'/'+layer+'.json' for key in ('seed',op+'-one') for layer in layers)
 return paths


class Recorder:
 """Persist raw process receipts before interpreting them; never reuses outputs.

 The process callback is the sole OS outcome authority. A successful JSON parse
 is only a schema witness and is bound to this receipt's successful execution.
 """
 def __init__(self,root,budget):
  from pathlib import Path
  from gate_receipts import binding_digest,ensure_run
  ensure_run(root)
  self.root=Path(root);self.binding=binding_digest(self.root);self.budget=budget;self.refs=[];self.directions=[];self.items={};(self.root/'receipts').mkdir()
 def _save(self,name,data):
  from bounded import need,no_symlink,sha
  need(type(data) is bytes and len(data)<=131072,'resource-limit')
  need(self.budget[0]+len(data)<=80*1024*1024,'resource-limit')
  path=no_symlink(self.root/name);path.parent.mkdir(parents=True,exist_ok=True)
  with open(path,'xb') as f:f.write(data)
  self.budget[0]+=len(data)
  return {'name':name,'bytes':len(data),'sha256':sha(data)}
 def reference(self,path,limit=262144):
  from bounded import read_file,sha,need
  data=read_file(path,limit);name=str(path.relative_to(self.root));self.budget[0]+=len(data);need(self.budget[0]<=80*1024*1024,'resource-limit')
  return {'name':name,'bytes':len(data),'sha256':sha(data)}
 def invoke(self,execute,ident,mode,side,argv,out,*,metadata=None,input_paths=(),point_keys=(),timeout=120,env=None):
  from bounded import need,load_json,read_file,Limits,LAYERS,Rejected
  import json,re
  need(re.fullmatch('[a-z0-9-]{1,120}',ident) and ident not in self.items,'schema-invalid')
  need(not out.exists() and not out.is_symlink(),'missing-evidence')
  inputs=[self.reference(p) for p in input_paths if p.is_relative_to(self.root)]
  outcome='harness-error';reason='missing-evidence';protocol=None
  try:proc=execute(argv,timeout=timeout,env=env)
  except Rejected as error:
   proc=getattr(error,'process_result',{'returncode':None,'stdout':b'','stderrTail':b'','stderrBytes':0});reason=error.reason
  stdout=self._save('receipts/'+ident+'-stdout.bin',proc['stdout']);stderr=self._save('receipts/'+ident+'-stderr.bin',proc['stderrTail'])
  allowed=['result.json','decoded.json'] if mode in ('cross','cross-negative') else ['result.json']+[f'{key}/{layer}.json' for key in point_keys for layer in LAYERS if layer.startswith(side+'-')]
  if mode=='ingress':allowed=ingress_paths(side)
  artifacts=[];layout_valid=True
  if out.exists():
   for path in out.rglob('*'):
    if path.is_symlink() or (path.is_file() and str(path.relative_to(out)) not in allowed):layout_valid=False
   for relative in allowed:
    p=out/relative
    if p.exists():
     try:artifacts.append(self.reference(p,131072 if relative=='result.json' else 262144))
     except (OSError,Rejected):layout_valid=False
  try:
   need(layout_valid and not proc.get('transportError'),'missing-evidence')
   witness=proc.get('executionWitness');need(type(witness) is dict and set(witness)=={'pid','startedNs','endedNs','argvSHA256'} and type(witness['pid']) is int and witness['pid']>0 and type(witness['startedNs']) is int and type(witness['endedNs']) is int and 0<witness['startedNs']<=witness['endedNs'],'missing-evidence')
   actual_argv=proc['originalArgv'] if isinstance(proc,VerifiedReplay) else argv
   need(witness['argvSHA256']==execution_witness(actual_argv,witness['pid'],witness['startedNs'],witness['endedNs'])['argvSHA256'],'missing-evidence')
   requested=proc['originalRequestedTimeoutNs'] if isinstance(proc,VerifiedReplay) else int(timeout*1000000000)
   rw=proc.get('resourceWitness');need(type(rw) is dict and set(rw)=={'stdoutBytes','stderrBytes','timeoutNs','deadlineNs','terminationNs','terminationReason'} and all(type(rw[k]) is int for k in ('stdoutBytes','stderrBytes','timeoutNs','deadlineNs','terminationNs')),'missing-evidence')
   need(0<requested<=120000000000 and rw['timeoutNs']==requested and rw['deadlineNs']==witness['startedNs']+requested and rw['stdoutBytes']==len(proc['stdout']) and rw['stderrBytes']==proc['stderrBytes'] and rw['terminationReason']=='normal' and rw['terminationNs']==0,'missing-evidence')
   protocol=load_json(read_file(out/'result.json',131072),Limits(bytes=131072,depth=16,nodes=16384,string=4096,strings=131072,array=512),entry='process-result')
   outcome=validate_exit(mode,side,proc['returncode'],protocol)
   if mode=='mutation':need(protocol['id']==(metadata or {}).get('mutationID'),'missing-evidence')
   if mode=='ingress':need(protocol['mode']==(metadata or {}).get('faultMode'),'missing-evidence')
   result_bytes=read_file(out/'result.json',131072)
   need(proc['stdout']==(result_bytes+b'\n' if side=='go' else b''),'missing-evidence')
   if mode=='case':
    need(protocol['caseID']==(metadata or {}).get('caseID'),'schema-invalid')
    refs={a['name']:a for a in artifacts}
    if outcome=='success':need({p['key'] for p in protocol['points']}==set(point_keys),'missing-evidence')
    for point in protocol['points']:
     if outcome=='success':need(len(point['artifacts'])==(5 if side=='go' else 4),'missing-evidence')
     for a in point['artifacts']:
      actual=refs.get(str(out.relative_to(self.root))+'/'+a['name']);need(actual is not None and actual['bytes']==a['bytes'] and actual['sha256']==a['sha256'],'missing-evidence')
   if mode in ('cross','cross-negative') and outcome=='success':
    decoded=next((a for a in artifacts if a['name'].endswith('/decoded.json')),None);need(decoded is not None,'missing-evidence')
    if side=='go':need(protocol.get('decodedSHA256')==decoded['sha256'],'missing-evidence')
   reason='none' if outcome=='success' else 'codec-rejected' if mode.startswith('cross') else 'invalid-fixture'
  except (Rejected,OSError,KeyError):outcome='harness-error'
  artifact_manifests=[]
  if mode=='ingress':
   nested=[a for a in artifacts if a['name']!=str(out.relative_to(self.root))+'/result.json']
   for i in range((len(nested)+255)//256):artifact_manifests.append(self._save('receipts/'+ident+'-artifacts-'+str(i)+'.json',json.dumps({'schemaVersion':2,'files':nested[i*256:(i+1)*256]},separators=(',',':')).encode()))
   artifacts=[a for a in artifacts if a['name']==str(out.relative_to(self.root))+'/result.json']
  value={'schemaVersion':3,'resourceWitness':proc.get('resourceWitness'),'requestedTimeoutNs':proc['originalRequestedTimeoutNs'] if isinstance(proc,VerifiedReplay) else int(timeout*1000000000),'bindingSHA256':self.binding,'artifactManifests':artifact_manifests,'id':ident,'mode':mode,'side':side,'metadata':metadata or {},'inputs':inputs,'returncode':proc['returncode'],'transportError':proc.get('transportError',''),'argv':proc['originalArgv'] if isinstance(proc,VerifiedReplay) else argv,'executionWitness':proc.get('executionWitness'),'outcome':outcome,'reason':reason,'stdout':stdout,'stderr':stderr,'stderrBytes':proc['stderrBytes'],'artifacts':artifacts,'schemaValidWitness':outcome=='success' and protocol is not None}
  self.items[ident]=value
  receipt=self._save('receipts/'+ident+'.json',json.dumps(value,separators=(',',':')).encode());self.refs.append(receipt);self.finish()
  return proc,protocol,value
 def direction(self,ident,verdict,comparisons):
  import json
  from bounded import need
  item=self.items[ident];need(item['mode']=='cross','schema-invalid')
  process_ref=next(r for r in self.refs if r['name']=='receipts/'+ident+'.json')
  decoded=next((r for r in item['artifacts'] if r['name'].endswith('/decoded.json')),None)
  value={'schemaVersion':3,'bindingSHA256':self.binding,'id':ident,'processReceiptSHA256':process_ref['sha256'],'input':item['inputs'][0],'decoded':decoded,'verdict':verdict,'comparisons':comparisons}
  ref=self._save('receipts/'+ident+'-direction.json',json.dumps(value,separators=(',',':')).encode());self.directions.append(ref);self.finish()

 def finish(self):
  import json
  from bounded import need,sha,no_symlink
  def replace(name,value):
   data=json.dumps(value,separators=(',',':')).encode();need(len(data)<=131072,'resource-limit')
   path=no_symlink(self.root/name);temp=no_symlink(self.root/(name+'.next'));previous=path.stat().st_size if path.exists() else 0
   need(self.budget[0]-previous+len(data)<=80*1024*1024,'resource-limit')
   with open(temp,'xb') as f:f.write(data);f.flush();os.fsync(f.fileno())
   os.replace(temp,path);self.budget[0]+=len(data)-previous
   return {'name':name,'bytes':len(data),'sha256':sha(data)}
  groups={}
  for group,refs,maximum in (('receipts',self.refs,3),('directions',self.directions,2)):
   need(len(refs)<=maximum*128,'resource-limit');parts=[]
   for i in range((len(refs)+127)//128):
    parts.append(replace('execution-'+group+'-part-'+str(i)+'.json',{'schemaVersion':3,'bindingSHA256':self.binding,'items':refs[i*128:(i+1)*128]}))
   groups[group]=parts
  return replace('execution-receipts.json',{'schemaVersion':3,'bindingSHA256':self.binding,'receiptParts':groups['receipts'],'directionParts':groups['directions']})
