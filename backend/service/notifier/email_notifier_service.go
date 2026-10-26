package service

import (
	iface "backend/interface"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SMTPConfig SMTP 配置缓存
type SMTPConfig struct {
	Host       string
	Port       int
	Encryption string
	User       string
	Password   string
	From       string
	FromName   string
}

// smtpDialTimeout 建立 TCP/TLS 连接的超时
const smtpDialTimeout = 10 * time.Second

// smtpSessionTimeout 覆盖整个 SMTP 会话 (连接建立后) 的截止时间, 防止对端无响应时发送 goroutine 永久挂起
const smtpSessionTimeout = 60 * time.Second

// EmailNotifier 邮件通知器
type EmailNotifier struct {
	configSvc *SystemConfigService
	mu        sync.RWMutex
	smtpCache *SMTPConfig
}

// NewEmailNotifier 创建邮件通知器
func NewEmailNotifier(configSvc *SystemConfigService) *EmailNotifier {
	n := &EmailNotifier{
		configSvc: configSvc,
	}
	n.RefreshConfig()
	return n
}

// Name 返回通知器名称
func (n *EmailNotifier) Name() string {
	return "email"
}

// RefreshConfig 刷新 SMTP 配置缓存
func (n *EmailNotifier) RefreshConfig() {
	host, _ := n.configSvc.GetValue("smtp_host")
	portStr, _ := n.configSvc.GetValue("smtp_port")
	encryption, _ := n.configSvc.GetValue("smtp_encryption")
	user, _ := n.configSvc.GetValue("smtp_user")
	password, _ := n.configSvc.GetValue("smtp_password")
	from, _ := n.configSvc.GetValue("smtp_from")
	fromName, _ := n.configSvc.GetValue("smtp_from_name")

	port := 587
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}

	if fromName == "" {
		fromName = "系统通知"
	}

	cfg := &SMTPConfig{
		Host:       host,
		Port:       port,
		Encryption: encryption,
		User:       user,
		Password:   password,
		From:       from,
		FromName:   fromName,
	}

	n.mu.Lock()
	n.smtpCache = cfg
	n.mu.Unlock()
}

// stripCRLF 去除 CR/LF, 防止邮件头注入 (Subject 来自备忘标题, To 来自用户邮箱, 均为用户可控内容)
func stripCRLF(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// Send 发送邮件通知
func (n *EmailNotifier) Send(to string, msg iface.NotifyMessage) error {
	n.mu.RLock()
	cfg := n.smtpCache
	n.mu.RUnlock()

	if cfg.Host == "" || cfg.User == "" || cfg.Password == "" || cfg.From == "" {
		return fmt.Errorf("SMTP 配置不完整，请先填写 SMTP 主机、用户名、密码和发件人邮箱")
	}

	safeTo := stripCRLF(to)
	data := fmt.Sprintf("From: %s <%s>\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\nMIME-Version: 1.0\r\n\r\n%s",
		stripCRLF(cfg.FromName), stripCRLF(cfg.From), safeTo, stripCRLF(msg.Subject), msg.Body)

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	auth := smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)
	dialer := &net.Dialer{Timeout: smtpDialTimeout}

	var conn net.Conn
	var err error
	tryStartTLS := true
	switch cfg.Encryption {
	case "ssl":
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: cfg.Host})
		tryStartTLS = false // 已在 TLS 层内, 不能再 STARTTLS
	case "starttls", "":
		conn, err = dialer.Dial("tcp", addr)
	default:
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("连接 SMTP 服务器失败: %w", err)
	}

	return n.sendOverConn(conn, cfg.Host, auth, cfg.From, safeTo, data, tryStartTLS)
}

// sendOverConn 在已建立的连接上完成 SMTP 会话 (认证/发信), 整个会话限时
func (n *EmailNotifier) sendOverConn(conn net.Conn, host string, auth smtp.Auth, from, to, data string, tryStartTLS bool) error {
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(smtpSessionTimeout)); err != nil {
		return fmt.Errorf("设置 SMTP 会话超时失败: %w", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("创建 SMTP 客户端失败: %w", err)
	}
	defer client.Close()

	if tryStartTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err = client.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return fmt.Errorf("启动 STARTTLS 失败: %w", err)
			}
		}
	}

	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP 认证失败: %w", err)
	}
	if err = client.Mail(from); err != nil {
		return fmt.Errorf("设置发件人失败: %w", err)
	}
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("设置收件人失败: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("获取数据 writer 失败: %w", err)
	}
	if _, err = w.Write([]byte(data)); err != nil {
		return fmt.Errorf("写入邮件内容失败: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("关闭数据 writer 失败: %w", err)
	}

	return client.Quit()
}
