package weixin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"miniclaw2/internal/config"
)

const (
	defaultLoginBotType     = "3"
	defaultLoginTimeout     = 8 * time.Minute
	loginPollTimeout        = 35 * time.Second
	loginStatusRefreshDelay = time.Second
	loginFixedBaseURL       = DefaultAPIBaseURL
)

type QRLoginOptions struct {
	Timeout time.Duration
	Verbose bool
	Logf    func(string)
	OnQRReady func(QRLoginStartResult)
}

type QRLoginStartResult struct {
	QRCode    string
	QRCodeURL string
	Message   string
}

type QRLoginResult struct {
	AccountID    string
	RawAccountID string
	UserID       string
	BaseURL      string
	Token        string
	QRCodeURL    string
	Message      string
}

type qrCodeResponse struct {
	QRCode    string `json:"qrcode"`
	QRCodeURL string `json:"qrcode_img_content"`
}

type qrStatusResponse struct {
	Status       string `json:"status"`
	BotToken     string `json:"bot_token"`
	ILinkBotID   string `json:"ilink_bot_id"`
	BaseURL      string `json:"baseurl"`
	ILinkUserID  string `json:"ilink_user_id"`
	RedirectHost string `json:"redirect_host"`
}

type loginSession struct {
	QRCode         string
	QRCodeURL      string
	CurrentBaseURL string
	RefreshCount   int
}

func LoginWithQR(ctx context.Context, cfg config.Config, options QRLoginOptions) (QRLoginResult, error) {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultLoginTimeout
	}
	start, err := StartQRLogin(ctx, cfg, options)
	if err != nil {
		return QRLoginResult{}, err
	}
	if options.OnQRReady != nil {
		options.OnQRReady(start)
	}
	session := loginSession{
		QRCode:         start.QRCode,
		QRCodeURL:      start.QRCodeURL,
		CurrentBaseURL: loginFixedBaseURL,
		RefreshCount:   1,
	}
	logLine(options.Logf, "等待扫码确认...")
	result, err := waitForQRLogin(ctx, cfg, session, timeout, options)
	if err != nil {
		return QRLoginResult{}, err
	}
	normalized := NormalizeAccountID(result.RawAccountID)
	if normalized == "" {
		return QRLoginResult{}, fmt.Errorf("weixin login succeeded but account id is empty")
	}
	if _, err := SaveAccount(cfg, normalized, AccountData{
		RawAccountID: result.RawAccountID,
		Token:        result.Token,
		SavedAt:      time.Now().Format(time.RFC3339),
		BaseURL:      firstNonEmpty(result.BaseURL, DefaultAPIBaseURL),
		UserID:       result.UserID,
	}); err != nil {
		return QRLoginResult{}, err
	}
	result.AccountID = normalized
	result.QRCodeURL = firstNonEmpty(result.QRCodeURL, start.QRCodeURL)
	return result, nil
}

func StartQRLogin(ctx context.Context, cfg config.Config, options QRLoginOptions) (QRLoginStartResult, error) {
	response := qrCodeResponse{}
	endpoint := "ilink/bot/get_bot_qrcode?bot_type=" + url.QueryEscape(defaultLoginBotType)
	if err := doLoginGET(ctx, cfg.RequestTimeout, loginFixedBaseURL, endpoint, &response); err != nil {
		return QRLoginStartResult{}, err
	}
	if strings.TrimSpace(response.QRCode) == "" || strings.TrimSpace(response.QRCodeURL) == "" {
		return QRLoginStartResult{}, fmt.Errorf("weixin login QR response is missing qrcode data")
	}
	logLine(options.Logf, "请在浏览器中打开以下二维码页面，再用微信扫码完成登录：")
	logLine(options.Logf, response.QRCodeURL)
	return QRLoginStartResult{QRCode: response.QRCode, QRCodeURL: response.QRCodeURL, Message: "scan the QR code URL with Weixin"}, nil
}

func waitForQRLogin(ctx context.Context, cfg config.Config, session loginSession, timeout time.Duration, options QRLoginOptions) (QRLoginResult, error) {
	deadline := time.Now().Add(timeout)
	current := session
	for time.Now().Before(deadline) {
		status, err := pollQRStatus(ctx, current.CurrentBaseURL, current.QRCode)
		if err != nil {
			if ctx.Err() != nil {
				return QRLoginResult{}, ctx.Err()
			}
			logLine(options.Logf, "二维码状态轮询失败，继续重试: "+err.Error())
			if !sleepWithContext(ctx, loginStatusRefreshDelay) {
				return QRLoginResult{}, ctx.Err()
			}
			continue
		}
		switch status.Status {
		case "", "wait":
			if options.Verbose {
				logLine(options.Logf, "等待扫码中...")
			}
		case "scaned":
			logLine(options.Logf, "已扫码，请在微信里确认授权...")
		case "scaned_but_redirect":
			if strings.TrimSpace(status.RedirectHost) != "" {
				current.CurrentBaseURL = "https://" + strings.TrimSpace(status.RedirectHost)
				logLine(options.Logf, "检测到登录 IDC 跳转，已切换轮询主机到 "+current.CurrentBaseURL)
			}
		case "expired":
			current.RefreshCount++
			logLine(options.Logf, fmt.Sprintf("二维码已过期，正在刷新 (%d)...", current.RefreshCount))
			start, err := StartQRLogin(ctx, cfg, options)
			if err != nil {
				return QRLoginResult{}, err
			}
			current.QRCode = start.QRCode
			current.QRCodeURL = start.QRCodeURL
			current.CurrentBaseURL = loginFixedBaseURL
		case "confirmed":
			if strings.TrimSpace(status.ILinkBotID) == "" || strings.TrimSpace(status.BotToken) == "" {
				return QRLoginResult{}, fmt.Errorf("登录成功但返回的账号或 token 为空")
			}
			return QRLoginResult{
				RawAccountID: strings.TrimSpace(status.ILinkBotID),
				UserID:       strings.TrimSpace(status.ILinkUserID),
				BaseURL:      firstNonEmpty(strings.TrimSpace(status.BaseURL), current.CurrentBaseURL, DefaultAPIBaseURL),
				Token:        strings.TrimSpace(status.BotToken),
				QRCodeURL:    current.QRCodeURL,
				Message:      "与微信连接成功",
			}, nil
		default:
			if options.Verbose {
				logLine(options.Logf, "登录状态: "+status.Status)
			}
		}
		if !sleepWithContext(ctx, loginStatusRefreshDelay) {
			return QRLoginResult{}, ctx.Err()
		}
	}
	return QRLoginResult{}, fmt.Errorf("登录超时，请重试")
}

func pollQRStatus(ctx context.Context, baseURL, qrcode string) (qrStatusResponse, error) {
	pollCtx, cancel := context.WithTimeout(ctx, loginPollTimeout)
	defer cancel()
	result := qrStatusResponse{}
	endpoint := "ilink/bot/get_qrcode_status?qrcode=" + url.QueryEscape(qrcode)
	if err := doLoginGET(pollCtx, 0, baseURL, endpoint, &result); err != nil {
		if pollCtx.Err() == context.DeadlineExceeded {
			return qrStatusResponse{Status: "wait"}, nil
		}
		return qrStatusResponse{}, err
	}
	return result, nil
}

func doLoginGET(ctx context.Context, requestTimeout int, baseURL, endpoint string, target any) error {
	client := &http.Client{}
	if requestTimeout > 0 {
		client.Timeout = time.Duration(requestTimeout) * time.Second
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/"+strings.TrimLeft(endpoint, "/"), nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("weixin login api error %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if target == nil {
		return nil
	}
	return json.Unmarshal(body, target)
}

func logLine(logf func(string), message string) {
	if logf != nil && strings.TrimSpace(message) != "" {
		logf(message)
	}
}
