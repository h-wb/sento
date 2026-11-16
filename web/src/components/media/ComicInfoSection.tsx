import { Button,Code,Collapse,Group,Switch,Text } from '@mantine/core'
import { Fragment,type ReactNode } from 'react'
import { api,type MediaSettings } from '../../api'
import classes from '../../pages/MediaPage.module.css'
import { Section } from './Section'

export function ComicInfoSection({ form, set, readerFields, readerInput, allFields, setAllFields, schema }: { form: MediaSettings; set: (patch: Partial<MediaSettings>) => void; readerFields: Awaited<ReturnType<typeof api.mediaSchema>>['readerFields']; readerInput: (field: Awaited<ReturnType<typeof api.mediaSchema>>['readerFields'][number]) => ReactNode; allFields: boolean; setAllFields: React.Dispatch<React.SetStateAction<boolean>>; schema: { data: Awaited<ReturnType<typeof api.mediaSchema>> | undefined } }) {
  return (
          <Section
            title="Inside each book"
            description="The ComicInfo.xml Sento writes into every book. Readers file, name and describe books by it; the two names below are yours to choose, the rest comes from MangaBaka and the release."
          >
            <Switch
              label="Embed metadata in books"
              description="Every import becomes a clean CBZ with a full ComicInfo.xml. Torrent downloads are copied so the original keeps seeding, which uses extra disk space while it seeds."
              checked={form.writeComicInfo}
              onChange={(e) => set({ writeComicInfo: e.currentTarget.checked })}
            />
            {!form.writeComicInfo && (
              <Switch
                label="Copy instead of hardlink"
                description="Books keep whatever metadata they came with. Use a full copy if your filesystem does not support hardlinks."
                checked={form.alwaysCopy}
                onChange={(e) => set({ alwaysCopy: e.currentTarget.checked })}
              />
            )}
            {readerFields.map(readerInput)}
            <div>
              <Button
                variant="subtle"
                size="compact-sm"
                px={0}
                aria-expanded={allFields}
                onClick={() => setAllFields((v) => !v)}
              >
                {allFields ? 'Hide' : 'Show'} every field Sento writes ({schema.data?.comicInfo.length ?? 0})
              </Button>
              <Collapse expanded={allFields}>
                <div className={classes.pairs} style={{ marginTop: 8 }}>
                  {(schema.data?.comicInfo ?? []).map((w) => (
                    <Fragment key={w.fields.join()}>
                      <Group gap={4}>
                        {w.fields.map((f) => (
                          <Code key={f}>{f}</Code>
                        ))}
                      </Group>
                      <Text size="xs" c="dimmed">
                        {w.from}
                      </Text>
                    </Fragment>
                  ))}
                </div>
              </Collapse>
            </div>
          </Section>

  )
}
