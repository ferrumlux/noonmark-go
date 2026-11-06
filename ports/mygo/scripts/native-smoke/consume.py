#!/usr/bin/env python3
"""Same-run artifact consumer; standard library only, no Go or dependency preparation."""
import argparse
import json
import os
import platform
from pathlib import Path
import shutil
import smoke


def consume(artifact, work, target, source, build_source, run_id, build_attempt, consumer_attempt):
    smoke.require(work.is_absolute() and artifact.is_absolute(), 'absolute isolated paths required')
    smoke.directory(work.parent)
    repo = Path(__file__).resolve().parents[4]
    smoke.require(work != repo and repo not in work.resolve().parents, 'work inside source repository')
    work.mkdir()  # fresh consumer identity; no reuse/reset
    status = {'passed': False, 'stage': 'provenance', 'source_sha': source,
              'build_source_sha': build_source, 'run_id': run_id,
              'build_attempt': build_attempt, 'consumer_attempt': consumer_attempt,
              'target': target, 'host_platform': platform.platform(),
              'python_version': platform.python_version(), 'python_implementation': platform.python_implementation(),
              'python_process_machine': platform.machine(), 'desktop_window_tested': False, 'ime_tested': False}
    def save():
        (work / 'stage.json').write_text(json.dumps(status, indent=2) + '\n')
    save()
    try:
        smoke.provenance(source, run_id, build_attempt)
        smoke.provenance(source, run_id, consumer_attempt)
        smoke.require(source == build_source, 'consumer/build source mismatch')
        status['stage'] = 'artifact-verification'
        save()
        exe = smoke.verify(artifact, target, source, run_id, build_attempt)
        shutil.copyfile(artifact / 'manifest.json', work / 'manifest.json')
        status['stage'] = 'native-host'
        save()
        status['native_host'] = smoke.native_host()
        smoke.require(status['native_host'] == target, 'native host/target mismatch')
        status['binary_sha256'] = smoke.digest(exe)
        status['stage'] = 'application-smoke'
        save()
        if os.name != 'nt':
            exe.chmod(0o700)
        smoke.smoke(exe, work)
        status['passed'] = True
        status['stage'] = 'completed'
    except Exception as error:
        status['failure'] = str(error)
        raise
    finally:
        save()


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__)
    for name in ('target', 'source', 'build-source', 'run-id', 'build-attempt', 'consumer-attempt'):
        p.add_argument('--' + name, required=True)
    p.add_argument('--artifact', type=Path, required=True)
    p.add_argument('--work', type=Path, required=True)
    a = p.parse_args()
    consume(a.artifact, a.work, a.target, a.source, a.build_source, a.run_id, a.build_attempt, a.consumer_attempt)
