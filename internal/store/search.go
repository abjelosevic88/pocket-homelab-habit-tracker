package store

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"
)

// searchWords is the most words of a search that are looked for; the rest is ignored.
const searchWords = 8

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// likeAll builds " AND (col LIKE ? …)" once per word of q, so a row matches when it has
// every word somewhere, in any order. SQLite's LIKE ignores case for a–z only, so a word
// with other letters (š, é, ж) is also tried in lower case, in capitals and capitalised.
func likeAll(col, q string) (string, []any) {
	var where strings.Builder
	var args []any
	words := strings.Fields(q)
	if len(words) > searchWords {
		words = words[:searchWords]
	}
	for _, w := range words {
		forms := []string{w}
		if strings.IndexFunc(w, func(r rune) bool { return r >= utf8.RuneSelf }) >= 0 {
			lower := strings.ToLower(w)
			first, n := utf8.DecodeRuneInString(lower)
			for _, f := range []string{lower, strings.ToUpper(w), string(unicode.ToUpper(first)) + lower[n:]} {
				seen := false
				for _, have := range forms {
					seen = seen || have == f
				}
				if !seen {
					forms = append(forms, f)
				}
			}
		}
		where.WriteString(" AND (")
		for i, f := range forms {
			if i > 0 {
				where.WriteString(" OR ")
			}
			where.WriteString(col + ` LIKE ? ESCAPE '\'`)
			args = append(args, "%"+likeEscaper.Replace(f)+"%")
		}
		where.WriteString(")")
	}
	return where.String(), args
}

// SearchHabits finds the user's habits whose name has every word of q: the ones still
// running first, in the order of the page, then the ended ones.
func (s *Store) SearchHabits(ctx context.Context, uid int64, q string, limit int) ([]Habit, error) {
	where, args := likeAll("name", q)
	if where == "" {
		return []Habit{}, nil
	}
	args = append(append([]any{uid}, args...), Today(), limit)
	rows, err := s.DB.QueryContext(ctx, `SELECT `+habitCols+` FROM habits WHERE user_id = ?`+where+`
		ORDER BY (end_date IS NOT NULL AND end_date < ?), sort_order, id LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Habit{}
	for rows.Next() {
		h, err := scanHabit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *h)
	}
	return out, rows.Err()
}

// SearchJournal finds the user's journal days that have every word of q in the good line,
// the bad line or the important note, the newest day first.
func (s *Store) SearchJournal(ctx context.Context, uid int64, q string, limit int) ([]JournalLine, error) {
	where, args := likeAll("(text || char(10) || bad_text || char(10) || important)", q)
	if where == "" {
		return []JournalLine{}, nil
	}
	args = append(append([]any{uid}, args...), limit)
	rows, err := s.DB.QueryContext(ctx, `SELECT date, text, bad_text, important FROM journal WHERE user_id = ?`+where+` ORDER BY date DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JournalLine{}
	for rows.Next() {
		var l JournalLine
		if err := rows.Scan(&l.Date, &l.Text, &l.Bad, &l.Important); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
