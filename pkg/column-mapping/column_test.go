// Copyright 2018 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package column

import (
	"fmt"
	"testing"

	. "github.com/pingcap/check"
)

func TestClient(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&testColumnMappingSuit{})

type testColumnMappingSuit struct{}

func (t *testColumnMappingSuit) TestRule(c *C) {
	// test invalid rules
	inValidRule := &Rule{"test*", "abc*", "id", "id", "Error", nil, "xxx"}
	c.Assert(inValidRule.Valid(), NotNil)

	inValidRule.TargetColumn = ""
	c.Assert(inValidRule.Valid(), NotNil)

	inValidRule.Expression = AddPrefix
	inValidRule.TargetColumn = "id"
	c.Assert(inValidRule.Valid(), NotNil)

	inValidRule.Arguments = []string{"1"}
	c.Assert(inValidRule.Valid(), IsNil)

	inValidRule.Expression = PartitionID
	c.Assert(inValidRule.Valid(), NotNil)

	inValidRule.Arguments = []string{"1", "test_", "t_"}
	c.Assert(inValidRule.Valid(), IsNil)
}

func (t *testColumnMappingSuit) TestHandle(c *C) {
	rules := []*Rule{
		{"test*", "xxx*", "", "id", AddPrefix, []string{"instance_id:"}, "xx"},
	}

	// initial column mapping
	m, err := NewMapping(rules)
	c.Assert(err, IsNil)
	c.Assert(m.cache.infos, HasLen, 0)

	// test add prefix, add suffix is similar
	vals, poss, err := m.HandleRowValue("test", "xxx", []string{"age", "id"}, []interface{}{1, "1"})
	c.Assert(err, IsNil)
	c.Assert(vals, DeepEquals, []interface{}{1, "instance_id:1"})
	c.Assert(poss, DeepEquals, []int{-1, 1})

	// test cache
	vals, poss, err = m.HandleRowValue("test", "xxx", []string{"name"}, []interface{}{1, "1"})
	c.Assert(err, IsNil)
	c.Assert(vals, DeepEquals, []interface{}{1, "instance_id:1"})
	c.Assert(poss, DeepEquals, []int{-1, 1})

	// test resetCache
	m.resetCache()
	_, _, err = m.HandleRowValue("test", "xxx", []string{"name"}, []interface{}{"1"})
	c.Assert(err, NotNil)

	// test DDL
	_, poss, err = m.HandleDDL("test", "xxx", []string{"id", "age"}, "create table xxx")
	c.Assert(err, NotNil)

	statement, poss, err := m.HandleDDL("abc", "xxx", []string{"id", "age"}, "create table xxx")
	c.Assert(err, IsNil)
	c.Assert(statement, Equals, "create table xxx")
	c.Assert(poss, IsNil)
}

func (t *testColumnMappingSuit) TestQueryColumnInfo(c *C) {
	SetPartitionRule(4, 7, 8)
	rules := []*Rule{
		{"test*", "xxx*", "", "id", PartitionID, []string{"8", "test_", "xxx_"}, "xx"},
	}

	// initial column mapping
	m, err := NewMapping(rules)
	c.Assert(err, IsNil)

	// test mismatch
	info, err := m.queryColumnInfo("test_2", "t_1", []string{"id", "name"})
	c.Assert(err, IsNil)
	c.Assert(info.ignore, IsTrue)

	// test matched
	info, err = m.queryColumnInfo("test_2", "xxx_1", []string{"id", "name"})
	c.Assert(err, IsNil)
	c.Assert(info, DeepEquals, &mappingInfo{
		sourcePosition: -1,
		targetPosition: 0,
		rule:           rules[0],
		instanceID:     int64(8 << 59),
		schemaID:       int64(2 << 52),
		tableID:        int64(1 << 44),
	})

	m.resetCache()
	SetPartitionRule(0, 0, 3)
	info, err = m.queryColumnInfo("test_2", "xxx_1", []string{"id", "name"})
	c.Assert(info, DeepEquals, &mappingInfo{
		sourcePosition: -1,
		targetPosition: 0,
		rule:           rules[0],
		instanceID:     int64(0),
		schemaID:       int64(0),
		tableID:        int64(1 << 60),
	})
}

func (t *testColumnMappingSuit) TestSetPartitionRule(c *C) {
	SetPartitionRule(4, 7, 8)
	c.Assert(instanceIDBitSize, Equals, 4)
	c.Assert(schemaIDBitSize, Equals, 7)
	c.Assert(tableIDBitSize, Equals, 8)
	c.Assert(maxOriginID, Equals, int64(1<<44))

	SetPartitionRule(0, 3, 4)
	c.Assert(instanceIDBitSize, Equals, 0)
	c.Assert(schemaIDBitSize, Equals, 3)
	c.Assert(tableIDBitSize, Equals, 4)
	c.Assert(maxOriginID, Equals, int64(1<<56))
}

func (t *testColumnMappingSuit) TestComputePartitionID(c *C) {
	SetPartitionRule(4, 7, 8)

	rule := &Rule{
		Arguments: []string{"test", "t"},
	}
	_, _, _, err := computePartitionID("test_1", "t_1", rule)
	c.Assert(err, NotNil)
	_, _, _, err = computePartitionID("test", "t", rule)
	c.Assert(err, NotNil)

	rule = &Rule{
		Arguments: []string{"2", "test_", "t_"},
	}
	instanceID, schemaID, tableID, err := computePartitionID("test_1", "t_1", rule)
	c.Assert(err, IsNil)
	c.Assert(instanceID, Equals, int64(2<<59))
	c.Assert(schemaID, Equals, int64(1<<52))
	c.Assert(tableID, Equals, int64(1<<44))

	SetPartitionRule(4, 0, 8)
	rule = &Rule{
		Arguments: []string{"2", "test_", "t_"},
	}
	instanceID, schemaID, tableID, err = computePartitionID("test_1", "t_1", rule)
	c.Assert(err, IsNil)
	c.Assert(instanceID, Equals, int64(2<<59))
	c.Assert(schemaID, Equals, int64(0))
	c.Assert(tableID, Equals, int64(1<<51))
}

func (t *testColumnMappingSuit) TestPartitionID(c *C) {
	SetPartitionRule(4, 7, 8)
	info := &mappingInfo{
		instanceID:     int64(2 << 59),
		schemaID:       int64(1 << 52),
		tableID:        int64(1 << 44),
		targetPosition: 1,
	}

	// test wrong type
	_, err := partitionID(info, []interface{}{1, "ha"})
	c.Assert(err, NotNil)

	// test exceed maxOriginID
	_, err = partitionID(info, []interface{}{"ha", 1 << 44})
	c.Assert(err, NotNil)

	vals, err := partitionID(info, []interface{}{"ha", 1})
	c.Assert(err, IsNil)
	c.Assert(vals, DeepEquals, []interface{}{"ha", int64(2<<59 | 1<<52 | 1<<44 | 1)})

	info.instanceID = 0
	vals, err = partitionID(info, []interface{}{"ha", "123"})
	c.Assert(err, IsNil)
	c.Assert(vals, DeepEquals, []interface{}{"ha", fmt.Sprintf("%d", int64(1<<52|1<<44|123))})
}
