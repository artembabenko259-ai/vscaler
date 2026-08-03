# VSCALER — AI Video Upscaler (NVIDIA GPU / Vulkan)

**VSCALER** is an automated AI-powered video upscaler built with **Go**, **Real-ESRGAN Vulkan AI engine**, and **Bubble Tea TUI**.

It automatically extracts video frames, upscales them using GPU acceleration (NVIDIA RTX 5060), interpolates motion/FPS, and re-encodes the final 4K video using FFmpeg.

---

## Features

- **Automated AI Engine Downloader**: Downloads pre-trained Real-ESRGAN Vulkan weights and binaries on first launch.
- **Hardware GPU Acceleration**: Runs inference directly on GPU via Vulkan (NVIDIA, AMD, Intel).
- **Scale Factor Options**: `2x` and `4x` (e.g. 720p ➔ 4K or 1080p ➔ 4K).
- **Multiple AI Models**:
  - `realesrgan-x4plus` — General photo-realistic video.
  - `realesr-animevideov3` — Anime & 2D animation.
- **FPS Interpolation**: Upscale to 60 FPS / 120 FPS.
- **Minimalist TUI**: Pure terminal interface built with Bubble Tea.

---

## Quick Start

### Build & Run
```bash
go build -o vscaler.exe ./cmd/vscaler
./vscaler.exe
```

---

## License

MIT License.
