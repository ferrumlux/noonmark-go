"""Fixed native smoke child ownership on Windows; not desktop automation.

NativeAPI is intentionally injectable for Linux failure-order tests. Methods
return opaque handles: create_job(), pipe()->(read, write), nul(),
create_suspended(argv,cwd,env,stdin,stdout,stderr)->(process,thread,pid),
assign(job,process), resume(thread), close(handle), poll(process)->None/exit,
terminate(process), terminate_job(job), active_processes(job)->int and
read_pipe(handle,limit)->None (not ready), bytes (data), b'' (EOF).
Allocating API methods either return ownership or close their partial handles.
Only Windows real-host runs can validate these Win32 calls and GUI capability.
"""
import ctypes as C
import os
import subprocess
import time

MAX_LOG = 1 << 20
CREATE_SUSPENDED = 0x00000004
CREATE_UNICODE_ENVIRONMENT = 0x00000400
EXTENDED_STARTUPINFO_PRESENT = 0x00080000
JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x00002000
PROC_THREAD_ATTRIBUTE_HANDLE_LIST = 0x00020002
STARTF_USESTDHANDLES = 0x00000100
ERROR_BROKEN_PIPE = 109
ERROR_NO_DATA = 232

# Fixed-width Win32 types, also inspectable on a non-Windows test host.
DWORD = C.c_uint32
BOOL = C.c_int32
WORD = C.c_uint16
HANDLE = C.c_void_p
SIZE_T = C.c_size_t
ULONG_PTR = C.c_size_t
LARGE_INTEGER = C.c_int64


class SECURITY_ATTRIBUTES(C.Structure):
    _fields_ = [('nLength', DWORD), ('lpSecurityDescriptor', C.c_void_p), ('bInheritHandle', BOOL)]


class STARTUPINFOW(C.Structure):
    _fields_ = [('cb', DWORD), ('lpReserved', C.c_wchar_p), ('lpDesktop', C.c_wchar_p),
                ('lpTitle', C.c_wchar_p), ('dwX', DWORD), ('dwY', DWORD),
                ('dwXSize', DWORD), ('dwYSize', DWORD), ('dwXCountChars', DWORD),
                ('dwYCountChars', DWORD), ('dwFillAttribute', DWORD), ('dwFlags', DWORD),
                ('wShowWindow', WORD), ('cbReserved2', WORD), ('lpReserved2', C.c_void_p),
                ('hStdInput', HANDLE), ('hStdOutput', HANDLE), ('hStdError', HANDLE)]


class STARTUPINFOEXW(C.Structure):
    _fields_ = [('StartupInfo', STARTUPINFOW), ('lpAttributeList', C.c_void_p)]


class PROCESS_INFORMATION(C.Structure):
    _fields_ = [('hProcess', HANDLE), ('hThread', HANDLE), ('dwProcessId', DWORD), ('dwThreadId', DWORD)]


class JOBOBJECT_BASIC_LIMIT_INFORMATION(C.Structure):
    _fields_ = [('PerProcessUserTimeLimit', LARGE_INTEGER), ('PerJobUserTimeLimit', LARGE_INTEGER),
                ('LimitFlags', DWORD), ('MinimumWorkingSetSize', SIZE_T), ('MaximumWorkingSetSize', SIZE_T),
                ('ActiveProcessLimit', DWORD), ('Affinity', ULONG_PTR), ('PriorityClass', DWORD),
                ('SchedulingClass', DWORD)]


class IO_COUNTERS(C.Structure):
    _fields_ = [(name, C.c_uint64) for name in ('ReadOperationCount', 'WriteOperationCount',
                'OtherOperationCount', 'ReadTransferCount', 'WriteTransferCount', 'OtherTransferCount')]


class JOBOBJECT_EXTENDED_LIMIT_INFORMATION(C.Structure):
    _fields_ = [('BasicLimitInformation', JOBOBJECT_BASIC_LIMIT_INFORMATION), ('IoInfo', IO_COUNTERS),
                ('ProcessMemoryLimit', SIZE_T), ('JobMemoryLimit', SIZE_T),
                ('PeakProcessMemoryUsed', SIZE_T), ('PeakJobMemoryUsed', SIZE_T)]


class JOBOBJECT_BASIC_ACCOUNTING_INFORMATION(C.Structure):
    _fields_ = [('TotalUserTime', LARGE_INTEGER), ('TotalKernelTime', LARGE_INTEGER),
                ('ThisPeriodTotalUserTime', LARGE_INTEGER), ('ThisPeriodTotalKernelTime', LARGE_INTEGER),
                ('TotalPageFaultCount', DWORD), ('TotalProcesses', DWORD),
                ('ActiveProcesses', DWORD), ('TotalTerminatedProcesses', DWORD)]


def environment_block(env):
    """Case-insensitive unique names, sorted UTF-16 environment, no implicit inheritance."""
    seen = set()
    rows = []
    for key, value in env.items():
        if not isinstance(key, str) or not isinstance(value, str) or not key or '=' in key or '\0' in key + value:
            raise ValueError('invalid controlled Windows environment')
        folded = key.upper()
        if folded in seen:
            raise ValueError('duplicate case-insensitive Windows environment key')
        seen.add(folded)
        rows.append((folded, key + '=' + value))
    return '\0'.join(row for _, row in sorted(rows)) + '\0\0'


class NativeAPI:
    def __init__(self):
        if os.name != 'nt':
            raise ValueError('Windows blocked: native adapter requires a Windows host')
        self.k = C.WinDLL('kernel32', use_last_error=True)
        signatures = {
            'CloseHandle': ([HANDLE], BOOL),
            'CreateJobObjectW': ([C.c_void_p, C.c_wchar_p], HANDLE),
            'SetInformationJobObject': ([HANDLE, C.c_int, C.c_void_p, DWORD], BOOL),
            'QueryInformationJobObject': ([HANDLE, C.c_int, C.c_void_p, DWORD, C.c_void_p], BOOL),
            'AssignProcessToJobObject': ([HANDLE, HANDLE], BOOL),
            'TerminateJobObject': ([HANDLE, C.c_uint], BOOL),
            'CreatePipe': ([C.POINTER(HANDLE), C.POINTER(HANDLE), C.POINTER(SECURITY_ATTRIBUTES), DWORD], BOOL),
            'SetHandleInformation': ([HANDLE, DWORD, DWORD], BOOL),
            'CreateFileW': ([C.c_wchar_p, DWORD, DWORD, C.POINTER(SECURITY_ATTRIBUTES), DWORD, DWORD, HANDLE], HANDLE),
            'InitializeProcThreadAttributeList': ([C.c_void_p, DWORD, DWORD, C.POINTER(SIZE_T)], BOOL),
            'UpdateProcThreadAttribute': ([C.c_void_p, DWORD, SIZE_T, C.c_void_p, SIZE_T, C.c_void_p, C.c_void_p], BOOL),
            'DeleteProcThreadAttributeList': ([C.c_void_p], None),
            'CreateProcessW': ([C.c_wchar_p, C.c_wchar_p, C.c_void_p, C.c_void_p, BOOL, DWORD,
                                C.c_void_p, C.c_wchar_p, C.POINTER(STARTUPINFOEXW), C.POINTER(PROCESS_INFORMATION)], BOOL),
            'ResumeThread': ([HANDLE], DWORD),
            'WaitForSingleObject': ([HANDLE, DWORD], DWORD),
            'GetExitCodeProcess': ([HANDLE, C.POINTER(DWORD)], BOOL),
            'TerminateProcess': ([HANDLE, C.c_uint], BOOL),
            'PeekNamedPipe': ([HANDLE, C.c_void_p, DWORD, C.c_void_p, C.POINTER(DWORD), C.c_void_p], BOOL),
            'ReadFile': ([HANDLE, C.c_void_p, DWORD, C.POINTER(DWORD), C.c_void_p], BOOL),
        }
        for name, (args, result) in signatures.items():
            fn = getattr(self.k, name)
            fn.argtypes, fn.restype = args, result

    @staticmethod
    def error(action):
        return OSError(C.get_last_error(), action + ' failed')

    def check(self, result, action):
        if not result:
            raise self.error(action)
        return result

    def close(self, handle):
        self.check(self.k.CloseHandle(handle), 'CloseHandle')

    def create_job(self):
        job = self.check(self.k.CreateJobObjectW(None, None), 'CreateJobObjectW')
        try:
            limits = JOBOBJECT_EXTENDED_LIMIT_INFORMATION()
            limits.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
            self.check(self.k.SetInformationJobObject(job, 9, C.byref(limits), C.sizeof(limits)), 'SetInformationJobObject')
            return job
        except BaseException:
            self.close(job)
            raise

    def pipe(self):
        read, write = HANDLE(), HANDLE()
        sa = SECURITY_ATTRIBUTES(C.sizeof(SECURITY_ATTRIBUTES), None, True)
        self.check(self.k.CreatePipe(C.byref(read), C.byref(write), C.byref(sa), 0), 'CreatePipe')
        try:
            # Only the write side is inherited, and only via HANDLE_LIST.
            self.check(self.k.SetHandleInformation(read, 1, 0), 'SetHandleInformation')
            return read.value, write.value
        except BaseException:
            try:
                self.close(read)
            finally:
                self.close(write)
            raise

    def nul(self):
        sa = SECURITY_ATTRIBUTES(C.sizeof(SECURITY_ATTRIBUTES), None, True)
        handle = self.k.CreateFileW('NUL', 0x80000000, 3, C.byref(sa), 3, 0x80, None)
        if handle == C.c_void_p(-1).value:
            raise self.error('CreateFileW(NUL)')
        return handle

    def create_suspended(self, argv, cwd, env, stdin, stdout, stderr):
        if not argv or any(not isinstance(arg, str) or '\0' in arg for arg in argv):
            raise ValueError('invalid fixed Windows argv')
        block = C.create_unicode_buffer(environment_block(env))
        command = C.create_unicode_buffer(subprocess.list2cmdline(argv))
        size = SIZE_T()
        C.set_last_error(0)
        sized = self.k.InitializeProcThreadAttributeList(None, 1, 0, C.byref(size))
        if sized or C.get_last_error() != 122 or not size.value:
            raise self.error('InitializeProcThreadAttributeList size')
        storage = C.create_string_buffer(size.value)
        self.check(self.k.InitializeProcThreadAttributeList(storage, 1, 0, C.byref(size)), 'InitializeProcThreadAttributeList')
        try:
            handles = (HANDLE * 3)(stdin, stdout, stderr)
            self.check(self.k.UpdateProcThreadAttribute(storage, 0, PROC_THREAD_ATTRIBUTE_HANDLE_LIST,
                       handles, C.sizeof(handles), None, None), 'UpdateProcThreadAttribute HANDLE_LIST')
            info = STARTUPINFOEXW()
            info.StartupInfo.cb = C.sizeof(info)
            info.StartupInfo.dwFlags = STARTF_USESTDHANDLES
            info.StartupInfo.hStdInput, info.StartupInfo.hStdOutput, info.StartupInfo.hStdError = stdin, stdout, stderr
            info.lpAttributeList = C.cast(storage, C.c_void_p)
            process = PROCESS_INFORMATION()
            flags = CREATE_SUSPENDED | CREATE_UNICODE_ENVIRONMENT | EXTENDED_STARTUPINFO_PRESENT
            self.check(self.k.CreateProcessW(argv[0], command, None, None, True, flags,
                       block, str(cwd), C.byref(info), C.byref(process)), 'CreateProcessW(CREATE_SUSPENDED)')
            return process.hProcess, process.hThread, int(process.dwProcessId)
        finally:
            self.k.DeleteProcThreadAttributeList(storage)

    def assign(self, job, process):
        self.check(self.k.AssignProcessToJobObject(job, process), 'AssignProcessToJobObject (environment blocked; no fallback)')

    def resume(self, thread):
        previous = self.k.ResumeThread(thread)
        if previous != 1:
            raise self.error('ResumeThread unexpected suspension count')

    def poll(self, process):
        state = self.k.WaitForSingleObject(process, 0)
        if state == 258:
            return None
        if state != 0:
            raise self.error('WaitForSingleObject')
        code = DWORD()
        self.check(self.k.GetExitCodeProcess(process, C.byref(code)), 'GetExitCodeProcess')
        return int(code.value)

    def terminate(self, process):
        if not self.k.TerminateProcess(process, 1) and self.poll(process) is None:
            raise self.error('TerminateProcess')

    def terminate_job(self, job):
        self.check(self.k.TerminateJobObject(job, 1), 'TerminateJobObject')

    def active_processes(self, job):
        info = JOBOBJECT_BASIC_ACCOUNTING_INFORMATION()
        self.check(self.k.QueryInformationJobObject(job, 1, C.byref(info), C.sizeof(info), None), 'QueryInformationJobObject')
        return int(info.ActiveProcesses)

    def read_pipe(self, handle, limit):
        available = DWORD()
        if not self.k.PeekNamedPipe(handle, None, 0, None, C.byref(available), None):
            if C.get_last_error() in (ERROR_BROKEN_PIPE, ERROR_NO_DATA):
                return b''
            raise self.error('PeekNamedPipe')
        if not available.value:
            return None
        size = min(limit, available.value)
        buffer, count = C.create_string_buffer(size), DWORD()
        # A sole reader consumes only Peek's available bytes, so ReadFile cannot
        # wait for a future write. This remains a software, not OS hard deadline.
        if not self.k.ReadFile(handle, buffer, size, C.byref(count), None):
            if C.get_last_error() in (ERROR_BROKEN_PIPE, ERROR_NO_DATA):
                return b''
            raise self.error('ReadFile')
        return buffer.raw[:count.value]


class Process:
    def __init__(self, api, handle, pid):
        self.api, self.handle, self.pid = api, handle, pid
        self.returncode = None

    def poll(self):
        if self.returncode is None:
            self.returncode = self.api.poll(self.handle)
        return self.returncode


class WindowsOwned:
    def __init__(self, api=None, deadline=None):
        self.api = api if api is not None else NativeAPI()
        self.deadline = float('inf') if deadline is None else deadline
        self.children, self.streams, self.handles = [], [], []
        self.job, self.failed = None, None

    def fail(self, error):
        if self.failed is None:
            self.failed = str(error)

    def check(self, deadline=None):
        if time.monotonic() >= min(self.deadline, self.deadline if deadline is None else deadline):
            raise TimeoutError('Windows software deadline exceeded')

    def own(self, handle):
        self.handles.append(handle)
        return handle

    def release(self, handle):
        # Keep failed-close handles owned so finally can make one cleanup attempt.
        self.api.close(handle)
        self.handles.remove(handle)

    def start(self, argv, cwd, env, stdout_path, stderr_path, marker=None, pass_fds=()):
        if pass_fds or self.children or self.job is not None:
            raise ValueError('Windows smoke owns exactly one app and fixed std handles')
        try:
            self.check()
            self.job = self.own(self.api.create_job())
            self.check()
            stdout_read, stdout_write = self.api.pipe()
            self.own(stdout_read); self.own(stdout_write)
            stderr_read, stderr_write = self.api.pipe()
            self.own(stderr_read); self.own(stderr_write)
            stdin = self.own(self.api.nul())
            for handle, path, parse in ((stdout_read, stdout_path, marker), (stderr_read, stderr_path, None)):
                output = path.open('xb')
                self.streams.append({'handle': handle, 'output': output, 'size': 0, 'marker': parse, 'eof': False})
            self.check()
            process, thread, pid = self.api.create_suspended(argv, cwd, env, stdin, stdout_write, stderr_write)
            self.own(process); self.own(thread)
            child = Process(self.api, process, pid)
            entry = {'process': child, 'pid': pid, 'assigned_to_job': False, 'resumed': False,
                     'reaped': False, 'job_empty': False, 'exit': None, 'cleanup_incomplete': False}
            self.children.append(entry)
            self.check()
            self.api.assign(self.job, process)
            entry['assigned_to_job'] = True
            self.check()
            self.api.resume(thread)
            entry['resumed'] = True
            self.release(thread)
            for handle in (stdin, stdout_write, stderr_write):
                self.release(handle)
            self.check()
            return child
        except BaseException as error:
            self.fail(error)
            # This includes Assign failure while the child is still suspended.
            # Never Resume after failure, and never return an unmanaged process.
            for entry in self.children:
                try:
                    self.api.terminate(entry['process'].handle)
                except Exception as cleanup_error:
                    entry['cleanup_incomplete'] = True
                    self.fail('cleanup-incomplete: ' + str(cleanup_error))
            raise

    def pump(self, deadline):
        try:
            self._pump(deadline)
        except BaseException as error:
            self.fail(error)
            raise

    def _pump(self, deadline):
        self.check(deadline)
        received = False
        for stream in self.streams:
            if stream['eof']:
                continue
            self.check(deadline)
            data = self.api.read_pipe(stream['handle'], 65536)
            if data is None:
                continue
            if not data:
                stream['eof'] = True
                continue
            received = True
            stream['size'] += len(data)
            if stream['size'] > MAX_LOG:
                raise ValueError('Windows excessive output')
            if stream['output'].write(data) != len(data):
                raise OSError('Windows short log write')
            if stream['marker'] is not None:
                stream['marker'].feed(data, time.monotonic())
        if not received:
            time.sleep(max(0, min(.02, deadline-time.monotonic(), self.deadline-time.monotonic())))
        self.check(deadline)

    def pipes_finished(self, child):
        return bool(self.children) and child is self.children[0]['process'] and all(s['eof'] for s in self.streams)

    def cleanup(self, deadline):
        deadline = min(deadline, self.deadline)
        incomplete = False
        job_empty = self.job is None
        try:
            if self.job is not None:
                try:
                    if self.api.active_processes(self.job):
                        self.api.terminate_job(self.job)
                except Exception as error:
                    incomplete = True
                    self.fail('cleanup-incomplete: ' + str(error))
            for entry in self.children:
                try:
                    if entry['process'].poll() is None:
                        self.api.terminate(entry['process'].handle)
                except Exception as error:
                    incomplete = True
                    self.fail('cleanup-incomplete: ' + str(error))
            # Poll real process termination AND Job accounting before its handle
            # is closed. KILL_ON_JOB_CLOSE alone is not proof of termination.
            while True:
                reaped = True
                for entry in self.children:
                    try:
                        code = entry['process'].poll()
                        entry.update(reaped=code is not None, exit=code)
                        reaped = reaped and code is not None
                    except Exception as error:
                        reaped = False
                        incomplete = True
                        self.fail('cleanup-incomplete: ' + str(error))
                try:
                    job_empty = self.job is None or self.api.active_processes(self.job) == 0
                except Exception as error:
                    incomplete = True
                    self.fail('cleanup-incomplete: ' + str(error))
                if reaped and job_empty:
                    break
                if time.monotonic() >= deadline:
                    incomplete = True
                    self.fail('cleanup-incomplete: process/Job termination unconfirmed')
                    break
                time.sleep(min(.02, max(0, deadline-time.monotonic())))
            # Parent write handles from any failed creation must be closed before
            # EOF can be observed. Preserve only read, process and Job handles.
            preserve = {s['handle'] for s in self.streams} | {e['process'].handle for e in self.children}
            if self.job is not None:
                preserve.add(self.job)
            for handle in list(self.handles):
                if handle not in preserve:
                    try:
                        self.release(handle)
                    except Exception as error:
                        incomplete = True
                        self.fail('cleanup-incomplete: ' + str(error))
            while any(not s['eof'] for s in self.streams) and time.monotonic() < deadline:
                try:
                    self.pump(deadline)
                except Exception as error:
                    incomplete = True
                    self.fail('cleanup-incomplete: ' + str(error))
                    break
            if any(not s['eof'] for s in self.streams):
                incomplete = True
                self.fail('cleanup-incomplete: pipes did not reach EOF')
        finally:
            for stream in self.streams:
                try:
                    stream['output'].close()
                except Exception as error:
                    incomplete = True
                    self.fail('cleanup-incomplete: log close: ' + str(error))
            # Job is closed last; its active count was observed while queryable.
            handles = [h for h in self.handles if h != self.job]
            if self.job in self.handles:
                handles.append(self.job)
            for handle in handles:
                try:
                    self.release(handle)
                except Exception as error:
                    incomplete = True
                    self.fail('cleanup-incomplete: handle close: ' + str(error))
            for entry in self.children:
                entry['job_empty'] = job_empty
                entry['cleanup_incomplete'] = entry['cleanup_incomplete'] or incomplete or not entry['reaped'] or not job_empty
        return [{key: value for key, value in entry.items() if key != 'process'} for entry in self.children]
