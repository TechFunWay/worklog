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
//
// 金额口径（v0.6.0 起）：WorkAmountCents = SUM(amount_cents)，**补贴已并入、扣款已减掉**，
// 它是全应用唯一的金额真相；SubsidyCents / DeductCents 只是把其中两项拆出来看一眼，
// 绝不能再往 WorkAmountCents 上加一次（加一次就重复计钱）。
type workerMonthly struct {
	WorkerID           uint    `json:"worker_id"`
	WorkerName         string  `json:"worker_name"`
	Days               float64 `json:"days"`
	Hours              float64 `json:"hours"`
	Pieces             float64 `json:"pieces"`
	WorkAmountCents    int64   `json:"work_amount_cents"`
	SubsidyCents       int64   `json:"subsidy_cents"`
	DeductCents        int64   `json:"deduct_cents"`
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
		// ⚠️ 两个「欠」字头口径，语义完全不同，前端与对账都不许混用、不许相加：
		//
		//   unsettled_cents —— 未结算记工：工记了、还没生成工资条（还在「待结算」池子里）。
		//   due_cents       —— 待付/欠薪：已经出了工资条（已结算），但付款流水没付够，
		//                      欠的钱留在结算单上，**绝不退回 unsettled 池**
		//                      （记工已被 settlement_id 锁定，退回会造成同一笔工结两次）。
		//
		// 补贴随 amount_cents 一起进 unsettled_cents；due_cents 随结算单的
		// payable_cents（= 记工 − 借支 + 调整）走，两边都不需要再加一次补贴。
		var out struct {
			Month          string `json:"month"`
			WorkerCount    int64  `json:"worker_count"`
			MonthRecordCnt int64  `json:"month_record_count"`
			MonthWorkCents int64  `json:"month_work_cents"`
			UnsettledCents int64  `json:"unsettled_cents"`
			UnsettledCount int64  `json:"unsettled_count"`
			AdvanceCents   int64  `json:"advance_cents"`
			SettledCents   int64  `json:"settled_cents"`
			DueCents       int64  `json:"due_cents"`
			DueCount       int64  `json:"due_count"`
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

		// 工资口径只算已审核记录；待审核在「报工审核」页单独看。
		recQ := "user_id = ? AND date BETWEEN ? AND ? AND status = ?"
		recArgs := []interface{}{uid, start, end, RecordStatusApproved}
		if isWorker {
			recQ += " AND worker_id = ?"
			recArgs = append(recArgs, selfWorker)
		}
		db.Model(&WorkRecord{}).Where(recQ, recArgs...).Count(&out.MonthRecordCnt)
		db.Model(&WorkRecord{}).Where(recQ, recArgs...).
			Select("COALESCE(SUM(amount_cents),0)").Scan(&out.MonthWorkCents)

		unRecQ := "user_id = ? AND settlement_id IS NULL AND status = ?"
		unRecArgs := []interface{}{uid, RecordStatusApproved}
		if isWorker {
			unRecQ += " AND worker_id = ?"
			unRecArgs = append(unRecArgs, selfWorker)
		}
		db.Model(&WorkRecord{}).Where(unRecQ, unRecArgs...).
			Select("COALESCE(SUM(amount_cents),0)").Scan(&out.UnsettledCents)
		db.Model(&WorkRecord{}).Where(unRecQ, unRecArgs...).Count(&out.UnsettledCount)

		unAdvQ := "user_id = ? AND settlement_id IS NULL"
		unAdvArgs := []interface{}{uid}
		if isWorker {
			unAdvQ += " AND worker_id = ?"
			unAdvArgs = append(unAdvArgs, selfWorker)
		}
		db.Model(&Advance{}).Where(unAdvQ, unAdvArgs...).
			Select("COALESCE(SUM(amount_cents),0)").Scan(&out.AdvanceCents)
		setQ := "user_id = ?"
		setArgs := []interface{}{uid}
		if isWorker {
			setQ += " AND worker_id = ?"
			setArgs = append(setArgs, selfWorker)
		}
		db.Model(&Settlement{}).Where(setQ, setArgs...).
			Select("COALESCE(SUM(work_amount_cents),0)").Scan(&out.SettledCents)

		// 待付（欠薪）口径：已结算单上「应付 − 已付」的正数部分之和。
		// 只算正数是刻意的：多付（付款流水打超了）不倒过来冲抵别的单的欠薪，
		// 负数由「已付」那一列本身去体现；借支倒挂造成的应付 ≤ 0 单子也不算欠薪
		// —— 与 amount.go 里 payStatusOf 的判定完全一致。
		// SQL 层一次聚合出金额与笔数，不逐单捞（避免 N+1）。
		var due struct {
			Cents int64
			Cnt   int64
		}
		dueSQL := "COALESCE(SUM(CASE WHEN payable_cents > paid_cents THEN payable_cents - paid_cents ELSE 0 END),0) AS cents, " +
			"COUNT(CASE WHEN payable_cents > paid_cents THEN 1 END) AS cnt"
		if err := db.Model(&Settlement{}).Where(setQ, setArgs...).Select(dueSQL).Scan(&due).Error; err != nil {
			response.ErrorInternal(c, "统计失败")
			return
		}
		out.DueCents = due.Cents
		out.DueCount = due.Cnt
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

		rq := db.Model(&WorkRecord{}).Where("user_id = ? AND date BETWEEN ? AND ? AND status = ?", uid, start, end, RecordStatusApproved)
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
			Subsidy  int64
			Deduct   int64
		}
		var sums []typeSum
		if err := rq.Select("worker_id, type, COALESCE(SUM(CAST(quantity AS REAL)),0) AS total, COALESCE(SUM(amount_cents),0) AS amount, COALESCE(SUM(subsidy_cents),0) AS subsidy, COALESCE(SUM(deduct_cents),0) AS deduct").
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
			// 合计只认 SUM(amount_cents)；补贴/扣款是「从合计里拆出来看」的口径，
			// 不要再拿去加回 WorkAmountCents。
			row.WorkAmountCents += s.Amount
			row.SubsidyCents += s.Subsidy
			row.DeductCents += s.Deduct
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
			q := db.Model(&WorkRecord{}).Where("user_id = ? AND date BETWEEN ? AND ? AND status = ?", uid, start, end, RecordStatusApproved)
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
		var advanceTotal, recordCount int64
		// work.Total 是合计真相（已含补贴、已减扣款）；Subsidy / Deduct 只把其中的补贴与扣款
		// 拆出来看，不参与任何重算 —— 见 models.go 里 WorkRecord 的金额口径注释。
		// 一次聚合出三个数，别为了拆项多发一条 SUM（同一份数据扫两遍容易在两处写法漂移）。
		var work struct {
			Total   int64
			Subsidy int64
			Deduct  int64
		}
		buildRecordQ().Select("COALESCE(SUM(amount_cents),0) AS total, COALESCE(SUM(subsidy_cents),0) AS subsidy, COALESCE(SUM(deduct_cents),0) AS deduct").Scan(&work)
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
			"work_amount_cents": work.Total, "advance_cents": advanceTotal,
			"payable_cents": work.Total - advanceTotal, "record_count": recordCount,
			// 补贴/扣款汇总：work_amount_cents 已包含它们，这里只是拆项展示用。
			"subsidy_cents": work.Subsidy, "deduct_cents": work.Deduct,
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
//
// 金额口径（v0.6.0 起）：AmountCents = 当天各笔 SUM(amount_cents)，已含补贴、已减扣款；
// SubsidyCents / DeductCents 只给「这天为什么和 数量×单价 对不上」一个说法，
// 不参与重算，前端要显示合计一律用 AmountCents。
type attendanceCell struct {
	Date         string  `json:"date"`
	Days         float64 `json:"days"`
	Hours        float64 `json:"hours"`
	Pieces       float64 `json:"pieces"`
	Rest         bool    `json:"rest"`
	RecordCount  int     `json:"record_count"`
	AmountCents  int64   `json:"amount_cents"`
	SubsidyCents int64   `json:"subsidy_cents"`
	DeductCents  int64   `json:"deduct_cents"`
}

// statsAttendance 考勤表：某工人某月的每日出勤汇总 + 月度合计。
// 支持 ?project_id= 按工地过滤（none=未分工地），切换工地即可看该工地的考勤。
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
		pf, ok := parseProjectFilter(c)
		if !ok {
			response.ErrorBadRequest(c, "project_id 参数不正确")
			return
		}
		start, end := monthRange(year, month)

		workerID := utils.Atoi(c.Query("worker_id"), 0)
		if sw, ok := ac.workerScope(); ok {
			// 成员行没绑工人档案：不能落到下面的「取第一个在职工人」，
			// 那等于把别的工人的考勤交给这名员工。
			if ac.workerUnbound() {
				forbidden(c)
				return
			}
			workerID = int(sw)
		}
		if workerID <= 0 {
			// 默认取第一个有记录的工人；都没有则取第一个在职工人
			var w Worker
			q := db.Where("user_id = ? AND status = ?", dsUID, "active")
			if err := q.Order("name ASC").First(&w).Error; err != nil {
				response.Success(c, gin.H{"worker": nil, "cells": []attendanceCell{}, "days": 0, "hours": 0, "pieces": 0, "rest_count": 0, "amount_cents": 0, "subsidy_cents": 0, "deduct_cents": 0, "project_id": pf.echoID()})
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
		q := db.Where("user_id = ? AND worker_id = ? AND date BETWEEN ? AND ? AND status = ?", dsUID, workerID, start, end, RecordStatusApproved)
		q = pf.apply(q)
		if err := q.Order("date ASC, id ASC").Find(&records).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		byDate := map[string]*attendanceCell{}
		var totalDays, totalHours, totalPieces float64
		var restCount int
		var totalAmount, totalSubsidy, totalDeduct int64
		for _, r := range records {
			cell := byDate[r.Date]
			if cell == nil {
				cell = &attendanceCell{Date: r.Date}
				byDate[r.Date] = cell
			}
			cell.RecordCount++
			// 合计仍以 amount_cents 为准（补贴/扣款已在写入口并进去）；
			// 这里只是把补贴、扣款原样带出来，供日历格子解释「为什么和 数量×单价 不等」。
			cell.AmountCents += r.AmountCents
			cell.SubsidyCents += r.SubsidyCents
			cell.DeductCents += r.DeductCents
			totalAmount += r.AmountCents
			totalSubsidy += r.SubsidyCents
			totalDeduct += r.DeductCents
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
			"worker":        worker,
			"year":          year,
			"month":         month,
			"project_id":    pf.echoID(),
			"days":          totalDays,
			"hours":         totalHours,
			"pieces":        totalPieces,
			"rest_count":    restCount,
			"amount_cents":  totalAmount,
			"subsidy_cents": totalSubsidy,
			"deduct_cents":  totalDeduct,
			"cells":         cells,
		})
	}
}

// projectStatRow 按工地统计的一行：记工笔数、应发工资、借支、净工资与已结算。
// 收支账（收入/支出）不属于本功能，这里不输出该维度。
//
// 金额口径（v0.6.0 起）：
//   - WorkAmountCents = 该工地 SUM(amount_cents)，补贴已并入、扣款已减掉，
//     SubsidyCents / DeductCents 只是拆项（老板要看「这个工地补了多少钱」），
//     **不参与重算**，净额一律以 WorkAmountCents 为准；
//   - AdvanceCents = 该工地 SUM(amount_cents) 的借支，**只统计 project_id 明确
//     指向该工地的借支**；未指定工地（project_id IS NULL）的借支不进任何一行，
//     「未分工地」行的 advance_cents 也保持 0 —— 它与结算口径一致：散借支只在
//     按人结算时抵扣（见 docs/06 §4.2），塞进「未分工地」行等于凭空造出一个
//     并不存在的抵扣额度，按项目结算时会被重复扣一次。
type projectStatRow struct {
	ProjectID       *uint  `json:"project_id"`
	ProjectName     string `json:"project_name"`
	RecordCount     int64  `json:"record_count"`
	WorkAmountCents int64  `json:"work_amount_cents"`
	SubsidyCents    int64  `json:"subsidy_cents"`
	DeductCents     int64  `json:"deduct_cents"`
	AdvanceCents    int64  `json:"advance_cents"`
	PayableCents    int64  `json:"payable_cents"`
	SettledCents    int64  `json:"settled_cents"`
	// DueCents 该工地已结算但尚未付清的部分（应付 − 已付，只累正数），
	// 与 unsettled（还没出工资条）是两回事，不能相加解读。
	DueCents int64 `json:"due_cents"`
}

// statsByProject 按工地单独统计工资（可限定月份），末行附带「未分工地」与合计。
func statsByProject(db *gorm.DB) gin.HandlerFunc {
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
		dateQ := "user_id = ? AND date BETWEEN ? AND ?"
		// 员工角色只看自己：与 statsMonthly / statsDashboard / statsRange 同一套口径。
		// 这里原先完全没有 worker 过滤，一名员工调本接口就能看到整个班组的工资分布；
		// 三条聚合（记工 / 借支 / 结算）必须一起加，加一条就是三套口径打架。
		// 未绑定工人档案的成员行拿到 sw = 0，`worker_id = 0` 天然查不到任何数据（fail closed），
		// 与 statsMonthly 的做法保持一致，不需要额外的 403 分支。
		selfWorker, isWorker := ac.workerScope()

		// 工资与借支按 project_id 分组（SQL 层聚合，避免 N+1）
		type workSum struct {
			ProjectID *uint
			Cnt       int64
			Amount    int64
			Subsidy   int64
			Deduct    int64
		}
		workQ := db.Model(&WorkRecord{}).Where(dateQ+" AND status = ?", dsUID, start, end, RecordStatusApproved)
		if isWorker {
			workQ = workQ.Where("worker_id = ?", selfWorker)
		}
		var workSums []workSum
		if err := workQ.
			Select("project_id, COUNT(*) AS cnt, COALESCE(SUM(amount_cents),0) AS amount, COALESCE(SUM(subsidy_cents),0) AS subsidy, COALESCE(SUM(deduct_cents),0) AS deduct").
			Group("project_id").Scan(&workSums).Error; err != nil {
			response.ErrorInternal(c, "统计失败")
			return
		}
		// 借支按 project_id 分组填进 AdvanceCents。
		//
		// 这里原本是个**现存真 bug**（docs/06 §2.4-A）：AdvanceCents 声明了、也参与
		// 「Payable = Work − Advance」赋值，但从头到尾没有任何查询给它赋值，恒为 0，
		// 于是「按工地统计」的净额虚高（借出去的钱没从应发里扣）。v0.6.0 借支有了
		// project_id，正好一条 GROUP BY 补上。
		// WHERE 里带 project_id IS NOT NULL：未指定工地的散借支不属于任何工地，
		// 由按人结算抵扣（见 projectStatRow 注释），这里计入就会重复扣钱。
		type advSum struct {
			ProjectID uint
			Amount    int64
		}
		advQ := db.Model(&Advance{}).Where("user_id = ? AND date BETWEEN ? AND ? AND project_id IS NOT NULL", dsUID, start, end)
		if isWorker {
			advQ = advQ.Where("worker_id = ?", selfWorker)
		}
		var advSums []advSum
		if err := advQ.
			Select("project_id, COALESCE(SUM(amount_cents),0) AS amount").
			Group("project_id").Scan(&advSums).Error; err != nil {
			response.ErrorInternal(c, "统计失败")
			return
		}
		// 已结算金额按结算单的项目维度汇总；顺带把该工地「已结算未付清」的待付额
		// 一并聚合出来（只算 payable > paid 的正数部分）。
		type setSum struct {
			ProjectID *uint
			Amount    int64
			Due       int64
		}
		setQ := db.Model(&Settlement{}).Where("user_id = ? AND settled_at >= ? AND settled_at < ?", dsUID, start+" 00:00:00", monthEndExclusive(year, month))
		if isWorker {
			setQ = setQ.Where("worker_id = ?", selfWorker)
		}
		var setSums []setSum
		if err := setQ.
			Select("project_id, COALESCE(SUM(work_amount_cents),0) AS amount, COALESCE(SUM(CASE WHEN payable_cents > paid_cents THEN payable_cents - paid_cents ELSE 0 END),0) AS due").
			Group("project_id").Scan(&setSums).Error; err != nil {
			response.ErrorInternal(c, "统计失败")
			return
		}
		rows := map[uint]*projectStatRow{}
		var noProject *projectStatRow
		getRow := func(pid *uint) *projectStatRow {
			if pid == nil {
				if noProject == nil {
					noProject = &projectStatRow{}
				}
				return noProject
			}
			row := rows[*pid]
			if row == nil {
				row = &projectStatRow{ProjectID: pid}
				rows[*pid] = row
			}
			return row
		}
		for _, s := range workSums {
			row := getRow(s.ProjectID)
			row.RecordCount += s.Cnt
			row.WorkAmountCents += s.Amount
			row.SubsidyCents += s.Subsidy
			row.DeductCents += s.Deduct
		}
		// 借支行可能落在本月没有任何记工的工地上（先借钱后开工），getRow 会补出
		// 一行 record_count=0、净额为负的真实数据，不能因为「没记工」就丢掉这笔借支。
		for _, s := range advSums {
			pid := s.ProjectID
			getRow(&pid).AdvanceCents += s.Amount
		}
		for _, s := range setSums {
			row := getRow(s.ProjectID)
			row.SettledCents += s.Amount
			row.DueCents += s.Due
		}

		// 填充工地名称
		names := map[uint]string{}
		if len(rows) > 0 {
			ids := make([]uint, 0, len(rows))
			for id := range rows {
				ids = append(ids, id)
			}
			var projects []Project
			db.Where("id IN ?", ids).Find(&projects)
			for _, p := range projects {
				names[p.ID] = p.Name
			}
		}

		result := make([]projectStatRow, 0, len(rows)+1)
		var total projectStatRow
		total.ProjectName = "合计"
		for id, row := range rows {
			row.ProjectName = names[id]
			result = append(result, *row)
		}
		for i := 0; i < len(result); i++ {
			for j := i + 1; j < len(result); j++ {
				if result[j].ProjectName < result[i].ProjectName {
					result[i], result[j] = result[j], result[i]
				}
			}
		}
		if noProject != nil {
			noProject.ProjectName = "未分工地"
			result = append(result, *noProject)
		}
		for i := range result {
			result[i].PayableCents = result[i].WorkAmountCents - result[i].AdvanceCents
		}
		for _, row := range result {
			total.RecordCount += row.RecordCount
			total.WorkAmountCents += row.WorkAmountCents
			total.SubsidyCents += row.SubsidyCents
			total.DeductCents += row.DeductCents
			total.AdvanceCents += row.AdvanceCents
			total.PayableCents += row.PayableCents
			total.SettledCents += row.SettledCents
			total.DueCents += row.DueCents
		}
		response.Success(c, gin.H{"year": year, "month": month, "start": start, "end": end, "rows": result, "total": total})
	}
}

// monthEndExclusive 月份结束的排他上界（下月 1 日 0 点），用于对时间戳字段
// （如 Settlement.settled_at）做区间过滤，避免依赖 SQLite 对字符串日期的隐式比较。
func monthEndExclusive(year, month int) string {
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	return first.AddDate(0, 1, 0).Format("2006-01-02 15:04:05")
}
