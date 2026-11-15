// Typed client for the Sento API. All payload types are generated from the
// Go structs (mise run gen) into ../types/ — the backend is the schema.

import { seriesApi } from './series'
import { editionsApi } from './editions'
import { filesApi } from './files'
import { acquisitionApi } from './acquisition'
import { settingsApi } from './settings'
import { systemApi, type LookupResult, type SystemStatus } from './system'
import type { Ledger } from './request'

import type { Binding, BlocklistEntry, CalendarEntry, Edition, Decision, File as SeriesFile, QueueItem, Series } from '../types/store'
import type {
  BrowseProwlarrBody,
  EditionPatch,
  IndexerBody,
  IndexerPatch,
  NamingPreview,
  OverridePatch,
  SearchResponse,
  SeriesDetail,
} from '../types/server'
import type {
  ConnectionSettings,
  ConnectionTest,
  ExtensionView,
  FileMappingResult,
  ImportProposal,
  IndexerView,
  MediaSettings,
  ProwlarrIndexer,
  ReleaseCandidate,
  ScanResult,
  SearchEvent,
  SearchScope,
  SeriesIndexerView,
} from '../types/service'
import type { TaskStatus } from '../types/scheduler'
import type { Manga as SourceManga, Preference, Source } from '../types/extensions'
import type { Condition, Format, Profile } from '../types/profile'
import type { ProfileDoc } from '../types/service'

export type {
  Binding,
  BrowseProwlarrBody,
  ConnectionSettings,
  ConnectionTest,
  ExtensionView,
  FileMappingResult,
  IndexerBody,
  IndexerPatch,
  IndexerView,
  Preference,
  ProwlarrIndexer,
  BlocklistEntry,
  CalendarEntry,
  Edition,
  EditionPatch,
  Decision,
  ImportProposal,
  MediaSettings,
  NamingPreview,
  QueueItem,
  ReleaseCandidate,
  ScanResult,
  SearchEvent,
  SearchResponse,
  Series,
  SeriesDetail,
  SeriesFile,
  SeriesIndexerView,
  Source,
  SourceManga,
  SearchScope,
  TaskStatus,
  Condition,
  Format,
  Profile,
  ProfileDoc,
}
export type { SystemStatus, LookupResult }
export type SiteOverride = OverridePatch
export type { Ledger }

export const api = {
  ...seriesApi,
  ...editionsApi,
  ...filesApi,
  ...acquisitionApi,
  ...settingsApi,
  ...systemApi,
}
