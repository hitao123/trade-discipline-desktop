export interface ChartDatum {
  date: string
  value: number
}

export interface ChartPoint extends ChartDatum {
  x: number
  y: number
}

export interface LineGeometry {
  path: string
  points: ChartPoint[]
  min: number
  max: number
  zeroY?: number
}

function rounded(value: number): number {
  return Math.round(value * 1000) / 1000
}

export function buildLineGeometry(data: ChartDatum[], width: number, height: number, padding: number): LineGeometry {
  if (data.length === 0) return { path: '', points: [], min: 0, max: 0 }

  const values = data.map(point => point.value)
  let min = Math.min(...values)
  let max = Math.max(...values)
  if (min === max) {
    const spread = Math.max(Math.abs(min) * 0.05, 1)
    min -= spread
    max += spread
  }

  const plotWidth = Math.max(0, width - padding * 2)
  const plotHeight = Math.max(0, height - padding * 2)
  const points = data.map((point, index): ChartPoint => {
    const x = data.length === 1 ? width / 2 : padding + (index / (data.length - 1)) * plotWidth
    const y = padding + ((max - point.value) / (max - min)) * plotHeight
    return { ...point, x: rounded(x), y: rounded(y) }
  })
  const path = points.map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x} ${point.y}`).join(' ')
  const geometry: LineGeometry = { path, points, min, max }
  if (min < 0 && max > 0) {
    geometry.zeroY = rounded(padding + (max / (max - min)) * plotHeight)
  }
  return geometry
}
