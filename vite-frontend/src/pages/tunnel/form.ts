interface TunnelChainNode {
  nodeId: number;
}

interface TunnelFormInput {
  name: string;
  type: number;
  inNodeId: TunnelChainNode[];
  outNodeId?: TunnelChainNode[];
  trafficRatio: number;
}

interface TunnelNodeInput {
  id: number;
  status: number;
}

export const createTunnelFormDefaults = () => {
  return {
    name: "",
    type: 1,
    inNodeId: [],
    outNodeId: [],
    chainNodes: [],
    flow: 1,
    trafficRatio: 1.0,
    inIp: "",
    ipPreference: "",
    status: 1,
  };
};

export const validateTunnelForm = (
  form: TunnelFormInput,
  nodes: TunnelNodeInput[],
): Record<string, string> => {
  const errors: Record<string, string> = {};

  if (!form.name.trim()) {
    errors.name = "请输入隧道名称";
  } else if (form.name.length < 2 || form.name.length > 50) {
    errors.name = "隧道名称长度应在2-50个字符之间";
  }

  if (!form.inNodeId || form.inNodeId.length === 0) {
    errors.inNodeId = "请至少选择一个入口节点";
  } else {
    const offlineInNodes = form.inNodeId.filter((item) => {
      const node = nodes.find((n) => n.id === item.nodeId);

      return node && node.status !== 1;
    });

    if (offlineInNodes.length > 0) {
      errors.inNodeId = "所有入口节点必须在线";
    }
  }

  if (form.trafficRatio <= 0 || form.trafficRatio > 100.0) {
    errors.trafficRatio = "流量倍率须大于0，支持小数（如 0.5）";
  }

  if (form.type === 2) {
    if (!form.outNodeId || form.outNodeId.length === 0) {
      errors.outNodeId = "请至少选择一个出口节点";
    } else {
      const offlineOutNodes = form.outNodeId.filter((item) => {
        const node = nodes.find((n) => n.id === item.nodeId);

        return node && node.status !== 1;
      });

      if (offlineOutNodes.length > 0) {
        errors.outNodeId = "所有出口节点必须在线";
      }

      const inNodeIds = form.inNodeId.map((item) => item.nodeId);
      const outNodeIds = form.outNodeId.map((item) => item.nodeId);
      const overlap = inNodeIds.filter((id) => outNodeIds.includes(id));

      if (overlap.length > 0) {
        errors.outNodeId = "隧道转发模式下，入口和出口不能有相同节点";
      }
    }
  }

  return errors;
};

export const getTunnelTypeDisplay = (type: number) => {
  switch (type) {
    case 1:
      return { text: "端口转发", color: "primary" };
    case 2:
      return { text: "隧道转发", color: "secondary" };
    default:
      return { text: "未知", color: "default" };
  }
};

export const getTunnelFlowDisplay = (flow: number) => {
  switch (flow) {
    case 1:
      return "单向计算";
    case 2:
      return "双向计算";
    default:
      return "未知";
  }
};
