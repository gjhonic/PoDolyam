"""Изолированный PostgreSQL для разработки/тестов; системный кластер не меняется."""
from pathlib import Path
import os
import secrets
import socket
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
CACHE = ROOT / ".cache"
CACHE.mkdir(exist_ok=True)
BIN = Path(os.environ.get("PG_BIN", "/usr/lib/postgresql/16/bin"))
def command(name, *args, **kwargs):
    return subprocess.run([str(BIN / name), *map(str, args)], check=True, **kwargs)

def start(folder, port):
    folder.mkdir(mode=0o700, parents=True, exist_ok=True)
    os.chmod(folder, 0o700)
    password_file = folder / "password"
    if not password_file.exists():
        password_file.write_text(secrets.token_hex(24))
        password_file.chmod(0o600)
    data = folder / "data"
    if not (data / "PG_VERSION").exists():
        command("initdb", "-D", data, "-U", "podolyam", "--encoding=UTF8", "--locale=C.UTF-8",
                "--auth-local=trust", "--auth-host=scram-sha-256", "--pwfile", password_file,
                stdout=subprocess.DEVNULL)
    if not (data / "postmaster.pid").exists():
        # Unix-сокет находится в каталоге 0700; TCP принимает только loopback.
        command("pg_ctl", "-D", data, "-l", folder / "postgres.log",
                "-o", f"-h 127.0.0.1 -p {port} -k {folder}", "-w", "start",
                stdout=subprocess.DEVNULL)
    password = password_file.read_text().strip()
    return f"postgresql://podolyam:{password}@127.0.0.1:{port}/postgres?sslmode=disable"

def stop(folder):
    data = folder / "data"
    if (data / "postmaster.pid").exists():
        command("pg_ctl", "-D", data, "-m", "fast", "-w", "stop", stdout=subprocess.DEVNULL)

def main():
    action = sys.argv[1]
    if action == "start":
        url = start(CACHE / "postgres", 55432)
        file = CACHE / "database-url"
        file.write_text(url)
        file.chmod(0o600)
        print("Локальная БД запущена на 127.0.0.1:55432; URL в .cache/database-url")
    elif action == "stop":
        stop(CACHE / "postgres")
    elif action == "test":
        env = os.environ.copy()
        if env.get("TEST_DATABASE_URL"):
            result = subprocess.run(["go", "test", "-tags=integration", "-race", "-count=1", "./internal/integration"], cwd=ROOT / "backend", env=env)
            raise SystemExit(result.returncode)
        # Временный кластер принадлежит только этому запуску и удаляется после остановки.
        with tempfile.TemporaryDirectory(prefix="pd-test-", dir=CACHE) as temp:
            folder = Path(temp)
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", 0))
                port = listener.getsockname()[1]
            try:
                env["TEST_DATABASE_URL"] = start(folder, port)
                result = subprocess.run(["go", "test", "-tags=integration", "-race", "-count=1", "./internal/integration"], cwd=ROOT / "backend", env=env)
            finally:
                stop(folder)
            raise SystemExit(result.returncode)
    else:
        raise SystemExit("Допустимые команды: start, stop, test")

if __name__ == "__main__":
    main()
