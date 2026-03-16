package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/minio/minio-go/v7"
	"github.com/scaleway/scaleway-sdk-go/api/account/v3"
	"github.com/scaleway/scaleway-sdk-go/api/k8s/v1"
	"github.com/scaleway/scaleway-sdk-go/api/registry/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

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
// Object browser — fetch
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

// ─────────────────────────────────────────────
// Create / Upload / Delete cmds
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
			_ = f.Close()
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
			defer func() { _ = f.Close() }()

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
