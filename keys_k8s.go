package main

import (
	tea "charm.land/bubbletea/v2"
)

// ─────────────────────────────────────────────
// Key handling — Kubernetes
// ─────────────────────────────────────────────

// clusterListUp / clusterListDown move the cluster cursor on the dashboard.
func (m rootModel) clusterListUp() (rootModel, tea.Cmd) {
	if len(m.clusters) > 0 {
		m.clusterCursor = (m.clusterCursor - 1 + len(m.clusters)) % len(m.clusters)
	}
	return m, nil
}

func (m rootModel) clusterListDown() (rootModel, tea.Cmd) {
	if len(m.clusters) > 0 {
		m.clusterCursor = (m.clusterCursor + 1) % len(m.clusters)
	}
	return m, nil
}

// clusterEnterKey opens the selected cluster (fetches its node pools).
func (m rootModel) clusterEnterKey() (rootModel, tea.Cmd) {
	if len(m.clusters) == 0 {
		return m, nil
	}
	cl := m.clusters[m.clusterCursor]
	m.loading = true
	return m, tea.Batch(m.spin.Tick, m.fetchNodePools(cl))
}

// k8sKubeConfigKey fetches the cluster kubeconfig and saves it to
// ~/.kube/scw-<cluster>.yaml (top bar flashes the saved path).
func (m rootModel) k8sKubeConfigKey() (rootModel, tea.Cmd) {
	if m.state == stateK8sBrowser && !m.loading && !m.k8sConfirmReboot && !m.k8sConfirmReplace {
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.fetchKubeConfig(m.k8sBrowserCluster))
	}
	return m, nil
}

// k8sTabKey switches between the node pools and nodes panes.
func (m rootModel) k8sTabKey() (rootModel, tea.Cmd) {
	if m.k8sBrowserFocus == 0 && len(m.k8sBrowserNodePools) > 0 {
		m.k8sBrowserFocus = 1
		m.k8sBrowserNodeCursor = 0
		m.k8sBrowserNodeScrollY = 0
	} else {
		m.k8sBrowserFocus = 0
	}
	return m, nil
}

// k8sRebootKey / k8sReplaceKey open the matching confirmation dialog for
// the selected node.
func (m rootModel) k8sRebootKey() (rootModel, tea.Cmd) {
	if len(m.k8sBrowserNodes) > 0 && m.k8sBrowserNodeCursor < len(m.k8sBrowserNodes) {
		n := m.k8sBrowserNodes[m.k8sBrowserNodeCursor]
		m.k8sRebootNodeID = n.id
		m.k8sRebootNodeName = n.name
		m.k8sConfirmReboot = true
	}
	return m, nil
}

func (m rootModel) k8sReplaceKey() (rootModel, tea.Cmd) {
	if len(m.k8sBrowserNodes) > 0 && m.k8sBrowserNodeCursor < len(m.k8sBrowserNodes) {
		n := m.k8sBrowserNodes[m.k8sBrowserNodeCursor]
		m.k8sReplaceNodeID = n.id
		m.k8sReplaceNodeName = n.name
		m.k8sConfirmReplace = true
	}
	return m, nil
}

// k8sRebootConfirmYes / k8sReplaceConfirmYes execute the confirmed node action.
func (m rootModel) k8sRebootConfirmYes() (rootModel, tea.Cmd) {
	m.k8sConfirmReboot = false
	m.loading = true
	return m, tea.Batch(m.spin.Tick, m.rebootNode(m.k8sRebootNodeID, m.k8sBrowserCluster.region))
}

func (m rootModel) k8sReplaceConfirmYes() (rootModel, tea.Cmd) {
	m.k8sConfirmReplace = false
	m.loading = true
	return m, tea.Batch(m.spin.Tick, m.replaceNode(m.k8sReplaceNodeID, m.k8sBrowserCluster.region))
}

// k8sCopyNodeIPKey copies the selected node's public IP from the nodes pane.
func (m rootModel) k8sCopyNodeIPKey() (rootModel, tea.Cmd) {
	if len(m.k8sBrowserNodes) > 0 && m.k8sBrowserNodeCursor < len(m.k8sBrowserNodes) {
		if ip := m.k8sBrowserNodes[m.k8sBrowserNodeCursor].publicIPv4; ip != "" {
			return m.copyToClipboard(ip)
		}
	}
	return m, nil
}

// k8sUp / k8sDown move the cursor in the active pane; moving between pools
// refetches that pool's nodes.
func (m rootModel) k8sUp() (rootModel, tea.Cmd) {
	if m.k8sBrowserFocus == 0 {
		if m.k8sBrowserPoolCursor > 0 {
			m.k8sBrowserPoolCursor--
			if m.k8sBrowserPoolCursor < m.k8sBrowserPoolScrollY {
				m.k8sBrowserPoolScrollY = m.k8sBrowserPoolCursor
			}
			m.k8sNodesLoading = true
			return m, m.fetchNodes(m.k8sBrowserCluster, m.k8sBrowserNodePools[m.k8sBrowserPoolCursor].id)
		}
	} else {
		if m.k8sBrowserNodeCursor > 0 {
			m.k8sBrowserNodeCursor--
			if m.k8sBrowserNodeCursor < m.k8sBrowserNodeScrollY {
				m.k8sBrowserNodeScrollY = m.k8sBrowserNodeCursor
			}
		}
	}
	return m, nil
}

func (m rootModel) k8sDown() (rootModel, tea.Cmd) {
	if m.k8sBrowserFocus == 0 {
		if m.k8sBrowserPoolCursor < len(m.k8sBrowserNodePools)-1 {
			m.k8sBrowserPoolCursor++
			if m.k8sBrowserPoolCursor >= m.k8sBrowserPoolScrollY+20 {
				m.k8sBrowserPoolScrollY++
			}
			m.k8sNodesLoading = true
			return m, m.fetchNodes(m.k8sBrowserCluster, m.k8sBrowserNodePools[m.k8sBrowserPoolCursor].id)
		}
	} else {
		if m.k8sBrowserNodeCursor < len(m.k8sBrowserNodes)-1 {
			m.k8sBrowserNodeCursor++
		}
	}
	return m, nil
}

// k8sEsc closes confirm dialogs first, then leaves the K8s browser for the
// dashboard.
func (m rootModel) k8sEsc() (rootModel, tea.Cmd) {
	if m.k8sConfirmReboot {
		m.k8sConfirmReboot = false
		return m, nil
	}
	if m.k8sConfirmReplace {
		m.k8sConfirmReplace = false
		return m, nil
	}
	if m.k8sBrowserFocus == 1 {
		m.k8sBrowserFocus = 0
		return m, nil
	}
	m.state = stateDashboard
	m.activeService = serviceK8s
	m.k8sBrowserNodes = nil
	m.k8sBrowserNodePools = nil
	m.k8sConfirmReboot = false
	m.k8sConfirmReplace = false
	return m, nil
}
