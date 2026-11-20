package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

//go:embed index.html
var assets embed.FS

const sessionCookie = "station_sid"

type server struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func newServer() *server {
	return &server{sessions: make(map[string]*Session)}
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("POST /api/command", s.handleCommand)
	mux.HandleFunc("POST /api/reset", s.handleReset)
	return mux
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

	if cookie, err := r.Cookie(sessionCookie); err == nil {
		if sess, ok := s.sessions[cookie.Value]; ok {
			return sess
		}
	}

	id := randomID()
	sess := NewSession()
	s.sessions[id] = sess

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		MaxAge:   24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return sess
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
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: newServer().handler(),
	}

	displayAddr := addr
	if strings.HasPrefix(displayAddr, ":") {
		displayAddr = "localhost" + displayAddr
	}
	log.Printf("станция «Гелиос-7» на связи: http://%s", displayAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
