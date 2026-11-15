import type { CollectedVersion, EditionMetadataPatch, CatalogLinkPreview, Edition } from '../types/store'
import type { Edition as CatalogEdition } from '../types/metadata'
import type {
  EditionOverview,
} from '../types/service'
import type {
  EditionPatch,
  CreateEditionBody,
  MergeEditionResponse,
} from '../types/server'
import { request } from './request'

export const editionsApi = {
  editionOverview: (id: number) => request<EditionOverview>(`/series/${id}/edition-overview`),
  saveVersion: (id: number, version: CollectedVersion) =>
    request<CollectedVersion>(`/series/${id}/versions${version.id ? `/${version.id}` : ''}`, {
      method: version.id ? 'PUT' : 'POST',
      body: JSON.stringify(version),
    }),
  deleteVersion: (id: number, tid: number) =>
    request<void>(`/series/${id}/versions/${tid}`, { method: 'DELETE' }),
  editEdition: (id: number, cid: number, patch: EditionMetadataPatch) =>
    request<void>(`/series/${id}/editions/${cid}/metadata`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  updateEdition: (id: number, cid: number, patch: EditionPatch) =>
    request<void>(`/series/${id}/editions/${cid}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  createEdition: (id: number, body: CreateEditionBody) =>
    request<Edition>(`/series/${id}/editions`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  mergePreview: (id: number, cid: number, into: number) =>
    request<CatalogLinkPreview>(`/series/${id}/editions/${cid}/merge?preview=1`, {
      method: 'POST',
      body: JSON.stringify({ into }),
    }),
  catalogLinkPreview: (id: number, cid: number, catalogId: number) =>
    request<CatalogLinkPreview>(`/series/${id}/editions/${cid}/catalog-link?preview=1`, {
      method: 'POST',
      body: JSON.stringify({ catalogId }),
    }),
  linkCatalog: (id: number, cid: number, catalogId: number) =>
    request<void>(`/series/${id}/editions/${cid}/catalog-link`, {
      method: 'POST',
      body: JSON.stringify({ catalogId }),
    }),
  // "Not the same edition": stops the link from being suggested again.
  declineCatalogLink: (id: number, cid: number, catalogId: number) =>
    request<void>(`/series/${id}/editions/${cid}/catalog-link?decline=1`, {
      method: 'POST',
      body: JSON.stringify({ catalogId }),
    }),
  catalogEditions: () => request<CatalogEdition[]>('/catalog/editions'),
  mergeEditions: (id: number, cid: number, into: number) =>
    request<MergeEditionResponse>(`/series/${id}/editions/${cid}/merge`, {
      method: 'POST',
      body: JSON.stringify({ into }),
    }),
  deleteEdition: (id: number, cid: number) =>
    request<void>(`/series/${id}/editions/${cid}`, { method: 'DELETE' }),
}
