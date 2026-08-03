package engine

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type Upscaler struct {
	downloader *Downloader
}

func NewUpscaler() *Upscaler {
	return &Upscaler{
		downloader: NewDownloader(),
	}
}

func (u *Upscaler) UpscaleStream(inputVideo, outputVideo string, scale int, modelName string, targetFPS float64, progressCallback func(percent float64, msg string)) error {
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
		"-i", inputVideo,
		"-o", outputVideo,
		"-s", fmt.Sprintf("%d", scale),
		"-n", modelName,
		"-m", filepath.Join(modelsDir, "models"),
		"-g", "0",     // NVIDIA RTX 5060 GPU
		"-j", "2:2:2", // 2:2:2 Multi-threading for Vulkan saturation
		"-f", "jpg",   // Fast stream format
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		cmdFallback := exec.Command(exePath,
			"-i", inputVideo,
			"-o", outputVideo,
			"-s", fmt.Sprintf("%d", scale),
			"-n", modelName,
			"-m", filepath.Join(modelsDir, "models"),
			"-g", "0",
			"-j", "2:2:2",
		)
		output, err = cmdFallback.CombinedOutput()
		if err != nil {
			return fmt.Errorf("realesrgan GPU execution failed: %v, output: %s", err, string(output))
		}
	}

	return nil
}

func (u *Upscaler) UpscaleFrames(inputDir, outputDir string, scale int, modelName string, progressCallback func(percent float64, msg string)) error {
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
		"-g", "0",
		"-j", "2:2:2",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("realesrgan frame upscale failed: %v, output: %s", err, string(output))
	}

	return nil
}

func checkNVENCAvailable() bool {
	cmd := exec.Command("ffmpeg", "-hide_banner", "-encoders")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "h264_nvenc") || strings.Contains(string(out), "hevc_nvenc")
}
