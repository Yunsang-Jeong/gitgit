import type { RemoteBadgeRule, RemoteInfo } from './types'

export interface RefBadge {
  ref: string
  label: string
  branch: string
  icon: string
  remote: boolean
  title: string
}

export interface RefBadgeSummary {
  primary: RefBadge | null
  remaining: RefBadge[]
}

export interface InspectorRefContext {
  label: 'Refs' | 'Branch' | 'Branches'
  values: string[]
}

export const remoteBadgeIconOptions = [
  { id: 'github', label: 'GitHub' },
  { id: 'gitlab', label: 'GitLab' },
  { id: 'bitbucket', label: 'Bitbucket' },
  { id: 'azure-devops', label: 'Azure DevOps' },
  { id: 'codeberg', label: 'Codeberg' },
  { id: 'gitea', label: 'Gitea' },
  { id: 'git', label: 'Git' },
  { id: 'cloud', label: 'Cloud' },
  { id: 'server', label: 'Self-hosted server' },
  { id: 'remote', label: 'Generic remote' },
] as const

export const defaultRemoteBadgeRules = (): RemoteBadgeRule[] => [
  { id: 'github', pattern: 'github.com', icon: 'github' },
  { id: 'gitlab', pattern: 'gitlab.com', icon: 'gitlab' },
]

export function normalizeRemoteBadgeIcon(value: string): string {
  const normalized = value.trim()
  if (normalized === '🐙') return 'github'
  if (normalized === '🦊') return 'gitlab'
  if (normalized === '🔗' || !normalized) return 'remote'
  return normalized
}

export function isEmbeddedRemoteBadgeIcon(value: string): boolean {
  const normalized = normalizeRemoteBadgeIcon(value)
  return remoteBadgeIconOptions.some((option) => option.id === normalized)
}

export function remoteBadgeIconLabel(value: string): string {
  const normalized = normalizeRemoteBadgeIcon(value)
  return remoteBadgeIconOptions.find((option) => option.id === normalized)?.label ?? normalized
}

export function defaultFirstRefs(refs: string[] | undefined, defaultBranch: string): string[] {
  const ordered = [...(refs ?? [])]
  const defaultIndex = ordered.indexOf(defaultBranch)
  if (defaultIndex <= 0) return ordered
  ordered.splice(defaultIndex, 1)
  ordered.unshift(defaultBranch)
  return ordered
}

export function inspectorRefContext(refs: string[] | undefined, branches: string[] | undefined, defaultBranch: string, historicalBranch = ''): InspectorRefContext {
  const exactRefs = defaultFirstRefs(refs, defaultBranch)
  if (exactRefs.length > 0) return { label: 'Refs', values: exactRefs }
  if (historicalBranch) return { label: 'Branch', values: [historicalBranch] }
  const containingBranches = defaultFirstRefs(branches, defaultBranch)
  if (containingBranches.length > 0) return { label: 'Branches', values: containingBranches }
  return { label: 'Refs', values: [] }
}

export function resolveRefBadge(ref: string, remotes: RemoteInfo[], rules: RemoteBadgeRule[]): RefBadge {
  const remote = remotes.find((candidate) => ref.startsWith(`${candidate.name}/`))
  if (!remote) return { ref, label: ref, branch: ref, icon: '', remote: false, title: ref }

  const branch = ref.slice(remote.name.length + 1)
  const normalizedURL = remote.url.toLocaleLowerCase()
  const rule = rules.find((candidate) => candidate.pattern.trim() && normalizedURL.includes(candidate.pattern.trim().toLocaleLowerCase()))
  const icon = normalizeRemoteBadgeIcon(rule?.icon ?? 'remote')
  return {
    ref,
    label: `${remoteBadgeIconLabel(icon)}/${branch}`,
    branch,
    icon,
    remote: true,
    title: `${remote.name}/${branch} · ${remote.url}`,
  }
}

export function visibleRefBadges(refs: string[] | undefined, remotes: RemoteInfo[], rules: RemoteBadgeRule[], showRemotes: boolean, defaultBranch = ''): RefBadge[] {
  return defaultFirstRefs(refs, defaultBranch)
    .map((ref) => resolveRefBadge(ref, remotes, rules))
    .filter((badge) => showRemotes || !badge.remote)
}

export function summarizeRefBadges(refs: string[] | undefined, remotes: RemoteInfo[], rules: RemoteBadgeRule[], showRemotes: boolean, defaultBranch = ''): RefBadgeSummary {
  const badges = visibleRefBadges(refs, remotes, rules, showRemotes, defaultBranch)
  return {
    primary: badges[0] ?? null,
    remaining: badges.slice(1),
  }
}

export function isLocalDefaultBranchBadge(badge: RefBadge | null, defaultBranch: string): boolean {
  return Boolean(defaultBranch) && badge?.ref === defaultBranch
}
