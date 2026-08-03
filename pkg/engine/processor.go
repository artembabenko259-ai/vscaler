package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ProgressCallback func(percent float64, step string)

type UpscaleOptions struct {
	VideoPath  string
	OutputDir  string
	Scale      int     // 2x, 4x
	ModelName  string  // realesrgan-x4plus, realesr-animevideov3
	TargetFPS  float64 // 0 = original, 60 = 60fps
}

type Processor struct {
	downloader *Downloader
	ffmpeg     *FFmpeg
	upscaler   *Upscaler
}

func NewProcessor() *Processor {
	return &Processor{
		downloader: NewDownloader(),
		ffmpeg:     NewFFmpeg(),
		upscaler:   NewUpscaler(),
	}
}

func (p *Processor) Process(opts UpscaleOptions, callback ProgressCallback) (string, error) {
	if !p.ffmpeg.IsInstalled() {
		return "", fmt.Errorf("FFmpeg is not installed in PATH")
	}

	if err := p.downloader.EnsureEngineInstalled(callback); err != nil {
		return "", fmt.Errorf("failed to initialize AI engine: %v", err)
	}

	if opts.OutputDir == "" {
		opts.OutputDir = filepath.Dir(opts.VideoPath)
	}

	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return "", err
	}

	baseName := strings.TrimSuffix(filepath.Base(opts.VideoPath), filepath.Ext(opts.VideoPath))
	outVideo := filepath.Join(opts.OutputDir, fmt.Sprintf("%s_4K_%dx.mp4", baseName, opts.Scale))

	if callback != nil {
		callback(10.0, "Initializing NVIDIA RTX GPU (NVENC + Vulkan FP16)...")
	}

	// High speed streaming pipeline using NVENC + FP16 Vulkan multi-threading
	if callback != nil {
		callback(30.0, fmt.Sprintf("High-Speed GPU AI Upscaling (%dx %s @ NVENC 60+ FPS)...", opts.Scale, opts.ModelName))
	}

	err := p.upscaler.UpscaleStream(opts.VideoPath, outVideo, opts.Scale, opts.ModelName, opts.TargetFPS, callback)
	if err != nil {
		// Fallback to frame pipeline if direct stream encountered format restriction
		return p.processFrameFallback(opts, baseName, callback)
	}

	if callback != nil {
		callback(100.0, fmt.Sprintf("DONE! High-Speed 4K Video saved to %s", outVideo))
	}

	return outVideo, nil
}

func (p *Processor) processFrameFallback(opts UpscaleOptions, baseName string, callback ProgressCallback) (string, error) {
	tempDir := filepath.Join(opts.OutputDir, baseName+"_vscaler_temp")
	inputFrames := filepath.Join(tempDir, "in")
	outputFrames := filepath.Join(tempDir, "out")
	audioPath := filepath.Join(tempDir, "audio.aac")
	outVideo := filepath.Join(opts.OutputDir, fmt.Sprintf("%s_4K_%dx.mp4", baseName, opts.Scale))

	_ = os.MkdirAll(inputFrames, 0755)
	_ = os.MkdirAll(outputFrames, 0755)
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	if callback != nil {
		callback(15.0, "Extracting video frames and audio...")
	}
	if err := p.ffmpeg.ExtractFrames(opts.VideoPath, inputFrames); err != nil {
		return "", err
	}
	_ = p.ffmpeg.ExtractAudio(opts.VideoPath, audioPath)

	origFPS, _ := p.ffmpeg.GetVideoFPS(opts.VideoPath)
	finalFPS := origFPS
	if opts.TargetFPS > 0 {
		finalFPS = opts.TargetFPS
	}

	if callback != nil {
		callback(40.0, fmt.Sprintf("GPU AI Upscaling frames (%dx %s)...", opts.Scale, opts.ModelName))
	}
	if err := p.upscaler.UpscaleFrames(inputFrames, outputFrames, opts.Scale, opts.ModelName, callback); err != nil {
		return "", err
	}

	if callback != nil {
		callback(85.0, "NVENC Assembling upscaled frames into 4K video...")
	}

	if fi, err := os.Stat(audioPath); err != nil || fi.Size() == 0 {
		audioPath = ""
	}

	if err := p.ffmpeg.AssembleVideo(outputFrames, audioPath, outVideo, finalFPS); err != nil {
		return "", err
	}

	if callback != nil {
		callback(100.0, fmt.Sprintf("DONE! Saved to %s", outVideo))
	}

	return outVideo, nil
}
