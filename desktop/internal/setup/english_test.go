package setup

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	ttsscripts "sano/scripts/tts"

	"sano/desktop/internal/tts"
)

func TestInitialEnglishStatus_CoBuocKokoro(t *testing.T) {
	st := InitialEnglishStatus()
	var keys []string
	for _, s := range st.Steps {
		keys = append(keys, s.Key)
		if s.State != StatePending {
			t.Errorf("bước %s chưa chạy mà state = %s", s.Key, s.State)
		}
	}
	want := []string{StepPython, StepKokoro, StepKokoroModels, StepFFmpeg, StepVerify}
	if len(keys) != len(want) {
		t.Fatalf("các bước = %v, muốn %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("bước %d = %s, muốn %s", i, keys[i], want[i])
		}
	}
}

func TestNew_ChonBuocTheoPack(t *testing.T) {
	vi := New(Config{}).Status()
	en := New(Config{Pack: PackEnglish}).Status()
	if vi.Pack != "" || en.Pack != PackEnglish {
		t.Errorf("Status.Pack: vi=%q en=%q", vi.Pack, en.Pack)
	}
	hasKey := func(st Status, key string) bool {
		for _, s := range st.Steps {
			if s.Key == key {
				return true
			}
		}
		return false
	}
	if hasKey(vi, StepKokoro) || !hasKey(vi, StepVieNeu) {
		t.Error("lượt cài VieNeu phải có bước vieneu, không có kokoro")
	}
	if !hasKey(en, StepKokoro) || hasKey(en, StepVieNeu) {
		t.Error("lượt cài tiếng Anh phải có bước kokoro, không có vieneu")
	}
}

func TestEnglishReady(t *testing.T) {
	l := tts.NewLayout(t.TempDir(), runtime.GOOS)
	if EnglishReady(l) {
		t.Fatal("chưa cài mà EnglishReady = true")
	}
	for _, p := range []string{l.KokoroPython(), filepath.Join(l.KokoroModels(), tts.KokoroModelFile)} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if EnglishReady(l) {
		t.Fatal("thiếu file giọng mà EnglishReady = true")
	}
	if err := os.WriteFile(filepath.Join(l.KokoroModels(), tts.KokoroVoicesFile), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !EnglishReady(l) {
		t.Fatal("đủ file mà EnglishReady = false")
	}
}

// Tên file mô hình trong layout phải khớp URL ghim (script và Go cùng tìm đúng file).
func TestKokoroFiles_KhopPins(t *testing.T) {
	pins, err := ttsscripts.Pins()
	if err != nil {
		t.Fatal(err)
	}
	for k, name := range map[string]string{"KOKORO_MODEL_URL": tts.KokoroModelFile, "KOKORO_VOICES_URL": tts.KokoroVoicesFile} {
		if got := filepath.Base(pins[k]); got != name {
			t.Errorf("%s trỏ tới %s, layout dùng %s", k, got, name)
		}
	}
}

func TestKokoroLayout_TrongRoot(t *testing.T) {
	l := tts.NewLayout(t.TempDir(), "linux")
	for _, p := range []string{l.Kokoro(), l.KokoroVenv(), l.KokoroPython(), l.KokoroModels()} {
		if rel, err := filepath.Rel(l.Root, p); err != nil || rel == ".." || len(rel) > 2 && rel[:3] == "../" {
			t.Errorf("%s nằm ngoài %s (gỡ bộ đọc sẽ không xoá hết)", p, l.Root)
		}
	}
	found := false
	for _, kv := range l.KokoroEnv() {
		if kv == tts.EnvKokoroDir+"="+l.KokoroModels() {
			found = true
		}
	}
	if !found {
		t.Error("KokoroEnv thiếu SANO_KOKORO_DIR")
	}
}
