import { MantineProvider, type CSSVariablesResolver } from '@mantine/core'
import { useEffect, useState, type ReactNode } from 'react'
import { buildTheme, PRESETS, THEME_STORAGE_KEY, ThemeCtx } from './theme'

const surfaces: CSSVariablesResolver = () => ({
  variables: {},
  light: { '--mantine-color-body': '#ffffff', '--mantine-color-default-border': '#e5e5e0' },
  dark: { '--mantine-color-body': '#181816', '--mantine-color-default-border': '#353532' },
})

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [themeId, setThemeId] = useState<string>(
    () => localStorage.getItem(THEME_STORAGE_KEY) ?? 'minimal',
  )
  const preset = PRESETS.find((p) => p.id === themeId) ?? PRESETS[0]

  useEffect(() => {
    localStorage.setItem(THEME_STORAGE_KEY, preset.id)
  }, [preset])

  return (
    <ThemeCtx.Provider value={{ themeId: preset.id, setThemeId, preset }}>
      <MantineProvider theme={buildTheme(preset)} defaultColorScheme="dark" cssVariablesResolver={surfaces}>
        {children}
      </MantineProvider>
    </ThemeCtx.Provider>
  )
}
