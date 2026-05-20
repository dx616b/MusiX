import { Link } from 'react-router-dom'
import { normalizeSearchQuery } from '../api'

type Props = {
  query: string
  /** Normalized query keys from listSearches (optional; avoids extra API calls). */
  historyKeys?: ReadonlySet<string>
  className?: string
}

export function searchPathForQuery(query: string, historyKeys?: ReadonlySet<string>): string {
  const q = query.trim()
  if (!q) return '/'
  const inHistory = historyKeys?.has(normalizeSearchQuery(q)) ?? false
  const params = new URLSearchParams({ q })
  if (inHistory) {
    return `/searches?${params}`
  }
  return `/?${params}`
}

export default function SearchQueryLink({ query, historyKeys, className = 'text-btn' }: Props) {
  const q = query.trim()
  if (!q) return null
  const to = searchPathForQuery(q, historyKeys)
  const title = historyKeys?.has(normalizeSearchQuery(q))
    ? 'Open saved search in History'
    : 'Run a new search'
  return (
    <Link to={to} className={className} title={title}>
      {q}
    </Link>
  )
}
