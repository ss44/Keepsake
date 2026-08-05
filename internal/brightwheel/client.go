// Package brightwheel implements the Brightwheel API client.
package brightwheel

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"keepsake/internal/log"
)

const (
	apiHost        = "schools.mybrightwheel.com"
	baseURL        = "https://" + apiHost
	defaultUAStr   = "Mozilla/5.0"
	defaultPageSiz = 1000
)

// Credentials holds the intercepted session values needed for API calls.
type Credentials struct {
	Cookie   string `json:"cookie"`
	UserUUID string `json:"user_uuid"`
}

// Student is a Brightwheel student.
type Student struct {
	ObjectID  string `json:"object_id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// Name returns the full student name.
func (s Student) Name() string {
	name := s.FirstName
	if s.LastName != "" {
		if name != "" {
			name += " "
		}
		name += s.LastName
	}
	return name
}

// Media represents one media entry on an activity.
type Media struct {
	ImageURL     string `json:"image_url"`
	VideoURL     string `json:"video_url"`
	ThumbnailURL string `json:"thumbnail_url"`
}

// VideoInfo represents the video_info block on an activity.
type VideoInfo struct {
	DownloadableURL string `json:"downloadable_url"`
	StreamableURL   string `json:"streamable_url"`
}

// Target is the activity target (student).
type Target struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// Activity is a Brightwheel activity. Media can be a single object or an
// array, so it is decoded via custom UnmarshalJSON.
type Activity struct {
	Note      string     `json:"note"`
	EventDate string     `json:"event_date"`
	CreatedAt string     `json:"created_at"`
	Target    Target     `json:"target"`
	Media     []Media    `json:"-"`
	VideoInfo *VideoInfo `json:"video_info"`
}

type rawActivity struct {
	Note      string          `json:"note"`
	EventDate string          `json:"event_date"`
	CreatedAt string          `json:"created_at"`
	Target    Target          `json:"target"`
	Media     json.RawMessage `json:"media"`
	VideoInfo *VideoInfo      `json:"video_info"`
}

// UnmarshalJSON handles media being either an object or an array.
func (a *Activity) UnmarshalJSON(data []byte) error {
	var r rawActivity
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	a.Note, a.EventDate, a.CreatedAt, a.Target, a.VideoInfo = r.Note, r.EventDate, r.CreatedAt, r.Target, r.VideoInfo
	if len(r.Media) == 0 || string(r.Media) == "null" {
		return nil
	}
	var single Media
	if err := json.Unmarshal(r.Media, &single); err == nil && (single.ImageURL != "" || single.VideoURL != "") {
		a.Media = []Media{single}
		return nil
	}
	var arr []Media
	if err := json.Unmarshal(r.Media, &arr); err == nil {
		a.Media = arr
	}
	return nil
}

// Date returns event_date or created_at, whichever is present.
func (a Activity) Date() string {
	if a.EventDate != "" {
		return a.EventDate
	}
	return a.CreatedAt
}

// StudentName returns the target student's full name.
func (a Activity) StudentName() string {
	name := a.Target.FirstName
	if a.Target.LastName != "" {
		if name != "" {
			name += " "
		}
		name += a.Target.LastName
	}
	return name
}

// APIClient abstracts the Brightwheel API for testing.
type APIClient interface {
	FetchStudents() ([]Student, error)
	FetchActivities(studentID string, page, pageSize int, start, end time.Time) ([]Activity, error)
	RemoteSize(rawURL string) (int64, bool)
	Download(rawURL string, dst io.Writer) error
}

// Client is the HTTP implementation of APIClient.
type Client struct {
	creds Credentials
	http  *http.Client
}

// NewClient builds a Client from intercepted credentials.
func NewClient(creds Credentials) *Client {
	return &Client{creds: creds, http: &http.Client{Timeout: 5 * time.Minute}}
}

func (c *Client) newRequest(method, rawURL string) (*http.Request, error) {
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", defaultUAStr)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	// Only send session credentials to Brightwheel itself; media URLs come
	// from API responses and could otherwise leak the cookie to any host.
	if req.URL.Host == apiHost {
		req.Header.Set("Cookie", c.creds.Cookie)
		if c.creds.UserUUID != "" {
			req.Header.Set("X-User-UUID", c.creds.UserUUID)
		}
	}
	return req, nil
}

func (c *Client) getJSON(rawURL string, out interface{}) error {
	body, err := c.get(rawURL)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func (c *Client) get(rawURL string) ([]byte, error) {
	req, err := c.newRequest(http.MethodGet, rawURL)
	if err != nil {
		return nil, err
	}
	log.Debugf("GET %s", rawURL)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d for %s: %.500s", resp.StatusCode, rawURL, body)
	}
	return body, nil
}

// FetchStudents returns the students linked to the logged-in account via
// the guardians endpoint: /api/v1/guardians/{userUUID}/students, whose
// elements wrap each student. Falls back to /users/me shapes when the
// user UUID is unknown.
func (c *Client) FetchStudents() ([]Student, error) {
	if c.creds.UserUUID != "" {
		u := fmt.Sprintf("%s/api/v1/guardians/%s/students?include%%5B%%5D=schools",
			baseURL, c.creds.UserUUID)
		body, err := c.get(u)
		if err != nil {
			return nil, err
		}
		var data struct {
			Count    int `json:"count"`
			Students []struct {
				Student *Student `json:"student"`
			} `json:"students"`
		}
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, fmt.Errorf("guardians/students parse failed: %w", err)
		}
		students := []Student{}
		for _, s := range data.Students {
			if s.Student != nil && s.Student.ObjectID != "" {
				students = append(students, *s.Student)
			}
		}
		return students, nil
	}
	return c.fetchStudentsViaUsersMe()
}

// fetchStudentsViaUsersMe is the fallback student discovery path.
func (c *Client) fetchStudentsViaUsersMe() ([]Student, error) {
	body, err := c.get(baseURL + "/api/v1/users/me")
	if err != nil {
		return nil, err
	}

	students := []Student{}
	seen := map[string]bool{}
	add := func(s *Student) {
		if s == nil || s.ObjectID == "" || seen[s.ObjectID] {
			return
		}
		seen[s.ObjectID] = true
		students = append(students, *s)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("users/me parse failed: %w", err)
	}

	// Shape 1: top-level guardianships array (possibly nested under "user").
	for _, key := range []string{"guardianships", "user"} {
		raw, ok := root[key]
		if !ok {
			continue
		}
		var gs []struct {
			Student *Student `json:"student"`
		}
		if key == "user" {
			var u struct {
				Guardianships []struct {
					Student *Student `json:"student"`
				} `json:"guardianships"`
			}
			if json.Unmarshal(raw, &u) == nil {
				for _, g := range u.Guardianships {
					add(g.Student)
				}
			}
			continue
		}
		if json.Unmarshal(raw, &gs) == nil {
			for _, g := range gs {
				add(g.Student)
			}
		}
	}

	// Shape 2: top-level students array.
	if len(students) == 0 {
		if raw, ok := root["students"]; ok {
			var ss []Student
			if json.Unmarshal(raw, &ss) == nil {
				for i := range ss {
					add(&ss[i])
				}
			}
		}
	}

	if len(students) == 0 {
		// Unknown shape: log the keys (and body in debug) to aid support.
		keys := make([]string, 0, len(root))
		for k := range root {
			keys = append(keys, k)
		}
		log.Infof("users/me returned no recognizable students; top-level keys: %v", keys)
		log.Debugf("users/me body: %s", body)
	}
	return students, nil
}

type activitiesResponse struct {
	Activities []Activity `json:"activities"`
}

// FetchActivities returns one page of activities for a student.
func (c *Client) FetchActivities(studentID string, page, pageSize int, start, end time.Time) ([]Activity, error) {
	if pageSize <= 0 {
		pageSize = defaultPageSiz
	}
	q := url.Values{}
	q.Set("page", fmt.Sprint(page))
	q.Set("page_size", fmt.Sprint(pageSize))
	q.Set("start_date", start.UTC().Format("2006-01-02T15:04:05.000Z"))
	q.Set("end_date", end.UTC().Format("2006-01-02T15:04:05.000Z"))
	q.Set("include_parent_actions", "true")
	u := fmt.Sprintf("%s/api/v1/students/%s/activities?%s", baseURL, studentID, q.Encode())
	var data activitiesResponse
	if err := c.getJSON(u, &data); err != nil {
		return nil, err
	}
	return data.Activities, nil
}

// RemoteSize returns the content-length of a remote object via HEAD.
func (c *Client) RemoteSize(rawURL string) (int64, bool) {
	req, err := http.NewRequest(http.MethodHead, rawURL, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("User-Agent", defaultUAStr)
	resp, err := c.http.Do(req)
	if err != nil {
		log.Debugf("HEAD %s failed: %v", rawURL, err)
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength < 0 {
		return 0, false
	}
	return resp.ContentLength, true
}

// Download streams a remote object into dst.
func (c *Client) Download(rawURL string, dst io.Writer) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", defaultUAStr)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d for %s", resp.StatusCode, rawURL)
	}
	_, err = io.Copy(dst, resp.Body)
	return err
}
