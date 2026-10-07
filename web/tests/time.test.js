import { describe, it, expect } from 'vitest'
import { formatTime } from '../src/utils/time'

describe('formatTime', () => {
  it('空值返回占位符', () => {
    expect(formatTime(null)).toBe('-')
    expect(formatTime(undefined)).toBe('-')
    expect(formatTime('')).toBe('-')
  })

  it('非法日期原样返回', () => {
    expect(formatTime('not a date')).toBe('not a date')
  })

  it('格式化为 YYYY-MM-DD HH:mm:ss', () => {
    const out = formatTime('2026-10-06T08:05:09')
    expect(out).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)
  })

  it('各段补零', () => {
    const d = new Date(2026, 0, 5, 9, 7, 3) // 2026-01-05 09:07:03
    expect(formatTime(d)).toBe('2026-01-05 09:07:03')
  })

  it('接受 Date 与合法日期字符串', () => {
    expect(formatTime(new Date(2026, 5, 1, 12, 0, 0))).toBe('2026-06-01 12:00:00')
    expect(formatTime('2026-06-01 12:00:00')).toMatch(/^2026-06-01 12:00:00$/)
  })
})