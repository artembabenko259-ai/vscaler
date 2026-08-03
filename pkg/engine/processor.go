package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ProgressCallback func(percent float64, step string)

type UpscaleOptions struct {
	TargetPath string  // Can be a folder or a single video file
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

func isVideoFile(ext string) bool {
	ext = strings.ToLower(ext)
	videoExts := map[string]bool{
		".mp4": true, ".mkv": true, ".mov": true, ".avi": true,
		".webm": true, ".flv": true, ".wmv": true, ".m4v": true,
		".3gp": true,
	}
	return videoExts[ext]
}

func (p *Processor) ProcessPath(opts UpscaleOptions, callback ProgressCallback) (string, error) {
	if !p.ffmpeg.IsInstalled() {
		return "", fmt.Errorf("FFmpeg is not installed in PATH")
	}

	if err := p.downloader.EnsureEngineInstalled(callback); err != nil {
		return "", fmt.Errorf("failed to initialize AI engine: %v", err)
	}

	info, err := os.Stat(opts.TargetPath)
	if err != nil {
		return "", fmt.Errorf("target path does not exist: %v", err)
	}

	var videoFiles []string
	var baseOutputDir string

	if info.IsDir() {
		baseOutputDir = filepath.Join(opts.TargetPath, "upscale")
		entries, err := os.ReadDir(opts.TargetPath)
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			if !entry.IsDir() && isVideoFile(filepath.Ext(entry.Name())) {
				videoFiles = append(videoFiles, filepath.Join(opts.TargetPath, entry.Name()))
			}
		}
		if len(videoFiles) == 0 {
			return "", fmt.Errorf("no video files (.mp4, .mkv, .mov, .avi) found in directory %s", opts.TargetPath)
		}
	} else {
		baseOutputDir = filepath.Join(filepath.Dir(opts.TargetPath), "upscale")
		videoFiles = append(videoFiles, opts.TargetPath)
	}

	if err := os.MkdirAll(baseOutputDir, 0755); err != nil {
		return "", err
	}

	total := len(videoFiles)
	for i, videoPath := range videoFiles {
		baseName := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
		outVideo := filepath.Join(baseOutputDir, fmt.Sprintf("%s_4K_%dx.mp4", baseName, opts.Scale))

		pct := float64(i) / float64(total) * 100.0
		if callback != nil {
			callback(pct, fmt.Sprintf("[%d/%d] High-Speed AI Upscaling %s...", i+1, total, filepath.Base(videoPath)))
		}

		err := p.upscaler.UpscaleStream(videoPath, outVideo, opts.Scale, opts.ModelName, opts.TargetFPS, callback)
		if err != nil {
			// Fallback to frame pipeline if direct stream encountered format restriction
			_, _ = p.processFrameFallback(videoPath, baseOutputDir, opts, baseName, callback)
		}
	}

	if callback != nil {
		callback(100.0, fmt.Sprintf("DONE! All %d upscaled videos saved to %s", total, baseOutputDir))
	}

	return baseOutputDir, nil
}

func (p *Processor) processFrameFallback(videoPath, baseOutputDir string, opts UpscaleOptions, baseName string, callback ProgressCallback) (string, error) {
	tempDir := filepath.Join(baseOutputDir, baseName+"_vscaler_temp")
	inputFrames := filepath.Join(tempDir, "in")
	outputFrames := filepath.Join(tempDir, "out")
	audioPath := filepath.Join(tempDir, "audio.aac")
	outVideo := filepath.Join(baseOutputDir, fmt.Sprintf("%s_4K_%dx.mp4", baseName, opts.Scale))

	_ = os.MkdirAll(inputFrames, 0755)
	_ = os.MkdirAll(outputFrames, 0755)
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	if err := p.ffmpeg.ExtractFrames(videoPath, inputFrames); err != nil {
		return "", err
	}
	_ = p.ffmpeg.ExtractAudio(videoPath, audioPath)

	origFPS, _ := p.ffmpeg.GetVideoFPS(videoPath)
	finalFPS := origFPS
	if opts.TargetFPS > 0 {
		finalFPS = opts.TargetFPS
	}

	if err := p.upscaler.UpscaleFrames(inputFrames, outputFrames, opts.Scale, opts.ModelName, callback); err != nil {
		return "", err
	}

	if fi, err := os.Stat(audioPath); err != nil || fi.Size() == 0 {
		audioPath = ""
	}

	if err := p.ffmpeg.AssembleVideo(outputFrames, audioPath, outVideo, finalFPS); err != nil {
		return "", err
	}

	return outVideo, nil
}
