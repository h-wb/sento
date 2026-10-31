import { useState } from 'react'
import { Alert, Button, Checkbox, Group, NumberInput, Select, Stack, Switch } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import type { CollectedVersion } from '../types/store'
import { EditorModal } from './EditorModal'
import { useEditor } from './useEditor'

// How one card collects its edition: the release profile, extras,
// undated books, a chapter cutoff, and whether it collects at all.
export function DownloadSettings({
  value,
  chapter,
  save,
  close,
}: {
  value: CollectedVersion
  chapter: boolean
  save: (v: CollectedVersion) => Promise<void>
  close: () => void
}) {
  const [v, set] = useState(value)
  const q = useQuery({ queryKey: ['profiles'], queryFn: api.listProfiles })
  const edit = useEditor()
  return (
    <EditorModal
      dirty={JSON.stringify(v) !== JSON.stringify(value)}
      busy={edit.busy}
      onClose={close}
      title="Download settings"
    >
      <form
        onSubmit={(e) => {
          e.preventDefault()
          void edit.run(() => save(v))
        }}
      >
        <Stack>
          <fieldset className="formFields" disabled={edit.busy}>
            <Stack>
              <Select
                label="Release profile"
                placeholder="The series' profile"
                clearable
                data={
                  q.data?.map((p) => ({
                    value: String(p.id),
                    label: p.name,
                  })) ?? []
                }
                value={v.profileId ? String(v.profileId) : null}
                onChange={(s) => set({ ...v, profileId: Number(s) || 0 })}
              />
              {chapter && (
                <NumberInput
                  label="Skip through chapter"
                  description="Skip this chapter and earlier ones. Use 0 to skip through chapter 0."
                  value={v.cutoffNum / 100}
                  min={0}
                  decimalScale={2}
                  onChange={(n) => set({ ...v, cutoffNum: Math.round(Number(n) * 100) })}
                />
              )}
              <Checkbox
                label="Include extras and specials"
                description="Collect bonus and other books too. Individual choices override this setting."
                checked={v.includeExtras}
                onChange={(e) => set({ ...v, includeExtras: e.currentTarget.checked })}
              />
              <Checkbox
                label="Search books with unknown publication dates"
                checked={v.acquireUnknown}
                onChange={(e) => set({ ...v, acquireUnknown: e.currentTarget.checked })}
              />
              <Switch
                label="Collect"
                checked={v.monitored}
                onChange={(e) => set({ ...v, monitored: e.currentTarget.checked })}
              />
            </Stack>
          </fieldset>
          {edit.error && <Alert color="red">{edit.error}</Alert>}
          <Group justify="flex-end">
            <Button type="submit" loading={edit.busy}>
              Save
            </Button>
          </Group>
        </Stack>
      </form>
    </EditorModal>
  )
}
