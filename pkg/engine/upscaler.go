package engine

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

type Upscaler struct {
	downloader *Downloader
}

func NewUpscaler() *Upscaler {
	return &Upscaler{
		downloader: NewDownloader(),
	}
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
		"-g", "0", // NVIDIA RTX 5060 GPU
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("realesrgan execution failed: %v, output: %s", err, string(output))
	}

	return nil
}
