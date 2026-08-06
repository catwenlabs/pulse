import { memo } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'
import rehypeKatex from 'rehype-katex'

/**
 * Renders Assistant Markdown safely: fenced and inline code, inline (`$...$`)
 * and block (`$$...$$`) LaTeX, and sanitized links. Raw model-supplied HTML is
 * escaped (react-markdown does not pass it through), and dangerous link schemes
 * such as `javascript:` are dropped so model output cannot execute scripts.
 */
export const ChatMarkdown = memo(function ChatMarkdown({ content }: { content: string }) {
  return (
    <div className="chat-markdown text-sm leading-6 text-foreground">
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkMath]}
        rehypePlugins={[rehypeKatex]}
        urlTransform={sanitizeUrl}
        components={{
          a: ({ href, ...props }) => (
            href == null || href === ''
              ? <span {...props} />
              : <a {...props} href={href} target="_blank" rel="noopener noreferrer" />
          ),
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  )
})

/**
 * sanitizeUrl drops URLs that could execute scripts. react-markdown calls this
 * for every link/image source; we keep http(s), mailto, tel, and relative URLs
 * and reject everything else (notably javascript: and data:).
 */
export function sanitizeUrl(url: string | undefined): string | undefined {
  if (!url) return undefined
  const trimmed = url.trim()
  if (/^[a-z][a-z0-9+.-]*:/i.test(trimmed)) {
    const scheme = trimmed.slice(0, trimmed.indexOf(':')).toLowerCase()
    if (!['http', 'https', 'mailto', 'tel'].includes(scheme)) {
      return undefined
    }
  }
  return trimmed
}
