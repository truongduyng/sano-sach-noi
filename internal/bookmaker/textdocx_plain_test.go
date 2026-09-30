package bookmaker

import "testing"

func TestPlainTextToDocx(t *testing.T) {
	// dòng bắt đầu bằng # hay "Chương" vẫn chỉ là chữ thường của đoạn văn
	if _, err := PlainTextToDocx("\ufeff# không phải tiêu đề\r\nChương 1 cũng vậy\r\nĐoạn hai.", "Sách thử"); err != nil {
		t.Fatal(err)
	}
	if _, err := PlainTextToDocx("\xff\xfe", "x"); err == nil {
		t.Error("cần lỗi khi không phải UTF-8")
	}
	if _, err := PlainTextToDocx("  \n ", "x"); err == nil {
		t.Error("cần lỗi khi file trống")
	}
}
