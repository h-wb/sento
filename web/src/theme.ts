// Theme system, qui-style: a registry of curated presets the user can switch
// between. Each preset is just a brand color scale + a few flags; it composes
// with Mantine's built-in light/dark color scheme. The choice is persisted.
// (Provider component lives in ThemeProvider.tsx.)

import { createContext, useContext } from 'react'
import { createTheme, type MantineColorsTuple } from '@mantine/core'

export interface Preset {
  id: string
  label: string
  swatch: string // representative color shown in the picker
  brand: MantineColorsTuple // 10 shades, light → dark (index 6 = primary)
  accentTo: string // second gradient stop (built-in Mantine color name)
  radius: 'xs' | 'sm' | 'md' | 'lg'
  display: boolean // use the Space Grotesk display font for headings
  effects: { mesh: boolean; gradient: boolean }
}

export const PRESETS: Preset[] = [
  {
    id: 'minimal',
    label: 'Minimal',
    swatch: '#52525b',
    // Neutral zinc scale — the qui-style monochrome default. Covers provide
    // the only real color.
    brand: [
      '#fafafa',
      '#f4f4f5',
      '#e4e4e7',
      '#d4d4d8',
      '#a1a1aa',
      '#71717a',
      '#52525b',
      '#3f3f46',
      '#27272a',
      '#18181b',
    ],
    accentTo: 'gray',
    radius: 'sm',
    display: false,
    effects: { mesh: false, gradient: false },
  },
  {
    id: 'aurora',
    label: 'Aurora',
    swatch: '#6c40ca',
    brand: [
      '#f2eefe',
      '#e1d9f5',
      '#c0aee8',
      '#9d80db',
      '#7f59d0',
      '#6c40ca',
      '#6233c8',
      '#5126b1',
      '#48219e',
      '#3d188b',
    ],
    accentTo: 'cyan',
    radius: 'md',
    display: true,
    effects: { mesh: true, gradient: true },
  },
  {
    id: 'nightwalker',
    label: 'Nightwalker',
    swatch: '#3b82f6',
    brand: [
      '#eff6ff',
      '#dbeafe',
      '#bfdbfe',
      '#93c5fd',
      '#60a5fa',
      '#3b82f6',
      '#2563eb',
      '#1d4ed8',
      '#1e40af',
      '#1e3a8a',
    ],
    accentTo: 'cyan',
    radius: 'md',
    display: true,
    effects: { mesh: true, gradient: true },
  },
  {
    id: 'emerald',
    label: 'Emerald',
    swatch: '#10b981',
    brand: [
      '#ecfdf5',
      '#d1fae5',
      '#a7f3d0',
      '#6ee7b7',
      '#34d399',
      '#10b981',
      '#059669',
      '#047857',
      '#065f46',
      '#064e3b',
    ],
    accentTo: 'teal',
    radius: 'md',
    display: true,
    effects: { mesh: true, gradient: false },
  },
  {
    id: 'sunset',
    label: 'Sunset',
    swatch: '#f97316',
    brand: [
      '#fff7ed',
      '#ffedd5',
      '#fed7aa',
      '#fdba74',
      '#fb923c',
      '#f97316',
      '#ea580c',
      '#c2410c',
      '#9a3412',
      '#7c2d12',
    ],
    accentTo: 'pink',
    radius: 'md',
    display: true,
    effects: { mesh: true, gradient: true },
  },
]

export const THEME_STORAGE_KEY = 'sento-theme'

export function buildTheme(p: Preset) {
  return createTheme({
    primaryColor: 'brand',
    colors: { brand: p.brand },
    defaultRadius: p.radius,
    fontFamily: 'Inter, system-ui, -apple-system, sans-serif',
    headings: {
      fontFamily: p.display ? '"Space Grotesk", Inter, sans-serif' : 'Inter, sans-serif',
      fontWeight: '600',
    },
    shadows: {
      sm: '0 1px 2px rgba(0,0,0,.2), 0 2px 8px rgba(0,0,0,.18)',
      md: '0 1px 3px rgba(0,0,0,.3), 0 8px 24px rgba(0,0,0,.2)',
    },
    cursorType: 'pointer',
    components: {
      // Flat surfaces, qui-style: defined by a hairline border, not a shadow.
      Card: { defaultProps: { withBorder: true, shadow: undefined } },
      Paper: { defaultProps: { withBorder: true, shadow: undefined } },
    },
  })
}

export interface ThemeCtxValue {
  themeId: string
  setThemeId: (id: string) => void
  preset: Preset
}
export const ThemeCtx = createContext<ThemeCtxValue | null>(null)

export function useThemePreset(): ThemeCtxValue {
  const c = useContext(ThemeCtx)
  if (!c) throw new Error('useThemePreset must be used within ThemeProvider')
  return c
}
