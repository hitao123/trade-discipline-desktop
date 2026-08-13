import { describe, expect, it } from 'vitest'

import { buildLineGeometry } from './chart'

describe('buildLineGeometry', () => {
  it('maps two hand-checked values into the padded plot area', () => {
    const geometry = buildLineGeometry([
      { date: '2026-08-11', value: 0 },
      { date: '2026-08-12', value: 10 },
    ], 100, 60, 10)

    expect(geometry.points).toEqual([
      { date: '2026-08-11', value: 0, x: 10, y: 50 },
      { date: '2026-08-12', value: 10, x: 90, y: 10 },
    ])
    expect(geometry.path).toBe('M 10 50 L 90 10')
  })

  it('keeps single and equal-value series finite and centered', () => {
    const single = buildLineGeometry([{ date: '2026-08-11', value: 5 }], 100, 60, 10)
    const equal = buildLineGeometry([
      { date: '2026-08-11', value: 5 },
      { date: '2026-08-12', value: 5 },
    ], 100, 60, 10)

    expect(single.points[0]).toMatchObject({ x: 50, y: 30 })
    expect(equal.points.map(point => point.y)).toEqual([30, 30])
    expect([...single.points, ...equal.points].every(point => Number.isFinite(point.x) && Number.isFinite(point.y))).toBe(true)
  })

  it('places the zero axis inside a series that crosses zero', () => {
    const geometry = buildLineGeometry([
      { date: '2026-08-11', value: -10 },
      { date: '2026-08-12', value: 10 },
    ], 100, 60, 10)

    expect(geometry.zeroY).toBe(30)
    expect(geometry.zeroY).toBeGreaterThan(10)
    expect(geometry.zeroY).toBeLessThan(50)
  })

  it('returns an empty geometry for an empty series', () => {
    expect(buildLineGeometry([], 100, 60, 10)).toEqual({ path: '', points: [], min: 0, max: 0 })
  })
})
