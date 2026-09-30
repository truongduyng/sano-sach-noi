package setup

// Gói giọng tiếng Anh (Kokoro-82M): cài riêng khi người dùng chọn sách tiếng
// Anh, dùng lại uv + Python + ffmpeg của bộ đọc VieNeu (cài chưa có thì cài luôn).
// Venv riêng trong <tts>/kokoro/venv để không đụng venv VieNeu (uv sync --frozen
// của VieNeu xoá gói lạ); thư viện cài từ kokoro-requirements.txt có SHA256 từng
// gói (`uv pip install --require-hashes`), mô hình kiểm SHA256 như mô hình VieNeu.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"sano/desktop/internal/tts"
)

// PackEnglish — Config.Pack của lượt cài gói giọng tiếng Anh ("" = bộ đọc VieNeu).
const PackEnglish = "en"

// Khoá các dòng tiến độ riêng của gói tiếng Anh (giao diện dùng chung StepPython,
// StepFFmpeg, StepVerify với lượt cài VieNeu).
const (
	StepKokoro       = "kokoro"
	StepKokoroModels = "kokoro-models"
)

const (
	kokoroLibBytes       int64 = 150 << 20 // venv: onnxruntime, numpy, espeak-ng...
	KokoroModelBytes     int64 = 92_361_271 + 28_214_398
	EnglishRequiredBytes int64 = 900 << 20 // cần trống trước khi cài (cache uv + venv + mô hình + ffmpeg)
	maxKokoroFileBytes   int64 = 200 << 20
	kokoroVerifyWait           = 20 * time.Second

	// EnglishDownloadBytes — ước tính tải về khi cài gói tiếng Anh (thư viện + mô hình; chưa tính Python/ffmpeg).
	EnglishDownloadBytes int64 = KokoroModelBytes + 70<<20

	kokoroRequirements = "kokoro-requirements.txt"
)

// verifySentenceEN — câu đọc thử của gói tiếng Anh.
const verifySentenceEN = "Hello, this is Sano reading a short English sentence."

// InitialEnglishStatus — các dòng của lượt cài gói giọng tiếng Anh.
func InitialEnglishStatus() Status {
	return Status{EtaSec: -1, Steps: []Step{
		{Key: StepPython, Label: "Python", State: StatePending},
		{Key: StepKokoro, Label: "Bộ đọc Kokoro (tiếng Anh)", State: StatePending},
		{Key: StepKokoroModels, Label: "Mô hình giọng tiếng Anh (" + humanMB(KokoroModelBytes) + ")", State: StatePending},
		{Key: StepFFmpeg, Label: "ffmpeg (ghi file MP3)", State: StatePending},
		{Key: StepVerify, Label: "Kiểm tra đọc thử", State: StatePending},
	}}
}

// EnglishReady báo gói giọng tiếng Anh đã cài đủ (python venv + 2 file mô hình).
func EnglishReady(l tts.Layout) bool {
	return fileExists(l.KokoroPython()) &&
		fileExists(filepath.Join(l.KokoroModels(), tts.KokoroModelFile)) &&
		fileExists(filepath.Join(l.KokoroModels(), tts.KokoroVoicesFile))
}

func (in *Installer) englishSteps() []installStep {
	return []installStep{
		{StepPython, in.stepPython},
		{StepKokoro, in.stepKokoro},
		{StepKokoroModels, in.stepKokoroModels},
		{StepFFmpeg, in.stepFFmpeg},
		{StepVerify, in.stepVerifyEnglish},
	}
}

// stepKokoro tạo venv riêng và cài thư viện ghim (kiểm hash từng gói).
func (in *Installer) stepKokoro(ctx context.Context) error {
	l, pins := in.cfg.Layout, in.cfg.Pins
	req, err := fs.ReadFile(in.cfg.Scripts, kokoroRequirements)
	if err != nil {
		return fmt.Errorf("thiếu %s nhúng trong app: %w", kokoroRequirements, err)
	}
	sum := sha256.Sum256(req)
	want := hex.EncodeToString(sum[:]) + "\n" + pins["PYTHON_VERSION"] + "\n" + pins["UV_VERSION"] + "\n"
	marker := filepath.Join(l.KokoroVenv(), syncedMarker)
	if fileExists(l.KokoroPython()) && readRaw(marker) == want {
		in.skip(StepKokoro, "Đã cài bộ đọc Kokoro")
		return nil
	}
	if err := os.RemoveAll(l.KokoroVenv()); err != nil { // cài lại từ đầu cho đúng bản ghim
		return err
	}
	if err := os.MkdirAll(l.Kokoro(), 0o755); err != nil {
		return err
	}
	reqFile := filepath.Join(l.Kokoro(), kokoroRequirements)
	if err := os.WriteFile(reqFile, req, 0o644); err != nil {
		return err
	}
	in.running(StepKokoro, 5, "Tạo môi trường Python")
	if _, err := in.run(ctx, l.Kokoro(), in.uvEnv(), l.UV(), "venv", "--python", pins["PYTHON_VERSION"], l.KokoroVenv()); err != nil {
		return fmt.Errorf("tạo môi trường Kokoro: %w", err)
	}
	in.running(StepKokoro, 10, "Cài thư viện Python (onnxruntime, numpy…)")
	stop := in.watchSize(ctx, StepKokoro, 10, 99, kokoroLibBytes, func(n int64) string {
		return "Cài thư viện Python · " + humanMB(n)
	}, l.Cache(), l.KokoroVenv())
	_, err = in.run(ctx, l.Kokoro(), in.uvEnv(), l.UV(), "pip", "install",
		"--python", l.KokoroPython(), "--require-hashes", "--no-deps", "-r", reqFile)
	stop()
	if err != nil {
		return fmt.Errorf("cài thư viện của Kokoro: %w", err)
	}
	if err := os.WriteFile(marker, []byte(want), 0o644); err != nil {
		return err
	}
	_ = os.RemoveAll(l.Cache())
	in.done(StepKokoro, "Bộ đọc Kokoro")
	return nil
}

// stepKokoroModels tải 2 file mô hình, kiểm SHA256 (file đã có mà đúng hash thì bỏ qua).
func (in *Installer) stepKokoroModels(ctx context.Context) error {
	l, pins := in.cfg.Layout, in.cfg.Pins
	files := []struct{ name, url, sha string }{
		{tts.KokoroModelFile, pins["KOKORO_MODEL_URL"], pins["KOKORO_MODEL_SHA256"]},
		{tts.KokoroVoicesFile, pins["KOKORO_VOICES_URL"], pins["KOKORO_VOICES_SHA256"]},
	}
	for _, f := range files {
		if f.url == "" || len(f.sha) != 64 {
			return fmt.Errorf("versions.env thiếu KOKORO_MODEL_* / KOKORO_VOICES_*")
		}
	}
	if err := os.MkdirAll(l.KokoroModels(), 0o755); err != nil {
		return err
	}
	in.running(StepKokoroModels, 0, "Kiểm mô hình đã có")
	var todo []int
	for i, f := range files {
		dst := filepath.Join(l.KokoroModels(), f.name)
		if got, err := fileSHA256(dst); err == nil && got == f.sha {
			in.log.Printf("Kokoro: %s đã có, SHA256 khớp", f.name)
			continue
		}
		todo = append(todo, i)
	}
	if len(todo) == 0 {
		in.skip(StepKokoroModels, "Đủ mô hình, đã kiểm SHA256")
		return nil
	}
	for n, i := range todo {
		f := files[i]
		base := float64(n) * 100 / float64(len(todo))
		span := 100 / float64(len(todo))
		in.running(StepKokoroModels, base, "Tải "+f.name)
		if _, err := download(ctx, in.cfg.HTTP, f.url, filepath.Join(l.KokoroModels(), f.name), f.sha, maxKokoroFileBytes, func(d, t int64) {
			in.setPct(StepKokoroModels, base+span*0.97*frac(d, t), "Tải "+f.name+" · "+progressText(d, t))
		}); err != nil {
			return err
		}
		in.log.Printf("Kokoro: %s SHA256 khớp %s", f.name, f.sha)
	}
	in.done(StepKokoroModels, "Đủ mô hình, đã kiểm SHA256")
	return nil
}

// stepVerifyEnglish đọc thử một câu tiếng Anh bằng đúng script + môi trường lúc chạy thật.
func (in *Installer) stepVerifyEnglish(ctx context.Context) error {
	l := in.cfg.Layout
	ff := in.findFFmpeg()
	if ff == "" {
		return fmt.Errorf("không tìm thấy ffmpeg. %s", tts.FFmpegHint(in.cfg.GOOS))
	}
	if _, err := in.run(ctx, l.Root, baseEnv(), ff, "-hide_banner", "-version"); err != nil {
		return fmt.Errorf("ffmpeg không chạy được: %w", err)
	}
	in.running(StepVerify, 15, "Đọc thử một câu")
	dir := filepath.Join(l.Root, "verify")
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "thu.txt"), []byte(verifySentenceEN+"\n"), 0o644); err != nil {
		return err
	}
	start := time.Now()
	stop := in.watchTime(ctx, StepVerify, 15, 97, kokoroVerifyWait)
	env := append(append(baseEnv(), l.KokoroEnv()...), "PYTHONPATH="+l.Scripts(), "PYTHONUNBUFFERED=1")
	_, err := in.run(ctx, dir, env, l.KokoroPython(), filepath.Join(l.Scripts(), "kokoro_gen_batch.py"), "thu.txt")
	stop()
	if err != nil {
		return fmt.Errorf("đọc thử: %w", err)
	}
	info, err := os.Stat(filepath.Join(dir, "thu_full.wav"))
	if err != nil || info.Size() < 10_000 {
		return fmt.Errorf("đọc thử không ra file âm thanh")
	}
	in.done(StepVerify, fmt.Sprintf("Đọc thử được (%.0f giây)", time.Since(start).Seconds()))
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
