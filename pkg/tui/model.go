package tui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"vscaler/pkg/engine"
)

type ViewState int

const (
	StateFilePicker ViewState = iota
	StateScaleSelect
	StateModelSelect
	StateFPSSelect
	StateConfirmProcess
	StateProcessing
	StateComplete
)

type FileItem struct {
	Name string
	Path string
}

type Model struct {
	State        ViewState
	Processor    *engine.Processor
	CurrentDir   string
	Files        []FileItem
	SelectedIdx  int
	SelectedPath string
	OutputDir    string

	// Upscale Options
	Scale       int      // 2 or 4
	Models      []string // realesrgan-x4plus, realesr-animevideov3
	ModelIdx    int
	FPSTargets  []float64 // 0 (original), 60, 120
	FPSIdx      int

	// Progress & State
	ProgressPercent float64
	ProgressStep    string
	ResultPath      string
	Err             error
	StatusMsg       string

	Width  int
	Height int
}

type ProgressMsg struct {
	Percent float64
	Step    string
}

type ProcessDoneMsg struct {
	ResultPath string
	Err        error
}

func NewModel() Model {
	dir, err := os.UserHomeDir()
	if err != nil || dir == "" {
		dir, _ = os.Getwd()
	}

	m := Model{
		State:       StateFilePicker,
		Processor:   engine.NewProcessor(),
		CurrentDir:  dir,
		Scale:       4, // Default 4x (1080p -> 4K)
		Models:      []string{"realesrgan-x4plus (General Video)", "realesr-animevideov3 (Anime)"},
		ModelIdx:    0,
		FPSTargets:  []float64{0.0, 60.0, 120.0},
		FPSIdx:      0,
	}

	m.loadFiles()
	return m
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

func normalizeKey(k string) string {
	k = strings.ToLower(k)
	switch k {
	case "s", "ы":
		return "s"
	case "m", "ь":
		return "m"
	case "f", "а":
		return "f"
	case "q", "й":
		return "q"
	case "y", "н":
		return "y"
	case "n", "т":
		return "n"
	case "r", "к":
		return "r"
	}
	return k
}

func (m *Model) loadFiles() {
	var items []FileItem

	_ = filepath.WalkDir(m.CurrentDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "AppData" || name == "$RECYCLE.BIN" || name == "System Volume Information" || name == "Windows" || name == "Program Files" || name == "Program Files (x86)" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(d.Name())
		if isVideoFile(ext) {
			rel, relErr := filepath.Rel(m.CurrentDir, path)
			displayName := rel
			if relErr != nil {
				displayName = d.Name()
			}
			items = append(items, FileItem{Name: displayName, Path: path})
		}
		return nil
	})

	m.Files = items
	m.SelectedIdx = 0
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height

	case ProgressMsg:
		m.ProgressPercent = msg.Percent
		m.ProgressStep = msg.Step

	case ProcessDoneMsg:
		if msg.Err != nil {
			m.Err = msg.Err
			m.State = StateFilePicker
		} else {
			m.ResultPath = msg.ResultPath
			m.State = StateComplete
		}

	case tea.KeyMsg:
		rawKey := msg.String()
		key := normalizeKey(rawKey)

		switch key {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "s":
			if m.Scale == 4 {
				m.Scale = 2
			} else {
				m.Scale = 4
			}
			m.StatusMsg = fmt.Sprintf("Scale set to %dx", m.Scale)
			return m, nil

		case "m":
			m.ModelIdx = (m.ModelIdx + 1) % len(m.Models)
			m.StatusMsg = fmt.Sprintf("AI Model set to %s", m.Models[m.ModelIdx])
			return m, nil

		case "f":
			m.FPSIdx = (m.FPSIdx + 1) % len(m.FPSTargets)
			fpsText := "Original"
			if m.FPSTargets[m.FPSIdx] > 0 {
				fpsText = fmt.Sprintf("%.0f FPS", m.FPSTargets[m.FPSIdx])
			}
			m.StatusMsg = fmt.Sprintf("Target FPS set to %s", fpsText)
			return m, nil

		case "r":
			m.loadFiles()
			m.StatusMsg = "Rescanned disk for video files"
			return m, nil
		}

		switch m.State {
		case StateFilePicker:
			switch rawKey {
			case "up", "k":
				if m.SelectedIdx > 0 {
					m.SelectedIdx--
				}
			case "down", "j":
				if m.SelectedIdx < len(m.Files)-1 {
					m.SelectedIdx++
				}
			case "enter":
				if len(m.Files) == 0 {
					break
				}
				item := m.Files[m.SelectedIdx]
				m.SelectedPath = item.Path
				m.OutputDir = filepath.Dir(item.Path)
				m.State = StateConfirmProcess
			}

		case StateConfirmProcess:
			switch key {
			case "y":
				m.State = StateProcessing
				return m, m.startProcessing()
			case "n":
				m.State = StateFilePicker
			}
			if rawKey == "enter" {
				m.State = StateProcessing
				return m, m.startProcessing()
			} else if rawKey == "esc" {
				m.State = StateFilePicker
			}

		case StateComplete:
			switch rawKey {
			case "enter", "esc":
				m.State = StateFilePicker
			}
		}
	}

	return m, nil
}

func (m Model) startProcessing() tea.Cmd {
	return func() tea.Msg {
		modelName := "realesrgan-x4plus"
		if m.ModelIdx == 1 {
			modelName = "realesr-animevideov3"
		}

		opts := engine.UpscaleOptions{
			VideoPath: m.SelectedPath,
			OutputDir: m.OutputDir,
			Scale:     m.Scale,
			ModelName: modelName,
			TargetFPS: m.FPSTargets[m.FPSIdx],
		}

		resPath, err := m.Processor.Process(opts, func(percent float64, step string) {
			// Callback
		})

		return ProcessDoneMsg{ResultPath: resPath, Err: err}
	}
}

func (m Model) View() string {
	var s strings.Builder

	s.WriteString(TitleStyle.Render("VSCALER - AI VIDEO UPSCALER (NVIDIA GPU)") + "\n")

	fpsText := "Original"
	if m.FPSTargets[m.FPSIdx] > 0 {
		fpsText = fmt.Sprintf("%.0f FPS", m.FPSTargets[m.FPSIdx])
	}

	s.WriteString(fmt.Sprintf("Scale: %dx  |  Model: %s  |  FPS: %s\n",
		m.Scale, m.Models[m.ModelIdx], fpsText))
	s.WriteString(strings.Repeat("-", 65) + "\n\n")

	if m.StatusMsg != "" {
		s.WriteString(SuccessStyle.Render("[OK] "+m.StatusMsg) + "\n\n")
	}
	if m.Err != nil {
		s.WriteString(ErrorStyle.Render("[ERROR] "+m.Err.Error()) + "\n\n")
	}

	switch m.State {
	case StateFilePicker:
		s.WriteString(SubtitleStyle.Render(fmt.Sprintf("Discovered Video Files (%d found):", len(m.Files))) + "\n")
		s.WriteString(MutedStyle.Render("Scanning User Disk: "+m.CurrentDir) + "\n\n")

		if len(m.Files) == 0 {
			s.WriteString(ItemStyle.Render("No video files (.mp4, .mkv, .mov, .avi) found."))
		} else {
			maxVisible := 10
			startIdx := 0
			if m.SelectedIdx >= maxVisible {
				startIdx = m.SelectedIdx - maxVisible + 1
			}
			endIdx := startIdx + maxVisible
			if endIdx > len(m.Files) {
				endIdx = len(m.Files)
			}

			if startIdx > 0 {
				s.WriteString(MutedStyle.Render("  ▲ ...") + "\n")
			}

			for i := startIdx; i < endIdx; i++ {
				file := m.Files[i]
				cursor := "  "
				style := ItemStyle
				if i == m.SelectedIdx {
					cursor = "> "
					style = SelectedItemStyle
				}
				s.WriteString(style.Render(cursor+file.Name) + "\n")
			}

			if endIdx < len(m.Files) {
				s.WriteString(MutedStyle.Render("  ▼ ...") + "\n")
			}
		}

		s.WriteString(HelpStyle.Render("\n[Enter] Upscale    [S] Scale (2x/4x)    [M] Model    [F] FPS    [R] Rescan    [Q] Quit"))

	case StateConfirmProcess:
		s.WriteString(SubtitleStyle.Render("Confirm AI Video Upscaling:") + "\n\n")
		s.WriteString(fmt.Sprintf("Video File: %s\n", filepath.Base(m.SelectedPath)))
		s.WriteString(fmt.Sprintf("Scale Factor: %dx\n", m.Scale))
		s.WriteString(fmt.Sprintf("AI Model:     %s\n", m.Models[m.ModelIdx]))
		s.WriteString(fmt.Sprintf("Target FPS:   %s\n\n", fpsText))
		s.WriteString(SuccessStyle.Render("Start GPU AI Upscaling now? [Y/n]") + "\n\n")
		s.WriteString(HelpStyle.Render("[Y / Enter] Start  [N / Esc] Cancel"))

	case StateProcessing:
		s.WriteString(SubtitleStyle.Render("AI Upscaling Video on GPU...") + "\n\n")
		s.WriteString(fmt.Sprintf("Video: %s\n", filepath.Base(m.SelectedPath)))
		s.WriteString(fmt.Sprintf("Scale: %dx (%s)\n\n", m.Scale, m.Models[m.ModelIdx]))
		s.WriteString(fmt.Sprintf("Progress: %.0f%%\n", m.ProgressPercent))
		s.WriteString(m.ProgressStep + "\n")

	case StateComplete:
		s.WriteString(SuccessStyle.Render("[SUCCESS] AI Video Upscaling Complete!") + "\n\n")
		s.WriteString("Saved 4K Video: " + m.ResultPath + "\n\n")
		s.WriteString(HelpStyle.Render("[Enter] Return to Video Selector"))
	}

	return HeaderBoxStyle.Render(s.String())
}
