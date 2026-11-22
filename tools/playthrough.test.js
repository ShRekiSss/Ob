const fs = require('fs');
const path = require('path');
const html = fs.readFileSync(path.join(__dirname, '..', 'index.html'), 'utf8');
const src = html.split('<script>')[1].split('</script>')[0];
const start = src.indexOf('// ── Тренажёр Linux');
const end = src.indexOf('  function showScare', start);
const code = src.slice(start, end) + '\nglobalThis.createLinuxGame = createLinuxGame;';
eval(code);

const g = globalThis.createLinuxGame();
const cmds = [
  ['ls /station'],
  ['cat /station/brief.txt'],
  ['grep READY /station/events.log'],
  ['whoami','id'],
  ['ls -la /home/analyst','cat /home/analyst/.notes'],
  ['find /var/log -name "*.log"'],
  ['grep -i "failed password" /var/log/auth.log'],
  ['chmod 640 /srv/incident/config.yml'],
  ['ps aux','kill 1337'],
  ['ss -tulpn','iptables -A INPUT -s 203.0.113.45 -j DROP'],
  ['cat /etc/ssh/sshd_config','sed -i "s/PermitRootLogin yes/PermitRootLogin no/" /etc/ssh/sshd_config','systemctl restart sshd'],
  ['sha256sum /var/log/auth.log'],
  ['tar -czf /srv/incident/evidence.tgz /var/log','systemctl restart sshd']
];
const secret = g.execute('cat /station/secrets.txt');
if (!secret.scare) throw new Error('секретный файл не включил скример');
if (!secret.output.includes('Совершенно секретно')) throw new Error('секретный файл не прочитался');

cmds.forEach((group, i) => {
  group.forEach(c => {
    const r = g.execute(c);
    if (r.output.includes('команда не найдена')) throw new Error('bad cmd: ' + c + ' -> ' + r.output);
    if (i === 3 && c.startsWith('grep')) console.log('GREP OUT:', r.output.slice(0, 120));
  });
  const s = g.state();
  if (s.done !== (s.completed ? s.total : i + 1)) throw new Error('mission ' + (i+1) + ' failed: ' + JSON.stringify(s));
});
const s = g.state();
if (!s.completed || s.score !== s.total * 100) throw new Error('final: ' + JSON.stringify(s));
console.log('OK offline linux:', JSON.stringify({done:s.done, score:s.score, completed:s.completed}));
