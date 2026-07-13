package main

import (
	"fmt"
	"sort"

	tea "charm.land/bubbletea/v2"
	iam "github.com/scaleway/scaleway-sdk-go/api/iam/v1alpha1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// ─────────────────────────────────────────────
// IAM — data fetching (organization-scoped, read-only)
// ─────────────────────────────────────────────

// iamLogsCap bounds how many (most-recent) audit-log rows we load — the Logs
// resource is effectively unbounded.
const iamLogsCap = 200

// fetchIAM loads all six IAM lists (Users, Applications, Groups, Policies,
// API keys, Logs) for the active organization in one command and returns them
// as a single iamDataMsg. Everything after is client-side (tab switch + filter).
func (m rootModel) fetchIAM() tea.Cmd {
	orgID := m.organizationID
	return func() tea.Msg {
		api := iam.NewAPI(m.scwClient)

		// ── Users ──
		var users []iamUser
		{
			var page int32 = 1
			for {
				resp, err := api.ListUsers(&iam.ListUsersRequest{
					OrganizationID: &orgID,
					Page:           scw.Int32Ptr(page),
					PageSize:       scw.Uint32Ptr(100),
				})
				if err != nil {
					return errMsg{fmt.Errorf("iam list users: %w", err)}
				}
				for _, u := range resp.Users {
					iu := iamUser{
						id:       u.ID,
						email:    u.Email,
						userType: string(u.Type),
						mfa:      u.Mfa,
						tags:     u.Tags,
					}
					// Status is deprecated in the SDK but remains the only field
					// carrying the console's activated/invitation-pending state.
					if u.Status != nil { //nolint:staticcheck // see comment above
						iu.status = string(*u.Status) //nolint:staticcheck // see comment above
					}
					if u.LastLoginAt != nil {
						iu.lastLoginAt = *u.LastLoginAt
					}
					users = append(users, iu)
				}
				if uint64(len(users)) >= uint64(resp.TotalCount) || len(resp.Users) == 0 {
					break
				}
				page++
			}
		}

		// ── Applications ──
		var apps []iamApplication
		{
			var page int32 = 1
			for {
				resp, err := api.ListApplications(&iam.ListApplicationsRequest{
					OrganizationID: orgID,
					Page:           scw.Int32Ptr(page),
					PageSize:       scw.Uint32Ptr(100),
				})
				if err != nil {
					return errMsg{fmt.Errorf("iam list applications: %w", err)}
				}
				for _, a := range resp.Applications {
					apps = append(apps, iamApplication{
						id:          a.ID,
						name:        a.Name,
						description: a.Description,
						nbAPIKeys:   a.NbAPIKeys,
						tags:        a.Tags,
					})
				}
				if uint64(len(apps)) >= uint64(resp.TotalCount) || len(resp.Applications) == 0 {
					break
				}
				page++
			}
		}

		// ── Groups ──
		var groups []iamGroup
		{
			var page int32 = 1
			for {
				resp, err := api.ListGroups(&iam.ListGroupsRequest{
					OrganizationID: orgID,
					Page:           scw.Int32Ptr(page),
					PageSize:       scw.Uint32Ptr(100),
				})
				if err != nil {
					return errMsg{fmt.Errorf("iam list groups: %w", err)}
				}
				for _, g := range resp.Groups {
					groups = append(groups, iamGroup{
						id:          g.ID,
						name:        g.Name,
						description: g.Description,
						nbUsers:     len(g.UserIDs),
						nbApps:      len(g.ApplicationIDs),
						tags:        g.Tags,
					})
				}
				if uint64(len(groups)) >= uint64(resp.TotalCount) || len(resp.Groups) == 0 {
					break
				}
				page++
			}
		}

		// ── Policies ──
		var policies []iamPolicy
		{
			var page int32 = 1
			for {
				resp, err := api.ListPolicies(&iam.ListPoliciesRequest{
					OrganizationID: orgID,
					Page:           scw.Int32Ptr(page),
					PageSize:       scw.Uint32Ptr(100),
				})
				if err != nil {
					return errMsg{fmt.Errorf("iam list policies: %w", err)}
				}
				for _, p := range resp.Policies {
					ip := iamPolicy{
						id:            p.ID,
						name:          p.Name,
						nbRules:       p.NbRules,
						principalKind: "—",
						tags:          p.Tags,
					}
					switch {
					case p.UserID != nil:
						ip.principalKind, ip.principalID = "User", *p.UserID
					case p.GroupID != nil:
						ip.principalKind, ip.principalID = "Group", *p.GroupID
					case p.ApplicationID != nil:
						ip.principalKind, ip.principalID = "Application", *p.ApplicationID
					}
					policies = append(policies, ip)
				}
				if uint64(len(policies)) >= uint64(resp.TotalCount) || len(resp.Policies) == 0 {
					break
				}
				page++
			}
		}

		// ── API keys ──
		var apiKeys []iamAPIKey
		{
			var page int32 = 1
			for {
				resp, err := api.ListAPIKeys(&iam.ListAPIKeysRequest{
					OrganizationID: &orgID,
					Page:           scw.Int32Ptr(page),
					PageSize:       scw.Uint32Ptr(100),
				})
				if err != nil {
					return errMsg{fmt.Errorf("iam list api keys: %w", err)}
				}
				for _, k := range resp.APIKeys {
					ik := iamAPIKey{
						accessKey:   k.AccessKey,
						description: k.Description,
						bearerKind:  "—",
					}
					switch {
					case k.UserID != nil:
						ik.bearerKind, ik.bearerID = "User", *k.UserID
					case k.ApplicationID != nil:
						ik.bearerKind, ik.bearerID = "Application", *k.ApplicationID
					}
					if k.CreatedAt != nil {
						ik.createdAt = *k.CreatedAt
					}
					if k.ExpiresAt != nil {
						ik.expiresAt = *k.ExpiresAt
					}
					apiKeys = append(apiKeys, ik)
				}
				if uint64(len(apiKeys)) >= uint64(resp.TotalCount) || len(resp.APIKeys) == 0 {
					break
				}
				page++
			}
		}

		// ── Logs (most recent, capped) ──
		var logs []iamLog
		{
			var page int32 = 1
			for {
				resp, err := api.ListLogs(&iam.ListLogsRequest{
					OrganizationID: orgID,
					Page:           scw.Int32Ptr(page),
					PageSize:       scw.Uint32Ptr(100),
				})
				if err != nil {
					return errMsg{fmt.Errorf("iam list logs: %w", err)}
				}
				for _, l := range resp.Logs {
					il := iamLog{
						id:           l.ID,
						action:       string(l.Action),
						resourceType: string(l.ResourceType),
						resourceID:   l.ResourceID,
						bearerID:     l.BearerID,
					}
					if l.CreatedAt != nil {
						il.createdAt = *l.CreatedAt
					}
					logs = append(logs, il)
				}
				if len(logs) >= iamLogsCap || uint64(len(logs)) >= resp.TotalCount || len(resp.Logs) == 0 {
					break
				}
				page++
			}
			if len(logs) > iamLogsCap {
				logs = logs[:iamLogsCap]
			}
			// Most recent first.
			sort.Slice(logs, func(i, j int) bool { return logs[i].createdAt.After(logs[j].createdAt) })
		}

		return iamDataMsg{
			users:        users,
			applications: apps,
			groups:       groups,
			policies:     policies,
			apiKeys:      apiKeys,
			logs:         logs,
		}
	}
}
