package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/config"
	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/store"
)

// capBox is a box with A (admin) and B (member), each with an API token. With
// a fake Core it also knows the link tokens pcx_<a|b>_<search|day|widget|
// capture|read|rw>; without one (fc nil) the app runs on its own.
func capBox(t *testing.T, fc *fakeCore) (srv *Server, A, B *client, link func(tok string) *client) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{AuthMode: "local", SessionTTL: time.Hour, DataDir: dir, DBPath: dir + "/t.db"}
	if fc != nil {
		cfg.CoreURL = fc.URL
		for _, who := range []string{"a", "b"} {
			for name, scopes := range map[string]string{"search": core.ScopeSearch, "day": core.ScopeDay, "widget": core.ScopeWidget, "capture": core.ScopeCapture,
				"read": "habits:read", "rw": "habits:read entries:write"} {
				fc.links["pcx_"+who+"_"+name] = [3]string{who + "@example.com", strings.ToUpper(who), scopes}
			}
		}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv = New(cfg, st, log, nil)
	if err := srv.core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
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
	link = func(tok string) *client { return &client{t: t, base: ts.URL, hc: http.DefaultClient, tok: tok} }
	return srv, link(tokA), link(tokB), link
}

// fetch reads a route's JSON answer into out, emptied first so nothing of an
// earlier answer stays.
func fetch[T any](c *client, path string, out *T) int {
	var empty T
	*out = empty
	return c.json("GET", path, "", out)
}

// mkHabit adds a habit (body is the JSON after the name) and returns its id.
func mkHabit(t *testing.T, c *client, name, rest string) int64 {
	t.Helper()
	var h store.Habit
	if code := c.json("POST", "/api/v1/habits", `{"name":"`+name+`"`+rest+`}`, &h); code != 201 {
		t.Fatalf("create habit %s: %d", name, code)
	}
	return h.ID
}

func mark(t *testing.T, c *client, id int64, date string, status int) {
	t.Helper()
	if code, body := c.do("PUT", "/api/v1/habits/"+itoa(id)+"/entries/"+date, `{"status":`+strconv.Itoa(status)+`}`); code != 200 {
		t.Fatalf("mark %d on %s: %d %s", id, date, code, body)
	}
}

func writeJournal(t *testing.T, c *client, date, body string) {
	t.Helper()
	if code, out := c.do("PUT", "/api/v1/journal/"+date, body); code != 200 {
		t.Fatalf("journal %s: %d %s", date, code, out)
	}
}

func journalOf(t *testing.T, c *client, date string) store.TodayPage {
	t.Helper()
	var day store.TodayPage
	if code := c.json("GET", "/api/v1/days/"+date, "", &day); code != 200 {
		t.Fatalf("day %s: %d", date, code)
	}
	return day
}

func TestPocketSearch(t *testing.T) {
	_, A, B, link := capBox(t, nil)
	run := mkHabit(t, A, "Morning run", `,"days":"135","start_date":"2026-10-01"`)
	ended := mkHabit(t, A, "Run to work", `,"start_date":"2026-09-01","end_date":"2026-09-30"`)
	book := mkHabit(t, A, "Read a book", "")
	mkHabit(t, B, "Run club", "")
	writeJournal(t, A, "2026-11-12", `{"text":"Coffee with Ana.\nLong run by the river.","bad":"Lost my keys.","important":"Dentist at nine."}`)
	writeJournal(t, A, "2026-11-10", `{"bad":"Skipped the run, 100% my fault."}`)
	writeJournal(t, A, "2026-11-08", `{"text":"Šetnja pored reke."}`)
	writeJournal(t, B, "2026-11-12", `{"text":"B's own run."}`)

	var got core.SearchResults
	if code := fetch(A, "/api/v1/pocket/search?q=run", &got); code != 200 || len(got.Items) != 4 {
		t.Fatalf("search: %d %+v", code, got)
	}
	// habits first, the ones still running before the ended one, then the journal, newest day first
	want := []core.SearchItem{
		{ID: "habit:" + itoa(run), Title: "Morning run", Snippet: "Mon Wed Fri · since 1 Oct 2026", Kind: "habit", Date: "2026-10-01", URL: "/manage?habit=" + itoa(run)},
		{ID: "habit:" + itoa(ended), Title: "Run to work", Snippet: "Every day · since 1 Sep 2026 · until 30 Sep 2026", Kind: "habit", Date: "2026-09-01", URL: "/manage?habit=" + itoa(ended)},
		{ID: "journal:2026-11-12", Title: "Journal, 12 Nov 2026", Snippet: "Long run by the river.", Kind: "journal", Date: "2026-11-12", URL: "/today/2026-11-12"},
		{ID: "journal:2026-11-10", Title: "Journal, 10 Nov 2026", Snippet: "Skipped the run, 100% my fault.", Kind: "journal", Date: "2026-11-10", URL: "/today/2026-11-10"},
	}
	for i, w := range want {
		if got.Items[i] != w {
			t.Errorf("item %d: %+v\n want %+v", i, got.Items[i], w)
		}
	}

	for q, id := range map[string]string{
		"keys":          "journal:2026-11-12", // the bad line
		"dentist":       "journal:2026-11-12", // the important note
		"river+coffee":  "journal:2026-11-12", // every word, in any order, on any line of the day
		"100%25":        "journal:2026-11-10", // a % is a % and not "anything"
		"%C5%A1etnja":   "journal:2026-11-08", // šetnja finds Šetnja
		"%C5%A0ETNJA":   "journal:2026-11-08",
		"book":          "habit:" + itoa(book),
		"MORNING":       "habit:" + itoa(run),
		"run%20morning": "habit:" + itoa(run),
	} {
		if fetch(A, "/api/v1/pocket/search?q="+q, &got); len(got.Items) != 1 || got.Items[0].ID != id {
			t.Errorf("q=%s: %+v, want only %s", q, got.Items, id)
		}
	}
	if fetch(A, "/api/v1/pocket/search?q=keys", &got); got.Items[0].Snippet != "Lost my keys." {
		t.Errorf("snippet is not the matching line: %+v", got.Items[0])
	}
	for _, q := range []string{"_", "river%20dentist%20nothing", "zzz"} {
		if fetch(A, "/api/v1/pocket/search?q="+q, &got); len(got.Items) != 0 {
			t.Errorf("q=%s found %+v", q, got.Items)
		}
	}
	if fetch(A, "/api/v1/pocket/search?q=run&limit=3", &got); len(got.Items) != 3 || got.Items[2].Kind != "journal" {
		t.Errorf("limit=3: %+v", got.Items)
	}
	if code := fetch(A, "/api/v1/pocket/search?q=", &got); code != 200 || got.Items == nil || len(got.Items) != 0 {
		t.Errorf("no words: %d %+v", code, got)
	}
	// strictly per person
	if fetch(B, "/api/v1/pocket/search?q=run", &got); len(got.Items) != 2 || got.Items[0].Title != "Run club" || got.Items[1].Snippet != "B's own run." {
		t.Errorf("B finds: %+v", got.Items)
	}
	if code, _ := link("").do("GET", "/api/v1/pocket/search?q=run", ""); code != 401 {
		t.Errorf("signed out: %d", code)
	}
}

// A long journal line keeps the words that were found after it is cut to length.
func TestMatchingLine(t *testing.T) {
	long := strings.Repeat("word ", 80) + "and then the Ferry left without us"
	got := matchingLine("ferry", "Short one.", long)
	if !strings.HasPrefix(got, "…") || !strings.Contains(got, "Ferry left") || len([]rune(got)) > 300 {
		t.Errorf("long line: %q", got)
	}
	if got := matchingLine("nothing here", "", "First line.\nSecond line."); got != "First line." {
		t.Errorf("no word found: %q", got)
	}
}

func TestPocketDay(t *testing.T) {
	_, A, B, _ := capBox(t, nil)
	const date = "2026-11-12" // a Thursday
	run := mkHabit(t, A, "Run", `,"start_date":"2026-11-01"`)
	read := mkHabit(t, A, "Read", `,"start_date":"2026-11-01"`)
	stretch := mkHabit(t, A, "Stretch", `,"days":"4","start_date":"2026-11-01"`)
	swim := mkHabit(t, A, "Swim", `,"days":"6","start_date":"2026-11-01"`)
	mkHabit(t, A, "Started later", `,"start_date":"2026-11-20"`)
	for _, d := range []string{"2026-11-10", "2026-11-11", date} {
		mark(t, A, run, d, 1)
	}
	mark(t, A, read, date, -1)
	mark(t, A, swim, date, 1) // not planned on a Thursday: as on the check-in page, it does not count
	writeJournal(t, A, date, `{"text":"Long walk by the river.","bad":"Lost my keys.","important":"Dentist at nine."}`)
	theirs := mkHabit(t, B, "B's habit", `,"start_date":"2026-11-01"`)
	mark(t, B, theirs, date, 1)

	var day core.Day
	if code := fetch(A, "/api/v1/pocket/day/"+date+"?tz=Europe/Sarajevo", &day); code != 200 {
		t.Fatalf("day: %d", code)
	}
	if day.Date != date || day.Summary != "1 of 3 habits done, journal written, an important note" {
		t.Errorf("day: %+v", day)
	}
	want := []core.DayItem{
		{ID: "habit:" + itoa(run), Title: "Run", Sub: "3-day streak", Kind: "done", Done: true, URL: "/today/" + date},
		{ID: "habit:" + itoa(read), Title: "Read", Kind: "missed", URL: "/today/" + date},
		{ID: "habit:" + itoa(stretch), Title: "Stretch", Kind: "open", URL: "/today/" + date},
		{ID: "journal:" + date, Title: "What was great about today?", Kind: "journal", Note: "Long walk by the river.", URL: "/today/" + date},
		{ID: "journal-bad:" + date, Title: "And one thing that was bad?", Kind: "journal", Note: "Lost my keys.", URL: "/today/" + date},
		{ID: "important:" + date, Title: "Important", Kind: "journal", Note: "Dentist at nine.", URL: "/today/" + date},
	}
	if len(day.Items) != len(want) {
		t.Fatalf("items: %+v", day.Items)
	}
	for i, w := range want {
		if day.Items[i] != w {
			t.Errorf("item %d: %+v\n want %+v", i, day.Items[i], w)
		}
	}
	if len(day.Metrics) != 1 || day.Metrics[0] != (core.Metric{Label: "Done", Value: "1 of 3"}) {
		t.Errorf("metrics: %+v", day.Metrics)
	}
	// the numbers are the ones GET /days/{date} gives
	if page := journalOf(t, A, date); page.Done != 1 || page.Total != 3 {
		t.Errorf("/days/%s: done %d of %d", date, page.Done, page.Total)
	}

	// one habit planned, none done; nothing written
	fetch(A, "/api/v1/pocket/day/2026-11-14", &day) // a Saturday: Run, Read and Swim
	if day.Summary != "0 of 3 habits done" || len(day.Items) != 3 || day.Items[2].Title != "Swim" || day.Items[2].Kind != "open" {
		t.Errorf("a day with nothing marked: %+v", day)
	}
	fetch(B, "/api/v1/pocket/day/"+date, &day)
	if day.Summary != "1 of 1 habit done" || len(day.Items) != 1 || day.Items[0].Title != "B's habit" || day.Items[0].Sub != "" {
		t.Errorf("B's day: %+v", day)
	}
	// only a note for a day ahead: nothing is "done" there yet
	writeJournal(t, B, "2999-01-02", `{"important":"Move house."}`)
	fetch(B, "/api/v1/pocket/day/2999-01-02", &day)
	if day.Summary != "1 habit planned, an important note" || len(day.Items) != 2 || day.Items[0].Kind != "open" || day.Items[1].Note != "Move house." || len(day.Metrics) != 0 {
		t.Errorf("a day to come: %+v", day)
	}
	// before any habit started and with nothing written: nothing
	if code := fetch(A, "/api/v1/pocket/day/2026-01-05", &day); code != 200 || day.Summary != "" || day.Items == nil || len(day.Items) != 0 {
		t.Errorf("a day with nothing: %d %+v", code, day)
	}
	// the journal alone
	writeJournal(t, A, "2026-01-05", `{"text":"New year, new notebook."}`)
	if fetch(A, "/api/v1/pocket/day/2026-01-05", &day); day.Summary != "Journal written" || len(day.Items) != 1 || day.Items[0].Kind != "journal" {
		t.Errorf("only the journal: %+v", day)
	}
	// the person's own heading names the journal line
	if code, body := A.do("PUT", "/api/v1/settings", `{"journal_prompt":"Best of the day"}`); code != 200 {
		t.Fatalf("settings: %d %s", code, body)
	}
	if fetch(A, "/api/v1/pocket/day/2026-01-05", &day); day.Items[0].Title != "Best of the day" {
		t.Errorf("heading: %+v", day.Items)
	}
	if code, _ := A.do("GET", "/api/v1/pocket/day/yesterday", ""); code != 400 {
		t.Errorf("not a day: %d", code)
	}
}

func TestPocketWidget(t *testing.T) {
	_, A, B, _ := capBox(t, nil)
	today := store.Today()
	now, _ := store.ParseDate(today)
	yesterday, before := now.AddDate(0, 0, -1).Format("2006-01-02"), now.AddDate(0, 0, -2).Format("2006-01-02")
	wd := int(now.Weekday())
	if wd == 0 {
		wd = 7
	}
	other := strconv.Itoa(wd%7 + 1) // a weekday that is not today

	// nobody has habits yet: the two numbers, no line, no rows
	var got core.WidgetData
	if code := fetch(A, "/api/v1/pocket/widget", &got); code != 200 || got.Line != "" || got.URL != "/today" || got.Items == nil || len(got.Items) != 0 {
		t.Fatalf("nothing yet: %d %+v", code, got)
	}

	streak := mkHabit(t, A, "Run", `,"start_date":"`+before+`"`)
	done := mkHabit(t, A, "Read", "")
	missed := mkHabit(t, A, "Stretch", "")
	open := mkHabit(t, A, "Floss", "")
	mkHabit(t, A, "Another day", `,"days":"`+other+`"`)
	mark(t, A, streak, before, 1)
	mark(t, A, streak, yesterday, 1)
	mark(t, A, done, today, 1)
	mark(t, A, missed, today, -1)
	mkHabit(t, B, "B's habit", "")

	fetch(A, "/api/v1/pocket/widget", &got)
	if got.Line != "2 left today" || got.URL != "/today" {
		t.Errorf("line: %+v", got)
	}
	// the ones still to mark first (with the run they keep going), then the missed one, then the done one
	want := []core.WidgetItem{
		{ID: itoa(streak), Title: "Run", Detail: "2-day streak", URL: "/today"},
		{ID: itoa(open), Title: "Floss", URL: "/today"},
		{ID: itoa(missed), Title: "Stretch", Detail: "Missed", URL: "/today"},
		{ID: itoa(done), Title: "Read", Detail: "Done", Done: true, URL: "/today"},
	}
	if len(got.Items) != len(want) {
		t.Fatalf("rows: %+v", got.Items)
	}
	for i, w := range want {
		if got.Items[i] != w {
			t.Errorf("row %d: %+v\n want %+v", i, got.Items[i], w)
		}
	}

	mark(t, A, streak, today, 1)
	mark(t, A, open, today, 1)
	fetch(A, "/api/v1/pocket/widget", &got)
	if got.Line != "3 of 4 done today" || got.Items[0].Title != "Stretch" || got.Items[1] != (core.WidgetItem{ID: itoa(streak), Title: "Run", Detail: "3-day streak", Done: true, URL: "/today"}) {
		t.Errorf("all marked, one missed: %+v", got)
	}
	mark(t, A, missed, today, 1)
	if fetch(A, "/api/v1/pocket/widget", &got); got.Line != "All done today" || len(got.Items) != 4 || !got.Items[3].Done {
		t.Errorf("all done: %+v", got)
	}

	// never more than eight rows, and the ones still to do are the ones kept
	for i := 0; i < 9; i++ {
		mkHabit(t, A, "More "+strconv.Itoa(i), "")
	}
	if fetch(A, "/api/v1/pocket/widget", &got); got.Line != "9 left today" || len(got.Items) != 8 || got.Items[0].Title != "More 0" || got.Items[7].Done {
		t.Errorf("a long day: %q %+v", got.Line, got.Items)
	}

	// strictly per person
	if fetch(B, "/api/v1/pocket/widget", &got); got.Line != "1 left today" || len(got.Items) != 1 || got.Items[0].Title != "B's habit" {
		t.Errorf("B's widget: %+v", got)
	}
	// habits, but none planned today
	if code, body := B.do("PUT", "/api/v1/habits/"+got.Items[0].ID, `{"name":"B's habit","days":"`+other+`"}`); code != 200 {
		t.Fatalf("reschedule: %d %s", code, body)
	}
	if fetch(B, "/api/v1/pocket/widget", &got); got.Line != "Nothing planned today" || len(got.Items) != 0 {
		t.Errorf("nothing planned: %+v", got)
	}
}

// Pocket Hub's phone apps draw the habit ring from the widget's first two
// numbers, found by these names: "Done" and "Scheduled", whole numbers, the
// same done and total GET /days/{date} gives (what Hub read before).
func TestPocketWidgetRingNumbers(t *testing.T) {
	_, A, B, _ := capBox(t, nil)
	today := store.Today()
	check := func(c *client, done, total int) {
		t.Helper()
		var got core.WidgetData
		if code := fetch(c, "/api/v1/pocket/widget", &got); code != 200 {
			t.Fatalf("widget: %d", code)
		}
		if len(got.Metrics) < 2 || got.Metrics[0].Label != "Done" || got.Metrics[1].Label != "Scheduled" {
			t.Fatalf("the first two numbers must be Done and Scheduled: %+v", got.Metrics)
		}
		page := journalOf(t, c, today)
		for i, want := range []int{page.Done, page.Total} {
			n, err := strconv.Atoi(got.Metrics[i].Value)
			if err != nil || got.Metrics[i].Value != strconv.Itoa(n) {
				t.Errorf("%s is not a whole number: %q", got.Metrics[i].Label, got.Metrics[i].Value)
			}
			if n != want {
				t.Errorf("%s = %d, /days/%s says %d", got.Metrics[i].Label, n, today, want)
			}
		}
		if page.Done != done || page.Total != total {
			t.Errorf("done %d of %d, want %d of %d", page.Done, page.Total, done, total)
		}
	}
	check(A, 0, 0) // no habits yet: still both numbers
	ids := []int64{}
	for _, name := range []string{"Run", "Read", "Stretch"} {
		ids = append(ids, mkHabit(t, A, name, ""))
	}
	check(A, 0, 3)
	mark(t, A, ids[0], today, 1)
	mark(t, A, ids[1], today, -1)
	check(A, 1, 3)
	mark(t, A, ids[1], today, 1)
	mark(t, A, ids[2], today, 1)
	check(A, 3, 3)
	check(B, 0, 0)
}

func capture(c *client, body string) (int, core.CaptureResult, string) {
	code, out := c.do("POST", "/api/v1/pocket/capture", body)
	var res core.CaptureResult
	_ = json.Unmarshal([]byte(out), &res)
	return code, res, out
}

func TestPocketCapture(t *testing.T) {
	_, A, B, _ := capBox(t, nil)
	today := store.Today()

	// an empty day: the line is the day's journal
	line := `{"request_id":"hub-0000000000000001","kind":"journal","text":"  Long walk by the river.  "}`
	code, first, body := capture(A, line)
	if code != 201 || first != (core.CaptureResult{ID: today, Title: "Long walk by the river.", URL: "/today/" + today}) {
		t.Fatalf("capture: %d %s", code, body)
	}
	if day := journalOf(t, A, today); day.Journal != "Long walk by the river." {
		t.Errorf("journal: %q", day.Journal)
	}
	// the same request again answers the same and adds nothing
	code, again, body := capture(A, line)
	if code != 201 || again != first {
		t.Errorf("same request_id: %d %s", code, body)
	}
	// …and it is not a way to keep something else
	if code, _, body := capture(A, `{"request_id":"hub-0000000000000001","text":"Something else"}`); code != 400 || !strings.Contains(body, "already kept") {
		t.Errorf("same request_id, other words: %d %s", code, body)
	}
	if day := journalOf(t, A, today); day.Journal != "Long walk by the river." {
		t.Errorf("journal after the repeats: %q", day.Journal)
	}

	// a day that already has text: the line goes under it, nothing is replaced
	writeJournal(t, A, today, `{"bad":"Lost my keys.","important":"Dentist at nine."}`)
	code, second, body := capture(A, `{"request_id":"hub-0000000000000002","text":"Great dinner"}`)
	if code != 201 || second.ID != today || second.Title != "Great dinner" {
		t.Fatalf("second line: %d %s", code, body)
	}
	capture(A, `{"request_id":"hub-0000000000000002","text":"Great dinner"}`)
	day := journalOf(t, A, today)
	if day.Journal != "Long walk by the river.\nGreat dinner" || day.JournalBad != "Lost my keys." || day.Important != "Dentist at nine." {
		t.Errorf("the day after a second line: %q / %q / %q", day.Journal, day.JournalBad, day.Important)
	}
	// the first request, sent once more after others, still adds nothing
	if code, again, _ := capture(A, line); code != 201 || again != first || journalOf(t, A, today).Journal != day.Journal {
		t.Errorf("a late repeat changed the journal: %d %q", code, journalOf(t, A, today).Journal)
	}

	// another day, and what the app's own writes do there stays the app's
	writeJournal(t, A, "2026-11-12", `{"text":"Typed in the app."}`)
	code, past, body := capture(A, `{"request_id":"hub-0000000000000003","text":"Remembered later","date":"2026-11-12"}`)
	if code != 201 || past != (core.CaptureResult{ID: "2026-11-12", Title: "Remembered later", URL: "/today/2026-11-12"}) {
		t.Fatalf("another day: %d %s", code, body)
	}
	if d := journalOf(t, A, "2026-11-12"); d.Journal != "Typed in the app.\nRemembered later" {
		t.Errorf("another day's journal: %q", d.Journal)
	}
	if journalOf(t, A, today).Journal != day.Journal {
		t.Errorf("today changed by a line for another day")
	}

	// another person's same request_id is their own line, in their own journal
	code, his, _ := capture(B, line)
	if code != 201 || his != first || journalOf(t, B, today).Journal != "Long walk by the river." {
		t.Errorf("B's capture: %d %+v", code, his)
	}
	if journalOf(t, A, today).Journal != day.Journal {
		t.Errorf("A's journal changed by B's line")
	}

	// several lines stay several lines; the answer names them on one
	_, lines, _ := capture(B, `{"request_id":"hub-0000000000000004","text":"Pasta\r\nand a film"}`)
	if lines.Title != "Pasta and a film" || journalOf(t, B, today).Journal != "Long walk by the river.\nPasta\nand a film" {
		t.Errorf("several lines: %+v %q", lines, journalOf(t, B, today).Journal)
	}

	// what cannot be kept says why, and keeps nothing
	full := strings.Repeat("x", store.JournalMax-10)
	writeJournal(t, B, "2026-11-11", `{"text":"`+full+`"}`)
	for name, bad := range map[string][2]string{
		"a day to come":    {`{"request_id":"hub-0000000000000005","text":"Tomorrow's news","date":"2999-01-02"}`, "today or a day that has passed"},
		"too long":         {`{"request_id":"hub-0000000000000005","text":"` + strings.Repeat("x", store.JournalMax+1) + `"}`, "under 4000 characters"},
		"a full day":       {`{"request_id":"hub-0000000000000005","text":"One more thing","date":"2026-11-11"}`, "journal is full"},
		"nothing to keep":  {`{"request_id":"hub-0000000000000005","kind":"journal"}`, "write something"},
		"only spaces":      {`{"request_id":"hub-0000000000000005","text":"   "}`, "write something"},
		"no request_id":    {`{"text":"Long walk"}`, "request_id"},
		"unknown kind":     {`{"request_id":"hub-0000000000000005","kind":"habit","text":"Long walk"}`, "does not keep"},
		"not a day":        {`{"request_id":"hub-0000000000000005","text":"Long walk","date":"yesterday"}`, "YYYY-MM-DD"},
		"not json":         {`nope`, "invalid json"},
		"a habit to check": {`{"request_id":"hub-0000000000000005","kind":"checkin","text":"Run"}`, "does not keep"},
	} {
		if code, _, body := capture(B, bad[0]); code != 400 || !strings.Contains(body, bad[1]) {
			t.Errorf("%s: %d %s", name, code, body)
		}
	}
	if j := journalOf(t, B, today).Journal; j != "Long walk by the river.\nPasta\nand a film" {
		t.Errorf("B's journal after the refusals: %q", j)
	}
	if j := journalOf(t, B, "2026-11-11").Journal; j != full {
		t.Errorf("the full day changed: %d characters", len(j))
	}
	// a refused request_id is free to be used for the line that fits
	if code, _, body := capture(B, `{"request_id":"hub-0000000000000005","text":"Short","date":"2026-11-11"}`); code != 201 {
		t.Errorf("after a refusal: %d %s", code, body)
	}
}

// The same line sent several times at once (a retry racing the first try)
// is added once, and every answer is the same.
func TestPocketCaptureAtOnce(t *testing.T) {
	_, A, _, _ := capBox(t, nil)
	today := store.Today()
	writeJournal(t, A, today, `{"text":"Already here."}`)
	const n = 8
	answers := make(chan string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, res, body := capture(A, `{"request_id":"hub-000000000000000a","text":"Only once"}`)
			if code != 201 {
				res.Title = "HTTP " + strconv.Itoa(code) + " " + body
			}
			answers <- res.ID + " " + res.Title
		}()
	}
	wg.Wait()
	close(answers)
	seen := map[string]int{}
	for a := range answers {
		seen[a]++
	}
	if len(seen) != 1 || seen[today+" Only once"] != n {
		t.Errorf("answers: %v", seen)
	}
	if j := journalOf(t, A, today).Journal; j != "Already here.\nOnly once" {
		t.Errorf("journal: %q", j)
	}

	// different lines at the same moment all stay
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			capture(A, `{"request_id":"hub-00000000000000b`+strconv.Itoa(i)+`","text":"Line `+strconv.Itoa(i)+`"}`)
		}()
	}
	wg.Wait()
	j := journalOf(t, A, today).Journal
	if got := strings.Split(j, "\n"); len(got) != 2+n || got[0] != "Already here." {
		t.Errorf("journal after %d lines at once: %q", n, j)
	}
	for i := 0; i < n; i++ {
		if strings.Count(j, "Line "+strconv.Itoa(i)) != 1 {
			t.Errorf("line %d is there %d times: %q", i, strings.Count(j, "Line "+strconv.Itoa(i)), j)
		}
	}
}

func TestPocketLinkTokens(t *testing.T) {
	fc := newFakeCore(t)
	srv, A, B, link := capBox(t, fc)
	today := store.Today()
	mine := mkHabit(t, B, "Bread baking", "")
	mark(t, B, mine, today, 1)
	writeJournal(t, B, today, `{"text":"Fresh bread for breakfast."}`)
	mkHabit(t, A, "Secret bread", "")
	writeJournal(t, A, today, `{"text":"Secret bread note."}`)

	line := `{"request_id":"hub-0000000000000001","text":"Kept through a link"}`
	routes := []struct {
		scope, method, path, body string
		ok                        int
	}{
		{"search", "GET", "/api/v1/pocket/search?q=bread", "", 200},
		{"day", "GET", "/api/v1/pocket/day/" + today + "?tz=Europe/Sarajevo", "", 200},
		{"widget", "GET", "/api/v1/pocket/widget", "", 200},
		{"capture", "POST", "/api/v1/pocket/capture", line, 201},
	}
	for _, tok := range []string{"search", "day", "widget", "capture", "read", "rw"} {
		c := link("pcx_b_" + tok)
		for _, rt := range routes {
			want := 403
			if rt.scope == tok {
				want = rt.ok
			}
			code, body := c.do(rt.method, rt.path, rt.body)
			if code != want {
				t.Errorf("pcx_b_%s %s %s: %d, want %d", tok, rt.method, rt.path, code, want)
			}
			if strings.Contains(body, "Secret") {
				t.Errorf("pcx_b_%s %s: another person's things: %s", tok, rt.path, body)
			}
		}
		var u store.User
		if code := c.json("GET", "/api/v1/auth/me", "", &u); code != 200 || u.Email != "b@example.com" {
			t.Errorf("pcx_b_%s /auth/me: %d %+v", tok, code, u)
		}
		if tok == "read" || tok == "rw" {
			continue
		}
		// a capability opens its one route with its one method, nothing else of the app
		for _, rt := range []struct{ method, path, body string }{
			{"GET", "/api/v1/habits", ""}, {"GET", "/api/v1/days/" + today, ""}, {"GET", "/api/v1/today", ""}, {"GET", "/api/v1/dashboard", ""},
			{"GET", "/api/v1/journal", ""}, {"PUT", "/api/v1/journal/" + today, `{"text":"x"}`}, {"POST", "/api/v1/checkin", `{"journal":"x"}`},
			{"PUT", "/api/v1/habits/" + itoa(mine) + "/entries/" + today, `{"status":-1}`}, {"POST", "/api/v1/habits", `{"name":"x"}`},
			{"GET", "/api/v1/export.json", ""}, {"GET", "/api/v1/pocket/portable", ""},
		} {
			if code, _ := c.do(rt.method, rt.path, rt.body); code != 403 {
				t.Errorf("pcx_b_%s %s %s: %d, want 403", tok, rt.method, rt.path, code)
			}
		}
	}
	// the other method on a capability's own route is no way in either
	for _, rt := range []struct{ tok, method, path string }{{"search", "POST", "/api/v1/pocket/search?q=bread"}, {"capture", "GET", "/api/v1/pocket/capture"},
		{"widget", "DELETE", "/api/v1/pocket/widget"}, {"day", "PUT", "/api/v1/pocket/day/" + today}} {
		if code, _ := link("pcx_b_"+rt.tok).do(rt.method, rt.path, ""); code != 403 && code != 405 {
			t.Errorf("pcx_b_%s %s %s: %d", rt.tok, rt.method, rt.path, code)
		}
	}

	// what came back is B's, and only B's
	var found core.SearchResults
	fetch(link("pcx_b_search"), "/api/v1/pocket/search?q=bread", &found)
	if len(found.Items) != 2 || found.Items[0].Title != "Bread baking" || found.Items[1].Snippet != "Fresh bread for breakfast." {
		t.Errorf("link search: %+v", found.Items)
	}
	var day core.Day
	fetch(link("pcx_b_day"), "/api/v1/pocket/day/"+today, &day)
	if day.Summary != "1 of 1 habit done, journal written" || len(day.Items) != 2 || day.Items[0].Title != "Bread baking" {
		t.Errorf("link day: %+v", day)
	}
	var wd core.WidgetData
	fetch(link("pcx_b_widget"), "/api/v1/pocket/widget", &wd)
	if wd.Line != "All done today" || len(wd.Items) != 1 || wd.Items[0].Title != "Bread baking" || wd.Metrics[0].Value != "1" || wd.Metrics[1].Value != "1" {
		t.Errorf("link widget: %+v", wd)
	}

	// the same capture again through the link: still one line, and it is in B's journal
	hub := link("pcx_b_capture")
	code, first, body := capture(hub, line)
	code2, again, _ := capture(hub, line)
	if code != 201 || code2 != 201 || first.ID != today || first != again {
		t.Fatalf("link capture twice: %d %d %s", code, code2, body)
	}
	if j := journalOf(t, B, today).Journal; j != "Fresh bread for breakfast.\nKept through a link" {
		t.Errorf("B's journal: %q", j)
	}
	if j := journalOf(t, A, today).Journal; j != "Secret bread note." {
		t.Errorf("A's journal changed through B's link: %q", j)
	}
	if code, _, body := capture(hub, `{"request_id":"hub-0000000000000009"}`); code != 400 {
		t.Errorf("link capture with nothing: %d %s", code, body)
	}

	// the line is published once, like a check-in, and names the app that sent it
	deadline := time.Now().Add(3 * time.Second)
	for {
		status, err := srv.core.OutboxStatus(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if status.Pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("outbox did not drain", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	sent := 0
	for _, ev := range fc.events {
		p, _ := ev["payload"].(map[string]any)
		if ev["topic"] == "habits.checkin" && p["via"] == "kitchen" {
			sent++
			if p["user"] != "b@example.com" || p["date"] != today || p["journal"] != true || ev["subject"] != "day:"+today {
				t.Errorf("the capture's event: %v", ev)
			}
		}
	}
	if sent != 1 {
		t.Errorf("%d events for the captured line, want 1", sent)
	}
}

// The manifest Core gets says what Habits can do, so nobody has to list it.
func TestManifestOffersCapabilities(t *testing.T) {
	srv, _, _, _ := capBox(t, nil)
	b, err := json.Marshal(srv.manifest())
	if err != nil {
		t.Fatal(err)
	}
	var sent map[string]any
	_ = json.Unmarshal(b, &sent)
	if sent["slug"] != "habits" || sent["category"] != "Everyday" {
		t.Errorf("manifest: slug %v, category %v", sent["slug"], sent["category"])
	}
	for _, k := range []string{"search", "day", "widget", "capture"} {
		if sent[k] == nil {
			t.Fatalf("manifest has no %q", k)
		}
	}
	if bangs, _ := json.Marshal(sent["search"].(map[string]any)["bangs"]); string(bangs) != `["habits","habit"]` {
		t.Errorf("bangs: %s", bangs)
	}
	if w := sent["widget"].(map[string]any); w["title"] != "Habits today" {
		t.Errorf("widget: %v", w)
	}
	kinds, _ := sent["capture"].(map[string]any)["kinds"].([]any)
	if len(kinds) != 1 || kinds[0].(map[string]any)["kind"] != "journal" {
		t.Errorf("capture kinds: %v", sent["capture"])
	}
	// the links other apps already use are still offered, and Register adds one offer per capability
	offered := map[string]bool{}
	for _, o := range append(append([]core.Offer{}, linkOffers...), capabilities.Offers()...) {
		offered[o.Scope] = true
	}
	for _, sc := range []string{"habits:read", "entries:write", core.ScopeSearch, core.ScopeDay, core.ScopeWidget, core.ScopeCapture} {
		if !offered[sc] {
			t.Errorf("no offer for %s: %v", sc, offered)
		}
	}
}
