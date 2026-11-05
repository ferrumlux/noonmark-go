import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import publish
import release


class PublishTests(unittest.TestCase):
    def test_draft_id_happy_path_and_failure_gates(self):
        for mode in ('success', 'source-mismatch', 'tag-moved', 'tag-moved-after-upload', 'existing', 'digest-mismatch', 'ambiguous-draft', 'upload-fails'):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp); output = root / 'assets'; output.mkdir()
                commit, tag = 'a' * 40, 'v1.0.0'
                (output / 'manifest.json').write_text(json.dumps({'commit': commit, 'version': tag, 'channel': 'stable', 'source_sha256': 'b'*64 if mode == 'source-mismatch' else release.source_digest({})}))
                assets = [{'name': p.name, 'size': p.stat().st_size, 'digest': 'sha256:' + release.sha(p.read_bytes())} for p in output.iterdir()]
                if mode == 'digest-mismatch': assets[0]['digest'] = 'sha256:' + '0' * 64
                draft = {'id': 42, 'tag_name': tag, 'draft': True, 'prerelease': False, 'assets': assets}
                final = dict(draft, draft=False, html_url='https://example.test/release')
                reads, writes = [], []
                matches = [[{'id': 41, 'tag_name': tag}]] if mode == 'existing' else [[], [draft, dict(draft, id=43)] if mode == 'ambiguous-draft' else [draft]]
                def api(path):
                    # Drafts must use /releases/ID, never the published-only /tags endpoint.
                    self.assertEqual(path, 'repos/' + release.REPOSITORY + '/releases/42')
                    reads.append(path)
                    return final if len(reads) == 2 else draft
                def run(args, check):
                    writes.append(args)
                    if mode == 'upload-fails': raise publish.subprocess.CalledProcessError(1, args)
                env = {'GITHUB_REPOSITORY': release.REPOSITORY, 'GITHUB_SHA': commit, 'GITHUB_REF_NAME': tag, 'GITHUB_EVENT_NAME': 'push'}
                with patch.dict(os.environ, env), patch.object(publish.sys, 'argv', ['publish.py', str(output)]), \
                     patch.object(release, 'git_files', return_value={}), patch.object(release, 'release_gate'), patch.object(release, 'verify'), \
                     patch.object(publish, 'tag_commit', side_effect=['b'*40] if mode == 'tag-moved' else [commit, 'b'*40 if mode == 'tag-moved-after-upload' else commit]), \
                     patch.object(publish, 'matching_releases', side_effect=matches), patch.object(publish, 'api', side_effect=api), \
                     patch.object(publish.subprocess, 'run', side_effect=run):
                    if mode == 'success': publish.main()
                    else:
                        with self.assertRaises((release.Refused, publish.subprocess.CalledProcessError)): publish.main()
                if mode == 'success':
                    self.assertEqual(len(writes), 2)
                    self.assertEqual(writes[0][:3], ['gh', 'release', 'create'])
                    self.assertIn('--draft', writes[0])
                    self.assertEqual(writes[1], ['gh', 'api', '--method', 'PATCH', 'repos/' + release.REPOSITORY + '/releases/42', '-F', 'draft=false', '-f', 'make_latest=true'])
                else:
                    self.assertEqual(len(writes), 0 if mode in ('source-mismatch', 'tag-moved', 'existing') else 1)
                    self.assertFalse(any('PATCH' in w for w in writes))

    def test_paginated_draft_lookup(self):
        pages = [[{'id': 1, 'tag_name': 'v0.1.0'}], [{'id': 2, 'tag_name': 'v1.0.0', 'draft': True}]]
        with patch.object(publish.subprocess, 'check_output', return_value=json.dumps(pages).encode()) as call:
            self.assertEqual(publish.matching_releases(release.REPOSITORY, 'v1.0.0'), pages[1])
            self.assertIn('--paginate', call.call_args.args[0])
            self.assertIn('--slurp', call.call_args.args[0])


if __name__ == '__main__': unittest.main()
