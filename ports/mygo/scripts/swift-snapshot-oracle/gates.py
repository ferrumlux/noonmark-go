# SPDX-License-Identifier: AGPL-3.0-only
"""Executed negative gates. Each pass is an assertion, not a child summary claim.

Rows are synthetic contract evidence only and never business comparison points.
"""
from pathlib import Path
import copy, dataclasses, json, os, sys, tempfile
from bounded import Limits, Rejected, load_json, load_spec, validate_envelope, read_file, sha, need, Archive, predict
from process import execute

INGRESS={
 'stdin':Limits(bytes=1024,depth=4,nodes=16,string=80,strings=1024,array=0),
 'registry':Limits(bytes=65536,depth=12,nodes=4096,string=8192,strings=32768,array=18),
 'vectors':Limits(bytes=16384,depth=6,nodes=256,string=8192,strings=16384,array=11),
 'field-schema':Limits(bytes=65536,depth=8,nodes=4096,string=256,strings=49152,array=19),
 'source-manifest':Limits(bytes=131072,depth=8,nodes=4096,string=256,array=64),
 'guard-evidence':Limits(bytes=65536,depth=6,nodes=512,string=80,array=64),
 'capture':Limits(bytes=131072,depth=16,nodes=8192,string=256,array=64),
}
for name in ('seed','swift-engine-projection','go-kernel-projection','go-from-domain-projection','swift-package','go-package','swift-export','go-export','swift-self-reencoded','go-self-reencoded','cross-swift','cross-go'):INGRESS[name]=Limits()

def raw(v):return json.dumps(v,ensure_ascii=False,separators=(',',':')).encode()
def row(name,fn):
 import time,gate_receipts as gr
 start=time.monotonic_ns()
 try:fn();outcome='pass'
 except Exception:outcome='fail'
 gr.record_result(name,outcome,start,time.monotonic_ns())
 return {'check':name,'outcome':outcome}
def reject_before_callback(data,limits):
 calls={k:0 for k in ('decode','clone','bridge','command','serializer')}
 try:
  load_json(data,limits)
 except Rejected:
  need(all(v==0 for v in calls.values()));return
 calls['decode']+=1;raise AssertionError('negative input admitted')
def tree_nodes(count,branch):
 if count<=1:return 0
 if branch<2:return {str(i):0 for i in range((count-1+1)//2)}
 children=[];remaining=count-1
 while remaining:
  take=min(remaining,max(1,(remaining+branch-len(children)-1)//(branch-len(children))))
  children.append(tree_nodes(take,branch));remaining-=take
 return children

def scanner_contracts():
 rows=[]
 for name,lim in INGRESS.items():
  # A stricter overlapping limit may reject first. Either way no downstream
  # domain operation is invoked; this never asserts unreachable allocation.
  strings=['x'*lim.string]*(lim.strings//max(1,lim.string))+['x'*(lim.strings%max(1,lim.string)+1)]
  data={
   'bytes':b'0'+b' '*lim.bytes,
   'depth':b'['*lim.depth+b'0'+b']'*lim.depth,
   'nodes':raw(tree_nodes(lim.nodes+1,max(0,lim.array))),
   'string':raw('x'*(lim.string+1)),
   'escaped-string':b'"'+b'\\u0061'*(lim.string+1)+b'"',
   'strings':raw(strings),
   'array':raw([0]*(lim.array+1)),
   'members':raw({str(i):0 for i in range(lim.members+1)}),
  }
  for dimension,b in data.items():rows.append(row('V06-'+name+'-'+dimension,lambda b=b,lim=lim:reject_before_callback(b,lim)))
  for label,b in [('duplicate-key',b'{"a":0,"\\u0061":1}'),('invalid-utf8',b'"\xff"'),('surrogate',b'"\\ud800"'),('float',b'1.5'),('nonfinite',b'NaN'),('integer-overflow',b'18446744073709551616')]:rows.append(row('V06-'+name+'-'+label,lambda b=b,lim=lim:reject_before_callback(b,lim)))
 return rows

def schema_contracts(spec):
 rows=[]
 for sid,seed in spec['seeds'].items():
  for field in ('taskCycleSeries','ideas'):
   def check(seed=seed,field=field):
    x=copy.deepcopy(seed);x['snapshot'][field]=[{}]
    try:validate_envelope(x,spec['schema'],spec['preferences'])
    except Rejected:return
    raise AssertionError('nonempty unsupported admitted')
   rows.append(row('V06-'+sid+'-'+field+'-one',check))
  for field,value in seed['snapshot']['classifications'].items():
   if type(value) is not list:continue
   for mutation in ('one','odd-pairs','duplicate-pairs'):
    def check(seed=seed,field=field,mutation=mutation):
     x=copy.deepcopy(seed);x['snapshot']['classifications'][field]=[{}] if mutation!='duplicate-pairs' else [{},{},{},{}]
     try:validate_envelope(x,spec['schema'],spec['preferences'])
     except Rejected:return
     raise AssertionError('nonempty history admitted')
    rows.append(row('V06-'+sid+'-'+field+'-'+mutation,check))
  for field,value in [('base64','eA=='),('unknown',0)]:
   def check(seed=seed,field=field,value=value):
    x=copy.deepcopy(seed);x[field]=value
    try:validate_envelope(x,spec['schema'],spec['preferences'])
    except Rejected:return
    raise AssertionError('unknown field admitted')
   rows.append(row('V06-'+sid+'-'+field,check))
 for section in ('days','chains','definitions','traces','subtasks'):
  def check(section=section):
   x=copy.deepcopy(spec['seeds']['multi-interleaved']);x['snapshot'][section]=x['snapshot'][section][:1]*17
   try:validate_envelope(x,spec['schema'],spec['preferences'])
   except Rejected:return
   raise AssertionError('entity array limit admitted')
  rows.append(row('V06-array-'+section+'-17',check))
 for child in ('plannedSubtasks','noteEntries','syncEndpointOptions'):
  def check(child=child):
   x=copy.deepcopy(spec['seeds']['planned-12'])
   if child=='plannedSubtasks':x['snapshot']['definitions'][0][child]=x['snapshot']['definitions'][0][child][:1]*3
   elif child=='noteEntries':x['snapshot']['chains'][0][child]=[{},{}]
   else:x['snapshot']['preferences'][child]=[{}]
   try:validate_envelope(x,spec['schema'],spec['preferences'])
   except Rejected:return
   raise AssertionError('nested array limit admitted')
  rows.append(row('V06-array-'+child+'-plus-one',check))
 # Every frozen field is checked for missing/unknown/null using the very same
 # typed schema validator as captures, including optional fields absent in M1.
 def visit(v,typ,path,seed):
  if typ.startswith('[]'):
   for i,z in enumerate(v):visit(z,typ[2:],path+[i],seed)
  elif typ in spec['schema']['objects']:
   for f in spec['schema']['objects'][typ]:
    key=f['wire']
    def check(path=path,key=key,seed=seed):
     x=copy.deepcopy(seed);obj=x
     for k in path:obj=obj[k]
     obj[key]=None
     try:validate_envelope(x,spec['schema'],spec['preferences'])
     except Rejected:return
     raise AssertionError('null field admitted')
    name='V06-field-'+typ+'-'+key+'-null'
    if not any(r['check']==name for r in rows):rows.append(row(name,check))
    if key in v:visit(v[key],f['type'],path+[key],seed)
 for seed in spec['seeds'].values():visit(seed,'Envelope',[],seed)
 synthetic=copy.deepcopy(spec['seeds']['planned-12'])
 chain=synthetic['snapshot']['chains'][0]
 command=next(c for c in spec['registry']['cases'] if c['id']=='create-valid')['steps'][0]
 chain['noteEntries']=[dict(id={'rawValue':command['noteGoID']},body='synthetic-negative-only',createdAt=chain['createdAt'],updatedAt=chain['updatedAt'])]
 visit(synthetic,'Envelope',[],synthetic)
 # Stable names are unique in the evidence file; repeated fixture paths are
 # separately exercised above but collapse only duplicate identical checks.
 unique={r['check']:r for r in rows};return list(unique.values())

def archive_contracts(work):
 rows=[]
 def run_one(kind):
  root=work/kind;root.mkdir();a=Archive(root,['observe-empty']);calls=[0]
  def qualify(b):load_json(b);calls[0]+=1
  if kind=='malformed':
   data=b'{"a":0,"a":1}';o=a.save('observe-empty','seed','swift-package',data,qualify)
   need(o['rawState']=='malformed-raw' and calls[0]==0 and read_file(root/o['artifact']['name'],262144)==data and o['artifact']['sha256']==sha(data))
  else:
   if kind=='count':a.counts['observe-empty']=18;data=b'{}'
   elif kind=='case-bytes':a.sizes['observe-empty']=4*1024*1024;data=b'{}'
   elif kind=='run-bytes':a.total=80*1024*1024-1024;data=b'x'*1025
   else:data=b'x'*262145
   try:a.save('observe-empty','seed','swift-package',data,qualify)
   except Rejected:
    need(calls[0]==0 and not (root/'observe-empty/seed/swift-package.json').exists());return
   raise AssertionError('archive limit admitted')
 for k in ('malformed','count','case-bytes','run-bytes','raw-bytes'):rows.append(row('V06-archive-'+k,lambda k=k:run_one(k)))
 return rows

def process_contracts():
 rows=[]
 for name,code,timeout in [('stdout','import os;os.write(1,b"x"*131073)',2),('stderr','import os\nwhile True:os.write(2,b"x"*8192)',2),('timeout','import time;time.sleep(2)',.02)]:
  def check(code=code,timeout=timeout):
   try:execute([sys.executable,'-c',code],timeout=timeout)
   except Rejected:return
   raise AssertionError('process budget admitted')
  rows.append(row('V07-process-'+name,check))
 return rows

def source_contracts(spec,work):
 """Same immutable manifest and row authentication as the compile admission."""
 import runner
 rows=[];manifest=spec['manifest']
 expected=raw(manifest)
 def reject_manifest(value):
  need(raw(value)!=expected) # Mutation construction itself must be effective.
  try:runner.authenticate_manifest(value)
  except (Rejected,runner.GuardError):return
  raise AssertionError('source mutation admitted')
 for key in ('commit','rootTree','coreTree','storageTree','appTree'):
  x=copy.deepcopy(manifest);x[key]='0'*40;rows.append(row('V01-'+key,lambda x=x:reject_manifest(x)))
 for mutation in ('missing','extra','duplicate'):
  x=copy.deepcopy(manifest)
  if mutation=='missing':x['files'].pop()
  elif mutation=='extra':x['files'].append(dict(x['files'][0],path='unexpected.swift'))
  else:x['files'].append(x['files'][0])
  rows.append(row('V01-manifest-'+mutation,lambda x=x:reject_manifest(x)))
 for i,item in enumerate(manifest['files']):
  x=copy.deepcopy(manifest);x['files'][i]['sha256']=format(int(item['sha256'][0],16)^1,'x')+item['sha256'][1:]
  rows.append(row('V01-source-'+str(i)+'-sha',lambda x=x:reject_manifest(x)))
  x=copy.deepcopy(manifest);x['files'][i]['gitBlob']=format(int(item['gitBlob'][0],16)^1,'x')+item['gitBlob'][1:]
  rows.append(row('V01-source-'+str(i)+'-blob',lambda x=x:reject_manifest(x)))
 # Tool validation uses exact frozen file set and the actual hash reader.
 tool=load_json(read_file(Path(runner.HERE)/'tool-manifest.json',131072),Limits(bytes=131072,depth=8,nodes=4096,string=256,array=64))
 for i in range(len(tool['files'])):
  value=copy.deepcopy(tool);value['files'][i]['sha256']=format(int(tool['files'][i]['sha256'][0],16)^1,'x')+tool['files'][i]['sha256'][1:];p=work/f'tool-mutation-{i}.json';p.write_bytes(raw(value))
  def check(p=p):
   try:runner.validate_tool_manifest(p)
   except (Rejected,runner.GuardError):return
   raise AssertionError('tool source mutation admitted')
  rows.append(row('V01-tool-'+str(i)+'-sha',check))
 for mutation in ('missing','extra','duplicate'):
  value=copy.deepcopy(tool)
  if mutation=='missing':value['files'].pop()
  elif mutation=='extra':value['files'].append(dict(value['files'][0],path='unexpected-tool.py'))
  else:value['files'].append(value['files'][0])
  p=work/f'tool-structural-{mutation}.json';p.write_bytes(raw(value))
  def check(p=p):
   try:runner.validate_tool_manifest(p)
   except (Rejected,runner.GuardError):return
   raise AssertionError('tool manifest shape admitted')
  rows.append(row('V01-tool-'+mutation,check))
 return rows

def isolation_contracts(scope,guard_raw,work):
 import controller,runner
 rows=[];guards=load_json(guard_raw,INGRESS['guard-evidence'])
 import gate_receipts as gr
 try:platform=gr.verify_isolation(work.parent,runner.verify_provenance(work.parent.parent,gr.ensure_run(work.parent)))
 except Exception:platform={}
 for item in guards:
  def check(name=item['check']):
   g=[v for v in guards if v['check']!=name];s=dict(scope,guardEvidenceSHA256=sha(raw(g)))
   try:controller.validate_scope(s,raw(g))
   except Rejected:return
   raise AssertionError('missing guard admitted')
  rows.append(row('V09-missing-'+item['check'],check))
  rows.append(row('V09-runtime-'+item['check'],lambda item=item:need(platform.get(item['check']) is True)))
 import gate_receipts as gr
 if gr.ACTIVE is not None:
  for mode in ('artifact-extra','overlap'):
   receipt=gr.capture_path(work.parent,mode,work);gr.ACTIVE.store(receipt);rows.append({'check':receipt['id'],'outcome':receipt['outcome']})
 else:
  # 非运行单测不产生本次平台/归档见证。
  rows.extend([{'check':'V09-artifact-extra','outcome':'fail'},{'check':'V09-overlap','outcome':'fail'}])
 return rows

def aggregate(rows,prefix):
 selected=[r for r in rows if r['check'].startswith(prefix+'-')]
 return 'pass' if selected and all(r['outcome']=='pass' for r in selected) else 'fail'

MUTATIONS=('raw-bytes','raw-depth','raw-nodes','raw-string','raw-string-sum','raw-array','raw-members','profile-empty-array','profile-pairs-odd','profile-pairs-duplicate','profile-pairs-nonempty','profile-base64','hex-odd','hex-invalid-utf8','hex-decoded-plus-one','hex-double-expansion','create-growth-nodes','create-growth-entities','create-growth-text','create-growth-output','schedule-growth-nodes','schedule-growth-entities','schedule-growth-text','schedule-growth-output')

def execute_runtime_gates(spec,round_cases,artifact_reader,cross_decode,mutation_call,work,scope,guard_raw,source_rows,*,ingress_call=None,python_entry_call=None):
 import gate_receipts as gr
 with gr.recording(work.parent):
  result=_execute_runtime_gates(spec,round_cases,artifact_reader,cross_decode,mutation_call,work,scope,guard_raw,source_rows,ingress_call=ingress_call,python_entry_call=python_entry_call)
  result['gates']=gr.finalize(work.parent,result['rows'])
  return result

def _execute_runtime_gates(spec,round_cases,artifact_reader,cross_decode,mutation_call,work,scope,guard_raw,source_rows,*,ingress_call=None,python_entry_call=None):
 from gates_comparison import execute_comparison_gates
 work.mkdir();rows=[]
 need(type(source_rows) is list and source_rows and all(set(r)=={'check','outcome'} and r['check'].startswith('V01-') and r['outcome'] in ('pass','fail') for r in source_rows),'missing-evidence')
 rows.extend(source_rows);rows.extend(scanner_contracts());rows.extend(schema_contracts(spec))
 import gate_receipts as gr
 rows.extend(gr.capture_resource_rows(work));rows.extend(isolation_contracts(scope,guard_raw,work))
 receipts=[]
 for side in ('swift','go'):
  for mutation in (*MUTATIONS,'control-valid'):
   prefix='V07' if '-growth-' in mutation or mutation=='control-valid' else 'V06'
   def check(side=side,mutation=mutation):
    result=mutation_call(side,mutation)
    need(type(result) is dict and set(result)=={'schemaVersion','id','accepted','reason','counters'})
    need(result['schemaVersion']==2 and result['id']==mutation and type(result['accepted']) is bool and type(result['reason']) is str and len(result['reason'].encode())<=256)
    counters=result['counters'];need(set(counters)=={'decode','clone','bridge','command','serializer'} and all(type(v) is int and v>=0 for v in counters.values()))
    if mutation=='control-valid':need(result['accepted'] is True and all(v>0 for v in counters.values()))
    else:need(result['accepted'] is False and result['reason']!='none' and all(v==0 for v in counters.values()))
    receipts.append({'side':side,'mutation':mutation,'result':result})
   rows.append(row(prefix+'-'+side+'-'+mutation,check))
 comp=execute_comparison_gates(spec,round_cases,artifact_reader,cross_decode=cross_decode);rows.extend(comp['rows'])
 for side in ('swift','go'):
  matrices={}
  for mode in ('normal','bypass','late'):
   def check(side=side,mode=mode):
    need(ingress_call is not None,'missing-evidence');matrices[mode]=ingress_call(side,mode)
    valid=grade_probe_ingress(side,matrices[mode]);need(valid if mode=='normal' else fault_detected(side,matrices[mode]),'missing-evidence')
   rows.append(row('V06-real-entry-'+side+'-'+mode,check))
 def python_entries():
  need(python_entry_call is not None,'missing-evidence');result=python_entry_call();need(result.get('pass') is True,'missing-evidence')
 rows.append(row('V06-real-python-entries',python_entries))
 outcomes={k:aggregate(rows,k) for k in ('V01','V06','V07','V09')};outcomes.update(comp['gates'])
 return {'schemaVersion':2,'gates':outcomes,'rows':rows,'mutationReceipts':receipts,'crossDecodeRecords':comp['crossDecodeRecords']}


def verify_recorded_gates(root,spec,rounds,receipts):
 from gate_receipts import verify_all
 return verify_all(root,spec,rounds,receipts)


# Independent closed row registry. Producer expectedRows is never the authority.
PROBE_ENTRIES={'go': {'config-FieldSchema-R2.json': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'cardinality'], 'config-SeedRegistry-R2.json': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'cardinality'], 'config-NoteVectors-R2.json': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'cardinality', 'hex-odd', 'hex-invalid-utf8', 'hex-decoded-plus-one', 'hex-double-expansion'], 'config-case': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'cardinality'], 'seed-empty': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'seed-planned-12': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'seed-planned-21': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'seed-multi-interleaved': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'go-kernel-projection': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'go-from-domain-projection': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'go-package': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'go-export': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'go-self-reencoded': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'cross': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'create': ['positive'], 'schedule': ['positive']}, 'swift': {'config-FieldSchema-R2.json': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'cardinality'], 'config-SeedRegistry-R2.json': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'cardinality'], 'config-NoteVectors-R2.json': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'cardinality', 'hex-odd', 'hex-invalid-utf8', 'hex-decoded-plus-one', 'hex-double-expansion'], 'config-case': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'cardinality'], 'seed-empty': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'seed-planned-12': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'seed-planned-21': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'seed-multi-interleaved': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'swift-engine-projection': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'swift-package': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'swift-export': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'swift-self-reencoded': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'cross': ['positive', 'bytes', 'depth', 'nodes', 'string', 'sum', 'array', 'members', 'utf8', 'duplicate', 'float', 'empty-array', 'pairs-odd', 'pairs-duplicate', 'pairs-nonempty', 'base64', 'entities-array', 'planned-array', 'notes-array', 'days-array', 'definitions-array', 'traces-array', 'subtasks-array', 'trace-notes-array'], 'create': ['positive'], 'schedule': ['positive']}}

def grade_probe_rows(side,value):
 import re
 expected=[e+'/'+d for e,ds in PROBE_ENTRIES[side].items() for d in ds]
 need(type(value) is dict and set(value)=={'schemaVersion','mode','pass','expectedRows','rows'} and value['schemaVersion']==2 and value['mode'] in ('normal','bypass','late') and type(value['pass']) is bool)
 need(value['expectedRows']==expected and type(value['rows']) is list and len(value['rows'])==len(expected))
 verdicts={}
 scanner={'bytes','depth','nodes','string','sum','array','members','utf8','duplicate','float'}
 for key,r in zip(expected,value['rows']):
  need(type(r) is dict and set(r)=={'entry','dimension','outcome','reason','reached','events','counters','pass','inputBytes','inputSHA256'} and r['entry']+'/'+r['dimension']==key)
  need(type(r['pass']) is bool and type(r['reached']) is bool and type(r['reason']) is str and len(r['reason'].encode())<=256 and type(r['events']) is list and len(r['events'])<=64 and all(e in ('scan','scan-pass','materialize','reject','profile','profile-reject','decode','clone','bridge','command','serializer') for e in r['events']))
  c=r['counters'];need(type(c) is dict and set(c)=={'materialize','decode','clone','bridge','command','serializer'} and all(type(n) is int and 0<=n<=64 for n in c.values()))
  need(type(r['inputBytes']) is int and 0<r['inputBytes']<=1048576 and type(r['inputSHA256']) is str and re.fullmatch('[0-9a-f]{64}',r['inputSHA256']))
  # Counts are witnessed event counts, not unconnected constants.
  valid=r['reached'] and all(c[k]==r['events'].count(k) for k in c)
  d=r['dimension'];events=r['events'];domain=[c[k] for k in ('decode','clone','bridge','command','serializer')]
  if r['entry'] in ('create','schedule'):
   valid=valid and d=='positive' and r['outcome']=='accepted' and all(n>0 for n in domain)
  elif d=='positive':valid=valid and r['outcome']=='accepted' and events[:2]==['scan','scan-pass'] and c['materialize']>0
  elif d in scanner:valid=valid and r['outcome']=='rejected' and bool(r['reason']) and events==['scan','reject'] and all(n==0 for n in c.values())
  else:valid=valid and r['outcome']=='rejected' and bool(r['reason']) and len(events)>=2 and events[0]=='scan' and events[1] in ('scan-pass','reject') and not any(domain)
  verdicts[key]=bool(valid)
 # The producer must also report honestly. It cannot grant acceptance itself.
 need(all(r['pass']==verdicts[k] for k,r in zip(expected,value['rows'])) and value['pass']==all(verdicts.values()),'missing-evidence')
 return verdicts

def grade_probe_ingress(side,value):
 try:return all(grade_probe_rows(side,value).values())
 except (Rejected,KeyError,TypeError,ValueError):return False

def fault_detected(side,value):
 try:
  grades=grade_probe_rows(side,value)
  return not all(grades.values()) and all(not grades[e+'/positive'] for e in PROBE_ENTRIES[side] if e not in ('create','schedule'))
 except (Rejected,KeyError,TypeError,ValueError):return False

def evaluate_ingress_receipts(root,spec,receipts):
 result={'V06':'fail','V07':'fail'}
 try:
  for side in ('swift','go'):
   for mode in ('normal','bypass','late'):
    r,v=receipts['ingress-'+side+'-'+mode]
    need(r['outcome']=='success' and r['schemaValidWitness'] is True and v['mode']==mode)
    need(grade_probe_ingress(side,v) if mode=='normal' else fault_detected(side,v))
  # Python entry witnesses are independently replayed below once available.
  python_check=globals().get('verify_controller_entries')
  need(python_check is not None and python_check(root,spec))
  result['V06']='pass'
  for side in ('swift','go'):
   for mutation in (*[m for m in MUTATIONS if '-growth-' in m],'control-valid'):
    r,v=receipts['mutation-'+side+'-'+mutation];need(r['outcome']=='success' and r['schemaValidWitness'] is True)
    counts=v['counters'];positive=mutation=='control-valid'
    need(v['accepted'] is positive and all(n>0 if positive else n==0 for n in counts.values()))
  result['V07']='pass'
 except (Rejected,KeyError,TypeError,ValueError):pass
 return result

# Actual controller ingress matrix. Kept separate from scanner_contracts(),
# which remains a parser unit test and must never substitute for these receipts.
PY_ENTRY_SCANNER_DIMENSIONS=('positive','bytes','depth','nodes','string','sum','array','members','utf8','duplicate','float')
PY_ENTRY_CONFIG=('stdin','registry','vectors','field-schema','source-manifest')
PY_ENTRY_SEEDS=tuple('seed-'+s for s in ('empty','planned-12','planned-21','multi-interleaved'))
PY_ENTRY_OUTPUTS=('swift-engine-projection','go-kernel-projection','go-from-domain-projection','swift-package','go-package','swift-export','go-export','swift-self-reencoded','go-self-reencoded','cross-swift','cross-go')
PY_ENTRY_ALL=PY_ENTRY_CONFIG+PY_ENTRY_SEEDS+PY_ENTRY_OUTPUTS+('capture','guard-evidence','tool-manifest')
PY_ENTRY_MATRIX={entry:PY_ENTRY_SCANNER_DIMENSIONS for entry in PY_ENTRY_ALL}

def _controller_entry_mutation(original,limits,dimension):
 """Closed generated bytes enter bounded.EntryTrace at the actual named reader."""
 if dimension=='positive':return original
 if dimension not in PY_ENTRY_SCANNER_DIMENSIONS:return _controller_schema_mutation(original,dimension)
 if dimension=='bytes':return b' '*(limits.bytes+1)
 if dimension=='depth':return b'['*limits.depth+b'0'+b']'*limits.depth
 if dimension=='nodes':return raw(tree_nodes(limits.nodes+2,max(2,limits.array)))
 if dimension=='string':return raw('x'*(limits.string+1))
 if dimension=='sum':
  chunks=['x'*limits.string]*(limits.strings//max(1,limits.string)+1)
  if limits.array>0 and len(chunks)>limits.array:chunks=[chunks[i:i+limits.array] for i in range(0,len(chunks),limits.array)]
  return raw(chunks)
 if dimension=='array':return raw([0]*(limits.array+1))
 if dimension=='members':return raw({f'k{i}':0 for i in range(limits.members+1)})
 if dimension=='utf8':return b'"\xff"'
 if dimension=='duplicate':return b'{"a":1,"a":2}'
 if dimension=='float':return b'{"a":1.5}'
 raise Rejected('unknown-dimension')

def _synthetic_schema_capture(spec):
 """Schema-only positive input, explicitly pending with zero business points."""
 from controller import TOOLCHAIN
 tc={key:('0'*64 if key.endswith('SHA256') else 'SYNTHETIC-SCHEMA-ONLY') for key in TOOLCHAIN}
 tc.update(architecture='x86_64',locale='en_US.UTF-8',timezone='UTC',runnerCommit='0'*40)
 scope={key:False for key in ('productionDataAccess','iCloudAccess','keychainAccess','aiNetworkAccess','actualAppExecuted')}
 scope.update(selectedStorageSources=True,guardEvidenceSHA256='0'*64)
 return dict(schemaVersion=2,caseID='observe-empty',registrySHA256=spec['registry_sha'],sourceManifestSHA256=sha(read_file(Path(__file__).parent/'spec/SourceManifest-R2.json',131072)),toolchain=tc,scope=scope,fixtureValidity='pending',points=[],summary=dict(fixtureValidity='pending',mapping='not-run',effectivePointCount=0,interopPointCount=0,negativeGatePassCount=0,reportComplete=False,observationAccepted=False,interopAccepted=False))

def _controller_entry_dispatch(entry,specdir,spec,capture,scope,guard_raw,tool_manifest,snapshot_data=None):
 """Each branch invokes an ordinary public reader, never a relabelled scanner."""
 import bounded as b
 if entry=='stdin':
  return b.invocation(raw(dict(schemaVersion=2,registrySHA256=spec['registry_sha'],caseID='observe-empty')),spec)
 if entry in ('registry','vectors','field-schema','source-manifest') or entry in PY_ENTRY_SEEDS:
  loaded=b.load_spec(specdir)
  if entry in PY_ENTRY_SEEDS:
   seed=loaded['seeds'][entry[5:]];b.validate_envelope(seed,loaded['schema'],loaded['preferences']);b.predict(seed)
  if entry=='source-manifest':
   from runner import authenticate_manifest
   authenticate_manifest(loaded['manifest'])
  return loaded
 if entry in PY_ENTRY_OUTPUTS:
  if snapshot_data is not None:
   need(entry in snapshot_data and type(snapshot_data[entry]) is bytes,'missing-evidence')
   return b.read_snapshot(snapshot_data[entry],spec,entry,export=entry in ('swift-export','go-export'))
  envelope=copy.deepcopy(spec['seeds']['empty']);export=entry in ('swift-export','go-export')
  if export:envelope['snapshot']['preferences']['localFirstSyncPolicy']['enabled']=False
  return b.read_snapshot(raw(envelope),spec,entry,export=export)
 if entry=='capture':return b.validate_capture(capture if type(capture) is bytes else raw(capture if capture is not None else _synthetic_schema_capture(spec)))
 if entry=='guard-evidence':
  if scope is None or guard_raw is None:raise Rejected('missing-evidence')
  from controller import validate_scope
  return validate_scope(scope,guard_raw)
 if entry=='tool-manifest':
  if tool_manifest is None:raise Rejected('missing-evidence')
  from runner import validate_tool_manifest
  return validate_tool_manifest(Path(tool_manifest))
 raise Rejected('unknown-entry')

def controller_entry_matrix(specdir,registry,capture,scope,tool_manifest,work,mode,*,entries=None,guard_raw=None,snapshot_data=None,snapshot_origins=None):
 """Real shared Python ingress witnesses; unavailable inputs remain not-run.

 registry may be an already verified load_spec result, or {} to load it here.
 No source-root mutation, monkeypatched labels, subprocesses, or tool manifest
 regeneration occurs. Work is retained in the API for the parent's evidence
 ledger; this function returns bounded receipts for that ledger to archive.
 """
 import bounded as b
 need(mode in ('normal','bypass','late'),'unknown-matrix-mode')
 entries=tuple(PY_ENTRY_ALL if entries is None else entries)
 need(len(set(entries))==len(entries) and set(entries)<=set(PY_ENTRY_ALL),'unknown-entry')
 expected=[entry+'/'+dimension for entry in entries for dimension in PY_ENTRY_MATRIX[entry]]
 report=dict(schemaVersion=2,mode=mode,expectedRows=expected,rows=[],pass_=False)
 # Setup uses the real loader too. An always-reject scanner must make the
 # positive matrix fail, not be bypassed to manufacture fixture metadata.
 setup_error=None
 try:spec=registry if isinstance(registry,dict) and set(('schema','registry','registry_sha','seeds','preferences'))<=set(registry) else b.load_spec(specdir)
 except Exception as error:setup_error=error;spec=None
 for entry in entries:
  for dimension in PY_ENTRY_MATRIX[entry]:
   trace=b.EntryTrace(entry,dimension,mode,_controller_entry_mutation)
   accepted=False;reason='';missing=setup_error is not None or (entry=='guard-evidence' and (scope is None or guard_raw is None)) or (entry=='tool-manifest' and tool_manifest is None)
   if missing:reason='missing-evidence' if setup_error is None else 'setup-rejected'
   else:
    try:
     with trace:_controller_entry_dispatch(entry,specdir,spec,capture,scope,guard_raw,tool_manifest,snapshot_data)
     accepted=True
    except Exception as error:reason=error.reason if isinstance(error,b.Rejected) else 'schema-or-source-rejected'
   if dimension=='positive':passed=trace.reached and accepted and trace.events[:2]==['scan','scan-pass'] and trace.counters['materialize']>0
   else:
    if dimension in PY_ENTRY_SCANNER_DIMENSIONS:passed=trace.reached and not accepted and trace.events==['scan','reject'] and all(value==0 for value in trace.counters.values())
    else:passed=trace.reached and not accepted and trace.events[:1]==['scan'] and trace.events[1:2] in (['reject'],['scan-pass']) and all(trace.counters[k]==0 for k in ('compare','process','archive'))
   report['rows'].append(dict(source=(snapshot_origins or {}).get(entry,'supplied-snapshot' if snapshot_data is not None and entry in PY_ENTRY_OUTPUTS else 'schema-only-snapshot' if entry in PY_ENTRY_OUTPUTS else 'actual-reader'),entry=entry,dimension=dimension,outcome='not-run' if missing or not trace.reached else 'accepted' if accepted else 'rejected',reason=reason,reached=trace.reached,inputBytes=trace.inputBytes,inputSHA256=trace.inputSHA256,events=trace.events[:128],counters=dict(trace.counters),**{'pass':passed}))
 report['pass']=all(row['pass'] for row in report['rows']) and len(report['rows'])==len(expected)
 del report['pass_']
 # The controller archives fixed entry shards, never an oversized aggregate.
 return report

PY_ENTRY_SCHEMA_DIMENSIONS={
 'stdin':('missing-field','unknown-field','null-field'),
 'registry':('registry-seeds','registry-cases'),
 'vectors':('vectors-count','hex-odd','hex-invalid-utf8','hex-decoded-plus-one','hex-double-expansion'),
 'field-schema':('schema-fields',),
 'source-manifest':('source-files',),
 'capture':('missing-field','unknown-field','null-field','capture-points','capture-stages','capture-bindings','capture-observations','capture-comparisons'),
 'guard-evidence':('guard-count',),
 'tool-manifest':('tool-files',),
}
for _entry in PY_ENTRY_SEEDS+PY_ENTRY_OUTPUTS:PY_ENTRY_SCHEMA_DIMENSIONS[_entry]=('unknown-field','null-field','empty-array','pairs-odd','pairs-duplicate','pairs-nonempty','days-array','chains-array','definitions-array','traces-array','subtasks-array','planned-array','notes-array','trace-notes-array')
PY_ENTRY_MATRIX={entry:PY_ENTRY_SCANNER_DIMENSIONS+PY_ENTRY_SCHEMA_DIMENSIONS.get(entry,()) for entry in PY_ENTRY_ALL}

def _controller_schema_mutation(original,dimension):
 """Fixed schema mutations, only after a bounded generator input is available."""
 need(len(original)<=262144,'resource-limit');v=json.loads(original)
 if dimension=='missing-field':v.pop('schemaVersion',None)
 elif dimension=='unknown-field':v['base64']='AA=='
 elif dimension=='null-field':
  if 'snapshot' in v:v['snapshot']['preferences']=None
  else:v['schemaVersion']=None
 elif dimension=='registry-seeds':v['seeds']=v['seeds'][:-1]
 elif dimension=='registry-cases':v['cases']=v['cases'][:-1]
 elif dimension=='vectors-count':v.append(copy.deepcopy(v[0]))
 elif dimension.startswith('hex-'):
  h='0' if dimension=='hex-odd' else 'ff' if dimension=='hex-invalid-utf8' else '61'*(4097 if dimension=='hex-decoded-plus-one' else 8192)
  v[0]['input'].update(present=True,utf8Hex=h)
 elif dimension=='schema-fields':v['objects']['Envelope']=v['objects']['Envelope'][:-1]
 elif dimension in ('source-files','tool-files'):v['files'].append(copy.deepcopy(v['files'][0]))
 elif dimension=='guard-count':v=v[:-1]
 elif dimension.startswith('capture-'):
  from controller import empty_point
  p=empty_point('seed');v['points']=[p]
  if dimension=='capture-points':v['points']=[copy.deepcopy(p) for _ in range(3)]
  elif dimension=='capture-stages':p['stages']=[copy.deepcopy(p['stages'][0]) for _ in range(33)]
  elif dimension=='capture-bindings':p['bindings']=[{} for _ in range(65)]
  elif dimension=='capture-observations':p['observations']=[{} for _ in range(10)]
  elif dimension=='capture-comparisons':p['comparisons']=p['comparisons'][:-1]
 else:
  s=v['snapshot']
  if dimension=='empty-array':s['ideas']=[{}]
  elif dimension.startswith('pairs-'):s['classifications']['currentByChainID']=['same']*(1 if dimension=='pairs-odd' else 4 if dimension=='pairs-duplicate' else 2)
  elif dimension in ('days-array','chains-array','definitions-array','traces-array','subtasks-array'):s[dimension[:-6]]=[{} for _ in range(17)]
  else:
   # Full authenticated boundary templates ensure the intended nested bound,
   # not a preceding missing-field check, is the first schema failure.
   from bounded import PINNED
   name='r2-seeds/multi-interleaved.json' if dimension=='trace-notes-array' else 'r2-seeds/planned-12.json'
   data=read_file(Path(__file__).parent/'spec'/name,262144);need(sha(data)==PINNED[name],'source-mismatch');t=json.loads(data)['snapshot']
   if dimension=='planned-array':d=t['definitions'][0];d['plannedSubtasks']=[copy.deepcopy(d['plannedSubtasks'][0]) for _ in range(3)];s['definitions']=[d]
   elif dimension=='notes-array':c=t['chains'][0];c['noteEntries']=[{},{}];s['chains']=[c]
   elif dimension=='trace-notes-array':t=t['traces'][0];t['noteEntries']=[{},{}];s['traces']=[t]
   else:raise Rejected('unknown-dimension')
 return raw(v)

def controller_entry_shards(matrix):
 """Fixed per-entry objects for controller's exclusive-write/budget ledger."""
 need(set(matrix)=={'schemaVersion','mode','expectedRows','rows','pass'} and matrix['schemaVersion']==2)
 mode=matrix['mode'];need(mode in ('normal','bypass','late'))
 output={}
 for entry in PY_ENTRY_ALL:
  rows=[r for r in matrix['rows'] if r['entry']==entry]
  if not rows:continue
  expected=[entry+'/'+d for d in PY_ENTRY_MATRIX[entry]]
  need([entry+'/'+r['dimension'] for r in rows]==expected,'missing-evidence')
  piece=dict(schemaVersion=2,mode=mode,entry=entry,expectedRows=expected,rows=rows,**{'pass':all(r['pass'] for r in rows)})
  data=raw(piece);need(len(data)<=131072,'resource-limit')
  # Structural budget is checked before exposing a shard as archivable.
  load_json(data,Limits(bytes=131072,depth=16,nodes=16384,string=4096,strings=131072,array=512))
  output[f'python-ingress-{mode}-{entry}.json']=piece
 return output

def controller_entry_inputs(root,spec):
 """Materialize shared-reader positives from real run evidence, read-only.

 A legitimate cross rejection has no decoded artifact. Only then a validated
 sender package may serve as the Python schema-reader positive control; source
 is explicit and never implies that the receiver decoder ran or accepted.
 """
 import bounded as b
 from controller import validate_scope
 from process import validate_exit
 root=b.no_symlink(root)
 context=b.load_json(b.read_file(root/'run-context.json',131072),Limits(bytes=131072,depth=8,nodes=4096,string=256,array=64))
 need(set(context)=={'schemaVersion','toolchain','scope'} and context['schemaVersion']==2,'missing-evidence')
 guard=b.read_file(root/'guard-evidence.json',65536);validate_scope(context['scope'],guard)
 first=spec['registry']['cases'][0];capture_path=root/'round-1'/(first['id']+'.capture.json')
 capture_bytes=b.read_file(capture_path,131072);capture=b.validate_capture(capture_bytes)
 need(capture['caseID']==first['id'] and capture['fixtureValidity']=='valid','missing-evidence')
 point=next((p for p in capture['points'] if p['key']=='seed'),None);need(point is not None,'missing-evidence')
 observations={o['layer']:o for o in point['observations']}
 need(set(observations)==set(b.LAYERS),'missing-evidence')
 def reference(ref,limit=262144):
  need(type(ref) is dict and set(ref)=={'name','bytes','sha256'},'missing-evidence')
  name=ref['name'];need(type(name) is str and not Path(name).is_absolute() and '..' not in Path(name).parts and len(name)<=160,'missing-evidence')
  data=b.read_file(root/name,limit);need(type(ref['bytes']) is int and ref['bytes']==len(data) and ref['sha256']==b.sha(data),'missing-evidence');return data
 snapshots={};origins={}
 for layer,o in observations.items():
  need(o['rawState']=='schema-valid','missing-evidence');ref=dict(o['artifact']);ref['name']='round-1/'+ref['name'];data=reference(ref)
  b.read_snapshot(data,spec,layer,export=layer.endswith('-export'));snapshots[layer]=data;origins[layer]='archived-observation'
 def cross_candidate(case,point_key,side,required=False):
  ident=f'r1-{case["id"]}-{point_key}-{side}-cross';path=root/'receipts'/(ident+'.json')
  if not path.exists():need(not required,'missing-evidence');return None
  receipt=b.load_json(b.read_file(path,131072),Limits(bytes=131072,depth=12,nodes=8192,string=256,array=64))
  need(receipt['id']==ident and receipt['mode']=='cross' and receipt['side']==side and receipt['metadata']=={'round':1,'caseID':case['id'],'pointKey':point_key},'missing-evidence')
  need(not receipt['transportError'],'missing-evidence')
  base=f'round-1/work-{case["id"]}/{point_key}-cross-{side}/'
  refs={a['name']:a for a in receipt['artifacts']};need(len(refs)==len(receipt['artifacts']) and base+'result.json' in refs,'missing-evidence')
  protocol=b.load_json(reference(refs[base+'result.json'],131072),Limits(bytes=131072,depth=16,nodes=8192,string=4096,array=64))
  outcome=validate_exit('cross',side,receipt['returncode'],protocol);need(receipt['outcome']==outcome and outcome in ('success','controlled-rejection'),'missing-evidence')
  sender='go' if side=='swift' else 'swift'
  need(len(receipt['inputs'])==1 and receipt['inputs'][0]['name']==f'round-1/{case["id"]}/{point_key}/{sender}-package.json','missing-evidence');reference(receipt['inputs'][0])
  if protocol['accepted'] is not True:return None
  need(base+'decoded.json' in refs,'missing-evidence');data=reference(refs[base+'decoded.json']);b.read_snapshot(data,spec,'cross-'+side);return data
 for side,sender in (('swift','go'),('go','swift')):
  decoded=cross_candidate(first,'seed',side,True)
  if decoded is None:
   for case in spec['registry']['cases']:
    for key in ['seed']+[c['commandKey'] for c in case['steps']]:
     if case['id']==first['id'] and key=='seed':continue
     decoded=cross_candidate(case,key,side)
     if decoded is not None:break
    if decoded is not None:break
  entry='cross-'+side
  snapshots[entry]=decoded if decoded is not None else snapshots[sender+'-package']
  origins[entry]='cross-decoded' if decoded is not None else 'sender-package-fallback'
  b.read_snapshot(snapshots[entry],spec,entry)
 return dict(specdir=Path(__file__).parent/'spec',registry=spec,capture=capture_bytes,scope=context['scope'],tool_manifest=Path(__file__).parent/'tool-manifest.json',work=root,guard_raw=guard,snapshot_data=snapshots,snapshot_origins=origins)

def controller_entry_index(matrix,shards):
 """Pure index construction; caller writes each exact raw(piece) with budget."""
 mode=matrix['mode'];expected=list(PY_ENTRY_ALL)
 need(set(shards)=={f'python-ingress-{mode}-{entry}.json' for entry in expected},'missing-evidence')
 refs=[dict(entry=entry,name=f'python-ingress-{mode}-{entry}.json',bytes=len(raw(shards[f'python-ingress-{mode}-{entry}.json'])),sha256=sha(raw(shards[f'python-ingress-{mode}-{entry}.json']))) for entry in expected]
 return dict(schemaVersion=2,mode=mode,expectedEntries=expected,shards=refs,**{'pass':matrix['pass']})

def verify_controller_entries(root,spec):
 """Re-run Python readers on original evidence and compare all fixed shards."""
 import bounded as b
 inputs=controller_entry_inputs(root,spec);root=Path(root)
 modes={}
 for mode in ('normal','bypass','late'):
  computed=controller_entry_matrix(**inputs,mode=mode);pieces=controller_entry_shards(computed);wanted=controller_entry_index(computed,pieces)
  index=b.load_json(b.read_file(root/f'python-ingress-{mode}.json',131072),Limits(bytes=131072,depth=8,nodes=4096,string=256,array=64))
  need(index==wanted,'missing-evidence')
  for ref in index['shards']:
   data=b.read_file(root/ref['name'],131072);need(len(data)==ref['bytes'] and b.sha(data)==ref['sha256'],'missing-evidence')
   value=b.load_json(data,Limits(bytes=131072,depth=16,nodes=16384,string=4096,strings=131072,array=512));need(value==pieces[ref['name']],'missing-evidence')
  if mode=='normal':need(computed['pass'],'missing-evidence')
  else:
   need(not computed['pass'],'missing-evidence')
   need(all(not row['pass'] for row in computed['rows'] if row['dimension']=='positive'),'missing-evidence')
  modes[mode]=dict(rows=len(computed['rows']),**{'pass':computed['pass']})
 return dict(schemaVersion=2,modes=modes,**{'pass':True})
