package gemini

import (
	"chatgpt-adapter/internal/common"
	"chatgpt-adapter/internal/gin.handler/response"
	"chatgpt-adapter/internal/plugin"
	"chatgpt-adapter/internal/vars"
	"chatgpt-adapter/logger"
	"chatgpt-adapter/pkg"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

const MODEL = "gemini"

var (
	Adapter = API{}
)

type candidatesResponse struct {
	Candidates []candidate `json:"candidates"`
}

type candidate struct {
	Content struct {
		Role  string                   `json:"role"`
		Parts []map[string]interface{} `json:"parts"`
	} `json:"content"`
	FinishReason string `json:"finishReason"`
	Index        int    `json:"index"`
}

type API struct {
	plugin.BaseAdapter
}

func (API) Match(_ *gin.Context, model string) bool {
	switch model {
	case "gemini-1.0-pro-latest",
		"gemini-1.5-pro-latest",
		"gemini-1.5-pro-002",
		"gemini-1.5-flash-latest",
		"gemini-1.5-flash-002",
		"gemini-1.5-pro-exp-0801",
		"gemini-1.5-pro-exp-0827",
		"models/text-embedding-004":
		return true
	default:
		return false
	}
}

func (API) Models() (slice []plugin.Model) {
	for _, model := range []string{
		"gemini-1.0-pro-latest",
		"gemini-1.5-pro-latest",
		"gemini-1.5-pro-002",
		"gemini-1.5-flash-latest",
		"gemini-1.5-flash-002",
		"gemini-1.5-pro-exp-0801",
		"gemini-1.5-pro-exp-0827",
		"models/text-embedding-004",
	} {
		slice = append(slice, plugin.Model{
			Id:      model,
			Object:  "model",
			Created: 1686935002,
			By:      "gemini-adapter",
		})
	}
	return
}

func (API) Completion(ctx *gin.Context) {
	var (
		cookie     = ctx.GetString("token")
		proxies    = ctx.GetString("proxies")
		completion = common.GetGinCompletion(ctx)
		matchers   = common.GetGinMatchers(ctx)
		echo       = ctx.GetBool(vars.GinEcho)
	)

	newMessages, tokens, err := mergeMessages(completion.Messages)
	if err != nil {
		logger.Error(err)
		response.Error(ctx, -1, err)
		return
	}

	if echo {
		bytes, _ := json.MarshalIndent(newMessages, "", "  ")
		response.Echo(ctx, completion.Model, string(bytes), completion.Stream)
		return
	}

	tc := pkg.Config.GetBool("google.tc")
	if tc && plugin.NeedToToolCall(ctx) {
		if completeToolCalls(ctx, cookie, proxies, completion) {
			return
		}
	}
	if tc {
		completion.Tools = nil
		completion.ToolChoice = nil
	}

	ctx.Set(ginTokens, tokens)
	r, err := build(common.GetGinContext(ctx), proxies, cookie, newMessages, completion)
	if err != nil {
		var urlError *url.Error
		if errors.As(err, &urlError) {
			urlError.URL = strings.ReplaceAll(urlError.URL, cookie, "AIzaSy***")
		}
		logger.Error(err)
		response.Error(ctx, -1, err)
		return
	}

	// 最近似乎很容易发送空消息？
	content := waitResponse(ctx, matchers, r, completion.Stream)
	if content == "" && response.NotResponse(ctx) {
		response.Error(ctx, -1, "EMPTY RESPONSE")
	}
}
