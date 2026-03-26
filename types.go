package main

import (
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

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
	id      string
	name    string
	status  string
	version string
	region  string
}

type nodePool struct {
	id             string
	name           string
	status         string
	nodeType       string
	size           uint32
	minSize        uint32
	maxSize        uint32
	version        string
	autoscaling    bool
	autohealing    bool
	zone           string
	rootVolumeType string
	rootVolumeSize uint64 // bytes
}

type k8sNode struct {
	id         string
	name       string
	status     string
	publicIPv4 string
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

type secretItem struct {
	id           string
	name         string
	status       string
	versionCount uint32
	description  string
	updatedAt    time.Time
	createdAt    time.Time
}

type secretVersion struct {
	revision    uint32
	status      string
	description string
	latest      bool
	createdAt   time.Time
	updatedAt   time.Time
}

// ─────────────────────────────────────────────
// Tea messages
// ─────────────────────────────────────────────

type dataMsg struct {
	buckets            []bucket
	clusters           []cluster
	projects           []projectItem
	registryNamespaces []registryNamespace
	secrets            []secretItem
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
	inputModeSecretNewVersion
	inputModeSecretUpdateDesc
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

type secretVersionsMsg struct {
	secret   secretItem
	versions []secretVersion
}

type secretVersionContentMsg struct {
	revision uint32
	content  string
}

type secretVersionCreatedMsg struct{}
type secretVersionUpdatedMsg struct{}

type k8sNodePoolsMsg struct {
	cluster   cluster
	nodePools []nodePool
}

type k8sNodesMsg struct {
	nodePoolID string
	nodes      []k8sNode
}

type k8sNodeRebootedMsg struct{}
type k8sNodePollTickMsg struct{}

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
	category string
	product  string
	valueEUR float64
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
	scwClient             *scw.Client
	minioClient           *minio.Client
	profileName           string
	region                string
	defaultProjectID      string // read directly from the profile, no API call needed
	defaultOrganizationID string // read directly from the profile, used for billing API
}
