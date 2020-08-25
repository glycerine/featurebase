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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/pilosa/pilosa/v2/pql"
	"github.com/pilosa/pilosa/v2/testhook"
)

func TestExecutor_TranslateGroupByCall(t *testing.T) {
	holder := NewHolder(DefaultPartitionN)
	defer holder.Close()

	cluster := NewTestCluster(t, 1)

	e := &executor{
		Holder:  holder,
		Cluster: cluster,
	}
	e.Holder.Path, _ = testhook.TempDirInDir(t, *TempDir, "pilosa-executor-")
	err := e.Holder.Open()
	if err != nil {
		t.Fatalf("opening holder: %v", err)
	}

	idx, err := e.Holder.CreateIndex("i", IndexOptions{})
	if err != nil {
		t.Fatalf("creating index: %v", err)
	}

	_, erra := idx.CreateField("ak", OptFieldKeys())
	_, errb := idx.CreateField("b")
	_, errc := idx.CreateField("ck", OptFieldKeys())
	if erra != nil || errb != nil || errc != nil {
		t.Fatalf("creating fields %v, %v, %v", erra, errb, errc)
	}

	query, err := pql.ParseString(`GroupBy(Rows(ak), Rows(b), Rows(ck), previous=["la", 0, "ha"], having=Condition(count > 10))`)
	if err != nil {
		t.Fatalf("parsing query: %v", err)
	}
	c := query.Calls[0]
	err = e.translateCall(context.Background(), "i", c, make(map[string]map[string]uint64))
	if err != nil {
		t.Fatalf("translating call: %v", err)
	}
	if len(c.Args["previous"].([]interface{})) != 3 {
		t.Fatalf("unexpected length for 'previous' arg %v", c.Args["previous"])
	}
	for i, v := range c.Args["previous"].([]interface{}) {
		if !isInt(v) {
			t.Fatalf("expected all items in previous to be ints, but '%v' at index %d is %[1]T", v, i)
		}
	}

	if having, hok := c.Args["having"].(*pql.Call); !hok {
		t.Fatal("expected having to be a call")
	} else if cond, cok := having.Args["count"].(*pql.Condition); !cok {
		t.Fatal("expected condition to be a count")
	} else if cond.Op != pql.GT {
		t.Fatal("expected condition op to be >")
	} else {
		val, ok := cond.Uint64Value()
		if !ok || val != uint64(10) {
			t.Fatal("expected condition val to be uint64(10)")
		}
	}

	errTests := []struct {
		pql string
		err string
	}{
		{
			pql: `GroupBy(Rows(notfound), previous=1)`,
			err: "'previous' argument must be list",
		},
		{
			pql: `GroupBy(Rows(ak), previous=["la", 0])`,
			err: "mismatched lengths",
		},
		{
			pql: `GroupBy(Rows(ak), previous=[1])`,
			err: "prev value must be a string",
		},
		{
			pql: `GroupBy(Rows(notfound), previous=[1])`,
			err: ErrFieldNotFound.Error(),
		},
		// TODO: an unknown key will actually allocate an id. this is probably bad.
		// {
		// 	pql: `GroupBy(Rows(ak), previous=["zoop"])`,
		// 	err: "translating row key '",
		// },
		{
			pql: `GroupBy(Rows(b), previous=["la"])`,
			err: "which doesn't use string keys",
		},
	}

	for i, test := range errTests {
		t.Run(fmt.Sprintf("#%d_%s", i, test.err), func(t *testing.T) {
			query, err := pql.ParseString(test.pql)
			if err != nil {
				t.Fatalf("parsing query: %v", err)
			}
			c := query.Calls[0]
			err = e.translateCall(context.Background(), "i", c, make(map[string]map[string]uint64))
			if err == nil {
				t.Fatalf("expected error, but translated call is '%s", c)
			}
			if !strings.Contains(err.Error(), test.err) {
				t.Fatalf("expected '%s', got '%v'", test.err, err)
			}
		})
	}
}

func TestExecutor_TranslateRowsOnBool(t *testing.T) {
	holder := NewHolder(DefaultPartitionN)
	defer holder.Close()

	e := &executor{
		Holder:  holder,
		Cluster: NewTestCluster(t, 1),
	}
	e.Holder.Path, _ = testhook.TempDirInDir(t, *TempDir, "pilosa-executor-")
	if err := e.Holder.Open(); err != nil {
		t.Fatalf("opening holder: %v", err)
	}

	idx, err := e.Holder.CreateIndex("i", IndexOptions{})
	if err != nil {
		t.Fatalf("creating index: %v", err)
	}

	tx, err := holder.BeginTx(writable, idx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	fb, errb := idx.CreateField("b", OptFieldTypeBool())
	_, errbk := idx.CreateField("bk", OptFieldTypeBool(), OptFieldKeys())
	if errb != nil || errbk != nil {
		t.Fatalf("creating fields %v, %v", errb, errbk)
	}

	_, err1 := fb.SetBit(tx, 1, 1, nil)
	_, err2 := fb.SetBit(tx, 2, 2, nil)
	_, err3 := fb.SetBit(tx, 3, 3, nil)
	if err1 != nil || err2 != nil || err3 != nil {
		t.Fatalf("setting bit %v, %v, %v", err1, err2, err3)
	}

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		pql string
	}{
		{pql: "Rows(b)"},
		{pql: "GroupBy(Rows(b))"},
		{pql: "Set(4, b=true)"},
	}

	for _, test := range tests {
		t.Run(test.pql, func(t *testing.T) {
			query, err := pql.ParseString(test.pql)
			if err != nil {
				t.Fatalf("parsing query: %v", err)
			}

			c := query.Calls[0]
			err = e.translateCall(context.Background(), "i", c, make(map[string]map[string]uint64))
			if err != nil {
				t.Fatalf("translating call: %v", err)
			}
		})
	}
}

func isInt(a interface{}) bool {
	switch a.(type) {
	case int, int64, uint, uint64:
		return true
	default:
		return false
	}
}

func TestFilterWithLimit(t *testing.T) {
	f := filterWithLimit(5)

	for i := uint64(0); i < 5; i++ {
		include, done := f(i, i*(1<<shardVsContainerExponent), nil)
		if done {
			t.Fatalf("limit filter ended early on iteration %d", i)
		}
		if !include {
			t.Fatalf("limit filter should always include until done")
		}
	}
	inc, done := f(5, 5*(1<<shardVsContainerExponent)+1, nil)
	if !done {
		t.Fatalf("limit filter should have been done, but got inc: %v done: %v", inc, done)
	}
}

func TestFilterWithRows(t *testing.T) {
	tests := []struct {
		rows     []uint64
		callWith []uint64
		expect   [][2]bool
	}{
		{
			rows:     []uint64{},
			callWith: []uint64{0},
			expect:   [][2]bool{{false, true}},
		},
		{
			rows:     []uint64{0},
			callWith: []uint64{0},
			expect:   [][2]bool{{true, true}},
		},
		{
			rows:     []uint64{1},
			callWith: []uint64{0, 2},
			expect:   [][2]bool{{false, false}, {false, true}},
		},
		{
			rows:     []uint64{0},
			callWith: []uint64{1, 2},
			expect:   [][2]bool{{false, true}, {false, true}},
		},
		{
			rows:     []uint64{3, 9},
			callWith: []uint64{1, 2, 3, 10},
			expect:   [][2]bool{{false, false}, {false, false}, {true, false}, {false, true}},
		},
		{
			rows:     []uint64{0, 1, 2},
			callWith: []uint64{0, 1, 2},
			expect:   [][2]bool{{true, false}, {true, false}, {true, true}},
		},
	}

	for num, test := range tests {
		t.Run(fmt.Sprintf("%d_%v_with_%v", num, test.rows, test.callWith), func(t *testing.T) {
			if len(test.callWith) != len(test.expect) {
				t.Fatalf("Badly specified test - must expect the same number of values as calls.")
			}
			f := filterWithRows(test.rows)
			for i, id := range test.callWith {
				inc, done := f(id, 0, nil)
				if inc != test.expect[i][0] || done != test.expect[i][1] {
					t.Fatalf("Calling with %d\nexp: %v,%v\ngot: %v,%v", id, test.expect[i][0], test.expect[i][1], inc, done)
				}
			}
		})
	}

}

func TestFieldRowMarshalJSON(t *testing.T) {
	fr := FieldRow{
		Field:  "blah",
		RowID:  0,
		RowKey: "ha",
	}
	b, err := json.Marshal(fr)
	if err != nil {
		t.Fatalf("marshalling fieldrow: %v", err)
	}
	if string(b) != `{"field":"blah","rowKey":"ha"}` {
		t.Fatalf("unexpected json: %s", b)
	}

	fr = FieldRow{
		Field:  "blah",
		RowID:  2,
		RowKey: "",
	}
	b, err = json.Marshal(fr)
	if err != nil {
		t.Fatalf("marshalling fieldrow: %v", err)
	}
	if string(b) != `{"field":"blah","rowID":2}` {
		t.Fatalf("unexpected json: %s", b)
	}
}

func TestExecutor_GroupCountCondition(t *testing.T) {
	t.Run("satisfiesCondition", func(t *testing.T) {
		type condCheck struct {
			cond string
			exp  bool
		}
		tests := []struct {
			groupCount GroupCount
			checks     []condCheck
		}{
			{
				groupCount: GroupCount{Count: 100},
				checks: []condCheck{
					{cond: "count == 99", exp: false},
					{cond: "count != 99", exp: true},
					{cond: "count < 99", exp: false},
					{cond: "count <= 99", exp: false},
					{cond: "count > 99", exp: true},
					{cond: "count >= 99", exp: true},

					{cond: "count == 100", exp: true},
					{cond: "count != 100", exp: false},
					{cond: "count < 100", exp: false},
					{cond: "count <= 100", exp: true},
					{cond: "count > 100", exp: false},
					{cond: "count >= 100", exp: true},

					{cond: "count == 101", exp: false},
					{cond: "count != 101", exp: true},
					{cond: "count < 101", exp: true},
					{cond: "count <= 101", exp: true},
					{cond: "count > 101", exp: false},
					{cond: "count >= 101", exp: false},

					{cond: "98 < count < 100", exp: false},
					{cond: "98 < count <= 100", exp: true},
					{cond: "98 < count < 101", exp: true},
					{cond: "100 <= count < 102", exp: true},
					{cond: "100 < count < 102", exp: false},
					{cond: "98 <= count <= 102", exp: true},
				},
			},
			{
				groupCount: GroupCount{Sum: 100},
				checks: []condCheck{
					{cond: "sum == 99", exp: false},
					{cond: "sum != 99", exp: true},
					{cond: "sum < 99", exp: false},
					{cond: "sum <= 99", exp: false},
					{cond: "sum > 99", exp: true},
					{cond: "sum >= 99", exp: true},

					{cond: "sum == 100", exp: true},
					{cond: "sum != 100", exp: false},
					{cond: "sum < 100", exp: false},
					{cond: "sum <= 100", exp: true},
					{cond: "sum > 100", exp: false},
					{cond: "sum >= 100", exp: true},

					{cond: "sum == 101", exp: false},
					{cond: "sum != 101", exp: true},
					{cond: "sum < 101", exp: true},
					{cond: "sum <= 101", exp: true},
					{cond: "sum > 101", exp: false},
					{cond: "sum >= 101", exp: false},

					{cond: "98 < sum < 100", exp: false},
					{cond: "98 < sum <= 100", exp: true},
					{cond: "98 < sum < 101", exp: true},
					{cond: "100 <= sum < 102", exp: true},
					{cond: "100 < sum < 102", exp: false},
					{cond: "98 <= sum <= 102", exp: true},
				},
			},
			{
				groupCount: GroupCount{Sum: -100},
				checks: []condCheck{
					{cond: "sum == -99", exp: false},
					{cond: "sum != -99", exp: true},
					{cond: "sum < -99", exp: true},
					{cond: "sum <= -99", exp: true},
					{cond: "sum > -99", exp: false},
					{cond: "sum >= -99", exp: false},

					{cond: "sum == -100", exp: true},
					{cond: "sum != -100", exp: false},
					{cond: "sum < -100", exp: false},
					{cond: "sum <= -100", exp: true},
					{cond: "sum > -100", exp: false},
					{cond: "sum >= -100", exp: true},

					{cond: "sum == -101", exp: false},
					{cond: "sum != -101", exp: true},
					{cond: "sum < -101", exp: false},
					{cond: "sum <= -101", exp: false},
					{cond: "sum > -101", exp: true},
					{cond: "sum >= -101", exp: true},

					{cond: "-100 < sum < -98", exp: false},
					{cond: "-100 <= sum < -98", exp: true},
					{cond: "-101 < sum < -98", exp: true},
					{cond: "-102 < sum <= -100", exp: true},
					{cond: "-102 < sum < -100", exp: false},
					{cond: "-102 <= sum <= -98", exp: true},
				},
			},
		}
		for i, test := range tests {
			t.Run(fmt.Sprintf("test (#%d):", i), func(t *testing.T) {
				for j, check := range test.checks {
					t.Run(fmt.Sprintf("check (#%d):", j), func(t *testing.T) {

						query, err := pql.ParseString(fmt.Sprintf("GroupBy(Rows(a), having=Condition(%s))", check.cond))
						if err != nil {
							t.Fatalf("parsing query: %v", err)
						}
						c := query.Calls[0]
						having := c.Args["having"].(*pql.Call)

						var got bool
						for subj, cond := range having.Args {
							switch subj {
							case "count", "sum":
								condition, ok := cond.(*pql.Condition)
								if !ok {
									t.Fatalf("not a valid condition")
								}
								got = test.groupCount.satisfiesCondition(subj, condition)
							}
						}

						if got != check.exp {
							t.Fatalf("expected: %v, but got: %v", check.exp, got)
						}
					})
				}
			})
		}
	})
}

func TestValCountComparisons(t *testing.T) {
	tests := []struct {
		name       string
		vc         ValCount
		other      ValCount
		expLarger  ValCount
		expSmaller ValCount
	}{
		{
			name: "zero",
		},
		{
			name:       "ints",
			vc:         ValCount{Val: 10, Count: 1},
			other:      ValCount{Val: 3, Count: 2},
			expLarger:  ValCount{Val: 10, Count: 1},
			expSmaller: ValCount{Val: 3, Count: 2},
		},
		{
			name:       "floats",
			vc:         ValCount{FloatVal: 10.2, Count: 1},
			other:      ValCount{FloatVal: 3.4, Count: 2},
			expLarger:  ValCount{FloatVal: 10.2, Count: 1},
			expSmaller: ValCount{FloatVal: 3.4, Count: 2},
		},
		{
			name:       "intsEquality",
			vc:         ValCount{Val: 10, Count: 1},
			other:      ValCount{Val: 10, Count: 2},
			expLarger:  ValCount{Val: 10, Count: 3},
			expSmaller: ValCount{Val: 10, Count: 3},
		},
		{
			name:       "floatsEquality",
			vc:         ValCount{FloatVal: 10.7, Count: 1},
			other:      ValCount{FloatVal: 10.7, Count: 2},
			expLarger:  ValCount{FloatVal: 10.7, Count: 3},
			expSmaller: ValCount{FloatVal: 10.7, Count: 3},
		},
	}

	for i, test := range tests {
		t.Run(test.name+strconv.Itoa(i), func(t *testing.T) {
			gotLarger := test.vc.larger(test.other)
			if gotLarger != test.expLarger {
				t.Fatalf("larger failed, expected:\n%+v\ngot:\n%+v", test.expLarger, gotLarger)
			}

			gotSmaller := test.vc.smaller(test.other)
			if gotSmaller != test.expSmaller {
				t.Fatalf("smaller failed, expected:\n%+v\ngot:\n%+v", test.expSmaller, gotSmaller)
			}
		})
	}
}

func TestToNegInt64(t *testing.T) {
	tests := []struct {
		u64      uint64
		i64      int64
		overflow bool
	}{
		{
			u64: uint64(1 << 63),
			i64: int64(-1 << 63),
		},
		{
			u64: uint64(1<<63) - 1,
			i64: int64(-1<<63) + 1,
		},
		{
			u64:      uint64(1<<63) + 1,
			overflow: true,
		},
	}

	for _, tc := range tests {
		val, err := toNegInt64(tc.u64)
		if err != nil && !tc.overflow {
			t.Fatalf("error: %+v, expected: %+v", err, tc)
		}

		if val != tc.i64 {
			t.Fatalf("Expected: %+v, Got: %+v", tc.i64, val)
		}
	}
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		u64      uint64
		i64      int64
		overflow bool
	}{
		{
			u64: uint64(1<<63) - 1,
			i64: 1<<63 - 1,
		},
		{
			u64: uint64(0),
			i64: 0,
		},
		{
			u64:      uint64(1 << 63),
			overflow: true,
		},
		{
			u64:      1<<64 - 1,
			overflow: true,
		},
	}

	for _, tc := range tests {
		val, err := toInt64(tc.u64)
		if err != nil && !tc.overflow {
			t.Fatalf("error: %+v, expected: %+v", err, tc)
		}

		if val != tc.i64 {
			t.Fatalf("Expected: %+v, Got: %+v", tc.i64, val)
		}
	}
}
