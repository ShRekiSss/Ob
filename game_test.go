package main

import (
	"strings"
	"testing"
)

func TestTerminalPaths(t *testing.T) {
	s := NewSession()

	res := s.Execute("cd /station")
	if res.State.Cwd != "/station" {
		t.Fatalf("после cd /station каталог = %q, ожидался /station", res.State.Cwd)
	}

	res = s.Execute("pwd")
	if res.Output != "/station" {
		t.Fatalf("pwd вернул %q, ожидался /station", res.Output)
	}

	res = s.Execute("cd /station/../station")
	if res.State.Cwd != "/station" {
		t.Fatalf("cd /station/../station не нормализовал путь: %q", res.State.Cwd)
	}

	res = s.Execute("cd brief.txt")
	if !strings.Contains(res.Output, "не каталог") {
		t.Fatalf("cd в файл должен вернуть ошибку, получено: %q", res.Output)
	}
	if res.State.Cwd != "/station" {
		t.Fatalf("неудачный cd изменил каталог на %q", res.State.Cwd)
	}

	res = s.Execute("cd missing")
	if !strings.Contains(res.Output, "нет такого файла или каталога") {
		t.Fatalf("cd missing должен вернуть ошибку, получено: %q", res.Output)
	}
	if res.State.Cwd != "/station" {
		t.Fatalf("cd missing изменил каталог на %q", res.State.Cwd)
	}

	res = s.Execute("cd ..")
	if res.State.Cwd != "/" {
		t.Fatalf("cd .. из /station должен вернуть в /, получено %q", res.State.Cwd)
	}
}

func TestLsAndCatErrors(t *testing.T) {
	s := NewSession()

	cases := []struct {
		command string
		want    string
	}{
		{"ls /missing", "нет такого файла или каталога"},
		{"cat /station", "это каталог"},
		{"cat /etc/passwd", "нет такого файла или каталога"},
		{"cat", "использование"},
		{"ls /station extra", "слишком много аргументов"},
		{"pwd extra", "не принимает аргументов"},
	}

	for _, tc := range cases {
		res := s.Execute(tc.command)
		if !strings.Contains(res.Output, tc.want) {
			t.Errorf("%q: получено %q, ожидалась подстрока %q", tc.command, res.Output, tc.want)
		}
	}

	if s.State().Cwd != "/" {
		t.Fatalf("ошибочные команды изменили каталог на %q", s.State().Cwd)
	}
}

func TestUnknownCommand(t *testing.T) {
	s := NewSession()

	res := s.Execute("rm -rf /")
	if !strings.Contains(res.Output, "команда не найдена") {
		t.Fatalf("неизвестная команда не дала ошибку: %q", res.Output)
	}
	if res.State.HP != 3 || res.State.Done != 0 {
		t.Fatalf("неизвестная команда изменила состояние: %+v", res.State)
	}
}

func TestViewedStationRequiresLs(t *testing.T) {
	s := NewSession()

	res := s.Execute("cd /station")
	if res.State.HP != 3 {
		t.Fatalf("один cd /station не должен снимать HP, HP = %d", res.State.HP)
	}
	if !strings.Contains(res.State.Hint, "Цель 1/3") || !strings.Contains(res.State.Hint, "выполни ls") {
		t.Fatalf("в /station подсказка должна предлагать ls: %q", res.State.Hint)
	}

	res = s.Execute("ls")
	if !strings.Contains(res.Output, "brief.txt") || !strings.Contains(res.Output, "events.log") {
		t.Fatalf("ls в /station не показал файлы: %q", res.Output)
	}
	if res.State.HP != 2 {
		t.Fatalf("после просмотра /station HP = %d, ожидалось 2", res.State.HP)
	}
}

func TestLsAbsolutePathAlsoCounts(t *testing.T) {
	s := NewSession()

	res := s.Execute("ls /station")
	if !strings.Contains(res.Output, "brief.txt") {
		t.Fatalf("ls /station не показал brief.txt: %q", res.Output)
	}
	if res.State.HP != 2 {
		t.Fatalf("ls /station должен снять 1 HP, HP = %d", res.State.HP)
	}
}

func TestRewardsAreAwardedOnce(t *testing.T) {
	s := NewSession()

	res := s.Execute("ls /station")
	if res.State.HP != 2 {
		t.Fatalf("после первой цели HP = %d, ожидалось 2", res.State.HP)
	}
	res = s.Execute("ls /station")
	if res.State.HP != 2 {
		t.Fatalf("повторный ls /station снял ещё HP: %d", res.State.HP)
	}

	first := s.Execute("cat /station/brief.txt")
	if !strings.Contains(first.Output, "READY") || !strings.Contains(first.Output, "events.log") {
		t.Fatalf("brief.txt не содержит нужных сведений: %q", first.Output)
	}
	if first.State.HP != 1 {
		t.Fatalf("после чтения brief.txt HP = %d, ожидалось 1", first.State.HP)
	}

	second := s.Execute("cat /station/brief.txt")
	if second.Output != first.Output {
		t.Fatalf("повторный cat brief.txt должен снова выводить текст")
	}
	if second.State.HP != 1 {
		t.Fatalf("повторное чтение brief.txt сняло ещё HP: %d", second.State.HP)
	}

	res = s.Execute("grep READY /station/events.log")
	if !strings.Contains(res.Output, "transmitter: READY") {
		t.Fatalf("grep не нашёл маркер: %q", res.Output)
	}
	if res.State.HP != 0 || !res.State.Won {
		t.Fatalf("после третьей цели HP = %d, won = %v", res.State.HP, res.State.Won)
	}

	res = s.Execute("grep READY /station/events.log")
	if res.State.HP != 0 || !res.State.Won {
		t.Fatalf("повторный grep изменил состояние: %+v", res.State)
	}
}

func TestGrepFromStationDirectory(t *testing.T) {
	s := NewSession()

	s.Execute("cd /station")
	res := s.Execute("grep READY events.log")
	if !strings.Contains(res.Output, "READY") {
		t.Fatalf("grep из /station не нашёл маркер: %q", res.Output)
	}
	if res.State.HP != 2 {
		t.Fatalf("grep READY events.log должен засчитать цель, HP = %d", res.State.HP)
	}
}

func TestCatLogDoesNotCompleteGrepGoal(t *testing.T) {
	s := NewSession()

	res := s.Execute("cat /station/events.log")
	if !strings.Contains(res.Output, "READY") {
		t.Fatalf("журнал должен содержать маркер: %q", res.Output)
	}
	if res.State.HP != 3 {
		t.Fatalf("cat журнала не должен заменять grep, HP = %d", res.State.HP)
	}
}

func TestGrepIsPlainSubstring(t *testing.T) {
	s := NewSession()

	res := s.Execute("grep READ /station/events.log")
	if !strings.Contains(res.Output, "transmitter: READY") {
		t.Fatalf("grep должен искать подстроку: %q", res.Output)
	}
	if res.State.HP != 3 {
		t.Fatalf("неполный маркер не должен засчитывать цель, HP = %d", res.State.HP)
	}

	res = s.Execute("grep -n READY /station/events.log")
	if !strings.Contains(res.Output, "флаги не поддерживаются") {
		t.Fatalf("grep с флагом должен вернуть понятную ошибку: %q", res.Output)
	}

	res = s.Execute("grep")
	if !strings.Contains(res.Output, "использование") {
		t.Fatalf("grep без аргументов должен вернуть подсказку: %q", res.Output)
	}
}

func TestReset(t *testing.T) {
	s := NewSession()

	s.Execute("ls /station")
	s.Execute("cat /station/brief.txt")
	if s.State().HP != 1 {
		t.Fatalf("подготовка сброса не сработала: %+v", s.State())
	}

	state := s.Reset()
	if state.HP != 3 || state.Done != 0 || state.Won || state.Cwd != "/" {
		t.Fatalf("после сброса состояние = %+v", state)
	}

	res := s.Execute("cd /station")
	if res.State.HP != 3 {
		t.Fatalf("после сброса цели не сброшены: HP = %d", res.State.HP)
	}
	res = s.Execute("ls")
	if res.State.HP != 2 {
		t.Fatalf("после сброса миссия не играется заново: HP = %d", res.State.HP)
	}
}

func TestHintProgresses(t *testing.T) {
	s := NewSession()

	hint := s.State().Hint
	if !strings.Contains(hint, "Цель 1/3") || !strings.Contains(hint, "ls /station") {
		t.Fatalf("начальная подсказка: %q", hint)
	}
	s.Execute("ls /station")
	hint = s.State().Hint
	if !strings.Contains(hint, "Цель 2/3") || !strings.Contains(hint, "cat /station/brief.txt") {
		t.Fatalf("подсказка после первой цели: %q", hint)
	}
	s.Execute("cat /station/brief.txt")
	hint = s.State().Hint
	if !strings.Contains(hint, "Цель 3/3") || !strings.Contains(hint, "grep READY /station/events.log") {
		t.Fatalf("подсказка после второй цели: %q", hint)
	}
	s.Execute("grep READY /station/events.log")
	if !strings.Contains(s.State().Hint, "Связь восстановлена") {
		t.Fatalf("подсказка после победы: %q", s.State().Hint)
	}
}

func TestHintsUseRelativePathsInsideStation(t *testing.T) {
	s := NewSession()
	s.Execute("cd /station")

	hint := s.State().Hint
	if !strings.Contains(hint, "Цель 1/3") || !strings.Contains(hint, "выполни ls") {
		t.Fatalf("подсказка в /station: %q", hint)
	}

	s.Execute("ls")
	hint = s.State().Hint
	if !strings.Contains(hint, "cat brief.txt") || strings.Contains(hint, "/station/brief.txt") {
		t.Fatalf("подсказка должна предлагать относительный путь: %q", hint)
	}

	s.Execute("cat brief.txt")
	hint = s.State().Hint
	if !strings.Contains(hint, "grep READY events.log") || strings.Contains(hint, "/station/events.log") {
		t.Fatalf("подсказка должна предлагать относительный путь: %q", hint)
	}
}

func TestHelpExplainsEveryCommand(t *testing.T) {
	s := NewSession()

	res := s.Execute("help")
	if res.State.HP != 3 {
		t.Fatalf("help не должен снимать HP: %d", res.State.HP)
	}
	for _, want := range []string{"pwd", "ls", "cd", "cat", "grep", "clear", "help", "/station/brief.txt", "READY"} {
		if !strings.Contains(res.Output, want) {
			t.Fatalf("в help нет %q:\n%s", want, res.Output)
		}
	}

	if !strings.Contains(s.Execute("?").Output, "СПРАВКА") {
		t.Fatal("псевдоним ? тоже должен показывать справку")
	}
}

func TestLsOrderIsStable(t *testing.T) {
	s := NewSession()
	first := s.Execute("ls /station").Output
	for i := 0; i < 20; i++ {
		if got := s.Execute("ls /station").Output; got != first {
			t.Fatalf("порядок вывода ls нестабилен:\n%s\n---\n%s", first, got)
		}
	}
}
