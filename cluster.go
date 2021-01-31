// Copyright 2017 Pilosa Corp.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pilosa

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/pilosa/pilosa/v2/disco"
	"github.com/pilosa/pilosa/v2/internal"
	"github.com/pilosa/pilosa/v2/logger"
	pnet "github.com/pilosa/pilosa/v2/net"
	"github.com/pilosa/pilosa/v2/roaring"
	"github.com/pilosa/pilosa/v2/topology"
	"github.com/pilosa/pilosa/v2/tracing"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"golang.org/x/sync/errgroup"
)

const (
	// ClusterState represents the state returned in the /status endpoint.
	ClusterStateStarting = "STARTING"
	ClusterStateDegraded = "DEGRADED" // cluster is running but we've lost some # of hosts >0 but < replicaN
	ClusterStateNormal   = "NORMAL"
	ClusterStateResizing = "RESIZING"

	// NodeState represents the state of a node during startup.
	nodeStateReady = "READY"
	nodeStateDown  = "DOWN"

	// resizeJob states.
	resizeJobStateRunning = "RUNNING"
	// Final states.
	resizeJobStateDone    = "DONE"
	resizeJobStateAborted = "ABORTED"

	resizeJobActionAdd    = "ADD"
	resizeJobActionRemove = "REMOVE"

	defaultConfirmDownRetries = 10
	defaultConfirmDownSleep   = 1 * time.Second
)

// nodeAction represents a node that is joining or leaving the cluster.
type nodeAction struct {
	node   *topology.Node
	action string
}

// cluster represents a collection of nodes.
type cluster struct { // nolint: maligned
	noder            topology.Noder
	unprotectedNoder topology.Noder

	id   string
	Node *topology.Node

	// Hashing algorithm used to assign partitions to nodes.
	Hasher topology.Hasher

	// The number of partitions in the cluster.
	partitionN int

	// The number of replicas a partition has.
	ReplicaN int

	// Human-readable name of the cluster.
	Name string

	// Maximum number of Set() or Clear() commands per request.
	maxWritesPerRequest int

	// Data directory path.
	Path     string
	Topology *Topology

	// Distributed Consensus
	disCo   disco.DisCo
	stator  disco.Stator
	resizer disco.Resizer
	sharder disco.Sharder

	// Required for cluster Resize.
	Static      bool // Static is primarily used for testing in a non-gossip environment.
	state       string
	Coordinator string
	holder      *Holder
	broadcaster broadcaster

	joiningLeavingNodes chan nodeAction

	// joining is held open until this node
	// receives ClusterStatus from the coordinator.
	joining chan struct{}
	joined  bool

	abortAntiEntropyCh chan struct{}
	muAntiEntropy      sync.Mutex

	translationSyncer TranslationSyncer

	mu         sync.RWMutex
	jobs       map[int64]*resizeJob
	currentJob *resizeJob

	// Close management
	wg      sync.WaitGroup
	closing chan struct{}

	logger logger.Logger

	InternalClient InternalClient

	confirmDownRetries int
	confirmDownSleep   time.Duration
}

// newCluster returns a new instance of Cluster with defaults.
func newCluster() *cluster {
	return &cluster{
		Hasher:     &topology.Jmphasher{},
		partitionN: topology.DefaultPartitionN,
		ReplicaN:   1,

		joiningLeavingNodes: make(chan nodeAction, 10), // buffered channel
		jobs:                make(map[int64]*resizeJob),
		closing:             make(chan struct{}),
		joining:             make(chan struct{}),

		translationSyncer: NopTranslationSyncer,

		InternalClient: newNopInternalClient(),

		logger: logger.NopLogger,

		confirmDownRetries: defaultConfirmDownRetries,
		confirmDownSleep:   defaultConfirmDownSleep,

		noder:  topology.NewEmptyLocalNoder(),
		stator: disco.NopStator,
	}
}

// initializeAntiEntropy is called by the anti entropy routine when it starts.
// If the AE channel is created without a routine reading from it, cluster will
// block indefinitely when calling abortAntiEntropy().
func (c *cluster) initializeAntiEntropy() {
	c.mu.Lock()
	c.abortAntiEntropyCh = make(chan struct{})
	c.mu.Unlock()
}

// abortAntiEntropyQ checks whether the cluster wants to abort the anti entropy
// process (so that it can resize). It does not block.
func (c *cluster) abortAntiEntropyQ() bool {
	select {
	case <-c.abortAntiEntropyCh:
		return true
	default:
		return false
	}
}

// abortAntiEntropy blocks until the anti-entropy routine calls abortAntiEntropyQ
func (c *cluster) abortAntiEntropy() {
	if c.abortAntiEntropyCh != nil {
		c.abortAntiEntropyCh <- struct{}{}
	}
}

func (c *cluster) coordinatorNode() *topology.Node {
	return c.unprotectedCoordinatorNode()
}

// unprotectedCoordinatorNode returns the coordinator node.
func (c *cluster) unprotectedCoordinatorNode() *topology.Node {
	// Create a snapshot of the cluster to use for node/partition calculations.
	snap := topology.NewClusterSnapshot(c.noder, c.Hasher, c.ReplicaN)
	return snap.PrimaryFieldTranslationNode()
}

// isCoordinator is true if this node is the coordinator.
func (c *cluster) isCoordinator() bool {
	return c.unprotectedIsCoordinator()
}

func (c *cluster) unprotectedIsCoordinator() bool {
	// Create a snapshot of the cluster to use for node/partition calculations.
	snap := topology.NewClusterSnapshot(c.noder, c.Hasher, c.ReplicaN)
	return snap.PrimaryFieldTranslationNode().ID == c.Node.ID
}

// setCoordinator tells the current node to become the
// Coordinator. In response to this, the current node
// will consider itself coordinator and update the other
// nodes with its version of Cluster.Status.
func (c *cluster) setCoordinator(n *topology.Node) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Verify that the new Coordinator value matches
	// this node.
	if c.Node.ID != n.ID {
		return fmt.Errorf("coordinator node does not match this node")
	}

	// Update IsCoordinator on all nodes (locally).
	_ = c.unprotectedUpdateCoordinator(n)

	// Send the update coordinator message to all nodes.
	err := c.unprotectedSendSync(
		&UpdateCoordinatorMessage{
			New: n,
		})
	if err != nil {
		return fmt.Errorf("problem sending UpdateCoordinator message: %v", err)
	}

	// Broadcast cluster status.
	return c.unprotectedSendSync(c.unprotectedStatus())
}

// unprotectedSendSync is used in place of c.broadcaster.SendSync (which is
// Server.SendSync) because Server.SendSync needs to obtain a cluster lock to
// get the list of nodes. TODO: the reference loop from
// Server->cluster->broadcaster(Server) will likely continue to cause confusion
// and should be refactored.
func (c *cluster) unprotectedSendSync(m Message) error {
	var eg errgroup.Group
	for _, node := range c.noder.Nodes() {
		node := node
		// Don't send to myself.
		if node.ID == c.Node.ID {
			continue
		}
		eg.Go(func() error { return c.broadcaster.SendTo(node, m) })
	}
	return eg.Wait()
}

// updateCoordinator updates this nodes Coordinator value as well as
// changing the corresponding node's IsCoordinator value
// to true, and sets all other nodes to false. Returns true if the value
// changed.
func (c *cluster) updateCoordinator(n *topology.Node) bool { // nolint: unparam
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.unprotectedUpdateCoordinator(n)
}

func (c *cluster) unprotectedUpdateCoordinator(n *topology.Node) bool {
	var changed bool
	if c.Coordinator != n.ID {
		c.Coordinator = n.ID
		changed = true
	}
	for _, node := range c.noder.Nodes() {
		if node.ID == n.ID {
			node.IsCoordinator = true
		} else {
			node.IsCoordinator = false
		}
	}
	return changed
}

// addNode adds a node to the Cluster and updates and saves the
// new topology. unprotected.
func (c *cluster) addNode(node *topology.Node) error {
	// If the node being added is the coordinator, set it for this node.
	if node.IsCoordinator {
		c.Coordinator = node.ID
	}

	// add to cluster
	if !c.addNodeBasicSorted(node) {
		return nil
	}

	// add to topology
	if c.Topology == nil {
		return fmt.Errorf("Cluster.Topology is nil")
	}
	if !c.Topology.addID(node.ID) {
		return nil
	}
	c.Topology.nodeStates[node.ID] = node.State

	// save topology
	return c.saveTopology()
}

// removeNode removes a node from the Cluster and updates and saves the
// new topology. unprotected.
func (c *cluster) removeNode(nodeID string) error {
	// remove from cluster
	c.removeNodeBasicSorted(nodeID)

	// remove from topology
	if c.Topology == nil {
		return fmt.Errorf("Cluster.Topology is nil")
	}
	if !c.Topology.removeID(nodeID) {
		return nil
	}

	// save topology
	return c.saveTopology()
}

// nodeIDs returns the list of IDs in the cluster.
func (c *cluster) nodeIDs() []string {
	return topology.Nodes(c.Nodes()).IDs()
}

func (c *cluster) unprotectedSetID(id string) {
	// Don't overwrite ClusterID.
	if c.id != "" {
		return
	}
	c.id = id

	// Make sure the Topology is updated.
	c.Topology.clusterID = c.id
}

func (c *cluster) State() (string, error) {
	state, err := c.stator.ClusterState(context.Background())
	if err != nil {
		return string(disco.ClusterStateUnknown), err
	}
	return string(state), nil
}

func (c *cluster) SetState(state string) {
	c.mu.Lock()
	c.unprotectedSetState(state)
	c.mu.Unlock()
}

func (c *cluster) unprotectedSetState(state string) {
	// Ignore cases where the state hasn't changed.
	if state == c.state {
		return
	}

	c.logger.Printf("change cluster state from %s to %s on %s", c.state, state, c.Node.ID)

	var doCleanup bool

	switch state {
	case ClusterStateNormal, ClusterStateDegraded:
		// If state is RESIZING -> [NORMAL, DEGRADED] then run cleanup.
		if c.state == ClusterStateResizing {
			doCleanup = true
		}
	}

	c.state = state

	switch state {
	case ClusterStateNormal:
		// Because the cluster state is changing to NORMAL,
		// we [potentially] need to reset the translation sync.
		// If, for example, the cluster has changed size and is
		// now settling to NORMAL, the partition ownership may
		// have changed, and this will force that to be recalculated.
		//
		// We can't call Reset() if Server.Open() hasn't run yet,
		// because that's where we start monitorResetTranslationSync()
		// which reads the reset channel. If we get here before
		// Server.Open(), this will deadlock on that channel read.
		// In order to address this, we call Reset() in a goroutine
		// so even if it blocks waiting for monitorResetTranslationSync()
		// to start, it doesn't cause a deadlock, and once Server.Open()
		// is called, then the sync reset (or in the STARTING case, the
		// initial sync start) will happen.
		go func() {
			if err := c.translationSyncer.Reset(); err != nil {
				c.logger.Printf("error resetting translation syncer: %s", err)
			}
		}()
	}

	// TODO: consider NOT running cleanup on an active node that has
	// been removed.
	// It's safe to do a cleanup after state changes back to normal.
	if doCleanup {
		var cleaner holderCleaner
		cleaner.Node = c.Node
		cleaner.Holder = c.holder
		cleaner.Cluster = c
		cleaner.Closing = c.closing

		// Clean holder. This is where the shard gets removed after resize.
		if err := cleaner.CleanHolder(); err != nil {
			c.logger.Printf("holder clean error: err=%s", err)
		}
	}
}

func (c *cluster) setMyNodeState(state string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Node.State = state
	nodes := c.noder.Nodes()
	for i, n := range nodes {
		if n.ID == c.Node.ID {
			nodes[i].State = state
		}
	}
}

// receiveNodeState sets node state in Topology in order for the
// Coordinator to keep track of, during startup, which nodes have
// finished opening their Holder.
func (c *cluster) receiveNodeState(nodeID string, state string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.unprotectedIsCoordinator() {
		return nil
	}

	c.Topology.mu.Lock()
	changed := false
	if c.Topology.nodeStates[nodeID] != state {
		changed = true
		c.Topology.nodeStates[nodeID] = state
		nodes := c.noder.Nodes()
		for i, n := range nodes {
			if n.ID == nodeID {
				nodes[i].Mu.Lock()
				nodes[i].State = state
				nodes[i].Mu.Unlock()
			}
		}
	}
	c.Topology.mu.Unlock()
	c.logger.Printf("received state %s (%s)", state, nodeID)

	if changed {
		return c.unprotectedSetStateAndBroadcast(c.determineClusterState())
	}
	return nil
}

// determineClusterState is unprotected.
func (c *cluster) determineClusterState() (clusterState string) {
	if c.state == ClusterStateResizing {
		return ClusterStateResizing
	}
	if c.haveTopologyAgreement() && c.allNodesReady() {
		return ClusterStateNormal
	}
	// TODO:
	// If the cluster is still STARTING, there's no need to put it into
	// state DEGRADED. It's possible to force a starting cluster to go
	// into state DEGRADED by, for example, restarting a 2-node cluster
	// with replica=3. In that case, the coordinator would come up and
	// it would immediately trigger this condition. Checking for
	// state != STARTING here would prevent that. Unfortunately, based
	// on test TestClusteringNodesReplica2, we expect a DEGRADED cluster
	// to go back into state STARTING if it loses more replicas than
	// can support queries. In that case, we might actually want it to
	// go from STARTING back to DEGRADED. Leaving it as is for now, but
	// noting that it's a little confusing that a cluster starting up
	// could possibly go into state DEGRADED.
	if len(c.Topology.nodeIDs)-len(c.nodeIDs()) < c.ReplicaN && c.allNodesReady() {
		return ClusterStateDegraded
	}
	return ClusterStateStarting
}

// unprotectedStatus returns the the cluster's status including what nodes it contains, its ID, and current state.
func (c *cluster) unprotectedStatus() *ClusterStatus {
	return &ClusterStatus{
		ClusterID: c.id,
		State:     c.state,
		Nodes:     c.noder.Nodes(),
		Schema:    &Schema{Indexes: c.holder.Schema()},
	}
}

func (c *cluster) nodeByID(id string) *topology.Node {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.unprotectedNodeByID(id)
}

// unprotectedNodeByID returns a node reference by ID.
func (c *cluster) unprotectedNodeByID(id string) *topology.Node {
	for _, n := range c.noder.Nodes() {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func (c *cluster) topologyContainsNode(id string) bool {
	c.Topology.mu.RLock()
	defer c.Topology.mu.RUnlock()
	for _, nid := range c.Topology.nodeIDs {
		if id == nid {
			return true
		}
	}
	return false
}

// nodePositionByID returns the position of the node in slice c.Nodes.
func (c *cluster) nodePositionByID(nodeID string) int {
	for i, n := range c.noder.Nodes() {
		if n.ID == nodeID {
			return i
		}
	}
	return -1
}

// addNodeBasicSorted adds a node to the cluster, sorted by id. Returns a
// pointer to the node and true if the node was added or updated. unprotected.
func (c *cluster) addNodeBasicSorted(node *topology.Node) bool {
	n := c.unprotectedNodeByID(node.ID)

	if n != nil {
		// prevent race on node.URI read against http/client.go:1929
		n.Mu.Lock()
		defer n.Mu.Unlock()

		if n.State != node.State || n.IsCoordinator != node.IsCoordinator || n.URI != node.URI {
			n.State = node.State
			n.IsCoordinator = node.IsCoordinator
			n.URI = node.URI
			n.GRPCURI = node.GRPCURI
			return true
		}
		return false
	}

	c.noder.AppendNode(node)

	// All hosts must be merged in the same order on all nodes in the cluster.
	// sort.Sort(topology.ByID(c.nodes)) // TODO: this should no longer apply

	return true
}

// Nodes returns a copy of the slice of nodes in the cluster. Safe for
// concurrent use, result may be modified.
func (c *cluster) Nodes() []*topology.Node {
	nodes := c.noder.Nodes()

	// Create a snapshot of the cluster to use for node/partition calculations.
	snap := topology.NewClusterSnapshot(topology.NewLocalNoder(nodes), c.Hasher, c.ReplicaN)
	primaryNode := snap.PrimaryFieldTranslationNode()

	// Set node states and IsPrimary.
	for _, node := range nodes {
		node.IsCoordinator = node.ID == primaryNode.ID
		// s, err := c.stator.NodeState(context.Background(), node.ID)
		// if err != nil {
		// 	node.State = nodeStateDown
		// 	continue
		// }
		// node.State = string(s)

	}

	return nodes
}

func (c *cluster) AllNodeStates() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Topology.nodeStates
}

// removeNodeBasicSorted removes a node from the cluster, maintaining the sort
// order. Returns true if the node was removed. unprotected.
func (c *cluster) removeNodeBasicSorted(nodeID string) bool {
	return c.noder.RemoveNode(nodeID)
}

// frag is a struct of basic fragment information.
type frag struct {
	field string
	view  string
	shard uint64
}

func fragsDiff(a, b []frag) []frag {
	m := make(map[frag]uint64)

	for _, y := range b {
		m[y]++
	}

	var ret []frag
	for _, x := range a {
		if m[x] > 0 {
			m[x]--
			continue
		}
		ret = append(ret, x)
	}

	return ret
}

type fragsByHost map[string][]frag

type viewsByField map[string][]string

func (a viewsByField) addView(field, view string) {
	a[field] = append(a[field], view)
}

func (c *cluster) fragsByHost(idx *Index) fragsByHost {
	// fieldViews is a map of field to slice of views.
	fieldViews := make(viewsByField)

	for _, field := range idx.Fields() {
		for _, view := range field.views() {
			fieldViews.addView(field.Name(), view.name)
		}
	}
	return c.fragCombos(idx.Name(), idx.AvailableShards(includeRemote), fieldViews)
}

// fragCombos returns a map (by uri) of lists of fragments for a given index
// by creating every combination of field/view specified in `fieldViews` up
// for the given set of shards with data.
func (c *cluster) fragCombos(idx string, availableShards *roaring.Bitmap, fieldViews viewsByField) fragsByHost {
	// Create a snapshot of the cluster to use for node/partition calculations.
	snap := topology.NewClusterSnapshot(c.noder, c.Hasher, c.ReplicaN)

	t := make(fragsByHost)
	_ = availableShards.ForEach(func(i uint64) error {
		nodes := snap.ShardNodes(idx, i)
		for _, n := range nodes {
			// for each field/view combination:
			for field, views := range fieldViews {
				for _, view := range views {
					t[n.ID] = append(t[n.ID], frag{field, view, i})
				}
			}
		}
		return nil
	})
	return t
}

// diff compares c with another cluster and determines if a node is being
// added or removed. An error is returned for any case other than where
// exactly one node is added or removed. unprotected.
func (c *cluster) diff(other *cluster) (action string, nodeID string, err error) {
	cNodes := c.noder.Nodes()
	otherNodes := other.noder.Nodes()
	lenFrom := len(cNodes)
	lenTo := len(otherNodes)
	// Determine if a node is being added or removed.
	if lenFrom == lenTo {
		return "", "", errors.New("clusters are the same size")
	}
	if lenFrom < lenTo {
		// Adding a node.
		if lenTo-lenFrom > 1 {
			return "", "", errors.New("adding more than one node at a time is not supported")
		}
		action = resizeJobActionAdd
		// Determine the node ID that is being added.
		for _, n := range otherNodes {
			if c.unprotectedNodeByID(n.ID) == nil {
				nodeID = n.ID
				break
			}
		}
	} else if lenFrom > lenTo {
		// Removing a node.
		if lenFrom-lenTo > 1 {
			return "", "", errors.New("removing more than one node at a time is not supported")
		}
		action = resizeJobActionRemove
		// Determine the node ID that is being removed.
		for _, n := range cNodes {
			if other.unprotectedNodeByID(n.ID) == nil {
				nodeID = n.ID
				break
			}
		}
	}
	return action, nodeID, nil
}

// fragSources returns a list of ResizeSources - for each node in the `to` cluster -
// required to move from cluster `c` to cluster `to`. unprotected.
func (c *cluster) fragSources(to *cluster, idx *Index) (map[string][]*ResizeSource, error) {
	m := make(map[string][]*ResizeSource)

	// Determine if a node is being added or removed.
	action, diffNodeID, err := c.diff(to)
	if err != nil {
		return nil, errors.Wrap(err, "diffing")
	}

	// Initialize the map with all the nodes in `to`.
	for _, n := range to.noder.Nodes() {
		m[n.ID] = nil
	}

	// If a node is being added, the source can be confined to the
	// primary fragments (i.e. no need to use replicas as source data).
	// In this case, source fragments can be based on a cluster with
	// replica = 1.
	// If a node is being removed, however, then it will most likely
	// require that a replica fragment be the source data.
	srcCluster := c
	if action == resizeJobActionAdd && c.ReplicaN > 1 {
		srcCluster = newCluster()
		srcCluster.noder.SetNodes(topology.Nodes(c.noder.Nodes()).Clone())
		srcCluster.Hasher = c.Hasher
		srcCluster.partitionN = c.partitionN
		srcCluster.ReplicaN = 1
	}

	// Represents the fragment location for the from/to clusters.
	fFrags := c.fragsByHost(idx)
	tFrags := to.fragsByHost(idx)

	// srcFrags is the frag map based on a source cluster of replica = 1.
	srcFrags := srcCluster.fragsByHost(idx)

	// srcNodesByFrag is the inverse representation of srcFrags.
	srcNodesByFrag := make(map[frag]string)
	for nodeID, frags := range srcFrags {
		// If a node is being removed, don't consider it as a source.
		if action == resizeJobActionRemove && nodeID == diffNodeID {
			continue
		}
		for _, frag := range frags {
			srcNodesByFrag[frag] = nodeID
		}
	}

	// Get the frag diff for each nodeID.
	diffs := make(fragsByHost)
	for nodeID, frags := range tFrags {
		if _, ok := fFrags[nodeID]; ok {
			diffs[nodeID] = fragsDiff(frags, fFrags[nodeID])
		} else {
			diffs[nodeID] = frags
		}
	}

	// Get the ResizeSource for each diff.
	for nodeID, diff := range diffs {
		m[nodeID] = []*ResizeSource{}
		for _, frag := range diff {
			// If there is no valid source node ID for a fragment,
			// it likely means that the replica factor was not
			// high enough for the remaining nodes to contain
			// the fragment.
			srcNodeID, ok := srcNodesByFrag[frag]
			if !ok {
				return nil, errors.New("not enough data to perform resize (replica factor may need to be increased)")
			}

			src := &ResizeSource{
				Node:  c.unprotectedNodeByID(srcNodeID),
				Index: idx.Name(),
				Field: frag.field,
				View:  frag.view,
				Shard: frag.shard,
			}

			m[nodeID] = append(m[nodeID], src)
		}
	}

	return m, nil
}

// translationNodes returns a list of translationResizeNodes - for each node
// in the `to` cluster - required to move from cluster `c` to cluster `to`. unprotected.
// Because the parition scheme for every index is the same, this is used as a template
// to create index-specific `TranslationResizeSource`s.
func (c *cluster) translationNodes(to *cluster) (map[string][]*translationResizeNode, error) {
	m := make(map[string][]*translationResizeNode)

	// Determine if a node is being added or removed.
	action, diffNodeID, err := c.diff(to)
	if err != nil {
		return nil, errors.Wrap(err, "diffing")
	}

	// Initialize the map with all the nodes in `to`.
	for _, n := range to.noder.Nodes() {
		m[n.ID] = nil
	}

	// Create a snapshot of the cluster to use for node/partition calculations.
	fSnap := topology.NewClusterSnapshot(c.noder, c.Hasher, c.ReplicaN)
	toSnap := topology.NewClusterSnapshot(to.noder, c.Hasher, to.ReplicaN)

	for pid := 0; pid < c.partitionN; pid++ {
		fNodes := fSnap.PartitionNodes(pid)
		tNodes := toSnap.PartitionNodes(pid)

		// For `to` cluster, we include all nodes containing a
		// replica for the partition. The source for each replica
		// will be the primary in the `from` cluster. For the `from`
		// cluster, we only need the first node, unless that node is
		// being removed, then we use the second node. If no second
		// node exists in that case, then we have to raise an error
		// indicating that not enough replicas exist to support
		// the resize.
		if len(tNodes) > 0 {
			var foundPrimary bool
			for i := range fNodes {
				if action == resizeJobActionRemove && fNodes[i].ID == diffNodeID {
					continue
				}
				// We only need to add the source if the nodes differ;
				// in other words if the primary partition is on the
				// same node, it doesn't need to retrieve it.
				for n := range tNodes {
					if tNodes[n].ID != fNodes[i].ID {
						m[tNodes[n].ID] = append(m[tNodes[n].ID],
							&translationResizeNode{
								node:        fNodes[i],
								partitionID: pid,
							})
					}
				}
				foundPrimary = true
				break
			}
			if !foundPrimary {
				return nil, ErrResizeNoReplicas
			}
		}
	}

	return m, nil
}

// shardDistributionByIndex returns a map of [nodeID][primaryOrReplica][]uint64,
// where the int slices are lists of shards.
func (c *cluster) shardDistributionByIndex(indexName string) map[string]map[string][]uint64 {
	dist := make(map[string]map[string][]uint64)

	for _, node := range c.noder.Nodes() {
		nodeDist := make(map[string][]uint64)
		nodeDist["primary-shards"] = make([]uint64, 0)
		nodeDist["replica-shards"] = make([]uint64, 0)
		dist[node.ID] = nodeDist
	}

	index := c.holder.Index(indexName)
	available := index.AvailableShards(includeRemote).Slice()

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Create a snapshot of the cluster to use for node/partition calculations.
	snap := topology.NewClusterSnapshot(c.noder, c.Hasher, c.ReplicaN)

	for _, shard := range available {
		p := snap.ShardToShardPartition(indexName, shard)
		nodes := snap.PartitionNodes(p)
		dist[nodes[0].ID]["primary-shards"] = append(dist[nodes[0].ID]["primary-shards"], shard)
		for k := 1; k < len(nodes); k++ {
			dist[nodes[k].ID]["replica-shards"] = append(dist[nodes[k].ID]["replica-shards"], shard)
		}
	}

	return dist
}

// shardPartition returns the shard-partition that a shard belongs to.
// NOTE: this is DIFFERENT from the key-partition
func (c *cluster) shardToShardPartition(index string, shard uint64) int {
	return shardToShardPartition(index, shard, c.partitionN)
}

func shardToShardPartition(index string, shard uint64, partitionN int) int {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], shard)

	// Hash the bytes and mod by partition count.
	h := fnv.New64a()
	_, _ = h.Write([]byte(index))
	_, _ = h.Write(buf[:])
	return int(h.Sum64() % uint64(partitionN))
}

// KeyPartition returns the key-partition that a key belongs to.
// NOTE: the key-partition is DIFFERENT from the shard-partition.
func (t *Topology) KeyPartition(index, key string) int {
	return keyToKeyPartition(index, key, t.PartitionN)
}

func keyToKeyPartition(index, key string, partitionN int) int {
	// Hash the bytes and mod by partition count.
	h := fnv.New64a()
	_, _ = h.Write([]byte(index))
	_, _ = h.Write([]byte(key))
	return int(h.Sum64() % uint64(partitionN))
}

// ShardNodes returns a list of nodes that own a fragment. Safe for concurrent use.
func (c *cluster) ShardNodes(index string, shard uint64) []*topology.Node {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.shardNodes(index, shard)
}

// shardNodes returns a list of nodes that own a shard. unprotected
func (c *cluster) shardNodes(index string, shard uint64) []*topology.Node {
	return c.partitionNodes(c.shardToShardPartition(index, shard))
}

// KeyNodes returns a list of nodes that own a fragment. Safe for concurrent use.
func (c *cluster) KeyNodes(index, key string) []*topology.Node {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.keyNodes(index, key)
}

// keyNodes returns a list of nodes that own a key. unprotected
func (c *cluster) keyNodes(index, key string) []*topology.Node {
	return c.partitionNodes(c.Topology.KeyPartition(index, key))
}

// partitionNodes returns a list of nodes that own a partition. unprotected.
func (c *cluster) partitionNodes(partitionID int) []*topology.Node {
	// Default replica count to between one and the number of nodes.
	// The replica count can be zero if there are no nodes.

	// Assume that c.nodes may be missing a node that is part of the cluster but not currently present.
	// The partition calculation must use the full cluster size in BOTH cases:
	// - use len(c.Topology.nodeIDs) instead of len(c.nodes),
	// - collect nodes from c.Topology.nodeIDs rather than from c.nodes,
	// - when the node is missing, it should be considered, found absent from c.nodes, then omitted from the return slice.

	// Use c.Topology to determine cluster membership when it
	// exists and contains data. Otherwise, fall back to using
	// c.nodes. The only time c.Topology should be nil is in
	// tests.
	var useTopology bool
	if c.Topology != nil && len(c.Topology.nodeIDs) > 0 {
		useTopology = true
	}

	cNodes := c.noder.Nodes()

	replicaN := c.ReplicaN
	var nodeN int
	if useTopology {
		nodeN = len(c.Topology.nodeIDs)
	} else {
		nodeN = len(cNodes)
	}
	if replicaN > nodeN {
		replicaN = nodeN
	} else if replicaN == 0 {
		replicaN = 1
	}

	// Determine primary owner node.
	if c.Topology == nil {
		c.Topology = NewTopology(c.Hasher, c.partitionN, c.ReplicaN, c)
	}
	nodeIndex := c.Topology.PrimaryNodeIndex(partitionID)
	if nodeIndex < 0 {
		// no nodes anyway
		return nil
	}
	// Collect nodes around the ring.
	nodes := make([]*topology.Node, 0, replicaN)
	for i := 0; i < replicaN; i++ {
		if useTopology {
			maybeNodeID := c.Topology.nodeIDs[(nodeIndex+i)%nodeN]
			if node := topology.Nodes(cNodes).NodeByID(maybeNodeID); node != nil {
				nodes = append(nodes, node)
			}
		} else {
			nodes = append(nodes, cNodes[(nodeIndex+i)%len(cNodes)])
		}
	}

	return nodes
}

func (c *cluster) primaryPartitionNode(partition int) *topology.Node {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.unprotectedPrimaryPartitionNode(partition)
}

// unprotectedPrimaryPartition returns tprimary node of partition.
func (c *cluster) unprotectedPrimaryPartitionNode(partition int) *topology.Node {
	if nodes := c.partitionNodes(partition); len(nodes) > 0 {
		return nodes[0]
	}
	return nil
}

func (t *Topology) IsPrimary(nodeID string, partitionID int) bool {
	primary := t.PrimaryNodeIndex(partitionID)
	return nodeID == t.nodeIDs[primary]
}

func (t *Topology) PrimaryNodeIndex(partitionID int) (nodeIndex int) {
	n := len(t.nodeIDs)
	if n == 0 {
		if t.cluster != nil {
			n = len(t.cluster.noder.Nodes())
		}
	}
	nodeIndex = t.Hasher.Hash(uint64(partitionID), n)
	return
}

func (t *Topology) GetNonPrimaryReplicas(partitionID int) (nonPrimaryReplicas []string) {

	primary := t.PrimaryNodeIndex(partitionID)
	nodeN := len(t.nodeIDs)

	// Collect nodes around the ring.
	for i := 1; i < nodeN; i++ {
		nodeID := t.nodeIDs[(primary+i)%nodeN]
		if i < t.ReplicaN {
			nonPrimaryReplicas = append(nonPrimaryReplicas, nodeID)
		}
	}
	return
}

// the map replicaNodeIDs[nodeID] will have a true value for the primary nodeID, and false for others.
func (t *Topology) GetReplicasForPrimary(primary int) (replicaNodeIDs, nonReplicas map[string]bool) {
	if primary < 0 {
		// no nodes anyway
		return
	}
	replicaNodeIDs = make(map[string]bool)
	nonReplicas = make(map[string]bool)

	nodeN := len(t.nodeIDs)

	// Collect nodes around the ring.
	for i := 0; i < nodeN; i++ {
		nodeID := t.nodeIDs[(primary+i)%nodeN]
		if i < t.ReplicaN {
			// mark true if primary
			replicaNodeIDs[nodeID] = (i == 0)
		} else {
			nonReplicas[nodeID] = false
		}
	}
	return
}

// containsShards is like OwnsShards, but it includes replicas.
func (c *cluster) containsShards(index string, availableShards *roaring.Bitmap, node *topology.Node) []uint64 {
	var shards []uint64
	_ = availableShards.ForEach(func(i uint64) error {
		p := c.shardToShardPartition(index, i)
		// Determine the nodes for partition.
		nodes := c.partitionNodes(p)
		for _, n := range nodes {
			if n.ID == node.ID {
				shards = append(shards, i)
			}
		}
		return nil
	})
	return shards
}

func (c *cluster) setup() error {
	// Cluster always comes up in state STARTING until cluster membership is determined.
	c.state = ClusterStateStarting

	// Load topology file if it exists.
	if err := c.loadTopology(); err != nil {
		return errors.Wrap(err, "loading topology")
	}

	c.id = c.Topology.clusterID

	// Only the coordinator needs to consider the .topology file.
	if c.isCoordinator() {
		err := c.considerTopology()
		if err != nil {
			return errors.Wrap(err, "considerTopology")
		}
	}

	// Add the local node to the cluster.
	err := c.addNode(c.Node)
	if err != nil {
		return errors.Wrap(err, "adding local node")
	}
	return nil
}

// open is only used in internal tests.
func (c *cluster) open() error {
	err := c.setup()
	if err != nil {
		return errors.Wrap(err, "setting up cluster")
	}
	return c.waitForStarted()
}

func (c *cluster) waitForStarted() error {
	return nil
}

func (c *cluster) close() error {
	// Notify goroutines of closing and wait for completion.
	close(c.closing)
	c.wg.Wait()

	return nil
}

func (c *cluster) markAsJoined() {
	if !c.joined {
		c.joined = true
		close(c.joining)
	}
}

// needTopologyAgreement is unprotected.
func (c *cluster) needTopologyAgreement() bool {
	return false
}

// haveTopologyAgreement is unprotected.
func (c *cluster) haveTopologyAgreement() bool {
	if c.Static {
		return true
	}
	return stringSlicesAreEqual(c.Topology.nodeIDs, c.nodeIDs())
}

// allNodesReady is unprotected.
func (c *cluster) allNodesReady() (ret bool) {
	if c.Static {
		return true
	}
	for _, id := range c.nodeIDs() {
		if c.Topology.nodeStates[id] != nodeStateReady {
			return false
		}
	}
	return true
}

func (c *cluster) handleNodeAction(nodeAction nodeAction) error {
	c.mu.Lock()
	j, err := c.unprotectedGenerateResizeJob(nodeAction)
	c.mu.Unlock()
	if err != nil {
		c.logger.Printf("generateResizeJob error: err=%s", err)
		if err := c.setStateAndBroadcast(ClusterStateNormal); err != nil {
			c.logger.Printf("setStateAndBroadcast error: err=%s", err)
		}
		return errors.Wrap(err, "setting state")
	}

	// j.Run() runs in a goroutine because in the case where the
	// job requires no action, it immediately writes to the j.result
	// channel, which is not consumed until the code below.
	var eg errgroup.Group
	eg.Go(func() error {
		return j.run()
	})

	// Wait for the resizeJob to finish or be aborted.
	c.logger.Printf("wait for jobResult")
	var jobResult string
	select {
	case <-c.closing:
		return errors.New("cluster shut down during resize")
	case jobResult = <-j.result:
	}

	// Make sure j.run() didn't return an error.
	if eg.Wait() != nil {
		return errors.Wrap(err, "running job")
	}

	c.logger.Printf("received jobResult: %s", jobResult)
	switch jobResult {
	case resizeJobStateDone:
		if err := c.completeCurrentJob(resizeJobStateDone); err != nil {
			return errors.Wrap(err, "completing finished job")
		}
		// Add/remove uri to/from the cluster.
		if j.action == resizeJobActionRemove {
			c.mu.Lock()
			defer c.mu.Unlock()
			return c.removeNode(nodeAction.node.ID)
		} else if j.action == resizeJobActionAdd {
			c.mu.Lock()
			defer c.mu.Unlock()
			return c.addNode(nodeAction.node)
		}
	case resizeJobStateAborted:
		if err := c.completeCurrentJob(resizeJobStateAborted); err != nil {
			return errors.Wrap(err, "completing aborted job")
		}
	}
	return nil
}

func (c *cluster) setStateAndBroadcast(state string) error { // nolint: unparam
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.unprotectedSetStateAndBroadcast(state)
}

func (c *cluster) unprotectedSetStateAndBroadcast(state string) error {
	c.unprotectedSetState(state)
	if c.Static {
		return nil
	}
	// Broadcast cluster status changes to the cluster.
	status := c.unprotectedStatus()
	return c.unprotectedSendSync(status) // TODO fix c.Status
}

func (c *cluster) sendTo(node *topology.Node, m Message) error {
	if err := c.broadcaster.SendTo(node, m); err != nil {
		return errors.Wrap(err, "sending")
	}
	return nil
}

// listenForJoins handles cluster-resize events.
func (c *cluster) listenForJoins() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		// When a cluster starts, the state is STARTING.
		// We first want to wait for at least one node to join.
		// Then we want to clear out the joiningLeavingNodes queue (buffered channel).
		// Then we want to set the cluster state to NORMAL and resume processing of joiningLeavingNodes events.
		// We use a bool `setNormal` to indicate when at least one node has joined.
		var setNormal bool
		for {
			// Handle all pending joins before changing state back to NORMAL.
			select {
			case nodeAction := <-c.joiningLeavingNodes:
				err := c.handleNodeAction(nodeAction)
				if err != nil {
					c.logger.Printf("handleNodeAction error: err=%s", err)
					continue
				}
				setNormal = true
				continue
			default:
			}

			// Only change state to NORMAL if we have successfully added at least one host.
			if setNormal {
				// Put the cluster back to state NORMAL and broadcast.
				if err := c.setStateAndBroadcast(ClusterStateNormal); err != nil {
					c.logger.Printf("setStateAndBroadcast error: err=%s", err)
				}
			}

			// Wait for a joining host or a close.
			select {
			case <-c.closing:
				return
			case nodeAction := <-c.joiningLeavingNodes:
				err := c.handleNodeAction(nodeAction)
				if err != nil {
					c.logger.Printf("handleNodeAction error: err=%s", err)
					continue
				}
				setNormal = true
				continue
			}
		}
	}()
}

// unprotectedGenerateResizeJob creates a new resizeJob based on the new node being
// added/removed. It also saves a reference to the resizeJob in the `jobs` map
// for future lookup by JobID.
func (c *cluster) unprotectedGenerateResizeJob(nodeAction nodeAction) (*resizeJob, error) {
	c.logger.Printf("generateResizeJob: %v", nodeAction)

	j, err := c.unprotectedGenerateResizeJobByAction(nodeAction)
	if err != nil {
		return nil, errors.Wrap(err, "generating job")
	}
	c.logger.Printf("generated resizeJob: %d", j.ID)

	// Save job in jobs map for future reference.
	c.jobs[j.ID] = j

	// Set job as currentJob.
	if c.currentJob != nil {
		return nil, fmt.Errorf("there is currently a resize job running")
	}
	c.currentJob = j

	return j, nil
}

// unprotectedGenerateResizeJobByAction returns a resizeJob with instructions based on
// the difference between Cluster and a new Cluster with/without uri.
// Broadcaster is associated to the resizeJob here for use in broadcasting
// the resize instructions to other nodes in the cluster.
func (c *cluster) unprotectedGenerateResizeJobByAction(nodeAction nodeAction) (*resizeJob, error) {
	j := newResizeJob(c.noder.Nodes(), nodeAction.node, nodeAction.action)
	// A *new* node which is being added needs a schema update even if
	// there's no data to send it.
	var sendSchemaToNewNode string
	j.Broadcaster = c.broadcaster

	// toCluster is a clone of Cluster with the new node added/removed for comparison.
	toCluster := newCluster()
	toCluster.noder.SetNodes(topology.Nodes(c.noder.Nodes()).Clone())
	toCluster.Hasher = c.Hasher
	toCluster.partitionN = c.partitionN
	toCluster.ReplicaN = c.ReplicaN
	if nodeAction.action == resizeJobActionRemove {
		toCluster.removeNodeBasicSorted(nodeAction.node.ID)
	} else if nodeAction.action == resizeJobActionAdd {
		toCluster.addNodeBasicSorted(nodeAction.node)
		sendSchemaToNewNode = nodeAction.node.ID
	}

	indexes := c.holder.Indexes()

	// fragmentSourcesByNode is a map of Node.ID to sources of fragment data.
	// It is initialized with all the nodes in toCluster.
	fragmentSourcesByNode := make(map[string][]*ResizeSource)
	for _, n := range toCluster.noder.Nodes() {
		fragmentSourcesByNode[n.ID] = nil
	}

	// Add to fragmentSourcesByNode the instructions for each index.
	for _, idx := range indexes {
		fragSources, err := c.fragSources(toCluster, idx)
		if err != nil {
			return nil, errors.Wrap(err, "getting sources")
		}

		for nodeid, sources := range fragSources {
			fragmentSourcesByNode[nodeid] = append(fragmentSourcesByNode[nodeid], sources...)
		}
	}

	// translationSourcesByNode is a map of Node.ID to sources of partitioned
	// key translation data for indexes.
	// It is initialized with all the nodes in toCluster.
	translationSourcesByNode := make(map[string][]*TranslationResizeSource)
	for _, n := range toCluster.noder.Nodes() {
		translationSourcesByNode[n.ID] = nil
	}

	if len(indexes) > 0 {
		// Add to translationSourcesByNode the instructions for the cluster.
		translationNodes, err := c.translationNodes(toCluster)
		if err != nil {
			return nil, errors.Wrap(err, "getting translation sources")
		}

		// Create a list of TranslationResizeSource for each index,
		// using translationNodes as a template.
		translationSources := make(map[string][]*TranslationResizeSource)
		for _, idx := range indexes {
			// Only include indexes with keys.
			if !idx.Keys() {
				continue
			}
			indexName := idx.Name()
			for node, resizeNodes := range translationNodes {
				for i := range resizeNodes {
					translationSources[node] = append(translationSources[node],
						&TranslationResizeSource{
							Node:        resizeNodes[i].node,
							Index:       indexName,
							PartitionID: resizeNodes[i].partitionID,
						})
				}
			}
		}

		for nodeid, sources := range translationSources {
			translationSourcesByNode[nodeid] = sources
		}
	}

	for _, node := range toCluster.noder.Nodes() {
		dataToSend := len(fragmentSourcesByNode[node.ID]) != 0 || len(translationSourcesByNode[node.ID]) != 0
		// If we're adding a new node, that node needs to get a resize
		// instruction even if there's no data it needs to read.
		// Existing nodes already got the schema and are assumed to be
		// up to date on it.
		if !dataToSend && node.ID != sendSchemaToNewNode {
			j.IDs[node.ID] = true
			continue
		}

		// Create a snapshot of the cluster to use for node/partition calculations.
		snap := topology.NewClusterSnapshot(c.noder, c.Hasher, c.ReplicaN)

		instr := &ResizeInstruction{
			JobID:              j.ID,
			Node:               toCluster.unprotectedNodeByID(node.ID),
			Coordinator:        snap.PrimaryFieldTranslationNode(),
			Sources:            fragmentSourcesByNode[node.ID],
			TranslationSources: translationSourcesByNode[node.ID],
			NodeStatus:         c.nodeStatus(), // Include the NodeStatus in order to ensure that schema and availableShards are in sync on the receiving node.
			ClusterStatus:      c.unprotectedStatus(),
		}
		j.Instructions = append(j.Instructions, instr)
	}

	return j, nil
}

// completeCurrentJob sets the state of the current resizeJob
// then removes the pointer to currentJob.
func (c *cluster) completeCurrentJob(state string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.unprotectedCompleteCurrentJob(state)
}

func (c *cluster) unprotectedCompleteCurrentJob(state string) error {
	// Create a snapshot of the cluster to use for node/partition calculations.
	snap := topology.NewClusterSnapshot(c.noder, c.Hasher, c.ReplicaN)
	if !snap.IsPrimaryFieldTranslationNode(c.Node.ID) {
		return ErrNodeNotCoordinator
	}
	if c.currentJob == nil {
		return ErrResizeNotRunning
	}
	c.currentJob.setState(state)
	c.currentJob = nil
	return nil
}

// followResizeInstruction is run by any node that receives a ResizeInstruction.
func (c *cluster) followResizeInstruction(instr *ResizeInstruction) error {
	c.logger.Printf("follow resize instruction on %s", c.Node.ID)
	// Make sure the cluster status on this node agrees with the Coordinator
	// before attempting a resize.
	if err := c.mergeClusterStatus(instr.ClusterStatus); err != nil {
		return errors.Wrap(err, "merging cluster status")
	}

	c.logger.Printf("done MergeClusterStatus, start goroutine (%s)", c.Node.ID)

	// The actual resizing runs in a goroutine because we don't want to block
	// the distribution of other ResizeInstructions to the rest of the cluster.
	go func() {

		// Make sure the holder has opened.
		c.holder.opened.Recv()

		// Prepare the return message.
		complete := &ResizeInstructionComplete{
			JobID: instr.JobID,
			Node:  instr.Node,
			Error: "",
		}

		// Stop processing on any error.
		if err := func() error {
			span, ctx := tracing.StartSpanFromContext(context.Background(), "Cluster.followResizeInstruction")
			defer span.Finish()

			// Sync the NodeStatus received in the resize instruction.
			// Sync schema.
			c.logger.Debugf("holder applySchema")
			if err := c.holder.applySchema(instr.NodeStatus.Schema); err != nil {
				return errors.Wrap(err, "applying schema")
			}

			// Sync available shards.
			for _, is := range instr.NodeStatus.Indexes {
				for _, fs := range is.Fields {
					f := c.holder.Field(is.Name, fs.Name)

					// if we don't know about a field locally, log an error because
					// fields should be created and synced prior to shard creation
					if f == nil {
						c.logger.Printf("local field not found: %s/%s", is.Name, fs.Name)
						continue
					}
					if err := f.AddRemoteAvailableShards(fs.AvailableShards); err != nil {
						return errors.Wrap(err, "adding remote available shards")
					}
				}
			}

			// Request each source file in ResizeSources.
			for _, src := range instr.Sources {
				srcURI := src.Node.URI
				c.logger.Printf("get shard %d for index %s from host %s", src.Shard, src.Index, srcURI)

				// Retrieve field.
				f := c.holder.Field(src.Index, src.Field)
				if f == nil {
					return newNotFoundError(ErrFieldNotFound, src.Field)
				}

				// Create view.
				var v *view
				if err := func() (err error) {
					v, err = f.createViewIfNotExists(src.View)
					return err
				}(); err != nil {
					return errors.Wrap(err, "creating view")
				}

				// Create the local fragment.
				frag, err := v.CreateFragmentIfNotExists(src.Shard)
				if err != nil {
					return errors.Wrap(err, "creating fragment")
				}

				// Stream shard from remote node.
				c.logger.Printf("retrieve shard %d for index %s from host %s", src.Shard, src.Index, srcURI)
				rd, err := c.InternalClient.RetrieveShardFromURI(ctx, src.Index, src.Field, src.View, src.Shard, srcURI)
				if err != nil {
					// For now it is an acceptable error if the fragment is not found
					// on the remote node. This occurs when a shard has been skipped and
					// therefore doesn't contain data. The coordinator correctly determined
					// the resize instruction to retrieve the shard, but it doesn't have data.
					// TODO: figure out a way to distinguish from "fragment not found" errors
					// which are true errors and which simply mean the fragment doesn't have data.
					if err == ErrFragmentNotFound {
						continue
					}
					return errors.Wrap(err, "retrieving shard")
				} else if rd == nil {
					return fmt.Errorf("shard %v doesn't exist on host: %s", src.Shard, srcURI)
				}

				// Write to local field and always close reader.
				if err := func() error {
					defer rd.Close()
					_, err := frag.ReadFrom(rd)
					return err
				}(); err != nil {
					return errors.Wrap(err, "copying remote shard")
				}
			}

			// Request each translation source file in TranslationResizeSources.
			for _, src := range instr.TranslationSources {
				srcURI := src.Node.URI

				idx := c.holder.Index(src.Index)
				if idx == nil {
					return newNotFoundError(ErrIndexNotFound, src.Index)
				}

				// Retrieve partition from remote node.
				c.logger.Printf("retrieve translate partition %d for index %s from host %s", src.PartitionID, src.Index, srcURI)
				rd, err := c.InternalClient.RetrieveTranslatePartitionFromURI(ctx, src.Index, src.PartitionID, srcURI)
				if err != nil {
					return errors.Wrap(err, "retrieving translate partition")
				} else if rd == nil {
					return fmt.Errorf("partition %d doesn't exist on host: %s", src.PartitionID, src.Node.URI)
				}

				// Write to local store and always close reader.
				if err := func() error {
					defer rd.Close()
					// Get the translate store for this index/partition.
					store := idx.TranslateStore(src.PartitionID)
					_, err = store.ReadFrom(rd)
					return errors.Wrap(err, "reading from reader")
				}(); err != nil {
					return errors.Wrap(err, "copying remote partition")
				}
			}

			return nil
		}(); err != nil {
			complete.Error = err.Error()
		}

		if err := c.sendTo(instr.Coordinator, complete); err != nil {
			c.logger.Printf("sending resizeInstructionComplete error: err=%s", err)
		}
	}()
	return nil
}

func (c *cluster) markResizeInstructionComplete(complete *ResizeInstructionComplete) error {
	j := c.job(complete.JobID)

	// Abort the job if an error exists in the complete object.
	if complete.Error != "" {
		j.result <- resizeJobStateAborted
		return errors.New(complete.Error)
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	if j.isComplete() {
		return fmt.Errorf("resize job %d is no longer running", j.ID)
	}

	// Mark host complete.
	j.IDs[complete.Node.ID] = true

	if !j.nodesArePending() {
		j.result <- resizeJobStateDone
	}

	return nil
}

// job returns a resizeJob by id.
func (c *cluster) job(id int64) *resizeJob {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.jobs[id]
}

type resizeJob struct {
	ID           int64
	IDs          map[string]bool
	Instructions []*ResizeInstruction
	Broadcaster  broadcaster

	action string
	result chan string

	mu    sync.RWMutex
	state string

	Logger logger.Logger
}

// newResizeJob returns a new instance of resizeJob.
func newResizeJob(existingNodes []*topology.Node, node *topology.Node, action string) *resizeJob {

	// Build a map of uris to track their resize status.
	// The value for a node will be set to true after that node
	// has indicated that it has completed all resize instructions.
	ids := make(map[string]bool)

	if action == resizeJobActionRemove {
		for _, n := range existingNodes {
			// Exclude the removed node from the map.
			if n.ID == node.ID {
				continue
			}
			ids[n.ID] = false
		}
	} else if action == resizeJobActionAdd {
		for _, n := range existingNodes {
			ids[n.ID] = false
		}
		// Include the added node in the map for tracking.
		ids[node.ID] = false
	}

	return &resizeJob{
		ID:     rand.Int63(),
		IDs:    ids,
		action: action,
		result: make(chan string),
		Logger: logger.NopLogger,
	}
}

func (j *resizeJob) setState(state string) {
	j.mu.Lock()
	if j.state == "" || j.state == resizeJobStateRunning {
		j.state = state
	}
	j.mu.Unlock()
}

// run distributes ResizeInstructions.
func (j *resizeJob) run() error {
	j.Logger.Printf("run resizeJob")
	// Set job state to RUNNING.
	j.setState(resizeJobStateRunning)

	// Job can be considered done in the case where it doesn't require any action.
	if !j.nodesArePending() {
		j.Logger.Printf("resizeJob contains no pending tasks; mark as done")
		j.result <- resizeJobStateDone
		return nil
	}

	j.Logger.Printf("distribute tasks for resizeJob")
	err := j.distributeResizeInstructions()
	if err != nil {
		j.result <- resizeJobStateAborted
		return errors.Wrap(err, "distributing instructions")
	}
	return nil
}

// isComplete return true if the job is any one of several completion states.
func (j *resizeJob) isComplete() bool {
	switch j.state {
	case resizeJobStateDone, resizeJobStateAborted:
		return true
	default:
		return false
	}
}

// nodesArePending returns true if any node is still working on the resize.
func (j *resizeJob) nodesArePending() bool {
	for _, complete := range j.IDs {
		if !complete {
			return true
		}
	}
	return false
}

func (j *resizeJob) distributeResizeInstructions() error {
	j.Logger.Printf("distributeResizeInstructions for job %d", j.ID)
	// Loop through the ResizeInstructions in resizeJob and send to each host.
	for _, instr := range j.Instructions {
		// Because the node may not be in the cluster yet, create
		// a dummy node object to use in the SendTo() method.
		node := &topology.Node{
			ID:      instr.Node.ID,
			URI:     instr.Node.URI,
			GRPCURI: instr.Node.GRPCURI,
		}
		j.Logger.Printf("send resize instructions: %v", instr)
		if err := j.Broadcaster.SendTo(node, instr); err != nil {
			return errors.Wrap(err, "sending instruction")
		}
	}
	return nil
}

type nodeIDs []string

func (n nodeIDs) Len() int           { return len(n) }
func (n nodeIDs) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }
func (n nodeIDs) Less(i, j int) bool { return n[i] < n[j] }

// ContainsID returns true if id matches one of the nodesets's IDs.
func (n nodeIDs) ContainsID(id string) bool {
	for _, nid := range n {
		if nid == id {
			return true
		}
	}
	return false
}

// Topology represents the list of hosts in the cluster.
// Topology now encapsulates all knowledge needed to
// determine the primary node in the replication scheme.
type Topology struct {
	mu      sync.RWMutex
	nodeIDs []string

	clusterID string

	// nodeStates holds the state of each node according to
	// the coordinator. Used during startup and data load.
	nodeStates map[string]string

	// moved Hasher, PartitionN and ReplicaN
	// from cluster for standalone use and comprehension:

	// Hashing algorithm used to assign partitions to nodes.
	Hasher topology.Hasher
	// The number of partitions in the cluster.
	PartitionN int
	// The number of replicas a partition has.
	ReplicaN int

	// can be nil
	cluster *cluster
}

// NewTopology creates a Topology.
//
// The arguments and members hasher, partitionN, and
// replicaN were refactored out of struct cluster
// to allow pilosa-fsck to load a Topology from
// backup and then compute primaries standalone -- without starting a cluster.
// As pilosa-fsck operates on all backups at once from
// a single cpu, starting a full cluster isn't possible.
//
// The hasher is the Hashing algorithm used to assign partitions to nodes.
// The cluster c should be provided if possible by pilosa code;
// the pilosa-fsck utility won't be able to provide it.
//
// For the cluster size N, the topology gives preference to
// len(t.nodeIDs) before falling back on len(c.nodes).
//
func NewTopology(hasher topology.Hasher, partitionN int, replicaN int, c *cluster) *Topology {
	return &Topology{
		Hasher:     hasher,
		PartitionN: partitionN,
		ReplicaN:   replicaN,
		nodeStates: make(map[string]string),
		cluster:    c,
	}
}

func (t *Topology) String() string {
	return fmt.Sprintf(`
&pilosa.Topology{
		nodeIDs:    %v,
		clusterID:  %v,
		nodeStates: %v,
		PartitionN: %v,
		ReplicaN:   %v,
}
`,
		t.nodeIDs,
		t.clusterID,
		t.nodeStates,
		t.PartitionN,
		t.ReplicaN,
	)
}

///////////////////////////////////////////
// Topology implements the Noder interface.

// Nodes implements the Noder interface.
func (t *Topology) Nodes() []*topology.Node {
	nodes := make([]*topology.Node, len(t.nodeIDs))
	for i, nodeID := range t.nodeIDs {
		nodes[i] = &topology.Node{
			ID: nodeID,
		}
	}
	return nodes
}

// SetNodes implements the Noder interface.
func (t *Topology) SetNodes(nodes []*topology.Node) {}

// AppendNode implements the Noder interface.
func (t *Topology) AppendNode(node *topology.Node) {}

// RemoveNode implements the Noder interface.
func (t *Topology) RemoveNode(nodeID string) bool {
	return false
}

// SetNodeState implements the Noder interface.
func (t *Topology) SetNodeState(nodeID string, state string) {}

///////////////////////////////////////////

///////////////////////////////////////////
// Cluster implements the Noder interface.
// This is temporary and should be removed once etcd is fully implemented as
// noder.

// SetNodes implements the Noder interface.
func (c *cluster) SetNodes(nodes []*topology.Node) {}

// AppendNode implements the Noder interface.
func (c *cluster) AppendNode(node *topology.Node) {}

// RemoveNode implements the Noder interface.
func (c *cluster) RemoveNode(nodeID string) bool {
	return false
}

// SetNodeState implements the Noder interface.
func (c *cluster) SetNodeState(nodeID string, state string) {}

///////////////////////////////////////////

func (t *Topology) GetNodeIDs() []string {
	return t.nodeIDs
}

// ContainsID returns true if id matches one of the topology's IDs.
func (t *Topology) ContainsID(id string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.containsID(id)
}

func (t *Topology) containsID(id string) bool {
	return nodeIDs(t.nodeIDs).ContainsID(id)
}

func (t *Topology) positionByID(nodeID string) int {
	for i, tid := range t.nodeIDs {
		if tid == nodeID {
			return i
		}
	}
	return -1
}

// addID adds the node ID to the topology and returns true if added.
func (t *Topology) addID(nodeID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.containsID(nodeID) {
		return false
	}
	t.nodeIDs = append(t.nodeIDs, nodeID)

	sort.Slice(t.nodeIDs,
		func(i, j int) bool {
			return t.nodeIDs[i] < t.nodeIDs[j]
		})

	return true
}

// removeID removes the node ID from the topology and returns true if removed.
func (t *Topology) removeID(nodeID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	i := t.positionByID(nodeID)
	if i < 0 {
		return false
	}

	copy(t.nodeIDs[i:], t.nodeIDs[i+1:])
	t.nodeIDs[len(t.nodeIDs)-1] = ""
	t.nodeIDs = t.nodeIDs[:len(t.nodeIDs)-1]

	return true
}

// encode converts t into its internal representation.
func (t *Topology) encode() *internal.Topology {
	return encodeTopology(t)
}

// loadTopology reads the topology for the node. unprotected.
func (c *cluster) loadTopology() error {
	buf, err := ioutil.ReadFile(filepath.Join(c.Path, ".topology"))
	if os.IsNotExist(err) {
		c.Topology = NewTopology(c.Hasher, c.partitionN, c.ReplicaN, c)
		return nil
	} else if err != nil {
		return errors.Wrap(err, "reading file")
	}

	var pb internal.Topology
	if err := proto.Unmarshal(buf, &pb); err != nil {
		return errors.Wrap(err, "unmarshalling")
	}
	top, err := DecodeTopology(&pb, c.Hasher, c.partitionN, c.ReplicaN, c)
	if err != nil {
		return errors.Wrap(err, "decoding")
	}
	c.Topology = top

	return nil
}

// saveTopology writes the current topology to disk. unprotected.
func (c *cluster) saveTopology() error {
	if err := os.MkdirAll(c.Path, 0777); err != nil {
		return errors.Wrap(err, "creating directory")
	}

	if buf, err := proto.Marshal(encodeTopology(c.Topology)); err != nil {
		return errors.Wrap(err, "marshalling")
	} else if err := ioutil.WriteFile(filepath.Join(c.Path, ".topology"), buf, 0666); err != nil {
		return errors.Wrap(err, "writing file")
	}
	return nil
}

func (c *cluster) considerTopology() error {
	// Create ClusterID if one does not already exist.
	if c.id == "" {
		u := uuid.NewV4()
		c.id = u.String()
		c.Topology.clusterID = c.id
	}

	if c.Static {
		return nil
	}

	// If there is no .topology file, it's safe to proceed.
	if len(c.Topology.nodeIDs) == 0 {
		return nil
	}

	// The local node (coordinator) must be in the .topology.
	if !c.Topology.ContainsID(c.Node.ID) {
		return fmt.Errorf("coordinator %s is not in topology: %v", c.Node.ID, c.Topology.nodeIDs)
	}

	// Keep the cluster in state "STARTING" until hearing from all nodes.
	// Topology contains 2+ hosts.
	return nil
}

// band aid to protect against false nodeLeave events from memberlist
// the test is the lightest weight endpoint of the node in question /version
// TODO provide more robust solution to false nodeLeave events
func (c *cluster) confirmNodeDown(uri pnet.URI) bool {
	u := url.URL{
		Scheme: uri.Scheme,
		Host:   uri.HostPort(),
		Path:   "version",
	}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		c.logger.Printf("bad request:%s %s", u.String(), err)
		return false
	}
	for i := 0; i < c.confirmDownRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), c.confirmDownSleep*2)
		defer cancel()
		resp, err := http.DefaultClient.Do(req.WithContext(ctx))
		var bod []byte
		if err == nil {
			bod, err = ioutil.ReadAll(resp.Body)
			if resp.StatusCode == 200 {
				return false
			}
		}

		c.logger.Printf("NodeLeave confirm with %s %d. err: '%v' bod: '%s'", uri.HostPort(), i, err, bod)
		time.Sleep(c.confirmDownSleep)
	}
	return true
}

// ReceiveEvent represents an implementation of EventHandler.
func (c *cluster) ReceiveEvent(e *NodeEvent) (err error) {
	// Ignore events sent from this node.
	if e.Node.ID == c.Node.ID {
		return nil
	}
	switch e.Event {
	case NodeJoin:
		e.Node.Mu.Lock()
		c.Node.Mu.Lock()
		c.logger.Debugf("nodeJoin of %s on %s", e.Node.URI, c.Node.URI)
		c.Node.Mu.Unlock()
		e.Node.Mu.Unlock()

		// Ignore the event if this is not the coordinator.
		if !c.isCoordinator() {
			return nil
		}
		return c.nodeJoin(e.Node)
	case NodeLeave:
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.unprotectedIsCoordinator() {
			c.logger.Printf("received node leave: %v", e.Node)
			// if removeNodeBasicSorted succeeds, that means that the node was
			// not already removed by a removeNode request. We treat this as the
			// host being temporarily unavailable, and expect it to come back
			// up.
			if c.confirmNodeDown(e.Node.URI) {
				if c.removeNodeBasicSorted(e.Node.ID) {
					c.Topology.nodeStates[e.Node.ID] = nodeStateDown
					// put the cluster into STARTING if we've lost a number of nodes
					// equal to or greater than ReplicaN
					err = c.unprotectedSetStateAndBroadcast(c.determineClusterState())
				}
			} else {
				c.logger.Printf("ignored received node leave: %v", e.Node)
			}
		}
	case NodeUpdate:
		c.logger.Printf("received node update event: id: %v, string: %v, uri: %v", e.Node.ID, e.Node.String(), e.Node.URI)
		// NodeUpdate is intentionally not implemented.
	}

	return err
}

// nodeJoin should only be called by the coordinator.
func (c *cluster) nodeJoin(node *topology.Node) error {
	c.abortAntiEntropy()
	// Technically there is a race condition here which could
	// allow the anti-entropy process to re-start (and acquire
	// the lock) before this lock has time to succeed. In that
	// case, the user would have to wait through an entire
	// anti-entropy cycle. We decided it wasn't worth the
	// complexity (of, for example, implementing this with
	// channels) to avoid that rare case.
	c.muAntiEntropy.Lock()
	defer c.muAntiEntropy.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.logger.Printf("node join event on coordinator, node: %s, id: %s", node.URI, node.ID)
	if c.needTopologyAgreement() {
		// A host that is not part of the topology can't be added to the STARTING cluster.
		if !c.Topology.ContainsID(node.ID) {
			err := fmt.Sprintf("host is not in topology: %s", node.ID)
			c.logger.Printf("%v", err)
			return errors.New(err)
		}

		if err := c.addNode(node); err != nil {
			return errors.Wrap(err, "adding node for agreement")
		}

		// Only change to normal if there is no existing data. Otherwise,
		// the coordinator needs to wait to receive READY messages (nodeStates)
		// from remote nodes before setting the cluster to state NORMAL.
		if ok, err := c.holder.HasData(); !ok && err == nil {
			// If the result of the previous AddNode completed the joining of nodes
			// in the topology, then change the state to NORMAL.
			if c.haveTopologyAgreement() {
				return c.unprotectedSetStateAndBroadcast(ClusterStateNormal)
			}
			// This lets the remote node to proceed with opening its holder,
			// instead of waiting in DOWN state because cluster is in STARTING state.
			return c.sendTo(node, c.unprotectedStatus())
		} else if err != nil {
			return errors.Wrap(err, "checking if holder has data")
		}

		if c.haveTopologyAgreement() && c.allNodesReady() {
			return c.unprotectedSetStateAndBroadcast(ClusterStateNormal)
		}
		// Send the status to the remote node. This lets the remote node
		// know that it can proceed with opening its Holder.
		return c.sendTo(node, c.unprotectedStatus())
	}

	// If the cluster already contains the node, just send it the cluster status.
	// This is useful in the case where a node is restarted or temporarily leaves
	// the cluster.
	if cnode := c.unprotectedNodeByID(node.ID); cnode != nil {
		if cnode.URI != node.URI {
			c.logger.Printf("node: %v changed URI from %s to %s", cnode.ID, cnode.URI, node.URI)
			cnode.URI = node.URI
		}
		if cnode.GRPCURI != node.GRPCURI {
			cnode.GRPCURI = node.GRPCURI
		}
		return c.unprotectedSetStateAndBroadcast(c.determineClusterState())
	}

	// If the holder does not yet contain data, go ahead and add the node.
	if ok, err := c.holder.HasData(); !ok && err == nil {
		if err := c.addNode(node); err != nil {
			return errors.Wrap(err, "adding node")
		}
		return c.unprotectedSetStateAndBroadcast(ClusterStateNormal)
	} else if err != nil {
		return errors.Wrap(err, "checking if holder has data2")
	}

	// If the cluster has data, we need to change to RESIZING and
	// kick off the resizing process.
	if err := c.unprotectedSetStateAndBroadcast(ClusterStateResizing); err != nil {
		return errors.Wrap(err, "broadcasting state")
	}
	c.joiningLeavingNodes <- nodeAction{node, resizeJobActionAdd}

	return nil
}

// nodeLeave initiates the removal of a node from the cluster.
func (c *cluster) nodeLeave(nodeID string) error {
	c.abortAntiEntropy()
	// Technically there is a race condition here which could
	// allow the anti-entropy process to re-start (and acquire
	// the lock) before this lock has time to succeed. In that
	// case, the user would have to wait through an entire
	// anti-entropy cycle. We decided it wasn't worth the
	// complexity (of, for example, implementing this with
	// channels) to avoid that rare case.
	c.muAntiEntropy.Lock()
	defer c.muAntiEntropy.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	// Refuse the request if this is not the coordinator.
	if !c.unprotectedIsCoordinator() {
		return fmt.Errorf("node removal requests are only valid on the coordinator node: %s",
			c.unprotectedCoordinatorNode().ID)
	}

	if c.state != ClusterStateNormal && c.state != ClusterStateDegraded {
		return fmt.Errorf("cluster must be '%s' or '%s' to remove a node but is '%s'",
			ClusterStateNormal, ClusterStateDegraded, c.state)
	}

	// Ensure that node is in the cluster.
	if !c.topologyContainsNode(nodeID) {
		return fmt.Errorf("Node is not a member of the cluster: %s", nodeID)
	}

	// Prevent removing the coordinator node (this node).
	if nodeID == c.Node.ID {
		return fmt.Errorf("coordinator cannot be removed; first, make a different node the new coordinator")
	}

	// See if resize job can be generated
	if _, err := c.unprotectedGenerateResizeJobByAction(
		nodeAction{
			node:   &topology.Node{ID: nodeID},
			action: resizeJobActionRemove},
	); err != nil {
		return errors.Wrap(err, "generating job")
	}

	// If the holder does not yet contain data, go ahead and remove the node.
	if ok, err := c.holder.HasData(); !ok && err == nil {
		if err := c.removeNode(nodeID); err != nil {
			return errors.Wrap(err, "removing node")
		}
		return c.unprotectedSetStateAndBroadcast(c.determineClusterState())
	} else if err != nil {
		return errors.Wrap(err, "checking if holder has data")
	}

	// If the cluster has data then change state to RESIZING and
	// kick off the resizing process.
	if err := c.unprotectedSetStateAndBroadcast(ClusterStateResizing); err != nil {
		return errors.Wrap(err, "broadcasting state")
	}
	c.joiningLeavingNodes <- nodeAction{node: &topology.Node{ID: nodeID}, action: resizeJobActionRemove}

	return nil
}

func (c *cluster) nodeStatus() *NodeStatus {
	ns := &NodeStatus{
		Node:   c.Node,
		Schema: &Schema{Indexes: c.holder.Schema()},
	}
	var availableShards *roaring.Bitmap
	for _, idx := range ns.Schema.Indexes {
		is := &IndexStatus{Name: idx.Name, CreatedAt: idx.CreatedAt}
		for _, f := range idx.Fields {
			if field := c.holder.Field(idx.Name, f.Name); field != nil {
				availableShards = field.AvailableShards(includeRemote)
			} else {
				availableShards = roaring.NewBitmap()
			}
			is.Fields = append(is.Fields, &FieldStatus{
				Name:            f.Name,
				CreatedAt:       f.CreatedAt,
				AvailableShards: availableShards,
			})
		}
		ns.Indexes = append(ns.Indexes, is)
	}
	return ns
}

func (c *cluster) mergeClusterStatus(cs *ClusterStatus) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logger.Printf("merge cluster status: node=%s cluster=%v, topologySize=%v", c.Node.ID, cs, len(c.Topology.nodeIDs))
	// Ignore status updates from self (coordinator).
	if c.unprotectedIsCoordinator() {
		return nil
	}

	// Set ClusterID.
	c.unprotectedSetID(cs.ClusterID)

	officialNodes := cs.Nodes

	// Add all nodes from the coordinator.
	for _, node := range officialNodes {
		if err := c.addNode(node); err != nil {
			return errors.Wrap(err, "adding node")
		}
	}

	// Remove any nodes not specified by the coordinator
	// except for self. Generate a list to remove first
	// so that nodes aren't removed mid-loop.
	nodeIDsToRemove := []string{}
	for _, node := range c.noder.Nodes() {
		// Don't remove this node.
		if node.ID == c.Node.ID {
			continue
		}
		if topology.Nodes(officialNodes).ContainsID(node.ID) {
			continue
		}
		nodeIDsToRemove = append(nodeIDsToRemove, node.ID)
	}

	for _, nodeID := range nodeIDsToRemove {
		if err := c.removeNode(nodeID); err != nil {
			return errors.Wrap(err, "removing node")
		}
	}

	c.unprotectedSetState(cs.State)

	c.markAsJoined()

	return nil
}

// unprotectedPreviousNode returns the node listed before the current node in c.Nodes.
// If there is only one node in the cluster, returns nil.
// If the current node is the first node in the list, returns the last node.
func (c *cluster) unprotectedPreviousNode() *topology.Node {
	cNodes := c.noder.Nodes()
	if len(cNodes) <= 1 {
		return nil
	}

	pos := c.nodePositionByID(c.Node.ID)
	if pos == -1 {
		return nil
	} else if pos == 0 {
		return cNodes[len(cNodes)-1]
	} else {
		return cNodes[pos-1]
	}
}

// PrimaryReplicaNode returns the node listed before the current node in c.Nodes.
// This is different than "previous node" as the first node always returns nil.
func (c *cluster) PrimaryReplicaNode() *topology.Node {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.unprotectedPrimaryReplicaNode()
}

func (c *cluster) unprotectedPrimaryReplicaNode() *topology.Node {
	pos := c.nodePositionByID(c.Node.ID)
	if pos <= 0 {
		return nil
	}
	cNodes := c.noder.Nodes()
	return cNodes[pos-1]
}

// translateFieldKeys is basically a wrapper around
// field.TranslateStore().TranslateKey(key), but in
// the case where the local node is not coordinator, then this method will forward the translation
// request to the coordinator.
func (c *cluster) translateFieldKeys(ctx context.Context, field *Field, keys []string, writable bool) (ids []uint64, err error) {
	// Create a snapshot of the cluster to use for node/partition calculations.
	snap := topology.NewClusterSnapshot(c.noder, c.Hasher, c.ReplicaN)

	primary := snap.PrimaryFieldTranslationNode()
	if primary == nil {
		return nil, errors.Errorf("translating field(%s/%s) keys(%v) - cannot find coordinator node", field.Index(), field.Name(), keys)
	}

	if c.Node.ID == primary.ID {
		ids, err = field.TranslateStore().TranslateKeys(keys, writable)
	} else {
		// If it's writable, then forward the request to the coordinator.
		ids, err = c.InternalClient.TranslateKeysNode(ctx, &primary.URI, field.Index(), field.Name(), keys, writable)
	}

	if err != nil {
		return nil, errors.Wrapf(err, "translating field(%s/%s) keys(%v)", field.Index(), field.Name(), keys)
	}

	return ids, nil
}

func (c *cluster) findFieldKeys(ctx context.Context, field *Field, keys ...string) (map[string]uint64, error) {
	if idx := field.ForeignIndex(); idx != "" {
		// The field uses foreign index keys.
		// Therefore, the field keys are actually column keys on a different index.
		return c.findIndexKeys(ctx, idx, keys...)
	}

	if !field.Keys() {
		return nil, errors.Wrap(ErrTranslatingKeyNotFound, "field is not keyed")
	}

	// Attempt to find the keys locally.
	localTranslations, err := field.TranslateStore().FindKeys(keys...)
	if err != nil {
		return nil, errors.Wrapf(err, "translating field(%s/%s) keys(%v) locally", field.Index(), field.Name(), keys)
	}

	// Check for missing keys.
	var missing []string
	if len(keys) > len(localTranslations) {
		// There are either duplicate keys or missing keys.
		// This should work either way.
		missing = make([]string, 0, len(keys)-len(localTranslations))
		for _, k := range keys {
			_, found := localTranslations[k]
			if !found {
				missing = append(missing, k)
			}
		}
	} else if len(localTranslations) > len(keys) {
		panic(fmt.Sprintf("more translations than keys! translation count=%v, key count=%v", len(localTranslations), len(keys)))
	}
	if len(missing) == 0 {
		// All keys were available locally.
		return localTranslations, nil
	}

	// It is possible that the missing keys exist, but have not been synced to the local replica.
	coordinator := c.coordinatorNode()
	if coordinator == nil {
		return nil, errors.Errorf("translating field(%s/%s) keys(%v) - cannot find coordinator node", field.Index(), field.Name(), keys)
	}
	if c.Node.ID == coordinator.ID {
		// The local copy is the authoritative copy.
		return localTranslations, nil
	}

	// Forward the missing keys to the coordinator.
	// The coordinator has the authoritative copy.
	remoteTranslations, err := c.InternalClient.FindFieldKeysNode(ctx, &coordinator.URI, field.Index(), field.Name(), missing...)
	if err != nil {
		return nil, errors.Wrapf(err, "translating field(%s/%s) keys(%v) remotely", field.Index(), field.Name(), keys)
	}

	// Merge the remote translations into the local translations.
	translations := localTranslations
	for key, id := range remoteTranslations {
		translations[key] = id
	}

	return translations, nil
}

func (c *cluster) createFieldKeys(ctx context.Context, field *Field, keys ...string) (map[string]uint64, error) {
	if idx := field.ForeignIndex(); idx != "" {
		// The field uses foreign index keys.
		// Therefore, the field keys are actually column keys on a different index.
		return c.createIndexKeys(ctx, idx, keys...)
	}

	if !field.Keys() {
		return nil, errors.Wrap(ErrTranslatingKeyNotFound, "field is not keyed")
	}

	// The coordinator is the only node that can create field keys, since it owns the authoritative copy.
	coordinator := c.coordinatorNode()
	if coordinator == nil {
		return nil, errors.Errorf("translating field(%s/%s) keys(%v) - cannot find coordinator node", field.Index(), field.Name(), keys)
	}
	if c.Node.ID == coordinator.ID {
		// The local copy is the authoritative copy.
		return field.TranslateStore().CreateKeys(keys...)
	}

	// Attempt to find the keys locally.
	// They cannot be created locally, but skipping keys that exist can reduce network usage.
	localTranslations, err := field.TranslateStore().FindKeys(keys...)
	if err != nil {
		return nil, errors.Wrapf(err, "translating field(%s/%s) keys(%v) locally", field.Index(), field.Name(), keys)
	}

	// Check for missing keys.
	var missing []string
	if len(keys) > len(localTranslations) {
		// There are either duplicate keys or missing keys.
		// This should work either way.
		missing = make([]string, 0, len(keys)-len(localTranslations))
		for _, k := range keys {
			_, found := localTranslations[k]
			if !found {
				missing = append(missing, k)
			}
		}
	} else if len(localTranslations) > len(keys) {
		panic(fmt.Sprintf("more translations than keys! translation count=%v, key count=%v", len(localTranslations), len(keys)))
	}
	if len(missing) == 0 {
		// All keys exist locally.
		// There is no need to create anything.
		return localTranslations, nil
	}

	// Forward the missing keys to the coordinator to be created.
	remoteTranslations, err := c.InternalClient.CreateFieldKeysNode(ctx, &coordinator.URI, field.Index(), field.Name(), missing...)
	if err != nil {
		return nil, errors.Wrapf(err, "translating field(%s/%s) keys(%v) remotely", field.Index(), field.Name(), keys)
	}

	// Merge the remote translations into the local translations.
	translations := localTranslations
	for key, id := range remoteTranslations {
		translations[key] = id
	}

	return translations, nil
}

func (c *cluster) translateFieldIDs(field *Field, ids map[uint64]struct{}) (map[uint64]string, error) {
	idList := make([]uint64, len(ids))
	{
		i := 0
		for id := range ids {
			idList[i] = id
			i++
		}
	}

	keyList, err := c.translateFieldListIDs(field, idList)
	if err != nil {
		return nil, err
	}

	mapped := make(map[uint64]string, len(idList))
	for i, key := range keyList {
		mapped[idList[i]] = key
	}
	return mapped, nil
}

func (c *cluster) translateFieldListIDs(field *Field, ids []uint64) (keys []string, err error) {
	// Create a snapshot of the cluster to use for node/partition calculations.
	snap := topology.NewClusterSnapshot(c.noder, c.Hasher, c.ReplicaN)

	primary := snap.PrimaryFieldTranslationNode()
	if primary == nil {
		return nil, errors.Errorf("translating field(%s/%s) ids(%v) - cannot find coordinator node", field.Index(), field.Name(), ids)
	}

	if c.Node.ID == primary.ID {
		keys, err = field.TranslateStore().TranslateIDs(ids)
	} else {
		keys, err = c.InternalClient.TranslateIDsNode(context.Background(), &primary.URI, field.Index(), field.Name(), ids)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "translating field(%s/%s) ids(%v)", field.Index(), field.Name(), ids)
	}

	return keys, err
}

func (c *cluster) translateIndexKey(ctx context.Context, indexName string, key string, writable bool) (uint64, error) {
	keyMap, err := c.translateIndexKeySet(ctx, indexName, map[string]struct{}{key: struct{}{}}, writable)
	if err != nil {
		return 0, err
	}
	return keyMap[key], nil
}

func (c *cluster) translateIndexKeys(ctx context.Context, indexName string, keys []string, writable bool) ([]uint64, error) {
	keySet := make(map[string]struct{})
	for _, key := range keys {
		keySet[key] = struct{}{}
	}

	keyMap, err := c.translateIndexKeySet(ctx, indexName, keySet, writable)
	if err != nil {
		return nil, err
	}

	// make sure that ids line up with keys, but
	// not appending, but assigning directly 1:1 into the slice.
	ids := make([]uint64, len(keys))
	for i, k := range keys {
		id, ok := keyMap[k]
		if !writable {
			if !ok || id == 0 {
				c.holder.Logger.Debugf("internal translateIndexKeys error: keyMap had no entry for k='%v', and was not writable", k)
				return nil, ErrTranslatingKeyNotFound
			}
		}
		ids[i] = id
	}
	return ids, nil
}

// The boltdb key translation stores are partitioned, designated by partitionIDs. These
// are shared between replicas, and one node is the primary for
// replication. So with 4 nodes and 3-way replication, each node has 3/4 of
// the translation stores on it.
func (t *Topology) GetPrimaryForColKeyTranslation(index, key string) (primary int) {
	partitionID := t.KeyPartition(index, key)
	return t.PrimaryNodeIndex(partitionID)
}

// should match cluster.go:1033 cluster.ownsShard(nodeID, index, shard)
// 	return Nodes(c.shardNodes(index, shard)).ContainsID(nodeID)
func (t *Topology) GetPrimaryForShardReplication(index string, shard uint64) int {
	n := len(t.nodeIDs)
	if n == 0 {
		return -1
	}
	partition := uint64(shardToShardPartition(index, shard, t.PartitionN))
	nodeIndex := t.Hasher.Hash(partition, n)
	return nodeIndex
}

func (c *cluster) translateIndexKeySet(ctx context.Context, indexName string, keySet map[string]struct{}, writable bool) (map[string]uint64, error) {
	keyMap := make(map[string]uint64)

	idx := c.holder.Index(indexName)
	if idx == nil {
		return nil, ErrIndexNotFound
	}

	// Create a snapshot of the cluster to use for node/partition calculations.
	snap := topology.NewClusterSnapshot(c.noder, c.Hasher, c.ReplicaN)

	// Split keys by partition.
	keysByPartition := make(map[int][]string, c.partitionN)
	for key := range keySet {
		partitionID := snap.KeyToKeyPartition(indexName, key)
		keysByPartition[partitionID] = append(keysByPartition[partitionID], key)
	}

	// Translate keys by partition.
	var g errgroup.Group
	var mu sync.Mutex
	for partitionID := range keysByPartition {
		partitionID := partitionID
		keys := keysByPartition[partitionID]

		g.Go(func() (err error) {
			var ids []uint64

			primary := snap.PrimaryPartitionNode(partitionID)
			if primary == nil {
				return errors.Errorf("translating index(%s) keys(%v) on partition(%d) - cannot find primary node", indexName, keys, partitionID)
			}

			if c.Node.ID == primary.ID {
				ids, err = idx.TranslateStore(partitionID).TranslateKeys(keys, writable)
			} else {
				ids, err = c.InternalClient.TranslateKeysNode(ctx, &primary.URI, indexName, "", keys, writable)
			}

			if err != nil {
				return errors.Wrapf(err, "translating index(%s) keys(%v) on partition(%d)", indexName, keys, partitionID)
			}

			mu.Lock()
			for i, id := range ids {
				if id != 0 {
					keyMap[keys[i]] = id
				}
			}
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return keyMap, nil
}

func (c *cluster) findIndexKeys(ctx context.Context, indexName string, keys ...string) (map[string]uint64, error) {
	done := ctx.Done()

	idx := c.holder.Index(indexName)
	if idx == nil {
		return nil, ErrIndexNotFound
	}

	// Split keys by partition.
	keysByPartition := make(map[int][]string, c.partitionN)
	for _, key := range keys {
		partitionID := c.Topology.KeyPartition(indexName, key)
		keysByPartition[partitionID] = append(keysByPartition[partitionID], key)
	}

	// TODO: use local replicas to short-circuit network traffic

	// Group keys by node.
	keysByNode := make(map[*topology.Node][]string)
	for partitionID, keys := range keysByPartition {
		// Find the primary node for this partition.
		primary := c.primaryPartitionNode(partitionID)
		if primary == nil {
			return nil, errors.Errorf("translating index(%s) keys(%v) on partition(%d) - cannot find primary node", indexName, keys, partitionID)
		}

		if c.Node.ID == primary.ID {
			// The partition is local.
			continue
		}

		// Group the partition to be processed remotely.
		keysByNode[primary] = append(keysByNode[primary], keys...)

		// Delete remote keys from the by-partition map so that it can be used for local translation.
		delete(keysByPartition, partitionID)
	}

	// Start translating keys remotely.
	// On child calls, there are no remote results since we were only sent the keys that we own.
	remoteResults := make(chan map[string]uint64, len(keysByNode))
	var g errgroup.Group
	defer g.Wait() //nolint:errcheck
	for node, keys := range keysByNode {
		node, keys := node, keys

		g.Go(func() error {
			translations, err := c.InternalClient.FindIndexKeysNode(ctx, &node.URI, indexName, keys...)
			if err != nil {
				return errors.Wrapf(err, "translating index(%s) keys(%v) on node %s", indexName, keys, node.ID)
			}

			remoteResults <- translations
			return nil
		})
	}

	// Translate local keys.
	translations := make(map[string]uint64)
	for partitionID, keys := range keysByPartition {
		// Handle cancellation.
		select {
		case <-done:
			return nil, ctx.Err()
		default:
		}

		// Find the keys within the partition.
		t, err := idx.TranslateStore(partitionID).FindKeys(keys...)
		if err != nil {
			return nil, errors.Wrapf(err, "translating index(%s) keys(%v) on partition(%d)", idx.Name(), keys, partitionID)
		}

		// Merge the translations from this partition.
		for key, id := range t {
			translations[key] = id
		}
	}

	// Wait for remote key sets.
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Merge the translations.
	// All data should have been written to here while we waited.
	// Closing the channel prevents the range from blocking.
	close(remoteResults)
	for t := range remoteResults {
		for key, id := range t {
			translations[key] = id
		}
	}
	return translations, nil
}

func (c *cluster) createIndexKeys(ctx context.Context, indexName string, keys ...string) (map[string]uint64, error) {
	// Check for early cancellation.
	done := ctx.Done()
	select {
	case <-done:
		return nil, ctx.Err()
	default:
	}

	idx := c.holder.Index(indexName)
	if idx == nil {
		return nil, ErrIndexNotFound
	}

	if !idx.keys {
		return nil, errors.Errorf("can't create index keys on unkeyed index %s", indexName)
	}

	// Split keys by partition.
	keysByPartition := make(map[int][]string, c.partitionN)
	for _, key := range keys {
		partitionID := c.Topology.KeyPartition(indexName, key)
		keysByPartition[partitionID] = append(keysByPartition[partitionID], key)
	}

	// TODO: use local replicas to short-circuit network traffic

	// Group keys by node.
	// Delete remote keys from the by-partition map so that it can be used for local translation.
	keysByNode := make(map[*topology.Node][]string)
	for partitionID, keys := range keysByPartition {
		// Find the primary node for this partition.
		primary := c.primaryPartitionNode(partitionID)
		if primary == nil {
			return nil, errors.Errorf("translating index(%s) keys(%v) on partition(%d) - cannot find primary node", indexName, keys, partitionID)
		}

		if c.Node.ID == primary.ID {
			// The partition is local.
			continue
		}

		// Group the partition to be processed remotely.
		keysByNode[primary] = append(keysByNode[primary], keys...)
		delete(keysByPartition, partitionID)
	}

	translateResults := make(chan map[string]uint64, len(keysByNode)+len(keysByPartition))
	var g errgroup.Group
	defer g.Wait() //nolint:errcheck

	// Start translating keys remotely.
	// On child calls, there are no remote results since we were only sent the keys that we own.
	for node, keys := range keysByNode {
		node, keys := node, keys

		g.Go(func() error {
			translations, err := c.InternalClient.CreateIndexKeysNode(ctx, &node.URI, indexName, keys...)
			if err != nil {
				return errors.Wrapf(err, "translating index(%s) keys(%v) on node %s", indexName, keys, node.ID)
			}

			translateResults <- translations
			return nil
		})
	}

	// Translate local keys.
	// TODO: make this less horrible (why fsync why?????)
	// 		This is kinda terrible because each goroutine does an fsync, thus locking up an entire OS thread.
	// 		AHHHHHHHHHHHHHHHHHH
	for partitionID, keys := range keysByPartition {
		partitionID, keys := partitionID, keys

		g.Go(func() error {
			// Handle cancellation.
			select {
			case <-done:
				return ctx.Err()
			default:
			}

			translations, err := idx.TranslateStore(partitionID).CreateKeys(keys...)
			if err != nil {
				return errors.Wrapf(err, "translating index(%s) keys(%v) on partition(%d)", idx.Name(), keys, partitionID)
			}

			translateResults <- translations
			return nil
		})
	}

	// Wait for remote key sets.
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Merge the translations.
	// All data should have been written to here while we waited.
	// Closing the channel prevents the range from blocking.
	translations := make(map[string]uint64, len(keys))
	close(translateResults)
	for t := range translateResults {
		for key, id := range t {
			translations[key] = id
		}
	}
	return translations, nil
}

func (c *cluster) translateIndexIDs(ctx context.Context, indexName string, ids []uint64) ([]string, error) {
	idSet := make(map[uint64]struct{})
	for _, id := range ids {
		idSet[id] = struct{}{}
	}

	idMap, err := c.translateIndexIDSet(ctx, indexName, idSet)
	if err != nil {
		return nil, err
	}

	keys := make([]string, len(ids))
	for i := range ids {
		keys[i] = idMap[ids[i]]
	}
	return keys, nil
}

func (c *cluster) translateIndexIDSet(ctx context.Context, indexName string, idSet map[uint64]struct{}) (map[uint64]string, error) {
	idMap := make(map[uint64]string, len(idSet))

	index := c.holder.Index(indexName)
	if index == nil {
		return nil, newNotFoundError(ErrIndexNotFound, indexName)
	}

	// Create a snapshot of the cluster to use for node/partition calculations.
	snap := topology.NewClusterSnapshot(c.noder, c.Hasher, c.ReplicaN)

	// Split ids by partition.
	idsByPartition := make(map[int][]uint64, c.partitionN)
	for id := range idSet {
		partitionID := snap.IDToShardPartition(indexName, id)
		idsByPartition[partitionID] = append(idsByPartition[partitionID], id)
	}

	// Translate ids by partition.
	var g errgroup.Group
	var mu sync.Mutex
	for partitionID := range idsByPartition {
		partitionID := partitionID
		ids := idsByPartition[partitionID]

		g.Go(func() (err error) {
			var keys []string

			primary := snap.PrimaryPartitionNode(partitionID)
			if primary == nil {
				return errors.Errorf("translating index(%s) ids(%v) on partition(%d) - cannot find primary node", indexName, ids, partitionID)
			}

			if c.Node.ID == primary.ID {
				keys, err = index.TranslateStore(partitionID).TranslateIDs(ids)
			} else {
				keys, err = c.InternalClient.TranslateIDsNode(ctx, &primary.URI, indexName, "", ids)
			}

			if err != nil {
				return errors.Wrapf(err, "translating index(%s) ids(%v) on partition(%d)", indexName, ids, partitionID)
			}

			mu.Lock()
			for i, id := range ids {
				idMap[id] = keys[i]
			}
			mu.Unlock()

			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return idMap, nil
}

// ClusterStatus describes the status of the cluster including its
// state and node topology.
type ClusterStatus struct {
	ClusterID string
	State     string
	Nodes     []*topology.Node
	Schema    *Schema
}

// ResizeInstruction contains the instruction provided to a node
// during a cluster resize operation.
type ResizeInstruction struct {
	JobID              int64
	Node               *topology.Node
	Coordinator        *topology.Node
	Sources            []*ResizeSource
	TranslationSources []*TranslationResizeSource
	NodeStatus         *NodeStatus
	ClusterStatus      *ClusterStatus
}

// ResizeSource is the source of data for a node acting on a
// ResizeInstruction.
type ResizeSource struct {
	Node  *topology.Node `protobuf:"bytes,1,opt,name=Node" json:"Node,omitempty"`
	Index string         `protobuf:"bytes,2,opt,name=Index,proto3" json:"Index,omitempty"`
	Field string         `protobuf:"bytes,3,opt,name=Field,proto3" json:"Field,omitempty"`
	View  string         `protobuf:"bytes,4,opt,name=View,proto3" json:"View,omitempty"`
	Shard uint64         `protobuf:"varint,5,opt,name=Shard,proto3" json:"Shard,omitempty"`
}

// TranslationResizeSource is the source of translation data for
// a node acting on a ResizeInstruction.
type TranslationResizeSource struct {
	Node        *topology.Node
	Index       string
	PartitionID int
}

// translateResizeNode holds the node/partition pairs used
// to create a TranslationResizeSource for each index.
type translationResizeNode struct {
	node        *topology.Node
	partitionID int
}

// Schema contains information about indexes and their configuration.
type Schema struct {
	Indexes []*IndexInfo `json:"indexes"`
}

func encodeTopology(topology *Topology) *internal.Topology {
	if topology == nil {
		return nil
	}
	return &internal.Topology{
		ClusterID: topology.clusterID,
		NodeIDs:   topology.nodeIDs,
	}
}

// the cluster c is optional but give it if you have it.
func DecodeTopology(topology *internal.Topology, hasher topology.Hasher, partitionN, replicaN int, c *cluster) (*Topology, error) {
	if topology == nil {
		return nil, nil
	}

	t := NewTopology(hasher, partitionN, replicaN, c)
	t.clusterID = topology.ClusterID
	t.nodeIDs = topology.NodeIDs
	sort.Slice(t.nodeIDs,
		func(i, j int) bool {
			return t.nodeIDs[i] < t.nodeIDs[j]
		})

	return t, nil
}

// CreateShardMessage is an internal message indicating shard creation.
type CreateShardMessage struct {
	Index string
	Field string
	Shard uint64
}

// CreateIndexMessage is an internal message indicating index creation.
type CreateIndexMessage struct {
	Index     string
	CreatedAt int64
	Meta      *IndexOptions
}

// DeleteIndexMessage is an internal message indicating index deletion.
type DeleteIndexMessage struct {
	Index string
}

// CreateFieldMessage is an internal message indicating field creation.
type CreateFieldMessage struct {
	Index     string
	Field     string
	CreatedAt int64
	Meta      *FieldOptions
}

// DeleteFieldMessage is an internal message indicating field deletion.
type DeleteFieldMessage struct {
	Index string
	Field string
}

// DeleteAvailableShardMessage is an internal message indicating available shard deletion.
type DeleteAvailableShardMessage struct {
	Index   string
	Field   string
	ShardID uint64
}

// CreateViewMessage is an internal message indicating view creation.
type CreateViewMessage struct {
	Index string
	Field string
	View  string
}

// DeleteViewMessage is an internal message indicating view deletion.
type DeleteViewMessage struct {
	Index string
	Field string
	View  string
}

// ResizeInstructionComplete is an internal message to the coordinator indicating
// that the resize instructions performed on a single node have completed.
type ResizeInstructionComplete struct {
	JobID int64
	Node  *topology.Node
	Error string
}

// SetCoordinatorMessage is an internal message instructing nodes to honor a new coordinator.
type SetCoordinatorMessage struct {
	New *topology.Node
}

// UpdateCoordinatorMessage is an internal message for reassigning the coordinator.
type UpdateCoordinatorMessage struct {
	New *topology.Node
}

// NodeStateMessage is an internal message for broadcasting a node's state.
type NodeStateMessage struct {
	NodeID string `protobuf:"bytes,1,opt,name=NodeID,proto3" json:"NodeID,omitempty"`
	State  string `protobuf:"bytes,2,opt,name=State,proto3" json:"State,omitempty"`
}

// NodeStatus is an internal message representing the contents of a node.
type NodeStatus struct {
	Node    *topology.Node
	Indexes []*IndexStatus
	Schema  *Schema
}

// IndexStatus is an internal message representing the contents of an index.
type IndexStatus struct {
	Name      string
	CreatedAt int64
	Fields    []*FieldStatus
}

// FieldStatus is an internal message representing the contents of a field.
type FieldStatus struct {
	Name            string
	CreatedAt       int64
	AvailableShards *roaring.Bitmap
}

// RecalculateCaches is an internal message for recalculating all caches
// within a holder.
type RecalculateCaches struct{}

// Transaction Actions
const (
	TRANSACTION_START    = "start"
	TRANSACTION_FINISH   = "finish"
	TRANSACTION_VALIDATE = "validate"
)

type TransactionMessage struct {
	Transaction *Transaction
	Action      string
}
