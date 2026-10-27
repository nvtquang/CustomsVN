package corpus

import (
	"regexp"
	"strings"

	"github.com/tamnd/luatdo/law"
)

// DocTypeVBHN is the document type vbpl.vn gives a consolidated text.
const DocTypeVBHN = "Văn bản hợp nhất"

// Row is one row of the th1nhng0 metadata config, with the field and sector
// columns luatdo does not read.
type Row struct {
	ID          string `parquet:"id"`
	Title       string `parquet:"title"`
	SoKyHieu    string `parquet:"so_ky_hieu"`
	LoaiVanBan  string `parquet:"loai_van_ban"`
	NgayBanHanh string `parquet:"ngay_ban_hanh"`
	NgayHieuLuc string `parquet:"ngay_co_hieu_luc"`
	NgayHetHieu string `parquet:"ngay_het_hieu_luc"`
	TinhTrang   string `parquet:"tinh_trang_hieu_luc"`
	CoQuan      string `parquet:"co_quan_ban_hanh"`
	LinhVuc     string `parquet:"linh_vuc"`
	Nganh       string `parquet:"nganh"`
}

// IsVBHN reports whether a row is a consolidated text. The type column is
// missing on some, so the number is read as well.
func IsVBHN(r *Row) bool {
	return r.LoaiVanBan == DocTypeVBHN || strings.Contains(strings.ToUpper(r.SoKyHieu), "VBHN")
}

// TierVBHN tiers a consolidated text by its title, field and sector. luatdo
// does not parse these rows, so they have no subjects to go by.
func TierVBHN(r *Row) int {
	t := r.Title + " " + r.LinhVuc + " " + r.Nganh
	switch {
	case len(Cues(t, CoreCues)) > 0:
		return Tier1
	case len(Cues(t, NearCues)) > 0:
		return Tier2
	}
	return Tier3
}

var trailingYear = regexp.MustCompile(`(\d{4})$`)

// VBHNID mints the identifier of a consolidated text. Its number has no year
// and repeats across ministries and years ("01/VBHN-BTC" exists many times), so
// the identifier carries the year, the body and the source row.
func VBHNID(r *Row) string {
	y := "0000"
	if m := trailingYear.FindStringSubmatch(r.NgayBanHanh); m != nil {
		y = m[1]
	}
	body := r.CoQuan
	if body == "" {
		body = "unknown"
	}
	return "vn:vbhn:" + y + ":" + law.Slug(strings.TrimSpace(r.SoKyHieu)) + ":" + law.Slug(body) + ":" + r.ID
}

// MetaOf is the document header of a metadata row under a given identifier.
func MetaOf(r *Row, id string) Meta {
	return Meta{
		ID: id, OfficialNumber: strings.TrimSpace(r.SoKyHieu), IssuingBody: r.CoQuan, Title: r.Title,
		DocType: r.LoaiVanBan, EffectiveFrom: r.NgayHieuLuc, SignedOn: r.NgayBanHanh, ExpiredOn: r.NgayHetHieu,
		ForceStatus: r.TinhTrang, SourceURL: SourceURL(r.ID), Status: "parsed",
	}
}

// SourceURL is the vbpl.vn page of a row.
func SourceURL(itemID string) string {
	return "https://vbpl.vn/pages/portal.aspx?ItemID=" + itemID
}

var itemID = regexp.MustCompile(`ItemID=([^&#]+)`)

// ItemID is the source row behind a vbpl.vn URL.
func ItemID(sourceURL string) string {
	if m := itemID.FindStringSubmatch(sourceURL); m != nil {
		return m[1]
	}
	return ""
}

// VBHNEmpty is the length below which a consolidated text is an empty page.
// 1,902 of the 2,242 at the source are an HTML shell around a link to a file.
const VBHNEmpty = 200

var (
	number      = regexp.MustCompile(`\d+[a-z]?/\d{4}/[A-ZĐa-zđ0-9&-]+`)
	amendedBy   = regexp.MustCompile(`được sửa đổi, bổ sung bởi|được sửa đổi bởi|được bổ sung bởi`)
	legalBasis  = regexp.MustCompile(`\n\s*Căn cứ`)
	footnote    = regexp.MustCompile(`(?m)^\s*\[(\d+)\]\s*(.+)$`)
	noteChanges = regexp.MustCompile(`sửa đổi|bổ sung|bãi bỏ|thay thế|hợp nhất|hiệu lực`)
)

// Consolidation is what a consolidated text says about itself: the instrument
// it consolidates, the instruments that amended it, and the footnotes that say
// which amendment changed which clause.
type Consolidation struct {
	Base     string
	Amenders []string
	Notes    map[string]string
}

// ReadConsolidation reads the preamble and footnotes of a consolidated text.
//
// Every VBHN opens with the same formula: "Thông tư số 12/2018/TT-BCT ngày
// ... có hiệu lực ..., được sửa đổi, bổ sung bởi:" and then one paragraph per
// amending instrument. The base is the number that opens the paragraph
// holding the phrase; the amenders are the first number of each paragraph
// after it, up to the legal basis. Taking every number instead picks up the
// instruments those paragraphs merely mention, and taking the first number of
// the title picks the decree a circular implements (47/VBHN-BCT names
// 69/2018/NĐ-CP before its base 12/2018/TT-BCT). Both are read from the text
// the Ministry published, never guessed.
func ReadConsolidation(title, text string) Consolidation {
	head := text
	if i := strings.Index(text, "Điều 1"); i >= 0 {
		head = text[:i]
	} else {
		head = runes(text, 3000)
	}
	parts := amendedBy.Split(head, 2)
	var c Consolidation
	if len(parts) > 1 {
		before := parts[0][strings.LastIndexByte(parts[0], '\n')+1:]
		c.Base = number.FindString(before)
	}
	if c.Base == "" {
		c.Base = number.FindString(title)
	}
	if c.Base == "" {
		c.Base = number.FindString(parts[0])
	}
	if len(parts) > 1 {
		seen := map[string]bool{}
		for para := range strings.SplitSeq(legalBasis.Split(parts[1], 2)[0], "\n") {
			n := number.FindString(para)
			if n == "" || seen[n] || c.Base != "" && NormNumber(n) == NormNumber(c.Base) {
				continue
			}
			seen[n] = true
			c.Amenders = append(c.Amenders, n)
		}
	}
	c.Notes = map[string]string{}
	for _, m := range footnote.FindAllStringSubmatch(text, -1) {
		if noteChanges.MatchString(m[2]) {
			c.Notes[m[1]] = runes(strings.TrimSpace(m[2]), 400)
		}
	}
	return c
}
