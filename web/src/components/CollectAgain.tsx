import { useState } from 'react'
import { Alert, Anchor, Button, Group, Select, Stack, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { Edition, CollectedVersion } from '../types/store'
import { EditorModal } from './EditorModal'
import { useEditor } from './useEditor'

// CollectAgain collects an edition once more under another release
// profile: the same books, wanted as the releases that profile accepts
// (black-and-white and colored, say). The profile is the only question.
export function CollectAgain({
  seriesID,
  edition,
  versions,
  seriesProfileId,
  save,
  close,
}: {
  seriesID: number
  edition: Edition
  // The edition's current ways of collecting it.
  versions: CollectedVersion[]
  // The series' release profile (0: the instance default).
  seriesProfileId: number
  save: () => Promise<void>
  close: () => void
}) {
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.listProfiles })
  const def = useQuery({ queryKey: ['default-profile'], queryFn: api.defaultProfile })
  const [picked, setPicked] = useState<string | null>(null)
  const edit = useEditor()
  const inherited = seriesProfileId || def.data?.profileId || 0
  const used = versions.map((v) => v.profileId || inherited)
  const all = profiles.data ?? []
  const choice = all.find((p) => String(p.id) === picked)
  // Cards whose profiles a release can satisfy together share files; the
  // server says which profiles cannot (profile.Apart).
  const overlapping = choice
    ? used.map((id) => all.find((p) => p.id === id)).filter((p) => p && !(choice.apartFrom ?? []).includes(p.id))
    : []
  const base = versions.find((v) => v.isDefault) ?? versions[0]
  const available = all.filter((p) => !used.includes(p.id))
  return (
    <EditorModal dirty={false} busy={edit.busy} onClose={close} title="Collect again with another profile">
      <Stack>
        <Text size="sm" c="dimmed">
          The same books of {edition.editionName}, collected again for the releases another profile
          accepts: a colored copy beside a black-and-white one, say. A file counts for every card
          whose profile accepts it.
        </Text>
        {profiles.data && available.length === 0 ? (
          <Alert variant="light" p="xs">
            <Text size="sm">
              Every release profile already collects this edition.{' '}
              <Anchor component={Link} to="/profiles" size="sm">
                Make another on Profiles
              </Anchor>{' '}
              (one that requires Colored, say), then come back.
            </Text>
          </Alert>
        ) : (
          <Select
            label="Release profile"
            placeholder="Choose a profile"
            data={available.map((p) => ({ value: String(p.id), label: p.name }))}
            value={picked}
            onChange={setPicked}
          />
        )}
        {overlapping.length > 0 && (
          <Alert variant="light" color="yellow" p="xs">
            <Text size="sm">
              {choice!.name} and {overlapping.map((p) => p!.name).join(', ')} accept some of the same
              releases, and a file both accept counts for both cards. To keep them apart, have one
              refuse a format the other requires.
            </Text>
          </Alert>
        )}
        {edit.error && <Alert color="red">{edit.error}</Alert>}
        <Group justify="flex-end">
          <Button
            disabled={!choice}
            loading={edit.busy}
            onClick={() =>
              void edit.run(async () => {
                await api.saveVersion(seriesID, {
                  ...base,
                  id: 0,
                  name: choice!.name,
                  isDefault: false,
                  monitored: true,
                  profileId: choice!.id,
                })
                await save()
              })
            }
          >
            Collect
          </Button>
        </Group>
      </Stack>
    </EditorModal>
  )
}
