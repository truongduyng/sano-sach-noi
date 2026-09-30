package bookmaker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// MaxPlainTextBytes — giới hạn file .txt nạp trực tiếp.
const MaxPlainTextBytes = 8 << 20

// IsSupportedInput — đuôi file Sano nhận làm tài liệu: .docx hoặc .txt.
func IsSupportedInput(path string) bool {
	ext := filepath.Ext(path)
	return strings.EqualFold(ext, ".docx") || strings.EqualFold(ext, ".txt")
}

// ParseInput nạp .txt (một đoạn văn liền mạch) hoặc .docx (nhiều cấp).
func ParseInput(path string) (*Book, error) {
	if strings.EqualFold(filepath.Ext(path), ".txt") {
		return ParseText(path)
	}
	return ParseDocx(path)
}

// ParseText đọc thẳng file .txt: cả file là một đoạn văn bản, một chương một
// tiểu mục lấy tên file làm tên. Không tìm cấu trúc sách trong file.
func ParseText(path string) (*Book, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("đọc file: %w", err)
	}
	if info.Size() > MaxPlainTextBytes {
		return nil, fmt.Errorf("file quá dài (hơn %d MB)", MaxPlainTextBytes>>20)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("đọc file: %w", err)
	}
	if !utf8.Valid(raw) {
		return nil, errors.New("file .txt không phải mã hoá UTF-8, hãy mở bằng Notepad rồi lưu lại với mã hoá UTF-8")
	}
	text := strings.TrimPrefix(string(raw), "\ufeff")
	var paras []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if l := strings.TrimSpace(line); l != "" {
			paras = append(paras, l)
		}
	}
	if len(paras) == 0 {
		return nil, errors.New("file .txt trống")
	}
	title := titleFromDocxName(path)
	if title == "" {
		title = "Nội dung"
	}
	book := &Book{Title: title, Chapters: []Chapter{{Title: title, Sections: []Section{{Title: title, Text: strings.Join(paras, "\n")}}}}}
	assignStems(book)
	return book, nil
}
