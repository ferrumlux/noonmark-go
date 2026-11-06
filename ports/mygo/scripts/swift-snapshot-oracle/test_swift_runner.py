# SPDX-License-Identifier: AGPL-3.0-only
"""Host-independent runner contracts. These tests never claim Swift execution."""
import importlib.util
from pathlib import Path
import tempfile
import unittest
HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('snapshot_runner', HERE/'runner.py')
r = importlib.util.module_from_spec(spec)
spec.loader.exec_module(r)
class Contracts(unittest.TestCase):
    def test_no_allow_default(self):
        policy = r.sandbox_policy(Path('/tmp/isolated'), '/Applications/Xcode_26.2.app/Contents/Developer', False)
        self.assertIn('(deny default)', policy)
        self.assertNotIn('(allow default)', policy)
        self.assertIn('(deny network*)', policy)
        self.assertNotIn('(subpath "/Users")', policy)
    def test_path_escape_rejected(self):
        for path in ('/tmp/a"b', '/tmp/a\nb', '/tmp/a\\b'):
            with self.assertRaises(r.GuardError): r.sandbox_policy(Path(path), '/sdk', False)
    def test_overlapping_paths_rejected(self):
        with self.assertRaises(r.GuardError): r.check_separate([Path('/tmp/a'),Path('/tmp/a/b')])
    def test_bounded_subprocess(self):
        import sys
        with tempfile.TemporaryDirectory() as p:
            with self.assertRaises(r.GuardError):
                r.bounded_run([sys.executable,'-c','import sys;sys.stdout.write("x"*131073)'],Path(p),{},2)
            with self.assertRaises(r.GuardError):
                r.bounded_run([sys.executable,'-c','import sys;sys.stderr.write("x"*1048577)'],Path(p),{},2)
    def test_tool_manifest_missing_duplicate_extra_and_changed(self):
        import json,hashlib
        old_repo,old_here=r.REPO,r.HERE
        try:
            with tempfile.TemporaryDirectory() as d:
                root=Path(d);r.REPO=root;r.HERE=root/'tools';r.HERE.mkdir()
                workflow=root/'.github/workflows/swift-snapshot-oracle.yml';workflow.parent.mkdir(parents=True)
                workflow.write_text('permissions: {contents: read}\n')
                (r.HERE/'probe.swift').write_text('// synthetic contract fixture\n')
                rows=[]
                for p in (workflow,r.HERE/'probe.swift'):
                    raw=p.read_bytes();rows.append(dict(path=str(p.relative_to(root)),role='tool',gitBlob='',sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw)))
                manifest=r.HERE/'tool-manifest.json'
                def write(rows):manifest.write_text(json.dumps(dict(schemaVersion=2,files=rows)))
                write(rows);r.validate_tool_manifest(manifest)
                for bad in (rows[:-1],rows+[rows[0]],rows+[dict(rows[0],path='unexpected')]):
                    write(bad)
                    with self.assertRaises((r.GuardError,ValueError)):r.validate_tool_manifest(manifest)
                for index,row in enumerate(rows):
                    wrong=dict(row,sha256=('0' if row['sha256'][0]!='0' else '1')+row['sha256'][1:])
                    write(rows[:index]+[wrong]+rows[index+1:])
                    with self.assertRaises(r.GuardError):r.validate_tool_manifest(manifest)
        finally:r.REPO,r.HERE=old_repo,old_here
    def test_all_19_frozen_source_rows_hash_gate(self):
        import json,hashlib
        manifest=json.loads((HERE/'spec/SourceManifest-R2.json').read_text())
        self.assertEqual(len(manifest['files']),19)
        self.assertEqual(sum(x['role']=='compiled' for x in manifest['files']),16)
        # Synthetic bytes test the shared hash gate, not upstream compilation.
        with tempfile.TemporaryDirectory() as d:
            for row in manifest['files']:
                p=Path(d)/row['path'];p.parent.mkdir(parents=True,exist_ok=True)
                raw=row['path'].encode();p.write_bytes(raw);digest=hashlib.sha256(raw).hexdigest()
                self.assertEqual(r.checked(p,digest,4*1024*1024),raw)
                bad=('0' if digest[0]!='0' else '1')+digest[1:]
                with self.assertRaises(r.GuardError):r.checked(p,bad,4*1024*1024)
    def test_actual_serializer_not_projection_encoder(self):
        source=(HERE/'SnapshotOracle.swift').read_text()
        wrapper=(HERE/'MutationGate.swift').read_text()
        self.assertIn('oracleEncode(snapshot)',source)
        self.assertIn('oracleDecode(bytes)',source)
        self.assertIn('ingressEvent("serializer");return try NoonmarkDataPackage.encode(snapshot)',wrapper)
        self.assertIn('ingressEvent("decode");return try NoonmarkDataPackage.decode(data)',wrapper)
        self.assertIn('timeIntervalSinceReferenceDate.bitPattern',(HERE/'DirectProjection.swift').read_text())
        self.assertNotIn('JSONEncoder()',source)
    def test_direct_projection_field_coverage(self):
        import json
        schema=json.loads((HERE/'spec'/'FieldSchema-R2.json').read_text())
        source=(HERE/'DirectProjection.swift').read_text()
        for name,fields in schema['objects'].items():
            for field in fields:
                self.assertIn('"'+field['wire']+'"',source,(name,field['wire']))
if __name__=='__main__': unittest.main()

class FailureAcceptance(unittest.TestCase):
 def test_false_summary_is_nonzero_and_preserved(self):
  import sys,time
  from unittest.mock import patch
  import controller
  result={'summary':{'reportComplete':True,'effectivePointCount':0,'negativeGatePassCount':0,'observationAccepted':False,'interopAccepted':False}}
  with tempfile.TemporaryDirectory() as d:
   work=Path(d)
   with patch.object(r,'i1_require_source_execution_bound',return_value=None),patch.object(sys,'argv',['runner.py']),patch.object(r,'prepare',return_value=(work,[],{}, {},b'[]',time.monotonic()+30)),patch.object(controller,'prepare_go_probe',return_value=([],{})),patch.object(controller,'run_suite',return_value=result),patch.object(r,'prepare_upload') as upload,patch('offline.verify_run',return_value=result):
    code=r.main();self.assertIs(type(code),int);self.assertNotEqual(code,0);self.assertGreaterEqual(upload.call_count,1)

class FailureEvidence(unittest.TestCase):
 def test_compiler_failure_nonzero_and_bounded_bundle(self):
  import os,sys,tarfile,json
  from unittest.mock import patch
  with tempfile.TemporaryDirectory() as d:
   error=r.GuardError('subprocess-rejected');error.diagnostic={'stderr':b'synthetic compiler diagnostic','stdout':b'','stderrBytes':29}
   output=Path(d)/'output'
   # Synthetic downstream failure only. Real unbound main is separately tested.
   with patch.object(r,'i1_require_source_execution_bound',return_value=None),patch.dict(os.environ,{'RUNNER_TEMP':d,'GITHUB_OUTPUT':str(output)}),patch.object(sys,'argv',['runner.py']),patch.object(r,'prepare',side_effect=error):
    self.assertEqual(r.main(),1)
   bundle=Path(output.read_text().strip().split('=',1)[1]);self.assertTrue(bundle.is_file())
   with tarfile.open(bundle) as archive:
    self.assertEqual(archive.extractfile('failure-stderr.bin').read(),b'synthetic compiler diagnostic')
    self.assertFalse(json.load(archive.extractfile('failure.json'))['observationAccepted'])
    self.assertTrue(all(not p.name.startswith('/') and '..' not in Path(p.name).parts for p in archive.getmembers()))
 def test_observation_difference_can_pass_not_interop(self):
  summary={'reportComplete':True,'observationAccepted':True,'interopAccepted':False,'effectivePointCount':32,'negativeGatePassCount':9}
  verified={'summary':summary,'negativeGates':{f'V{i:02}':'pass' for i in range(1,10)}}
  self.assertTrue(r.observation_ready(verified))
  for key,value in [('effectivePointCount',31),('negativeGatePassCount',8),('observationAccepted',False)]:
   changed={'summary':dict(summary,**{key:value}),'negativeGates':verified['negativeGates']}
   self.assertFalse(r.observation_ready(changed))
  verified['negativeGates']['V06']='fail';self.assertFalse(r.observation_ready(verified))

class R3ArchivalContracts(unittest.TestCase):
 """第三轮先红合同：真实本地文件／进程，不是 Swift 互通证据。"""
 def setUp(self):
  r.COMMAND_EVIDENCE.clear();r.RUN_PROVENANCE.clear()
 def test_oversized_result_keeps_reserved_failure_bundle(self):
  import os,tarfile,json
  from unittest.mock import patch
  with tempfile.TemporaryDirectory() as d:
   work=Path(d);target=work/'run/round-1/work-observe-empty/go/result.json'
   target.parent.mkdir(parents=True);target.write_bytes(b'x'*131073)
   r.persist_diagnostic(work,r.GuardError('schema-invalid'))
   with patch.dict(os.environ,{'GITHUB_OUTPUT':str(work/'output')}):status=r.prepare_upload(work)
   self.assertIsInstance(status,dict);self.assertFalse(status['complete'])
   self.assertTrue(status['bundle'].is_file());self.assertEqual(target.stat().st_size,131073)
   excluded=next(x for x in status['exclusions'] if x['path']==str(target.relative_to(work)))
   self.assertEqual(excluded['reason'],'item-limit');self.assertNotIn('sha256',excluded)
   with tarfile.open(status['bundle']) as archive:
    self.assertIn('failure.json',archive.getnames());self.assertIn('budget.json',archive.getnames())
    self.assertNotIn(str(target.relative_to(work)),archive.getnames())
    budget=json.load(archive.extractfile('budget.json'))
    self.assertEqual(budget['bundleBytes'],status['bundle'].stat().st_size)
    self.assertLessEqual(budget['totalBytes'],83886080)
 def test_bounded_malformed_preserved_exactly(self):
  import os,tarfile
  from unittest.mock import patch
  with tempfile.TemporaryDirectory() as d:
   work=Path(d);target=work/'run/round-1/work-observe-empty/go/result.json'
   target.parent.mkdir(parents=True);raw=b'x'*32;target.write_bytes(raw)
   r.persist_diagnostic(work,r.GuardError('schema-invalid'))
   with patch.dict(os.environ,{'GITHUB_OUTPUT':str(work/'output')}):status=r.prepare_upload(work)
   self.assertIsInstance(status,dict)
   with tarfile.open(status['bundle']) as archive:self.assertEqual(archive.extractfile(str(target.relative_to(work))).read(),raw)
 def test_ordinary_cannot_increase_stdout_limit(self):
  import sys
  with tempfile.TemporaryDirectory() as d:
   with self.assertRaises(r.GuardError):r.bounded_run([sys.executable,'-c','print("x"*161192)'],Path(d),{},2,4*1024*1024)
 def test_provenance_has_pure_validation_entry(self):
  self.assertTrue(callable(getattr(r,'verify_provenance',None)))

class R3SourceContracts(unittest.TestCase):
 """同大小合成19项fixture，真实本地git cat-file；不声称官方源／Swift已运行。"""
 def setUp(self):
  r.COMMAND_EVIDENCE.clear();r.RUN_PROVENANCE.clear();r.ACTIVE_WORK=None
 def fixture(self,base):
  import json,os,subprocess,copy
  from contextlib import ExitStack
  from unittest.mock import patch
  import gate_receipts
  work=base/'work';work.mkdir();fixture=base/'fixture';(fixture/'spec').mkdir(parents=True)
  manifest=copy.deepcopy(json.loads((HERE/'spec/SourceManifest-R2.json').read_text()))
  repo=work/'upstream.git';env={'PATH':'/usr/bin:/bin','GIT_CONFIG_NOSYSTEM':'1','GIT_CONFIG_GLOBAL':'/dev/null','GIT_AUTHOR_NAME':'Synthetic fixture','GIT_AUTHOR_EMAIL':'fixture@example.invalid','GIT_COMMITTER_NAME':'Synthetic fixture','GIT_COMMITTER_EMAIL':'fixture@example.invalid','GIT_AUTHOR_DATE':'2020-01-01T00:00:00Z','GIT_COMMITTER_DATE':'2020-01-01T00:00:00Z'}
  def git(*args,input=b''):
   return subprocess.run(['/usr/bin/git','--git-dir='+str(repo),*args],input=input,stdout=subprocess.PIPE,stderr=subprocess.PIPE,env=env,check=True).stdout.strip().decode()
  git('init','--bare');tree={};contents={}
  for i,row in enumerate(manifest['files']):
   size=row['bytes'] if row['bytes'] in (161192,153461) else 32+i
   raw=(b'SYNTHETIC-NOT-SWIFT-'+str(i).encode()+b'\n')*((size//20)+1);raw=(raw+b'x'*size)[:size]
   row.update(bytes=len(raw),sha256=r.sha(raw),gitBlob=git('hash-object','-w','--stdin',input=raw));contents[row['path']]=raw
   node=tree
   for part in row['path'].split('/')[:-1]:node=node.setdefault(part,{})
   node[row['path'].split('/')[-1]]=row['gitBlob']
  def write_tree(node):
   lines=[]
   for name,value in sorted(node.items()):
    kind='tree' if type(value) is dict else 'blob';ident=write_tree(value) if kind=='tree' else value
    lines.append(('040000' if kind=='tree' else '100644')+' '+kind+' '+ident+'\t'+name+'\n')
   return git('mktree',input=''.join(lines).encode())
  root=write_tree(tree);commit=git('commit-tree',root,input=b'Synthetic source contract only\n');manifest['commit']=commit;manifest['rootTree']=root
  manifest['coreTree']=write_tree(tree['Sources']['NoonmarkCore']);manifest['storageTree']=write_tree(tree['Sources']['NoonmarkStorage'])
  raw=json.dumps(manifest).encode();(fixture/'spec/SourceManifest-R2.json').write_bytes(raw)
  (fixture/'tool-manifest.json').write_bytes((HERE/'tool-manifest.json').read_bytes())
  stack=ExitStack();stack.enter_context(patch.object(r,'HERE',fixture));stack.enter_context(patch.object(r,'SOURCE_SHA',r.sha(raw)));stack.enter_context(patch.object(r,'COMMIT',commit));stack.enter_context(patch.object(gate_receipts,'HERE',fixture))
  return stack,work,manifest,contents,env
 def test_two_legal_large_sources_measured_without_raw_provenance_copy(self):
  import json
  import gate_receipts
  with tempfile.TemporaryDirectory() as d:
   stack,work,manifest,contents,env=self.fixture(Path(d))
   with stack:
    for row in manifest['files']:
     if row['bytes'] not in (161192,153461):continue
     raw=r.bounded_run(r.source_command(work,row['path']),work,env,5,4194304,source_path=row['path'])
     self.assertEqual(raw,contents[row['path']])
    r.persist_provenance(work);commands=r.verify_provenance(work,gate_receipts.ensure_run(work/'run'))
    self.assertEqual(sorted(x['stdoutBytes'] for x in commands),[153461,161192])
    for index,command in enumerate(commands):
     self.assertEqual(command['kind'],'frozen-source-object');self.assertGreater(command['pid'],0)
     self.assertEqual(command['stdoutSHA256'],r.sha(contents[command['source']['path']]))
     self.assertTrue(command['stdoutComplete']);self.assertEqual(command['returncode'],0)
     self.assertFalse((work/f'provenance/command-{index}-stdout.bin').exists())
 def test_source_kind_argv_hash_binding_and_extra_fields_fail_closed(self):
  import json,copy
  from unittest.mock import patch
  import gate_receipts
  with tempfile.TemporaryDirectory() as d:
   stack,work,manifest,contents,env=self.fixture(Path(d))
   with stack:
    row=next(x for x in manifest['files'] if x['bytes']==161192)
    bad=r.source_command(work,row['path']);bad[-1]+='-wrong'
    with patch.object(r.subprocess,'Popen',side_effect=AssertionError('must not run')):
     with self.assertRaises(r.GuardError):r.bounded_run(bad,work,env,5,4194304,source_path=row['path'])
    r.bounded_run(r.source_command(work,row['path']),work,env,5,4194304,source_path=row['path']);r.persist_provenance(work)
    binding=gate_receipts.ensure_run(work/'run');path=work/'provenance/index.json';original=json.loads(path.read_bytes())
    changes=[('kind','ordinary-command'),('stdoutSHA256','0'*64),('bindingSHA256','0'*64),('stdoutBytes',1),('extra',True),('argv',bad),('stdoutComplete',False),('returncode',-9)]
    for key,value in changes:
     changed=copy.deepcopy(original);changed['commands'][0][key]=value;path.write_text(json.dumps(changed))
     with self.assertRaises((r.GuardError,ValueError)),patch.object(r.subprocess,'Popen',side_effect=AssertionError('offline must not run')):r.verify_provenance(work,binding)
    path.write_text(json.dumps(original))
    with patch.object(r.subprocess,'Popen',side_effect=AssertionError('offline must not run')),patch.object(Path,'mkdir',side_effect=AssertionError('offline must not write')):
     self.assertEqual(len(r.verify_provenance(work,binding)),1)
 def test_real_git_failure_has_no_complete_source_hash(self):
  import gate_receipts
  with tempfile.TemporaryDirectory() as d:
   stack,work,manifest,contents,env=self.fixture(Path(d))
   with stack:
    row=next(x for x in manifest['files'] if x['bytes']==153461);blob=row['gitBlob']
    (work/'upstream.git/objects'/blob[:2]/blob[2:]).unlink()
    with self.assertRaises(r.GuardError):r.bounded_run(r.source_command(work,row['path']),work,env,5,4194304,source_path=row['path'])
    receipt=r.COMMAND_EVIDENCE[-1];self.assertFalse(receipt['stdoutComplete']);self.assertIsNone(receipt['stdoutSHA256']);self.assertIsNone(receipt['stdoutGitBlob'])
    self.assertEqual(receipt['stdoutBytes'],0);self.assertNotEqual(receipt['returncode'],0)
    r.persist_provenance(work);self.assertFalse(r.verify_provenance(work,gate_receipts.ensure_run(work/'run'))[0]['stdoutComplete'])

class R3PoisonAndBudgetContracts(unittest.TestCase):
 def setUp(self):
  r.COMMAND_EVIDENCE.clear();r.RUN_PROVENANCE.clear();r.ACTIVE_WORK=None
 def test_multiple_poison_paths_and_fifo_do_not_discard_safe_raw(self):
  import os,tarfile
  from unittest.mock import patch
  with tempfile.TemporaryDirectory() as d:
   work=Path(d);base=work/'run/round-1/work-observe-empty';(base/'go').mkdir(parents=True);(base/'swift').mkdir()
   (base/'go/result.json').write_bytes(b'x'*131073);os.mkfifo(base/'swift/result.json')
   poison=work/'run/round-2/work-observe-empty/go';poison.parent.mkdir(parents=True);poison.symlink_to(base/'go',target_is_directory=True)
   (work/'source-gates.json').mkdir();(work/'compile.sb').symlink_to(base/'go/result.json')
   raw=b'malformed-but-bounded';target=work/'run/round-3/work-observe-empty/go/result.json';target.parent.mkdir(parents=True);target.write_bytes(raw)
   r.persist_diagnostic(work,r.GuardError('schema-invalid'))
   with patch.dict(os.environ,{'GITHUB_OUTPUT':str(work/'output')}),patch.object(r,'_artifact_read',wraps=r._artifact_read) as reads:status=r.prepare_upload(work)
   reasons={x['reason'] for x in status['exclusions']};self.assertTrue({'item-limit','nonregular','unsafe-path'}<=reasons);self.assertFalse(status['complete'])
   names=[x.args[1] for x in reads.call_args_list];self.assertEqual(len(names),len(set(names)))
   with tarfile.open(status['bundle']) as archive:self.assertEqual(archive.extractfile(str(target.relative_to(work))).read(),raw)
 def test_normal_tar_failure_uses_reserved_bundle_without_rereading_poison(self):
  import os,tarfile,json
  from unittest.mock import patch
  with tempfile.TemporaryDirectory() as d:
   work=Path(d);target=work/'run/round-1/work-observe-empty/go/result.json';target.parent.mkdir(parents=True);target.write_bytes(b'x'*131073)
   r.persist_diagnostic(work,r.GuardError('schema-invalid'))
   with patch.dict(os.environ,{'GITHUB_OUTPUT':str(work/'output')}),patch.object(r,'_artifact_read',wraps=r._artifact_read) as reads,patch.object(tarfile,'open',side_effect=OSError('synthetic normal tar failure')):
    status=r.prepare_upload(work)
   self.assertTrue(status['budget']['fallback']);self.assertFalse(status['complete']);self.assertTrue(status['bundle'].is_file())
   names=[x.args[1] for x in reads.call_args_list];self.assertEqual(len(names),len(set(names)))
   with tarfile.open(status['bundle']) as archive:
    self.assertIn('failure.json',archive.getnames());self.assertIn('packaging-failure.json',archive.getnames());self.assertIn('budget.json',archive.getnames())
   self.assertLessEqual(status['budget']['totalBytes'],83886080)
 def test_cumulative_and_exact_tar_costs_stay_inside_total(self):
  import os,tarfile,json
  from unittest.mock import patch
  import gate_receipts
  with tempfile.TemporaryDirectory() as d:
   work=Path(d)
   for i in range(1,4):
    target=work/f'run/round-{i}/work-observe-empty/go/result.json';target.parent.mkdir(parents=True);target.write_bytes(b'x'*131072)
   r.persist_diagnostic(work,r.GuardError('schema-invalid'))
   with patch.dict(os.environ,{'GITHUB_OUTPUT':str(work/'output')}),patch.object(gate_receipts,'NORMAL_BYTES',400000),patch.object(gate_receipts,'TOTAL_BYTES',1448576):status=r.prepare_upload(work)
   self.assertIn('total-budget',{x['reason'] for x in status['exclusions']});self.assertFalse(status['complete']);self.assertTrue(status['budget']['fallback']);budget=status['budget']
   self.assertEqual(budget['normalLimit']+budget['diagnosticReserve'],budget['totalLimit'])
   self.assertEqual(budget['totalBytes'],budget['retainedRawBytes']+budget['metadataBytes']+budget['bundleBytes'])
   self.assertEqual(budget['bundleBytes'],sum(budget[k] for k in ('bundlePayloadBytes','tarHeaderBytes','tarPaddingBytes','tarEndBytes','tarRecordPaddingBytes')))
   self.assertEqual(budget['bundleBytes'],status['bundle'].stat().st_size);self.assertLessEqual(budget['totalBytes'],1448576)
   with tarfile.open(status['bundle']) as archive:self.assertEqual(json.load(archive.extractfile('budget.json')),budget)
 def test_changed_during_read_is_not_a_complete_original(self):
  import os
  from unittest.mock import patch
  with tempfile.TemporaryDirectory() as d:
   work=Path(d);path=work/'x';path.write_bytes(b'x'*32);real=os.read;changed=False
   def mutate(fd,amount):
    nonlocal changed
    data=real(fd,amount)
    if not changed:path.write_bytes(b'y'*33);changed=True
    return data
   with patch.object(r.os,'read',side_effect=mutate):data,reason,size=r._artifact_read(work,'x',100)
   self.assertIsNone(data);self.assertEqual(reason,'changed')
 def test_packaging_incomplete_cannot_turn_observation_green(self):
  import sys,time
  from unittest.mock import patch
  import controller
  verified={'summary':{'reportComplete':True,'observationAccepted':True,'interopAccepted':False,'effectivePointCount':32,'negativeGatePassCount':9},'negativeGates':{f'V{i:02}':'pass' for i in range(1,10)}}
  with tempfile.TemporaryDirectory() as d:
   work=Path(d);(work/'run').mkdir()
   with patch.object(r,'i1_require_source_execution_bound',return_value=None),patch.object(sys,'argv',['runner.py']),patch.object(r,'prepare',return_value=(work,[],{}, {},b'[]',time.monotonic()+30)),patch.object(controller,'prepare_go_probe',return_value=([],{})),patch.object(controller,'run_suite',return_value=verified),patch.object(r,'persist_provenance'),patch.object(r,'prepare_upload',return_value={'bundle':None,'exclusions':[{}],'budget':{},'complete':False}),patch('offline.verify_run',return_value=verified):
    self.assertEqual(r.main(),1)

class R3DeclaredBudgetContracts(unittest.TestCase):
 def setUp(self):
  r.COMMAND_EVIDENCE.clear();r.RUN_PROVENANCE.clear();r.ACTIVE_WORK=None
 def test_declared_budget_is_preserved_and_bad_declarations_cannot_expand_it(self):
  import os,tarfile,json
  from unittest.mock import patch
  from gate_receipts import NORMAL_BYTES,TOTAL_BYTES,DIAGNOSTIC_RESERVE
  for declared in (NORMAL_BYTES,83886081,-1,True,'100'):
   with self.subTest(declared=declared),tempfile.TemporaryDirectory() as d:
    work=Path(d);r.persist_diagnostic(work,r.GuardError('schema-invalid'))
    with patch.dict(os.environ,{'GITHUB_OUTPUT':str(work/'output')}):status=r.prepare_upload(work,{'evidenceBytesAccounted':declared})
    budget=status['budget'];self.assertTrue(status['bundle'].is_file());self.assertFalse(status['complete'])
    self.assertLessEqual(budget['totalBytes'],TOTAL_BYTES)
    if type(declared) is int and 0<=declared<=NORMAL_BYTES:self.assertEqual(budget['retainedRawBytes'],declared)
    else:
     self.assertEqual(budget['accounting']['state'],'untrusted');self.assertIsNone(budget['accounting']['declaredEvidenceBytes'])
     self.assertTrue(budget['fallback']);self.assertLessEqual(budget['metadataBytes']+budget['bundleBytes'],DIAGNOSTIC_RESERVE)
    with tarfile.open(status['bundle']) as archive:self.assertIn('budget.json',archive.getnames())
 def test_reserved_package_explicitly_counts_omitted_diagnostics_when_necessary(self):
  from gate_receipts import NORMAL_BYTES,DIAGNOSTIC_RESERVE,TOTAL_BYTES
  exclusions=[{'path':'run/'+str(i)+'-'+'x'*150,'kind':'artifact','limit':131072,'complete':False,'reason':'item-limit','observedBytes':131073} for i in range(5000)]
  payload,budget=r._package_plan({'failure.json':b'{}'},NORMAL_BYTES,exclusions,False,True)
  self.assertGreater(budget['omittedDiagnosticCount'],0);self.assertEqual(budget['excludedCount'],5000)
  self.assertLessEqual(budget['metadataBytes']+budget['bundleBytes'],DIAGNOSTIC_RESERVE);self.assertLessEqual(budget['totalBytes'],TOTAL_BYTES)
  import json
  index=json.loads(payload['exclusions.json']);self.assertEqual(index['count'],5000);self.assertEqual(index['omittedDiagnosticCount'],budget['omittedDiagnosticCount']);self.assertIsNotNone(index['omittedDiagnosticsSHA256'])

class R3PreexistingBundleContract(unittest.TestCase):
 def test_preexisting_oversized_tar_is_poison_not_new_evidence_budget(self):
  import os,tarfile
  from unittest.mock import patch
  r.COMMAND_EVIDENCE.clear();r.RUN_PROVENANCE.clear();r.ACTIVE_WORK=None
  with tempfile.TemporaryDirectory() as d:
   work=Path(d);poison=work/'synthetic-evidence.tar'
   with poison.open('xb') as f:f.truncate(83886081)
   r.persist_diagnostic(work,r.GuardError('schema-invalid'))
   with patch.dict(os.environ,{'GITHUB_OUTPUT':str(work/'output')}):status=r.prepare_upload(work)
   self.assertEqual(poison.stat().st_size,83886081);self.assertTrue(status['budget']['fallback']);self.assertFalse(status['complete'])
   self.assertGreaterEqual(status['budget']['observedExcludedBytes'],83886081);self.assertLessEqual(status['budget']['totalBytes'],83886080)
   with tarfile.open(status['bundle']) as archive:self.assertIn('failure.json',archive.getnames())

class R3SelectedClosureContracts(unittest.TestCase):
 def fixture(self,decoded=True):
  binding='0'*64;selected={}
  def add(name,value):
   data=value if type(value) is bytes else r._json_bytes(value);selected['run/'+name]=data
   return {'name':name,'bytes':len(data),'sha256':r.sha(data)}
  ident='r1-observe-empty-seed-go-cross';raw=add('round-1/work-observe-empty/seed-cross-go/result.json',b'{}')
  artifacts=[raw]
  if decoded:artifacts.append(add('round-1/work-observe-empty/seed-cross-go/decoded.json',b'{}'))
  receipt={'schemaVersion':3,'resourceWitness':{},'requestedTimeoutNs':2000000000,'bindingSHA256':binding,'artifactManifests':[],'id':ident,'mode':'cross','side':'go','metadata':{},'inputs':[],'returncode':0 if decoded else 2,'transportError':'','argv':[],'executionWitness':{},'outcome':'success' if decoded else 'controlled-rejection','reason':'none','stdout':add('receipts/'+ident+'-stdout.bin',b''),'stderr':add('receipts/'+ident+'-stderr.bin',b''),'stderrBytes':0,'artifacts':artifacts,'schemaValidWitness':decoded}
  process=add('receipts/'+ident+'.json',receipt)
  part=add('execution-receipts-part-0.json',{'schemaVersion':3,'bindingSHA256':binding,'items':[process]})
  add('execution-receipts.json',{'schemaVersion':3,'bindingSHA256':binding,'receiptParts':[part],'directionParts':[]})
  return selected
 def test_missing_or_changed_declared_decoded_is_not_complete(self):
  from unittest.mock import patch
  for mode in ('missing','changed'):
   selected=self.fixture();allowed=set(selected);name='run/round-1/work-observe-empty/seed-cross-go/decoded.json'
   if mode=='missing':del selected[name]
   else:selected[name]=b'{ }'
   with patch.object(r,'_artifact_read',side_effect=AssertionError('must not reread')):problems=r.verify_selected_closure(selected,allowed)
   self.assertIn({'path':name,'reason':'reference-missing' if mode=='missing' else 'reference-hash'},problems)
 def test_controlled_rejection_does_not_require_unclaimed_decoded(self):
  selected=self.fixture(decoded=False);self.assertEqual(r.verify_selected_closure(selected,set(selected)),[])
 def test_unknown_reference_does_not_expand_allowlist(self):
  selected=self.fixture();allowed=set(selected)
  name='run/execution-receipts-part-0.json'
  selected[name]=r._json_bytes({'schemaVersion':3,'bindingSHA256':'0'*64,'items':[{'name':'../outside','bytes':1,'sha256':'0'*64}]})
  problems=r.verify_selected_closure(selected,allowed);self.assertIn({'path':name,'reason':'reference-allowlist'},problems);self.assertNotIn('../outside',allowed)
 def test_prepare_upload_checks_closure_before_complete(self):
  import os
  from unittest.mock import patch
  r.COMMAND_EVIDENCE.clear();r.RUN_PROVENANCE.clear();r.ACTIVE_WORK=None
  with tempfile.TemporaryDirectory() as d:
   work=Path(d);selected=self.fixture();missing='run/round-1/work-observe-empty/seed-cross-go/decoded.json';del selected[missing]
   for name,data in selected.items():target=work/name;target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(data)
   r.persist_diagnostic(work,r.GuardError('schema-invalid'))
   with patch.dict(os.environ,{'GITHUB_OUTPUT':str(work/'output')}):status=r.prepare_upload(work)
   self.assertFalse(status['complete']);self.assertIn((missing,'reference-missing'),{(x['path'],x['reason']) for x in status['exclusions']});self.assertTrue(status['bundle'].is_file())

class I1SealedArchiveContracts(unittest.TestCase):
 def test_plan_is_immutable_and_detached_from_caller_dicts(self):
  import json
  payload={'failure.json':b'{}'}
  plan,budget=r._package_plan(payload,2,[],True,False)
  original=plan['failure.json'];payload['failure.json']=b'changed';budget['bundleBytes']=1
  self.assertEqual(plan['failure.json'],original)
  with self.assertRaises(TypeError):plan.members=()
  with self.assertRaises(TypeError):plan.members[0]=('evil',b'evil')
  with tempfile.TemporaryDirectory() as d:
   path=Path(d)/'out.tar';r._write_tar(path,plan)
   self.assertEqual(path.stat().st_size,json.loads(plan.budget_bytes)['bundleBytes'])
 def test_unsealed_dict_never_creates_tar(self):
  with tempfile.TemporaryDirectory() as d:
   path=Path(d)/'out.tar'
   with self.assertRaises(r.GuardError):r._write_tar(path,{'failure.json':b'{}'})
   self.assertFalse(path.exists())
 def test_exact_normal_boundary_and_plus_one_refusal(self):
  import gate_receipts
  from unittest.mock import patch
  payload={'failure.json':b'x'*512}
  # Keep numeric widths fixed while locating the self-accounted boundary.
  normal=100000;total=normal+1048576
  for _ in range(8):
   with patch.object(gate_receipts,'NORMAL_BYTES',normal),patch.object(gate_receipts,'TOTAL_BYTES',total):
    _,budget=r._package_plan(payload,512,[],True,False)
   if normal==budget['totalBytes']:break
   normal=budget['totalBytes'];total=normal+1048576
  with patch.object(gate_receipts,'NORMAL_BYTES',normal),patch.object(gate_receipts,'TOTAL_BYTES',total):
   _,budget=r._package_plan(payload,512,[],True,False);self.assertEqual(budget['totalBytes'],normal)
  with patch.object(gate_receipts,'NORMAL_BYTES',normal-1),patch.object(gate_receipts,'TOTAL_BYTES',total-1):
   with self.assertRaises(r.GuardError):r._package_plan(payload,512,[],True,False)
 def test_payload_hashes_once_across_fixed_point_and_references(self):
  from unittest.mock import patch
  with patch.object(r,'sha',wraps=r.sha) as digest:
   payload=r._Payloads();data=b'UNIQUE-PAYLOAD'*100;payload['failure.json']=data
   for _ in range(20):self.assertEqual(r._payload_digest(payload,'failure.json'),payload.digest('failure.json'))
   r._package_plan(payload,len(data),[],True,False)
   self.assertEqual(sum(c.args[0] is data for c in digest.call_args_list),1)
  with self.assertRaises(r.GuardError):payload['failure.json']=b'replaced'
 def test_padding_and_ustar_size_are_exact(self):
  import tarfile
  for size in (511,512,513,10239,10240,10241):
   with self.subTest(size=size),tempfile.TemporaryDirectory() as d:
    plan,budget=r._package_plan({'raw.bin':b'x'*size},size,[],True,False)
    path=Path(d)/'out.tar';r._write_tar(path,plan)
    self.assertEqual(path.stat().st_size,budget['bundleBytes'])
    with tarfile.open(path) as archive:self.assertEqual(archive.extractfile('raw.bin').read(),b'x'*size)
 def test_ustar_invalid_name_rejected_before_any_writer(self):
  with self.assertRaises((ValueError,r.GuardError)):
   r._package_plan({'x'*300:b'bad'},3,[],True,False)
 def test_unuploaded_raw_retained_and_partial_counted_once(self):
  retained=500000
  plan,budget=r._package_plan({'failure.json':b'{}'},retained,[],False,True)
  self.assertEqual(budget['retainedRawBytes'],retained)
  self.assertEqual(budget['totalBytes'],retained+budget['metadataBytes']+budget['bundleBytes'])
  partial=12345
  _,after=r._package_plan({'failure.json':b'{}'},retained+partial,[],False,True)
  self.assertEqual(after['retainedRawBytes'],retained+partial)
 def test_overwrite_and_overwrite_length_fail_closed(self):
  from unittest.mock import patch
  plan,budget=r._package_plan({'raw.bin':b'123'},3,[],True,False)
  with tempfile.TemporaryDirectory() as d:
   path=Path(d)/'out.tar';path.write_bytes(b'prior')
   with self.assertRaises(FileExistsError):r._write_tar(path,plan)
   self.assertEqual(path.read_bytes(),b'prior')
  with tempfile.TemporaryDirectory() as d:
   path=Path(d)/'out.tar'
   def overrun(**kwargs):
    kwargs['fileobj'].write(b'x'*(budget['bundleBytes']+1))
   with patch('tarfile.open',side_effect=overrun):
    with self.assertRaises(r.GuardError) as ctx:r._write_tar(path,plan)
   self.assertTrue(hasattr(ctx.exception,'created_bundle'));self.assertEqual(path.stat().st_size,0)

 def test_replaced_dataclass_is_not_an_issued_plan(self):
  plan,_=r._package_plan({'raw.bin':b'123'},3,[],True,False)
  forged=r._SealedPlan(plan.members,plan.headers,plan.budget_bytes,plan.fallback,plan.seal)
  with tempfile.TemporaryDirectory() as d:
   path=Path(d)/'out.tar'
   with self.assertRaises(r.GuardError):r._write_tar(path,forged)
   self.assertFalse(path.exists())
 def test_large_exclusion_set_uses_bounded_honest_fallback(self):
  import json
  exclusions=[{'path':'poison-'+str(i),'kind':'artifact','reason':'excluded','limit':131072,'complete':False} for i in range(8192)]
  plan,budget=r._package_plan({'failure.json':b'{}'},2,exclusions,False,True)
  self.assertFalse(budget['complete']);self.assertTrue(budget['fallback'])
  self.assertGreater(budget['omittedDiagnosticCount'],0)
  self.assertEqual(json.loads(plan['exclusions.json'])['count'],8192)

 def test_no_dict_and_object_setattr_cannot_change_approved_state(self):
  import json
  for field in ('members','headers','budget_bytes','fallback','seal'):
   with self.subTest(field=field),tempfile.TemporaryDirectory() as d:
    plan,_=r._package_plan({'raw.bin':b'GOOD'},4,[],True,False)
    with self.assertRaises(AttributeError):plan.__dict__['members']=()
    forged={'members':tuple((n,b'EVIL' if n=='raw.bin' else v) for n,v in plan.members),'headers':tuple(bytes(bytearray(h)) for h in plan.headers),'budget_bytes':bytes(bytearray(plan.budget_bytes)),'fallback':True,'seal':object()}[field]
    object.__setattr__(plan,field,forged)
    target=Path(d)/'must-not-exist.tar'
    with self.assertRaises(r.GuardError):r._write_tar(target,plan)
    self.assertFalse(target.exists())
 def test_actual_fsync_cleanup_and_inode_failures_keep_raw_and_charge_once(self):
  import os,json
  from unittest.mock import patch
  for mode in ('cleaned','unlink-fails','inode-replaced','same-inode-modified'):
   with self.subTest(mode=mode),tempfile.TemporaryDirectory() as d:
    work=Path(d);r.persist_diagnostic(work,r.GuardError('controlled-test-failure'))
    original_failure=(work/'failure.json').read_bytes();real_sync=r.os.fsync;real_unlink=Path.unlink;fired=False
    main=work/'synthetic-evidence.tar';owned_copy=work/'owned-original.tar';written=[]
    def failing_sync(fd):
     nonlocal fired
     if not fired:
      fired=True;written.append(os.fstat(fd).st_size)
      if mode=='inode-replaced':
       main.rename(owned_copy);main.write_bytes(b'P'*written[0])
       return real_sync(fd)
      if mode=='same-inode-modified':
       with main.open('r+b') as changed:changed.write(b'EVIL')
       return real_sync(fd)
      raise OSError('injected fsync failure')
     return real_sync(fd)
    def unlink(path,*args,**kwargs):
     if mode=='unlink-fails' and path==main:raise OSError('injected cleanup failure')
     return real_unlink(path,*args,**kwargs)
    with patch.dict(os.environ,{'GITHUB_OUTPUT':str(work/'output')}),patch.object(r.os,'fsync',side_effect=failing_sync),patch.object(Path,'unlink',unlink),patch.object(r,'_artifact_read',wraps=r._artifact_read) as reads:
     result=r.prepare_upload(work)
    self.assertFalse(result['complete']);self.assertTrue(result['budget']['fallback']);self.assertEqual((work/'failure.json').read_bytes(),original_failure)
    raw=result['budget']['accounting']['safeCandidateBytes'];expected=raw+(0 if mode in ('cleaned','same-inode-modified') else written[0])
    self.assertEqual(result['budget']['retainedRawBytes'],expected)
    self.assertLessEqual(result['budget']['metadataBytes']+result['budget']['bundleBytes'],1048576)
    self.assertLessEqual(result['budget']['totalBytes'],83886080)
    names=[c.args[1] for c in reads.call_args_list];self.assertEqual(len(names),len(set(names)))
    if mode in ('cleaned','same-inode-modified'):self.assertFalse(main.exists())
    elif mode=='inode-replaced':self.assertEqual(main.read_bytes(),b'P'*written[0]);self.assertTrue(owned_copy.exists())
    else:self.assertEqual(main.stat().st_size,written[0])
    self.assertIn('synthetic-evidence-failure.tar',(work/'output').read_text())
 def test_fallback_reserve_and_total_exact_boundaries(self):
  import gate_receipts
  from unittest.mock import patch
  reserve=20000;retained=500000
  for _ in range(8):
   with patch.object(gate_receipts,'NORMAL_BYTES',retained),patch.object(gate_receipts,'DIAGNOSTIC_RESERVE',reserve),patch.object(gate_receipts,'TOTAL_BYTES',retained+reserve):
    _,budget=r._package_plan({'failure.json':b'{}'},retained,[],False,True)
   cost=budget['metadataBytes']+budget['bundleBytes']
   if reserve==cost:break
   reserve=cost
  with patch.object(gate_receipts,'NORMAL_BYTES',retained),patch.object(gate_receipts,'DIAGNOSTIC_RESERVE',reserve),patch.object(gate_receipts,'TOTAL_BYTES',retained+reserve):
   _,budget=r._package_plan({'failure.json':b'{}'},retained,[],False,True)
   self.assertEqual(budget['totalBytes'],retained+reserve)
  with patch.object(gate_receipts,'NORMAL_BYTES',retained),patch.object(gate_receipts,'DIAGNOSTIC_RESERVE',reserve-1),patch.object(gate_receipts,'TOTAL_BYTES',retained+reserve):
   with self.assertRaises(r.GuardError):r._package_plan({'failure.json':b'{}'},retained,[],False,True)
  with patch.object(gate_receipts,'NORMAL_BYTES',retained),patch.object(gate_receipts,'DIAGNOSTIC_RESERVE',reserve),patch.object(gate_receipts,'TOTAL_BYTES',retained+reserve-1):
   with self.assertRaises(r.GuardError):r._package_plan({'failure.json':b'{}'},retained,[],False,True)
 def test_manifest_255_256_257_entry_boundaries(self):
  import json,re
  for entries in (255,256,257):
   payload={f'raw-{i:03}.bin':b'x' for i in range(entries-2)}
   plan,_=r._package_plan(payload,len(payload),[],True,False)
   parts=[json.loads(d)['files'] for n,d in plan.members if re.fullmatch(r'upload-manifest-\d+\.json',n)]
   self.assertEqual(sum(map(len,parts)),entries);self.assertEqual(len(parts),(entries+255)//256)
   self.assertTrue(all(len(part)<=256 for part in parts))
 def test_fixed_point_nonconvergence_never_returns_writer_permission(self):
  from unittest.mock import patch
  original=r._json_bytes;calls=0
  def unstable(value):
   nonlocal calls
   data=original(value)
   if isinstance(value,dict) and 'bundlePayloadBytes' in value:
    calls+=1;return data+(b' '*(600 if calls%2 else 1))
   return data
  with patch.object(r,'_json_bytes',side_effect=unstable):
   with self.assertRaises(r.GuardError) as error:r._package_plan({'raw.bin':b'x'},1,[],True,False)
  self.assertEqual(str(error.exception),'budget-not-stable');self.assertEqual(calls,16)
 def test_same_inode_mutation_before_publication_is_not_published(self):
  import os
  from unittest.mock import patch
  with tempfile.TemporaryDirectory() as d:
   work=Path(d);r.persist_diagnostic(work,r.GuardError('controlled-test-failure'))
   real=r._verify_bundle_identity;changed=False
   def replace_after_check(path,owned,expected):
    nonlocal changed
    real(path,owned,expected)
    if path.name=='synthetic-evidence.tar' and not changed:
     changed=True
     with path.open('r+b') as handle:handle.write(b'EVIL')
   output=work/'output'
   with patch.dict(os.environ,{'GITHUB_OUTPUT':str(output)}),patch.object(r,'_verify_bundle_identity',side_effect=replace_after_check):
    with self.assertRaises(r.GuardError):r.prepare_upload(work)
   self.assertTrue(changed);self.assertFalse(output.exists())
 def test_moved_owned_prefix_and_short_or_exception_writes_are_charged(self):
  import os
  from unittest.mock import patch
  for mode in ('accepted1000','short333','raise333','closed333'):
   with self.subTest(mode=mode),tempfile.TemporaryDirectory() as d:
    work=Path(d);r.persist_diagnostic(work,r.GuardError('controlled-test-failure'))
    main=work/'synthetic-evidence.tar';moved=work/'moved-owned.tar';real_open=Path.open
    def poison():
     main.rename(moved);main.write_bytes(b'FOREIGN-POISON')
    class Target:
     def __init__(self,raw):self.raw=raw
     def __enter__(self):return self
     def __exit__(self,*args):return self.raw.__exit__(*args)
     def fileno(self):return self.raw.fileno()
     def flush(self):return self.raw.flush()
     def write(self,data):
      self.raw.write(data[:333]);poison()
      if mode=='closed333':self.raw.close();raise OSError('write closed descriptor after prefix')
      if mode=='raise333':raise OSError('write failed after prefix')
      return 333
    def opening(path,*args,**kwargs):
     raw=real_open(path,*args,**kwargs)
     return Target(raw) if path==main and args and args[0]=='xb' and mode!='accepted1000' else raw
    def fault(*args,**kwargs):
     kwargs['fileobj'].write(b'x'*1000)
     poison();raise OSError('archive failed after accepted prefix')
    with patch.dict(os.environ,{'GITHUB_OUTPUT':str(work/'output')}),patch.object(Path,'open',opening),patch('tarfile.open',side_effect=fault),patch.object(r,'_artifact_read',wraps=r._artifact_read) as reads:
     result=r.prepare_upload(work)
    actual=moved.stat().st_size;self.assertEqual(actual,1000 if mode=='accepted1000' else 333)
    expected=1000 if mode in ('accepted1000','closed333') else 333
    raw=result['budget']['accounting']['safeCandidateBytes']
    self.assertEqual(result['budget']['retainedRawBytes'],raw+expected)
    self.assertGreaterEqual(expected,actual);self.assertFalse(result['complete']);self.assertTrue(result['budget']['fallback'])
    self.assertEqual(main.read_bytes(),b'FOREIGN-POISON')
    self.assertLessEqual(result['budget']['metadataBytes']+result['budget']['bundleBytes'],1048576)
    self.assertLessEqual(result['budget']['totalBytes'],83886080)
    names=[c.args[1] for c in reads.call_args_list];self.assertEqual(len(names),len(set(names)))

# D1 draft tests. Not executed during the parent's performance window.
class I1DeadlineTransportContracts(unittest.TestCase):
 def setUp(self):
  r.COMMAND_EVIDENCE.clear()
 def _request(self,seconds):
  import time
  start=time.monotonic_ns()
  return {'schemaVersion':1,'bindingSHA256':'0'*64,'integrationPolicySHA256':'1'*64,'toolManifestSHA256':'2'*64,'runID':'00000000-0000-0000-0000-000000000001','parentStartedNs':start,'deadlineNs':start+int(seconds*1e9)}
 def _run(self,script,seconds=2):
  import sys,os
  with tempfile.TemporaryDirectory() as d:
   work=Path(d);env={'PATH':os.path.dirname(sys.executable),'HOME':d,'TMPDIR':d}
   return r._i1_supervise([sys.executable,'-I','-S','-B','-c',script],work,env,self._request(seconds))
 def test_real_exit_and_success_json_before_sleep_do_not_bypass_deadline(self):
  script="import sys,time;sys.stdin.read();print('{\"status\":\"success\"}',flush=True);time.sleep(5)"
  out,err,receipt=self._run(script,.2)
  self.assertIn(b'success',out);self.assertEqual(receipt['outcome'],'failed')
  self.assertEqual(receipt['returncode'],-9);self.assertIsNotNone(receipt['killRequestedNs'])
  self.assertGreaterEqual(receipt['reapedNs'],receipt['killRequestedNs'])
 def test_real_positive_transport_and_abnormal_exit(self):
  out,_,good=self._run("import sys;sys.stdin.read();print('transport fixture')")
  self.assertEqual(good['outcome'],'transport-success');self.assertEqual(good['returncode'],0)
  _,_,bad=self._run("import sys;sys.stdin.read();print('success');sys.exit(2)")
  self.assertEqual(bad['outcome'],'failed');self.assertEqual(bad['returncode'],2)
 def test_real_stdout_limit_kills_without_promotion(self):
  _,_,receipt=self._run("import sys,time;sys.stdin.read();sys.stdout.write('x'*20000);sys.stdout.flush();time.sleep(5)")
  self.assertEqual(receipt['outcome'],'failed');self.assertEqual(receipt['transportError'],'stdout-resource-limit')
 def test_only_real_platform_permit_can_enter_fetch(self):
  from unittest.mock import patch
  with patch.object(r.subprocess,'Popen',side_effect=AssertionError('must not launch')):
   with self.assertRaises(r.GuardError):r.i1_fetch_supervised(object())
 def test_current_missing_public_ca_configuration_fails_closed(self):
  from unittest.mock import patch
  with tempfile.TemporaryDirectory() as d,patch.object(r,'I1_DEP_PUBLIC_CA_FILE',None),patch.object(r.subprocess,'Popen',side_effect=AssertionError('must not launch')):
   with self.assertRaises(r.GuardError):r.i1_run_acquisition_controls(Path(d),'/not-reviewed',('127.0.0.1',))
 def test_actual_timeout_stops_later_partial_writes(self):
  import sys,os,time,hashlib
  with tempfile.TemporaryDirectory() as d:
   work=Path(d);path=work/'synthetic.part'
   code="import sys,time,pathlib;sys.stdin.read();p=pathlib.Path('synthetic.part');f=p.open('wb',buffering=0)\nwhile True:\n f.write(b'x');time.sleep(.02)"
   _,_,receipt=r._i1_supervise([sys.executable,'-I','-S','-B','-c',code],work,{'PATH':os.path.dirname(sys.executable),'HOME':d,'TMPDIR':d},self._request(.2))
   self.assertEqual(receipt['returncode'],-9);before=path.read_bytes();time.sleep(.1);self.assertEqual(path.read_bytes(),before)
   self.assertFalse((work/'cache/go-mod').exists())
 def test_three_stages_share_one_parent_deadline(self):
  _,_,receipt=self._run("import sys,time;sys.stdin.read()\nfor i in range(3):\n print(i,flush=True);time.sleep(.15)",.3)
  self.assertEqual(receipt['outcome'],'failed');self.assertEqual(receipt['transportError'],'timeout')
 def test_fixed_120_second_real_stall_control(self):
  # Mandatory real fixed-limit control. This is intentionally ~120 seconds;
  # do not run it until the parent releases the timing/build window.
  _,_,receipt=self._run("import sys,time;sys.stdin.read();time.sleep(130)",r.I1_FETCH_NS/1e9)
  self.assertEqual(receipt['deadlineNs']-receipt['parentStartedNs'],120000000000)
  self.assertEqual(receipt['returncode'],-9);self.assertEqual(receipt['outcome'],'failed')
  self.assertGreaterEqual(receipt['firstDeadlineObservedNs'],receipt['deadlineNs'])
  self.assertGreaterEqual(receipt['reapedNs'],receipt['killRequestedNs'])

class I1DeadlineRevisionContracts(unittest.TestCase):
 def setUp(self):r.COMMAND_EVIDENCE.clear();r._I1_PENDING_CHILDREN.clear()
 def request(self):
  import time
  start=time.monotonic_ns()
  return {'schemaVersion':1,'bindingSHA256':'0'*64,'integrationPolicySHA256':'1'*64,'toolManifestSHA256':'2'*64,'runID':'00000000-0000-0000-0000-000000000001','parentStartedNs':start,'deadlineNs':start+1000000000}
 def test_bootstrap_parser_never_imports_local_helpers(self):
  import builtins
  from unittest.mock import patch
  original=builtins.__import__
  def guarded(name,*args,**kwargs):
   if name in ('bounded','controller','gate_receipts'):raise AssertionError('pre-auth import')
   return original(name,*args,**kwargs)
  with patch.object(builtins,'__import__',side_effect=guarded):
   self.assertEqual(r._i1_json(b'{"x":1}'),{'x':1})
   for data in (b'{"x":1,"x":2}',b'{"x":1.0}',b'{"x":NaN}'):
    with self.assertRaises(r.GuardError):r._i1_json(data)
   with self.assertRaises(r.GuardError):r._i1_bootstrap_tools('0'*64)
 def test_changed_helper_rejected_without_execution(self):
  import shutil,json
  from unittest.mock import patch
  with tempfile.TemporaryDirectory() as d:
   root=Path(d);here=root/'ports/mygo/scripts/swift-snapshot-oracle';here.parent.mkdir(parents=True)
   shutil.copytree(HERE,here,ignore=shutil.ignore_patterns('__pycache__'))
   workflow=root/'.github/workflows';workflow.mkdir(parents=True)
   shutil.copyfile(r.REPO/'.github/workflows/swift-snapshot-oracle.yml',workflow/'swift-snapshot-oracle.yml')
   expected=r.sha((here/'tool-manifest.json').read_bytes())
   marker=root/'executed';(here/'bounded.py').write_text('open('+repr(str(marker))+',"w").write("bad")')
   with patch.object(r,'HERE',here),patch.object(r,'REPO',root):
    with self.assertRaises(r.GuardError):r._i1_bootstrap_tools(expected)
   self.assertFalse(marker.exists())
 def test_launch_failure_retains_honest_receipt(self):
  from unittest.mock import patch
  with tempfile.TemporaryDirectory() as d,patch.object(r.subprocess,'Popen',side_effect=OSError('launch')):
   _,_,receipt=r._i1_supervise(['never'],Path(d),{},self.request())
  self.assertIsNone(receipt['pid']);self.assertIsNone(receipt['reapedNs']);self.assertEqual(receipt['outcome'],'failed')
  self.assertEqual(len(r.COMMAND_EVIDENCE),1)
 def test_signal_wait_cleanup_errors_remain_pending(self):
  from unittest.mock import patch,Mock
  fake=Mock();fake.pid=987654;fake.returncode=None
  fake.stdin.fileno.side_effect=OSError('setup')
  fake.wait.side_effect=OSError('wait');fake.stdin.closed=False;fake.stdout.closed=False;fake.stderr.closed=False
  fake.stdin.close.side_effect=OSError('close')
  with tempfile.TemporaryDirectory() as d,patch.object(r.subprocess,'Popen',return_value=fake),patch.object(r.os,'getpgid',return_value=fake.pid),patch.object(r.os,'killpg',side_effect=PermissionError('signal')):
   _,_,receipt=r._i1_supervise(['fake'],Path(d),{},self.request())
  self.assertIsNone(receipt['reapedNs']);self.assertEqual(receipt['outcome'],'pending-termination')
  self.assertEqual({v['stage'] for v in receipt['cleanupIssues']},{'transport','terminate','reap','stream-close'})
  self.assertIs(r._I1_PENDING_CHILDREN[0],fake)
 def test_failure_drain_retains_queued_output(self):
  import sys,os
  from unittest.mock import patch
  real_read=os.read;seen=[False]
  def fail_once(fd,size):
   # Popen's private error pipe is read with 50000, not the transport's8192.
   if size==8192 and not seen[0]:
    import time
    time.sleep(.1);seen[0]=True;raise OSError('injected first transport read')
   return real_read(fd,size)
  with tempfile.TemporaryDirectory() as d,patch.object(r.os,'read',side_effect=fail_once):
   out,err,receipt=r._i1_supervise([sys.executable,'-I','-S','-B','-c','import sys,time;sys.stdin.read();print("queued stdout",flush=True);print("queued stderr",file=sys.stderr,flush=True);time.sleep(5)'],Path(d),{},self.request())
  self.assertIn(b'queued stdout',out);self.assertIn(b'queued stderr',err)
  self.assertEqual(receipt['outcome'],'failed');self.assertIsNotNone(receipt['reapedNs'])
 def test_profile_does_not_grant_signal(self):
  import sys
  with tempfile.TemporaryDirectory() as d:
   ca=Path(d)/'public.pem';ca.write_bytes(b'fixture')
   policy=r.i1_acquisition_policy(Path(d),sys.executable,sys.base_prefix,ca,('127.0.0.1',))
  self.assertNotIn('(allow signal',policy)
 def test_http_float_scalar_rejected(self):
  request=self.request();resources=[]
  for ordinal,kind in enumerate(r.I1_HTTP_KINDS):
   size,digest=r.I1_DEP_IDENTITIES[kind]
   path='/golang.org/x/text/@v/v0.43.0.'+kind
   row={'ordinal':ordinal,'requestKind':kind,'requestHost':'proxy.golang.org','requestPath':path,'responseHost':'proxy.golang.org','responsePath':path,'statusCode':200,'startedNs':request['parentStartedNs'],'endedNs':request['parentStartedNs'],'receivedBytes':size,'complete':True,'error':None}
   resources.append({'kind':kind,'relativePath':'input/dependency-download/v0.43.0.'+kind+'.part','bytes':size,'sha256':digest,'complete':True,'httpReceipts':[row]})
  value={'schemaVersion':1,'bindingSHA256':request['bindingSHA256'],'integrationPolicySHA256':request['integrationPolicySHA256'],'status':'success','resources':resources}
  r.i1_validate_child_response(value,request)
  for key in ('statusCode','receivedBytes'):
   row=resources[0]['httpReceipts'][0];old=row[key];row[key]=float(old)
   with self.assertRaises(r.GuardError):r.i1_validate_child_response(value,request)
   row[key]=old
 def test_pending_transport_persisted_without_snapshot(self):
  from unittest.mock import patch
  import weakref
  with tempfile.TemporaryDirectory() as d:
   work=Path(d);stage=work/'input/dependency-download';stage.mkdir(parents=True)
   profile=work/'acquisition.sb';profile.write_bytes(b'fixed profile fixture')
   permit=r._I1Permit();digest='0'*64;policy='1'*64
   r._I1_PERMITS[id(permit)]=(weakref.ref(permit),(work,digest,policy,'2'*64,'00000000-0000-0000-0000-000000000001',profile,r.sha(profile.read_bytes()),('fixture',),{},(0,1,2)))
   receipt={'reapedNs':None,'outcome':'pending-termination','stdoutRef':None,'stderrRef':None,'resourceRefs':[]}
   with patch.object(r,'_i1_binding',return_value=({},digest)),patch.object(r,'_i1_supervise',return_value=(b'partial output',b'failure',receipt)),patch.object(r,'i1_download_snapshot',side_effect=AssertionError('must not snapshot')):
    result=r.i1_fetch_supervised(permit)
   self.assertEqual(result['outcome'],'pending-termination')
   completion=r._i1_json((work/'provenance/dependency-completion.json').read_bytes())
   self.assertEqual(completion['controlCommandIndices'],[0,1,2]);self.assertEqual(completion['profileRef']['sha256'],r.sha(profile.read_bytes()))
   for key in ('transportRef','stdoutRef','stderrRef','profileRef'):
    ref=completion[key];raw=(work/ref['name']).read_bytes();self.assertEqual(len(raw),ref['bytes']);self.assertEqual(r.sha(raw),ref['sha256'])
   self.assertEqual(completion['resourceRefs'],[]);self.assertEqual(completion['httpRefs'],[])
 def test_success_transport_persists_http_chain(self):
  from unittest.mock import patch
  import weakref
  with tempfile.TemporaryDirectory() as d:
   work=Path(d);(work/'input/dependency-download').mkdir(parents=True)
   profile=work/'acquisition.sb';profile.write_bytes(b'fixed profile fixture');digest='0'*64;policy='1'*64
   permit=r._I1Permit();r._I1_PERMITS[id(permit)]=(weakref.ref(permit),(work,digest,policy,'2'*64,'00000000-0000-0000-0000-000000000001',profile,r.sha(profile.read_bytes()),('fixture',),{},(0,1,2)))
   def transport(argv,work,env,request):
    rows=[]
    for ordinal,kind in enumerate(r.I1_HTTP_KINDS):
     size,hash_value=r.I1_DEP_IDENTITIES[kind];path='/golang.org/x/text/@v/v0.43.0.'+kind
     http={'ordinal':ordinal,'requestKind':kind,'requestHost':'proxy.golang.org','requestPath':path,'responseHost':'proxy.golang.org','responsePath':path,'statusCode':200,'startedNs':request['parentStartedNs'],'endedNs':request['parentStartedNs'],'receivedBytes':size,'complete':True,'error':None}
     rows.append({'kind':kind,'relativePath':'input/dependency-download/v0.43.0.'+kind+'.part','bytes':size,'sha256':hash_value,'complete':True,'httpReceipts':[http]})
    response={'schemaVersion':1,'bindingSHA256':digest,'integrationPolicySHA256':policy,'status':'success','resources':rows}
    return r._json_bytes(response),b'',{'reapedNs':request['parentStartedNs'],'outcome':'transport-success','stdoutRef':None,'stderrRef':None,'resourceRefs':[]}
   with patch.object(r,'_i1_binding',return_value=({},digest)),patch.object(r,'_i1_supervise',side_effect=transport),patch.object(r,'i1_download_snapshot',return_value={'name':'fixture-only','bytes':0,'sha256':r.sha(b'')}):
    result=r.i1_fetch_supervised(permit)
   completion=r._i1_json((work/'provenance/dependency-completion.json').read_bytes())
   self.assertEqual(completion['outcome'],'transport-success');self.assertEqual(len(completion['httpRefs']),3)
   raw=r._i1_json((work/completion['stdoutRef']['name']).read_bytes())
   for ref,resource in zip(completion['httpRefs'],raw['resources']):
    data=(work/ref['name']).read_bytes();self.assertEqual(r.sha(data),ref['sha256']);self.assertEqual(r._i1_json(data),resource['httpReceipts'][0])
 def test_authenticated_loader_ignores_valid_poisoned_pyc(self):
  import sys,os,py_compile,importlib
  name='_i1_verified_source_fixture'
  with tempfile.TemporaryDirectory() as d:
   source=Path(d)/(name+'.py');bad=b'VALUE="POISON"\n';good=b'VALUE="SOURCE"\n';self.assertEqual(len(bad),len(good))
   source.write_bytes(bad);stamp=source.stat().st_mtime;py_compile.compile(str(source),doraise=True)
   source.write_bytes(good);os.utime(source,(stamp,stamp));sys.path.insert(0,d)
   try:
    module=importlib.import_module(name);self.assertEqual(module.VALUE,'POISON');del sys.modules[name]
    loader=r._i1_install_source_loader({name:(str(source),good)})
    try:self.assertEqual(importlib.import_module(name).VALUE,'SOURCE')
    finally:sys.meta_path.remove(loader);sys.modules.pop(name,None)
   finally:sys.path.remove(d)
 def test_child_json_limits_not_manifest_limits(self):
  import json
  for value in ({'path':'x'*241}, [[0]*64 for _ in range(9)]):
   raw=json.dumps(value).encode()
   with self.assertRaises(r.GuardError):r._i1_json(raw)
   r._i1_json(raw,131072,manifest=True)
 def test_exact_child_and_manifest_json_boundaries(self):
  import json
  for manifest,width,good_nodes in ((False,240,[[0]*64 for _ in range(7)]+[[0]*55]),(True,4096,[[0]*63 for _ in range(63)]+[[0]*62])):
   kwargs={'limit':131072 if manifest else 16384,'manifest':manifest}
   r._i1_json(json.dumps('x'*width).encode(),**kwargs)
   with self.assertRaises(r.GuardError):r._i1_json(json.dumps('x'*(width+1)).encode(),**kwargs)
   r._i1_json(json.dumps(good_nodes).encode(),**kwargs)
   good_nodes[-1].append(0)
   with self.assertRaises(r.GuardError):r._i1_json(json.dumps(good_nodes).encode(),**kwargs)
 def test_bootstrap_fifo_rejected_without_open_wait(self):
  import os,subprocess,sys
  with tempfile.TemporaryDirectory() as d:
   fifo=Path(d)/'fifo';os.mkfifo(fifo)
   code='import importlib.util;from pathlib import Path;s=importlib.util.spec_from_file_location("r",'+repr(str(HERE/'runner.py'))+');r=importlib.util.module_from_spec(s);s.loader.exec_module(r)\ntry:r._i1_bootstrap_read(Path('+repr(str(fifo))+'),4096)\nexcept r.GuardError:print("rejected")\nelse:raise SystemExit(1)'
   result=subprocess.run([sys.executable,'-I','-S','-B','-c',code],capture_output=True,timeout=2)
   self.assertEqual(result.returncode,0);self.assertEqual(result.stdout,b'rejected\n')

class I1SourceBatchContracts(unittest.TestCase):
 def test_policy_identity_and_full_role_inventory(self):
  import controller
  policy=controller.i1_integration_policy();self.assertEqual(policy['productSourceBase'],'0a680da10c4119d48ca6176e9efcbf8b6584dcef');self.assertIsNone(policy['runnerCommit'])
  rows=r.i1_source_rows();self.assertEqual((len(rows),len({x['oid'] for x in rows}),sum(x['bytes'] for x in rows),sum(x['chunks'] for x in rows)),(139,133,9462103,196))
  slots=r.i1_source_command_slots();self.assertTrue(r.i1_validate_source_slot_inventory(slots));self.assertEqual(slots[246]['role'],'i1-source-delta-five-batch-v1')
  for bad in (slots[:-1],slots+[slots[-1]],slots+[slots[-1]]*2):
   with self.assertRaises(r.GuardError):r.i1_validate_source_slot_inventory(bad)
  for field,value in (('role','ordinary-command'),('slot',True),('ordinal',0),('extra','unknown')):
   bad=[dict(v) for v in slots];bad[0][field]=value
   with self.assertRaises(r.GuardError):r.i1_validate_source_slot_inventory(bad)
  policy['productBlobCount']=1;self.assertEqual(controller.i1_integration_policy()['productBlobCount'],139)
 def test_unbound_production_never_starts_process(self):
  from unittest.mock import patch
  candidates=(None,object(),{}, {'schemaVersion':3,'bindingSHA256':'0'*64}, {'schemaVersion':4,'runnerCommit':'0'*40,'runnerTree':'0'*40,'integrationPolicySHA256':'0'*64})
  with patch.object(r.subprocess,'Popen',side_effect=AssertionError('must not launch')) as child,patch.object(r,'prepare',side_effect=AssertionError('must not prepare')) as prepare:
   for forged in candidates:
    with self.subTest(binding=type(forged).__name__),self.assertRaisesRegex(r.GuardError,'i1-final-runner-unbound'):r.i1_run_source_batch(forged)
   child.assert_not_called();prepare.assert_not_called()
 def _binary_batch(self):
  rows=[];stdout=b''
  for i,data in enumerate((b'\0a\nb',b'without newline',b'\n',b'',b'last\0\nbyte')):
   oid=r._i1_object_id('blob',data);header=(oid+' blob '+str(len(data))+'\n').encode();offset=len(stdout)
   rows.append(dict(index=i,path=str(i),oid=oid,bytes=len(data),sha256=r.sha(data),headerOffset=offset,headerLength=len(header),canonicalHeader=header.decode(),payloadOffset=offset+len(header),payloadLength=len(data),delimiterOffset=offset+len(header)+len(data),delimiterHex='0a'))
   stdout+=header+data+b'\n'
  return rows,stdout
 def test_binary_nul_internal_lf_empty_and_no_tail_lf(self):
  rows,stdout=self._binary_batch();self.assertEqual(r._i1_parse_fixed_batch(stdout,rows),(b'\0a\nb',b'without newline',b'\n',b'',b'last\0\nbyte'))
 def test_each_truncation_and_extra_byte_is_rejected_atomically(self):
  rows,stdout=self._binary_batch()
  for end in range(len(stdout)):
   with self.subTest(end=end),self.assertRaises(r.GuardError):r._i1_parse_fixed_batch(stdout[:end],rows)
  for extra in (b'\0',b'\n',stdout[:rows[1]['headerOffset']]):
   with self.assertRaises(r.GuardError):r._i1_parse_fixed_batch(stdout+extra,rows)
 def test_headers_order_duplicates_hash_and_mapping(self):
  import copy
  rows,stdout=self._binary_batch();first=rows[1]['headerOffset'];second=rows[2]['headerOffset']
  bads=[stdout.replace(b' blob ',b' tree ',1),stdout.replace(b' blob 4\n',b' blob 04\n',1),b'0'*40+stdout[40:],stdout[first:second]+stdout[:first]+stdout[second:],stdout[:first]*2+stdout[second:]]
  bads.append(stdout[:rows[0]['payloadOffset']]+b'X'+stdout[rows[0]['payloadOffset']+1:])
  for bad in bads:
   with self.assertRaises(r.GuardError):r._i1_parse_fixed_batch(bad,rows)
  for field in ('headerOffset','payloadOffset','delimiterOffset','payloadLength'):
   changed=copy.deepcopy(rows);changed[4][field]+=1
   with self.assertRaises(r.GuardError):r._i1_parse_fixed_batch(stdout,changed)
 def test_canonical_transport_schema_strict_types(self):
  good={'schemaVersion':1,'pid':123,'pgid':123,'parentStartedNs':1,'deadlineNs':120000000001,'endedNs':10,'firstDeadlineObservedNs':None,'killRequestedNs':None,'reapedNs':9,'returncode':0,'signal':None,'transportError':'','stdinBytes':205,'stdinClosed':True,'stdoutObservedBytes':50352,'stderrObservedBytes':0,'stdoutRetainedBytes':50352,'stderrRetainedBytes':0,'stdoutEOFObserved':True,'stderrEOFObserved':True,'complete':True,'outcome':'transport-success','cleanupIssues':[]}
  r.i1_validate_batch_transport(good,b'x'*50352,b'')
  for key,bad in [('transportError',None),('transportError',False),('transportError',0),('transportError',[]),('transportError','error'),('returncode',False),('returncode',1),('reapedNs',None),('stdoutEOFObserved',False),('stdinClosed',False),('stdinBytes',204),('stdoutObservedBytes',50353),('complete',False),('cleanupIssues',[{}]),('extra',1)]:
   with self.subTest(key=key,bad=bad),self.assertRaises(r.GuardError):r.i1_validate_batch_transport({**good,key:bad},b'x'*50352,b'')
 def test_retention_caps_include_all_copies(self):
  raw={f'p/{i:03}.bin':b'x'*131072 for i in range(160)};self.assertEqual(r.i1_source_retention_budget(raw)['rawBytes'],20971520)
  with self.assertRaises(r.GuardError):r.i1_source_retention_budget({**raw,'p/extra.bin':b'x'})
  empty={f'p/{i:04}.bin':b'' for i in range(8192)};r.i1_source_retention_budget(empty)
  with self.assertRaises(r.GuardError):r.i1_source_retention_budget({**empty,'p/extra.bin':b''})
  with self.assertRaises(r.GuardError):r.i1_source_retention_budget({'p/a.bin':b'x'*131073})
  with self.assertRaises(r.GuardError):r.i1_source_retention_budget({'../escape':b''})

# Real local process controls, not a macOS or I1 admission witness.
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import unittest
from unittest.mock import patch
b = r

RAW = (b'a'*40+b'\n')*5
RESULTS = []
class I1SourceBatchTransportContracts(unittest.TestCase):
    def run_child(self, source, timeout=1_000_000_000):
        with patch.object(b, 'I1_BATCH_TIMEOUT_NS', timeout):
            result = b._i1_batch_transport([sys.executable, '-c', source], Path.cwd(), {'PATH':'/usr/bin:/bin'}, RAW)
        RESULTS.append(result[2])
        self.assertFalse(any('sha' in k.lower() for k in result[2]))
        return result
    def test_echo(self):
        out, tail, r = self.run_child('import sys; sys.stdout.buffer.write(sys.stdin.buffer.read())')
        self.assertEqual(out, RAW); self.assertTrue(r['complete']); self.assertEqual(r['transportError'], '')
        self.assertTrue(r['stdinClosed']); self.assertEqual(r['stdinBytes'], 205)
    def test_binary(self):
        out, _, r = self.run_child('import sys; sys.stdin.buffer.read(); sys.stdout.buffer.write(bytes(range(256)))')
        self.assertEqual(out, bytes(range(256))); self.assertTrue(r['complete'])
    def test_nonzero(self):
        _, _, r = self.run_child('import sys; sys.stdin.buffer.read(); print("complete"); sys.exit(7)')
        self.assertEqual(r['returncode'], 7); self.assertFalse(r['complete'])
        self.assertIsNone(r['killRequestedNs'])
    def test_sleep_after_stdout(self):
        out, _, r = self.run_child('import sys,time; sys.stdin.buffer.read(); print("complete",flush=True); time.sleep(5)', 100_000_000)
        self.assertEqual(out, b'complete\n'); self.assertEqual(r['transportError'], 'timeout'); self.assertFalse(r['complete'])
    def test_sleep_after_eof(self):
        _, _, r = self.run_child('import os,sys,time; sys.stdin.buffer.read(); os.close(1); os.close(2); time.sleep(5)', 100_000_000)
        self.assertTrue(r['stdoutEOFObserved']); self.assertTrue(r['stderrEOFObserved'])
        self.assertEqual(r['transportError'], 'timeout'); self.assertFalse(r['complete'])
    def test_stdout_cap(self):
        out, _, r = self.run_child('import sys; sys.stdin.buffer.read(); sys.stdout.buffer.write(b"x"*131073)')
        self.assertEqual(r['transportError'], 'stdout-resource-limit'); self.assertEqual(len(out),131072)
        self.assertGreater(r['stdoutObservedBytes'],131072); self.assertFalse(r['complete'])
    def test_stderr_cap(self):
        _, tail, r = self.run_child('import sys; sys.stdin.buffer.read(); sys.stderr.buffer.write(b"x"*1048577)')
        self.assertEqual(r['transportError'],'stderr-resource-limit'); self.assertEqual(len(tail),65536)
        self.assertGreater(r['stderrObservedBytes'],1048576); self.assertFalse(r['complete'])
    def test_exact_caps(self):
        out, tail, r = self.run_child('import sys; sys.stdin.buffer.read(); sys.stdout.buffer.write(b"x"*131072); sys.stderr.buffer.write(b"z"*1048576)')
        self.assertTrue(r['complete']); self.assertEqual(len(out),131072); self.assertEqual(tail,b'z'*65536)
    def test_short_write(self):
        original = os.write
        def short(fd,data): return original(fd,data[:100])
        with patch.object(b.os,'write',short):
            _, _, r = self.run_child('import sys; sys.stdin.buffer.read()')
        self.assertEqual(r['transportError'],'stdin-short-write'); self.assertEqual(r['stdinBytes'],100); self.assertFalse(r['complete'])
    def test_real_broken_pipe(self):
        original = os.write
        def delayed(fd,data): time.sleep(.1); return original(fd,data)
        with patch.object(b.os,'write',delayed):
            _, _, r = self.run_child('import os,time; os.close(0); time.sleep(5)')
        self.assertEqual(r['transportError'],'stdin-broken-pipe'); self.assertFalse(r['complete'])
    def test_close_failure(self):
        real_popen = subprocess.Popen
        def popen(*a,**kw):
            p=real_popen(*a,**kw)
            p.stdin.close=lambda: (_ for _ in ()).throw(OSError('injected-close'))
            return p
        with patch.object(b.subprocess,'Popen',popen):
            _, _, r = self.run_child('import sys; sys.stdin.buffer.read()')
        self.assertEqual(r['transportError'],'stdin-close-error'); self.assertFalse(r['complete']); self.assertFalse(r['stdinClosed'])
    def test_pending_wait(self):
        real_wait = subprocess.Popen.wait
        held=[]
        def no_wait(proc,*a,**kw):
            held.append(proc); raise subprocess.TimeoutExpired(proc.args,1)
        try:
            with patch.object(b.subprocess.Popen,'wait',no_wait):
                _, _, r = self.run_child('import sys; sys.stdin.buffer.read()')
            self.assertFalse(r['complete']); self.assertEqual(r['outcome'],'pending-termination')
            self.assertIn(r['pid'], b._I1_BATCH_PENDING_CHILDREN); self.assertIsNone(r['reapedNs'])
        finally:
            for p in set(held):
                real_wait(p,timeout=2); b._I1_BATCH_PENDING_CHILDREN.pop(p.pid,None)
    def test_group_mismatch(self):
        with patch.object(b.os, 'getpgid', return_value=-1):
            _, _, r = self.run_child('import time; time.sleep(5)')
        self.assertFalse(r['complete']); self.assertEqual(r['transportError'], 'i1-owned-group')
        self.assertIsNotNone(r['reapedNs'])
    def test_no_signal_after_reaped_wait_error(self):
        real_wait = subprocess.Popen.wait
        def fail_after_reap(proc,*a,**kw):
            result = real_wait(proc,*a,**kw)
            raise OSError('injected-after-reap')
        with patch.object(b.subprocess.Popen,'wait',fail_after_reap), patch.object(b.os,'killpg') as kill:
            _, _, r = self.run_child('import sys; sys.stdin.buffer.read()')
        kill.assert_not_called()
        self.assertFalse(r['complete']); self.assertIsNone(r['killRequestedNs'])
        # Injected wait prevents a reaping timestamp, so conservatively pending.
        b._I1_BATCH_PENDING_CHILDREN.pop(r['pid'],None)
    def test_descendant_holds_output(self):
        _, _, r = self.run_child('import os,sys,time; sys.stdin.buffer.read(); pid=os.fork(); time.sleep(5) if pid==0 else sys.exit(0)',100_000_000)
        self.assertFalse(r['complete']); self.assertEqual(r['transportError'],'timeout')
        self.assertIsNotNone(r['killRequestedNs']); self.assertIsNotNone(r['reapedNs'])
    def test_invalid_length(self):
        with patch.object(b.subprocess,'Popen') as p:
            with self.assertRaises(ValueError): b._i1_batch_transport([],Path.cwd(),{},b'x')
            p.assert_not_called()



class I1SourceCaptureBlockContracts(unittest.TestCase):
 def test_main_unbound_before_prepare_or_any_child(self):
  import sys
  from unittest.mock import patch
  with patch.object(sys,'argv',['runner.py']),patch.object(r,'prepare',side_effect=AssertionError('must not prepare')),patch.object(r.subprocess,'Popen',side_effect=AssertionError('must not launch')),patch.object(r,'persist_diagnostic') as diagnostic,patch.object(r,'prepare_upload'):
   self.assertEqual(r.main(),1)
  self.assertEqual(str(diagnostic.call_args.args[1]),'i1-final-runner-unbound')
