"""Locate installed build tools, with an external per-user cache fallback."""
from pathlib import Path
import os
import shutil
import sys


def find_tool(name, *relative_paths):
    installed = shutil.which(name)
    if installed:
        return installed
    default_local = Path.home() / ('Library/Caches' if sys.platform == 'darwin' else '.cache')
    local = Path(os.environ.get('LOCALAPPDATA', default_local))
    caches = [Path(os.environ['PAIRSHELL_BUILD_TOOLS'])] if os.environ.get('PAIRSHELL_BUILD_TOOLS') else []
    caches.append(local / 'PairShell' / 'BuildTools')
    for cache in caches:
        for relative in relative_paths:
            candidate = cache / relative
            if candidate.is_file():
                return str(candidate)
    raise FileNotFoundError(f"Required build tool {name!r} was not found on PATH or in the PairShell build tools cache")
