# SPDX-License-Identifier: AGPL-3.0-only
"""Offline synthetic protocol tests only. No macOS, subprocess, Swift or network."""
import ast
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import stat
import sys
import tempfile
import unittest
from unittest.mock import patch
import zipfile

HERE = Path(__file__).absolute().parent
spec = importlib.util.spec_from_file_location('sandbox_diagnostic', HERE / 'driver.py')
d = importlib.util.module_from_spec(spec); spec.loader.exec_module(d)


class Contracts(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='sandbox-diagnostic-mock-', dir=os.path.realpath(tempfile.gettempdir()))
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name); self.work = self.root / 'work'; self.out = self.root / 'evidence'
        self.driver = d.sha((HERE / 'driver.py').read_bytes())
        self.r = d.load_runner()
        self.calls = []
        self.rcs = [0, 0, 0]
        self.mode = None
        self.now = 0.0
        self.exe = [1, 2, stat.S_IFREG | 0o755, 0, 2, 500000, 1, 1]

    def fake(self, argv, work, env, seconds, label, ledger):
        i = d.LABELS.index(label)
        self.calls.append((argv, work, env, seconds, label))
        self.assertEqual(argv, d.argv(work, i)); self.assertEqual(seconds, 20)
        self.assertEqual(env, d.child_env(work)); self.assertEqual(set(env), {'PATH','HOME','TMPDIR','LANG','LC_ALL','TZ','DEVELOPER_DIR','CLANG_MODULE_CACHE_PATH'})
        if self.mode == 'missing-receipt': raise OSError('synthetic disk failure')
        rc = self.rcs[i]
        transport = self.mode not in ('overflow', 'timeout')
        error = None if rc == 0 else 'nonzero exit: ' + str(rc)
        if not transport: rc = -9; error = 'process deadline'
        row = {'argv': argv, 'returncode': rc, 'seconds': 0.1, 'transportComplete': transport,
               'error': error, 'cleanupComplete': self.mode != 'cleanup',
               'cleanupErrors': ['reap: controlled synthetic failure'] if self.mode == 'cleanup' else [],
               'stdoutCount': 0, 'stderrCount': 0}
        output = b'' if i == 0 else b'Xcode 26.2\nBuild version synthetic fixture\n'
        if self.mode == 'wrong-version': output = b'Xcode 26.3\n'
        if self.mode == 'padded-version': output = b' Xcode 26.2\n'
        if self.mode == 'invalid-utf8': output = b'\xff'
        if self.mode == 'private': output = b'access_token=synthetic-secret-marker'
        if self.mode == 'boundary': output = (b'Xcode 26.2\n' + b'x' * (65536 - 11)) if i else b'x' * 65536
        if self.mode == 'over-limit-retained': output = b'x' * 65537
        for stream, raw in [('stdout', output), ('stderr', b'')]:
            suffix = '.raw' if transport else '.diagnostic-prefix'
            name = label + '-' + stream + suffix
            d.write(work / name, raw)
            row[stream] = {'path': name, 'bytes': len(raw), 'sha256': d.sha(raw)}
            row[stream+'Count'] = len(raw)
        if self.mode == 'overflow': row['stdoutCount'] = 65537
        ledger.append(row); d.write(work / (label+'-command.json'), d.encoded(row))
        self.now += 0.1
        if self.mode == 'drift': (work/'compile.sb').write_text('(allow default)')
        if self.mode == 'deadline': self.now = 120.1
        if self.mode == 'remaining': self.now = 89.1
        if self.mode == 'unexpected': raise RuntimeError('unexpected after valid receipt')
        if rc or not row['cleanupComplete']: raise ValueError('helper fail closed')
        return output.decode('utf-8').strip()

    def run_probe(self, executable=None):
        with patch.object(d.platform, 'system', return_value='Darwin'), \
             patch.object(d, 'directory', side_effect=lambda p: os.stat(self.root) if p == d.DEVELOPER else self.directory(p)), \
             patch.object(d, 'executable', side_effect=executable or (lambda p: (self.exe, None))), \
             patch.object(self.r, 'bounded_process', side_effect=self.fake), \
             patch.object(d.time, 'monotonic', side_effect=lambda: self.now):
            return d.probe(self.r, self.work, '1234', '1', 'a'*40, self.driver)

    @property
    def directory(self):
        return ORIGINAL_DIRECTORY

    def summary(self): return d.decode(d.read(self.work/'summary.json',32768))

    def export(self): return d.export(self.r,self.work,self.out,'1234','1','a'*40,self.driver)

    def test_verify_exact_closure(self):
        d.verify(self.driver)

    def test_bootstrap_rejects_changed_runner_before_exec(self):
        original=d.read
        def altered(path,cap):
            raw=original(path,cap)
            return raw+b'\n' if path==d.ORACLE/'runner.py' else raw
        with patch.object(d,'read',side_effect=altered), patch('builtins.exec') as execute:
            with self.assertRaises(ValueError):d.load_runner()
            execute.assert_not_called()

    def test_driver_wrong_hash(self):
        with self.assertRaises(ValueError):d.verify('0'*64)

    def test_no_capture_or_alternate_execution_api(self):
        tree=ast.parse((HERE/'driver.py').read_text())
        for n in ast.walk(tree):
            if isinstance(n,ast.Call) and isinstance(n.func,ast.Attribute):
                if not (n.func.attr == 'system' and isinstance(n.func.value, ast.Name) and n.func.value.id == 'platform'):
                    self.assertNotIn(n.func.attr,('capture','Popen','system','run','check_output','call','rglob','glob'))
        self.assertEqual(sum(isinstance(n,ast.Call) and isinstance(n.func,ast.Attribute) and n.func.attr=='bounded_process' for n in ast.walk(tree)),1)

    def test_workflow_bootstrap_executes_verified_bytes(self):
        workflow = d.WORKFLOW.read_text()
        start = workflow.index('          import hashlib, os, pathlib, stat, sys')
        end = workflow.index('          PY\n', start)
        source = '\n'.join(line[10:] for line in workflow[start:end].splitlines())
        args = ['-', self.driver, 'verify', '--reviewed-driver-sha256', self.driver,
                '--reviewed-manifest-sha256', d.MANIFEST_SHA]
        with patch.object(sys, 'argv', args), patch.object(Path, 'cwd', return_value=d.REPO):
            with self.assertRaises(SystemExit) as result:
                exec(compile(source, '<reviewed-workflow-bootstrap>', 'exec'), {})
            self.assertEqual(result.exception.code, 0)
        args[1] = '0' * 64
        with patch.object(sys, 'argv', args), patch.object(Path, 'cwd', return_value=d.REPO):
            with self.assertRaises(SystemExit) as result:
                exec(compile(source, '<reviewed-workflow-bootstrap>', 'exec'), {})
            self.assertEqual(result.exception.code, 'diagnostic driver hash mismatch')

    def test_stop_receipts_can_export_incomplete_without_upgrade(self):
        self.mode='timeout';self.run_probe();self.assertEqual(self.export(),0)
        out=d.decode(d.read(self.out/'export-summary.json',32768))
        self.assertFalse(out['diagnosticComplete']);self.assertEqual(out['status'],'incomplete')

    def test_invalid_utf8_or_private_stream_exports_no_zip(self):
        self.mode='private';self.run_probe();self.assertEqual(self.export(),1)
        self.assertFalse((self.out/'sandbox-diagnostic-evidence.zip').exists())
        self.assertEqual(d.decode(d.read(self.out/'export-summary.json',32768))['reason'],'redaction-required')

    def test_raw_and_prefix_ambiguity_rejected(self):
        self.run_probe();d.write(self.work/'basic-true-stdout.diagnostic-prefix',b'')
        self.assertEqual(self.export(),1)

    def test_export_hardlink_rejected(self):
        self.run_probe();os.link(self.work/'basic-true-stdout.raw',self.work/'alias')
        self.assertEqual(self.export(),1)

    def test_export_symlink_rejected(self):
        self.run_probe();p=self.work/'basic-true-stdout.raw';p.unlink();p.symlink_to(self.root/'missing')
        self.assertEqual(self.export(),1)

    def test_same_bytes_inode_replacement_stops(self):
        original=self.fake
        def changed(*args):
            value=original(*args)
            p=self.work/'compile.sb';data=p.read_bytes();p.unlink();p.write_bytes(data)
            return value
        with patch.object(self,'fake',side_effect=changed):self.run_probe()
        self.assertEqual(len(self.calls),1);self.assertFalse(self.summary()['diagnosticComplete'])

    def test_executable_ancestor_symlink_rejected(self):
        p=self.root/'actual';p.mkdir();f=p/'exe';f.write_bytes(b'1');f.chmod(0o755)
        link=self.root/'link';link.symlink_to(p,target_is_directory=True)
        self.assertEqual(d.executable(link/'exe')[1],'symlink')

    def test_three_synthetic_successes_never_capture(self):
        self.assertEqual(self.run_probe(),0);self.assertEqual(len(self.calls),3)
        s=self.summary();self.assertEqual(s['status'],'completed');self.assertTrue(s['diagnosticComplete'])
        for k in d.FALSE_FIELDS:self.assertIs(s[k],False)
        self.assertEqual(self.export(),0)
        with zipfile.ZipFile(self.out/'sandbox-diagnostic-evidence.zip') as z:
            self.assertEqual(len(z.namelist()),11)
            self.assertFalse(any('/' in n for n in z.namelist()))
            outer=d.decode(d.read(self.out/'export-summary.json',32768))
            for f in outer['files']:
                raw=z.read(f['path']);self.assertEqual(d.sha(raw),f['sha256']);self.assertEqual(len(raw),f['bytes'])

    def test_nonzero_aborted_a_b_continue_only_after_cleanup(self):
        self.rcs=[-6,-6,0]
        self.assertEqual(self.run_probe(),1);self.assertEqual(len(self.calls),3)
        self.assertEqual(self.summary()['status'],'probe-failure');self.assertTrue(self.summary()['diagnosticComplete'])
        self.assertEqual(self.export(),0)

    def test_missing_c_no_fallback(self):
        self.assertEqual(self.run_probe(lambda p:(None,'missing') if p==d.PROGRAMS[2] else (self.exe,None)),1)
        self.assertEqual(len(self.calls),2);self.assertEqual(self.summary()['probes'][2]['reason'],'missing')
        self.assertTrue(self.summary()['diagnosticComplete']);self.assertEqual(self.export(),0)

    def test_missing_sandbox_stops_all(self):
        self.assertEqual(self.run_probe(lambda p:(None,'missing') if p==d.SANDBOX else (self.exe,None)),1)
        self.assertEqual(self.calls,[]);self.assertFalse(self.summary()['diagnosticComplete']);self.assertEqual(self.export(),0)

    def test_missing_a_stops_all(self):
        self.run_probe(lambda p:(None,'missing') if p==d.PROGRAMS[0] else (self.exe,None))
        self.assertEqual(self.calls,[]);self.assertFalse(self.summary()['diagnosticComplete'])

    def test_aggregate_deadline_not_reset(self):
        self.mode='deadline';self.run_probe();self.assertEqual(len(self.calls),1)
        self.assertEqual(self.summary()['probes'][1]['reason'],'aggregate-budget')
        self.assertEqual(self.export(),0)

    def test_remaining_30_9_seconds_does_not_launch(self):
        self.mode='remaining';self.run_probe();self.assertEqual(len(self.calls),1)
        self.assertEqual(self.summary()['probes'][1]['reason'],'aggregate-budget');self.assertEqual(self.export(),0)

    def test_env_does_not_inherit(self):
        with patch.dict(os.environ, {'GITHUB_TOKEN':'sentinel','DYLD_INSERT_LIBRARIES':'sentinel','HOME':'sentinel'}):
            env=d.child_env(self.work)
            self.assertNotIn('sentinel',env.values());self.assertEqual(len(env),8)

    def test_exact_pipe_bound_export(self):
        self.mode='boundary';self.assertEqual(self.run_probe(),0);self.assertEqual(self.export(),0)
        self.assertLessEqual((self.out/'sandbox-diagnostic-evidence.zip').stat().st_size,524288)

    def test_retained_pipe_over_limit_rejected(self):
        self.mode='over-limit-retained';self.assertEqual(self.run_probe(),1);self.assertEqual(len(self.calls),1)
        self.assertEqual(self.summary()['probes'][0]['state'],'unknown')

    def test_aggregate_archive_budget_fail_closed(self):
        self.run_probe()
        with patch.object(d,'MAX_TOTAL',100):self.assertEqual(self.export(),1)
        self.assertFalse((self.out/'sandbox-diagnostic-evidence.zip').exists())

    def test_summary_over_budget_rejected(self):
        self.run_probe();(self.work/'summary.json').write_bytes(b' ' * 32769)
        self.assertEqual(self.export(),1)

    def test_profile_policy_change_rejected(self):
        self.run_probe();p=self.work/'compile.sb';p.write_bytes(p.read_bytes()+b'(allow default)\n')
        self.assertEqual(self.export(),1)

    def test_runtime_profile_rejected(self):
        self.run_probe();(self.work/'compile.sb').write_text(self.r.sandbox(self.work,d.DEVELOPER,False))
        self.assertEqual(self.export(),1)

    def test_duplicate_summary_key_rejected(self):
        self.run_probe();p=self.work/'summary.json';raw=p.read_bytes();p.write_bytes(raw[:-2]+b',"schema":"extra"}\n')
        self.assertEqual(self.export(),1)

    def test_reordered_slots_rejected(self):
        self.run_probe();s=self.summary();s['probes'].reverse();(self.work/'summary.json').write_bytes(d.encoded(s))
        self.assertEqual(self.export(),1)

    def test_attempt_after_unknown_rejected(self):
        self.run_probe();s=self.summary();s['probes'][0]={'label':'basic-true','state':'unknown','reason':'receipt-uncertain'}
        s['diagnosticComplete']=False;s['status']='incomplete';(self.work/'summary.json').write_bytes(d.encoded(s))
        self.assertEqual(self.export(),1)

    def test_oversized_image_value_rejected(self):
        with patch.dict(os.environ,{'ImageOS':'x'*81}):
            with self.assertRaises(ValueError):d.metadata()

    def test_staged_content_mutation_stops(self):
        original=self.fake
        def changed(*args):
            value=original(*args);p=self.work/'input'/'INPUT.json';p.chmod(0o644);p.write_bytes(b'changed')
            return value
        with patch.object(self,'fake',side_effect=changed):self.run_probe()
        self.assertEqual(len(self.calls),1);self.assertFalse(self.summary()['diagnosticComplete'])

    def test_profile_reference_exact(self):
        profile=self.r.sandbox(Path('/Users/runner/work/_temp/noonmark-ordinary-category-42343581550-1'),d.DEVELOPER,True).encode()
        self.assertEqual(d.sha(profile),'a654e48828caf57a44a923a9f56334672ae8ce9dab45b80f29b3d07e1a7269ad')

    def test_large_regular_executable_accepted(self):
        f=self.root/'exe';f.write_bytes(b'x'*300000);f.chmod(0o755)
        self.assertIsNone(d.executable(f)[1])
        os.link(f,self.root/'hard');self.assertIsNone(d.executable(f)[1])

    def test_typed_json_rejections(self):
        for raw in (b'{"x":1,"x":2}',b'{"x":NaN}',b'{"x":Infinity}'):
            with self.subTest(raw=raw),self.assertRaises(ValueError):d.decode(raw)
        for value in (True,False,-1,1.0):
            with self.subTest(value=value),self.assertRaises(ValueError):d.integer(value)
        for value in (True,float('inf'),float('nan'),-1):
            with self.subTest(value=value),self.assertRaises(ValueError):d.finite(value)

    def test_paths_ids_overlap(self):
        for path in ('relative','/tmp/../tmp/work','/tmp/quote"','/tmp/new\nline'):
            with self.subTest(path=path),self.assertRaises(ValueError):d.safe_path(path)
        for run,attempt,head in [('0','1','a'*40),('1;sh','1','a'*40),('1',True,'a'*40),('1','1','x'*40)]:
            with self.subTest(run=run,attempt=attempt),self.assertRaises(ValueError):d.inputs(self.work,self.out,run,attempt,head)
        with self.assertRaises(ValueError):d.inputs(self.work,self.work/'child','1','1','a'*40)
        with self.assertRaises(ValueError):d.inputs(d.REPO/'child',self.out,'1','1','a'*40)

    def test_read_links_fifo_oversize_rejected(self):
        f=self.root/'file';f.write_bytes(b'123');os.link(f,self.root/'hard')
        with self.assertRaises(ValueError):d.read(f,10)
        for name,target in [('link',f),('dangling',self.root/'missing')]:
            p=self.root/name;p.symlink_to(target)
            with self.assertRaises((OSError,ValueError)):d.read(p,10)
        fifo=self.root/'fifo';os.mkfifo(fifo)
        with self.assertRaises(ValueError):d.read(fifo,10)
        p=self.root/'big';p.write_bytes(b'12345')
        with self.assertRaises(ValueError):d.read(p,4)
        link=self.root/'dirlink';link.symlink_to(self.root,target_is_directory=True)
        with self.assertRaises(OSError):d.read(link/'big',10)

    def test_preexisting_roots_rejected(self):
        self.work.symlink_to(self.root/'absent')
        with self.assertRaises(ValueError):d.fresh(self.work)
        self.out.mkdir()
        with self.assertRaises(ValueError):self.export()

    def test_missing_work_export_only_failure(self):
        self.assertEqual(self.export(),1)
        self.assertFalse((self.out/'sandbox-diagnostic-evidence.zip').exists())
        self.assertFalse(d.decode(d.read(self.out/'export-summary.json',32768))['evidenceValidated'])

    def test_unlisted_private_sentinel_not_archived(self):
        self.run_probe();(self.work/'home'/'private').write_text('SECRET_SENTINEL_NOT_FOR_EXPORT')
        self.assertEqual(self.export(),0)
        with zipfile.ZipFile(self.out/'sandbox-diagnostic-evidence.zip') as z:
            self.assertFalse(any(b'SECRET_SENTINEL_NOT_FOR_EXPORT' in z.read(n) for n in z.namelist()))

    def test_export_rejects_receipt_hash_drift(self):
        self.run_probe();p=self.work/(d.LABELS[0]+'-command.json');p.write_bytes(p.read_bytes()+b' ')
        self.assertEqual(self.export(),1);self.assertFalse((self.out/'sandbox-diagnostic-evidence.zip').exists())

    def test_original_oracle_schema_rejected(self):
        self.run_probe();(self.work/'summary.json').write_text('{"schema":"ordinary-category-artifacts-v1"}')
        self.assertEqual(self.export(),1)


ORIGINAL_DIRECTORY=d.directory

def failure_case(mode,expected_calls=1):
    def test(self):
        self.mode=mode;self.assertEqual(self.run_probe(),1);self.assertEqual(len(self.calls),expected_calls)
        self.assertNotEqual(self.summary()['status'],'completed')
        self.assertFalse(self.summary()['diagnosticComplete'])
    return test

for _mode in ('missing-receipt','cleanup','timeout','overflow','invalid-utf8','private','drift','unexpected'):
    setattr(Contracts,'test_stop_'+_mode.replace('-','_'),failure_case(_mode))

def version_case(mode):
    def test(self):
        self.mode=mode;self.assertEqual(self.run_probe(),1);self.assertEqual(len(self.calls),3)
        self.assertEqual(self.summary()['status'],'probe-failure');self.assertEqual(self.export(),0)
    return test
for _mode in ('wrong-version','padded-version'):setattr(Contracts,'test_'+_mode.replace('-','_'),version_case(_mode))

def exe_case(kind):
    def test(self):
        p=self.root/'bad'
        if kind=='missing': pass
        elif kind=='symlink':p.symlink_to(self.root/'absent')
        elif kind=='nonregular':p.mkdir()
        else:p.write_bytes(b'0');p.chmod(0o644 if kind=='not-executable' else 0o777)
        self.assertEqual(d.executable(p)[1],kind)
    return test
for _kind in ('missing','symlink','nonregular','not-executable','writable'):setattr(Contracts,'test_executable_'+_kind.replace('-','_'),exe_case(_kind))

def summary_mutation(key,value):
    def test(self):
        self.run_probe();s=self.summary();s[key]=value;(self.work/'summary.json').write_bytes(d.encoded(s))
        self.assertEqual(self.export(),1);self.assertFalse((self.out/'sandbox-diagnostic-evidence.zip').exists())
    return test
for _key,_value in [('captureComplete',True),('swiftExecuted',True),('semanticParityVerified',True),('runtimeAdmission',True),('headSHA','b'*40),('runID','999'),('driverSHA256','0'*64),('profileSHA256','0'*64),('diagnosticComplete',False),('status','probe-failure'),('commandPhaseSeconds',-1),('unknown','field')]:
    setattr(Contracts,'test_summary_reject_'+_key,summary_mutation(_key,_value))

def receipt_mutation(key,value):
    def test(self):
        self.run_probe();p=self.work/'basic-true-command.json';row=d.decode(d.read(p,8192));row[key]=value
        raw=d.encoded(row);p.write_bytes(raw)
        s=self.summary();s['probes'][0]['receiptSHA256']=d.sha(raw);(self.work/'summary.json').write_bytes(d.encoded(s))
        self.assertEqual(self.export(),1);self.assertFalse((self.out/'sandbox-diagnostic-evidence.zip').exists())
    return test
for _key,_value in [('argv',['/usr/bin/true']),('returncode',True),('returncode',None),('transportComplete',1),('cleanupComplete',1),('cleanupErrors',['controlled failure']),('seconds',-1),('stdoutCount',True),('stderrCount',-1),('error','nonzero exit: 9'),('unknown','field'),('stdout',{'path':'../private','bytes':0,'sha256':d.sha(b'')})]:
    setattr(Contracts,'test_receipt_reject_'+_key+'_'+str(type(_value).__name__),receipt_mutation(_key,_value))

def dependency_mutation(filename):
    def test(self):
        original=d.read
        def altered(path,cap):
            raw=original(path,cap)
            return raw+b' ' if path==d.ORACLE/filename else raw
        with patch.object(d,'read',side_effect=altered),patch.object(self.r,'bounded_process') as proc:
            with self.assertRaises(ValueError):d.verify(self.driver)
            proc.assert_not_called()
    return test
for _name in ('TOOL-MANIFEST.json','SOURCE-CLOSURE.json','INPUT.json'):
    setattr(Contracts,'test_dependency_'+_name.replace('.','_'),dependency_mutation(_name))

def workflow_mutation(old,new):
    def test(self):
        original=d.read
        def altered(path,cap):
            raw=original(path,cap)
            if path==d.WORKFLOW:
                self.assertIn(old.encode(),raw);raw=raw.replace(old.encode(),new.encode(),1)
            return raw
        with patch.object(d,'read',side_effect=altered):
            with self.assertRaises(ValueError):d.verify(self.driver)
    return test
for _name,_old,_new in [
 ('branch','branches: [main]','branches: [dev]'),
 ('permission','contents: read','contents: write'),
 ('credentials','persist-credentials: false','persist-credentials: true'),
 ('checkout','checkout@d23441a48e516b6c34aea4fa41551a30e30af803','checkout@main'),
 ('upload','${{ env.EVIDENCE_ROOT }}/sandbox-diagnostic-evidence.zip','${{ env.EVIDENCE_ROOT }}/**'),
 ('timeout','timeout-minutes: 5','timeout-minutes: 360'),
 ('continue','        run: |','        continue-on-error: true\n        run: |'),
 ('main-guard',"if: github.ref == 'refs/heads/main'",'if: true')]:
    setattr(Contracts,'test_workflow_reject_'+_name,workflow_mutation(_old,_new))

if __name__=='__main__':unittest.main()
