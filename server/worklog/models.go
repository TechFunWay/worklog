// Package worklog 记工记账业务包：按天/按时/计件记工、借支、结算与工资条、统计与导出。
package worklog

import (
	"gorm.io/gorm"
	"smallgo/server/apps"
	"smallgo/server/database"
	"time"
)

// Worker 工人档案。日薪/时薪为记工录入时的默认单价。
type Worker struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	UserID          uint      `gorm:"index;not null" json:"-"`
	Name            string    `gorm:"not null" json:"name"`
	Phone           string    `json:"phone"`
	Note            string    `json:"note"`
	DailyWageCents  int64     `json:"daily_wage_cents"`
	HourlyWageCents int64     `json:"hourly_wage_cents"`
	Status          string    `gorm:"default:active;index" json:"status"` // active / archived
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Project 工地/项目（选填维度）。
type Project struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"-"`
	Name      string    `gorm:"not null" json:"name"`
	Note      string    `json:"note"`
	Status    string    `gorm:"default:active;index" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

// WorkRecord 记工记录。Type: day(点工)/hour(点时)/piece(计件)。
// 数量存 decimal 文本，金额以整数分存储。SettlementID 非空表示已结算锁定。
type WorkRecord struct {
	ID             uint      `gorm:"primarykey" json:"id"`
	UserID         uint      `gorm:"index;not null" json:"-"`
	WorkerID       uint      `gorm:"index;not null" json:"worker_id"`
	ProjectID      *uint     `json:"project_id"`
	PieceItemID    *uint     `json:"piece_item_id"`
	Date           string    `gorm:"index;not null" json:"date"` // YYYY-MM-DD
	Type           string    `gorm:"index;not null" json:"type"`
	Quantity       string    `gorm:"not null" json:"quantity"`
	UnitPriceCents int64     `json:"unit_price_cents"`
	AmountCents    int64     `json:"amount_cents"`
	Note           string    `json:"note"`
	SettlementID   *uint     `gorm:"index" json:"settlement_id"`
	CreatedBy      uint      `json:"created_by"` // 录入人账号（班组内区分老板/组长代记）
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// 展示字段（联查填充）
	WorkerName  string `gorm:"-" json:"worker_name"`
	ProjectName string `gorm:"-" json:"project_name"`
	PieceName   string `gorm:"-" json:"piece_name"`
	PieceUnit   string `gorm:"-" json:"piece_unit"`
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
	database.RegisterModels(&Worker{}, &Project{}, &PieceItem{}, &WorkRecord{}, &Advance{}, &Settlement{})
	database.RegisterUserDeleteGuard(hasWorklogData)
	database.RegisterUserDeleteCleanup(cleanupWorklogUserData)

	database.RegisterModels(&Team{}, &TeamMember{})
	apps.Register(apps.App{
		Name:        "worklog",
		DisplayName: "记工",
		Icon:        "hard-hat",
		RoutePrefix: "/worklog",
		NavPosition: 10,
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
	for _, model := range []interface{}{&WorkRecord{}, &Advance{}, &Settlement{}, &Worker{}, &Project{}, &PieceItem{}} {
		if err := db.Where("user_id = ?", userID).Delete(model).Error; err != nil {
			return err
		}
	}
	return nil
}
