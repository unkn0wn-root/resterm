package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
)

var newFileExtensions = []string{".http", ".rest"}

func (m *Model) openNewFileModal() {
	m.prepareNewFileModal(false)
}

func (m *Model) openSaveAsModal() {
	m.prepareNewFileModal(true)
}

func (m *Model) prepareNewFileModal(fromSave bool) {
	m.showNewFileModal = true
	m.newFileError = ""
	m.newFileExtIndex = 0
	m.newFileInput.SetValue("")
	m.newFileInput.Focus()
	m.showHelp = false
	m.showEnvSelector = false
	m.showThemeSelector = false
	m.closeOpenModal()
	m.newFileFromSave = fromSave
}

func (m *Model) closeNewFileModal() {
	m.showNewFileModal = false
	m.newFileError = ""
	m.newFileInput.Blur()
	m.newFileInput.SetValue("")
	m.newFileFromSave = false
}

func (m *Model) cycleNewFileExtension(delta int) {
	count := len(newFileExtensions)
	if count == 0 {
		return
	}
	m.newFileExtIndex = (m.newFileExtIndex + delta) % count
	if m.newFileExtIndex < 0 {
		m.newFileExtIndex += count
	}
}

func (m *Model) submitNewFile() tea.Cmd {
	name := strings.TrimSpace(m.newFileInput.Value())
	if name == "" {
		m.newFileError = "Enter a file name"
		return nil
	}

	selectedExt := newFileExtensions[m.newFileExtIndex]
	cleanInput := filepath.Clean(name)
	if cleanInput == "." || cleanInput == ".." {
		m.newFileError = "Provide a valid file name"
		return nil
	}

	inputExt := strings.ToLower(filepath.Ext(cleanInput))
	switch inputExt {
	case "":
		cleanInput += selectedExt
	case ".http", ".rest":
		if inputExt != selectedExt {
			m.newFileError = fmt.Sprintf("Use the %s extension or change selection", selectedExt)
			return nil
		}
	default:
		m.newFileError = "Only .http or .rest files are supported"
		return nil
	}

	finalPath := cleanInput
	if !filepath.IsAbs(finalPath) {
		finalPath = filepath.Join(m.workspaceRoot, finalPath)
	}
	finalPath = filepath.Clean(finalPath)
	if !m.ensureWorkspaceFile(finalPath) {
		m.newFileError = "File must be inside the workspace"
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		m.newFileError = fmt.Sprintf("create directories: %v", err)
		return nil
	}

	if _, err := os.Stat(finalPath); err == nil {
		m.newFileError = "File already exists"
		return nil
	} else if !os.IsNotExist(err) {
		m.newFileError = fmt.Sprintf("check file: %v", err)
		return nil
	}

	content := []byte("")
	if m.newFileFromSave {
		content = []byte(m.editor.Value())
	}

	if err := os.WriteFile(finalPath, content, 0o644); err != nil {
		m.newFileError = fmt.Sprintf("create file: %v", err)
		return nil
	}

	fromSave := m.newFileFromSave
	m.closeNewFileModal()
	entries, err := filesvc.ListRequestFiles(m.workspaceRoot, m.workspaceRecursive)
	if err != nil {
		return func() tea.Msg {
			return statusMsg{text: fmt.Sprintf("workspace error: %v", err), level: statusError}
		}
	}
	m.fileList.SetItems(makeFileItems(entries))
	m.selectFileByPath(finalPath)
	focusCmd := m.setFocus(focusEditor)
	cmd := m.openFile(finalPath)
	label := "Created"
	if fromSave {
		label = "Saved"
	}
	m.setStatusMessage(
		statusMsg{
			text:  fmt.Sprintf("%s %s", label, filepath.Base(finalPath)),
			level: statusSuccess,
		},
	)
	return batchCommands(focusCmd, cmd)
}
