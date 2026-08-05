// Package downloader implements the media download engine: naming, dedup,
// legacy migration and per-file events. Ported from download_media.rb.
package downloader

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"keepsake/internal/brightwheel"
	"keepsake/internal/library"
	"keepsake/internal/log"
	"keepsake/internal/metadata"
)

const pageSize = 1000

// FileEvent describes a completed (or skipped) file for the UI.
type FileEvent struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
	Skipped  bool   `json:"skipped"`
	IsVideo  bool   `json:"is_video"`
	Err      string `json:"err,omitempty"`
}

// ProgressEvent reports overall progress.
type ProgressEvent struct {
	Total int `json:"total"`
	Done  int `json:"done"`
}

// Events receives download lifecycle callbacks.
type Events interface {
	OnFile(ev FileEvent)
	OnProgress(ev ProgressEvent)
	OnStatus(msg string)
}

// Engine downloads media for students into a destination folder.
type Engine struct {
	api        brightwheel.APIClient
	meta       metadata.Writer
	events     Events
	cancel     atomic.Bool
	total      atomic.Int64
	done       atomic.Int64
	fixedTotal bool
	lastProg   atomic.Int64 // unix nano of last throttled progress emit

	// Anonymize replaces student names in status messages (demo mode).
	Anonymize bool
}

// CountMedia returns how many downloadable media items an activity holds,
// matching the extraction logic in processActivity.
func CountMedia(a brightwheel.Activity) int {
	n := 0
	for _, m := range a.Media {
		if m.ImageURL != "" {
			n++
		}
		if m.VideoURL != "" {
			n++
		}
	}
	if a.VideoInfo != nil && (a.VideoInfo.DownloadableURL != "" || a.VideoInfo.StreamableURL != "") {
		n++
	}
	return n
}

// NewEngine builds an Engine.
func NewEngine(api brightwheel.APIClient, meta metadata.Writer, events Events) *Engine {
	return &Engine{api: api, meta: meta, events: events}
}

// emitProgress throttles progress events to ~5/sec; force emits regardless.
func (e *Engine) emitProgress(force bool) {
	now := time.Now().UnixNano()
	if !force && now-e.lastProg.Load() < int64(200*time.Millisecond) {
		return
	}
	e.lastProg.Store(now)
	e.events.OnProgress(ProgressEvent{Total: int(e.total.Load()), Done: int(e.done.Load())})
}

// SetTotal fixes the expected total (from a pre-count pass) so progress
// percentages are meaningful from the start. When unset, the total grows
// as items are discovered.
func (e *Engine) SetTotal(n int) {
	e.total.Store(int64(n))
	e.fixedTotal = n > 0
}

// Cancel requests cancellation; checked between activities.
func (e *Engine) Cancel() { e.cancel.Store(true) }

func (e *Engine) cancelled() bool { return e.cancel.Load() }

// Run downloads media for the given students and date range. Blocking;
// run it in a goroutine from the UI layer.
func (e *Engine) Run(ctx context.Context, folder string, students []brightwheel.Student, start, end time.Time) error {
	e.cancel.Store(false)
	if !e.fixedTotal {
		e.total.Store(0)
	}
	e.done.Store(0)
	if err := os.MkdirAll(folder, 0755); err != nil {
		return err
	}

	for i, student := range students {
		if e.cancelled() {
			e.events.OnStatus("cancelled")
			return nil
		}
		label := student.Name()
		if e.Anonymize {
			label = fmt.Sprintf("student %d of %d", i+1, len(students))
		}
		e.events.OnStatus(fmt.Sprintf("Processing %s", label))
		if err := e.downloadForStudent(folder, student, start, end); err != nil {
			log.Errorf("student %s failed: %v", student.ObjectID, err)
			e.events.OnStatus(fmt.Sprintf("Error for %s: %v", label, err))
		}
	}
	e.emitProgress(true)
	e.events.OnStatus("done")
	return nil
}

func (e *Engine) downloadForStudent(folder string, student brightwheel.Student, start, end time.Time) error {
	for page := 0; ; page++ {
		if e.cancelled() {
			return nil
		}
		log.Debugf("fetching page %d for student %s", page, student.ObjectID)
		activities, err := e.api.FetchActivities(student.ObjectID, page, pageSize, start, end)
		if err != nil {
			return err
		}
		if len(activities) == 0 {
			return nil
		}
		for _, activity := range activities {
			if e.cancelled() {
				return nil
			}
			e.processActivity(activity, folder)
		}
		e.emitProgress(false)
		if len(activities) < pageSize {
			return nil
		}
	}
}

func (e *Engine) processActivity(a brightwheel.Activity, folder string) {
	studentName := a.StudentName()
	note := a.Note
	dateStr := a.Date()

	for idx, m := range a.Media {
		if m.ImageURL != "" {
			e.downloadFile(m.ImageURL, folder, studentName, dateStr, idx, note)
		}
		if m.VideoURL != "" {
			e.downloadFile(m.VideoURL, folder, studentName, dateStr, idx, note)
		}
	}

	if a.VideoInfo != nil {
		if a.VideoInfo.DownloadableURL != "" {
			e.downloadFile(a.VideoInfo.DownloadableURL, folder, studentName, dateStr, 100, note)
		} else if a.VideoInfo.StreamableURL != "" {
			e.downloadFile(a.VideoInfo.StreamableURL, folder, studentName, dateStr, 100, note)
		}
	}
}

var unsafeChars = regexp.MustCompile(`[^a-z0-9]`)
var multiUnderscore = regexp.MustCompile(`_+`)

func safeName(studentName string) string {
	if studentName == "" {
		return "unknown_student"
	}
	s := unsafeChars.ReplaceAllString(strings.ToLower(studentName), "_")
	s = multiUnderscore.ReplaceAllString(s, "_")
	return strings.TrimRight(s, "_")
}

func formattedDate(dateStr string) string {
	if dateStr == "" {
		return "unknown_date"
	}
	if t, err := metadata.ParseDate(dateStr); err == nil {
		return t.Format("2006-01-02_15-04-05")
	}
	return "unknown_date"
}

// ExpectedFilename computes the base filename a media item will be saved
// as (before collision increments), so the UI can preview pending items
// and match them to completed downloads.
func ExpectedFilename(studentName, dateStr string, index int, rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s-%s_%d%s",
		safeName(studentName), formattedDate(dateStr), index, path.Ext(u.Path))
}

// downloadFile implements the dedup/collision logic ported from the Ruby
// script: find a free slot by comparing remote content-length with local
// size; on size match update metadata only.
func (e *Engine) downloadFile(rawURL, folder, studentName, dateStr string, index int, description string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		log.Errorf("bad url %s: %v", rawURL, err)
		return
	}
	ext := path.Ext(u.Path)
	name := safeName(studentName)
	date := formattedDate(dateStr)

	if !e.fixedTotal {
		e.total.Add(1)
	}
	defer e.done.Add(1)
	defer e.emitProgress(false)
	isVideo := library.VideoExts[strings.ToLower(ext)]

	currentIdx := index
	var target string
	for {
		filename := fmt.Sprintf("%s-%s_%d%s", name, date, currentIdx, ext)
		candidate := filepath.Join(folder, filename)

		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			// Legacy migration: if slot 0, rename an unindexed legacy file,
			// then re-check it like any existing file (size compare below).
			if currentIdx == 0 {
				legacy := filepath.Join(folder, fmt.Sprintf("%s-%s%s", name, date, ext))
				if _, err := os.Stat(legacy); err == nil {
					if rerr := os.Rename(legacy, candidate); rerr == nil {
						log.Infof("migrated legacy file %s", legacy)
						continue
					}
				}
			}
			target = candidate
			break
		}

		remoteSize, ok := e.api.RemoteSize(rawURL)
		if ok {
			if fi, err := os.Stat(candidate); err == nil && fi.Size() == remoteSize {
				log.Debugf("%s matches remote size, skipping download", filename)
				_ = e.meta.Update(candidate, description, dateStr)
				e.events.OnFile(FileEvent{Path: candidate, Filename: filename, Skipped: true, IsVideo: isVideo})
				return
			}
		}
		currentIdx++
	}

	filename := filepath.Base(target)
	log.Debugf("downloading %s from %s", filename, rawURL)

	f, err := os.Create(target)
	if err != nil {
		e.events.OnFile(FileEvent{Path: target, Filename: filename, IsVideo: isVideo, Err: err.Error()})
		return
	}
	err = e.api.Download(rawURL, f)
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(target)
		e.events.OnFile(FileEvent{Path: target, Filename: filename, IsVideo: isVideo, Err: err.Error()})
		return
	}

	_ = e.meta.Update(target, description, dateStr)
	e.events.OnFile(FileEvent{Path: target, Filename: filename, IsVideo: isVideo})
}
