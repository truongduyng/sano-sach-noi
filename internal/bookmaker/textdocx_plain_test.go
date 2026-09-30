package bookmaker

import "testing"

func TestPlainTextToDocx(t *testing.T) {
	for name, in := range map[string]string{
		"co-dau-thang":   "# Chương 1\nNội dung",
		"co-chuong":      "Chương 1. Mở đầu\nLời.\nChương 2. Tiếp\nLời nữa.",
		"khong-cau-truc": "\ufeffChỉ là một đoạn văn.\r\nĐoạn hai.",
	} {
		if _, err := PlainTextToDocx(in, "Sách thử"); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := PlainTextToDocx("\xff\xfe", "x"); err == nil {
		t.Error("cần lỗi khi không phải UTF-8")
	}
}
