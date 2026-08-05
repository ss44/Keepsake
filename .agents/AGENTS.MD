# AGENTS.md — Keepsake

## What this is

Keepsake is a cross-platform desktop app (Go + Wails v2 + Vue 3) that lets
parents download their own children's photos/videos from their Brightwheel
account. It is a port of the Ruby script `../download_media.rb` (and
`../lib/*.rb`), which remains the behavioral spec for download/dedup logic.

**Legal framing (do not dilute):** no affiliation with Brightwheel;
"Brightwheel" appears only as nominative description (API package name,
"Log in with Brightwheel"). App name, binary, titles are all "Keepsake".
Non-commercial, MIT licensed. See README "Purpose & legal notice".

## Stack & invariants

- Wails **v2** (v2.13.0) — do NOT migrate to v3 casually.
- Go module name: `keepsake`. Backend does ALL heavy lifting; the Vue
  frontend is a thin UI layer (Pinia store + daisyUI components).
- SOLID via interfaces: `brightwheel.APIClient`, `metadata.Writer`,
  `downloader.Events` — enables mock-based tests without network/disk.
- Build on this machine (Arch/CachyOS, webkit2gtk-**4.1**) requires the
  `webkit2_41` build tag: `wails dev -tags webkit2_41`,
  `wails build -tags webkit2_41`. `wails doctor` reports "libwebkit not
  found" — that is a false negative (it probes for 4.0).
- App args: `wails dev` passes them via `-appargs "-demo -debug"` (NOT
  `--`); the built binary takes `--demo`/`--debug` directly.
- Tests: `go test ./internal/...` (root package needs `frontend/dist`
  built first; internal packages don't).
- `frontend/dist` is gitignored; regenerate via `npm run build` or any
  wails command. After changing Go bindings run `wails generate module`.

## Auth design (the non-obvious part)

Wails v2 has **no multi-window and no cookie API** (that's v3), so login
uses a local reverse proxy (`internal/auth/proxy.go`):

1. Frontend navigates the WebView to `http://127.0.0.1:<port>` served by
   the proxy; the proxy forwards to `schools.mybrightwheel.com`.
2. Cookie capture: every response's cookies merge into a jar; a single
   (atomic-guarded) background validation hits `/api/v1/users/me`; on 200
   the session is valid → `done=true`, user UUID recorded, `auth:success`
   event emitted.
3. **CORS workaround:** Brightwheel's SPA JS calls absolute API URLs, so
   responses are rewritten in `ModifyResponse`: `Accept-Encoding` is
   stripped upstream, bodies (html/js/css/json) get
   `https://schools.mybrightwheel.com` → proxy origin (plain + `\/`
   escaped), `Location` rewritten, `Domain`/`Secure` stripped from
   `Set-Cookie`.
4. **CDN workaround:** the SPA bundle comes from `cdn.mybrightwheel.com`;
   it is proxied under the path prefix `/__cdn/` and its URL rewritten in
   bodies the same way, so API URLs inside the bundle get rewritten too.
5. **Return-to-app:** Brightwheel is an SPA — no full navigation happens
   after login/2FA — so a tiny poller `<script>` is injected before
   `</body>` that polls `/__auth_status` and redirects to the (sanitized)
   app origin when `done`. `sanitizeReturnURL` allows only loopback/Wails
   origins (open-redirect hardening).
6. `context canceled` proxy errors during the return navigation are
   normal; logged at debug level.

## Brightwheel API facts (verified against a real account)

- Students: `GET /api/v1/guardians/{userUUID}/students?include[]=schools`
  → `{"count":N,"students":[{"student":{object_id,first_name,last_name,
  ...}}]}` (wrapped!). Fallback: `/api/v1/users/me` guardianships shapes.
- Activities: `GET /api/v1/students/{id}/activities?page=N&page_size=1000&
  start_date=..&end_date=..&include_parent_actions=true` →
  `{"activities":[...]}`; stop when page empty or `< page_size`.
- `media` is a single object OR an array (custom UnmarshalJSON in the
  client); entries have `image_url`, `video_url`, `thumbnail_url`.
- `video_info`: `downloadable_url` preferred, else `streamable_url`,
  index offset 100 to avoid colliding with media-array indices.
- Auth needs only `Cookie` (+ `X-User-UUID` when known), `User-Agent`,
  `Accept`. Media/CDN URLs are signed and public (no auth needed for
  download/HEAD).
- `event_date` may be date-only (`2024-03-01`) — `metadata.ParseDate` is
  the single shared parser; never add a second date-layout list.

## Download engine (`internal/downloader`)

- Filename: `{safe_name}-{YYYY-MM-DD_HH-MM-SS}_{index}{ext}`;
  safe_name = lowercase, non-alnum→`_`, collapse `_`, trim trailing.
  `ExpectedFilename` is exported for the UI's pending-preview matching.
- Dedup: for slot N, if file exists, HEAD `content-length` vs local size;
  match → metadata refresh only; mismatch → N+1. Legacy unindexed files
  migrate to `_0` when slot 0 is free.
- Progress: optional pre-count pass (`SetTotal` + `CountMedia`) fixes the
  total so the UI shows a real %; otherwise total grows as discovered.
  Events throttled (~5/s progress; status on student boundaries only).
- `Anonymize` (demo mode) → status says "student 1 of 2", never names.

## Metadata (`internal/metadata`)

- Pure Go: `dsoprea/go-exif/v3` + `go-jpeg-image-structure/v2`.
  `SegmentList.ConstructExifBuilder` handles missing/corrupt EXIF.
- JPEG only: `ImageDescription`, EXIF `DateTimeOriginal`,
  `DateTimeDigitized`. MP4/MOV embedded metadata is an ACCEPTED no-op
  (documented limitation); `os.Chtimes` always applied LAST (EXIF rewrite
  touches mtime). Writes go to a temp file + rename (never corrupt
  originals).

## Frontend (`frontend/src`)

- Pinia store (`stores/app.js`) holds session, students, files, pending
  previews, progress, thumbs cache, `demo` flag.
- MediaGrid merges downloaded files + pending remote previews keyed by
  expected filename; pending = grayscale remote `thumbnail_url`; when the
  download event lands the SAME DOM node flips class and a CSS `filter`
  transition animates B&W→color. Deselecting a student hides their items
  (pending via `student_id`; files via safe-name prefix match).
- File events are batched (200 ms flush + Set dedup) to avoid O(n²).
- Preview = `PreviewMedia`: ONE page_size=1000 call per student, first 10
  items only. Keep API calls minimal — never page for previews.
- Security: `GetThumbnail`/`ScanFolder` are confined to the user-chosen
  download folder (`resolveInAllowedFolder`) because Wails injects its
  bridge into every WebView document, including the remote login page.
- Demo mode: backend pixelates thumbnails (`/17` downscale +
  nearest-neighbor upscale); frontend blurs pending remote imgs (24px),
  names/captions (6px); status text unblurred (backend anonymizes).

## Sensitive-data rules

- NEVER commit session cookies/tokens. The parent Ruby script contains
  expired ones — do not copy. User-shared curls in chat contained live
  credentials; treat them as secrets, never write them to the repo.

## Repo state

Own git repo (nested in the parent project for context), currently with
no commits. Binary: `build/bin/keepsake`.
