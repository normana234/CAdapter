package plugin

import (
	"chatgpt-adapter/internal/common"
	"chatgpt-adapter/internal/gin.handler/response"
	"chatgpt-adapter/internal/vars"
	"chatgpt-adapter/logger"
	"chatgpt-adapter/pkg"
	"fmt"
	"github.com/bincooo/emit.io"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/gin-gonic/gin"
	socketio "github.com/zishang520/socket.io/socket"
)

var (
	HTTPClient     *emit.Session
	ClashAPIClient *emit.Session

	IO *socketio.Server
)

type Model struct {
	Id      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	By      string `json:"owned_by"`
}

func init() {
	common.AddInitialized(func() {
		var err error
		options := common.GetIdleConnectOptions()
		connTimeout := pkg.Config.GetInt("server-conn.connTimeout")
		if connTimeout == 0 {
			connTimeout = 180
		}

		options = append(options, emit.Ja3Helper(emit.Echo{RandomTLSExtension: true, HelloID: profiles.Chrome_124}, connTimeout))
		HTTPClient, err = emit.NewSession(vars.Proxies, emit.SimpleWithes("127.0.0.1"), options...)
		if err != nil {
			logger.Error("Error initializing HTTPClient: ", err)
		}

		IO = socketio.NewServer(nil, nil)

		if value := pkg.Config.GetString("clash.proxies"); value != "" {
			ClashAPIClient, err = emit.NewSession(vars.Proxies, emit.SimpleWithes("127.0.0.1"), options...)
			if err != nil {
				logger.Error("Error initializing ClashAPIClient: ", err)
			}
		}
	})
}

type Adapter interface {
	Match(ctx *gin.Context, model string) bool
	Models() []Model
	Completion(ctx *gin.Context)
	Generation(ctx *gin.Context)
	Embedding(ctx *gin.Context)
	HandleMessages(ctx *gin.Context) (messages []pkg.Keyv[interface{}], err error)
}

type BaseAdapter struct {
}

type ExtensionAdapter struct {
	slice []Adapter
}

func (BaseAdapter) Models() []Model {
	return nil
}

func (BaseAdapter) Completion(*gin.Context) {
}

func (BaseAdapter) Generation(*gin.Context) {
}

func (BaseAdapter) Embedding(*gin.Context) {}

func (BaseAdapter) HandleMessages(ctx *gin.Context) (messages []pkg.Keyv[interface{}], err error) {
	var (
		completion = common.GetGinCompletion(ctx)
	)

	messages = completion.Messages
	return
}

func NewGlobalAdapter() *ExtensionAdapter {
	return &ExtensionAdapter{
		slice: make([]Adapter, 0),
	}
}

func (adapter *ExtensionAdapter) Match(ctx *gin.Context, model string) bool {
	for _, extension := range adapter.slice {
		if extension.Match(ctx, model) {
			return true
		}
	}
	return false
}

func (adapter *ExtensionAdapter) Models() (models []Model) {
	for _, extension := range adapter.slice {
		models = append(models, extension.Models()...)
	}
	return
}

func (adapter *ExtensionAdapter) Completion(ctx *gin.Context) {
	completion := common.GetGinCompletion(ctx)
	for _, extension := range adapter.slice {
		if extension.Match(ctx, completion.Model) {
			messages, err := extension.HandleMessages(ctx)
			if err != nil {
				logger.Error("Error handling messages: ", err)
				response.Error(ctx, 500, err)
				return
			}

			completion.Messages = messages
			ctx.Set(vars.GinCompletion, completion)
			extension.Completion(ctx)
			return
		}
	}
	response.Error(ctx, -1, fmt.Sprintf("model '%s' is not not yet supported", completion.Model))
}

func (adapter *ExtensionAdapter) Embedding(ctx *gin.Context) {
	embedding := common.GetGinEmbedding(ctx)
	for _, extension := range adapter.slice {
		if extension.Match(ctx, embedding.Model) {
			extension.Embedding(ctx)
			return
		}
	}
	response.Error(ctx, -1, fmt.Sprintf("model '%s' is not not yet supported", embedding.Model))
}

func (adapter *ExtensionAdapter) Messages(ctx *gin.Context) {
	completion := common.GetGinCompletion(ctx)
	for _, extension := range adapter.slice {
		if extension.Match(ctx, completion.Model) {
			exec, ok := extension.(interface{ Messages(ctx *gin.Context) })
			if ok {
				exec.Messages(ctx)
				return
			}
		}
	}
	response.Error(ctx, -1, fmt.Sprintf("model '%s' is not not yet supported", completion.Model))
}

func (adapter *ExtensionAdapter) Generation(ctx *gin.Context) {
	completion := common.GetGinGeneration(ctx)
	for _, extension := range adapter.slice {
		if extension.Match(ctx, completion.Model) {
			extension.Generation(ctx)
			return
		}
	}
}

func (adapter *ExtensionAdapter) Add(adapters ...Adapter) {
	adapter.slice = append(adapter.slice, adapters...)
}
