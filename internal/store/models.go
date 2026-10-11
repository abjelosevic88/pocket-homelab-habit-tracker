package store

// User is an account that can sign in.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Habit is a yes/no daily habit.
type Habit struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Color     string  `json:"color"`
	Days      string  `json:"days"` // ISO weekdays 1..7 the habit is scheduled on
	SortOrder int     `json:"sort_order"`
	StartDate string  `json:"start_date"`
	EndDate   *string `json:"end_date"`
	CreatedAt string  `json:"created_at"`
}

// Tracker is a daily number: a 1-5 score or a free measurement.
type Tracker struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	Kind           string  `json:"kind"` // scale | number | yesno
	MinValue       float64 `json:"min_value"`
	MaxValue       float64 `json:"max_value"`
	Step           float64 `json:"step"`
	Unit           string  `json:"unit"`
	HigherIsBetter bool    `json:"higher_is_better"`
	SortOrder      int     `json:"sort_order"`
	StartDate      string  `json:"start_date"`
	EndDate        *string `json:"end_date"`
	// DefaultValue is assumed on active past days with no entry (nil = none).
	DefaultValue *float64 `json:"default_value"`
	CreatedAt    string   `json:"created_at"`
}

// Settings are user-tunable preferences stored as key/value pairs.
type Settings struct {
	Title           string `json:"title"`
	WeekStart       int    `json:"week_start"`        // 1 = Monday, 7 = Sunday
	Landing         string `json:"landing"`           // month | today
	JournalPrompt   string `json:"journal_prompt"`    // heading of the journal column
	BadThingEnabled bool   `json:"bad_thing_enabled"` // optional second line: one bad thing
	BadPrompt       string `json:"bad_prompt"`
	ReminderTime    string `json:"reminder_time"`   // HH:MM local, empty = off
	ReminderURL     string `json:"reminder_url"`    // ntfy topic URL, Apprise endpoint or webhook
	ReminderFormat  string `json:"reminder_format"` // ntfy | apprise | json
	ReminderLast    string `json:"-"`
	SetupComplete   bool   `json:"setup_complete"`
	DefaultsSeeded  bool   `json:"-"`

	// The day's weather from Open-Meteo, for the home location (or where the phone was).
	WeatherEnabled bool    `json:"weather_enabled"`
	WeatherLat     float64 `json:"weather_lat"`
	WeatherLon     float64 `json:"weather_lon"`
	WeatherPlace   string  `json:"weather_place"`
	WeatherUnits   string  `json:"weather_units"` // celsius | fahrenheit
	WeatherLast    string  `json:"weather_last"`  // when the last fetch ran, RFC 3339; read-only
}
