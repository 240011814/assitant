package service

// 米哈游接口协议层 (接口与 PTAbabybearR/MihoyoDailyCheckin 的 qr_login.py / sign.py 同源):
//   扫码: passport web createQRLogin / queryQRLoginStatus  (DS 盐: passport web)
//   兑换: stoken → cookie_token / ltoken
//   签到: binding getUserGameRolesByCookie + event/luna/info|sign  (DS 盐: web, x-rpc-signgame 区分游戏)
// 常量随官方更新失效时改这里。

import (
	"crypto/md5"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"backend/model"
)

const (
	mihoyoBBSAppID     = "bll8iq97cem8"                     // 米游社 bbs
	mihoyoPassportSalt = "JwYDpKvLj6MrMqqYU6jTKF17KNO2PXoS" // passport web DS 盐 (失效时更新)
	mihoyoLunaSalt     = "xV8v4Qu54lUKrEYFZkJhB8cuOh9Asafs" // web 端 DS 盐 (失效时更新)
	mihoyoAppVersion   = "2.71.1"                           // 米游社 App 版本号
	mihoyoLang         = "zh-cn"

	genshinGameBiz  = "hk4e_cn"
	genshinSignGame = "hk4e"
	genshinActID    = "e202311201442471"

	urlMihoyoCreateQR    = "https://passport-api.mihoyo.com/account/ma-cn-passport/web/createQRLogin"
	urlMihoyoQueryQR     = "https://passport-api.mihoyo.com/account/ma-cn-passport/web/queryQRLoginStatus"
	urlMihoyoGameRoles   = "https://api-takumi.mihoyo.com/binding/api/getUserGameRolesByCookie"
	urlMihoyoLunaInfo    = "https://api-takumi.mihoyo.com/event/luna/info"
	urlMihoyoLunaSign    = "https://api-takumi.mihoyo.com/event/luna/sign"
	urlMihoyoBySToken    = "https://api-takumi.mihoyo.com/auth/api/getCookieAccountInfoBySToken"
	urlMihoyoLTokenBySTK = "https://passport-api.mihoyo.com/account/auth/api/getLTokenBySToken"
)

// luna 系接口判定登录态失效的 retcode 命中即用 stoken 续期后重试
var mihoyoAuthRetcodes = map[int]bool{
	-100:  true, // InVaild error / cookie 无效
	-101:  true,
	10001: true,
	10002: true,
}

// ===== DS 动态签名 =====

// mihoyoPassportDS passport web DS: md5(salt=&t=&r=&b=&q=)
func mihoyoPassportDS(body, query string) string {
	t := strconv.FormatInt(time.Now().Unix(), 10)
	r := mihoyoRandomStr(6)
	sum := md5.Sum([]byte(fmt.Sprintf("salt=%s&t=%s&r=%s&b=%s&q=%s", mihoyoPassportSalt, t, r, body, query)))
	return fmt.Sprintf("%s,%s,%s", t, r, hex.EncodeToString(sum[:]))
}

// mihoyoLunaDS web 端 DS: md5(salt=&t=&r=)
func mihoyoLunaDS() string {
	t := strconv.FormatInt(time.Now().Unix(), 10)
	r := mihoyoRandomStr(6)
	sum := md5.Sum([]byte(fmt.Sprintf("salt=%s&t=%s&r=%s", mihoyoLunaSalt, t, r)))
	return fmt.Sprintf("%s,%s,%s", t, r, hex.EncodeToString(sum[:]))
}

func mihoyoRandomStr(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	_, _ = mihoyoRandRead(b)
	for i, v := range b {
		b[i] = letters[int(v)%len(letters)]
	}
	return string(b)
}

// mihoyoRandomHex 随机 hex 设备号 (等宽 uuid/32)
func mihoyoRandomHex(n int) string {
	b := make([]byte, (n+1)/2)
	_, _ = mihoyoRandRead(b)
	return hex.EncodeToString(b)[:n]
}

// mihoyoRandRead 随机字节来源 (crypto/rand)
func mihoyoRandRead(b []byte) (int, error) {
	return crand.Read(b)
}

// ===== 请求头 =====

func passportHeader(deviceID, body string) http.Header {
	h := http.Header{}
	h.Set("x-rpc-app_id", mihoyoBBSAppID)
	h.Set("x-rpc-device_id", deviceID)
	h.Set("x-rpc-client_type", "2")
	h.Set("DS", mihoyoPassportDS(body, ""))
	h.Set("Content-Type", "application/json")
	h.Set("User-Agent", "okhttp/4.9.3")
	return h
}

func lunaHeader(cookie, signgame string) http.Header {
	h := http.Header{}
	h.Set("Cookie", cookie)
	h.Set("DS", mihoyoLunaDS())
	h.Set("x-rpc-app_version", mihoyoAppVersion)
	h.Set("x-rpc-client_type", "5")
	h.Set("x-rpc-platform", "4")
	h.Set("x-rpc-signgame", signgame) // luna 接口靠它区分游戏, 缺了/错了报 -500001
	h.Set("x-rpc-channel", "appstore")
	h.Set("x-rpc-device_id", mihoyoRandomHex(32))
	h.Set("User-Agent", fmt.Sprintf("Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) "+
		"AppleWebKit/605.1.15 (KHTML, like Gecko) miHoYoBBS/%s", mihoyoAppVersion))
	h.Set("Referer", "https://act.mihoyo.com")
	h.Set("Origin", "https://act.mihoyo.com")
	h.Set("Accept", "application/json, text/plain, */*")
	return h
}

// ===== 统一请求 =====

type mihoyoResp struct {
	RetCode int             `json:"retcode"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func mihoyoRetError(mr *mihoyoResp) error {
	return fmt.Errorf("%s (retcode=%d)", mr.Message, mr.RetCode)
}

// doMihoyoJSON 发请求并解析米哈游响应包 {retcode,message,data}
func doMihoyoJSON(client *http.Client, method, urlStr, body string, header http.Header, out interface{}) (*mihoyoResp, error) {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, urlStr, rd)
	if err != nil {
		return nil, err
	}
	for key, vals := range header {
		for _, v := range vals {
			req.Header.Add(key, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("接口 HTTP %d: %s", resp.StatusCode, mihoyoSnippet(raw))
	}
	var mr mihoyoResp
	if err := json.Unmarshal(raw, &mr); err != nil {
		return nil, fmt.Errorf("接口返回非 JSON: %s", mihoyoSnippet(raw))
	}
	// 请求全链路响应日志: body/set-cookie 经过掩码 (敏感 token 只留前 10 字符), 便于直接看回执原貌
	log.Printf("[mihoyo] %s %s -> HTTP %d retcode=%d message=%q body=%s",
		method, urlStr, resp.StatusCode, mr.RetCode, mr.Message, maskTokens(mihoyoLogSnippet(raw)))
	for _, sc := range resp.Header.Values("Set-Cookie") {
		log.Printf("[mihoyo]   set-cookie: %s", maskTokens(sc))
	}
	if out != nil && len(mr.Data) > 0 {
		if err := json.Unmarshal(mr.Data, out); err != nil {
			return nil, fmt.Errorf("接口 data 解析失败: %v", err)
		}
	}
	return &mr, nil
}

func mihoyoSnippet(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// mihoyoLogSnippet 日志用片段截断 (确认回执带 user_info+tokens, 200 放不下)
func mihoyoLogSnippet(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 1500 {
		s = s[:1500] + "..."
	}
	return s
}

// mihoyoTokenMaskRe 敏感字段值只保留前 10 字符 (日志里不落完整凭据)
var mihoyoTokenMaskRe = regexp.MustCompile(
	`(?i)((?:stoken|ltoken|cookie_token|token_value|game_token|account_id|ltuid|mid)[^A-Za-z0-9]*[A-Za-z0-9_+/=.%+-]{10})[A-Za-z0-9_+/=.%+-]*`)

// maskTokens 日志掩码
func maskTokens(s string) string {
	return mihoyoTokenMaskRe.ReplaceAllString(s, "$1...")
}

// ===== 协议载荷 =====

type passportQRCreateData struct {
	URL    string `json:"url"`
	Ticket string `json:"ticket"`
}

type passportQRToken struct {
	TokenType  int    `json:"token_type"`
	TokenValue string `json:"token_value"`
}

type passportQRQueryData struct {
	Status   string `json:"status"`
	UserInfo struct {
		AID      string `json:"aid"`
		MID      string `json:"mid"`
		Nickname string `json:"nickname"`
		Links    []struct {
			Thirdparty string `json:"thirdparty"`
			Nickname   string `json:"nickname"`
		} `json:"links"`
	} `json:"user_info"`
	Tokens []passportQRToken `json:"tokens"`
	Ext    string            `json:"ext"`
}

type passportCookieTokenData struct {
	CookieToken string `json:"cookie_token"`
}

type passportLtokenData struct {
	Ltoken string `json:"ltoken"`
}

type gameRole struct {
	GameBiz  string `json:"game_biz"`
	Region   string `json:"region"`
	GameUID  string `json:"game_uid"`
	Nickname string `json:"nickname"`
	Level    int    `json:"level"`
}

type gameRolesData struct {
	List []gameRole `json:"list"`
}

// lunaAward 签到奖励条目 (cnt 类型回执不固定, 不判死)
type lunaAward struct {
	Icon string `json:"icon"`
	Name string `json:"name"`
	Cnt  any    `json:"cnt"`
}

type lunaInfoData struct {
	IsSign       bool      `json:"is_sign"`
	TotalSignDay int       `json:"total_sign_day"`
	TodaySignDay int       `json:"today_sign_day"`
	FirstBind    bool      `json:"first_bind"`
	Award        lunaAward `json:"award"`
}

type lunaSignData struct {
	Code      string      `json:"code"`
	GT        string      `json:"gt"`
	Challenge string      `json:"challenge"`
	RiskCode  interface{} `json:"risk_code"`
}

// mihoyoRiskHappened risk_code 类型不固定(int/string), 统一判定非空非 0
func mihoyoRiskHappened(d *lunaSignData) bool {
	switch v := d.RiskCode.(type) {
	case nil:
		return false
	case string:
		return v != "" && v != "0"
	case float64:
		return v != 0
	default:
		return true
	}
}

// ===== 各接口 =====

// passportCreateQR 生成扫码登录二维码
func passportCreateQR(client *http.Client, deviceID string) (*passportQRCreateData, error) {
	body := "{}"
	var data passportQRCreateData
	mr, err := doMihoyoJSON(client, http.MethodPost, urlMihoyoCreateQR, body, passportHeader(deviceID, body), &data)
	if err != nil {
		return nil, err
	}
	if mr.RetCode != 0 {
		return nil, fmt.Errorf("取二维码失败: %w", mihoyoRetError(mr))
	}
	if data.Ticket == "" || data.URL == "" {
		return nil, fmt.Errorf("取二维码失败: 响应缺少 url/ticket")
	}
	return &data, nil
}

// passportQueryQR 轮询扫码状态。retcode != 0 (如 ticket 过期) 返回 (nil, resp, nil) 由调用方处理。
func passportQueryQR(client *http.Client, deviceID, ticket string) (*passportQRQueryData, *mihoyoResp, error) {
	body, _ := json.Marshal(map[string]string{"ticket": ticket})
	var data passportQRQueryData
	mr, err := doMihoyoJSON(client, http.MethodPost, urlMihoyoQueryQR, string(body), passportHeader(deviceID, string(body)), &data)
	if err != nil {
		return nil, nil, err
	}
	if mr.RetCode != 0 {
		return nil, mr, nil
	}
	return &data, mr, nil
}

// exchangeCookieToken stoken → cookie_token
func exchangeCookieToken(client *http.Client, stoken, aid, mid string) (string, error) {
	q := url.Values{"stoken": {stoken}, "uid": {aid}}
	h := http.Header{}
	h.Set("Cookie", fmt.Sprintf("stuid=%s;stoken=%s;mid=%s", aid, stoken, mid))
	h.Set("x-rpc-app_id", mihoyoBBSAppID)
	var out passportCookieTokenData
	mr, err := doMihoyoJSON(client, http.MethodGet, urlMihoyoBySToken+"?"+q.Encode(), "", h, &out)
	if err != nil {
		return "", err
	}
	if mr.RetCode != 0 {
		return "", fmt.Errorf("兑换 cookie_token 失败: %w", mihoyoRetError(mr))
	}
	return out.CookieToken, nil
}

// exchangeLtoken stoken → ltoken
func exchangeLtoken(client *http.Client, stoken, aid, mid string) (string, error) {
	h := http.Header{}
	h.Set("Cookie", fmt.Sprintf("stuid=%s;stoken=%s;mid=%s", aid, stoken, mid))
	h.Set("x-rpc-app_id", mihoyoBBSAppID)
	h.Set("DS", mihoyoPassportDS("", ""))
	var out passportLtokenData
	mr, err := doMihoyoJSON(client, http.MethodGet, urlMihoyoLTokenBySTK, "", h, &out)
	if err != nil {
		return "", err
	}
	if mr.RetCode != 0 {
		return "", fmt.Errorf("兑换 ltoken 失败: %w", mihoyoRetError(mr))
	}
	return out.Ltoken, nil
}

// fetchGameRoles 取 Cookie 下某游戏的国服角色列表
func fetchGameRoles(client *http.Client, cookie string) ([]gameRole, *mihoyoResp, error) {
	q := url.Values{"game_biz": {genshinGameBiz}}
	var data gameRolesData
	mr, err := doMihoyoJSON(client, http.MethodGet, urlMihoyoGameRoles+"?"+q.Encode(), "", lunaHeader(cookie, genshinSignGame), &data)
	if err != nil {
		return nil, nil, err
	}
	if mr.RetCode != 0 {
		return nil, mr, fmt.Errorf("获取角色失败: %w", mihoyoRetError(mr))
	}
	return data.List, mr, nil
}

// lunaInfo 查询某人某活动签到状态
func lunaInfo(client *http.Client, cookie, region, uid string) (*lunaInfoData, error) {
	q := url.Values{
		"lang":   {mihoyoLang},
		"act_id": {genshinActID},
		"region": {region},
		"uid":    {uid},
	}
	var out lunaInfoData
	mr, err := doMihoyoJSON(client, http.MethodGet, urlMihoyoLunaInfo+"?"+q.Encode(), "", lunaHeader(cookie, genshinSignGame), &out)
	if err != nil {
		return nil, err
	}
	if mr.RetCode != 0 {
		return nil, mihoyoRetError(mr)
	}
	return &out, nil
}

// lunaSign 执行签到
func lunaSign(client *http.Client, cookie, region, uid string) (*lunaSignData, *mihoyoResp, error) {
	body, _ := json.Marshal(map[string]string{
		"act_id": genshinActID,
		"region": region,
		"uid":    uid,
		"lang":   mihoyoLang,
	})
	var out lunaSignData
	mr, err := doMihoyoJSON(client, http.MethodPost, urlMihoyoLunaSign, string(body), lunaHeader(cookie, genshinSignGame), &out)
	if err != nil {
		return nil, nil, err
	}
	return &out, mr, nil
}

// mihoyoCookieHosts 会话 Cookie 的落点域 (jar 序列化/读取/摘要共用)
var mihoyoCookieHosts = []struct{ label, url string }{
	{"passport", "https://passport-api.mihoyo.com/"},
	{"takumi", "https://api-takumi.mihoyo.com/"},
}

// tokenTypesSummary 诊断摘要 (不输出值): token_type + 长度/v2_ 标记
func tokenTypesSummary(tokens []passportQRToken) string {
	if len(tokens) == 0 {
		return "no-tokens"
	}
	parts := make([]string, 0, len(tokens))
	for _, t := range tokens {
		switch {
		case strings.HasPrefix(t.TokenValue, "v2_"):
			parts = append(parts, fmt.Sprintf("type=%d(v2_)", t.TokenType))
		case t.TokenValue != "":
			parts = append(parts, fmt.Sprintf("type=%d(len:%d)", t.TokenType, len(t.TokenValue)))
		default:
			parts = append(parts, fmt.Sprintf("type=%d(empty)", t.TokenType))
		}
	}
	return strings.Join(parts, ", ")
}

// jarCookieSummary 诊断摘要 (不输出值): 会话里的 Cookie 名
func jarCookieSummary(jar *cookiejar.Jar) string {
	var parts []string
	if jar != nil {
		for _, h := range mihoyoCookieHosts {
			if u, err := url.Parse(h.url); err == nil {
				for _, ck := range jar.Cookies(u) {
					parts = append(parts, fmt.Sprintf("%s@%s", ck.Name, h.label))
				}
			}
		}
	}
	if len(parts) == 0 {
		return "no-cookies"
	}
	return strings.Join(parts, ", ")
}

// ===== Cookie 工具 =====

// buildCookie 由凭据拼签到用 Cookie (与 qr_login.py 方案二一致)
func buildCookie(cred *model.MihoyoCredential) string {
	parts := []string{
		fmt.Sprintf("account_id=%s", cred.MihoyoUID),
		fmt.Sprintf("ltuid=%s", cred.MihoyoUID),
	}
	if cred.CookieToken != "" {
		parts = append(parts, "cookie_token="+cred.CookieToken)
	}
	if cred.Stoken != "" {
		parts = append(parts, "stoken="+cred.Stoken)
	}
	if cred.Ltoken != "" {
		parts = append(parts, "ltoken="+cred.Ltoken)
	}
	return strings.Join(parts, "; ")
}

// jarCookieValue 会话 Cookie 取值; names 优先级从高到低 (v2 优先于旧名), 在任一落点域命即返回
func jarCookieValue(jar *cookiejar.Jar, names ...string) string {
	if jar == nil {
		return ""
	}
	for _, h := range mihoyoCookieHosts {
		u, err := url.Parse(h.url)
		if err != nil {
			continue
		}
		byName := make(map[string]string, 8)
		for _, ck := range jar.Cookies(u) {
			byName[ck.Name] = ck.Value
		}
		for _, name := range names {
			if v, ok := byName[name]; ok && v != "" {
				return v
			}
		}
	}
	return ""
}
