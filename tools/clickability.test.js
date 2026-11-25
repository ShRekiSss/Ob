// Тест кликабельности MINI-DOM: проверяем обработчики кнопки сброса,
// формы ввода и истории стрелками без настоящего браузера.
const fs = require('fs');
const path = require('path');
const html = fs.readFileSync(path.join(__dirname, '..', 'index.html'), 'utf8');
let src = html.split('<script>')[1].split('</script>')[0];
src = src.replace('hintEl.textContent = "Не удалось загрузить состояние. Перезагрузи страницу.";',
                  'hintEl.textContent = "ERR:" + err.message;');

const handlers = {};
const calls = [];
function el(id) {
  return {
    id, textContent: '', value: '', hidden: false, disabled: false,
    className: '', style: {}, children: [], scrollTop: 0, scrollHeight: 100,
    classList: { toggle() {}, add() {}, remove() {} },
    listeners: {},
    addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); handlers[id + ':' + type] = fn; },
    setAttribute() {}, appendChild(c) { this.children.push(c); }, replaceChildren() { this.children = []; },
    focus() {}, setSelectionRange() {},
    getContext() { return { clearRect() {}, fillRect() {}, beginPath() {}, moveTo() {}, lineTo() {}, stroke() {}, fill() {}, arc() {}, save() {}, restore() {}, fillText() {} }; },
    get firstElementChild() { return el(id + '-child'); },
  };
}
const elements = {};
const ids = ['terminal','form','input','reset','prompt','hp-text','hp-track','hint','hint-box',
  'channel-label','arcade-btn','arcade-overlay','arcade-close','arcade-restart','arcade-kills',
  'arcade-lives','arcade-ammo','arcade-reload-bar','arcade-reload','arcade-canvas',
  'victory-overlay','victory-name','victory-detail','victory-restart',
  'defeat-overlay','defeat-name','defeat-detail','defeat-retry','defeat-reset','defeat-close',
  'linux-panel','linux-title','linux-brief','linux-goal','linux-progress','linux-score',
  'scare-overlay','scare-img','scare-close'];
for (const id of ids) elements[id] = el(id);

const document = {
  title: '',
  getElementById: id => elements[id] || el(id),
  createElement: tag => el(tag),
};
const window = {
  location: { protocol: 'http:' },
  matchMedia: () => ({ matches: false }),
  addEventListener(type, fn) { handlers['window:' + type] = fn; },
};

const state = { mission: 1, total: 13, title: '1. Брифинг отдела ИБ',
  brief: '...', goal: 'Разберись, что находится в каталоге /station.',
  hint: '...', done: 0, step_done: 0, step_total: 1, score: 0, cwd: '/home/analyst', completed: false };
const fetch = async (url, options = {}) => {
  const method = options.method || 'GET';
  calls.push({ url, method, body: options.body });
  let data = state;
  if (url.includes('/command')) {
    const cmd = JSON.parse(options.body).command;
    const scare = cmd.includes('secrets.txt');
    data = { output: scare ? 'Совершенно секретно...' : 'brief.txt\nevents.log',
      state: { ...state, done: 1, step_done: 1, score: 100 }, scare };
  }
  return { ok: true, status: 200, json: async () => data, body: { getReader() {} } };
};

const timers = { set: [], clear: [] };
new Function('document', 'window', 'fetch', 'setTimeout', 'clearTimeout', 'setInterval',
  'clearInterval', 'AbortController', src)(
  document, window, fetch,
  (fn, ms) => { timers.set.push({ fn, ms }); return timers.set.length; },
  id => { timers.clear.push(id); },
  () => 1, () => {}, AbortController
);

(async () => {
  await new Promise(r => setTimeout(r, 10));

  if (!calls.some(c => c.url === '/api/linux/state')) throw new Error('не запрошено состояние тренажёра');
  if (elements['linux-panel'].hidden) throw new Error('панель заданий должна быть видна');
  if (!elements['linux-title'].textContent.includes('Брифинг')) {
    throw new Error('не отрисовано задание: title=' + JSON.stringify(elements['linux-title'].textContent) + ' hint=' + JSON.stringify(elements['hint'].textContent));
  }

  if (!handlers['form:submit']) throw new Error('нет обработчика формы ввода');
  elements['input'].value = 'ls /station';
  await handlers['form:submit']({ preventDefault() {} });
  await new Promise(r => setTimeout(r, 5));
  const cmdCall = calls.find(c => c.url === '/api/linux/command');
  if (!cmdCall || JSON.parse(cmdCall.body).command !== 'ls /station') throw new Error('команда не отправлена');
  if (!elements['terminal'].textContent.includes('brief.txt')) throw new Error('вывод терминала пуст');

  if (!handlers['input:keydown']) throw new Error('нет обработчика истории команд');
  elements['input'].value = '';
  handlers['input:keydown']({ key: 'ArrowUp', preventDefault() {} });
  if (elements['input'].value !== 'ls /station') throw new Error('стрелка вверх не восстановила команду');

  if (!handlers['reset:click']) throw new Error('нет обработчика кнопки «Начать заново»');
  await handlers['reset:click']();
  await new Promise(r => setTimeout(r, 5));
  if (!calls.some(c => c.url === '/api/linux/reset')) throw new Error('кнопка сброса не вызвала API');
  if (!elements['terminal'].textContent.includes('миссия начата заново')) throw new Error('нет сообщения о сбросе');

  elements['input'].value = 'cat /station/secrets.txt';
  await handlers['form:submit']({ preventDefault() {} });
  await new Promise(r => setTimeout(r, 5));
  if (elements['scare-overlay'].hidden) throw new Error('скример не показался');
  if (!elements['scare-img'].src.startsWith('data:image/gif;base64,')) throw new Error('GIF не встроен');
  if (!handlers['scare-close:click']) throw new Error('нет кнопки закрытия скримера');
  handlers['scare-close:click']();
  if (!elements['scare-overlay'].hidden) throw new Error('скример не закрылся');

  if (!handlers['arcade-btn:click']) throw new Error('нет кнопки аркады');
  handlers['arcade-btn:click']();
  if (elements['arcade-overlay'].hidden) throw new Error('аркада не открылась');
  if (!handlers['arcade-close:click']) throw new Error('нет кнопки закрытия аркады');
  if (!handlers['arcade-restart:click']) throw new Error('нет кнопки перезапуска аркады');
  handlers['arcade-close:click']();
  if (!elements['arcade-overlay'].hidden) throw new Error('аркада не закрылась');

  console.log('OK UI clickability: form / history / reset / scare / arcade');
})().catch(e => { console.error('FAIL UI:', e.message); process.exit(1); });
