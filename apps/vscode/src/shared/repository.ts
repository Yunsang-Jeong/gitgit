import * as path from 'node:path'

export function isCommitObjectID(value: string): boolean {
  return /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/u.test(value)
}

export function isSafeRepositoryRelativePath(value: string): boolean {
  if (!value || /[\0\n\r]/u.test(value) || value.includes('\\') || value.startsWith('/') || /^[A-Za-z]:/u.test(value)) {
    return false
  }
  const segments = value.split('/')
  if (segments.some((segment) => !segment || segment === '.' || segment === '..')) return false
  return segments[0]?.toLocaleLowerCase() !== '.git'
}

export function repositoryRelativePath(repositoryRoot: string, filePath: string): string | undefined {
  const relative = path.relative(path.resolve(repositoryRoot), path.resolve(filePath))
  if (relative === '' || relative === '..' || relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative)) {
    return undefined
  }
  return relative.split(path.sep).join('/')
}

export function pathIsWithin(parentPath: string, candidatePath: string): boolean {
  const relative = path.relative(path.resolve(parentPath), path.resolve(candidatePath))
  return relative === '' || (!path.isAbsolute(relative) && relative !== '..' && !relative.startsWith(`..${path.sep}`))
}

export interface RevisionRepositoryTarget {
  candidates: string[]
  expectedRoot: string
}

export function revisionRepositoryTarget(
  repositoryRoot: string,
  currentRoot: string | undefined,
  workspaceRoots: readonly string[],
): RevisionRepositoryTarget | undefined {
  if (currentRoot === repositoryRoot) return { candidates: [repositoryRoot], expectedRoot: repositoryRoot }
  if (workspaceRoots.some((workspaceRoot) => pathIsWithin(workspaceRoot, repositoryRoot))) {
    return { candidates: [repositoryRoot], expectedRoot: repositoryRoot }
  }
  const candidates = [...new Set(workspaceRoots.filter((workspaceRoot) => pathIsWithin(repositoryRoot, workspaceRoot)))]
  return candidates.length > 0 ? { candidates, expectedRoot: repositoryRoot } : undefined
}
