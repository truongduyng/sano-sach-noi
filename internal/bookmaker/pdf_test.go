package bookmaker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testdata/book.pdf: trang bìa + "Chương 1. Khởi đầu" + "Chương 2. Dòng tiền", mỗi chương một trang, tiếng Việt.
func TestParsePDF_ChuongTheoDongChuong(t *testing.T) {
	b, err := ParseInput("testdata/book.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Chapters) != 2 {
		t.Fatalf("muốn 2 chương (trang bìa bị bỏ), có %d: %+v", len(b.Chapters), b.Chapters)
	}
	for i, want := range []string{"Chương 1. Khởi đầu", "Chương 2. Dòng tiền"} {
		c := b.Chapters[i]
		if c.Title != want || len(c.Sections) != 1 || c.Sections[0].Stem == "" {
			t.Errorf("chương %d = %q (%d mục, stem %q)", i, c.Title, len(c.Sections), c.Sections[0].Stem)
		}
		txt := c.Sections[0].Text
		if !strings.Contains(txt, "Mọi mô hình kinh doanh đều bắt đầu từ một bài toán thực tế") {
			t.Errorf("chương %d mất chữ tiếng Việt: %.80q", i, txt)
		}
		// dòng bị ngắt cuối dòng đã nối lại, không còn "phù\nhợp"
		if strings.Contains(txt, "phù hợp và") == false {
			t.Errorf("chương %d chưa nối dòng: %.200q", i, txt)
		}
	}
}

func TestParsePDF_Loi(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "hong.pdf")
	_ = os.WriteFile(bad, []byte("không phải PDF"), 0o644)
	if _, err := ParseInput(bad); err == nil {
		t.Error("file hỏng phải báo lỗi (không panic)")
	}
	if !IsSupportedInput("a.PDF") || IsSupportedInput("a.doc") {
		t.Error("IsSupportedInput sai với .pdf / .doc")
	}
}

func TestChaptersFromParagraphs_KhongCoChuong(t *testing.T) {
	b := chaptersFromParagraphs([]string{"Đoạn một.", "Đoạn hai."}, "Sách thử")
	if len(b.Chapters) != 1 || b.Chapters[0].Title != "Sách thử" || b.Chapters[0].Sections[0].Text != "Đoạn một.\nĐoạn hai." {
		t.Errorf("%+v", b.Chapters)
	}
}

func TestDropRepeatedPDFLines(t *testing.T) {
	mk := func(body string) []pdfLine {
		return []pdfLine{{"Tên sách – Tác giả", 10}, {body, 50}, {"12", 90}}
	}
	out := dropRepeatedPDFLines([][]pdfLine{mk("a"), mk("b"), mk("c")})
	for i, p := range out {
		if len(p) != 1 {
			t.Errorf("trang %d còn %v, muốn chỉ thân bài", i, p)
		}
	}
}

func TestPDFParagraphs_NgatTuVaNoiTrang(t *testing.T) {
	got := pdfParagraphs([][]pdfLine{
		{{"Đây là câu về busi-", 10}, {"ness và tiếp tục", 26}, {"Đoạn mới sau khoảng trống lớn", 80}, {"nhưng câu này chưa hết", 96}},
		{{"mà tiếp sang trang sau.", 10}},
	})
	want := []string{"Đây là câu về business và tiếp tục", "Đoạn mới sau khoảng trống lớn nhưng câu này chưa hết mà tiếp sang trang sau."}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %q", got)
	}
}
