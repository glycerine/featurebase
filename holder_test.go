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

package pilosa_test

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pilosa/pilosa/v2"
	"github.com/pilosa/pilosa/v2/pql"
	"github.com/pilosa/pilosa/v2/test"
	"github.com/pkg/errors"
)

func TestHolder_Open(t *testing.T) {
	t.Run("ErrIndexName", func(t *testing.T) {
		h := test.MustOpenHolder()

		bufLogger := test.NewBufferLogger()
		h.Holder.Logger = bufLogger

		defer h.Close()

		if err := os.Mkdir(h.IndexPath("!"), 0777); err != nil {
			t.Fatal(err)
		} else if err := h.Holder.Close(); err != nil {
			t.Fatal(err)
		}
		if err := h.Reopen(); err != nil {
			t.Fatal(err)
		}

		if bufbytes, err := bufLogger.ReadAll(); err != nil {
			t.Fatal(err)
		} else if !bytes.Contains(bufbytes, []byte("ERROR opening index: !")) {
			t.Fatalf("expected log error:\n%s", bufbytes)
		}
	})

	t.Run("ErrIndexPermission", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("Skipping permissions test since user is root.")
		}
		h := test.MustOpenHolder()
		defer h.Close()

		if _, err := h.CreateIndex("test", pilosa.IndexOptions{}); err != nil {
			t.Fatal(err)
		} else if err := h.Holder.Close(); err != nil {
			t.Fatal(err)
		} else if err := os.Chmod(h.IndexPath("test"), 0000); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = os.Chmod(h.IndexPath("test"), 0755)
		}()

		if err := h.Reopen(); err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("unexpected error: %s", err)
		}
	})
	t.Run("ErrIndexAttrStoreCorrupt", func(t *testing.T) {
		h := test.MustOpenHolder()
		defer h.Close()

		if _, err := h.CreateIndex("test", pilosa.IndexOptions{}); err != nil {
			t.Fatal(err)
		} else if err := h.Holder.Close(); err != nil {
			t.Fatal(err)
		} else if err := os.Truncate(filepath.Join(h.IndexPath("test"), ".data"), 2); err != nil {
			t.Fatal(err)
		}

		if err := h.Reopen(); err == nil || !strings.Contains(err.Error(), "open index: name=test, err=opening attrstore: opening storage: invalid database") {
			t.Fatalf("unexpected error: %s", err)
		}
	})

	t.Run("ErrFieldPermission", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("Skipping permissions test since user is root.")
		}
		h := test.MustOpenHolder()
		defer h.Close()

		if idx, err := h.CreateIndex("foo", pilosa.IndexOptions{}); err != nil {
			t.Fatal(err)
		} else if _, err := idx.CreateField("bar", pilosa.OptFieldTypeDefault()); err != nil {
			t.Fatal(err)
		} else if err := h.Holder.Close(); err != nil {
			t.Fatal(err)
		} else if err := os.Chmod(filepath.Join(h.Path, "foo", "bar"), 0000); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = os.Chmod(filepath.Join(h.Path, "foo", "bar"), 0755)
		}()
		if err := h.Reopen(); err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("unexpected error: %s", err)
		}
	})
	t.Run("ErrFieldOptionsCorrupt", func(t *testing.T) {
		h := test.MustOpenHolder()
		defer h.Close()

		if idx, err := h.CreateIndex("foo", pilosa.IndexOptions{}); err != nil {
			t.Fatal(err)
		} else if _, err := idx.CreateField("bar", pilosa.OptFieldTypeDefault()); err != nil {
			t.Fatal(err)
		} else if err := h.Holder.Close(); err != nil {
			t.Fatal(err)
		} else if err := os.Truncate(filepath.Join(h.Path, "foo", "bar", ".meta"), 2); err != nil {
			t.Fatal(err)
		}

		if err := h.Reopen(); err == nil || !strings.Contains(err.Error(), "open index: name=foo, err=opening fields: open field: name=bar, err=loading meta: unmarshaling: unexpected EOF") {
			t.Fatalf("unexpected error: %s", err)
		}
	})
	t.Run("ErrFieldAttrStoreCorrupt", func(t *testing.T) {
		h := test.MustOpenHolder()
		defer h.Close()

		if idx, err := h.CreateIndex("foo", pilosa.IndexOptions{}); err != nil {
			t.Fatal(err)
		} else if _, err := idx.CreateField("bar", pilosa.OptFieldTypeDefault()); err != nil {
			t.Fatal(err)
		} else if err := h.Holder.Close(); err != nil {
			t.Fatal(err)
		} else if err := os.Truncate(filepath.Join(h.Path, "foo", "bar", ".data"), 2); err != nil {
			t.Fatal(err)
		}

		if err := h.Reopen(); err == nil || !strings.Contains(err.Error(), "open index: name=foo, err=opening fields: open field: name=bar, err=opening attrstore: opening storage: invalid database") {
			t.Fatalf("unexpected error: %s", err)
		}
	})

	t.Run("ErrFragmentStoragePermission", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("Skipping permissions test since user is root.")
		}
		h := test.MustOpenHolder()
		defer h.Close()

		if idx, err := h.CreateIndex("foo", pilosa.IndexOptions{}); err != nil {
			t.Fatal(err)
		} else if field, err := idx.CreateField("bar", pilosa.OptFieldTypeDefault()); err != nil {
			t.Fatal(err)
		} else if _, err := field.SetBit(0, 0, nil); err != nil {
			t.Fatal(err)
		} else if err := h.Holder.Close(); err != nil {
			t.Fatal(err)
		} else if err := os.Chmod(filepath.Join(h.Path, "foo", "bar", "views", "standard", "fragments", "0"), 0000); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = os.Chmod(filepath.Join(h.Path, "foo", "bar", "views", "standard", "fragments", "0"), 0644)
		}()
		if err := h.Reopen(); err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("unexpected error: %s", err)
		}
	})
	t.Run("ErrFragmentStorageCorrupt", func(t *testing.T) {
		h := test.MustOpenHolder()
		defer h.Close()

		if idx, err := h.CreateIndex("foo", pilosa.IndexOptions{}); err != nil {
			t.Fatal(err)
		} else if field, err := idx.CreateField("bar", pilosa.OptFieldTypeDefault()); err != nil {
			t.Fatal(err)
		} else if _, err := field.SetBit(0, 0, nil); err != nil {
			t.Fatal(err)
		} else if err := h.Holder.Close(); err != nil {
			t.Fatal(err)
		} else if err := os.Truncate(filepath.Join(h.Path, "foo", "bar", "views", "standard", "fragments", "0"), 2); err != nil {
			t.Fatal(err)
		}

		if err := h.Reopen(); err == nil || !strings.Contains(err.Error(), "open fragment: shard=0, err=opening storage: unmarshal storage") {
			t.Fatalf("unexpected error: %s", err)
		}
	})
	t.Run("ErrFragmentStorageRecoverable", func(t *testing.T) {
		h := test.MustOpenHolder()
		defer h.Close()

		if idx, err := h.CreateIndex("foo", pilosa.IndexOptions{}); err != nil {
			t.Fatal(err)
		} else if field, err := idx.CreateField("bar", pilosa.OptFieldTypeDefault()); err != nil {
			t.Fatal(err)
		} else if _, err := field.SetBit(0, 0, nil); err != nil {
			t.Fatal(err)
		} else if err := h.Holder.Close(); err != nil {
			t.Fatal(err)
		} else if err := os.Truncate(filepath.Join(h.Path, "foo", "bar", "views", "standard", "fragments", "0"), 20); err != nil {
			t.Fatal(err)
		}

		if err := h.Reopen(); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
	})

	t.Run("ForeignIndex", func(t *testing.T) {
		t.Run("ErrForeignIndexNotFound", func(t *testing.T) {
			h := test.MustOpenHolder()
			defer h.Close()

			if idx, err := h.CreateIndex("foo", pilosa.IndexOptions{}); err != nil {
				t.Fatal(err)
			} else {
				_, err := idx.CreateField("bar", pilosa.OptFieldTypeInt(0, 100), pilosa.OptFieldForeignIndex("nonexistent"))
				if err == nil {
					t.Fatalf("expected error: %s", pilosa.ErrForeignIndexNotFound)
				} else if errors.Cause(err) != pilosa.ErrForeignIndexNotFound {
					t.Fatalf("expected error: %s, but got: %s", pilosa.ErrForeignIndexNotFound, err)
				}
			}
		})

		// Foreign index zzz is opened after foo/bar.
		t.Run("ForeignIndexNotOpenYet", func(t *testing.T) {
			h := test.MustOpenHolder()
			defer h.Close()

			if _, err := h.CreateIndex("zzz", pilosa.IndexOptions{}); err != nil {
				t.Fatal(err)
			} else if idx, err := h.CreateIndex("foo", pilosa.IndexOptions{}); err != nil {
				t.Fatal(err)
			} else if _, err := idx.CreateField("bar", pilosa.OptFieldTypeInt(0, 100), pilosa.OptFieldForeignIndex("zzz")); err != nil {
				t.Fatal(err)
			} else if err := h.Holder.Close(); err != nil {
				t.Fatal(err)
			}

			if err := h.Reopen(); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
		})

		// Foreign index aaa is opened before foo/bar.
		t.Run("ForeignIndexIsOpen", func(t *testing.T) {
			h := test.MustOpenHolder()
			defer h.Close()

			if _, err := h.CreateIndex("aaa", pilosa.IndexOptions{}); err != nil {
				t.Fatal(err)
			} else if idx, err := h.CreateIndex("foo", pilosa.IndexOptions{}); err != nil {
				t.Fatal(err)
			} else if _, err := idx.CreateField("bar", pilosa.OptFieldTypeInt(0, 100), pilosa.OptFieldForeignIndex("aaa")); err != nil {
				t.Fatal(err)
			} else if err := h.Holder.Close(); err != nil {
				t.Fatal(err)
			}

			if err := h.Reopen(); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
		})

		// Try to re-create existing index
		t.Run("CreateIndexIfNotExists", func(t *testing.T) {
			h := test.MustOpenHolder()
			defer h.Close()

			idx1, err := h.CreateIndexIfNotExists("aaa", pilosa.IndexOptions{})
			if err != nil {
				t.Fatal(err)
			}

			if _, err = h.CreateIndex("aaa", pilosa.IndexOptions{}); err == nil {
				t.Fatalf("expected: ConflictError, got: nil")
			} else if _, ok := err.(pilosa.ConflictError); !ok {
				t.Fatalf("expected: ConflictError, got: %s", err)
			}

			idx2, err := h.CreateIndexIfNotExists("aaa", pilosa.IndexOptions{})
			if err != nil {
				t.Fatal(err)
			}

			if idx1 != idx2 {
				t.Fatalf("expected the same indexes, got: %s and %s", idx1.Name(), idx2.Name())
			}
		})
	})
}

func TestHolder_HasData(t *testing.T) {
	t.Run("IndexDirectory", func(t *testing.T) {
		h := test.MustOpenHolder()
		defer h.Close()

		if ok, err := h.HasData(); ok || err != nil {
			t.Fatal("expected HasData to return false, no err, but", ok, err)
		}

		if _, err := h.CreateIndex("test", pilosa.IndexOptions{}); err != nil {
			t.Fatal(err)
		}

		if ok, err := h.HasData(); !ok || err != nil {
			t.Fatal("expected HasData to return true, but ", ok, err)
		}
	})

	t.Run("Peek", func(t *testing.T) {
		h := test.NewHolder()

		if ok, err := h.HasData(); ok || err != nil {
			t.Fatal("expected HasData to return false, no err, but", ok, err)
		}

		// Create an index directory to indicate data exists.
		if err := os.Mkdir(h.IndexPath("test"), 0777); err != nil {
			t.Fatal(err)
		}

		if ok, err := h.HasData(); !ok || err != nil {
			t.Fatal("expected HasData to return true, no err, but", ok, err)
		}
	})

	t.Run("Peek at missing directory", func(t *testing.T) {
		h := test.NewHolder()

		// Ensure that hasData is false when dir doesn't exist.
		h.Path = "bad-path"

		if ok, err := h.HasData(); ok || err != nil {
			t.Fatal("expected HasData to return false, no err, but", ok, err)
		}
	})
}

// Ensure holder can delete an index and its underlying files.
func TestHolder_DeleteIndex(t *testing.T) {
	hldr := test.MustOpenHolder()
	defer hldr.Close()

	// Write bits to separate indexes.
	hldr.SetBit("i0", "f", 100, 200)
	hldr.SetBit("i1", "f", 100, 200)

	// Ensure i0 exists.
	if _, err := os.Stat(hldr.IndexPath("i0")); err != nil {
		t.Fatal(err)
	}

	// Delete i0.
	if err := hldr.DeleteIndex("i0"); err != nil {
		t.Fatal(err)
	}

	// Ensure i0 files are removed & i1 still exists.
	if _, err := os.Stat(hldr.IndexPath("i0")); !os.IsNotExist(err) {
		t.Fatal("expected i0 file deletion")
	} else if _, err := os.Stat(hldr.IndexPath("i1")); err != nil {
		t.Fatal("expected i1 files to still exist", err)
	}
}

// Ensure holder can sync with a remote holder.
func TestHolderSyncer_SyncHolder(t *testing.T) {
	c := test.MustNewCluster(t, 2)
	c[0].Config.Cluster.ReplicaN = 2
	c[0].Config.AntiEntropy.Interval = 0
	c[1].Config.Cluster.ReplicaN = 2
	c[1].Config.AntiEntropy.Interval = 0
	err := c.Start()
	if err != nil {
		t.Fatalf("starting cluster: %v", err)
	}
	defer c.Close()

	_, err = c[0].API.CreateIndex(context.Background(), "i", pilosa.IndexOptions{})
	if err != nil {
		t.Fatalf("creating index i: %v", err)
	}
	_, err = c[0].API.CreateIndex(context.Background(), "y", pilosa.IndexOptions{})
	if err != nil {
		t.Fatalf("creating index y: %v", err)
	}
	_, err = c[0].API.CreateField(context.Background(), "i", "f", pilosa.OptFieldTypeSet(pilosa.DefaultCacheType, pilosa.DefaultCacheSize))
	if err != nil {
		t.Fatalf("creating field f: %v", err)
	}
	_, err = c[0].API.CreateField(context.Background(), "i", "f0", pilosa.OptFieldTypeSet(pilosa.DefaultCacheType, pilosa.DefaultCacheSize))
	if err != nil {
		t.Fatalf("creating field f0: %v", err)
	}
	_, err = c[0].API.CreateField(context.Background(), "y", "z", pilosa.OptFieldTypeMutex(pilosa.DefaultCacheType, pilosa.DefaultCacheSize))
	if err != nil {
		t.Fatalf("creating field z in y: %v", err)
	}
	_, err = c[0].API.CreateField(context.Background(), "y", "b", pilosa.OptFieldTypeBool())
	if err != nil {
		t.Fatalf("creating field b in y: %v", err)
	}

	hldr0 := &test.Holder{Holder: c[0].Server.Holder()}
	hldr1 := &test.Holder{Holder: c[1].Server.Holder()}

	// Set data on the local holder.
	hldr0.SetBit("i", "f", 0, 10)
	hldr0.SetBit("i", "f", 2, 20)
	hldr0.SetBit("i", "f", 120, 10)
	hldr0.SetBit("i", "f", 200, 4)

	hldr0.SetBit("i", "f0", 9, ShardWidth+5)

	// Set a bit to create the fragment.
	hldr0.SetBit("y", "z", 0, 0)
	hldr0.SetBit("y", "b", 0, 0) // rowID = 0 means false

	// Set data on the remote holder.
	hldr1.SetBit("i", "f", 0, 4000)
	hldr1.SetBit("i", "f", 3, 10)
	hldr1.SetBit("i", "f", 120, 10)

	hldr1.SetBit("y", "z", 10, (3*ShardWidth)+4)
	hldr1.SetBit("y", "z", 10, (3*ShardWidth)+5)
	hldr1.SetBit("y", "z", 10, (3*ShardWidth)+7)

	hldr1.SetBit("y", "b", 1, (3*ShardWidth)+4) // true
	hldr1.SetBit("y", "b", 0, (3*ShardWidth)+5) // false
	hldr1.SetBit("y", "b", 1, (3*ShardWidth)+7) // true

	err = c[0].Server.SyncData()
	if err != nil {
		t.Fatalf("syncing node 0: %v", err)
	}
	err = c[1].Server.SyncData()
	if err != nil {
		t.Fatalf("syncing node 1: %v", err)
	}

	// Verify data is the same on both nodes.
	for i, hldr := range []*test.Holder{hldr0, hldr1} {
		if a := hldr.Row("i", "f", 0).Columns(); !reflect.DeepEqual(a, []uint64{10, 4000}) {
			t.Errorf("unexpected columns(%d/0): %+v", i, a)
		}
		if a := hldr.Row("i", "f", 2).Columns(); !reflect.DeepEqual(a, []uint64{20}) {
			t.Errorf("unexpected columns(%d/2): %+v", i, a)
		}
		if a := hldr.Row("i", "f", 3).Columns(); !reflect.DeepEqual(a, []uint64{10}) {
			t.Errorf("unexpected columns(%d/3): %+v", i, a)
		}
		if a := hldr.Row("i", "f", 120).Columns(); !reflect.DeepEqual(a, []uint64{10}) {
			t.Errorf("unexpected columns(%d/120): %+v", i, a)
		}
		if a := hldr.Row("i", "f", 200).Columns(); !reflect.DeepEqual(a, []uint64{4}) {
			t.Errorf("unexpected columns(%d/200): %+v", i, a)
		}

		if a := hldr.Row("i", "f0", 9).Columns(); !reflect.DeepEqual(a, []uint64{ShardWidth + 5}) {
			t.Errorf("unexpected columns(%d/d/f0): %+v", i, a)
		}

		if a := hldr.Row("y", "z", 10).Columns(); !reflect.DeepEqual(a, []uint64{(3 * ShardWidth) + 4, (3 * ShardWidth) + 5, (3 * ShardWidth) + 7}) {
			t.Errorf("unexpected columns(%d/y/z): %+v", i, a)
		}

		if a := hldr.Row("y", "b", 0).Columns(); !reflect.DeepEqual(a, []uint64{0, (3 * ShardWidth) + 5}) {
			t.Errorf("unexpected false columns(%d/y/b): %+v", i, a)
		}
		if a := hldr.Row("y", "b", 1).Columns(); !reflect.DeepEqual(a, []uint64{(3 * ShardWidth) + 4, (3 * ShardWidth) + 7}) {
			t.Errorf("unexpected true columns(%d/y/b): %+v", i, a)
		}
	}
}

// Ensure holder can sync time quantum views with a remote holder.
func TestHolderSyncer_TimeQuantum(t *testing.T) {
	c := test.MustNewCluster(t, 2)
	c[0].Config.Cluster.ReplicaN = 2
	c[0].Config.AntiEntropy.Interval = 0
	c[1].Config.Cluster.ReplicaN = 2
	c[1].Config.AntiEntropy.Interval = 0
	err := c.Start()
	if err != nil {
		t.Fatalf("starting cluster: %v", err)
	}
	defer c.Close()

	quantum := "D"

	_, err = c[0].API.CreateIndex(context.Background(), "i", pilosa.IndexOptions{})
	if err != nil {
		t.Fatalf("creating index i: %v", err)
	}
	_, err = c[0].API.CreateField(context.Background(), "i", "f", pilosa.OptFieldTypeTime(pilosa.TimeQuantum(quantum)))
	if err != nil {
		t.Fatalf("creating field f: %v", err)
	}

	hldr0 := &test.Holder{Holder: c[0].Server.Holder()}
	hldr1 := &test.Holder{Holder: c[1].Server.Holder()}

	// Set data on the local holder for node0.
	t1 := time.Date(2018, 8, 1, 12, 30, 0, 0, time.UTC)
	t2 := time.Date(2018, 8, 2, 12, 30, 0, 0, time.UTC)
	hldr0.SetBitTime("i", "f", 0, 1, &t1)
	hldr0.SetBitTime("i", "f", 0, 2, &t2)

	// Set data on node1.
	hldr1.SetBitTime("i", "f", 0, 22, &t2)

	err = c[0].Server.SyncData()
	if err != nil {
		t.Fatalf("syncing node 0: %v", err)
	}

	// Verify data is the same on both nodes.
	for i, hldr := range []*test.Holder{hldr0, hldr1} {
		if a := hldr.RowTime("i", "f", 0, t1, quantum).Columns(); !reflect.DeepEqual(a, []uint64{1}) {
			t.Errorf("unexpected columns(%d/0): %+v", i, a)
		}
		if a := hldr.RowTime("i", "f", 0, t2, quantum).Columns(); !reflect.DeepEqual(a, []uint64{2, 22}) {
			t.Errorf("unexpected columns(%d/0): %+v", i, a)
		}
	}
}

// Ensure holder can sync integer views with a remote holder.
func TestHolderSyncer_IntField(t *testing.T) {
	t.Run("BasicSync", func(t *testing.T) {
		c := test.MustNewCluster(t, 2)
		c[0].Config.Cluster.ReplicaN = 2
		c[0].Config.AntiEntropy.Interval = 0
		c[1].Config.Cluster.ReplicaN = 2
		c[1].Config.AntiEntropy.Interval = 0
		err := c.Start()
		if err != nil {
			t.Fatalf("starting cluster: %v", err)
		}
		defer c.Close()

		_, err = c[0].API.CreateIndex(context.Background(), "i", pilosa.IndexOptions{})
		if err != nil {
			t.Fatalf("creating index i: %v", err)
		}
		_, err = c[0].API.CreateField(context.Background(), "i", "f", pilosa.OptFieldTypeInt(0, 100))
		if err != nil {
			t.Fatalf("creating field f: %v", err)
		}

		hldr0 := &test.Holder{Holder: c[0].Server.Holder()}
		hldr1 := &test.Holder{Holder: c[1].Server.Holder()}

		// Set data on the local holder for node0.
		hldr0.SetValue("i", "f", 1, 1)

		// Set data on node1.
		hldr1.SetValue("i", "f", 2, 2)

		err = c[0].Server.SyncData()
		if err != nil {
			t.Fatalf("syncing node 0: %v", err)
		}

		// Verify data is the same on both nodes.
		for i, hldr := range []*test.Holder{hldr0, hldr1} {
			if a, exists := hldr.Value("i", "f", 1); !exists || a != 1 {
				t.Errorf("unexpected value(node%d/0): %d, exists: %v", i, a, exists)
			}
			if a, exists := hldr.Value("i", "f", 2); exists {
				t.Errorf("unexpected value(node%d/1): %d, exists: %v", i, a, exists)
			}
		}
	})

	t.Run("MultiShard", func(t *testing.T) {
		c := test.MustNewCluster(t, 2)
		c[0].Config.Cluster.ReplicaN = 2
		c[0].Config.AntiEntropy.Interval = 0
		c[1].Config.Cluster.ReplicaN = 2
		c[1].Config.AntiEntropy.Interval = 0
		err := c.Start()
		if err != nil {
			t.Fatalf("starting cluster: %v", err)
		}
		defer c.Close()

		_, err = c[0].API.CreateIndex(context.Background(), "i", pilosa.IndexOptions{})
		if err != nil {
			t.Fatalf("creating index i: %v", err)
		}
		_, err = c[0].API.CreateField(context.Background(), "i", "f", pilosa.OptFieldTypeInt(math.MinInt64, math.MaxInt64))
		if err != nil {
			t.Fatalf("creating field f: %v", err)
		}

		hldr0 := &test.Holder{Holder: c[0].Server.Holder()}
		hldr1 := &test.Holder{Holder: c[1].Server.Holder()}

		// Set data on the local holder for node0.
		hldr0.SetValue("i", "f", 1*pilosa.ShardWidth, 11)
		hldr0.SetValue("i", "f", 3*pilosa.ShardWidth, 32)
		hldr0.SetValue("i", "f", 4*pilosa.ShardWidth, math.MinInt32)
		hldr0.SetValue("i", "f", 7*pilosa.ShardWidth, math.MinInt32)

		// Set data on node1.
		hldr1.SetValue("i", "f", 0*pilosa.ShardWidth, 2)
		hldr1.SetValue("i", "f", 2*pilosa.ShardWidth, 22)
		hldr1.SetValue("i", "f", 4*pilosa.ShardWidth, math.MaxInt32)
		hldr1.SetValue("i", "f", 7*pilosa.ShardWidth, math.MaxInt32)

		// Primary for shards (for index "i"):
		// node0: [0,3,7]
		// node1: [1,2,4]

		err = c[0].Server.SyncData()
		if err != nil {
			t.Fatalf("syncing node 0: %v", err)
		}
		err = c[1].Server.SyncData()
		if err != nil {
			t.Fatalf("syncing node 1: %v", err)
		}

		// Verify data is the same on both nodes.
		for i, hldr := range []*test.Holder{hldr0, hldr1} {
			if a := hldr.Range("i", "f", pql.GT, 0); !reflect.DeepEqual(a.Columns(), []uint64{2 * pilosa.ShardWidth, 3 * pilosa.ShardWidth, 4 * pilosa.ShardWidth}) {
				t.Errorf("unexpected columns(node%d/0): %d", i, a.Columns())
			}
			if a := hldr.Range("i", "f", pql.LT, 0); !reflect.DeepEqual(a.Columns(), []uint64{7 * pilosa.ShardWidth}) {
				t.Errorf("unexpected columns(node%d/0): %d", i, a.Columns())
			}
		}
	})
}
