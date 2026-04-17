import { useState, useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { AnimatePresence, motion } from "framer-motion";
import toast from "react-hot-toast";

import { Button } from "@/shadcn-bridge/heroui/button";
import { Card, CardBody, CardHeader } from "@/shadcn-bridge/heroui/card";
import { Input } from "@/shadcn-bridge/heroui/input";
import { Textarea } from "@/shadcn-bridge/heroui/input";
import { Spinner } from "@/shadcn-bridge/heroui/spinner";
import { Divider } from "@/shadcn-bridge/heroui/divider";
import { Switch } from "@/shadcn-bridge/heroui/switch";
import { Select, SelectItem } from "@/shadcn-bridge/heroui/select";
import { Checkbox } from "@/shadcn-bridge/heroui/checkbox";
import {
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@/shadcn-bridge/heroui/modal";
import {
  updateConfigs,
  activateLicense,
  exportBackup,
  importBackup,
  getAnnouncement,
  updateAnnouncement,
  type AnnouncementData,
} from "@/api";
import { BackIcon, SettingsIcon } from "@/components/icons";
import { ThemeSettings } from "@/components/theme-settings";
import { isAdmin } from "@/utils/auth";
import { getCachedConfigs, configCache, updateSiteConfig } from "@/config/site";
import {
  type UpdateReleaseChannel,
  getUpdateReleaseChannel,
  setUpdateReleaseChannel,
} from "@/utils/version-update";
import {
  convertBrandAssetToPngDataURL,
  isPngDataURL,
  type BrandAssetKind,
} from "@/utils/brand-asset";

// 简单的保存图标组件
const SaveIcon = ({ className }: { className?: string }) => (
  <svg
    className={className}
    fill="none"
    stroke="currentColor"
    strokeLinecap="round"
    strokeLinejoin="round"
    strokeWidth="2"
    viewBox="0 0 24 24"
  >
    <path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z" />
    <polyline points="17,21 17,13 7,13 7,21" />
    <polyline points="7,3 7,8 15,8" />
  </svg>
);

interface ConfigItem {
  key: string;
  label: string;
  placeholder?: string;
  description?: string;
  type: "input" | "switch" | "select" | "bg_image";
  options?: { label: string; value: string; description?: string }[];
  dependsOn?: string; // 依赖的配置项key
  dependsValue?: string; // 依赖的配置项值
}

const BRAND_PREVIEW_KEYS = ["app_logo", "app_favicon"] as const;

type BrandPreviewKey = (typeof BRAND_PREVIEW_KEYS)[number];

const isBrandPreviewKey = (key: string): key is BrandPreviewKey =>
  BRAND_PREVIEW_KEYS.includes(key as BrandPreviewKey);

const BRAND_FILE_ACCEPT = "image/png,image/jpeg,image/webp,image/svg+xml";

const toBrandAssetKind = (key: BrandPreviewKey): BrandAssetKind => {
  return key === "app_logo" ? "logo" : "favicon";
};

// 网站配置项定义
const CONFIG_ITEMS: ConfigItem[] = [
  {
    key: "app_bg_image",
    label: "自定义背景",
    description: "上传自定义背景图片（建议使用深色/浅色均可看清的图片，或使用半透明模糊效果）",
    type: "bg_image",
  },
  {
    key: "ip",
    label: "面板后端地址",
    placeholder: "请输入面板后端IP:PORT",
    description:
      '格式"ip:port"或"domain:port",用于对接节点时使用。支持套CDN和HTTPS,通讯数据有加密',
    type: "input",
  },
  {
    key: "panel_domain",
    label: "面板域名",
    placeholder: "请输入面板域名",
    description: "当前面板的域名，用于与其他面板进行联邦共享时验证身份",
    type: "input",
  },
  {
    key: "app_name",
    label: "应用名称",
    placeholder: "请输入应用名称",
    description: "在浏览器标签页和导航栏显示的应用名称",
    type: "input",
  },
  {
    key: "app_logo",
    label: "网页角标 Logo",
    description: "用于页面左上角导航角标，上传后会自动转换为 PNG 并持久化保存",
    type: "input",
  },
  {
    key: "app_favicon",
    label: "浏览器缩略图标",
    description: "用于浏览器标签页图标，上传后会自动转换为 PNG 并持久化保存",
    type: "input",
  },
  {
    key: "hide_footer_brand",
    label: "隐藏页面底部 FLVX 版权信息",
    description: "需商业版授权才能生效",
    type: "switch",
  },
  {
    key: "forward_compact_mode",
    label: "规则页面精简模式",
    description: "开启后，规则页面列表使用 2.1.6-alpha8 样式（全局配置）",
    type: "switch",
  },
  {
    key: "monitor_tunnel_quality_enabled",
    label: "实时隧道质量检测",
    description: "关闭后，前端停止自动刷新，后端停止实时隧道质量探测（全局配置）",
    type: "switch",
  },
  {
    key: "captcha_enabled",
    label: "启用验证码",
    description: "开启后，用户登录时需要完成验证码验证",
    type: "switch",
  },
  {
    key: "cloudflare_site_key",
    label: "Cloudflare Site Key",
    placeholder: "请输入 Cloudflare Site Key",
    description: "Cloudflare Turnstile 站点密钥",
    type: "input",
    dependsOn: "captcha_enabled",
    dependsValue: "true",
  },
  {
    key: "cloudflare_secret_key",
    label: "Cloudflare Secret Key",
    placeholder: "请输入 Cloudflare Secret Key",
    description: "Cloudflare Turnstile 密钥",
    type: "input",
    dependsOn: "captcha_enabled",
    dependsValue: "true",
  },
  {
    key: "github_proxy_enabled",
    label: "开启 GitHub 加速",
    description: "用于节点更新和安装脚本下载，解决部分地区 GitHub 访问受限问题",
    type: "switch",
  },
  {
    key: "github_proxy_url",
    label: "加速地址",
    placeholder: "https://gcode.hostcentral.cc",
    description: "GitHub 下载加速代理地址，开启加速后生效",
    type: "input",
    dependsOn: "github_proxy_enabled",
    dependsValue: "true",
  },
];

const BACKUP_TYPE_OPTIONS = [
  { value: "users", label: "用户" },
  { value: "nodes", label: "节点" },
  { value: "tunnels", label: "隧道" },
  { value: "forwards", label: "规则" },
  { value: "userTunnels", label: "用户隧道权限" },
  { value: "speedLimits", label: "限速规则" },
  { value: "tunnelGroups", label: "隧道分组" },
  { value: "userGroups", label: "用户分组" },
  { value: "permissions", label: "分组权限" },
  { value: "configs", label: "系统配置" },
] as const;

const BACKUP_TYPE_VALUES = BACKUP_TYPE_OPTIONS.map((option) => option.value);

// 初始化时从缓存读取配置，避免闪烁
const getInitialConfigs = (): Record<string, string> => {
  if (typeof window === "undefined") return {};

  const configKeys = [
    "app_name",
    "captcha_enabled",
    "cloudflare_site_key",
    "cloudflare_secret_key",
    "forward_compact_mode",
    "monitor_tunnel_quality_enabled",
    "ip",
    "panel_domain",
    "app_logo",
    "app_favicon",
    "github_proxy_enabled",
    "github_proxy_url",
  ];
  const initialConfigs: Record<string, string> = {};

  try {
    configKeys.forEach((key) => {
      const cachedValue = localStorage.getItem("vite_config_" + key);

      if (cachedValue) {
        initialConfigs[key] = cachedValue;
      }
    });
  } catch {}

  return initialConfigs;
};

export default function ConfigPage() {
  const navigate = useNavigate();
  const initialConfigs = getInitialConfigs();
  const [configs, setConfigs] =
    useState<Record<string, string>>(initialConfigs);
  const [loading, setLoading] = useState(
    Object.keys(initialConfigs).length === 0,
  );
  const [saving, setSaving] = useState(false);
  const [hasChanges, setHasChanges] = useState(false);
  const [originalConfigs, setOriginalConfigs] =
    useState<Record<string, string>>(initialConfigs);

  const [exportTypes, setExportTypes] = useState<string[]>([]);
  const [importTypes, setImportTypes] = useState<string[]>([]);
  const [exporting, setExporting] = useState(false);
  const [importing, setImporting] = useState(false);
  const [exportSelectorOpen, setExportSelectorOpen] = useState(false);
  const [importSelectorOpen, setImportSelectorOpen] = useState(false);
  const [importFileName, setImportFileName] = useState("");
  const backupFileInputRef = useRef<HTMLInputElement>(null);

  const [activatingLicense, setActivatingLicense] = useState(false);
  const [licenseKeyInput, setLicenseKeyInput] = useState("");

  const logoFileInputRef = useRef<HTMLInputElement>(null);
  const faviconFileInputRef = useRef<HTMLInputElement>(null);
  const bgImageFileInputRef = useRef<HTMLInputElement>(null);
  const [bgImageUploading, setBgImageUploading] = useState(false);

  const [announcement, setAnnouncement] = useState<AnnouncementData>({
    content: "",
    enabled: 0,
  });
  const [announcementLoading, setAnnouncementLoading] = useState(true);
  const [announcementSaving, setAnnouncementSaving] = useState(false);
  const [updateChannel, setUpdateChannel] = useState<UpdateReleaseChannel>(
    getUpdateReleaseChannel(),
  );
  const [previewLoadFailed, setPreviewLoadFailed] = useState<
    Partial<Record<BrandPreviewKey, boolean>>
  >({});
  const [brandUploading, setBrandUploading] = useState<
    Partial<Record<BrandPreviewKey, boolean>>
  >({});

  const canGoBack =
    typeof window !== "undefined" &&
    typeof window.history.state?.idx === "number" &&
    window.history.state.idx > 0;

  const handleBack = () => {
    if (canGoBack) {
      navigate(-1);

      return;
    }

    navigate("/profile", { replace: true });
  };

  // 权限检查
  useEffect(() => {
    if (!isAdmin()) {
      toast.error("权限不足，只有管理员可以访问此页面");
      navigate("/dashboard", { replace: true });

      return;
    }
  }, [navigate]);

  // 加载配置数据（优先从缓存）
  const loadConfigs = async (currentConfigs?: Record<string, string>) => {
    const configsToCompare = currentConfigs || configs;
    const hasInitialData = Object.keys(configsToCompare).length > 0;

    // 如果已有缓存数据，不显示loading，静默更新
    if (!hasInitialData) {
      setLoading(true);
    }

    try {
      const configData = await getCachedConfigs();

      // 只有在数据有变化时才更新
      const hasDataChanged =
        JSON.stringify(configData) !== JSON.stringify(configsToCompare);

      if (hasDataChanged) {
        setConfigs(configData);
        setOriginalConfigs({ ...configData });
        setHasChanges(false);
      } else {
      }
    } catch {
      // 只有在没有缓存数据时才显示错误
      if (!hasInitialData) {
        toast.error("加载配置出错，请重试");
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    const timer = setTimeout(() => {
      loadConfigs(initialConfigs);
      loadAnnouncement();
    }, 100);

    return () => clearTimeout(timer);
  }, []);

  const loadAnnouncement = async () => {
    setAnnouncementLoading(true);
    try {
      const res = await getAnnouncement();

      if (res.code === 0 && res.data) {
        setAnnouncement(res.data);
      }
    } catch {
    } finally {
      setAnnouncementLoading(false);
    }
  };

  const saveAnnouncement = async () => {
    setAnnouncementSaving(true);
    try {
      const res = await updateAnnouncement(announcement);

      if (res.code === 0) {
        toast.success("公告保存成功");
      } else {
        toast.error(res.msg || "保存失败");
      }
    } catch {
      toast.error("保存公告失败，请重试");
    } finally {
      setAnnouncementSaving(false);
    }
  };

  const handleUpdateChannelChange = (channel: UpdateReleaseChannel) => {
    setUpdateChannel(channel);
    setUpdateReleaseChannel(channel);
    toast.success(
      `更新通道已切换为${channel === "stable" ? "稳定版" : "开发版"}`,
    );
  };

  const handleActivateLicense = async () => {
    if (!licenseKeyInput.trim()) {
      toast.error("请输入有效的商业授权码");
      return;
    }
    setActivatingLicense(true);
    try {
      const res = await activateLicense(licenseKeyInput.trim());
      if (res.code === 0) {
        toast.success("商业版授权激活成功！");
        setLicenseKeyInput("");
        await loadConfigs();
        window.dispatchEvent(new CustomEvent("configUpdated"));
      } else {
        toast.error(res.msg || "授权激活失败");
      }
    } catch (e: any) {
      toast.error(e.message || "授权激活出错");
    } finally {
      setActivatingLicense(false);
    }
  };

  const handleConfigChange = (key: string, value: string) => {
    const newConfigs = { ...configs, [key]: value };

    setConfigs(newConfigs);

    if (isBrandPreviewKey(key)) {
      setPreviewLoadFailed((prev) => ({ ...prev, [key]: false }));
    }

    const hasChangesNow =
      Object.keys(newConfigs).some(
        (k) => newConfigs[k] !== originalConfigs[k],
      ) ||
      Object.keys(originalConfigs).some(
        (k) => originalConfigs[k] !== newConfigs[k],
      );

    setHasChanges(hasChangesNow);
  };

  // 保存配置
  const handleSave = async () => {
    setSaving(true);
    try {
      const changedKeys = Object.keys(configs).filter(
        (key) => configs[key] !== originalConfigs[key],
      );

      if (changedKeys.length === 0) {
        setHasChanges(false);

        return;
      }

      const changedPayload: Record<string, string> = {};

      changedKeys.forEach((key) => {
        changedPayload[key] = configs[key] || "";
      });

      const response = await updateConfigs(changedPayload);

      if (response.code === 0) {
        toast.success("配置保存成功");

        Object.entries(configs).forEach(([key, value]) => {
          configCache.set(key, value);
        });

        setOriginalConfigs({ ...configs });
        setHasChanges(false);

        if (
          changedKeys.some((key) =>
            ["app_name", "app_logo", "app_favicon"].includes(key),
          )
        ) {
          await updateSiteConfig(configs);
        }

        // 触发配置更新事件，通知其他组件
        window.dispatchEvent(
          new CustomEvent("configUpdated", {
            detail: { changedKeys },
          }),
        );

        // 如果隧道质量检测开关变更，通知 tunnel-monitor-view
        if (changedKeys.includes("monitor_tunnel_quality_enabled")) {
          window.dispatchEvent(
            new CustomEvent("monitorTunnelQualityEnabledChanged", {
              detail: { enabled: configs["monitor_tunnel_quality_enabled"] === "true" },
            }),
          );
        }
      } else {
        toast.error("保存配置失败: " + response.msg);
      }
    } catch {
      toast.error("保存配置出错，请重试");
    } finally {
      setSaving(false);
    }
  };

  // 检查配置项是否应该显示（依赖检查）
  const shouldShowItem = (item: ConfigItem): boolean => {
    // 隐藏商业版专属的设置项，它们会在专门的卡片中展示
    if (["app_name", "app_logo", "app_favicon", "hide_footer_brand"].includes(item.key)) {
      return false;
    }

    if (!item.dependsOn || !item.dependsValue) {
      return true;
    }

    return configs[item.dependsOn] === item.dependsValue;
  };

  const getBrandInputRef = (key: BrandPreviewKey) => {
    return key === "app_logo" ? logoFileInputRef : faviconFileInputRef;
  };

  const triggerBrandFilePicker = (key: BrandPreviewKey) => {
    if (brandUploading[key]) {
      return;
    }

    getBrandInputRef(key).current?.click();
  };

  const clearBrandAsset = (key: BrandPreviewKey) => {
    handleConfigChange(key, "");
    setPreviewLoadFailed((prev) => ({ ...prev, [key]: false }));
  };

  const handleBrandFileChange = async (
    key: BrandPreviewKey,
    event: React.ChangeEvent<HTMLInputElement>,
  ) => {
    const file = event.target.files?.[0];

    if (!file) {
      return;
    }

    setBrandUploading((prev) => ({ ...prev, [key]: true }));

    try {
      const pngDataURL = await convertBrandAssetToPngDataURL(
        file,
        toBrandAssetKind(key),
      );

      handleConfigChange(key, pngDataURL);
      toast.success(key === "app_logo" ? "Logo 上传成功" : "Favicon 上传成功");
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "图片处理失败，请重试";

      toast.error(message);
    } finally {
      setBrandUploading((prev) => ({ ...prev, [key]: false }));
      event.target.value = "";
    }
  };

  const handleBgImageUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    if (!file.type.startsWith("image/")) {
      toast.error("只能上传图片文件");
      return;
    }

    setBgImageUploading(true);
    try {
      const compressedImage = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = (event) => {
          const img = new Image();
          img.onload = () => {
            const canvas = document.createElement("canvas");
            let width = img.width;
            let height = img.height;
            const MAX_WIDTH = 1920;
            const MAX_HEIGHT = 1080;

            if (width > height) {
              if (width > MAX_WIDTH) {
                height = Math.round((height * MAX_WIDTH) / width);
                width = MAX_WIDTH;
              }
            } else {
              if (height > MAX_HEIGHT) {
                width = Math.round((width * MAX_HEIGHT) / height);
                height = MAX_HEIGHT;
              }
            }

            canvas.width = width;
            canvas.height = height;
            const ctx = canvas.getContext("2d");
            if (!ctx) {
              reject(new Error("Canvas context is null"));
              return;
            }
            ctx.drawImage(img, 0, 0, width, height);
            
            // Output as webp for better compression
            const dataUrl = canvas.toDataURL("image/webp", 0.8);
            resolve(dataUrl);
          };
          img.onerror = () => reject(new Error("图片加载失败"));
          img.src = event.target?.result as string;
        };
        reader.onerror = () => reject(new Error("文件读取失败"));
        reader.readAsDataURL(file);
      });

      handleConfigChange("app_bg_image", compressedImage);
      toast.success("自定义背景上传成功");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "图片处理失败");
    } finally {
      setBgImageUploading(false);
      e.target.value = "";
    }
  };

  const renderBgImageUploader = () => {
    const bgImage = configs["app_bg_image"] || "";
    const isImage = bgImage.startsWith("http") || bgImage.startsWith("data:") || bgImage.startsWith("/") || bgImage.startsWith("blob:");
    const isTheme = bgImage === "theme";
    const isSolidColor = bgImage && !isImage && !isTheme;
    
    return (
      <div className="flex flex-col gap-4 w-full">
        <div className="flex flex-wrap items-center gap-4">
          <input
            type="file"
            accept="image/*"
            ref={bgImageFileInputRef}
            className="hidden"
            onChange={handleBgImageUpload}
          />
          <Button
            color="primary"
            variant="flat"
            onPress={() => bgImageFileInputRef.current?.click()}
            isLoading={bgImageUploading}
          >
            上传图片
          </Button>
          <Button
            color="secondary"
            variant="flat"
            onPress={() => handleConfigChange("app_bg_image", "theme")}
            isDisabled={bgImageUploading || isTheme}
          >
            自适应纯色 (跟随深色模式)
          </Button>
          <Button
            color="default"
            variant="flat"
            onPress={() => handleConfigChange("app_bg_image", "#ffffff")}
            isDisabled={bgImageUploading || bgImage === "#ffffff"}
          >
            白色纯色
          </Button>
          {bgImage && (
            <Button
              color="danger"
              variant="flat"
              onPress={() => handleConfigChange("app_bg_image", "")}
              isDisabled={bgImageUploading}
            >
              恢复默认
            </Button>
          )}
        </div>
        
        {bgImage && isImage && (
          <div className="relative rounded-xl overflow-hidden border border-divider">
            <img
              src={bgImage}
              alt="背景预览"
              className="w-full max-h-48 object-cover opacity-80"
            />
            <div className="absolute inset-0 bg-gradient-to-t from-black/50 to-transparent flex items-end p-4">
              <span className="text-white text-sm font-medium">预览效果</span>
            </div>
          </div>
        )}

        {bgImage && isTheme && (
          <div className="relative rounded-xl border border-divider bg-background h-32 flex items-center justify-center">
            <span className="text-foreground text-sm font-medium">当前为自适应纯色背景</span>
          </div>
        )}

        {bgImage && isSolidColor && (
          <div 
            className="relative rounded-xl border border-divider h-32 flex items-center justify-center" 
            style={{ backgroundColor: bgImage }}
          >
            <span className="text-gray-500 bg-white/80 dark:bg-black/80 px-2 py-1 rounded text-sm font-medium border border-gray-200 dark:border-gray-800 shadow-sm">
              纯色背景 ({bgImage})
            </span>
          </div>
        )}
      </div>
    );
  };

  const renderBrandPreview = (key: BrandPreviewKey) => {
    const previewUrl = (configs[key] || "").trim();
    const appNamePreview = (configs.app_name || "").trim() || "应用名称";
    const failed = previewLoadFailed[key] === true;
    const showImage = previewUrl.length > 0 && !failed;

    return (
      <div className="mt-3 rounded-lg border border-default-200 dark:border-default-100/30 bg-default-50/60 dark:bg-default-100/10 p-3">
        <p className="text-xs text-default-500">实时预览</p>
        <div className="mt-2 rounded-md border border-default-200 dark:border-default-100/30 bg-white dark:bg-black px-3 py-2">
          {key === "app_logo" ? (
            <div className="flex h-10 items-center gap-2">
              {showImage ? (
                <img
                  alt="logo preview"
                  className="h-7 w-7 rounded-sm border border-default-200 object-cover dark:border-default-100/30"
                  src={previewUrl}
                  onError={() =>
                    setPreviewLoadFailed((prev) => ({ ...prev, [key]: true }))
                  }
                  onLoad={() =>
                    setPreviewLoadFailed((prev) => ({ ...prev, [key]: false }))
                  }
                />
              ) : (
                <div className="flex h-7 w-7 items-center justify-center rounded-sm bg-default-200 text-[10px] font-semibold text-default-600 dark:bg-default-700 dark:text-default-200">
                  LOGO
                </div>
              )}
              <span className="truncate text-sm font-semibold text-foreground">
                {appNamePreview}
              </span>
            </div>
          ) : (
            <div className="flex h-7 max-w-[260px] items-center gap-2 rounded border border-default-200 bg-default-100/70 px-2 dark:border-default-100/30 dark:bg-default-100/20">
              {showImage ? (
                <img
                  alt="favicon preview"
                  className="h-4 w-4 rounded-sm object-contain"
                  src={previewUrl}
                  onError={() =>
                    setPreviewLoadFailed((prev) => ({ ...prev, [key]: true }))
                  }
                  onLoad={() =>
                    setPreviewLoadFailed((prev) => ({ ...prev, [key]: false }))
                  }
                />
              ) : (
                <div className="h-4 w-4 rounded-sm bg-default-300 dark:bg-default-600" />
              )}
              <span className="truncate text-xs text-default-700 dark:text-default-300">
                {appNamePreview}
              </span>
            </div>
          )}
        </div>

        {previewUrl.length === 0 ? (
          <p className="mt-2 text-xs text-default-500">
            上传图片后会实时显示预览
          </p>
        ) : null}

        {previewUrl.length > 0 && failed ? (
          <p className="mt-2 text-xs text-danger">图片加载失败，请重新上传</p>
        ) : null}

        {previewUrl.length > 0 && !isPngDataURL(previewUrl) ? (
          <p className="mt-2 text-xs text-warning-600 dark:text-warning-400">
            当前是旧版 URL 配置，建议重新上传图片以启用无闪烁加载
          </p>
        ) : null}
      </div>
    );
  };

  const renderBrandAssetUploader = (
    key: BrandPreviewKey,
    isChanged: boolean,
  ) => {
    const value = (configs[key] || "").trim();
    const uploading = brandUploading[key] === true;
    const isLogo = key === "app_logo";
    const isCommercialDisabled = configs.is_commercial !== "true";

    return (
      <div
        className={`rounded-lg border p-3 ${
          isChanged
            ? "border-warning-300"
            : "border-default-200 dark:border-default-100/30"
        }`}
      >
        <input
          ref={getBrandInputRef(key)}
          accept={BRAND_FILE_ACCEPT}
          className="hidden"
          disabled={uploading || isCommercialDisabled}
          type="file"
          onChange={(event) => {
            void handleBrandFileChange(key, event);
          }}
        />

        <div className="flex flex-wrap items-center gap-2">
          <Button
            color="primary"
            isLoading={uploading}
            isDisabled={isCommercialDisabled}
            size="sm"
            variant="flat"
            onPress={() => triggerBrandFilePicker(key)}
          >
            {value.length > 0
              ? isLogo
                ? "替换 Logo"
                : "替换 Favicon"
              : isLogo
                ? "上传 Logo"
                : "上传 Favicon"}
          </Button>
          <Button
            isDisabled={value.length === 0 || uploading}
            size="sm"
            variant="light"
            onPress={() => clearBrandAsset(key)}
          >
            清除
          </Button>
          <span className="text-xs text-default-500">
            仅支持图片文件，自动转换为 PNG
          </span>
        </div>

        <p className="mt-2 text-xs text-default-500">
          {isLogo
            ? "建议上传方形图片，系统会统一转换为 96x96 PNG"
            : "建议上传方形图片，系统会统一转换为 64x64 PNG"}
        </p>

        {renderBrandPreview(key)}
      </div>
    );
  };

  // 渲染不同类型的配置项
  const renderConfigItem = (item: ConfigItem) => {
    const isChanged =
      hasChanges && configs[item.key] !== originalConfigs[item.key];
    const isCommercialDisabled = ["app_name", "app_logo", "app_favicon", "hide_footer_brand"].includes(item.key) && configs.is_commercial !== "true";

    switch (item.type) {
      case "bg_image":
        return renderBgImageUploader();

      case "input":
        if (isBrandPreviewKey(item.key)) {
          return renderBrandAssetUploader(item.key, isChanged);
        }

        return (
          <Input
            classNames={{
              input: "text-sm",
              inputWrapper: isChanged
                ? "border-warning-300 data-[hover=true]:border-warning-400"
                : "",
            }}
            placeholder={item.placeholder}
            size="md"
            value={configs[item.key] || ""}
            variant="bordered"
            onChange={(e) => handleConfigChange(item.key, e.target.value)}
            isDisabled={isCommercialDisabled}
            description={isCommercialDisabled ? "需商业版授权才能修改此项" : undefined}
          />
        );

      case "switch":
        return (
          <Switch
            classNames={{
              wrapper: isChanged ? "border-warning-300" : "",
            }}
            isSelected={configs[item.key] === "true"}
            size="sm"
            onValueChange={(checked) =>
              handleConfigChange(item.key, checked ? "true" : "false")
            }
            isDisabled={isCommercialDisabled}
          >
            <span className="text-sm text-gray-700 dark:text-gray-300">
              {configs[item.key] === "true" ? "已启用" : "已禁用"}
            </span>
          </Switch>
        );

      case "select":
        return (
          <Select
            classNames={{
              trigger: isChanged
                ? "border-warning-300 data-[hover=true]:border-warning-400"
                : "",
            }}
            placeholder="请选择验证码类型"
            selectedKeys={configs[item.key] ? [configs[item.key]] : []}
            size="md"
            variant="bordered"
            onSelectionChange={(keys) => {
              const selectedKey = Array.from(keys)[0] as string;

              if (selectedKey) {
                handleConfigChange(item.key, selectedKey);
              }
            }}
          >
            {item.options?.map((option) => (
              <SelectItem key={option.value} description={option.description}>
                {option.label}
              </SelectItem>
            )) || []}
          </Select>
        );

      default:
        return null;
    }
  };

  const handleExport = async () => {
    if (exportTypes.length === 0) {
      toast.error("请至少选择一种数据类型");

      return;
    }
    setExporting(true);
    try {
      await exportBackup(exportTypes);
      toast.success("导出成功");
      setExportSelectorOpen(false);
    } catch {
      toast.error("导出失败，请重试");
    } finally {
      setExporting(false);
    }
  };

  const triggerImportFilePicker = () => {
    if (importTypes.length === 0) {
      toast.error("请先选择要导入的数据类型");

      return;
    }

    setImportSelectorOpen(false);
    requestAnimationFrame(() => backupFileInputRef.current?.click());
  };

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];

    if (!file) return;

    if (importTypes.length === 0) {
      toast.error("请先选择要导入的数据类型");

      return;
    }

    setImportFileName(file.name);
    setImporting(true);

    try {
      const text = await file.text();
      const data = JSON.parse(text);

      const response = await importBackup({
        types: importTypes,
        ...data,
      });

      if (response.code === 0) {
        toast.success(`导入成功: ${JSON.stringify(response.data)}`);
        setImportTypes([]);
        setImportFileName("");
      } else {
        toast.error("导入失败: " + response.msg);
      }
    } catch {
      toast.error("导入失败，请检查文件格式");
    } finally {
      setImporting(false);
      if (backupFileInputRef.current) {
        backupFileInputRef.current.value = "";
      }
    }
  };

  const toggleTypeSelection = (
    type: string,
    setTypes: React.Dispatch<React.SetStateAction<string[]>>,
  ) => {
    setTypes((prev) =>
      prev.includes(type)
        ? prev.filter((item) => item !== type)
        : [...prev, type],
    );
  };

  const isAllTypesSelected = (types: string[]) =>
    BACKUP_TYPE_VALUES.every((type) => types.includes(type));

  const renderTypeSelection = (
    label: string,
    selectedTypes: string[],
    setTypes: React.Dispatch<React.SetStateAction<string[]>>,
  ) => {
    const allSelected = isAllTypesSelected(selectedTypes);

    return (
      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-sm font-medium text-default-700 dark:text-default-300">
            {label}
          </span>
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="flat"
              onPress={() =>
                setTypes(allSelected ? [] : [...BACKUP_TYPE_VALUES])
              }
            >
              {allSelected ? "取消全选" : "全选"}
            </Button>
            <Button size="sm" variant="light" onPress={() => setTypes([])}>
              清空
            </Button>
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
          {BACKUP_TYPE_OPTIONS.map((option) => {
            const isSelected = selectedTypes.includes(option.value);

            return (
              <button
                key={option.value}
                aria-pressed={isSelected}
                className={`w-full px-4 py-3 rounded-lg border transition-all duration-200 cursor-pointer text-left ${
                  isSelected
                    ? "bg-primary-50 dark:bg-primary-900/20 border-primary-300 dark:border-primary-500/50 shadow-sm"
                    : "bg-white dark:bg-default-50 border-default-200 dark:border-default-100/30 hover:border-primary-200 dark:hover:border-primary-500/30 hover:shadow-sm"
                }`}
                type="button"
                onClick={() => toggleTypeSelection(option.value, setTypes)}
              >
                <div className="flex items-center gap-3">
                  <Checkbox
                    classNames={{
                      base: "pointer-events-none",
                    }}
                    color="primary"
                    isSelected={isSelected}
                    size="md"
                  />
                  <span
                    className={`font-medium ${
                      isSelected
                        ? "text-default-900 dark:text-default-100"
                        : "text-default-700 dark:text-default-500"
                    }`}
                  >
                    {option.label}
                  </span>
                </div>
              </button>
            );
          })}
        </div>
      </div>
    );
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Spinner label="加载配置中..." size="lg" />
      </div>
    );
  }

  return (
    <div className="p-6 max-w-4xl mx-auto">
      {/* 页面标题 */}
      <div className="flex items-center gap-3 mb-6">
        <Button
          isIconOnly
          aria-label="返回上一页"
          className="min-w-0 w-9 h-9"
          size="sm"
          variant="flat"
          onPress={handleBack}
        >
          <BackIcon className="w-5 h-5" />
        </Button>
        <SettingsIcon className="w-8 h-8 text-primary" />
        <div>
          <h1 className="text-2xl font-bold">网站配置</h1>
          <p className="text-gray-600 dark:text-gray-400">
            管理网站的基本信息和显示设置
          </p>
        </div>
      </div>

      <Card className="shadow-md mb-6">
        <CardHeader className="pb-6">
          <div className="flex items-center w-full">
            <div>
              <h2 className="text-xl font-semibold">商业版授权</h2>
              <p className="text-sm text-gray-600 dark:text-gray-400">
                激活商业版授权以解锁自定义品牌功能（替换 Logo、应用名称，移除底部版权信息等）
              </p>
            </div>
          </div>
        </CardHeader>
        <Divider />
        <CardBody className="pt-8">
          <div className="flex items-end gap-3 max-w-lg mb-6">
            <Input
              label="授权激活码"
              placeholder="请输入 FLVX- 开头的商业授权码"
              value={licenseKeyInput}
              variant="bordered"
              onChange={(e) => setLicenseKeyInput(e.target.value)}
              isDisabled={configs.is_commercial === "true"}
              description={configs.is_commercial === "true" ? (configs.license_expiry && configs.license_expiry !== "never" ? `已激活商业版授权 (有效期至: ${new Date(configs.license_expiry).toLocaleDateString()})` : "已激活商业版授权 (永久有效)") : "需商业授权才能修改站名、图标并隐藏页脚品牌"}
            />
            <Button
              color="primary"
              className="mb-6"
              isDisabled={configs.is_commercial === "true" || !licenseKeyInput.trim()}
              isLoading={activatingLicense}
              onPress={handleActivateLicense}
            >
              {configs.is_commercial === "true" ? "已授权" : "激活授权"}
            </Button>
          </div>
          
          {configs.is_commercial === "true" && (
            <div className="space-y-6">
              <Divider className="my-2" />
              <h3 className="text-md font-medium text-default-700">白标与品牌设置</h3>
              {CONFIG_ITEMS.filter(item => ["app_name", "app_logo", "app_favicon", "hide_footer_brand"].includes(item.key)).map((item, index) => (
                <div key={item.key}>
                  <div className="grid grid-cols-1 gap-6 md:grid-cols-[1fr_2fr]">
                    <div className="space-y-1">
                      <label className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70">
                        {item.label}
                      </label>
                      {item.description && (
                        <p className="text-xs text-gray-500 dark:text-gray-400">
                          {item.description}
                        </p>
                      )}
                    </div>
                    <div className="flex items-start">
                      {renderConfigItem(item)}
                    </div>
                  </div>
                  {index < 3 && <Divider className="mt-6" />}
                </div>
              ))}
            </div>
          )}
        </CardBody>
      </Card>

      <Card className="shadow-md">
        <CardHeader className="pb-6">
          <div className="flex items-center w-full">
            <div>
              <h2 className="text-xl font-semibold">基本设置</h2>
              <p className="text-sm text-gray-600 dark:text-gray-400">
                配置网站的基本信息，这些设置会影响网站的显示效果
              </p>
            </div>
          </div>
        </CardHeader>

        <Divider />

        <CardBody className="space-y-6 pt-8 md:pt-8">
          {CONFIG_ITEMS.map((item, index) => {
            // 检查配置项是否应该显示
            if (!shouldShowItem(item)) {
              return null;
            }

            // 计算是否是最后一个显示的项目（用于决定是否显示分隔线）
            const remainingItems = CONFIG_ITEMS.slice(index + 1).filter(
              shouldShowItem,
            );
            const isLastItem = remainingItems.length === 0;

            return (
              <div key={item.key} className="space-y-3">
                <div className="flex flex-col gap-1">
                  <label className="text-sm font-medium text-gray-700 dark:text-gray-300">
                    {item.label}
                  </label>
                  {item.description && (
                    <p className="text-xs text-gray-500 dark:text-gray-400">
                      {item.description}
                    </p>
                  )}
                </div>

                {/* 渲染配置项 */}
                {renderConfigItem(item)}

                {/* 分隔线 */}
                {!isLastItem && <Divider className="mt-6" />}
              </div>
            );
          })}

          <Divider className="my-2" />

          <div className="space-y-3">
            <div className="flex flex-col gap-1">
              <p className="text-sm font-medium text-gray-700 dark:text-gray-300">
                更新通道
              </p>
              <p className="text-xs text-gray-500 dark:text-gray-400">
                稳定版仅匹配纯数字版本；开发版仅匹配包含 alpha / beta / rc
                的版本。
              </p>
            </div>

            <Select
              selectedKeys={[updateChannel]}
              size="md"
              variant="bordered"
              onSelectionChange={(keys) => {
                const selected =
                  (Array.from(keys)[0] as UpdateReleaseChannel) || "stable";

                handleUpdateChannelChange(selected);
              }}
            >
              <SelectItem key="stable" description="仅纯数字版本，如 2.1.4">
                稳定版
              </SelectItem>
              <SelectItem
                key="dev"
                description="仅 alpha / beta / rc 关键字版本"
              >
                开发版
              </SelectItem>
            </Select>
          </div>

          <div className="flex justify-end pt-6 border-t border-divider/50 mt-4">
            <Button
              color="primary"
              disabled={!hasChanges}
              isLoading={saving}
              startContent={<SaveIcon className="w-4 h-4" />}
              onPress={handleSave}
            >
              {saving ? "保存中..." : "保存配置"}
            </Button>
          </div>
        </CardBody>
      </Card>

      {/* 主题设置 */}
      <div className="mt-6">
        <ThemeSettings />
      </div>

      {hasChanges && (
        <Card className="mt-4 bg-warning-50 dark:bg-warning-900/20 border-warning-200 dark:border-warning-800 shadow-sm overflow-hidden">
          <div className="h-10 flex items-center justify-center gap-2 text-warning-700 dark:text-warning-300">
            <div className="w-2 h-2 bg-warning-500 rounded-full animate-pulse flex-shrink-0" />
            <span className="text-sm font-medium leading-none">
              检测到配置变更，请记得保存您的修改
            </span>
          </div>
        </Card>
      )}

      <Card className="mt-6 shadow-md">
        <CardHeader className="pb-6">
          <div className="flex justify-between items-center w-full">
            <div>
              <h2 className="text-xl font-semibold">公告管理</h2>
              <p className="text-sm text-gray-600 dark:text-gray-400">
                设置首页显示的公告内容
              </p>
            </div>
          </div>
        </CardHeader>

        <Divider />

        <CardBody className="space-y-4 pt-8 md:pt-8">
          {announcementLoading ? (
            <div className="flex justify-center py-8">
              <Spinner size="lg" />
            </div>
          ) : (
            <>
              <div className="space-y-2">
                <Switch
                  isSelected={announcement.enabled === 1}
                  onValueChange={(checked) =>
                    setAnnouncement({
                      ...announcement,
                      enabled: checked ? 1 : 0,
                    })
                  }
                >
                  <span className="text-sm text-gray-700 dark:text-gray-300">
                    {announcement.enabled === 1 ? "已启用" : "已禁用"}
                  </span>
                </Switch>
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  启用后，公告将在首页顶部显示
                </p>
              </div>

              <Textarea
                label="公告内容"
                minRows={4}
                placeholder="支持 Markdown，例如：**加粗**、[链接](https://example.com)、- 列表"
                value={announcement.content}
                variant="bordered"
                onChange={(e) =>
                  setAnnouncement({ ...announcement, content: e.target.value })
                }
              />
              <p className="text-xs text-gray-500 dark:text-gray-400">
                公告支持 Markdown 语法，链接会在新标签页打开
              </p>

              <div className="flex justify-end mt-4 pt-4 border-t border-divider/50">
                <Button
                  color="primary"
                  isLoading={announcementSaving}
                  startContent={<SaveIcon className="w-4 h-4" />}
                  onPress={saveAnnouncement}
                >
                  保存公告
                </Button>
              </div>
            </>
          )}
        </CardBody>
      </Card>

      {/* 备份与恢复 */}
      <Card className="mt-6 shadow-md">
        <CardHeader className="pb-6">
          <div className="flex justify-between items-center w-full">
            <div>
              <h2 className="text-xl font-semibold">数据备份与恢复</h2>
              <p className="text-sm text-gray-600 dark:text-gray-400">
                导出或导入系统数据，支持选择特定数据类型
              </p>
            </div>
          </div>
        </CardHeader>

        <Divider />

        <CardBody className="space-y-6 pt-8 md:pt-8">
          {/* 导出部分 */}
          <div className="space-y-4">
            <h3 className="text-lg font-medium">导出数据</h3>
            <p className="text-sm text-gray-600 dark:text-gray-400">
              选择要导出的数据类型，导出为 JSON 格式文件
            </p>
            <p className="text-xs text-default-500">
              当前已选 {exportTypes.length} / {BACKUP_TYPE_VALUES.length}
            </p>

            <div className="flex justify-end gap-3 pt-4">
              <Button
                color="primary"
                isLoading={exporting}
                onPress={() => setExportSelectorOpen(true)}
              >
                {exporting ? "导出中..." : "选择并导出"}
              </Button>
            </div>
          </div>

          <Divider />

          {/* 导入部分 */}
          <div className="space-y-4">
            <h3 className="text-lg font-medium">导入数据</h3>
            <p className="text-sm text-gray-600 dark:text-gray-400">
              选择要导入的数据类型，支持从备份文件恢复数据
            </p>
            <p className="text-xs text-default-500">
              当前已选 {importTypes.length} / {BACKUP_TYPE_VALUES.length}
            </p>

            <input
              ref={backupFileInputRef}
              accept=".json"
              className="hidden"
              type="file"
              onChange={handleFileChange}
            />

            <div className="flex justify-end gap-3 pt-4">
              <Button
                color="primary"
                isLoading={importing}
                variant="flat"
                onPress={() => setImportSelectorOpen(true)}
              >
                {importing ? "导入中..." : "选择并导入"}
              </Button>
              {importFileName && (
                <span className="self-center text-sm text-gray-600 dark:text-gray-400">
                  已选择: {importFileName}
                </span>
              )}
            </div>
          </div>
        </CardBody>
      </Card>

      <Modal
        backdrop="blur"
        classNames={{
          base: "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-2xl overflow-hidden",
        }}
        isOpen={exportSelectorOpen}
        onOpenChange={setExportSelectorOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>选择导出内容</ModalHeader>
              <ModalBody>
                {renderTypeSelection("导出内容", exportTypes, setExportTypes)}
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color="primary"
                  isLoading={exporting}
                  onPress={handleExport}
                >
                  {exporting ? "导出中..." : "确认导出"}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <Modal
        backdrop="blur"
        classNames={{
          base: "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-2xl overflow-hidden",
        }}
        isOpen={importSelectorOpen}
        onOpenChange={setImportSelectorOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>选择导入内容</ModalHeader>
              <ModalBody>
                {renderTypeSelection("导入内容", importTypes, setImportTypes)}
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color="primary"
                  isDisabled={importTypes.length === 0}
                  onPress={triggerImportFilePicker}
                >
                  下一步选择文件
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* Floating Save Button (FAB) */}
      <AnimatePresence>
        {hasChanges && (
          <motion.div
            initial={{ y: 100, opacity: 0 }}
            animate={{ y: 0, opacity: 1 }}
            exit={{ y: 100, opacity: 0 }}
            transition={{ type: "spring", damping: 20, stiffness: 300 }}
            className="fixed bottom-6 right-6 z-50"
          >
            <Button
              isIconOnly
              color="primary"
              size="lg"
              className="w-12 h-12 rounded-full shadow-lg"
              isLoading={saving}
              onPress={handleSave}
            >
              {!saving && <SaveIcon className="w-5 h-5" />}
            </Button>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}
