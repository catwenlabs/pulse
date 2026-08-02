export type SourceKind = 'rss' | 'json-api' | 'html' | 'webhook' | 'manual' | 'file' | 'annotations'

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
  unread_count: number
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
  source_ids: string[]
}

export interface CreateSourceInput {
  name: string
  kind: SourceKind
  locator: string
  config?: Record<string, unknown>
}

export interface UpdateSourceInput {
  name: string
  locator: string
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
  author?: string
  summary?: string
  content_html?: string
  published_at?: string
  discovered_at: string
  annotation?: AnnotationDetail
}

export interface Tag {
  id: string
  name: string
}

export interface StoryRef {
  id: string
  entry_count: number
  source_count: number
  display_title?: string
  note?: string
  tags?: Tag[]
  read_at?: string
  starred_at?: string
  hidden_at?: string
  later_at?: string
}

export interface SourceEntry {
  entry: Entry
  story: StoryRef
}

export interface ReaderCounts {
  inbox_stories: number
  unread_stories: number
  starred_stories: number
  later_stories: number
  hidden_stories: number
}

export interface Story {
  id: string
  display_title?: string
  note?: string
  tags?: Tag[]
  representative: Entry
  entries?: Entry[]
  matched_entry?: Entry
  entry_count: number
  source_count: number
  first_published_at?: string
  last_published_at?: string
  read_at?: string
  starred_at?: string
  hidden_at?: string
  later_at?: string
}

export interface StoryPage {
  stories: Story[]
  total_stories: number
  reader_counts: ReaderCounts
  next_cursor?: string
}

export interface SourceEntryPage {
  entries: SourceEntry[]
  total_entries: number
  reader_counts: ReaderCounts
  next_cursor?: string
}

export interface EntryQuery {
  q?: string
  state?: string
  tag?: string
  sourceId?: string
  limit?: number
  cursor?: string
}

export interface StoryPatch {
  read?: boolean
  starred?: boolean
  hidden?: boolean
  later?: boolean
  display_title?: string
  note?: string
}

export interface SplitOptions {
  copy_display_title?: boolean
  move_display_title?: boolean
  copy_note?: boolean
  move_note?: boolean
  copy_tags?: boolean
  move_tags?: boolean
}

export interface ManualEntryInput {
  url: string
  title: string
}

export interface AnnotationInput {
  id?: string
  provider: string
  book_identity?: string
  book_title: string
  book_author?: string
  chapter?: string
  location?: string
  highlight_color?: string
  highlight: string
  note?: string
  highlighted_at?: string
}

export interface AnnotationDetail {
  provider: string
  book_identity: string
  book_title: string
  book_author: string
  chapter: string
  location: string
  highlight_color: string
  annotation_note: string
  highlighted_at?: string
}

export interface Problem {
	code?: string
	detail?: string
	story_id?: string
	display_title?: string
	note?: string
	entry_count?: number
}

export interface DeletionConfirmation {
  code?: string
  detail?: string
  story_id?: string
  display_title?: string
  note?: string
  entry_count?: number
}

export class APIError extends Error {
  readonly status: number
  readonly problem: DeletionConfirmation

  constructor(status: number, message: string, problem: DeletionConfirmation = {}) {
    super(message)
    this.name = 'APIError'
    this.status = status
    this.problem = problem
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init)
  if (!response.ok) {
    let message = `请求失败（${response.status}）`
    let problem: Problem = {}
    try {
      problem = (await response.json()) as Problem
      if (problem.detail) message = problem.detail
    } catch {
      // Keep the status-based fallback when the server did not return JSON.
    }
    throw new APIError(response.status, message, problem)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

async function requestList<T>(path: string): Promise<T[]> {
  return (await request<T[] | null>(path)) ?? []
}

export function listSources(): Promise<Source[]> {
  return requestList<Source>('/api/v1/sources')
}

export function listFolders(): Promise<Folder[]> {
  return requestList<Folder>('/api/v1/folders')
}

export function createFolder(name: string): Promise<Folder> {
  return request<Folder>('/api/v1/folders', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

export function addSourceToFolder(folderId: string, sourceId: string): Promise<void> {
  return request<void>(`/api/v1/folders/${folderId}/sources/${sourceId}`, { method: 'PUT' })
}

export function removeSourceFromFolder(folderId: string, sourceId: string): Promise<void> {
  return request<void>(`/api/v1/folders/${folderId}/sources/${sourceId}`, { method: 'DELETE' })
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

export function updateSource(sourceId: string, input: UpdateSourceInput): Promise<Source> {
  return request<Source>(`/api/v1/sources/${sourceId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function archiveSource(sourceId: string): Promise<void> {
  return request<void>(`/api/v1/sources/${sourceId}`, { method: 'DELETE' })
}

export function getSourceHealth(sourceId: string): Promise<SourceHealth> {
  return request<SourceHealth>(`/api/v1/sources/${sourceId}/health`)
}

export async function checkServiceHealth(signal?: AbortSignal): Promise<boolean> {
  try {
    const response = await fetch('/healthz', { signal })
    return response.ok
  } catch {
    return false
  }
}

export function previewSource(input: CreateSourceInput): Promise<PreviewResult> {
  return request<PreviewResult>('/api/v1/sources/preview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function createManualEntry(
  sourceId: string,
  input: ManualEntryInput,
): Promise<{ id: string; status: string }> {
  const idempotencyKey = typeof globalThis.crypto?.randomUUID === 'function'
    ? globalThis.crypto.randomUUID()
    : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
  return request(`/api/v1/sources/${sourceId}/entries`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': idempotencyKey,
    },
    body: JSON.stringify(input),
  })
}

export function importAnnotations(
  sourceId: string,
  annotations: AnnotationInput[],
): Promise<{ id: string; status: string }> {
  const idempotencyKey = typeof globalThis.crypto?.randomUUID === 'function'
    ? globalThis.crypto.randomUUID()
    : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
  return request(`/api/v1/sources/${sourceId}/annotations`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': idempotencyKey,
    },
    body: JSON.stringify({ annotations }),
  })
}

export function listSourceEntries(sourceId: string, query: EntryQuery = {}): Promise<SourceEntryPage> {
  const parameters = new URLSearchParams()
  if (query.q) parameters.set('q', query.q)
  if (query.state) parameters.set('state', query.state)
  if (query.tag) parameters.set('tag', query.tag)
  if (query.limit) parameters.set('limit', String(query.limit))
  if (query.cursor) parameters.set('cursor', query.cursor)
  const suffix = parameters.size > 0 ? `?${parameters}` : ''
  return request<SourceEntryPage>(`/api/v1/sources/${sourceId}/entries${suffix}`)
}

export function listStories(query: EntryQuery = {}): Promise<StoryPage> {
  const parameters = new URLSearchParams()
  if (query.q) parameters.set('q', query.q)
  if (query.state) parameters.set('state', query.state)
  if (query.tag) parameters.set('tag', query.tag)
  if (query.sourceId) parameters.set('source_id', query.sourceId)
  if (query.limit) parameters.set('limit', String(query.limit))
  if (query.cursor) parameters.set('cursor', query.cursor)
  const suffix = parameters.size > 0 ? `?${parameters}` : ''
  return request<StoryPage>(`/api/v1/stories${suffix}`)
}

export function getStory(id: string): Promise<Story> {
	return request<Story>(`/api/v1/stories/${id}`)
}

export function deleteEntry(id: string, confirm = false): Promise<void> {
  const suffix = confirm ? '?confirm=true' : ''
  return request<void>(`/api/v1/entries/${id}${suffix}`, { method: 'DELETE' })
}

export function updateStory(
  id: string,
  patch: StoryPatch,
): Promise<Story> {
  return request<Story>(`/api/v1/stories/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  })
}

export function setStoryRepresentative(storyId: string, entryId: string): Promise<Story> {
  return request<Story>(`/api/v1/stories/${storyId}/representative`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ entry_id: entryId }),
  })
}

export function markStoriesRead(sourceId?: string): Promise<{ updated_count: number }> {
  const parameters = new URLSearchParams()
  if (sourceId) parameters.set('source_id', sourceId)
  const suffix = parameters.size > 0 ? `?${parameters}` : ''
  return request<{ updated_count: number }>(`/api/v1/stories${suffix}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ read: true }),
  })
}

export function mergeStory(storyId: string, intoStoryId: string, options: { display_title?: string; note?: string } = {}): Promise<Story> {
  return request<Story>(`/api/v1/stories/${storyId}/merge`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ into: intoStoryId, ...options }),
  })
}

export function splitStory(storyId: string, entryId: string, options: SplitOptions = {}): Promise<Story> {
  return request<Story>(`/api/v1/stories/${storyId}/split`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ entry_id: entryId, ...options }),
  })
}

export function addStoryTag(storyId: string, name: string): Promise<Tag> {
  return request<Tag>(`/api/v1/stories/${storyId}/tags`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

export function removeStoryTag(storyId: string, tagId: string): Promise<void> {
  return request<void>(`/api/v1/stories/${storyId}/tags/${tagId}`, {
    method: 'DELETE',
  })
}

export function reclusterStories(): Promise<{ processed: number }> {
  return request<{ processed: number }>('/api/v1/stories/recluster', {
    method: 'POST',
  })
}
