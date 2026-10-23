// export_nas.go — 把考勤表导出到 NAS 文件夹（服务端目录）。
//
// 背景（2026-10-23 第三轮粉丝反馈）：导出只下载到浏览器「下载」文件夹，粉丝想要
// 直接把 Excel/图片存到 NAS 的共享文件夹。NAS 场景下应用运行在飞牛主机上，所以
// 「导出到 NAS」= 由服务端把文件写进 data 目录（或管理员配置的绝对路径目录），
// 浏览器通过「导出文件」面板查看、下载、删除。
//
// 本文件提供：
//   - attendanceExportSpec / parseAttendanceExportSpec：xlsx 导出与图片保存共用
//     的时间段+工地+工人参数解析（权限：老板/班组长）；
//   - 导出目录配置（sysconfig worklog.export_dir，默认 <data-dir>/exports）；
//   - 保存 Excel、保存前端生成的图片、列目录、目录信息、下载与删除。
package worklog

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/config"
	"smallgo/server/response"
	"smallgo/server/sysconfig"
	"smallgo/server/utils"
)

// exportDirConfigKey 导出目录的系统配置键（管理员可在「系统设置」修改）。
const exportDirConfigKey = "worklog.export_dir"

// picturesSubdir 图片导出子目录；放在 Excel 之外，避免和表格混在一起。
const picturesSubdir = "pictures"

// maxImageUploadBytes 单张考勤表图片上限（前端 html2canvas 生成，一般 1-3MB）。
const maxImageUploadBytes = 20 << 20

func init() {
	sysconfig.RegisterConfig(sysconfig.ConfigDef{
		Key:         exportDirConfigKey,
		Scope:       sysconfig.ScopeSystem,
		Type:        sysconfig.TypeString,
		Group:       "system",
		Label:       "导出目录",
		Description: "考勤表 Excel/图片在 NAS 上的保存目录（绝对路径）；留空使用数据目录下的 exports",
	})
}

// attendanceExportSpec 考勤表导出参数。
type attendanceExportSpec struct {
	// 网格构建使用（attendance_grid.go）
	dsUID       uint
	start       string
	end         string
	pf          projectFilter
	workerID    uint
	projectName string
	rangeMode   bool
	year        int
	month       int
	// 导出文件名与回显使用
	Start         string
	End           string
	Year          *int
	Month         *int
	ProjectFilter projectFilter
	ProjectName   string
	Worker        *Worker
	WorkerID      uint
	BaseName      string
}

// exportDirBase 数据目录的绝对路径；相对路径按当前工作目录展开。
func exportDirBase() string {
	base := strings.TrimSpace(config.C.DataDir)
	if base == "" {
		base = "data"
	}
	if abs, err := filepath.Abs(base); err == nil {
		return abs
	}
	return base
}

// defaultExportDir 未配置时的默认导出目录：<data-dir>/exports。
func defaultExportDir() string {
	return filepath.Join(exportDirBase(), "exports")
}

// resolveExportDir 解析并创建导出目录。必须是绝对路径，避免工作目录变化后
// 文件散落到不可预期的位置；目录不可用时返回错误，由接口转成明确提示。
func resolveExportDir(db *gorm.DB) (string, error) {
	configured := ""
	if db != nil {
		if v, err := sysconfig.GetConfig(db, exportDirConfigKey, 0); err == nil {
			configured = strings.TrimSpace(v)
		}
	}
	if configured == "" {
		configured = defaultExportDir()
	}
	if !filepath.IsAbs(configured) {
		return "", fmt.Errorf("导出目录必须是绝对路径：%s", configured)
	}
	dir := filepath.Clean(configured)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建导出目录失败：%w", err)
	}
	return dir, nil
}

// safeFilePart 清理文件名片段：去掉路径分隔符、Windows 非法字符与控制字符，
// 截断到 60 个字符，防止从工地名/工人名注入目录层级。
func safeFilePart(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			continue
		}
		if r < 0x20 {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.Trim(b.String(), ". ")
	if runes := []rune(out); len(runes) > 60 {
		out = string(runes[:60])
	}
	return out
}

// attendanceExportBaseName 导出文件名主干：考勤表-起至止[-工地][-工人]，
// Excel/图片共用，扩展名与时间戳由调用方补上。
func attendanceExportBaseName(spec *attendanceExportSpec) string {
	name := fmt.Sprintf("考勤表-%s至%s", spec.Start, spec.End)
	if spec.ProjectName != "" && spec.ProjectName != "全部工地" {
		name += "-" + spec.ProjectName
	}
	if spec.Worker != nil {
		name += "-" + spec.Worker.Name
	}
	if clean := safeFilePart(name); clean != "" {
		return clean
	}
	return "考勤表"
}

// parseAttendanceExportSpec 解析 GET 查询参数（year/month 或 start/end + project_id
// + worker_id），返回 (spec, 0, "") 表示成功；否则 code/msg 说明失败原因。
func parseAttendanceExportSpec(c *gin.Context, db *gorm.DB) (*attendanceExportSpec, int, string) {
	ac := loadAccess(db, currentUID(c))
	dsUID := ac.scopeUser()
	// 员工只能导出自己的考勤；未绑定工人档案时 fail closed（与 CSV 导出口径一致）。
	forcedWorker := 0
	if sw, scoped := ac.workerScope(); scoped {
		if ac.workerUnbound() {
			return nil, http.StatusForbidden, "未绑定工人档案，请联系班组长"
		}
		forcedWorker = int(sw)
	}

	spec := &attendanceExportSpec{}
	start, end := c.Query("start"), c.Query("end")
	if strings.TrimSpace(start) != "" || strings.TrimSpace(end) != "" {
		s, e, ok := parseDateRange(c)
		if !ok {
			return nil, http.StatusBadRequest, "时间段不正确：请提供 start/end（YYYY-MM-DD），起止顺序正确且不超过 731 天"
		}
		spec.Start, spec.End = s, e
		spec.rangeMode = true
	} else {
		now := time.Now()
		year := atoiDefault(c.Query("year"), now.Year())
		month := atoiDefault(c.Query("month"), int(now.Month()))
		if month < 1 || month > 12 {
			return nil, http.StatusBadRequest, "月份不正确"
		}
		spec.Start, spec.End = monthRange(year, month)
		spec.Year, spec.Month = &year, &month
		spec.year, spec.month = year, month
	}

	pf, ok := parseProjectFilter(c)
	if !ok {
		return nil, http.StatusBadRequest, "project_id 参数不正确"
	}
	spec.ProjectFilter = pf
	spec.dsUID = dsUID
	spec.pf = pf
	spec.start, spec.end = spec.Start, spec.End
	name, err := pf.resolveProjectName(db, dsUID)
	if errors.Is(err, errProjectNotFound) {
		return nil, http.StatusBadRequest, "工地不存在"
	}
	if err != nil {
		return nil, http.StatusInternalServerError, "查询工地失败"
	}
	spec.ProjectName = name
	spec.projectName = name

	workerID := forcedWorker
	if workerID == 0 {
		if raw := strings.TrimSpace(c.Query("worker_id")); raw != "" {
			id := utils.Atoi(raw, 0)
			if id <= 0 {
				return nil, http.StatusBadRequest, "worker_id 参数不正确"
			}
			workerID = id
		}
	}
	if workerID > 0 {
		var w Worker
		if err := db.Where("user_id = ? AND id = ?", dsUID, workerID).First(&w).Error; err != nil {
			return nil, http.StatusBadRequest, "工人不存在"
		}
		spec.Worker = &w
		spec.WorkerID = w.ID
		spec.workerID = w.ID
	}

	spec.BaseName = attendanceExportBaseName(spec)
	return spec, 0, ""
}

// writeSpecError 把 parseAttendanceExportSpec 的错误码转成统一响应。
func writeSpecError(c *gin.Context, code int, msg string) {
	switch code {
	case http.StatusForbidden:
		forbidden(c)
	case http.StatusInternalServerError:
		response.ErrorInternal(c, msg)
	default:
		response.ErrorBadRequest(c, msg)
	}
}

// saveExportFile 把数据原子写入目录并返回最终文件名（同名自动加 -2、-3）。
// 先写临时文件再 rename，避免 NAS 上写入中断留下半个文件。
func saveExportFile(dir, name string, data []byte) (string, error) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	ext := filepath.Ext(name)
	cleanBase := safeFilePart(base)
	if cleanBase == "" {
		cleanBase = "导出文件"
	}
	if ext == "" {
		ext = ".bin"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	final := filepath.Join(dir, cleanBase+ext)
	for i := 2; i <= 999; i++ {
		if _, err := os.Stat(final); errors.Is(err, os.ErrNotExist) {
			break
		}
		final = filepath.Join(dir, fmt.Sprintf("%s-%d%s", cleanBase, i, ext))
	}
	tmp, err := os.CreateTemp(dir, ".export-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		cleanup()
		return "", err
	}
	if err := os.Rename(tmpName, final); err != nil {
		cleanup()
		return "", err
	}
	return filepath.Base(final), nil
}

// exportAttendanceXLSX 生成并下载考勤表 Excel（不落盘）。
func exportAttendanceXLSX(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		spec, code, msg := parseAttendanceExportSpec(c, db)
		if spec == nil {
			writeSpecError(c, code, msg)
			return
		}
		data, err := buildAttendanceXLSX(db, spec)
		if err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		utils.WriteBytes(c, utils.ExportFilename(spec.BaseName, "xlsx"),
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
	}
}

// saveNasAttendanceXLSX 生成考勤表 Excel 并保存到 NAS 导出目录。
func saveNasAttendanceXLSX(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 存到 NAS 是共享目录操作，仅老板/班组长可用；普通员工只保留浏览器下载。
		if !loadAccess(db, currentUID(c)).canManage() {
			forbidden(c)
			return
		}
		spec, code, msg := parseAttendanceExportSpec(c, db)
		if spec == nil {
			writeSpecError(c, code, msg)
			return
		}
		data, err := buildAttendanceXLSX(db, spec)
		if err != nil {
			response.ErrorInternal(c, "生成 Excel 失败")
			return
		}
		dir, err := resolveExportDir(db)
		if err != nil {
			response.ErrorInternal(c, "导出目录不可用："+err.Error())
			return
		}
		name, err := saveExportFile(dir, utils.ExportFilename(spec.BaseName, "xlsx"), data)
		if err != nil {
			response.ErrorInternal(c, "保存失败："+err.Error())
			return
		}
		response.Success(c, gin.H{"file": name, "dir": dir, "size": len(data), "kind": "xlsx"})
	}
}

// saveNasAttendanceImage 保存前端生成的考勤表图片（PNG/JPEG）到 NAS 导出目录。
// 图片由浏览器 html2canvas 渲染后上传，服务端只校验类型与大小。
func saveNasAttendanceImage(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !ac.canManage() {
			forbidden(c)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImageUploadBytes+512*1024)
		file, err := c.FormFile("file")
		if err != nil {
			response.ErrorBadRequest(c, "请选择要保存的图片")
			return
		}
		if file.Size <= 0 {
			response.ErrorBadRequest(c, "图片内容为空")
			return
		}
		if file.Size > maxImageUploadBytes {
			response.ErrorBadRequest(c, "图片大小不能超过 20MB")
			return
		}
		f, err := file.Open()
		if err != nil {
			response.ErrorInternal(c, "读取图片失败")
			return
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxImageUploadBytes+1))
		if err != nil {
			response.ErrorInternal(c, "读取图片失败")
			return
		}
		if len(data) == 0 {
			response.ErrorBadRequest(c, "图片内容为空")
			return
		}
		if len(data) > maxImageUploadBytes {
			response.ErrorBadRequest(c, "图片大小不能超过 20MB")
			return
		}
		head := data
		if len(head) > 512 {
			head = head[:512]
		}
		ext := ""
		switch http.DetectContentType(head) {
		case "image/png":
			ext = "png"
		case "image/jpeg":
			ext = "jpg"
		default:
			response.ErrorBadRequest(c, "只支持 PNG 或 JPEG 图片")
			return
		}

		nameBase := "考勤表图片"
		start := strings.TrimSpace(c.PostForm("start"))
		end := strings.TrimSpace(c.PostForm("end"))
		if validDate(start) && validDate(end) {
			nameBase = fmt.Sprintf("考勤表-%s至%s", start, end)
		}
		if project := strings.TrimSpace(c.PostForm("project_name")); project != "" && project != "全部工地" {
			nameBase += "-" + project
		}
		if worker := strings.TrimSpace(c.PostForm("worker_name")); worker != "" {
			nameBase += "-" + worker
		}
		dir, err := resolveExportDir(db)
		if err != nil {
			response.ErrorInternal(c, "导出目录不可用："+err.Error())
			return
		}
		picDir := filepath.Join(dir, picturesSubdir)
		name, err := saveExportFile(picDir, utils.ExportFilename(safeFilePart(nameBase), ext), data)
		if err != nil {
			response.ErrorInternal(c, "保存失败："+err.Error())
			return
		}
		response.Success(c, gin.H{"file": name, "dir": picDir, "size": len(data), "kind": "image"})
	}
}

// nasExportFile 「导出文件」列表中的一项。
type nasExportFile struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	Kind     string `json:"kind"` // xlsx / image
	URL      string `json:"url"`
}

// scanExportFiles 扫描导出目录（及 pictures 子目录）里的导出文件。
func scanExportFiles(dir, forceKind string) []nasExportFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]nasExportFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".tmp") {
			continue
		}
		kind := forceKind
		if kind == "" {
			switch strings.ToLower(filepath.Ext(name)) {
			case ".xlsx":
				kind = "xlsx"
			case ".png", ".jpg", ".jpeg":
				kind = "image"
			default:
				continue
			}
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		rel := name
		if forceKind == "image" {
			rel = picturesSubdir + "/" + name
		}
		out = append(out, nasExportFile{
			Name:     rel,
			Size:     info.Size(),
			Modified: info.ModTime().Unix(),
			Kind:     kind,
			URL:      "/api/worklog/exports/download?name=" + url.QueryEscape(rel),
		})
	}
	return out
}

// nasExportList 列出已导出到 NAS 的文件，供前端「导出文件」面板使用。
func nasExportList(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		out := gin.H{"enabled": ac.canManage(), "files": []nasExportFile{}, "dir": ""}
		if !ac.canManage() {
			response.Success(c, out)
			return
		}
		dir, err := resolveExportDir(db)
		if err != nil {
			out["error"] = err.Error()
			response.Success(c, out)
			return
		}
		files := scanExportFiles(dir, "")
		files = append(files, scanExportFiles(filepath.Join(dir, picturesSubdir), "image")...)
		sort.Slice(files, func(i, j int) bool { return files[i].Modified > files[j].Modified })
		const maxListed = 100
		if len(files) > maxListed {
			files = files[:maxListed]
		}
		out["dir"] = dir
		out["files"] = files
		response.Success(c, out)
	}
}

// nasExportInfo 返回导出目录信息（目录、文件数）。
func nasExportInfo(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !ac.canManage() {
			forbidden(c)
			return
		}
		out := gin.H{"enabled": true, "default_dir": defaultExportDir()}
		dir, err := resolveExportDir(db)
		if err != nil {
			out["error"] = err.Error()
			response.Success(c, out)
			return
		}
		count := 0
		if entries, err := os.ReadDir(dir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
					count++
				}
			}
		}
		if entries, err := os.ReadDir(filepath.Join(dir, picturesSubdir)); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
					count++
				}
			}
		}
		out["dir"] = dir
		out["file_count"] = count
		response.Success(c, out)
	}
}

// safeExportRelName 校验列表/下载/删除接口传入的文件名：只允许
// "文件名" 或 "pictures/文件名" 两种形式，不允许路径穿越。
func safeExportRelName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 || strings.ContainsAny(name, "\\\x00") {
		return "", false
	}
	if strings.Contains(name, "/") {
		parts := strings.Split(name, "/")
		if len(parts) != 2 || parts[0] != picturesSubdir {
			return "", false
		}
		name = parts[1]
	}
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return "", false
	}
	if filepath.Base(name) != name {
		return "", false
	}
	return name, true
}

// resolveExportFile 把安全相对名解析成绝对路径，并确认仍在导出目录内。
func resolveExportFile(dir, rel string) (string, error) {
	clean, ok := safeExportRelName(rel)
	if !ok {
		return "", errors.New("文件名不合法")
	}
	path := filepath.Join(dir, filepath.FromSlash(clean))
	if strings.HasPrefix(rel, picturesSubdir+"/") {
		path = filepath.Join(dir, picturesSubdir, clean)
	}
	inside, err := filepath.Rel(dir, path)
	if err != nil || strings.HasPrefix(inside, "..") || filepath.IsAbs(inside) {
		return "", errors.New("文件名不合法")
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return "", errors.New("文件不存在")
	}
	return path, nil
}

// nasExportDownload 下载 NAS 上已保存的导出文件。
func nasExportDownload(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !ac.canManage() {
			forbidden(c)
			return
		}
		dir, err := resolveExportDir(db)
		if err != nil {
			response.ErrorInternal(c, "导出目录不可用")
			return
		}
		path, err := resolveExportFile(dir, c.Query("name"))
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		c.FileAttachment(path, filepath.Base(path))
	}
}

// nasExportRemove 删除 NAS 上的导出文件（支持一次传多个 name）。
func nasExportRemove(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !ac.canManage() {
			forbidden(c)
			return
		}
		var in struct {
			Name  string   `json:"name"`
			Names []string `json:"names"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		names := in.Names
		if in.Name != "" {
			names = append(names, in.Name)
		}
		if len(names) == 0 {
			response.ErrorBadRequest(c, "请选择要删除的文件")
			return
		}
		dir, err := resolveExportDir(db)
		if err != nil {
			response.ErrorInternal(c, "导出目录不可用")
			return
		}
		removed := make([]string, 0, len(names))
		failed := make([]string, 0, len(names))
		for _, raw := range names {
			path, err := resolveExportFile(dir, raw)
			if err != nil {
				failed = append(failed, raw)
				continue
			}
			if err := os.Remove(path); err != nil {
				failed = append(failed, raw)
				continue
			}
			removed = append(removed, raw)
		}
		response.Success(c, gin.H{"removed": removed, "failed": failed})
	}
}
