package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Тесты в этом файле не дают офлайн-режиму (index.html) и серверу (game.go)
// незаметно разъехаться. README прямо рекомендует править оба места —
// теперь расхождение ловится автоматически на этапе go test.

func pageHTML(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatalf("не удалось прочитать index.html: %v", err)
	}
	return string(data)
}

// jsTemplateValue возвращает содержимое nth-го шаблонного литерала,
// который начинается с маркера `data: “.
func jsTemplateValue(t *testing.T, page, marker string, nth int) string {
	t.Helper()
	parts := strings.Split(page, marker)
	if len(parts) <= nth+1 {
		t.Fatalf("в index.html не найден %d-й `%s`", nth+1, marker)
	}
	rest := parts[nth+1]
	end := strings.Index(rest, "`")
	if end < 0 {
		t.Fatalf("не закрыт шаблонный литерал после `%s`", marker)
	}
	return rest[:end]
}

func TestOfflineFilesMatchServerFS(t *testing.T) {
	page := pageHTML(t)
	want := []struct {
		name, server, marker string
		nth                  int
	}{
		{"brief.txt", briefText, "data: `", 0},
		{"events.log", eventsLog, "data: `", 1},
		{"antenna.conf", antennaConfig, "data: `", 2},
	}
	for _, tc := range want {
		got := jsTemplateValue(t, page, tc.marker, tc.nth)
		if got != tc.server {
			t.Errorf("содержимое %s в index.html не совпадает с game.go:\n--- HTML ---\n%s\n--- Go ---\n%s", tc.name, got, tc.server)
		}
	}
}

func TestOfflineHelpMatchesServerHelp(t *testing.T) {
	page := pageHTML(t)
	for _, line := range strings.Split(helpText, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.Contains(page, line) {
			t.Errorf("в index.html нет строки из help: %q", line)
		}
	}
}

func TestOfflineVersionAndDataMatchServer(t *testing.T) {
	page := pageHTML(t)

	if !strings.Contains(page, "v"+version) {
		t.Errorf("в index.html указана не та версия, что в main.go (ожидалась v%s)", version)
	}
	for p := range missionFS {
		if !strings.Contains(page, "\""+p+"\"") {
			t.Errorf("в index.html отсутствует путь %q из missionFS", p)
		}
	}
	for _, gadget := range gadgets {
		if !strings.Contains(page, "\""+gadget+"\"") {
			t.Errorf("в index.html отсутствует гаджет %q из game.go", gadget)
		}
	}
}

func TestOfflineNameLimitMatchesServer(t *testing.T) {
	page := pageHTML(t)
	// Оба режима ограничивают имя 24 символами.
	if !strings.Contains(page, "slice(0, 24)") {
		t.Error("в index.html изменилось ограничение имени (ожидалось 24 символа)")
	}
}

func TestOfflineLinuxTrainerMatchesServer(t *testing.T) {
	page := pageHTML(t)
	if !strings.Contains(page, "L_MISSIONS") || !strings.Contains(page, "ОТДЕЛ ИБ · РАССЛЕДОВАНИЕ") {
		t.Fatal("в index.html нет офлайн-копии тренажёра Linux")
	}
	for _, m := range linuxMissions {
		if !strings.Contains(page, `title:"`+m.Title+`"`) {
			t.Errorf("в index.html нет задания %q", m.Title)
		}
	}
	for _, api := range []string{"/api/linux/state", "/api/linux/command", "/api/linux/reset"} {
		if !strings.Contains(page, api) {
			t.Errorf("в index.html не используется %s", api)
		}
	}
}

// По образцу tools/check.js из ob-master: страница не должна содержать
// ссылки на несуществующие id и несбалансированные теги.
func TestHTMLStructure(t *testing.T) {
	page := pageHTML(t)

	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(`\sid="([^"]+)"`).FindAllStringSubmatch(page, -1) {
		ids[m[1]] = true
	}
	used := regexp.MustCompile(`getElementById\("([^"]+)"\)`)
	for _, m := range used.FindAllStringSubmatch(page, -1) {
		if !ids[m[1]] {
			t.Errorf("JS обращается к несуществующему id %q", m[1])
		}
	}

	for _, tag := range []string{"div", "section", "button", "style", "script", "span", "form", "p"} {
		open := len(regexp.MustCompile(`<`+tag+`[ >]`).FindAllString(page, -1))
		close := strings.Count(page, "</"+tag+">")
		if open != close {
			t.Errorf("тег <%s>: открыто %d, закрыто %d", tag, open, close)
		}
	}
}
