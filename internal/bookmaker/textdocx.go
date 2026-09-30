package bookmaker

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxPastedTextBytes — giới hạn văn bản dán (≈ vài cuốn sách dày), tránh dán nhầm
// cả khối dữ liệu khổng lồ làm treo app.
const MaxPastedTextBytes = 8 << 20

// ErrNoChapterLine — văn bản dán không có dòng chương nào ("# Tên chương").
var ErrNoChapterLine = errors.New("không thấy dòng chương nào: mỗi chương cần một dòng bắt đầu bằng dấu # (ví dụ \"# Chương 1. Tên chương\")")

// TextToDocx đổi văn bản AI trả về (cấp 2/3, lối dự phòng khi AI không tạo được
// file Word) thành .docx để đi tiếp luồng nạp file cũ. Quy ước dòng:
//
//	"% Tên sách"  → Title
//	"# Chương"    → Heading1
//	"## Mục"      → Heading2 (### trở xuống cũng coi là mục)
//	dòng khác     → một đoạn văn thường
//
// Lời chào hỏi AI viết trước dòng %/# đầu tiên bị bỏ; dòng rào khối mã (```),
// dấu in đậm ** và dấu gạch đầu dòng bị bỏ để không bị đọc thành tiếng.
func TextToDocx(text string) ([]byte, error) {
	if len(text) > MaxPastedTextBytes {
		return nil, fmt.Errorf("văn bản quá dài (hơn %d MB)", MaxPastedTextBytes>>20)
	}
	if !utf8.ValidString(text) {
		return nil, errors.New("văn bản có ký tự lỗi mã hoá, hãy sao chép lại từ AI")
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	para := func(style, s string) {
		b.WriteString(`<w:p>`)
		if style != "" {
			b.WriteString(`<w:pPr><w:pStyle w:val="` + style + `"/></w:pPr>`)
		}
		b.WriteString(`<w:r><w:t xml:space="preserve">` + xmlEscape(s) + `</w:t></w:r></w:p>`)
	}

	started, chapters := false, 0
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		style, body := classifyLine(line)
		if !started {
			if style == "" {
				continue // lời chào / giải thích của AI trước nội dung sách
			}
			started = true
		}
		body = cleanInline(body)
		if body == "" {
			continue
		}
		if style == "Heading1" {
			chapters++
		}
		para(style, body)
	}
	if chapters == 0 {
		return nil, ErrNoChapterLine
	}
	b.WriteString(`</w:body></w:document>`)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range []struct{ name, data string }{
		{"[Content_Types].xml", textDocxContentTypes},
		{"_rels/.rels", sampleRootRels},
		{"word/document.xml", b.String()},
	} {
		fw, err := zw.Create(e.name)
		if err != nil {
			return nil, fmt.Errorf("zip create %q: %w", e.name, err)
		}
		if _, err := fw.Write([]byte(e.data)); err != nil {
			return nil, fmt.Errorf("zip write %q: %w", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("zip close: %w", err)
	}
	return buf.Bytes(), nil
}

// classifyLine trả kiểu đoạn theo dấu đầu dòng và phần chữ còn lại.
func classifyLine(line string) (style, body string) {
	switch {
	case strings.HasPrefix(line, "%"):
		return "Title", strings.TrimSpace(strings.TrimLeft(line, "%"))
	case strings.HasPrefix(line, "##"):
		return "Heading2", strings.TrimSpace(strings.TrimLeft(line, "#"))
	case strings.HasPrefix(line, "#"):
		return "Heading1", strings.TrimSpace(strings.TrimLeft(line, "#"))
	}
	return "", line
}

// cleanInline bỏ dấu định dạng markdown hay gặp trong câu trả lời của AI.
func cleanInline(s string) string {
	for _, p := range []string{"- ", "* ", "• "} {
		if strings.HasPrefix(s, p) {
			s = strings.TrimSpace(s[len(p):])
			break
		}
	}
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	return strings.TrimSpace(s)
}

const textDocxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`

// chapterLineRe nhận dòng đầu chương trong file .txt không có dấu #.
var chapterLineRe = regexp.MustCompile(`(?i)^(chương|chuong|chapter|phần|phan|part)\s+([0-9]+|[ivxlc]+)\b`)

// PlainTextToDocx đổi nội dung file .txt người dùng chọn thành .docx tạm.
// File đã có dòng "#" / "%" thì theo đúng quy ước của TextToDocx. Không có thì
// dòng "Chương N …" / "Chapter N …" là đầu chương; vẫn không có thì cả file là
// một chương lấy tên file (title) làm tên.
func PlainTextToDocx(text, title string) ([]byte, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	if len(text) > MaxPastedTextBytes {
		return nil, fmt.Errorf("file quá dài (hơn %d MB)", MaxPastedTextBytes>>20)
	}
	if !utf8.ValidString(text) {
		return nil, errors.New("file .txt không phải mã hoá UTF-8, hãy mở bằng Notepad rồi lưu lại với mã hoá UTF-8")
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	marked, chapters := false, 0
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "%") {
			marked = true
			break
		}
		if chapterLineRe.MatchString(line) {
			chapters++
		}
	}
	if marked {
		return TextToDocx(text)
	}
	var b strings.Builder
	if chapters == 0 {
		title = strings.TrimSpace(title)
		if title == "" {
			title = "Nội dung"
		}
		b.WriteString("# " + title + "\n")
	}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if chapters > 0 && chapterLineRe.MatchString(line) {
			b.WriteString("# " + line + "\n")
		} else if line != "" {
			// đoạn văn thường: bỏ dấu đầu dòng markdown giả để không bị coi là tiêu đề
			b.WriteString(line + "\n")
		}
	}
	return TextToDocx(b.String())
}
