package gateway

import (
	"io"
	"net/http"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/desc"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// GrpcChainHandler 实现 gRPC 的 handler interface
type GrpcChainHandler struct {
	writer    http.ResponseWriter
	request   *http.Request
	marshaler jsonpb.Marshaler
	chains    []RpcHandler
	Status    *status.Status

	respHeader metadata.MD
}

// OnResolveMethod is called with a descriptor of the method that is being invoked.
func (h *GrpcChainHandler) OnResolveMethod(desc *desc.MethodDescriptor) {
	for _, chn := range h.chains {
		if nil == chn {
			continue
		}
		chn.OnResolveMethod(desc)
	}
}

// OnSendHeaders is called with the request metadata that is being sent.
func (h *GrpcChainHandler) OnSendHeaders(md metadata.MD) {
	for _, chn := range h.chains {
		if nil == chn {
			continue
		}
		md = chn.OnSendHeaders(h.request, md)
	}
}

// OnReceiveHeaders is called when response headers have been received.
func (h *GrpcChainHandler) OnReceiveHeaders(md metadata.MD) {
	for _, chn := range h.chains {
		if nil == chn {
			continue
		}
		md = chn.OnReceiveHeaders(md)
	}
	h.respHeader = md
}

// OnReceiveResponse is called for each response message received.
func (h *GrpcChainHandler) OnReceiveResponse(message proto.Message) {
	resp, err := h.marshaler.MarshalToString(message)
	if err != nil {
		logx.Error(err)
	}

	for _, chn := range h.chains {
		if nil == chn {
			continue
		}
		resp = chn.OnReceiveResponse(resp, h.respHeader, h.writer)
	}

	_, _ = io.WriteString(h.writer, resp)
}

// OnReceiveTrailers is called when response trailers and final RPC status have been received.
func (h *GrpcChainHandler) OnReceiveTrailers(status *status.Status, md metadata.MD) {
	h.Status = status
	for _, chn := range h.chains {
		if nil == chn {
			continue
		}
		md = chn.OnReceiveTrailers(status, md)
	}
}
