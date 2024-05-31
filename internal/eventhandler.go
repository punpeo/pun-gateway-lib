package internal

import (
	"fmt"
	"io"
	"strconv"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/desc"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type EventHandler struct {
	Status        *status.Status
	writer        io.Writer
	marshaler     jsonpb.Marshaler
	XStatusCode   uint32
	XErrorMessage string
}

func NewEventHandler(writer io.Writer, resolver jsonpb.AnyResolver) *EventHandler {
	return &EventHandler{
		writer: writer,
		marshaler: jsonpb.Marshaler{
			EmitDefaults: true,
			AnyResolver:  resolver,
		},
	}
}

func (h *EventHandler) OnReceiveResponse(message proto.Message) {
	//if err := header.marshaler.Marshal(header.writer, message); err != nil {
	//	logx.Error(err)
	//}
	//gateway 调整返回值
	jsonStr, err := h.marshaler.MarshalToString(message)
	if err != nil {
		logx.Error(err)
	}
	//成功返回 --- 最好转成struct 待优化。。。 //todo
	respCode := 1000
	respMsg := "成功"
	if h.XStatusCode != 0 {
		respCode = int(h.XStatusCode)
	}
	if h.XErrorMessage != "" {
		respMsg = h.XErrorMessage
	}
	successStrPrefix := fmt.Sprintf("{\"code\":%d, \"msg\": \"%s\", \"data\":", respCode, respMsg)
	io.WriteString(h.writer, successStrPrefix)
	io.WriteString(h.writer, jsonStr)
	io.WriteString(h.writer, "}")
}

func (h *EventHandler) OnReceiveTrailers(status *status.Status, _ metadata.MD) {
	h.Status = status
}

func (h *EventHandler) OnResolveMethod(_ *desc.MethodDescriptor) {
}

func (h *EventHandler) OnSendHeaders(_ metadata.MD) {
}

func (h *EventHandler) OnReceiveHeaders(md metadata.MD) {
	xStatusCodeArr := md.Get("X-Status-Code")
	if len(xStatusCodeArr) > 0 {
		codeStr := xStatusCodeArr[0]
		codeInt64, _ := strconv.ParseInt(codeStr, 10, 64)
		h.XStatusCode = uint32(codeInt64)
	}
	xErrorMessage := md.Get("X-Error-Message")
	if len(xErrorMessage) > 0 {
		h.XErrorMessage = xErrorMessage[0]
	}
}
