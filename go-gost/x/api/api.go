package api

import (
	"embed"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/go-gost/core/auth"
)

var (
	//go:embed swagger.yaml
	swaggerDoc embed.FS
)

type Response struct {
	Code int    `json:"code,omitempty"`
	Msg  string `json:"msg,omitempty"`
}

type Options struct {
	AccessLog  bool
	PathPrefix string
	Auther     auth.Authenticator
}

func Register(r *gin.Engine, opts *Options) {
	if opts == nil {
		opts = &Options{}
	}

	r.Use(
		cors.New((cors.Config{
			AllowAllOrigins:     true,
			AllowMethods:        []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowHeaders:        []string{"*"},
			AllowPrivateNetwork: true,
		})),
		gin.Recovery(),
		GlobalInterceptor(),
	)
	if opts.AccessLog {
		r.Use(mwLogger())
	}

	router := r.Group("")
	if opts.PathPrefix != "" {
		router = router.Group(opts.PathPrefix)
	}

	router.StaticFS("/docs", http.FS(swaggerDoc))

	config := router.Group("/config")
	config.Use(mwBasicAuth(opts.Auther), configTransaction())

	config.GET("", getConfig)
	config.POST("", saveConfig)

	config.POST("/reload", reloadConfig)

	config.POST("/services", createService)
	config.PUT("/services/:service", updateService)
	config.DELETE("/services/:service", deleteService)
	config.POST("/services/:service/pause", pauseService)
	config.POST("/services/:service/resume", resumeService)
	config.GET("/services/:service/status", getServiceStatus)
	config.POST("/services/batch", createServices)
	config.PUT("/services/batch", updateServices)
	config.DELETE("/services/batch", deleteServices)
	config.POST("/services/batch/pause", pauseServices)
	config.POST("/services/batch/resume", resumeServices)

	config.POST("/chains", createChain)
	config.PUT("/chains/:chain", updateChain)
	config.DELETE("/chains/:chain", deleteChain)

	config.POST("/hops", createHop)
	config.PUT("/hops/:hop", updateHop)
	config.DELETE("/hops/:hop", deleteHop)

	config.POST("/authers", createAuther)
	config.PUT("/authers/:auther", updateAuther)
	config.DELETE("/authers/:auther", deleteAuther)

	config.POST("/admissions", createAdmission)
	config.PUT("/admissions/:admission", updateAdmission)
	config.DELETE("/admissions/:admission", deleteAdmission)

	config.POST("/bypasses", createBypass)
	config.PUT("/bypasses/:bypass", updateBypass)
	config.DELETE("/bypasses/:bypass", deleteBypass)

	config.POST("/resolvers", createResolver)
	config.PUT("/resolvers/:resolver", updateResolver)
	config.DELETE("/resolvers/:resolver", deleteResolver)

	config.POST("/hosts", createHosts)
	config.PUT("/hosts/:hosts", updateHosts)
	config.DELETE("/hosts/:hosts", deleteHosts)

	config.POST("/ingresses", createIngress)
	config.PUT("/ingresses/:ingress", updateIngress)
	config.DELETE("/ingresses/:ingress", deleteIngress)

	config.POST("/routers", createRouter)
	config.PUT("/routers/:router", updateRouter)
	config.DELETE("/routers/:router", deleteRouter)

	config.POST("/observers", createObserver)
	config.PUT("/observers/:observer", updateObserver)
	config.DELETE("/observers/:observer", deleteObserver)

	config.POST("/recorders", createRecorder)
	config.PUT("/recorders/:recorder", updateRecorder)
	config.DELETE("/recorders/:recorder", deleteRecorder)

	config.POST("/sds", createSD)
	config.PUT("/sds/:sd", updateSD)
	config.DELETE("/sds/:sd", deleteSD)

	config.POST("/limiters", createLimiter)
	config.PUT("/limiters/:limiter", updateLimiter)
	config.DELETE("/limiters/:limiter", deleteLimiter)

	config.POST("/climiters", createConnLimiter)
	config.PUT("/climiters/:limiter", updateConnLimiter)
	config.DELETE("/climiters/:limiter", deleteConnLimiter)

	config.POST("/rlimiters", createRateLimiter)
	config.PUT("/rlimiters/:limiter", updateRateLimiter)
	config.DELETE("/rlimiters/:limiter", deleteRateLimiter)
}
