"""Synthetic Win32 ownership tests. Never a Windows host-pass claim."""
import importlib.util
from pathlib import Path
import tempfile
import time
import unittest
from unittest import mock
import ctypes as C


def adapter():
    spec = importlib.util.spec_from_file_location('windows_job_tested', Path(__file__).with_name('windows_job.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class FakeAPI:
    def __init__(self, fail=None):
        self.fail_at = fail
        self.events = []
        self.live = set()
        self.serial = 0
        self.process = None
        self.exit = None
        self.active = 0
        self.readers = []
        self.data = {}
        self.foreign = {'unrelated-process', 'unrelated-job'}

    def event(self, name, *args):
        self.events.append((name, *args))
        if name == self.fail_at:
            raise OSError('injected ' + name)

    def handle(self, kind):
        self.serial += 1
        h = kind + str(self.serial)
        self.live.add(h)
        return h

    def create_job(self):
        self.event('create_job')
        return self.handle('job')

    def pipe(self):
        self.event('pipe')
        r,w = self.handle('read'), self.handle('write')
        self.readers.append(r)
        return r,w

    def nul(self):
        self.event('nul')
        return self.handle('nul')

    def create_suspended(self, argv, cwd, env, stdin, stdout, stderr):
        self.event('create_suspended', argv, cwd, env, stdin, stdout, stderr)
        self.process = self.handle('process')
        return self.process, self.handle('thread'), 1234

    def assign(self, job, process):
        self.event('assign', job, process)
        self.active = 1

    def resume(self, thread):
        self.event('resume', thread)

    def close(self, handle):
        assert handle in self.live, 'double/foreign close: ' + str(handle)
        self.event('close', handle)
        self.live.remove(handle)

    def poll(self, process):
        self.event('poll', process)
        return self.exit

    def terminate(self, process):
        self.event('terminate', process)
        self.exit = 1
        self.active = 0

    def terminate_job(self, job):
        self.event('terminate_job', job)
        self.exit = 1 if self.exit is None else self.exit
        self.active = 0

    def active_processes(self, job):
        self.event('active_processes', job)
        return self.active

    def read_pipe(self, handle, limit):
        self.event('read_pipe', handle, limit)
        if self.data.get(handle):
            return self.data[handle].pop(0)
        return b'' if self.exit is not None else None


class Marker:
    def __init__(self): self.chunks=[]
    def feed(self, data, now): self.chunks.append(data)


class WindowsOwnershipTests(unittest.TestCase):
    def launch(self, api, root, marker=None):
        owner=adapter().WindowsOwned(api=api, deadline=time.monotonic()+5)
        child=owner.start(['C:\\Program Files\\App\\app.exe', '-screen', 'day'], root,
                          {'HOME':str(root), 'SYNTHETIC':'a b'}, root/'stdout', root/'stderr', marker)
        return owner,child

    def test_suspended_assign_resume_order_and_only_standard_handles(self):
        api=FakeAPI()
        with tempfile.TemporaryDirectory() as tmp:
            owner,child=self.launch(api,Path(tmp))
            names=[e[0] for e in api.events]
            self.assertLess(names.index('create_job'),names.index('create_suspended'))
            self.assertLess(names.index('create_suspended'),names.index('assign'))
            self.assertLess(names.index('assign'),names.index('resume'))
            call=next(e for e in api.events if e[0]=='create_suspended')
            self.assertEqual(len(set(call[4:])),3)
            self.assertTrue(all(not h.startswith(('job','read','process','thread')) for h in call[4:]))
            api.exit=0;api.active=0
            owner.pump(time.monotonic()+1)
            self.assertEqual(child.poll(),0)
            self.assertTrue(owner.pipes_finished(child))
            records=owner.cleanup(time.monotonic()+1)
            self.assertTrue(records[0]['reaped'])
            self.assertTrue(records[0]['job_empty'])
            self.assertFalse(api.live)
            self.assertEqual(api.foreign,{'unrelated-process','unrelated-job'})
            query=max(i for i,e in enumerate(api.events) if e[0]=='active_processes')
            jobclose=next(i for i,e in enumerate(api.events) if e[0]=='close' and e[1].startswith('job'))
            self.assertLess(query,jobclose)

    def test_each_start_failure_cleans_handles_never_unmanaged_resume(self):
        for failed in ('create_job','pipe','nul','create_suspended','assign','resume'):
            with self.subTest(failed=failed),tempfile.TemporaryDirectory() as tmp:
                api=FakeAPI(failed);owner=None
                try:
                    owner=adapter().WindowsOwned(api=api,deadline=time.monotonic()+2)
                    with self.assertRaises(OSError):
                        owner.start(['C:\\app.exe'],Path(tmp),{},Path(tmp)/'stdout',Path(tmp)/'stderr')
                finally:
                    if owner: owner.cleanup(time.monotonic()+1)
                self.assertFalse(api.live,api.events)
                names=[e[0] for e in api.events]
                if failed in ('create_job','pipe','nul','create_suspended','assign'):
                    self.assertNotIn('resume',names)
                if failed in ('assign','resume'):
                    self.assertIn('terminate',names)
                    self.assertIn('poll',names)

    def test_marker_only_stdout_and_bounded_nonblocking_pipes(self):
        api=FakeAPI();marker=Marker()
        with tempfile.TemporaryDirectory() as tmp:
            owner,child=self.launch(api,Path(tmp),marker)
            api.data[api.readers[0]]=[b'NOONMARK_NATIVE_',b'WINDOW_READY_V1\n']
            api.data[api.readers[1]]=[b'NOONMARK_NATIVE_WINDOW_READY_V1\n']
            owner.pump(time.monotonic()+1);owner.pump(time.monotonic()+1)
            self.assertEqual(b''.join(marker.chunks),b'NOONMARK_NATIVE_WINDOW_READY_V1\n')
            api.exit=0;api.active=0
            owner.cleanup(time.monotonic()+1)
            self.assertFalse(api.live)

    def test_failed_assignment_remains_failure_even_after_exit0(self):
        api=FakeAPI('assign')
        with tempfile.TemporaryDirectory() as tmp:
            owner=adapter().WindowsOwned(api=api,deadline=time.monotonic()+2)
            with self.assertRaises(OSError): owner.start(['C:\\app.exe'],Path(tmp),{},Path(tmp)/'stdout',Path(tmp)/'stderr')
            api.exit=0
            owner.cleanup(time.monotonic()+1)
            self.assertIsNotNone(owner.failed)
            self.assertFalse(api.live)

    def test_job_query_failure_is_incomplete_not_success(self):
        api=FakeAPI()
        with tempfile.TemporaryDirectory() as tmp:
            owner,child=self.launch(api,Path(tmp));api.exit=0;api.active=0;api.fail_at='active_processes'
            records=owner.cleanup(time.monotonic()+.1)
            self.assertIsNotNone(owner.failed)
            self.assertFalse(records[0]['job_empty'])
            self.assertFalse(api.live)

    def test_unreaped_process_or_active_job_cannot_pass(self):
        api=FakeAPI()
        with tempfile.TemporaryDirectory() as tmp:
            owner,child=self.launch(api,Path(tmp))
            api.terminate=lambda p:api.event('terminate_ignored',p)
            api.terminate_job=lambda p:api.event('terminate_job_ignored',p)
            records=owner.cleanup(time.monotonic()+.03)
            self.assertIsNotNone(owner.failed)
            self.assertFalse(records[0]['reaped'])
            self.assertFalse(records[0]['job_empty'])
            self.assertFalse(api.live)

    def test_oversized_log_locks_failure_and_still_closes_handles(self):
        api=FakeAPI()
        with tempfile.TemporaryDirectory() as tmp:
            owner,child=self.launch(api,Path(tmp))
            api.data[api.readers[0]]=[b'x'*(1<<20), b'y']
            with self.assertRaises(Exception):
                owner.pump(time.monotonic()+1);owner.pump(time.monotonic()+1)
            owner.cleanup(time.monotonic()+1)
            self.assertIsNotNone(owner.failed)
            self.assertFalse(api.live)

    def test_one_close_failure_does_not_skip_other_handles(self):
        api=FakeAPI()
        with tempfile.TemporaryDirectory() as tmp:
            owner,child=self.launch(api,Path(tmp));api.exit=0;api.active=0
            original=api.close;failed=[]
            def close_once(handle):
                if not failed:
                    failed.append(handle)
                    raise OSError('injected close failure')
                original(handle)
            api.close=close_once
            owner.cleanup(time.monotonic()+1)
            self.assertIsNotNone(owner.failed)
            self.assertLessEqual(len(api.live),1)

    def test_process_observation_failure_cannot_skip_remaining_cleanup(self):
        api=FakeAPI()
        with tempfile.TemporaryDirectory() as tmp:
            owner,child=self.launch(api,Path(tmp));api.fail_at='poll'
            records=owner.cleanup(time.monotonic()+.05)
            self.assertIsNotNone(owner.failed)
            self.assertFalse(records[0]['reaped'])
            self.assertFalse(api.live)

    def test_expired_start_does_not_create_product(self):
        api=FakeAPI()
        with tempfile.TemporaryDirectory() as tmp:
            owner=adapter().WindowsOwned(api=api,deadline=time.monotonic()-1)
            with self.assertRaises(Exception): owner.start(['C:\\app.exe'],Path(tmp),{},Path(tmp)/'stdout',Path(tmp)/'stderr')
            owner.cleanup(time.monotonic()+.1)
            self.assertNotIn('create_suspended',[e[0] for e in api.events])
            self.assertFalse(api.live)


    def test_windows_stage_uses_adapter_fixed_argv_and_normal_report_validation(self):
        import test_run
        import json
        api=FakeAPI()
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);owner=adapter().WindowsOwned(api=api,deadline=time.monotonic()+5)
            original=api.resume
            def resume(thread):
                original(thread);api.exit=0;api.active=0
                api.data[api.readers[0]]=[test_run.m.MARKER+b'\n']
                (root/'stage1.json').write_text(json.dumps(test_run.report()))
            api.resume=resume
            with mock.patch.object(test_run.m,'windows_owner',return_value=owner), mock.patch.object(test_run.m,'snapshot',return_value={'synthetic':'hash'}), mock.patch.object(test_run.m.subprocess,'Popen') as popen:
                record=test_run.m.stage(Path('C:/app.exe'),root,1,'windows/amd64',time.monotonic()+60)
                popen.assert_not_called()
            self.assertTrue(record['passed'])
            self.assertTrue(record['processes'][0]['job_empty'])
            call=next(e for e in api.events if e[0]=='create_suspended')
            self.assertEqual(call[1],['C:/app.exe','-init-session',str(root/'session'),'-screen','day','-native-window-smoke',str(root/'stage1.json')])
            self.assertFalse(api.live)


class NativeCallStructureTests(unittest.TestCase):
    """Exercise the actual ctypes shim against an injected kernel, not Windows."""
    def test_actual_create_call_has_suspended_flags_and_exact_handle_list(self):
        m=adapter();calls=[];saved={}
        class Kernel:
            def InitializeProcThreadAttributeList(self, ptr, count, flags, size):
                calls.append('attributes-init')
                C.cast(size,C.POINTER(m.SIZE_T))[0]=64
                return 0 if ptr is None else 1
            def UpdateProcThreadAttribute(self, ptr, flags, attribute, handles, size, previous, returned):
                saved['handles']=list(handles);saved['attribute']=attribute
                self_test.assertEqual(size,3*C.sizeof(m.HANDLE))
                return 1
            def CreateProcessW(self,exe,command,psa,tsa,inherit,flags,env,cwd,info,pi):
                calls.append('create')
                saved.update(exe=exe,command=command.value,inherit=inherit,flags=flags,
                             env=env[:],cwd=cwd,psa=psa,tsa=tsa)
                startup=C.cast(info,C.POINTER(m.STARTUPINFOEXW)).contents
                self_test.assertEqual(startup.StartupInfo.cb,C.sizeof(m.STARTUPINFOEXW))
                self_test.assertEqual(startup.StartupInfo.dwFlags,m.STARTF_USESTDHANDLES)
                self_test.assertEqual([startup.StartupInfo.hStdInput,startup.StartupInfo.hStdOutput,startup.StartupInfo.hStdError],[11,12,13])
                result=C.cast(pi,C.POINTER(m.PROCESS_INFORMATION)).contents
                result.hProcess=21;result.hThread=22;result.dwProcessId=23
                return 1
            def DeleteProcThreadAttributeList(self,ptr): calls.append('attributes-delete')
        self_test=self;api=m.NativeAPI.__new__(m.NativeAPI);api.k=Kernel()
        argv=['C:\\Program Files\\app.exe','a b','quote"here','trailing\\']
        with mock.patch.object(C,'get_last_error',return_value=122,create=True), mock.patch.object(C,'set_last_error',create=True):
            self.assertEqual(api.create_suspended(argv,'C:\\private',{'Z':'last','A':'first'},11,12,13),(21,22,23))
        self.assertEqual(saved['handles'],[11,12,13])
        self.assertEqual(saved['attribute'],m.PROC_THREAD_ATTRIBUTE_HANDLE_LIST)
        self.assertEqual(saved['flags'],0x00000004|0x00000400|0x00080000)
        self.assertEqual(saved['flags'] & 0x01000000,0) # CREATE_BREAKAWAY_FROM_JOB prohibited
        self.assertTrue(saved['inherit']);self.assertIsNone(saved['psa']);self.assertIsNone(saved['tsa'])
        self.assertEqual(saved['command'],m.subprocess.list2cmdline(argv))
        self.assertEqual(saved['exe'],argv[0])
        self.assertTrue(saved['env'].startswith('A=first\0Z=last\0\0'))
        self.assertEqual(calls[-1],'attributes-delete')

    def test_attribute_list_freed_on_create_failure(self):
        m=adapter();events=[]
        class Kernel:
            def InitializeProcThreadAttributeList(self,ptr,n,f,size):
                C.cast(size,C.POINTER(m.SIZE_T))[0]=64;return ptr is not None
            def UpdateProcThreadAttribute(self,*args):return 1
            def CreateProcessW(self,*args):events.append('create-failed');return 0
            def DeleteProcThreadAttributeList(self,*args):events.append('delete')
        api=m.NativeAPI.__new__(m.NativeAPI);api.k=Kernel()
        with mock.patch.object(C,'get_last_error',return_value=122,create=True), mock.patch.object(C,'set_last_error',create=True):
            with self.assertRaises(OSError):api.create_suspended(['C:\\app.exe'],'C:\\private',{},1,2,3)
        self.assertEqual(events,['create-failed','delete'])

    def test_job_limit_is_kill_on_close_and_configuration_failure_closes_job(self):
        m=adapter();events=[]
        class Kernel:
            def CreateJobObjectW(self,sa,name):
                self_test.assertIsNone(sa);self_test.assertIsNone(name);return 7
            def SetInformationJobObject(self,job,kind,info,size):
                value=C.cast(info,C.POINTER(m.JOBOBJECT_EXTENDED_LIMIT_INFORMATION)).contents
                self_test.assertEqual(value.BasicLimitInformation.LimitFlags,0x2000)
                self_test.assertEqual(kind,9);return 0
            def CloseHandle(self,h):events.append(('close',h));return 1
        self_test=self;api=m.NativeAPI.__new__(m.NativeAPI);api.k=Kernel()
        with mock.patch.object(C,'get_last_error',return_value=5,create=True):
            with self.assertRaises(OSError):api.create_job()
        self.assertEqual(events,[('close',7)])

    def test_exit259_requires_signaled_handle_not_still_active_inference(self):
        m=adapter()
        class Kernel:
            state=258
            def WaitForSingleObject(self,h,wait):self_test.assertEqual(wait,0);return self.state
            def GetExitCodeProcess(self,h,out):C.cast(out,C.POINTER(m.DWORD))[0]=259;return 1
        self_test=self;api=m.NativeAPI.__new__(m.NativeAPI);api.k=Kernel()
        self.assertIsNone(api.poll(4));api.k.state=0;self.assertEqual(api.poll(4),259)

    def test_pipe_read_handle_is_not_inherited_and_failure_closes_both(self):
        m=adapter();closed=[]
        class Kernel:
            def CreatePipe(self,r,w,sa,size):
                self_test.assertTrue(C.cast(sa,C.POINTER(m.SECURITY_ATTRIBUTES)).contents.bInheritHandle)
                C.cast(r,C.POINTER(m.HANDLE))[0]=51;C.cast(w,C.POINTER(m.HANDLE))[0]=52;return 1
            def SetHandleInformation(self,h,mask,flags):
                self_test.assertEqual((h.value,mask,flags),(51,1,0));return 0
            def CloseHandle(self,h):closed.append(h.value if hasattr(h,'value') else h);return 1
        self_test=self;api=m.NativeAPI.__new__(m.NativeAPI);api.k=Kernel()
        with mock.patch.object(C,'get_last_error',return_value=5,create=True):
            with self.assertRaises(OSError):api.pipe()
        self.assertEqual(closed,[51,52])

    def test_environment_is_explicit_and_rejects_ambiguous_or_injected_keys(self):
        m=adapter();self.assertEqual(m.environment_block({'z':'2','A':'1'}),'A=1\0z=2\0\0')
        for env in ({'Path':'a','PATH':'b'},{'X=Y':'z'},{'X':'nul\0data'},{'':'z'}):
            with self.assertRaises(ValueError):m.environment_block(env)


if __name__=='__main__': unittest.main()
