import { Badge } from '@mantine/core'

export function ActivityBadge({ active, attention }: { active: number; attention: number }) {
  return (<Badge size="xs" circle color={attention > 0 ? 'orange' : 'brand'}>
            {active}
          </Badge>)
}
