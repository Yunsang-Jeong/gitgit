import type { SparseState } from './types'

export type SparseAction = 'set' | 'expand' | 'contract' | 'none'

export type SparsePlan = {
  action: SparseAction
  directories: string[]
  added: string[]
  removed: string[]
}

// Cone mode selects a directory and everything under it, so naming a child
// beside its ancestor says nothing new. Collapsing them keeps the request and
// the diff below honest about what actually changes.
export function normalizeConeSelection(paths: Iterable<string>): string[] {
  const cleaned = [...new Set([...paths].map(normalizeConePath).filter((path) => path !== ''))].sort()
  return cleaned.filter((path) => !cleaned.some((other) => other !== path && isConeAncestor(other, path)))
}

function normalizeConePath(path: string): string {
  return path.trim().replace(/^\/+/, '').replace(/\/+$/, '')
}

function isConeAncestor(ancestor: string, path: string): boolean {
  return path.startsWith(ancestor + '/')
}

export function sparseSelectionDiff(current: string[], next: string[]): { added: string[]; removed: string[] } {
  const currentSet = new Set(current)
  const nextSet = new Set(next)
  return {
    added: next.filter((path) => !currentSet.has(path)),
    removed: current.filter((path) => !nextSet.has(path)),
  }
}

// `disable` is deliberately not an outcome here. An empty selection is
// ambiguous between "root files only" and "give me the whole checkout back",
// so the dialog asks for that explicitly instead of guessing.
export function sparsePlan(current: SparseState, selection: Iterable<string>): SparsePlan {
  const next = normalizeConeSelection(selection)
  if (!current.enabled) {
    return { action: next.length > 0 ? 'set' : 'none', directories: next, added: next, removed: [] }
  }

  const existing = normalizeConeSelection(current.directories ?? [])
  const { added, removed } = sparseSelectionDiff(existing, next)
  if (added.length === 0 && removed.length === 0) {
    return { action: 'none', directories: next, added, removed }
  }
  if (removed.length === 0) {
    return { action: 'expand', directories: added, added, removed }
  }
  if (added.length === 0) {
    // SparseContract removes entries by exact string match against what Git
    // reports, so the payload has to name those strings rather than the
    // normalized ones the picker works in.
    return { action: 'contract', directories: contractPayload(current.directories ?? [], removed), added, removed }
  }
  // A mixed change goes out as one atomic set, which is also the variant that
  // runs the dirty-path probe.
  return { action: 'set', directories: next, added, removed }
}

function contractPayload(reported: string[], removed: string[]): string[] {
  const removedSet = new Set(removed)
  const payload = reported.filter((directory) => {
    const normalized = normalizeConePath(directory)
    return removedSet.has(normalized) || removed.some((path) => isConeAncestor(path, normalized))
  })
  return payload.length > 0 ? payload : removed
}
