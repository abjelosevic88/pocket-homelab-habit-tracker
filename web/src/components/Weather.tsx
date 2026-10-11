import type { WeatherCondition, WeatherDay } from '../lib/types'

export const CONDITION_ICON: Record<string, string> = { clear: '☀️', partly: '⛅', cloudy: '☁️', fog: '🌫️', rain: '🌧️', snow: '❄️', storm: '⛈️' }
export const CONDITION_LABEL: Record<string, string> = { clear: 'Clear', partly: 'Partly cloudy', cloudy: 'Overcast', fog: 'Fog', rain: 'Rain', snow: 'Snow', storm: 'Thunderstorm' }
export const CONDITIONS: WeatherCondition[] = ['clear', 'partly', 'cloudy', 'fog', 'rain', 'snow', 'storm']

export function temp(v: number | null | undefined, units = 'celsius'): string {
  if (v === null || v === undefined) return '–'
  return units === 'fahrenheit' ? `${Math.round(v * 9 / 5 + 32)}°` : `${Math.round(v)}°`
}

/** "Rain · 9–14 °C · 1.2 h sun · 10.4 h daylight" */
export function weatherLine(w: WeatherDay, units = 'celsius'): string {
  const parts: string[] = []
  if (w.condition) parts.push(CONDITION_LABEL[w.condition] || w.condition)
  if (w.t_min !== null || w.t_max !== null) parts.push(`${temp(w.t_min, units)}–${temp(w.t_max, units)}${units === 'fahrenheit' ? 'F' : 'C'}`)
  if (w.precip_mm !== null && w.precip_mm > 0) parts.push(`${w.precip_mm} mm`)
  if (w.sunshine_h !== null) parts.push(`${w.sunshine_h} h sun`)
  if (w.daylight_h !== null) parts.push(`${w.daylight_h} h daylight`)
  if (w.place) parts.push(w.place)
  return parts.join(' · ')
}

/** The icon with a title, for a strip or a header. */
export function WeatherIcon({ w, units = 'celsius', size = 14 }: { w: WeatherDay; units?: string; size?: number }) {
  return <span className="wx-icon" style={{ fontSize: size }} title={weatherLine(w, units)} aria-label={weatherLine(w, units)}>{CONDITION_ICON[w.condition] || '·'}</span>
}
