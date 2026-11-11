// Accent presets share the same quiet surfaces and typography.
// (Provider component lives in ThemeProvider.tsx.)

import { createContext, useContext } from 'react'
import classes from './theme.module.css'
import {
  createTheme,
  defaultVariantColorsResolver,
  type CSSVariablesResolver,
  type MantineColorsTuple,
  type VariantColorsResolver,
} from '@mantine/core'

export interface Preset {
  id: string
  label: string
  swatch: string // representative color shown in the picker
  brand: MantineColorsTuple // 10 shades, light → dark (index 6 = primary)
  // A neutral scale's dark-scheme fill is nearly the page color, so a
  // checked switch or a primary button would vanish. Such a preset fills
  // with a light shade and dark text on dark surfaces instead.
  darkFill?: { filled: string; hover: string; text: string; tint: string; tintHover: string }
  radius: 'xs' | 'sm' | 'md' | 'lg'
}

export const PRESETS: Preset[] = [
  {
    id: 'minimal',
    label: 'Minimal',
    swatch: '#52525b',
    // Neutral zinc scale. Covers provide
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
    darkFill: {
      filled: '#e4e4e7',
      hover: '#d4d4d8',
      text: '#18181b',
      tint: 'rgba(228, 228, 231, 0.08)',
      tintHover: 'rgba(228, 228, 231, 0.14)',
    },
    radius: 'sm',
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
    radius: 'sm',
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
    radius: 'sm',
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
    radius: 'sm',
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
    radius: 'sm',
  },
]

export const THEME_STORAGE_KEY = 'sento-theme'

// Text on a primary fill follows the scheme's contrast color, which a
// darkFill preset turns dark in the dark scheme.
const variantColorResolver: VariantColorsResolver = (input) => {
  const colors = defaultVariantColorsResolver(input)
  const primary = !input.color || input.color === 'brand'
  return input.variant === 'filled' && primary
    ? { ...colors, color: 'var(--mantine-primary-color-contrast)' }
    : colors
}

export const cssVariables =
  (p: Preset): CSSVariablesResolver =>
  () => ({
    variables: {},
    light: { '--mantine-color-body': '#ffffff', '--mantine-color-default-border': '#e5e5e0' },
    dark: {
      '--mantine-color-body': '#181816',
      '--mantine-color-default-border': '#353532',
      ...(p.darkFill && {
        '--mantine-color-brand-filled': p.darkFill.filled,
        '--mantine-color-brand-filled-hover': p.darkFill.hover,
        '--mantine-primary-color-contrast': p.darkFill.text,
        '--mantine-color-brand-light': p.darkFill.tint,
        '--mantine-color-brand-light-hover': p.darkFill.tintHover,
        '--mantine-color-brand-light-color': p.darkFill.filled,
      }),
    },
  })

export function buildTheme(p: Preset) {
  return createTheme({
    primaryColor: 'brand',
    variantColorResolver,
    defaultRadius: p.radius,
    fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
    colors: {
      brand: p.brand,
      dark: ['#e8e8e6', '#b6b6b2', '#92928e', '#666662', '#40403e', '#30302e', '#242422', '#1d1d1b', '#181816', '#121210'],
    },
    headings: {
      fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, sans-serif',
      fontWeight: '600',
    },
    shadows: {
      sm: '0 1px 2px rgba(0,0,0,.2), 0 2px 8px rgba(0,0,0,.18)',
      md: '0 1px 3px rgba(0,0,0,.3), 0 8px 24px rgba(0,0,0,.2)',
    },
    cursorType: 'pointer',
    components: {
      Button: { defaultProps: { fw: 500 } },
      Table: { classNames: { th: classes.tableHeader, td: classes.tableCell } },
      Input: { classNames: { input: classes.input } },
      Modal: { classNames: { title: classes.modalTitle }, defaultProps: { closeButtonProps: { 'aria-label': 'Close dialog' }, overlayProps: { backgroundOpacity: 0.35, blur: 2 } } },
      Tabs: { classNames: { list: classes.tabsList, tab: classes.tab } },
      Badge: { classNames: { label: classes.badgeLabel }, defaultProps: { radius: 'sm', variant: 'light', fw: 500 } },
      Checkbox: {
        vars: (_theme: unknown, props: { color?: string }) =>
          props.color ? { root: {} } : { root: { '--checkbox-icon-color': 'var(--mantine-primary-color-contrast)' } },
      },
      Switch: { classNames: { thumb: classes.switchThumb } },
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
