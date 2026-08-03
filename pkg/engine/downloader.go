package engine

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const RealESRGANZipURL = "https://github.com/xinntao/Real-ESRGAN/releases/download/v0.2.5.0/realesrgan-ncnn-vulkan-20220424-windows.zip"

type Downloader struct{}

func NewDownloader() *Downloader {
	return &Downloader{}
}

func (d *Downloader) GetEngineDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".vscaler")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func (d *Downloader) GetExePath() (string, error) {
	dir, err := d.GetEngineDir()
	if err != nil {
		return "", err
	}
	exePath := filepath.Join(dir, "realesrgan-ncnn-vulkan.exe")
	return exePath, nil
}

func (d *Downloader) IsInstalled() bool {
	exePath, err := d.GetExePath()
	if err != nil {
		return false
	}
	info, err := os.Stat(exePath)
	return err == nil && !info.IsDir()
}

func (d *Downloader) EnsureEngineInstalled(progressCallback func(percent float64, msg string)) error {
	if d.IsInstalled() {
		return nil
	}

	dir, err := d.GetEngineDir()
	if err != nil {
		return err
	}

	zipPath := filepath.Join(dir, "realesrgan.zip")

	if progressCallback != nil {
		progressCallback(10, "Downloading Real-ESRGAN Vulkan AI engine from GitHub...")
	}

	resp, err := http.Get(RealESRGANZipURL)
	if err != nil {
		return fmt.Errorf("failed to download Real-ESRGAN zip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return err
	}

	if progressCallback != nil {
		progressCallback(60, "Extracting AI model weights and binaries...")
	}

	if err := unzip(zipPath, dir); err != nil {
		return fmt.Errorf("failed to unzip Real-ESRGAN package: %v", err)
	}

	_ = os.Remove(zipPath)

	if progressCallback != nil {
		progressCallback(100, "AI Engine successfully installed!")
	}

	return nil
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// Clean destination path
		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
