package bookmaker

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// Giới hạn khi nạp PDF: chặn file lạ cỡ lớn làm treo / ngốn bộ nhớ.
const (
	maxPDFBytes     = 100 << 20
	maxPDFPages     = 3000
	maxPDFTextBytes = 16 << 20
)

// ErrPDFNoText — PDF không có lớp chữ (thường là bản quét ảnh).
var ErrPDFNoText = errors.New("PDF này không có chữ để đọc (có thể là bản quét ảnh). Sano chưa nhận dạng được chữ trong ảnh — hãy dùng PDF có thể bôi chọn chữ, hoặc chuyển sang Word")

// pdfChapterRe — dòng đầu chương: "Chương 1", "Chapter II", "Phần 3"... (đoạn ngắn, mở đầu bằng từ khoá).
var pdfChapterRe = regexp.MustCompile(`(?i)^(chương|chuong|chapter|phần|phan|part)\s+([0-9]+|[ivxlc]+)\b`)

// pdfLine — một dòng chữ của trang kèm vị trí dọc (để biết chỗ ngắt đoạn).
type pdfLine struct {
	text string
	y    float64
}

// ParsePDF đọc file .pdf có lớp chữ thành Book: chương theo dòng "Chương N" /
// "Chapter N" (không có thì cả file là một chương lấy tên file làm tên). Bỏ
// đầu trang / chân trang lặp lại và số trang; dòng cùng đoạn được nối lại.
func ParsePDF(path string) (book *Book, err error) {
	defer func() { // thư viện đọc PDF có thể panic với file hỏng
		if r := recover(); r != nil {
			book, err = nil, fmt.Errorf("không đọc được file PDF (file hỏng?): %v", r)
		}
	}()
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("đọc file: %w", err)
	}
	if info.Size() > maxPDFBytes {
		return nil, fmt.Errorf("file PDF quá lớn (hơn %d MB)", maxPDFBytes>>20)
	}
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("không mở được file PDF (file hỏng hoặc có mật khẩu): %w", err)
	}
	defer f.Close()
	if r.NumPage() > maxPDFPages {
		return nil, fmt.Errorf("file PDF quá dài (hơn %d trang)", maxPDFPages)
	}

	pages := make([][]pdfLine, 0, r.NumPage())
	total := 0
	for i := 1; i <= r.NumPage(); i++ {
		rows, perr := r.Page(i).GetTextByRow()
		if perr != nil {
			continue // trang lỗi / trang ảnh: bỏ qua, các trang khác vẫn đọc
		}
		sort.SliceStable(rows, func(a, b int) bool { return rows[a].Position < rows[b].Position })
		var lines []pdfLine
		for _, row := range rows {
			var sb strings.Builder
			for _, t := range row.Content {
				sb.WriteString(t.S)
			}
			if s := strings.TrimSpace(sb.String()); s != "" {
				total += len(s)
				lines = append(lines, pdfLine{text: s, y: float64(row.Position)})
			}
		}
		if total > maxPDFTextBytes {
			return nil, fmt.Errorf("nội dung PDF quá dài (hơn %d MB chữ)", maxPDFTextBytes>>20)
		}
		pages = append(pages, lines)
	}
	if total == 0 {
		return nil, ErrPDFNoText
	}

	paras := pdfParagraphs(dropRepeatedPDFLines(pages))
	book = chaptersFromParagraphs(paras, titleFromDocxName(path))
	if len(book.Chapters) == 0 {
		return nil, ErrPDFNoText
	}
	assignStems(book)
	return book, nil
}

// pdfKey — dạng so khớp đầu/chân trang: chữ thường, bỏ số (số trang đổi mỗi trang).
func pdfKey(s string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)), " ")
}

// dropRepeatedPDFLines bỏ dòng đầu / cuối trang lặp lại ở ≥ nửa số trang (đầu
// trang, chân trang, số trang) và dòng chỉ gồm số ở đầu / cuối trang.
func dropRepeatedPDFLines(pages [][]pdfLine) [][]pdfLine {
	count := map[string]int{}
	for _, p := range pages {
		seen := map[string]bool{}
		for _, idx := range edgeIdx(len(p)) {
			if k := pdfKey(p[idx].text); k != "" && !seen[k] {
				seen[k] = true
				count[k]++
			}
		}
	}
	repeated := func(s string) bool {
		k := pdfKey(s)
		return len(pages) >= 3 && k != "" && count[k]*2 >= len(pages)
	}
	pureNumber := func(s string) bool { return pdfKey(s) == "" }
	out := make([][]pdfLine, len(pages))
	for i, p := range pages {
		edge := map[int]bool{}
		for _, idx := range edgeIdx(len(p)) {
			edge[idx] = true
		}
		for j, ln := range p {
			if edge[j] && (repeated(ln.text) || pureNumber(ln.text)) {
				continue
			}
			out[i] = append(out[i], ln)
		}
	}
	return out
}

// edgeIdx — chỉ số 2 dòng đầu và 2 dòng cuối của trang (nơi đầu / chân trang nằm).
func edgeIdx(n int) []int {
	var idx []int
	for _, i := range []int{0, 1, n - 2, n - 1} {
		if i >= 0 && i < n && (len(idx) == 0 || idx[len(idx)-1] != i) {
			idx = append(idx, i)
		}
	}
	return idx
}

// pdfParagraphs nối các dòng thành đoạn: khoảng cách dọc lớn hơn bình thường →
// đoạn mới; đoạn đang dở ở cuối trang (không kết thúc câu) nối sang trang sau.
func pdfParagraphs(pages [][]pdfLine) []string {
	var paras []string
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			paras = append(paras, s)
		}
		cur.Reset()
	}
	for _, p := range pages {
		gaps := make([]float64, 0, len(p))
		for i := 1; i < len(p); i++ {
			if g := p[i].y - p[i-1].y; g > 0 {
				gaps = append(gaps, g)
			}
		}
		line := medianFloat(gaps)
		for i, ln := range p {
			newPara := false
			switch {
			case cur.Len() == 0:
			case i == 0:
				newPara = endsSentence(cur.String())
			default:
				newPara = line > 0 && ln.y-p[i-1].y > line*1.35
			}
			if pdfChapterRe.MatchString(ln.text) && utf8.RuneCountInString(ln.text) <= 120 {
				newPara = true
			}
			if newPara {
				flush()
			}
			if cur.Len() > 0 {
				if s := cur.String(); strings.HasSuffix(s, "-") && hyphenBreak(s, ln.text) {
					// "busi-" + "ness": ngắt từ cuối dòng → bỏ gạch nối, nối liền
					cur.Reset()
					cur.WriteString(strings.TrimSuffix(s, "-"))
				} else {
					cur.WriteByte(' ')
				}
			}
			cur.WriteString(ln.text)
		}
	}
	flush()
	return paras
}

func endsSentence(s string) bool {
	s = strings.TrimRight(s, " \"”’)]»")
	return s != "" && strings.ContainsRune(".!?…:;", []rune(s)[len([]rune(s))-1])
}

// hyphenBreak — "…chữ-" + "chữ thường…": gạch nối ngắt từ ở cuối dòng.
func hyphenBreak(prev, next string) bool {
	pr := []rune(prev)
	nr, _ := utf8.DecodeRuneInString(next)
	return len(pr) >= 2 && unicode.IsLetter(pr[len(pr)-2]) && unicode.IsLower(nr)
}

func medianFloat(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	return v[len(v)/2]
}

// chaptersFromParagraphs gom đoạn thành chương theo dòng "Chương N"; phần trước
// chương đầu tiên chỉ giữ khi đủ dài (trang bìa / tên tác giả thì bỏ). Không có
// dòng chương nào → cả file là một chương mang tên title.
func chaptersFromParagraphs(paras []string, title string) *Book {
	if title == "" {
		title = "Nội dung"
	}
	book := &Book{Title: title}
	var curTitle string
	var body []string
	open := false
	flush := func() {
		text := strings.Join(body, "\n")
		if strings.TrimSpace(text) != "" || open {
			if curTitle == "" {
				curTitle = title
			}
			book.Chapters = append(book.Chapters, Chapter{Title: curTitle, Sections: []Section{{Title: curTitle, Text: text}}})
		}
		body, curTitle, open = nil, "", false
	}
	for _, p := range paras {
		if pdfChapterRe.MatchString(p) && utf8.RuneCountInString(p) <= 120 {
			if len(book.Chapters) == 0 && !open && utf8.RuneCountInString(strings.Join(body, "")) < 400 {
				body = nil // trang bìa, mục lục ngắn trước chương 1
			} else {
				flush()
			}
			curTitle, open = p, true
			continue
		}
		body = append(body, p)
	}
	flush()
	return book
}
