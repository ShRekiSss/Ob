package main

import (
	"strings"
	"testing"
)

func TestLinuxFullPlaythrough(t *testing.T) {
	s := NewLinuxSession()

	commands := [][]string{
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
			want = 10
		}
		if state.Done != want {
			t.Fatalf("после задания %d: Done=%d, ожидалось %d (%s)", i+1, state.Done, want, state.Goal)
		}
	}

	state := s.State()
	if !state.Completed || state.Score != 1000 {
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
	s.Execute("whoami")
	s.Execute("id")
	state := s.State()
	if state.Done != 1 || state.Score != 85 {
		t.Fatalf("счёт после подсказки и ошибки: %+v", state)
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
