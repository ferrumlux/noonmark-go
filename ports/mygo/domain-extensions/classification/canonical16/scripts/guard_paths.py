#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""无写入路径门禁：保护源码 aggregate／Git repo，拒绝非规范路径、链接／特殊文件及交叠缓存。"""
import argparse
import os
from pathlib import Path
import stat
import sys

class UnsafePath(ValueError):
    pass

def overlaps(a, b):
    return a == b or a in b.parents or b in a.parents

def checked_absolute(raw, label):
    if not raw or any(not character.isprintable() for character in raw):
        raise UnsafePath(label + ' must not contain control or non-printable characters')
    if raw.startswith('//'):
        raise UnsafePath(label + ' must use exactly one leading slash')
    if not os.path.isabs(raw):
        raise UnsafePath(label + ' must be absolute')
    if os.path.normpath(raw) != raw:
        raise UnsafePath(label + ' must be normalized, without dot or parent components')
    path = Path(raw)
    # lstat each component before resolve: never conceal a symlink with realpath.
    for part in reversed([path, *path.parents]):
        try:
            info = part.lstat()
        except FileNotFoundError:
            continue
        if stat.S_ISLNK(info.st_mode):
            raise UnsafePath(label + ' contains a symlink')
        if not stat.S_ISDIR(info.st_mode):
            raise UnsafePath(label + ' has a non-directory component')
    return path

def is_repository(path):
    marker = path/'.git'
    # Sandboxed workspaces may expose empty read-only .git placeholders. They
    # are not repositories and must not expand the source boundary to /workspace.
    return marker.is_file() or marker.is_symlink() or (marker/'HEAD').is_file()

def source_roots(module_root):
    module = Path(module_root).resolve(strict=True)
    aggregate = module.parent  # Includes domain-core and application siblings.
    roots = {aggregate}
    # In a normal checkout, exclude the entire repository, not just ports/mygo.
    for parent in [module, *module.parents]:
        if is_repository(parent):
            roots.add(parent)
    # An isolated copy may sit next to the official/read-only reference repos.
    # Protect those repository roots as well, without reading their contents.
    workspace = aggregate.parent
    for sibling in workspace.iterdir():
        if sibling.is_dir() and is_repository(sibling):
            roots.add(sibling.resolve(strict=True))
    return sorted(roots)

def reject_unsafe_entries(root):
    if not root.exists():
        return
    def failed(error):
        raise UnsafePath('cannot inspect writable tree: ' + str(error))
    for current, directories, files in os.walk(root, followlinks=False, onerror=failed):
        for name in directories + files:
            info = (Path(current)/name).lstat()
            if stat.S_ISLNK(info.st_mode):
                raise UnsafePath('writable output/cache tree contains a symlink')
            if stat.S_ISREG(info.st_mode):
                if info.st_nlink != 1:
                    raise UnsafePath('writable output/cache tree contains a hardlink')
            elif not stat.S_ISDIR(info.st_mode):
                raise UnsafePath('writable output/cache tree contains a special file')

def validate(module_root, environment):
    roots = source_roots(module_root)
    paths = {name: checked_absolute(environment.get(name, ''), name)
             for name in ('NOONMARK_MYGO_BUILD_ROOT', 'GOPATH', 'GOCACHE')}
    # This script intentionally accepts one GOPATH, not a list whose secondary
    # entry could be hidden inside a source tree or overlap another cache.
    if os.pathsep in environment['GOPATH']:
        raise UnsafePath('GOPATH must contain exactly one absolute directory')
    for name, path in paths.items():
        for source in roots:
            if overlaps(path, source):
                raise UnsafePath(name + ' overlaps a source aggregate or repository')
    items = list(paths.items())
    for i, (name, path) in enumerate(items):
        for other_name, other in items[i+1:]:
            if overlaps(path, other):
                raise UnsafePath(name + ' overlaps ' + other_name)
    # Validate derived writable paths too; an existing nested symlink must not
    # redirect Go's module/temp writes after the root checks have passed.
    checked_absolute(str(paths['GOPATH']/'pkg/mod'), 'GOMODCACHE')
    checked_absolute(str(paths['NOONMARK_MYGO_BUILD_ROOT']/'tmp'), 'TMPDIR')
    for path in paths.values():
        reject_unsafe_entries(path)
    return paths

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--module-root', type=Path, required=True)
    args = parser.parse_args()
    try:
        paths = validate(args.module_root, os.environ)
    except (UnsafePath, OSError) as error:
        print('Path guard rejected: ' + str(error), file=sys.stderr)
        return 2
    # Validation has no path-valued stdout: callers must retain original bytes.
    return 0

if __name__ == '__main__':
    raise SystemExit(main())
