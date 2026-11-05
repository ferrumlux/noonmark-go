#!/usr/bin/env python3
"""Compare complete packages or rebuilds; extract only verified source members."""
import json
from pathlib import Path
import sys
import release


def compare(first, second):
    release.verify(first)
    release.verify(second)
    names = [p.name for p in first.iterdir() if p.is_file()]
    for name in names:
        release.require((first / name).read_bytes() == (second / name).read_bytes(), 'non-reproducible artifact: ' + name)
    print('PASS: every package, metadata and checksum is byte-identical across build roots')


def extract(package, output):
    release.verify(package)
    release.require(output.is_dir() and not any(output.iterdir()), 'extraction needs empty destination')
    meta = json.loads((package / 'manifest.json').read_bytes())
    archive = package / f'noonmark-go-{meta["version"]}-source.tar.gz'
    for name, (data, mode) in release.archive_entries(archive).items():
        path = output / name
        path.parent.mkdir(parents=True, exist_ok=True)
        release.write_new(path, data)
        path.chmod(mode)


def binaries(package, output):
    release.verify(package)
    meta = json.loads((package / 'manifest.json').read_bytes())
    for target, record in meta['binaries'].items():
        path = output / ('noonmark-mygo-' + target.replace('/', '-') + ('.exe' if target.startswith('windows/') else ''))
        release.require(release.sha(path.read_bytes()) == record['sha256'], 'source rebuild differs: ' + target)
    print('PASS: offline corresponding-source rebuild matches all six released binaries')


if __name__ == '__main__':
    if sys.argv[1] == '--extract': extract(Path(sys.argv[2]), release.physical(Path(sys.argv[3])))
    elif sys.argv[1] == '--binaries': binaries(Path(sys.argv[2]), Path(sys.argv[3]))
    else: compare(Path(sys.argv[1]), Path(sys.argv[2]))
