package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/image/draw"

	"keepsake/internal/auth"
	"keepsake/internal/brightwheel"
	"keepsake/internal/downloader"
	"keepsake/internal/library"
	"keepsake/internal/log"
	"keepsake/internal/metadata"
)

// App is the Wails binding facade (thin layer over internal packages).
type App struct {
	ctx context.Context

	mu            sync.Mutex
	proxy         *auth.LoginProxy
	creds         *brightwheel.Credentials
	client        brightwheel.APIClient
	students      []brightwheel.Student
	engine        *downloader.Engine
	running       bool
	allowedFolder string
	demo          bool
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// --- Auth ---

// StartLogin starts the login proxy and returns the URL the WebView should
// navigate to. returnOrigin is the app origin to return to after login.
func (a *App) StartLogin(returnOrigin string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.proxy = auth.NewLoginProxy(func(creds brightwheel.Credentials) {
		a.mu.Lock()
		c := creds
		a.creds = &c
		a.client = brightwheel.NewClient(creds)
		a.mu.Unlock()
		runtime.EventsEmit(a.ctx, "auth:success")
	})
	return a.proxy.Start(returnOrigin)
}

// AuthStatus reports whether login has completed (polling fallback).
func (a *App) AuthStatus() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.creds != nil
}

// Logout clears the session.
func (a *App) Logout() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.creds = nil
	a.client = nil
	a.students = nil
	if a.proxy != nil {
		a.proxy.Stop()
		a.proxy = nil
	}
}

// --- Students ---

// FetchStudents loads the account's students (requires login).
func (a *App) FetchStudents() ([]brightwheel.Student, error) {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()
	if client == nil {
		return nil, fmt.Errorf("not logged in")
	}
	students, err := client.FetchStudents()
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.students = students
	a.mu.Unlock()
	return students, nil
}

// --- Folder / library ---

// ChooseFolder opens a directory picker and returns the selection.
func (a *App) ChooseFolder() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose download folder",
	})
	if err == nil && dir != "" {
		a.mu.Lock()
		a.allowedFolder = dir
		a.mu.Unlock()
	}
	return dir, err
}

// resolveInAllowedFolder confines filesystem bindings to the user-chosen
// download folder, so JS running in the WebView (e.g. the remote login
// page) cannot read or enumerate arbitrary local paths.
func (a *App) resolveInAllowedFolder(path string) (string, error) {
	a.mu.Lock()
	base := a.allowedFolder
	a.mu.Unlock()
	if base == "" {
		return "", fmt.Errorf("no download folder selected")
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if absPath != absBase && !strings.HasPrefix(absPath, absBase+string(os.PathSeparator)) {
		return "", fmt.Errorf("path outside the download folder")
	}
	return absPath, nil
}

// ScanFolder lists media files in a folder.
func (a *App) ScanFolder(folder string) ([]library.File, error) {
	dir, err := a.resolveInAllowedFolder(folder)
	if err != nil {
		return nil, err
	}
	return library.Scan(dir)
}

// thumbSem bounds concurrent thumbnail decoding (large images decode to
// tens of MB of RGBA each).
var thumbSem = make(chan struct{}, 4)

// IsDemo reports whether demo mode (pixelated media) is active.
func (a *App) IsDemo() bool { return a.demo }

// GetThumbnail returns a base64 JPEG data URL for an image file,
// downscaled to fit maxDim. Videos return an empty string.
func (a *App) GetThumbnail(path string, maxDim int) (string, error) {
	safePath, err := a.resolveInAllowedFolder(path)
	if err != nil {
		return "", err
	}
	if maxDim <= 0 {
		maxDim = 256
	}
	thumbSem <- struct{}{}
	defer func() { <-thumbSem }()
	f, err := os.Open(safePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return "", err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > maxDim || h > maxDim {
		if w >= h {
			h = h * maxDim / w
			w = maxDim
		} else {
			w = w * maxDim / h
			h = maxDim
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if a.demo {
		// Pixelate: shrink to a few pixels, then upscale with nearest
		// neighbor so no detail survives.
		pw, ph := w/17, h/17
		if pw < 2 {
			pw = 2
		}
		if ph < 2 {
			ph = 2
		}
		tiny := image.NewRGBA(image.Rect(0, 0, pw, ph))
		draw.CatmullRom.Scale(tiny, tiny.Bounds(), img, b, draw.Over, nil)
		draw.NearestNeighbor.Scale(dst, dst.Bounds(), tiny, tiny.Bounds(), draw.Over, nil)
	} else {
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 75}); err != nil {
		return "", err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// RemoteMediaItem is a not-yet-downloaded media item found in a
// student's activity feed, used for the grayscale preview grid.
type RemoteMediaItem struct {
	URL             string `json:"url"`
	StudentID       string `json:"student_id"`
	ExpectedName    string `json:"expected_name"`
	IsVideo         bool   `json:"is_video"`
	IsMoreIndicator bool   `json:"is_more_indicator,omitempty"`
	MoreCount       int    `json:"more_count,omitempty"`
}

// PreviewMedia fetches a capped sample of media items for the selected
// students so the UI can show grayscale previews before download.
func (a *App) PreviewMedia(studentIDs []string, startDate, endDate string) ([]RemoteMediaItem, error) {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()
	if client == nil {
		return nil, fmt.Errorf("not logged in")
	}

	start, end := parseDateRange(startDate, endDate)

	const maxItems = 10 // keep preview light; don't hammer their servers
	items := []RemoteMediaItem{}

	limitPerStudent := maxItems / len(studentIDs)
	if limitPerStudent < 1 {
		limitPerStudent = 1
	}

	add := func(rawURL, thumbURL, studentID, studentName, dateStr string, index int) {
		if rawURL == "" || len(items) >= maxItems {
			return
		}
		name := downloader.ExpectedFilename(studentName, dateStr, index, rawURL)
		if name == "" {
			return
		}
		previewURL := thumbURL
		if previewURL == "" {
			previewURL = rawURL
		}
		ext := strings.ToLower(filepath.Ext(name))
		items = append(items, RemoteMediaItem{
			URL:          previewURL,
			StudentID:    studentID,
			ExpectedName: name,
			IsVideo:      library.VideoExts[ext],
		})
	}

	// One big page per student (same size the downloader uses) — a single
	// API call yields plenty of preview items; no small paged requests.
	for _, id := range studentIDs {
		// Stop adding if we hit a global hard cap to prevent unbounded UI growth,
		// though normally limitPerStudent keeps this in check.
		if len(items) >= maxItems*2 {
			break
		}
		activities, err := client.FetchActivities(id, 0, 1000, start, end)
		if err != nil {
			log.Errorf("preview fetch failed for %s: %v", id, err)
			continue
		}
		
		studentTotalInPage := 0
		for _, act := range activities {
			studentTotalInPage += downloader.CountMedia(act)
		}

		studentName := ""
		studentItemsCount := 0
		for _, act := range activities {
			if act.StudentName() != "" {
				studentName = act.StudentName()
			}
			for idx, m := range act.Media {
				if m.ImageURL != "" && studentItemsCount < limitPerStudent {
					add(m.ImageURL, m.ThumbnailURL, id, studentName, act.Date(), idx)
					studentItemsCount++
				}
				if m.VideoURL != "" && studentItemsCount < limitPerStudent {
					add(m.VideoURL, "", id, studentName, act.Date(), idx)
					studentItemsCount++
				}
			}
			if act.VideoInfo != nil && studentItemsCount < limitPerStudent {
				v := act.VideoInfo
				u := v.DownloadableURL
				if u == "" {
					u = v.StreamableURL
				}
				if u != "" {
					add(u, "", id, studentName, act.Date(), 100)
					studentItemsCount++
				}
			}
			if studentItemsCount >= limitPerStudent {
				break
			}
		}
		
		if studentTotalInPage > studentItemsCount {
			items = append(items, RemoteMediaItem{
				StudentID:       id,
				ExpectedName:    fmt.Sprintf("__more_%s", id),
				IsMoreIndicator: true,
				MoreCount:       studentTotalInPage - studentItemsCount,
			})
		}
	}
	return items, nil
}

// --- Download ---

type wailsEvents struct{ ctx context.Context }

func (e wailsEvents) OnFile(ev downloader.FileEvent) {
	runtime.EventsEmit(e.ctx, "download:file", ev)
}
func (e wailsEvents) OnProgress(ev downloader.ProgressEvent) {
	runtime.EventsEmit(e.ctx, "download:progress", ev)
}
func (e wailsEvents) OnStatus(msg string) {
	runtime.EventsEmit(e.ctx, "download:status", msg)
}

// parseDateRange applies the same defaults as the Ruby script:
// 2021-02-04 through now, with inclusive end-of-day.
func parseDateRange(startDate, endDate string) (time.Time, time.Time) {
	start := time.Date(2021, 2, 4, 5, 0, 0, 0, time.UTC)
	end := time.Now().UTC()
	if t, err := time.Parse("2006-01-02", startDate); err == nil && startDate != "" {
		start = t.UTC()
	}
	if t, err := time.Parse("2006-01-02", endDate); err == nil && endDate != "" {
		end = t.Add(23*time.Hour + 59*time.Minute + 59*time.Second).UTC()
	}
	return start, end
}

// StartDownload begins downloading for the selected students in the
// background. folder must be (or be inside) the folder chosen via
// ChooseFolder. Progress is delivered via Wails events.
func (a *App) StartDownload(folder string, studentIDs []string, startDate, endDate string) error {
	// Confine the destination to the folder chosen via ChooseFolder; a
	// JS-supplied path must never widen filesystem access.
	safeFolder, err := a.resolveInAllowedFolder(folder)
	if err != nil {
		return err
	}

	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("download already running")
	}
	client := a.client
	if client == nil {
		a.mu.Unlock()
		return fmt.Errorf("not logged in")
	}
	selected := []brightwheel.Student{}
	for _, s := range a.students {
		for _, id := range studentIDs {
			if s.ObjectID == id {
				selected = append(selected, s)
			}
		}
	}
	a.mu.Unlock()
	if len(selected) == 0 {
		return fmt.Errorf("no students selected")
	}

	start, end := parseDateRange(startDate, endDate)

	engine := downloader.NewEngine(client, metadata.NewExifWriter(), wailsEvents{ctx: a.ctx})
	engine.Anonymize = a.demo
	a.mu.Lock()
	a.engine = engine
	a.running = true
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			a.running = false
			a.mu.Unlock()
			runtime.EventsEmit(a.ctx, "download:finished")
		}()

		// Pre-count pass: page through activities once (metadata only, no
		// media downloads) so the progress bar has a real total and %.
		label := func(i int) string {
			if a.demo {
				return fmt.Sprintf("student %d of %d", i+1, len(selected))
			}
			return selected[i].Name()
		}
		total := 0
		for i, s := range selected {
			runtime.EventsEmit(a.ctx, "download:status", "Counting media for "+label(i)+"…")
			for page := 0; ; page++ {
				activities, err := client.FetchActivities(s.ObjectID, page, 1000, start, end)
				if err != nil {
					log.Errorf("count pass failed for %s: %v", s.ObjectID, err)
					break
				}
				for _, act := range activities {
					total += downloader.CountMedia(act)
				}
				if len(activities) < 1000 {
					break
				}
			}
		}
		engine.SetTotal(total)
		runtime.EventsEmit(a.ctx, "download:progress", downloader.ProgressEvent{Total: total, Done: 0})

		if err := engine.Run(a.ctx, safeFolder, selected, start, end); err != nil {
			log.Errorf("download run failed: %v", err)
			runtime.EventsEmit(a.ctx, "download:status", "Error: "+err.Error())
		}
	}()
	return nil
}

// CancelDownload requests cancellation of the running download.
func (a *App) CancelDownload() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.engine != nil {
		a.engine.Cancel()
	}
}
