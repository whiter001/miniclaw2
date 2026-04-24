package weixin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"miniclaw2/internal/config"
)

const (
	DefaultAPIBaseURL = "https://ilinkai.weixin.qq.com"
	DefaultCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
)

var weixinAccountFileMu sync.Mutex

type AccountData struct {
	RawAccountID string `json:"raw_account_id,omitempty"`
	Token        string `json:"token,omitempty"`
	SavedAt      string `json:"saved_at,omitempty"`
	BaseURL      string `json:"base_url,omitempty"`
	UserID       string `json:"user_id,omitempty"`
}

type Account struct {
	ID           string
	RawAccountID string
	Token        string
	BaseURL      string
	CDNBaseURL   string
	UserID       string
	SavedAt      string
	Configured   bool
}

func NormalizeAccountID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), "-")
}

func ResolveAccount(cfg config.Config) (Account, error) {
	if strings.TrimSpace(cfg.WeixinToken) != "" {
		id := NormalizeAccountID(cfg.WeixinAccountID)
		if id == "" {
			id = "manual"
		}
		return Account{
			ID:           id,
			RawAccountID: firstNonEmpty(strings.TrimSpace(cfg.WeixinAccountID), id),
			Token:        strings.TrimSpace(cfg.WeixinToken),
			BaseURL:      firstNonEmpty(strings.TrimSpace(cfg.WeixinAPIBase), DefaultAPIBaseURL),
			CDNBaseURL:   firstNonEmpty(strings.TrimSpace(cfg.WeixinCDNBase), DefaultCDNBaseURL),
			Configured:   true,
		}, nil
	}
	selected := NormalizeAccountID(firstNonEmpty(strings.TrimSpace(cfg.WeixinAccountID), LoadActiveAccountID(cfg)))
	ids, err := ListAccountIDs(cfg)
	if err != nil {
		return Account{}, err
	}
	if selected == "" {
		switch len(ids) {
		case 0:
			return Account{}, fmt.Errorf("weixin gateway is not configured yet.\nrun `miniclaw gateway login --channel weixin` first, or set weixin_token in ~/.config/miniclaw/config.")
		case 1:
			selected = ids[0]
		default:
			return Account{}, fmt.Errorf("multiple weixin accounts are registered; set weixin_account_id in ~/.config/miniclaw/config or pass --account")
		}
	}
	data, err := LoadAccount(cfg, selected)
	if err != nil {
		return Account{}, err
	}
	if strings.TrimSpace(data.Token) == "" {
		return Account{}, fmt.Errorf("weixin account %s is missing a token; rerun `miniclaw gateway login --channel weixin`", selected)
	}
	resolved := Account{
		ID:           selected,
		RawAccountID: firstNonEmpty(strings.TrimSpace(data.RawAccountID), selected),
		Token:        strings.TrimSpace(data.Token),
		BaseURL:      firstNonEmpty(strings.TrimSpace(cfg.WeixinAPIBase), strings.TrimSpace(data.BaseURL), DefaultAPIBaseURL),
		CDNBaseURL:   firstNonEmpty(strings.TrimSpace(cfg.WeixinCDNBase), DefaultCDNBaseURL),
		UserID:       strings.TrimSpace(data.UserID),
		SavedAt:      strings.TrimSpace(data.SavedAt),
		Configured:   true,
	}
	if err := SetActiveAccountID(cfg, selected); err != nil {
		return Account{}, err
	}
	return resolved, nil
}

func ListAccountIDs(cfg config.Config) ([]string, error) {
	data, err := os.ReadFile(resolveAccountIndexPath(cfg))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var parsed []string
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(parsed))
	seen := map[string]struct{}{}
	for _, item := range parsed {
		normalized := NormalizeAccountID(item)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result, nil
}

func LoadAccount(cfg config.Config, accountID string) (AccountData, error) {
	normalized := NormalizeAccountID(accountID)
	if normalized == "" {
		return AccountData{}, fmt.Errorf("weixin account id is empty")
	}
	data, err := os.ReadFile(resolveAccountPath(cfg, normalized))
	if err != nil {
		if os.IsNotExist(err) {
			return AccountData{}, os.ErrNotExist
		}
		return AccountData{}, err
	}
	var parsed AccountData
	if err := json.Unmarshal(data, &parsed); err != nil {
		return AccountData{}, err
	}
	return parsed, nil
}

func SaveAccount(cfg config.Config, accountID string, update AccountData) (string, error) {
	normalized := NormalizeAccountID(accountID)
	if normalized == "" {
		return "", fmt.Errorf("weixin account id is empty")
	}
	if err := os.MkdirAll(resolveAccountsDir(cfg), 0o755); err != nil {
		return "", err
	}
	weixinAccountFileMu.Lock()
	defer weixinAccountFileMu.Unlock()
	existing, err := readAccountFile(resolveAccountPath(cfg, normalized))
	if err != nil {
		return "", err
	}
	merged := AccountData{
		RawAccountID: firstNonEmpty(strings.TrimSpace(update.RawAccountID), strings.TrimSpace(existing.RawAccountID), normalized),
		Token:        firstNonEmpty(strings.TrimSpace(update.Token), strings.TrimSpace(existing.Token)),
		SavedAt:      firstNonEmpty(strings.TrimSpace(update.SavedAt), strings.TrimSpace(existing.SavedAt)),
		BaseURL:      firstNonEmpty(strings.TrimSpace(update.BaseURL), strings.TrimSpace(existing.BaseURL)),
		UserID:       firstNonEmpty(strings.TrimSpace(update.UserID), strings.TrimSpace(existing.UserID)),
	}
	if strings.TrimSpace(update.Token) != "" && strings.TrimSpace(merged.SavedAt) == "" {
		merged.SavedAt = time.Now().Format(time.RFC3339)
	}
	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return "", err
	}
	path := resolveAccountPath(cfg, normalized)
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	if err := registerAccountID(cfg, normalized); err != nil {
		return "", err
	}
	if err := SetActiveAccountID(cfg, normalized); err != nil {
		return "", err
	}
	return path, nil
}

func ClearAccount(cfg config.Config, accountID string) error {
	normalized := NormalizeAccountID(accountID)
	if normalized == "" {
		return fmt.Errorf("weixin account id is empty")
	}
	weixinAccountFileMu.Lock()
	defer weixinAccountFileMu.Unlock()
	_ = os.Remove(resolveAccountPath(cfg, normalized))
	_ = os.Remove(resolveCursorPath(cfg, normalized))
	if err := unregisterAccountID(cfg, normalized); err != nil {
		return err
	}
	if LoadActiveAccountID(cfg) == normalized {
		_ = os.Remove(resolveActiveAccountPath(cfg))
	}
	return nil
}

func LoadActiveAccountID(cfg config.Config) string {
	data, err := os.ReadFile(resolveActiveAccountPath(cfg))
	if err != nil {
		return ""
	}
	return NormalizeAccountID(string(data))
}

func SetActiveAccountID(cfg config.Config, accountID string) error {
	normalized := NormalizeAccountID(accountID)
	if normalized == "" {
		return fmt.Errorf("weixin account id is empty")
	}
	if err := os.MkdirAll(resolveWeixinStateDir(cfg), 0o755); err != nil {
		return err
	}
	return os.WriteFile(resolveActiveAccountPath(cfg), []byte(normalized+"\n"), 0o644)
}

func resolveWeixinStateDir(cfg config.Config) string {
	return filepath.Join(cfg.Workspace, "state", "weixin")
}

func resolveAccountsDir(cfg config.Config) string {
	return filepath.Join(resolveWeixinStateDir(cfg), "accounts")
}

func resolveAccountPath(cfg config.Config, accountID string) string {
	return filepath.Join(resolveAccountsDir(cfg), NormalizeAccountID(accountID)+".json")
}

func resolveAccountIndexPath(cfg config.Config) string {
	return filepath.Join(resolveWeixinStateDir(cfg), "accounts.json")
}

func resolveActiveAccountPath(cfg config.Config) string {
	return filepath.Join(resolveWeixinStateDir(cfg), "active_account.txt")
}

func readAccountFile(path string) (AccountData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return AccountData{}, nil
		}
		return AccountData{}, err
	}
	var parsed AccountData
	if err := json.Unmarshal(data, &parsed); err != nil {
		return AccountData{}, err
	}
	return parsed, nil
}

func registerAccountID(cfg config.Config, accountID string) error {
	ids, err := ListAccountIDs(cfg)
	if err != nil {
		return err
	}
	normalized := NormalizeAccountID(accountID)
	for _, item := range ids {
		if item == normalized {
			return nil
		}
	}
	ids = append(ids, normalized)
	return writeAccountIDs(cfg, ids)
}

func unregisterAccountID(cfg config.Config, accountID string) error {
	ids, err := ListAccountIDs(cfg)
	if err != nil {
		return err
	}
	normalized := NormalizeAccountID(accountID)
	filtered := make([]string, 0, len(ids))
	for _, item := range ids {
		if item != normalized {
			filtered = append(filtered, item)
		}
	}
	return writeAccountIDs(cfg, filtered)
}

func writeAccountIDs(cfg config.Config, ids []string) error {
	if err := os.MkdirAll(resolveWeixinStateDir(cfg), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ids, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(resolveAccountIndexPath(cfg), append(data, '\n'), 0o644)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
