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
	Scale      int    // 2x, 4x
	ModelName  string // realesrgan-x4plus, realesrgan-x4plus-anime, realesr-animevideov3
	TargetFPS  float64 // 0 = keep original, 60 = interpolate to 60fps
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
	tempDir := filepath.Join(opts.OutputDir, baseName+"_vscaler_temp")
	inputFrames := filepath.Join(tempDir, "in")
	outputFrames := filepath.Join(tempDir, "out")
	audioPath := filepath.Join(tempDir, "audio.aac")
	outVideo := filepath.Join(opts.OutputDir, fmt.Sprintf("%s_upscaled_%dx.mp4", baseName, opts.Scale))

	_ = os.MkdirAll(inputFrames, 0755)
	_ = os.MkdirAll(outputFrames, 0755)
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Step 1: Extract Frames & Audio
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

	// Step 2: AI GPU Upscaling
	if callback != nil {
		callback(40.0, fmt.Sprintf("AI Upscaling frames on GPU (%dx using %s)...", opts.Scale, opts.ModelName))
	}
	if err := p.upscaler.UpscaleFrames(inputFrames, outputFrames, opts.Scale, opts.ModelName, callback); err != nil {
		return "", err
	}

	// Step 3: Assemble High Quality Video
	if callback != nil {
		callback(85.0, "Assembling upscaled frames into 4K video...")
	}

	// If audio file is empty/non-existent, pass empty audioPath
	if fi, err := os.Stat(audioPath); err != nil || fi.Size() == 0 {
		audioPath = ""
	}

	if err := p.ffmpeg.AssembleVideo(outputFrames, audioPath, outVideo, finalFPS); err != nil {
		return "", err
	}

	if callback != nil {
		callback(100.0, fmt.Sprintf("DONE! Upscaled video saved to %s", outVideo))
	}

	return outVideo, nil
}
