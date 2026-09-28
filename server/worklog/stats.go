package worklog

import (
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/response"
	"smallgo/server/utils"
)

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func trimFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	return s
}

func monthRange(year, month int) (string, string) {
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	last := first.AddDate(0, 1, -1)
	return first.Format("2006-01-02"), last.Format("2006-01-02")
}

// workerMonthly 月度统计的一行。
type workerMonthly struct {
	WorkerID           uint    `json:"worker_id"`
	WorkerName         string  `json:"worker_name"`
	Days               float64 `json:"days"`
	Hours              float64 `json:"hours"`
	Pieces             float64 `json:"pieces"`
	WorkAmountCents    int64   `json:"work_amount_cents"`
	AdvanceCents       int64   `json:"advance_cents"`
	PayableCents       int64   `json:"payable_cents"`
	SettledAmountCents int64   `json:"settled_amount_cents"`
}

func statsDashboard(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		uid = ac.scopeUser()
		now := time.Now()
		start, end := monthRange(now.Year(), int(now.Month()))
		selfWorker, isWorker := ac.workerScope()
		var out struct {
			Month           string `json:"month"`
			WorkerCount     int64  `json:"worker_count"`
			MonthRecordCnt  int64  `json:"month_record_count"`
			MonthWorkCents  int64  `json:"month_work_cents"`
			UnsettledCents  int64  `json:"unsettled_cents"`
			UnsettledCount  int64  `json:"unsettled_count"`
			AdvanceCents    int64  `json:"advance_cents"`
			SettledCents    int64  `json:"settled_cents"`
		}
		out.Month = start[:7]
		workerQ := "user_id = ?"
		if isWorker {
			workerQ += " AND id = ?"
		}
		workerArgs := []interface{}{uid}
		if isWorker {
			workerArgs = append(workerArgs, selfWorker)
		}
		db.Model(&Worker{}).Where(workerQ, workerArgs...).Count(&out.WorkerCount)

		recQ := "user_id = ? AND date BETWEEN ? AND ?"
		recArgs := []interface{}{uid, start, end}
		if isWorker {
			recQ += " AND worker_id = ?"
			recArgs = append(recArgs, selfWorker)
		}
		db.Model(&WorkRecord{}).Where(recQ, recArgs...).Count(&out.MonthRecordCnt)
		db.Model(&WorkRecord{}).Where(recQ, recArgs...).
			Select("COALESCE(SUM(amount_cents),0)").Scan(&out.MonthWorkCents)

		unQ := "user_id = ? AND settlement_id IS NULL"
		unArgs := []interface{}{uid}
		if isWorker {
			unQ += " AND worker_id = ?"
			unArgs = append(unArgs, selfWorker)
		}
		db.Model(&WorkRecord{}).Where(unQ, unArgs...).
			Select("COALESCE(SUM(amount_cents),0)").Scan(&out.UnsettledCents)
		db.Model(&WorkRecord{}).Where(unQ, unArgs...).Count(&out.UnsettledCount)
		db.Model(&Advance{}).Where(unQ, unArgs...).
			Select("COALESCE(SUM(amount_cents),0)").Scan(&out.AdvanceCents)
		setQ := "user_id = ?"
		setArgs := []interface{}{uid}
		if isWorker {
			setQ += " AND worker_id = ?"
			setArgs = append(setArgs, selfWorker)
		}
		db.Model(&Settlement{}).Where(setQ, setArgs...).
			Select("COALESCE(SUM(work_amount_cents),0)").Scan(&out.SettledCents)
		response.Success(c, out)
	}
}

func statsMonthly(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		uid = ac.scopeUser()
		now := time.Now()
		year := atoiDefault(c.Query("year"), now.Year())
		month := atoiDefault(c.Query("month"), int(now.Month()))
		if month < 1 || month > 12 {
			response.ErrorBadRequest(c, "月份不正确")
			return
		}
		start, end := monthRange(year, month)
		projectID := c.Query("project_id")

		rq := db.Model(&WorkRecord{}).Where("user_id = ? AND date BETWEEN ? AND ?", uid, start, end)
		aq := db.Model(&Advance{}).Where("user_id = ? AND date BETWEEN ? AND ?", uid, start, end)
		if projectID != "" {
			rq = rq.Where("project_id = ?", projectID)
		}
		if sw, ok := ac.workerScope(); ok {
			rq = rq.Where("worker_id = ?", sw)
			aq = aq.Where("worker_id = ?", sw)
		}
		type typeSum struct {
			WorkerID uint
			Type     string
			Total    float64
			Amount   int64
		}
		var sums []typeSum
		if err := rq.Select("worker_id, type, COALESCE(SUM(CAST(quantity AS REAL)),0) AS total, COALESCE(SUM(amount_cents),0) AS amount").
			Group("worker_id, type").Scan(&sums).Error; err != nil {
			response.ErrorInternal(c, "统计失败")
			return
		}
		type advSum struct {
			WorkerID uint
			Total    int64
		}
		var advs []advSum
		if err := aq.Select("worker_id, COALESCE(SUM(amount_cents),0) AS total").
			Group("worker_id").Scan(&advs).Error; err != nil {
			response.ErrorInternal(c, "统计失败")
			return
		}

		byWorker := map[uint]*workerMonthly{}
		for _, s := range sums {
			row := byWorker[s.WorkerID]
			if row == nil {
				row = &workerMonthly{WorkerID: s.WorkerID}
				byWorker[s.WorkerID] = row
			}
			switch s.Type {
			case TypeDay:
				row.Days += s.Total
			case TypeHour:
				row.Hours += s.Total
			case TypePiece:
				row.Pieces += s.Total
			}
			row.WorkAmountCents += s.Amount
		}
		for _, a := range advs {
			row := byWorker[a.WorkerID]
			if row == nil {
				row = &workerMonthly{WorkerID: a.WorkerID}
				byWorker[a.WorkerID] = row
			}
			row.AdvanceCents += a.Total
		}
		// 填充姓名
		ids := make([]uint, 0, len(byWorker))
		for id := range byWorker {
			ids = append(ids, id)
		}
		if len(ids) > 0 {
			var workers []Worker
			db.Where("id IN ?", ids).Find(&workers)
			for _, w := range workers {
				if row := byWorker[w.ID]; row != nil {
					row.WorkerName = w.Name
				}
			}
		}
		result := make([]workerMonthly, 0, len(byWorker))
		for _, row := range byWorker {
			row.PayableCents = row.WorkAmountCents - row.AdvanceCents
			result = append(result, *row)
		}
		// 简单按姓名排序
		for i := 0; i < len(result); i++ {
			for j := i + 1; j < len(result); j++ {
				if result[j].WorkerName < result[i].WorkerName {
					result[i], result[j] = result[j], result[i]
				}
			}
		}
		response.Success(c, gin.H{"year": year, "month": month, "start": start, "end": end, "rows": result})
	}
}

func statsRange(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		uid = ac.scopeUser()
		start, end := c.Query("start"), c.Query("end")
		if !validDate(start) || !validDate(end) {
			response.ErrorBadRequest(c, "请提供 start/end（YYYY-MM-DD）")
			return
		}
		workerID := c.Query("worker_id")
		if sw, ok := ac.workerScope(); ok {
			workerID = strconv.FormatUint(uint64(sw), 10)
		}
		projectID := c.Query("project_id")

		// buildRecordQ 每次返回新链：rq 若在 Select(SUM) 后被 Find 复用，
		// GORM 的 Count/Scan 会把残留的 SELECT 子句带回下一条查询，扫进
		// WorkRecord 时报错（整页 500）。
		buildRecordQ := func() *gorm.DB {
			q := db.Model(&WorkRecord{}).Where("user_id = ? AND date BETWEEN ? AND ?", uid, start, end)
			if workerID != "" {
				q = q.Where("worker_id = ?", workerID)
			}
			if projectID != "" {
				q = q.Where("project_id = ?", projectID)
			}
			return q
		}
		aq := db.Model(&Advance{}).Where("user_id = ? AND date BETWEEN ? AND ?", uid, start, end)
		if workerID != "" {
			aq = aq.Where("worker_id = ?", workerID)
		}
		var workTotal, advanceTotal, recordCount int64
		buildRecordQ().Select("COALESCE(SUM(amount_cents),0)").Scan(&workTotal)
		buildRecordQ().Count(&recordCount)
		aq.Select("COALESCE(SUM(amount_cents),0)").Scan(&advanceTotal)

		var records []WorkRecord
		if err := buildRecordQ().Order("date DESC, id DESC").Limit(2000).Find(&records).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		fillRecordNames(db, records)
		response.Success(c, gin.H{
			"start": start, "end": end,
			"work_amount_cents": workTotal, "advance_cents": advanceTotal,
			"payable_cents": workTotal - advanceTotal, "record_count": recordCount,
			"records": records,
		})
	}
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}

// attendanceCell 考勤日历的一天。
type attendanceCell struct {
	Date        string  `json:"date"`
	Days        float64 `json:"days"`
	Hours       float64 `json:"hours"`
	Pieces      float64 `json:"pieces"`
	Rest        bool    `json:"rest"`
	RecordCount int     `json:"record_count"`
	AmountCents int64   `json:"amount_cents"`
}

// statsAttendance 考勤表：某工人某月的每日出勤汇总 + 月度合计。
func statsAttendance(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		dsUID := ac.scopeUser()
		now := time.Now()
		year := atoiDefault(c.Query("year"), now.Year())
		month := atoiDefault(c.Query("month"), int(now.Month()))
		if month < 1 || month > 12 {
			response.ErrorBadRequest(c, "月份不正确")
			return
		}
		start, end := monthRange(year, month)

		workerID := utils.Atoi(c.Query("worker_id"), 0)
		if sw, ok := ac.workerScope(); ok {
			workerID = int(sw)
		}
		if workerID <= 0 {
			// 默认取第一个有记录的工人；都没有则取第一个在职工人
			var w Worker
			q := db.Where("user_id = ? AND status = ?", dsUID, "active")
			if err := q.Order("name ASC").First(&w).Error; err != nil {
				response.Success(c, gin.H{"worker": nil, "cells": []attendanceCell{}, "days": 0, "hours": 0, "pieces": 0, "rest_count": 0, "amount_cents": 0})
				return
			}
			workerID = int(w.ID)
		}
		var worker Worker
		if err := db.Where("user_id = ? AND id = ?", dsUID, workerID).First(&worker).Error; err != nil {
			response.ErrorBadRequest(c, "工人不存在")
			return
		}

		var records []WorkRecord
		if err := db.Where("user_id = ? AND worker_id = ? AND date BETWEEN ? AND ?", dsUID, workerID, start, end).
			Order("date ASC, id ASC").Find(&records).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		byDate := map[string]*attendanceCell{}
		var totalDays, totalHours, totalPieces float64
		var restCount int
		var totalAmount int64
		for _, r := range records {
			cell := byDate[r.Date]
			if cell == nil {
				cell = &attendanceCell{Date: r.Date}
				byDate[r.Date] = cell
			}
			cell.RecordCount++
			cell.AmountCents += r.AmountCents
			totalAmount += r.AmountCents
			if r.Type == TypeRest {
				cell.Rest = true
				continue
			}
			q := parseFloat(r.Quantity)
			switch r.Type {
			case TypeDay:
				cell.Days += q
				totalDays += q
			case TypeHour:
				cell.Hours += q
				totalHours += q
			case TypePiece:
				cell.Pieces += q
				totalPieces += q
			}
		}
		for _, cell := range byDate {
			if cell.Rest {
				restCount++
			}
		}
		// 按日期排序输出
		dates := make([]string, 0, len(byDate))
		for d := range byDate {
			dates = append(dates, d)
		}
		sort.Strings(dates)
		cells := make([]attendanceCell, 0, len(dates))
		for _, d := range dates {
			cells = append(cells, *byDate[d])
		}
		response.Success(c, gin.H{
			"worker":       worker,
			"year":         year,
			"month":        month,
			"days":         totalDays,
			"hours":        totalHours,
			"pieces":       totalPieces,
			"rest_count":   restCount,
			"amount_cents": totalAmount,
			"cells":        cells,
		})
	}
}
