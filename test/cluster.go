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

package test

import (
	"context"
	"fmt"
	"io/ioutil"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pilosa/pilosa/v2"
	"github.com/pilosa/pilosa/v2/server"
	"github.com/pkg/errors"
)

// modHasher represents a simple, mod-based hashing.
type ModHasher struct{}

func (*ModHasher) Hash(key uint64, n int) int { return int(key) % n }

// Cluster represents a Pilosa cluster (multiple Command instances)
type Cluster []*Command

// Query executes an API.Query through one of the cluster's node's API. It fails
// the test if there is an error.
func (c Cluster) Query(t testing.TB, index, query string) pilosa.QueryResponse {
	t.Helper()
	if len(c) == 0 {
		t.Fatal("must have at least one node in cluster to query")
	}

	return c[0].QueryAPI(t, &pilosa.QueryRequest{Index: index, Query: query})
}

func (c Cluster) ImportBits(t testing.TB, index, field string, rowcols [][2]uint64) {
	t.Helper()
	byShard := make(map[uint64][][2]uint64)
	for _, rowcol := range rowcols {
		shard := rowcol[1] / pilosa.ShardWidth
		byShard[shard] = append(byShard[shard], rowcol)
	}

	for shard, bits := range byShard {
		rowIDs := make([]uint64, len(bits))
		colIDs := make([]uint64, len(bits))
		for i, bit := range bits {
			rowIDs[i] = bit[0]
			colIDs[i] = bit[1]
		}
		nodes, err := c[0].API.ShardNodes(context.Background(), index, shard)
		if err != nil {
			t.Fatalf("getting shard nodes: %v", err)
		}
		// TODO won't be necessary to do all nodes once that works hits
		// (travis) this TODO is not clear to me, but I think it's
		// suggesting that elsewhere we would support importing to a
		// single node, regardless of where the data ends up.
		for _, node := range nodes {
			for _, com := range c {
				if com.API.Node().ID != node.ID {
					continue
				}
				err := com.API.Import(context.Background(), &pilosa.ImportRequest{
					Index:     index,
					Field:     field,
					Shard:     shard,
					RowIDs:    rowIDs,
					ColumnIDs: colIDs,
				})
				if err != nil {
					t.Fatalf("importing data: %v", err)
				}
			}
		}
	}
}

// CreateField creates the index (if necessary) and field specified.
func (c Cluster) CreateField(t testing.TB, index string, iopts pilosa.IndexOptions, field string, fopts ...pilosa.FieldOption) *pilosa.Field {
	t.Helper()
	idx, err := c[0].API.CreateIndex(context.Background(), index, iopts)
	if err != nil && !strings.Contains(err.Error(), "index already exists") {
		t.Fatalf("creating index: %v", err)
	} else if err != nil { // index exists
		idx, err = c[0].API.Index(context.Background(), index)
		if err != nil {
			t.Fatalf("getting index: %v", err)
		}
	}
	if idx.Options() != iopts {
		t.Logf("existing index options:\n%v\ndon't match given opts:\n%v\n in pilosa/test.Cluster.CreateField", idx.Options(), iopts)
	}

	f, err := c[0].API.CreateField(context.Background(), index, field, fopts...)
	// we'll assume the field doesn't exist because checking if the options
	// match seems painful.
	if err != nil {
		t.Fatalf("creating field: %v", err)
	}
	return f
}

// Start runs a Cluster
func (c Cluster) Start() error {
	var gossipSeeds = make([]string, len(c))
	for i, cc := range c {
		cc.Config.Gossip.Port = "0"
		cc.Config.Gossip.Seeds = gossipSeeds[:i]
		if err := cc.Start(); err != nil {
			return errors.Wrapf(err, "starting server %d", i)
		}
		gossipSeeds[i] = cc.GossipAddress()
	}
	return nil
}

// Stop stops a Cluster
func (c Cluster) Close() error {
	for i, cc := range c {
		if err := cc.Close(); err != nil {
			return errors.Wrapf(err, "stopping server %d", i)
		}
	}
	return nil
}

// AwaitState waits for the cluster coordinator (assumed to be the first
// node) to reach a specified state.
func (c Cluster) AwaitCoordinatorState(expectedState string, timeout time.Duration) error {
	if len(c) < 1 {
		return errors.New("can't await coordinator state on an empty cluster")
	}
	return c[:1].AwaitState(expectedState, timeout)
}

// ExceptionalState returns an error if any node in the cluster is not
// in the expected state.
func (c Cluster) ExceptionalState(expectedState string) error {
	for _, node := range c {
		state := node.API.State()
		if state != expectedState {
			return fmt.Errorf("node %q: state %s", node.ID(), state)
		}
	}
	return nil
}

// AwaitState waits for the whole cluster to reach a specified state.
func (c Cluster) AwaitState(expectedState string, timeout time.Duration) (err error) {
	if len(c) < 1 {
		return errors.New("can't await state of an empty cluster")
	}
	startTime := time.Now()
	var elapsed time.Duration
	for elapsed = 0; elapsed <= timeout; elapsed = time.Since(startTime) {
		// Counterintuitive: We're returning if the err *is* nil,
		// meaning we've reached the expected state.
		if err = c.ExceptionalState(expectedState); err == nil {
			return err
		}
		time.Sleep(1 * time.Millisecond)
	}
	return fmt.Errorf("waited %v for cluster to reach state %q: %v",
		elapsed, expectedState, err)
}

// MustNewCluster creates a new cluster. If opts contains only one
// slice of command options, those options are used with every node.
// If it is empty, default options are used. Otherwise, it must contain size
// slices of command options, which are used with corresponding nodes.
func MustNewCluster(tb testing.TB, size int, opts ...[]server.CommandOption) Cluster {
	tb.Helper()
	c, err := newCluster(tb, size, opts...)
	if err != nil {
		tb.Fatalf("new cluster: %v", err)
	}
	return c
}

// CheckClusterState polls a given cluster for its state until it
// receives a matching state. It polls up to n times before returning.
func CheckClusterState(m *Command, state string, n int) bool {
	for i := 0; i < n; i++ {
		if m.API.State() == state {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// newCluster creates a new cluster
func newCluster(tb testing.TB, size int, opts ...[]server.CommandOption) (Cluster, error) {
	if size == 0 {
		return nil, errors.New("cluster must contain at least one node")
	}
	if len(opts) != size && len(opts) != 0 && len(opts) != 1 {
		return nil, errors.New("Slice of CommandOptions must be of length 0, 1, or equal to the number of cluster nodes")
	}

	cluster := make(Cluster, size)
	name := tb.Name()
	for i := 0; i < size; i++ {
		var commandOpts []server.CommandOption
		if len(opts) > 0 {
			commandOpts = opts[i%len(opts)]
		}
		m := NewCommandNode(i == 0, commandOpts...)
		err := ioutil.WriteFile(path.Join(m.Config.DataDir, ".id"), []byte(name+"_"+strconv.Itoa(i)), 0600)
		if err != nil {
			return nil, errors.Wrap(err, "writing node id")
		}
		cluster[i] = m
	}

	return cluster, nil
}

// runCluster creates and starts a new cluster
func runCluster(tb testing.TB, size int, opts ...[]server.CommandOption) (Cluster, error) {
	cluster, err := newCluster(tb, size, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "new cluster")
	}

	if err = cluster.Start(); err != nil {
		return nil, errors.Wrap(err, "starting cluster")
	}
	return cluster, nil
}

// MustRunCluster creates and starts a new cluster. The opts parameter
// is slightly magical; see MustNewCluster.
func MustRunCluster(tb testing.TB, size int, opts ...[]server.CommandOption) Cluster {
	// We want tests to default to using the in-memory translate store, so we
	// prepend opts with that functional option. If a different translate store
	// has been specified, it will override this one.
	opts = prependOpts(opts)

	tb.Helper()
	c, err := runCluster(tb, size, opts...)
	if err != nil {
		tb.Fatalf("run cluster: %v", err)
	}
	return c
}

// prependOpts applies prependTestServerOpts to each of the ops (one per
// node, or one for the entire cluser).
func prependOpts(opts [][]server.CommandOption) [][]server.CommandOption {
	if len(opts) == 0 {
		opts = [][]server.CommandOption{
			prependTestServerOpts([]server.CommandOption{}),
		}
	} else {
		for i := range opts {
			opts[i] = prependTestServerOpts(opts[i])
		}
	}
	return opts
}

// prependTestServerOpts prepends opts with the OpenInMemTranslateStore.
func prependTestServerOpts(opts []server.CommandOption) []server.CommandOption {
	defaultOpts := []server.CommandOption{
		server.OptCommandServerOptions(pilosa.OptServerOpenTranslateStore(pilosa.OpenInMemTranslateStore), pilosa.OptServerNodeDownRetries(5, 100*time.Millisecond)),
	}
	return append(defaultOpts, opts...)
}
