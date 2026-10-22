package service

// 米哈游 (原神) 每日签到服务
//
// 登录: 前端在个人中心「原神签到」Tab 调 POST /api/mihoyo/qr/start 拿二维码 →
//       米游社 App 扫码确认 → 轮询 GET /api/mihoyo/qr/status, 确认后兑换并保存登录凭据。
// 签到: 每日定时任务 mihoyo.genshin_checkin 遍历所有已绑定账号签到原神;
//       个人中心也可 POST /api/mihoyo/checkin 手动触发, 结果经接口返回 + 控制台日志, 不落库。
//
// go 会话里每张二维码对应一个 cookiejar: queryQRLoginStatus 确认后的登录态
// 会落进会话 Cookie (qr_login.py 的"方案一"), tokens 数组是"方案二"兜底。

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"

	"backend/model"

	"gorm.io/gorm"
)

// mihoyoQRSessionTTL 扫码会话有效期 (passport 二维码实际 1~2 分钟, 轮询入口自会报过期)
const mihoyoQRSessionTTL = 10 * time.Minute

// mihoyoQRSession 一次扫码登录会话 (同一 client/jar 贯穿 create → query)
type mihoyoQRSession struct {
	userID    uint
	deviceID  string
	jar       *cookiejar.Jar
	client    *http.Client
	createdAt time.Time
}

type MihoyoService struct {
	mu       sync.Mutex
	sessions map[string]*mihoyoQRSession // ticket -> session
}

func NewMihoyoService() *MihoyoService {
	return &MihoyoService{sessions: make(map[string]*mihoyoQRSession)}
}

// ============ 扫码登录 ============

// CreateQRLogin 生成二维码, 返回 ticket + 内容串 (前端自行渲染成二维码图片)
func (s *MihoyoService) CreateQRLogin(userID uint) (*model.MihoyoQRStartData, error) {
	// 顺手清理过期会话
	now := time.Now()
	s.mu.Lock()
	for key, sess := range s.sessions {
		if now.Sub(sess.createdAt) > mihoyoQRSessionTTL {
			delete(s.sessions, key)
		}
	}
	s.mu.Unlock()

	sess, err := newQRSession(userID)
	if err != nil {
		return nil, err
	}
	data, err := passportCreateQR(sess.client, sess.deviceID)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.sessions[data.Ticket] = sess
	s.mu.Unlock()

	return &model.MihoyoQRStartData{
		Ticket:    data.Ticket,
		QRUrl:     data.URL,
		ExpiresIn: int(mihoyoQRSessionTTL.Seconds()),
	}, nil
}

// QRLoginStatus 轮询扫码状态; Confirmed 时完成登录并绑定凭据
func (s *MihoyoService) QRLoginStatus(userID uint, ticket string) (*model.MihoyoQRStatusData, error) {
	s.mu.Lock()
	sess, ok := s.sessions[ticket]
	s.mu.Unlock()
	if !ok || sess.userID != userID {
		return nil, fmt.Errorf("登录会话不存在或已过期, 请重新扫码")
	}

	data, mr, err := passportQueryQR(sess.client, sess.deviceID, ticket)
	if err != nil {
		return nil, err
	}
	if mr != nil && mr.RetCode != 0 {
		// 二维码过期/被撤销等, 会话可以扔了
		s.removeSession(ticket)
		return &model.MihoyoQRStatusData{Status: "Expired", Message: mr.Message}, nil
	}

	switch data.Status {
	case "Confirmed":
		res, err := s.completeLogin(sess, data)
		s.removeSession(ticket) // ticket 一次性: 无论登录兑换成败都结束会话
		if err != nil {
			return nil, err
		}
		return res, nil
	case "Created", "Scanned", "":
		status := data.Status
		if status == "" {
			status = "Created"
		}
		return &model.MihoyoQRStatusData{Status: status}, nil
	default:
		// 未知状态原样透传 (如 Canceled)
		return &model.MihoyoQRStatusData{Status: data.Status}, nil
	}
}

// completeLogin 确认后取登录态 → 兑换 cookie_token/ltoken → 落库
func (s *MihoyoService) completeLogin(sess *mihoyoQRSession, data *passportQRQueryData) (*model.MihoyoQRStatusData, error) {
	aid := data.UserInfo.AID
	mid := data.UserInfo.MID
	if aid == "" || mid == "" {
		return nil, fmt.Errorf("确认成功但未返回账号信息 (aid=%q mid=%q), 请重试", aid, mid)
	}

	// 官方文档 (米游社扫码登录): Confirmed 后登录态全在响应 Set-Cookie 里, tokens 恒为空;
	// 可获取 account_id(_v2)/mid(_v2)/ltoken(_v2)/cookie_token(_v2), 老版本流程才含 stoken。
	cookieToken := jarCookieValue(sess.jar, "cookie_token_v2", "cookie_token")
	ltoken := jarCookieValue(sess.jar, "ltoken_v2", "ltoken")
	accountID := jarCookieValue(sess.jar, "account_id_v2", "account_id")
	stoken := jarCookieValue(sess.jar, "stoken_v2", "stoken")
	if accountID != "" {
		aid = accountID // user_info aid 与 Set-Cookie 冲突时以登录 Cookie 为准
	}
	if stoken != "" && (cookieToken == "" || ltoken == "") {
		// 只有 stoken 的旧流程: 兑换补齐下级凭据
		if c, err := exchangeCookieToken(sess.client, stoken, aid, mid); err == nil && c != "" {
			cookieToken = c
		}
		if l, err := exchangeLtoken(sess.client, stoken, aid, mid); err == nil && l != "" {
			ltoken = l
		}
	}
	if cookieToken == "" && ltoken == "" && stoken == "" {
		return nil, fmt.Errorf("确认成功但响应未带登录 Cookie, 无法绑定, 请重试 (cookies=[%s], tokens=%s)",
			jarCookieSummary(sess.jar), tokenTypesSummary(data.Tokens))
	}

	// 昵称取确认回执里的 links 第三方昵称 (不额外拉角色列表, 绑定瞬间不多发米游社请求)
	nickname := data.UserInfo.Nickname
	if nickname == "" {
		for _, l := range data.UserInfo.Links {
			if l.Nickname != "" {
				nickname = l.Nickname
				break
			}
		}
	}

	if err := upsertMihoyoCredential(sess.userID, aid, mid, nickname, stoken, cookieToken, ltoken); err != nil {
		return nil, err
	}

	return &model.MihoyoQRStatusData{
		Status:   "Confirmed",
		Nickname: nickname,
		UID:      aid,
		Bound:    true,
	}, nil
}

// newQRSession userID 从入口贯穿进来: 会话归属判定靠它, 不落值归属校验永远不过
func newQRSession(userID uint) (*mihoyoQRSession, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &mihoyoQRSession{
		userID:    userID,
		deviceID:  mihoyoRandomHex(32),
		jar:       jar,
		client:    &http.Client{Timeout: 20 * time.Second, Jar: jar},
		createdAt: time.Now(),
	}, nil
}

func (s *MihoyoService) removeSession(ticket string) {
	s.mu.Lock()
	delete(s.sessions, ticket)
	s.mu.Unlock()
}

func upsertMihoyoCredential(userID uint, aid, mid, nickname, stoken, cookieToken, ltoken string) error {
	var cred model.MihoyoCredential
	err := DB.Where("user_id = ?", userID).First(&cred).Error
	notFound := errors.Is(err, gorm.ErrRecordNotFound)
	if err != nil && !notFound {
		return err
	}

	cred.UserID = userID
	cred.MihoyoUID = aid
	cred.MihoyoMid = mid
	if nickname != "" {
		cred.Nickname = nickname
	}
	if stoken != "" {
		// 扫码 web 流程可能不带 stoken, 只有无值时才允许覆盖 (避免洗掉已存的续期凭据)
		cred.Stoken = stoken
	}
	if cookieToken != "" {
		cred.CookieToken = cookieToken
	}
	if ltoken != "" {
		cred.Ltoken = ltoken
	}

	if notFound {
		return DB.Create(&cred).Error
	}
	return DB.Save(&cred).Error
}

func getMihoyoCredential(userID uint) (*model.MihoyoCredential, error) {
	var cred model.MihoyoCredential
	err := DB.Where("user_id = ?", userID).First(&cred).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cred, nil
}

// ============ 绑定信息 ============

// BindStatus 当前用户绑定状态
func (s *MihoyoService) BindStatus(userID uint) (*model.MihoyoBindStatusData, error) {
	cred, err := getMihoyoCredential(userID)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return &model.MihoyoBindStatusData{Bound: false}, nil
	}
	return &model.MihoyoBindStatusData{
		Bound:    true,
		Nickname: cred.Nickname,
		UID:      cred.MihoyoUID,
		Mid:      cred.MihoyoMid,
	}, nil
}

// Unbind 解绑 (删除凭据)
func (s *MihoyoService) Unbind(userID uint) error {
	if err := DB.Where("user_id = ?", userID).Delete(&model.MihoyoCredential{}).Error; err != nil {
		return err
	}
	return nil
}

// ============ 签到 ============

func mihoyoJitter(minMS, maxMS int) time.Duration {
	return time.Duration(minMS+rand.IntN(maxMS-minMS)) * time.Millisecond
}

// CheckinUser 手动为单个用户签到
func (s *MihoyoService) CheckinUser(userID uint) ([]model.MihoyoCheckinResult, error) {
	cred, err := getMihoyoCredential(userID)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, fmt.Errorf("未绑定米游社账号, 请先扫码登录")
	}
	return s.checkinCredential(cred), nil
}

// CheckinStatus 签到信息查询: 查每个原神角色的今日已签/累计天数/今日奖励, 不触发签到
func (s *MihoyoService) CheckinStatus(userID uint) (*model.MihoyoCheckinStatusData, error) {
	cred, err := getMihoyoCredential(userID)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return &model.MihoyoCheckinStatusData{Bound: false, Roles: []model.MihoyoCheckinStatusRole{}}, nil
	}

	client := newHTTPClient(20 * time.Second)
	cookie := buildCookie(cred)
	roles, mr, err := fetchGameRoles(client, cookie)
	if err != nil && mr != nil && mihoyoAuthRetcodes[mr.RetCode] && cred.Stoken != "" {
		// 登录态失效: stoken 续期后重试一次 (与签到共用同一条续期链路)
		if newCookie, rerr := s.refreshCredential(cred, client); rerr == nil {
			cookie = newCookie
			// 续期后 mr 在后续分支里不再使用, 忽略避免静态检查报未使用赋值
			roles, _, err = fetchGameRoles(client, cookie)
		}
	}
	if err != nil {
		return nil, err
	}

	out := &model.MihoyoCheckinStatusData{Bound: true, Roles: make([]model.MihoyoCheckinStatusRole, 0, len(roles))}
	for _, role := range roles {
		item := model.MihoyoCheckinStatusRole{
			GameUID:  role.GameUID,
			Nickname: role.Nickname,
		}
		// 单角色信息查询失败不阻塞整页: 字段留空由前端显示 '-'
		if info, ierr := lunaInfo(client, cookie, role.Region, role.GameUID); ierr == nil {
			item.Signed = info.IsSign
			item.TotalSignDay = info.TotalSignDay
			item.AwardName = info.Award.Name
			item.AwardCnt = fmt.Sprintf("%v", info.Award.Cnt)
			item.AwardIcon = info.Award.Icon
		}
		out.Roles = append(out.Roles, item)
	}
	return out, nil
}

// CheckinAll 定时任务入口: 遍历全部绑定账号签到 (有失败返回 error 触发告警)
func (s *MihoyoService) CheckinAll() error {
	var creds []model.MihoyoCredential
	if err := DB.Find(&creds).Error; err != nil {
		return err
	}
	if len(creds) == 0 {
		log.Println("[Mihoyo] 定时签到跳过: 没有绑定的米游社账号")
		return nil
	}

	log.Printf("[Mihoyo] 定时签到开始: %d 个账号", len(creds))
	failed := 0
	for i := range creds {
		cred := creds[i]
		for _, res := range s.checkinCredential(&cred) {
			if res.Status == model.MihoyoCheckinStatusBlocked || res.Status == model.MihoyoCheckinStatusFailed {
				failed++
			}
		}
		// 账号间随机间隔, 降低风控
		if i < len(creds)-1 {
			time.Sleep(mihoyoJitter(2000, 5000))
		}
	}
	if failed > 0 {
		return fmt.Errorf("原神签到有 %d 处失败 (验证码/接口错误), 请查看任务日志并手动处理", failed)
	}
	return nil
}

// checkinCredential 对一个凭据下的所有原神角色签到, 打印结果
func (s *MihoyoService) checkinCredential(cred *model.MihoyoCredential) []model.MihoyoCheckinResult {
	client := newHTTPClient(20 * time.Second)
	cookie := buildCookie(cred)

	roles, mr, err := fetchGameRoles(client, cookie)
	if err != nil {
		// 登录态失效: 用 stoken 续期后重试一次
		if mr != nil && mihoyoAuthRetcodes[mr.RetCode] && cred.Stoken != "" {
			if newCookie, rerr := s.refreshCredential(cred, client); rerr == nil {
				cookie = newCookie
				// 续期后 mr 在后续分支里不再使用, 忽略避免静态检查报未使用赋值
				roles, _, err = fetchGameRoles(client, cookie)
			}
		}
	}
	if err != nil {
		return []model.MihoyoCheckinResult{emitCheckin(cred, nil,
			model.MihoyoCheckinStatusFailed, fmt.Sprintf("获取角色失败: %v", err))}
	}
	if len(roles) == 0 {
		return []model.MihoyoCheckinResult{emitCheckin(cred, nil,
			model.MihoyoCheckinStatusNoRole, "该账号下没有原神(国服)角色, 无需签到")}
	}

	results := make([]model.MihoyoCheckinResult, 0, len(roles))
	for i, role := range roles {
		if i > 0 {
			// 角色间随机间隔, 降低风控
			time.Sleep(mihoyoJitter(1000, 3000))
		}
		results = append(results, s.signOneRole(cred, client, cookie, role))
	}
	return results
}

// signOneRole 单角色: 查已签天数 → 签到, 按 python 版语义分派状态
func (s *MihoyoService) signOneRole(cred *model.MihoyoCredential, client *http.Client, cookie string, role gameRole) model.MihoyoCheckinResult {
	tag := fmt.Sprintf("%s(%s)", role.Nickname, role.GameUID)

	info, err := lunaInfo(client, cookie, role.Region, role.GameUID)
	if err != nil {
		return emitCheckin(cred, &role, model.MihoyoCheckinStatusFailed,
			fmt.Sprintf("查询签到状态失败: %v", err))
	}
	if info.IsSign {
		return emitCheckin(cred, &role, model.MihoyoCheckinStatusAlready,
			fmt.Sprintf("%s 今天已签过, 本月累计 %d 天", tag, info.TotalSignDay))
	}

	out, resp, err := lunaSign(client, cookie, role.Region, role.GameUID)
	if err != nil {
		return emitCheckin(cred, &role, model.MihoyoCheckinStatusFailed,
			fmt.Sprintf("签到请求异常: %v", err))
	}
	if resp.RetCode == 0 && !mihoyoRiskHappened(out) {
		return emitCheckin(cred, &role, model.MihoyoCheckinStatusSuccess,
			fmt.Sprintf("%s 签到成功, 本月累计 %d 天", tag, info.TotalSignDay+1))
	}
	if resp.RetCode == -5003 {
		return emitCheckin(cred, &role, model.MihoyoCheckinStatusAlready, fmt.Sprintf("%s 今天已签过", tag))
	}
	if mihoyoRiskHappened(out) || out.GT != "" {
		return emitCheckin(cred, &role, model.MihoyoCheckinStatusBlocked,
			fmt.Sprintf("%s 触发验证码(geetest)被拦截, 需手动到米游社活动页补签 (国服随机触发, 隔天一般恢复)", tag))
	}
	return emitCheckin(cred, &role, model.MihoyoCheckinStatusFailed,
		fmt.Sprintf("%s 签到失败: %s", tag, mihoyoRetError(resp)))
}

// emitCheckin 打印结果 (仅日志, 不落库)
func emitCheckin(cred *model.MihoyoCredential, role *gameRole, status, message string) model.MihoyoCheckinResult {
	res := model.MihoyoCheckinResult{Status: status, Message: message}
	if role != nil {
		res.GameUID = role.GameUID
		res.Nickname = role.Nickname
	}

	logLine := message
	if label := firstNonEmpty(cred.Nickname, cred.MihoyoUID); label != "" {
		logLine = fmt.Sprintf("[账号 %s] %s", label, message)
	}
	switch status {
	case model.MihoyoCheckinStatusSuccess:
		log.Printf("[Mihoyo] ✅ %s", logLine)
	case model.MihoyoCheckinStatusAlready, model.MihoyoCheckinStatusNoRole:
		log.Printf("[Mihoyo] ➖ %s", logLine)
	case model.MihoyoCheckinStatusBlocked:
		log.Printf("[Mihoyo] ⚠️ %s", logLine)
	default:
		log.Printf("[Mihoyo] ❌ %s", logLine)
	}
	return res
}

// refreshCredential 用 stoken 续期 cookie_token/ltoken, 更新库后返回新 Cookie
func (s *MihoyoService) refreshCredential(cred *model.MihoyoCredential, client *http.Client) (string, error) {
	if cred.Stoken == "" {
		return "", fmt.Errorf("登录态已失效且无 stoken, 请重新扫码绑定")
	}
	cookieToken, _ := exchangeCookieToken(client, cred.Stoken, cred.MihoyoUID, cred.MihoyoMid)
	ltoken, _ := exchangeLtoken(client, cred.Stoken, cred.MihoyoUID, cred.MihoyoMid)
	if cookieToken == "" && ltoken == "" {
		return "", fmt.Errorf("stoken 已过期, 请重新扫码绑定")
	}
	if cookieToken != "" {
		cred.CookieToken = cookieToken
	}
	if ltoken != "" {
		cred.Ltoken = ltoken
	}
	if err := DB.Model(&model.MihoyoCredential{}).Where("id = ?", cred.ID).
		Updates(map[string]interface{}{
			"cookie_token": cred.CookieToken,
			"ltoken":       cred.Ltoken,
		}).Error; err != nil {
		log.Printf("[Mihoyo] 更新续期后的凭据失败: %v", err)
	}
	return buildCookie(cred), nil
}

// ============ 定时任务注册 ============// RegisterCronTasks 注册原神签到定时任务 (job_definitions 里可改 cron/参数)
func (s *MihoyoService) RegisterCronTasks(js *JobScheduler) {
	js.RegisterTask(model.TaskNameMihoyoGenshinCheckin,
		"原神每日签到 (所有已扫码绑定的账号, 失败可邮件告警)",
		json.RawMessage(`{}`),
		func(_ *model.JobDefinition, _ json.RawMessage) error {
			return s.CheckinAll()
		})
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
