import { useEffect, useMemo, useState } from "react";
import toast from "react-hot-toast";
import {
  CheckCircle2,
  Clock,
  DownloadCloud,
  KeyRound,
  Loader2,
  RefreshCw,
  ShieldAlert,
  TerminalSquare,
} from "lucide-react";

import {
  activateLocalLicense,
  checkLocalLicenseUpdate,
  getLocalLicenseUpdateLog,
  getLocalLicenseStatus,
  heartbeatLocalLicense,
  runLocalLicenseUpdate,
  type LocalLicenseUpdateApiData,
  type LocalLicenseUpdateLogApiData,
  type LocalLicenseStatusApiData,
} from "@/api";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";

const stateText: Record<string, string> = {
  active: "已激活",
  grace: "宽限期",
  inactive: "未激活",
  invalid: "无效",
  expired: "已到期",
  expired_grace: "宽限期已过",
  disabled: "已停用",
};

const featureNameMap: Record<string, string> = {
  onlineUpdate: "在线更新",
  install: "远程安装",
  commercial: "商业能力",
  monitoring: "监控功能",
  payment: "支付功能",
  maxActivations: "可激活实例数",
};

function formatTime(ms?: number) {
  if (!ms) return "永久";

  return new Date(ms).toLocaleString("zh-CN", { hour12: false });
}

function statusVariant(status?: LocalLicenseStatusApiData | null) {
  if (!status?.configured) return "warning";
  if (status.valid && status.state === "active") return "success";
  if (status.valid && status.state === "grace") return "warning";

  return "destructive";
}

export default function LicensePage() {
  const [status, setStatus] = useState<LocalLicenseStatusApiData | null>(null);
  const [loading, setLoading] = useState(false);
  const [activating, setActivating] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [checkingUpdate, setCheckingUpdate] = useState(false);
  const [runningUpdate, setRunningUpdate] = useState(false);
  const [loadingLog, setLoadingLog] = useState(false);
  const [updateInfo, setUpdateInfo] =
    useState<LocalLicenseUpdateApiData | null>(null);
  const [updateLog, setUpdateLog] =
    useState<LocalLicenseUpdateLogApiData | null>(null);
  const [form, setForm] = useState({
    centerUrl: "",
    publicKey: "",
    licenseKey: "",
  });

  const readableState = useMemo(() => {
    if (!status?.configured) return "未激活";

    return stateText[status.state] || status.state || "未知";
  }, [status]);

  const featureEntries = useMemo(() => {
    const features = status?.features || {};

    return Object.entries(features).map(([key, value]) => ({
      name: featureNameMap[key] || key,
      value:
        typeof value === "boolean"
          ? value
            ? "已开放"
            : "未开放"
          : String(value),
    }));
  }, [status]);

  const loadStatus = async () => {
    setLoading(true);
    try {
      const res = await getLocalLicenseStatus();

      if (res.code === 0 && res.data) {
        setStatus(res.data);
        setForm((prev) => ({
          ...prev,
          centerUrl: prev.centerUrl || res.data.centerUrl || "",
        }));
      } else {
        toast.error(res.msg || "读取授权状态失败");
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadStatus();
  }, []);

  useEffect(() => {
    if (!runningUpdate) return;

    void handleLoadUpdateLog(true);
    const timer = window.setInterval(() => {
      void handleLoadUpdateLog(true);
    }, 1500);

    return () => window.clearInterval(timer);
  }, [runningUpdate]);

  const handleActivate = async () => {
    if (
      !form.centerUrl.trim() ||
      !form.publicKey.trim() ||
      !form.licenseKey.trim()
    ) {
      toast.error("请填写授权中心地址、官方公钥和授权码");

      return;
    }
    setActivating(true);
    try {
      const res = await activateLocalLicense({
        centerUrl: form.centerUrl.trim(),
        publicKey: form.publicKey.trim(),
        licenseKey: form.licenseKey.trim(),
      });

      if (res.code === 0 && res.data) {
        setStatus(res.data);
        setForm((prev) => ({ ...prev, licenseKey: "" }));
        toast.success("授权激活成功");
      } else {
        toast.error(res.msg || "授权激活失败");
      }
    } finally {
      setActivating(false);
    }
  };

  const handleHeartbeat = async () => {
    setSyncing(true);
    try {
      const res = await heartbeatLocalLicense();

      if (res.code === 0 && res.data) {
        setStatus(res.data);
        toast.success("授权状态已同步");
      } else {
        toast.error(res.msg || "同步授权状态失败");
      }
    } finally {
      setSyncing(false);
    }
  };

  const handleCheckUpdate = async () => {
    setCheckingUpdate(true);
    try {
      const res = await checkLocalLicenseUpdate();

      if (res.code === 0 && res.data) {
        setUpdateInfo(res.data);
        toast.success(res.data.hasUpdate ? "发现可用版本" : "当前已是最新版本");
      } else {
        toast.error(res.msg || "检查更新失败");
      }
    } finally {
      setCheckingUpdate(false);
    }
  };

  const handleRunUpdate = async () => {
    setRunningUpdate(true);
    setUpdateLog({
      log: `[${new Date().toLocaleString("zh-CN", { hour12: false })}] 正在启动在线更新，请勿关闭页面。\n`,
      deployDir: "",
      logPath: "",
    });
    try {
      const res = await runLocalLicenseUpdate({
        version: updateInfo?.latestVersion,
        channel: updateInfo?.channel,
      });

      if (res.code === 0 && res.data) {
        toast.success(res.data.message || "升级任务已启动");
        void handleLoadUpdateLog();
      } else {
      toast.error(res.msg || "启动更新失败");
      }
    } finally {
      void handleLoadUpdateLog(true);
      setRunningUpdate(false);
    }
  };

  const handleLoadUpdateLog = async (silent = false) => {
    if (!silent) setLoadingLog(true);
    try {
      const res = await getLocalLicenseUpdateLog();

      if (res.code === 0 && res.data) {
        setUpdateLog(res.data);
      } else if (!silent) {
        toast.error(res.msg || "读取升级日志失败");
      }
    } finally {
      if (!silent) setLoadingLog(false);
    }
  };

  return (
    <div className="min-h-full overflow-y-auto px-2 py-2">
      <div className="mx-auto flex w-full max-w-[1280px] flex-col gap-5">
        <header className="flex flex-col gap-2">
          <h1 className="text-3xl font-bold text-foreground">商业授权</h1>
          <p className="text-sm text-default-500">
            绑定官方授权中心并激活当前商业 v1
            实例。授权失效后，核心业务功能会被限制。
          </p>
        </header>

        <Alert variant={statusVariant(status)}>
          <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
            <div className="flex gap-3">
              {status?.valid ? (
                <CheckCircle2 className="mt-0.5 h-5 w-5 flex-shrink-0" />
              ) : (
                <ShieldAlert className="mt-0.5 h-5 w-5 flex-shrink-0" />
              )}
              <div>
                <AlertTitle>授权状态：{readableState}</AlertTitle>
                <AlertDescription>
                  {status?.reason ||
                    status?.lastError ||
                    "当前实例授权状态正常。"}
                </AlertDescription>
              </div>
            </div>
            <div className="flex gap-2">
              <Button
                className="gap-2"
                disabled={loading}
                variant="outline"
                onClick={loadStatus}
              >
                {loading ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <RefreshCw className="h-4 w-4" />
                )}
                刷新
              </Button>
              <Button
                className="gap-2"
                disabled={syncing || !status?.configured}
                onClick={handleHeartbeat}
              >
                {syncing ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Clock className="h-4 w-4" />
                )}
                同步心跳
              </Button>
            </div>
          </div>
        </Alert>

        <section className="grid gap-4 lg:grid-cols-4">
          <div className="native-panel p-5">
            <p className="text-sm text-default-500">授权前缀</p>
            <p className="mt-2 font-mono text-lg font-semibold">
              {status?.keyPrefix || "-"}
            </p>
          </div>
          <div className="native-panel p-5">
            <p className="text-sm text-default-500">实例 ID</p>
            <p className="mt-2 break-all font-mono text-sm">
              {status?.instanceId || "-"}
            </p>
          </div>
          <div className="native-panel p-5">
            <p className="text-sm text-default-500">下次心跳</p>
            <p className="mt-2 text-sm font-semibold">
              {formatTime(status?.nextHeartbeatAt)}
            </p>
          </div>
          <div className="native-panel p-5">
            <p className="text-sm text-default-500">宽限期截止</p>
            <p className="mt-2 text-sm font-semibold">
              {formatTime(status?.graceUntil)}
            </p>
          </div>
        </section>

        <section className="native-panel p-5">
          <div className="mb-4">
            <h2 className="text-lg font-semibold">激活授权</h2>
            <p className="mt-1 text-sm text-default-500">
              从官方授权中心复制公钥，并输入购买获得的授权码。授权码不会在前端再次展示。
            </p>
          </div>
          <div className="grid gap-4 lg:grid-cols-2">
            <div className="space-y-1">
              <label
                className="text-sm font-medium"
                htmlFor="license-center-url"
              >
                授权中心地址
              </label>
              <Input
                id="license-center-url"
                placeholder="https://official.example.com"
                value={form.centerUrl}
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, centerUrl: e.target.value }))
                }
              />
            </div>
            <div className="space-y-1">
              <label className="text-sm font-medium" htmlFor="license-key">
                授权码
              </label>
              <Input
                id="license-key"
                placeholder="FLVX-xxxxx-xxxxx-xxxxx"
                value={form.licenseKey}
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, licenseKey: e.target.value }))
                }
              />
            </div>
            <div className="space-y-1 lg:col-span-2">
              <label
                className="text-sm font-medium"
                htmlFor="license-public-key"
              >
                官方公钥
              </label>
              <Textarea
                className="min-h-[120px] font-mono"
                id="license-public-key"
                placeholder="从官方授权中心复制的 Ed25519 公钥"
                value={form.publicKey}
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, publicKey: e.target.value }))
                }
              />
            </div>
          </div>
          <div className="mt-4 flex justify-end">
            <Button
              className="gap-2"
              disabled={activating}
              onClick={handleActivate}
            >
              {activating ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <KeyRound className="h-4 w-4" />
              )}
              激活授权
            </Button>
          </div>
        </section>

        <section className="native-panel p-5">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <h2 className="text-lg font-semibold">在线更新</h2>
              <p className="mt-1 text-sm text-default-500">
                从官方授权中心获取签名版本清单，校验通过后下载并替换商业包部署文件。
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                className="gap-2"
                disabled={checkingUpdate || !status?.valid}
                variant="outline"
                onClick={handleCheckUpdate}
              >
                {checkingUpdate ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <RefreshCw className="h-4 w-4" />
                )}
                检查更新
              </Button>
              <Button
                className="gap-2"
                disabled={
                  runningUpdate ||
                  !status?.valid ||
                  !updateInfo?.hasUpdate ||
                  updateInfo?.capability?.capable === false
                }
                onClick={handleRunUpdate}
              >
                {runningUpdate ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <DownloadCloud className="h-4 w-4" />
                )}
                立即更新
              </Button>
              <Button
                className="gap-2"
                disabled={loadingLog}
                variant="outline"
                onClick={() => handleLoadUpdateLog(false)}
              >
                {loadingLog ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <TerminalSquare className="h-4 w-4" />
                )}
                查看日志
              </Button>
            </div>
          </div>

          <div className="mt-4 grid gap-3 lg:grid-cols-4">
            <div className="rounded-xl bg-default-100 p-4">
              <p className="text-xs text-default-500">当前版本</p>
              <p className="mt-1 font-mono text-sm font-semibold">
                {updateInfo?.currentVersion || "-"}
              </p>
            </div>
            <div className="rounded-xl bg-default-100 p-4">
              <p className="text-xs text-default-500">最新版本</p>
              <p className="mt-1 font-mono text-sm font-semibold">
                {updateInfo?.latestVersion || "-"}
              </p>
            </div>
            <div className="rounded-xl bg-default-100 p-4">
              <p className="text-xs text-default-500">发布通道</p>
              <p className="mt-1 text-sm font-semibold">
                {updateInfo?.channel || status?.versionChannel || "-"}
              </p>
            </div>
            <div className="rounded-xl bg-default-100 p-4">
              <p className="text-xs text-default-500">执行环境</p>
              <p className="mt-1 text-sm font-semibold">
                {updateInfo?.capability?.capable === false
                  ? "不可更新"
                  : updateInfo
                    ? "可更新"
                    : "-"}
              </p>
            </div>
          </div>

          {updateInfo?.releaseNotes && (
            <div className="mt-4 rounded-xl bg-default-100 p-4">
              <p className="text-sm font-medium">版本说明</p>
              <p className="mt-2 whitespace-pre-wrap text-sm text-default-600">
                {updateInfo.releaseNotes}
              </p>
            </div>
          )}

          {updateInfo?.capability?.capable === false && (
            <Alert className="mt-4" variant="destructive">
              <ShieldAlert className="h-4 w-4" />
              <AlertTitle>当前环境暂不能在线更新</AlertTitle>
              <AlertDescription>
                {updateInfo.capability.reasons.join("；")}
              </AlertDescription>
            </Alert>
          )}

          {(runningUpdate || updateLog?.log) && (
            <div className="mt-4 overflow-hidden rounded-xl border border-zinc-800 bg-zinc-950">
              <div className="flex items-center justify-between border-b border-zinc-800 px-4 py-3">
                <div>
                  <p className="text-sm font-semibold text-zinc-100">
                    更新过程
                  </p>
                  <p className="mt-1 text-xs text-zinc-400">
                    {runningUpdate
                      ? "正在执行，日志会自动刷新。"
                      : "最近一次在线更新日志。"}
                  </p>
                </div>
                {runningUpdate && (
                  <Loader2 className="h-4 w-4 animate-spin text-zinc-100" />
                )}
              </div>
              <pre className="max-h-[420px] min-h-[180px] overflow-auto p-4 text-xs leading-relaxed text-zinc-100">
                {updateLog?.log || "正在等待后端写入更新日志..."}
              </pre>
            </div>
          )}
        </section>

        <section className="native-panel p-5">
          <h2 className="text-lg font-semibold">授权能力</h2>
          {featureEntries.length > 0 ? (
            <div className="mt-3 grid gap-3 md:grid-cols-2 lg:grid-cols-3">
              {featureEntries.map((item) => (
                <div key={item.name} className="rounded-xl bg-default-100 p-4">
                  <p className="text-sm text-default-500">{item.name}</p>
                  <p className="mt-1 text-sm font-semibold">{item.value}</p>
                </div>
              ))}
            </div>
          ) : (
            <p className="mt-3 rounded-xl bg-default-100 p-4 text-sm text-default-500">
              当前授权暂未配置额外能力。
            </p>
          )}
        </section>
      </div>
    </div>
  );
}
