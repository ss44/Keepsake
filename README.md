# Keepsake

<p align="center">
  <img src="keepsake.svg" alt="Keepsake Logo" width="300" height="300" />
</p>

<p align="center">
  Download: <a href="https://github.com/ss44/Keepsake/releases/latest/download/keepsake-windows-amd64.zip">🪟 Windows</a> | <a href="https://github.com/ss44/Keepsake/releases/latest/download/keepsake-macos-universal.zip">🍎 macOS</a> | <a href="https://github.com/ss44/Keepsake/releases/latest/download/keepsake-linux-amd64.tar.gz">🐧 Linux</a>
</p>

Keepsake is a friendly, cross-platform desktop app (built with Go, Wails v2, and Vue 3) designed to help parents easily download their children's photos and videos from Brightwheel. It takes care of the tedious parts by automatically handling deduplication, normalizing filenames, and even restoring original EXIF metadata and filesystem timestamps.

## Purpose & Legal Notice

*   **Keepsake has no affiliation with Brightwheel.** It is not endorsed, sponsored, or associated with Brightwheel in any way. "Brightwheel" is referenced only to identify the service the app connects to.
*   **Sole purpose:** Keepsake is built to help parents and guardians download photos and videos of *their own children* from *their own accounts*, using their own login credentials. It does not bypass any access control. You must be able to log in to the Brightwheel website normally (including resolving any MFA/2FA challenges) to use it.
*   **Non-commercial and open source:** This project is entirely free, makes no money, contains no ads or tracking, and is licensed under the MIT License (see `LICENSE`).
*   **Terms of Service:** Your use of this app may be subject to Brightwheel's Terms of Service. You are responsible for your own compliance. Please only download content you are authorized to access.
*   **A friendly note to Brightwheel:** This tool exists simply because parents currently lack an official way to export their children's precious daily memories in bulk. We would love nothing more than for this project to be made happily obsolete by Brightwheel offering parents a direct "download my child's photos" export option in the official platform!

## Features

*   **Secure login:** The app loads the real Brightwheel site via a local reverse proxy to safely capture the session cookies (including HttpOnly) once you log in. Your password is never seen or stored by Keepsake, and CAPTCHA/MFA flows work exactly as they do in your browser.
*   **Student selection:** Automatically fetches your children's profiles from the Brightwheel API and lets you select which student's media to download.
*   **Pending previews:** Previews of new media appear as grayscale thumbnails and transition to beautiful full color as the files complete downloading.
*   **Date range filters:** Easily narrow down your downloads with optional start and end date pickers (defaults to downloading all time).
*   **Smart deduplication:** The app checks existing files against remote sizes using lightweight HEAD requests. Identical matches are skipped (while keeping metadata up to date) and any genuine filename collisions are automatically resolved with incremented `_N` suffixes.
*   **Legacy migration:** Automatically cleans up older unindexed `name-date.jpg` files by renaming them to the modern, clean `name-date_0.jpg` format.
*   **Metadata restoration:** JPEGs automatically get `ImageDescription`, `DateTimeOriginal`, and `DateTimeDigitized` EXIF headers written in pure Go. All files (including videos) have their filesystem creation/modification times correctly matched to the actual activity date.
*   **Media grid:** Browse your downloaded collection directly in the app. The library is scanned dynamically, showing lazy-loaded thumbnails that update in real time as new downloads arrive.
*   **Progress tracking:** View your download status at a glance with an interactive progress bar and live-scrolling action logs, complete with full cancel support.
*   **Demo mode:** Run the app with `--demo` to pixelate all media and anonymize student names—perfect for safe sharing or screen recording.

## Known Limitations

*   **Video metadata:** Embedded metadata inside MP4 and MOV files is not directly modified—only their filesystem timestamps are set. This is a deliberate design choice to keep Keepsake written entirely in pure Go without requiring heavy external tools like `exiftool`.
*   **Web app changes:** The login flow depends on Brightwheel's cookie-based web sessions. If Brightwheel makes significant updates to their web platform, the session detection might require a quick update.
*   **Under the hood (auth):** Since Wails v2 doesn't have built-in APIs for cookie interception or managing multiple windows, Keepsake runs a lightweight local reverse proxy (`internal/auth`) to capture cookies securely, redirecting your view back to the main app interface once you are successfully logged in.

## Prerequisites

To build and run Keepsake locally, you'll need:

*   Go 1.25+
*   Node.js 20+ and npm
*   Wails CLI v2: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
*   Platform WebView dependencies (see [Wails Installation Guide](https://wails.io/docs/gettingstarted/installation)):
    *   **Linux:** `webkit2gtk` (if your system uses webkit2gtk-4.1, use the `webkit2_41` build tag described below)
    *   **macOS / Windows:** Supported out of the box

## Development

Get started by launching a hot-reloading development environment:

```sh
wails dev                 # Standard hot-reload (add -tags webkit2_41 on Linux if using webkit2gtk-4.1)
wails dev -tags webkit2_41
```

If you need verbose logging or want to test demo mode (which pixelates photos and blurs names for safe recordings), pass arguments to the app using `-appargs` (note that `wails dev` uses `-appargs` rather than standard flags):

```sh
wails dev -tags webkit2_41 -appargs "-debug"          # dev mode + detailed logs
wails dev -tags webkit2_41 -appargs "-demo -debug"    # dev mode + safe demo mode
./build/bin/keepsake --debug                          # running a built binary with logs
./build/bin/keepsake --demo                           # running a built binary in demo mode
```

## Building the App

Compile a production-ready binary for your platform:

```sh
# Linux (with webkit2gtk-4.1)
wails build -tags webkit2_41

# Linux (with webkit2gtk-4.0), macOS, or Windows
wails build
```

Your compiled app will be placed in `build/bin/`. You can run it directly with these optional flags:

```sh
./build/bin/keepsake            # Normal launch
./build/bin/keepsake --debug    # Launch with verbose logging
./build/bin/keepsake --demo     # Launch in demo mode (pixelated images, anonymized names)
./build/bin/keepsake --demo --debug
```

## Running Tests

Keep things running smoothly with the Go test suite:

```sh
go test ./internal/...
```

*Note on frontend building:* Because the Vue frontend assets (`frontend/dist`) are generated and excluded from git, running plain Go commands from the root directory (like `go build` or `go vet`) requires building the frontend at least once first (using `npm run build` inside `frontend/` or by running `wails dev`/`wails build`). Tests scoped to `./internal/...` do not require this and will always run immediately.

Our test coverage includes the core download engine (naming conventions, deduplication logic, duplicate indexing, and legacy migration paths), API client parsing, auth proxy rewriting, and the EXIF metadata writer.

## Architecture

Here's how Keepsake is organized under the hood:

```
main.go                  Wails entry point & flag parsing (--debug, --demo)
app.go                   Wails app binding facade
internal/
  auth/                  Login reverse proxy & secure cookie capture
  brightwheel/           API client & data transfer objects (DTOs)
  downloader/            Download queue, deduplication, and file writer
  metadata/              EXIF metadata injection and timestamp writer
  library/               Local destination folder scanner
  log/                   Simple leveled, debug-gated logger
frontend/                Vue 3 frontend built with Vite, Pinia, Tailwind CSS, and daisyUI
```

Communication between the Go backend and the Vue frontend relies on asynchronous Wails events (`auth:success`, `download:file`, `download:progress`, `download:status`, `download:finished`).

## Contributing

We welcome contributions of all shapes and sizes! Whether you are fixing a bug, suggesting a new feature, or improving the documentation, your help is highly appreciated.

To get started:
1.  **Fork the repository** and create your branch from `main`.
2.  **Make your changes**, keeping in mind our friendly and clean standards.
3.  **Run the tests** (`go test ./internal/...`) to ensure everything still works beautifully.
4.  **Submit a Pull Request** with a clear description of your changes.

If you are planning to make a larger structural change, feel free to open an issue first so we can discuss the best way to integrate it. Thank you for helping make Keepsake better for parents everywhere!

## AI Disclosure

This project was built with the assistance of AI tools and models, specifically Kilo and Kimi 3.

