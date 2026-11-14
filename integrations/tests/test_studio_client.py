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

    def test_inspect_lore_extracts_project_repo_namespace(self):
        repo = Path(self.temp.name) / 'lore-namespaced'
        (repo / '.lore').mkdir(parents=True)
        (repo / '.lore' / 'config.toml').write_text(
            'remote_url = "lores://token:secret@lore.roguefairy.net/ghostlore/client"\n',
            encoding='utf-8',
        )
        (repo / '.lore' / 'view').write_text('main@5\n', encoding='utf-8')
        with patch.object(bridge, '_run_cmd', return_value=(1, '', '')):
            info = bridge.inspect_vcs(repo, slug='ghostlore-client')
        self.assertEqual(info['remoteUri'], 'lores://lore.roguefairy.net/ghostlore/client')
        self.assertEqual(info['loreProject'], 'ghostlore')
        self.assertEqual(info['loreRepo'], 'client')

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

    def test_rest_v1_cross_tool_queries_and_artifact_flush_against_go_server(self):
        import os
        import subprocess
        server_dir = Path(__file__).parents[2] / 'studio-server'
        ext = '.exe' if os.name == 'nt' else ''
        bin_path = server_dir / 'bin' / f'studio-server{ext}'
        if not bin_path.is_file():
            subprocess.run(['go', 'build', '-o', str(bin_path), './cmd/studio-server'], cwd=str(server_dir), check=True)

        svc_key = 'stk_python_test_admin_key_9999'
        env = dict(os.environ)
        env['STUDIO_DATABASE_URL'] = ''
        env['DATABASE_URL'] = ''
        env['STUDIO_ADDR'] = '127.0.0.1:0'
        env['STUDIO_PRINT_LISTENER'] = '1'
        env['STUDIO_SERVICE_KEYS'] = f'pyci:admin:{svc_key}'

        proc = subprocess.Popen(
            [str(bin_path)],
            cwd=str(server_dir),
            env=env,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        try:
            ready_line = proc.stdout.readline().strip()
            self.assertTrue(ready_line.startswith('STUDIO_READY addr='))
            api_url = 'http://' + ready_line.split('addr=', 1)[1]

            with patch.dict(os.environ, {'STUDIO_KEY': svc_key, 'STUDIO_API_URL': api_url}):
                rest_client = bridge.Client(url='', project='', outbox=self.temp.name, api_url=api_url)
                me = rest_client.whoami(client_app='geppetto')
                self.assertEqual(me['clientApp'], 'geppetto')
                self.assertTrue(me['isAdmin'])

                created = rest_client.rest('POST', '/v1/projects', {'name': 'Fae Chronicles', 'kind': 'digital'})
                pid = created['id']
                rest_client.project_id = pid

                rev = created['revision']
                res1 = rest_client.rest('POST', f'/v1/projects/{pid}/commands', {
                    'revision': rev,
                    'command': 'repository',
                    'input': {'name': 'Fae Lore Art', 'system': 'lore', 'loreRepo': 'art-2d'},
                })
                res2 = rest_client.rest('POST', f'/v1/projects/{pid}/commands', {
                    'revision': res1['revision'],
                    'command': 'artifact',
                    'input': {'name': 'Fae Queen', 'kind': 'Character', 'track': 'art_2d', 'tool': 'geppetto'},
                })
                art_id = res2['project']['artifacts'][0]['id']

                # Query Aetherion projects & Geppetto Lore workspaces via REST /v1
                aeth = rest_client.my_projects(client_app='aetherion')
                self.assertEqual(aeth['clientApp'], 'aetherion')
                self.assertEqual(len(aeth['projects']), 1)
                self.assertEqual(aeth['projects'][0]['name'], 'Fae Chronicles')

                gepp = rest_client.my_workspaces(client_app='geppetto', vcs='lore', only_writable=True)
                self.assertEqual(len(gepp['workspaces']), 1)
                self.assertEqual(
                    gepp['workspaces'][0]['repositories'][0]['remoteUri'],
                    'lores://lore.roguefairy.net/fae-chronicles/art-2d',
                )
                self.assertEqual(gepp['workspaces'][0]['artifacts'][0]['id'], art_id)

                # Upload & publish local .kra file via durable outbox -> REST /v1/projects/{id}/files + /events
                kra_file = Path(self.temp.name) / 'fae_queen.kra'
                kra_file.write_bytes(b'KRITA_ZIP_ARCHIVE_CONTENT')
                rc = bridge.main([
                    '--outbox', self.temp.name,
                    '--api-url', api_url,
                    '--project-id', pid,
                    'artifact', str(kra_file),
                    '--artifact-id', art_id,
                    '--source', 'geppetto',
                ])
                self.assertEqual(rc, 0)
                updated_proj = rest_client.project()
                self.assertEqual(len(updated_proj['state']['artifactVersions']), 1)

                # Create "Klaus Syndicate" (Aetherion engine) and verify generalist + project configs (JSON & YAML)
                klaus = rest_client.rest('POST', '/v1/projects', {
                    'name': 'Klaus Syndicate',
                    'kind': 'digital',
                    'engine': 'aetherion',
                })
                klaus_id = klaus['id']
                k_rev = klaus['revision']
                k_res1 = rest_client.rest('POST', f'/v1/projects/{klaus_id}/commands', {
                    'revision': k_rev,
                    'command': 'repository',
                    'input': {
                        'name': 'Klaus Engine',
                        'slug': 'klaus-engine',
                        'system': 'git',
                        'remoteUri': 'git@github.com:RogueFairyStudios/klaus-syndicate.git',
                        'workspacePath': 'games/klaus-syndicate/project.godot',
                    },
                })
                rest_client.rest('POST', f'/v1/projects/{klaus_id}/commands', {
                    'revision': k_res1['revision'],
                    'command': 'repository',
                    'input': {
                        'name': 'Klaus Art & Animation',
                        'slug': 'klaus-art',
                        'system': 'lore',
                        'loreRepo': 'art',
                    },
                })

                studio_aeth_json = Path(self.temp.name) / 'roguefairy-aetherion.json'
                rc_cfg1 = bridge.main([
                    '--api-url', api_url,
                    'config', 'aetherion',
                    '--scope', 'studio',
                    '-o', str(studio_aeth_json),
                ])
                self.assertEqual(rc_cfg1, 0)
                parsed_studio_aeth = json.loads(studio_aeth_json.read_text(encoding='utf-8'))
                self.assertEqual(parsed_studio_aeth['scope'], 'studio')
                self.assertEqual(parsed_studio_aeth['tool'], 'aetherion')
                self.assertIn('auth.telluriancorp.info', parsed_studio_aeth['authentik']['baseUrl'])

                klaus_gepp_yaml = Path(self.temp.name) / 'klaus-syndicate-geppetto.yaml'
                rc_cfg2 = bridge.main([
                    '--api-url', api_url,
                    'config', 'geppetto',
                    '--project', 'klaus-syndicate',
                    '--format', 'yaml',
                    '-o', str(klaus_gepp_yaml),
                ])
                self.assertEqual(rc_cfg2, 0)
                yaml_text = klaus_gepp_yaml.read_text(encoding='utf-8')
                self.assertIn('schemaVersion: "rfs.studio.config/v1"', yaml_text)
                self.assertIn('name: "Klaus Syndicate"', yaml_text)
                self.assertIn('lores://lore.roguefairy.net/klaus-syndicate/art', yaml_text)

                klaus_dev_cfg = json.loads(rest_client.fetch_config('aetherion', project='klaus-syndicate', fmt='json'))
                self.assertEqual(klaus_dev_cfg['project']['engine'], 'aetherion')
                self.assertTrue(klaus_dev_cfg['project']['usesAetherion'])
                self.assertEqual(klaus_dev_cfg['workspace']['primaryWorkspacePath'], 'games/klaus-syndicate/project.godot')
        finally:
            proc.terminate()
            proc.wait(timeout=5)
            if proc.stdout:
                proc.stdout.close()
            if proc.stderr:
                proc.stderr.close()


if __name__ == '__main__':
    unittest.main()
