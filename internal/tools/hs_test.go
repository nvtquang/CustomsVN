package tools

import "testing"

func TestTreeByCode(t *testing.T) {
	all := []HSLine{
		{Position: 0, Code: "87.03", Level: 0, DescriptionVI: "Ô tô"},
		{Position: 1, Code: "8703.22.90", Level: 1, DescriptionVI: "Loại khác"},
		// The source lost the dashes here: the subheading looks deeper than
		// the line above it, which is not its parent.
		{Position: 2, Code: "8703.23", Level: 2, DescriptionVI: "Dung tích xilanh trên 1.500 cc"},
		{Position: 3, Code: "", Level: 3, DescriptionVI: "Dạng CKD:"},
		{Position: 4, Code: "8703.23.61", Level: 1, DescriptionVI: "Loại khác"},
		{Position: 5, Code: "8703.24", Level: 2, DescriptionVI: "Dung tích trên 3.000 cc"},
	}
	l := with(all, 4, false)
	if len(l.Path) != 2 || l.Path[0].Code != "87.03" || l.Path[1].Code != "8703.23" {
		t.Errorf("path %+v", l.Path)
	}
	h := with(all, 2, true)
	if len(h.Path) != 1 || h.Path[0].Code != "87.03" {
		t.Errorf("path %+v", h.Path)
	}
	if len(h.Children) != 2 || h.Children[1].Code != "8703.23.61" {
		t.Errorf("children %+v", h.Children)
	}
}
