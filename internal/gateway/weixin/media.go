package weixin

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/md5"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	messageItemTypeText  = 1
	messageItemTypeImage = 2
	messageItemTypeFile  = 4
	messageItemTypeVideo = 5
	messageTypeBot       = 2
	messageStateFinish   = 2
	mediaTypeImage       = 1
	mediaTypeVideo       = 2
	mediaTypeFile        = 3
)

type UploadInfo struct {
	DownloadEncryptedQueryParam string
	AESKey                      []byte
	FileSize                    int
	FileSizeCiphertext          int
	FileName                    string
	MediaKind                   int
}

type RenderedReply struct {
	Text         string
	MediaSources []string
}

func ParseRenderedReply(content string) RenderedReply {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	textLines := make([]string, 0, len(lines))
	mediaSources := []string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(trimmed), "MEDIA:") {
			source := strings.TrimSpace(trimmed[len("MEDIA:"):])
			if source != "" {
				mediaSources = append(mediaSources, source)
			}
			continue
		}
		textLines = append(textLines, line)
	}
	return RenderedReply{Text: strings.TrimSpace(strings.Join(textLines, "\n")), MediaSources: mediaSources}
}

func (c *Client) SendRenderedReply(ctx context.Context, content, targetUserID, contextToken string, sequence int) error {
	reply := ParseRenderedReply(content)
	if sequence <= 1 || len(reply.MediaSources) == 0 {
		return c.SendTextMessage(ctx, targetUserID, contextToken, content)
	}
	for index, source := range reply.MediaSources {
		caption := ""
		if index == 0 {
			caption = reply.Text
		}
		if err := c.SendMediaSource(ctx, targetUserID, contextToken, caption, source); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) SendMediaSource(ctx context.Context, targetUserID, contextToken, caption, mediaSource string) error {
	filePath, cleanup, err := c.resolveMediaSource(ctx, mediaSource)
	if err != nil {
		return err
	}
	defer cleanup()
	upload, err := c.uploadFile(ctx, filePath, targetUserID)
	if err != nil {
		return err
	}
	return c.sendUploadedMedia(ctx, targetUserID, contextToken, caption, upload)
}

func (c *Client) resolveMediaSource(ctx context.Context, mediaSource string) (string, func(), error) {
	trimmed := strings.TrimSpace(mediaSource)
	if trimmed == "" {
		return "", func() {}, fmt.Errorf("media source is empty")
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		path, err := c.downloadRemoteMediaToTemp(ctx, trimmed)
		if err != nil {
			return "", func() {}, err
		}
		return path, func() { _ = os.Remove(path) }, nil
	}
	if strings.HasPrefix(trimmed, "file://") {
		trimmed = strings.TrimPrefix(trimmed, "file://")
	}
	if !filepath.IsAbs(trimmed) {
		absolute, err := filepath.Abs(trimmed)
		if err != nil {
			return "", func() {}, err
		}
		trimmed = absolute
	}
	return trimmed, func() {}, nil
}

func (c *Client) downloadRemoteMediaToTemp(ctx context.Context, mediaURL string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return "", err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("remote media download failed: %d %s", response.StatusCode, strings.TrimSpace(response.Status))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(c.mediaTempDir(), 0o755); err != nil {
		return "", err
	}
	ext := extensionFromContentTypeOrURL(response.Header.Get("Content-Type"), mediaURL)
	file, err := os.CreateTemp(c.mediaTempDir(), "weixin-remote-*"+ext)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := file.Write(body); err != nil {
		return "", err
	}
	return file.Name(), nil
}

func (c *Client) uploadFile(ctx context.Context, filePath, targetUserID string) (UploadInfo, error) {
	plaintext, err := os.ReadFile(filePath)
	if err != nil {
		return UploadInfo{}, err
	}
	mimeType := mimeFromFilename(filePath)
	mediaType, itemType := mediaAndItemKindFromMime(mimeType)
	aesKey := make([]byte, 16)
	if _, err := cryptorand.Read(aesKey); err != nil {
		return UploadInfo{}, err
	}
	fileKeyBytes := make([]byte, 16)
	if _, err := cryptorand.Read(fileKeyBytes); err != nil {
		return UploadInfo{}, err
	}
	fileKey := hex.EncodeToString(fileKeyBytes)
	rawMD5 := md5.Sum(plaintext)
	fileSizeCiphertext := aesEcbPaddedSize(len(plaintext))
	payload := map[string]any{
		"filekey":       fileKey,
		"media_type":    mediaType,
		"to_user_id":    targetUserID,
		"rawsize":       len(plaintext),
		"rawfilemd5":    hex.EncodeToString(rawMD5[:]),
		"filesize":      fileSizeCiphertext,
		"no_need_thumb": true,
		"aeskey":        hex.EncodeToString(aesKey),
		"base_info":     c.baseInfo(),
	}
	var response struct {
		Ret           int    `json:"ret"`
		ErrCode       int    `json:"errcode"`
		ErrMsg        string `json:"errmsg"`
		UploadParam   string `json:"upload_param"`
		UploadFullURL string `json:"upload_full_url"`
	}
	if err := c.postJSON(ctx, "ilink/bot/getuploadurl", payload, &response); err != nil {
		return UploadInfo{}, err
	}
	if response.Ret != 0 {
		return UploadInfo{}, fmt.Errorf("weixin getuploadurl api error %d: %s", response.ErrCode, strings.TrimSpace(response.ErrMsg))
	}
	downloadParam, err := c.uploadCiphertextToCDN(ctx, plaintext, response.UploadFullURL, response.UploadParam, fileKey, aesKey)
	if err != nil {
		return UploadInfo{}, err
	}
	return UploadInfo{
		DownloadEncryptedQueryParam: downloadParam,
		AESKey:                      aesKey,
		FileSize:                    len(plaintext),
		FileSizeCiphertext:          fileSizeCiphertext,
		FileName:                    filepath.Base(filePath),
		MediaKind:                   itemType,
	}, nil
}

func (c *Client) uploadCiphertextToCDN(ctx context.Context, plaintext []byte, uploadFullURL, uploadParam, fileKey string, aesKey []byte) (string, error) {
	ciphertext, err := encryptAESECB(plaintext, aesKey)
	if err != nil {
		return "", err
	}
	uploadURL := strings.TrimSpace(uploadFullURL)
	if uploadURL == "" {
		if strings.TrimSpace(uploadParam) == "" {
			return "", fmt.Errorf("weixin CDN upload URL is missing")
		}
		uploadURL = strings.TrimRight(c.account.CDNBaseURL, "/") + "/upload?encrypted_query_param=" + url.QueryEscape(uploadParam) + "&filekey=" + url.QueryEscape(fileKey)
	}
	for attempt := 0; attempt < 3; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(ciphertext))
		if err != nil {
			return "", err
		}
		request.Header.Set("Content-Type", "application/octet-stream")
		response, err := c.httpClient.Do(request)
		if err != nil {
			if attempt == 2 {
				return "", err
			}
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		response.Body.Close()
		if readErr != nil {
			return "", readErr
		}
		if response.StatusCode >= 400 && response.StatusCode < 500 {
			message := firstNonEmpty(response.Header.Get("x-error-message"), strings.TrimSpace(string(body)))
			return "", fmt.Errorf("weixin CDN upload client error %d: %s", response.StatusCode, message)
		}
		if response.StatusCode != http.StatusOK {
			if attempt == 2 {
				message := firstNonEmpty(response.Header.Get("x-error-message"), strings.TrimSpace(string(body)), response.Status)
				return "", fmt.Errorf("weixin CDN upload server error: %s", message)
			}
			continue
		}
		downloadParam := strings.TrimSpace(response.Header.Get("x-encrypted-param"))
		if downloadParam == "" {
			return "", fmt.Errorf("weixin CDN upload response missing x-encrypted-param header")
		}
		return downloadParam, nil
	}
	return "", fmt.Errorf("weixin CDN upload failed after retries")
}

func (c *Client) sendUploadedMedia(ctx context.Context, targetUserID, contextToken, caption string, upload UploadInfo) error {
	if strings.TrimSpace(caption) != "" {
		if err := c.SendTextMessage(ctx, targetUserID, contextToken, caption); err != nil {
			return err
		}
	}
	item := map[string]any{"type": upload.MediaKind}
	switch upload.MediaKind {
	case messageItemTypeImage:
		item["image_item"] = map[string]any{
			"media": map[string]any{
				"encrypt_query_param": upload.DownloadEncryptedQueryParam,
				"aes_key":             base64.StdEncoding.EncodeToString(upload.AESKey),
				"encrypt_type":        1,
			},
			"mid_size": upload.FileSizeCiphertext,
		}
	case messageItemTypeVideo:
		item["video_item"] = map[string]any{
			"media": map[string]any{
				"encrypt_query_param": upload.DownloadEncryptedQueryParam,
				"aes_key":             base64.StdEncoding.EncodeToString(upload.AESKey),
				"encrypt_type":        1,
			},
			"video_size": upload.FileSizeCiphertext,
		}
	default:
		item["file_item"] = map[string]any{
			"media": map[string]any{
				"encrypt_query_param": upload.DownloadEncryptedQueryParam,
				"aes_key":             base64.StdEncoding.EncodeToString(upload.AESKey),
				"encrypt_type":        1,
			},
			"file_name": upload.FileName,
			"len":       strconv.Itoa(upload.FileSize),
		}
	}
	return c.sendMessageItems(ctx, targetUserID, contextToken, []map[string]any{item})
}

func (c *Client) sendMessageItems(ctx context.Context, targetUserID, contextToken string, items []map[string]any) error {
	for _, item := range items {
		payload := map[string]any{
			"msg": map[string]any{
				"from_user_id":  "",
				"to_user_id":    targetUserID,
				"client_id":     newWeixinClientID(),
				"message_type":  messageTypeBot,
				"message_state": messageStateFinish,
				"item_list":     []map[string]any{item},
				"context_token": emptyToNil(contextToken),
			},
			"base_info": c.baseInfo(),
		}
		if err := c.postJSON(ctx, "ilink/bot/sendmessage", payload, nil); err != nil {
			return err
		}
	}
	return nil
}

func mimeFromFilename(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func extensionFromContentTypeOrURL(contentType, rawURL string) string {
	mimeType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch mimeType {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "application/pdf":
		return ".pdf"
	case "application/zip":
		return ".zip"
	case "text/plain":
		return ".txt"
	}
	ext := path.Ext(rawURL)
	if ext == "" {
		return ".bin"
	}
	return ext
}

func mediaAndItemKindFromMime(mimeType string) (int, int) {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return mediaTypeImage, messageItemTypeImage
	case strings.HasPrefix(mimeType, "video/"):
		return mediaTypeVideo, messageItemTypeVideo
	default:
		return mediaTypeFile, messageItemTypeFile
	}
}

func aesEcbPaddedSize(size int) int {
	return ((size + 1 + aes.BlockSize - 1) / aes.BlockSize) * aes.BlockSize
}

func encryptAESECB(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padding := aes.BlockSize - (len(plaintext) % aes.BlockSize)
	if padding == 0 {
		padding = aes.BlockSize
	}
	padded := append(append([]byte{}, plaintext...), bytes.Repeat([]byte{byte(padding)}, padding)...)
	ciphertext := make([]byte, len(padded))
	for offset := 0; offset < len(padded); offset += aes.BlockSize {
		block.Encrypt(ciphertext[offset:offset+aes.BlockSize], padded[offset:offset+aes.BlockSize])
	}
	return ciphertext, nil
}

func newWeixinClientID() string {
	buffer := make([]byte, 8)
	if _, err := cryptorand.Read(buffer); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(buffer)
}

func emptyToNil(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}
