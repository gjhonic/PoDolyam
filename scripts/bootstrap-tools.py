"""Локальные Go/Node для Linux amd64, без sudo и изменения профиля shell."""
from pathlib import Path
import hashlib
import platform
import tarfile
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
DEST = ROOT / ".tools"
ARCHIVES = [
    ("https://go.dev/dl/go1.27.1.linux-amd64.tar.gz",
     "63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445",
     "go/bin/go"),
    ("https://nodejs.org/dist/v24.21.0/node-v24.21.0-linux-x64.tar.xz",
     "fd8e59d5a511510f6a298afb548f18c7d2b1be404d8b4a27d94fbe49f56cb2d6",
     "node-v24.21.0-linux-x64/bin/node"),
]

if platform.system() != "Linux" or platform.machine() != "x86_64":
    raise SystemExit("Автоустановка рассчитана на Linux amd64 (включая WSL). См. docs/DEVELOPMENT.md.")
DEST.mkdir(exist_ok=True)
for url, expected, binary in ARCHIVES:
    if (DEST / binary).exists():
        print("Уже установлен:", binary, flush=True)
        continue
    archive = DEST / url.rsplit("/", 1)[-1]
    print("Загрузка:", url, flush=True)
    with urllib.request.urlopen(url, timeout=120) as response, archive.open("wb") as output:
        while chunk := response.read(1024 * 1024):
            output.write(chunk)
    actual = hashlib.sha256(archive.read_bytes()).hexdigest()
    if actual != expected:
        raise SystemExit("SHA256 не совпадает: " + archive.name)
    with tarfile.open(archive) as source:
        source.extractall(DEST, filter="data")
    archive.unlink()
    print("Установлен:", binary, flush=True)
