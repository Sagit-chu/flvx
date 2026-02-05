package com.admin.controller;

import com.admin.common.aop.LogAnnotation;
import com.admin.common.annotation.RequireRole;
import com.admin.common.dto.BatchDeleteDto;
import com.admin.common.dto.BatchRedeployDto;
import com.admin.common.dto.TunnelDto;
import com.admin.common.dto.TunnelUpdateDto;

import com.admin.common.dto.UserTunnelBatchAssignDto;
import com.admin.common.dto.UserTunnelDto;
import com.admin.common.dto.UserTunnelQueryDto;
import com.admin.common.dto.UserTunnelUpdateDto;
import com.admin.common.lang.R;
import com.admin.service.TunnelService;
import com.admin.service.UserTunnelService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.validation.annotation.Validated;
import org.springframework.web.bind.annotation.*;

import java.util.Map;

/**
 * <p>
 * 隧道前端控制器
 * </p>
 *
 * @author QAQ
 * @since 2025-06-03
 */
@RestController
@CrossOrigin
@RequestMapping("/api/v1/tunnel")
public class TunnelController extends BaseController {

    @Autowired
    private TunnelService tunnelService;
    
    @Autowired
    private UserTunnelService userTunnelService;

    @LogAnnotation
    @RequireRole
    @PostMapping("/create")
    public R create(@Validated @RequestBody TunnelDto tunnelDto) {
        return tunnelService.createTunnel(tunnelDto);
    }

    @LogAnnotation
    @RequireRole
    @PostMapping("/list")
    public R readAll() {
        return tunnelService.getAllTunnels();
    }

    @LogAnnotation
    @RequireRole
    @PostMapping("/update")
    public R update(@Validated @RequestBody TunnelUpdateDto tunnelUpdateDto) {
        return tunnelService.updateTunnel(tunnelUpdateDto);
    }

    @LogAnnotation
    @RequireRole
    @PostMapping("/delete")
    public R delete(@RequestBody Map<String, Object> params) {
        Long id = Long.valueOf(params.get("id").toString());
        return tunnelService.deleteTunnel(id);
    }

    // ============ 用户隧道权限管理相关方法 ============
    
    /**
     * 分配用户隧道权限
     * @param userTunnelDto 用户隧道权限数据
     * @return 操作结果
     */
    @LogAnnotation
    @RequireRole
    @PostMapping("/user/assign")
    public R assignUserTunnel(@Validated @RequestBody UserTunnelDto userTunnelDto) {
        return userTunnelService.assignUserTunnel(userTunnelDto);
    }

    @LogAnnotation
    @RequireRole
    @PostMapping("/user/batch-assign")
    public R batchAssignUserTunnel(@Validated @RequestBody UserTunnelBatchAssignDto batchAssignDto) {
        return userTunnelService.batchAssignUserTunnel(batchAssignDto);
    }
    
    /**
     * 查询用户隧道权限列表
     * @param queryDto 查询条件
     * @return 用户隧道权限列表
     */
    @LogAnnotation
    @RequireRole
    @PostMapping("/user/list")
    public R getUserTunnelList(@RequestBody @Validated UserTunnelQueryDto queryDto) {
        return userTunnelService.getUserTunnelList(queryDto);
    }
    
    /**
     * 删除用户隧道权限
     * @param params 包含userId和tunnelId的参数
     * @return 操作结果
     */
    @LogAnnotation
    @RequireRole
    @PostMapping("/user/remove")
    public R removeUserTunnel(@RequestBody Map<String, Object> params) {
        Integer id = Integer.valueOf(params.get("id").toString());
        return userTunnelService.removeUserTunnel(id);
    }

    
    /**
     * 更新用户隧道权限（包含流量、流量重置时间、到期时间）
     * @param updateDto 更新数据
     * @return 操作结果
     */
    @LogAnnotation
    @RequireRole
    @PostMapping("/user/update")
    public R updateUserTunnel(@Validated @RequestBody UserTunnelUpdateDto updateDto) {
        return userTunnelService.updateUserTunnel(updateDto);
    }


    @LogAnnotation
    @PostMapping("/user/tunnel")
    public R userTunnel() {
        return tunnelService.userTunnel();
    }

    /**
     * 隧道诊断功能
     * @param params 包含tunnelId的参数
     * @return 诊断结果
     */
    @LogAnnotation
    @RequireRole
    @PostMapping("/diagnose")
    public R diagnoseTunnel(@RequestBody Map<String, Object> params) {
        Long tunnelId = Long.valueOf(params.get("tunnelId").toString());
        return tunnelService.diagnoseTunnel(tunnelId);
    }

    /**
     * 更新隧道排序
     * @param params 包含tunnels数组的参数，每个元素包含id和inx
     * @return 更新结果
     */
    @LogAnnotation
    @RequireRole
    @PostMapping("/update-order")
    public R updateTunnelOrder(@RequestBody Map<String, Object> params) {
        return tunnelService.updateTunnelOrder(params);
    }

    @LogAnnotation
    @RequireRole
    @PostMapping("/batch-delete")
    public R batchDelete(@Validated @RequestBody BatchDeleteDto batchDeleteDto) {
        return tunnelService.batchDeleteTunnels(batchDeleteDto);
    }

    @LogAnnotation
    @RequireRole
    @PostMapping("/batch-redeploy")
    public R batchRedeploy(@Validated @RequestBody BatchRedeployDto batchRedeployDto) {
        return tunnelService.batchRedeployTunnels(batchRedeployDto);
    }

}
