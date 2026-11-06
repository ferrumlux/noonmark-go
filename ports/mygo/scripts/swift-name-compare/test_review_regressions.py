#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""Independent SYNTHETIC-only regression tests; do not certify Swift behavior.

Uses the same explicit external build-root and Go environment as test_compare.py.
These tests never write into the source tree or contact a service.
"""
import copy
import unittest

import compare as c
import test_compare as fixtures


class ReviewRegressionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        fixtures.ComparisonTests.setUpClass()
        cls.capture = fixtures.ComparisonTests.capture
        cls.probe = fixtures.ComparisonTests.probe
        cls.inputs = fixtures.ComparisonTests.inputs

    def test_go_schema_requires_exact_integer(self):
        for value in (True, 1.0):
            with self.subTest(value=value):
                probe = copy.deepcopy(self.probe)
                probe['schemaVersion'] = value
                with self.assertRaises(c.Rejected):
                    c.compare(self.capture, probe)

    def test_go_scalars_require_exact_integers(self):
        probe = copy.deepcopy(self.probe)
        row = next(x for x in probe['cases'] if x['id'] == 'ascii')
        row['input']['unicodeScalars'] = [float(x) for x in row['input']['unicodeScalars']]
        with self.assertRaises(c.Rejected):
            c.compare(self.capture, probe)

    def test_go_pair_utf8_flags_are_internally_consistent(self):
        probe = copy.deepcopy(self.probe)
        pair = probe['pairs'][0]
        pair['input']['utf8Equal'] = not pair['input']['utf8Equal']
        with self.assertRaises(c.Rejected):
            c.compare(self.capture, probe)

    def test_go_shapes_and_identity_rejected(self):
        mutations = {
            'extra field': lambda p: p.__setitem__('unknown', True),
            'missing field': lambda p: p.pop('algorithmVersion'),
            'missing case': lambda p: p['cases'].pop(),
            'duplicate case': lambda p: p['cases'].__setitem__(1, p['cases'][0]),
            'reordered pairs': lambda p: p['pairs'].reverse(),
            'entity identity': lambda p: p['entityFields'][0].__setitem__('entity', 'Other'),
            'byte mismatch': lambda p: p['cases'][1]['input'].__setitem__('utf8Hex', '00'),
            'pair integer': lambda p: p['pairs'][0]['input'].__setitem__('swiftEqual', 1),
        }
        for label, mutation in mutations.items():
            with self.subTest(label=label):
                probe = copy.deepcopy(self.probe)
                mutation(probe)
                with self.assertRaises(c.Rejected):
                    c.compare(self.capture, probe)

    def test_coverage_paths_are_unique_and_boundaries_not_counted(self):
        capture = copy.deepcopy(self.capture)
        # Replace deliberately synthetic segmentation with one scalar per record.
        # Flip whitespace and entity flags too: none may become a compared field.
        for case in capture['cases']:
            for field in c.FIELDS:
                text = case[field]
                text['characters'] = [{'startScalar': i, 'endScalar': i+1, 'isWhitespace': True}
                                      for i in range(len(text['text']))]
                text['characterCount'] = len(text['text'])
        for row in capture['entityRoundTrips']:
            row.update(encodedByteCount=123, roundTripEqual=False, nameUTF8Equal=False, keyUTF8Equal=False)
        self.assertTrue(c.validate_capture(capture, self.inputs, fixtures.IDENTITY, synthetic=True))
        report = c.compare(capture, self.probe)
        paths = [row['path'] for row in report['checks']]
        self.assertEqual(len(paths), 1807)
        self.assertEqual(len(set(paths)), 1807)
        self.assertEqual(report['coverage']['mismatchCount'], 0)
        self.assertFalse(report['swiftVerified'])
        for excluded in ('characterCount', 'characters', 'isWhitespace', 'decodeStatus', 'encodedByteCount', 'roundTripEqual', 'nameUTF8Equal', 'keyUTF8Equal'):
            self.assertFalse(any(excluded in path for path in paths), excluded)
        for required in ('swiftCharacterBoundaries', 'swiftCharacterCount', 'swiftCharacterWhitespace', 'entityDecodeAndWire'):
            self.assertIn(required, report['notCompared'])


if __name__ == '__main__':
    unittest.main(verbosity=2)
