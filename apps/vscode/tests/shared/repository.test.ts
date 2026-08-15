import assert from 'node:assert/strict'
import * as path from 'node:path'
import test from 'node:test'

import {
  isCommitObjectID,
  isSafeRepositoryRelativePath,
  pathIsWithin,
  repositoryRelativePath,
  revisionRepositoryTarget,
} from '../../src/shared/repository.js'

test('selection identifiers stay inside the discovered repository', () => {
  assert.equal(isCommitObjectID('a'.repeat(40)), true)
  assert.equal(isCommitObjectID('b'.repeat(64)), true)
  assert.equal(isCommitObjectID('a'.repeat(12)), false)
  assert.equal(isCommitObjectID('A'.repeat(40)), false)
  assert.equal(isCommitObjectID('HEAD'), false)
  assert.equal(isSafeRepositoryRelativePath('src/main.go'), true)
  assert.equal(isSafeRepositoryRelativePath('../outside.go'), false)
  assert.equal(isSafeRepositoryRelativePath('.git/config'), false)
  assert.equal(isSafeRepositoryRelativePath('.GIT/config'), false)
  assert.equal(isSafeRepositoryRelativePath('src\\main.go'), false)
  assert.equal(isSafeRepositoryRelativePath('src/line\nbreak.go'), false)
})

test('repositoryRelativePath returns portable paths and rejects escapes', () => {
  const root = path.resolve(path.join('fixture', 'repository'))
  assert.equal(repositoryRelativePath(root, path.join(root, 'src', 'main.go')), 'src/main.go')
  assert.equal(repositoryRelativePath(root, root), undefined)
  assert.equal(repositoryRelativePath(root, path.resolve(root, '..', 'outside.go')), undefined)
})

test('pathIsWithin accepts the root and descendants but rejects siblings and prefixes', () => {
  assert.equal(pathIsWithin('/workspace/project', '/workspace/project'), true)
  assert.equal(pathIsWithin('/workspace/project', '/workspace/project/nested/repo'), true)
  assert.equal(pathIsWithin('/workspace/project', '/workspace/project-other'), false)
  assert.equal(pathIsWithin('/workspace/project', '/workspace/other'), false)
})

test('revisionRepositoryTarget safely covers workspace parents and repository subdirectories', () => {
  assert.deepEqual(
    revisionRepositoryTarget('/workspace/repositories/project', undefined, ['/workspace']),
    { candidates: ['/workspace/repositories/project'], expectedRoot: '/workspace/repositories/project' },
  )
  assert.deepEqual(
    revisionRepositoryTarget('/workspace/project', undefined, [
      '/unrelated/repository',
      '/workspace/project/packages/app',
      '/workspace/project/packages/other',
    ]),
    {
      candidates: ['/workspace/project/packages/app', '/workspace/project/packages/other'],
      expectedRoot: '/workspace/project',
    },
  )
  assert.deepEqual(
    revisionRepositoryTarget('/workspace/project', '/workspace/project', ['/workspace/other']),
    { candidates: ['/workspace/project'], expectedRoot: '/workspace/project' },
  )
  assert.equal(
    revisionRepositoryTarget('/outside/project', '/workspace/other', ['/workspace/project']),
    undefined,
  )
})
