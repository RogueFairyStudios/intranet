import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('studio_client', Path(__file__).parents[1] / 'cli' / 'studio_client.py')
bridge = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bridge)


class BridgeTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.client = bridge.Client('http://127.0.0.1:3317/graphql', 'project', self.temp.name)

    def tearDown(self):
        self.temp.cleanup()

    def test_conflicts_retry_with_fresh_revision(self):
        with patch.object(self.client, 'project', side_effect=[{'revision': 1}, {'revision': 2}]), patch.object(self.client, 'graphql', side_effect=[bridge.Conflict('retry'), {'ok': True}]) as send:
            self.assertEqual(self.client.submit({'eventId': '1'}), {'ok': True})
            self.assertEqual(send.call_args[0][1]['revision'], 2)

    def test_outbox_retains_failed_events_and_recovers(self):
        event = {'source': 'goethe', 'eventId': 'safe', 'type': 'validation', 'payload': {'name': 'Dialogue', 'kind': 'test', 'result': 'passed'}}
        file = self.client.queue(event)
        with patch.object(self.client, 'submit', side_effect=bridge.StudioError('offline')):
            self.assertEqual(self.client.flush()['pending'], 1)
        self.assertEqual(json.loads(file.read_text())['event']['eventId'], 'safe')
        with patch.object(self.client, 'submit', return_value={}):
            self.assertEqual(self.client.flush(force=True)['sent'], 1)
        self.assertFalse(file.exists())

    def test_credit_checksum_is_verified_before_atomic_export(self):
        raw = '{"title":"Créditos","entries":[]}'
        checksum = hashlib.sha256(raw.encode()).hexdigest()
        destination = Path(self.temp.name) / 'credits.json'
        with patch.object(self.client, 'project', return_value={'state': {'credits': [{'id': 'snapshot', 'status': 'approved', 'checksum': checksum}]}}), patch.object(self.client, 'graphql', return_value={'studioCredits': raw}):
            self.client.credits('snapshot', destination)
        self.assertEqual(destination.read_bytes(), raw.encode())
        with patch.object(self.client, 'project', return_value={'state': {'credits': [{'id': 'snapshot', 'status': 'approved', 'checksum': 'invalid'}]}}), patch.object(self.client, 'graphql', return_value={'studioCredits': '{}'}):
            with self.assertRaises(bridge.StudioError):
                self.client.credits('snapshot', destination)
        self.assertEqual(destination.read_bytes(), raw.encode())

    def test_event_id_cannot_escape_outbox(self):
        path = self.client.queue({'source': 'generic', 'eventId': '../../escape', 'type': 'artifact', 'payload': {}})
        self.assertEqual(path.parent, Path(self.temp.name))

    def test_changed_queued_file_is_rejected(self):
        file = Path(self.temp.name) / 'art.png'
        file.write_bytes(b'new')
        with self.assertRaises(bridge.StudioError):
            self.client.upload_local_files({'payload': {'uri': file.as_uri(), 'checksum': 'a' * 64}})

    def test_non_loopback_http_is_rejected(self):
        self.client.url = 'http://studio.example/graphql'
        with self.assertRaises(bridge.StudioError):
            self.client.graphql('query {x}', {})

    def test_missing_file_does_not_block_later_events(self):
        self.client.queue({'source': 'generic', 'type': 'artifactVersion', 'payload': {'uri': (Path(self.temp.name) / 'missing.png').as_uri(), 'checksum': 'a' * 64}})
        self.client.queue({'source': 'generic', 'type': 'validation', 'payload': {'name': 'Test', 'kind': 'test', 'result': 'passed'}})
        with patch.object(self.client, 'submit', return_value={}):
            result = self.client.flush()
        self.assertEqual(result, {'sent': 1, 'pending': 1})


if __name__ == '__main__':
    unittest.main()
