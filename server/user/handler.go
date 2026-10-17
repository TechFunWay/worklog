package user

import (
	"errors"
	"net/http"
	"strconv"

	"smallgo/server/audit"
	"smallgo/server/database"
	"smallgo/server/middleware"
	"smallgo/server/response"
	"smallgo/server/sysconfig"
	"smallgo/server/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func parseUserID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.ErrorBadRequest(c, "无效的用户 ID")
		return 0, false
	}
	return uint(id), true
}

func handleSetupRequired(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var count int64
		if err := db.Model(&database.User{}).Count(&count).Error; err != nil {
			response.ErrorInternal(c, "检查初始化状态失败")
			return
		}
		response.Success(c, map[string]interface{}{
			"setup_required": count == 0,
		})
	}
}

func handleRegister(db *gorm.DB, gatewayPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请输入用户名和密码")
			return
		}

		secret := getJWTSecret(db)
		// 注册只创建应用账号，不读取、不绑定网关注入的飞牛身份。网关域上注册
		// 后的会话由下方种下的应用会话 cookie 维持，不再依赖绑定飞牛账号。
		result, err := Register(db, req.Username, req.Password, secret)
		if err != nil {
			switch {
			case errors.Is(err, ErrRegisterDisabled):
				response.Error(c, http.StatusForbidden, response.CodeRegisterDisabled, err.Error())
			case errors.Is(err, ErrUserExists):
				response.Error(c, http.StatusConflict, response.CodeUserExists, err.Error())
			case errors.Is(err, ErrPasswordTooShort), errors.Is(err, ErrPasswordTooLong):
				response.ErrorBadRequest(c, err.Error())
			default:
				response.ErrorInternal(c, "注册失败")
			}
			return
		}

		setAppSessionCookie(c, gatewayPrefix, sessionToken(result), int(loginTokenTTL(db).Seconds()))
		response.Success(c, result)
	}
}

func handleLogin(db *gorm.DB, gatewayPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Username    string `json:"username" binding:"required"`
			Password    string `json:"password" binding:"required"`
			PasswordMd5 string `json:"password_md5"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请输入用户名和密码")
			return
		}

		secret := getJWTSecret(db)
		result, err := Login(db, req.Username, req.Password, req.PasswordMd5, secret)
		if err != nil {
			response.Error(c, http.StatusUnauthorized, response.CodeInvalidCredentials, err.Error())
			return
		}

		// 网关域上应用自己的 Authorization 送不进应用（接入层拦截），账密登录
		// 换来的 JWT 改由 HttpOnly cookie 携带（Path=网关前缀，与兄弟应用互不
		// 干扰），后续请求里它优先于网关隐式认人——这正是账密登录不再需要绑定
		// 飞牛账号也能维持会话的关键。
		setAppSessionCookie(c, gatewayPrefix, sessionToken(result), int(loginTokenTTL(db).Seconds()))

		// Populate context so the audit entry is attributed to the logged-in user.
		if u, ok := result["user"].(map[string]interface{}); ok {
			if uid, ok := u["id"].(uint); ok {
				c.Set("userID", uid)
				c.Set("username", req.Username)
			}
		}
		audit.Log(db, c, "login", "user", c.GetUint("userID"), "用户登录")

		response.Success(c, result)
	}
}

// appSessionCookieName 是网关域上应用自有会话的 cookie 名，middleware.authenticate
// 里 c.Cookie("token") 与它对应，两边不能各改各的。
const appSessionCookieName = "token"

// setAppSessionCookie 在飞牛网关域上种下应用会话 cookie；gatewayPrefix 为空
// （非飞牛部署、直连端口语境）时不种。
func setAppSessionCookie(c *gin.Context, gatewayPrefix, token string, maxAge int) {
	if gatewayPrefix == "" || token == "" {
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     appSessionCookieName,
		Value:    token,
		Path:     gatewayPrefix,
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// 飞牛网关在局域网走 http，Secure 会导致浏览器拒发 cookie。
		Secure: false,
	})
}

// clearAppSessionCookie 清掉应用会话 cookie：飞牛登录/绑定流程要恢复「会话归
// 网关所有」的语义（跟随 NAS 退出），残留的 cookie 会抢在网关隐式认人前面把
// 用户按回账密登录的那个账号。
func clearAppSessionCookie(c *gin.Context, gatewayPrefix string) {
	if gatewayPrefix == "" {
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     appSessionCookieName,
		Value:    "",
		Path:     gatewayPrefix,
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false,
	})
}

func sessionToken(result map[string]interface{}) string {
	if token, ok := result["token"].(string); ok {
		return token
	}
	return ""
}

// fnOSIdentity reads only the headers injected by the fnOS unified gateway.
// These handlers are registered exclusively for -fnos-app, which listens on
// the gateway's Unix socket rather than a public TCP port.
func fnOSIdentity(c *gin.Context) (FnOSIdentity, bool) {
	if !middleware.OnFnOSGateway(c.Request.Context()) {
		response.ErrorUnauthorized(c, "请从飞牛桌面中的应用入口使用一键登录")
		return FnOSIdentity{}, false
	}
	uid, err := strconv.ParseUint(c.GetHeader("X-Trim-Userid"), 10, 32)
	username := c.GetHeader("X-Trim-Username")
	if err != nil || uid == 0 || username == "" {
		response.ErrorUnauthorized(c, "未获取到飞牛 NAS 登录信息")
		return FnOSIdentity{}, false
	}
	return FnOSIdentity{
		UserID:   uint(uid),
		Username: username,
		IsAdmin:  c.GetHeader("X-Trim-Isadmin") == "true",
	}, true
}

func setLoginAuditContext(c *gin.Context, result map[string]interface{}) {
	user, ok := result["user"].(map[string]interface{})
	if !ok {
		return
	}
	if uid, ok := user["id"].(uint); ok {
		c.Set("userID", uid)
	}
	if username, ok := user["username"].(string); ok {
		c.Set("username", username)
	}
}

func handleFnOSIdentity() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := fnOSIdentity(c)
		if !ok {
			return
		}
		response.Success(c, map[string]interface{}{
			"fnos_username": identity.Username,
		})
	}
}

func handleFnOSLogin(db *gorm.DB, gatewayPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := fnOSIdentity(c)
		if !ok {
			return
		}
		// 走到飞牛流程就会话归网关所有：先清掉可能残留的应用会话 cookie，
		// 否则它抢在网关隐式认人前面，用户确认了飞牛登录却还停在账密登录的
		// 那个账号上。
		clearAppSessionCookie(c, gatewayPrefix)
		result, err := LoginWithFnOS(db, identity, getJWTSecret(db))
		if errors.Is(err, ErrFnOSNotBound) {
			var accountCount int64
			if countErr := db.Model(&database.User{}).Count(&accountCount).Error; countErr != nil {
				response.ErrorInternal(c, "检查应用账号状态失败")
				return
			}
			var matchingAccounts int64
			if countErr := db.Model(&database.User{}).Where("username = ?", identity.Username).Count(&matchingAccounts).Error; countErr != nil {
				response.ErrorInternal(c, "检查飞牛账号绑定状态失败")
				return
			}
			suggestedMode := "register"
			if accountCount > 0 {
				suggestedMode = "bind"
			}
			suggestedUsername := ""
			if matchingAccounts > 0 {
				suggestedUsername = identity.Username
			}
			response.Success(c, map[string]interface{}{
				"binding_required":   true,
				"fnos_username":      identity.Username,
				"has_accounts":       accountCount > 0,
				"suggested_mode":     suggestedMode,
				"suggested_username": suggestedUsername,
			})
			return
		}
		if err != nil {
			response.Error(c, http.StatusUnauthorized, response.CodeInvalidCredentials, "飞牛一键登录失败")
			return
		}
		setLoginAuditContext(c, result)
		audit.Log(db, c, "fnos_login", "user", c.GetUint("userID"), "飞牛 NAS 一键登录")
		response.Success(c, result)
	}
}

func handleFnOSBind(db *gorm.DB, gatewayPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := fnOSIdentity(c)
		if !ok {
			return
		}
		// 绑定成功后会话归网关所有（语义同 handleFnOSLogin），先清掉应用会话
		// cookie 再进入绑定。
		clearAppSessionCookie(c, gatewayPrefix)
		var req struct {
			Mode     string `json:"mode" binding:"required"`
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请输入应用账号和密码")
			return
		}
		result, err := BindFnOSAccount(db, identity, req.Username, req.Password, req.Mode, getJWTSecret(db))
		if err != nil {
			switch {
			case errors.Is(err, ErrRegisterDisabled):
				response.Error(c, http.StatusForbidden, response.CodeRegisterDisabled, err.Error())
			case errors.Is(err, ErrUserExists), errors.Is(err, ErrFnOSAlreadyBound):
				response.Error(c, http.StatusConflict, response.CodeUserExists, err.Error())
			case errors.Is(err, ErrPasswordTooShort), errors.Is(err, ErrPasswordTooLong):
				response.ErrorBadRequest(c, err.Error())
			default:
				response.ErrorBadRequest(c, err.Error())
			}
			return
		}
		setLoginAuditContext(c, result)
		audit.Log(db, c, "fnos_bind", "user", c.GetUint("userID"), "绑定飞牛 NAS 账号")
		response.Success(c, result)
	}
}

func handleCheckAuth(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		requireLogin, _ := sysconfig.GetConfig(db, "require_login", 0)

		userID, _ := c.Get("userID")
		uid, _ := userID.(uint)

		result := CheckAuth(db, uid, requireLogin)
		// 网关域上的「登录态来源」。客户端据此区分两种会话：
		// gateway —— 仅靠网关注入的 NAS 身份隐式维持登录态，会话实际归 NAS
		//            所有，NAS 那侧一退出，应用必须跟着退出；
		// app     —— 应用自己的凭证换来的会话（账密登录的会话 cookie、直连
		//            端口的 JWT、API Key），归应用自己所有，可以独立退出，
		//            也不需要跟随 NAS。
		// 判据是 authenticate 记下的 auth_via：这次请求实际靠什么认的人。
		if middleware.OnFnOSGateway(c.Request.Context()) {
			if via, _ := c.Get("auth_via"); via == middleware.AuthViaGateway {
				result["session_source"] = "gateway"
			} else {
				result["session_source"] = "app"
			}
		}
		response.Success(c, result)
	}
}

// handleLogout 结束**应用自己的**登录态。
//
// 直连端口上会话就是应用签发的 JWT，前端清掉就结束，服务端只需把这个动作
// 记进审计；网关域上光清前端没用——服务端每个请求都能从网关注入的 NAS 身份
// 重新认出应用账号，所以必须落一条持久化抑制标记，告诉网关隐式认人「这位
// 用户主动登出了，别再自动放行」。两种环境返回同样的结果，前端不必分支。
//
// 这里刻意走 optionalAuth：退出必须幂等，重复点击或会话已失效也应返回成功。
func handleLogout(db *gorm.DB, gatewayPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("userID")
		uid, _ := userID.(uint)
		if err := LogoutApplicationSession(db, uid); err != nil {
			response.ErrorInternal(c, "退出登录失败，请稍后重试")
			return
		}
		// 飞牛部署下 cookie 与直连端口同宿主机共享（cookie 不分端口），无论这次
		// 退出发生在哪个监听器上都把它清掉，避免「在直连端口退出、网关域又被
		// 会话 cookie 认回来」。
		clearAppSessionCookie(c, gatewayPrefix)
		if uid != 0 {
			audit.Log(db, c, "logout", "user", uid, "用户退出登录")
		}
		response.Success(c, nil)
	}
}

func handleGetCurrentUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("userID")
		uid, _ := userID.(uint)

		result, err := GetCurrentUser(db, uid)
		if err != nil {
			response.ErrorInternal(c, "获取用户信息失败")
			return
		}

		response.Success(c, result)
	}
}

func handleChangePassword(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			OldPassword    string `json:"old_password" binding:"required"`
			OldPasswordMd5 string `json:"old_password_md5"`
			NewPassword    string `json:"new_password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请输入旧密码和新密码")
			return
		}

		userID, _ := c.Get("userID")
		uid, _ := userID.(uint)

		if err := ChangePassword(db, uid, req.OldPassword, req.OldPasswordMd5, req.NewPassword); err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}

		response.Success(c, nil)
	}
}

func handleRegenerateAPIKey(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("userID")
		uid, _ := userID.(uint)

		newKey, err := RegenerateAPIKey(db, uid)
		if err != nil {
			response.ErrorInternal(c, "重新生成 API Key 失败")
			return
		}

		response.Success(c, map[string]string{"api_key": newKey})
	}
}

func handleGetAllUsers(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		page := utils.Atoi(c.Query("page"), 1)
		pageSize := utils.Atoi(c.Query("pageSize"), utils.DefaultPageSize)
		search := c.Query("search")

		users, total, err := GetAllUsers(db, page, pageSize, search)
		if err != nil {
			response.ErrorInternal(c, "获取用户列表失败")
			return
		}

		page, pageSize = utils.NormalizePage(page, pageSize)
		response.SuccessPage(c, users, total, page, pageSize)
	}
}

func handleUpdateUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Role   *string `json:"role"`
			Status *int    `json:"status"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请求无效")
			return
		}

		userID, ok := parseUserID(c)
		if !ok {
			return
		}

		if err := UpdateUser(db, userID, req.Role, req.Status); err != nil {
			if errors.Is(err, ErrUserNotFound) {
				response.Error(c, http.StatusNotFound, response.CodeNotFound, err.Error())
			} else {
				response.ErrorBadRequest(c, err.Error())
			}
			return
		}

		audit.Log(db, c, "user_update", "user", userID, "管理员修改用户信息")
		response.Success(c, nil)
	}
}

func handleDeleteUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := parseUserID(c)
		if !ok {
			return
		}
		if userID == c.GetUint("userID") {
			response.ErrorBadRequest(c, "不能删除自己的账号")
			return
		}
		if err := DeleteUser(db, userID); err != nil {
			switch {
			case errors.Is(err, ErrUserNotFound):
				response.Error(c, http.StatusNotFound, response.CodeNotFound, err.Error())
			case errors.Is(err, ErrLastAdmin), errors.Is(err, ErrUserDeleteBlocked):
				response.ErrorBadRequest(c, err.Error())
			default:
				response.ErrorInternal(c, "删除用户失败")
			}
			return
		}

		audit.Log(db, c, "user_delete", "user", userID, "管理员删除用户")
		response.Success(c, nil)
	}
}

func handleToggleStatus(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := parseUserID(c)
		if !ok {
			return
		}

		if err := ToggleUserStatus(db, userID); err != nil {
			switch {
			case errors.Is(err, ErrUserNotFound):
				response.Error(c, http.StatusNotFound, response.CodeNotFound, err.Error())
			case errors.Is(err, ErrLastAdmin):
				response.ErrorBadRequest(c, err.Error())
			default:
				response.ErrorInternal(c, "切换用户状态失败")
			}
			return
		}

		audit.Log(db, c, "user_status", "user", userID, "管理员切换用户状态")
		response.Success(c, nil)
	}
}

func handleResetPassword(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			NewPassword string `json:"new_password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请输入新密码")
			return
		}

		userID, ok := parseUserID(c)
		if !ok {
			return
		}
		if err := ResetPassword(db, userID, req.NewPassword); err != nil {
			if errors.Is(err, ErrPasswordTooShort) || errors.Is(err, ErrPasswordTooLong) {
				response.ErrorBadRequest(c, err.Error())
			} else if errors.Is(err, ErrUserNotFound) {
				response.Error(c, http.StatusNotFound, response.CodeNotFound, err.Error())
			} else {
				response.ErrorInternal(c, "重置密码失败")
			}
			return
		}

		audit.Log(db, c, "password_reset", "user", userID, "管理员重置用户密码")
		response.Success(c, nil)
	}
}

// RegisterRoutes wires user and authentication routes. gatewayPrefix 是飞牛
// 部署的网关前缀（如 /app/techfunway-worklog），用作应用会话 cookie 的 Path；
// 非飞牛部署传空串，cookie 相关逻辑全部静默跳过。
func RegisterRoutes(publicGroup *gin.RouterGroup, optionalAuthGroup *gin.RouterGroup, authGroup *gin.RouterGroup, adminGroup *gin.RouterGroup, db *gorm.DB, fnOSApp bool, gatewayPrefix string) {
	publicGroup.GET("/auth/setup-required", handleSetupRequired(db))
	publicGroup.POST("/auth/register", handleRegister(db, gatewayPrefix))
	publicGroup.POST("/auth/login", handleLogin(db, gatewayPrefix))
	if fnOSApp {
		publicGroup.GET("/auth/fnos/identity", handleFnOSIdentity())
		publicGroup.POST("/auth/fnos/login", handleFnOSLogin(db, gatewayPrefix))
		publicGroup.POST("/auth/fnos/bind", handleFnOSBind(db, gatewayPrefix))
	}

	optionalAuthGroup.GET("/auth/check", handleCheckAuth(db))

	// 退出登录走 optionalAuth：会话已失效时重复调用也必须成功（幂等），
	// 网关域上它还会落一条「别再自动认人」的持久化标记（见 handleLogout）。
	optionalAuthGroup.POST("/auth/logout", handleLogout(db, gatewayPrefix))

	authGroup.GET("/auth/me", handleGetCurrentUser(db))
	authGroup.PUT("/auth/password", handleChangePassword(db))
	authGroup.POST("/auth/apikey", handleRegenerateAPIKey(db))

	adminGroup.GET("/users", handleGetAllUsers(db))
	adminGroup.PUT("/users/:id", handleUpdateUser(db))
	adminGroup.DELETE("/users/:id", handleDeleteUser(db))
	adminGroup.PUT("/users/:id/status", handleToggleStatus(db))
	adminGroup.PUT("/users/:id/password", handleResetPassword(db))
}
