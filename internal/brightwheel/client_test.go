package brightwheel

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchStudentsGuardiansShape(t *testing.T) {
	body := `{"count":2,"students":[` +
		`{"relationship_type":"parent","guardian_id":"u1","student":{"object_id":"s1","first_name":"Deyal Singh","last_name":"Padda"}},` +
		`{"relationship_type":"parent","guardian_id":"u1","student":{"object_id":"s2","first_name":"Jodh Singh","last_name":"Padda"}}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/api/v1/guardians/u1/students"
		if r.URL.Path != want {
			t.Errorf("path = %q, want %q", r.URL.Path, want)
		}
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := NewClient(Credentials{Cookie: "x", UserUUID: "u1"})
	// baseURL is a const, so test through a transport that rewrites the host.
	c.http.Transport = rewriteHostTransport{target: srv.URL}
	students, err := c.FetchStudents()
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 2 {
		t.Fatalf("got %d students", len(students))
	}
	if students[0].ObjectID != "s1" || students[0].Name() != "Deyal Singh Padda" {
		t.Fatalf("student 0 = %+v", students[0])
	}
	if students[1].ObjectID != "s2" {
		t.Fatalf("student 1 = %+v", students[1])
	}
}

type rewriteHostTransport struct{ target string }

func (rt rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = "http"
	req2.URL.Host = rt.target[len("http://"):]
	req2.Host = req2.URL.Host
	return http.DefaultTransport.RoundTrip(req2)
}
