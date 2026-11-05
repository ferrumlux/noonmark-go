import copy
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
import tarfile
from unittest.mock import patch
import release
import check_hosted_ci


class ReleaseTests(unittest.TestCase):
    def approved(self):
        files = {'main.go': (b'package main\n', 0o644)}
        approval = {'schema': 1, 'approved': True, 'version': 'v1.0.0', 'source_sha256': release.source_digest(files),
                    'evidence': {}, 'reason': 'Synthetic unit-test fixture, not real acceptance.'}
        for name in release.REQUIREMENTS:
            path = release.EVIDENCE + name + '.json'
            evidence = {'schema': 1, 'requirement': name, 'result': 'passed', 'source_sha256': approval['source_sha256'],
                        'commands': ['synthetic test only'], 'observations': 'unit fixture', 'execution': 'native-desktop'}
            payload = release.canonical(evidence)
            files[path] = (payload, 0o644)
            approval['evidence'][name] = {'path': path, 'sha256': release.sha(payload)}
        files[release.APPROVAL] = (release.canonical(approval), 0o644)
        return files, approval

    def set_approval(self, files, approval):
        files[release.APPROVAL] = (release.canonical(approval), 0o644)

    def test_complete_source_bound_approval(self):
        files, _ = self.approved()
        release.release_gate(files, 'v1.0.0')

    def test_stable_tag_syntax(self):
        files, _ = self.approved()
        for tag in ('v01.0.0', 'v1.0', '1.0.0', 'v1.0.0-rc.1', 'v1.0.0;echo x', '../v1.0.0', 'v1.0.0\n'):
            with self.subTest(tag=tag), self.assertRaises(release.Refused): release.release_gate(files, tag)

    def test_default_approval_blocks(self):
        files, approval = self.approved()
        approval['approved'] = False
        self.set_approval(files, approval)
        with self.assertRaisesRegex(release.Refused, 'not approved'):
            release.release_gate(files, 'v1.0.0')

    def test_missing_unknown_changed_approval(self):
        for mutate in (lambda a: a.update(approved=False), lambda a: a.update(approved='true'),
                       lambda a: a.update(version='v2.0.0'), lambda a: a.update(source_sha256='0'*64),
                       lambda a: a['evidence'].pop('native-darwin-arm64'), lambda a: a.update(override=True)):
            files, approval = self.approved(); mutate(approval); self.set_approval(files, approval)
            with self.assertRaises(release.Refused): release.release_gate(files, 'v1.0.0')

    def test_source_change_invalidates_approval(self):
        files, _ = self.approved(); files['main.go'] = (b'package changed\n', 0o644)
        with self.assertRaisesRegex(release.Refused, 'stale'): release.release_gate(files, 'v1.0.0')

    def test_mode_change_invalidates_approval(self):
        files, _ = self.approved(); files['main.go'] = (files['main.go'][0], 0o755)
        with self.assertRaisesRegex(release.Refused, 'stale'): release.release_gate(files, 'v1.0.0')

    def test_evidence_must_match(self):
        for field, value in (('execution', 'cross-compiled'), ('result', 'pending'), ('source_sha256', '0'*64),
                             ('commands', []), ('observations', ''), ('requirement', 'different')):
            files, approval = self.approved()
            item = approval['evidence']['native-darwin-arm64']; path = item['path']
            report = json.loads(files[path][0]); report[field] = value
            payload = release.canonical(report); files[path] = (payload, 0o644)
            item['sha256'] = release.sha(payload); self.set_approval(files, approval)
            with self.subTest(field=field), self.assertRaises(release.Refused): release.release_gate(files, 'v1.0.0')

    def test_evidence_hash_tamper(self):
        files, _ = self.approved(); path = release.EVIDENCE + 'native-linux-amd64.json'
        files[path] = (files[path][0] + b' ', 0o644)
        with self.assertRaisesRegex(release.Refused, 'hash mismatch'): release.release_gate(files, 'v1.0.0')

    def test_path_safety(self):
        for name in ('', '/root', '../x', 'a/../b', 'a//b', 'a/./b', 'a\\b', 'a\0b'):
            with self.subTest(name=name), self.assertRaises(release.Refused): release.safe_name(name)
        self.assertEqual(release.safe_name('sub/LICENSE'), 'sub/LICENSE')

    def test_output_external_and_symlink_guard(self):
        with self.assertRaises(release.Refused): release.external(release.ROOT / 'output')
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp); (path / 'target').mkdir(); (path / 'link').symlink_to(path / 'target')
            with self.assertRaises(release.Refused): release.physical(path / 'link/new')

    def test_archives_deterministic_and_permissions(self):
        entries = {'b/file': (b'hello', 0o644), 'a': (b'world', 0o755)}
        for writer, ext in ((release.tar_bytes, '.tar.gz'), (release.zip_bytes, '.zip')):
            with self.subTest(ext=ext), tempfile.TemporaryDirectory() as tmp:
                first = writer(entries); second = writer(dict(reversed(list(entries.items()))))
                self.assertEqual(first, second)
                archive = Path(tmp) / ('archive' + ext); archive.write_bytes(first)
                self.assertEqual(release.archive_entries(archive), entries)

    def test_reject_symlink_and_duplicate_tar(self):
        for duplicate in (False, True):
            with tempfile.TemporaryDirectory() as tmp:
                path = Path(tmp) / 'bad.tar.gz'
                with tarfile.open(path, 'w:gz') as tar:
                    item = tarfile.TarInfo('a')
                    if duplicate:
                        tar.addfile(item, io.BytesIO()); tar.addfile(item, io.BytesIO())
                    else:
                        item.type = tarfile.SYMTYPE; item.linkname = '../../private'; tar.addfile(item)
                with self.assertRaises(release.Refused): release.archive_entries(path)

    def test_binary_target_checks(self):
        for target in release.TARGETS:
            with self.subTest(target=target), self.assertRaises(release.Refused): release.validate_binary(b'not an executable', target)

    def test_dependency_license_detection(self):
        for name in ('LICENSE', 'sub/LICENSE-MIT', 'NOTICE', 'COPYING.txt', 'font/OFL-LICENSE.txt'):
            self.assertTrue(release.license_name(name))
        self.assertFalse(release.license_name('license_parser.go'))  # implementation source is not a license notice

    def test_exact_hosted_ci(self):
        commit = 'a'*40
        run = {'head_sha': commit, 'status': 'completed', 'conclusion': 'success', 'event': 'push', 'head_branch': 'main',
               'head_repository': {'full_name': release.REPOSITORY}, 'path': '.github/workflows/mygo.yml'}
        check_hosted_ci.validate([run], commit, release.REPOSITORY)
        for field, value in (('head_sha', 'b'*40), ('status', 'in_progress'), ('conclusion', 'failure'),
                             ('event', 'pull_request'), ('head_branch', 'feature'), ('path', 'unrelated.yml'),
                             ('head_repository', {'full_name': 'other/repo'})):
            bad = dict(run, **{field: value})
            with self.subTest(field=field), self.assertRaises(release.Refused): check_hosted_ci.validate([bad], commit, release.REPOSITORY)
        with self.assertRaises(release.Refused): check_hosted_ci.validate([run], commit, 'other/repo')


if __name__ == '__main__':
    unittest.main()
