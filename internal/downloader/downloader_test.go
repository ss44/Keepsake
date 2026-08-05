package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"keepsake/internal/brightwheel"
)

type mockAPI struct {
	activities []brightwheel.Activity
	server     *httptest.Server
}

func (m *mockAPI) FetchStudents() ([]brightwheel.Student, error) { return nil, nil }
func (m *mockAPI) FetchActivities(id string, page, size int, s, e time.Time) ([]brightwheel.Activity, error) {
	if page > 0 {
		return nil, nil
	}
	return m.activities, nil
}
func (m *mockAPI) RemoteSize(u string) (int64, bool) {
	req, _ := http.NewRequest(http.MethodHead, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	return resp.ContentLength, true
}
func (m *mockAPI) Download(u string, w io.Writer) error {
	resp, err := http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(w, resp.Body)
	return err
}

type nopMeta struct{}

func (nopMeta) Update(path, desc, date string) error { return nil }

type collectEvents struct {
	files    []FileEvent
	statuses []string
}

func (c *collectEvents) OnFile(ev FileEvent)         { c.files = append(c.files, ev) }
func (c *collectEvents) OnProgress(ev ProgressEvent) {}
func (c *collectEvents) OnStatus(msg string)         { c.statuses = append(c.statuses, msg) }

func newMock(t *testing.T) (*mockAPI, string) {
	t.Helper()
	payload := "fake-media-bytes"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		if r.Method == http.MethodHead {
			return
		}
		fmt.Fprint(w, payload)
	}))
	t.Cleanup(srv.Close)

	m := &mockAPI{server: srv, activities: []brightwheel.Activity{
		{
			Note:      "first",
			EventDate: "2024-03-01T10:00:00.000Z",
			Target:    brightwheel.Target{FirstName: "Jane", LastName: "Doe"},
			Media:     []brightwheel.Media{{ObjectID: "1111222233334444", ImageURL: srv.URL + "/a.jpg"}},
		},
		{
			Note:      "video",
			EventDate: "2024-03-02T10:00:00.000Z",
			Target:    brightwheel.Target{FirstName: "Jane", LastName: "Doe"},
			VideoInfo: &brightwheel.VideoInfo{ObjectID: "5555666677778888", DownloadableURL: srv.URL + "/v.mp4"},
		},
	}}
	return m, payload
}

func TestDownloadNamingAndContent(t *testing.T) {
	m, payload := newMock(t)
	dir := t.TempDir()
	ev := &collectEvents{}
	e := NewEngine(m, nopMeta{}, ev)

	students := []brightwheel.Student{{ObjectID: "s1", FirstName: "Jane", LastName: "Doe"}}
	if err := e.Run(context.Background(), dir, students, time.Time{}, time.Now()); err != nil {
		t.Fatal(err)
	}

	img := filepath.Join(dir, "jane_doe-2024-03-01_10-00-00_4444.jpg")
	vid := filepath.Join(dir, "jane_doe-2024-03-02_10-00-00_8888.mp4")
	for _, p := range []string{img, vid} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("expected %s: %v", p, err)
		}
		if string(data) != payload {
			t.Fatalf("bad content in %s", p)
		}
	}
	if len(ev.files) != 2 {
		t.Fatalf("expected 2 file events, got %d", len(ev.files))
	}
}

func TestDedupSkipsMatchingSize(t *testing.T) {
	m, payload := newMock(t)
	dir := t.TempDir()

	// Pre-create the target file with identical size.
	existing := filepath.Join(dir, "jane_doe-2024-03-01_10-00-00_4444.jpg")
	if err := os.WriteFile(existing, []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}

	ev := &collectEvents{}
	e := NewEngine(m, nopMeta{}, ev)
	students := []brightwheel.Student{{ObjectID: "s1"}}
	if err := e.Run(context.Background(), dir, students, time.Time{}, time.Now()); err != nil {
		t.Fatal(err)
	}

	// The pre-existing image must be skipped, not re-downloaded.
	if len(ev.files) != 2 || !ev.files[0].Skipped {
		t.Fatalf("expected first file to be skipped: %+v", ev.files)
	}
	// No collision files should exist.
	if _, err := os.Stat(filepath.Join(dir, "jane_doe-2024-03-01_10-00-00_4444-1.jpg")); !os.IsNotExist(err) {
		t.Fatal("unexpected collision file created")
	}
}

func TestCollisionIncrementsIndex(t *testing.T) {
	m, payload := newMock(t)
	dir := t.TempDir()

	// Occupy slot 0 with different content (size mismatch).
	existing := filepath.Join(dir, "jane_doe-2024-03-01_10-00-00_4444.jpg")
	differentContent := make([]byte, 200000) // 200KB difference
	if err := os.WriteFile(existing, differentContent, 0644); err != nil {
		t.Fatal(err)
	}

	ev := &collectEvents{}
	e := NewEngine(m, nopMeta{}, ev)
	students := []brightwheel.Student{{ObjectID: "s1"}}
	if err := e.Run(context.Background(), dir, students, time.Time{}, time.Now()); err != nil {
		t.Fatal(err)
	}

	collided := filepath.Join(dir, "jane_doe-2024-03-01_10-00-00_4444-1.jpg")
	data, err := os.ReadFile(collided)
	if err != nil {
		t.Fatalf("expected collision file: %v", err)
	}
	if string(data) != payload {
		t.Fatal("collision file has wrong content")
	}
	// Original untouched.
	orig, _ := os.ReadFile(existing)
	if len(orig) != 200000 {
		t.Fatal("original file was overwritten")
	}
}

func TestLegacyMigration(t *testing.T) {
	m, payload := newMock(t)
	// Override to empty UUID so it uses array index 0
	m.activities[0].Media[0].ObjectID = ""
	dir := t.TempDir()

	// Legacy unindexed file with matching size.
	legacy := filepath.Join(dir, "jane_doe-2024-03-01_10-00-00.jpg")
	if err := os.WriteFile(legacy, []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}

	ev := &collectEvents{}
	e := NewEngine(m, nopMeta{}, ev)
	students := []brightwheel.Student{{ObjectID: "s1"}}
	if err := e.Run(context.Background(), dir, students, time.Time{}, time.Now()); err != nil {
		t.Fatal(err)
	}

	migrated := filepath.Join(dir, "jane_doe-2024-03-01_10-00-00_0.jpg")
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy file still present")
	}
	if _, err := os.Stat(migrated); err != nil {
		t.Fatalf("migrated file missing: %v", err)
	}
	if len(ev.files) == 0 || !ev.files[0].Skipped {
		t.Fatalf("expected migrated file to be skipped: %+v", ev.files)
	}
}

func TestFormattedDateDateOnly(t *testing.T) {
	if got := formattedDate("2024-03-01"); got != "2024-03-01_00-00-00" {
		t.Fatalf("formattedDate date-only = %q", got)
	}
	if got := formattedDate("2024-03-01T10:00:00.000Z"); got != "2024-03-01_10-00-00" {
		t.Fatalf("formattedDate RFC3339 = %q", got)
	}
	if got := formattedDate("garbage"); got != "unknown_date" {
		t.Fatalf("formattedDate garbage = %q", got)
	}
}

func TestSafeName(t *testing.T) {
	cases := map[string]string{
		"Jane Doe":    "jane_doe",
		"  Bob  Smith ": "bob_smith",
		"O'Brien":     "o_brien",
		"":            "unknown_student",
	}
	for in, want := range cases {
		if got := safeName(strings.TrimSpace(in)); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
}
