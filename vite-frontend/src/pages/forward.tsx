import type { SpeedLimitApiItem } from "@/api/types";

import { useState, useEffect, useMemo, useRef } from "react";
import toast from "react-hot-toast";
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  useSensor,
  useSensors,
  DragEndEvent,
} from "@dnd-kit/core";
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  rectSortingStrategy,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

import { SearchBar } from "@/components/search-bar";
import { AnimatedPage } from "@/components/animated-page";
import { Card, CardBody, CardHeader } from "@/shadcn-bridge/heroui/card";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Input } from "@/shadcn-bridge/heroui/input";
import { Textarea } from "@/shadcn-bridge/heroui/input";
import { Select, SelectItem } from "@/shadcn-bridge/heroui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableColumn,
  TableHeader,
  TableRow,
} from "@/shadcn-bridge/heroui/table";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from "@/shadcn-bridge/heroui/modal";
import { Chip } from "@/shadcn-bridge/heroui/chip";
import { Spinner } from "@/shadcn-bridge/heroui/spinner";
import { Switch } from "@/shadcn-bridge/heroui/switch";
import { Alert } from "@/shadcn-bridge/heroui/alert";
import { Checkbox } from "@/shadcn-bridge/heroui/checkbox";
import {
  createForward,
  getForwardList,
  getSpeedLimitList,
  getPeerShareList,
  getPeerRemoteUsageList,
  updateForward,
  deleteForward,
  forceDeleteForward,
  userTunnel,
  getTunnelList,
  getNodeList,
  pauseForwardService,
  resumeForwardService,
  diagnoseForward,
  updateForwardOrder,
  getConfigByName,
  updateConfig,
} from "@/api";
import {
  type ForwardAddressItem,
  formatInAddress,
  formatRemoteAddress,
  hasMultipleAddresses,
  resolveForwardAddressAction,
} from "@/pages/forward/address";
import {
  buildForwardDiagnosisFallbackResult,
  getForwardDiagnosisQualityDisplay,
  type ForwardDiagnosisResult,
} from "@/pages/forward/diagnosis";
import { diagnoseForwardStream } from "@/api/diagnosis-stream";
import {
  executeForwardBatchChangeTunnel,
  executeForwardBatchDelete,
  executeForwardBatchRedeploy,
  executeForwardBatchToggleService,
} from "@/pages/forward/batch-actions";
import {
  convertNyItemToForwardInput,
  parseNyFormatData,
} from "@/pages/forward/import-format";
import { buildForwardOrder, FORWARD_ORDER_KEY } from "@/pages/forward/order";
import { PageLoadingState } from "@/components/page-state";
import { useMobileBreakpoint } from "@/hooks/useMobileBreakpoint";
import { useLocalStorageState } from "@/hooks/use-local-storage-state";
import { saveOrder } from "@/utils/order-storage";
import { JwtUtil } from "@/utils/jwt";
import {
  PlusIcon,
  DeleteIcon,
  CheckAllIcon,
  XCircleIcon,
  UploadIcon,
  DownloadIcon,
  ListCheckIcon,
  XIcon,
  PauseIcon,
  PlayIcon,
  SendIcon,
  GitBranchIcon,
  FunnelIcon,
  ViewListIcon,
  ViewSplitIcon,
} from "@/components/icons";

interface Forward {
  id: number;
  name: string;
  tunnelId: number;
  tunnelName: string;
  inIp: string;
  inPort: number;
  remoteAddr: string;
  interfaceName?: string;
  strategy: string;
  status: number;
  inFlow: number;
  outFlow: number;
  serviceRunning: boolean;
  federationShareFlow?: number;
  createdTime: string;
  userName?: string;
  userId?: number;
  inx?: number;
  speedId?: number | null;
}

interface Tunnel {
  id: number;
  name: string;
  type?: number;
  inIp?: string;
  inNodeId?: Array<{ nodeId: number }>;
  inNodePortSta?: number;
  inNodePortEnd?: number;
  portRangeMin?: number;
  portRangeMax?: number;
}

interface Node {
  id: number;
  name?: string;
  serverIp?: string;
  serverIpV4?: string;
  serverIpV6?: string;
  extraIPs?: string;
}

interface ForwardForm {
  id?: number;
  userId?: number;
  name: string;
  tunnelId: number | null;
  inPort: number | null;
  inIp: string;
  remoteAddr: string;
  interfaceName?: string;
  strategy: string;
  speedId: number | null;
}

interface ForwardUserGroup {
  userId: number;
  userName: string;
  tunnels: ForwardTunnelGroup[];
}

interface ForwardTunnelGroup {
  tunnelKey: string;
  tunnelName: string;
  items: Forward[];
}

type ForwardGroupOrderMap = Record<string, string[]>;
type ForwardGroupCollapsedMap = Record<string, boolean>;

const UNKNOWN_FORWARD_USER_NAME = "未知用户";
const UNCATEGORIZED_FORWARD_TUNNEL_NAME = "未分类";
const FORWARD_COMPACT_MODE_CONFIG_KEY = "forward_compact_mode";
const FORWARD_COMPACT_MODE_EVENT = "forwardCompactModeChanged";
const FORWARD_GROUP_ORDER_CONFIG_KEY = "forward_group_order_map";
const FORWARD_GROUP_COLLAPSED_CONFIG_KEY = "forward_group_collapsed_map";
const FORWARD_GROUP_ORDER_LOCAL_STORAGE_PREFIX = "forward-group-order";
const FORWARD_GROUP_COLLAPSED_LOCAL_STORAGE_PREFIX = "forward-group-collapsed";
const FORWARD_TUNNEL_GROUP_SORTABLE_PREFIX = "forward-tunnel-group";
const FORWARD_GROUPED_TABLE_MIN_WIDTH_CLASS = "min-w-[1320px]";
const FORWARD_GROUPED_TABLE_COLUMN_CLASS = {
  select: "w-14",
  drag: "w-10 pl-4",
  name: "w-[200px]",
  inbound: "w-[280px]",
  target: "w-[280px]",
  strategy: "w-[100px]",
  totalFlow: "w-[120px]",
  status: "w-[100px]",
  actions: "w-[144px] text-right",
} as const;

const normalizeForwardUserName = (userName?: string): string => {
  const normalized = (userName || UNKNOWN_FORWARD_USER_NAME).trim();

  return normalized || UNKNOWN_FORWARD_USER_NAME;
};

const compareForwardUserNameAsc = (a: string, b: string): number => {
  return a.localeCompare(b, "en", {
    sensitivity: "base",
    numeric: true,
  });
};

const normalizeForwardTunnelName = (tunnelName?: string): string => {
  const normalized = (tunnelName || "").trim();

  return normalized || UNCATEGORIZED_FORWARD_TUNNEL_NAME;
};

const buildForwardTunnelGroupKey = (tunnelName?: string): string => {
  const normalized = normalizeForwardTunnelName(tunnelName);

  if (normalized === UNCATEGORIZED_FORWARD_TUNNEL_NAME) {
    return "__uncategorized__";
  }

  return normalized.toLocaleLowerCase();
};

const compareForwardTunnelNameAsc = (a: string, b: string): number => {
  return a.localeCompare(b, "en", {
    sensitivity: "base",
    numeric: true,
  });
};

const compareForwardTunnelGroupKeyAsc = (a: string, b: string): number => {
  const aIsUncategorized = a === "__uncategorized__";
  const bIsUncategorized = b === "__uncategorized__";

  if (aIsUncategorized !== bIsUncategorized) {
    return aIsUncategorized ? 1 : -1;
  }

  return compareForwardTunnelNameAsc(a, b);
};

const buildForwardGroupOrderLocalKey = (tokenUserId: number): string => {
  return `${FORWARD_GROUP_ORDER_LOCAL_STORAGE_PREFIX}:u:${tokenUserId}`;
};

const buildForwardGroupCollapsedLocalKey = (tokenUserId: number): string => {
  return `${FORWARD_GROUP_COLLAPSED_LOCAL_STORAGE_PREFIX}:u:${tokenUserId}`;
};

const parsePreferenceMap = <T,>(
  raw: string | null,
): Record<string, T> | null => {
  if (!raw) {
    return null;
  }

  try {
    const parsed = JSON.parse(raw);

    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return null;
    }

    return parsed as Record<string, T>;
  } catch {
    return null;
  }
};

const parseGroupOrderMap = (raw: string | null): ForwardGroupOrderMap => {
  const parsed = parsePreferenceMap<unknown>(raw);

  if (!parsed) {
    return {};
  }

  const result: ForwardGroupOrderMap = {};

  Object.entries(parsed).forEach(([userId, value]) => {
    if (!Array.isArray(value)) {
      return;
    }

    const keys = value
      .map((item) => (typeof item === "string" ? item.trim() : ""))
      .filter((item) => item !== "");

    if (keys.length > 0) {
      result[userId] = Array.from(new Set(keys));
    }
  });

  return result;
};

const parseGroupCollapsedMap = (
  raw: string | null,
): ForwardGroupCollapsedMap => {
  const parsed = parsePreferenceMap<unknown>(raw);

  if (!parsed) {
    return {};
  }

  const result: ForwardGroupCollapsedMap = {};

  Object.entries(parsed).forEach(([key, value]) => {
    if (typeof value === "boolean") {
      result[key] = value;
    }
  });

  return result;
};

const sanitizeGroupOrderMap = (
  source: ForwardGroupOrderMap,
  availableTunnelKeysByUser: Map<number, Set<string>>,
): ForwardGroupOrderMap => {
  const sanitized: ForwardGroupOrderMap = {};

  availableTunnelKeysByUser.forEach((availableKeys, userId) => {
    if (availableKeys.size === 0) {
      return;
    }

    const orderFromSource = source[userId.toString()] || [];
    const used = new Set<string>();
    const merged: string[] = [];

    orderFromSource.forEach((key) => {
      if (!availableKeys.has(key) || used.has(key)) {
        return;
      }

      used.add(key);
      merged.push(key);
    });

    Array.from(availableKeys)
      .sort(compareForwardTunnelGroupKeyAsc)
      .forEach((key) => {
        if (!used.has(key)) {
          used.add(key);
          merged.push(key);
        }
      });

    if (merged.length > 0) {
      sanitized[userId.toString()] = merged;
    }
  });

  return sanitized;
};

const sanitizeGroupCollapsedMap = (
  source: ForwardGroupCollapsedMap,
  availableCollapseKeys: Set<string>,
): ForwardGroupCollapsedMap => {
  const sanitized: ForwardGroupCollapsedMap = {};

  availableCollapseKeys.forEach((key) => {
    if (source[key] === true) {
      sanitized[key] = true;
    }
  });

  return sanitized;
};

const buildTunnelGroupCollapseKey = (
  userId: number,
  tunnelKey: string,
): string => {
  return `${userId}:${tunnelKey}`;
};

const buildTunnelGroupSortableId = (
  userId: number,
  tunnelKey: string,
): string => {
  return `${FORWARD_TUNNEL_GROUP_SORTABLE_PREFIX}:${userId}:${tunnelKey}`;
};

const parseTunnelGroupSortableId = (
  value: unknown,
): { userId: number; tunnelKey: string } | null => {
  if (typeof value !== "string") {
    return null;
  }

  if (!value.startsWith(`${FORWARD_TUNNEL_GROUP_SORTABLE_PREFIX}:`)) {
    return null;
  }

  const parts = value.split(":");

  if (parts.length < 3) {
    return null;
  }

  const userId = Number(parts[1]);
  const tunnelKey = parts.slice(2).join(":").trim();

  if (!Number.isFinite(userId) || tunnelKey === "") {
    return null;
  }

  return { userId, tunnelKey };
};

const buildAvailableGroupData = (
  forwards: Forward[],
): {
  availableTunnelKeysByUser: Map<number, Set<string>>;
  availableCollapseKeys: Set<string>;
} => {
  const availableTunnelKeysByUser = new Map<number, Set<string>>();
  const availableCollapseKeys = new Set<string>();

  forwards.forEach((forward) => {
    const userId = forward.userId ?? 0;
    const tunnelKey = buildForwardTunnelGroupKey(forward.tunnelName);

    let set = availableTunnelKeysByUser.get(userId);

    if (!set) {
      set = new Set<string>();
      availableTunnelKeysByUser.set(userId, set);
    }

    set.add(tunnelKey);
    availableCollapseKeys.add(buildTunnelGroupCollapseKey(userId, tunnelKey));
  });

  return { availableTunnelKeysByUser, availableCollapseKeys };
};

const isSameStringArray = (a: string[], b: string[]): boolean => {
  if (a.length !== b.length) {
    return false;
  }

  for (let i = 0; i < a.length; i += 1) {
    if (a[i] !== b[i]) {
      return false;
    }
  }

  return true;
};

const isSameGroupOrderMap = (
  a: ForwardGroupOrderMap,
  b: ForwardGroupOrderMap,
): boolean => {
  const aKeys = Object.keys(a).sort(compareForwardTunnelNameAsc);
  const bKeys = Object.keys(b).sort(compareForwardTunnelNameAsc);

  if (!isSameStringArray(aKeys, bKeys)) {
    return false;
  }

  for (const key of aKeys) {
    if (!isSameStringArray(a[key] || [], b[key] || [])) {
      return false;
    }
  }

  return true;
};

const isSameGroupCollapsedMap = (
  a: ForwardGroupCollapsedMap,
  b: ForwardGroupCollapsedMap,
): boolean => {
  const aKeys = Object.keys(a).sort(compareForwardTunnelNameAsc);
  const bKeys = Object.keys(b).sort(compareForwardTunnelNameAsc);

  if (!isSameStringArray(aKeys, bKeys)) {
    return false;
  }

  for (const key of aKeys) {
    if (a[key] !== b[key]) {
      return false;
    }
  }

  return true;
};

export default function ForwardPage() {
  const [loading, setLoading] = useState(true);
  const [forwards, setForwards] = useState<Forward[]>([]);
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [allTunnels, setAllTunnels] = useState<Tunnel[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [speedLimits, setSpeedLimits] = useState<SpeedLimitApiItem[]>([]);
  const isMobile = useMobileBreakpoint();
  const [searchKeyword, setSearchKeyword] = useLocalStorageState(
    "forward-search-keyword",
    "",
  );
  const [isSearchVisible, setIsSearchVisible] = useState(false);
  const [compactMode, setCompactMode] = useState(false);

  // 显示模式状态 - 从localStorage读取，默认为平铺显示
  const [viewMode, setViewMode] = useState<"grouped" | "direct">(() => {
    try {
      const savedMode = localStorage.getItem("forward-view-mode");

      return (savedMode as "grouped" | "direct") || "direct";
    } catch {
      return "direct";
    }
  });

  // 筛选状态
  const [filterUserId, setFilterUserId, resetFilterUserId] =
    useLocalStorageState<string>("forward-filter-user-id", "all");
  const [filterTunnelId, setFilterTunnelId, resetFilterTunnelId] =
    useLocalStorageState<string>("forward-filter-tunnel-id", "all");

  // 拖拽排序相关状态
  const [forwardOrder, setForwardOrder] = useState<number[]>([]);

  // 模态框状态
  const [modalOpen, setModalOpen] = useState(false);
  const [isFilterModalOpen, setIsFilterModalOpen] = useState(false);
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [addressModalOpen, setAddressModalOpen] = useState(false);
  const [diagnosisModalOpen, setDiagnosisModalOpen] = useState(false);
  const [isEdit, setIsEdit] = useState(false);
  const [submitLoading, setSubmitLoading] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [diagnosisLoading, setDiagnosisLoading] = useState(false);
  const [forwardToDelete, setForwardToDelete] = useState<Forward | null>(null);
  const [currentDiagnosisForward, setCurrentDiagnosisForward] =
    useState<Forward | null>(null);
  const [diagnosisResult, setDiagnosisResult] =
    useState<ForwardDiagnosisResult | null>(null);
  const [diagnosisProgress, setDiagnosisProgress] = useState({
    total: 0,
    completed: 0,
    success: 0,
    failed: 0,
    timedOut: false,
  });
  const diagnosisAbortRef = useRef<AbortController | null>(null);
  const [addressModalTitle, setAddressModalTitle] = useState("");
  const [addressList, setAddressList] = useState<ForwardAddressItem[]>([]);

  // 导出相关状态
  const [exportModalOpen, setExportModalOpen] = useState(false);
  const [exportData, setExportData] = useState("");
  const [exportLoading, setExportLoading] = useState(false);
  const [selectedTunnelForExport, setSelectedTunnelForExport] = useState<
    number | null
  >(null);

  // 导入相关状态
  type ImportFormat = "flvx" | "ny";
  const [importModalOpen, setImportModalOpen] = useState(false);
  const [importData, setImportData] = useState("");
  const [importLoading, setImportLoading] = useState(false);
  const [importFormat, setImportFormat] = useState<ImportFormat>("flvx");
  const [selectedTunnelForImport, setSelectedTunnelForImport] = useState<
    number | null
  >(null);
  const [importResults, setImportResults] = useState<
    Array<{
      line: string;
      success: boolean;
      message: string;
      forwardName?: string;
    }>
  >([]);

  // 表单状态
  const [form, setForm] = useState<ForwardForm>({
    name: "",
    tunnelId: null,
    inPort: null,
    inIp: "",
    remoteAddr: "",
    interfaceName: "",
    strategy: "fifo",
    speedId: null,
  });
  const [inIpTouched, setInIpTouched] = useState(false);

  // 表单验证错误
  const [errors, setErrors] = useState<{ [key: string]: string }>({});

  // 批量操作相关状态
  const [selectMode, setSelectMode] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [batchDeleteModalOpen, setBatchDeleteModalOpen] = useState(false);
  const [batchChangeTunnelModalOpen, setBatchChangeTunnelModalOpen] =
    useState(false);
  const [batchTargetTunnelId, setBatchTargetTunnelId] = useState<number | null>(
    null,
  );
  const [batchLoading, setBatchLoading] = useState(false);
  const [groupOrderMap, setGroupOrderMap] = useState<ForwardGroupOrderMap>({});
  const [collapsedTunnelGroups, setCollapsedTunnelGroups] =
    useState<ForwardGroupCollapsedMap>({});
  const [groupPreferenceHydrated, setGroupPreferenceHydrated] = useState(false);
  const tokenUserId = JwtUtil.getUserIdFromToken();
  const tokenRoleId = JwtUtil.getRoleIdFromToken();
  const isAdmin = tokenRoleId === 0;

  const parseNodeIPs = (node?: Node): string[] => {
    if (!node) {
      return [];
    }

    const ips: string[] = [];
    const add = (value?: string) => {
      const trimmed = (value || "").trim();

      if (trimmed) {
        ips.push(trimmed);
      }
    };

    add(node.serverIpV4);
    add(node.serverIpV6);
    add(node.serverIp);

    (node.extraIPs || "")
      .split(",")
      .map((v) => v.trim())
      .filter((v) => v)
      .forEach((v) => ips.push(v));

    return Array.from(new Set(ips));
  };

  const tunnelInIpOptionMap = useMemo(() => {
    const map = new Map<number, string[]>();
    const nodeMap = new Map<number, Node>(nodes.map((n) => [n.id, n]));

    for (const tunnel of allTunnels) {
      const collected: string[] = [];
      const entryNodes = tunnel.inNodeId || [];

      for (const entry of entryNodes) {
        collected.push(...parseNodeIPs(nodeMap.get(entry.nodeId)));
      }

      if (collected.length === 0) {
        (tunnel.inIp || "")
          .split(",")
          .map((v) => v.trim())
          .filter((v) => v)
          .forEach((v) => collected.push(v));
      }

      map.set(tunnel.id, Array.from(new Set(collected)));
    }

    return map;
  }, [allTunnels, nodes]);

  const currentTunnelIpOptions = useMemo(() => {
    if (!form.tunnelId) {
      return [];
    }

    return tunnelInIpOptionMap.get(form.tunnelId) || [];
  }, [form.tunnelId, tunnelInIpOptionMap]);

  const isCurrentTunnelMultiEntrance = useMemo(() => {
    if (!form.tunnelId) {
      return false;
    }

    const currentTunnel = allTunnels.find(
      (tunnel) => tunnel.id === form.tunnelId,
    );

    return (currentTunnel?.inNodeId?.length || 0) > 1;
  }, [allTunnels, form.tunnelId]);

  const currentTunnelPortRange = useMemo(() => {
    if (!form.tunnelId) {
      return null;
    }

    const currentTunnel = allTunnels.find(
      (tunnel) => tunnel.id === form.tunnelId,
    );

    if (
      currentTunnel?.portRangeMin &&
      currentTunnel?.portRangeMax &&
      currentTunnel.portRangeMin > 0 &&
      currentTunnel.portRangeMax > 0
    ) {
      return {
        min: currentTunnel.portRangeMin,
        max: currentTunnel.portRangeMax,
      };
    }

    return null;
  }, [allTunnels, form.tunnelId]);

  useEffect(() => {
    return () => {
      diagnosisAbortRef.current?.abort();
      diagnosisAbortRef.current = null;
    };
  }, []);

  const persistGroupOrderToLocal = (nextOrderMap: ForwardGroupOrderMap) => {
    if (tokenUserId === null) {
      return;
    }

    try {
      localStorage.setItem(
        buildForwardGroupOrderLocalKey(tokenUserId),
        JSON.stringify(nextOrderMap),
      );
    } catch {}
  };

  const persistGroupCollapsedToLocal = (
    nextCollapsedMap: ForwardGroupCollapsedMap,
  ) => {
    if (tokenUserId === null) {
      return;
    }

    try {
      localStorage.setItem(
        buildForwardGroupCollapsedLocalKey(tokenUserId),
        JSON.stringify(nextCollapsedMap),
      );
    } catch {}
  };

  const persistGroupOrderToGlobal = async (
    nextOrderMap: ForwardGroupOrderMap,
  ): Promise<void> => {
    if (!isAdmin || tokenUserId === null) {
      return;
    }

    try {
      const currentRes = await getConfigByName(FORWARD_GROUP_ORDER_CONFIG_KEY);
      const globalMap =
        parsePreferenceMap<ForwardGroupOrderMap>(
          currentRes.code === 0 && typeof currentRes.data?.value === "string"
            ? currentRes.data.value
            : null,
        ) || {};

      globalMap[tokenUserId.toString()] = nextOrderMap;

      const saveRes = await updateConfig(
        FORWARD_GROUP_ORDER_CONFIG_KEY,
        JSON.stringify(globalMap),
      );

      if (saveRes.code !== 0) {
        toast.error(saveRes.msg || "保存分组排序失败");
      }
    } catch {
      toast.error("保存分组排序失败");
    }
  };

  const persistGroupCollapsedToGlobal = async (
    nextCollapsedMap: ForwardGroupCollapsedMap,
  ): Promise<void> => {
    if (!isAdmin || tokenUserId === null) {
      return;
    }

    try {
      const currentRes = await getConfigByName(
        FORWARD_GROUP_COLLAPSED_CONFIG_KEY,
      );
      const globalMap =
        parsePreferenceMap<ForwardGroupCollapsedMap>(
          currentRes.code === 0 && typeof currentRes.data?.value === "string"
            ? currentRes.data.value
            : null,
        ) || {};

      globalMap[tokenUserId.toString()] = nextCollapsedMap;

      const saveRes = await updateConfig(
        FORWARD_GROUP_COLLAPSED_CONFIG_KEY,
        JSON.stringify(globalMap),
      );

      if (saveRes.code !== 0) {
        toast.error(saveRes.msg || "保存分组折叠状态失败");
      }
    } catch {
      toast.error("保存分组折叠状态失败");
    }
  };

  useEffect(() => {
    let cancelled = false;

    const loadGroupPreferences = async () => {
      if (tokenUserId === null) {
        if (!cancelled) {
          setGroupOrderMap({});
          setCollapsedTunnelGroups({});
          setGroupPreferenceHydrated(true);
        }

        return;
      }

      let localOrderMap: ForwardGroupOrderMap = {};
      let localCollapsedMap: ForwardGroupCollapsedMap = {};

      try {
        localOrderMap = parseGroupOrderMap(
          localStorage.getItem(buildForwardGroupOrderLocalKey(tokenUserId)),
        );
      } catch {
        localOrderMap = {};
      }

      try {
        localCollapsedMap = parseGroupCollapsedMap(
          localStorage.getItem(buildForwardGroupCollapsedLocalKey(tokenUserId)),
        );
      } catch {
        localCollapsedMap = {};
      }

      if (isAdmin) {
        try {
          const [globalOrderRes, globalCollapsedRes] = await Promise.all([
            getConfigByName(FORWARD_GROUP_ORDER_CONFIG_KEY),
            getConfigByName(FORWARD_GROUP_COLLAPSED_CONFIG_KEY),
          ]);

          const globalOrderMap = parsePreferenceMap<ForwardGroupOrderMap>(
            globalOrderRes.code === 0 &&
              typeof globalOrderRes.data?.value === "string"
              ? globalOrderRes.data.value
              : null,
          );
          const globalCollapsedMap =
            parsePreferenceMap<ForwardGroupCollapsedMap>(
              globalCollapsedRes.code === 0 &&
                typeof globalCollapsedRes.data?.value === "string"
                ? globalCollapsedRes.data.value
                : null,
            );

          const globalOrderBucket = globalOrderMap?.[tokenUserId.toString()];
          const globalCollapsedBucket =
            globalCollapsedMap?.[tokenUserId.toString()];

          if (
            globalOrderBucket &&
            typeof globalOrderBucket === "object" &&
            !Array.isArray(globalOrderBucket)
          ) {
            localOrderMap = parseGroupOrderMap(
              JSON.stringify(globalOrderBucket),
            );
          }

          if (
            globalCollapsedBucket &&
            typeof globalCollapsedBucket === "object" &&
            !Array.isArray(globalCollapsedBucket)
          ) {
            localCollapsedMap = parseGroupCollapsedMap(
              JSON.stringify(globalCollapsedBucket),
            );
          }
        } catch {}
      }

      if (cancelled) {
        return;
      }

      setGroupOrderMap(localOrderMap);
      setCollapsedTunnelGroups(localCollapsedMap);
      persistGroupOrderToLocal(localOrderMap);
      persistGroupCollapsedToLocal(localCollapsedMap);
      setGroupPreferenceHydrated(true);
    };

    setGroupPreferenceHydrated(false);
    loadGroupPreferences();

    return () => {
      cancelled = true;
    };
  }, [tokenUserId, isAdmin]);

  useEffect(() => {
    const loadForwardCompactMode = async () => {
      try {
        const response = await getConfigByName(FORWARD_COMPACT_MODE_CONFIG_KEY);
        const enabled =
          response.code === 0 &&
          typeof response.data?.value === "string" &&
          response.data.value === "true";

        setCompactMode(enabled);
      } catch {
        setCompactMode(false);
      }
    };

    const handleCompactModeChanged = (event: Event) => {
      const customEvent = event as CustomEvent<{ enabled?: boolean }>;

      if (typeof customEvent.detail?.enabled === "boolean") {
        setCompactMode(customEvent.detail.enabled);
      }
    };

    loadForwardCompactMode();
    window.addEventListener(
      FORWARD_COMPACT_MODE_EVENT,
      handleCompactModeChanged,
    );

    return () => {
      window.removeEventListener(
        FORWARD_COMPACT_MODE_EVENT,
        handleCompactModeChanged,
      );
    };
  }, []);

  const parseShareIdFromTunnelName = (tunnelName: string): number | null => {
    const normalized = (tunnelName || "").trim();

    if (!normalized.startsWith("Share-")) {
      return null;
    }

    const raw = normalized.slice("Share-".length);
    const idx = raw.indexOf("-Port-");

    if (idx <= 0) {
      return null;
    }

    const shareId = Number(raw.slice(0, idx).trim());

    return Number.isFinite(shareId) && shareId > 0 ? shareId : null;
  };

  const mergeFederationShareFlow = async (
    forwardsData: Forward[],
  ): Promise<Forward[]> => {
    if (forwardsData.length === 0) {
      return forwardsData;
    }

    try {
      const [usageRes, localShareRes] = await Promise.all([
        getPeerRemoteUsageList(),
        getPeerShareList(),
      ]);

      const flowByShare = new Map<number, number>();
      const shareIdsByTunnel = new Map<number, Set<number>>();

      if (usageRes.code === 0 && Array.isArray(usageRes.data)) {
        usageRes.data.forEach((item: Record<string, unknown>) => {
          const shareId = Number(item.shareId || 0);
          const currentFlow = Number(item.currentFlow || 0);

          if (
            Number.isFinite(shareId) &&
            shareId > 0 &&
            Number.isFinite(currentFlow) &&
            currentFlow > 0
          ) {
            const prev = flowByShare.get(shareId) || 0;

            flowByShare.set(shareId, Math.max(prev, currentFlow));
          }

          if (Number.isFinite(shareId) && shareId > 0) {
            const bindings = Array.isArray(item.bindings)
              ? (item.bindings as Array<Record<string, unknown>>)
              : [];

            bindings.forEach((binding) => {
              const tunnelId = Number(binding.tunnelId || 0);
              const chainType = Number(binding.chainType || 0);

              if (!Number.isFinite(tunnelId) || tunnelId <= 0) {
                return;
              }

              if (Number.isFinite(chainType) && chainType !== 1) {
                return;
              }

              let shareSet = shareIdsByTunnel.get(tunnelId);

              if (!shareSet) {
                shareSet = new Set<number>();
                shareIdsByTunnel.set(tunnelId, shareSet);
              }

              shareSet.add(shareId);
            });
          }
        });
      }

      if (localShareRes.code === 0 && Array.isArray(localShareRes.data)) {
        localShareRes.data.forEach((item: Record<string, unknown>) => {
          const shareId = Number(item.id || 0);
          const currentFlow = Number(item.currentFlow || 0);

          if (
            Number.isFinite(shareId) &&
            shareId > 0 &&
            Number.isFinite(currentFlow) &&
            currentFlow > 0
          ) {
            const prev = flowByShare.get(shareId) || 0;

            flowByShare.set(shareId, Math.max(prev, currentFlow));
          }
        });
      }

      if (flowByShare.size === 0) {
        return forwardsData;
      }

      const resolveShareIdForForward = (forward: Forward): number | null => {
        const candidates = new Set<number>();
        const shareIdFromName = parseShareIdFromTunnelName(
          forward.tunnelName || "",
        );

        if (shareIdFromName) {
          candidates.add(shareIdFromName);
        }

        const tunnelId = Number(forward.tunnelId || 0);
        const shareSetByTunnel = shareIdsByTunnel.get(tunnelId);

        if (shareSetByTunnel && shareSetByTunnel.size > 0) {
          shareSetByTunnel.forEach((shareId) => {
            if (Number.isFinite(shareId) && shareId > 0) {
              candidates.add(shareId);
            }
          });
        }

        if (candidates.size === 0) {
          return null;
        }

        let bestShareId: number | null = null;
        let bestFlow = 0;

        candidates.forEach((shareId) => {
          const shareFlow = flowByShare.get(shareId) || 0;

          if (shareFlow > bestFlow) {
            bestFlow = shareFlow;
            bestShareId = shareId;
          }
        });

        return bestShareId;
      };

      const resolvedShareByForwardId = new Map<number, number>();

      forwardsData.forEach((forward) => {
        const shareId = resolveShareIdForForward(forward);

        if (shareId) {
          resolvedShareByForwardId.set(forward.id, shareId);
        }
      });

      const forwardCountByShare = new Map<number, number>();

      forwardsData.forEach((forward) => {
        const shareId = resolvedShareByForwardId.get(forward.id) || null;

        if (!shareId || !flowByShare.has(shareId)) {
          return;
        }

        forwardCountByShare.set(
          shareId,
          (forwardCountByShare.get(shareId) || 0) + 1,
        );
      });

      return forwardsData.map((forward) => {
        const shareId = resolvedShareByForwardId.get(forward.id) || null;

        if (!shareId) {
          return { ...forward, federationShareFlow: undefined };
        }

        const shareFlow = flowByShare.get(shareId) || 0;

        if (shareFlow <= 0) {
          return { ...forward, federationShareFlow: undefined };
        }

        const directFlow = (forward.inFlow || 0) + (forward.outFlow || 0);

        if (directFlow > 0) {
          return { ...forward, federationShareFlow: undefined };
        }

        const count = forwardCountByShare.get(shareId) || 1;
        const estimated = Math.max(1, Math.floor(shareFlow / count));

        return { ...forward, federationShareFlow: estimated };
      });
    } catch {
      return forwardsData;
    }
  };

  const getForwardDisplayFlow = (forward: Forward): number => {
    const directFlow = (forward.inFlow || 0) + (forward.outFlow || 0);

    if (directFlow > 0) {
      return directFlow;
    }

    return forward.federationShareFlow || 0;
  };

  useEffect(() => {
    loadData();
  }, []);

  // 切换显示模式并保存到localStorage
  const handleViewModeChange = () => {
    const newMode = viewMode === "grouped" ? "direct" : "grouped";

    setViewMode(newMode);
    try {
      localStorage.setItem("forward-view-mode", newMode);
    } catch {}
  };

  // 加载所有数据
  const loadData = async (lod = true) => {
    setLoading(lod);
    try {
      const [forwardsRes, tunnelsRes, speedLimitsRes] = await Promise.all([
        getForwardList(),
        userTunnel(),
        getSpeedLimitList(),
      ]);
      const [allTunnelsRes, nodesRes] = await Promise.allSettled([
        getTunnelList(),
        getNodeList(),
      ]);

      if (forwardsRes.code === 0) {
        const forwardsData =
          forwardsRes.data?.map((forward: any) => ({
            ...forward,
            serviceRunning: forward.status === 1,
          })) || [];

        const mergedForwards = await mergeFederationShareFlow(forwardsData);

        setForwards(mergedForwards);

        // 初始化拖拽排序顺序
        const currentUserId = JwtUtil.getUserIdFromToken();
        const { order, fromDatabase } = buildForwardOrder(
          mergedForwards,
          currentUserId,
        );

        setForwardOrder(order);

        if (fromDatabase) {
          saveOrder(FORWARD_ORDER_KEY, order);
        }
      } else {
        toast.error(forwardsRes.msg || "获取规则列表失败");
      }

      if (tunnelsRes.code === 0) {
        setTunnels(tunnelsRes.data || []);
      } else {
      }

      if (
        allTunnelsRes.status === "fulfilled" &&
        allTunnelsRes.value.code === 0
      ) {
        setAllTunnels((allTunnelsRes.value.data || []) as Tunnel[]);
      }

      if (nodesRes.status === "fulfilled" && nodesRes.value.code === 0) {
        setNodes((nodesRes.value.data || []) as Node[]);
      }

      if (speedLimitsRes.code === 0) {
        setSpeedLimits(speedLimitsRes.data || []);
      }
    } catch {
      toast.error("加载数据失败");
    } finally {
      setLoading(false);
    }
  };

  // 表单验证
  const noLimitSpeedLimitIds = useMemo(() => {
    return new Set(
      speedLimits
        .filter((speedLimit) => speedLimit.name.trim() === "不限速")
        .map((speedLimit) => speedLimit.id),
    );
  }, [speedLimits]);

  const speedLimitIds = useMemo(() => {
    return new Set(speedLimits.map((speedLimit) => speedLimit.id));
  }, [speedLimits]);

  const availableSpeedLimits = useMemo(() => {
    return speedLimits.filter(
      (speedLimit) => !noLimitSpeedLimitIds.has(speedLimit.id),
    );
  }, [speedLimits, noLimitSpeedLimitIds]);

  const normalizeSpeedId = (speedId?: number | null): number | null => {
    if (speedId === null || speedId === undefined) {
      return null;
    }

    if (noLimitSpeedLimitIds.has(speedId)) {
      return null;
    }

    if (speedLimits.length > 0 && !speedLimitIds.has(speedId)) {
      return null;
    }

    return speedId;
  };

  const isMissingSpeedLimit = (speedId?: number | null): boolean => {
    if (speedId === null || speedId === undefined) {
      return false;
    }

    if (speedLimits.length === 0 || noLimitSpeedLimitIds.has(speedId)) {
      return false;
    }

    return !speedLimitIds.has(speedId);
  };

  const selectedSpeedId = normalizeSpeedId(form.speedId);

  const validateForm = (): boolean => {
    const newErrors: { [key: string]: string } = {};

    if (!form.name.trim()) {
      newErrors.name = "请输入规则名称";
    } else if (form.name.length < 2 || form.name.length > 50) {
      newErrors.name = "规则名称长度应在2-50个字符之间";
    }

    if (!form.tunnelId) {
      newErrors.tunnelId = "请选择关联隧道";
    }

    if (
      form.inPort !== null &&
      form.inPort !== undefined &&
      form.inPort > 0 &&
      currentTunnelPortRange
    ) {
      if (
        form.inPort < currentTunnelPortRange.min ||
        form.inPort > currentTunnelPortRange.max
      ) {
        newErrors.inPort = `端口 ${currentTunnelPortRange.min}-${currentTunnelPortRange.max} 超出允许范围`;
      }
    }

    if (!form.remoteAddr.trim()) {
      newErrors.remoteAddr = "请输入远程地址";
    } else {
      // 验证地址格式
      const addresses = form.remoteAddr
        .split("\n")
        .map((addr) => addr.trim())
        .filter((addr) => addr);
      const ipv4Pattern =
        /^(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?):\d+$/;
      const ipv6FullPattern =
        /^\[((([0-9a-fA-F]{1,4}:){7}([0-9a-fA-F]{1,4}|:))|(([0-9a-fA-F]{1,4}:){6}(:[0-9a-fA-F]{1,4}|((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9a-fA-F]{1,4}:){5}(((:[0-9a-fA-F]{1,4}){1,2})|:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9a-fA-F]{1,4}:){4}(((:[0-9a-fA-F]{1,4}){1,3})|((:[0-9a-fA-F]{1,4})?:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9a-fA-F]{1,4}:){3}(((:[0-9a-fA-F]{1,4}){1,4})|((:[0-9a-fA-F]{1,4}){0,2}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9a-fA-F]{1,4}:){2}(((:[0-9a-fA-F]{1,4}){1,5})|((:[0-9a-fA-F]{1,4}){0,3}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9a-fA-F]{1,4}:){1}(((:[0-9a-fA-F]{1,4}){1,6})|((:[0-9a-fA-F]{1,4}){0,4}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(:(((:[0-9a-fA-F]{1,4}){1,7})|((:[0-9a-fA-F]{1,4}){0,5}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:)))\]:\d+$/;
      const domainPattern =
        /^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*:\d+$/;

      for (let i = 0; i < addresses.length; i++) {
        const addr = addresses[i];

        if (
          !ipv4Pattern.test(addr) &&
          !ipv6FullPattern.test(addr) &&
          !domainPattern.test(addr)
        ) {
          newErrors.remoteAddr = `第${i + 1}行地址格式错误`;
          break;
        }
      }
    }

    setErrors(newErrors);

    return Object.keys(newErrors).length === 0;
  };

  // 新增规则
  const handleAdd = () => {
    setIsEdit(false);
    setInIpTouched(false);
    setForm({
      name: "",
      tunnelId: null,
      inPort: null,
      inIp: "",
      remoteAddr: "",
      interfaceName: "",
      strategy: "fifo",
      speedId: null,
    });
    setErrors({});
    setModalOpen(true);
  };

  // 编辑规则
  const handleEdit = (forward: Forward) => {
    setIsEdit(true);
    setInIpTouched(false);
    setForm({
      id: forward.id,
      userId: forward.userId,
      name: forward.name,
      tunnelId: forward.tunnelId,
      inPort: forward.inPort,
      inIp: forward.inIp || "",
      remoteAddr: forward.remoteAddr.split(",").join("\n"),
      interfaceName: forward.interfaceName || "",
      strategy: forward.strategy || "fifo",
      speedId: normalizeSpeedId(forward.speedId),
    });
    setErrors({});
    setModalOpen(true);
  };

  // 显示删除确认
  const handleDelete = (forward: Forward) => {
    setForwardToDelete(forward);
    setDeleteModalOpen(true);
  };

  // 确认删除规则
  const confirmDelete = async () => {
    if (!forwardToDelete) return;

    setDeleteLoading(true);
    try {
      const res = await deleteForward(forwardToDelete.id);

      if (res.code === 0) {
        toast.success("删除成功");
        setDeleteModalOpen(false);
        loadData();
      } else {
        // 删除失败，询问是否强制删除
        const confirmed = window.confirm(
          `常规删除失败：${res.msg || "删除失败"}\n\n是否需要强制删除？\n\n⚠️ 注意：强制删除不会去验证节点端是否已经删除对应的规则服务。`,
        );

        if (confirmed) {
          const forceRes = await forceDeleteForward(forwardToDelete.id);

          if (forceRes.code === 0) {
            toast.success("强制删除成功");
            setDeleteModalOpen(false);
            loadData();
          } else {
            toast.error(forceRes.msg || "强制删除失败");
          }
        }
      }
    } catch {
      toast.error("删除失败");
    } finally {
      setDeleteLoading(false);
    }
  };

  // 处理隧道选择变化
  const handleTunnelChange = (tunnelId: string) => {
    const nextTunnelId = parseInt(tunnelId);
    const options = tunnelInIpOptionMap.get(nextTunnelId) || [];

    setInIpTouched(false);

    setForm((prev) => {
      const tunnelChanged = prev.tunnelId !== nextTunnelId;

      return {
        ...prev,
        tunnelId: nextTunnelId,
        inIp: tunnelChanged ? "" : options.includes(prev.inIp) ? prev.inIp : "",
      };
    });
  };

  // 提交表单
  const handleSubmit = async () => {
    if (!validateForm()) return;

    setSubmitLoading(true);
    try {
      const processedRemoteAddr = form.remoteAddr
        .split("\n")
        .map((addr) => addr.trim())
        .filter((addr) => addr)
        .join(",");

      const addressCount = processedRemoteAddr.split(",").length;

      let res: { code: number; msg: string };
      const normalizedSpeedId = normalizeSpeedId(form.speedId);
      const speedLimitAutoCleared = isMissingSpeedLimit(form.speedId);

      if (isEdit) {
        const updateData = {
          id: form.id,
          name: form.name,
          tunnelId: form.tunnelId,
          inPort: form.inPort,
          ...(inIpTouched ? { inIp: form.inIp || "" } : {}),
          remoteAddr: processedRemoteAddr,
          strategy: addressCount > 1 ? form.strategy : "fifo",
          speedId: normalizedSpeedId,
        };

        res = await updateForward(updateData);
      } else {
        const createData = {
          name: form.name,
          tunnelId: form.tunnelId,
          inPort: form.inPort,
          inIp: form.inIp || undefined,
          remoteAddr: processedRemoteAddr,
          strategy: addressCount > 1 ? form.strategy : "fifo",
          speedId: normalizedSpeedId,
        };

        res = await createForward(createData);
      }

      if (res.code === 0) {
        const warningItems = Array.isArray((res as any).data?.warnings)
          ? (res as any).data.warnings
              .map((item: unknown) =>
                typeof item === "string" ? item.trim() : "",
              )
              .filter((item: string) => item)
          : [];

        warningItems.forEach((warning: string) => {
          toast(warning, {
            icon: "⚠️",
            duration: 5000,
          });
        });
        if (speedLimitAutoCleared) {
          toast("所选限速规则不存在，已自动清除为不限速", {
            icon: "⚠️",
            duration: 5000,
          });
        }
        toast.success(isEdit ? "修改成功" : "创建成功");
        setModalOpen(false);
        loadData();
      } else {
        toast.error(res.msg || "操作失败");
      }
    } catch {
      toast.error("操作失败");
    } finally {
      setSubmitLoading(false);
    }
  };

  // 处理服务开关
  const handleServiceToggle = async (forward: Forward) => {
    if (forward.status !== 1 && forward.status !== 0) {
      toast.error("规则状态异常，无法操作");

      return;
    }

    const targetState = !forward.serviceRunning;

    try {
      // 乐观更新UI
      setForwards((prev) =>
        prev.map((f) =>
          f.id === forward.id ? { ...f, serviceRunning: targetState } : f,
        ),
      );

      let res: { code: number; msg: string };

      if (targetState) {
        res = await resumeForwardService(forward.id);
      } else {
        res = await pauseForwardService(forward.id);
      }

      if (res.code === 0) {
        toast.success(targetState ? "服务已启动" : "服务已暂停");
        // 更新规则状态
        setForwards((prev) =>
          prev.map((f) =>
            f.id === forward.id ? { ...f, status: targetState ? 1 : 0 } : f,
          ),
        );
      } else {
        // 操作失败，恢复UI状态
        setForwards((prev) =>
          prev.map((f) =>
            f.id === forward.id ? { ...f, serviceRunning: !targetState } : f,
          ),
        );
        toast.error(res.msg || "操作失败");
      }
    } catch {
      // 操作失败，恢复UI状态
      setForwards((prev) =>
        prev.map((f) =>
          f.id === forward.id ? { ...f, serviceRunning: !targetState } : f,
        ),
      );
      toast.error("网络错误，操作失败");
    }
  };

  // 诊断规则
  const handleDiagnose = async (forward: Forward) => {
    diagnosisAbortRef.current?.abort();
    const abortController = new AbortController();

    diagnosisAbortRef.current = abortController;

    setCurrentDiagnosisForward(forward);
    setDiagnosisModalOpen(true);
    setDiagnosisLoading(true);
    setDiagnosisProgress({
      total: 0,
      completed: 0,
      success: 0,
      failed: 0,
      timedOut: false,
    });
    setDiagnosisResult({
      forwardName: forward.name,
      timestamp: Date.now(),
      results: [],
    });

    try {
      let streamErrorMessage = "";
      const streamResult = await diagnoseForwardStream(
        forward.id,
        {
          onStart: (payload) => {
            const startForwardName =
              typeof payload.forwardName === "string" &&
              payload.forwardName.trim() !== ""
                ? payload.forwardName
                : forward.name;
            const startTotal = Number(payload.total);
            const startItems = Array.isArray(payload.items)
              ? (payload.items as ForwardDiagnosisResult["results"])
              : [];

            setDiagnosisResult((prev) => ({
              forwardName: startForwardName,
              timestamp: Date.now(),
              results: startItems.length > 0 ? startItems : prev?.results || [],
            }));
            if (Number.isFinite(startTotal) && startTotal >= 0) {
              setDiagnosisProgress((prev) => ({
                ...prev,
                total: startTotal,
              }));
            }
          },
          onItem: ({ result, progress }) => {
            setDiagnosisResult((prev) => {
              const base: ForwardDiagnosisResult = prev || {
                forwardName: forward.name,
                timestamp: Date.now(),
                results: [],
              };
              const nextResults = [...base.results];
              const existingIndex = nextResults.findIndex(
                (item) =>
                  item.description === result.description &&
                  item.nodeId === result.nodeId &&
                  item.targetIp === result.targetIp &&
                  item.targetPort === result.targetPort,
              );

              if (existingIndex >= 0) {
                nextResults[existingIndex] = {
                  ...result,
                  diagnosing: false,
                };
              } else {
                nextResults.push({
                  ...result,
                  diagnosing: false,
                });
              }

              return {
                ...base,
                timestamp: Date.now(),
                results: nextResults,
              };
            });
            setDiagnosisProgress({
              total: progress.total,
              completed: progress.completed,
              success: progress.success,
              failed: progress.failed,
              timedOut: Boolean(progress.timedOut),
            });
          },
          onDone: (progress) => {
            setDiagnosisProgress({
              total: progress.total,
              completed: progress.completed,
              success: progress.success,
              failed: progress.failed,
              timedOut: Boolean(progress.timedOut),
            });
          },
          onError: (message) => {
            streamErrorMessage = message;
          },
        },
        abortController.signal,
      );

      if (streamResult.fallback) {
        const response = await diagnoseForward(forward.id);

        if (response.code === 0) {
          const resultData = response.data as ForwardDiagnosisResult;
          const successCount = resultData.results.filter(
            (r) => r.success,
          ).length;
          const failedCount = resultData.results.length - successCount;

          setDiagnosisResult(resultData);
          setDiagnosisProgress({
            total: resultData.results.length,
            completed: resultData.results.length,
            success: successCount,
            failed: failedCount,
            timedOut: false,
          });
        } else {
          toast.error(response.msg || "诊断失败");
          setDiagnosisResult(
            buildForwardDiagnosisFallbackResult({
              forwardName: forward.name,
              remoteAddr: forward.remoteAddr,
              description: "诊断失败",
              message: response.msg || "诊断过程中发生错误",
            }),
          );
          setDiagnosisProgress({
            total: 1,
            completed: 1,
            success: 0,
            failed: 1,
            timedOut: false,
          });
        }

        return;
      }

      if (streamErrorMessage) {
        toast.error(streamErrorMessage);
      }
      if (streamResult.timedOut) {
        toast.error("诊断超时（单条30秒 / 整体2分钟），已返回当前结果");
      }
    } catch {
      if (abortController.signal.aborted) {
        return;
      }
      toast.error("网络错误，请重试");
      setDiagnosisResult(
        buildForwardDiagnosisFallbackResult({
          forwardName: forward.name,
          remoteAddr: forward.remoteAddr,
          description: "网络错误",
          message: "无法连接到服务器",
        }),
      );
      setDiagnosisProgress({
        total: 1,
        completed: 1,
        success: 0,
        failed: 1,
        timedOut: false,
      });
    } finally {
      if (diagnosisAbortRef.current === abortController) {
        diagnosisAbortRef.current = null;
      }
      setDiagnosisLoading(false);
    }
  };

  // 格式化流量
  const formatFlow = (value: number): string => {
    if (value === 0) return "0 B";
    if (value < 1024) return value + " B";
    if (value < 1024 * 1024) return (value / 1024).toFixed(2) + " KB";
    if (value < 1024 * 1024 * 1024)
      return (value / (1024 * 1024)).toFixed(2) + " MB";

    return (value / (1024 * 1024 * 1024)).toFixed(2) + " GB";
  };

  // 显示地址列表弹窗
  const showAddressModal = (
    addressString: string,
    port: number | null,
    title: string,
  ) => {
    const action = resolveForwardAddressAction(addressString, port, title);

    if (action.type === "none") {
      return;
    }

    if (action.type === "copy") {
      copyToClipboard(action.text, action.label);

      return;
    }

    setAddressList(action.items);
    setAddressModalTitle(action.title);
    setAddressModalOpen(true);
  };

  // 复制到剪贴板
  const copyToClipboard = async (text: string, label: string = "内容") => {
    try {
      await navigator.clipboard.writeText(text);
      toast.success(`已复制${label}`);
    } catch {
      toast.error("复制失败");
    }
  };

  // 复制地址
  const copyAddress = async (addressItem: ForwardAddressItem) => {
    try {
      setAddressList((prev) =>
        prev.map((item) =>
          item.id === addressItem.id ? { ...item, copying: true } : item,
        ),
      );
      await copyToClipboard(addressItem.address, "地址");
    } catch {
      toast.error("复制失败");
    } finally {
      setAddressList((prev) =>
        prev.map((item) =>
          item.id === addressItem.id ? { ...item, copying: false } : item,
        ),
      );
    }
  };

  // 复制所有地址
  const copyAllAddresses = async () => {
    if (addressList.length === 0) return;
    const allAddresses = addressList.map((item) => item.address).join("\n");

    await copyToClipboard(allAddresses, "所有地址");
  };

  // 导出规则数据
  const handleExport = () => {
    setSelectedTunnelForExport(null);
    setExportData("");
    setExportModalOpen(true);
  };

  // 执行导出
  const executeExport = () => {
    if (!selectedTunnelForExport) {
      toast.error("请选择要导出的隧道");

      return;
    }

    setExportLoading(true);

    try {
      // 获取要导出的规则列表
      const forwardsToExport = sortedForwards.filter(
        (forward) => forward.tunnelId === selectedTunnelForExport,
      );

      if (forwardsToExport.length === 0) {
        toast.error("所选隧道没有规则数据");
        setExportLoading(false);

        return;
      }

      // 格式化导出数据：remoteAddr|name|inPort
      const exportLines = forwardsToExport.map((forward) => {
        return `${forward.remoteAddr}|${forward.name}|${forward.inPort}`;
      });

      const exportText = exportLines.join("\n");

      setExportData(exportText);
    } catch {
      toast.error("导出失败");
    } finally {
      setExportLoading(false);
    }
  };

  // 复制导出数据
  const copyExportData = async () => {
    await copyToClipboard(exportData, "规则数据");
  };

  // 导入规则数据
  const handleImport = () => {
    setImportData("");
    setImportResults([]);
    setSelectedTunnelForImport(null);
    setImportModalOpen(true);
  };

  // 执行导入
  const executeImport = async () => {
    if (!importData.trim()) {
      toast.error("请输入要导入的数据");

      return;
    }

    if (!selectedTunnelForImport) {
      toast.error("请选择要导入的隧道");

      return;
    }

    setImportLoading(true);
    setImportResults([]);

    try {
      if (importFormat === "ny") {
        const parsedItems = parseNyFormatData(importData);

        if (parsedItems.length === 0) {
          toast.error("未解析到有效的ny格式数据");

          setImportLoading(false);

          return;
        }

        for (const item of parsedItems) {
          if (item.error) {
            setImportResults((prev) => [
              {
                line: item.line,
                success: false,
                message: item.error || "解析失败",
              },
              ...prev,
            ]);

            continue;
          }

          if (!item.parsed) {
            setImportResults((prev) => [
              {
                line: item.line,
                success: false,
                message: "解析失败",
              },
              ...prev,
            ]);

            continue;
          }

          const parsedNyItem = item.parsed;
          const nyForwardInput = convertNyItemToForwardInput(parsedNyItem);

          try {
            const response = await createForward({
              name: nyForwardInput.name,
              tunnelId: selectedTunnelForImport,
              inPort: nyForwardInput.inPort,
              remoteAddr: nyForwardInput.remoteAddr,
              strategy: nyForwardInput.strategy,
            });

            if (response.code === 0) {
              setImportResults((prev) => [
                {
                  line: item.line,
                  success: true,
                  message: `创建成功 (${parsedNyItem.dest.length}个目标)`,
                  forwardName: nyForwardInput.name,
                },
                ...prev,
              ]);
            } else {
              setImportResults((prev) => [
                {
                  line: item.line,
                  success: false,
                  message: response.msg || "创建失败",
                },
                ...prev,
              ]);
            }
          } catch {
            setImportResults((prev) => [
              {
                line: item.line,
                success: false,
                message: "网络错误，创建失败",
              },
              ...prev,
            ]);
          }
        }
      } else {
        const lines = importData
          .trim()
          .split("\n")
          .filter((line) => line.trim());

        for (let i = 0; i < lines.length; i++) {
          const line = lines[i].trim();
          const parts = line.split("|");

          if (parts.length < 2) {
            setImportResults((prev) => [
              {
                line,
                success: false,
                message: "格式错误：需要至少包含目标地址和规则名称",
              },
              ...prev,
            ]);
            continue;
          }

          const [remoteAddr, name, inPort] = parts;

          if (!remoteAddr.trim() || !name.trim()) {
            setImportResults((prev) => [
              {
                line,
                success: false,
                message: "目标地址和规则名称不能为空",
              },
              ...prev,
            ]);
            continue;
          }

          const addresses = remoteAddr.trim().split(",");
          const addressPattern = /^[^:]+:\d+$/;
          const isValidFormat = addresses.every((addr) =>
            addressPattern.test(addr.trim()),
          );

          if (!isValidFormat) {
            setImportResults((prev) => [
              {
                line,
                success: false,
                message:
                  "目标地址格式错误，应为 地址:端口 格式，多个地址用逗号分隔",
              },
              ...prev,
            ]);
            continue;
          }

          try {
            let portNumber: number | null = null;

            if (inPort && inPort.trim()) {
              const port = parseInt(inPort.trim());

              if (isNaN(port) || port < 1 || port > 65535) {
                setImportResults((prev) => [
                  {
                    line,
                    success: false,
                    message: "入口端口格式错误，应为1-65535之间的数字",
                  },
                  ...prev,
                ]);
                continue;
              }
              portNumber = port;
            }

            const response = await createForward({
              name: name.trim(),
              tunnelId: selectedTunnelForImport,
              inPort: portNumber,
              remoteAddr: remoteAddr.trim(),
              strategy: "fifo",
            });

            if (response.code === 0) {
              setImportResults((prev) => [
                {
                  line,
                  success: true,
                  message: "创建成功",
                  forwardName: name.trim(),
                },
                ...prev,
              ]);
            } else {
              setImportResults((prev) => [
                {
                  line,
                  success: false,
                  message: response.msg || "创建失败",
                },
                ...prev,
              ]);
            }
          } catch {
            setImportResults((prev) => [
              {
                line,
                success: false,
                message: "网络错误，创建失败",
              },
              ...prev,
            ]);
          }
        }
      }

      toast.success("导入执行完成");

      await loadData(false);
    } catch {
      toast.error("导入过程中发生错误");
    } finally {
      setImportLoading(false);
    }
  };

  // 获取状态显示
  const getStatusDisplay = (status: number) => {
    switch (status) {
      case 1:
        return { color: "success", text: "正常" };
      case 0:
        return { color: "warning", text: "暂停" };
      case -1:
        return { color: "danger", text: "异常" };
      default:
        return { color: "default", text: "未知" };
    }
  };

  // 获取策略显示
  const getStrategyDisplay = (strategy: string) => {
    switch (strategy) {
      case "fifo":
        return { color: "primary", text: "主备" };
      case "round":
        return { color: "success", text: "轮询" };
      case "rand":
        return { color: "warning", text: "随机" };
      default:
        return { color: "default", text: "未知" };
    }
  };

  // 获取地址数量
  const getAddressCount = (addressString: string): number => {
    if (!addressString) return 0;
    const addresses = addressString
      .split("\n")
      .map((addr) => addr.trim())
      .filter((addr) => addr);

    return addresses.length;
  };

  // 处理拖拽结束
  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event;

    if (!active || !over || active.id === over.id) return;

    const activeGroup = parseTunnelGroupSortableId(active.id);
    const overGroup = parseTunnelGroupSortableId(over.id);

    if (activeGroup && overGroup) {
      if (compactMode || !groupPreferenceHydrated) {
        return;
      }

      if (activeGroup.userId !== overGroup.userId) {
        return;
      }

      const userIdKey = activeGroup.userId.toString();
      const currentOrder = groupOrderMap[userIdKey] || [];
      const oldIndex = currentOrder.indexOf(activeGroup.tunnelKey);
      const newIndex = currentOrder.indexOf(overGroup.tunnelKey);

      if (oldIndex < 0 || newIndex < 0 || oldIndex === newIndex) {
        return;
      }

      const moved = arrayMove(currentOrder, oldIndex, newIndex);
      const nextOrderMap: ForwardGroupOrderMap = {
        ...groupOrderMap,
        [userIdKey]: moved,
      };

      setGroupOrderMap(nextOrderMap);
      persistGroupOrderToLocal(nextOrderMap);
      void persistGroupOrderToGlobal(nextOrderMap);

      return;
    }

    // 确保 forwardOrder 存在且有效
    if (!forwardOrder || forwardOrder.length === 0) return;

    const activeId = Number(active.id);
    const overId = Number(over.id);

    // 检查 ID 是否有效
    if (isNaN(activeId) || isNaN(overId)) return;

    const activeForward = forwards.find((forward) => forward.id === activeId);
    const overForward = forwards.find((forward) => forward.id === overId);
    const activeUserId = activeForward?.userId ?? 0;
    const overUserId = overForward?.userId ?? 0;
    const activeTunnelGroupKey = buildForwardTunnelGroupKey(
      activeForward?.tunnelName,
    );
    const overTunnelGroupKey = buildForwardTunnelGroupKey(
      overForward?.tunnelName,
    );

    // 非精简模式仅允许在同一用户+隧道分组内拖拽，避免混排
    if (!compactMode) {
      if (
        activeUserId !== overUserId ||
        activeTunnelGroupKey !== overTunnelGroupKey
      ) {
        return;
      }
    }

    let oldIndex: number;
    let newIndex: number;
    let currentOrder: number[];

    if (compactMode) {
      currentOrder = sortedForwards.map((f) => f.id);
      oldIndex = currentOrder.indexOf(activeId);
      newIndex = currentOrder.indexOf(overId);
    } else {
      currentOrder = forwardOrder;
      oldIndex = forwardOrder.indexOf(activeId);
      newIndex = forwardOrder.indexOf(overId);
    }

    if (oldIndex !== -1 && newIndex !== -1 && oldIndex !== newIndex) {
      const newOrder = arrayMove(currentOrder, oldIndex, newIndex);

      if (!compactMode) {
        setForwardOrder(newOrder);
        saveOrder(FORWARD_ORDER_KEY, newOrder);
      }

      // 持久化到数据库
      try {
        const forwardsToUpdate = newOrder.map((id, index) => ({
          id,
          inx: index,
        }));

        const response = await updateForwardOrder({
          forwards: forwardsToUpdate,
        });

        if (response.code === 0) {
          // 更新本地数据中的 inx 字段
          setForwards((prev) =>
            prev.map((forward) => {
              const updatedForward = forwardsToUpdate.find(
                (f) => f.id === forward.id,
              );

              if (updatedForward) {
                return { ...forward, inx: updatedForward.inx };
              }

              return forward;
            }),
          );
        } else {
          toast.error("保存排序失败：" + (response.msg || "未知错误"));
        }
      } catch {
        toast.error("保存排序失败，请重试");
      }
    }
  };

  const toggleSelectMode = () => {
    setSelectMode(!selectMode);
    if (selectMode) {
      setSelectedIds(new Set());
    }
  };

  const toggleSelect = (id: number) => {
    const newSet = new Set(selectedIds);

    if (newSet.has(id)) {
      newSet.delete(id);
    } else {
      newSet.add(id);
    }
    setSelectedIds(newSet);
  };

  const selectAll = () => {
    const allIds = sortedForwards.map((f) => f.id);

    setSelectedIds(new Set(allIds));
  };

  const deselectAll = () => {
    setSelectedIds(new Set());
  };

  const handleBatchDelete = async () => {
    if (selectedIds.size === 0) return;
    setBatchLoading(true);
    try {
      const outcome = await executeForwardBatchDelete(Array.from(selectedIds));

      if (outcome.toastVariant === "success") {
        toast.success(outcome.toastMessage);
      } else {
        toast.error(outcome.toastMessage);
      }

      if (outcome.shouldRefresh) {
        setSelectedIds(new Set());
        setSelectMode(false);
        if (outcome.closeDeleteModal) {
          setBatchDeleteModalOpen(false);
        }
        loadData(false);
      }
    } finally {
      setBatchLoading(false);
    }
  };

  const handleBatchToggleService = async (enable: boolean) => {
    if (selectedIds.size === 0) return;
    setBatchLoading(true);
    try {
      const outcome = await executeForwardBatchToggleService(
        Array.from(selectedIds),
        enable,
      );

      if (outcome.toastVariant === "success") {
        toast.success(outcome.toastMessage);
      } else {
        toast.error(outcome.toastMessage);
      }

      if (outcome.shouldRefresh) {
        setSelectedIds(new Set());
        setSelectMode(false);
        loadData(false);
      }
    } finally {
      setBatchLoading(false);
    }
  };

  const handleBatchRedeploy = async () => {
    if (selectedIds.size === 0) return;
    setBatchLoading(true);
    try {
      const outcome = await executeForwardBatchRedeploy(
        Array.from(selectedIds),
      );

      if (outcome.toastVariant === "success") {
        toast.success(outcome.toastMessage);
      } else {
        toast.error(outcome.toastMessage);
      }

      if (outcome.shouldRefresh) {
        setSelectedIds(new Set());
        setSelectMode(false);
        loadData(false);
      }
    } finally {
      setBatchLoading(false);
    }
  };

  const handleBatchChangeTunnel = async () => {
    if (selectedIds.size === 0 || !batchTargetTunnelId) return;
    setBatchLoading(true);
    try {
      const outcome = await executeForwardBatchChangeTunnel(
        Array.from(selectedIds),
        batchTargetTunnelId,
      );

      if (outcome.toastVariant === "success") {
        toast.success(outcome.toastMessage);
      } else {
        toast.error(outcome.toastMessage);
      }

      if (outcome.shouldRefresh) {
        setSelectedIds(new Set());
        setSelectMode(false);
        if (outcome.closeChangeTunnelModal) {
          setBatchChangeTunnelModalOpen(false);
        }
        if (outcome.resetTargetTunnel) {
          setBatchTargetTunnelId(null);
        }
        loadData(false);
      }
    } finally {
      setBatchLoading(false);
    }
  };

  // 传感器配置 - 使用默认配置避免错误
  const sensors = useSensors(
    useSensor(MouseSensor, {
      activationConstraint: {
        distance: 8,
      },
    }),
    useSensor(TouchSensor, {
      activationConstraint: {
        delay: 250,
        tolerance: 8,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  // 根据排序顺序获取规则列表
  const orderedForwards = useMemo((): Forward[] => {
    // 确保 forwards 数组存在且有效
    if (!forwards || forwards.length === 0) {
      return [];
    }

    let filteredForwards = forwards;

    if (filterUserId !== "all") {
      const targetUserId = parseInt(filterUserId);

      filteredForwards = filteredForwards.filter(
        (f) => f.userId === targetUserId || (targetUserId === 0 && !f.userId),
      );
    }
    if (filterTunnelId !== "all") {
      const targetTunnelId = parseInt(filterTunnelId);

      filteredForwards = filteredForwards.filter(
        (f) => f.tunnelId === targetTunnelId,
      );
    }

    if (searchKeyword.trim()) {
      const lowerKeyword = searchKeyword.toLowerCase();

      filteredForwards = filteredForwards.filter(
        (f) =>
          (f.name && f.name.toLowerCase().includes(lowerKeyword)) ||
          (f.remoteAddr && f.remoteAddr.toLowerCase().includes(lowerKeyword)) ||
          (f.userName && f.userName.toLowerCase().includes(lowerKeyword)),
      );
    }

    // 确保过滤后的规则列表有效
    if (!filteredForwards || filteredForwards.length === 0) {
      return [];
    }

    // 优先使用数据库中的 inx 字段进行排序
    const sortedByDb = [...filteredForwards].sort((a, b) => {
      const aInx = a.inx ?? 0;
      const bInx = b.inx ?? 0;

      if (aInx !== bInx) {
        return aInx - bInx;
      }

      return (a.id ?? 0) - (b.id ?? 0);
    });

    // 如果数据库中没有排序信息，则使用本地存储的顺序
    if (
      forwardOrder &&
      forwardOrder.length > 0 &&
      sortedByDb.every((f) => f.inx === undefined || f.inx === 0)
    ) {
      const forwardMap = new Map(filteredForwards.map((f) => [f.id, f]));
      const localSortedForwards: Forward[] = [];

      forwardOrder.forEach((id) => {
        const forward = forwardMap.get(id);

        if (forward) {
          localSortedForwards.push(forward);
        }
      });

      // 添加不在排序列表中的规则（新添加的）
      filteredForwards.forEach((forward) => {
        if (!forwardOrder.includes(forward.id)) {
          localSortedForwards.push(forward);
        }
      });

      return localSortedForwards;
    }

    return sortedByDb;
  }, [forwards, forwardOrder, filterUserId, filterTunnelId, searchKeyword]);

  const availableGroupData = useMemo(
    () => buildAvailableGroupData(forwards),
    [forwards],
  );

  const sanitizedGroupOrderMap = useMemo(
    () =>
      sanitizeGroupOrderMap(
        groupOrderMap,
        availableGroupData.availableTunnelKeysByUser,
      ),
    [groupOrderMap, availableGroupData],
  );

  const sanitizedCollapsedTunnelGroups = useMemo(
    () =>
      sanitizeGroupCollapsedMap(
        collapsedTunnelGroups,
        availableGroupData.availableCollapseKeys,
      ),
    [collapsedTunnelGroups, availableGroupData],
  );

  useEffect(() => {
    if (!groupPreferenceHydrated || tokenUserId === null) {
      return;
    }

    if (forwards.length === 0) {
      return;
    }

    if (!isSameGroupOrderMap(groupOrderMap, sanitizedGroupOrderMap)) {
      setGroupOrderMap(sanitizedGroupOrderMap);
      persistGroupOrderToLocal(sanitizedGroupOrderMap);
      void persistGroupOrderToGlobal(sanitizedGroupOrderMap);
    }

    if (
      !isSameGroupCollapsedMap(
        collapsedTunnelGroups,
        sanitizedCollapsedTunnelGroups,
      )
    ) {
      setCollapsedTunnelGroups(sanitizedCollapsedTunnelGroups);
      persistGroupCollapsedToLocal(sanitizedCollapsedTunnelGroups);
      void persistGroupCollapsedToGlobal(sanitizedCollapsedTunnelGroups);
    }
  }, [
    groupPreferenceHydrated,
    tokenUserId,
    forwards,
    groupOrderMap,
    sanitizedGroupOrderMap,
    collapsedTunnelGroups,
    sanitizedCollapsedTunnelGroups,
  ]);

  const groupedForwards = useMemo((): ForwardUserGroup[] => {
    if (orderedForwards.length === 0) {
      return [];
    }

    type MutableForwardUserGroup = {
      userId: number;
      userName: string;
      tunnelMap: Map<string, ForwardTunnelGroup>;
    };

    const userGroupMap = new Map<number, MutableForwardUserGroup>();

    orderedForwards.forEach((forward) => {
      const userId = forward.userId ?? 0;
      const userName = normalizeForwardUserName(forward.userName);
      const tunnelName = normalizeForwardTunnelName(forward.tunnelName);
      const tunnelKey = buildForwardTunnelGroupKey(forward.tunnelName);

      let existingGroup = userGroupMap.get(userId);

      if (!existingGroup) {
        existingGroup = {
          userId,
          userName,
          tunnelMap: new Map<string, ForwardTunnelGroup>(),
        };
        userGroupMap.set(userId, existingGroup);
      } else if (
        existingGroup.userName === UNKNOWN_FORWARD_USER_NAME &&
        userName !== UNKNOWN_FORWARD_USER_NAME
      ) {
        existingGroup.userName = userName;
      }

      const existingTunnelGroup = existingGroup.tunnelMap.get(tunnelKey);

      if (!existingTunnelGroup) {
        existingGroup.tunnelMap.set(tunnelKey, {
          tunnelKey,
          tunnelName,
          items: [forward],
        });

        return;
      }

      existingTunnelGroup.items.push(forward);

      if (
        existingTunnelGroup.tunnelName === UNCATEGORIZED_FORWARD_TUNNEL_NAME &&
        tunnelName !== UNCATEGORIZED_FORWARD_TUNNEL_NAME
      ) {
        existingTunnelGroup.tunnelName = tunnelName;
      }
    });

    const groups = Array.from(userGroupMap.values()).map((group) => {
      const tunnels = Array.from(group.tunnelMap.values());
      const tunnelOrder = sanitizedGroupOrderMap[group.userId.toString()] || [];
      const tunnelOrderIndex = new Map<string, number>();

      tunnelOrder.forEach((key, index) => {
        tunnelOrderIndex.set(key, index);
      });

      tunnels.sort((a, b) => {
        const aIndex = tunnelOrderIndex.get(a.tunnelKey);
        const bIndex = tunnelOrderIndex.get(b.tunnelKey);

        if (aIndex !== undefined || bIndex !== undefined) {
          if (aIndex === undefined) {
            return 1;
          }

          if (bIndex === undefined) {
            return -1;
          }

          return aIndex - bIndex;
        }

        const nameCompare = compareForwardTunnelNameAsc(
          a.tunnelName,
          b.tunnelName,
        );

        if (nameCompare !== 0) {
          return nameCompare;
        }

        return compareForwardTunnelNameAsc(a.tunnelKey, b.tunnelKey);
      });

      return {
        userId: group.userId,
        userName: group.userName,
        tunnels,
      };
    });

    groups.sort((a, b) => {
      if (isAdmin && tokenUserId !== null) {
        const aIsSelf = a.userId === tokenUserId;
        const bIsSelf = b.userId === tokenUserId;

        if (aIsSelf !== bIsSelf) {
          return aIsSelf ? -1 : 1;
        }
      }

      const nameCompare = compareForwardUserNameAsc(a.userName, b.userName);

      if (nameCompare !== 0) {
        return nameCompare;
      }

      return a.userId - b.userId;
    });

    return groups;
  }, [orderedForwards, isAdmin, tokenUserId, sanitizedGroupOrderMap]);

  const sortedForwards = useMemo(() => {
    if (compactMode) {
      return orderedForwards;
    }

    return groupedForwards.flatMap((group) =>
      group.tunnels.flatMap((tunnel) => tunnel.items),
    );
  }, [compactMode, orderedForwards, groupedForwards]);

  const sortableForwardIds = useMemo(
    () => sortedForwards.map((f) => f.id).filter((id) => id > 0),
    [sortedForwards],
  );

  const toggleTunnelGroupCollapsed = (userId: number, tunnelKey: string) => {
    const collapseKey = buildTunnelGroupCollapseKey(userId, tunnelKey);
    const nextCollapsedMap: ForwardGroupCollapsedMap = {
      ...sanitizedCollapsedTunnelGroups,
    };

    if (nextCollapsedMap[collapseKey] === true) {
      delete nextCollapsedMap[collapseKey];
    } else {
      nextCollapsedMap[collapseKey] = true;
    }

    setCollapsedTunnelGroups(nextCollapsedMap);
    persistGroupCollapsedToLocal(nextCollapsedMap);
    void persistGroupCollapsedToGlobal(nextCollapsedMap);
  };

  const SortableTunnelGroupContainer = ({
    groupUserId,
    tunnel,
    collapsed,
    onToggleCollapsed,
    wrapperClassName,
    headerClassName,
    titleClassName,
    countClassName,
    bodyClassName,
    children,
  }: {
    groupUserId: number;
    tunnel: ForwardTunnelGroup;
    collapsed: boolean;
    onToggleCollapsed: () => void;
    wrapperClassName: string;
    headerClassName: string;
    titleClassName: string;
    countClassName: string;
    bodyClassName: string;
    children: React.ReactNode;
  }) => {
    const sortableId = buildTunnelGroupSortableId(
      groupUserId,
      tunnel.tunnelKey,
    );
    const {
      attributes,
      listeners,
      setNodeRef,
      transform,
      transition,
      isDragging,
    } = useSortable({ id: sortableId });

    const style: React.CSSProperties = {
      transform: transform
        ? CSS.Transform.toString({
            ...transform,
            x: Math.round(transform.x),
            y: Math.round(transform.y),
          })
        : undefined,
      transition: isDragging ? undefined : transition || undefined,
      opacity: isDragging ? 0.55 : 1,
      willChange: isDragging ? "transform" : undefined,
      zIndex: isDragging ? 1 : undefined,
    };

    return (
      <div ref={setNodeRef} className={wrapperClassName} style={style}>
        <div className={headerClassName}>
          <div className="flex items-center gap-2 min-w-0">
            <Button
              isIconOnly
              aria-label={collapsed ? "展开分组" : "折叠分组"}
              className="h-7 w-7 min-w-7"
              size="sm"
              variant="light"
              onPress={onToggleCollapsed}
            >
              <svg
                aria-hidden="true"
                className={`h-4 w-4 transition-transform ${collapsed ? "-rotate-90" : "rotate-0"}`}
                fill="none"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="2"
                viewBox="0 0 24 24"
              >
                <path d="m6 9 6 6 6-6" />
              </svg>
            </Button>
            <span className={titleClassName}>{tunnel.tunnelName}</span>
          </div>
          <div className="flex items-center gap-2">
            <span className={countClassName}>{tunnel.items.length} 条规则</span>
            <div
              className="cursor-grab active:cursor-grabbing p-1 text-default-400 hover:text-default-600 transition-colors"
              title="拖拽分组排序"
              {...attributes}
              {...listeners}
            >
              <svg
                aria-hidden="true"
                className="w-4 h-4"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path d="M7 2a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 2zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 8zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 14zm6-8a2 2 0 1 1-.001-4.001A2 2 0 0 1 13 6zm0 2a2 2 0 1 1 .001 4.001A2 2 0 0 1 13 8zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 13 14z" />
              </svg>
            </div>
          </div>
        </div>
        {!collapsed && <div className={bodyClassName}>{children}</div>}
      </div>
    );
  };

  // 可拖拽的规则卡片组件
  const SortableForwardCard = ({ forward }: { forward: Forward }) => {
    const {
      attributes,
      listeners,
      setNodeRef,
      transform,
      transition,
      isDragging,
    } = useSortable({ id: forward.id });

    const style: React.CSSProperties = {
      transform: transform
        ? CSS.Transform.toString({
            ...transform,
            x: Math.round(transform.x),
            y: Math.round(transform.y),
          })
        : undefined,
      transition: isDragging ? undefined : transition || undefined,
      opacity: isDragging ? 0.5 : 1,
      willChange: isDragging ? "transform" : undefined,
    };

    return (
      <div ref={setNodeRef} className="h-full" style={style} {...attributes}>
        {renderForwardCard(forward, listeners)}
      </div>
    );
  };

  // 生成用作筛选项的用户和隧道列表
  const uniqueUsers = useMemo(() => {
    const userMap = new Map<number, { id: number; name: string }>();

    forwards.forEach((f) => {
      const uId = f.userId ?? 0;
      const userName = normalizeForwardUserName(f.userName);
      const existingUser = userMap.get(uId);

      if (!existingUser) {
        userMap.set(uId, { id: uId, name: userName });

        return;
      }

      if (
        existingUser.name === UNKNOWN_FORWARD_USER_NAME &&
        userName !== UNKNOWN_FORWARD_USER_NAME
      ) {
        existingUser.name = userName;
      }
    });

    const users = Array.from(userMap.values());

    users.sort((a, b) => {
      if (isAdmin && tokenUserId !== null) {
        const aIsSelf = a.id === tokenUserId;
        const bIsSelf = b.id === tokenUserId;

        if (aIsSelf !== bIsSelf) {
          return aIsSelf ? -1 : 1;
        }
      }

      const nameCompare = compareForwardUserNameAsc(a.name, b.name);

      if (nameCompare !== 0) {
        return nameCompare;
      }

      return a.id - b.id;
    });

    return users;
  }, [forwards, isAdmin, tokenUserId]);

  // 可拖拽的表格行组件
  const SortableTableRow = ({
    forward,
    selectMode,
    selectedIds,
    toggleSelect,
    getStrategyDisplay,
    formatInAddress,
    formatRemoteAddress,
    handleServiceToggle,
    handleEdit,
    handleDelete,
    handleDiagnose,
    showAddressModal,
    hasMultipleAddresses,
    formatFlow,
  }: any) => {
    const {
      attributes,
      listeners,
      setNodeRef,
      transform,
      transition,
      isDragging,
    } = useSortable({ id: forward.id });

    const style = {
      transform: transform
        ? CSS.Transform.toString({
            ...transform,
            x: Math.round(transform.x),
            y: Math.round(transform.y),
          })
        : undefined,
      transition: isDragging ? undefined : transition || undefined,
      opacity: isDragging ? 0.5 : 1,
      backgroundColor: isDragging ? "var(--nextui-default-100)" : undefined,
    };

    const strategyDisplay = getStrategyDisplay(forward.strategy);

    return (
      <TableRow key={forward.id} ref={setNodeRef} style={style}>
        {selectMode && (
          <TableCell className={FORWARD_GROUPED_TABLE_COLUMN_CLASS.select}>
            <Checkbox
              isSelected={selectedIds.has(forward.id)}
              onValueChange={() => toggleSelect(forward.id)}
            />
          </TableCell>
        )}
        <TableCell className={FORWARD_GROUPED_TABLE_COLUMN_CLASS.drag}>
          <div
            className="cursor-grab active:cursor-grabbing p-1 text-default-400 hover:text-default-600 transition-colors"
            {...attributes}
            {...listeners}
            title="拖拽排序"
          >
            <svg
              aria-hidden="true"
              className="w-4 h-4"
              fill="currentColor"
              viewBox="0 0 20 20"
            >
              <path d="M7 2a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 2zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 8zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 14zm6-8a2 2 0 1 1-.001-4.001A2 2 0 0 1 13 6zm0 2a2 2 0 1 1 .001 4.001A2 2 0 0 1 13 8zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 13 14z" />
            </svg>
          </div>
        </TableCell>
        <TableCell
          className={`${FORWARD_GROUPED_TABLE_COLUMN_CLASS.name} whitespace-nowrap font-semibold text-foreground`}
        >
          {forward.name}
        </TableCell>
        <TableCell
          className={`${FORWARD_GROUPED_TABLE_COLUMN_CLASS.inbound} max-w-[280px]`}
        >
          <button
            className={`w-full truncate rounded-md bg-default-100/50 px-2.5 py-1.5 text-left font-mono text-xs font-medium text-default-700 transition-all ${
              hasMultipleAddresses(forward.inIp)
                ? "hover:bg-default-200 hover:shadow-sm"
                : ""
            }`}
            title={formatInAddress(forward.inIp, forward.inPort)}
            type="button"
            onClick={() =>
              showAddressModal(forward.inIp, forward.inPort, "入口端口")
            }
          >
            {formatInAddress(forward.inIp, forward.inPort)}
          </button>
        </TableCell>
        <TableCell
          className={`${FORWARD_GROUPED_TABLE_COLUMN_CLASS.target} max-w-[280px]`}
        >
          <button
            className={`w-full truncate rounded-md bg-default-100/50 px-2.5 py-1.5 text-left font-mono text-xs font-medium text-default-700 transition-all ${
              hasMultipleAddresses(forward.remoteAddr)
                ? "hover:bg-default-200 hover:shadow-sm"
                : ""
            }`}
            title={formatRemoteAddress(forward.remoteAddr)}
            type="button"
            onClick={() =>
              showAddressModal(forward.remoteAddr, null, "目标地址")
            }
          >
            {formatRemoteAddress(forward.remoteAddr)}
          </button>
        </TableCell>
        <TableCell className={FORWARD_GROUPED_TABLE_COLUMN_CLASS.strategy}>
          <Chip
            className="text-xs font-medium"
            color={strategyDisplay.color as any}
            size="sm"
            variant="flat"
          >
            {strategyDisplay.text}
          </Chip>
        </TableCell>
        <TableCell
          className={`${FORWARD_GROUPED_TABLE_COLUMN_CLASS.totalFlow} whitespace-nowrap`}
        >
          <span className="text-sm font-medium text-default-600 font-mono">
            {formatFlow(getForwardDisplayFlow(forward))}
          </span>
        </TableCell>
        <TableCell className={FORWARD_GROUPED_TABLE_COLUMN_CLASS.status}>
          <div className="flex items-center gap-2.5 whitespace-nowrap">
            <Switch
              color="success"
              isDisabled={forward.status !== 1 && forward.status !== 0}
              isSelected={forward.serviceRunning}
              size="sm"
              onValueChange={() => handleServiceToggle(forward)}
            />
          </div>
        </TableCell>
        <TableCell className={FORWARD_GROUPED_TABLE_COLUMN_CLASS.actions}>
          <div className="flex justify-end gap-2">
            <Button
              isIconOnly
              className="bg-primary/10 text-primary hover:bg-primary/20"
              size="sm"
              title="编辑"
              onPress={() => handleEdit(forward)}
            >
              <svg
                className="h-4 w-4"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                />
              </svg>
            </Button>
            <Button
              isIconOnly
              className="bg-warning/10 text-warning hover:bg-warning/20"
              size="sm"
              title="诊断"
              onPress={() => handleDiagnose(forward)}
            >
              <svg
                className="h-4 w-4"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                />
              </svg>
            </Button>
            <Button
              isIconOnly
              className="bg-danger/10 text-danger hover:bg-danger/20"
              size="sm"
              title="删除"
              onPress={() => handleDelete(forward)}
            >
              <svg
                className="h-4 w-4"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                />
              </svg>
            </Button>
          </div>
        </TableCell>
      </TableRow>
    );
  };

  const SortableCompactTableRow = ({
    forward,
    selectMode,
    selectedIds,
    toggleSelect,
    getStrategyDisplay,
    formatInAddress,
    formatRemoteAddress,
    handleServiceToggle,
    handleEdit,
    handleDelete,
    handleDiagnose,
    showAddressModal,
    hasMultipleAddresses,
    formatFlow,
  }: any) => {
    const {
      attributes,
      listeners,
      setNodeRef,
      transform,
      transition,
      isDragging,
    } = useSortable({ id: forward.id });

    const style = {
      transform: transform
        ? CSS.Transform.toString({
            ...transform,
            x: Math.round(transform.x),
            y: Math.round(transform.y),
          })
        : undefined,
      transition: isDragging ? undefined : transition || undefined,
      opacity: isDragging ? 0.5 : 1,
      backgroundColor: isDragging ? "var(--nextui-default-100)" : undefined,
    };

    const strategyDisplay = getStrategyDisplay(forward.strategy);

    return (
      <TableRow key={forward.id} ref={setNodeRef} style={style}>
        {selectMode && (
          <TableCell>
            <Checkbox
              isSelected={selectedIds.has(forward.id)}
              onValueChange={() => toggleSelect(forward.id)}
            />
          </TableCell>
        )}
        <TableCell>
          <div
            className="cursor-grab active:cursor-grabbing p-1 text-default-400 hover:text-default-600 transition-colors"
            {...attributes}
            {...listeners}
            title="拖拽排序"
          >
            <svg
              aria-hidden="true"
              className="w-4 h-4"
              fill="currentColor"
              viewBox="0 0 20 20"
            >
              <path d="M7 2a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 2zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 8zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 14zm6-8a2 2 0 1 1-.001-4.001A2 2 0 0 1 13 6zm0 2a2 2 0 1 1 .001 4.001A2 2 0 0 1 13 8zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 13 14z" />
            </svg>
          </div>
        </TableCell>
        <TableCell className="whitespace-nowrap">
          <span className="text-sm font-medium text-default-700">
            {forward.userName || "未知用户"}
          </span>
        </TableCell>
        <TableCell className="whitespace-nowrap font-semibold text-foreground">
          {forward.name}
        </TableCell>
        <TableCell className="whitespace-nowrap">
          <Chip
            className="border-none bg-secondary/10 px-2"
            color="secondary"
            size="sm"
          >
            <span className="font-medium text-secondary-700">
              {forward.tunnelName}
            </span>
          </Chip>
        </TableCell>
        <TableCell className="max-w-[220px]">
          <button
            className={`w-full truncate rounded-md bg-default-100/50 px-2.5 py-1.5 text-left font-mono text-xs font-medium text-default-700 transition-all ${
              hasMultipleAddresses(forward.inIp)
                ? "hover:bg-default-200 hover:shadow-sm"
                : ""
            }`}
            title={formatInAddress(forward.inIp, forward.inPort)}
            type="button"
            onClick={() =>
              showAddressModal(forward.inIp, forward.inPort, "入口端口")
            }
          >
            {formatInAddress(forward.inIp, forward.inPort)}
          </button>
        </TableCell>
        <TableCell className="max-w-[240px]">
          <button
            className={`w-full truncate rounded-md bg-default-100/50 px-2.5 py-1.5 text-left font-mono text-xs font-medium text-default-700 transition-all ${
              hasMultipleAddresses(forward.remoteAddr)
                ? "hover:bg-default-200 hover:shadow-sm"
                : ""
            }`}
            title={formatRemoteAddress(forward.remoteAddr)}
            type="button"
            onClick={() =>
              showAddressModal(forward.remoteAddr, null, "目标地址")
            }
          >
            {formatRemoteAddress(forward.remoteAddr)}
          </button>
        </TableCell>
        <TableCell>
          <Chip
            className="text-xs font-medium"
            color={strategyDisplay.color as any}
            size="sm"
            variant="flat"
          >
            {strategyDisplay.text}
          </Chip>
        </TableCell>
        <TableCell className="whitespace-nowrap">
          <span className="text-sm font-medium text-default-600 font-mono">
            {formatFlow(getForwardDisplayFlow(forward))}
          </span>
        </TableCell>
        <TableCell>
          <div className="flex items-center gap-2.5 whitespace-nowrap">
            <Switch
              color="success"
              isDisabled={forward.status !== 1 && forward.status !== 0}
              isSelected={forward.serviceRunning}
              size="sm"
              onValueChange={() => handleServiceToggle(forward)}
            />
          </div>
        </TableCell>
        <TableCell>
          <div className="flex justify-end gap-2">
            <Button
              isIconOnly
              className="bg-primary/10 text-primary hover:bg-primary/20"
              size="sm"
              title="编辑"
              onPress={() => handleEdit(forward)}
            >
              <svg
                className="h-4 w-4"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                />
              </svg>
            </Button>
            <Button
              isIconOnly
              className="bg-warning/10 text-warning hover:bg-warning/20"
              size="sm"
              title="诊断"
              onPress={() => handleDiagnose(forward)}
            >
              <svg
                className="h-4 w-4"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                />
              </svg>
            </Button>
            <Button
              isIconOnly
              className="bg-danger/10 text-danger hover:bg-danger/20"
              size="sm"
              title="删除"
              onPress={() => handleDelete(forward)}
            >
              <svg
                className="h-4 w-4"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                />
              </svg>
            </Button>
          </div>
        </TableCell>
      </TableRow>
    );
  };

  // 渲染规则卡片
  const renderForwardCard = (forward: Forward, listeners?: any) => {
    const statusDisplay = getStatusDisplay(forward.status);
    const strategyDisplay = getStrategyDisplay(forward.strategy);

    return (
      <Card
        key={forward.id}
        className="group h-full flex flex-col shadow-sm border border-divider hover:shadow-md transition-shadow duration-200 overflow-hidden"
      >
        <CardHeader className="pb-2 md:pb-2">
          <div className="flex justify-between items-start w-full">
            {selectMode && (
              <Checkbox
                className="mr-2"
                isSelected={selectedIds.has(forward.id)}
                onValueChange={() => toggleSelect(forward.id)}
              />
            )}
            <div className="flex-1 min-w-0">
              <h3 className="font-semibold text-foreground truncate text-sm">
                {forward.name}
              </h3>
              <p className="text-xs text-default-500 truncate">
                {normalizeForwardTunnelName(forward.tunnelName)}
              </p>
            </div>
            <div className="flex items-center gap-1.5 ml-2">
              {viewMode === "direct" && (
                <div
                  className={`cursor-grab active:cursor-grabbing p-2 text-default-400 hover:text-default-600 transition-colors touch-manipulation ${
                    isMobile
                      ? "opacity-100" // 移动端始终显示
                      : "opacity-0 group-hover:opacity-100 sm:opacity-0 sm:group-hover:opacity-100"
                  }`}
                  {...listeners}
                  style={{ touchAction: "none" }}
                  title={isMobile ? "长按拖拽排序" : "拖拽排序"}
                >
                  <svg
                    aria-hidden="true"
                    className="w-4 h-4"
                    fill="currentColor"
                    viewBox="0 0 20 20"
                  >
                    <path d="M7 2a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 2zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 8zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 14zm6-8a2 2 0 1 1-.001-4.001A2 2 0 0 1 13 6zm0 2a2 2 0 1 1 .001 4.001A2 2 0 0 1 13 8zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 13 14z" />
                  </svg>
                </div>
              )}
              <Switch
                isDisabled={forward.status !== 1 && forward.status !== 0}
                isSelected={forward.serviceRunning}
                size="sm"
                onValueChange={() => handleServiceToggle(forward)}
              />
              <Chip
                className="text-xs"
                color={statusDisplay.color as any}
                size="sm"
                variant="flat"
              >
                {statusDisplay.text}
              </Chip>
            </div>
          </div>
        </CardHeader>

        <CardBody className="flex flex-1 flex-col pt-0 pb-3 md:pt-0 md:pb-3">
          <div className="space-y-2 flex-1">
            {/* 地址信息 */}
            <div className="space-y-1">
              <button
                className={`cursor-pointer px-2 py-1 bg-default-50 dark:bg-default-100/50 rounded border border-default-200 dark:border-default-300 transition-colors duration-200 ${
                  hasMultipleAddresses(forward.inIp)
                    ? "hover:bg-default-100 dark:hover:bg-default-200/50"
                    : ""
                }`}
                title={formatInAddress(forward.inIp, forward.inPort)}
                type="button"
                onClick={() =>
                  showAddressModal(forward.inIp, forward.inPort, "入口端口")
                }
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-1.5 min-w-0 flex-1">
                    <span className="text-xs font-medium text-default-600 flex-shrink-0">
                      入口:
                    </span>
                    <code className="text-xs font-mono text-foreground truncate min-w-0">
                      {formatInAddress(forward.inIp, forward.inPort)}
                    </code>
                  </div>
                  {hasMultipleAddresses(forward.inIp) && (
                    <svg
                      aria-hidden="true"
                      className="w-3 h-3 text-default-400 flex-shrink-0"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                      />
                    </svg>
                  )}
                </div>
              </button>

              <button
                className={`cursor-pointer px-2 py-1 bg-default-50 dark:bg-default-100/50 rounded border border-default-200 dark:border-default-300 transition-colors duration-200 ${
                  hasMultipleAddresses(forward.remoteAddr)
                    ? "hover:bg-default-100 dark:hover:bg-default-200/50"
                    : ""
                }`}
                title={formatRemoteAddress(forward.remoteAddr)}
                type="button"
                onClick={() =>
                  showAddressModal(forward.remoteAddr, null, "目标地址")
                }
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-1.5 min-w-0 flex-1">
                    <span className="text-xs font-medium text-default-600 flex-shrink-0">
                      目标:
                    </span>
                    <code className="text-xs font-mono text-foreground truncate min-w-0">
                      {formatRemoteAddress(forward.remoteAddr)}
                    </code>
                  </div>
                  {hasMultipleAddresses(forward.remoteAddr) && (
                    <svg
                      aria-hidden="true"
                      className="w-3 h-3 text-default-400 flex-shrink-0"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                      />
                    </svg>
                  )}
                </div>
              </button>
            </div>

            {/* 统计信息 */}
            <div className="flex flex-wrap items-center justify-between pt-2 border-t border-divider gap-1">
              <Chip
                className="text-xs whitespace-nowrap"
                color={strategyDisplay.color as any}
                size="sm"
                variant="flat"
              >
                {strategyDisplay.text}
              </Chip>
              {(forward.inFlow || 0) + (forward.outFlow || 0) > 0 ? (
                <>
                  <div className="flex items-center gap-1">
                    <Chip
                      className="text-xs whitespace-nowrap"
                      color="primary"
                      size="sm"
                      variant="flat"
                    >
                      ↑{formatFlow(forward.inFlow || 0)}
                    </Chip>
                  </div>
                  <Chip
                    className="text-xs whitespace-nowrap"
                    color="success"
                    size="sm"
                    variant="flat"
                  >
                    ↓{formatFlow(forward.outFlow || 0)}
                  </Chip>
                </>
              ) : (forward.federationShareFlow || 0) > 0 ? (
                <Chip
                  className="text-xs whitespace-nowrap"
                  color="secondary"
                  size="sm"
                  variant="flat"
                >
                  共享 {formatFlow(forward.federationShareFlow || 0)}
                </Chip>
              ) : (
                <Chip
                  className="text-xs whitespace-nowrap"
                  color="default"
                  size="sm"
                  variant="flat"
                >
                  总流量 {formatFlow(0)}
                </Chip>
              )}
            </div>
          </div>

          <div className="flex gap-1.5 mt-3">
            <Button
              className="flex-1 min-h-8"
              color="primary"
              size="sm"
              startContent={
                <svg
                  aria-hidden="true"
                  className="w-3 h-3"
                  fill="currentColor"
                  viewBox="0 0 20 20"
                >
                  <path d="M13.586 3.586a2 2 0 112.828 2.828l-.793.793-2.828-2.828.793-.793zM11.379 5.793L3 14.172V17h2.828l8.38-8.379-2.83-2.828z" />
                </svg>
              }
              variant="flat"
              onPress={() => handleEdit(forward)}
            >
              编辑
            </Button>
            <Button
              className="flex-1 min-h-8"
              color="warning"
              size="sm"
              startContent={
                <svg
                  aria-hidden="true"
                  className="w-3 h-3"
                  fill="currentColor"
                  viewBox="0 0 20 20"
                >
                  <path
                    clipRule="evenodd"
                    d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z"
                    fillRule="evenodd"
                  />
                </svg>
              }
              variant="flat"
              onPress={() => handleDiagnose(forward)}
            >
              诊断
            </Button>
            <Button
              className="flex-1 min-h-8"
              color="danger"
              size="sm"
              startContent={
                <svg
                  aria-hidden="true"
                  className="w-3 h-3"
                  fill="currentColor"
                  viewBox="0 0 20 20"
                >
                  <path
                    clipRule="evenodd"
                    d="M9 2a1 1 0 000 2h2a1 1 0 100-2H9z"
                    fillRule="evenodd"
                  />
                  <path
                    clipRule="evenodd"
                    d="M10 18a8 8 0 100-16 8 8 0 000 16zM8 7a1 1 0 012 0v4a1 1 0 11-2 0V7zM12 7a1 1 0 012 0v4a1 1 0 11-2 0V7z"
                    fillRule="evenodd"
                  />
                </svg>
              }
              variant="flat"
              onPress={() => handleDelete(forward)}
            >
              删除
            </Button>
          </div>
        </CardBody>
      </Card>
    );
  };

  if (loading || !groupPreferenceHydrated) {
    return <PageLoadingState message="正在加载..." />;
  }

  return (
    <AnimatedPage className="px-3 lg:px-6 py-8">
      {/* 页面头部 */}
      <div className="flex flex-row items-center justify-between mb-6 gap-3 overflow-x-auto pb-1 no-scrollbar">
        <div
          className={`flex-1 max-w-sm flex items-center gap-2 shrink-0 ${
            isSearchVisible ? "min-w-[200px]" : "min-w-0"
          }`}
        >
          <SearchBar
            isVisible={isSearchVisible}
            placeholder="搜索规则名称、地址或用户名"
            value={searchKeyword}
            onChange={setSearchKeyword}
            onClose={() => setIsSearchVisible(false)}
            onOpen={() => setIsSearchVisible(true)}
          />
        </div>
        <div className="flex h-8 items-center justify-end gap-2 whitespace-nowrap sm:gap-3 shrink-0">
          {selectMode ? (
            <>
              <span className="text-sm text-default-600 shrink-0 hidden sm:inline-block">
                已选择 {selectedIds.size} 项
              </span>
              <Button
                isIconOnly
                className="h-8 w-8 min-w-8"
                color="primary"
                size="sm"
                title="全选"
                variant="flat"
                onPress={selectAll}
              >
                <CheckAllIcon className="w-4 h-4" />
              </Button>
              <Button
                isIconOnly
                className="h-8 w-8 min-w-8"
                color="secondary"
                size="sm"
                title="清空"
                variant="flat"
                onPress={deselectAll}
              >
                <XCircleIcon className="w-4 h-4" />
              </Button>
              <Button
                isIconOnly
                className="h-8 w-8 min-w-8"
                color="danger"
                isDisabled={selectedIds.size === 0}
                size="sm"
                title="删除"
                variant="flat"
                onPress={() => setBatchDeleteModalOpen(true)}
              >
                <DeleteIcon className="w-4 h-4" />
              </Button>
              <Button
                isIconOnly
                className="h-8 w-8 min-w-8"
                color="warning"
                isDisabled={selectedIds.size === 0}
                isLoading={batchLoading}
                size="sm"
                title="停用"
                variant="flat"
                onPress={() => handleBatchToggleService(false)}
              >
                <PauseIcon className="w-4 h-4" />
              </Button>
              <Button
                isIconOnly
                className="h-8 w-8 min-w-8"
                color="success"
                isDisabled={selectedIds.size === 0}
                isLoading={batchLoading}
                size="sm"
                title="启用"
                variant="flat"
                onPress={() => handleBatchToggleService(true)}
              >
                <PlayIcon className="w-4 h-4" />
              </Button>
              <Button
                isIconOnly
                className="bg-cyan-100 text-cyan-700 hover:bg-cyan-200 dark:bg-cyan-900/30 dark:text-cyan-300 dark:hover:bg-cyan-900/45 h-8 w-8 min-w-8"
                color="default"
                isDisabled={selectedIds.size === 0}
                isLoading={batchLoading}
                size="sm"
                title="下发"
                variant="flat"
                onPress={handleBatchRedeploy}
              >
                <SendIcon className="w-4 h-4" />
              </Button>
              <Button
                isIconOnly
                className="bg-violet-100 text-violet-700 hover:bg-violet-200 dark:bg-violet-900/30 dark:text-violet-300 dark:hover:bg-violet-900/45 h-8 w-8 min-w-8"
                color="default"
                isDisabled={selectedIds.size === 0}
                size="sm"
                title="更换隧道"
                variant="flat"
                onPress={() => setBatchChangeTunnelModalOpen(true)}
              >
                <GitBranchIcon className="w-4 h-4" />
              </Button>
              <Button
                isIconOnly
                className="h-8 w-8 min-w-8"
                color="default"
                size="sm"
                title="退出"
                variant="solid"
                onPress={toggleSelectMode}
              >
                <XIcon className="w-4 h-4" />
              </Button>
            </>
          ) : (
            <>
              {/* 筛选按钮 */}
              <Button
                isIconOnly
                aria-label="筛选条件"
                className={`h-8 w-8 min-w-8 relative ${
                  filterUserId !== "all" || filterTunnelId !== "all"
                    ? "bg-orange-200 text-orange-800 dark:bg-orange-900/50 dark:text-orange-300"
                    : "bg-orange-100 text-orange-700 hover:bg-orange-200 dark:bg-orange-900/30 dark:text-orange-300 dark:hover:bg-orange-900/45"
                }`}
                color="default"
                size="sm"
                title="筛选条件"
                variant="flat"
                onPress={() => setIsFilterModalOpen(true)}
              >
                <FunnelIcon className="w-4 h-4" />
                {(filterUserId !== "all" || filterTunnelId !== "all") && (
                  <span className="absolute top-1.5 right-1.5 flex h-1.5 w-1.5 rounded-full bg-red-500" />
                )}
              </Button>
              {/* 显示模式切换按钮 */}
              <Button
                isIconOnly
                aria-label={
                  viewMode === "grouped" ? "切换到直接显示" : "切换到分类显示"
                }
                className="text-sm h-8 w-8 min-w-8 bg-indigo-100 text-indigo-700 hover:bg-indigo-200 dark:bg-indigo-900/30 dark:text-indigo-300 dark:hover:bg-indigo-900/45"
                color="default"
                size="sm"
                title={
                  viewMode === "grouped" ? "切换到直接显示" : "切换到分类显示"
                }
                variant="flat"
                onPress={handleViewModeChange}
              >
                {viewMode === "grouped" ? (
                  <ViewListIcon className="w-4 h-4" />
                ) : (
                  <ViewSplitIcon className="w-4 h-4" />
                )}
              </Button>

              {/* 导入按钮 */}
              <Button
                isIconOnly
                className="h-8 w-8 min-w-8"
                color="warning"
                size="sm"
                title="导入"
                variant="flat"
                onPress={handleImport}
              >
                <UploadIcon className="w-4 h-4" />
              </Button>

              {/* 导出按钮 */}
              <Button
                isIconOnly
                className="h-8 w-8 min-w-8"
                color="success"
                isLoading={exportLoading}
                size="sm"
                title="导出"
                variant="flat"
                onPress={handleExport}
              >
                <DownloadIcon className="w-4 h-4" />
              </Button>

              <Button
                isIconOnly
                className="bg-violet-100 text-violet-700 hover:bg-violet-200 dark:bg-violet-900/30 dark:text-violet-300 dark:hover:bg-violet-900/45 h-8 w-8 min-w-8"
                color="default"
                size="sm"
                title="选择"
                variant="flat"
                onPress={toggleSelectMode}
              >
                <ListCheckIcon className="w-4 h-4" />
              </Button>

              <Button
                isIconOnly
                className="h-8 w-8 min-w-8"
                color="primary"
                size="sm"
                title="新增"
                variant="flat"
                onPress={handleAdd}
              >
                <PlusIcon className="w-4 h-4" />
              </Button>
            </>
          )}
        </div>
      </div>

      {/* 根据显示模式渲染不同内容 */}
      {compactMode ? (
        viewMode === "grouped" ? (
          sortedForwards.length > 0 ? (
            <div className="overflow-hidden rounded-xl border border-divider bg-content1 shadow-md">
              <DndContext
                collisionDetection={closestCenter}
                sensors={sensors}
                onDragEnd={handleDragEnd}
              >
                <SortableContext
                  items={sortableForwardIds}
                  strategy={verticalListSortingStrategy}
                >
                  <Table
                    aria-label="全部规则列表"
                    classNames={{
                      th: "bg-default-100/50 text-default-600 font-semibold text-sm border-b border-divider py-3 uppercase tracking-wider",
                      td: "py-3 border-b border-divider/50 group-data-[last=true]:border-b-0",
                      tr: "hover:bg-default-50/50 transition-colors",
                    }}
                  >
                    <TableHeader>
                      {selectMode && (
                        <TableColumn className="w-14">选择</TableColumn>
                      )}
                      <TableColumn className="w-10 pl-4" />
                      <TableColumn>用户</TableColumn>
                      <TableColumn>名称</TableColumn>
                      <TableColumn>隧道</TableColumn>
                      <TableColumn>入口</TableColumn>
                      <TableColumn>目标</TableColumn>
                      <TableColumn>策略</TableColumn>
                      <TableColumn>总流量</TableColumn>
                      <TableColumn>状态</TableColumn>
                      <TableColumn className="text-right">操作</TableColumn>
                    </TableHeader>
                    <TableBody
                      emptyContent="暂无规则配置"
                      items={sortedForwards}
                    >
                      {(forward) => (
                        <SortableCompactTableRow
                          formatFlow={formatFlow}
                          formatInAddress={formatInAddress}
                          formatRemoteAddress={formatRemoteAddress}
                          forward={forward}
                          getStrategyDisplay={getStrategyDisplay}
                          handleDelete={handleDelete}
                          handleDiagnose={handleDiagnose}
                          handleEdit={handleEdit}
                          handleServiceToggle={handleServiceToggle}
                          hasMultipleAddresses={hasMultipleAddresses}
                          selectMode={selectMode}
                          selectedIds={selectedIds}
                          showAddressModal={showAddressModal}
                          toggleSelect={toggleSelect}
                        />
                      )}
                    </TableBody>
                  </Table>
                </SortableContext>
              </DndContext>
            </div>
          ) : (
            <Card className="shadow-sm border border-gray-200 dark:border-gray-700 bg-default-50/50">
              <CardBody className="text-center py-20 flex flex-col items-center justify-center min-h-[240px]">
                <h3 className="text-xl font-medium text-foreground tracking-tight mb-2">
                  暂无规则配置
                </h3>
                <p className="text-default-500 text-sm max-w-xs mx-auto leading-relaxed">
                  还没有创建任何规则配置，点击上方按钮开始创建
                </p>
              </CardBody>
            </Card>
          )
        ) : sortedForwards.length > 0 ? (
          <>
            <div className="flex items-center justify-between px-1 mb-3">
              <span className="text-sm font-semibold text-foreground">
                全部规则
              </span>
              <span className="text-xs text-default-600">
                {sortedForwards.length} 条规则
              </span>
            </div>
            <DndContext
              collisionDetection={closestCenter}
              sensors={sensors}
              onDragEnd={handleDragEnd}
              onDragStart={() => {}}
            >
              <SortableContext
                items={sortableForwardIds}
                strategy={rectSortingStrategy}
              >
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-4">
                  {sortedForwards.map((forward) =>
                    forward && forward.id ? (
                      <SortableForwardCard key={forward.id} forward={forward} />
                    ) : null,
                  )}
                </div>
              </SortableContext>
            </DndContext>
          </>
        ) : (
          <Card className="shadow-sm border border-gray-200 dark:border-gray-700 bg-default-50/50">
            <CardBody className="text-center py-20 flex flex-col items-center justify-center min-h-[240px]">
              <h3 className="text-xl font-medium text-foreground tracking-tight mb-2">
                暂无规则配置
              </h3>
              <p className="text-default-500 text-sm max-w-xs mx-auto leading-relaxed">
                还没有创建任何规则配置，点击上方按钮开始创建
              </p>
            </CardBody>
          </Card>
        )
      ) : viewMode === "grouped" ? (
        sortedForwards.length > 0 ? (
          <div className="space-y-4">
            {groupedForwards.map((group) => {
              const isSelfGroup =
                isAdmin && tokenUserId !== null && group.userId === tokenUserId;
              const groupForwardCount = group.tunnels.reduce(
                (total, tunnel) => total + tunnel.items.length,
                0,
              );

              return (
                <div
                  key={`grouped-table-${group.userId}-${group.userName}`}
                  className="overflow-hidden rounded-xl border border-divider bg-content1 shadow-md"
                >
                  <div className="flex items-center justify-between border-b border-divider bg-default-100/40 px-4 py-3">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-semibold text-foreground">
                        {group.userName}
                      </span>
                      {isSelfGroup && (
                        <Chip color="primary" size="sm" variant="flat">
                          管理员本人
                        </Chip>
                      )}
                    </div>
                    <span className="text-xs text-default-600">
                      {groupForwardCount} 条规则
                    </span>
                  </div>

                  <div className="space-y-4 p-4">
                    <DndContext
                      collisionDetection={closestCenter}
                      sensors={sensors}
                      onDragEnd={handleDragEnd}
                    >
                      <SortableContext
                        items={group.tunnels.map((tunnel) =>
                          buildTunnelGroupSortableId(
                            group.userId,
                            tunnel.tunnelKey,
                          ),
                        )}
                        strategy={verticalListSortingStrategy}
                      >
                        {group.tunnels.map((tunnel) => {
                          const tunnelSortableForwardIds = tunnel.items
                            .map((item) => item.id)
                            .filter((id) => id > 0);
                          const collapsed =
                            sanitizedCollapsedTunnelGroups[
                              buildTunnelGroupCollapseKey(
                                group.userId,
                                tunnel.tunnelKey,
                              )
                            ] === true;

                          return (
                            <SortableTunnelGroupContainer
                              key={`grouped-table-${group.userId}-${tunnel.tunnelKey}`}
                              bodyClassName=""
                              collapsed={collapsed}
                              countClassName="text-xs text-secondary-700"
                              groupUserId={group.userId}
                              headerClassName="flex items-center justify-between border-b border-secondary/20 bg-secondary/10 px-4 py-2.5"
                              titleClassName="truncate text-sm font-semibold text-secondary-700"
                              tunnel={tunnel}
                              wrapperClassName="overflow-hidden rounded-lg border border-secondary/20 bg-secondary/5"
                              onToggleCollapsed={() =>
                                toggleTunnelGroupCollapsed(
                                  group.userId,
                                  tunnel.tunnelKey,
                                )
                              }
                            >
                              <DndContext
                                collisionDetection={closestCenter}
                                sensors={sensors}
                                onDragEnd={handleDragEnd}
                              >
                                <Table
                                  aria-label={`${group.userName}-${tunnel.tunnelName}规则列表`}
                                  className={`table-fixed ${FORWARD_GROUPED_TABLE_MIN_WIDTH_CLASS}`}
                                  classNames={{
                                    th: "bg-default-100/50 text-default-600 font-semibold text-sm border-b border-divider py-3 uppercase tracking-wider",
                                    td: "py-3 border-b border-divider/50 group-data-[last=true]:border-b-0",
                                    tr: "hover:bg-default-50/50 transition-colors",
                                  }}
                                >
                                  <TableHeader>
                                    {selectMode && (
                                      <TableColumn
                                        className={
                                          FORWARD_GROUPED_TABLE_COLUMN_CLASS.select
                                        }
                                      >
                                        选择
                                      </TableColumn>
                                    )}
                                    <TableColumn
                                      className={
                                        FORWARD_GROUPED_TABLE_COLUMN_CLASS.drag
                                      }
                                    />
                                    <TableColumn
                                      className={
                                        FORWARD_GROUPED_TABLE_COLUMN_CLASS.name
                                      }
                                    >
                                      名称
                                    </TableColumn>
                                    <TableColumn
                                      className={
                                        FORWARD_GROUPED_TABLE_COLUMN_CLASS.inbound
                                      }
                                    >
                                      入口
                                    </TableColumn>
                                    <TableColumn
                                      className={
                                        FORWARD_GROUPED_TABLE_COLUMN_CLASS.target
                                      }
                                    >
                                      目标
                                    </TableColumn>
                                    <TableColumn
                                      className={
                                        FORWARD_GROUPED_TABLE_COLUMN_CLASS.strategy
                                      }
                                    >
                                      策略
                                    </TableColumn>
                                    <TableColumn
                                      className={
                                        FORWARD_GROUPED_TABLE_COLUMN_CLASS.totalFlow
                                      }
                                    >
                                      总流量
                                    </TableColumn>
                                    <TableColumn
                                      className={
                                        FORWARD_GROUPED_TABLE_COLUMN_CLASS.status
                                      }
                                    >
                                      状态
                                    </TableColumn>
                                    <TableColumn
                                      className={
                                        FORWARD_GROUPED_TABLE_COLUMN_CLASS.actions
                                      }
                                    >
                                      操作
                                    </TableColumn>
                                  </TableHeader>
                                  <TableBody
                                    emptyContent="暂无规则配置"
                                    items={tunnel.items}
                                  >
                                    {(forward) => (
                                      <SortableContext
                                        key={forward.id}
                                        items={tunnelSortableForwardIds}
                                        strategy={verticalListSortingStrategy}
                                      >
                                        <SortableTableRow
                                          formatFlow={formatFlow}
                                          formatInAddress={formatInAddress}
                                          formatRemoteAddress={
                                            formatRemoteAddress
                                          }
                                          forward={forward}
                                          getStrategyDisplay={
                                            getStrategyDisplay
                                          }
                                          handleDelete={handleDelete}
                                          handleDiagnose={handleDiagnose}
                                          handleEdit={handleEdit}
                                          handleServiceToggle={
                                            handleServiceToggle
                                          }
                                          hasMultipleAddresses={
                                            hasMultipleAddresses
                                          }
                                          selectMode={selectMode}
                                          selectedIds={selectedIds}
                                          showAddressModal={showAddressModal}
                                          toggleSelect={toggleSelect}
                                        />
                                      </SortableContext>
                                    )}
                                  </TableBody>
                                </Table>
                              </DndContext>
                            </SortableTunnelGroupContainer>
                          );
                        })}
                      </SortableContext>
                    </DndContext>
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          <Card className="shadow-sm border border-gray-200 dark:border-gray-700 bg-default-50/50">
            <CardBody className="text-center py-20 flex flex-col items-center justify-center min-h-[240px]">
              <h3 className="text-xl font-medium text-foreground tracking-tight mb-2">
                暂无规则配置
              </h3>
              <p className="text-default-500 text-sm max-w-xs mx-auto leading-relaxed">
                还没有创建任何规则配置，点击上方按钮开始创建
              </p>
            </CardBody>
          </Card>
        )
      ) : sortedForwards.length > 0 ? (
        <div className="space-y-5">
          {groupedForwards.map((group) => {
            const isSelfGroup =
              isAdmin && tokenUserId !== null && group.userId === tokenUserId;
            const groupForwardCount = group.tunnels.reduce(
              (total, tunnel) => total + tunnel.items.length,
              0,
            );

            return (
              <div
                key={`direct-group-${group.userId}-${group.userName}`}
                className="space-y-3"
              >
                <div className="flex items-center justify-between px-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-semibold text-foreground">
                      {group.userName}
                    </span>
                    {isSelfGroup && (
                      <Chip color="primary" size="sm" variant="flat">
                        管理员本人
                      </Chip>
                    )}
                  </div>
                  <span className="text-xs text-default-600">
                    {groupForwardCount} 条规则
                  </span>
                </div>

                <div className="space-y-4">
                  <DndContext
                    collisionDetection={closestCenter}
                    sensors={sensors}
                    onDragEnd={handleDragEnd}
                  >
                    <SortableContext
                      items={group.tunnels.map((tunnel) =>
                        buildTunnelGroupSortableId(
                          group.userId,
                          tunnel.tunnelKey,
                        ),
                      )}
                      strategy={verticalListSortingStrategy}
                    >
                      {group.tunnels.map((tunnel) => {
                        const tunnelSortableForwardIds = tunnel.items
                          .map((item) => item.id)
                          .filter((id) => id > 0);
                        const collapsed =
                          sanitizedCollapsedTunnelGroups[
                            buildTunnelGroupCollapseKey(
                              group.userId,
                              tunnel.tunnelKey,
                            )
                          ] === true;

                        return (
                          <SortableTunnelGroupContainer
                            key={`direct-group-${group.userId}-${tunnel.tunnelKey}`}
                            bodyClassName="p-3"
                            collapsed={collapsed}
                            countClassName="text-xs text-secondary-700"
                            groupUserId={group.userId}
                            headerClassName="flex items-center justify-between rounded-lg bg-secondary/10 px-3 py-2"
                            titleClassName="truncate text-sm font-semibold text-secondary-700"
                            tunnel={tunnel}
                            wrapperClassName="rounded-xl border border-secondary/20 bg-secondary/5 space-y-3"
                            onToggleCollapsed={() =>
                              toggleTunnelGroupCollapsed(
                                group.userId,
                                tunnel.tunnelKey,
                              )
                            }
                          >
                            <DndContext
                              collisionDetection={closestCenter}
                              sensors={sensors}
                              onDragEnd={handleDragEnd}
                              onDragStart={() => {}}
                            >
                              <SortableContext
                                items={tunnelSortableForwardIds}
                                strategy={rectSortingStrategy}
                              >
                                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-4">
                                  {tunnel.items.map((forward) =>
                                    forward && forward.id ? (
                                      <SortableForwardCard
                                        key={forward.id}
                                        forward={forward}
                                      />
                                    ) : null,
                                  )}
                                </div>
                              </SortableContext>
                            </DndContext>
                          </SortableTunnelGroupContainer>
                        );
                      })}
                    </SortableContext>
                  </DndContext>
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        <Card className="shadow-sm border border-gray-200 dark:border-gray-700 bg-default-50/50">
          <CardBody className="text-center py-20 flex flex-col items-center justify-center min-h-[240px]">
            <h3 className="text-xl font-medium text-foreground tracking-tight mb-2">
              暂无规则配置
            </h3>
            <p className="text-default-500 text-sm max-w-xs mx-auto leading-relaxed">
              还没有创建任何规则配置，点击上方按钮开始创建
            </p>
          </CardBody>
        </Card>
      )}

      {/* 新增/编辑模态框 */}
      <Modal
        backdrop="blur"
        isOpen={modalOpen}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onOpenChange={setModalOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <h2 className="text-xl font-bold">
                  {isEdit ? "编辑规则" : "新增规则"}
                </h2>
                <p className="text-small text-default-500">
                  {isEdit ? "修改现有规则配置的信息" : "创建新的规则配置"}
                </p>
              </ModalHeader>
              <ModalBody>
                <div className="space-y-4 pb-4">
                  <Input
                    errorMessage={errors.name}
                    isInvalid={!!errors.name}
                    label="规则名称"
                    placeholder="请输入规则名称"
                    value={form.name}
                    variant="bordered"
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, name: e.target.value }))
                    }
                  />

                  {isAdmin && (
                    <Select
                      label="限速规则"
                      placeholder="不限速"
                      selectedKeys={
                        selectedSpeedId !== null
                          ? [selectedSpeedId.toString()]
                          : []
                      }
                      variant="bordered"
                      onSelectionChange={(keys) => {
                        const selectedKey = Array.from(keys)[0] as
                          | string
                          | undefined;

                        setForm((prev) => ({
                          ...prev,
                          speedId: selectedKey ? Number(selectedKey) : null,
                        }));
                      }}
                    >
                      {availableSpeedLimits.map((speedLimit) => (
                        <SelectItem
                          key={speedLimit.id.toString()}
                          textValue={speedLimit.name}
                        >
                          {speedLimit.name}
                        </SelectItem>
                      ))}
                    </Select>
                  )}

                  <Select
                    description={
                      isEdit
                        ? "更改隧道将释放原端口并在新隧道分配端口"
                        : undefined
                    }
                    errorMessage={errors.tunnelId}
                    isInvalid={!!errors.tunnelId}
                    label="选择隧道"
                    placeholder="请选择关联的隧道"
                    selectedKeys={
                      form.tunnelId ? [form.tunnelId.toString()] : []
                    }
                    variant="bordered"
                    onSelectionChange={(keys) => {
                      const selectedKey = Array.from(keys)[0] as string;

                      if (selectedKey) {
                        handleTunnelChange(selectedKey);
                      }
                    }}
                  >
                    {tunnels.map((tunnel) => (
                      <SelectItem key={tunnel.id}>{tunnel.name}</SelectItem>
                    ))}
                  </Select>

                  <Input
                    description={
                      currentTunnelPortRange
                        ? `指定入口端口，留空自动分配 (允许范围: ${currentTunnelPortRange.min}-${currentTunnelPortRange.max})`
                        : "指定入口端口，留空则从节点可用端口中自动分配"
                    }
                    errorMessage={errors.inPort}
                    isInvalid={!!errors.inPort}
                    label="入口端口"
                    placeholder="留空则自动分配可用端口"
                    type="number"
                    value={form.inPort !== null ? form.inPort.toString() : ""}
                    variant="bordered"
                    onChange={(e) => {
                      const value = e.target.value;

                      setForm((prev) => ({
                        ...prev,
                        inPort: value ? parseInt(value) : null,
                      }));
                    }}
                  />

                  <Select
                    description={
                      isCurrentTunnelMultiEntrance
                        ? "多入口隧道不支持自定义监听IP，使用各节点默认IP"
                        : "从入口节点IP中选择，留空使用默认"
                    }
                    isDisabled={
                      !form.tunnelId ||
                      currentTunnelIpOptions.length === 0 ||
                      isCurrentTunnelMultiEntrance
                    }
                    label="监听IP"
                    placeholder={
                      isCurrentTunnelMultiEntrance
                        ? "多入口隧道使用节点默认IP"
                        : form.tunnelId
                          ? currentTunnelIpOptions.length > 0
                            ? "选择入口监听IP"
                            : "当前隧道入口节点暂无可选IP"
                          : "请先选择隧道"
                    }
                    selectedKeys={[form.inIp || "__default__"]}
                    variant="bordered"
                    onSelectionChange={(keys) => {
                      const selectedKey = Array.from(keys)[0] as string;

                      setInIpTouched(true);

                      setForm((prev) => ({
                        ...prev,
                        inIp: selectedKey === "__default__" ? "" : selectedKey,
                      }));
                    }}
                  >
                    <SelectItem key="__default__">默认入口IP</SelectItem>
                    {currentTunnelIpOptions.map((ip) => (
                      <SelectItem key={ip}>{ip}</SelectItem>
                    ))}
                  </Select>

                  <Textarea
                    description="格式: IP:端口 或 域名:端口，支持多个地址（每行一个）"
                    errorMessage={errors.remoteAddr}
                    isInvalid={!!errors.remoteAddr}
                    label="远程地址"
                    maxRows={6}
                    minRows={3}
                    placeholder="请输入远程地址，多个地址用换行分隔&#10;例如:&#10;192.168.1.100:8080&#10;example.com:3000"
                    value={form.remoteAddr}
                    variant="bordered"
                    onChange={(e) =>
                      setForm((prev) => ({
                        ...prev,
                        remoteAddr: e.target.value,
                      }))
                    }
                  />

                  {getAddressCount(form.remoteAddr) > 1 && (
                    <Select
                      description="多个目标地址的负载均衡策略"
                      label="负载策略"
                      placeholder="请选择负载均衡策略"
                      selectedKeys={[form.strategy]}
                      variant="bordered"
                      onSelectionChange={(keys) => {
                        const selectedKey = Array.from(keys)[0] as string;

                        setForm((prev) => ({ ...prev, strategy: selectedKey }));
                      }}
                    >
                      <SelectItem key="fifo">主备模式 - 自上而下</SelectItem>
                      <SelectItem key="round">轮询模式 - 依次轮换</SelectItem>
                      <SelectItem key="rand">随机模式 - 随机选择</SelectItem>
                      <SelectItem key="hash">哈希模式 - IP哈希</SelectItem>
                    </Select>
                  )}
                </div>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color="primary"
                  isLoading={submitLoading}
                  onPress={handleSubmit}
                >
                  {isEdit ? "保存修改" : "创建规则"}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* 删除确认模态框 */}
      <Modal
        backdrop="blur"
        isOpen={deleteModalOpen}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onOpenChange={setDeleteModalOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <h2 className="text-lg font-bold text-danger">确认删除</h2>
              </ModalHeader>
              <ModalBody>
                <p className="text-default-600">
                  确定要删除规则{" "}
                  <span className="font-semibold text-foreground">
                    &quot;{forwardToDelete?.name}&quot;
                  </span>{" "}
                  吗？
                </p>
                <p className="text-small text-default-500 mt-2">
                  此操作无法撤销，删除后该规则将永久消失。
                </p>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color="danger"
                  isLoading={deleteLoading}
                  onPress={confirmDelete}
                >
                  确认删除
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* 地址列表弹窗 */}
      <Modal
        isOpen={addressModalOpen}
        scrollBehavior="outside"
        size="lg"
        onClose={() => setAddressModalOpen(false)}
      >
        <ModalContent>
          <ModalHeader className="text-base">{addressModalTitle}</ModalHeader>
          <ModalBody className="pb-6">
            <div className="mb-4 text-right">
              <Button size="sm" onPress={copyAllAddresses}>
                复制
              </Button>
            </div>

            <div className="space-y-2 max-h-60 overflow-y-auto">
              {addressList.map((item) => (
                <div
                  key={item.id}
                  className="flex justify-between items-center p-3 border border-default-200 dark:border-default-100 rounded-lg"
                >
                  <code className="text-sm flex-1 mr-3 text-foreground">
                    {item.address}
                  </code>
                  <Button
                    isLoading={item.copying}
                    size="sm"
                    variant="light"
                    onPress={() => copyAddress(item)}
                  >
                    复制
                  </Button>
                </div>
              ))}
            </div>
          </ModalBody>
        </ModalContent>
      </Modal>

      {/* 导出数据模态框 */}
      <Modal
        backdrop="blur"
        isOpen={exportModalOpen}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onClose={() => {
          setExportModalOpen(false);
          setSelectedTunnelForExport(null);
          setExportData("");
        }}
      >
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">
            <h2 className="text-xl font-bold">导出规则数据</h2>
            <p className="text-small text-default-500">
              格式：目标地址|规则名称|入口端口
            </p>
          </ModalHeader>
          <ModalBody className="pb-6">
            <div className="space-y-4">
              {/* 隧道选择 */}
              <div>
                <Select
                  isRequired
                  label="选择导出隧道"
                  placeholder="请选择要导出的隧道"
                  selectedKeys={
                    selectedTunnelForExport
                      ? [selectedTunnelForExport.toString()]
                      : []
                  }
                  variant="bordered"
                  onSelectionChange={(keys) => {
                    const selectedKey = Array.from(keys)[0] as string;

                    setSelectedTunnelForExport(
                      selectedKey ? parseInt(selectedKey) : null,
                    );
                  }}
                >
                  {tunnels.map((tunnel) => (
                    <SelectItem
                      key={tunnel.id.toString()}
                      textValue={tunnel.name}
                    >
                      {tunnel.name}
                    </SelectItem>
                  ))}
                </Select>
              </div>

              {/* 导出按钮和数据 */}
              {exportData && (
                <div className="flex justify-between items-center">
                  <Button
                    color="primary"
                    isDisabled={!selectedTunnelForExport}
                    isLoading={exportLoading}
                    size="sm"
                    startContent={
                      <svg
                        aria-hidden="true"
                        className="w-4 h-4"
                        fill="currentColor"
                        viewBox="0 0 20 20"
                      >
                        <path
                          clipRule="evenodd"
                          d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zM6.293 6.707a1 1 0 010-1.414l3-3a1 1 0 011.414 0l3 3a1 1 0 01-1.414 1.414L11 5.414V13a1 1 0 11-2 0V5.414L7.707 6.707a1 1 0 01-1.414 0z"
                          fillRule="evenodd"
                        />
                      </svg>
                    }
                    onPress={executeExport}
                  >
                    重新生成
                  </Button>
                  <Button
                    color="secondary"
                    size="sm"
                    startContent={
                      <svg
                        aria-hidden="true"
                        className="w-4 h-4"
                        fill="currentColor"
                        viewBox="0 0 20 20"
                      >
                        <path d="M8 3a1 1 0 011-1h2a1 1 0 110 2H9a1 1 0 01-1-1z" />
                        <path d="M6 3a2 2 0 00-2 2v11a2 2 0 002 2h8a2 2 0 002-2V5a2 2 0 00-2-2 3 3 0 01-3 3H9a3 3 0 01-3-3z" />
                      </svg>
                    }
                    onPress={copyExportData}
                  >
                    复制
                  </Button>
                </div>
              )}

              {/* 初始导出按钮 */}
              {!exportData && (
                <div className="text-right">
                  <Button
                    color="primary"
                    isDisabled={!selectedTunnelForExport}
                    isLoading={exportLoading}
                    size="sm"
                    startContent={
                      <svg
                        aria-hidden="true"
                        className="w-4 h-4"
                        fill="currentColor"
                        viewBox="0 0 20 20"
                      >
                        <path
                          clipRule="evenodd"
                          d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zM6.293 6.707a1 1 0 010-1.414l3-3a1 1 0 011.414 0l3 3a1 1 0 01-1.414 1.414L11 5.414V13a1 1 0 11-2 0V5.414L7.707 6.707a1 1 0 01-1.414 0z"
                          fillRule="evenodd"
                        />
                      </svg>
                    }
                    onPress={executeExport}
                  >
                    生成导出数据
                  </Button>
                </div>
              )}

              {/* 导出数据显示 */}
              {exportData && (
                <div className="relative">
                  <Textarea
                    readOnly
                    className="font-mono text-sm"
                    classNames={{
                      input: "font-mono text-sm",
                    }}
                    maxRows={20}
                    minRows={10}
                    placeholder="暂无数据"
                    value={exportData}
                    variant="bordered"
                  />
                </div>
              )}
            </div>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={() => setExportModalOpen(false)}>
              关闭
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* 导入数据模态框 */}
      <Modal
        backdrop="blur"
        isOpen={importModalOpen}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onClose={() => setImportModalOpen(false)}
      >
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">
            <h2 className="text-xl font-bold">导入规则数据</h2>
            {importFormat === "flvx" ? (
              <>
                <p className="text-small text-default-500">
                  格式：目标地址|规则名称|入口端口，每行一个，入口端口留空将自动分配可用端口
                </p>
                <p className="text-small text-default-400">
                  目标地址支持单个地址(如：example.com:8080)或多个地址用逗号分隔(如：3.3.3.3:3,4.4.4.4:4)
                </p>
              </>
            ) : (
              <>
                <p className="text-small text-default-500">
                  ny格式：JSON对象，支持多个目标地址（负载均衡），按所选隧道导入
                </p>
                <p className="text-small text-default-400">
                  格式：&#123;&quot;dest&quot;:[&quot;地址:端口&quot;],&quot;listen_port&quot;:端口,&quot;name&quot;:&quot;名称&quot;&#125;（listen_port可省略，自动分配端口）
                </p>
              </>
            )}
          </ModalHeader>
          <ModalBody className="pb-6">
            <div className="space-y-4">
              {/* 格式选择 */}
              <Select
                isRequired
                label="导入格式"
                placeholder="请选择导入格式"
                selectedKeys={[importFormat]}
                variant="bordered"
                onSelectionChange={(keys) => {
                  const selectedKey = Array.from(keys)[0] as ImportFormat;

                  if (selectedKey) {
                    setImportFormat(selectedKey);
                    setSelectedTunnelForImport(null);
                    setImportData("");
                    setImportResults([]);
                  }
                }}
              >
                <SelectItem key="flvx" textValue="flvx格式">
                  flvx格式（管道分隔）
                </SelectItem>
                <SelectItem key="ny" textValue="ny格式">
                  ny格式（JSON）
                </SelectItem>
              </Select>

              {/* 隧道选择 - 两种格式都需要 */}
              <Select
                isRequired
                label="选择导入隧道"
                placeholder="请选择要导入的隧道"
                selectedKeys={
                  selectedTunnelForImport
                    ? [selectedTunnelForImport.toString()]
                    : []
                }
                variant="bordered"
                onSelectionChange={(keys) => {
                  const selectedKey = Array.from(keys)[0] as string;

                  setSelectedTunnelForImport(
                    selectedKey ? parseInt(selectedKey) : null,
                  );
                }}
              >
                {tunnels.map((tunnel) => (
                  <SelectItem
                    key={tunnel.id.toString()}
                    textValue={tunnel.name}
                  >
                    {tunnel.name}
                  </SelectItem>
                ))}
              </Select>

              {/* 输入区域 */}
              <Textarea
                classNames={{
                  input: "font-mono text-sm",
                }}
                label="导入数据"
                maxRows={12}
                minRows={8}
                placeholder={
                  importFormat === "flvx"
                    ? "请输入要导入的规则数据，格式：目标地址|规则名称|入口端口"
                    : '请输入ny格式数据，每行一个JSON对象，如：{"dest":["1.2.3.4:80"],"listen_port":8080,"name":"规则1"}；listen_port可省略自动分配'
                }
                value={importData}
                variant="flat"
                onChange={(e) => setImportData(e.target.value)}
              />

              {/* 导入结果 */}
              {importResults.length > 0 && (
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <h3 className="text-base font-semibold">导入结果</h3>
                    <span className="text-xs text-default-500">
                      成功：{importResults.filter((r) => r.success).length} /
                      总计：{importResults.length}
                    </span>
                  </div>

                  <div
                    className="max-h-40 overflow-y-auto space-y-1"
                    style={{
                      scrollbarWidth: "thin",
                      scrollbarColor: "rgb(156 163 175) transparent",
                    }}
                  >
                    {importResults.map((result, index) => (
                      <div
                        key={index}
                        className={`p-2 rounded border ${
                          result.success
                            ? "bg-success-50 dark:bg-success-100/10 border-success-200 dark:border-success-300/20"
                            : "bg-danger-50 dark:bg-danger-100/10 border-danger-200 dark:border-danger-300/20"
                        }`}
                      >
                        <div className="flex items-center gap-2">
                          {result.success ? (
                            <svg
                              aria-hidden="true"
                              className="w-3 h-3 text-success-600 flex-shrink-0"
                              fill="currentColor"
                              viewBox="0 0 20 20"
                            >
                              <path
                                clipRule="evenodd"
                                d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                                fillRule="evenodd"
                              />
                            </svg>
                          ) : (
                            <svg
                              aria-hidden="true"
                              className="w-3 h-3 text-danger-600 flex-shrink-0"
                              fill="currentColor"
                              viewBox="0 0 20 20"
                            >
                              <path
                                clipRule="evenodd"
                                d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
                                fillRule="evenodd"
                              />
                            </svg>
                          )}
                          <div className="flex-1 min-w-0">
                            <div className="flex items-center gap-2 mb-0.5">
                              <span
                                className={`text-xs font-medium ${
                                  result.success
                                    ? "text-success-700 dark:text-success-300"
                                    : "text-danger-700 dark:text-danger-300"
                                }`}
                              >
                                {result.success ? "成功" : "失败"}
                              </span>
                              <span className="text-xs text-default-500">
                                |
                              </span>
                              <code className="text-xs font-mono text-default-600 truncate">
                                {result.line}
                              </code>
                            </div>
                            <div
                              className={`text-xs ${
                                result.success
                                  ? "text-success-600 dark:text-success-400"
                                  : "text-danger-600 dark:text-danger-400"
                              }`}
                            >
                              {result.message}
                            </div>
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={() => setImportModalOpen(false)}>
              关闭
            </Button>
            <Button
              color="warning"
              isDisabled={!importData.trim() || !selectedTunnelForImport}
              isLoading={importLoading}
              onPress={executeImport}
            >
              开始导入
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* 诊断结果模态框 */}
      <Modal
        backdrop="blur"
        classNames={{
          base: "rounded-2xl",
          header: "rounded-t-2xl",
          body: "rounded-none",
          footer: "rounded-b-2xl",
        }}
        isOpen={diagnosisModalOpen}
        placement="center"
        scrollBehavior="inside"
        size="4xl"
        onOpenChange={(open) => {
          setDiagnosisModalOpen(open);
          if (!open) {
            diagnosisAbortRef.current?.abort();
            diagnosisAbortRef.current = null;
            setDiagnosisLoading(false);
          }
        }}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1 bg-content1 border-b border-divider">
                <h2 className="text-xl font-bold">规则诊断结果</h2>
                {currentDiagnosisForward && (
                  <div className="flex items-center gap-2 min-w-0">
                    <span className="text-small text-default-500 truncate flex-1 min-w-0">
                      {currentDiagnosisForward.name}
                    </span>
                    <Chip
                      className="flex-shrink-0"
                      color="primary"
                      size="sm"
                      variant="flat"
                    >
                      规则服务
                    </Chip>
                  </div>
                )}
              </ModalHeader>
              <ModalBody className="bg-content1">
                {diagnosisResult ? (
                  <div className="space-y-4">
                    {diagnosisLoading && (
                      <div className="flex items-center justify-between rounded-lg border border-primary/20 bg-primary/5 px-3 py-2">
                        <div className="flex items-center gap-2 text-sm text-primary">
                          <Spinner size="sm" />
                          <span>
                            正在诊断 {diagnosisProgress.completed}/
                            {diagnosisProgress.total > 0
                              ? diagnosisProgress.total
                              : "?"}
                          </span>
                        </div>
                        <Chip color="primary" size="sm" variant="flat">
                          流式更新中
                        </Chip>
                      </div>
                    )}

                    {diagnosisProgress.timedOut && (
                      <Alert
                        color="warning"
                        description="诊断超时（单条30秒 / 整体2分钟），以下为当前已完成结果。"
                        title="诊断超时"
                        variant="flat"
                      />
                    )}

                    {/* 统计摘要 */}
                    <div className="grid grid-cols-3 gap-3">
                      <div className="text-center p-3 bg-default-100 dark:bg-gray-800 rounded-lg border border-divider">
                        <div className="text-2xl font-bold text-foreground">
                          {diagnosisProgress.total > 0
                            ? diagnosisProgress.total
                            : diagnosisResult.results.length}
                        </div>
                        <div className="text-xs text-default-500 mt-1">
                          总测试数
                        </div>
                      </div>
                      <div className="text-center p-3 bg-success-50 dark:bg-success-900/20 rounded-lg border border-success-200 dark:border-success-700">
                        <div className="text-2xl font-bold text-success-600 dark:text-success-400">
                          {diagnosisProgress.completed > 0 ||
                          diagnosisProgress.total > 0
                            ? diagnosisProgress.success
                            : diagnosisResult.results.filter((r) => r.success)
                                .length}
                        </div>
                        <div className="text-xs text-success-600 dark:text-success-400/80 mt-1">
                          成功
                        </div>
                      </div>
                      <div className="text-center p-3 bg-danger-50 dark:bg-danger-900/20 rounded-lg border border-danger-200 dark:border-danger-700">
                        <div className="text-2xl font-bold text-danger-600 dark:text-danger-400">
                          {diagnosisProgress.completed > 0 ||
                          diagnosisProgress.total > 0
                            ? diagnosisProgress.failed
                            : diagnosisResult.results.filter((r) => !r.success)
                                .length}
                        </div>
                        <div className="text-xs text-danger-600 dark:text-danger-400/80 mt-1">
                          失败
                        </div>
                      </div>
                    </div>

                    {/* 桌面端表格展示 */}
                    <div className="hidden md:block space-y-3">
                      {(() => {
                        // 使用后端返回的 chainType 和 inx 字段进行分组
                        const groupedResults = {
                          entry: diagnosisResult.results.filter(
                            (r) => r.fromChainType === 1,
                          ),
                          chains: {} as Record<
                            number,
                            typeof diagnosisResult.results
                          >,
                          exit: diagnosisResult.results.filter(
                            (r) => r.fromChainType === 3,
                          ),
                        };

                        // 按 inx 分组链路测试
                        diagnosisResult.results.forEach((r) => {
                          if (r.fromChainType === 2 && r.fromInx != null) {
                            if (!groupedResults.chains[r.fromInx]) {
                              groupedResults.chains[r.fromInx] = [];
                            }
                            groupedResults.chains[r.fromInx].push(r);
                          }
                        });

                        const renderTableSection = (
                          title: string,
                          results: typeof diagnosisResult.results,
                        ) => {
                          if (results.length === 0) return null;

                          return (
                            <div
                              key={title}
                              className="border border-divider rounded-lg overflow-hidden bg-white dark:bg-gray-800"
                            >
                              <div className="bg-primary/10 dark:bg-primary/20 px-3 py-2 border-b border-divider">
                                <h3 className="text-sm font-semibold text-primary">
                                  {title}
                                </h3>
                              </div>
                              <table className="w-full text-sm">
                                <thead className="bg-default-100 dark:bg-gray-700">
                                  <tr>
                                    <th className="px-3 py-2 text-left font-semibold text-xs">
                                      路径
                                    </th>
                                    <th className="px-3 py-2 text-center font-semibold text-xs w-20">
                                      状态
                                    </th>
                                    <th className="px-3 py-2 text-center font-semibold text-xs w-24">
                                      延迟(ms)
                                    </th>
                                    <th className="px-3 py-2 text-center font-semibold text-xs w-24">
                                      丢包率
                                    </th>
                                    <th className="px-3 py-2 text-center font-semibold text-xs w-20">
                                      质量
                                    </th>
                                  </tr>
                                </thead>
                                <tbody className="divide-y divide-divider bg-white dark:bg-gray-800">
                                  {results.map((result, index) => {
                                    const isDiagnosing = Boolean(
                                      result.diagnosing,
                                    );
                                    const isSuccess = result.success === true;
                                    const quality =
                                      getForwardDiagnosisQualityDisplay(
                                        result.averageTime,
                                        result.packetLoss,
                                      );

                                    return (
                                      <tr
                                        key={index}
                                        className={`hover:bg-default-50 dark:hover:bg-gray-700/50 ${
                                          isDiagnosing
                                            ? "bg-warning-50 dark:bg-warning-900/20"
                                            : isSuccess
                                              ? "bg-white dark:bg-gray-800"
                                              : "bg-danger-50 dark:bg-danger-900/30"
                                        }`}
                                      >
                                        <td className="px-3 py-2">
                                          <div className="flex items-center gap-2">
                                            {isDiagnosing ? (
                                              <Spinner size="sm" />
                                            ) : (
                                              <span
                                                className={`w-5 h-5 rounded-full flex items-center justify-center text-xs ${
                                                  isSuccess
                                                    ? "bg-success text-white"
                                                    : "bg-danger text-white"
                                                }`}
                                              >
                                                {isSuccess ? "✓" : "✗"}
                                              </span>
                                            )}
                                            <div className="flex-1 min-w-0">
                                              <div className="font-medium text-foreground truncate">
                                                {result.description}
                                              </div>
                                              <div className="text-xs text-default-500 truncate">
                                                {result.targetIp}:
                                                {result.targetPort}
                                              </div>
                                            </div>
                                          </div>
                                        </td>
                                        <td className="px-3 py-2 text-center">
                                          <Chip
                                            color={
                                              isDiagnosing
                                                ? "warning"
                                                : isSuccess
                                                  ? "success"
                                                  : "danger"
                                            }
                                            size="sm"
                                            variant="flat"
                                          >
                                            {isDiagnosing
                                              ? "诊断中"
                                              : isSuccess
                                                ? "成功"
                                                : "失败"}
                                          </Chip>
                                        </td>
                                        <td className="px-3 py-2 text-center">
                                          {isSuccess ? (
                                            <span className="font-semibold text-primary">
                                              {result.averageTime?.toFixed(0)}
                                            </span>
                                          ) : (
                                            <span className="text-default-400">
                                              -
                                            </span>
                                          )}
                                        </td>
                                        <td className="px-3 py-2 text-center">
                                          {isSuccess ? (
                                            <span
                                              className={`font-semibold ${
                                                (result.packetLoss || 0) > 0
                                                  ? "text-warning"
                                                  : "text-success"
                                              }`}
                                            >
                                              {result.packetLoss?.toFixed(1)}%
                                            </span>
                                          ) : (
                                            <span className="text-default-400">
                                              -
                                            </span>
                                          )}
                                        </td>
                                        <td className="px-3 py-2 text-center">
                                          {isSuccess && quality ? (
                                            <Chip
                                              className="text-xs whitespace-nowrap"
                                              color={quality.color as any}
                                              size="sm"
                                              variant="flat"
                                            >
                                              {quality.text}
                                            </Chip>
                                          ) : (
                                            <span className="text-default-400">
                                              -
                                            </span>
                                          )}
                                        </td>
                                      </tr>
                                    );
                                  })}
                                </tbody>
                              </table>
                            </div>
                          );
                        };

                        return (
                          <>
                            {/* 入口测试 */}
                            {renderTableSection(
                              "🚪 入口测试",
                              groupedResults.entry,
                            )}

                            {/* 链路测试（按跳数排序） */}
                            {Object.keys(groupedResults.chains)
                              .map(Number)
                              .sort((a, b) => a - b)
                              .map((hop) =>
                                renderTableSection(
                                  `🔗 转发链 - 第${hop}跳`,
                                  groupedResults.chains[hop],
                                ),
                              )}

                            {/* 出口测试 */}
                            {renderTableSection(
                              "🚀 出口测试",
                              groupedResults.exit,
                            )}
                          </>
                        );
                      })()}
                    </div>

                    {/* 移动端卡片展示 */}
                    <div className="md:hidden space-y-3">
                      {(() => {
                        // 使用后端返回的 chainType 和 inx 字段进行分组
                        const groupedResults = {
                          entry: diagnosisResult.results.filter(
                            (r) => r.fromChainType === 1,
                          ),
                          chains: {} as Record<
                            number,
                            typeof diagnosisResult.results
                          >,
                          exit: diagnosisResult.results.filter(
                            (r) => r.fromChainType === 3,
                          ),
                        };

                        // 按 inx 分组链路测试
                        diagnosisResult.results.forEach((r) => {
                          if (r.fromChainType === 2 && r.fromInx != null) {
                            if (!groupedResults.chains[r.fromInx]) {
                              groupedResults.chains[r.fromInx] = [];
                            }
                            groupedResults.chains[r.fromInx].push(r);
                          }
                        });

                        const renderCardSection = (
                          title: string,
                          results: typeof diagnosisResult.results,
                        ) => {
                          if (results.length === 0) return null;

                          return (
                            <div key={title} className="space-y-2">
                              <div className="px-2 py-1.5 bg-primary/10 dark:bg-primary/20 rounded-lg border border-primary/30">
                                <h3 className="text-sm font-semibold text-primary">
                                  {title}
                                </h3>
                              </div>
                              {results.map((result, index) => {
                                const isDiagnosing = Boolean(result.diagnosing);
                                const isSuccess = result.success === true;
                                const quality =
                                  getForwardDiagnosisQualityDisplay(
                                    result.averageTime,
                                    result.packetLoss,
                                  );

                                return (
                                  <div
                                    key={index}
                                    className={`border rounded-lg p-3 ${
                                      isDiagnosing
                                        ? "border-warning-200 dark:border-warning-300/30 bg-warning-50 dark:bg-warning-900/20"
                                        : isSuccess
                                          ? "border-divider bg-white dark:bg-gray-800"
                                          : "border-danger-200 dark:border-danger-300/30 bg-danger-50 dark:bg-danger-900/30"
                                    }`}
                                  >
                                    <div className="flex items-start gap-2 mb-2">
                                      {isDiagnosing ? (
                                        <Spinner size="sm" />
                                      ) : (
                                        <span
                                          className={`w-6 h-6 rounded-full flex items-center justify-center text-xs flex-shrink-0 ${
                                            isSuccess
                                              ? "bg-success text-white"
                                              : "bg-danger text-white"
                                          }`}
                                        >
                                          {isSuccess ? "✓" : "✗"}
                                        </span>
                                      )}
                                      <div className="flex-1 min-w-0">
                                        <div className="font-semibold text-sm text-foreground break-words">
                                          {result.description}
                                        </div>
                                        <div className="text-xs text-default-500 mt-0.5 break-all">
                                          {result.targetIp}:{result.targetPort}
                                        </div>
                                      </div>
                                      <Chip
                                        className="flex-shrink-0"
                                        color={
                                          isDiagnosing
                                            ? "warning"
                                            : isSuccess
                                              ? "success"
                                              : "danger"
                                        }
                                        size="sm"
                                        variant="flat"
                                      >
                                        {isDiagnosing
                                          ? "诊断中"
                                          : isSuccess
                                            ? "成功"
                                            : "失败"}
                                      </Chip>
                                    </div>

                                    {isSuccess ? (
                                      <div className="grid grid-cols-3 gap-2 mt-2 pt-2 border-t border-divider">
                                        <div className="text-center">
                                          <div className="text-lg font-bold text-primary">
                                            {result.averageTime?.toFixed(0)}
                                          </div>
                                          <div className="text-xs text-default-500">
                                            延迟(ms)
                                          </div>
                                        </div>
                                        <div className="text-center">
                                          <div
                                            className={`text-lg font-bold ${
                                              (result.packetLoss || 0) > 0
                                                ? "text-warning"
                                                : "text-success"
                                            }`}
                                          >
                                            {result.packetLoss?.toFixed(1)}%
                                          </div>
                                          <div className="text-xs text-default-500">
                                            丢包率
                                          </div>
                                        </div>
                                        <div className="text-center">
                                          {quality && (
                                            <>
                                              <Chip
                                                className="text-xs whitespace-nowrap"
                                                color={quality.color as any}
                                                size="sm"
                                                variant="flat"
                                              >
                                                {quality.text}
                                              </Chip>
                                              <div className="text-xs text-default-500 mt-0.5">
                                                质量
                                              </div>
                                            </>
                                          )}
                                        </div>
                                      </div>
                                    ) : (
                                      <div className="mt-2 pt-2 border-t border-divider">
                                        <div
                                          className={`text-xs ${
                                            isDiagnosing
                                              ? "text-warning"
                                              : "text-danger"
                                          }`}
                                        >
                                          {isDiagnosing
                                            ? result.message || "诊断中..."
                                            : result.message || "连接失败"}
                                        </div>
                                      </div>
                                    )}
                                  </div>
                                );
                              })}
                            </div>
                          );
                        };

                        return (
                          <>
                            {/* 入口测试 */}
                            {renderCardSection(
                              "🚪 入口测试",
                              groupedResults.entry,
                            )}

                            {/* 链路测试（按跳数排序） */}
                            {Object.keys(groupedResults.chains)
                              .map(Number)
                              .sort((a, b) => a - b)
                              .map((hop) =>
                                renderCardSection(
                                  `🔗 转发链 - 第${hop}跳`,
                                  groupedResults.chains[hop],
                                ),
                              )}

                            {/* 出口测试 */}
                            {renderCardSection(
                              "🚀 出口测试",
                              groupedResults.exit,
                            )}
                          </>
                        );
                      })()}
                    </div>

                    {/* 失败详情（仅桌面端显示，移动端已在卡片中显示） */}
                    {diagnosisResult.results.some(
                      (r) => r.success === false && !r.diagnosing,
                    ) && (
                      <div className="space-y-2 hidden md:block">
                        <h4 className="text-sm font-semibold text-danger">
                          失败详情
                        </h4>
                        <div className="space-y-2">
                          {diagnosisResult.results
                            .filter((r) => r.success === false && !r.diagnosing)
                            .map((result, index) => (
                              <Alert
                                key={index}
                                className="text-xs"
                                color="danger"
                                description={result.message || "连接失败"}
                                title={result.description}
                                variant="flat"
                              />
                            ))}
                        </div>
                      </div>
                    )}
                  </div>
                ) : (
                  <div className="text-center py-16">
                    <div className="w-16 h-16 bg-default-100 rounded-full flex items-center justify-center mx-auto mb-4">
                      <svg
                        aria-hidden="true"
                        className="w-8 h-8 text-default-400"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          d="M9.75 9.75l4.5 4.5m0-4.5l-4.5 4.5M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={1.5}
                        />
                      </svg>
                    </div>
                    <h3 className="text-lg font-semibold text-foreground">
                      暂无诊断数据
                    </h3>
                  </div>
                )}
              </ModalBody>
              <ModalFooter className="bg-content1 border-t border-divider">
                <Button variant="light" onPress={onClose}>
                  关闭
                </Button>
                {currentDiagnosisForward && (
                  <Button
                    color="primary"
                    isLoading={diagnosisLoading}
                    onPress={() => handleDiagnose(currentDiagnosisForward)}
                  >
                    重新诊断
                  </Button>
                )}
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* 批量删除确认模态框 */}
      <Modal
        isOpen={batchDeleteModalOpen}
        onOpenChange={setBatchDeleteModalOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>确认删除</ModalHeader>
              <ModalBody>
                <p>
                  确定要删除选中的 {selectedIds.size} 项规则吗？此操作不可撤销。
                </p>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color="danger"
                  isLoading={batchLoading}
                  onPress={handleBatchDelete}
                >
                  确认删除
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* 批量换隧道模态框 */}
      <Modal
        isOpen={batchChangeTunnelModalOpen}
        onOpenChange={setBatchChangeTunnelModalOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>更换隧道</ModalHeader>
              <ModalBody>
                <p className="mb-4">
                  将选中的 {selectedIds.size} 项规则迁移到新隧道：
                </p>
                <Select
                  label="目标隧道"
                  placeholder="请选择目标隧道"
                  selectedKeys={
                    batchTargetTunnelId ? [String(batchTargetTunnelId)] : []
                  }
                  onSelectionChange={(keys) => {
                    const selected = Array.from(keys)[0];

                    setBatchTargetTunnelId(selected ? Number(selected) : null);
                  }}
                >
                  {tunnels.map((tunnel) => (
                    <SelectItem key={String(tunnel.id)}>
                      {tunnel.name}
                    </SelectItem>
                  ))}
                </Select>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color="primary"
                  isDisabled={!batchTargetTunnelId}
                  isLoading={batchLoading}
                  onPress={handleBatchChangeTunnel}
                >
                  确认换隧道
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* 筛选模态框 */}
      <Modal
        isOpen={isFilterModalOpen}
        placement="center"
        size="md"
        onOpenChange={setIsFilterModalOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                筛选条件
              </ModalHeader>
              <ModalBody>
                <div className="flex flex-col gap-4 py-2">
                  <div className="flex flex-col gap-2">
                    <p className="text-sm font-medium">按用户筛选</p>
                    <Select
                      aria-label="筛选用户"
                      className="w-full"
                      selectedKeys={[filterUserId]}
                      variant="bordered"
                      onSelectionChange={(keys) => {
                        const key = Array.from(keys)[0] as string;

                        setFilterUserId(key || "all");
                      }}
                    >
                      <SelectItem key="all">全部用户</SelectItem>
                      {uniqueUsers.map((user) => (
                        <SelectItem key={user.id.toString()}>
                          {user.name}
                        </SelectItem>
                      ))}
                    </Select>
                  </div>
                  <div className="flex flex-col gap-2">
                    <p className="text-sm font-medium">按隧道筛选</p>
                    <Select
                      aria-label="筛选隧道"
                      className="w-full"
                      selectedKeys={[filterTunnelId]}
                      variant="bordered"
                      onSelectionChange={(keys) => {
                        const key = Array.from(keys)[0] as string;

                        setFilterTunnelId(key || "all");
                      }}
                    >
                      <SelectItem key="all">全部隧道</SelectItem>
                      {tunnels.map((tunnel) => (
                        <SelectItem key={tunnel.id.toString()}>
                          {tunnel.name}
                        </SelectItem>
                      ))}
                    </Select>
                  </div>
                </div>
              </ModalBody>
              <ModalFooter>
                <Button
                  color="default"
                  variant="flat"
                  onPress={() => {
                    resetFilterUserId();
                    resetFilterTunnelId();
                  }}
                >
                  重置
                </Button>
                <Button color="primary" onPress={onClose}>
                  完成
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </AnimatedPage>
  );
}
