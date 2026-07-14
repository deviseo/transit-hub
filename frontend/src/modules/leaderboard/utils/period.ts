import type { LeaderboardDateRange, LeaderboardPeriod } from '../types'

const timezone = 'Asia/Shanghai'

const shanghaiDateParts = (value: Date): [number, number, number] => {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(value)
  const part = (type: Intl.DateTimeFormatPartTypes): number => Number(parts.find((item) => item.type === type)?.value)
  return [part('year'), part('month'), part('day')]
}

const addDays = (value: Date, days: number): Date => {
  const next = new Date(value)
  next.setUTCDate(next.getUTCDate() + days)
  return next
}

const formatDate = (value: Date): string => value.toISOString().slice(0, 10)

export const leaderboardDateRange = (period: LeaderboardPeriod, now = new Date()): LeaderboardDateRange => {
  const [year, month, day] = shanghaiDateParts(now)
  const today = new Date(Date.UTC(year, month - 1, day))
  const days = period === '30d' ? 30 : period === '7d' ? 7 : 1
  return {
    startDate: formatDate(addDays(today, -(days - 1))),
    endDate: formatDate(addDays(today, 1)),
  }
}
