const $ = (id) => document.getElementById(id);
const fmtTime = (s) => s.slice(11, 19) || s;

function tick() {
  const d = new Date();
  $("clock").textContent = d.toTimeString().slice(0, 8);
}

function confetti() {
  const glyphs = ["🎉", "⭐", "🏆", "✨", "🔥", "⚡"];
  for (let i = 0; i < 40; i++) {
    const e = document.createElement("div");
    e.className = "fetti";
    e.textContent = glyphs[i % glyphs.length];
    e.style.left = Math.random() * 100 + "vw";
    e.style.animationDuration = (2 + Math.random() * 2.5) + "s";
    e.style.animationDelay = (Math.random() * 0.8) + "s";
    document.body.appendChild(e);
    setTimeout(() => e.remove(), 5500);
  }
}

async function refresh() {
  try {
    const st = await (await fetch("/api/state")).json();
    const rows = $("rows");
    if (!st.connections.length) {
      rows.innerHTML = '<tr><td colspan="4" class="muted">пока тишина…</td></tr>';
    } else {
      rows.innerHTML = st.connections.map((c) =>
        `<tr${c.user.startsWith("student") ? ' class="hot"' : ""}>` +
        `<td>${c.user}</td><td>${c.tty}</td><td>${c.ip}</td><td>${c.time}</td></tr>`
      ).join("");
    }
    const users = new Set(st.connections.map((c) => c.user));
    $("total").textContent = st.connections.length;
    $("unique").textContent = users.size;
    const w = st.winner;
    const box = $("winner");
    if (w && box.style.display !== "block") {
      $("whoName").textContent = "🎓 " + w.name;
      $("whoTime").textContent = "зафиксировано: " + w.time;
      box.style.display = "block";
      confetti();
    } else if (!w) {
      box.style.display = "none";
    }
  } catch (e) { /* сервер ещё поднимается */ }
}

$("btnWin").onclick = async () => {
  const name = prompt("Имя победителя:", "Зеленюк Д.Д.") || "Студент";
  await fetch("/api/winner", {
    method: "POST",
    headers: {"Content-Type": "application/x-www-form-urlencoded"},
    body: "name=" + encodeURIComponent(name),
  });
  refresh();
};

$("btnReset").onclick = async () => {
  await fetch("/api/reset", {method: "POST"});
  refresh();
};

tick();
setInterval(tick, 1000);
refresh();
setInterval(refresh, 2000);
