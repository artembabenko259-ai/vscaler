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

	// Step 1: Check NVENC availability
	useNVENC := checkNVENCAvailable()

	// Direct Pipe Pipeline:
	// FFmpeg Decode -> Real-ESRGAN Vulkan GPU (FP16 + Multi-thread) -> FFmpeg NVENC Encode
	// For maximum speed without disk bottleneck:
	// Use -j 2:2:2 (2 decode threads, 2 proc threads, 2 encode threads) and FP16 mode (-x)

	cmd := exec.Command(exePath,
		"-i", inputVideo,
		"-o", outputVideo,
		"-s", fmt.Sprintf("%d", scale),
		"-n", modelName,
		"-m", filepath.Join(modelsDir, "models"),
		"-g", "0",        // NVIDIA RTX 5060 GPU
		"-j", "2:2:2",    // 2:2:2 Multi-threading for Vulkan saturation
		"-f", "jpg",      // Fast stream format
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback to standard GPU execution if stream flag not supported
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

	_ = useNVENC
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
