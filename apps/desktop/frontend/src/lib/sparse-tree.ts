import type { RepositoryTreeEntry } from './types'

// The tree is loaded a level at a time because a large repository has far too
// many directories to fetch up front. `children` is null until the level has
// been read, which is what tells the view a node is still unexplored.
export type SparseTreeNode = {
  name: string
  path: string
  checked: boolean
  expanded: boolean
  children: SparseTreeNode[] | null
}

export function toSparseNodes(entries: RepositoryTreeEntry[], checkedPaths: Iterable<string> = []): SparseTreeNode[] {
  const checked = new Set(checkedPaths)
  return entries
    .filter((entry) => entry.object_type === 'tree')
    .map((entry) => ({
      name: entry.name,
      path: entry.path,
      // A directory covered by a checked ancestor is itself included, so it
      // shows as checked even though only the ancestor was selected.
      checked: checked.has(entry.path) || [...checked].some((selected) => entry.path.startsWith(selected + '/')),
      expanded: false,
      children: null,
    }))
}

export function flattenSparseNodes(nodes: SparseTreeNode[], depth = 0): { node: SparseTreeNode; depth: number }[] {
  const visible: { node: SparseTreeNode; depth: number }[] = []
  for (const node of nodes) {
    visible.push({ node, depth })
    if (node.expanded && node.children) visible.push(...flattenSparseNodes(node.children, depth + 1))
  }
  return visible
}

export function findSparseNode(nodes: SparseTreeNode[], path: string): SparseTreeNode | null {
  for (const node of nodes) {
    if (node.path === path) return node
    if (node.children) {
      const found = findSparseNode(node.children, path)
      if (found) return found
    }
  }
  return null
}

function mapSparseNodes(nodes: SparseTreeNode[], transform: (node: SparseTreeNode) => SparseTreeNode): SparseTreeNode[] {
  return nodes.map((node) => {
    const next = transform(node)
    return next.children ? { ...next, children: mapSparseNodes(next.children, transform) } : next
  })
}

export function setSparseNodeChildren(nodes: SparseTreeNode[], path: string, children: SparseTreeNode[]): SparseTreeNode[] {
  return mapSparseNodes(nodes, (node) => {
    if (node.path !== path) return node
    // A level loaded under an already-checked directory inherits its state,
    // because cone mode includes everything beneath a selected directory.
    return { ...node, children: node.checked ? children.map((child) => ({ ...child, checked: true })) : children }
  })
}

export function toggleSparseNode(nodes: SparseTreeNode[], path: string, checked: boolean): SparseTreeNode[] {
  return mapSparseNodes(nodes, (node) => {
    if (node.path === path || node.path.startsWith(path + '/')) return { ...node, checked }
    return node
  })
}

export function toggleSparseExpansion(nodes: SparseTreeNode[], path: string, expanded: boolean): SparseTreeNode[] {
  return mapSparseNodes(nodes, (node) => (node.path === path ? { ...node, expanded } : node))
}

// Only the topmost checked directory is reported. Everything under it is
// implied by cone mode, so listing descendants would just be noise the plan
// has to collapse again.
export function selectedSparsePaths(nodes: SparseTreeNode[]): string[] {
  const selected: string[] = []
  const walk = (level: SparseTreeNode[]) => {
    for (const node of level) {
      if (node.checked) {
        selected.push(node.path)
        continue
      }
      if (node.children) walk(node.children)
    }
  }
  walk(nodes)
  return selected
}
