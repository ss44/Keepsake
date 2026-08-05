// Package library scans the destination folder and models media files.
package library

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// File is one media file in the destination folder.
type File struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"`
	IsVideo bool   `json:"is_video"`
}

// VideoExts classifies video file extensions; shared by the metadata
// writer, downloader and library scan so classification never drifts.
var VideoExts = map[string]bool{".mp4": true, ".mov": true}

var videoExts = VideoExts
var mediaExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".heic": true,
	".mp4": true, ".mov": true,
}

// Scan lists media files in folder, newest first.
func Scan(folder string) ([]File, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	files := []File{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !mediaExts[ext] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, File{
			Path:    filepath.Join(folder, e.Name()),
			Name:    e.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
			IsVideo: videoExts[ext],
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ModTime > files[j].ModTime })
	return files, nil
}
