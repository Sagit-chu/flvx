package com.admin.service;

import com.admin.common.dto.BatchDeleteDto;
import com.admin.common.dto.NodeDto;
import com.admin.common.dto.NodeUpdateDto;
import com.admin.common.lang.R;
import com.admin.entity.Node;
import com.baomidou.mybatisplus.extension.service.IService;

import java.util.Map;

/**
 * <p>
 *  服务类
 * </p>
 *
 * @author QAQ
 * @since 2025-06-03
 */
public interface NodeService extends IService<Node> {

    R createNode(NodeDto nodeDto);

    R getAllNodes();

    R updateNode(NodeUpdateDto nodeUpdateDto);

    R deleteNode(Long id);

    R getInstallCommand(Long id);

    /**
     * 更新节点排序（管理员）
     * @param params 包含nodes数组的参数，每个元素包含id和inx
     */
    R updateNodeOrder(Map<String, Object> params);

    R batchDeleteNodes(BatchDeleteDto batchDeleteDto);

}
