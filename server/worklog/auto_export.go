// auto_export.go — 定时导出与导出目录选择。
//
// 粉丝诉求：Excel 表能设定每天几点自动导出到指定文件夹。
// 设计口径：
//   - 导出目录沿用系统级 worklog.export_dir（飞牛上可指向共享文件夹，Docker 上指向挂载目录）；
//   - 定时导出的「时间 / 内容范围 / 保留天数」是每个老板自己的偏好（user scope 配置）；
//   - 目录浏览接口只能在自己的「允许根目录」内活动，防止路径穿越（上架自查第 4 条要求）。
package worklog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/database"
	"smallgo/server/logger"
	"smallgo/server/response"
	"smallgo/server/scheduler"
	"smallgo/server/sysconfig"
)

const (
	allowedRootsKey    = "worklog.allowed_export_roots"
	autoExportEnabled  = "worklog.auto_export_enabled"
	autoExportTimeKey  = "worklog.auto_export_time"
	autoExportScopeKey = "worklog.auto_export_scope"
	autoExportKeepKey  = "worklog.auto_export_keep_days"
)

func init() {
	sysconfig.RegisterConfig(sysconfig.ConfigDef{
		Key: allowedRootsKey, Scope: sysconfig.ScopeSystem, Type: sysconfig.TypeString,
		Group: "storage", Label: "允许选择的目录",
		Description: "目录选择器可见的根目录，多个用英文逗号分隔的绝对路径；留空则默认导出目录与数据目录",
	})
	sysconfig.RegisterConfig(sysconfig.ConfigDef{
		Key: autoExportEnabled, Scope: sysconfig.ScopeUser, Type: sysconfig.TypeBool,
		Default: "false", Group: "auto", Label: "启用定时导出",
		Description: "每天在设定时间自动把 Excel 导出到上面的导出目录",
	})
	sysconfig.RegisterConfig(sysconfig.ConfigDef{
		Key: autoExportTimeKey, Scope: sysconfig.ScopeUser, Type: sysconfig.TypeString,
		Default: "20:00", Group: "auto", Label: "每天导出时间",
		Description: "24 小时制 HH:MM，如 20:00",
	})
	sysconfig.RegisterConfig(sysconfig.ConfigDef{
		Key: autoExportScopeKey, Scope: sysconfig.ScopeUser, Type: sysconfig.TypeSelect,
		Default: "attendance", Options: []string{"attendance", "settlements", "both"},
		Group: "auto", Label: "导出内容",
		Description: "attendance=当月考勤与工资条；settlements=全部结算单；both=两种都导",
	})
	sysconfig.RegisterConfig(sysconfig.ConfigDef{
		Key: autoExportKeepKey, Scope: sysconfig.ScopeUser, Type: sysconfig.TypeInt,
		Default: "30", Group: "auto", Label: "自动导出保留天数",
		Description: "超过该天数的自动导出文件会被清理，0 表示不清理",
	})

	scheduler.Register(scheduler.Job{
		Name:     "worklog_auto_export",
		Interval: time.Minute,
		Run:      runAutoExportTick,
	})
}

var autoExportMu sync.Mutex
var autoExportLast = map[uint]string{} // uid -> YYYY-MM-DD，当天已跑过就不重复
// autoExportDB 由 setupRoutes 注入（scheduler 的 Run 不带参数）。
var autoExportDB *gorm.DB

func userConfig(db *gorm.DB, uid uint, key, def string) string {
	v, err := sysconfig.GetConfig(db, key, uid)
	if err != nil || strings.TrimSpace(v) == "" {
		if d, ok := sysconfig.GetConfigDef(key); ok && d.Default != "" {
			return d.Default
		}
		return def
	}
	return v
}

func runAutoExportTick() {
	db := autoExportDB
	if db == nil {
		return
	}
	now := time.Now()
	hhmm := now.Format("15:04")
	today := now.Format("2006-01-02")
	var users []database.User
	if err := db.Where("status = ?", 1).Find(&users).Error; err != nil {
		return
	}
	for _, u := range users {
		if userConfig(db, u.ID, autoExportEnabled, "false") != "true" {
			continue
		}
		if userConfig(db, u.ID, autoExportTimeKey, "20:00") != hhmm {
			continue
		}
		autoExportMu.Lock()
		if autoExportLast[u.ID] == today {
			autoExportMu.Unlock()
			continue
		}
		autoExportLast[u.ID] = today
		autoExportMu.Unlock()
		if _, err := exportForUser(db, u.ID, userConfig(db, u.ID, autoExportScopeKey, "attendance"), now); err != nil {
			// 定时任务失败只记日志，不 panic：下一次启动或次日会再试
			logAutoExportError(u.ID, err)
		}
	}
}

func logAutoExportError(uid uint, err error) {
	defer func() { _ = recover() }()
	logger.Warn("worklog 定时导出失败 user_id=%d err=%v", uid, err)
}

// exportForUser 为某个老板生成一次自动导出，返回生成的文件路径。
func exportForUser(db *gorm.DB, uid uint, scope string, now time.Time) ([]string, error) {
	base, err := resolveExportDir(db)
	if err != nil {
		return nil, err
	}
	var uname string
	db.Table("users").Select("username").Where("id = ?", uid).Scan(&uname)
	if strings.TrimSpace(uname) == "" {
		uname = "user" + strconv.FormatUint(uint64(uid), 10)
	}
	dir := filepath.Join(base, "auto", now.Format("2006-01"), safeFilePart(uname))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建导出目录失败：%w", err)
	}
	stamp := now.Format("20060102-1504")
	var out []string
	if scope == "attendance" || scope == "both" || scope == "" {
		y, m := now.Year(), int(now.Month())
		spec := &attendanceExportSpec{
			dsUID: uid, year: y, month: m,
			start: monthRangeStart(y, m), end: monthRangeEnd(y, m),
		}
		spec.Start, spec.End = spec.start, spec.end
		yy, mm := y, m
		spec.Year, spec.Month = &yy, &mm
		data, err := buildAttendanceXLSX(db, spec)
		if err != nil {
			return out, fmt.Errorf("生成考勤 Excel 失败：%w", err)
		}
		name := fmt.Sprintf("工记考勤-%s-%s.xlsx", now.Format("2006-01"), stamp)
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return out, err
		}
		out = append(out, p)
	}
	if scope == "settlements" || scope == "both" {
		data, err := buildSettlementsXLSX(db, uid)
		if err != nil {
			return out, fmt.Errorf("生成结算 Excel 失败：%w", err)
		}
		name := fmt.Sprintf("工记结算-%s.xlsx", stamp)
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return out, err
		}
		out = append(out, p)
	}
	pruneAutoExports(db, uid, base, now)
	return out, nil
}

func monthRangeStart(y, m int) string {
	return fmt.Sprintf("%04d-%02d-01", y, m)
}

func monthRangeEnd(y, m int) string {
	last := time.Date(y, time.Month(m)+1, 0, 0, 0, 0, 0, time.Local).Day()
	return fmt.Sprintf("%04d-%02d-%02d", y, m, last)
}

// buildSettlementsXLSX 结算单 Excel（一列一列对齐 CSV 的口径，方便直接做工资表）。
func buildSettlementsXLSX(db *gorm.DB, uid uint) ([]byte, error) {
	var items []Settlement
	if err := db.Where("user_id = ?", uid).Order("settled_at ASC, id ASC").Limit(20000).Find(&items).Error; err != nil {
		return nil, err
	}
	wmap := map[uint]Worker{}
	var ws []Worker
	db.Where("user_id = ?", uid).Find(&ws)
	for _, w := range ws {
		wmap[w.ID] = w
	}
	head := []string{"结算单号", "工人", "工号", "期间起", "期间止", "应发(元)", "借支(元)", "实发(元)", "记工条数", "结算时间", "备注", "调整项(元)", "调整原因", "已付(元)", "尚欠(元)", "付款状态", "开户行"}
	rows := [][]xlsxCell{
		func() []xlsxCell {
			cs := make([]xlsxCell, len(head))
			for i, h := range head {
				cs[i] = xlsxText(h, xlsxStyleHeader)
			}
			return cs
		}(),
	}
	for _, s := range items {
		w := wmap[s.WorkerID]
		r := []xlsxCell{
			xlsxText(fmt.Sprintf("JS%06d", s.ID), 0), xlsxText(s.WorkerName, 0), xlsxText(w.WorkerNo, 0),
			xlsxText(s.PeriodStart, 0), xlsxText(s.PeriodEnd, 0),
			xlsxNum(float64(s.WorkAmountCents)/100, xlsxStyleMoney), xlsxNum(float64(s.AdvanceAmountCents)/100, xlsxStyleMoney),
			xlsxNum(float64(s.PayableCents)/100, xlsxStyleMoney), xlsxNum(float64(s.RecordCount), 0),
			xlsxText(s.SettledAt.Format("2006-01-02 15:04"), 0), xlsxText(s.Note, 0),
			xlsxNum(float64(s.AdjustCents)/100, xlsxStyleMoney), xlsxText(s.AdjustNote, 0),
			xlsxNum(float64(s.PaidCents)/100, xlsxStyleMoney), xlsxNum(float64(settlementDueCents(s.PayableCents, s.PaidCents))/100, xlsxStyleMoney),
			xlsxText(csvPayStatusLabel(s.PayStatus, s.PayableCents, s.PaidCents), 0), xlsxText(w.BankName, 0),
		}
		rows = append(rows, r)
	}
	return buildXLSX([]xlsxSheet{{Name: "结算单", Rows: rows}})
}

func pruneAutoExports(db *gorm.DB, uid uint, base string, now time.Time) {
	days := 30
	if v := userConfig(db, uid, autoExportKeepKey, "30"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			days = n
		}
	}
	if days <= 0 {
		return
	}
	root := filepath.Join(base, "auto")
	cutoff := now.AddDate(0, 0, -days)
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(p)
		}
		return nil
	})
}

// ---- HTTP：手动导出一次 + 目录浏览 ----

func runAutoExportNow(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !ac.isBoss() {
			forbidden(c)
			return
		}
		uid := ac.scopeUser()
		scope := c.Query("scope")
		if scope == "" {
			scope = userConfig(db, uid, autoExportScopeKey, "attendance")
		}
		files, err := exportForUser(db, uid, scope, time.Now())
		if err != nil {
			response.ErrorInternal(c, "导出失败："+err.Error())
			return
		}
		response.Success(c, gin.H{"files": files, "count": len(files)})
	}
}

type dirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// allowedRoots 允许浏览/写入的根目录：配置项优先，否则导出目录与数据目录。
func allowedRoots(db *gorm.DB) []string {
	var roots []string
	if v, err := sysconfig.GetConfig(db, allowedRootsKey, 0); err == nil {
		for _, p := range strings.Split(v, ",") {
			if s := strings.TrimSpace(p); s != "" && filepath.IsAbs(s) {
				roots = append(roots, filepath.Clean(s))
			}
		}
	}
	if len(roots) == 0 {
		if d, err := resolveExportDir(db); err == nil {
			roots = append(roots, d)
		}
		roots = append(roots, exportDirBase())
	}
	return roots
}

// pathWithinRoots 真实路径必须落在某个允许根目录内（防 .. 穿越与符号链接逃逸）。
func pathWithinRoots(p string, roots []string) (string, bool) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", false
	}
	abs = filepath.Clean(abs)
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		real = abs // 目录还不存在（准备新建）时按清理后的绝对路径判断
	}
	for _, r := range roots {
		rAbs, err := filepath.Abs(r)
		if err != nil {
			continue
		}
		rReal := rAbs
		if v, err := filepath.EvalSymlinks(rAbs); err == nil {
			rReal = v
		}
		if real == rReal {
			return real, true
		}
		if strings.HasPrefix(real, rReal+string(os.PathSeparator)) {
			return real, true
		}
	}
	return "", false
}

func fsListDirs(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !loadAccess(db, currentUID(c)).isBoss() {
			forbidden(c)
			return
		}
		roots := allowedRoots(db)
		p := strings.TrimSpace(c.Query("path"))
		if p == "" {
			entries := make([]dirEntry, 0, len(roots))
			for _, r := range roots {
				entries = append(entries, dirEntry{Name: r, Path: r})
			}
			response.Success(c, gin.H{"path": "", "roots": roots, "dirs": entries})
			return
		}
		real, ok := pathWithinRoots(p, roots)
		if !ok {
			response.ErrorBadRequest(c, "该目录不在允许范围内")
			return
		}
		ents, err := os.ReadDir(real)
		if err != nil {
			response.ErrorBadRequest(c, "目录不可读："+err.Error())
			return
		}
		dirs := make([]dirEntry, 0, len(ents))
		for _, e := range ents {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirs = append(dirs, dirEntry{Name: e.Name(), Path: filepath.Join(real, e.Name())})
			}
		}
		sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
		response.Success(c, gin.H{"path": real, "parent": filepath.Dir(real), "roots": roots, "dirs": dirs})
	}
}

func fsMkdir(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !loadAccess(db, currentUID(c)).isBoss() {
			forbidden(c)
			return
		}
		var in struct {
			Path string `json:"path"`
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Name) == "" {
			response.ErrorBadRequest(c, "请填写文件夹名")
			return
		}
		if strings.ContainsAny(in.Name, "/\\:*?\"<>|") || strings.Contains(in.Name, "..") {
			response.ErrorBadRequest(c, "文件夹名不合法")
			return
		}
		base := strings.TrimSpace(in.Path)
		if base == "" {
			response.ErrorBadRequest(c, "缺少父目录")
			return
		}
		real, ok := pathWithinRoots(base, allowedRoots(db))
		if !ok {
			response.ErrorBadRequest(c, "该目录不在允许范围内")
			return
		}
		target := filepath.Join(real, in.Name)
		if _, ok := pathWithinRoots(target, allowedRoots(db)); !ok {
			response.ErrorBadRequest(c, "目标路径越界")
			return
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			response.ErrorInternal(c, "创建文件夹失败："+err.Error())
			return
		}
		response.Success(c, gin.H{"path": target})
	}
}
