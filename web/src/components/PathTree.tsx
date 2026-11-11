import { Text, UnstyledButton } from '@mantine/core'
import { IconFile, IconFolder } from '@tabler/icons-react'
import type { PathExample } from '../types/service'
import classes from './PathTree.module.css'

// What each level of a path is set by, as the Media page names its fields.
const pathSettings: Record<string, string> = {
  volumesRoot: 'Volumes top folder',
  chaptersRoot: 'Chapters top folder',
  seriesFolder: 'Series folder',
  bookFolder: 'Book folder',
  volumeFile: 'Volume file',
  chapterFile: 'Chapter file',
}

type Node = { text: string; setting: string; examples: string[]; children: Node[] }

// The sample paths as one tree: where they share folders they share rows,
// so the tree shows where copies part ways.
function merge(examples: PathExample[]): Node[] {
  const roots: Node[] = []
  for (const example of examples) {
    let level = roots
    example.parts.forEach((part, i) => {
      const file = i === example.parts.length - 1
      let node = level.find((n) => n.text === part.text && n.setting === part.setting)
      if (!node) {
        node = { text: part.text, setting: part.setting, examples: [], children: [] }
        level.push(node)
      }
      if (file) node.examples.push(example.label)
      level = node.children
    })
  }
  return roots
}

// Where sample books land, level by level, each level naming the setting
// that makes it; picking a level goes to that setting.
export function PathTree({ examples, onPick }: { examples: PathExample[]; onPick: (setting: string) => void }) {
  const rows: { node: Node; depth: number }[] = []
  const walk = (nodes: Node[], depth: number) =>
    nodes.forEach((node) => {
      rows.push({ node, depth })
      walk(node.children, depth + 1)
    })
  walk(merge(examples), 0)
  return (
    <div className={classes.tree} data-testid="path-tree">
      <Text size="xs" c="dimmed" className={classes.root}>
        Library folder
      </Text>
      {rows.map(({ node, depth }, i) => {
        const file = node.children.length === 0
        const setting = pathSettings[node.setting] ?? node.setting
        return (
          <UnstyledButton
            key={i}
            className={classes.row}
            style={{ paddingLeft: 10 + depth * 18 }}
            onClick={() => onPick(node.setting)}
            aria-label={`${node.text}: ${setting}${file ? `, ${node.examples.join(' and ')}` : ''}`}
          >
            {file ? <IconFile size={14} className={classes.icon} /> : <IconFolder size={14} className={classes.icon} />}
            <span className={classes.name}>{node.text}</span>
            <span className={classes.setting}>{file ? `${node.examples.join(' · ')} · ${setting}` : setting}</span>
          </UnstyledButton>
        )
      })}
    </div>
  )
}
