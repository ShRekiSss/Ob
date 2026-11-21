package main

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
)

const maxHP = 3

const briefText = `БОРТОВОЙ ЖУРНАЛ СТАНЦИИ «ГЕЛИОС-7»
Запись от 14.11.2087

После сбоя питания основной канал связи не отвечает.
Передатчик, скорее всего, цел: аварийный модуль должен
был перейти в режим ожидания.

В журнале событий (events.log) последняя запись о состоянии
передатчика помечена маркером READY. Найди эту строку —
по ней мы поймём, что канал можно восстанавливать.

Не запускай перезагрузку антенны, пока не увидишь READY.`

const eventsLog = `2087-11-14T02:11:03Z boot power-on
2087-11-14T02:11:04Z reactor: nominal
2087-11-14T02:11:19Z comm: carrier lost
2087-11-14T02:12:40Z antenna: parked
2087-11-14T02:15:02Z diagnostic: transmitter self-test
2087-11-14T02:15:03Z transmitter: OFFLINE (no power)
2087-11-14T02:18:27Z solar array: charging
2087-11-14T02:41:55Z battery: 82%
2087-11-14T03:02:10Z transmitter: warming up
2087-11-14T03:04:44Z transmitter: READY
2087-11-14T03:05:01Z comm: handshake available`

const antennaConfig = `# Конфигурация антенны «Гелиос-7»
band = X
azimuth = 117
elevation = 24
auto_restart = false`

type fsNode struct {
	dir  bool
	data string
}

// missionFS — фиксированная виртуальная файловая система.
// Никакие реальные файлы или команды ОС не используются.
// Такая же копия данных есть в index.html для автономного режима file://.
var missionFS = map[string]fsNode{
	"/":                     {dir: true},
	"/station":              {dir: true},
	"/station/brief.txt":    {data: briefText},
	"/station/events.log":   {data: eventsLog},
	"/station/antenna.conf": {data: antennaConfig},
}

// Session хранит состояние одной игры: текущий каталог и три признака
// выполненных учебных целей. HP не хранится отдельно, а вычисляется из них.
type Session struct {
	mu sync.Mutex

	cwd string

	viewedStation bool
	readBrief     bool
	foundReady    bool
}

type Snapshot struct {
	Cwd   string `json:"cwd"`
	HP    int    `json:"hp"`
	MaxHP int    `json:"max_hp"`
	Done  int    `json:"done"`
	Won   bool   `json:"won"`
	Hint  string `json:"hint"`
}

type Result struct {
	Output string   `json:"output"`
	State  Snapshot `json:"state"`
}

func NewSession() *Session {
	return &Session{cwd: "/"}
}

func (s *Session) Execute(line string) Result {
	s.mu.Lock()
	defer s.mu.Unlock()

	output := s.run(line)
	return Result{Output: output, State: s.snapshot()}
}

func (s *Session) State() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot()
}

func (s *Session) Reset() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cwd = "/"
	s.viewedStation = false
	s.readBrief = false
	s.foundReady = false
	return s.snapshot()
}

func (s *Session) snapshot() Snapshot {
	done := 0
	if s.viewedStation {
		done++
	}
	if s.readBrief {
		done++
	}
	if s.foundReady {
		done++
	}

	return Snapshot{
		Cwd:   s.cwd,
		HP:    maxHP - done,
		MaxHP: maxHP,
		Done:  done,
		Won:   done == maxHP,
		Hint:  s.hint(),
	}
}

func (s *Session) hint() string {
	switch {
	case s.viewedStation && s.readBrief && s.foundReady:
		return "Связь восстановлена. Можно осмотреться или начать миссию заново."
	case !s.viewedStation:
		if s.cwd == "/station" {
			return "Цель 1/3: выполни ls — увидишь brief.txt и events.log."
		}
		return "Цель 1/3: выполни ls /station или cd /station, затем ls."
	case !s.readBrief:
		if s.cwd == "/station" {
			return "Цель 2/3: выполни cat brief.txt — прочитай бортовой журнал."
		}
		return "Цель 2/3: выполни cat /station/brief.txt — прочитай бортовой журнал."
	default:
		if s.cwd == "/station" {
			return "Цель 3/3: выполни grep READY events.log — найди строку с маркером READY."
		}
		return "Цель 3/3: выполни grep READY /station/events.log — найди строку с маркером READY."
	}
}

func (s *Session) run(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}

	cmd, args := fields[0], fields[1:]
	switch cmd {
	case "pwd":
		if len(args) != 0 {
			return "pwd: команда не принимает аргументов"
		}
		return s.cwd
	case "ls":
		return s.ls(args)
	case "cd":
		return s.cd(args)
	case "cat":
		return s.cat(args)
	case "grep":
		return s.grep(args)
	default:
		return fmt.Sprintf("команда не найдена: %s\nДоступные команды: pwd, ls, cd, cat, grep", cmd)
	}
}

func (s *Session) ls(args []string) string {
	if len(args) > 1 {
		return "ls: слишком много аргументов\nиспользование: ls [путь]"
	}

	arg := "."
	if len(args) == 1 {
		arg = args[0]
	}

	target, err := resolvePath(s.cwd, arg)
	if err != nil {
		return "ls: " + err.Error()
	}

	node, ok := missionFS[target]
	if !ok {
		return fmt.Sprintf("ls: %s: нет такого файла или каталога", arg)
	}
	if !node.dir {
		return path.Base(target)
	}

	// Цель №1: игрок увидел содержимое каталога /station.
	if target == "/station" {
		s.viewedStation = true
	}

	names := childrenOf(target)
	if len(names) == 0 {
		return "(пусто)"
	}

	return strings.Join(names, "\n")
}

func (s *Session) cd(args []string) string {
	if len(args) > 1 {
		return "cd: слишком много аргументов\nиспользование: cd [путь]"
	}

	target := "/"
	displayArg := "/"
	if len(args) == 1 {
		displayArg = args[0]
		resolved, err := resolvePath(s.cwd, args[0])
		if err != nil {
			return "cd: " + err.Error()
		}
		target = resolved
	}

	node, ok := missionFS[target]
	if !ok {
		return fmt.Sprintf("cd: %s: нет такого файла или каталога", displayArg)
	}
	if !node.dir {
		return fmt.Sprintf("cd: %s: не каталог", displayArg)
	}

	s.cwd = target
	return ""
}

func (s *Session) cat(args []string) string {
	if len(args) != 1 {
		return "cat: использование: cat ФАЙЛ"
	}

	target, err := resolvePath(s.cwd, args[0])
	if err != nil {
		return "cat: " + err.Error()
	}

	node, ok := missionFS[target]
	if !ok {
		return fmt.Sprintf("cat: %s: нет такого файла или каталога", args[0])
	}
	if node.dir {
		return fmt.Sprintf("cat: %s: это каталог", args[0])
	}

	// Цель №2: игрок прочитал инструкцию brief.txt.
	if target == "/station/brief.txt" {
		s.readBrief = true
	}

	return strings.TrimRight(node.data, "\n")
}

func (s *Session) grep(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return "grep: флаги не поддерживаются\nиспользование: grep СЛОВО ФАЙЛ"
		}
	}
	if len(args) != 2 {
		return "grep: использование: grep СЛОВО ФАЙЛ\nфлаги и регулярные выражения не поддерживаются"
	}

	pattern, fileArg := args[0], args[1]

	target, err := resolvePath(s.cwd, fileArg)
	if err != nil {
		return "grep: " + err.Error()
	}

	node, ok := missionFS[target]
	if !ok {
		return fmt.Sprintf("grep: %s: нет такого файла или каталога", fileArg)
	}
	if node.dir {
		return fmt.Sprintf("grep: %s: это каталог", fileArg)
	}

	var matches []string
	for _, line := range strings.Split(strings.TrimRight(node.data, "\n"), "\n") {
		if strings.Contains(line, pattern) {
			matches = append(matches, line)
		}
	}
	if len(matches) == 0 {
		return fmt.Sprintf("grep: «%s» не найдено в %s", pattern, path.Base(target))
	}

	// Цель №3: найден маркер READY в журнале событий.
	if target == "/station/events.log" && pattern == "READY" {
		s.foundReady = true
	}

	return strings.Join(matches, "\n")
}

func resolvePath(cwd, arg string) (string, error) {
	if arg == "" {
		arg = "."
	}

	var resolved string
	if strings.HasPrefix(arg, "/") {
		resolved = path.Clean(arg)
	} else {
		resolved = path.Clean(path.Join(cwd, arg))
	}

	if !strings.HasPrefix(resolved, "/") {
		return "", fmt.Errorf("%s: путь вне виртуальной ФС", arg)
	}
	return resolved, nil
}

func childrenOf(dir string) []string {
	names := make([]string, 0, 4)
	for p, node := range missionFS {
		if p == "/" || p == dir {
			continue
		}
		if path.Dir(p) != dir {
			continue
		}
		name := path.Base(p)
		if node.dir {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
