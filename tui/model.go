package tui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/sznmelvin/sentinel/config"
)

// --- CONFIG ---
const (
	maxListItems = 2000
	cacheFile    = ".sentinel_cache.json"
)

var (
	bufPool = sync.Pool{New: func() interface{} { return make([]byte, 32*1024) }}
)

// --- STYLES ---
var (
	subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	highlight = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	special   = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	text      = lipgloss.AdaptiveColor{Light: "#333333", Dark: "#DDDDDD"}

	titleRendered = lipgloss.NewStyle().Foreground(special).Bold(true).Padding(0, 1).Background(subtle).Render
	docStyle      = lipgloss.NewStyle().Margin(1, 2)
	sidebarStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(highlight).Padding(0, 1).MarginRight(1).Width(25)
	mainStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(subtle).Padding(0, 1)
	itemStyle     = lipgloss.NewStyle().PaddingLeft(2).Foreground(text)
	selectedStyle = lipgloss.NewStyle().PaddingLeft(0).Foreground(highlight).SetString("│ ")
)

// --- DATA MODELS ---

type SessionState int

const (
	StateOverview SessionState = iota
	StateIssues
	StateTODOs
)

type TodoItem struct {
	File string
	Line int
	Text string
}

type Issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	User   struct {
		Login string `json:"login"`
	} `json:"user"`
}

type RepoInfo struct {
	Path       string
	Branch     string
	CommitHash string
	Clean      bool
	Todos      []TodoItem
	Issues     []Issue
	Owner      string
	RepoName   string
}

type Model struct {
	State      SessionState
	RepoPath   string
	RepoData   RepoInfo
	WindowW    int
	WindowH    int
	SidebarIdx int
	Loaded     bool
	Err        error
	Syncing    bool

	// List & Search State
	ListScroll    int
	SearchInput   textinput.Model
	Searching     bool
	
	// Dynamic filtered lists
	FilteredTodos  []TodoItem
	FilteredIssues []Issue

	// Progress tracking
	ProgressScanned int
	ProgressTotal   int
	ProgressMsg    string
	
	// Config
	Markers []string
}

func InitialModel(path string, cfg *config.Config) Model {
	ti := textinput.New()
	ti.Placeholder = "Filter..."
	ti.CharLimit = 156
	ti.Width = 30

	markers := []string{"TODO", "FIXME", "BUG", "HACK"}
	if cfg != nil && len(cfg.Markers) > 0 {
		markers = cfg.Markers
	}
	
	githubToken := os.Getenv("GITHUB_TOKEN")
	if cfg != nil && cfg.GithubToken != "" {
		githubToken = cfg.GithubToken
	}

	return Model{
		State:       StateOverview,
		RepoPath:    path,
		SearchInput: ti,
		Markers:     markers,
	}
}

// --- COMMANDS ---

type RepoMsg RepoInfo
type SyncMsg []Issue
type ProgressMsg struct {
	Scanned int
	Total   int
}
type ErrMsg error

// 1. Load Local Data
func getRepoInfo(path string, markers []string) tea.Cmd {
	markerBytes := make([][]byte, len(markers))
	for i, m := range markers {
		markerBytes[i] = []byte(m)
	}
	
	return func() tea.Msg {
		r, err := git.PlainOpen(path)
		if err != nil { return ErrMsg(err) }

		info := RepoInfo{Path: path, Todos: make([]TodoItem, 0)}

		// Git Metadata
		ref, _ := r.Head()
		if ref != nil {
			info.Branch = ref.Name().Short()
			info.CommitHash = ref.Hash().String()[:7]
			
			list, _ := r.Remotes()
			if len(list) > 0 {
				urls := list[0].Config().URLs
				if len(urls) > 0 {
					parts := strings.Split(strings.TrimSuffix(urls[0], ".git"), "/")
					if len(parts) >= 2 {
						info.RepoName = parts[len(parts)-1]
						info.Owner = parts[len(parts)-2]
					}
				}
			}
		}

		// Load Cache
		if data, err := os.ReadFile(cacheFile); err == nil {
			var cached []Issue
			if json.Unmarshal(data, &cached) == nil {
				info.Issues = cached
			}
		}

		// Scan TODOs
		if ref != nil {
			c, _ := r.CommitObject(ref.Hash())
			tree, _ := c.Tree()
			
			numWorkers := runtime.NumCPU()
			fileChan := make(chan *object.File, numWorkers*2)
			resultChan := make(chan []TodoItem, numWorkers)
			var wg sync.WaitGroup

			for i := 0; i < numWorkers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					var local []TodoItem
					bufPtr := bufPool.Get().([]byte)
					defer bufPool.Put(bufPtr)

					for f := range fileChan {
						if isIgnored(f.Name) { continue }
						
						reader, err := f.Reader()
						if err != nil { continue }
						
						header := make([]byte, 512)
						n, _ := reader.Read(header)
						if bytes.IndexByte(header[:n], 0) != -1 { reader.Close(); continue }
						reader.Close()

						reader, _ = f.Reader()
						scanner := bufio.NewScanner(reader)
						scanner.Buffer(bufPtr, len(bufPtr))
						
						lineNum := 0
						for scanner.Scan() {
							lineNum++
							lineBytes := scanner.Bytes()
							for _, mrk := range markerBytes {
								if bytes.Contains(lineBytes, mrk) {
									txt := strings.TrimSpace(string(lineBytes))
									if len(txt) > 80 { txt = txt[:80] + "..." }
									local = append(local, TodoItem{f.Name, lineNum, txt})
									break
								}
							}
							if len(local) > maxListItems/numWorkers { break }
						}
						reader.Close()
					}
					resultChan <- local
				}()
			}
			go func() {
				_ = tree.Files().ForEach(func(f *object.File) error { 
					if !isIgnored(f.Name) { fileChan <- f } 
					return nil 
				})
				close(fileChan)
			}()
			wg.Wait()
			close(resultChan)
			for res := range resultChan { info.Todos = append(info.Todos, res...) }
		}

		w, _ := r.Worktree()
		if w != nil { s, _ := w.Status(); info.Clean = s.IsClean() }

		return RepoMsg(info)
	}
}
	
	return func() tea.Msg {
		r, err := git.PlainOpen(path)
		if err != nil { return ErrMsg(err) }

		info := RepoInfo{Path: path, Todos: make([]TodoItem, 0)}

		// Git Metadata
		ref, _ := r.Head()
		if ref != nil {
			info.Branch = ref.Name().Short()
			info.CommitHash = ref.Hash().String()[:7]
			
			list, _ := r.Remotes()
			if len(list) > 0 {
				urls := list[0].Config().URLs
				if len(urls) > 0 {
					parts := strings.Split(strings.TrimSuffix(urls[0], ".git"), "/")
					if len(parts) >= 2 {
						info.RepoName = parts[len(parts)-1]
						info.Owner = parts[len(parts)-2]
					}
				}
			}
		}

		// Load Cache
		if data, err := os.ReadFile(cacheFile); err == nil {
			var cached []Issue
			if json.Unmarshal(data, &cached) == nil {
				info.Issues = cached
			}
		}

		// Scan TODOs
		if ref != nil {
			c, _ := r.CommitObject(ref.Hash())
			tree, _ := c.Tree()
			
			numWorkers := runtime.NumCPU()
			fileChan := make(chan *object.File, numWorkers*2)
			resultChan := make(chan []TodoItem, numWorkers)
			var wg sync.WaitGroup

			for i := 0; i < numWorkers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					var local []TodoItem
					bufPtr := bufPool.Get().([]byte)
					defer bufPool.Put(bufPtr)

					for f := range fileChan {
						if isIgnored(f.Name) { continue }
						reader, err := f.Reader()
						if err != nil { continue }
						
						header := make([]byte, 512)
						n, _ := reader.Read(header)
						if bytes.IndexByte(header[:n], 0) != -1 { reader.Close(); continue }
						reader.Close()

						reader, _ = f.Reader()
						scanner := bufio.NewScanner(reader)
						scanner.Buffer(bufPtr, len(bufPtr))
						
						lineNum := 0
						for scanner.Scan() {
							lineNum++
							lineBytes := scanner.Bytes()
							for _, m := range markers {
								if bytes.Contains(lineBytes, m) {
									txt := strings.TrimSpace(string(lineBytes))
									if len(txt) > 80 { txt = txt[:80] + "..." }
									local = append(local, TodoItem{f.Name, lineNum, txt})
									break
								}
							}
							if len(local) > maxListItems/numWorkers { break }
						}
						reader.Close()
					}
					resultChan <- local
				}()
			}
			go func() {
				_ = tree.Files().ForEach(func(f *object.File) error { 
					if !isIgnored(f.Name) { fileChan <- f } 
					return nil 
				})
				close(fileChan)
			}()
			wg.Wait()
			close(resultChan)
			for res := range resultChan { info.Todos = append(info.Todos, res...) }
		}

		w, _ := r.Worktree()
		if w != nil { s, _ := w.Status(); info.Clean = s.IsClean() }

		return RepoMsg(info)
	}
}

// 2. Fetch from GitHub
func fetchIssuesCmd(owner, repo string) tea.Cmd {
	return func() tea.Msg {
		token := os.Getenv("GITHUB_TOKEN")
		if token == "" { return ErrMsg(fmt.Errorf("no GITHUB_TOKEN env var set")) }

		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues?state=open&per_page=100", owner, repo)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "token "+token)
		
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil { return ErrMsg(err) }
		defer resp.Body.Close()

		if resp.StatusCode != 200 { return ErrMsg(fmt.Errorf("API error: %s", resp.Status)) }

		var issues []Issue
		if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil { return ErrMsg(err) }

		if data, err := json.MarshalIndent(issues, "", "  "); err == nil {
			_ = os.WriteFile(cacheFile, data, 0644)
		}
		return SyncMsg(issues)
	}
}

func isIgnored(path string) bool {
	if strings.Contains(path, "vendor/") { return true }
	for _, e := range []string{".png", ".jpg", ".sum", ".lock", ".pdf", ".git"} {
		if strings.HasSuffix(path, e) { return true }
	}
	return false
}

// --- INIT ---
func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.EnterAltScreen, getRepoInfo(m.RepoPath, m.Markers))
}

// --- UPDATE ---

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.Searching {
			switch msg.String() {
			case "enter", "esc":
				m.Searching = false
				m.SearchInput.Blur()
			default:
				m.SearchInput, cmd = m.SearchInput.Update(msg)
				m.updateFilteredLists()
				return m, cmd
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q": return m, tea.Quit
		case "tab":
			m.State = (m.State + 1) % 3
			m.SidebarIdx = int(m.State)
			m.ListScroll = 0
		case "/":
			if m.State == StateTODOs || m.State == StateIssues {
				m.Searching = true
				m.SearchInput.Focus()
				return m, textinput.Blink
			}
		case "s":
			if m.State == StateIssues && !m.Syncing {
				m.Syncing = true
				return m, fetchIssuesCmd(m.RepoData.Owner, m.RepoData.RepoName)
			}
		case "j", "down": m.moveListCursor(1)
		case "k", "up":   m.moveListCursor(-1)
		}

	case tea.WindowSizeMsg:
		m.WindowW, m.WindowH = msg.Width, msg.Height
		sidebarStyle = sidebarStyle.Height(msg.Height - 4)
		mainStyle = mainStyle.Width(msg.Width - 30).Height(msg.Height - 4)

	case RepoMsg:
		m.RepoData = RepoInfo(msg)
		m.updateFilteredLists()
		m.Loaded = true

	case SyncMsg:
		m.RepoData.Issues = []Issue(msg)
		m.updateFilteredLists()
		m.Syncing = false

	case ProgressMsg:
		m.ProgressScanned = msg.Scanned
		m.ProgressTotal = msg.Total
		m.ProgressMsg = fmt.Sprintf("Scanning: %d/%d files", msg.Scanned, msg.Total)

	case ErrMsg:
		m.Err = msg
		m.Syncing = false
	}
	return m, nil
}

func (m *Model) moveListCursor(delta int) {
	var listLen int
	if m.State == StateTODOs {
		listLen = len(m.FilteredTodos)
	} else if m.State == StateIssues {
		listLen = len(m.FilteredIssues)
	} else {
		m.State = SessionState(int(m.State) + delta)
		if m.State < 0 { m.State = StateOverview }
		if m.State > StateTODOs { m.State = StateTODOs }
		m.SidebarIdx = int(m.State)
		m.ListScroll = 0
		return
	}

	if listLen == 0 {
		m.State = SessionState(int(m.State) + delta)
		if m.State < 0 { m.State = StateOverview }
		if m.State > StateTODOs { m.State = StateTODOs }
		m.SidebarIdx = int(m.State)
		m.ListScroll = 0
		return
	}

	target := m.ListScroll + delta
	if delta < 0 && target < 0 {
		m.State--
		m.SidebarIdx = int(m.State)
		m.ListScroll = 0
		return
	}
	if target >= listLen {
		m.State++
		if m.State > StateTODOs { m.State = StateTODOs }
		m.SidebarIdx = int(m.State)
		m.ListScroll = 0
		return
	}
	m.ListScroll = target
}

func (m *Model) updateFilteredLists() {
	term := strings.ToLower(m.SearchInput.Value())
	
	if term == "" {
		m.FilteredTodos = m.RepoData.Todos
		m.FilteredIssues = m.RepoData.Issues
	} else {
		m.FilteredTodos = nil
		for _, t := range m.RepoData.Todos {
			if strings.Contains(strings.ToLower(t.Text), term) || strings.Contains(strings.ToLower(t.File), term) {
				m.FilteredTodos = append(m.FilteredTodos, t)
			}
		}
		m.FilteredIssues = nil
		for _, i := range m.RepoData.Issues {
			if strings.Contains(strings.ToLower(i.Title), term) || strings.Contains(strings.ToLower(i.User.Login), term) {
				m.FilteredIssues = append(m.FilteredIssues, i)
			}
		}
	}
	m.ListScroll = 0
}

// --- VIEW ---

func (m Model) View() string {
	if m.Err != nil { return fmt.Sprintf("Error: %v", m.Err) }
	if !m.Loaded { 
		loading := fmt.Sprintf("\n  Loading... (Using %d threads)", runtime.NumCPU())
		if m.ProgressMsg != "" {
			loading = "\n  " + m.ProgressMsg
		}
		return loading
	}

	var sb strings.Builder
	
	sb.WriteString(titleRendered("SENTINEL") + "\n\n")
	menu := []string{"Overview", "Issues / PRs", "Action Items"}
	for i, item := range menu {
		if i == m.SidebarIdx { sb.WriteString(selectedStyle.Render(item) + "\n") } else { sb.WriteString(itemStyle.Render(item) + "\n") }
	}
	
	sb.WriteString("\n\n")
	if m.RepoData.Owner != "" {
		sb.WriteString(itemStyle.Render(fmt.Sprintf("Repo: %s/%s", m.RepoData.Owner, m.RepoData.RepoName)) + "\n")
	}
	sb.WriteString(itemStyle.Render("Branch: " + m.RepoData.Branch))

	left := sidebarStyle.Render(sb.String())
	sb.Reset()

	switch m.State {
	case StateOverview:
		sb.WriteString(titleRendered("Repository Overview") + "\n\n")
		sb.WriteString(fmt.Sprintf("Path:   %s\n", m.RepoData.Path))
		sb.WriteString(fmt.Sprintf("Commit: %s\n", m.RepoData.CommitHash))
		status := "Clean ✨"
		if !m.RepoData.Clean { status = "Dirty 🚧" }
		sb.WriteString(fmt.Sprintf("Status: %s\n", status))
		sb.WriteString(fmt.Sprintf("\nMetrics:\n• %d Local Action Items\n• %d Cached Issues", len(m.RepoData.Todos), len(m.RepoData.Issues)))

	case StateIssues:
		header := fmt.Sprintf("Issues & PRs (%d)", len(m.FilteredIssues))
		if m.Syncing { header += " [Syncing...]" }
		sb.WriteString(titleRendered(header) + "  ")
		
		if m.Searching || m.SearchInput.Value() != "" {
			sb.WriteString(m.SearchInput.View())
		} else {
			sb.WriteString(lipgloss.NewStyle().Foreground(subtle).Render("'s' sync • '/' search"))
		}
		sb.WriteString("\n\n")

		if len(m.FilteredIssues) == 0 {
			if m.RepoData.Owner == "" {
				sb.WriteString("Could not detect GitHub remote.\nOnly local git analysis available.")
			} else {
				sb.WriteString("No issues found in cache.\nPress 's' to fetch from GitHub (requires GITHUB_TOKEN).")
			}
		} else {
			renderList(&sb, m.ListScroll, m.WindowH-6, len(m.FilteredIssues), func(i int) string {
				issue := m.FilteredIssues[i]
				return fmt.Sprintf("#%d %s [%s]", issue.Number, issue.Title, issue.User.Login)
			})
		}

	case StateTODOs:
		header := fmt.Sprintf("Action Items (%d)", len(m.FilteredTodos))
		sb.WriteString(titleRendered(header) + "  ")
		if m.Searching || m.SearchInput.Value() != "" { sb.WriteString(m.SearchInput.View()) } else { sb.WriteString(lipgloss.NewStyle().Foreground(subtle).Render("'/' search")) }
		sb.WriteString("\n\n")

		if len(m.FilteredTodos) == 0 {
			sb.WriteString("No items found.")
		} else {
			renderList(&sb, m.ListScroll, m.WindowH-6, len(m.FilteredTodos), func(i int) string {
				t := m.FilteredTodos[i]
				return fmt.Sprintf("%s:%d %s", t.File, t.Line, t.Text)
			})
		}
	}

	mainStyle = mainStyle.Width(m.WindowW - 30)
	right := mainStyle.Render(sb.String())
	help := lipgloss.NewStyle().Foreground(subtle).Render("\n  ↑/↓: navigate • tab: switch mode • /: search • s: sync • q: quit")
	return docStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, left, right) + help)
}

func renderList(sb *strings.Builder, start, height, total int, renderer func(int) string) {
	if height < 1 { height = 1 }
	end := start + height
	if end > total { end = total }
	
	for i := start; i < end; i++ {
		sb.WriteString(fmt.Sprintf("• %s\n", renderer(i)))
	}
	if total > height {
		pct := int((float64(start) / float64(total)) * 100)
		sb.WriteString(fmt.Sprintf("\n[Scroll: %d%%]", pct))
	}
}