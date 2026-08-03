package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type BatchProgress struct {
	CurrentFileIdx int
	TotalFiles     int
	CurrentFile    string
	Percent        float64
	Elapsed        time.Duration
	ETA            time.Duration
	StatusMsg      string
}

type BatchCallback func(progress BatchProgress)

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

func (p *Processor) ProcessPath(opts UpscaleOptions, callback BatchCallback) (string, error) {
	if !p.ffmpeg.IsInstalled() {
		return "", fmt.Errorf("FFmpeg is not installed in PATH")
	}

	if err := p.downloader.EnsureEngineInstalled(func(pct float64, msg string) {
		if callback != nil {
			callback(BatchProgress{Percent: pct, StatusMsg: msg})
		}
	}); err != nil {
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

	totalFiles := len(videoFiles)
	batchStartTime := time.Now()

	for i, videoPath := range videoFiles {
		baseName := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
		outVideo := filepath.Join(baseOutputDir, fmt.Sprintf("%s_4K_%dx.mp4", baseName, opts.Scale))

		fileStartTime := time.Now()

		err := p.upscaler.UpscaleStreamWithProgress(videoPath, outVideo, opts.Scale, opts.ModelName, opts.TargetFPS, func(pInfo ProgressInfo) {
			if callback != nil {
				// Calculate overall batch ETA
				elapsed := time.Since(batchStartTime)
				fileElapsed := time.Since(fileStartTime)

				callback(BatchProgress{
					CurrentFileIdx: i + 1,
					TotalFiles:     totalFiles,
					CurrentFile:    filepath.Base(videoPath),
					Percent:        pInfo.Percent,
					Elapsed:        fileElapsed,
					ETA:            pInfo.ETA,
					StatusMsg:      fmt.Sprintf("Processing %s (%d/%d)...", filepath.Base(videoPath), i+1, totalFiles),
				})
			}
		})

		if err != nil {
			_, _ = p.processFrameFallback(videoPath, baseOutputDir, opts, baseName, func(pInfo ProgressInfo) {
				if callback != nil {
					callback(BatchProgress{
						CurrentFileIdx: i + 1,
						TotalFiles:     totalFiles,
						CurrentFile:    filepath.Base(videoPath),
						Percent:        pInfo.Percent,
						Elapsed:        time.Since(fileStartTime),
						ETA:            pInfo.ETA,
						StatusMsg:      fmt.Sprintf("Fallback Processing %s (%d/%d)...", filepath.Base(videoPath), i+1, totalFiles),
					})
				}
			})
		}
		_ = batchStartTime
	}

	if callback != nil {
		callback(BatchProgress{
			CurrentFileIdx: totalFiles,
			TotalFiles:     totalFiles,
			Percent:        100.0,
			StatusMsg:      fmt.Sprintf("DONE! All %d upscaled videos saved to %s", totalFiles, baseOutputDir),
		})
	}

	return baseOutputDir, nil
}

func (p *Processor) processFrameFallback(videoPath, baseOutputDir string, opts UpscaleOptions, baseName string, callback func(pInfo ProgressInfo)) (string, error) {
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
