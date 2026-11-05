#!/usr/bin/env python3
"""Publish one new, already approved release; never replace assets or retry writes."""
import json
import os
from pathlib import Path
import subprocess
import sys
import release


def api(path):
    return json.loads(subprocess.check_output(['gh', 'api', path]))


def matching_releases(repository, tag):
    pages = json.loads(subprocess.check_output(['gh', 'api', '--paginate', '--slurp', f'repos/{repository}/releases?per_page=100']))
    return [r for page in pages for r in page if r['tag_name'] == tag]


def tag_commit(repository, tag):
    obj = api(f'repos/{repository}/git/ref/tags/{tag}')['object']
    for _ in range(8):
        if obj['type'] == 'commit':
            return obj['sha']
        release.require(obj['type'] == 'tag', 'tag does not identify a commit')
        obj = api(f'repos/{repository}/git/tags/{obj["sha"]}')['object']
    raise release.Refused('excessively nested tag')


def main():
    output = Path(sys.argv[1])
    repository, commit, tag = (os.environ[k] for k in ('GITHUB_REPOSITORY', 'GITHUB_SHA', 'GITHUB_REF_NAME'))
    release.require(os.environ['GITHUB_EVENT_NAME'] == 'push' and repository == release.REPOSITORY, 'tag publication context required')
    files = release.git_files()
    release.release_gate(files, tag)
    release.verify(output)
    manifest = json.loads((output / 'manifest.json').read_bytes())
    release.require(manifest['commit'] == commit and manifest['version'] == tag and manifest['channel'] == 'stable', 'artifact identity differs from approved tag')
    release.require(manifest['source_sha256'] == release.source_digest(files), 'artifact source differs from approved checkout')
    release.require(tag_commit(repository, tag) == commit, 'remote tag changed')
    release.require(not matching_releases(repository, tag), 'release or draft already exists; inspect it instead of retrying')
    assets = sorted(p for p in output.iterdir() if p.is_file())
    notes = output.parent / 'release-notes.md'
    notes.write_text(f'Noonmark Go {tag}\n\n源码 commit：`{commit}`。\n\n六目标便携归档、完整对应源代码、依赖许可、manifest.json 与 SHA256SUMS 一并提供。'
                     '\n\n产物没有发布者代码签名或 notarization；不是安装器。平台运行支持以源码中的验收报告为准。请先核对 SHA256SUMS。\n')
    # GitHub rejects an existing release. No --clobber and no automatic retries.
    subprocess.run(['gh', 'release', 'create', tag, '--repo', repository, '--verify-tag', '--draft', '--target', commit,
                    '--title', 'Noonmark Go ' + tag, '--notes-file', str(notes), *map(str, assets)], check=True)
    drafts = matching_releases(repository, tag)
    release.require(len(drafts) == 1, 'new draft cannot be uniquely identified; stop before publication')
    draft_id = drafts[0]['id']
    release.require(type(draft_id) is int and draft_id > 0, 'invalid draft release ID')
    published = api(f'repos/{repository}/releases/{draft_id}')
    expected = {p.name: (p.stat().st_size, 'sha256:' + release.sha(p.read_bytes())) for p in assets}
    actual = {x['name']: (x['size'], x.get('digest')) for x in published['assets']}
    release.require(published['id'] == draft_id and published['tag_name'] == tag and published['draft'] is True and published['prerelease'] is False and actual == expected,
                    'uploaded draft assets differ or lack server SHA256; leave draft unpublished for inspection')
    release.require(tag_commit(repository, tag) == commit, 'remote tag changed during upload; draft left unpublished')
    subprocess.run(['gh', 'api', '--method', 'PATCH', f'repos/{repository}/releases/{draft_id}', '-F', 'draft=false', '-f', 'make_latest=true'], check=True)
    final = api(f'repos/{repository}/releases/{draft_id}')
    release.require(final['draft'] is False and final['tag_name'] == tag, 'publication outcome not confirmed')
    print(final['html_url'])


if __name__ == '__main__':
    main()
