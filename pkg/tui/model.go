package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	ti := textinput.New()
	ti.Placeholder = `Paste folder or file path (e.g. C:\Videos\Interns)`
	ti.Focus()
	ti.CharLimit = 500

	cwd, _ := os.Getwd()
	ti.SetValue(cwd)

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

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

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
				val := strings.TrimSpace(m.PathInput.Value())
				val = strings.Trim(val, `"`) // Strip quotes from drag & drop
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

			// Options toggles when not actively editing text
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

		opts := engine.UpscaleOptions{
			TargetPath: strings.Trim(strings.TrimSpace(m.PathInput.Value()), `"`),
			Scale:      m.Scale,
			ModelName:  modelName,
			TargetFPS:  m.FPSTargets[m.FPSIdx],
		}

		resPath, err := m.Processor.ProcessPath(opts, func(percent float64, step string) {
			// Callback
		})

		return ProcessDoneMsg{ResultPath: resPath, Err: err}
	}
}

func (m Model) View() string {
	var s strings.Builder

	s.WriteString(TitleStyle.Render("VSCALER - BATCH AI VIDEO UPSCALER") + "\n")

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
	case StatePathInput:
		s.WriteString(SubtitleStyle.Render("Enter or paste Target Folder / Video File Path:") + "\n\n")
		s.WriteString(m.PathInput.View() + "\n\n")
		s.WriteString(HelpStyle.Render("[Enter] Process    [S] Scale (2x/4x)    [M] Model    [F] FPS    [Esc] Exit"))

	case StateConfirmProcess:
		s.WriteString(SubtitleStyle.Render("Confirm Batch AI Video Upscaling:") + "\n\n")
		s.WriteString(fmt.Sprintf("Target Path:  %s\n", m.PathInput.Value()))
		s.WriteString(fmt.Sprintf("Output Subfolder: %s\\upscale\\\n", strings.TrimRight(m.PathInput.Value(), `\/`)))
		s.WriteString(fmt.Sprintf("Scale Factor: %dx\n", m.Scale))
		s.WriteString(fmt.Sprintf("AI Model:     %s\n", m.Models[m.ModelIdx]))
		s.WriteString(fmt.Sprintf("Target FPS:   %s\n\n", fpsText))
		s.WriteString(SuccessStyle.Render("Start batch processing into upscale/ folder? [Y/n]") + "\n\n")
		s.WriteString(HelpStyle.Render("[Y / Enter] Start    [N / Esc] Cancel"))

	case StateProcessing:
		s.WriteString(SubtitleStyle.Render("High-Speed Batch GPU AI Upscaling...") + "\n\n")
		s.WriteString(fmt.Sprintf("Target: %s\n", filepath.Base(m.PathInput.Value())))
		s.WriteString(fmt.Sprintf("Scale:  %dx (%s)\n\n", m.Scale, m.Models[m.ModelIdx]))
		s.WriteString(fmt.Sprintf("Progress: %.0f%%\n", m.ProgressPercent))
		s.WriteString(m.ProgressStep + "\n")

	case StateComplete:
		s.WriteString(SuccessStyle.Render("[SUCCESS] Batch AI Video Upscaling Complete!") + "\n\n")
		s.WriteString("Saved Output Directory: " + m.ResultPath + "\n\n")
		s.WriteString(HelpStyle.Render("[Enter] Process another folder/file"))
	}

	return HeaderBoxStyle.Render(s.String())
}
