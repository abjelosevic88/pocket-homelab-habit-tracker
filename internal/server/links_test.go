package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/config"
	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/store"
)

// A linked app (Pocket Kitchen, Pocket Day) acting for B through Pocket Core
// sees and ticks only B's habits, only within the scopes Core granted, and
// nothing else in the app.
func TestCoreLinkScopes(t *testing.T) {
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

	a := &store.User{Email: "a@example.com", Name: "A", Role: "admin", PasswordHash: "x"}
	b := &store.User{Email: "b@example.com", Name: "B", Role: "member", PasswordHash: "x"}
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
	core.links["pcx_b_full"] = [3]string{"b@example.com", "B", "habits:read entries:write"}
	core.links["pcx_b_read"] = [3]string{"b@example.com", "B", "habits:read"}
	core.links["pcx_b_write"] = [3]string{"b@example.com", "B", "entries:write"}
	core.links["pcx_a_full"] = [3]string{"a@example.com", "A", "habits:read entries:write"}

	A := &client{t: t, base: ts.URL, hc: http.DefaultClient, tok: tokA}
	B := &client{t: t, base: ts.URL, hc: http.DefaultClient, tok: tokB}
	full := &client{t: t, base: ts.URL, hc: http.DefaultClient, tok: "pcx_b_full"}
	read := &client{t: t, base: ts.URL, hc: http.DefaultClient, tok: "pcx_b_read"}
	write := &client{t: t, base: ts.URL, hc: http.DefaultClient, tok: "pcx_b_write"}
	adminLink := &client{t: t, base: ts.URL, hc: http.DefaultClient, tok: "pcx_a_full"}
	today := store.Today()

	var ah, bh store.Habit
	if c := A.json("POST", "/api/v1/habits", `{"name":"Secret habit"}`, &ah); c != 201 {
		t.Fatalf("create A's habit %d", c)
	}
	if c := B.json("POST", "/api/v1/habits", `{"name":"Cooked at home"}`, &bh); c != 201 {
		t.Fatalf("create B's habit %d", c)
	}
	aid, bid := itoa(ah.ID), itoa(bh.ID)

	// habits:read — the list, one habit, a day; only B's
	for _, c := range []*client{full, read} {
		for _, path := range []string{"/api/v1/habits", "/api/v1/habits/" + bid, "/api/v1/days/" + today} {
			code, body := c.do("GET", path, "")
			if code != 200 || !strings.Contains(body, "Cooked at home") || strings.Contains(body, "Secret habit") {
				t.Errorf("%s GET %s = %d %s", c.tok, path, code, body)
			}
		}
		if code, _ := c.do("GET", "/api/v1/habits/"+aid, ""); code != 404 {
			t.Errorf("%s: A's habit through B's link = %d, want 404", c.tok, code)
		}
	}
	// auth/me answers any linked app, as the person it acts for
	for _, c := range []*client{full, read, write} {
		var u store.User
		if code := c.json("GET", "/api/v1/auth/me", "", &u); code != 200 || u.Email != "b@example.com" {
			t.Errorf("%s auth/me = %d %+v", c.tok, code, u)
		}
	}

	// entries:write — tick B's habit, then clear it
	for _, c := range []*client{full, write} {
		if code, body := c.do("PUT", "/api/v1/habits/"+bid+"/entries/"+today, `{"status":1}`); code != 200 {
			t.Fatalf("%s tick = %d %s", c.tok, code, body)
		}
		var day store.TodayPage
		B.json("GET", "/api/v1/days/"+today, "", &day)
		if len(day.Habits) != 1 || day.Habits[0].ID != bh.ID || day.Habits[0].Status != 1 {
			t.Fatalf("%s: B's day after the tick: %+v", c.tok, day.Habits)
		}
		if code, body := c.do("DELETE", "/api/v1/habits/"+bid+"/entries/"+today, ""); code != 204 {
			t.Fatalf("%s clear = %d %s", c.tok, code, body)
		}
		// someone else's habit is not there for B's link
		if code, _ := c.do("PUT", "/api/v1/habits/"+aid+"/entries/"+today, `{"status":1}`); code != 404 {
			t.Errorf("%s: ticked A's habit: %d", c.tok, code)
		}
	}
	var aday store.TodayPage
	A.json("GET", "/api/v1/days/"+today, "", &aday)
	if len(aday.Habits) != 1 || aday.Habits[0].Status != 0 {
		t.Errorf("A's day changed through B's link: %+v", aday.Habits)
	}

	// a token without the scope
	if code, _ := read.do("PUT", "/api/v1/habits/"+bid+"/entries/"+today, `{"status":1}`); code != 403 {
		t.Errorf("read-only link ticked a habit: %d", code)
	}
	if code, _ := read.do("DELETE", "/api/v1/habits/"+bid+"/entries/"+today, ""); code != 403 {
		t.Errorf("read-only link cleared a habit: %d", code)
	}
	for _, path := range []string{"/api/v1/habits", "/api/v1/habits/" + bid, "/api/v1/days/" + today} {
		if code, _ := write.do("GET", path, ""); code != 403 {
			t.Errorf("write-only link read %s: %d", path, code)
		}
	}

	// everything else is closed, even when the link acts for the admin
	for _, c := range []*client{full, adminLink} {
		for _, r := range []struct{ method, path, body string }{
			{"POST", "/api/v1/habits", `{"name":"x"}`},
			{"PUT", "/api/v1/habits/" + bid, `{"name":"x"}`},
			{"DELETE", "/api/v1/habits/" + bid, ""},
			{"POST", "/api/v1/habits/" + bid + "/end", `{}`},
			{"PUT", "/api/v1/habits/reorder", `{"ids":[]}`},
			{"PUT", "/api/v1/habits/" + bid + "/entries/today", `{"status":1}`},
			{"PUT", "/api/v1/auth/me", `{"name":"pwned"}`},
			{"POST", "/api/v1/auth/logout", ""},
			{"GET", "/api/v1/today", ""},
			{"GET", "/api/v1/months/current", ""},
			{"POST", "/api/v1/checkin", `{"habits":{}}`},
			{"GET", "/api/v1/dashboard", ""},
			{"GET", "/api/v1/insights", ""},
			{"GET", "/api/v1/trackers", ""},
			{"PUT", "/api/v1/trackers/1/entries/" + today, `{"value":1}`},
			{"GET", "/api/v1/journal", ""},
			{"PUT", "/api/v1/journal/" + today, `{"text":"x"}`},
			{"GET", "/api/v1/weather", ""},
			{"GET", "/api/v1/location", ""},
			{"GET", "/api/v1/settings", ""},
			{"PUT", "/api/v1/settings", `{"title":"x"}`},
			{"GET", "/api/v1/settings/system", ""},
			{"GET", "/api/v1/tokens", ""},
			{"POST", "/api/v1/tokens", `{"name":"x"}`},
			{"GET", "/api/v1/backup", ""},
			{"GET", "/api/v1/export.json", ""},
			{"GET", "/api/v1/export.csv", ""},
			{"POST", "/api/v1/import", `{}`},
		} {
			if code, body := c.do(r.method, r.path, r.body); code != 403 || !strings.Contains(body, "link does not cover") {
				t.Errorf("%s %s %s = %d %s, want 403", c.tok, r.method, r.path, code, body)
			}
		}
	}
	// a link token Core no longer vouches for is just unauthorized
	if code, _ := (&client{t: t, base: ts.URL, hc: http.DefaultClient, tok: "pcx_revoked"}).do("GET", "/api/v1/habits", ""); code != 401 {
		t.Errorf("unknown link token = %d, want 401", code)
	}
	// A's own name is untouched
	if u, _ := st.GetUserByEmail(ctx, "a@example.com"); u.Name != "A" {
		t.Errorf("A renamed through a link: %+v", u)
	}

	// the ticks made through the link name the acting app in their events
	time.Sleep(200 * time.Millisecond)
	core.mu.Lock()
	defer core.mu.Unlock()
	via := 0
	for _, ev := range core.events {
		p, _ := ev["payload"].(map[string]any)
		if p["via"] == "kitchen" {
			via++
			if p["user"] != "b@example.com" {
				t.Errorf("linked event for the wrong person: %v", ev)
			}
		}
	}
	if via != 4 {
		t.Errorf("%d events name the acting app, want 4 (two ticks, two clears)", via)
	}
}
