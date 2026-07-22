import assert from 'node:assert/strict'
import test from 'node:test'

import {
  defaultRemoteBadgeRules,
  defaultFirstRefs,
  isEmbeddedRemoteBadgeIcon,
  inspectorRefContext,
  normalizeRemoteBadgeIcon,
  remoteBadgeIconOptions,
  resolveRefBadge,
  summarizeRefBadges,
  visibleRefBadges,
} from '../src/lib/remotes.ts'

const remotes = [
  { name: 'origin', url: 'git@github.com:yunsang/GitGit.git' },
  { name: 'company', url: 'https://mymy.gitlab.internal/platform/GitGit.git' },
  { name: 'mirror', url: 'ssh://git@example.internal/GitGit.git' },
]

test('major hosts use embedded provider icons and unknown hosts use remote', () => {
  const rules = defaultRemoteBadgeRules()
  assert.equal(resolveRefBadge('origin/main', remotes, rules).label, 'GitHub/main')
  assert.equal(resolveRefBadge('origin/main', remotes, rules).icon, 'github')
  assert.equal(resolveRefBadge('mirror/main', remotes, rules).label, 'Generic remote/main')
  assert.equal(resolveRefBadge('feature/local', remotes, rules).label, 'feature/local')
})

test('custom URL mapping supports a private GitLab host', () => {
  const badge = resolveRefBadge('company/main', remotes, [
    ...defaultRemoteBadgeRules(),
    { id: 'private-gitlab', pattern: 'mymy.gitlab.internal', icon: 'gitlab' },
  ])
  assert.equal(badge.label, 'GitLab/main')
  assert.equal(badge.remote, true)
})

test('legacy emoji settings migrate to embedded icon ids', () => {
  assert.equal(normalizeRemoteBadgeIcon('🐙'), 'github')
  assert.equal(normalizeRemoteBadgeIcon('🦊'), 'gitlab')
  assert.equal(normalizeRemoteBadgeIcon('🔗'), 'remote')
  assert.equal(isEmbeddedRemoteBadgeIcon('🐙'), true)
  assert.deepEqual(remoteBadgeIconOptions.map((option) => option.id), [
    'github', 'gitlab', 'bitbucket', 'azure-devops', 'codeberg',
    'gitea', 'git', 'cloud', 'server', 'remote',
  ])
  assert.ok(remoteBadgeIconOptions.every((option) => isEmbeddedRemoteBadgeIcon(option.id)))
})

test('remote refs are only exposed when the All branches view requests them', () => {
  const refs = ['main', 'origin/main']
  assert.deepEqual(
    visibleRefBadges(refs, remotes, defaultRemoteBadgeRules(), false).map((badge) => badge.label),
    ['main'],
  )
  assert.deepEqual(
    visibleRefBadges(refs, remotes, defaultRemoteBadgeRules(), true).map((badge) => badge.label),
    ['main', 'GitHub/main'],
  )
})

test('default branch is first without changing the source ref order', () => {
  const refs = ['feature/topic', 'main', 'v1.0.0']
  assert.deepEqual(defaultFirstRefs(refs, 'main'), ['main', 'feature/topic', 'v1.0.0'])
  assert.deepEqual(refs, ['feature/topic', 'main', 'v1.0.0'])
  assert.deepEqual(
    visibleRefBadges(refs, remotes, defaultRemoteBadgeRules(), true, 'main').map((badge) => badge.label),
    ['main', 'feature/topic', 'v1.0.0'],
  )
})

test('commit rows keep one default-first ref and summarize the rest', () => {
  const refs = ['feature/topic', 'origin/main', 'main', 'v1.0.0']
  const localSummary = summarizeRefBadges(refs, remotes, defaultRemoteBadgeRules(), false, 'main')
  assert.equal(localSummary.primary?.branch, 'main')
  assert.deepEqual(localSummary.remaining.map((badge) => badge.branch), ['feature/topic', 'v1.0.0'])

  const allSummary = summarizeRefBadges(refs, remotes, defaultRemoteBadgeRules(), true, 'main')
  assert.equal(allSummary.primary?.branch, 'main')
  assert.deepEqual(allSummary.remaining.map((badge) => badge.ref), ['feature/topic', 'origin/main', 'v1.0.0'])
  assert.deepEqual(refs, ['feature/topic', 'origin/main', 'main', 'v1.0.0'])
})

test('inspector falls back to containing branches when a past commit has no exact ref', () => {
  assert.deepEqual(inspectorRefContext([], ['feature/topic', 'main'], 'main'), {
    label: 'Branches',
    values: ['main', 'feature/topic'],
  })
  assert.deepEqual(inspectorRefContext(['v1.0.0'], ['main'], 'main'), {
    label: 'Refs',
    values: ['v1.0.0'],
  })
  assert.deepEqual(inspectorRefContext([], ['main'], 'main', 'aws_s3_buckets'), {
    label: 'Branch',
    values: ['aws_s3_buckets'],
  })
  assert.deepEqual(inspectorRefContext(['release/1.0'], ['main'], 'main', 'feature/topic'), {
    label: 'Refs',
    values: ['release/1.0'],
  })
  assert.deepEqual(inspectorRefContext(undefined, undefined, 'main'), {
    label: 'Refs',
    values: [],
  })
})
