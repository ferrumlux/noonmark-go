#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""Prepare authenticated isolated hosted macOS capture; never claims local Swift.

Source fetch alone is network-enabled. Compile and probe commands are OS-sandboxed.
No product source modifications, App launch, persistence, user account or secrets.
"""
from pathlib import Path
import hashlib
import json
import os
import platform
import selectors
import signal
import socket
import subprocess
import sys
import time

HERE=Path(__file__).resolve().parent
REPO=HERE.parents[3]
COMMIT='43e6eefaae3a04be89cab3e234d470111afd80ef'
DEVELOPER='/Applications/Xcode_26.2.app/Contents/Developer'
SOURCE_SHA='eaa83e76297691c122fb018629df44d4659dd0bd1937d092e805b59dfdfe8d35'
class GuardError(Exception): pass
COMMAND_EVIDENCE=[]
RUN_PROVENANCE={}
ACTIVE_WORK=None
SOURCE_CREATION=None

def require(ok,reason):
    if not ok: raise GuardError(reason)

def sha(raw): return hashlib.sha256(raw).hexdigest()

def source_command(work,path):
    return ['/usr/bin/git','-c','core.hooksPath=/dev/null','-c','protocol.file.allow=never',
            '--git-dir='+str(work/'upstream.git'),'cat-file','blob',COMMIT+':'+path]

def source_manifest():
    from bounded import load_json,Limits
    value=authenticate_manifest(load_json(checked(HERE/'spec/SourceManifest-R2.json',SOURCE_SHA,131072),Limits(bytes=131072,depth=8,nodes=4096,string=256,array=64)))
    require(value['commit']==COMMIT,'source-commit');return value

def bounded_run(command,cwd,env,timeout=120,stdout_limit=131072,*,source_path=None):
    """增量实读；仅确切冻结 source 命令可用4MiB，不按大小自动升级。"""
    from gate_receipts import ensure_run,binding_digest
    require(type(command) is list and 1<=len(command)<=128 and all(type(x) is str and 0<len(x.encode())<=4096 and '\0' not in x for x in command),'command-schema')
    require(type(stdout_limit) is int and stdout_limit>0 and timeout>0,'command-limits')
    row=None
    if source_path is None:require(stdout_limit==131072,'ordinary-stdout-limit')
    else:
        rows=source_manifest()['files'];matches=[v for v in rows if v['path']==source_path]
        require(len(rows)==19 and len(matches)==1,'source-command')
        row=matches[0];require(command==source_command(Path(cwd),source_path) and stdout_limit==4194304,'source-command')
    command_work=Path(ACTIVE_WORK if ACTIVE_WORK is not None else cwd)
    require(not command_work.resolve().is_relative_to(REPO.resolve()),'external-command-work-required')
    require(len(COMMAND_EVIDENCE)<256,'provenance-command-limit')
    root=command_work/'run'
    ensure_run(root);binding=binding_digest(root)
    started=time.monotonic_ns()
    proc=subprocess.Popen(command,cwd=cwd,env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,start_new_session=True)
    out=bytearray();tail=bytearray();stdout_count=0;stderr_count=0;deadline=time.monotonic()+timeout
    transport='';complete=False;sel=selectors.DefaultSelector()
    sel.register(proc.stdout,selectors.EVENT_READ,'stdout');sel.register(proc.stderr,selectors.EVENT_READ,'stderr')
    try:
        while sel.get_map():
            require(time.monotonic()<deadline,'timeout')
            for key,_ in sel.select(min(.1,max(0,deadline-time.monotonic()))):
                block=os.read(key.fileobj.fileno(),8192)
                if not block:sel.unregister(key.fileobj);continue
                if key.data=='stdout':
                    stdout_count+=len(block)
                    out.extend(block[:max(0,stdout_limit-len(out))])
                    require(stdout_count<=stdout_limit,'stdout-resource-limit')
                else:
                    stderr_count+=len(block);tail.extend(block)
                    if len(tail)>65536:del tail[:-65536]
                    require(stderr_count<=1048576,'stderr-resource-limit')
        require(proc.wait(timeout=max(.01,deadline-time.monotonic()))==0,'subprocess-rejected')
        complete=True
        if row is not None:
            require(len(out)==row['bytes'] and sha(out)==row['sha256'],'source-blob-hash')
            require(hashlib.sha1(b'blob '+str(len(out)).encode()+b'\0'+out).hexdigest()==row['gitBlob'],'git-blob-hash')
        return bytes(out)
    except BaseException as error:
        transport=str(error) if isinstance(error,GuardError) else 'subprocess-error'
        if proc.poll() is None:
            try:os.killpg(proc.pid,signal.SIGKILL)
            except ProcessLookupError:pass
        proc.wait()
        if isinstance(error,GuardError):error.diagnostic={'reason':str(error),'stdout':bytes(out[-131072:]),'stderr':bytes(tail),'stderrBytes':stderr_count}
        raise
    finally:
        # 任一失败只标partial；不得把manifest声明值当作完整观测值。
        complete=complete and not transport
        item={'schemaVersion':3,'kind':'frozen-source-object' if row is not None else 'ordinary-command',
              'bindingSHA256':binding,'argv':command[:],'pid':proc.pid,'startedNs':started,'endedNs':time.monotonic_ns(),
              'returncode':proc.returncode,'signal':-proc.returncode if proc.returncode is not None and proc.returncode<0 else None,
              'transportError':transport,'stdoutBytes':stdout_count,'stdoutComplete':complete,
              'stdoutSHA256':sha(out) if complete else None,'stderrBytes':stderr_count,'stderr':bytes(tail)}
        if row is None:item['stdout']=bytes(out)
        else:
            item['source']={'path':row['path'],'commit':COMMIT,'gitBlob':row['gitBlob'],'expectedBytes':row['bytes'],'expectedSHA256':row['sha256']}
            item['stdoutGitBlob']=hashlib.sha1(b'blob '+str(len(out)).encode()+b'\0'+out).hexdigest() if complete else None
        COMMAND_EVIDENCE.append(item)
        sel.close();proc.stdout.close();proc.stderr.close()

def check_separate(paths):
    resolved=[]
    for p in paths:
        require(p.is_absolute() and not p.is_symlink(),'path-symlink-or-relative')
        q=p.resolve()
        require(all(q!=r and q not in r.parents and r not in q.parents for r in resolved),'paths-overlap')
        resolved.append(q)

def safe_path(path):
    value=str(path)
    require(value.startswith('/') and not any(c in value for c in ('"','\\','\n','\r','\0')),'sandbox-path')
    return value

def sandbox_policy(work,developer,compile_mode,extra_reads=()):
    work=safe_path(work);developer=safe_path(developer)
    read=['/System/Library','/usr/lib','/usr/bin','/usr/share','/bin',developer,work]
    read.extend(safe_path(p) for p in extra_reads)
    ancestors=sorted({str(parent) for path in read for parent in Path(path).parents})
    # No allow-default or broad /Users, /Volumes, /Library/Keychains. The
    # isolated writable directories are disjoint from the source tree.
    lines=['(version 1)','(deny default)','(deny network*)',
           '(allow process-exec process-fork signal sysctl-read)',
           '(allow mach-lookup (global-name "com.apple.system.logger") (global-name "com.apple.system.notification_center"))',
           '(allow file-read-metadata '+' '.join('(literal "'+safe_path(p)+'")' for p in ancestors)+')',
           '(allow file-read* '+ ' '.join('(subpath "'+p+'")' for p in read)+')',
           '(allow file-read* (literal "/dev/null") (literal "/dev/urandom") (literal "/dev/random"))',
           '(allow file-write* (literal "/dev/null"))']
    writable=('build','cache','tmp','home') if compile_mode else ('run','tmp','home')
    lines.append('(allow file-write* '+' '.join('(subpath "'+work+'/'+p+'")' for p in writable)+')')
    return '\n'.join(lines)+'\n'

def checked(path,digest,limit):
    require(path.is_file() and not path.is_symlink() and path.stat().st_size<=limit,'source-input-boundary')
    with path.open('rb') as f: raw=f.read(limit+1)
    require(len(raw)<=limit and sha(raw)==digest,'source-mismatch');return raw

def authenticate_manifest(value):
    """Exact frozen source admission, shared by runtime and V01 mutations."""
    from bounded import load_json,Limits
    raw=checked(HERE/'spec/SourceManifest-R2.json',SOURCE_SHA,131072)
    expected=load_json(raw,Limits(bytes=131072,depth=8,nodes=4096,string=256,array=64))
    def exact(a,b):
        if type(a) is not type(b):return False
        if type(b) is dict:return set(a)==set(b) and all(exact(a[k],b[k]) for k in b)
        if type(b) is list:return len(a)==len(b) and all(exact(x,y) for x,y in zip(a,b))
        return a==b
    require(exact(value,expected),'source-manifest-mismatch')
    return value

def validate_tool_manifest(path):
    from bounded import load_json, Limits
    raw=path.read_bytes() if path.is_file() and not path.is_symlink() and path.stat().st_size<=131072 else b''
    require(bool(raw),'missing-tool-manifest')
    value=load_json(raw,Limits(bytes=131072,depth=8,nodes=4096,string=256,strings=65536,array=64,members=32),entry='tool-manifest')
    require(set(value)=={'schemaVersion','files'} and value['schemaVersion']==2,'tool-manifest-schema')
    rows=value['files'];require(type(rows) is list and 1<=len(rows)<=45,'tool-manifest-count')
    paths=set()
    for row in rows:
        require(set(row)=={'path','role','gitBlob','sha256','bytes'} and row['role']=='tool' and row['gitBlob']=='','tool-row-schema')
        p=row['path'];require(type(p) is str and len(p.encode())<=160 and p not in paths,'tool-path')
        require(not Path(p).is_absolute() and '..' not in Path(p).parts,'tool-path')
        target=REPO/p;require(target.resolve().is_relative_to(REPO.resolve()),'tool-path')
        raw=checked(target,row['sha256'],4*1024*1024);require(len(raw)==row['bytes'],'tool-bytes');paths.add(p)
    relevant={str(p.relative_to(REPO)) for p in HERE.rglob('*') if p.is_file() and p.name!='tool-manifest.json' and '__pycache__' not in p.parts}
    relevant.add('.github/workflows/swift-snapshot-oracle.yml')
    require(relevant==paths,'tool-manifest-incomplete-or-extra')
    return sha(path.read_bytes())

def environment(work):
    return {'PATH':'/usr/bin:/bin:/usr/sbin:/sbin','HOME':str(work/'home'),'CFFIXED_USER_HOME':str(work/'home'),
            'TMPDIR':str(work/'tmp'),'XDG_CACHE_HOME':str(work/'cache'),'CLANG_MODULE_CACHE_PATH':str(work/'cache/clang'),
            'SWIFT_MODULECACHE_PATH':str(work/'cache/swift'),'XCRUN_CACHE_PATH':str(work/'cache/xcrun'),
            'DEVELOPER_DIR':DEVELOPER,'LANG':'en_US.UTF-8','LC_ALL':'en_US.UTF-8','TZ':'UTC',
            'GIT_CONFIG_NOSYSTEM':'1','GIT_CONFIG_GLOBAL':'/dev/null','GIT_TERMINAL_PROMPT':'0',
            'GIT_ATTR_NOSYSTEM':'1','GIT_CONFIG_COUNT':'0','PYTHONDONTWRITEBYTECODE':'1'}

def fetch_sources(work,manifest,env):
    gitbase=['/usr/bin/git','-c','core.hooksPath=/dev/null','-c','protocol.file.allow=never','--git-dir='+str(work/'upstream.git')]
    def git(*args,limit=131072):return bounded_run([*gitbase,*args],work,env,120,limit)
    git('init','--bare');git('fetch','--no-tags','--depth=1',manifest['repository'],COMMIT)
    for ref,want in [('FETCH_HEAD',COMMIT),(COMMIT+'^{tree}',manifest['rootTree']),
                     (COMMIT+':Sources/NoonmarkCore',manifest['coreTree']),(COMMIT+':Sources/NoonmarkStorage',manifest['storageTree'])]:
        require(git('rev-parse',ref).decode().strip()==want,'source-tree-mismatch')
    rows=manifest['files'];require(len(rows)==19,'source-count')
    core=[r for r in rows if r['path'].startswith('Sources/NoonmarkCore/')]
    storage=[r for r in rows if r['path'].startswith('Sources/NoonmarkStorage/')]
    require(len(core)==14 and len(storage)==2 and sum(r['role']=='compiled' for r in rows)==16,'source-role')
    listing=git('ls-tree',COMMIT+':Sources/NoonmarkCore').decode().splitlines()
    require(sorted(listing)==sorted('100644 blob '+r['gitBlob']+'\t'+Path(r['path']).name for r in core),'core-file-set')
    for row in rows:
        require(git('rev-parse',COMMIT+':'+row['path']).decode().strip()==row['gitBlob'],'source-blob-link')
        raw=bounded_run(source_command(work,row['path']),work,env,120,4*1024*1024,source_path=row['path'])
        require(len(raw)==row['bytes'] and sha(raw)==row['sha256'],'source-blob-hash')
        require(hashlib.sha1(b'blob '+str(len(raw)).encode()+b'\0'+raw).hexdigest()==row['gitBlob'],'git-blob-hash')
        p=work/'source'/row['path'];p.parent.mkdir(parents=True,exist_ok=True)
        with p.open('xb') as f:f.write(raw)
        p.chmod(0o444)
    return core,storage

def guard_evidence(work,compile_prefix,runtime_prefix,env):
    from gate_receipts import binding_digest
    import stat
    forbidden=work.parent/(work.name+'-denied-sentinels');forbidden.mkdir(mode=0o700)
    checks={};facts={}
    def denied(cmd):
        index=len(COMMAND_EVIDENCE)
        try:bounded_run(cmd,work,env,10);blocked=False
        except GuardError as e:blocked=str(e)=='subprocess-rejected'
        require(len(COMMAND_EVIDENCE)==index+1,'isolation-command-missing')
        return blocked,{'commandIndex':index,'argv':cmd}
    for key in ('production','icloud','keychain'):
        p=forbidden/key;p.write_text('synthetic-guard-only\n');name='runtime-'+key+'-denied'
        checks[name],facts[name]=denied([*runtime_prefix,'/bin/cat',str(p)])
    for label,prefix in [('compile',compile_prefix),('runtime',runtime_prefix)]:
        with socket.socket() as listener:
            listener.bind(('127.0.0.1',0));listener.listen(1);listener.settimeout(.1);port=listener.getsockname()[1]
            blocked,fact=denied([*prefix,'/usr/bin/nc','-z','-w','1','127.0.0.1',str(port)])
            try:conn,_=listener.accept();conn.close();accepted=True
            except socket.timeout:accepted=False
            name=label+'-network-denied';checks[name]=blocked and not accepted
            facts[name]=dict(fact,host='127.0.0.1',port=port,accepted=accepted)
    index=len(COMMAND_EVIDENCE);cmd=[*compile_prefix,'/bin/cat',DEVELOPER+'/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk/SDKSettings.json']
    sdk=bounded_run(cmd,work,env,10);checks['sdk-read-allowed']=bool(sdk)
    facts['sdk-read-allowed']={'commandIndex':index,'argv':cmd,'stdoutBytes':len(sdk)}
    files=[];parents=set()
    for row in source_manifest()['files']:
        p=work/'source'/row['path'];v=p.lstat()
        files.append({'path':row['path'],'mode':v.st_mode,'device':v.st_dev,'inode':v.st_ino,'bytes':v.st_size,'symlink':stat.S_ISLNK(v.st_mode)})
        parents.update(str(x) for x in p.parents if x==work/'source' or (work/'source') in x.parents)
    ancestors=[{'path':name,'symlink':Path(name).is_symlink()} for name in sorted(parents)]
    checks['source-no-symlink']=len(files)==19 and all(not x['symlink'] and stat.S_ISREG(x['mode']) for x in files) and all(not x['symlink'] for x in ancestors)
    facts['source-no-symlink']={'root':str(work/'source'),'files':files,'ancestors':ancestors}
    current=(work/'source').lstat();checks['source-root-new']=SOURCE_CREATION is not None and SOURCE_CREATION['beforeAbsent'] and SOURCE_CREATION['device']==current.st_dev and SOURCE_CREATION['inode']==current.st_ino and stat.S_ISDIR(current.st_mode)
    facts['source-root-new']=SOURCE_CREATION
    paths=[work/x for x in ('source','build','cache','home','tmp','run')];check_separate(paths)
    resolved=[str(x.resolve()) for x in paths];facts['paths-no-overlap']={'requested':[str(x) for x in paths],'resolved':resolved}
    checks['paths-no-overlap']=all(x!=y and Path(x) not in Path(y).parents and Path(y) not in Path(x).parents for i,x in enumerate(resolved) for y in resolved[i+1:])
    facts['runtime-home-isolated']={'HOME':env['HOME'],'CFFIXED_USER_HOME':env['CFFIXED_USER_HOME'],'expected':str(work/'home')}
    checks['runtime-home-isolated']=env['HOME']==str(work/'home') and env['CFFIXED_USER_HOME']==env['HOME']
    facts['runtime-temp-isolated']={'TMPDIR':env['TMPDIR'],'expected':str(work/'tmp')}
    checks['runtime-temp-isolated']=env['TMPDIR']==str(work/'tmp')
    checks['artifact-allowlist'],facts['artifact-allowlist']=denied([*runtime_prefix,'/usr/bin/touch',str(forbidden/'escape')])
    payload={'schemaVersion':3,'bindingSHA256':binding_digest(work/'run'),'checks':facts}
    data=_json_bytes(payload);require(len(data)<=131072,'isolation-witness-limit')
    with (work/'isolation-witness.json').open('xb') as handle:handle.write(data)
    require(len(checks)==12 and all(checks.values()),'isolation-guard-failed')
    return [{'check':k,'outcome':'pass' if v else 'fail'} for k,v in sorted(checks.items())]

def prepare():
    global ACTIVE_WORK,SOURCE_CREATION
    from bounded import load_spec
    require(platform.system()=='Darwin','macos-required-swift-not-run')
    require(os.environ.get('GITHUB_ACTIONS')=='true' and os.environ.get('RUNNER_ENVIRONMENT')=='github-hosted','hosted-runner-required')
    require(os.environ.get('DEVELOPER_DIR')==DEVELOPER and Path(DEVELOPER).is_dir(),'xcode-26.2-required')
    tool_sha=validate_tool_manifest(HERE/'tool-manifest.json');RUN_PROVENANCE['toolManifestSHA256']=tool_sha
    # load_spec hashes and token-preflights every frozen attachment before JSON.
    spec=load_spec(HERE/'spec')
    checked(HERE/'spec/SourceManifest-R2.json',SOURCE_SHA,131072)
    manifest=authenticate_manifest(spec['manifest'])
    require(manifest['commit']==COMMIT,'source-commit')
    parent=Path(os.environ.get('RUNNER_TEMP','')).resolve(strict=True)
    require(parent.is_dir() and not parent.is_relative_to(REPO),'external-build-required')
    work=parent/'noonmark-swift-snapshot-oracle'
    require(not work.exists() and not work.is_symlink(),'new-root-required');work.mkdir(mode=0o700);ACTIVE_WORK=work
    SOURCE_CREATION=None
    for name in ('source','build','cache','home','tmp','run'):
        path=work/name;started=time.monotonic_ns();absent=not path.exists() and not path.is_symlink()
        require(absent,'new-directory-required');path.mkdir(mode=0o700);current=path.lstat();ended=time.monotonic_ns()
        if name=='source':SOURCE_CREATION={'path':str(path),'startedNs':started,'endedNs':ended,'beforeAbsent':absent,'mode':current.st_mode,'device':current.st_dev,'inode':current.st_ino}
    from gate_receipts import ensure_run
    ensure_run(work/'run')
    env=environment(work)
    actual_commit=bounded_run(['/usr/bin/git','-c','core.hooksPath=/dev/null','rev-parse','HEAD'],REPO,env).decode().strip()
    require(actual_commit==os.environ.get('GITHUB_SHA'),'runner-commit-mismatch')
    base_tree=bounded_run(['/usr/bin/git','rev-parse',manifest['goCommit']+'^{tree}'],REPO,env).decode().strip()
    RUN_PROVENANCE.update(runnerCommit=actual_commit,goBase=manifest['goCommit'],goBaseTree=base_tree,sourceManifestSHA256=SOURCE_SHA,compiledInputs=[r for r in manifest['files'] if r['role']=='compiled'])
    core,storage=fetch_sources(work,manifest,env)
    compile_deadline=None
    def run(cmd,timeout=120):
        if compile_deadline is not None:
            remaining=compile_deadline-time.monotonic();require(remaining>0,'compile-timeout');timeout=min(timeout,remaining)
        return bounded_run(cmd,work,env,timeout)
    xcode=run(['/usr/bin/xcodebuild','-version']).decode().strip();require(xcode.splitlines()[0]=='Xcode 26.2','xcode-mismatch')
    swift=run(['/usr/bin/xcrun','--no-cache','swiftc','--version']).decode().strip();require('Swift version 6.' in swift,'swift-version')
    sdk=run(['/usr/bin/xcrun','--no-cache','--sdk','macosx','--show-sdk-version']).decode().strip()
    osversion=run(['/usr/bin/sw_vers']).decode().strip();arch=platform.machine();require(arch in ('arm64','x86_64'),'architecture')
    # Compiler discovery precedes sandbox execution; no compiler subprocess is
    # started until every source/tool and deny-policy check has succeeded.
    swiftc=run(['/usr/bin/xcrun','--no-cache','--find','swiftc']).decode().strip()
    compile_sb=work/'compile.sb';runtime_sb=work/'runtime.sb'
    compile_sb.write_text(sandbox_policy(work,DEVELOPER,True,(HERE,)))
    runtime_sb.write_text(sandbox_policy(work,DEVELOPER,False,(HERE,)))
    cp=['/usr/bin/sandbox-exec','-f',str(compile_sb)]
    rp=['/usr/bin/sandbox-exec','-f',str(runtime_sb)]
    guards=guard_evidence(work,cp,rp,env)
    from gates import source_contracts
    source_gate_root=work/'gate-source';source_gate_root.mkdir(mode=0o700)
    from gate_receipts import recording
    with recording(work/'run'):source_rows=source_contracts(spec,source_gate_root)
    source_raw=(json.dumps(source_rows,separators=(',',':'))+'\n').encode()
    require(len(source_raw)<=131072,'source-gates-resource-limit')
    with (work/'source-gates.json').open('xb') as handle:handle.write(source_raw)
    require(source_rows and all(row.get('outcome')=='pass' for row in source_rows),'source-gates-failed')
    build=work/'build';target=arch+'-apple-macosx14.0'
    flags=['-swift-version','6','-target',target,'-module-cache-path',str(work/'cache/swift')]
    compile_deadline=time.monotonic()+900
    for module,rows,extra in [('NoonmarkCore',core,[]),('NoonmarkStorage',storage,['-I',str(build),'-L',str(build),'-lNoonmarkCore'])]:
        run([*cp,swiftc,*flags,'-parse-as-library','-module-name',module,'-package-name','Noonmark',
             '-emit-library','-emit-module','-emit-module-path',str(build/(module+'.swiftmodule')),
             '-o',str(build/('lib'+module+'.dylib')),*extra,*[str(work/'source'/r['path']) for r in rows]],900)
    swiftfiles=['BoundedJSON.swift','FrozenSchema.swift','FrozenInputs.swift','DirectProjection.swift','MutationGate.swift','SnapshotOracle.swift']
    run([*cp,swiftc,*flags,'-parse-as-library','-I',str(build),'-L',str(build),'-lNoonmarkCore','-lNoonmarkStorage',
         '-Xlinker','-rpath','-Xlinker',str(build),*[str(HERE/p) for p in swiftfiles],'-o',str(build/'snapshot-oracle')],900)
    # A clean environment is part of both runtime prefixes; controller does not
    # inherit HOME, credentials or a live user configuration into either probe.
    envprefix=['/usr/bin/env','-i',*[k+'='+v for k,v in sorted(env.items())]]
    metadata={'swiftVersion':swift,'xcodeVersion':xcode,'sdkVersion':sdk,'osVersion':osversion,
              'architecture':arch,'target':target,'locale':'en_US.UTF-8','timezone':'UTC',
              'runnerCommit':os.environ.get('GITHUB_SHA',''),'runnerSHA256':sha(Path(__file__).read_bytes()),
              'harnessSwiftSHA256':sha((HERE/'SnapshotOracle.swift').read_bytes()),
              'compiledSourceManifestSHA256':SOURCE_SHA}
    guardraw=(json.dumps(guards,separators=(',',':'))+'\n').encode()
    scope={k:False for k in ('productionDataAccess','iCloudAccess','keychainAccess','aiNetworkAccess','actualAppExecuted')}
    scope.update(selectedStorageSources=True,guardEvidenceSHA256=sha(guardraw))
    return work,envprefix+rp+[str(build/'snapshot-oracle')],metadata,scope,guardraw,compile_deadline

def persist_diagnostic(work,error):
    from bounded import no_symlink
    import re
    work=no_symlink(work);work.mkdir(parents=True,exist_ok=True)
    details=getattr(error,'diagnostic',{})
    reason=str(error) if isinstance(error,GuardError) else type(error).__name__
    if not re.fullmatch('[a-zA-Z0-9-]{1,80}',reason):reason='harness-error'
    payload={'schemaVersion':2,'reason':reason,'observationAccepted':False,'interopAccepted':False}
    with (work/'failure.json').open('xb') as f:f.write((json.dumps(payload,separators=(',',':'))+'\n').encode())
    for name,key,limit in [('failure-stdout.bin','stdout',131072),('failure-stderr.bin','stderr',65536)]:
        data=details.get(key,b'')[-limit:]
        with (work/name).open('xb') as f:f.write(data)

def persist_provenance(work):
    from bounded import read_file,no_symlink
    from gate_receipts import ensure_run,binding_digest
    ensure_run(work/'run');binding=binding_digest(work/'run')
    directory=no_symlink(work/'provenance');directory.mkdir(exist_ok=True)
    def save(name,data):
        require(type(data) is bytes and len(data)<=131072,'provenance-resource-limit')
        p=directory/name
        if p.exists():require(read_file(p,131072)==data,'provenance-mismatch')
        else:
            with p.open('xb') as f:f.write(data)
        return {'name':'provenance/'+name,'bytes':len(data),'sha256':sha(data)}
    tools=save('tool-manifest.json',read_file(HERE/'tool-manifest.json',131072))
    source=save('source-manifest.json',read_file(HERE/'spec/SourceManifest-R2.json',131072))
    commands=[]
    for index,item in enumerate(COMMAND_EVIDENCE):
        require(index<256 and item.get('bindingSHA256')==binding,'provenance-binding')
        command=dict(item);command['stderr']=save(f'command-{index}-stderr.bin',item['stderr'])
        if item['kind']=='ordinary-command':command['stdout']=save(f'command-{index}-stdout.bin',item['stdout'])
        commands.append(command)
    value={'schemaVersion':3,'bindingSHA256':binding,'run':RUN_PROVENANCE,'toolManifest':tools,'sourceManifest':source,'commands':commands}
    save('index.json',(json.dumps(value,separators=(',',':'))+'\n').encode())
    verify_provenance(work,ensure_run(work/'run'))

def verify_provenance(work,binding):
    """纯读取验证；不创建binding、不启动命令、不访问源仓库或平台。"""
    from bounded import read_file,load_json,Limits
    import re,uuid
    def parse(raw):return load_json(raw,Limits(bytes=131072,depth=12,nodes=16384,string=4096,strings=131072,array=256,members=64))
    raw_binding=read_file(work/'run/run-binding.json',4096);actual=parse(raw_binding)
    keys={'schemaVersion','runID','toolManifestSHA256','sourceManifestSHA256','catalogSHA256'}
    require(type(binding) is dict and set(binding)==keys and actual==binding and type(binding['schemaVersion']) is int and binding['schemaVersion']==3,'provenance-binding')
    require(str(uuid.UUID(binding['runID']))==binding['runID'] and all(re.fullmatch('[0-9a-f]{64}',binding[x]) for x in keys-{'schemaVersion','runID'}),'provenance-binding')
    digest=sha(raw_binding)
    def ref(item,name,limit):
        require(type(item) is dict and set(item)=={'name','bytes','sha256'} and item['name']==name,'provenance-ref')
        raw=read_file(work/name,limit)
        require(type(item['bytes']) is int and len(raw)==item['bytes'] and sha(raw)==item['sha256'],'provenance-ref');return raw
    value=parse(read_file(work/'provenance/index.json',131072))
    require(type(value) is dict and set(value)=={'schemaVersion','bindingSHA256','run','toolManifest','sourceManifest','commands'} and type(value['schemaVersion']) is int and value['schemaVersion']==3 and value['bindingSHA256']==digest,'provenance-schema')
    tools=ref(value['toolManifest'],'provenance/tool-manifest.json',131072)
    manifest_raw=ref(value['sourceManifest'],'provenance/source-manifest.json',131072)
    require(sha(tools)==binding['toolManifestSHA256'] and sha(manifest_raw)==binding['sourceManifestSHA256']==SOURCE_SHA,'provenance-binding')
    manifest=authenticate_manifest(parse(manifest_raw));rows={x['path']:x for x in manifest['files']}
    require(type(value['run']) is dict and set(value['run'])<= {'toolManifestSHA256','runnerCommit','goBase','goBaseTree','sourceManifestSHA256','compiledInputs'},'provenance-run')
    if 'toolManifestSHA256' in value['run']:require(value['run']['toolManifestSHA256']==binding['toolManifestSHA256'],'provenance-run')
    if 'sourceManifestSHA256' in value['run']:require(value['run']['sourceManifestSHA256']==binding['sourceManifestSHA256'],'provenance-run')
    commands=value['commands'];require(type(commands) is list and len(commands)<=256,'provenance-schema')
    common={'schemaVersion','kind','bindingSHA256','argv','pid','startedNs','endedNs','returncode','signal','transportError','stdoutBytes','stdoutComplete','stdoutSHA256','stderrBytes','stderr'}
    for i,item in enumerate(commands):
        require(type(item) is dict and item.get('kind') in ('ordinary-command','frozen-source-object'),'provenance-kind')
        source=item['kind']=='frozen-source-object'
        require(set(item)==common|({'source','stdoutGitBlob'} if source else {'stdout'}),'provenance-schema')
        require(type(item['schemaVersion']) is int and item['schemaVersion']==3 and item['bindingSHA256']==digest,'provenance-binding')
        argv=item['argv'];require(type(argv) is list and 1<=len(argv)<=128 and all(type(x) is str and 0<len(x.encode())<=4096 and '\0' not in x for x in argv),'provenance-argv')
        require(all(type(item[k]) is int for k in ('pid','startedNs','endedNs','returncode','stdoutBytes','stderrBytes')) and item['pid']>0 and 0<item['startedNs']<=item['endedNs'],'provenance-process')
        require(-128<=item['returncode']<=255 and (item['signal'] is None or type(item['signal']) is int) and item['signal']==(-item['returncode'] if item['returncode']<0 else None) and type(item['stdoutComplete']) is bool,'provenance-process')
        require(item['transportError'] in ('','timeout','stdout-resource-limit','stderr-resource-limit','subprocess-rejected','subprocess-error','source-blob-hash','git-blob-hash'),'provenance-process')
        complete=item['stdoutComplete'];require(complete==(item['returncode']==0 and item['transportError']==''),'provenance-process')
        limit=4194304 if source else 131072
        require(0<=item['stdoutBytes']<=limit+8192 and 0<=item['stderrBytes']<=1048576+8192,'provenance-resource-limit')
        err=ref(item['stderr'],f'provenance/command-{i}-stderr.bin',65536)
        require(len(err)==min(item['stderrBytes'],65536),'provenance-stderr')
        if not complete:require(item['stdoutSHA256'] is None,'provenance-partial')
        if source:
            data=item['source'];require(type(data) is dict and set(data)=={'path','commit','gitBlob','expectedBytes','expectedSHA256'} and data.get('path') in rows,'provenance-source')
            row=rows[data['path']];require(data=={'path':row['path'],'commit':COMMIT,'gitBlob':row['gitBlob'],'expectedBytes':row['bytes'],'expectedSHA256':row['sha256']},'provenance-source')
            require(len(argv)==9 and argv[5].startswith('--git-dir=/') and argv[5].endswith('/upstream.git'),'provenance-argv')
            original=Path(argv[5][10:]).parent
            require(argv==source_command(original,row['path']) and '..' not in original.parts and '.' not in original.parts,'provenance-argv')
            if complete:require(item['stdoutBytes']==row['bytes'] and item['stdoutSHA256']==row['sha256'] and item['stdoutGitBlob']==row['gitBlob'],'provenance-source')
            else:require(item['stdoutGitBlob'] is None,'provenance-partial')
        else:
            out=ref(item['stdout'],f'provenance/command-{i}-stdout.bin',131072)
            require(len(out)==min(item['stdoutBytes'],131072),'provenance-stdout')
            if complete:require(item['stdoutSHA256']==sha(out),'provenance-stdout')
            require(not (len(argv)==9 and argv[0]=='/usr/bin/git' and argv[6:8]==['cat-file','blob'] and argv[-1].startswith(COMMIT+':') and argv[-1].split(':',1)[1] in rows),'provenance-kind')
    return commands

def _artifact_read(work,name,limit):
    """固定相对路径仅打开一次；逐层dirfd拒绝符号链接及读取期间变化。"""
    import stat,errno
    require(type(name) is str and 0<len(name.encode())<=200 and not Path(name).is_absolute() and all(x not in ('','.','..') for x in name.split('/')),'artifact-path')
    opened=[];observed=None
    try:
        fd=os.open(work,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW);opened.append(fd)
        for part in name.split('/')[:-1]:
            fd=os.open(part,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=fd);opened.append(fd)
        fd=os.open(name.split('/')[-1],os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK,dir_fd=fd);opened.append(fd)
        before=os.fstat(fd);observed=before.st_size
        if not stat.S_ISREG(before.st_mode):return None,'nonregular',observed
        if observed>limit:return None,'item-limit',observed
        data=bytearray()
        while len(data)<=limit:
            block=os.read(fd,min(8192,limit+1-len(data)))
            if not block:break
            data.extend(block)
        after=os.fstat(fd)
        if len(data)>limit:return None,'item-limit',after.st_size
        if (before.st_dev,before.st_ino,before.st_size,before.st_mtime_ns,before.st_ctime_ns)!=(after.st_dev,after.st_ino,after.st_size,after.st_mtime_ns,after.st_ctime_ns) or len(data)!=observed:return None,'changed',after.st_size
        return bytes(data),None,observed
    except FileNotFoundError:return None,'absent',None
    except OSError as error:return None,'unsafe-path' if error.errno in (errno.ELOOP,errno.ENOTDIR) else 'read-error',observed
    finally:
        for fd in reversed(opened):os.close(fd)

def _json_bytes(value):return (json.dumps(value,separators=(',',':'),ensure_ascii=False)+'\n').encode()

class _Payloads(dict):
    """Immutable bytes and memoized digests; names cannot be replaced after admission."""
    def __init__(self):
        super().__init__();self._digests={}
    def __setitem__(self,name,data):
        require(name not in self and type(data) is bytes,'payload-replacement')
        super().__setitem__(name,data);self._digests[name]=(data,sha(data))
    def digest(self,name):
        data,digest=self._digests[name]
        require(self[name] is data,'payload-replacement')
        return digest

def _payload_digest(payloads,name):
    return payloads.digest(name) if isinstance(payloads,_Payloads) else sha(payloads[name])

import weakref
_PLAN_SEAL=object()
_ISSUED_PLANS={}
_APPROVED_BUNDLES={}
class _SealedPlan:
    __slots__=('members','headers','budget_bytes','fallback','seal','__weakref__')
    def __init__(self,members,headers,budget_bytes,fallback,seal):
        for name,value in zip(('members','headers','budget_bytes','fallback','seal'),(members,headers,budget_bytes,fallback,seal)):
            object.__setattr__(self,name,value)
    def __setattr__(self,name,value):raise TypeError('immutable sealed plan')
    def __getitem__(self,name):
        for key,data in self.members:
            if key==name:return data
        raise KeyError(name)

def _register_plan(plan):
    # Independent immutable issuance state; never trust mutable object slots alone.
    key=id(plan)
    state=(plan.members,plan.headers,plan.budget_bytes,plan.fallback,plan.seal)
    _ISSUED_PLANS[key]=(weakref.ref(plan,lambda ref,k=key:_ISSUED_PLANS.pop(k,None)),state)

def _issued_plan_state(plan):
    require(type(plan) is _SealedPlan,'sealed-plan-required')
    issued=_ISSUED_PLANS.get(id(plan))
    require(issued is not None and issued[0]() is plan,'sealed-plan-required')
    state=issued[1]
    actual=(plan.members,plan.headers,plan.budget_bytes,plan.fallback,plan.seal)
    require(all(a is b for a,b in zip(actual,state)),'sealed-plan-mutated')
    return state

def _package_plan(payloads,retained,exclusions,complete,fallback,accounting=None,diagnostic_limit=None):
    """含原件、元数据、USTAR头／补齐和最终副本的精确固定点账本。"""
    from gate_receipts import TOTAL_BYTES,NORMAL_BYTES,DIAGNOSTIC_RESERVE
    import tarfile
    require(type(retained) is int and retained>=sum(len(d) for d in payloads.values()),'retained-budget')
    require(len(payloads)<=8192,'candidate-count-limit')
    def row(name,data):return {'path':name,'bytes':len(data),'sha256':sha(data)}
    payload_rows=[{'path':n,'bytes':len(d),'sha256':_payload_digest(payloads,n)} for n,d in payloads.items()]
    # 最小包仅在保留额容纳不了完整排除明细时明确省略后段诊断，不冒称完整包。
    included=exclusions if diagnostic_limit is None else exclusions[:diagnostic_limit]
    omitted=exclusions[len(included):]
    meta={};parts=[]
    for index in range((len(included)+255)//256):
        name=f'exclusions-{index}.json';data=_json_bytes({'schemaVersion':3,'items':included[index*256:(index+1)*256]})
        require(len(data)<=131072,'diagnostic-resource-limit');meta[name]=data;parts.append(row(name,data))
    meta['exclusions.json']=_json_bytes({'schemaVersion':3,'count':len(exclusions),'parts':parts,'omittedDiagnosticCount':len(omitted),'omittedDiagnosticsSHA256':sha(_json_bytes(omitted)) if omitted else None})
    if fallback:meta['packaging-failure.json']=_json_bytes({'schemaVersion':3,'reason':'archive-incomplete','observationAccepted':False,'interopAccepted':False})
    budget={'schemaVersion':3,'totalLimit':TOTAL_BYTES,'normalLimit':NORMAL_BYTES,'diagnosticReserve':DIAGNOSTIC_RESERVE,
            'retainedRawBytes':retained,'metadataBytes':0,'bundlePayloadBytes':0,'tarHeaderBytes':0,'tarPaddingBytes':0,
            'tarEndBytes':1024,'tarRecordPaddingBytes':0,'bundleBytes':0,'totalBytes':0,'complete':complete,'fallback':fallback,
            'admittedCount':len(payloads),'excludedCount':len(exclusions),
            'observedExcludedBytes':sum(x.get('observedBytes',0) for x in exclusions),
            'unknownExcludedCount':sum('observedBytes' not in x for x in exclusions),
            'omittedDiagnosticCount':len(omitted),
            'accounting':accounting or {'state':'valid','declaredEvidenceBytes':None,'safeCandidateBytes':retained}}
    for _ in range(16):
        generated=dict(meta);generated['budget.json']=_json_bytes(budget)
        entries=payload_rows+[row(n,d) for n,d in generated.items()];parts=[]
        for i in range((len(entries)+255)//256):
            name=f'upload-manifest-{i}.json';data=_json_bytes({'schemaVersion':2,'files':entries[i*256:(i+1)*256]})
            require(len(data)<=131072,'manifest-resource-limit');generated[name]=data;parts.append(row(name,data))
        generated['upload-manifest.json']=_json_bytes({'schemaVersion':2,'parts':parts})
        require(not set(payloads).intersection(generated),'metadata-collision')
        if len(generated)>64 or any(len(d)>131072 for d in generated.values()) or sum(map(len,generated.values()))>8388608:
            if fallback and len(included)>256:return _package_plan(payloads,retained,exclusions,complete,fallback,accounting,256)
            raise GuardError('metadata-resource-limit')
        all_data={**payloads,**generated};size=sum(map(len,all_data.values()));headers=512*len(all_data);padding=sum((-len(x))%512 for x in all_data.values())
        raw_tar=size+headers+padding+1024;record_padding=(-raw_tar)%10240;bundle=raw_tar+record_padding
        updated=dict(budget,metadataBytes=sum(map(len,generated.values())),bundlePayloadBytes=size,tarHeaderBytes=headers,tarPaddingBytes=padding,tarRecordPaddingBytes=record_padding,bundleBytes=bundle)
        updated['totalBytes']=retained+updated['metadataBytes']+bundle
        if updated==budget:
            fits=(budget['totalBytes']<=TOTAL_BYTES and budget['metadataBytes']+bundle<=DIAGNOSTIC_RESERVE) if fallback else budget['totalBytes']<=NORMAL_BYTES
            if not fits and fallback and len(included)>256:
                return _package_plan(payloads,retained,exclusions,complete,fallback,accounting,256)
            require(fits,'bundle-resource-limit')
            headers=[]
            for name,data in all_data.items():
                require(type(name) is str and len(name.encode())<=240 and not any(c in name for c in ('\0','\n','\r')) and not name.startswith('/') and all(x not in ('','.','..') for x in name.split('/')) and type(data) is bytes,'artifact-path')
                info=tarfile.TarInfo(name);info.size=len(data);info.mode=0o600;info.mtime=0
                headers.append(info.tobuf(format=tarfile.USTAR_FORMAT))
            plan=_SealedPlan(tuple(all_data.items()),tuple(headers),_json_bytes(budget),fallback,_PLAN_SEAL)
            _register_plan(plan)
            return plan,json.loads(plan.budget_bytes)
        budget=updated
    raise GuardError('budget-not-stable')

def _write_tar(path,plan,*,fallback=False):
    import tarfile,io
    from gate_receipts import NORMAL_BYTES,TOTAL_BYTES,DIAGNOSTIC_RESERVE
    members,headers,budget_bytes,planned_fallback,seal=_issued_plan_state(plan)
    require(seal is _PLAN_SEAL and planned_fallback is fallback,'sealed-plan-required')
    budget=json.loads(budget_bytes)
    require(budget['totalBytes']==budget['retainedRawBytes']+budget['metadataBytes']+budget['bundleBytes'],'sealed-plan-budget')
    require(budget['totalBytes']<=TOTAL_BYTES and (budget['metadataBytes']+budget['bundleBytes']<=DIAGNOSTIC_RESERVE if fallback else budget['totalBytes']<=NORMAL_BYTES),'bundle-resource-limit')
    require(len(members)==len(headers),'sealed-plan-members')
    # Validate all USTAR headers before exclusive creation; no PAX fallback.
    for (name,data),header in zip(members,headers):
        info=tarfile.TarInfo(name);info.size=len(data);info.mode=0o600;info.mtime=0
        require(info.tobuf(format=tarfile.USTAR_FORMAT)==header,'sealed-plan-header')
    class LimitedWriter:
        def __init__(self,target):self.target=target;self.count=0;self.reserved=0;self.digest=hashlib.sha256()
        def write(self,data):
            require(self.count+len(data)<=budget['bundleBytes'],'bundle-write-limit')
            self.reserved=self.count+len(data)
            written=self.target.write(data)
            require(type(written) is int and 0<=written<=len(data),'bundle-write-result')
            self.count+=written;self.digest.update(data[:written])
            require(written==len(data),'bundle-short-write');return written
        def tell(self):return self.count
    target=path.open('xb',buffering=0);identity=os.fstat(target.fileno());owned=(identity.st_dev,identity.st_ino)
    try:
        with target:
            try:
                output=LimitedWriter(target)
                if not fallback:
                    with tarfile.open(fileobj=output,mode='w',format=tarfile.USTAR_FORMAT) as archive:
                        for name,data in members:
                            info=tarfile.TarInfo(name);info.size=len(data);info.mode=0o600;info.mtime=0
                            archive.addfile(info,io.BytesIO(data))
                else:
                    for (_,data),header in zip(members,headers):
                        output.write(header);output.write(data);output.write(b'\0'*((-len(data))%512))
                    output.write(b'\0'*1024);output.write(b'\0'*((-output.tell())%10240))
                require(output.count==budget['bundleBytes'],'bundle-size')
                target.flush();before=_bundle_stamp(os.fstat(target.fileno()));os.fsync(target.fileno())
                require(_bundle_stamp(os.fstat(target.fileno()))==before,'bundle-changed')
                _verify_written_bundle(path,owned,budget['bundleBytes'],output.digest.hexdigest(),before)
            except Exception as error:
                try:error.created_bundle_bytes=max(os.fstat(target.fileno()).st_size,output.count)
                except (OSError,ValueError):error.created_bundle_bytes=max(output.count,output.reserved)
                raise
    except Exception as error:
        error.created_bundle=owned
        if not hasattr(error,'created_bundle_bytes'):error.created_bundle_bytes=max(output.count,output.reserved) if 'output' in locals() else 0
        raise
    return owned

def _bundle_stamp(value):
    return (value.st_dev,value.st_ino,value.st_size,value.st_mtime_ns,value.st_ctime_ns)

def _verify_written_bundle(path,owned,expected,digest,stamp):
    import stat
    fd=os.open(path,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK)
    try:
        before=os.fstat(fd)
        require(stat.S_ISREG(before.st_mode) and (before.st_dev,before.st_ino)==owned and _bundle_stamp(before)==stamp,'bundle-identity')
        measured=hashlib.sha256();count=0
        while True:
            block=os.read(fd,min(65536,expected-count+1))
            if not block:break
            count+=len(block);require(count<=expected,'bundle-size');measured.update(block)
        require(count==expected and measured.hexdigest()==digest,'bundle-content')
        require(_bundle_stamp(os.fstat(fd))==stamp and _bundle_stamp(path.lstat())==stamp,'bundle-changed')
        _APPROVED_BUNDLES[owned]=stamp
    finally:os.close(fd)

def _verify_bundle_identity(path,owned,expected):
    import stat
    current=path.lstat()
    require(stat.S_ISREG(current.st_mode) and (current.st_dev,current.st_ino)==owned and current.st_size==expected and _bundle_stamp(current)==_APPROVED_BUNDLES.get(owned),'bundle-identity')


def verify_selected_closure(selected,allowed):
    """只核已选内存bytes的引用闭包，绝不重新读文件或扩张允许名单。"""
    from bounded import load_json,Limits,validate_capture
    import re
    problems=[];seen=set()
    def problem(owner,reason):
        if (owner,reason) not in seen:seen.add((owner,reason));problems.append({'path':owner,'reason':reason})
    def reference(owner,base,value):
        if type(value) is not dict or set(value)!={'name','bytes','sha256'}:
            problem(owner,'reference-schema');return
        name=value['name']
        if type(name) is not str or not name or len(name.encode())>200 or name.startswith('/') or any(x in ('','.','..') for x in name.split('/')) or base+name not in allowed:
            problem(owner,'reference-allowlist');return
        target=base+name
        if target not in selected:problem(target,'reference-missing');return
        if type(value['bytes']) is not int or value['bytes']!=len(selected[target]) or value['sha256']!=_payload_digest(selected,target):problem(target,'reference-hash')
    def walk(owner,base,value,skip_result=False):
        if type(value) is dict:
            if {'name','bytes','sha256'}<=set(value):reference(owner,base,value);return
            for key,child in value.items():
                if skip_result and key=='result':continue
                walk(owner,base,child,skip_result)
        elif type(value) is list:
            for child in value:walk(owner,base,child,skip_result)
    for name,data in selected.items():
        capture=re.fullmatch(r'run/round-[123]/[a-z0-9-]+\.capture\.json',name)
        execution_part=re.fullmatch(r'run/execution-(receipts|directions)-part-([0-2])\.json',name)
        receipt=name.startswith('run/receipts/') and name.endswith('.json')
        gates=name in ('run/gate-children.json','run/gate-inputs.json','run/negative-gates.json') or name.startswith(('run/gate-children/','run/gate-children-index-'))
        if not (capture or execution_part or receipt or gates or name in ('provenance/index.json','run/execution-receipts.json','provenance/dependency-controller-transport.json','provenance/dependency-downloads.json','provenance/dependency-completion.json')):continue
        try:
            value=load_json(data,Limits(bytes=262144,depth=32,nodes=32768,string=8192,strings=262144,array=2048,members=128))
            require(type(value) is dict,'reference-schema')
            if name=='run/execution-receipts.json':
                require(set(value)=={'schemaVersion','bindingSHA256','receiptParts','directionParts'} and value['schemaVersion']==3,'reference-schema')
                for field,group,maximum in (('receiptParts','receipts',3),('directionParts','directions',2)):
                    refs=value[field];require(type(refs) is list and len(refs)<=maximum,'reference-schema')
                    for i,ref in enumerate(refs):require(type(ref) is dict and ref.get('name')==f'execution-{group}-part-{i}.json','reference-schema')
            if execution_part:
                require(set(value)=={'schemaVersion','bindingSHA256','items'} and value['schemaVersion']==3 and type(value['items']) is list and 1<=len(value['items'])<=128,'reference-schema')
            if receipt:
                if '-artifacts-' in name:require(set(value)=={'schemaVersion','files'} and type(value['files']) is list,'reference-schema')
                elif name.endswith('-direction.json'):require(value.get('schemaVersion')==3 and set(value)=={'schemaVersion','bindingSHA256','id','processReceiptSHA256','input','decoded','verdict','comparisons'},'reference-schema')
                else:require(value.get('schemaVersion')==3 and set(value)=={'schemaVersion','bindingSHA256','resourceWitness','requestedTimeoutNs','artifactManifests','id','mode','side','metadata','inputs','returncode','transportError','argv','executionWitness','outcome','reason','stdout','stderr','stderrBytes','artifacts','schemaValidWitness'},'reference-schema')
            if capture:validate_capture(data)
            base=(name.rsplit('/',1)[0]+'/') if capture else '' if name.startswith('provenance/') else 'run/'
            # archive-operation.result.artifact使用该操作自身的相对目录；其完整root引用已在payload.artifacts。
            walk(name,base,value,skip_result=value.get('kind')=='archive-operation')
        except Exception:problem(name,'reference-schema')
    return problems

def prepare_upload(work,report=None,guard_raw=None):
    """逐件隔离poison，失败也保留总80MiB内的封闭最小包；返回显式状态。"""
    from bounded import load_spec,no_symlink
    from gates import MUTATIONS
    from gate_receipts import TOTAL_BYTES,NORMAL_BYTES
    work=no_symlink(work);root=work/'run';selected=_Payloads();exclusions=[];excluded_names=set();total=0;safe_total=0;safe_committed=0;seen=set()
    def exclude(name,reason,limit=0,size=None):
        if name in excluded_names:return
        excluded_names.add(name)
        item={'path':name,'kind':'artifact','limit':limit,'complete':False,'reason':reason}
        if size is not None:item['observedBytes']=size
        exclusions.append(item)
    try:spec=load_spec(HERE/'spec')
    except Exception:spec={'registry':{'cases':[]}};exclude('spec','spec-unavailable')
    def add(path,limit=262144,required=False):
        nonlocal total,safe_total,safe_committed
        name=str(path.relative_to(work)) if path.is_relative_to(work) else 'invalid-path'
        if name in seen:return
        seen.add(name)
        try:data,reason,size=_artifact_read(work,name,limit)
        except (GuardError,ValueError):data,reason,size=None,'unsafe-path',None
        if reason:
            if reason!='absent' or required:exclude(name,'required-missing' if reason=='absent' else reason,limit,0 if reason=='absent' else size)
            return
        safe_total+=len(data)
        if safe_committed+len(data)<=NORMAL_BYTES:safe_committed+=len(data)
        else:exclude(name,'uncommitted-raw-budget',limit,size);return
        # Bounded candidates are not tar-admitted until the sealed exact plan.
        if len(selected)>=8192:exclude(name,'candidate-count-limit',limit,size);return
        selected[name]=data;total+=len(data)
    for name in ('failure.json','failure-stdout.bin','failure-stderr.bin','source-gates.json','isolation-witness.json','compile.sb','runtime.sb','go-compile.sb'):add(work/name)
    for name,limit in i1_acquisition_artifact_paths():add(work/name,limit)
    for name in ('summary.json','verification.json','run-context.json','guard-evidence.json','negative-gates.json','execution-receipts.json'):add(root/name)
    for group,count in (('receipts',3),('directions',2)):
        for i in range(count):add(root/f'execution-{group}-part-{i}.json',131072)
    def receipt(ident,direction=False):
        for suffix in ('.json','-stdout.bin','-stderr.bin'):add(root/'receipts'/(ident+suffix))
        if direction:add(root/'receipts'/(ident+'-direction.json'))
    layers=('swift-engine-projection','go-kernel-projection','go-from-domain-projection','swift-package','go-package','swift-export','go-export','swift-self-reencoded','go-self-reencoded')
    for repeat in range(1,4):
        rr=root/f'round-{repeat}'
        for case in spec['registry']['cases']:
            cid=case['id'];keys=['seed']+[c['commandKey'] for c in case['steps']];base=rr/('work-'+cid)
            add(rr/(cid+'.capture.json'),131072);add(base/'case.json')
            for side in ('swift','go'):
                add(base/side/'result.json',131072);receipt(f'r{repeat}-{cid}-{side}-case')
                for key in keys:
                    cross=base/(key+'-cross-'+side)
                    add(cross/'result.json',131072);add(cross/'decoded.json');receipt(f'r{repeat}-{cid}-{key}-{side}-cross',True)
            for key in keys:
                for layer in layers:add(rr/cid/key/(layer+'.json'));add(base/layer.split('-')[0]/key/(layer+'.json'))
    from gates import PY_ENTRY_ALL
    for mode in ('normal','bypass','late'):
        add(root/('python-ingress-'+mode+'.json'),131072)
        for entry in PY_ENTRY_ALL:add(root/('python-ingress-'+mode+'-'+entry+'.json'),131072)
    for side in ('swift','go'):
        for fault_mode in ('normal','bypass','late'):
            name='ingress-'+side+'-'+fault_mode;receipt(name)
            from process import ingress_paths
            for relative in ingress_paths(side):add(root/name/relative,131072 if relative.endswith('result.json') else 262144)
            for i in range(7):add(root/'receipts'/(name+'-artifacts-'+str(i)+'.json'),131072)
        for mutation in (*MUTATIONS,'control-valid'):
            name='mutation-'+side+'-'+mutation;add(root/name/'result.json',131072);receipt(name)
    for sender,receiver in (('swift','go'),('go','swift')):
        for mutation in ('whitespace','escape','key-order','array-order'):
            name='cross-negative-'+receiver+'-v04-'+sender+'-'+mutation
            add(root/(name+'.json'));add(root/name/'result.json',131072);add(root/name/'decoded.json');receipt(name)
    # 来源记录失败不能静默忽略：排除收据令整次结果强制失败。
    try:persist_provenance(work)
    except Exception:exclude('provenance/index.json','provenance-failed',131072)
    from bounded import load_json,Limits,read_file
    try:tool_count=len(load_json(read_file(HERE/'tool-manifest.json',131072),Limits(bytes=131072,depth=8,nodes=4096,string=256,array=64))['files'])
    except Exception:tool_count=0;exclude('provenance/tool-manifest.json','manifest-unavailable',131072)
    if not 0<=tool_count<=45:exclude('provenance/tool-manifest.json','manifest-count',131072);tool_count=0
    for i in range(tool_count):add(work/'gate-source'/f'tool-mutation-{i}.json',131072)
    for shape in ('missing','extra','duplicate'):add(work/'gate-source'/f'tool-structural-{shape}.json',131072)
    for name in ('index.json','tool-manifest.json','source-manifest.json'):add(work/'provenance'/name,131072,required=True)
    for i in range(min(len(COMMAND_EVIDENCE),256)):
        add(work/'provenance'/f'command-{i}-stderr.bin',65536,required=True)
        if COMMAND_EVIDENCE[i].get('kind')=='ordinary-command':add(work/'provenance'/f'command-{i}-stdout.bin',131072,required=True)
    from gate_receipts import exact_upload_paths
    for name,limit in exact_upload_paths():
        required=name in ('run-binding.json','observation-start.json','gate-children.json','gate-inputs.json','gate-checks/unexpected-output/unexpected.json') or name.startswith(('gate-children/','gate-children-index-','gate-resources/process-'))
        if name.startswith('gate-resources/archive-'):
            positive=name.split('/')[1] in ('archive-positive','archive-malformed')
            required=(name.endswith('swift-package.json') if positive else name.endswith('diagnostic-observe-empty-0.json'))
        add(root/name,limit,required=report is not None and required)
    if report is not None:
        # 已访问过的核心路径另外核缺失，不重复读取。
        for name in ('source-gates.json','isolation-witness.json','run/summary.json','run/run-context.json','run/guard-evidence.json','run/negative-gates.json','run/execution-receipts.json'):
            if name not in selected and name not in excluded_names:exclude(name,'required-missing',131072)
    for issue in verify_selected_closure(selected,seen):exclude(issue['path'],issue['reason'])
    declared=None;untrusted=safe_total>NORMAL_BYTES
    if report is not None:
        declared=report.get('evidenceBytesAccounted') if type(report) is dict else None
        if type(declared) is not int or not 0<=declared<=NORMAL_BYTES:
            exclude('run/summary.json','untrusted-budget');declared=None;untrusted=True
    if safe_total>NORMAL_BYTES:exclude('budget.json','uncommitted-raw-budget',NORMAL_BYTES)
    retained=max(safe_committed,declared or 0)
    accounting={'state':'untrusted' if untrusted else 'valid','declaredEvidenceBytes':declared,'safeCandidateBytes':safe_total}
    complete=not exclusions and 'failure.json' not in selected and report is not None
    bundle=work/'synthetic-evidence.tar';fallback=False
    owned=None
    try:
        require(not untrusted,'untrusted-budget')
        payloads,budget=_package_plan(selected,retained,exclusions,complete,False,accounting)
        owned=_write_tar(bundle,payloads)
        _verify_bundle_identity(bundle,owned,budget['bundleBytes'])
    except Exception as package_error:
        import stat
        try:bundle_stat=bundle.lstat()
        except FileNotFoundError:bundle_stat=None
        exclude('synthetic-evidence.tar','total-budget' if isinstance(package_error,GuardError) and str(package_error) in ('bundle-resource-limit','untrusted-budget') else 'archive-failed',TOTAL_BYTES,bundle_stat.st_size if bundle_stat is not None else None)
        complete=False;fallback=True
        safe=_Payloads()
        for n,d in selected.items():
            if n in ('failure.json','failure-stdout.bin','failure-stderr.bin','run/run-binding.json'):
                dict.__setitem__(safe,n,d);safe._digests[n]=selected._digests[n]
        for name,data in selected.items():
            if name not in safe:exclude(name,'archive-omitted',len(data),len(data))
        # 只清理本次exclusive创建且inode仍相同的partial；预先存在的poison不读不删。
        identity=owned or getattr(package_error,'created_bundle',None)
        partial=getattr(package_error,'created_bundle_bytes',budget['bundleBytes'] if owned is not None else 0)
        if bundle_stat is not None and stat.S_ISREG(bundle_stat.st_mode) and identity==(bundle_stat.st_dev,bundle_stat.st_ino):
            partial=bundle_stat.st_size
            try:bundle.unlink();partial=0
            except OSError:pass
        payloads,budget=_package_plan(safe,retained+partial,exclusions,False,True,accounting)
        bundle=work/'synthetic-evidence-failure.tar'
        fallback_owned=_write_tar(bundle,payloads,fallback=True)
        _verify_bundle_identity(bundle,fallback_owned,budget['bundleBytes'])
        require(budget['totalBytes']<=TOTAL_BYTES,'bundle-resource-limit')
    # 元数据以tar中的完整原件为准；不覆盖work中的任何同名文件。
    output=os.environ.get('GITHUB_OUTPUT');require(output is not None,'github-output-required')
    _verify_bundle_identity(bundle,fallback_owned if fallback else owned,budget['bundleBytes'])
    with open(output,'a',encoding='utf-8') as handle:handle.write('artifact_paths='+str(bundle)+'\n')
    return {'bundle':bundle,'exclusions':exclusions,'budget':budget,'complete':complete}

def observation_ready(verified):
    s=verified.get('summary',{})
    return (s.get('reportComplete') is True and s.get('observationAccepted') is True
            and type(s.get('effectivePointCount')) is int and s['effectivePointCount']==32
            and type(s.get('negativeGatePassCount')) is int and s['negativeGatePassCount']==9
            and verified.get('negativeGates')=={f'V{i:02}':'pass' for i in range(1,10)})

def main():
    global ACTIVE_WORK
    ACTIVE_WORK=None;COMMAND_EVIDENCE.clear();RUN_PROVENANCE.clear()
    work=None;report=None;guardraw=None
    try:
        require(len(sys.argv)==1,'no-runtime-arguments')
        i1_require_source_execution_bound()
        work,swift_command,metadata,scope,guardraw,compile_deadline=prepare()
        import controller
        from bounded import load_spec
        from offline import verify_run
        remaining=compile_deadline-time.monotonic();require(remaining>0,'compile-timeout')
        go_command,go_metadata=controller.prepare_go_probe(work,timeout=remaining);metadata.update(go_metadata)
        persist_provenance(work)
        report=controller.run_suite(swift_command,go_command,work/'run',metadata,scope,guard_raw=guardraw)
        persist_provenance(work)
        verified=verify_run(work/'run',load_spec(HERE/'spec'))
        (work/'run').mkdir(exist_ok=True)
        with (work/'run/verification.json').open('xb') as f:f.write((json.dumps(verified,separators=(',',':'))+'\n').encode())
        packaging=prepare_upload(work,report,guardraw)
        print(json.dumps(verified['summary'],sort_keys=True))
        return 0 if observation_ready(verified) and packaging['complete'] else 1
    except Exception as error:
        if work is None:work=ACTIVE_WORK
        if work is None:
            parent=Path(os.environ.get('RUNNER_TEMP','/tmp')).resolve()
            work=parent/('noonmark-snapshot-failure-'+str(os.getpid()))
        try:
            persist_diagnostic(work,error);prepare_upload(work,report,guardraw)
        except Exception as evidence_error:
            print('snapshot-oracle: failure-evidence-'+type(evidence_error).__name__,file=sys.stderr)
        print('snapshot-oracle: '+type(error).__name__,file=sys.stderr)
        return 1
# EXTERNAL I1 DRAFT. Embed in runner.py only after independent review.
import base64 as i1_base64
import hashlib as i1_hashlib
import http.client as i1_http
import io as i1_io
import json as i1_json
import os as i1_os
from pathlib import Path as i1_Path
import re as i1_re
import ssl as i1_ssl
import stat as i1_stat
import time as i1_time
from types import MappingProxyType as i1_MappingProxyType
import urllib.parse as i1_url
import zipfile as i1_zipfile

I1_DEP_PREFIX = 'golang.org/x/text@v0.43.0/'
I1_DEP_SUM = 'h1:1QivrlhwAsnMOcqgOdl9my7s9OyZl8O5/UGUTJVXoWc='
I1_DEP_MOD_SUM = 'h1:hbSIYA/amXcRXTFSZqh8tWHEoqRvvbKPJ2uz/KU+a+s='
I1_DEP_IDENTITIES = i1_MappingProxyType({
 'zip': (7337809, 'b75131f628f6e2e4892fd638d33127e952a5027775e0162b94ba069cb4e13863'),
 'mod': (190, '63270b8ca6290a720212e1d6e58b8d12a8aa315a1cb960411d7ec2998c0aab59'),
 'info': (191, '9bcbfda1bbd803b06a02932a06d8a1ba56e2bc944ce30674e415e010b48580fb')})
I1_DEP_URLS = i1_MappingProxyType({k: 'https://proxy.golang.org/golang.org/x/text/@v/v0.43.0.'+k for k in ('info','mod','zip')})

class i1_DependencyError(ValueError):
    def __init__(self, message, receipt=None, partials=()):
        super().__init__(message)
        self.receipt = receipt
        self.partials = partials


def i1_require(condition, message):
    if not condition:
        raise i1_DependencyError(message)


def i1_sha(data):
    return i1_hashlib.sha256(data).hexdigest()


def i1_freeze(value):
    if isinstance(value, dict):
        return i1_MappingProxyType({k:i1_freeze(v) for k,v in value.items()})
    if isinstance(value, (list, tuple)):
        return tuple(i1_freeze(v) for v in value)
    return value


def i1_hash1(files):
    h = i1_hashlib.sha256()
    for name, data in sorted(files, key=lambda item:item[0].encode('utf-8')):
        i1_require('\n' not in name, 'Hash1 newline filename')
        h.update((i1_sha(data)+'  '+name+'\n').encode('utf-8'))
    return 'h1:'+i1_base64.b64encode(h.digest()).decode('ascii')


def i1_safe_path(name):
    i1_require(isinstance(name, str) and 0 < len(name.encode('utf-8')) <= 240, 'path length')
    parts=name.split('/')
    i1_require(len(parts)<=16 and all(p not in ('','.','..') for p in parts), 'path traversal/depth')
    i1_require(all(ord(c)>=32 and ord(c)!=127 and c not in '\\:*?"<>|' for c in name), 'invalid path character')
    i1_require(all(not p.endswith(('.', ' ')) for p in parts), 'invalid path suffix')
    return parts


def i1_zip_payload(zip_bytes):
    # Exact raw identity is additionally enforced by authenticate; this helper is
    # independently exercised on malformed ZIPs, without relaxing production API.
    i1_require(type(zip_bytes) is bytes and len(zip_bytes)<=8388608, 'ZIP byte limit')
    files={}; nodes={}; total=0
    try:
        with i1_zipfile.ZipFile(i1_io.BytesIO(zip_bytes)) as archive:
            entries=archive.infolist()
            i1_require(len(entries)==487, 'ZIP entry count')
            for ent in entries:
                name=ent.filename
                i1_require(name==ent.orig_filename and name.startswith(I1_DEP_PREFIX), 'ZIP prefix/NUL')
                rel=name[len(I1_DEP_PREFIX):]; parts=i1_safe_path(rel)
                mode=ent.external_attr>>16
                i1_require(not ent.is_dir() and not (ent.external_attr & 0x10), 'ZIP directory')
                i1_require(i1_stat.S_IFMT(mode) in (0,i1_stat.S_IFREG), 'ZIP nonregular')
                i1_require(not ent.flag_bits & 1 and ent.compress_type in (0,8), 'ZIP encrypted/codec')
                i1_require(0<=ent.file_size<=8388608, 'ZIP entry size')
                i1_require(rel not in files, 'ZIP duplicate')
                i1_require(parts[-1].lower()!='go.mod' or rel=='go.mod', 'ZIP nested/case go.mod')
                for n in range(1,len(parts)+1):
                    node='/'.join(parts[:n]); key=node.casefold(); kind='f' if n==len(parts) else 'd'
                    i1_require(key not in nodes or nodes[key]==(node,kind), 'ZIP case/type collision')
                    nodes[key]=(node,kind)
                i1_require(sum(k=='d' for _,k in nodes.values())<=128, 'ZIP directory limit')
                i1_require(total+ent.file_size<=33554432, 'ZIP expanded limit')
                with archive.open(ent) as stream:
                    chunks=[]; count=0
                    while True:
                        chunk=stream.read(min(65536,ent.file_size-count+1))
                        if not chunk: break
                        count+=len(chunk)
                        i1_require(count<=ent.file_size, 'ZIP actual size')
                        chunks.append(chunk)
                i1_require(count==ent.file_size, 'ZIP truncated entry')
                data=b''.join(chunks);files[rel]=data;total+=count
    except (i1_zipfile.BadZipFile, RuntimeError, EOFError, OSError) as exc:
        raise i1_DependencyError('ZIP read/CRC failure') from exc
    i1_require(len(files)==487 and total==29575637, 'ZIP exact expanded inventory')
    i1_require(i1_hash1((I1_DEP_PREFIX+n,b) for n,b in files.items())==I1_DEP_SUM, 'ZIP Hash1 mismatch')
    return files


def i1_authenticate_dependency_bytes(zip_bytes, mod_bytes, info_bytes):
    raw={'zip':zip_bytes,'mod':mod_bytes,'info':info_bytes}
    for kind,data in raw.items():
        i1_require(type(data) is bytes, 'dependency requires immutable bytes')
        size,digest=I1_DEP_IDENTITIES[kind]
        i1_require(len(data)==size and i1_sha(data)==digest, kind+' fixed identity mismatch')
    i1_require(i1_hash1([('go.mod',mod_bytes)])==I1_DEP_MOD_SUM, 'go.mod Hash1 mismatch')
    files=i1_zip_payload(zip_bytes)
    i1_require(files.get('go.mod')==mod_bytes, 'ZIP/download go.mod mismatch')
    # info raw identity above authenticates the exact frozen official JSON, not
    # arbitrary server-provided Version/Origin claims.
    expected={'golang.org/x/text@v0.43.0/'+n:b for n,b in files.items()}
    for kind,data in raw.items():
        expected['cache/download/golang.org/x/text/@v/v0.43.0.'+kind]=data
    expected['cache/download/golang.org/x/text/@v/v0.43.0.ziphash']=I1_DEP_SUM.encode('ascii')
    records=tuple({'path':n,'bytes':len(b),'sha256':i1_sha(b),'mode':292} for n,b in sorted(expected.items()))
    return i1_freeze({'raw':raw,'files':files,'cacheFiles':expected,'manifest':records,'sum':I1_DEP_SUM,'goModSum':I1_DEP_MOD_SUM})


def i1_check_directory_chain(path):
    path=i1_Path(path).absolute()
    for p in reversed((path,)+tuple(path.parents)):
        s=p.lstat()
        i1_require(i1_stat.S_ISDIR(s.st_mode) and not i1_stat.S_ISLNK(s.st_mode), 'directory ancestor type')
    return path


def i1_materialize_dependency(work, authenticated_records):
    # Reauthenticate caller-supplied records; never accept an invented manifest.
    raw=authenticated_records['raw']
    auth=i1_authenticate_dependency_bytes(raw['zip'],raw['mod'],raw['info'])
    work=i1_check_directory_chain(work)
    cache=work/'cache'
    if not cache.exists(): cache.mkdir(mode=0o700)
    i1_check_directory_chain(cache)
    root=cache/'go-mod'
    i1_require(not root.exists() and not root.is_symlink(), 'module cache must be fresh')
    root.mkdir(mode=0o700)
    for rel,data in auth['cacheFiles'].items():
        target=root/rel;target.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
        i1_check_directory_chain(target.parent)
        fd=i1_os.open(target,i1_os.O_WRONLY|i1_os.O_CREAT|i1_os.O_EXCL|i1_os.O_NOFOLLOW,0o600)
        with i1_os.fdopen(fd,'wb') as stream: stream.write(data)
        target.chmod(0o444)
    for parent, dirs, files in i1_os.walk(root,topdown=False,followlinks=False):
        i1_Path(parent).chmod(0o555)
    return i1_verify_dependency_cache(work,auth)


def i1_verify_dependency_cache(work, authenticated_records):
    raw=authenticated_records['raw']
    auth=i1_authenticate_dependency_bytes(raw['zip'],raw['mod'],raw['info'])
    root=i1_check_directory_chain(i1_Path(work)/'cache'/'go-mod')
    expected=auth['cacheFiles'];actual={};directories=set();total=0
    expected_dirs=set()
    for rel in expected:
        p=i1_Path(rel).parent
        while str(p)!='.':expected_dirs.add(p.as_posix());p=p.parent
    stack=[root]
    while stack:
        directory=stack.pop()
        i1_require(i1_stat.S_IMODE(directory.lstat().st_mode)==0o555,'cache directory mode')
        with i1_os.scandir(directory) as iterator:
            for entry in iterator:
                rel=i1_Path(entry.path).relative_to(root).as_posix();i1_safe_path(rel)
                s=entry.stat(follow_symlinks=False)
                if i1_stat.S_ISDIR(s.st_mode):
                    i1_require(rel in expected_dirs,'extra cache directory');directories.add(rel);stack.append(i1_Path(entry.path))
                else:
                    i1_require(i1_stat.S_ISREG(s.st_mode) and s.st_nlink==1,'cache nonregular/hardlink')
                    i1_require(rel in expected and i1_stat.S_IMODE(s.st_mode)==0o444,'extra cache file/mode')
                    i1_require(s.st_size==len(expected[rel]),'cache file size')
                    fd=i1_os.open(entry.path,i1_os.O_RDONLY|i1_os.O_NOFOLLOW)
                    with i1_os.fdopen(fd,'rb') as stream:
                        before=i1_os.fstat(stream.fileno());data=stream.read(len(expected[rel])+1);after=i1_os.fstat(stream.fileno())
                    i1_require((s.st_dev,s.st_ino,s.st_size,s.st_mtime_ns,s.st_ctime_ns)==(before.st_dev,before.st_ino,before.st_size,before.st_mtime_ns,before.st_ctime_ns)==(after.st_dev,after.st_ino,after.st_size,after.st_mtime_ns,after.st_ctime_ns),'cache concurrent mutation')
                    i1_require(data==expected[rel],'cache byte mismatch');actual[rel]=data;total+=len(data)
                i1_require(len(actual)<=600 and len(directories)<=128 and total<=41951232,'cache aggregate limit')
    i1_require(set(actual)==set(expected) and directories==expected_dirs,'cache missing inventory')
    i1_require(i1_hash1((I1_DEP_PREFIX+n,actual['golang.org/x/text@v0.43.0/'+n]) for n in auth['files'])==I1_DEP_SUM,'expanded cache Hash1')
    return i1_freeze({'files':auth['manifest'],'fileCount':len(actual),'directoryCount':len(directories),'totalBytes':total})


def i1_validate_dependency_url(url, kind, initial=False):
    p=i1_url.urlsplit(url)
    i1_require(p.scheme=='https' and p.username is None and p.password is None and p.port in (None,443) and not p.fragment,'HTTPS URL authority')
    i1_require(not any(ord(c)<33 or ord(c)==127 for c in url), 'URL control/whitespace')
    if initial:
        i1_require(url==I1_DEP_URLS[kind], 'initial URL mismatch')
    else:
        i1_require(kind=='zip' and p.hostname=='storage.googleapis.com' and p.netloc in ('storage.googleapis.com','storage.googleapis.com:443'), 'redirect host/resource')
        prefix='/proxy-golang-org-prod/'
        tail=p.path[len(prefix):] if p.path.startswith(prefix) else ''
        decoded=i1_url.unquote(tail,errors='strict')
        i1_require(bool(i1_re.fullmatch(r'[A-Za-z0-9._~:@!$&()+,;=%-]+',tail)) and decoded not in ('.','..') and '/' not in decoded and '\\' not in decoded and all(32<ord(c)<127 for c in decoded), 'redirect object path')
    return p


def i1_dependency_artifacts(authenticated_records):
    """Return bytes only; parent owns provenance directory writes and schema refs."""
    raw=authenticated_records['raw']
    auth=i1_authenticate_dependency_bytes(raw['zip'],raw['mod'],raw['info'])
    artifacts={}
    for number,offset in enumerate(range(0,len(raw['zip']),131072)):
        artifacts['dependency-zip-%03d.bin'%number]=raw['zip'][offset:offset+131072]
    i1_require(len(artifacts)==56,'dependency ZIP chunk count')
    artifacts['dependency-module.mod']=raw['mod']
    artifacts['dependency-module.info']=raw['info']
    # Fixed <=64 rows per manifest part; no source path is used as artifact name.
    records=[dict(row) for row in auth['manifest']]
    for number,offset in enumerate(range(0,len(records),64)):
        artifacts['dependency-expanded-%02d.json'%number]=i1_json.dumps(records[offset:offset+64],sort_keys=True,separators=(',',':')).encode('utf-8')
    i1_require(len(artifacts)==66 and all(len(b)<=131072 for b in artifacts.values()),'dependency artifact inventory limit')
    inventory=tuple({'name':n,'bytes':len(b),'sha256':i1_sha(b)} for n,b in sorted(artifacts.items()))
    return i1_freeze({'artifacts':artifacts,'inventory':inventory})

# D1 §8: parent fixes this to a reviewed public CA file for the actual platform.
# None deliberately blocks fetch. Never use SSL_CERT_FILE/DIR or user Keychain.
I1_DEP_PUBLIC_CA_FILE = None
I1_DEP_PART_NAMES = i1_MappingProxyType({k: 'v0.43.0.'+k+'.part' for k in ('info','mod','zip')})


def i1_child_response_bytes(response):
    """Closed response construction only; no file writes. Parent emits once."""
    nodes=0
    def visit(value, depth):
        nonlocal nodes
        nodes+=1
        i1_require(nodes<=512 and depth<=8,'child response structure limit')
        if isinstance(value,str):
            i1_require(len(value.encode('utf-8'))<=240,'child response string limit')
        elif isinstance(value,dict):
            for key,item in value.items():visit(key,depth+1);visit(item,depth+1)
        elif isinstance(value,(list,tuple)):
            for item in value:visit(item,depth+1)
        else:
            i1_require(value is None or type(value) in (int,bool),'child response scalar type')
    visit(response,0)
    encoded=i1_json.dumps(response,sort_keys=True,separators=(',',':'),ensure_ascii=True).encode('ascii')
    i1_require(len(encoded)<=16384,'child response byte limit')
    return encoded


def i1_child_request(request):
    required={'schemaVersion','bindingSHA256','integrationPolicySHA256','toolManifestSHA256','runID','parentStartedNs','deadlineNs'}
    i1_require(type(request) is dict and set(request)==required,'child request fields')
    i1_require(type(request['schemaVersion']) is int and request['schemaVersion']==1,'child request schema')
    for field in ('bindingSHA256','integrationPolicySHA256','toolManifestSHA256'):
        i1_require(type(request[field]) is str and bool(i1_re.fullmatch('[0-9a-f]{64}',request[field])),'child request digest')
    i1_require(type(request['runID']) is str and 0<len(request['runID'].encode('utf-8'))<=240 and all(32<ord(c)<127 for c in request['runID']),'child request run ID')
    start=request['parentStartedNs'];deadline=request['deadlineNs']
    i1_require(type(start) is int and type(deadline) is int and 0<start<2**63 and deadline<2**63 and deadline-start==120000000000,'child request deadline')
    i1_require(len(i1_json.dumps(request,separators=(',',':')).encode('utf-8'))<=4096,'child request byte limit')
    return deadline


def i1_child_fetch(request, work):
    """D1 child-only fetch. Returns JSON bytes, never authenticates/promotes cache.

    Parent dispatch must first authenticate runner/tool/policy/binding/run identity,
    enforce raw stdin <=4096 and reject duplicate keys. It must supervise this
    child in the reviewed macOS sandbox with actual three-control evidence.
    The absolute deadline here is ADVISORY. Only the parent's owned process-group
    supervisor can terminate blocked DNS/headers and establish timely reap.
    """
    # Invalid request cannot safely claim an authenticated binding in failure JSON.
    # Raise without response; parent records protocol/controller failure.
    deadline=i1_child_request(request)
    common={'schemaVersion':1,'bindingSHA256':request['bindingSHA256'],'integrationPolicySHA256':request['integrationPolicySHA256']}
    completed=[];partial=None;transaction_count=0;total=0
    try:
        work=i1_check_directory_chain(work)
        i1_require(work==i1_Path.cwd().absolute(),'child cwd/work mismatch')
        stage=i1_check_directory_chain(work/'input'/'dependency-download')
        i1_require(not any(stage.iterdir()),'child staging must be empty')
        # Parent stages this directory; child never mkdir/rename/write receipts.
        i1_require(type(I1_DEP_PUBLIC_CA_FILE) is str and i1_Path(I1_DEP_PUBLIC_CA_FILE).is_absolute(),'reviewed public CA path missing')
        context=i1_ssl.SSLContext(i1_ssl.PROTOCOL_TLS_CLIENT)
        context.load_verify_locations(cafile=I1_DEP_PUBLIC_CA_FILE)
        i1_require(context.check_hostname and context.verify_mode==i1_ssl.CERT_REQUIRED,'TLS verification required')
        for kind in ('info','mod','zip'):
            limit=8388608 if kind=='zip' else 4096
            relative='input/dependency-download/'+I1_DEP_PART_NAMES[kind]
            digest=i1_hashlib.sha256();count=0;receipts=[]
            partial={'kind':kind,'relativePath':relative,'bytes':0,'sha256':digest.hexdigest(),'complete':False,'httpReceipts':receipts}
            # No proxy/cookie/netrc/credential machinery: direct stdlib connection,
            # explicit HTTPS context, only the fixed request headers below.
            fd=i1_os.open(stage/I1_DEP_PART_NAMES[kind],i1_os.O_WRONLY|i1_os.O_CREAT|i1_os.O_EXCL|i1_os.O_NOFOLLOW,0o600)
            with i1_os.fdopen(fd,'wb',buffering=0) as output:
                url=I1_DEP_URLS[kind];redirects=0
                while True:
                    parsed=i1_validate_dependency_url(url,kind,initial=(redirects==0))
                    i1_require(len(parsed.path.encode('utf-8'))<=240,'HTTP path receipt bound')
                    now=i1_time.monotonic_ns()
                    i1_require(now<deadline,'child advisory deadline')
                    i1_require(transaction_count<5,'HTTP transaction limit')
                    record={'ordinal':transaction_count,'requestKind':kind,'requestHost':parsed.hostname,'requestPath':parsed.path,'responseHost':parsed.hostname,'responsePath':parsed.path,'statusCode':None,'startedNs':now,'endedNs':None,'receivedBytes':0,'complete':False,'error':None}
                    transaction_count+=1;receipts.append(record)
                    connection=None
                    try:
                        connection=i1_http.HTTPSConnection(parsed.hostname,443,timeout=(deadline-now)/1000000000,context=context)
                        target=parsed.path+('?' +parsed.query if parsed.query else '')
                        connection.request('GET',target,headers={'Accept-Encoding':'identity','Connection':'close'})
                        response=connection.getresponse();record['statusCode']=response.status
                        i1_require(i1_time.monotonic_ns()<deadline,'child advisory deadline')
                        if response.status in (301,302,303,307,308):
                            i1_require(kind=='zip' and redirects<2,'HTTP redirect count/resource')
                            locations=response.headers.get_all('Location',[])
                            i1_require(len(locations)==1,'HTTP redirect Location count')
                            destination=i1_validate_dependency_url(locations[0],kind)
                            i1_require(len(destination.path.encode('utf-8'))<=240,'HTTP redirect path receipt bound')
                            url=locations[0];redirects+=1;record['complete']=True
                            continue
                        i1_require(response.status==200,'HTTP non-200')
                        enc=response.headers.get_all('Content-Encoding',[])
                        i1_require(not enc or enc==['identity'],'HTTP content encoding')
                        lengths=response.headers.get_all('Content-Length',[])
                        transfers=response.headers.get_all('Transfer-Encoding',[])
                        i1_require(len(lengths)<=1 and not(lengths and transfers),'HTTP ambiguous framing')
                        i1_require(not transfers or transfers==['chunked'],'HTTP transfer encoding')
                        declared=None
                        if lengths:
                            i1_require(len(lengths[0])<=10 and bool(i1_re.fullmatch('[0-9]+',lengths[0])),'HTTP Content-Length syntax')
                            declared=int(lengths[0]);i1_require(declared<=limit,'HTTP declared size limit')
                        while True:
                            remaining=deadline-i1_time.monotonic_ns()
                            i1_require(remaining>0,'child advisory deadline')
                            if response.isclosed():break
                            response.fp.raw._sock.settimeout(remaining/1000000000)
                            chunk=response.read1(min(65536,limit-count+1,8396800-total+1))
                            if not chunk:break
                            # Count observed bytes honestly even for a rejected
                            # over-limit byte; never write that byte into staging.
                            record['receivedBytes']+=len(chunk);total+=len(chunk)
                            i1_require(count+len(chunk)<=limit and total<=8396800,'HTTP actual byte limit')
                            # FileIO can short-write. Count/hash only bytes actually
                            # written, so failure resource fields match its .part.
                            offset=0
                            while offset<len(chunk):
                                wrote=output.write(chunk[offset:])
                                i1_require(type(wrote) is int and wrote>0,'staging short write')
                                digest.update(chunk[offset:offset+wrote]);count+=wrote;offset+=wrote
                                partial['bytes']=count;partial['sha256']=digest.hexdigest()
                            i1_require(i1_time.monotonic_ns()<deadline,'child advisory deadline')
                        i1_require(declared is None or count==declared,'HTTP truncated response')
                        record['complete']=True;partial['complete']=True
                        break
                    except Exception as exc:
                        # Never serialize exception text: it may expose signed URLs.
                        record['error']=type(exc).__name__[:120]
                        raise
                    finally:
                        record['endedNs']=i1_time.monotonic_ns()
                        if connection is not None:connection.close()
            completed.append(partial);partial=None
        i1_require(i1_time.monotonic_ns()<deadline,'child advisory deadline')
        return i1_child_response_bytes(dict(common,status='success',resources=completed))
    except Exception as exc:
        # No partial deletion/truncation/rename. Parent reads only after real reap.
        return i1_child_response_bytes(dict(common,status='failed',reason=type(exc).__name__[:120],completedResources=completed,partialResource=partial))

# I1 D1: fixed child acquisition. Not connected to the capture pipeline until
# binding4/source/provenance admission is installed and platform controls pass.
I1_FETCH_NS=120000000000
I1_FETCH_FLAG='--i1-fetch-child'
I1_CONTROL_ROLES=('acquisition-fork-positive','acquisition-fork-denied','acquisition-write-boundary')
I1_HTTP_KINDS=('info','mod','zip')


def _i1_json(data,limit=16384,*,manifest=False):
    """Bounded stdlib-only bootstrap: no local helper executes to parse input."""
    require(type(data) is bytes and len(data)<=limit,'i1-json-size')
    def pairs(rows):
        result={}
        for key,value in rows:
            require(key not in result,'i1-json-duplicate');result[key]=value
        return result
    def constant(value):raise GuardError('i1-json-number')
    try:value=json.loads(data,object_pairs_hook=pairs,parse_constant=constant)
    except (ValueError,UnicodeError,RecursionError):raise GuardError('i1-json-invalid')
    nodes=0;strings=0
    def visit(v,depth):
        nonlocal nodes,strings
        nodes+=1;require(depth<=8 and nodes<=(4096 if manifest else 512),'i1-json-bound')
        if type(v) is str:
            size=len(v.encode());strings+=size;require(size<=(4096 if manifest else 240) and strings<=(65536 if manifest else 8192),'i1-json-string')
        elif type(v) is dict:
            require(len(v)<=32,'i1-json-members')
            for k,x in v.items():visit(k,depth+1);visit(x,depth+1)
        elif type(v) is list:
            require(len(v)<=64,'i1-json-array')
            for x in v:visit(x,depth+1)
        else:require(type(v) in (int,bool,type(None)),'i1-json-scalar')
    visit(value,0);return value


def _i1_bootstrap_read(path,limit):
    import stat
    path=Path(path).absolute()
    require('..' not in path.parts,'i1-bootstrap-path')
    directory=os.open(path.anchor,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
    try:
        for part in path.parts[1:-1]:
            next_directory=os.open(part,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=directory)
            os.close(directory);directory=next_directory
        fd=os.open(path.name,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK,dir_fd=directory)
    finally:os.close(directory)
    try:
        before=os.fstat(fd);require(stat.S_ISREG(before.st_mode) and before.st_size<=limit,'i1-bootstrap-size')
        with os.fdopen(fd,'rb',closefd=False) as handle:data=handle.read(limit+1)
        after=os.fstat(fd)
        require(len(data)<=limit and (before.st_dev,before.st_ino,before.st_size,before.st_mtime_ns,before.st_ctime_ns)==(after.st_dev,after.st_ino,after.st_size,after.st_mtime_ns,after.st_ctime_ns),'i1-bootstrap-changed')
        return data
    finally:os.close(fd)


def _i1_bootstrap_tools(expected):
    import re
    require(type(expected) is str and re.fullmatch('[0-9a-f]{64}',expected),'i1-bootstrap-hash')
    raw=_i1_bootstrap_read(HERE/'tool-manifest.json',131072)
    require(sha(raw)==expected,'i1-bootstrap-manifest')
    value=_i1_json(raw,131072,manifest=True)
    require(type(value) is dict and set(value)=={'schemaVersion','files'} and type(value['schemaVersion']) is int and value['schemaVersion']==2,'i1-bootstrap-schema')
    rows=value['files'];require(type(rows) is list and len(rows)==45,'i1-bootstrap-count');seen=set();sources={}
    for row in rows:
        require(type(row) is dict and set(row)=={'path','role','gitBlob','sha256','bytes'} and row['role']=='tool' and row['gitBlob']=='','i1-bootstrap-row')
        name=row['path'];require(type(name) is str and not Path(name).is_absolute() and '..' not in Path(name).parts and name not in seen,'i1-bootstrap-path')
        require(type(row['bytes']) is int and 0<=row['bytes']<=4194304,'i1-bootstrap-length')
        data=_i1_bootstrap_read(REPO/name,4194304)
        require(len(data)==row['bytes'] and sha(data)==row['sha256'],'i1-bootstrap-tool');seen.add(name)
        target=REPO/name
        if target.parent==HERE and target.suffix=='.py':sources[target.stem]=(str(target),data)
    expected_paths={str(p.relative_to(REPO)) for p in HERE.rglob('*') if p.is_file() and p.name!='tool-manifest.json' and '__pycache__' not in p.parts}
    expected_paths.add('.github/workflows/swift-snapshot-oracle.yml')
    require(seen==expected_paths,'i1-bootstrap-extra')
    return sources


def _i1_install_source_loader(sources):
    # Consume the authenticated immutable bytes, never timestamp-valid .pyc.
    import importlib.abc,importlib.util
    class Loader(importlib.abc.MetaPathFinder,importlib.abc.Loader):
        def find_spec(self,fullname,path=None,target=None):
            if fullname in sources:return importlib.util.spec_from_loader(fullname,self,origin=sources[fullname][0])
        def create_module(self,spec):return None
        def exec_module(self,module):
            filename,data=sources[module.__name__]
            module.__file__=filename
            exec(compile(data,filename,'exec',dont_inherit=True),module.__dict__)
    require(not any(name in sys.modules for name in sources if name!='runner'),'i1-bootstrap-preimport')
    loader=Loader();sys.meta_path.insert(0,loader);return loader


def _i1_binding(work):
    import re,uuid
    raw=_i1_bootstrap_read(work/'run/run-binding.json',4096)
    binding=_i1_json(raw,4096)
    keys={'schemaVersion','runID','toolManifestSHA256','sourceManifestSHA256','catalogSHA256','integrationPolicySHA256'}
    require(type(binding) is dict and set(binding)==keys and binding['schemaVersion']==4,'i1-binding4-required')
    require(str(uuid.UUID(binding['runID']))==binding['runID'],'i1-run-id')
    require(all(type(binding[k]) is str and re.fullmatch('[0-9a-f]{64}',binding[k]) for k in keys-{'schemaVersion','runID'}),'i1-binding-hash')
    _i1_bootstrap_tools(binding['toolManifestSHA256'])
    import controller
    require(binding['integrationPolicySHA256']==controller.i1_integration_policy_digest(),'i1-policy-mismatch')
    require(binding['toolManifestSHA256']==validate_tool_manifest(HERE/'tool-manifest.json') and binding['sourceManifestSHA256']==SOURCE_SHA,'i1-tool-source-mismatch')
    import gate_receipts
    require(binding['catalogSHA256']==gate_receipts.catalog_sha(),'i1-catalog-mismatch')
    return binding,sha(raw)


def _i1_validate_request(request,work):
    binding,digest=_i1_binding(work)
    keys={'schemaVersion','bindingSHA256','integrationPolicySHA256','toolManifestSHA256','runID','parentStartedNs','deadlineNs'}
    require(type(request) is dict and set(request)==keys and type(request['schemaVersion']) is int and request['schemaVersion']==1,'i1-request-schema')
    require(request['bindingSHA256']==digest and all(request[k]==binding[k] for k in ('integrationPolicySHA256','toolManifestSHA256','runID')),'i1-request-binding')
    require(type(request['parentStartedNs']) is int and type(request['deadlineNs']) is int and 0<request['parentStartedNs']<request['deadlineNs'] and request['deadlineNs']-request['parentStartedNs']==I1_FETCH_NS,'i1-request-deadline')
    return binding


def _i1_supervise(argv,work,env,request):
    """Owned-process transport. A deadline prohibits success, not late cleanup.
    Unknown termination is retained as pending; it never authorizes raw snapshots.
    """
    raw=_json_bytes(request);require(len(raw)<=4096,'i1-request-limit')
    start=request['parentStartedNs'];deadline=request['deadlineNs']
    require(type(start) is int and type(deadline) is int and 0<start<deadline,'i1-deadline')
    proc=None;pgid=None;out=bytearray();tail=bytearray();out_count=err_count=sent=0
    transport='';first_late=kill_at=reaped=None;issues=[];eof=set()
    selector=selectors.DefaultSelector()
    require(len(COMMAND_EVIDENCE)<256,'provenance-command-limit');index=len(COMMAND_EVIDENCE)
    def issue(stage,error):
        issues.append({'stage':stage,'errorType':type(error).__name__})
    def consume(tag,block):
        nonlocal out_count,err_count
        if tag=='out':
            out_count+=len(block);out.extend(block[:max(0,16384-len(out))])
        else:
            err_count+=len(block);tail.extend(block)
            if len(tail)>65536:del tail[:-65536]
    try:
        require(time.monotonic_ns()<deadline,'timeout')
        proc=subprocess.Popen(argv,cwd=work,env=env,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,start_new_session=True)
        pgid=os.getpgid(proc.pid);require(pgid==proc.pid,'i1-owned-group')
        for stream,event,tag in ((proc.stdin,selectors.EVENT_WRITE,'in'),(proc.stdout,selectors.EVENT_READ,'out'),(proc.stderr,selectors.EVENT_READ,'err')):
            os.set_blocking(stream.fileno(),False);selector.register(stream,event,tag)
        while selector.get_map():
            now=time.monotonic_ns()
            if now>=deadline:first_late=now;raise GuardError('timeout')
            for key,_ in selector.select(min(.05,(deadline-now)/1e9)):
                if key.data=='in':
                    sent+=os.write(key.fileobj.fileno(),raw[sent:])
                    if sent==len(raw):selector.unregister(key.fileobj);key.fileobj.close()
                    continue
                block=os.read(key.fileobj.fileno(),8192)
                if not block:eof.add(key.data);selector.unregister(key.fileobj);continue
                consume(key.data,block)
                require(out_count<=16384,'stdout-resource-limit')
                require(err_count<=1048576,'stderr-resource-limit')
        remaining=(deadline-time.monotonic_ns())/1e9;require(remaining>0,'timeout')
        try:proc.wait(timeout=remaining)
        except subprocess.TimeoutExpired:raise GuardError('timeout')
        reaped=time.monotonic_ns()
        if reaped>deadline:first_late=reaped;raise GuardError('timeout')
        require(proc.returncode==0,'subprocess-rejected')
    except BaseException as error:
        transport=str(error) if isinstance(error,GuardError) else 'subprocess-error'
        if transport not in ('timeout','stdout-resource-limit','stderr-resource-limit','subprocess-rejected'):transport='subprocess-error'
        issue('transport',error)
        if transport=='timeout' and first_late is None:first_late=time.monotonic_ns()
        if proc is not None and reaped is None:
            if proc.returncode is None:
                kill_at=time.monotonic_ns()
                try:
                    if pgid==proc.pid:os.killpg(pgid,signal.SIGKILL)
                    else:proc.kill()
                except ProcessLookupError:pass
                except BaseException as failure:issue('terminate',failure)
            try:
                # Bounded retry observation only. A denial/stuck OS remains
                # pending, with the owned object retained for later recovery.
                proc.wait(timeout=1);reaped=time.monotonic_ns()
            except BaseException as failure:issue('reap',failure)
    finally:
        # Drain queued bytes only after proven termination. Nonblocking and
        # bounded by observed-byte caps, never another child execution window.
        if proc is not None and reaped is not None:
            for stream,tag,cap in ((proc.stdout,'out',131072),(proc.stderr,'err',1048576)):
                try:
                    if stream is None or stream.closed:continue
                    os.set_blocking(stream.fileno(),False)
                    while (out_count if tag=='out' else err_count)<cap:
                        remaining=cap-(out_count if tag=='out' else err_count)
                        try:block=os.read(stream.fileno(),min(8192,remaining))
                        except BlockingIOError:break
                        if not block:eof.add(tag);break
                        consume(tag,block)
                except BaseException as failure:issue('drain-'+tag,failure)
        try:selector.close()
        except BaseException as failure:issue('selector-close',failure)
        if proc is not None:
            for stream in (proc.stdin,proc.stdout,proc.stderr):
                try:
                    if stream is not None and not stream.closed:stream.close()
                except BaseException as failure:issue('stream-close',failure)
    ended=time.monotonic_ns();code=proc.returncode if proc is not None else None
    if issues and not transport:transport='cleanup-error'
    complete=proc is not None and reaped is not None and code==0 and not transport and reaped<=deadline
    if proc is not None and reaped is None:_I1_PENDING_CHILDREN[index]=proc
    signal_value=-code if type(code) is int and code<0 else None
    item={'schemaVersion':3,'kind':'ordinary-command','bindingSHA256':request['bindingSHA256'],'argv':argv[:],'pid':proc.pid if proc else None,'startedNs':start,'endedNs':ended,'returncode':code,'signal':signal_value,'transportError':transport,'stdoutBytes':out_count,'stdoutComplete':complete,'stdoutSHA256':sha(out) if complete else None,'stderrBytes':err_count,'stdout':bytes(out),'stderr':bytes(tail)}
    COMMAND_EVIDENCE.append(item)
    receipt={'schemaVersion':1,'bindingSHA256':request['bindingSHA256'],'integrationPolicySHA256':request['integrationPolicySHA256'],'commandIndex':index,'pid':proc.pid if proc else None,'pgid':pgid,'parentStartedNs':start,'deadlineNs':deadline,'firstDeadlineObservedNs':first_late,'killRequestedNs':kill_at,'reapedNs':reaped,'returncode':code,'signal':signal_value,'transportError':transport,'stdoutRef':None,'stderrRef':None,'resourceRefs':[],'outcome':'transport-success' if complete else ('pending-termination' if proc is not None and reaped is None else 'failed'),'cleanupIssues':issues,'stdoutObservedBytes':out_count,'stderrObservedBytes':err_count,'stdoutEOFObserved':'out' in eof,'stderrEOFObserved':'err' in eof,'stdoutRetainedBytes':len(out),'stderrRetainedBytes':len(tail)}
    return bytes(out),bytes(tail),receipt

_I1_PENDING_CHILDREN={}


def i1_acquisition_policy(work,python_path,python_prefix,ca_file,dns_servers):
    """Draft macOS profile: compatibility MUST pass real controls; no widening.
    No permission claim is made merely from constructing this string.
    """
    import ipaddress
    work=Path(work).resolve();python_path=Path(python_path).resolve();python_prefix=Path(python_prefix).resolve();ca_file=Path(ca_file).resolve()
    require(python_path.is_relative_to(python_prefix),'i1-python-prefix')
    require(ca_file.is_file() and not any(x.lower() in str(ca_file).lower() for x in ('keychain','icloud')),'i1-public-ca-required')
    require(type(dns_servers) is tuple and 1<=len(dns_servers)<=3,'i1-dns-set')
    dns=tuple(str(ipaddress.ip_address(x)) for x in dns_servers)
    # /Users and user Library are never broad read roots. Public CA and actual
    # Python/HERE are individually validated; no Keychain TLS fallback.
    reads=['/System/Library','/usr/lib','/usr/share','/private/etc/resolv.conf','/private/etc/hosts','/private/etc/services',str(python_prefix),str(HERE),str(work/'run/run-binding.json'),str(work/'input/dependency-download'),str(REPO/'.github/workflows/swift-snapshot-oracle.yml'),str(ca_file),'/dev/null','/dev/urandom','/dev/random']
    require(all(safe_path(p) for p in reads),'i1-policy-path')
    ancestors=sorted({str(p) for path in reads+[str(work/'input/dependency-download')] for p in Path(path).parents})
    lines=['(version 1)','(deny default)','(deny process-fork)',
           '(allow process-exec (literal "'+safe_path(python_path)+'"))',
           '(allow sysctl-read)',
           '(allow file-read-metadata '+' '.join('(literal "'+safe_path(p)+'")' for p in ancestors)+')',
           '(allow file-read* '+' '.join('(subpath "'+safe_path(p)+'")' for p in reads)+')',
           '(deny file-read* (subpath "/System/Library/Keychains"))',
           '(allow file-write* (literal "/dev/null") '+ ' '.join('(literal "'+safe_path(work/'input/dependency-download'/('v0.43.0.'+k+'.part'))+'")' for k in I1_HTTP_KINDS)+')',
           '(allow network-outbound (remote tcp "*:443"))']
    lines.extend('(allow network-outbound (remote udp "'+ip+':53"))' for ip in dns)
    # No guessed Mach service exceptions. If the host resolver requires a
    # service absent here, acquisition fails and the exact permission is reviewed.
    return '\n'.join(lines)+'\n'


_I1_FORK_PROBE="import os,json,errno;readfd,writefd=os.pipe();p=None\ntry:\n p=os.fork()\nexcept OSError as e:\n os.close(readfd);os.close(writefd);print(json.dumps({'schemaVersion':1,'attempted':True,'denied':e.errno in (errno.EPERM,errno.EACCES),'errno':e.errno}))\nelse:\n if p==0:\n  os.close(readfd);os.write(writefd,b'P');os.close(writefd);os._exit(0)\n os.close(writefd);observed=os.read(readfd,2);os.close(readfd);child,status=os.waitpid(p,0);print(json.dumps({'schemaVersion':1,'attempted':True,'denied':False,'childPID':child,'status':status,'childByte':observed.hex()}))"
_I1_WRITE_PROBE="import os,json,errno,pathlib;root=pathlib.Path.cwd();positive=root/'input/dependency-download/v0.43.0.info.part';fd=os.open(positive,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600);os.write(fd,b'P');os.close(fd);rows=[]\nfor p in (root/'cache/i1-write-sentinel',root/'i1-outside-sentinel'):\n try:\n  fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600);os.close(fd);rows.append({'path':str(p.relative_to(root)),'denied':False,'errno':0})\n except OSError as e:rows.append({'path':str(p.relative_to(root)),'denied':e.errno in (errno.EPERM,errno.EACCES),'errno':e.errno})\nprint(json.dumps({'schemaVersion':1,'positiveBytes':1,'denials':rows}))"
_I1_PERMITS={}
class _I1Permit:
    __slots__=('__weakref__',)


def i1_run_acquisition_controls(work,ca_file,dns_servers):
    require(I1_DEP_PUBLIC_CA_FILE is not None and str(Path(ca_file).resolve())==I1_DEP_PUBLIC_CA_FILE,'i1-reviewed-ca-not-configured')
    require(platform.system()=='Darwin' and os.environ.get('GITHUB_ACTIONS')=='true' and os.environ.get('RUNNER_ENVIRONMENT')=='github-hosted','i1-hosted-macos-required')
    work=Path(work).resolve();binding,digest=_i1_binding(work)
    require(not work.is_relative_to(REPO),'external-command-work-required')
    staging=work/'input/dependency-download';staging.mkdir(parents=True,exist_ok=False)
    (work/'cache').mkdir(exist_ok=True)
    python=str(Path(sys.executable).resolve());prefix=str(Path(sys.base_prefix).resolve())
    policy=i1_acquisition_policy(work,python,prefix,ca_file,dns_servers);policy_path=work/'acquisition.sb'
    with policy_path.open('x') as f:f.write(policy)
    env=environment(work);env['SSL_CERT_FILE']=str(Path(ca_file).resolve())
    argv_base=[python,'-I','-S','-B'];sandbox=['/usr/bin/sandbox-exec','-f',str(policy_path)]
    first=len(COMMAND_EVIDENCE)
    positive=_i1_json(bounded_run(argv_base+['-c',_I1_FORK_PROBE],work,env,10))
    require(set(positive)=={'schemaVersion','attempted','denied','childPID','status','childByte'} and type(positive['schemaVersion']) is int and positive['schemaVersion']==1 and positive['attempted'] is True and positive['denied'] is False and type(positive['childPID']) is int and positive['childPID']>0 and positive['status']==0 and positive['childByte']=='50','i1-fork-positive')
    negative=_i1_json(bounded_run(sandbox+argv_base+['-c',_I1_FORK_PROBE],work,env,10))
    require(set(negative)=={'schemaVersion','attempted','denied','errno'} and type(negative['schemaVersion']) is int and negative['schemaVersion']==1 and negative['attempted'] is True and negative['denied'] is True and negative['errno'] in (1,13),'i1-fork-denied')
    writes=_i1_json(bounded_run(sandbox+argv_base+['-c',_I1_WRITE_PROBE],work,env,10))
    require(set(writes)=={'schemaVersion','positiveBytes','denials'} and type(writes['schemaVersion']) is int and writes['schemaVersion']==1 and type(writes['positiveBytes']) is int and writes['positiveBytes']==1 and writes['denials']==[{'path':p,'denied':True,'errno':x['errno']} for p,x in zip(('cache/i1-write-sentinel','i1-outside-sentinel'),writes['denials'])] and len(writes['denials'])==2 and all(x['errno'] in (1,13) for x in writes['denials']),'i1-write-boundary')
    require(len(COMMAND_EVIDENCE)==first+3,'i1-control-count')
    from bounded import read_file,no_symlink
    probe=no_symlink(staging/'v0.43.0.info.part');require(read_file(probe,1)==b'P','i1-write-positive')
    evidence=work/'provenance';evidence.mkdir(exist_ok=True)
    target=evidence/'dependency-write-probe.bin';require(not target.exists(),'i1-probe-path');probe.rename(target)
    require(not list(staging.iterdir()),'i1-stage-not-empty')
    permit=_I1Permit();key=id(permit)
    state=(work,digest,binding['integrationPolicySHA256'],binding['toolManifestSHA256'],binding['runID'],policy_path,sha(policy.encode()),tuple(sandbox+argv_base+[str(HERE/'runner.py'),I1_FETCH_FLAG]),env.copy(),tuple(range(first,first+3)))
    _I1_PERMITS[key]=(weakref.ref(permit,lambda ref,k=key:_I1_PERMITS.pop(k,None)),state)
    return permit



def i1_download_snapshot(work,binding_digest):
    """Post-reap diagnostic bytes only. Never a successful admission witness."""
    directory=work/'provenance';directory.mkdir(exist_ok=True);resources=[]
    for kind in I1_HTTP_KINDS:
        name='input/dependency-download/v0.43.0.'+kind+'.part'
        # Oversized untrusted parts remain in place and are excluded, never
        # truncated into an apparent original. Accepted-source ZIP has56parts.
        maximum=I1_DEP_IDENTITIES[kind][0]
        data,reason,observed=_artifact_read(work,name,maximum)
        row={'kind':kind,'state':'captured' if reason is None else 'excluded','reason':reason,'observedBytes':observed,'sha256':sha(data) if data is not None else None,'parts':[]}
        if data is not None:
            for index,offset in enumerate(range(0,len(data),131072)):
                chunk=data[offset:offset+131072];filename='dependency-download-'+kind+'-'+str(index)+'.bin'
                with (directory/filename).open('xb') as handle:handle.write(chunk)
                row['parts'].append({'name':'provenance/'+filename,'bytes':len(chunk),'sha256':sha(chunk)})
        resources.append(row)
    raw=_json_bytes({'schemaVersion':1,'bindingSHA256':binding_digest,'resources':resources})
    require(len(raw)<=131072,'i1-download-index-limit')
    with (directory/'dependency-downloads.json').open('xb') as handle:handle.write(raw)
    return {'name':'provenance/dependency-downloads.json','bytes':len(raw),'sha256':sha(raw)}


def i1_acquisition_artifact_paths():
    paths=[('provenance/dependency-completion.json',131072),('acquisition.sb',65536),('provenance/dependency-write-probe.bin',1),('provenance/dependency-controller-transport.json',131072),('provenance/dependency-fetch-stdout.bin',16384),('provenance/dependency-fetch-stderr.bin',65536),('provenance/dependency-downloads.json',131072)]
    paths.extend(('provenance/dependency-http-'+str(i)+'.json',4096) for i in range(5))
    for kind,count in (('info',1),('mod',1),('zip',56)):
        paths.extend(('provenance/dependency-download-'+kind+'-'+str(i)+'.bin',131072) for i in range(count))
    return tuple(paths)

def i1_fetch_supervised(permit):
    entry=_I1_PERMITS.pop(id(permit),None)
    require(type(permit) is _I1Permit and entry is not None and entry[0]() is permit,'i1-real-platform-permit-required')
    work,digest,policy_sha,tools_sha,run_id,policy_path,profile_sha,argv,env,controls=entry[1]
    from bounded import read_file
    binding,current=_i1_binding(work)
    require(current==digest and sha(read_file(policy_path,65536))==profile_sha,'i1-permit-changed')
    require(not list((work/'input/dependency-download').iterdir()),'i1-stage-not-empty')
    start=time.monotonic_ns();request={'schemaVersion':1,'bindingSHA256':digest,'integrationPolicySHA256':policy_sha,'toolManifestSHA256':tools_sha,'runID':run_id,'parentStartedNs':start,'deadlineNs':start+I1_FETCH_NS}
    out,err,receipt=_i1_supervise(list(argv),work,env,request)
    directory=work/'provenance';directory.mkdir(exist_ok=True)
    def save(name,data):
        require(type(data) is bytes and len(data)<=131072,'i1-receipt-limit')
        with (directory/name).open('xb') as f:f.write(data)
        return {'name':'provenance/'+name,'bytes':len(data),'sha256':sha(data)}
    receipt['stdoutRef']=save('dependency-fetch-stdout.bin',out);receipt['stderrRef']=save('dependency-fetch-stderr.bin',err)
    transport_ref=save('dependency-controller-transport.json',_json_bytes(receipt))
    completion={'schemaVersion':1,'bindingSHA256':digest,'integrationPolicySHA256':policy_sha,'transportRef':transport_ref,'stdoutRef':receipt['stdoutRef'],'stderrRef':receipt['stderrRef'],'profileRef':{'name':'acquisition.sb','bytes':policy_path.stat().st_size,'sha256':profile_sha},'controlCommandIndices':list(controls),'httpRefs':[],'resourceRefs':[],'outcome':'failed','errorType':None}
    response=None
    try:
        require(receipt['reapedNs'] is not None,'i1-reap-unconfirmed')
        completion['resourceRefs']=[i1_download_snapshot(work,digest)]
        require(receipt['outcome']=='transport-success','i1-transport-failed')
        response=_i1_json(out);i1_validate_child_response(response,request)
        for resource in response['resources']:
            for http in resource['httpReceipts']:
                completion['httpRefs'].append(save('dependency-http-'+str(len(completion['httpRefs']))+'.json',_json_bytes(http)))
        require(len(completion['httpRefs'])<=5,'i1-http-count')
        completion['outcome']='transport-success'
    except Exception as error:
        completion['errorType']=type(error).__name__
        if receipt['reapedNs'] is None:completion['outcome']='pending-termination'
    completion_ref=save('dependency-completion.json',_json_bytes(completion))
    # Transport closure only, not an authenticated cache promotion permit.
    return {'outcome':completion['outcome'],'controller':receipt,'response':response,'completion':completion,'completionRef':completion_ref}


def _i1_child_entry():
    # -I/-S removed ambient user imports. Only this fixed, authenticated tool
    # directory is reintroduced; no cwd/PYTHONPATH lookup is admitted.
    require(platform.system()=='Darwin','i1-macos-child-required')
    raw=sys.stdin.buffer.read(4097);require(len(raw)<=4096,'i1-request-limit')
    request=_i1_json(raw,4096)
    require(type(request) is dict and set(request)=={'schemaVersion','bindingSHA256','integrationPolicySHA256','toolManifestSHA256','runID','parentStartedNs','deadlineNs'},'i1-request-schema')
    sources=_i1_bootstrap_tools(request['toolManifestSHA256'])
    _i1_install_source_loader(sources)
    sys.path.insert(0,str(HERE))
    work=Path.cwd().resolve();_i1_validate_request(request,work)
    data=i1_child_fetch(request,work);require(type(data) is bytes and len(data)<=16384,'i1-response-limit')
    response=_i1_json(data)
    sys.stdout.buffer.write(data);sys.stdout.buffer.flush()
    return 0 if response['status']=='success' else 1


def i1_validate_child_response(value,request):
    import re
    keys={'schemaVersion','bindingSHA256','integrationPolicySHA256','status','resources'}
    require(type(value) is dict and set(value)==keys and type(value['schemaVersion']) is int and value['schemaVersion']==1 and value['status']=='success','i1-response-schema')
    require(all(value[k]==request[k] for k in ('bindingSHA256','integrationPolicySHA256')),'i1-response-binding')
    require(type(value['resources']) is list and len(value['resources'])==3,'i1-response-resources')
    ordinal=0;last_end=request['parentStartedNs']
    for kind,resource in zip(I1_HTTP_KINDS,value['resources']):
        require(type(resource) is dict and set(resource)=={'kind','relativePath','bytes','sha256','complete','httpReceipts'},'i1-resource-schema')
        expected_bytes,expected_hash=I1_DEP_IDENTITIES[kind]
        require(resource['kind']==kind and resource['relativePath']=='input/dependency-download/v0.43.0.'+kind+'.part' and resource['complete'] is True and type(resource['bytes']) is int and resource['bytes']==expected_bytes and resource['sha256']==expected_hash,'i1-resource-identity')
        rows=resource['httpReceipts'];require(type(rows) is list and 1<=len(rows)<=(3 if kind=='zip' else 1),'i1-http-count')
        for number,row in enumerate(rows):
            required={'ordinal','requestKind','requestHost','requestPath','responseHost','responsePath','statusCode','startedNs','endedNs','receivedBytes','complete','error'}
            require(type(row) is dict and set(row)==required and row['ordinal']==ordinal and type(row['ordinal']) is int and row['requestKind']==kind,'i1-http-schema')
            require(type(row['startedNs']) is int and type(row['endedNs']) is int and last_end<=row['startedNs']<=row['endedNs']<=request['deadlineNs'],'i1-http-deadline')
            require(row['requestHost']==row['responseHost'] and row['requestPath']==row['responsePath'] and row['complete'] is True and row['error'] is None,'i1-http-completion')
            i1_validate_dependency_url('https://'+row['requestHost']+row['requestPath'],kind,initial=number==0)
            require(type(row['statusCode']) is int and type(row['receivedBytes']) is int,'i1-http-number-type')
            final=number==len(rows)-1
            require((row['statusCode']==200 and row['receivedBytes']==expected_bytes) if final else (row['statusCode'] in (301,302,303,307,308) and row['receivedBytes']==0),'i1-http-result')
            last_end=row['endedNs'];ordinal+=1
    require(ordinal<=5,'i1-http-count')
    return value


# I1 source rebind v2: mechanical source contracts only. Capture remains blocked.
# The final runner identity/full role binding is intentionally not issued here.
def _i1_source_policy():
    import controller
    return controller.i1_integration_policy()


def _i1_exact(actual, expected):
    """Strict JSON equality: bool is never an integer and extra keys fail."""
    if type(actual) is not type(expected):return False
    if type(expected) is dict:return set(actual)==set(expected) and all(_i1_exact(actual[k],v) for k,v in expected.items())
    if type(expected) is list:return len(actual)==len(expected) and all(_i1_exact(a,b) for a,b in zip(actual,expected))
    return actual==expected


def i1_source_rows():
    policy=_i1_source_policy()['sourcePolicy'];fields=('path','mode','kind','oid','bytes','sha256','chunks')
    rows=sorted(policy['singlePathRows']+[{k:r[k] for k in fields} for r in policy['mapping']],key=lambda r:r['path'])
    require(len(rows)==139 and len({r['path'] for r in rows})==139 and len({r['oid'] for r in rows})==133,'i1-source-path-count')
    require(sum(r['bytes'] for r in rows)==9462103 and sum(r['chunks'] for r in rows)==196,'i1-source-byte-count')
    return rows


def _i1_object_id(kind,raw):
    require(type(raw) is bytes,'i1-object-bytes')
    return hashlib.sha1(kind.encode()+b' '+str(len(raw)).encode()+b'\0'+raw).hexdigest()


def _i1_tree_entries(raw):
    """Parse original Git tree bytes, including canonical Git name ordering."""
    require(type(raw) is bytes and len(raw)<=65536,'i1-tree-size')
    result={};pos=0;previous=None
    while pos<len(raw):
        space=raw.find(b' ',pos);nul=raw.find(b'\0',space+1)
        require(space>pos and nul>space+1 and nul+21<=len(raw),'i1-tree-truncated')
        mode=raw[pos:space];name=raw[space+1:nul]
        require(mode in (b'40000',b'100644',b'100755') and name not in (b'.',b'..') and b'/' not in name,'i1-tree-entry')
        try:text=name.decode('utf-8')
        except UnicodeError:raise GuardError('i1-tree-name')
        require(text not in result and len(name)<=240 and not any(c in text for c in ('\n','\r','\\')),'i1-tree-name')
        order=name+(b'/' if mode==b'40000' else b'')
        require(previous is None or previous<order,'i1-tree-order');previous=order
        result[text]=(mode.decode(),raw[nul+1:nul+21].hex());pos=nul+21
    return result


def i1_authenticate_source_originals(commits,trees):
    """Offline object proof for frozen spec reference and new Go source only.

    No git/process/network calls. This is not the missing final runner proof.
    """
    policy=_i1_source_policy();expected={policy['frozenSpecReference']:policy['frozenSpecTree'],policy['productSourceBase']:policy['productSourceTree']}
    require(type(commits) is dict and set(commits)==set(expected),'i1-commit-set')
    require(type(trees) is dict and 1<=len(trees)<=39,'i1-tree-count')
    used=set();parsed={}
    def tree(oid):
        require(oid in trees,'i1-tree-missing');raw=trees[oid]
        require(type(raw) is bytes and _i1_object_id('tree',raw)==oid,'i1-tree-hash')
        used.add(oid)
        if oid not in parsed:parsed[oid]=_i1_tree_entries(raw)
        return parsed[oid]
    def path(root,parts):
        item=('40000',root)
        for part in parts:
            require(item[0]=='40000','i1-path-not-tree');entries=tree(item[1]);require(part in entries,'i1-path-missing');item=entries[part]
        return item
    def descend(oid,prefix,rows):
        for name,(mode,child) in tree(oid).items():
            full=prefix+'/'+name
            if mode=='40000':descend(child,full,rows)
            else:rows.append({'path':full,'mode':mode,'kind':'blob','oid':child})
    actual={}
    for commit,root in expected.items():
        raw=commits[commit];require(type(raw) is bytes and len(raw)<=16384 and _i1_object_id('commit',raw)==commit,'i1-commit-hash')
        require(raw.startswith(b'tree '+root.encode()+b'\n') and raw.count(b'\ntree ')==0,'i1-commit-tree')
        tree(root);path(root,['ports','mygo']);rows=[]
        for name in sorted(policy['inputs']):
            mode,oid=path(root,name.split('/'))
            if name.endswith(('go.mod','go.sum')):rows.append({'path':name,'mode':mode,'kind':'blob','oid':oid})
            else:
                require(mode=='40000','i1-domain-mode');descend(oid,name,rows)
            if commit==policy['productSourceBase']:require(oid==policy['inputs'][name],'i1-input-identity')
        actual[commit]=sorted(rows,key=lambda r:r['path'])
    require(used==set(trees),'i1-tree-extra')
    current=i1_source_rows();fields=('path','mode','kind','oid')
    expected_new=[{k:r[k] for k in fields} for r in current]
    old_by_path={r['path']:r for r in current};batch=policy['sourcePolicy']['mapping']
    for row in batch:old_by_path.pop(row['path'])
    for row in policy['sourcePolicy']['frozenChangedRows']:old_by_path[row['path']]=row
    expected_old=[{k:r[k] for k in fields} for r in sorted(old_by_path.values(),key=lambda r:r['path'])]
    require(len(expected_old)==136 and _i1_exact(actual[policy['frozenSpecReference']],expected_old),'i1-frozen-source-tree')
    require(_i1_exact(actual[policy['productSourceBase']],expected_new),'i1-new-source-tree')
    return {'productPaths':139,'distinctBlobOids':133,'sourceCommit':policy['productSourceBase'],'sourceTree':policy['productSourceTree'],'commitCount':len(commits),'treeCount':len(trees),'runnerBound':False}


def i1_source_batch_stdin(commits,trees):
    i1_authenticate_source_originals(commits,trees)
    policy=_i1_source_policy()['sourcePolicy'];raw=b''.join(r['oid'].encode('ascii')+b'\n' for r in policy['mapping'])
    require(len(raw)==205 and sha(raw)==policy['stdin']['sha256'],'i1-batch-stdin')
    return raw


def _i1_parse_fixed_batch(stdout,mapping):
    """Internal binary parser; lengths/offsets originate only in bound policy.

    Nothing escapes until all headers, payload hashes, delimiters and EOF pass.
    """
    require(type(stdout) is bytes and len(stdout)<=131072,'i1-batch-stdout-limit')
    pos=0;values=[]
    for i,row in enumerate(mapping):
        size=row['bytes'];header=(row['oid']+' blob '+str(size)+'\n').encode('ascii')
        require(type(size) is int and 0<=size<=4194304,'i1-batch-size')
        require(row['index']==i and row['headerOffset']==pos and row['headerLength']==len(header) and row['canonicalHeader'].encode('ascii')==header,'i1-batch-header-mapping')
        require(stdout[pos:pos+len(header)]==header,'i1-batch-header');pos+=len(header)
        require(row['payloadOffset']==pos and row['payloadLength']==size,'i1-batch-payload-mapping')
        data=stdout[pos:pos+size];require(len(data)==size,'i1-batch-short-payload')
        require(_i1_object_id('blob',data)==row['oid'] and sha(data)==row['sha256'],'i1-batch-payload-hash');pos+=size
        require(row['delimiterOffset']==pos and row['delimiterHex']=='0a' and stdout[pos:pos+1]==b'\n','i1-batch-delimiter');pos+=1
        values.append(data)
    require(pos==len(stdout),'i1-batch-extra-output')
    return tuple(values)


def i1_parse_source_batch(stdin,stdout,commits,trees):
    require(type(stdin) is bytes and stdin==i1_source_batch_stdin(commits,trees),'i1-batch-stdin')
    policy=_i1_source_policy()['sourcePolicy']
    require(type(stdout) is bytes and len(stdout)==50352,'i1-batch-stdout-size')
    return _i1_parse_fixed_batch(stdout,policy['mapping'])


def i1_source_command_slots():
    """Full *maximum* role inventory, not executable argv permissions.

    Only exact prebound commit/tree overlap may shrink the final inventory.
    The final runner objects and existing concrete commands remain unresolved.
    """
    policy=_i1_source_policy()['sourcePolicy'];rows=[]
    for group in policy['roleGroups']:
        for ordinal in range(1,group['maximumCount']+1):
            row={'slot':len(rows)+1,'role':group['role'],'ordinal':ordinal}
            if group['role']=='frozen-go-input-object':row.update({k:policy['singlePathRows'][ordinal-1][k] for k in ('path','oid')})
            elif group['role']=='raw-tree':row['binding']='sorted unique domain union + frozen/base/runner ancestor OIDs, finalized before execution'
            rows.append(row)
    require(len(rows)==255 and rows[246]['role']=='i1-source-delta-five-batch-v1','i1-command-inventory')
    return rows


def i1_validate_source_slot_inventory(rows):
    require(type(rows) is list and len(rows)<=256,'provenance-command-limit')
    require(_i1_exact(rows,i1_source_command_slots()),'i1-command-slot-mismatch')
    return True


def i1_require_source_execution_bound():
    policy=_i1_source_policy()
    require(policy['runnerCommit'] is not None and policy['runnerTree'] is not None,'i1-final-runner-unbound')
    # No permit is issued until binding4/provenance4 + every existing concrete
    # command/phase has a reviewed closed mapping. A role label is not permission.
    raise GuardError('i1-full-command-bindings-unavailable')


def _i1_source_batch_context(work,object_db):
    """Internal binding expansion, never a public arbitrary-command runner."""
    work=Path(work);object_db=Path(object_db)
    require(work.is_absolute() and object_db.is_absolute() and os.path.normpath(str(work))==str(work) and os.path.normpath(str(object_db))==str(object_db) and '..' not in work.parts and '..' not in object_db.parts,'i1-source-path-binding')
    require(not work.is_relative_to(REPO) and work!=object_db and not work.is_relative_to(object_db) and not object_db.is_relative_to(work),'i1-source-path-overlap')
    policy=_i1_source_policy()['sourcePolicy'];argv=[s.replace('${SOURCE_OBJECT_DB}',str(object_db)) for s in policy['argv']]
    env=environment(work);env['GIT_NO_REPLACE_OBJECTS']='1'
    return {'role':policy['role'],'phase':policy['phase'],'argv':argv,'cwd':str(work),'env':env,'shell':False}


def i1_run_source_batch(permit=None):
    """No production entry while final runner and full execution plan are unset."""
    i1_require_source_execution_bound()
    raise GuardError('i1-source-permit-unavailable')


def _i1_source_ref(name,raw):
    require(type(raw) is bytes and len(raw)<=131072,'i1-source-raw-cap')
    return {'name':name,'bytes':len(raw),'sha256':sha(raw)}


def i1_source_raw_members(commits,trees,products,stdin,stdout,probes,*,manifest_bytes=None):
    """Build immutable, full path-accounted evidence; not a source snapshot.

    Includes all duplicate-OID paths independently, plus stdin AND framing.
    All callers must also charge dependency/receipt/index raw and tar copies.
    """
    import controller
    i1_authenticate_source_originals(commits,trees);batch=i1_parse_source_batch(stdin,stdout,commits,trees)
    rows=i1_source_rows();require(type(products) is dict and set(products)=={r['path'] for r in rows},'i1-product-path-set')
    require(type(probes) is dict,'i1-probe-set')
    manifest=json.loads(checked(HERE/'tool-manifest.json',validate_tool_manifest(HERE/'tool-manifest.json'),131072)) if manifest_bytes is None else _i1_capture_manifest(manifest_bytes)
    expected=[r for r in manifest['files'] if r['path'].startswith('ports/mygo/scripts/swift-snapshot-oracle/go-probe/') and r['path'].endswith('.go')]
    require(len(expected)==7 and set(probes)=={r['path'] for r in expected},'i1-probe-set')
    members={};inputs=[];chunks=0
    for index,row in enumerate(rows):
        raw=products[row['path']]
        require(type(raw) is bytes and len(raw)==row['bytes'] and sha(raw)==row['sha256'] and _i1_object_id('blob',raw)==row['oid'],'i1-product-identity')
        count=(len(raw)+131071)//131072;require(count==row['chunks'] and count<=32,'i1-product-chunks');chunks+=count;refs=[]
        for piece in range(count):
            name=f'provenance/integration-product-{index:03}-chunk-{piece:02}.bin';data=raw[piece*131072:(piece+1)*131072];members[name]=data;refs.append(_i1_source_ref(name,data))
        empty=None
        if not raw:
            name=f'provenance/integration-product-{index:03}-empty.bin';members[name]=b'';empty=_i1_source_ref(name,b'')
        inputs.append({**row,'raw':refs,'emptyOriginal':empty})
    require(chunks==196 and chunks<=208,'i1-product-chunks')
    for row,data in zip(_i1_source_policy()['sourcePolicy']['mapping'],batch):require(products[row['path']]==data,'i1-batch-child-original')
    probe_bytes=0
    for index,row in enumerate(sorted(expected,key=lambda r:r['path'])):
        raw=probes[row['path']];require(type(raw) is bytes and len(raw)==row['bytes'] and sha(raw)==row['sha256'],'i1-probe-identity')
        name=f'provenance/integration-probe-{index:02}.bin';members[name]=raw;probe_bytes+=len(raw)
    require(len(products)+len(probes)==146 and probe_bytes<=1048576 and sum(r['bytes'] for r in rows)+probe_bytes<=10485760,'i1-input-limit')
    for kind,objects in (('commit',commits),('tree',trees)):
        for index,(oid,raw) in enumerate(sorted(objects.items())):members[f'provenance/integration-{kind}-{index:02}-{oid}.bin']=raw
    members['provenance/integration-batch-stdin.bin']=stdin;members['provenance/integration-batch-stdout.bin']=stdout
    for index in range((len(inputs)+63)//64):members[f'provenance/integration-inputs-{index:02}.json']=_json_bytes({'schemaVersion':1,'integrationPolicySHA256':controller.i1_integration_policy_digest(),'rows':inputs[index*64:(index+1)*64]})
    i1_source_retention_budget(members)
    return members


def i1_source_retention_budget(members):
    require(type(members) is dict and len(members)<=8192,'i1-retention-count')
    total=0
    for name,raw in members.items():
        require(type(name) is str and type(raw) is bytes and len(raw)<=131072,'i1-retention-member')
        require(len(name.encode('utf-8'))<=240 and len(Path(name).parts)<=16 and not Path(name).is_absolute() and '..' not in Path(name).parts,'i1-retention-path')
        total+=len(raw)
    require(total<=20971520,'i1-new-retained-cap')
    return {'files':len(members),'rawBytes':total,'rawLimit':20971520,'remainingRawBytes':20971520-total,'tarAccounted':False}

"""Internal transport only. Admission/authentication belongs to its fixed-role caller."""
import os
import selectors
import signal
import subprocess
import time

I1_BATCH_TIMEOUT_NS = 120_000_000_000
_I1_BATCH_PENDING_CHILDREN = {}


def _i1_batch_transport(argv, work, env, raw):
    if type(raw) is not bytes or len(raw) != 205:
        raise ValueError('i1-batch-stdin-length')
    return _i1_fixed_source_transport(argv, work, env, raw, False)


def _i1_fixed_source_transport(argv, work, env, raw, single):
    """No public command entry point, retries, evidence append, or success hashes.

    The caller supplies only its authenticated fixed argv/env and derived stdin.
    Returned bytes are diagnostic unless complete is true AND caller validates
    the complete fixed framing. Cleanup gets at most one additional second.
    """
    if type(single) is not bool or type(raw) is not bytes or len(raw) != (0 if single else 205):
        raise ValueError('i1-source-stdin-length')
    stdout_cap = 4194304 if single else 131072
    start = time.monotonic_ns()
    deadline = start + I1_BATCH_TIMEOUT_NS
    proc = None
    pgid = None
    selector = None
    out, tail = bytearray(), bytearray()
    out_count = err_count = sent = 0
    stdin_closed = single
    eof = set()
    issues = []
    transport = ''
    first_late = kill_at = reaped = None

    class TransportFailure(Exception):
        pass

    def issue(stage, error):
        issues.append({'stage': stage, 'errorType': type(error).__name__})

    def check_deadline():
        nonlocal first_late
        now = time.monotonic_ns()
        if now >= deadline:
            first_late = now
            raise TransportFailure('timeout')
        return now

    def consume(tag, block):
        nonlocal out_count, err_count
        if tag == 'out':
            out_count += len(block)
            out.extend(block[:max(0, stdout_cap - len(out))])
            if out_count > stdout_cap:
                raise TransportFailure('stdout-resource-limit')
        else:
            err_count += len(block)
            tail.extend(block)
            if len(tail) > 65536:
                del tail[:-65536]
            if err_count > 1048576:
                raise TransportFailure('stderr-resource-limit')

    try:
        selector = selectors.DefaultSelector()
        check_deadline()
        proc = subprocess.Popen(argv, cwd=work, env=env, shell=False,
                                stdin=subprocess.DEVNULL if single else subprocess.PIPE, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, start_new_session=True,
                                bufsize=0)
        pgid = os.getpgid(proc.pid)
        if pgid != proc.pid:
            raise TransportFailure('i1-owned-group')
        streams=[(proc.stdout, selectors.EVENT_READ, 'out'),(proc.stderr, selectors.EVENT_READ, 'err')]
        if not single:streams.insert(0,(proc.stdin, selectors.EVENT_WRITE, 'in'))
        for stream, event, tag in streams:
            os.set_blocking(stream.fileno(), False)
            selector.register(stream, event, tag)
        while selector.get_map():
            now = check_deadline()
            for key, _ in selector.select(min(.05, (deadline - now) / 1e9)):
                check_deadline()
                if key.data == 'in':
                    chunk = raw[sent:]
                    try:
                        n = os.write(key.fileobj.fileno(), chunk)
                    except BlockingIOError:
                        continue
                    except BrokenPipeError as error:
                        raise TransportFailure('stdin-broken-pipe') from error
                    sent += n
                    if n != len(chunk):
                        raise TransportFailure('stdin-short-write')
                    selector.unregister(key.fileobj)
                    try:
                        key.fileobj.close()
                        if not key.fileobj.closed:
                            raise OSError('stdin-not-closed')
                    except Exception as error:
                        raise TransportFailure('stdin-close-error') from error
                    stdin_closed = True
                else:
                    try:
                        block = os.read(key.fileobj.fileno(), 8192)
                    except BlockingIOError:
                        continue
                    if not block:
                        eof.add(key.data)
                        selector.unregister(key.fileobj)
                    else:
                        consume(key.data, block)
        now = check_deadline()
        try:
            proc.wait(timeout=(deadline - now) / 1e9)
        except subprocess.TimeoutExpired as error:
            raise TransportFailure('timeout') from error
        reaped = time.monotonic_ns()
        check_deadline()
        if proc.returncode != 0:
            raise TransportFailure('subprocess-rejected')
    except BaseException as error:
        transport = str(error) if isinstance(error, TransportFailure) else 'subprocess-error'
        issue('transport', error)
        if transport == 'timeout' and first_late is None:
            first_late = time.monotonic_ns()
        if proc is not None and reaped is None:
            # Do not poll then signal: polling reaps and permits PID reuse.
            # A returncode already set by wait proves reaping; never signal it.
            if proc.returncode is None:
                kill_at = time.monotonic_ns()
                try:
                    if pgid == proc.pid:
                        os.killpg(pgid, signal.SIGKILL)
                    else:
                        proc.kill()
                except ProcessLookupError:
                    pass
                except BaseException as failure:
                    issue('terminate', failure)
            try:
                proc.wait(timeout=1)
                reaped = time.monotonic_ns()
            except BaseException as failure:
                issue('reap', failure)
    finally:
        if selector is not None:
            try:
                selector.close()
            except BaseException as failure:
                issue('selector-close', failure)
        if proc is not None:
            for tag, stream in (('stdin', proc.stdin), ('stdout', proc.stdout), ('stderr', proc.stderr)):
                try:
                    if stream is not None and not stream.closed:
                        stream.close()
                    if tag == 'stdin' and stream is not None:
                        stdin_closed = bool(stream.closed)
                except BaseException as failure:
                    issue(tag + '-close', failure)
    ended = time.monotonic_ns()
    code = proc.returncode if proc is not None else None
    if issues and not transport:
        transport = 'cleanup-error'
    complete = (proc is not None and reaped is not None and reaped < deadline
                and sent == len(raw) and stdin_closed and eof == {'out', 'err'}
                and code == 0 and not transport and not issues)
    if proc is not None and reaped is None:
        _I1_BATCH_PENDING_CHILDREN[proc.pid] = proc
    receipt = {
        'schemaVersion': 1, 'pid': proc.pid if proc is not None else None,
        'pgid': pgid, 'parentStartedNs': start, 'deadlineNs': deadline,
        'endedNs': ended, 'firstDeadlineObservedNs': first_late,
        'killRequestedNs': kill_at, 'reapedNs': reaped,
        'returncode': code, 'signal': -code if type(code) is int and code < 0 else None,
        'transportError': transport, 'stdinBytes': sent, 'stdinClosed': stdin_closed,
        'stdoutObservedBytes': out_count, 'stderrObservedBytes': err_count,
        'stdoutRetainedBytes': len(out), 'stderrRetainedBytes': len(tail),
        'stdoutEOFObserved': 'out' in eof, 'stderrEOFObserved': 'err' in eof,
        'complete': complete,
        'outcome': 'transport-success' if complete else (
            'pending-termination' if proc is not None and reaped is None else 'failed'),
        'cleanupIssues': issues,
    }
    return bytes(out), bytes(tail), receipt


def i1_validate_batch_transport(receipt,stdout,stderr):
    return _i1_validate_source_transport(receipt,stdout,stderr,50352,205)


def _i1_validate_source_transport(receipt,stdout,stderr,expected_size,stdin_size):
    """Closed success schema. No null/false/number aliases for transportError."""
    integers=('pid','pgid','parentStartedNs','deadlineNs','endedNs','reapedNs','returncode','stdinBytes','stdoutObservedBytes','stderrObservedBytes','stdoutRetainedBytes','stderrRetainedBytes')
    bools=('stdinClosed','stdoutEOFObserved','stderrEOFObserved','complete')
    keys=set(integers)|set(bools)|{'schemaVersion','firstDeadlineObservedNs','killRequestedNs','signal','transportError','outcome','cleanupIssues'}
    require(type(receipt) is dict and set(receipt)==keys,'i1-batch-transport-schema')
    require(type(receipt['schemaVersion']) is int and receipt['schemaVersion']==1,'i1-batch-transport-schema')
    require(all(type(receipt[k]) is int for k in integers) and all(receipt[k] is True for k in bools),'i1-batch-transport-schema')
    require(type(receipt['transportError']) is str and receipt['transportError']=='','i1-batch-transport-error')
    require(receipt['pid']>0 and receipt['pgid']==receipt['pid'] and receipt['returncode']==0 and receipt['outcome']=='transport-success','i1-batch-process')
    require(receipt['firstDeadlineObservedNs'] is None and receipt['killRequestedNs'] is None and receipt['signal'] is None and receipt['cleanupIssues']==[],'i1-batch-process')
    start=receipt['parentStartedNs'];deadline=receipt['deadlineNs'];reaped=receipt['reapedNs']
    require(0<start<reaped<deadline and deadline-start==120000000000 and receipt['endedNs']>=reaped,'i1-batch-deadline')
    require(type(stdout) is bytes and len(stdout)==expected_size and receipt['stdoutObservedBytes']==receipt['stdoutRetainedBytes']==len(stdout),'i1-batch-output-complete')
    require(type(stderr) is bytes and 0<=receipt['stderrObservedBytes']<=1048576 and len(stderr)==receipt['stderrRetainedBytes']==min(receipt['stderrObservedBytes'],65536),'i1-batch-stderr')
    require(receipt['stdinBytes']==stdin_size,'i1-batch-stdin-complete')
    return True


def _i1_batch_mechanical_record(context,transport,stdin,stdout,stderr,commits,trees):
    """Stage evidence is explicitly NOT binding4/provenance4 admission."""
    import controller
    require(type(context) is dict and set(context)=={'role','phase','argv','cwd','env','shell'},'i1-batch-context')
    require(type(context['argv']) is list and len(context['argv'])==9 and type(context['argv'][6]) is str and context['argv'][6].startswith('--git-dir='),'i1-batch-command')
    expected=_i1_source_batch_context(Path(context['cwd']),Path(context['argv'][6][10:]))
    require(_i1_exact(context,expected),'i1-batch-context')
    children=i1_parse_source_batch(stdin,stdout,commits,trees);i1_validate_batch_transport(transport,stdout,stderr)
    policy=_i1_source_policy();source=policy['sourcePolicy'];binding={'schemaVersion':1,'scope':'local-source-batch-mechanical-only','sourceBase':policy['productSourceBase'],'sourceTree':policy['productSourceTree'],'runnerCommit':None,'integrationPolicySHA256':controller.i1_integration_policy_digest(),'context':context}
    binding_sha=sha(_json_bytes(binding));receipt_id=sha(_json_bytes({'bindingSHA256':binding_sha,'role':source['role'],'ordinal':1}))
    members={'provenance/integration-batch-stdin.bin':stdin,'provenance/integration-batch-stdout.bin':stdout,'provenance/integration-batch-stderr.bin':stderr};rows=[]
    for i,(row,raw) in enumerate(zip(source['mapping'],children)):
        name=f'provenance/integration-batch-child-{i:02}.bin';members[name]=raw;rows.append({**row,'commandReceiptID':receipt_id,'raw':_i1_source_ref(name,raw)})
    record={'schemaVersion':1,'scope':'local-source-batch-mechanical-only','runtimeAdmission':False,'receiptID':receipt_id,'binding':binding,'bindingSHA256':binding_sha,'transport':transport,'stdin':_i1_source_ref('provenance/integration-batch-stdin.bin',stdin),'stdout':_i1_source_ref('provenance/integration-batch-stdout.bin',stdout),'stderr':_i1_source_ref('provenance/integration-batch-stderr.bin',stderr),'children':rows}
    name='provenance/integration-batch-receipt.json';members[name]=_json_bytes(record);i1_source_retention_budget(members)
    return members


def i1_verify_batch_mechanical_evidence(members,context,commits,trees):
    """Pure offline reconstruction. Never re-executes or proves historical OS exit."""
    names={'provenance/integration-batch-'+n for n in ('stdin.bin','stdout.bin','stderr.bin','receipt.json')}|{f'provenance/integration-batch-child-{i:02}.bin' for i in range(5)}
    require(type(members) is dict and set(members)==names,'i1-batch-evidence-set');i1_source_retention_budget(members)
    name='provenance/integration-batch-receipt.json'
    record=_i1_json(members[name],131072,manifest=True)
    require(type(record) is dict and set(record)=={'schemaVersion','scope','runtimeAdmission','receiptID','binding','bindingSHA256','transport','stdin','stdout','stderr','children'},'i1-batch-receipt-schema')
    rebuilt=_i1_batch_mechanical_record(context,record['transport'],members['provenance/integration-batch-stdin.bin'],members['provenance/integration-batch-stdout.bin'],members['provenance/integration-batch-stderr.bin'],commits,trees)
    require(_i1_exact(members,rebuilt),'i1-batch-evidence-mismatch')
    return {'mechanicalEvidenceVerified':True,'runtimeAdmission':False,'offlineOSReexecution':False,'receiptID':record['receiptID']}

# SPDX-License-Identifier: AGPL-3.0-only
"""Finite source-stage filesystem operations, not a runtime sandbox.

All traversal starts at an opened / and opens each component no-follow. Repeated
measurements detect observed changes; they do not exclude replace-and-restore
races by a malicious same-user process between measurements.
"""
import hashlib
import os
from pathlib import Path
import re
import stat
import time
import zlib


DB_CONFIG = b'[core]\n\trepositoryformatversion = 0\n\tbare = true\n\tfilemode = true\n\tlogallrefupdates = false\n'
_DIR_FLAGS = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC
_FILE_FLAGS = os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC
SCOPES = ('ports/mygo/domain-codec', 'ports/mygo/domain-core',
          'ports/mygo/domain-extensions', 'ports/mygo/go.mod', 'ports/mygo/go.sum')


def _i1_capture_fs_sha(raw):
    return hashlib.sha256(raw).hexdigest()


def _i1_capture_fs_relative(name):
    require(type(name) is str and name and len(name.encode('utf-8')) <= 240,
            'capture-path')
    parts = name.split('/')
    require(len(parts) <= 16 and all(p not in ('', '.', '..') for p in parts)
            and not any(c in name for c in ('\\', '\0', '\n', '\r')), 'capture-path')
    return parts


def _i1_capture_fs_absolute(path):
    value = os.fspath(path)
    require(type(value) is str and value.startswith('/') and value != '/'
            and os.path.normpath(value) == value and not value.startswith('//'), 'capture-absolute-path')
    require(all(p not in ('', '.', '..') for p in value[1:].split('/')), 'capture-absolute-path')
    return value


def _i1_capture_fs_identity(st):
    kind = 'directory' if stat.S_ISDIR(st.st_mode) else 'file' if stat.S_ISREG(st.st_mode) else 'special'
    return {'type': kind, 'mode': stat.S_IMODE(st.st_mode), 'dev': st.st_dev,
            'inode': st.st_ino, 'nlink': st.st_nlink, 'size': st.st_size,
            'allocatedBytes': st.st_blocks * 512}


def _i1_capture_fs_open_root(path):
    """Caller owns returned safe root fd; chain evidence excludes volatile size."""
    value = _i1_capture_fs_absolute(path)
    fd = os.open('/', _DIR_FLAGS)
    chain = []
    try:
        for part in value[1:].split('/'):
            child = os.open(part, _DIR_FLAGS, dir_fd=fd)
            os.close(fd)
            fd = child
            st = os.fstat(fd)
            chain.append({'component': part, 'dev': st.st_dev, 'inode': st.st_ino,
                          'type': 'directory', 'mode': stat.S_IMODE(st.st_mode)})
        return fd, chain
    except BaseException:
        os.close(fd)
        raise


def _i1_capture_fs_root_identity(path):
    fd, chain = _i1_capture_fs_open_root(path)
    try:
        st = os.fstat(fd)
        return {'path': _i1_capture_fs_absolute(path), 'dev': st.st_dev, 'inode': st.st_ino,
                'type': 'directory', 'mode': stat.S_IMODE(st.st_mode), 'chain': chain}
    finally:
        os.close(fd)


def _i1_capture_fs_validate_roots(checkout, object_db, work):
    roots = [_i1_capture_fs_root_identity(p) for p in (checkout, object_db, work)]
    for i, left in enumerate(roots):
        for right in roots[i + 1:]:
            a, b = Path(left['path']), Path(right['path'])
            require(not a.is_relative_to(b) and not b.is_relative_to(a)
                    and (left['dev'], left['inode']) != (right['dev'], right['inode']),
                    'capture-root-overlap')
    return dict(zip(('checkout', 'objectDB', 'work'), roots))


def _i1_capture_fs_open_relative(root, name, directory=False):
    parts = _i1_capture_fs_relative(name)
    fd = os.dup(root)
    try:
        for part in parts[:-1]:
            child = os.open(part, _DIR_FLAGS, dir_fd=fd)
            os.close(fd)
            fd = child
        return os.open(parts[-1], _DIR_FLAGS if directory else _FILE_FLAGS, dir_fd=fd)
    finally:
        os.close(fd)


def _i1_capture_fs_read(root, name, limit):
    fd = _i1_capture_fs_open_relative(root, name)
    try:
        before = os.fstat(fd)
        require(stat.S_ISREG(before.st_mode) and before.st_nlink == 1, 'capture-regular-single-link')
        require(0 <= before.st_size <= limit, 'capture-file-limit')
        raw = bytearray()
        while True:
            block = os.read(fd, min(65536, limit + 1 - len(raw)))
            if not block:
                break
            raw.extend(block)
            require(len(raw) <= limit, 'capture-file-limit')
        after = os.fstat(fd)
        require(_i1_capture_fs_identity(before) == _i1_capture_fs_identity(after) and len(raw) == after.st_size,
                'capture-read-drift')
        return bytes(raw), {**_i1_capture_fs_identity(after), 'bytes': len(raw), 'sha256': _i1_capture_fs_sha(raw)}
    finally:
        os.close(fd)


def _i1_capture_fs_directories(files):
    result = {''}
    for name in files:
        parts = _i1_capture_fs_relative(name)
        result.update('/'.join(parts[:i]) for i in range(1, len(parts)))
    return result


def _i1_capture_fs_scan(root, files, directories, readonly=False):
    """Scan only the fixed expected directories; never recurse into extras."""
    result = {}
    for name in sorted(directories):
        fd = os.dup(root) if not name else _i1_capture_fs_open_relative(root, name, True)
        try:
            st = os.fstat(fd)
            if readonly:
                require(stat.S_IMODE(st.st_mode) == 0o555, 'capture-directory-mode')
            prefix = name + '/' if name else ''
            expected = {n[len(prefix):].split('/')[0] for n in files | directories
                        if n.startswith(prefix) and n != name}
            actual = set()
            with os.scandir(fd) as iterator:
                for entry in iterator:
                    require(entry.name in expected and entry.name not in actual, 'capture-directory-set')
                    actual.add(entry.name)
            require(actual == expected, 'capture-directory-set')
            result[name] = _i1_capture_fs_identity(st)
        finally:
            os.close(fd)
    return result


def _i1_capture_fs_authenticate_db(path, objects, product_base):
    """Accept exactly two commits, 36 trees and 133 blobs as loose originals."""
    require(type(objects) is dict and len(objects) == 171, 'capture-db-object-count')
    require({kind: sum(v[0] == kind for v in objects.values()) for kind in ('commit', 'tree', 'blob')}
            == {'commit': 2, 'tree': 36, 'blob': 133}, 'capture-db-object-kinds')
    require(product_base in objects and objects[product_base][0] == 'commit', 'capture-db-base')
    files = {'HEAD', 'config'}
    limits = {}
    for oid, (kind, raw) in objects.items():
        require(re.fullmatch('[0-9a-f]{40}', oid) is not None and type(raw) is bytes,
                'capture-db-object')
        require(len(raw) <= {'commit': 16384, 'tree': 65536, 'blob': 4194304}[kind], 'capture-db-raw-limit')
        full = kind.encode() + b' ' + str(len(raw)).encode() + b'\0' + raw
        require(hashlib.sha1(full).hexdigest() == oid, 'capture-db-object-id')
        name = 'objects/' + oid[:2] + '/' + oid[2:]
        files.add(name)
        limits[name] = full
    directories = _i1_capture_fs_directories(files) | {'refs'}
    require(len(directories) <= 174 and len(files) == 173, 'capture-db-layout-limit')
    fd, chain = _i1_capture_fs_open_root(path)
    try:
        dirs = _i1_capture_fs_scan(fd, files, directories)
        records = {}
        for name in sorted(files):
            expected = DB_CONFIG if name == 'config' else (product_base + '\n').encode() if name == 'HEAD' else limits[name]
            raw, record = _i1_capture_fs_read(fd, name, 4096 if name in ('HEAD', 'config') else len(expected) + 1024)
            if name in ('HEAD', 'config'):
                require(raw == expected, 'capture-db-fixed-file')
            else:
                decoder = zlib.decompressobj()
                decoded = decoder.decompress(raw, len(expected) + 1)
                require(decoded == expected and decoder.eof and not decoder.unconsumed_tail
                        and not decoder.unused_data, 'capture-db-loose-stream')
                record['objectSHA1'] = hashlib.sha1(decoded).hexdigest()
            records[name] = record
        total = sum(row['bytes'] for row in records.values())
        require(total <= 10485760, 'capture-db-byte-limit')
        return {'path': _i1_capture_fs_absolute(path), 'chain': chain, 'directories': dirs, 'files': records,
                'logicalBytes': total, 'allocatedBytes': sum(r['allocatedBytes'] for r in list(dirs.values()) + list(records.values()))}
    finally:
        os.close(fd)


def _i1_capture_fs_inputs(inputs, exact=False):
    require(type(inputs) is dict and len(inputs) <= 146, 'capture-input-count')
    for name, value in inputs.items():
        _i1_capture_fs_relative(name)
        require(type(value) is dict and type(value.get('bytes')) is bytes
                and value.get('mode') in ('100644', '100755') and len(value['bytes']) <= 4194304,
                'capture-input')
    total = sum(len(v['bytes']) for v in inputs.values())
    require(total <= 10485760, 'capture-input-bytes')
    if exact:
        require(len(inputs) == 146 and total == 9525260, 'capture-current-input-identity')
    directories = _i1_capture_fs_directories(set(inputs))
    require(len(directories) <= 64, 'capture-directory-limit')
    return directories


def _i1_capture_fs_verify_checkout(root, inputs):
    """Read exactly the five product scopes, including ignored/untracked files."""
    _i1_capture_fs_inputs(inputs)
    require(len(inputs) == 139 and sum(len(v['bytes']) for v in inputs.values()) == 9462103,
            'capture-checkout-input-count')
    require(all(any(n == s or n.startswith(s + '/') for s in SCOPES) for n in inputs), 'capture-checkout-scope')
    fd, chain = _i1_capture_fs_open_root(root)
    records = {}
    directories = {}
    try:
        for scope in SCOPES[:3]:
            subset = {n[len(scope) + 1:] for n in inputs if n.startswith(scope + '/')}
            child = _i1_capture_fs_open_relative(fd, scope, True)
            try:
                measured = _i1_capture_fs_scan(child, subset, _i1_capture_fs_directories(subset))
                directories.update({scope + ('/' + n if n else ''): r for n, r in measured.items()})
            finally:
                os.close(child)
        for name, value in sorted(inputs.items()):
            raw, record = _i1_capture_fs_read(fd, name, len(value['bytes']))
            require(raw == value['bytes'] and record['mode'] == int(value['mode'][-3:], 8), 'capture-checkout-content-mode')
            records[name] = record
        return {'path': _i1_capture_fs_absolute(root), 'chain': chain, 'directories': directories, 'files': records}
    finally:
        os.close(fd)


def _i1_capture_fs_export_snapshot(destination, inputs, usage=None):
    """Exclusive write. Failure leaves partial evidence in place, never retries."""
    directories = _i1_capture_fs_inputs(inputs, exact=True)
    destination = Path(_i1_capture_fs_absolute(destination))
    parent, _ = _i1_capture_fs_open_root(destination.parent)
    root = None
    try:
        os.mkdir(destination.name, 0o700, dir_fd=parent)
        root = os.open(destination.name, _DIR_FLAGS, dir_fd=parent)
        for name in sorted(directories - {''}, key=lambda n: (n.count('/'), n)):
            parts = _i1_capture_fs_relative(name)
            fd = os.dup(root) if len(parts) == 1 else _i1_capture_fs_open_relative(root, '/'.join(parts[:-1]), True)
            try:
                os.mkdir(parts[-1], 0o700, dir_fd=fd)
            finally:
                os.close(fd)
        written = 0
        completed=set()
        for name, value in sorted(inputs.items()):
            parts = _i1_capture_fs_relative(name)
            fd = os.dup(root) if len(parts) == 1 else _i1_capture_fs_open_relative(root, '/'.join(parts[:-1]), True)
            try:
                out = os.open(parts[-1], os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600, dir_fd=fd)
                try:
                    raw = value['bytes']
                    require(written + len(raw) <= 9525260, 'capture-write-budget')
                    offset = 0
                    while offset < len(raw):
                        size = os.write(out, raw[offset:offset + 65536])
                        require(size > 0, 'capture-short-write')
                        offset += size
                    written += offset
                    os.fchmod(out, 0o444 if value['mode'] == '100644' else 0o555)
                    st = os.fstat(out)
                    require(st.st_size == len(raw) and st.st_nlink == 1, 'capture-write-drift')
                finally:
                    os.close(out)
            finally:
                os.close(fd)
            completed.add(name)
            if usage is not None:usage.append({'kind':'export-partial',**_i1_capture_fs_usage(destination,completed,directories,10485760)})
        for name in sorted(directories, key=lambda n: (-n.count('/'), n)):
            fd = os.dup(root) if not name else _i1_capture_fs_open_relative(root, name, True)
            try:
                os.fchmod(fd, 0o555)
            finally:
                os.close(fd)
    finally:
        if root is not None:
            os.close(root)
        os.close(parent)
    return _i1_capture_fs_measure_snapshot(destination, inputs)


def _i1_capture_fs_measure_snapshot(destination, inputs):
    directories = _i1_capture_fs_inputs(inputs, exact=True)
    start = time.monotonic_ns()
    fd, chain = _i1_capture_fs_open_root(destination)
    try:
        dirs = _i1_capture_fs_scan(fd, set(inputs), directories, readonly=True)
        records = {}
        for name, value in sorted(inputs.items()):
            raw, record = _i1_capture_fs_read(fd, name, len(value['bytes']))
            require(raw == value['bytes'] and record['mode'] == (0o444 if value['mode'] == '100644' else 0o555),
                    'capture-export-content-mode')
            records[name] = record
        return {'path': _i1_capture_fs_absolute(destination), 'startedNs': start, 'endedNs': time.monotonic_ns(),
                'chain': chain, 'directories': dirs, 'files': records,
                'logicalBytes': sum(r['bytes'] for r in records.values()),
                'allocatedBytes': sum(r['allocatedBytes'] for r in list(dirs.values()) + list(records.values()))}
    finally:
        os.close(fd)


def _i1_capture_fs_compare_measurements(first, second):
    require(first['endedNs'] < second['startedNs'], 'capture-measurement-order')
    require({k: v for k, v in first.items() if k not in ('startedNs', 'endedNs')}
            == {k: v for k, v in second.items() if k not in ('startedNs', 'endedNs')},
            'capture-export-identity-drift')
    return True


def _i1_capture_fs_reject_shared_files(measurement, *sources):
    """Export uses new inodes, even for empty or shared-OID product paths."""
    originals = {(row['dev'], row['inode']) for source in sources
                 for row in source['files'].values()}
    actual = [(row['dev'], row['inode']) for row in measurement['files'].values()]
    require(len(actual) == len(set(actual)) and not (set(actual) & originals),
            'capture-export-shared-inode')
    return True


def _i1_capture_fs_make_parent(path):
    """Create one exclusive parent directory beneath an already safe root."""
    path = Path(_i1_capture_fs_absolute(path))
    fd, _ = _i1_capture_fs_open_root(path.parent)
    try:
        os.mkdir(path.name, 0o700, dir_fd=fd)
        child = os.open(path.name, _DIR_FLAGS, dir_fd=fd)
        try:
            require(stat.S_IMODE(os.fstat(child).st_mode) == 0o700, 'capture-parent-mode')
        finally:
            os.close(child)
    finally:
        os.close(fd)
    return _i1_capture_fs_root_identity(path)


# Local mechanical source stage. This deliberately issues no production permit.
def _i1_capture_manifest(raw):
    import re
    value=_i1_json(raw,131072,manifest=True)
    require(type(value) is dict and set(value)=={'schemaVersion','files'} and type(value['schemaVersion']) is int and value['schemaVersion']==2,'i1-capture-manifest')
    rows=value['files'];require(type(rows) is list and len(rows)==45,'i1-capture-tool-count');seen=set()
    for row in rows:
        require(type(row) is dict and set(row)=={'path','role','gitBlob','sha256','bytes'},'i1-capture-tool-row')
        name=row['path'];i1_safe_path(name)
        require(name not in seen and row['role']=='tool' and row['gitBlob']=='' and type(row['bytes']) is int and 0<=row['bytes']<=4194304 and type(row['sha256']) is str and re.fullmatch('[0-9a-f]{64}',row['sha256']),'i1-capture-tool-row');seen.add(name)
    return value


def _i1_capture_inputs(products,probes,manifest):
    rows=i1_source_rows();expected={r['path']:r for r in rows}
    require(type(products) is dict and set(products)==set(expected),'i1-capture-product-set')
    inputs={};identities=[]
    for row in rows:
        raw=products[row['path']]
        require(type(raw) is bytes and len(raw)==row['bytes'] and sha(raw)==row['sha256'] and _i1_object_id('blob',raw)==row['oid'],'i1-capture-product')
        inputs[row['path']]={'bytes':raw,'mode':row['mode']}
        identities.append({**row,'exportMode':0o444 if row['mode']=='100644' else 0o555})
    prows=sorted((r for r in manifest['files'] if r['path'].startswith('ports/mygo/scripts/swift-snapshot-oracle/go-probe/') and r['path'].endswith('.go')),key=lambda r:r['path'])
    require(len(prows)==7 and set(probes)=={r['path'] for r in prows} and sum(r['bytes'] for r in prows)==63157,'i1-capture-probes')
    for row in prows:
        raw=probes[row['path']]
        require(type(raw) is bytes and len(raw)==row['bytes'] and sha(raw)==row['sha256'],'i1-capture-probe')
        inputs[row['path']]={'bytes':raw,'mode':'100644'}
        identities.append({**row,'mode':'100644','exportMode':0o444})
    require(len(inputs)==146 and sum(len(x['bytes']) for x in inputs.values())==9525260,'i1-capture-inputs')
    directories={'.'}
    for name in inputs:
        for parent in Path(name).parents:
            if str(parent)!='.':directories.add(str(parent))
    require(len(directories)<=64,'i1-capture-directory-limit')
    return inputs,{'rows':sorted(identities,key=lambda x:x['path']),'directories':sorted(directories),'payloadBytes':9525260}


def _i1_capture_single_context(stage,ordinal):
    rows=_i1_source_policy()['sourcePolicy']['singlePathRows']
    require(type(ordinal) is int and 1<=ordinal<=134,'i1-capture-ordinal')
    row=rows[ordinal-1];work=Path(stage['work']['path'])
    env=environment(work);env['GIT_NO_REPLACE_OBJECTS']='1'
    return {'role':'frozen-go-input-object','phase':'authenticated-source-acquisition-before-snapshot','ordinal':ordinal,'slot':112+ordinal,
            'argv':['/usr/bin/git','--no-replace-objects','-c','core.hooksPath=/dev/null','-c','protocol.file.allow=never','--git-dir='+stage['objectDB']['path'],'cat-file','blob',stage['productSourceBase']+':'+row['path']],
            'cwd':str(work),'env':env,'shell':False,'source':row,'sourceCommit':stage['productSourceBase'],'sourceTree':stage['productSourceTree']}


def _i1_capture_merge(target,source):
    for name,raw in source.items():
        require(name not in target or target[name]==raw,'i1-capture-member-collision');target[name]=raw


def _i1_capture_original_objects(commits,trees,objects):
    i1_authenticate_source_originals(commits,trees)
    require(len(commits)==2 and len(trees)==36 and type(objects) is dict and len(objects)==171,'i1-capture-object-count')
    expected={oid:('commit',raw) for oid,raw in commits.items()};expected.update({oid:('tree',raw) for oid,raw in trees.items()})
    for row in i1_source_rows():
        require(row['oid'] in objects,'i1-capture-object-missing')
        kind,raw=objects[row['oid']]
        require(kind=='blob' and type(raw) is bytes and len(raw)==row['bytes'] and sha(raw)==row['sha256'] and _i1_object_id(kind,raw)==row['oid'],'i1-capture-object-identity')
        expected[row['oid']]=(kind,raw)
    require(objects==expected,'i1-capture-object-set')


def _i1_capture_source_stage(work,checkout,object_db,commits,trees,objects):
    """Internal test slice; source originals are DB expectations, NEVER results.

    No retries, production binding, build, or claim of resistance to malicious
    same-user replacement-and-restoration between pathname measurements.
    """
    import uuid,controller
    work=Path(work);checkout=Path(checkout);object_db=Path(object_db)
    roots=[_i1_capture_fs_root_identity(p) for p in (work,checkout,object_db)]
    for a in (work,checkout,object_db):
        for b in (work,checkout,object_db):
            if a!=b:require(not a.is_relative_to(b),'i1-capture-root-overlap')
    require(len({(r['dev'],r['inode']) for r in roots})==3,'i1-capture-root-alias')
    _i1_capture_original_objects(commits,trees,objects)
    manifest_sha=validate_tool_manifest(HERE/'tool-manifest.json')
    tool_root,_=_i1_capture_fs_open_root(REPO)
    try:
        manifest_raw,_=_i1_capture_fs_read(tool_root,str((HERE/'tool-manifest.json').relative_to(REPO)),131072)
        require(sha(manifest_raw)==manifest_sha,'i1-capture-manifest-drift');manifest=_i1_capture_manifest(manifest_raw)
        probes={r['path']:_i1_capture_fs_read(tool_root,r['path'],131072)[0] for r in manifest['files'] if r['path'].startswith('ports/mygo/scripts/swift-snapshot-oracle/go-probe/') and r['path'].endswith('.go')}
    finally:os.close(tool_root)
    policy=_i1_source_policy()
    stage={'schemaVersion':1,'scope':'local-source-capture-mechanical-only','runtimeAdmission':False,'runnerCommit':None,'runID':str(uuid.uuid4()),'productSourceBase':policy['productSourceBase'],'productSourceTree':policy['productSourceTree'],'integrationPolicySHA256':controller.i1_integration_policy_digest(),'toolManifestSHA256':manifest_sha,'work':roots[0],'checkout':roots[1],'objectDB':roots[2]}
    stage_sha=sha(_json_bytes(stage));db=_i1_capture_fs_authenticate_db(object_db,objects,policy['productSourceBase']);db_sha=sha(json.dumps(db,sort_keys=True,separators=(',',':')).encode())
    members={'provenance/capture-stage.json':_json_bytes(stage),'provenance/capture-tool-manifest.json':manifest_raw};products={};receipts=[]
    _i1_capture_store_measurement(members,'database',db,stage_sha)
    usage=[{'kind':'work-empty',**_i1_capture_fs_usage(work,set(),{''},10485760)}]
    exported=False
    def work_usage():
        files={'input/go-source/'+p for p in inputs} if exported else set()
        dirs=_i1_capture_fs_directories(files)
        usage.append({'kind':'work-export' if exported else 'work-empty',**_i1_capture_fs_usage(work,files,dirs,10485760)})
    def check():
        work_usage()
        for p,expected in zip((work,checkout,object_db),roots):require(_i1_capture_fs_root_identity(p)==expected,'i1-capture-root-drift')
        measured=_i1_capture_fs_authenticate_db(object_db,objects,policy['productSourceBase'])
        require(measured==db,'i1-capture-db-drift')
        return {'measuredNs':time.monotonic_ns(),'databaseSHA256':db_sha,'stageSHA256':stage_sha}
    held_db,held_chain=_i1_capture_fs_open_root(object_db)
    try:
        require(held_chain==db['chain'],'i1-capture-held-db')
        for ordinal,row in enumerate(policy['sourcePolicy']['singlePathRows'],1):
            context=_i1_capture_single_context(stage,ordinal);before=check()
            stdout,stderr,transport=_i1_fixed_source_transport(context['argv'],work,context['env'],b'',True)
            after=check();_i1_validate_source_transport(transport,stdout,stderr,row['bytes'],0)
            require(sha(stdout)==row['sha256'] and _i1_object_id('blob',stdout)==row['oid'],'i1-capture-single-identity')
            products[row['path']]=stdout
            name=f'provenance/capture-single-{ordinal:03}-stderr.bin';members[name]=stderr
            record={'schemaVersion':1,'scope':stage['scope'],'runtimeAdmission':False,'stageSHA256':stage_sha,'context':context,'transport':transport,'before':before,'after':after,'stderr':_i1_source_ref(name,stderr)}
            index=next(i for i,v in enumerate(i1_source_rows()) if v['path']==row['path'])
            refs=[];empty=None
            for piece in range(row['chunks']):
                n=f'provenance/integration-product-{index:03}-chunk-{piece:02}.bin';data=stdout[piece*131072:(piece+1)*131072];members[n]=data;refs.append(_i1_source_ref(n,data))
            if not stdout:
                n=f'provenance/integration-product-{index:03}-empty.bin';members[n]=b'';empty=_i1_source_ref(n,b'')
            record['stdout']=refs;record['emptyOriginal']=empty
            members[f'provenance/capture-single-{ordinal:03}.json']=_json_bytes(record)
            receipts.append(record)
            i1_source_retention_budget(members)
        context=_i1_source_batch_context(work,object_db);stdin=i1_source_batch_stdin(commits,trees);before=check()
        stdout,stderr,transport=_i1_batch_transport(context['argv'],work,context['env'],stdin);after=check()
        batch_members=_i1_batch_mechanical_record(context,transport,stdin,stdout,stderr,commits,trees)
        for row,raw in zip(policy['sourcePolicy']['mapping'],i1_parse_source_batch(stdin,stdout,commits,trees)):products[row['path']]=raw
        _i1_capture_merge(members,batch_members)
        members['provenance/capture-batch-link.json']=_json_bytes({'stageSHA256':stage_sha,'before':before,'after':after,'receipt':_i1_source_ref('provenance/integration-batch-receipt.json',batch_members['provenance/integration-batch-receipt.json'])})
        raw_members=i1_source_raw_members(commits,trees,products,stdin,stdout,probes,manifest_bytes=manifest_raw);_i1_capture_merge(members,raw_members)
        input_rows=[]
        for i in range(3):input_rows.extend(json.loads(raw_members[f'provenance/integration-inputs-{i:02}.json'])['rows'])
        bypath={r['path']:r for r in input_rows}
        for ordinal,record in enumerate(receipts,1):
            row=bypath[record['context']['source']['path']];record['stdout']=row['raw'];record['emptyOriginal']=row['emptyOriginal']
            members[f'provenance/capture-single-{ordinal:03}.json']=_json_bytes(record)
        inputs,expected=_i1_capture_inputs(products,probes,manifest)
        checkout_measurement=_i1_capture_fs_verify_checkout(checkout,{p:v for p,v in inputs.items() if p in products})
        checkout_measurement['logicalBytes']=sum(r['bytes'] for r in checkout_measurement['files'].values())
        checkout_measurement['allocatedBytes']=sum(r['allocatedBytes'] for r in list(checkout_measurement['files'].values())+list(checkout_measurement['directories'].values()))
        _i1_capture_store_measurement(members,'checkout',checkout_measurement,stage_sha)
        expected.update(stageSHA256=stage_sha);members['provenance/capture-sourceExpected.json']=_json_bytes(expected)
        # Bounded measurements are split by fixed 64-row parts, not arbitrary metadata.
        _i1_capture_store_measurement(members,'database',db,stage_sha)
        destination=work/'input'/'go-source'
        _i1_capture_fs_make_parent(work/'input')
        postwrite=_i1_capture_fs_export_snapshot(destination,inputs,usage)
        exported=True
        _i1_capture_fs_reject_shared_files(postwrite,checkout_measurement,db)
        _i1_capture_store_measurement(members,'postWrite',postwrite,stage_sha)
        check()
        postexport=_i1_capture_fs_measure_snapshot(destination,inputs)
        _i1_capture_fs_compare_measurements(postwrite,postexport)
        _i1_capture_store_measurement(members,'postExport',postexport,stage_sha)
        work_usage()
        _i1_capture_store_usage(members,usage,stage_sha)
        i1_source_retention_budget(members)
        _i1_verify_source_capture(members,manifest_sha)
        retained=sum(map(len,members.values()))+9525260+db['logicalBytes']
        plan,budget=_package_plan(members,retained,[],True,False)
        bundle=work/'source-capture.tar';_write_tar(bundle,plan)
        final_usage=_i1_capture_fs_usage(work,{'input/go-source/'+p for p in inputs}|{'source-capture.tar'},_i1_capture_fs_directories({'input/go-source/'+p for p in inputs}),79*1048576)
        return {'scope':stage['scope'],'runtimeAdmission':False,'snapshotReady':True,'processes':135,'members':members,'bundle':str(bundle),'budget':budget,'stage':stage,'postArchiveUsage':final_usage}
    except BaseException as error:
        # Preserve bounded partial originals and the actual failed transport. Do
        # not delete a partial export or a pending child from its registry.
        _i1_capture_store_usage(members,usage,stage_sha)
        error.capture_members=members
        if 'transport' in locals():error.capture_transport=transport
        if 'stdout' in locals():error.capture_stdout=stdout[:131072]
        if 'stderr' in locals():error.capture_stderr=stderr[-65536:]
        raise
    finally:
        os.close(held_db)


def _i1_capture_store_measurement(members,label,value,stage_sha):
    # Store deterministic bounded pieces; counts and names are derived by verifier.
    header={k:v for k,v in value.items() if k not in ('files','directories')}
    parts=[]
    for group in ('directories','files'):
        rows=sorted(value[group].items())
        for i in range((len(rows)+63)//64):
            name=f'provenance/capture-{label}-{group}-{i:02}.json'
            raw=_json_bytes({'stageSHA256':stage_sha,'rows':rows[i*64:(i+1)*64]});members[name]=raw;parts.append(_i1_source_ref(name,raw))
    members[f'provenance/capture-{label}.json']=_json_bytes({'stageSHA256':stage_sha,'header':header,'parts':parts})


def _i1_capture_load_measurement(members,label,stage_sha,used):
    name=f'provenance/capture-{label}.json';used.add(name);index=_i1_json(members[name],131072,manifest=True)
    require(type(index) is dict and set(index)=={'stageSHA256','header','parts'} and index['stageSHA256']==stage_sha,'i1-capture-measurement')
    require(type(index['header']) is dict and not {'files','directories'}&set(index['header']),'i1-capture-measurement')
    result={**index['header'],'files':{},'directories':{}}
    refs=index['parts'];require(type(refs) is list and len(refs)<=7,'i1-capture-measurement-parts')
    for ref in refs:
        target=ref['name'];require(target not in used and ref==_i1_source_ref(target,members[target]),'i1-capture-measurement-ref');used.add(target)
        group='directories' if '-directories-' in target else 'files'
        part=_i1_json(members[target],131072,manifest=True)
        require(type(part) is dict and set(part)=={'stageSHA256','rows'} and part['stageSHA256']==stage_sha and type(part['rows']) is list and 1<=len(part['rows'])<=64,'i1-capture-measurement-part')
        for row in part['rows']:
            require(type(row) is list and len(row)==2 and type(row[0]) is str and row[0] not in result[group],'i1-capture-measurement-row');result[group][row[0]]=row[1]
    rebuilt={};_i1_capture_store_measurement(rebuilt,label,result,stage_sha)
    require(all(members[n]==raw for n,raw in rebuilt.items()) and set(rebuilt)=={name}|{r['name'] for r in refs},'i1-capture-measurement-canonical')
    return result


def _i1_capture_validate_root(value):
    require(type(value) is dict and set(value)=={'path','dev','inode','type','mode','chain'},'i1-capture-root-schema')
    _i1_capture_fs_absolute(value['path'])
    require(value['type']=='directory' and all(type(value[k]) is int and value[k]>=0 for k in ('dev','inode','mode')),'i1-capture-root-schema')
    chain=value['chain'];parts=Path(value['path']).parts[1:]
    require(type(chain) is list and len(chain)==len(parts),'i1-capture-root-chain')
    for name,row in zip(parts,chain):
        require(type(row) is dict and set(row)=={'component','dev','inode','type','mode'} and row['component']==name and row['type']=='directory' and all(type(row[k]) is int and row[k]>=0 for k in ('dev','inode','mode')),'i1-capture-root-chain')
    require(all(chain[-1][k]==value[k] for k in ('dev','inode','type','mode')),'i1-capture-root-chain')


def _i1_capture_validate_measurement(value,inputs,root,db=False,checkout=False):
    numeric=('mode','dev','inode','nlink','size','allocatedBytes')
    expected_dirs=_i1_capture_fs_directories(set(inputs))|({'refs'} if db else set())
    if checkout:expected_dirs={n for n in expected_dirs if any(n==scope or n.startswith(scope+'/') for scope in SCOPES[:3])}
    require(set(value['files'])==set(inputs) and set(value['directories'])==expected_dirs,'i1-capture-measured-set')
    require(value['path']==root['path'] and value['chain']==root['chain'],'i1-capture-measured-root')
    for name,row in value['directories'].items():
        require(type(row) is dict and set(row)=={'type',*numeric} and row['type']=='directory' and all(type(row[k]) is int and row[k]>=0 for k in numeric),'i1-capture-directory-record')
        if not db and not checkout:require(row['mode']==0o555,'i1-capture-directory-readonly')
    if not checkout:require(all(value['directories'][''][k]==root[k] for k in ('dev','inode','type','mode')),'i1-capture-measured-root')
    for name,row in value['files'].items():
        expected=inputs[name]
        keys={'type',*numeric,'bytes','sha256'}|({'objectSHA1'} if db and name.startswith('objects/') else set())
        require(type(row) is dict and set(row)==keys and row['type']=='file' and all(type(row[k]) is int and row[k]>=0 for k in (*numeric,'bytes')) and row['nlink']==1 and row['bytes']==row['size'],'i1-capture-file-record')
        if db:
            require(0<=row['bytes']<=expected['limit'],'i1-capture-db-record-limit')
            if 'raw' in expected:require(row['bytes']==len(expected['raw']) and row['sha256']==sha(expected['raw']),'i1-capture-db-fixed-record')
            else:require(row['objectSHA1']==expected['oid'],'i1-capture-db-object-record')
        else:require(row['bytes']==len(expected['bytes']) and row['sha256']==sha(expected['bytes']) and row['mode']==(int(expected['mode'][-3:],8) if checkout else 0o444 if expected['mode']=='100644' else 0o555),'i1-capture-export-record')
        require(type(row['sha256']) is str and re.fullmatch('[0-9a-f]{64}',row['sha256']),'i1-capture-file-hash')
    require(len({(r['dev'],r['inode']) for r in value['files'].values()})==len(value['files']),'i1-capture-file-inode-unique')
    require(len({(r['dev'],r['inode']) for r in value['directories'].values()})==len(value['directories']),'i1-capture-directory-inode-unique')
    require(value['logicalBytes']==sum(r['bytes'] for r in value['files'].values())<=10485760 and value['allocatedBytes']==sum(r['allocatedBytes'] for r in list(value['files'].values())+list(value['directories'].values())),'i1-capture-measured-bytes')


def _i1_verify_source_capture(members,trusted_manifest_sha):
    """Explicit bytes only. Checks records, not historical OS or disk behavior."""
    import uuid,controller
    i1_source_retention_budget(members)
    stage_raw=members['provenance/capture-stage.json'];stage=_i1_json(stage_raw,131072,manifest=True);stage_sha=sha(stage_raw)
    expected_keys={'schemaVersion','scope','runtimeAdmission','runnerCommit','runID','productSourceBase','productSourceTree','integrationPolicySHA256','toolManifestSHA256','work','checkout','objectDB'}
    require(type(stage) is dict and set(stage)==expected_keys and type(stage['schemaVersion']) is int and stage['schemaVersion']==1 and stage['scope']=='local-source-capture-mechanical-only' and stage['runtimeAdmission'] is False and stage['runnerCommit'] is None and str(uuid.UUID(stage['runID']))==stage['runID'],'i1-capture-stage')
    policy=_i1_source_policy()
    require(stage['productSourceBase']==policy['productSourceBase'] and stage['productSourceTree']==policy['productSourceTree'] and stage['integrationPolicySHA256']==controller.i1_integration_policy_digest(),'i1-capture-policy')
    require(stage['toolManifestSHA256']==trusted_manifest_sha==sha(members['provenance/capture-tool-manifest.json']),'i1-capture-manifest-binding')
    for name in ('work','checkout','objectDB'):_i1_capture_validate_root(stage[name])
    roots=[stage[n] for n in ('work','checkout','objectDB')]
    for i,left in enumerate(roots):
        for right in roots[i+1:]:require(not Path(left['path']).is_relative_to(right['path']) and not Path(right['path']).is_relative_to(left['path']) and (left['dev'],left['inode'])!=(right['dev'],right['inode']),'i1-capture-root-overlap')
    manifest=_i1_capture_manifest(members['provenance/capture-tool-manifest.json'])
    commits={};trees={}
    for name,raw in members.items():
        match=re.fullmatch(r'provenance/integration-(commit|tree)-[0-9]{2}-([0-9a-f]{40})\.bin',name)
        if match:
            target=commits if match[1]=='commit' else trees
            require(match[2] not in target,'i1-capture-duplicate-object');target[match[2]]=raw
    i1_authenticate_source_originals(commits,trees)
    products={}
    for index,row in enumerate(i1_source_rows()):
        raw=b''.join(members[f'provenance/integration-product-{index:03}-chunk-{i:02}.bin'] for i in range(row['chunks']))
        if not row['bytes']:require(members[f'provenance/integration-product-{index:03}-empty.bin']==b'','i1-capture-empty')
        products[row['path']]=raw
    prows=sorted((r for r in manifest['files'] if r['path'].startswith('ports/mygo/scripts/swift-snapshot-oracle/go-probe/') and r['path'].endswith('.go')),key=lambda r:r['path'])
    probes={r['path']:members[f'provenance/integration-probe-{i:02}.bin'] for i,r in enumerate(prows)}
    inputs,expected=_i1_capture_inputs(products,probes,manifest);expected.update(stageSHA256=stage_sha)
    require(members['provenance/capture-sourceExpected.json']==_json_bytes(expected),'i1-capture-sourceExpected')
    raw_members=i1_source_raw_members(commits,trees,products,members['provenance/integration-batch-stdin.bin'],members['provenance/integration-batch-stdout.bin'],probes,manifest_bytes=members['provenance/capture-tool-manifest.json'])
    require(all(members[n]==raw for n,raw in raw_members.items()),'i1-capture-raw-closure')
    used=set(raw_members)|{'provenance/capture-stage.json','provenance/capture-tool-manifest.json','provenance/capture-sourceExpected.json'}
    db=_i1_capture_load_measurement(members,'database',stage_sha,used);db_sha=sha(json.dumps(db,sort_keys=True,separators=(',',':')).encode())
    require(set(db)=={'path','chain','directories','files','logicalBytes','allocatedBytes'},'i1-capture-database-schema')
    objects={**{oid:('commit',raw) for oid,raw in commits.items()},**{oid:('tree',raw) for oid,raw in trees.items()}}
    for row in i1_source_rows():objects[row['oid']]=('blob',products[row['path']])
    require(len(objects)==171,'i1-capture-db-objects')
    db_inputs={'HEAD':{'raw':(policy['productSourceBase']+'\n').encode(),'limit':4096},'config':{'raw':DB_CONFIG,'limit':4096}}
    for oid,(kind,raw) in objects.items():db_inputs['objects/'+oid[:2]+'/'+oid[2:]]={'oid':oid,'limit':len(kind)+len(str(len(raw)))+2+len(raw)+1024}
    _i1_capture_validate_measurement(db,db_inputs,stage['objectDB'],True)
    usage=_i1_capture_verify_usage(members,inputs,stage_sha,used)
    previous=0;processes=set();last_transport=None
    def check_span(before,after,transport):
        nonlocal previous,last_transport
        for row in (before,after):require(type(row) is dict and set(row)=={'measuredNs','databaseSHA256','stageSHA256'} and row['databaseSHA256']==db_sha and row['stageSHA256']==stage_sha and type(row['measuredNs']) is int,'i1-capture-call-measurement')
        require(previous<before['measuredNs']<=transport['parentStartedNs']<transport['endedNs']<=after['measuredNs'],'i1-capture-call-order')
        pre=usage[1+2*len(processes)];post=usage[2+2*len(processes)]
        require(pre['endedNs']<=before['measuredNs']<=transport['parentStartedNs'] and transport['endedNs']<=post['startedNs']<post['endedNs']<=after['measuredNs'],'i1-capture-usage-call-order')
        previous=after['measuredNs'];last_transport=transport
        require(transport['pid'] not in processes,'i1-capture-process-reuse');processes.add(transport['pid'])
    input_rows=[]
    for i in range(3):input_rows.extend(json.loads(raw_members[f'provenance/integration-inputs-{i:02}.json'])['rows'])
    bypath={r['path']:r for r in input_rows}
    for ordinal in range(1,135):
        name=f'provenance/capture-single-{ordinal:03}.json';used.add(name);record=_i1_json(members[name],131072,manifest=True)
        context=_i1_capture_single_context(stage,ordinal);row=context['source'];refs=bypath[row['path']]
        stderr_name=f'provenance/capture-single-{ordinal:03}-stderr.bin';used.add(stderr_name);stderr=members[stderr_name]
        require(type(record) is dict and set(record)=={'schemaVersion','scope','runtimeAdmission','stageSHA256','context','transport','before','after','stderr','stdout','emptyOriginal'},'i1-capture-receipt-schema')
        wanted={'schemaVersion':1,'scope':stage['scope'],'runtimeAdmission':False,'stageSHA256':stage_sha,'context':context,'transport':record['transport'],'before':record['before'],'after':record['after'],'stderr':_i1_source_ref(stderr_name,stderr),'stdout':refs['raw'],'emptyOriginal':refs['emptyOriginal']}
        require(_i1_exact(record,wanted),'i1-capture-single-receipt')
        _i1_validate_source_transport(record['transport'],products[row['path']],stderr,row['bytes'],0);check_span(record['before'],record['after'],record['transport'])
    batch_names={'provenance/integration-batch-'+n for n in ('stdin.bin','stdout.bin','stderr.bin','receipt.json')}|{f'provenance/integration-batch-child-{i:02}.bin' for i in range(5)}
    batch_context=_i1_source_batch_context(Path(stage['work']['path']),Path(stage['objectDB']['path']))
    i1_verify_batch_mechanical_evidence({n:members[n] for n in batch_names},batch_context,commits,trees);used|=batch_names
    link_name='provenance/capture-batch-link.json';used.add(link_name);link=_i1_json(members[link_name],131072,manifest=True)
    require(type(link) is dict and set(link)=={'stageSHA256','before','after','receipt'} and link['stageSHA256']==stage_sha and link['receipt']==_i1_source_ref('provenance/integration-batch-receipt.json',members['provenance/integration-batch-receipt.json']),'i1-capture-batch-link')
    check_span(link['before'],link['after'],json.loads(members['provenance/integration-batch-receipt.json'])['transport'])
    checkout=_i1_capture_load_measurement(members,'checkout',stage_sha,used)
    require(set(checkout)=={'path','chain','directories','files','logicalBytes','allocatedBytes'},'i1-capture-checkout-schema')
    _i1_capture_validate_measurement(checkout,{p:v for p,v in inputs.items() if p in products},stage['checkout'],checkout=True)
    first=_i1_capture_load_measurement(members,'postWrite',stage_sha,used);second=_i1_capture_load_measurement(members,'postExport',stage_sha,used)
    for measurement in (first,second):
        require(set(measurement)=={'path','chain','directories','files','logicalBytes','allocatedBytes','startedNs','endedNs'} and type(measurement['startedNs']) is int and type(measurement['endedNs']) is int and previous<measurement['startedNs']<measurement['endedNs'],'i1-capture-export-time')
        require(measurement['path']==stage['work']['path']+'/input/go-source' and measurement['chain'][:-2]==stage['work']['chain'],'i1-capture-export-path')
        root={k:measurement['directories'][''][k] for k in ('dev','inode','type','mode')};root.update(path=measurement['path'],chain=measurement['chain']);_i1_capture_validate_root(root)
        _i1_capture_validate_measurement(measurement,inputs,root)
        _i1_capture_fs_reject_shared_files(measurement,checkout,db)
        require(not {(r['dev'],r['inode']) for r in measurement['files'].values()} & {(r['dev'],r['inode']) for r in db['files'].values()},'i1-capture-export-db-alias')
    _i1_capture_fs_compare_measurements(first,second)
    require(previous<usage[271]['startedNs'] and usage[416]['endedNs']<first['startedNs'] and first['endedNs']<usage[417]['startedNs']<usage[417]['endedNs']<second['startedNs'] and second['endedNs']<usage[418]['startedNs'],'i1-capture-usage-export-order')
    require(set(members)==used and len(processes)==135,'i1-capture-member-set')
    return {'scope':stage['scope'],'runtimeAdmission':False,'mechanicalEvidenceVerified':True,'processes':135,'offlineOSReexecution':False,'offlineDiskReexecution':False,'stageSHA256':stage_sha}



def _i1_verify_source_capture_archive(archive_bytes,trusted_manifest_sha):
    """Read an explicit canonical USTAR byte string, with no filesystem access."""
    import io,tarfile
    require(type(archive_bytes) is bytes and len(archive_bytes)<=79*1048576,'i1-capture-archive-limit')
    selected={}
    with tarfile.open(fileobj=io.BytesIO(archive_bytes),mode='r:') as archive:
        for member in archive:
            require(len(selected)<8192 and member.isfile() and member.name not in selected and 0<=member.size<=131072,'i1-capture-archive-member')
            i1_safe_path(member.name);raw=archive.extractfile(member).read(131073)
            require(len(raw)==member.size,'i1-capture-archive-short');selected[member.name]=raw
    source={n:raw for n,raw in selected.items() if n.startswith('provenance/')}
    result=_i1_verify_source_capture(source,trusted_manifest_sha)
    stage_sha=sha(source['provenance/capture-stage.json']);db=_i1_capture_load_measurement(source,'database',stage_sha,set())
    retained=sum(map(len,source.values()))+9525260+db['logicalBytes']
    plan,budget=_package_plan(source,retained,[],True,False)
    require(dict(plan.members)==selected,'i1-capture-archive-closure')
    expected=bytearray()
    for (_,raw),header in zip(plan.members,plan.headers):
        expected.extend(header);expected.extend(raw);expected.extend(b'\0'*((-len(raw))%512))
    expected.extend(b'\0'*1024);expected.extend(b'\0'*((-len(expected))%10240))
    require(archive_bytes==bytes(expected),'i1-capture-archive-canonical')
    return {**result,'snapshotReady':True,'archiveBytes':len(archive_bytes),'budget':budget}


def _i1_capture_fs_usage(work, files, directories, logical_limit):
    """Bounded observed usage, not an OS quota or an adversarial race guarantee.

    The caller derives the exact current file/directory inventory from its fixed
    stage. Missing entries, additional entries, links and special files reject.
    Logical bytes count regular files; allocated bytes include directories.
    """
    require(type(files) is set and type(directories) is set and '' in directories
            and not (files & directories), 'capture-usage-inventory')
    require(type(logical_limit) is int and 0 <= logical_limit <= 82837504
            and len(files) <= 8192 and len(directories) <= 64,
            'capture-usage-limit')
    for name in files | (directories - {''}):
        _i1_capture_fs_relative(name)
    require(_i1_capture_fs_directories(files | (directories - {''})) <= directories,
            'capture-usage-parent-set')
    started = time.monotonic_ns()
    fd, _ = _i1_capture_fs_open_root(work)
    logical = 0
    allocated = 0
    try:
        measured_dirs = _i1_capture_fs_scan(fd, files, directories)
        allocated = sum(row['allocatedBytes'] for row in measured_dirs.values())
        for name in sorted(files):
            child = _i1_capture_fs_open_relative(fd, name)
            try:
                st = os.fstat(child)
                require(stat.S_ISREG(st.st_mode) and st.st_nlink == 1,
                        'capture-usage-regular-single-link')
                logical += st.st_size
                allocated += st.st_blocks * 512
                require(logical <= logical_limit, 'capture-usage-logical-limit')
            finally:
                os.close(child)
        return {'startedNs': started, 'endedNs': time.monotonic_ns(),
                'files': len(files), 'directories': len(directories),
                'logicalBytes': logical, 'allocatedBytes': allocated}
    finally:
        os.close(fd)



def _i1_capture_store_usage(members,usage,stage_sha):
    refs=[]
    for i in range((len(usage)+63)//64):
        name=f'provenance/capture-usage-{i:02}.json';raw=_json_bytes({'stageSHA256':stage_sha,'rows':usage[i*64:(i+1)*64]});members[name]=raw;refs.append(_i1_source_ref(name,raw))
    members['provenance/capture-usage.json']=_json_bytes({'stageSHA256':stage_sha,'count':len(usage),'parts':refs})


def _i1_capture_verify_usage(members,inputs,stage_sha,used):
    name='provenance/capture-usage.json';used.add(name);index=_i1_json(members[name],131072,manifest=True)
    require(type(index) is dict and set(index)=={'stageSHA256','count','parts'} and index['stageSHA256']==stage_sha and type(index['count']) is int and index['count']==419 and type(index['parts']) is list and len(index['parts'])==7,'i1-capture-usage-index')
    rows=[]
    for i,ref in enumerate(index['parts']):
        n=f'provenance/capture-usage-{i:02}.json';used.add(n)
        require(ref==_i1_source_ref(n,members[n]),'i1-capture-usage-ref');part=_i1_json(members[n],131072,manifest=True)
        require(type(part) is dict and set(part)=={'stageSHA256','rows'} and part['stageSHA256']==stage_sha and type(part['rows']) is list and len(part['rows'])==min(64,419-i*64),'i1-capture-usage-part');rows.extend(part['rows'])
    previous=0;partial=0
    export_dirs=len(_i1_capture_fs_directories(set(inputs)))
    work_dirs=len(_i1_capture_fs_directories({'input/go-source/'+p for p in inputs}))
    sizes=[len(v['bytes']) for p,v in sorted(inputs.items())]
    for i,row in enumerate(rows):
        require(type(row) is dict and set(row)=={'kind','startedNs','endedNs','files','directories','logicalBytes','allocatedBytes'} and all(type(row[k]) is int and row[k]>=0 for k in row if k!='kind'),'i1-capture-usage-row')
        require(previous<row['startedNs']<row['endedNs'] and row['logicalBytes']<=10485760,'i1-capture-usage-time');previous=row['endedNs']
        if i<271:expected=('work-empty',0,1,0)
        elif i<417:
            partial+=sizes[i-271];expected=('export-partial',i-270,export_dirs,partial)
        else:expected=('work-export',146,work_dirs,9525260)
        require((row['kind'],row['files'],row['directories'],row['logicalBytes'])==expected,'i1-capture-usage-scope')
    return rows


if __name__=='__main__':
    # controller须共享本次runner状态，不能另导入一份空COMMAND_EVIDENCE。
    sys.modules['runner']=sys.modules[__name__]
    sys.exit(_i1_child_entry() if sys.argv[1:]==[I1_FETCH_FLAG] else main())
