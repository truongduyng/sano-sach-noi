package bookmaker

import (
	"regexp"
	"strconv"
	"strings"
)

// Ngôn ngữ của sách (Options.Lang, outMeta.Language).
const (
	LangVI = "vi"
	LangEN = "en"
)

// DefaultEnglishVoice — giọng mặc định cho sách tiếng Anh.
const DefaultEnglishVoice = "Heart"

// englishVoices — giọng Kokoro-82M Sano cho dùng (id trong model → tên hiện ra).
// Chỉ giữ giọng tiếng Anh xếp loại C trở lên trong bảng của Kokoro; bảng cùng
// tên/id nằm trong scripts/tts/kokoro_gen_batch.py (test khớp hai bên).
var englishVoices = []struct {
	Name, ID, Desc string
	Featured       bool
}{
	{"Heart", "af_heart", "Nữ · Mỹ · Ấm, tự nhiên (chất lượng cao nhất)", true},
	{"Bella", "af_bella", "Nữ · Mỹ · Rõ, biểu cảm", true},
	{"Nicole", "af_nicole", "Nữ · Mỹ · Nhẹ, thì thầm", false},
	{"Aoede", "af_aoede", "Nữ · Mỹ", false},
	{"Kore", "af_kore", "Nữ · Mỹ", false},
	{"Sarah", "af_sarah", "Nữ · Mỹ", false},
	{"Michael", "am_michael", "Nam · Mỹ · Trầm, vững", true},
	{"Fenrir", "am_fenrir", "Nam · Mỹ · Mạnh", false},
	{"Puck", "am_puck", "Nam · Mỹ", false},
	{"Emma", "bf_emma", "Nữ · Anh · Điềm đạm", true},
	{"George", "bm_george", "Nam · Anh · Trầm, kể chuyện", true},
	{"Fable", "bm_fable", "Nam · Anh", false},
}

// EnglishVoices trả danh sách giọng tiếng Anh (cùng dạng ListVoices của bộ đọc Việt).
func EnglishVoices() []Voice {
	out := make([]Voice, len(englishVoices))
	for i, v := range englishVoices {
		out[i] = Voice{Name: v.Name, Desc: v.Desc, Featured: v.Featured}
	}
	return out
}

// IsEnglishVoice — tên giọng thuộc bộ đọc tiếng Anh (không phân biệt hoa thường).
func IsEnglishVoice(name string) bool {
	name = strings.TrimSpace(name)
	for _, v := range englishVoices {
		if strings.EqualFold(v.Name, name) || strings.EqualFold(v.ID, name) {
			return true
		}
	}
	return false
}

// LangOfVoice — ngôn ngữ của giọng: các giọng Kokoro là tiếng Anh, còn lại tiếng Việt.
func LangOfVoice(name string) string {
	if IsEnglishVoice(name) {
		return LangEN
	}
	return LangVI
}

// NewEnglishNormalizer dựng bộ chuẩn hoá cho sách tiếng Anh: KHÔNG dùng luật
// tiếng Việt (viết tắt, số La Mã, "Thứ nhất", ngoặc kép, "&"→"và"), từ điển
// cách đọc chỉ gồm lớp người dùng (bộ chuẩn nhúng sẵn là cho tiếng Việt).
func NewEnglishNormalizer(keepHeadingNumbers bool, layers ...map[string]string) *Normalizer {
	d := &pronunciationDict{entries: map[string]string{}}
	d.overlay(layers...)
	return &Normalizer{dict: d, keepHeadingNumbers: keepHeadingNumbers, english: true}
}

// scriptEN — kịch bản đọc cho sách tiếng Anh: dọn ký tự lạ, dòng số trang, ký
// hiệu danh sách, áp từ điển người dùng, gộp khoảng trắng. Giữ nguyên dấu câu
// và ngoặc kép (bộ đọc Kokoro tự xử lý).
func (n *Normalizer) scriptEN(original string) string {
	s := controlCharRe.ReplaceAllString(original, "")
	s = dropPageNumberLines(s)
	s = englishListMarkerLines(s)
	s = n.dict.apply(s)
	return strings.TrimSpace(collapseSpacesKeepLines(s))
}

var englishOrdinals = []string{"First", "Second", "Third", "Fourth", "Fifth", "Sixth", "Seventh", "Eighth", "Ninth", "Tenth"}

var englishOrdinalStartRe = regexp.MustCompile(`(?i)^(first|second|third|fourth|fifth|sixth|seventh|eighth|ninth|tenth)\b`)

// englishListMarkerLines bỏ ký hiệu danh sách; danh sách đánh số 1–10 đọc "First, …".
func englishListMarkerLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if m := numberedLineRe.FindStringSubmatch(ln); m != nil {
			n, _ := strconv.Atoi(m[1])
			if englishOrdinalStartRe.MatchString(m[2]) || n < 1 || n > len(englishOrdinals) {
				lines[i] = m[2]
			} else {
				lines[i] = englishOrdinals[n-1] + ", " + m[2]
			}
			continue
		}
		if m := bulletLineRe.FindStringSubmatch(ln); m != nil {
			lines[i] = m[1]
			continue
		}
		if m := letteredLineRe.FindStringSubmatch(ln); m != nil {
			lines[i] = m[1]
		}
	}
	return strings.Join(lines, "\n")
}
