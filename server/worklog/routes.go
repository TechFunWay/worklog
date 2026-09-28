package worklog

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupRoutes 注册业务路由（认证组，user_id 数据隔离由各 handler 保证）。
func setupRoutes(api *gin.RouterGroup, db *gorm.DB) {
	g := api.Group("/worklog")

	// 基础档案
	g.GET("/workers", listWorkers(db))
	g.POST("/workers", createWorker(db))
	g.PUT("/workers/:id", updateWorker(db))
	g.DELETE("/workers/:id", deleteWorker(db))
	g.GET("/projects", listProjects(db))
	g.POST("/projects", createProject(db))
	g.PUT("/projects/:id", updateProject(db))
	g.DELETE("/projects/:id", deleteProject(db))
	g.GET("/piece-items", listPieceItems(db))
	g.POST("/piece-items", createPieceItem(db))
	g.PUT("/piece-items/:id", updatePieceItem(db))
	g.DELETE("/piece-items/:id", deletePieceItem(db))

	// 记工
	g.GET("/records", recordsByDate(db))
	g.GET("/records/list", listRecords(db))
	g.POST("/records", createRecord(db))
	g.PUT("/records/:id", updateRecord(db))
	g.DELETE("/records/:id", deleteRecord(db))

	// 借支
	g.GET("/advances", listAdvances(db))
	g.POST("/advances", createAdvance(db))
	g.DELETE("/advances/:id", deleteAdvance(db))

	// 结算
	g.GET("/settlements", listSettlements(db))
	g.GET("/settlements/unsettled", unsettledList(db))
	g.GET("/settlements/unsettled-projects", unsettledProjects(db))
	g.GET("/settlements/preview", settlementPreview(db))
	g.POST("/settlements", createSettlement(db))
	g.POST("/settlements/batch", batchSettlement(db))
	g.GET("/settlements/:id", getSettlement(db))
	g.PUT("/settlements/:id/note", updateSettlementNote(db))
	g.DELETE("/settlements/:id", revertSettlement(db))

	// 统计与考勤
	g.GET("/stats/dashboard", statsDashboard(db))
	g.GET("/stats/monthly", statsMonthly(db))
	g.GET("/stats/range", statsRange(db))
	g.GET("/attendance", statsAttendance(db))

	// 角色与班组
	g.GET("/me", handleMe(db))
	g.GET("/team", getTeam(db))
	g.POST("/teams", createTeam(db))
	g.POST("/team/join", joinTeam(db))
	g.PUT("/team/members/:id/role", updateMemberRole(db))
	g.DELETE("/team/members/:id", removeMember(db))
	g.DELETE("/team/leave", leaveTeam(db))
	g.DELETE("/team", disbandTeam(db))

	// 导出（本人数据）
	g.GET("/export/records.csv", exportRecordsCSV(db))
	g.GET("/export/monthly.csv", exportMonthlyCSV(db))
	g.GET("/export/advances.csv", exportAdvancesCSV(db))
	g.GET("/export/settlements.csv", exportSettlementsCSV(db))
	g.GET("/export/json", exportJSON(db))
}

// setupAdminRoutes 管理员路由（全库备份下载与恢复导入）。
func setupAdminRoutes(admin *gin.RouterGroup, db *gorm.DB) {
	admin.GET("/worklog/export/db", exportDB(db))
	admin.POST("/worklog/import/db", restoreDB(db))
}
