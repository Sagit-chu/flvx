package com.admin.service.impl;

import cn.hutool.core.util.IdUtil;
import cn.hutool.core.util.StrUtil;
import com.admin.common.dto.GostDto;
import com.admin.common.dto.NodeDto;
import com.admin.common.dto.NodeUpdateDto;
import com.admin.common.lang.R;
import com.admin.common.utils.GostUtil;
import com.admin.common.utils.WebSocketServer;
import com.admin.entity.*;
import com.admin.mapper.NodeMapper;
import com.admin.mapper.TunnelMapper;
import com.admin.service.*;
import com.alibaba.fastjson.JSONArray;
import com.alibaba.fastjson.JSONObject;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import com.baomidou.mybatisplus.core.conditions.update.LambdaUpdateWrapper;
import com.baomidou.mybatisplus.extension.plugins.pagination.Page;
import com.baomidou.mybatisplus.extension.service.impl.ServiceImpl;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.BeanUtils;
import org.springframework.context.annotation.Lazy;
import org.springframework.stereotype.Service;
 
import javax.annotation.Resource;
import java.util.HashMap;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.stream.Collectors;
import java.util.regex.Pattern;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.transaction.annotation.Transactional;

@Service
@Slf4j
public class NodeServiceImpl extends ServiceImpl<NodeMapper, Node> implements NodeService {


    @Resource
    @Lazy
    private TunnelService tunnelService;

    @Resource
    ViteConfigService viteConfigService;

    @Resource
    ChainTunnelService chainTunnelService;

    @Resource
    ForwardPortService forwardPortService;


    @Override
    public R createNode(NodeDto nodeDto) {
        validatePortRange(nodeDto.getPort());

        String normalizedV4 = normalizeV4(nodeDto.getServerIpV4(), nodeDto.getServerIp());
        String normalizedV6 = normalizeV6(nodeDto.getServerIpV6(), nodeDto.getServerIp());
        String primaryServerIp = pickPrimaryServerIp(normalizedV4, normalizedV6, nodeDto.getServerIp());

        Node node = new Node();
        node.setSecret(IdUtil.simpleUUID());
        node.setStatus(0);
        node.setPort(nodeDto.getPort());
        node.setName(nodeDto.getName());
        node.setServerIp(primaryServerIp);
        node.setServerIpV4(normalizedV4);
        node.setServerIpV6(normalizedV6);
        long currentTime = System.currentTimeMillis();
        node.setCreatedTime(currentTime);
        node.setUpdatedTime(currentTime);
        node.setInterfaceName(nodeDto.getInterfaceName());
        this.save(node);
        return R.ok();
    }

    @Override
    public R getAllNodes() {
        List<Node> nodeList = this.list(new QueryWrapper<Node>().orderByAsc("inx").orderByAsc("id"));
        nodeList.forEach(node -> node.setSecret(null));
        return R.ok(nodeList);
    }

    @Override
    @Transactional
    public R updateNodeOrder(Map<String, Object> params) {
        if (!params.containsKey("nodes")) {
            return R.err("缺少nodes参数");
        }

        @SuppressWarnings("unchecked")
        List<Map<String, Object>> nodesList = (List<Map<String, Object>>) params.get("nodes");
        if (nodesList == null || nodesList.isEmpty()) {
            return R.err("nodes参数不能为空");
        }

        List<Node> nodesToUpdate = new ArrayList<>();
        for (Map<String, Object> nodeData : nodesList) {
            Long id = Long.valueOf(nodeData.get("id").toString());
            Integer inx = Integer.valueOf(nodeData.get("inx").toString());

            Node node = new Node();
            node.setId(id);
            node.setInx(inx);
            nodesToUpdate.add(node);
        }

        this.updateBatchById(nodesToUpdate);
        return R.ok();
    }

    @Override
    public R updateNode(NodeUpdateDto nodeUpdateDto) {
        Node node = this.getById(nodeUpdateDto.getId());
        if (node == null) {
            return R.err("节点不存在");
        }

        boolean online = node.getStatus() != null && node.getStatus() == 1;
        Integer newHttp = nodeUpdateDto.getHttp();
        Integer newTls = nodeUpdateDto.getTls();
        Integer newSocks = nodeUpdateDto.getSocks();

        boolean httpChanged = newHttp != null && !newHttp.equals(node.getHttp());
        boolean tlsChanged = newTls != null && !newTls.equals(node.getTls());
        boolean socksChanged = newSocks != null && !newSocks.equals(node.getSocks());

        if (online && (httpChanged || tlsChanged || socksChanged)) {
            JSONObject req = new JSONObject();
            req.put("http", newHttp);
            req.put("tls", newTls);
            req.put("socks", newSocks);

            GostDto gostResult = WebSocketServer.send_msg(node.getId(), req, "SetProtocol");
            if (!Objects.equals(gostResult.getMsg(), "OK")){
                return R.err(gostResult.getMsg());
            }
        }



        Node updateNode = buildUpdateNode(nodeUpdateDto);
        // Use LambdaUpdateWrapper to explicitly set nullable fields (serverIpV4/V6)
        // because updateById() skips null fields by default
        LambdaUpdateWrapper<Node> wrapper = new LambdaUpdateWrapper<>();
        wrapper.eq(Node::getId, updateNode.getId())
               .set(Node::getName, updateNode.getName())
               .set(Node::getServerIp, updateNode.getServerIp())
               .set(Node::getServerIpV4, updateNode.getServerIpV4())
               .set(Node::getServerIpV6, updateNode.getServerIpV6())
               .set(Node::getPort, updateNode.getPort())
               .set(Node::getHttp, updateNode.getHttp())
               .set(Node::getTls, updateNode.getTls())
               .set(Node::getSocks, updateNode.getSocks())
               .set(Node::getInterfaceName, updateNode.getInterfaceName())
               .set(Node::getTcpListenAddr, updateNode.getTcpListenAddr())
               .set(Node::getUdpListenAddr, updateNode.getUdpListenAddr())
               .set(Node::getUpdatedTime, updateNode.getUpdatedTime());
        this.update(wrapper);
        return R.ok();
    }

    @Override
    public R deleteNode(Long id) {
        Node node = this.getById(id);
        if (node == null) {
            return R.err("节点不存在");
        }

        List<ChainTunnel> affected = chainTunnelService.list(new QueryWrapper<ChainTunnel>().eq("node_id", id));
        Map<Long, List<ChainTunnel>> byTunnelId = affected.stream()
                .filter(ct -> ct.getTunnelId() != null)
                .collect(Collectors.groupingBy(ChainTunnel::getTunnelId));

        for (Map.Entry<Long, List<ChainTunnel>> entry : byTunnelId.entrySet()) {
            Long tunnelId = entry.getKey();
            Tunnel tunnel = tunnelService.getById(tunnelId);

            List<ChainTunnel> before = chainTunnelService.list(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnelId));

            // Remove the node from the tunnel definition (do NOT delete the tunnel).
            chainTunnelService.remove(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnelId).eq("node_id", id));

            if (tunnel == null) {
                continue;
            }

            List<ChainTunnel> after = chainTunnelService.list(new QueryWrapper<ChainTunnel>().eq("tunnel_id", tunnelId));
            Integer removedChainType = entry.getValue().isEmpty() ? null : entry.getValue().get(0).getChainType();

            // Keep tunnel.inIp consistent when it was auto-derived from entry nodes.
            String oldDerivedInIp = buildDerivedInIp(before);
            String newDerivedInIp = buildDerivedInIp(after);
            if (shouldUpdateTunnelInIp(tunnel.getInIp(), oldDerivedInIp)) {
                updateTunnelInIp(tunnelId, newDerivedInIp);
            }

            boolean valid = isTunnelConfigValid(tunnel, after);
            if (!valid) {
                disableTunnelAndCleanupGostIfNeeded(tunnel, after, "node-delete");
                continue;
            }

            // For tunnel-forwarding (type=2), removing a chain/out node requires rebuilding config.
            // Removing an entry node (chainType=1) does not affect remaining nodes' chain targets.
            if (tunnel.getType() != null && tunnel.getType() == 2 && removedChainType != null && removedChainType != 1) {
                try {
                    cleanupGostConfig(after, tunnelId);
                    rebuildGostConfig(after, tunnel);
                } catch (Exception e) {
                    log.warn("Failed to rebuild gost config after node delete. tunnelId={}, nodeId={}, err={}", tunnelId, id, e.getMessage(), e);
                    disableTunnelAndCleanupGostIfNeeded(tunnel, after, "node-delete:rebuild-failed");
                }
            }
        }

        // Remove per-forward port allocations on this node (avoid orphan ForwardPort rows).
        try {
            forwardPortService.remove(new QueryWrapper<ForwardPort>().eq("node_id", id));
        } catch (Exception e) {
            log.warn("Failed to cleanup forward ports when deleting node. nodeId={}, err={}", id, e.getMessage(), e);
        }

        this.removeById(id);
        return R.ok();
    }

    private boolean isTunnelConfigValid(Tunnel tunnel, List<ChainTunnel> chainTunnels) {
        if (tunnel == null || chainTunnels == null) {
            return false;
        }

        long inCount = chainTunnels.stream()
                .filter(ct -> ct.getChainType() != null && ct.getChainType() == 1)
                .count();
        if (inCount <= 0) {
            return false;
        }

        if (tunnel.getType() != null && tunnel.getType() == 2) {
            long outCount = chainTunnels.stream()
                    .filter(ct -> ct.getChainType() != null && ct.getChainType() == 3)
                    .count();
            return outCount > 0;
        }

        return true;
    }

    private boolean shouldUpdateTunnelInIp(String currentInIp, String oldDerivedInIp) {
        if (StrUtil.isBlank(currentInIp)) {
            return true;
        }
        if (oldDerivedInIp == null) {
            return false;
        }
        return Objects.equals(currentInIp, oldDerivedInIp);
    }

    private void updateTunnelInIp(Long tunnelId, String derivedInIp) {
        Tunnel update = new Tunnel();
        update.setId(tunnelId);
        update.setInIp(derivedInIp == null ? "" : derivedInIp);
        update.setUpdatedTime(System.currentTimeMillis());
        tunnelService.updateById(update);
    }

    private String buildDerivedInIp(List<ChainTunnel> chainTunnels) {
        if (chainTunnels == null) {
            return "";
        }
        List<ChainTunnel> inNodes = chainTunnels.stream()
                .filter(ct -> ct.getChainType() != null && ct.getChainType() == 1)
                .collect(Collectors.toList());
        if (inNodes.isEmpty()) {
            return "";
        }

        StringBuilder inIp = new StringBuilder();
        for (ChainTunnel inNode : inNodes) {
            Node n = this.getById(inNode.getNodeId());
            if (n == null || StrUtil.isBlank(n.getServerIp())) {
                return null;
            }
            inIp.append(n.getServerIp()).append(",");
        }
        inIp.deleteCharAt(inIp.length() - 1);
        return inIp.toString();
    }

    private void disableTunnelAndCleanupGostIfNeeded(Tunnel tunnel, List<ChainTunnel> remaining, String reason) {
        try {
            Tunnel update = new Tunnel();
            update.setId(tunnel.getId());
            update.setStatus(0);
            update.setUpdatedTime(System.currentTimeMillis());
            tunnelService.updateById(update);
        } catch (Exception e) {
            log.warn("Failed to disable tunnel. tunnelId={}, reason={}, err={}", tunnel.getId(), reason, e.getMessage(), e);
        }

        if (tunnel.getType() != null && tunnel.getType() == 2) {
            try {
                cleanupGostConfig(remaining, tunnel.getId());
            } catch (Exception e) {
                log.warn("Failed to cleanup gost config when disabling tunnel. tunnelId={}, reason={}, err={}", tunnel.getId(), reason, e.getMessage(), e);
            }
        }
    }

    private void cleanupGostConfig(List<ChainTunnel> chainTunnels, Long tunnelId) {
        if (chainTunnels == null) {
            return;
        }
        for (ChainTunnel chainTunnel : chainTunnels) {
            if (chainTunnel.getChainType() == null) {
                continue;
            }
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

    private void rebuildGostConfig(List<ChainTunnel> chainTunnels, Tunnel tunnel) {
        if (tunnel == null || chainTunnels == null) {
            return;
        }

        Map<Long, Node> nodes = new HashMap<>();
        for (ChainTunnel ct : chainTunnels) {
            Node n = this.getById(ct.getNodeId());
            if (n != null) {
                nodes.put(n.getId(), n);
            }
        }

        List<ChainTunnel> inNodes = chainTunnels.stream()
                .filter(ct -> ct.getChainType() != null && ct.getChainType() == 1)
                .collect(Collectors.toList());

        Map<Integer, List<ChainTunnel>> chainNodesMap = chainTunnels.stream()
                .filter(ct -> ct.getChainType() != null && ct.getChainType() == 2)
                .collect(Collectors.groupingBy(ct -> ct.getInx() != null ? ct.getInx() : 0));

        List<List<ChainTunnel>> chainNodesList = chainNodesMap.entrySet().stream()
                .sorted(Map.Entry.comparingByKey())
                .map(Map.Entry::getValue)
                .collect(Collectors.toList());

        List<ChainTunnel> outNodes = chainTunnels.stream()
                .filter(ct -> ct.getChainType() != null && ct.getChainType() == 3)
                .collect(Collectors.toList());

        if (tunnel.getType() != null && tunnel.getType() == 2) {
            for (ChainTunnel inNode : inNodes) {
                if (chainNodesList.isEmpty()) {
                    GostUtil.AddChains(inNode.getNodeId(), outNodes, nodes);
                } else {
                    GostUtil.AddChains(inNode.getNodeId(), chainNodesList.get(0), nodes);
                }
            }

            for (int i = 0; i < chainNodesList.size(); i++) {
                for (ChainTunnel chainNode : chainNodesList.get(i)) {
                    if (i + 1 >= chainNodesList.size()) {
                        GostUtil.AddChains(chainNode.getNodeId(), outNodes, nodes);
                    } else {
                        GostUtil.AddChains(chainNode.getNodeId(), chainNodesList.get(i + 1), nodes);
                    }
                    GostUtil.AddChainService(chainNode.getNodeId(), chainNode, nodes);
                }
            }

            for (ChainTunnel outNode : outNodes) {
                GostUtil.AddChainService(outNode.getNodeId(), outNode, nodes);
            }
        }
    }


    @Override
    public R getInstallCommand(Long id) {
        Node node = this.getById(id);
        if (node == null) {
            return R.err("节点不存在");
        }
        ViteConfig viteConfig = viteConfigService.getOne(new QueryWrapper<ViteConfig>().eq("name", "ip"));
        if (viteConfig == null) return R.err("请先前往网站配置中设置ip");
        StringBuilder command = new StringBuilder();
        command.append("curl -L https://github.com/Sagit-chu/flux-panel/releases/latest/download/install.sh")
                .append(" -o ./install.sh && chmod +x ./install.sh && ");
        String processedServerAddr = GostUtil.processServerAddress(viteConfig.getValue());
        command.append("./install.sh")
                .append(" -a ").append(processedServerAddr)  // 服务器地址
                .append(" -s ").append(node.getSecret());    // 节点密钥
        return R.ok(command);

    }


    private Node buildUpdateNode(NodeUpdateDto nodeUpdateDto) {
        validatePortRange(nodeUpdateDto.getPort());

        String normalizedV4 = normalizeV4(nodeUpdateDto.getServerIpV4(), nodeUpdateDto.getServerIp());
        String normalizedV6 = normalizeV6(nodeUpdateDto.getServerIpV6(), nodeUpdateDto.getServerIp());
        String primaryServerIp = pickPrimaryServerIp(normalizedV4, normalizedV6, nodeUpdateDto.getServerIp());

        Node node = new Node();
        node.setId(nodeUpdateDto.getId());
        node.setName(nodeUpdateDto.getName());
        node.setServerIp(primaryServerIp);
        node.setServerIpV4(normalizedV4);
        node.setServerIpV6(normalizedV6);
        node.setPort(nodeUpdateDto.getPort());
        node.setHttp(nodeUpdateDto.getHttp());
        node.setTls(nodeUpdateDto.getTls());
        node.setSocks(nodeUpdateDto.getSocks());
        node.setUpdatedTime(System.currentTimeMillis());
        node.setInterfaceName(nodeUpdateDto.getInterfaceName());
        node.setTcpListenAddr(nodeUpdateDto.getTcpListenAddr());
        node.setUdpListenAddr(nodeUpdateDto.getUdpListenAddr());
        return node;
    }

    private String pickPrimaryServerIp(String serverIpV4, String serverIpV6, String fallback) {
        if (StrUtil.isNotBlank(serverIpV4)) {
            return serverIpV4.trim();
        }
        if (StrUtil.isNotBlank(serverIpV6)) {
            return serverIpV6.trim();
        }
        return fallback != null ? fallback.trim() : null;
    }

    private String normalizeV4(String serverIpV4, String legacyServerIp) {
        if (StrUtil.isNotBlank(serverIpV4)) {
            return serverIpV4.trim();
        }
        if (StrUtil.isNotBlank(legacyServerIp) && looksLikeIpv4(legacyServerIp.trim())) {
            return legacyServerIp.trim();
        }
        return null;
    }

    private String normalizeV6(String serverIpV6, String legacyServerIp) {
        if (StrUtil.isNotBlank(serverIpV6)) {
            return serverIpV6.trim();
        }
        if (StrUtil.isNotBlank(legacyServerIp) && looksLikeIpv6(legacyServerIp.trim())) {
            return legacyServerIp.trim();
        }
        return null;
    }

    private boolean looksLikeIpv4(String value) {
        // 仅用于判定地址族（不解析域名）
        Pattern ipv4 = Pattern.compile("^(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$");
        return ipv4.matcher(value).matches();
    }

    private boolean looksLikeIpv6(String value) {
        // 粗略判定 IPv6（与 GostUtil.processServerAddress 一致思路）
        long colonCount = value.chars().filter(ch -> ch == ':').count();
        return colonCount >= 2;
    }


    private void validatePortRange(String port) {
        Pattern PORT_PATTERN = Pattern.compile(   "([0-9]{1,5})(-([0-9]{1,5}))?");
        if (port == null || port.isEmpty()) {
            throw new RuntimeException("可用端口不合法");
        }
        String[] parts = port.split(",");
        for (String part : parts) {
            part = part.trim();
            if (!PORT_PATTERN.matcher(part).matches()) {
                throw new RuntimeException("可用端口不合法");
            }
            if (part.contains("-")) {
                String[] range = part.split("-");
                int start = Integer.parseInt(range[0]);
                int end = Integer.parseInt(range[1]);
                if (start < 0 || end < 0 || end > 65535 || start > end) {
                    throw new RuntimeException("可用端口不合法");
                }
            } else {
                int ports = Integer.parseInt(part);
                if (ports < 0 || ports > 65535) {
                    throw new RuntimeException("可用端口不合法");
                }
            }
        }
    }




}
