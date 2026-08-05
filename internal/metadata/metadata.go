// Package metadata writes timestamps and descriptions into downloaded media.
package metadata

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	jpegstructure "github.com/dsoprea/go-jpeg-image-structure/v2"

	"keepsake/internal/library"
	"keepsake/internal/log"
)

// Writer abstracts metadata writing for testing.
type Writer interface {
	Update(path, description, dateStr string) error
}

// ExifWriter writes JPEG EXIF metadata in pure Go and always applies
// filesystem timestamps. Video embedded metadata is a documented no-op.
type ExifWriter struct{}

// NewExifWriter returns the default Writer.
func NewExifWriter() *ExifWriter { return &ExifWriter{} }

var videoExts = library.VideoExts

// ParseDate parses the date formats Brightwheel returns (full timestamps
// or date-only). Shared by the downloader so filenames and metadata never
// disagree on what is a valid date.
func ParseDate(s string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, &time.ParseError{Value: s}
}

// Update applies filesystem timestamps and, for JPEGs, EXIF metadata.
func (w *ExifWriter) Update(path, description, dateStr string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}

	var parsed time.Time
	hasDate := false
	if dateStr != "" {
		if t, err := ParseDate(dateStr); err == nil {
			parsed = t
			hasDate = true
		} else {
			log.Debugf("could not parse date %q for %s", dateStr, path)
		}
	}

	ext := strings.ToLower(filepath.Ext(path))
	isJpeg := ext == ".jpg" || ext == ".jpeg"
	if isJpeg {
		if err := writeJpegExif(path, description, parsed, hasDate); err != nil {
			log.Errorf("could not write EXIF to %s: %v", path, err)
		}
	} else if videoExts[ext] {
		// Known limitation: embedded metadata is only written for JPEG.
		// MP4/MOV rely on filesystem timestamps only.
		log.Debugf("skipping embedded metadata for video %s (unsupported)", path)
	}

	// Always apply filesystem timestamps last (EXIF writes modify mtime).
	if hasDate {
		if err := os.Chtimes(path, parsed, parsed); err != nil {
			log.Errorf("could not set file timestamps on %s: %v", path, err)
		}
	}
	return nil
}

// writeJpegExif sets ImageDescription, DateTimeOriginal and
// DateTimeDigitized on a JPEG, preserving any existing EXIF.
func writeJpegExif(path, description string, date time.Time, hasDate bool) error {
	jmp := jpegstructure.NewJpegMediaParser()

	intfc, err := jmp.ParseFile(path)
	if err != nil {
		return err
	}
	sl := intfc.(*jpegstructure.SegmentList)

	rootIb, err := sl.ConstructExifBuilder()
	if err != nil {
		// Corrupt existing EXIF: rebuild from scratch.
		im, err2 := exifcommon.NewIfdMappingWithStandard()
		if err2 != nil {
			return err2
		}
		ti := exif.NewTagIndex()
		if err2 := exif.LoadStandardTags(ti); err2 != nil {
			return err2
		}
		rootIb = exif.NewIfdBuilder(im, ti, exifcommon.IfdStandardIfdIdentity, exifcommon.EncodeDefaultByteOrder)
	}

	if description != "" {
		if err := rootIb.AddStandardWithName("ImageDescription", description); err != nil {
			log.Debugf("ImageDescription failed on %s: %v", path, err)
		}
	}
	if hasDate {
		exifDate := date.Format("2006:01:02 15:04:05")
		exifIb, err := exif.GetOrCreateIbFromRootIb(rootIb, "IFD/Exif")
		if err != nil {
			return err
		}
		if err := exifIb.AddStandardWithName("DateTimeOriginal", exifDate); err != nil {
			log.Debugf("DateTimeOriginal failed on %s: %v", path, err)
		}
		if err := exifIb.AddStandardWithName("DateTimeDigitized", exifDate); err != nil {
			log.Debugf("DateTimeDigitized failed on %s: %v", path, err)
		}
	}

	if err := sl.SetExif(rootIb); err != nil {
		return err
	}

	// Stream to a temp file in the same directory, then atomically rename;
	// avoids buffering the whole JPEG and never corrupts the original.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".exif-*.jpg")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := sl.Write(tmp); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
