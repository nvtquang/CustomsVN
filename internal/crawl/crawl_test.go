package crawl

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"

	"github.com/nvtquang/CustomsVN/internal/corpus"
	"github.com/nvtquang/CustomsVN/internal/prep"
)

func TestInScope(t *testing.T) {
	scope := map[string]bool{"decree-08": true}
	for _, c := range []struct {
		it   Item
		want bool
	}{
		{Item{Title: "Thông tư quy định thủ tục hải quan đối với hàng hóa gia công"}, true},
		{Item{Title: "Nghị định quy định chi tiết Luật Hóa chất"}, true},
		{Item{Title: "Quyết định phê duyệt quy hoạch", Field: "Xuất nhập khẩu"}, true},
		// No cue, but it amends a decree already in scope.
		{Item{Title: "Nghị định sửa đổi Nghị định số 08/2015", Relationships: map[string][]string{"Sửa đổi, bổ sung": {"decree-08"}}}, true},
		{Item{Title: "Quyết định về việc tặng bằng khen"}, false},
		{Item{Title: "Nghị quyết về giáo dục", Relationships: map[string][]string{"Căn cứ": {"other"}}}, false},
	} {
		if got := InScope(c.it, scope); got != c.want {
			t.Errorf("%q: got %v", c.it.Title, got)
		}
	}
}

func TestRecheckTakesActedOnDocumentsAndAShareOfTheRest(t *testing.T) {
	scope := map[string]bool{}
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		scope[id] = true
	}
	news := []Item{{ID: "n1", Relationships: map[string][]string{"Thay thế": {"a"}, "Căn cứ": {"b"}, "Bãi bỏ": {"outside"}}}}
	week := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	got := Recheck(news, scope, 4, week)
	if !slices.Contains(got, "a") || slices.Contains(got, "outside") {
		t.Errorf("acted-on documents: %v", got)
	}
	// Over four consecutive weeks every in-scope document comes up.
	all := map[string]bool{}
	for w := 0; w < 4; w++ {
		for _, id := range Recheck(nil, scope, 4, week.AddDate(0, 0, 7*w)) {
			all[id] = true
		}
	}
	if len(all) != len(scope) {
		t.Errorf("four weeks rechecked %d of %d", len(all), len(scope))
	}
}

func TestChanged(t *testing.T) {
	old := corpus.Row{Title: "T", TinhTrang: "Còn hiệu lực", NgayHieuLuc: "01/01/2020"}
	if Changed(old, Item{Title: "T", Status: "Còn hiệu lực", EffectiveFrom: "01/01/2020"}) {
		t.Error("same document reported changed")
	}
	// The portal leaving a field out is not a change: the upsert keeps it.
	if Changed(old, Item{Title: "T"}) {
		t.Error("empty fields reported as a change")
	}
	if !Changed(old, Item{Title: "T", Status: "Hết hiệu lực toàn bộ", ExpiredOn: "17/01/2026"}) {
		t.Error("expiry not seen")
	}
}

func TestLatestRevisionAndPrune(t *testing.T) {
	data := t.TempDir()
	raw := filepath.Join(data, "raw", "th1nhng0")
	write := func(name, fetched string) {
		dir := filepath.Join(raw, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"fetched_at": "`+fetched+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := prep.LatestRevision(data); got != prep.Revision {
		t.Errorf("empty raw store: %s", got)
	}
	write(prep.Revision, "2026-10-27T00:00:00Z")
	write(Prefix+"20261101T020000", "2026-11-01T02:00:00Z")
	write(Prefix+"20261108T020000", "2026-11-08T02:00:00Z")
	write(Prefix+"20261115T020000", "2026-11-15T02:00:00Z")
	if got := prep.LatestRevision(data); got != Prefix+"20261115T020000" {
		t.Errorf("latest %s", got)
	}
	prune(raw, 2, func(string, ...any) {})
	entries, _ := os.ReadDir(raw)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{prep.Revision, Prefix + "20261108T020000", Prefix + "20261115T020000"}
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Errorf("after prune %v", names)
	}
}

func TestLastSlot(t *testing.T) {
	loc := time.FixedZone("ICT", 7*3600)
	s := Schedule{Enabled: true, Weekday: int(time.Sunday), Hour: 2, MaxPages: 10}
	for now, want := range map[time.Time]time.Time{
		// Wednesday: the Sunday before.
		time.Date(2026, 10, 28, 9, 0, 0, 0, loc): time.Date(2026, 10, 25, 2, 0, 0, 0, loc),
		// Sunday before two o'clock: the Sunday a week earlier.
		time.Date(2026, 11, 1, 1, 59, 0, 0, loc): time.Date(2026, 10, 25, 2, 0, 0, 0, loc),
		// Sunday at two o'clock: now.
		time.Date(2026, 11, 1, 2, 0, 0, 0, loc): time.Date(2026, 11, 1, 2, 0, 0, 0, loc),
	} {
		if got := s.LastSlot(now); !got.Equal(want) {
			t.Errorf("%s: got %s, want %s", now, got, want)
		}
	}
	m := Schedule{Enabled: true, Every: Monthly, Day: 1, Hour: 2, MaxPages: 20}
	for now, want := range map[time.Time]time.Time{
		// Mid-month: the 1st of this month.
		time.Date(2026, 10, 29, 9, 0, 0, 0, loc): time.Date(2026, 10, 1, 2, 0, 0, 0, loc),
		// The 1st before two o'clock: the 1st of the month before, across a year.
		time.Date(2027, 1, 1, 1, 0, 0, 0, loc): time.Date(2026, 12, 1, 2, 0, 0, 0, loc),
	} {
		if got := m.LastSlot(now); !got.Equal(want) {
			t.Errorf("monthly %s: got %s, want %s", now, got, want)
		}
	}
	if (Schedule{Every: Weekly, Weekday: 7, Day: 1, MaxPages: 1}).Validate() == nil ||
		(Schedule{Every: Monthly, Day: 31, MaxPages: 1}).Validate() == nil || DefaultSchedule().Validate() != nil {
		t.Error("validate")
	}
}

func TestMergeContentReplacesByID(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, rows []contentRow) string {
		p := filepath.Join(dir, name)
		if err := parquet.WriteFile(p, rows); err != nil {
			t.Fatal(err)
		}
		return p
	}
	base := write("base.parquet", []contentRow{{"1", "old one"}, {"2", "two"}, {"3", "three"}})
	fresh := write("fresh.parquet", []contentRow{{"1", "new one"}, {"4", "four"}})
	out := filepath.Join(dir, "out.parquet")
	if err := mergeContent(base, fresh, out); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[contentRow](out)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r.ID] = r.ContentHTML
	}
	if len(rows) != 4 || got["1"] != "new one" || got["2"] != "two" || got["4"] != "four" {
		t.Errorf("merged %v", rows)
	}
}
