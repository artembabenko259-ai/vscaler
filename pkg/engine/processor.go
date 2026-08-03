package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type UpscaleOptions struct {
	TargetPath string  // Can be a folder or a single video file
	Scale      int     // 2x, 4x
	ModelName  string  // realesrgan-x4plus, realesr-animevideov3
	TargetFPS  float64 // 0 = original, 60 = 60fps
}

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

func isVideoFile(ext string) bool {
	ext = strings.ToLower(ext)
	videoExts := map[string]bool{
		".mp4": true, ".mkv": true, ".mov": true, ".avi": true,
		".webm": true, ".flv": true, ".wmv": true, ".m4v": true,
		".3gp": true,
	}
	return videoExts[ext]
}

func getRAMDiskTempDir() string {
	drives := []string{"R:", "Z:", "Y:", "X:", "V:"}
	for _, drive := range drives {
		if fi, err := os.Stat(drive + "\\"); err == nil && fi.IsDir() {
			ramTemp := drive + "\\vscaler_ramtemp"
			if err := os.MkdirAll(ramTemp, 0755); err == nil {
				return ramTemp
			}
		}
	}
	return ""
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

	cleanPath := filepath.ToSlash(strings.Trim(strings.TrimSpace(opts.TargetPath), `"`))
	info, err := os.Stat(cleanPath)
	if err != nil {
		return "", fmt.Errorf("target path does not exist: %v", err)
	}

	var videoFiles []string
	var baseOutputDir string

	if info.IsDir() {
		baseOutputDir = filepath.Join(cleanPath, "upscale")
		entries, err := os.ReadDir(cleanPath)
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			if !entry.IsDir() && isVideoFile(filepath.Ext(entry.Name())) {
				videoFiles = append(videoFiles, filepath.Join(cleanPath, entry.Name()))
			}
		}
		if len(videoFiles) == 0 {
			return "", fmt.Errorf("no video files (.mp4, .mkv, .mov, .webm) found in directory %s", cleanPath)
		}
	} else {
		baseOutputDir = filepath.Join(filepath.Dir(cleanPath), "upscale")
		videoFiles = append(videoFiles, cleanPath)
	}

	if err := os.MkdirAll(baseOutputDir, 0755); err != nil {
		return "", err
	}

	totalFiles := len(videoFiles)

	for i, videoPath := range videoFiles {
		baseName := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
		outVideo := filepath.Join(baseOutputDir, fmt.Sprintf("%s_4K_%dx.mp4", baseName, opts.Scale))

		fileStartTime := time.Now()

		ramTemp := getRAMDiskTempDir()
		var tempDir string
		if ramTemp != "" {
			tempDir = filepath.Join(ramTemp, baseName+"_vscaler_temp")
		} else {
			tempDir = filepath.Join(baseOutputDir, baseName+"_vscaler_temp")
		}

		inputFrames := filepath.Join(tempDir, "in")
		outputFrames := filepath.Join(tempDir, "out")
		audioPath := filepath.Join(tempDir, "audio.aac")

		_ = os.MkdirAll(inputFrames, 0755)
		_ = os.MkdirAll(outputFrames, 0755)

		// Step 1: Extract Frames & Audio
		if callback != nil {
			callback(BatchProgress{
				CurrentFileIdx: i + 1,
				TotalFiles:     totalFiles,
				CurrentFile:    filepath.Base(videoPath),
				Percent:        5.0,
				StatusMsg:      "Extracting video frames and audio...",
			})
		}

		if err := p.ffmpeg.ExtractFrames(videoPath, inputFrames); err != nil {
			_ = os.RemoveAll(tempDir)
			return "", fmt.Errorf("failed to extract frames: %v", err)
		}
		_ = p.ffmpeg.ExtractAudio(videoPath, audioPath)

		origFPS, _ := p.ffmpeg.GetVideoFPS(videoPath)
		finalFPS := origFPS
		if opts.TargetFPS > 0 {
			finalFPS = opts.TargetFPS
		}

		// Step 2: GPU AI Frame Upscaling with Real-Time ETA
		if callback != nil {
			callback(BatchProgress{
				CurrentFileIdx: i + 1,
				TotalFiles:     totalFiles,
				CurrentFile:    filepath.Base(videoPath),
				Percent:        10.0,
				StatusMsg:      fmt.Sprintf("AI GPU Upscaling on RTX 5060 (%dx %s)...", opts.Scale, opts.ModelName),
			})
		}

		err = p.upscaler.UpscaleFramesWithProgress(inputFrames, outputFrames, opts.Scale, opts.ModelName, func(pInfo ProgressInfo) {
			if callback != nil {
				fileElapsed := time.Since(fileStartTime)
				callback(BatchProgress{
					CurrentFileIdx: i + 1,
					TotalFiles:     totalFiles,
					CurrentFile:    filepath.Base(videoPath),
					Percent:        pInfo.Percent,
					Elapsed:        fileElapsed,
					ETA:            pInfo.ETA,
					StatusMsg:      fmt.Sprintf("AI Upscaling %s (%d/%d)...", filepath.Base(videoPath), i+1, totalFiles),
				})
			}
		})

		if err != nil {
			_ = os.RemoveAll(tempDir)
			return "", fmt.Errorf("AI upscaler failed: %v", err)
		}

		// Step 3: NVENC Video Assembly
		if callback != nil {
			callback(BatchProgress{
				CurrentFileIdx: i + 1,
				TotalFiles:     totalFiles,
				CurrentFile:    filepath.Base(videoPath),
				Percent:        90.0,
				StatusMsg:      "NVENC Assembling upscaled frames into 4K video...",
			})
		}

		if fi, err := os.Stat(audioPath); err != nil || fi.Size() == 0 {
			audioPath = ""
		}

		if err := p.ffmpeg.AssembleVideo(outputFrames, audioPath, outVideo, finalFPS); err != nil {
			_ = os.RemoveAll(tempDir)
			return "", fmt.Errorf("failed to assemble video: %v", err)
		}

		_ = os.RemoveAll(tempDir)
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
