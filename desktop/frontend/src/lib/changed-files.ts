import type { FileChange } from './types'

export type ChangedFileTreeNode = {
  kind: 'directory' | 'file'
  name: string
  path: string
  children: ChangedFileTreeNode[]
  file?: FileChange
}

export type ChangedDiffLine = {
  kind: 'addition' | 'deletion'
  text: string
}

export function buildChangedFileTree(files: FileChange[]): ChangedFileTreeNode[] {
  const root: ChangedFileTreeNode = { kind: 'directory', name: '', path: '', children: [] }

  for (const file of files) {
    const parts = file.path.split('/').filter(Boolean)
    let parent = root
    let currentPath = ''

    for (const [index, part] of parts.entries()) {
      currentPath = currentPath ? `${currentPath}/${part}` : part
      const isFile = index === parts.length - 1
      let node = parent.children.find((candidate) => candidate.name === part && candidate.kind === (isFile ? 'file' : 'directory'))
      if (!node) {
        node = {
          kind: isFile ? 'file' : 'directory',
          name: part,
          path: currentPath,
          children: [],
          file: isFile ? file : undefined,
        }
        parent.children.push(node)
      }
      parent = node
    }
  }

  sortTree(root.children)
  return root.children
}

export function addedAndDeletedLines(diff: string): ChangedDiffLine[] {
  if (!diff) return []
  const lines: ChangedDiffLine[] = []
  for (const text of diff.split('\n')) {
    if (text.startsWith('+') && !text.startsWith('+++')) lines.push({ text, kind: 'addition' })
    if (text.startsWith('-') && !text.startsWith('---')) lines.push({ text, kind: 'deletion' })
  }
  return lines
}

export function changedFileStatusMap(files: FileChange[]): Record<string, string> {
  const statuses: Record<string, string> = {}
  for (const file of files) {
    if (file.path) statuses[file.path] = file.status
    if (file.old_path) statuses[file.old_path] = file.status
  }
  return statuses
}

export function changedDirectoryPaths(files: FileChange[]): Set<string> {
  const directories = new Set<string>()
  for (const file of files) {
    for (const path of [file.path, file.old_path].filter((value): value is string => Boolean(value))) {
      const parts = path.split('/').filter(Boolean)
      let directory = ''
      for (const part of parts.slice(0, -1)) {
        directory = directory ? `${directory}/${part}` : part
        directories.add(directory)
      }
    }
  }
  return directories
}

export function changedFilesSignature(files: FileChange[]): string {
  return files
    .flatMap((file) => [file.path, file.old_path].filter((value): value is string => Boolean(value)).map((path) => `${file.status}:${path}`))
    .sort()
    .join('|')
}

function sortTree(nodes: ChangedFileTreeNode[]): void {
  nodes.sort((left, right) => {
    if (left.kind !== right.kind) return left.kind === 'directory' ? -1 : 1
    return left.name.localeCompare(right.name, undefined, { numeric: true, sensitivity: 'base' })
  })
  for (const node of nodes) sortTree(node.children)
}
