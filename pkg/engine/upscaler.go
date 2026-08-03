package engine

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ProgressInfo struct {
	Percent   float64
	Elapsed   time.Duration
	ETA       time.Duration
	StatusMsg string
}

type Upscaler struct {
	downloader *Downloader
}

func NewUpscaler() *Upscaler {
	return &Upscaler{
		downloader: NewDownloader(),
	}
}

func (u *Upscaler) UpscaleFramesWithProgress(inputDir, outputDir string, scale int, modelName string, progressCallback func(info ProgressInfo)) error {
	exePath, err := u.downloader.GetExePath()
	if err != nil {
		return err
	}

	if scale <= 0 {
		scale = 4
	}
	if modelName == "" {
		modelName = "realesrgan-x4plus"
	}

	modelsDir := filepath.Dir(exePath)

	cmd := exec.Command(exePath,
		"-i", inputDir,
		"-o", outputDir,
		"-s", fmt.Sprintf("%d", scale),
		"-n", modelName,
		"-m", filepath.Join(modelsDir, "models"),
		"-g", "0",     // NVIDIA RTX GPU
		"-j", "2:2:2", // Vulkan Multi-threaded GPU saturation
	)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return cmd.Run()
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	startTime := time.Now()

	// Parse real-time progress percentages (e.g. 15.20%)
	go func(r io.Reader) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			pct := parsePercent(line)
			if pct > 0 {
				elapsed := time.Since(startTime)
				var eta time.Duration
				if pct > 0.5 {
					totalSecs := elapsed.Seconds() / (pct / 100.0)
					remainingSecs := totalSecs - elapsed.Seconds()
					if remainingSecs > 0 {
						eta = time.Duration(remainingSecs) * time.Second
					}
				}

				if progressCallback != nil {
					progressCallback(ProgressInfo{
						Percent:   pct,
						Elapsed:   elapsed,
						ETA:       eta,
						StatusMsg: line,
					})
				}
			}
		}
	}(stderr)

	return cmd.Wait()
}

func parsePercent(line string) float64 {
	idx := strings.Index(line, "%")
	if idx > 0 {
		start := idx - 1
		for start >= 0 && (line[start] >= '0' && line[start] <= '9' || line[start] == '.') {
			start--
		}
		numStr := line[start+1 : idx]
		if val, err := strconv.ParseFloat(numStr, 64); err == nil {
			return val
		}
	}
	return 0
}
