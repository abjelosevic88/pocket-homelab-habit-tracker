package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/store"
)

// The day's weather comes from Open-Meteo (free, no key): the forecast API answers the last
// three months with past_days, the archive API anything older. One fetch a day for the home
// location, and one per day the phone reported another place.

const openMeteoForecast = "https://api.open-meteo.com/v1/forecast"
const openMeteoArchive = "https://archive-api.open-meteo.com/v1/archive"
const openMeteoGeocode = "https://geocoding-api.open-meteo.com/v1/search"
const dailyVars = "weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum,sunshine_duration,daylight_duration"

var weatherHTTP = &http.Client{Timeout: 20 * time.Second}

type openMeteoDaily struct {
	Time             []string   `json:"time"`
	WeatherCode      []*float64 `json:"weather_code"`
	TMax             []*float64 `json:"temperature_2m_max"`
	TMin             []*float64 `json:"temperature_2m_min"`
	Precip           []*float64 `json:"precipitation_sum"`
	SunshineDuration []*float64 `json:"sunshine_duration"`
	DaylightDuration []*float64 `json:"daylight_duration"`
}

// fetchWeather asks Open-Meteo for the days from..to at a place and returns one record per day it answered.
func fetchWeather(ctx context.Context, lat, lon float64, from, to, place string) ([]store.WeatherDay, error) {
	today := store.Today()
	if to > today {
		to = today
	}
	if from > to {
		return nil, nil
	}
	// The forecast API reaches back 92 days; older days come from the archive.
	cut := time.Now().AddDate(0, 0, -85).Format("2006-01-02")
	var out []store.WeatherDay
	if from < cut {
		archTo := to
		if archTo >= cut {
			archTo = time.Now().AddDate(0, 0, -86).Format("2006-01-02")
		}
		days, err := fetchOpenMeteo(ctx, openMeteoArchive, lat, lon, from, archTo, place)
		if err != nil {
			return nil, err
		}
		out = append(out, days...)
		from = cut
	}
	if from <= to {
		days, err := fetchOpenMeteo(ctx, openMeteoForecast, lat, lon, from, to, place)
		if err != nil {
			return nil, err
		}
		out = append(out, days...)
	}
	return out, nil
}

func fetchOpenMeteo(ctx context.Context, base string, lat, lon float64, from, to, place string) ([]store.WeatherDay, error) {
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.4f", lat))
	q.Set("longitude", fmt.Sprintf("%.4f", lon))
	q.Set("daily", dailyVars)
	q.Set("timezone", "auto")
	q.Set("start_date", from)
	q.Set("end_date", to)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "pocket-habits/"+Version)
	res, err := weatherHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open-meteo: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		var e struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(res.Body).Decode(&e)
		if e.Reason == "" {
			e.Reason = res.Status
		}
		return nil, fmt.Errorf("open-meteo: %s", e.Reason)
	}
	var body struct {
		Daily openMeteoDaily `json:"daily"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("open-meteo: %w", err)
	}
	now := store.Now()
	var out []store.WeatherDay
	at := func(a []*float64, i int) *float64 {
		if i < len(a) && a[i] != nil {
			v := math.Round(*a[i]*10) / 10
			return &v
		}
		return nil
	}
	for i, d := range body.Daily.Time {
		code := -1
		if c := at(body.Daily.WeatherCode, i); c != nil {
			code = int(*c)
		}
		if code < 0 && at(body.Daily.TMax, i) == nil {
			continue // a day the station has nothing for (today before the first hours, or too old)
		}
		w := store.WeatherDay{Date: d, Code: code, Condition: store.ConditionFromCode(code), TMax: at(body.Daily.TMax, i), TMin: at(body.Daily.TMin, i),
			PrecipMM: at(body.Daily.Precip, i), Lat: lat, Lon: lon, Place: place, FetchedAt: now}
		if s := at(body.Daily.SunshineDuration, i); s != nil {
			h := math.Round(*s/360) / 10
			w.SunshineH = &h
		}
		if s := at(body.Daily.DaylightDuration, i); s != nil {
			h := math.Round(*s/360) / 10
			w.DaylightH = &h
		}
		out = append(out, w)
	}
	return out, nil
}

// refreshWeather fetches the days in from..to: the phone's place for days that have one,
// the home location for the rest. Returns how many days were written.
func (s *Server) refreshWeather(ctx context.Context, uid int64, st store.Settings, from, to string) (int, error) {
	if !st.WeatherEnabled || (st.WeatherLat == 0 && st.WeatherLon == 0) {
		return 0, fmt.Errorf("weather is off or the home location is not set")
	}
	today := store.Today()
	if to > today {
		to = today
	}
	locs, err := s.store.Locations(ctx, uid, from, to)
	if err != nil {
		return 0, err
	}
	n := 0
	// Home-location days as one range; the phone's days one by one (each is its own place).
	homeDays := []string{}
	start, _ := store.ParseDate(from)
	end, _ := store.ParseDate(to)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		if l, ok := locs[ds]; ok {
			days, err := fetchWeather(ctx, l.Lat, l.Lon, ds, ds, l.Place)
			if err != nil {
				return n, err
			}
			for _, w := range days {
				if err := s.store.UpsertWeather(ctx, uid, w); err != nil {
					return n, err
				}
				n++
			}
		} else {
			homeDays = append(homeDays, ds)
		}
	}
	// Contiguous runs of home days, so one call covers a whole month.
	for i := 0; i < len(homeDays); {
		j := i
		for j+1 < len(homeDays) {
			a, _ := store.ParseDate(homeDays[j])
			b, _ := store.ParseDate(homeDays[j+1])
			if b.Sub(a) != 24*time.Hour {
				break
			}
			j++
		}
		days, err := fetchWeather(ctx, st.WeatherLat, st.WeatherLon, homeDays[i], homeDays[j], st.WeatherPlace)
		if err != nil {
			return n, err
		}
		for _, w := range days {
			if err := s.store.UpsertWeather(ctx, uid, w); err != nil {
				return n, err
			}
			n++
		}
		i = j + 1
	}
	_ = s.store.SetSetting(ctx, uid, "weather_last", store.Now())
	return n, nil
}

// backfillWeather fetches every day since the first habit or tracker that has no weather yet,
// at most two years back.
func (s *Server) backfillWeather(ctx context.Context, uid int64, st store.Settings) (int, error) {
	from, err := s.store.FirstTrackedDay(ctx, uid)
	if err != nil {
		from = time.Now().AddDate(0, -3, 0).Format("2006-01-02")
	}
	if floor := time.Now().AddDate(-2, 0, 0).Format("2006-01-02"); from < floor {
		from = floor
	}
	missing, err := s.store.MissingWeatherDays(ctx, uid, from, store.Today())
	if err != nil || len(missing) == 0 {
		return 0, err
	}
	return s.refreshWeather(ctx, uid, st, missing[0], missing[len(missing)-1])
}

// RunWeather is called by the scheduler every minute for each user: every three hours it
// refreshes today and yesterday at their place (today's numbers firm up during the day),
// and fills in their missing days once.
func (s *Server) RunWeather(ctx context.Context, uid int64) {
	st, err := s.store.GetSettings(ctx, uid)
	if err != nil || !st.WeatherEnabled || (st.WeatherLat == 0 && st.WeatherLon == 0) {
		return
	}
	if st.WeatherLast != "" {
		if last, err := time.Parse(time.RFC3339Nano, st.WeatherLast); err == nil && time.Since(last) < 3*time.Hour {
			return
		}
	}
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if _, err := s.refreshWeather(ctx, uid, st, yesterday, store.Today()); err != nil {
		s.log.Warn("weather refresh failed", "user", uid, "err", err)
		_ = s.store.SetSetting(ctx, uid, "weather_last", store.Now()) // try again in three hours, not every minute
		return
	}
	if n, err := s.backfillWeather(ctx, uid, st); err != nil {
		s.log.Warn("weather backfill failed", "user", uid, "err", err)
	} else if n > 0 {
		s.log.Info("weather backfilled", "user", uid, "days", n)
	}
}

// ---------- handlers ----------

func (s *Server) handleWeather(w http.ResponseWriter, r *http.Request) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if to == "" {
		to = store.Today()
	}
	if from == "" {
		from = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	}
	if !validDate(from) || !validDate(to) {
		writeErr(w, http.StatusBadRequest, "from/to must be YYYY-MM-DD")
		return
	}
	days, err := s.store.Weather(r.Context(), me(r), from, to)
	if err != nil {
		s.fail(w, err, "weather")
		return
	}
	list := make([]store.WeatherDay, 0, len(days))
	for _, d := range days {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Date < list[j].Date })
	writeJSON(w, http.StatusOK, list)
}

// handleSetWeather sets a day's condition by hand: {"condition": "rain"}; an empty condition
// puts the fetched one back.
func (s *Server) handleSetWeather(w http.ResponseWriter, r *http.Request) {
	date := chi.URLParam(r, "date")
	if !validDate(date) || date > store.Today() {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD and not in the future")
		return
	}
	var in struct {
		Condition string `json:"condition"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Condition = strings.ToLower(strings.TrimSpace(in.Condition))
	ctx := r.Context()
	uid := me(r)
	if in.Condition == "" {
		if err := s.store.ClearWeatherOverride(ctx, uid, date); err != nil {
			s.fail(w, err, "weather")
			return
		}
	} else {
		ok := false
		for _, c := range store.Conditions {
			if c == in.Condition {
				ok = true
			}
		}
		if !ok {
			writeErr(w, http.StatusBadRequest, "condition must be one of "+strings.Join(store.Conditions, ", "))
			return
		}
		if err := s.store.SetWeatherOverride(ctx, uid, date, in.Condition); err != nil {
			s.fail(w, err, "weather")
			return
		}
	}
	d, err := s.store.WeatherDay(ctx, uid, date)
	if err != nil {
		s.fail(w, err, "weather")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// handleRefreshWeather fetches now: {"from":"…","to":"…"} or nothing for the backfill plus today.
func (s *Server) handleRefreshWeather(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSettings(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	_ = decode(r, &in)
	var n int
	if in.From != "" || in.To != "" {
		if in.To == "" {
			in.To = store.Today()
		}
		if in.From == "" {
			in.From = in.To
		}
		if !validDate(in.From) || !validDate(in.To) {
			writeErr(w, http.StatusBadRequest, "from/to must be YYYY-MM-DD")
			return
		}
		n, err = s.refreshWeather(r.Context(), me(r), st, in.From, in.To)
	} else {
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		n, err = s.refreshWeather(r.Context(), me(r), st, yesterday, store.Today())
		if err == nil {
			var more int
			more, err = s.backfillWeather(r.Context(), me(r), st)
			n += more
		}
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": n})
}

// handleGeocode looks a place name up (Open-Meteo's geocoder), for the settings page.
func (s *Server) handleGeocode(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	v := url.Values{}
	v.Set("name", q)
	v.Set("count", "8")
	v.Set("language", "en")
	v.Set("format", "json")
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, openMeteoGeocode+"?"+v.Encode(), nil)
	req.Header.Set("User-Agent", "pocket-habits/"+Version)
	res, err := weatherHTTP.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "geocoder: "+err.Error())
		return
	}
	defer res.Body.Close()
	var body struct {
		Results []struct {
			Name     string  `json:"name"`
			Country  string  `json:"country"`
			Admin1   string  `json:"admin1"`
			Lat      float64 `json:"latitude"`
			Lon      float64 `json:"longitude"`
			Timezone string  `json:"timezone"`
		} `json:"results"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadGateway, "geocoder: "+err.Error())
		return
	}
	type place struct {
		Name     string  `json:"name"`
		Label    string  `json:"label"`
		Lat      float64 `json:"lat"`
		Lon      float64 `json:"lon"`
		Timezone string  `json:"timezone"`
	}
	out := make([]place, 0, len(body.Results))
	for _, p := range body.Results {
		parts := []string{p.Name}
		if p.Admin1 != "" && p.Admin1 != p.Name {
			parts = append(parts, p.Admin1)
		}
		if p.Country != "" {
			parts = append(parts, p.Country)
		}
		out = append(out, place{Name: p.Name, Label: strings.Join(parts, ", "), Lat: math.Round(p.Lat*10000) / 10000, Lon: math.Round(p.Lon*10000) / 10000, Timezone: p.Timezone})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetLocation takes the phone's position for a day: {"date":"…","lat":..,"lon":..,"place":"…","source":"iphone"}.
// The day's weather is fetched for that place at once.
func (s *Server) handleSetLocation(w http.ResponseWriter, r *http.Request) {
	var in store.LocationDay
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Date == "" {
		in.Date = store.Today()
	}
	if !validDate(in.Date) || in.Date > store.Today() {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD and not in the future")
		return
	}
	if in.Lat < -90 || in.Lat > 90 || in.Lon < -180 || in.Lon > 180 || (in.Lat == 0 && in.Lon == 0) {
		writeErr(w, http.StatusBadRequest, "lat/lon out of range")
		return
	}
	in.Lat, in.Lon = math.Round(in.Lat*1000)/1000, math.Round(in.Lon*1000)/1000 // ~100 m is plenty for the weather
	in.Place = strings.TrimSpace(in.Place)
	in.Source = strings.TrimSpace(in.Source)
	ctx := r.Context()
	uid := me(r)
	if err := s.store.SetLocation(ctx, uid, in); err != nil {
		s.fail(w, err, "location")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	var weather *store.WeatherDay
	if st.WeatherEnabled {
		if days, err := fetchWeather(ctx, in.Lat, in.Lon, in.Date, in.Date, in.Place); err == nil {
			for _, d := range days {
				_ = s.store.UpsertWeather(ctx, uid, d)
			}
			if d, err := s.store.WeatherDay(ctx, uid, in.Date); err == nil {
				weather = &d
			}
		} else {
			s.log.Warn("weather for reported location failed", "err", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"location": in, "weather": weather})
}

func (s *Server) handleLocations(w http.ResponseWriter, r *http.Request) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if to == "" {
		to = store.Today()
	}
	if from == "" {
		from = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	}
	if !validDate(from) || !validDate(to) {
		writeErr(w, http.StatusBadRequest, "from/to must be YYYY-MM-DD")
		return
	}
	locs, err := s.store.Locations(r.Context(), me(r), from, to)
	if err != nil {
		s.fail(w, err, "locations")
		return
	}
	list := make([]store.LocationDay, 0, len(locs))
	for _, l := range locs {
		list = append(list, l)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Date < list[j].Date })
	writeJSON(w, http.StatusOK, list)
}
