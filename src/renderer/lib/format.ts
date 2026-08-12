export function formatCNY(fen: number): string {
  return new Intl.NumberFormat('zh-CN', { style: 'currency', currency: 'CNY', minimumFractionDigits: 2 }).format(fen / 100)
}

export function formatPrice(minor: number, currency = 'CNY'): string {
  return new Intl.NumberFormat('zh-CN', { style: 'currency', currency, minimumFractionDigits: 2 }).format(minor / 100)
}

export function formatPercentBP(bp: number): string {
  return `${bp >= 0 ? '+' : ''}${(bp / 100).toFixed(2)}%`
}

export function formatTurnover(fen: number): string {
  const yuan = fen / 100
  if (yuan >= 100_000_000) return `${(yuan / 100_000_000).toFixed(2)} 亿`
  if (yuan >= 10_000) return `${(yuan / 10_000).toFixed(0)} 万`
  return `${yuan.toFixed(0)} 元`
}

export function dateTimeLocal(date = new Date()): string {
  const offset = date.getTimezoneOffset() * 60_000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

export function dateLocal(date = new Date()): string {
  const offset = date.getTimezoneOffset() * 60_000
  return new Date(date.getTime() - offset).toISOString().slice(0, 10)
}
