package server

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/isyscore/isc-gobase/server/rsp"

	"github.com/isyscore/isc-gobase/config"
	"github.com/isyscore/isc-gobase/isc"

	"github.com/isyscore/isc-gobase/logger"

	"github.com/gin-gonic/gin"
	"github.com/isyscore/isc-gobase/websocket"
)

type HttpMethod int

const (
	HmAll HttpMethod = iota
	HmGet
	HmPost
	HmPut
	HmDelete
	HmOptions
	HmHead
	HmGetPost
)

var ApiPrefix = "/api"

var engine *gin.Engine = nil

func init() {
	isc.PrintBanner()
	config.LoadConfig()

	if config.ExistConfigFile() && config.GetValueBoolDefault("base.server.enable", true) {
		InitServer()
	}
}

func InitServer() {
	if !config.ExistConfigFile() {
		logger.Error("没有找到任何配置文件，服务启动失败")
		return
	}
	mode := config.BaseCfg.Server.Gin.Mode
	if "debug" == mode {
		gin.SetMode(gin.DebugMode)
	} else if "test" == mode {
		gin.SetMode(gin.TestMode)
	} else if "release" == mode {
		gin.SetMode(gin.ReleaseMode)
		gin.DefaultWriter = ioutil.Discard
	}

	engine = gin.New()
	engine.Use(Cors(), gin.Recovery())

	// 注册 异常返回值打印
	if config.GetValueBoolDefault("base.server.exception.print.enable", true) {
		engine.Use(rsp.ResponseHandler(config.BaseCfg.Server.Exception.Print.Except...))
	}

	ap := config.GetValueStringDefault("base.api.prefix", "")
	if ap != "" {
		ApiPrefix = ap
	}

	// 注册 健康检查endpoint
	if config.GetValueBoolDefault("base.endpoint.health.enable", false) {
		RegisterHealthCheckEndpoint(ApiPrefix + "/" + config.ApiModule)
	}

	// 注册 配置检测endpoint
	if config.GetValueBoolDefault("base.endpoint.config.enable", false) {
		RegisterConfigWatchEndpoint(ApiPrefix + "/" + config.ApiModule)
	}
	level := config.GetValueStringDefault("base.logger.level", "info")
	timeFieldFormat := config.GetValueStringDefault("base.logger.time.format", time.RFC3339)
	colored := config.GetValueBoolDefault("base.logger.color.enable", false)
	appName := config.GetValueStringDefault("base.application.name", "isc-gobase")
	splitEnable := config.GetValueBoolDefault("base.logger.split.enable", false)
	splitSize := config.GetValueInt64Default("base.logger.split.size", 300)
	logger.InitLog(level, timeFieldFormat, colored, appName, splitEnable, splitSize)
}

func Run() {
	StartServer()
}

func StartServer() {
	if !checkEngine() {
		return
	}

	if !config.BaseCfg.Server.Enable {
		return
	}

	logger.Info("开始启动服务")
	port := config.GetValueIntDefault("base.server.port", 8080)
	logger.Info("服务端口号: %d", port)
	err := engine.Run(fmt.Sprintf(":%d", port))
	if err != nil {
		logger.Error("启动服务异常 (%v)", err)
	}
}

func RegisterStatic(relativePath string, rootPath string) gin.IRoutes {
	if !checkEngine() {
		return nil
	}
	engine.Static(relativePath, rootPath)
	return engine
}

func RegisterStaticFile(relativePath string, filePath string) gin.IRoutes {
	if !checkEngine() {
		return nil
	}
	engine.StaticFile(relativePath, filePath)
	return engine
}

func RegisterPlugin(plugin gin.HandlerFunc) gin.IRoutes {
	if !checkEngine() {
		return nil
	}
	engine.Use(plugin)
	return engine
}

func Engine() gin.IRoutes {
	return engine
}

func RegisterHealthCheckEndpoint(apiBase string) gin.IRoutes {
	if "" == apiBase {
		return nil
	}
	RegisterRoute(apiBase+"/system/status", HmAll, healthSystemStatus)
	RegisterRoute(apiBase+"/system/init", HmAll, healthSystemInit)
	RegisterRoute(apiBase+"/system/destroy", HmAll, healthSystemDestroy)
	return engine
}

func RegisterConfigWatchEndpoint(apiBase string) gin.IRoutes {
	if "" == apiBase {
		return nil
	}
	RegisterRoute(apiBase+"/config/values", HmGet, config.GetConfigValues)
	RegisterRoute(apiBase+"/config/value/:key", HmGet, config.GetConfigValue)
	RegisterRoute(apiBase+"/config/update", HmPut, config.UpdateConfig)
	return engine
}

func RegisterCustomHealthCheck(apiBase string, status func() string, init func() string, destroy func() string) gin.IRoutes {
	if !checkEngine() {
		return nil
	}
	RegisterRoute(apiBase+"/system/status", HmAll, func(c *gin.Context) {
		c.Data(200, "application/json; charset=utf-8", []byte(status()))
	})
	RegisterRoute(apiBase+"/system/init", HmAll, func(c *gin.Context) {
		c.Data(200, "application/json; charset=utf-8", []byte(init()))
	})
	RegisterRoute(apiBase+"/system/destroy", HmAll, func(c *gin.Context) {
		c.Data(200, "application/json; charset=utf-8", []byte(destroy()))
	})
	return engine
}

func checkEngine() bool {
	if engine == nil {
		logger.Error("服务没有初始化，请先调用 InitServer")
		return false
	}
	return true
}

func RegisterRoute(path string, method HttpMethod, handler gin.HandlerFunc) gin.IRoutes {
	if !checkEngine() {
		return nil
	}
	switch method {
	case HmAll:
		engine.GET(path, handler)
		engine.POST(path, handler)
		engine.PUT(path, handler)
		engine.DELETE(path, handler)
		engine.OPTIONS(path, handler)
		engine.HEAD(path, handler)
	case HmGet:
		engine.GET(path, handler)
	case HmPost:
		engine.POST(path, handler)
	case HmPut:
		engine.PUT(path, handler)
	case HmDelete:
		engine.DELETE(path, handler)
	case HmOptions:
		engine.OPTIONS(path, handler)
	case HmHead:
		engine.HEAD(path, handler)
	case HmGetPost:
		engine.GET(path, handler)
		engine.POST(path, handler)
	}
	return engine
}

func RegisterRouteWithHeaders(path string, method HttpMethod, header []string, versionName []string, handler gin.HandlerFunc) gin.IRoutes {
	if !checkEngine() {
		return nil
	}
	p := GetApiPath(path, method)
	if p == nil {
		p = NewApiPath(path, method)
		switch method {
		case HmAll:
			engine.GET(path, p.Handler)
			engine.POST(path, p.Handler)
			engine.PUT(path, p.Handler)
			engine.DELETE(path, p.Handler)
			engine.OPTIONS(path, p.Handler)
			engine.HEAD(path, p.Handler)
		case HmGet:
			engine.GET(path, p.Handler)
		case HmPost:
			engine.POST(path, p.Handler)
		case HmPut:
			engine.PUT(path, p.Handler)
		case HmDelete:
			engine.DELETE(path, p.Handler)
		case HmOptions:
			engine.OPTIONS(path, p.Handler)
		case HmHead:
			engine.HEAD(path, p.Handler)
		case HmGetPost:
			engine.GET(path, p.Handler)
			engine.POST(path, p.Handler)
		}
	}
	p.AddVersion(header, versionName, handler)
	return engine
}

func RegisterWebSocketRoute(path string, svr *websocket.Server) gin.IRoutes {
	if !checkEngine() {
		return nil
	}
	engine.GET(path, svr.Handler())
	return engine
}

func Post(path string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRoute(getPathAppendApiModel(path), HmPost, handler)
}

func Delete(path string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRoute(getPathAppendApiModel(path), HmDelete, handler)
}

func Put(path string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRoute(getPathAppendApiModel(path), HmPut, handler)
}

func Head(path string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRoute(getPathAppendApiModel(path), HmHead, handler)
}

func Get(path string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRoute(getPathAppendApiModel(path), HmGet, handler)
}

func Options(path string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRoute(getPathAppendApiModel(path), HmOptions, handler)
}

func GetPost(path string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRoute(getPathAppendApiModel(path), HmGetPost, handler)
}

func All(path string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRoute(getPathAppendApiModel(path), HmAll, handler)
}

func PostWith(path string, header []string, versionName []string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRouteWithHeaders(getPathAppendApiModel(path), HmPost, header, versionName, handler)
}

func DeleteWith(path string, header []string, versionName []string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRouteWithHeaders(getPathAppendApiModel(path), HmDelete, header, versionName, handler)
}

func PutWith(path string, header []string, versionName []string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRouteWithHeaders(getPathAppendApiModel(path), HmPut, header, versionName, handler)
}

func HeadWith(path string, header []string, versionName []string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRouteWithHeaders(getPathAppendApiModel(path), HmHead, header, versionName, handler)
}

func GetWith(path string, header []string, versionName []string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRouteWithHeaders(getPathAppendApiModel(path), HmGet, header, versionName, handler)
}

func OptionsWith(path string, header []string, versionName []string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRouteWithHeaders(getPathAppendApiModel(path), HmOptions, header, versionName, handler)
}

func GetPostWith(path string, header []string, versionName []string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRouteWithHeaders(getPathAppendApiModel(path), HmGetPost, header, versionName, handler)
}

func AllWith(path string, header []string, versionName []string, handler gin.HandlerFunc) gin.IRoutes {
	return RegisterRouteWithHeaders(getPathAppendApiModel(path), HmAll, header, versionName, handler)
}

func getPathAppendApiModel(path string) string {
	// 获取 api-module
	apiModel := isc.ISCString(config.GetValueString("api-module")).Trim("/")
	// 获取api前缀
	ap := isc.ISCString(config.GetValueStringDefault("base.api.prefix", "")).Trim("/")
	if ap != "" {
		ApiPrefix = "/" + string(ap)
	}
	p2 := isc.ISCString(path).Trim("/")
	return fmt.Sprintf("/%s/%s/%s", ap, apiModel, p2)
}
