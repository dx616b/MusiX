import type { MouseEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { getStoredSearch } from '../api'

type Props = {
  query: string
  className?: string
}

export default function SearchQueryLink({ query, className = 'text-btn' }: Props) {
  const navigate = useNavigate()
  const q = query.trim()
  if (!q) return null

  async function open(e: MouseEvent) {
    e.preventDefault()
    try {
      await getStoredSearch(q)
      navigate(`/searches?q=${encodeURIComponent(q)}`)
    } catch {
      navigate(`/?q=${encodeURIComponent(q)}`)
    }
  }

  return (
    <a
      href={`/searches?q=${encodeURIComponent(q)}`}
      className={className}
      title="Open saved search or run a new search"
      onClick={(e) => void open(e)}
    >
      {q}
    </a>
  )
}
