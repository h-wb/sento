import { createContext, useContext } from 'react'

// Lets any page open the global "Add series" modal (provided by App).
export const AddSeriesCtx = createContext<() => void>(() => {})
export const useAddSeries = () => useContext(AddSeriesCtx)
