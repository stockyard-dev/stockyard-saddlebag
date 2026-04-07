package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/stockyard-dev/stockyard-saddlebag/internal/store"
)

const resourceName = "backups"

type Server struct {
	db      *store.DB
	mux     *http.ServeMux
	limits  Limits
	dataDir string
	pCfg    map[string]json.RawMessage
}

func New(db *store.DB, limits Limits, dataDir string) *Server {
	s := &Server{
		db:      db,
		mux:     http.NewServeMux(),
		limits:  limits,
		dataDir: dataDir,
	}
	s.loadPersonalConfig()

	// Backups CRUD
	s.mux.HandleFunc("GET /api/backups", s.list)
	s.mux.HandleFunc("POST /api/backups", s.create)
	s.mux.HandleFunc("GET /api/backups/{id}", s.get)
	s.mux.HandleFunc("PUT /api/backups/{id}", s.update)
	s.mux.HandleFunc("DELETE /api/backups/{id}", s.del)

	// Run completion / quick status update
	s.mux.HandleFunc("POST /api/backups/{id}/run", s.markRun)

	// Stats / health
	s.mux.HandleFunc("GET /api/stats", s.stats)
	s.mux.HandleFunc("GET /api/health", s.health)

	// Personalization
	s.mux.HandleFunc("GET /api/config", s.configHandler)

	// Extras
	s.mux.HandleFunc("GET /api/extras/{resource}", s.listExtras)
	s.mux.HandleFunc("GET /api/extras/{resource}/{id}", s.getExtras)
	s.mux.HandleFunc("PUT /api/extras/{resource}/{id}", s.putExtras)

	// Tier
	s.mux.HandleFunc("GET /api/tier", func(w http.ResponseWriter, r *http.Request) {
		wj(w, 200, map[string]any{
			"tier":        s.limits.Tier,
			"upgrade_url": "https://stockyard.dev/saddlebag/",
		})
	})

	// Dashboard
	s.mux.HandleFunc("GET /ui", s.dashboard)
	s.mux.HandleFunc("GET /ui/", s.dashboard)
	s.mux.HandleFunc("GET /", s.root)

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func wj(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func we(w http.ResponseWriter, code int, msg string) {
	wj(w, code, map[string]string{"error": msg})
}

func oe[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (s *Server) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/ui", 302)
}

// ─── personalization ──────────────────────────────────────────────

func (s *Server) loadPersonalConfig() {
	path := filepath.Join(s.dataDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("saddlebag: warning: could not parse config.json: %v", err)
		return
	}
	s.pCfg = cfg
	log.Printf("saddlebag: loaded personalization from %s", path)
}

func (s *Server) configHandler(w http.ResponseWriter, r *http.Request) {
	if s.pCfg == nil {
		wj(w, 200, map[string]any{})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.pCfg)
}

// ─── extras ───────────────────────────────────────────────────────

func (s *Server) listExtras(w http.ResponseWriter, r *http.Request) {
	resource := r.PathValue("resource")
	all := s.db.AllExtras(resource)
	out := make(map[string]json.RawMessage, len(all))
	for id, data := range all {
		out[id] = json.RawMessage(data)
	}
	wj(w, 200, out)
}

func (s *Server) getExtras(w http.ResponseWriter, r *http.Request) {
	resource := r.PathValue("resource")
	id := r.PathValue("id")
	data := s.db.GetExtras(resource, id)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(data))
}

func (s *Server) putExtras(w http.ResponseWriter, r *http.Request) {
	resource := r.PathValue("resource")
	id := r.PathValue("id")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		we(w, 400, "read body")
		return
	}
	var probe map[string]any
	if err := json.Unmarshal(body, &probe); err != nil {
		we(w, 400, "invalid json")
		return
	}
	if err := s.db.SetExtras(resource, id, string(body)); err != nil {
		we(w, 500, "save failed")
		return
	}
	wj(w, 200, map[string]string{"ok": "saved"})
}

// ─── backups ──────────────────────────────────────────────────────

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	filters := map[string]string{}
	if v := r.URL.Query().Get("status"); v != "" {
		filters["status"] = v
	}
	if q != "" || len(filters) > 0 {
		wj(w, 200, map[string]any{"backups": oe(s.db.Search(q, filters))})
		return
	}
	wj(w, 200, map[string]any{"backups": oe(s.db.List())})
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	if s.limits.MaxItems > 0 && s.db.Count() >= s.limits.MaxItems {
		we(w, 402, "Free tier limit reached. Upgrade at https://stockyard.dev/saddlebag/")
		return
	}
	var e store.Backup
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		we(w, 400, "invalid json")
		return
	}
	if e.Name == "" {
		we(w, 400, "name required")
		return
	}
	if err := s.db.Create(&e); err != nil {
		we(w, 500, "create failed")
		return
	}
	wj(w, 201, s.db.Get(e.ID))
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	e := s.db.Get(r.PathValue("id"))
	if e == nil {
		we(w, 404, "not found")
		return
	}
	wj(w, 200, e)
}

// update accepts a partial backup. Original handler had the empty/zero
// preserve pattern for all 7 fields including 'if size_bytes == 0
// preserve' which prevented zero-sized backup runs from being recorded.
func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	existing := s.db.Get(r.PathValue("id"))
	if existing == nil {
		we(w, 404, "not found")
		return
	}

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		we(w, 400, "invalid json")
		return
	}

	patch := *existing
	if v, ok := raw["name"]; ok {
		var s string
		json.Unmarshal(v, &s)
		if s != "" {
			patch.Name = s
		}
	}
	if v, ok := raw["source"]; ok {
		json.Unmarshal(v, &patch.Source)
	}
	if v, ok := raw["destination"]; ok {
		json.Unmarshal(v, &patch.Destination)
	}
	if v, ok := raw["size_bytes"]; ok {
		json.Unmarshal(v, &patch.SizeBytes)
	}
	if v, ok := raw["status"]; ok {
		var s string
		json.Unmarshal(v, &s)
		if s != "" {
			patch.Status = s
		}
	}
	if v, ok := raw["schedule"]; ok {
		json.Unmarshal(v, &patch.Schedule)
	}
	if v, ok := raw["last_run_at"]; ok {
		json.Unmarshal(v, &patch.LastRunAt)
	}

	if err := s.db.Update(&patch); err != nil {
		we(w, 500, "update failed")
		return
	}
	wj(w, 200, s.db.Get(patch.ID))
}

// markRun is a dedicated endpoint for the most common automation:
// after a backup runs, the runner POSTs the new status, size, and
// timestamp without having to read+merge the full record.
func (s *Server) markRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.db.Get(id) == nil {
		we(w, 404, "not found")
		return
	}
	var req struct {
		Status    string `json:"status"`
		SizeBytes int    `json:"size_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		we(w, 400, "invalid json")
		return
	}
	if req.Status == "" {
		req.Status = "completed"
	}
	if err := s.db.MarkRun(id, req.Status, req.SizeBytes); err != nil {
		we(w, 500, "update failed")
		return
	}
	wj(w, 200, s.db.Get(id))
}

func (s *Server) del(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.db.Delete(id)
	s.db.DeleteExtras(resourceName, id)
	wj(w, 200, map[string]string{"deleted": "ok"})
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	wj(w, 200, s.db.Stats())
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	wj(w, 200, map[string]any{
		"status":  "ok",
		"service": "saddlebag",
		"backups": s.db.Count(),
	})
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
