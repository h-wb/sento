import { Badge,Tooltip,type BadgeProps } from '@mantine/core'
import { createElement,type ComponentType,type ReactNode } from 'react'
import { api } from '../api'
import { plural } from '../format'

// Badge is polymorphic; a plain component type is enough to build the element by hand.
const BadgeRoot = Badge as ComponentType<BadgeProps & { children?: ReactNode; 'aria-hidden'?: boolean }>

export function useNavBadges(data: Awaited<ReturnType<typeof api.healthChecks>> | undefined) {
  const healthIssues = data?.issues ?? []
  const healthSummary = `${healthIssues.length} health ${plural(healthIssues.length, 'issue')}`
  // The count is announced as a description so the link keeps its name.
  const healthBadge =
    healthIssues.length > 0 ? createElement(Tooltip, { label: healthSummary },
      createElement(BadgeRoot, { 'aria-hidden': true, size: 'xs', circle: true, color: healthIssues.some((i) => i.level === 'error') ? 'red' : 'yellow' }, healthIssues.length)) : undefined

  return { healthIssues, healthSummary, healthBadge }
}
