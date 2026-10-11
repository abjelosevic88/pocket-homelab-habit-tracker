export interface User { id: number; email: string; name: string; role: string; created_at: string }

export interface Settings {
  title: string
  week_start: number
  landing: 'month' | 'today'
  journal_prompt: string
  bad_thing_enabled: boolean
  bad_prompt: string
  reminder_time: string
  reminder_url: string
  reminder_format: 'ntfy' | 'apprise' | 'json'
  setup_complete: boolean
  weather_enabled: boolean
  weather_lat: number
  weather_lon: number
  weather_place: string
  weather_units: 'celsius' | 'fahrenheit'
  weather_last: string
}

export type WeatherCondition = 'clear' | 'partly' | 'cloudy' | 'fog' | 'rain' | 'snow' | 'storm' | ''
export interface WeatherDay { date: string; code: number; condition: WeatherCondition; t_max: number | null; t_min: number | null; precip_mm: number | null; sunshine_h: number | null; daylight_h: number | null; overridden: boolean; lat: number; lon: number; place: string; fetched_at: string }
export interface WeatherScore { tracker_id: number; tracker_name: string; avg: number | null; n: number }
export interface WeatherBucket { key: string; label: string; days: number; scores: WeatherScore[]; habit_rate: number | null; done: number; missed: number }
export interface WeatherInsights { days: number; by_condition: WeatherBucket[]; by_temperature: WeatherBucket[]; by_daylight: WeatherBucket[]; units: string }
export interface GeoPlace { name: string; label: string; lat: number; lon: number; timezone: string }

export interface Habit {
  id: number
  name: string
  color: string
  days: string
  sort_order: number
  start_date: string
  end_date: string | null
  created_at: string
}

export interface Tracker {
  id: number
  name: string
  kind: 'scale' | 'number' | 'yesno'
  min_value: number
  max_value: number
  step: number
  unit: string
  higher_is_better: boolean
  sort_order: number
  start_date: string
  end_date: string | null
  created_at: string
}

export interface Day { date: string; day: number; weekday: number; label: string; weekend: boolean; today: boolean; future: boolean }

export interface MonthHabit extends Habit { entries: Record<string, number>; done: number; missed: number; scheduled: number; streak: number }
export interface MonthTracker extends Tracker { entries: Record<string, number>; avg: number | null; min: number | null; max: number | null; first: number | null; last: number | null }
export interface MonthSummary { done: number; missed: number; unmarked: number; rate: number | null; perfect_days: number; journal_days: number }
export interface MonthPage { month: string; days: Day[]; habits: MonthHabit[]; trackers: MonthTracker[]; journal: Record<string, string>; journal_bad: Record<string, string>; summary: MonthSummary; weather?: Record<string, WeatherDay> | null }

export interface TodayHabit extends Habit { status: number; scheduled: boolean; streak: number }
export interface TodayTracker extends Tracker { value: number | null; previous: number | null }
export interface TodayPage { date: string; weekday: number; habits: TodayHabit[]; trackers: TodayTracker[]; journal: string; journal_bad: string; done: number; total: number; weather?: WeatherDay | null }

export interface Dashboard {
  date: string; today_done: number; today_total: number; today_left: number; journal_today: boolean
  month_done: number; month_missed: number; month_rate: number | null; month_rate_pct: number | null
  perfect_days: number; perfect_streak: number; best_habit_streak: number; best_habit: string
}

export interface HabitInsight extends Habit { done: number; missed: number; scheduled: number; rate: number | null; streak: number; best_streak: number; weekdays: (number | null)[] }
export interface TrackerPoint { date: string; value: number }
export interface TrackerInsight extends Tracker { count: number; avg: number | null; min: number | null; max: number | null; first: number | null; last: number | null; series: TrackerPoint[]; weekly: Record<string, number> }
export interface Correlation { tracker_id: number; tracker_name: string; kind: 'habit' | 'yesno'; habit_id: number; habit_name: string; avg_done: number | null; avg_missed: number | null; n_done: number; n_missed: number; delta: number | null }
export interface Insights { from: string; to: string; habits: HabitInsight[]; trackers: TrackerInsight[]; correlations: Correlation[]; journal: { date: string; text: string; bad?: string }[]; weather?: WeatherInsights | null }

export interface APIToken { id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }
