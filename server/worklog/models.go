// Package worklog 记工记账业务包：按天/按时/计件记工、借支、结算与工资条、统计与导出。
package worklog

import (
	"gorm.io/gorm"
	"smallgo/server/apps"
	"smallgo/server/database"
	"time"
)

// Worker 工人档案。日薪/时薪为记工录入时的默认单价；月薪工只填月薪，
// 记工时按当月实际天数自动折算日薪（缺勤不记工即自然扣款）。
type Worker struct {
	ID               uint      `gorm:"primarykey" json:"id"`
	UserID           uint      `gorm:"index;not null" json:"-"`
	Name             string    `gorm:"not null" json:"name"`
	Phone            string    `json:"phone"`
	Note             string    `json:"note"`
	DailyWageCents   int64     `json:"daily_wage_cents"`
	HourlyWageCents  int64     `json:"hourly_wage_cents"`
	MonthlyWageCents int64     `json:"monthly_wage_cents"`
	Status           string    `gorm:"default:active;index" json:"status"` // active / archived
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Project 工地/项目（选填维度）。DayPriceCents/HourPriceCents 是工地默认单价
// （0=未设置，记工时回落到工人自身单价）；「某工人某工地单独定价」见
// ProjectWorkerPrice，优先级高于工地默认单价。
type Project struct {
	ID             uint      `gorm:"primarykey" json:"id"`
	UserID         uint      `gorm:"index;not null" json:"-"`
	Name           string    `gorm:"not null" json:"name"`
	Note           string    `json:"note"`
	Status         string    `gorm:"default:active;index" json:"status"`
	DayPriceCents  int64     `json:"day_price_cents"`  // 工地默认日工单价（分/个工）
	HourPriceCents int64     `json:"hour_price_cents"` // 工地默认小时单价（分/小时）
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ProjectWorkerPrice 工人在某工地的单独定价：覆盖工地默认单价与工人自身单价。
// 同一 (project_id, worker_id) 只有一行；两个单价都为 0 时该行删除（视为无覆盖）。
type ProjectWorkerPrice struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	UserID          uint      `gorm:"index;not null" json:"-"`
	ProjectID       uint      `gorm:"not null;uniqueIndex:idx_project_worker" json:"project_id"`
	WorkerID        uint      `gorm:"not null;uniqueIndex:idx_project_worker" json:"worker_id"`
	DailyWageCents  int64     `json:"daily_wage_cents"`
	HourlyWageCents int64     `json:"hourly_wage_cents"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// PieceItem 计件项目：名称 + 计量单位 + 默认单价。
type PieceItem struct {
	ID             uint      `gorm:"primarykey" json:"id"`
	UserID         uint      `gorm:"index;not null" json:"-"`
	Name           string    `gorm:"not null" json:"name"`
	Unit           string    `json:"unit"`
	UnitPriceCents int64     `json:"unit_price_cents"`
	Note           string    `json:"note"`
	Status         string    `gorm:"default:active;index" json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// 记工状态：员工提交后待班组长审核，只有 approved 计入工资/统计/结算。
// 历史数据与老板/组长直接录入的记录都是 approved。
const (
	RecordStatusApproved = "approved"
	RecordStatusPending  = "pending"
	RecordStatusRejected = "rejected"

	RecordSourceManual = "manual" // 应用内手工记工
	RecordSourceQR     = "qr"     // 扫报工码提交
	RecordSourceBatch  = "batch"  // 班组长批量报工
)

// WorkRecord 记工记录。Type: day(点工)/hour(点时)/piece(计件)。
// 数量存 decimal 文本，金额以整数分存储。SettlementID 非空表示已结算锁定。
type WorkRecord struct {
	ID             uint   `gorm:"primarykey" json:"id"`
	UserID         uint   `gorm:"index;not null" json:"-"`
	WorkerID       uint   `gorm:"index;not null" json:"worker_id"`
	ProjectID      *uint  `json:"project_id"`
	PieceItemID    *uint  `json:"piece_item_id"`
	Date           string `gorm:"index;not null" json:"date"` // YYYY-MM-DD
	Type           string `gorm:"index;not null" json:"type"`
	Quantity       string `gorm:"not null" json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	AmountCents    int64  `json:"amount_cents"`
	Note           string `json:"note"`
	SettlementID   *uint  `gorm:"index" json:"settlement_id"`
	CreatedBy      uint   `json:"created_by"` // 录入人账号（班组内区分老板/组长代记；扫码报工为 0）
	// 审核流：默认 approved 兼容历史数据；员工/扫码提交先落 pending。
	Status        string     `gorm:"default:approved;index" json:"status"`
	Source        string     `gorm:"default:manual" json:"source"` // manual / qr / batch
	EntryCodeID   *uint      `gorm:"index" json:"entry_code_id"`
	EntryCodeName string     `json:"entry_code_name"` // 报工码名称快照（码删除后仍可追溯）
	SubmitterName string     `json:"submitter_name"`  // 扫码/员工提交人姓名快照
	ReviewedBy    *uint      `json:"reviewed_by"`
	ReviewedAt    *time.Time `json:"reviewed_at"`
	ReviewNote    string     `json:"review_note"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// 展示字段（联查填充）
	WorkerName   string `gorm:"-" json:"worker_name"`
	ProjectName  string `gorm:"-" json:"project_name"`
	PieceName    string `gorm:"-" json:"piece_name"`
	PieceUnit    string `gorm:"-" json:"piece_unit"`
	ReviewerName string `gorm:"-" json:"reviewer_name"`
}

// EntryCode 扫码报工码：老板/组长生成，工人扫码免登录提交。
// 快捷形态：
//   - 只绑产品：扫产品码 → 选姓名 → 填数量；
//   - 只绑工人：扫个人码 → 选产品 → 填数量；
//   - 产品 + 工人都绑：扫码后只剩数量一个输入框（可印成每人每产品一张）。
type EntryCode struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	UserID      uint      `gorm:"index;not null" json:"-"` // 数据集归属（班组老板）
	Name        string    `gorm:"not null" json:"name"`
	Token       string    `gorm:"uniqueIndex;not null" json:"token"`
	Type        string    `gorm:"default:piece" json:"type"` // 目前固定计件
	PieceItemID *uint     `gorm:"index" json:"piece_item_id"`
	WorkerID    *uint     `gorm:"index" json:"worker_id"`
	ProjectID   *uint     `json:"project_id"`
	BaseURL     string    `json:"base_url"` // 报工地址前缀（内网/公网直连），空=按当前访问地址
	AutoApprove bool      `json:"auto_approve"`
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	ExpiresAt   *string   `json:"expires_at"` // YYYY-MM-DD，空=长期有效
	Note        string    `json:"note"`
	CreatedBy   uint      `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// 展示字段
	PieceName    string `gorm:"-" json:"piece_name"`
	PieceUnit    string `gorm:"-" json:"piece_unit"`
	WorkerName   string `gorm:"-" json:"worker_name"`
	ProjectName  string `gorm:"-" json:"project_name"`
	TodayCount   int64  `gorm:"-" json:"today_count"`
	PendingCount int64  `gorm:"-" json:"pending_count"`
	TotalCount   int64  `gorm:"-" json:"total_count"`
}

// Advance 借支记录。
type Advance struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	UserID       uint      `gorm:"index;not null" json:"-"`
	WorkerID     uint      `gorm:"index;not null" json:"worker_id"`
	Date         string    `gorm:"index;not null" json:"date"`
	AmountCents  int64     `json:"amount_cents"`
	Method       string    `json:"method"` // cash / wechat / alipay / other
	Note         string    `json:"note"`
	SettlementID *uint     `gorm:"index" json:"settlement_id"`
	CreatedBy    uint      `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`

	WorkerName string `gorm:"-" json:"worker_name"`
}

// Settlement 结算单（工资条）。冗余工人姓名与期间汇总，Details 存明细 JSON 快照。
type Settlement struct {
	ID                 uint      `gorm:"primarykey" json:"id"`
	UserID             uint      `gorm:"index;not null" json:"-"`
	WorkerID           uint      `gorm:"index;not null" json:"worker_id"`
	WorkerName         string    `json:"worker_name"`
	ProjectID          *uint     `gorm:"index" json:"project_id"` // 选填：按项目结算
	ProjectName        string    `json:"project_name"`
	PeriodStart        string    `json:"period_start"`
	PeriodEnd          string    `json:"period_end"`
	WorkAmountCents    int64     `json:"work_amount_cents"`
	AdvanceAmountCents int64     `json:"advance_amount_cents"`
	PayableCents       int64     `json:"payable_cents"`
	RecordCount        int       `json:"record_count"`
	Details            string    `json:"details"` // JSON 快照
	Note               string    `json:"note"`
	SettledAt          time.Time `json:"settled_at"`
	CreatedAt          time.Time `json:"created_at"`
}

func init() {
	database.RegisterModels(&Worker{}, &Project{}, &ProjectWorkerPrice{}, &PieceItem{}, &WorkRecord{}, &Advance{}, &Settlement{}, &EntryCode{})
	database.RegisterUserDeleteGuard(hasWorklogData)
	database.RegisterUserDeleteCleanup(cleanupWorklogUserData)

	database.RegisterModels(&Team{}, &TeamMember{})
	apps.Register(apps.App{
		Name:        "worklog",
		DisplayName: "记工",
		Icon:        "hard-hat",
		RoutePrefix: "/worklog",
		NavPosition: 10,
		Setup:       setupPublicRoutes,
		SetupAuth:   setupRoutes,
		SetupAdmin:  setupAdminRoutes,
	})
}

func hasWorklogData(db *gorm.DB, userID uint) (bool, error) {
	var n int64
	if err := db.Model(&WorkRecord{}).Where("user_id = ?", userID).Count(&n).Error; err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	if err := db.Model(&Advance{}).Where("user_id = ?", userID).Count(&n).Error; err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	if err := db.Model(&Settlement{}).Where("user_id = ?", userID).Count(&n).Error; err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	if err := db.Model(&Worker{}).Where("user_id = ?", userID).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

func cleanupWorklogUserData(db *gorm.DB, userID uint) error {
	for _, model := range []interface{}{&WorkRecord{}, &Advance{}, &Settlement{}, &Worker{}, &Project{}, &ProjectWorkerPrice{}, &PieceItem{}, &EntryCode{}} {
		if err := db.Where("user_id = ?", userID).Delete(model).Error; err != nil {
			return err
		}
	}
	return nil
}
