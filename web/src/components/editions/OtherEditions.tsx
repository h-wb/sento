import {
  Button,
  Card,
  Group,
  Select,
  Stack,
  Text,
} from '@mantine/core'
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react'
import type { Dispatch, SetStateAction } from 'react'
import { api } from '../../api'
import type { CollectedVersion, Edition } from '../../types/store'
import { editionDetails, editionMedia } from '../editionText'

export function OtherEditions({
  others,
  otherEditions,
  setOtherEditions,
  language,
  setLanguage,
  layout,
  setLayout,
  busy,
  run,
  seriesID,
  allVersions,
  onAddEdition,
}: {
  others: Edition[]
  otherEditions: boolean
  setOtherEditions: Dispatch<SetStateAction<boolean>>
  language: string | null
  setLanguage: Dispatch<SetStateAction<string | null>>
  layout: string | null
  setLayout: Dispatch<SetStateAction<string | null>>
  busy: boolean
  run: (fn: () => Promise<unknown>) => Promise<void>
  seriesID: number
  allVersions: CollectedVersion[]
  onAddEdition: () => void
}) {
  return (
    <>
      <Group justify="space-between">
        <Button
          variant="subtle"
          size="compact-sm"
          leftSection={
            otherEditions ? <IconChevronDown size={14} /> : <IconChevronRight size={14} />
          }
          onClick={() => setOtherEditions(!otherEditions)}
          aria-expanded={otherEditions}
        >
          Other editions ({others.length})
        </Button>
        <Button size="xs" variant="default" onClick={() => onAddEdition()}>
          Add edition
        </Button>
      </Group>
      {otherEditions && (
        <Stack gap="sm">
          <Group>
            <Select
              placeholder="All languages"
              clearable
              value={language}
              onChange={setLanguage}
              data={[...new Set(others.map((c) => c.language))]}
            />
            <Select
              placeholder="All media"
              clearable
              value={layout}
              onChange={setLayout}
              data={[...new Set(others.flatMap(editionMedia))]}
            />
          </Group>
          {others
            .filter(
              (c) =>
                (!language || c.language === language) &&
                (!layout || editionMedia(c).includes(layout)),
            )
            .map((c) => (
              <Card key={c.id} withBorder padding="sm">
                <Group justify="space-between">
                  <div>
                    <Text fw={500}>{c.editionName}</Text>
                    <Text size="xs" c="dimmed">
                      {editionDetails(c)}
                    </Text>
                  </div>
                  <Button
                    size="xs"
                    variant="default"
                    disabled={busy}
                    onClick={() => {
                      const t = allVersions.find((t) => t.editionId === c.id && t.isDefault)
                      if (t) void run(() => api.saveVersion(seriesID, { ...t, monitored: true }))
                    }}
                  >
                    Collect edition
                  </Button>
                </Group>
              </Card>
            ))}
        </Stack>
      )}
    </>
  )
}
