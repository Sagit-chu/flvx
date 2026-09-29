package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-gost/x/config"
	"github.com/go-gost/x/config/loader"
	"github.com/go-gost/x/config/parsing/parser"
	"github.com/go-gost/x/registry"
	xservice "github.com/go-gost/x/service"
)

// swagger:parameters reloadConfigRequest
type reloadConfigRequest struct{}

// successful operation.
// swagger:response reloadConfigResponse
type reloadConfigResponse struct {
	Data Response
}

func reloadConfig(ctx *gin.Context) {
	// swagger:route POST /config/reload Reload reloadConfigRequest
	//
	// Hot reload config.
	//
	//     Security:
	//       basicAuth: []
	//
	//     Responses:
	//       200: reloadConfigResponse

	cfg, err := parser.Parse()
	if err != nil {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeInvalid, err.Error()))
		return
	}

	if err := loader.Reload(cfg, func(*config.Config) error {
		for _, svc := range registry.ServiceRegistry().GetAll() {
			go svc.Serve()
		}
		return nil
	}); err != nil {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeInvalid, err.Error()))
		return
	}
	activeServices := make(map[string]struct{}, len(cfg.Services))
	for _, svc := range cfg.Services {
		if svc != nil {
			activeServices[svc.Name] = struct{}{}
		}
	}
	xservice.GetGlobalTrafficManager().RetainServices(activeServices)

	ctx.JSON(http.StatusOK, Response{
		Msg: "OK",
	})
}
