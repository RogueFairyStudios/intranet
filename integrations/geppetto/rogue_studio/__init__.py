from krita import Krita
from .plugin import StudioExtension

Krita.instance().addExtension(StudioExtension(Krita.instance()))
