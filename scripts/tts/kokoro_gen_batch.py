#!/usr/bin/env python3
"""
Đọc giọng tiếng Anh nhiều file văn bản bằng Kokoro-82M (ONNX, chạy CPU).
Cùng giao diện dòng lệnh với audio_gen_batch.py (VieNeu) để bookmaker/tts.go gọi
như nhau: mỗi file <stem>.txt → <stem>_full.wav (24 kHz) ở thư mục hiện tại, in
"✨ <stem>_full.wav" khi xong từng file, --list-voices in danh sách giọng.

Mô hình nằm trong thư mục $SANO_KOKORO_DIR (kokoro-v1.0.int8.onnx + voices-v1.0.bin),
do phần mềm desktop tải và kiểm SHA256 lúc cài gói giọng tiếng Anh.

Usage (python của venv Kokoro):
    python kokoro_gen_batch.py chuong1.txt chuong2.txt
    python kokoro_gen_batch.py --voice Emma chuong*.txt
    python kokoro_gen_batch.py --list-voices
"""

import argparse
import os
import sys
import time
from pathlib import Path

# In tiếng Việt + emoji an toàn trên console Windows (mặc định cp1252).
for _s in (sys.stdout, sys.stderr):
    try:
        _s.reconfigure(encoding="utf-8", errors="replace")
    except Exception:
        pass

MODEL_FILE = "kokoro-v1.0.int8.onnx"
VOICES_FILE = "voices-v1.0.bin"
DEFAULT_VOICE = "Heart"
GAP_SEC = 0.45  # nghỉ giữa hai đoạn (dòng)

# (tên hiện ra, id trong voices-v1.0.bin, mô tả, nổi bật). Khớp englishVoices trong
# internal/bookmaker/english.go (test kiểm hai bên giống nhau). Id bắt đầu "a" =
# tiếng Anh Mỹ, "b" = Anh.
VOICES = [
    ("Heart", "af_heart", "Nữ · Mỹ · Ấm, tự nhiên (chất lượng cao nhất)", True),
    ("Bella", "af_bella", "Nữ · Mỹ · Rõ, biểu cảm", True),
    ("Nicole", "af_nicole", "Nữ · Mỹ · Nhẹ, thì thầm", False),
    ("Aoede", "af_aoede", "Nữ · Mỹ", False),
    ("Kore", "af_kore", "Nữ · Mỹ", False),
    ("Sarah", "af_sarah", "Nữ · Mỹ", False),
    ("Michael", "am_michael", "Nam · Mỹ · Trầm, vững", True),
    ("Fenrir", "am_fenrir", "Nam · Mỹ · Mạnh", False),
    ("Puck", "am_puck", "Nam · Mỹ", False),
    ("Emma", "bf_emma", "Nữ · Anh · Điềm đạm", True),
    ("George", "bm_george", "Nam · Anh · Trầm, kể chuyện", True),
    ("Fable", "bm_fable", "Nam · Anh", False),
]


def print_voices():
    print("\n📋 Giọng có sẵn:")
    for name, _id, desc, featured in VOICES:
        print(f"   • {'⭐ ' if featured else ''}{name} — {desc}")


def pick_voice(query):
    """Trả (tên, id): khớp đúng tên/id, rồi khớp một phần (không phân biệt hoa thường)."""
    q = (query or DEFAULT_VOICE).strip().casefold()
    hit = next((v for v in VOICES if q in (v[0].casefold(), v[1].casefold())), None)
    hit = hit or next((v for v in VOICES if q in v[0].casefold()), None)
    if hit is None:
        print_voices()
        raise SystemExit(f"❌ Không có giọng '{query}'.")
    print(f"✅ Giọng: {hit[0]}")
    return hit[0], hit[1]


# espeak-ng (bộ phiên âm của Kokoro) chỉ mở được thư mục dữ liệu khi đường dẫn
# ngắn hơn ~155 ký tự — dài hơn thì báo "không thấy phontab" rồi hỏng (đo thực
# tế: 150 chạy, 160 hỏng). Venv của Sano thường nằm dưới ~130 ký tự; máy có tên
# người dùng dài / SANO_DATA_DIR sâu thì chép dữ liệu (~19 MB) sang thư mục tạm.
ESPEAK_MAX_PATH = 140


def espeak_data_path():
    import espeakng_loader

    src = espeakng_loader.get_data_path()
    if len(src) <= ESPEAK_MAX_PATH:
        return src
    import shutil
    import tempfile

    dst = Path(tempfile.gettempdir()) / "sano-espeak-data"
    if not (dst / "phontab").exists():
        tmp = dst.with_name(dst.name + f".{os.getpid()}")
        shutil.rmtree(tmp, ignore_errors=True)
        shutil.copytree(src, tmp, symlinks=True)
        try:
            tmp.rename(dst)
        except OSError:  # tiến trình khác vừa chép xong
            shutil.rmtree(tmp, ignore_errors=True)
    if len(str(dst)) > ESPEAK_MAX_PATH:
        raise SystemExit(f"❌ Đường dẫn dữ liệu espeak quá dài ({dst}); đặt TMPDIR ngắn hơn.")
    return str(dst)


def load_tts():
    d = Path(os.environ.get("SANO_KOKORO_DIR", "."))
    model, voices = d / MODEL_FILE, d / VOICES_FILE
    for f in (model, voices):
        if not f.exists():
            raise SystemExit(f"❌ Thiếu file mô hình {f} — cài lại gói giọng tiếng Anh.")
    from kokoro_onnx import EspeakConfig, Kokoro

    return Kokoro(str(model), str(voices), espeak_config=EspeakConfig(data_path=espeak_data_path()))


def paragraphs(text):
    return [ln.strip() for ln in text.replace("\r\n", "\n").split("\n") if ln.strip()]


def synth_file(tts, voice_id, input_path, output_path):
    """Đọc cả file → WAV. Trả (thời lượng audio giây, thời gian đọc giây)."""
    import numpy as np
    import soundfile as sf

    paras = paragraphs(Path(input_path).read_text(encoding="utf-8").lstrip("﻿"))
    if not paras:
        raise ValueError(f"File rỗng: {input_path}")
    lang = "en-gb" if voice_id.startswith("b") else "en-us"
    start = time.time()
    parts, rate = [], 24000
    for p in paras:
        samples, rate = tts.create(p, voice=voice_id, speed=1.0, lang=lang)
        parts.append(samples)
        parts.append(np.zeros(int(GAP_SEC * rate), dtype=np.float32))
    wav = np.concatenate(parts[:-1])
    compute = time.time() - start
    sf.write(str(output_path), wav, rate)
    return len(wav) / rate, compute


def process_file(tts, voice_id, input_file):
    input_path = Path(input_file)
    if not input_path.exists():
        print(f"\n❌ File không tồn tại: {input_file}")
        return {"file": input_file, "ok": False, "error": "Not found"}
    output = Path(input_path.stem + "_full.wav")
    print(f"\n{'=' * 70}\n  📄 {input_file}\n{'=' * 70}")
    try:
        duration, compute = synth_file(tts, voice_id, input_path, output)
    except Exception as e:  # 1 file lỗi không làm hỏng cả lượt
        print(f"  ❌ Lỗi: {e}")
        return {"file": input_file, "ok": False, "error": str(e)[:60]}
    print(f"  ✨ {output.name}")
    print(f"     Audio: {duration:.1f}s · đọc mất {compute:.1f}s ({compute / max(duration, 1):.2f}x realtime)")
    return {"file": input_file, "ok": True}


def main():
    parser = argparse.ArgumentParser(description="Đọc giọng tiếng Anh nhiều file văn bản bằng Kokoro-82M")
    parser.add_argument("files", nargs="*", help="Các file txt input")
    parser.add_argument("--voice", default=DEFAULT_VOICE, help=f'Tên giọng (mặc định: "{DEFAULT_VOICE}")')
    parser.add_argument("--list-voices", action="store_true", help="In danh sách giọng rồi thoát")
    args = parser.parse_args()

    if args.list_voices:
        print_voices()
        return
    if not args.files:
        parser.error("cần ít nhất 1 file txt")
    _name, voice_id = pick_voice(args.voice)
    print("🎤 Loading Kokoro-82M (1 lần duy nhất)...")
    t0 = time.time()
    tts = load_tts()
    print(f"   Loaded trong {time.time() - t0:.0f}s")

    results = [process_file(tts, voice_id, f) for f in sorted(args.files)]
    failed = [r for r in results if not r["ok"]]
    if failed:
        print(f"  ⚠️  {len(failed)} file lỗi")
        raise SystemExit(1)


if __name__ == "__main__":
    main()
