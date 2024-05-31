package gateway

import (
	"net/http"

	"github.com/jhump/protoreflect/desc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// RpcHandler Rpc处理
type RpcHandler interface {
	OnReceiveResponse(string, metadata.MD, http.ResponseWriter) string
	OnReceiveTrailers(*status.Status, metadata.MD) metadata.MD
	OnResolveMethod(*desc.MethodDescriptor)
	OnSendHeaders(*http.Request, metadata.MD) metadata.MD
	OnReceiveHeaders(metadata.MD) metadata.MD
}

var _ RpcHandler = new(BasicRpcHandler)

// BasicRpcHandler RPC Handler 的基本实现
type BasicRpcHandler struct {
}

func (h *BasicRpcHandler) OnReceiveResponse(respJson string, _ metadata.MD, _ http.ResponseWriter) string {
	return respJson
}

func (h *BasicRpcHandler) OnReceiveTrailers(_ *status.Status, md metadata.MD) metadata.MD {
	return md
}

func (h *BasicRpcHandler) OnResolveMethod(_ *desc.MethodDescriptor) {
}

func (h *BasicRpcHandler) OnSendHeaders(_ *http.Request, md metadata.MD) metadata.MD {
	return md
}

func (h *BasicRpcHandler) OnReceiveHeaders(md metadata.MD) metadata.MD {
	return md
}
