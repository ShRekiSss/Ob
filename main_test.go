package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestHTTPMissionFlow(t *testing.T) {
	srv := httptest.NewServer(newServer().handler())
	defer srv.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	res, err := client.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET / вернул %d", res.StatusCode)
	}
	if !strings.Contains(string(page), `<pre id="terminal"`) {
		t.Fatalf("страница не содержит поле вывода терминала")
	}
	if !strings.Contains(string(page), `<form`) {
		t.Fatalf("страница не содержит форму ввода")
	}

	var state Snapshot
	getJSON(t, client, srv.URL+"/api/state", &state)
	if state.HP != 3 || state.Cwd != "/" || state.Won {
		t.Fatalf("начальное состояние = %+v", state)
	}

	var result Result
	postJSON(t, client, srv.URL+"/api/command", map[string]string{"command": "ls /station"}, &result)
	if result.State.HP != 2 {
		t.Fatalf("после ls /station HP = %d", result.State.HP)
	}
	if !strings.Contains(result.Output, "brief.txt") {
		t.Fatalf("ls /station не показал brief.txt: %q", result.Output)
	}

	postJSON(t, client, srv.URL+"/api/command", map[string]string{"command": "cat /station/brief.txt"}, &result)
	if result.State.HP != 1 {
		t.Fatalf("после cat HP = %d", result.State.HP)
	}
	if !strings.Contains(result.Output, "READY") {
		t.Fatalf("brief.txt не содержит маркер: %q", result.Output)
	}

	postJSON(t, client, srv.URL+"/api/command", map[string]string{"command": "grep READY /station/events.log"}, &result)
	if result.State.HP != 0 || !result.State.Won {
		t.Fatalf("после третьей цели состояние = %+v", result.State)
	}

	postJSON(t, client, srv.URL+"/api/reset", nil, &state)
	if state.HP != 3 || state.Cwd != "/" || state.Won || state.Done != 0 {
		t.Fatalf("после сброса состояние = %+v", state)
	}
}

func TestHTTPSessionsAreIndependent(t *testing.T) {
	srv := httptest.NewServer(newServer().handler())
	defer srv.Close()

	jarA, _ := cookiejar.New(nil)
	jarB, _ := cookiejar.New(nil)
	clientA := &http.Client{Jar: jarA}
	clientB := &http.Client{Jar: jarB}

	var result Result
	postJSON(t, clientA, srv.URL+"/api/command", map[string]string{"command": "ls /station"}, &result)
	if result.State.HP != 2 {
		t.Fatalf("игрок A: HP = %d", result.State.HP)
	}

	var state Snapshot
	getJSON(t, clientB, srv.URL+"/api/state", &state)
	if state.HP != 3 {
		t.Fatalf("игрок B получил состояние игрока A: %+v", state)
	}
}

func TestHTTPRejectsBrokenJSON(t *testing.T) {
	srv := httptest.NewServer(newServer().handler())
	defer srv.Close()

	res, err := http.Post(srv.URL+"/api/command", "application/json", bytes.NewBufferString("{"))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("некорректный JSON вернул %d", res.StatusCode)
	}
}

func TestHTTPUnknownPath(t *testing.T) {
	srv := httptest.NewServer(newServer().handler())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("неизвестный путь вернул %d", res.StatusCode)
	}
}

func TestIsCertificateError(t *testing.T) {
	if !isCertificateError(errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority")) {
		t.Fatal("ошибка сертификата не распознана")
	}
	if isCertificateError(errors.New("dial tcp: connection refused")) {
		t.Fatal("обычная сетевая ошибка принята за сертификат")
	}
	if isCertificateError(nil) {
		t.Fatal("nil принят за ошибку")
	}
}

func TestVersionNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v1.4.0", "1.3.0", true},
		{"v1.3.1", "1.3.0", true},
		{"v1.3.0", "1.3.0", false},
		{"v1.2.9", "1.3.0", false},
		{"2.0.0", "1.9.9", true},
	}
	for _, c := range cases {
		if got := versionNewer(c.latest, c.current); got != c.want {
			t.Errorf("versionNewer(%q, %q) = %v, хочется %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestHTTPClearAndUnicodeName(t *testing.T) {
	srv := httptest.NewServer(newServer().handler())
	defer srv.Close()
	// Cookie-jar обязателен: без него каждый запрос открывал бы новую сессию.
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	postJSON(t, client, srv.URL+"/api/command", map[string]string{"command": "ls /station"}, &Result{})

	var cleared Result
	postJSON(t, client, srv.URL+"/api/command", map[string]string{"command": "clear"}, &cleared)
	if cleared.Output != "" || cleared.State.HP != 2 || cleared.State.Done != 1 {
		t.Fatalf("clear через API изменил состояние: %+v", cleared)
	}

	var named Result
	postJSON(t, client, srv.URL+"/api/command",
		map[string]string{"command": "name " + strings.Repeat("Я", 30)}, &named)
	if got := []rune(named.State.Name); len(got) != 24 {
		t.Fatalf("имя через API обрезано до %d символов: %q", len(got), named.State.Name)
	}
	if !utf8.ValidString(named.State.Name) {
		t.Fatalf("имя через API содержит битый UTF-8: %q", named.State.Name)
	}
}

func TestHTTPLinuxTrainerFlow(t *testing.T) {
	srv := httptest.NewServer(newServer().handler())
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	var state LinuxSnapshot
	getJSON(t, client, srv.URL+"/api/linux/state", &state)
	if state.Mission != 1 || state.Total != 13 {
		t.Fatalf("начальное состояние тренажёра: %+v", state)
	}

	var res LinuxResult
	postJSON(t, client, srv.URL+"/api/linux/command", map[string]string{"command": "ls /station"}, &res)
	if !strings.Contains(res.Output, "brief.txt") || res.State.Done != 1 {
		t.Fatalf("ls /station: %+v", res)
	}
	postJSON(t, client, srv.URL+"/api/linux/command", map[string]string{"command": "cat /station/brief.txt"}, &res)
	if res.State.Done != 2 || res.State.Score != 200 {
		t.Fatalf("после cat: %+v", res.State)
	}

	postJSON(t, client, srv.URL+"/api/linux/reset", nil, &state)
	if state.Done != 0 || state.Score != 0 {
		t.Fatalf("после сброса тренажёра: %+v", state)
	}
}

func TestHTTPHealth(t *testing.T) {
	srv := httptest.NewServer(newServer().handler())
	defer srv.Close()

	var body struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
		Uptime  string `json:"uptime"`
	}
	getJSON(t, &http.Client{}, srv.URL+"/api/health", &body)

	if !body.OK || body.Version != version || body.Uptime == "" {
		t.Fatalf("/api/health вернул %+v", body)
	}
}

func TestStaleSessionsAreEvicted(t *testing.T) {
	srv := newServer()
	srv.ttl = time.Minute
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	srv.now = func() time.Time { return now }

	ts := httptest.NewServer(srv.handler())
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	jarA, _ := cookiejar.New(nil)
	jarB, _ := cookiejar.New(nil)
	clientA := &http.Client{Jar: jarA}
	clientB := &http.Client{Jar: jarB}

	var state Snapshot
	getJSON(t, clientA, ts.URL+"/api/state", &state)
	getJSON(t, clientB, ts.URL+"/api/state", &state)

	if got := len(srv.sessions); got != 2 {
		t.Fatalf("активных сессий = %d, ожидалось 2", got)
	}

	oldID := jarA.Cookies(tsURL)[0].Value
	now = now.Add(2 * srv.ttl)

	getJSON(t, clientA, ts.URL+"/api/state", &state)

	if got := len(srv.sessions); got != 1 {
		t.Fatalf("после очистки сессий = %d, ожидалась 1", got)
	}
	if newID := jarA.Cookies(tsURL)[0].Value; newID == oldID {
		t.Fatalf("просроченная сессия не заменена: %q", newID)
	}
}

func getJSON(t *testing.T, client *http.Client, url string, out any) {
	t.Helper()

	res, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s вернул %d", url, res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		t.Fatal(err)
	}
}

func postJSON(t *testing.T, client *http.Client, url string, body any, out any) {
	t.Helper()

	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = bytes.NewReader(data)
	} else {
		payload = bytes.NewReader(nil)
	}

	res, err := client.Post(url, "application/json", payload)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(res.Body)
		t.Fatalf("POST %s вернул %d: %s", url, res.StatusCode, data)
	}
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
}
