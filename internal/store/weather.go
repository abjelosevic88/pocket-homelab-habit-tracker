package store

import (
	"context"
	"database/sql"
	"fmt"
)

// WeatherDay is one day's weather at the place that mattered that day: the home location, or
// where the phone was. The condition is one of a handful of words so it can be grouped.
type WeatherDay struct {
	Date       string   `json:"date"`
	Code       int      `json:"code"`
	Condition  string   `json:"condition"`
	TMax       *float64 `json:"t_max"`
	TMin       *float64 `json:"t_min"`
	PrecipMM   *float64 `json:"precip_mm"`
	SunshineH  *float64 `json:"sunshine_h"`
	DaylightH  *float64 `json:"daylight_h"`
	Overridden bool     `json:"overridden"`
	Lat        float64  `json:"lat"`
	Lon        float64  `json:"lon"`
	Place      string   `json:"place"`
	FetchedAt  string   `json:"fetched_at"`
}

// LocationDay is where the phone was on a day, sent by the companion app.
type LocationDay struct {
	Date       string  `json:"date"`
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
	Place      string  `json:"place"`
	Source     string  `json:"source"`
	RecordedAt string  `json:"recorded_at"`
}

// Conditions in the order the insights show them.
var Conditions = []string{"clear", "partly", "cloudy", "fog", "rain", "snow", "storm"}

// ConditionLabel is the word shown for a condition.
func ConditionLabel(c string) string {
	switch c {
	case "clear":
		return "Clear"
	case "partly":
		return "Partly cloudy"
	case "cloudy":
		return "Overcast"
	case "fog":
		return "Fog"
	case "rain":
		return "Rain"
	case "snow":
		return "Snow"
	case "storm":
		return "Thunderstorm"
	}
	return "Unknown"
}

// ConditionFromCode groups a WMO weather code into a condition.
func ConditionFromCode(code int) string {
	switch {
	case code == 0:
		return "clear"
	case code == 1 || code == 2:
		return "partly"
	case code == 3:
		return "cloudy"
	case code == 45 || code == 48:
		return "fog"
	case code >= 51 && code <= 67, code >= 80 && code <= 82:
		return "rain"
	case code >= 71 && code <= 77, code == 85 || code == 86:
		return "snow"
	case code >= 95:
		return "storm"
	}
	return ""
}

const weatherCols = `date, code, condition, t_max, t_min, precip_mm, sunshine_h, daylight_h, overridden, lat, lon, place, fetched_at`

func scanWeather(row interface{ Scan(dest ...any) error }) (WeatherDay, error) {
	var w WeatherDay
	var ov int
	var tmax, tmin, pr, sun, day sql.NullFloat64
	if err := row.Scan(&w.Date, &w.Code, &w.Condition, &tmax, &tmin, &pr, &sun, &day, &ov, &w.Lat, &w.Lon, &w.Place, &w.FetchedAt); err != nil {
		return w, err
	}
	w.Overridden = ov == 1
	f := func(n sql.NullFloat64) *float64 {
		if n.Valid {
			v := n.Float64
			return &v
		}
		return nil
	}
	w.TMax, w.TMin, w.PrecipMM, w.SunshineH, w.DaylightH = f(tmax), f(tmin), f(pr), f(sun), f(day)
	return w, nil
}

// Weather returns the user's stored days in a range, by date.
func (s *Store) Weather(ctx context.Context, uid int64, from, to string) (map[string]WeatherDay, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+weatherCols+` FROM weather_days WHERE user_id = ? AND date >= ? AND date <= ? ORDER BY date`, uid, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]WeatherDay{}
	for rows.Next() {
		w, err := scanWeather(rows)
		if err != nil {
			return nil, err
		}
		out[w.Date] = w
	}
	return out, rows.Err()
}

// WeatherDay returns one of the user's days, or sql.ErrNoRows.
func (s *Store) WeatherDay(ctx context.Context, uid int64, date string) (WeatherDay, error) {
	return scanWeather(s.DB.QueryRowContext(ctx, `SELECT `+weatherCols+` FROM weather_days WHERE user_id = ? AND date = ?`, uid, date))
}

// UpsertWeather writes a fetched day. A condition set by hand is kept; the numbers are refreshed.
func (s *Store) UpsertWeather(ctx context.Context, uid int64, w WeatherDay) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO weather_days (user_id, `+weatherCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET code = excluded.code,
			condition = CASE WHEN weather_days.overridden = 1 THEN weather_days.condition ELSE excluded.condition END,
			t_max = excluded.t_max, t_min = excluded.t_min, precip_mm = excluded.precip_mm, sunshine_h = excluded.sunshine_h, daylight_h = excluded.daylight_h,
			lat = excluded.lat, lon = excluded.lon, place = excluded.place, fetched_at = excluded.fetched_at`,
		uid, w.Date, w.Code, w.Condition, w.TMax, w.TMin, w.PrecipMM, w.SunshineH, w.DaylightH, w.Lat, w.Lon, w.Place, w.FetchedAt)
	return err
}

// SetWeatherOverride sets the condition by hand (the day row is made when there is none yet).
func (s *Store) SetWeatherOverride(ctx context.Context, uid int64, date, condition string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO weather_days (user_id, date, code, condition, overridden) VALUES (?, ?, -1, ?, 1)
		ON CONFLICT(user_id, date) DO UPDATE SET condition = excluded.condition, overridden = 1`, uid, date, condition)
	return err
}

// ClearWeatherOverride puts the fetched condition back (from the stored code).
func (s *Store) ClearWeatherOverride(ctx context.Context, uid int64, date string) error {
	w, err := s.WeatherDay(ctx, uid, date)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE weather_days SET overridden = 0, condition = ? WHERE user_id = ? AND date = ?`, ConditionFromCode(w.Code), uid, date)
	return err
}

// MissingWeatherDays lists the days in a range with no fetched weather (an overridden day
// without numbers counts as missing too).
func (s *Store) MissingWeatherDays(ctx context.Context, uid int64, from, to string) ([]string, error) {
	have, err := s.Weather(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	start, err := ParseDate(from)
	if err != nil {
		return nil, err
	}
	end, err := ParseDate(to)
	if err != nil {
		return nil, err
	}
	var out []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		if w, ok := have[ds]; !ok || w.FetchedAt == "" {
			out = append(out, ds)
		}
	}
	return out, nil
}

// Locations returns the user's days with a phone location in a range.
func (s *Store) Locations(ctx context.Context, uid int64, from, to string) (map[string]LocationDay, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date, lat, lon, place, source, recorded_at FROM location_days WHERE user_id = ? AND date >= ? AND date <= ? ORDER BY date`, uid, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]LocationDay{}
	for rows.Next() {
		var l LocationDay
		if err := rows.Scan(&l.Date, &l.Lat, &l.Lon, &l.Place, &l.Source, &l.RecordedAt); err != nil {
			return nil, err
		}
		out[l.Date] = l
	}
	return out, rows.Err()
}

// SetLocation records where the phone was on a day; the latest report of a day wins.
func (s *Store) SetLocation(ctx context.Context, uid int64, l LocationDay) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO location_days (user_id, date, lat, lon, place, source, recorded_at) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET lat = excluded.lat, lon = excluded.lon, place = excluded.place, source = excluded.source, recorded_at = excluded.recorded_at`,
		uid, l.Date, l.Lat, l.Lon, l.Place, l.Source, Now())
	return err
}

// FirstTrackedDay is the earliest start date of one of the user's habits or trackers, for the backfill.
func (s *Store) FirstTrackedDay(ctx context.Context, uid int64) (string, error) {
	var d sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT MIN(d) FROM (SELECT MIN(start_date) AS d FROM habits WHERE user_id = ? UNION ALL SELECT MIN(start_date) FROM trackers WHERE user_id = ?)`, uid, uid).Scan(&d)
	if err != nil {
		return "", err
	}
	if !d.Valid || d.String == "" {
		return "", fmt.Errorf("nothing tracked yet")
	}
	return d.String, nil
}

// ---------- insights ----------

// WeatherScore is a scale tracker's average on the days of a bucket.
type WeatherScore struct {
	TrackerID   int64    `json:"tracker_id"`
	TrackerName string   `json:"tracker_name"`
	Avg         *float64 `json:"avg"`
	N           int      `json:"n"`
}

// WeatherBucket groups days: by condition, temperature band or daylight; with each score's
// average and the habits' completion rate on those days.
type WeatherBucket struct {
	Key       string         `json:"key"`
	Label     string         `json:"label"`
	Days      int            `json:"days"`
	Scores    []WeatherScore `json:"scores"`
	HabitRate *float64       `json:"habit_rate"`
	Done      int            `json:"done"`
	Missed    int            `json:"missed"`
}

// WeatherInsights is the weather section of the insights page.
type WeatherInsights struct {
	Days          int             `json:"days"`
	ByCondition   []WeatherBucket `json:"by_condition"`
	ByTemperature []WeatherBucket `json:"by_temperature"`
	ByDaylight    []WeatherBucket `json:"by_daylight"`
	Units         string          `json:"units"`
}

// WeatherInsightsFor groups the range's days by weather and averages the scale trackers and
// the habit rate per group. Buckets with no day are left out.
func (s *Store) WeatherInsightsFor(ctx context.Context, uid int64, from, to string, habits []Habit, trackers []Tracker, hEntries map[int64]map[string]int, tEntries map[int64]map[string]float64, units string) (*WeatherInsights, error) {
	weather, err := s.Weather(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	if len(weather) == 0 {
		return nil, nil
	}
	out := &WeatherInsights{Days: len(weather), Units: units}
	type acc struct {
		days   int
		sum    map[int64]float64
		n      map[int64]int
		done   int
		missed int
	}
	newAcc := func() *acc { return &acc{sum: map[int64]float64{}, n: map[int64]int{}} }
	add := func(a *acc, date string) {
		a.days++
		for _, t := range trackers {
			if t.Kind != "scale" {
				continue
			}
			if v, ok := tEntries[t.ID][date]; ok {
				a.sum[t.ID] += v
				a.n[t.ID]++
			}
		}
		for _, h := range habits {
			switch hEntries[h.ID][date] {
			case 1:
				a.done++
			case -1:
				a.missed++
			}
		}
	}
	finish := func(key, label string, a *acc) WeatherBucket {
		b := WeatherBucket{Key: key, Label: label, Days: a.days, Scores: []WeatherScore{}, Done: a.done, Missed: a.missed}
		for _, t := range trackers {
			if t.Kind != "scale" {
				continue
			}
			sc := WeatherScore{TrackerID: t.ID, TrackerName: t.Name, N: a.n[t.ID]}
			if sc.N > 0 {
				v := round2(a.sum[t.ID] / float64(sc.N))
				sc.Avg = &v
			}
			b.Scores = append(b.Scores, sc)
		}
		if n := a.done + a.missed; n > 0 {
			r := round2(float64(a.done) / float64(n))
			b.HabitRate = &r
		}
		return b
	}
	byCond := map[string]*acc{}
	tempBands := []struct {
		key, label string
		lo, hi     float64
	}{{"cold", "Below 5 °C", -1e9, 5}, {"cool", "5–15 °C", 5, 15}, {"mild", "15–25 °C", 15, 25}, {"hot", "Above 25 °C", 25, 1e9}}
	if units == "fahrenheit" {
		tempBands = []struct {
			key, label string
			lo, hi     float64
		}{{"cold", "Below 41 °F", -1e9, 5}, {"cool", "41–59 °F", 5, 15}, {"mild", "59–77 °F", 15, 25}, {"hot", "Above 77 °F", 25, 1e9}}
	}
	byTemp := map[string]*acc{}
	dayBands := []struct {
		key, label string
		lo, hi     float64
	}{{"short", "Under 10 h of daylight", -1, 10}, {"medium", "10–13 h", 10, 13}, {"long", "Over 13 h", 13, 99}}
	byDay := map[string]*acc{}
	for date, w := range weather {
		if w.Condition != "" {
			if byCond[w.Condition] == nil {
				byCond[w.Condition] = newAcc()
			}
			add(byCond[w.Condition], date)
		}
		if w.TMax != nil {
			for _, b := range tempBands {
				if *w.TMax >= b.lo && *w.TMax < b.hi {
					if byTemp[b.key] == nil {
						byTemp[b.key] = newAcc()
					}
					add(byTemp[b.key], date)
				}
			}
		}
		if w.DaylightH != nil {
			for _, b := range dayBands {
				if *w.DaylightH >= b.lo && *w.DaylightH < b.hi {
					if byDay[b.key] == nil {
						byDay[b.key] = newAcc()
					}
					add(byDay[b.key], date)
				}
			}
		}
	}
	out.ByCondition = []WeatherBucket{}
	for _, c := range Conditions {
		if a := byCond[c]; a != nil {
			out.ByCondition = append(out.ByCondition, finish(c, ConditionLabel(c), a))
		}
	}
	out.ByTemperature = []WeatherBucket{}
	for _, b := range tempBands {
		if a := byTemp[b.key]; a != nil {
			out.ByTemperature = append(out.ByTemperature, finish(b.key, b.label, a))
		}
	}
	out.ByDaylight = []WeatherBucket{}
	for _, b := range dayBands {
		if a := byDay[b.key]; a != nil {
			out.ByDaylight = append(out.ByDaylight, finish(b.key, b.label, a))
		}
	}
	return out, nil
}
