import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api'
import type { ExtensionRepos } from '../../types/server'
import { invalidateExtensions } from '../../queries'
import { notify } from '../../ui'

// Adds one repo URL to the sidecar's list. The cache update covers both
// pages that add repos: the repos query is set from the response (what the
// Connections page shows) and the extension-side keys are invalidated (what
// the Indexers page refreshes).
export function useAddExtensionRepo(handlers?: {
  onAdded?: (data: ExtensionRepos, url: string) => void
  onFailed?: (url: string) => void
}) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (url: string) => {
      const current = await api.extensionRepos()
      return api.setExtensionRepos([...current.urls, url])
    },
    onSuccess: (data, url) => {
      queryClient.setQueryData(['extension-repos'], data)
      invalidateExtensions(queryClient)
      handlers?.onAdded?.(data, url)
    },
    onError: (e, url) => {
      notify.err(e)
      handlers?.onFailed?.(url)
    },
  })
}
