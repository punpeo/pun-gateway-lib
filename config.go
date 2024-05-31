package gateway

import (
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
)

type (
	// GatewayConf is the configuration for gateway.
	GatewayConf struct {
		rest.RestConf
		Upstreams []Upstream
		//管理后台相关权限rpc服务
		AccessControlRpc zrpc.RpcClientConf
		//是否校验强制登录map
		AuthCheckMapping map[string]map[string]bool `json:",optional"`
		//是否校验功能权限map
		VerifyFuncControlMapping map[string]map[string]bool `json:",optional"`
		//路由配置Map
		UpstreamsRouteMap map[string]map[string]RouteMapping
		//app、h5的security_key签名
		Safe Safe
		//php内部调用sign
		SignKey string
	}

	// RouteMapping is a mapping between a gateway route and an upstream rpc method.
	RouteMapping struct {
		// Method is the HTTP method, like GET, POST, PUT, DELETE.
		Method string
		// Path is the HTTP path.
		Path string
		// RpcPath is the gRPC rpc method, with format of package.service/method
		RpcPath string
		// AuthCheck token检查，默认为检查
		AuthCheck bool `json:",optional,default=true"`
		// Plugins 单一路由的插件，将完全覆盖全局插件
		Plugins []string `json:",optional"`
		// OrigName 单一路由控制 是否启用OriginName  未配置则使用 Upstream.OrigName
		OrigName *bool `json:",optional"`
		// VerifyFuncControl 功能权限检查，默认为不检查
		VerifyFuncControl bool        `json:",optional,default=false"`
		UriDispatch       UriDispatch `json:",optional"`
	}

	// Upstream is the configuration for an upstream.
	Upstream struct {
		// Name is the name of the upstream.
		Name string `json:",optional"`
		// Grpc is the target of the upstream.
		Grpc zrpc.RpcClientConf
		// ProtoSets is the file list of proto set, like [hello.pb].
		// if your proto file import another proto file, you need to write multi-file slice,
		// like [hello.pb, common.pb].
		ProtoSets []string `json:",optional"`
		// Mappings is the mapping between gateway routes and Upstream rpc methods.
		// Keep it blank if annotations are added in rpc methods.
		Mappings []RouteMapping `json:",optional"`
		// Plugins 全局插件
		Plugins []string `json:",optional"`
		// OrigName  是否启用OriginName 默认不开启
		OrigName bool `json:",optional,default=false"`
	}

	Safe struct {
		Key string
		Iv  string
	}

	UriDispatch struct {
		//调度方案 0-直连go服务  1-用户灰度方案(新旧接口，不同服务) 2-兜底双请求校验 3-直连php服务 4-内部版本灰度(同服务接口，不同版本) 5-灰度＋兜底
		DispatchRule int8 `json:",optional,default=0"`
		//兜底优先 [缺省]0-php  2-go
		Priority int8 `json:",optional,default=0"`
		//灰度方案 1-用户取模 userId % GrayDivisor 2-配置模式 (指定用户ID) 3-配置模式 (指定ip)
		GrayScheme int8 `json:",optional"`
		//灰度比例 GrayScheme == 1 用户模: [0,1,2,3]
		GrayRate []int8 `json:",optional"`
		//灰度取模 除数
		GrayDivisor int16 `json:",optional,default=100"`
		//直连php host
		DirectHost string `json:",optional"`
		//直连php地址
		DirectPath string `json:",optional"`
		//配置模式 文件路径
		GrayConfigPath string `json:",optional"`
		//内部版本灰度 [缺省]0-php  1-go, 增加灰度头 canary:1
		DispatchServer int8 `json:",optional"`
	}
)
