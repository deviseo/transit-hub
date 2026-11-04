// 图表悬停层的公共规则：悬停时不能遮住正在看的那一天的数据。
//   - 整列高亮画在柱子和折线下面（ECharts 默认把 axisPointer 画在 z=50，会盖住数据）；
//   - 坐标轴 tooltip 放在悬停这一列的旁边，而不是鼠标右下角（列较宽时鼠标右下角正好是这一列的柱子和折线点）。

/** 悬停时整列的浅色底，放在系列（默认 z=2）下面。color 一般用主题的边框色。 */
export const columnHighlightPointer = (color: string) => ({
  type: 'shadow' as const,
  z: 1,
  shadowStyle: { color },
})

/** ECharts tooltip position 回调里的尺寸参数。 */
export interface TooltipSize {
  contentSize: number[]
  viewSize: number[]
}

const EDGE = 4
const GAP = 12
// 一列里柱子和折线点的最大半宽（仪表盘每列两根 18px 的柱子）：tooltip 至少离列中心这么远再加 GAP。
const MAX_HALF_COLUMN = 24

/**
 * 坐标轴 tooltip 的位置：放在悬停列的右侧，右侧放不下就放左侧；垂直方向以鼠标为中心并限制在图表内。
 * 图表太窄、两侧都放不下时，水平对准这一列，垂直贴到离鼠标较远的上沿或下沿。
 * columnCenter / bandWidth 是这一列中心的像素位置和列宽，拿不到时按鼠标位置处理。
 */
export function axisTooltipPosition(
  mouse: number[],
  size: TooltipSize,
  columnCenter: number | null,
  bandWidth: number | null,
): [number, number] {
  const [mouseX, mouseY] = mouse
  const [width, height] = size.contentSize
  const [viewWidth, viewHeight] = size.viewSize
  const center = columnCenter ?? mouseX
  const half = Math.min((bandWidth ?? 0) / 2, MAX_HALF_COLUMN)
  const besideY = Math.max(EDGE, Math.min(viewHeight - height - EDGE, mouseY - height / 2))

  const right = center + half + GAP
  if (right + width <= viewWidth - EDGE) return [right, besideY]
  const left = center - half - GAP - width
  if (left >= EDGE) return [left, besideY]

  const x = Math.max(EDGE, Math.min(viewWidth - width - EDGE, center - width / 2))
  const y = mouseY > viewHeight / 2 ? EDGE : Math.max(EDGE, viewHeight - height - EDGE)
  return [x, y]
}
