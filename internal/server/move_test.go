package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/config"
	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/store"
)

// moveBox is a box with A (admin) and B (member) and Pocket Move links
// pcx_<a|b>_<export|import|read> acting for them.
func moveBox(t *testing.T) (A, B *client, link func(tok string) *client) {
	t.Helper()
	fc := newFakeCore(t)
	dir := t.TempDir()
	cfg := &config.Config{AuthMode: "local", SessionTTL: time.Hour, DataDir: dir, DBPath: dir + "/t.db", CoreURL: fc.URL}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv := New(cfg, st, log, nil)
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
	for _, who := range []string{"a", "b"} {
		fc.links["pcx_"+who+"_export"] = [3]string{who + "@example.com", who, core.ScopeMoveExport}
		fc.links["pcx_"+who+"_import"] = [3]string{who + "@example.com", who, core.ScopeMoveImport}
		fc.links["pcx_"+who+"_read"] = [3]string{who + "@example.com", who, "habits:read"}
	}
	mk := func(tok string) *client { return &client{t: t, base: ts.URL, hc: http.DefaultClient, tok: tok} }
	return mk(tokA), mk(tokB), mk
}

func TestMoveLinkScopes(t *testing.T) {
	_, _, link := moveBox(t)
	exp, imp, read := link("pcx_b_export"), link("pcx_b_import"), link("pcx_b_read")
	for _, c := range []struct {
		c            *client
		method, path string
		want         int
	}{
		{exp, "GET", core.PortablePath, 200},
		{exp, "GET", "/api/v1/export.json", 200},
		{exp, "GET", "/api/v1/export.csv", 200},
		{exp, "POST", "/api/v1/import", 403},
		{exp, "GET", "/api/v1/settings", 403},
		{exp, "GET", "/api/v1/habits", 403},
		{exp, "GET", "/api/v1/backup", 403},
		{imp, "GET", core.PortablePath, 200},
		{imp, "GET", "/api/v1/export.json", 403},
		{imp, "PUT", "/api/v1/settings", 403},
		{imp, "POST", "/api/v1/habits", 403},
		{imp, "POST", "/api/v1/checkin", 403},
		{read, "GET", core.PortablePath, 403},
		{read, "GET", "/api/v1/export.json", 403},
	} {
		if code, body := c.c.do(c.method, c.path, "{}"); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%s)", c.c.tok, c.method, c.path, code, c.want, body)
		}
	}
	_, body := exp.do("GET", core.PortablePath, "")
	var p core.Portable
	if err := json.Unmarshal([]byte(body), &p); err != nil || len(p.Exports) != 2 || len(p.Imports) != 1 || p.Exports[0].Kind != core.KindPocket {
		t.Fatalf("portable: %s", body)
	}
}

// Move brings A's habits to B: the import adds once, leaves B's own days and
// journal alone, and keeps the two apart.
func TestMoveImportsAddOnce(t *testing.T) {
	A, B, link := moveBox(t)
	today := store.Today()
	var h struct{ ID int64 }
	if code := A.json("POST", "/api/v1/habits", `{"name":"Alice walk"}`, &h); code != 201 && code != 200 {
		t.Fatalf("habit: %d", code)
	}
	if code, body := A.do("PUT", "/api/v1/habits/"+itoa(h.ID)+"/entries/"+today, `{"status":1}`); code != 200 {
		t.Fatalf("mark: %d %s", code, body)
	}
	if code, body := A.do("PUT", "/api/v1/journal/"+today, `{"text":"A's day"}`); code != 200 {
		t.Fatalf("journal: %d %s", code, body)
	}
	if code, body := B.do("PUT", "/api/v1/journal/"+today, `{"text":"B's own day"}`); code != 200 {
		t.Fatalf("journal: %d %s", code, body)
	}

	code, export := link("pcx_a_export").do("GET", "/api/v1/export.json", "")
	if code != 200 || !strings.Contains(export, "Alice walk") || !strings.Contains(export, "A's day") {
		t.Fatalf("A's export: %d %s", code, export)
	}
	if _, bx := link("pcx_b_export").do("GET", "/api/v1/export.json", ""); strings.Contains(bx, "Alice walk") || strings.Contains(bx, "A's day") {
		t.Fatalf("B's export holds A's data: %s", bx)
	}
	imp := func() map[string]float64 {
		code, out := link("pcx_b_import").do("POST", "/api/v1/import", export)
		if code != 200 {
			t.Fatalf("import → %d %s", code, out)
		}
		var m map[string]float64
		_ = json.Unmarshal([]byte(out), &m)
		return m
	}
	if first := imp(); first["created"] < 1 || first["entries"] != 1 {
		t.Fatalf("first import: %v", first)
	}
	if second := imp(); second["created"] != 0 || second["entries"] != 0 {
		t.Fatalf("second import added: %v", second)
	}
	_, bx := link("pcx_b_export").do("GET", "/api/v1/export.json", "")
	if !strings.Contains(bx, "Alice walk") || !strings.Contains(bx, "B's own day") || strings.Contains(bx, "A's day") {
		t.Fatalf("B after the import: %s", bx)
	}
	var habits []map[string]any
	A.json("GET", "/api/v1/habits", "", &habits)
	n := 0
	for _, x := range habits {
		if x["name"] == "Alice walk" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("A's habits changed: %v", habits)
	}
}
