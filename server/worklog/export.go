package worklog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/response"
	"smallgo/server/utils"
)

func exportRecordsCSV(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		q := db.Where("user_id = ?", uid)
		if v := c.Query("worker_id"); v != "" {
			q = q.Where("worker_id = ?", v)
		}
		if v := c.Query("start"); validDate(v) {
			q = q.Where("date >= ?", v)
		}
		if v := c.Query("end"); validDate(v) {
			q = q.Where("date <= ?", v)
		}
		var records []WorkRecord
		if err := q.Order("date ASC, id ASC").Limit(50000).Find(&records).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		fillRecordNames(db, records)
		rows := [][]string{{"日期", "工人", "类型", "数量", "单价(元)", "金额(元)", "工地", "计件项目", "状态", "备注"}}
		for _, r := range records {
			status := "未结算"
			if r.SettlementID != nil {
				status = "已结算"
			}
			rows = append(rows, []string{
				r.Date, r.WorkerName, typeLabels[r.Type], r.Quantity,
				centsToYuanString(r.UnitPriceCents), centsToYuanString(r.AmountCents),
				r.ProjectName, pieceLabel(r), status, r.Note,
			})
		}
		utils.WriteCSV(c, utils.ExportFilename("记工明细", "csv"), rows)
	}
}

func pieceLabel(r WorkRecord) string {
	if r.PieceName == "" {
		return ""
	}
	return fmt.Sprintf("%s(%s)", r.PieceName, r.PieceUnit)
}

func exportMonthlyCSV(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		now := time.Now()
		year := atoiDefault(c.Query("year"), now.Year())
		month := atoiDefault(c.Query("month"), int(now.Month()))
		start, end := monthRange(year, month)
		var records []WorkRecord
		if err := db.Where("user_id = ? AND date BETWEEN ? AND ?", uid, start, end).
			Order("worker_id ASC, date ASC").Find(&records).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		fillRecordNames(db, records)
		rows := [][]string{{"工人", "日期", "类型", "数量", "单价(元)", "金额(元)", "工地", "计件项目", "备注"}}
		for _, r := range records {
			rows = append(rows, []string{
				r.WorkerName, r.Date, typeLabels[r.Type], r.Quantity,
				centsToYuanString(r.UnitPriceCents), centsToYuanString(r.AmountCents),
				r.ProjectName, pieceLabel(r), r.Note,
			})
		}
		utils.WriteCSV(c, utils.ExportFilename(fmt.Sprintf("工资月报-%d-%02d", year, month), "csv"), rows)
	}
}

// exportAttendanceCSV 导出某月「考勤表」：按工人逐日列出点工/点时/计件/休息
// 与当天工资，每人末尾带一行合计。不传 worker_id 时导出全部在职工人（含当月
// 有记录的已归档工人），传 worker_id 时只导出该工人；员工角色强制只导出自己。
func exportAttendanceCSV(db *gorm.DB) gin.HandlerFunc {
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

		var workers []Worker
		qw := db.Where("user_id = ?", dsUID)
		if workerID > 0 {
			qw = qw.Where("id = ?", workerID)
		} else {
			qw = qw.Where("status = ?", "active")
		}
		if err := qw.Order("name ASC, id ASC").Find(&workers).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		if workerID > 0 && len(workers) == 0 {
			response.ErrorBadRequest(c, "工人不存在")
			return
		}

		var records []WorkRecord
		q := db.Where("user_id = ? AND date BETWEEN ? AND ?", dsUID, start, end)
		if workerID > 0 {
			q = q.Where("worker_id = ?", workerID)
		}
		if err := q.Order("worker_id ASC, date ASC, id ASC").Find(&records).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}

		// 当月有记录、但已归档或不在职工人列表里的工人也要出现在考勤表里，
		// 否则「上月记了工、这月归档」的人会整月从考勤表消失。
		known := make(map[uint]bool, len(workers))
		for _, w := range workers {
			known[w.ID] = true
		}
		var extraIDs []uint
		for _, r := range records {
			if !known[r.WorkerID] {
				known[r.WorkerID] = true
				extraIDs = append(extraIDs, r.WorkerID)
			}
		}
		if len(extraIDs) > 0 {
			var extra []Worker
			if err := db.Where("user_id = ? AND id IN ?", dsUID, extraIDs).Find(&extra).Error; err != nil {
				response.ErrorInternal(c, "导出失败")
				return
			}
			workers = append(workers, extra...)
			sort.SliceStable(workers, func(i, j int) bool {
				if workers[i].Name == workers[j].Name {
					return workers[i].ID < workers[j].ID
				}
				return workers[i].Name < workers[j].Name
			})
		}

		// 工人 + 日期 聚合
		type dayAgg struct {
			days, hours, pieces float64
			rest                bool
			amountCents         int64
			notes               []string
		}
		agg := make(map[uint]map[string]*dayAgg, len(workers))
		for _, r := range records {
			byDate := agg[r.WorkerID]
			if byDate == nil {
				byDate = map[string]*dayAgg{}
				agg[r.WorkerID] = byDate
			}
			cell := byDate[r.Date]
			if cell == nil {
				cell = &dayAgg{}
				byDate[r.Date] = cell
			}
			cell.amountCents += r.AmountCents
			if r.Type == TypeRest {
				cell.rest = true
			} else {
				qty := parseFloat(r.Quantity)
				switch r.Type {
				case TypeDay:
					cell.days += qty
				case TypeHour:
					cell.hours += qty
				case TypePiece:
					cell.pieces += qty
				}
			}
			if r.Note != "" {
				cell.notes = append(cell.notes, r.Note)
			}
		}

		rows := [][]string{{"工人", "日期", "点工(天)", "点时(小时)", "计件(数量)", "休息", "金额(元)", "备注"}}
		for _, w := range workers {
			byDate := agg[w.ID]
			dates := make([]string, 0, len(byDate))
			for d := range byDate {
				dates = append(dates, d)
			}
			sort.Strings(dates)

			var totalDays, totalHours, totalPieces float64
			var totalRest int
			var totalAmount int64
			for _, d := range dates {
				cell := byDate[d]
				restMark := ""
				if cell.rest {
					restMark = "休"
					totalRest++
				}
				rows = append(rows, []string{
					w.Name, d, trimFloat(cell.days), trimFloat(cell.hours), trimFloat(cell.pieces),
					restMark, centsToYuanString(cell.amountCents), strings.Join(cell.notes, "；"),
				})
				totalDays += cell.days
				totalHours += cell.hours
				totalPieces += cell.pieces
				totalAmount += cell.amountCents
			}
			// 每人一行合计，没有记录的人也列出 0，考勤表才能覆盖全部在职工人。
			rows = append(rows, []string{
				w.Name, "合计", trimFloat(totalDays), trimFloat(totalHours),
				trimFloat(totalPieces), fmt.Sprintf("%d", totalRest),
				centsToYuanString(totalAmount), "",
			})
		}
		utils.WriteCSV(c, utils.ExportFilename(fmt.Sprintf("考勤表-%d-%02d", year, month), "csv"), rows)
	}
}

func exportAdvancesCSV(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		var items []Advance
		if err := db.Where("user_id = ?", uid).Order("date ASC, id ASC").Find(&items).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		fillAdvanceNames(db, items)
		rows := [][]string{{"日期", "工人", "金额(元)", "方式", "状态", "备注"}}
		for _, a := range items {
			status := "未结算"
			if a.SettlementID != nil {
				status = "已结算"
			}
			rows = append(rows, []string{a.Date, a.WorkerName, centsToYuanString(a.AmountCents), methodLabels[a.Method], status, a.Note})
		}
		utils.WriteCSV(c, utils.ExportFilename("借支记录", "csv"), rows)
	}
}

func exportSettlementsCSV(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		var items []Settlement
		if err := db.Where("user_id = ?", uid).Order("settled_at ASC, id ASC").Limit(20000).Find(&items).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		rows := [][]string{{"结算单号", "工人", "期间起", "期间止", "应发(元)", "借支(元)", "实发(元)", "记工条数", "结算时间", "备注"}}
		for _, s := range items {
			rows = append(rows, []string{
				fmt.Sprintf("JS%06d", s.ID), s.WorkerName, s.PeriodStart, s.PeriodEnd,
				centsToYuanString(s.WorkAmountCents), centsToYuanString(s.AdvanceAmountCents),
				centsToYuanString(s.PayableCents), fmt.Sprintf("%d", s.RecordCount),
				s.SettledAt.Format("2006-01-02 15:04"), s.Note,
			})
		}
		utils.WriteCSV(c, utils.ExportFilename("结算记录", "csv"), rows)
	}
}

// exportJSON 导出当前用户全部业务数据（JSON 包）。
func exportJSON(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		var workers, projects, pieces []interface{}
		var records, advances, settlements []interface{}
		db.Where("user_id = ?", uid).Find(&workers)
		db.Where("user_id = ?", uid).Find(&projects)
		db.Where("user_id = ?", uid).Find(&pieces)
		db.Where("user_id = ?", uid).Find(&records)
		db.Where("user_id = ?", uid).Find(&advances)
		db.Where("user_id = ?", uid).Find(&settlements)
		data := gin.H{
			"app":         "worklog",
			"exported_at": time.Now().Format(time.RFC3339),
			"workers":     workers,
			"projects":    projects,
			"piece_items": pieces,
			"records":     records,
			"advances":    advances,
			"settlements": settlements,
		}
		if err := utils.WriteJSONFile(c, utils.ExportFilename("工记数据备份", "json"), data); err != nil {
			response.ErrorInternal(c, "导出失败")
		}
	}
}

// exportDB 管理员下载 SQLite 数据库快照（VACUUM INTO 临时文件后流式下载）。
func exportDB(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil {
			response.ErrorInternal(c, "备份失败")
			return
		}
		tmpDir, err := os.MkdirTemp("", "worklog-backup-*")
		if err != nil {
			response.ErrorInternal(c, "备份失败")
			return
		}
		defer os.RemoveAll(tmpDir)
		backupPath := filepath.Join(tmpDir, fmt.Sprintf("worklog-backup-%s.db", time.Now().Format("20060102-150405")))
		if _, err := sqlDB.Exec("VACUUM INTO ?", backupPath); err != nil {
			response.ErrorInternal(c, "备份失败: "+err.Error())
			return
		}
		c.Header("Content-Type", "application/octet-stream")
		c.FileAttachment(backupPath, filepath.Base(backupPath))
	}
}
