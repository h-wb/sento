import type { Dispatch, SetStateAction } from 'react'
import { Button, Card, Group, Menu, Stack, Switch, Text } from '@mantine/core'
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react'
import { api } from '../../api'
import { plural } from '../../format'
import type {
  AssetCoverage,
  CollectedVersion,
  Edition,
  File as SeriesFile,
} from '../../types/store'
import type { VersionSummary } from '../../types/service'
import type { BookSelection } from '../BookDialog'
import { editionSearch } from '../bookSelection'
import type { Confirm } from '../ConfirmAction'
import EditionActions from '../EditionActions'
import EditionBooks from '../EditionBooks'
import { cardLabel, cardTitle, editionDetails } from '../editionText'
import { EditionOrigin } from '../Origin'
import { RefusedFilesNotice } from '../RefusedFiles'
import type { Refused } from '../refusal'
import type { EditionDialogState } from './useEditionDialogs'

type Props = {
  seriesID: number
  edition: Edition
  version: CollectedVersion
  // Every version of this edition, for the card title.
  editionVersions: CollectedVersion[]
  allVersions: CollectedVersion[]
  stats: VersionSummary | undefined
  cardCount: number
  refusal: Refused
  // The series' page on MangaBaka.
  catalogUrl: string
  // Every edition in display order; the move dialog offers the others.
  visible: Edition[]
  // A card's series name in readers, when it is not just the series title.
  readerName: (versionId: number) => string | undefined
  files: SeriesFile[]
  coverage: AssetCoverage[]
  busy: boolean
  expanded: Record<number, boolean>
  setExpanded: Dispatch<SetStateAction<Record<number, boolean>>>
  run: (fn: () => Promise<unknown>) => Promise<void>
  changed: () => Promise<void>
  onBook: (s: BookSelection) => void
  dialogs: EditionDialogState
  confirm: Confirm
}

export function EditionCard({
  seriesID,
  edition: c,
  version: t,
  editionVersions: versions,
  allVersions,
  stats,
  cardCount,
  refusal,
  catalogUrl,
  visible,
  readerName,
  files,
  coverage,
  busy,
  expanded,
  setExpanded,
  run,
  changed,
  onBook,
  dialogs,
  confirm,
}: Props) {
  const owned = stats?.owned ?? 0
  const total = stats?.total ?? 0
  const noun = plural(total, c.type === 'chapter' ? 'chapter' : 'volume')
  // With one or two cards there is nothing to scan past, but a
  // card with nothing on disk and nothing wanted (chapters the
  // volumes already cover) is only a wall of paused numbers.
  const idle = !!stats && stats.onDisk === 0 && stats.wanted === 0 && stats.review === 0
  const show = expanded[t.id] ?? (cardCount <= 2 && !idle)
  const title = cardTitle(c, t, allVersions)
  const missing = stats?.wanted ?? 0
  const findMissing = () => t && onBook(editionSearch(c, t, allVersions))
  return (
    <Card padding="md" withBorder>
      <Stack gap="sm">
        <Group justify="space-between" align="flex-start" wrap="nowrap" gap="sm">
          <div style={{ minWidth: 0 }}>
            <Group gap="xs">
              <Text fw={600} className="wrapAnywhere">
                {title}
              </Text>
              <EditionOrigin edition={c} href={catalogUrl} />
            </Group>
            <Text size="xs" c="dimmed" mt={2}>
              {/* A profile chosen here and not in the title. */}
              {editionDetails(
                c,
                versions.length === 1 && t.profileName,
                readerName(t.id) && `in readers: ${readerName(t.id)}`,
              )}
            </Text>
          </div>
          <Group gap="xs" wrap="nowrap">
            {c.syncState === 'stale' && (
              <Button
                size="compact-xs"
                variant="default"
                onClick={() =>
                  confirm({
                    title: 'Continue as a local edition?',
                    message:
                      'Keep these books and files and manage their metadata yourself. Future catalog refreshes will not update this edition.',
                    label: 'Continue locally',
                    onConfirm: async () => {
                      await api.editEdition(seriesID, c.id, {
                        makeLocal: true,
                        reset: [],
                      })
                      await changed()
                    },
                  })
                }
              >
                Continue locally
              </Button>
            )}
            {t && missing > 0 && (
              <Button variant="default" size="xs" onClick={findMissing}>
                Search {missing} missing
              </Button>
            )}
            <EditionActions
              seriesID={seriesID}
              edition={c}
              title={title}
              others={visible.filter((o) => o.id !== c.id)}
              run={async (fn) => {
                await run(fn)
              }}
              more={
                <>
                  {versions.length > 1 && (
                    <Menu.Item
                      color="red"
                      onClick={() =>
                        confirm({
                          title: `Stop collecting ${title} this way?`,
                          message:
                            'Files stay on disk, and the edition’s other profiles keep theirs.',
                          label: 'Stop collecting',
                          onConfirm: async () => {
                            await api.deleteVersion(seriesID, t.id)
                            await changed()
                          },
                        })
                      }
                    >
                      Stop collecting this way…
                    </Menu.Item>
                  )}
                  {c.custom && (
                    <Menu.Item onClick={() => dialogs.setLink(c)}>Link to MangaBaka</Menu.Item>
                  )}
                </>
              }
            >
              <Menu.Item onClick={findMissing}>Search releases</Menu.Item>
              <Menu.Item onClick={() => dialogs.setEdition(c)}>Edit edition</Menu.Item>
              <Menu.Item onClick={() => dialogs.setVersion(t)}>Download settings</Menu.Item>
              <Menu.Item onClick={() => dialogs.setCollectAgain(c)}>
                Collect again with another profile…
              </Menu.Item>
              <Menu.Item
                onClick={() =>
                  dialogs.setBook({
                    id: 0,
                    editionId: c.id,
                    sequenceNum: 0,
                    sequenceStr: '',
                    numberKnown: true,
                    countType: 'main',
                    pages: 0,
                    syncState: 'local',
                    overrides: [],
                    copies: 0,
                  })
                }
              >
                Add {c.type === 'chapter' ? 'chapter' : 'volume'}
              </Menu.Item>
            </EditionActions>
          </Group>
        </Group>
        <RefusedFilesNotice
          refused={refusal}
          chapters={c.type === 'chapter'}
          onCorrect={() =>
            dialogs.setCorrecting({
              fileIds: refusal.fileIds,
              hint: `${title} doesn’t use these files${refusal.reason ? `: ${refusal.reason.charAt(0).toLowerCase()}${refusal.reason.slice(1)}` : ''}. If that was read wrong, say what they are.`,
            })
          }
          onCollectAgain={() => dialogs.setCollectAgain(c)}
          onSearch={findMissing}
        />
        {
          <Group justify="space-between" gap="sm">
            <Group gap="sm">
              {total > 0 ? (
                <Button
                  variant="subtle"
                  size="compact-sm"
                  px={4}
                  leftSection={
                    show ? <IconChevronDown size={14} /> : <IconChevronRight size={14} />
                  }
                  aria-expanded={show}
                  aria-label={`${show ? 'Hide' : 'Show'} ${noun} (${total})`}
                  onClick={() => setExpanded({ ...expanded, [t.id]: !show })}
                >
                  {total} {noun} · {owned} owned
                  {(stats?.wanted ?? 0) > 0 && ` · ${stats?.wanted} wanted`}
                </Button>
              ) : (
                <Text size="sm" c="dimmed">
                  No {noun} yet
                </Text>
              )}
            </Group>
            <Switch
              size="sm"
              label="Collect"
              checked={t.monitored}
              disabled={busy}
              onChange={(e) =>
                void run(() =>
                  api.saveVersion(seriesID, {
                    ...t,
                    monitored: e.currentTarget.checked,
                  }),
                )
              }
            />
          </Group>
        }
        {show && (
          <EditionBooks
            key={t.id}
            seriesID={seriesID}
            edition={c}
            version={t}
            label={cardLabel(c, t, allVersions)}
            files={files}
            coverage={coverage}
            onBook={onBook}
            onEdit={dialogs.setBook}
            onFile={dialogs.setFile}
            run={run}
            busy={busy}
            explained={refusal.books.length ? refusal.reason : undefined}
          />
        )}
      </Stack>
    </Card>
  )
}
