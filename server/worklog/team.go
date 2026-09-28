package worklog

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/response"
)

// Team 班组：老板创建，成员凭邀请码加入。班组业务数据归属老板账号（user_id）。
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
}

const (
	RoleBoss   = "boss"
	RoleLeader = "leader"
	RoleWorker = "worker"
)

// access 当前请求的访问上下文：数据集归属 + 角色。
type access struct {
	OwnerUID     uint  // 数据集归属用户（班组模式下=老板）
	UID          uint  // 当前登录账号
	Role         string // boss / leader / worker
	TeamID       *uint
	TeamName     string
	SelfWorkerID *uint // worker 角色绑定的工人档案
}

func (a *access) isBoss() bool   { return a.Role == RoleBoss }
func (a *access) canManage() bool { return a.Role == RoleBoss || a.Role == RoleLeader }
func (a *access) inTeam() bool   { return a.TeamID != nil }

// loadAccess 解析当前用户的访问上下文。
func loadAccess(db *gorm.DB, uid uint) *access {
	ac := &access{OwnerUID: uid, UID: uid, Role: RoleBoss}
	// 我拥有的班组
	var owned Team
	if err := db.Where("owner_user_id = ?", uid).First(&owned).Error; err == nil {
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
func (a *access) workerScope() (uint, bool) {
	if a.Role == RoleWorker && a.SelfWorkerID != nil {
		return *a.SelfWorkerID, true
	}
	return 0, false
}

func requireManage(ac *access) bool { return ac.canManage() }

func genInviteCode() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	code := hex.EncodeToString(b) // 8 位十六进制
	return code
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

func createTeam(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		// 已拥有班组或已加入班组则拒绝
		var n int64
		db.Model(&Team{}).Where("owner_user_id = ?", uid).Count(&n)
		if n > 0 {
			response.ErrorBadRequest(c, "你已拥有班组")
			return
		}
		db.Model(&TeamMember{}).Where("user_id = ? AND status = ?", uid, "active").Count(&n)
		if n > 0 {
			response.ErrorBadRequest(c, "你已加入其他班组，请先退出后再创建")
			return
		}
		var in struct {
			Name string `json:"name"`
			Note string `json:"note"`
		}
		if err := c.ShouldBindJSON(&in); err != nil || len([]rune(in.Name)) < 2 {
			response.ErrorBadRequest(c, "请填写班组名称（至少 2 个字）")
			return
		}
		team := Team{Name: in.Name, OwnerUserID: uid, InviteCode: genInviteCode(), Note: in.Note}
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
		response.Success(c, team)
	}
}

func getTeam(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !ac.inTeam() {
			response.Success(c, nil)
			return
		}
		var team Team
		if err := db.Where("id = ?", *ac.TeamID).First(&team).Error; err != nil {
			notFound(c, "班组不存在")
			return
		}
		var members []TeamMember
		db.Where("team_id = ? AND status = ?", team.ID, "active").Order("role ASC, created_at ASC").Find(&members)
		// 填充用户名与工人姓名
		for i := range members {
			var uname string
			db.Table("users").Select("username").Where("id = ?", members[i].UserID).Scan(&uname)
			members[i].Username = uname
			if members[i].WorkerID != nil {
				var w Worker
				if err := db.Where("id = ?", *members[i].WorkerID).First(&w).Error; err == nil {
					members[i].WorkerName = w.Name
				}
			}
		}
		isBoss := ac.isBoss()
		if !isBoss {
			team.InviteCode = "" // 邀请码仅老板可见
		}
		response.Success(c, gin.H{"team": team, "members": members, "my_role": ac.Role, "is_owner": isBoss})
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

func updateMemberRole(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !ac.isBoss() || ac.TeamID == nil {
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
		var member TeamMember
		if err := db.Where("id = ? AND team_id = ?", c.Param("id"), *ac.TeamID).First(&member).Error; err != nil {
			notFound(c, "成员不存在")
			return
		}
		if member.Role == RoleBoss {
			response.ErrorBadRequest(c, "不能修改老板角色")
			return
		}
		if err := db.Model(&member).Update("role", in.Role).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}

func removeMember(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !ac.isBoss() || ac.TeamID == nil {
			response.ErrorForbidden(c, "仅班组老板可操作")
			return
		}
		var member TeamMember
		if err := db.Where("id = ? AND team_id = ?", c.Param("id"), *ac.TeamID).First(&member).Error; err != nil {
			notFound(c, "成员不存在")
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
			return tx.Delete(&member).Error
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

func disbandTeam(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		var team Team
		if err := db.Where("owner_user_id = ?", uid).First(&team).Error; err != nil {
			response.ErrorBadRequest(c, "你没有班组")
			return
		}
		err := db.Transaction(func(tx *gorm.DB) error {
			// 成员绑定的工人档案归档
			var members []TeamMember
			tx.Where("team_id = ? AND status = ?", team.ID, "active").Find(&members)
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
			return tx.Delete(&team).Error
		})
		if err != nil {
			response.ErrorInternal(c, "解散失败")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}
