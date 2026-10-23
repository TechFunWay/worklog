// xlsx.go — 用标准库生成最小可用的 .xlsx（OOXML）。
//
// 粉丝反馈要「Excel 工资条形式」导出，但 server 依赖里没有 excelize，也不
// 希望为导出功能引入新依赖，因此这里用 archive/zip 直接拼 OOXML。字符串一律
// 用 inlineStr 写入，Excel 不会把 `=...` 当公式执行（同时也避免 CSV 注入类问题）；
// 数字写 <v> 保留数值类型，金额用样式 164（#,##0.00）显示两位小数。
package worklog

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// xlsxCell 一个单元格。Number 非空按数字写，否则按文本写。
type xlsxCell struct {
	Text   string
	Number *float64
	Style  int
}

func xlsxText(s string, style int) xlsxCell {
	return xlsxCell{Text: s, Style: style}
}

func xlsxNum(v float64, style int) xlsxCell {
	return xlsxCell{Number: &v, Style: style}
}

// xlsxSheet 一个工作表。
type xlsxSheet struct {
	Name       string
	Cols       []float64 // 每列宽度（字符数），从 A 列开始
	Rows       [][]xlsxCell
	Merges     []string // 例："A1:H1"
	FreezeRows int      // 冻结行数
	FreezeCols int      // 冻结列数
}

// xlsx 样式索引（与下方 stylesXML 的 cellXfs 顺序一致）。
const (
	xlsxStyleDefault    = 0
	xlsxStyleTitle      = 1
	xlsxStyleHeader     = 2
	xlsxStyleBody       = 3
	xlsxStyleMoney      = 4
	xlsxStyleTotal      = 5
	xlsxStyleTotalMoney = 6
	xlsxStyleBodyLeft   = 7
	xlsxStyleInfo       = 8
)

const xlsxXMLHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

// buildXLSX 组装 .xlsx 字节。sheets 至少一个。
func buildXLSX(sheets []xlsxSheet) ([]byte, error) {
	if len(sheets) == 0 {
		return nil, fmt.Errorf("xlsx: 至少需要一个工作表")
	}
	names := make([]string, 0, len(sheets))
	used := make(map[string]bool, len(sheets))
	for i, sh := range sheets {
		name := sanitizeSheetName(sh.Name)
		if name == "" {
			name = "Sheet" + strconv.Itoa(i+1)
		}
		base := name
		for n := 2; used[name]; n++ {
			name = base + "-" + strconv.Itoa(n)
		}
		used[name] = true
		names = append(names, name)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}

	if err := add("[Content_Types].xml", []byte(contentTypesXML(len(sheets)))); err != nil {
		return nil, err
	}
	if err := add("_rels/.rels", []byte(rootRelsXML)); err != nil {
		return nil, err
	}
	if err := add("docProps/core.xml", []byte(corePropsXML)); err != nil {
		return nil, err
	}
	if err := add("docProps/app.xml", []byte(appPropsXML(len(sheets)))); err != nil {
		return nil, err
	}
	if err := add("xl/workbook.xml", []byte(workbookXML(names))); err != nil {
		return nil, err
	}
	if err := add("xl/_rels/workbook.xml.rels", []byte(workbookRelsXML(len(sheets)))); err != nil {
		return nil, err
	}
	if err := add("xl/styles.xml", []byte(stylesXML)); err != nil {
		return nil, err
	}
	for i, sh := range sheets {
		if err := add(fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), xlsxSheetXML(sh)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func contentTypesXML(sheetCount int) string {
	var b strings.Builder
	b.WriteString(xlsxXMLHeader)
	b.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	b.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	b.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	b.WriteString(`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`)
	b.WriteString(`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>`)
	for i := 1; i <= sheetCount; i++ {
		b.WriteString(fmt.Sprintf(`<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, i))
	}
	b.WriteString(`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>`)
	b.WriteString(`<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>`)
	b.WriteString(`</Types>`)
	return b.String()
}

const rootRelsXML = xlsxXMLHeader +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
	`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
	`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>` +
	`</Relationships>`

const corePropsXML = xlsxXMLHeader +
	`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
	`<dc:creator>工记</dc:creator><cp:lastModifiedBy>工记</cp:lastModifiedBy>` +
	`<dcterms:created xsi:type="dcterms:W3CDTF">2026-01-01T00:00:00Z</dcterms:created>` +
	`<dcterms:modified xsi:type="dcterms:W3CDTF">2026-01-01T00:00:00Z</dcterms:modified>` +
	`</cp:coreProperties>`

func appPropsXML(sheetCount int) string {
	return xlsxXMLHeader +
		`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">` +
		`<Application>工记</Application><DocSecurity>0</DocSecurity><ScaleCrop>false</ScaleCrop>` +
		`</Properties>`
}

func workbookXML(names []string) string {
	var b strings.Builder
	b.WriteString(xlsxXMLHeader)
	b.WriteString(`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)
	b.WriteString(`<bookViews><workbookView xWindow="0" yWindow="0" windowWidth="20480" windowHeight="12000"/></bookViews>`)
	b.WriteString(`<sheets>`)
	for i, name := range names {
		b.WriteString(fmt.Sprintf(`<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, xlsxEscape(name), i+1, i+1))
	}
	b.WriteString(`</sheets>`)
	b.WriteString(`<calcPr calcId="191029"/>`)
	b.WriteString(`</workbook>`)
	return b.String()
}

func workbookRelsXML(sheetCount int) string {
	var b strings.Builder
	b.WriteString(xlsxXMLHeader)
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i := 1; i <= sheetCount; i++ {
		b.WriteString(fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, i, i))
	}
	b.WriteString(fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`, sheetCount+1))
	b.WriteString(`</Relationships>`)
	return b.String()
}

// stylesXML 固定样式表，索引与 xlsxStyle* 常量对应。
const stylesXML = xlsxXMLHeader +
	`<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
	`<numFmts count="1"><numFmt numFmtId="164" formatCode="#,##0.00"/></numFmts>` +
	`<fonts count="5">` +
	`<font><sz val="11"/><name val="等线"/><family val="2"/></font>` +
	`<font><b/><sz val="16"/><name val="等线"/><family val="2"/></font>` +
	`<font><b/><sz val="11"/><color rgb="FFFFFFFF"/><name val="等线"/><family val="2"/></font>` +
	`<font><b/><sz val="11"/><name val="等线"/><family val="2"/></font>` +
	`<font><sz val="9"/><color rgb="FF8A8A8A"/><name val="等线"/><family val="2"/></font>` +
	`</fonts>` +
	`<fills count="4"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill>` +
	`<fill><patternFill patternType="solid"><fgColor rgb="FF0EA5E9"/><bgColor indexed="64"/></patternFill></fill>` +
	`<fill><patternFill patternType="solid"><fgColor rgb="FFEFF9FE"/><bgColor indexed="64"/></patternFill></fill></fills>` +
	`<borders count="2"><border><left/><right/><top/><bottom/><diagonal/></border>` +
	`<border><left style="thin"><color rgb="FFB6C3CE"/></left><right style="thin"><color rgb="FFB6C3CE"/></right>` +
	`<top style="thin"><color rgb="FFB6C3CE"/></top><bottom style="thin"><color rgb="FFB6C3CE"/></bottom><diagonal/></border></borders>` +
	`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
	`<cellXfs count="9">` +
	`<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>` +
	`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyAlignment="1"><alignment horizontal="center" vertical="center"/></xf>` +
	`<xf numFmtId="0" fontId="2" fillId="2" borderId="1" xfId="0" applyFont="1" applyFill="1" applyBorder="1" applyAlignment="1"><alignment horizontal="center" vertical="center" wrapText="1"/></xf>` +
	`<xf numFmtId="0" fontId="0" fillId="0" borderId="1" xfId="0" applyBorder="1" applyAlignment="1"><alignment horizontal="center" vertical="center" wrapText="1"/></xf>` +
	`<xf numFmtId="164" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1" applyAlignment="1"><alignment horizontal="right" vertical="center"/></xf>` +
	`<xf numFmtId="0" fontId="3" fillId="3" borderId="1" xfId="0" applyFont="1" applyFill="1" applyBorder="1" applyAlignment="1"><alignment horizontal="center" vertical="center"/></xf>` +
	`<xf numFmtId="164" fontId="3" fillId="3" borderId="1" xfId="0" applyNumberFormat="1" applyFont="1" applyFill="1" applyBorder="1" applyAlignment="1"><alignment horizontal="right" vertical="center"/></xf>` +
	`<xf numFmtId="0" fontId="0" fillId="0" borderId="1" xfId="0" applyBorder="1" applyAlignment="1"><alignment horizontal="left" vertical="center" wrapText="1"/></xf>` +
	`<xf numFmtId="0" fontId="4" fillId="0" borderId="0" xfId="0" applyFont="1" applyAlignment="1"><alignment horizontal="left" vertical="center"/></xf>` +
	`</cellXfs>` +
	`<cellStyles count="1"><cellStyle name="常规" xfId="0" builtinId="0"/></cellStyles>` +
	`<dxfs count="0"/><tableStyles count="0" defaultTableStyle="TableStyleMedium2" defaultPivotStyle="PivotStyleLight16"/>` +
	`</styleSheet>`

// xlsxSheetXML 生成单个工作表 XML。
func xlsxSheetXML(sh xlsxSheet) []byte {
	maxCols := 0
	for _, row := range sh.Rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	lastCol := "A"
	if maxCols > 0 {
		lastCol = xlsxColName(maxCols)
	}
	dimension := "A1"
	if len(sh.Rows) > 0 && maxCols > 0 {
		dimension = "A1:" + lastCol + strconv.Itoa(len(sh.Rows))
	}

	var b strings.Builder
	b.WriteString(xlsxXMLHeader)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	b.WriteString(`<dimension ref="` + dimension + `"/>`)
	if sh.FreezeRows > 0 || sh.FreezeCols > 0 {
		topLeft := xlsxRef(sh.FreezeCols+1, sh.FreezeRows+1)
		b.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane`)
		if sh.FreezeCols > 0 {
			b.WriteString(` xSplit="` + strconv.Itoa(sh.FreezeCols) + `"`)
		}
		if sh.FreezeRows > 0 {
			b.WriteString(` ySplit="` + strconv.Itoa(sh.FreezeRows) + `"`)
		}
		b.WriteString(` topLeftCell="` + topLeft + `" activePane="bottomRight" state="frozen"/>`)
		b.WriteString(`<selection pane="bottomRight" activeCell="` + topLeft + `" sqref="` + topLeft + `"/></sheetView></sheetViews>`)
	} else {
		b.WriteString(`<sheetViews><sheetView workbookViewId="0"/></sheetViews>`)
	}
	b.WriteString(`<sheetFormatPr defaultColWidth="9" defaultRowHeight="20"/>`)
	if len(sh.Cols) > 0 {
		b.WriteString(`<cols>`)
		for i, width := range sh.Cols {
			if width <= 0 {
				continue
			}
			b.WriteString(fmt.Sprintf(`<col min="%d" max="%d" width="%s" customWidth="1"/>`, i+1, i+1, strconv.FormatFloat(width, 'f', -1, 64)))
		}
		b.WriteString(`</cols>`)
	}
	b.WriteString(`<sheetData>`)
	for ri, row := range sh.Rows {
		b.WriteString(`<row r="` + strconv.Itoa(ri+1) + `">`)
		for ci, cell := range row {
			if cell.Number == nil && cell.Text == "" {
				continue
			}
			ref := xlsxRef(ci+1, ri+1)
			b.WriteString(`<c r="` + ref + `"`)
			if cell.Style > 0 {
				b.WriteString(` s="` + strconv.Itoa(cell.Style) + `"`)
			}
			if cell.Number != nil {
				b.WriteString(`><v>` + xlsxNumber(*cell.Number) + `</v></c>`)
			} else {
				b.WriteString(` t="inlineStr"><is><t xml:space="preserve">` + xlsxEscape(cell.Text) + `</t></is></c>`)
			}
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData>`)
	if len(sh.Merges) > 0 {
		b.WriteString(`<mergeCells count="` + strconv.Itoa(len(sh.Merges)) + `">`)
		for _, m := range sh.Merges {
			b.WriteString(`<mergeCell ref="` + m + `"/>`)
		}
		b.WriteString(`</mergeCells>`)
	}
	b.WriteString(`</worksheet>`)
	return []byte(b.String())
}

func xlsxColName(n int) string {
	if n < 1 {
		n = 1
	}
	name := ""
	for n > 0 {
		n--
		name = string(rune('A'+n%26)) + name
		n /= 26
	}
	return name
}

func xlsxRef(col, row int) string {
	return xlsxColName(col) + strconv.Itoa(row)
}

func xlsxNumber(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if s == "-0" {
		s = "0"
	}
	return s
}

func sanitizeSheetName(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '[', ']', ':', '*', '?', '/', '\\':
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if len(out) > 31 {
		out = out[:31]
	}
	return out
}

// xlsxEscape 转义 XML 文本，并去掉控制字符（Excel 对非法字符会报文件损坏）。
func xlsxEscape(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			valid := r == '\t' || r == '\n' || r == '\r' ||
				(r >= 0x20 && r <= 0xD7FF) ||
				(r >= 0xE000 && r <= 0xFFFD) ||
				(r >= 0x10000 && r <= 0x10FFFF)
			if valid {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}
