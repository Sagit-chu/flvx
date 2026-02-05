package com.admin.service.impl;

import com.admin.common.dto.*;

import com.admin.common.lang.R;
import com.admin.common.utils.GostUtil;
import com.admin.common.utils.JwtUtil;
import com.admin.common.utils.WebSocketServer;
import com.admin.entity.*;
import com.admin.mapper.TunnelMapper;
import com.admin.mapper.UserTunnelMapper;
import com.admin.service.*;
import com.alibaba.fastjson.JSONArray;
import com.alibaba.fastjson.JSONObject;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import com.baomidou.mybatisplus.extension.service.impl.ServiceImpl;
import lombok.Data;
import org.apache.commons.lang3.StringUtils;
import org.springframework.beans.BeanUtils;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import javax.annotation.Resource;
import java.util.*;
import java.util.concurrent.CompletableFuture;
import java.util.stream.Collectors;

/**
 *
 * @author QAQ
 * @since 2025-06-03
 */
@Service
public class TunnelServiceImpl extends ServiceImpl<TunnelMapper, Tunnel> implements TunnelService {


    @Resource
    UserTunnelMapper userTunnelMapper;

    @Resource
    NodeService nodeService;

    @Resource
    ForwardService forwardService;

    @Resource
    UserTunnelService userTunnelService;

    @Resource
    ChainTunnelService chainTunnelService;

    @Resource
    ForwardPortService forwardPortService;


    @Override
    public R createTunnel(TunnelDto tunnelDto) {

        int count = this.count(new QueryWrapper<Tunnel>().eq("name", tunnelDto.getName()));
        if (count > 0) return R.err("隧道名称重复");
        if (tunnelDto.getType() == 2 && tunnelDto.getOutNodeId() == null) return R.err("出口不能为空");


        List<ChainTunnel> chainTunnels = new ArrayList<>();
        Map<Long, Node> nodes = new HashMap<>();

        List<Long> node_ids = new ArrayList<>();
        for (ChainTunnel in_node : tunnelDto.getInNodeId()) {
            node_ids.add(in_node.getNodeId());
            chainTunnels.add(in_node);

            Node node = nodeService.getById(in_node.getNodeId());
            if (node == null) return R.err("节点不存在");
            nodes.put(node.getId(), node);
        }

        if (tunnelDto.getType() == 2) {
            // 处理转发链节点，为每一跳设置inx
            int inx = 1;
            for (List<ChainTunnel> chainNode : tunnelDto.getChainNodes()) {
                for (ChainTunnel chain_node : chainNode) {
                    node_ids.add(chain_node.getNodeId());
                    Node node = nodeService.getById(chain_node.getNodeId());
                    if (node == null) return R.err("节点不存在");
                    nodes.put(node.getId(), node);
                    Integer nodePort = getNodePort(chain_node.getNodeId());
                    chain_node.setPort(nodePort);
                    chain_node.setInx(inx); // 设置转发链序号
                    chainTunnels.add(chain_node);
                }
                inx++; // 每一跳递增
            }
            for (ChainTunnel out_node : tunnelDto.getOutNodeId()) {
                node_ids.add(out_node.getNodeId());
                Node node = nodeService.getById(out_node.getNodeId());
                if (node == null) return R.err("节点不存在");
                nodes.put(node.getId(), node);
                Integer nodePort = getNodePort(out_node.getNodeId());
                out_node.setPort(nodePort);
                chainTunnels.add(out_node);
            }

        }
        Set<Long> set = new HashSet<>(node_ids);
        boolean hasDuplicate = set.size() != node_ids.size();
        if (hasDuplicate) return R.err("节点重复");

        List<Node> list = nodeService.list(new QueryWrapper<Node>().in("id", node_ids));
        if (list.size() != node_ids.size()) return R.err("部分节点不存在");
        for (Node node : list) {
            if (node.getStatus() != 1) return R.err("部分节点不在线");
        }


        Tunnel tunnel = new Tunnel();
        BeanUtils.copyProperties(tunnelDto, tunnel);
        tunnel.setStatus(1);
        long currentTime = System.currentTimeMillis();
        tunnel.setCreatedTime(currentTime);
        tunnel.setUpdatedTime(currentTime);

        // When tunnels are ordered via `inx`, new tunnels should be appended.
        // Only apply this when an order already exists (max `inx` > 0) to avoid
        // changing behavior for deployments still relying on local ordering.
        Tunnel lastByInx = this.getOne(new QueryWrapper<Tunnel>()
                .select("inx")
                .orderByDesc("inx")
                .orderByDesc("id")
                .last("LIMIT 1"));
        Integer maxInx = lastByInx == null ? null : lastByInx.getInx();
        if (maxInx != null && maxInx > 0) {
            tunnel.setInx(maxInx + 1);
        }
        if (StringUtils.isEmpty(tunnel.getInIp())){
            java.util.LinkedHashSet<String> inIps = new java.util.LinkedHashSet<>();
            for (ChainTunnel chainTunnel : tunnelDto.getInNodeId()) {
                Node node = nodes.get(chainTunnel.getNodeId());
                if (node == null) continue;
                if (cn.hutool.core.util.StrUtil.isNotBlank(node.getServerIpV4())) {
                    inIps.add(node.getServerIpV4().trim());
                }
                if (cn.hutool.core.util.StrUtil.isNotBlank(node.getServerIpV6())) {
                    inIps.add(node.getServerIpV6().trim());
                }
                if (cn.hutool.core.util.StrUtil.isBlank(node.getServerIpV4())
                        && cn.hutool.core.util.StrUtil.isBlank(node.getServerIpV6())
                        && cn.hutool.core.util.StrUtil.isNotBlank(node.getServerIp())) {
                    inIps.add(node.getServerIp().trim());
                }
            }
            if (!inIps.isEmpty()) {
                tunnel.setInIp(String.join(",", inIps));
            }
        }

        this.save(tunnel);
        for (ChainTunnel chainTunnel : chainTunnels) {
            chainTunnel.setTunnelId(tunnel.getId());
        }
        chainTunnelService.saveBatch(chainTunnels);

        List<JSONObject> chain_success = new ArrayList<>();
        List<JSONObject> service_success = new ArrayList<>();



        if (tunnel.getType() == 2) {

            for (ChainTunnel in_node : tunnelDto.getInNodeId()) {
                // 创建Chain， 指向chainNode的第一跳。如果chainNode为空就是指向出口
                if (tunnelDto.getChainNodes().isEmpty()) { // 指向出口
                    GostDto gostDto;
                    try {
                        gostDto = GostUtil.AddChains(in_node.getNodeId(), tunnelDto.getOutNodeId(), nodes);
                    } catch (RuntimeException e) {
                        this.removeById(tunnel.getId());
                        chainTunnelService.remove(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId()));
                        return R.err(e.getMessage());
                    }
                    if (!Objects.equals(gostDto.getMsg(), "OK")) {
                        this.removeById(tunnel.getId());
                        chainTunnelService.remove(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId()));
                        return R.err(gostDto.getMsg());
                    }

                } else {
                    GostDto gostDto;
                    try {
                        gostDto = GostUtil.AddChains(in_node.getNodeId(), tunnelDto.getChainNodes().getFirst(), nodes);// 指向第一跳
                    } catch (RuntimeException e) {
                        this.removeById(tunnel.getId());
                        chainTunnelService.remove(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId()));
                        for (JSONObject chainSuccess : chain_success) {
                            GostDto deleteChains = GostUtil.DeleteChains(chainSuccess.getLong("node_id"), chainSuccess.getString("name"));
                            System.out.println(deleteChains);
                        }
                        return R.err(e.getMessage());
                    }
                    if (Objects.equals(gostDto.getMsg(), "OK")){
                        JSONObject data = new JSONObject();
                        data.put("node_id", in_node.getNodeId());
                        data.put("name", "chains_" + tunnel.getId());
                        chain_success.add(data);
                    }else {
                        this.removeById(tunnel.getId());
                        chainTunnelService.remove(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId()));
                        for (JSONObject chainSuccess : chain_success) {
                            GostDto deleteChains = GostUtil.DeleteChains(chainSuccess.getLong("node_id"), chainSuccess.getString("name"));
                            System.out.println(deleteChains);
                        }
                        return R.err(gostDto.getMsg());
                    }
                }
            }

            for (int i = 0; i < tunnelDto.getChainNodes().size(); i++) {
                //  创建Chain和Service。每一条的Chain都是指向下一跳。最后一跳指向出口， Service是监听端口
                List<ChainTunnel> chainTunnels1 = tunnelDto.getChainNodes().get(i);
                for (ChainTunnel chainTunnel : chainTunnels1) {
                    int inx = i+1;
                    if (inx >= tunnelDto.getChainNodes().size()) { // 指向出口
                        GostDto gostDto;
                        try {
                            gostDto = GostUtil.AddChains(chainTunnel.getNodeId(), tunnelDto.getOutNodeId(), nodes);
                        } catch (RuntimeException e) {
                            this.removeById(tunnel.getId());
                            chainTunnelService.remove(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId()));
                            for (JSONObject chainSuccess : chain_success) {
                                GostDto deleteChains = GostUtil.DeleteChains(chainSuccess.getLong("node_id"), chainSuccess.getString("name"));
                                System.out.println(deleteChains);
                            }
                            return R.err(e.getMessage());
                        }
                        if (Objects.equals(gostDto.getMsg(), "OK")){
                            JSONObject data = new JSONObject();
                            data.put("node_id", chainTunnel.getNodeId());
                            data.put("name", "chains_" + tunnel.getId());
                            chain_success.add(data);
                        }else {
                            this.removeById(tunnel.getId());
                            chainTunnelService.remove(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId()));
                            for (JSONObject chainSuccess : chain_success) {
                                GostDto deleteChains = GostUtil.DeleteChains(chainSuccess.getLong("node_id"), chainSuccess.getString("name"));
                                System.out.println(deleteChains);
                            }
                            return R.err(gostDto.getMsg());
                        }
                    } else {
                        GostDto gostDto;
                        try {
                            gostDto = GostUtil.AddChains(chainTunnel.getNodeId(), tunnelDto.getChainNodes().get(inx), nodes);
                        } catch (RuntimeException e) {
                            this.removeById(tunnel.getId());
                            chainTunnelService.remove(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId()));
                            for (JSONObject chainSuccess : chain_success) {
                                GostDto deleteChains = GostUtil.DeleteChains(chainSuccess.getLong("node_id"), chainSuccess.getString("name"));
                                System.out.println(deleteChains);
                            }
                            return R.err(e.getMessage());
                        }
                        if (Objects.equals(gostDto.getMsg(), "OK")){
                            JSONObject data = new JSONObject();
                            data.put("node_id", chainTunnel.getNodeId());
                            data.put("name", "chains_" + tunnel.getId());
                            chain_success.add(data);
                        }else {
                            this.removeById(tunnel.getId());
                            chainTunnelService.remove(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId()));
                            for (JSONObject chainSuccess : chain_success) {
                                GostDto deleteChains = GostUtil.DeleteChains(chainSuccess.getLong("node_id"), chainSuccess.getString("name"));
                                System.out.println(deleteChains);
                            }
                            return R.err(gostDto.getMsg());
                        }
                    }

                    GostDto gostDto = GostUtil.AddChainService(chainTunnel.getNodeId(), chainTunnel, nodes);
                    if (Objects.equals(gostDto.getMsg(), "OK")){
                        JSONObject data = new JSONObject();
                        data.put("node_id", chainTunnel.getNodeId());
                        data.put("name", tunnel.getId() + "_tls");
                        service_success.add(data);
                    }else {
                        this.removeById(tunnel.getId());
                        chainTunnelService.remove(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId()));
                        for (JSONObject serviceSuccess : service_success) {
                            JSONArray jsonArray = new JSONArray();
                            jsonArray.add(serviceSuccess.getString("name"));
                            GostDto deleteService = GostUtil.DeleteService(serviceSuccess.getLong("node_id"), jsonArray);
                            System.out.println(deleteService);
                        }
                        return R.err(gostDto.getMsg());
                    }
                }

            }


            for (ChainTunnel out_node : tunnelDto.getOutNodeId()) {
                GostDto gostDto = GostUtil.AddChainService(out_node.getNodeId(), out_node, nodes);
                if (Objects.equals(gostDto.getMsg(), "OK")){
                    JSONObject data = new JSONObject();
                    data.put("node_id", out_node.getNodeId());
                    data.put("name", tunnel.getId() + "_tls");
                    service_success.add(data);
                }else {
                    this.removeById(tunnel.getId());
                    chainTunnelService.remove(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId()));
                    for (JSONObject serviceSuccess : service_success) {
                        JSONArray jsonArray = new JSONArray();
                        jsonArray.add(serviceSuccess.getString("name"));
                        GostDto deleteService = GostUtil.DeleteService(serviceSuccess.getLong("node_id"), jsonArray);
                        System.out.println(deleteService);
                    }
                    return R.err(gostDto.getMsg());
                }
            }

        }
        return R.ok();
    }


    @Override
    public R getAllTunnels() {
        List<Tunnel> tunnelList = this.list(new QueryWrapper<Tunnel>().orderByAsc("inx").orderByAsc("id"));
        
        // 查询所有隧道的ChainTunnel信息
        List<Long> tunnelIds = tunnelList.stream()
                .map(Tunnel::getId)
                .collect(Collectors.toList());
        
        if (tunnelIds.isEmpty()) {
            return R.ok(new ArrayList<TunnelDetailDto>());
        }
        
        // 批量查询所有ChainTunnel记录
        List<ChainTunnel> allChainTunnels = chainTunnelService.list(
                new QueryWrapper<ChainTunnel>().in("tunnel_id", tunnelIds)
        );
        
        // 按tunnelId分组
        Map<Long, List<ChainTunnel>> chainTunnelMap = allChainTunnels.stream()
                .collect(Collectors.groupingBy(ChainTunnel::getTunnelId));
        
        // 转换为TunnelDetailDto列表
        List<TunnelDetailDto> detailDtoList = tunnelList.stream()
                .map(tunnel -> {
                    TunnelDetailDto detailDto = new TunnelDetailDto();
                    BeanUtils.copyProperties(tunnel, detailDto);
                    
                    List<ChainTunnel> chainTunnels = chainTunnelMap.getOrDefault(tunnel.getId(), new ArrayList<>());
                    
                    // 按chainType分类节点
                    // 入口节点 (chainType = 1)
                    List<ChainTunnel> inNodes = chainTunnels.stream()
                            .filter(ct -> ct.getChainType() != null && ct.getChainType() == 1)
                            .collect(Collectors.toList());
                    detailDto.setInNodeId(inNodes);

                    detailDto.setInIp(tunnel.getInIp());
                    
                    // 转发链节点 (chainType = 2) - 按inx分组
                    Map<Integer, List<ChainTunnel>> chainNodesMap = chainTunnels.stream()
                            .filter(ct -> ct.getChainType() != null && ct.getChainType() == 2)
                            .collect(Collectors.groupingBy(
                                    ct -> ct.getInx() != null ? ct.getInx() : 0,
                                    Collectors.toList()
                            ));
                    
                    // 将Map转换为按inx排序的二维列表
                    List<List<ChainTunnel>> chainNodesList = chainNodesMap.entrySet().stream()
                            .sorted(Map.Entry.comparingByKey())
                            .map(Map.Entry::getValue)
                            .collect(Collectors.toList());
                    detailDto.setChainNodes(chainNodesList);
                    
                    // 出口节点 (chainType = 3)
                    List<ChainTunnel> outNodes = chainTunnels.stream()
                            .filter(ct -> ct.getChainType() != null && ct.getChainType() == 3)
                            .collect(Collectors.toList());
                    detailDto.setOutNodeId(outNodes);
                    
                    return detailDto;
                })
                .collect(Collectors.toList());
        
        return R.ok(detailDtoList);
    }

    @Override
    @Transactional
    public R updateTunnelOrder(Map<String, Object> params) {
        if (!params.containsKey("tunnels")) {
            return R.err("缺少tunnels参数");
        }

        @SuppressWarnings("unchecked")
        List<Map<String, Object>> tunnelsList = (List<Map<String, Object>>) params.get("tunnels");
        if (tunnelsList == null || tunnelsList.isEmpty()) {
            return R.err("tunnels参数不能为空");
        }

        List<Tunnel> tunnelsToUpdate = new ArrayList<>();
        for (Map<String, Object> tunnelData : tunnelsList) {
            Long id = Long.valueOf(tunnelData.get("id").toString());
            Integer inx = Integer.valueOf(tunnelData.get("inx").toString());

            Tunnel tunnel = new Tunnel();
            tunnel.setId(id);
            tunnel.setInx(inx);
            tunnelsToUpdate.add(tunnel);
        }

        this.updateBatchById(tunnelsToUpdate);
        return R.ok();
    }


    @Override
    public R updateTunnel(TunnelUpdateDto tunnelUpdateDto) {
        Tunnel existingTunnel = this.getById(tunnelUpdateDto.getId());
        if (existingTunnel == null) return R.err("隧道不存在");

        List<ChainTunnel> oldChainTunnels = chainTunnelService.list(
                new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnelUpdateDto.getId())
        );

        boolean hasNodeChanges = detectNodeChanges(oldChainTunnels, tunnelUpdateDto);

        if (hasNodeChanges && tunnelUpdateDto.getInNodeId() != null) {
            List<ChainTunnel> backupChains = deepCopyChainTunnels(oldChainTunnels);

            Set<Long> oldEntryNodeIds = oldChainTunnels.stream()
                    .filter(ct -> ct.getChainType() != null && ct.getChainType() == 1)
                    .map(ChainTunnel::getNodeId)
                    .collect(Collectors.toSet());

            Set<Long> newEntryNodeIds = tunnelUpdateDto.getInNodeId().stream()
                    .map(ChainTunnel::getNodeId)
                    .collect(Collectors.toSet());

            List<Long> nodeIds = new ArrayList<>();
            Map<Long, Node> nodes = new HashMap<>();

            for (ChainTunnel inNode : tunnelUpdateDto.getInNodeId()) {
                nodeIds.add(inNode.getNodeId());
                Node node = nodeService.getById(inNode.getNodeId());
                if (node == null) return R.err("入口节点不存在: " + inNode.getNodeId());
                if (node.getStatus() != 1) return R.err("入口节点不在线: " + node.getName());
                nodes.put(node.getId(), node);
            }

            List<ChainTunnel> newChainTunnels = new ArrayList<>();
            for (ChainTunnel inNode : tunnelUpdateDto.getInNodeId()) {
                inNode.setTunnelId(existingTunnel.getId());
                inNode.setChainType(1);
                newChainTunnels.add(inNode);
            }

            if (existingTunnel.getType() == 2) {
                if (tunnelUpdateDto.getOutNodeId() == null || tunnelUpdateDto.getOutNodeId().isEmpty()) {
                    return R.err("隧道转发类型必须配置出口节点");
                }

                List<List<ChainTunnel>> chainNodes = tunnelUpdateDto.getChainNodes() == null ?
                        new ArrayList<>() : tunnelUpdateDto.getChainNodes();

                int inx = 1;
                for (List<ChainTunnel> hop : chainNodes) {
                    for (ChainTunnel chainNode : hop) {
                        nodeIds.add(chainNode.getNodeId());
                        Node node = nodeService.getById(chainNode.getNodeId());
                        if (node == null) return R.err("转发链节点不存在: " + chainNode.getNodeId());
                        if (node.getStatus() != 1) return R.err("转发链节点不在线: " + node.getName());
                        nodes.put(node.getId(), node);

                        Integer port = getNodePort(chainNode.getNodeId());
                        chainNode.setPort(port);
                        chainNode.setInx(inx);
                        chainNode.setChainType(2);
                        chainNode.setTunnelId(existingTunnel.getId());
                        newChainTunnels.add(chainNode);
                    }
                    inx++;
                }

                for (ChainTunnel outNode : tunnelUpdateDto.getOutNodeId()) {
                    nodeIds.add(outNode.getNodeId());
                    Node node = nodeService.getById(outNode.getNodeId());
                    if (node == null) return R.err("出口节点不存在: " + outNode.getNodeId());
                    if (node.getStatus() != 1) return R.err("出口节点不在线: " + node.getName());
                    nodes.put(node.getId(), node);

                    Integer port = getNodePort(outNode.getNodeId());
                    outNode.setPort(port);
                    outNode.setChainType(3);
                    outNode.setTunnelId(existingTunnel.getId());
                    newChainTunnels.add(outNode);
                }
            }

            Set<Long> nodeIdSet = new HashSet<>(nodeIds);
            if (nodeIdSet.size() != nodeIds.size()) {
                return R.err("节点配置重复");
            }

            try {
                cleanupGostConfig(oldChainTunnels, existingTunnel.getId());

                chainTunnelService.remove(
                        new QueryWrapper<ChainTunnel>().eq("tunnel_id", existingTunnel.getId())
                );

                R applyResult = applyNewGostConfig(tunnelUpdateDto, existingTunnel, nodes);
                if (applyResult.getCode() != 0) {
                    chainTunnelService.saveBatch(backupChains);
                    rebuildGostConfig(backupChains, existingTunnel);
                    return R.err("更新失败，已回滚: " + applyResult.getMsg());
                }

                chainTunnelService.saveBatch(newChainTunnels);

                syncForwardsForEntryNodeChanges(existingTunnel.getId(), oldEntryNodeIds, newEntryNodeIds);

            } catch (Exception e) {
                chainTunnelService.saveBatch(backupChains);
                rebuildGostConfig(backupChains, existingTunnel);
                return R.err("更新失败，已回滚: " + e.getMessage());
            }
        }

        Tunnel tunnel = new Tunnel();
        tunnel.setId(tunnelUpdateDto.getId());
        tunnel.setName(tunnelUpdateDto.getName());
        tunnel.setFlow(tunnelUpdateDto.getFlow());
        tunnel.setTrafficRatio(tunnelUpdateDto.getTrafficRatio());
        tunnel.setInIp(tunnelUpdateDto.getInIp());

        boolean forceRegenerateInIp = hasNodeChanges && tunnelUpdateDto.getInNodeId() != null;
        if (StringUtils.isEmpty(tunnel.getInIp()) || forceRegenerateInIp) {
            java.util.LinkedHashSet<String> inIps = new java.util.LinkedHashSet<>();
            List<ChainTunnel> chainTunnels = chainTunnelService.list(
                    new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId()).eq("chain_type", 1)
            );
            for (ChainTunnel chainTunnel : chainTunnels) {
                Node node = nodeService.getById(chainTunnel.getNodeId());
                if (node == null) return R.err("隧道节点数据错误，部分节点不存在");

                if (cn.hutool.core.util.StrUtil.isNotBlank(node.getServerIpV4())) {
                    inIps.add(node.getServerIpV4().trim());
                }
                if (cn.hutool.core.util.StrUtil.isNotBlank(node.getServerIpV6())) {
                    inIps.add(node.getServerIpV6().trim());
                }
                if (cn.hutool.core.util.StrUtil.isBlank(node.getServerIpV4())
                        && cn.hutool.core.util.StrUtil.isBlank(node.getServerIpV6())
                        && cn.hutool.core.util.StrUtil.isNotBlank(node.getServerIp())) {
                    inIps.add(node.getServerIp().trim());
                }
            }
            if (!inIps.isEmpty()) {
                tunnel.setInIp(String.join(",", inIps));
            }
        }

        tunnel.setUpdatedTime(System.currentTimeMillis());
        this.updateById(tunnel);
        return R.ok();
    }


    @Override
    public R deleteTunnel(Long id) {
        Tunnel tunnel = this.getById(id);
        if (tunnel == null) return R.err("隧道不存在");
        List<Forward> forwardList = forwardService.list(new QueryWrapper<Forward>().eq("tunnel_id", id));
        for (Forward forward : forwardList) {
            forwardService.deleteForward(forward.getId());
        }
        forwardService.remove(new QueryWrapper<Forward>().eq("tunnel_id", id));
        userTunnelService.remove(new QueryWrapper<UserTunnel>().eq("tunnel_id", id));
        this.removeById(id);

        List<ChainTunnel> chainTunnels = chainTunnelService.list(new QueryWrapper<ChainTunnel>().eq("tunnel_id", id));
        for (ChainTunnel chainTunnel : chainTunnels) {
            if (chainTunnel.getChainType() == 1){ // 入口
                GostUtil.DeleteChains(chainTunnel.getNodeId(), "chains_" + chainTunnel.getTunnelId());
            }
            else if (chainTunnel.getChainType() == 2){ // 链
                GostUtil.DeleteChains(chainTunnel.getNodeId(), "chains_" + chainTunnel.getTunnelId());
                JSONArray services = new JSONArray();
                services.add(chainTunnel.getTunnelId() + "_tls");
                GostUtil.DeleteService(chainTunnel.getNodeId(), services);
            }
            else { // 出口
                JSONArray services = new JSONArray();
                services.add(chainTunnel.getTunnelId() + "_tls");
                GostUtil.DeleteService(chainTunnel.getNodeId(), services);
            }
        }
        chainTunnelService.remove(new QueryWrapper<ChainTunnel>().eq("tunnel_id", id));
        return R.ok();
    }


    @Override
    public R userTunnel() {
        List<Tunnel> tunnelEntities;
        Integer roleId = JwtUtil.getRoleIdFromToken();
        Integer userId = JwtUtil.getUserIdFromToken();
        if (roleId == 0) {
            tunnelEntities = this.list(new QueryWrapper<Tunnel>()
                    .eq("status", 1)
                    .orderByAsc("inx")
                    .orderByAsc("id"));
        } else {
            tunnelEntities = java.util.Collections.emptyList(); // 返回空列表
            List<UserTunnel> userTunnels = userTunnelMapper.selectList(
                    new QueryWrapper<UserTunnel>().eq("user_id", userId)
            );
            if (!userTunnels.isEmpty()) {
                List<Integer> tunnelIds = userTunnels.stream()
                        .map(UserTunnel::getTunnelId)
                        .collect(Collectors.toList());
                tunnelEntities = this.list(new QueryWrapper<Tunnel>()
                        .in("id", tunnelIds)
                        .eq("status", 1)
                        .orderByAsc("inx")
                        .orderByAsc("id"));
            }

        }
        return R.ok(tunnelEntities);
    }


    @Override
    public R diagnoseTunnel(Long tunnelId) {
        Tunnel tunnel = this.getById(tunnelId);
        if (tunnel == null) {
            return R.err("隧道不存在");
        }

        List<ChainTunnel> chainTunnels = chainTunnelService.list(
                new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnelId)
        );

        if (chainTunnels.isEmpty()) {
            return R.err("隧道配置不完整");
        }

        List<ChainTunnel> inNodes = chainTunnels.stream()
                .filter(ct -> ct.getChainType() == 1)
                .toList();

        Map<Integer, List<ChainTunnel>> chainNodesMap = chainTunnels.stream()
                .filter(ct -> ct.getChainType() == 2)
                .collect(Collectors.groupingBy(
                        ct -> ct.getInx() != null ? ct.getInx() : 0,
                        Collectors.toList()
                ));

        List<List<ChainTunnel>> chainNodesList = chainNodesMap.entrySet().stream()
                .sorted(Map.Entry.comparingByKey())
                .map(Map.Entry::getValue)
                .toList();

        List<ChainTunnel> outNodes = chainTunnels.stream()
                .filter(ct -> ct.getChainType() == 3)
                .toList();

        List<CompletableFuture<DiagnosisResult>> futures = new ArrayList<>();

        if (tunnel.getType() == 1) {
            for (ChainTunnel inNode : inNodes) {
                Node node = nodeService.getById(inNode.getNodeId());
                if (node != null) {
                    final Node finalNode = node;
                    futures.add(CompletableFuture.supplyAsync(() -> {
                        DiagnosisResult result = performTcpPingDiagnosisWithConnectionCheck(
                                finalNode, "www.google.com", 443, "入口(" + finalNode.getName() + ")->外网"
                        );
                        result.setFromChainType(1);
                        return result;
                    }));
                }
            }
        } else if (tunnel.getType() == 2) {
            for (ChainTunnel inNode : inNodes) {
                Node fromNode = nodeService.getById(inNode.getNodeId());
                
                if (fromNode != null) {
                    if (!chainNodesList.isEmpty()) {
                        for (ChainTunnel firstChainNode : chainNodesList.getFirst()) {
                            Node toNode = nodeService.getById(firstChainNode.getNodeId());
                            if (toNode != null) {
                                final Node finalFromNode = fromNode;
                                final Node finalToNode = toNode;
                                final ChainTunnel finalFirstChainNode = firstChainNode;
                                futures.add(CompletableFuture.supplyAsync(() -> {
                                    DiagnosisResult result = performTcpPingDiagnosisWithConnectionCheck(
                                            finalFromNode, GostUtil.selectDialHost(finalFromNode, finalToNode), finalFirstChainNode.getPort(),
                                            "入口(" + finalFromNode.getName() + ")->第1跳(" + finalToNode.getName() + ")"
                                    );
                                    result.setFromChainType(1);
                                    result.setToChainType(2);
                                    result.setToInx(finalFirstChainNode.getInx());
                                    return result;
                                }));
                            }
                        }
                    } else if (!outNodes.isEmpty()) {
                        for (ChainTunnel outNode : outNodes) {
                            Node toNode = nodeService.getById(outNode.getNodeId());
                            if (toNode != null) {
                                final Node finalFromNode = fromNode;
                                final Node finalToNode = toNode;
                                final ChainTunnel finalOutNode = outNode;
                                futures.add(CompletableFuture.supplyAsync(() -> {
                                    DiagnosisResult result = performTcpPingDiagnosisWithConnectionCheck(
                                            finalFromNode, GostUtil.selectDialHost(finalFromNode, finalToNode), finalOutNode.getPort(),
                                            "入口(" + finalFromNode.getName() + ")->出口(" + finalToNode.getName() + ")"
                                    );
                                    result.setFromChainType(1);
                                    result.setToChainType(3);
                                    return result;
                                }));
                            }
                        }
                    }
                }
            }

            for (int i = 0; i < chainNodesList.size(); i++) {
                List<ChainTunnel> currentHop = chainNodesList.get(i);
                final int hopIndex = i;
                
                for (ChainTunnel currentNode : currentHop) {
                    Node fromNode = nodeService.getById(currentNode.getNodeId());
                    
                    if (fromNode != null) {
                        if (i + 1 < chainNodesList.size()) {
                            for (ChainTunnel nextNode : chainNodesList.get(i + 1)) {
                                Node toNode = nodeService.getById(nextNode.getNodeId());
                                if (toNode != null) {
                                    final Node finalFromNode = fromNode;
                                    final Node finalToNode = toNode;
                                    final ChainTunnel finalCurrentNode = currentNode;
                                    final ChainTunnel finalNextNode = nextNode;
                                    futures.add(CompletableFuture.supplyAsync(() -> {
                                        DiagnosisResult result = performTcpPingDiagnosisWithConnectionCheck(
                                                    finalFromNode, GostUtil.selectDialHost(finalFromNode, finalToNode), finalNextNode.getPort(),
                                                "第" + (hopIndex + 1) + "跳(" + finalFromNode.getName() + ")->第" + (hopIndex + 2) + "跳(" + finalToNode.getName() + ")"
                                        );
                                        result.setFromChainType(2);
                                        result.setFromInx(finalCurrentNode.getInx());
                                        result.setToChainType(2);
                                        result.setToInx(finalNextNode.getInx());
                                        return result;
                                    }));
                                }
                            }
                        } else if (!outNodes.isEmpty()) {
                            for (ChainTunnel outNode : outNodes) {
                                Node toNode = nodeService.getById(outNode.getNodeId());
                                if (toNode != null) {
                                    final Node finalFromNode = fromNode;
                                    final Node finalToNode = toNode;
                                    final ChainTunnel finalCurrentNode = currentNode;
                                    final ChainTunnel finalOutNode = outNode;
                                    futures.add(CompletableFuture.supplyAsync(() -> {
                                        DiagnosisResult result = performTcpPingDiagnosisWithConnectionCheck(
                                                    finalFromNode, GostUtil.selectDialHost(finalFromNode, finalToNode), finalOutNode.getPort(),
                                                "第" + (hopIndex + 1) + "跳(" + finalFromNode.getName() + ")->出口(" + finalToNode.getName() + ")"
                                        );
                                        result.setFromChainType(2);
                                        result.setFromInx(finalCurrentNode.getInx());
                                        result.setToChainType(3);
                                        return result;
                                    }));
                                }
                            }
                        }
                    }
                }
            }
            for (ChainTunnel outNode : outNodes) {
                Node node = nodeService.getById(outNode.getNodeId());
                if (node != null) {
                    final Node finalNode = node;
                    futures.add(CompletableFuture.supplyAsync(() -> {
                        DiagnosisResult result = performTcpPingDiagnosisWithConnectionCheck(
                                finalNode, "www.google.com", 443, "出口(" + finalNode.getName() + ")->外网"
                        );
                        result.setFromChainType(3);
                        return result;
                    }));
                }
            }
        }

        List<DiagnosisResult> results = futures.stream()
                .map(CompletableFuture::join)
                .collect(Collectors.toList());

        Map<String, Object> diagnosisReport = new HashMap<>();
        diagnosisReport.put("tunnelId", tunnelId);
        diagnosisReport.put("tunnelName", tunnel.getName());
        diagnosisReport.put("tunnelType", tunnel.getType() == 1 ? "端口转发" : "隧道转发");
        diagnosisReport.put("results", results);
        diagnosisReport.put("timestamp", System.currentTimeMillis());

        return R.ok(diagnosisReport);
    }

    public Integer getNodePort(Long nodeId) {

        Node node = nodeService.getById(nodeId);
        if (node == null){
            throw new RuntimeException("节点不存在");
        }

        // 1. 查询隧道转发链占用的端口
        List<ChainTunnel> chainTunnels = chainTunnelService.list(
                new QueryWrapper<ChainTunnel>().eq("node_id", nodeId)
        );
        Set<Integer> usedPorts = chainTunnels.stream()
                .map(ChainTunnel::getPort)
                .filter(Objects::nonNull)
                .collect(Collectors.toSet());


        List<ForwardPort> list = forwardPortService.list(new QueryWrapper<ForwardPort>().eq("node_id", nodeId));
        Set<Integer> forwardUsedPorts = new HashSet<>();
        for (ForwardPort forwardPort : list) {
            forwardUsedPorts.add(forwardPort.getPort());
        }
        usedPorts.addAll(forwardUsedPorts);

        // 3. 从可用端口范围中筛选未被占用的端口
        List<Integer> parsedPorts = parsePorts(node.getPort());
        List<Integer> availablePorts = parsedPorts.stream()
                .filter(p -> !usedPorts.contains(p))
                .toList();

        if (availablePorts.isEmpty()) {
            throw new RuntimeException("节点端口已满，无可用端口");
        }
        return availablePorts.getFirst();
    }

    public static List<Integer> parsePorts(String input) {
        Set<Integer> set = new HashSet<>();
        String[] parts = input.split(",");
        for (String part : parts) {
            part = part.trim();
            if (part.contains("-")) {
                String[] range = part.split("-");
                int start = Integer.parseInt(range[0]);
                int end = Integer.parseInt(range[1]);
                for (int i = start; i <= end; i++) {
                    set.add(i);
                }
            } else {
                set.add(Integer.parseInt(part));
            }
        }
        return set.stream().sorted().collect(Collectors.toList());
    }

    private void isError(GostDto gostDto){
        if (gostDto == null) {
            throw new RuntimeException("节点无响应");
        }
        if (!Objects.equals(gostDto.getMsg(), "OK")) {
            throw new RuntimeException(gostDto.getMsg());
        }
    }

    private DiagnosisResult performTcpPingDiagnosis(Node node, String targetIp, int port, String description) {
        try {
            // 构建TCP ping请求数据
            JSONObject tcpPingData = new JSONObject();
            tcpPingData.put("ip", targetIp);
            tcpPingData.put("port", port);
            tcpPingData.put("count", 4);
            tcpPingData.put("timeout", 5000); // 5秒超时

            // 发送TCP ping命令到节点
            GostDto gostResult = WebSocketServer.send_msg(node.getId(), tcpPingData, "TcpPing");

            DiagnosisResult result = new DiagnosisResult();
            result.setNodeId(node.getId());
            result.setNodeName(node.getName());
            result.setTargetIp(targetIp);
            result.setTargetPort(port);
            result.setDescription(description);
            result.setTimestamp(System.currentTimeMillis());

            if (gostResult != null && "OK".equals(gostResult.getMsg())) {
                // 尝试解析TCP ping响应数据
                try {
                    if (gostResult.getData() != null) {
                        JSONObject tcpPingResponse = (JSONObject) gostResult.getData();
                        boolean success = tcpPingResponse.getBooleanValue("success");

                        result.setSuccess(success);
                        if (success) {
                            result.setMessage("TCP连接成功");
                            result.setAverageTime(tcpPingResponse.getDoubleValue("averageTime"));
                            result.setPacketLoss(tcpPingResponse.getDoubleValue("packetLoss"));
                        } else {
                            result.setMessage(tcpPingResponse.getString("errorMessage"));
                            result.setAverageTime(-1.0);
                            result.setPacketLoss(100.0);
                        }
                    } else {
                        // 没有详细数据，使用默认值
                        result.setSuccess(true);
                        result.setMessage("TCP连接成功");
                        result.setAverageTime(0.0);
                        result.setPacketLoss(0.0);
                    }
                } catch (Exception e) {
                    // 解析响应数据失败，但TCP ping命令本身成功了
                    result.setSuccess(true);
                    result.setMessage("TCP连接成功，但无法解析详细数据");
                    result.setAverageTime(0.0);
                    result.setPacketLoss(0.0);
                }
            } else {
                result.setSuccess(false);
                result.setMessage(gostResult != null ? gostResult.getMsg() : "节点无响应");
                result.setAverageTime(-1.0);
                result.setPacketLoss(100.0);
            }

            return result;
        } catch (Exception e) {
            DiagnosisResult result = new DiagnosisResult();
            result.setNodeId(node.getId());
            result.setNodeName(node.getName());
            result.setTargetIp(targetIp);
            result.setTargetPort(port);
            result.setDescription(description);
            result.setSuccess(false);
            result.setMessage("诊断执行异常: " + e.getMessage());
            result.setTimestamp(System.currentTimeMillis());
            result.setAverageTime(-1.0);
            result.setPacketLoss(100.0);
            return result;
        }
    }

    private DiagnosisResult performTcpPingDiagnosisWithConnectionCheck(Node node, String targetIp, int port, String description) {
        DiagnosisResult result = new DiagnosisResult();
        result.setNodeId(node.getId());
        result.setNodeName(node.getName());
        result.setTargetIp(targetIp);
        result.setTargetPort(port);
        result.setDescription(description);
        result.setTimestamp(System.currentTimeMillis());

        try {
            return performTcpPingDiagnosis(node, targetIp, port, description);
        } catch (Exception e) {
            result.setSuccess(false);
            result.setMessage("连接检查异常: " + e.getMessage());
            result.setAverageTime(-1.0);
            result.setPacketLoss(100.0);
            return result;
        }
    }

    private boolean detectNodeChanges(List<ChainTunnel> oldChains, TunnelUpdateDto dto) {
        if (dto.getInNodeId() == null) {
            return false;
        }
        
        List<ChainTunnel> oldInNodes = oldChains.stream()
                .filter(ct -> ct.getChainType() != null && ct.getChainType() == 1)
                .collect(Collectors.toList());
        List<ChainTunnel> oldChainNodes = oldChains.stream()
                .filter(ct -> ct.getChainType() != null && ct.getChainType() == 2)
                .collect(Collectors.toList());
        List<ChainTunnel> oldOutNodes = oldChains.stream()
                .filter(ct -> ct.getChainType() != null && ct.getChainType() == 3)
                .collect(Collectors.toList());

        Set<Long> oldInNodeIds = oldInNodes.stream().map(ChainTunnel::getNodeId).collect(Collectors.toSet());
        Set<Long> newInNodeIds = dto.getInNodeId().stream().map(ChainTunnel::getNodeId).collect(Collectors.toSet());
        if (!oldInNodeIds.equals(newInNodeIds)) {
            return true;
        }

        List<ChainTunnel> flatNewChainNodes = (dto.getChainNodes() == null) ? new ArrayList<>() :
                dto.getChainNodes().stream().flatMap(List::stream).collect(Collectors.toList());
        if (oldChainNodes.size() != flatNewChainNodes.size()) {
            return true;
        }
        for (int i = 0; i < oldChainNodes.size(); i++) {
            ChainTunnel oldCt = oldChainNodes.get(i);
            boolean found = flatNewChainNodes.stream().anyMatch(newCt ->
                    Objects.equals(oldCt.getNodeId(), newCt.getNodeId()) &&
                    Objects.equals(oldCt.getProtocol(), newCt.getProtocol()) &&
                    Objects.equals(oldCt.getStrategy(), newCt.getStrategy()) &&
                    Objects.equals(oldCt.getInx(), newCt.getInx())
            );
            if (!found) {
                return true;
            }
        }

        List<ChainTunnel> newOutNodes = (dto.getOutNodeId() == null) ? new ArrayList<>() : dto.getOutNodeId();
        if (oldOutNodes.size() != newOutNodes.size()) {
            return true;
        }
        Set<Long> oldOutNodeIds = oldOutNodes.stream().map(ChainTunnel::getNodeId).collect(Collectors.toSet());
        Set<Long> newOutNodeIds = newOutNodes.stream().map(ChainTunnel::getNodeId).collect(Collectors.toSet());
        if (!oldOutNodeIds.equals(newOutNodeIds)) {
            return true;
        }
        for (ChainTunnel oldOut : oldOutNodes) {
            boolean found = newOutNodes.stream().anyMatch(newOut ->
                    Objects.equals(oldOut.getNodeId(), newOut.getNodeId()) &&
                    Objects.equals(oldOut.getProtocol(), newOut.getProtocol()) &&
                    Objects.equals(oldOut.getStrategy(), newOut.getStrategy())
            );
            if (!found) {
                return true;
            }
        }

        return false;
    }

    private void cleanupGostConfig(List<ChainTunnel> chainTunnels, Long tunnelId) {
        for (ChainTunnel chainTunnel : chainTunnels) {
            if (chainTunnel.getChainType() == 1) {
                GostUtil.DeleteChains(chainTunnel.getNodeId(), "chains_" + tunnelId);
            } else if (chainTunnel.getChainType() == 2) {
                GostUtil.DeleteChains(chainTunnel.getNodeId(), "chains_" + tunnelId);
                JSONArray services = new JSONArray();
                services.add(tunnelId + "_tls");
                GostUtil.DeleteService(chainTunnel.getNodeId(), services);
            } else if (chainTunnel.getChainType() == 3) {
                JSONArray services = new JSONArray();
                services.add(tunnelId + "_tls");
                GostUtil.DeleteService(chainTunnel.getNodeId(), services);
            }
        }
    }

    private List<ChainTunnel> deepCopyChainTunnels(List<ChainTunnel> original) {
        List<ChainTunnel> copy = new ArrayList<>();
        for (ChainTunnel ct : original) {
            ChainTunnel newCt = new ChainTunnel();
            newCt.setId(ct.getId());
            newCt.setTunnelId(ct.getTunnelId());
            newCt.setChainType(ct.getChainType());
            newCt.setNodeId(ct.getNodeId());
            newCt.setPort(ct.getPort());
            newCt.setStrategy(ct.getStrategy());
            newCt.setInx(ct.getInx());
            newCt.setProtocol(ct.getProtocol());
            copy.add(newCt);
        }
        return copy;
    }

    private R applyNewGostConfig(TunnelUpdateDto dto, Tunnel tunnel, Map<Long, Node> nodes) {
        List<JSONObject> chainSuccess = new ArrayList<>();
        List<JSONObject> serviceSuccess = new ArrayList<>();

        if (tunnel.getType() == 2) {
            List<List<ChainTunnel>> chainNodes = dto.getChainNodes() == null ? new ArrayList<>() : dto.getChainNodes();

            for (ChainTunnel inNode : dto.getInNodeId()) {
                GostDto gostDto;
                if (chainNodes.isEmpty()) {
                    gostDto = GostUtil.AddChains(inNode.getNodeId(), dto.getOutNodeId(), nodes);
                } else {
                    gostDto = GostUtil.AddChains(inNode.getNodeId(), chainNodes.get(0), nodes);
                }
                if (!Objects.equals(gostDto.getMsg(), "OK")) {
                    rollbackGostChanges(chainSuccess, serviceSuccess);
                    return R.err("创建入口Chain失败: " + gostDto.getMsg());
                }
                JSONObject data = new JSONObject();
                data.put("node_id", inNode.getNodeId());
                data.put("name", "chains_" + tunnel.getId());
                chainSuccess.add(data);
            }

            for (int i = 0; i < chainNodes.size(); i++) {
                List<ChainTunnel> currentHop = chainNodes.get(i);
                for (ChainTunnel chainTunnel : currentHop) {
                    GostDto gostDto;
                    if (i + 1 >= chainNodes.size()) {
                        gostDto = GostUtil.AddChains(chainTunnel.getNodeId(), dto.getOutNodeId(), nodes);
                    } else {
                        gostDto = GostUtil.AddChains(chainTunnel.getNodeId(), chainNodes.get(i + 1), nodes);
                    }
                    if (!Objects.equals(gostDto.getMsg(), "OK")) {
                        rollbackGostChanges(chainSuccess, serviceSuccess);
                        return R.err("创建转发链Chain失败: " + gostDto.getMsg());
                    }
                    JSONObject chainData = new JSONObject();
                    chainData.put("node_id", chainTunnel.getNodeId());
                    chainData.put("name", "chains_" + tunnel.getId());
                    chainSuccess.add(chainData);

                    GostDto serviceResult = GostUtil.AddChainService(chainTunnel.getNodeId(), chainTunnel, nodes);
                    if (!Objects.equals(serviceResult.getMsg(), "OK")) {
                        rollbackGostChanges(chainSuccess, serviceSuccess);
                        return R.err("创建转发链Service失败: " + serviceResult.getMsg());
                    }
                    JSONObject serviceData = new JSONObject();
                    serviceData.put("node_id", chainTunnel.getNodeId());
                    serviceData.put("name", tunnel.getId() + "_tls");
                    serviceSuccess.add(serviceData);
                }
            }

            for (ChainTunnel outNode : dto.getOutNodeId()) {
                GostDto gostDto = GostUtil.AddChainService(outNode.getNodeId(), outNode, nodes);
                if (!Objects.equals(gostDto.getMsg(), "OK")) {
                    rollbackGostChanges(chainSuccess, serviceSuccess);
                    return R.err("创建出口Service失败: " + gostDto.getMsg());
                }
                JSONObject serviceData = new JSONObject();
                serviceData.put("node_id", outNode.getNodeId());
                serviceData.put("name", tunnel.getId() + "_tls");
                serviceSuccess.add(serviceData);
            }
        }

        return R.ok();
    }

    private void rollbackGostChanges(List<JSONObject> chainSuccess, List<JSONObject> serviceSuccess) {
        for (JSONObject chain : chainSuccess) {
            GostUtil.DeleteChains(chain.getLong("node_id"), chain.getString("name"));
        }
        for (JSONObject service : serviceSuccess) {
            JSONArray services = new JSONArray();
            services.add(service.getString("name"));
            GostUtil.DeleteService(service.getLong("node_id"), services);
        }
    }

    private void rebuildGostConfig(List<ChainTunnel> chainTunnels, Tunnel tunnel) {
        Map<Long, Node> nodes = new HashMap<>();
        for (ChainTunnel ct : chainTunnels) {
            Node node = nodeService.getById(ct.getNodeId());
            if (node != null) {
                nodes.put(node.getId(), node);
            }
        }

        List<ChainTunnel> inNodes = chainTunnels.stream()
                .filter(ct -> ct.getChainType() == 1)
                .collect(Collectors.toList());
        Map<Integer, List<ChainTunnel>> chainNodesMap = chainTunnels.stream()
                .filter(ct -> ct.getChainType() == 2)
                .collect(Collectors.groupingBy(ct -> ct.getInx() != null ? ct.getInx() : 0));
        List<List<ChainTunnel>> chainNodesList = chainNodesMap.entrySet().stream()
                .sorted(Map.Entry.comparingByKey())
                .map(Map.Entry::getValue)
                .collect(Collectors.toList());
        List<ChainTunnel> outNodes = chainTunnels.stream()
                .filter(ct -> ct.getChainType() == 3)
                .collect(Collectors.toList());

        if (tunnel.getType() == 2) {
            for (ChainTunnel inNode : inNodes) {
                if (chainNodesList.isEmpty()) {
                    GostUtil.AddChains(inNode.getNodeId(), outNodes, nodes);
                } else {
                    GostUtil.AddChains(inNode.getNodeId(), chainNodesList.get(0), nodes);
                }
            }

            for (int i = 0; i < chainNodesList.size(); i++) {
                for (ChainTunnel chainTunnel : chainNodesList.get(i)) {
                    if (i + 1 >= chainNodesList.size()) {
                        GostUtil.AddChains(chainTunnel.getNodeId(), outNodes, nodes);
                    } else {
                        GostUtil.AddChains(chainTunnel.getNodeId(), chainNodesList.get(i + 1), nodes);
                    }
                    GostUtil.AddChainService(chainTunnel.getNodeId(), chainTunnel, nodes);
                }
            }

            for (ChainTunnel outNode : outNodes) {
                GostUtil.AddChainService(outNode.getNodeId(), outNode, nodes);
            }
        }
    }

    private void syncForwardsForEntryNodeChanges(Long tunnelId, Set<Long> oldEntryNodeIds, Set<Long> newEntryNodeIds) {
        Set<Long> addedNodeIds = new HashSet<>(newEntryNodeIds);
        addedNodeIds.removeAll(oldEntryNodeIds);

        Set<Long> removedNodeIds = new HashSet<>(oldEntryNodeIds);
        removedNodeIds.removeAll(newEntryNodeIds);

        if (addedNodeIds.isEmpty() && removedNodeIds.isEmpty()) {
            return;
        }

        List<Forward> forwards = forwardService.list(
                new QueryWrapper<Forward>().eq("tunnel_id", tunnelId.intValue())
        );

        if (forwards.isEmpty()) {
            return;
        }

        Tunnel tunnel = this.getById(tunnelId);
        if (tunnel == null) {
            return;
        }

        for (Forward forward : forwards) {
            if (forward.getStatus() != 1) {
                continue;
            }

            UserTunnel userTunnel = userTunnelService.getOne(
                    new QueryWrapper<UserTunnel>()
                            .eq("user_id", forward.getUserId())
                            .eq("tunnel_id", tunnelId.intValue())
            );

            for (Long removedNodeId : removedNodeIds) {
                ForwardPort forwardPort = forwardPortService.getOne(
                        new QueryWrapper<ForwardPort>()
                                .eq("forward_id", forward.getId())
                                .eq("node_id", removedNodeId)
                );

                if (forwardPort != null) {
                    String serviceName = buildForwardServiceName(forward.getId(), forward.getUserId(), userTunnel);
                    JSONArray services = new JSONArray();
                    services.add(serviceName + "_tcp");
                    services.add(serviceName + "_udp");
                    GostUtil.DeleteService(removedNodeId, services);

                    forwardPortService.removeById(forwardPort.getId());
                }
            }

            for (Long addedNodeId : addedNodeIds) {
                ForwardPort existingPort = forwardPortService.getOne(
                        new QueryWrapper<ForwardPort>()
                                .eq("forward_id", forward.getId())
                                .eq("node_id", addedNodeId)
                );

                if (existingPort != null) {
                    continue;
                }

                List<ForwardPort> existingPorts = forwardPortService.list(
                        new QueryWrapper<ForwardPort>().eq("forward_id", forward.getId())
                );

                Integer targetPort = null;
                if (!existingPorts.isEmpty()) {
                    targetPort = existingPorts.get(0).getPort();
                }

                Integer allocatedPort = allocatePortForNode(addedNodeId, targetPort, forward.getId());
                if (allocatedPort == null) {
                    System.err.println("Failed to allocate port on node " + addedNodeId + " for forward " + forward.getId());
                    continue;
                }

                ForwardPort newForwardPort = new ForwardPort();
                newForwardPort.setForwardId(forward.getId());
                newForwardPort.setNodeId(addedNodeId);
                newForwardPort.setPort(allocatedPort);
                forwardPortService.save(newForwardPort);

                Node node = nodeService.getById(addedNodeId);
                if (node != null) {
                    String serviceName = buildForwardServiceName(forward.getId(), forward.getUserId(), userTunnel);
                    Integer limiter = (userTunnel != null && userTunnel.getSpeedId() != null) ? userTunnel.getSpeedId() : null;
                    GostUtil.AddAndUpdateService(serviceName, limiter, node, forward, newForwardPort, tunnel, "AddService");
                }
            }
        }
    }

    private Integer allocatePortForNode(Long nodeId, Integer preferredPort, Long forwardId) {
        Node node = nodeService.getById(nodeId);
        if (node == null || node.getPort() == null) {
            return null;
        }

        Set<Integer> usedPorts = new HashSet<>();

        List<ChainTunnel> chainTunnels = chainTunnelService.list(
                new QueryWrapper<ChainTunnel>().eq("node_id", nodeId)
        );
        for (ChainTunnel ct : chainTunnels) {
            if (ct.getPort() != null) {
                usedPorts.add(ct.getPort());
            }
        }

        List<ForwardPort> forwardPorts = forwardPortService.list(
                new QueryWrapper<ForwardPort>()
                        .eq("node_id", nodeId)
                        .ne("forward_id", forwardId)
        );
        for (ForwardPort fp : forwardPorts) {
            if (fp.getPort() != null) {
                usedPorts.add(fp.getPort());
            }
        }

        List<Integer> availablePorts = parsePorts(node.getPort());

        if (preferredPort != null && availablePorts.contains(preferredPort) && !usedPorts.contains(preferredPort)) {
            return preferredPort;
        }

        for (Integer port : availablePorts) {
            if (!usedPorts.contains(port)) {
                return port;
            }
        }

        return null;
    }

    private String buildForwardServiceName(Long forwardId, Integer userId, UserTunnel userTunnel) {
        int userTunnelId = (userTunnel != null) ? userTunnel.getId() : 0;
        return forwardId + "_" + userId + "_" + userTunnelId;
    }

    @Override
    @Transactional
    public R batchDeleteTunnels(BatchDeleteDto batchDeleteDto) {
        BatchOperationResultDto result = new BatchOperationResultDto();
        
        for (Long id : batchDeleteDto.getIds()) {
            try {
                Tunnel tunnel = this.getById(id);
                if (tunnel == null) {
                    result.addFailedItem(id, "隧道不存在");
                    continue;
                }
                
                List<Forward> forwardList = forwardService.list(new QueryWrapper<Forward>().eq("tunnel_id", id));
                for (Forward forward : forwardList) {
                    forwardService.deleteForward(forward.getId());
                }
                forwardService.remove(new QueryWrapper<Forward>().eq("tunnel_id", id));
                userTunnelService.remove(new QueryWrapper<UserTunnel>().eq("tunnel_id", id));
                this.removeById(id);
                
                List<ChainTunnel> chainTunnels = chainTunnelService.list(new QueryWrapper<ChainTunnel>().eq("tunnel_id", id));
                for (ChainTunnel chainTunnel : chainTunnels) {
                    if (chainTunnel.getChainType() == 1) {
                        GostUtil.DeleteChains(chainTunnel.getNodeId(), "chains_" + chainTunnel.getTunnelId());
                    } else if (chainTunnel.getChainType() == 2) {
                        GostUtil.DeleteChains(chainTunnel.getNodeId(), "chains_" + chainTunnel.getTunnelId());
                        JSONArray services = new JSONArray();
                        services.add(chainTunnel.getTunnelId() + "_tls");
                        GostUtil.DeleteService(chainTunnel.getNodeId(), services);
                    } else {
                        JSONArray services = new JSONArray();
                        services.add(chainTunnel.getTunnelId() + "_tls");
                        GostUtil.DeleteService(chainTunnel.getNodeId(), services);
                    }
                }
                chainTunnelService.remove(new QueryWrapper<ChainTunnel>().eq("tunnel_id", id));
                
                result.incrementSuccess();
            } catch (Exception e) {
                result.addFailedItem(id, e.getMessage());
            }
        }
        
        return R.ok(result);
    }

    @Override
    @Transactional
    public R batchRedeployTunnels(BatchRedeployDto batchRedeployDto) {
        BatchOperationResultDto result = new BatchOperationResultDto();
        
        for (Long id : batchRedeployDto.getIds()) {
            try {
                Tunnel tunnel = this.getById(id);
                if (tunnel == null) {
                    result.addFailedItem(id, "隧道不存在");
                    continue;
                }
                
                if (tunnel.getType() != 2) {
                    result.incrementSuccess();
                    continue;
                }
                
                List<ChainTunnel> chainTunnels = chainTunnelService.list(
                    new QueryWrapper<ChainTunnel>().eq("tunnel_id", id)
                );
                
                if (chainTunnels.isEmpty()) {
                    result.addFailedItem(id, "隧道配置不完整");
                    continue;
                }
                
                cleanupGostConfig(chainTunnels, id);
                rebuildGostConfig(chainTunnels, tunnel);
                
                result.incrementSuccess();
            } catch (Exception e) {
                result.addFailedItem(id, e.getMessage());
            }
        }
        
        return R.ok(result);
    }


}
