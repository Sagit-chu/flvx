import { useState, useEffect, useMemo } from "react";
import toast from "react-hot-toast";

import { Card, CardBody, CardHeader } from "@/shadcn-bridge/heroui/card";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Input } from "@/shadcn-bridge/heroui/input";
import { Select, SelectItem } from "@/shadcn-bridge/heroui/select";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from "@/shadcn-bridge/heroui/modal";
import { Chip } from "@/shadcn-bridge/heroui/chip";
import {
  createSpeedLimit,
  getSpeedLimitList,
  updateSpeedLimit,
  deleteSpeedLimit,
  getTunnelList,
} from "@/api";
import { SearchIcon } from "@/components/icons";
import { PageLoadingState } from "@/components/page-state";

interface SpeedLimitRule {
  id: number;
  name: string;
  speed: number;
  status: number;
  tunnelId: number;
  tunnelName: string;
  createdTime: string;
  updatedTime: string;
}

interface Tunnel {
  id: number;
  name: string;
}

interface SpeedLimitForm {
  id?: number;
  name: string;
  speed: number;
  tunnelId: number | null;
  tunnelName: string;
  status: number;
}

export default function LimitPage() {
  const [loading, setLoading] = useState(true);
  const [rules, setRules] = useState<SpeedLimitRule[]>([]);
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [searchKeyword, setSearchKeyword] = useState("");
  const [isSearchVisible, setIsSearchVisible] = useState(false);

  const filteredRules = useMemo(() => {
    if (!searchKeyword.trim()) return rules;
    const lowerKeyword = searchKeyword.toLowerCase();

    return rules.filter(r =>
      (r.name && r.name.toLowerCase().includes(lowerKeyword)) ||
      (r.tunnelName && r.tunnelName.toLowerCase().includes(lowerKeyword))
    );
  }, [rules, searchKeyword]);

  // 模态框状态
  const [modalOpen, setModalOpen] = useState(false);
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [isEdit, setIsEdit] = useState(false);
  const [submitLoading, setSubmitLoading] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [ruleToDelete, setRuleToDelete] = useState<SpeedLimitRule | null>(null);

  // 表单状态
  const [form, setForm] = useState<SpeedLimitForm>({
    name: "",
    speed: 100,
    tunnelId: null,
    tunnelName: "",
    status: 1,
  });

  // 表单验证错误
  const [errors, setErrors] = useState<{ [key: string]: string }>({});

  useEffect(() => {
    loadData();
  }, []);

  // 加载所有数据
  const loadData = async () => {
    setLoading(true);
    try {
      const [rulesRes, tunnelsRes] = await Promise.all([
        getSpeedLimitList(),
        getTunnelList(),
      ]);

      if (rulesRes.code === 0) {
        setRules(rulesRes.data || []);
      } else {
        toast.error(rulesRes.msg || "获取限速规则失败");
      }

      if (tunnelsRes.code === 0) {
        setTunnels(tunnelsRes.data || []);
      } else {
      }
    } catch {
      toast.error("加载数据失败");
    } finally {
      setLoading(false);
    }
  };

  // 表单验证
  const validateForm = (): boolean => {
    const newErrors: { [key: string]: string } = {};

    if (!form.name.trim()) {
      newErrors.name = "请输入规则名称";
    } else if (form.name.length < 2 || form.name.length > 50) {
      newErrors.name = "规则名称长度应在2-50个字符之间";
    }

    if (!form.speed || form.speed < 1) {
      newErrors.speed = "请输入有效的速度限制（≥1 Mbps）";
    }

    if (!form.tunnelId) {
      newErrors.tunnelId = "请选择要绑定的隧道";
    }

    setErrors(newErrors);

    return Object.keys(newErrors).length === 0;
  };

  // 新增规则
  const handleAdd = () => {
    setIsEdit(false);
    setForm({
      name: "",
      speed: 100,
      tunnelId: null,
      tunnelName: "",
      status: 1,
    });
    setErrors({});
    setModalOpen(true);
  };

  // 编辑规则
  const handleEdit = (rule: SpeedLimitRule) => {
    setIsEdit(true);
    setForm({
      id: rule.id,
      name: rule.name,
      speed: rule.speed,
      tunnelId: rule.tunnelId,
      tunnelName: rule.tunnelName,
      status: rule.status,
    });
    setErrors({});
    setModalOpen(true);
  };

  // 显示删除确认
  const handleDelete = (rule: SpeedLimitRule) => {
    setRuleToDelete(rule);
    setDeleteModalOpen(true);
  };

  // 确认删除规则
  const confirmDelete = async () => {
    if (!ruleToDelete) return;

    setDeleteLoading(true);
    try {
      const res = await deleteSpeedLimit(ruleToDelete.id);

      if (res.code === 0) {
        toast.success("删除成功");
        setDeleteModalOpen(false);
        loadData();
      } else {
        toast.error(res.msg || "删除失败");
      }
    } catch {
      toast.error("删除失败");
    } finally {
      setDeleteLoading(false);
    }
  };

  // 提交表单
  const handleSubmit = async () => {
    if (!validateForm()) return;

    setSubmitLoading(true);
    try {
      let res: { code: number; msg: string };

      if (isEdit) {
        res = await updateSpeedLimit(form);
      } else {
        const createData = { ...form };

        delete createData.id;

        res = await createSpeedLimit(createData);
      }

      if (res.code === 0) {
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

  if (loading) {
    return <PageLoadingState message="正在加载..." />;
  }

  return (
    <div className="px-3 lg:px-6 py-8">
      <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between mb-6 gap-3">
        <div className="flex-1 max-w-sm flex items-center gap-2">
          {!isSearchVisible ? (
            <Button
              isIconOnly
              aria-label="搜索"
              className="text-default-600"
              color="default"
              size="sm"
              variant="flat"
              onPress={() => setIsSearchVisible(true)}
            >
              <SearchIcon className="w-4 h-4" />
            </Button>
          ) : (
            <div className="flex w-full items-center gap-2 animate-appearance-in">
              <Input
                autoFocus
                classNames={{
                  base: "bg-default-100",
                  input: "bg-transparent",
                  inputWrapper:
                    "bg-default-100 border-2 border-default-200 hover:border-default-300 data-[hover=true]:border-default-300",
                }}
                placeholder="搜索规则名称或绑定隧道"
                value={searchKeyword}
                onChange={(e) => setSearchKeyword(e.target.value)}
              />
              <Button
                isIconOnly
                aria-label="关闭搜索"
                className="text-default-600 shrink-0"
                color="default"
                size="sm"
                variant="light"
                onPress={() => {
                  setIsSearchVisible(false);
                  setSearchKeyword("");
                }}
              >
                <svg
                  aria-hidden="true"
                  className="w-4 h-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    d="M6 18L18 6M6 6l12 12"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                  />
                </svg>
              </Button>
            </div>
          )}
        </div>

        <Button color="primary" size="sm" variant="flat" onPress={handleAdd}>
          新增
        </Button>
      </div>

      {/* 统一卡片网格 */}
      {filteredRules.length > 0 ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-4">
          {filteredRules.map((rule) => (
            <Card
              key={rule.id}
              className="shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden"
            >
              <CardHeader className="pb-2 md:pb-2">
                <div className="flex justify-between items-start w-full">
                  <div>
                    <h3 className="font-semibold text-foreground">
                      {rule.name}
                    </h3>
                  </div>
                  <Chip
                    color={rule.status === 1 ? "success" : "danger"}
                    size="sm"
                    variant="flat"
                  >
                    {rule.status === 1 ? "运行" : "异常"}
                  </Chip>
                </div>
              </CardHeader>
              <CardBody className="pt-0 pb-3 md:pt-0 md:pb-3">
                <div className="space-y-3">
                  <div className="flex justify-between items-center">
                    <span className="text-small text-default-600">
                      速度限制
                    </span>
                    <Chip color="secondary" size="sm" variant="flat">
                      {rule.speed} Mbps
                    </Chip>
                  </div>
                  <div className="flex justify-between items-center">
                    <span className="text-small text-default-600">
                      绑定隧道
                    </span>
                    {rule.tunnelName ? (
                      <Chip color="primary" size="sm" variant="flat">
                        {rule.tunnelName}
                      </Chip>
                    ) : (
                      <span className="text-default-400 text-small">
                        未绑定
                      </span>
                    )}
                  </div>
                </div>

                <div className="flex gap-2 mt-4">
                  <Button
                    className="flex-1"
                    color="primary"
                    size="sm"
                    startContent={
                      <svg
                        aria-hidden="true"
                        className="w-4 h-4"
                        fill="currentColor"
                        viewBox="0 0 20 20"
                      >
                        <path d="M13.586 3.586a2 2 0 112.828 2.828l-.793.793-2.828-2.828.793-.793zM11.379 5.793L3 14.172V17h2.828l8.38-8.379-2.83-2.828z" />
                      </svg>
                    }
                    variant="flat"
                    onPress={() => handleEdit(rule)}
                  >
                    编辑
                  </Button>
                  <Button
                    className="flex-1"
                    color="danger"
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
                    onPress={() => handleDelete(rule)}
                  >
                    删除
                  </Button>
                </div>
              </CardBody>
            </Card>
          ))}
        </div>
      ) : (
        /* 空状态 */
        <Card className="shadow-sm border border-gray-200 dark:border-gray-700 bg-default-50/50">
          <CardBody className="text-center py-20 flex flex-col items-center justify-center min-h-[240px]">
            <h3 className="text-xl font-medium text-foreground tracking-tight mb-2">
              暂无限速规则
            </h3>
            <p className="text-default-500 text-sm max-w-xs mx-auto leading-relaxed">
              还没有创建任何限速规则，点击上方按钮开始创建
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
                  {isEdit ? "编辑限速规则" : "新增限速规则"}
                </h2>
                <p className="text-small text-default-500">
                  {isEdit
                    ? "修改现有限速规则的配置信息"
                    : "创建新的限速规则并绑定到隧道"}
                </p>
              </ModalHeader>
              <ModalBody>
                <div className="space-y-4">
                  <Input
                    errorMessage={errors.name}
                    isInvalid={!!errors.name}
                    label="规则名称"
                    placeholder="请输入限速规则名称"
                    value={form.name}
                    variant="bordered"
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, name: e.target.value }))
                    }
                  />

                  <Input
                    endContent={
                      <div className="pointer-events-none flex items-center">
                        <span className="text-default-400 text-small">
                          Mbps
                        </span>
                      </div>
                    }
                    errorMessage={errors.speed}
                    isInvalid={!!errors.speed}
                    label="速度限制"
                    placeholder="请输入速度限制"
                    type="number"
                    value={form.speed.toString()}
                    variant="bordered"
                    onChange={(e) =>
                      setForm((prev) => ({
                        ...prev,
                        speed: parseInt(e.target.value) || 0,
                      }))
                    }
                  />

                  <Select
                    description={isEdit ? "编辑时无法修改绑定隧道" : undefined}
                    errorMessage={errors.tunnelId}
                    isDisabled={isEdit}
                    isInvalid={!!errors.tunnelId}
                    label="绑定隧道"
                    placeholder="请选择要绑定的隧道"
                    selectedKeys={
                      form.tunnelId ? [form.tunnelId.toString()] : []
                    }
                    variant="bordered"
                    onSelectionChange={(keys) => {
                      const selectedKey = Array.from(keys)[0] as string;

                      if (selectedKey) {
                        const selectedTunnel = tunnels.find(
                          (tunnel) => tunnel.id === parseInt(selectedKey),
                        );

                        setForm((prev) => ({
                          ...prev,
                          tunnelId: parseInt(selectedKey),
                          tunnelName: selectedTunnel?.name || "",
                        }));
                      } else {
                        setForm((prev) => ({
                          ...prev,
                          tunnelId: null,
                          tunnelName: "",
                        }));
                      }
                    }}
                  >
                    {tunnels.map((tunnel) => (
                      <SelectItem key={tunnel.id}>{tunnel.name}</SelectItem>
                    ))}
                  </Select>
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
                  确定要删除限速规则{" "}
                  <span className="font-semibold text-foreground">
                    &quot;{ruleToDelete?.name}&quot;
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
    </div>
  );
}
