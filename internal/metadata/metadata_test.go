package metadata

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	jpegstructure "github.com/dsoprea/go-jpeg-image-structure/v2"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
)

func makeJpeg(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 128
	}
	img.Set(0, 0, color.White)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
}

func TestExifWriterJpeg(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.jpg")
	makeJpeg(t, path)

	w := NewExifWriter()
	if err := w.Update(path, "a nice note", "2024-03-01T10:00:00.000Z"); err != nil {
		t.Fatal(err)
	}

	jmp := jpegstructure.NewJpegMediaParser()
	intfc, err := jmp.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rootIfd, _, err := intfc.(*jpegstructure.SegmentList).Exif()
	if err != nil {
		t.Fatal(err)
	}

	results, err := rootIfd.FindTagWithName("ImageDescription")
	if err != nil || len(results) == 0 {
		t.Fatal("ImageDescription missing")
	}
	v, _ := results[0].Value()
	if v.(string) != "a nice note" {
		t.Fatalf("ImageDescription = %v", v)
	}

	exifIfd, err := rootIfd.ChildWithIfdPath(exifcommon.IfdExifStandardIfdIdentity)
	if err != nil {
		t.Fatal(err)
	}
	exifTags, err := exifIfd.FindTagWithName("DateTimeOriginal")
	if err != nil || len(exifTags) == 0 {
		t.Fatal("DateTimeOriginal missing")
	}
	dv, _ := exifTags[0].Value()
	if dv.(string) != "2024:03:01 10:00:00" {
		t.Fatalf("DateTimeOriginal = %v", dv)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.ModTime().Year() != 2024 || fi.ModTime().Month() != 3 {
		t.Fatalf("mtime not applied: %v", fi.ModTime())
	}
}

func TestExifWriterVideoNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(path, []byte("not-a-real-video"), 0644); err != nil {
		t.Fatal(err)
	}
	w := NewExifWriter()
	if err := w.Update(path, "desc", "2024-03-01T10:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	if fi.ModTime().Year() != 2024 {
		t.Fatal("mtime not applied to video")
	}
}
