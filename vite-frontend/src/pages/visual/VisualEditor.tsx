import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import ReactFlow, {
  Background,
  type Connection,
  Controls,
  type Edge,
  MarkerType,
  MiniMap,
  type Node,
  type OnConnectStartParams,
  type XYPosition,
  useEdgesState,
  useNodesState,
  useReactFlow,
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
import ComfyServerNode, {
  type ComfyServerNodeData,
} from "./nodes/ComfyServerNode";
import ComfyTunnelNode, {
  type ComfyTunnelNodeData,
} from "./nodes/ComfyTunnelNode";
import ComfyForwardNode, {
  type ComfyForwardNodeData,
} from "./nodes/ComfyForwardNode";
import ComfyLandingNode, {
  type ComfyLandingNodeData,
} from "./nodes/ComfyLandingNode";
import PortMappingEdge, {
  type PortMappingEdgeData,
} from "./edges/PortMappingEdge";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

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

interface UndoSnapshot {
  draftForwards: ForwardEditorState[];
  draftTunnels: TunnelEditorState[];
  forwards: ForwardEditorState[];
  tunnels: TunnelEditorState[];
}

// ---------------------------------------------------------------------------
// Node & edge type registration
// ---------------------------------------------------------------------------

const nodeTypes = {
  comfyForward: ComfyForwardNode,
  comfyLanding: ComfyLandingNode,
  comfyServer: ComfyServerNode,
  comfyTunnel: ComfyTunnelNode,
};

const edgeTypes = {
  portMapping: PortMappingEdge,
};

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const TUNNEL_PROTOCOL_OPTIONS = ["tls", "wss", "tcp", "mtls", "mwss", "mtcp"];
const TUNNEL_STRATEGY_OPTIONS = ["fifo", "round", "rand"];

const SYSTEM_PREFIX = "system:";
const TUNNEL_PREFIX = "tunnel:";
const TUNNEL_DRAFT_PREFIX = "tunnel:draft:";
const FORWARD_PREFIX = "forward:";
const FORWARD_DRAFT_PREFIX = "forward:draft:";

const MAX_UNDO_HISTORY = 30;

// ---------------------------------------------------------------------------
// ID helpers
// ---------------------------------------------------------------------------

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

const extractVisualNumericId = (
  value: string,
  prefix: string,
): number | null => {
  if (!value.startsWith(prefix)) return null;
  const parsed = Number.parseInt(value.slice(prefix.length), 10);
  return Number.isFinite(parsed) ? parsed : null;
};

const extractDraftId = (value: string, prefix: string): string | null => {
  if (!value.startsWith(prefix)) return null;
  const parsed = value.slice(prefix.length).trim();
  return parsed || null;
};

// ---------------------------------------------------------------------------
// Clone / merge helpers
// ---------------------------------------------------------------------------

const cloneChainNode = (
  item: TunnelChainNodePayload,
): TunnelChainNodePayload => ({ ...item });

const cloneTunnelState = (item: TunnelEditorState): TunnelEditorState => ({
  ...item,
  chainNodes: (item.chainNodes || []).map((g) => g.map(cloneChainNode)),
  inNodeId: (item.inNodeId || []).map(cloneChainNode),
  outNodeId: (item.outNodeId || []).map(cloneChainNode),
});

const cloneForwardState = (item: ForwardEditorState): ForwardEditorState => ({
  ...item,
});

const splitMultiValue = (value?: string): string[] =>
  String(value || "")
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter((s) => s !== "");

const commaToMultiline = (value?: string) => splitMultiValue(value).join("\n");
const multilineToComma = (value?: string) => splitMultiValue(value).join(",");

const normalizeAddressSignature = (value?: string) =>
  splitMultiValue(value)
    .map((s) => s.toLowerCase())
    .join(",");

const selectionToStrings = (keys: unknown): string[] => {
  if (!keys || keys === "all") return [];
  return Array.from(keys as Iterable<unknown>).map((item) => String(item));
};

const selectionToNodeIds = (keys: unknown): number[] =>
  selectionToStrings(keys)
    .map((s) => Number.parseInt(s, 10))
    .filter((n) => Number.isFinite(n));

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
    .map(buildDefault);
  return [...kept, ...added];
};

const getTunnelNodeIds = (item: TunnelEditorState): number[] => {
  const result = new Set<number>();
  (item.inNodeId || []).forEach((n) => n.nodeId > 0 && result.add(n.nodeId));
  (item.outNodeId || []).forEach((n) => n.nodeId > 0 && result.add(n.nodeId));
  (item.chainNodes || []).forEach((g) =>
    g.forEach((n) => n.nodeId > 0 && result.add(n.nodeId)),
  );
  return Array.from(result);
};

const getSelectedChainNodeIds = (item: TunnelEditorState): number[] =>
  (item.chainNodes || [])
    .flatMap((g) => g.map((n) => n.nodeId))
    .filter((id) => id > 0);

const getNodeIpOptions = (nodes: NodeApiItem[], nodeId: number): string[] => {
  const node = nodes.find((item) => item.id === nodeId);
  if (!node) return [];
  const values = [
    String(node.serverIpV4 || "").trim(),
    String(node.serverIpV6 || "").trim(),
    String(node.serverIp || "").trim(),
    ...String(node.extraIPs || "")
      .split(",")
      .map((s) => s.trim())
      .filter((s) => s !== ""),
  ].filter((s) => s !== "");
  return Array.from(new Set(values));
};

const getCommonIpOptions = (
  nodes: NodeApiItem[],
  nodeIds: number[],
): string[] => {
  if (nodeIds.length === 0) return [];
  const sets = nodeIds.map((id) => new Set(getNodeIpOptions(nodes, id)));
  const base = sets[0];
  return Array.from(base).filter((ip) => sets.every((s) => s.has(ip)));
};

const removeNodeFromTunnel = (
  tunnel: TunnelEditorState,
  nodeId: number,
): TunnelEditorState => ({
  ...tunnel,
  chainNodes: (tunnel.chainNodes || [])
    .map((g) => g.filter((n) => n.nodeId !== nodeId))
    .filter((g) => g.length > 0),
  inNodeId: (tunnel.inNodeId || []).filter((n) => n.nodeId !== nodeId),
  outNodeId: (tunnel.outNodeId || []).filter((n) => n.nodeId !== nodeId),
});

// ---------------------------------------------------------------------------
// Draft factories
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// API mappers
// ---------------------------------------------------------------------------

const mapTunnelApiItems = (items: TunnelApiItem[]): TunnelEditorState[] =>
  (items || []).map((item) => ({
    chainNodes: Array.isArray(item.chainNodes)
      ? item.chainNodes.map((g) => (g || []).map((n) => ({ ...n })))
      : [],
    flow: typeof item.flow === "number" ? item.flow : 1,
    id: item.id,
    inIp: typeof item.inIp === "string" ? item.inIp : "",
    inNodeId: Array.isArray(item.inNodeId)
      ? item.inNodeId.map((n) => ({ ...n }))
      : [],
    ipPreference:
      typeof item.ipPreference === "string" ? item.ipPreference : "",
    name: typeof item.name === "string" ? item.name : "",
    outNodeId: Array.isArray(item.outNodeId)
      ? item.outNodeId.map((n) => ({ ...n }))
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

// ---------------------------------------------------------------------------
// Layout persistence
// ---------------------------------------------------------------------------

const parseSavedPositions = (
  graphJSON?: string,
): Record<string, XYPosition> => {
  if (!graphJSON) return {};
  try {
    const parsed = JSON.parse(graphJSON);
    if (parsed && typeof parsed === "object") {
      if (parsed.positions && typeof parsed.positions === "object") {
        return Object.entries(parsed.positions).reduce<
          Record<string, XYPosition>
        >((acc, [key, value]) => {
          const x = Number((value as { x?: number }).x);
          const y = Number((value as { y?: number }).y);
          if (Number.isFinite(x) && Number.isFinite(y)) acc[key] = { x, y };
          return acc;
        }, {});
      }
      if (Array.isArray(parsed.nodes)) {
        return (parsed.nodes as Array<any>).reduce<Record<string, XYPosition>>(
          (acc, item) => {
            const id = String(item?.id || "");
            const x = Number(item?.position?.x);
            const y = Number(item?.position?.y);
            if (id && Number.isFinite(x) && Number.isFinite(y))
              acc[id] = { x, y };
            return acc;
          },
          {},
        );
      }
    }
  } catch {}
  return {};
};

const serializeLayout = (items: Node[]) =>
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

// ---------------------------------------------------------------------------
// Graph builders (ComfyUI nodes)
// ---------------------------------------------------------------------------

const buildGraphNodes = (
  systemNodes: NodeApiItem[],
  tunnels: TunnelEditorState[],
  forwards: ForwardEditorState[],
  positions: Record<string, XYPosition>,
  selectedNodeId: string | null,
  probeCache: Record<number, VisualProbeData>,
): Node[] => {
  const tunnelNodeMap = new Map<number, number[]>();
  tunnels.forEach((tunnel) => {
    if (typeof tunnel.id === "number" && tunnel.id > 0)
      tunnelNodeMap.set(tunnel.id, getTunnelNodeIds(tunnel));
  });

  const forwardCountByTunnel = new Map<number, number>();
  forwards.forEach((f) => {
    if (f.tunnelId)
      forwardCountByTunnel.set(
        f.tunnelId,
        (forwardCountByTunnel.get(f.tunnelId) || 0) + 1,
      );
  });

  // Server nodes
  const serverFlowNodes: Node<ComfyServerNodeData>[] = systemNodes.map(
    (item, index) => {
      const visualId = getSystemVisualId(item.id);
      const probe = probeCache[item.id];
      return {
        data: {
          allowedPorts: probe?.allowed_ports,
          connections: probe?.connections,
          cpuUsage: probe?.cpu_usage,
          id: item.id,
          ip:
            String(item.serverIp || "").trim() ||
            String(item.serverIpV4 || "").trim() ||
            "未配置 IP",
          ipV4: String(item.serverIpV4 || "").trim() || undefined,
          ipV6: String(item.serverIpV6 || "").trim() || undefined,
          memUsage: probe?.mem_usage,
          name: item.name,
          occupiedPorts: probe?.occupied_ports,
          online: item.status === 1,
        },
        id: visualId,
        position: positions[visualId] || { x: 60, y: 60 + index * 220 },
        selected: visualId === selectedNodeId,
        type: "comfyServer",
      };
    },
  );

  // Tunnel nodes
  const tunnelFlowNodes: Node<ComfyTunnelNodeData>[] = tunnels.map(
    (item, index) => {
      const visualId = buildTunnelNodeId(item);
      const entryCount = (item.inNodeId || []).filter(
        (n) => n.nodeId > 0,
      ).length;
      const exitCount = (item.outNodeId || []).filter(
        (n) => n.nodeId > 0,
      ).length;
      const chainCount = (item.chainNodes || []).filter((g) =>
        g.some((n) => n.nodeId > 0),
      ).length;
      const relatedForwards =
        typeof item.id === "number"
          ? forwardCountByTunnel.get(item.id) || 0
          : 0;
      return {
        data: {
          chainCount,
          draftId: item.draftId,
          entryCount,
          exitCount,
          exitProtocol: item.outNodeId?.[0]?.protocol,
          exitStrategy: item.outNodeId?.[0]?.strategy,
          flow: item.flow,
          id: item.id,
          name: item.name || "未命名隧道",
          relatedForwards,
          status: item.status,
          trafficRatio: item.trafficRatio,
          type: item.type,
        },
        id: visualId,
        position: positions[visualId] || { x: 440, y: 60 + index * 240 },
        selected: visualId === selectedNodeId,
        type: "comfyTunnel",
      };
    },
  );

  // Forward nodes
  const forwardFlowNodes: Node<ComfyForwardNodeData>[] = forwards.map(
    (item, index) => {
      const visualId = buildForwardNodeId(item);
      return {
        data: {
          draftId: item.draftId,
          id: item.id,
          inIp: item.inIp,
          inPort: item.inPort,
          name: item.name || "未命名规则",
          remoteAddrs: splitMultiValue(item.remoteAddr),
          status: item.status,
          strategy: item.strategy,
          tunnelId: item.tunnelId,
          tunnelName: item.tunnelName,
        },
        id: visualId,
        position: positions[visualId] || { x: 840, y: 60 + index * 200 },
        selected: visualId === selectedNodeId,
        type: "comfyForward",
      };
    },
  );

  // Landing nodes (derived from forwards with remoteAddr)
  const landingFlowNodes: Node<ComfyLandingNodeData>[] = forwards
    .filter((item) => splitMultiValue(item.remoteAddr).length > 0)
    .map((item, index) => {
      const visualId = `landing:${buildForwardNodeId(item)}`;
      const addrs = splitMultiValue(item.remoteAddr);
      return {
        data: {
          addresses: addrs,
          forwardId: item.id,
          forwardName: item.name,
          inPort: item.inPort,
        },
        id: visualId,
        position: positions[visualId] || { x: 1220, y: 60 + index * 180 },
        selected: visualId === selectedNodeId,
        type: "comfyLanding",
      };
    });

  return [
    ...serverFlowNodes,
    ...tunnelFlowNodes,
    ...forwardFlowNodes,
    ...landingFlowNodes,
  ];
};

const buildGraphEdges = (
  systemNodes: NodeApiItem[],
  tunnels: TunnelEditorState[],
  forwards: ForwardEditorState[],
): Edge<PortMappingEdgeData>[] => {
  const existingNodeIds = new Set<string>([
    ...systemNodes.map((item) => getSystemVisualId(item.id)),
    ...tunnels.map((item) => buildTunnelNodeId(item)),
    ...forwards.map((item) => buildForwardNodeId(item)),
  ]);

  const result: Edge<PortMappingEdgeData>[] = [];

  const pushEdge = (
    id: string,
    source: string,
    target: string,
    sourceHandle: string,
    targetHandle: string,
    edgeData: PortMappingEdgeData,
  ) => {
    if (!existingNodeIds.has(source) || !existingNodeIds.has(target)) return;
    result.push({
      data: edgeData,
      id,
      markerEnd: { type: MarkerType.ArrowClosed },
      source,
      sourceHandle,
      target,
      targetHandle,
      type: "portMapping",
    });
  };

  // Tunnel → Server edges
  tunnels.forEach((tunnel) => {
    const tunnelId = buildTunnelNodeId(tunnel);

    // Entry connections: Server.out → Tunnel.entry
    (tunnel.inNodeId || []).forEach((item, index) => {
      if (item.nodeId <= 0) return;
      const sourceNode = systemNodes.find((n) => n.id === item.nodeId);
      const diagSourceId = item.nodeId;
      // Find any exit node for diagnostic target
      const exitNode = (tunnel.outNodeId || [])[0];
      const diagTargetId = exitNode?.nodeId && exitNode.nodeId > 0 ? exitNode.nodeId : undefined;

      pushEdge(
        `${tunnelId}:entry:${item.nodeId}:${index}`,
        getSystemVisualId(item.nodeId),
        tunnelId,
        "out",
        "entry",
        {
          diagnosticSourceId: diagSourceId,
          diagnosticTargetId: diagTargetId,
          edgeType: "entry",
          label:
            tunnel.inNodeId.length > 1 ? `Entry ${index + 1}` : "Entry",
          portMapping: sourceNode
            ? String(sourceNode.serverIp || "").trim()
            : undefined,
        },
      );
    });

    // Chain hop connections: Server.out → Tunnel.entry (with hop label)
    (tunnel.chainNodes || []).forEach((group, groupIndex) => {
      group.forEach((item) => {
        if (item.nodeId <= 0) return;
        pushEdge(
          `${tunnelId}:hop:${groupIndex + 1}:${item.nodeId}`,
          getSystemVisualId(item.nodeId),
          tunnelId,
          "out",
          "entry",
          {
            edgeType: "hop",
            label: `Hop ${groupIndex + 1}`,
            portMapping: item.protocol
              ? `${item.protocol.toUpperCase()} / ${item.strategy || "round"}`
              : undefined,
          },
        );
      });
    });

    // Exit connections: Tunnel.exit → Server.in
    (tunnel.outNodeId || []).forEach((item, index) => {
      if (item.nodeId <= 0) return;
      const entryNode = (tunnel.inNodeId || [])[0];
      const diagSourceId =
        entryNode?.nodeId && entryNode.nodeId > 0
          ? entryNode.nodeId
          : undefined;
      pushEdge(
        `${tunnelId}:exit:${item.nodeId}:${index}`,
        tunnelId,
        getSystemVisualId(item.nodeId),
        "exit",
        "in",
        {
          diagnosticSourceId: diagSourceId,
          diagnosticTargetId: item.nodeId,
          edgeType: "exit",
          label: tunnel.outNodeId.length > 1 ? `Exit ${index + 1}` : "Exit",
          portMapping: item.protocol
            ? `${item.protocol.toUpperCase()} / ${item.strategy || "round"}`
            : undefined,
        },
      );
    });
  });

  // Forward → Tunnel edges: Tunnel.rules → Forward.tunnel
  forwards.forEach((forward) => {
    if (!forward.tunnelId) return;
    const portMapping = forward.inPort
      ? `${forward.inPort} → ${splitMultiValue(forward.remoteAddr)[0] || "?"}`
      : splitMultiValue(forward.remoteAddr)[0] || undefined;
    pushEdge(
      `${buildForwardNodeId(forward)}:rule`,
      getTunnelVisualId(forward.tunnelId),
      buildForwardNodeId(forward),
      "rules",
      "tunnel",
      {
        edgeType: "rule",
        label: "Rule",
        portMapping,
      },
    );
  });

  // Forward → Landing edges
  forwards.forEach((forward) => {
    const addrs = splitMultiValue(forward.remoteAddr);
    if (addrs.length === 0) return;
    const forwardVisualId = buildForwardNodeId(forward);
    const landingVisualId = `landing:${forwardVisualId}`;
    if (!existingNodeIds.has(forwardVisualId)) return;
    // Landing nodes are generated, so add them to the set
    existingNodeIds.add(landingVisualId);
    result.push({
      data: {
        edgeType: "rule" as const,
        label: "Landing",
        portMapping:
          addrs.length === 1
            ? addrs[0]
            : `${addrs.length} targets`,
      },
      id: `${forwardVisualId}:landing`,
      markerEnd: { type: MarkerType.ArrowClosed },
      source: forwardVisualId,
      sourceHandle: undefined,
      target: landingVisualId,
      targetHandle: "in",
      type: "portMapping",
    });
  });

  return result;
};

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

const validateForwardEditor = (
  editor: ForwardEditorState,
): Record<string, string> => {
  const errors: Record<string, string> = {};
  if (!editor.name.trim()) errors.name = "请输入规则名称";
  if (!editor.tunnelId || editor.tunnelId <= 0)
    errors.tunnelId = "请选择关联隧道";
  if (
    editor.inPort !== null &&
    (!Number.isInteger(editor.inPort) ||
      editor.inPort <= 0 ||
      editor.inPort > 65535)
  )
    errors.inPort = "端口必须在 1-65535 之间";
  if (splitMultiValue(editor.remoteAddr).length === 0)
    errors.remoteAddr = "请输入目标地址";
  return errors;
};

// ---------------------------------------------------------------------------
// Auto-layout
// ---------------------------------------------------------------------------

const computeAutoLayout = (
  systemNodes: NodeApiItem[],
  tunnels: TunnelEditorState[],
  forwards: ForwardEditorState[],
): Record<string, XYPosition> => {
  const positions: Record<string, XYPosition> = {};
  const colGap = 400;
  const rowGap = 220;

  // Column 0: Server nodes
  systemNodes.forEach((item, i) => {
    positions[getSystemVisualId(item.id)] = { x: 60, y: 60 + i * rowGap };
  });

  // Column 1: Tunnels
  tunnels.forEach((item, i) => {
    positions[buildTunnelNodeId(item)] = {
      x: 60 + colGap,
      y: 60 + i * rowGap,
    };
  });

  // Column 2: Forwards
  forwards.forEach((item, i) => {
    positions[buildForwardNodeId(item)] = {
      x: 60 + colGap * 2,
      y: 60 + i * (rowGap - 20),
    };
  });

  // Column 3: Landing nodes
  const fwdsWithTargets = forwards.filter(
    (f) => splitMultiValue(f.remoteAddr).length > 0,
  );
  fwdsWithTargets.forEach((item, i) => {
    positions[`landing:${buildForwardNodeId(item)}`] = {
      x: 60 + colGap * 3,
      y: 60 + i * (rowGap - 40),
    };
  });

  return positions;
};

// ---------------------------------------------------------------------------
// JSON export
// ---------------------------------------------------------------------------

const buildExportJSON = (
  tunnels: TunnelEditorState[],
  forwards: ForwardEditorState[],
  systemNodes: NodeApiItem[],
) =>
  JSON.stringify(
    {
      exportedAt: new Date().toISOString(),
      forwards: forwards.map((f) => ({
        id: f.id,
        inIp: f.inIp,
        inPort: f.inPort,
        name: f.name,
        remoteAddr: f.remoteAddr,
        status: f.status,
        strategy: f.strategy,
        tunnelId: f.tunnelId,
      })),
      nodes: systemNodes.map((n) => ({
        id: n.id,
        name: n.name,
        serverIp: n.serverIp,
        status: n.status,
      })),
      tunnels: tunnels.map((t) => ({
        chainNodes: t.chainNodes,
        flow: t.flow,
        id: t.id,
        inIp: t.inIp,
        inNodeId: t.inNodeId,
        ipPreference: t.ipPreference,
        name: t.name,
        outNodeId: t.outNodeId,
        status: t.status,
        trafficRatio: t.trafficRatio,
        type: t.type,
      })),
      version: 1,
    },
    null,
    2,
  );

// ---------------------------------------------------------------------------
// getTunnelTypeLabel / getStatusLabel
// ---------------------------------------------------------------------------

const getTunnelTypeLabel = (type: number) =>
  type === 2 ? "隧道转发" : "端口转发";
const getStatusLabel = (status: number) => (status === 1 ? "启用" : "停用");

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export default function VisualEditor() {
  const reactFlowInstance = useReactFlow();

  // Data state
  const [systemNodes, setSystemNodes] = useState<NodeApiItem[]>([]);
  const [tunnels, setTunnels] = useState<TunnelEditorState[]>([]);
  const [forwards, setForwards] = useState<ForwardEditorState[]>([]);
  const [draftTunnels, setDraftTunnels] = useState<TunnelEditorState[]>([]);
  const [draftForwards, setDraftForwards] = useState<ForwardEditorState[]>([]);
  const [savedPositions, setSavedPositions] = useState<
    Record<string, XYPosition>
  >({});

  // Flow state
  const [nodes, setNodes, onNodesChange] = useNodesState([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);

  // UI state
  const [loading, setLoading] = useState(true);
  const [layoutSaving, setLayoutSaving] = useState(false);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [syncing, setSyncing] = useState(false);
  const [deploying, setDeploying] = useState(false);
  const [browseOpen, setBrowseOpen] = useState(false);
  const [focusedTunnelId, setFocusedTunnelId] = useState<string | null>(null);
  const [importPanelOpen, setImportPanelOpen] = useState(false);
  const [wizardOpen, setWizardOpen] = useState(false);
  const [wizardMode, setWizardMode] = useState<"tunnel" | "port">("tunnel");
  const [wizardNodeA, setWizardNodeA] = useState<number | null>(null);
  const [wizardNodeB, setWizardNodeB] = useState<number | null>(null);
  const [wizardLanding, setWizardLanding] = useState("");
  const [wizardPort, setWizardPort] = useState("");
  const [wizardName, setWizardName] = useState("");
  const [wizardCreating, setWizardCreating] = useState(false);

  // Probe cache
  const [probeCache, setProbeCache] = useState<
    Record<number, VisualProbeData>
  >({});

  // Inspector editors
  const [tunnelEditor, setTunnelEditor] = useState<TunnelEditorState | null>(
    null,
  );
  const [forwardEditor, setForwardEditor] =
    useState<ForwardEditorState | null>(null);
  const [tunnelErrors, setTunnelErrors] = useState<Record<string, string>>({});
  const [forwardErrors, setForwardErrors] = useState<Record<string, string>>(
    {},
  );

  // Undo/redo
  const [undoStack, setUndoStack] = useState<UndoSnapshot[]>([]);
  const [redoStack, setRedoStack] = useState<UndoSnapshot[]>([]);

  // Connection drag state (for visual creation)
  const connectingRef = useRef<OnConnectStartParams | null>(null);

  // Derived
  const allTunnels = useMemo(
    () => [...tunnels, ...draftTunnels],
    [tunnels, draftTunnels],
  );
  const allForwards = useMemo(
    () => [...forwards, ...draftForwards],
    [forwards, draftForwards],
  );

  const selectedSystemNodeId = useMemo(() => {
    if (!isSystemNodeId(selectedNodeId)) return null;
    return extractVisualNumericId(selectedNodeId, SYSTEM_PREFIX);
  }, [selectedNodeId]);

  const selectedSystemNode = useMemo(() => {
    if (!selectedSystemNodeId) return null;
    return systemNodes.find((n) => n.id === selectedSystemNodeId) || null;
  }, [selectedSystemNodeId, systemNodes]);

  const relatedTunnelsForSystem = useMemo(() => {
    if (!selectedSystemNodeId) return [];
    return allTunnels.filter((t) =>
      getTunnelNodeIds(t).includes(selectedSystemNodeId),
    );
  }, [allTunnels, selectedSystemNodeId]);

  const relatedTunnelIdSet = useMemo(
    () =>
      new Set(
        relatedTunnelsForSystem
          .map((t) => t.id)
          .filter((id): id is number => typeof id === "number" && id > 0),
      ),
    [relatedTunnelsForSystem],
  );

  const relatedForwardsForSystem = useMemo(() => {
    if (!selectedSystemNodeId) return [];
    return allForwards.filter(
      (f) => typeof f.tunnelId === "number" && relatedTunnelIdSet.has(f.tunnelId),
    );
  }, [allForwards, relatedTunnelIdSet, selectedSystemNodeId]);

  // -----------------------------------------------------------------------
  // Undo/redo helpers
  // -----------------------------------------------------------------------

  const pushUndo = useCallback(() => {
    setUndoStack((stack) => [
      ...stack.slice(-(MAX_UNDO_HISTORY - 1)),
      {
        draftForwards: draftForwards.map(cloneForwardState),
        draftTunnels: draftTunnels.map(cloneTunnelState),
        forwards: forwards.map(cloneForwardState),
        tunnels: tunnels.map(cloneTunnelState),
      },
    ]);
    setRedoStack([]);
  }, [draftForwards, draftTunnels, forwards, tunnels]);

  const handleUndo = useCallback(() => {
    setUndoStack((stack) => {
      if (stack.length === 0) return stack;
      const prev = stack[stack.length - 1];
      setRedoStack((redo) => [
        ...redo,
        {
          draftForwards: draftForwards.map(cloneForwardState),
          draftTunnels: draftTunnels.map(cloneTunnelState),
          forwards: forwards.map(cloneForwardState),
          tunnels: tunnels.map(cloneTunnelState),
        },
      ]);
      setTunnels(prev.tunnels);
      setForwards(prev.forwards);
      setDraftTunnels(prev.draftTunnels);
      setDraftForwards(prev.draftForwards);
      return stack.slice(0, -1);
    });
  }, [draftForwards, draftTunnels, forwards, tunnels]);

  const handleRedo = useCallback(() => {
    setRedoStack((stack) => {
      if (stack.length === 0) return stack;
      const next = stack[stack.length - 1];
      setUndoStack((undo) => [
        ...undo,
        {
          draftForwards: draftForwards.map(cloneForwardState),
          draftTunnels: draftTunnels.map(cloneTunnelState),
          forwards: forwards.map(cloneForwardState),
          tunnels: tunnels.map(cloneTunnelState),
        },
      ]);
      setTunnels(next.tunnels);
      setForwards(next.forwards);
      setDraftTunnels(next.draftTunnels);
      setDraftForwards(next.draftForwards);
      return stack.slice(0, -1);
    });
  }, [draftForwards, draftTunnels, forwards, tunnels]);

  // Keyboard shortcuts
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "z" && !e.shiftKey) {
        e.preventDefault();
        handleUndo();
      }
      if (
        (e.ctrlKey || e.metaKey) &&
        (e.key === "y" || (e.key === "z" && e.shiftKey))
      ) {
        e.preventDefault();
        handleRedo();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [handleUndo, handleRedo]);

  // -----------------------------------------------------------------------
  // Data loading
  // -----------------------------------------------------------------------

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
      if (nodesRes.code !== 0)
        throw new Error(nodesRes.msg || "加载节点失败");
      if (tunnelsRes.code !== 0)
        throw new Error(tunnelsRes.msg || "加载隧道失败");
      if (forwardsRes.code !== 0)
        throw new Error(forwardsRes.msg || "加载规则失败");
      return {
        forwards: mapForwardApiItems(
          (forwardsRes.data || []) as ForwardApiItem[],
        ),
        positions:
          reloadLayout && layoutRes?.code === 0
            ? parseSavedPositions(layoutRes.data?.graph_json)
            : undefined,
        systemNodes: (nodesRes.data || []) as NodeApiItem[],
        tunnels: mapTunnelApiItems(
          (tunnelsRes.data || []) as TunnelApiItem[],
        ),
      };
    },
    [],
  );

  const applySnapshot = useCallback(
    (snapshot: VisualSnapshot, reloadLayout: boolean) => {
      setSystemNodes(snapshot.systemNodes);
      setTunnels(snapshot.tunnels);
      setForwards(snapshot.forwards);
      if (reloadLayout) setSavedPositions(snapshot.positions || {});
    },
    [],
  );

  const loadProbeData = useCallback(async (nodeList: NodeApiItem[]) => {
    const results: Record<number, VisualProbeData> = {};
    await Promise.allSettled(
      nodeList.map(async (node) => {
        try {
          const res = await Network.post<VisualProbeData>(
            `/probe/node/${node.id}`,
          );
          if (res.code === 0 && res.data) results[node.id] = res.data;
        } catch {}
      }),
    );
    setProbeCache(results);
  }, []);

  const loadData = useCallback(
    async (options: { reloadLayout?: boolean; silent?: boolean } = {}) => {
      const reloadLayout = options.reloadLayout !== false;
      if (!options.silent) setLoading(true);
      try {
        const snapshot = await fetchSnapshot(reloadLayout);
        applySnapshot(snapshot, reloadLayout);
        void loadProbeData(snapshot.systemNodes);
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : "加载可视化数据失败",
        );
      } finally {
        if (!options.silent) setLoading(false);
      }
    },
    [applySnapshot, fetchSnapshot, loadProbeData],
  );

  useEffect(() => {
    void loadData();
  }, [loadData]);

  // -----------------------------------------------------------------------
  // Focused tunnel chain data (must be before graph sync)
  // -----------------------------------------------------------------------

  const focusedTunnel = useMemo(() => {
    if (!focusedTunnelId) return null;
    return allTunnels.find((t) => buildTunnelNodeId(t) === focusedTunnelId) || null;
  }, [allTunnels, focusedTunnelId]);

  const focusedChain = useMemo(() => {
    if (!focusedTunnel) return null;
    const entryNodes = (focusedTunnel.inNodeId || [])
      .filter((n) => n.nodeId > 0)
      .map((n) => ({
        ...n,
        node: systemNodes.find((s) => s.id === n.nodeId),
      }));
    const chainHops = (focusedTunnel.chainNodes || []).map((group, i) => ({
      groupIndex: i,
      nodes: group
        .filter((n) => n.nodeId > 0)
        .map((n) => ({
          ...n,
          node: systemNodes.find((s) => s.id === n.nodeId),
        })),
    }));
    const exitNodes = (focusedTunnel.outNodeId || [])
      .filter((n) => n.nodeId > 0)
      .map((n) => ({
        ...n,
        node: systemNodes.find((s) => s.id === n.nodeId),
      }));
    const relatedForwards = allForwards.filter(
      (f) =>
        typeof focusedTunnel.id === "number" &&
        focusedTunnel.id > 0 &&
        f.tunnelId === focusedTunnel.id,
    );
    return { chainHops, entryNodes, exitNodes, forwards: relatedForwards };
  }, [allForwards, focusedTunnel, systemNodes]);

  const focusedNodeIds = useMemo(() => {
    if (!focusedTunnel || !focusedChain) return null;
    const ids = new Set<string>();
    ids.add(buildTunnelNodeId(focusedTunnel));
    focusedChain.entryNodes.forEach((n) =>
      ids.add(getSystemVisualId(n.nodeId)),
    );
    focusedChain.exitNodes.forEach((n) =>
      ids.add(getSystemVisualId(n.nodeId)),
    );
    focusedChain.chainHops.forEach((hop) =>
      hop.nodes.forEach((n) => ids.add(getSystemVisualId(n.nodeId))),
    );
    focusedChain.forwards.forEach((f) => {
      ids.add(buildForwardNodeId(f));
      // Include landing node for each forward with targets
      if (splitMultiValue(f.remoteAddr).length > 0) {
        ids.add(`landing:${buildForwardNodeId(f)}`);
      }
    });
    return ids;
  }, [focusedChain, focusedTunnel]);

  // -----------------------------------------------------------------------
  // Graph sync
  // -----------------------------------------------------------------------

  useEffect(() => {
    const nextNodes = buildGraphNodes(
      systemNodes,
      allTunnels,
      allForwards,
      savedPositions,
      selectedNodeId,
      probeCache,
    );
    const nextEdges = buildGraphEdges(systemNodes, allTunnels, allForwards);

    // Filter to focused tunnel group if active
    if (focusedNodeIds) {
      setNodes(nextNodes.filter((n) => focusedNodeIds.has(n.id)));
      setEdges(
        nextEdges.filter(
          (e) => focusedNodeIds.has(e.source) && focusedNodeIds.has(e.target),
        ),
      );
    } else {
      setNodes(nextNodes);
      setEdges(nextEdges);
    }
  }, [
    allForwards,
    allTunnels,
    focusedNodeIds,
    probeCache,
    savedPositions,
    selectedNodeId,
    setEdges,
    setNodes,
    systemNodes,
  ]);

  // -----------------------------------------------------------------------
  // Inspector sync
  // -----------------------------------------------------------------------

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
          ? allTunnels.find((t) => t.id === tunnelId)
          : undefined) ||
        (draftId
          ? draftTunnels.find((t) => t.draftId === draftId)
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
          ? allForwards.find((f) => f.id === forwardId)
          : undefined) ||
        (draftId
          ? draftForwards.find((f) => f.draftId === draftId)
          : undefined);
      setTunnelEditor(null);
      setForwardEditor(match ? forwardToEditor(match) : null);
      return;
    }
    setTunnelEditor(null);
    setForwardEditor(null);
  }, [allForwards, allTunnels, draftForwards, draftTunnels, selectedNodeId]);

  // -----------------------------------------------------------------------
  // Editor update helpers
  // -----------------------------------------------------------------------

  const updateTunnelEditor = useCallback(
    (updater: (c: TunnelEditorState) => TunnelEditorState) => {
      setTunnelEditor((current) => {
        if (!current) return current;
        const next = updater(current);
        if (current.draftId)
          setDraftTunnels((items) =>
            items.map((t) =>
              t.draftId === current.draftId ? editorToTunnelState(next) : t,
            ),
          );
        if (current.id)
          setTunnels((items) =>
            items.map((t) =>
              t.id === current.id ? editorToTunnelState(next) : t,
            ),
          );
        return next;
      });
    },
    [],
  );

  const updateForwardEditor = useCallback(
    (updater: (c: ForwardEditorState) => ForwardEditorState) => {
      setForwardEditor((current) => {
        if (!current) return current;
        const next = updater(current);
        if (current.draftId)
          setDraftForwards((items) =>
            items.map((f) =>
              f.draftId === current.draftId ? editorToForwardState(next) : f,
            ),
          );
        if (current.id)
          setForwards((items) =>
            items.map((f) =>
              f.id === current.id ? editorToForwardState(next) : f,
            ),
          );
        return next;
      });
    },
    [],
  );

  const applyTunnelLocalChange = useCallback(
    (
      visualId: string,
      updater: (c: TunnelEditorState) => TunnelEditorState,
    ): TunnelEditorState | null => {
      const current = allTunnels.find(
        (t) => buildTunnelNodeId(t) === visualId,
      );
      if (!current) return null;
      const next = updater(cloneTunnelState(current));
      if (current.draftId)
        setDraftTunnels((items) =>
          items.map((t) =>
            t.draftId === current.draftId ? editorToTunnelState(next) : t,
          ),
        );
      else if (current.id)
        setTunnels((items) =>
          items.map((t) =>
            t.id === current.id ? editorToTunnelState(next) : t,
          ),
        );
      if (selectedNodeId === visualId) setTunnelEditor(tunnelToEditor(next));
      return next;
    },
    [allTunnels, selectedNodeId],
  );

  const applyForwardLocalChange = useCallback(
    (
      visualId: string,
      updater: (c: ForwardEditorState) => ForwardEditorState,
    ): ForwardEditorState | null => {
      const current = allForwards.find(
        (f) => buildForwardNodeId(f) === visualId,
      );
      if (!current) return null;
      const next = updater(cloneForwardState(current));
      if (current.draftId)
        setDraftForwards((items) =>
          items.map((f) =>
            f.draftId === current.draftId ? editorToForwardState(next) : f,
          ),
        );
      else if (current.id)
        setForwards((items) =>
          items.map((f) =>
            f.id === current.id ? editorToForwardState(next) : f,
          ),
        );
      if (selectedNodeId === visualId) setForwardEditor(forwardToEditor(next));
      return next;
    },
    [allForwards, selectedNodeId],
  );

  // -----------------------------------------------------------------------
  // Layout actions
  // -----------------------------------------------------------------------

  const handleSaveLayout = useCallback(async () => {
    setLayoutSaving(true);
    try {
      const graphJSON = serializeLayout(nodes);
      const res = await Network.post<any>("/nodes/graph", {
        graph_json: graphJSON,
      });
      if (res.code === 0) {
        setSavedPositions(parseSavedPositions(graphJSON));
        toast.success("布局已保存");
      } else {
        toast.error(res.msg || "保存布局失败");
      }
    } catch {
      toast.error("保存布局失败");
    } finally {
      setLayoutSaving(false);
    }
  }, [nodes]);

  const handleAutoLayout = useCallback(() => {
    pushUndo();
    const nextPositions = computeAutoLayout(
      systemNodes,
      allTunnels,
      allForwards,
    );
    setSavedPositions(nextPositions);
    setTimeout(() => reactFlowInstance.fitView({ padding: 0.15 }), 100);
    toast.success("已自动排列布局");
  }, [allForwards, allTunnels, pushUndo, reactFlowInstance, systemNodes]);

  // -----------------------------------------------------------------------
  // Add / remove drafts
  // -----------------------------------------------------------------------

  const handleAddTunnel = useCallback(
    (position?: XYPosition) => {
      pushUndo();
      const draftId = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      const draft = createDraftTunnel(draftId);
      setDraftTunnels((items) => [...items, draft]);
      if (position) {
        setSavedPositions((p) => ({
          ...p,
          [getTunnelDraftVisualId(draftId)]: position,
        }));
      }
      setSelectedNodeId(getTunnelDraftVisualId(draftId));
    },
    [pushUndo],
  );

  const handleAddForward = useCallback(
    (position?: XYPosition) => {
      pushUndo();
      const draftId = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      const draft = createDraftForward(draftId);
      setDraftForwards((items) => [...items, draft]);
      if (position) {
        setSavedPositions((p) => ({
          ...p,
          [getForwardDraftVisualId(draftId)]: position,
        }));
      }
      setSelectedNodeId(getForwardDraftVisualId(draftId));
    },
    [pushUndo],
  );

  const removeSelectedDraftTunnel = useCallback(() => {
    if (!tunnelEditor?.draftId) return;
    pushUndo();
    setDraftTunnels((items) =>
      items.filter((t) => t.draftId !== tunnelEditor.draftId),
    );
    setSelectedNodeId(null);
  }, [pushUndo, tunnelEditor]);

  const removeSelectedDraftForward = useCallback(() => {
    if (!forwardEditor?.draftId) return;
    pushUndo();
    setDraftForwards((items) =>
      items.filter((f) => f.draftId !== forwardEditor.draftId),
    );
    setSelectedNodeId(null);
  }, [forwardEditor, pushUndo]);

  // -----------------------------------------------------------------------
  // Visual connection handling
  // -----------------------------------------------------------------------

  const handleConnectStart = useCallback(
    (_: any, params: OnConnectStartParams) => {
      connectingRef.current = params;
    },
    [],
  );

  const handleConnectEnd = useCallback(
    (event: MouseEvent | TouchEvent) => {
      const params = connectingRef.current;
      connectingRef.current = null;
      if (!params?.nodeId) return;

      // Check if the connection was dropped on a node (handled by onConnect)
      const targetElement =
        event instanceof MouseEvent
          ? document.elementFromPoint(event.clientX, event.clientY)
          : document.elementFromPoint(
              (event as TouchEvent).changedTouches[0].clientX,
              (event as TouchEvent).changedTouches[0].clientY,
            );

      if (targetElement?.closest(".react-flow__node")) return;

      // Dropped on empty canvas — create a new node
      const bounds = (
        targetElement?.closest(".react-flow") as HTMLElement
      )?.getBoundingClientRect();
      if (!bounds) return;

      const clientX =
        event instanceof MouseEvent
          ? event.clientX
          : (event as TouchEvent).changedTouches[0].clientX;
      const clientY =
        event instanceof MouseEvent
          ? event.clientY
          : (event as TouchEvent).changedTouches[0].clientY;

      const position = reactFlowInstance.project({
        x: clientX - bounds.left,
        y: clientY - bounds.top,
      });

      // Server → create tunnel
      if (isSystemNodeId(params.nodeId)) {
        const nodeId = extractVisualNumericId(params.nodeId, SYSTEM_PREFIX);
        if (!nodeId) return;

        const draftId = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
        const draft = createDraftTunnel(draftId);
        draft.inNodeId = [{ chainType: 1, nodeId }];
        draft.name = `隧道 (自动创建)`;

        pushUndo();
        setDraftTunnels((items) => [...items, draft]);
        setSavedPositions((p) => ({
          ...p,
          [getTunnelDraftVisualId(draftId)]: position,
        }));
        setSelectedNodeId(getTunnelDraftVisualId(draftId));
        toast.success("拖拽创建了新隧道，已自动关联入口节点");
        return;
      }

      // Tunnel → create forward
      if (isTunnelNodeId(params.nodeId) && params.handleId === "rules") {
        const tunnelId = extractVisualNumericId(params.nodeId, TUNNEL_PREFIX);
        const tunnel = allTunnels.find(
          (t) => buildTunnelNodeId(t) === params.nodeId,
        );
        if (!tunnelId || !tunnel) {
          toast.error("请先保存隧道，再创建关联规则");
          return;
        }

        const draftId = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
        const draft = createDraftForward(draftId);
        draft.tunnelId = tunnelId;
        draft.tunnelName = tunnel.name;
        draft.name = `规则 (自动创建)`;

        pushUndo();
        setDraftForwards((items) => [...items, draft]);
        setSavedPositions((p) => ({
          ...p,
          [getForwardDraftVisualId(draftId)]: position,
        }));
        setSelectedNodeId(getForwardDraftVisualId(draftId));
        toast.success("拖拽创建了新规则，已自动关联隧道");
        return;
      }
    },
    [allTunnels, pushUndo, reactFlowInstance],
  );

  const handleConnect = useCallback(
    (connection: Connection) => {
      const source = connection.source;
      const target = connection.target;
      if (!source || !target || source === target) return;

      pushUndo();

      // Server → Tunnel = entry
      if (isSystemNodeId(source) && isTunnelNodeId(target)) {
        const nodeId = extractVisualNumericId(source, SYSTEM_PREFIX);
        if (!nodeId) return;
        applyTunnelLocalChange(target, (current) => {
          const next = removeNodeFromTunnel(current, nodeId);
          return {
            ...next,
            inNodeId: mergeOrderedNodes(
              next.inNodeId || [],
              [...(next.inNodeId || []).map((n) => n.nodeId), nodeId],
              (v) => ({ chainType: 1, nodeId: v }),
            ),
          };
        });
        setSelectedNodeId(target);
        toast.success("节点已连接为隧道入口");
        return;
      }

      // Tunnel → Server = exit
      if (isTunnelNodeId(source) && isSystemNodeId(target)) {
        const nodeId = extractVisualNumericId(target, SYSTEM_PREFIX);
        if (!nodeId) return;
        applyTunnelLocalChange(source, (current) => {
          const next = removeNodeFromTunnel(current, nodeId);
          const baseProtocol = next.outNodeId?.[0]?.protocol || "tls";
          const baseStrategy = next.outNodeId?.[0]?.strategy || "round";
          const baseConnectIp = next.outNodeId?.[0]?.connectIp || "";
          return {
            ...next,
            outNodeId: mergeOrderedNodes(
              next.outNodeId || [],
              [...(next.outNodeId || []).map((n) => n.nodeId), nodeId],
              (v) => ({
                chainType: 3,
                connectIp: baseConnectIp,
                nodeId: v,
                protocol: baseProtocol,
                strategy: baseStrategy,
              }),
            ),
            type: 2,
          };
        });
        setSelectedNodeId(source);
        toast.success("节点已连接为隧道出口");
        return;
      }

      // Tunnel → Forward = rule
      if (isTunnelNodeId(source) && isForwardNodeId(target)) {
        const tunnelId = extractVisualNumericId(source, TUNNEL_PREFIX);
        const tunnel = allTunnels.find(
          (t) => buildTunnelNodeId(t) === source,
        );
        if (!tunnelId || !tunnel) {
          toast.error("请先保存隧道，再关联规则");
          return;
        }
        applyForwardLocalChange(target, (current) => ({
          ...current,
          tunnelId,
          tunnelName: tunnel.name,
        }));
        setSelectedNodeId(target);
        toast.success("规则已绑定到隧道");
        return;
      }

      toast.error("支持的连线: 节点→隧道、隧道→节点、隧道→规则");
    },
    [
      allTunnels,
      applyForwardLocalChange,
      applyTunnelLocalChange,
      pushUndo,
    ],
  );

  // -----------------------------------------------------------------------
  // Save (single item)
  // -----------------------------------------------------------------------

  const handleTunnelSave = useCallback(async () => {
    if (!tunnelEditor) return;
    const errors = validateTunnelForm(
      tunnelEditor,
      systemNodes.map((n) => ({ id: n.id, status: n.status })),
    );
    setTunnelErrors(errors);
    if (Object.keys(errors).length > 0) return;

    setSyncing(true);
    try {
      const payload: TunnelMutationPayload = {
        ...tunnelEditor,
        chainNodes: (tunnelEditor.chainNodes || [])
          .map((g) => g.filter((n) => n.nodeId > 0))
          .filter((g) => g.length > 0),
        inIp: multilineToComma(tunnelEditor.inIp),
        inNodeId: (tunnelEditor.inNodeId || []).filter((n) => n.nodeId > 0),
        outNodeId: (tunnelEditor.outNodeId || []).filter((n) => n.nodeId > 0),
      };
      const res =
        typeof tunnelEditor.id === "number" && tunnelEditor.id > 0
          ? await updateTunnel(payload)
          : await createTunnel(payload);
      if (res.code !== 0) {
        toast.error(
          res.msg || (tunnelEditor.id ? "同步隧道失败" : "创建隧道失败"),
        );
        return;
      }
      const currentVisualId = buildTunnelNodeId(tunnelEditor);
      const currentPosition = nodes.find(
        (n) => n.id === currentVisualId,
      )?.position;
      const snapshot = await fetchSnapshot(false);
      applySnapshot(snapshot, false);
      if (tunnelEditor.draftId)
        setDraftTunnels((items) =>
          items.filter((t) => t.draftId !== tunnelEditor.draftId),
        );
      let nextId: string | null =
        tunnelEditor.id && tunnelEditor.id > 0
          ? getTunnelVisualId(tunnelEditor.id)
          : null;
      if (!nextId) {
        const created = snapshot.tunnels.find(
          (t) => t.name.trim() === tunnelEditor.name.trim(),
        );
        if (created?.id) nextId = getTunnelVisualId(created.id);
      }
      if (nextId && currentPosition)
        setSavedPositions((p) => ({ ...p, [nextId]: currentPosition }));
      setSelectedNodeId(nextId);
      toast.success(tunnelEditor.id ? "隧道已同步" : "隧道已创建");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "同步隧道失败");
    } finally {
      setSyncing(false);
    }
  }, [applySnapshot, fetchSnapshot, nodes, systemNodes, tunnelEditor]);

  const handleForwardSave = useCallback(async () => {
    if (!forwardEditor) return;
    const errors = validateForwardEditor(forwardEditor);
    setForwardErrors(errors);
    if (Object.keys(errors).length > 0) return;
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
      const res =
        typeof forwardEditor.id === "number" && forwardEditor.id > 0
          ? await updateForward(payload)
          : await createForward(payload);
      if (res.code !== 0) {
        toast.error(
          res.msg || (forwardEditor.id ? "同步规则失败" : "创建规则失败"),
        );
        return;
      }
      const currentVisualId = buildForwardNodeId(forwardEditor);
      const currentPosition = nodes.find(
        (n) => n.id === currentVisualId,
      )?.position;
      const normalizedRemote = normalizeAddressSignature(
        forwardEditor.remoteAddr,
      );
      const snapshot = await fetchSnapshot(false);
      applySnapshot(snapshot, false);
      if (forwardEditor.draftId)
        setDraftForwards((items) =>
          items.filter((f) => f.draftId !== forwardEditor.draftId),
        );
      let nextId: string | null =
        forwardEditor.id && forwardEditor.id > 0
          ? getForwardVisualId(forwardEditor.id)
          : null;
      if (!nextId) {
        const candidates = snapshot.forwards
          .filter(
            (f) =>
              f.name.trim() === forwardEditor.name.trim() &&
              f.tunnelId === forwardEditor.tunnelId,
          )
          .sort((a, b) => (b.id || 0) - (a.id || 0));
        const created =
          candidates.find(
            (f) =>
              normalizeAddressSignature(f.remoteAddr) === normalizedRemote,
          ) || candidates[0];
        if (created?.id) nextId = getForwardVisualId(created.id);
      }
      if (nextId && currentPosition)
        setSavedPositions((p) => ({ ...p, [nextId]: currentPosition }));
      setSelectedNodeId(nextId);
      toast.success(forwardEditor.id ? "规则已同步" : "规则已创建");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "同步规则失败");
    } finally {
      setSyncing(false);
    }
  }, [applySnapshot, fetchSnapshot, forwardEditor, nodes]);

  // -----------------------------------------------------------------------
  // Deploy All
  // -----------------------------------------------------------------------

  const handleDeployAll = useCallback(async () => {
    const draftsToSync = [
      ...draftTunnels.map((t) => ({ kind: "tunnel" as const, item: t })),
      ...draftForwards.map((f) => ({ kind: "forward" as const, item: f })),
    ];

    if (draftsToSync.length === 0) {
      toast("没有草稿需要部署");
      return;
    }

    setDeploying(true);
    let successCount = 0;
    let failCount = 0;

    // Deploy tunnels first (forwards depend on tunnel IDs)
    for (const draft of draftTunnels) {
      try {
        const payload: TunnelMutationPayload = {
          ...draft,
          chainNodes: (draft.chainNodes || [])
            .map((g) => g.filter((n) => n.nodeId > 0))
            .filter((g) => g.length > 0),
          inIp: multilineToComma(draft.inIp),
          inNodeId: (draft.inNodeId || []).filter((n) => n.nodeId > 0),
          outNodeId: (draft.outNodeId || []).filter((n) => n.nodeId > 0),
        };
        const res = await createTunnel(payload);
        if (res.code === 0) successCount++;
        else failCount++;
      } catch {
        failCount++;
      }
    }

    // Reload to get new tunnel IDs before deploying forwards
    if (draftTunnels.length > 0) {
      const snapshot = await fetchSnapshot(false);
      applySnapshot(snapshot, false);
    }

    for (const draft of draftForwards) {
      try {
        const payload: ForwardMutationPayload = {
          inIp: draft.inIp || "",
          inPort: draft.inPort,
          name: draft.name,
          remoteAddr: multilineToComma(draft.remoteAddr),
          strategy:
            splitMultiValue(draft.remoteAddr).length > 1
              ? draft.strategy
              : "fifo",
          tunnelId: draft.tunnelId,
        };
        const res = await createForward(payload);
        if (res.code === 0) successCount++;
        else failCount++;
      } catch {
        failCount++;
      }
    }

    // Final reload
    const finalSnapshot = await fetchSnapshot(false);
    applySnapshot(finalSnapshot, false);
    setDraftTunnels([]);
    setDraftForwards([]);
    setSelectedNodeId(null);
    setDeploying(false);

    if (failCount === 0) {
      toast.success(`部署完成: ${successCount} 项全部成功`);
    } else {
      toast.error(`部署完成: ${successCount} 成功, ${failCount} 失败`);
    }
  }, [applySnapshot, draftForwards, draftTunnels, fetchSnapshot]);

  // -----------------------------------------------------------------------
  // JSON export
  // -----------------------------------------------------------------------

  const handleExportJSON = useCallback(() => {
    const json = buildExportJSON(allTunnels, allForwards, systemNodes);
    const blob = new Blob([json], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `visual-config-${Date.now()}.json`;
    a.click();
    URL.revokeObjectURL(url);
    toast.success("配置已导出为 JSON");
  }, [allForwards, allTunnels, systemNodes]);

  // -----------------------------------------------------------------------
  // Inspector panels
  // -----------------------------------------------------------------------

  const renderSystemInspector = () => {
    if (!selectedSystemNode) return null;
    const probe = probeCache[selectedSystemNode.id];
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
              {`${Number(probe?.cpu_usage || 0).toFixed(1)}%`}
            </div>
          </div>
          <div className="rounded-xl border border-default-200 bg-default-50 p-3 dark:bg-zinc-900">
            <div className="text-xs text-default-500">内存</div>
            <div className="mt-1 text-lg font-semibold">
              {`${Number(probe?.mem_usage || 0).toFixed(1)}%`}
            </div>
          </div>
          <div className="rounded-xl border border-default-200 bg-default-50 p-3 dark:bg-zinc-900">
            <div className="text-xs text-default-500">连接数</div>
            <div className="mt-1 text-lg font-semibold">
              {probe?.connections || 0}
            </div>
          </div>
          <div className="rounded-xl border border-default-200 bg-default-50 p-3 dark:bg-zinc-900">
            <div className="text-xs text-default-500">端口范围</div>
            <div className="mt-1 text-sm font-medium">
              {String(probe?.allowed_ports || "未配置")}
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-default-200 p-4">
          <div className="text-sm font-semibold">占用端口</div>
          <div className="mt-3 flex flex-wrap gap-2">
            {(probe?.occupied_ports || []).length > 0 ? (
              probe?.occupied_ports?.map((port) => (
                <span
                  key={port}
                  className="rounded-full bg-default-100 px-2 py-1 text-xs text-default-700 dark:bg-zinc-900"
                >
                  {port}
                </span>
              ))
            ) : (
              <span className="text-xs text-default-500">
                当前没有占用端口
              </span>
            )}
          </div>
        </div>
        <div className="rounded-xl border border-default-200 p-4">
          <div className="text-sm font-semibold">关联隧道</div>
          <div className="mt-3 space-y-2">
            {relatedTunnelsForSystem.length > 0 ? (
              relatedTunnelsForSystem.map((t) => (
                <button
                  key={buildTunnelNodeId(t)}
                  className="w-full rounded-lg border border-default-200 px-3 py-2 text-left text-sm transition-colors hover:border-primary hover:bg-default-50 dark:hover:bg-zinc-900"
                  onClick={() => setSelectedNodeId(buildTunnelNodeId(t))}
                >
                  <div className="font-medium">{t.name}</div>
                  <div className="mt-1 text-xs text-default-500">
                    {getTunnelTypeLabel(t.type)}
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
              relatedForwardsForSystem.map((f) => (
                <button
                  key={buildForwardNodeId(f)}
                  className="w-full rounded-lg border border-default-200 px-3 py-2 text-left text-sm transition-colors hover:border-primary hover:bg-default-50 dark:hover:bg-zinc-900"
                  onClick={() => setSelectedNodeId(buildForwardNodeId(f))}
                >
                  <div className="font-medium">{f.name}</div>
                  <div className="mt-1 text-xs text-default-500">
                    {splitMultiValue(f.remoteAddr)[0] || "未配置"}
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
    if (!tunnelEditor) return null;
    const selectedChainNodeIds = getSelectedChainNodeIds(tunnelEditor);
    const relatedForwards = tunnelEditor.id
      ? forwards.filter((f) => f.tunnelId === tunnelEditor.id)
      : [];
    return (
      <div className="space-y-4">
        <div className="flex flex-wrap gap-2">
          <Chip color={tunnelEditor.draftId ? "warning" : "primary"}>
            {tunnelEditor.draftId
              ? "草稿"
              : getStatusLabel(tunnelEditor.status)}
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
          onChange={(e) =>
            updateTunnelEditor((c) => ({ ...c, name: e.target.value }))
          }
        />
        <div className="grid grid-cols-2 gap-3">
          <Select
            isDisabled={Boolean(tunnelEditor.id)}
            label="隧道类型"
            selectedKeys={[String(tunnelEditor.type)]}
            variant="bordered"
            onSelectionChange={(keys) => {
              const [v] = selectionToStrings(keys);
              const nextType = v === "2" ? 2 : 1;
              updateTunnelEditor((c) => ({
                ...c,
                chainNodes: nextType === 2 ? c.chainNodes : [],
                ipPreference: nextType === 2 ? c.ipPreference : "",
                outNodeId: nextType === 2 ? c.outNodeId : [],
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
              const [v] = selectionToStrings(keys);
              updateTunnelEditor((c) => ({
                ...c,
                status: v === "0" ? 0 : 1,
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
              const [v] = selectionToStrings(keys);
              updateTunnelEditor((c) => ({
                ...c,
                flow: v === "2" ? 2 : 1,
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
            onChange={(e) =>
              updateTunnelEditor((c) => ({
                ...c,
                trafficRatio: Number.parseFloat(e.target.value) || 0,
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
          onChange={(e) =>
            updateTunnelEditor((c) => ({ ...c, inIp: e.target.value }))
          }
        />
        {tunnelEditor.type === 2 && (
          <Select
            label="连接地址偏好"
            selectedKeys={
              tunnelEditor.ipPreference ? [tunnelEditor.ipPreference] : []
            }
            variant="bordered"
            onSelectionChange={(keys) => {
              const [v] = selectionToStrings(keys);
              updateTunnelEditor((c) => ({
                ...c,
                ipPreference: v || "",
              }));
            }}
          >
            <SelectItem key="v4">优先 IPv4</SelectItem>
            <SelectItem key="v6">优先 IPv6</SelectItem>
          </Select>
        )}
        <Divider />
        <Select
          disabledKeys={[
            ...systemNodes
              .filter((n) => n.status !== 1)
              .map((n) => String(n.id)),
            ...(tunnelEditor.outNodeId || [])
              .filter((n) => n.nodeId > 0)
              .map((n) => String(n.nodeId)),
            ...selectedChainNodeIds.map((n) => String(n)),
          ]}
          errorMessage={tunnelErrors.inNodeId}
          isInvalid={Boolean(tunnelErrors.inNodeId)}
          label="入口节点"
          selectedKeys={(tunnelEditor.inNodeId || []).map((n) =>
            String(n.nodeId),
          )}
          selectionMode="multiple"
          variant="bordered"
          onSelectionChange={(keys) => {
            const ids = selectionToNodeIds(keys);
            updateTunnelEditor((c) => ({
              ...c,
              inNodeId: mergeOrderedNodes(
                c.inNodeId || [],
                ids,
                (nodeId) => ({ chainType: 1, nodeId }),
              ),
            }));
          }}
        >
          {systemNodes.map((n) => (
            <SelectItem key={n.id} textValue={n.name}>
              {n.name}
            </SelectItem>
          ))}
        </Select>
        {tunnelEditor.type === 2 && (
          <>
            <Divider />
            <div className="flex items-center justify-between">
              <div className="text-sm font-semibold">转发链</div>
              <Button
                color="primary"
                size="sm"
                variant="flat"
                onPress={() =>
                  updateTunnelEditor((c) => ({
                    ...c,
                    chainNodes: [
                      ...(c.chainNodes || []),
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
                {(tunnelEditor.chainNodes || []).map((group, gi) => {
                  const groupNodeIds = group
                    .filter((n) => n.nodeId > 0)
                    .map((n) => n.nodeId);
                  const groupProto = group[0]?.protocol || "tls";
                  const groupStrat = group[0]?.strategy || "round";
                  const groupConnIp = group[0]?.connectIp || "";
                  const commonIps = getCommonIpOptions(
                    systemNodes,
                    groupNodeIds,
                  );
                  const isMulti = groupNodeIds.length > 1;
                  const otherIds = (tunnelEditor.chainNodes || [])
                    .flatMap((g2, j) =>
                      j === gi
                        ? []
                        : g2
                            .map((n) => n.nodeId)
                            .filter((id) => id > 0),
                    )
                    .map((id) => String(id));
                  return (
                    <div
                      key={`chain-${gi}`}
                      className="rounded-xl border border-default-200 p-4"
                    >
                      <div className="mb-3 flex items-center justify-between">
                        <div className="font-medium">跳点 {gi + 1}</div>
                        <Button
                          color="danger"
                          size="sm"
                          variant="light"
                          onPress={() =>
                            updateTunnelEditor((c) => ({
                              ...c,
                              chainNodes: (c.chainNodes || []).filter(
                                (_, j) => j !== gi,
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
                              .filter((n) => n.status !== 1)
                              .map((n) => String(n.id)),
                            ...(tunnelEditor.inNodeId || []).map((n) =>
                              String(n.nodeId),
                            ),
                            ...(tunnelEditor.outNodeId || [])
                              .filter((n) => n.nodeId > 0)
                              .map((n) => String(n.nodeId)),
                            ...otherIds,
                          ]}
                          label="跳点节点"
                          selectedKeys={groupNodeIds.map((n) => String(n))}
                          selectionMode="multiple"
                          variant="bordered"
                          onSelectionChange={(keys) => {
                            const ids = selectionToNodeIds(keys);
                            updateTunnelEditor((c) => {
                              const next = [...(c.chainNodes || [])];
                              const cur = next[gi] || [];
                              const curProto = cur[0]?.protocol || "tls";
                              const curStrat = cur[0]?.strategy || "round";
                              const curConn = cur[0]?.connectIp || "";
                              const real = cur.filter((n) => n.nodeId > 0);
                              const merged = mergeOrderedNodes(
                                real,
                                ids,
                                (nodeId) => ({
                                  chainType: 2,
                                  connectIp: curConn,
                                  nodeId,
                                  protocol: curProto,
                                  strategy: curStrat,
                                }),
                              );
                              next[gi] =
                                merged.length > 0
                                  ? merged
                                  : [
                                      {
                                        chainType: 2,
                                        connectIp: curConn,
                                        nodeId: -1,
                                        protocol: curProto,
                                        strategy: curStrat,
                                      },
                                    ];
                              return { ...c, chainNodes: next };
                            });
                          }}
                        >
                          {systemNodes.map((n) => (
                            <SelectItem key={n.id} textValue={n.name}>
                              {n.name}
                            </SelectItem>
                          ))}
                        </Select>
                        <div className="grid grid-cols-2 gap-3">
                          <Select
                            label="协议"
                            selectedKeys={[groupProto]}
                            variant="bordered"
                            onSelectionChange={(keys) => {
                              const [v] = selectionToStrings(keys);
                              updateTunnelEditor((c) => {
                                const next = [...(c.chainNodes || [])];
                                next[gi] = (next[gi] || []).map((n) => ({
                                  ...n,
                                  protocol: v || "tls",
                                }));
                                return { ...c, chainNodes: next };
                              });
                            }}
                          >
                            {TUNNEL_PROTOCOL_OPTIONS.map((p) => (
                              <SelectItem key={p}>
                                {p.toUpperCase()}
                              </SelectItem>
                            ))}
                          </Select>
                          <Select
                            label="策略"
                            selectedKeys={[groupStrat]}
                            variant="bordered"
                            onSelectionChange={(keys) => {
                              const [v] = selectionToStrings(keys);
                              updateTunnelEditor((c) => {
                                const next = [...(c.chainNodes || [])];
                                next[gi] = (next[gi] || []).map((n) => ({
                                  ...n,
                                  strategy: v || "round",
                                }));
                                return { ...c, chainNodes: next };
                              });
                            }}
                          >
                            {TUNNEL_STRATEGY_OPTIONS.map((s) => (
                              <SelectItem key={s}>{s}</SelectItem>
                            ))}
                          </Select>
                        </div>
                        <Select
                          isDisabled={
                            groupNodeIds.length === 0 ||
                            commonIps.length === 0 ||
                            isMulti
                          }
                          label="连接 IP"
                          placeholder={
                            isMulti
                              ? "多节点跳点不支持自定义 IP"
                              : "默认连接 IP"
                          }
                          selectedKeys={[groupConnIp || "__default__"]}
                          variant="bordered"
                          onSelectionChange={(keys) => {
                            const [v] = selectionToStrings(keys);
                            const ip = v === "__default__" ? "" : v || "";
                            updateTunnelEditor((c) => {
                              const next = [...(c.chainNodes || [])];
                              next[gi] = (next[gi] || []).map((n) => ({
                                ...n,
                                connectIp: ip,
                              }));
                              return { ...c, chainNodes: next };
                            });
                          }}
                        >
                          <SelectItem key="__default__">
                            默认连接 IP
                          </SelectItem>
                          {commonIps.map((ip) => (
                            <SelectItem key={ip}>{ip}</SelectItem>
                          ))}
                        </Select>
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : (
              <div className="rounded-xl border border-dashed border-default-300 p-4 text-sm text-default-500">
                还没有转发链跳点，可拖拽节点连接或手动添加。
              </div>
            )}
            <Divider />
            {(() => {
              const outIds = (tunnelEditor.outNodeId || [])
                .filter((n) => n.nodeId > 0)
                .map((n) => n.nodeId);
              const outProto = tunnelEditor.outNodeId?.[0]?.protocol || "tls";
              const outStrat =
                tunnelEditor.outNodeId?.[0]?.strategy || "round";
              const outConnIp =
                tunnelEditor.outNodeId?.[0]?.connectIp || "";
              const commonOutIps = getCommonIpOptions(systemNodes, outIds);
              const isMultiExit = outIds.length > 1;
              return (
                <div className="space-y-3">
                  <div className="text-sm font-semibold">出口节点</div>
                  <Select
                    disabledKeys={[
                      ...systemNodes
                        .filter((n) => n.status !== 1)
                        .map((n) => String(n.id)),
                      ...(tunnelEditor.inNodeId || []).map((n) =>
                        String(n.nodeId),
                      ),
                      ...selectedChainNodeIds.map((n) => String(n)),
                    ]}
                    errorMessage={tunnelErrors.outNodeId}
                    isInvalid={Boolean(tunnelErrors.outNodeId)}
                    label="出口节点"
                    selectedKeys={outIds.map((n) => String(n))}
                    selectionMode="multiple"
                    variant="bordered"
                    onSelectionChange={(keys) => {
                      const ids = selectionToNodeIds(keys);
                      updateTunnelEditor((c) => {
                        const cur = c.outNodeId || [];
                        const curProto = cur[0]?.protocol || "tls";
                        const curStrat = cur[0]?.strategy || "round";
                        const curConn = cur[0]?.connectIp || "";
                        const real = cur.filter((n) => n.nodeId > 0);
                        return {
                          ...c,
                          outNodeId: mergeOrderedNodes(real, ids, (nodeId) => ({
                            chainType: 3,
                            connectIp: curConn,
                            nodeId,
                            protocol: curProto,
                            strategy: curStrat,
                          })),
                        };
                      });
                    }}
                  >
                    {systemNodes.map((n) => (
                      <SelectItem key={n.id} textValue={n.name}>
                        {n.name}
                      </SelectItem>
                    ))}
                  </Select>
                  <div className="grid grid-cols-2 gap-3">
                    <Select
                      label="出口协议"
                      selectedKeys={[outProto]}
                      variant="bordered"
                      onSelectionChange={(keys) => {
                        const [v] = selectionToStrings(keys);
                        updateTunnelEditor((c) => ({
                          ...c,
                          outNodeId:
                            (c.outNodeId || []).length > 0
                              ? (c.outNodeId || []).map((n) => ({
                                  ...n,
                                  protocol: v || "tls",
                                }))
                              : [
                                  {
                                    chainType: 3,
                                    connectIp: "",
                                    nodeId: -1,
                                    protocol: v || "tls",
                                    strategy: "round",
                                  },
                                ],
                        }));
                      }}
                    >
                      {TUNNEL_PROTOCOL_OPTIONS.map((p) => (
                        <SelectItem key={p}>{p.toUpperCase()}</SelectItem>
                      ))}
                    </Select>
                    <Select
                      label="出口策略"
                      selectedKeys={[outStrat]}
                      variant="bordered"
                      onSelectionChange={(keys) => {
                        const [v] = selectionToStrings(keys);
                        updateTunnelEditor((c) => ({
                          ...c,
                          outNodeId:
                            (c.outNodeId || []).length > 0
                              ? (c.outNodeId || []).map((n) => ({
                                  ...n,
                                  strategy: v || "round",
                                }))
                              : [
                                  {
                                    chainType: 3,
                                    connectIp: "",
                                    nodeId: -1,
                                    protocol: "tls",
                                    strategy: v || "round",
                                  },
                                ],
                        }));
                      }}
                    >
                      {TUNNEL_STRATEGY_OPTIONS.map((s) => (
                        <SelectItem key={s}>{s}</SelectItem>
                      ))}
                    </Select>
                  </div>
                  <Select
                    isDisabled={
                      outIds.length === 0 ||
                      commonOutIps.length === 0 ||
                      isMultiExit
                    }
                    label="出口连接 IP"
                    placeholder={
                      isMultiExit ? "多出口不支持自定义 IP" : "默认连接 IP"
                    }
                    selectedKeys={[outConnIp || "__default__"]}
                    variant="bordered"
                    onSelectionChange={(keys) => {
                      const [v] = selectionToStrings(keys);
                      const ip = v === "__default__" ? "" : v || "";
                      updateTunnelEditor((c) => ({
                        ...c,
                        outNodeId: (c.outNodeId || []).map((n) => ({
                          ...n,
                          connectIp: ip,
                        })),
                      }));
                    }}
                  >
                    <SelectItem key="__default__">默认连接 IP</SelectItem>
                    {commonOutIps.map((ip) => (
                      <SelectItem key={ip}>{ip}</SelectItem>
                    ))}
                  </Select>
                </div>
              );
            })()}
          </>
        )}
        <div className="flex gap-2">
          {tunnelEditor.draftId && (
            <Button
              color="danger"
              variant="flat"
              onPress={removeSelectedDraftTunnel}
            >
              删除草稿
            </Button>
          )}
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
    if (!forwardEditor) return null;
    const currentTunnel =
      typeof forwardEditor.tunnelId === "number"
        ? tunnels.find((t) => t.id === forwardEditor.tunnelId) || null
        : null;
    const tunnelNodeIds = (currentTunnel?.inNodeId || []).map(
      (n) => n.nodeId,
    );
    const tunnelIpOpts = getCommonIpOptions(systemNodes, tunnelNodeIds);
    const isMultiEntry = tunnelNodeIds.length > 1;
    return (
      <div className="space-y-4">
        <div className="flex flex-wrap gap-2">
          <Chip color={forwardEditor.draftId ? "warning" : "primary"}>
            {forwardEditor.draftId
              ? "草稿"
              : getStatusLabel(forwardEditor.status)}
          </Chip>
          <Chip variant="flat">
            {currentTunnel?.name ||
              forwardEditor.tunnelName ||
              "未关联隧道"}
          </Chip>
        </div>
        <Input
          errorMessage={forwardErrors.name}
          isInvalid={Boolean(forwardErrors.name)}
          label="规则名称"
          value={forwardEditor.name}
          variant="bordered"
          onChange={(e) =>
            updateForwardEditor((c) => ({ ...c, name: e.target.value }))
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
            const [v] = selectionToStrings(keys);
            const nextId = Number.parseInt(v || "", 10);
            const sel = tunnels.find((t) => t.id === nextId);
            updateForwardEditor((c) => ({
              ...c,
              inIp: "",
              tunnelId: Number.isFinite(nextId) ? nextId : null,
              tunnelName: sel?.name,
            }));
          }}
        >
          {tunnels.map((t) => (
            <SelectItem key={t.id} textValue={t.name}>
              {t.name}
            </SelectItem>
          ))}
        </Select>
        <Input
          errorMessage={forwardErrors.inPort}
          isInvalid={Boolean(forwardErrors.inPort)}
          label="入口端口"
          placeholder="留空时自动分配"
          type="number"
          value={
            forwardEditor.inPort !== null ? String(forwardEditor.inPort) : ""
          }
          variant="bordered"
          onChange={(e) =>
            updateForwardEditor((c) => ({
              ...c,
              inPort: e.target.value
                ? Number.parseInt(e.target.value, 10)
                : null,
            }))
          }
        />
        <Select
          isDisabled={
            !forwardEditor.tunnelId ||
            tunnelIpOpts.length === 0 ||
            isMultiEntry
          }
          label="监听 IP"
          placeholder={
            isMultiEntry ? "多入口隧道不支持自定义监听 IP" : "默认入口 IP"
          }
          selectedKeys={[forwardEditor.inIp || "__default__"]}
          variant="bordered"
          onSelectionChange={(keys) => {
            const [v] = selectionToStrings(keys);
            updateForwardEditor((c) => ({
              ...c,
              inIp: v === "__default__" ? "" : v || "",
            }));
          }}
        >
          <SelectItem key="__default__">默认入口 IP</SelectItem>
          {tunnelIpOpts.map((ip) => (
            <SelectItem key={ip}>{ip}</SelectItem>
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
          onChange={(e) =>
            updateForwardEditor((c) => ({
              ...c,
              remoteAddr: e.target.value,
            }))
          }
        />
        {splitMultiValue(forwardEditor.remoteAddr).length > 1 && (
          <Select
            label="负载策略"
            selectedKeys={[forwardEditor.strategy]}
            variant="bordered"
            onSelectionChange={(keys) => {
              const [v] = selectionToStrings(keys);
              updateForwardEditor((c) => ({
                ...c,
                strategy: v || "fifo",
              }));
            }}
          >
            <SelectItem key="fifo">fifo</SelectItem>
            <SelectItem key="round">round</SelectItem>
            <SelectItem key="rand">rand</SelectItem>
            <SelectItem key="hash">hash</SelectItem>
          </Select>
        )}
        <div className="flex gap-2">
          {forwardEditor.draftId && (
            <Button
              color="danger"
              variant="flat"
              onPress={removeSelectedDraftForward}
            >
              删除草稿
            </Button>
          )}
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

  // -----------------------------------------------------------------------
  // Render
  // -----------------------------------------------------------------------

  // -----------------------------------------------------------------------
  // Workflow wizards
  // -----------------------------------------------------------------------

  const resetWizard = useCallback(() => {
    setWizardNodeA(null);
    setWizardNodeB(null);
    setWizardLanding("");
    setWizardPort("");
    setWizardName("");
    setWizardCreating(false);
  }, []);

  const handleOpenWizard = useCallback(
    (mode: "tunnel" | "port") => {
      setWizardMode(mode);
      resetWizard();
      setWizardOpen(true);
      setBrowseOpen(false);
      setImportPanelOpen(false);
    },
    [resetWizard],
  );

  // Flow 1: Tunnel forwarding — A(entry) → B(exit) tunnel + forward to landing
  const handleWizardTunnelCreate = useCallback(async () => {
    if (!wizardNodeA || !wizardNodeB || !wizardLanding.trim()) {
      toast.error("请填写完整：入口节点、出口节点、落地地址");
      return;
    }
    setWizardCreating(true);
    try {
      // Create tunnel (type=2, A entry, B exit)
      const tunnelName = wizardName.trim() || `隧道 ${wizardNodeA}→${wizardNodeB}`;
      const tunnelRes = await createTunnel({
        chainNodes: [],
        flow: 1,
        inIp: "",
        inNodeId: [{ chainType: 1, nodeId: wizardNodeA }],
        ipPreference: "",
        name: tunnelName,
        outNodeId: [
          {
            chainType: 3,
            connectIp: "",
            nodeId: wizardNodeB,
            protocol: "tls",
            strategy: "round",
          },
        ],
        status: 1,
        trafficRatio: 1,
        type: 2,
      });
      if (tunnelRes.code !== 0) {
        toast.error(tunnelRes.msg || "创建隧道失败");
        return;
      }
      // Reload to get new tunnel ID
      const snap = await fetchSnapshot(false);
      const newTunnel = snap.tunnels.find(
        (t) => t.name.trim() === tunnelName.trim(),
      );
      if (!newTunnel?.id) {
        toast.error("隧道创建成功但未找到，请手动添加规则");
        applySnapshot(snap, false);
        return;
      }
      // Create forward rule pointing to landing
      const fwdRes = await createForward({
        inIp: "",
        inPort: wizardPort ? Number.parseInt(wizardPort, 10) : null,
        name: `${tunnelName} 落地规则`,
        remoteAddr: wizardLanding.trim(),
        strategy: "fifo",
        tunnelId: newTunnel.id,
      });
      if (fwdRes.code !== 0) {
        toast.error(fwdRes.msg || "创建规则失败，隧道已创建");
      } else {
        toast.success("隧道转发链路创建成功");
      }
      // Reload all
      const finalSnap = await fetchSnapshot(false);
      applySnapshot(finalSnap, false);
      setWizardOpen(false);
      resetWizard();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "创建失败");
    } finally {
      setWizardCreating(false);
    }
  }, [
    applySnapshot,
    fetchSnapshot,
    resetWizard,
    wizardLanding,
    wizardName,
    wizardNodeA,
    wizardNodeB,
    wizardPort,
  ]);

  // Flow 2: Port forwarding chain — A forwards to B:port, B forwards to landing
  const handleWizardPortCreate = useCallback(async () => {
    if (!wizardNodeA || !wizardNodeB || !wizardLanding.trim() || !wizardPort.trim()) {
      toast.error("请填写完整：A 节点、B 节点、中转端口、落地地址");
      return;
    }
    const port = Number.parseInt(wizardPort, 10);
    if (!Number.isFinite(port) || port <= 0 || port > 65535) {
      toast.error("端口必须在 1-65535 之间");
      return;
    }
    setWizardCreating(true);
    try {
      const baseName = wizardName.trim() || `端口转发 ${wizardNodeA}→${wizardNodeB}`;
      const nodeB = systemNodes.find((n) => n.id === wizardNodeB);
      const nodeBIp =
        String(nodeB?.serverIp || "").trim() ||
        String(nodeB?.serverIpV4 || "").trim() ||
        "";

      if (!nodeBIp) {
        toast.error("B 节点没有配置 IP 地址");
        return;
      }

      // Tunnel 1: Port forwarding on A, rule forwards to B:port
      const tunnel1Res = await createTunnel({
        chainNodes: [],
        flow: 1,
        inIp: "",
        inNodeId: [{ chainType: 1, nodeId: wizardNodeA }],
        name: `${baseName} (A段)`,
        status: 1,
        trafficRatio: 1,
        type: 1,
      });
      if (tunnel1Res.code !== 0) {
        toast.error(tunnel1Res.msg || "创建 A 段隧道失败");
        return;
      }

      // Tunnel 2: Port forwarding on B, rule forwards to landing
      const tunnel2Res = await createTunnel({
        chainNodes: [],
        flow: 1,
        inIp: "",
        inNodeId: [{ chainType: 1, nodeId: wizardNodeB }],
        name: `${baseName} (B段)`,
        status: 1,
        trafficRatio: 1,
        type: 1,
      });
      if (tunnel2Res.code !== 0) {
        toast.error(tunnel2Res.msg || "创建 B 段隧道失败");
        return;
      }

      // Reload to get tunnel IDs
      const snap = await fetchSnapshot(false);
      const t1 = snap.tunnels.find(
        (t) => t.name.trim() === `${baseName} (A段)`,
      );
      const t2 = snap.tunnels.find(
        (t) => t.name.trim() === `${baseName} (B段)`,
      );

      if (!t1?.id || !t2?.id) {
        toast.error("隧道创建成功但未找到 ID，请手动添加规则");
        applySnapshot(snap, false);
        return;
      }

      // Rule 1: A tunnel forwards to B's IP:port
      const fwd1Res = await createForward({
        inIp: "",
        inPort: port,
        name: `${baseName} A→B`,
        remoteAddr: `${nodeBIp}:${port}`,
        strategy: "fifo",
        tunnelId: t1.id,
      });

      // Rule 2: B tunnel forwards to landing address
      const fwd2Res = await createForward({
        inIp: "",
        inPort: port,
        name: `${baseName} B→落地`,
        remoteAddr: wizardLanding.trim(),
        strategy: "fifo",
        tunnelId: t2.id,
      });

      if (fwd1Res.code !== 0 || fwd2Res.code !== 0) {
        toast.error("部分规则创建失败，请检查");
      } else {
        toast.success("端口转发链路创建成功 (A→B→落地)");
      }

      const finalSnap = await fetchSnapshot(false);
      applySnapshot(finalSnap, false);
      setWizardOpen(false);
      resetWizard();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "创建失败");
    } finally {
      setWizardCreating(false);
    }
  }, [
    applySnapshot,
    fetchSnapshot,
    resetWizard,
    systemNodes,
    wizardLanding,
    wizardName,
    wizardNodeA,
    wizardNodeB,
    wizardPort,
  ]);

  const draftCount = draftTunnels.length + draftForwards.length;

  // Group forwards by tunnel for the browse panel
  const tunnelGroups = useMemo(() => {
    const groups: {
      forwards: ForwardEditorState[];
      tunnel: TunnelEditorState;
    }[] = [];
    const ungrouped: ForwardEditorState[] = [];

    for (const tunnel of allTunnels) {
      const related = allForwards.filter(
        (f) =>
          (typeof tunnel.id === "number" &&
            tunnel.id > 0 &&
            f.tunnelId === tunnel.id) ||
          (tunnel.draftId &&
            f.tunnelName === tunnel.name &&
            !f.tunnelId),
      );
      groups.push({ forwards: related, tunnel });
    }

    for (const f of allForwards) {
      const hasTunnel = allTunnels.some(
        (t) =>
          (typeof t.id === "number" && t.id > 0 && f.tunnelId === t.id) ||
          (t.draftId && f.tunnelName === t.name && !f.tunnelId),
      );
      if (!hasTunnel) ungrouped.push(f);
    }

    return { groups, ungrouped };
  }, [allForwards, allTunnels]);

  // Handle entering focus mode with auto-layout for the chain
  const handleFocusTunnel = useCallback(
    (tunnelVisualId: string) => {
      setFocusedTunnelId(tunnelVisualId);
      setImportPanelOpen(false);
      setBrowseOpen(false);

      // Find the tunnel and compute a chain-specific layout
      const tunnel = allTunnels.find(
        (t) => buildTunnelNodeId(t) === tunnelVisualId,
      );
      if (!tunnel) return;

      const entries = (tunnel.inNodeId || [])
        .filter((n) => n.nodeId > 0)
        .map((n) => n.nodeId);
      const hops = (tunnel.chainNodes || []).flatMap((g) =>
        g.filter((n) => n.nodeId > 0).map((n) => n.nodeId),
      );
      const exits = (tunnel.outNodeId || [])
        .filter((n) => n.nodeId > 0)
        .map((n) => n.nodeId);
      const fwds = allForwards.filter(
        (f) => typeof tunnel.id === "number" && f.tunnelId === tunnel.id,
      );

      const positions: Record<string, XYPosition> = {};
      const colW = 380;
      let col = 0;

      // Col 0: Entry nodes
      entries.forEach((id, i) => {
        positions[getSystemVisualId(id)] = { x: col * colW + 60, y: 80 + i * 220 };
      });
      col++;

      // Col 1: Tunnel
      const tunnelY = Math.max(0, ((Math.max(entries.length, 1) - 1) * 220) / 2);
      positions[buildTunnelNodeId(tunnel)] = { x: col * colW + 60, y: 80 + tunnelY };
      col++;

      // Col 2: Hop nodes (if any)
      if (hops.length > 0) {
        hops.forEach((id, i) => {
          positions[getSystemVisualId(id)] = { x: col * colW + 60, y: 80 + i * 220 };
        });
        col++;
      }

      // Col N-1: Exit nodes (for tunnel forwarding)
      if (exits.length > 0) {
        exits.forEach((id, i) => {
          positions[getSystemVisualId(id)] = { x: col * colW + 60, y: 80 + i * 220 };
        });
        col++;
      }

      // Col N: Forward rules
      if (fwds.length > 0) {
        fwds.forEach((f, i) => {
          positions[buildForwardNodeId(f)] = { x: col * colW + 60, y: 80 + i * 200 };
        });
        col++;
      }

      // Col N+1: Landing nodes
      const fwdsWithTargets = fwds.filter(
        (f) => splitMultiValue(f.remoteAddr).length > 0,
      );
      if (fwdsWithTargets.length > 0) {
        fwdsWithTargets.forEach((f, i) => {
          positions[`landing:${buildForwardNodeId(f)}`] = {
            x: col * colW + 60,
            y: 80 + i * 180,
          };
        });
      }

      setSavedPositions((prev) => ({ ...prev, ...positions }));
      setTimeout(() => reactFlowInstance.fitView({ padding: 0.2 }), 150);
    },
    [allForwards, allTunnels, reactFlowInstance],
  );

  const handleExitFocusMode = useCallback(() => {
    setFocusedTunnelId(null);
    setTimeout(() => reactFlowInstance.fitView({ padding: 0.15 }), 100);
  }, [reactFlowInstance]);

  const renderImportPanel = () => (
    <div
      className={[
        "absolute left-0 top-0 z-20 h-full w-full max-w-[420px] border-r border-zinc-800 bg-zinc-950/98 shadow-2xl backdrop-blur transition-transform duration-300",
        importPanelOpen
          ? "translate-x-0"
          : "-translate-x-full pointer-events-none",
      ].join(" ")}
    >
      <div className="flex items-center justify-between border-b border-zinc-800 px-5 py-4">
        <div>
          <div className="text-[10px] uppercase tracking-[0.2em] text-zinc-600">
            Import & Edit
          </div>
          <div className="mt-1 text-base font-semibold text-zinc-100">
            导入隧道组编辑
          </div>
        </div>
        <Button
          isIconOnly
          onPress={() => setImportPanelOpen(false)}
          variant="light"
        >
          ×
        </Button>
      </div>
      <div className="h-[calc(100%-81px)] overflow-y-auto p-4 space-y-3">
        <div className="rounded-xl border border-sky-500/20 bg-sky-500/5 p-3 text-xs text-sky-300">
          选择一个隧道组，画布将自动聚焦并展示完整转发链路：
          <span className="font-semibold"> 入口 → 隧道 → 跳点 → 出口 → 规则(落地)</span>
          。可直接编辑后同步到服务器。
        </div>

        {allTunnels.length === 0 ? (
          <div className="rounded-xl border border-dashed border-zinc-800 p-6 text-center text-sm text-zinc-600">
            服务器上暂无隧道。请先通过「新增隧道」创建。
          </div>
        ) : (
          <div className="space-y-2">
            {allTunnels.map((tunnel) => {
              const tId = buildTunnelNodeId(tunnel);
              const isFocused = focusedTunnelId === tId;
              const isDraft = Boolean(tunnel.draftId);
              const isTF = tunnel.type === 2;
              const entries = (tunnel.inNodeId || []).filter(
                (n) => n.nodeId > 0,
              );
              const exits = (tunnel.outNodeId || []).filter(
                (n) => n.nodeId > 0,
              );
              const hopCount = (tunnel.chainNodes || []).filter((g) =>
                g.some((n) => n.nodeId > 0),
              ).length;
              const fwdCount = allForwards.filter(
                (f) =>
                  typeof tunnel.id === "number" &&
                  tunnel.id > 0 &&
                  f.tunnelId === tunnel.id,
              ).length;

              return (
                <div
                  key={tId}
                  className={[
                    "overflow-hidden rounded-xl border transition-all",
                    isFocused
                      ? "border-sky-500/50 bg-sky-500/5 ring-1 ring-sky-500/20"
                      : "border-zinc-800 hover:border-zinc-700",
                  ].join(" ")}
                >
                  {/* Tunnel header */}
                  <div className="flex items-center justify-between px-3 py-2.5">
                    <div className="flex items-center gap-2 min-w-0">
                      <span
                        className={[
                          "h-2.5 w-2.5 flex-shrink-0 rounded-full",
                          isDraft
                            ? "bg-amber-500"
                            : tunnel.status === 1
                              ? "bg-emerald-500"
                              : "bg-zinc-600",
                        ].join(" ")}
                      />
                      <span className="truncate text-sm font-semibold text-zinc-200">
                        {tunnel.name || "未命名"}
                      </span>
                      <span
                        className={[
                          "flex-shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium",
                          isTF
                            ? "bg-sky-500/15 text-sky-400"
                            : "bg-violet-500/15 text-violet-400",
                        ].join(" ")}
                      >
                        {isTF ? "隧道转发" : "端口转发"}
                      </span>
                    </div>
                    <Button
                      color={isFocused ? "warning" : "primary"}
                      onPress={() =>
                        isFocused
                          ? handleExitFocusMode()
                          : handleFocusTunnel(tId)
                      }
                      size="sm"
                      variant={isFocused ? "flat" : "solid"}
                    >
                      {isFocused ? "退出聚焦" : "聚焦编辑"}
                    </Button>
                  </div>

                  {/* Chain preview */}
                  <div className="border-t border-zinc-800/40 bg-zinc-900/30 px-3 py-2.5">
                    <div className="flex items-center gap-1.5 flex-wrap text-[10px]">
                      {/* Entry nodes */}
                      {entries.length > 0 ? (
                        entries.map((n, i) => {
                          const sys = systemNodes.find(
                            (s) => s.id === n.nodeId,
                          );
                          return (
                            <span key={`e-${i}`} className="flex items-center gap-1">
                              {i > 0 && (
                                <span className="text-zinc-700">/</span>
                              )}
                              <span className="rounded bg-emerald-500/15 px-1.5 py-0.5 font-medium text-emerald-400">
                                {sys?.name || `#${n.nodeId}`}
                              </span>
                            </span>
                          );
                        })
                      ) : (
                        <span className="rounded bg-zinc-800 px-1.5 py-0.5 text-zinc-600">
                          无入口
                        </span>
                      )}

                      <span className="text-zinc-600">→</span>

                      {/* Tunnel */}
                      <span className="rounded bg-sky-500/15 px-1.5 py-0.5 font-medium text-sky-400">
                        {tunnel.name}
                      </span>

                      {/* Hops */}
                      {hopCount > 0 && (
                        <>
                          <span className="text-zinc-600">→</span>
                          <span className="rounded bg-sky-500/10 px-1.5 py-0.5 text-sky-500">
                            {hopCount} 跳
                          </span>
                        </>
                      )}

                      {/* Exits */}
                      {isTF && (
                        <>
                          <span className="text-zinc-600">→</span>
                          {exits.length > 0 ? (
                            exits.map((n, i) => {
                              const sys = systemNodes.find(
                                (s) => s.id === n.nodeId,
                              );
                              return (
                                <span
                                  key={`x-${i}`}
                                  className="flex items-center gap-1"
                                >
                                  {i > 0 && (
                                    <span className="text-zinc-700">/</span>
                                  )}
                                  <span className="rounded bg-teal-500/15 px-1.5 py-0.5 font-medium text-teal-400">
                                    {sys?.name || `#${n.nodeId}`}
                                  </span>
                                </span>
                              );
                            })
                          ) : (
                            <span className="rounded bg-zinc-800 px-1.5 py-0.5 text-zinc-600">
                              无出口
                            </span>
                          )}
                        </>
                      )}

                      {/* Forward count */}
                      <span className="text-zinc-600">→</span>
                      <span className="rounded bg-amber-500/15 px-1.5 py-0.5 font-medium text-amber-400">
                        {fwdCount} 规则
                      </span>
                    </div>
                  </div>

                  {/* Expanded forwards when focused */}
                  {isFocused && focusedChain && (
                    <div className="border-t border-zinc-800/40">
                      {focusedChain.forwards.length > 0 ? (
                        <div className="divide-y divide-zinc-800/30">
                          {focusedChain.forwards.map((fwd) => {
                            const fId = buildForwardNodeId(fwd);
                            const addrs = splitMultiValue(fwd.remoteAddr);
                            return (
                              <div
                                key={fId}
                                className="flex items-start justify-between px-3 py-2"
                              >
                                <div className="min-w-0 flex-1">
                                  <div className="flex items-center gap-1.5">
                                    <span className="h-1.5 w-1.5 rounded-full bg-amber-500" />
                                    <span className="text-xs font-medium text-zinc-300">
                                      {fwd.name}
                                    </span>
                                  </div>
                                  <div className="mt-1 pl-3 text-[10px] text-zinc-500">
                                    <span className="font-mono text-emerald-500">
                                      :{fwd.inPort || "auto"}
                                    </span>
                                    <span className="mx-1 text-zinc-700">→</span>
                                    {addrs.length > 0 ? (
                                      <span className="space-y-0.5">
                                        {addrs.map((addr, ai) => (
                                          <span
                                            key={ai}
                                            className="mr-1 inline-block rounded bg-zinc-800 px-1 py-0.5 font-mono text-amber-400"
                                          >
                                            {addr}
                                          </span>
                                        ))}
                                      </span>
                                    ) : (
                                      <span className="italic text-zinc-700">
                                        未配置
                                      </span>
                                    )}
                                  </div>
                                </div>
                                <Button
                                  onPress={() => {
                                    setSelectedNodeId(fId);
                                    const node = nodes.find(
                                      (n) => n.id === fId,
                                    );
                                    if (node)
                                      reactFlowInstance.setCenter(
                                        node.position.x + 130,
                                        node.position.y + 80,
                                        { duration: 300, zoom: 1.2 },
                                      );
                                  }}
                                  size="sm"
                                  variant="light"
                                >
                                  编辑
                                </Button>
                              </div>
                            );
                          })}
                        </div>
                      ) : (
                        <div className="px-3 py-3 text-[11px] italic text-zinc-700">
                          此隧道暂无关联规则。可拖拽创建或从「新增规则」添加。
                        </div>
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );

  const focusNode = useCallback(
    (visualId: string) => {
      setSelectedNodeId(visualId);
      setBrowseOpen(false);
      const node = nodes.find((n) => n.id === visualId);
      if (node) {
        reactFlowInstance.setCenter(node.position.x + 130, node.position.y + 80, {
          duration: 400,
          zoom: 1,
        });
      }
    },
    [nodes, reactFlowInstance],
  );

  const renderBrowsePanel = () => (
    <div
      className={[
        "absolute left-0 top-0 z-20 h-full w-full max-w-[400px] border-r border-zinc-800 bg-zinc-950/98 shadow-2xl backdrop-blur transition-transform duration-300",
        browseOpen ? "translate-x-0" : "-translate-x-full pointer-events-none",
      ].join(" ")}
    >
      <div className="flex items-center justify-between border-b border-zinc-800 px-5 py-4">
        <div>
          <div className="text-[10px] uppercase tracking-[0.2em] text-zinc-600">
            Browse
          </div>
          <div className="mt-1 text-base font-semibold text-zinc-100">
            隧道与规则管理
          </div>
        </div>
        <Button isIconOnly onPress={() => setBrowseOpen(false)} variant="light">
          ×
        </Button>
      </div>
      <div className="h-[calc(100%-81px)] overflow-y-auto p-4">
        {tunnelGroups.groups.length === 0 && tunnelGroups.ungrouped.length === 0 ? (
          <div className="rounded-xl border border-dashed border-zinc-800 p-6 text-center text-sm text-zinc-600">
            暂无隧道或规则。点击「新增隧道」或从节点拖拽来创建。
          </div>
        ) : (
          <div className="space-y-4">
            {tunnelGroups.groups.map((group) => {
              const tId = buildTunnelNodeId(group.tunnel);
              const isTunnelForward = group.tunnel.type === 2;
              const isDraft = Boolean(group.tunnel.draftId);
              return (
                <div
                  key={tId}
                  className="overflow-hidden rounded-xl border border-zinc-800"
                >
                  {/* Tunnel header */}
                  <div className="flex items-center justify-between border-b border-zinc-800/60 bg-zinc-900/50 px-3 py-2.5">
                    <div className="flex items-center gap-2 min-w-0">
                      <span
                        className={[
                          "h-2 w-2 flex-shrink-0 rounded-full",
                          isDraft
                            ? "bg-amber-500"
                            : group.tunnel.status === 1
                              ? "bg-emerald-500"
                              : "bg-zinc-600",
                        ].join(" ")}
                      />
                      <span className="truncate text-sm font-semibold text-zinc-200">
                        {group.tunnel.name || "未命名隧道"}
                      </span>
                      <span
                        className={[
                          "flex-shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium",
                          isTunnelForward
                            ? "bg-sky-500/15 text-sky-400"
                            : "bg-violet-500/15 text-violet-400",
                        ].join(" ")}
                      >
                        {isTunnelForward ? "隧道转发" : "端口转发"}
                      </span>
                      {isDraft && (
                        <span className="rounded bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-medium text-amber-400">
                          草稿
                        </span>
                      )}
                    </div>
                    <Button
                      color="primary"
                      onPress={() => focusNode(tId)}
                      size="sm"
                      variant="flat"
                    >
                      编辑
                    </Button>
                  </div>
                  {/* Tunnel details */}
                  <div className="border-b border-zinc-800/40 bg-zinc-950 px-3 py-2 text-[11px] text-zinc-500">
                    <div className="flex flex-wrap gap-x-4 gap-y-1">
                      <span>
                        入口:{" "}
                        {(group.tunnel.inNodeId || [])
                          .filter((n) => n.nodeId > 0)
                          .map(
                            (n) =>
                              systemNodes.find((s) => s.id === n.nodeId)
                                ?.name || `#${n.nodeId}`,
                          )
                          .join(", ") || "未配置"}
                      </span>
                      {isTunnelForward && (
                        <span>
                          出口:{" "}
                          {(group.tunnel.outNodeId || [])
                            .filter((n) => n.nodeId > 0)
                            .map(
                              (n) =>
                                systemNodes.find((s) => s.id === n.nodeId)
                                  ?.name || `#${n.nodeId}`,
                            )
                            .join(", ") || "未配置"}
                        </span>
                      )}
                      <span>倍率: {group.tunnel.trafficRatio}x</span>
                      <span>
                        流量: {group.tunnel.flow === 2 ? "双向" : "单向"}
                      </span>
                    </div>
                  </div>
                  {/* Associated forwards */}
                  {group.forwards.length > 0 ? (
                    <div className="divide-y divide-zinc-800/40">
                      {group.forwards.map((fwd) => {
                        const fId = buildForwardNodeId(fwd);
                        const fwdDraft = Boolean(fwd.draftId);
                        const addrs = splitMultiValue(fwd.remoteAddr);
                        return (
                          <div
                            key={fId}
                            className="flex items-center justify-between bg-zinc-950 px-3 py-2"
                          >
                            <div className="min-w-0 flex-1">
                              <div className="flex items-center gap-2">
                                <span className="h-1.5 w-1.5 flex-shrink-0 rounded-full bg-amber-500" />
                                <span className="truncate text-xs font-medium text-zinc-300">
                                  {fwd.name || "未命名规则"}
                                </span>
                                {fwdDraft && (
                                  <span className="rounded bg-amber-500/15 px-1 py-0.5 text-[9px] text-amber-400">
                                    草稿
                                  </span>
                                )}
                              </div>
                              <div className="mt-0.5 flex items-center gap-1 pl-3.5 text-[10px] text-zinc-600">
                                <span className="font-mono">
                                  :{fwd.inPort || "auto"}
                                </span>
                                <span>→</span>
                                <span className="truncate font-mono">
                                  {addrs[0] || "未配置"}
                                  {addrs.length > 1 &&
                                    ` (+${addrs.length - 1})`}
                                </span>
                                {addrs.length > 1 && (
                                  <span className="text-zinc-700">
                                    [{fwd.strategy}]
                                  </span>
                                )}
                              </div>
                            </div>
                            <Button
                              onPress={() => focusNode(fId)}
                              size="sm"
                              variant="light"
                            >
                              编辑
                            </Button>
                          </div>
                        );
                      })}
                    </div>
                  ) : (
                    <div className="bg-zinc-950 px-3 py-2 text-[11px] italic text-zinc-700">
                      此隧道暂无关联规则
                    </div>
                  )}
                </div>
              );
            })}

            {/* Ungrouped forwards */}
            {tunnelGroups.ungrouped.length > 0 && (
              <div className="overflow-hidden rounded-xl border border-zinc-800">
                <div className="border-b border-zinc-800/60 bg-zinc-900/50 px-3 py-2.5">
                  <span className="text-sm font-semibold text-zinc-400">
                    未关联隧道的规则
                  </span>
                </div>
                <div className="divide-y divide-zinc-800/40">
                  {tunnelGroups.ungrouped.map((fwd) => {
                    const fId = buildForwardNodeId(fwd);
                    const addrs = splitMultiValue(fwd.remoteAddr);
                    return (
                      <div
                        key={fId}
                        className="flex items-center justify-between bg-zinc-950 px-3 py-2"
                      >
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <span className="h-1.5 w-1.5 flex-shrink-0 rounded-full bg-amber-500" />
                            <span className="truncate text-xs font-medium text-zinc-300">
                              {fwd.name}
                            </span>
                          </div>
                          <div className="mt-0.5 pl-3.5 text-[10px] text-zinc-600">
                            <span className="font-mono">
                              :{fwd.inPort || "auto"} →{" "}
                              {addrs[0] || "未配置"}
                            </span>
                          </div>
                        </div>
                        <Button
                          onPress={() => focusNode(fId)}
                          size="sm"
                          variant="light"
                        >
                          编辑
                        </Button>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );

  return (
    <div className="relative h-full w-full overflow-hidden rounded-2xl border border-default-200 bg-zinc-950">
      {/* Toolbar */}
      <div className="absolute left-4 top-4 z-10 flex flex-wrap items-center gap-2 rounded-xl border border-zinc-800 bg-zinc-950/95 p-2.5 shadow-2xl backdrop-blur">
        <Button
          color="primary"
          isLoading={layoutSaving}
          onPress={handleSaveLayout}
          size="sm"
        >
          保存布局
        </Button>
        <Button
          onPress={() => void loadData({ reloadLayout: true })}
          size="sm"
          variant="flat"
        >
          刷新
        </Button>
        <Divider className="h-6" orientation="vertical" />
        <Button onPress={() => handleAddTunnel()} size="sm" variant="flat">
          新增隧道
        </Button>
        <Button onPress={() => handleAddForward()} size="sm" variant="flat">
          新增规则
        </Button>
        <Button
          color={wizardOpen ? "secondary" : "default"}
          onPress={() => handleOpenWizard("tunnel")}
          size="sm"
          variant={wizardOpen ? "solid" : "flat"}
        >
          快速创建
        </Button>
        <Divider className="h-6" orientation="vertical" />
        <Button onPress={handleAutoLayout} size="sm" variant="flat">
          自动排列
        </Button>
        <Button onPress={handleExportJSON} size="sm" variant="flat">
          导出 JSON
        </Button>
        <Button
          color={browseOpen ? "primary" : "default"}
          onPress={() => setBrowseOpen((v) => !v)}
          size="sm"
          variant={browseOpen ? "solid" : "flat"}
        >
          查看编辑
        </Button>
        <Button
          color={importPanelOpen ? "secondary" : "default"}
          onPress={() => {
            setImportPanelOpen((v) => !v);
            if (!importPanelOpen) setBrowseOpen(false);
          }}
          size="sm"
          variant={importPanelOpen ? "solid" : "flat"}
        >
          导入编辑
        </Button>
        {draftCount > 0 && (
          <>
            <Divider className="h-6" orientation="vertical" />
            <Button
              color="success"
              isLoading={deploying}
              onPress={handleDeployAll}
              size="sm"
            >
              部署全部 ({draftCount})
            </Button>
          </>
        )}
        <Divider className="h-6" orientation="vertical" />
        <Button
          isDisabled={undoStack.length === 0}
          onPress={handleUndo}
          size="sm"
          variant="light"
        >
          Undo
        </Button>
        <Button
          isDisabled={redoStack.length === 0}
          onPress={handleRedo}
          size="sm"
          variant="light"
        >
          Redo
        </Button>
      </div>

      {/* Canvas */}
      <ReactFlow
        className="h-full w-full"
        defaultEdgeOptions={{
          markerEnd: { type: MarkerType.ArrowClosed },
          type: "portMapping",
        }}
        edges={edges}
        edgeTypes={edgeTypes}
        fitView
        nodeTypes={nodeTypes}
        nodes={nodes}
        nodesConnectable
        onConnect={handleConnect}
        onConnectStart={handleConnectStart}
        onConnectEnd={handleConnectEnd as any}
        onEdgesChange={onEdgesChange}
        onNodeClick={(_, node) => setSelectedNodeId(node.id)}
        onNodeDragStop={(_, node) =>
          setSavedPositions((p) => ({ ...p, [node.id]: node.position }))
        }
        onNodesChange={onNodesChange}
        onPaneClick={() => setSelectedNodeId(null)}
      >
        <MiniMap
          nodeColor={(node) => {
            const kind = node.type;
            if (kind === "comfyServer") return "#22c55e";
            if (kind === "comfyTunnel") return "#0ea5e9";
            if (kind === "comfyLanding") return "#f43f5e";
            return "#f59e0b";
          }}
          pannable
          style={{ backgroundColor: "#18181b" }}
          zoomable
        />
        <Controls />
        <Background color="#3f3f46" gap={20} />
      </ReactFlow>

      {/* Legend */}
      <div className="absolute bottom-4 left-4 z-10 flex items-center gap-4 rounded-xl border border-zinc-800 bg-zinc-950/95 px-4 py-2.5 text-[10px] font-medium uppercase tracking-wider text-zinc-600 shadow-lg backdrop-blur">
        <span className="flex items-center gap-1.5">
          <span className="h-2 w-2 rounded-full bg-emerald-500" />
          Server
        </span>
        <span className="flex items-center gap-1.5">
          <span className="h-2 w-2 rounded-full bg-sky-500" />
          Tunnel
        </span>
        <span className="flex items-center gap-1.5">
          <span className="h-2 w-2 rounded-full bg-amber-500" />
          Rule
        </span>
        <span className="flex items-center gap-1.5">
          <span className="h-2 w-2 rounded-full bg-rose-500" />
          Landing
        </span>
        <Divider className="h-3" orientation="vertical" />
        <span>
          Drag from a node to empty space to create linked items
        </span>
      </div>

      {/* Focus mode banner */}
      {focusedTunnel && (
        <div className="absolute left-1/2 top-20 z-10 -translate-x-1/2 flex items-center gap-3 rounded-xl border border-sky-500/30 bg-zinc-950/95 px-4 py-2.5 shadow-2xl backdrop-blur">
          <span className="h-2 w-2 animate-pulse rounded-full bg-sky-500" />
          <span className="text-xs font-medium text-sky-300">
            聚焦模式: {focusedTunnel.name}
          </span>
          <span className="text-[10px] text-zinc-600">
            仅显示此隧道的转发链路
          </span>
          <Button
            color="warning"
            onPress={handleExitFocusMode}
            size="sm"
            variant="flat"
          >
            退出聚焦
          </Button>
        </div>
      )}

      {/* Wizard Panel */}
      <div
        className={[
          "absolute left-0 top-0 z-20 h-full w-full max-w-[400px] border-r border-zinc-800 bg-zinc-950/98 shadow-2xl backdrop-blur transition-transform duration-300",
          wizardOpen
            ? "translate-x-0"
            : "-translate-x-full pointer-events-none",
        ].join(" ")}
      >
        <div className="flex items-center justify-between border-b border-zinc-800 px-5 py-4">
          <div>
            <div className="text-[10px] uppercase tracking-[0.2em] text-zinc-600">
              Quick Create
            </div>
            <div className="mt-1 text-base font-semibold text-zinc-100">
              快速创建转发链路
            </div>
          </div>
          <Button
            isIconOnly
            onPress={() => setWizardOpen(false)}
            variant="light"
          >
            ×
          </Button>
        </div>
        <div className="h-[calc(100%-81px)] overflow-y-auto p-4 space-y-4">
          {/* Mode tabs */}
          <div className="flex rounded-lg border border-zinc-800 overflow-hidden">
            <button
              className={[
                "flex-1 px-3 py-2 text-xs font-semibold transition-colors",
                wizardMode === "tunnel"
                  ? "bg-sky-500/15 text-sky-400"
                  : "text-zinc-500 hover:text-zinc-300",
              ].join(" ")}
              onClick={() => {
                setWizardMode("tunnel");
                resetWizard();
              }}
            >
              隧道转发
            </button>
            <button
              className={[
                "flex-1 px-3 py-2 text-xs font-semibold transition-colors border-l border-zinc-800",
                wizardMode === "port"
                  ? "bg-violet-500/15 text-violet-400"
                  : "text-zinc-500 hover:text-zinc-300",
              ].join(" ")}
              onClick={() => {
                setWizardMode("port");
                resetWizard();
              }}
            >
              端口转发链
            </button>
          </div>

          {/* Flow description */}
          {wizardMode === "tunnel" ? (
            <div className="rounded-xl border border-sky-500/20 bg-sky-500/5 p-3 text-xs text-sky-300 space-y-1">
              <div className="font-semibold">隧道转发模式</div>
              <div>A 节点(入口) → B 节点(出口) 隧道 + 规则指向落地机器</div>
              <div className="flex items-center gap-1 mt-2 text-[10px]">
                <span className="rounded bg-emerald-500/15 px-1.5 py-0.5 text-emerald-400">A 入口</span>
                <span className="text-zinc-600">→</span>
                <span className="rounded bg-sky-500/15 px-1.5 py-0.5 text-sky-400">隧道</span>
                <span className="text-zinc-600">→</span>
                <span className="rounded bg-teal-500/15 px-1.5 py-0.5 text-teal-400">B 出口</span>
                <span className="text-zinc-600">→</span>
                <span className="rounded bg-rose-500/15 px-1.5 py-0.5 text-rose-400">落地</span>
              </div>
            </div>
          ) : (
            <div className="rounded-xl border border-violet-500/20 bg-violet-500/5 p-3 text-xs text-violet-300 space-y-1">
              <div className="font-semibold">端口转发链模式</div>
              <div>
                创建两段端口转发：A 节点转发到 B 节点端口，B 节点转发到落地机器
              </div>
              <div className="flex items-center gap-1 mt-2 text-[10px] flex-wrap">
                <span className="rounded bg-emerald-500/15 px-1.5 py-0.5 text-emerald-400">A :port</span>
                <span className="text-zinc-600">→</span>
                <span className="rounded bg-violet-500/15 px-1.5 py-0.5 text-violet-400">B IP:port</span>
                <span className="text-zinc-600">→</span>
                <span className="rounded bg-rose-500/15 px-1.5 py-0.5 text-rose-400">落地 IP:port</span>
              </div>
            </div>
          )}

          {/* Form */}
          <div className="space-y-3">
            <Input
              label="链路名称"
              placeholder={
                wizardMode === "tunnel"
                  ? "隧道转发链路"
                  : "端口转发链路"
              }
              value={wizardName}
              variant="bordered"
              onChange={(e) => setWizardName(e.target.value)}
            />
            <Select
              label={wizardMode === "tunnel" ? "A 入口节点" : "A 节点"}
              selectedKeys={wizardNodeA ? [String(wizardNodeA)] : []}
              variant="bordered"
              onSelectionChange={(keys) => {
                const [v] = selectionToStrings(keys);
                setWizardNodeA(v ? Number.parseInt(v, 10) : null);
              }}
            >
              {systemNodes
                .filter((n) => n.status === 1)
                .map((n) => (
                  <SelectItem key={n.id} textValue={n.name}>
                    <div className="flex items-center justify-between w-full">
                      <span>{n.name}</span>
                      <span className="text-xs text-zinc-500">
                        {String(n.serverIp || n.serverIpV4 || "").trim()}
                      </span>
                    </div>
                  </SelectItem>
                ))}
            </Select>
            <Select
              label={wizardMode === "tunnel" ? "B 出口节点" : "B 节点 (中转)"}
              disabledKeys={wizardNodeA ? [String(wizardNodeA)] : []}
              selectedKeys={wizardNodeB ? [String(wizardNodeB)] : []}
              variant="bordered"
              onSelectionChange={(keys) => {
                const [v] = selectionToStrings(keys);
                setWizardNodeB(v ? Number.parseInt(v, 10) : null);
              }}
            >
              {systemNodes
                .filter((n) => n.status === 1)
                .map((n) => (
                  <SelectItem key={n.id} textValue={n.name}>
                    <div className="flex items-center justify-between w-full">
                      <span>{n.name}</span>
                      <span className="text-xs text-zinc-500">
                        {String(n.serverIp || n.serverIpV4 || "").trim()}
                      </span>
                    </div>
                  </SelectItem>
                ))}
            </Select>
            {wizardMode === "port" && (
              <Input
                label="中转端口"
                placeholder="如 8080"
                type="number"
                value={wizardPort}
                variant="bordered"
                onChange={(e) => setWizardPort(e.target.value)}
              />
            )}
            {wizardMode === "tunnel" && (
              <Input
                label="入口端口 (可选)"
                placeholder="留空自动分配"
                type="number"
                value={wizardPort}
                variant="bordered"
                onChange={(e) => setWizardPort(e.target.value)}
              />
            )}
            <Textarea
              description="落地服务器的目标地址，如 1.2.3.4:443。多个地址一行一个。"
              label="落地地址"
              maxRows={4}
              minRows={2}
              value={wizardLanding}
              variant="bordered"
              onChange={(e) => setWizardLanding(e.target.value)}
            />
          </div>

          {/* Chain preview */}
          {wizardNodeA && wizardNodeB && wizardLanding.trim() && (
            <div className="rounded-xl border border-zinc-800 bg-zinc-900/30 p-3">
              <div className="text-[10px] font-medium uppercase tracking-wider text-zinc-600 mb-2">
                Preview
              </div>
              {wizardMode === "tunnel" ? (
                <div className="flex items-center gap-1.5 flex-wrap text-[10px]">
                  <span className="rounded bg-emerald-500/15 px-1.5 py-0.5 font-medium text-emerald-400">
                    {systemNodes.find((n) => n.id === wizardNodeA)?.name || "A"}
                  </span>
                  <span className="text-zinc-600">→ 隧道 →</span>
                  <span className="rounded bg-teal-500/15 px-1.5 py-0.5 font-medium text-teal-400">
                    {systemNodes.find((n) => n.id === wizardNodeB)?.name || "B"}
                  </span>
                  <span className="text-zinc-600">→ 规则 →</span>
                  {splitMultiValue(wizardLanding).map((addr, i) => (
                    <span
                      key={i}
                      className="rounded bg-rose-500/15 px-1.5 py-0.5 font-mono text-rose-400"
                    >
                      {addr}
                    </span>
                  ))}
                </div>
              ) : (
                <div className="space-y-1.5 text-[10px]">
                  <div className="flex items-center gap-1.5 flex-wrap">
                    <span className="text-zinc-500">A段:</span>
                    <span className="rounded bg-emerald-500/15 px-1.5 py-0.5 font-medium text-emerald-400">
                      {systemNodes.find((n) => n.id === wizardNodeA)?.name || "A"}
                    </span>
                    <span className="text-zinc-600">:{wizardPort || "?"} →</span>
                    <span className="rounded bg-violet-500/15 px-1.5 py-0.5 font-mono text-violet-400">
                      {(() => {
                        const nb = systemNodes.find(
                          (n) => n.id === wizardNodeB,
                        );
                        const ip = String(
                          nb?.serverIp || nb?.serverIpV4 || "B",
                        ).trim();
                        return `${ip}:${wizardPort || "?"}`;
                      })()}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5 flex-wrap">
                    <span className="text-zinc-500">B段:</span>
                    <span className="rounded bg-violet-500/15 px-1.5 py-0.5 font-medium text-violet-400">
                      {systemNodes.find((n) => n.id === wizardNodeB)?.name || "B"}
                    </span>
                    <span className="text-zinc-600">:{wizardPort || "?"} →</span>
                    {splitMultiValue(wizardLanding).map((addr, i) => (
                      <span
                        key={i}
                        className="rounded bg-rose-500/15 px-1.5 py-0.5 font-mono text-rose-400"
                      >
                        {addr}
                      </span>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}

          {/* Create button */}
          <Button
            className="w-full"
            color="primary"
            isLoading={wizardCreating}
            onPress={
              wizardMode === "tunnel"
                ? handleWizardTunnelCreate
                : handleWizardPortCreate
            }
            size="lg"
          >
            {wizardMode === "tunnel"
              ? "创建隧道转发链路"
              : "创建端口转发链路"}
          </Button>
        </div>
      </div>

      {/* Import Panel */}
      {renderImportPanel()}

      {/* Browse Panel */}
      {renderBrowsePanel()}

      {/* Inspector Panel */}
      <div
        className={[
          "absolute right-0 top-0 z-20 h-full w-full max-w-[420px] border-l border-zinc-800 bg-zinc-950/98 shadow-2xl backdrop-blur transition-transform duration-300",
          selectedNodeId
            ? "translate-x-0"
            : "translate-x-full pointer-events-none",
        ].join(" ")}
      >
        <div className="flex items-center justify-between border-b border-zinc-800 px-5 py-4">
          <div>
            <div className="text-[10px] uppercase tracking-[0.2em] text-zinc-600">
              Inspector
            </div>
            <div className="mt-1 text-base font-semibold text-zinc-100">
              {selectedSystemNode
                ? selectedSystemNode.name
                : tunnelEditor
                  ? tunnelEditor.name || "隧道编辑"
                  : forwardEditor
                    ? forwardEditor.name || "规则编辑"
                    : "—"}
            </div>
          </div>
          <Button
            isIconOnly
            onPress={() => setSelectedNodeId(null)}
            variant="light"
          >
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
                : null}
        </div>
      </div>

      {/* Loading overlay */}
      {loading && (
        <div className="absolute inset-0 z-30 flex items-center justify-center bg-zinc-950/80 backdrop-blur-sm">
          <div className="flex items-center gap-3 rounded-xl border border-zinc-800 bg-zinc-950 px-6 py-4 shadow-2xl">
            <div className="flex gap-1">
              <div className="h-2 w-2 animate-bounce rounded-full bg-emerald-500 [animation-delay:-0.3s]" />
              <div className="h-2 w-2 animate-bounce rounded-full bg-sky-500 [animation-delay:-0.15s]" />
              <div className="h-2 w-2 animate-bounce rounded-full bg-amber-500" />
            </div>
            <span className="text-sm text-zinc-400">
              Loading visual editor...
            </span>
          </div>
        </div>
      )}
    </div>
  );
}
