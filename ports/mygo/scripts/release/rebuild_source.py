#!/usr/bin/env python3
"""Rebuild the supplied corresponding source without downloading modules."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', required=True, type=Path, help='new, absolute directory outside this source archive')
    parser.add_argument('--target', default='all')
    parser.add_argument('--go', default=os.environ.get('NOONMARK_GO', 'go'))
    args = parser.parse_args()
    root = Path(__file__).resolve().parent
    meta = json.loads((root / 'SOURCE.json').read_text())
    targets = meta['targets'] if args.target == 'all' else [args.target]
    if any(t not in meta['targets'] for t in targets):
        parser.error('unsupported target')
    output = args.output
    if not output.is_absolute() or output != output.resolve() or output == root or root in output.parents:
        parser.error('output must be an absolute physical directory outside extracted source')
    output.mkdir()
    env = dict(os.environ, GOWORK=str(root / 'go.work'), GOTOOLCHAIN='local', GOTELEMETRY='off', GOENV='off',
               GOPROXY='off', GOSUMDB='off', GOEXPERIMENT='', GODEBUG='', GOFLAGS='-mod=readonly', CGO_ENABLED='0', GOAMD64='v1', GOARM64='v8.0',
               GOCACHE=str(output / 'cache'), GOPATH=str(output / 'gopath'), GOMODCACHE=str(output / 'gopath/pkg/mod'),
               TMPDIR=str(output / 'tmp'), TMP=str(output / 'tmp'), TEMP=str(output / 'tmp'), GOMAXPROCS=os.environ.get('GOMAXPROCS', '2'))
    (output / 'tmp').mkdir()
    version = subprocess.check_output([args.go, 'env', 'GOVERSION'], env=env).decode().strip()
    if version != meta['go']:
        parser.error('exact recorded Go toolchain required: ' + meta['go'])
    for target in targets:
        system, arch = target.split('/')
        name = f'noonmark-mygo-{system}-{arch}' + ('.exe' if system == 'windows' else '')
        print('Offline source build ' + target, flush=True)
        subprocess.run([args.go, 'build', *meta['build_flags'], '-o', str(output / name), '.'],
                       cwd=root / 'repository/ports/mygo', env=dict(env, GOOS=system, GOARCH=arch), check=True)
    return 0


if __name__ == '__main__':
    sys.exit(main())
