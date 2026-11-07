#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""Export a bounded, closed synthetic-evidence set; never run Swift or subprocesses.
The unchanged reference runner alone owns capture. Failed/partial runs are diagnostics,
not goldens. No directory recursion or work-root archive is used for export.
"""
import argparse
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import re
import zipfile

HERE = Path(__file__).resolve().parent
ORACLE = HERE.with_name('ordinary-category-oracle')
SPEC = importlib.util.spec_from_file_location('ordinary_category_reference', ORACLE / 'runner.py')
r = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(r)
REVIEWED = '4d81f1784ef074bba86895dda6127fcce4b293ad1f6a6ac3ed666a28519a0fe8'
WORKFLOW_SHA = '6e68e7e54705105148c9f3c6c6b697b755981aa656eadbd514029808c8c49a70'
REPO = HERE.parents[3]
WORKFLOW = REPO / '.github/workflows/ordinary-category-oracle.yml'
LABELS = ('xcode-version', 'swiftc-path', 'swift-version', 'macos-version', 'sdk-version',
          'compile-core', 'compile-oracle', 'capture')
MAX_EXPORT = 20 * 1024 * 1024
MAX_FILES = 550
PROVENANCE = ('SOURCE-CLOSURE.json', 'TOOL-MANIFEST.json', 'INPUT.json', 'LICENSE', 'staged-identity.json')


def encoded(value):
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':')) + '\n').encode()


def check_workflow(raw):
    # Exact-byte fail-closed lock, not a permissive/partial YAML security parser.
    r.require(hashlib.sha256(raw).hexdigest() == WORKFLOW_SHA, 'workflow differs from reviewed safety contract')


def add(files, name, raw, cap=262144):
    r.require(re.fullmatch(r'(?:provenance/|commands/|run/)?[A-Za-z0-9][A-Za-z0-9._-]{0,119}', name), 'export name')
    r.require(name not in files and isinstance(raw, bytes) and len(raw) <= cap, 'export file budget/duplicate')
    raw.decode('utf-8', errors='strict')  # No executables, opaque bytes or binaries.
    r.require(len(files) < MAX_FILES and sum(map(len, files.values())) + len(raw) <= MAX_EXPORT, 'export total budget')
    files[name] = raw


def read_into(files, work, name, target=None, cap=262144):
    raw = r.checked(work / name, cap)
    add(files, target or name, raw, cap)
    return raw


def optional(path):
    # Broken links are present and will be rejected by checked(), never ignored.
    return path.exists() or path.is_symlink()


def commands(files, work, complete):
    receipt_hashes = []
    seen_gap = False
    for label in LABELS:
        name = label + '-command.json'
        if not optional(work / name):
            r.require(not complete, 'missing command receipt')
            seen_gap = True
            continue
        r.require(not seen_gap, 'non-sequential command receipts')
        raw = read_into(files, work, name, 'commands/' + name, 65536)
        value = json.loads(raw)
        r.object_fields(value, {'argv', 'returncode', 'seconds', 'transportComplete', 'error', 'cleanupComplete',
                                'cleanupErrors', 'stdoutCount', 'stderrCount', 'stdout', 'stderr'})
        r.require(isinstance(value['argv'], list) and 1 <= len(value['argv']) <= 100 and
                  all(isinstance(v, str) and len(v) <= 4096 for v in value['argv']), 'command argv')
        r.require(type(value['transportComplete']) is bool and type(value['cleanupComplete']) is bool, 'command flags')
        r.require(isinstance(value['seconds'], (int, float)) and 0 <= value['seconds'] <= 1800, 'command duration')
        r.require(value['returncode'] is None or type(value['returncode']) is int, 'command return code')
        r.require(value['error'] is None or (isinstance(value['error'], str) and len(value['error']) <= 4096), 'command error')
        r.require(isinstance(value['cleanupErrors'], list) and len(value['cleanupErrors']) <= 8 and
                  all(isinstance(v, str) and len(v) <= 4096 for v in value['cleanupErrors']), 'cleanup diagnostics')
        if complete:
            r.require(value['transportComplete'] and value['cleanupComplete'] and value['returncode'] == 0 and
                      value['error'] is None and value['cleanupErrors'] == [], 'incomplete command witness')
        for pipe in ('stdout', 'stderr'):
            ref = value[pipe]
            suffix = '.raw' if value['transportComplete'] else '.diagnostic-prefix'
            filename = label + '-' + pipe + suffix
            r.object_fields(ref, {'path', 'bytes', 'sha256'})
            r.require(ref['path'] == filename, 'command pipe path')
            content = read_into(files, work, filename, 'commands/' + filename, 65536)
            r.require(type(ref['bytes']) is int and ref['bytes'] == len(content) and ref['sha256'] == r.digest(content), 'command pipe identity')
            count = value[pipe + 'Count']
            r.require(type(count) is int and len(content) <= count <= 73728, 'command pipe count')
            if value['transportComplete']:
                r.require(count == len(content), 'complete command count')
        receipt_hashes.append(r.digest(raw))
    return receipt_hashes


def collect(work, outcome):
    r.require(outcome in ('success', 'failure', 'skipped', 'cancelled', ''), 'capture outcome')
    r.verify(REVIEWED)
    files = {}
    complete = outcome == 'success'
    if not work.exists():
        r.require(not work.is_symlink() and not complete, 'missing successful capture')
        return files, False, 'capture-not-started'
    r.real_directory(work)
    if complete:
        result = r.verify_capture(work / 'run', r.checked(ORACLE / 'INPUT.json', 16384))
        receipt_raw = r.checked(work / 'capture-receipt.json', 65536)
        receipt = json.loads(receipt_raw)
        r.object_fields(receipt, set(result) | {'reviewedToolManifestSHA256', 'stagedIdentitySHA256', 'commandReceiptSHA256s'})
        r.require(all(receipt[k] == v for k, v in result.items() if k != 'sourceBoundExecution') and
                  receipt['sourceBoundExecution'] is True and receipt['reviewedToolManifestSHA256'] == REVIEWED, 'capture receipt binding')
        r.require(not optional(work / 'runner-failure.json') and not optional(work / 'run/failure.json'), 'conflicting failure witness')
        add(files, 'capture-receipt.json', receipt_raw, 65536)
        manifest_raw = read_into(files, work, 'run/manifest.json', cap=131072)
        r.require(r.digest(manifest_raw) == receipt['outputManifestSHA256'], 'output manifest changed')
        manifest = json.loads(manifest_raw)
        # verify_capture restricts these names, roles, case IDs and references; recheck bytes on export.
        for ref in manifest['files']:
            cap = 65536 if ref['role'] == 'observation' else 16384 if ref['role'] == 'frozen-original-input' else 262144
            raw = read_into(files, work, 'run/' + ref['path'], cap=cap)
            r.require(len(raw) == ref['bytes'] and r.digest(raw) == ref['sha256'], 'capture bytes changed')
        for name in PROVENANCE:
            raw = read_into(files, work, 'provenance/' + name)
            if name == 'staged-identity.json':
                r.require(r.digest(raw) == receipt['stagedIdentitySHA256'], 'stage witness identity')
            else:
                expected = ORACLE / ('upstream/LICENSE' if name == 'LICENSE' else name)
                r.require(raw == r.checked(expected), 'provenance differs from reviewed input')
    else:
        # Failed raw observation sets lack a complete manifest and are deliberately not exported.
        if optional(work / 'runner-failure.json'):
            raw = r.checked(work / 'runner-failure.json', 16384)
            value = json.loads(raw)
            r.object_fields(value, {'captureComplete', 'error', 'commandsCompletedOrFailed', 'semanticParityVerified', 'runtimeAdmission'})
            r.require(all(value[k] is False for k in ('captureComplete', 'semanticParityVerified', 'runtimeAdmission')) and
                      isinstance(value['error'], str) and len(value['error']) <= 2048 and
                      type(value['commandsCompletedOrFailed']) is int and 0 <= value['commandsCompletedOrFailed'] <= 8, 'runner failure contract')
            add(files, 'runner-failure.json', raw, 16384)
        if optional(work / 'run/failure.json'):
            raw = r.checked(work / 'run/failure.json', 16384)
            value = json.loads(raw)
            r.object_fields(value, {'schema', 'captureComplete', 'semanticParityVerified', 'error'})
            r.require(value['schema'] == 'ordinary-category-failure-v1' and value['captureComplete'] is False and
                      value['semanticParityVerified'] is False, 'harness failure boundary')
            r.stage(value['error'], ('error',))
            add(files, 'run/failure.json', raw, 16384)
    if optional(work / 'platform.json'):
        raw = r.checked(work / 'platform.json', 16384)
        value = json.loads(raw)
        r.object_fields(value, {'xcode', 'swift', 'os', 'sdk', 'arch', 'imageOS', 'imageVersion', 'runnerOS',
                                'officialHostedImageVerified', 'reviewedToolManifestSHA256', 'sourceClosureSHA256'})
        r.require(value['officialHostedImageVerified'] is False and value['reviewedToolManifestSHA256'] == REVIEWED and
                  value['sourceClosureSHA256'] == r.SOURCE_SHA, 'platform scope')
        add(files, 'platform.json', raw, 16384)
    else:
        r.require(not complete, 'missing platform metadata')
    hashes = commands(files, work, complete)
    if complete:
        r.require(hashes == receipt['commandReceiptSHA256s'], 'command receipt identity')
    return files, complete, 'complete-reference-capture' if complete else 'incomplete-reference-capture'


def export(work, evidence, outcome):
    r.require(work.is_absolute() and evidence.is_absolute() and work != evidence and
              work not in evidence.parents and evidence not in work.parents, 'disjoint absolute evidence path')
    r.require(not evidence.exists() and not evidence.is_symlink(), 'evidence root must be new')
    r.real_directory(evidence.parent)
    r.require(REPO not in evidence.parents and REPO not in work.parents and work != REPO and evidence != REPO, 'work and evidence must be external')
    evidence.mkdir(mode=0o700)
    valid = True
    try:
        files, complete, reason = collect(work, outcome)
    except Exception:
        # Never echo unchecked exception/file contents into an uploaded artifact.
        files, complete, reason, valid = {}, False, 'evidence-validation-failed', False
    summary = {'schema': 'ordinary-category-ci-export-v1', 'captureOutcome': outcome if outcome in ('success', 'failure', 'skipped', 'cancelled', '') else 'invalid',
               'captureComplete': complete, 'evidenceValidated': valid, 'reasonCode': reason,
               'semanticParityVerified': False, 'runtimeAdmission': False, 'fullSnapshotExecuted': False,
               'reviewedToolManifestSHA256': REVIEWED, 'sourceClosureSHA256': r.SOURCE_SHA,
               'inputSHA256': r.INPUT_SHA, 'files': [{'path': n, 'bytes': len(v), 'sha256': r.digest(v)} for n, v in sorted(files.items())]}
    add(files, 'export-summary.json', encoded(summary), 262144)
    archive = io.BytesIO()
    with zipfile.ZipFile(archive, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=6) as z:
        for name, raw in sorted(files.items()):
            info = zipfile.ZipInfo(name, (1980, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o100444 << 16
            z.writestr(info, raw)
    r.require(len(archive.getvalue()) <= MAX_EXPORT + 131072, 'zip budget')
    r.write(evidence / 'ordinary-category-evidence.zip', archive.getvalue())
    r.write(evidence / 'export-summary.json', encoded(summary))
    return valid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--work-root', required=True)
    parser.add_argument('--evidence-root', required=True)
    parser.add_argument('--capture-outcome', required=True)
    args = parser.parse_args()
    return 0 if export(Path(args.work_root), Path(args.evidence_root), args.capture_outcome) else 1


if __name__ == '__main__':
    raise SystemExit(main())
