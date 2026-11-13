#!/usr/bin/env python3
"""Dependency-free Studio bridge. Credentials are only read from process environment."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

ALLOWED_SOURCES = (
    'geppetto',
    'goethe',
    'godot',
    'unity',
    'carnifex',
    'animafaber',
    'fontpit',
    'lore',
    'urc',
    'uefn',
    'git',
    'github',
    'generic',
)
ALLOWED_EVENT_TYPES = (
    'artifact',
    'artifactVersion',
    'artifactStage',
    'build',
    'validation',
    'vcsStatus',
)
PIPELINE_STAGES = (
    'concept',
    'blockout',
    'authoring',
    'surface_loc',
    'engine_integration',
    'polish',
)


class StudioError(Exception):
    pass


class Conflict(StudioError):
    pass


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise StudioError('Endpoint redirected; configure its final URL')


def atomic_json(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    temp = path.with_name(path.name + '.' + uuid.uuid4().hex + '.tmp')
    with temp.open('w', encoding='utf-8') as stream:
        json.dump(value, stream, ensure_ascii=False, indent=2)
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temp, path)


def sha256(path):
    digest = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            digest.update(chunk)
    return digest.hexdigest()


def slugify(value):
    clean = re.sub(r'[^a-z0-9._-]+', '-', str(value or '').strip().lower()).strip('-')
    if not clean or not clean[0].isalnum():
        clean = 'repo-' + (clean or 'default')
    return clean[:120]


def sanitize_remote_uri(raw):
    raw = (raw or '').strip()
    if not raw:
        return ''
    if re.match(r'^git@[A-Za-z0-9._-]+:[A-Za-z0-9._/-]+(?:\.git)?$', raw):
        return raw
    parsed = urllib.parse.urlsplit(raw)
    if not parsed.scheme:
        return ''
    host = parsed.hostname or ''
    if not host:
        return raw if '@' not in raw else ''
    netloc = host
    if parsed.port:
        netloc = f'{host}:{parsed.port}'
    if parsed.scheme == 'ssh' and parsed.username == 'git':
        netloc = f'git@{netloc}'
    return urllib.parse.urlunsplit((parsed.scheme, netloc, parsed.path, parsed.query, ''))


def _run_cmd(argv, cwd=None, timeout=15):
    try:
        proc = subprocess.run(
            argv,
            cwd=str(cwd) if cwd else None,
            capture_output=True,
            text=True,
            encoding='utf-8',
            errors='replace',
            timeout=timeout,
            check=False,
        )
        return proc.returncode, proc.stdout or '', proc.stderr or ''
    except (OSError, subprocess.SubprocessError) as exc:
        raise StudioError(f'Command execution failed: {argv[0]}') from exc


def parse_lore_ndjson(raw_output):
    events = []
    for line in (raw_output or '').splitlines():
        text_line = line.strip()
        if not text_line or not text_line.startswith('{'):
            continue
        try:
            obj = json.loads(text_line)
        except ValueError:
            continue
        if isinstance(obj, dict):
            events.append(obj)
    return events


def _inspect_git(repo_dir, git_bin='git'):
    code, branch_out, _ = _run_cmd([git_bin, 'rev-parse', '--abbrev-ref', 'HEAD'], cwd=repo_dir)
    branch = branch_out.strip() if code == 0 and branch_out.strip() else 'main'
    code, rev_out, _ = _run_cmd([git_bin, 'rev-parse', 'HEAD'], cwd=repo_dir)
    revision = rev_out.strip() if code == 0 else ''
    commit_message = ''
    author = ''
    if revision:
        code, log_out, _ = _run_cmd([git_bin, 'log', '-1', '--format=%s%n%an'], cwd=repo_dir)
        if code == 0:
            lines = log_out.splitlines()
            commit_message = lines[0].strip() if len(lines) > 0 else ''
            author = lines[1].strip() if len(lines) > 1 else ''
    _, remote_out, _ = _run_cmd([git_bin, 'config', '--get', 'remote.origin.url'], cwd=repo_dir)
    remote_uri = sanitize_remote_uri(remote_out.strip())

    ahead = behind = staged = modified = untracked = 0
    modified_files = []
    code, status_out, _ = _run_cmd([git_bin, 'status', '--porcelain=v1', '-b'], cwd=repo_dir)
    if code == 0:
        for line in status_out.splitlines():
            if line.startswith('## '):
                ahead_match = re.search(r'ahead\s+(\d+)', line)
                behind_match = re.search(r'behind\s+(\d+)', line)
                if ahead_match:
                    ahead = int(ahead_match.group(1))
                if behind_match:
                    behind = int(behind_match.group(1))
                continue
            if len(line) < 3:
                continue
            xy = line[:2]
            rel = line[3:].strip()
            if ' -> ' in rel:
                rel = rel.split(' -> ', 1)[1].strip()
            rel = rel.strip('"').replace('\\', '/').lstrip('/')
            if xy == '??':
                untracked += 1
                continue
            if xy[0] not in (' ', '?'):
                staged += 1
            if xy[1] not in (' ', '?'):
                modified += 1
            if rel and '..' not in rel.split('/') and len(modified_files) < 100:
                modified_files.append(rel)

    total_mod = staged + modified
    return {
        'system': 'git',
        'remoteUri': remote_uri,
        'branch': branch,
        'revision': revision,
        'revisionNumber': None,
        'commitMessage': commit_message,
        'author': author,
        'clean': total_mod == 0,
        'modifiedCount': total_mod,
        'stagedCount': staged,
        'untrackedCount': untracked,
        'ahead': ahead,
        'behind': behind,
        'modifiedFiles': modified_files,
        'locks': [],
        'compression': '',
    }


def _read_lore_config_remote(lore_dir):
    config_file = lore_dir / 'config.toml'
    if not config_file.is_file():
        return ''
    content = config_file.read_text(encoding='utf-8', errors='replace')
    match = re.search(r'remote_url\s*=\s*["\']([^"\']+)["\']', content)
    return sanitize_remote_uri(match.group(1)) if match else ''


def _inspect_lore(repo_dir, lore_bin='lore'):
    lore_dir = repo_dir / '.lore'
    remote_uri = _read_lore_config_remote(lore_dir)
    branch = 'main'
    revision = ''
    revision_number = None
    commit_message = ''
    author = ''
    modified_files = []
    locks = []

    view_file = lore_dir / 'view'
    if view_file.is_file():
        raw_view = view_file.read_text(encoding='utf-8', errors='replace').strip()
        if '@' in raw_view and len(raw_view) < 200:
            parts = raw_view.split('@', 1)
            if parts[0].strip():
                branch = parts[0].strip()
            if parts[1].strip().isdigit():
                revision_number = int(parts[1].strip())

    try:
        code, rev_out, _ = _run_cmd([lore_bin, '--repository', str(repo_dir), '--json', 'revision', 'info'])
    except StudioError:
        code, rev_out = 1, ''
    if code == 0:
        for ev in parse_lore_ndjson(rev_out):
            data = ev.get('data') if isinstance(ev.get('data'), dict) else ev
            if data.get('branch'):
                branch = str(data['branch'])
            sig = data.get('signature') or data.get('revision') or data.get('id')
            if sig:
                revision = str(sig)
            num = data.get('revisionNumber', data.get('revision_number', data.get('number')))
            if isinstance(num, int) and num >= 0:
                revision_number = num
            msg = data.get('message') or data.get('description') or data.get('summary')
            if msg:
                commit_message = str(msg).splitlines()[0][:1000]
            who = data.get('author') or data.get('owner') or data.get('user')
            if who:
                author = str(who)[:200]

    clean = True
    try:
        code, st_out, _ = _run_cmd([lore_bin, '--repository', str(repo_dir), '--json', 'status'])
    except StudioError:
        code, st_out = 1, ''
    if code == 0:
        for ev in parse_lore_ndjson(st_out):
            tag = str(ev.get('tagName', '')).lower()
            data = ev.get('data') if isinstance(ev.get('data'), dict) else ev
            if 'clean' in data and isinstance(data['clean'], bool):
                clean = clean and data['clean']
            entries = data.get('files') or data.get('changes') or data.get('modified')
            if isinstance(entries, list):
                for item in entries:
                    rel = item.get('path') if isinstance(item, dict) else str(item)
                    rel = str(rel or '').replace('\\', '/').lstrip('/')
                    if rel and '..' not in rel.split('/') and rel not in modified_files and len(modified_files) < 100:
                        modified_files.append(rel)
            elif any(k in tag for k in ('file', 'change', 'dirty', 'modified', 'added', 'deleted')) or 'path' in data:
                state_val = str(data.get('status', data.get('state', 'modified'))).lower()
                if state_val not in ('clean', 'unchanged', 'synced'):
                    rel = str(data.get('path', '')).replace('\\', '/').lstrip('/')
                    if rel and '..' not in rel.split('/') and rel not in modified_files and len(modified_files) < 100:
                        modified_files.append(rel)
        if modified_files:
            clean = False

    try:
        code, lk_out, _ = _run_cmd([lore_bin, '--repository', str(repo_dir), '--json', 'lock', 'query'])
    except StudioError:
        code, lk_out = 1, ''
    if code == 0:
        for ev in parse_lore_ndjson(lk_out):
            data = ev.get('data') if isinstance(ev.get('data'), dict) else ev
            candidates = data.get('locks') if isinstance(data.get('locks'), list) else [data]
            for entry in candidates:
                if not isinstance(entry, dict):
                    continue
                resource = entry.get('resource') if isinstance(entry.get('resource'), dict) else {}
                lpath = entry.get('path') or resource.get('description') or resource.get('path') or ''
                lpath = str(lpath).replace('\\', '/').lstrip('/')
                owner = entry.get('owner') or entry.get('user') or ''
                lbranch = entry.get('branch') or resource.get('branch') or branch
                locked_at = entry.get('lockedAt') or entry.get('locked_at') or None
                if lpath and owner and '..' not in lpath.split('/') and len(locks) < 200:
                    locks.append({
                        'path': lpath,
                        'owner': str(owner)[:200],
                        'branch': str(lbranch)[:200] if lbranch else '',
                        'lockedAt': str(locked_at) if locked_at else None,
                    })

    return {
        'system': 'lore',
        'remoteUri': remote_uri,
        'branch': branch,
        'revision': revision or (f'{branch}@{revision_number}' if revision_number is not None else ''),
        'revisionNumber': revision_number,
        'commitMessage': commit_message,
        'author': author,
        'clean': clean and len(modified_files) == 0,
        'modifiedCount': len(modified_files),
        'stagedCount': 0,
        'untrackedCount': 0,
        'ahead': 0,
        'behind': 0,
        'modifiedFiles': modified_files,
        'locks': locks,
        'compression': 'zstd',
    }


def _inspect_urc(repo_dir):
    uefn_files = sorted(repo_dir.glob('*.uefnproject'))
    project_id = ''
    title = repo_dir.name
    if uefn_files:
        try:
            doc = json.loads(uefn_files[0].read_text(encoding='utf-8-sig'))
            title = doc.get('title') or uefn_files[0].stem
            bindings = doc.get('bindings') if isinstance(doc.get('bindings'), dict) else {}
            project_id = str(bindings.get('projectId') or '')
        except (OSError, ValueError):
            title = uefn_files[0].stem
    remote_uri = f'uefn://open?project={urllib.parse.quote(project_id or slugify(title))}'
    has_urc = (repo_dir / '.urc').is_dir()
    return {
        'system': 'urc',
        'name': title,
        'remoteUri': remote_uri,
        'branch': 'main',
        'revision': project_id[:16] if project_id else '',
        'revisionNumber': None,
        'commitMessage': 'UEFN / Unreal Revision Control workspace',
        'author': '',
        'clean': True,
        'modifiedCount': 0,
        'stagedCount': 0,
        'untrackedCount': 0,
        'ahead': 0,
        'behind': 0,
        'modifiedFiles': [],
        'locks': [],
        'compression': 'oodle' if has_urc else '',
    }


def inspect_vcs(repo_dir, system='auto', slug=None, name=None, workspace_path=None, lore_bin=None, git_bin=None):
    root = Path(repo_dir).resolve(strict=True)
    detected = system
    if detected == 'auto':
        if (root / '.lore').is_dir():
            detected = 'lore'
        elif list(root.glob('*.uefnproject')) or (root / '.urc').is_dir():
            detected = 'urc'
        elif (root / '.git').exists():
            detected = 'git'
        else:
            raise StudioError('Unable to detect VCS system (.git, .lore, or .uefnproject not found)')
    if detected == 'git':
        info = _inspect_git(root, git_bin=git_bin or os.environ.get('GIT_BIN', 'git'))
    elif detected == 'lore':
        info = _inspect_lore(root, lore_bin=lore_bin or os.environ.get('LORE_BIN', 'lore'))
    elif detected == 'urc':
        info = _inspect_urc(root)
    else:
        raise StudioError(f'Unsupported VCS system: {detected}')

    resolved_name = name or info.get('name') or root.name
    resolved_slug = slugify(slug or resolved_name)
    info['name'] = resolved_name
    info['slug'] = resolved_slug
    if workspace_path:
        clean_ws = str(workspace_path).replace('\\', '/').lstrip('/')
        if '..' in clean_ws.split('/'):
            raise StudioError('Invalid workspace relative path')
        info['workspacePath'] = clean_ws
    return info


class Client:
    def __init__(self, url=None, project=None, outbox=None):
        self.url = url or os.environ.get('STUDIO_URL', '')
        self.project_id = project or os.environ.get('STUDIO_PROJECT_ID', '')
        self.outbox = Path(outbox or os.environ.get('STUDIO_OUTBOX', str(Path.home() / '.rogue-fairy' / 'outbox')))

    def graphql(self, query, variables):
        endpoint = urllib.parse.urlparse(self.url)
        if endpoint.scheme != 'https' and not (endpoint.scheme == 'http' and endpoint.hostname in ('localhost', '127.0.0.1', '::1')):
            raise StudioError('STUDIO_URL must be an HTTPS GraphQL endpoint (HTTP allowed only on loopback)')
        if endpoint.username or endpoint.password:
            raise StudioError('Do not embed authorization in STUDIO_URL')
        headers = {'Content-Type': 'application/json'}
        for env, header in [('STUDIO_AUTHORIZATION', 'Authorization'), ('STUDIO_COOKIE', 'Cookie')]:
            if os.environ.get(env):
                headers[header] = os.environ[env]
        request = urllib.request.Request(self.url, json.dumps({'query': query, 'variables': variables}).encode(), headers)
        try:
            with urllib.request.build_opener(NoRedirect).open(request, timeout=30) as response:
                data = json.load(response)
        except urllib.error.HTTPError as exc:
            raise StudioError('Studio HTTP status %s' % exc.code) from None
        except (urllib.error.URLError, TimeoutError, OSError, ValueError):
            raise StudioError('Studio connection or response failed') from None
        if data.get('errors'):
            if any('Project changed' in e.get('message', '') for e in data['errors']):
                raise Conflict('Project revision conflict')
            # Never echo server error bodies, which may include request headers.
            raise StudioError('Studio rejected request; check access and manifest fields')
        if not isinstance(data.get('data'), dict):
            raise StudioError('Invalid Studio response')
        return data['data']

    def project(self):
        data = self.graphql('query($id:String!){studioProject(id:$id){id revision state}}', {'id': self.project_id})['studioProject']
        data['state'] = json.loads(data['state']) if isinstance(data['state'], str) else data['state']
        return data

    def submit(self, event):
        for attempt in range(3):
            project = self.project()
            try:
                return self.graphql('mutation($projectId:String!,$revision:Int!,$command:String!,$input:String!){studioCommand(projectId:$projectId,revision:$revision,command:$command,input:$input){id revision}}', {'projectId': self.project_id, 'revision': project['revision'], 'command': 'integrationEvent', 'input': json.dumps(event)})
            except Conflict:
                if attempt == 2:
                    raise

    def queue(self, event):
        if not self.project_id or not self.url:
            raise StudioError('Set STUDIO_URL and STUDIO_PROJECT_ID')
        if event.get('source') not in ALLOWED_SOURCES or event.get('type') not in ALLOWED_EVENT_TYPES or not isinstance(event.get('payload'), dict):
            raise StudioError('Invalid integration event')
        event = dict(event)
        event.setdefault('eventId', str(uuid.uuid4()))
        # Random filenames prevent an eventId from becoming a filesystem path.
        path = self.outbox / (uuid.uuid4().hex + '.json')
        atomic_json(path, {'url': self.url, 'projectId': self.project_id, 'event': event, 'attempts': 0, 'nextAttempt': 0})
        return path

    def flush(self, force=False):
        self.outbox.mkdir(parents=True, exist_ok=True)
        lock = self.outbox / '.flush-lock'
        try:
            lock.mkdir()
        except FileExistsError:
            raise StudioError('Outbox is busy; after a crashed process remove .flush-lock manually') from None
        sent = pending = 0
        try:
            for path in sorted(self.outbox.glob('*.json')):
                record = json.loads(path.read_text(encoding='utf-8'))
                if record['url'] != self.url or record['projectId'] != self.project_id:
                    continue
                if not force and record['nextAttempt'] > time.time():
                    pending += 1
                    continue
                try:
                    self.upload_local_files(record['event'])
                    atomic_json(path, record)
                    self.submit(record['event'])
                    path.unlink()
                    sent += 1
                except (StudioError, OSError, ValueError, KeyError):
                    record['attempts'] += 1
                    record['nextAttempt'] = time.time() + min(3600, 2 ** min(record['attempts'], 12))
                    atomic_json(path, record)
                    pending += 1
        finally:
            lock.rmdir()
        return {'sent': sent, 'pending': pending}

    def upload_local_files(self, event):
        payload = event['payload']
        for field in ('uri', 'previewUri'):
            value = payload.get(field, '')
            if not value.startswith('file:'):
                continue
            parsed = urllib.parse.urlparse(value)
            if parsed.netloc not in ('', 'localhost'):
                raise StudioError('Local uploads require a local file path')
            file = Path(urllib.request.url2pathname(parsed.path))
            if field == 'uri' and sha256(file) != payload.get('checksum'):
                raise StudioError('Queued file changed; create a new export event')
            payload[field] = self.upload(file)['url']

    def upload(self, path):
        endpoint = urllib.parse.urlparse(self.url)
        if endpoint.scheme != 'https' and not (endpoint.scheme == 'http' and endpoint.hostname in ('localhost', '127.0.0.1', '::1')):
            raise StudioError('Upload requires HTTPS or loopback')
        if endpoint.username or endpoint.password:
            raise StudioError('Invalid upload endpoint')
        path = Path(path)
        if path.stat().st_size > 50 * 1024 * 1024:
            raise StudioError('Upload exceeds 50 MiB; use a shared artifact URL')
        content = path.read_bytes()
        boundary = 'Studio' + uuid.uuid4().hex
        # The server keeps the content hash, so a fixed safe transport filename suffices.
        safe_name = re.sub(r'[^A-Za-z0-9._-]', '_', path.name)[:180] or 'artifact.bin'
        body = ('--' + boundary + '\r\nContent-Disposition: form-data; name="file"; filename="' + safe_name + '"\r\nContent-Type: application/octet-stream\r\n\r\n').encode() + content + ('\r\n--' + boundary + '--\r\n').encode()
        url = urllib.parse.urljoin(self.url, '/p/studio/files/' + urllib.parse.quote(self.project_id, safe=''))
        headers = {'Content-Type': 'multipart/form-data; boundary=' + boundary}
        for env, header in [('STUDIO_AUTHORIZATION', 'Authorization'), ('STUDIO_COOKIE', 'Cookie')]:
            if os.environ.get(env):
                headers[header] = os.environ[env]
        request = urllib.request.Request(url, body, headers)
        try:
            with urllib.request.build_opener(NoRedirect).open(request, timeout=60) as response:
                result = json.load(response)
        except (urllib.error.URLError, OSError, ValueError):
            raise StudioError('Artifact upload failed') from None
        if result.get('checksum') != hashlib.sha256(content).hexdigest():
            raise StudioError('Uploaded checksum mismatch')
        result['url'] = urllib.parse.urljoin(self.url, result['path'])
        if urllib.parse.urlparse(result['url']).netloc != endpoint.netloc:
            raise StudioError('Invalid artifact response location')
        return result

    def credits(self, snapshot_id, destination):
        project = self.project()
        snapshot = next((s for s in project['state']['credits'] if s['id'] == snapshot_id), None)
        if not snapshot or snapshot['status'] != 'approved':
            raise StudioError('Select an approved credits snapshot')
        raw = self.graphql('query($projectId:String!,$snapshotId:String!,$format:String!){studioCredits(projectId:$projectId,snapshotId:$snapshotId,format:$format)}', {'projectId': self.project_id, 'snapshotId': snapshot_id, 'format': 'json'})['studioCredits']
        content = raw.encode('utf-8')
        checksum = hashlib.sha256(content).hexdigest()
        if checksum != snapshot['checksum']:
            raise StudioError('Credits checksum mismatch')
        destination = Path(destination)
        destination.parent.mkdir(parents=True, exist_ok=True)
        temp = destination.with_name(destination.name + '.tmp-' + uuid.uuid4().hex)
        temp.write_bytes(content)
        os.replace(temp, destination)
        atomic_json(str(destination) + '.receipt.json', {'projectId': self.project_id, 'snapshotId': snapshot_id, 'checksum': checksum, 'status': 'approved'})
        return {'path': str(destination), 'checksum': checksum}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--outbox')
    sub = parser.add_subparsers(dest='command', required=True)
    sub.add_parser('tasks')
    flush = sub.add_parser('flush')
    flush.add_argument('--force', action='store_true')
    event = sub.add_parser('event')
    event.add_argument('manifest')
    artifact = sub.add_parser('artifact')
    artifact.add_argument('file')
    artifact.add_argument('--artifact-id', required=True)
    artifact.add_argument('--source', default='generic')
    artifact.add_argument('--version')
    artifact.add_argument('--preview')
    artifact.add_argument('--stage', choices=PIPELINE_STAGES)
    artifact.add_argument('--tool-uri')
    artifact.add_argument('--vcs-revision')
    artifact.add_argument('--vcs-branch')
    stage_cmd = sub.add_parser('stage')
    stage_cmd.add_argument('target')
    stage_cmd.add_argument('stage', choices=PIPELINE_STAGES)
    stage_cmd.add_argument('--by-id', action='store_true')
    stage_cmd.add_argument('--source', default='generic')
    stage_cmd.add_argument('--tool-uri')
    stage_cmd.add_argument('--vcs-revision')
    stage_cmd.add_argument('--note')
    vcs_cmd = sub.add_parser('vcs-sync')
    vcs_cmd.add_argument('path')
    vcs_cmd.add_argument('--slug')
    vcs_cmd.add_argument('--name')
    vcs_cmd.add_argument('--system', choices=('auto', 'git', 'lore', 'urc'), default='auto')
    vcs_cmd.add_argument('--workspace-path')
    build = sub.add_parser('build')
    build.add_argument('file')
    build.add_argument('--source', default='carnifex')
    build.add_argument('--release-id', required=True)
    build.add_argument('--snapshot-id', required=True)
    build.add_argument('--name', required=True)
    build.add_argument('--vcs-revision')
    credits = sub.add_parser('credits')
    credits.add_argument('snapshot_id')
    credits.add_argument('destination')
    args = parser.parse_args(argv)
    client = Client(outbox=args.outbox)
    try:
        if args.command == 'tasks':
            result = client.project()['state']['tasks']
        elif args.command == 'credits':
            result = client.credits(args.snapshot_id, args.destination)
        elif args.command == 'flush':
            result = client.flush(args.force)
        else:
            if args.command == 'event':
                event_doc = json.loads(Path(args.manifest).read_text(encoding='utf-8-sig'))
            elif args.command == 'vcs-sync':
                payload = inspect_vcs(
                    args.path,
                    system=args.system,
                    slug=args.slug,
                    name=args.name,
                    workspace_path=args.workspace_path,
                )
                event_doc = {'source': payload['system'], 'type': 'vcsStatus', 'payload': payload}
            elif args.command == 'stage':
                payload = {'stage': args.stage}
                if args.by_id:
                    payload['id'] = args.target
                else:
                    payload['externalId'] = args.target
                if args.tool_uri:
                    payload['toolUri'] = args.tool_uri
                if args.vcs_revision:
                    payload['vcsRevision'] = args.vcs_revision
                if args.note:
                    payload['note'] = args.note
                event_doc = {'source': args.source, 'type': 'artifactStage', 'payload': payload}
            else:
                file = Path(args.file).resolve(strict=True)
                payload = {'uri': file.as_uri(), 'checksum': sha256(file)}
                if args.command == 'artifact':
                    payload.update(artifactId=args.artifact_id, name=file.name)
                    payload['version'] = args.version or payload['checksum']
                    if args.preview:
                        payload['previewUri'] = Path(args.preview).resolve(strict=True).as_uri()
                    if args.stage:
                        payload['stage'] = args.stage
                    if args.tool_uri:
                        payload['toolUri'] = args.tool_uri
                    if args.vcs_revision:
                        payload['vcsRevision'] = args.vcs_revision
                    if args.vcs_branch:
                        payload['vcsBranch'] = args.vcs_branch
                else:
                    payload.update(releaseId=args.release_id, snapshotId=args.snapshot_id, name=args.name)
                    if args.vcs_revision:
                        payload['vcsRevision'] = args.vcs_revision
                event_doc = {'source': args.source, 'type': 'artifactVersion' if args.command == 'artifact' else 'build', 'payload': payload}
            queued = client.queue(event_doc)
            result = client.flush()
            result['queuedFile'] = str(queued) if queued.exists() else None
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 2 if isinstance(result, dict) and result.get('pending') else 0
    except (StudioError, OSError, ValueError, KeyError):
        print('Studio operation failed. Check configuration, access, manifest, and durable outbox.', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
