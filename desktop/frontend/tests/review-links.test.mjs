import assert from 'node:assert/strict'
import test from 'node:test'

import { buildReviewLink } from '../src/lib/review-links.ts'

const parents = ['first-parent', 'second-parent']

test('GitHub merge messages build a pull request URL from the upstream remote', () => {
  const link = buildReviewLink(
    'Merge pull request #49006 from hashicorp/feature',
    parents,
    [
      { name: 'mirror', url: 'https://example.invalid/hashicorp/terraform-provider-aws.git' },
      { name: 'origin', url: 'git@github.com:hashicorp/terraform-provider-aws.git' },
    ],
    'origin/main',
  )
  assert.deepEqual(link, {
    kind: 'pull-request',
    label: 'PR #49006',
    url: 'https://github.com/hashicorp/terraform-provider-aws/pull/49006',
  })
})

test('GitLab merge messages build a merge request URL for a private host', () => {
  const link = buildReviewLink(
    "Merge branch 'feature' into 'main'\n\nSee merge request platform/GitGit!73",
    parents,
    [{ name: 'origin', url: 'ssh://git@mymy.gitlab.internal/platform/GitGit.git' }],
  )
  assert.deepEqual(link, {
    kind: 'merge-request',
    label: 'MR !73',
    url: 'https://mymy.gitlab.internal/platform/GitGit/-/merge_requests/73',
  })
})

test('explicit review links work while non-merge and ambiguous messages stay unlinked', () => {
  assert.deepEqual(
    buildReviewLink('Merge pull request https://github.com/acme/repo/pull/12.', parents, []),
    { kind: 'pull-request', label: 'PR #12', url: 'https://github.com/acme/repo/pull/12' },
  )
  assert.equal(buildReviewLink('fix: mention #12', ['one-parent'], [{ name: 'origin', url: 'https://github.com/acme/repo.git' }]), null)
  assert.equal(buildReviewLink('Merge branch feature', parents, [{ name: 'origin', url: '/local/repo' }]), null)
})
