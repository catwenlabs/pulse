import { FileText, Loader2 } from 'lucide-react'

import type { StoryAISummary } from '../api'
import { Button } from './ui/button'

export const statusLabels: Record<string, string> = {
  not_requested: '尚未生成',
  queued: '排队中',
  pending: '排队中',
  running: '生成中',
  retry: '等待重试',
  completed: '已完成',
  partial: '部分完成',
  failed: '生成失败',
  dead: '已停止',
  stale: '内容已变化',
  unavailable: 'AI 不可用',
}

export function isStoredStorySummary(summary?: StoryAISummary) {
  return Boolean(summary && summary.status !== 'not_requested')
}

export function isActiveStorySummary(summary?: StoryAISummary) {
  return summary?.status === 'queued' || summary?.status === 'running'
}

export function StorySummaryCard({
  summary,
  loading = false,
  loadError = null,
  onRetry,
}: {
  summary?: StoryAISummary
  loading?: boolean
  loadError?: Error | null
  onRetry?: () => void
}) {
  if (loading) {
    return (
      <section className="ai-summary-card story-summary-card" aria-live="polite" role="status">
        <div className="ai-loading-state">
          <span className="ai-loading-icon" aria-hidden="true"><Loader2 size={22} className="animate-spin motion-reduce:animate-none" /></span>
          <strong>正在加载 AI 摘要</strong>
          <p>正在读取这个 Story 的摘要状态，请稍候。</p>
        </div>
      </section>
    )
  }
  if (loadError) {
    return (
      <section className="ai-summary-card story-summary-card" role="alert">
        <div className="ai-loading-state ai-error">
          <strong>暂时无法加载 AI 摘要</strong>
          <p>{loadError.message}</p>
          {onRetry && <Button variant="secondary" size="sm" onClick={onRetry}>重试加载</Button>}
        </div>
      </section>
    )
  }
  if (!summary || summary.status === 'not_requested') {
    return (
      <section className="ai-summary-card story-summary-card pb-6" aria-labelledby="story-summary-title">
        <header className="ai-card-heading story-summary-heading">
          <div className="ai-card-heading-title">
            <span className="ai-card-icon" aria-hidden="true"><FileText size={18} /></span>
            <div>
              <p className="ai-eyebrow">AI STORY SUMMARY</p>
              <h2 id="story-summary-title">内容摘要</h2>
            </div>
          </div>
          <span className="story-summary-request-status">按需生成</span>
        </header>
        <p className="story-summary-empty-copy">还没有摘要。点击「生成AI摘要」后，AI 才会读取这个 Story 的内容。</p>
      </section>
    )
  }
  return (
    <section className="ai-summary-card story-summary-card pb-6" aria-labelledby="story-summary-title">
      <header className="ai-card-heading story-summary-heading">
        <div className="ai-card-heading-title">
          <span className="ai-card-icon" aria-hidden="true"><FileText size={18} /></span>
          <div>
            <p className="ai-eyebrow">AI STORY SUMMARY</p>
            <h2 id="story-summary-title">内容摘要</h2>
          </div>
        </div>
        <span className={`ai-status ai-status-${summary.status}`}>{statusLabels[summary.status] ?? summary.status}</span>
      </header>
      {summary.error && <p className="ai-error-box">{summary.error}</p>}
      {isActiveStorySummary(summary) && (
        <p className="story-summary-processing" role="status">
          <Loader2 size={15} className="animate-spin motion-reduce:animate-none" aria-hidden="true" />
          摘要正在生成，完成后会自动更新。
        </p>
      )}
      {summary.overview && (
        <div className="ai-overview">
          <span className="ai-overview-label">AI 速览</span>
          <p>{summary.overview}</p>
        </div>
      )}
      {summary.key_points && summary.key_points.length > 0 && (
        <section className="story-summary-section" aria-labelledby="story-key-points-title">
          <div className="story-summary-section-heading">
            <h3 id="story-key-points-title">重点提要</h3>
            <span>{summary.key_points.length} 条</span>
          </div>
          <ul className="ai-key-point-list">{summary.key_points.map((point) => <li key={point}>{point}</li>)}</ul>
        </section>
      )}
      {summary.sources && summary.sources.length > 0 && (
        <section className="ai-source-section story-summary-sources" aria-labelledby="story-summary-sources-title">
          <div className="ai-subsection-heading">
            <div>
              <p className="ai-eyebrow">REFERENCES</p>
              <h3 id="story-summary-sources-title">来源说明</h3>
            </div>
            <span>{summary.sources.length} 个</span>
          </div>
          <div className="ai-summary-source-list">
            {summary.sources.map((source) => (
              <a className="ai-summary-source" href={`#entry-${source.entry_id}`} key={source.entry_id}>
                <strong>{source.label} · {source.title}</strong>
                <span>{source.note || '来源已纳入摘要'}</span>
              </a>
            ))}
          </div>
        </section>
      )}
    </section>
  )
}
