import ActivityPanel from '../components/ActivityPanel'
import { PageHeader } from '../components/PageHeader'
import { usePageTitle } from '../ui'

export default function QueuePage() {
  usePageTitle('Activity')
  return (
    <>
      <PageHeader
        title="Activity"
        description="Downloads, imports, and the decisions that need you."
      />
      <ActivityPanel />
    </>
  )
}
