import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location(
    'rogue_uri_handler',
    Path(__file__).parents[1] / 'uri_handler' / 'rogue_uri_handler.py',
)
handler = importlib.util.module_from_spec(spec)
spec.loader.exec_module(handler)


class UriHandlerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.ws = Path(self.temp.name)
        (self.ws / 'GhostLore' / 'dialogues').mkdir(parents=True)
        (self.ws / 'GhostLore' / 'dialogues' / 'intro.yaml').write_text('nodes: []\n', encoding='utf-8')
        (self.ws / 'GhostLore' / 'project.godot').write_text('[application]\n', encoding='utf-8')
        (self.ws / 'art' / 'portraits').mkdir(parents=True)
        (self.ws / 'art' / 'portraits' / 'hero.kra').write_bytes(b'KRA')
        (self.ws / 'carnifex_game').mkdir(parents=True)
        (self.ws / 'fortnite' / 'Island').mkdir(parents=True)
        (self.ws / 'fortnite' / 'Island' / 'Island.uefnproject').write_text('{}', encoding='utf-8')
        self.cfg = handler.load_config(workspace_override=str(self.ws))

    def tearDown(self):
        self.temp.cleanup()

    def test_dispatches_studio_tool_schemes(self):
        goethe_plan = handler.build_launch_plan('goethe://open?repo=GhostLore&path=dialogues/intro.yaml', self.cfg)
        self.assertEqual(goethe_plan['argv'][0], 'goethe_imgui_demo')
        self.assertTrue(goethe_plan['argv'][1].endswith('intro.yaml'))

        geppetto_plan = handler.build_launch_plan(
            'geppetto://open?repo=art&path=portraits/hero.kra&workspace=Animation',
            self.cfg,
        )
        self.assertEqual(geppetto_plan['argv'][:4], ['krita', '--nosplash', '--workspace', 'Animation'])
        self.assertEqual(geppetto_plan['argv'][-2], '--')

        godot_plan = handler.build_launch_plan(
            'godot://open?project=GhostLore&scene=res://scenes/main.tscn',
            self.cfg,
        )
        self.assertIn('-e', godot_plan['argv'])
        self.assertIn('res://scenes/main.tscn', godot_plan['argv'])

        aetherion_plan = handler.build_launch_plan(
            'aetherion://open?project=GhostLore&scene=res://scenes/main.tscn',
            self.cfg,
        )
        self.assertEqual(aetherion_plan['argv'][0], 'aetherion')
        self.assertIn('-e', aetherion_plan['argv'])
        self.assertIn('res://scenes/main.tscn', aetherion_plan['argv'])

        carnifex_plan = handler.build_launch_plan(
            'carnifex://open?game=carnifex_game&mod=id1&map=e1m1',
            self.cfg,
        )
        self.assertIn('+map', carnifex_plan['argv'])
        self.assertIn('e1m1', carnifex_plan['argv'])

        lore_sync = handler.build_launch_plan(
            'lore://open?repo=GhostLore&path=dialogues/intro.yaml&action=sync',
            self.cfg,
        )
        self.assertEqual(lore_sync['argv'][0], 'lore')
        self.assertIn('--root-file', lore_sync['argv'])

        uefn_plan = handler.build_launch_plan('uefn://open?project=fortnite/Island', self.cfg)
        self.assertTrue(uefn_plan['argv'][1].endswith('Island.uefnproject'))

        anima_plan = handler.build_launch_plan('animafaber://open?spirit=faust_01', self.cfg)
        self.assertEqual(anima_plan['kind'], 'url')
        self.assertEqual(anima_plan['url'], 'https://animafaber.roguefairy.net/?spirit=faust_01')

        font_plan = handler.build_launch_plan('fontpit://open?family=inter-tight&mode=cli', self.cfg)
        self.assertEqual(font_plan['argv'], ['fontpit', 'info', 'inter-tight'])

    def test_rejects_traversal_flags_and_credentials(self):
        bad_uris = [
            'goethe://open?repo=GhostLore&path=../../etc/passwd',
            'geppetto://open?repo=art&path=-evil.kra',
            'carnifex://open?game=carnifex_game&map=-basedir',
            'godot://open?project=GhostLore&scene=res://../../secret.tscn',
            'goethe://user:pass@open?repo=GhostLore&path=dialogues/intro.yaml',
            'javascript:alert(1)',
        ]
        for uri in bad_uris:
            with self.assertRaises(handler.UriError, msg=f'Should reject {uri}'):
                handler.build_launch_plan(uri, self.cfg)

    def test_dry_run_install_and_doctor(self):
        spec_out = handler.install_handlers(self.cfg, dry_run=True)
        self.assertIn('goethe', spec_out['windowsRegistry'])
        self.assertIn('x-scheme-handler/goethe;', spec_out['linuxDesktopSample'])
        doc = handler.doctor(self.cfg)
        self.assertTrue(doc['workspaceExists'])
        self.assertIn('lore', doc['schemes'])


if __name__ == '__main__':
    unittest.main()
