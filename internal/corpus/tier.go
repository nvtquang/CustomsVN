package corpus

import (
	"slices"
	"strings"
)

// Tiers. Tier 1 is customs proper: procedures, import and export duty, origin,
// valuation, classification, trade remedies, bonded areas, smuggling. Tier 2
// is what customs work keeps running into: other taxes, trade, fees, foreign
// exchange, borders and transport, and the national acts tier 1 leans on.
// Tier 3 stays in the store and is never retrieved for the customs bot.
const (
	Tier1 = 1
	Tier2 = 2
	Tier3 = 3
)

var coreSubjects = []string{"thue/hai-quan", "thue/thue-xuat-nhap-khau", "thuong-mai/xuat-nhap-khau"}

var nearSubjects = []string{"thue", "thuong-mai/canh-tranh", "thuong-mai/hoat-dong-thuong-mai",
	"ngan-hang-tai-chinh/ngoai-hoi", "tai-chinh-cong/phi-le-phi"}

// CoreCues are title phrases that put a document in tier 1 on their own. They
// are matched as lowercase substrings, so each one is specific enough not to
// fire inside an unrelated word.
var CoreCues = []string{"hải quan", "xuất khẩu", "nhập khẩu", "xuất, nhập khẩu", "quá cảnh", "tạm nhập", "tái xuất",
	"tái nhập", "tạm xuất", "kho ngoại quan", "kho bảo thuế", "thu gom hàng lẻ", "xuất xứ hàng hóa",
	"xuất xứ hàng hoá", "trị giá hải quan", "phân loại hàng hóa", "mã số hàng hóa", "biểu thuế",
	"chế xuất", "phi thuế quan", "buôn lậu", "gian lận thương mại", "phòng vệ thương mại",
	"chống bán phá giá", "chống trợ cấp", "biện pháp tự vệ", "quản lý ngoại thương", "một cửa quốc gia",
	"một cửa asean", "kiểm tra chuyên ngành", "kiểm dịch", "hành lý", "cửa khẩu", "gia công hàng hóa",
	"chuyển khẩu", "tạm dừng làm thủ tục", "doanh nghiệp ưu tiên", "hàng hóa xuất", "hàng hóa nhập",
	"thuế xuất khẩu", "thuế nhập khẩu", "hạn ngạch thuế quan", "ưu đãi đặc biệt"}

// NearCues put a document in tier 2.
var NearCues = []string{"biên giới", "xuất cảnh", "nhập cảnh", "logistics", "vận tải quốc tế", "vận tải đa phương thức",
	"cảng biển", "cảng hàng không", "hiệp định thương mại", "miễn thuế", "hoàn thuế", "hóa đơn",
	"quản lý thuế", "thuế giá trị gia tăng", "thuế tiêu thụ đặc biệt", "thuế bảo vệ môi trường",
	"xử phạt vi phạm hành chính", "ngoại hối", "thương mại điện tử", "chất lượng sản phẩm, hàng hóa",
	"an toàn thực phẩm", "nhãn hàng hóa", "khu kinh tế", "bưu chính", "chuyển phát"}

var provincial = []string{"ubnd", "ủy ban nhân dân", "uỷ ban nhân dân", "hđnd", "hội đồng nhân dân"}

// Cues returns the cues found in text, in the order of the list.
func Cues(text string, cues []string) []string {
	t := strings.ToLower(text)
	var out []string
	for _, c := range cues {
		if strings.Contains(t, c) {
			out = append(out, c)
		}
	}
	return out
}

// Provincial reports whether a body is a people's committee or council. A
// provincial act can be about customs and still not be customs law.
func Provincial(issuingBody string) bool {
	b := strings.ToLower(issuingBody)
	for _, p := range provincial {
		if strings.Contains(b, p) {
			return true
		}
	}
	return false
}

// Tiering is the tier of one document and the evidence for it.
type Tiering struct {
	Tier int      `json:"tier"`
	Why  []string `json:"why"`
}

// TierOf places a document by its subjects and its title. Subjects must be
// sorted.
func TierOf(m *Meta, subjects []string) Tiering {
	var core []string
	for _, s := range subjects {
		if slices.Contains(coreSubjects, s) {
			core = append(core, s)
		}
	}
	coreCues := Cues(m.Title, CoreCues)
	local := Provincial(m.IssuingBody)
	if (len(core) > 0 || len(coreCues) > 0) && !local {
		return Tiering{Tier1, concat(core, head(coreCues, 3))}
	}
	nearCues := Cues(m.Title, NearCues)
	if len(core) > 0 || len(coreCues) > 0 || nearSubject(subjects) || len(nearCues) > 0 {
		cues := coreCues
		if len(cues) == 0 {
			cues = nearCues
		}
		return Tiering{Tier2, concat(head(subjects, 2), head(cues, 3))}
	}
	return Tiering{Tier3, []string{}}
}

func nearSubject(subjects []string) bool {
	for _, s := range subjects {
		for _, n := range nearSubjects {
			if s == n || strings.HasPrefix(s, n+"/") {
				return true
			}
		}
	}
	return false
}

// PromoteCited is how many tier 1 documents must cite a tier 3 national act
// before it is lifted to tier 2. Two is the smallest count that is not one
// document's habit.
const PromoteCited = 2

func head(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func concat(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	return append(append(out, a...), b...)
}
