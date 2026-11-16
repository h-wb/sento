import {
Group,
Image,
SimpleGrid,
Text,
Tooltip,
UnstyledButton
} from '@mantine/core'
import {
IconBooks,
IconEyeOff
} from '@tabler/icons-react'
import { Link } from 'react-router-dom'
import classes from '../../pages/LibraryPage.module.css'
import type { Series } from '../../types/store'


export function PosterGrid({ shown, owned }: { shown: Series[]; owned: (id: number) => { complete: boolean; share: number; label: string; title: string } | null }) {
  return (
        <SimpleGrid className={classes.posterGrid} spacing="xl" verticalSpacing={30}>
          {shown.map((s) => (
            <UnstyledButton
              key={s.id}
              className={classes.poster}
              component={Link}
              to={`/series/${s.id}`}
              data-monitored={s.monitored}
            >
              <div className={classes.cover}>
                {s.coverUrl ? <Image src={s.coverUrl} alt={s.title} className={classes.coverImage}
                  fallbackSrc="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='240' height='360'%3E%3Crect width='240' height='360' fill='%2352524e'/%3E%3C/svg%3E" /> :
                  <div className={classes.coverFallback}><IconBooks size={32} stroke={1} /><Text size="sm" ta="center">{s.title}</Text></div>}
                {!s.monitored && <Tooltip label="Unmonitored"><span className={classes.monitorFlag}><IconEyeOff size={14} /></span></Tooltip>}
                {owned(s.id) && (
                  <span className={classes.progress} data-complete={owned(s.id)!.complete}>
                    <span style={{ width: `${owned(s.id)!.share}%` }} />
                  </span>
                )}
              </div>
              <Text className={classes.posterTitle} fw={500} size="sm" mt={11} lineClamp={2}>{s.title}</Text>
              <Group gap={6} mt={4} className={classes.metadata}>
                {s.status && <><span className={classes.releaseDot} data-ongoing={s.status === 'ongoing'} /><Text size="xs" tt="capitalize">{s.status.replaceAll('_', ' ')}</Text></>}
                {owned(s.id) ? (
                  <Text size="xs" title={owned(s.id)!.title}>· {owned(s.id)!.label}</Text>
                ) : (
                  s.finalVolume > 0 && <Text size="xs">· {s.finalVolume} vol</Text>
                )}
                {!s.status && (s.year ?? 0) > 0 && <Text size="xs">{s.year}</Text>}
              </Group>
            </UnstyledButton>
          ))}
        </SimpleGrid>
  )
}
