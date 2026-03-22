package sink

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
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

	if cfg.Script != "" {
		var err error
		req, res, replies, skipped, err = s.runScript(cfg.Script, msg, req, d.Channel.ID)
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

func (s *Webhook) runScript(script string, msg webhookPayload, req *reqData, channelID string) (
	outReq *reqData, outRes *resData, replies []string, skipped bool, err error,
) {
	vm := goja.New()
	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))

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
	vm.Set("reply", func(text string) { replies = append(replies, text) })
	vm.Set("skip", func() { skipped = true })

	// Define onRequest/onResponse as top-level functions
	_, err = vm.RunString(script)
	if err != nil {
		return nil, nil, nil, false, err
	}

	// Phase 1: onRequest
	if fn := vm.Get("onRequest"); fn != nil && !goja.IsUndefined(fn) {
		if callable, ok := goja.AssertFunction(fn); ok {
			if _, err := callable(goja.Undefined(), vm.Get("ctx")); err != nil {
				return nil, nil, nil, false, err
			}
		}
	}

	if skipped {
		return nil, nil, replies, true, nil
	}

	// Extract modified req from ctx
	outReq = extractReqFromCtx(vm.Get("ctx").Export(), req)

	// Execute HTTP
	outRes = doHTTP(outReq, channelID)

	// Phase 2: onResponse
	if outRes != nil {
		if fn := vm.Get("onResponse"); fn != nil && !goja.IsUndefined(fn) {
			if callable, ok := goja.AssertFunction(fn); ok {
				// Set ctx.res
				ctxObj := vm.Get("ctx").ToObject(vm)
				ctxObj.Set("res", map[string]any{
					"status":  outRes.Status,
					"headers": outRes.Headers,
					"body":    outRes.Body,
				})
				if _, err := callable(goja.Undefined(), vm.Get("ctx")); err != nil {
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
		payload, _ := json.Marshal(map[string]string{"content": text})
		s.DB.SaveMessage(&database.Message{
			BotID:     d.BotDBID,
			ChannelID: &chID,
			Direction: "outbound",
			Recipient: d.Message.Sender,
			MsgType:   "text",
			Payload:   payload,
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
	Event     string                 `json:"event"`
	ChannelID string                 `json:"channel_id"`
	BotID     string                 `json:"bot_id"`
	SeqID     int64                  `json:"seq_id"`
	Sender    string                 `json:"sender"`
	MsgType   string                 `json:"msg_type"`
	Content   string                 `json:"content"`
	Timestamp int64                  `json:"timestamp"`
	Items     []provider.MessageItem `json:"items"`
}

func buildPayload(d Delivery) webhookPayload {
	return webhookPayload{
		Event: "message", ChannelID: d.Channel.ID, BotID: d.BotDBID,
		SeqID: d.SeqID, Sender: d.Message.Sender, MsgType: d.MsgType,
		Content: d.Content, Timestamp: d.Message.Timestamp, Items: d.Message.Items,
	}
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
