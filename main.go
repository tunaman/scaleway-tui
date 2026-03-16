package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/minio/minio-go/v7"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// ─────────────────────────────────────────────
// Constants
// ─────────────────────────────────────────────

const (
	stateProfilePicker = iota
	stateDashboard
	stateObjectBrowser
	stateBilling
	stateRegistryBrowser
)

// pickerAction is what Enter triggers on the profile picker's action buttons.
const (
	pickerActionConnect = iota
	pickerActionQuit
	pickerActionCount
)

const (
	focusNav = iota
	focusContent
)

const (
	serviceObjectStorage = iota
	serviceK8s
	serviceBilling
	serviceRegistry
	serviceCount
)

const (
	navWidth        = 22
	detailPaneWidth = 36
	topBarHeight    = 3
	statusBarHeight = 1
	listRowOverhead = 4 // top-border(1) + header(1) + divider(1) + bottom-border(1)
)

// ─────────────────────────────────────────────
// Dracula palette
// ─────────────────────────────────────────────

var (
	colBg      = lipgloss.Color("#282a36")
	colBg2     = lipgloss.Color("#1e1f29")
	colBg3     = lipgloss.Color("#383a4a")
	colFg      = lipgloss.Color("#f8f8f2")
	colComment = lipgloss.Color("#6272a4")
	colRed     = lipgloss.Color("#ff5555")
	colGreen   = lipgloss.Color("#50fa7b")
	colYellow  = lipgloss.Color("#f1fa8c")
	colBlue    = lipgloss.Color("#8be9fd")
	colPurple  = lipgloss.Color("#bd93f9")
	colBorder  = lipgloss.Color("#44475a")

	// teaProgram is set once before p.Run() and used by upload goroutines to
	// call Send() from outside the Bubbletea event loop.
	teaProgram *tea.Program
)

// ─────────────────────────────────────────────
// Logo
// ─────────────────────────────────────────────

const logo = `
 ██████╗   ██████╗  ██╗    ██╗    ████████╗██╗   ██╗██╗
██╔════╝  ██╔════╝  ██║    ██║       ██╔══╝██║   ██║██║
╚█████╗   ██║       ██║ █╗ ██║       ██║   ██║   ██║██║
 ╚════██╗ ██║       ██║███╗██║       ██║   ██║   ██║██║
 ██████╔╝ ╚██████╗  ╚███╔███╔╝       ██║   ╚██████╔╝██║
 ╚═════╝   ╚═════╝   ╚══╝╚══╝        ╚═╝    ╚═════╝ ╚═╝`

// ─────────────────────────────────────────────
// Model
// ─────────────────────────────────────────────

type rootModel struct {
	// UI state
	state         int
	focus         int
	showDropdown  bool
	dropdownIndex int
	pickerAction  int // which action button is highlighted on the picker
	loading       bool
	err           error

	// Profile picker state
	profileNames  []string
	profileCursor int
	scwCfg        *scw.Config

	// Active profile info (displayed in top bar / status bar)
	activeProfileName string
	activeRegion      string

	// Dashboard selection state
	activeService   int
	bucketCursor    int
	bucketScrollY   int    // top row index of the visible viewport
	bucketScrollX   int    // horizontal character offset for name column
	bucketFilter    string // live filter string (empty = show all)
	bucketFiltering bool   // true while the user is typing a filter
	clusterCursor   int
	prevBucketSel   int
	project         string
	projectID       string

	// Data
	buckets            []bucket
	clusters           []cluster
	projects           []projectItem
	registryNamespaces []registryNamespace

	// Registry state
	registryCursor    int
	registryScrollY   int
	registryFilter    string
	registryFiltering bool

	// Registry browser state (stateRegistryBrowser)
	regBrowserNamespace     registryNamespace
	regBrowserImages        []registryImage
	regBrowserCursor        int
	regBrowserScrollY       int
	regBrowserFilter        string
	regBrowserFiltering     bool
	regBrowserFocus         int // 0 = images pane, 1 = versions pane
	regBrowserTagCursor     int
	regBrowserTagScrollY    int
	regTagActionOverlay     bool
	regTagsLoading          bool
	regTagFilter            string
	regTagFiltering         bool
	regTagSelected          map[string]bool // selected tag names in current image
	regConfirmDeleteTags    bool
	regConfirmTagsToDelete  []registryTag
	regConfirmDeleteImgID   string
	regConfirmDeleteImgName string

	// Object browser state (stateObjectBrowser)
	browserBucket   string
	browserPrefix   string // current path prefix, e.g. "folder/subfolder/"
	browserEntries  []bucketEntry
	browserCursor   int
	browserScrollY  int
	browserScrollX  int
	browserSelected map[string]bool // selected entry fullKeys

	// Delete confirmation overlay (shown on top of stateObjectBrowser)
	showConfirm  bool
	confirmItems []bucketEntry // items queued for deletion

	// Input overlay — used for bucket/folder/upload creation
	input struct {
		active bool
		mode   inputMode
		value  string // current text being typed
		cursor int    // rune index of the insert cursor
		errStr string // inline error (e.g. "name already exists")
	}

	// Upload progress overlay
	upload struct {
		active    bool
		filename  string
		bytesRead int64
		total     int64
	}

	// Clients (nil until a profile is activated)
	minioClient *minio.Client
	scwClient   *scw.Client

	// Billing state (stateBilling)
	billingMonths     []billingMonth
	billingDetail     []billingConsumptionRow
	billingPeriod     string // "YYYY-MM" currently shown
	billingCursor     int    // row cursor in detail table
	billingScrollY    int
	billingExportMsg string // confirmation message to show

	// Widgets
	spin spinner.Model

	// Terminal size
	width, height int
}

// ─────────────────────────────────────────────
// Init
// ─────────────────────────────────────────────

func (m rootModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, tea.EnableBracketedPaste, tea.SetWindowTitle("Scaleway TUI"))
}

// ─────────────────────────────────────────────
// Update
// ─────────────────────────────────────────────

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case clientsReadyMsg:
		m.loading = false
		m.scwClient = msg.scwClient
		m.minioClient = msg.minioClient
		m.activeProfileName = msg.profileName
		m.activeRegion = msg.region
		// Seed projectID from the profile so we never need ListProjects.
		m.projectID = msg.defaultProjectID
		if msg.defaultProjectID != "" {
			m.project = msg.defaultProjectID // replaced with a real name once we have it
		}
		m.state = stateDashboard
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.fetchData())

	case uploadProgressMsg:
		if msg.done {
			m.upload.active = false
			m.loading = true
			return m, tea.Batch(m.spin.Tick, m.fetchBucketContents(m.browserBucket, m.browserPrefix))
		}
		m.upload.active = true
		m.upload.filename = msg.filename
		m.upload.bytesRead = msg.bytesRead
		m.upload.total = msg.total
		return m, nil

	case errMsg:
		m.loading = false
		m.upload.active = false // clear progress overlay if upload failed
		m.err = msg.err
		return m, nil

	case createDoneMsg:
		m.loading = true
		if msg.isBucket {
			return m, tea.Batch(m.spin.Tick, m.fetchData())
		}
		return m, tea.Batch(m.spin.Tick, m.fetchBucketContents(msg.bucket, msg.prefix))

	case deleteMsg:
		m.loading = true
		m.browserSelected = make(map[string]bool)
		return m, tea.Batch(m.spin.Tick, m.fetchBucketContents(msg.bucket, msg.prefix))

	case registryImagesMsg:
		m.loading = false
		m.regBrowserNamespace = msg.namespace
		m.regBrowserImages = msg.images
		m.regBrowserCursor = 0
		m.regBrowserScrollY = 0
		m.regBrowserFocus = 0
		m.regBrowserTagCursor = 0
		m.regBrowserTagScrollY = 0
		m.regTagActionOverlay = false
		m.regConfirmDeleteTags = false
		m.regTagSelected = nil
		m.regTagFilter = ""
		m.regTagFiltering = false
		m.state = stateRegistryBrowser
		// Kick off tag fetch for the first image immediately.
		if len(msg.images) > 0 {
			m.regTagsLoading = true
			return m, m.fetchRegistryTags(msg.images[0])
		}
		return m, nil

	case registryTagsMsg:
		m.regTagsLoading = false
		for i := range m.regBrowserImages {
			if m.regBrowserImages[i].id == msg.imageID {
				m.regBrowserImages[i].tags = msg.tags
				break
			}
		}
		return m, nil

	case registryImageDeletedMsg:
		m.loading = false
		// Remove deleted image from the list by name (cursor points to it).
		visible := m.filteredRegistryImages()
		if len(visible) > 0 && m.regBrowserCursor < len(visible) {
			target := visible[m.regBrowserCursor].id
			for i, img := range m.regBrowserImages {
				if img.id == target {
					m.regBrowserImages = append(m.regBrowserImages[:i], m.regBrowserImages[i+1:]...)
					break
				}
			}
		}
		if m.regBrowserCursor >= len(m.regBrowserImages) {
			m.regBrowserCursor = max(0, len(m.regBrowserImages)-1)
		}
		m.regBrowserTagCursor = 0
		m.regBrowserTagScrollY = 0
		return m, nil

	case registryTagsDeletedMsg:
		m.loading = false
		for i := range m.regBrowserImages {
			if m.regBrowserImages[i].id != msg.imageID {
				continue
			}
			deleted := make(map[string]bool, len(msg.tagNames))
			for _, n := range msg.tagNames {
				deleted[n] = true
			}
			var remaining []registryTag
			for _, t := range m.regBrowserImages[i].tags {
				if !deleted[t.name] {
					remaining = append(remaining, t)
				}
			}
			m.regBrowserImages[i].tags = remaining
			if m.regBrowserTagCursor >= len(remaining) {
				m.regBrowserTagCursor = max(0, len(remaining)-1)
			}
			break
		}
		m.regTagSelected = nil
		return m, nil

	case bucketContentsMsg:
		m.loading = false
		m.browserBucket = msg.bucket
		m.browserPrefix = msg.prefix
		m.browserEntries = msg.entries
		m.browserCursor = 0
		m.browserScrollY = 0
		m.browserScrollX = 0
		m.browserSelected = make(map[string]bool)
		m.state = stateObjectBrowser
		return m, nil

	case dataMsg:
		m.loading = false
		m.buckets = msg.buckets
		m.clusters = msg.clusters
		m.projects = msg.projects
		m.registryNamespaces = msg.registryNamespaces
		m.bucketCursor = 0
		m.bucketScrollY = 0
		m.bucketScrollX = 0
		m.clusterCursor = 0
		m.registryCursor = 0
		m.registryScrollY = 0
		m.prevBucketSel = -1
		// Update the display name now that we've resolved it.
		if len(msg.projects) > 0 {
			m.project = msg.projects[0].name
		}
		return m, m.maybeCalculateSize()

	case sizeMsg:
		for i := range m.buckets {
			if m.buckets[i].name == msg.bucketName {
				m.buckets[i].sizeBytes = msg.sizeBytes
				m.buckets[i].objCount = msg.objCount
				m.buckets[i].sizeReady = true
				break
			}
		}
		return m, nil

	case billingOverviewMsg:
		m.loading = false
		m.billingMonths = msg.months
		m.billingDetail = msg.detail
		m.billingPeriod = msg.period
		m.billingCursor = 0
		m.billingScrollY = 0
		m.state = stateBilling
		return m, nil

	case billingExportDoneMsg:
		m.loading = false
		m.billingExportMsg = "Exported → " + msg.path
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// ─────────────────────────────────────────────
// View
// ─────────────────────────────────────────────

func (m rootModel) View() string {
	switch m.state {
	case stateProfilePicker:
		return m.drawProfilePicker()
	case stateObjectBrowser:
		return m.drawObjectBrowser()
	case stateRegistryBrowser:
		return m.drawRegistryBrowser()
	case stateBilling:
		return m.drawBilling()
	default:
		return m.drawDashboard()
	}
}

// ─────────────────────────────────────────────
// main
// ─────────────────────────────────────────────

func main() {
	// Use the SDK's own config loader — reads ~/.config/scw/config.yaml
	// (or $SCW_CONFIG_PATH) and handles all XDG path logic for us.
	scwCfg, err := scw.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading Scaleway config: %v\n", err)
		fmt.Fprintln(os.Stderr, "hint: run `scw init` to create a config file")
		os.Exit(1)
	}

	// Extract profile names from the SDK config.
	// scw.Config exposes Profiles as a map[string]*Profile.
	names := make([]string, 0, len(scwCfg.Profiles))
	for name := range scwCfg.Profiles {
		names = append(names, name)
	}
	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "error: no profiles found in Scaleway config — run `scw init` first")
		os.Exit(1)
	}

	tuiCfg := loadTUIConfig()

	// Pre-select the last-used profile if it still exists.
	cursor := 0
	for i, n := range names {
		if n == tuiCfg.ActiveProfile {
			cursor = i
			break
		}
	}

	spin := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(colRed)),
	)

	m := rootModel{
		spin:          spin,
		state:         stateProfilePicker,
		project:       "Select Project...",
		scwCfg:        scwCfg,
		profileNames:  names,
		profileCursor: cursor,
		pickerAction:  pickerActionConnect,
		prevBucketSel: -1,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	teaProgram = p
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
