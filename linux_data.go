package main

// Данные тренажёра Linux для специальности 10.02.05.
// Настоящая ФС, процессы и сеть не трогаются — всё виртуальное.

type linuxFile struct {
	Dir  bool
	Data string
	Mode string
	Hash string
}

var linuxFSTemplate = map[string]linuxFile{
	"/":             {Dir: true},
	"/home":         {Dir: true},
	"/etc":          {Dir: true},
	"/etc/ssh":      {Dir: true},
	"/var":          {Dir: true},
	"/var/log":      {Dir: true},
	"/srv":          {Dir: true},
	"/srv/incident": {Dir: true},
	"/home/analyst": {Dir: true},
	"/home/analyst/.notes": {
		Data: "Analyst: sudo есть только у admin.\nКлюч-подсказка: /srv/incident/keys.txt.",
		Mode: "644",
	},
	"/home/analyst/keys.txt": {Data: "key = OBF-2026-ALPHA", Mode: "600"},
	"/srv/incident/config.yml": {
		Data: "db_host: 10.0.0.5\ndb_password: s3cr3t-do-not-share",
		Mode: "666",
	},
	"/srv/incident/hashes.sha256": {
		Data: "9f2c4a1b7e3d8065c11b0a2f4d6e8a90  /var/log/auth.log",
		Mode: "644",
	},
	"/etc/ssh/sshd_config": {
		Data: "Port 22\nPermitRootLogin yes\nPasswordAuthentication yes\nMaxAuthTries 6",
		Mode: "644",
	},
	"/var/log/syslog": {
		Data: "cron: session opened\nkernel: eth0 link up\nsyslog: rotation complete",
		Mode: "640",
	},
}

var authLog = []string{
	"Nov 21 03:12:11 ks sshd[1201]: Accepted password for analyst from 10.0.0.12 port 54321",
	"Nov 21 03:13:02 ks sshd[1204]: Failed password for root from 203.0.113.45 port 40122 ssh2",
	"Nov 21 03:13:05 ks sshd[1204]: Failed password for root from 203.0.113.45 port 40124 ssh2",
	"Nov 21 03:13:09 ks sshd[1204]: Failed password for root from 203.0.113.45 port 40127 ssh2",
	"Nov 21 03:14:01 ks sshd[1210]: Accepted password for analyst from 10.0.0.12 port 54322",
}

type linuxProcess struct {
	PID  int
	User string
	CPU  string
	Cmd  string
}

var linuxProcessTemplate = []linuxProcess{
	{PID: 1, User: "root", CPU: "0.0", Cmd: "/sbin/init"},
	{PID: 1201, User: "analyst", CPU: "0.0", Cmd: "sshd: analyst@pts/0"},
	{PID: 1337, User: "nobody", CPU: "98.7", Cmd: "./xmrig --pool 203.0.113.45:4444"},
}

type linuxSocket struct {
	Proto string
	Addr  string
	State string
	Proc  string
}

var linuxSocketTemplate = []linuxSocket{
	{Proto: "tcp", Addr: "0.0.0.0:22", State: "LISTEN", Proc: "1201/sshd"},
	{Proto: "tcp", Addr: "10.0.0.12:54321", State: "ESTAB", Proc: "1201/sshd"},
	{Proto: "tcp", Addr: "0.0.0.0:4444", State: "LISTEN", Proc: "1337/xmrig"},
}

// linuxMission: проверка — имя команды + обязательные куски аргументов.
type linuxCheck struct {
	Cmd  string
	Args []string
}

type linuxMission struct {
	Title  string
	Brief  string
	Goal   string
	Hint   string
	Checks []linuxCheck
}

var linuxMissions = []linuxMission{
	{
		Title:  "1. Кто я в системе",
		Brief:  "Ты стажёр отдела ИБ. Прежде чем что-то делать, узнай свою учётную запись и права.",
		Goal:   "Выполни whoami, затем id.",
		Hint:   "whoami покажет пользователя, id — UID, GID и группы. Запомни группу sudo.",
		Checks: []linuxCheck{{Cmd: "whoami"}, {Cmd: "id"}},
	},
	{
		Title: "2. Скрытые файлы",
		Brief: "В домашнем каталоге есть скрытая заметка. Обычный ls её не покажет.",
		Goal:  "Выполни ls -la /home/analyst, затем cat /home/analyst/.notes.",
		Hint:  "Ключ -a показывает скрытые файлы, -l — права и владельца.",
		Checks: []linuxCheck{
			{Cmd: "ls", Args: []string{"-la", "/home/analyst"}},
			{Cmd: "cat", Args: []string{"/home/analyst/.notes"}},
		},
	},
	{
		Title:  "3. Поиск логов",
		Brief:  "Нужно быстро найти все журналы под /var/log, не перебирая каталоги вручную.",
		Goal:   "Выполни find /var/log -name \"*.log\".",
		Hint:   "find ПУТЬ -name ШАБЛОН. Шаблон возьми в кавычках: \"*.log\".",
		Checks: []linuxCheck{{Cmd: "find", Args: []string{"/var/log", "-name", "*.log"}}},
	},
	{
		Title:  "4. Поиск атаки в журнале",
		Brief:  "В auth.log есть неудачные попытки входа. Регистр сообщений может отличаться.",
		Goal:   "Выполни grep -i \"failed password\" /var/log/auth.log.",
		Hint:   "Флаг -i делает поиск регистронезависимым. Запиши IP атакующего.",
		Checks: []linuxCheck{{Cmd: "grep", Args: []string{"-i", "failed password", "/var/log/auth.log"}}},
	},
	{
		Title:  "5. Опасные права",
		Brief:  "Конфиг с паролем доступен всем на запись. По принципу наименьших прав это надо исправить.",
		Goal:   "Выполни chmod 640 /srv/incident/config.yml.",
		Hint:   "chmod 640 = владелец читает/пишет, группа читает, остальные — ничего.",
		Checks: []linuxCheck{{Cmd: "chmod", Args: []string{"640", "/srv/incident/config.yml"}}},
	},
	{
		Title:  "6. Лишний процесс",
		Brief:  "В ps aux виден процесс с подозрительно высоким CPU и майнинговым пулом в аргументах.",
		Goal:   "Выполни ps aux, затем kill 1337.",
		Hint:   "Сначала найди PID в выводе ps aux, потом kill <PID>.",
		Checks: []linuxCheck{{Cmd: "ps"}, {Cmd: "kill", Args: []string{"1337"}}},
	},
	{
		Title: "7. Подозрительный порт",
		Brief: "Слушающий порт 4444 и тот же IP из логов. Закрой доступ с этого адреса.",
		Goal:  "Выполни ss -tulpn, затем iptables -A INPUT -s 203.0.113.45 -j DROP.",
		Hint:  "ss показывает слушающие сокеты и PID. iptables -A INPUT -s IP -j DROP блокирует адрес.",
		Checks: []linuxCheck{
			{Cmd: "ss", Args: []string{"-tulpn"}},
			{Cmd: "iptables", Args: []string{"-A", "INPUT", "-s", "203.0.113.45", "-j", "DROP"}},
		},
	},
	{
		Title: "8. Укрепление SSH",
		Brief: "В sshd_config включён root-логин. Отключи его и перезапусти службу.",
		Goal:  "cat /etc/ssh/sshd_config → sed -i \"s/PermitRootLogin yes/PermitRootLogin no/\" /etc/ssh/sshd_config → systemctl restart sshd.",
		Hint:  "sed -i меняет файл на месте. После правки конфига службу надо перезапустить.",
		Checks: []linuxCheck{
			{Cmd: "cat", Args: []string{"/etc/ssh/sshd_config"}},
			{Cmd: "sed", Args: []string{"-i", "PermitRootLogin yes", "PermitRootLogin no", "/etc/ssh/sshd_config"}},
			{Cmd: "systemctl", Args: []string{"restart", "sshd"}},
		},
	},
	{
		Title:  "9. Контроль целостности",
		Brief:  "Эталонная сумма лежит в hashes.sha256. Сверь журнал с эталоном.",
		Goal:   "Выполни sha256sum /var/log/auth.log.",
		Hint:   "sha256sum выводит хеш и имя файла. Сравни его с содержимым hashes.sha256.",
		Checks: []linuxCheck{{Cmd: "sha256sum", Args: []string{"/var/log/auth.log"}}},
	},
	{
		Title: "10. Финал: доказательства",
		Brief: "Упакуй логи в архив и перезапусти службу после всех изменений.",
		Goal:  "tar -czf /srv/incident/evidence.tgz /var/log → systemctl restart sshd.",
		Hint:  "tar -czf АРХИВ ПАПКА создаёт сжатый архив. В конце снова перезапусти sshd.",
		Checks: []linuxCheck{
			{Cmd: "tar", Args: []string{"-czf", "/srv/incident/evidence.tgz", "/var/log"}},
			{Cmd: "systemctl", Args: []string{"restart", "sshd"}},
		},
	},
}
