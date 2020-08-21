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
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"testing/quick"

	"golang.org/x/sync/errgroup"

	"github.com/davecgh/go-spew/spew"
	"github.com/pilosa/pilosa/v2/pql"
	"github.com/pilosa/pilosa/v2/roaring"
	"github.com/pkg/errors"
)

// Test flags
var (
	// In order to generate the sample fragment file,
	// run an import and copy PILOSA_DATA_DIR/INDEX_NAME/FRAME_NAME/0 to testdata/sample_view
	FragmentPath = flag.String("fragment", "testdata/sample_view/0", "fragment path")

	TempDir = flag.String("temp-dir", "", "Directory in which to place temporary data (e.g. for benchmarking). Useful if you are trying to benchmark different storage configurations.")
)

// Ensure a fragment can set a bit and retrieve it.
func TestFragment_SetBit(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Set bits on the fragment.
	if _, err := f.setBit(tx, 120, 1); err != nil {
		t.Fatal(err)
	} else if _, err := f.setBit(tx, 120, 6); err != nil {
		t.Fatal(err)
	} else if _, err := f.setBit(tx, 121, 0); err != nil {
		t.Fatal(err)
	}
	// should have two containers set in the fragment.

	// Verify counts on rows.
	if n := f.mustRow(tx, 120).Count(); n != 2 {
		t.Fatalf("unexpected count: %d", n)
	} else if n := f.mustRow(tx, 121).Count(); n != 1 {
		t.Fatalf("unexpected count: %d", n)
	}

	// commit the change, and verify it is still there
	panicOn(tx.Commit())

	// Close and reopen the fragment & verify the data.
	err := f.Reopen() // roaring data not being flushed? red on roaring
	if err != nil {
		t.Fatal(err)
	}
	tx = idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
	defer tx.Rollback()

	if n := f.mustRow(tx, 120).Count(); n != 2 {
		t.Fatalf("unexpected count (reopen): %d", n)
	} else if n := f.mustRow(tx, 121).Count(); n != 1 {
		t.Fatalf("unexpected count (reopen): %d", n)
	}
}

// Ensure a fragment can clear a set bit.
func TestFragment_ClearBit(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Set and then clear bits on the fragment.
	if _, err := f.setBit(tx, 1000, 1); err != nil {
		t.Fatal(err)
	} else if _, err := f.setBit(tx, 1000, 2); err != nil {
		t.Fatal(err)
	} else if _, err := f.clearBit(tx, 1000, 1); err != nil {
		t.Fatal(err)
	}

	// Verify count on row.
	if n := f.mustRow(tx, 1000).Count(); n != 1 {
		t.Fatalf("unexpected count: %d", n)
	}
	// The Reopen below implies this test is looking at storage consistency.
	// In that spirit, we will check that the Tx Commit is visible afterwards.
	panicOn(tx.Commit())

	tx = idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
	defer tx.Rollback()

	// Close and reopen the fragment & verify the data.
	if err := f.Reopen(); err != nil {
		t.Fatal(err)
	} else if n := f.mustRow(tx, 1000).Count(); n != 1 {
		t.Fatalf("unexpected count (reopen): %d", n)
	}
}

/* We suspect this test is no longer valid under the new Tx
   framework in which we always copy mmap-ed rows before
   returning them. So we will comment it out for now.
   If someone knows any reason for this to stick around,
   let us know; we couldn't figure out how to adapt
   to do a meaningful test under Tx. - jaten / tgruben

// What about rowcache timing.
func TestFragment_RowcacheMap(t *testing.T) {
	var done int64
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	_ = idx
	// Obtain transaction.
	tx := f.txTestingOnly
    defer tx.Rollback()

	// Under -race, this test turns out to take a fairly long time
	// to run with larger OpN, because we write 50,000 bits to
	// the bitmap, and everything is being race-detected, and we don't
	// actually need that many to get the result we care about.
	f.MaxOpN = 2000
	defer f.Clean(t) // failing here with     TestFragment_RowcacheMap: fragment_internal_test.go:2859: fragment /var/folders/2x/hm9gp5ys3k9gmm5f_vzm_6wc0000gn/T/pilosa-fragment-001943331: unmarshalled bitmap different: differing containers for key 0: <run container, N=10131, len 2000x interval> vs <run container, N=12000, len 2000x interval>

	ch := make(chan struct{})

	for i := 0; i < f.MaxOpN; i++ {
		_, _ = f.setBit(tx, 0, uint64(i*32))
	}
	// force snapshot so we get a mmapped row...
	_ = f.Snapshot()
	row := f.mustRow(tx, 0)
	tx.Commit(0)
	segment := row.Segments()[0]
	bitmap := segment.data

	// request information from the frozen bitmap we got back
	go func() {
		for atomic.LoadInt64(&done) == 0 {
			for i := 0; i < f.MaxOpN; i++ {
				_ = bitmap.Contains(uint64(i * 32))
			}
		}
		close(ch)
	}()

	// modify the original bitmap, until it causes a snapshot, which
	// then invalidates the other map...
	for j := 0; j < 5; j++ {
		for i := 0; i < f.MaxOpN; i++ {
			_, _ = f.setBit(tx, 0, uint64(i*32+j+1))
		}
	}
	atomic.StoreInt64(&done, 1)
	<-ch
}
*/

// Ensure a fragment can clear a row.
func TestFragment_ClearRow(t *testing.T) {
	notBlueGreenTest(t)

	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Set and then clear bits on the fragment.
	if _, err := f.setBit(tx, 1000, 1); err != nil {
		t.Fatal(err)
	} else if _, err := f.setBit(tx, 1000, 65536); err != nil {
		t.Fatal(err)
	} else if _, err := f.unprotectedClearRow(tx, 1000); err != nil {
		t.Fatal(err)
	}

	// Verify count on row.
	if n := f.mustRow(tx, 1000).Count(); n != 0 {
		t.Fatalf("unexpected count: %d", n)
	}
	panicOn(tx.Commit())
	tx = idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
	defer tx.Rollback()

	// Close and reopen the fragment & verify the data.
	if err := f.Reopen(); err != nil {
		t.Fatal(err)
	} else if n := f.mustRow(tx, 1000).Count(); n != 0 {
		t.Fatalf("unexpected count (reopen): %d", n)
	}
}

// Ensure a fragment can set a row.
func TestFragment_SetRow(t *testing.T) {
	notBlueGreenTest(t)
	f, idx := mustOpenFragment("i", "f", viewStandard, 7, "")
	_ = idx
	defer f.Clean(t)

	// Obtain transction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	rowID := uint64(1000)

	// Set bits on the fragment.
	if _, err := f.setBit(tx, rowID, 7*ShardWidth+1); err != nil {
		t.Fatal(err)
	} else if _, err := f.setBit(tx, rowID, 7*ShardWidth+65536); err != nil {
		t.Fatal(err)
	}

	// Verify data on row.
	if cols := f.mustRow(tx, rowID).Columns(); !reflect.DeepEqual(cols, []uint64{7*ShardWidth + 1, 7*ShardWidth + 65536}) {
		t.Fatalf("unexpected columns: %+v", cols)
	}
	// Verify count on row.
	if n := f.mustRow(tx, rowID).Count(); n != 2 {
		t.Fatalf("unexpected count: %d", n)
	}

	// Set row (overwrite existing data).
	row := NewRow(7*ShardWidth+1, 7*ShardWidth+65537, 7*ShardWidth+140000)
	if changed, err := f.unprotectedSetRow(tx, row, rowID); err != nil {
		t.Fatal(err)
	} else if !changed {
		t.Fatalf("expected changed value: %v", changed)
	}

	panicOn(tx.Commit())
	tx = idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
	defer tx.Rollback()

	// Verify data on row.
	if cols := f.mustRow(tx, rowID).Columns(); !reflect.DeepEqual(cols, []uint64{7*ShardWidth + 1, 7*ShardWidth + 65537, 7*ShardWidth + 140000}) {
		t.Fatalf("unexpected columns after set row: %+v", cols)
	}
	// Verify count on row.
	if n := f.mustRow(tx, rowID).Count(); n != 3 {
		t.Fatalf("unexpected count after set row: %d", n)
	}

	panicOn(tx.Commit())
	tx = idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
	defer tx.Rollback()

	// Close and reopen the fragment & verify the data.
	if err := f.Reopen(); err != nil {
		t.Fatal(err)
	} else if n := f.mustRow(tx, rowID).Count(); n != 3 {
		t.Fatalf("unexpected count (reopen): %d", n)
	}
}

// Ensure a fragment can set & read a value.
func TestFragment_SetValue(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		// Set value.
		if changed, err := f.setValue(tx, 100, 16, 3829); err != nil {
			t.Fatal(err)
		} else if !changed {
			t.Fatal("expected change")
		}

		// Read value.
		if value, exists, err := f.value(tx, 100, 16); err != nil {
			t.Fatal(err)
		} else if value != 3829 {
			t.Fatalf("unexpected value: %d", value)
		} else if !exists {
			t.Fatal("expected to exist")
		}

		// Setting value should return no change.
		if changed, err := f.setValue(tx, 100, 16, 3829); err != nil {
			t.Fatal(err)
		} else if changed {
			t.Fatal("expected no change")
		}

		// same after Commit
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		tx = idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
		defer tx.Rollback()

		// Read value.
		if value, exists, err := f.value(tx, 100, 16); err != nil {
			t.Fatal(err)
		} else if value != 3829 {
			t.Fatalf("unexpected value: %d", value)
		} else if !exists {
			t.Fatal("expected to exist")
		}

		// Setting value should return no change.
		if changed, err := f.setValue(tx, 100, 16, 3829); err != nil {
			t.Fatal(err)
		} else if changed {
			t.Fatal("expected no change")
		}
	})

	t.Run("Overwrite", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		// Set value.
		if changed, err := f.setValue(tx, 100, 16, 3829); err != nil {
			t.Fatal(err)
		} else if !changed {
			t.Fatal("expected change")
		}

		// Overwriting value should overwrite all bits.
		if changed, err := f.setValue(tx, 100, 16, 2028); err != nil {
			t.Fatal(err)
		} else if !changed {
			t.Fatal("expected change")
		}

		// Read value.
		if value, exists, err := f.value(tx, 100, 16); err != nil {
			t.Fatal(err)
		} else if value != 2028 {
			t.Fatalf("unexpected value: %d", value)
		} else if !exists {
			t.Fatal("expected to exist")
		}

		// Read value after commit.
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		tx = idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
		defer tx.Rollback()

		if value, exists, err := f.value(tx, 100, 16); err != nil {
			t.Fatal(err)
		} else if value != 2028 {
			t.Fatalf("unexpected value: %d", value)
		} else if !exists {
			t.Fatal("expected to exist")
		}

	})

	t.Run("Clear", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		// Set value.
		if changed, err := f.setValue(tx, 100, 16, 3829); err != nil {
			t.Fatal(err)
		} else if !changed {
			t.Fatal("expected change")
		}

		// Clear value should overwrite all bits, and set not-null to 0.
		if changed, err := f.clearValue(tx, 100, 16, 2028); err != nil {
			t.Fatal(err)
		} else if !changed {
			t.Fatal("expected change")
		}

		// Read value.
		if value, exists, err := f.value(tx, 100, 16); err != nil {
			t.Fatal(err)
		} else if value != 0 {
			t.Fatalf("unexpected value: %d", value)
		} else if exists {
			t.Fatal("expected to not exist")
		}

		// Same after Commit
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		tx = idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
		defer tx.Rollback()

		if value, exists, err := f.value(tx, 100, 16); err != nil {
			t.Fatal(err)
		} else if value != 0 {
			t.Fatalf("unexpected value: %d", value)
		} else if exists {
			t.Fatal("expected to not exist")
		}
	})

	t.Run("NotExists", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		// Set value.
		if changed, err := f.setValue(tx, 100, 10, 20); err != nil {
			t.Fatal(err)
		} else if !changed {
			t.Fatal("expected change")
		}

		// Non-existent value.
		if value, exists, err := f.value(tx, 101, 11); err != nil {
			t.Fatal(err)
		} else if value != 0 {
			t.Fatalf("unexpected value: %d", value)
		} else if exists {
			t.Fatal("expected to not exist")
		}

	})

	t.Run("QuickCheck", func(t *testing.T) {
		if err := quick.Check(func(bitDepth uint, bitN uint64, values []uint64) bool {
			// Limit bit depth & maximum values.
			bitDepth = (bitDepth % 62) + 1
			bitN = (bitN % 99) + 1

			for i := range values {
				values[i] = values[i] % (1 << bitDepth)
			}

			f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
			_ = idx
			defer f.Clean(t)

			// Obtain transaction.
			tx := f.txTestingOnly
			defer tx.Rollback()

			// Set values.
			m := make(map[uint64]int64)
			for _, value := range values {
				columnID := value % bitN

				m[columnID] = int64(value)

				if _, err := f.setValue(tx, columnID, bitDepth, int64(value)); err != nil {
					t.Fatal(err)
				}
			}

			// Ensure values are set.
			for columnID, value := range m {
				v, exists, err := f.value(tx, columnID, bitDepth)
				if err != nil {
					t.Fatal(err)
				} else if value != int64(v) {
					t.Fatalf("value mismatch: columnID=%d, bitdepth=%d, value: %d != %d", columnID, bitDepth, value, v)
				} else if !exists {
					t.Fatalf("value should exist: columnID=%d", columnID)
				}
			}

			// Same after Commit
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			tx = idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
			defer tx.Rollback()

			// Ensure values are set.
			for columnID, value := range m {
				v, exists, err := f.value(tx, columnID, bitDepth)
				if err != nil {
					t.Fatal(err)
				} else if value != int64(v) {
					t.Fatalf("value mismatch: columnID=%d, bitdepth=%d, value: %d != %d", columnID, bitDepth, value, v)
				} else if !exists {
					t.Fatalf("value should exist: columnID=%d", columnID)
				}
			}

			return true
		}, nil); err != nil {
			t.Fatal(err)
		}
	})
}

// Ensure a fragment can sum values.
func TestFragment_Sum(t *testing.T) {
	const bitDepth = 16

	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Set values.
	vals := []struct {
		cid uint64
		val int64
	}{
		{1000, 382},
		{2000, 300},
		{2500, -600},
		{3000, 2818},
		{4000, 300},
	}
	for _, v := range vals {
		if _, err := f.setValue(tx, v.cid, bitDepth, v.val); err != nil {
			t.Fatal(err)
		}
	}

	panicOn(tx.Commit())
	tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
	defer tx.Rollback()

	t.Run("NoFilter", func(t *testing.T) {
		if sum, n, err := f.sum(tx, nil, bitDepth); err != nil {
			t.Fatal(err)
		} else if n != 5 {
			t.Fatalf("unexpected count: %d", n)
		} else if got, exp := sum, int64(382+300-600+2818+300); got != exp {
			t.Fatalf("unexpected sum: got: %d, exp: %d", sum, exp)
		}
	})

	t.Run("WithFilter", func(t *testing.T) {
		if sum, n, err := f.sum(tx, NewRow(2000, 4000, 5000), bitDepth); err != nil {
			t.Fatal(err)
		} else if n != 2 {
			t.Fatalf("unexpected count: %d", n)
		} else if got, exp := sum, int64(300+300); got != exp {
			t.Fatalf("unexpected sum: got: %d, exp: %d", sum, exp)
		}
	})

	panicOn(tx.Commit())
	tx = idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
	defer tx.Rollback()

	// verify that clearValue clears values
	if _, err := f.clearValue(tx, 1000, bitDepth, 23); err != nil {
		t.Fatal(err)
	}

	panicOn(tx.Commit())
	tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
	defer tx.Rollback()

	t.Run("ClearValue", func(t *testing.T) {
		if sum, n, err := f.sum(tx, nil, bitDepth); err != nil {
			t.Fatal(err)
		} else if n != 4 {
			t.Fatalf("unexpected count: %d", n)
		} else if got, exp := sum, int64(3800-382-600); got != exp {
			t.Fatalf("unexpected sum: got: %d, exp: %d", sum, exp)
		}
	})
}

// Ensure a fragment can find the min and max of values.
func TestFragment_MinMax(t *testing.T) {
	const bitDepth = 16

	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Set values.
	if _, err := f.setValue(tx, 1000, bitDepth, 382); err != nil {
		t.Fatal(err)
	} else if _, err := f.setValue(tx, 2000, bitDepth, 300); err != nil {
		t.Fatal(err)
	} else if _, err := f.setValue(tx, 3000, bitDepth, 2818); err != nil {
		t.Fatal(err)
	} else if _, err := f.setValue(tx, 4000, bitDepth, 300); err != nil {
		t.Fatal(err)
	} else if _, err := f.setValue(tx, 5000, bitDepth, 2818); err != nil {
		t.Fatal(err)
	} else if _, err := f.setValue(tx, 6000, bitDepth, 2817); err != nil {
		t.Fatal(err)
	} else if _, err := f.setValue(tx, 7000, bitDepth, 0); err != nil {
		t.Fatal(err)
	}

	panicOn(tx.Commit())

	// the new tx is shared by Min/Max below.
	tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
	defer tx.Rollback()

	t.Run("Min", func(t *testing.T) {
		tests := []struct {
			filter *Row
			exp    int64
			cnt    uint64
		}{
			{filter: nil, exp: 0, cnt: 1},
			{filter: NewRow(2000, 4000, 5000), exp: 300, cnt: 2},
			{filter: NewRow(2000, 4000), exp: 300, cnt: 2},
			{filter: NewRow(1), exp: 0, cnt: 0},
			{filter: NewRow(1000), exp: 382, cnt: 1},
			{filter: NewRow(7000), exp: 0, cnt: 1},
		}
		for i, test := range tests {
			if min, cnt, err := f.min(tx, test.filter, bitDepth); err != nil {
				t.Fatal(err)
			} else if min != test.exp {
				t.Errorf("test %d expected min: %v, but got: %v", i, test.exp, min)
			} else if cnt != test.cnt {
				t.Errorf("test %d expected cnt: %v, but got: %v", i, test.cnt, cnt)
			}
		}
	})

	t.Run("Max", func(t *testing.T) {
		tests := []struct {
			filter *Row
			exp    int64
			cnt    uint64
		}{
			{filter: nil, exp: 2818, cnt: 2},
			{filter: NewRow(2000, 4000, 5000), exp: 2818, cnt: 1},
			{filter: NewRow(2000, 4000), exp: 300, cnt: 2},
			{filter: NewRow(1), exp: 0, cnt: 0},
			{filter: NewRow(1000), exp: 382, cnt: 1},
			{filter: NewRow(7000), exp: 0, cnt: 1},
		}
		for i, test := range tests {
			var columns []uint64
			if test.filter != nil {
				columns = test.filter.Columns()
			}

			if max, cnt, err := f.max(tx, test.filter, bitDepth); err != nil {
				t.Fatal(err)
			} else if max != test.exp || cnt != test.cnt {
				t.Errorf("%d. max(%v, %v)=(%v, %v), expected (%v, %v)", i, columns, bitDepth, max, cnt, test.exp, test.cnt)
			}
		}
	})
}

// Ensure a fragment query for matching values.
func TestFragment_Range(t *testing.T) {
	const bitDepth = 16

	t.Run("EQ", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		// Set values.
		if _, err := f.setValue(tx, 1000, bitDepth, 382); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 2000, bitDepth, 300); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 3000, bitDepth, 2818); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 4000, bitDepth, 300); err != nil {
			t.Fatal(err)
		}

		// Query for equality.
		if b, err := f.rangeOp(tx, pql.EQ, bitDepth, 300); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{2000, 4000}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}
	})

	t.Run("EQOversizeRegression", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		// Set values.
		if _, err := f.setValue(tx, 1000, 1, 0); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 2000, 1, 1); err != nil {
			t.Fatal(err)
		}

		// Query for equality.
		if b, err := f.rangeOp(tx, pql.EQ, 1, 3); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}
		if b, err := f.rangeOp(tx, pql.EQ, 1, 4); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}
	})

	t.Run("NEQ", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		// Set values.
		if _, err := f.setValue(tx, 1000, bitDepth, 382); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 2000, bitDepth, 300); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 3000, bitDepth, 2818); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 4000, bitDepth, 300); err != nil {
			t.Fatal(err)
		}

		// Query for inequality.
		if b, err := f.rangeOp(tx, pql.NEQ, bitDepth, 300); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{1000, 3000}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}
	})

	t.Run("LT", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		// Set values.
		if _, err := f.setValue(tx, 1000, bitDepth, 382); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 2000, bitDepth, 300); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 3000, bitDepth, 2817); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 4000, bitDepth, 301); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 5000, bitDepth, 1); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 6000, bitDepth, 0); err != nil {
			t.Fatal(err)
		}

		// Query for values less than (ending with set column).
		if b, err := f.rangeOp(tx, pql.LT, bitDepth, 301); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{2000, 5000, 6000}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}

		// Query for values less than (ending with unset column).
		if b, err := f.rangeOp(tx, pql.LT, bitDepth, 300); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{5000, 6000}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}

		// Query for values less than or equal to (ending with set column).
		if b, err := f.rangeOp(tx, pql.LTE, bitDepth, 301); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{2000, 4000, 5000, 6000}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}

		// Query for values less than or equal to (ending with unset column).
		if b, err := f.rangeOp(tx, pql.LTE, bitDepth, 300); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{2000, 5000, 6000}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}
	})

	t.Run("LTRegression", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		if _, err := f.setValue(tx, 1, 1, 1); err != nil {
			t.Fatal(err)
		}

		if b, err := f.rangeOp(tx, pql.LT, 1, 2); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{1}) {
			t.Fatalf("unepxected coulmns: %+v", b.Columns())
		}
	})

	t.Run("LTMaxRegression", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		if _, err := f.setValue(tx, 1, 2, 3); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 2, 2, 0); err != nil {
			t.Fatal(err)
		}

		if b, err := f.rangeLTUnsigned(tx, NewRow(1, 2), 2, 3, false); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{2}) {
			t.Fatalf("unepxected coulmns: %+v", b.Columns())
		}
	})

	t.Run("GT", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		// Set values.
		if _, err := f.setValue(tx, 1000, bitDepth, 382); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 2000, bitDepth, 300); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 3000, bitDepth, 2817); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 4000, bitDepth, 301); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 5000, bitDepth, 1); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 6000, bitDepth, 0); err != nil {
			t.Fatal(err)
		}

		// Query for values greater than (ending with unset bit).
		if b, err := f.rangeOp(tx, pql.GT, bitDepth, 300); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{1000, 3000, 4000}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}

		// Query for values greater than (ending with set bit).
		if b, err := f.rangeOp(tx, pql.GT, bitDepth, 301); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{1000, 3000}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}

		// Query for values greater than or equal to (ending with unset bit).
		if b, err := f.rangeOp(tx, pql.GTE, bitDepth, 300); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{1000, 2000, 3000, 4000}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}

		// Query for values greater than or equal to (ending with set bit).
		if b, err := f.rangeOp(tx, pql.GTE, bitDepth, 301); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{1000, 3000, 4000}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}
	})

	t.Run("GTMinRegression", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		if _, err := f.setValue(tx, 1, 2, 0); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 2, 2, 1); err != nil {
			t.Fatal(err)
		}

		if b, err := f.rangeGTUnsigned(tx, NewRow(1, 2), 2, 0, false); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{2}) {
			t.Fatalf("unepxected coulmns: %+v", b.Columns())
		}
	})

	t.Run("GTOversizeRegression", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		if _, err := f.setValue(tx, 1, 2, 0); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 2, 2, 1); err != nil {
			t.Fatal(err)
		}

		if b, err := f.rangeGTUnsigned(tx, NewRow(1, 2), 2, 4, false); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{}) {
			t.Fatalf("unepxected coulmns: %+v", b.Columns())
		}
	})

	t.Run("BETWEEN", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		// Set values.
		if _, err := f.setValue(tx, 1000, bitDepth, 382); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 2000, bitDepth, 300); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 3000, bitDepth, 2817); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 4000, bitDepth, 301); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 5000, bitDepth, 1); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 6000, bitDepth, 0); err != nil {
			t.Fatal(err)
		}

		// Query for values greater than (ending with unset column).
		if b, err := f.rangeBetween(tx, bitDepth, 300, 2817); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{1000, 2000, 3000, 4000}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}

		// Query for values greater than (ending with set column).
		if b, err := f.rangeBetween(tx, bitDepth, 301, 2817); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{1000, 3000, 4000}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}

		// Query for values greater than or equal to (ending with unset column).
		if b, err := f.rangeBetween(tx, bitDepth, 301, 2816); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{1000, 4000}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}

		// Query for values greater than or equal to (ending with set column).
		if b, err := f.rangeBetween(tx, bitDepth, 300, 2816); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{1000, 2000, 4000}) {
			t.Fatalf("unexpected columns: %+v", b.Columns())
		}
	})

	t.Run("BetweenCommonBitsRegression", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		if _, err := f.setValue(tx, 1, 64, 0xf0); err != nil {
			t.Fatal(err)
		} else if _, err := f.setValue(tx, 2, 64, 0xf1); err != nil {
			t.Fatal(err)
		}

		if b, err := f.rangeBetweenUnsigned(tx, NewRow(1, 2), 64, 0xf0, 0xf1); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(b.Columns(), []uint64{1, 2}) {
			t.Fatalf("unepxected coulmns: %+v", b.Columns())
		}
	})
}

// benchmarkSetValues is a helper function to explore, very roughly, the cost
// of setting values.
func benchmarkSetValues(b *testing.B, bitDepth uint, f *fragment, cfunc func(uint64) uint64) {
	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	column := uint64(0)
	for i := 0; i < b.N; i++ {
		// We're not checking the error because this is a benchmark.
		// That does mean the result could be completely wrong...
		_, _ = f.setValue(tx, column, bitDepth, int64(i))
		column = cfunc(column)
	}
}

// Benchmark performance of setValue for BSI ranges.
func BenchmarkFragment_SetValue(b *testing.B) {
	depths := []uint{4, 8, 16}
	for _, bitDepth := range depths {
		name := fmt.Sprintf("Depth%d", bitDepth)
		f, idx := mustOpenFragment("i", "f", viewBSIGroupPrefix+"foo", 0, "none")
		_ = idx

		b.Run(name+"_Sparse", func(b *testing.B) {
			benchmarkSetValues(b, bitDepth, f, func(u uint64) uint64 { return (u + 70000) & (ShardWidth - 1) })
		})
		f.Clean(b)
		f, idx = mustOpenFragment("i", "f", viewBSIGroupPrefix+"foo", 0, "none")
		_ = idx
		b.Run(name+"_Dense", func(b *testing.B) {
			benchmarkSetValues(b, bitDepth, f, func(u uint64) uint64 { return (u + 1) & (ShardWidth - 1) })
		})
		f.Clean(b)
	}
}

// benchmarkImportValues is a helper function to explore, very roughly, the cost
// of setting values using the special setter used for imports.
func benchmarkImportValues(b *testing.B, bitDepth uint, f *fragment, cfunc func(uint64) uint64) {
	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	column := uint64(0)
	b.StopTimer()
	columns := make([]uint64, b.N)
	values := make([]int64, b.N)
	for i := 0; i < b.N; i++ {
		values[i] = int64(i)
		columns[i] = column
		column = cfunc(column)
	}
	b.StartTimer()
	err := f.importValue(tx, columns, values, bitDepth, false)
	if err != nil {
		b.Fatalf("error importing values: %s", err)
	}
}

// Benchmark performance of setValue for BSI ranges.
func BenchmarkFragment_ImportValue(b *testing.B) {
	depths := []uint{4, 8, 16}
	for _, bitDepth := range depths {
		name := fmt.Sprintf("Depth%d", bitDepth)
		f, idx := mustOpenBSIFragment("i", "f", viewBSIGroupPrefix+"foo", 0)
		_ = idx
		b.Run(name+"_Sparse", func(b *testing.B) {
			benchmarkImportValues(b, bitDepth, f, func(u uint64) uint64 { return (u + 70000) & (ShardWidth - 1) })
		})
		f.Clean(b)
		f, idx = mustOpenBSIFragment("i", "f", viewBSIGroupPrefix+"foo", 0)
		_ = idx
		b.Run(name+"_Dense", func(b *testing.B) {
			benchmarkImportValues(b, bitDepth, f, func(u uint64) uint64 { return (u + 1) & (ShardWidth - 1) })
		})
		f.Clean(b)
	}
}

// BenchmarkFragment_RepeatedSmallImports tests the situation where updates are
// constantly coming in across a large column space so that each fragment gets
// only a few updates during each import.
//
// We test a variety of combinations of the number of separate updates(imports),
// the number of bits in the import, the number of rows in the fragment (which
// is a pretty good proxy for fragment size on disk), and the MaxOpN on the
// fragment which controls how many set bits occur before a snapshot is done. If
// the number of bits in a given import is greater than MaxOpN, bulkImport will
// always go through the standard snapshotting import path.
func BenchmarkFragment_RepeatedSmallImports(b *testing.B) {
	for _, numUpdates := range []int{100} {
		for _, bitsPerUpdate := range []int{100, 1000} {
			for _, numRows := range []int{1000, 100000, 1000000} {
				for _, opN := range []int{1, 5000, 50000} {
					b.Run(fmt.Sprintf("Rows%dUpdates%dBits%dOpN%d", numRows, numUpdates, bitsPerUpdate, opN), func(b *testing.B) {
						for a := 0; a < b.N; a++ {
							b.StopTimer()
							// build the update data set all at once - this will get applied
							// to a fragment in numUpdates batches
							updateRows := make([]uint64, numUpdates*bitsPerUpdate)
							updateCols := make([]uint64, numUpdates*bitsPerUpdate)
							for i := 0; i < numUpdates*bitsPerUpdate; i++ {
								updateRows[i] = uint64(rand.Int63n(int64(numRows))) // row id
								updateCols[i] = uint64(rand.Int63n(ShardWidth))     // column id
							}
							f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
							_ = idx
							f.MaxOpN = opN
							defer f.Clean(b)

							// Obtain transaction.
							tx := f.txTestingOnly
							defer tx.Rollback()

							err := f.importRoaringT(tx, getZipfRowsSliceRoaring(uint64(numRows), 1, 0, ShardWidth), false)
							if err != nil {
								b.Fatalf("importing base data for benchmark: %v", err)
							}
							b.StartTimer()
							for i := 0; i < numUpdates; i++ {
								err := f.bulkImportStandard(tx,
									updateRows[bitsPerUpdate*i:bitsPerUpdate*(i+1)],
									updateRows[bitsPerUpdate*i:bitsPerUpdate*(i+1)],
									&ImportOptions{},
								)
								if err != nil {
									b.Fatalf("doing small bulk import: %v", err)
								}
							}
							tx.Rollback() // don't exhaust the Tx space under b.N iterations.
						}
					})
				}
			}
		}
	}
}

func BenchmarkFragment_RepeatedSmallImportsRoaring(b *testing.B) {
	for _, numUpdates := range []int{100} {
		for _, bitsPerUpdate := range []uint64{100, 1000} {
			for _, numRows := range []uint64{1000, 100000, 1000000} {
				for _, opN := range []int{1, 5000, 50000} {
					b.Run(fmt.Sprintf("Rows%dUpdates%dBits%dOpN%d", numRows, numUpdates, bitsPerUpdate, opN), func(b *testing.B) {
						for a := 0; a < b.N; a++ {
							b.StopTimer()
							// build the update data set all at once - this will get applied
							// to a fragment in numUpdates batches
							f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
							_ = idx
							f.MaxOpN = opN
							defer f.Clean(b)
							tx := f.txTestingOnly
							defer tx.Rollback()

							err := f.importRoaringT(tx, getZipfRowsSliceRoaring(numRows, 1, 0, ShardWidth), false)
							if err != nil {
								b.Fatalf("importing base data for benchmark: %v", err)
							}
							for i := 0; i < numUpdates; i++ {
								data := getUpdataRoaring(numRows, bitsPerUpdate, int64(i))
								b.StartTimer()
								err := f.importRoaringT(tx, data, false)
								b.StopTimer()
								if err != nil {
									b.Fatalf("doing small roaring import: %v", err)
								}
							}
						}
					})
				}
			}
		}
	}
}

func BenchmarkFragment_RepeatedSmallValueImports(b *testing.B) {
	initialCols := make([]uint64, 0, ShardWidth)
	initialVals := make([]int64, 0, ShardWidth)
	for i := uint64(0); i < ShardWidth; i++ {
		// every 29 columns, skip between 0 and 12 columns
		if i%29 == 0 {
			i += i % 13
		}
		initialCols = append(initialCols, i)
		initialVals = append(initialVals, int64(rand.Int63n(1<<21)))
	}

	for _, numUpdates := range []int{100} {
		for _, valsPerUpdate := range []int{10, 100} {
			updateCols := make([]uint64, numUpdates*valsPerUpdate)
			updateVals := make([]int64, numUpdates*valsPerUpdate)
			for i := 0; i < numUpdates*valsPerUpdate; i++ {
				updateCols[i] = uint64(rand.Int63n(ShardWidth))
				updateVals[i] = int64(rand.Int63n(1 << 21))
			}

			for _, opN := range []int{1, 5000, 50000} {
				b.Run(fmt.Sprintf("Updates%dVals%dOpN%d", numUpdates, valsPerUpdate, opN), func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						b.StopTimer()
						f, idx := mustOpenBSIFragment("i", "f", viewBSIGroupPrefix+"foo", 0)
						_ = idx
						f.MaxOpN = opN

						// Obtain transaction.
						tx := f.txTestingOnly
						defer tx.Rollback()

						err := f.importValue(tx, initialCols, initialVals, 21, false)
						if err != nil {
							b.Fatalf("initial value import: %v", err)
						}
						b.StartTimer()
						for j := 0; j < numUpdates; j++ {
							err := f.importValue(tx,
								updateCols[valsPerUpdate*j:valsPerUpdate*(j+1)],
								updateVals[valsPerUpdate*j:valsPerUpdate*(j+1)],
								21,
								false,
							)
							if err != nil {
								b.Fatalf("importing values: %v", err)
							}
						}
						tx.Rollback() // don't exhaust the Tx over the b.N iterations.
					}
				})
			}

		}
	}
}

// Ensure a fragment can snapshot correctly.
func TestFragment_Snapshot(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Set and then clear bits on the fragment.
	if _, err := f.setBit(tx, 1000, 1); err != nil {
		t.Fatal(err)
	} else if _, err := f.setBit(tx, 1000, 2); err != nil {
		t.Fatal(err)
	} else if _, err := f.clearBit(tx, 1000, 1); err != nil {
		t.Fatal(err)
	}
	panicOn(tx.Commit())
	tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
	defer tx.Rollback()

	// Snapshot bitmap and verify data.
	if err := f.Snapshot(); err != nil {
		t.Fatal(err)
	} else if n := f.mustRow(tx, 1000).Count(); n != 1 {
		t.Fatalf("unexpected count: %d", n)
	}

	// Close and reopen the fragment & verify the data.
	if err := f.Reopen(); err != nil {
		t.Fatal(err)
	} else if n := f.mustRow(tx, 1000).Count(); n != 1 {
		t.Fatalf("unexpected count (reopen): %d", n)
	}
}

// Ensure a fragment can iterate over all bits in order.
func TestFragment_ForEachBit(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Set bits on the fragment.
	if _, err := f.setBit(tx, 100, 20); err != nil {
		t.Fatal(err)
	} else if _, err := f.setBit(tx, 2, 38); err != nil {
		t.Fatal(err)
	} else if _, err := f.setBit(tx, 2, 37); err != nil {
		t.Fatal(err)
	}

	// Iterate over bits.
	var result [][2]uint64
	if err := f.forEachBit(tx, func(rowID, columnID uint64) error {
		result = append(result, [2]uint64{rowID, columnID})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Verify bits are correct.
	if !reflect.DeepEqual(result, [][2]uint64{{2, 37}, {2, 38}, {100, 20}}) {
		t.Fatalf("unexpected result: %#v", result)
	}
}

// Ensure a fragment can return the top n results.
func TestFragment_Top(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, CacheTypeRanked)
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Set bits on the rows 100, 101, & 102.
	f.mustSetBits(tx, 100, 1, 3, 200)
	f.mustSetBits(tx, 101, 1)
	f.mustSetBits(tx, 102, 1, 2)
	f.RecalculateCache()

	// Retrieve top rows.
	if pairs, err := f.top(tx, topOptions{N: 2}); err != nil {
		t.Fatal(err)
	} else if len(pairs) != 2 {
		t.Fatalf("unexpected count: %d", len(pairs))
	} else if pairs[0] != (Pair{ID: 100, Count: 3}) {
		t.Fatalf("unexpected pair(0): %v", pairs[0])
	} else if pairs[1] != (Pair{ID: 102, Count: 2}) {
		t.Fatalf("unexpected pair(1): %v", pairs[1])
	}
}

// Ensure a fragment can filter rows when retrieving the top n rows.
func TestFragment_Top_Filter(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, CacheTypeRanked)
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Set bits on the rows 100, 101, & 102.
	f.mustSetBits(tx, 100, 1, 3, 200)
	f.mustSetBits(tx, 101, 1)
	f.mustSetBits(tx, 102, 1, 2)
	f.RecalculateCache()
	// Assign attributes.
	err := f.RowAttrStore.SetAttrs(101, map[string]interface{}{"x": int64(10)})
	if err != nil {
		t.Fatalf("setAttrs: %v", err)
	}
	err = f.RowAttrStore.SetAttrs(102, map[string]interface{}{"x": int64(20)})
	if err != nil {
		t.Fatalf("setAttrs: %v", err)
	}

	panicOn(tx.Commit())
	tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
	defer tx.Rollback()

	// Retrieve top rows.
	if pairs, err := f.top(tx, topOptions{
		N:            2,
		FilterName:   "x",
		FilterValues: []interface{}{int64(10), int64(15), int64(20)},
	}); err != nil {
		t.Fatal(err)
	} else if len(pairs) != 2 {
		t.Fatalf("unexpected count: %d", len(pairs))
	} else if pairs[0] != (Pair{ID: 102, Count: 2}) {
		t.Fatalf("unexpected pair(0): %v", pairs[0])
	} else if pairs[1] != (Pair{ID: 101, Count: 1}) {
		t.Fatalf("unexpected pair(1): %v", pairs[1])
	}
}

// Ensure a fragment can return top rows that intersect with an input row.
func TestFragment_TopN_Intersect(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, CacheTypeRanked)
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Create an intersecting input row.
	src := NewRow(1, 2, 3)

	// Set bits on various rows.
	f.mustSetBits(tx, 100, 1, 10, 11, 12)    // one intersection
	f.mustSetBits(tx, 101, 1, 2, 3, 4)       // three intersections
	f.mustSetBits(tx, 102, 1, 2, 4, 5, 6)    // two intersections
	f.mustSetBits(tx, 103, 1000, 1001, 1002) // no intersection
	f.RecalculateCache()

	// Retrieve top rows.
	if pairs, err := f.top(tx, topOptions{N: 3, Src: src}); err != nil {
		t.Fatal(err)
	} else if !reflect.DeepEqual(pairs, []Pair{
		{ID: 101, Count: 3},
		{ID: 102, Count: 2},
		{ID: 100, Count: 1},
	}) {
		t.Fatalf("unexpected pairs: %s", spew.Sdump(pairs))
	}
}

// Ensure a fragment can return top rows that have many columns set.
func TestFragment_TopN_Intersect_Large(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}

	f, idx := mustOpenFragment("i", "f", viewStandard, 0, CacheTypeRanked)
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Create an intersecting input row.
	src := NewRow(
		980, 981, 982, 983, 984, 985, 986, 987, 988, 989,
		990, 991, 992, 993, 994, 995, 996, 997, 998, 999,
	)

	bm := roaring.NewBTreeBitmap()
	// Set bits on rows 0 - 999. Higher rows have higher bit counts.
	for i := uint64(0); i < 1000; i++ {
		for j := uint64(0); j < i; j++ {
			addToBitmap(bm, i, j)
		}
	}
	b := &bytes.Buffer{}
	_, err := bm.WriteTo(b)
	if err != nil {
		t.Fatalf("writing to bytes: %v", err)
	}
	err = f.importRoaringT(tx, b.Bytes(), false)
	if err != nil {
		t.Fatalf("importing data: %v", err)
	}
	f.RecalculateCache()

	// Retrieve top rows.
	if pairs, err := f.top(tx, topOptions{N: 10, Src: src}); err != nil {
		t.Fatal(err)
	} else if !reflect.DeepEqual(pairs, []Pair{
		{ID: 999, Count: 19},
		{ID: 998, Count: 18},
		{ID: 997, Count: 17},
		{ID: 996, Count: 16},
		{ID: 995, Count: 15},
		{ID: 994, Count: 14},
		{ID: 993, Count: 13},
		{ID: 992, Count: 12},
		{ID: 991, Count: 11},
		{ID: 990, Count: 10},
	}) {
		t.Fatalf("unexpected pairs: %s", spew.Sdump(pairs))
	}
}

// Ensure a fragment can return top rows when specified by ID.
func TestFragment_TopN_IDs(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, CacheTypeRanked)
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Set bits on various rows.
	f.mustSetBits(tx, 100, 1, 2, 3)
	f.mustSetBits(tx, 101, 4, 5, 6, 7)
	f.mustSetBits(tx, 102, 8, 9, 10, 11, 12)

	// Retrieve top rows.
	if pairs, err := f.top(tx, topOptions{RowIDs: []uint64{100, 101, 200}}); err != nil {
		t.Fatal(err)
	} else if !reflect.DeepEqual(pairs, []Pair{
		{ID: 101, Count: 4},
		{ID: 100, Count: 3},
	}) {
		t.Fatalf("unexpected pairs: %s", spew.Sdump(pairs))
	}
}

// Ensure a fragment return none if CacheTypeNone is set
func TestFragment_TopN_NopCache(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, CacheTypeNone)
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Set bits on various rows.
	f.mustSetBits(tx, 100, 1, 2, 3)
	f.mustSetBits(tx, 101, 4, 5, 6, 7)
	f.mustSetBits(tx, 102, 8, 9, 10, 11, 12)

	// Retrieve top rows.
	if pairs, err := f.top(tx, topOptions{RowIDs: []uint64{100, 101, 200}}); err != nil {
		t.Fatal(err)
	} else if !reflect.DeepEqual(pairs, []Pair{}) {
		t.Fatalf("unexpected pairs: %s", spew.Sdump(pairs))
	}
}

// Ensure the fragment cache limit works
func TestFragment_TopN_CacheSize(t *testing.T) {
	shard := uint64(0)
	cacheSize := uint32(3)

	// Create Index.
	index := mustOpenIndex(IndexOptions{})
	defer index.Close()

	// Create field.
	field, err := index.CreateFieldIfNotExists("f", OptFieldTypeSet(CacheTypeRanked, cacheSize))
	if err != nil {
		t.Fatal(err)
	}

	// Create view.
	view, err := field.createViewIfNotExists(viewStandard)
	if err != nil {
		t.Fatal(err)
	}

	// Create fragment.
	frag, err := view.CreateFragmentIfNotExists(shard)
	if err != nil {
		t.Fatal(err)
	}
	// Close the storage so we can re-open it without encountering a flock.
	frag.Close()

	f := frag
	if err := f.Open(); err != nil {
		panic(err)
	}
	defer f.Clean(t)

	// Obtain transaction.
	tx := index.Txf.NewTx(Txo{Write: writable, Index: index})
	defer tx.Rollback()

	// Set bits on various rows.
	f.mustSetBits(tx, 100, 1, 2, 3)
	f.mustSetBits(tx, 101, 4, 5, 6, 7)
	f.mustSetBits(tx, 102, 8, 9, 10, 11, 12)
	f.mustSetBits(tx, 103, 8, 9, 10, 11, 12, 13)
	f.mustSetBits(tx, 104, 8, 9, 10, 11, 12, 13, 14)
	f.mustSetBits(tx, 105, 10, 11)

	f.RecalculateCache()

	p := []Pair{
		{ID: 104, Count: 7},
		{ID: 103, Count: 6},
		{ID: 102, Count: 5},
	}

	// Retrieve top rows.
	if pairs, err := f.top(tx, topOptions{N: 5}); err != nil {
		t.Fatal(err)
	} else if len(pairs) > int(cacheSize) {
		t.Fatalf("TopN count cannot exceed cache size: %d", cacheSize)
	} else if pairs[0] != (Pair{ID: 104, Count: 7}) {
		t.Fatalf("unexpected pair(0): %v", pairs)
	} else if !reflect.DeepEqual(pairs, p) {
		t.Fatalf("Invalid TopN result set: %s", spew.Sdump(pairs))
	}
}

// Ensure fragment can return a checksum for its blocks.
func TestFragment_Checksum(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Retrieve checksum and set bits.
	orig, err := f.Checksum()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.setBit(tx, 1, 200); err != nil {
		t.Fatal(err)
	} else if _, err := f.setBit(tx, HashBlockSize*2, 200); err != nil {
		t.Fatal(err)
	}
	panicOn(tx.Commit())

	// Ensure new checksum is different.
	if chksum, err := f.Checksum(); err != nil {
		t.Fatal(err)
	} else if bytes.Equal(chksum, orig) {
		t.Fatalf("expected checksum to change: %x - %x", chksum, orig)
	}
}

// Ensure fragment can return a checksum for a given block.
func TestFragment_Blocks(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly

	// Retrieve initial checksum.
	var prev []FragmentBlock

	// Set first bit.
	if _, err := f.setBit(tx, 0, 0); err != nil {
		t.Fatal(err)
	}
	panicOn(tx.Commit())
	blocks, err := f.Blocks() // FAIL: TestFragment_Blocks b/c 0 blocks back
	if err != nil {
		t.Fatal(err)
	} else if blocks[0].Checksum == nil {
		t.Fatalf("expected checksum: %x", blocks[0].Checksum)
	}
	prev = blocks

	tx = idx.Txf.NewTx(Txo{Write: true, Index: idx, Fragment: f})
	// Set bit on different row.
	if _, err := f.setBit(tx, 20, 0); err != nil {
		t.Fatal(err)
	}
	panicOn(tx.Commit())
	blocks, err = f.Blocks()
	if err != nil {
		t.Fatal(err)
	} else if bytes.Equal(blocks[0].Checksum, prev[0].Checksum) {
		t.Fatalf("expected checksum to change: %x", blocks[0].Checksum)
	}
	prev = blocks

	// Set bit on different column.
	tx = idx.Txf.NewTx(Txo{Write: true, Index: idx, Fragment: f})
	f.txTestingOnly = tx // let the Clean do the Rollback
	if _, err := f.setBit(tx, 20, 100); err != nil {
		t.Fatal(err)
	}
	panicOn(tx.Commit())
	blocks, err = f.Blocks()
	if err != nil {
		t.Fatal(err)
	} else if bytes.Equal(blocks[0].Checksum, prev[0].Checksum) {
		t.Fatalf("expected checksum to change: %x", blocks[0].Checksum)
	}
}

// Ensure fragment returns an empty checksum if no data exists for a block.
func TestFragment_Blocks_Empty(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Set bits on a different block.
	if _, err := f.setBit(tx, 100, 1); err != nil {
		t.Fatal(err)
	}
	panicOn(tx.Commit()) // f.Blocks() will start a new Tx, so the SetBit needs to be visible before that.

	// Ensure checksum for block 1 is blank.
	if blocks, err := f.Blocks(); err != nil {
		t.Fatal(err)
	} else if len(blocks) != 1 {
		t.Fatalf("unexpected block count: %d", len(blocks))
	} else if blocks[0].ID != 1 {
		t.Fatalf("unexpected block id: %d", blocks[0].ID)
	}
}

// Ensure a fragment's cache can be persisted between restarts.
func TestFragment_LRUCache_Persistence(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, CacheTypeLRU)
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Set bits on the fragment.
	for i := uint64(0); i < 1000; i++ {
		if _, err := f.setBit(tx, i, 0); err != nil {
			t.Fatal(err)
		}
	}

	// Verify correct cache type and size.
	if cache, ok := f.cache.(*lruCache); !ok {
		t.Fatalf("unexpected cache: %T", f.cache)
	} else if cache.Len() != 1000 {
		t.Fatalf("unexpected cache len: %d", cache.Len())
	}

	panicOn(tx.Commit())

	// Reopen the fragment.
	if err := f.Reopen(); err != nil {
		t.Fatal(err)
	}

	// Re-verify correct cache type and size.
	if cache, ok := f.cache.(*lruCache); !ok {
		t.Fatalf("unexpected cache: %T", f.cache)
	} else if cache.Len() != 1000 {
		t.Fatalf("unexpected cache len: %d", cache.Len())
	}
}

// Ensure a fragment's cache can be persisted between restarts.
func TestFragment_RankCache_Persistence(t *testing.T) {
	roaringOnlyTest(t)

	index := mustOpenIndex(IndexOptions{})
	defer index.Close()

	// Create field.
	field, err := index.CreateFieldIfNotExists("f", OptFieldTypeSet(CacheTypeRanked, DefaultCacheSize))
	if err != nil {
		t.Fatal(err)
	}

	// Create view.
	view, err := field.createViewIfNotExists(viewStandard)
	if err != nil {
		t.Fatal(err)
	}

	// Create fragment.
	f, err := view.CreateFragmentIfNotExists(0)
	if err != nil {
		t.Fatal(err)
	}

	// Obtain transaction.
	tx := index.Txf.NewTx(Txo{Write: writable, Index: index})
	defer tx.Rollback()

	// Set bits on the fragment.
	for i := uint64(0); i < 1000; i++ {
		if _, err := f.setBit(tx, i, 0); err != nil {
			t.Fatal(err)
		}
	}

	panicOn(tx.Commit())
	tx = index.Txf.NewTx(Txo{Write: !writable, Index: index, Fragment: f})
	defer tx.Rollback()

	// Verify correct cache type and size.
	if cache, ok := f.cache.(*rankCache); !ok {
		t.Fatalf("unexpected cache: %T", f.cache)
	} else if cache.Len() != 1000 {
		t.Fatalf("unexpected cache len: %d", cache.Len())
	}

	// Reopen the index.
	if err := index.reopen(); err != nil {
		t.Fatal(err)
	}

	// Re-fetch fragment.
	f = index.Field("f").view(viewStandard).Fragment(0)

	// Re-verify correct cache type and size.
	if cache, ok := f.cache.(*rankCache); !ok {
		t.Fatalf("unexpected cache: %T", f.cache)
	} else if cache.Len() != 1000 {
		t.Fatalf("unexpected cache len: %d", cache.Len())
	}
}

func roaringOnlyTest(t *testing.T) {
	if os.Getenv("PILOSA_TXSRC") != "roaring" {
		t.Skip("skip for everything but roaring")
	}
}

func roaringOnlyBenchmark(b *testing.B) {
	if os.Getenv("PILOSA_TXSRC") != "roaring" {
		b.Skip("skip for everything but roaring")
	}
}

// Ensure a fragment can be copied to another fragment.
func TestFragment_WriteTo_ReadFrom(t *testing.T) {
	roaringOnlyTest(t)

	f0, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	_ = idx
	defer f0.Clean(t)

	tx := f0.txTestingOnly
	defer tx.Rollback()

	// Set and then clear bits on the fragment.
	if _, err := f0.setBit(tx, 1000, 1); err != nil {
		t.Fatal(err)
	} else if _, err := f0.setBit(tx, 1000, 2); err != nil {
		t.Fatal(err)
	} else if _, err := f0.clearBit(tx, 1000, 1); err != nil {
		t.Fatal(err)
	}

	// Verify cache is populated.
	if n := f0.cache.Len(); n != 1 {
		t.Fatalf("unexpected cache size: %d", n)
	}

	// Write fragment to a buffer.
	var buf bytes.Buffer
	wn, err := f0.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}

	// Read into another fragment.
	f1, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	_ = idx
	defer f1.Clean(t)

	if rn, err := f1.ReadFrom(&buf); err != nil { // eventually calls fragment.fillFragmentFromArchive
		t.Fatal(err)
	} else if wn != rn {
		t.Fatalf("read/write byte count mismatch: wn=%d, rn=%d", wn, rn)
	}

	// Verify cache is in other fragment.
	if n := f1.cache.Len(); n != 1 {
		t.Fatalf("unexpected cache size: %d", n)
	}

	// Verify data in other fragment.
	if a := f1.mustRow(tx, 1000).Columns(); !reflect.DeepEqual(a, []uint64{2}) {
		t.Fatalf("unexpected columns: %+v", a)
	}

	// Close and reopen the fragment & verify the data.
	if err := f1.Reopen(); err != nil {
		t.Fatal(err)
	} else if n := f1.cache.Len(); n != 1 {
		t.Fatalf("unexpected cache size (reopen): %d", n)
	} else if a := f1.mustRow(tx, 1000).Columns(); !reflect.DeepEqual(a, []uint64{2}) {
		t.Fatalf("unexpected columns (reopen): %+v", a)
	}
}

func BenchmarkFragment_Blocks(b *testing.B) {
	if *FragmentPath == "" {
		b.Skip("no fragment specified")
	}

	// Open the fragment specified by the path.
	f := newFragment(NewHolder(DefaultPartitionN), *FragmentPath, "i", "f", viewStandard, 0, 0)
	if err := f.Open(); err != nil {
		b.Fatal(err)
	}
	defer f.CleanKeep(b)

	// Reset timer and execute benchmark.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if a, err := f.Blocks(); err != nil {
			b.Fatal(err)
		} else if len(a) == 0 {
			b.Fatal("no blocks in fragment")
		}
	}
}

func BenchmarkFragment_IntersectionCount(b *testing.B) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	defer f.Clean(b)
	f.MaxOpN = math.MaxInt32

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Generate some intersecting data.
	for i := 0; i < 10000; i += 2 {
		if _, err := f.setBit(tx, 1, uint64(i)); err != nil {
			b.Fatal(err)
		}
	}
	for i := 0; i < 10000; i += 3 {
		if _, err := f.setBit(tx, 2, uint64(i)); err != nil {
			b.Fatal(err)
		}
	}

	panicOn(tx.Commit())
	tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
	defer tx.Rollback()

	// Snapshot to disk before benchmarking.
	if err := f.Snapshot(); err != nil {
		b.Fatal(err)
	}

	// Start benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if n := f.mustRow(tx, 1).intersectionCount(f.mustRow(tx, 2)); n == 0 {
			b.Fatalf("unexpected count: %d", n)
		}
	}
}

func TestFragment_Tanimoto(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, CacheTypeRanked)
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	src := NewRow(1, 2, 3)

	// Set bits on the rows 100, 101, & 102.
	f.mustSetBits(tx, 100, 1, 3, 2, 200)
	f.mustSetBits(tx, 101, 1, 3)
	f.mustSetBits(tx, 102, 1, 2, 10, 12)
	f.RecalculateCache()

	if pairs, err := f.top(tx, topOptions{TanimotoThreshold: 50, Src: src}); err != nil {
		t.Fatal(err)
	} else if len(pairs) != 2 {
		t.Fatalf("unexpected count: %d", len(pairs))
	} else if pairs[0] != (Pair{ID: 100, Count: 3}) {
		t.Fatalf("unexpected pair(0): %v", pairs[0])
	} else if pairs[1] != (Pair{ID: 101, Count: 2}) {
		t.Fatalf("unexpected pair(1): %v", pairs[1])
	}
}

func TestFragment_Zero_Tanimoto(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, CacheTypeRanked)
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	src := NewRow(1, 2, 3)

	// Set bits on the rows 100, 101, & 102.
	f.mustSetBits(tx, 100, 1, 3, 2, 200)
	f.mustSetBits(tx, 101, 1, 3)
	f.mustSetBits(tx, 102, 1, 2, 10, 12)
	f.RecalculateCache()

	if pairs, err := f.top(tx, topOptions{TanimotoThreshold: 0, Src: src}); err != nil {
		t.Fatal(err)
	} else if len(pairs) != 3 {
		t.Fatalf("unexpected count: %d", len(pairs))
	} else if pairs[0] != (Pair{ID: 100, Count: 3}) {
		t.Fatalf("unexpected pair(0): %v", pairs[0])
	} else if pairs[1] != (Pair{ID: 101, Count: 2}) {
		t.Fatalf("unexpected pair(1): %v", pairs[1])
	} else if pairs[2] != (Pair{ID: 102, Count: 2}) {
		t.Fatalf("unexpected pair(1): %v", pairs[2])
	}
}

func TestFragment_Snapshot_Run(t *testing.T) {
	roaringOnlyTest(t)

	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Set bits on the fragment.
	for i := uint64(1); i < 3; i++ {
		if _, err := f.setBit(tx, 1000, i); err != nil {
			t.Fatal(err)
		}
	}

	// Snapshot bitmap and verify data.
	if err := f.Snapshot(); err != nil {
		t.Fatal(err)
	} else if n := f.mustRow(tx, 1000).Count(); n != 2 {
		t.Fatalf("unexpected count: %d", n)
	}

	// Close and reopen the fragment & verify the data.
	if err := f.Reopen(); err != nil {
		t.Fatal(err)
	} else if n := f.mustRow(tx, 1000).Count(); n != 2 {
		t.Fatalf("unexpected count (reopen): %d", n)
	}
}

// Ensure a fragment can set mutually exclusive values.
func TestFragment_SetMutex(t *testing.T) {
	f, idx := mustOpenMutexFragment("i", "f", viewStandard, 0, "")
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	var cols []uint64

	// Set a value on column 100.
	if _, err := f.setBit(tx, 1, 100); err != nil {
		t.Fatal(err)
	}
	// Verify the value was set.
	cols = f.mustRow(tx, 1).Columns()
	if !reflect.DeepEqual(cols, []uint64{100}) {
		t.Fatalf("mutex unexpected columns: %v", cols)
	}

	// Set a different value on column 100.
	if _, err := f.setBit(tx, 2, 100); err != nil {
		t.Fatal(err)
	}
	// Verify that value (row 1) was replaced (by row 2).
	cols = f.mustRow(tx, 1).Columns()
	if !reflect.DeepEqual(cols, []uint64{}) {
		t.Fatalf("mutex unexpected columns: %v", cols)
	}
	cols = f.mustRow(tx, 2).Columns()
	if !reflect.DeepEqual(cols, []uint64{100}) {
		t.Fatalf("mutex unexpected columns: %v", cols)
	}
}

// Ensure a fragment can import into set fields.
func TestFragment_ImportSet(t *testing.T) {
	tests := []struct {
		setRowIDs   []uint64
		setColIDs   []uint64
		setExp      map[uint64][]uint64
		clearRowIDs []uint64
		clearColIDs []uint64
		clearExp    map[uint64][]uint64
	}{
		{
			[]uint64{1, 1, 1, 1},
			[]uint64{0, 1, 2, 3},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
			},
			[]uint64{},
			[]uint64{},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
			},
		},
		{
			[]uint64{1, 1, 1, 1, 2, 2, 2, 2},
			[]uint64{0, 1, 2, 3, 0, 1, 2, 3},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
				2: {0, 1, 2, 3},
			},
			[]uint64{1, 1, 2},
			[]uint64{1, 2, 3},
			map[uint64][]uint64{
				1: {0, 3},
				2: {0, 1, 2},
			},
		},
		{
			[]uint64{1, 1, 1, 1, 2},
			[]uint64{0, 1, 2, 3, 1},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
				2: {1},
			},
			[]uint64{1, 1, 1, 1},
			[]uint64{0, 1, 2, 3},
			map[uint64][]uint64{
				1: {},
				2: {1},
			},
		},
		{
			[]uint64{1, 1, 1, 1, 2, 2, 1},
			[]uint64{0, 1, 2, 3, 1, 8, 1},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
				2: {1, 8},
			},
			[]uint64{1, 1},
			[]uint64{0, 0},
			map[uint64][]uint64{
				1: {1, 2, 3},
				2: {1, 8},
			},
		},
		{
			[]uint64{1, 2, 3},
			[]uint64{8, 8, 8},
			map[uint64][]uint64{
				1: {8},
				2: {8},
				3: {8},
			},
			[]uint64{1, 2, 3},
			[]uint64{9, 9, 9},
			map[uint64][]uint64{
				1: {8},
				2: {8},
				3: {8},
			},
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("importset%d", i), func(t *testing.T) {
			f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
			_ = idx
			defer f.Clean(t)

			// Obtain transaction.
			tx := f.txTestingOnly
			defer tx.Rollback()

			// Set import.
			err := f.bulkImport(tx, test.setRowIDs, test.setColIDs, &ImportOptions{})
			if err != nil {
				t.Fatalf("bulk importing ids: %v", err)
			}

			// Check for expected results.
			for k, v := range test.setExp {
				cols := f.mustRow(tx, k).Columns()
				if !reflect.DeepEqual(cols, v) {
					t.Fatalf("expected: %v, but got: %v", v, cols)
				}
			}

			// Clear import.
			err = f.bulkImport(tx, test.clearRowIDs, test.clearColIDs, &ImportOptions{Clear: true})
			if err != nil {
				t.Fatalf("bulk clearing ids: %v", err)
			}

			// Check for expected results.
			for k, v := range test.clearExp {
				cols := f.mustRow(tx, k).Columns()
				if !reflect.DeepEqual(cols, v) {
					t.Fatalf("expected: %v, but got: %v", v, cols)
				}
			}
		})
	}
}

func TestFragment_ImportSet_WithTxCommit(t *testing.T) {
	tests := []struct {
		setRowIDs   []uint64
		setColIDs   []uint64
		setExp      map[uint64][]uint64
		clearRowIDs []uint64
		clearColIDs []uint64
		clearExp    map[uint64][]uint64
	}{
		{
			[]uint64{1, 1, 1, 1},
			[]uint64{0, 1, 2, 3},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
			},
			[]uint64{},
			[]uint64{},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
			},
		},
		{
			[]uint64{1, 1, 1, 1, 2, 2, 2, 2},
			[]uint64{0, 1, 2, 3, 0, 1, 2, 3},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
				2: {0, 1, 2, 3},
			},
			[]uint64{1, 1, 2},
			[]uint64{1, 2, 3},
			map[uint64][]uint64{
				1: {0, 3},
				2: {0, 1, 2},
			},
		},
		{
			[]uint64{1, 1, 1, 1, 2},
			[]uint64{0, 1, 2, 3, 1},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
				2: {1},
			},
			[]uint64{1, 1, 1, 1},
			[]uint64{0, 1, 2, 3},
			map[uint64][]uint64{
				1: {},
				2: {1},
			},
		},
		{
			[]uint64{1, 1, 1, 1, 2, 2, 1},
			[]uint64{0, 1, 2, 3, 1, 8, 1},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
				2: {1, 8},
			},
			[]uint64{1, 1},
			[]uint64{0, 0},
			map[uint64][]uint64{
				1: {1, 2, 3},
				2: {1, 8},
			},
		},
		{
			[]uint64{1, 2, 3},
			[]uint64{8, 8, 8},
			map[uint64][]uint64{
				1: {8},
				2: {8},
				3: {8},
			},
			[]uint64{1, 2, 3},
			[]uint64{9, 9, 9},
			map[uint64][]uint64{
				1: {8},
				2: {8},
				3: {8},
			},
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("importset%d", i), func(t *testing.T) {
			f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
			_ = idx
			defer f.Clean(t)

			// Obtain transaction.
			tx := f.txTestingOnly
			defer tx.Rollback()

			// Set import.
			err := f.bulkImport(tx, test.setRowIDs, test.setColIDs, &ImportOptions{})
			if err != nil {
				t.Fatalf("bulk importing ids: %v", err)
			}

			panicOn(tx.Commit())
			tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
			defer tx.Rollback()

			// Check for expected results.
			for k, v := range test.setExp {
				cols := f.mustRow(tx, k).Columns()
				if !reflect.DeepEqual(cols, v) {
					t.Fatalf("expected: %v, but got: %v", v, cols)
				}
			}

			panicOn(tx.Commit())
			tx = idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
			defer tx.Rollback()

			// Clear import.
			err = f.bulkImport(tx, test.clearRowIDs, test.clearColIDs, &ImportOptions{Clear: true})
			if err != nil {
				t.Fatalf("bulk clearing ids: %v", err)
			}

			panicOn(tx.Commit())
			tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
			defer tx.Rollback()

			// Check for expected results.
			for k, v := range test.clearExp {
				cols := f.mustRow(tx, k).Columns()
				if !reflect.DeepEqual(cols, v) {
					t.Fatalf("expected: %v, but got: %v", v, cols)
				}
			}
		})
	}
}

func TestFragment_ConcurrentImport(t *testing.T) {
	t.Run("bulkImportStandard", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		defer f.Clean(t)

		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()

		eg := errgroup.Group{}
		eg.Go(func() error { return f.bulkImportStandard(tx, []uint64{1, 2}, []uint64{1, 2}, &ImportOptions{}) })
		eg.Go(func() error { return f.bulkImportStandard(tx, []uint64{3, 4}, []uint64{3, 4}, &ImportOptions{}) })
		err := eg.Wait()
		if err != nil {
			t.Fatalf("importing data to fragment: %v", err)
		}
	})
}

// Ensure a fragment can import mutually exclusive values.
func TestFragment_ImportMutex(t *testing.T) {
	tests := []struct {
		setRowIDs   []uint64
		setColIDs   []uint64
		setExp      map[uint64][]uint64
		clearRowIDs []uint64
		clearColIDs []uint64
		clearExp    map[uint64][]uint64
	}{
		{
			[]uint64{1, 1, 1, 1},
			[]uint64{0, 1, 2, 3},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
			},
			[]uint64{},
			[]uint64{},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
			},
		},
		{
			[]uint64{1, 1, 1, 1, 2, 2, 2, 2},
			[]uint64{0, 1, 2, 3, 0, 1, 2, 3},
			map[uint64][]uint64{
				1: {},
				2: {0, 1, 2, 3},
			},
			[]uint64{1, 1, 2},
			[]uint64{1, 2, 3},
			map[uint64][]uint64{
				1: {},
				2: {0, 1, 2},
			},
		},
		{
			[]uint64{1, 1, 1, 1, 2},
			[]uint64{0, 1, 2, 3, 1},
			map[uint64][]uint64{
				1: {0, 2, 3},
				2: {1},
			},
			[]uint64{1, 1, 1, 1},
			[]uint64{0, 1, 2, 3},
			map[uint64][]uint64{
				1: {},
				2: {1},
			},
		},
		{
			[]uint64{1, 1, 1, 1, 2, 2, 1},
			[]uint64{0, 1, 2, 3, 1, 8, 1},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
				2: {8},
			},
			[]uint64{1, 1},
			[]uint64{0, 0},
			map[uint64][]uint64{
				1: {1, 2, 3},
				2: {8},
			},
		},
		{
			[]uint64{1, 2, 3},
			[]uint64{8, 8, 8},
			map[uint64][]uint64{
				1: {},
				2: {},
				3: {8},
			},
			[]uint64{1, 2, 3},
			[]uint64{9, 9, 9},
			map[uint64][]uint64{
				1: {},
				2: {},
				3: {8},
			},
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("importmutex%d", i), func(t *testing.T) {
			f, idx := mustOpenMutexFragment("i", "f", viewStandard, 0, "")
			_ = idx
			defer f.Clean(t)

			// Obtain transaction.
			tx := f.txTestingOnly
			defer tx.Rollback()

			// Set import.
			err := f.bulkImport(tx, test.setRowIDs, test.setColIDs, &ImportOptions{})
			if err != nil {
				t.Fatalf("bulk importing ids: %v", err)
			}

			// Check for expected results.
			for k, v := range test.setExp {
				cols := f.mustRow(tx, k).Columns()
				if !reflect.DeepEqual(cols, v) {
					t.Fatalf("row: %d, expected: %v, but got: %v", k, v, cols)
				}
			}

			// Clear import.
			err = f.bulkImport(tx, test.clearRowIDs, test.clearColIDs, &ImportOptions{Clear: true})
			if err != nil {
				t.Fatalf("bulk clearing ids: %v", err)
			}

			// Check for expected results.
			for k, v := range test.clearExp {
				cols := f.mustRow(tx, k).Columns()
				if !reflect.DeepEqual(cols, v) {
					t.Fatalf("row: %d expected: %v, but got: %v", k, v, cols)
				}
			}
		})
	}
}

// Ensure a fragment can import mutually exclusive values.
// Now with Commits in the middle.
func TestFragment_ImportMutex_WithTxCommit(t *testing.T) {
	tests := []struct {
		setRowIDs   []uint64
		setColIDs   []uint64
		setExp      map[uint64][]uint64
		clearRowIDs []uint64
		clearColIDs []uint64
		clearExp    map[uint64][]uint64
	}{
		{
			[]uint64{1, 1, 1, 1},
			[]uint64{0, 1, 2, 3},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
			},
			[]uint64{},
			[]uint64{},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
			},
		},
		{
			[]uint64{1, 1, 1, 1, 2, 2, 2, 2},
			[]uint64{0, 1, 2, 3, 0, 1, 2, 3},
			map[uint64][]uint64{
				1: {},
				2: {0, 1, 2, 3},
			},
			[]uint64{1, 1, 2},
			[]uint64{1, 2, 3},
			map[uint64][]uint64{
				1: {},
				2: {0, 1, 2},
			},
		},
		{
			[]uint64{1, 1, 1, 1, 2},
			[]uint64{0, 1, 2, 3, 1},
			map[uint64][]uint64{
				1: {0, 2, 3},
				2: {1},
			},
			[]uint64{1, 1, 1, 1},
			[]uint64{0, 1, 2, 3},
			map[uint64][]uint64{
				1: {},
				2: {1},
			},
		},
		{
			[]uint64{1, 1, 1, 1, 2, 2, 1},
			[]uint64{0, 1, 2, 3, 1, 8, 1},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
				2: {8},
			},
			[]uint64{1, 1},
			[]uint64{0, 0},
			map[uint64][]uint64{
				1: {1, 2, 3},
				2: {8},
			},
		},
		{
			[]uint64{1, 2, 3},
			[]uint64{8, 8, 8},
			map[uint64][]uint64{
				1: {},
				2: {},
				3: {8},
			},
			[]uint64{1, 2, 3},
			[]uint64{9, 9, 9},
			map[uint64][]uint64{
				1: {},
				2: {},
				3: {8},
			},
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("importmutex%d", i), func(t *testing.T) {
			f, idx := mustOpenMutexFragment("i", "f", viewStandard, 0, "")
			defer f.Clean(t)

			// Obtain transaction.
			tx := f.txTestingOnly
			defer tx.Rollback()

			// Set import.
			err := f.bulkImport(tx, test.setRowIDs, test.setColIDs, &ImportOptions{})
			if err != nil {
				t.Fatalf("bulk importing ids: %v", err)
			}

			panicOn(tx.Commit())
			tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
			defer tx.Rollback()

			// Check for expected results.
			for k, v := range test.setExp {
				cols := f.mustRow(tx, k).Columns()
				if !reflect.DeepEqual(cols, v) {
					t.Fatalf("row: %d, expected: %v, but got: %v", k, v, cols)
				}
			}

			panicOn(tx.Commit())
			tx = idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
			defer tx.Rollback()

			// Clear import.
			err = f.bulkImport(tx, test.clearRowIDs, test.clearColIDs, &ImportOptions{Clear: true})
			if err != nil {
				t.Fatalf("bulk clearing ids: %v", err)
			}

			panicOn(tx.Commit())
			tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
			defer tx.Rollback()

			// Check for expected results.
			for k, v := range test.clearExp {
				cols := f.mustRow(tx, k).Columns()
				if !reflect.DeepEqual(cols, v) {
					t.Fatalf("row: %d expected: %v, but got: %v", k, v, cols)
				}
			}
		})
	}
}

// Ensure a fragment can import bool values.
func TestFragment_ImportBool(t *testing.T) {
	tests := []struct {
		setRowIDs   []uint64
		setColIDs   []uint64
		setExp      map[uint64][]uint64
		clearRowIDs []uint64
		clearColIDs []uint64
		clearExp    map[uint64][]uint64
	}{
		{
			[]uint64{1, 1, 1, 1},
			[]uint64{0, 1, 2, 3},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
			},
			[]uint64{},
			[]uint64{},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
			},
		},
		{
			[]uint64{0, 0, 0, 0, 1, 1, 1, 1},
			[]uint64{0, 1, 2, 3, 0, 1, 2, 3},
			map[uint64][]uint64{
				0: {},
				1: {0, 1, 2, 3},
			},
			[]uint64{1, 1, 2},
			[]uint64{1, 2, 3},
			map[uint64][]uint64{
				0: {},
				1: {0, 3},
				2: {},
			},
		},
		{
			[]uint64{0, 0, 0, 0, 1},
			[]uint64{0, 1, 2, 3, 1},
			map[uint64][]uint64{
				0: {0, 2, 3},
				1: {1},
			},
			[]uint64{1, 1, 1, 1},
			[]uint64{0, 1, 2, 3},
			map[uint64][]uint64{
				0: {0, 2, 3},
				1: {},
			},
		},
		{
			[]uint64{1, 1, 1, 1, 0, 0, 1},
			[]uint64{0, 1, 2, 3, 1, 8, 1},
			map[uint64][]uint64{
				0: {8},
				1: {0, 1, 2, 3},
			},
			[]uint64{1, 1},
			[]uint64{0, 0},
			map[uint64][]uint64{
				0: {8},
				1: {1, 2, 3},
			},
		},
		{
			[]uint64{0, 1, 2},
			[]uint64{8, 8, 8},
			map[uint64][]uint64{
				0: {},
				1: {}, // This isn't {8} because fragment doesn't validate bool values.
				2: {8},
			},
			[]uint64{1, 2, 3},
			[]uint64{9, 9, 9},
			map[uint64][]uint64{
				0: {},
				1: {},
				2: {8},
			},
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("importmutex%d", i), func(t *testing.T) {
			f, idx := mustOpenBoolFragment("i", "f", viewStandard, 0, "")
			_ = idx
			defer f.Clean(t)

			// Obtain transaction.
			tx := f.txTestingOnly
			defer tx.Rollback()

			// Set import.
			err := f.bulkImport(tx, test.setRowIDs, test.setColIDs, &ImportOptions{})
			if err != nil {
				t.Fatalf("bulk importing ids: %v", err)
			}

			// Check for expected results.
			for k, v := range test.setExp {
				cols := f.mustRow(tx, k).Columns()
				if !reflect.DeepEqual(cols, v) {
					t.Fatalf("expected: %v, but got: %v", v, cols)
				}
			}

			// Clear import.
			err = f.bulkImport(tx, test.clearRowIDs, test.clearColIDs, &ImportOptions{Clear: true})
			if err != nil {
				t.Fatalf("bulk importing ids: %v", err)
			}

			// Check for expected results.
			for k, v := range test.clearExp {
				cols := f.mustRow(tx, k).Columns()
				if !reflect.DeepEqual(cols, v) {
					t.Fatalf("expected: %v, but got: %v", v, cols)
				}
			}
		})
	}
}

// Ensure a fragment can import bool values, with Commits in between writes and reads.
func TestFragment_ImportBool_WithTxCommit(t *testing.T) {
	tests := []struct {
		setRowIDs   []uint64
		setColIDs   []uint64
		setExp      map[uint64][]uint64
		clearRowIDs []uint64
		clearColIDs []uint64
		clearExp    map[uint64][]uint64
	}{
		{
			[]uint64{1, 1, 1, 1},
			[]uint64{0, 1, 2, 3},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
			},
			[]uint64{},
			[]uint64{},
			map[uint64][]uint64{
				1: {0, 1, 2, 3},
			},
		},
		{
			[]uint64{0, 0, 0, 0, 1, 1, 1, 1},
			[]uint64{0, 1, 2, 3, 0, 1, 2, 3},
			map[uint64][]uint64{
				0: {},
				1: {0, 1, 2, 3},
			},
			[]uint64{1, 1, 2},
			[]uint64{1, 2, 3},
			map[uint64][]uint64{
				0: {},
				1: {0, 3},
				2: {},
			},
		},
		{
			[]uint64{0, 0, 0, 0, 1},
			[]uint64{0, 1, 2, 3, 1},
			map[uint64][]uint64{
				0: {0, 2, 3},
				1: {1},
			},
			[]uint64{1, 1, 1, 1},
			[]uint64{0, 1, 2, 3},
			map[uint64][]uint64{
				0: {0, 2, 3},
				1: {},
			},
		},
		{
			[]uint64{1, 1, 1, 1, 0, 0, 1},
			[]uint64{0, 1, 2, 3, 1, 8, 1},
			map[uint64][]uint64{
				0: {8},
				1: {0, 1, 2, 3},
			},
			[]uint64{1, 1},
			[]uint64{0, 0},
			map[uint64][]uint64{
				0: {8},
				1: {1, 2, 3},
			},
		},
		{
			[]uint64{0, 1, 2},
			[]uint64{8, 8, 8},
			map[uint64][]uint64{
				0: {},
				1: {}, // This isn't {8} because fragment doesn't validate bool values.
				2: {8},
			},
			[]uint64{1, 2, 3},
			[]uint64{9, 9, 9},
			map[uint64][]uint64{
				0: {},
				1: {},
				2: {8},
			},
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("importmutex%d", i), func(t *testing.T) {
			f, idx := mustOpenBoolFragment("i", "f", viewStandard, 0, "")
			_ = idx
			defer f.Clean(t)

			// Obtain transaction.
			tx := f.txTestingOnly
			defer tx.Rollback()

			// Set import.
			err := f.bulkImport(tx, test.setRowIDs, test.setColIDs, &ImportOptions{})
			if err != nil {
				t.Fatalf("bulk importing ids: %v", err)
			}

			panicOn(tx.Commit())
			tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
			defer tx.Rollback()

			// Check for expected results.
			for k, v := range test.setExp {
				cols := f.mustRow(tx, k).Columns()
				if !reflect.DeepEqual(cols, v) {
					t.Fatalf("expected: %v, but got: %v", v, cols)
				}
			}

			panicOn(tx.Commit())
			tx = idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
			defer tx.Rollback()

			// Clear import.
			err = f.bulkImport(tx, test.clearRowIDs, test.clearColIDs, &ImportOptions{Clear: true})
			if err != nil {
				t.Fatalf("bulk importing ids: %v", err)
			}

			panicOn(tx.Commit())
			tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
			defer tx.Rollback()

			// Check for expected results.
			for k, v := range test.clearExp {
				cols := f.mustRow(tx, k).Columns()
				if !reflect.DeepEqual(cols, v) {
					t.Fatalf("expected: %v, but got: %v", v, cols)
				}
			}
		})
	}
}

func BenchmarkFragment_Snapshot(b *testing.B) {
	if *FragmentPath == "" {
		b.Skip("no fragment specified")
	}

	b.ReportAllocs()
	// Open the fragment specified by the path.
	f := newFragment(NewHolder(DefaultPartitionN), *FragmentPath, "i", "f", viewStandard, 0, 0)
	if err := f.Open(); err != nil {
		b.Fatal(err)
	}
	defer f.CleanKeep(b)
	b.ResetTimer()

	// Reset timer and execute benchmark.
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		err := f.Snapshot()
		if err != nil {
			b.Fatalf("unexpected count (reopen): %s", err)
		}
	}
}

func BenchmarkFragment_FullSnapshot(b *testing.B) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	_ = idx
	defer f.Clean(b)

	// Generate some intersecting data.
	maxX := ShardWidth / 2
	sz := maxX
	rows := make([]uint64, sz)
	cols := make([]uint64, sz)

	options := &ImportOptions{}
	max := 0
	for row := 0; row < 100; row++ {
		val := 1
		i := 0
		for col := 0; col < ShardWidth/2; col++ {
			rows[i] = uint64(row)
			cols[i] = uint64(val)
			val += 2
			i++
		}

		tx := idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
		defer tx.Rollback()

		if err := f.bulkImport(tx, rows, cols, options); err != nil {
			b.Fatalf("Error Building Sample: %s", err)
		}
		panicOn(tx.Commit())
		if row > max {
			max = row
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := f.Snapshot(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFragment_Import(b *testing.B) {
	b.StopTimer()
	maxX := ShardWidth * 5 * 2
	sz := maxX
	rows := make([]uint64, sz)
	cols := make([]uint64, sz)
	i := 0
	for row := 0; row < 100; row++ {
		val := 1
		for col := 0; col < ShardWidth/2; col++ {
			rows[i] = uint64(row)
			cols[i] = uint64(val)
			val += 2
			i++
		}
		if i == maxX {
			break
		}
	}
	rowsUse, colsUse := make([]uint64, len(rows)), make([]uint64, len(cols))
	b.ReportAllocs()
	options := &ImportOptions{}
	for i := 0; i < b.N; i++ {
		// since bulkImport modifies the input slices, we make new copies for each round
		copy(rowsUse, rows)
		copy(colsUse, cols)
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		// Obtain transaction.
		tx := f.txTestingOnly
		defer tx.Rollback()
		b.StartTimer()
		if err := f.bulkImport(tx, rowsUse, colsUse, options); err != nil {
			b.Errorf("Error Building Sample: %s", err)
		}
		b.StopTimer()
		tx.Rollback()
		f.Clean(b)
	}
}

var (
	rowCases         = []uint64{2, 50, 1000, 10000, 100000}
	colCases         = []uint64{20, 1000, 5000, 50000, 500000}
	concurrencyCases = []int{2, 4, 16}
	cacheCases       = []string{CacheTypeNone, CacheTypeRanked}
)

func BenchmarkImportRoaring(b *testing.B) {
	for _, numRows := range rowCases {
		data := getZipfRowsSliceRoaring(numRows, 1, 0, ShardWidth)
		b.Logf("%dRows: %.2fMB\n", numRows, float64(len(data))/1024/1024)
		for _, cacheType := range cacheCases {
			b.Run(fmt.Sprintf("Rows%dCache_%s", numRows, cacheType), func(b *testing.B) {
				b.StopTimer()
				for i := 0; i < b.N; i++ {
					f, _ := mustOpenFragment("i", fmt.Sprintf("r%dc%s", numRows, cacheType), viewStandard, 0, cacheType)
					b.StartTimer()
					tx := f.txTestingOnly
					defer tx.Rollback()

					err := f.importRoaringT(tx, data, false)
					if err != nil {
						// we don't actually particularly
						// care whether this succeeds,
						// but if it's happening we want
						// it to be done.
						_ = defaultSnapshotQueue.Await(f)
						f.Clean(b)
						b.Fatalf("import error: %v", err)
					}
					b.StopTimer()
					f.Clean(b)
				}
			})
		}
	}
}

func BenchmarkImportRoaringConcurrent(b *testing.B) {
	if testing.Short() {
		b.SkipNow()
	}
	for _, numRows := range rowCases {
		data := make([][]byte, 0, len(concurrencyCases))
		data = append(data, getZipfRowsSliceRoaring(numRows, 0, 0, ShardWidth))
		b.Logf("%dRows: %.2fMB\n", numRows, float64(len(data[0]))/1024/1024)
		for _, concurrency := range concurrencyCases {
			// add more data sets
			for j := len(data); j < concurrency; j++ {
				data = append(data, getZipfRowsSliceRoaring(numRows, int64(j), 0, ShardWidth))
			}
			for _, cacheType := range cacheCases {
				b.Run(fmt.Sprintf("Rows%dConcurrency%dCache_%s", numRows, concurrency, cacheType), func(b *testing.B) {
					b.StopTimer()
					frags := make([]*fragment, concurrency)
					for i := 0; i < b.N; i++ {
						for j := 0; j < concurrency; j++ {
							frags[j], _ = mustOpenFragment("i", "f", viewStandard, uint64(j), cacheType)
						}
						eg := errgroup.Group{}
						b.StartTimer()
						for j := 0; j < concurrency; j++ {
							j := j
							eg.Go(func() error {
								tx := frags[j].txTestingOnly
								defer tx.Rollback()

								err := frags[j].importRoaringT(tx, data[j], false)
								// error unimportant if it happened, but we want
								// any snapshots to have finished.
								_ = defaultSnapshotQueue.Await(frags[j])
								return err
							})
						}
						err := eg.Wait()
						if err != nil {
							b.Errorf("importing fragment: %v", err)
						}
						b.StopTimer()
						for j := 0; j < concurrency; j++ {
							frags[j].Clean(b)
						}
					}
				})
			}
		}
	}
}
func BenchmarkImportRoaringUpdateConcurrent(b *testing.B) {
	roaringOnlyBenchmark(b)
	if testing.Short() {
		b.SkipNow()
	}
	for _, numRows := range rowCases {
		for _, numCols := range colCases {
			data := getZipfRowsSliceRoaring(numRows, 1, 0, ShardWidth)
			updata := getUpdataRoaring(numRows, numCols, 1)
			for _, concurrency := range concurrencyCases {
				for _, cacheType := range cacheCases {
					b.Run(fmt.Sprintf("Rows%dCols%dConcurrency%dCache_%s", numRows, numCols, concurrency, cacheType), func(b *testing.B) {
						b.StopTimer()
						frags := make([]*fragment, concurrency)
						for i := 0; i < b.N; i++ {
							for j := 0; j < concurrency; j++ {
								frags[j], _ = mustOpenFragment("i", "f", viewStandard, uint64(j), cacheType)

								// the cost of actually doing the op log for the large initial data set
								// is excessive. force storage into snapshotted state, then use import
								// to generate an op log and/or snapshot.
								// note: skipped for badger, above.
								_, _, err := frags[j].storage.ImportRoaringBits(data, false, false, 0)
								if err != nil {
									b.Fatalf("importing roaring: %v", err)
								}
								err = defaultSnapshotQueue.Immediate(frags[j])
								if err != nil {
									b.Fatalf("snapshot after import: %v", err)
								}
							}
							eg := errgroup.Group{}
							b.StartTimer()
							for j := 0; j < concurrency; j++ {
								j := j
								eg.Go(func() error {
									tx := frags[j].txTestingOnly
									defer tx.Rollback()

									err := frags[j].importRoaringT(tx, updata, false)
									err2 := defaultSnapshotQueue.Await(frags[j])
									if err == nil {
										err = err2
									}
									return err
								})
							}
							err := eg.Wait()
							if err != nil {
								b.Errorf("importing fragment: %v", err)
							}
							b.StopTimer()
							for j := 0; j < concurrency; j++ {
								frags[j].Clean(b)
							}
						}
					})
				}
			}
		}
	}
}

func BenchmarkImportStandard(b *testing.B) {
	for _, cacheType := range cacheCases {
		for _, numRows := range rowCases {
			rowIDsOrig, columnIDsOrig := getZipfRowsSliceStandard(numRows, 1, 0, ShardWidth)
			rowIDs, columnIDs := make([]uint64, len(rowIDsOrig)), make([]uint64, len(columnIDsOrig))
			b.Run(fmt.Sprintf("Rows%dCache_%s", numRows, cacheType), func(b *testing.B) {
				b.StopTimer()
				for i := 0; i < b.N; i++ {
					copy(rowIDs, rowIDsOrig)
					copy(columnIDs, columnIDsOrig)
					f, idx := mustOpenFragment("i", fmt.Sprintf("r%dc%s", numRows, cacheType), viewStandard, 0, cacheType)
					_ = idx
					// Obtain transaction.
					tx := f.txTestingOnly
					defer tx.Rollback()

					b.StartTimer()
					err := f.bulkImport(tx, rowIDs, columnIDs, &ImportOptions{})
					if err != nil {
						b.Errorf("import error: %v", err)
					}
					b.StopTimer()
					tx.Rollback()
					f.Clean(b)
				}
			})
		}
	}
}

func BenchmarkImportRoaringUpdate(b *testing.B) {
	fileSize := make(map[string]int64)
	names := []string{}
	for _, numRows := range rowCases {
		data := getZipfRowsSliceRoaring(numRows, 1, 0, ShardWidth)
		for _, numCols := range colCases {
			updata := getUpdataRoaring(numRows, numCols, 1)
			for _, cacheType := range cacheCases {
				name := fmt.Sprintf("Rows%dCols%dCache_%s", numRows, numCols, cacheType)
				names = append(names, name)
				b.Run(name, func(b *testing.B) {
					b.StopTimer()
					for i := 0; i < b.N; i++ {
						f, idx := mustOpenFragment("i", fmt.Sprintf("r%dc%dcache_%s", numRows, numCols, cacheType), viewStandard, 0, cacheType)
						_ = idx
						tx := f.txTestingOnly
						defer tx.Rollback()

						// the cost of actually doing the op log for the large initial data set
						// is excessive. force storage into snapshotted state, then use import
						// to generate an op log and/or snapshot.
						itr, err := roaring.NewRoaringIterator(data)
						panicOn(err)
						_, _, err = tx.ImportRoaringBits(f.index, f.field, f.view, f.shard, itr, false, false, 0, nil)
						if err != nil {
							b.Errorf("import error: %v", err)
						}
						err = defaultSnapshotQueue.Immediate(f)
						if err != nil {
							b.Errorf("snapshot after import error: %v", err)
						}
						b.StartTimer()
						err = f.importRoaringT(tx, updata, false)
						if err != nil {
							f.Clean(b)
							b.Errorf("import error: %v", err)
						}
						err = defaultSnapshotQueue.Await(f)
						if err != nil {
							b.Errorf("snapshot after import error: %v", err)
						}
						b.StopTimer()
						var stat os.FileInfo
						var statTarget io.Writer
						err = f.gen.Transaction(&statTarget, func() error {
							targetFile, ok := statTarget.(*os.File)
							if ok {
								stat, _ = targetFile.Stat()
							} else {
								b.Errorf("couldn't stat file")
							}
							return nil
						})
						if err != nil {
							b.Errorf("transaction error: %v", err)
						}
						fileSize[name] = stat.Size()
						f.Clean(b)
					}
				})

			}
		}
	}
	for _, name := range names {
		b.Logf("%s: %.2fMB\n", name, float64(fileSize[name])/1024/1024)
	}
}

// BenchmarkUpdatePathological imports more data into an existing fragment than
// already exists, but the larger data comes after the smaller data. If
// SliceContainers are in use, this can cause horrible performance.
func BenchmarkUpdatePathological(b *testing.B) {
	exists := getZipfRowsSliceRoaring(100000, 1, 0, 400000)
	inc := getZipfRowsSliceRoaring(100000, 2, 400000, ShardWidth)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, DefaultCacheType)
		_ = idx
		tx := f.txTestingOnly
		defer tx.Rollback()
		err := f.importRoaringT(tx, exists, false)
		if err != nil {
			b.Fatalf("importing roaring: %v", err)
		}
		b.StartTimer()
		err = f.importRoaringT(tx, inc, false)
		if err != nil {
			b.Fatalf("importing second: %v", err)
		}

	}

}

var bigFrag string

func initBigFrag() {
	if bigFrag == "" {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, DefaultCacheType)
		_ = idx
		tx := f.txTestingOnly
		defer tx.Rollback()
		for i := int64(0); i < 10; i++ {
			// 10 million rows, 1 bit per column, random seeded by i
			data := getZipfRowsSliceRoaring(10000000, i, 0, ShardWidth)
			err := f.importRoaringT(tx, data, false)
			if err != nil {
				panic(fmt.Sprintf("setting up fragment data: %v", err))
			}
		}
		err := f.Close()
		if err != nil {
			panic(fmt.Sprintf("closing fragment: %v", err))
		}
		bigFrag = f.path
		panicOn(tx.Commit())
	}
}

func BenchmarkImportIntoLargeFragment(b *testing.B) {
	b.StopTimer()
	initBigFrag()
	rowsOrig, colsOrig := getUpdataSlices(10000000, 11000, 0)
	rows, cols := make([]uint64, len(rowsOrig)), make([]uint64, len(colsOrig))
	opts := &ImportOptions{}
	for i := 0; i < b.N; i++ {
		origF, err := os.Open(bigFrag)
		if err != nil {
			b.Fatalf("opening frag file: %v", err)
		}
		fi, err := ioutil.TempFile(*TempDir, "")
		if err != nil {
			b.Fatalf("getting temp file: %v", err)
		}
		_, err = io.Copy(fi, origF)
		if err != nil {
			b.Fatalf("copying fragment file: %v", err)
		}
		origF.Close()
		fi.Close()

		h := NewHolder(DefaultPartitionN)
		h.Path = fi.Name()
		idx, err := h.CreateIndex("i", IndexOptions{})
		panicOn(err)

		nf := newFragment(h, fi.Name(), "i", "f", viewStandard, 0, 0)
		err = nf.Open()
		if err != nil {
			b.Fatalf("opening fragment: %v", err)
		}

		// Obtain transaction.
		tx := idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: nf})
		defer tx.Rollback()

		copy(rows, rowsOrig)
		copy(cols, colsOrig)
		b.StartTimer()
		err = nf.bulkImport(tx, rows, cols, opts)
		b.StopTimer()
		if err != nil {
			b.Fatalf("bulkImport: %v", err)
		}
		panicOn(tx.Commit())
		nf.Clean(b)
	}
}

func BenchmarkImportRoaringIntoLargeFragment(b *testing.B) {
	b.StopTimer()
	initBigFrag()
	updata := getUpdataRoaring(10000000, 11000, 0)
	for i := 0; i < b.N; i++ {
		origF, err := os.Open(bigFrag)
		if err != nil {
			b.Fatalf("opening frag file: %v", err)
		}
		fi, err := ioutil.TempFile(*TempDir, "")
		if err != nil {
			b.Fatalf("getting temp file: %v", err)
		}
		_, err = io.Copy(fi, origF)
		if err != nil {
			b.Fatalf("copying fragment file: %v", err)
		}
		origF.Close()
		fi.Close()

		// want to do this, but no path argument.
		//nf, idx := mustOpenFragmentFlags(index, field, view string, shard uint64, cacheType string, flags byte)

		th := newTestHolder()
		idx := fragTestMustOpenIndex(filepath.Dir(fi.Name()), "i", th, IndexOptions{})
		if th.NeedsSnapshot() {
			th.SnapshotQueue = newSnapshotQueue(1, 1, nil)
		}
		nf := newFragment(th, fi.Name(), "i", "f", viewStandard, 0, 0)

		tx := idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: nf})
		defer tx.Rollback()

		err = nf.Open()
		if err != nil {
			b.Fatalf("opening fragment: %v", err)
		}
		b.StartTimer()
		err = nf.importRoaringT(tx, updata, false)
		b.StopTimer()
		if err != nil {
			b.Fatalf("bulkImport: %v", err)
		}

		nf.Clean(b)
	}
}

func TestGetZipfRowsSliceRoaring(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, DefaultCacheType)
	_ = idx

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	data := getZipfRowsSliceRoaring(10, 1, 0, ShardWidth)
	err := f.importRoaringT(tx, data, false)
	if err != nil {
		t.Fatalf("importing roaring: %v", err)
	}
	rows, err := f.rows(context.Background(), tx, 0)
	if err != nil {
		t.Fatal(err)
	} else if !reflect.DeepEqual(rows, []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}) {
		t.Fatalf("unexpected rows: %v", rows)
	}
	for i := uint64(1); i < 10; i++ {
		if f.mustRow(tx, i).Count() >= f.mustRow(tx, i-1).Count() {
			t.Fatalf("suspect distribution from getZipfRowsSliceRoaring")
		}
	}
	f.Clean(t)
}

// getZipfRowsSliceRoaring generates a random fragment with the given number of
// rows, and 1 bit set in each column. The row each bit is set in is chosen via
// the Zipf generator, and so will be skewed toward lower row numbers. If this
// is edited to change the data distribution, getZipfRowsSliceStandard should be
// edited as well. TODO switch to generating row-major for perf boost
func getZipfRowsSliceRoaring(numRows uint64, seed int64, startCol, endCol uint64) []byte {
	b := roaring.NewBTreeBitmap()
	s := rand.NewSource(seed)
	r := rand.New(s)
	z := rand.NewZipf(r, 1.6, 50, numRows-1)
	bufSize := 1 << 14
	posBuf := make([]uint64, 0, bufSize)
	for i := uint64(startCol); i < endCol; i++ {
		row := z.Uint64()
		posBuf = append(posBuf, row*ShardWidth+i)
		if len(posBuf) == bufSize {
			sort.Slice(posBuf, func(i int, j int) bool { return posBuf[i] < posBuf[j] })
			b.DirectAddN(posBuf...)
		}
	}
	b.DirectAddN(posBuf...)
	buf := bytes.NewBuffer(make([]byte, 0, 100000))
	_, err := b.WriteTo(buf)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func getUpdataSlices(numRows, numCols uint64, seed int64) (rows, cols []uint64) {
	getUpdataInto(func(row, col uint64) bool {
		rows = append(rows, row)
		cols = append(cols, col)
		return true
	}, numRows, numCols, seed)
	return rows, cols
}

// getUpdataRoaring gets a byte slice containing a roaring bitmap which
// represents numCols set bits distributed randomly throughout a shard's column
// space and zipfianly throughout numRows rows.
func getUpdataRoaring(numRows, numCols uint64, seed int64) []byte {
	b := roaring.NewBitmap()

	getUpdataInto(func(row, col uint64) bool {
		return b.DirectAdd(row*ShardWidth + col)
	}, numRows, numCols, seed)

	buf := bytes.NewBuffer(make([]byte, 0, 100000))
	_, err := b.WriteTo(buf)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func getUpdataInto(f func(row, col uint64) bool, numRows, numCols uint64, seed int64) int {
	s := rand.NewSource(seed)
	r := rand.New(s)
	z := rand.NewZipf(r, 1.6, 50, numRows-1)

	i := uint64(0)
	// ensure we get exactly the number we asked for. it turns out we had
	// a horrible pathological edge case for exactly 10,000 entries in an
	// imported bitmap, and hit it only occasionally...
	for i < numCols {
		col := uint64(r.Int63n(ShardWidth))
		row := z.Uint64()
		if f(row, col) {
			i++
		}
	}
	return int(i)
}

// getZipfRowsSliceStandard is the same as getZipfRowsSliceRoaring, but returns
// row and column ids instead of a byte slice containing roaring bitmap data.
func getZipfRowsSliceStandard(numRows uint64, seed int64, startCol, endCol uint64) (rowIDs, columnIDs []uint64) {
	s := rand.NewSource(seed)
	r := rand.New(s)
	z := rand.NewZipf(r, 1.6, 50, numRows-1)
	rowIDs, columnIDs = make([]uint64, ShardWidth), make([]uint64, ShardWidth)
	for i := uint64(startCol); i < endCol; i++ {
		rowIDs[i] = z.Uint64()
		columnIDs[i] = i
	}
	return rowIDs, columnIDs
}

func BenchmarkFileWrite(b *testing.B) {
	for _, numRows := range rowCases {
		data := getZipfRowsSliceRoaring(numRows, 1, 0, ShardWidth)
		b.Run(fmt.Sprintf("Rows%d", numRows), func(b *testing.B) {
			b.StopTimer()
			for i := 0; i < b.N; i++ {
				f, err := ioutil.TempFile(*TempDir, "")
				if err != nil {
					b.Fatalf("getting temp file: %v", err)
				}
				b.StartTimer()
				_, err = f.Write(data)
				if err != nil {
					b.Fatal(err)
				}
				err = f.Sync()
				if err != nil {
					b.Fatal(err)
				}
				err = f.Close()
				if err != nil {
					b.Fatal(err)
				}
				b.StopTimer()
				os.Remove(f.Name())
			}
		})
	}

}

/////////////////////////////////////////////////////////////////////

// not called under badger b/c f.idx.NeedsSnapshot() in Clean() avoids it.
func (f *fragment) sanityCheck(t testing.TB) {
	newBM := roaring.NewFileBitmap()
	file, err := os.Open(f.path)
	if err != nil {
		t.Fatalf("sanityCheck couldn't open file %s: %v", f.path, err)
	}
	defer file.Close()
	data, err := ioutil.ReadAll(file)
	if err != nil {
		t.Fatalf("sanityCheck couldn't read fragment %s: %v", f.path, err)
	}
	err = newBM.UnmarshalBinary(data)
	if err != nil {
		t.Fatalf("sanityCheck couldn't unmarshal fragment %s: %v", f.path, err)
	}
	// Refactor fragment.storage
	// note: not called for badger, see above.
	if equal, reason := newBM.BitwiseEqual(f.storage); !equal {
		t.Fatalf("fragment %s: unmarshalled bitmap different: %v", f.path, reason)
	}
}

func (f *fragment) Clean(t testing.TB) {
	f.mu.Lock()
	// we need to ensure that we unlock the mutex before terminating
	// the clean operation, but we need it held during the sanity
	// check or else, in some cases, the background snapshot queue
	// can decide to pick it up.
	func() {
		// should we skip snapshot queue stuff under badger/rbf?
		defer f.mu.Unlock()

		// badger doesn't need snapshot, so this stuff is skipped.
		// The snapshot queue stuff doesn't work under badger.
		if f.idx.NeedsSnapshot() {
			err := defaultSnapshotQueue.Await(f)
			if err != nil {
				t.Fatalf("snapshot failed before sanity check: %v", err)
			}
			f.sanityCheck(t)
			if f.storage != nil && f.storage.Source != nil {
				if f.storage.Source.Dead() {
					t.Fatalf("cleaning up fragment %s, source %s, source already dead", f.path, f.storage.Source.ID())
				}
			}
		}
	}()
	if f.txTestingOnly != nil {
		f.txTestingOnly.Rollback()
		panicOn(f.idx.Txf.CloseIndex(f.idx))
	}
	errc := f.Close()
	// prevent double-closes of generation during testing.
	f.gen = nil
	var errf error
	if FileExists(f.path) {
		errf = os.Remove(f.path) // remove /var/folders/2x/hm9gp5ys3k9gmm5f_vzm_6wc0000gn/T/pilosa-index-768377904/i/f/views/standard/fragments/0: no such file or directory
	}
	errp := os.Remove(f.cachePath())
	if errc != nil || errf != nil {
		t.Fatal("cleaning up fragment: ", errc, errf, errp)
	}
	// not all fragments have cache files
	if errp != nil && !os.IsNotExist(errp) {
		t.Fatalf("cleaning up fragment cache: %v", errp)
	}
}

// importRoaringT calls importRoaring with context.Background() for convenience
func (f *fragment) importRoaringT(tx Tx, data []byte, clear bool) error {

	return f.importRoaring(context.Background(), tx, data, clear)
}

// CleanKeep is just like Clean(), but it doesn't remove the
// fragment file (note that it DOES remove the cache file).
func (f *fragment) CleanKeep(t testing.TB) {
	errc := f.Close()
	errp := os.Remove(f.cachePath())
	if errc != nil {
		t.Fatal("closing fragment: ", errc, errp)
	}
	// not all fragments have cache files
	if errp != nil && !os.IsNotExist(errp) {
		t.Fatalf("cleaning up fragment cache: %v", errp)
	}
}

// mustOpenFragment returns a new instance of Fragment with a temporary path.
func mustOpenFragment(index, field, view string, shard uint64, cacheType string) (*fragment, *Index) {
	return mustOpenFragmentFlags(index, field, view, shard, cacheType, 0)
}

func mustOpenBSIFragment(index, field, view string, shard uint64) (*fragment, *Index) {
	return mustOpenFragmentFlags(index, field, view, shard, "", 1)
}

func newTestHolder() *Holder {
	h := NewHolder(DefaultPartitionN)
	//h.SnapshotQueue = newSnapshotQueue(1, 1, nil)
	return h
}

// fragTestMustOpenIndex returns a new, opened index at a temporary path. Panic on error.
func fragTestMustOpenIndex(holderDir, index string, holder *Holder, opt IndexOptions) *Index {
	holder.Path = holderDir
	holder.mu.Lock()
	idx, err := holder.createIndex(index, opt)
	holder.mu.Unlock()
	panicOn(err)

	idx.keys = opt.Keys
	idx.trackExistence = opt.TrackExistence

	if err := idx.Open(false); err != nil {
		panic(err)
	}
	return idx
}

// mustOpenFragment returns a new instance of Fragment with a temporary path.
func mustOpenFragmentFlags(index, field, view string, shard uint64, cacheType string, flags byte) (*fragment, *Index) {

	holderDir, err := ioutil.TempDir(*TempDir, "holder-dir")
	panicOn(err)

	if cacheType == "" {
		cacheType = DefaultCacheType
	}

	th := newTestHolder()
	idx := fragTestMustOpenIndex(holderDir, index, th, IndexOptions{})
	if th.NeedsSnapshot() {
		th.SnapshotQueue = newSnapshotQueue(1, 1, nil)
	}

	fragDir := fmt.Sprintf("%v/%v/views/%v/fragments/", idx.path, field, view)
	panicOn(os.MkdirAll(fragDir, 0777))
	fragPath := fragDir + fmt.Sprintf("%v", shard)
	f := newFragment(th, fragPath, index, field, view, shard, flags)

	tx := idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
	f.txTestingOnly = tx

	f.CacheType = cacheType
	f.RowAttrStore = &memAttrStore{
		store: make(map[uint64]map[string]interface{}),
	}

	if err := f.Open(); err != nil {
		panic(err)
	}
	return f, idx
}

// mustOpenMutexFragment returns a new instance of Fragment for a mutex field.
func mustOpenMutexFragment(index, field, view string, shard uint64, cacheType string) (*fragment, *Index) {
	frag, idx := mustOpenFragment(index, field, view, shard, cacheType)
	frag.mutexVector = newRowsVector(frag)
	return frag, idx
}

// mustOpenBoolFragment returns a new instance of Fragment for a bool field.
func mustOpenBoolFragment(index, field, view string, shard uint64, cacheType string) (*fragment, *Index) {
	frag, idx := mustOpenFragment(index, field, view, shard, cacheType)
	frag.mutexVector = newBoolVector(frag)
	return frag, idx
}

// Reopen closes the fragment and reopens it as a new instance.
func (f *fragment) Reopen() error {
	if err := f.Close(); err != nil {
		return err
	}
	if err := f.Open(); err != nil {
		return err
	}
	return nil
}

// mustSetBits sets columns on a row. Panic on error.
// This function does not accept a timestamp or quantum.
func (f *fragment) mustSetBits(tx Tx, rowID uint64, columnIDs ...uint64) {
	for _, columnID := range columnIDs {
		if _, err := f.setBit(tx, rowID, columnID); err != nil {
			panic(err)
		}
	}
}

func addToBitmap(bm *roaring.Bitmap, rowID uint64, columnIDs ...uint64) {
	// we'll reuse the columnIDs slice and fill it with positions for DirectAddN
	for i, c := range columnIDs {
		columnIDs[i] = rowID*ShardWidth + c%ShardWidth
	}
	bm.DirectAddN(columnIDs...)
}

// Test Various methods of retrieving RowIDs
func TestFragment_RowsIteration(t *testing.T) {
	t.Run("firstContainer", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		tx := f.txTestingOnly
		defer tx.Rollback()
		defer f.Clean(t)

		expectedAll := make([]uint64, 0)
		expectedOdd := make([]uint64, 0)
		for i := uint64(100); i < uint64(200); i++ {
			if _, err := f.setBit(tx, i, i%2); err != nil {
				t.Fatal(err)
			}
			expectedAll = append(expectedAll, i)
			if i%2 == 1 {
				expectedOdd = append(expectedOdd, i)
			}
		}

		ids, err := f.rows(context.Background(), tx, 0)
		if err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(expectedAll, ids) {
			t.Fatalf("Do not match %v %v", expectedAll, ids)
		}

		ids, err = f.rows(context.Background(), tx, 0, filterColumn(1))
		if err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(expectedOdd, ids) {
			t.Fatalf("Do not match %v %v", expectedOdd, ids)
		}
	})

	t.Run("secondRow", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		tx := f.txTestingOnly
		defer tx.Rollback()
		defer f.Clean(t)

		expected := []uint64{1, 2}
		if _, err := f.setBit(tx, 1, 66000); err != nil {
			t.Fatal(err)
		} else if _, err := f.setBit(tx, 2, 66000); err != nil {
			t.Fatal(err)
		} else if _, err := f.setBit(tx, 2, 166000); err != nil {
			t.Fatal(err)
		}
		panicOn(tx.Commit())

		tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
		defer tx.Rollback()

		ids, err := f.rows(context.Background(), tx, 0)
		if err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(expected, ids) {
			t.Fatalf("Do not match %v %v", expected, ids)
		}

		ids, err = f.rows(context.Background(), tx, 0, filterColumn(66000))
		if err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(expected, ids) {
			t.Fatalf("Do not match %v %v", expected, ids)
		}
	})

	t.Run("combinations", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
		_ = idx
		tx := f.txTestingOnly
		defer tx.Rollback()
		defer f.Clean(t)

		expectedRows := make([]uint64, 0)
		for r := uint64(1); r < uint64(10000); r += 250 {
			expectedRows = append(expectedRows, r)
			for c := uint64(1); c < uint64(ShardWidth-1); c += (ShardWidth >> 5) {
				if _, err := f.setBit(tx, r, c); err != nil {
					t.Fatal(err)
				}

				ids, err := f.rows(context.Background(), tx, 0)
				if err != nil {
					t.Fatal(err)
				} else if !reflect.DeepEqual(expectedRows, ids) {
					t.Fatalf("Do not match %v %v", expectedRows, ids)
				}
				ids, err = f.rows(context.Background(), tx, 0, filterColumn(c))
				if err != nil {
					t.Fatal(err)
				} else if !reflect.DeepEqual(expectedRows, ids) {
					t.Fatalf("Do not match %v %v", expectedRows, ids)
				}
			}
		}
	})
}

// Test Importing roaring data.
func TestFragment_RoaringImport(t *testing.T) {
	tests := [][][]uint64{
		{
			[]uint64{0},
			[]uint64{1},
		},
		{
			[]uint64{0, 65535, 65536, 65537, 65538, 65539, 130000},
			[]uint64{1000, 67000, 130000},
		},
		{
			[]uint64{0, 65535, 65536, 65537, 65538, 65539, 130000},
			[]uint64{0, 65535, 65536, 65537, 65538, 65539, 130000},
		},
		{
			[]uint64{0, 65535, 65536, 1<<20 + 1, 1<<20*2 + 1},
			[]uint64{1, 1<<20 + 2, 1<<20*2 + 2},
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("importroaring%d", i), func(t *testing.T) {
			f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
			_ = idx
			defer f.Clean(t)
			tx := f.txTestingOnly
			defer tx.Rollback()

			for num, input := range test {
				buf := &bytes.Buffer{}
				bm := roaring.NewBitmap(input...)
				_, err := bm.WriteTo(buf)
				if err != nil {
					t.Fatalf("writing to buffer: %v", err)
				}
				err = f.importRoaringT(tx, buf.Bytes(), false)
				if err != nil {
					t.Fatalf("importing roaring: %v", err)
				}

				exp := calcExpected(test[:num+1]...)
				for row, expCols := range exp {
					cols := f.mustRow(tx, uint64(row)).Columns()
					t.Logf("\nrow: %d\n  exp:%v\n  got:%v", row, expCols, cols)
					if !reflect.DeepEqual(cols, expCols) {
						t.Fatalf("input%d, row %d\n  exp:%v\n  got:%v", num, row, expCols, cols)
					}
				}
			}
		})
	}
}

// Test Importing roaring data.
func TestFragment_RoaringImportTopN(t *testing.T) {
	tests := []struct {
		rowIDs  []uint64
		colIDs  []uint64
		rowIDs2 []uint64
		colIDs2 []uint64
		roaring []uint64
	}{
		{
			rowIDs:  []uint64{4, 4, 4, 4},
			colIDs:  []uint64{0, 1, 2, 3},
			rowIDs2: []uint64{5, 5, 5, 5, 5},
			colIDs2: []uint64{0, 1, 2, 3, 4},
			roaring: []uint64{0, 65535, 65536, 1<<20 + 1, 1<<20 + 2, 1<<20*2 + 1},
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("importroaring%d", i), func(t *testing.T) {
			f, idx := mustOpenFragment("i", "f", viewStandard, 0, CacheTypeRanked)
			_ = idx
			defer f.Clean(t)

			tx := f.txTestingOnly
			defer tx.Rollback()

			options := &ImportOptions{}
			err := f.bulkImport(tx, test.rowIDs, test.colIDs, options)
			if err != nil {
				t.Fatalf("bulk importing ids: %v", err)
			}

			expPairs := calcTop(test.rowIDs, test.colIDs)
			pairs, err := f.top(tx, topOptions{})
			if err != nil {
				t.Fatalf("executing top after bulk import: %v", err)
			}
			if !reflect.DeepEqual(expPairs, pairs) {
				t.Fatalf("post bulk import:\n  exp: %v\n  got: %v\n", expPairs, pairs)
			}

			err = f.bulkImport(tx, test.rowIDs2, test.colIDs2, options)
			if err != nil {
				t.Fatalf("bulk importing ids: %v", err)
			}
			test.rowIDs = append(test.rowIDs, test.rowIDs2...)
			test.colIDs = append(test.colIDs, test.colIDs2...)
			expPairs = calcTop(test.rowIDs, test.colIDs)
			pairs, err = f.top(tx, topOptions{})
			if err != nil {
				t.Fatalf("executing top after bulk import: %v", err)
			}
			if !reflect.DeepEqual(expPairs, pairs) {
				t.Fatalf("post bulk import2:\n  exp: %v\n  got: %v\n", expPairs, pairs)
			}

			buf := &bytes.Buffer{}
			bm := roaring.NewBitmap(test.roaring...)
			_, err = bm.WriteTo(buf)
			if err != nil {
				t.Fatalf("writing to buffer: %v", err)
			}
			err = f.importRoaringT(tx, buf.Bytes(), false)
			if err != nil {
				t.Fatalf("importing roaring: %v", err)
			}
			rows, cols := toRowsCols(test.roaring)
			expPairs = calcTop(append(test.rowIDs, rows...), append(test.colIDs, cols...))
			pairs, err = f.top(tx, topOptions{})
			if err != nil {
				t.Fatalf("executing top after roaring import: %v", err)
			}
			if !reflect.DeepEqual(expPairs, pairs) {
				t.Fatalf("post Roaring:\n  exp: %v\n  got: %v\n", expPairs, pairs)
			}
		})
	}
}

func toRowsCols(roaring []uint64) (rowIDs, colIDs []uint64) {
	rowIDs, colIDs = make([]uint64, len(roaring)), make([]uint64, len(roaring))
	for i, bit := range roaring {
		rowIDs[i] = bit / ShardWidth
		colIDs[i] = bit % ShardWidth
	}
	return rowIDs, colIDs
}

func calcTop(rowIDs, colIDs []uint64) []Pair {
	if len(rowIDs) != len(colIDs) {
		panic("row and col ids must be of equal len")
	}
	// make map of rowID to colID set in order to dedup
	counts := make(map[uint64]map[uint64]struct{})
	for i := 0; i < len(rowIDs); i++ {
		row, col := rowIDs[i], colIDs[i]
		m, ok := counts[row]
		if !ok {
			m = make(map[uint64]struct{})
			counts[row] = m
		}
		m[col] = struct{}{}
	}

	// build slice of pairs from map
	ret := make([]Pair, 0)
	for row, cols := range counts {
		ret = append(ret, Pair{ID: row, Count: uint64(len(cols))})
	}

	// reverse sort by count
	sort.Slice(ret, func(i, j int) bool { return ret[i].Count > ret[j].Count })
	return ret
}

// calcExpected takes a number of slices of uint64 represented data to be added
// to a fragment. It calculates which rows and bits set in each row would be
// expected after importing that data, and returns a [][]uint64 where the index
// into the first slice is row id.
func calcExpected(inputs ...[]uint64) [][]uint64 {
	// create map of row id to set of column values in that row.
	rows := make(map[uint64]map[uint64]struct{})
	var maxrow uint64
	for _, input := range inputs {
		for _, val := range input {
			row := val / ShardWidth
			if row > maxrow {
				maxrow = row
			}
			m, ok := rows[row]
			if !ok {
				m = make(map[uint64]struct{})
				rows[row] = m
			}
			m[val%ShardWidth] = struct{}{}
		}
	}

	// initialize ret slice
	ret := make([][]uint64, maxrow+1)
	for i := range ret {
		ret[i] = make([]uint64, 0)
	}

	// populate ret slices from rows map
	for row, vals := range rows {
		for val := range vals {
			ret[row] = append(ret[row], val)
		}
	}

	// sort ret slices
	for _, slice := range ret {
		sort.Slice(slice, func(i int, j int) bool { return slice[i] < slice[j] })
	}

	return ret
}

func TestFragmentRowIterator(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", "v", 0, CacheTypeRanked)
		_ = idx
		tx := f.txTestingOnly
		defer tx.Rollback()
		defer f.Clean(t)

		f.mustSetBits(tx, 0, 0)
		f.mustSetBits(tx, 1, 0)
		f.mustSetBits(tx, 2, 0)
		f.mustSetBits(tx, 3, 0)

		iter, err := f.rowIterator(tx, false)
		if err != nil {
			t.Fatal(err)
		}
		for i := uint64(0); i < 4; i++ {
			row, id, _, wrapped, err := iter.Next()
			if err != nil {
				t.Fatal(err)
			}
			if id != i {
				t.Fatalf("expected row %d but got %d", i, id)
			}
			if wrapped {
				t.Fatalf("shouldn't have wrapped")
			}
			if !reflect.DeepEqual(row.Columns(), []uint64{0}) {
				t.Fatalf("got wrong columns back on iteration %d - should just be 0 but %v", i, row.Columns())
			}
		}
		row, id, _, wrapped, err := iter.Next()
		if err != nil {
			t.Fatal(err)
		}
		if row != nil {
			t.Fatalf("row should be nil after iterator is exhausted, got %v", row.Columns())
		}
		if id != 0 {
			t.Fatalf("id should be 0 after iterator is exhausted, got %d", id)
		}
		if !wrapped {
			t.Fatalf("wrapped should be true after iterator is exhausted")
		}
	})

	t.Run("skipped rows", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", "v", 0, CacheTypeRanked)
		_ = idx
		defer f.Clean(t)
		tx := f.txTestingOnly
		defer tx.Rollback()

		f.mustSetBits(tx, 1, 0)
		f.mustSetBits(tx, 3, 0)
		f.mustSetBits(tx, 5, 0)
		f.mustSetBits(tx, 7, 0)

		iter, err := f.rowIterator(tx, false)
		if err != nil {
			t.Fatal(err)
		}
		for i := uint64(1); i < 8; i += 2 {
			row, id, _, wrapped, err := iter.Next()
			if err != nil {
				t.Fatal(err)
			}
			if id != i {
				t.Fatalf("expected row %d but got %d", i, id)
			}
			if wrapped {
				t.Fatalf("shouldn't have wrapped")
			}
			if !reflect.DeepEqual(row.Columns(), []uint64{0}) {
				t.Fatalf("got wrong columns back on iteration %d - should just be 0 but %v", i, row.Columns())
			}
		}
		row, id, _, wrapped, err := iter.Next()
		if err != nil {
			t.Fatal(err)
		}
		if row != nil {
			t.Fatalf("row should be nil after iterator is exhausted, got %v", row.Columns())
		}
		if id != 0 {
			t.Fatalf("id should be 0 after iterator is exhausted, got %d", id)
		}
		if !wrapped {
			t.Fatalf("wrapped should be true after iterator is exhausted")
		}
	})

	t.Run("basic wrapped", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", "v", 0, CacheTypeRanked)
		_ = idx
		defer f.Clean(t)
		tx := f.txTestingOnly
		defer tx.Rollback()

		f.mustSetBits(tx, 0, 0)
		f.mustSetBits(tx, 1, 0)
		f.mustSetBits(tx, 2, 0)
		f.mustSetBits(tx, 3, 0)

		iter, err := f.rowIterator(tx, true)
		if err != nil {
			t.Fatal(err)
		}
		for i := uint64(0); i < 5; i++ {
			row, id, _, wrapped, err := iter.Next()
			if err != nil {
				t.Fatal(err)
			}
			if id != i%4 {
				t.Fatalf("expected row %d but got %d", i%4, id)
			}
			if wrapped && i < 4 {
				t.Fatalf("shouldn't have wrapped")
			} else if !wrapped && i >= 4 {
				t.Fatalf("should have wrapped")
			}
			if !reflect.DeepEqual(row.Columns(), []uint64{0}) {
				t.Fatalf("got wrong columns back on iteration %d - should just be 0 but %v", i, row.Columns())
			}
		}
	})

	t.Run("skipped rows wrapped", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", "v", 0, CacheTypeRanked)
		_ = idx
		defer f.Clean(t)
		tx := f.txTestingOnly
		defer tx.Rollback()

		f.mustSetBits(tx, 1, 0)
		f.mustSetBits(tx, 3, 0)
		f.mustSetBits(tx, 5, 0)
		f.mustSetBits(tx, 7, 0)

		iter, err := f.rowIterator(tx, true)
		if err != nil {
			t.Fatal(err)
		}
		for i := uint64(1); i < 10; i += 2 {
			row, id, _, wrapped, err := iter.Next()
			if err != nil {
				t.Fatal(err)
			}
			if id != i%8 {
				t.Errorf("expected row %d but got %d", i%8, id)
			}
			if wrapped && i < 8 {
				t.Errorf("shouldn't have wrapped")
			} else if !wrapped && i >= 8 {
				t.Errorf("should have wrapped")
			}
			if !reflect.DeepEqual(row.Columns(), []uint64{0}) {
				t.Fatalf("got wrong columns back on iteration %d - should just be 0 but %v", i, row.Columns())
			}
		}
	})
}

// same, with commits
func TestFragmentRowIterator_WithTxCommit(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", "v", 0, CacheTypeRanked)
		_ = idx
		tx := f.txTestingOnly
		defer tx.Rollback()
		defer f.Clean(t)

		f.mustSetBits(tx, 0, 0)
		f.mustSetBits(tx, 1, 0)
		f.mustSetBits(tx, 2, 0)
		f.mustSetBits(tx, 3, 0)

		panicOn(tx.Commit())
		tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
		defer tx.Rollback()

		iter, err := f.rowIterator(tx, false)
		if err != nil {
			t.Fatal(err)
		}
		for i := uint64(0); i < 4; i++ {
			row, id, _, wrapped, err := iter.Next()
			if err != nil {
				t.Fatal(err)
			}
			if id != i {
				t.Fatalf("expected row %d but got %d", i, id)
			}
			if wrapped {
				t.Fatalf("shouldn't have wrapped")
			}
			if !reflect.DeepEqual(row.Columns(), []uint64{0}) {
				t.Fatalf("got wrong columns back on iteration %d - should just be 0 but %v", i, row.Columns())
			}
		}
		row, id, _, wrapped, err := iter.Next()
		if err != nil {
			t.Fatal(err)
		}
		if row != nil {
			t.Fatalf("row should be nil after iterator is exhausted, got %v", row.Columns())
		}
		if id != 0 {
			t.Fatalf("id should be 0 after iterator is exhausted, got %d", id)
		}
		if !wrapped {
			t.Fatalf("wrapped should be true after iterator is exhausted")
		}
	})

	t.Run("skipped rows", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", "v", 0, CacheTypeRanked)
		_ = idx
		defer f.Clean(t)
		tx := f.txTestingOnly
		defer tx.Rollback()

		f.mustSetBits(tx, 1, 0)
		f.mustSetBits(tx, 3, 0)
		f.mustSetBits(tx, 5, 0)
		f.mustSetBits(tx, 7, 0)

		panicOn(tx.Commit())
		tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
		defer tx.Rollback()

		iter, err := f.rowIterator(tx, false)
		if err != nil {
			t.Fatal(err)
		}
		for i := uint64(1); i < 8; i += 2 {
			row, id, _, wrapped, err := iter.Next()
			if err != nil {
				t.Fatal(err)
			}
			if id != i {
				t.Fatalf("expected row %d but got %d", i, id)
			}
			if wrapped {
				t.Fatalf("shouldn't have wrapped")
			}
			if !reflect.DeepEqual(row.Columns(), []uint64{0}) {
				t.Fatalf("got wrong columns back on iteration %d - should just be 0 but %v", i, row.Columns())
			}
		}
		row, id, _, wrapped, err := iter.Next()
		if err != nil {
			t.Fatal(err)
		}
		if row != nil {
			t.Fatalf("row should be nil after iterator is exhausted, got %v", row.Columns())
		}
		if id != 0 {
			t.Fatalf("id should be 0 after iterator is exhausted, got %d", id)
		}
		if !wrapped {
			t.Fatalf("wrapped should be true after iterator is exhausted")
		}
	})

	t.Run("basic wrapped", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", "v", 0, CacheTypeRanked)
		_ = idx
		defer f.Clean(t)
		tx := f.txTestingOnly
		defer tx.Rollback()

		f.mustSetBits(tx, 0, 0)
		f.mustSetBits(tx, 1, 0)
		f.mustSetBits(tx, 2, 0)
		f.mustSetBits(tx, 3, 0)

		panicOn(tx.Commit())
		tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
		defer tx.Rollback()

		iter, err := f.rowIterator(tx, true)
		if err != nil {
			t.Fatal(err)
		}
		for i := uint64(0); i < 5; i++ {
			row, id, _, wrapped, err := iter.Next()
			if err != nil {
				t.Fatal(err)
			}
			if id != i%4 {
				t.Fatalf("expected row %d but got %d", i%4, id)
			}
			if wrapped && i < 4 {
				t.Fatalf("shouldn't have wrapped")
			} else if !wrapped && i >= 4 {
				t.Fatalf("should have wrapped")
			}
			if !reflect.DeepEqual(row.Columns(), []uint64{0}) {
				t.Fatalf("got wrong columns back on iteration %d - should just be 0 but %v", i, row.Columns())
			}
		}
	})

	t.Run("skipped rows wrapped", func(t *testing.T) {
		f, idx := mustOpenFragment("i", "f", "v", 0, CacheTypeRanked)
		_ = idx
		defer f.Clean(t)
		tx := f.txTestingOnly
		defer tx.Rollback()

		f.mustSetBits(tx, 1, 0)
		f.mustSetBits(tx, 3, 0)
		f.mustSetBits(tx, 5, 0)
		f.mustSetBits(tx, 7, 0)

		panicOn(tx.Commit())
		tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
		defer tx.Rollback()

		iter, err := f.rowIterator(tx, true)
		if err != nil {
			t.Fatal(err)
		}
		for i := uint64(1); i < 10; i += 2 {
			row, id, _, wrapped, err := iter.Next()
			if err != nil {
				t.Fatal(err)
			} else if row == nil {
				t.Fatal("expected row")
			}
			if id != i%8 {
				t.Errorf("expected row %d but got %d", i%8, id)
			}
			if wrapped && i < 8 {
				t.Errorf("shouldn't have wrapped")
			} else if !wrapped && i >= 8 {
				t.Errorf("should have wrapped")
			}
			if !reflect.DeepEqual(row.Columns(), []uint64{0}) {
				t.Fatalf("got wrong columns back on iteration %d - should just be 0 but %v", i, row.Columns())
			}
		}
	})
}

func TestUnionInPlaceMapped(t *testing.T) {
	roaringOnlyTest(t)

	f, idx := mustOpenFragment("i", "f", "v", 0, CacheTypeNone)
	// note: clean has to be deferred first, because it has to run with
	// the lock *not* held, because it is sometimes so it has to grab the
	// lock...
	_ = idx
	defer f.Clean(t)

	f.mu.Lock()
	defer f.mu.Unlock()
	r0 := rand.New(rand.NewSource(2))
	r1 := rand.New(rand.NewSource(1))
	data0 := randPositions(1000000, r0)
	setBM0 := roaring.NewBitmap()
	setBM0.OpWriter = nil
	_, err := setBM0.Add(data0...)
	if err != nil {
		t.Fatalf("adding bits: %v", err)
	}
	count0 := setBM0.Count()

	data1 := randPositions(1000000, r1)
	setBM1 := roaring.NewBitmap()
	setBM1.OpWriter = nil
	_, err = setBM1.Add(data1...)
	if err != nil {
		t.Fatalf("adding bits: %v", err)
	}
	count1 := setBM1.Count()

	// now we write setBM0 into f.storage.
	_, err = unprotectedWriteToFragment(f, setBM0)
	if err != nil {
		t.Fatalf("trying to flush fragment to disk: %v", err)
	}
	countF := f.storage.Count()

	f.storage.UnionInPlace(setBM1)
	countUnion := f.storage.Count()

	// UnionInPlace produces no ops log, we have to make it snapshot, to
	// ensure that the on-disk representation is correct. Note, UIP is
	// not used for things that are modifying real fragments, usually;
	// it's used only in computation of things that usually don't go to
	// disk, which is why we handle this specially in testing and not
	// generically.
	err = defaultSnapshotQueue.Immediate(f)
	if err != nil {
		t.Fatalf("snapshot after union-in-place: %v", err)
	}

	if count0 != countF {
		t.Fatalf("writing bitmap to storage changed count: %d => %d", count0, countF)
	}
	min := count0
	if count1 > min {
		min = count1
	}
	max := count0 + count1
	// We don't know how many bits we should have, because of overlap,
	// but it should be between the size of the largest bitmap and the
	// sum of the bitmaps.
	if countUnion < min || countUnion > max {
		t.Fatalf("union of sets with cardinality %d and %d should be between %d and %d, got %d",
			count0, count1, min, max, countUnion)
	}
}

func randPositions(n int, r *rand.Rand) []uint64 {
	ret := make([]uint64, n)
	for i := 0; i < n; i++ {
		ret[i] = uint64(r.Int63n(ShardWidth))
	}
	return ret
}

func TestFragmentPositionsForValue(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", "v", 0, CacheTypeNone)
	_ = idx
	defer f.Clean(t)

	tests := []struct {
		columnID uint64
		bitDepth uint
		value    int64
		clear    bool
		toSet    []uint64
		toClear  []uint64
	}{
		{
			columnID: 0,
			bitDepth: 1,
			value:    0,
			toSet:    []uint64{0},                          // exists bit only
			toClear:  []uint64{ShardWidth, ShardWidth * 2}, // sign bit & 1-position
		},
		{
			columnID: 0,
			bitDepth: 3,
			value:    0,
			toSet:    []uint64{0},                                                              // exists bit only
			toClear:  []uint64{ShardWidth * 1, ShardWidth * 2, ShardWidth * 3, ShardWidth * 4}, // sign bit, 1, 2, 4
		},
		{
			columnID: 1,
			bitDepth: 3,
			value:    0,
			toSet:    []uint64{1},                                                                    // exists bit only
			toClear:  []uint64{ShardWidth + 1, ShardWidth*2 + 1, ShardWidth*3 + 1, ShardWidth*4 + 1}, // sign bit, 1, 2, 4
		},
		{
			columnID: 0,
			bitDepth: 1,
			value:    1,
			toSet:    []uint64{0, ShardWidth * 2}, // exists bit, 1
			toClear:  []uint64{ShardWidth},        // sign bit only
		},
		{
			columnID: 0,
			bitDepth: 4,
			value:    10,
			toSet:    []uint64{0, ShardWidth * 3, ShardWidth * 5},              // exists bit, 2, 8
			toClear:  []uint64{ShardWidth * 1, ShardWidth * 2, ShardWidth * 4}, // sign bit, 1, 4
		},
		{
			columnID: 0,
			bitDepth: 5,
			value:    10,
			toSet:    []uint64{0, ShardWidth * 3, ShardWidth * 5},                              // exists bit, 2, 8
			toClear:  []uint64{ShardWidth * 1, ShardWidth * 2, ShardWidth * 4, ShardWidth * 6}, // sign bit, 1, 4, 16
		},
	}

	for i, test := range tests {
		toSet, toClear := make([]uint64, 0), make([]uint64, 0)
		var err error
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			toSet, toClear, err = f.positionsForValue(test.columnID, test.bitDepth, test.value, test.clear, toSet, toClear)
			if err != nil {
				t.Fatalf("getting positions: %v", err)
			}

			if len(toSet) != len(test.toSet) || len(toClear) != len(test.toClear) {
				t.Fatalf("differing lengths (exp/got): set\n%v\n%v\nclear\n%v\n%v\n", test.toSet, toSet, test.toClear, toClear)
			}
			for i := 0; i < len(toSet); i++ {
				if toSet[i] != test.toSet[i] {
					t.Errorf("toSet don't match at %d - exp:%d and got:%d", i, test.toSet[i], toSet[i])
				}
			}
			for i := 0; i < len(toClear); i++ {
				if toClear[i] != test.toClear[i] {
					t.Errorf("toClear don't match at %d - exp:%d and got:%d", i, test.toClear[i], toClear[i])
				}
			}
		})
	}
}

func TestIntLTRegression(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", "v", 0, CacheTypeNone)
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	_, err := f.setValue(tx, 1, 6, 33)
	if err != nil {
		t.Fatalf("setting value: %v", err)
	}

	row, err := f.rangeOp(tx, pql.LT, 6, 33)
	if err != nil {
		t.Fatalf("doing range of: %v", err)
	}

	if !row.IsEmpty() {
		t.Errorf("expected nothing, but got: %v", row.Columns())
	}
}

func sliceEq(x, y []uint64) bool {
	if len(x) == 0 {
		x = nil
	}
	if len(y) == 0 {
		y = nil
	}
	return reflect.DeepEqual(x, y)
}

func TestFragmentBSIUnsigned(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", "v", 0, CacheTypeNone)
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Number of bits to test.
	const k = 6

	// Load all numbers into an effectively diagonal matrix.
	for i := 0; i < 1<<k; i++ {
		ok, err := f.setValue(tx, uint64(i), k, int64(i))
		if err != nil {
			t.Fatalf("failed to set col %d to %d: %v", uint64(i), int64(i), err)
		} else if !ok {
			t.Fatalf("no change when setting col %d to %d", uint64(i), int64(i))
		}
	}

	// Generate a list of columns.
	cols := make([]uint64, 1<<k)
	for i := range cols {
		cols[i] = uint64(i)
	}

	// Minimum and maximum checking bounds.
	// This is mostly arbitrary.
	minCheck, maxCheck := -3, 1<<(k+1)

	t.Run("<", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			row, err := f.rangeLT(tx, k, int64(i), false)
			if err != nil {
				t.Fatalf("failed to query fragment: %v", err)
			}
			var expect []uint64
			switch {
			case i < 0:
			case i < len(cols):
				expect = cols[:i]
			default:
				expect = cols
			}
			got := row.Columns()
			if !sliceEq(expect, got) {
				t.Errorf("expected %v but got %v for x < %d", expect, got, i)
			}
		}
	})
	t.Run("<=", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			row, err := f.rangeLT(tx, k, int64(i), true)
			if err != nil {
				t.Fatalf("failed to query fragment: %v", err)
			}
			var expect []uint64
			switch {
			case i < 0:
			case i < len(cols)-1:
				expect = cols[:i+1]
			default:
				expect = cols
			}
			got := row.Columns()
			if !sliceEq(expect, got) {
				t.Errorf("expected %v but got %v for x <= %d", expect, got, i)
			}
		}
	})
	t.Run(">", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			row, err := f.rangeGT(tx, k, int64(i), false)
			if err != nil {
				t.Fatalf("failed to query fragment: %v", err)
			}
			var expect []uint64
			switch {
			case i < 0:
				expect = cols
			case i < len(cols)-1:
				expect = cols[i+1:]
			default:
			}
			got := row.Columns()
			if !sliceEq(expect, got) {
				t.Errorf("expected %v but got %v for x > %d", expect, got, i)
			}
		}
	})
	t.Run(">=", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			row, err := f.rangeGT(tx, k, int64(i), true)
			if err != nil {
				t.Fatalf("failed to query fragment: %v", err)
			}
			var expect []uint64
			switch {
			case i < 0:
				expect = cols
			case i < len(cols):
				expect = cols[i:]
			default:
			}
			got := row.Columns()
			if !sliceEq(expect, got) {
				t.Errorf("expected %v but got %v for x >= %d", expect, got, i)
			}
		}
	})
	t.Run("Range", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			for j := i; j < maxCheck; j++ {
				row, err := f.rangeBetween(tx, k, int64(i), int64(j))
				if err != nil {
					t.Fatalf("failed to query fragment: %v", err)
				}
				var lower, upper int
				switch {
				case i < 0:
					lower = 0
				case i > len(cols):
					lower = len(cols)
				default:
					lower = i
				}
				switch {
				case j < 0:
					upper = 0
				case j >= len(cols):
					upper = len(cols)
				default:
					upper = j + 1
				}
				expect := cols[lower:upper]
				got := row.Columns()
				if !sliceEq(expect, got) {
					t.Errorf("expected %v but got %v for %d <= x <= %d", expect, got, i, j)
				}
			}
		}
	})
	t.Run("==", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			row, err := f.rangeEQ(tx, k, int64(i))
			if err != nil {
				t.Fatalf("failed to query fragment: %v", err)
			}
			var expect []uint64
			if i >= 0 && i < len(cols) {
				expect = cols[i : i+1]
			}
			got := row.Columns()
			if !sliceEq(expect, got) {
				t.Errorf("expected %v but got %v for x == %d", expect, got, i)
			}
		}
	})
}

// same, WithTxCommit version
func TestFragmentBSIUnsigned_WithTxCommit(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", "v", 0, CacheTypeNone)
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Number of bits to test.
	const k = 6

	// Load all numbers into an effectively diagonal matrix.
	for i := 0; i < 1<<k; i++ {
		ok, err := f.setValue(tx, uint64(i), k, int64(i))
		if err != nil {
			t.Fatalf("failed to set col %d to %d: %v", uint64(i), int64(i), err)
		} else if !ok {
			t.Fatalf("no change when setting col %d to %d", uint64(i), int64(i))
		}
	}

	panicOn(tx.Commit())
	tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
	defer tx.Rollback()

	// Generate a list of columns.
	cols := make([]uint64, 1<<k)
	for i := range cols {
		cols[i] = uint64(i)
	}

	// Minimum and maximum checking bounds.
	// This is mostly arbitrary.
	minCheck, maxCheck := -3, 1<<(k+1)

	t.Run("<", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			row, err := f.rangeLT(tx, k, int64(i), false)
			if err != nil {
				t.Fatalf("failed to query fragment: %v", err)
			}
			var expect []uint64
			switch {
			case i < 0:
			case i < len(cols):
				expect = cols[:i]
			default:
				expect = cols
			}
			got := row.Columns()
			if !sliceEq(expect, got) {
				t.Errorf("expected %v but got %v for x < %d", expect, got, i)
			}
		}
	})
	t.Run("<=", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			row, err := f.rangeLT(tx, k, int64(i), true)
			if err != nil {
				t.Fatalf("failed to query fragment: %v", err)
			}
			var expect []uint64
			switch {
			case i < 0:
			case i < len(cols)-1:
				expect = cols[:i+1]
			default:
				expect = cols
			}
			got := row.Columns()
			if !sliceEq(expect, got) {
				t.Errorf("expected %v but got %v for x <= %d", expect, got, i)
			}
		}
	})
	t.Run(">", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			row, err := f.rangeGT(tx, k, int64(i), false)
			if err != nil {
				t.Fatalf("failed to query fragment: %v", err)
			}
			var expect []uint64
			switch {
			case i < 0:
				expect = cols
			case i < len(cols)-1:
				expect = cols[i+1:]
			default:
			}
			got := row.Columns()
			if !sliceEq(expect, got) {
				t.Errorf("expected %v but got %v for x > %d", expect, got, i)
			}
		}
	})
	t.Run(">=", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			row, err := f.rangeGT(tx, k, int64(i), true)
			if err != nil {
				t.Fatalf("failed to query fragment: %v", err)
			}
			var expect []uint64
			switch {
			case i < 0:
				expect = cols
			case i < len(cols):
				expect = cols[i:]
			default:
			}
			got := row.Columns()
			if !sliceEq(expect, got) {
				t.Errorf("expected %v but got %v for x >= %d", expect, got, i)
			}
		}
	})
	t.Run("Range", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			for j := i; j < maxCheck; j++ {
				row, err := f.rangeBetween(tx, k, int64(i), int64(j))
				if err != nil {
					t.Fatalf("failed to query fragment: %v", err)
				}
				var lower, upper int
				switch {
				case i < 0:
					lower = 0
				case i > len(cols):
					lower = len(cols)
				default:
					lower = i
				}
				switch {
				case j < 0:
					upper = 0
				case j >= len(cols):
					upper = len(cols)
				default:
					upper = j + 1
				}
				expect := cols[lower:upper]
				got := row.Columns()
				if !sliceEq(expect, got) {
					t.Errorf("expected %v but got %v for %d <= x <= %d", expect, got, i, j)
				}
			}
		}
	})
	t.Run("==", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			row, err := f.rangeEQ(tx, k, int64(i))
			if err != nil {
				t.Fatalf("failed to query fragment: %v", err)
			}
			var expect []uint64
			if i >= 0 && i < len(cols) {
				expect = cols[i : i+1]
			}
			got := row.Columns()
			if !sliceEq(expect, got) {
				t.Errorf("expected %v but got %v for x == %d", expect, got, i)
			}
		}
	})
}

func TestFragmentBSISigned(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", "v", 0, CacheTypeNone)
	_ = idx
	defer f.Clean(t)

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// Number of bits to test.
	const k = 6

	// Load all numbers into an effectively diagonal matrix.
	minVal, maxVal := 1-(1<<k), (1<<k)-1
	for i := minVal; i <= maxVal; i++ {
		ok, err := f.setValue(tx, uint64(i-minVal), k, int64(i))
		if err != nil {
			t.Fatalf("failed to set col %d to %d: %v", uint64(i-minVal), int64(i), err)
		} else if !ok {
			t.Fatalf("no change when setting col %d to %d", uint64(i-minVal), int64(i))
		}
	}

	// Generate a list of columns.
	cols := make([]uint64, (maxVal-minVal)+1)
	for i := range cols {
		cols[i] = uint64(i)
	}

	// Minimum and maximum checking bounds.
	// This is mostly arbitrary.
	minCheck, maxCheck := 2*minVal, 2*maxVal

	t.Run("<", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			row, err := f.rangeLT(tx, k, int64(i), false)
			if err != nil {
				t.Fatalf("failed to query fragment: %v", err)
			}
			var expect []uint64
			switch {
			case i < minVal:
			case i <= maxVal:
				expect = cols[:i-minVal]
			default:
				expect = cols
			}
			got := row.Columns()
			if !sliceEq(expect, got) {
				t.Errorf("expected %v but got %v for x < %d", expect, got, i)
			}
		}
	})
	t.Run("<=", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			row, err := f.rangeLT(tx, k, int64(i), true)
			if err != nil {
				t.Fatalf("failed to query fragment: %v", err)
			}
			var expect []uint64
			switch {
			case i < minVal:
			case i < maxVal:
				expect = cols[:(i-minVal)+1]
			default:
				expect = cols
			}
			got := row.Columns()
			if !sliceEq(expect, got) {
				t.Errorf("expected %v but got %v for x <= %d", expect, got, i)
			}
		}
	})
	t.Run(">", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			row, err := f.rangeGT(tx, k, int64(i), false)
			if err != nil {
				t.Fatalf("failed to query fragment: %v", err)
			}
			var expect []uint64
			switch {
			case i < minVal:
				expect = cols
			case i < maxVal:
				expect = cols[(i-minVal)+1:]
			default:
			}
			got := row.Columns()
			if !sliceEq(expect, got) {
				t.Errorf("expected %v but got %v for x > %d", expect, got, i)
			}
		}
	})
	t.Run(">=", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			row, err := f.rangeGT(tx, k, int64(i), true)
			if err != nil {
				t.Fatalf("failed to query fragment: %v", err)
			}
			var expect []uint64
			switch {
			case i < minVal:
				expect = cols
			case i <= maxVal:
				expect = cols[i-minVal:]
			default:
			}
			got := row.Columns()
			if !sliceEq(expect, got) {
				t.Errorf("expected %v but got %v for x >= %d", expect, got, i)
			}
		}
	})
	t.Run("Range", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			for j := i; j < maxCheck; j++ {
				row, err := f.rangeBetween(tx, k, int64(i), int64(j))
				if err != nil {
					t.Fatalf("failed to query fragment: %v", err)
				}
				var lower, upper int
				switch {
				case i < minVal:
					lower = 0
				case i > maxVal:
					lower = len(cols)
				default:
					lower = i - minVal
				}
				switch {
				case j < minVal:
					upper = 0
				case j > maxVal:
					upper = len(cols)
				default:
					upper = (j - minVal) + 1
				}
				expect := cols[lower:upper]
				got := row.Columns()
				if !sliceEq(expect, got) {
					t.Errorf("expected %v but got %v for %d <= x <= %d", expect, got, i, j)
				}
			}
		}
	})
	t.Run("==", func(t *testing.T) {
		for i := minCheck; i < maxCheck; i++ {
			row, err := f.rangeEQ(tx, k, int64(i))
			if err != nil {
				t.Fatalf("failed to query fragment: %v", err)
			}
			var expect []uint64
			if i >= minVal && i <= maxVal {
				expect = cols[i-minVal : (i-minVal)+1]
			}
			got := row.Columns()
			if !sliceEq(expect, got) {
				t.Errorf("expected %v but got %v for x == %d", expect, got, i)
			}
		}
	})
}

func TestImportClearRestart(t *testing.T) {
	roaringOnlyTest(t)

	tests := []struct {
		rows []uint64
		cols []uint64
	}{
		{
			rows: []uint64{1},
			cols: []uint64{1},
		},
		{
			rows: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 1},
			cols: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 500000},
		},
		{
			rows: []uint64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			cols: []uint64{0, 65535, 65536, 131071, 131072, 196607, 196608, 262143, 262144, 1000000},
		},
		{
			rows: []uint64{1, 2, 20, 200, 2000, 200000},
			cols: []uint64{1, 1, 1, 1, 1, 1},
		},
	}
	for i, test := range tests {
		for _, maxOpN := range []int{0, 10000} {
			t.Run(fmt.Sprintf("%dMaxOpN%d", i, maxOpN), func(t *testing.T) {
				testrows, testcols := make([]uint64, len(test.rows)), make([]uint64, len(test.rows))
				copy(testrows, test.rows)
				copy(testcols, test.cols)
				exp := make(map[uint64]map[uint64]struct{}) // row num to cols
				if len(testrows) != len(testcols) {
					t.Fatalf("bad test spec-need same number of rows/cols, %d/%d", len(testrows), len(testcols))
				}
				// set up expected data
				expOpN := 0
				for i := range testrows {
					row, col := testrows[i], testcols[i]
					cols, ok := exp[row]
					if !ok {
						exp[row] = make(map[uint64]struct{})
						cols = exp[row]
					}
					if _, ok = cols[col]; !ok {
						expOpN++
						cols[col] = struct{}{}
					}
				}

				f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
				_ = idx
				f.MaxOpN = maxOpN

				// Obtain transaction.
				tx := f.txTestingOnly
				defer tx.Rollback()

				err := f.bulkImport(tx, testrows, testcols, &ImportOptions{})
				if err != nil {
					t.Fatalf("initial small import: %v", err)
				}
				if idx.Txf.TxType() == RoaringTxn {
					if expOpN <= maxOpN && f.opN != expOpN {
						t.Errorf("unexpected opN - %d is not %d", f.opN, expOpN)
					}
				}
				check(t, tx, f, exp)

				err = f.Close()
				if err != nil {
					t.Fatalf("closing fragment: %v", err)
				}
				panicOn(tx.Commit())

				err = f.Open()
				tx = idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
				defer tx.Rollback()
				if err != nil {
					t.Fatalf("reopening fragment: %v", err)
				}

				if idx.Txf.TxType() == RoaringTxn {
					if expOpN <= maxOpN && f.opN != expOpN {
						t.Errorf("unexpected opN after close/open %d is not %d", f.opN, expOpN)
					}
				}

				check(t, tx, f, exp)

				h := NewHolder(DefaultPartitionN)
				h.Path = filepath.Dir(f.path)
				idx2, err := h.CreateIndex("i", IndexOptions{})
				_ = idx2
				panicOn(err)

				// OVERWRITING the f.path with a new fragment
				f2 := newFragment(h, f.path, "i", "f", viewStandard, 0, 0)

				//				f2, idx2 := mustOpenFragment("i", "f", viewStandard, 0, "")
				//				_ = idx2

				f2.MaxOpN = maxOpN
				f2.CacheType = f.CacheType

				panicOn(tx.Commit()) // match the f.closeStorage which overlaps the f2 creation.

				tx2 := idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f2})
				defer tx2.Rollback()

				err = f.closeStorage()
				if err != nil {
					t.Fatalf("closing storage: %v", err)
				}

				err = f2.Open()
				if err != nil {
					t.Fatalf("opening new fragment: %v", err)
				}

				if idx.Txf.TxType() == RoaringTxn {
					if expOpN <= maxOpN && f2.opN != expOpN {
						t.Errorf("unexpected opN after close/open %d is not %d", f2.opN, expOpN)
					}
				}

				check(t, tx2, f2, exp)

				copy(testrows, test.rows)
				copy(testcols, test.cols)
				err = f2.bulkImport(tx2, testrows, testcols, &ImportOptions{Clear: true})
				if err != nil {
					t.Fatalf("clearing imported data: %v", err)
				}

				// clear exp, but leave rows in so we re-query them in `check`
				for row := range exp {
					exp[row] = nil
				}

				check(t, tx2, f2, exp)

				panicOn(tx2.Commit())

				h3 := NewHolder(DefaultPartitionN)
				h3.Path = filepath.Dir(f2.path)
				idx3, err := h3.CreateIndex("i", IndexOptions{})
				_ = idx3
				panicOn(err)

				f3 := newFragment(h3, f2.path, "i", "f", viewStandard, 0, 0)
				f3.MaxOpN = maxOpN
				f3.CacheType = f.CacheType

				tx3 := idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f3})
				defer tx3.Rollback()

				err = f2.closeStorage()
				if err != nil {
					t.Fatalf("f2 closing storage: %v", err)
				}

				err = f3.Open()
				if err != nil {
					t.Fatalf("opening f3: %v", err)
				}
				defer f3.Clean(t)

				check(t, tx3, f3, exp)

			})

		}
	}
}

func check(t *testing.T, tx Tx, f *fragment, exp map[uint64]map[uint64]struct{}) {

	for rowID, colsExp := range exp {
		colsAct := f.mustRow(tx, rowID).Columns()
		if len(colsAct) != len(colsExp) {
			t.Errorf("row %d len mismatch got: %d exp:%d", rowID, len(colsAct), len(colsExp))
		}
		for _, colAct := range colsAct {
			if _, ok := colsExp[colAct]; !ok {
				t.Errorf("extra column: %d", colAct)
			}
		}
		for colExp := range colsExp {
			found := false
			for _, colAct := range colsAct {
				if colExp == colAct {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %d, but not found", colExp)
			}
		}
	}

}

func TestImportValueConcurrent(t *testing.T) {
	f, idx := mustOpenBSIFragment("i", "f", viewBSIGroupPrefix+"foo", 0)

	// produces false positives under blue_green because of races
	// between commits, and is probematic under single writer
	// backends like rbf and lmdb. Marking as roaring-only.
	roaringOnlyTest(t)

	// Since eg.Go gets called multiple times below, each
	// time needs its own Tx. So close the default one and
	// make a new one each time.
	tx := f.txTestingOnly
	tx.Rollback()

	defer f.Clean(t)
	eg := &errgroup.Group{}
	for i := 0; i < 4; i++ {
		i := i
		eg.Go(func() error {
			tx := idx.Txf.NewTx(Txo{Write: writable, Index: idx, Fragment: f})
			defer tx.Rollback()
			for j := uint64(0); j < 10; j++ {
				err := f.importValue(tx, []uint64{j}, []int64{int64(rand.Int63n(1000))}, 10, i%2 == 0)
				if err != nil {
					return err
				}
			}
			return nil
		})
	}
	err := eg.Wait()
	if err != nil {
		t.Fatalf("concurrently importing values: %v", err)
	}
}

func TestImportMultipleValues(t *testing.T) {
	tests := []struct {
		cols      []uint64
		vals      []int64
		checkCols []uint64
		checkVals []int64
		depth     uint
	}{
		{
			cols:      []uint64{0, 0},
			vals:      []int64{97, 100},
			depth:     7,
			checkCols: []uint64{0},
			checkVals: []int64{100},
		},
	}

	for i, test := range tests {
		for _, maxOpN := range []int{0, 10000} { // test small/large write
			t.Run(fmt.Sprintf("%dLowOpN", i), func(t *testing.T) {
				f, idx := mustOpenBSIFragment("i", "f", viewBSIGroupPrefix+"foo", 0)
				_ = idx
				f.MaxOpN = maxOpN
				defer f.Clean(t)

				// Obtain transaction.
				tx := f.txTestingOnly
				defer tx.Rollback()

				err := f.importValue(tx, test.cols, test.vals, test.depth, false)
				if err != nil {
					t.Fatalf("importing values: %v", err)
				}

				// probably too slow, would hit disk alot:
				//panicOn(tx.Commit())
				//tx = idx.Txf.NewTx(Txo{Write: !writable, Index: idx, Fragment: f})
				//defer tx.Rollback()

				for i := range test.checkCols {
					cc, cv := test.checkCols[i], test.checkVals[i]
					n, exists, err := f.value(tx, cc, test.depth)
					if err != nil {
						t.Fatalf("getting value: %v", err)
					}
					if !exists {
						t.Errorf("column %d should exist", cc)
					}
					if n != cv {
						t.Errorf("wrong value: %d is not %d", n, cv)
					}
				}
			})

		}
	}
}

func TestImportValueRowCache(t *testing.T) {
	type testCase struct {
		cols      []uint64
		vals      []int64
		checkCols []uint64
		depth     uint
	}
	tests := []struct {
		tc1 testCase
		tc2 testCase
	}{
		{
			tc1: testCase{
				cols:      []uint64{2},
				vals:      []int64{1},
				depth:     1,
				checkCols: []uint64{2},
			},
			tc2: testCase{
				cols:      []uint64{1000},
				vals:      []int64{1},
				depth:     1,
				checkCols: []uint64{2, 1000},
			},
		},
	}

	for i, test := range tests {
		for _, maxOpN := range []int{1, 10000} {
			t.Run(fmt.Sprintf("%dMaxOpN%d", i, maxOpN), func(t *testing.T) {
				f, idx := mustOpenBSIFragment("i", "f", viewBSIGroupPrefix+"foo", 0)
				f.MaxOpN = maxOpN
				_ = idx
				defer f.Clean(t)

				// Obtain transaction.
				tx := f.txTestingOnly
				defer tx.Rollback()

				// First import (tc1)
				if err := f.importValue(tx, test.tc1.cols, test.tc1.vals, test.tc1.depth, false); err != nil {
					t.Fatalf("importing values: %v", err)
				}

				if r, err := f.rangeOp(tx, pql.GT, test.tc1.depth, 0); err != nil {
					t.Error("getting range of values")
				} else if !reflect.DeepEqual(r.Columns(), test.tc1.checkCols) {
					t.Errorf("wrong column values. expected: %v, but got: %v", test.tc1.checkCols, r.Columns())
				}

				// Second import (tc2)
				if err := f.importValue(tx, test.tc2.cols, test.tc2.vals, test.tc2.depth, false); err != nil {
					t.Fatalf("importing values: %v", err)
				}

				if r, err := f.rangeOp(tx, pql.GT, test.tc2.depth, 0); err != nil {
					t.Error("getting range of values")
				} else if !reflect.DeepEqual(r.Columns(), test.tc2.checkCols) {
					t.Errorf("wrong column values. expected: %v, but got: %v", test.tc2.checkCols, r.Columns())
				}
			})
		}
	}
}

func TestFragmentConcurrentReadWrite(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, CacheTypeRanked)
	_ = idx
	defer f.Clean(t)

	// Obtain transaction, but don't start another b/c the
	// two goroutines below need the same view.
	tx := f.txTestingOnly
	defer tx.Rollback()

	eg := &errgroup.Group{}
	eg.Go(func() error {
		for i := uint64(0); i < 1000; i++ {
			_, err := f.setBit(tx, i%4, i)
			if err != nil {
				return errors.Wrap(err, "setting bit")
			}
		}
		return nil
	})

	acc := uint64(0)
	for i := uint64(0); i < 100; i++ {
		r := f.mustRow(tx, i%4)
		acc += r.Count()
	}
	if err := eg.Wait(); err != nil {
		t.Errorf("error from setting a bit: %v", err)
	}

	t.Logf("%d", acc)
}

func TestRemapCache(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	_ = idx
	index, field, view, shard := f.index, f.field, f.view, f.shard

	// request a panic that doesn't kill the program on fault
	wouldFault := debug.SetPanicOnFault(true)
	defer func() {
		debug.SetPanicOnFault(wouldFault)
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				// special case: if we caught a page fault, we diagnose that directly. sadly,
				// we can't see the actual values that were used to generate this, probably.
				if err.Error() == "runtime error: invalid memory address or nil pointer dereference" {
					t.Fatalf("segfault trapped during remap test (expected failure mode)")
				}
			}
			t.Fatalf("unexpected panic: %v", r)
		}
	}()

	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// create a container
	_, err := tx.Add(index, field, view, shard, !doBatched, 65537)
	if err != nil {
		t.Fatalf("storage add: %v", err)
	}
	// cause the container to be mapped
	err = f.Snapshot()
	if err != nil {
		t.Fatalf("storage snapshot: %v", err)
	}
	// freeze the row
	_ = f.mustRow(tx, 0)
	// add a bit that isn't in that container, so that container doesn't
	// change
	_, err = tx.Add(index, field, view, shard, !doBatched, 2)
	if err != nil {
		t.Fatalf("storage add: %v", err)
	}
	// make the original container be the most recent, thus cached, container
	_, err = f.bit(tx, 0, 65537)
	if err != nil {
		t.Fatalf("storage bit check: %v", err)
	}
	// force snapshot, remapping the containers
	err = f.Snapshot()
	if err != nil {
		t.Fatalf("storage snapshot: %v", err)
	}
	// get rid of the old mapping
	runtime.GC()
	// try to read that container again
	_, err = f.bit(tx, 0, 65537)
	if err != nil {
		t.Fatalf("storage bit check: %v", err)
	}
}

func TestFragment_Bug_Q2DoubleDelete(t *testing.T) {
	f, idx := mustOpenFragment("i", "f", viewStandard, 0, "")
	_ = idx
	// Obtain transaction.
	tx := f.txTestingOnly
	defer tx.Rollback()

	// byShardWidth is a map of the same roaring (fragment) data generated
	// with different shard widths.
	// TODO: a better approach may be to generate this in the test based
	// on shard width.
	byShardWidth := make(map[uint64][]byte)
	// row/col: 1/1
	byShardWidth[1<<20] = []byte{60, 48, 0, 0, 1, 0, 0, 0, 16, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 24, 0, 0, 0, 1, 0}
	byShardWidth[1<<22] = []byte{60, 48, 0, 0, 1, 0, 0, 0, 64, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 24, 0, 0, 0, 1, 0}

	var b []byte
	if data, ok := byShardWidth[ShardWidth]; ok {
		b = data
	}

	_ = idx
	defer f.Clean(t)

	err := f.importRoaringT(tx, b, false)
	if err != nil {
		t.Fatalf("importing roaring: %v", err)
	}

	//check the bit
	res := f.mustRow(tx, 1).Columns()
	if len(res) < 1 || f.mustRow(tx, 1).Columns()[0] != 1 {
		t.Fatalf("expecting 1 got: %v", res)
	}
	//clear the bit
	changed, _ := f.clearBit(tx, 1, 1)
	if !changed {
		t.Fatalf("expected change got %v", changed)
	}

	//check missing
	res = f.mustRow(tx, 1).Columns()
	if len(res) != 0 {
		t.Fatalf("expected nothing got %v", res)
	}

	// import again
	err = f.importRoaringT(tx, b, false)
	if err != nil {
		t.Fatalf("importing roaring: %v", err)
	}

	//check
	res = f.mustRow(tx, 1).Columns()
	if len(res) < 1 || f.mustRow(tx, 1).Columns()[0] != 1 {
		t.Fatalf("again expecting 1 got: %v", res)
	}

	changed, _ = f.clearBit(tx, 1, 1)
	if !changed {
		t.Fatalf("again expected change got %v", changed) // again expected change got false
	}

	//check missing
	res = f.mustRow(tx, 1).Columns()
	if len(res) != 0 {
		t.Fatalf("expected nothing got %v", res)
	}
}

func notBlueGreenTest(t *testing.T) {
	src := os.Getenv("PILOSA_TXSRC")
	if strings.Contains(src, "_") {
		if strings.Contains(src, "roaring") {
			t.Skip("skip under blue green with roaring")
		}
	}
}
