package uridispatch

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/google/go-cmp/cmp"
	json "github.com/json-iterator/go"
	gateway "github.com/punpeo/pun-gateway-lib"
	"github.com/punpeo/punpeo-lib/rest/restyclient"
	"github.com/punpeo/punpeo-lib/rest/result"
	"github.com/punpeo/punpeo-lib/rest/xerr"
	"github.com/samber/lo"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/metric"
	"github.com/zeromicro/go-zero/core/threading"
	"github.com/zeromicro/go-zero/rest/httpx"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type IUriProcessor interface {
	Process(w http.ResponseWriter, r *http.Request, next http.Handler)
}

type UriDispatch struct {
	RouteConfig  gateway.RouteMapping
	UriProcessor IUriProcessor
}

var Mode string

var grayUserMap sync.Map

func NewUriDispatch(routeConfig gateway.RouteMapping) *UriDispatch {
	return &UriDispatch{
		RouteConfig: routeConfig,
	}
}
func (h *UriDispatch) SetDispatchHandler(d IUriProcessor) {
	h.UriProcessor = d
}
func (h *UriDispatch) Handler(w http.ResponseWriter, r *http.Request, next http.Handler) {
	switch h.RouteConfig.UriDispatch.DispatchRule {
	case 1: //1-用户接口迁移灰度方案
		isGray, err := h.isGray(w, r)
		if err != nil {
			logx.Errorf("路由调度错误, %+v", errors.Unwrap(err))
			return
		}
		if isGray {
			h.SetDispatchHandler(NewDirectGoServer())
		} else {
			h.SetDispatchHandler(NewDirectPhpSource(h.RouteConfig.UriDispatch.DirectHost, h.RouteConfig.UriDispatch.DirectPath))
		}
	case 2: // 2-兜底双请求校验
		h.SetDispatchHandler(NewReserveServer(NewDirectGoServer(), NewDirectPhpSource(h.RouteConfig.UriDispatch.DirectHost, h.RouteConfig.UriDispatch.DirectPath), h.RouteConfig.UriDispatch.Priority))
	case 3: //3-直连php服务
		h.SetDispatchHandler(NewDirectPhpSource(h.RouteConfig.UriDispatch.DirectHost, h.RouteConfig.UriDispatch.DirectPath))
	case 4: //4-内部接口版本灰度方案
		newServerFn, ok := newServerFuncMap[h.RouteConfig.UriDispatch.DispatchServer]
		if !ok {
			logx.Errorf("调度服务不存在")
			return
		}
		server := newServerFn(h.RouteConfig, r)
		isGray, err := h.isGray(w, r)
		if err != nil {
			logx.Errorf("路由调度错误, %+v", errors.Unwrap(err))
			return
		}
		if isGray {
			r.Header.Set("canary", "1")
		}
		h.SetDispatchHandler(server)

	case 5: //5-灰度＋兜底
		isGray, err := h.isGray(w, r)
		if err != nil {
			logx.Errorf("路由调度错误, %+v", errors.Unwrap(err))
			return
		}
		if isGray {
			//灰度走兜底
			h.SetDispatchHandler(NewReserveServer(NewDirectGoServer(), NewDirectPhpSource(h.RouteConfig.UriDispatch.DirectHost, h.RouteConfig.UriDispatch.DirectPath), h.RouteConfig.UriDispatch.Priority))
		} else {
			//非灰度 直连php
			h.SetDispatchHandler(NewDirectPhpSource(h.RouteConfig.UriDispatch.DirectHost, h.RouteConfig.UriDispatch.DirectPath))
		}
	case 0: //直连go服务
		fallthrough
	default:
		h.SetDispatchHandler(NewDirectGoServer())
	}
	if h.UriProcessor == nil {
		logx.Error("未设置调度器")
		httpx.WriteJson(w, http.StatusInternalServerError, nil)
		return
	}
	h.UriProcessor.Process(w, r, next)
}

func (h *UriDispatch) isGray(_ http.ResponseWriter, r *http.Request) (bool, error) {
	ids := getValueByCtx(r.Context(), "uid")
	uid, _ := strconv.ParseInt(ids, 10, 64)
	uri := strings.Replace(r.URL.Path, "//", "/", 1)
	var userBucket []string
	grayDivisor := h.RouteConfig.UriDispatch.GrayDivisor
	if grayDivisor == 0 {
		grayDivisor = 100
	}
	switch h.RouteConfig.UriDispatch.GrayScheme {
	case 1:
		uidMod := uid % int64(grayDivisor)
		//按用户取模
		if uid > 0 && lo.Contains(h.RouteConfig.UriDispatch.GrayRate, int8(uidMod)) {
			//用户id取模
			log(r, "info", 200, fmt.Sprintf("接口进入灰度,userId : %d", uid))

			return true, nil
		}
	case 2:
		if uid > 0 {
			key := fmt.Sprintf("userId:%s:%s", h.RouteConfig.Method, uri)
			// 按配置加载用户ID
			iUserGrayBucket, ok := grayUserMap.Load(key)
			if !ok {
				content, err := readFile(h.RouteConfig.UriDispatch.GrayConfigPath)
				if err != nil {
					logx.Errorf("加载配置文件失败,%+v", err)
					return false, err
				}
				userBucket = strings.Split(content, ",")
				userBucket = lo.Uniq(userBucket)
				grayUserMap.Store(key, userBucket)
			} else {
				userBucket = iUserGrayBucket.([]string)
			}
			if lo.Contains(userBucket, strconv.FormatInt(uid, 10)) {
				log(r, "info", 200, fmt.Sprintf("接口进入灰度,userId : %d", uid))
				return true, nil
			}
		}
	case 3:
		ip := ClientIP(r)
		if ip != "" {
			key := fmt.Sprintf("ip:%s:%s", h.RouteConfig.Method, uri)
			// 按配置加载用户ID
			iUserGrayBucket, ok := grayUserMap.Load(key)
			if !ok {
				content, err := readFile(h.RouteConfig.UriDispatch.GrayConfigPath)
				if err != nil {
					logx.Errorf("加载配置文件失败,%+v", err)
					return false, err
				}
				userBucket = strings.Split(content, ",")
				userBucket = lo.Uniq(userBucket)
				grayUserMap.Store(key, userBucket)
			} else {
				userBucket = iUserGrayBucket.([]string)
			}
			if lo.Contains(userBucket, ip) {
				log(r, "info", 200, fmt.Sprintf("接口进入灰度,ip : %s", ip))
				return true, nil
			}
		}
	}

	return false, nil
}

type newServerFunc func(conf gateway.RouteMapping, r *http.Request) IUriProcessor

var newServerFuncMap = map[int8]newServerFunc{
	0: func(conf gateway.RouteMapping, r *http.Request) IUriProcessor {
		return NewDirectPhpSource(conf.UriDispatch.DirectHost, conf.UriDispatch.DirectPath)
	},
	1: func(conf gateway.RouteMapping, r *http.Request) IUriProcessor {
		return NewDirectGoServer()
	},
}

/**
直连go服务
*/
type DirectGoServer struct {
	*ServerResponseWriter
}

func NewDirectGoServer() *DirectGoServer {
	return &DirectGoServer{}
}

func (w *DirectGoServer) SetWriter(writer *ServerResponseWriter) {
	w.ServerResponseWriter = writer
}

func (s *DirectGoServer) Process(w http.ResponseWriter, r *http.Request, next http.Handler) {
	log(r, "info", 200, "发起go请求")
	next.ServeHTTP(w, r)
}

/**
直连php服务
*/
type DirectPhpServer struct {
	*ServerResponseWriter
	//directHost  php服务地址
	DirectHost string
	//directPath  php服务路径
	DirectPath string
}

func NewDirectPhpSource(host, path string) *DirectPhpServer {
	return &DirectPhpServer{
		DirectHost: host,
		DirectPath: path,
	}
}

func (w *DirectPhpServer) SetWriter(writer *ServerResponseWriter) {
	w.ServerResponseWriter = writer
}

var client *resty.Client
var once sync.Once

func NewHttpClient() *resty.Client {
	once.Do(func() {
		client = resty.New().SetTimeout(8 * time.Second)
	})

	return client
}

func (s *DirectPhpServer) Process(w http.ResponseWriter, r *http.Request, _ http.Handler) {
	log(r, "info", 200, "发起php请求"+s.DirectHost+"/"+s.DirectPath+"?"+r.URL.RawQuery)
	httpResult := restyclient.HttpResult{}
	// 发起请求
	header := r.Header
	header["Accept-Encoding"] = []string{"gzip"}
	//直连不转发
	header["Direct"] = []string{"1"}
	resp, err := NewHttpClient().
		R().
		SetContext(r.Context()).
		SetQueryString(r.URL.RawQuery).
		SetHeaderMultiValues(header).
		SetBody(r.Body).
		SetFormDataFromValues(r.PostForm).
		SetResult(&httpResult).
		Execute(strings.ToUpper(r.Method), s.DirectHost+"/"+s.DirectPath)
	if err != nil {
		log(r, "error", 500, fmt.Sprintf("php请求失败, %+v", err))
		httpx.WriteJson(w, resp.StatusCode(), &result.ResponseSuccessBean{Code: 0, Msg: err.Error(), Data: nil})
		return
	}

	httpx.WriteJson(w, resp.StatusCode(), &result.ResponseSuccessBean{Code: uint32(httpResult.Code), Msg: httpResult.Msg, Data: httpResult.Data})

	return
}

type ReserveServer struct {
	DirectGoServer  *DirectGoServer
	DirectPhpServer *DirectPhpServer
	Priority        int8
}

func NewReserveServer(goServer *DirectGoServer, phpServer *DirectPhpServer, priority int8) *ReserveServer {
	return &ReserveServer{
		DirectGoServer:  goServer,
		DirectPhpServer: phpServer,
		Priority:        priority,
	}
}

type ServerResponseWriter struct {
	ResponseBody []byte
	statusCode   int
	header       http.Header
}

type ApiResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

func (w *ServerResponseWriter) Header() http.Header {
	return w.header
}

func (w *ServerResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func NewServerResponseWriter() *ServerResponseWriter {
	return &ServerResponseWriter{
		header: make(http.Header),
	}
}

func (w *ServerResponseWriter) Write(bytes []byte) (int, error) {
	w.ResponseBody = append(w.ResponseBody, bytes...)

	return len(bytes), nil
}

func (s *ReserveServer) Process(w http.ResponseWriter, r *http.Request, next http.Handler) {
	s.DirectGoServer.SetWriter(NewServerResponseWriter())
	s.DirectPhpServer.SetWriter(NewServerResponseWriter())
	var phpErr, goErr error
	var goResponse, phpResponse ApiResponse
	var g threading.RoutineGroup
	g.RunSafe(func() {
		s.DirectGoServer.Process(s.DirectGoServer.ServerResponseWriter, r, next)
		goErr = json.Unmarshal(s.DirectGoServer.ResponseBody, &goResponse)
	})
	g.RunSafe(func() {
		s.DirectPhpServer.Process(s.DirectPhpServer.ServerResponseWriter, r, next)
		phpErr = json.Unmarshal(s.DirectPhpServer.ResponseBody, &phpResponse)
	})
	g.Wait()
	if s.Priority == 0 {
		//php 优先
		if phpErr != nil {
			log(r, "error", 500, fmt.Sprintf("php请求解析失败, %+v", phpErr))
			httpx.WriteJson(w, http.StatusOK, &result.ResponseSuccessBean{Code: xerr.SERVER_COMMON_ERROR, Msg: phpErr.Error(), Data: nil})

			return
		}
		isSame, res := CompareoRespnse(&goResponse, &phpResponse)
		if !isSame {
			log(r, "error", 500, fmt.Sprintf("新接口与旧接口不一致, %s", res))
			sendWxReport(fmt.Sprintf("%s/%s", strings.TrimRight(r.Host, "/"), strings.TrimLeft(r.URL.String(), "/")), res)
			//对比不一致，返回php服务内容
			writeOutput(w, s.DirectPhpServer.statusCode, s.DirectPhpServer.ResponseBody)

			return
		}
		logx.Info("返回go响应")
		if s.DirectGoServer.statusCode == 0 {
			s.DirectGoServer.statusCode = http.StatusOK
		}
		writeOutput(w, s.DirectGoServer.statusCode, s.DirectGoServer.ResponseBody)
	} else {
		//go 优先
		if s.DirectGoServer.statusCode == 0 {
			s.DirectGoServer.statusCode = http.StatusOK
		}
		if goErr != nil {
			log(r, "error", 500, fmt.Sprintf("go请求解析失败, %+v", goErr))
			//go服务错误，返回服务内容
			writeOutput(w, s.DirectGoServer.statusCode, s.DirectGoServer.ResponseBody)

			return
		}
		isSame, res := CompareoRespnse(&goResponse, &phpResponse)
		if !isSame {
			//对比不一致，返回go服务内容
			log(r, "error", 500, fmt.Sprintf("新接口与旧接口不一致, %s", res))
			sendWxReport(fmt.Sprintf("%s/%s", strings.TrimRight(r.Host, "/"), strings.TrimLeft(r.URL.String(), "/")), res)
			writeOutput(w, s.DirectGoServer.statusCode, s.DirectGoServer.ResponseBody)
			return
		}
		//否则返回php
		logx.Info("返回php响应")
		writeOutput(w, s.DirectPhpServer.statusCode, s.DirectPhpServer.ResponseBody)
	}

	return
}

//比较路由，忽略参数顺序
func compareUrl(a, b string) bool {
	if a == b {
		return true
	}
	aUrl, err := url.Parse(a)
	if err != nil {
		//非url
		return false
	}
	bUrl, err := url.Parse(b)
	if err != nil {
		//非url
		return false
	}
	if strings.TrimRight(aUrl.Host, "/") != strings.TrimRight(bUrl.Host, "/") {
		return false
	}
	if strings.Trim(aUrl.Path, "/") != strings.Trim(bUrl.Path, "/") {
		return false
	}

	query1 := aUrl.Query()
	query2 := bUrl.Query()
	return cmp.Equal(query1, query2, cmp.Comparer(compareUrl))
}

func CompareoRespnse(aResponse, bResponse *ApiResponse) (bool, string) {
	if aResponse == nil && bResponse != nil {
		return false, "php响应为空"
	}
	if aResponse != nil && bResponse == nil {
		return false, "go响应为空"
	}

	if aResponse.Code != bResponse.Code {

		return false, fmt.Sprintf("响应业务状态码不一致, php响应Code:%d, go响应Code:%d", bResponse.Code, aResponse.Code)
	}
	if aResponse.Code == 1000 {
		//1000的时候才校验data
		diff := cmp.Diff(aResponse.Data, bResponse.Data, cmp.Comparer(compareUrl))
		if diff != "" {
			logx.Errorf("新旧接口不一致, [%s]", diff)
			return false, diff
		}
	}
	return true, ""
}

func writeOutput(w http.ResponseWriter, code int, v []byte) {
	if err := doWriteJson(w, code, v); err != nil {
		logx.Error(err)
	}
}

func doWriteJson(w http.ResponseWriter, code int, bs []byte) error {
	w.Header().Set(httpx.ContentType, httpx.JsonContentType)
	w.WriteHeader(code)

	if n, err := w.Write(bs); err != nil {
		// http.ErrHandlerTimeout has been handled by http.TimeoutHandler,
		// so it's ignored here.
		if !errors.Is(err, http.ErrHandlerTimeout) {
			return fmt.Errorf("write response failed, error: %w", err)
		}
	} else if n < len(bs) {
		return fmt.Errorf("actual bytes: %d, written bytes: %d", len(bs), n)
	}

	return nil
}

func readFile(fileName string) (string, error) {
	_, err := os.Stat(fileName)
	if err != nil {
		return "", err
	}
	file, err := os.Open(fileName)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := ioutil.ReadAll(file)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func getValueByCtx(ctx context.Context, key string) string {
	uidData := ctx.Value("moreMd").([]string)
	for _, v := range uidData {
		sep := strings.Split(v, ":")
		if len(sep) != 2 {
			continue
		}
		if sep[0] == key {
			return sep[1]
		}
	}
	return ""
}

// ClientIP 尽最大努力实现获取客户端 IP 的算法。
// 解析 X-Real-IP 和 X-Forwarded-For 以便于反向代理（nginx 或 haproxy）可以正常工作。
func ClientIP(r *http.Request) string {
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	ip := strings.TrimSpace(strings.Split(xForwardedFor, ",")[0])
	if ip != "" && net.ParseIP(ip) != nil {
		return ip
	}

	ip = strings.TrimSpace(r.Header.Get("X-Real-Ip"))
	if ip != "" && net.ParseIP(ip) != nil {
		return ip
	}
	ip, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))

	if err == nil && net.ParseIP(ip) != nil {
		return ip
	}

	return ""
}

var metricServerReqCodeTotal = metric.NewCounterVec(&metric.CounterVecOpts{
	Namespace: "http_server",
	Subsystem: "requests",
	Name:      "gray_err_total",
	Help:      "http server gray requests error count.",
	Labels:    []string{"method", "path", "code"},
})

func log(r *http.Request, level string, code int64, msg string) {
	body, _ := ioutil.ReadAll(r.Body)
	path := strings.Replace(r.URL.Path, "//", "/", 1)
	l := logx.WithContext(r.Context()).WithFields(logx.LogField{
		Key:   "method",
		Value: r.URL.String(),
	}, logx.LogField{
		Key:   "host",
		Value: r.URL.Host,
	}, logx.LogField{
		Key:   "postForm",
		Value: r.PostForm,
	}, logx.LogField{
		Key:   "body",
		Value: string(body),
	})
	switch level {
	case "info":
		l.Infof(msg)
	case "error":
		l.Errorf(msg)
		metricServerReqCodeTotal.Inc(r.Method, path, strconv.FormatInt(code, 10))
	default:
		l.Info(msg)
	}
}

type WxMsg struct {
	MsgType  string `json:"msgtype"`
	Markdown struct {
		Content string `json:"content"`
	} `json:"markdown"`
}

func sendWxReport(url, str string) {
	if Mode == "pro" {
		return
	}
	threading.RunSafe(func() {
		path := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=124c77e3-6a01-460e-b733-da7ea6ec9b41"
		content := fmt.Sprintf("\n##### 灰度接口校验不一致\n> [%s](%s)\n>\n> 环境：%s \n>\n> 对比：`%s`\n", url, url, Mode, str)
		if len(content) > 2048 {
			rs := []rune(content)
			content = string(rs[:2048])
		}
		msg := WxMsg{
			MsgType: "markdown",
			Markdown: struct {
				Content string `json:"content"`
			}{Content: content},
		}
		resp, err := NewHttpClient().R().SetHeader("Content-Type", "application/json").SetBody(msg).
			Execute("POST", path)
		fmt.Println(resp)
		fmt.Println(err)
	})
}
