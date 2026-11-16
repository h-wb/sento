import { Card,Code,Text,Title,UnstyledButton } from '@mantine/core'
import { Fragment } from 'react'
import { api } from '../../api'
import classes from '../../pages/MediaPage.module.css'

export function TokenTable({ schema, insertToken }: { schema: { data: Awaited<ReturnType<typeof api.mediaSchema>> | undefined }; insertToken: (syntax: string) => void }) {
  return (
        <Card padding="md" className={classes.tokens}>
          <Title order={4} mb={4}>
            Naming tokens
          </Title>
          <Text size="xs" c="dimmed" mb="sm">
            Click a token to put it into the template you are editing. <Code>:00</Code> pads a
            number.
          </Text>
          <div className={classes.pairs}>
            {(schema.data?.tokens ?? []).map((t) => (
              <Fragment key={t.syntax}>
                <span style={{ whiteSpace: 'nowrap' }}>
                  <UnstyledButton
                    aria-label={`Insert ${t.syntax}`}
                    // Keep the caret in the template being edited.
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => insertToken(t.syntax)}
                  >
                    <Code>{t.syntax}</Code>
                  </UnstyledButton>
                  {t.perBook && (
                    <Text span size="xs" c="dimmed" aria-hidden>
                      {' *'}
                    </Text>
                  )}
                </span>
                <Text size="xs" c="dimmed">
                  {t.help}
                </Text>
              </Fragment>
            ))}
          </div>
          <Text size="xs" c="dimmed" mt="sm">
            Brackets around an empty token disappear: <Code>[{'{group}'}]</Code> vanishes when
            there is no group.
          </Text>
          <Text size="xs" c="dimmed" mt={4}>
            * names one book, so it cannot be part of a name in readers.
          </Text>
        </Card>
  )
}
