package engine

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type FFmpeg struct{}

func NewFFmpeg() *FFmpeg {
	return &FFmpeg{}
}

func (f *FFmpeg) IsInstalled() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

func (f *FFmpeg) ExtractFrames(videoPath, framesDir string) error {
	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", videoPath,
		"-qscale:v", "2",
		filepathJoin(framesDir, "%08d.png"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg extract frames failed: %v, output: %s", err, string(output))
	}
	return nil
}

func (f *FFmpeg) ExtractAudio(videoPath, audioPath string) error {
	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", videoPath,
		"-vn",
		"-c:a", "copy",
		audioPath,
	)
	_, err := cmd.CombinedOutput()
	if err != nil {
		cmdFallback := exec.Command("ffmpeg", "-y", "-i", videoPath, "-vn", "-acodec", "libmp3lame", audioPath)
		_ = cmdFallback.Run()
	}
	return nil
}

func (f *FFmpeg) GetVideoFPS(videoPath string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "0",
		"-of", "csv=p=0",
		"-select_streams", "v:0",
		"-show_entries", "stream=r_frame_rate",
		videoPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 30.0, nil
	}

	str := strings.TrimSpace(string(output))
	parts := strings.Split(str, "/")
	if len(parts) == 2 {
		num, _ := strconv.ParseFloat(parts[0], 64)
		den, _ := strconv.ParseFloat(parts[1], 64)
		if den > 0 {
			return num / den, nil
		}
	}

	return 30.0, nil
}

func (f *FFmpeg) AssembleVideo(framesDir, audioPath, outputPath string, fps float64) error {
	args := []string{
		"-y",
		"-r", fmt.Sprintf("%.2f", fps),
		"-i", filepathJoin(framesDir, "%08d.png"),
	}

	if audioPath != "" {
		args = append(args, "-i", audioPath)
	}

	args = append(args,
		"-c:v", "libx264",
		"-crf", "18",
		"-pix_fmt", "yuv420p",
	)

	if audioPath != "" {
		args = append(args, "-c:a", "aac", "-shortest")
	}

	args = append(args, outputPath)

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg assemble video failed: %v, output: %s", err, string(output))
	}
	return nil
}

func filepathJoin(elem ...string) string {
	return strings.Join(elem, "/")
}
