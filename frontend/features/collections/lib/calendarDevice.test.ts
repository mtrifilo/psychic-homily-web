import { describe, it, expect } from 'vitest'
import { canAddOnDevice, detectCalendarDevice } from './calendarDevice'

const USER_AGENTS = {
  iphoneSafari:
    'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1',
  iphoneChrome:
    'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/131.0.6778.73 Mobile/15E148 Safari/604.1',
  iphoneInstagram:
    'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 Instagram 357.0.0.25.101',
  ipadDesktopMode:
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15',
  android:
    'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36',
  macChrome:
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
  windows:
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
} as const

describe('detectCalendarDevice', () => {
  it.each([
    ['iphoneSafari', { kind: 'iphone', isSafari: true }],
    ['iphoneChrome', { kind: 'iphone', isSafari: false }],
    ['iphoneInstagram', { kind: 'iphone', isSafari: false }],
    ['ipadDesktopMode', { kind: 'mac', isSafari: false }],
    ['android', { kind: 'android', isSafari: false }],
    ['macChrome', { kind: 'mac', isSafari: false }],
    ['windows', { kind: 'computer', isSafari: false }],
  ] as const)('%s', (name, want) => {
    expect(detectCalendarDevice(USER_AGENTS[name])).toEqual(want)
  })
})

describe('canAddOnDevice', () => {
  const device = (name: keyof typeof USER_AGENTS) =>
    detectCalendarDevice(USER_AGENTS[name])

  it.each([
    ['google', 'macChrome', true],
    ['google', 'windows', true],
    ['google', 'iphoneSafari', false],
    ['google', 'android', false],
    ['apple', 'macChrome', true],
    ['apple', 'iphoneSafari', true],
    ['apple', 'iphoneChrome', false],
    ['apple', 'windows', false],
    ['apple', 'android', false],
  ] as const)('%s on %s: %s', (target, name, want) => {
    expect(canAddOnDevice(target, device(name))).toBe(want)
  })
})
