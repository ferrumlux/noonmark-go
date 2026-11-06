# SPDX-License-Identifier: AGPL-3.0-only
"""Synthetic controller contracts, never Swift interoperability evidence."""
import copy, hashlib, json, pathlib, tempfile, unittest
import bounded as b
HERE=pathlib.Path(__file__).resolve().parent
class Tests(unittest.TestCase):
 def test_frozen(self):
  s=b.load_spec(HERE/'spec'); self.assertEqual(len(s['registry']['cases']),18)
  for seed in s['seeds'].values(): b.validate_envelope(seed,s['schema'],s['preferences'])
 def test_scanner_boundaries(self):
  for field,raw,limit in [('bytes',b'[] ',2),('depth',b'[[0]]',2),('nodes',b'{"a":0}',2),('string',b'"abc"',2),('strings',b'["ab","cd"]',3),('array',b'[0,1]',1),('members',b'{"a":0,"b":0}',1)]:
   with self.subTest(field=field),self.assertRaises(b.Rejected): b.load_json(raw,b.Limits(**{field:limit}))
 def test_bad_tokens(self):
  for raw in [b'{"a":1,"a":2}',b'{"a":0,"\\u0061":2}',b'01',b'-0',b'1.0',b'1e2',b'NaN',b'"\\ud800"',b'"\xff"',b'[] x',b'{"a":}',b'[1,]']:
   with self.subTest(raw=raw),self.assertRaises(b.Rejected): b.load_json(raw)
 def test_profile(self):
  s=b.load_spec(HERE/'spec')
  for key,val in [('unknownBase64','YWJj'),('taskCycleSeries',[{}])]:
   x=copy.deepcopy(s['seeds']['empty']);x['snapshot'][key]=val
   with self.assertRaises(b.Rejected):b.validate_envelope(x,s['schema'],s['preferences'])
  x=copy.deepcopy(s['seeds']['planned-12']);x['snapshot']['definitions'][0]['descriptionText']=None
  with self.assertRaises(b.Rejected):b.validate_envelope(x,s['schema'],s['preferences'])
 def test_archive_before_schema(self):
  with tempfile.TemporaryDirectory() as d:
   a=b.Archive(pathlib.Path(d),['observe-empty']);raw=b'{"bad":'
   obs=a.save('observe-empty','seed','swift-package',raw,lambda x:b.load_json(x))
   self.assertEqual(obs['rawState'],'malformed-raw');p=pathlib.Path(d)/obs['artifact']['name']
   self.assertEqual(p.read_bytes(),raw);self.assertEqual(obs['artifact']['sha256'],hashlib.sha256(raw).hexdigest())
   with self.assertRaises(b.Rejected):a.save('observe-empty','seed','swift-package',raw,lambda x:x)
   with self.assertRaises(b.Rejected):a.save('observe-empty','seed','go-package',b'x'*(262144+1),lambda x:x)
   self.assertFalse((pathlib.Path(d)/'observe-empty/seed/go-package.json').exists())
 def test_invocation(self):
  s=b.load_spec(HERE/'spec');ok={'schemaVersion':2,'registrySHA256':s['registry_sha'],'caseID':'observe-empty'}
  self.assertEqual(b.invocation(json.dumps(ok).encode(),s)['id'],'observe-empty')
  ok['base64']='eA=='
  with self.assertRaises(b.Rejected):b.invocation(json.dumps(ok).encode(),s)
 def test_no_follow_or_overwrite(self):
  with tempfile.TemporaryDirectory() as d:
   p=pathlib.Path(d);(p/'real').mkdir();(p/'link').symlink_to(p/'real',target_is_directory=True)
   with self.assertRaises(b.Rejected):b.Archive(p/'link',['observe-empty'])
 def test_prediction(self):
  self.assertGreater(b.predict({'a':'x'*4096}),24576)
  with self.assertRaises(b.Rejected): b.predict({'a':'x'*50000})
if __name__=='__main__':unittest.main()

class RealEntryWitness(unittest.TestCase):
 def test_reject_everything_fails_positive_entry(self):
  import gates
  from unittest.mock import patch
  spec=b.load_spec(HERE/'spec')
  with tempfile.TemporaryDirectory() as d:
   good=gates.controller_entry_matrix(HERE/'spec',{},None,None,None,pathlib.Path(d),'normal',entries=('stdin',))
   self.assertTrue(good['pass'])
   with patch.object(b,'scan_json',side_effect=b.Rejected('resource-limit')):
    bad=gates.controller_entry_matrix(HERE/'spec',{},None,None,None,pathlib.Path(d),'normal',entries=('stdin',))
   self.assertFalse(bad['pass'])
 def test_shared_preflight_bypass_is_detected(self):
  import gates
  with tempfile.TemporaryDirectory() as d:
   for mode in ('bypass','late'):
    result=gates.controller_entry_matrix(HERE/'spec',{},None,None,None,pathlib.Path(d),mode,entries=('stdin',))
    self.assertFalse(result['pass'])

 def test_real_config_seed_snapshot_capture_paths(self):
  import gates
  spec=b.load_spec(HERE/'spec')
  entries=gates.PY_ENTRY_CONFIG+gates.PY_ENTRY_SEEDS+gates.PY_ENTRY_OUTPUTS+('capture',)
  with tempfile.TemporaryDirectory() as d:
   for mode in ('normal','bypass','late'):
    r=gates.controller_entry_matrix(HERE/'spec',spec,None,None,None,pathlib.Path(d),mode,entries=entries)
    self.assertEqual(r['pass'],mode=='normal')
    self.assertEqual(len(r['rows']),sum(len(gates.PY_ENTRY_MATRIX[x]) for x in entries))
    for row in r['rows']:
     self.assertTrue(row['reached'],row)
     self.assertEqual(len(row['inputSHA256']),64)
     if row['dimension']=='positive':self.assertEqual(row['pass'],mode=='normal',row)
 def test_scope_actual_reader_and_missing_tool_not_run(self):
  import gates,controller
  guard=json.dumps([{'check':x,'outcome':'pass'} for x in controller.GUARDS]).encode()
  # This is a schema-only guard input, NOT actual sandbox or production proof.
  scope={k:False for k in ('productionDataAccess','iCloudAccess','keychainAccess','aiNetworkAccess','actualAppExecuted')}
  scope.update(selectedStorageSources=True,guardEvidenceSHA256=b.sha(guard))
  with tempfile.TemporaryDirectory() as d:
   r=gates.controller_entry_matrix(HERE/'spec',{},None,scope,None,pathlib.Path(d),'normal',entries=('guard-evidence',),guard_raw=guard)
   self.assertTrue(r['pass'],r)
   missing=gates.controller_entry_matrix(HERE/'spec',{},None,scope,None,pathlib.Path(d),'normal',entries=('tool-manifest',),guard_raw=guard)
   self.assertFalse(missing['pass']);self.assertTrue(all(x['outcome']=='not-run' for x in missing['rows']))
 def test_snapshot_bytes_are_actual_target(self):
  import gates
  spec=b.load_spec(HERE/'spec');blob=json.dumps(spec['seeds']['planned-21']).encode()
  with tempfile.TemporaryDirectory() as d:
   r=gates.controller_entry_matrix(HERE/'spec',spec,None,None,None,pathlib.Path(d),'normal',entries=('cross-go',),snapshot_data={'cross-go':blob})
   self.assertTrue(r['pass']);positive=r['rows'][0]
   self.assertEqual(positive['inputSHA256'],b.sha(blob));self.assertEqual(positive['inputBytes'],len(blob));self.assertGreater(positive['counters']['schema'],0)
 def test_tool_manifest_actual_reader(self):
  import gates,runner
  # Temporary manifest binds current actual tool bytes for reader testing only.
  # It is not the independently reviewed implementation manifest.
  files=[p for p in HERE.rglob('*') if p.is_file() and p.name!='tool-manifest.json' and '__pycache__' not in p.parts]
  files.append(runner.REPO/'.github/workflows/swift-snapshot-oracle.yml')
  manifest={'schemaVersion':2,'files':[dict(path=str(p.relative_to(runner.REPO)),role='tool',gitBlob='',sha256=b.sha(p.read_bytes()),bytes=p.stat().st_size) for p in sorted(files)]}
  with tempfile.TemporaryDirectory() as d:
   path=pathlib.Path(d)/'current-tool-input.json';path.write_text(json.dumps(manifest))
   for mode in ('normal','bypass','late'):
    r=gates.controller_entry_matrix(HERE/'spec',{},None,None,path,pathlib.Path(d),mode,entries=('tool-manifest',))
    self.assertEqual(r['pass'],mode=='normal',r)
    self.assertTrue(all(x['reached'] for x in r['rows']))
 def test_fixed_shards_and_index_limits(self):
  import gates,runner,controller
  spec=b.load_spec(HERE/'spec')
  files=[p for p in HERE.rglob('*') if p.is_file() and p.name!='tool-manifest.json' and '__pycache__' not in p.parts]+[runner.REPO/'.github/workflows/swift-snapshot-oracle.yml']
  manifest={'schemaVersion':2,'files':[dict(path=str(p.relative_to(runner.REPO)),role='tool',gitBlob='',sha256=b.sha(p.read_bytes()),bytes=p.stat().st_size) for p in sorted(files)]}
  guard=json.dumps([{'check':x,'outcome':'pass'} for x in controller.GUARDS]).encode()
  scope={k:False for k in ('productionDataAccess','iCloudAccess','keychainAccess','aiNetworkAccess','actualAppExecuted')};scope.update(selectedStorageSources=True,guardEvidenceSHA256=b.sha(guard))
  with tempfile.TemporaryDirectory() as d:
   path=pathlib.Path(d)/'manifest.json';path.write_text(json.dumps(manifest))
   result=gates.controller_entry_matrix(HERE/'spec',spec,None,scope,path,pathlib.Path(d),'normal',guard_raw=guard)
   self.assertTrue(result['pass'],[r for r in result['rows'] if not r['pass']])
   shards=gates.controller_entry_shards(result);index=gates.controller_entry_index(result,shards)
   self.assertEqual(len(shards),len(gates.PY_ENTRY_ALL));self.assertEqual(index['expectedEntries'],list(gates.PY_ENTRY_ALL))
   for ref in index['shards']:
    blob=gates.raw(shards[ref['name']]);self.assertLessEqual(len(blob),131072);self.assertEqual(ref['sha256'],b.sha(blob))
    b.load_json(blob,b.Limits(bytes=131072,depth=16,nodes=16384,string=4096,strings=131072,array=512))
   with self.assertRaises(b.Rejected):gates.controller_entry_index(result,{})
 def test_actual_run_inputs_require_capture_and_receipts(self):
  import gates
  with tempfile.TemporaryDirectory() as d,self.assertRaises((b.Rejected,FileNotFoundError)):
   gates.controller_entry_inputs(pathlib.Path(d),b.load_spec(HERE/'spec'))
