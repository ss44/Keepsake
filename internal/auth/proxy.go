// Package auth implements the Brightwheel login flow. Because Wails v2 has
// no multi-window or cookie-interception API, login runs through a local
// reverse proxy: the app WebView navigates to the proxy, the proxy forwards
// to schools.mybrightwheel.com, captures session cookies (including
// HttpOnly), validates them against the API, then redirects back to the app.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"keepsake/internal/brightwheel"
	"keepsake/internal/log"
)

const targetHost = "schools.mybrightwheel.com"
const cdnHost = "cdn.mybrightwheel.com"

// cdnPrefix routes CDN assets through the proxy so their JS (which holds
// absolute API URLs) can be rewritten; otherwise XHRs would bypass the
// proxy and fail CORS.
const cdnPrefix = "/__cdn"

// LoginProxy manages the login interception session.
type LoginProxy struct {
	mu         sync.Mutex
	server     *http.Server
	port       int
	returnURL  string
	creds      *brightwheel.Credentials
	done       bool
	validating atomic.Bool
	httpClient *http.Client
	onSuccess  func(brightwheel.Credentials)
}

// NewLoginProxy creates a proxy. onSuccess fires once valid credentials
// have been captured.
func NewLoginProxy(onSuccess func(brightwheel.Credentials)) *LoginProxy {
	return &LoginProxy{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		onSuccess:  onSuccess,
	}
}

// Start spins up the proxy and returns the URL to navigate to.
func (p *LoginProxy) Start(returnURL string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.server != nil {
		return "", fmt.Errorf("login already in progress")
	}
	p.returnURL = sanitizeReturnURL(returnURL)
	p.done = false
	p.creds = nil
	p.validating.Store(false)

	target := &url.URL{Scheme: "https", Host: targetHost}
	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			if req.URL.Path == cdnPrefix || strings.HasPrefix(req.URL.Path, cdnPrefix+"/") {
				req.URL.Scheme = "https"
				req.URL.Host = cdnHost
				req.Host = cdnHost
				req.URL.Path = strings.TrimPrefix(req.URL.Path, cdnPrefix)
				if !strings.HasPrefix(req.URL.Path, "/") {
					req.URL.Path = "/" + req.URL.Path
				}
				req.Header.Del("Origin")
				req.Header.Del("Referer")
			} else {
				req.URL.Scheme = target.Scheme
				req.URL.Host = target.Host
				req.Host = targetHost
				req.Header.Set("Origin", "https://"+targetHost)
				req.Header.Set("Referer", "https://"+targetHost+"/")
			}
			// Force identity encoding so ModifyResponse can rewrite bodies.
			req.Header.Del("Accept-Encoding")
		},
		ModifyResponse: func(resp *http.Response) error {
			p.captureCookies(resp)
			return p.rewriteResponse(resp)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// WebView navigation aborts in-flight requests; that's normal
			// during the return-to-app redirect, so keep it at debug level.
			if errors.Is(err, context.Canceled) {
				log.Debugf("proxy request aborted: %s %s", r.Method, r.URL.Path)
			} else {
				log.Infof("proxy error: %v", err)
			}
			w.WriteHeader(http.StatusBadGateway)
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Status endpoint polled by the injected script (SPAs don't do
		// full navigations after login, so we can't rely on intercepting
		// one).
		if r.URL.Path == "/__auth_status" {
			done, ret := p.redirectTarget()
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"done":%t,"return":%q}`, done, ret)
			return
		}
		// Once credentials are validated, bounce the browser back to the app
		// on the next top-level document request.
		if done, ret := p.redirectTarget(); done && r.Method == http.MethodGet &&
			strings.Contains(r.Header.Get("Accept"), "text/html") {
			http.Redirect(w, r, ret, http.StatusFound)
			return
		}
		rp.ServeHTTP(w, r)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	p.port = ln.Addr().(*net.TCPAddr).Port
	p.server = &http.Server{Handler: handler}
	go func() {
		if err := p.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Errorf("login proxy error: %v", err)
		}
	}()
	log.Infof("Login proxy listening on http://127.0.0.1:%d", p.port)
	return fmt.Sprintf("http://127.0.0.1:%d/", p.port), nil
}

// proxyOrigin returns the proxy's own origin for URL rewriting.
func (p *LoginProxy) proxyOrigin() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return fmt.Sprintf("http://127.0.0.1:%d", p.port)
}

// rewriteResponse makes the proxied Brightwheel app work from the proxy
// origin: absolute URLs to the real host are rewritten to the proxy (so
// XHR/fetch calls stay same-origin and avoid CORS), redirects are
// rewritten, and cookies are made valid for the http loopback origin.
func (p *LoginProxy) rewriteResponse(resp *http.Response) error {
	origin := p.proxyOrigin()

	// Rewrite redirect targets back through the proxy.
	if loc := resp.Header.Get("Location"); loc != "" {
		resp.Header.Set("Location", strings.ReplaceAll(loc,
			"https://"+targetHost, origin))
	}

	// Cookies must be valid for http://127.0.0.1: drop Domain and Secure.
	if setCookies := resp.Header.Values("Set-Cookie"); len(setCookies) > 0 {
		resp.Header.Del("Set-Cookie")
		for _, sc := range setCookies {
			parts := strings.Split(sc, ";")
			kept := parts[:0]
			for _, part := range parts {
				attr := strings.ToLower(strings.TrimSpace(part))
				if strings.HasPrefix(attr, "domain=") || attr == "secure" {
					continue
				}
				kept = append(kept, part)
			}
			resp.Header.Add("Set-Cookie", strings.Join(kept, ";"))
		}
	}

	// Rewrite absolute URLs in text bodies.
	ct := resp.Header.Get("Content-Type")
	rewriteable := strings.Contains(ct, "text/html") ||
		strings.Contains(ct, "javascript") ||
		strings.Contains(ct, "text/css") ||
		strings.Contains(ct, "application/json")
	if !rewriteable {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	resp.Body.Close()

	targetURL := "https://" + targetHost
	cdnURL := "https://" + cdnHost
	cdnProxy := origin + cdnPrefix
	// CDN first: its URL contains no overlap with the target host, but
	// rewriting it first keeps ordering unambiguous.
	body = bytes.ReplaceAll(body, []byte(cdnURL), []byte(cdnProxy))
	body = bytes.ReplaceAll(body, []byte(targetURL), []byte(origin))
	// Escaped forms inside JS/JSON strings.
	body = bytes.ReplaceAll(body,
		[]byte(strings.ReplaceAll(cdnURL, "/", "\\/")),
		[]byte(strings.ReplaceAll(cdnProxy, "/", "\\/")))
	body = bytes.ReplaceAll(body,
		[]byte(strings.ReplaceAll(targetURL, "/", "\\/")),
		[]byte(strings.ReplaceAll(origin, "/", "\\/")))

	// Inject the return-to-app poller into HTML pages. Brightwheel is an
	// SPA: after login it routes client-side without a full navigation, so
	// the page itself must detect completed auth and redirect.
	if strings.Contains(ct, "text/html") {
		const poller = `<script>(function(){var i=setInterval(function(){` +
			`fetch("/__auth_status").then(function(r){return r.json()}).then(function(d){` +
			`if(d.done){clearInterval(i);window.location.href=d["return"];}}).catch(function(){});` +
			`},1000);})();</script>`
		if idx := bytes.LastIndex(bytes.ToLower(body), []byte("</body>")); idx >= 0 {
			body = append(body[:idx], append([]byte(poller), body[idx:]...)...)
		} else {
			body = append(body, []byte(poller)...)
		}
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", fmt.Sprint(len(body)))
	resp.Header.Del("Content-Encoding")
	return nil
}

// captureCookies stores session cookies from every response and attempts
// validation in the background.
func (p *LoginProxy) captureCookies(resp *http.Response) {
	p.mu.Lock()
	if p.done {
		p.mu.Unlock()
		return
	}
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		p.mu.Unlock()
		return
	}
	jar := map[string]string{}
	if p.creds != nil && p.creds.Cookie != "" {
		for _, part := range strings.Split(p.creds.Cookie, "; ") {
			if kv := strings.SplitN(part, "=", 2); len(kv) == 2 {
				jar[kv[0]] = kv[1]
			}
		}
	}
	for _, c := range cookies {
		jar[c.Name] = c.Value
	}
	parts := make([]string, 0, len(jar))
	for k, v := range jar {
		parts = append(parts, k+"="+v)
	}
	if p.creds == nil {
		p.creds = &brightwheel.Credentials{}
	}
	p.creds.Cookie = strings.Join(parts, "; ")
	creds := *p.creds
	p.mu.Unlock()

	log.Debugf("captured %d cookies; validating session...", len(jar))
	// At most one validation request in flight; a login flow sets cookies
	// on many responses.
	if p.validating.CompareAndSwap(false, true) {
		go p.validate(creds)
	}
}

// validate checks the cookie jar against the API; on success it records
// credentials and lets the next navigation redirect back to the app.
func (p *LoginProxy) validate(creds brightwheel.Credentials) {
	defer p.validating.Store(false)
	var me struct {
		ObjectID string `json:"object_id"`
		UUID     string `json:"uuid"`
	}
	req, err := http.NewRequest(http.MethodGet, "https://"+targetHost+"/api/v1/users/me", nil)
	if err != nil {
		return
	}
	req.Header.Set("Cookie", creds.Cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	resp, err := p.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return
	}
	creds.UserUUID = firstNonEmpty(me.ObjectID, me.UUID)

	p.mu.Lock()
	p.creds = &creds
	p.done = true
	p.mu.Unlock()
	log.Infof("Login successful (user %s)", creds.UserUUID)
	if p.onSuccess != nil {
		p.onSuccess(creds)
	}
}

// ServeHTTP-style redirect is handled inside the proxy via a wrapper.
func (p *LoginProxy) redirectTarget() (bool, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.done, p.returnURL
}

// sanitizeReturnURL only allows redirecting back to the app's own origins
// (local dev server or the Wails asset origin); anything else falls back
// to the production asset origin.
func sanitizeReturnURL(raw string) string {
	const fallback = "http://wails.localhost/"
	u, err := url.Parse(raw)
	if err != nil {
		return fallback
	}
	switch u.Scheme {
	case "wails":
		return raw
	case "http", "https":
		host := u.Hostname()
		if host == "localhost" || host == "127.0.0.1" || host == "wails.localhost" {
			return raw
		}
	}
	return fallback
}

// Stop shuts the proxy down.
func (p *LoginProxy) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.server.Shutdown(ctx)
		p.server = nil
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
