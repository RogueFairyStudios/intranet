"""Krita / Geppetto Studio Workspace & Lore 2D Art integration plugin."""
import json
import os
from pathlib import Path
from krita import Extension, Krita, InfoObject

try:
    from PyQt6.QtCore import QProcess, Qt
    from PyQt6.QtWidgets import (
        QDialog,
        QFileDialog,
        QHBoxLayout,
        QLabel,
        QListWidget,
        QListWidgetItem,
        QMessageBox,
        QPushButton,
        QSplitter,
        QTextBrowser,
        QVBoxLayout,
        QWidget,
    )
    USER_ROLE = Qt.ItemDataRole.UserRole
except ImportError:
    from PyQt5.QtCore import QProcess, Qt
    from PyQt5.QtWidgets import (
        QDialog,
        QFileDialog,
        QHBoxLayout,
        QLabel,
        QListWidget,
        QListWidgetItem,
        QMessageBox,
        QPushButton,
        QSplitter,
        QTextBrowser,
        QVBoxLayout,
        QWidget,
    )
    USER_ROLE = Qt.UserRole


def resolve_cli_script():
    configured = os.environ.get('STUDIO_CLIENT_PATH', '').strip()
    if configured and Path(configured).is_file():
        return str(Path(configured))
    bundled = Path(__file__).resolve().parents[2] / 'cli' / 'studio_client.py'
    if bundled.is_file():
        return str(bundled)
    return ''


def format_workspace_details(ws):
    lines = [
        f"<h3>{ws.get('projectName', 'Project')} <small>({ws.get('phase', '')} · {ws.get('role', '')})</small></h3>",
    ]
    lore = ws.get('lore') or {}
    if lore.get('projectSlug'):
        actions = ', '.join(lore.get('actions') or []) or 'none'
        lines.append(
            f"<p><b>Lore Namespace:</b> <code>{lore.get('projectSlug')}/*</code> "
            f"({lore.get('visibility', 'internal')}) — actions: <code>{actions}</code></p>"
        )

    repos = ws.get('repositories') or []
    lines.append('<h4>Lore Art Repositories</h4>')
    if not repos:
        lines.append('<p><i>No Lore repositories registered for this project.</i></p>')
    else:
        lines.append('<ul>')
        for r in repos:
            badge = 'writable' if r.get('canWrite') else 'read-only'
            locks = r.get('locks') or []
            lock_note = f" — {len(locks)} active lock(s)" if locks else ''
            lines.append(
                f"<li><b>{r.get('name')}</b>: <code>{r.get('remoteUri')}</code> "
                f"[{badge}{lock_note}]<br/>"
                f"<small>Clone: <code>{' '.join(r.get('cloneCommand') or [])}</code></small></li>"
            )
        lines.append('</ul>')

    tasks = ws.get('tasks') or []
    if tasks:
        lines.append('<h4>Assigned &amp; 2D Art Tasks</h4><ul>')
        for t in tasks:
            mine = ' <b>(Assigned to you)</b>' if t.get('assignedToUser') else ''
            lines.append(f"<li>[{t.get('status')}] {t.get('title')}{mine}</li>")
        lines.append('</ul>')

    slots = ws.get('storefrontAssets') or []
    if slots:
        lines.append('<h4>Pending Storefront Graphical Capsules</h4><ul>')
        for s in slots[:12]:
            lines.append(
                f"<li><b>{s.get('targetName')} — {s.get('name')}</b>: "
                f"<code>{s.get('dimensions')}</code> ({s.get('format')})</li>"
            )
        lines.append('</ul>')

    return '\n'.join(lines)


class StudioWorkspaceDialog(QDialog):
    """Displays projects, Lore art repositories, locks, and 2D art artifacts assigned to the artist."""

    def __init__(self, extension, parent=None):
        super().__init__(parent)
        self.extension = extension
        self.workspaces = []
        self.query_proc = None
        self.setWindowTitle('Rogue Fairy Studio — Geppetto Projects & Lore Art')
        self.resize(860, 540)

        root = QVBoxLayout(self)
        header = QHBoxLayout()
        self.status_label = QLabel('Querying Studio Server (/v1/me/workspaces?client=geppetto&vcs=lore)...')
        studio_cfg_btn = QPushButton('Studio Config (YAML/JSON)...')
        studio_cfg_btn.clicked.connect(lambda: self.export_geppetto_config(project_scope=False))
        refresh_btn = QPushButton('Refresh')
        refresh_btn.clicked.connect(self.reload_workspaces)
        login_btn = QPushButton('Sign In (Authentik)...')
        login_btn.clicked.connect(self.start_device_login)
        header.addWidget(self.status_label, 1)
        header.addWidget(studio_cfg_btn)
        header.addWidget(login_btn)
        header.addWidget(refresh_btn)
        root.addLayout(header)

        splitter = QSplitter(self)
        left = QWidget()
        left_layout = QVBoxLayout(left)
        left_layout.setContentsMargins(0, 0, 0, 0)
        left_layout.addWidget(QLabel('Projects (Lore Art):'))
        self.project_list = QListWidget()
        self.project_list.currentRowChanged.connect(self._on_project_selected)
        left_layout.addWidget(self.project_list, 1)

        left_layout.addWidget(QLabel('2D Art Artifacts (.kra / Capsules):'))
        self.artifact_list = QListWidget()
        self.artifact_list.currentItemChanged.connect(self._on_artifact_selected)
        left_layout.addWidget(self.artifact_list, 1)
        splitter.addWidget(left)

        self.details = QTextBrowser()
        self.details.setOpenExternalLinks(True)
        splitter.addWidget(self.details)
        splitter.setSizes([320, 540])
        root.addWidget(splitter, 1)

        footer = QHBoxLayout()
        self.target_label = QLabel(self.extension.active_target_summary())
        proj_cfg_btn = QPushButton('Export Project Config...')
        proj_cfg_btn.clicked.connect(lambda: self.export_geppetto_config(project_scope=True))
        bind_btn = QPushButton('Use Selected Artifact')
        bind_btn.clicked.connect(self.bind_selected_target)
        publish_btn = QPushButton('Publish Active Document Now')
        publish_btn.clicked.connect(self.bind_and_publish)
        close_btn = QPushButton('Close')
        close_btn.clicked.connect(self.accept)
        footer.addWidget(self.target_label, 1)
        footer.addWidget(proj_cfg_btn)
        footer.addWidget(bind_btn)
        footer.addWidget(publish_btn)
        footer.addWidget(close_btn)
        root.addLayout(footer)

        self.reload_workspaces()

    def reload_workspaces(self):
        script = resolve_cli_script()
        if not script:
            self.status_label.setText('STUDIO_CLIENT_PATH not found.')
            return
        if self.query_proc is not None and self.query_proc.state() != QProcess.ProcessState.NotRunning:
            return
        self.status_label.setText('Loading Geppetto workspaces from Studio Server...')
        self.query_proc = QProcess(self)
        self.query_proc.setProgram(os.environ.get('STUDIO_PYTHON', 'python'))
        self.query_proc.setArguments([script, 'my-work', '--client', 'geppetto', '--vcs', 'lore'])
        self.query_proc.finished.connect(self._on_workspaces_loaded)
        self.query_proc.start()

    def _on_workspaces_loaded(self, code, _status):
        raw_out = bytes(self.query_proc.readAllStandardOutput()).decode('utf-8', errors='replace')
        if code != 0:
            self.status_label.setText('Failed to load Studio workspaces. Sign in with Authentik or check STUDIO_API_URL.')
            return
        try:
            doc = json.loads(raw_out)
        except ValueError:
            self.status_label.setText('Invalid JSON received from Studio CLI.')
            return

        user = doc.get('user') or {}
        who = user.get('name') or user.get('email') or user.get('sub') or 'artist'
        self.workspaces = doc.get('workspaces') or []
        self.status_label.setText(f'Signed in as {who} — {len(self.workspaces)} Lore art workspace(s).')

        self.project_list.clear()
        self.artifact_list.clear()
        for ws in self.workspaces:
            badge = '★ ' if ws.get('hasAssignedWork') else ''
            item = QListWidgetItem(f"{badge}{ws.get('projectName')} ({ws.get('role')})")
            item.setData(USER_ROLE, ws)
            self.project_list.addItem(item)
        if self.workspaces:
            self.project_list.setCurrentRow(0)
        else:
            self.details.setHtml('<p>No Lore art workspaces found for your account.</p>')

    def _on_project_selected(self, row):
        self.artifact_list.clear()
        if row < 0 or row >= len(self.workspaces):
            return
        ws = self.workspaces[row]
        self.details.setHtml(format_workspace_details(ws))
        for art in ws.get('artifacts') or []:
            lock = f" [locked by {art.get('lockedBy')}]" if art.get('lockedBy') else ''
            label = f"{art.get('name')} ({art.get('stage')}){lock}"
            item = QListWidgetItem(label)
            item.setData(USER_ROLE, {'projectId': ws.get('projectId'), 'projectName': ws.get('projectName'), 'artifact': art})
            self.artifact_list.addItem(item)
        if self.artifact_list.count() > 0:
            self.artifact_list.setCurrentRow(0)

    def _on_artifact_selected(self, current, _previous):
        if current is None:
            return
        data = current.data(USER_ROLE) or {}
        art = data.get('artifact') or {}
        if art.get('assetPath'):
            self.status_label.setText(f"Lore path: {art.get('loreFullName', '')} -> {art.get('assetPath')}")

    def bind_selected_target(self):
        current = self.artifact_list.currentItem()
        if current is None:
            QMessageBox.information(self, 'Studio', 'Select a 2D Art artifact from the list first.')
            return False
        data = current.data(USER_ROLE) or {}
        art = data.get('artifact') or {}
        self.extension.selected_project_id = str(data.get('projectId') or '')
        self.extension.selected_artifact_id = str(art.get('id') or '')
        self.extension.selected_label = f"{data.get('projectName')} / {art.get('name')}"
        self.target_label.setText(self.extension.active_target_summary())
        return True

    def bind_and_publish(self):
        if self.bind_selected_target():
            self.extension.publish()

    def export_geppetto_config(self, project_scope=True):
        script = resolve_cli_script()
        if not script:
            QMessageBox.warning(self, 'Studio', 'STUDIO_CLIENT_PATH not found.')
            return
        project_ref = ''
        default_name = 'roguefairy-geppetto.yaml'
        if project_scope:
            row = self.project_list.currentRow()
            if row < 0 or row >= len(self.workspaces):
                QMessageBox.information(self, 'Studio', 'Select a project first.')
                return
            ws = self.workspaces[row]
            lore = ws.get('lore') or {}
            project_ref = str(ws.get('projectSlug') or lore.get('projectSlug') or ws.get('projectId') or '')
            slug_label = str(ws.get('projectSlug') or lore.get('projectSlug') or 'project')
            default_name = f'{slug_label}-geppetto.yaml'

        default_path = str(Path.home() / default_name)
        save_path, _ = QFileDialog.getSaveFileName(
            self,
            'Save Geppetto Configuration Manifest',
            default_path,
            'YAML Manifest (*.yaml *.yml);;JSON Manifest (*.json)',
        )
        if not save_path:
            return
        fmt = 'json' if save_path.lower().endswith('.json') else 'yaml'
        argv = [script, 'config', 'geppetto', '--format', fmt, '--client', 'geppetto', '-o', save_path]
        if project_scope and project_ref:
            argv.extend(['--project', project_ref])
        else:
            argv.extend(['--scope', 'studio'])

        proc = QProcess(self)
        proc.setProgram(os.environ.get('STUDIO_PYTHON', 'python'))
        proc.setArguments(argv)
        proc.finished.connect(
            lambda code, _st: QMessageBox.information(
                self,
                'Studio Configuration',
                f'Saved Geppetto configuration to:\n{save_path}'
                if code == 0
                else 'Failed to download Geppetto configuration. Verify Authentik session and STUDIO_API_URL.',
            )
        )
        proc.start()

    def start_device_login(self):
        script = resolve_cli_script()
        if not script:
            return
        proc = QProcess(self)
        proc.setProgram(os.environ.get('STUDIO_PYTHON', 'python'))
        proc.setArguments([script, 'login', '--app', 'geppetto'])
        proc.readyReadStandardError.connect(
            lambda: QMessageBox.information(
                self,
                'Tellurian Authentik Login',
                bytes(proc.readAllStandardError()).decode('utf-8', errors='replace'),
            )
        )
        proc.finished.connect(lambda code, _st: self.reload_workspaces() if code == 0 else None)
        proc.start()


class StudioExtension(Extension):
    def setup(self):
        self.process = None
        self.selected_project_id = os.environ.get('STUDIO_PROJECT_ID', '')
        self.selected_artifact_id = os.environ.get('STUDIO_ARTIFACT_ID', '')
        self.selected_label = ''

    def active_target_summary(self):
        if self.selected_label:
            return f'Active target: {self.selected_label}'
        if self.selected_artifact_id:
            return f'Active artifact: {self.selected_artifact_id}'
        return 'No active Studio artifact selected'

    def createActions(self, window):
        ws_action = window.createAction(
            'rogue_studio_workspace',
            'Studio Projects && Lore Art...',
            'tools/scripts',
        )
        ws_action.triggered.connect(self.open_workspace)

        export_action = window.createAction(
            'rogue_studio_export',
            'Publish artwork to Studio',
            'tools/scripts',
        )
        export_action.triggered.connect(self.publish)

    def open_workspace(self):
        dlg = StudioWorkspaceDialog(self)
        dlg.exec()

    def publish(self):
        document = Krita.instance().activeDocument()
        if not document or not document.fileName() or document.modified():
            QMessageBox.information(None, 'Studio', 'Save the document before publishing this version.')
            return
        script = resolve_cli_script()
        artifact_id = self.selected_artifact_id or os.environ.get('STUDIO_ARTIFACT_ID', '')
        project_id = self.selected_project_id or os.environ.get('STUDIO_PROJECT_ID', '')
        if not script or not artifact_id:
            QMessageBox.warning(
                None,
                'Studio',
                'Select a target artifact in "Studio Projects & Lore Art..." (or set STUDIO_CLIENT_PATH and STUDIO_ARTIFACT_ID).',
            )
            return
        if self.process is not None and self.process.state() != QProcess.ProcessState.NotRunning:
            return
        source = Path(document.fileName())
        preview = source.with_name(source.stem + '.studio-preview.png')
        if not document.exportImage(str(preview), InfoObject()):
            QMessageBox.warning(None, 'Studio', 'Preview export failed.')
            return
        argv = [script]
        if project_id:
            argv.extend(['--project-id', project_id])
        argv.extend([
            'artifact',
            str(source),
            '--artifact-id',
            artifact_id,
            '--source',
            'geppetto',
            '--preview',
            str(preview),
        ])
        self.process = QProcess()
        self.process.setProgram(os.environ.get('STUDIO_PYTHON', 'python'))
        self.process.setArguments(argv)
        self.process.finished.connect(self.finished)
        self.process.start()

    def finished(self, code, status):
        text = 'Artwork published.' if code == 0 else 'Export is pending or failed. Check the Studio outbox and retry with the CLI.'
        QMessageBox.information(None, 'Studio', text)
