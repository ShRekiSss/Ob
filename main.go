package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed index.html
var assets embed.FS

const (
	sessionCookie  = "station_sid"
	sessionTTL     = 24 * time.Hour
	version        = "1.4.2"
	latestRelease  = "https://api.github.com/repos/ShRekiSss/Ob/releases/latest"
	applyUpdateArg = "--apply-update"
)

var startedAt = time.Now()

type server struct {
	mu       sync.Mutex
	sessions map[string]sessionEntry

	// ttl и now вынесены в поля, чтобы тесты могли управлять временем.
	ttl time.Duration
	now func() time.Time
}

type sessionEntry struct {
	sess *Session
	seen time.Time
}

func newServer() *server {
	return &server{
		sessions: make(map[string]sessionEntry),
		ttl:      sessionTTL,
		now:      time.Now,
	}
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("POST /api/command", s.handleCommand)
	mux.HandleFunc("POST /api/reset", s.handleReset)
	return s.logRequests(mux)
}

// logRequests пишет в консоль сервера строку на каждый запрос:
// метод, путь, статус и время обработки — как в обычном серверном логе.
func (s *server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s → %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(status int) {
	rec.status = status
	rec.ResponseWriter.WriteHeader(status)
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"version": version,
		"uptime":  time.Since(startedAt).Round(time.Second).String(),
	})
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	page, err := assets.ReadFile("index.html")
	if err != nil {
		http.Error(w, "не удалось загрузить страницу", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(page)
}

func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	sess := s.session(w, r)
	writeJSON(w, http.StatusOK, sess.State())
}

func (s *server) handleCommand(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Command string `json:"command"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "некорректный запрос: ожидается JSON вида {\"command\": \"ls\"}",
		})
		return
	}

	sess := s.session(w, r)
	writeJSON(w, http.StatusOK, sess.Execute(req.Command))
}

func (s *server) handleReset(w http.ResponseWriter, r *http.Request) {
	sess := s.session(w, r)
	writeJSON(w, http.StatusOK, sess.Reset())
}

func (s *server) session(w http.ResponseWriter, r *http.Request) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()

	if cookie, err := r.Cookie(sessionCookie); err == nil {
		if entry, ok := s.sessions[cookie.Value]; ok && now.Sub(entry.seen) < s.ttl {
			entry.seen = now
			s.sessions[cookie.Value] = entry
			return entry.sess
		}
		delete(s.sessions, cookie.Value)
	}

	s.sweep(now)

	id := randomID()
	sess := NewSession()
	s.sessions[id] = sessionEntry{sess: sess, seen: now}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		MaxAge:   int(s.ttl / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return sess
}

// sweep удаляет сессии, к которым не обращались дольше ttl.
// Вызывается под s.mu при создании новой сессии, чтобы карта не росла
// бесконечно от случайных или брошенных cookie.
func (s *server) sweep(now time.Time) {
	for id, entry := range s.sessions {
		if now.Sub(entry.seen) >= s.ttl {
			delete(s.sessions, id)
		}
	}
}

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("не удалось создать идентификатор сессии: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("ошибка записи JSON: %v", err)
	}
}

// ── Самообновление ──────────────────────────────────────────────────────
// При старте exe тихо спрашивает GitHub о последнем релизе. Если он новее,
// скачивает helios7.exe, запускает отдельный процесс-установщик и выходит;
// установщик дожидается выхода старого процесса, меняет файл и запускает
// новую версию. Никаких батников и ручных скачиваний не требуется.

type releaseInfo struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func versionParts(s string) []int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	parts := strings.Split(s, ".")
	out := make([]int, len(parts))
	for i, part := range parts {
		n := 0
		_, _ = fmt.Sscanf(part, "%d", &n)
		out[i] = n
	}
	return out
}

func versionNewer(latest, current string) bool {
	a, b := versionParts(latest), versionParts(current)
	for i := 0; i < max(len(a), len(b)); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func updateClient(timeout time.Duration) *http.Client {
	return updateClientMode(timeout, os.Getenv("NO_UPDATE_TLS_VERIFY") == "1")
}

func updateClientMode(timeout time.Duration, insecure bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}

	// Если сеть использует свой корневой сертификат (корпоративный прокси,
	// антивирус), его можно положить в ca.pem рядом с helios7.exe.
	if exe, exeErr := os.Executable(); exeErr == nil {
		if pem, pemErr := os.ReadFile(filepath.Join(filepath.Dir(exe), "ca.pem")); pemErr == nil {
			if pool.AppendCertsFromPEM(pem) {
				log.Print("загружен дополнительный сертификат ca.pem")
			}
		}
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}
	if insecure {
		tlsConfig.InsecureSkipVerify = true //nolint:gosec -- явный обход для сетей с подменой TLS
	}
	transport.TLSClientConfig = tlsConfig
	return &http.Client{Timeout: timeout, Transport: transport}
}

func isCertificateError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "x509") || strings.Contains(msg, "certificate")
}

func fetchLatestReleaseWith(client *http.Client) (releaseInfo, error) {
	var rel releaseInfo
	req, err := http.NewRequest(http.MethodGet, latestRelease, nil)
	if err != nil {
		return rel, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "helios7-updater")
	res, err := client.Do(req)
	if err != nil {
		return rel, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return rel, fmt.Errorf("GitHub вернул %s", res.Status)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&rel); err != nil {
		return rel, err
	}
	return rel, nil
}

// fetchLatestRelease пробует обычную проверку сертификата, а если сеть
// подменяет TLS (корпоративный прокси/антивирус), повторяет без проверки.
func fetchLatestRelease() (releaseInfo, bool, error) {
	envInsecure := os.Getenv("NO_UPDATE_TLS_VERIFY") == "1"

	rel, err := fetchLatestReleaseWith(updateClient(8 * time.Second))
	if err == nil {
		return rel, envInsecure, nil
	}
	if !isCertificateError(err) || envInsecure {
		return rel, envInsecure, err
	}

	log.Print("сертификат GitHub не прошёл проверку — повторяем с обходом TLS")
	rel, err = fetchLatestReleaseWith(updateClientMode(8*time.Second, true))
	if err != nil {
		return rel, false, err
	}
	return rel, true, nil
}

func maybeUpdate() {
	if os.Getenv("NO_UPDATE") == "1" {
		return
	}
	// Обновляемся только в Windows-exe; при разработке на Linux это отключено.
	if runtime.GOOS != "windows" && os.Getenv("FORCE_UPDATE") != "1" {
		return
	}

	rel, insecureTLS, err := fetchLatestRelease()
	if err != nil {
		log.Printf("проверка обновлений пропущена: %v", err)
		if strings.Contains(strings.ToLower(err.Error()), "certificate") {
			log.Print("совет: сверь дату и время в Windows; если сеть подменяет сертификат, положи его корень в ca.pem рядом с exe или задай NO_UPDATE_TLS_VERIFY=1")
		}
		return
	}
	if !versionNewer(rel.TagName, version) {
		return
	}

	var assetURL string
	for _, asset := range rel.Assets {
		if asset.Name == "helios7.exe" && strings.HasPrefix(asset.BrowserDownloadURL, "https://github.com/") {
			assetURL = asset.BrowserDownloadURL
			break
		}
	}
	if assetURL == "" {
		log.Printf("вышла версия %s, но exe в релизе не найден", rel.TagName)
		return
	}

	exe, err := os.Executable()
	if err != nil {
		log.Printf("проверка обновлений пропущена: %v", err)
		return
	}
	dir := filepath.Dir(exe)
	updatePath := filepath.Join(dir, "helios7-update.exe")
	installPath := filepath.Join(dir, "helios7-install.exe")

	client := updateClientMode(40*time.Second, insecureTLS)
	res, err := client.Get(assetURL)
	if err != nil {
		log.Printf("скачивание обновления не удалось: %v", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Printf("скачивание обновления вернуло %s", res.Status)
		return
	}

	f, err := os.Create(updatePath)
	if err != nil {
		log.Printf("не удалось сохранить обновление: %v", err)
		return
	}
	written, copyErr := io.Copy(f, io.LimitReader(res.Body, 64<<20))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil || written >= 64<<20 {
		log.Printf("не удалось сохранить обновление: %v %v (байт: %d)", copyErr, closeErr, written)
		_ = os.Remove(updatePath)
		return
	}

	data, err := os.ReadFile(updatePath)
	if err != nil || len(data) < 2 || data[0] != 'M' || data[1] != 'Z' {
		log.Print("скачанный файл не похож на Windows-exe, обновление отменено")
		_ = os.Remove(updatePath)
		return
	}
	if err := os.WriteFile(installPath, data, 0700); err != nil {
		_ = os.Remove(updatePath)
		log.Printf("не удалось подготовить установщик: %v", err)
		return
	}

	log.Printf("найдена версия %s — устанавливаем и перезапускаемся…", rel.TagName)
	cmd := exec.Command(installPath, applyUpdateArg, exe, updatePath, strconv.Itoa(os.Getpid()))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Printf("не удалось запустить установщик: %v", err)
		_ = os.Remove(installPath)
		_ = os.Remove(updatePath)
		return
	}
	os.Exit(0)
}

func applyUpdate(target, updatePath string, pid int) {
	log.Printf("установка обновления (процесс %d)…", pid)
	// Даём старому процессу выйти и освободить файл.
	time.Sleep(1500 * time.Millisecond)

	for i := 0; i < 30; i++ {
		err := os.Remove(target)
		if err == nil || os.IsNotExist(err) {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	if err := os.Rename(updatePath, target); err != nil {
		log.Printf("не удалось установить обновление: %v", err)
		return
	}
	log.Printf("обновление установлено, запускаю новую версию")
	if err := exec.Command(target).Start(); err != nil {
		log.Printf("не удалось запустить обновлённую версию: %v", err)
	}
}

func main() {
	if len(os.Args) == 5 && os.Args[1] == applyUpdateArg {
		pid, _ := strconv.Atoi(os.Args[4])
		applyUpdate(os.Args[2], os.Args[3], pid)
		return
	}

	maybeUpdate()

	addr := os.Getenv("ADDR")
	if addr == "" {
		// Слушаем только loopback: так игра не просит доступ в брандмауэре
		// Windows и не торчит в локальную сеть.
		addr = "127.0.0.1:8080"
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           newServer().handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	displayAddr := addr
	if strings.HasPrefix(displayAddr, ":") {
		displayAddr = "localhost" + displayAddr
	}
	pageURL := "http://" + displayAddr
	log.Printf("╔════════════════════════════════════════════╗")
	log.Printf("║  ГЕЛИОС-7  //  сервер связи  //  v%s        ║", version)
	log.Printf("╚════════════════════════════════════════════╝")
	log.Printf("станция на связи: %s", pageURL)
	log.Printf("проверка работоспособности: %s/api/health", pageURL)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		log.Print("завершение работы, закрываем соединения…")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	// При двойном щелчке по exe на Windows сразу открываем браузер.
	if runtime.GOOS == "windows" {
		go func() {
			time.Sleep(400 * time.Millisecond)
			_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", pageURL).Start()
		}()
	}

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("не удалось запустить сервер: %v", err)
		log.Printf("скорее всего порт %s занят. Задай другой, например:", addr)
		log.Printf("  Windows:  set ADDR=127.0.0.1:9090 && helios7.exe")
		log.Printf("  Linux:    ADDR=127.0.0.1:9090 ./helios7")
		if runtime.GOOS == "windows" {
			fmt.Print("Нажми Enter, чтобы закрыть окно…")
			_, _ = fmt.Scanln()
		}
		os.Exit(1)
	}
}
