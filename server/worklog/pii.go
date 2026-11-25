// pii.go — 工人敏感信息（身份证号、银行卡号）的加密存储与脱敏。
//
// 这两个字段都是选填：不填不影响任何功能。填了就必须按敏感信息处理：
//   - 落库前 AES-256-GCM 加密，密钥保存在 system_configs（internal，绝不外发）；
//   - 列表与详情接口只下发掩码（身份证前6后4、银行卡后4），完整值只给老板/管理员；
//   - 任何日志、导出默认不带明文，导出「工资表（含卡号）」需显式请求且有权限。
package worklog

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
	"smallgo/server/sysconfig"
)

const piiKeyConfigKey = "worklog.pii_key"

func init() {
	sysconfig.RegisterConfig(sysconfig.ConfigDef{
		Key: piiKeyConfigKey, Scope: sysconfig.ScopeSystem, Type: sysconfig.TypeString,
		Internal: true, Group: "internal", Label: "敏感信息加密密钥",
		Description: "工人身份证/银行卡的加密密钥，自动生成，切勿泄露；丢失后已存数据无法解密",
	})
}

// getPIIKey 取（或首次生成）32 字节密钥。密钥只在服务端使用，任何接口都不下发。
func getPIIKey(db *gorm.DB) ([]byte, error) {
	s, err := sysconfig.GetConfig(db, piiKeyConfigKey, 0)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// 首次使用时该键还不存在，GetConfig 会回 ErrRecordNotFound —— 视为空，走下面的生成分支。
	if s != "" {
		b, e := base64.StdEncoding.DecodeString(s)
		if e == nil && len(b) == 32 {
			return b, nil
		}
	}
	buf := make([]byte, 32)
	if _, e := rand.Read(buf); e != nil {
		return nil, e
	}
	val := base64.StdEncoding.EncodeToString(buf)
	if e := sysconfig.UpdateConfig(db, piiKeyConfigKey, val, 0); e != nil {
		return nil, e
	}
	return buf, nil
}

func piiEncrypt(db *gorm.DB, plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", nil
	}
	key, err := getPIIKey(db)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func piiDecrypt(db *gorm.DB, enc string) (string, error) {
	if strings.TrimSpace(enc) == "" {
		return "", nil
	}
	key, err := getPIIKey(db)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", fmt.Errorf("密文格式错误")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("密文长度不足")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("解密失败")
	}
	return string(plain), nil
}

// maskIDCard 身份证脱敏：保留前 6 后 4，中间打码（够核对、不足以复原）。
func maskIDCard(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= 10 {
		return strings.Repeat("*", len(r))
	}
	return string(r[:6]) + strings.Repeat("*", len(r)-10) + string(r[len(r)-4:])
}

// maskBankCard 银行卡脱敏：只保留后 4 位。
func maskBankCard(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= 4 {
		return strings.Repeat("*", len(r))
	}
	return "**** **** **** " + string(r[len(r)-4:])
}

var (
	bankCardRe = regexp.MustCompile(`^\d{12,19}$`)
	idCardRe   = regexp.MustCompile(`^\d{17}[\dXx]$`)
)

// normalizeBankCard 去掉空格与短横线，返回纯数字。
func normalizeBankCard(s string) string {
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	return strings.TrimSpace(s)
}

func validateBankCard(s string) error {
	s = normalizeBankCard(s)
	if s == "" {
		return nil // 选填
	}
	if !bankCardRe.MatchString(s) {
		return fmt.Errorf("银行卡号应为 12-19 位数字")
	}
	return nil
}

func validateIDCard(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil // 选填
	}
	if !idCardRe.MatchString(s) {
		return fmt.Errorf("身份证号应为 18 位（末位可为 X）")
	}
	return nil
}

// fillWorkerPIIMasks 给工人列表/详情补掩码展示字段（不改变加密列）。
func fillWorkerPIIMasks(db *gorm.DB, ws []Worker) {
	for i := range ws {
		if ws[i].IDCardEnc != "" {
			if p, err := piiDecrypt(db, ws[i].IDCardEnc); err == nil {
				ws[i].IDCardMasked = maskIDCard(p)
				ws[i].IDCardSet = true
			}
		}
		if ws[i].BankCardEnc != "" {
			if p, err := piiDecrypt(db, ws[i].BankCardEnc); err == nil {
				ws[i].BankCardMasked = maskBankCard(p)
				ws[i].BankCardSet = true
			}
		}
	}
}
