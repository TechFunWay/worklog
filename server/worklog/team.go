package worklog

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/database"
	"smallgo/server/response"
)

// Team 班组：老板创建，成员凭邀请码加入。班组业务数据归属老板账号（user_id）。
// 一个老板可创建多个班组（上限 maxTeamsPerUser）；成员同一时间只属于一个班组。
type Team struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	Name        string    `gorm:"not null" json:"name"`
	OwnerUserID uint      `gorm:"index;not null" json:"-"`
	InviteCode  string    `gorm:"uniqueIndex;not null" json:"invite_code"`
	Note        string    `json:"note"`
	CreatedAt   time.Time `json:"created_at"`
}

// TeamMember 班组成员。Role: boss / leader / worker。
// WorkerID 指向班组老板名下的工人档案（worker 角色必填，即"自己"）。
type TeamMember struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	TeamID    uint      `gorm:"index;not null" json:"team_id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Role      string    `gorm:"not null" json:"role"`
	WorkerID  *uint     `json:"worker_id"`
	Status    string    `gorm:"default:active" json:"status"`
	CreatedAt time.Time `json:"created_at"`

	Username   string `gorm:"-" json:"username"`
	WorkerName string `gorm:"-" json:"worker_name"`
	// 单价来自成员绑定的工人档案（展示用，不落 TeamMember 表）
	DailyWageCents  int64 `gorm:"-" json:"daily_wage_cents"`
	HourlyWageCents int64 `gorm:"-" json:"hourly_wage_cents"`
}

const (
	RoleBoss   = "boss"
	RoleLeader = "leader"
	RoleWorker = "worker"
)

// maxTeamsPerUser 单个老板可创建的班组上限。
const maxTeamsPerUser = 20

// TeamCard 班组列表卡片（GET /worklog/teams）。
type TeamCard struct {
	ID          uint      `json:"id"`
	Name        string    `json:"name"`
	Note        string    `json:"note"`
	InviteCode  string    `json:"invite_code"`
	CreatedAt   time.Time `json:"created_at"`
	IsOwner     bool      `json:"is_owner"`
	MyRole      string    `json:"my_role"`
	MemberCount int64     `json:"member_count"`
}

// access 当前请求的访问上下文：数据集归属 + 角色。
type access struct {
	OwnerUID     uint   // 数据集归属用户（班组模式下=老板）
	UID          uint   // 当前登录账号
	Role         string // boss / leader / worker
	TeamID       *uint
	TeamName     string
	SelfWorkerID *uint // worker 角色绑定的工人档案
}

func (a *access) isBoss() bool    { return a.Role == RoleBoss }
func (a *access) canManage() bool { return a.Role == RoleBoss || a.Role == RoleLeader }
func (a *access) inTeam() bool    { return a.TeamID != nil }

// loadAccess 解析当前用户的访问上下文。多班组下保持"第一个自有班组"作为
// 默认上下文（业务数据归属不变，仍按 owner 账号取数）；需要指定班组的新接口
// 用显式 team_id 参数。
func loadAccess(db *gorm.DB, uid uint) *access {
	ac := &access{OwnerUID: uid, UID: uid, Role: RoleBoss}
	// 我拥有的班组（多个时取最早创建的一个作为默认上下文）
	var owned Team
	if err := db.Where("owner_user_id = ?", uid).Order("created_at ASC, id ASC").First(&owned).Error; err == nil {
		ac.TeamID = &owned.ID
		ac.TeamName = owned.Name
		return ac
	}
	// 我加入的班组
	var member TeamMember
	if err := db.Where("user_id = ? AND status = ?", uid, "active").First(&member).Error; err == nil {
		var team Team
		if err := db.Where("id = ?", member.TeamID).First(&team).Error; err == nil {
			ac.TeamID = &team.ID
			ac.TeamName = team.Name
			ac.Role = member.Role
			ac.OwnerUID = team.OwnerUserID
			ac.SelfWorkerID = member.WorkerID
			return ac
		}
	}
	return ac
}

// scopeUser 返回业务查询应使用的数据集归属 user_id。
func (a *access) scopeUser() uint { return a.OwnerUID }

// workerScope 供 worker 角色强制过滤自己的记录；非 worker 返回 false。
//
// worker 角色但成员行没绑工人档案（WorkerID 为 nil，历史数据或半途失败的事务
// 会留下这种行）时返回 (0, true)：0 不是合法工人 ID，凡是 `worker_id = ?` /
// `id = ?` 的过滤都会落空。**必须 fail closed**——早先这里返回 false，调用方
// 见 ok=false 就不加过滤，等于把老板视角的全量数据交给一名普通员工（2026-10-18
// 审查实测复现）。需要「全部数据」的只有老板/组长，那条路 Role 不是 worker。
func (a *access) workerScope() (uint, bool) {
	if a.Role == RoleWorker {
		if a.SelfWorkerID != nil {
			return *a.SelfWorkerID, true
		}
		return 0, true
	}
	return 0, false
}

// workerUnbound 标记「worker 角色但未绑定工人档案」这种不完整状态。
// 调用方在 workerScope 命中的前提下，若拿到的 id 为 0，应直接拒绝而不是
// 退回「全部」或「第一个工人」。
func (a *access) workerUnbound() bool {
	return a.Role == RoleWorker && a.SelfWorkerID == nil
}

func requireManage(ac *access) bool { return ac.canManage() }

func genInviteCode() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	code := hex.EncodeToString(b) // 8 位十六进制
	return code
}

// buildTeamCard 组装班组卡片；邀请码只对老板可见。
func buildTeamCard(db *gorm.DB, team Team, isOwner bool, myRole string) TeamCard {
	var count int64
	db.Model(&TeamMember{}).Where("team_id = ? AND status = ?", team.ID, "active").Count(&count)
	card := TeamCard{
		ID:          team.ID,
		Name:        team.Name,
		Note:        team.Note,
		CreatedAt:   team.CreatedAt,
		IsOwner:     isOwner,
		MyRole:      myRole,
		MemberCount: count,
	}
	if isOwner {
		card.InviteCode = team.InviteCode
	}
	return card
}

// fillMemberDisplay 填充成员的账号名、工人姓名与单价（来自绑定的工人档案）。
func fillMemberDisplay(db *gorm.DB, members []TeamMember) {
	for i := range members {
		var uname string
		db.Table("users").Select("username").Where("id = ?", members[i].UserID).Scan(&uname)
		members[i].Username = uname
		if members[i].WorkerID != nil {
			var w Worker
			if err := db.Where("id = ?", *members[i].WorkerID).First(&w).Error; err == nil {
				members[i].WorkerName = w.Name
				members[i].DailyWageCents = w.DailyWageCents
				members[i].HourlyWageCents = w.HourlyWageCents
			}
		}
	}
}

// memberOwnedBy 按成员 id 查找成员与所属班组，并校验班组属于当前老板。
// 返回 (成员, 班组, 是否属于当前老板)；成员不存在时成员为 nil。
func memberOwnedBy(db *gorm.DB, uid uint, memberID string) (*TeamMember, *Team, bool) {
	var member TeamMember
	if err := db.Where("id = ?", memberID).First(&member).Error; err != nil {
		return nil, nil, false
	}
	var team Team
	if err := db.Where("id = ? AND owner_user_id = ?", member.TeamID, uid).First(&team).Error; err != nil {
		return &member, nil, false
	}
	return &member, &team, true
}

// disbandTeamTx 解散班组：成员绑定的工人档案归档，成员解绑，班组删除。
// 业务数据（记工/借支/结算/工人档案）保持归属老板账号不变。
func disbandTeamTx(tx *gorm.DB, team *Team) error {
	var members []TeamMember
	if err := tx.Where("team_id = ? AND status = ?", team.ID, "active").Find(&members).Error; err != nil {
		return err
	}
	for _, m := range members {
		if m.WorkerID != nil {
			if err := tx.Model(&Worker{}).Where("id = ?", *m.WorkerID).
				Update("status", "archived").Error; err != nil {
				return err
			}
		}
	}
	if err := tx.Where("team_id = ?", team.ID).Delete(&TeamMember{}).Error; err != nil {
		return err
	}
	return tx.Delete(team).Error
}

// ---- 班组 handlers ----

type meInfo struct {
	UID          uint   `json:"uid"`
	Username     string `json:"username"`
	Role         string `json:"role"` // boss / leader / worker
	InTeam       bool   `json:"in_team"`
	TeamID       *uint  `json:"team_id"`
	TeamName     string `json:"team_name"`
	SelfWorkerID *uint  `json:"self_worker_id"`
	SelfWorker   string `json:"self_worker_name"`
}

func handleMe(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		info := meInfo{UID: uid, Role: ac.Role, InTeam: ac.inTeam(), TeamID: ac.TeamID, TeamName: ac.TeamName, SelfWorkerID: ac.SelfWorkerID}
		var username string
		db.Table("users").Select("username").Where("id = ?", uid).Scan(&username)
		info.Username = username
		if ac.SelfWorkerID != nil {
			var w Worker
			if err := db.Where("id = ?", *ac.SelfWorkerID).First(&w).Error; err == nil {
				info.SelfWorker = w.Name
			}
		}
		response.Success(c, info)
	}
}

// listTeams 我的班组列表：老板返回自己全部班组，成员返回所在班组（joined）。
func listTeams(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		var owned []Team
		if err := db.Where("owner_user_id = ?", uid).Order("created_at ASC, id ASC").Find(&owned).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		teams := make([]TeamCard, 0, len(owned))
		for _, t := range owned {
			teams = append(teams, buildTeamCard(db, t, true, RoleBoss))
		}
		var joined *TeamCard
		if len(owned) == 0 {
			var member TeamMember
			if err := db.Where("user_id = ? AND status = ?", uid, "active").First(&member).Error; err == nil {
				var team Team
				if err := db.Where("id = ?", member.TeamID).First(&team).Error; err == nil {
					card := buildTeamCard(db, team, false, member.Role)
					joined = &card
				}
			}
		}
		response.Success(c, gin.H{"teams": teams, "joined": joined})
	}
}

// createTeam 创建班组：老板可创建多个（上限 20），已加入其他班组时不能创建。
func createTeam(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		var in struct {
			Name string `json:"name"`
			Note string `json:"note"`
		}
		if err := c.ShouldBindJSON(&in); err != nil || len([]rune(strings.TrimSpace(in.Name))) < 2 {
			response.ErrorBadRequest(c, "请填写班组名称（至少 2 个字）")
			return
		}
		// 一个账号只能加入一个班组
		var n int64
		if err := db.Model(&TeamMember{}).Where("user_id = ? AND status = ?", uid, "active").Count(&n).Error; err != nil {
			response.ErrorInternal(c, "创建失败")
			return
		}
		if n > 0 {
			response.ErrorBadRequest(c, "你已加入其他班组，请先退出后再创建")
			return
		}
		if err := db.Model(&Team{}).Where("owner_user_id = ?", uid).Count(&n).Error; err != nil {
			response.ErrorInternal(c, "创建失败")
			return
		}
		if n >= maxTeamsPerUser {
			response.ErrorBadRequest(c, "最多创建 20 个班组")
			return
		}
		team := Team{
			Name:        strings.TrimSpace(in.Name),
			OwnerUserID: uid,
			InviteCode:  genInviteCode(),
			Note:        strings.TrimSpace(in.Note),
		}
		// 邀请码防碰撞
		for i := 0; i < 5; i++ {
			var dup int64
			db.Model(&Team{}).Where("invite_code = ?", team.InviteCode).Count(&dup)
			if dup == 0 {
				break
			}
			team.InviteCode = genInviteCode()
		}
		if err := db.Create(&team).Error; err != nil {
			response.ErrorInternal(c, "创建失败")
			return
		}
		response.Success(c, buildTeamCard(db, team, true, RoleBoss))
	}
}

// updateTeam 修改班组名称/备注（老板，按 team_id 指定）。
func updateTeam(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		var team Team
		if err := db.Where("id = ? AND owner_user_id = ?", c.Param("id"), uid).First(&team).Error; err != nil {
			notFound(c, "班组不存在或你不是该班组老板")
			return
		}
		var in struct {
			Name *string `json:"name"`
			Note *string `json:"note"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		updates := map[string]interface{}{}
		if in.Name != nil {
			name := strings.TrimSpace(*in.Name)
			if len([]rune(name)) < 2 {
				response.ErrorBadRequest(c, "请填写班组名称（至少 2 个字）")
				return
			}
			updates["name"] = name
		}
		if in.Note != nil {
			updates["note"] = strings.TrimSpace(*in.Note)
		}
		if len(updates) == 0 {
			response.ErrorBadRequest(c, "没有需要修改的内容")
			return
		}
		if err := db.Model(&team).Updates(updates).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		if err := db.Where("id = ?", team.ID).First(&team).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		response.Success(c, buildTeamCard(db, team, true, RoleBoss))
	}
}

// getTeam 班组详情：支持显式 team_id（老板可查自己任一班组，成员只能查自己所在班组）；
// 不传 team_id 时保持旧行为（默认上下文 = 第一个班组）。
func getTeam(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		var team Team
		role := RoleBoss
		isBoss := false
		if teamID := uint(atoiDefault(c.Query("team_id"), 0)); teamID > 0 {
			if err := db.Where("id = ?", teamID).First(&team).Error; err != nil {
				notFound(c, "班组不存在")
				return
			}
			if team.OwnerUserID == uid {
				isBoss, role = true, RoleBoss
			} else {
				var member TeamMember
				if err := db.Where("team_id = ? AND user_id = ? AND status = ?", team.ID, uid, "active").First(&member).Error; err != nil {
					response.ErrorForbidden(c, "你不在该班组中")
					return
				}
				role = member.Role
			}
		} else {
			ac := loadAccess(db, uid)
			if !ac.inTeam() {
				response.Success(c, nil)
				return
			}
			if err := db.Where("id = ?", *ac.TeamID).First(&team).Error; err != nil {
				notFound(c, "班组不存在")
				return
			}
			isBoss, role = ac.isBoss(), ac.Role
		}
		var members []TeamMember
		db.Where("team_id = ? AND status = ?", team.ID, "active").Order("role ASC, created_at ASC").Find(&members)
		fillMemberDisplay(db, members)
		if !isBoss {
			team.InviteCode = "" // 邀请码仅老板可见
		}
		response.Success(c, gin.H{"team": team, "members": members, "my_role": role, "is_owner": isBoss})
	}
}

func joinTeam(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		var in struct {
			InviteCode string `json:"invite_code"`
			WorkerName string `json:"worker_name"`
		}
		if err := c.ShouldBindJSON(&in); err != nil || in.InviteCode == "" {
			response.ErrorBadRequest(c, "请填写邀请码")
			return
		}
		if len([]rune(in.WorkerName)) < 1 {
			response.ErrorBadRequest(c, "请填写你的姓名（用于记工）")
			return
		}
		// 已在班组 / 已拥有班组
		var n int64
		db.Model(&Team{}).Where("owner_user_id = ?", uid).Count(&n)
		if n > 0 {
			response.ErrorBadRequest(c, "你是班组老板，无需加入")
			return
		}
		db.Model(&TeamMember{}).Where("user_id = ? AND status = ?", uid, "active").Count(&n)
		if n > 0 {
			response.ErrorBadRequest(c, "你已加入班组，请先退出")
			return
		}
		var team Team
		if err := db.Where("invite_code = ?", in.InviteCode).First(&team).Error; err != nil {
			response.ErrorBadRequest(c, "邀请码无效")
			return
		}
		if team.OwnerUserID == uid {
			response.ErrorBadRequest(c, "这是你自己的班组")
			return
		}
		// 创建工人档案（归属班组老板数据集，绑定当前账号）
		worker := Worker{UserID: team.OwnerUserID, Name: in.WorkerName, Status: "active"}
		if err := db.Create(&worker).Error; err != nil {
			response.ErrorInternal(c, "加入失败")
			return
		}
		member := TeamMember{TeamID: team.ID, UserID: uid, Role: RoleWorker, WorkerID: &worker.ID, Status: "active"}
		if err := db.Create(&member).Error; err != nil {
			response.ErrorInternal(c, "加入失败")
			return
		}
		response.Success(c, gin.H{"team": team, "worker": worker})
	}
}

// addTeamMember 老板在用户管理里直接把已有用户加进自己的班组。
// 事务内创建归属老板数据集的工人档案（带日薪/时薪）+ 成员关系。
func addTeamMember(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		var in struct {
			TeamID          uint   `json:"team_id"`
			UserID          uint   `json:"user_id"`
			Role            string `json:"role"`
			WorkerName      string `json:"worker_name"`
			DailyWageCents  int64  `json:"daily_wage_cents"`
			HourlyWageCents int64  `json:"hourly_wage_cents"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		if in.Role != RoleLeader && in.Role != RoleWorker {
			response.ErrorBadRequest(c, "角色只能是员工或组长")
			return
		}
		if in.TeamID == 0 || in.UserID == 0 {
			response.ErrorBadRequest(c, "请选择班组和用户")
			return
		}
		if in.UserID == uid {
			response.ErrorBadRequest(c, "不能把自己加入班组")
			return
		}
		if in.DailyWageCents < 0 || in.HourlyWageCents < 0 {
			response.ErrorBadRequest(c, "单价不能为负数")
			return
		}
		// 目标班组必须属于当前老板
		var team Team
		if err := db.Where("id = ? AND owner_user_id = ?", in.TeamID, uid).First(&team).Error; err != nil {
			response.ErrorForbidden(c, "班组不存在或你不是该班组老板")
			return
		}
		// 目标用户存在且启用
		var target database.User
		if err := db.Where("id = ?", in.UserID).First(&target).Error; err != nil {
			response.ErrorBadRequest(c, "目标用户不存在")
			return
		}
		if target.Status != 1 {
			response.ErrorBadRequest(c, "该用户已被禁用，无法加入班组")
			return
		}
		var n int64
		if err := db.Model(&Team{}).Where("owner_user_id = ?", in.UserID).Count(&n).Error; err != nil {
			response.ErrorInternal(c, "添加失败")
			return
		}
		if n > 0 {
			response.ErrorBadRequest(c, "该用户自己就是班组老板，不能加入其他班组")
			return
		}
		// 是否已在当前班组
		if err := db.Model(&TeamMember{}).Where("team_id = ? AND user_id = ? AND status = ?", team.ID, in.UserID, "active").Count(&n).Error; err != nil {
			response.ErrorInternal(c, "添加失败")
			return
		}
		if n > 0 {
			response.ErrorBadRequest(c, "该用户已在当前班组中")
			return
		}
		// 是否已在其他班组（一个账号同一时间只能加入一个班组）
		if err := db.Model(&TeamMember{}).Where("user_id = ? AND status = ?", in.UserID, "active").Count(&n).Error; err != nil {
			response.ErrorInternal(c, "添加失败")
			return
		}
		if n > 0 {
			response.ErrorBadRequest(c, "该用户已加入其他班组，请先让其退出后再添加")
			return
		}
		name := strings.TrimSpace(in.WorkerName)
		if name == "" {
			name = target.Username
		}
		if name == "" {
			response.ErrorBadRequest(c, "请填写成员姓名")
			return
		}
		var worker Worker
		var member TeamMember
		err := db.Transaction(func(tx *gorm.DB) error {
			worker = Worker{
				UserID:          uid,
				Name:            name,
				DailyWageCents:  in.DailyWageCents,
				HourlyWageCents: in.HourlyWageCents,
				Status:          "active",
			}
			if err := tx.Create(&worker).Error; err != nil {
				return err
			}
			member = TeamMember{TeamID: team.ID, UserID: in.UserID, Role: in.Role, WorkerID: &worker.ID, Status: "active"}
			return tx.Create(&member).Error
		})
		if err != nil {
			response.ErrorInternal(c, "添加成员失败")
			return
		}
		member.Username = target.Username
		member.WorkerName = worker.Name
		member.DailyWageCents = worker.DailyWageCents
		member.HourlyWageCents = worker.HourlyWageCents
		response.Success(c, gin.H{"member": member, "worker": worker})
	}
}

// updateMemberWage 按成员设置日薪/时薪（老板，成员须属于本人拥有的班组）。
// 单价落在成员绑定的工人档案上，记工选工人时自动带出。
func updateMemberWage(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		member, _, ok := memberOwnedBy(db, uid, c.Param("id"))
		if member == nil {
			notFound(c, "成员不存在")
			return
		}
		if !ok {
			response.ErrorForbidden(c, "仅班组老板可操作")
			return
		}
		if member.WorkerID == nil {
			response.ErrorBadRequest(c, "该成员未绑定工人档案，无法设置单价")
			return
		}
		var in struct {
			DailyWageCents  *int64 `json:"daily_wage_cents"`
			HourlyWageCents *int64 `json:"hourly_wage_cents"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		updates := map[string]interface{}{}
		if in.DailyWageCents != nil {
			if *in.DailyWageCents < 0 {
				response.ErrorBadRequest(c, "日薪不能为负数")
				return
			}
			updates["daily_wage_cents"] = *in.DailyWageCents
		}
		if in.HourlyWageCents != nil {
			if *in.HourlyWageCents < 0 {
				response.ErrorBadRequest(c, "时薪不能为负数")
				return
			}
			updates["hourly_wage_cents"] = *in.HourlyWageCents
		}
		if len(updates) == 0 {
			response.ErrorBadRequest(c, "没有需要修改的内容")
			return
		}
		if err := db.Model(&Worker{}).Where("id = ? AND user_id = ?", *member.WorkerID, uid).Updates(updates).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		var w Worker
		if err := db.Where("id = ?", *member.WorkerID).First(&w).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		response.Success(c, gin.H{"ok": true, "daily_wage_cents": w.DailyWageCents, "hourly_wage_cents": w.HourlyWageCents})
	}
}

func updateMemberRole(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		member, _, ok := memberOwnedBy(db, uid, c.Param("id"))
		if member == nil {
			notFound(c, "成员不存在")
			return
		}
		if !ok {
			response.ErrorForbidden(c, "仅班组老板可操作")
			return
		}
		var in struct {
			Role string `json:"role"`
		}
		if err := c.ShouldBindJSON(&in); err != nil || (in.Role != RoleLeader && in.Role != RoleWorker) {
			response.ErrorBadRequest(c, "角色不正确")
			return
		}
		if member.Role == RoleBoss {
			response.ErrorBadRequest(c, "不能修改老板角色")
			return
		}
		if err := db.Model(member).Update("role", in.Role).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}

func removeMember(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		member, _, ok := memberOwnedBy(db, uid, c.Param("id"))
		if member == nil {
			notFound(c, "成员不存在")
			return
		}
		if !ok {
			response.ErrorForbidden(c, "仅班组老板可操作")
			return
		}
		if member.Role == RoleBoss {
			response.ErrorBadRequest(c, "不能移除老板")
			return
		}
		err := db.Transaction(func(tx *gorm.DB) error {
			if member.WorkerID != nil {
				if err := tx.Model(&Worker{}).Where("id = ?", *member.WorkerID).
					Update("status", "archived").Error; err != nil {
					return err
				}
			}
			return tx.Delete(member).Error
		})
		if err != nil {
			response.ErrorInternal(c, "移除失败")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}

func leaveTeam(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		var member TeamMember
		if err := db.Where("user_id = ? AND status = ?", uid, "active").First(&member).Error; err != nil {
			response.ErrorBadRequest(c, "你不在任何班组中")
			return
		}
		if member.Role == RoleBoss {
			response.ErrorBadRequest(c, "老板请使用解散班组")
			return
		}
		err := db.Transaction(func(tx *gorm.DB) error {
			if member.WorkerID != nil {
				if err := tx.Model(&Worker{}).Where("id = ?", *member.WorkerID).
					Update("status", "archived").Error; err != nil {
					return err
				}
			}
			return tx.Delete(&member).Error
		})
		if err != nil {
			response.ErrorInternal(c, "退出失败")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}

// deleteTeam 解散指定班组（老板）。
func deleteTeam(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		var team Team
		if err := db.Where("id = ? AND owner_user_id = ?", c.Param("id"), uid).First(&team).Error; err != nil {
			notFound(c, "班组不存在或你不是该班组老板")
			return
		}
		if err := db.Transaction(func(tx *gorm.DB) error { return disbandTeamTx(tx, &team) }); err != nil {
			response.ErrorInternal(c, "解散失败")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}

// disbandTeam 解散默认班组（旧接口兼容：第一个自有班组）。
func disbandTeam(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		var team Team
		if err := db.Where("owner_user_id = ?", uid).Order("created_at ASC, id ASC").First(&team).Error; err != nil {
			response.ErrorBadRequest(c, "你没有班组")
			return
		}
		if err := db.Transaction(func(tx *gorm.DB) error { return disbandTeamTx(tx, &team) }); err != nil {
			response.ErrorInternal(c, "解散失败")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}
