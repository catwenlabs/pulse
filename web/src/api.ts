export type SourceKind = 'rss' | 'json-api' | 'html' | 'webhook' | 'manual' | 'file'

export interface Source {
  id: string
  name: string
  kind: SourceKind
  locator: string
  normalized_locator: string
  config: Record<string, unknown>
  enabled: boolean
  created_at: string
  updated_at: string
  archived_at?: string
}

export interface SourceHealth {
  source_id: string
  status: string
  last_requested_at?: string
  last_finished_at?: string
  next_scheduled_at?: string
  duration_milliseconds: number
  candidate_count: number
  new_count: number
  updated_count: number
  consecutive_failures: number
  last_error?: string
}

export interface Folder {
  id: string
  name: string
  source_count: number
}

export interface CreateSourceInput {
  name: string
  kind: SourceKind
  locator: string
  config?: Record<string, unknown>
}

export interface PreviewCandidate {
  external_id?: string
  url?: string
  title?: string
  author?: string
  summary?: string
  published_at?: string
  identity_key: string
  identity_warning?: string
}

export interface PreviewResult {
  candidates: PreviewCandidate[]
  diagnostics: {
    status: string
    candidate_count: number
    details?: Record<string, string>
  }
}

export interface Entry {
  id: string
  source_id: string
  identity_key: string
  external_id?: string
  canonical_url?: string
  source_title: string
  display_title: string
  author?: string
  summary?: string
  content_html?: string
  discovered_at: string
  read_at?: string
  starred_at?: string
  hidden_at?: string
  later_at?: string
  note: string
}

export interface EntryQuery {
  q?: string
  state?: string
  tag?: string
  sourceId?: string
  limit?: number
}

export interface EntryPatch {
  read?: boolean
  starred?: boolean
  hidden?: boolean
  later?: boolean
  display_title?: string
  note?: string
}

interface Problem {
  detail?: string
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init)
  if (!response.ok) {
    let message = `请求失败（${response.status}）`
    try {
      const problem = (await response.json()) as Problem
      if (problem.detail) message = problem.detail
    } catch {
      // Keep the status-based fallback when the server did not return JSON.
    }
    throw new Error(message)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export function listSources(): Promise<Source[]> {
  return request<Source[]>('/api/v1/sources')
}

export function listFolders(): Promise<Folder[]> {
  return request<Folder[]>('/api/v1/folders')
}

export function createSource(input: CreateSourceInput): Promise<Source> {
  return request<Source>('/api/v1/sources', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function runSource(sourceId: string): Promise<{ id: string; status: string }> {
  return request(`/api/v1/sources/${sourceId}/runs`, { method: 'POST' })
}

export function setSourceEnabled(sourceId: string, enabled: boolean): Promise<Source> {
  return request(`/api/v1/sources/${sourceId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enabled }),
  })
}

export function archiveSource(sourceId: string): Promise<void> {
  return request<void>(`/api/v1/sources/${sourceId}`, { method: 'DELETE' })
}

export function getSourceHealth(sourceId: string): Promise<SourceHealth> {
  return request<SourceHealth>(`/api/v1/sources/${sourceId}/health`)
}

export function previewSource(input: CreateSourceInput): Promise<PreviewResult> {
  return request<PreviewResult>('/api/v1/sources/preview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function listEntries(query: EntryQuery = {}): Promise<Entry[]> {
  const parameters = new URLSearchParams()
  if (query.q) parameters.set('q', query.q)
  if (query.state) parameters.set('state', query.state)
  if (query.tag) parameters.set('tag', query.tag)
  if (query.sourceId) parameters.set('source_id', query.sourceId)
  if (query.limit) parameters.set('limit', String(query.limit))
  const suffix = parameters.size > 0 ? `?${parameters}` : ''
  return request<Entry[]>(`/api/v1/entries${suffix}`)
}

export function updateEntry(id: string, patch: EntryPatch): Promise<Entry> {
  return request<Entry>(`/api/v1/entries/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  })
}

export function markEntriesRead(sourceId?: string): Promise<{ updated_count: number }> {
  const parameters = new URLSearchParams()
  if (sourceId) parameters.set('source_id', sourceId)
  const suffix = parameters.size > 0 ? `?${parameters}` : ''
  return request<{ updated_count: number }>(`/api/v1/entries${suffix}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ read: true }),
  })
}
