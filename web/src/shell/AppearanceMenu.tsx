import { ActionIcon,ColorSwatch,Menu,useMantineColorScheme } from '@mantine/core'
import { IconCheck,IconDeviceDesktop,IconMoon,IconPalette,IconSun } from '@tabler/icons-react'
import { PRESETS,useThemePreset } from '../theme'

const MODES = [
  { value: 'light', label: 'Light', icon: IconSun },
  { value: 'dark', label: 'Dark', icon: IconMoon },
  { value: 'auto', label: 'System', icon: IconDeviceDesktop },
] as const

export function AppearanceMenu() {
  const { colorScheme, setColorScheme } = useMantineColorScheme()
  const { themeId, setThemeId } = useThemePreset()
  return (
            <Menu position="bottom-end" withArrow width={220} closeOnItemClick={false}>
              <Menu.Target>
                <ActionIcon variant="default" size="lg" radius="md" aria-label="Appearance">
                  <IconPalette size={18} />
                </ActionIcon>
              </Menu.Target>
              <Menu.Dropdown>
                <Menu.Label>Mode</Menu.Label>
                {MODES.map((m) => (
                  <Menu.Item
                    key={m.value}
                    leftSection={<m.icon size={16} />}
                    rightSection={
                      colorScheme === m.value ? <IconCheck size={14} /> : undefined
                    }
                    onClick={() => setColorScheme(m.value)}
                  >
                    {m.label}
                  </Menu.Item>
                ))}
                <Menu.Divider />
                <Menu.Label>Theme</Menu.Label>
                {PRESETS.map((p) => (
                  <Menu.Item
                    key={p.id}
                    leftSection={<ColorSwatch color={p.swatch} size={14} />}
                    rightSection={
                      p.id === themeId ? <IconCheck size={14} /> : undefined
                    }
                    onClick={() => setThemeId(p.id)}
                  >
                    {p.label}
                  </Menu.Item>
                ))}
              </Menu.Dropdown>
            </Menu>
  )
}
