#!/usr/bin/env python3
"""Cross-platform URI protocol dispatcher and user-scope installer for Rogue Fairy Studios tools."""
import argparse
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import urllib.parse
import webbrowser

SUPPORTED_SCHEMES = (
    'goethe',
    'geppetto',
    'godot',
    'carnifex',
    'lore',
    'uefn',
    'animafaber',
    'fontpit',
)

DEFAULT_BINS = {
    'goethe': 'goethe_imgui_demo',
    'geppetto': 'krita',
    'godot': 'godot',
    'carnifex': 'carnifex',
    'lore': 'lore',
    'uefn': 'UnrealEditorFortnite-Win64-Shipping.exe',
    'fontpit': 'fontpit',
    'code': 'code',
}

DEFAULT_URLS = {
    'animafaber': 'https://animafaber.roguefairy.net',
    'fontpit': 'https://fontpit.roguefairy.net',
}


class UriError(Exception):
    pass


def default_config_path():
    return Path(os.environ.get('ROGUE_URI_CONFIG', str(Path.home() / '.rogue-fairy' / 'uri-handler.json')))


def load_config(config_path=None, workspace_override=None):
    cfg_file = Path(config_path) if config_path else default_config_path()
    loaded = {}
    if cfg_file.is_file():
        loaded = json.loads(cfg_file.read_text(encoding='utf-8'))
    ws = (
        workspace_override
        or os.environ.get('ROGUE_WORKSPACE_ROOT')
        or loaded.get('workspaceRoot')
        or str(Path(__file__).resolve().parents[3])
    )
    bins = dict(DEFAULT_BINS)
    if isinstance(loaded.get('bins'), dict):
        bins.update({k: str(v) for k, v in loaded['bins'].items() if v})
    urls = dict(DEFAULT_URLS)
    if isinstance(loaded.get('urls'), dict):
        urls.update({k: str(v) for k, v in loaded['urls'].items() if v})
    if os.environ.get('ANIMAFABER_URL'):
        urls['animafaber'] = os.environ['ANIMAFABER_URL']
    if os.environ.get('FONTPIT_URL'):
        urls['fontpit'] = os.environ['FONTPIT_URL']
    return {
        'configPath': str(cfg_file),
        'workspaceRoot': str(Path(ws).resolve()),
        'bins': bins,
        'urls': urls,
    }


def _check_no_control(value):
    if any(ord(ch) < 32 or ord(ch) == 127 for ch in value):
        raise UriError('Control characters are prohibited in URI')


def safe_token(value, allow_colon=False):
    val = str(value or '').strip()
    if not val:
        raise UriError('Missing identifier parameter')
    _check_no_control(val)
    if val.startswith('-'):
        raise UriError('Identifier must not start with "-"')
    pattern = r'^[A-Za-z0-9._:/-]+$' if allow_colon else r'^[A-Za-z0-9._/-]+$'
    if not re.match(pattern, val) or '..' in val.replace('\\', '/').split('/'):
        raise UriError('Unsafe identifier parameter')
    return val


def safe_rel_path(value):
    raw = str(value or '').strip()
    if not raw:
        raise UriError('Missing relative path')
    _check_no_control(raw)
    if raw.startswith(('-', '/', '\\')) or re.match(r'^[A-Za-z]:', raw):
        raise UriError('Absolute or flag-like paths are prohibited')
    norm = raw.replace('\\', '/').strip('/')
    parts = [p for p in norm.split('/') if p]
    if not parts or any(p in ('.', '..') or p.startswith('-') for p in parts):
        raise UriError('Unsafe relative path traversal')
    return '/'.join(parts)


def resolve_in_workspace(workspace_root, *segments):
    base = Path(workspace_root).resolve()
    cleaned = [safe_rel_path(seg) for seg in segments if seg]
    if not cleaned:
        raise UriError('Missing workspace target path')
    candidate = base.joinpath(*cleaned).resolve()
    if candidate != base and base not in candidate.parents:
        raise UriError('Target escapes workspace root')
    return candidate


def safe_godot_ref(value):
    val = str(value or '').strip()
    _check_no_control(val)
    if val.startswith('-'):
        raise UriError('Flag injection rejected')
    if val.startswith(('res://', 'uid://')):
        tail = val.split('://', 1)[1]
        if not tail or '..' in tail.replace('\\', '/').split('/'):
            raise UriError('Invalid Godot resource URI')
        return val
    return 'res://' + safe_rel_path(val)


def safe_https_base(url_str):
    parsed = urllib.parse.urlsplit(url_str)
    is_loopback_http = parsed.scheme == 'http' and parsed.hostname in ('localhost', '127.0.0.1', '::1')
    if parsed.scheme != 'https' and not is_loopback_http:
        raise UriError('Web target must use HTTPS (or loopback HTTP)')
    if parsed.username or parsed.password:
        raise UriError('Web target must not contain credentials')
    return url_str.rstrip('/')


def build_launch_plan(raw_uri, config):
    raw_uri = str(raw_uri or '').strip()
    if not raw_uri or len(raw_uri) > 4000:
        raise UriError('Invalid URI length')
    _check_no_control(raw_uri)
    parsed = urllib.parse.urlsplit(raw_uri)
    scheme = parsed.scheme.lower()
    if scheme not in SUPPORTED_SCHEMES:
        raise UriError(f'Unsupported URI scheme: {scheme}')
    if parsed.username or parsed.password:
        raise UriError('Credentials are not allowed in tool URIs')

    params = dict(urllib.parse.parse_qsl(parsed.query, keep_blank_values=False))
    ws_root = config['workspaceRoot']
    bins = config['bins']
    urls = config['urls']

    if scheme == 'goethe':
        repo = params.get('repo', '')
        rel_path = params.get('path', '')
        target = resolve_in_workspace(ws_root, repo, rel_path)
        if str(target).endswith('.gproj.yaml') or params.get('editor') == 'code':
            line_num = int(params.get('line', '1')) if str(params.get('line', '1')).isdigit() else 1
            return {'scheme': scheme, 'kind': 'process', 'argv': [bins['code'], '--goto', f'{target}:{line_num}'], 'cwd': str(target.parent)}
        return {'scheme': scheme, 'kind': 'process', 'argv': [bins['goethe'], str(target)], 'cwd': str(target.parent)}

    if scheme == 'geppetto':
        repo = params.get('repo', '')
        rel_path = params.get('path', '')
        target = resolve_in_workspace(ws_root, repo, rel_path)
        argv = [bins['geppetto'], '--nosplash']
        if params.get('workspace'):
            argv.extend(['--workspace', safe_token(params['workspace'])])
        if params.get('layer'):
            layer_target = resolve_in_workspace(ws_root, repo, params['layer'])
            argv.extend(['--file-layer', str(layer_target)])
        argv.extend(['--', str(target)])
        return {'scheme': scheme, 'kind': 'process', 'argv': argv, 'cwd': str(target.parent)}

    if scheme == 'godot':
        project = params.get('project') or params.get('repo', '')
        proj_dir = resolve_in_workspace(ws_root, project)
        if proj_dir.is_file() and proj_dir.name == 'project.godot':
            proj_dir = proj_dir.parent
        scene = params.get('scene') or params.get('path', '')
        if params.get('debug') == '1':
            argv = [bins['godot'], '--path', str(proj_dir)]
            if scene:
                argv.extend(['--scene', safe_godot_ref(scene)])
            argv.append('-d')
            if params.get('breakpoint'):
                argv.extend(['-b', safe_godot_ref(params['breakpoint'])])
        else:
            argv = [bins['godot'], '--path', str(proj_dir), '-e']
            if scene:
                argv.append(safe_godot_ref(scene))
        return {'scheme': scheme, 'kind': 'process', 'argv': argv, 'cwd': str(proj_dir)}

    if scheme == 'carnifex':
        game = params.get('game') or params.get('repo', '')
        basedir = resolve_in_workspace(ws_root, game)
        argv = [bins['carnifex'], '-basedir', str(basedir)]
        if params.get('mod'):
            argv.extend(['-game', safe_token(params['mod'])])
        argv.append('-window')
        if params.get('map'):
            map_name = safe_token(Path(params['map']).stem)
            argv.extend(['+map', map_name])
        return {'scheme': scheme, 'kind': 'process', 'argv': argv, 'cwd': str(basedir)}

    if scheme == 'lore':
        repo = params.get('repo', '')
        rel_path = params.get('path', '')
        repo_dir = resolve_in_workspace(ws_root, repo)
        action = params.get('action', 'reveal')
        if action == 'sync':
            argv = [bins['lore'], '--repository', str(repo_dir), 'sync']
            if rel_path:
                argv.extend(['--root-file', safe_rel_path(rel_path)])
            return {'scheme': scheme, 'kind': 'process', 'argv': argv, 'cwd': str(repo_dir)}
        target = resolve_in_workspace(ws_root, repo, rel_path) if rel_path else repo_dir
        if os.name == 'nt':
            argv = ['explorer.exe', f'/select,{target}'] if rel_path else ['explorer.exe', str(repo_dir)]
        elif sys.platform == 'darwin':
            argv = ['open', '-R', str(target)] if rel_path else ['open', str(repo_dir)]
        else:
            argv = ['xdg-open', str(target.parent if rel_path else repo_dir)]
        return {'scheme': scheme, 'kind': 'reveal', 'argv': argv, 'cwd': str(repo_dir)}

    if scheme == 'uefn':
        project = params.get('project') or params.get('repo', '')
        target = resolve_in_workspace(ws_root, project)
        if target.is_dir():
            ext = '*.code-workspace' if params.get('verse') == '1' else '*.uefnproject'
            matches = sorted(target.glob(ext))
            if matches:
                target = matches[0]
        if params.get('verse') == '1':
            return {'scheme': scheme, 'kind': 'process', 'argv': [bins['code'], '--', str(target)], 'cwd': str(target.parent)}
        return {'scheme': scheme, 'kind': 'process', 'argv': [bins['uefn'], str(target)], 'cwd': str(target.parent)}

    if scheme == 'animafaber':
        spirit = safe_token(params.get('spirit') or parsed.path.strip('/'))
        base = safe_https_base(urls['animafaber'])
        return {'scheme': scheme, 'kind': 'url', 'url': f'{base}/?spirit={urllib.parse.quote(spirit)}', 'argv': []}

    if scheme == 'fontpit':
        family = safe_token(params.get('family') or parsed.path.strip('/'))
        if params.get('mode') == 'cli':
            return {'scheme': scheme, 'kind': 'process', 'argv': [bins['fontpit'], 'info', family], 'cwd': ws_root}
        base = safe_https_base(urls['fontpit'])
        return {'scheme': scheme, 'kind': 'url', 'url': f'{base}/v1/families/{urllib.parse.quote(family)}', 'argv': []}

    raise UriError(f'Unhandled scheme: {scheme}')


def windows_registry_entries(python_exe, script_path, config_path, schemes=SUPPORTED_SCHEMES):
    cmd = f'"{python_exe}" "{script_path}" --config "{config_path}" open "%1"'
    return {
        scheme: {
            'key': f'Software\\Classes\\{scheme}',
            'description': f'URL:Rogue Fairy {scheme.capitalize()} Protocol',
            'command': cmd,
        }
        for scheme in schemes
    }


def linux_desktop_entry(scheme, python_exe, script_path, config_path):
    return '\n'.join([
        '[Desktop Entry]',
        'Type=Application',
        f'Name=Rogue Fairy {scheme.capitalize()} Handler',
        f'Exec="{python_exe}" "{script_path}" --config "{config_path}" open %u',
        'Terminal=false',
        'NoDisplay=true',
        f'MimeType=x-scheme-handler/{scheme};',
        '',
    ])


def install_handlers(config, schemes=SUPPORTED_SCHEMES, dry_run=False):
    cfg_path = Path(config['configPath'])
    script_path = Path(__file__).resolve()
    python_exe = Path(sys.executable).resolve()
    reg_spec = windows_registry_entries(python_exe, script_path, cfg_path, schemes)
    result = {'platform': sys.platform, 'configPath': str(cfg_path), 'schemes': list(schemes), 'dryRun': dry_run}
    if dry_run:
        result['windowsRegistry'] = reg_spec
        result['linuxDesktopSample'] = linux_desktop_entry(schemes[0], python_exe, script_path, cfg_path)
        return result

    cfg_path.parent.mkdir(parents=True, exist_ok=True)
    cfg_path.write_text(json.dumps({
        'workspaceRoot': config['workspaceRoot'],
        'bins': config['bins'],
        'urls': config['urls'],
    }, indent=2), encoding='utf-8')

    if os.name == 'nt':
        import winreg
        for _, spec in reg_spec.items():
            with winreg.CreateKey(winreg.HKEY_CURRENT_USER, spec['key']) as root_key:
                winreg.SetValueEx(root_key, '', 0, winreg.REG_SZ, spec['description'])
                winreg.SetValueEx(root_key, 'URL Protocol', 0, winreg.REG_SZ, '')
            with winreg.CreateKey(winreg.HKEY_CURRENT_USER, spec['key'] + '\\shell\\open\\command') as cmd_key:
                winreg.SetValueEx(cmd_key, '', 0, winreg.REG_SZ, spec['command'])
    else:
        apps_dir = Path.home() / '.local' / 'share' / 'applications'
        apps_dir.mkdir(parents=True, exist_ok=True)
        xdg_mime = shutil.which('xdg-mime')
        for scheme in schemes:
            desktop_file = apps_dir / f'rogue-fairy-{scheme}.desktop'
            desktop_file.write_text(linux_desktop_entry(scheme, python_exe, script_path, cfg_path), encoding='utf-8')
            if xdg_mime:
                subprocess.run([xdg_mime, 'default', desktop_file.name, f'x-scheme-handler/{scheme}'], check=False)
    return result


def uninstall_handlers(schemes=SUPPORTED_SCHEMES, dry_run=False):
    removed = []
    if dry_run:
        return {'removed': list(schemes), 'dryRun': True}
    if os.name == 'nt':
        import winreg
        for scheme in schemes:
            base = f'Software\\Classes\\{scheme}'
            for sub in ('\\shell\\open\\command', '\\shell\\open', '\\shell', ''):
                try:
                    winreg.DeleteKey(winreg.HKEY_CURRENT_USER, base + sub)
                    if not sub:
                        removed.append(scheme)
                except OSError:
                    pass
    else:
        apps_dir = Path.home() / '.local' / 'share' / 'applications'
        for scheme in schemes:
            desktop_file = apps_dir / f'rogue-fairy-{scheme}.desktop'
            if desktop_file.exists():
                desktop_file.unlink()
                removed.append(scheme)
    return {'removed': removed, 'dryRun': False}


def doctor(config):
    ws = Path(config['workspaceRoot'])
    binaries = {}
    for key, val in config['bins'].items():
        resolved = shutil.which(val) or (str(Path(val)) if Path(val).is_file() else None)
        binaries[key] = {'configured': val, 'resolved': resolved, 'available': bool(resolved)}
    return {
        'configPath': config['configPath'],
        'workspaceRoot': str(ws),
        'workspaceExists': ws.is_dir(),
        'binaries': binaries,
        'urls': config['urls'],
        'schemes': list(SUPPORTED_SCHEMES),
    }


def execute_plan(plan):
    if plan['kind'] == 'url':
        webbrowser.open(plan['url'])
        return {'launched': plan['url'], 'kind': 'url'}
    cwd = plan.get('cwd')
    cwd_arg = cwd if cwd and Path(cwd).is_dir() else None
    kwargs = {'cwd': cwd_arg}
    if os.name == 'nt':
        kwargs['creationflags'] = getattr(subprocess, 'DETACHED_PROCESS', 0) | getattr(subprocess, 'CREATE_NEW_PROCESS_GROUP', 0)
    else:
        kwargs['start_new_session'] = True
    subprocess.Popen(plan['argv'], **kwargs)
    return {'launched': plan['argv'], 'kind': plan['kind']}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--config', help='Path to uri-handler.json')
    parser.add_argument('--workspace-root', help='Override studio workspace root')
    sub = parser.add_subparsers(dest='command', required=True)

    open_cmd = sub.add_parser('open', help='Dispatch a Rogue Fairy tool URI')
    open_cmd.add_argument('uri')
    open_cmd.add_argument('--dry-run', action='store_true')

    inst_cmd = sub.add_parser('install', help='Register URI schemes in user scope')
    inst_cmd.add_argument('--schemes', nargs='+', choices=SUPPORTED_SCHEMES, default=list(SUPPORTED_SCHEMES))
    inst_cmd.add_argument('--dry-run', action='store_true')

    uninst_cmd = sub.add_parser('uninstall', help='Remove user-scope URI registrations')
    uninst_cmd.add_argument('--schemes', nargs='+', choices=SUPPORTED_SCHEMES, default=list(SUPPORTED_SCHEMES))
    uninst_cmd.add_argument('--dry-run', action='store_true')

    sub.add_parser('doctor', help='Verify workspace root and configured tool binaries')

    args = parser.parse_args(argv)
    try:
        config = load_config(args.config, args.workspace_root)
        if args.command == 'open':
            plan = build_launch_plan(args.uri, config)
            output = plan if args.dry_run else execute_plan(plan)
        elif args.command == 'install':
            output = install_handlers(config, schemes=args.schemes, dry_run=args.dry_run)
        elif args.command == 'uninstall':
            output = uninstall_handlers(schemes=args.schemes, dry_run=args.dry_run)
        else:
            output = doctor(config)
        print(json.dumps(output, ensure_ascii=False, indent=2))
        return 0
    except (UriError, OSError, ValueError) as exc:
        print(f'URI handler error: {exc}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
