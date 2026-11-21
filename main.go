package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed index.html
var assets embed.FS

const (
	sessionCookie = "station_sid"
	sessionTTL    = 24 * time.Hour
	version       = "1.2.0"
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

func main() {
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
