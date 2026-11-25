package main

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
)

// LinuxSession — состояние тренажёра Linux. Полностью виртуальное:
// файлы, процессы, сокеты и правила iptables живут только в памяти сессии.
type LinuxSession struct {
	mu sync.Mutex

	fs       map[string]linuxFile
	procs    []linuxProcess
	sockets  []linuxSocket
	rules    []string
	cwd      string
	mission  int // 0-based индекс текущего задания
	checked  [][]bool
	hints    int
	mistakes int
	sshdUp   bool
	finished bool
	scare    bool
}

type LinuxSnapshot struct {
	Mission   int    `json:"mission"`
	Total     int    `json:"total"`
	Title     string `json:"title"`
	Brief     string `json:"brief"`
	Goal      string `json:"goal"`
	Hint      string `json:"hint"`
	Done      int    `json:"done"`
	StepDone  int    `json:"step_done"`
	StepTotal int    `json:"step_total"`
	Score     int    `json:"score"`
	Cwd       string `json:"cwd"`
	Completed bool   `json:"completed"`
}

type LinuxResult struct {
	Output string        `json:"output"`
	State  LinuxSnapshot `json:"state"`
	Scare  bool          `json:"scare"`
}

func NewLinuxSession() *LinuxSession {
	fs := make(map[string]linuxFile, len(linuxFSTemplate))
	for k, v := range linuxFSTemplate {
		fs[k] = v
	}
	fs["/var/log/auth.log"] = linuxFile{Data: strings.Join(authLog, "\n"), Mode: "640", Hash: "9f2c4a1b7e3d8065c11b0a2f4d6e8a90"}
	checked := make([][]bool, len(linuxMissions))
	for i, m := range linuxMissions {
		checked[i] = make([]bool, len(m.Checks))
	}
	return &LinuxSession{
		fs:      fs,
		procs:   append([]linuxProcess(nil), linuxProcessTemplate...),
		sockets: append([]linuxSocket(nil), linuxSocketTemplate...),
		cwd:     "/home/analyst",
		checked: checked,
		sshdUp:  true,
	}
}

func (s *LinuxSession) Execute(line string) LinuxResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scare = false
	out := s.run(line)
	return LinuxResult{Output: out, State: s.snapshot(), Scare: s.scare}
}

func (s *LinuxSession) State() LinuxSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot()
}

func (s *LinuxSession) Reset() LinuxSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetLocked()
	return s.snapshot()
}

// resetLocked сбрасывает состояние. Вызывается только с удержанным mu:
// sync.Mutex не реентерабелен, поэтому повторный Lock из run("reset")
// навсегда блокировал бы сессию.
func (s *LinuxSession) resetLocked() {
	fresh := NewLinuxSession()
	s.fs, s.procs, s.sockets = fresh.fs, fresh.procs, fresh.sockets
	s.rules, s.cwd, s.mission = nil, fresh.cwd, 0
	s.checked, s.hints, s.mistakes = fresh.checked, 0, 0
	s.sshdUp, s.finished, s.scare = true, false, false
}

func (s *LinuxSession) snapshot() LinuxSnapshot {
	m := linuxMissions[s.mission]
	stepDone := 0
	for _, ok := range s.checked[s.mission] {
		if ok {
			stepDone++
		}
	}
	completed := s.mission
	if s.finished {
		completed = len(linuxMissions)
	}
	score := completed*100 - s.hints*10 - s.mistakes*5
	if score < 0 {
		score = 0
	}
	return LinuxSnapshot{
		Mission: s.mission + 1, Total: len(linuxMissions),
		Title: m.Title, Brief: m.Brief, Goal: m.Goal, Hint: m.Hint,
		Done: func() int {
			if s.finished {
				return len(linuxMissions)
			}
			return s.mission
		}(), StepDone: stepDone, StepTotal: len(m.Checks),
		Score: score, Cwd: s.cwd, Completed: s.finished,
	}
}

// stripQuotes убирает кавычки, чтобы "failed password" и '*.log'
// совпадали с шаблонами заданий.
func stripQuotes(s string) string {
	return strings.Trim(s, "\"'")
}

func linuxArgs(line string) (string, []string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", nil
	}
	args := make([]string, 0, len(fields)-1)
	for i := 1; i < len(fields); i++ {
		a := fields[i]
		quote := byte(0)
		if len(a) > 0 && (a[0] == '"' || a[0] == '\'') {
			quote = a[0]
		}
		if quote != 0 && !(len(a) > 1 && a[len(a)-1] == quote) {
			// Собираем "failed password" обратно в один аргумент.
			for i+1 < len(fields) {
				i++
				a += " " + fields[i]
				if fields[i][len(fields[i])-1] == quote {
					break
				}
			}
		}
		args = append(args, stripQuotes(a))
	}
	return strings.ToLower(fields[0]), args
}

func (s *LinuxSession) checkProgress(cmd string, args []string) {
	if s.mission >= len(linuxMissions) {
		return
	}
	for i, check := range linuxMissions[s.mission].Checks {
		if s.checked[s.mission][i] || check.Cmd != cmd {
			continue
		}
		ok := true
		for _, need := range check.Args {
			found := false
			for _, a := range args {
				if strings.Contains(a, need) {
					found = true
					break
				}
			}
			if !found {
				ok = false
				break
			}
		}
		if ok {
			s.checked[s.mission][i] = true
		}
	}
	all := true
	for _, ok := range s.checked[s.mission] {
		if !ok {
			all = false
		}
	}
	if all {
		if s.mission == len(linuxMissions)-1 {
			s.finished = true
		} else {
			s.mission++
		}
	}
}

const linuxHelp = `ТРЕНАЖЁР LINUX — 13 заданий для 10.02.05

КОМАНДЫ
  whoami, id          учётная запись, UID/GID, группы
  pwd, ls [-la], cd   навигация и скрытые файлы
  cat ФАЙЛ            чтение файла
  find ПУТЬ -name ШАБЛОН   поиск файлов
  grep [-i] [-c] СЛОВО ФАЙЛ   поиск в журнале
  chmod ПРАВА ФАЙЛ    смена прав доступа
  ps aux, kill PID    процессы и завершение процесса
  ss -tulpn           слушающие порты
  iptables -A INPUT -s IP -j DROP   блокировка адреса
  sed -i ... ФАЙЛ     правка конфига на месте
  sha256sum ФАЙЛ      контроль целостности
  tar -czf АРХИВ ПАПКА   архивация доказательств
  systemctl restart sshd   перезапуск службы
  hint                подсказка (−10 очков)
  clear               очистить экран
  reset               начать тренажёр заново`

func (s *LinuxSession) run(line string) string {
	cmd, args := linuxArgs(line)
	if cmd == "" {
		return ""
	}

	s.checkProgress(cmd, args)

	switch cmd {
	case "whoami":
		return "analyst"
	case "id":
		return "uid=1001(analyst) gid=1001(analyst) groups=1001(analyst),27(sudo),100(security)"
	case "pwd":
		return s.cwd
	case "cd":
		return s.cd(args)
	case "ls":
		return s.ls(args)
	case "cat":
		return s.cat(args)
	case "find":
		return s.find(args)
	case "grep":
		return s.grep(args)
	case "chmod":
		return s.chmod(args)
	case "ps":
		return s.ps()
	case "kill":
		return s.kill(args)
	case "ss":
		return s.ss()
	case "iptables":
		return s.iptables(args)
	case "sed":
		return s.sed(args)
	case "sha256sum":
		return s.sha256sum(args)
	case "tar":
		return s.tar(args)
	case "systemctl":
		return s.systemctl(args)
	case "hint":
		s.hints++
		return "Подсказка: " + linuxMissions[s.mission].Hint
	case "help", "?":
		return linuxHelp
	case "reset":
		s.resetLocked()
		return "Тренажёр Linux начат заново."
	case "clear":
		return ""
	default:
		s.mistakes++
		return fmt.Sprintf("команда не найдена: %s\nНабери help — там список команд тренажёра.", cmd)
	}
}

func (s *LinuxSession) cd(args []string) string {
	target := "/"
	if len(args) > 0 {
		resolved, err := resolvePath(s.cwd, args[0])
		if err != nil {
			return "cd: " + err.Error()
		}
		target = resolved
	}
	node, ok := s.fs[target]
	if !ok {
		return fmt.Sprintf("cd: %s: нет такого каталога", target)
	}
	if !node.Dir {
		return fmt.Sprintf("cd: %s: не каталог", target)
	}
	s.cwd = target
	return ""
}

func (s *LinuxSession) ls(args []string) string {
	long := false
	var target string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			if strings.Contains(a, "l") || strings.Contains(a, "a") {
				long = true
			}
			continue
		}
		target = a
	}
	resolved, err := resolvePath(s.cwd, target)
	if err != nil {
		return "ls: " + err.Error()
	}
	node, ok := s.fs[resolved]
	if !ok {
		return fmt.Sprintf("ls: %s: нет такого файла или каталога", target)
	}
	if !node.Dir {
		return path.Base(resolved)
	}

	names := make([]string, 0)
	for p := range s.fs {
		if p == resolved || path.Dir(p) != resolved {
			continue
		}
		names = append(names, path.Base(p))
	}
	sort.Strings(names)
	if !long {
		if len(names) == 0 {
			return "(пусто)"
		}
		return strings.Join(names, "\n")
	}

	lines := []string{"drwxr-xr-x  .", "drwxr-xr-x  .."}
	for _, name := range names {
		f := s.fs[path.Join(resolved, name)]
		mode := f.Mode
		if mode == "" {
			mode = "755"
		}
		prefix := "-"
		if f.Dir {
			prefix = "d"
		}
		lines = append(lines, prefix+mode+"  "+name)
	}
	return strings.Join(lines, "\n")
}

func (s *LinuxSession) cat(args []string) string {
	if len(args) == 0 {
		return "cat: использование: cat ФАЙЛ"
	}
	target, err := resolvePath(s.cwd, args[0])
	if err != nil {
		return "cat: " + err.Error()
	}
	node, ok := s.fs[target]
	if !ok {
		return fmt.Sprintf("cat: %s: нет такого файла", args[0])
	}
	if node.Dir {
		return fmt.Sprintf("cat: %s: это каталог", args[0])
	}
	if target == "/station/secrets.txt" {
		s.scare = true
	}
	return strings.TrimRight(node.Data, "\n")
}

func (s *LinuxSession) find(args []string) string {
	root, pattern := "/", ""
	for i := 0; i < len(args); i++ {
		if args[i] == "-name" && i+1 < len(args) {
			pattern = args[i+1]
			i++
		} else if !strings.HasPrefix(args[i], "-") {
			root = args[i]
		}
	}
	resolved, err := resolvePath(s.cwd, root)
	if err != nil {
		return "find: " + err.Error()
	}
	// resolved == "/" давал префикс "//" и не находил ничего.
	prefix := strings.TrimSuffix(resolved, "/") + "/"
	matches := make([]string, 0)
	for p := range s.fs {
		if !strings.HasPrefix(p, prefix) || s.fs[p].Dir {
			continue
		}
		if pattern == "" || matchGlob(path.Base(p), pattern) {
			matches = append(matches, p)
		}
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return "ничего не найдено"
	}
	return strings.Join(matches, "\n")
}

// matchGlob поддерживает только * и ? — этого достаточно для "*.log".
func matchGlob(name, pattern string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return name == pattern
	}
	if !strings.HasPrefix(name, parts[0]) {
		return false
	}
	rest := name[len(parts[0]):]
	for i := 1; i < len(parts); i++ {
		part := parts[i]
		if part == "" {
			continue
		}
		idx := strings.Index(rest, part)
		if idx < 0 {
			return false
		}
		rest = rest[idx+len(part):]
	}
	return true
}

func (s *LinuxSession) grep(args []string) string {
	insensitive, count := false, false
	var rest []string
	for _, a := range args {
		switch a {
		case "-i":
			insensitive = true
		case "-c":
			count = true
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) < 2 {
		return "grep: использование: grep [-i] [-c] СЛОВО ФАЙЛ"
	}
	needle, fileArg := rest[0], rest[len(rest)-1]
	target, err := resolvePath(s.cwd, fileArg)
	if err != nil {
		return "grep: " + err.Error()
	}
	node, ok := s.fs[target]
	if !ok {
		return fmt.Sprintf("grep: %s: нет такого файла", fileArg)
	}
	hay := node.Data
	compareNeedle := needle
	if insensitive {
		compareNeedle = strings.ToLower(needle)
	}
	// Сравниваем в нижнем регистре, но в выводе возвращаем исходную строку:
	// раньше -i печатал нижний регистр и мог подменить строку дубликатом.
	matches := make([]string, 0)
	for _, line := range strings.Split(strings.TrimRight(hay, "\n"), "\n") {
		cmp := line
		if insensitive {
			cmp = strings.ToLower(cmp)
		}
		if strings.Contains(cmp, compareNeedle) {
			matches = append(matches, line)
		}
	}
	if count {
		return fmt.Sprintf("%d", len(matches))
	}
	if len(matches) == 0 {
		return fmt.Sprintf("совпадений не найдено: %s", needle)
	}
	return strings.Join(matches, "\n")
}

func (s *LinuxSession) chmod(args []string) string {
	if len(args) < 2 {
		return "chmod: использование: chmod ПРАВА ФАЙЛ"
	}
	target, err := resolvePath(s.cwd, args[1])
	if err != nil {
		return "chmod: " + err.Error()
	}
	node, ok := s.fs[target]
	if !ok {
		return fmt.Sprintf("chmod: %s: нет такого файла", args[1])
	}
	node.Mode = args[0]
	s.fs[target] = node
	return fmt.Sprintf("права %s установлены: %s", target, args[0])
}

func (s *LinuxSession) ps() string {
	lines := []string{"USER       PID  %CPU  COMMAND"}
	for _, p := range s.procs {
		lines = append(lines, fmt.Sprintf("%-10s %-4d %-5s %s", p.User, p.PID, p.CPU, p.Cmd))
	}
	return strings.Join(lines, "\n")
}

func (s *LinuxSession) kill(args []string) string {
	if len(args) == 0 {
		return "kill: использование: kill PID"
	}
	var pid int
	if _, err := fmt.Sscanf(args[0], "%d", &pid); err != nil {
		return "kill: неверный PID"
	}
	for i, p := range s.procs {
		if p.PID == pid {
			s.procs = append(s.procs[:i], s.procs[i+1:]...)
			for j := 0; j < len(s.sockets); j++ {
				if strings.HasPrefix(s.sockets[j].Proc, fmt.Sprintf("%d/", pid)) {
					s.sockets = append(s.sockets[:j], s.sockets[j+1:]...)
					j--
				}
			}
			return fmt.Sprintf("процесс %d завершён", pid)
		}
	}
	return fmt.Sprintf("kill: процесс %d не найден", pid)
}

func (s *LinuxSession) ss() string {
	lines := []string{"Proto  Local Address        State    PID/Program"}
	for _, c := range s.sockets {
		lines = append(lines, fmt.Sprintf("%-6s %-20s %-8s %s", c.Proto, c.Addr, c.State, c.Proc))
	}
	return strings.Join(lines, "\n")
}

func (s *LinuxSession) iptables(args []string) string {
	rule := strings.Join(args, " ")
	s.rules = append(s.rules, rule)
	return "правило добавлено: " + rule
}

func (s *LinuxSession) sed(args []string) string {
	if len(args) < 3 {
		return "sed: использование: sed -i \"s/БЫЛО/СТАЛО/\" ФАЙЛ"
	}
	target, err := resolvePath(s.cwd, args[len(args)-1])
	if err != nil {
		return "sed: " + err.Error()
	}
	node, ok := s.fs[target]
	if !ok {
		return fmt.Sprintf("sed: %s: нет такого файла", target)
	}
	expr := ""
	for _, a := range args {
		if strings.HasPrefix(a, "s/") {
			expr = a
		}
	}
	parts := strings.Split(strings.TrimPrefix(expr, "s/"), "/")
	if len(parts) >= 2 {
		node.Data = strings.ReplaceAll(node.Data, parts[0], parts[1])
	}
	s.fs[target] = node
	return fmt.Sprintf("файл %s изменён", target)
}

func (s *LinuxSession) sha256sum(args []string) string {
	if len(args) == 0 {
		return "sha256sum: использование: sha256sum ФАЙЛ"
	}
	target, err := resolvePath(s.cwd, args[0])
	if err != nil {
		return "sha256sum: " + err.Error()
	}
	node, ok := s.fs[target]
	if !ok {
		return fmt.Sprintf("sha256sum: %s: нет такого файла", args[0])
	}
	hash := node.Hash
	if hash == "" {
		hash = "d41d8cd98f00b204e9800998ecf8427e"
	}
	return hash + "  " + target
}

func (s *LinuxSession) tar(args []string) string {
	var archive string
	for i, a := range args {
		if a == "-czf" && i+1 < len(args) {
			archive = args[i+1]
		}
	}
	if archive == "" {
		return "tar: использование: tar -czf АРХИВ ПАПКА"
	}
	resolved, err := resolvePath(s.cwd, archive)
	if err != nil {
		return "tar: " + err.Error()
	}
	s.fs[resolved] = linuxFile{Data: "[архив]", Mode: "644"}
	return "архив создан: " + resolved
}

func (s *LinuxSession) systemctl(args []string) string {
	if len(args) < 2 || args[0] != "restart" {
		return "systemctl: использование: systemctl restart ИМЯ"
	}
	s.sshdUp = true
	return fmt.Sprintf("служба %s перезапущена", args[1])
}
