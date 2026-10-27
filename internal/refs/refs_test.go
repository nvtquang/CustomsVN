package refs

import (
	"slices"
	"strings"
	"testing"
)

func TestParseProvisionWithName(t *testing.T) {
	f := Parse("Theo điểm b khoản 1 Điều 18 Luật Hải quan, người khai hải quan có quyền gì?")
	if len(f.Provisions) != 1 {
		t.Fatalf("got %+v", f.Provisions)
	}
	p := f.Provisions[0]
	if p.Point != "b" || p.Clause != "1" || p.Article != "18" || !strings.HasPrefix(NameKey(p.Name), "luat-hai-quan") || p.Number != "" {
		t.Errorf("got %+v", p)
	}
	if p.Tail() != "article-18:clause-1:point-b" {
		t.Errorf("tail %q", p.Tail())
	}
}

func TestParseProvisionWithNumber(t *testing.T) {
	for _, q := range []string{
		"khoản 2 Điều 25 Thông tư 38/2015/TT-BTC",
		"khoản 2 Điều 25 của Thông tư số 38/2015/TT-BTC",
		"khoản 2, Điều 25 38/2015/TT-BTC",
	} {
		f := Parse(q)
		if len(f.Provisions) != 1 || f.Provisions[0].Number != "38/2015/TT-BTC" || f.Provisions[0].Clause != "2" {
			t.Errorf("%q: got %+v", q, f.Provisions)
		}
	}
}

func TestParseNameThenNumber(t *testing.T) {
	f := Parse("Điều 4 Luật Hải quan số 54/2014/QH13 giải thích từ ngữ")
	p := f.Provisions[0]
	if p.Name != "Luật Hải quan" || p.Number != "54/2014/QH13" {
		t.Errorf("got %+v", p)
	}
}

func TestParsePointDd(t *testing.T) {
	p := Parse("điểm đ khoản 3 Điều 7").Provisions[0]
	if p.Tail() != "article-7:clause-3:point-dd" {
		t.Errorf("tail %q", p.Tail())
	}
}

func TestParseNumbersCodesDates(t *testing.T) {
	f := Parse("Nghị định 08/2015/NĐ-CP và 47/VBHN-BCT áp dụng từ ngày 1 tháng 7 năm 2025 (01/07/2025) cho mã 8703.23.61 và mã HS 870324, hạn 1/7")
	if !slices.Equal(f.Numbers, []string{"08/2015/NĐ-CP", "47/VBHN-BCT"}) {
		t.Errorf("numbers %v", f.Numbers)
	}
	if !slices.Equal(f.HSCodes, []string{"8703.23.61", "8703.24"}) {
		t.Errorf("hs %v", f.HSCodes)
	}
	if !slices.Equal(f.Dates, []string{"2025-07-01"}) {
		t.Errorf("dates %v", f.Dates)
	}
}

func TestParseNothing(t *testing.T) {
	if f := Parse("Thủ tục hải quan đối với hàng hóa gia công là gì?"); !f.Empty() {
		t.Errorf("got %+v", f)
	}
}

func TestNameKey(t *testing.T) {
	if NameKey("LUẬT HẢI QUAN") != "luat-hai-quan" || NameKey("Luật Hải quan năm 2014") != "luat-hai-quan" {
		t.Errorf("%q %q", NameKey("LUẬT HẢI QUAN"), NameKey("Luật Hải quan năm 2014"))
	}
}

func TestParseNameWithComma(t *testing.T) {
	p := Parse("Điều 16 Luật Thuế xuất khẩu, thuế nhập khẩu quy định miễn thuế").Provisions[0]
	if !strings.HasPrefix(NameKey(p.Name), "luat-thue-xuat-khau-thue-nhap-khau") {
		t.Errorf("name %q", p.Name)
	}
}
