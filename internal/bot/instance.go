package bot

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	ilink "github.com/openilink/openilink-sdk-go"
	"github.com/openilink/openilink-hub/internal/database"
)

// Instance wraps an iLink client with its monitor goroutine.
type Instance struct {
	DBID   string // DB UUID
	BotID  string // ilink_bot_id
	Client *ilink.Client
	cancel context.CancelFunc
	status atomic.Value // string
	mu     sync.Mutex   // protects Client.Push/SendText
}

func NewInstance(bot *database.Bot) *Instance {
	opts := []ilink.Option{}
	if bot.BaseURL != "" {
		opts = append(opts, ilink.WithBaseURL(bot.BaseURL))
	}
	client := ilink.NewClient(bot.BotToken, opts...)

	inst := &Instance{
		DBID:   bot.ID,
		BotID:  bot.BotID,
		Client: client,
	}
	inst.status.Store("disconnected")
	return inst
}

func (i *Instance) Status() string  { return i.status.Load().(string) }
func (i *Instance) SetStatus(s string) { i.status.Store(s) }

// SendText sends a message with mutex protection for concurrent sublevel pushes.
func (i *Instance) SendText(ctx context.Context, to, text string) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.Client.Push(ctx, to, text)
}

// Start begins the Monitor loop in a goroutine.
func (i *Instance) Start(ctx context.Context, db *database.DB, onMessage func(inst *Instance, msg ilink.WeixinMessage), onStatusChange func(inst *Instance, status string)) {
	ctx, i.cancel = context.WithCancel(ctx)
	i.SetStatus("connected")
	_ = db.UpdateBotStatus(i.DBID, "connected")
	if onStatusChange != nil {
		onStatusChange(i, "connected")
	}

	bot, _ := db.GetBot(i.DBID)
	initialBuf := ""
	if bot != nil {
		initialBuf = bot.SyncBuf
	}

	go func() {
		err := i.Client.Monitor(ctx, func(msg ilink.WeixinMessage) {
			onMessage(i, msg)
		}, &ilink.MonitorOptions{
			InitialBuf: initialBuf,
			OnBufUpdate: func(buf string) {
				_ = db.UpdateBotSyncBuf(i.DBID, buf)
			},
			OnError: func(err error) {
				slog.Warn("bot monitor error", "bot", i.DBID, "err", err)
			},
			OnSessionExpired: func() {
				slog.Error("bot session expired", "bot", i.DBID)
				i.SetStatus("session_expired")
				_ = db.UpdateBotStatus(i.DBID, "session_expired")
				if onStatusChange != nil {
					onStatusChange(i, "session_expired")
				}
			},
		})
		var newStatus string
		if err != nil && err != context.Canceled {
			slog.Error("bot monitor stopped", "bot", i.DBID, "err", err)
			newStatus = "error"
		} else {
			newStatus = "disconnected"
		}
		i.SetStatus(newStatus)
		_ = db.UpdateBotStatus(i.DBID, newStatus)
		if onStatusChange != nil {
			onStatusChange(i, newStatus)
		}
	}()
}

func (i *Instance) Stop() {
	if i.cancel != nil {
		i.cancel()
	}
}
