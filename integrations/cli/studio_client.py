#!/usr/bin/env python3
"""Dependency-free Studio bridge. Credentials are only read from process environment."""
import argparse
import hashlib
import json
import os
import re
from pathlib import Path
import sys
import time
import uuid
import urllib.error
import urllib.parse
import urllib.request


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
        if event.get('source') not in ('geppetto', 'goethe', 'godot', 'unity', 'carnifex', 'generic', 'github') or event.get('type') not in ('artifact', 'artifactVersion', 'build', 'validation') or not isinstance(event.get('payload'), dict):
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
    build = sub.add_parser('build')
    build.add_argument('file')
    build.add_argument('--source', default='carnifex')
    build.add_argument('--release-id', required=True)
    build.add_argument('--snapshot-id', required=True)
    build.add_argument('--name', required=True)
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
                event = json.loads(Path(args.manifest).read_text(encoding='utf-8-sig'))
            else:
                file = Path(args.file).resolve(strict=True)
                payload = {'uri': file.as_uri(), 'checksum': sha256(file)}
                if args.command == 'artifact':
                    payload.update(artifactId=args.artifact_id, name=file.name)
                    payload['version'] = args.version or payload['checksum']
                    if args.preview:
                        payload['previewUri'] = Path(args.preview).resolve(strict=True).as_uri()
                else:
                    payload.update(releaseId=args.release_id, snapshotId=args.snapshot_id, name=args.name)
                event = {'source': args.source, 'type': 'artifactVersion' if args.command == 'artifact' else 'build', 'payload': payload}
            queued = client.queue(event)
            result = client.flush()
            result['queuedFile'] = str(queued) if queued.exists() else None
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 2 if isinstance(result, dict) and result.get('pending') else 0
    except (StudioError, OSError, ValueError, KeyError):
        print('Studio operation failed. Check configuration, access, manifest, and durable outbox.', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
