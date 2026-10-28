package corpus

import "strings"

// Relations an edge can stand for, after luatdo has turned every vbpl.vn label
// to run from the acting document to the one acted on.
const (
	RelAmends       = "amends"       // sửa đổi, bổ sung
	RelReplaces     = "replaces"     // thay thế
	RelRepeals      = "repeals"      // bãi bỏ
	RelExpires      = "expires"      // làm hết hiệu lực, toàn bộ hoặc một phần
	RelSuspends     = "suspends"     // ngưng, đình chỉ
	RelCorrects     = "corrects"     // đính chính
	RelDetails      = "details"      // quy định chi tiết, hướng dẫn thi hành
	RelConsolidates = "consolidates" // hợp nhất
	RelCites        = "cites"        // căn cứ, dẫn chiếu, liên quan
)

// relationOf maps a vbpl.vn label to its relation. Both wordings of a pair
// ("Văn bản sửa đổi" on one page, "Văn bản được sửa đổi" on the other) map to
// the same relation, because luatdo has already turned them the same way.
var relationOf = map[string]string{
	"Thay thế":                              RelReplaces,
	"Bãi bỏ":                                RelRepeals,
	"Sửa đổi, bổ sung":                      RelAmends,
	"Văn bản được sửa đổi":                  RelAmends,
	"Văn bản được bổ sung":                  RelAmends,
	"Văn bản sửa đổi":                       RelAmends,
	"Văn bản bổ sung":                       RelAmends,
	"Đính chính":                            RelCorrects,
	"Tạm ngưng hiệu lực":                    RelSuspends,
	"Đình chỉ thi hành":                     RelSuspends,
	"Văn bản bị đình chỉ":                   RelSuspends,
	"Văn bản bị đình chỉ 1 phần":            RelSuspends,
	"Văn bản đình chỉ":                      RelSuspends,
	"Văn bản đình chỉ 1 phần":               RelSuspends,
	"Văn bản hết hiệu lực":                  RelExpires,
	"Văn bản bị hết hiệu lực 1 phần":        RelExpires,
	"Văn bản quy định hết hiệu lực":         RelExpires,
	"Văn bản quy định hết hiệu lực 1 phần":  RelExpires,
	"Quy định chi tiết, hướng dẫn thi hành": RelDetails,
	"Văn bản được HD, QĐ chi tiết":          RelDetails,
	"Văn bản HD, QĐ chi tiết":               RelDetails,
	"Hướng dẫn áp dụng":                     RelDetails,
	"Hợp nhất":                              RelConsolidates,
}

// Relation is the relation of a vbpl.vn label; a label the vocabulary does
// not have is a citation, the weakest claim.
func Relation(label string) string {
	if r, ok := relationOf[strings.TrimSpace(label)]; ok {
		return r
	}
	return RelCites
}

// Acts reports whether a relation changes the force or wording of the
// document it points at, so it belongs on that document's timeline.
func Acts(relation string) bool {
	switch relation {
	case RelAmends, RelReplaces, RelRepeals, RelExpires, RelSuspends, RelCorrects, RelConsolidates:
		return true
	}
	return false
}
