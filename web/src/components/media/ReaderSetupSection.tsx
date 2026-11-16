import { Badge,Group,SimpleGrid,Stack,Text,UnstyledButton } from '@mantine/core'
import { IconAlertTriangle,IconCheck,IconCircleCheck } from '@tabler/icons-react'
import { api,type MediaSettings } from '../../api'
import classes from '../../pages/MediaPage.module.css'
import type { NamingSetup } from '../../types/service'
import { Section } from './Section'

export function ReaderSetupSection({ setups, setup, form, set, preview }: { setups: NamingSetup[]; setup: NamingSetup | undefined; form: MediaSettings; set: (patch: Partial<MediaSettings>) => void; preview: { data: Awaited<ReturnType<typeof api.previewNaming>> | undefined } }) {
  return (
          <Section
            title="Your reader"
            description="Readers decide what a series is in their own way. A setup fills the series folder, the book folder and the names in readers below so yours shows each edition as you mean; nothing changes until you save."
          >
            <SimpleGrid cols={{ base: 1, sm: 2 }} spacing="sm">
              {setups.map((s, i) => {
                const on = setup?.key === s.key
                return (
                  <UnstyledButton
                    key={s.key}
                    className={classes.setup}
                    data-active={on}
                    aria-pressed={on}
                    onClick={() =>
                      set({ seriesFolder: s.seriesFolder, bookFolder: s.bookFolder, readerFields: { ...form.readerFields, ...s.readerFields } })
                    }
                  >
                    <Group justify="space-between" wrap="nowrap" gap="xs">
                      <Text fw={600} size="sm">
                        {s.title}
                      </Text>
                      {on ? (
                        <IconCheck size={16} aria-hidden />
                      ) : (
                        i === 0 && <Badge size="xs" variant="light" color="gray">Recommended</Badge>
                      )}
                    </Group>
                    <Text size="xs" c="dimmed" mt={4} lh={1.5}>
                      {s.summary}
                    </Text>
                  </UnstyledButton>
                )
              })}
            </SimpleGrid>
            {setups.length > 0 && !setup && (
              <Text size="xs" c="dimmed">
                Your own arrangement: it matches neither setup.
              </Text>
            )}
            <Stack gap={8} aria-label="How readers will show it" role="group">
              {(preview.data?.readers ?? []).map((r) => (
                <Group key={r.reader} gap="xs" wrap="nowrap" align="flex-start">
                  {r.fine ? (
                    <IconCircleCheck size={18} color="var(--mantine-color-green-filled)" style={{ flexShrink: 0 }} />
                  ) : (
                    <IconAlertTriangle size={18} color="var(--mantine-color-yellow-filled)" style={{ flexShrink: 0 }} />
                  )}
                  <Text size="sm">
                    <Text span fw={600}>
                      {r.reader}
                    </Text>{' '}
                    {r.says.charAt(0).toLowerCase() + r.says.slice(1)}
                  </Text>
                </Group>
              ))}
            </Stack>
            {(setup?.tips?.length ?? 0) > 0 && (
              <div className={classes.tips}>
                <Text size="xs" fw={600} mb={4}>
                  In your reader
                </Text>
                {[...new Set(setup!.tips.map((t) => t.reader))].map((reader) => (
                  <div key={reader}>
                    <Text size="xs" fw={500} mt={6}>
                      {reader}
                    </Text>
                    <ul className={classes.tipList}>
                      {setup!.tips
                        .filter((t) => t.reader === reader)
                        .map((t) => (
                          <li key={t.tip}>
                            <Text size="xs" c="dimmed">
                              {t.tip}
                            </Text>
                          </li>
                        ))}
                    </ul>
                  </div>
                ))}
              </div>
            )}
          </Section>

  )
}
