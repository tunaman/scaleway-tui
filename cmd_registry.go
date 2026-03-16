package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scaleway/scaleway-sdk-go/api/registry/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// ─────────────────────────────────────────────
// Registry commands
// ─────────────────────────────────────────────

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
