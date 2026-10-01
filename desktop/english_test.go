package main

import (
	"errors"
	"testing"

	"sano/internal/bookmaker"
)

// Giọng tiếng Anh dùng gói Kokoro; chưa cài thì lỗi báo rõ để giao diện mời cài.
func TestToolsFor_GiongTiengAnhCanGoiKokoro(t *testing.T) {
	t.Setenv("SANO_DATA_DIR", t.TempDir())
	a := &App{}
	if _, err := a.toolsFor("Heart"); !errors.Is(err, ErrEnglishNotInstalled) {
		t.Errorf("toolsFor(Heart) khi chưa cài = %v, muốn ErrEnglishNotInstalled", err)
	}
	if _, err := a.toolsFor(bookmaker.DefaultVoice); err == nil || errors.Is(err, ErrEnglishNotInstalled) {
		t.Errorf("giọng Việt không được đòi gói tiếng Anh: %v", err)
	}
}

// options() nhận cả giọng Việt lẫn giọng Anh và luôn dựng được bộ chuẩn hoá.
func TestOptions_TheoGiong(t *testing.T) {
	t.Setenv("SANO_DATA_DIR", t.TempDir())
	for _, voice := range []string{"Heart", bookmaker.DefaultVoice} {
		s := BookSettings{Path: t.TempDir() + "/a.txt", Voice: voice}
		opts, err := s.options(toolPaths{}, t.TempDir(), nil)
		if err != nil {
			t.Fatalf("%s: %v", voice, err)
		}
		if opts.Norm == nil || opts.TTS.Voice != voice {
			t.Errorf("%s: Norm=%v Voice=%q", voice, opts.Norm, opts.TTS.Voice)
		}
	}
}
