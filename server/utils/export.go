// Package utils — export.go provides download helpers for handlers that
// expose CSV/JSON exports of their data (audit logs, app records, reports…).
// They set the browser-facing Content-Disposition header so GET endpoints can
// hand the response straight to a file download.
package utils

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
)

// contentDisposition builds an RFC 5987 header value so non-ASCII (e.g.
// Chinese) filenames download correctly in modern browsers.
func contentDisposition(filename string) string {
	ascii := filename
	if len(ascii) > 60 {
		ascii = ascii[:60]
	}
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		ascii, url.PathEscape(filename))
}

// WriteCSV streams rows (first row = header) to the client as a downloadable
// CSV file with a UTF-8 BOM so Excel opens Chinese text correctly.
//
//	utils.WriteCSV(c, "notes_2026.csv", [][]string{{"ID", "标题"}, {"1", "hello"}})
func WriteCSV(c *gin.Context, filename string, rows [][]string) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", contentDisposition(filename))
	c.Status(http.StatusOK)

	// UTF-8 BOM keeps Excel from mojibake-ing CJK cells.
	c.Writer.WriteString("\xEF\xBB\xBF")
	w := csv.NewWriter(c.Writer)
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return
		}
	}
	w.Flush()
}

// WriteJSONFile streams an arbitrary JSON value as a downloadable file —
// the counterpart of WriteCSV for structured backups/exports.
func WriteJSONFile(c *gin.Context, filename string, data interface{}) error {
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Header("Content-Disposition", contentDisposition(filename))
	c.Data(http.StatusOK, "application/json; charset=utf-8", payload)
	return nil
}

// ExportFilename returns "name_YYYYMMDD_HHMMSS.ext" so repeated exports never
// collide in the browser's download folder.
func ExportFilename(name, ext string) string {
	stamp := time.Now().Format("20060102_150405")
	return name + "_" + stamp + "." + ext
}
