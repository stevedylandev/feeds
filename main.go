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
		Cache:     newFeedCache(5*time.Minute, 512),
		Images:    newImageCache(10*time.Minute, 512),
		renderSem: make(chan struct{}, renderSlots),
	}

	addr := getenv("HOST", "0.0.0.0") + ":" + getenv("PORT", "3000")
	logger.Info("feeds server running", "addr", addr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           limitInFlight(app.routes(), 300),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
