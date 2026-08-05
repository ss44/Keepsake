package auth

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"keepsake/internal/brightwheel"
)

func testProxy() *LoginProxy {
	p := NewLoginProxy(func(brightwheel.Credentials) {})
	p.port = 39883
	return p
}

func TestRewriteResponseBodyURLs(t *testing.T) {
	p := testProxy()
	body := `{"api":"https://schools.mybrightwheel.com/api/v1/sessions/start","esc":"https:\/\/schools.mybrightwheel.com\/x","cdn":"https://cdn.mybrightwheel.com/static/assets/index.js"}`
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(strings.NewReader(body)),
	}
	if err := p.rewriteResponse(resp); err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(resp.Body)
	s := string(out)
	if strings.Contains(s, "schools.mybrightwheel.com") {
		t.Fatalf("target host not rewritten: %s", s)
	}
	if !strings.Contains(s, "http://127.0.0.1:39883/api/v1/sessions/start") {
		t.Fatalf("plain URL not rewritten: %s", s)
	}
	if !strings.Contains(s, `http:\/\/127.0.0.1:39883\/x`) {
		t.Fatalf("escaped URL not rewritten: %s", s)
	}
	if !strings.Contains(s, "http://127.0.0.1:39883/__cdn/static/assets/index.js") {
		t.Fatalf("CDN URL not rewritten: %s", s)
	}
	if resp.Header.Get("Content-Length") != "0" && resp.ContentLength != int64(len(out)) {
		t.Fatal("content length not updated")
	}
}

func TestRewriteResponseCookiesAndLocation(t *testing.T) {
	p := testProxy()
	resp := &http.Response{
		Header: http.Header{
			"Content-Type": []string{"text/html"},
			"Location":     []string{"https://schools.mybrightwheel.com/dashboard"},
			"Set-Cookie": []string{
				"_session=abc; Domain=.mybrightwheel.com; Secure; HttpOnly; Path=/",
			},
		},
		Body: io.NopCloser(strings.NewReader("<html></html>")),
	}
	if err := p.rewriteResponse(resp); err != nil {
		t.Fatal(err)
	}
	if loc := resp.Header.Get("Location"); loc != "http://127.0.0.1:39883/dashboard" {
		t.Fatalf("Location = %q", loc)
	}
	sc := resp.Header.Get("Set-Cookie")
	if strings.Contains(strings.ToLower(sc), "domain=") || strings.Contains(strings.ToLower(sc), "secure") {
		t.Fatalf("Set-Cookie not sanitized: %q", sc)
	}
	if !strings.Contains(sc, "_session=abc") || !strings.Contains(sc, "HttpOnly") {
		t.Fatalf("Set-Cookie mangled: %q", sc)
	}
}

func TestRewriteResponseInjectsPoller(t *testing.T) {
	p := testProxy()
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/html"}},
		Body:   io.NopCloser(strings.NewReader("<html><body>login</body></html>")),
	}
	if err := p.rewriteResponse(resp); err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(resp.Body)
	s := string(out)
	if !strings.Contains(s, "/__auth_status") {
		t.Fatal("poller script not injected")
	}
	if !strings.HasSuffix(s, "</html>") {
		t.Fatal("poller injected in wrong place")
	}
	if resp.ContentLength != int64(len(out)) {
		t.Fatal("content length not updated after injection")
	}
}

func TestRewriteResponseSkipsBinary(t *testing.T) {
	p := testProxy()
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"image/png"}},
		Body:   io.NopCloser(strings.NewReader("https://schools.mybrightwheel.com")),
	}
	if err := p.rewriteResponse(resp); err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(resp.Body)
	if string(out) != "https://schools.mybrightwheel.com" {
		t.Fatal("binary body was rewritten")
	}
}
