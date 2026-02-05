package com.admin.service.impl;

import com.admin.common.dto.*;
import com.admin.common.lang.R;
import com.admin.common.utils.GostUtil;
import com.admin.common.utils.JwtUtil;
import com.admin.common.utils.WebSocketServer;
import com.admin.entity.*;
import com.admin.mapper.ForwardMapper;
import com.admin.service.*;
import com.alibaba.fastjson.JSONArray;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import com.baomidou.mybatisplus.extension.service.impl.ServiceImpl;
import com.alibaba.fastjson.JSONObject;
import lombok.Data;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.BeanUtils;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.context.annotation.Lazy;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import javax.annotation.Resource;
import java.util.*;
import java.util.concurrent.CompletableFuture;
import java.util.stream.Collectors;

/**
 * <p>
 * 端口转发服务实现类
 * </p>
 *
 * @author QAQ
 * @since 2025-06-03
 */
@Slf4j
@Service
public class ForwardServiceImpl extends ServiceImpl<ForwardMapper, Forward> implements ForwardService {

    private static final long BYTES_TO_GB = 1024L * 1024L * 1024L;

    @Resource
    @Lazy
    private TunnelService tunnelService;

    @Resource
    UserTunnelService userTunnelService;

    @Resource
    UserService userService;

    @Resource
    NodeService nodeService;

    @Resource
    ChainTunnelService chainTunnelService;

    @Resource
    ForwardPortService forwardPortService;

    @Override
    public R getAllForwards() {
        UserInfo currentUser = getCurrentUserInfo();
        List<ForwardWithTunnelDto> forwardList;
        if (currentUser.getRoleId() != 0) {
            forwardList = baseMapper.selectForwardsWithTunnelByUserId(currentUser.getUserId());
        } else {
            forwardList = baseMapper.selectAllForwardsWithTunnel();
        }

        // 填充入口IP和端口信息
        for (ForwardWithTunnelDto forward : forwardList) {
            // 获取隧道信息
            Tunnel tunnel = tunnelService.getById(forward.getTunnelId());
            if (tunnel == null) continue;

            // 获取该转发的所有ForwardPort记录
            List<ForwardPort> forwardPorts = forwardPortService.list(
                    new QueryWrapper<ForwardPort>().eq("forward_id", forward.getId())
            );

            if (forwardPorts.isEmpty()) continue;

            // 判断是否使用隧道的inIp
            boolean useTunnelInIp = tunnel.getInIp() != null && !tunnel.getInIp().trim().isEmpty();

            Set<String> ipPortSet = new LinkedHashSet<>();

            if (useTunnelInIp) {
                // 使用隧道的inIp（求笛卡尔积）
                List<String> ipList = new ArrayList<>();
                List<Integer> portList = new ArrayList<>();
                
                String[] tunnelInIps = tunnel.getInIp().split(",");
                for (String ip : tunnelInIps) {
                    if (ip != null && !ip.trim().isEmpty()) {
                        ipList.add(ip.trim());
                    }
                }
                
                // 收集所有端口
                for (ForwardPort forwardPort : forwardPorts) {
                    if (forwardPort.getPort() != null) {
                        portList.add(forwardPort.getPort());
                    }
                }
                
                // 去重
                List<String> uniqueIps = ipList.stream().distinct().toList();
                List<Integer> uniquePorts = portList.stream().distinct().toList();
                
                // 组合 IP:Port（笛卡尔积）
                for (String ip : uniqueIps) {
                    for (Integer port : uniquePorts) {
                        ipPortSet.add(ip + ":" + port);
                    }
                }
                
                // inPort设置为第一个端口（用于向后兼容）
                if (!uniquePorts.isEmpty()) {
                    forward.setInPort(uniquePorts.getFirst());
                }
            } else {
                // 使用节点的serverIp（一对一，不求笛卡尔积）
                for (ForwardPort forwardPort : forwardPorts) {
                    Node node = nodeService.getById(forwardPort.getNodeId());
                    if (node != null && node.getServerIp() != null && forwardPort.getPort() != null) {
                        ipPortSet.add(node.getServerIp() + ":" + forwardPort.getPort());
                    }
                }
                
                // inPort设置为第一个端口（用于向后兼容）
                if (!forwardPorts.isEmpty() && forwardPorts.getFirst().getPort() != null) {
                    forward.setInPort(forwardPorts.getFirst().getPort());
                }
            }

            // 设置入口IP
            if (!ipPortSet.isEmpty()) {
                forward.setInIp(String.join(",", ipPortSet));
            }
        }

        return R.ok(forwardList);
    }

    @Override
    public R createForward(ForwardDto forwardDto) {
        UserInfo currentUser = getCurrentUserInfo();

        Tunnel tunnel = validateTunnel(forwardDto.getTunnelId());
        if (tunnel == null) {
            return R.err("隧道不存在");
        }

        if (tunnel.getStatus() != 1) {
            return R.err("隧道已禁用，无法创建转发");
        }

        UserPermissionResult permissionResult = checkUserPermissions(currentUser, tunnel, null);
        if (permissionResult.isHasError()) {
            return R.err(permissionResult.getErrorMessage());
        }
        Forward forward = new Forward();
        BeanUtils.copyProperties(forwardDto, forward);
        forward.setStatus(1);
        forward.setUserId(currentUser.getUserId());
        forward.setUserName(currentUser.getUserName());
        forward.setCreatedTime(System.currentTimeMillis());
        forward.setUpdatedTime(System.currentTimeMillis());
        List<JSONObject> success = new ArrayList<>();
        List<ChainTunnel> chainTunnels = chainTunnelService.list(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId()).eq("chain_type", 1));
        chainTunnels = get_port(chainTunnels, forwardDto.getInPort(), 0L);
        this.save(forward);

        for (ChainTunnel chainTunnel : chainTunnels) {

            ForwardPort forwardPort = new ForwardPort();
            forwardPort.setForwardId(forward.getId());
            forwardPort.setNodeId(chainTunnel.getNodeId());
            forwardPort.setPort(chainTunnel.getPort());
            forwardPortService.save(forwardPort);
            String serviceName = buildServiceName(forward.getId(), forward.getUserId(), permissionResult.getUserTunnel());
            Integer limiter = permissionResult.getLimiter();

            Node node = nodeService.getById(chainTunnel.getNodeId());
            if (node == null) {
                return R.err("部分节点不存在");
            }
            GostDto gostDto = GostUtil.AddAndUpdateService(serviceName, limiter, node, forward, forwardPort, tunnel, "AddService");
            if (Objects.equals(gostDto.getMsg(), "OK")) {
                JSONObject data = new JSONObject();
                data.put("node_id", node.getId());
                data.put("name", serviceName);
                success.add(data);
            } else {
                this.removeById(forward.getId());
                forwardPortService.remove(new QueryWrapper<ForwardPort>().eq("forward_id", forward.getId()));
                for (JSONObject jsonObject : success) {
                    JSONArray se = new JSONArray();
                    se.add(jsonObject.getString("name") + "_tcp");
                    se.add(jsonObject.getString("name") + "_udp");
                    GostUtil.DeleteService(jsonObject.getLong("node_id"), se);
                    return R.err(gostDto.getMsg());
                }
            }

        }
        return R.ok();
    }

    @Override
    @Transactional
    public R updateForward(ForwardUpdateDto forwardUpdateDto) {
        UserInfo currentUser = getCurrentUserInfo();

        Forward existForward = validateForwardExists(forwardUpdateDto.getId(), currentUser);
        if (existForward == null) {
            return R.err("转发不存在");
        }

        Integer oldTunnelId = existForward.getTunnelId();
        Integer newTunnelId = forwardUpdateDto.getTunnelId() != null ? forwardUpdateDto.getTunnelId() : oldTunnelId;
        boolean tunnelChanged = !Objects.equals(oldTunnelId, newTunnelId);

        Tunnel oldTunnel = validateTunnel(oldTunnelId);
        if (oldTunnel == null) {
            return R.err("原隧道不存在");
        }

        UserTunnel oldUserTunnel;
        if (currentUser.getRoleId() != 0) {
            oldUserTunnel = getUserTunnel(currentUser.getUserId(), oldTunnelId);
            if (oldUserTunnel == null) {
                return R.err("你没有原隧道权限");
            }
        } else {
            oldUserTunnel = getUserTunnel(existForward.getUserId(), oldTunnelId);
        }

        if (tunnelChanged) {
            Tunnel newTunnel = validateTunnel(newTunnelId);
            if (newTunnel == null) {
                return R.err("新隧道不存在");
            }
            if (newTunnel.getStatus() != 1) {
                return R.err("新隧道已禁用，无法切换");
            }

            UserPermissionResult newPermResult = checkUserPermissions(currentUser, newTunnel, null);
            if (newPermResult.isHasError()) {
                return R.err(newPermResult.getErrorMessage());
            }

            UserTunnel newUserTunnel;
            if (currentUser.getRoleId() != 0) {
                newUserTunnel = getUserTunnel(currentUser.getUserId(), newTunnelId);
                if (newUserTunnel == null) {
                    return R.err("你没有新隧道权限");
                }
            } else {
                newUserTunnel = getUserTunnel(existForward.getUserId(), newTunnelId);
            }

            releaseOldTunnelResources(existForward, oldTunnel, oldUserTunnel);

            existForward.setTunnelId(newTunnelId);
            existForward.setRemoteAddr(forwardUpdateDto.getRemoteAddr());
            existForward.setName(forwardUpdateDto.getName());
            existForward.setStrategy(forwardUpdateDto.getStrategy());
            existForward.setStatus(1);
            existForward.setUpdatedTime(System.currentTimeMillis());
            this.updateById(existForward);

            R allocResult = allocateNewTunnelResources(existForward, newTunnel, forwardUpdateDto.getInPort(), newPermResult, newUserTunnel);
            if (allocResult.getCode() != 0) {
                throw new RuntimeException(allocResult.getMsg());
            }

            return R.ok();
        }

        UserPermissionResult permissionResult = checkUserPermissions(currentUser, oldTunnel, null);
        if (permissionResult.isHasError()) {
            return R.err(permissionResult.getErrorMessage());
        }

        existForward.setRemoteAddr(forwardUpdateDto.getRemoteAddr());
        existForward.setName(forwardUpdateDto.getName());
        existForward.setStrategy(forwardUpdateDto.getStrategy());
        existForward.setStatus(1);
        existForward.setUpdatedTime(System.currentTimeMillis());
        this.updateById(existForward);

        List<ChainTunnel> chainTunnels = chainTunnelService.list(new QueryWrapper<ChainTunnel>().eq("tunnel_id", oldTunnel.getId()).eq("chain_type", 1));
        chainTunnels = get_port(chainTunnels, forwardUpdateDto.getInPort(), existForward.getId());

        for (ChainTunnel chainTunnel : chainTunnels) {
            String serviceName = buildServiceName(existForward.getId(), existForward.getUserId(), oldUserTunnel);
            Integer limiter = permissionResult.getLimiter();
            Node node = nodeService.getById(chainTunnel.getNodeId());
            if (node == null) {
                return R.err("部分节点不存在");
            }
            ForwardPort forwardPort = forwardPortService.getOne(new QueryWrapper<ForwardPort>().eq("forward_id", existForward.getId()).eq("node_id", node.getId()));
            if (forwardPort == null) {
                return R.err("部分节点不存在1");
            }
            forwardPort.setPort(chainTunnel.getPort());
            forwardPortService.updateById(forwardPort);
            GostDto gostDto = GostUtil.AddAndUpdateService(serviceName, limiter, node, existForward, forwardPort, oldTunnel, "UpdateService");
            if (!Objects.equals(gostDto.getMsg(), "OK")) return R.err(gostDto.getMsg());
        }

        return R.ok();
    }

    private void releaseOldTunnelResources(Forward forward, Tunnel oldTunnel, UserTunnel oldUserTunnel) {
        List<ChainTunnel> oldInNodes = chainTunnelService.list(
                new QueryWrapper<ChainTunnel>()
                        .eq("tunnel_id", oldTunnel.getId())
                        .eq("chain_type", 1)
        );

        for (ChainTunnel chainTunnel : oldInNodes) {
            String serviceName = buildServiceName(forward.getId(), forward.getUserId(), oldUserTunnel);
            Node node = nodeService.getById(chainTunnel.getNodeId());
            if (node != null) {
                JSONArray services = new JSONArray();
                services.add(serviceName + "_tcp");
                services.add(serviceName + "_udp");
                GostUtil.DeleteService(node.getId(), services);
            }
        }

        forwardPortService.remove(new QueryWrapper<ForwardPort>().eq("forward_id", forward.getId()));
    }

    private R allocateNewTunnelResources(Forward forward, Tunnel newTunnel, Integer requestedPort,
                                         UserPermissionResult permResult, UserTunnel newUserTunnel) {
        List<ChainTunnel> newInNodes = chainTunnelService.list(
                new QueryWrapper<ChainTunnel>()
                        .eq("tunnel_id", newTunnel.getId())
                        .eq("chain_type", 1)
        );

        newInNodes = get_port(newInNodes, requestedPort, forward.getId());

        List<JSONObject> successServices = new ArrayList<>();

        for (ChainTunnel chainTunnel : newInNodes) {
            ForwardPort forwardPort = new ForwardPort();
            forwardPort.setForwardId(forward.getId());
            forwardPort.setNodeId(chainTunnel.getNodeId());
            forwardPort.setPort(chainTunnel.getPort());
            forwardPortService.save(forwardPort);

            String serviceName = buildServiceName(forward.getId(), forward.getUserId(), newUserTunnel);
            Integer limiter = permResult.getLimiter();
            Node node = nodeService.getById(chainTunnel.getNodeId());

            if (node == null) {
                rollbackCreatedServices(successServices);
                forwardPortService.remove(new QueryWrapper<ForwardPort>().eq("forward_id", forward.getId()));
                return R.err("新隧道部分节点不存在");
            }

            GostDto gostDto = GostUtil.AddAndUpdateService(serviceName, limiter, node, forward, forwardPort, newTunnel, "AddService");

            if (!Objects.equals(gostDto.getMsg(), "OK")) {
                rollbackCreatedServices(successServices);
                forwardPortService.remove(new QueryWrapper<ForwardPort>().eq("forward_id", forward.getId()));
                return R.err("在新隧道创建服务失败: " + gostDto.getMsg());
            }

            JSONObject data = new JSONObject();
            data.put("node_id", node.getId());
            data.put("name", serviceName);
            successServices.add(data);
        }

        return R.ok();
    }

    private void rollbackCreatedServices(List<JSONObject> created) {
        for (JSONObject jsonObject : created) {
            JSONArray se = new JSONArray();
            se.add(jsonObject.getString("name") + "_tcp");
            se.add(jsonObject.getString("name") + "_udp");
            GostUtil.DeleteService(jsonObject.getLong("node_id"), se);
        }
    }

    @Override
    public R deleteForward(Long id) {

        // 1. 获取当前用户信息
        UserInfo currentUser = getCurrentUserInfo();


        // 2. 检查转发是否存在
        Forward forward = validateForwardExists(id, currentUser);
        if (forward == null) {
            return R.err("转发不存在");
        }


        Tunnel tunnel = validateTunnel(forward.getTunnelId());
        if (tunnel == null) {
            return R.err("隧道不存在");
        }

        UserPermissionResult permissionResult = checkUserPermissions(currentUser, tunnel, null);
        if (permissionResult.isHasError()) {
            return R.err(permissionResult.getErrorMessage());
        }
        UserTunnel userTunnel = null;
        if (currentUser.getRoleId() != 0) {
            userTunnel = getUserTunnel(currentUser.getUserId(), tunnel.getId().intValue());
            if (userTunnel == null) {
                return R.err("你没有该隧道权限");
            }
        } else {
            // 管理员用户也需要获取UserTunnel（如果存在的话），用于构建正确的服务名称
            // 通过forward记录获取原始的用户ID
            userTunnel = getUserTunnel(forward.getUserId(), tunnel.getId().intValue());
        }

        List<ChainTunnel> chainTunnels = chainTunnelService.list(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId()).eq("chain_type", 1));
        for (ChainTunnel chainTunnel : chainTunnels) {

            String serviceName = buildServiceName(forward.getId(), forward.getUserId(), userTunnel);
            Node node = nodeService.getById(chainTunnel.getNodeId());
            if (node == null) {
                return R.err("部分节点不存在");
            }

            JSONArray services = new JSONArray();
            services.add(serviceName + "_tcp");
            services.add(serviceName + "_udp");
            GostUtil.DeleteService(node.getId(), services);
        }
        forwardPortService.remove(new QueryWrapper<ForwardPort>().eq("forward_id", id));
        this.removeById(id);
        return R.ok();
    }

    @Override
    public R pauseForward(Long id) {
        return changeForwardStatus(id, 0, "PauseService");
    }

    @Override
    public R resumeForward(Long id) {
        return changeForwardStatus(id, 1, "ResumeService");
    }

    @Override
    public R forceDeleteForward(Long id) {
        UserInfo currentUser = getCurrentUserInfo();
        Forward forward = validateForwardExists(id, currentUser);
        if (forward == null) {
            return R.err("端口转发不存在");
        }
        this.removeById(id);
        forwardPortService.remove(new QueryWrapper<ForwardPort>().eq("forward_id", id));
        return R.ok();
    }

    @Override
    public R diagnoseForward(Long id) {
        // 1. 获取当前用户信息
        UserInfo currentUser = getCurrentUserInfo();

        // 2. 检查转发是否存在且用户有权限访问
        Forward forward = validateForwardExists(id, currentUser);
        if (forward == null) {
            return R.err("转发不存在");
        }

        // 3. 获取隧道信息
        Tunnel tunnel = validateTunnel(forward.getTunnelId());
        if (tunnel == null) {
            return R.err("隧道不存在");
        }

        // 4. 获取隧道的ChainTunnel信息
        List<ChainTunnel> chainTunnels = chainTunnelService.list(
                new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId())
        );

        if (chainTunnels.isEmpty()) {
            return R.err("隧道配置不完整");
        }

        // 分类节点
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
        String[] remoteAddresses = forward.getRemoteAddr().split(",");

        // 根据隧道类型执行不同的诊断策略（并行执行所有诊断任务）
        if (tunnel.getType() == 1) {
            // 端口转发：入口节点直接TCP ping目标地址
            for (ChainTunnel inNode : inNodes) {
                Node node = nodeService.getById(inNode.getNodeId());
                if (node != null) {
                    for (String remoteAddress : remoteAddresses) {
                        String targetIp = extractIpFromAddress(remoteAddress);
                        int targetPort = extractPortFromAddress(remoteAddress);
                        if (targetIp != null && targetPort != -1) {
                            final Node finalNode = node;
                            final String finalTargetIp = targetIp;
                            final int finalTargetPort = targetPort;
                            final String finalRemoteAddress = remoteAddress;
                            futures.add(CompletableFuture.supplyAsync(() -> {
                                DiagnosisResult result = performTcpPingDiagnosisWithConnectionCheck(
                                        finalNode, finalTargetIp, finalTargetPort,
                                        "入口(" + finalNode.getName() + ")->目标(" + finalRemoteAddress + ")"
                                );
                                result.setFromChainType(1);
                                return result;
                            }));
                        }
                    }
                }
            }
        } else if (tunnel.getType() == 2) {
            // 隧道转发：测试完整链路
            // 1. 入口->第一跳（或出口）
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

            // 2. 链路测试
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

            // 3. 出口->目标地址
            for (ChainTunnel outNode : outNodes) {
                Node node = nodeService.getById(outNode.getNodeId());
                if (node != null) {
                    for (String remoteAddress : remoteAddresses) {
                        String targetIp = extractIpFromAddress(remoteAddress);
                        int targetPort = extractPortFromAddress(remoteAddress);
                        if (targetIp != null && targetPort != -1) {
                            final Node finalNode = node;
                            final String finalTargetIp = targetIp;
                            final int finalTargetPort = targetPort;
                            final String finalRemoteAddress = remoteAddress;
                            futures.add(CompletableFuture.supplyAsync(() -> {
                                DiagnosisResult result = performTcpPingDiagnosisWithConnectionCheck(
                                        finalNode, finalTargetIp, finalTargetPort,
                                        "出口(" + finalNode.getName() + ")->目标(" + finalRemoteAddress + ")"
                                );
                                result.setFromChainType(3);
                                return result;
                            }));
                        }
                    }
                }
            }
        }

        // 等待所有诊断任务完成并收集结果
        List<DiagnosisResult> results = futures.stream()
                .map(CompletableFuture::join)
                .collect(Collectors.toList());

        // 构建诊断报告
        Map<String, Object> diagnosisReport = new HashMap<>();
        diagnosisReport.put("forwardId", id);
        diagnosisReport.put("forwardName", forward.getName());
        diagnosisReport.put("tunnelType", tunnel.getType() == 1 ? "端口转发" : "隧道转发");
        diagnosisReport.put("results", results);
        diagnosisReport.put("timestamp", System.currentTimeMillis());

        return R.ok(diagnosisReport);
    }

    @Override
    @Transactional
    public R updateForwardOrder(Map<String, Object> params) {
        // 1. 获取当前用户信息
        UserInfo currentUser = getCurrentUserInfo();

        // 2. 验证参数
        if (!params.containsKey("forwards")) {
            return R.err("缺少forwards参数");
        }

        @SuppressWarnings("unchecked")
        List<Map<String, Object>> forwardsList = (List<Map<String, Object>>) params.get("forwards");
        if (forwardsList == null || forwardsList.isEmpty()) {
            return R.err("forwards参数不能为空");
        }

        // 3. 验证用户权限（只能更新自己的转发）
        if (currentUser.getRoleId() != 0) {
            // 普通用户只能更新自己的转发
            List<Long> forwardIds = forwardsList.stream()
                    .map(item -> Long.valueOf(item.get("id").toString()))
                    .collect(Collectors.toList());

            // 检查所有转发是否属于当前用户
            QueryWrapper<Forward> queryWrapper = new QueryWrapper<>();
            queryWrapper.in("id", forwardIds);
            queryWrapper.eq("user_id", currentUser.getUserId());

            long count = this.count(queryWrapper);
            if (count != forwardIds.size()) {
                return R.err("只能更新自己的转发排序");
            }
        }

        // 4. 批量更新排序
        List<Forward> forwardsToUpdate = new ArrayList<>();
        for (Map<String, Object> forwardData : forwardsList) {
            Long id = Long.valueOf(forwardData.get("id").toString());
            Integer inx = Integer.valueOf(forwardData.get("inx").toString());

            Forward forward = new Forward();
            forward.setId(id);
            forward.setInx(inx);
            forwardsToUpdate.add(forward);
        }

        // 5. 执行批量更新
        this.updateBatchById(forwardsToUpdate);
        return R.ok();


    }


    private R changeForwardStatus(Long id, int targetStatus, String gostMethod) {
        UserInfo currentUser = getCurrentUserInfo();
        if (currentUser.getRoleId() != 0) {
            User user = userService.getById(currentUser.getUserId());
            if (user == null) return R.err("用户不存在");
            if (user.getStatus() == 0) return R.err("用户已到期或被禁用");
        }
        Forward forward = validateForwardExists(id, currentUser);
        if (forward == null) {
            return R.err("转发不存在");
        }

        Tunnel tunnel = validateTunnel(forward.getTunnelId());
        if (tunnel == null) {
            return R.err("隧道不存在");
        }

        UserTunnel userTunnel = null;
        if (targetStatus == 1) {
            if (tunnel.getStatus() != 1) {
                return R.err("隧道已禁用，无法恢复服务");
            }
            if (currentUser.getRoleId() != 0) {
                R flowCheckResult = checkUserFlowLimits(currentUser.getUserId(), tunnel);
                if (flowCheckResult.getCode() != 0) {
                    return flowCheckResult;
                }
                userTunnel = getUserTunnel(currentUser.getUserId(), tunnel.getId().intValue());
                if (userTunnel == null) {
                    return R.err("你没有该隧道权限");
                }
                if (userTunnel.getStatus() != 1) {
                    return R.err("隧道被禁用");
                }
            }
        }
        if (currentUser.getRoleId() != 0 && userTunnel == null) {
            userTunnel = getUserTunnel(currentUser.getUserId(), tunnel.getId().intValue());
            if (userTunnel == null) {
                return R.err("你没有该隧道权限");
            }
        }

        if (userTunnel == null) {
            userTunnel = getUserTunnel(forward.getUserId(), tunnel.getId().intValue());
        }

        List<ChainTunnel> chainTunnels = chainTunnelService.list(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId()).eq("chain_type", 1));

        for (ChainTunnel chainTunnel : chainTunnels) {
            String serviceName = buildServiceName(forward.getId(), forward.getUserId(), userTunnel);
            Node node = nodeService.getById(chainTunnel.getNodeId());
            if (node == null) {
                return R.err("部分节点不存在");
            }
            GostDto gostDto = GostUtil.PauseAndResumeService(node.getId(), serviceName, gostMethod);
            if (!Objects.equals(gostDto.getMsg(), "OK")) return R.err(gostDto.getMsg());
        }
        forward.setStatus(targetStatus);
        forward.setUpdatedTime(System.currentTimeMillis());
        this.updateById(forward);
        return R.ok();
    }

    private String extractIpFromAddress(String address) {
        if (address == null || address.trim().isEmpty()) {
            return null;
        }

        address = address.trim();

        // IPv6格式: [ipv6]:port
        if (address.startsWith("[")) {
            int closeBracket = address.indexOf(']');
            if (closeBracket > 1) {
                return address.substring(1, closeBracket);
            }
        }

        // IPv4或域名格式: ip:port 或 domain:port
        int lastColon = address.lastIndexOf(':');
        if (lastColon > 0) {
            return address.substring(0, lastColon);
        }

        // 如果没有端口，直接返回地址
        return address;
    }

    private int extractPortFromAddress(String address) {
        if (address == null || address.trim().isEmpty()) {
            return -1;
        }

        address = address.trim();

        // IPv6格式: [ipv6]:port
        if (address.startsWith("[")) {
            int closeBracket = address.indexOf(']');
            if (closeBracket > 1 && closeBracket + 1 < address.length() && address.charAt(closeBracket + 1) == ':') {
                String portStr = address.substring(closeBracket + 2);
                try {
                    return Integer.parseInt(portStr);
                } catch (NumberFormatException e) {
                    return -1;
                }
            }
        }

        // IPv4或域名格式: ip:port 或 domain:port
        int lastColon = address.lastIndexOf(':');
        if (lastColon > 0 && lastColon + 1 < address.length()) {
            String portStr = address.substring(lastColon + 1);
            try {
                return Integer.parseInt(portStr);
            } catch (NumberFormatException e) {
                return -1;
            }
        }

        // 如果没有端口，返回-1表示无法解析
        return -1;
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

    private UserInfo getCurrentUserInfo() {
        Integer userId = JwtUtil.getUserIdFromToken();
        Integer roleId = JwtUtil.getRoleIdFromToken();
        String userName = JwtUtil.getNameFromToken();
        return new UserInfo(userId, roleId, userName);
    }

    private Tunnel validateTunnel(Integer tunnelId) {
        return tunnelService.getById(tunnelId);
    }

    private Forward validateForwardExists(Long forwardId, UserInfo currentUser) {
        Forward forward = this.getById(forwardId);
        if (forward == null) {
            return null;
        }

        // 普通用户只能操作自己的转发
        if (currentUser.getRoleId() != 0 &&
                !Objects.equals(currentUser.getUserId(), forward.getUserId())) {
            return null;
        }

        return forward;
    }

    private UserPermissionResult checkUserPermissions(UserInfo currentUser, Tunnel tunnel, Long excludeForwardId) {
        if (currentUser.getRoleId() == 0) {
            return UserPermissionResult.success(null, null);
        }

        // 获取用户信息
        User userInfo = userService.getById(currentUser.getUserId());
        if (userInfo.getExpTime() != null && userInfo.getExpTime() <= System.currentTimeMillis()) {
            return UserPermissionResult.error("当前账号已到期");
        }

        // 检查用户隧道权限
        UserTunnel userTunnel = getUserTunnel(currentUser.getUserId(), tunnel.getId().intValue());
        if (userTunnel == null) {
            return UserPermissionResult.error("你没有该隧道权限");
        }

        if (userTunnel.getStatus() != 1) {
            return UserPermissionResult.error("隧道被禁用");
        }

        // 检查隧道权限到期时间
        if (userTunnel.getExpTime() != null && userTunnel.getExpTime() <= System.currentTimeMillis()) {
            return UserPermissionResult.error("该隧道权限已到期");
        }

        // 流量限制检查
        if (userInfo.getFlow() <= 0) {
            return UserPermissionResult.error("用户总流量已用完");
        }
        if (userTunnel.getFlow() <= 0) {
            return UserPermissionResult.error("该隧道流量已用完");
        }

        // 转发数量限制检查
        R quotaCheckResult = checkForwardQuota(currentUser.getUserId(), tunnel.getId().intValue(), userTunnel, userInfo, excludeForwardId);
        if (quotaCheckResult.getCode() != 0) {
            return UserPermissionResult.error(quotaCheckResult.getMsg());
        }

        return UserPermissionResult.success(userTunnel.getSpeedId(), userTunnel);
    }

    private R checkForwardQuota(Integer userId, Integer tunnelId, UserTunnel userTunnel, User userInfo, Long excludeForwardId) {
        // 检查用户总转发数量限制
        long userForwardCount = this.count(new QueryWrapper<Forward>().eq("user_id", userId));
        if (userForwardCount >= userInfo.getNum()) {
            return R.err("用户总转发数量已达上限，当前限制：" + userInfo.getNum() + "个");
        }

        // 检查用户在该隧道的转发数量限制
        QueryWrapper<Forward> tunnelQuery = new QueryWrapper<Forward>()
                .eq("user_id", userId)
                .eq("tunnel_id", tunnelId);

        if (excludeForwardId != null) {
            tunnelQuery.ne("id", excludeForwardId);
        }

        long tunnelForwardCount = this.count(tunnelQuery);
        if (tunnelForwardCount >= userTunnel.getNum()) {
            return R.err("该隧道转发数量已达上限，当前限制：" + userTunnel.getNum() + "个");
        }

        return R.ok();
    }

    private R checkUserFlowLimits(Integer userId, Tunnel tunnel) {
        User userInfo = userService.getById(userId);
        if (userInfo.getExpTime() != null && userInfo.getExpTime() <= System.currentTimeMillis()) {
            return R.err("当前账号已到期");
        }

        UserTunnel userTunnel = getUserTunnel(userId, tunnel.getId().intValue());
        if (userTunnel == null) {
            return R.err("你没有该隧道权限");
        }

        // 检查隧道权限到期时间
        if (userTunnel.getExpTime() != null && userTunnel.getExpTime() <= System.currentTimeMillis()) {
            return R.err("该隧道权限已到期，无法恢复服务");
        }

        // 检查用户总流量限制
        if (userInfo.getFlow() * BYTES_TO_GB <= userInfo.getInFlow() + userInfo.getOutFlow()) {
            return R.err("用户总流量已用完，无法恢复服务");
        }

        // 检查隧道流量限制
        // 数据库中的流量已按计费类型处理，直接使用总和
        long tunnelFlow = userTunnel.getInFlow() + userTunnel.getOutFlow();

        if (userTunnel.getFlow() * BYTES_TO_GB <= tunnelFlow) {
            return R.err("该隧道流量已用完，无法恢复服务");
        }

        return R.ok();
    }

    private UserTunnel getUserTunnel(Integer userId, Integer tunnelId) {
        return userTunnelService.getOne(new QueryWrapper<UserTunnel>()
                .eq("user_id", userId)
                .eq("tunnel_id", tunnelId));
    }

    private String buildServiceName(Long forwardId, Integer userId, UserTunnel userTunnel) {
        int userTunnelId = (userTunnel != null) ? userTunnel.getId() : 0;
        return forwardId + "_" + userId + "_" + userTunnelId;
    }


    public List<ChainTunnel> get_port(List<ChainTunnel> chainTunnelList, Integer in_port, Long forward_id) {
        List<List<Integer>> list = new ArrayList<>();

        // 获取每个节点的端口列表
        for (ChainTunnel tunnel : chainTunnelList) {
            List<Integer> nodePort = getNodePort(tunnel.getNodeId(), forward_id);
            if (nodePort.isEmpty()) {
                throw new RuntimeException("暂无可用端口");
            }
            list.add(nodePort);
        }

        // ========== 如果指定了 in_port，优先检查公有 ==========
        if (in_port != null) {
            for (List<Integer> ports : list) {
                if (!ports.contains(in_port)) {
                    throw new RuntimeException("指定端口 " + in_port + " 不可用（并非所有节点都有此端口）");
                }
            }

            // 所有节点都有该端口 设置回 ChainTunnel
            for (ChainTunnel tunnel : chainTunnelList) {
                tunnel.setPort(in_port);
            }
            return chainTunnelList;
        }

        // ========== 未指定 in_port 查找最小的共同端口 ==========
        Set<Integer> intersection = new HashSet<>(list.getFirst());
        for (int i = 1; i < list.size(); i++) {
            intersection.retainAll(list.get(i));
        }

        if (!intersection.isEmpty()) {
            // 找最小端口
            Integer commonMin = intersection.stream().min(Integer::compareTo).orElseThrow();

            // 设置到所有节点
            for (ChainTunnel tunnel : chainTunnelList) {
                tunnel.setPort(commonMin);
            }

            return chainTunnelList;
        }

        // ========== 没有共同端口取各自第一个可用端口 ==========
        for (int i = 0; i < chainTunnelList.size(); i++) {
            List<Integer> ports = list.get(i);
            Integer first = ports.getFirst();
            chainTunnelList.get(i).setPort(first);
        }

        return chainTunnelList;
    }

    public List<Integer> getNodePort(Long nodeId, Long forward_id) {

        Node node = nodeService.getById(nodeId);
        if (node == null) {
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


        List<ForwardPort> list = forwardPortService.list(new QueryWrapper<ForwardPort>().eq("node_id", nodeId).ne("forward_id", forward_id));
        Set<Integer> forwardUsedPorts = new HashSet<>();
        for (ForwardPort forwardPort : list) {
            forwardUsedPorts.add(forwardPort.getPort());
        }
        usedPorts.addAll(forwardUsedPorts);

        List<Integer> parsedPorts = TunnelServiceImpl.parsePorts(node.getPort());
        return parsedPorts.stream()
                .filter(p -> !usedPorts.contains(p))
                .toList();
    }



    // ========== 内部数据类 ==========

    @Data
    private static class UserInfo {
        private final Integer userId;
        private final Integer roleId;
        private final String userName;

        public UserInfo(Integer userId, Integer roleId, String userName) {
            this.userId = userId;
            this.roleId = roleId;
            this.userName = userName;
        }
    }

    @Data
    private static class UserPermissionResult {
        private final boolean hasError;
        private final String errorMessage;
        private final Integer limiter;
        private final UserTunnel userTunnel;

        private UserPermissionResult(boolean hasError, String errorMessage, Integer limiter, UserTunnel userTunnel) {
            this.hasError = hasError;
            this.errorMessage = errorMessage;
            this.limiter = limiter;
            this.userTunnel = userTunnel;
        }

        public static UserPermissionResult success(Integer limiter, UserTunnel userTunnel) {
            return new UserPermissionResult(false, null, limiter, userTunnel);
        }

        public static UserPermissionResult error(String errorMessage) {
            return new UserPermissionResult(true, errorMessage, null, null);
        }
    }

    @Data
    public static class DiagnosisResult {
        private Long nodeId;
        private String nodeName;
        private String targetIp;
        private Integer targetPort;
        private String description;
        private boolean success;
        private String message;
        private double averageTime;
        private double packetLoss;
        private long timestamp;

        // 链路类型相关字段
        private Integer fromChainType; // 1: 入口, 2: 链, 3: 出口
        private Integer fromInx;
        private Integer toChainType;
        private Integer toInx;
    }

    @Override
    @Transactional
    public R batchDeleteForwards(BatchDeleteDto batchDeleteDto) {
        UserInfo currentUser = getCurrentUserInfo();
        BatchOperationResultDto result = new BatchOperationResultDto();
        
        for (Long id : batchDeleteDto.getIds()) {
            try {
                Forward forward = validateForwardExists(id, currentUser);
                if (forward == null) {
                    result.addFailedItem(id, "转发不存在或无权限");
                    continue;
                }
                
                Tunnel tunnel = validateTunnel(forward.getTunnelId());
                if (tunnel == null) {
                    result.addFailedItem(id, "隧道不存在");
                    continue;
                }
                
                UserTunnel userTunnel = null;
                if (currentUser.getRoleId() != 0) {
                    userTunnel = getUserTunnel(currentUser.getUserId(), tunnel.getId().intValue());
                    if (userTunnel == null) {
                        result.addFailedItem(id, "没有该隧道权限");
                        continue;
                    }
                } else {
                    userTunnel = getUserTunnel(forward.getUserId(), tunnel.getId().intValue());
                }
                
                List<ChainTunnel> chainTunnels = chainTunnelService.list(
                    new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnel.getId()).eq("chain_type", 1)
                );
                
                boolean deleteSuccess = true;
                for (ChainTunnel chainTunnel : chainTunnels) {
                    String serviceName = buildServiceName(forward.getId(), forward.getUserId(), userTunnel);
                    Node node = nodeService.getById(chainTunnel.getNodeId());
                    if (node != null) {
                        JSONArray services = new JSONArray();
                        services.add(serviceName + "_tcp");
                        services.add(serviceName + "_udp");
                        GostUtil.DeleteService(node.getId(), services);
                    }
                }
                
                if (deleteSuccess) {
                    forwardPortService.remove(new QueryWrapper<ForwardPort>().eq("forward_id", id));
                    this.removeById(id);
                    result.incrementSuccess();
                }
            } catch (Exception e) {
                result.addFailedItem(id, e.getMessage());
            }
        }
        
        if (result.isAllSuccess()) {
            return R.ok(result);
        } else {
            return R.ok("部分操作失败", result);
        }
    }

    @Override
    @Transactional
    public R batchRedeployForwards(BatchRedeployDto batchRedeployDto) {
        UserInfo currentUser = getCurrentUserInfo();
        BatchOperationResultDto result = new BatchOperationResultDto();
        
        for (Long id : batchRedeployDto.getIds()) {
            try {
                Forward forward = validateForwardExists(id, currentUser);
                if (forward == null) {
                    result.addFailedItem(id, "转发不存在或无权限");
                    continue;
                }
                
                Tunnel tunnel = validateTunnel(forward.getTunnelId());
                if (tunnel == null) {
                    result.addFailedItem(id, "隧道不存在");
                    continue;
                }
                
                if (tunnel.getStatus() != 1) {
                    result.addFailedItem(id, "隧道已禁用");
                    continue;
                }
                
                UserPermissionResult permissionResult = checkUserPermissions(currentUser, tunnel, id);
                if (permissionResult.isHasError()) {
                    result.addFailedItem(id, permissionResult.getErrorMessage());
                    continue;
                }
                
                List<ForwardPort> forwardPorts = forwardPortService.list(
                    new QueryWrapper<ForwardPort>().eq("forward_id", id)
                );
                
                for (ForwardPort forwardPort : forwardPorts) {
                    String serviceName = buildServiceName(forward.getId(), forward.getUserId(), permissionResult.getUserTunnel());
                    Node node = nodeService.getById(forwardPort.getNodeId());
                    if (node != null) {
                        GostUtil.AddAndUpdateService(serviceName, permissionResult.getLimiter(), 
                            node, forward, forwardPort, tunnel, "UpdateService");
                    }
                }
                
                result.incrementSuccess();
            } catch (Exception e) {
                result.addFailedItem(id, e.getMessage());
            }
        }
        
        if (result.isAllSuccess()) {
            return R.ok(result);
        } else {
            return R.ok("部分操作失败", result);
        }
    }

    @Override
    @Transactional
    public R batchChangeTunnel(BatchChangeTunnelDto batchChangeTunnelDto) {
        UserInfo currentUser = getCurrentUserInfo();
        BatchOperationResultDto result = new BatchOperationResultDto();
        
        Long targetTunnelId = batchChangeTunnelDto.getTargetTunnelId();
        Tunnel targetTunnel = tunnelService.getById(targetTunnelId);
        if (targetTunnel == null) {
            return R.err("目标隧道不存在");
        }
        if (targetTunnel.getStatus() != 1) {
            return R.err("目标隧道已禁用");
        }
        
        for (Long forwardId : batchChangeTunnelDto.getForwardIds()) {
            try {
                Forward forward = validateForwardExists(forwardId, currentUser);
                if (forward == null) {
                    result.addFailedItem(forwardId, "转发不存在或无权限");
                    continue;
                }
                
                if (forward.getTunnelId().equals(targetTunnelId.intValue())) {
                    result.addFailedItem(forwardId, "已是目标隧道");
                    continue;
                }
                
                Tunnel oldTunnel = validateTunnel(forward.getTunnelId());
                if (oldTunnel != null) {
                    UserTunnel oldUserTunnel = null;
                    if (currentUser.getRoleId() != 0) {
                        oldUserTunnel = getUserTunnel(currentUser.getUserId(), oldTunnel.getId().intValue());
                    } else {
                        oldUserTunnel = getUserTunnel(forward.getUserId(), oldTunnel.getId().intValue());
                    }
                    
                    List<ChainTunnel> oldChainTunnels = chainTunnelService.list(
                        new QueryWrapper<ChainTunnel>().eq("tunnel_id", oldTunnel.getId()).eq("chain_type", 1)
                    );
                    for (ChainTunnel chainTunnel : oldChainTunnels) {
                        String serviceName = buildServiceName(forward.getId(), forward.getUserId(), oldUserTunnel);
                        Node node = nodeService.getById(chainTunnel.getNodeId());
                        if (node != null) {
                            JSONArray services = new JSONArray();
                            services.add(serviceName + "_tcp");
                            services.add(serviceName + "_udp");
                            GostUtil.DeleteService(node.getId(), services);
                        }
                    }
                }
                
                forwardPortService.remove(new QueryWrapper<ForwardPort>().eq("forward_id", forwardId));
                
                forward.setTunnelId(targetTunnelId.intValue());
                forward.setUpdatedTime(System.currentTimeMillis());
                this.updateById(forward);
                
                UserPermissionResult permissionResult = checkUserPermissions(currentUser, targetTunnel, forwardId);
                if (permissionResult.isHasError()) {
                    result.addFailedItem(forwardId, "切换成功但无法下发: " + permissionResult.getErrorMessage());
                    continue;
                }
                
                List<ChainTunnel> newChainTunnels = chainTunnelService.list(
                    new QueryWrapper<ChainTunnel>().eq("tunnel_id", targetTunnel.getId()).eq("chain_type", 1)
                );
                List<ChainTunnel> chainTunnelsWithPort = get_port(newChainTunnels, null, forwardId);
                
                for (ChainTunnel chainTunnel : chainTunnelsWithPort) {
                    ForwardPort forwardPort = new ForwardPort();
                    forwardPort.setForwardId(forwardId);
                    forwardPort.setNodeId(chainTunnel.getNodeId());
                    forwardPort.setPort(chainTunnel.getPort());
                    forwardPortService.save(forwardPort);
                    
                    String serviceName = buildServiceName(forward.getId(), forward.getUserId(), permissionResult.getUserTunnel());
                    Node node = nodeService.getById(chainTunnel.getNodeId());
                    if (node != null) {
                        GostUtil.AddAndUpdateService(serviceName, permissionResult.getLimiter(), 
                            node, forward, forwardPort, targetTunnel, "AddService");
                    }
                }
                
                result.incrementSuccess();
            } catch (Exception e) {
                result.addFailedItem(forwardId, e.getMessage());
            }
        }
        
        if (result.isAllSuccess()) {
            return R.ok(result);
        } else {
            return R.ok("部分操作失败", result);
        }
    }

}
