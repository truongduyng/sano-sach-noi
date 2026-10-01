package bookmaker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTextIsOnePassage(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bai-tho_mot.txt")
	if err := os.WriteFile(p, []byte("\ufeff# không phải tiêu đề\r\nChương 1 cũng vậy\r\n\r\nĐoạn hai."), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := ParseInput(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Chapters) != 1 || len(b.Chapters[0].Sections) != 1 || b.Title != "bai tho mot" {
		t.Fatalf("cần đúng một chương một mục: %+v", b)
	}
	if got := b.Chapters[0].Sections[0].Text; got != "# không phải tiêu đề\nChương 1 cũng vậy\nĐoạn hai." {
		t.Errorf("text = %q", got)
	}
	bad := filepath.Join(t.TempDir(), "x.txt")
	_ = os.WriteFile(bad, []byte{0xff, 0xfe}, 0o644)
	if _, err := ParseInput(bad); err == nil {
		t.Error("cần lỗi khi không phải UTF-8")
	}
}
