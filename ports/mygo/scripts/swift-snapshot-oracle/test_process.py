# SPDX-License-Identifier: AGPL-3.0-only
import sys,unittest
from process import execute
from bounded import Rejected
class Tests(unittest.TestCase):
 def test_small(self):self.assertEqual(execute([sys.executable,'-c','print("ok")'],timeout=2)['stdout'],b'ok\n')
 def test_stdout(self):
  with self.assertRaises(Rejected):execute([sys.executable,'-c','import os;os.write(1,b"x"*131073)'],timeout=2)
 def test_stderr(self):
  with self.assertRaises(Rejected):execute([sys.executable,'-c','import os;\nwhile True:os.write(2,b"x"*8192)'],timeout=2)
 def test_timeout(self):
  with self.assertRaises(Rejected):execute([sys.executable,'-c','import time;time.sleep(2)'],timeout=.02)
if __name__=='__main__':unittest.main()

class ExitProtocol(unittest.TestCase):
 def test_killed_success_json_never_valid(self):
  from process import validate_exit
  for rc in (-9,1,2,99):
   with self.assertRaises(Rejected):validate_exit('case','go',rc,{'schemaVersion':2,'fixtureValidity':'valid'})
 def test_controlled_inspect_rejection(self):
  from process import validate_exit
  value={'schemaVersion':2,'accepted':False,'status':'inspect-rejected','inspectCalls':1,'decodeCalls':0,'requiresClassificationAudit':False}
  self.assertEqual(validate_exit('cross','go',2,value),'controlled-rejection')
  for rc in (-9,0,1):
   with self.assertRaises(Rejected):validate_exit('cross','go',rc,value)
 def test_positive_requires_normal_exit(self):
  from process import validate_exit
  with self.assertRaises(Rejected):validate_exit('mutation','swift',-9,{'schemaVersion':2,'accepted':True,'id':'control-valid'})

class ActualExitReceipt(unittest.TestCase):
 def test_complete_json_before_abnormal_exit_is_retained_not_accepted(self):
  import pathlib,tempfile,json,hashlib
  from process import Recorder
  value={'schemaVersion':2,'accepted':True,'status':'accepted','inspectCalls':1,'decodeCalls':1,'requiresClassificationAudit':False,'decodedSHA256':hashlib.sha256(b'{}').hexdigest()}
  for ending,expected in [('sys.exit(0)','success'),('sys.exit(1)','harness-error'),('os.kill(os.getpid(),9)','harness-error')]:
   with tempfile.TemporaryDirectory() as d:
    root=pathlib.Path(d);out=root/'probe';raw=json.dumps(value,separators=(',',':')).encode()
    script='import pathlib,sys,os;p=pathlib.Path(sys.argv[1]);p.mkdir();(p/"decoded.json").write_bytes(b"{}");raw='+repr(raw)+';(p/"result.json").write_bytes(raw);os.write(1,raw+b"\\n");'+ending
    recorder=Recorder(root,[0]);proc,protocol,receipt=recorder.invoke(execute,'actual-exit','cross','go',[sys.executable,'-c',script,str(out)],out)
    self.assertEqual(receipt['outcome'],expected);self.assertEqual(receipt['schemaValidWitness'],expected=='success')
    self.assertEqual((out/'result.json').read_bytes(),raw);self.assertEqual(len(receipt['artifacts']),2)
    self.assertGreater(receipt['executionWitness']['pid'],0)
 def test_unbound_synthetic_exit_zero_cannot_witness_success(self):
  import pathlib,tempfile,json
  from process import Recorder
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);out=root/'probe';v={'schemaVersion':2,'mode':'normal','pass':False,'rows':[],'expectedRows':[]};raw=json.dumps(v).encode()
   def fake(argv,**kw):
    out.mkdir();(out/'result.json').write_bytes(raw)
    return {'returncode':0,'stdout':raw+b'\n','stderrTail':b'','stderrBytes':0}
   _,_,r=Recorder(root,[0]).invoke(fake,'fake','ingress','go',['not-an-os-process'],out)
   self.assertEqual(r['outcome'],'harness-error');self.assertFalse(r['schemaValidWitness'])
