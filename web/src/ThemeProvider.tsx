import { MantineProvider } from '@mantine/core'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { buildTheme, cssVariables, PRESETS, THEME_STORAGE_KEY, ThemeCtx } from './theme'

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [themeId, setThemeId] = useState<string>(
    () => localStorage.getItem(THEME_STORAGE_KEY) ?? 'minimal',
  )
  const preset = PRESETS.find((p) => p.id === themeId) ?? PRESETS[0]
  const theme = useMemo(() => buildTheme(preset), [preset])
  const resolver = useMemo(() => cssVariables(preset), [preset])

  useEffect(() => {
    localStorage.setItem(THEME_STORAGE_KEY, preset.id)
  }, [preset])

  return (
    <ThemeCtx.Provider value={{ themeId: preset.id, setThemeId, preset }}>
      <MantineProvider theme={theme} defaultColorScheme="dark" cssVariablesResolver={resolver}>
        {children}
      </MantineProvider>
    </ThemeCtx.Provider>
  )
}
