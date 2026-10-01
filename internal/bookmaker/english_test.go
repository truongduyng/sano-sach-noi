package bookmaker

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Bảng giọng trong kokoro_gen_batch.py phải khớp englishVoices (tên, id, mô tả, nổi bật).
func TestEnglishVoices_KhopScriptPython(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/tts/kokoro_gen_batch.py")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\("([^"]+)", "([^"]+)", "([^"]+)", (True|False)\)`)
	got := re.FindAllStringSubmatch(string(raw), -1)
	if len(got) != len(englishVoices) {
		t.Fatalf("script có %d giọng, Go có %d", len(got), len(englishVoices))
	}
	for i, m := range got {
		v := englishVoices[i]
		if m[1] != v.Name || m[2] != v.ID || m[3] != v.Desc || (m[4] == "True") != v.Featured {
			t.Errorf("giọng %d lệch: script %v, Go %+v", i, m[1:], v)
		}
	}
	if !IsEnglishVoice(DefaultEnglishVoice) {
		t.Errorf("giọng mặc định %q không có trong danh sách", DefaultEnglishVoice)
	}
}

func TestLangOfVoice(t *testing.T) {
	for voice, want := range map[string]string{
		"Heart": LangEN, "heart": LangEN, "af_heart": LangEN, " Emma ": LangEN,
		DefaultVoice: LangVI, "Trúc Ly": LangVI, "": LangVI,
	} {
		if got := LangOfVoice(voice); got != want {
			t.Errorf("LangOfVoice(%q) = %s, muốn %s", voice, got, want)
		}
	}
}

// Không giọng Anh nào trùng tên giọng Việt: định tuyến bộ đọc theo tên giọng.
func TestEnglishVoices_KhongTrungGiongViet(t *testing.T) {
	for _, v := range parseVoices("   • ⭐ Hải Đăng — Nam · Bắc\n   • Trúc Ly — Nữ · Bắc\n") {
		if IsEnglishVoice(v.Name) {
			t.Errorf("%q bị coi là giọng tiếng Anh", v.Name)
		}
	}
	// và --list-voices của script Kokoro đọc được bằng cùng bộ phân tích
	var lines []string
	for _, v := range EnglishVoices() {
		star := ""
		if v.Featured {
			star = "⭐ "
		}
		lines = append(lines, "   • "+star+v.Name+" — "+v.Desc)
	}
	back := parseVoices(strings.Join(lines, "\n"))
	if len(back) != len(englishVoices) || back[0].Name != "Heart" || !back[0].Featured {
		t.Errorf("parseVoices không đọc được danh sách giọng Anh: %+v", back)
	}
}

func TestEnglishNormalizer(t *testing.T) {
	n := NewEnglishNormalizer(false, map[string]string{"SQL": "sequel"})
	in := "Chapter IV\n1. Buy milk\n- Dr. Smith said “no” & left.\nSee SQL docs, 3/4 of them.\n12"
	got := n.script(in)
	for _, want := range []string{"Chapter IV", "First, Buy milk", "Dr. Smith said “no” & left.", "See sequel docs, 3/4 of them."} {
		if !strings.Contains(got, want) {
			t.Errorf("thiếu %q trong %q", want, got)
		}
	}
	for _, bad := range []string{"Thứ", "và", "ba", "Chương"} {
		if strings.Contains(got, bad) {
			t.Errorf("luật tiếng Việt lọt vào sách Anh (%q): %q", bad, got)
		}
	}
	if strings.Contains(got, "\n12") {
		t.Errorf("dòng số trang chưa bị bỏ: %q", got)
	}
	// tiêu đề: bỏ số mục đầu, giữ nguyên khi giữ số
	if got := n.spokenTitle("1.2 Getting started"); got != "Getting started" {
		t.Errorf("spokenTitle = %q", got)
	}
	if got := NewEnglishNormalizer(true).spokenTitle("1.2 Getting started"); got != "1.2 Getting started" {
		t.Errorf("spokenTitle (giữ số) = %q", got)
	}
	// bộ chuẩn Việt không đổi hành vi
	if got := normalizeReadingScript("1. Mua sữa"); !strings.HasPrefix(got, "Thứ nhất") {
		t.Errorf("bộ chuẩn tiếng Việt bị đổi: %q", got)
	}
}

func TestNarratorLabel_English(t *testing.T) {
	if got := narratorLabel(TTSConfig{Voice: "Heart"}); got != "Kokoro-82M (Heart)" {
		t.Errorf("narratorLabel = %q", got)
	}
	if got := narratorLabel(TTSConfig{Voice: DefaultVoice}); got != "VieNeu-TTS (Hải Đăng)" {
		t.Errorf("narratorLabel = %q", got)
	}
}
