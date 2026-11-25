package main

import (
	"strings"
	"testing"
	"time"
)

func TestLinuxFullPlaythrough(t *testing.T) {
	s := NewLinuxSession()

	commands := [][]string{
		{"ls /station"},
		{"cat /station/brief.txt"},
		{"grep READY /station/events.log"},
		{"whoami", "id"},
		{"ls -la /home/analyst", "cat /home/analyst/.notes"},
		{`find /var/log -name "*.log"`},
		{`grep -i "failed password" /var/log/auth.log`},
		{"chmod 640 /srv/incident/config.yml"},
		{"ps aux", "kill 1337"},
		{"ss -tulpn", "iptables -A INPUT -s 203.0.113.45 -j DROP"},
		{`cat /etc/ssh/sshd_config`, `sed -i "s/PermitRootLogin yes/PermitRootLogin no/" /etc/ssh/sshd_config`, "systemctl restart sshd"},
		{"sha256sum /var/log/auth.log"},
		{"tar -czf /srv/incident/evidence.tgz /var/log", "systemctl restart sshd"},
	}

	for i, group := range commands {
		for _, cmd := range group {
			res := s.Execute(cmd)
			if strings.Contains(res.Output, "команда не найдена") {
				t.Fatalf("задание %d, команда %q: unexpected: %s", i+1, cmd, res.Output)
			}
		}
		state := s.State()
		want := i + 1
		if state.Completed {
			want = len(linuxMissions)
		}
		if state.Done != want {
			t.Fatalf("после задания %d: Done=%d, ожидалось %d (%s)", i+1, state.Done, want, state.Goal)
		}
	}

	state := s.State()
	if !state.Completed || state.Score != 1300 {
		t.Fatalf("финал: %+v", state)
	}
	if got := s.fs["/srv/incident/config.yml"].Mode; got != "640" {
		t.Fatalf("chmod не применился: %s", got)
	}
	if strings.Contains(s.fs["/etc/ssh/sshd_config"].Data, "PermitRootLogin yes") {
		t.Fatalf("sed не изменил конфиг: %s", s.fs["/etc/ssh/sshd_config"].Data)
	}
	for _, p := range s.procs {
		if p.PID == 1337 {
			t.Fatal("процесс 1337 не завершён")
		}
	}
	if _, ok := s.fs["/srv/incident/evidence.tgz"]; !ok {
		t.Fatal("архив доказательств не создан")
	}
}

func TestLinuxScoreAndHint(t *testing.T) {
	s := NewLinuxSession()
	s.Execute("hint")
	s.Execute("bogus-command")
	s.Execute("ls /station")
	state := s.State()
	if state.Done != 1 || state.Score != 85 {
		t.Fatalf("счёт после подсказки и ошибки: %+v", state)
	}
}

func TestLinuxSecretFileScare(t *testing.T) {
	s := NewLinuxSession()

	res := s.Execute("cat /station/secrets.txt")
	if !res.Scare || !strings.Contains(res.Output, "Совершенно секретно") {
		t.Fatalf("секретный файл не сработал: %+v", res)
	}
	if next := s.Execute("ls /station"); next.Scare {
		t.Fatal("флаг скримера не сбросился на следующей команде")
	}
	if _, ok := s.fs["/station/secrets.txt"]; !ok {
		t.Fatal("secrets.txt отсутствует в виртуальной ФС")
	}
}

func TestLinuxReset(t *testing.T) {
	s := NewLinuxSession()
	s.Execute("whoami")
	s.Execute("id")
	s.Execute("kill 1337")
	s.Reset()
	state := s.State()
	if state.Done != 0 || state.Score != 0 || state.Mission != 1 {
		t.Fatalf("после сброса: %+v", state)
	}
	for _, p := range s.procs {
		if p.PID == 1337 {
			return
		}
	}
	t.Fatal("сброс не восстановил процессы")
}

// Команда reset внутри тренажёра раньше вызывала Reset() с уже
// заблокированным мьютексом и навсегда вешала сессию.
func TestLinuxResetCommandDoesNotDeadlock(t *testing.T) {
	s := NewLinuxSession()
	s.Execute("whoami")
	done := make(chan string, 1)
	go func() {
		res := s.Execute("reset")
		done <- res.Output
	}()
	select {
	case out := <-done:
		if !strings.Contains(out, "начат заново") {
			t.Fatalf("reset: %q", out)
		}
	case <-time.After(time.Second):
		t.Fatal("команда reset заблокировала сессию")
	}
	if st := s.State(); st.Mission != 1 || st.Done != 0 || st.Score != 0 {
		t.Fatalf("состояние после reset: %+v", st)
	}
	// Сессия должна отвечать на следующие команды.
	if out := s.Execute("whoami").Output; out != "analyst" {
		t.Fatalf("сессия не отвечает после reset: %q", out)
	}
}

func TestLinuxFindRootAndCase(t *testing.T) {
	s := NewLinuxSession()
	out := s.Execute(`find / -name "*.log"`).Output
	if !strings.Contains(out, "/var/log/auth.log") {
		t.Fatalf("find / не нашёл логи: %q", out)
	}
	if !strings.Contains(out, "/station/events.log") {
		t.Fatalf("find / должен находить логи во всех подкаталогах: %q", out)
	}
}

func TestLinuxGrepInsensitiveKeepsOriginalCase(t *testing.T) {
	s := NewLinuxSession()
	out := s.Execute(`grep -i "failed password" /var/log/auth.log`).Output
	if !strings.Contains(out, "Failed password for root") {
		t.Fatalf("grep -i должен сохранять исходный регистр: %q", out)
	}
	if count := s.Execute(`grep -i -c "failed password" /var/log/auth.log`).Output; count != "3" {
		t.Fatalf("grep -i -c: %q", count)
	}
}

func TestLinuxHelpMissionCount(t *testing.T) {
	if !strings.Contains(linuxHelp, "13 заданий") {
		t.Fatal("в help указано неверное число заданий")
	}
}
