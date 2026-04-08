package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// ConversationModel manages the message list and text input.
type ConversationModel struct {
	messages     []Message
	input        string
	cursorPos    int
	width        int
	height       int
	focused      bool
	scrollOffset int
}

// NewConversationModel creates an empty ConversationModel.
func NewConversationModel() ConversationModel {
	return ConversationModel{
		focused: true,
	}
}

// Init returns the initial command for the conversation.
func (m ConversationModel) Init() tea.Cmd {
	return nil
}

// SetSize updates the conversation dimensions.
func (m *ConversationModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// AddMessage appends a message and scrolls to the bottom.
func (m *ConversationModel) AddMessage(msg Message) {
	m.messages = append(m.messages, msg)
	m.scrollToBottom()
}

// RemoveLastSystem removes the most recent system message (used to clear "Thinking...").
func (m *ConversationModel) RemoveLastSystem() {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == RoleSystem {
			m.messages = append(m.messages[:i], m.messages[i+1:]...)
			return
		}
	}
}

// UpdateLastSystem updates the content of the most recent system message.
func (m *ConversationModel) UpdateLastSystem(content string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == RoleSystem {
			m.messages[i].Content = content
			return
		}
	}
}

// Update handles key events for the conversation.
func (m ConversationModel) Update(msg tea.Msg) (ConversationModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			trimmed := strings.TrimSpace(m.input)
			if trimmed == "" {
				return m, nil
			}
			m.input = ""
			m.cursorPos = 0

			// Handle slash commands
			lower := strings.ToLower(trimmed)
			switch {
			case lower == "/undo":
				m.AddMessage(Message{Role: RoleUser, Content: trimmed})
				m.AddMessage(Message{Role: RoleSystem, Content: "Undoing..."})
				return m, func() tea.Msg { return UndoRequestMsg{} }
			case lower == "/quit" || lower == "/exit":
				return m, tea.Quit
			case lower == "/routes":
				m.AddMessage(Message{Role: RoleUser, Content: trimmed})
				return m, func() tea.Msg { return RoutesRequestMsg{} }
			case lower == "/status":
				m.AddMessage(Message{Role: RoleUser, Content: trimmed})
				return m, func() tea.Msg { return StatusRequestMsg{} }
			case lower == "/help":
				m.AddMessage(Message{Role: RoleUser, Content: trimmed})
				m.AddMessage(Message{Role: RoleAssistant, Content: "Commands:\n  /routes — List all API routes\n  /status — Show project status\n  /undo   — Rollback last change\n  /help   — Show this help\n  /quit   — Exit VibeServe\n  Ctrl+C  — Quit immediately\n\nAnything else is sent to the AI to create/modify your API."})
				return m, nil
			default:
				// Send as prompt to AI
				return m, func() tea.Msg { return SubmitPromptMsg(trimmed) }
			}

		case "backspace":
			if m.cursorPos > 0 && len(m.input) > 0 {
				m.input = m.input[:m.cursorPos-1] + m.input[m.cursorPos:]
				m.cursorPos--
			}

		case "left":
			if m.cursorPos > 0 {
				m.cursorPos--
			}

		case "right":
			if m.cursorPos < len(m.input) {
				m.cursorPos++
			}

		case "home", "ctrl+a":
			m.cursorPos = 0

		case "end", "ctrl+e":
			m.cursorPos = len(m.input)

		case "ctrl+u":
			m.input = m.input[m.cursorPos:]
			m.cursorPos = 0

		case "ctrl+k":
			m.input = m.input[:m.cursorPos]

		case "pgup":
			m.scrollUp(10)

		case "pgdown":
			m.scrollDown(10)

		default:
			// Printable character input
			text := msg.Text
			if len(text) > 0 && text[0] >= 32 {
				m.input = m.input[:m.cursorPos] + text + m.input[m.cursorPos:]
				m.cursorPos += len(text)
			}
		}
	}

	return m, nil
}

// View renders the conversation pane.
func (m ConversationModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	// Reserve 3 lines for input area (border + input + border)
	inputAreaHeight := 3
	msgAreaHeight := m.height - inputAreaHeight
	if msgAreaHeight < 1 {
		msgAreaHeight = 1
	}

	// Render message list
	msgView := m.renderMessages(msgAreaHeight)

	// Render input field
	inputView := m.renderInput()

	return lipgloss.JoinVertical(lipgloss.Left, msgView, inputView)
}

// renderWelcome renders the welcome screen shown on startup.
func (m ConversationModel) renderWelcome(height int) string {
	w := m.width - 2
	if w < 20 {
		w = 20
	}

	bannerStyle := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	cmdKeyStyle := lipgloss.NewStyle().Foreground(colorSecondary).Bold(true)
	cmdDescStyle := lipgloss.NewStyle().Foreground(colorMuted)
	welcomeStyle := lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)

	var lines []string
	lines = append(lines, "")
	lines = append(lines, bannerStyle.Render("  __     __ _ _          ____"))
	lines = append(lines, bannerStyle.Render(`  \ \   / /(_)| |__   __/ ___|  ___ _ ____   _____`))
	lines = append(lines, bannerStyle.Render(`   \ \ / / | || '_ \ / _ \___ \ / _ \ '__\ \ / / _ \`))
	lines = append(lines, bannerStyle.Render(`    \ V /  | || |_) |  __/___) |  __/ |   \ V /  __/`))
	lines = append(lines, bannerStyle.Render(`     \_/   |_||_.__/ \___|____/ \___|_|    \_/ \___|`))
	lines = append(lines, "")
	lines = append(lines, "")
	lines = append(lines, "  "+cmdKeyStyle.Render("Commands:"))
	lines = append(lines, "    "+cmdKeyStyle.Render("/help")+"       "+cmdDescStyle.Render("Show available commands"))
	lines = append(lines, "    "+cmdKeyStyle.Render("/undo")+"       "+cmdDescStyle.Render("Rollback last change"))
	lines = append(lines, "    "+cmdKeyStyle.Render("/quit")+"       "+cmdDescStyle.Render("Exit VibeServe"))
	lines = append(lines, "")
	lines = append(lines, "  "+cmdKeyStyle.Render("Shortcuts:"))
	lines = append(lines, "    "+cmdKeyStyle.Render("Tab")+"         "+cmdDescStyle.Render("Switch pane"))
	lines = append(lines, "    "+cmdKeyStyle.Render("Esc")+"         "+cmdDescStyle.Render("Back to chat"))
	lines = append(lines, "    "+cmdKeyStyle.Render("Ctrl+C")+"      "+cmdDescStyle.Render("Quit immediately"))
	lines = append(lines, "")
	lines = append(lines, "  "+welcomeStyle.Render("Welcome to VibeServe! Type your message to create an API."))

	content := strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		Width(w).
		Height(height).
		Padding(1, 0).
		Render(content)
}

// renderMessages renders the scrollable message list.
func (m ConversationModel) renderMessages(height int) string {
	if len(m.messages) == 0 {
		return m.renderWelcome(height)
	}

	// Build all rendered message lines
	var lines []string
	contentWidth := m.width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	for _, msg := range m.messages {
		var rendered string
		switch msg.Role {
		case RoleUser:
			prefix := styleUserMsg.Render("You: ")
			body := lipgloss.NewStyle().
				Foreground(colorText).
				Width(contentWidth - lipgloss.Width(prefix)).
				Render(msg.Content)
			rendered = prefix + body
		case RoleAssistant:
			prefix := styleAssistantMsg.Render("VibeServe: ")
			body := lipgloss.NewStyle().
				Foreground(colorText).
				Width(contentWidth - lipgloss.Width(prefix)).
				Render(msg.Content)
			rendered = prefix + body
		case RoleSystem:
			rendered = styleSystemMsg.Width(contentWidth).Render("✓ " + msg.Content)
		case RoleError:
			rendered = styleErrorMsg.Width(contentWidth).Render("✗ " + msg.Content)
		}

		msgLines := strings.Split(rendered, "\n")
		lines = append(lines, msgLines...)
		lines = append(lines, "") // blank line between messages
	}

	// Apply scroll offset
	visibleStart := m.scrollOffset
	if visibleStart > len(lines) {
		visibleStart = len(lines)
	}
	visibleLines := lines[visibleStart:]

	// Truncate to fit height
	if len(visibleLines) > height {
		visibleLines = visibleLines[len(visibleLines)-height:]
	}

	// Pad if too few lines
	for len(visibleLines) < height {
		visibleLines = append([]string{""}, visibleLines...)
	}

	return lipgloss.NewStyle().
		Width(m.width).
		Height(height).
		Render(strings.Join(visibleLines, "\n"))
}

// renderInput renders the input field with a prompt indicator.
func (m ConversationModel) renderInput() string {
	prompt := stylePrompt.Render("vibe> ")
	promptWidth := lipgloss.Width(prompt)
	inputWidth := m.width - promptWidth - 4
	if inputWidth < 1 {
		inputWidth = 1
	}

	// Show cursor
	displayInput := m.input
	if m.focused {
		if m.cursorPos >= len(displayInput) {
			displayInput += "\u2588" // block cursor at end
		} else {
			displayInput = displayInput[:m.cursorPos] + "\u2588" + displayInput[m.cursorPos+1:]
		}
	}

	// Truncate if too long for display
	if len(displayInput) > inputWidth {
		start := len(displayInput) - inputWidth
		displayInput = displayInput[start:]
	}

	inputContent := lipgloss.NewStyle().
		Foreground(colorText).
		Width(inputWidth).
		Render(displayInput)

	line := prompt + inputContent

	borderStyle := styleBorderDim
	if m.focused {
		borderStyle = styleBorderFocused
	}

	return borderStyle.
		Width(m.width - 2). // account for border
		Render(line)
}

func (m *ConversationModel) scrollToBottom() {
	// Reset offset so renderMessages shows the newest messages
	m.scrollOffset = 0
}

func (m *ConversationModel) scrollUp(n int) {
	m.scrollOffset += n
	totalLines := m.countTotalLines()
	if m.scrollOffset > totalLines {
		m.scrollOffset = totalLines
	}
}

func (m *ConversationModel) scrollDown(n int) {
	m.scrollOffset -= n
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m ConversationModel) countTotalLines() int {
	count := 0
	for range m.messages {
		count += 2 // rough estimate: each message ~ 1 line + blank
	}
	return count
}

// String renders message content for display (used for fmt.Sprintf compatibility).
func (r Role) String() string {
	switch r {
	case RoleUser:
		return "user"
	case RoleAssistant:
		return "assistant"
	case RoleSystem:
		return "system"
	case RoleError:
		return "error"
	default:
		return fmt.Sprintf("unknown(%d)", int(r))
	}
}
