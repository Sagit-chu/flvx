import { useCallback, useEffect, useMemo, useState } from "react";
import ReactFlow, {
  Background,
  Controls,
  MarkerType,
  MiniMap,
  type Edge,
  type Node,
  type XYPosition,
  useEdgesState,
  useNodesState,
} from "reactflow";
import "reactflow/dist/style.css";
import toast from "react-hot-toast";

import {
  createForward,
  createTunnel,
  getForwardList,
  getNodeList,
  getTunnelList,
  updateForward,
  updateTunnel,
} from "@/api";
import type {
  ForwardApiItem,
  ForwardMutationPayload,
  NodeApiItem,
  TunnelApiItem,
  TunnelChainNodePayload,
  TunnelMutationPayload,
} from "@/api/types";
import Network from "@/api/network";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Chip } from "@/shadcn-bridge/heroui/chip";
import { Divider } from "@/shadcn-bridge/heroui/divider";
import { Input, Textarea } from "@/shadcn-bridge/heroui/input";
import { Select, SelectItem } from "@/shadcn-bridge/heroui/select";
import { validateTunnelForm } from "@/pages/tunnel/form";
import VisualEntityNode, {
  type VisualEntityNodeData,
} from "./nodes/VisualEntityNode";

interface TunnelEditorState {
  chainNodes: TunnelChainNodePayload[][];
  draftId?: string;
  flow: number;
  id?: number;
  inIp: string;
  inNodeId: TunnelChainNodePayload[];
  ipPreference: string;
  name: string;
  outNodeId: TunnelChainNodePayload[];
  status: number;
  trafficRatio: number;
  type: number;
}

interface ForwardEditorState {
  draftId?: string;
  id?: number;
  inIp: string;
  inPort: number | null;
  name: string;
  remoteAddr: string;
  status: number;
  strategy: string;
  tunnelId: number | null;
  tunnelName?: string;
}

interface VisualProbeData {
  allowed_ports?: string;
  connections?: number;
  cpu_usage?: number;
  mem_usage?: number;
  metric_timestamp?: number;
  occupied_ports?: number[];
  status?: string;
}

interface VisualSnapshot {
  forwards: ForwardEditorState[];
  positions?: Record<string, XYPosition>;
  systemNodes: NodeApiItem[];
  tunnels: TunnelEditorState[];
}

const nodeTypes = {
  entity: VisualEntityNode,
};

const TUNNEL_PROTOCOL_OPTIONS = ["tls", "wss", "tcp", "mtls", "mwss", "mtcp"];
const TUNNEL_STRATEGY_OPTIONS = ["fifo", "round", "rand"];

const SYSTEM_PREFIX = "system:";
const TUNNEL_PREFIX = "tunnel:";
const TUNNEL_DRAFT_PREFIX = "tunnel:draft:";
const FORWARD_PREFIX = "forward:";
const FORWARD_DRAFT_PREFIX = "forward:draft:";

const getSystemVisualId = (id: number) => `${SYSTEM_PREFIX}${id}`;
const getTunnelVisualId = (id: number) => `${TUNNEL_PREFIX}${id}`;
const getTunnelDraftVisualId = (draftId: string) =>
  `${TUNNEL_DRAFT_PREFIX}${draftId}`;
const getForwardVisualId = (id: number) => `${FORWARD_PREFIX}${id}`;
const getForwardDraftVisualId = (draftId: string) =>
  `${FORWARD_DRAFT_PREFIX}${draftId}`;

const buildTunnelNodeId = (item: TunnelEditorState) =>
  typeof item.id === "number" && item.id > 0
    ? getTunnelVisualId(item.id)
    : getTunnelDraftVisualId(item.draftId || "draft");

const buildForwardNodeId = (item: ForwardEditorState) =>
  typeof item.id === "number" && item.id > 0
    ? getForwardVisualId(item.id)
    : getForwardDraftVisualId(item.draftId || "draft");

const isSystemNodeId = (value: string | null | undefined): value is string =>
  typeof value === "string" && value.startsWith(SYSTEM_PREFIX);
const isTunnelNodeId = (value: string | null | undefined): value is string =>
  typeof value === "string" &&
  (value.startsWith(TUNNEL_PREFIX) || value.startsWith(TUNNEL_DRAFT_PREFIX));
const isForwardNodeId = (value: string | null | undefined): value is string =>
  typeof value === "string" &&
  (value.startsWith(FORWARD_PREFIX) || value.startsWith(FORWARD_DRAFT_PREFIX));

const extractVisualNumericId = (value: string, prefix: string): number | null => {
  if (!value.startsWith(prefix)) {
    return null;
  }

  const parsed = Number.parseInt(value.slice(prefix.length), 10);

  return Number.isFinite(parsed) ? parsed : null;
};

const extractDraftId = (value: string, prefix: string): string | null => {
  if (!value.startsWith(prefix)) {
    return null;
  }

  const parsed = value.slice(prefix.length).trim();

  return parsed || null;
};

const cloneChainNode = (
  item: TunnelChainNodePayload,
): TunnelChainNodePayload => ({
  ...item,
});

const cloneTunnelState = (item: TunnelEditorState): TunnelEditorState => ({
  ...item,
  chainNodes: (item.chainNodes || []).map((group) => group.map(cloneChainNode)),
  inNodeId: (item.inNodeId || []).map(cloneChainNode),
  outNodeId: (item.outNodeId || []).map(cloneChainNode),
});

const cloneForwardState = (item: ForwardEditorState): ForwardEditorState => ({
  ...item,
});

const splitMultiValue = (value?: string): string[] =>
  String(value || "")
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter((item) => item !== "");

const commaToMultiline = (value?: string) => splitMultiValue(value).join("\n");
const multilineToComma = (value?: string) => splitMultiValue(value).join(",");

const normalizeAddressSignature = (value?: string) =>
  splitMultiValue(value)
    .map((item) => item.toLowerCase())
    .join(",");

const buildAddressPreview = (value?: string) => {
  const items = splitMultiValue(value);

  if (items.length === 0) {
    return "未配置目标地址";
  }

  if (items.length === 1) {
    return items[0];
  }

  return `${items[0]} 等 ${items.length} 个目标`;
};

const getTunnelTypeLabel = (type: number) =>
  type === 2 ? "隧道转发" : "端口转发";

const getStatusLabel = (status: number) => (status === 1 ? "启用" : "停用");

const selectionToStrings = (keys: unknown): string[] => {
  if (!keys || keys === "all") {
    return [];
  }

  return Array.from(keys as Iterable<unknown>).map((item) => String(item));
};

const selectionToNodeIds = (keys: unknown): number[] =>
  selectionToStrings(keys)
    .map((item) => Number.parseInt(item, 10))
    .filter((item) => Number.isFinite(item));

const mergeOrderedNodes = (
  currentNodes: TunnelChainNodePayload[],
  selectedNodeIds: number[],
  buildDefault: (nodeId: number) => TunnelChainNodePayload,
): TunnelChainNodePayload[] => {
  const selectedSet = new Set(selectedNodeIds);
  const kept = currentNodes.filter((item) => selectedSet.has(item.nodeId));
  const keptIds = new Set(kept.map((item) => item.nodeId));
  const added = selectedNodeIds
    .filter((nodeId) => !keptIds.has(nodeId))
    .map((nodeId) => buildDefault(nodeId));

  return [...kept, ...added];
};

const getTunnelNodeIds = (item: TunnelEditorState): number[] => {
  const result = new Set<number>();

  (item.inNodeId || []).forEach((node) => {
    if (node.nodeId > 0) {
      result.add(node.nodeId);
    }
  });

  (item.outNodeId || []).forEach((node) => {
    if (node.nodeId > 0) {
      result.add(node.nodeId);
    }
  });

  (item.chainNodes || []).forEach((group) => {
    group.forEach((node) => {
      if (node.nodeId > 0) {
        result.add(node.nodeId);
      }
    });
  });

  return Array.from(result);
};

const getSelectedChainNodeIds = (item: TunnelEditorState): number[] =>
  (item.chainNodes || [])
    .flatMap((group) => group.map((node) => node.nodeId))
    .filter((nodeId) => nodeId > 0);

const getNodeIpOptions = (nodes: NodeApiItem[], nodeId: number): string[] => {
  const node = nodes.find((item) => item.id === nodeId);

  if (!node) {
    return [];
  }

  const values = [
    String(node.serverIpV4 || "").trim(),
    String(node.serverIpV6 || "").trim(),
    String(node.serverIp || "").trim(),
    ...String(node.extraIPs || "")
      .split(",")
      .map((item) => item.trim())
      .filter((item) => item !== ""),
  ].filter((item) => item !== "");

  return Array.from(new Set(values));
};

const getCommonIpOptions = (nodes: NodeApiItem[], nodeIds: number[]): string[] => {
  if (nodeIds.length === 0) {
    return [];
  }

  const optionSets = nodeIds.map((nodeId) => new Set(getNodeIpOptions(nodes, nodeId)));
  const base = optionSets[0];

  return Array.from(base).filter((item) =>
    optionSets.every((optionSet) => optionSet.has(item)),
  );
};

const createDraftTunnel = (draftId: string): TunnelEditorState => ({
  chainNodes: [],
  draftId,
  flow: 1,
  inIp: "",
  inNodeId: [],
  ipPreference: "",
  name: "新建隧道",
  outNodeId: [],
  status: 1,
  trafficRatio: 1,
  type: 1,
});

const createDraftForward = (draftId: string): ForwardEditorState => ({
  draftId,
  inIp: "",
  inPort: null,
  name: "新建规则",
  remoteAddr: "",
  status: 1,
  strategy: "fifo",
  tunnelId: null,
});

const mapTunnelApiItems = (items: TunnelApiItem[]): TunnelEditorState[] =>
  (items || []).map((item) => ({
    chainNodes: Array.isArray(item.chainNodes)
      ? item.chainNodes.map((group) =>
          (group || []).map((node) => ({ ...node })),
        )
      : [],
    flow: typeof item.flow === "number" ? item.flow : 1,
    id: item.id,
    inIp: typeof item.inIp === "string" ? item.inIp : "",
    inNodeId: Array.isArray(item.inNodeId)
      ? item.inNodeId.map((node) => ({ ...node }))
      : [],
    ipPreference: typeof item.ipPreference === "string" ? item.ipPreference : "",
    name: typeof item.name === "string" ? item.name : "",
    outNodeId: Array.isArray(item.outNodeId)
      ? item.outNodeId.map((node) => ({ ...node }))
      : [],
    status: typeof item.status === "number" ? item.status : 1,
    trafficRatio:
      typeof item.trafficRatio === "number" && Number.isFinite(item.trafficRatio)
        ? item.trafficRatio
        : 1,
    type: typeof item.type === "number" ? item.type : 1,
  }));

const mapForwardApiItems = (items: ForwardApiItem[]): ForwardEditorState[] =>
  (items || []).map((item) => ({
    id: item.id,
    inIp: typeof item.inIp === "string" ? item.inIp : "",
    inPort:
      typeof item.inPort === "number" && Number.isFinite(item.inPort)
        ? item.inPort
        : null,
    name: typeof item.name === "string" ? item.name : "",
    remoteAddr: typeof item.remoteAddr === "string" ? item.remoteAddr : "",
    status: typeof item.status === "number" ? item.status : 1,
    strategy: typeof item.strategy === "string" ? item.strategy : "fifo",
    tunnelId:
      typeof item.tunnelId === "number" && Number.isFinite(item.tunnelId)
        ? item.tunnelId
        : null,
    tunnelName:
      typeof item.tunnelName === "string" ? item.tunnelName : undefined,
  }));

const tunnelToEditor = (item: TunnelEditorState): TunnelEditorState => ({
  ...cloneTunnelState(item),
  inIp: commaToMultiline(item.inIp),
});

const editorToTunnelState = (item: TunnelEditorState): TunnelEditorState =>
  cloneTunnelState(item);

const forwardToEditor = (item: ForwardEditorState): ForwardEditorState => ({
  ...cloneForwardState(item),
  remoteAddr: commaToMultiline(item.remoteAddr),
});

const editorToForwardState = (item: ForwardEditorState): ForwardEditorState =>
  cloneForwardState(item);

const parseSavedPositions = (
  graphJSON?: string,
): Record<string, XYPosition> => {
  if (!graphJSON) {
    return {};
  }

  try {
    const parsed = JSON.parse(graphJSON);

    if (parsed && typeof parsed === "object") {
      if (parsed.positions && typeof parsed.positions === "object") {
        return Object.entries(parsed.positions).reduce<Record<string, XYPosition>>(
          (acc, [key, value]) => {
            const x = Number((value as { x?: number }).x);
            const y = Number((value as { y?: number }).y);

            if (Number.isFinite(x) && Number.isFinite(y)) {
              acc[key] = { x, y };
            }

            return acc;
          },
          {},
        );
      }

      if (Array.isArray(parsed.nodes)) {
        return (parsed.nodes as Array<any>).reduce<Record<string, XYPosition>>(
          (acc, item) => {
          const id = String(item?.id || "");
          const x = Number(item?.position?.x);
          const y = Number(item?.position?.y);

          if (id && Number.isFinite(x) && Number.isFinite(y)) {
            acc[id] = { x, y };
          }

          return acc;
          },
          {},
        );
      }
    }
  } catch {}

  return {};
};

const serializeLayout = (items: Node<VisualEntityNodeData>[]) =>
  JSON.stringify({
    positions: Object.fromEntries(
      items.map((item) => [
        item.id,
        {
          x: Math.round(item.position.x * 100) / 100,
          y: Math.round(item.position.y * 100) / 100,
        },
      ]),
    ),
    version: 2,
  });

const buildGraphNodes = (
  systemNodes: NodeApiItem[],
  tunnels: TunnelEditorState[],
  forwards: ForwardEditorState[],
  positions: Record<string, XYPosition>,
  selectedNodeId: string | null,
): Node<VisualEntityNodeData>[] => {
  const nodeTunnelCount = new Map<number, number>();
  const nodeForwardCount = new Map<number, number>();
  const tunnelNodeMap = new Map<number, number[]>();

  tunnels.forEach((tunnel) => {
    const nodeIds = getTunnelNodeIds(tunnel);

    if (typeof tunnel.id === "number" && tunnel.id > 0) {
      tunnelNodeMap.set(tunnel.id, nodeIds);
    }

    nodeIds.forEach((nodeId) => {
      nodeTunnelCount.set(nodeId, (nodeTunnelCount.get(nodeId) || 0) + 1);
    });
  });

  forwards.forEach((forward) => {
    if (!forward.tunnelId) {
      return;
    }

    const nodeIds = tunnelNodeMap.get(forward.tunnelId) || [];

    nodeIds.forEach((nodeId) => {
      nodeForwardCount.set(nodeId, (nodeForwardCount.get(nodeId) || 0) + 1);
    });
  });

  const systemFlowNodes: Node<VisualEntityNodeData>[] = systemNodes.map(
    (item, index) => {
      const visualId = getSystemVisualId(item.id);

      return {
        data: {
          kind: "system",
          meta: [
            `隧道 ${nodeTunnelCount.get(item.id) || 0}`,
            `规则 ${nodeForwardCount.get(item.id) || 0}`,
          ],
          statusLabel: item.status === 1 ? "在线" : "离线",
          subtitle:
            String(item.serverIp || "").trim() ||
            String(item.serverIpV4 || "").trim() ||
            String(item.serverIpV6 || "").trim() ||
            "未配置 IP",
          title: item.name,
        },
        id: visualId,
        position: positions[visualId] || { x: 80, y: 80 + index * 170 },
        selected: visualId === selectedNodeId,
        type: "entity",
      };
    },
  );

  const tunnelFlowNodes: Node<VisualEntityNodeData>[] = tunnels.map(
    (item, index) => {
      const visualId = buildTunnelNodeId(item);
      const entryCount = (item.inNodeId || []).filter((node) => node.nodeId > 0)
        .length;
      const exitCount = (item.outNodeId || []).filter((node) => node.nodeId > 0)
        .length;
      const hopCount = (item.chainNodes || []).filter((group) =>
        group.some((node) => node.nodeId > 0),
      ).length;

      return {
        data: {
          description:
            item.type === 2
              ? `入口 ${entryCount} / 跳点 ${hopCount} / 出口 ${exitCount}`
              : `入口 ${entryCount}`,
          kind: "tunnel",
          meta: [
            getTunnelTypeLabel(item.type),
            `倍率 ${item.trafficRatio || 1}x`,
            `流量 ${item.flow === 2 ? "双向" : "单向"}`,
          ],
          statusLabel: item.draftId ? "草稿" : getStatusLabel(item.status),
          subtitle: item.draftId ? "未同步到 tunnel 接口" : "同步到 tunnel 接口",
          title: item.name || "未命名隧道",
        },
        id: visualId,
        position: positions[visualId] || { x: 460, y: 80 + index * 200 },
        selected: visualId === selectedNodeId,
        type: "entity",
      };
    },
  );

  const forwardFlowNodes: Node<VisualEntityNodeData>[] = forwards.map(
    (item, index) => {
      const visualId = buildForwardNodeId(item);

      return {
        data: {
          description: buildAddressPreview(item.remoteAddr),
          kind: "forward",
          meta: [
            item.inPort ? `端口 ${item.inPort}` : "端口自动分配",
            item.tunnelName || "未关联隧道",
          ],
          statusLabel: item.draftId ? "草稿" : getStatusLabel(item.status),
          subtitle: item.draftId
            ? "未同步到 forward 接口"
            : "同步到 forward 接口",
          title: item.name || "未命名规则",
        },
        id: visualId,
        position: positions[visualId] || { x: 860, y: 80 + index * 160 },
        selected: visualId === selectedNodeId,
        type: "entity",
      };
    },
  );

  return [...systemFlowNodes, ...tunnelFlowNodes, ...forwardFlowNodes];
};

const buildGraphEdges = (
  systemNodes: NodeApiItem[],
  tunnels: TunnelEditorState[],
  forwards: ForwardEditorState[],
): Edge[] => {
  const existingNodeIds = new Set<string>([
    ...systemNodes.map((item) => getSystemVisualId(item.id)),
    ...tunnels.map((item) => buildTunnelNodeId(item)),
    ...forwards.map((item) => buildForwardNodeId(item)),
  ]);

  const result: Edge[] = [];
  const pushEdge = (
    id: string,
    source: string,
    target: string,
    label: string,
    color: string,
  ) => {
    if (!existingNodeIds.has(source) || !existingNodeIds.has(target)) {
      return;
    }

    result.push({
      animated: label === "规则",
      id,
      label,
      markerEnd: {
        type: MarkerType.ArrowClosed,
      },
      source,
      style: {
        stroke: color,
        strokeWidth: 2,
      },
      target,
      type: "smoothstep",
    });
  };

  tunnels.forEach((tunnel) => {
    const tunnelId = buildTunnelNodeId(tunnel);

    (tunnel.inNodeId || []).forEach((item, index) => {
      if (item.nodeId <= 0) {
        return;
      }

      pushEdge(
        `${tunnelId}:entry:${item.nodeId}:${index}`,
        getSystemVisualId(item.nodeId),
        tunnelId,
        tunnel.inNodeId.length > 1 ? `入口 ${index + 1}` : "入口",
        "#16a34a",
      );
    });

    (tunnel.chainNodes || []).forEach((group, groupIndex) => {
      group.forEach((item) => {
        if (item.nodeId <= 0) {
          return;
        }

        pushEdge(
          `${tunnelId}:hop:${groupIndex + 1}:${item.nodeId}`,
          getSystemVisualId(item.nodeId),
          tunnelId,
          `跳 ${groupIndex + 1}`,
          "#0284c7",
        );
      });
    });

    (tunnel.outNodeId || []).forEach((item, index) => {
      if (item.nodeId <= 0) {
        return;
      }

      pushEdge(
        `${tunnelId}:exit:${item.nodeId}:${index}`,
        tunnelId,
        getSystemVisualId(item.nodeId),
        tunnel.outNodeId.length > 1 ? `出口 ${index + 1}` : "出口",
        "#0f766e",
      );
    });
  });

  forwards.forEach((forward) => {
    if (!forward.tunnelId) {
      return;
    }

    pushEdge(
      `${buildForwardNodeId(forward)}:rule`,
      getTunnelVisualId(forward.tunnelId),
      buildForwardNodeId(forward),
      "规则",
      "#d97706",
    );
  });

  return result;
};

const validateForwardEditor = (
  editor: ForwardEditorState,
): Record<string, string> => {
  const nextErrors: Record<string, string> = {};

  if (!editor.name.trim()) {
    nextErrors.name = "请输入规则名称";
  }

  if (!editor.tunnelId || editor.tunnelId <= 0) {
    nextErrors.tunnelId = "请选择关联隧道";
  }

  if (
    editor.inPort !== null &&
    (!Number.isInteger(editor.inPort) ||
      editor.inPort <= 0 ||
      editor.inPort > 65535)
  ) {
    nextErrors.inPort = "端口必须在 1-65535 之间";
  }

  if (splitMultiValue(editor.remoteAddr).length === 0) {
    nextErrors.remoteAddr = "请输入目标地址";
  }

  return nextErrors;
};

export default function VisualEditor() {
  const [systemNodes, setSystemNodes] = useState<NodeApiItem[]>([]);
  const [tunnels, setTunnels] = useState<TunnelEditorState[]>([]);
  const [forwards, setForwards] = useState<ForwardEditorState[]>([]);
  const [draftTunnels, setDraftTunnels] = useState<TunnelEditorState[]>([]);
  const [draftForwards, setDraftForwards] = useState<ForwardEditorState[]>([]);
  const [savedPositions, setSavedPositions] = useState<
    Record<string, XYPosition>
  >({});
  const [nodes, setNodes, onNodesChange] =
    useNodesState<VisualEntityNodeData>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);
  const [loading, setLoading] = useState(true);
  const [layoutSaving, setLayoutSaving] = useState(false);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [probeLoading, setProbeLoading] = useState(false);
  const [probeData, setProbeData] = useState<VisualProbeData | null>(null);
  const [tunnelEditor, setTunnelEditor] = useState<TunnelEditorState | null>(
    null,
  );
  const [forwardEditor, setForwardEditor] = useState<ForwardEditorState | null>(
    null,
  );
  const [tunnelErrors, setTunnelErrors] = useState<Record<string, string>>({});
  const [forwardErrors, setForwardErrors] = useState<Record<string, string>>(
    {},
  );
  const [syncing, setSyncing] = useState(false);

  const allTunnels = useMemo(
    () => [...tunnels, ...draftTunnels],
    [draftTunnels, tunnels],
  );
  const allForwards = useMemo(
    () => [...forwards, ...draftForwards],
    [draftForwards, forwards],
  );

  const selectedSystemNodeId = useMemo(() => {
    if (!isSystemNodeId(selectedNodeId)) {
      return null;
    }

    return extractVisualNumericId(selectedNodeId, SYSTEM_PREFIX);
  }, [selectedNodeId]);

  const selectedSystemNode = useMemo(() => {
    if (!selectedSystemNodeId) {
      return null;
    }

    return systemNodes.find((item) => item.id === selectedSystemNodeId) || null;
  }, [selectedSystemNodeId, systemNodes]);

  const relatedTunnelsForSystem = useMemo(() => {
    if (!selectedSystemNodeId) {
      return [];
    }

    return allTunnels.filter((item) =>
      getTunnelNodeIds(item).includes(selectedSystemNodeId),
    );
  }, [allTunnels, selectedSystemNodeId]);

  const relatedTunnelIdSet = useMemo(
    () =>
      new Set(
        relatedTunnelsForSystem
          .map((item) => item.id)
          .filter((item): item is number => typeof item === "number" && item > 0),
      ),
    [relatedTunnelsForSystem],
  );

  const relatedForwardsForSystem = useMemo(() => {
    if (!selectedSystemNodeId) {
      return [];
    }

    return allForwards.filter(
      (item) =>
        typeof item.tunnelId === "number" && relatedTunnelIdSet.has(item.tunnelId),
    );
  }, [allForwards, relatedTunnelIdSet, selectedSystemNodeId]);

  const fetchSnapshot = useCallback(
    async (reloadLayout: boolean): Promise<VisualSnapshot> => {
      const [nodesRes, tunnelsRes, forwardsRes, layoutRes] = await Promise.all([
        getNodeList(),
        getTunnelList(),
        getForwardList(),
        reloadLayout
          ? Network.get<any>("/nodes/graph").catch(() => null)
          : Promise.resolve(null),
      ]);

      if (nodesRes.code !== 0) {
        throw new Error(nodesRes.msg || "加载节点失败");
      }

      if (tunnelsRes.code !== 0) {
        throw new Error(tunnelsRes.msg || "加载隧道失败");
      }

      if (forwardsRes.code !== 0) {
        throw new Error(forwardsRes.msg || "加载规则失败");
      }

      return {
        forwards: mapForwardApiItems((forwardsRes.data || []) as ForwardApiItem[]),
        positions:
          reloadLayout && layoutRes?.code === 0
            ? parseSavedPositions(layoutRes.data?.graph_json)
            : undefined,
        systemNodes: (nodesRes.data || []) as NodeApiItem[],
        tunnels: mapTunnelApiItems((tunnelsRes.data || []) as TunnelApiItem[]),
      };
    },
    [],
  );

  const applySnapshot = useCallback(
    (snapshot: VisualSnapshot, reloadLayout: boolean) => {
      setSystemNodes(snapshot.systemNodes);
      setTunnels(snapshot.tunnels);
      setForwards(snapshot.forwards);

      if (reloadLayout) {
        setSavedPositions(snapshot.positions || {});
      }
    },
    [],
  );

  const loadData = useCallback(
    async (
      options: {
        reloadLayout?: boolean;
        silent?: boolean;
      } = {},
    ) => {
      const reloadLayout = options.reloadLayout !== false;

      if (!options.silent) {
        setLoading(true);
      }

      try {
        const snapshot = await fetchSnapshot(reloadLayout);

        applySnapshot(snapshot, reloadLayout);
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : "加载可视化数据失败",
        );
      } finally {
        if (!options.silent) {
          setLoading(false);
        }
      }
    },
    [applySnapshot, fetchSnapshot],
  );

  useEffect(() => {
    void loadData();
  }, [loadData]);

  useEffect(() => {
    const nextNodes = buildGraphNodes(
      systemNodes,
      allTunnels,
      allForwards,
      savedPositions,
      selectedNodeId,
    );
    const nextEdges = buildGraphEdges(systemNodes, allTunnels, allForwards);

    setNodes(nextNodes);
    setEdges(nextEdges);
  }, [
    allForwards,
    allTunnels,
    savedPositions,
    selectedNodeId,
    setEdges,
    setNodes,
    systemNodes,
  ]);

  useEffect(() => {
    setTunnelErrors({});
    setForwardErrors({});

    if (!selectedNodeId) {
      setTunnelEditor(null);
      setForwardEditor(null);
      return;
    }

    if (isTunnelNodeId(selectedNodeId)) {
      const tunnelId = extractVisualNumericId(selectedNodeId, TUNNEL_PREFIX);
      const draftId = extractDraftId(selectedNodeId, TUNNEL_DRAFT_PREFIX);
      const match =
        (typeof tunnelId === "number"
          ? allTunnels.find((item) => item.id === tunnelId)
          : undefined) ||
        (draftId
          ? draftTunnels.find((item) => item.draftId === draftId)
          : undefined);

      setForwardEditor(null);
      setTunnelEditor(match ? tunnelToEditor(match) : null);
      return;
    }

    if (isForwardNodeId(selectedNodeId)) {
      const forwardId = extractVisualNumericId(selectedNodeId, FORWARD_PREFIX);
      const draftId = extractDraftId(selectedNodeId, FORWARD_DRAFT_PREFIX);
      const match =
        (typeof forwardId === "number"
          ? allForwards.find((item) => item.id === forwardId)
          : undefined) ||
        (draftId
          ? draftForwards.find((item) => item.draftId === draftId)
          : undefined);

      setTunnelEditor(null);
      setForwardEditor(match ? forwardToEditor(match) : null);
      return;
    }

    setTunnelEditor(null);
    setForwardEditor(null);
  }, [allForwards, allTunnels, draftForwards, draftTunnels, selectedNodeId]);

  useEffect(() => {
    if (!selectedSystemNodeId) {
      setProbeData(null);
      return;
    }

    let active = true;

    setProbeLoading(true);
    void Network.post<VisualProbeData>(`/probe/node/${selectedSystemNodeId}`)
      .then((response) => {
        if (!active) {
          return;
        }

        if (response.code === 0 && response.data) {
          setProbeData(response.data);
          return;
        }

        setProbeData(null);
      })
      .catch(() => {
        if (active) {
          setProbeData(null);
        }
      })
      .finally(() => {
        if (active) {
          setProbeLoading(false);
        }
      });

    return () => {
      active = false;
    };
  }, [selectedSystemNodeId]);

  const updateTunnelEditor = useCallback(
    (updater: (current: TunnelEditorState) => TunnelEditorState) => {
      setTunnelEditor((current) => {
        if (!current) {
          return current;
        }

        const next = updater(current);

        if (current.draftId) {
          setDraftTunnels((items) =>
            items.map((item) =>
              item.draftId === current.draftId ? editorToTunnelState(next) : item,
            ),
          );
        }

        return next;
      });
    },
    [],
  );

  const updateForwardEditor = useCallback(
    (updater: (current: ForwardEditorState) => ForwardEditorState) => {
      setForwardEditor((current) => {
        if (!current) {
          return current;
        }

        const next = updater(current);

        if (current.draftId) {
          setDraftForwards((items) =>
            items.map((item) =>
              item.draftId === current.draftId ? editorToForwardState(next) : item,
            ),
          );
        }

        return next;
      });
    },
    [],
  );

  const handleSaveLayout = useCallback(async () => {
    setLayoutSaving(true);

    try {
      const graphJSON = serializeLayout(nodes);
      const response = await Network.post<any>("/nodes/graph", {
        graph_json: graphJSON,
      });

      if (response.code === 0) {
        setSavedPositions(parseSavedPositions(graphJSON));
        toast.success("可视化布局已保存");
      } else {
        toast.error(response.msg || "保存布局失败");
      }
    } catch {
      toast.error("保存布局失败");
    } finally {
      setLayoutSaving(false);
    }
  }, [nodes]);

  const handleAddTunnel = useCallback(() => {
    const draftId = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;

    setDraftTunnels((items) => [...items, createDraftTunnel(draftId)]);
    setSelectedNodeId(getTunnelDraftVisualId(draftId));
  }, []);

  const handleAddForward = useCallback(() => {
    const draftId = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;

    setDraftForwards((items) => [...items, createDraftForward(draftId)]);
    setSelectedNodeId(getForwardDraftVisualId(draftId));
  }, []);

  const removeSelectedDraftTunnel = useCallback(() => {
    if (!tunnelEditor?.draftId) {
      return;
    }

    const draftId = tunnelEditor.draftId;

    setDraftTunnels((items) => items.filter((item) => item.draftId !== draftId));
    setSelectedNodeId(null);
  }, [tunnelEditor]);

  const removeSelectedDraftForward = useCallback(() => {
    if (!forwardEditor?.draftId) {
      return;
    }

    const draftId = forwardEditor.draftId;

    setDraftForwards((items) =>
      items.filter((item) => item.draftId !== draftId),
    );
    setSelectedNodeId(null);
  }, [forwardEditor]);

  const handleTunnelSave = useCallback(async () => {
    if (!tunnelEditor) {
      return;
    }

    const validationErrors = validateTunnelForm(
      tunnelEditor,
      systemNodes.map((item) => ({ id: item.id, status: item.status })),
    );

    setTunnelErrors(validationErrors);

    if (Object.keys(validationErrors).length > 0) {
      return;
    }

    setSyncing(true);

    try {
      const payload: TunnelMutationPayload = {
        ...tunnelEditor,
        chainNodes: (tunnelEditor.chainNodes || [])
          .map((group) => group.filter((item) => item.nodeId > 0))
          .filter((group) => group.length > 0),
        inIp: multilineToComma(tunnelEditor.inIp),
        inNodeId: (tunnelEditor.inNodeId || []).filter((item) => item.nodeId > 0),
        outNodeId: (tunnelEditor.outNodeId || []).filter(
          (item) => item.nodeId > 0,
        ),
      };

      const response =
        typeof tunnelEditor.id === "number" && tunnelEditor.id > 0
          ? await updateTunnel(payload)
          : await createTunnel(payload);

      if (response.code !== 0) {
        toast.error(
          response.msg ||
            (tunnelEditor.id ? "同步隧道失败" : "创建隧道失败"),
        );
        return;
      }

      const currentVisualId = buildTunnelNodeId(tunnelEditor);
      const currentPosition = nodes.find((item) => item.id === currentVisualId)
        ?.position;
      const snapshot = await fetchSnapshot(false);

      applySnapshot(snapshot, false);

      if (tunnelEditor.draftId) {
        setDraftTunnels((items) =>
          items.filter((item) => item.draftId !== tunnelEditor.draftId),
        );
      }

      let nextSelectedId: string | null =
        tunnelEditor.id && tunnelEditor.id > 0
          ? getTunnelVisualId(tunnelEditor.id)
          : null;

      if (!nextSelectedId) {
        const created = snapshot.tunnels.find(
          (item) => item.name.trim() === tunnelEditor.name.trim(),
        );

        if (created?.id) {
          nextSelectedId = getTunnelVisualId(created.id);
        }
      }

      if (nextSelectedId && currentPosition) {
        setSavedPositions((items) => ({
          ...items,
          [nextSelectedId]: currentPosition,
        }));
      }

      setSelectedNodeId(nextSelectedId);
      toast.success(tunnelEditor.id ? "隧道已同步" : "隧道已创建");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "同步隧道失败");
    } finally {
      setSyncing(false);
    }
  }, [applySnapshot, fetchSnapshot, nodes, systemNodes, tunnelEditor]);

  const handleForwardSave = useCallback(async () => {
    if (!forwardEditor) {
      return;
    }

    const validationErrors = validateForwardEditor(forwardEditor);

    setForwardErrors(validationErrors);

    if (Object.keys(validationErrors).length > 0) {
      return;
    }

    setSyncing(true);

    try {
      const payload: ForwardMutationPayload = {
        id: forwardEditor.id,
        inIp: forwardEditor.inIp || "",
        inPort: forwardEditor.inPort,
        name: forwardEditor.name,
        remoteAddr: multilineToComma(forwardEditor.remoteAddr),
        strategy:
          splitMultiValue(forwardEditor.remoteAddr).length > 1
            ? forwardEditor.strategy
            : "fifo",
        tunnelId: forwardEditor.tunnelId,
      };

      const response =
        typeof forwardEditor.id === "number" && forwardEditor.id > 0
          ? await updateForward(payload)
          : await createForward(payload);

      if (response.code !== 0) {
        toast.error(
          response.msg ||
            (forwardEditor.id ? "同步规则失败" : "创建规则失败"),
        );
        return;
      }

      const currentVisualId = buildForwardNodeId(forwardEditor);
      const currentPosition = nodes.find((item) => item.id === currentVisualId)
        ?.position;
      const normalizedRemote = normalizeAddressSignature(forwardEditor.remoteAddr);
      const snapshot = await fetchSnapshot(false);

      applySnapshot(snapshot, false);

      if (forwardEditor.draftId) {
        setDraftForwards((items) =>
          items.filter((item) => item.draftId !== forwardEditor.draftId),
        );
      }

      let nextSelectedId: string | null =
        forwardEditor.id && forwardEditor.id > 0
          ? getForwardVisualId(forwardEditor.id)
          : null;

      if (!nextSelectedId) {
        const candidates = snapshot.forwards
          .filter(
            (item) =>
              item.name.trim() === forwardEditor.name.trim() &&
              item.tunnelId === forwardEditor.tunnelId,
          )
          .sort((a, b) => (b.id || 0) - (a.id || 0));

        const created =
          candidates.find(
            (item) =>
              normalizeAddressSignature(item.remoteAddr) === normalizedRemote,
          ) || candidates[0];

        if (created?.id) {
          nextSelectedId = getForwardVisualId(created.id);
        }
      }

      if (nextSelectedId && currentPosition) {
        setSavedPositions((items) => ({
          ...items,
          [nextSelectedId]: currentPosition,
        }));
      }

      setSelectedNodeId(nextSelectedId);
      toast.success(forwardEditor.id ? "规则已同步" : "规则已创建");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "同步规则失败");
    } finally {
      setSyncing(false);
    }
  }, [applySnapshot, fetchSnapshot, forwardEditor, nodes]);

  const renderSystemInspector = () => {
    if (!selectedSystemNode) {
      return null;
    }

    return (
      <div className="space-y-4">
        <div className="flex flex-wrap gap-2">
          <Chip color={selectedSystemNode.status === 1 ? "success" : "default"}>
            {selectedSystemNode.status === 1 ? "在线" : "离线"}
          </Chip>
          <Chip variant="flat">
            {String(selectedSystemNode.serverIp || "").trim() || "未配置 IP"}
          </Chip>
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div className="rounded-xl border border-default-200 bg-default-50 p-3 dark:bg-zinc-900">
            <div className="text-xs text-default-500">CPU</div>
            <div className="mt-1 text-lg font-semibold">
              {probeLoading
                ? "..."
                : `${Number(probeData?.cpu_usage || 0).toFixed(1)}%`}
            </div>
          </div>
          <div className="rounded-xl border border-default-200 bg-default-50 p-3 dark:bg-zinc-900">
            <div className="text-xs text-default-500">内存</div>
            <div className="mt-1 text-lg font-semibold">
              {probeLoading
                ? "..."
                : `${Number(probeData?.mem_usage || 0).toFixed(1)}%`}
            </div>
          </div>
          <div className="rounded-xl border border-default-200 bg-default-50 p-3 dark:bg-zinc-900">
            <div className="text-xs text-default-500">连接数</div>
            <div className="mt-1 text-lg font-semibold">
              {probeLoading ? "..." : probeData?.connections || 0}
            </div>
          </div>
          <div className="rounded-xl border border-default-200 bg-default-50 p-3 dark:bg-zinc-900">
            <div className="text-xs text-default-500">端口范围</div>
            <div className="mt-1 text-sm font-medium">
              {String(probeData?.allowed_ports || "未配置")}
            </div>
          </div>
        </div>

        <div className="rounded-xl border border-default-200 p-4">
          <div className="text-sm font-semibold">占用端口</div>
          <div className="mt-3 flex flex-wrap gap-2">
            {(probeData?.occupied_ports || []).length > 0 ? (
              probeData?.occupied_ports?.map((port) => (
                <span
                  key={port}
                  className="rounded-full bg-default-100 px-2 py-1 text-xs text-default-700 dark:bg-zinc-900"
                >
                  {port}
                </span>
              ))
            ) : (
              <span className="text-xs text-default-500">当前没有占用端口</span>
            )}
          </div>
        </div>

        <div className="rounded-xl border border-default-200 p-4">
          <div className="text-sm font-semibold">关联隧道</div>
          <div className="mt-3 space-y-2">
            {relatedTunnelsForSystem.length > 0 ? (
              relatedTunnelsForSystem.map((item) => (
                <button
                  key={buildTunnelNodeId(item)}
                  className="w-full rounded-lg border border-default-200 px-3 py-2 text-left text-sm transition-colors hover:border-primary hover:bg-default-50 dark:hover:bg-zinc-900"
                  onClick={() => setSelectedNodeId(buildTunnelNodeId(item))}
                >
                  <div className="font-medium">{item.name}</div>
                  <div className="mt-1 text-xs text-default-500">
                    {getTunnelTypeLabel(item.type)}
                  </div>
                </button>
              ))
            ) : (
              <div className="text-sm text-default-500">没有关联隧道</div>
            )}
          </div>
        </div>

        <div className="rounded-xl border border-default-200 p-4">
          <div className="text-sm font-semibold">关联规则</div>
          <div className="mt-3 space-y-2">
            {relatedForwardsForSystem.length > 0 ? (
              relatedForwardsForSystem.map((item) => (
                <button
                  key={buildForwardNodeId(item)}
                  className="w-full rounded-lg border border-default-200 px-3 py-2 text-left text-sm transition-colors hover:border-primary hover:bg-default-50 dark:hover:bg-zinc-900"
                  onClick={() => setSelectedNodeId(buildForwardNodeId(item))}
                >
                  <div className="font-medium">{item.name}</div>
                  <div className="mt-1 text-xs text-default-500">
                    {buildAddressPreview(item.remoteAddr)}
                  </div>
                </button>
              ))
            ) : (
              <div className="text-sm text-default-500">没有关联规则</div>
            )}
          </div>
        </div>
      </div>
    );
  };
  const renderTunnelInspector = () => {
    if (!tunnelEditor) {
      return null;
    }

    const selectedChainNodeIds = getSelectedChainNodeIds(tunnelEditor);
    const relatedForwards = tunnelEditor.id
      ? forwards.filter((item) => item.tunnelId === tunnelEditor.id)
      : [];

    return (
      <div className="space-y-4">
        <div className="rounded-xl border border-sky-200 bg-sky-50/80 p-4 text-sm text-sky-700 dark:border-sky-900 dark:bg-sky-950/30 dark:text-sky-200">
          保存时继续走原有 `tunnel/create` 和 `tunnel/update` 接口。
        </div>

        <div className="flex flex-wrap gap-2">
          <Chip color={tunnelEditor.draftId ? "warning" : "primary"}>
            {tunnelEditor.draftId ? "草稿" : getStatusLabel(tunnelEditor.status)}
          </Chip>
          <Chip variant="flat">{getTunnelTypeLabel(tunnelEditor.type)}</Chip>
          <Chip variant="flat">关联规则 {relatedForwards.length}</Chip>
        </div>

        <Input
          errorMessage={tunnelErrors.name}
          isInvalid={Boolean(tunnelErrors.name)}
          label="隧道名称"
          value={tunnelEditor.name}
          variant="bordered"
          onChange={(event) =>
            updateTunnelEditor((current) => ({
              ...current,
              name: event.target.value,
            }))
          }
        />

        <div className="grid grid-cols-2 gap-3">
          <Select
            isDisabled={Boolean(tunnelEditor.id)}
            label="隧道类型"
            selectedKeys={[String(tunnelEditor.type)]}
            variant="bordered"
            onSelectionChange={(keys) => {
              const [value] = selectionToStrings(keys);
              const nextType = value === "2" ? 2 : 1;

              updateTunnelEditor((current) => ({
                ...current,
                chainNodes: nextType === 2 ? current.chainNodes : [],
                ipPreference: nextType === 2 ? current.ipPreference : "",
                outNodeId: nextType === 2 ? current.outNodeId : [],
                type: nextType,
              }));
            }}
          >
            <SelectItem key="1">端口转发</SelectItem>
            <SelectItem key="2">隧道转发</SelectItem>
          </Select>

          <Select
            label="状态"
            selectedKeys={[String(tunnelEditor.status)]}
            variant="bordered"
            onSelectionChange={(keys) => {
              const [value] = selectionToStrings(keys);

              updateTunnelEditor((current) => ({
                ...current,
                status: value === "0" ? 0 : 1,
              }));
            }}
          >
            <SelectItem key="1">启用</SelectItem>
            <SelectItem key="0">停用</SelectItem>
          </Select>

          <Select
            label="流量计算"
            selectedKeys={[String(tunnelEditor.flow)]}
            variant="bordered"
            onSelectionChange={(keys) => {
              const [value] = selectionToStrings(keys);

              updateTunnelEditor((current) => ({
                ...current,
                flow: value === "2" ? 2 : 1,
              }));
            }}
          >
            <SelectItem key="1">单向</SelectItem>
            <SelectItem key="2">双向</SelectItem>
          </Select>

          <Input
            errorMessage={tunnelErrors.trafficRatio}
            isInvalid={Boolean(tunnelErrors.trafficRatio)}
            label="流量倍率"
            step="any"
            type="number"
            value={String(tunnelEditor.trafficRatio)}
            variant="bordered"
            onChange={(event) =>
              updateTunnelEditor((current) => ({
                ...current,
                trafficRatio: Number.parseFloat(event.target.value) || 0,
              }))
            }
          />
        </div>

        <Textarea
          description="一行一个入口 IP，留空则由系统自动生成。"
          label="入口 IP"
          maxRows={5}
          minRows={3}
          value={tunnelEditor.inIp}
          variant="bordered"
          onChange={(event) =>
            updateTunnelEditor((current) => ({
              ...current,
              inIp: event.target.value,
            }))
          }
        />

        {tunnelEditor.type === 2 ? (
          <Select
            label="连接地址偏好"
            selectedKeys={
              tunnelEditor.ipPreference ? [tunnelEditor.ipPreference] : []
            }
            variant="bordered"
            onSelectionChange={(keys) => {
              const [value] = selectionToStrings(keys);

              updateTunnelEditor((current) => ({
                ...current,
                ipPreference: value || "",
              }));
            }}
          >
            <SelectItem key="v4">优先 IPv4</SelectItem>
            <SelectItem key="v6">优先 IPv6</SelectItem>
          </Select>
        ) : null}

        <Divider />

        <Select
          disabledKeys={[
            ...systemNodes
              .filter((item) => item.status !== 1)
              .map((item) => String(item.id)),
            ...(tunnelEditor.outNodeId || [])
              .filter((item) => item.nodeId > 0)
              .map((item) => String(item.nodeId)),
            ...selectedChainNodeIds.map((item) => String(item)),
          ]}
          errorMessage={tunnelErrors.inNodeId}
          isInvalid={Boolean(tunnelErrors.inNodeId)}
          label="入口节点"
          selectedKeys={(tunnelEditor.inNodeId || []).map((item) =>
            String(item.nodeId),
          )}
          selectionMode="multiple"
          variant="bordered"
          onSelectionChange={(keys) => {
            const selectedIds = selectionToNodeIds(keys);

            updateTunnelEditor((current) => ({
              ...current,
              inNodeId: mergeOrderedNodes(
                current.inNodeId || [],
                selectedIds,
                (nodeId) => ({ chainType: 1, nodeId }),
              ),
            }));
          }}
        >
          {systemNodes.map((item) => (
            <SelectItem key={item.id} textValue={item.name}>
              {item.name}
            </SelectItem>
          ))}
        </Select>

        {tunnelEditor.type === 2 ? (
          <>
            <Divider />

            <div className="flex items-center justify-between">
              <div className="text-sm font-semibold">转发链</div>
              <Button
                color="primary"
                size="sm"
                variant="flat"
                onPress={() =>
                  updateTunnelEditor((current) => ({
                    ...current,
                    chainNodes: [
                      ...(current.chainNodes || []),
                      [
                        {
                          chainType: 2,
                          connectIp: "",
                          nodeId: -1,
                          protocol: "tls",
                          strategy: "round",
                        },
                      ],
                    ],
                  }))
                }
              >
                添加跳点
              </Button>
            </div>

            {(tunnelEditor.chainNodes || []).length > 0 ? (
              <div className="space-y-3">
                {(tunnelEditor.chainNodes || []).map((group, groupIndex) => {
                  const selectedGroupNodeIds = group
                    .filter((item) => item.nodeId > 0)
                    .map((item) => item.nodeId);
                  const groupProtocol = group[0]?.protocol || "tls";
                  const groupStrategy = group[0]?.strategy || "round";
                  const groupConnectIp = group[0]?.connectIp || "";
                  const commonIpOptions = getCommonIpOptions(
                    systemNodes,
                    selectedGroupNodeIds,
                  );
                  const isMultiGroup = selectedGroupNodeIds.length > 1;
                  const otherGroupIds = (tunnelEditor.chainNodes || [])
                    .flatMap((items, index) =>
                      index === groupIndex
                        ? []
                        : items.map((item) => item.nodeId).filter((id) => id > 0),
                    )
                    .map((item) => String(item));

                  return (
                    <div
                      key={`chain-${groupIndex}`}
                      className="rounded-xl border border-default-200 p-4"
                    >
                      <div className="mb-3 flex items-center justify-between">
                        <div className="font-medium">跳点 {groupIndex + 1}</div>
                        <Button
                          color="danger"
                          size="sm"
                          variant="light"
                          onPress={() =>
                            updateTunnelEditor((current) => ({
                              ...current,
                              chainNodes: (current.chainNodes || []).filter(
                                (_, index) => index !== groupIndex,
                              ),
                            }))
                          }
                        >
                          删除
                        </Button>
                      </div>

                      <div className="space-y-3">
                        <Select
                          disabledKeys={[
                            ...systemNodes
                              .filter((item) => item.status !== 1)
                              .map((item) => String(item.id)),
                            ...(tunnelEditor.inNodeId || []).map((item) =>
                              String(item.nodeId),
                            ),
                            ...(tunnelEditor.outNodeId || [])
                              .filter((item) => item.nodeId > 0)
                              .map((item) => String(item.nodeId)),
                            ...otherGroupIds,
                          ]}
                          label="跳点节点"
                          selectedKeys={selectedGroupNodeIds.map((item) =>
                            String(item),
                          )}
                          selectionMode="multiple"
                          variant="bordered"
                          onSelectionChange={(keys) => {
                            const selectedIds = selectionToNodeIds(keys);

                            updateTunnelEditor((current) => {
                              const nextGroups = [...(current.chainNodes || [])];
                              const currentGroup = nextGroups[groupIndex] || [];
                              const currentProtocol =
                                currentGroup[0]?.protocol || "tls";
                              const currentStrategy =
                                currentGroup[0]?.strategy || "round";
                              const currentConnectIp =
                                currentGroup[0]?.connectIp || "";
                              const realNodes = currentGroup.filter(
                                (item) => item.nodeId > 0,
                              );
                              const mergedNodes = mergeOrderedNodes(
                                realNodes,
                                selectedIds,
                                (nodeId) => ({
                                  chainType: 2,
                                  connectIp: currentConnectIp,
                                  nodeId,
                                  protocol: currentProtocol,
                                  strategy: currentStrategy,
                                }),
                              );

                              nextGroups[groupIndex] =
                                mergedNodes.length > 0
                                  ? mergedNodes
                                  : [
                                      {
                                        chainType: 2,
                                        connectIp: currentConnectIp,
                                        nodeId: -1,
                                        protocol: currentProtocol,
                                        strategy: currentStrategy,
                                      },
                                    ];

                              return {
                                ...current,
                                chainNodes: nextGroups,
                              };
                            });
                          }}
                        >
                          {systemNodes.map((item) => (
                            <SelectItem key={item.id} textValue={item.name}>
                              {item.name}
                            </SelectItem>
                          ))}
                        </Select>

                        <div className="grid grid-cols-2 gap-3">
                          <Select
                            label="协议"
                            selectedKeys={[groupProtocol]}
                            variant="bordered"
                            onSelectionChange={(keys) => {
                              const [value] = selectionToStrings(keys);

                              updateTunnelEditor((current) => {
                                const nextGroups = [...(current.chainNodes || [])];

                                nextGroups[groupIndex] = (
                                  nextGroups[groupIndex] || []
                                ).map((item) => ({
                                  ...item,
                                  protocol: value || "tls",
                                }));

                                return {
                                  ...current,
                                  chainNodes: nextGroups,
                                };
                              });
                            }}
                          >
                            {TUNNEL_PROTOCOL_OPTIONS.map((item) => (
                              <SelectItem key={item}>{item.toUpperCase()}</SelectItem>
                            ))}
                          </Select>

                          <Select
                            label="策略"
                            selectedKeys={[groupStrategy]}
                            variant="bordered"
                            onSelectionChange={(keys) => {
                              const [value] = selectionToStrings(keys);

                              updateTunnelEditor((current) => {
                                const nextGroups = [...(current.chainNodes || [])];

                                nextGroups[groupIndex] = (
                                  nextGroups[groupIndex] || []
                                ).map((item) => ({
                                  ...item,
                                  strategy: value || "round",
                                }));

                                return {
                                  ...current,
                                  chainNodes: nextGroups,
                                };
                              });
                            }}
                          >
                            {TUNNEL_STRATEGY_OPTIONS.map((item) => (
                              <SelectItem key={item}>{item}</SelectItem>
                            ))}
                          </Select>
                        </div>

                        <Select
                          isDisabled={
                            selectedGroupNodeIds.length === 0 ||
                            commonIpOptions.length === 0 ||
                            isMultiGroup
                          }
                          label="连接 IP"
                          placeholder={
                            isMultiGroup
                              ? "多节点跳点不支持自定义 IP"
                              : "默认连接 IP"
                          }
                          selectedKeys={[groupConnectIp || "__default__"]}
                          variant="bordered"
                          onSelectionChange={(keys) => {
                            const [value] = selectionToStrings(keys);
                            const connectIp =
                              value === "__default__" ? "" : value || "";

                            updateTunnelEditor((current) => {
                              const nextGroups = [...(current.chainNodes || [])];

                              nextGroups[groupIndex] = (
                                nextGroups[groupIndex] || []
                              ).map((item) => ({
                                ...item,
                                connectIp,
                              }));

                              return {
                                ...current,
                                chainNodes: nextGroups,
                              };
                            });
                          }}
                        >
                          <SelectItem key="__default__">默认连接 IP</SelectItem>
                          {commonIpOptions.map((item) => (
                            <SelectItem key={item}>{item}</SelectItem>
                          ))}
                        </Select>
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : (
              <div className="rounded-xl border border-dashed border-default-300 p-4 text-sm text-default-500">
                当前还没有转发链，隧道转发模式下可以按跳点继续补充。
              </div>
            )}

            <Divider />

            {(() => {
              const selectedOutNodeIds = (tunnelEditor.outNodeId || [])
                .filter((item) => item.nodeId > 0)
                .map((item) => item.nodeId);
              const outProtocol = tunnelEditor.outNodeId?.[0]?.protocol || "tls";
              const outStrategy = tunnelEditor.outNodeId?.[0]?.strategy || "round";
              const outConnectIp =
                tunnelEditor.outNodeId?.[0]?.connectIp || "";
              const commonOutIpOptions = getCommonIpOptions(
                systemNodes,
                selectedOutNodeIds,
              );
              const isMultiExit = selectedOutNodeIds.length > 1;

              return (
                <div className="space-y-3">
                  <div className="text-sm font-semibold">出口节点</div>
                  <Select
                    disabledKeys={[
                      ...systemNodes
                        .filter((item) => item.status !== 1)
                        .map((item) => String(item.id)),
                      ...(tunnelEditor.inNodeId || []).map((item) =>
                        String(item.nodeId),
                      ),
                      ...selectedChainNodeIds.map((item) => String(item)),
                    ]}
                    errorMessage={tunnelErrors.outNodeId}
                    isInvalid={Boolean(tunnelErrors.outNodeId)}
                    label="出口节点"
                    selectedKeys={selectedOutNodeIds.map((item) => String(item))}
                    selectionMode="multiple"
                    variant="bordered"
                    onSelectionChange={(keys) => {
                      const selectedIds = selectionToNodeIds(keys);

                      updateTunnelEditor((current) => {
                        const currentOutNodes = current.outNodeId || [];
                        const currentProtocol =
                          currentOutNodes[0]?.protocol || "tls";
                        const currentStrategy =
                          currentOutNodes[0]?.strategy || "round";
                        const currentConnectIp =
                          currentOutNodes[0]?.connectIp || "";
                        const realNodes = currentOutNodes.filter(
                          (item) => item.nodeId > 0,
                        );

                        return {
                          ...current,
                          outNodeId: mergeOrderedNodes(
                            realNodes,
                            selectedIds,
                            (nodeId) => ({
                              chainType: 3,
                              connectIp: currentConnectIp,
                              nodeId,
                              protocol: currentProtocol,
                              strategy: currentStrategy,
                            }),
                          ),
                        };
                      });
                    }}
                  >
                    {systemNodes.map((item) => (
                      <SelectItem key={item.id} textValue={item.name}>
                        {item.name}
                      </SelectItem>
                    ))}
                  </Select>

                  <div className="grid grid-cols-2 gap-3">
                    <Select
                      label="出口协议"
                      selectedKeys={[outProtocol]}
                      variant="bordered"
                      onSelectionChange={(keys) => {
                        const [value] = selectionToStrings(keys);

                        updateTunnelEditor((current) => {
                          const currentOutNodes = current.outNodeId || [];
                          const nextNodes =
                            currentOutNodes.length > 0
                              ? currentOutNodes.map((item) => ({
                                  ...item,
                                  protocol: value || "tls",
                                }))
                              : [
                                  {
                                    chainType: 3,
                                    connectIp: "",
                                    nodeId: -1,
                                    protocol: value || "tls",
                                    strategy: "round",
                                  },
                                ];

                          return {
                            ...current,
                            outNodeId: nextNodes,
                          };
                        });
                      }}
                    >
                      {TUNNEL_PROTOCOL_OPTIONS.map((item) => (
                        <SelectItem key={item}>{item.toUpperCase()}</SelectItem>
                      ))}
                    </Select>

                    <Select
                      label="出口策略"
                      selectedKeys={[outStrategy]}
                      variant="bordered"
                      onSelectionChange={(keys) => {
                        const [value] = selectionToStrings(keys);

                        updateTunnelEditor((current) => {
                          const currentOutNodes = current.outNodeId || [];
                          const nextNodes =
                            currentOutNodes.length > 0
                              ? currentOutNodes.map((item) => ({
                                  ...item,
                                  strategy: value || "round",
                                }))
                              : [
                                  {
                                    chainType: 3,
                                    connectIp: "",
                                    nodeId: -1,
                                    protocol: "tls",
                                    strategy: value || "round",
                                  },
                                ];

                          return {
                            ...current,
                            outNodeId: nextNodes,
                          };
                        });
                      }}
                    >
                      {TUNNEL_STRATEGY_OPTIONS.map((item) => (
                        <SelectItem key={item}>{item}</SelectItem>
                      ))}
                    </Select>
                  </div>

                  <Select
                    isDisabled={
                      selectedOutNodeIds.length === 0 ||
                      commonOutIpOptions.length === 0 ||
                      isMultiExit
                    }
                    label="出口连接 IP"
                    placeholder={
                      isMultiExit ? "多出口不支持自定义 IP" : "默认连接 IP"
                    }
                    selectedKeys={[outConnectIp || "__default__"]}
                    variant="bordered"
                    onSelectionChange={(keys) => {
                      const [value] = selectionToStrings(keys);
                      const connectIp = value === "__default__" ? "" : value || "";

                      updateTunnelEditor((current) => ({
                        ...current,
                        outNodeId: (current.outNodeId || []).map((item) => ({
                          ...item,
                          connectIp,
                        })),
                      }));
                    }}
                  >
                    <SelectItem key="__default__">默认连接 IP</SelectItem>
                    {commonOutIpOptions.map((item) => (
                      <SelectItem key={item}>{item}</SelectItem>
                    ))}
                  </Select>
                </div>
              );
            })()}
          </>
        ) : null}

        <div className="flex gap-2">
          {tunnelEditor.draftId ? (
            <Button color="danger" variant="flat" onPress={removeSelectedDraftTunnel}>
              删除草稿
            </Button>
          ) : null}
          <Button
            className="flex-1"
            color="primary"
            isLoading={syncing}
            onPress={handleTunnelSave}
          >
            {tunnelEditor.id ? "同步隧道" : "创建隧道"}
          </Button>
        </div>
      </div>
    );
  };
  const renderForwardInspector = () => {
    if (!forwardEditor) {
      return null;
    }

    const currentTunnel =
      typeof forwardEditor.tunnelId === "number"
        ? tunnels.find((item) => item.id === forwardEditor.tunnelId) || null
        : null;
    const currentTunnelNodeIds = (currentTunnel?.inNodeId || []).map(
      (item) => item.nodeId,
    );
    const currentTunnelIpOptions = getCommonIpOptions(
      systemNodes,
      currentTunnelNodeIds,
    );
    const isCurrentTunnelMultiEntrance = currentTunnelNodeIds.length > 1;

    return (
      <div className="space-y-4">
        <div className="rounded-xl border border-amber-200 bg-amber-50/80 p-4 text-sm text-amber-700 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-200">
          保存时继续走原有 `forward/create` 和 `forward/update` 接口。
        </div>

        <div className="flex flex-wrap gap-2">
          <Chip color={forwardEditor.draftId ? "warning" : "primary"}>
            {forwardEditor.draftId ? "草稿" : getStatusLabel(forwardEditor.status)}
          </Chip>
          <Chip variant="flat">
            {currentTunnel?.name || forwardEditor.tunnelName || "未关联隧道"}
          </Chip>
        </div>

        <Input
          errorMessage={forwardErrors.name}
          isInvalid={Boolean(forwardErrors.name)}
          label="规则名称"
          value={forwardEditor.name}
          variant="bordered"
          onChange={(event) =>
            updateForwardEditor((current) => ({
              ...current,
              name: event.target.value,
            }))
          }
        />

        <Select
          errorMessage={forwardErrors.tunnelId}
          isInvalid={Boolean(forwardErrors.tunnelId)}
          label="关联隧道"
          selectedKeys={
            forwardEditor.tunnelId ? [String(forwardEditor.tunnelId)] : []
          }
          variant="bordered"
          onSelectionChange={(keys) => {
            const [value] = selectionToStrings(keys);
            const nextTunnelId = Number.parseInt(value || "", 10);
            const selectedTunnel = tunnels.find((item) => item.id === nextTunnelId);

            updateForwardEditor((current) => ({
              ...current,
              inIp: "",
              tunnelId: Number.isFinite(nextTunnelId) ? nextTunnelId : null,
              tunnelName: selectedTunnel?.name,
            }));
          }}
        >
          {tunnels.map((item) => (
            <SelectItem key={item.id} textValue={item.name}>
              {item.name}
            </SelectItem>
          ))}
        </Select>

        <Input
          errorMessage={forwardErrors.inPort}
          isInvalid={Boolean(forwardErrors.inPort)}
          label="入口端口"
          placeholder="留空时自动分配"
          type="number"
          value={forwardEditor.inPort !== null ? String(forwardEditor.inPort) : ""}
          variant="bordered"
          onChange={(event) =>
            updateForwardEditor((current) => ({
              ...current,
              inPort: event.target.value
                ? Number.parseInt(event.target.value, 10)
                : null,
            }))
          }
        />

        <Select
          isDisabled={
            !forwardEditor.tunnelId ||
            currentTunnelIpOptions.length === 0 ||
            isCurrentTunnelMultiEntrance
          }
          label="监听 IP"
          placeholder={
            isCurrentTunnelMultiEntrance ? "多入口隧道不支持自定义监听 IP" : "默认入口 IP"
          }
          selectedKeys={[forwardEditor.inIp || "__default__"]}
          variant="bordered"
          onSelectionChange={(keys) => {
            const [value] = selectionToStrings(keys);

            updateForwardEditor((current) => ({
              ...current,
              inIp: value === "__default__" ? "" : value || "",
            }));
          }}
        >
          <SelectItem key="__default__">默认入口 IP</SelectItem>
          {currentTunnelIpOptions.map((item) => (
            <SelectItem key={item}>{item}</SelectItem>
          ))}
        </Select>

        <Textarea
          description="一行一个目标地址，支持多地址规则。"
          errorMessage={forwardErrors.remoteAddr}
          isInvalid={Boolean(forwardErrors.remoteAddr)}
          label="目标地址"
          maxRows={6}
          minRows={3}
          value={forwardEditor.remoteAddr}
          variant="bordered"
          onChange={(event) =>
            updateForwardEditor((current) => ({
              ...current,
              remoteAddr: event.target.value,
            }))
          }
        />

        {splitMultiValue(forwardEditor.remoteAddr).length > 1 ? (
          <Select
            label="负载策略"
            selectedKeys={[forwardEditor.strategy]}
            variant="bordered"
            onSelectionChange={(keys) => {
              const [value] = selectionToStrings(keys);

              updateForwardEditor((current) => ({
                ...current,
                strategy: value || "fifo",
              }));
            }}
          >
            <SelectItem key="fifo">fifo</SelectItem>
            <SelectItem key="round">round</SelectItem>
            <SelectItem key="rand">rand</SelectItem>
            <SelectItem key="hash">hash</SelectItem>
          </Select>
        ) : null}

        <div className="flex gap-2">
          {forwardEditor.draftId ? (
            <Button
              color="danger"
              variant="flat"
              onPress={removeSelectedDraftForward}
            >
              删除草稿
            </Button>
          ) : null}
          <Button
            className="flex-1"
            color="primary"
            isLoading={syncing}
            onPress={handleForwardSave}
          >
            {forwardEditor.id ? "同步规则" : "创建规则"}
          </Button>
        </div>
      </div>
    );
  };

  return (
    <div className="relative h-full w-full overflow-hidden rounded-2xl border border-default-200 bg-[radial-gradient(circle_at_top_left,_rgba(56,189,248,0.08),_transparent_35%),radial-gradient(circle_at_bottom_right,_rgba(251,191,36,0.08),_transparent_28%)]">
      <div className="absolute left-4 top-4 z-10 flex flex-wrap items-center gap-2 rounded-2xl border border-default-200 bg-white/90 p-3 shadow-lg backdrop-blur dark:bg-zinc-950/90">
        <Button color="primary" isLoading={layoutSaving} onPress={handleSaveLayout}>
          保存布局
        </Button>
        <Button variant="flat" onPress={() => void loadData({ reloadLayout: true })}>
          刷新数据
        </Button>
        <Button variant="flat" onPress={handleAddTunnel}>
          新增隧道
        </Button>
        <Button variant="flat" onPress={handleAddForward}>
          新增规则
        </Button>
        <div className="text-xs text-default-500">
          tunnel / forward 同步继续复用原接口
        </div>
      </div>

      <ReactFlow
        className="h-full w-full bg-transparent"
        defaultEdgeOptions={{
          markerEnd: {
            type: MarkerType.ArrowClosed,
          },
          type: "smoothstep",
        }}
        edges={edges}
        fitView
        nodeTypes={nodeTypes}
        nodes={nodes}
        nodesConnectable={false}
        onEdgesChange={onEdgesChange}
        onNodeClick={(_, node) => setSelectedNodeId(node.id)}
        onNodeDragStop={(_, node) =>
          setSavedPositions((items) => ({
            ...items,
            [node.id]: node.position,
          }))
        }
        onNodesChange={onNodesChange}
        onPaneClick={() => setSelectedNodeId(null)}
      >
        <MiniMap
          pannable
          zoomable
          nodeColor={(node) => {
            if (node.data?.kind === "system") {
              return "#16a34a";
            }

            if (node.data?.kind === "tunnel") {
              return "#0284c7";
            }

            return "#d97706";
          }}
        />
        <Controls />
        <Background color="#cbd5e1" gap={18} />
      </ReactFlow>

      <div className="absolute bottom-4 left-4 z-10 rounded-2xl border border-default-200 bg-white/90 px-4 py-3 text-xs text-default-600 shadow-lg backdrop-blur dark:bg-zinc-950/90">
        绿色是物理节点，蓝色是隧道，橙色是规则。右侧编辑完成后会直接同步到现有
        `tunnel` / `forward` 接口。
      </div>

      <div className="absolute right-0 top-0 z-20 h-full w-full max-w-[440px] border-l border-default-200 bg-white/95 shadow-2xl backdrop-blur dark:bg-zinc-950/95">
        <div className="flex items-center justify-between border-b border-default-200 px-5 py-4">
          <div>
            <div className="text-xs uppercase tracking-[0.2em] text-default-500">
              Inspector
            </div>
            <div className="mt-1 text-base font-semibold text-foreground">
              {selectedSystemNode
                ? selectedSystemNode.name
                : tunnelEditor
                  ? tunnelEditor.name || "隧道编辑"
                  : forwardEditor
                    ? forwardEditor.name || "规则编辑"
                    : "可视化编辑器"}
            </div>
          </div>
          <Button isIconOnly variant="light" onPress={() => setSelectedNodeId(null)}>
            ×
          </Button>
        </div>

        <div className="h-[calc(100%-81px)] overflow-y-auto p-5">
          {selectedSystemNode
            ? renderSystemInspector()
            : tunnelEditor
              ? renderTunnelInspector()
              : forwardEditor
                ? renderForwardInspector()
                : (
                  <div className="rounded-2xl border border-dashed border-default-300 p-6 text-sm leading-7 text-default-500">
                    从画布选择任意物理节点、隧道或规则，即可在这里编辑。新的隧道和规则也可以直接从左上角按钮创建，然后同步到后端现有接口。
                  </div>
                )}
        </div>
      </div>

      {loading ? (
        <div className="absolute inset-0 z-30 flex items-center justify-center bg-white/70 backdrop-blur-sm dark:bg-zinc-950/70">
          <div className="rounded-2xl border border-default-200 bg-white px-6 py-4 text-sm text-default-600 shadow-xl dark:bg-zinc-950">
            正在加载可视化数据...
          </div>
        </div>
      ) : null}
    </div>
  );
}
