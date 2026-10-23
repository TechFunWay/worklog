// attendance.go — 考勤的时间段查询与「一键对工」。
//
// 背景（2026-10-23 粉丝反馈）：考勤表导出是给组长和工人对工用的，只有月度日历
// 不够——组长要按工地切换着看，还要能自定义时间段查「考勤天数 / 计件个数」，
// 并把某个人某段时间的工账一键生成对工单发给工人核对。
//
// 本文件提供：
//   - projectFilter：本项目统一的工地筛选参数（全部 / 未分工地 / 指定工地），
//     月度考勤、时间段考勤、对工单与导出共用同一套解析口径；
//   - statsAttendanceRange：按人聚合的自定义时间段考勤；
//   - buildReconcile / statsReconcile：对工单数据（JSON 与 CSV 共用）。
package worklog

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/response"
	"smallgo/server/utils"
)

// projectFilter 工地筛选：未启用=全部工地；none=未分工地（project_id IS NULL）。
type projectFilter struct {
	enabled bool
	none    bool
	id      uint
}

// parseProjectFilter 解析 ?project_id=：空=全部；none=未分工地；正整数=指定工地。
// 返回 false 表示参数非法，由调用方回 400。
func parseProjectFilter(c *gin.Context) (projectFilter, bool) {
	raw := strings.TrimSpace(c.Query("project_id"))
	if raw == "" {
		return projectFilter{}, true
	}
	if strings.EqualFold(raw, "none") {
		return projectFilter{enabled: true, none: true}, true
	}
	id, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || id == 0 {
		return projectFilter{}, false
	}
	return projectFilter{enabled: true, id: uint(id)}, true
}

// apply 把工地筛选条件挂到查询上（列名固定为 project_id）。
func (f projectFilter) apply(q *gorm.DB) *gorm.DB {
	if !f.enabled {
		return q
	}
	if f.none {
		return q.Where("project_id IS NULL")
	}
	return q.Where("project_id = ?", f.id)
}

// echoID 返回给前端的 project_id 回显：指定工地时是 id，全部/未分工地时为 null。
func (f projectFilter) echoID() *uint {
	if !f.enabled || f.none {
		return nil
	}
	id := f.id
	return &id
}

// resolveProjectName 解析筛选工地的展示名，供对工单与导出使用。
// 指定工地不存在时报 errProjectNotFound，避免导出一张空白的对工单。
func (f projectFilter) resolveProjectName(db *gorm.DB, dsUID uint) (string, error) {
	if !f.enabled {
		return "全部工地", nil
	}
	if f.none {
		return "未分工地", nil
	}
	var p Project
	if err := db.Where("user_id = ? AND id = ?", dsUID, f.id).First(&p).Error; err != nil {
		return "", errProjectNotFound
	}
	return p.Name, nil
}

var (
	errWorkerNotFound  = errors.New("worker not found")
	errProjectNotFound = errors.New("project not found")
)

// maxAttendanceRangeDays 自定义时间段查询的最大跨度（含首尾 731 天）。
// 工地考勤一般按周/月看，跨两年已足够；上限防止误传 1970 年造成全表扫描。
const maxAttendanceRangeDays = 731

// parseDateRange 解析必填的 start/end：格式合法、起止顺序正确、跨度不超上限。
func parseDateRange(c *gin.Context) (string, string, bool) {
	start, end := c.Query("start"), c.Query("end")
	if !validDate(start) || !validDate(end) {
		return "", "", false
	}
	st, err1 := time.Parse("2006-01-02", start)
	et, err2 := time.Parse("2006-01-02", end)
	if err1 != nil || err2 != nil || st.After(et) {
		return "", "", false
	}
	if int(et.Sub(st).Hours()/24)+1 > maxAttendanceRangeDays {
		return "", "", false
	}
	return start, end, true
}

// attendanceRangeRow 时间段考勤的一行（按工人聚合）。
type attendanceRangeRow struct {
	WorkerID    uint    `json:"worker_id"`
	WorkerName  string  `json:"worker_name"`
	AttendDays  int     `json:"attend_days"` // 有非休息记录的自然日数
	Days        float64 `json:"days"`        // 点工数量合计（折合工数）
	Hours       float64 `json:"hours"`
	Pieces      float64 `json:"pieces"`
	RestCount   int     `json:"rest_count"` // 有休息记录的自然日数
	AmountCents int64   `json:"amount_cents"`
	RecordCount int64   `json:"record_count"`
}

// statsAttendanceRange 时间段考勤：按工人返回出勤天数、点工/计时/计件、休息与工钱。
//
// 与月度日历的区别：跨月、按人成表，零记录的在职工人也出行（组长能一眼看出谁这
// 段时间没记工）。员工角色强制只看自己绑定的工人档案。
func statsAttendanceRange(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		dsUID := ac.scopeUser()

		start, end, ok := parseDateRange(c)
		if !ok {
			response.ErrorBadRequest(c, "时间段不正确：请提供 start/end（YYYY-MM-DD），起止顺序正确且不超过 731 天")
			return
		}
		pf, ok := parseProjectFilter(c)
		if !ok {
			response.ErrorBadRequest(c, "project_id 参数不正确")
			return
		}
		workerID := utils.Atoi(c.Query("worker_id"), 0)
		if sw, scoped := ac.workerScope(); scoped {
			if ac.workerUnbound() {
				forbidden(c)
				return
			}
			workerID = int(sw)
		}

		// 每次返回新链，避免 Select/Scan 的残留子句污染下一条查询（statsRange 踩过）。
		buildRecordQ := func() *gorm.DB {
			q := db.Model(&WorkRecord{}).Where("user_id = ? AND date BETWEEN ? AND ?", dsUID, start, end)
			q = pf.apply(q)
			if workerID > 0 {
				q = q.Where("worker_id = ?", workerID)
			}
			return q
		}

		// 选定要出行的工人：指定工人 / 全部在职工人 + 时间段内有记录的工人。
		var workers []Worker
		var singleWorker *Worker
		if workerID > 0 {
			var w Worker
			if err := db.Where("user_id = ? AND id = ?", dsUID, workerID).First(&w).Error; err != nil {
				response.ErrorBadRequest(c, "工人不存在")
				return
			}
			singleWorker = &w
			workers = []Worker{w}
		} else {
			var touched []uint
			if err := buildRecordQ().Select("DISTINCT worker_id").Scan(&touched).Error; err != nil {
				response.ErrorInternal(c, "查询失败")
				return
			}
			wq := db.Where("user_id = ? AND status = ?", dsUID, "active")
			if len(touched) > 0 {
				wq = db.Where("user_id = ? AND (status = ? OR id IN ?)", dsUID, "active", touched)
			}
			if err := wq.Order("name ASC, id ASC").Find(&workers).Error; err != nil {
				response.ErrorInternal(c, "查询失败")
				return
			}
		}

		type typeSum struct {
			WorkerID uint
			Type     string
			Total    float64
			Amount   int64
			Cnt      int64
		}
		var sums []typeSum
		if err := buildRecordQ().
			Select("worker_id, type, COALESCE(SUM(CAST(quantity AS REAL)),0) AS total, COALESCE(SUM(amount_cents),0) AS amount, COUNT(*) AS cnt").
			Group("worker_id, type").Scan(&sums).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}

		type dayCount struct {
			WorkerID uint
			N        int
		}
		var attendRows, restRows []dayCount
		if err := buildRecordQ().Where("type <> ?", TypeRest).
			Select("worker_id, COUNT(DISTINCT date) AS n").Group("worker_id").Scan(&attendRows).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		if err := buildRecordQ().Where("type = ?", TypeRest).
			Select("worker_id, COUNT(DISTINCT date) AS n").Group("worker_id").Scan(&restRows).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}

		byWorker := make(map[uint]*attendanceRangeRow, len(workers))
		for _, w := range workers {
			byWorker[w.ID] = &attendanceRangeRow{WorkerID: w.ID, WorkerName: w.Name}
		}
		for _, s := range sums {
			row := byWorker[s.WorkerID]
			if row == nil {
				continue
			}
			switch s.Type {
			case TypeDay:
				row.Days += s.Total
			case TypeHour:
				row.Hours += s.Total
			case TypePiece:
				row.Pieces += s.Total
			}
			row.AmountCents += s.Amount
			row.RecordCount += s.Cnt
		}
		for _, d := range attendRows {
			if row := byWorker[d.WorkerID]; row != nil {
				row.AttendDays = d.N
			}
		}
		for _, d := range restRows {
			if row := byWorker[d.WorkerID]; row != nil {
				row.RestCount = d.N
			}
		}

		rows := make([]attendanceRangeRow, 0, len(workers))
		total := attendanceRangeRow{WorkerName: "合计"}
		for _, w := range workers {
			row := *byWorker[w.ID]
			rows = append(rows, row)
			total.AttendDays += row.AttendDays
			total.Days += row.Days
			total.Hours += row.Hours
			total.Pieces += row.Pieces
			total.RestCount += row.RestCount
			total.AmountCents += row.AmountCents
			total.RecordCount += row.RecordCount
		}
		// 指定单个工人时也按姓名排序（单行无影响）；全部工人按姓名/档案号稳定排序。
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].WorkerName == rows[j].WorkerName {
				return rows[i].WorkerID < rows[j].WorkerID
			}
			return rows[i].WorkerName < rows[j].WorkerName
		})

		var workerOut interface{}
		if singleWorker != nil {
			workerOut = *singleWorker
		}
		response.Success(c, gin.H{
			"start":      start,
			"end":        end,
			"project_id": pf.echoID(),
			"worker":     workerOut,
			"rows":       rows,
			"total":      total,
		})
	}
}

// reconcileItem 对工单里的一条记工明细。
type reconcileItem struct {
	ID             uint   `json:"id"`
	Date           string `json:"date"`
	Type           string `json:"type"`
	TypeLabel      string `json:"type_label"`
	Quantity       string `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	AmountCents    int64  `json:"amount_cents"`
	ProjectName    string `json:"project_name"`
	PieceName      string `json:"piece_name"`
	PieceUnit      string `json:"piece_unit"`
	Note           string `json:"note"`
	Rest           bool   `json:"rest"`
}

// reconcileResult 对工单数据：工人 + 期间 + 工地 + 合计 + 逐条明细。
// JSON 接口与 CSV 导出共用同一份计算结果，保证两处数字一致。
type reconcileResult struct {
	Worker      Worker          `json:"worker"`
	Start       string          `json:"start"`
	End         string          `json:"end"`
	ProjectID   *uint           `json:"project_id"`
	ProjectName string          `json:"project_name"`
	AttendDays  int             `json:"attend_days"`
	Days        float64         `json:"days"`
	Hours       float64         `json:"hours"`
	Pieces      float64         `json:"pieces"`
	RestCount   int             `json:"rest_count"`
	AmountCents int64           `json:"amount_cents"`
	RecordCount int             `json:"record_count"`
	Items       []reconcileItem `json:"items"`
	GeneratedAt string          `json:"generated_at"`
}

// buildReconcile 计算某个工人在指定期间/工地下的对工单。
// 调用方负责角色与数据归属校验（dsUID 必须是该工人所属数据集）。
func buildReconcile(db *gorm.DB, dsUID, workerID uint, start, end string, pf projectFilter) (*reconcileResult, error) {
	var w Worker
	if err := db.Where("user_id = ? AND id = ?", dsUID, workerID).First(&w).Error; err != nil {
		return nil, errWorkerNotFound
	}
	projectName, err := pf.resolveProjectName(db, dsUID)
	if err != nil {
		return nil, err
	}

	q := db.Where("user_id = ? AND worker_id = ? AND date BETWEEN ? AND ?", dsUID, workerID, start, end)
	q = pf.apply(q)
	var records []WorkRecord
	if err := q.Order("date ASC, id ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	fillRecordNames(db, records)

	res := &reconcileResult{
		Worker:      w,
		Start:       start,
		End:         end,
		ProjectID:   pf.echoID(),
		ProjectName: projectName,
		Items:       make([]reconcileItem, 0, len(records)),
		GeneratedAt: time.Now().Format("2006-01-02 15:04"),
	}
	attendDates := map[string]bool{}
	restDates := map[string]bool{}
	for _, r := range records {
		label := typeLabels[r.Type]
		rest := r.Type == TypeRest
		if rest {
			label = "休息"
		}
		res.Items = append(res.Items, reconcileItem{
			ID:             r.ID,
			Date:           r.Date,
			Type:           r.Type,
			TypeLabel:      label,
			Quantity:       r.Quantity,
			UnitPriceCents: r.UnitPriceCents,
			AmountCents:    r.AmountCents,
			ProjectName:    r.ProjectName,
			PieceName:      r.PieceName,
			PieceUnit:      r.PieceUnit,
			Note:           r.Note,
			Rest:           rest,
		})
		res.AmountCents += r.AmountCents
		res.RecordCount++
		if rest {
			restDates[r.Date] = true
			continue
		}
		attendDates[r.Date] = true
		qty := parseFloat(r.Quantity)
		switch r.Type {
		case TypeDay:
			res.Days += qty
		case TypeHour:
			res.Hours += qty
		case TypePiece:
			res.Pieces += qty
		}
	}
	res.AttendDays = len(attendDates)
	res.RestCount = len(restDates)
	return res, nil
}

// statsReconcile 一键对工：某个工人某段时间（可按工地）的工账明细与合计。
func statsReconcile(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		dsUID := ac.scopeUser()

		start, end, ok := parseDateRange(c)
		if !ok {
			response.ErrorBadRequest(c, "时间段不正确：请提供 start/end（YYYY-MM-DD），起止顺序正确且不超过 731 天")
			return
		}
		pf, ok := parseProjectFilter(c)
		if !ok {
			response.ErrorBadRequest(c, "project_id 参数不正确")
			return
		}
		workerID := utils.Atoi(c.Query("worker_id"), 0)
		if sw, scoped := ac.workerScope(); scoped {
			if ac.workerUnbound() {
				forbidden(c)
				return
			}
			workerID = int(sw)
		}
		if workerID <= 0 {
			response.ErrorBadRequest(c, "请选择要核对的工人")
			return
		}
		res, err := buildReconcile(db, dsUID, uint(workerID), start, end, pf)
		if err == errWorkerNotFound {
			response.ErrorBadRequest(c, "工人不存在")
			return
		}
		if err == errProjectNotFound {
			response.ErrorBadRequest(c, "工地不存在")
			return
		}
		if err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		response.Success(c, res)
	}
}
