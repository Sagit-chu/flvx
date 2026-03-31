package api

import (
	"fmt"

	"net/http"

	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-gost/core/service"
	"github.com/go-gost/x/config"
	parser "github.com/go-gost/x/config/parsing/service"
	kill "github.com/go-gost/x/internal/util/port"
	"github.com/go-gost/x/registry"
)

// swagger:parameters createServiceRequest
type createServiceRequest struct {
	// in: body
	Data config.ServiceConfig `json:"data"`
}

// successful operation.
// swagger:response createServiceResponse
type createServiceResponse struct {
	Data Response
}

// swagger:parameters createServicesRequest
type createServicesRequest struct {
	// in: body
	Data []config.ServiceConfig `json:"data"`
}

// successful operation.
// swagger:response createServicesResponse
type createServicesResponse struct {
	Data Response
}

func createService(ctx *gin.Context) {
	// swagger:route POST /config/services Service createServiceRequest
	//
	// Create a new service, the name of the service must be unique in service list.
	//
	//     Security:
	//       basicAuth: []
	//
	//     Responses:
	//       200: createServiceResponse

	var req createServiceRequest
	ctx.ShouldBindJSON(&req.Data)

	name := strings.TrimSpace(req.Data.Name)
	if name == "" {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeInvalid, "service name is required"))
		return
	}
	req.Data.Name = name

	if registry.ServiceRegistry().IsRegistered(name) {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeDup, fmt.Sprintf("service %s already exists", name)))
		return
	}

	svc, err := parser.ParseService(&req.Data)
	if err != nil {
		writeError(ctx, NewError(http.StatusInternalServerError, ErrCodeFailed, fmt.Sprintf("create service %s failed: %s", name, err.Error())))
		return
	}

	if err := registry.ServiceRegistry().Register(name, svc); err != nil {
		svc.Close()
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeDup, fmt.Sprintf("service %s already exists", name)))
		return
	}

	go svc.Serve()

	config.OnUpdate(func(c *config.Config) error {
		c.Services = append(c.Services, &req.Data)
		return nil
	})

	ctx.JSON(http.StatusOK, Response{
		Msg: "OK",
	})
}

func createServices(ctx *gin.Context) {
	// swagger:route POST /config/services/batch Service createServicesRequest
	//
	// Create multiple services at once (transactional: all succeed or all fail).
	//
	//     Security:
	//       basicAuth: []
	//
	//     Responses:
	//       200: createServicesResponse

	var req createServicesRequest
	ctx.ShouldBindJSON(&req.Data)

	if len(req.Data) == 0 {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeInvalid, "services list cannot be empty"))
		return
	}

	// 第一阶段：验证所有服务配置
	var parsedServices []struct {
		config  config.ServiceConfig
		service service.Service
	}

	for _, serviceConfig := range req.Data {
		name := strings.TrimSpace(serviceConfig.Name)
		if name == "" {
			writeError(ctx, NewError(http.StatusBadRequest, ErrCodeInvalid, "service name is required"))
			return
		}
		serviceConfig.Name = name

		if registry.ServiceRegistry().IsRegistered(name) {
			writeError(ctx, NewError(http.StatusBadRequest, ErrCodeDup, fmt.Sprintf("service %s already exists", name)))
			return
		}

		svc, err := parser.ParseService(&serviceConfig)
		if err != nil {
			writeError(ctx, NewError(http.StatusInternalServerError, ErrCodeFailed, fmt.Sprintf("create service %s failed: %s", name, err.Error())))
			return
		}

		parsedServices = append(parsedServices, struct {
			config  config.ServiceConfig
			service service.Service
		}{serviceConfig, svc})
	}

	// 第二阶段：注册所有服务
	var registeredServices []string
	for _, ps := range parsedServices {
		if err := registry.ServiceRegistry().Register(ps.config.Name, ps.service); err != nil {
			// 如果注册失败，回滚已注册的服务
			for _, regName := range registeredServices {
				if svc := registry.ServiceRegistry().Get(regName); svc != nil {
					registry.ServiceRegistry().Unregister(regName)
					svc.Close()
				}
			}
			writeError(ctx, NewError(http.StatusBadRequest, ErrCodeDup, fmt.Sprintf("service %s already exists", ps.config.Name)))
			return
		}
		registeredServices = append(registeredServices, ps.config.Name)
	}

	// 第三阶段：启动所有服务
	for _, ps := range parsedServices {
		if svc := registry.ServiceRegistry().Get(ps.config.Name); svc != nil {
			go svc.Serve()
		}
	}

	// 第四阶段：更新配置
	config.OnUpdate(func(c *config.Config) error {
		for _, ps := range parsedServices {
			c.Services = append(c.Services, &ps.config)
		}
		return nil
	})

	ctx.JSON(http.StatusOK, Response{
		Msg: "OK",
	})
}

// swagger:parameters updateServiceRequest
type updateServiceRequest struct {
	// in: path
	// required: true
	Service string `uri:"service" json:"service"`
	// in: body
	Data config.ServiceConfig `json:"data"`
}

// successful operation.
// swagger:response updateServiceResponse
type updateServiceResponse struct {
	Data Response
}

// swagger:parameters updateServicesRequest
type updateServicesRequest struct {
	// in: body
	Data []config.ServiceConfig `json:"data"`
}

// successful operation.
// swagger:response updateServicesResponse
type updateServicesResponse struct {
	Data Response
}

func updateService(ctx *gin.Context) {
	// swagger:route PUT /config/services/{service} Service updateServiceRequest
	//
	// Update service by name, the service must already exist.
	//
	//     Security:
	//       basicAuth: []
	//
	//     Responses:
	//       200: updateServiceResponse

	var req updateServiceRequest
	ctx.ShouldBindUri(&req)
	ctx.ShouldBindJSON(&req.Data)

	name := strings.TrimSpace(req.Service)

	old := registry.ServiceRegistry().Get(name)
	if old == nil {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeNotFound, fmt.Sprintf("service %s not found", name)))
		return
	}
	old.Close()
	registry.ServiceRegistry().Unregister(name)

	// 等待端口释放
	time.Sleep(500 * time.Millisecond)

	req.Data.Name = name

	svc, err := parser.ParseService(&req.Data)
	if err != nil {
		writeError(ctx, NewError(http.StatusInternalServerError, ErrCodeFailed, fmt.Sprintf("create service %s failed: %s", name, err.Error())))
		return
	}

	if err := registry.ServiceRegistry().Register(name, svc); err != nil {
		svc.Close()
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeDup, fmt.Sprintf("service %s already exists", name)))
		return
	}

	go svc.Serve()

	config.OnUpdate(func(c *config.Config) error {
		for i := range c.Services {
			if c.Services[i].Name == name {
				c.Services[i] = &req.Data
				break
			}
		}
		return nil
	})

	ctx.JSON(http.StatusOK, Response{
		Msg: "OK",
	})
}

func updateServices(ctx *gin.Context) {
	// swagger:route PUT /config/services/batch Service updateServicesRequest
	//
	// Update multiple services at once (transactional: all succeed or all fail).
	//
	//     Security:
	//       basicAuth: []
	//
	//     Responses:
	//       200: updateServicesResponse

	var req updateServicesRequest
	ctx.ShouldBindJSON(&req.Data)

	if len(req.Data) == 0 {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeInvalid, "services list cannot be empty"))
		return
	}

	// 第一阶段：验证所有服务存在
	for _, serviceConfig := range req.Data {
		name := strings.TrimSpace(serviceConfig.Name)
		if name == "" {
			writeError(ctx, NewError(http.StatusBadRequest, ErrCodeInvalid, "service name is required"))
			return
		}
		serviceConfig.Name = name

		old := registry.ServiceRegistry().Get(name)
		if old == nil {
			writeError(ctx, NewError(http.StatusBadRequest, ErrCodeNotFound, fmt.Sprintf("service %s not found", name)))
			return
		}
	}

	// 第二阶段：按照原来的updateService逻辑，逐个更新服务
	for _, serviceConfig := range req.Data {
		name := strings.TrimSpace(serviceConfig.Name)
		serviceConfig.Name = name

		// 1. 获取旧服务
		old := registry.ServiceRegistry().Get(name)

		// 2. 关闭旧服务
		old.Close()

		// 3. 从注册表移除旧服务
		registry.ServiceRegistry().Unregister(name)

		// 4. 解析新服务配置
		svc, err := parser.ParseService(&serviceConfig)
		if err != nil {
			writeError(ctx, NewError(http.StatusInternalServerError, ErrCodeFailed, fmt.Sprintf("create service %s failed: %s", name, err.Error())))
			return
		}

		// 5. 注册新服务
		if err := registry.ServiceRegistry().Register(name, svc); err != nil {
			svc.Close()
			writeError(ctx, NewError(http.StatusBadRequest, ErrCodeDup, fmt.Sprintf("service %s already exists", name)))
			return
		}

		// 6. 启动新服务
		go svc.Serve()
	}

	// 第三阶段：更新配置
	config.OnUpdate(func(c *config.Config) error {
		for _, serviceConfig := range req.Data {
			for i := range c.Services {
				if c.Services[i].Name == serviceConfig.Name {
					c.Services[i] = &serviceConfig
					break
				}
			}
		}
		return nil
	})

	ctx.JSON(http.StatusOK, Response{
		Msg: "OK",
	})
}

// swagger:parameters deleteServiceRequest
type deleteServiceRequest struct {
	// in: path
	// required: true
	Service string `uri:"service" json:"service"`
}

// successful operation.
// swagger:response deleteServiceResponse
type deleteServiceResponse struct {
	Data Response
}

// swagger:parameters deleteServicesRequest
type deleteServicesRequest struct {
	// in: body
	Services []string `json:"services"`
}

// successful operation.
// swagger:response deleteServicesResponse
type deleteServicesResponse struct {
	Data Response
}

func deleteService(ctx *gin.Context) {
	// swagger:route DELETE /config/services/{service} Service deleteServiceRequest
	//
	// Delete service by name.
	//
	//     Security:
	//       basicAuth: []
	//
	//     Responses:
	//       200: deleteServiceResponse

	var req deleteServiceRequest
	ctx.ShouldBindUri(&req)

	name := strings.TrimSpace(req.Service)

	svc := registry.ServiceRegistry().Get(name)
	if svc == nil {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeNotFound, fmt.Sprintf("service %s not found", name)))
		return
	}

	registry.ServiceRegistry().Unregister(name)
	svc.Close()

	config.OnUpdate(func(c *config.Config) error {
		services := c.Services
		c.Services = nil
		for _, s := range services {
			if s.Name == name {
				continue
			}
			c.Services = append(c.Services, s)
		}
		return nil
	})

	ctx.JSON(http.StatusOK, Response{
		Msg: "OK",
	})
}

func deleteServices(ctx *gin.Context) {
	// swagger:route DELETE /config/services/batch Service deleteServicesRequest
	//
	// Delete multiple services at once (transactional: all succeed or all fail).
	//
	//     Security:
	//       basicAuth: []
	//
	//     Responses:
	//       200: deleteServicesResponse

	var req deleteServicesRequest
	ctx.ShouldBindJSON(&req)

	if len(req.Services) == 0 {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeInvalid, "services list cannot be empty"))
		return
	}

	// 第一阶段：验证所有服务是否存在
	var servicesToDelete []struct {
		name    string
		service service.Service
	}

	for _, serviceName := range req.Services {
		name := strings.TrimSpace(serviceName)
		if name == "" {
			writeError(ctx, NewError(http.StatusBadRequest, ErrCodeInvalid, "service name is required"))
			return
		}

		svc := registry.ServiceRegistry().Get(name)
		if svc == nil {
			writeError(ctx, NewError(http.StatusBadRequest, ErrCodeNotFound, fmt.Sprintf("service %s not found", name)))
			return
		}

		servicesToDelete = append(servicesToDelete, struct {
			name    string
			service service.Service
		}{name, svc})
	}

	// 第二阶段：删除所有服务
	for _, std := range servicesToDelete {
		registry.ServiceRegistry().Unregister(std.name)
		std.service.Close()
	}

	// 第三阶段：更新配置
	config.OnUpdate(func(c *config.Config) error {
		services := c.Services
		c.Services = nil
		for _, s := range services {
			shouldDelete := false
			for _, std := range servicesToDelete {
				if s.Name == std.name {
					shouldDelete = true
					break
				}
			}
			if !shouldDelete {
				c.Services = append(c.Services, s)
			}
		}
		return nil
	})

	ctx.JSON(http.StatusOK, Response{
		Msg: "OK",
	})
}

// swagger:parameters pauseServiceRequest
type pauseServiceRequest struct {
	// in: path
	// required: true
	Service string `uri:"service" json:"service"`
}

// successful operation.
// swagger:response pauseServiceResponse
type pauseServiceResponse struct {
	Data Response
}

// swagger:parameters resumeServiceRequest
type resumeServiceRequest struct {
	// in: path
	// required: true
	Service string `uri:"service" json:"service"`
}

// successful operation.
// swagger:response resumeServiceResponse
type resumeServiceResponse struct {
	Data Response
}

func pauseService(ctx *gin.Context) {
	// swagger:route POST /config/services/{service}/pause Service pauseServiceRequest
	//
	// Pause service by name, the service will be stopped but configuration remains.
	//
	//     Security:
	//       basicAuth: []
	//
	//     Responses:
	//       200: pauseServiceResponse

	var req pauseServiceRequest
	ctx.ShouldBindUri(&req)

	name := strings.TrimSpace(req.Service)

	svc := registry.ServiceRegistry().Get(name)
	if svc == nil {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeNotFound, fmt.Sprintf("service %s not found", name)))
		return
	}

	// 获取服务地址用于强制断开连接
	var serviceAddr string
	cfg := config.Global()
	for _, s := range cfg.Services {
		if s.Name == name {
			serviceAddr = s.Addr
			break
		}
	}

	// 强制断开端口的所有连接
	if serviceAddr != "" {
		_ = kill.ForceClosePortConnections(serviceAddr)
	}

	// 更新配置中的暂停状态
	config.OnUpdate(func(c *config.Config) error {
		for i := range c.Services {
			if c.Services[i].Name == name {
				if c.Services[i].Metadata == nil {
					c.Services[i].Metadata = make(map[string]any)
				}
				c.Services[i].Metadata["paused"] = true
				break
			}
		}
		return nil
	})

	ctx.JSON(http.StatusOK, Response{
		Msg: "OK",
	})
}

func resumeService(ctx *gin.Context) {
	// swagger:route POST /config/services/{service}/resume Service resumeServiceRequest
	//
	// Resume paused service by name.
	//
	//     Security:
	//       basicAuth: []
	//
	//     Responses:
	//       200: resumeServiceResponse

	var req resumeServiceRequest
	ctx.ShouldBindUri(&req)

	name := strings.TrimSpace(req.Service)

	// 检查服务是否存在于注册表中
	existingSvc := registry.ServiceRegistry().Get(name)
	if existingSvc == nil {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeNotFound, fmt.Sprintf("service %s not found", name)))
		return
	}

	// 查找配置中的服务
	var serviceConfig *config.ServiceConfig
	cfg := config.Global()
	for _, s := range cfg.Services {
		if s.Name == name {
			serviceConfig = s
			break
		}
	}

	if serviceConfig == nil {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeNotFound, fmt.Sprintf("service %s configuration not found", name)))
		return
	}

	// 检查是否处于暂停状态
	if serviceConfig.Metadata == nil {
		serviceConfig.Metadata = make(map[string]any)
	}

	paused, exists := serviceConfig.Metadata["paused"]
	if !exists || paused != true {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeInvalid, fmt.Sprintf("service %s is not paused", name)))
		return
	}

	// 先关闭现有服务
	existingSvc.Close()
	registry.ServiceRegistry().Unregister(name)

	// 强制断开端口的所有连接
	if serviceConfig.Addr != "" {
		_ = kill.ForceClosePortConnections(serviceConfig.Addr)
	}

	// 等待端口释放
	time.Sleep(500 * time.Millisecond)

	// 重新解析并启动服务
	svc, err := parser.ParseService(serviceConfig)
	if err != nil {
		writeError(ctx, NewError(http.StatusInternalServerError, ErrCodeFailed, fmt.Sprintf("resume service %s failed: %s", name, err.Error())))
		return
	}

	if err := registry.ServiceRegistry().Register(name, svc); err != nil {
		svc.Close()
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeDup, fmt.Sprintf("service %s already exists", name)))
		return
	}

	go svc.Serve()

	// 更新配置，移除暂停状态
	config.OnUpdate(func(c *config.Config) error {
		for i := range c.Services {
			if c.Services[i].Name == name {
				if c.Services[i].Metadata != nil {
					delete(c.Services[i].Metadata, "paused")
					// 如果 metadata 为空，设置为 nil
					if len(c.Services[i].Metadata) == 0 {
						c.Services[i].Metadata = nil
					}
				}
				break
			}
		}
		return nil
	})

	ctx.JSON(http.StatusOK, Response{
		Msg: "OK",
	})
}

// swagger:parameters getServiceStatusRequest
type getServiceStatusRequest struct {
	// in: path
	// required: true
	Service string `uri:"service" json:"service"`
}

// successful operation.
// swagger:response getServiceStatusResponse
type getServiceStatusResponse struct {
	Data ServiceStatusInfo
}

type ServiceStatusInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"` // running, paused, stopped
	Paused bool   `json:"paused"`
}

func getServiceStatus(ctx *gin.Context) {
	// swagger:route GET /config/services/{service}/status Service getServiceStatusRequest
	//
	// Get service status by name.
	//
	//     Security:
	//       basicAuth: []
	//
	//     Responses:
	//       200: getServiceStatusResponse

	var req getServiceStatusRequest
	ctx.ShouldBindUri(&req)

	name := strings.TrimSpace(req.Service)

	// 检查服务是否在注册表中
	svc := registry.ServiceRegistry().Get(name)
	if svc == nil {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeNotFound, fmt.Sprintf("service %s not found", name)))
		return
	}

	// 检查配置中的暂停状态
	var paused bool
	cfg := config.Global()
	for _, s := range cfg.Services {
		if s.Name == name && s.Metadata != nil {
			if pausedVal, exists := s.Metadata["paused"]; exists && pausedVal == true {
				paused = true
			}
			break
		}
	}

	status := "running"
	if paused {
		status = "paused"
	}

	ctx.JSON(http.StatusOK, getServiceStatusResponse{
		Data: ServiceStatusInfo{
			Name:   name,
			Status: status,
			Paused: paused,
		},
	})
}

// 回滚已暂停的服务 - 重新启动它们
func rollbackPausedServices(pausedServices []struct {
	name          string
	service       service.Service
	serviceConfig *config.ServiceConfig
}) {
	for _, pss := range pausedServices {
		// 重新解析并启动服务
		svc, err := parser.ParseService(pss.serviceConfig)
		if err != nil {
			continue // 回滚失败，记录日志但继续处理其他服务
		}

		if err := registry.ServiceRegistry().Register(pss.name, svc); err != nil {
			svc.Close()
			continue // 回滚失败，记录日志但继续处理其他服务
		}

		go svc.Serve()

		// 移除暂停状态标记
		config.OnUpdate(func(c *config.Config) error {
			for i := range c.Services {
				if c.Services[i].Name == pss.name {
					if c.Services[i].Metadata != nil {
						delete(c.Services[i].Metadata, "paused")
						if len(c.Services[i].Metadata) == 0 {
							c.Services[i].Metadata = nil
						}
					}
					break
				}
			}
			return nil
		})
	}
}

// 回滚已恢复的服务 - 将它们重新暂停
func rollbackResumedServices(resumedServices []struct {
	name          string
	service       service.Service
	serviceConfig *config.ServiceConfig
}) {
	for _, rss := range resumedServices {
		// 关闭已恢复的服务
		if svc := registry.ServiceRegistry().Get(rss.name); svc != nil {
			svc.Close()
		}

		// 重新标记为暂停状态
		config.OnUpdate(func(c *config.Config) error {
			for i := range c.Services {
				if c.Services[i].Name == rss.name {
					if c.Services[i].Metadata == nil {
						c.Services[i].Metadata = make(map[string]any)
					}
					c.Services[i].Metadata["paused"] = true
					break
				}
			}
			return nil
		})
	}
}

// swagger:parameters pauseServicesRequest
type pauseServicesRequest struct {
	// in: body
	Services []string `json:"services"`
}

// successful operation.
// swagger:response pauseServicesResponse
type pauseServicesResponse struct {
	Data Response
}

// swagger:parameters resumeServicesRequest
type resumeServicesRequest struct {
	// in: body
	Services []string `json:"services"`
}

// successful operation.
// swagger:response resumeServicesResponse
type resumeServicesResponse struct {
	Data Response
}

func pauseServices(ctx *gin.Context) {
	// swagger:route POST /config/services/batch/pause Service pauseServicesRequest
	//
	// Pause multiple services at once (transactional: all succeed or all fail).
	//
	//     Security:
	//       basicAuth: []
	//
	//     Responses:
	//       200: pauseServicesResponse

	var req pauseServicesRequest
	ctx.ShouldBindJSON(&req)

	if len(req.Services) == 0 {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeInvalid, "services list cannot be empty"))
		return
	}

	// 第一阶段：验证所有服务是否存在，并筛选需要暂停的服务
	var servicesToPause []struct {
		name    string
		service service.Service
	}
	var skippedServices []string

	cfg := config.Global()
	for _, serviceName := range req.Services {
		name := strings.TrimSpace(serviceName)
		if name == "" {
			writeError(ctx, NewError(http.StatusBadRequest, ErrCodeInvalid, "service name is required"))
			return
		}

		svc := registry.ServiceRegistry().Get(name)
		if svc == nil {
			writeError(ctx, NewError(http.StatusBadRequest, ErrCodeNotFound, fmt.Sprintf("service %s not found", name)))
			return
		}

		// 检查服务是否已经暂停
		var serviceConfig *config.ServiceConfig
		for _, s := range cfg.Services {
			if s.Name == name {
				serviceConfig = s
				break
			}
		}

		// 如果服务已经暂停，跳过
		if serviceConfig != nil && serviceConfig.Metadata != nil {
			if pausedVal, exists := serviceConfig.Metadata["paused"]; exists && pausedVal == true {
				skippedServices = append(skippedServices, name)
				continue
			}
		}

		servicesToPause = append(servicesToPause, struct {
			name    string
			service service.Service
		}{name, svc})
	}

	// 第二阶段：事务性暂停所有服务
	var pausedServices []struct {
		name          string
		service       service.Service
		serviceConfig *config.ServiceConfig
	}

	// 获取服务配置
	serviceConfigs := make(map[string]*config.ServiceConfig)
	for _, s := range cfg.Services {
		serviceConfigs[s.Name] = s
	}

	// 逐个暂停服务，如果失败则回滚
	for _, stp := range servicesToPause {
		serviceConfig := serviceConfigs[stp.name]
		if serviceConfig == nil {
			// 找不到配置，回滚已暂停的服务
			rollbackPausedServices(pausedServices)
			writeError(ctx, NewError(http.StatusInternalServerError, ErrCodeFailed, fmt.Sprintf("service %s configuration not found", stp.name)))
			return
		}

		// 强制断开端口的所有连接
		if serviceConfig.Addr != "" {
			_ = kill.ForceClosePortConnections(serviceConfig.Addr)
		}

		// 记录已暂停的服务
		pausedServices = append(pausedServices, struct {
			name          string
			service       service.Service
			serviceConfig *config.ServiceConfig
		}{stp.name, stp.service, serviceConfig})
	}

	// 第三阶段：更新配置，标记暂停状态
	err := config.OnUpdate(func(c *config.Config) error {
		for _, stp := range servicesToPause {
			for i := range c.Services {
				if c.Services[i].Name == stp.name {
					if c.Services[i].Metadata == nil {
						c.Services[i].Metadata = make(map[string]any)
					}
					c.Services[i].Metadata["paused"] = true
					break
				}
			}
		}
		return nil
	})

	if err != nil {
		// 配置更新失败，需要回滚所有暂停的服务
		rollbackPausedServices(pausedServices)
		writeError(ctx, NewError(http.StatusInternalServerError, ErrCodeFailed, fmt.Sprintf("Failed to update config, rolling back paused services: %v", err)))
		return
	}

	ctx.JSON(http.StatusOK, Response{
		Msg: "OK",
	})
}

func resumeServices(ctx *gin.Context) {
	// swagger:route POST /config/services/batch/resume Service resumeServicesRequest
	//
	// Resume multiple paused services at once (transactional: all succeed or all fail).
	//
	//     Security:
	//       basicAuth: []
	//
	//     Responses:
	//       200: resumeServicesResponse

	var req resumeServicesRequest
	ctx.ShouldBindJSON(&req)

	if len(req.Services) == 0 {
		writeError(ctx, NewError(http.StatusBadRequest, ErrCodeInvalid, "services list cannot be empty"))
		return
	}

	// 第一阶段：验证所有服务是否存在，并筛选需要恢复的服务
	var servicesToResume []struct {
		name          string
		service       service.Service
		serviceConfig *config.ServiceConfig
	}
	var skippedServices []string

	cfg := config.Global()
	for _, serviceName := range req.Services {
		name := strings.TrimSpace(serviceName)
		if name == "" {
			writeError(ctx, NewError(http.StatusBadRequest, ErrCodeInvalid, "service name is required"))
			return
		}

		// 检查服务是否存在
		svc := registry.ServiceRegistry().Get(name)
		if svc == nil {
			writeError(ctx, NewError(http.StatusBadRequest, ErrCodeNotFound, fmt.Sprintf("service %s not found", name)))
			return
		}

		// 查找配置中的服务
		var serviceConfig *config.ServiceConfig
		for _, s := range cfg.Services {
			if s.Name == name {
				serviceConfig = s
				break
			}
		}

		if serviceConfig == nil {
			writeError(ctx, NewError(http.StatusBadRequest, ErrCodeNotFound, fmt.Sprintf("service %s configuration not found", name)))
			return
		}

		// 检查是否处于暂停状态
		paused := false
		if serviceConfig.Metadata != nil {
			if pausedVal, exists := serviceConfig.Metadata["paused"]; exists && pausedVal == true {
				paused = true
			}
		}

		// 如果服务没有暂停(即正在运行)，跳过
		if !paused {
			skippedServices = append(skippedServices, name)
			continue
		}

		servicesToResume = append(servicesToResume, struct {
			name          string
			service       service.Service
			serviceConfig *config.ServiceConfig
		}{name, svc, serviceConfig})
	}

	// 第二阶段：事务性恢复所有服务
	var resumedServices []struct {
		name          string
		service       service.Service
		serviceConfig *config.ServiceConfig
	}

	// 逐个恢复服务，如果失败则回滚
	for _, str := range servicesToResume {
		// 先关闭现有服务
		str.service.Close()
		registry.ServiceRegistry().Unregister(str.name)

		// 强制断开端口的所有连接
		if str.serviceConfig.Addr != "" {
			_ = kill.ForceClosePortConnections(str.serviceConfig.Addr)
		}

		// 等待端口释放
		time.Sleep(500 * time.Millisecond)

		// 重新解析并启动服务
		svc, err := parser.ParseService(str.serviceConfig)
		if err != nil {
			// 恢复失败，回滚已恢复的服务
			rollbackResumedServices(resumedServices)
			writeError(ctx, NewError(http.StatusInternalServerError, ErrCodeFailed, fmt.Sprintf("resume service %s failed: %s", str.name, err.Error())))
			return
		}

		if err := registry.ServiceRegistry().Register(str.name, svc); err != nil {
			svc.Close()
			// 恢复失败，回滚已恢复的服务
			rollbackResumedServices(resumedServices)
			writeError(ctx, NewError(http.StatusBadRequest, ErrCodeDup, fmt.Sprintf("service %s already exists", str.name)))
			return
		}

		go svc.Serve()

		// 记录已成功恢复的服务
		resumedServices = append(resumedServices, str)
	}

	// 第三阶段：更新配置，移除暂停状态
	err := config.OnUpdate(func(c *config.Config) error {
		for _, str := range servicesToResume {
			for i := range c.Services {
				if c.Services[i].Name == str.name {
					if c.Services[i].Metadata != nil {
						delete(c.Services[i].Metadata, "paused")
						// 如果 metadata 为空，设置为 nil
						if len(c.Services[i].Metadata) == 0 {
							c.Services[i].Metadata = nil
						}
					}
					break
				}
			}
		}
		return nil
	})

	if err != nil {
		// 配置更新失败，回滚所有已恢复的服务
		rollbackResumedServices(resumedServices)
		writeError(ctx, NewError(http.StatusInternalServerError, ErrCodeFailed, fmt.Sprintf("Failed to update config, rolling back resumed services: %v", err)))
		return
	}

	ctx.JSON(http.StatusOK, Response{
		Msg: "OK",
	})
}
