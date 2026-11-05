#!/usr/bin/env python3
"""Synthetic, offline fixtures only. All generated files stay in tempfile directories."""
import difflib
import importlib.util
import json
import os
from pathlib import Path
import stat
import subprocess
import tempfile
import unittest
import zipfile

HERE = Path(__file__).absolute().parent
spec = importlib.util.spec_from_file_location('prepare_dependency', HERE / 'prepare_dependency.py')
p = importlib.util.module_from_spec(spec)
spec.loader.exec_module(p)
os.sys.modules['prepare_dependency'] = p
verify_spec = importlib.util.spec_from_file_location('verify_selection', HERE / 'verify_selection.py')
v = importlib.util.module_from_spec(verify_spec)
verify_spec.loader.exec_module(v)


class PrepareTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='noonmark-dependency-fixture-')
        self.base = Path(self.tmp.name)
        self.repo = self.base / 'repo'
        self.source = self.repo / 'ports/mygo'
        self.source.mkdir(parents=True)
        self.bundle = self.base / 'bundle'
        self.bundle.mkdir()
        self.build = self.base / 'build'
        self.build.mkdir(mode=0o700)
        self.cache = self.base / 'cache'
        self.cache.mkdir()
        self.archive = self.cache / 'fixture.zip'
        self.manifest = self.bundle / 'manifest.json'
        self.original = b'package framework\n\nfunc Value() string { return "original" }\n'
        self.patched = b'package framework\n\nfunc NewAPI() string { return "patched" }\n'
        self.files = {'go.mod': b'module example.test/framework\n\ngo 1.27.3\n', 'LICENSE': b'MIT License\n\nSynthetic fixture copyright\n', 'framework.go': self.original}
        self.m = {'schema': 1, 'go': '1.27.3', 'module': {'path': 'example.test/framework', 'version': 'v1.0.0'}, 'patches': []}
        self.zip_fixture()
        self.patch_fixture({'framework.go': (self.original, self.patched)})
        for index, name in enumerate(p.MODULE_DIRS):
            path = self.source if name == '.' else self.source / name
            path.mkdir(exist_ok=True)
            (path / 'go.mod').write_text(f'module example.test/app{index}\n\ngo 1.27.3\n\nrequire example.test/framework v1.0.0\n')
            (path / 'example_test.go').write_text('package app\nimport ("testing"; f "example.test/framework")\nfunc TestNewAPI(t *testing.T) { if f.NewAPI() != "patched" { t.Fatal("not patched") } }\n')
        self.sync_sums()
        self.save()

    def tearDown(self):
        # Only this test's exclusive tempfile fixture, never user paths.
        self.tmp.cleanup()

    def zip_fixture(self, entries=None):
        prefix = 'example.test/framework@v1.0.0/'
        with zipfile.ZipFile(self.archive, 'w') as z:
            for name, content in (entries or self.files).items():
                info = zipfile.ZipInfo(prefix + name)
                info.external_attr = 0
                z.writestr(info, content)
        mod = self.m['module']
        mod.update(sum=p.hash1({prefix + n: d for n, d in self.files.items()}), gomod_sum=p.hash1({'go.mod': self.files['go.mod']}), zip_sha256=p.sha(self.archive.read_bytes()), license_sha256=p.sha(self.files['LICENSE']))

    def patch_fixture(self, changes):
        data = b''
        targets = {}
        for name, (before, after) in changes.items():
            old = '/dev/null' if before is None else 'a/' + name
            new = '/dev/null' if after is None else 'b/' + name
            data += ''.join(difflib.unified_diff((before or b'').decode().splitlines(True), (after or b'').decode().splitlines(True), fromfile=old, tofile=new)).encode()
            targets[name] = {'before': None if before is None else p.sha(before), 'after': None if after is None else p.sha(after)}
        (self.bundle / '0001.patch').write_bytes(data)
        self.m['patches'] = [{'file': '0001.patch', 'sha256': p.sha(data), 'files': targets}]

    def sync_sums(self):
        mod = self.m['module']
        text = f"{mod['path']} {mod['version']} {mod['sum']}\n{mod['path']} {mod['version']}/go.mod {mod['gomod_sum']}\n"
        (self.source / 'go.sum').write_text(text)

    def save(self):
        self.manifest.write_bytes(p.canonical(self.m))

    def run_prepare(self):
        return p.prepare(str(self.manifest), str(self.archive), str(self.source), str(self.repo), str(self.build))

    def rejected(self, message=None):
        self.save()
        with self.assertRaises((p.Refused, OSError), msg=message):
            self.run_prepare()

    def test_success_reuse_and_originals_unchanged(self):
        before = {path: path.read_bytes() for path in [self.archive, self.manifest, self.source / 'go.mod', self.source / 'go.sum']}
        work = self.run_prepare()
        self.assertEqual((work.parent / 'mygo/framework.go').read_bytes(), self.patched)
        self.assertEqual((work.parent / 'mygo/LICENSE').read_bytes(), self.files['LICENSE'])
        self.assertEqual(work, self.run_prepare())
        self.assertEqual(before, {path: path.read_bytes() for path in before})

    def test_archive_sha_mismatch(self):
        self.m['module']['zip_sha256'] = '0' * 64
        self.rejected()

    def test_official_h1_mismatch(self):
        self.m['module']['sum'] = 'h1:' + 'A' * 43 + '='
        self.rejected()

    def test_gomod_h1_mismatch(self):
        self.m['module']['gomod_sum'] = 'h1:' + 'A' * 43 + '='
        self.rejected()

    def test_license_mismatch(self):
        self.m['module']['license_sha256'] = '0' * 64
        self.rejected()

    def test_root_pin_mismatch(self):
        path = self.source / 'go.mod'
        path.write_text(path.read_text().replace('v1.0.0', 'v1.0.1'))
        self.rejected()

    def test_root_checksum_mismatch(self):
        path = self.source / 'go.sum'
        path.write_text(path.read_text().replace(self.m['module']['sum'], 'h1:' + 'A' * 43 + '='))
        self.rejected()

    def test_root_replace_refused(self):
        with (self.source / 'go.mod').open('a') as f:
            f.write('\nreplace example.test/framework => /tmp/unknown\n')
        self.rejected()

    def test_patch_hash_mismatch(self):
        self.m['patches'][0]['sha256'] = '0' * 64
        self.rejected()

    def test_patch_before_hash_mismatch(self):
        self.m['patches'][0]['files']['framework.go']['before'] = '0' * 64
        self.rejected()

    def test_patch_after_hash_mismatch(self):
        self.m['patches'][0]['files']['framework.go']['after'] = '0' * 64
        self.rejected()

    def mutate_patch(self, old, new):
        path = self.bundle / '0001.patch'
        data = path.read_bytes().replace(old, new)
        path.write_bytes(data)
        self.m['patches'][0]['sha256'] = p.sha(data)

    def test_patch_context_mismatch_no_fuzz(self):
        self.mutate_patch(b' package framework', b' package notframework')
        self.rejected()

    def test_patch_line_offset_refused(self):
        self.mutate_patch(b'@@ -1,3 +1,3 @@', b'@@ -2,3 +2,3 @@')
        self.rejected()

    def test_patch_undeclared_target_refused(self):
        self.mutate_patch(b'framework.go', b'other.go')
        self.rejected()

    def test_patch_parent_traversal_refused(self):
        self.mutate_patch(b'framework.go', b'../outside.go')
        self.rejected()

    def test_patch_protected_file_refused(self):
        self.patch_fixture({'LICENSE': (self.files['LICENSE'], b'MIT License\nchanged\n')})
        self.rejected()

    def test_patch_add_delete(self):
        self.patch_fixture({'framework.go': (self.original, None), 'replacement.go': (None, self.patched)})
        self.save()
        work = self.run_prepare()
        self.assertFalse((work.parent / 'mygo/framework.go').exists())
        self.assertEqual((work.parent / 'mygo/replacement.go').read_bytes(), self.patched)

    def test_patch_multiple_files_no_partial_application(self):
        self.patch_fixture({'framework.go': (self.original, self.patched), 'new.go': (None, b'package framework\n')})
        self.m['patches'][0]['files']['new.go']['after'] = '0' * 64
        self.rejected()
        self.assertEqual(list(self.build.iterdir()), [])

    def test_no_newline_marker(self):
        self.files['framework.go'] = b'old'
        self.zip_fixture()
        self.sync_sums()
        data = b'--- a/framework.go\n+++ b/framework.go\n@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file\n'
        (self.bundle / '0001.patch').write_bytes(data)
        self.m['patches'][0] = {'file': '0001.patch', 'sha256': p.sha(data), 'files': {'framework.go': {'before': p.sha(b'old'), 'after': p.sha(b'new')}}}
        self.save()
        work = self.run_prepare()
        self.assertEqual((work.parent / 'mygo/framework.go').read_bytes(), b'new')

    def test_build_symlink_refused(self):
        link = self.base / 'linked-build'
        link.symlink_to(self.build, target_is_directory=True)
        self.build = link
        self.rejected()

    def test_archive_symlink_refused(self):
        original = self.archive
        self.archive = self.cache / 'linked.zip'
        self.archive.symlink_to(original)
        self.rejected()

    def test_archive_hardlink_refused(self):
        os.link(self.archive, self.cache / 'hardlink.zip')
        self.rejected()

    def test_bundle_symlink_refused(self):
        target = self.bundle / '0001.patch'
        target.rename(self.bundle / 'original.patch')
        target.symlink_to(self.bundle / 'original.patch')
        self.rejected()

    def test_build_under_repo_refused(self):
        self.build = self.repo / 'build'
        self.build.mkdir(mode=0o700)
        self.rejected()

    def test_build_world_writable_refused(self):
        self.build.chmod(0o777)
        self.rejected()

    def test_unknown_namespace_untouched(self):
        namespace = self.build / p.NAMESPACE
        namespace.mkdir(mode=0o700)
        marker = namespace / 'unrelated.txt'
        marker.write_text('keep me')
        self.rejected()
        self.assertEqual(marker.read_text(), 'keep me')

    def test_output_tamper_refused_no_repair(self):
        work = self.run_prepare()
        target = work.parent / 'mygo/framework.go'
        target.chmod(0o600)
        target.write_bytes(b'tampered')
        target.chmod(0o400)
        self.rejected()
        self.assertEqual(target.read_bytes(), b'tampered')

    def test_output_extra_file_refused(self):
        work = self.run_prepare()
        work.parent.chmod(0o700)
        (work.parent / 'extra').write_bytes(b'unknown')
        work.parent.chmod(0o500)
        self.rejected()

    def test_output_symlink_refused(self):
        work = self.run_prepare()
        folder = work.parent / 'mygo'
        folder.chmod(0o700)
        target = folder / 'framework.go'
        target.unlink()
        target.symlink_to(self.source / 'go.mod')
        folder.chmod(0o500)
        self.rejected()

    def test_stale_lock_refused_no_cleanup(self):
        work = self.run_prepare()
        lock = work.parent.parent / (work.parent.name + '.lock')
        lock.mkdir(mode=0o700)
        self.rejected()
        self.assertTrue(lock.exists())

    def test_recipe_changes_with_workspace_mod(self):
        first = self.run_prepare()
        with (self.source / 'domain-core/go.mod').open('a') as f:
            f.write('// new source recipe\n')
        second = self.run_prepare()
        self.assertNotEqual(first, second)
        self.assertTrue(first.exists())

    def test_archive_prefix_mismatch(self):
        self.m['module']['version'] = 'v2.0.0'
        self.rejected()

    def test_archive_symlink_entry_refused(self):
        prefix = 'example.test/framework@v1.0.0/'
        with zipfile.ZipFile(self.archive, 'w') as z:
            for name, content in self.files.items():
                info = zipfile.ZipInfo(prefix + name)
                info.external_attr = ((stat.S_IFLNK | 0o777) << 16) if name == 'framework.go' else 0
                z.writestr(info, content)
        self.m['module']['zip_sha256'] = p.sha(self.archive.read_bytes())
        self.rejected()

    def test_archive_parent_traversal_refused(self):
        self.files['../escape'] = b'bad'
        self.zip_fixture()
        self.sync_sums()
        self.rejected()

    def test_workspace_go_version_too_old(self):
        self.m['go'] = '1.26.0'
        self.rejected()

    def test_duplicate_manifest_key_refused(self):
        raw = p.canonical(self.m).decode().replace('"schema":1', '"schema":1,"schema":1')
        self.manifest.write_text(raw)
        with self.assertRaises(p.Refused):
            self.run_prepare()

    def test_unknown_manifest_field_refused(self):
        self.m['ignore_mismatch'] = True
        self.rejected()

    def test_incomplete_output_refused(self):
        work = self.run_prepare()
        original = work.parent
        original.rename(original.parent / (original.name + '.kept-fixture'))
        original.mkdir(mode=0o500)
        self.rejected()
        self.assertEqual(list(original.iterdir()), [])

    def test_receipt_tamper_refused(self):
        work = self.run_prepare()
        receipt = work.parent / 'receipt.json'
        receipt.chmod(0o600)
        receipt.write_bytes(b'{}\n')
        receipt.chmod(0o400)
        self.rejected()

    def test_frozen_workspace_tamper_refused(self):
        work = self.run_prepare()
        work.chmod(0o600)
        work.write_bytes(b'go 1.27.3\n')
        work.chmod(0o400)
        self.rejected()

    def selection_fixture(self, work):
        directory = str(work.parent / 'mygo')
        return {'Path': 'example.test/framework', 'Version': 'v1.0.0', 'Dir': directory,
                'Replace': {'Path': directory, 'Dir': directory, 'GoMod': directory + '/go.mod'}}

    def test_selection_gate_success(self):
        work = self.run_prepare()
        v.verify_selection(p.canonical(self.selection_fixture(work)), self.manifest, work)

    def test_selection_gate_mvs_drift(self):
        work = self.run_prepare()
        selection = self.selection_fixture(work)
        selection['Version'] = 'v1.1.0'
        with self.assertRaises(p.Refused):
            v.verify_selection(p.canonical(selection), self.manifest, work)

    def test_selection_gate_wrong_replacement(self):
        work = self.run_prepare()
        selection = self.selection_fixture(work)
        selection['Replace']['Dir'] = str(self.source)
        with self.assertRaises(p.Refused):
            v.verify_selection(p.canonical(selection), self.manifest, work)

    def test_selection_gate_main_module_refused(self):
        work = self.run_prepare()
        selection = self.selection_fixture(work)
        selection['Main'] = True
        with self.assertRaises(p.Refused):
            v.verify_selection(p.canonical(selection), self.manifest, work)

    def move_archive_into_default_cache(self, root=None):
        root = root or self.build
        cached = root / 'go-path/pkg/mod/cache/download/example.test/framework/@v/v1.0.0.zip'
        cached.parent.mkdir(parents=True)
        self.archive.rename(cached)
        self.archive = cached
        return root / 'go-path/pkg/mod'

    def test_default_gopath_inside_build_is_read_only(self):
        cache = self.move_archive_into_default_cache()
        before = {str(path.relative_to(cache)): (path.read_bytes(), path.stat().st_mtime_ns)
                  for path in cache.rglob('*') if path.is_file()}
        work = self.run_prepare()
        self.assertEqual(work, self.run_prepare())
        after = {str(path.relative_to(cache)): (path.read_bytes(), path.stat().st_mtime_ns)
                 for path in cache.rglob('*') if path.is_file()}
        self.assertEqual(before, after)
        self.assertNotIn(cache, work.parents)

    def test_filesystem_root_refused(self):
        self.build = Path('/')
        self.rejected()

    def test_build_inside_canonical_module_cache_refused(self):
        cache = self.move_archive_into_default_cache(self.base / 'cache-owner')
        self.build = cache / 'unsafe-build'
        self.build.mkdir(mode=0o700)
        self.rejected()
        self.assertEqual(list(self.build.iterdir()), [])

    def test_namespace_cannot_contain_module_cache(self):
        cache = self.move_archive_into_default_cache(self.build / p.NAMESPACE)
        self.rejected()
        self.assertFalse((self.build / p.NAMESPACE / 'OWNER').exists())
        self.assertTrue(self.archive.exists())

    def test_concurrent_preparers(self):
        command = [os.sys.executable, '-B', str(HERE / 'prepare_dependency.py'), '--manifest', str(self.manifest), '--archive', str(self.archive), '--source-root', str(self.source), '--repo-root', str(self.repo), '--build-root', str(self.build)]
        processes = [subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE) for _ in range(12)]
        results = [(proc, *proc.communicate(timeout=20)) for proc in processes]
        successes = [out.decode().strip() for proc, out, err in results if proc.returncode == 0]
        self.assertTrue(successes)
        self.assertEqual(len(set(successes)), 1)
        for proc, out, err in results:
            if proc.returncode:
                self.assertIn(proc.returncode, (2,))
                self.assertTrue(b'lock' in err or b'OWNER' in err or b'File exists' in err, err)
        self.assertEqual(str(self.run_prepare()), successes[0])
        self.assertFalse(list((self.build / p.NAMESPACE).glob('*.partial-*')))

    @unittest.skipUnless(os.environ.get('NOONMARK_TEST_GO'), 'set NOONMARK_TEST_GO for offline real Go workspace smoke')
    def test_go_root_and_independent_modules_use_new_api(self):
        work = self.run_prepare()
        env = dict(os.environ, GOWORK=str(work), GOPROXY='off', GOSUMDB='off', GOTOOLCHAIN='local', GOTELEMETRY='off', GOENV='off', GOPATH=str(self.base / 'gopath'), GOCACHE=str(self.base / 'go-cache'), GOMAXPROCS='2', GOFLAGS='-mod=readonly')
        for name in p.MODULE_DIRS:
            path = self.source if name == '.' else self.source / name
            selected = subprocess.run([env['NOONMARK_TEST_GO'], 'list', '-m', '-json', 'example.test/framework'], cwd=path, env=env, capture_output=True, timeout=30)
            self.assertEqual(selected.returncode, 0, selected.stderr)
            v.verify_selection(selected.stdout, self.manifest, work)
            result = subprocess.run([env['NOONMARK_TEST_GO'], 'test', '-p=1', './...'], cwd=path, env=env, capture_output=True, text=True, timeout=180)
            self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        self.assertEqual(work, self.run_prepare())


if __name__ == '__main__':
    unittest.main(verbosity=2)
