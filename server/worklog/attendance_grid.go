// attendance_grid.go — 考勤网格数据（考勤表按日期展开）。
//
// 背景（2026-10-23 粉丝反馈）：考勤表导出要能生成 Excel 与图片并保存到 NAS。
// 前端渲染图片、后端生成 xlsx 都需要「每个工人每天」的完整网格，而不是只有
// 汇总行；本文件把时间段内的记工记录聚合成 dates × workers 的稀疏网格，
// 与 CSV 导出口径保持一致（网格空位=当天没有记录）。
package worklog

import (
	"errors"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/response"
)

// attendanceGridTotals 汇总口径，与时间段考勤（attendanceRangeRow）一致；
// 后三个字段给工资条分项金额用，不参与接口契约的必填项。
type attendanceGridTotals struct {
	AttendDays       int     `json:"attend_days"`
	Days             float64 `json:"days"`
	Hours            float64 `json:"hours"`
	Pieces           float64 `json:"pieces"`
	RestCount        int     `json:"rest_count"`
	AmountCents      int64   `json:"amount_cents"`
	RecordCount      int     `json:"record_count"`
	DayAmountCents   int64   `json:"day_amount_cents,omitempty"`
	HourAmountCents  int64   `json:"hour_amount_cents,omitempty"`
	PieceAmountCents int64   `json:"piece_amount_cents,omitempty"`
}

func (t *attendanceGridTotals) add(o attendanceGridTotals) {
	t.AttendDays += o.AttendDays
	t.Days += o.Days
	t.Hours += o.Hours
	t.Pieces += o.Pieces
	t.RestCount += o.RestCount
	t.AmountCents += o.AmountCents
	t.RecordCount += o.RecordCount
	t.DayAmountCents += o.DayAmountCents
	t.HourAmountCents += o.HourAmountCents
	t.PieceAmountCents += o.PieceAmountCents
}

// attendanceGridCell 某工人某天的考勤聚合（同一天多条记录合并）。
type attendanceGridCell struct {
	Date        string   `json:"date"`
	Days        float64  `json:"days"`
	Hours       float64  `json:"hours"`
	Pieces      float64  `json:"pieces"`
	Rest        bool     `json:"rest"`
	AmountCents int64    `json:"amount_cents"`
	RecordCount int      `json:"record_count"`
	Notes       []string `json:"notes,omitempty"`
}

type attendanceGridWorker struct {
	WorkerID   uint                 `json:"worker_id"`
	WorkerName string               `json:"worker_name"`
	Cells      []attendanceGridCell `json:"cells"`
	Totals     attendanceGridTotals `json:"totals"`
}

// attendanceGridData 考勤网格接口返回体（前端渲染图片用）。
type attendanceGridData struct {
	Mode        string                 `json:"mode"`
	Start       string                 `json:"start"`
	End         string                 `json:"end"`
	Year        *int                   `json:"year,omitempty"`
	Month       *int                   `json:"month,omitempty"`
	ProjectID   *uint                  `json:"project_id"`
	ProjectName string                 `json:"project_name"`
	Dates       []string               `json:"dates"`
	Workers     []attendanceGridWorker `json:"workers"`
	Total       attendanceGridTotals   `json:"total"`
}

// eachDateInRange 展开 start~end 的每一天（调用方已保证跨度 ≤ 731 天）。
func eachDateInRange(start, end string) []string {
	st, err1 := time.Parse("2006-01-02", start)
	et, err2 := time.Parse("2006-01-02", end)
	if err1 != nil || err2 != nil || st.After(et) {
		return nil
	}
	dates := make([]string, 0, int(et.Sub(st).Hours()/24)+1)
	for d := st; !d.After(et); d = d.AddDate(0, 0, 1) {
		dates = append(dates, d.Format("2006-01-02"))
	}
	return dates
}

// attendanceGridAgg 中间聚合结构。
type attendanceGridAgg struct {
	days, hours, pieces float64
	rest                bool
	attend              bool
	amountCents         int64
	dayAmountCents      int64
	hourAmountCents     int64
	pieceAmountCents    int64
	recordCount         int
	notes               []string
}

// buildAttendanceGrid 按导出参数构建考勤网格。
//
// 行顺序与 CSV 导出一致：在职工人按姓名排序，时间段内有记录的已归档工人
// 也补进来（否则「上月记了工、这月归档」的人会从考勤表消失）。
func buildAttendanceGrid(db *gorm.DB, spec *attendanceExportSpec) (*attendanceGridData, error) {
	var workers []Worker
	wq := db.Where("user_id = ?", spec.dsUID)
	if spec.workerID > 0 {
		wq = wq.Where("id = ?", spec.workerID)
	} else {
		wq = wq.Where("status = ?", "active")
	}
	if err := wq.Order("name ASC, id ASC").Find(&workers).Error; err != nil {
		return nil, err
	}
	if spec.workerID > 0 && len(workers) == 0 {
		return nil, errWorkerNotFound
	}

	buildRecordQ := func() *gorm.DB {
		q := db.Model(&WorkRecord{}).Where("user_id = ? AND date BETWEEN ? AND ?", spec.dsUID, spec.start, spec.end)
		q = spec.pf.apply(q)
		if spec.workerID > 0 {
			q = q.Where("worker_id = ?", spec.workerID)
		}
		return q
	}

	var records []WorkRecord
	if err := buildRecordQ().Order("worker_id ASC, date ASC, id ASC").Find(&records).Error; err != nil {
		return nil, err
	}

	// 补上时间段内有记录、但已不在在职工人列表里的工人。
	if spec.workerID == 0 {
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
			if err := db.Where("user_id = ? AND id IN ?", spec.dsUID, extraIDs).Find(&extra).Error; err != nil {
				return nil, err
			}
			workers = append(workers, extra...)
			sort.SliceStable(workers, func(i, j int) bool {
				if workers[i].Name == workers[j].Name {
					return workers[i].ID < workers[j].ID
				}
				return workers[i].Name < workers[j].Name
			})
		}
	}

	// 按工地筛选时只保留该工地有记录的工人：整班人马都挂在某个工地的考勤表上
	// 会误导对工（「全部工地」不筛选，仍保留在职工人，方便看谁整月没出工）。
	if spec.pf.enabled && spec.workerID == 0 {
		seen := make(map[uint]bool, len(records))
		for _, r := range records {
			seen[r.WorkerID] = true
		}
		kept := workers[:0]
		for _, w := range workers {
			if seen[w.ID] {
				kept = append(kept, w)
			}
		}
		workers = kept
	}

	agg := make(map[uint]map[string]*attendanceGridAgg, len(workers))
	for _, r := range records {
		byDate := agg[r.WorkerID]
		if byDate == nil {
			byDate = make(map[string]*attendanceGridAgg)
			agg[r.WorkerID] = byDate
		}
		cell := byDate[r.Date]
		if cell == nil {
			cell = &attendanceGridAgg{}
			byDate[r.Date] = cell
		}
		cell.recordCount++
		cell.amountCents += r.AmountCents
		if r.Note != "" {
			cell.notes = append(cell.notes, r.Note)
		}
		if r.Type == TypeRest {
			cell.rest = true
			continue
		}
		cell.attend = true
		qty := parseFloat(r.Quantity)
		switch r.Type {
		case TypeDay:
			cell.days += qty
			cell.dayAmountCents += r.AmountCents
		case TypeHour:
			cell.hours += qty
			cell.hourAmountCents += r.AmountCents
		case TypePiece:
			cell.pieces += qty
			cell.pieceAmountCents += r.AmountCents
		}
	}

	out := &attendanceGridData{
		Mode:        "range",
		Start:       spec.start,
		End:         spec.end,
		ProjectID:   spec.pf.echoID(),
		ProjectName: spec.projectName,
		Dates:       eachDateInRange(spec.start, spec.end),
		Workers:     make([]attendanceGridWorker, 0, len(workers)),
	}
	if !spec.rangeMode {
		y, m := spec.year, spec.month
		out.Mode = "month"
		out.Year = &y
		out.Month = &m
	}

	for _, w := range workers {
		byDate := agg[w.ID]
		dates := make([]string, 0, len(byDate))
		for d := range byDate {
			dates = append(dates, d)
		}
		sort.Strings(dates)

		row := attendanceGridWorker{
			WorkerID:   w.ID,
			WorkerName: w.Name,
			Cells:      make([]attendanceGridCell, 0, len(dates)),
		}
		for _, d := range dates {
			cell := byDate[d]
			row.Cells = append(row.Cells, attendanceGridCell{
				Date:        d,
				Days:        cell.days,
				Hours:       cell.hours,
				Pieces:      cell.pieces,
				Rest:        cell.rest,
				AmountCents: cell.amountCents,
				RecordCount: cell.recordCount,
				Notes:       cell.notes,
			})
			if cell.attend {
				row.Totals.AttendDays++
			}
			if cell.rest {
				row.Totals.RestCount++
			}
			row.Totals.Days += cell.days
			row.Totals.Hours += cell.hours
			row.Totals.Pieces += cell.pieces
			row.Totals.AmountCents += cell.amountCents
			row.Totals.RecordCount += cell.recordCount
			row.Totals.DayAmountCents += cell.dayAmountCents
			row.Totals.HourAmountCents += cell.hourAmountCents
			row.Totals.PieceAmountCents += cell.pieceAmountCents
		}
		out.Workers = append(out.Workers, row)
		out.Total.add(row.Totals)
	}
	return out, nil
}

// statsAttendanceGrid GET /attendance/grid。
func statsAttendanceGrid(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		spec, code, msg := parseAttendanceExportSpec(c, db)
		if spec == nil {
			writeSpecError(c, code, msg)
			return
		}
		data, err := buildAttendanceGrid(db, spec)
		if err != nil {
			if errors.Is(err, errWorkerNotFound) {
				response.ErrorBadRequest(c, "工人不存在")
				return
			}
			response.ErrorInternal(c, "查询失败")
			return
		}
		response.Success(c, data)
	}
}
