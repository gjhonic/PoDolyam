"""Сквозная проверка на отдельных PostgreSQL, Go и Vite; процессы убираются в finally."""
from pathlib import Path
import os
import runpy
import socket
import subprocess
import tempfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
pg = runpy.run_path(str(ROOT / "scripts/postgres.py"), run_name="pgtools")
def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]

with tempfile.TemporaryDirectory(prefix="pd-e2e-", dir=ROOT / ".cache") as temp:
    folder = Path(temp)
    processes = []
    logs = []
    env = os.environ.copy()
    try:
        env["DATABASE_URL"] = pg["start"](folder, free_port())
        backend_port, frontend_port = free_port(), free_port()
        env["HTTP_ADDR"] = f"127.0.0.1:{backend_port}"
        env["APP_ORIGIN"] = f"http://localhost:{frontend_port}"
        env["API_PROXY_TARGET"] = f"http://127.0.0.1:{backend_port}"
        env["E2E_BASE_URL"] = env["APP_ORIGIN"]
        subprocess.run(["go", "run", "./cmd/migrate"], cwd=ROOT / "backend", env=env, check=True)
        subprocess.run(["go", "build", "-o", "bin/server", "./cmd/server"], cwd=ROOT / "backend", env=env, check=True)
        commands = [
            ([str(ROOT / "backend/bin/server")], ROOT),
            (["node", "node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", str(frontend_port)], ROOT / "frontend"),
        ]
        for i, (command, cwd) in enumerate(commands):
            log = (folder / f"service-{i}.log").open("w+")
            logs.append(log)
            processes.append(subprocess.Popen(command, cwd=cwd, env=env, stdout=log, stderr=log))
        for url in [f"http://127.0.0.1:{backend_port}/readyz", f"http://127.0.0.1:{frontend_port}/"]:
            for attempt in range(100):
                if any(p.poll() is not None for p in processes):
                    raise RuntimeError("Сервис остановился до теста")
                try:
                    with urllib.request.urlopen(url, timeout=1) as r:
                        if r.status == 200:
                            break
                except OSError:
                    time.sleep(.1)
            else:
                raise RuntimeError("Сервис не запустился")
        subprocess.run(["npx", "playwright", "test"], cwd=ROOT / "frontend", env=env, check=True)
    except Exception:
        for log in logs:
            log.flush()
            log.seek(0)
            print(log.read())
        raise
    finally:
        for p in processes:
            p.terminate()
        for p in processes:
            try:
                p.wait(timeout=15)
            except subprocess.TimeoutExpired:
                p.kill()
                p.wait()
        for log in logs:
            log.close()
        pg["stop"](folder)
