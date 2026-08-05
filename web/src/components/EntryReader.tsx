import type { ReactNode } from 'react'
import type { Entry } from '../api'
import { sanitizeEntryHTML } from '../lib/sanitizeEntryHTML'
import { cn } from '../lib/utils'

export type EntryReaderEntry = Pick<Entry, 'canonical_url' | 'content_html' | 'summary'>

export function getEntryReaderHTML(entry: EntryReaderEntry): string {
  const content = entry.content_html?.trim() || entry.summary?.trim() || ''
  return content ? sanitizeEntryHTML(content, entry.canonical_url) : ''
}

export function EntryReader({
  entry,
  className,
  title,
  empty,
}: {
  entry: EntryReaderEntry
  className?: string
  title?: string
  empty?: ReactNode
}) {
  const html = getEntryReaderHTML(entry)
  const readerTitle = title?.trim()
  if (!html && !readerTitle && !empty) return null

  return (
    <div className={cn('entry-prose mx-auto max-w-[75ch]', className)}>
      {readerTitle && (
        <h2 className="entry-reader-title">
          {entry.canonical_url ? (
            <a href={entry.canonical_url} rel="noreferrer" target="_blank" title="查看原文">
              {readerTitle}
            </a>
          ) : (
            readerTitle
          )}
        </h2>
      )}
      {html ? (
        <div className="entry-reader-content" dangerouslySetInnerHTML={{ __html: html }} />
      ) : (
        empty
      )}
    </div>
  )
}
