import { useCallback, useEffect, useMemo, useState } from "react";
import toast from "react-hot-toast";

import {
  getRuntimeEngineProgress,
  getRuntimeEngineSettings,
  updateRuntimeEngineSettings,
} from "@/api";
import type {
  RuntimeEngine,
  RuntimeEngineSettingsApiData,
  RuntimeNodeProgressApiItem,
} from "@/api/types";
import { Alert } from "@/shadcn-bridge/heroui/alert";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Card, CardBody } from "@/shadcn-bridge/heroui/card";
import { Chip } from "@/shadcn-bridge/heroui/chip";
import { Spinner } from "@/shadcn-bridge/heroui/spinner";
import { isAdmin } from "@/utils/auth";

const POLL_INTERVAL_MS = 3000;

const ENGINE_LABELS: Record<RuntimeEngine, string> = {
  gost: "Gost",
  dash: "Dash",
};

const getSwitchStatusMeta = (status: RuntimeEngineSettingsApiData["switchStatus"]) => {
  switch (status) {
    case "switching":
      return { label: "切换中", color: "warning" as const };
    case "failed":
      return { label: "失败", color: "danger" as const };
    default:
      return { label: "空闲", color: "success" as const };
  }
};

const getProgressMeta = (status: RuntimeNodeProgressApiItem["progress"]) => {
  switch (status) {
    case "running":
      return { label: "处理中", color: "primary" as const };
    case "pending":
      return { label: "待处理", color: "warning" as const };
    case "failed":
      return { label: "失败", color: "danger" as const };
    case "switching":
      return { label: "切换中", color: "warning" as const };
    default:
      return { label: "已完成", color: "success" as const };
  }
};

const getPhaseLabel = (phase?: string) => {
  switch (phase) {
    case "deploy_nodes":
      return "部署节点";
    case "rebuild_runtime":
      return "重建运行时";
    case "persist_runtime_engine":
      return "写入目标内核";
    case "completed":
      return "已完成";
    case "failed":
      return "失败";
    default:
      return phase || "未知阶段";
  }
};

const isProgressActive = (settings: RuntimeEngineSettingsApiData | null) => {
  if (!settings) {
    return false;
  }

  if (settings.switchStatus === "switching") {
    return true;
  }

  return ["pending", "running", "switching"].includes(
    settings.runtimeProgress.state,
  );
};

const shouldShowNodeDetails = (settings: RuntimeEngineSettingsApiData | null) => {
  if (!settings || settings.nodes.length === 0) {
    return false;
  }

  return settings.nodes.some((node) => node.progress !== "succeeded");
};

export const RuntimeEngineCard = () => {
  const admin = isAdmin();
  const [settings, setSettings] = useState<RuntimeEngineSettingsApiData | null>(
    null,
  );
  const [loading, setLoading] = useState(true);
  const [savingEngine, setSavingEngine] = useState<RuntimeEngine | null>(null);
  const [requestError, setRequestError] = useState("");

  const applySettings = useCallback(
    (nextSettings: RuntimeEngineSettingsApiData | null, errorMessage?: string) => {
      if (nextSettings) {
        setSettings(nextSettings);
        setRequestError("");

        return true;
      }

      setRequestError(errorMessage || "加载运行时设置失败");

      return false;
    },
    [],
  );

  const markProgressRequestFailed = useCallback((message?: string) => {
    setRequestError(message || "加载运行时设置失败");
    setSettings((previous) => {
      if (!previous) {
        return previous;
      }
      return {
        ...previous,
        switchStatus: previous.switchStatus === "switching" ? "failed" : previous.switchStatus,
        runtimeProgress: {
          ...previous.runtimeProgress,
          state: "failed",
          message: message || previous.runtimeProgress.message || "刷新切换进度失败",
        },
      };
    });
  }, []);

  const loadSettings = useCallback(async () => {
    setLoading(true);

    try {
      const response = await getRuntimeEngineSettings();

      if (response.code === 0 && response.data) {
        applySettings(response.data, response.msg);
      } else {
        markProgressRequestFailed(response.msg || "加载运行时设置失败");
      }
    } finally {
      setLoading(false);
    }
  }, [applySettings, markProgressRequestFailed]);

  const loadProgress = useCallback(async () => {
    const response = await getRuntimeEngineProgress();
    if (response.code === 0 && response.data) {
      applySettings(response.data, response.msg);
      return;
    }
    markProgressRequestFailed(response.msg || "刷新切换进度失败");
  }, [applySettings, markProgressRequestFailed]);

  useEffect(() => {
    void loadSettings();
  }, [loadSettings]);

  const activeProgress = useMemo(() => isProgressActive(settings), [settings]);

  useEffect(() => {
    if (!activeProgress) {
      return;
    }

    void loadProgress();

    const timer = window.setInterval(() => {
      void loadProgress();
    }, POLL_INTERVAL_MS);

    return () => {
      window.clearInterval(timer);
    };
  }, [activeProgress, loadProgress]);

  const handleSwitch = async (engine: RuntimeEngine) => {
    if (!admin || !settings || savingEngine || activeProgress) {
      return;
    }

    if (settings.currentEngine === engine) {
      return;
    }

    const confirmed = window.confirm(
      `确定将全局运行时切换为 ${ENGINE_LABELS[engine]} 吗？切换期间节点会逐步同步到新引擎。`,
    );

    if (!confirmed) {
      return;
    }

    setSavingEngine(engine);
    try {
      const response = await updateRuntimeEngineSettings({ engine });

      if (applySettings(response.data, response.msg)) {
        toast.success(`已提交切换到 ${ENGINE_LABELS[engine]} 的请求`);
      } else {
        toast.error(response.msg || "切换运行时失败");
      }
    } finally {
      setSavingEngine(null);
    }
  };

  const controlsDisabled = !admin || loading || !!savingEngine || activeProgress;
  const switchStatus = settings ? getSwitchStatusMeta(settings.switchStatus) : null;
  const runtimeState = settings ? getProgressMeta(settings.runtimeProgress.state) : null;
  const showNodeDetails = shouldShowNodeDetails(settings);

  return (
    <Card>
      <CardBody className="p-6 space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <h2 className="text-lg font-medium text-gray-900 dark:text-white">
              运行时引擎
            </h2>
            <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
              管理全局运行时引擎，并查看节点同步进度。
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            {switchStatus ? (
              <Chip color={switchStatus.color} size="sm" variant="flat">
                状态 {switchStatus.label}
              </Chip>
            ) : null}
            {runtimeState ? (
              <Chip color={runtimeState.color} size="sm" variant="flat">
                进度 {runtimeState.label}
              </Chip>
            ) : null}
          </div>
        </div>

        {loading && !settings ? (
          <div className="flex items-center gap-2 py-4 text-sm text-gray-500 dark:text-gray-400">
            <Spinner size="sm" />
            <span>加载运行时设置中...</span>
          </div>
        ) : null}

        {!loading && requestError && !settings ? (
          <Alert color="danger" description={requestError} title="加载运行时设置失败" />
        ) : null}

        {settings ? (
          <>
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              <div className="rounded-lg border border-gray-200 px-4 py-3 dark:border-gray-700">
                <p className="text-xs text-gray-500 dark:text-gray-400">当前引擎</p>
                <p className="mt-1 text-base font-medium text-gray-900 dark:text-white">
                  {ENGINE_LABELS[settings.currentEngine]}
                </p>
                <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  切换完成前，这里会保持显示当前生效的运行时。
                </p>
              </div>
              <div className="rounded-lg border border-gray-200 px-4 py-3 dark:border-gray-700">
                <p className="text-xs text-gray-500 dark:text-gray-400">节点同步概览</p>
                <div className="mt-2 flex flex-wrap gap-2">
                  <Chip color="primary" size="sm" variant="flat">
                    总计 {settings.nodeSummary.total}
                  </Chip>
                  <Chip color="success" size="sm" variant="flat">
                    就绪 {settings.nodeSummary.ready}
                  </Chip>
                  <Chip color="success" size="sm" variant="bordered">
                    已完成 {settings.nodeSummary.completed}
                  </Chip>
                  <Chip color="warning" size="sm" variant="flat">
                    待处理 {settings.nodeSummary.pending}
                  </Chip>
                  <Chip color="danger" size="sm" variant="flat">
                    失败 {settings.nodeSummary.failed}
                  </Chip>
                </div>
                <p className="mt-2 text-xs text-gray-500 dark:text-gray-400">
                  代次 #{settings.generation}
                </p>
              </div>
            </div>

            {settings.runtimeProgress.message ? (
              <Alert
                color={runtimeState?.color === "danger" ? "danger" : "warning"}
                description={`${settings.runtimeProgress.message}${settings.runtimeProgress.phase ? `（阶段：${getPhaseLabel(settings.runtimeProgress.phase)}）` : ""}`}
                title={`整体进度：${runtimeState?.label || "未知"}`}
              />
            ) : null}

            {settings.rebuildProgress?.message ? (
              <Alert
                color={getProgressMeta(settings.rebuildProgress.state).color === "danger" ? "danger" : "default"}
                description={`${settings.rebuildProgress.message}${settings.rebuildProgress.phase ? `（阶段：${getPhaseLabel(settings.rebuildProgress.phase)}）` : ""}`}
                title={`运行时重建：${getProgressMeta(settings.rebuildProgress.state).label}`}
              />
            ) : null}

            {settings.warnings?.length ? (
              <Alert
                color="warning"
                description={settings.warnings.join("；")}
                title={`运行时警告（${settings.warnings.length}）`}
              />
            ) : null}

            {settings.lastError ? (
              <Alert
                color="danger"
                description={settings.lastError}
                title="最近一次切换错误"
              />
            ) : null}

            {requestError && settings ? (
              <Alert
                color="danger"
                description={requestError}
                title="运行时状态刷新失败"
              />
            ) : null}

            <div className="space-y-3 rounded-lg border border-gray-200 px-4 py-4 dark:border-gray-700">
              <div>
                <p className="text-sm font-medium text-gray-900 dark:text-white">
                  切换引擎
                </p>
                <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  仅管理员可切换。切换过程中会自动轮询节点同步进度。
                </p>
              </div>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                {(["gost", "dash"] as RuntimeEngine[]).map((engine) => {
                  const selected = settings.currentEngine === engine;

                  return (
                    <Button
                      key={engine}
                      color={selected ? "primary" : "default"}
                      isDisabled={controlsDisabled || selected}
                      isLoading={savingEngine === engine}
                      variant={selected ? "solid" : "bordered"}
                      onPress={() => void handleSwitch(engine)}
                    >
                      {selected
                        ? `当前为 ${ENGINE_LABELS[engine]}`
                        : `切换到 ${ENGINE_LABELS[engine]}`}
                    </Button>
                  );
                })}
              </div>
              {!admin ? (
                <p className="text-xs text-amber-600 dark:text-amber-400">
                  仅管理员可修改该全局配置。
                </p>
              ) : null}
              {activeProgress ? (
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  当前正在同步节点状态，完成前暂不可再次提交切换。
                </p>
              ) : null}
            </div>

            {showNodeDetails ? (
              <div className="space-y-2">
                <p className="text-sm font-medium text-gray-900 dark:text-white">
                  节点进度
                </p>
                <div className="space-y-2">
                  {settings.nodes.map((node) => {
                    const progress = getProgressMeta(node.progress);

                    return (
                      <div
                        key={node.nodeId}
                        className="rounded-lg border border-gray-200 px-4 py-3 dark:border-gray-700"
                      >
                        <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                          <div>
                            <p className="text-sm font-medium text-gray-900 dark:text-white">
                              {node.nodeName}
                            </p>
                            <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                              运行时 {ENGINE_LABELS[node.engine]}
                              {node.ready ? "，已就绪" : "，未就绪"}
                            </p>
                          </div>
                          <Chip color={progress.color} size="sm" variant="flat">
                            {progress.label}
                          </Chip>
                        </div>
                        {node.message ? (
                          <p className="mt-2 text-xs text-gray-500 dark:text-gray-400">
                            {node.message}
                          </p>
                        ) : null}
                      </div>
                    );
                  })}
                </div>
              </div>
            ) : null}
          </>
        ) : null}
      </CardBody>
    </Card>
  );
};
