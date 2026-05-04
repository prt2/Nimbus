// Checks CPU scoring and a small simulation of packing pods onto fewer nodes.

package scheduler

import "testing"

// TestCPUUtilizationScorePrefersBusierNodes checks that higher CPU use produces a higher score.
func TestCPUUtilizationScorePrefersBusierNodes(t *testing.T) {
	quietNode := cpuUtilizationScore(200, 2000)
	busyNode := cpuUtilizationScore(1400, 2000)

	if busyNode <= quietNode {
		t.Fatalf("expected busy node score %d to exceed quiet node score %d", busyNode, quietNode)
	}
}

// TestBinPackingConcentratesWorkloadsOntoFewerNodes simulates placing three pods and expects one active node.
// This simulation checks capacity before scoring; it does not call the live Kubernetes scheduler.
func TestBinPackingConcentratesWorkloadsOntoFewerNodes(t *testing.T) {
	type node struct {
		used  int64
		total int64
	}

	nodes := []node{
		{used: 0, total: 1000},
		{used: 0, total: 1000},
		{used: 0, total: 1000},
	}

	pods := []int64{250, 250, 250}

	for _, podCPU := range pods {
		bestIdx := -1
		var bestScore int64 = -1
		for i, node := range nodes {
			if node.used+podCPU > node.total {
				continue
			}
			score := cpuUtilizationScore(node.used, node.total)
			if score > bestScore {
				bestScore = score
				bestIdx = i
			}
		}
		if bestIdx == -1 {
			t.Fatalf("no node could fit pod requesting %dm", podCPU)
		}
		nodes[bestIdx].used += podCPU
	}

	activeNodes := 0
	for _, node := range nodes {
		if node.used > 0 {
			activeNodes++
		}
	}

	if activeNodes != 1 {
		t.Fatalf("expected bin-packing simulation to use 1 active node, got %d", activeNodes)
	}
}
