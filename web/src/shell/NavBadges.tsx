import { Badge, Tooltip } from '@mantine/core'
import type { HealthIssue } from '../types/service'

export function ActivityBadge({ active, attention }: { active: number; attention: number }) {
  return (
    <Badge size="xs" circle color={attention > 0 ? 'orange' : 'brand'}>
      {active}
    </Badge>
  )
}

// The count is announced as a description so the link keeps its name.
export function HealthBadge({ issues, summary }: { issues: HealthIssue[]; summary: string }) {
  return (
    <Tooltip label={summary}>
      <Badge
        aria-hidden
        size="xs"
        circle
        color={issues.some((i) => i.level === 'error') ? 'red' : 'yellow'}
      >
        {issues.length}
      </Badge>
    </Tooltip>
  )
}
