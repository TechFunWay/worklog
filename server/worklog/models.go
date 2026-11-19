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
	ID               uint   `gorm:"primarykey" json:"id"`
	UserID           uint   `gorm:"index;not null" json:"-"`
	Name             string `gorm:"not null" json:"name"`
	Phone            string `json:"phone"`
	Note             string `json:"note"`
	DailyWageCents   int64  `json:"daily_wage_cents"`
	HourlyWageCents  int64  `json:"hourly_wage_cents"`
	MonthlyWageCents int64  `json:"monthly_wage_cents"`
	// SubsidyCents 档案级默认补贴（分/出勤日，0=未设置）。记工时按
	// 「手填 > 工地单独 > 工地默认 > 工人档案」带出，可单条改或清零。
	SubsidyCents int64     `json:"subsidy_cents"`
	Status       string    `gorm:"default:active;index" json:"status"` // active / archived
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Project 工地/项目（选填维度）。DayPriceCents/HourPriceCents 是工地默认单价
// （0=未设置，记工时回落到工人自身单价）；SubsidyCents 是工地默认补贴（分/出勤日）。
// 「某工人某工地单独定价」见 ProjectWorkerPrice，优先级高于工地默认值。
type Project struct {
	ID             uint   `gorm:"primarykey" json:"id"`
	UserID         uint   `gorm:"index;not null" json:"-"`
	Name           string `gorm:"not null" json:"name"`
	Note           string `json:"note"`
	Status         string `gorm:"default:active;index" json:"status"`
	DayPriceCents  int64  `json:"day_price_cents"`  // 工地默认日工单价（分/个工）
	HourPriceCents int64  `json:"hour_price_cents"` // 工地默认小时单价（分/小时）
	// SubsidyCents 工地默认补贴（分/出勤日，如「饭补 15/天」，0=未设置）。
	SubsidyCents int64     `json:"subsidy_cents"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ProjectWorkerPrice 工人在某工地的单独定价：覆盖工地默认单价与工人自身单价。
// 同一 (project_id, worker_id) 只有一行；日价/时价/补贴三项都为 0 时该行删除（视为无覆盖）。
type ProjectWorkerPrice struct {
	ID              uint  `gorm:"primarykey" json:"id"`
	UserID          uint  `gorm:"index;not null" json:"-"`
	ProjectID       uint  `gorm:"not null;uniqueIndex:idx_project_worker" json:"project_id"`
	WorkerID        uint  `gorm:"not null;uniqueIndex:idx_project_worker" json:"worker_id"`
	DailyWageCents  int64 `json:"daily_wage_cents"`
	HourlyWageCents int64 `json:"hourly_wage_cents"`
	// SubsidyCents 该工人在该工地的默认补贴（分/出勤日）；与两个单价一起构成
	// 「单独定价」的一行，三项全 0 时删除该行（视为无覆盖）。
	SubsidyCents int64     `json:"subsidy_cents"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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
//
// 金额口径（v0.6.0 起）：
//
//	WageCents    = 数量 × UnitPriceCents          纯工钱
//	AmountCents  = WageCents + SubsidyCents - DeductCents   该笔应得合计
//
// AmountCents 仍是全应用唯一的「金额真相」：统计、考勤、对工、结算一律只 SUM 它，
// 所以补贴与扣款必须在服务端写入口并进 AmountCents，不要在下游再加一遍。
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
	WageCents      int64  `json:"wage_cents"`    // 纯工钱 = 数量 × 单价（补贴与扣款之外的部分）
	SubsidyCents   int64  `json:"subsidy_cents"` // 补贴合计（≥0，与数量无关的一笔钱）
	SubsidyNote    string `json:"subsidy_note"`  // 补贴说明，如「饭补15+车补10」
	DeductCents    int64  `json:"deduct_cents"`  // 扣款合计（≥0，迟到/损坏/罚款等）
	AmountCents    int64  `json:"amount_cents"`  // 应得合计 = Wage + Subsidy - Deduct
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

// Advance 借支记录。ProjectID 选填（v0.6.0 起，粉丝反馈「借支分不清是哪个工地出的」）：
// NULL = 未指定工地，只在按人结算时抵扣；指定工地的借支在按该项目结算时抵扣。
// 一笔借支只会被一张结算单挂走（settlement_id 锁定），两条路径互斥、不会重复扣。
type Advance struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	UserID       uint      `gorm:"index;index:idx_advance_user_project,priority:1;not null" json:"-"`
	WorkerID     uint      `gorm:"index;index:idx_advance_scope,priority:2;not null" json:"worker_id"`
	ProjectID    *uint     `gorm:"index;index:idx_advance_user_project,priority:2" json:"project_id"`
	Date         string    `gorm:"index;not null" json:"date"`
	AmountCents  int64     `json:"amount_cents"`
	Method       string    `json:"method"` // cash / wechat / alipay / bank / other
	Note         string    `json:"note"`
	SettlementID *uint     `gorm:"index;index:idx_advance_scope,priority:3" json:"settlement_id"`
	CreatedBy    uint      `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`

	WorkerName  string `gorm:"-" json:"worker_name"`
	ProjectName string `gorm:"-" json:"project_name"`
}

// 结算单付款状态：把「算清该给多少」与「实际给了多少」分开。
// 应付 PayableCents 由系统算出、人不可直接改；已付 PaidCents 由付款流水聚合。
const (
	PayStatusUnpaid  = "unpaid"  // 分文未付
	PayStatusPartial = "partial" // 部分支付
	PayStatusPaid    = "paid"    // 已付清
)

// Settlement 结算单（工资条）。冗余工人姓名与期间汇总，Details 存明细 JSON 快照。
//
// 金额口径（v0.6.0 起）：
//
//	应付 PayableCents = WorkAmountCents - AdvanceAmountCents + AdjustCents
//	尚欠 DueCents     = PayableCents - PaidCents（派生，不落库）
//
// 欠薪留在结算单上，绝不退回未结算池：记工已被 settlement_id 锁定，
// 再进未结算列表会让同一笔工被结第二次（真金白银的双付）。
type Settlement struct {
	ID                 uint   `gorm:"primarykey" json:"id"`
	UserID             uint   `gorm:"index;not null" json:"-"`
	WorkerID           uint   `gorm:"index;not null" json:"worker_id"`
	WorkerName         string `json:"worker_name"`
	ProjectID          *uint  `gorm:"index" json:"project_id"` // 选填：按项目结算
	ProjectName        string `json:"project_name"`
	PeriodStart        string `json:"period_start"`
	PeriodEnd          string `json:"period_end"`
	WorkAmountCents    int64  `json:"work_amount_cents"`
	AdvanceAmountCents int64  `json:"advance_amount_cents"`
	// 调整项：抹零/迟到扣款为负，额外奖励为正。工资条要能把「差的这点钱」说清楚。
	AdjustCents  int64  `json:"adjust_cents"`
	AdjustNote   string `json:"adjust_note"`
	PayableCents int64  `json:"payable_cents"`
	RecordCount  int    `json:"record_count"`
	Details      string `json:"details"` // JSON 快照
	Note         string `json:"note"`
	// PaidCents 是 SettlementPayment 的聚合冗余，只在同一事务内由 SUM 重算，不做累加。
	PaidCents int64     `json:"paid_cents"`
	PayStatus string    `gorm:"index;default:paid" json:"pay_status"` // unpaid / partial / paid
	SettledAt time.Time `json:"settled_at"`
	CreatedAt time.Time `json:"created_at"`

	// 展示字段（接口层派生/联查填充，不落库，避免双写不一致）
	DueCents     int64               `gorm:"-" json:"due_cents"`
	PaymentCount int                 `gorm:"-" json:"payment_count"`
	Payments     []SettlementPayment `gorm:"-" json:"payments,omitempty"`
}

// SettlementPayment 结算单的付款流水：一张结算单可分多笔支付（v0.6.0）。

// 粉丝反馈「有时不一定一次性工资结算完」。实付挂在结算单上、不挂记工上，
// 是为了让记工的 settlement_id 锁定语义保持不变（欠薪不回未结算池，见 Settlement 注释）。
// 后续「付款凭证图片 / 微信转账单号 / 押金代付」都往这张表加列即可，不动结算逻辑。
type SettlementPayment struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	UserID       uint      `gorm:"index;not null" json:"-"` // 数据集归属，与结算单一致
	SettlementID uint      `gorm:"index;not null" json:"settlement_id"`
	WorkerID     uint      `gorm:"index;not null" json:"worker_id"` // 冗余：流水要能独立按人查
	AmountCents  int64     `json:"amount_cents"`
	Method       string    `json:"method"`                        // 复用 methodLabels：cash / wechat / alipay / bank / other
	PaidOn       string    `gorm:"index;not null" json:"paid_on"` // YYYY-MM-DD，工人要核对哪天到账
	Note         string    `json:"note"`
	CreatedBy    uint      `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`

	WorkerName   string `gorm:"-" json:"worker_name"`
	SettlementNo string `gorm:"-" json:"settlement_no"`
}

func init() {
	database.RegisterModels(&Worker{}, &Project{}, &ProjectWorkerPrice{}, &PieceItem{}, &WorkRecord{}, &Advance{}, &Settlement{}, &SettlementPayment{}, &EntryCode{})
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
	// 付款流水要在结算单之前删（它按 settlement_id 关联）；新表必须登记进这份清单，否则删账号会留孤儿数据。
	for _, model := range []interface{}{&SettlementPayment{}, &WorkRecord{}, &Advance{}, &Settlement{}, &Worker{}, &Project{}, &ProjectWorkerPrice{}, &PieceItem{}, &EntryCode{}} {
		if err := db.Where("user_id = ?", userID).Delete(model).Error; err != nil {
			return err
		}
	}
	return nil
}
