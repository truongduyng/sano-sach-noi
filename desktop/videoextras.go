package main

// Âm thanh thêm cho video cả cuốn (wireframe D16, mở đầu kiểu Fonos): câu đọc thêm
// ("Bạn đang nghe sách nói, tạo bằng Sano", giới thiệu sách, lời kết) đọc bằng bộ đọc
// + đúng giọng của cuốn; nhạc hiệu (bản tổng hợp tạm của Sano, hoặc file nhạc người dùng
// chọn, cắt ~4,5 giây, lên nhẹ rồi tắt dần, nhỏ hơn giọng đọc ~20%); tiếng chuông sang chương. Mỗi thứ ra một
// file mp3 trong ~/Sano/.tam/video-them (nghe thử trong hộp thoại) kèm mã để
// BookVideoFinish ghép vào; cùng nội dung thì dùng lại, không đọc lại.

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"sano/desktop/internal/tts"
	"sano/internal/bookmaker"
)

// VideoExtraLine — một câu đọc thêm (Key: brand | info | end).
type VideoExtraLine struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// VideoExtrasRequest — âm thanh thêm cần cho một lượt video.
type VideoExtrasRequest struct {
	Slug      string           `json:"slug"`
	Lines     []VideoExtraLine `json:"lines"`
	Music     string           `json:"music"` // sano | file | none
	MusicPath string           `json:"musicPath"`
	Chime     bool             `json:"chime"`
}

// VideoExtra — một file âm thanh thêm đã sẵn sàng.
type VideoExtra struct {
	ID     string  `json:"id"`
	Key    string  `json:"key"` // brand | info | end | music | chime
	URL    string  `json:"url"`
	DurSec float64 `json:"durSec"`
}

const maxExtraLineRunes = 400

var videoExtras struct {
	mu    sync.Mutex
	files map[string]string // mã → đường dẫn mp3
}

func (a *App) extrasDir() (string, error) {
	d := filepath.Join(a.lib.Root(), ".tam", "video-them")
	return d, os.MkdirAll(d, 0o755)
}

func extraID(parts ...string) string {
	h := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:10])
}

func registerExtra(id, path string) {
	videoExtras.mu.Lock()
	defer videoExtras.mu.Unlock()
	if videoExtras.files == nil {
		videoExtras.files = map[string]string{}
	}
	videoExtras.files[id] = path
}

// extraPath — file của mã đã tạo trong phiên này (không nhận đường dẫn từ giao diện).
func extraPath(id string) (string, bool) {
	videoExtras.mu.Lock()
	defer videoExtras.mu.Unlock()
	p, ok := videoExtras.files[id]
	return p, ok && fileExists(p)
}

// Nhạc hiệu tạm của Sano: tự tổng hợp (không vướng bản quyền) — nền pad hợp âm Đô trưởng
// thêm 7, bốn nốt chuông rải C–E–G–B, lên nhẹ rồi tắt dần trong 4,5 giây.
const sanoJingleExpr = "0.07*sin(2*PI*130.81*t)+0.05*sin(2*PI*196*t)+0.045*sin(2*PI*261.63*t)+0.035*sin(2*PI*329.63*t)" +
	"+0.16*sin(2*PI*523.25*t)*exp(-2.2*t)" +
	"+0.15*sin(2*PI*659.25*(t-0.28))*exp(-2.2*(t-0.28))*gte(t,0.28)" +
	"+0.14*sin(2*PI*783.99*(t-0.56))*exp(-2.2*(t-0.56))*gte(t,0.56)" +
	"+0.13*sin(2*PI*987.77*(t-0.84))*exp(-1.6*(t-0.84))*gte(t,0.84)" +
	"+0.10*sin(2*PI*1046.5*(t-1.3))*exp(-1.1*(t-1.3))*gte(t,1.3)"

// Tiếng chuông sang chương: hai hoạ âm chuông nhỏ, tắt trong ~1,2 giây.
const chimeExpr = "0.22*sin(2*PI*1318.5*t)*exp(-3.2*t)+0.12*sin(2*PI*1975.5*t)*exp(-4.5*t)+0.06*sin(2*PI*2637*t)*exp(-6*t)"

// BookVideoExtras tạo (hoặc lấy lại) các file âm thanh thêm; câu trống thì bỏ.
func (a *App) BookVideoExtras(req VideoExtrasRequest) ([]VideoExtra, error) {
	d, err := a.lib.Get(req.Slug)
	if err != nil {
		return nil, err
	}
	ffmpeg := findFFmpeg()
	if ffmpeg == "" {
		return nil, fmt.Errorf("không tìm thấy ffmpeg. %s", tts.FFmpegHint(runtime.GOOS))
	}
	dir, err := a.extrasDir()
	if err != nil {
		return nil, err
	}
	var out []VideoExtra
	add := func(id, key, path string) error {
		dur, err := mp3Dur(ffmpeg, path)
		if err != nil {
			return err
		}
		registerExtra(id, path)
		out = append(out, VideoExtra{ID: id, Key: key, URL: a.mediaURL(path), DurSec: dur})
		return nil
	}

	// Câu đọc thêm — bộ đọc + giọng của cuốn + từ điển cách đọc của cuốn.
	voice := d.Voice
	if voice == "" {
		voice = bookmaker.DefaultVoice
	}
	var lines []VideoExtraLine
	for _, l := range req.Lines {
		l.Text = strings.TrimSpace(l.Text)
		if l.Text != "" && len([]rune(l.Text)) <= maxExtraLineRunes {
			lines = append(lines, l)
		}
	}
	if len(lines) > 0 {
		if err := a.editBusyErr(); err != nil {
			return nil, err
		}
		bookDict, _ := a.lib.BookDict(req.Slug)
		norm, err := a.normalizerFor(voice, false, bookDict)
		if err != nil {
			return nil, err
		}
		release, err := a.beginTTSUse()
		if err != nil {
			return nil, err
		}
		defer release()
		t, err := a.toolsFor(voice)
		if err != nil {
			return nil, err
		}
		for _, l := range lines {
			id := extraID("voice", voice, l.Text)
			path := filepath.Join(dir, "doc-"+id+".mp3")
			if !fileExists(path) {
				file, err := bookmaker.Speak(a.context(), t.ttsConfig(voice), norm, l.Text, dir, "doc-"+id)
				if err != nil {
					return nil, fmt.Errorf("đọc câu giới thiệu: %w", err)
				}
				path = file
			}
			if err := add(id, l.Key, path); err != nil {
				return nil, err
			}
		}
	}

	// Nhạc hiệu.
	switch req.Music {
	case "sano":
		id := extraID("music", "sano-v2")
		path := filepath.Join(dir, "nhac-"+id+".mp3")
		if !fileExists(path) {
			if err := synth(ffmpeg, path, sanoJingleExpr, 4.5, "aecho=0.8:0.6:70|140:0.25|0.15,afade=t=in:d=0.35,afade=t=out:st=2.6:d=1.9,loudnorm=I=-20:TP=-2,volume=0.8"); err != nil {
				return nil, fmt.Errorf("tạo nhạc hiệu: %w", err)
			}
		}
		if err := add(id, "music", path); err != nil {
			return nil, err
		}
	case "file":
		src := req.MusicPath
		info, err := os.Stat(src)
		if err != nil || info.IsDir() || !isAudioFile(src) {
			return nil, errors.New("không đọc được file nhạc đã chọn")
		}
		id := extraID("music-file-v2", src, strconv.FormatInt(info.ModTime().UnixNano(), 10), strconv.FormatInt(info.Size(), 10))
		path := filepath.Join(dir, "nhac-"+id+".mp3")
		if !fileExists(path) {
			if err := runQuietBG(ffmpeg, dir, []string{"-hide_banner", "-loglevel", "error", "-y", "-t", "5", "-i", src, "-vn",
				"-af", "aresample=44100,aformat=channel_layouts=mono,afade=t=in:d=0.4,afade=t=out:st=3:d=2,loudnorm=I=-20:TP=-2,volume=0.8",
				"-c:a", "libmp3lame", "-b:a", "160k", path}); err != nil {
				return nil, fmt.Errorf("đọc file nhạc: %w", err)
			}
		}
		if err := add(id, "music", path); err != nil {
			return nil, err
		}
	}

	if req.Chime {
		id := extraID("chime", "v1")
		path := filepath.Join(dir, "chuong-"+id+".mp3")
		if !fileExists(path) {
			if err := synth(ffmpeg, path, chimeExpr, 1.3, "aecho=0.7:0.5:90:0.2,afade=t=out:st=0.9:d=0.4"); err != nil {
				return nil, fmt.Errorf("tạo tiếng chuông: %w", err)
			}
		}
		if err := add(id, "chime", path); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func isAudioFile(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".mp3", ".m4a", ".aac", ".wav", ".ogg", ".flac", ".opus":
		return true
	}
	return false
}

func synth(ffmpeg, out, expr string, dur float64, af string) error {
	return runQuietBG(ffmpeg, filepath.Dir(out), []string{"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("aevalsrc='%s':s=44100:d=%s", expr, strconv.FormatFloat(dur, 'f', 2, 64)),
		"-af", af, "-ac", "1", "-c:a", "libmp3lame", "-b:a", "160k", out})
}

func runQuietBG(bin, dir string, args []string) error {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	tts.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = msg[i+1:]
		}
		return errors.New(firstNonEmpty(msg, err.Error()))
	}
	return nil
}

// mp3Dur — thời lượng thật (giải mã), đúng tới mili giây để ghép dòng thời gian.
func mp3Dur(ffmpeg, path string) (float64, error) {
	cmd := exec.Command(ffmpeg, "-hide_banner", "-i", path, "-f", "null", "-")
	tts.HideWindow(cmd)
	out, _ := cmd.CombinedOutput()
	s := string(out)
	i := strings.LastIndex(s, "time=")
	if i < 0 {
		return 0, fmt.Errorf("không đọc được thời lượng %s", filepath.Base(path))
	}
	f := strings.Fields(s[i+5:])
	if len(f) == 0 {
		return 0, errors.New("không đọc được thời lượng")
	}
	var h, m int
	var sec float64
	if _, err := fmt.Sscanf(f[0], "%d:%d:%f", &h, &m, &sec); err != nil {
		return 0, fmt.Errorf("không đọc được thời lượng: %w", err)
	}
	return float64(h*3600+m*60) + sec, nil
}

// PickMusicFile — hộp chọn file nhạc hiệu của người dùng ("" nếu huỷ).
func (a *App) PickMusicFile() (string, error) {
	if a.ctx == nil {
		return "", errors.New("ứng dụng chưa khởi động xong")
	}
	return wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:   "Chọn file nhạc hiệu",
		Filters: []wruntime.FileFilter{{DisplayName: "Âm thanh (*.mp3, *.m4a, *.wav, *.ogg, *.flac)", Pattern: "*.mp3;*.m4a;*.aac;*.wav;*.ogg;*.flac;*.opus"}},
	})
}
