import {
ActionIcon,
Group,
Menu,
TextInput,
Tooltip,
UnstyledButton
} from '@mantine/core'
import {
IconArrowsSort,
IconChevronDown,
IconChevronUp,
IconLayoutGrid,
IconLayoutList,
IconSearch
} from '@tabler/icons-react'
import classes from '../../pages/LibraryPage.module.css'
import type { Series } from '../../types/store'


type SortField = 'sortTitle' | 'type' | 'status' | 'year' | 'finalVolume' | 'totalChapters' | 'addedAt'

const SORTS: { value: SortField; label: string }[] = [
  { value: 'sortTitle', label: 'Title' },
  { value: 'addedAt', label: 'Recently added' },
  { value: 'year', label: 'Year' },
  { value: 'status', label: 'Status' },
]

export function LibraryToolbar({ series, scope, setScope, setSelected, filter, setFilter, sortField, sortDir, toggleSort, view, setView }: { series: Series[]; scope: 'all' | 'monitored' | 'unmonitored'; setScope: (scope: 'all' | 'monitored' | 'unmonitored') => void; setSelected: (value: Set<number>) => void; filter: string; setFilter: (filter: string) => void; sortField: SortField; sortDir: 'asc' | 'desc'; toggleSort: (field: SortField) => void; view: 'grid' | 'table'; setView: (view: 'grid' | 'table') => void }) {
  return (
        <div className={classes.toolbar}>
          <div className={classes.scopes} role="group" aria-label="Monitoring filter">
            {(['all', 'monitored', 'unmonitored'] as const).map((value) => {
              const count = value === 'all' ? series.length : series.filter((s) => value === 'monitored' ? s.monitored : !s.monitored).length
              // A filter that would show nothing is not worth a tab.
              if (count === 0 && scope !== value) return null
              return (
                <UnstyledButton key={value} className={classes.scope} data-active={scope === value}
                  aria-pressed={scope === value} onClick={() => { setScope(value); setSelected(new Set()) }}>
                  {value === 'all' ? 'All series' : value === 'monitored' ? 'Monitored' : 'Unmonitored'}
                  <span>{count}</span>
                </UnstyledButton>
              )
            })}
          </div>
          <Group gap="xs" wrap="nowrap" className={classes.tools}>
            <TextInput size="sm" placeholder="Filter by title…" aria-label="Filter by title"
              leftSection={<IconSearch size={15} />} value={filter}
              onChange={(e) => { setFilter(e.currentTarget.value); setSelected(new Set()) }} className={classes.search} />
            <Menu position="bottom-end" withArrow>
              <Menu.Target>
                <Tooltip label="Sort" openDelay={400}>
                  <ActionIcon size="lg" variant="default" aria-label="Sort">
                    <IconArrowsSort size={16} />
                  </ActionIcon>
                </Tooltip>
              </Menu.Target>
              <Menu.Dropdown>
                <Menu.Label>Sort by</Menu.Label>
                {SORTS.map((s) => (
                  <Menu.Item
                    key={s.value}
                    onClick={() => toggleSort(s.value)}
                    rightSection={
                      sortField === s.value ? (
                        sortDir === 'asc' ? (
                          <IconChevronUp size={14} />
                        ) : (
                          <IconChevronDown size={14} />
                        )
                      ) : undefined
                    }
                  >
                    {s.label}
                  </Menu.Item>
                ))}
              </Menu.Dropdown>
            </Menu>

            <ActionIcon.Group>
              <Tooltip label="Poster grid" openDelay={400}>
                <ActionIcon
                  size="lg"
                  variant={view === 'grid' ? 'filled' : 'default'}
                  onClick={() => { setView('grid'); setSelected(new Set()) }}
                  aria-label="Poster grid"
                  aria-pressed={view === 'grid'}
                >
                  <IconLayoutGrid size={16} />
                </ActionIcon>
              </Tooltip>
              <Tooltip label="Table" openDelay={400}>
                <ActionIcon
                  size="lg"
                  variant={view === 'table' ? 'filled' : 'default'}
                  onClick={() => setView('table')}
                  aria-label="Table"
                  aria-pressed={view === 'table'}
                >
                  <IconLayoutList size={16} />
                </ActionIcon>
              </Tooltip>
            </ActionIcon.Group>

          </Group>
        </div>
  )
}
