import { MantineProvider } from '@mantine/core'
import { useEffect, useState, type ReactNode } from 'react'
import { buildTheme, PRESETS, THEME_STORAGE_KEY, ThemeCtx } from './theme'

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [themeId, setThemeId] = useState<string>(
    () => localStorage.getItem(THEME_STORAGE_KEY) ?? 'minimal',
  )
  const preset = PRESETS.find((p) => p.id === themeId) ?? PRESETS[0]

  useEffect(() => {
    localStorage.setItem(THEME_STORAGE_KEY, preset.id)
    // Lets index.css gate the background mesh on the active theme.
    document.documentElement.dataset.mesh = preset.effects.mesh ? 'on' : 'off'
  }, [preset])

  return (
    <ThemeCtx.Provider value={{ themeId: preset.id, setThemeId, preset }}>
      <MantineProvider theme={buildTheme(preset)} defaultColorScheme="dark">
        {children}
      </MantineProvider>
    </ThemeCtx.Provider>
  )
}
