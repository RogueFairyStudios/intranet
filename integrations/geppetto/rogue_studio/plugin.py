"""Krita/Geppetto export adapter using the shared Studio CLI."""
import os
from pathlib import Path
from krita import Extension, Krita, InfoObject
try:
    from PyQt6.QtCore import QProcess
    from PyQt6.QtWidgets import QMessageBox
except ImportError:
    from PyQt5.QtCore import QProcess
    from PyQt5.QtWidgets import QMessageBox


class StudioExtension(Extension):
    def setup(self):
        self.process = None

    def createActions(self, window):
        action = window.createAction('rogue_studio_export', 'Publish artwork to Studio', 'tools/scripts')
        action.triggered.connect(self.publish)

    def publish(self):
        document = Krita.instance().activeDocument()
        if not document or not document.fileName() or document.modified():
            QMessageBox.information(None, 'Studio', 'Save the document before publishing this version.')
            return
        script = os.environ.get('STUDIO_CLIENT_PATH', '')
        artifact_id = os.environ.get('STUDIO_ARTIFACT_ID', '')
        if not Path(script).is_file() or not artifact_id:
            QMessageBox.warning(None, 'Studio', 'Set STUDIO_CLIENT_PATH and STUDIO_ARTIFACT_ID before starting the editor.')
            return
        if self.process is not None and self.process.state() != QProcess.ProcessState.NotRunning:
            return
        source = Path(document.fileName())
        preview = source.with_name(source.stem + '.studio-preview.png')
        if not document.exportImage(str(preview), InfoObject()):
            QMessageBox.warning(None, 'Studio', 'Preview export failed.')
            return
        self.process = QProcess()
        self.process.setProgram(os.environ.get('STUDIO_PYTHON', 'python'))
        self.process.setArguments([script, 'artifact', str(source), '--artifact-id', artifact_id, '--source', 'geppetto', '--preview', str(preview)])
        self.process.finished.connect(self.finished)
        self.process.start()

    def finished(self, code, status):
        text = 'Artwork published.' if code == 0 else 'Export is pending or failed. Check the Studio outbox and retry with the CLI.'
        QMessageBox.information(None, 'Studio', text)
