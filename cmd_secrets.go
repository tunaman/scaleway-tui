package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scaleway/scaleway-sdk-go/api/secret/v1beta1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// ─────────────────────────────────────────────
// Secrets commands
// ─────────────────────────────────────────────

func (m rootModel) fetchSecretVersions(s secretItem) tea.Cmd {
	return func() tea.Msg {
		api := secret.NewAPI(m.scwClient)
		region := scw.Region(m.activeRegion)
		if region == "" {
			region = scw.RegionNlAms
		}
		req := &secret.ListSecretVersionsRequest{
			Region:   region,
			SecretID: s.id,
		}
		var versions []secretVersion
		var page int32 = 1
		for {
			resp, err := api.ListSecretVersions(req)
			if err != nil {
				return errMsg{fmt.Errorf("list secret versions: %w", err)}
			}
			for _, v := range resp.Versions {
				sv := secretVersion{
					revision: v.Revision,
					status:   string(v.Status),
					latest:   v.Latest,
				}
				if v.Description != nil {
					sv.description = *v.Description
				}
				if v.CreatedAt != nil {
					sv.createdAt = *v.CreatedAt
				}
				if v.UpdatedAt != nil {
					sv.updatedAt = *v.UpdatedAt
				}
				versions = append(versions, sv)
			}
			if uint64(len(versions)) >= resp.TotalCount {
				break
			}
			page++
			req.Page = scw.Int32Ptr(page)
		}
		return secretVersionsMsg{secret: s, versions: versions}
	}
}

func (m rootModel) accessSecretVersion(secretID string, revision uint32) tea.Cmd {
	return func() tea.Msg {
		api := secret.NewAPI(m.scwClient)
		region := scw.Region(m.activeRegion)
		if region == "" {
			region = scw.RegionNlAms
		}
		rev := fmt.Sprintf("%d", revision)
		resp, err := api.AccessSecretVersion(&secret.AccessSecretVersionRequest{
			Region:   region,
			SecretID: secretID,
			Revision: rev,
		})
		if err != nil {
			return errMsg{fmt.Errorf("access secret version: %w", err)}
		}
		return secretVersionContentMsg{revision: revision, content: string(resp.Data)}
	}
}

func (m rootModel) createSecretVersion(secretID, data string) tea.Cmd {
	return func() tea.Msg {
		api := secret.NewAPI(m.scwClient)
		region := scw.Region(m.activeRegion)
		if region == "" {
			region = scw.RegionNlAms
		}
		_, err := api.CreateSecretVersion(&secret.CreateSecretVersionRequest{
			Region:   region,
			SecretID: secretID,
			Data:     []byte(data),
		})
		if err != nil {
			return errMsg{fmt.Errorf("create secret version: %w", err)}
		}
		return secretVersionCreatedMsg{}
	}
}

func (m rootModel) deleteSecret(id string) tea.Cmd {
	return func() tea.Msg {
		api := secret.NewAPI(m.scwClient)
		region := scw.Region(m.activeRegion)
		if region == "" {
			region = scw.RegionNlAms
		}
		err := api.DeleteSecret(&secret.DeleteSecretRequest{
			Region:   region,
			SecretID: id,
		})
		if err != nil {
			return errMsg{fmt.Errorf("delete secret: %w", err)}
		}
		return secretDeletedMsg{}
	}
}

func (m rootModel) createSecret(name string) tea.Cmd {
	return func() tea.Msg {
		api := secret.NewAPI(m.scwClient)
		region := scw.Region(m.activeRegion)
		if region == "" {
			region = scw.RegionNlAms
		}
		projectID := m.projectID
		_, err := api.CreateSecret(&secret.CreateSecretRequest{
			Region:    region,
			ProjectID: projectID,
			Name:      name,
		})
		if err != nil {
			return errMsg{fmt.Errorf("create secret: %w", err)}
		}
		return secretCreatedMsg{}
	}
}

func (m rootModel) updateSecretVersionDesc(secretID string, revision uint32, desc string) tea.Cmd {
	return func() tea.Msg {
		api := secret.NewAPI(m.scwClient)
		region := scw.Region(m.activeRegion)
		if region == "" {
			region = scw.RegionNlAms
		}
		rev := fmt.Sprintf("%d", revision)
		_, err := api.UpdateSecretVersion(&secret.UpdateSecretVersionRequest{
			Region:      region,
			SecretID:    secretID,
			Revision:    rev,
			Description: &desc,
		})
		if err != nil {
			return errMsg{fmt.Errorf("update secret version: %w", err)}
		}
		return secretVersionUpdatedMsg{}
	}
}
