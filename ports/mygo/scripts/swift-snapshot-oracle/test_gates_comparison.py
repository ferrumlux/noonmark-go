# SPDX-License-Identifier: AGPL-3.0-only
"""Only contract-unit fixtures; no runtime gate acceptance claimed."""
import unittest
from gates_comparison import execute_comparison_gates, verify_raw_claim, raw_mutations, validate_cross_receipt
from comparator import compare_raw, ReportError

class GateContracts(unittest.TestCase):
 def test_forged_equal_rejected(self):
  a,b=b'{"x":1}',b'{ "x":1}'
  c=compare_raw('raw-package-bytes',a,b);c['verdict']='equal'
  with self.assertRaises(ReportError):verify_raw_claim(c,a,b)
 def test_missing_three_real_rounds_fails_closed(self):
  result=execute_comparison_gates({},[],lambda *args:b'')
  self.assertTrue(all(v=='fail' for v in result['gates'].values()))
 def test_go_inspect_rejection_is_terminal(self):
  from bounded import sha
  raw=b'{}'
  receipt=dict(accepted=False,inputSHA256=sha(raw),preflightSucceeded=True,inspectInvoked=True,decoderInvoked=False,rejectedAt='inspect')
  validate_cross_receipt('go',raw,receipt)
  for field,value in [('inspectInvoked',False),('inspectInvoked',None),('decoderInvoked',True),('accepted',True),('rejectedAt','decode'),('preflightSucceeded',False)]:
   changed=dict(receipt);changed[field]=value
   with self.assertRaises(ReportError):validate_cross_receipt('go',raw,changed)
  with self.assertRaises(ReportError):validate_cross_receipt('swift',raw,receipt)
 def test_normal_cross_requires_decoder(self):
  from bounded import sha
  raw=b'{}'
  for side in ('swift','go'):
   for accepted in (True,False):
    receipt=dict(accepted=accepted,inputSHA256=sha(raw),preflightSucceeded=True,decoderInvoked=True)
    validate_cross_receipt(side,raw,receipt)
    receipt['decoderInvoked']=False
    with self.assertRaises(ReportError):validate_cross_receipt(side,raw,receipt)
 def test_four_mutations_change_bytes(self):
  raw=b'{"formatVersion":7,"snapshot":{"traces":[1,2],"subtasks":[],"definitions":[]}}'
  mutations=raw_mutations(raw)
  self.assertEqual(set(mutations),{'whitespace','escape','key-order','array-order'})
  self.assertTrue(all(raw!=v for v in mutations.values()))
if __name__=='__main__':unittest.main()
