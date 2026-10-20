package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/config"
	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/store"
)

// fakeCore answers token introspection (pcd_<who> → that person) and swallows events.
type fakeCore struct {
	*httptest.Server
	mu     sync.Mutex
	people map[string][2]string // token → email, name
	links  map[string][3]string // pcx_ token → email, name, scopes (a linked app acting for that person)
	events []map[string]any
}

func newFakeCore(t *testing.T) *fakeCore {
	c := &fakeCore{people: map[string][2]string{}, links: map[string][3]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/introspect", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Token string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		c.mu.Lock()
		p, ok := c.people[in.Token]
		l, isLink := c.links[in.Token]
		c.mu.Unlock()
		if isLink {
			json.NewEncoder(w).Encode(map[string]any{"active": true, "kind": "delegated", "scope": l[2], "audience": "habits",
				"user": map[string]any{"id": 9, "email": l[0], "name": l[1], "role": "member"}, "app": map[string]any{"id": 3, "slug": "kitchen", "name": "Pocket Kitchen"}})
			return
		}
		if !ok {
			json.NewEncoder(w).Encode(map[string]any{"active": false})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"active": true, "kind": "device", "user": map[string]any{"id": 9, "email": p[0], "name": p[1], "role": "member"}})
	})
	mux.HandleFunc("/api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		var ev map[string]any
		_ = json.NewDecoder(r.Body).Decode(&ev)
		c.mu.Lock()
		c.events = append(c.events, ev)
		c.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{}`))
	})
	c.Server = httptest.NewServer(mux)
	t.Cleanup(c.Close)
	return c
}

type client struct {
	t    *testing.T
	base string
	hc   *http.Client
	tok  string
}

func (c *client) do(method, path, body string) (int, string) {
	c.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.tok != "" {
		req.Header.Set("Authorization", "Bearer "+c.tok)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func (c *client) json(method, path, body string, out any) int {
	c.t.Helper()
	code, b := c.do(method, path, body)
	if out != nil && code < 300 {
		if err := json.Unmarshal([]byte(b), out); err != nil {
			c.t.Fatalf("%s %s: %v (%s)", method, path, err, b)
		}
	}
	return code
}

// TestPerUserIsolation: user B (a member, reached via API token, Pocket Core device token
// and a password session) can list, read, change, delete, search and export none of user
// A's data; another user's ids answer 404, never 403.
func TestPerUserIsolation(t *testing.T) {
	core := newFakeCore(t)
	dir := t.TempDir()
	cfg := &config.Config{AuthMode: "local", SessionTTL: time.Hour, DataDir: dir, DBPath: dir + "/t.db", CoreURL: core.URL}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv := New(cfg, st, log, nil)
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)
	ctx := context.Background()

	pw, _ := bcrypt.GenerateFromPassword([]byte("password-b"), bcrypt.MinCost)
	a := &store.User{Email: "a@example.com", Name: "A", Role: "admin", PasswordHash: "x"}
	b := &store.User{Email: "b@example.com", Name: "B", Role: "member", PasswordHash: string(pw)}
	for _, u := range []*store.User{a, b} {
		if err := st.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		if err := srv.setupUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	tokA, _, _ := st.CreateAPIToken(ctx, a.ID, "a")
	tokB, _, _ := st.CreateAPIToken(ctx, b.ID, "b")
	core.people["pcd_b"] = [2]string{"b@example.com", "B"}
	core.people["pcd_newcomer"] = [2]string{"newcomer@example.com", "Newcomer"}

	A := &client{t: t, base: ts.URL, hc: http.DefaultClient, tok: tokA}
	today := store.Today()

	// A's data: a habit, a tracker, entries, journal, weather override, location, settings.
	var h store.Habit
	if c := A.json("POST", "/api/v1/habits", `{"name":"Secret habit"}`, &h); c != 201 {
		t.Fatalf("create habit %d", c)
	}
	var tr store.Tracker
	if c := A.json("POST", "/api/v1/trackers", `{"name":"Secret tracker","kind":"number"}`, &tr); c != 201 {
		t.Fatalf("create tracker %d", c)
	}
	if c, body := A.do("POST", "/api/v1/checkin", `{"habits":{"`+itoa(h.ID)+`":1},"trackers":{"`+itoa(tr.ID)+`":42},"journal":"secret journal","important":"secret important"}`); c != 200 {
		t.Fatalf("checkin %d %s", c, body)
	}
	if c, _ := A.do("PUT", "/api/v1/weather/"+today, `{"condition":"rain"}`); c != 200 {
		t.Fatalf("weather override %d", c)
	}
	if c, _ := A.do("POST", "/api/v1/location", `{"lat":44.77,"lon":17.2,"place":"Secret place"}`); c != 200 {
		t.Fatalf("location %d", c)
	}
	if c, _ := A.do("PUT", "/api/v1/settings", `{"reminder_url":"https://ntfy.example/secret-topic","journal_prompt":"Secret prompt"}`); c != 200 {
		t.Fatalf("settings %d", c)
	}

	hid, tid := itoa(h.ID), itoa(tr.ID)
	secrets := []string{"Secret habit", "Secret tracker", "secret journal", "secret important", "Secret place", "secret-topic", "Secret prompt"}

	jar, _ := cookiejar.New(nil)
	session := &client{t: t, base: ts.URL, hc: &http.Client{Jar: jar}}
	if c, _ := session.do("POST", "/api/v1/auth/login", `{"email":"b@example.com","password":"password-b"}`); c != 200 {
		t.Fatalf("B login %d", c)
	}
	for _, B := range []*client{
		{t: t, base: ts.URL, hc: http.DefaultClient, tok: tokB},           // API token
		{t: t, base: ts.URL, hc: http.DefaultClient, tok: "pcd_b"},        // Pocket Core device token
		{t: t, base: ts.URL, hc: http.DefaultClient, tok: "pcd_newcomer"}, // Core token, account made on first sight
		session, // password session
	} {
		name := B.tok
		if name == "" {
			name = "session"
		} else if strings.HasPrefix(name, "ph_") {
			name = "api token"
		}
		// reads and searches: nothing of A's shows up anywhere
		for _, path := range []string{
			"/api/v1/habits?all=1", "/api/v1/trackers?all=1", "/api/v1/months/current", "/api/v1/days/" + today, "/api/v1/today",
			"/api/v1/dashboard", "/api/v1/insights?months=2", "/api/v1/journal", "/api/v1/weather", "/api/v1/location",
			"/api/v1/settings", "/api/v1/export.json", "/api/v1/export.csv", "/api/v1/settings/system",
		} {
			c, body := B.do("GET", path, "")
			if c != 200 {
				t.Errorf("%s: GET %s = %d", name, path, c)
			}
			for _, sec := range secrets {
				if strings.Contains(body, sec) {
					t.Errorf("%s: GET %s leaks %q", name, path, sec)
				}
			}
		}
		// another person's ids are 404, never 403
		for _, r := range []struct{ method, path, body string }{
			{"GET", "/api/v1/habits/" + hid, ""},
			{"PUT", "/api/v1/habits/" + hid, `{"name":"pwned"}`},
			{"POST", "/api/v1/habits/" + hid + "/end", `{}`},
			{"POST", "/api/v1/habits/" + hid + "/resume", `{}`},
			{"PUT", "/api/v1/habits/" + hid + "/entries/" + today, `{"status":-1}`},
			{"DELETE", "/api/v1/habits/" + hid + "/entries/" + today, ""},
			{"DELETE", "/api/v1/habits/" + hid, ""},
			{"GET", "/api/v1/trackers/" + tid, ""},
			{"PUT", "/api/v1/trackers/" + tid, `{"name":"pwned","kind":"number"}`},
			{"POST", "/api/v1/trackers/" + tid + "/end", `{}`},
			{"POST", "/api/v1/trackers/" + tid + "/resume", `{}`},
			{"PUT", "/api/v1/trackers/" + tid + "/entries/" + today, `{"value":1}`},
			{"DELETE", "/api/v1/trackers/" + tid + "/entries/" + today, ""},
			{"DELETE", "/api/v1/trackers/" + tid, ""},
			{"POST", "/api/v1/checkin", `{"habits":{"` + hid + `":-1}}`},
			{"POST", "/api/v1/checkin", `{"trackers":{"` + tid + `":null}}`},
		} {
			if c, body := B.do(r.method, r.path, r.body); c != 404 {
				t.Errorf("%s: %s %s = %d %s (want 404)", name, r.method, r.path, c, body)
			}
		}
		// reordering with A's ids, writing B's own journal/weather/settings and importing A's
		// names touch only B's rows
		B.do("PUT", "/api/v1/habits/reorder", `{"ids":[`+hid+`]}`)
		B.do("PUT", "/api/v1/trackers/reorder", `{"ids":[`+tid+`]}`)
		B.do("PUT", "/api/v1/journal/"+today, `{"text":"b's own line","important":""}`)
		B.do("PUT", "/api/v1/weather/"+today, `{"condition":"clear"}`)
		B.do("PUT", "/api/v1/settings", `{"title":"Hijacked","reminder_url":"https://b.example"}`)
		// the whole-database download is not for members (nor for anyone once others live here)
		if c, _ := B.do("GET", "/api/v1/backup", ""); c != 403 {
			t.Errorf("%s: backup = %d, want 403", name, c)
		}
	}

	// An import that names A's habit (same name, A's id) lands in B's own dataset.
	Bt := &client{t: t, base: ts.URL, hc: http.DefaultClient, tok: tokB}
	if c, body := Bt.do("POST", "/api/v1/import", `{"habits":[{"id":`+hid+`,"name":"Secret habit"}],"habit_entries":[{"habit_id":`+hid+`,"date":"`+today+`","status":-1}],"journal":[{"date":"`+today+`","text":"imported by b"}]}`); c != 200 || !strings.Contains(body, `"created":1`) {
		t.Errorf("B import %d %s", c, body)
	}
	if bh, _ := st.ListHabits(ctx, b.ID, "", true); len(bh) != 1 || bh[0].ID == h.ID {
		t.Errorf("B's import: %+v", bh)
	}

	// A's data is exactly as A left it.
	var got store.Habit
	if c := A.json("GET", "/api/v1/habits/"+hid, "", &got); c != 200 || got.Name != "Secret habit" || got.EndDate != nil {
		t.Errorf("A's habit changed: %d %+v", c, got)
	}
	var day store.TodayPage
	A.json("GET", "/api/v1/today", "", &day)
	if len(day.Habits) != 1 || day.Habits[0].Status != 1 || day.Journal != "secret journal" || day.Important != "secret important" || day.Weather == nil || day.Weather.Condition != "rain" {
		t.Errorf("A's day changed: %+v", day)
	}
	found := false
	for _, x := range day.Trackers {
		if x.ID == tr.ID && x.Value != nil && *x.Value == 42 {
			found = true
		}
	}
	if !found {
		t.Errorf("A's tracker value changed: %+v", day.Trackers)
	}
	var habits []store.Habit
	A.json("GET", "/api/v1/habits?all=1", "", &habits)
	if len(habits) != 1 {
		t.Errorf("A sees %d habits, want 1 (B's imports must stay B's)", len(habits))
	}
	var sts store.Settings
	A.json("GET", "/api/v1/settings", "", &sts)
	if sts.ReminderURL != "https://ntfy.example/secret-topic" || sts.Title != "Pocket Habits" {
		t.Errorf("A's settings changed or a member renamed the box: %+v", sts)
	}
	// the admin may rename the box; a member cannot (checked above)
	A.do("PUT", "/api/v1/settings", `{"title":"Our habits","reminder_url":"https://ntfy.example/secret-topic"}`)
	if st, _ := st.GetSettings(ctx, b.ID); st.Title != "Our habits" || st.ReminderURL != "https://b.example" {
		t.Errorf("box title / B's own settings: %+v", st)
	}
	// with a second person on the box, the admin cannot download everybody's data either
	if c, _ := A.do("GET", "/api/v1/backup", ""); c != 403 {
		t.Errorf("admin backup on a shared box = %d, want 403", c)
	}

	// the newcomer from Core became a member with their own default trackers, not an admin
	nu, err := st.GetUserByEmail(ctx, "newcomer@example.com")
	if err != nil || nu.Role != "member" {
		t.Fatalf("newcomer: %v %+v", err, nu)
	}
	if trs, _ := st.ListTrackers(ctx, nu.ID, "", true); len(trs) != 3 {
		t.Errorf("newcomer has %d trackers, want the 3 defaults", len(trs))
	}

	// events name the person they are about
	time.Sleep(200 * time.Millisecond)
	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.events) == 0 {
		t.Fatal("no events published")
	}
	for _, ev := range core.events {
		p, _ := ev["payload"].(map[string]any)
		if p["user"] != "a@example.com" && p["user"] != "b@example.com" && p["user"] != "newcomer@example.com" {
			t.Errorf("event without its person: %v", ev)
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
