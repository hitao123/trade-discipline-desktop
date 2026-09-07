import { describe, expect, it } from 'vitest'

import { useRemoteData } from './useRemoteData'

describe('useRemoteData', () => {
  it('discards a stale response when a newer refresh finishes first', async () => {
    let resolveFirst!: (value: string) => void
    const first = new Promise<string>((resolve) => { resolveFirst = resolve })
    let calls = 0
    const { data, refresh } = useRemoteData(() => {
      calls += 1
      return calls === 1 ? first : Promise.resolve('second')
    })

    const pendingFirst = refresh()
    const pendingSecond = refresh()
    resolveFirst('first')
    await Promise.all([pendingFirst, pendingSecond])

    expect(data.value).toBe('second')
  })
})
