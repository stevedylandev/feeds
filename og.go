package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// errRenderBusy signals that the render semaphore was saturated and the request
// gave up rather than piling on. Mapped to HTTP 503 by the OG handler.
var errRenderBusy = errors.New("render busy")

const (
	ogWidth  = 1200
	ogHeight = 630
	ogMargin = 80.0
)

var (
	fontOnce              sync.Once
	fontRegular, fontBold *opentype.Font
	fontErr               error
)

// loadFonts parses the embedded CommitMono OTFs once. They use CFF outlines,
// which the freetype loader gg ships with can't read, so we parse them with
// x/image/font/opentype (sfnt) instead.
func loadFonts() {
	fontOnce.Do(func() {
		rb, err := appFS.ReadFile("static/fonts/CommitMono-400-Regular.otf")
		if err != nil {
			fontErr = err
			return
		}
		bb, err := appFS.ReadFile("static/fonts/CommitMono-700-Regular.otf")
		if err != nil {
			fontErr = err
			return
		}
		if fontRegular, err = opentype.Parse(rb); err != nil {
			fontErr = err
			return
		}
		if fontBold, err = opentype.Parse(bb); err != nil {
			fontErr = err
			return
		}
	})
}

// newFace builds a font.Face at the given size. Faces are created per call
// because opentype faces are not safe for concurrent use.
func newFace(bold bool, size float64) (font.Face, error) {
	loadFonts()
	if fontErr != nil {
		return nil, fontErr
	}
	src := fontRegular
	if bold {
		src = fontBold
	}
	return opentype.NewFace(src, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
}

func (a *App) ogImageHandler(w http.ResponseWriter, r *http.Request) {
	urls := feedURLsFromRequest(r)
	key := ogCacheKey(urls)

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	// getOrRender serves a cached PNG or, on a miss, runs the render once even
	// under concurrent requests for the same key.
	png, err := a.Images.getOrRender(key, func() ([]byte, error) {
		title, desc := "Feeds", "Experience RSS feeds"
		if len(urls) > 0 {
			items, titles := previewURLs(ctx, urls, 0, a.Cache, a.Log)
			title, desc = feedMeta(urls, titles, len(items))
		}

		// Bound concurrent renders to cap CPU and peak memory (each render
		// allocates a ~3MB bitmap). Overloaded requests bail rather than pile up.
		select {
		case a.renderSem <- struct{}{}:
			defer func() { <-a.renderSem }()
		case <-ctx.Done():
			return nil, errRenderBusy
		}
		return renderOGImage(title, desc)
	})
	switch {
	case err == errRenderBusy:
		http.Error(w, "render busy", http.StatusServiceUnavailable)
		return
	case err != nil:
		a.Log.Error("og render failed", "err", err)
		http.Error(w, "og render error", http.StatusInternalServerError)
		return
	}
	writePNG(w, png)
}

// ogCacheKey normalizes a URL set into a stable cache key (order-independent).
func ogCacheKey(urls []string) string {
	sorted := append([]string(nil), urls...)
	sort.Strings(sorted)
	return strings.Join(sorted, "\n")
}

func writePNG(w http.ResponseWriter, png []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(png)
}

// renderOGImage draws a 1200x630 social card with the feed title and a subtitle.
func renderOGImage(title, desc string) ([]byte, error) {
	dc := gg.NewContext(ogWidth, ogHeight)
	dc.SetHexColor("#ffffff")
	dc.Clear()

	// "FEEDS" wordmark.
	wordmark, err := newFace(true, 34)
	if err != nil {
		return nil, err
	}
	dc.SetFontFace(wordmark)
	dc.SetHexColor("#1a1a1a")
	dc.DrawString("FEEDS", ogMargin, 120)

	// Title, wrapped, truncated so it can't run into the subtitle.
	titleFace, err := newFace(true, 68)
	if err != nil {
		return nil, err
	}
	dc.SetFontFace(titleFace)
	dc.SetHexColor("#1a1a1a")
	dc.DrawStringWrapped(truncate(title, 90), ogMargin, 200, 0, 0, ogWidth-2*ogMargin, 1.35, gg.AlignLeft)

	// Subtitle pinned near the bottom.
	subFace, err := newFace(false, 34)
	if err != nil {
		return nil, err
	}
	dc.SetFontFace(subFace)
	dc.SetHexColor("#6b6b6b")
	dc.DrawString(desc, ogMargin, ogHeight-ogMargin)

	var buf bytes.Buffer
	if err := dc.EncodePNG(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func truncate(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}
