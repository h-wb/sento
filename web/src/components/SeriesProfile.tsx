import { Button, Menu } from '@mantine/core'
import { IconAdjustmentsHorizontal } from '@tabler/icons-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import { notify } from '../ui'

// ProfileBarButton shows the series' release profile in the action bar
// and switches it from a dropdown. profileId 0 = the instance default.
export function ProfileBarButton({ seriesID, profileId }: { seriesID: number; profileId: number }) {
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.listProfiles })
  const current = profiles.data?.find((p) => p.id === profileId)
  return (
    <Menu position="bottom-start" withinPortal>
      <Menu.Target>
        <Button
          size="sm"
          variant="subtle"
          color="gray"
          leftSection={<IconAdjustmentsHorizontal size={15} />}
          aria-label="Release profile"
        >
          {current?.name ?? 'Profile'}
        </Button>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Label>Release profile</Menu.Label>
        <ProfileMenuItems seriesID={seriesID} profileId={profileId} />
      </Menu.Dropdown>
    </Menu>
  )
}

// ProfileMenuItems picks the series' release profile.
export function ProfileMenuItems({ seriesID, profileId }: { seriesID: number; profileId: number }) {
  const qc = useQueryClient()
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.listProfiles })
  const def = useQuery({ queryKey: ['default-profile'], queryFn: api.defaultProfile })
  const effective = profileId || def.data?.profileId || 0
  const assign = (id: number) =>
    api
      .setSeriesProfile(seriesID, id)
      .then(() => qc.invalidateQueries({ queryKey: ['series', seriesID] }))
      .catch(notify.err)
  return (
    <>
      {(profiles.data ?? []).map((p) => (
        <Menu.Item key={p.id} onClick={() => void assign(p.id)}>
          {p.name}
          {p.id === effective ? ' ✓' : ''}
          {p.id === effective && profileId === 0 ? ' (default)' : ''}
        </Menu.Item>
      ))}
      {profileId !== 0 && <Menu.Item onClick={() => void assign(0)}>Use instance default</Menu.Item>}
    </>
  )
}
