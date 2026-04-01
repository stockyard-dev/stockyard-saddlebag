package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/stockyard-dev/stockyard-saddlebag/internal/store"
)

type Server struct {
	db     *store.DB
	mux    *http.ServeMux
	port   int
	limits Limits
}

func New(db *store.DB, port int, limits Limits) *Server {
	s := &Server{db: db, mux: http.NewServeMux(), port: port, limits: limits}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /api/upload", s.handleUpload)
	s.mux.HandleFunc("GET /api/files", s.handleListFiles)
	s.mux.HandleFunc("GET /api/files/{id}", s.handleGetFile)
	s.mux.HandleFunc("DELETE /api/files/{id}", s.handleDeleteFile)
	s.mux.HandleFunc("GET /d/{id}", s.handleDownload)
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /ui", s.handleUI)
	s.mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"product": "stockyard-saddlebag", "version": "0.1.0"})
	})
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("[saddlebag] listening on %s", addr)
	// Cleanup goroutine
	go func() {
		for {
			time.Sleep(1 * time.Hour)
			n, _ := s.db.Cleanup()
			if n > 0 {
				log.Printf("[cleanup] removed %d expired files", n)
			}
		}
	}()
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if s.limits.MaxFiles > 0 && LimitReached(s.limits.MaxFiles, s.db.TotalFiles()) {
		writeJSON(w, 402, map[string]string{"error": fmt.Sprintf("free tier limit: %d files", s.limits.MaxFiles), "upgrade": "https://stockyard.dev/saddlebag/"})
		return
	}

	maxSize := int64(100 * 1024 * 1024) // 100MB
	if s.limits.MaxSizeMB > 0 {
		maxSize = int64(s.limits.MaxSizeMB) * 1024 * 1024
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)

	if err := r.ParseMultipartForm(maxSize); err != nil {
		writeJSON(w, 400, map[string]string{"error": "file too large or invalid form"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "file field required"})
		return
	}
	defer file.Close()

	password := r.FormValue("password")
	if password != "" && !s.limits.PasswordLock {
		password = "" // silently ignore on free
	}
	expiresAt := r.FormValue("expires_at")
	maxDL := 0
	if v := r.FormValue("max_downloads"); v != "" {
		fmt.Sscanf(v, "%d", &maxDL)
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	rec, err := s.db.CreateFile(header.Filename, contentType, header.Size, maxDL, password, expiresAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	dst, err := os.Create(filepath.Join(s.db.FilesDir(), rec.ID))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to save file"})
		return
	}
	defer dst.Close()
	written, _ := io.Copy(dst, file)
	log.Printf("[upload] %s (%s, %d bytes)", rec.Filename, rec.ID, written)

	downloadURL := fmt.Sprintf("http://localhost:%d/d/%s", s.port, rec.ID)
	writeJSON(w, 201, map[string]any{"file": rec, "download_url": downloadURL})
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	f, err := s.db.GetFile(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if s.db.IsExpired(f) {
		http.Error(w, "This file has expired", 410)
		return
	}
	pw := s.db.GetFilePassword(id)
	if pw != "" {
		provided := r.URL.Query().Get("password")
		if provided != pw {
			writeJSON(w, 401, map[string]string{"error": "password required — add ?password=YOUR_PASSWORD"})
			return
		}
	}

	filePath := filepath.Join(s.db.FilesDir(), id)
	fd, err := os.Open(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer fd.Close()

	s.db.IncrementDownloads(id)
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, f.Filename))
	io.Copy(w, fd)
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	files, _ := s.db.ListFiles()
	if files == nil { files = []store.File{} }
	writeJSON(w, 200, map[string]any{"files": files, "count": len(files)})
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	f, err := s.db.GetFile(r.PathValue("id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "file not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"file": f, "download_url": fmt.Sprintf("http://localhost:%d/d/%s", s.port, f.ID)})
}

func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	s.db.DeleteFile(r.PathValue("id"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.db.Stats()) }
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
