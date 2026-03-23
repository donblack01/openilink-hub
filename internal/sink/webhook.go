package sink

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/openilink/openilink-hub/internal/database"
	"github.com/openilink/openilink-hub/internal/provider"
)

// Webhook pushes messages to a configured HTTP endpoint.
//
// Script API (two-phase middleware):
//
//	// ctx.msg  — inbound message (read-only)
//	// ctx.req  — {url, method, headers, body} (modify before send)
//	// ctx.res  — {status, headers, body} (available in onResponse)
//	// ctx.reply(text) — send reply back to user via bot
//	// ctx.skip()      — cancel this webhook delivery
//	//
//	// Export two optional functions:
//	function onRequest(ctx) {
//	    ctx.req.headers["X-Custom"] = "value";
//	    ctx.req.body = JSON.stringify({text: ctx.msg.content});
//	}
//	function onResponse(ctx) {
//	    var data = JSON.parse(ctx.res.body);
//	    if (data.answer) ctx.reply(data.answer);
//	}
type Webhook struct {
	DB *database.DB
}

func (s *Webhook) Name() string { return "webhook" }

func (s *Webhook) Handle(d Delivery) {
	cfg := d.Channel.WebhookConfig
	if cfg.URL == "" {
		return
	}

	msg := buildPayload(d)
	body, _ := json.Marshal(msg)
	req := &reqData{
		URL:     cfg.URL,
		Method:  "POST",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    string(body),
	}
	applyAuth(req, cfg.Auth, body)

	var res *resData
	var replies []string
	skipped := false

	// Resolve script: plugin_id takes precedence over inline script
	script := cfg.Script
	if cfg.PluginID != "" {
		plugin, err := s.DB.GetPlugin(cfg.PluginID)
		if err != nil {
			slog.Error("webhook plugin not found", "channel", d.Channel.ID, "plugin", cfg.PluginID, "err", err)
		} else if plugin.Status != "approved" {
			slog.Warn("webhook plugin not approved", "channel", d.Channel.ID, "plugin", cfg.PluginID, "status", plugin.Status)
		} else {
			script = plugin.Script
		}
	}

	if script != "" {
		var err error
		req, res, replies, skipped, err = s.runScript(script, msg, req, d.Channel.ID)
		if err != nil {
			slog.Error("webhook script error", "channel", d.Channel.ID, "err", err)
			return
		}
		if skipped || req == nil {
			return
		}
	}

	// Send HTTP request (if script didn't already trigger it via onResponse)
	if res == nil {
		res = doHTTP(req, d.Channel.ID)
	}

	// Auto-reply from response {"reply": "..."}
	if res != nil && len(replies) == 0 {
		var body struct{ Reply string }
		if json.Unmarshal([]byte(res.Body), &body) == nil && body.Reply != "" {
			replies = append(replies, body.Reply)
		}
	}

	s.sendReplies(d, replies)
}

const (
	scriptTimeout      = 5 * time.Second
	scriptMaxCallStack = 64
	scriptMaxReplies   = 10
)

// runScriptWithTimeout runs a goja function with a timeout guard.
// If the script exceeds the deadline, the VM is interrupted.
func runScriptWithTimeout(vm *goja.Runtime, fn goja.Callable, args ...goja.Value) (goja.Value, error) {
	done := make(chan struct{})
	timer := time.AfterFunc(scriptTimeout, func() {
		vm.Interrupt("script execution timeout")
	})
	defer timer.Stop()

	var result goja.Value
	var err error
	go func() {
		defer close(done)
		result, err = fn(goja.Undefined(), args...)
	}()
	<-done

	// Clear interrupt for potential reuse (won't happen but good hygiene)
	vm.ClearInterrupt()
	return result, err
}

// parseScriptPerms extracts @grant, @match, @connect from the script metadata.
func parseScriptPerms(script string) (grants map[string]bool, matchTypes map[string]bool, connectDomains map[string]bool) {
	grants = map[string]bool{}
	matchTypes = map[string]bool{"*": true} // default: all
	connectDomains = map[string]bool{"*": true}

	metaRe := regexp.MustCompile(`//\s*@(\w+)\s+(.+)`)
	hasGrant, hasMatch, hasConnect := false, false, false

	for _, line := range strings.Split(script, "\n") {
		m := metaRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		switch m[1] {
		case "grant":
			if !hasGrant {
				hasGrant = true
				grants = map[string]bool{}
			}
			for _, g := range strings.Split(strings.TrimSpace(m[2]), ",") {
				grants[strings.TrimSpace(g)] = true
			}
		case "match":
			if !hasMatch {
				hasMatch = true
				matchTypes = map[string]bool{}
			}
			for _, t := range strings.Split(strings.TrimSpace(m[2]), ",") {
				matchTypes[strings.TrimSpace(t)] = true
			}
		case "connect":
			if !hasConnect {
				hasConnect = true
				connectDomains = map[string]bool{}
			}
			for _, d := range strings.Split(strings.TrimSpace(m[2]), ",") {
				connectDomains[strings.TrimSpace(d)] = true
			}
		}
	}
	return
}

func (s *Webhook) runScript(script string, msg webhookPayload, req *reqData, channelID string) (
	outReq *reqData, outRes *resData, replies []string, skipped bool, err error,
) {
	// Parse permissions from script metadata
	grants, matchTypes, connectDomains := parseScriptPerms(script)

	// @match enforcement: skip if message type doesn't match
	if !matchTypes["*"] && !matchTypes[msg.MsgType] {
		return req, nil, nil, false, nil // pass through without running script
	}

	vm := goja.New()
	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
	vm.SetMaxCallStackSize(scriptMaxCallStack)

	// Sandbox: remove potentially dangerous globals
	for _, name := range []string{"eval", "Function"} {
		vm.GlobalObject().Delete(name)
	}

	// Build ctx
	ctx := map[string]any{
		"msg": msg,
		"req": map[string]any{
			"url":     req.URL,
			"method":  req.Method,
			"headers": req.Headers,
			"body":    req.Body,
		},
	}
	vm.Set("ctx", ctx)

	// @grant enforcement: only expose granted APIs
	if grants["reply"] || grants["none"] == false && len(grants) == 0 {
		// grant reply or no @grant declared (backward compat: all allowed)
		vm.Set("reply", func(text string) {
			if len(replies) < scriptMaxReplies {
				replies = append(replies, text)
			}
		})
	} else {
		vm.Set("reply", func(text string) {
			panic(vm.NewGoError(fmt.Errorf("reply() not granted — add @grant reply")))
		})
	}

	if grants["skip"] || grants["none"] == false && len(grants) == 0 {
		vm.Set("skip", func() { skipped = true })
	} else {
		vm.Set("skip", func() {
			panic(vm.NewGoError(fmt.Errorf("skip() not granted — add @grant skip")))
		})
	}

	// If @grant none, block both
	if grants["none"] {
		vm.Set("reply", func(text string) {
			panic(vm.NewGoError(fmt.Errorf("reply() blocked by @grant none")))
		})
		vm.Set("skip", func() {
			panic(vm.NewGoError(fmt.Errorf("skip() blocked by @grant none")))
		})
	}

	_ = connectDomains // used after script execution

	// Define onRequest/onResponse as top-level functions (with timeout)
	timer := time.AfterFunc(scriptTimeout, func() {
		vm.Interrupt("script parse timeout")
	})
	_, err = vm.RunString(script)
	timer.Stop()
	vm.ClearInterrupt()
	if err != nil {
		return nil, nil, nil, false, err
	}

	// Phase 1: onRequest
	if fn := vm.Get("onRequest"); fn != nil && !goja.IsUndefined(fn) {
		if callable, ok := goja.AssertFunction(fn); ok {
			if _, err := runScriptWithTimeout(vm, callable, vm.Get("ctx")); err != nil {
				return nil, nil, nil, false, err
			}
		}
	}

	if skipped {
		return nil, nil, replies, true, nil
	}

	// Extract modified req from ctx
	outReq = extractReqFromCtx(vm.Get("ctx").Export(), req)

	// @connect enforcement: validate URL domain
	if !connectDomains["*"] && outReq.URL != req.URL {
		if !isDomainAllowed(outReq.URL, connectDomains) {
			return nil, nil, nil, false, fmt.Errorf("URL domain not in @connect whitelist: %s", outReq.URL)
		}
	}

	// Execute HTTP
	outRes = doHTTP(outReq, channelID)

	// Phase 2: onResponse
	if outRes != nil {
		if fn := vm.Get("onResponse"); fn != nil && !goja.IsUndefined(fn) {
			if callable, ok := goja.AssertFunction(fn); ok {
				ctxObj := vm.Get("ctx").ToObject(vm)
				ctxObj.Set("res", map[string]any{
					"status":  outRes.Status,
					"headers": outRes.Headers,
					"body":    outRes.Body,
				})
				if _, err := runScriptWithTimeout(vm, callable, vm.Get("ctx")); err != nil {
					slog.Error("webhook onResponse error", "channel", channelID, "err", err)
				}
			}
		}
	}

	return outReq, outRes, replies, false, nil
}

func (s *Webhook) sendReplies(d Delivery, replies []string) {
	for _, text := range replies {
		if text == "" {
			continue
		}
		_, err := d.Provider.Send(context.Background(), provider.OutboundMessage{
			Recipient: d.Message.Sender,
			Text:      text,
		})
		if err != nil {
			slog.Error("webhook reply failed", "channel", d.Channel.ID, "err", err)
			continue
		}
		chID := d.Channel.ID
		itemList, _ := json.Marshal([]map[string]any{{"type": "text", "text": text}})
		s.DB.SaveMessage(&database.Message{
			BotID:       d.BotDBID,
			ChannelID:   &chID,
			Direction:   "outbound",
			ToUserID:    d.Message.Sender,
			MessageType: 2,
			ItemList:    itemList,
		})
	}
}

// --- Types ---

type reqData struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type resData struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type webhookPayload struct {
	Event     string        `json:"event"`
	ChannelID string        `json:"channel_id"`
	BotID     string        `json:"bot_id"`
	SeqID     int64         `json:"seq_id"`
	Sender    string        `json:"sender"`
	MsgType   string        `json:"msg_type"`
	Content   string        `json:"content"`
	Timestamp int64         `json:"timestamp"`
	Items     []webhookItem `json:"items"`
}

type webhookItem struct {
	Type     string `json:"type"`               // "text", "image", "voice", "file", "video"
	Text     string `json:"text,omitempty"`
	FileName string `json:"file_name,omitempty"`
	MediaURL string `json:"media_url,omitempty"` // download URL (MinIO or CDN proxy)
	FileSize int64  `json:"file_size,omitempty"`
	// Voice
	PlayTime int `json:"play_time,omitempty"`
	// Video
	PlayLength  int `json:"play_length,omitempty"`
	ThumbWidth  int `json:"thumb_width,omitempty"`
	ThumbHeight int `json:"thumb_height,omitempty"`
	// Quoted message
	RefTitle string       `json:"ref_title,omitempty"`
	RefItem  *webhookItem `json:"ref_item,omitempty"`
}

func buildPayload(d Delivery) webhookPayload {
	items := make([]webhookItem, len(d.Message.Items))
	for i, item := range d.Message.Items {
		items[i] = convertWebhookItem(item)
	}
	return webhookPayload{
		Event: "message", ChannelID: d.Channel.ID, BotID: d.BotDBID,
		SeqID: d.SeqID, Sender: d.Message.Sender, MsgType: d.MsgType,
		Content: d.Content, Timestamp: d.Message.Timestamp, Items: items,
	}
}

func convertWebhookItem(item provider.MessageItem) webhookItem {
	wi := webhookItem{
		Type:     item.Type,
		Text:     item.Text,
		FileName: item.FileName,
	}
	if item.Media != nil {
		wi.MediaURL = item.Media.URL
		wi.FileSize = item.Media.FileSize
		wi.PlayTime = item.Media.PlayTime
		wi.PlayLength = item.Media.PlayLength
		wi.ThumbWidth = item.Media.ThumbWidth
		wi.ThumbHeight = item.Media.ThumbHeight
	}
	if item.RefMsg != nil {
		wi.RefTitle = item.RefMsg.Title
		ref := convertWebhookItem(item.RefMsg.Item)
		wi.RefItem = &ref
	}
	return wi
}

// --- HTTP helpers ---

func applyAuth(req *reqData, auth *database.WebhookAuth, body []byte) {
	if auth == nil {
		return
	}
	switch auth.Type {
	case "bearer":
		req.Headers["Authorization"] = "Bearer " + auth.Token
	case "header":
		req.Headers[auth.Name] = auth.Value
	case "hmac":
		mac := hmac.New(sha256.New, []byte(auth.Secret))
		mac.Write(body)
		req.Headers["X-Hub-Signature"] = "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}
}

func doHTTP(req *reqData, channelID string) *resData {
	httpReq, err := http.NewRequest(req.Method, req.URL, bytes.NewReader([]byte(req.Body)))
	if err != nil {
		slog.Error("webhook build failed", "channel", channelID, "err", err)
		return nil
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		slog.Error("webhook delivery failed", "channel", channelID, "err", err)
		return nil
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	headers := make(map[string]string)
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}
	if resp.StatusCode >= 400 {
		slog.Warn("webhook error status", "channel", channelID, "status", resp.StatusCode)
	}
	return &resData{Status: resp.StatusCode, Headers: headers, Body: string(respBody)}
}

func extractReqFromCtx(obj any, fallback *reqData) *reqData {
	m, ok := obj.(map[string]any)
	if !ok {
		return fallback
	}
	rm, ok := m["req"].(map[string]any)
	if !ok {
		return fallback
	}
	out := &reqData{URL: fallback.URL, Method: fallback.Method, Headers: make(map[string]string), Body: fallback.Body}
	for k, v := range fallback.Headers {
		out.Headers[k] = v
	}
	if u, ok := rm["url"].(string); ok && u != "" {
		out.URL = u
	}
	if m, ok := rm["method"].(string); ok && m != "" {
		out.Method = m
	}
	if b, ok := rm["body"].(string); ok {
		out.Body = b
	}
	if h, ok := rm["headers"].(map[string]any); ok {
		for k, v := range h {
			if vs, ok := v.(string); ok {
				out.Headers[k] = vs
			}
		}
	}
	return out
}

// isDomainAllowed checks if a URL's host is in the allowed domains set.
func isDomainAllowed(rawURL string, allowed map[string]bool) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if allowed[host] {
		return true
	}
	// Check if any allowed domain is a suffix (e.g. ".feishu.cn" matches "open.feishu.cn")
	for domain := range allowed {
		if strings.HasSuffix(host, "."+domain) || host == domain {
			return true
		}
	}
	return false
}
