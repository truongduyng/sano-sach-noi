# Giấy phép bên thứ ba — phần mềm Sano

Sano (giấy phép MIT) dùng các thành phần dưới đây. Phần mềm desktop **không đóng gói sẵn** bộ đọc: lần mở đầu, máy người dùng tự tải từng thành phần từ nguồn chính thức, đúng phiên bản ghim trong `scripts/tts/versions.env` và kiểm SHA256 (xem `desktop/internal/setup`). File này đi kèm app (Cài đặt → Giới thiệu → Giấy phép bên thứ ba).

## Bộ đọc giọng

### VieNeu-TTS — Apache License 2.0

- Tác giả: Phạm Nguyễn Ngọc Bảo (pnnbao97)
- Mã nguồn: https://github.com/pnnbao97/VieNeu-TTS — commit `c1390abbdb2eedcdf58eafb546966c06ce27af71` (bản 3.8.3)
- Sano gọi thư viện `vieneu` qua script `scripts/tts/audio_gen_batch.py`; không sửa mã VieNeu.
- Toàn văn giấy phép: `Apache-2.0.txt` (chép nguyên file LICENSE của VieNeu-TTS). Upstream không có file NOTICE.

### Mô hình VieNeu-TTS v3 Turbo — Apache License 2.0

- Tác giả: pnnbao-ump
- Nguồn: https://huggingface.co/pnnbao-ump/VieNeu-TTS-v3-Turbo — revision `61b85e3d937fbbacb387714180e8182823512523`
- Dùng các file ONNX trong `onnx_update/` + `denoiser.onnx`, không chỉnh sửa.

### MOSS Audio Tokenizer Nano (ONNX) — Apache License 2.0

- Tác giả: OpenMOSS Team
- Nguồn: https://huggingface.co/OpenMOSS-Team/MOSS-Audio-Tokenizer-Nano-ONNX — revision `ceff0d0749bfb3fa2d61149794ec6feef0d1e1ae`
- Codec giải mã âm thanh cho VieNeu-TTS v3 Turbo, không chỉnh sửa.

Lưu ý: Sano chỉ dùng VieNeu-TTS v3 Turbo và 25 giọng dựng sẵn đi kèm mô hình (Apache-2.0). VieNeu v4 và kho giọng trên vieneu.io là sản phẩm độc quyền của tác giả VieNeu — Sano không dùng.

### Gói giọng tiếng Anh (tải riêng khi người dùng chọn sách tiếng Anh)

- **Kokoro-82M** — Apache License 2.0 — tác giả hexgrad — https://huggingface.co/hexgrad/Kokoro-82M. Sano dùng bản ONNX int8 (`kokoro-v1.0.int8.onnx`) và file giọng `voices-v1.0.bin` từ GitHub Release `model-files-v1.0` của https://github.com/thewh1teagle/kokoro-onnx, ghim SHA256 trong `scripts/tts/versions.env`, không chỉnh sửa.
- **kokoro-onnx** — MIT — thewh1teagle — https://github.com/thewh1teagle/kokoro-onnx (thư viện Python chạy mô hình).
- **espeak-ng** (qua gói `espeakng-loader`) và **phonemizer** — GNU GPL v3 — https://github.com/espeak-ng/espeak-ng, https://github.com/bootphon/phonemizer. Dùng để đổi chữ tiếng Anh thành âm. Sano không đóng gói hay phân phối lại: máy người dùng tự cài từ PyPI (đúng bản và SHA256 ghim trong `scripts/tts/kokoro-requirements.txt`) vào venv riêng, và Sano chỉ gọi `kokoro_gen_batch.py` như một chương trình riêng (không liên kết vào Sano).
- Các thư viện Python khác của gói (onnxruntime, numpy, soundfile…) giữ giấy phép riêng; danh sách phiên bản: `scripts/tts/kokoro-requirements.txt`.

## Công cụ cài đặt

### uv — MIT hoặc Apache License 2.0

- Astral Software Inc. — https://github.com/astral-sh/uv (bản 0.11.14, tải từ GitHub Release chính thức)
- Dùng để tải Python và cài thư viện theo `uv.lock` của VieNeu-TTS.

### Python (CPython, bản dựng python-build-standalone) — Python Software Foundation License

- https://www.python.org — bản 3.12 do uv tải và kiểm SHA256.
- Thư viện Python (onnxruntime, numpy, scipy, soundfile, huggingface-hub…) cài theo `uv.lock` của VieNeu-TTS, mỗi thư viện giữ giấy phép riêng (MIT, BSD, Apache-2.0…). Danh sách phiên bản: `scripts/tts/requirements.txt`.

### ffmpeg — GNU GPL v3 (chỉ tải khi máy chưa có ffmpeg)

Sano gọi ffmpeg như một chương trình riêng (chuyển WAV → MP3), không liên kết vào Sano. Bản dựng tĩnh bật `--enable-gpl --enable-version3`, không bật `--enable-nonfree`, nên được phân phối lại theo GPLv3. Mã nguồn ffmpeg: https://ffmpeg.org/download.html.

- macOS: bản dựng của Martin Riedl, ffmpeg 9.0.2 — https://ffmpeg.martin-riedl.de
- Windows: bản dựng gyan.dev 9.0.2 essentials — https://www.gyan.dev/ffmpeg/builds/ (GitHub Release `9.0.2` của https://github.com/GyanD/codexffmpeg)
- Linux: bản dựng BtbN 9.0.1 GPL — https://github.com/BtbN/FFmpeg-Builds (release `autobuild-2026-08-31-13-27`)

## Phần mềm desktop

- Wails v2 — MIT — https://github.com/wailsapp/wails
- Vue 3 — MIT — https://github.com/vuejs/core
- Tailwind CSS — MIT — https://github.com/tailwindlabs/tailwindcss
- node-qrcode (mã QR tải app nghe sách) — MIT — https://github.com/soldair/node-qrcode
- lucide (biểu tượng) — ISC — https://github.com/lucide-icons/lucide
- golang.org/x/sys — BSD-3-Clause — https://go.googlesource.com/sys
- github.com/ledongthuc/pdf (đọc chữ trong file PDF) — BSD-3-Clause — https://github.com/ledongthuc/pdf
- github.com/ulikunitz/xz (giải nén bản ffmpeg Linux) — BSD-3-Clause — https://github.com/ulikunitz/xz
- Các thư viện Go / npm khác: xem `go.mod`, `desktop/go.mod`, `desktop/frontend/package.json` (đều MIT, BSD hoặc Apache-2.0).
