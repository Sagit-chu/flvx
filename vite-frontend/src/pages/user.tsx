import { useState, useEffect } from "react";
import toast from "react-hot-toast";
import { parseDate } from "@internationalized/date";

import { Button } from "@/shadcn-bridge/heroui/button";
import { Card, CardBody, CardHeader } from "@/shadcn-bridge/heroui/card";
import { Input } from "@/shadcn-bridge/heroui/input";
import {
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
} from "@/shadcn-bridge/heroui/table";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
} from "@/shadcn-bridge/heroui/modal";
import { Chip } from "@/shadcn-bridge/heroui/chip";
import { Select, SelectItem } from "@/shadcn-bridge/heroui/select";
import { RadioGroup, Radio } from "@/shadcn-bridge/heroui/radio";
import { Checkbox } from "@/shadcn-bridge/heroui/checkbox";
import { DatePicker } from "@/shadcn-bridge/heroui/date-picker";
import { Spinner } from "@/shadcn-bridge/heroui/spinner";
import { Progress } from "@/shadcn-bridge/heroui/progress";
import {
  User,
  UserForm,
  UserGroup,
  UserTunnel,
  TunnelAssignItem,
  Tunnel,
  SpeedLimit,
  Pagination as PaginationType,
} from "@/types";
import {
  getAllUsers,
  createUser,
  updateUser,
  deleteUser,
  getTunnelList,
  batchAssignUserTunnel,
  getUserTunnelList,
  removeUserTunnel,
  updateUserTunnel,
  getSpeedLimitList,
  resetUserFlow,
  getUserGroupList,
  getUserGroups,
} from "@/api";
import {
  SearchIcon,
  EditIcon,
  DeleteIcon,
  SettingsIcon,
} from "@/components/icons";
import { PageLoadingState } from "@/components/page-state";

// 工具函数
const formatFlow = (value: number, unit: string = "bytes"): string => {
  if (unit === "gb") {
    return `${value} GB`;
  } else {
    if (value === 0) return "0 B";
    if (value < 1024) return `${value} B`;
    if (value < 1024 * 1024) return `${(value / 1024).toFixed(2)} KB`;
    if (value < 1024 * 1024 * 1024)
      return `${(value / (1024 * 1024)).toFixed(2)} MB`;

    return `${(value / (1024 * 1024 * 1024)).toFixed(2)} GB`;
  }
};

const formatDate = (timestamp: number): string => {
  return new Date(timestamp).toLocaleString();
};

const getExpireStatus = (expTime: number) => {
  const now = Date.now();

  if (expTime < now) {
    return { color: "danger" as const, text: "已过期" };
  }
  const diffDays = Math.ceil((expTime - now) / (1000 * 60 * 60 * 24));

  if (diffDays <= 7) {
    return { color: "warning" as const, text: `${diffDays}天后过期` };
  }

  return { color: "success" as const, text: "正常" };
};

// 获取用户状态（根据status字段）
const getUserStatus = (user: User) => {
  if (user.status === 1) {
    return { color: "success" as const, text: "正常" };
  } else {
    return { color: "danger" as const, text: "禁用" };
  }
};

const calculateUserTotalUsedFlow = (user: User): number => {
  return (user.inFlow || 0) + (user.outFlow || 0);
};

const calculateTunnelUsedFlow = (tunnel: UserTunnel): number => {
  const inFlow = tunnel.inFlow || 0;
  const outFlow = tunnel.outFlow || 0;

  // 后端已按计费类型处理流量，前端直接使用入站+出站总和
  return inFlow + outFlow;
};

export default function UserPage() {
  // 状态管理
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState("");
  const [isSearchVisible, setIsSearchVisible] = useState(false);
  const [pagination, setPagination] = useState<PaginationType>({
    current: 1,
    size: 10,
    total: 0,
  });

  // 用户表单相关状态
  const {
    isOpen: isUserModalOpen,
    onOpen: onUserModalOpen,
    onClose: onUserModalClose,
  } = useDisclosure();
  const [isEdit, setIsEdit] = useState(false);
  const [userForm, setUserForm] = useState<UserForm>({
    user: "",
    pwd: "",
    status: 1,
    flow: 100,
    num: 10,
    expTime: null,
    flowResetTime: 0,
  });
  const [userFormLoading, setUserFormLoading] = useState(false);

  // 隧道权限管理相关状态
  const {
    isOpen: isTunnelModalOpen,
    onOpen: onTunnelModalOpen,
    onClose: onTunnelModalClose,
  } = useDisclosure();
  const [currentUser, setCurrentUser] = useState<User | null>(null);
  const [userTunnels, setUserTunnels] = useState<UserTunnel[]>([]);
  const [tunnelListLoading, setTunnelListLoading] = useState(false);

  // 分配新隧道权限相关状态
  const [assignLoading, setAssignLoading] = useState(false);
  const [batchTunnelSelections, setBatchTunnelSelections] = useState<
    Map<number, number | null>
  >(new Map());

  // 编辑隧道权限相关状态
  const {
    isOpen: isEditTunnelModalOpen,
    onOpen: onEditTunnelModalOpen,
    onClose: onEditTunnelModalClose,
  } = useDisclosure();
  const [editTunnelForm, setEditTunnelForm] = useState<UserTunnel | null>(null);
  const [editTunnelLoading, setEditTunnelLoading] = useState(false);

  // 删除确认相关状态
  const {
    isOpen: isDeleteModalOpen,
    onOpen: onDeleteModalOpen,
    onClose: onDeleteModalClose,
  } = useDisclosure();
  const [userToDelete, setUserToDelete] = useState<User | null>(null);

  // 删除隧道权限确认相关状态
  const {
    isOpen: isDeleteTunnelModalOpen,
    onOpen: onDeleteTunnelModalOpen,
    onClose: onDeleteTunnelModalClose,
  } = useDisclosure();
  const [tunnelToDelete, setTunnelToDelete] = useState<UserTunnel | null>(null);

  // 重置流量确认相关状态
  const {
    isOpen: isResetFlowModalOpen,
    onOpen: onResetFlowModalOpen,
    onClose: onResetFlowModalClose,
  } = useDisclosure();
  const [userToReset, setUserToReset] = useState<User | null>(null);
  const [resetFlowLoading, setResetFlowLoading] = useState(false);

  // 重置隧道流量确认相关状态
  const {
    isOpen: isResetTunnelFlowModalOpen,
    onOpen: onResetTunnelFlowModalOpen,
    onClose: onResetTunnelFlowModalClose,
  } = useDisclosure();
  const [tunnelToReset, setTunnelToReset] = useState<UserTunnel | null>(null);
  const [resetTunnelFlowLoading, setResetTunnelFlowLoading] = useState(false);

  // 其他数据
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [speedLimits, setSpeedLimits] = useState<SpeedLimit[]>([]);
  const [userGroups, setUserGroups] = useState<UserGroup[]>([]);

  // 生命周期
  useEffect(() => {
    loadUsers();
    loadTunnels();
    loadSpeedLimits();
    loadUserGroups();
  }, [pagination.current, pagination.size, searchKeyword]);

  // 数据加载函数
  const loadUsers = async () => {
    setLoading(true);
    try {
      const response = await getAllUsers({
        current: pagination.current,
        size: pagination.size,
        keyword: searchKeyword,
      });

      if (response.code === 0) {
        setUsers(Array.isArray(response.data) ? response.data : []);
      } else {
        toast.error(response.msg || "获取用户列表失败");
      }
    } catch {
      toast.error("获取用户列表失败");
    } finally {
      setLoading(false);
    }
  };

  const loadTunnels = async () => {
    try {
      const response = await getTunnelList();

      if (response.code === 0) {
        setTunnels(Array.isArray(response.data) ? response.data : []);
      }
    } catch { }
  };

  const loadSpeedLimits = async () => {
    try {
      const response = await getSpeedLimitList();

      if (response.code === 0) {
        const speedLimitList = Array.isArray(response.data)
          ? response.data.map((item) => ({
            ...item,
            uploadSpeed: item.uploadSpeed ?? item.speed ?? 0,
            downloadSpeed: item.downloadSpeed ?? item.speed ?? 0,
          }))
          : [];

        setSpeedLimits(speedLimitList);
      }
    } catch { }
  };

  const loadUserGroups = async () => {
    try {
      const response = await getUserGroupList();

      if (response.code === 0) {
        setUserGroups(Array.isArray(response.data) ? response.data : []);
      }
    } catch { }
  };

  const loadUserTunnels = async (userId: number) => {
    setTunnelListLoading(true);
    try {
      const response = await getUserTunnelList({ userId });

      if (response.code === 0) {
        setUserTunnels(Array.isArray(response.data) ? response.data : []);
      } else {
        toast.error(response.msg || "获取隧道权限列表失败");
      }
    } catch {
      toast.error("获取隧道权限列表失败");
    } finally {
      setTunnelListLoading(false);
    }
  };

  // 用户管理操作
  const handleSearch = () => {
    setPagination((prev) => ({ ...prev, current: 1 }));
    loadUsers();
  };

  const handleAdd = () => {
    setIsEdit(false);
    setUserForm({
      user: "",
      pwd: "",
      status: 1,
      flow: 100,
      num: 10,
      expTime: null,
      flowResetTime: 0,
      groupIds: [],
    });
    onUserModalOpen();
  };

  const handleEdit = async (user: User) => {
    setIsEdit(true);
    let currentGroupIds: number[] = [];

    try {
      const groupRes = await getUserGroups(user.id);

      if (groupRes.code === 0) {
        currentGroupIds = groupRes.data || [];
      }
    } catch { }

    setUserForm({
      id: user.id,
      name: user.name,
      user: user.user,
      pwd: "",
      status: user.status,
      flow: user.flow,
      num: user.num,
      expTime: user.expTime ? new Date(user.expTime) : null,
      flowResetTime: user.flowResetTime ?? 0,
      groupIds: currentGroupIds,
    });
    onUserModalOpen();
  };

  const handleDelete = (user: User) => {
    setUserToDelete(user);
    onDeleteModalOpen();
  };

  const handleConfirmDelete = async () => {
    if (!userToDelete) return;

    try {
      const response = await deleteUser(userToDelete.id);

      if (response.code === 0) {
        toast.success("删除成功");
        loadUsers();
        onDeleteModalClose();
        setUserToDelete(null);
      } else {
        toast.error(response.msg || "删除失败");
      }
    } catch {
      toast.error("删除失败");
    }
  };

  const handleSubmitUser = async () => {
    if (!userForm.user || (!userForm.pwd && !isEdit) || !userForm.expTime) {
      toast.error("请填写完整信息");

      return;
    }

    setUserFormLoading(true);
    try {
      const submitData: any = {
        ...userForm,
        expTime: userForm.expTime.getTime(),
        groupIds: userForm.groupIds ?? [],
      };

      if (isEdit && !submitData.pwd) {
        delete submitData.pwd;
      }

      const response = isEdit
        ? await updateUser(submitData)
        : await createUser(submitData);

      if (response.code === 0) {
        toast.success(isEdit ? "更新成功" : "创建成功");
        onUserModalClose();
        loadUsers();
      } else {
        toast.error(response.msg || (isEdit ? "更新失败" : "创建失败"));
      }
    } catch {
      toast.error(isEdit ? "更新失败" : "创建失败");
    } finally {
      setUserFormLoading(false);
    }
  };

  // 隧道权限管理操作
  const handleManageTunnels = (user: User) => {
    setCurrentUser(user);
    setBatchTunnelSelections(new Map());
    onTunnelModalOpen();
    loadUserTunnels(user.id);
  };

  const handleBatchAssignTunnel = async () => {
    if (batchTunnelSelections.size === 0 || !currentUser) {
      toast.error("请选择至少一个隧道");

      return;
    }

    setAssignLoading(true);
    try {
      const tunnelsToAssign: TunnelAssignItem[] = Array.from(
        batchTunnelSelections.entries(),
      ).map(([tunnelId, speedId]) => ({ tunnelId, speedId }));

      const response = await batchAssignUserTunnel({
        userId: currentUser.id,
        tunnels: tunnelsToAssign,
      });

      if (response.code === 0) {
        toast.success(response.msg || "分配成功");
        setBatchTunnelSelections(new Map());
        loadUserTunnels(currentUser.id);
      } else {
        toast.error(response.msg || "分配失败");
      }
    } catch {
      toast.error("分配失败");
    } finally {
      setAssignLoading(false);
    }
  };

  const handleEditTunnel = (userTunnel: UserTunnel) => {
    setEditTunnelForm({
      ...userTunnel,
      expTime: userTunnel.expTime,
    });
    onEditTunnelModalOpen();
  };

  const handleUpdateTunnel = async () => {
    if (!editTunnelForm) return;

    setEditTunnelLoading(true);
    try {
      const response = await updateUserTunnel({
        id: editTunnelForm.id,
        flow: editTunnelForm.flow,
        num: editTunnelForm.num,
        expTime: editTunnelForm.expTime,
        flowResetTime: editTunnelForm.flowResetTime,
        speedId: editTunnelForm.speedId,
        status: editTunnelForm.status,
      });

      if (response.code === 0) {
        toast.success("更新成功");
        onEditTunnelModalClose();
        if (currentUser) {
          loadUserTunnels(currentUser.id);
        }
      } else {
        toast.error(response.msg || "更新失败");
      }
    } catch {
      toast.error("更新失败");
    } finally {
      setEditTunnelLoading(false);
    }
  };

  const handleRemoveTunnel = (userTunnel: UserTunnel) => {
    setTunnelToDelete(userTunnel);
    onDeleteTunnelModalOpen();
  };

  const handleConfirmRemoveTunnel = async () => {
    if (!tunnelToDelete) return;

    try {
      const response = await removeUserTunnel({ id: tunnelToDelete.id });

      if (response.code === 0) {
        toast.success("删除成功");
        if (currentUser) {
          loadUserTunnels(currentUser.id);
        }
        onDeleteTunnelModalClose();
        setTunnelToDelete(null);
      } else {
        toast.error(response.msg || "删除失败");
      }
    } catch {
      toast.error("删除失败");
    }
  };

  // 重置流量相关函数
  const handleResetFlow = (user: User) => {
    setUserToReset(user);
    onResetFlowModalOpen();
  };

  const handleConfirmResetFlow = async () => {
    if (!userToReset) return;

    setResetFlowLoading(true);
    try {
      const response = await resetUserFlow({
        id: userToReset.id,
        type: 1, // 1表示重置用户流量
      });

      if (response.code === 0) {
        toast.success("流量重置成功");
        onResetFlowModalClose();
        setUserToReset(null);
        loadUsers(); // 重新加载用户列表
      } else {
        toast.error(response.msg || "重置失败");
      }
    } catch {
      toast.error("重置失败");
    } finally {
      setResetFlowLoading(false);
    }
  };

  // 隧道流量重置相关函数
  const handleResetTunnelFlow = (userTunnel: UserTunnel) => {
    setTunnelToReset(userTunnel);
    onResetTunnelFlowModalOpen();
  };

  const handleConfirmResetTunnelFlow = async () => {
    if (!tunnelToReset) return;

    setResetTunnelFlowLoading(true);
    try {
      const response = await resetUserFlow({
        id: tunnelToReset.id,
        type: 2, // 2表示重置隧道流量
      });

      if (response.code === 0) {
        toast.success("隧道流量重置成功");
        onResetTunnelFlowModalClose();
        setTunnelToReset(null);
        if (currentUser) {
          loadUserTunnels(currentUser.id); // 重新加载隧道权限列表
        }
      } else {
        toast.error(response.msg || "重置失败");
      }
    } catch {
      toast.error("重置失败");
    } finally {
      setResetTunnelFlowLoading(false);
    }
  };

  const editAvailableSpeedLimits = speedLimits.filter(
    (speedLimit) => speedLimit.tunnelId === editTunnelForm?.tunnelId,
  );

  const getSpeedLimitsForTunnel = (tunnelId: number) => {
    return speedLimits.filter((sl) => sl.tunnelId === tunnelId);
  };

  const toggleTunnelSelection = (tunnelId: number) => {
    setBatchTunnelSelections((prev) => {
      const newMap = new Map(prev);

      if (newMap.has(tunnelId)) {
        newMap.delete(tunnelId);
      } else {
        newMap.set(tunnelId, null);
      }

      return newMap;
    });
  };

  const updateTunnelSpeedLimit = (tunnelId: number, speedId: number | null) => {
    setBatchTunnelSelections((prev) => {
      const newMap = new Map(prev);

      newMap.set(tunnelId, speedId);

      return newMap;
    });
  };

  const isTunnelAssigned = (tunnelId: number) => {
    return userTunnels.some((ut) => ut.tunnelId === tunnelId);
  };

  return (
    <div className="px-3 lg:px-6 py-8">
      {/* 页面头部 */}
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
                placeholder="搜索用户名"
                value={searchKeyword}
                onChange={(e) => setSearchKeyword(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && handleSearch()}
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

      {/* 用户列表 */}
      {loading ? (
        <PageLoadingState message="正在加载..." />
      ) : users.length === 0 ? (
        <Card className="shadow-sm border border-gray-200 dark:border-gray-700 bg-default-50/50">
          <CardBody className="text-center py-20 flex flex-col items-center justify-center min-h-[240px]">
            <h3 className="text-xl font-medium text-foreground tracking-tight mb-2">
              暂无用户数据
            </h3>
            <p className="text-default-500 text-sm max-w-xs mx-auto leading-relaxed">
              还没有创建任何用户，点击上方按钮开始创建
            </p>
          </CardBody>
        </Card>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-4">
          {users.map((user) => {
            const userStatus = getUserStatus(user);
            const expStatus = user.expTime
              ? getExpireStatus(user.expTime)
              : null;
            const usedFlow = calculateUserTotalUsedFlow(user);
            const flowPercent =
              user.flow > 0
                ? Math.min(
                  (usedFlow / (user.flow * 1024 * 1024 * 1024)) * 100,
                  100,
                )
                : 0;

            return (
              <Card
                key={user.id}
                className="shadow-sm border border-divider hover:shadow-md transition-shadow duration-200 overflow-hidden"
              >
                <CardHeader className="pb-2 md:pb-2">
                  <div className="flex justify-between items-start w-full">
                    <div className="flex-1 min-w-0">
                      <h3 className="font-semibold text-foreground truncate text-sm">
                        {user.name || user.user}
                      </h3>
                      <p className="text-xs text-default-500 truncate">
                        @{user.user}
                      </p>
                    </div>
                    <div className="flex items-center gap-1.5 ml-2">
                      <Chip
                        className="text-xs"
                        color={userStatus.color}
                        size="sm"
                        variant="flat"
                      >
                        {userStatus.text}
                      </Chip>
                    </div>
                  </div>
                </CardHeader>

                <CardBody className="pt-0 pb-3 md:pt-0 md:pb-3">
                  <div className="space-y-2">
                    {/* 流量信息 */}
                    <div className="space-y-1.5">
                      <div className="flex justify-between text-sm">
                        <span className="text-default-600">流量限制</span>
                        <span className="font-medium text-xs">
                          {formatFlow(user.flow, "gb")}
                        </span>
                      </div>
                      <div className="flex justify-between text-sm">
                        <span className="text-default-600">已使用</span>
                        <span className="font-medium text-xs text-danger">
                          {formatFlow(usedFlow)}
                        </span>
                      </div>
                      <Progress
                        aria-label={`流量使用 ${flowPercent.toFixed(1)}%`}
                        className="mt-1"
                        color={
                          flowPercent > 90
                            ? "danger"
                            : flowPercent > 70
                              ? "warning"
                              : "success"
                        }
                        size="sm"
                        value={flowPercent}
                      />
                    </div>

                    {/* 其他信息 */}
                    <div className="space-y-1.5 pt-2 border-t border-divider">
                      <div className="flex justify-between text-sm">
                        <span className="text-default-600">转发数量</span>
                        <span className="font-medium text-xs">{user.num}</span>
                      </div>
                      <div className="flex justify-between text-sm">
                        <span className="text-default-600">重置日期</span>
                        <span className="text-xs">
                          {user.flowResetTime === 0
                            ? "不重置"
                            : `每月${user.flowResetTime}号`}
                        </span>
                      </div>
                      {user.expTime && (
                        <div className="flex justify-between text-sm">
                          <span className="text-default-600">过期时间</span>
                          <div className="text-right">
                            {expStatus && expStatus.color === "success" ? (
                              <div className="text-xs">
                                {formatDate(user.expTime)}
                              </div>
                            ) : (
                              <Chip
                                className="text-xs"
                                color={expStatus?.color || "default"}
                                size="sm"
                                variant="flat"
                              >
                                {expStatus?.text || "未知状态"}
                              </Chip>
                            )}
                          </div>
                        </div>
                      )}
                    </div>
                  </div>

                  <div className="space-y-1.5 mt-3">
                    {/* 第一行：编辑和重置 */}
                    <div className="flex gap-1.5">
                      <Button
                        className="flex-1 min-h-8"
                        color="primary"
                        size="sm"
                        startContent={<EditIcon className="w-3 h-3" />}
                        variant="flat"
                        onPress={() => handleEdit(user)}
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
                              d="M4 2a1 1 0 011 1v2.101a7.002 7.002 0 0111.601 2.566 1 1 0 11-1.885.666A5.002 5.002 0 005.999 7H9a1 1 0 010 2H4a1 1 0 01-1-1V3a1 1 0 011-1zm.008 9.057a1 1 0 011.276.61A5.002 5.002 0 0014.001 13H11a1 1 0 110-2h5a1 1 0 011 1v5a1 1 0 11-2 0v-2.101a7.002 7.002 0 01-11.601-2.566 1 1 0 01.61-1.276z"
                              fillRule="evenodd"
                            />
                          </svg>
                        }
                        variant="flat"
                        onPress={() => handleResetFlow(user)}
                      >
                        重置
                      </Button>
                    </div>

                    {/* 第二行：权限和删除 */}
                    <div className="flex gap-1.5">
                      <Button
                        className="flex-1 min-h-8"
                        color="success"
                        size="sm"
                        startContent={<SettingsIcon className="w-3 h-3" />}
                        variant="flat"
                        onPress={() => handleManageTunnels(user)}
                      >
                        权限
                      </Button>
                      <Button
                        className="flex-1 min-h-8"
                        color="danger"
                        size="sm"
                        startContent={<DeleteIcon className="w-3 h-3" />}
                        variant="flat"
                        onPress={() => handleDelete(user)}
                      >
                        删除
                      </Button>
                    </div>
                  </div>
                </CardBody>
              </Card>
            );
          })}
        </div>
      )}

      {/* 用户表单模态框 */}
      <Modal
        backdrop="blur"
        isOpen={isUserModalOpen}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onClose={onUserModalClose}
      >
        <ModalContent>
          <ModalHeader>{isEdit ? "编辑用户" : "新增用户"}</ModalHeader>
          <ModalBody>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Input
                isRequired
                label="用户名"
                value={userForm.user}
                onChange={(e) =>
                  setUserForm((prev) => ({ ...prev, user: e.target.value }))
                }
              />
              <Input
                isRequired={!isEdit}
                label="密码"
                placeholder={isEdit ? "留空则不修改密码" : "请输入密码"}
                type="password"
                value={userForm.pwd}
                onChange={(e) =>
                  setUserForm((prev) => ({ ...prev, pwd: e.target.value }))
                }
              />
              <Input
                isRequired
                label="流量限制(GB)"
                max="99999"
                min="1"
                type="number"
                value={userForm.flow.toString()}
                onChange={(e) => {
                  const value = Math.min(
                    Math.max(Number(e.target.value) || 0, 1),
                    99999,
                  );

                  setUserForm((prev) => ({ ...prev, flow: value }));
                }}
              />
              <Input
                isRequired
                label="转发数量"
                max="99999"
                min="1"
                type="number"
                value={userForm.num.toString()}
                onChange={(e) => {
                  const value = Math.min(
                    Math.max(Number(e.target.value) || 0, 1),
                    99999,
                  );

                  setUserForm((prev) => ({ ...prev, num: value }));
                }}
              />
              <Select
                label="流量重置日期"
                selectedKeys={[userForm.flowResetTime.toString()]}
                onSelectionChange={(keys) => {
                  const value = Array.from(keys)[0] as string;

                  setUserForm((prev) => ({
                    ...prev,
                    flowResetTime: Number(value),
                  }));
                }}
              >
                <>
                  <SelectItem key="0" textValue="不重置">
                    不重置
                  </SelectItem>
                  {Array.from({ length: 31 }, (_, i) => i + 1).map((day) => (
                    <SelectItem
                      key={day.toString()}
                      textValue={`每月${day}号（0点重置）`}
                    >
                      每月{day}号（0点重置）
                    </SelectItem>
                  ))}
                </>
              </Select>
              <DatePicker
                isRequired
                showMonthAndYearPickers
                label="过期时间"
                value={
                  userForm.expTime
                    ? (parseDate(
                      userForm.expTime.toISOString().split("T")[0],
                    ) as any)
                    : null
                }
                onChange={(date) => {
                  if (date) {
                    const jsDate = new Date(
                      date.year,
                      date.month - 1,
                      date.day,
                      23,
                      59,
                      59,
                    );

                    setUserForm((prev) => ({ ...prev, expTime: jsDate }));
                  } else {
                    setUserForm((prev) => ({ ...prev, expTime: null }));
                  }
                }}
              />
            </div>

            <RadioGroup
              label="状态"
              orientation="horizontal"
              value={userForm.status.toString()}
              onValueChange={(value: string) =>
                setUserForm((prev) => ({ ...prev, status: Number(value) }))
              }
            >
              <Radio value="1">正常</Radio>
              <Radio value="0">禁用</Radio>
            </RadioGroup>

            {userGroups.length > 0 && (
              <Select
                label="用户分组（可选）"
                placeholder="选择要加入的分组"
                selectedKeys={new Set((userForm.groupIds ?? []).map(String))}
                selectionMode="multiple"
                onSelectionChange={(keys) => {
                  const selected = Array.from(keys as Set<string>).map(Number);

                  setUserForm((prev) => ({ ...prev, groupIds: selected }));
                }}
              >
                {userGroups.map((g) => (
                  <SelectItem key={g.id.toString()} textValue={g.name}>
                    {g.name}
                  </SelectItem>
                ))}
              </Select>
            )}
          </ModalBody>
          <ModalFooter>
            <Button onPress={onUserModalClose}>取消</Button>
            <Button
              color="primary"
              isLoading={userFormLoading}
              onPress={handleSubmitUser}
            >
              确定
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* 隧道权限管理模态框 */}
      <Modal
        backdrop="blur"
        classNames={{
          base: "max-w-[95vw] sm:max-w-4xl",
        }}
        isDismissable={false}
        isOpen={isTunnelModalOpen}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onClose={onTunnelModalClose}
      >
        <ModalContent>
          <ModalHeader>用户 {currentUser?.user} 的隧道权限管理</ModalHeader>
          <ModalBody>
            <div className="space-y-6">
              {/* 分配新权限部分 */}
              <div>
                <h3 className="text-lg font-semibold mb-4">分配新权限</h3>
                <div className="space-y-4">
                  <div className="text-sm text-default-500 bg-default-100 dark:bg-default-50 p-3 rounded-lg border border-default-200 dark:border-default-100/30">
                    流量限制、转发数量、到期时间、流量重置时间将自动继承用户设置
                  </div>

                  <div className="grid gap-2 max-h-72 overflow-y-auto pr-1">
                    {tunnels.map((tunnel) => {
                      const isAssigned = isTunnelAssigned(tunnel.id);
                      const isSelected = batchTunnelSelections.has(tunnel.id);
                      const tunnelSpeedLimits = getSpeedLimitsForTunnel(
                        tunnel.id,
                      );

                      return (
                        <div
                          key={tunnel.id}
                          aria-disabled={isAssigned}
                          className={`
                            px-4 py-3 rounded-lg border transition-all duration-200 cursor-pointer
                            ${isAssigned
                              ? "bg-default-100/50 dark:bg-default-50/50 border-default-200/50 dark:border-default-100/20 opacity-60 cursor-not-allowed"
                              : isSelected
                                ? "bg-primary-50 dark:bg-primary-900/20 border-primary-300 dark:border-primary-500/50 shadow-sm"
                                : "bg-white dark:bg-default-50 border-default-200 dark:border-default-100/30 hover:border-primary-200 dark:hover:border-primary-500/30 hover:shadow-sm"
                            }
                          `}
                          role="button"
                          tabIndex={isAssigned ? -1 : 0}
                          onClick={() =>
                            !isAssigned && toggleTunnelSelection(tunnel.id)
                          }
                          onKeyDown={(event) => {
                            if (isAssigned) {
                              return;
                            }

                            if (event.key === "Enter" || event.key === " ") {
                              event.preventDefault();
                              toggleTunnelSelection(tunnel.id);
                            }
                          }}
                        >
                          <div className="flex items-center justify-between gap-4">
                            <div className="flex items-center gap-3 flex-1 min-w-0">
                              <Checkbox
                                color="primary"
                                isDisabled={isAssigned}
                                isSelected={isSelected}
                                size="md"
                                onClick={(event) => event.stopPropagation()}
                                onKeyDown={(event) => event.stopPropagation()}
                                onValueChange={() =>
                                  toggleTunnelSelection(tunnel.id)
                                }
                              />
                              <span
                                className={`font-medium truncate ${isAssigned ? "text-default-400" : "text-default-700 dark:text-default-600"}`}
                              >
                                {tunnel.name}
                              </span>
                              {isAssigned && (
                                <Chip
                                  className="shrink-0"
                                  color="default"
                                  size="sm"
                                  variant="flat"
                                >
                                  已分配
                                </Chip>
                              )}
                            </div>

                            {isSelected && !isAssigned && (
                              <div>
                                <Select
                                  className="w-36"
                                  classNames={{
                                    trigger: "min-h-10 h-10",
                                  }}
                                  placeholder="限速规则"
                                  selectedKeys={
                                    batchTunnelSelections.get(tunnel.id) !==
                                      null &&
                                      batchTunnelSelections.get(tunnel.id) !==
                                      undefined
                                      ? [
                                        batchTunnelSelections
                                          .get(tunnel.id)!
                                          .toString(),
                                      ]
                                      : ["null"]
                                  }
                                  size="sm"
                                  onClick={(e) => e.stopPropagation()}
                                  onSelectionChange={(keys) => {
                                    const value = Array.from(keys)[0] as string;

                                    updateTunnelSpeedLimit(
                                      tunnel.id,
                                      value === "null" ? null : Number(value),
                                    );
                                  }}
                                >
                                  {[
                                    <SelectItem key="null" textValue="不限速">
                                      不限速
                                    </SelectItem>,
                                    ...tunnelSpeedLimits.map((sl) => (
                                      <SelectItem
                                        key={sl.id.toString()}
                                        textValue={sl.name}
                                      >
                                        {sl.name}
                                      </SelectItem>
                                    )),
                                  ]}
                                </Select>
                              </div>
                            )}
                          </div>
                        </div>
                      );
                    })}
                    {tunnels.length === 0 && (
                      <div className="p-8 text-center text-default-400 bg-default-50 dark:bg-default-100/50 rounded-lg border border-dashed border-default-200 dark:border-default-100/30">
                        暂无可用隧道
                      </div>
                    )}
                  </div>

                  <div className="flex flex-wrap items-center gap-2">
                    <Button
                      className="w-full sm:w-auto"
                      color="primary"
                      isDisabled={batchTunnelSelections.size === 0}
                      isLoading={assignLoading}
                      onPress={handleBatchAssignTunnel}
                    >
                      分配权限
                    </Button>
                    {batchTunnelSelections.size > 0 && (
                      <Chip color="primary" size="sm" variant="flat">
                        已选 {batchTunnelSelections.size} 个隧道
                      </Chip>
                    )}
                  </div>
                </div>
              </div>

              {/* 已有权限部分 */}
              <div>
                <h3 className="text-lg font-semibold mb-4">已有权限</h3>
                <Table
                  aria-label="用户隧道权限列表"
                  classNames={{
                    wrapper: "shadow-none",
                    th: "bg-gray-50 dark:bg-gray-800 text-gray-700 dark:text-gray-300 font-medium",
                  }}
                >
                  <TableHeader>
                    <TableColumn>隧道名称</TableColumn>
                    <TableColumn>流量统计</TableColumn>
                    <TableColumn>转发数量</TableColumn>
                    <TableColumn>状态</TableColumn>
                    <TableColumn>限速规则</TableColumn>
                    <TableColumn>重置时间</TableColumn>
                    <TableColumn>到期时间</TableColumn>
                    <TableColumn>操作</TableColumn>
                  </TableHeader>
                  <TableBody
                    emptyContent="暂无隧道权限"
                    isLoading={tunnelListLoading}
                    items={userTunnels}
                    loadingContent={<Spinner />}
                  >
                    {(userTunnel) => (
                      <TableRow key={userTunnel.id}>
                        <TableCell>{userTunnel.tunnelName}</TableCell>
                        <TableCell>
                          <div className="flex flex-col gap-1">
                            <div className="flex justify-between text-small">
                              <span className="text-gray-600">限制:</span>
                              <span className="font-medium">
                                {formatFlow(userTunnel.flow, "gb")}
                              </span>
                            </div>
                            <div className="flex justify-between text-small">
                              <span className="text-gray-600">已用:</span>
                              <span className="font-medium text-danger">
                                {formatFlow(
                                  calculateTunnelUsedFlow(userTunnel),
                                )}
                              </span>
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>{userTunnel.num}</TableCell>
                        <TableCell>
                          <Chip
                            color={
                              userTunnel.status === 1 ? "success" : "danger"
                            }
                            size="sm"
                            variant="flat"
                          >
                            {userTunnel.status === 1 ? "正常" : "禁用"}
                          </Chip>
                        </TableCell>
                        <TableCell>
                          <Chip
                            color={
                              userTunnel.speedLimitName ? "warning" : "success"
                            }
                            size="sm"
                            variant="flat"
                          >
                            {userTunnel.speedLimitName || "不限速"}
                          </Chip>
                        </TableCell>
                        <TableCell>
                          {userTunnel.flowResetTime === 0
                            ? "不重置"
                            : `每月${userTunnel.flowResetTime}号`}
                        </TableCell>
                        <TableCell>{formatDate(userTunnel.expTime)}</TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <Button
                              isIconOnly
                              aria-label="编辑隧道权限"
                              color="primary"
                              size="sm"
                              variant="flat"
                              onPress={() => handleEditTunnel(userTunnel)}
                            >
                              <EditIcon className="w-4 h-4" />
                            </Button>
                            <Button
                              isIconOnly
                              aria-label="重置隧道流量"
                              color="warning"
                              size="sm"
                              title="重置流量"
                              variant="flat"
                              onPress={() => handleResetTunnelFlow(userTunnel)}
                            >
                              <svg
                                aria-hidden="true"
                                className="w-4 h-4"
                                fill="currentColor"
                                viewBox="0 0 20 20"
                              >
                                <path
                                  clipRule="evenodd"
                                  d="M4 2a1 1 0 011 1v2.101a7.002 7.002 0 0111.601 2.566 1 1 0 11-1.885.666A5.002 5.002 0 005.999 7H9a1 1 0 010 2H4a1 1 0 01-1-1V3a1 1 0 011-1zm.008 9.057a1 1 0 011.276.61A5.002 5.002 0 0014.001 13H11a1 1 0 110-2h5a1 1 0 011 1v5a1 1 0 11-2 0v-2.101a7.002 7.002 0 01-11.601-2.566 1 1 0 01.61-1.276z"
                                  fillRule="evenodd"
                                />
                              </svg>
                            </Button>
                            <Button
                              isIconOnly
                              aria-label="删除隧道权限"
                              color="danger"
                              size="sm"
                              variant="flat"
                              onPress={() => handleRemoveTunnel(userTunnel)}
                            >
                              <DeleteIcon className="w-4 h-4" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </div>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button onPress={onTunnelModalClose}>关闭</Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* 编辑隧道权限模态框 */}
      <Modal
        backdrop="blur"
        isDismissable={false}
        isOpen={isEditTunnelModalOpen}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onClose={onEditTunnelModalClose}
      >
        <ModalContent>
          <ModalHeader>编辑隧道权限 - {editTunnelForm?.tunnelName}</ModalHeader>
          <ModalBody>
            {editTunnelForm && (
              <>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <Input
                    label="流量限制(GB)"
                    max="99999"
                    min="1"
                    type="number"
                    value={editTunnelForm.flow.toString()}
                    onChange={(e) => {
                      const value = Math.min(
                        Math.max(Number(e.target.value) || 0, 1),
                        99999,
                      );

                      setEditTunnelForm((prev) =>
                        prev ? { ...prev, flow: value } : null,
                      );
                    }}
                  />

                  <Input
                    label="转发数量"
                    max="99999"
                    min="1"
                    type="number"
                    value={editTunnelForm.num.toString()}
                    onChange={(e) => {
                      const value = Math.min(
                        Math.max(Number(e.target.value) || 0, 1),
                        99999,
                      );

                      setEditTunnelForm((prev) =>
                        prev ? { ...prev, num: value } : null,
                      );
                    }}
                  />

                  <Select
                    label="限速规则"
                    selectedKeys={
                      editTunnelForm.speedId
                        ? [editTunnelForm.speedId.toString()]
                        : ["null"]
                    }
                    onSelectionChange={(keys) => {
                      const value = Array.from(keys)[0] as string;

                      setEditTunnelForm((prev) =>
                        prev
                          ? {
                            ...prev,
                            speedId: value === "null" ? null : Number(value),
                          }
                          : null,
                      );
                    }}
                  >
                    {[
                      <SelectItem key="null" textValue="不限速">
                        不限速
                      </SelectItem>,
                      ...editAvailableSpeedLimits.map((speedLimit) => (
                        <SelectItem
                          key={speedLimit.id.toString()}
                          textValue={speedLimit.name}
                        >
                          {speedLimit.name}
                        </SelectItem>
                      )),
                    ]}
                  </Select>

                  <Select
                    label="流量重置日期"
                    selectedKeys={[editTunnelForm.flowResetTime.toString()]}
                    onSelectionChange={(keys) => {
                      const value = Array.from(keys)[0] as string;

                      setEditTunnelForm((prev) =>
                        prev ? { ...prev, flowResetTime: Number(value) } : null,
                      );
                    }}
                  >
                    <>
                      <SelectItem key="0" textValue="不重置">
                        不重置
                      </SelectItem>
                      {Array.from({ length: 31 }, (_, i) => i + 1).map(
                        (day) => (
                          <SelectItem
                            key={day.toString()}
                            textValue={`每月${day}号（0点重置）`}
                          >
                            每月{day}号（0点重置）
                          </SelectItem>
                        ),
                      )}
                    </>
                  </Select>

                  <DatePicker
                    isRequired
                    showMonthAndYearPickers
                    label="到期时间"
                    value={
                      editTunnelForm.expTime
                        ? (parseDate(
                          new Date(editTunnelForm.expTime)
                            .toISOString()
                            .split("T")[0],
                        ) as any)
                        : null
                    }
                    onChange={(date) => {
                      if (date) {
                        const jsDate = new Date(
                          date.year,
                          date.month - 1,
                          date.day,
                          23,
                          59,
                          59,
                        );

                        setEditTunnelForm((prev) =>
                          prev ? { ...prev, expTime: jsDate.getTime() } : null,
                        );
                      } else {
                        setEditTunnelForm((prev) =>
                          prev ? { ...prev, expTime: Date.now() } : null,
                        );
                      }
                    }}
                  />
                </div>

                <RadioGroup
                  label="状态"
                  orientation="horizontal"
                  value={editTunnelForm.status.toString()}
                  onValueChange={(value: string) =>
                    setEditTunnelForm((prev) =>
                      prev ? { ...prev, status: Number(value) } : null,
                    )
                  }
                >
                  <Radio value="1">正常</Radio>
                  <Radio value="0">禁用</Radio>
                </RadioGroup>
              </>
            )}
          </ModalBody>
          <ModalFooter>
            <Button onPress={onEditTunnelModalClose}>取消</Button>
            <Button
              color="primary"
              isLoading={editTunnelLoading}
              onPress={handleUpdateTunnel}
            >
              确定
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* 删除确认对话框 */}
      <Modal
        backdrop="blur"
        isOpen={isDeleteModalOpen}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onClose={onDeleteModalClose}
      >
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">
            确认删除用户
          </ModalHeader>
          <ModalBody>
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 bg-danger-100 rounded-full flex items-center justify-center">
                <DeleteIcon className="w-6 h-6 text-danger" />
              </div>
              <div className="flex-1">
                <p className="text-foreground">
                  确定要删除用户{" "}
                  <span className="font-semibold text-danger">
                    &quot;{userToDelete?.user}&quot;
                  </span>{" "}
                  吗？
                </p>
                <p className="text-small text-default-500 mt-1">
                  此操作不可撤销，用户的所有数据将被永久删除。
                </p>
              </div>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onDeleteModalClose}>
              取消
            </Button>
            <Button color="danger" onPress={handleConfirmDelete}>
              确认删除
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* 删除隧道权限确认对话框 */}
      <Modal
        backdrop="blur"
        isOpen={isDeleteTunnelModalOpen}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onClose={onDeleteTunnelModalClose}
      >
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">
            确认删除隧道权限
          </ModalHeader>
          <ModalBody>
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 bg-danger-100 rounded-full flex items-center justify-center">
                <DeleteIcon className="w-6 h-6 text-danger" />
              </div>
              <div className="flex-1">
                <p className="text-foreground">
                  确定要删除用户{" "}
                  <span className="font-semibold">{currentUser?.user}</span>{" "}
                  对隧道{" "}
                  <span className="font-semibold text-danger">
                    &quot;{tunnelToDelete?.tunnelName}&quot;
                  </span>{" "}
                  的权限吗？
                </p>
                <p className="text-small text-default-500 mt-1">
                  删除后该用户将无法使用此隧道创建转发，此操作不可撤销。
                </p>
              </div>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onDeleteTunnelModalClose}>
              取消
            </Button>
            <Button color="danger" onPress={handleConfirmRemoveTunnel}>
              确认删除
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* 重置流量确认对话框 */}
      <Modal
        backdrop="blur"
        isOpen={isResetFlowModalOpen}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onClose={onResetFlowModalClose}
      >
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">
            确认重置流量
          </ModalHeader>
          <ModalBody>
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 bg-warning-100 rounded-full flex items-center justify-center">
                <svg
                  aria-hidden="true"
                  className="w-6 h-6 text-warning"
                  fill="currentColor"
                  viewBox="0 0 20 20"
                >
                  <path
                    clipRule="evenodd"
                    d="M4 2a1 1 0 011 1v2.101a7.002 7.002 0 0111.601 2.566 1 1 0 11-1.885.666A5.002 5.002 0 005.999 7H9a1 1 0 010 2H4a1 1 0 01-1-1V3a1 1 0 011-1zm.008 9.057a1 1 0 011.276.61A5.002 5.002 0 0014.001 13H11a1 1 0 110-2h5a1 1 0 011 1v5a1 1 0 11-2 0v-2.101a7.002 7.002 0 01-11.601-2.566 1 1 0 01.61-1.276z"
                    fillRule="evenodd"
                  />
                </svg>
              </div>
              <div className="flex-1">
                <p className="text-foreground">
                  确定要重置用户{" "}
                  <span className="font-semibold text-warning">
                    &quot;{userToReset?.user}&quot;
                  </span>{" "}
                  的流量吗？
                </p>
                <p className="text-small text-default-500 mt-1">
                  该操作只会重置账号流量不会重置隧道权限流量，重置后该用户的上下行流量将归零，此操作不可撤销。
                </p>
                <div className="mt-2 p-2 bg-warning-50 dark:bg-warning-100/10 rounded text-xs">
                  <div className="text-warning-700 dark:text-warning-300">
                    当前流量使用情况：
                  </div>
                  <div className="mt-1 space-y-1">
                    <div className="flex justify-between">
                      <span>上行流量：</span>
                      <span className="font-mono">
                        {userToReset
                          ? formatFlow(userToReset.inFlow || 0)
                          : "-"}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>下行流量：</span>
                      <span className="font-mono">
                        {userToReset
                          ? formatFlow(userToReset.outFlow || 0)
                          : "-"}
                      </span>
                    </div>
                    <div className="flex justify-between font-medium">
                      <span>总计：</span>
                      <span className="font-mono text-warning-700 dark:text-warning-300">
                        {userToReset
                          ? formatFlow(calculateUserTotalUsedFlow(userToReset))
                          : "-"}
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onResetFlowModalClose}>
              取消
            </Button>
            <Button
              color="warning"
              isLoading={resetFlowLoading}
              onPress={handleConfirmResetFlow}
            >
              确认重置
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* 重置隧道流量确认对话框 */}
      <Modal
        backdrop="blur"
        isOpen={isResetTunnelFlowModalOpen}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onClose={onResetTunnelFlowModalClose}
      >
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">
            确认重置隧道流量
          </ModalHeader>
          <ModalBody>
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 bg-warning-100 rounded-full flex items-center justify-center">
                <svg
                  aria-hidden="true"
                  className="w-6 h-6 text-warning"
                  fill="currentColor"
                  viewBox="0 0 20 20"
                >
                  <path
                    clipRule="evenodd"
                    d="M4 2a1 1 0 011 1v2.101a7.002 7.002 0 0111.601 2.566 1 1 0 11-1.885.666A5.002 5.002 0 005.999 7H9a1 1 0 010 2H4a1 1 0 01-1-1V3a1 1 0 011-1zm.008 9.057a1 1 0 011.276.61A5.002 5.002 0 0014.001 13H11a1 1 0 110-2h5a1 1 0 011 1v5a1 1 0 11-2 0v-2.101a7.002 7.002 0 01-11.601-2.566 1 1 0 01.61-1.276z"
                    fillRule="evenodd"
                  />
                </svg>
              </div>
              <div className="flex-1">
                <p className="text-foreground">
                  确定要重置用户{" "}
                  <span className="font-semibold">{currentUser?.user}</span>{" "}
                  对隧道{" "}
                  <span className="font-semibold text-warning">
                    &quot;{tunnelToReset?.tunnelName}&quot;
                  </span>{" "}
                  的流量吗？
                </p>
                <p className="text-small text-default-500 mt-1">
                  该操作只会重置隧道权限流量不会重置账号流量，重置后该隧道权限的上下行流量将归零，此操作不可撤销。
                </p>
                <div className="mt-2 p-2 bg-warning-50 dark:bg-warning-100/10 rounded text-xs">
                  <div className="text-warning-700 dark:text-warning-300">
                    当前流量使用情况：
                  </div>
                  <div className="mt-1 space-y-1">
                    <div className="flex justify-between">
                      <span>上行流量：</span>
                      <span className="font-mono">
                        {tunnelToReset
                          ? formatFlow(tunnelToReset.inFlow || 0)
                          : "-"}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>下行流量：</span>
                      <span className="font-mono">
                        {tunnelToReset
                          ? formatFlow(tunnelToReset.outFlow || 0)
                          : "-"}
                      </span>
                    </div>
                    <div className="flex justify-between font-medium">
                      <span>总计：</span>
                      <span className="font-mono text-warning-700 dark:text-warning-300">
                        {tunnelToReset
                          ? formatFlow(calculateTunnelUsedFlow(tunnelToReset))
                          : "-"}
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onResetTunnelFlowModalClose}>
              取消
            </Button>
            <Button
              color="warning"
              isLoading={resetTunnelFlowLoading}
              onPress={handleConfirmResetTunnelFlow}
            >
              确认重置
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
