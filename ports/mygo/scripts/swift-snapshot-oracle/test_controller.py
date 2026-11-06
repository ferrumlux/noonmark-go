# SPDX-License-Identifier: AGPL-3.0-only
import unittest
import controller as c
class Tests(unittest.TestCase):
 def test_failure_stage_set(self):
  for point in ('seed','create-one','schedule-one'):
   p=c.empty_point(point)
   self.assertEqual(len(p['stages']),29);self.assertEqual(len(p['comparisons']),7)
 def test_unverified_guards_rejected(self):
  with self.assertRaises(ValueError):c.validate_scope({'productionDataAccess':False})
 def test_claims_do_not_count(self):
  p=c.empty_point('seed');self.assertFalse(any(x['verdict']=='equal' for x in p['comparisons']))
if __name__=='__main__':unittest.main()

class SyntheticFailureReport(unittest.TestCase):
 def test_invalid_fixture_all_cases_stays_zero(self):
  # Protocol simulation only, no Swift execution or interoperability claim.
  import json,tempfile,pathlib
  from unittest.mock import patch
  def fail_probe(argv,**kwargs):
   out=pathlib.Path(argv[argv.index('--output')+1]);out.mkdir()
   case=json.loads(pathlib.Path(argv[argv.index('--case')+1]).read_text())
   value={'schemaVersion':2,'caseID':case['id'],'fixtureValidity':'invalid-fixture','points':[]}
   (out/'result.json').write_text(json.dumps(value))
   return {'returncode':2,'stdout':b'', 'stderrTail':b'', 'stderrBytes':0}
  guard=c._json([{'check':k,'outcome':'pass'} for k in c.GUARDS])
  scope={k:False for k in ('productionDataAccess','iCloudAccess','keychainAccess','aiNetworkAccess','actualAppExecuted')}
  scope.update(selectedStorageSources=True,guardEvidenceSHA256=c.sha(guard))
  tc={k:'contract-test-only' for k in c.TOOLCHAIN}
  for k in tc:
   if k.endswith('SHA256'):tc[k]='0'*64
  tc.update(runnerCommit='0'*40,architecture='arm64',locale='en_US.UTF-8',timezone='UTC')
  with tempfile.TemporaryDirectory() as d,patch.object(c,'execute',fail_probe):
   report=c.run_suite(['synthetic-not-swift'],['synthetic-not-go'],pathlib.Path(d),tc,scope,guard_raw=guard,run_negative_gates=False)
   self.assertEqual(report['summary']['effectivePointCount'],0)
   self.assertFalse(report['summary']['observationAccepted']);self.assertFalse(report['summary']['interopAccepted'])
   self.assertEqual(len(list(pathlib.Path(d).glob('round-*/*.capture.json'))),54)

class GateContracts(unittest.TestCase):
 def test_resource_gate_callback_never_runs(self):
  import gates
  result=gates.scanner_contracts()
  self.assertTrue(result);self.assertTrue(all(r['outcome']=='pass' for r in result))

class EntryMatrixProtocol(unittest.TestCase):
 def test_ingress_requires_os_success(self):
  import process
  p={'schemaVersion':2,'mode':'normal','pass':False,'rows':[],'expectedRows':[]}
  self.assertEqual(process.validate_exit('ingress','go',0,p),'success')
  for rc in (-9,1,2):
   with self.assertRaises(ValueError):process.validate_exit('ingress','go',rc,p)
 def test_empty_or_self_certified_matrix_does_not_pass(self):
  import gates
  for side in ('go','swift'):
   p={'schemaVersion':2,'mode':'normal','pass':True,'rows':[],'expectedRows':[]}
   self.assertFalse(gates.grade_probe_ingress(side,p));self.assertFalse(gates.fault_detected(side,p))

class CompleteChildReceipts(unittest.TestCase):
 def test_catalog_is_closed_and_resource_children_cannot_disappear(self):
  import gate_receipts as gr
  catalog=gr.catalog()
  self.assertEqual(len(catalog),789)
  for child in ('V07-process-timeout','V07-process-stdout','V07-process-stderr','V06-archive-count','V06-archive-run-bytes'):
   self.assertIn(child,catalog)
  with self.assertRaises(ValueError):gr.aggregate_verified({},catalog)
 def test_archive_and_process_records_use_actual_operations(self):
  import gate_receipts as gr,tempfile,pathlib
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);gr.ensure_run(root)
   for mode in ('positive','malformed','count','case-bytes','run-bytes','raw-bytes'):
    record=gr.capture_archive(root,mode)
    self.assertTrue(gr.verify_archive(root,record))
   record=gr.capture_process(root,'positive')
   self.assertTrue(gr.verify_process(root,record))
 def test_timeout_bypass_is_red_offline(self):
  import gate_receipts as gr,process,tempfile,pathlib
  def faulty(argv,**kw):kw['timeout']=3;return process.execute(argv,**kw)
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);gr.ensure_run(root)
   receipt=gr.capture_process(root,'timeout',execute=faulty)
   self.assertFalse(gr.verify_process(root,receipt))

class CatalogFormatOnly(unittest.TestCase):
 """这里只测试封闭收据格式，人工fail行不作为业务或运行成功证据。"""
 def test_missing_unknown_wrong_kind_stale_binding_and_forged_score(self):
  import gate_receipts as gr,tempfile,pathlib,json,hashlib
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);gr.ensure_run(root)
   for name in ('run-context.json','guard-evidence.json','gate-inputs.json'):(root/name).write_bytes(b'{}')
   rows=[]
   with gr.recording(root) as journal:
    for ident,item in gr.catalog().items():
     r=gr.envelope(root,ident,{'recipe':ident},'fail',1,2)
     if item['kind'] not in ('source-mutation','archive-operation','resource-process','path-operation'):
      r['inputRefs']=[gr.reference(root,root/n) for n in ('run-context.json','guard-evidence.json','gate-inputs.json')]
     if item['kind']=='platform-operation':r['payload']['witnessSHA256']=None
     journal.store(r);rows.append({'check':ident,'outcome':'fail'})
   gr.finalize(root,rows);self.assertEqual(len(gr.read_children(root)),789)
   child=root/'gate-children/child-0.json';original=child.read_bytes();part=root/'gate-children-index-0.json';original_part=part.read_bytes();top=root/'gate-children.json';original_top=top.read_bytes()
   def replace_child(value):
    child.write_bytes(gr.raw(value));p=json.loads(original_part);p['children'][0]['ref']=gr.reference(root,child);part.write_bytes(gr.raw(p));index=json.loads(original_top);index['parts'][0]=gr.reference(root,part);top.write_bytes(gr.raw(index))
   for field,value in [('kind','resource-process'),('bindingSHA256','0'*64),('id','unknown-child')]:
    v=json.loads(original);v[field]=value;replace_child(v)
    with self.assertRaises(ValueError):gr.read_children(root)
   child.write_bytes(original);part.write_bytes(original_part);top.write_bytes(original_top)
   child.unlink()
   with self.assertRaises((ValueError,OSError)):gr.read_children(root)
   child.write_bytes(original);(root/'gate-children/unknown.json').write_bytes(b'{}')
   with self.assertRaises(ValueError):gr.read_children(root)
   counts=dict.fromkeys(gr.catalog(),'pass');counts['V07-process-timeout']='fail'
   self.assertEqual(gr.aggregate_verified(counts)['V07'],'fail')
   counts.pop('V06-archive-count')
   with self.assertRaises(ValueError):gr.aggregate_verified(counts)

class ReadOnlyBindingContracts(unittest.TestCase):
 def test_missing_binding_does_not_change_evidence(self):
  import tempfile,gate_receipts as gr
  from pathlib import Path
  from bounded import Rejected
  with tempfile.TemporaryDirectory() as d:
   root=Path(d);(root/'original').write_bytes(b'original')
   before={p.name:p.read_bytes() for p in root.iterdir()}
   with self.assertRaises(Rejected):gr.binding_digest(root)
   self.assertEqual(before,{p.name:p.read_bytes() for p in root.iterdir()})
