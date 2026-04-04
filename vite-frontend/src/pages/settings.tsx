import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import toast from "react-hot-toast";

import { Input } from "@/shadcn-bridge/heroui/input";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Card, CardBody } from "@/shadcn-bridge/heroui/card";
import { Select, SelectItem } from "@/shadcn-bridge/heroui/select";
import { Switch } from "@/shadcn-bridge/heroui/switch";
import { reinitializeBaseURL } from "@/api/network";
import { getConfigByName, updateConfig } from "@/api";
import { BackIcon } from "@/components/icons";
import {
  type UpdateReleaseChannel,
  getUpdateReleaseChannel,
  setUpdateReleaseChannel,
} from "@/utils/version-update";
import { isAdmin } from "@/utils/auth";
import {
  getPanelAddresses,
  savePanelAddress,
  setCurrentPanelAddress,
  deletePanelAddress,
  validatePanelAddress,
} from "@/utils/panel";

interface PanelAddress {
  name: string;
  address: string;
  inx: boolean;
}

const FORWARD_COMPACT_MODE_CONFIG_KEY = "forward_compact_mode";

const parseBooleanConfig = (value: unknown, defaultValue: boolean) =>
  typeof value === "string" ? value === "true" : defaultValue;

export const SettingsPage = () => {
  const navigate = useNavigate();
  const [panelAddresses, setPanelAddresses] = useState<PanelAddress[]>([]);
  const [newName, setNewName] = useState("");
  const [newAddress, setNewAddress] = useState("");
  const [updateChannel, setUpdateChannel] = useState<UpdateReleaseChannel>(
    getUpdateReleaseChannel(),
  );
  const [forwardCompactMode, setForwardCompactMode] = useState(false);
  const [forwardCompactModeSaving, setForwardCompactModeSaving] =
    useState(false);

  const admin = isAdmin();

  const setPanelAddressesFunc = (newAddress: PanelAddress[]) => {
    setPanelAddresses(newAddress);
  };

  useEffect(() => {
    (window as any).setPanelAddresses = setPanelAddressesFunc;

    return () => {
      delete (window as any).setPanelAddresses;
    };
  }, []);

  // 加载面板地址列表
  const loadPanelAddresses = async () => {
    getPanelAddresses();
  };

  // 添加新面板地址
  const addPanelAddress = async () => {
    if (!newName.trim() || !newAddress.trim()) {
      toast.error("请输入名称和地址");

      return;
    }

    // 验证地址格式
    if (!validatePanelAddress(newAddress.trim())) {
      toast.error(
        "地址格式不正确，请检查：\n• 必须是完整的URL格式\n• 必须以 http:// 或 https:// 开头\n• 支持域名、IPv4、IPv6 地址\n• 端口号范围：1-65535\n• 示例：http://192.168.1.100:3000",
      );

      return;
    }
    savePanelAddress(newName.trim(), newAddress.trim());
    setNewName("");
    setNewAddress("");
    toast.success("添加成功");
  };

  // 设置当前面板地址
  const setCurrentPanel = async (name: string) => {
    setCurrentPanelAddress(name);
    reinitializeBaseURL();
  };

  // 删除面板地址
  const handleDeletePanelAddress = async (name: string) => {
    deletePanelAddress(name);
    reinitializeBaseURL();
    toast.success("删除成功");
  };

  // 页面加载时获取数据
  useEffect(() => {
    loadPanelAddresses();
    loadForwardCompactMode();
  }, []);

  const loadForwardCompactMode = async () => {
    try {
      const res = await getConfigByName(FORWARD_COMPACT_MODE_CONFIG_KEY);
      setForwardCompactMode(parseBooleanConfig(res.data?.value, false));
    } catch {
      setForwardCompactMode(false);
    }
  };

  const handleForwardCompactModeChange = async (enabled: boolean) => {
    if (!admin || forwardCompactModeSaving) {
      return;
    }

    const previous = forwardCompactMode;

    setForwardCompactMode(enabled);
    setForwardCompactModeSaving(true);
    try {
      const response = await updateConfig(
        FORWARD_COMPACT_MODE_CONFIG_KEY,
        enabled ? "true" : "false",
      );

      if (response.code === 0) {
        toast.success(`规则页面精简模式已${enabled ? "开启" : "关闭"}`);
        window.dispatchEvent(
          new CustomEvent("forwardCompactModeChanged", {
            detail: { enabled },
          }),
        );
      } else {
        setForwardCompactMode(previous);
        toast.error(response.msg || "保存精简模式失败");
      }
    } catch {
      setForwardCompactMode(previous);
      toast.error("保存精简模式失败");
    } finally {
      setForwardCompactModeSaving(false);
    }
  };

  const handleUpdateChannelChange = (channel: UpdateReleaseChannel) => {
    setUpdateChannel(channel);
    setUpdateReleaseChannel(channel);
    toast.success(
      `更新通道已切换为${channel === "stable" ? "稳定版" : "开发版"}`,
    );
  };

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-black">
      {/* 顶部导航 */}
      <div className="bg-white dark:bg-black border-b border-gray-200 dark:border-gray-700">
        <div className="max-w-4xl mx-auto px-4 py-4">
          <div className="flex items-center gap-3">
            <Button
              isIconOnly
              aria-label="返回上一页"
              className="text-gray-600 dark:text-gray-300"
              variant="light"
              onPress={() => navigate(-1)}
            >
              <BackIcon className="w-5 h-5" />
            </Button>
            <h1 className="text-xl font-semibold text-gray-900 dark:text-white">
              面板设置
            </h1>
          </div>
        </div>
      </div>

      {/* 内容区域 */}
      <div className="max-w-4xl mx-auto px-4 py-6">
        <div className="space-y-6">
          <Card>
            <CardBody className="p-6">
              <h2 className="text-lg font-medium text-gray-900 dark:text-white mb-4">
                更新设置
              </h2>
              <div className="space-y-2">
                <Select
                  label="更新通道"
                  selectedKeys={[updateChannel]}
                  onSelectionChange={(keys) => {
                    const selected =
                      (Array.from(keys)[0] as UpdateReleaseChannel) || "stable";

                    handleUpdateChannelChange(selected);
                  }}
                >
                  <SelectItem key="stable" textValue="stable">
                    稳定版（纯数字版本，如 2.1.4）
                  </SelectItem>
                  <SelectItem key="dev" textValue="dev">
                    开发版（含 alpha / beta / rc）
                  </SelectItem>
                </Select>
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  版本提示会根据该通道检查最新版本。
                </p>
              </div>
            </CardBody>
          </Card>

          <Card>
            <CardBody className="p-6">
              <h2 className="text-lg font-medium text-gray-900 dark:text-white mb-4">
                显示设置
              </h2>
              <div className="space-y-3">
                <div className="rounded-lg border border-gray-200 dark:border-gray-700 px-4 py-3">
                  <div className="flex items-center justify-between gap-4">
                    <div>
                      <p className="text-sm font-medium text-gray-900 dark:text-white">
                        规则页面精简模式
                      </p>
                      <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                        开启后，规则页面列表使用 2.1.6-alpha8 样式。{" "}
                      </p>
                    </div>
                    <Switch
                      color="primary"
                      isDisabled={!admin || forwardCompactModeSaving}
                      isSelected={forwardCompactMode}
                      onValueChange={handleForwardCompactModeChange}
                    />
                  </div>
                  {!admin && (
                    <p className="mt-2 text-xs text-amber-600 dark:text-amber-400">
                      仅管理员可修改该全局配置。
                    </p>
                  )}
                </div>
              </div>
            </CardBody>
          </Card>

          {/* 添加新地址 */}
          <Card>
            <CardBody className="p-6">
              <h2 className="text-lg font-medium text-gray-900 dark:text-white mb-4">
                添加新面板地址
              </h2>
              <div className="space-y-4">
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <Input
                    label="名称"
                    placeholder="请输入面板名称"
                    value={newName}
                    onChange={(e) => setNewName(e.target.value)}
                  />
                  <Input
                    label="地址"
                    placeholder="http://192.168.1.100:3000"
                    value={newAddress}
                    onChange={(e) => setNewAddress(e.target.value)}
                  />
                </div>
                <Button color="primary" onPress={addPanelAddress}>
                  添加
                </Button>
              </div>
            </CardBody>
          </Card>

          {/* 地址列表 */}
          <Card>
            <CardBody className="p-6">
              <h2 className="text-lg font-medium text-gray-900 dark:text-white mb-4">
                已保存的面板地址
              </h2>
              {panelAddresses.length === 0 ? (
                <p className="text-gray-500 dark:text-gray-400 text-center py-8">
                  暂无保存的面板地址
                </p>
              ) : (
                <div className="space-y-3">
                  {panelAddresses.map((panel, index) => (
                    <div
                      key={index}
                      className="border border-gray-200 dark:border-gray-600 rounded-lg p-4"
                    >
                      <div className="flex items-center justify-between">
                        <div className="flex-1">
                          <div className="flex items-center gap-2">
                            <span className="font-medium text-gray-900 dark:text-white">
                              {panel.name}
                            </span>
                            {panel.inx && (
                              <span className="px-2 py-1 bg-green-100 dark:bg-green-500/20 text-green-700 dark:text-green-300 text-xs rounded">
                                当前
                              </span>
                            )}
                          </div>
                          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                            {panel.address}
                          </p>
                        </div>
                        <div className="flex items-center gap-2">
                          {!panel.inx && (
                            <Button
                              color="primary"
                              size="sm"
                              variant="flat"
                              onPress={() => setCurrentPanel(panel.name)}
                            >
                              设为当前
                            </Button>
                          )}
                          <Button
                            color="danger"
                            size="sm"
                            variant="light"
                            onPress={() => handleDeletePanelAddress(panel.name)}
                          >
                            删除
                          </Button>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardBody>
          </Card>
        </div>
      </div>
    </div>
  );
};
