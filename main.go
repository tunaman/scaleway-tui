package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/scaleway/scaleway-sdk-go/api/account/v3"
	billing "github.com/scaleway/scaleway-sdk-go/api/billing/v2beta1"
	"github.com/scaleway/scaleway-sdk-go/api/k8s/v1"
	"github.com/scaleway/scaleway-sdk-go/api/registry/v1"
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
// Config — SDK-native profile loading
//
// The Scaleway SDK reads ~/.config/scw/config.yaml natively via scw.LoadConfig().
// We use scw.Config.GetProfile(name) to load individual profiles by name, and
// scw.WithProfile(p) to build a fully-configured client from each one.
// This means we never need to parse YAML ourselves or depend on gopkg.in/yaml.v3
// for Scaleway credentials.
//
// Our own ~/.config/scw-tui/config.yaml only stores UI preferences (last profile).
// ─────────────────────────────────────────────

// tuiConfig is stored at ~/.config/scw-tui/config.yaml.
// It is intentionally minimal — credentials live only in the Scaleway config.
type tuiConfig struct {
	ActiveProfile string `json:"active_profile"`
}

// loadTUIConfig reads our TUI config. Returns empty struct on first run.
func loadTUIConfig() tuiConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return tuiConfig{}
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "scw-tui", "config.json"))
	if err != nil {
		return tuiConfig{}
	}
	var cfg tuiConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return tuiConfig{}
	}
	return cfg
}

// saveTUIConfig writes our TUI config (best-effort).
func saveTUIConfig(cfg tuiConfig) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".config", "scw-tui")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600)
}

// ─────────────────────────────────────────────
// Client factory — uses SDK Profile directly
// ─────────────────────────────────────────────

// buildClients constructs a Scaleway API client and a MinIO/S3 client from a
// named profile loaded via the SDK's own config parser.
func buildClients(cfg *scw.Config, profileName string) (*scw.Client, *minio.Client, string, error) {
	prof, err := cfg.GetProfile(profileName)
	if err != nil {
		return nil, nil, "", fmt.Errorf("profile %q: %w", profileName, err)
	}

	if prof.SecretKey == nil || *prof.SecretKey == "" {
		return nil, nil, "", fmt.Errorf("profile %q has no secret_key", profileName)
	}
	if prof.AccessKey == nil || *prof.AccessKey == "" {
		return nil, nil, "", fmt.Errorf("profile %q has no access_key", profileName)
	}

	scwClient, err := scw.NewClient(scw.WithProfile(prof))
	if err != nil {
		return nil, nil, "", fmt.Errorf("building Scaleway client: %w", err)
	}

	// Derive S3 endpoint from profile region, falling back to nl-ams.
	region := "nl-ams"
	if prof.DefaultRegion != nil && *prof.DefaultRegion != "" {
		region = string(*prof.DefaultRegion)
	}

	mc, err := minio.New(fmt.Sprintf("s3.%s.scw.cloud", region), &minio.Options{
		Creds:  credentials.NewStaticV4(*prof.AccessKey, *prof.SecretKey, ""),
		Secure: true,
	})
	if err != nil {
		return nil, nil, "", fmt.Errorf("building S3 client: %w", err)
	}

	return scwClient, mc, region, nil
}

// ─────────────────────────────────────────────
// Data types
// ─────────────────────────────────────────────

// bucketEntry represents one row in the object browser — either a virtual
// folder (isDir=true, Size=0) or a real object.
type bucketEntry struct {
	name         string // display name (basename only, no prefix)
	fullKey      string // full S3 key / prefix
	isDir        bool
	size         int64
	lastModified time.Time
	storageClass string
}

type bucket struct {
	name      string
	created   string
	sizeBytes int64
	objCount  int
	sizeReady bool
}

type cluster struct {
	name    string
	status  string
	version string
}

type registryNamespace struct {
	id         string
	name       string
	endpoint   string
	imageCount uint32
	sizeBytes  uint64
	status     string
	isPublic   bool
}

type registryTag struct {
	id   string
	name string
}

type registryImage struct {
	id         string
	name       string
	tags       []registryTag
	sizeBytes  uint64
	status     string
	visibility string
	updatedAt  time.Time
}

type projectItem struct {
	name string
	id   string
}

// ─────────────────────────────────────────────
// Tea messages
// ─────────────────────────────────────────────

type dataMsg struct {
	buckets            []bucket
	clusters           []cluster
	projects           []projectItem
	registryNamespaces []registryNamespace
}

type sizeMsg struct {
	bucketName string
	sizeBytes  int64
	objCount   int
}

type bucketContentsMsg struct {
	bucket  string
	prefix  string
	entries []bucketEntry
}

type deleteMsg struct {
	bucket string
	prefix string // prefix to re-fetch after deletion
}

// inputMode describes what the input overlay is creating.
type inputMode int

const (
	inputModeBucket inputMode = iota
	inputModeFolder
	inputModeUpload
)

// createDoneMsg is sent after a successful create/upload operation.
// uploadProgressMsg carries the latest byte count from the upload goroutine.
type uploadProgressMsg struct {
	filename  string
	bytesRead int64
	total     int64
	done      bool
}

type createDoneMsg struct {
	// For bucket creation we re-fetch the full bucket list.
	// For folder/upload we re-fetch the current browser prefix.
	isBucket bool
	bucket   string
	prefix   string
}

type registryImagesMsg struct {
	namespace registryNamespace
	images    []registryImage
}

type registryImageDeletedMsg struct{}
type registryTagsDeletedMsg struct {
	imageID  string
	tagNames []string
}
type registryTagsMsg struct {
	imageID string
	tags    []registryTag
}

type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

// ─────────────────────────────────────────────
// Billing data types
// ─────────────────────────────────────────────

// billingMonth holds aggregated consumption for a single billing period.
type billingMonth struct {
	period     string             // "YYYY-MM"
	totalExTax float64            // total excl. tax in EUR
	byCategory map[string]float64 // category → EUR
}

// billingConsumptionRow is one row in the detail table.
type billingConsumptionRow struct {
	category    string
	product     string
	projectName string
	valueEUR    float64
}

// billingOverviewMsg carries the data for the billing overview screen.
type billingOverviewMsg struct {
	months []billingMonth          // last N months, chronological
	detail []billingConsumptionRow // current/selected month detail
	period string                  // currently displayed period "YYYY-MM"
}

// billingExportDoneMsg is sent when CSV export completes.
type billingExportDoneMsg struct{ path string }

// clientsReadyMsg is sent after a profile is selected and clients are built.
type clientsReadyMsg struct {
	scwClient        *scw.Client
	minioClient      *minio.Client
	profileName      string
	region           string
	defaultProjectID string // read directly from the profile, no API call needed
}

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
	billingExportPath string // set after a successful export
	billingExportMsg  string // confirmation message to show

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
// Key handling
// ─────────────────────────────────────────────

func (m rootModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ── Input overlay: intercept all keys while active ──
	if m.input.active {
		runes := []rune(m.input.value)

		// Helper: clamp cursor into valid range
		clamp := func(c int) int {
			if c < 0 {
				return 0
			}
			if c > len(runes) {
				return len(runes)
			}
			return c
		}

		switch msg.String() {
		case "esc":
			m.input.active = false
			m.input.value = ""
			m.input.cursor = 0
			m.input.errStr = ""

		case "enter":
			val := strings.TrimSpace(m.input.value)
			if val == "" {
				m.input.errStr = "Name cannot be empty."
				return m, nil
			}
			switch m.input.mode {
			case inputModeBucket:
				m.input.active = false
				m.loading = true
				return m, tea.Batch(m.spin.Tick, m.createBucket(val))
			case inputModeFolder:
				m.input.active = false
				m.loading = true
				return m, tea.Batch(m.spin.Tick, m.createFolder(m.browserBucket, m.browserPrefix, val))
			case inputModeUpload:
				// Extract filename now so the overlay shows it immediately.
				base := val
				if idx := strings.LastIndexAny(val, "/\\"); idx >= 0 {
					base = val[idx+1:]
				}
				m.input.active = false
				m.upload.active = true
				m.upload.filename = base
				m.upload.bytesRead = 0
				m.upload.total = 0
				return m, m.uploadFile(m.browserBucket, m.browserPrefix, val)
			}

		case "backspace", "ctrl+h":
			if m.input.cursor > 0 {
				newRunes := append(runes[:m.input.cursor-1], runes[m.input.cursor:]...)
				m.input.value = string(newRunes)
				m.input.cursor = clamp(m.input.cursor - 1)
			}
			m.input.errStr = ""

		case "delete", "ctrl+d":
			if m.input.cursor < len(runes) {
				newRunes := append(runes[:m.input.cursor], runes[m.input.cursor+1:]...)
				m.input.value = string(newRunes)
			}
			m.input.errStr = ""

		case "left", "ctrl+b":
			m.input.cursor = clamp(m.input.cursor - 1)

		case "right", "ctrl+f":
			m.input.cursor = clamp(m.input.cursor + 1)

		case "home", "ctrl+a":
			m.input.cursor = 0

		case "end", "ctrl+e":
			m.input.cursor = len(runes)

		case "ctrl+u":
			// Kill to beginning of line (readline convention).
			m.input.value = string(runes[m.input.cursor:])
			m.input.cursor = 0
			m.input.errStr = ""

		case "ctrl+k":
			// Kill to end of line.
			m.input.value = string(runes[:m.input.cursor])
			m.input.errStr = ""

		case "ctrl+w":
			// Kill word backwards.
			if m.input.cursor > 0 {
				i := m.input.cursor - 1
				for i > 0 && runes[i-1] != '/' && runes[i-1] != ' ' {
					i--
				}
				newRunes := append(runes[:i], runes[m.input.cursor:]...)
				m.input.value = string(newRunes)
				m.input.cursor = i
			}
			m.input.errStr = ""

		default:
			// msg.Runes contains the typed/pasted characters.
			if len(msg.Runes) > 0 {
				insert := msg.Runes
				newRunes := make([]rune, 0, len(runes)+len(insert))
				newRunes = append(newRunes, runes[:m.input.cursor]...)
				newRunes = append(newRunes, insert...)
				newRunes = append(newRunes, runes[m.input.cursor:]...)
				m.input.value = string(newRunes)
				// Cursor advances by the number of inserted chars.
				// Don't use clamp() here — it closes over the pre-insert runes slice.
				m.input.cursor += len(insert)
				m.input.errStr = ""
			}
		}
		return m, nil
	}
	// ── Filter mode: intercept all printable keys ──
	if m.bucketFiltering {
		switch msg.String() {
		case "esc":
			m.bucketFiltering = false
			m.bucketFilter = ""
			m.bucketCursor = 0
			m.bucketScrollY = 0
		case "enter":
			// Lock in the first match and exit filter mode.
			m.bucketFiltering = false
			m.bucketCursor = 0
			m.bucketScrollY = 0
		case "backspace", "ctrl+h":
			if len([]rune(m.bucketFilter)) > 0 {
				runes := []rune(m.bucketFilter)
				m.bucketFilter = string(runes[:len(runes)-1])
			} else {
				// Empty filter + backspace → exit filter mode.
				m.bucketFiltering = false
			}
			m.bucketCursor = 0
			m.bucketScrollY = 0
		case "up", "k":
			return m.handleUp()
		case "down", "j":
			return m.handleDown()
		default:
			// Only accept printable single chars.
			if len(msg.Runes) == 1 {
				m.bucketFilter += string(msg.Runes)
				m.bucketCursor = 0
				m.bucketScrollY = 0
			}
		}
		return m, nil
	}
	// ── Tag action overlay: pull instructions ──
	if m.regTagActionOverlay {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		default:
			m.regTagActionOverlay = false
		}
		return m, nil
	}

	// ── Tag delete confirm ──
	if m.regConfirmDeleteTags {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "y", "Y":
			imageID := m.regConfirmDeleteImgID
			tags := m.regConfirmTagsToDelete
			m.regConfirmDeleteTags = false
			m.regConfirmTagsToDelete = nil
			m.loading = true
			return m, tea.Batch(m.spin.Tick, m.deleteRegistryTags(imageID, tags))
		default:
			m.regConfirmDeleteTags = false
			m.regConfirmTagsToDelete = nil
		}
		return m, nil
	}

	// ── Registry browser filter mode ──
	if m.regBrowserFiltering {
		switch msg.String() {
		case "esc":
			m.regBrowserFiltering = false
			m.regBrowserFilter = ""
			m.regBrowserCursor = 0
			m.regBrowserScrollY = 0
		case "enter":
			m.regBrowserFiltering = false
			m.regBrowserCursor = 0
			m.regBrowserScrollY = 0
		case "backspace", "ctrl+h":
			if len([]rune(m.regBrowserFilter)) > 0 {
				runes := []rune(m.regBrowserFilter)
				m.regBrowserFilter = string(runes[:len(runes)-1])
			} else {
				m.regBrowserFiltering = false
			}
			m.regBrowserCursor = 0
			m.regBrowserScrollY = 0
		case "up", "k":
			return m.handleUp()
		case "down", "j":
			return m.handleDown()
		default:
			if len(msg.Runes) == 1 {
				m.regBrowserFilter += string(msg.Runes)
				m.regBrowserCursor = 0
				m.regBrowserScrollY = 0
			}
		}
		return m, nil
	}
	// ── Registry tag filter mode ──
	if m.regTagFiltering {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.regTagFiltering = false
			m.regTagFilter = ""
			m.regBrowserTagCursor = 0
			m.regBrowserTagScrollY = 0
		case "enter":
			m.regTagFiltering = false
			m.regBrowserTagCursor = 0
			m.regBrowserTagScrollY = 0
		case "backspace", "ctrl+h":
			if len([]rune(m.regTagFilter)) > 0 {
				runes := []rune(m.regTagFilter)
				m.regTagFilter = string(runes[:len(runes)-1])
			} else {
				m.regTagFiltering = false
			}
			m.regBrowserTagCursor = 0
			m.regBrowserTagScrollY = 0
		case "up", "k":
			return m.handleUp()
		case "down", "j":
			return m.handleDown()
		default:
			if len(msg.Runes) == 1 {
				m.regTagFilter += string(msg.Runes)
				m.regBrowserTagCursor = 0
				m.regBrowserTagScrollY = 0
			}
		}
		return m, nil
	}
	// ── Registry filter mode ──
	if m.registryFiltering {
		switch msg.String() {
		case "esc":
			m.registryFiltering = false
			m.registryFilter = ""
			m.registryCursor = 0
			m.registryScrollY = 0
		case "enter":
			m.registryFiltering = false
			m.registryCursor = 0
			m.registryScrollY = 0
		case "backspace", "ctrl+h":
			if len([]rune(m.registryFilter)) > 0 {
				runes := []rune(m.registryFilter)
				m.registryFilter = string(runes[:len(runes)-1])
			} else {
				m.registryFiltering = false
			}
			m.registryCursor = 0
			m.registryScrollY = 0
		case "up", "k":
			return m.handleUp()
		case "down", "j":
			return m.handleDown()
		default:
			if len(msg.Runes) == 1 {
				m.registryFilter += string(msg.Runes)
				m.registryCursor = 0
				m.registryScrollY = 0
			}
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "q":
		if m.showConfirm || m.input.active {
			// Dismiss overlays rather than quitting.
			m.showConfirm = false
			m.input.active = false
			return m, nil
		}
		return m, tea.Quit
	case "esc":
		if m.showConfirm {
			m.showConfirm = false
			m.confirmItems = nil
			return m, nil
		}
		// Dismiss tag action overlay.
		if m.regTagActionOverlay {
			m.regTagActionOverlay = false
			return m, nil
		}
		// Dismiss registry tag delete confirm.
		if m.regConfirmDeleteTags {
			m.regConfirmDeleteTags = false
			m.regConfirmTagsToDelete = nil
			return m, nil
		}
		// Clear tag filter if active.
		if m.regTagFiltering {
			m.regTagFiltering = false
			m.regTagFilter = ""
			m.regBrowserTagCursor = 0
			m.regBrowserTagScrollY = 0
			return m, nil
		}
		if m.regTagFilter != "" {
			m.regTagFilter = ""
			m.regBrowserTagCursor = 0
			m.regBrowserTagScrollY = 0
			return m, nil
		}
		// Return to images pane from versions pane.
		if m.state == stateRegistryBrowser && m.regBrowserFocus == 1 {
			m.regBrowserFocus = 0
			m.regTagSelected = nil
			return m, nil
		}
		// Clear registry browser filter if active.
		if m.regBrowserFilter != "" {
			m.regBrowserFilter = ""
			m.regBrowserCursor = 0
			m.regBrowserScrollY = 0
			return m, nil
		}
		// Clear registry filter if active.
		if m.registryFilter != "" {
			m.registryFilter = ""
			m.registryCursor = 0
			m.registryScrollY = 0
			return m, nil
		}
		// Clear bucket filter if active without entering filter mode.
		if m.bucketFilter != "" {
			m.bucketFilter = ""
			m.bucketCursor = 0
			m.bucketScrollY = 0
			return m, nil
		}
		return m.handleEsc()
	case "f5":
		if !m.loading {
			m.loading = true
			if m.state == stateObjectBrowser {
				return m, tea.Batch(m.spin.Tick, m.fetchBucketContents(m.browserBucket, m.browserPrefix))
			}
			if m.state == stateBilling {
				return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))
			}
			if m.state == stateRegistryBrowser {
				return m, tea.Batch(m.spin.Tick, m.fetchRegistryImages(m.regBrowserNamespace))
			}
			return m, tea.Batch(m.spin.Tick, m.fetchData())
		}
	case "e", "E":
		if m.state == stateBilling && !m.loading {
			m.loading = true
			m.billingExportMsg = ""
			return m, tea.Batch(m.spin.Tick, m.exportBillingCSV(12))
		}
	case "/":
		if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceObjectStorage {
			m.bucketFiltering = true
			m.bucketFilter = ""
			m.bucketCursor = 0
			m.bucketScrollY = 0
		}
		if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceRegistry {
			m.registryFiltering = true
			m.registryFilter = ""
			m.registryCursor = 0
			m.registryScrollY = 0
		}
		if m.state == stateRegistryBrowser && m.regBrowserFocus == 1 && !m.loading {
			m.regTagFiltering = true
			m.regTagFilter = ""
			m.regBrowserTagCursor = 0
			m.regBrowserTagScrollY = 0
			return m, nil
		}
		if m.state == stateRegistryBrowser && m.regBrowserFocus == 0 {
			m.regBrowserFiltering = true
			m.regBrowserFilter = ""
			m.regBrowserCursor = 0
			m.regBrowserScrollY = 0
		}
	case "y", "Y":
		if m.showConfirm {
			items := m.confirmItems
			bucket := m.browserBucket
			prefix := m.browserPrefix
			m.showConfirm = false
			m.confirmItems = nil
			m.loading = true
			return m, tea.Batch(m.spin.Tick, m.deleteEntries(bucket, prefix, items))
		}
		if m.regConfirmDeleteTags {
			imageID := m.regConfirmDeleteImgID
			tags := m.regConfirmTagsToDelete
			m.regConfirmDeleteTags = false
			m.regConfirmTagsToDelete = nil
			m.loading = true
			return m, tea.Batch(m.spin.Tick, m.deleteRegistryTags(imageID, tags))
		}
	case "n", "N":
		if m.showConfirm {
			m.showConfirm = false
			m.confirmItems = nil
			return m, nil
		}
		if m.regConfirmDeleteTags {
			m.regConfirmDeleteTags = false
			m.regConfirmTagsToDelete = nil
		}
	case "c", "C":
		if m.loading || m.showConfirm {
			return m, nil
		}
		if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceObjectStorage {
			m.input.active = true
			m.input.mode = inputModeBucket
			m.input.value = ""
			m.input.cursor = 0
			m.input.errStr = ""
		} else if m.state == stateObjectBrowser {
			m.input.active = true
			m.input.mode = inputModeFolder
			m.input.value = ""
			m.input.cursor = 0
			m.input.errStr = ""
		}
	case "u", "U":
		if m.state == stateObjectBrowser && !m.loading && !m.showConfirm {
			m.input.active = true
			m.input.mode = inputModeUpload
			m.input.value = ""
			m.input.cursor = 0
			m.input.errStr = ""
		}
	case "d", "D":
		if m.state == stateObjectBrowser && !m.loading && !m.showConfirm {
			var targets []bucketEntry
			if len(m.browserSelected) > 0 {
				for _, e := range m.browserEntries {
					if m.browserSelected[e.fullKey] {
						targets = append(targets, e)
					}
				}
			} else if len(m.browserEntries) > 0 {
				targets = []bucketEntry{m.browserEntries[m.browserCursor]}
			}
			if len(targets) > 0 {
				m.confirmItems = targets
				m.showConfirm = true
			}
		}
		if m.state == stateRegistryBrowser && !m.loading && !m.regTagActionOverlay && !m.regConfirmDeleteTags {
			imgs := m.filteredRegistryImages()
			if m.regBrowserFocus == 1 && len(imgs) > 0 && m.regBrowserCursor < len(imgs) {
				img := imgs[m.regBrowserCursor]
				filteredTags := m.filteredRegistryTags(img)
				var toDelete []registryTag
				if len(m.regTagSelected) > 0 {
					for _, t := range filteredTags {
						if m.regTagSelected[t.name] {
							toDelete = append(toDelete, t)
						}
					}
				} else if len(filteredTags) > 0 && m.regBrowserTagCursor < len(filteredTags) {
					toDelete = []registryTag{filteredTags[m.regBrowserTagCursor]}
				}
				if len(toDelete) > 0 {
					m.regConfirmTagsToDelete = toDelete
					m.regConfirmDeleteImgID = img.id
					m.regConfirmDeleteImgName = img.name
					m.regConfirmDeleteTags = true
				}
			}
			// Image-level deletion removed — only tag deletion is supported.
		}
	case "tab":
		if m.state == stateDashboard {
			m.focus = focusNav + (m.focus-focusNav+1)%2
			m.showDropdown = false
		}
		if m.state == stateRegistryBrowser && !m.loading && !m.regTagActionOverlay && !m.regConfirmDeleteTags {
			imgs := m.filteredRegistryImages()
			if m.regBrowserFocus == 0 {
				if len(imgs) > 0 && m.regBrowserCursor < len(imgs) {
					if !m.regTagsLoading {
						m.regBrowserFocus = 1
						m.regBrowserTagCursor = 0
						m.regBrowserTagScrollY = 0
					}
				}
			} else {
				m.regBrowserFocus = 0
				m.regTagSelected = nil
			}
		}
	case "left", "h":
		if m.state == stateProfilePicker && !m.loading {
			m.pickerAction = (m.pickerAction - 1 + pickerActionCount) % pickerActionCount
		} else if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceObjectStorage {
			m.bucketScrollX = max(0, m.bucketScrollX-4)
		} else if m.state == stateObjectBrowser {
			m.browserScrollX = max(0, m.browserScrollX-4)
		} else if m.state == stateBilling && !m.loading {
			m.billingPeriod = prevMonth(m.billingPeriod)
			m.loading = true
			m.billingExportMsg = ""
			return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))
		}
	case "right", "l":
		if m.state == stateProfilePicker && !m.loading {
			m.pickerAction = (m.pickerAction + 1) % pickerActionCount
		} else if m.state == stateDashboard && m.focus == focusContent && m.activeService == serviceObjectStorage {
			m.bucketScrollX += 4
		} else if m.state == stateObjectBrowser {
			m.browserScrollX += 4
		} else if m.state == stateBilling && !m.loading {
			next := nextMonth(m.billingPeriod)
			// Don't navigate into the future beyond current month
			now := time.Now().Format("2006-01")
			if next <= now {
				m.billingPeriod = next
				m.loading = true
				m.billingExportMsg = ""
				return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))
			}
		}
	case "up", "k":
		return m.handleUp()
	case "down", "j":
		return m.handleDown()
	case "enter":
		return m.handleEnter()
	case " ":
		if m.state == stateObjectBrowser && len(m.browserEntries) > 0 {
			key := m.browserEntries[m.browserCursor].fullKey
			if m.browserSelected == nil {
				m.browserSelected = make(map[string]bool)
			}
			m.browserSelected[key] = !m.browserSelected[key]
			if m.browserCursor < len(m.browserEntries)-1 {
				m.browserCursor++
			}
		}
		if m.state == stateRegistryBrowser && m.regBrowserFocus == 1 && !m.regTagsLoading {
			imgs := m.filteredRegistryImages()
			if len(imgs) > 0 && m.regBrowserCursor < len(imgs) {
				filteredTags := m.filteredRegistryTags(imgs[m.regBrowserCursor])
				if len(filteredTags) > 0 && m.regBrowserTagCursor < len(filteredTags) {
					tag := filteredTags[m.regBrowserTagCursor]
					if m.regTagSelected == nil {
						m.regTagSelected = make(map[string]bool)
					}
					m.regTagSelected[tag.name] = !m.regTagSelected[tag.name]
					if !m.regTagSelected[tag.name] {
						delete(m.regTagSelected, tag.name)
					}
					if m.regBrowserTagCursor < len(filteredTags)-1 {
						m.regBrowserTagCursor++
					}
				}
			}
		}
	case "a":
		if m.state == stateObjectBrowser && len(m.browserEntries) > 0 {
			if m.browserSelected == nil {
				m.browserSelected = make(map[string]bool)
			}
			allSelected := len(m.browserSelected) == len(m.browserEntries)
			for _, e := range m.browserEntries {
				if allSelected {
					delete(m.browserSelected, e.fullKey)
				} else {
					m.browserSelected[e.fullKey] = true
				}
			}
		}
		if m.state == stateRegistryBrowser && m.regBrowserFocus == 1 && !m.regTagsLoading {
			imgs := m.filteredRegistryImages()
			if len(imgs) > 0 && m.regBrowserCursor < len(imgs) {
				filteredTags := m.filteredRegistryTags(imgs[m.regBrowserCursor])
				if len(filteredTags) > 0 {
					if m.regTagSelected == nil {
						m.regTagSelected = make(map[string]bool)
					}
					allSelected := len(m.regTagSelected) == len(filteredTags)
					for _, t := range filteredTags {
						if allSelected {
							delete(m.regTagSelected, t.name)
						} else {
							m.regTagSelected[t.name] = true
						}
					}
				}
			}
		}
	}
	return m, nil
}

func (m rootModel) handleEsc() (rootModel, tea.Cmd) {
	switch m.state {
	case stateRegistryBrowser:
		m.state = stateDashboard
		m.activeService = serviceRegistry
		m.regBrowserFilter = ""
		m.regBrowserFiltering = false
		m.regBrowserFocus = 0
		m.regBrowserTagCursor = 0
		m.regBrowserTagScrollY = 0
		m.regTagActionOverlay = false
		m.regConfirmDeleteTags = false
		m.regTagSelected = nil
		m.regTagFilter = ""
		m.regTagFiltering = false
	case stateObjectBrowser:
		if m.browserPrefix == "" {
			// At root of bucket — go back to the bucket list.
			m.state = stateDashboard
		} else {
			// Go up one level.
			parent := parentPrefix(m.browserPrefix)
			m.loading = true
			return m, tea.Batch(m.spin.Tick, m.fetchBucketContents(m.browserBucket, parent))
		}
	case stateBilling:
		m.state = stateDashboard
		m.billingExportMsg = ""
	case stateDashboard:
		m.showDropdown = false
		m.state = stateProfilePicker
		m.err = nil
	}
	return m, nil
}

func (m rootModel) handleUp() (rootModel, tea.Cmd) {
	switch {
	case m.state == stateProfilePicker && !m.loading:
		m.profileCursor = (m.profileCursor - 1 + len(m.profileNames)) % len(m.profileNames)

	case m.showDropdown && len(m.projects) > 0:
		m.dropdownIndex = (m.dropdownIndex - 1 + len(m.projects)) % len(m.projects)

	case m.state == stateObjectBrowser && len(m.browserEntries) > 0:
		if m.browserCursor > 0 {
			m.browserCursor--
			m.browserScrollX = 0
			if m.browserCursor < m.browserScrollY {
				m.browserScrollY = m.browserCursor
			}
		}

	case m.state == stateBilling && len(m.billingDetail) > 0:
		if m.billingCursor > 0 {
			m.billingCursor--
			if m.billingCursor < m.billingScrollY {
				m.billingScrollY = m.billingCursor
			}
		}

	case m.state == stateRegistryBrowser:
		imgs := m.filteredRegistryImages()
		if m.regBrowserFocus == 0 && len(imgs) > 0 {
			if m.regBrowserCursor > 0 {
				m.regBrowserCursor--
				if m.regBrowserCursor < m.regBrowserScrollY {
					m.regBrowserScrollY = m.regBrowserCursor
				}
				m.regBrowserTagCursor = 0
				m.regBrowserTagScrollY = 0
				m.regTagsLoading = true
				return m, m.fetchRegistryTags(imgs[m.regBrowserCursor])
			}
		} else if m.regBrowserFocus == 1 && len(imgs) > 0 && m.regBrowserCursor < len(imgs) {
			if m.regBrowserTagCursor > 0 {
				m.regBrowserTagCursor--
				if m.regBrowserTagCursor < m.regBrowserTagScrollY {
					m.regBrowserTagScrollY = m.regBrowserTagCursor
				}
			}
		}

	case m.state == stateDashboard && m.focus == focusNav:
		m.activeService = (m.activeService - 1 + serviceCount) % serviceCount

	case m.state == stateDashboard && m.focus == focusContent:
		fb := m.filteredBuckets()
		if m.activeService == serviceObjectStorage && len(fb) > 0 {
			if m.bucketCursor > 0 {
				m.bucketCursor--
				m.bucketScrollX = 0
				if m.bucketCursor < m.bucketScrollY {
					m.bucketScrollY = m.bucketCursor
				}
				return m, m.maybeCalculateSize()
			}
			return m, nil
		}
		if m.activeService == serviceK8s && len(m.clusters) > 0 {
			m.clusterCursor = (m.clusterCursor - 1 + len(m.clusters)) % len(m.clusters)
		}
		if m.activeService == serviceRegistry && len(m.registryNamespaces) > 0 {
			if m.registryCursor > 0 {
				m.registryCursor--
				if m.registryCursor < m.registryScrollY {
					m.registryScrollY = m.registryCursor
				}
			}
		}
	}
	return m, nil
}

func (m rootModel) handleDown() (rootModel, tea.Cmd) {
	switch {
	case m.state == stateProfilePicker && !m.loading:
		m.profileCursor = (m.profileCursor + 1) % len(m.profileNames)

	case m.showDropdown && len(m.projects) > 0:
		m.dropdownIndex = (m.dropdownIndex + 1) % len(m.projects)

	case m.state == stateObjectBrowser && len(m.browserEntries) > 0:
		if m.browserCursor < len(m.browserEntries)-1 {
			m.browserCursor++
			m.browserScrollX = 0
		}

	case m.state == stateBilling && len(m.billingDetail) > 0:
		if m.billingCursor < len(m.billingDetail)-1 {
			m.billingCursor++
		}

	case m.state == stateRegistryBrowser:
		imgs := m.filteredRegistryImages()
		if m.regBrowserFocus == 0 && len(imgs) > 0 {
			if m.regBrowserCursor < len(imgs)-1 {
				m.regBrowserCursor++
				m.regBrowserTagCursor = 0
				m.regBrowserTagScrollY = 0
				m.regTagsLoading = true
				return m, m.fetchRegistryTags(imgs[m.regBrowserCursor])
			}
		} else if m.regBrowserFocus == 1 && len(imgs) > 0 && m.regBrowserCursor < len(imgs) {
			tags := imgs[m.regBrowserCursor].tags
			if m.regBrowserTagCursor < len(tags)-1 {
				m.regBrowserTagCursor++
			}
		}

	case m.state == stateDashboard && m.focus == focusNav:
		m.activeService = (m.activeService + 1) % serviceCount

	case m.state == stateDashboard && m.focus == focusContent:
		fb := m.filteredBuckets()
		if m.activeService == serviceObjectStorage && len(fb) > 0 {
			if m.bucketCursor < len(fb)-1 {
				m.bucketCursor++
				m.bucketScrollX = 0
				return m, m.maybeCalculateSize()
			}
			return m, nil
		}
		if m.activeService == serviceK8s && len(m.clusters) > 0 {
			m.clusterCursor = (m.clusterCursor + 1) % len(m.clusters)
		}
		if m.activeService == serviceRegistry && len(m.registryNamespaces) > 0 {
			if m.registryCursor < len(m.registryNamespaces)-1 {
				m.registryCursor++
			}
		}
	}
	return m, nil
}

func (m rootModel) handleEnter() (rootModel, tea.Cmd) {
	switch {
	case m.state == stateProfilePicker && !m.loading:
		if len(m.profileNames) == 0 {
			return m, nil
		}
		switch m.pickerAction {
		case pickerActionConnect:
			chosen := m.profileNames[m.profileCursor]
			m.loading = true
			m.err = nil
			return m, tea.Batch(m.spin.Tick, m.activateProfile(chosen))
		case pickerActionQuit:
			return m, tea.Quit
		}

	case m.state == stateDashboard && m.focus == focusContent &&
		m.activeService == serviceObjectStorage && len(m.filteredBuckets()) > 0:
		fb := m.filteredBuckets()
		b := fb[m.bucketCursor]
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.fetchBucketContents(b.name, ""))

	case m.state == stateDashboard && m.focus == focusContent &&
		m.activeService == serviceRegistry && len(m.filteredRegistryNamespaces()) > 0:
		fn := m.filteredRegistryNamespaces()
		ns := fn[m.registryCursor]
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.fetchRegistryImages(ns))

	case m.state == stateDashboard && m.focus == focusContent &&
		m.activeService == serviceBilling:
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.fetchBillingOverview(m.billingPeriod))

	case m.state == stateRegistryBrowser && m.regBrowserFocus == 1:
		visible := m.filteredRegistryImages()
		if len(visible) > 0 && m.regBrowserCursor < len(visible) {
			img := visible[m.regBrowserCursor]
			if len(img.tags) > 0 && m.regBrowserTagCursor < len(img.tags) {
				m.regTagActionOverlay = true
			}
		}
		return m, nil

	case m.state == stateObjectBrowser && len(m.browserEntries) > 0:
		entry := m.browserEntries[m.browserCursor]
		if entry.isDir {
			// Descend into the folder.
			m.loading = true
			return m, tea.Batch(m.spin.Tick, m.fetchBucketContents(m.browserBucket, entry.fullKey))
		}
		// File selected — no action for now (could open detail overlay later).
	}
	return m, nil
}

// ─────────────────────────────────────────────
// Profile activation (Cmd)
// ─────────────────────────────────────────────

func (m rootModel) activateProfile(name string) tea.Cmd {
	return func() tea.Msg {
		scwClient, mc, region, err := buildClients(m.scwCfg, name)
		if err != nil {
			return errMsg{err}
		}
		saveTUIConfig(tuiConfig{ActiveProfile: name})

		// Read default_project_id directly from the profile — no API call needed.
		projectID := ""
		if prof, err := m.scwCfg.GetProfile(name); err == nil && prof.DefaultProjectID != nil {
			projectID = *prof.DefaultProjectID
		}

		return clientsReadyMsg{
			scwClient:        scwClient,
			minioClient:      mc,
			profileName:      name,
			region:           region,
			defaultProjectID: projectID,
		}
	}
}

// ─────────────────────────────────────────────
// filteredRegistryNamespaces
// ─────────────────────────────────────────────

func (m rootModel) filteredRegistryNamespaces() []registryNamespace {
	if m.registryFilter == "" {
		return m.registryNamespaces
	}
	needle := strings.ToLower(m.registryFilter)
	var out []registryNamespace
	for _, ns := range m.registryNamespaces {
		if strings.Contains(strings.ToLower(ns.name), needle) {
			out = append(out, ns)
		}
	}
	return out
}

// ─────────────────────────────────────────────
// filteredRegistryImages
// ─────────────────────────────────────────────

func (m rootModel) filteredRegistryImages() []registryImage {
	if m.regBrowserFilter == "" {
		return m.regBrowserImages
	}
	needle := strings.ToLower(m.regBrowserFilter)
	var out []registryImage
	for _, img := range m.regBrowserImages {
		if strings.Contains(strings.ToLower(img.name), needle) {
			out = append(out, img)
		}
	}
	return out
}

// filteredRegistryTags returns the tags of img filtered by the current regTagFilter.
func (m rootModel) filteredRegistryTags(img registryImage) []registryTag {
	if m.regTagFilter == "" {
		return img.tags
	}
	needle := strings.ToLower(m.regTagFilter)
	var out []registryTag
	for _, t := range img.tags {
		if strings.Contains(strings.ToLower(t.name), needle) {
			out = append(out, t)
		}
	}
	return out
}

// deleteRegistryTags deletes one or more tags from an image.
func (m rootModel) deleteRegistryTags(imageID string, tags []registryTag) tea.Cmd {
	return func() tea.Msg {
		regAPI := registry.NewAPI(m.scwClient)
		region := scw.Region(m.activeRegion)
		if region == "" {
			region = scw.RegionNlAms
		}
		var deleted []string
		for _, tag := range tags {
			tagID := tag.id
			if tagID == "" {
				// Resolve tag ID via API if not cached.
				resp, err := regAPI.ListTags(&registry.ListTagsRequest{
					Region:  region,
					ImageID: imageID,
				})
				if err != nil {
					return errMsg{fmt.Errorf("list tags: %w", err)}
				}
				for _, t := range resp.Tags {
					if t.Name == tag.name {
						tagID = t.ID
						break
					}
				}
			}
			if tagID == "" {
				continue // skip if not found
			}
			_, err := regAPI.DeleteTag(&registry.DeleteTagRequest{
				Region: region,
				TagID:  tagID,
			})
			if err != nil {
				return errMsg{fmt.Errorf("delete tag %q: %w", tag.name, err)}
			}
			deleted = append(deleted, tag.name)
		}
		return registryTagsDeletedMsg{imageID: imageID, tagNames: deleted}
	}
}

// ─────────────────────────────────────────────
// fetchRegistryImages
// ─────────────────────────────────────────────

func (m rootModel) fetchRegistryImages(ns registryNamespace) tea.Cmd {
	return func() tea.Msg {
		regAPI := registry.NewAPI(m.scwClient)
		region := scw.Region(m.activeRegion)
		if region == "" {
			region = scw.RegionNlAms
		}
		req := &registry.ListImagesRequest{
			Region:      region,
			NamespaceID: &ns.id,
		}
		var images []registryImage
		var page int32 = 1
		for {
			resp, err := regAPI.ListImages(req)
			if err != nil {
				return errMsg{fmt.Errorf("list images: %w", err)}
			}
			for _, img := range resp.Images {
				ri := registryImage{
					id:         img.ID,
					name:       img.Name,
					sizeBytes:  uint64(img.Size),
					status:     string(img.Status),
					visibility: string(img.Visibility),
				}
				if img.UpdatedAt != nil {
					ri.updatedAt = *img.UpdatedAt
				}
				// Tags are lazily fetched via fetchRegistryTags when the image is selected.
				images = append(images, ri)
			}
			if uint64(len(images)) >= uint64(resp.TotalCount) {
				break
			}
			page++
			req.Page = scw.Int32Ptr(page)
		}
		return registryImagesMsg{namespace: ns, images: images}
	}
}

// ─────────────────────────────────────────────
// fetchRegistryTags
// ─────────────────────────────────────────────

func (m rootModel) fetchRegistryTags(img registryImage) tea.Cmd {
	return func() tea.Msg {
		regAPI := registry.NewAPI(m.scwClient)
		region := scw.Region(m.activeRegion)
		if region == "" {
			region = scw.RegionNlAms
		}
		req := &registry.ListTagsRequest{
			Region:  region,
			ImageID: img.id,
		}
		var tags []registryTag
		var page int32 = 1
		var totalFetched uint64
		for {
			resp, err := regAPI.ListTags(req)
			if err != nil {
				return errMsg{fmt.Errorf("list tags: %w", err)}
			}
			totalFetched += uint64(len(resp.Tags))
			for _, t := range resp.Tags {
				if !strings.HasPrefix(t.Name, "sha256-") {
					tags = append(tags, registryTag{id: t.ID, name: t.Name})
				}
			}
			if totalFetched >= uint64(resp.TotalCount) {
				break
			}
			page++
			req.Page = scw.Int32Ptr(page)
		}
		return registryTagsMsg{imageID: img.id, tags: tags}
	}
}

// ─────────────────────────────────────────────
// filteredBuckets
// ─────────────────────────────────────────────

// filteredBuckets returns the subset of m.buckets whose names contain the
// current filter string (case-insensitive). When the filter is empty the full
// slice is returned without allocating a new one.
func (m rootModel) filteredBuckets() []bucket {
	if m.bucketFilter == "" {
		return m.buckets
	}
	needle := strings.ToLower(m.bucketFilter)
	var out []bucket
	for _, b := range m.buckets {
		if strings.Contains(strings.ToLower(b.name), needle) {
			out = append(out, b)
		}
	}
	return out
}

// ─────────────────────────────────────────────
// maybeCalculateSize
// ─────────────────────────────────────────────

func (m *rootModel) maybeCalculateSize() tea.Cmd {
	if m.activeService != serviceObjectStorage {
		return nil
	}
	fb := m.filteredBuckets()
	if len(fb) == 0 || m.bucketCursor >= len(fb) {
		return nil
	}
	if m.bucketCursor == m.prevBucketSel {
		return nil
	}
	m.prevBucketSel = m.bucketCursor
	return m.calculateSize()
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
// Profile picker view
// ─────────────────────────────────────────────

func (m rootModel) drawProfilePicker() string {
	if m.loading {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			m.spin.View()+" Connecting to Scaleway...")
	}

	// ── Logo ──
	logoStr := lipgloss.NewStyle().Foreground(colRed).Render(strings.TrimPrefix(logo, "\n"))

	// ── Profile list ──
	const listW = 44
	title := lipgloss.NewStyle().Foreground(colComment).Bold(true).Render("SELECT PROFILE")
	divider := lipgloss.NewStyle().Foreground(colBg3).Render(strings.Repeat("─", listW))

	var rows []string
	for i, name := range m.profileNames {
		region := "?"
		if prof, err := m.scwCfg.GetProfile(name); err == nil && prof.DefaultRegion != nil {
			region = string(*prof.DefaultRegion)
		}
		nameCol := lipgloss.NewStyle().Width(26).Render(name)
		regionCol := lipgloss.NewStyle().Width(10).Render(region)
		line := " " + nameCol + regionCol
		if i == m.profileCursor {
			rows = append(rows, lipgloss.NewStyle().
				Background(colBlue).Foreground(colBg).Bold(true).
				Width(listW).Render(line))
		} else {
			rows = append(rows, lipgloss.NewStyle().
				Foreground(colFg).Width(listW).Render(line))
		}
	}

	// ── Action buttons — all same width, always bordered, active = filled ──
	const btnW = 14
	type actionDef struct {
		label string
		color lipgloss.Color
	}
	actions := []actionDef{
		{"CONNECT", colGreen},
		{"QUIT", colRed},
	}
	var btns []string
	for i, a := range actions {
		label := lipgloss.NewStyle().Width(btnW).Align(lipgloss.Center).Render(a.label)
		if i == m.pickerAction {
			btns = append(btns, lipgloss.NewStyle().
				Background(a.color).Foreground(colBg).Bold(true).
				Border(lipgloss.RoundedBorder()).BorderForeground(a.color).
				Padding(0, 1).Render(label))
		} else {
			btns = append(btns, lipgloss.NewStyle().
				Foreground(a.color).
				Border(lipgloss.RoundedBorder()).BorderForeground(colBg3).
				Padding(0, 1).Render(label))
		}
	}
	buttonRow := lipgloss.JoinHorizontal(lipgloss.Top, btns[0], "  ", btns[1])

	hint := lipgloss.NewStyle().Foreground(colComment).Faint(true).
		Render("↑↓ profile · ←→ action · Enter confirm")

	errLine := ""
	if m.err != nil {
		errLine = "\n" + lipgloss.NewStyle().Foreground(colRed).Render("✗ "+m.err.Error())
	}

	content := lipgloss.JoinVertical(lipgloss.Center,
		logoStr,
		"",
		title,
		divider,
		lipgloss.JoinVertical(lipgloss.Left, rows...),
		"",
		buttonRow,
		"",
		hint,
		errLine,
	)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// ─────────────────────────────────────────────
// Dashboard view
// ─────────────────────────────────────────────

func (m rootModel) drawDashboard() string {
	topBar := m.renderTopBar()
	statusBar := m.renderStatusBar()

	if m.err != nil {
		errPane := panelBox("ERROR", m.width-4, m.height-topBarHeight-statusBarHeight-4, colRed,
			lipgloss.NewStyle().Foreground(colRed).Render("✗ "+m.err.Error()),
		)
		return lipgloss.NewStyle().Margin(1, 2).Render(
			lipgloss.JoinVertical(lipgloss.Left, topBar, errPane, statusBar),
		)
	}

	if m.loading {
		inner := lipgloss.Place(
			m.width-4, m.height-topBarHeight-statusBarHeight-4,
			lipgloss.Center, lipgloss.Center,
			m.spin.View()+" Syncing Scaleway...",
		)
		return lipgloss.NewStyle().Margin(1, 2).Render(
			lipgloss.JoinVertical(lipgloss.Left, topBar, inner, statusBar),
		)
	}

	contentH := m.height - topBarHeight - statusBarHeight - 6
	nav := m.renderNav(contentH)
	content := m.renderContent(contentH)

	base := lipgloss.NewStyle().Margin(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			topBar,
			lipgloss.JoinHorizontal(lipgloss.Top, nav, content),
			statusBar,
		),
	)

	if m.input.active {
		return m.renderInputOverlay(base)
	}
	return base
}

// ─────────────────────────────────────────────
// Top bar
// ─────────────────────────────────────────────

func (m rootModel) renderTopBar() string {
	projectLabel := lipgloss.NewStyle().Foreground(colComment).Render("PROJECT ")
	projectVal := lipgloss.NewStyle().
		Foreground(colGreen).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Padding(0, 1).
		Render(" " + m.project + " ")

	region := lipgloss.NewStyle().Foreground(colComment).Render("  Region: ") +
		lipgloss.NewStyle().Foreground(colBlue).Render(" "+m.activeRegion+" ")
	clock := lipgloss.NewStyle().Foreground(colComment).Render(" " + time.Now().Format("15:04") + " ")

	leftPart := lipgloss.JoinHorizontal(lipgloss.Center, projectLabel, projectVal, region)
	spacer := strings.Repeat(" ", max(0, m.width-lipgloss.Width(leftPart)-lipgloss.Width(clock)-8))
	row := leftPart + spacer + clock

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(colBorder).
		Width(m.width-4).
		Padding(0, 1).
		Render(row)
}

// ─────────────────────────────────────────────
// Status bar
// ─────────────────────────────────────────────

func (m rootModel) renderStatusBar() string {
	hotkey := func(key, desc string) string {
		// Spaces baked into the label string — no Padding() — so the colBg3
		// background doesn't bleed into adjacent cells via padding inheritance.
		k := lipgloss.NewStyle().Background(colBg3).Foreground(colYellow).Bold(true).Render(" " + key + " ")
		d := lipgloss.NewStyle().Foreground(colComment).Background(colBg2).Render(" " + desc + " ")
		return k + d
	}
	keys := lipgloss.JoinHorizontal(lipgloss.Top,
		hotkey("F5", "Refresh"),
		hotkey("Tab", "Focus"),
		hotkey("↑↓", "Navigate"),
		hotkey("Enter", "Open"),
		hotkey("/", "Filter"),
		hotkey("C", "New bucket"),
		hotkey("Esc", "Back"),
		hotkey("Q", "Quit"),
	)
	// barW matches the Width passed to the outer style — no extra Padding() so
	// there is no off-by-two and no background-colour rectangle at the right edge.
	barW := m.width - 4
	spacer := lipgloss.NewStyle().Background(colBg2).Width(max(0, barW-lipgloss.Width(keys))).Render("")
	return lipgloss.NewStyle().
		Background(colBg2).
		Width(barW).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, keys, spacer))
}

// ─────────────────────────────────────────────
// Nav panel
// ─────────────────────────────────────────────

func (m rootModel) renderNav(height int) string {
	services := []struct{ label string }{
		{"Object Storage"},
		{"K8s Clusters"},
		{"Billing"},
		{"Container Registry"},
	}

	sectionHeader := lipgloss.NewStyle().Foreground(colComment).PaddingLeft(1).PaddingBottom(1).Render("SERVICES")

	var rows []string
	rows = append(rows, sectionHeader)
	for i, svc := range services {
		if i == m.activeService {
			label := lipgloss.NewStyle().Foreground(colFg).Bold(true).Render(svc.label)
			rows = append(rows, lipgloss.NewStyle().
				Background(colBg3).PaddingLeft(1).Width(navWidth-2).
				Render(label))
		} else {
			label := lipgloss.NewStyle().Foreground(colComment).Render(svc.label)
			rows = append(rows, lipgloss.NewStyle().PaddingLeft(1).Width(navWidth-2).Render(label))
		}
	}

	focusColor := colBorder
	if m.focus == focusNav {
		focusColor = colRed
	}
	return panelBox("NAV", navWidth, height, focusColor,
		lipgloss.JoinVertical(lipgloss.Left, rows...))
}

// ─────────────────────────────────────────────
// Content panel
// ─────────────────────────────────────────────

func (m rootModel) renderContent(height int) string {
	contentW := m.width - navWidth - 8
	focusColor := colBorder
	if m.focus == focusContent {
		focusColor = colBlue
	}
	switch m.activeService {
	case serviceObjectStorage:
		return m.renderBuckets(contentW, height, focusColor)
	case serviceK8s:
		return m.renderClusters(contentW, height, focusColor)
	case serviceBilling:
		return m.renderBillingPreview(contentW, height, focusColor)
	case serviceRegistry:
		return m.renderRegistry(contentW, height, focusColor)
	}
	return ""
}

// ─────────────────────────────────────────────
// Object Storage view
// ─────────────────────────────────────────────

func (m rootModel) renderBuckets(totalW, height int, borderColor lipgloss.Color) string {
	listW := totalW - detailPaneWidth - 1
	// scrollW=1 col reserved inside content for the vertical scrollbar.
	// Row layout: prefix(2) + name(nameW) + scrollbar(1) = innerW = listW-2
	scrollW := 1
	nameW := listW - 2 - 2 - scrollW // innerW(listW-2) - prefix(2) - scrollbar(1)

	visible := m.filteredBuckets()
	listH := max(1, height-listRowOverhead)

	// ── Scroll viewport ──
	scrollY := m.bucketScrollY
	if m.bucketCursor >= scrollY+listH {
		scrollY = m.bucketCursor - listH + 1
	}
	if m.bucketCursor < scrollY {
		scrollY = m.bucketCursor
	}
	scrollY = max(0, scrollY)

	// ── Scrollbar column ──
	vScrollBar := renderVScrollBar(len(visible), scrollY, listH)

	// ── Build visible rows ──
	var rows []string
	if len(visible) == 0 {
		msg := "  No buckets found in this project."
		if m.bucketFilter != "" {
			msg = "  No buckets match \"" + m.bucketFilter + "\"."
		}
		// Pad with scrollbar chars on the right.
		for si := 0; si < listH; si++ {
			sb := ""
			if si < len(vScrollBar) {
				sb = vScrollBar[si]
			}
			if si == 0 {
				rows = append(rows, lipgloss.NewStyle().Faint(true).Width(listW-2-scrollW).Render(msg)+sb)
			} else {
				rows = append(rows, strings.Repeat(" ", listW-2-scrollW)+sb)
			}
		}
	}

	end := min(scrollY+listH, len(visible))
	for i := scrollY; i < end; i++ {
		b := visible[i]
		var name string
		if m.bucketFilter != "" {
			name = highlightMatch(b.name, m.bucketFilter)
		} else {
			name = b.name
			runes := []rune(name)
			if m.bucketScrollX > 0 {
				if m.bucketScrollX >= len(runes) {
					name = ""
				} else {
					name = string(runes[m.bucketScrollX:])
				}
			}
		}
		sb := ""
		if i-scrollY < len(vScrollBar) {
			sb = vScrollBar[i-scrollY]
		}
		rowStr := padRight(name, nameW) + sb
		if i == m.bucketCursor {
			rowStr = lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(listW - 2).Render("▌ " + padRight(name, nameW) + sb)
		} else {
			rowStr = lipgloss.NewStyle().Foreground(colFg).Width(listW - 2).Render("  " + padRight(name, nameW) + sb)
		}
		rows = append(rows, rowStr)
	}

	// ── Header / filter bar ──
	var header string
	switch {
	case m.bucketFiltering:
		header = lipgloss.NewStyle().Foreground(colComment).Render("/") +
			lipgloss.NewStyle().Foreground(colFg).Render(m.bucketFilter) +
			lipgloss.NewStyle().Foreground(colGreen).Render("▌")
	case m.bucketFilter != "":
		header = lipgloss.NewStyle().Foreground(colYellow).Render("/ "+m.bucketFilter) +
			lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("  Esc to clear")
	default:
		hint := ""
		if m.bucketScrollX > 0 {
			hint = fmt.Sprintf(" ◀+%d", m.bucketScrollX)
		}
		hintW := lipgloss.Width(hint)
		header = "  " + lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(padRight("NAME", nameW-hintW))
		if hint != "" {
			header += lipgloss.NewStyle().Foreground(colComment).Faint(true).Render(hint)
		}
	}

	panelTitle := "OBJECT STORAGE"
	if m.bucketFilter != "" {
		panelTitle = fmt.Sprintf("OBJECT STORAGE  %d/%d", len(visible), len(m.buckets))
	}

	listContent := lipgloss.JoinVertical(lipgloss.Left,
		header,
		strings.Repeat("─", listW-2),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	// No gutter — right border is always the plain panel border.
	listPane := panelBox(panelTitle, listW, height, borderColor, listContent)
	detailPane := panelBox("BUCKET INFO", detailPaneWidth, height, colPurple, m.renderBucketDetail())
	return lipgloss.JoinHorizontal(lipgloss.Top, listPane, detailPane)
}

// renderVScrollBar returns a slice of single-character strings representing a
// minimal vertical scrollbar, one string per visible row.
func renderVScrollBar(total, offset, visible int) []string {
	out := make([]string, visible)
	for i := range out {
		out[i] = lipgloss.NewStyle().Foreground(colBg3).Render("│")
	}
	if total <= visible {
		return out
	}
	thumbH := max(1, visible*visible/total)
	thumbTop := (offset * (visible - thumbH)) / max(1, total-visible)
	for i := thumbTop; i < thumbTop+thumbH && i < visible; i++ {
		out[i] = lipgloss.NewStyle().Foreground(colComment).Render("█")
	}
	return out
}

// min is a helper for Go versions before 1.21.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (m rootModel) renderBucketDetail() string {
	fb := m.filteredBuckets()
	if len(fb) == 0 || m.bucketCursor >= len(fb) {
		return lipgloss.NewStyle().Faint(true).Render("Select a bucket")
	}
	b := fb[m.bucketCursor]

	// Inner width: subtract borders (2) and padding (2).
	innerW := detailPaneWidth - 4
	nameDisplay := b.name
	if lipgloss.Width(nameDisplay) > innerW {
		nameDisplay = string([]rune(nameDisplay)[:innerW-1]) + "…"
	}

	// Align values in the Usage block by padding keys to the same width.
	usageKey := func(key, val string, valColor lipgloss.Color) string {
		k := lipgloss.NewStyle().Foreground(colComment).Render(padRight(key, 9))
		v := lipgloss.NewStyle().Foreground(valColor).Render(" " + val + " ")
		return k + v
	}

	lines := []string{
		lipgloss.NewStyle().Foreground(colBlue).Bold(true).Render(" " + nameDisplay + " "),
		"",
		lipgloss.NewStyle().Foreground(colComment).Render("Created: ") +
			lipgloss.NewStyle().Foreground(colFg).Render(" "+b.created+" "),
		lipgloss.NewStyle().Foreground(colComment).Render("Region:  ") +
			lipgloss.NewStyle().Foreground(colBlue).Render(" "+m.activeRegion+" "),
		"",
		lipgloss.NewStyle().Foreground(colComment).Bold(true).Render("Usage:"),
	}

	if b.sizeReady {
		lines = append(lines,
			usageKey("Objects:", fmt.Sprintf("%d", b.objCount), colGreen),
			usageKey("Size:", formatBytes(b.sizeBytes), colBlue),
		)
	} else {
		lines = append(lines,
			lipgloss.NewStyle().Faint(true).Render("  Calculating…"),
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// ─────────────────────────────────────────────
// K8s Clusters view
// ─────────────────────────────────────────────

func (m rootModel) renderClusters(totalW, height int, borderColor lipgloss.Color) string {
	nameW := totalW - 30
	statusW := 12
	versionW := 10

	header := lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
		padRight("CLUSTER", nameW) + padRight("VERSION", versionW) + padRight("STATUS", statusW),
	)

	var rows []string
	if len(m.clusters) == 0 {
		rows = append(rows, lipgloss.NewStyle().Faint(true).Render("No clusters found in this region."))
	}
	for i, cl := range m.clusters {
		statusColor := colGreen
		switch strings.ToLower(cl.status) {
		case "warning", "upgrading", "scaling":
			statusColor = colYellow
		case "error", "locked", "unknown":
			statusColor = colRed
		}
		status := lipgloss.NewStyle().Foreground(statusColor).Render(cl.status)
		rowStr := padRight(cl.name, nameW) + padRight(cl.version, versionW) + status
		if i == m.clusterCursor {
			rowStr = lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(totalW - 4).Render("▌ " + rowStr)
		} else {
			rowStr = lipgloss.NewStyle().Foreground(colFg).Width(totalW - 4).Render("  " + rowStr)
		}
		rows = append(rows, rowStr)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		strings.Repeat("─", totalW-4),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	return panelBox("K8S CLUSTERS", totalW, height, borderColor, content)
}

// ─────────────────────────────────────────────
// Container Registry view
// ─────────────────────────────────────────────

func (m rootModel) renderRegistry(totalW, height int, borderColor lipgloss.Color) string {
	nameW := totalW - 32
	imagesW := 8
	sizeW := 10
	visW := 8

	visible := m.filteredRegistryNamespaces()
	listH := max(1, height-listRowOverhead)
	scrollY := m.registryScrollY
	if m.registryCursor >= scrollY+listH {
		scrollY = m.registryCursor - listH + 1
	}
	if m.registryCursor < scrollY {
		scrollY = m.registryCursor
	}

	var rows []string
	if len(visible) == 0 {
		msg := "  No container registry namespaces found."
		if m.registryFilter != "" {
			msg = "  No namespaces match \"" + m.registryFilter + "\"."
		}
		rows = append(rows, lipgloss.NewStyle().Faint(true).Render(msg))
	}
	end := min(scrollY+listH, len(visible))
	for i := scrollY; i < end; i++ {
		ns := visible[i]

		statusColor := colGreen
		switch ns.status {
		case "error", "locked":
			statusColor = colRed
		case "deleting":
			statusColor = colYellow
		}
		vis := "private"
		if ns.isPublic {
			vis = "public"
		}
		sizeStr := formatBytes(int64(ns.sizeBytes))
		imagesStr := fmt.Sprintf("%d", ns.imageCount)

		var nameStr string
		if m.registryFilter != "" {
			nameStr = highlightMatch(ns.name, m.registryFilter)
		} else {
			nameStr = lipgloss.NewStyle().Foreground(statusColor).Render(ns.name)
		}
		rowStr := padRight(nameStr, nameW) + padRight(imagesStr, imagesW) + padRight(sizeStr, sizeW) + padRight(vis, visW)

		if i == m.registryCursor {
			rows = append(rows, lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(totalW-4).Render("▌ "+rowStr))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(colFg).Width(totalW-4).Render("  "+rowStr))
		}
	}

	// ── Header / filter bar ──
	var header string
	switch {
	case m.registryFiltering:
		header = lipgloss.NewStyle().Foreground(colComment).Render("/") +
			lipgloss.NewStyle().Foreground(colFg).Render(m.registryFilter) +
			lipgloss.NewStyle().Foreground(colGreen).Render("▌")
	case m.registryFilter != "":
		header = lipgloss.NewStyle().Foreground(colYellow).Render("/ "+m.registryFilter) +
			lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("  Esc to clear")
	default:
		header = "  " + lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
			padRight("NAMESPACE", nameW) + padRight("IMAGES", imagesW) + padRight("SIZE", sizeW) + padRight("VIS", visW),
		)
	}

	panelTitle := "CONTAINER REGISTRY"
	if m.registryFilter != "" {
		panelTitle = fmt.Sprintf("CONTAINER REGISTRY  %d/%d", len(visible), len(m.registryNamespaces))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		strings.Repeat("─", totalW-4),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	return panelBox(panelTitle, totalW, height, borderColor, content)
}

// ─────────────────────────────────────────────
// Render helpers
// ─────────────────────────────────────────────

// panelBox renders a btop-style bordered box with a title embedded in the top border.
// rightGutter, if non-nil, provides per-row right-border characters (used for scrollbars).
func panelBox(title string, w, h int, borderColor lipgloss.Color, content string, rightGutter ...string) string {
	// Always use the same border characters — active state is shown via colour only,
	// not border thickness, so all panels stay visually aligned.
	bc := lipgloss.NormalBorder()

	titleStr := " " + title + " "
	titleRendered := lipgloss.NewStyle().Foreground(borderColor).Bold(true).Render(titleStr)

	// Top border: TopLeft + Top + title + dashes + TopRight
	// Total fixed border chars: TopLeft(1) + Top(1) before title + TopRight(1) = 3
	dashCount := max(0, w-lipgloss.Width(titleStr)-3)
	borderSt := lipgloss.NewStyle().Foreground(borderColor)
	topLine := borderSt.Render(bc.TopLeft+bc.Top) +
		titleRendered +
		borderSt.Render(strings.Repeat(bc.Top, dashCount)+bc.TopRight)

	innerH := max(1, h-2)
	innerW := max(1, w-2)
	// contentW is always innerW — the gutter character replaces defaultSideR (same 1 col).
	contentW := innerW

	contentLines := strings.Split(content, "\n")
	for len(contentLines) < innerH {
		contentLines = append(contentLines, "")
	}
	if len(contentLines) > innerH {
		contentLines = contentLines[:innerH]
	}

	side := borderSt.Render(bc.Left)
	defaultSideR := borderSt.Render(bc.Right)
	bottomLine := borderSt.Render(bc.BottomLeft + strings.Repeat(bc.Bottom, innerW) + bc.BottomRight)

	var sb strings.Builder
	sb.WriteString(topLine + "\n")
	for i, line := range contentLines {
		vis := lipgloss.Width(line)
		pad := ""
		if vis < contentW {
			pad = strings.Repeat(" ", contentW-vis)
		}
		sideR := defaultSideR
		if i < len(rightGutter) {
			sideR = rightGutter[i]
		}
		sb.WriteString(side + line + pad + sideR + "\n")
	}
	sb.WriteString(bottomLine)
	return sb.String()
}

// renderBar draws a btop-style █░ usage bar with a label and percentage.
func renderBar(label string, value, maxVal float64, width int) string {
	if maxVal <= 0 {
		maxVal = 1
	}
	pct := value / maxVal
	if pct > 1 {
		pct = 1
	}
	barW := width - 2
	filled := int(pct * float64(barW))
	empty := barW - filled
	barColor := colGreen
	switch {
	case pct > 0.85:
		barColor = colRed
	case pct > 0.60:
		barColor = colYellow
	}
	bar := lipgloss.NewStyle().Foreground(barColor).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(colBg3).Render(strings.Repeat("░", empty))
	pctStr := lipgloss.NewStyle().Foreground(colComment).Render(fmt.Sprintf(" %.0f%%", pct*100))
	return lipgloss.NewStyle().Foreground(colComment).Render(label+": ") + "\n" + bar + pctStr
}

// highlightMatch wraps the first case-insensitive occurrence of needle in s
// with a yellow colour for display in the bucket filter list.
func highlightMatch(s, needle string) string {
	lower := strings.ToLower(s)
	lowerN := strings.ToLower(needle)
	idx := strings.Index(lower, lowerN)
	if idx < 0 {
		return s
	}
	before := s[:idx]
	match := s[idx : idx+len(needle)]
	after := s[idx+len(needle):]
	return before +
		lipgloss.NewStyle().Foreground(colYellow).Bold(true).Render(match) +
		after
}

// padRight pads s to exactly n visible characters.
// If s is wider than n it is truncated and a trailing … is appended so the
// reader can see the value was clipped. Horizontal scroll lets them reveal the rest.
func padRight(s string, n int) string {
	if n <= 0 {
		return ""
	}
	vis := lipgloss.Width(s)
	if vis <= n {
		return s + strings.Repeat(" ", n-vis)
	}
	// Truncate to n-1 runes and append ellipsis.
	runes := []rune(s)
	cut := n - 1
	if cut < 0 {
		cut = 0
	}
	return string(runes[:cut]) + "…"
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.2f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.2f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ─────────────────────────────────────────────
// Data fetching
// ─────────────────────────────────────────────

func (m rootModel) fetchData() tea.Cmd {
	return func() tea.Msg {
		var buckets []bucket
		var clusters []cluster
		var projects []projectItem
		var registryNamespaces []registryNamespace

		// ── Project name ──
		// We skip ListProjects entirely (insufficient permissions) and instead
		// use the default_project_id from the profile. We try a single GetProject
		// call to resolve the human-readable name; if that also fails (same perm
		// issue), we fall back to showing the raw ID — the rest of the app still
		// works fine either way.
		if m.projectID != "" {
			pAPI := account.NewProjectAPI(m.scwClient)
			if resp, err := pAPI.GetProject(&account.ProjectAPIGetProjectRequest{
				ProjectID: m.projectID,
			}); err == nil {
				projects = []projectItem{{name: resp.Name, id: resp.ID}}
			} else {
				// Perm denied on GetProject too — just show the ID.
				projects = []projectItem{{name: m.projectID, id: m.projectID}}
			}
		}

		// ── Clusters ──
		k8sAPI := k8s.NewAPI(m.scwClient)
		region := scw.Region(m.activeRegion)
		if region == "" {
			region = scw.RegionNlAms
		}
		kReq := &k8s.ListClustersRequest{Region: region}
		if m.projectID != "" {
			kReq.ProjectID = &m.projectID
		}
		var clPage int32 = 1
		for {
			resp, err := k8sAPI.ListClusters(kReq)
			if err != nil {
				return errMsg{fmt.Errorf("list clusters: %w", err)}
			}
			for _, cl := range resp.Clusters {
				clusters = append(clusters, cluster{
					name:    cl.Name,
					status:  string(cl.Status),
					version: cl.Version,
				})
			}
			if uint64(len(clusters)) >= uint64(resp.TotalCount) {
				break
			}
			clPage++
			kReq.Page = scw.Int32Ptr(clPage)
		}

		// ── Buckets ──
		bkts, err := m.minioClient.ListBuckets(context.Background())
		if err != nil {
			return errMsg{fmt.Errorf("list buckets: %w", err)}
		}
		for _, b := range bkts {
			buckets = append(buckets, bucket{
				name:    b.Name,
				created: b.CreationDate.Format("2006-01-02"),
			})
		}

		// ── Registry namespaces ──
		regAPI := registry.NewAPI(m.scwClient)
		rReq := &registry.ListNamespacesRequest{Region: region}
		if m.projectID != "" {
			rReq.ProjectID = &m.projectID
		}
		var rPage int32 = 1
		for {
			resp, err := regAPI.ListNamespaces(rReq)
			if err != nil {
				break // non-fatal: registry may not be enabled
			}
			for _, ns := range resp.Namespaces {
				registryNamespaces = append(registryNamespaces, registryNamespace{
					id:         ns.ID,
					name:       ns.Name,
					endpoint:   ns.Endpoint,
					imageCount: ns.ImageCount,
					sizeBytes:  uint64(ns.Size),
					status:     string(ns.Status),
					isPublic:   ns.IsPublic,
				})
			}
			if uint64(len(registryNamespaces)) >= uint64(resp.TotalCount) {
				break
			}
			rPage++
			rReq.Page = scw.Int32Ptr(rPage)
		}

		return dataMsg{buckets: buckets, clusters: clusters, projects: projects, registryNamespaces: registryNamespaces}
	}
}

func (m rootModel) calculateSize() tea.Cmd {
	fb := m.filteredBuckets()
	if len(fb) == 0 || m.bucketCursor >= len(fb) {
		return nil
	}
	targetBucket := fb[m.bucketCursor].name
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var totalSize int64
		var count int
		for obj := range m.minioClient.ListObjects(ctx, targetBucket, minio.ListObjectsOptions{Recursive: true}) {
			if obj.Err != nil {
				continue
			}
			count++
			totalSize += obj.Size
		}
		return sizeMsg{bucketName: targetBucket, sizeBytes: totalSize, objCount: count}
	}
}

// ─────────────────────────────────────────────
// Object browser — fetch and render
// ─────────────────────────────────────────────

// fetchBucketContents lists one "directory level" of a bucket using a `/`
// delimiter so virtual folders appear as their own entries.
func (m rootModel) fetchBucketContents(bucketName, prefix string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var entries []bucketEntry

		for obj := range m.minioClient.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
			Prefix:    prefix,
			Recursive: false,
		}) {
			if obj.Err != nil {
				return errMsg{fmt.Errorf("listing %s/%s: %w", bucketName, prefix, obj.Err)}
			}
			// Virtual folder — Key ends with "/"
			if strings.HasSuffix(obj.Key, "/") {
				name := strings.TrimPrefix(obj.Key, prefix)
				name = strings.TrimSuffix(name, "/")
				entries = append(entries, bucketEntry{
					name:    name,
					fullKey: obj.Key,
					isDir:   true,
				})
			} else {
				name := strings.TrimPrefix(obj.Key, prefix)
				sc := obj.StorageClass
				if sc == "" {
					sc = "STANDARD"
				}
				entries = append(entries, bucketEntry{
					name:         name,
					fullKey:      obj.Key,
					isDir:        false,
					size:         obj.Size,
					lastModified: obj.LastModified,
					storageClass: sc,
				})
			}
		}

		return bucketContentsMsg{bucket: bucketName, prefix: prefix, entries: entries}
	}
}

// parentPrefix returns the prefix one level up.
// e.g. "a/b/c/" → "a/b/"  "a/" → ""
func parentPrefix(prefix string) string {
	trimmed := strings.TrimSuffix(prefix, "/")
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 {
		return ""
	}
	return trimmed[:idx+1]
}

// drawObjectBrowser renders the full-screen object browser.
func (m rootModel) drawObjectBrowser() string {
	topBar := m.renderBrowserTopBar()
	statusBar := m.renderBrowserStatusBar()

	if m.upload.active {
		// Show progress overlay — render the content behind it first.
		contentH := m.height - topBarHeight - statusBarHeight - 6
		content := m.renderBrowserContent(m.width-8, contentH)
		base := lipgloss.NewStyle().Margin(1, 2).Render(
			lipgloss.JoinVertical(lipgloss.Left, topBar, content, statusBar),
		)
		return m.renderUploadProgress(base)
	}

	if m.loading {
		inner := lipgloss.Place(
			m.width-4, m.height-topBarHeight-statusBarHeight-4,
			lipgloss.Center, lipgloss.Center,
			m.spin.View()+" Loading…",
		)
		return lipgloss.NewStyle().Margin(1, 2).Render(
			lipgloss.JoinVertical(lipgloss.Left, topBar, inner, statusBar),
		)
	}

	contentH := m.height - topBarHeight - statusBarHeight - 6
	content := m.renderBrowserContent(m.width-8, contentH)

	base := lipgloss.NewStyle().Margin(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, topBar, content, statusBar),
	)

	if m.showConfirm {
		return m.renderConfirmDialog(base)
	}
	if m.input.active {
		return m.renderInputOverlay(base)
	}
	return base
}

// ─────────────────────────────────────────────
// Registry browser view
// ─────────────────────────────────────────────

func (m rootModel) drawRegistryBrowser() string {
	ns := m.regBrowserNamespace
	visible := m.filteredRegistryImages()

	// ── Top bar ──
	crumb := lipgloss.NewStyle().Foreground(colComment).Render("REGISTRY ")
	nsPart := lipgloss.NewStyle().
		Foreground(colGreen).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Padding(0, 1).
		Render(ns.name)
	leftPart := lipgloss.JoinHorizontal(lipgloss.Center, crumb, nsPart)
	countStr := lipgloss.NewStyle().Foreground(colComment).Render(
		fmt.Sprintf("%d images", len(m.regBrowserImages)),
	)
	spacer := strings.Repeat(" ", max(0, m.width-lipgloss.Width(leftPart)-lipgloss.Width(countStr)-8))
	topBar := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(colBorder).
		Width(m.width-4).Padding(0, 1).
		Render(leftPart + spacer + countStr)

	// ── Status bar ──
	hotkey := func(key, desc string) string {
		k := lipgloss.NewStyle().Background(colBg3).Foreground(colYellow).Bold(true).Render(" " + key + " ")
		d := lipgloss.NewStyle().Foreground(colComment).Background(colBg2).Render(" " + desc + " ")
		return k + d
	}
	var keys string
	if m.regBrowserFocus == 1 {
		keys = lipgloss.JoinHorizontal(lipgloss.Top,
			hotkey("↑↓", "Navigate"),
			hotkey("Tab", "Images"),
			hotkey("Enter", "Pull"),
			hotkey("Space", "Select"),
			hotkey("A", "Select all"),
			hotkey("D", "Delete"),
			hotkey("/", "Filter"),
			hotkey("Esc", "Back"),
			hotkey("Q", "Quit"),
		)
	} else {
		keys = lipgloss.JoinHorizontal(lipgloss.Top,
			hotkey("↑↓", "Navigate"),
			hotkey("Tab", "Versions"),
			hotkey("/", "Filter"),
			hotkey("Esc", "Back"),
			hotkey("F5", "Refresh"),
			hotkey("Q", "Quit"),
		)
	}
	barW := m.width - 4
	spacerBar := lipgloss.NewStyle().Background(colBg2).Width(max(0, barW-lipgloss.Width(keys))).Render("")
	statusBar := lipgloss.NewStyle().Background(colBg2).Width(barW).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, keys, spacerBar))

	if m.loading {
		inner := lipgloss.Place(
			m.width-4, m.height-topBarHeight-statusBarHeight-4,
			lipgloss.Center, lipgloss.Center,
			m.spin.View()+" Loading…",
		)
		return lipgloss.NewStyle().Margin(1, 2).Render(
			lipgloss.JoinVertical(lipgloss.Left, topBar, inner, statusBar),
		)
	}

	// ── Column layout ──
	const regDetailPaneW = 52
	contentW := m.width - 8
	contentH := m.height - topBarHeight - statusBarHeight - 6
	listW := contentW - regDetailPaneW - 1
	const scrollW = 1
	const sizeW = 10
	const modW = 16
	const prefixW = 2
	rowW := listW - 2
	nameW := rowW - prefixW - sizeW - modW - scrollW
	if nameW < 8 {
		nameW = 8
	}

	listH := max(1, contentH-listRowOverhead)
	scrollY := m.regBrowserScrollY
	if m.regBrowserCursor >= scrollY+listH {
		scrollY = m.regBrowserCursor - listH + 1
	}
	if m.regBrowserCursor < scrollY {
		scrollY = m.regBrowserCursor
	}

	vScrollBar := renderVScrollBar(len(visible), scrollY, listH)

	listBorderColor := colPurple
	if m.regBrowserFocus == 1 {
		listBorderColor = colBorder
	}

	// ── Image list header / filter bar ──
	var listHeader string
	switch {
	case m.regBrowserFiltering:
		listHeader = lipgloss.NewStyle().Foreground(colComment).Render("/") +
			lipgloss.NewStyle().Foreground(colFg).Render(m.regBrowserFilter) +
			lipgloss.NewStyle().Foreground(colGreen).Render("▌")
	case m.regBrowserFilter != "":
		listHeader = lipgloss.NewStyle().Foreground(colYellow).Render("/ "+m.regBrowserFilter) +
			lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("  Esc to clear")
	default:
		listHeader = "  " + lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
			padRight("IMAGE", nameW) + padRight("MODIFIED", modW) + padRight("SIZE", sizeW),
		)
	}

	// ── Image rows ──
	var rows []string
	if len(visible) == 0 {
		noMsg := "  No images in this namespace."
		if m.regBrowserFilter != "" {
			noMsg = "  No images match \"" + m.regBrowserFilter + "\"."
		}
		for si := 0; si < listH; si++ {
			sb := ""
			if si < len(vScrollBar) {
				sb = vScrollBar[si]
			}
			if si == 0 {
				rows = append(rows, lipgloss.NewStyle().Faint(true).Width(rowW-scrollW).Render(noMsg)+sb)
			} else {
				rows = append(rows, strings.Repeat(" ", rowW-scrollW)+sb)
			}
		}
	}

	end := min(scrollY+listH, len(visible))
	for i := scrollY; i < end; i++ {
		img := visible[i]
		sb := ""
		if i-scrollY < len(vScrollBar) {
			sb = vScrollBar[i-scrollY]
		}

		statusColor := colGreen
		switch img.status {
		case "error", "locked":
			statusColor = colRed
		case "deleting":
			statusColor = colYellow
		}

		sizeStr := formatBytes(int64(img.sizeBytes))
		modStr := ""
		if !img.updatedAt.IsZero() {
			modStr = img.updatedAt.Format("2006-01-02")
		}

		var nameCol string
		if m.regBrowserFilter != "" {
			nameCol = padRight(highlightMatch(img.name, m.regBrowserFilter), nameW)
		} else {
			nameCol = lipgloss.NewStyle().Foreground(statusColor).Render(padRight(img.name, nameW))
		}
		modCol := lipgloss.NewStyle().Foreground(colComment).Render(padRight(modStr, modW))
		rowStr := nameCol + modCol + padRight(sizeStr, sizeW) + sb

		if i == m.regBrowserCursor {
			rows = append(rows, lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(rowW).Render("▌ "+rowStr))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(colFg).Width(rowW).Render("  "+rowStr))
		}
	}

	panelTitle := ns.endpoint
	if m.regBrowserFilter != "" {
		panelTitle = fmt.Sprintf("%s  %d/%d", ns.endpoint, len(visible), len(m.regBrowserImages))
	}

	listContent := lipgloss.JoinVertical(lipgloss.Left,
		listHeader,
		strings.Repeat("─", rowW),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	listPane := panelBox(panelTitle, listW, contentH, listBorderColor, listContent)
	detailPane := m.renderRegistryVersionPane(regDetailPaneW, contentH)

	content := lipgloss.JoinHorizontal(lipgloss.Top, listPane, detailPane)
	base := lipgloss.NewStyle().Margin(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, topBar, content, statusBar),
	)

	if m.regTagActionOverlay {
		return m.renderRegistryTagActionOverlay()
	}
	if m.regConfirmDeleteTags {
		return m.renderRegistryTagsDeleteConfirm(base)
	}
	return base
}

// renderRegistryVersionPane renders the right-hand versions/tags detail pane.
func (m rootModel) renderRegistryVersionPane(paneW, paneH int) string {
	borderColor := colBorder
	if m.regBrowserFocus == 1 {
		borderColor = colPurple
	}

	visible := m.filteredRegistryImages()
	if len(visible) == 0 || m.regBrowserCursor >= len(visible) {
		return panelBox("VERSIONS", paneW, paneH, borderColor,
			lipgloss.NewStyle().Faint(true).Render("Select an image"))
	}

	img := visible[m.regBrowserCursor]

	if m.regTagsLoading {
		return panelBox("VERSIONS", paneW, paneH, borderColor,
			lipgloss.NewStyle().Foreground(colComment).Render("Loading…"))
	}

	tags := m.filteredRegistryTags(img)

	const scrollW = 1
	const prefixW = 2
	const chkW = 4 // "[x] " or "[ ] "
	innerW := paneW - 2
	tagW := innerW - prefixW - scrollW - chkW

	tagListH := max(1, paneH-listRowOverhead)
	scrollY := m.regBrowserTagScrollY
	if m.regBrowserTagCursor >= scrollY+tagListH {
		scrollY = m.regBrowserTagCursor - tagListH + 1
	}
	if m.regBrowserTagCursor < scrollY {
		scrollY = m.regBrowserTagCursor
	}

	vScrollBar := renderVScrollBar(len(tags), scrollY, tagListH)

	// Title: show selected count, filter count, or plain count.
	var title string
	switch {
	case len(m.regTagSelected) > 0:
		title = fmt.Sprintf("VERSIONS (%d selected)", len(m.regTagSelected))
	case m.regTagFilter != "":
		title = fmt.Sprintf("VERSIONS (%d/%d)", len(tags), len(img.tags))
	default:
		title = fmt.Sprintf("VERSIONS (%d)", len(tags))
	}

	// Header: filter bar when filtering, otherwise column header with hint.
	var headerStr string
	switch {
	case m.regTagFiltering:
		headerStr = lipgloss.NewStyle().Foreground(colComment).Render("/") +
			lipgloss.NewStyle().Foreground(colFg).Render(m.regTagFilter) +
			lipgloss.NewStyle().Foreground(colGreen).Render("▌")
	case m.regTagFilter != "":
		headerStr = lipgloss.NewStyle().Foreground(colYellow).Render("/ "+m.regTagFilter) +
			lipgloss.NewStyle().Foreground(colComment).Faint(true).Render("  Esc to clear")
	case m.regBrowserFocus == 1:
		const hintStr = "Enter  Pull"
		const hintW = len(hintStr)
		headerStr = lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(padRight("TAG", tagW+chkW-hintW)) +
			lipgloss.NewStyle().Foreground(colComment).Faint(true).Render(hintStr)
	default:
		headerStr = lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(padRight("TAG", tagW+chkW))
	}

	var rows []string
	if len(tags) == 0 {
		noMsg := "  No tags"
		if m.regTagFilter != "" {
			noMsg = "  No tags match \"" + m.regTagFilter + "\""
		}
		for si := 0; si < tagListH; si++ {
			sb := ""
			if si < len(vScrollBar) {
				sb = vScrollBar[si]
			}
			if si == 0 {
				rows = append(rows, lipgloss.NewStyle().Faint(true).Width(innerW-scrollW).Render(noMsg)+sb)
			} else {
				rows = append(rows, strings.Repeat(" ", innerW-scrollW)+sb)
			}
		}
	}

	end := min(scrollY+tagListH, len(tags))
	for i := scrollY; i < end; i++ {
		tag := tags[i]
		sb := ""
		if i-scrollY < len(vScrollBar) {
			sb = vScrollBar[i-scrollY]
		}

		isSelected := m.regTagSelected[tag.name]
		isCursor := m.regBrowserFocus == 1 && i == m.regBrowserTagCursor

		var chk string
		if isSelected {
			chk = lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render("[x] ")
		} else {
			chk = lipgloss.NewStyle().Foreground(colComment).Render("[ ] ")
		}

		var tagCol string
		if m.regTagFilter != "" {
			tagCol = padRight(highlightMatch(tag.name, m.regTagFilter), tagW)
		} else {
			tagCol = lipgloss.NewStyle().Foreground(colGreen).Render(padRight(tag.name, tagW))
		}
		rowStr := chk + tagCol + sb

		if isCursor {
			rows = append(rows, lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(innerW).Render("▌ "+rowStr))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(colFg).Width(innerW).Render("  "+rowStr))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		headerStr,
		strings.Repeat("─", innerW),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	return panelBox(title, paneW, paneH, borderColor, content)
}

// renderRegistryTagActionOverlay shows pull instructions and a delete button for
// the tag currently selected in the versions pane.
func (m rootModel) renderRegistryTagActionOverlay() string {
	visible := m.filteredRegistryImages()
	if len(visible) == 0 || m.regBrowserCursor >= len(visible) {
		return ""
	}
	img := visible[m.regBrowserCursor]
	if len(img.tags) == 0 || m.regBrowserTagCursor >= len(img.tags) {
		return ""
	}
	tag := img.tags[m.regBrowserTagCursor]
	ns := m.regBrowserNamespace

	pullBase := ns.endpoint + "/" + img.name
	var pullCmd string
	if strings.HasPrefix(tag.name, "sha256-") {
		pullCmd = "docker pull " + pullBase + "@sha256:" + tag.name[len("sha256-"):]
	} else {
		pullCmd = "docker pull " + pullBase + ":" + tag.name
	}

	dialogW := min(m.width-8, 90)
	innerW := dialogW - 6
	bg := lipgloss.NewStyle().Background(colBg2)

	heading := bg.Foreground(colPurple).Bold(true).Width(innerW).Render("Pull Instructions")
	imgLine := bg.Width(innerW).Render(
		bg.Foreground(colComment).Render("Image: ") +
			bg.Foreground(colFg).Render(img.name),
	)
	tagLine := bg.Width(innerW).Render(
		bg.Foreground(colComment).Render("Tag:   ") +
			bg.Foreground(colGreen).Render(tag.name),
	)
	empty := bg.Width(innerW).Render("")

	codeBlock := lipgloss.NewStyle().
		Background(colBg3).Foreground(colGreen).
		Padding(0, 1).Width(innerW).
		Render("$ " + pullCmd)

	divider := bg.Foreground(colBg3).Width(innerW).Render(strings.Repeat("─", innerW))

	closeBtn := lipgloss.NewStyle().
		Background(colBg3).Foreground(colFg).
		Width(innerW).Align(lipgloss.Center).
		Render("Esc  Close")

	body := bg.Width(innerW).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			heading, empty, imgLine, tagLine, empty, codeBlock, empty, divider, empty, closeBtn,
		),
	)
	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colPurple).
		Background(colBg2).
		Padding(1, 2).
		Width(dialogW).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(colBg),
	)
}

// renderRegistryTagsDeleteConfirm shows a delete confirmation for one or more tags.
func (m rootModel) renderRegistryTagsDeleteConfirm(base string) string {
	tags := m.regConfirmTagsToDelete
	if len(tags) == 0 {
		return base
	}
	n := len(tags)

	const dialogW = 54
	const innerW = dialogW - 6
	bg := lipgloss.NewStyle().Background(colBg2)

	countStr := "1 tag"
	if n > 1 {
		countStr = fmt.Sprintf("%d tags", n)
	}
	heading := bg.Foreground(colRed).Bold(true).Width(innerW).Render("Delete " + countStr + "?")

	imgDisplay := m.regConfirmDeleteImgName
	if lipgloss.Width(imgDisplay) > innerW-8 {
		rr := []rune(imgDisplay)
		imgDisplay = string(rr[:innerW-9]) + "\u2026"
	}
	imgLine := bg.Width(innerW).Render(
		lipgloss.NewStyle().Background(colBg2).Foreground(colComment).Render("Image: ") +
			lipgloss.NewStyle().Background(colBg2).Foreground(colFg).Render(imgDisplay),
	)

	const maxShow = 5
	var tagLines []string
	for i, t := range tags {
		if i >= maxShow {
			more := bg.Foreground(colComment).Faint(true).Width(innerW).
				Render(fmt.Sprintf("  \u2026 and %d more", n-maxShow))
			tagLines = append(tagLines, more)
			break
		}
		line := bg.Width(innerW).Render(
			lipgloss.NewStyle().Background(colBg2).Foreground(colComment).Render("  \u00b7 ") +
				lipgloss.NewStyle().Background(colBg2).Foreground(colGreen).Bold(true).Render(t.name),
		)
		tagLines = append(tagLines, line)
	}

	warn := bg.Foreground(colComment).Width(innerW).Render("This action is irreversible.")
	divider := bg.Foreground(colBg3).Width(innerW).Render(strings.Repeat("\u2500", innerW))
	empty := bg.Width(innerW).Render("")

	btnW := (innerW - 1) / 2
	leftW := btnW + ((innerW - 1) % 2)
	yesBtn := lipgloss.NewStyle().
		Background(colRed).Foreground(lipgloss.Color("#ffffff")).
		Bold(true).Width(leftW).Align(lipgloss.Center).
		Render("Y  Yes, delete")
	noBtn := lipgloss.NewStyle().
		Background(colBg3).Foreground(colFg).
		Width(btnW).Align(lipgloss.Center).
		Render("N  Cancel")
	buttons := lipgloss.JoinHorizontal(lipgloss.Top,
		yesBtn,
		lipgloss.NewStyle().Background(colBg2).Width(1).Render(""),
		noBtn,
	)

	allLines := []string{heading, empty, imgLine, empty}
	allLines = append(allLines, tagLines...)
	allLines = append(allLines, empty, warn, empty, divider, empty, buttons)

	body := bg.Width(innerW).Render(lipgloss.JoinVertical(lipgloss.Left, allLines...))
	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colRed).
		Background(colBg2).
		Padding(1, 2).
		Width(dialogW).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(colBg),
	)
}

// renderConfirmDialog overlays a centred confirmation box on top of the base view.
func (m rootModel) renderConfirmDialog(base string) string {
	n := len(m.confirmItems)

	hasDir := false
	for _, e := range m.confirmItems {
		if e.isDir {
			hasDir = true
			break
		}
	}

	const dialogW = 54
	const innerW = dialogW - 6 // border(1*2) + padding(2*2)

	bg := lipgloss.NewStyle().Background(colBg2)

	// ── Header ──
	countStr := "1 item"
	if n > 1 {
		countStr = fmt.Sprintf("%d items", n)
	}
	heading := bg.Foreground(colRed).Bold(true).Width(innerW).
		Render("Delete " + countStr + "?")

	warnText := "This action cannot be undone."
	if hasDir {
		warnText = "Folders will be deleted recursively."
	}
	warn := bg.Foreground(colComment).Width(innerW).Render(warnText)

	// ── Item list (max 5 shown) ──
	const maxShow = 5
	var itemLines []string
	for i, e := range m.confirmItems {
		if i >= maxShow {
			more := bg.Foreground(colComment).Faint(true).Width(innerW).
				Render(fmt.Sprintf("  … and %d more", n-maxShow))
			itemLines = append(itemLines, more)
			break
		}
		var itemIcon string
		if e.isDir {
			itemIcon = lipgloss.NewStyle().Background(colBg2).Foreground(colYellow).Render("▸ ")
		} else {
			itemIcon = lipgloss.NewStyle().Background(colBg2).Foreground(colComment).Render("  ")
		}
		name := e.name
		maxNameW := innerW - 4
		if lipgloss.Width(name) > maxNameW {
			rr := []rune(name)
			name = string(rr[:maxNameW-1]) + "…"
		}
		line := bg.Width(innerW).Render(
			"  " + itemIcon + lipgloss.NewStyle().Background(colBg2).Foreground(colFg).Render(name),
		)
		itemLines = append(itemLines, line)
	}

	// ── Divider ──
	divider := bg.Foreground(colBg3).Width(innerW).Render(strings.Repeat("─", innerW))

	// ── Buttons ──
	btnW := (innerW - 1) / 2
	leftW := btnW + ((innerW - 1) % 2)

	yesBtn := lipgloss.NewStyle().
		Background(colRed).Foreground(lipgloss.Color("#ffffff")).
		Bold(true).Width(leftW).Align(lipgloss.Center).
		Render("Y  Yes, delete")

	noBtn := lipgloss.NewStyle().
		Background(colBg3).Foreground(colFg).
		Width(btnW).Align(lipgloss.Center).
		Render("N  Cancel")

	buttons := lipgloss.JoinHorizontal(lipgloss.Top,
		yesBtn,
		lipgloss.NewStyle().Background(colBg2).Width(1).Render(""),
		noBtn,
	)

	empty := bg.Width(innerW).Render("")

	body := bg.Width(innerW).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			heading, warn, empty,
			lipgloss.JoinVertical(lipgloss.Left, itemLines...),
			empty, divider, empty,
			buttons,
		),
	)

	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colRed).
		Background(colBg2).
		Padding(1, 2).
		Width(dialogW).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(colBg),
	)
}

// deleteEntries deletes all listed entries. Folders are expanded recursively
// using ListObjects before deletion. After completion it returns a deleteMsg
// so the browser re-fetches the current prefix.
func (m rootModel) deleteEntries(bucket, prefix string, entries []bucketEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		for _, e := range entries {
			if e.isDir {
				// Recursively list and delete everything under this prefix.
				objectsCh := m.minioClient.ListObjects(ctx, bucket, minio.ListObjectsOptions{
					Prefix:    e.fullKey,
					Recursive: true,
				})
				for obj := range objectsCh {
					if obj.Err != nil {
						return errMsg{fmt.Errorf("listing for delete %s: %w", obj.Key, obj.Err)}
					}
					if err := m.minioClient.RemoveObject(ctx, bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil {
						return errMsg{fmt.Errorf("deleting %s: %w", obj.Key, err)}
					}
				}
			} else {
				if err := m.minioClient.RemoveObject(ctx, bucket, e.fullKey, minio.RemoveObjectOptions{}); err != nil {
					return errMsg{fmt.Errorf("deleting %s: %w", e.fullKey, err)}
				}
			}
		}
		return deleteMsg{bucket: bucket, prefix: prefix}
	}
}

func (m rootModel) renderBrowserTopBar() string {
	crumb := lipgloss.NewStyle().Foreground(colComment).Render("BUCKET ")
	bucketPart := lipgloss.NewStyle().
		Foreground(colGreen).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Padding(0, 1).
		Render(m.browserBucket)

	path := ""
	if m.browserPrefix != "" {
		parts := strings.Split(strings.TrimSuffix(m.browserPrefix, "/"), "/")
		for _, p := range parts {
			path += lipgloss.NewStyle().Foreground(colComment).Render(" › ") +
				lipgloss.NewStyle().Foreground(colBlue).Render(p)
		}
	}

	// Right side: selected count (if any) + total items
	countStr := fmt.Sprintf("%d items", len(m.browserEntries))
	if len(m.browserSelected) > 0 {
		countStr = lipgloss.NewStyle().Foreground(colGreen).Render(
			fmt.Sprintf("%d selected", len(m.browserSelected)),
		) + lipgloss.NewStyle().Foreground(colComment).Render(
			fmt.Sprintf(" / %d items", len(m.browserEntries)),
		)
	} else {
		countStr = lipgloss.NewStyle().Foreground(colComment).Render(countStr)
	}

	leftPart := lipgloss.JoinHorizontal(lipgloss.Center, crumb, bucketPart, path)
	spacer := strings.Repeat(" ", max(0, m.width-lipgloss.Width(leftPart)-lipgloss.Width(countStr)-8))

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(colBorder).
		Width(m.width-4).
		Padding(0, 1).
		Render(leftPart + spacer + countStr)
}

func (m rootModel) renderBrowserStatusBar() string {
	hotkey := func(key, desc string) string {
		k := lipgloss.NewStyle().Background(colBg3).Foreground(colYellow).Bold(true).Render(" " + key + " ")
		d := lipgloss.NewStyle().Foreground(colComment).Background(colBg2).Render(" " + desc + " ")
		return k + d
	}
	keys := lipgloss.JoinHorizontal(lipgloss.Top,
		hotkey("↑↓", "Navigate"),
		hotkey("Enter", "Open"),
		hotkey("Space", "Select"),
		hotkey("A", "All"),
		hotkey("C", "New folder"),
		hotkey("U", "Upload"),
		hotkey("D", "Delete"),
		hotkey("Esc", "Back"),
		hotkey("Q", "Quit"),
	)
	count := fmt.Sprintf("%d items", len(m.browserEntries))
	status := lipgloss.NewStyle().Foreground(colComment).Background(colBg2).Render(count)
	barW := m.width - 4
	spacer := lipgloss.NewStyle().Background(colBg2).Width(max(0, barW-lipgloss.Width(keys)-lipgloss.Width(status))).Render("")
	return lipgloss.NewStyle().
		Background(colBg2).
		Width(barW).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, keys, spacer, status))
}

func (m rootModel) renderBrowserContent(totalW, height int) string {
	// ── Column widths ──
	// Row layout: prefix(2) + chk(4) + icon(2) + name(nameW) + mod(11) + size(11) + class(11) + scroll(1)
	// All of that must equal innerW = totalW-2 (panelBox adds left+right border).
	// So: nameW = (totalW-2) - 2 - 4 - 2 - 11 - 11 - 11 - 1 = totalW - 44
	chkW := 4
	iconW := 2
	modW := 11
	sizeW := 11
	classW := 11
	scrollW := 1
	nameW := totalW - 2 - 2 - chkW - iconW - modW - sizeW - classW - scrollW

	listH := max(1, height-listRowOverhead)

	scrollY := m.browserScrollY
	if m.browserCursor >= scrollY+listH {
		scrollY = m.browserCursor - listH + 1
	}
	if m.browserCursor < scrollY {
		scrollY = m.browserCursor
	}
	scrollY = max(0, scrollY)

	selectedCount := len(m.browserSelected)
	totalCount := len(m.browserEntries)

	// ── Scrollbar column (computed once, appended per row) ──
	vScrollBar := renderVScrollBar(totalCount, scrollY, listH)

	// ── Select-all checkbox state ──
	var selectAllBox string
	switch {
	case totalCount == 0 || selectedCount == 0:
		selectAllBox = lipgloss.NewStyle().Foreground(colComment).Render("[ ]")
	case selectedCount == totalCount:
		selectAllBox = lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render("[x]")
	default:
		selectAllBox = lipgloss.NewStyle().Foreground(colYellow).Bold(true).Render("[~]")
	}

	// ── Header ──
	hint := ""
	if m.browserScrollX > 0 {
		hint = fmt.Sprintf(" ◀+%d", m.browserScrollX)
	}
	hintRendered := lipgloss.NewStyle().Foreground(colComment).Faint(true).Render(hint)
	hintW := lipgloss.Width(hint)
	headerNameW := max(1, nameW-hintW)

	headerCols := lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
		padRight("NAME", headerNameW),
	) + hintRendered +
		lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
			padRight("MODIFIED", modW)+padRight("SIZE", sizeW)+padRight("CLASS", classW),
		)
	headerPrefix := "  " + selectAllBox + " " + lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(padRight("", iconW))
	// Header scrollbar cell — always plain track char
	headerSB := lipgloss.NewStyle().Foreground(colBg3).Render("│")
	header := headerPrefix + headerCols + headerSB

	// ── Rows ──
	var rows []string
	if totalCount == 0 {
		rows = append(rows, lipgloss.NewStyle().Faint(true).Render("  Empty folder."))
	}

	end := min(scrollY+listH, totalCount)
	for i := scrollY; i < end; i++ {
		e := m.browserEntries[i]
		isSelected := m.browserSelected[e.fullKey]

		chk := lipgloss.NewStyle().Foreground(colComment).Render("[ ] ")
		if isSelected {
			chk = lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render("[x] ")
		}

		var icon string
		if e.isDir {
			icon = lipgloss.NewStyle().Foreground(colYellow).Bold(true).Render("▸ ")
		} else {
			icon = lipgloss.NewStyle().Foreground(colComment).Render("· ")
		}

		name := e.name
		runes := []rune(name)
		if m.browserScrollX > 0 {
			if m.browserScrollX >= len(runes) {
				name = ""
			} else {
				name = string(runes[m.browserScrollX:])
			}
		}

		modStr, sizeStr, classStr := "", "", ""
		if !e.isDir {
			modStr = e.lastModified.Format("2006-01-02")
			sizeStr = formatBytes(e.size)
			classStr = e.storageClass
		}

		// Scrollbar char for this row
		sb := ""
		if i-scrollY < len(vScrollBar) {
			sb = vScrollBar[i-scrollY]
		}

		inner := chk + icon +
			padRight(name, nameW) +
			padRight(modStr, modW) +
			padRight(sizeStr, sizeW) +
			padRight(classStr, classW) +
			sb

		var rowStr string
		if i == m.browserCursor {
			prefix := lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render("▌ ")
			rowStr = prefix + lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(totalW-4). // innerW(totalW-2) - prefix(2)
				Render(inner)
		} else {
			rowStr = "  " + lipgloss.NewStyle().Foreground(colFg).Render(inner)
		}
		rows = append(rows, rowStr)
	}

	// Divider also needs a scrollbar-column placeholder to stay aligned
	dividerSB := lipgloss.NewStyle().Foreground(colBg3).Render("│")
	divider := strings.Repeat("─", totalW-2-scrollW) + dividerSB

	listContent := lipgloss.JoinVertical(lipgloss.Left,
		header,
		divider,
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	// No gutter — right border is always the plain panel border.
	return panelBox(m.browserBucket, totalW, height, colBlue, listContent)
}

// ─────────────────────────────────────────────
// Create / Upload cmds
// ─────────────────────────────────────────────

func (m rootModel) createBucket(name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		region := m.activeRegion
		if region == "" {
			region = "nl-ams"
		}
		if err := m.minioClient.MakeBucket(ctx, name, minio.MakeBucketOptions{
			Region: region,
		}); err != nil {
			return errMsg{fmt.Errorf("create bucket: %w", err)}
		}
		return createDoneMsg{isBucket: true}
	}
}

func (m rootModel) createFolder(bucket, prefix, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		// S3 folders are zero-byte objects whose key ends with "/".
		key := prefix + strings.TrimSuffix(name, "/") + "/"
		_, err := m.minioClient.PutObject(ctx, bucket, key,
			strings.NewReader(""), 0,
			minio.PutObjectOptions{ContentType: "application/x-directory"},
		)
		if err != nil {
			return errMsg{fmt.Errorf("create folder: %w", err)}
		}
		return createDoneMsg{bucket: bucket, prefix: prefix}
	}
}

func (m rootModel) uploadFile(bucket, prefix, localPath string) tea.Cmd {
	// The cmd fires a goroutine and returns nil immediately.
	// All progress and completion signals arrive via teaProgram.Send().
	return func() tea.Msg {
		f, err := os.Open(localPath)
		if err != nil {
			return errMsg{fmt.Errorf("open file: %w", err)}
		}

		info, err := f.Stat()
		if err != nil {
			f.Close()
			return errMsg{fmt.Errorf("stat file: %w", err)}
		}
		totalBytes := info.Size()

		base := localPath
		if idx := strings.LastIndexAny(localPath, "/\\"); idx >= 0 {
			base = localPath[idx+1:]
		}
		key := prefix + base
		mc := m.minioClient

		// Notify immediately so the overlay shows total size before reads begin.
		teaProgram.Send(uploadProgressMsg{filename: base, bytesRead: 0, total: totalBytes})

		go func() {
			defer f.Close()

			pr := &progressReader{
				r:        f,
				filename: base,
				total:    totalBytes,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			_, uploadErr := mc.PutObject(ctx, bucket, key, pr, totalBytes,
				minio.PutObjectOptions{},
			)
			if uploadErr != nil {
				teaProgram.Send(errMsg{fmt.Errorf("upload %s: %w", localPath, uploadErr)})
				return
			}
			teaProgram.Send(uploadProgressMsg{
				filename:  base,
				bytesRead: totalBytes,
				total:     totalBytes,
				done:      true,
			})
		}()

		return nil
	}
}

// progressReader wraps the file and calls teaProgram.Send() as bytes are read.
// Sends are throttled to 50ms so the render loop has time to paint each frame.
type progressReader struct {
	r        io.Reader
	filename string
	total    int64
	read     int64
	lastSent time.Time
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	if n > 0 {
		pr.read += int64(n)
		now := time.Now()
		if now.Sub(pr.lastSent) >= 50*time.Millisecond || pr.read == pr.total {
			pr.lastSent = now
			teaProgram.Send(uploadProgressMsg{
				filename:  pr.filename,
				bytesRead: pr.read,
				total:     pr.total,
			})
		}
	}
	return n, err
}

// renderUploadProgress draws a centred upload progress overlay.
func (m rootModel) renderUploadProgress(base string) string {
	const dialogW = 56
	const innerW = dialogW - 6 // border(1*2) + padding(2*2) = 6

	bg := lipgloss.NewStyle().Background(colBg2)

	pct := 0.0
	if m.upload.total > 0 {
		pct = float64(m.upload.bytesRead) / float64(m.upload.total)
		if pct > 1 {
			pct = 1
		}
	}

	// ── Title ──
	titleStr := bg.Foreground(colGreen).Bold(true).Width(innerW).Render("UPLOADING")

	// ── Filename ──
	fname := m.upload.filename
	if fname == "" {
		fname = "—"
	}
	if lipgloss.Width(fname) > innerW {
		rr := []rune(fname)
		fname = "…" + string(rr[len(rr)-(innerW-1):])
	}
	fileLabel := bg.Foreground(colFg).Width(innerW).Render(fname)

	// ── Progress bar: barW chars + 7-char label "100.0%" right-aligned ──
	const pctW = 7 // " 100.0%"
	barW := innerW - pctW - 1
	filled := int(float64(barW) * pct)
	if filled > barW {
		filled = barW
	}
	barFilled := lipgloss.NewStyle().Background(colBg2).Foreground(colGreen).Render(strings.Repeat("█", filled))
	barEmpty := lipgloss.NewStyle().Background(colBg2).Foreground(colBg3).Render(strings.Repeat("░", barW-filled))
	pctLabel := bg.Foreground(colComment).Width(pctW + 1).Align(lipgloss.Right).
		Render(fmt.Sprintf("%.1f%%", pct*100))
	barLine := lipgloss.JoinHorizontal(lipgloss.Top, barFilled, barEmpty, pctLabel)

	// ── Stats ──
	transferred := fmt.Sprintf("%s / %s", formatBytes(m.upload.bytesRead), formatBytes(m.upload.total))
	statsStr := bg.Foreground(colComment).Width(innerW).Render(transferred)

	// ── Divider ──
	divider := bg.Foreground(colBg3).Width(innerW).Render(strings.Repeat("─", innerW))

	hint := bg.Foreground(colComment).Faint(true).Width(innerW).Render("Esc not available during upload")

	body := bg.Width(innerW).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			titleStr, fileLabel, "",
			barLine, statsStr,
			"", divider, hint,
		),
	)

	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Background(colBg2).
		Padding(1, 2).
		Width(dialogW).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(colBg),
	)
}

// renderInputOverlay draws a centred input dialog on top of the base view.
func (m rootModel) renderInputOverlay(base string) string {
	dialogW := 58

	var title, subtitle, placeholder string
	switch m.input.mode {
	case inputModeBucket:
		title = "  NEW BUCKET"
		subtitle = "  Enter a name for the new bucket."
		placeholder = "my-bucket-name"
	case inputModeFolder:
		path := m.browserBucket
		if m.browserPrefix != "" {
			path += "/" + strings.TrimSuffix(m.browserPrefix, "/")
		}
		title = "  NEW FOLDER"
		subtitle = "  Creating in: " + path
		placeholder = "folder-name"
	case inputModeUpload:
		title = "  UPLOAD FILE"
		subtitle = "  Enter the full local path of the file to upload."
		placeholder = "/path/to/local/file.csv"
	}

	titleRendered := lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render(title)
	subtitleRendered := lipgloss.NewStyle().Foreground(colComment).Render(subtitle)

	// Text field: split value at cursor position, insert cursor glyph between.
	fieldW := dialogW - 6 // inner width after border + padding
	var fieldContent string
	if m.input.value == "" {
		// Show placeholder with cursor at start.
		ph := lipgloss.NewStyle().Foreground(colBg3).Render(placeholder)
		cur := lipgloss.NewStyle().Foreground(colGreen).Render("▌")
		fieldContent = cur + ph
	} else {
		runes := []rune(m.input.value)
		cur := m.input.cursor
		if cur > len(runes) {
			cur = len(runes)
		}
		before := string(runes[:cur])
		after := string(runes[cur:])

		// Scroll the visible window so the cursor is always visible.
		// fieldW accounts for 1 cell taken by the cursor glyph.
		visW := fieldW - 1
		beforeRunes := []rune(before)
		if len(beforeRunes) > visW {
			// Trim the leftmost characters so cursor stays in view.
			before = string(beforeRunes[len(beforeRunes)-visW:])
		}

		curGlyph := lipgloss.NewStyle().Foreground(colGreen).Render("▌")
		afterStyled := lipgloss.NewStyle().Foreground(colFg).Render(after)
		fieldContent = lipgloss.NewStyle().Foreground(colFg).Render(before) + curGlyph + afterStyled
	}

	field := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Padding(0, 1).
		Width(fieldW).
		Render(fieldContent)

	errLine := ""
	if m.input.errStr != "" {
		errLine = "\n" + lipgloss.NewStyle().Foreground(colRed).Render("  ✗ "+m.input.errStr)
	}

	hint := lipgloss.NewStyle().Foreground(colComment).Faint(true).
		Render("  Enter · Esc · ←→ · Home/End · Ctrl+W · Ctrl+U/K")

	body := lipgloss.JoinVertical(lipgloss.Left,
		titleRendered,
		subtitleRendered,
		"",
		"  "+field,
		errLine,
		"",
		hint,
	)

	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Background(colBg2).
		Padding(1, 2).
		Width(dialogW).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(colBg),
	)
}

// ─────────────────────────────────────────────
// main
// ─────────────────────────────────────────────

// ─────────────────────────────────────────────
// Billing — helpers
// ─────────────────────────────────────────────

// moneyToFloat converts a *scw.Money to a float64 EUR value.
func moneyToFloat(m *scw.Money) float64 {
	if m == nil {
		return 0
	}
	return float64(m.Units) + float64(m.Nanos)/1e9
}

// prevMonth returns the YYYY-MM string one month before the given one.
func prevMonth(period string) string {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return period
	}
	return t.AddDate(0, -1, 0).Format("2006-01")
}

// nextMonth returns the YYYY-MM string one month after the given one.
func nextMonth(period string) string {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return period
	}
	return t.AddDate(0, 1, 0).Format("2006-01")
}

// ─────────────────────────────────────────────
// Billing — data fetching
// ─────────────────────────────────────────────

// fetchBillingOverview fetches the last 6 months of totals plus the detail
// rows for the given period (defaults to current month if empty).
func (m rootModel) fetchBillingOverview(period string) tea.Cmd {
	return func() tea.Msg {
		api := billing.NewAPI(m.scwClient)

		if period == "" {
			period = time.Now().Format("2006-01")
		}

		// ── Last 6 months of aggregated totals ──
		months := make([]billingMonth, 0, 6)
		for i := 5; i >= 0; i-- {
			p := time.Now().AddDate(0, -i, 0).Format("2006-01")
			bm := billingMonth{
				period:     p,
				byCategory: make(map[string]float64),
			}
			var page int32 = 1
			for {
				resp, err := api.ListConsumptions(&billing.ListConsumptionsRequest{
					BillingPeriod: &p,
					Page:          scw.Int32Ptr(page),
				})
				if err != nil {
					break // billing perms may be restricted — skip silently
				}
				for _, c := range resp.Consumptions {
					v := moneyToFloat(c.Value)
					bm.totalExTax += v
					bm.byCategory[c.CategoryName] += v
				}
				if uint64(len(resp.Consumptions)) >= uint64(resp.TotalCount) {
					break
				}
				page++
			}
			months = append(months, bm)
		}

		// ── Detail rows for the selected period ──
		detail, err := fetchConsumptionDetail(api, period)
		if err != nil {
			return errMsg{fmt.Errorf("billing detail: %w", err)}
		}

		return billingOverviewMsg{months: months, detail: detail, period: period}
	}
}

// fetchConsumptionDetail returns sorted consumption rows for a given period.
func fetchConsumptionDetail(api *billing.API, period string) ([]billingConsumptionRow, error) {
	var rows []billingConsumptionRow
	var page int32 = 1
	for {
		resp, err := api.ListConsumptions(&billing.ListConsumptionsRequest{
			BillingPeriod: &period,
			Page:          scw.Int32Ptr(page),
		})
		if err != nil {
			return nil, err
		}
		for _, c := range resp.Consumptions {
			rows = append(rows, billingConsumptionRow{
				category:    c.CategoryName,
				product:     c.ProductName,
				projectName: c.ProjectID, // resolved to name if possible
				valueEUR:    moneyToFloat(c.Value),
			})
		}
		if uint64(len(resp.Consumptions)) >= uint64(resp.TotalCount) {
			break
		}
		page++
	}
	// Sort by value descending
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].valueEUR > rows[j].valueEUR
	})
	return rows, nil
}

// exportBillingCSV fetches the last N months and writes a pivot CSV to ~/scw-tui-export-YYYYMM.csv
func (m rootModel) exportBillingCSV(numMonths int) tea.Cmd {
	return func() tea.Msg {
		api := billing.NewAPI(m.scwClient)

		// Collect all periods
		periods := make([]string, numMonths)
		for i := 0; i < numMonths; i++ {
			periods[numMonths-1-i] = time.Now().AddDate(0, -i, 0).Format("2006-01")
		}

		// category → period → total
		type key struct{ category, period string }
		totals := make(map[key]float64)
		categories := make(map[string]bool)

		for _, p := range periods {
			var page int32 = 1
			period := p
			for {
				resp, err := api.ListConsumptions(&billing.ListConsumptionsRequest{
					BillingPeriod: &period,
					Page:          scw.Int32Ptr(page),
				})
				if err != nil {
					break
				}
				for _, c := range resp.Consumptions {
					k := key{c.CategoryName, period}
					totals[k] += moneyToFloat(c.Value)
					categories[c.CategoryName] = true
				}
				if uint64(len(resp.Consumptions)) >= uint64(resp.TotalCount) {
					break
				}
				page++
			}
		}

		// Sort categories
		cats := make([]string, 0, len(categories))
		for c := range categories {
			cats = append(cats, c)
		}
		sort.Strings(cats)

		// Build CSV
		home, err := os.UserHomeDir()
		if err != nil {
			return errMsg{fmt.Errorf("find home dir: %w", err)}
		}
		fname := fmt.Sprintf("scw-tui-export-%s.csv", time.Now().Format("200601"))
		path := filepath.Join(home, fname)
		f, err := os.Create(path)
		if err != nil {
			return errMsg{fmt.Errorf("create csv: %w", err)}
		}
		defer f.Close()

		w := csv.NewWriter(f)

		// Header: Category, Jan-2025, Feb-2025, ...
		header := []string{"Category"}
		for _, p := range periods {
			t, err := time.Parse("2006-01", p)
			if err != nil {
				header = append(header, p)
			} else {
				header = append(header, t.Format("Jan 2006"))
			}
		}
		header = append(header, "Total")
		if err := w.Write(header); err != nil {
			return errMsg{fmt.Errorf("write header: %w", err)}
		}

		// Rows
		for _, cat := range cats {
			row := []string{cat}
			var total float64
			for _, p := range periods {
				v := totals[key{cat, p}]
				total += v
				row = append(row, fmt.Sprintf("%.2f", v))
			}
			row = append(row, fmt.Sprintf("%.2f", total))
			if err := w.Write(row); err != nil {
				return errMsg{fmt.Errorf("write row: %w", err)}
			}
		}

		// Totals row
		totRow := []string{"TOTAL"}
		var grandTotal float64
		for _, p := range periods {
			var sum float64
			for _, cat := range cats {
				sum += totals[key{cat, p}]
			}
			grandTotal += sum
			totRow = append(totRow, fmt.Sprintf("%.2f", sum))
		}
		totRow = append(totRow, fmt.Sprintf("%.2f", grandTotal))
		if err := w.Write(totRow); err != nil {
			return errMsg{fmt.Errorf("write totals row: %w", err)}
		}

		w.Flush()
		if err := w.Error(); err != nil {
			return errMsg{fmt.Errorf("flush csv: %w", err)}
		}
		return billingExportDoneMsg{path: path}
	}
}

// ─────────────────────────────────────────────
// Billing — views
// ─────────────────────────────────────────────

// renderBillingPreview is shown in the dashboard content area when Billing
// service is selected. It shows a "Press Enter to open" prompt with last total.
func (m rootModel) renderBillingPreview(totalW, height int, borderColor lipgloss.Color) string {
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(colComment).Render("Press Enter to open billing details"))
	if len(m.billingMonths) > 0 {
		last := m.billingMonths[len(m.billingMonths)-1]
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(colComment).Render("Last period: ")+
			lipgloss.NewStyle().Foreground(colFg).Render(last.period))
		lines = append(lines, lipgloss.NewStyle().Foreground(colComment).Render("Total excl. tax: ")+
			lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render(fmt.Sprintf("€%.2f", last.totalExTax)))
	}
	return panelBox("BILLING", totalW, height, borderColor,
		lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// drawBilling renders the full-screen billing view.
func (m rootModel) drawBilling() string {
	topBar := m.renderBillingTopBar()
	statusBar := m.renderBillingStatusBar()

	if m.loading {
		inner := lipgloss.Place(
			m.width-4, m.height-topBarHeight-statusBarHeight-4,
			lipgloss.Center, lipgloss.Center,
			m.spin.View()+" Loading billing data…",
		)
		return lipgloss.NewStyle().Margin(1, 2).Render(
			lipgloss.JoinVertical(lipgloss.Left, topBar, inner, statusBar),
		)
	}

	contentH := m.height - topBarHeight - statusBarHeight - 6
	contentW := m.width - 8

	// Split: chart on left (60%), detail table on right (40%)
	chartW := (contentW * 6) / 10
	tableW := contentW - chartW - 1

	chart := m.renderBillingChart(chartW, contentH)
	table := m.renderBillingDetail(tableW, contentH)

	base := lipgloss.NewStyle().Margin(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			topBar,
			lipgloss.JoinHorizontal(lipgloss.Top, chart, table),
			statusBar,
		),
	)
	return base
}

// renderBillingTopBar shows the current period and total.
func (m rootModel) renderBillingTopBar() string {
	left := lipgloss.NewStyle().Foreground(colComment).Render("BILLING ") +
		lipgloss.NewStyle().Foreground(colPurple).Border(lipgloss.RoundedBorder()).
			BorderForeground(colPurple).Padding(0, 1).Render(" "+m.billingPeriod+" ")

	// Find current period total
	total := 0.0
	for _, bm := range m.billingMonths {
		if bm.period == m.billingPeriod {
			total = bm.totalExTax
			break
		}
	}
	totalStr := lipgloss.NewStyle().Foreground(colComment).Render("  Total excl. tax: ") +
		lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render(fmt.Sprintf(" €%.2f ", total))

	exportMsg := ""
	if m.billingExportMsg != "" {
		exportMsg = "  " + lipgloss.NewStyle().Foreground(colGreen).Render(" ✓ "+m.billingExportMsg+" ")
	}

	clock := lipgloss.NewStyle().Foreground(colComment).Render(" " + time.Now().Format("15:04") + " ")
	leftPart := lipgloss.JoinHorizontal(lipgloss.Center, left, totalStr, exportMsg)
	spacer := strings.Repeat(" ", max(0, m.width-lipgloss.Width(leftPart)-lipgloss.Width(clock)-8))

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(colBorder).
		Width(m.width-4).
		Padding(0, 1).
		Render(leftPart + spacer + clock)
}

func (m rootModel) renderBillingStatusBar() string {
	hotkey := func(key, desc string) string {
		k := lipgloss.NewStyle().Background(colBg3).Foreground(colYellow).Bold(true).Render(" " + key + " ")
		d := lipgloss.NewStyle().Foreground(colComment).Background(colBg2).Render(" " + desc + " ")
		return k + d
	}
	keys := lipgloss.JoinHorizontal(lipgloss.Top,
		hotkey("←→", "Month"),
		hotkey("↑↓", "Navigate"),
		hotkey("E", "Export CSV"),
		hotkey("F5", "Refresh"),
		hotkey("Esc", "Back"),
		hotkey("Q", "Quit"),
	)
	barW := m.width - 4
	spacer := lipgloss.NewStyle().Background(colBg2).Width(max(0, barW-lipgloss.Width(keys))).Render("")
	return lipgloss.NewStyle().
		Background(colBg2).
		Width(barW).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, keys, spacer))
}

// renderBillingChart draws an ASCII bar chart of the last N months.
func (m rootModel) renderBillingChart(w, h int) string {
	innerW := w - 2 // inside panel borders

	if len(m.billingMonths) == 0 {
		return panelBox("6-MONTH OVERVIEW", w, h, colPurple,
			lipgloss.NewStyle().Faint(true).Render("No data"))
	}

	// Find max for scaling
	maxVal := 0.0
	for _, bm := range m.billingMonths {
		if bm.totalExTax > maxVal {
			maxVal = bm.totalExTax
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	// Chart area height: innerH - header(1) - divider(1) - x-axis(1) - labels(1)
	chartH := max(4, h-listRowOverhead-2)
	barAreaW := innerW - 8 // leave 8 cols for Y-axis labels
	barW := max(1, barAreaW/len(m.billingMonths))

	// Build chart lines top→bottom
	lines := make([]string, chartH)
	for row := 0; row < chartH; row++ {
		threshold := maxVal * float64(chartH-row) / float64(chartH)
		line := lipgloss.NewStyle().Foreground(colComment).Render(
			fmt.Sprintf("%6s ", formatEuroShort(threshold)),
		)
		for _, bm := range m.billingMonths {
			filled := bm.totalExTax >= threshold
			barColor := colPurple
			if bm.period == m.billingPeriod {
				barColor = colGreen
			}
			cell := strings.Repeat(" ", barW)
			if filled {
				cell = lipgloss.NewStyle().Foreground(barColor).Render(strings.Repeat("█", barW))
			}
			line += cell
		}
		lines[row] = line
	}

	// X-axis labels
	xAxis := strings.Repeat(" ", 7)
	for _, bm := range m.billingMonths {
		label := bm.period
		t, err := time.Parse("2006-01", bm.period)
		if err == nil {
			label = t.Format("Jan")
		}
		xAxis += padRight(label, barW)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		append(lines, strings.Repeat("─", innerW), xAxis)...,
	)
	return panelBox("6-MONTH OVERVIEW", w, h, colPurple, content)
}

// renderBillingDetail shows the consumption table for the current period.
func (m rootModel) renderBillingDetail(w, h int) string {
	catW := 14
	prodW := max(1, w-catW-12-2) // remainder for product name
	valW := 10

	listH := max(1, h-listRowOverhead)

	// Scroll viewport
	scrollY := m.billingScrollY
	if m.billingCursor >= scrollY+listH {
		scrollY = m.billingCursor - listH + 1
	}
	if m.billingCursor < scrollY {
		scrollY = m.billingCursor
	}
	scrollY = max(0, scrollY)

	header := lipgloss.NewStyle().Foreground(colComment).Bold(true).Render(
		padRight("CATEGORY", catW) +
			padRight("PRODUCT", prodW) +
			padRight("COST (€)", valW),
	)

	var rows []string
	if len(m.billingDetail) == 0 {
		rows = append(rows, lipgloss.NewStyle().Faint(true).Render("  No data for this period."))
	}

	end := min(scrollY+listH, len(m.billingDetail))
	for i := scrollY; i < end; i++ {
		r := m.billingDetail[i]
		cost := fmt.Sprintf("%.2f", r.valueEUR)
		rowStr := padRight(r.category, catW) + padRight(r.product, prodW) + padRight(cost, valW)
		if i == m.billingCursor {
			rowStr = lipgloss.NewStyle().
				Background(colBg3).Foreground(colFg).Bold(true).
				Width(w - 2).Render("▌ " + rowStr)
		} else {
			rowStr = lipgloss.NewStyle().Foreground(colFg).Width(w - 2).Render("  " + rowStr)
		}
		rows = append(rows, rowStr)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		strings.Repeat("─", w-2),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
	return panelBox(m.billingPeriod, w, h, colGreen, content)
}

// formatEuroShort formats a float as a short euro value (e.g. "€12K", "€1.2K", "€345").
func formatEuroShort(v float64) string {
	switch {
	case v >= 10000:
		return fmt.Sprintf("€%.0fK", v/1000)
	case v >= 1000:
		return fmt.Sprintf("€%.1fK", v/1000)
	default:
		return fmt.Sprintf("€%.0f", v)
	}
}

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
