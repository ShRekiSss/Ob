#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Exam Board: табло демоэкзамена — кто подключился, во сколько, «ПОБЕДИТЕЛЬ»."""
import json, os, re, subprocess, threading, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, quote

STATE_DIR = "/var/lib/board"
WINNER_FILE = os.path.join(STATE_DIR, "winner.json")
HTML_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "index.html")
LOCK = threading.Lock()
CACHE = {"connections": [], "updated": 0}


def read_winner():
    try:
        with open(WINNER_FILE, encoding="utf-8") as f:
            return json.load(f)
    except Exception:
        return None


def parse_last():
    """Разбирает вывод `last -F -n 300`: пользователь, tty, IP, время входа."""
    rows = []
    try:
        out = subprocess.run(["last", "-F", "-n", "300"], capture_output=True,
                             text=True, timeout=5).stdout
    except Exception:
        return rows
    for line in out.splitlines():
        if not line.strip() or "reboot" in line or "wtmp begins" in line:
            continue
        m = re.match(r"^(\S+)\s+(\S+)\s+(\S+)\s+(\w{3} \w{3}\s+\d+\s[\d:]+ \d{4})", line)
        if m and m.group(2) != "logged":
            rows.append({"user": m.group(1), "tty": m.group(2),
                         "ip": m.group(3), "time": m.group(4)})
    return rows[:50]


def refresh_loop():
    while True:
        rows = parse_last()
        with LOCK:
            CACHE["connections"] = rows
            CACHE["updated"] = time.time()
        time.sleep(3)


def api_state():
    with LOCK:
        conns = CACHE["connections"]
    return json.dumps({"now": time.strftime("%Y-%m-%d %H:%M:%S"),
                       "connections": conns,
                       "winner": read_winner()}, ensure_ascii=False)


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def _send(self, code, body, ctype):
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        base = os.path.dirname(os.path.abspath(__file__))
        static = {"/style.css": ("style.css", "text/css; charset=utf-8"),
                  "/app.js": ("app.js", "application/javascript; charset=utf-8")}
        if self.path == "/api/state":
            self._send(200, api_state().encode("utf-8"), "application/json")
        elif self.path in ("/", "/index.html"):
            with open(HTML_FILE, "rb") as f:
                self._send(200, f.read(), "text/html; charset=utf-8")
        elif self.path in static:
            fname, ctype = static[self.path]
            with open(os.path.join(base, fname), "rb") as f:
                self._send(200, f.read(), ctype)
        else:
            self._send(404, b"not found", "text/plain")

    def do_POST(self):
        ln = int(self.headers.get("Content-Length", 0) or 0)
        form = parse_qs(self.rfile.read(ln).decode("utf-8"))
        if self.path == "/api/winner":
            name = (form.get("name", [""])[0] or "Студент")[:40]
            data = {"name": name, "time": time.strftime("%Y-%m-%d %H:%M:%S")}
            with open(WINNER_FILE, "w", encoding="utf-8") as f:
                json.dump(data, f, ensure_ascii=False)
            self._send(200, json.dumps(data, ensure_ascii=False).encode("utf-8"),
                       "application/json")
        elif self.path == "/api/reset":
            try:
                os.remove(WINNER_FILE)
            except FileNotFoundError:
                pass
            self._send(200, b'{"ok": true}', "application/json")
        else:
            self._send(404, b"not found", "text/plain")


if __name__ == "__main__":
    os.makedirs(STATE_DIR, exist_ok=True)
    threading.Thread(target=refresh_loop, daemon=True).start()
    srv = ThreadingHTTPServer(("0.0.0.0", 8080), Handler)
    print("Exam Board on :8080", flush=True)
    srv.serve_forever()
