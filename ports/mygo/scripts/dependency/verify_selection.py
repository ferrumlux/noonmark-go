#!/usr/bin/env python3
"""Read-only post-prepare gate: consume `go list -m -json MODULE` on stdin."""
import argparse
from pathlib import Path
import sys

sys.dont_write_bytecode = True
import prepare_dependency as p


def verify_selection(data, manifest, work):
    m, _ = p.read_manifest(p.physical(manifest, False))
    work = p.physical(work, False)
    receipt = p.load_json(p.read_file(work.parent / 'receipt.json'))
    p.require(receipt['recipe']['manifest'] == m, 'prepared manifest mismatch')
    p.require(receipt['files']['go.work'] == p.sha(p.read_file(work)), 'workspace content mismatch')
    result = p.load_json(data)
    expected = m['module']
    p.require(not result.get('Error') and not result.get('Main'), 'MyGo must remain an ordinary resolved dependency')
    p.require(result.get('Path') == expected['path'] and result.get('Version') == expected['version'], 'resolved module/version mismatch (MVS drift)')
    replacement = result.get('Replace', {})
    directory = str(work.parent / 'mygo')
    p.require(replacement.get('Path') == directory and replacement.get('Dir') == directory and result.get('Dir') == directory, 'resolved replacement directory mismatch')
    p.require(not replacement.get('Version') and replacement.get('GoMod') == directory + '/go.mod', 'unexpected replacement version/go.mod')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', required=True)
    parser.add_argument('--gowork', required=True)
    args = parser.parse_args()
    try:
        data = sys.stdin.buffer.read(1024 * 1024 + 1)
        p.require(len(data) <= 1024 * 1024, 'module JSON too large')
        verify_selection(data, args.manifest, args.gowork)
    except (p.Refused, OSError, ValueError, KeyError, TypeError, UnicodeError) as error:
        print('verify_selection: refused: ' + str(error), file=sys.stderr)
        return 2
    return 0


if __name__ == '__main__':
    sys.exit(main())
