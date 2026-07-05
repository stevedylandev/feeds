package main

import (
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"time"

	_ "golang.org/x/crypto/x509roots/fallback"
)

func main() {
	loadDotEnv(".env")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	tmpl := template.Must(template.New("").ParseFS(appFS, "templates/*.html"))
	renderSlots := runtime.NumCPU()
	app := &App{
		Log:       logger,
		Templates: tmpl,
		BaseURL:   getenv("BASE_URL", "http://localhost:3000"),
		Cache:     newFeedCache(5 * time.Minute),
		Images:    newImageCache(10*time.Minute, 512),
		renderSem: make(chan struct{}, renderSlots),
	}

	addr := getenv("HOST", "0.0.0.0") + ":" + getenv("PORT", "3000")
	logger.Info("feeds server running", "addr", addr)
	if err := http.ListenAndServe(addr, app.routes()); err != nil {
		log.Fatal(err)
	}
}
