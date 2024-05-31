package plugins

import (
	gateway "github.com/punpeo/pun-gateway-lib"
	"github.com/zeromicro/go-zero/rest"
)

// PluginEmpty 空插件，为屏蔽某一路由的插件使用
type PluginEmpty struct {
	gateway.BasicRpcHandler
}

func NewPluginEmpty() *PluginEmpty {
	return &PluginEmpty{}
}

func (p *PluginEmpty) Name() string {
	return "empty"
}

func (p *PluginEmpty) Middleware() rest.Middleware {
	return nil
}
