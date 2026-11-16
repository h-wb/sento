import type {
  ConnectionSettings,
  ConnectionTest,
  ExtensionView,
  MediaSettings,
  IndexerView,
  IndexerSearchCheck,
  ProwlarrIndexer,
} from '../types/service'
import type { ProfileDoc } from '../types/service'
import type {
  DefaultProfileResponse,
  ExtensionRepos,
  IndexerBody,
  IndexerPatch,
  BrowseProwlarrBody,
  NamingPreview,
} from '../types/server'
import type { Preference, Source } from '../types/extensions'
import type { Format, Profile } from '../types/profile'
import type { Manga as SourceManga } from '../types/extensions'
import type { MediaSchema } from '../types/service'
import { request } from './request'

export const settingsApi = {
  listProfiles: () => request<Profile[]>('/profiles/'),
  createProfile: (p: Omit<Profile, 'id'>) =>
    request<Profile>('/profiles/', { method: 'POST', body: JSON.stringify(p) }),
  updateProfile: (id: number, p: Omit<Profile, 'id'>) =>
    request<void>(`/profiles/${id}/`, {
      method: 'PUT',
      body: JSON.stringify(p),
    }),
  deleteProfile: (id: number) => request<void>(`/profiles/${id}/`, { method: 'DELETE' }),
  exportProfile: (id: number) => request<ProfileDoc>(`/profiles/${id}/export`),
  importProfile: (doc: ProfileDoc) =>
    request<Profile>('/profiles/import', {
      method: 'POST',
      body: JSON.stringify(doc),
    }),
  defaultProfile: () => request<DefaultProfileResponse>('/profiles/default'),
  setDefaultProfile: (profileId: number) =>
    request<void>('/profiles/default', {
      method: 'PUT',
      body: JSON.stringify({ profileId }),
    }),
  listFormats: () => request<Format[]>('/formats/'),
  createFormat: (f: Omit<Format, 'id'>) =>
    request<Format>('/formats/', { method: 'POST', body: JSON.stringify(f) }),
  updateFormat: (id: number, f: Omit<Format, 'id'>) =>
    request<void>(`/formats/${id}`, { method: 'PUT', body: JSON.stringify(f) }),
  deleteFormat: (id: number) => request<void>(`/formats/${id}`, { method: 'DELETE' }),

  connections: () => request<ConnectionSettings>('/settings/connections'),
  saveConnections: (c: ConnectionSettings) =>
    request<ConnectionSettings>('/settings/connections', {
      method: 'PUT',
      body: JSON.stringify(c),
    }),
  testConnections: (c: ConnectionSettings) =>
    request<ConnectionTest[]>('/settings/connections/test', {
      method: 'POST',
      body: JSON.stringify(c),
    }),

  extensions: () => request<ExtensionView[]>('/extensions'),
  extensionRepos: () => request<ExtensionRepos>('/extensions/repos'),
  setExtensionRepos: (urls: string[]) =>
    request<ExtensionRepos>('/extensions/repos', {
      method: 'PUT',
      body: JSON.stringify({ urls }),
    }),
  // Preferences are addressed by host source id (settings.sourceId of a
  // source entry), not entry id: the indexer modal edits them before the
  // entry exists.
  sourcePreferences: (sourceId: string) =>
    request<Preference[]>(`/extensions/sources/${encodeURIComponent(sourceId)}/preferences`),
  setSourcePreference: (sourceId: string, key: string, value: unknown) =>
    request<void>(`/extensions/sources/${encodeURIComponent(sourceId)}/preferences`, {
      method: 'PUT',
      body: JSON.stringify({ key, value }),
    }),
  installExtension: (pkg: string) =>
    request<void>(`/extensions/${encodeURIComponent(pkg)}`, { method: 'POST' }),
  extensionSources: (pkg: string) =>
    request<Source[]>(`/extensions/${encodeURIComponent(pkg)}/sources`),

  mediaSettings: () => request<MediaSettings>('/settings/media'),
  saveMediaSettings: (m: MediaSettings) =>
    request<MediaSettings>('/settings/media', {
      method: 'PUT',
      body: JSON.stringify(m),
    }),
  // The naming tokens and reader fields, from the renderer's own lists.
  mediaSchema: () => request<MediaSchema>('/settings/media/schema'),
  previewNaming: (m: MediaSettings) =>
    request<NamingPreview>('/settings/media/preview', {
      method: 'POST',
      body: JSON.stringify(m),
    }),

  indexers: () => request<IndexerView[]>('/indexers'),
  addIndexer: (body: IndexerBody) =>
    request<IndexerView>('/indexers', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  updateIndexer: (iid: number, body: IndexerBody) =>
    request<IndexerView>(`/indexers/${iid}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),
  patchIndexer: (iid: number, patch: IndexerPatch) =>
    request<IndexerView>(`/indexers/${iid}`, {
      method: 'PATCH',
      body: JSON.stringify(patch),
    }),
  deleteIndexer: (iid: number) => request<void>(`/indexers/${iid}`, { method: 'DELETE' }),
  testIndexer: (iid: number) => request<void>(`/indexers/${iid}/test`, { method: 'POST' }),
  testIndexerSearch: (iid: number, query: string) =>
    request<IndexerSearchCheck>(`/indexers/${iid}/search-test`, {
      method: 'POST',
      body: JSON.stringify({ query }),
    }),
  browseProwlarr: (body: BrowseProwlarrBody) =>
    request<ProwlarrIndexer[]>('/indexers/browse', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  sourceSearch: (sid: string, q: string) =>
    request<SourceManga[]>(`/sources/${sid}/search?q=${encodeURIComponent(q)}`),
}
