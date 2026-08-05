import { defineStore } from 'pinia'
import {
  StartLogin, AuthStatus, Logout, FetchStudents,
  ChooseFolder, ScanFolder, StartDownload, CancelDownload, GetThumbnail,
  PreviewMedia, IsDemo,
} from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

// safeName mirrors the Go/downloader filename sanitization.
export function safeName(name) {
  if (!name) return 'unknown_student'
  return name.toLowerCase().replace(/[^a-z0-9]/g, '_').replace(/_+/g, '_').replace(/_+$/, '')
}

export const useAppStore = defineStore('app', {
  state: () => ({
    loggedIn: false,
    students: [],
    selected: {},
    folder: '',
    files: [],
    progress: { total: 0, done: 0 },
    status: '',
    downloading: false,
    thumbs: {},
    pending: [],
    previewLoading: false,
    demo: false,
    eventsBound: false,
    _knownPaths: new Set(),
    _pendingFiles: [],
    _flushTimer: null,
  }),
  getters: {
    selectedIds: (s) => Object.keys(s.selected).filter((k) => s.selected[k]),
    allSelected: (s) => s.students.length > 0 && s.students.every((st) => s.selected[st.object_id]),
  },
  actions: {
    bindEvents() {
      if (this.eventsBound) return
      this.eventsBound = true
      EventsOn('auth:success', () => { this.loggedIn = true; this.loadStudents() })
      EventsOn('download:file', (ev) => {
        // Buffer events and flush in batches: thousands of per-file events
        // would otherwise each trigger an O(n) find + reactive unshift.
        if (!this._knownPaths.has(ev.path)) {
          this._knownPaths.add(ev.path)
          this._pendingFiles.unshift({
            path: ev.path, name: ev.filename, size: 0,
            mod_time: Date.now() / 1000,
            is_video: !!ev.is_video,
          })
        }
        if (!this._flushTimer) {
          this._flushTimer = setTimeout(() => {
            this._flushTimer = null
            if (this._pendingFiles.length) {
              this.files = [...this._pendingFiles, ...this.files]
              this._pendingFiles = []
            }
          }, 200)
        }
      })
      EventsOn('download:progress', (ev) => { this.progress = ev })
      EventsOn('download:status', (msg) => { this.status = msg })
      EventsOn('download:finished', () => { this.downloading = false; this.rescan() })
    },
    async init() {
      this.bindEvents()
      this.demo = await IsDemo()
      this.loggedIn = await AuthStatus()
      if (this.loggedIn) this.loadStudents()
    },
    async login() {
      const url = await StartLogin(window.location.origin)
      window.location.href = url
    },
    async logout() {
      await Logout()
      this.loggedIn = false
      this.students = []
      this.selected = {}
    },
    async loadStudents() {
      try {
        this.students = await FetchStudents()
        const sel = {}
        for (const st of this.students) sel[st.object_id] = true
        this.selected = sel
        this.loadPreview('', '')
      } catch (e) {
        this.status = 'Failed to fetch students: ' + e
      }
    },
    // loadPreview fetches a sample of not-yet-downloaded media for the
    // selected students to show as grayscale thumbnails.
    async loadPreview(startDate, endDate) {
      if (!this.loggedIn || this.selectedIds.length === 0) {
        this.pending = []
        return
      }
      this.previewLoading = true
      try {
        this.pending = await PreviewMedia(this.selectedIds, startDate || '', endDate || '')
      } catch (e) {
        this.status = 'Preview failed: ' + e
      } finally {
        this.previewLoading = false
      }
    },
    toggleAll() {
      const v = !this.allSelected
      const sel = {}
      for (const st of this.students) sel[st.object_id] = v
      this.selected = sel
    },
    async pickFolder() {
      const dir = await ChooseFolder()
      if (dir) {
        this.folder = dir
        this.rescan()
      }
    },
    async rescan() {
      if (!this.folder) return
      this.files = await ScanFolder(this.folder)
      this._knownPaths = new Set(this.files.map((f) => f.path))
      this._pendingFiles = []
      this.thumbs = {}
    },
    async startDownload(startDate, endDate) {
      this.progress = { total: 0, done: 0 }
      this.downloading = true
      try {
        await StartDownload(this.folder, this.selectedIds, startDate, endDate)
      } catch (e) {
        this.status = String(e)
        this.downloading = false
      }
    },
    async cancel() { await CancelDownload() },
    // studentIdForFile maps a downloaded filename back to a student via
    // the safe-name prefix used in the filename scheme.
    studentIdForFile(filename) {
      for (const st of this.students) {
        const prefix = safeName(`${st.first_name} ${st.last_name}`.trim()) + '-'
        if (filename.startsWith(prefix)) return st.object_id
      }
      return null
    },
    async thumb(file) {
      if (this.thumbs[file.path] !== undefined) return this.thumbs[file.path]
      this.thumbs[file.path] = null
      try {
        this.thumbs[file.path] = await GetThumbnail(file.path, 256)
      } catch (e) {
        this.thumbs[file.path] = ''
      }
      return this.thumbs[file.path]
    },
  },
})
