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

    def test_inspect_git_repository_status_and_strip_credentials(self):
        repo = Path(self.temp.name) / 'git-repo'
        repo.mkdir()
        import subprocess
        subprocess.run(['git', 'init', '-b', 'main'], cwd=str(repo), check=True, capture_output=True)
        subprocess.run(['git', 'config', 'user.name', 'Fairy Dev'], cwd=str(repo), check=True)
        subprocess.run(['git', 'config', 'user.email', 'dev@roguefairy.net'], cwd=str(repo), check=True)
        subprocess.run(['git', 'remote', 'add', 'origin', 'https://token:secret@github.com/RogueFairyStudios/GhostLore.git'], cwd=str(repo), check=True)
        tracked = repo / 'dialogue.yaml'
        tracked.write_text('id: intro\n', encoding='utf-8')
        subprocess.run(['git', 'add', 'dialogue.yaml'], cwd=str(repo), check=True)
        subprocess.run(['git', 'commit', '-m', 'Initial dialogue'], cwd=str(repo), check=True, capture_output=True)

        clean_info = bridge.inspect_vcs(repo, slug='ghostlore')
        self.assertEqual(clean_info['system'], 'git')
        self.assertEqual(clean_info['branch'], 'main')
        self.assertTrue(clean_info['clean'])
        self.assertEqual(clean_info['remoteUri'], 'https://github.com/RogueFairyStudios/GhostLore.git')
        self.assertEqual(clean_info['commitMessage'], 'Initial dialogue')

        tracked.write_text('id: intro_v2\n', encoding='utf-8')
        (repo / 'scratch.tmp').write_text('tmp', encoding='utf-8')
        dirty_info = bridge.inspect_vcs(repo, slug='ghostlore')
        self.assertFalse(dirty_info['clean'])
        self.assertEqual(dirty_info['modifiedCount'], 1)
        self.assertEqual(dirty_info['untrackedCount'], 1)
        self.assertIn('dialogue.yaml', dirty_info['modifiedFiles'])

    def test_inspect_lore_ndjson_revisions_and_locks(self):
        repo = Path(self.temp.name) / 'lore-repo'
        (repo / '.lore').mkdir(parents=True)
        (repo / '.lore' / 'config.toml').write_text('remote_url = "lores://user:pw@lore.roguefairy.net:41337/ghostlore"\n', encoding='utf-8')
        (repo / '.lore' / 'view').write_text('main@19\n', encoding='utf-8')

        def fake_run(argv, cwd=None, timeout=15):
            joined = ' '.join(argv)
            if 'revision info' in joined:
                return 0, json.dumps({
                    'tagName': 'revisionInfo',
                    'data': {
                        'branch': 'main',
                        'signature': 'e' * 64,
                        'revisionNumber': 21,
                        'message': 'Update spirit rig',
                        'author': 'rigger@roguefairy.net',
                    },
                }) + '\n', ''
            if 'status' in joined:
                return 0, json.dumps({
                    'tagName': 'fileStatus',
                    'data': {'path': 'Content\\Rigs\\Spirit.uasset', 'status': 'modified'},
                }) + '\n', ''
            if 'lock query' in joined:
                return 0, json.dumps({
                    'tagName': 'lock',
                    'data': {
                        'resource': {'description': 'Content/Rigs/Spirit.uasset', 'branch': 'main'},
                        'owner': 'rigger@roguefairy.net',
                        'locked_at': '2026-11-13T10:00:00Z',
                    },
                }) + '\n', ''
            return 1, '', ''

        with patch.object(bridge, '_run_cmd', side_effect=fake_run):
            info = bridge.inspect_vcs(repo, slug='ghostlore-lore')
        self.assertEqual(info['system'], 'lore')
        self.assertEqual(info['remoteUri'], 'lores://lore.roguefairy.net:41337/ghostlore')
        self.assertEqual(info['revisionNumber'], 21)
        self.assertFalse(info['clean'])
        self.assertEqual(info['modifiedFiles'], ['Content/Rigs/Spirit.uasset'])
        self.assertEqual(len(info['locks']), 1)
        self.assertEqual(info['locks'][0]['owner'], 'rigger@roguefairy.net')
        event = {'source': 'lore', 'type': 'vcsStatus', 'payload': info}
        queued = self.client.queue(event)
        self.assertTrue(queued.exists())

    def test_inspect_uefn_urc_workspace_and_stage_cli(self):
        repo = Path(self.temp.name) / 'TicketToDeath'
        (repo / '.urc').mkdir(parents=True)
        (repo / 'TicketToDeath.uefnproject').write_text(
            json.dumps({'title': 'Ticket To Death', 'bindings': {'projectId': '8feets-uuid-1234'}}),
            encoding='utf-8',
        )
        info = bridge.inspect_vcs(repo)
        self.assertEqual(info['system'], 'urc')
        self.assertEqual(info['compression'], 'oodle')
        self.assertIn('uefn://open?project=8feets-uuid-1234', info['remoteUri'])

        stage_event = {
            'source': 'goethe',
            'type': 'artifactStage',
            'payload': {'externalId': 'dlg-intro', 'stage': 'engine_integration'},
        }
        queued = self.client.queue(stage_event)
        self.assertTrue(queued.exists())


if __name__ == '__main__':
    unittest.main()
