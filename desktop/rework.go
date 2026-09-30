package main

// Phần "Sửa sách" của App (wireframe D11): đọc lại các tiểu mục đã sửa, đổi giọng
// cả cuốn, đổi tên kèm vẽ lại bìa + lời mở đầu, đổi bìa. Việc đọc chạy nền, mỗi
// lúc một lượt, không chạy chung với Tạo sách (cùng một bộ đọc).

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"sano/desktop/internal/library"
	"sano/internal/bookmaker"
)

// Sự kiện Wails của lượt sửa sách.
const (
	eventEditProgress = "edit:progress"
	eventEditFinished = "edit:finished"
)

// Loại lượt sửa.
const (
	editKindSections = "sections" // đọc lại các tiểu mục đã sửa
	editKindVoice    = "voice"    // đổi giọng cả cuốn
)

// SectionEdit — một tiểu mục người dùng đã sửa (tiêu đề + chữ).
type SectionEdit struct {
	Index int    `json:"index"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

// EditStatus — trạng thái lượt sửa hiện tại / gần nhất.
type EditStatus struct {
	Running   bool   `json:"running"`
	Done      bool   `json:"done"`
	Cancelled bool   `json:"cancelled"`
	Error     string `json:"error"`
	Kind      string `json:"kind"`
	Slug      string `json:"slug"`
	BookTitle string `json:"bookTitle"`
	Voice     string `json:"voice"`
	Total     int    `json:"total"`
	Finished  int    `json:"finished"`  // số mục đã đọc xong (kể cả lượt trước khi nối tiếp)
	Current   int    `json:"current"`   // chỉ số tiểu mục đang đọc (-1 khi chưa rõ)
	DoneIdx   []int  `json:"doneIdx"`   // các tiểu mục đã thay xong trong lượt này
	StartedAt int64  `json:"startedAt"` // unix giây
	Remaining int    `json:"remainSec"` // ước lượng giây còn lại (0 = chưa rõ)
	Queued    []int  `json:"queuedIdx"` // các tiểu mục của lượt (đọc lại từng mục)
}

type editJob struct {
	cancel  context.CancelFunc
	discard bool // huỷ đổi giọng: bỏ phần đã đọc (false = giữ để đọc tiếp lần sau)
	status  EditStatus
}

// EditView — thông tin cho màn Sửa sách, kèm URL phát / bìa.
type EditView struct {
	library.EditInfo
	CoverURL string            `json:"coverUrl"`
	URLs     map[string]string `json:"urls"` // stem → URL MP3 (chống cache theo giờ sửa file)
}

// EditBook đọc một cuốn để sửa.
func (a *App) EditBook(slug string) (*EditView, error) {
	e, err := a.lib.Edit(slug)
	if err != nil {
		return nil, err
	}
	v := &EditView{EditInfo: *e, URLs: map[string]string{}}
	v.CoverURL = a.bookView(e.Book).CoverURL
	for _, s := range e.Sections {
		v.URLs[s.Stem] = a.mediaURLStamp(s.File)
	}
	return v, nil
}

// mediaURLStamp — URL phát file rel (tương đối Root) kèm giờ sửa file, để đọc
// lại xong trình phát lấy bản mới mà không nạp lại bản cũ trong bộ nhớ đệm.
func (a *App) mediaURLStamp(rel string) string {
	u := mediaPrefix + rel
	if full, err := a.lib.Resolve(rel); err == nil {
		if info, err := os.Stat(full); err == nil {
			u += fmt.Sprintf("?v=%x", info.ModTime().UnixNano())
		}
	}
	return u
}

// normFor dựng bộ chuẩn hóa khớp lúc tạo sách (đoán có đọc số mục không từ lời
// đọc cũ của chính tiểu mục đó) + từ điển chung + từ điển của cuốn.
func (a *App) normFor(voice string, sec library.EditSection, book map[string]string) *bookmaker.Normalizer {
	n, err := a.normalizerFor(voice, bookmaker.KeepsHeadingNumbers(sec.Title, sec.Script), book)
	if err != nil {
		return nil
	}
	return n
}

// scriptFor dựng lời đọc mới của một tiểu mục đã sửa.
func (a *App) scriptFor(voice string, sec library.EditSection, book map[string]string, title, text string) string {
	if sec.Intro {
		return bookmaker.IntroScript(a.normFor(voice, sec, book), text)
	}
	return bookmaker.SectionScript(a.normFor(voice, sec, book), title, text)
}

// editBusyLocked — lý do không bắt đầu lượt sửa lúc này (đang giữ a.mu).
func (a *App) editBusyLocked() error {
	switch {
	case a.job != nil && a.job.status.Running:
		return errors.New("đang tạo một cuốn sách khác, đợi tạo xong rồi đọc lại")
	case a.edit != nil && a.edit.status.Running && a.edit.status.Kind == editKindVoice:
		return fmt.Errorf("đang đổi giọng cuốn %q, đợi xong rồi đọc lại", a.edit.status.BookTitle)
	case a.edit != nil && a.edit.status.Running:
		return errors.New("đang đọc lại một lượt khác, đợi xong rồi thử lại")
	}
	return a.renderBusyLocked()
}

// editBusyErr — đang có lượt sửa sách chạy (chặn nghe thử / tạo sách dùng chung bộ đọc).
func (a *App) editBusyErr() error {
	if st := a.editing(); st != nil {
		if st.Kind == editKindVoice {
			return fmt.Errorf("đang đổi giọng cuốn %q, đợi xong rồi thử lại", st.BookTitle)
		}
		return errors.New("đang đọc lại sách, đợi vài giây rồi thử lại")
	}
	return nil
}

// StartReread đọc lại các tiểu mục đã sửa của một cuốn (giọng hiện tại của
// cuốn). Mục nào xong thì thay ngay; hết lượt thì đóng gói lại zip.
func (a *App) StartReread(slug string, edits []SectionEdit) (*EditStatus, error) {
	if len(edits) == 0 {
		return nil, errors.New("chưa có mục nào để đọc lại")
	}
	info, err := a.lib.Edit(slug)
	if err != nil {
		return nil, err
	}
	type prepared struct {
		idx                 int
		title, text, script string
		stem                string
	}
	bookDict, err := a.lib.BookDict(slug)
	if err != nil {
		return nil, err
	}
	var items []prepared
	seen := map[int]bool{}
	for _, e := range edits {
		if e.Index < 0 || e.Index >= len(info.Sections) || seen[e.Index] {
			continue
		}
		seen[e.Index] = true
		sec := info.Sections[e.Index]
		title := strings.TrimSpace(e.Title)
		if title == "" {
			title = sec.Title
		}
		text := strings.TrimSpace(e.Text)
		if text == "" && !sec.Intro {
			return nil, fmt.Errorf("mục %q chưa có chữ để đọc", title)
		}
		script := a.scriptFor(info.Voice, sec, bookDict, title, text)
		if strings.TrimSpace(script) == "" {
			return nil, fmt.Errorf("mục %q chưa có chữ để đọc", title)
		}
		items = append(items, prepared{idx: e.Index, title: title, text: text, script: script, stem: sec.Stem})
	}
	if len(items) == 0 {
		return nil, errors.New("chưa có mục nào để đọc lại")
	}
	voice := info.Voice
	if voice == "" {
		voice = bookmaker.DefaultVoice
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
	a.mu.Lock()
	if err := a.editBusyLocked(); err != nil {
		a.mu.Unlock()
		return nil, err
	}
	work, err := a.lib.SectionsWorkDir(slug)
	if err != nil {
		a.mu.Unlock()
		return nil, err
	}
	ctx, cancel := context.WithCancel(a.context())
	job := &editJob{cancel: cancel, status: EditStatus{
		Running: true, Kind: editKindSections, Slug: slug, BookTitle: info.Title, Voice: voice,
		Total: len(items), Current: items[0].idx, DoneIdx: []int{}, StartedAt: time.Now().Unix(),
	}}
	for _, it := range items {
		job.status.Queued = append(job.status.Queued, it.idx)
	}
	a.edit = job
	st := job.status
	a.mu.Unlock()

	jobs := make([]bookmaker.ScriptJob, 0, len(items))
	byStem := map[string]prepared{}
	for i, it := range items {
		jobs = append(jobs, bookmaker.ScriptJob{Stem: it.stem, Text: it.script})
		byStem[it.stem] = items[i]
	}
	go func() {
		var runErr error
		defer func() {
			if r := recover(); r != nil {
				log.Printf("đọc lại panic: %v\n%s", r, debug.Stack())
				runErr = fmt.Errorf("lỗi không mong muốn: %v", r)
			}
			// Mục nào đã thay thì gói zip phải có bản mới, kể cả khi huỷ / lỗi giữa chừng.
			a.mu.Lock()
			changed := len(job.status.DoneIdx) > 0
			a.mu.Unlock()
			if changed {
				if err := a.lib.Repack(slug, ""); err != nil && runErr == nil {
					runErr = fmt.Errorf("đóng gói lại sách: %w", err)
				}
			}
			_ = os.RemoveAll(work)
			_ = os.Remove(filepath.Dir(work)) // .doc-lai rỗng (không có đổi giọng dở) thì dọn luôn
			a.finishEdit(ctx, job, runErr)
		}()
		start := time.Now()
		runErr = bookmaker.RenderScripts(ctx, t.ttsConfig(voice), work, jobs, func(stem string) {
			it := byStem[stem]
			err := a.lib.ApplySection(slug, it.idx, it.title, it.text, it.script, filepath.Join(work, stem+".mp3"))
			a.mu.Lock()
			if err != nil && job.status.Error == "" {
				job.status.Error = err.Error()
			}
			if err == nil {
				job.status.DoneIdx = append(job.status.DoneIdx, it.idx)
			}
			job.status.Finished++
			if next := job.status.Finished; next < len(items) {
				job.status.Current = items[next].idx
				job.status.Remaining = int(time.Since(start).Seconds() / float64(next) * float64(len(items)-next))
			}
			cur := job.status
			a.mu.Unlock()
			a.emit(eventEditProgress, cur)
		})
	}()
	return &st, nil
}

// StartVoiceChange đọc lại cả cuốn bằng giọng voice. Đọc vào thư mục riêng, xong
// hết mới thay; đang dở cùng giọng (lần trước tắt app) thì đọc tiếp phần còn lại.
func (a *App) StartVoiceChange(slug, voice string) (*EditStatus, error) {
	voice = strings.TrimSpace(voice)
	if voice == "" {
		return nil, errors.New("chưa chọn giọng")
	}
	info, err := a.lib.Edit(slug)
	if err != nil {
		return nil, err
	}
	if info.Voice == voice && info.VoiceJob == nil {
		return nil, fmt.Errorf("cuốn này đang đọc bằng giọng %s", voice)
	}
	if info.Voice != "" && bookmaker.LangOfVoice(info.Voice) != bookmaker.LangOfVoice(voice) {
		return nil, errors.New("giọng tiếng Anh chỉ đọc sách tiếng Anh (và ngược lại) — chọn giọng cùng ngôn ngữ với sách")
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
	a.mu.Lock()
	if err := a.editBusyLocked(); err != nil {
		a.mu.Unlock()
		return nil, err
	}
	vj, err := a.lib.BeginVoice(slug, voice)
	if err != nil {
		a.mu.Unlock()
		return nil, err
	}
	work, err := a.lib.VoiceWorkDir(slug)
	if err != nil {
		a.mu.Unlock()
		return nil, err
	}
	done := map[string]bool{}
	for _, s := range vj.Done {
		done[s] = true
	}
	var jobs []bookmaker.ScriptJob
	idxOf := map[string]int{}
	for _, s := range info.Sections {
		idxOf[s.Stem] = s.Index
		if done[s.Stem] && fileExists(filepath.Join(work, s.Stem+".mp3")) {
			continue
		}
		script := s.Script
		if strings.TrimSpace(script) == "" { // sách cũ thiếu lời đọc: dựng lại từ chữ
			bookDict, _ := a.lib.BookDict(slug)
			script = a.scriptFor(info.Voice, s, bookDict, s.Title, s.Text)
		}
		jobs = append(jobs, bookmaker.ScriptJob{Stem: s.Stem, Text: script})
	}
	ctx, cancel := context.WithCancel(a.context())
	job := &editJob{cancel: cancel, status: EditStatus{
		Running: true, Kind: editKindVoice, Slug: slug, BookTitle: info.Title, Voice: voice,
		Total: len(info.Sections), Finished: len(info.Sections) - len(jobs), Current: -1,
		DoneIdx: []int{}, StartedAt: time.Now().Unix(),
	}}
	if len(jobs) > 0 {
		job.status.Current = idxOf[jobs[0].Stem]
	}
	a.edit = job
	st := job.status
	a.mu.Unlock()

	go func() {
		var runErr error
		defer func() {
			if r := recover(); r != nil {
				log.Printf("đổi giọng panic: %v\n%s", r, debug.Stack())
				runErr = fmt.Errorf("lỗi không mong muốn: %v", r)
			}
			switch {
			case runErr == nil && ctx.Err() == nil:
				runErr = a.lib.FinishVoice(slug)
			case job.discard:
				_ = a.lib.CancelVoice(slug)
			}
			a.finishEdit(ctx, job, runErr)
		}()
		start, n := time.Now(), 0
		runErr = bookmaker.RenderScripts(ctx, t.ttsConfig(voice), work, jobs, func(stem string) {
			err := a.lib.MarkVoiceDone(slug, stem)
			n++
			a.mu.Lock()
			if err != nil && job.status.Error == "" {
				job.status.Error = err.Error()
			}
			job.status.Finished++
			job.status.DoneIdx = append(job.status.DoneIdx, idxOf[stem])
			if n < len(jobs) {
				job.status.Current = idxOf[jobs[n].Stem]
				job.status.Remaining = int(time.Since(start).Seconds() / float64(n) * float64(len(jobs)-n))
			}
			cur := job.status
			a.mu.Unlock()
			a.emit(eventEditProgress, cur)
		})
	}()
	return &st, nil
}

func (a *App) finishEdit(ctx context.Context, job *editJob, runErr error) {
	a.mu.Lock()
	job.status.Running = false
	job.status.Remaining = 0
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		job.status.Cancelled = true
	case runErr != nil:
		job.status.Error = runErr.Error()
	case job.status.Error == "":
		job.status.Done = true
	}
	cur := job.status
	a.mu.Unlock()
	job.cancel()
	a.emit(eventEditFinished, cur)
}

// CancelEdit dừng lượt sửa đang chạy. Đổi giọng: discard = bỏ phần đã đọc, giữ
// giọng cũ; false = giữ phần đã đọc để lần sau đọc tiếp.
func (a *App) CancelEdit(discard bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.edit != nil && a.edit.status.Running {
		a.edit.discard = discard
		a.edit.cancel()
	}
}

// DiscardVoiceChange bỏ đổi giọng đang dở (không chạy) của một cuốn.
func (a *App) DiscardVoiceChange(slug string) error {
	a.mu.Lock()
	running := a.edit != nil && a.edit.status.Running && a.edit.status.Slug == slug && a.edit.status.Kind == editKindVoice
	a.mu.Unlock()
	if running {
		a.CancelEdit(true)
		return nil
	}
	return a.lib.CancelVoice(slug)
}

// EditStatus trả trạng thái lượt sửa hiện tại / gần nhất (nil nếu chưa có).
func (a *App) EditStatus() *EditStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.edit == nil {
		return nil
	}
	st := a.edit.status
	return &st
}

func (a *App) editing() *EditStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.edit == nil || !a.edit.status.Running {
		return nil
	}
	st := a.edit.status
	return &st
}

// SaveInfoResult — kết quả lưu Thông tin & bìa.
type SaveInfoResult struct {
	Book BookView `json:"book"`
	// IntroEdit — lời mở đầu mới cần đọc lại (nil = không có lời mở đầu hoặc không đổi).
	IntroEdit *SectionEdit `json:"introEdit"`
	// CoverRedrawn — đã vẽ lại bìa tự vẽ theo tên mới.
	CoverRedrawn bool `json:"coverRedrawn"`
}

// SaveBookInfo lưu tên, tác giả, danh mục, bộ sách. Đổi tên / tác giả: bìa tự
// vẽ thì vẽ lại; có lời mở đầu thì trả lời mở đầu mới để giao diện đọc lại.
func (a *App) SaveBookInfo(slug string, info library.Info) (*SaveInfoResult, error) {
	before, err := a.lib.Edit(slug)
	if err != nil {
		return nil, err
	}
	d, err := a.lib.UpdateInfo(slug, info)
	if err != nil {
		return nil, err
	}
	res := &SaveInfoResult{}
	if d.Title != before.Title || d.Author != before.Author {
		if before.CoverAuto {
			if err := a.lib.RedrawCover(slug); err != nil {
				return nil, err
			}
			res.CoverRedrawn = true
		}
		if len(before.Sections) > 0 && before.Sections[0].Intro {
			intro := before.Sections[0]
			if t := library.RetitleIntro(intro.Text, d.Title, d.Author); t != intro.Text {
				res.IntroEdit = &SectionEdit{Index: 0, Title: d.Title, Text: t}
			}
		}
	}
	after, err := a.lib.Get(slug)
	if err != nil {
		return nil, err
	}
	res.Book = a.bookView(after.Book)
	return res, nil
}

// SetBookCover dùng ảnh path làm bìa của cuốn.
func (a *App) SetBookCover(slug, path string) (*BookView, error) {
	if _, err := describeCover(path); err != nil { // kiểm loại + dung lượng
		return nil, err
	}
	if err := a.lib.SetCoverImage(slug, path); err != nil {
		return nil, err
	}
	return a.bookAfterEdit(slug)
}

// UseAutoCover quay về bìa tự vẽ theo tên sách.
func (a *App) UseAutoCover(slug string) (*BookView, error) {
	if err := a.lib.RedrawCover(slug); err != nil {
		return nil, err
	}
	return a.bookAfterEdit(slug)
}

func (a *App) bookAfterEdit(slug string) (*BookView, error) {
	d, err := a.lib.Get(slug)
	if err != nil {
		return nil, err
	}
	v := a.bookView(d.Book)
	return &v, nil
}

// resumeVoiceJobs: lần trước tắt app giữa lúc đổi giọng → đọc tiếp khi bộ đọc sẵn sàng.
func (a *App) resumeVoiceJobs() {
	for slug, j := range a.lib.PendingVoiceJobs() {
		if _, err := a.StartVoiceChange(slug, j.Voice); err != nil {
			log.Printf("đọc tiếp đổi giọng %s: %v", slug, err)
		}
		return // mỗi lúc một lượt; cuốn còn lại đọc tiếp ở lần mở sau
	}
}
