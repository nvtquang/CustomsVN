package corpus

import (
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/luatdo/law"
)

const asOf = "2026-10-27"

func TestISODate(t *testing.T) {
	for in, want := range map[string]string{
		"1/7/2016":     "2016-07-01",
		"17/08/2007":   "2007-08-17",
		"2016-07-01":   "2016-07-01",
		"":             "",
		" 01/01/2020 ": "2020-01-01",
	} {
		if got := ISODate(in); got != want {
			t.Errorf("ISODate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestForceTrustsTheExpiryDateOverTheLabel(t *testing.T) {
	// The source updates the date before the label.
	m := &Meta{ForceStatus: StatusInForce, ExpiredOn: "01/07/2025"}
	if f := Force(m, asOf); f != "" {
		t.Errorf("expired document has force %q", f)
	}
	m.ForceStatus = StatusPartly
	if f := Force(m, asOf); f != ForcePartly {
		t.Errorf("partly expired document has force %q, want %q", f, ForcePartly)
	}
}

func TestForce(t *testing.T) {
	for _, c := range []struct {
		m    Meta
		want string
	}{
		{Meta{ForceStatus: StatusInForce, EffectiveFrom: "01/01/2020"}, ForceInForce},
		{Meta{ForceStatus: StatusInForce, EffectiveFrom: "01/01/2027"}, ForceNotYet},
		{Meta{ForceStatus: StatusNotYet}, ForceNotYet},
		{Meta{ForceStatus: StatusExpired}, ""},
		{Meta{ForceStatus: StatusUndetermined}, ""},
		{Meta{ForceStatus: ""}, ""},
	} {
		if got := Force(&c.m, asOf); got != c.want {
			t.Errorf("Force(%+v) = %q, want %q", c.m, got, c.want)
		}
	}
}

func TestKeptNeedsAParsedDocument(t *testing.T) {
	m := &Meta{ForceStatus: StatusInForce, Status: "quarantined"}
	if f, why := Kept(m, asOf); f != "" || why != "in force but not usable: quarantined" {
		t.Errorf("Kept = %q, %q", f, why)
	}
}

func TestTierOf(t *testing.T) {
	core := &Meta{Title: "Thông tư quy định về thủ tục Hải quan", IssuingBody: "Bộ Tài chính"}
	if got := TierOf(core, nil); got.Tier != Tier1 || !slices.Equal(got.Why, []string{"hải quan"}) {
		t.Errorf("customs circular: %+v", got)
	}
	// A province can legislate about customs; it is still not customs law.
	local := &Meta{Title: "Quyết định về phí cửa khẩu", IssuingBody: "UBND tỉnh Lạng Sơn"}
	if got := TierOf(local, nil); got.Tier != Tier2 {
		t.Errorf("provincial act: %+v", got)
	}
	near := &Meta{Title: "Nghị định về hóa đơn, chứng từ"}
	if got := TierOf(near, nil); got.Tier != Tier2 {
		t.Errorf("invoice decree: %+v", got)
	}
	bySubject := &Meta{Title: "Nghị định"}
	if got := TierOf(bySubject, []string{"thue/thue-xuat-nhap-khau"}); got.Tier != Tier1 {
		t.Errorf("by subject: %+v", got)
	}
	if got := TierOf(&Meta{Title: "Luật Đất đai"}, []string{"dat-dai"}); got.Tier != Tier3 {
		t.Errorf("land law: %+v", got)
	}
}

func TestVerdict(t *testing.T) {
	for q, want := range map[string]string{
		"no article structure found in body":                                          VerdictNoStructure,
		"body carries official number 09/2015/TT-BTC, metadata says 9/2015/TT-BTC":    VerdictFormat,
		"body carries official number 38/2015/TT-BTC, metadata says 38/2016/TT-BTC":   VerdictYearTypo,
		"body carries official number 39/2015/TT-BTC, metadata says 38/2015/TT-BTC":   VerdictNumberDiffers,
		"body carries official number 12/2010/NĐ-CP, metadata says 38/2015/TT-BTC":    VerdictOtherDocument,
		"body carries official number 128/2020/NĐ-CP, metadata says 128/2020/NĐ-CP-X": VerdictFormat,
	} {
		if got := Verdict(q); got != want {
			t.Errorf("Verdict(%q) = %q, want %q", q, got, want)
		}
	}
}

func TestStoreID(t *testing.T) {
	if got := StoreID("38/2015/TT-BTC"); got != "vn:law:2015:38-2015-tt-btc" {
		t.Errorf("got %q", got)
	}
	if got := StoreID("08/2015/NĐ-CP"); got != "vn:law:2015:08-2015-nd-cp" {
		t.Errorf("got %q", got)
	}
	if got := StoreID("43/VBHN-BCT"); got != "" {
		t.Errorf("number without year gave %q", got)
	}
}

func TestReadConsolidation(t *testing.T) {
	text := "THÔNG TƯ\nQuy định về thủ tục hải quan\n" +
		"Thông tư số 38/2015/TT-BTC ngày 25 tháng 3 năm 2015 của Bộ Tài chính, có hiệu lực kể từ ngày 01/4/2015, được sửa đổi, bổ sung bởi:\n" +
		"1. Thông tư số 39/2018/TT-BTC ngày 20 tháng 4 năm 2018;\n" +
		"2. Thông tư số 38/2015/TT-BTC (nhắc lại);\n" +
		"Căn cứ Luật Hải quan số 54/2014/QH13;\n" +
		"Điều 1. Phạm vi điều chỉnh [1]\n" +
		"[1] Điều này được sửa đổi, bổ sung theo quy định tại khoản 1 Điều 1 Thông tư số 39/2018/TT-BTC\n" +
		"[2] Ghi chú khác\n"
	c := ReadConsolidation("Văn bản hợp nhất số 25/VBHN-BTC", text)
	if c.Base != "38/2015/TT-BTC" {
		t.Errorf("base %q", c.Base)
	}
	// The legal basis after "Căn cứ" is not an amender, and neither is the base.
	if !slices.Equal(c.Amenders, []string{"39/2018/TT-BTC"}) {
		t.Errorf("amenders %v", c.Amenders)
	}
	if len(c.Notes) != 1 || !strings.HasPrefix(c.Notes["1"], "Điều này được sửa đổi") {
		t.Errorf("notes %v", c.Notes)
	}
}

func TestReadConsolidationIgnoresWhatTheTitleAndListMention(t *testing.T) {
	// 47/VBHN-BCT: the title names the decree the circular implements before
	// the circular, and an amender's paragraph names other instruments.
	text := "THÔNG TƯ\n" +
		"Thông tư số 12/2018/TT-BCT ngày 15 tháng 06 năm 2018 quy định chi tiết Nghị định số 69/2018/NĐ-CP, được sửa đổi, bổ sung bởi:\n" +
		"1. Thông tư số 42/2019/TT-BCT ngày 18 tháng 12 năm 2019 sửa đổi Thông tư số 59/2015/TT-BCT;\n" +
		"2. Thông tư số 08/2023/TT-BCT ngày 31 tháng 3 năm 2023.\n" +
		"Căn cứ Luật Quản lý ngoại thương;\n" +
		"Điều 1. Phạm vi điều chỉnh\n"
	c := ReadConsolidation("Văn bản hợp nhất số 47/VBHN-BCT Thông tư quy định chi tiết Nghị định số 69/2018/NĐ-CP", text)
	if c.Base != "12/2018/TT-BCT" {
		t.Errorf("base %q", c.Base)
	}
	if !slices.Equal(c.Amenders, []string{"42/2019/TT-BCT", "08/2023/TT-BCT"}) {
		t.Errorf("amenders %v", c.Amenders)
	}
}

func TestVBHNID(t *testing.T) {
	r := &Row{ID: "84a3fb80", SoKyHieu: " 43/VBHN-BCT ", CoQuan: "Bộ Công Thương", NgayBanHanh: "04/06/2026"}
	if got := VBHNID(r); got != "vn:vbhn:2026:43-vbhn-bct:bo-cong-thuong:84a3fb80" {
		t.Errorf("got %q", got)
	}
}

func TestItemIDKeepsAUUID(t *testing.T) {
	// The prototype read ItemID=(\d+), cut this one to "49828", and gave the
	// decree on airports the text of a provincial resolution on school fees.
	u := "https://vbpl.vn/pages/portal.aspx?ItemID=49828ac0-6e5a-11f1-b670-d3ec94724b75"
	if got := ItemID(u); got != "49828ac0-6e5a-11f1-b670-d3ec94724b75" {
		t.Errorf("got %q", got)
	}
}

func collect(t *testing.T) (*Chunker, *[]Chunk) {
	t.Helper()
	var out []Chunk
	return NewChunker(func(c Chunk) error { out = append(out, c); return nil }), &out
}

func ids(cs []Chunk) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return out
}

func TestTreeChunksOneArticleWithItsClauses(t *testing.T) {
	d := "vn:law:2014:54-2014-qh13"
	ps := []law.Provision{
		{ID: d + ":chapter-1", Kind: "chapter", Number: "1", Heading: "NHỮNG QUY ĐỊNH CHUNG"},
		{ID: d + ":chapter-1:article-1", ParentID: d + ":chapter-1", Kind: "article", Number: "1", Heading: "Phạm vi điều chỉnh"},
		{ID: d + ":chapter-1:article-1:clause-1", Kind: "clause", Number: "1", Text: "Luật này quy định"},
		{ID: d + ":chapter-1:article-1:clause-1:point-a", Kind: "point", Number: "a", Text: "thủ tục hải quan"},
	}
	ch, out := collect(t)
	src := &Source{Kind: SourceOriginal, Tier: Tier1, Force: ForceInForce}
	if err := ch.Tree(&Meta{ID: d}, ps, src, nil); err != nil {
		t.Fatal(err)
	}
	if len(*out) != 1 {
		t.Fatalf("got %v", ids(*out))
	}
	c := (*out)[0]
	if c.Text != "1. Luật này quy định\na) thủ tục hải quan" {
		t.Errorf("text %q", c.Text)
	}
	if !slices.Equal(c.Path, []string{"Chương 1. NHỮNG QUY ĐỊNH CHUNG", "Điều 1. Phạm vi điều chỉnh"}) {
		t.Errorf("path %q", c.Path)
	}
}

func TestTreeSplitsALongArticleAtItsClauses(t *testing.T) {
	d := "vn:law:2020:1-2020-tt-btc"
	long := strings.Repeat("chữ ", 600) // 2,400 characters, 3,000+ bytes
	ps := []law.Provision{
		{ID: d + ":article-5", Kind: "article", Number: "5", Text: "Mở đầu"},
		{ID: d + ":article-5:clause-1", Kind: "clause", Number: "1", Text: long},
		{ID: d + ":article-5:clause-2", Kind: "clause", Number: "2", Text: long},
	}
	ch, out := collect(t)
	if err := ch.Tree(&Meta{ID: d}, ps, &Source{Kind: SourceOriginal, Tier: Tier2}, nil); err != nil {
		t.Fatal(err)
	}
	want := []string{d + ":article-5", d + ":article-5:clause-1", d + ":article-5:clause-2"}
	if !slices.Equal(ids(*out), want) {
		t.Errorf("got %v, want %v", ids(*out), want)
	}
}

func TestTreeGivesAnArticleInAnAnnexOneChunk(t *testing.T) {
	// A regulation issued as an annex has articles of its own. Each is one
	// chunk, and the annex does not carry them again.
	d := "vn:law:2021:80-2021-tt-btc"
	long := strings.Repeat("văn bản ", 300)
	ps := []law.Provision{
		{ID: d + ":annex-1", Kind: "annex", Number: "1", Text: "Quy chế"},
		{ID: d + ":annex-1:article-1", ParentID: d + ":annex-1", Kind: "article", Number: "1", Text: long},
		{ID: d + ":annex-1:article-2", ParentID: d + ":annex-1", Kind: "article", Number: "2", Text: long},
	}
	ch, out := collect(t)
	if err := ch.Tree(&Meta{ID: d}, ps, &Source{Kind: SourceOriginal, Tier: Tier1}, nil); err != nil {
		t.Fatal(err)
	}
	want := []string{d + ":annex-1", d + ":annex-1:article-1", d + ":annex-1:article-2"}
	if !slices.Equal(ids(*out), want) {
		t.Errorf("got %v, want %v", ids(*out), want)
	}
	if (*out)[0].Text != "Quy chế" {
		t.Errorf("annex chunk carries %q", (*out)[0].Text)
	}
}

func TestTreeAttachesAmendmentNotes(t *testing.T) {
	d := "vn:vbhn:2026:1-vbhn-btc:bo-tai-chinh:1"
	ps := []law.Provision{{ID: d + ":article-1", Kind: "article", Number: "1", Text: "Phạm vi [3]"}}
	ch, out := collect(t)
	notes := map[string]string{"3": "được sửa đổi bởi Thông tư 39/2018/TT-BTC"}
	if err := ch.Tree(&Meta{ID: d}, ps, &Source{Kind: SourceConsolidated, Tier: Tier1}, notes); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal((*out)[0].AmendmentNotes, []string{notes["3"]}) {
		t.Errorf("notes %v", (*out)[0].AmendmentNotes)
	}
}

func TestCutLinesKeepsLinesWhole(t *testing.T) {
	line := strings.Repeat("x", 1000)
	text := strings.Join([]string{line, line, line, line, line}, "\n")
	pieces := cutLines(text, MaxPiece)
	if len(pieces) != 3 {
		t.Fatalf("got %d pieces", len(pieces))
	}
	if strings.Join(pieces, "\n") != text {
		t.Error("pieces do not rejoin to the text")
	}
}

func TestEmitCutsALongChunkIntoParts(t *testing.T) {
	ch, out := collect(t)
	text := strings.Repeat(strings.Repeat("ô", 1000)+"\n", 6)
	src := &Source{Kind: SourceOriginal, Tier: Tier1}
	if err := ch.emit(&Meta{ID: "d"}, src, "d:annex-1", nil, text, nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(*out), []string{"d:annex-1#part-1", "d:annex-1#part-2", "d:annex-1#part-3"}) {
		t.Errorf("got %v", ids(*out))
	}
	if ch.Stats["chunks_cut_long"] != 1 || ch.Stats["chunks_tier1"] != 3 {
		t.Errorf("stats %v", ch.Stats)
	}
}

func TestTextDropsTheSignatureBlock(t *testing.T) {
	ch, out := collect(t)
	text := "Quyết định\nĐoạn một\nĐoạn hai\nNơi nhận:\n- Như trên"
	if err := ch.Text(&Meta{ID: "d", Title: "Quyết định"}, text, &Source{Kind: SourceRescued, Tier: Tier2}); err != nil {
		t.Fatal(err)
	}
	if len(*out) != 1 || (*out)[0].ID != "d:para-1" || strings.Contains((*out)[0].Text, "Như trên") {
		t.Errorf("got %+v", *out)
	}
}

func TestHSRowsBuildsTheParentChain(t *testing.T) {
	tables := [][][]string{{
		{"Mã hàng", "Mô tả hàng hóa", "Đơn vị tính", "", "Description", ""},
		{"01.01", "Ngựa, lừa, la sống.", "", "", "Live horses, asses, mules and hinnies.", ""},
		{"", "- Ngựa:", "", "", "- Horses:", ""},
		{"0101.21.00", "- - Loại thuần chủng để nhân giống", "con", "", "- - Pure-bred breeding animals", ""},
		{"0101.29", "- - Loại khác:", "", "", "- - Other:", ""},
		{"0101.29.10", "- - - Loại dùng cho đua", "con", "", "- - - For racing", ""},
	}}
	rows := HSRows(tables)
	if len(rows) != 5 {
		t.Fatalf("got %d rows", len(rows))
	}
	r := rows[2]
	if r.Code != "0101.21.00" || r.Level != 2 || r.ParentDescription != "Ngựa:" || r.Chapter != "01" || r.Unit != "con" {
		t.Errorf("tariff line %+v", r)
	}
	if rows[4].ParentCode != "0101.29" || rows[1].ParentCode != "01.01" {
		t.Errorf("parents %q %q", rows[4].ParentCode, rows[1].ParentCode)
	}
	if !IsTariffLine(rows[4].Code) || IsTariffLine(rows[3].Code) {
		t.Error("IsTariffLine")
	}
}

func TestTablesSkipLayoutAndPlaceUnderTheirProvision(t *testing.T) {
	d := "vn:law:2025:86-2025-tt-btc"
	ps := []law.Provision{
		{ID: d + ":article-3", Kind: "article", Number: "3", Heading: "Mức thu", Text: "Mức thu như sau:\nPhí hải quan đối với hàng hóa\n20.000 đồng/tờ khai"},
	}
	tables := [][][]string{
		{{"BỘ TÀI CHÍNH", "CỘNG HÒA XÃ HỘI CHỦ NGHĨA VIỆT NAM"}, {"Số: 86/2025/TT-BTC", "Hà Nội"}},
		{{"STT", "Nội dung thu", "Mức thu"}, {"1", "Phí hải quan đối với hàng hóa", "20.000 đồng/tờ khai"}},
	}
	ch, out := collect(t)
	if err := ch.Tables(&Meta{ID: d, Title: "Thông tư"}, tables, ps, &Source{Kind: SourceOriginal, Tier: Tier1}); err != nil {
		t.Fatal(err)
	}
	if len(*out) != 1 {
		t.Fatalf("got %v", ids(*out))
	}
	c := (*out)[0]
	if c.ID != d+":table-2" || c.Kind != "table" || !slices.Equal(c.Path, []string{"Điều 3. Mức thu"}) {
		t.Errorf("got %+v", c)
	}
	if !strings.HasPrefix(c.Text, "| STT | Nội dung thu | Mức thu |\n| --- | --- | --- |\n| 1 |") {
		t.Errorf("text %q", c.Text)
	}
}
