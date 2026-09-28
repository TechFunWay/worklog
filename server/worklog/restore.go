package worklog

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"smallgo/server/config"
	"smallgo/server/logger"
	"smallgo/server/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// restoreDB 管理员上传 .db 备份恢复全库，与 exportDB 的快照下载对称。
// 流程：校验备份 → 当前库先 VACUUM 一份快照 → 停连接、替换数据库文件 →
// 重启进程。重启后 AutoMigrate / RunUpgrades 会把旧版本备份补齐到当前结构。
// 整个过程要求替换文件后重启生效，因此以先应答、后台延迟换文件再重启的方式执行。
func restoreDB(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			response.ErrorBadRequest(c, "请选择要恢复的 .db 备份文件")
			return
		}
		if file.Size > 512<<20 {
			response.ErrorBadRequest(c, "备份文件过大（上限 512MB）")
			return
		}

		dbPath := config.C.DBPath
		if dbPath == "" {
			response.ErrorInternal(c, "未配置数据库路径")
			return
		}

		tmpDir, err := os.MkdirTemp("", "worklog-restore-*")
		if err != nil {
			response.ErrorInternal(c, "恢复失败")
			return
		}
		stagePath := filepath.Join(tmpDir, "restore.db")
		if err := c.SaveUploadedFile(file, stagePath); err != nil {
			os.RemoveAll(tmpDir)
			response.ErrorInternal(c, "保存上传文件失败")
			return
		}

		if err := validateBackupDB(stagePath); err != nil {
			os.RemoveAll(tmpDir)
			response.ErrorBadRequest(c, err.Error())
			return
		}

		// 恢复前把当前数据库做成一份完整快照留在同目录，误恢复时还能救回来。
		sqlDB, err := db.DB()
		if err != nil {
			os.RemoveAll(tmpDir)
			response.ErrorInternal(c, "恢复失败")
			return
		}
		prePath := fmt.Sprintf("%s.pre-restore-%s", dbPath, time.Now().Format("20060102-150405"))
		if _, err := sqlDB.Exec("VACUUM INTO ?", prePath); err != nil {
			os.RemoveAll(tmpDir)
			response.ErrorInternal(c, "备份当前数据失败，已中止恢复")
			return
		}

		go func() {
			// 留出时间让 HTTP 响应先写回客户端，再动数据库与进程。
			time.Sleep(800 * time.Millisecond)
			sqlDB.Close()
			os.Remove(dbPath + "-wal")
			os.Remove(dbPath + "-shm")
			if err := replaceFile(stagePath, dbPath); err != nil {
				logger.Error("恢复数据库：替换数据库文件失败: %v", err)
				return
			}
			os.RemoveAll(tmpDir)
			logger.Info("数据库已从备份恢复（恢复前快照: %s），正在重启服务", filepath.Base(prePath))
			if err := restartProcess(); err != nil {
				logger.Error("自动重启失败，数据已就位，请手动重启服务: %v", err)
			}
		}()

		response.Success(c, gin.H{
			"message":            "备份校验通过，服务正在恢复数据并重启，页面将自动刷新",
			"pre_restore_backup": filepath.Base(prePath),
		})
	}
}

// validateBackupDB 只读打开上传的备份做基本校验：是合法 SQLite、结构完好、
// 且带本应用的用户数据（防止拿错文件把库换空）。
func validateBackupDB(path string) error {
	vdb, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		return errors.New("不是有效的 SQLite 数据库文件")
	}
	defer vdb.Close()

	var check string
	if err := vdb.QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		return errors.New("备份文件已损坏，无法用于恢复")
	}
	var users int
	if err := vdb.QueryRow("SELECT count(*) FROM users").Scan(&users); err != nil || users < 1 {
		return errors.New("备份中缺少用户数据，不是本应用的完整备份")
	}
	return nil
}

// replaceFile 把恢复文件挪到数据库路径；跨设备 rename 失败时退回复制。
func replaceFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}
