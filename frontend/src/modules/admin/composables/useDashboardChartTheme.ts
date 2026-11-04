import { onBeforeUnmount, onMounted, ref } from 'vue'

export interface DashboardChartTheme {
  foreground: string
  muted: string
  border: string
  card: string
  primary: string
  signal: string
  warning: string
  destructive: string
}

const fallbackTheme: DashboardChartTheme = {
  foreground: '#1a1d23',
  muted: '#697386',
  border: '#e2e4e8',
  card: '#ffffff',
  primary: '#2463d4',
  signal: '#27945f',
  warning: '#e69a05',
  destructive: '#e5484d',
}

// CSS 变量是空格分隔的「H S% L%」。必须转成逗号分隔的 hsl()/hsla()：ECharts（zrender）只认这种写法，
// 悬停时它要按原色算出高亮色，认不出空格写法就得到空颜色，悬停的柱子和整条折线会直接消失。
const cssColor = (styles: CSSStyleDeclaration, variable: string, fallback: string, alpha?: number) => {
  const parts = styles.getPropertyValue(variable).trim().split(/[\s,]+/).filter(Boolean)
  if (parts.length !== 3) return fallback
  return alpha == null ? `hsl(${parts.join(', ')})` : `hsla(${parts.join(', ')}, ${alpha})`
}

// ECharts 绘制到 canvas，无法直接解析 CSS var()。这里在主题切换时读取项目现有
// 颜色变量并转换为实际 hsl 颜色，保证图表和普通组件使用同一套明暗主题。
export function useDashboardChartTheme() {
  const theme = ref<DashboardChartTheme>({ ...fallbackTheme })
  let observer: MutationObserver | null = null

  const refresh = () => {
    const styles = getComputedStyle(document.documentElement)
    theme.value = {
      foreground: cssColor(styles, '--foreground', fallbackTheme.foreground),
      muted: cssColor(styles, '--muted-foreground', fallbackTheme.muted),
      border: cssColor(styles, '--border', fallbackTheme.border, 0.72),
      card: cssColor(styles, '--card', fallbackTheme.card),
      primary: cssColor(styles, '--primary', fallbackTheme.primary),
      signal: cssColor(styles, '--signal', fallbackTheme.signal),
      warning: cssColor(styles, '--warning', fallbackTheme.warning),
      destructive: cssColor(styles, '--destructive', fallbackTheme.destructive),
    }
  }

  onMounted(() => {
    refresh()
    observer = new MutationObserver(refresh)
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
  })

  onBeforeUnmount(() => observer?.disconnect())

  return { theme }
}
