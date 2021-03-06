// Copyright 2020 PingCAP, Inc.
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

package slowquery

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/jinzhu/gorm"
)

const (
	SlowQueryTable = "INFORMATION_SCHEMA.CLUSTER_SLOW_QUERY"
	SelectStmt     = "*, (unix_timestamp(Time) + 0E0) as timestamp"
)

type Base struct {
	// list fields
	Digest string `gorm:"column:Digest" json:"digest"`
	Query  string `gorm:"column:Query" json:"query"`

	Instance     string `gorm:"column:INSTANCE" json:"instance"`
	DB           string `gorm:"column:DB" json:"db"`
	ConnectionID uint   `gorm:"column:Conn_ID" json:"connection_id"`
	Success      int    `gorm:"column:Succ" json:"success"`

	Time        string  `gorm:"column:Time" json:"time_str"`         // finish time
	Timestamp   float64 `gorm:"column:timestamp" json:"timestamp"`   // unix_timestamp(Time) as timestamp
	QueryTime   float64 `gorm:"column:Query_time" json:"query_time"` // latency
	ParseTime   float64 `gorm:"column:Parse_time" json:"parse_time"`
	CompileTime float64 `gorm:"column:Compile_time" json:"compile_time"`
	ProcessTime float64 `gorm:"column:Process_time" json:"process_time"`

	MemoryMax  int  `gorm:"column:Mem_max" json:"memory_max"`
	TxnStartTS uint `gorm:"column:Txn_start_ts" json:"txn_start_ts"`
}

type SlowQuery struct {
	*Base `gorm:"embedded"`

	// Detail
	PrevStmt string `gorm:"column:Prev_stmt" json:"prev_stmt"`
	Plan     string `gorm:"column:Plan" json:"plan"`

	// Basic
	IsInternal   int    `gorm:"column:Is_internal" json:"is_internal"`
	IndexNames   string `gorm:"column:Index_name" json:"index_names"`
	Stats        string `gorm:"column:Stats" json:"stats"`
	BackoffTypes string `gorm:"column:Backoff_types" json:"backoff_types"`

	// Connection
	User string `gorm:"column:User" json:"user"`
	Host string `gorm:"column:Host" json:"host"`

	// Time
	WaitTime           float64 `gorm:"column:Wait_time" json:"wait_time"`
	BackoffTime        float64 `gorm:"column:Backoff_time" json:"backoff_time"`
	GetCommitTSTime    float64 `gorm:"column:Get_commit_ts_time" json:"get_commit_ts_time"`
	LocalLatchWaitTime float64 `gorm:"column:Local_latch_wait_time" json:"local_latch_wait_time"`
	ResolveLockTime    float64 `gorm:"column:Resolve_lock_time" json:"resolve_lock_time"`
	PrewriteTime       float64 `gorm:"column:Prewrite_time" json:"prewrite_time"`
	CommitTime         float64 `gorm:"column:Commit_time" json:"commit_time"`
	CommitBackoffTime  float64 `gorm:"column:Commit_backoff_time" json:"commit_backoff_time"`
	CopProcAvg         float64 `gorm:"column:Cop_proc_avg" json:"cop_proc_avg"`
	CopProcP90         float64 `gorm:"column:Cop_proc_p90" json:"cop_proc_p90"`
	CopProcMax         float64 `gorm:"column:Cop_proc_max" json:"cop_proc_max"`
	CopWaitAvg         float64 `gorm:"column:Cop_wait_avg" json:"cop_wait_avg"`
	CopWaitP90         float64 `gorm:"column:Cop_wait_p90" json:"cop_wait_p90"`
	CopWaitMax         float64 `gorm:"column:Cop_wait_max" json:"cop_wait_max"`

	// Transaction
	WriteKeys      int `gorm:"column:Write_keys" json:"write_keys"`
	WriteSize      int `gorm:"column:Write_size" json:"write_size"`
	PrewriteRegion int `gorm:"column:Prewrite_region" json:"prewrite_region"`
	TxnRetry       int `gorm:"column:Txn_retry" json:"txn_retry"`

	// Coprocessor
	RequestCount uint   `gorm:"column:Request_count" json:"request_count"`
	ProcessKeys  uint   `gorm:"column:Process_keys" json:"process_keys"`
	TotalKeys    uint   `gorm:"column:Total_keys" json:"total_keys"`
	CopProcAddr  string `gorm:"column:Cop_proc_addr" json:"cop_proc_addr"`
	CopWaitAddr  string `gorm:"column:Cop_wait_addr" json:"cop_wait_addr"`
}

type GetListRequest struct {
	LogStartTS int64    `json:"logStartTS" form:"logStartTS"`
	LogEndTS   int64    `json:"logEndTS" form:"logEndTS"`
	DB         []string `json:"db" form:"db"`
	Limit      int      `json:"limit" form:"limit"`
	Text       string   `json:"text" form:"text"`
	OrderBy    string   `json:"orderBy" form:"orderBy"`
	DESC       bool     `json:"desc" form:"desc"`

	// for showing slow queries in the statement detail page
	Plans  []string `json:"plans" form:"plans"`
	Digest string   `json:"digest" form:"digest"`
}

type GetDetailRequest struct {
	Digest    string  `json:"digest" form:"digest"`
	Time      float64 `json:"time" form:"time"`
	ConnectID int64   `json:"connect_id" form:"connect_id"`
}

func getAllColumnNames() []string {
	t := reflect.TypeOf(Base{})
	fieldsNum := t.NumField()
	ret := make([]string, 0, fieldsNum)
	for i := 0; i < fieldsNum; i++ {
		field := t.Field(i)
		if s, ok := field.Tag.Lookup("gorm"); ok {
			list := strings.Split(s, ":")
			if len(list) < 1 {
				panic(fmt.Sprintf("Unknown gorm tag field: %s", s))
			}
			ret = append(ret, list[1])
		}
	}
	ret = append(ret, "Time")
	return ret
}

func isValidColumnName(name string) bool {
	for _, item := range getAllColumnNames() {
		if name == item {
			return true
		}
	}
	return false
}

func QuerySlowLogList(db *gorm.DB, req *GetListRequest) ([]Base, error) {
	tx := db.
		Table(SlowQueryTable).
		Select(SelectStmt).
		Where("time between from_unixtime(?) and from_unixtime(?)", req.LogStartTS, req.LogEndTS).
		Limit(req.Limit)

	if req.Text != "" {
		lowerStr := strings.ToLower(req.Text)
		tx = tx.Where("txn_start_ts REGEXP ? OR LOWER(digest) REGEXP ? OR LOWER(CONVERT(prev_stmt USING utf8)) REGEXP ? OR LOWER(CONVERT(query USING utf8)) REGEXP ?",
			lowerStr,
			lowerStr,
			lowerStr,
			lowerStr,
		)
	}

	if len(req.DB) > 0 {
		tx = tx.Where("DB IN (?)", req.DB)
	}

	order := req.OrderBy
	if isValidColumnName(order) {
		if req.DESC {
			tx = tx.Order(fmt.Sprintf("%s desc", order))
		} else {
			tx = tx.Order(fmt.Sprintf("%s asc", order))
		}
	}

	if len(req.Plans) > 0 {
		tx = tx.Where("Plan_digest IN (?)", req.Plans)
	}

	if len(req.Digest) > 0 {
		tx = tx.Where("Digest = ?", req.Digest)
	}

	var results []Base
	err := tx.Find(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

func QuerySlowLogDetail(db *gorm.DB, req *GetDetailRequest) (*SlowQuery, error) {
	var result SlowQuery
	err := db.
		Table(SlowQueryTable).
		Select(SelectStmt).
		Where("Digest = ?", req.Digest).
		Where("Time = from_unixtime(?)", req.Time).
		Where("Conn_id = ?", req.ConnectID).
		First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}
