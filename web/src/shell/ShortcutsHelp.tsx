import { Group, Kbd, Modal, Table, Text } from '@mantine/core'
import { SHORTCUTS } from './navigation'

export function ShortcutsHelp({
  helpOpen,
  closeHelp,
}: {
  helpOpen: boolean
  closeHelp: () => void
}) {
  return (
    <Modal opened={helpOpen} onClose={closeHelp} title="Keyboard shortcuts" centered>
      <Table verticalSpacing="xs">
        <Table.Tbody>
          {SHORTCUTS.map(([keys, desc]) => (
            <Table.Tr key={desc}>
              <Table.Td w={140}>
                <Group gap={6}>
                  {keys.map((k) => (
                    <Kbd key={k} size="sm">
                      {k}
                    </Kbd>
                  ))}
                </Group>
              </Table.Td>
              <Table.Td>
                <Text size="sm">{desc}</Text>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
    </Modal>
  )
}
