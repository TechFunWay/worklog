// project_prices.go — 工地单价与「工人在某工地的单独单价」。
//
// 粉丝反馈（2026-10-23 第 2 轮）：项目可切换后要能一键记工，但不同工地工程的
// 价格不一样——工地需要能设默认单价，个别人在个别工地还要单独定价。
//
// 价格优先级（与前端契约一致，见 worklog/tmp/feedback-2026-10-23-project-prices-contract.md）：
//  1. 手动填写 unit_price_cents；
//  2. 工人在该工地的单独单价（project_worker_prices，>0 才生效）；
//  3. 工地默认单价（projects.day_price_cents / hour_price_cents，>0 才生效）；
//  4. 工人自身单价（月薪工点工按记工日期所在月天数折算，否则日薪/时薪）；
//  5. 计件仍按计件项目单价（本次不加工地计件价，手动填写优先）。
package worklog

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/response"
	"smallgo/server/utils"
)

// 价格来源（返回给前端显示「这个价从哪来」）。
const (
	priceSourceProjectWorker = "project_worker"
	priceSourceProject       = "project"
	priceSourceMonthly       = "monthly"
	priceSourceWorker        = "worker"
	priceSourceNone          = "none"
)

// rateInfo 某个工人某天的生效单价与来源。
type rateInfo struct {
	Daily        int64
	DailySource  string
	Hourly       int64
	HourlySource string
}

// findProjectWorkerPrice 查某工人某工地的单独单价，没有返回 nil。
func findProjectWorkerPrice(db *gorm.DB, uid, projectID, workerID uint) *ProjectWorkerPrice {
	var row ProjectWorkerPrice
	if err := db.Where("user_id = ? AND project_id = ? AND worker_id = ?", uid, projectID, workerID).
		First(&row).Error; err != nil {
		return nil
	}
	return &row
}

// effectiveRatesWith 按优先级算出生效单价；override 由调用方预先查好（避免列表 N+1）。
// project 为 nil 表示这笔记工不分工地。
func effectiveRatesWith(w Worker, project *Project, override *ProjectWorkerPrice, date string) rateInfo {
	res := rateInfo{DailySource: priceSourceNone, HourlySource: priceSourceNone}

	// 点工日价
	switch {
	case override != nil && override.DailyWageCents > 0:
		res.Daily, res.DailySource = override.DailyWageCents, priceSourceProjectWorker
	case project != nil && project.DayPriceCents > 0:
		res.Daily, res.DailySource = project.DayPriceCents, priceSourceProject
	case w.MonthlyWageCents > 0:
		res.Daily, res.DailySource = dailyFromMonthly(w.MonthlyWageCents, date), priceSourceMonthly
	case w.DailyWageCents > 0:
		res.Daily, res.DailySource = w.DailyWageCents, priceSourceWorker
	}

	// 点时小时价
	switch {
	case override != nil && override.HourlyWageCents > 0:
		res.Hourly, res.HourlySource = override.HourlyWageCents, priceSourceProjectWorker
	case project != nil && project.HourPriceCents > 0:
		res.Hourly, res.HourlySource = project.HourPriceCents, priceSourceProject
	case w.HourlyWageCents > 0:
		res.Hourly, res.HourlySource = w.HourlyWageCents, priceSourceWorker
	}
	return res
}

// effectiveRates 查一次覆盖价后算生效单价（记工校验与生效价接口共用）。
func effectiveRates(db *gorm.DB, uid uint, w Worker, project *Project, date string) rateInfo {
	var override *ProjectWorkerPrice
	if project != nil {
		override = findProjectWorkerPrice(db, uid, project.ID, w.ID)
	}
	return effectiveRatesWith(w, project, override, date)
}

// projectPriceRow 工地单价管理页的一行：覆盖价 + 当前生效价 + 来源。
type projectPriceRow struct {
	WorkerID                uint   `json:"worker_id"`
	WorkerName              string `json:"worker_name"`
	Status                  string `json:"status"`
	WorkerDailyWageCents    int64  `json:"worker_daily_wage_cents"`
	WorkerHourlyWageCents   int64  `json:"worker_hourly_wage_cents"`
	MonthlyWageCents        int64  `json:"monthly_wage_cents"`
	DailyWageCents          int64  `json:"daily_wage_cents"`
	HourlyWageCents         int64  `json:"hourly_wage_cents"`
	EffectiveDailyWageCents int64  `json:"effective_daily_wage_cents"`
	EffectiveHourlyCents    int64  `json:"effective_hourly_wage_cents"`
	DailySource             string `json:"daily_source"`
	HourlySource            string `json:"hourly_source"`
}

// projectPrices 返回某工地的默认单价与该工地所有工人的覆盖价/生效价。
func projectPrices(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !requireManage(ac) {
			forbidden(c)
			return
		}
		dsUID := ac.scopeUser()
		var project Project
		if err := db.Where("user_id = ? AND id = ?", dsUID, c.Param("id")).First(&project).Error; err != nil {
			notFound(c, "工地不存在")
			return
		}

		var overrides []ProjectWorkerPrice
		if err := db.Where("user_id = ? AND project_id = ?", dsUID, project.ID).Find(&overrides).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		overrideByWorker := make(map[uint]*ProjectWorkerPrice, len(overrides))
		orderedIDs := make([]uint, 0, len(overrides))
		for i := range overrides {
			overrideByWorker[overrides[i].WorkerID] = &overrides[i]
			orderedIDs = append(orderedIDs, overrides[i].WorkerID)
		}

		// 在职工人 + 有覆盖价的工人（含已归档，便于查看/清除历史单价）。
		wq := db.Where("user_id = ? AND status = ?", dsUID, "active")
		if len(orderedIDs) > 0 {
			wq = db.Where("user_id = ? AND (status = ? OR id IN ?)", dsUID, "active", orderedIDs)
		}
		var workers []Worker
		if err := wq.Order("name ASC, id ASC").Find(&workers).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}

		today := time.Now().Format("2006-01-02")
		rows := make([]projectPriceRow, 0, len(workers))
		for _, w := range workers {
			override := overrideByWorker[w.ID]
			rates := effectiveRatesWith(w, &project, override, today)
			row := projectPriceRow{
				WorkerID:                w.ID,
				WorkerName:              w.Name,
				Status:                  w.Status,
				WorkerDailyWageCents:    w.DailyWageCents,
				WorkerHourlyWageCents:   w.HourlyWageCents,
				MonthlyWageCents:        w.MonthlyWageCents,
				EffectiveDailyWageCents: rates.Daily,
				EffectiveHourlyCents:    rates.Hourly,
				DailySource:             rates.DailySource,
				HourlySource:            rates.HourlySource,
			}
			if override != nil {
				row.DailyWageCents = override.DailyWageCents
				row.HourlyWageCents = override.HourlyWageCents
			}
			rows = append(rows, row)
		}

		response.Success(c, gin.H{
			"project":                    project,
			"project_daily_price_cents":  project.DayPriceCents,
			"project_hourly_price_cents": project.HourPriceCents,
			"rows":                       rows,
		})
	}
}

// updateProjectPrices 批量保存某工地的工人单独单价；两个单价都传 0 表示删除覆盖。
func updateProjectPrices(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !requireManage(ac) {
			forbidden(c)
			return
		}
		dsUID := ac.scopeUser()
		var project Project
		if err := db.Where("user_id = ? AND id = ?", dsUID, c.Param("id")).First(&project).Error; err != nil {
			notFound(c, "工地不存在")
			return
		}

		var in struct {
			Rows []struct {
				WorkerID        uint  `json:"worker_id"`
				DailyWageCents  int64 `json:"daily_wage_cents"`
				HourlyWageCents int64 `json:"hourly_wage_cents"`
			} `json:"rows"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}

		seen := make(map[uint]bool, len(in.Rows))
		for _, r := range in.Rows {
			if r.WorkerID == 0 {
				response.ErrorBadRequest(c, "参数错误：缺少 worker_id")
				return
			}
			if seen[r.WorkerID] {
				response.ErrorBadRequest(c, "参数错误：同一工人重复提交")
				return
			}
			seen[r.WorkerID] = true
			if r.DailyWageCents < 0 || r.HourlyWageCents < 0 {
				response.ErrorBadRequest(c, "单价不能为负数")
				return
			}
			var n int64
			if err := db.Model(&Worker{}).Where("user_id = ? AND id = ?", dsUID, r.WorkerID).Count(&n).Error; err != nil {
				response.ErrorInternal(c, "保存失败")
				return
			}
			if n == 0 {
				response.ErrorBadRequest(c, "工人不存在")
				return
			}
		}

		err := db.Transaction(func(tx *gorm.DB) error {
			for _, r := range in.Rows {
				q := tx.Where("user_id = ? AND project_id = ? AND worker_id = ?", dsUID, project.ID, r.WorkerID)
				if r.DailyWageCents == 0 && r.HourlyWageCents == 0 {
					if err := q.Delete(&ProjectWorkerPrice{}).Error; err != nil {
						return err
					}
					continue
				}
				var row ProjectWorkerPrice
				if err := q.First(&row).Error; err != nil {
					if err != gorm.ErrRecordNotFound {
						return err
					}
					row = ProjectWorkerPrice{UserID: dsUID, ProjectID: project.ID, WorkerID: r.WorkerID}
				}
				row.DailyWageCents = r.DailyWageCents
				row.HourlyWageCents = r.HourlyWageCents
				if err := tx.Save(&row).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}

// effectivePrice 记工弹窗自动带价用：某工人某天在某工地的生效日价/时价与来源。
// 不传 project_id 表示不分工地；不传 date 用今天（月薪折算靠记工日期定当月天数）。
func effectivePrice(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		dsUID := ac.scopeUser()

		workerID := utils.Atoi(c.Query("worker_id"), 0)
		if sw, ok := ac.workerScope(); ok {
			if ac.workerUnbound() {
				forbidden(c)
				return
			}
			workerID = int(sw)
		}
		if workerID <= 0 {
			response.ErrorBadRequest(c, "请指定工人")
			return
		}
		var worker Worker
		if err := db.Where("user_id = ? AND id = ?", dsUID, workerID).First(&worker).Error; err != nil {
			response.ErrorBadRequest(c, "工人不存在")
			return
		}

		var project *Project
		if raw := strings.TrimSpace(c.Query("project_id")); raw != "" && !strings.EqualFold(raw, "none") {
			id, err := strconv.ParseUint(raw, 10, 32)
			if err != nil || id == 0 {
				response.ErrorBadRequest(c, "project_id 参数不正确")
				return
			}
			var p Project
			if err := db.Where("user_id = ? AND id = ?", dsUID, id).First(&p).Error; err != nil {
				response.ErrorBadRequest(c, "工地不存在")
				return
			}
			project = &p
		}

		date := strings.TrimSpace(c.Query("date"))
		if date == "" {
			date = time.Now().Format("2006-01-02")
		} else if !validDate(date) {
			response.ErrorBadRequest(c, "日期格式应为 YYYY-MM-DD")
			return
		}
		rates := effectiveRates(db, dsUID, worker, project, date)

		out := gin.H{
			"worker_id":                  worker.ID,
			"project_id":                 nil,
			"date":                       date,
			"daily_wage_cents":           rates.Daily,
			"daily_source":               rates.DailySource,
			"hourly_wage_cents":          rates.Hourly,
			"hourly_source":              rates.HourlySource,
			"monthly_wage_cents":         worker.MonthlyWageCents,
			"worker_daily_wage_cents":    worker.DailyWageCents,
			"worker_hourly_wage_cents":   worker.HourlyWageCents,
			"project_daily_price_cents":  int64(0),
			"project_hourly_price_cents": int64(0),
		}
		if project != nil {
			out["project_id"] = project.ID
			out["project_daily_price_cents"] = project.DayPriceCents
			out["project_hourly_price_cents"] = project.HourPriceCents
		}
		response.Success(c, out)
	}
}
