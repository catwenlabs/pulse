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
  ai_summary?: StoryAISummary
}

export type AIJobKind = 'story_summary' | 'digest'
export type AIJobStatus = 'pending' | 'running' | 'retry' | 'completed' | 'partial' | 'failed' | 'dead'
export type AIArtifactStatus = 'not_requested' | 'queued' | 'running' | 'completed' | 'partial' | 'failed' | 'stale' | 'unavailable'

export interface AIJob {
  id: string
  kind: AIJobKind
  target_id: string
  status: AIJobStatus
}

export interface SummarySource {
  label: string
  entry_id: string
  title: string
  source_title?: string
  note: string
}

export interface StoryAISummary {
  story_id: string
  status: AIArtifactStatus
  overview?: string
  key_points?: string[]
  sources?: SummarySource[]
  provider?: string
  model?: string
  prompt_version?: string
  input_fingerprint?: string
  error?: string
  created_at?: string
  updated_at?: string
}

export interface DigestScope {
  start_at?: string
  end_at?: string
  max_stories?: number
}

export interface DigestPreview {
  scope: DigestScope
  matching_stories: number
  matching_stories_truncated: boolean
  selected_stories: number
  safety_limit: number
  can_queue: boolean
}

export interface DigestTheme {
  title: string
  summary: string
  story_ids: string[]
}

export interface DigestPriority {
  rank: number
  title: string
  reason: string
  story_ids: string[]
}

export interface DigestStory {
  label: string
  story_id: string
  title: string
  entry_count: number
  source_count: number
  available: boolean
}

export interface DigestOmission {
  label: string
  story_id?: string
  title: string
  reason: string
}

export interface Digest {
  id: string
  status: Exclude<AIArtifactStatus, 'not_requested' | 'stale' | 'unavailable'>
  mode: 'catch_up'
  story_count: number
  start_at?: string
  end_at?: string
  overview?: string
  themes?: DigestTheme[]
  priorities?: DigestPriority[]
  stories?: DigestStory[]
  omissions?: DigestOmission[]
  provider?: string
  model?: string
  prompt_version?: string
  input_fingerprint?: string
  error?: string
  created_at?: string
  updated_at?: string
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

export interface LibraryChangeEvent {
  source_id?: string
}

export const libraryEventsPath = '/api/v1/events'

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

export function reorderRootSources(sourceIDs: string[]): Promise<void> {
  return request<void>('/api/v1/sources/order', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ source_ids: sourceIDs }),
  })
}

export function reorderFolders(folderIDs: string[]): Promise<void> {
  return request<void>('/api/v1/folders/order', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ folder_ids: folderIDs }),
  })
}

export function reorderFolderSources(folderID: string, sourceIDs: string[]): Promise<void> {
  return request<void>(`/api/v1/folders/${folderID}/sources/order`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ source_ids: sourceIDs }),
  })
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

export function requestStorySummary(storyID: string): Promise<AIJob> {
  return request<AIJob>(`/api/v1/stories/${storyID}/ai-summary`, {
    method: 'POST',
  })
}

export function listDigests(limit = 50): Promise<Digest[]> {
  const parameters = new URLSearchParams({ limit: String(limit) })
  return requestList<Digest>(`/api/v1/digests?${parameters}`)
}

export function previewDigest(scope: DigestScope = {}): Promise<DigestPreview> {
  const parameters = new URLSearchParams()
  if (scope.start_at) parameters.set('start_at', scope.start_at)
  if (scope.end_at) parameters.set('end_at', scope.end_at)
  if (scope.max_stories !== undefined) parameters.set('max_stories', String(scope.max_stories))
  const suffix = parameters.size > 0 ? `?${parameters}` : ''
  return request<DigestPreview>(`/api/v1/digests/preview${suffix}`)
}

export function createDigest(scope: DigestScope = {}): Promise<AIJob> {
  return request<AIJob>('/api/v1/digests', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(scope),
  })
}

export function getDigest(digestID: string): Promise<Digest> {
  return request<Digest>(`/api/v1/digests/${digestID}`)
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

export function markStoriesRead(options: { sourceId?: string; storyIDs?: string[] } = {}): Promise<{ updated_count: number }> {
  const parameters = new URLSearchParams()
  if (options.sourceId) parameters.set('source_id', options.sourceId)
  const suffix = parameters.size > 0 ? `?${parameters}` : ''
  const body = {
    read: true,
    ...(options.storyIDs && options.storyIDs.length > 0 ? { story_ids: options.storyIDs } : {}),
  }
  return request<{ updated_count: number }>(`/api/v1/stories${suffix}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export function markDigestRead(digestId: string): Promise<{ updated_count: number }> {
  return request<{ updated_count: number }>(`/api/v1/digests/${digestId}/mark-read`, {
    method: 'POST',
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

export interface SelectionTool {
  id: string
  name: string
  prompt_template: string
  enabled: boolean
  position: number
  created_at: string
  updated_at: string
}

export interface SelectionToolInput {
  name: string
  prompt_template: string
  enabled: boolean
}

export interface ChatConversation {
  id: string
  selected_text: string
  tool_name: string
  prompt_template: string
  created_at: string
  updated_at: string
}

export interface ConversationPage {
  items: ChatConversation[]
  next_cursor?: string
  has_more: boolean
}

export type ChatMessageRole = 'user' | 'assistant'
export type ChatMessageStatus = 'streaming' | 'completed' | 'cancelled' | 'failed'

export interface ChatMessage {
  id: string
  conversation_id: string
  role: ChatMessageRole
  content: string
  status?: ChatMessageStatus
  provider?: string
  model?: string
  prompt_tokens?: number
  completion_tokens?: number
  finish_reason?: string
  error?: string
  created_at: string
  updated_at: string
}

export interface ConversationDetail {
  conversation: ChatConversation
  messages: ChatMessage[]
}

export interface ConversationCreated {
  conversation: ChatConversation
  user_message: ChatMessage
}

export interface CreateConversationInput {
  tool_id: string
  selection: string
}

export type ChatStreamEventKind = 'metadata' | 'delta' | 'completed' | 'cancelled' | 'failed'

export interface ChatStreamEvent {
  kind: ChatStreamEventKind
  conversation_id?: string
  message_id?: string
  delta?: string
  content?: string
  status?: ChatMessageStatus
  provider?: string
  model?: string
  prompt_tokens?: number
  completion_tokens?: number
  finish_reason?: string
  error?: string
}

export function listSelectionTools(): Promise<SelectionTool[]> {
  return requestList<SelectionTool>('/api/v1/ai/tools')
}

export function createSelectionTool(input: SelectionToolInput): Promise<SelectionTool> {
  return request<SelectionTool>('/api/v1/ai/tools', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function updateSelectionTool(id: string, input: SelectionToolInput): Promise<SelectionTool> {
  return request<SelectionTool>(`/api/v1/ai/tools/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function deleteSelectionTool(id: string): Promise<void> {
  return request<void>(`/api/v1/ai/tools/${id}`, { method: 'DELETE' })
}

export function reorderSelectionTools(toolIds: string[]): Promise<SelectionTool[]> {
  return request<SelectionTool[]>('/api/v1/ai/tools/order', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tool_ids: toolIds }),
  })
}

export function listConversations(limit = 50, cursor = ''): Promise<ConversationPage> {
  const parameters = new URLSearchParams({ limit: String(limit) })
  if (cursor) parameters.set('cursor', cursor)
  return request<ConversationPage>(`/api/v1/ai/conversations?${parameters}`)
}

export function getConversation(id: string): Promise<ConversationDetail> {
  return request<ConversationDetail>(`/api/v1/ai/conversations/${id}`)
}

export function deleteConversation(id: string): Promise<void> {
  return request<void>(`/api/v1/ai/conversations/${id}`, { method: 'DELETE' })
}

export function createConversation(input: CreateConversationInput, idempotencyKey: string): Promise<ConversationCreated> {
  return request<ConversationCreated>('/api/v1/ai/conversations', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify(input),
  })
}

export function sendFollowUp(conversationId: string, content: string, idempotencyKey: string): Promise<ChatMessage> {
  return request<ChatMessage>(`/api/v1/ai/conversations/${conversationId}/messages`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify({ content }),
  })
}

export function stopGeneration(conversationId: string): Promise<void> {
  return request<void>(`/api/v1/ai/conversations/${conversationId}/stop`, { method: 'POST' })
}

/**
 * Drives an Assistant generation stream. Resolves with the terminal event
 * (completed / cancelled / failed). The caller passes an onEvent callback that
 * receives metadata and delta events as they arrive. An AbortSignal cancels the
 * underlying fetch, which the server treats as a genuine disconnect (failed).
 */
export async function streamAssistant(
  conversationId: string,
  mode: 'generate' | 'retry',
  idempotencyKey: string,
  onEvent: (event: ChatStreamEvent) => void,
  signal?: AbortSignal,
): Promise<ChatStreamEvent> {
  const response = await fetch(`/api/v1/ai/conversations/${conversationId}/${mode}`, {
    method: 'POST',
    headers: { 'Idempotency-Key': idempotencyKey },
    signal,
  })
  if (!response.ok || !response.body) {
    let detail = `请求失败（${response.status}）`
    try {
      const problem = (await response.json()) as Problem
      if (problem.detail) detail = problem.detail
    } catch {
      // Keep the status-based fallback.
    }
    throw new APIError(response.status, detail)
  }
  return consumeChatStream(response.body, onEvent)
}

/**
 * Parses the text/event-stream response into ChatStreamEvents. Exposed for
 * testing so the parser can be verified without a network round-trip.
 */
export async function consumeChatStream(
  body: ReadableStream<Uint8Array>,
  onEvent: (event: ChatStreamEvent) => void,
): Promise<ChatStreamEvent> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let terminal: ChatStreamEvent | undefined
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const frames = buffer.split('\n\n')
    buffer = frames.pop() ?? ''
    for (const frame of frames) {
      const event = parseChatFrame(frame)
      if (!event) continue
      onEvent(event)
      if (event.kind === 'completed' || event.kind === 'cancelled' || event.kind === 'failed') {
        terminal = event
      }
    }
  }
  const trailing = parseChatFrame(buffer)
  if (trailing) {
    onEvent(trailing)
    if (trailing.kind === 'completed' || trailing.kind === 'cancelled' || trailing.kind === 'failed') {
      terminal = trailing
    }
  }
  if (!terminal) {
    throw new APIError(0, 'AI 流式响应意外结束')
  }
  return terminal
}

function parseChatFrame(frame: string): ChatStreamEvent | undefined {
  let dataLine = ''
  for (const line of frame.split('\n')) {
    if (line.startsWith('data: ')) dataLine += line.slice('data: '.length)
    else if (line.startsWith('data:')) dataLine += line.slice('data:'.length)
  }
  if (!dataLine) return undefined
  try {
    return JSON.parse(dataLine) as ChatStreamEvent
  } catch {
    return undefined
  }
}
