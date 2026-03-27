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

// --- STYLES (THE HACKER REVAMP) ---
var (
	bloodRed  = lipgloss.Color("#FF0000")
	darkRed   = lipgloss.Color("#8B0000")
	dimText   = lipgloss.Color("#777777")
	whiteText = lipgloss.Color("#FFFFFF")

	titleRendered = lipgloss.NewStyle().Foreground(bloodRed).Bold(true).Render
	docStyle      = lipgloss.NewStyle().Margin(1, 2)
	
	// Sharp, angular borders replacing the rounded ones
	sidebarStyle  = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(darkRed).Padding(0, 1).MarginRight(1).Width(30)
	mainStyle     = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(darkRed).Padding(0, 1)
	
	// List items
	itemStyle     = lipgloss.NewStyle().PaddingLeft(2).Foreground(dimText)
	selectedStyle = lipgloss.NewStyle().PaddingLeft(0).Foreground(bloodRed).Bold(true).SetString("► ")
	
	// Terminal output style
	bracketStyle  = lipgloss.NewStyle().Foreground(darkRed)
)

const sentinelAscii = `
 ███████╗███████╗███╗   ██╗████████╗██╗███╗   ██╗███████╗██╗
 ██╔════╝██╔════╝████╗  ██║╚══██╔══╝██║████╗  ██║██╔════╝██║
 ███████╗█████╗  ██╔██╗ ██║   ██║   ██║██╔██╗ ██║█████╗  ██║
 ╚════██║██╔══╝  ██║╚██╗██║   ██║   ██║██║╚██╗██║██╔══╝  ██║
 ███████║███████╗██║ ╚████║   ██║   ██║██║ ╚████║███████╗███████╗
 ╚══════╝╚══════╝╚═╝  ╚═══╝   ╚═╝   ╚═╝╚═╝  ╚═══╝╚══════╝╚══════╝
`

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
	ti.Placeholder = "filter_target..."
	ti.Prompt = "[>] "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(bloodRed)
	ti.TextStyle = lipgloss.NewStyle().Foreground(whiteText)
	ti.CharLimit = 156
	ti.Width = 30

	markers := []string{"TODO", "FIXME", "BUG", "HACK"}
	if cfg != nil && len(cfg.Markers) > 0 {
		markers = cfg.Markers
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
		mainStyle = mainStyle.Width(msg.Width - 35).Height(msg.Height - 4)

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
	if m.Err != nil { return lipgloss.NewStyle().Foreground(bloodRed).Render(fmt.Sprintf("\n [!] FATAL ERROR: %v\n", m.Err)) }
	if !m.Loaded { 
		loading := fmt.Sprintf("\n  [+] INITIALIZING... (THREADS: %d)", runtime.NumCPU())
		if m.ProgressMsg != "" {
			loading = "\n  [>] " + m.ProgressMsg
		}
		return lipgloss.NewStyle().Foreground(bloodRed).Render(loading)
	}

	var sb strings.Builder
	
	sb.WriteString(titleRendered("S E N T I N E L") + "\n\n")
	menu := []string{"System Overview", "Remote Targets", "Local Artifacts"}
	for i, item := range menu {
		if i == m.SidebarIdx { 
			sb.WriteString(selectedStyle.Render(item) + "\n") 
		} else { 
			sb.WriteString(itemStyle.Render(item) + "\n") 
		}
	}
	
	sb.WriteString("\n\n")
	if m.RepoData.Owner != "" {
		sb.WriteString(itemStyle.Render(fmt.Sprintf("Target: %s/%s", m.RepoData.Owner, m.RepoData.RepoName)) + "\n")
	}
	sb.WriteString(itemStyle.Render("Branch: " + m.RepoData.Branch))

	left := sidebarStyle.Render(sb.String())
	sb.Reset()

	switch m.State {
	case StateOverview:
		// Insert Massive ASCII art here
		sb.WriteString(titleRendered(sentinelAscii) + "\n\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(bloodRed).Render("[ SYSTEM DIAGNOSTICS ]") + "\n\n")
		
		fmtStr := lipgloss.NewStyle().Foreground(dimText).Render("Path:   ") + "%s\n" +
				  lipgloss.NewStyle().Foreground(dimText).Render("Commit: ") + "%s\n"
		
		sb.WriteString(fmt.Sprintf(fmtStr, m.RepoData.Path, m.RepoData.CommitHash))
		
		status := lipgloss.NewStyle().Foreground(bloodRed).Render("DIRTY [!] ")
		if m.RepoData.Clean { status = lipgloss.NewStyle().Foreground(dimText).Render("CLEAN ") }
		sb.WriteString(lipgloss.NewStyle().Foreground(dimText).Render("Status: ") + status + "\n")
		
		sb.WriteString(fmt.Sprintf("\n[ METRICS ]\n%s %d Local Artifacts\n%s %d Remote Issues\n", bracketStyle.Render("[+]"), len(m.RepoData.Todos), bracketStyle.Render("[+]"), len(m.RepoData.Issues)))

	case StateIssues:
		header := fmt.Sprintf("[ REMOTE TARGETS : %d ]", len(m.FilteredIssues))
		if m.Syncing { header += " [ SYNCING... ]" }
		sb.WriteString(titleRendered(header) + "  ")
		
		if m.Searching || m.SearchInput.Value() != "" {
			sb.WriteString(m.SearchInput.View())
		} else {
			sb.WriteString(lipgloss.NewStyle().Foreground(dimText).Render("'s' sync • '/' filter"))
		}
		sb.WriteString("\n\n")

		if len(m.FilteredIssues) == 0 {
			if m.RepoData.Owner == "" {
				sb.WriteString(lipgloss.NewStyle().Foreground(darkRed).Render("[-] Could not detect remote target.\n    Only local file analysis available."))
			} else {
				sb.WriteString(lipgloss.NewStyle().Foreground(dimText).Render("[-] No issues found in cache.\n    Press 's' to pull from remote (requires GITHUB_TOKEN)."))
			}
		} else {
			renderList(&sb, m.ListScroll, m.WindowH-6, len(m.FilteredIssues), func(i int) string {
				issue := m.FilteredIssues[i]
				return fmt.Sprintf("#%-4d %s %s", issue.Number, issue.Title, lipgloss.NewStyle().Foreground(darkRed).Render("["+issue.User.Login+"]"))
			})
		}

	case StateTODOs:
		header := fmt.Sprintf("[ LOCAL ARTIFACTS : %d ]", len(m.FilteredTodos))
		sb.WriteString(titleRendered(header) + "  ")
		if m.Searching || m.SearchInput.Value() != "" { sb.WriteString(m.SearchInput.View()) } else { sb.WriteString(lipgloss.NewStyle().Foreground(dimText).Render("'/' filter")) }
		sb.WriteString("\n\n")

		if len(m.FilteredTodos) == 0 {
			sb.WriteString(lipgloss.NewStyle().Foreground(dimText).Render("[-] No artifacts found."))
		} else {
			renderList(&sb, m.ListScroll, m.WindowH-6, len(m.FilteredTodos), func(i int) string {
				t := m.FilteredTodos[i]
				location := lipgloss.NewStyle().Foreground(darkRed).Render(fmt.Sprintf("%s:%d", t.File, t.Line))
				return fmt.Sprintf("%s %s", location, t.Text)
			})
		}
	}

	mainStyle = mainStyle.Width(m.WindowW - 35)
	right := mainStyle.Render(sb.String())
	help := lipgloss.NewStyle().Foreground(darkRed).Render("\n  [ ↑/↓: nav ]  [ tab: mode ]  [ /: filter ]  [ s: sync ]  [ q: exit ]")
	return docStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, left, right) + help)
}

func renderList(sb *strings.Builder, start, height, total int, renderer func(int) string) {
	if height < 1 { height = 1 }
	end := start + height
	if end > total { end = total }
	
	for i := start; i < end; i++ {
		sb.WriteString(fmt.Sprintf("%s %s\n", bracketStyle.Render("[+]"), renderer(i)))
	}
	if total > height {
		pct := int((float64(start) / float64(total)) * 100)
		sb.WriteString(lipgloss.NewStyle().Foreground(darkRed).Render(fmt.Sprintf("\n[ %d%% ]", pct)))
	}
}