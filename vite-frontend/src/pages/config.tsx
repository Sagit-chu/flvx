import { useState, useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";
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
import toast from "react-hot-toast";

import {
  updateConfigs,
  exportBackup,
  importBackup,
  getAnnouncement,
  updateAnnouncement,
  type AnnouncementData,
} from "@/api";
import { SettingsIcon } from "@/components/icons";
import { isAdmin } from "@/utils/auth";
import {
  getCachedConfigs,
  clearConfigCache,
  updateSiteConfig,
} from "@/config/site";

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
  type: "input" | "switch" | "select";
  options?: { label: string; value: string; description?: string }[];
  dependsOn?: string; // 依赖的配置项key
  dependsValue?: string; // 依赖的配置项值
}

// 网站配置项定义
const CONFIG_ITEMS: ConfigItem[] = [
  {
    key: "ip",
    label: "面板后端地址",
    placeholder: "请输入面板后端IP:PORT",
    description:
      "格式“ip:port”,用于对接节点时使用,ip是你安装面板服务器的公网ip,端口是安装脚本内输入的后端端口。不要套CDN,不支持https,通讯数据有加密",
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
];

const BACKUP_TYPE_OPTIONS = [
  { value: "users", label: "用户" },
  { value: "nodes", label: "节点" },
  { value: "tunnels", label: "隧道" },
  { value: "forwards", label: "转发" },
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
    "ip",
    "panel_domain",
  ];
  const initialConfigs: Record<string, string> = {};

  try {
    configKeys.forEach((key) => {
      const cachedValue = localStorage.getItem("vite_config_" + key);

      if (cachedValue) {
        initialConfigs[key] = cachedValue;
      }
    });
  } catch { }

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
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [announcement, setAnnouncement] = useState<AnnouncementData>({
    content: "",
    enabled: 0,
  });
  const [announcementLoading, setAnnouncementLoading] = useState(true);
  const [announcementSaving, setAnnouncementSaving] = useState(false);

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
    } catch (error) {
      console.error("Failed to load announcement:", error);
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

  const handleConfigChange = (key: string, value: string) => {
    const newConfigs = { ...configs, [key]: value };

    setConfigs(newConfigs);

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
      const response = await updateConfigs(configs);

      if (response.code === 0) {
        toast.success("配置保存成功");

        // 清除所有配置缓存，强制下次重新获取
        clearConfigCache();

        // 获取变更的配置项
        const changedKeys = Object.keys(configs).filter(
          (key) => configs[key] !== originalConfigs[key],
        );

        setOriginalConfigs({ ...configs });
        setHasChanges(false);

        // 如果应用名称发生变化，立即更新网站配置
        if (changedKeys.includes("app_name")) {
          await updateSiteConfig();
        }

        // 触发配置更新事件，通知其他组件
        window.dispatchEvent(
          new CustomEvent("configUpdated", {
            detail: { changedKeys },
          }),
        );
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
    if (!item.dependsOn || !item.dependsValue) {
      return true;
    }

    return configs[item.dependsOn] === item.dependsValue;
  };

  // 渲染不同类型的配置项
  const renderConfigItem = (item: ConfigItem) => {
    const isChanged =
      hasChanges && configs[item.key] !== originalConfigs[item.key];

    switch (item.type) {
      case "input":
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
          />
        );

      case "switch":
        return (
          <Switch
            classNames={{
              wrapper: isChanged ? "border-warning-300" : "",
            }}
            color="primary"
            isSelected={configs[item.key] === "true"}
            size="md"
            onValueChange={(checked) =>
              handleConfigChange(item.key, checked ? "true" : "false")
            }
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
    requestAnimationFrame(() => fileInputRef.current?.click());
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
      if (fileInputRef.current) {
        fileInputRef.current.value = "";
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
                className={`w-full px-4 py-3 rounded-lg border transition-all duration-200 cursor-pointer text-left ${isSelected
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
                    className={`font-medium ${isSelected
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
        <SettingsIcon className="w-8 h-8 text-primary" />
        <div>
          <h1 className="text-2xl font-bold">网站配置</h1>
          <p className="text-gray-600 dark:text-gray-400">
            管理网站的基本信息和显示设置
          </p>
        </div>
      </div>

      <Card className="shadow-md">
        <CardHeader className="pb-6">
          <div className="flex justify-between items-center w-full">
            <div>
              <h2 className="text-xl font-semibold">基本设置</h2>
              <p className="text-sm text-gray-600 dark:text-gray-400">
                配置网站的基本信息，这些设置会影响网站的显示效果
              </p>
            </div>
            <div className="flex gap-2">
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
          </div>
        </CardHeader>

        <Divider />

        <CardBody className="space-y-6 pt-6">
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
        </CardBody>
      </Card>

      {hasChanges && (
        <Card className="mt-4 bg-warning-50 dark:bg-warning-900/20 border-warning-200 dark:border-warning-800">
          <CardBody className="py-3">
            <div className="w-full flex items-center justify-center gap-2 text-warning-700 dark:text-warning-300">
              <div className="w-2 h-2 bg-warning-500 rounded-full animate-pulse" />
              <span className="text-sm font-medium">
                检测到配置变更，请记得保存您的修改
              </span>
            </div>
          </CardBody>
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

        <CardBody className="space-y-4 pt-6">
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
                placeholder="请输入公告内容"
                value={announcement.content}
                variant="bordered"
                onChange={(e) =>
                  setAnnouncement({ ...announcement, content: e.target.value })
                }
              />

              <div className="flex justify-end">
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

        <CardBody className="space-y-6 pt-6">
          {/* 导出部分 */}
          <div className="space-y-4">
            <h3 className="text-lg font-medium">导出数据</h3>
            <p className="text-sm text-gray-600 dark:text-gray-400">
              选择要导出的数据类型，导出为 JSON 格式文件
            </p>
            <p className="text-xs text-default-500">
              当前已选 {exportTypes.length} / {BACKUP_TYPE_VALUES.length}
            </p>

            <div className="flex gap-3">
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
              ref={fileInputRef}
              accept=".json"
              className="hidden"
              type="file"
              onChange={handleFileChange}
            />

            <div className="flex gap-3">
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

      <Modal isOpen={exportSelectorOpen} onOpenChange={setExportSelectorOpen}>
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

      <Modal isOpen={importSelectorOpen} onOpenChange={setImportSelectorOpen}>
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
    </div>
  );
}
