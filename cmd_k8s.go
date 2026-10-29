package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/api/k8s/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// fetchNodePools fetches all node pools for the given cluster.
func (m rootModel) fetchNodePools(cl cluster) tea.Cmd {
	return func() tea.Msg {
		k8sAPI := k8s.NewAPI(m.scwClient)
		region := scw.Region(cl.region)
		var pools []nodePool
		var page int32 = 1
		req := &k8s.ListPoolsRequest{Region: region, ClusterID: cl.id}
		for {
			resp, err := k8sAPI.ListPools(req)
			if err != nil {
				return errMsg{fmt.Errorf("list pools: %w", err)}
			}
			for _, p := range resp.Pools {
				volSize := uint64(0)
				if p.RootVolumeSize != nil {
					volSize = uint64(*p.RootVolumeSize)
				}
				pools = append(pools, nodePool{
					id:             p.ID,
					name:           p.Name,
					status:         string(p.Status),
					nodeType:       p.NodeType,
					size:           p.Size,
					minSize:        p.MinSize,
					maxSize:        p.MaxSize,
					version:        p.Version,
					autoscaling:    p.Autoscaling,
					autohealing:    p.Autohealing,
					zone:           string(p.Zone),
					rootVolumeType: string(p.RootVolumeType),
					rootVolumeSize: volSize,
				})
			}
			if uint64(len(pools)) >= resp.TotalCount {
				break
			}
			page++
			req.Page = scw.Int32Ptr(page)
		}

		// Annotate each pool with the stock availability of its node type.
		// Best effort: a failed lookup leaves availability empty rather than
		// blocking the pool list.
		byZone := map[string]map[string]string{}
		for i := range pools {
			zone := pools[i].zone
			if _, ok := byZone[zone]; !ok {
				byZone[zone] = fetchServerTypesAvailability(m.scwClient, zone)
			}
			pools[i].availability = byZone[zone][poolNodeTypeToServerType(pools[i].nodeType)]
		}
		return k8sNodePoolsMsg{cluster: cl, nodePools: pools}
	}
}

// fetchServerTypesAvailability returns server type → availability for a zone,
// or nil if the lookup fails.
func fetchServerTypesAvailability(client *scw.Client, zone string) map[string]string {
	instanceAPI := instance.NewAPI(client)
	resp, err := instanceAPI.GetServerTypesAvailability(&instance.GetServerTypesAvailabilityRequest{
		Zone: scw.Zone(zone),
	}, scw.WithAllPages())
	if err != nil {
		return nil
	}
	out := make(map[string]string, len(resp.Servers))
	for name, a := range resp.Servers {
		if a != nil {
			out[name] = string(a.Availability)
		}
	}
	return out
}

// poolNodeTypeToServerType maps a K8s node type ("pop2_8c_32g") to the
// Instance server type name ("POP2-8C-32G").
func poolNodeTypeToServerType(nodeType string) string {
	return strings.ToUpper(strings.ReplaceAll(nodeType, "_", "-"))
}

// fetchNodes fetches all nodes for the given cluster, optionally filtered by poolID.
func (m rootModel) fetchNodes(cl cluster, poolID string) tea.Cmd {
	return func() tea.Msg {
		k8sAPI := k8s.NewAPI(m.scwClient)
		region := scw.Region(cl.region)
		var nodes []k8sNode
		var page int32 = 1
		req := &k8s.ListNodesRequest{Region: region, ClusterID: cl.id}
		if poolID != "" {
			req.PoolID = &poolID
		}
		for {
			resp, err := k8sAPI.ListNodes(req)
			if err != nil {
				return errMsg{fmt.Errorf("list nodes: %w", err)}
			}
			for _, n := range resp.Nodes {
				ipStr := ""
				if n.PublicIPV4 != nil && len(*n.PublicIPV4) > 0 { //nolint:staticcheck
					ipStr = n.PublicIPV4.String() //nolint:staticcheck
				}
				nodes = append(nodes, k8sNode{
					id:         n.ID,
					name:       n.Name,
					status:     string(n.Status),
					publicIPv4: ipStr,
				})
			}
			if uint64(len(nodes)) >= resp.TotalCount {
				break
			}
			page++
			req.Page = scw.Int32Ptr(page)
		}
		return k8sNodesMsg{nodePoolID: poolID, nodes: nodes}
	}
}

// pollNodesAfterDelay waits 3 seconds then fires a k8sNodePollTickMsg,
// used to refresh node status while any node is in "rebooting" state.
func pollNodesAfterDelay() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(3 * time.Second)
		return k8sNodePollTickMsg{}
	}
}

// replaceNode drains a node and provisions a fresh replacement.
func (m rootModel) replaceNode(nodeID, region string) tea.Cmd {
	return func() tea.Msg {
		k8sAPI := k8s.NewAPI(m.scwClient)
		_, err := k8sAPI.ReplaceNode(&k8s.ReplaceNodeRequest{
			Region: scw.Region(region),
			NodeID: nodeID,
		})
		if err != nil {
			return errMsg{fmt.Errorf("replace node: %w", err)}
		}
		return k8sNodeReplacedMsg{}
	}
}

// rebootNode sends a reboot request for a single node.
func (m rootModel) rebootNode(nodeID, region string) tea.Cmd {
	return func() tea.Msg {
		k8sAPI := k8s.NewAPI(m.scwClient)
		_, err := k8sAPI.RebootNode(&k8s.RebootNodeRequest{
			Region: scw.Region(region),
			NodeID: nodeID,
		})
		if err != nil {
			return errMsg{fmt.Errorf("reboot node: %w", err)}
		}
		return k8sNodeRebootedMsg{}
	}
}
