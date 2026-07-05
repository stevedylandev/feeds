package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed templates/*.html static/*
var appFS embed.FS

type App struct {
	Log       *slog.Logger
	Templates *template.Template
	BaseURL   string
}

type templateItem struct {
	Title         string
	Link          string
	Author        string
	FormattedDate string
}

type indexPageData struct {
	BaseURL  string
	Items    []templateItem
	FeedURLs []string
	Error    string
}

func (a *App) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", a.indexHandler)
	mux.HandleFunc("GET /api/resolve", a.resolveHandler)
	mux.HandleFunc("GET /static/", embeddedHandler(appFS, "static"))
	return mux
}

// embeddedHandler serves files from an embed.FS under the given URL prefix.
func embeddedHandler(fs embed.FS, prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/"+prefix+"/")
		path := filepath.ToSlash(filepath.Join(prefix, name))
		data, err := fs.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		_, _ = w.Write(data)
	}
}

// render executes a named template into w. Errors are logged and surfaced as HTTP 500.
func render(t *template.Template, w http.ResponseWriter, name string, data any, log *slog.Logger) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		if log != nil {
			log.Error("template render failed", "name", name, "err", err)
		}
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// writeJSON writes data as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// writeError writes a JSON error response of the form {"error": msg}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

// getenv returns the trimmed value of key or fallback when unset/blank.
func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// loadDotEnv reads KEY=VALUE pairs from a .env file in the working directory
// and sets them as environment variables, without overriding vars already set.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
}
