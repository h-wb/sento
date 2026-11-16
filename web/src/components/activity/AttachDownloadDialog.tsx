import { Button, Modal, Stack, Text, TextInput } from '@mantine/core'
import type { Dispatch, SetStateAction } from 'react'
import { api } from '../../api'
import type { ActivityDownload } from '../../types/service'

export function AttachDownloadDialog({
  attach,
  setAttach,
  remoteID,
  setRemoteID,
  mutation,
}: {
  attach: ActivityDownload | null
  setAttach: Dispatch<SetStateAction<ActivityDownload | null>>
  remoteID: string
  setRemoteID: Dispatch<SetStateAction<string>>
  mutation: { isPending: boolean; mutate: (work: () => Promise<unknown>) => void }
}) {
  return (
    <Modal
      opened={!!attach}
      onClose={() => {
        if (!mutation.isPending) setAttach(null)
      }}
      title="Confirm existing download"
    >
      <Stack>
        <Text size="sm">
          Find this download in your client and enter its existing ID. Sento will check it without
          sending another download.
        </Text>
        <TextInput
          label="Client download ID"
          value={remoteID}
          onChange={(event) => setRemoteID(event.currentTarget.value)}
        />
        <Button
          disabled={!remoteID.trim()}
          loading={mutation.isPending}
          onClick={() =>
            mutation.mutate(async () => {
              await api.retryQueue(attach!.job.id, remoteID.trim())
              setAttach(null)
            })
          }
        >
          Attach download
        </Button>
      </Stack>
    </Modal>
  )
}
