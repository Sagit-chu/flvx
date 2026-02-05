package com.admin.service;

import com.admin.common.dto.BatchDeleteDto;
import com.admin.common.dto.BatchRedeployDto;
import com.admin.common.dto.TunnelDto;
import com.admin.common.dto.TunnelUpdateDto;

import com.admin.common.lang.R;
import com.admin.entity.Tunnel;
import com.baomidou.mybatisplus.extension.service.IService;

import java.util.Map;

/**
 * <p>
 * 隧道服务类
 * </p>
 *
 * @author QAQ
 * @since 2025-06-03
 */
public interface TunnelService extends IService<Tunnel> {

    /**
     * 创建隧道
     * @param tunnelDto 隧道数据
     * @return 结果
     */
    R createTunnel(TunnelDto tunnelDto);

    /**
     * 获取隧道列表
     * @return 结果
     */
    R getAllTunnels();

    /**
     * 更新隧道（只允许修改名称、流量计费、端口范围）
     * @param tunnelUpdateDto 更新数据
     * @return 结果
     */
    R updateTunnel(TunnelUpdateDto tunnelUpdateDto);

    /**
     * 删除隧道
     * @param id 隧道ID
     * @return 结果
     */
    R deleteTunnel(Long id);

    /**
     * 获取用户可用的隧道列表
     * @return 结果
     */
    R userTunnel();

    /**
     * 隧道诊断功能
     * @param tunnelId 隧道ID
     * @return 诊断结果
     */
    R diagnoseTunnel(Long tunnelId);

    /**
     * 更新隧道排序（管理员）
     * @param params 包含tunnels数组的参数，每个元素包含id和inx
     */
    R updateTunnelOrder(Map<String, Object> params);

    /**
     * 批量删除隧道
     * @param batchDeleteDto 批量删除数据
     * @return 操作结果
     */
    R batchDeleteTunnels(BatchDeleteDto batchDeleteDto);

    /**
     * 批量重新下发隧道配置
     * @param batchRedeployDto 批量重新下发数据
     * @return 操作结果
     */
    R batchRedeployTunnels(BatchRedeployDto batchRedeployDto);
}
