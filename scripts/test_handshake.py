import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('handshake', Path(__file__).with_name('handshake.py'))
h = importlib.util.module_from_spec(spec)
spec.loader.exec_module(h)


class HandshakeTests(unittest.TestCase):
    def test_source_requires_success_and_permit_workflow(self):
        h.validate('a' * 40, '123', {'conclusion': 'success', 'path': '.github/workflows/permit-feed.yml'})
        for metadata in ({'conclusion': 'failure'}, {'conclusion': 'success', 'path': 'ci.yml'}):
            with self.assertRaises(ValueError):
                h.validate('a' * 40, '123', metadata)

    def test_rejects_untrusted_identifiers(self):
        with self.assertRaises(ValueError):
            h.validate('main; echo injected', '123', {})
