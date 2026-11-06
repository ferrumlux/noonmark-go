# SPDX-License-Identifier: AGPL-3.0-only
"""Fail-closed bounded inputs and raw-first evidence archive. No domain decoder here."""
from dataclasses import dataclass
from pathlib import Path
import hashlib, json, math, os, re, stat, struct

class Rejected(ValueError):
 def __init__(self,reason='schema-invalid'): self.reason=reason;super().__init__(reason)
def need(ok,reason='schema-invalid'):
 if not ok:raise Rejected(reason)
def sha(raw):return hashlib.sha256(raw).hexdigest()
@dataclass(frozen=True)
class Limits:
 bytes:int=262144
 depth:int=24
 nodes:int=8192
 string:int=4096
 strings:int=65536
 array:int=96
 members:int=32

def scan_json(raw,limits=Limits()):
 """Token-only pass before json.loads. Counts object keys as nodes; root depth one."""
 need(type(raw) is bytes and len(raw)<=limits.bytes,'resource-limit')
 try:s=raw.decode('utf-8','strict')
 except UnicodeError:raise Rejected()
 n=len(s);i=0;nodes=0;strings=0
 def ws():
  nonlocal i
  while i<n and s[i] in ' \t\r\n':i+=1
 def node():
  nonlocal nodes
  nodes+=1;need(nodes<=limits.nodes,'resource-limit')
 def string():
  nonlocal i,strings
  start=i;need(i<n and s[i]=='"');i+=1;size=0
  while i<n and s[i]!='"':
   c=s[i];i+=1
   if c=='\\':
    need(i<n);c=s[i];i+=1
    if c=='u':
     h=s[i:i+4];need(len(h)==4 and re.fullmatch('[0-9a-fA-F]{4}',h));i+=4;v=int(h,16)
     if 0xD800<=v<=0xDBFF:
      need(s[i:i+2]=='\\u');h=s[i+2:i+6];need(len(h)==4 and re.fullmatch('[0-9a-fA-F]{4}',h));w=int(h,16);need(0xDC00<=w<=0xDFFF);i+=6;v=0x10000+((v-0xD800)<<10)+w-0xDC00
     else:need(not 0xDC00<=v<=0xDFFF)
     size+=len(chr(v).encode('utf-8'))
    else:need(c in '"\\/bfnrt');size+=1
   else:need(ord(c)>=32);size+=len(c.encode('utf-8'))
   need(size<=limits.string,'resource-limit')
  need(i<n);i+=1;strings+=size;need(strings<=limits.strings,'resource-limit')
  return json.loads(s[start:i])
 def value(depth):
  nonlocal i
  need(depth<=limits.depth,'resource-limit');node();ws();need(i<n);c=s[i]
  if c=='"':string();return
  if c in '[{':
   obj=c=='{';end='}' if obj else ']';i+=1;ws();count=0;keys=set()
   if i<n and s[i]==end:i+=1;return
   while True:
    count+=1;need(count<=(limits.members if obj else limits.array),'resource-limit')
    if obj:
     node();k=string();need(k not in keys);keys.add(k);ws();need(i<n and s[i]==':');i+=1
    value(depth+1);ws();need(i<n)
    if s[i]==end:i+=1;return
    need(s[i]==',');i+=1;ws()
  for token in ('true','false','null'):
   if s.startswith(token,i):i+=len(token);return
  m=re.match(r'-?(?:0|[1-9][0-9]*)',s[i:]);need(m is not None);token=m.group();need(token!='-0');need(len(token)<=20);v=int(token);need(-(1<<63)<=v<(1<<64));i+=len(token)
 value(1);ws();need(i==n)
 return s

_TRACE=None
class EntryTrace:
 """Sealed test context observes the same normal-path wrappers, not fake zeros."""
 def __init__(self,entry,dimension,mode,mutate):
  self.entry=entry;self.dimension=dimension;self.mode=mode;self.mutate=mutate;self.reached=False;self.events=[];self.inputBytes=0;self.inputSHA256='';self.counters={k:0 for k in ('materialize','schema','compare','process','archive')}
 def __enter__(self):
  global _TRACE
  need(_TRACE is None);_TRACE=self;return self
 def __exit__(self,*args):
  global _TRACE
  _TRACE=None
 def operation(self,name):
  if self.reached:
   self.events.append(name)
   if name in self.counters:self.counters[name]+=1

def operation(name):
 if _TRACE is not None:_TRACE.operation(name)

def load_json(raw,limits=Limits(),*,entry='internal'):
 trace=_TRACE
 targeted=trace is not None and not trace.reached and trace.entry==entry
 if targeted:
  raw=trace.mutate(raw,limits,trace.dimension);trace.reached=True;trace.inputBytes=len(raw);trace.inputSHA256=sha(raw)
 def materialize(value):
  operation('materialize')
  try:return json.loads(value)
  except (ValueError,UnicodeError):raise Rejected('schema-invalid')
 def scan():
  operation('scan')
  try:value=scan_json(raw,limits)
  except Rejected:operation('reject');raise
  operation('scan-pass');return value
 if targeted and trace.mode=='bypass':return materialize(raw)
 if targeted and trace.mode=='late':
  value=materialize(raw);scan();return value
 return materialize(scan())


PINNED={'Design.md':'dc6fea57ba43e19fa8f2dfdf742fa1ea17e32fc711daa45ad80b910ab7f541a2','FieldSchema-R2.json':'7a0940e9a1fbc7b847aaa7ea059042e008faaac74b132b8300d0fbb35341d07c','FixtureSchema.md':'51f99d6b21b8df13891d50da9be986dc7af77227ed5cdd186702119a9eb7c00b','Matrix.md':'b1b9e531417a3fbfb5a1a0267d9c17bcdaeb25ea21b28c9422839522349a5719','NoteVectors-R2.json':'8651e7dab7dbb359202c0d84d8ad6e02010f4b5e7e8f280208105d9a33c48a5e','SOURCE-SHA256.tsv':'a62dc8acf664b70eb0f8d92fc16f9923a5e1bb80ba5dab9f79a518137b43fc65','SOURCE-VERIFICATION.json':'23aedd34bbdc31041b68a9b6d6f7b15cec1b0cbc4636a06d32ac32b7c857c1ef','SeedRegistry-R2.json':'b15a50ddaa5315913aceb3d2b14bb823caf57580c7dd8e9c2db32bb71914032a','SourceManifest-R2.json':'eaa83e76297691c122fb018629df44d4659dd0bd1937d092e805b59dfdfe8d35','r2-seeds/empty.json':'9a7523eca7c2938824edaf548c44af04e8d96587c49bc53d06c09c57c066da65','r2-seeds/multi-interleaved.json':'4e3159d88491d994a2410972513c6312258609d8ce3ad89aceed28c5585120da','r2-seeds/planned-12.json':'aecba81054e72fbdf749c928cbd5d8346c856c258e3e718f30a6f0ff71c2decb','r2-seeds/planned-21.json':'0a9c83113057da75914e27425988abd102a3218e7679c7bed0517a755e9eda18'}
def no_symlink(path):
 p=Path(path).absolute()
 for x in [p,*p.parents]:need(not x.is_symlink(),'source-mismatch')
 return p
def read_file(path,limit):
 p=no_symlink(path)
 fd=os.open(p,os.O_RDONLY|os.O_NOFOLLOW)
 try:
  need(stat.S_ISREG(os.fstat(fd).st_mode),'source-mismatch');need(os.fstat(fd).st_size<=limit,'resource-limit')
  with os.fdopen(fd,'rb',closefd=False) as f:raw=f.read(limit+1)
  need(len(raw)<=limit,'resource-limit');return raw
 finally:os.close(fd)
# Parsed fingerprints authenticate frozen input semantics after token admission.
# This never rewrites observed serializer bytes or array order.
FROZEN_PARSED={'FieldSchema-R2.json': '80f44b86f06abe562cd86e7dfc2d1adc8e9f9ccc2a717eb54b68bc937c8d9d62', 'SeedRegistry-R2.json': '14e7a2c19a251c6af6936160f62dec099c47c4639f0277f1d78baf1cca4bbe37', 'NoteVectors-R2.json': '0636544e9a19a93249429dc8951084e4661ea794cb05ba8212f1ec528d33a2da', 'SourceManifest-R2.json': 'd2b6c983c2b8049fa8470c9c41f03e20e85c567faf6b9a895eec1a5cb88e7c98', 'r2-seeds/empty.json': '4aca6b890282fe201032a7c2cd3709d8f0ee23f962272f62eadc14b331ec59f7', 'r2-seeds/planned-12.json': '2cafb6df2651f7a3c108a8e7503b2d5ed15cd71139e7c05eb5dc1a4ce81cd1e4', 'r2-seeds/planned-21.json': '38f7f2e02a6cfd61afd314add57879bb1f41c58b5152f8f1c72f25a888fce375', 'r2-seeds/multi-interleaved.json': '40b37e4d71c7a1b29ad2fb142eddd3a5d2e042a4de0391d7820b1fc11e9395bd'}
def frozen_parsed(name,value):
 operation('schema')
 need(sha(json.dumps(value,sort_keys=True,separators=(',',':'),ensure_ascii=True).encode())==FROZEN_PARSED[name],'source-mismatch')
 return value

def load_spec(root):
 root=no_symlink(root);data={}
 for name,digest in PINNED.items():
  raw=read_file(root/name,262144);need(sha(raw)==digest,'source-mismatch');data[name]=raw
 schema=load_json(data['FieldSchema-R2.json'],Limits(bytes=65536,depth=8,nodes=4096,string=256,strings=49152,array=19),entry='field-schema')
 frozen_parsed('FieldSchema-R2.json',schema)
 need(len(schema['objects'])==17 and sum(map(len,schema['objects'].values()))==117)
 registry=load_json(data['SeedRegistry-R2.json'],Limits(bytes=65536,depth=12,nodes=4096,string=8192,strings=32768,array=18),entry='registry')
 vectors=load_json(data['NoteVectors-R2.json'],Limits(bytes=16384,depth=6,nodes=256,string=8192,strings=16384,array=11),entry='vectors')
 manifest=load_json(data['SourceManifest-R2.json'],Limits(bytes=131072,depth=8,nodes=4096,string=256,array=64),entry='source-manifest')
 frozen_parsed('SeedRegistry-R2.json',registry);frozen_parsed('NoteVectors-R2.json',vectors);frozen_parsed('SourceManifest-R2.json',manifest)
 need(len(registry['seeds'])==4 and len(registry['cases'])==18 and sum(1+len(c['steps']) for c in registry['cases'])==32)
 seeds={x['id']:frozen_parsed(x['file'],load_json(data[x['file']],entry='seed-'+x['id'])) for x in registry['seeds']}
 return dict(schema=schema,registry=registry,registry_sha=PINNED['SeedRegistry-R2.json'],vectors=vectors,manifest=manifest,seeds=seeds,preferences=seeds['empty']['snapshot']['preferences'])
def invocation(raw,spec):
 x=load_json(raw,Limits(bytes=1024,depth=4,nodes=16,string=80,strings=1024,array=0),entry='stdin')
 need(type(x) is dict and set(x)=={'schemaVersion','registrySHA256','caseID'})
 need(type(x['schemaVersion']) is int and x['schemaVersion']==2 and x['registrySHA256']==spec['registry_sha'],'registry-mismatch')
 cases=[c for c in spec['registry']['cases'] if c['id']==x['caseID']];need(len(cases)==1,'registry-mismatch');return cases[0]

def validate_envelope(x,schema,preferences,export=False):
 operation('schema');entities=0
 def visit(v,t,path):
  nonlocal entities
  if t in schema['aliases']:return visit(v,schema['aliases'][t],path)
  if t.startswith('[]') or t.startswith('pairs:'):
   need(type(v) is list);limit=2 if path.endswith('.plannedSubtasks') else 1 if path.endswith('.noteEntries') else 16
   need(len(v)<=limit,'resource-limit')
   if t.startswith('pairs:'):need(len(v)%2==0);need(not v,'profile-violation');return
   for item in v:visit(item,t[2:],path+'[]')
   return
  if t in schema['objects']:
   need(type(v) is dict);fields=schema['objects'][t];need(set(v)<=set(f['wire'] for f in fields))
   if t in ('Day','Chain','Definition','Trace','Subtask','PlannedSubtask','Note'):entities+=1;need(entities<=96,'resource-limit')
   for f in fields:
    k=f['wire']
    if k not in v:need(f['optional']);continue
    m=f['m1'];z=v[k];need(z is not None)
    if m.startswith('absent-only') and k!='descriptionText':raise Rejected('profile-violation')
    if m=='empty-array-only':need(type(z) is list and not z,'profile-violation')
    elif m in ('active','pending'):need(z==m,'profile-violation')
    elif m in ('0','1'):need(type(z) is int and z==int(m),'profile-violation')
    visit(z,f['type'],path+'.'+k)
   return
  if t=='string':need(type(v) is str and len(v.encode('utf-8'))<=4096);return
  if t=='bool':need(type(v) is bool);return
  if t in ('int','uint','stamp'):
   need(type(v) is int);need((-(1<<63) if t=='int' else 0)<=v<(1<<63 if t=='int' else 1<<64))
   if t=='stamp':need(math.isfinite(struct.unpack('>d',v.to_bytes(8,'big'))[0]))
   if path.split('.')[-1] in ('year','month','day'):need(-(1<<31)<=v<(1<<31))
   return
  if t=='uuid':need(type(v) is str and re.fullmatch('[0-9A-F]{8}(?:-[0-9A-F]{4}){3}-[0-9A-F]{12}',v));return
  if t.startswith('enum:'):need(type(v) is str and v in t[5:].split(','));return
  if t.startswith('enumint:'):need(type(v) is int and v in map(int,t[8:].split(',')));return
  raise Rejected('profile-violation')
 visit(x,'Envelope','');need(x['formatVersion']==7)
 expected=json.loads(json.dumps(preferences))
 if export:expected['localFirstSyncPolicy']['enabled']=False
 need(x['snapshot']['preferences']==expected,'profile-violation')
 return x

def predict(value):
 nodes=0;strings=0
 def walk(v,depth):
  nonlocal nodes,strings
  need(depth<=24,'resource-limit');nodes+=1
  if type(v) is dict:
   need(len(v)<=32,'resource-limit')
   for k,z in v.items():nodes+=1;strings+=len(k.encode('utf-8'));walk(z,depth+1)
  elif type(v) is list:
   need(len(v)<=96,'resource-limit')
   for z in v:walk(z,depth+1)
  elif type(v) is str:need(len(v.encode('utf-8'))<=4096,'resource-limit');strings+=len(v.encode('utf-8'))
  need(nodes<=8192 and strings<=65536,'resource-limit')
 walk(value,1);upper=128*nodes+6*strings+2;need(upper<=262144,'resource-limit');return upper

LAYERS=('swift-engine-projection','go-kernel-projection','go-from-domain-projection','swift-package','go-package','swift-export','go-export','swift-self-reencoded','go-self-reencoded')
class Archive:
 def __init__(self,root,cases,event_sink=None):
  self.event_sink=event_sink
  self.root=no_symlink(root);need(self.root.is_dir());self.cases=set(cases);self.counts={};self.sizes={};self.total=0;self.diagnostics={}
 def event(self,name):
  if self.event_sink is not None:self.event_sink(name)
 def reserve(self,case,amount,count=1):
  need(case in self.cases and type(amount) is int and amount>=0);need(self.counts.get(case,0)+count<=18 and self.sizes.get(case,0)+amount<=4*1024*1024 and self.total+amount<=80*1024*1024,'resource-limit')
 def diagnose(self,case,point,layer,count):
  n=self.diagnostics.get(case,0)
  need(n<18,'resource-limit')
  raw=json.dumps(dict(point=point,side=layer.split('-')[0],layer=layer,actualBytes=count,limitBytes=262144,reason='resource-limit'),separators=(',',':')).encode()
  need(len(raw)<=1024 and self.total+len(raw)<=80*1024*1024,'resource-limit')
  p=self.root/f'diagnostic-{case}-{n}.json'
  try:
   with open(no_symlink(p),'xb') as f:f.write(raw)
  except OSError:raise Rejected('missing-evidence')
  self.total+=len(raw);self.diagnostics[case]=n+1;self.event('diagnostic')
 def save(self,case,point,layer,raw,validator):
  self.event('attempt')
  need(point in ('seed','create-one','schedule-one') and layer in LAYERS and case in self.cases)
  need(re.fullmatch('[a-z][a-z0-9-]{0,63}',case) and type(raw) is bytes)
  try:
   need(len(raw)<=262144,'resource-limit');self.reserve(case,len(raw));self.event('reserved')
  except Rejected:
   self.event('rejected')
   self.diagnose(case,point,layer,len(raw));raise
  name=f'{case}/{point}/{layer}.json';p=self.root/name
  for parent in (p.parent.parent,p.parent):
   if not parent.exists():parent.mkdir()
   no_symlink(parent)
  operation('archive')
  try:fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
  except OSError:raise Rejected('missing-evidence')
  try:
   with os.fdopen(fd,'wb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
  except OSError:raise Rejected('missing-evidence')
  self.counts[case]=self.counts.get(case,0)+1;self.sizes[case]=self.sizes.get(case,0)+len(raw);self.total+=len(raw);self.event('written')
  obs={'layer':layer,'artifact':{'name':name,'bytes':len(raw),'sha256':sha(raw)},'rawState':'archived-unchecked','reason':'none'}
  self.event('qualify')
  try:validator(raw);obs['rawState']='schema-valid';self.event('qualified')
  except Rejected as e:obs['rawState']='malformed-raw';obs['reason']=e.reason;self.event('malformed')
  return obs

REASONS={'none','resource-limit','schema-invalid','registry-mismatch','invalid-fixture','profile-violation','domain-rejected','codec-rejected','mapping-ambiguous','dependency-failed','missing-evidence','timeout','source-mismatch','not-applicable'}
VALIDITY={'pending','valid','invalid-fixture','unsupported','harness-error'}
def validate_capture(raw):
 """Closed wire capture schema; statistics still recomputed by comparator."""
 x=load_json(raw,Limits(bytes=131072,depth=16,nodes=8192,string=256,strings=65536,array=64),entry='capture');operation('schema')
 def keys(v,k):need(type(v) is dict and set(v)==set(k.split()))
 def pat(v,p):need(type(v) is str and re.fullmatch(p,v))
 def digest(v,empty=False):pat(v,'(?:[0-9a-f]{64})?' if empty else '[0-9a-f]{64}')
 keys(x,'schemaVersion caseID registrySHA256 sourceManifestSHA256 toolchain scope fixtureValidity points summary');need(x['schemaVersion']==2 and type(x['schemaVersion']) is int);pat(x['caseID'],'[a-z][a-z0-9-]{0,63}')
 digest(x['registrySHA256']);digest(x['sourceManifestSHA256']);need(x['fixtureValidity'] in VALIDITY)
 tc=x['toolchain'];keys(tc,'swiftVersion goVersion xcodeVersion sdkVersion osVersion architecture target locale timezone runnerCommit runnerSHA256 harnessSwiftSHA256 harnessGoSHA256 comparatorSHA256 compiledSourceManifestSHA256')
 for k,v in tc.items():
  need(type(v) is str and 0<len(v.encode())<=256)
  if k.endswith('SHA256'):digest(v)
 pat(tc['runnerCommit'],'[0-9a-f]{40}');need(len(tc['target'].encode())<=80 and tc['architecture'] in ('arm64','x86_64') and tc['locale']=='en_US.UTF-8' and tc['timezone']=='UTC')
 scope=x['scope'];keys(scope,'productionDataAccess iCloudAccess keychainAccess aiNetworkAccess actualAppExecuted selectedStorageSources guardEvidenceSHA256')
 for k,v in scope.items():
  if k.endswith('SHA256'):digest(v)
  else:need(type(v) is bool)
 need(type(x['points']) is list and len(x['points'])<=2)
 for p in x['points']:
  keys(p,'key stages bindings observations comparisons');need(p['key'] in ('seed','create-one','schedule-one'))
  need(type(p['stages']) is list and len(p['stages'])<=32)
  for s in p['stages']:
   keys(s,'side name outcome reason');need(s['side'] in ('swift','go','controller') and s['reason'] in REASONS)
   need(s['outcome'] in ('success','rejected','unsupported','not-run','harness-error','not-applicable'))
  need(type(p['bindings']) is list and len(p['bindings'])<=64)
  for b in p['bindings']:
   keys(b,'kind key swiftID goID origin rule ownerKey lineageKey');need(b['kind'] in ('day','chain','definition','planned','lineage','trace','subtask','note'))
   for k in ('swiftID','goID'):pat(b[k],'[0-9A-F]{8}(?:-[0-9A-F]{4}){3}-[0-9A-F]{12}')
   pat(b['key'],'[a-z][a-z0-9-]{0,79}')
   for k in ('ownerKey','lineageKey'):pat(b[k],'(?:[a-z][a-z0-9-]{0,79})?')
   need(b['origin'] in ('seed','create-one','schedule-one') and b['rule'] in ('seed-id','returned-chain','new-owner-definition','new-owner-note','returned-trace','date-day','trace-lineage'))
  need(type(p['observations']) is list and len(p['observations'])<=9)
  for o in p['observations']:
   keys(o,'layer artifact rawState reason');need(o['layer'] in LAYERS and o['rawState'] in ('archived-unchecked','schema-valid','malformed-raw') and o['reason'] in REASONS)
   a=o['artifact'];keys(a,'name bytes sha256');need(a['name']==f"{x['caseID']}/{p['key']}/{o['layer']}.json" and len(a['name'].encode())<=160)
   need(type(a['bytes']) is int and 0<=a['bytes']<=262144);digest(a['sha256'])
  need(type(p['comparisons']) is list and len(p['comparisons'])==7)
  for c in p['comparisons']:
   keys(c,'layer verdict reason firstPath firstOffset leftSHA256 rightSHA256');need(c['verdict'] in ('equal','different','rejected','not-comparable','unsupported','not-run','harness-error') and c['reason'] in REASONS)
   need(type(c['firstPath']) is str and len(c['firstPath'].encode())<=256 and type(c['firstOffset']) is int and -1<=c['firstOffset']<=262144)
   digest(c['leftSHA256'],True);digest(c['rightSHA256'],True)
 summary=x['summary'];keys(summary,'fixtureValidity mapping effectivePointCount interopPointCount negativeGatePassCount reportComplete observationAccepted interopAccepted')
 need(summary['fixtureValidity'] in VALIDITY and summary['mapping'] in ('complete','ambiguous','not-run'))
 for k in ('effectivePointCount','interopPointCount','negativeGatePassCount'):need(type(summary[k]) is int and 0<=summary[k]<=32)
 for k in ('reportComplete','observationAccepted','interopAccepted'):need(type(summary[k]) is bool)
 return x


def read_snapshot(raw,spec,entry,export=False):
 return validate_envelope(load_json(raw,entry=entry),spec['schema'],spec['preferences'],export=export)
