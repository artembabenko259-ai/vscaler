package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"vscaler/pkg/engine"
)

type ViewState int

const (
	StatePathInput ViewState = iota
	StateConfirmProcess
	StateProcessing
	StateComplete
)

type Model struct {
	State     ViewState
	Processor *engine.Processor
	PathInput textinput.Model

	// Options
	Scale      int
	Models     []string
	ModelIdx   int
	FPSTargets []float64
	FPSIdx     int

	// Real-time Progress & ETA
	CurrentFileIdx int
	TotalFiles     int
	CurrentFile    string
	ProgressPct    float64
	Elapsed        time.Duration
	ETA            time.Duration
	StatusMsg      string
	ResultPath     string
	Err            error

	Width  int
	Height int
}

type BatchProgressMsg engine.BatchProgress

type ProcessDoneMsg struct {
	ResultPath string
	Err        error
}

func cleanInputPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, `"`)
	p = strings.Trim(p, `'`)
	return filepath.ToSlash(p)
}

func NewModel() Model {
	ti := textinput.New()
	ti.Placeholder = `Paste folder or file path (e.g. C:/Users/User/Music/yt-glp)`
	ti.Focus()
	ti.CharLimit = 1000
	ti.Width = 120 // Prevent visual text truncation

	cwd, _ := os.Getwd()
	ti.SetValue(cleanInputPath(cwd))

	m := Model{
		State:      StatePathInput,
		Processor:  engine.NewProcessor(),
		PathInput:  ti,
		Scale:      4,
		Models:     []string{"realesrgan-x4plus (General Video)", "realesr-animevideov3 (Anime)"},
		ModelIdx:   0,
		FPSTargets: []float64{0.0, 60.0, 120.0},
		FPSIdx:     0,
	}

	return m
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
	}
	return k
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "Calculating..."
	}
	secs := int(d.Seconds())
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60

	if h > 0 {
		return fmt.Sprintf("%02dh %02dm %02ds", h, m, s)
	}
	return fmt.Sprintf("%02dm %02ds", m, s)
}

func renderProgressBar(pct float64, width int) string {
	if width <= 0 {
		width = 35
	}
	filledLen := int((pct / 100.0) * float64(width))
	if filledLen > width {
		filledLen = width
	}
	if filledLen < 0 {
		filledLen = 0
	}
	emptyLen := width - filledLen

	bar := strings.Repeat("█", filledLen) + strings.Repeat("░", emptyLen)
	return fmt.Sprintf("[%s] %.1f%%", bar, pct)
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		if msg.Width > 20 {
			m.PathInput.Width = msg.Width - 10
		}

	case BatchProgressMsg:
		m.CurrentFileIdx = msg.CurrentFileIdx
		m.TotalFiles = msg.TotalFiles
		m.CurrentFile = msg.CurrentFile
		m.ProgressPct = msg.Percent
		m.Elapsed = msg.Elapsed
		m.ETA = msg.ETA
		m.StatusMsg = msg.StatusMsg

	case ProcessDoneMsg:
		if msg.Err != nil {
			m.Err = msg.Err
			m.State = StatePathInput
		} else {
			m.ResultPath = msg.ResultPath
			m.State = StateComplete
		}

	case tea.KeyMsg:
		rawKey := msg.String()
		key := normalizeKey(rawKey)

		if m.State == StatePathInput {
			switch rawKey {
			case "enter":
				val := cleanInputPath(m.PathInput.Value())
				if val == "" {
					m.Err = fmt.Errorf("Please enter a valid directory or video file path")
					return m, nil
				}
				m.PathInput.SetValue(val)
				m.Err = nil
				m.State = StateConfirmProcess
				return m, nil

			case "esc":
				return m, tea.Quit
			}

			switch key {
			case "ctrl+c":
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
			}

			m.PathInput, cmd = m.PathInput.Update(msg)
			return m, cmd
		}

		switch m.State {
		case StateConfirmProcess:
			switch key {
			case "y":
				m.State = StateProcessing
				return m, m.startProcessing()
			case "n":
				m.State = StatePathInput
			}
			if rawKey == "enter" {
				m.State = StateProcessing
				return m, m.startProcessing()
			} else if rawKey == "esc" {
				m.State = StatePathInput
			}

		case StateComplete:
			switch rawKey {
			case "enter", "esc":
				m.State = StatePathInput
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

		cleanPath := cleanInputPath(m.PathInput.Value())

		opts := engine.UpscaleOptions{
			TargetPath: cleanPath,
			Scale:      m.Scale,
			ModelName:  modelName,
			TargetFPS:  m.FPSTargets[m.FPSIdx],
		}

		resPath, err := m.Processor.ProcessPath(opts, func(bp engine.BatchProgress) {
			// Progress
		})

		return ProcessDoneMsg{ResultPath: resPath, Err: err}
	}
}

func (m Model) View() string {
	var s strings.Builder

	s.WriteString(TitleStyle.Render("VSCALER - BATCH AI VIDEO UPSCALER (REAL-TIME GPU)") + "\n")

	fpsText := "Original"
	if m.FPSTargets[m.FPSIdx] > 0 {
		fpsText = fmt.Sprintf("%.0f FPS", m.FPSTargets[m.FPSIdx])
	}

	s.WriteString(fmt.Sprintf("Scale: %dx  |  Model: %s  |  FPS: %s\n",
		m.Scale, m.Models[m.ModelIdx], fpsText))
	s.WriteString(strings.Repeat("-", 65) + "\n\n")

	if m.StatusMsg != "" && m.State != StateProcessing {
		s.WriteString(SuccessStyle.Render("[OK] "+m.StatusMsg) + "\n\n")
	}
	if m.Err != nil {
		s.WriteString(ErrorStyle.Render("[ERROR] "+m.Err.Error()) + "\n\n")
	}

	switch m.State {
	case StatePathInput:
		s.WriteString(SubtitleStyle.Render("Enter or paste Target Folder / Video File Path:") + "\n\n")
		s.WriteString(m.PathInput.View() + "\n\n")
		s.WriteString(HelpStyle.Render("[Enter] Process    [S] Scale (2x/4x)    [M] Model    [F] FPS    [Esc] Exit"))

	case StateConfirmProcess:
		s.WriteString(SubtitleStyle.Render("Confirm Batch AI Video Upscaling:") + "\n\n")
		s.WriteString(fmt.Sprintf("Target Path:      %s\n", m.PathInput.Value()))
		s.WriteString(fmt.Sprintf("Output Subfolder: %s/upscale/\n", strings.TrimRight(cleanInputPath(m.PathInput.Value()), `/`)))
		s.WriteString(fmt.Sprintf("Scale Factor:     %dx\n", m.Scale))
		s.WriteString(fmt.Sprintf("AI Model:         %s\n", m.Models[m.ModelIdx]))
		s.WriteString(fmt.Sprintf("Target FPS:       %s\n\n", fpsText))
		s.WriteString(SuccessStyle.Render("Start GPU AI Upscaling into upscale/ folder? [Y/n]") + "\n\n")
		s.WriteString(HelpStyle.Render("[Y / Enter] Start    [N / Esc] Cancel"))

	case StateProcessing:
		s.WriteString(SubtitleStyle.Render("High-Speed Batch GPU AI Upscaling In Progress...") + "\n\n")

		if m.TotalFiles > 0 {
			s.WriteString(fmt.Sprintf("Batch Progress:   File %d / %d\n", m.CurrentFileIdx, m.TotalFiles))
		}
		if m.CurrentFile != "" {
			s.WriteString(fmt.Sprintf("Current File:     %s\n", m.CurrentFile))
		}

		s.WriteString("\n" + renderProgressBar(m.ProgressPct, 35) + "\n\n")

		s.WriteString(fmt.Sprintf("Time Elapsed:     %s\n", formatDuration(m.Elapsed)))
		s.WriteString(fmt.Sprintf("Time Remaining:   %s (ETA)\n\n", formatDuration(m.ETA)))

		if m.StatusMsg != "" {
			s.WriteString(MutedStyle.Render("Status: "+m.StatusMsg) + "\n")
		}

	case StateComplete:
		s.WriteString(SuccessStyle.Render("[SUCCESS] Batch AI Video Upscaling Complete!") + "\n\n")
		s.WriteString("Saved Output Directory: " + m.ResultPath + "\n\n")
		s.WriteString(HelpStyle.Render("[Enter] Process another folder/file"))
	}

	return HeaderBoxStyle.Render(s.String())
}
