import { useCallback, useEffect, useMemo, useState } from "react";
import { Card, CardBody, CardHeader } from "@/shadcn-bridge/heroui/card";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Input } from "@/shadcn-bridge/heroui/input";
import {
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  useDisclosure,
} from "@/shadcn-bridge/heroui/modal";
import { Select, SelectItem } from "@/shadcn-bridge/heroui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableColumn,
  TableHeader,
  TableRow,
} from "@/shadcn-bridge/heroui/table";
import { Chip } from "@/shadcn-bridge/heroui/chip";
import { Spinner } from "@/shadcn-bridge/heroui/spinner";
import toast from "react-hot-toast";

import {
  assignGroupPermission,
  assignTunnelsToGroup,
  assignUsersToGroup,
  createTunnelGroup,
  createUserGroup,
  deleteTunnelGroup,
  deleteUserGroup,
  getAllUsers,
  getGroupPermissionList,
  getTunnelGroupList,
  getTunnelList,
  getUserGroupList,
  removeGroupPermission,
  updateTunnelGroup,
  updateUserGroup,
} from "@/api";
import { getAdminFlag } from "@/utils/session";

interface TunnelItem {
  id: number;
  name: string;
}

interface UserItem {
  id: number;
  user: string;
}

interface TunnelGroup {
  id: number;
  name: string;
  status: number;
  tunnelIds: number[];
  tunnelNames: string[];
  createdTime: number;
}

interface UserGroup {
  id: number;
  name: string;
  status: number;
  userIds: number[];
  userNames: string[];
  createdTime: number;
}

interface GroupPermission {
  id: number;
  userGroupId: number;
  userGroupName: string;
  tunnelGroupId: number;
  tunnelGroupName: string;
  createdTime: number;
}

const formatDate = (timestamp?: number): string => {
  if (!timestamp) {
    return "-";
  }

  return new Date(timestamp).toLocaleString();
};

export default function GroupPage() {
  const [loading, setLoading] = useState(true);
  const [isAdmin] = useState(getAdminFlag());

  const [tunnelGroups, setTunnelGroups] = useState<TunnelGroup[]>([]);
  const [userGroups, setUserGroups] = useState<UserGroup[]>([]);
  const [permissions, setPermissions] = useState<GroupPermission[]>([]);
  const [tunnels, setTunnels] = useState<TunnelItem[]>([]);
  const [users, setUsers] = useState<UserItem[]>([]);

  const [selectedUserGroupId, setSelectedUserGroupId] = useState<number | null>(
    null,
  );
  const [selectedTunnelGroupId, setSelectedTunnelGroupId] = useState<
    number | null
  >(null);

  const [savingPermission, setSavingPermission] = useState(false);

  const {
    isOpen: tunnelGroupModalOpen,
    onOpen: onTunnelGroupModalOpen,
    onClose: onTunnelGroupModalClose,
    onOpenChange: onTunnelGroupModalChange,
  } = useDisclosure();
  const {
    isOpen: userGroupModalOpen,
    onOpen: onUserGroupModalOpen,
    onClose: onUserGroupModalClose,
    onOpenChange: onUserGroupModalChange,
  } = useDisclosure();
  const {
    isOpen: tunnelAssignModalOpen,
    onOpen: onTunnelAssignModalOpen,
    onClose: onTunnelAssignModalClose,
    onOpenChange: onTunnelAssignModalChange,
  } = useDisclosure();
  const {
    isOpen: userAssignModalOpen,
    onOpen: onUserAssignModalOpen,
    onClose: onUserAssignModalClose,
    onOpenChange: onUserAssignModalChange,
  } = useDisclosure();

  const [editingTunnelGroup, setEditingTunnelGroup] =
    useState<TunnelGroup | null>(null);
  const [editingUserGroup, setEditingUserGroup] = useState<UserGroup | null>(
    null,
  );
  const [groupName, setGroupName] = useState("");
  const [groupStatus, setGroupStatus] = useState("1");
  const [savingGroup, setSavingGroup] = useState(false);

  const [assignTunnelGroup, setAssignTunnelGroup] =
    useState<TunnelGroup | null>(null);
  const [assignUserGroup, setAssignUserGroup] = useState<UserGroup | null>(
    null,
  );
  const [selectedTunnelKeys, setSelectedTunnelKeys] = useState<Set<string>>(
    new Set(),
  );
  const [selectedUserKeys, setSelectedUserKeys] = useState<Set<string>>(
    new Set(),
  );
  const [savingAssign, setSavingAssign] = useState(false);

  const tunnelNameMap = useMemo(() => {
    const map = new Map<number, string>();

    tunnels.forEach((item) => {
      map.set(item.id, item.name);
    });

    return map;
  }, [tunnels]);

  const userNameMap = useMemo(() => {
    const map = new Map<number, string>();

    users.forEach((item) => {
      map.set(item.id, item.user);
    });

    return map;
  }, [users]);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const [tunnelGroupRes, userGroupRes, permissionRes, tunnelRes, userRes] =
        await Promise.all([
          getTunnelGroupList(),
          getUserGroupList(),
          getGroupPermissionList(),
          getTunnelList(),
          getAllUsers(),
        ]);

      if (tunnelGroupRes.code === 0) {
        setTunnelGroups(
          Array.isArray(tunnelGroupRes.data) ? tunnelGroupRes.data : [],
        );
      }
      if (userGroupRes.code === 0) {
        setUserGroups(
          Array.isArray(userGroupRes.data) ? userGroupRes.data : [],
        );
      }
      if (permissionRes.code === 0) {
        setPermissions(
          Array.isArray(permissionRes.data) ? permissionRes.data : [],
        );
      }
      if (tunnelRes.code === 0) {
        setTunnels(Array.isArray(tunnelRes.data) ? tunnelRes.data : []);
      }
      if (userRes.code === 0) {
        setUsers(Array.isArray(userRes.data) ? userRes.data : []);
      }

      if (
        tunnelGroupRes.code !== 0 ||
        userGroupRes.code !== 0 ||
        permissionRes.code !== 0
      ) {
        toast.error("部分分组数据加载失败");
      }
    } catch {
      toast.error("分组数据加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const openCreateTunnelGroup = () => {
    setEditingTunnelGroup(null);
    setGroupName("");
    setGroupStatus("1");
    onTunnelGroupModalOpen();
  };

  const openEditTunnelGroup = (group: TunnelGroup) => {
    setEditingTunnelGroup(group);
    setGroupName(group.name);
    setGroupStatus(String(group.status));
    onTunnelGroupModalOpen();
  };

  const openCreateUserGroup = () => {
    setEditingUserGroup(null);
    setGroupName("");
    setGroupStatus("1");
    onUserGroupModalOpen();
  };

  const openEditUserGroup = (group: UserGroup) => {
    setEditingUserGroup(group);
    setGroupName(group.name);
    setGroupStatus(String(group.status));
    onUserGroupModalOpen();
  };

  const saveTunnelGroup = async () => {
    if (!groupName.trim()) {
      toast.error("请输入分组名称");

      return;
    }
    setSavingGroup(true);
    try {
      const payload = { name: groupName.trim(), status: Number(groupStatus) };
      const res = editingTunnelGroup
        ? await updateTunnelGroup({ id: editingTunnelGroup.id, ...payload })
        : await createTunnelGroup(payload);

      if (res.code === 0) {
        toast.success(editingTunnelGroup ? "更新成功" : "创建成功");
        onTunnelGroupModalClose();
        loadData();
      } else {
        toast.error(res.msg || "保存失败");
      }
    } catch {
      toast.error("保存失败");
    } finally {
      setSavingGroup(false);
    }
  };

  const saveUserGroup = async () => {
    if (!groupName.trim()) {
      toast.error("请输入分组名称");

      return;
    }
    setSavingGroup(true);
    try {
      const payload = { name: groupName.trim(), status: Number(groupStatus) };
      const res = editingUserGroup
        ? await updateUserGroup({ id: editingUserGroup.id, ...payload })
        : await createUserGroup(payload);

      if (res.code === 0) {
        toast.success(editingUserGroup ? "更新成功" : "创建成功");
        onUserGroupModalClose();
        loadData();
      } else {
        toast.error(res.msg || "保存失败");
      }
    } catch {
      toast.error("保存失败");
    } finally {
      setSavingGroup(false);
    }
  };

  const handleDeleteTunnelGroup = async (id: number) => {
    try {
      const res = await deleteTunnelGroup(id);

      if (res.code === 0) {
        toast.success("删除成功");
        loadData();
      } else {
        toast.error(res.msg || "删除失败");
      }
    } catch {
      toast.error("删除失败");
    }
  };

  const handleDeleteUserGroup = async (id: number) => {
    try {
      const res = await deleteUserGroup(id);

      if (res.code === 0) {
        toast.success("删除成功");
        loadData();
      } else {
        toast.error(res.msg || "删除失败");
      }
    } catch {
      toast.error("删除失败");
    }
  };

  const openAssignTunnels = (group: TunnelGroup) => {
    setAssignTunnelGroup(group);
    setSelectedTunnelKeys(new Set(group.tunnelIds.map((id) => String(id))));
    onTunnelAssignModalOpen();
  };

  const openAssignUsers = (group: UserGroup) => {
    setAssignUserGroup(group);
    setSelectedUserKeys(new Set(group.userIds.map((id) => String(id))));
    onUserAssignModalOpen();
  };

  const saveAssignTunnels = async () => {
    if (!assignTunnelGroup) return;
    setSavingAssign(true);
    try {
      const tunnelIds = Array.from(selectedTunnelKeys).map((id) => Number(id));
      const res = await assignTunnelsToGroup({
        groupId: assignTunnelGroup.id,
        tunnelIds,
      });

      if (res.code === 0) {
        toast.success("分配成功");
        onTunnelAssignModalClose();
        loadData();
      } else {
        toast.error(res.msg || "分配失败");
      }
    } catch {
      toast.error("分配失败");
    } finally {
      setSavingAssign(false);
    }
  };

  const saveAssignUsers = async () => {
    if (!assignUserGroup) return;
    setSavingAssign(true);
    try {
      const userIds = Array.from(selectedUserKeys).map((id) => Number(id));
      const res = await assignUsersToGroup({
        groupId: assignUserGroup.id,
        userIds,
      });

      if (res.code === 0) {
        toast.success("分配成功");
        onUserAssignModalClose();
        loadData();
      } else {
        toast.error(res.msg || "分配失败");
      }
    } catch {
      toast.error("分配失败");
    } finally {
      setSavingAssign(false);
    }
  };

  const handleAssignPermission = async () => {
    if (!selectedUserGroupId || !selectedTunnelGroupId) {
      toast.error("请选择用户分组和隧道分组");

      return;
    }
    setSavingPermission(true);
    try {
      const res = await assignGroupPermission({
        userGroupId: selectedUserGroupId,
        tunnelGroupId: selectedTunnelGroupId,
      });

      if (res.code === 0) {
        toast.success(res.msg || "权限分配成功");
        loadData();
      } else {
        toast.error(res.msg || "权限分配失败");
      }
    } catch {
      toast.error("权限分配失败");
    } finally {
      setSavingPermission(false);
    }
  };

  const handleRemovePermission = async (id: number) => {
    try {
      const res = await removeGroupPermission(id);

      if (res.code === 0) {
        toast.success("权限回收成功");
        loadData();
      } else {
        toast.error(res.msg || "权限回收失败");
      }
    } catch {
      toast.error("权限回收失败");
    }
  };

  if (!isAdmin) {
    return (
      <div className="px-3 lg:px-6 py-8">
        <Card>
          <CardBody>
            <p className="text-danger">
              权限不足，只有管理员可以访问分组管理页面。
            </p>
          </CardBody>
        </Card>
      </div>
    );
  }

  return (
    <div className="px-3 lg:px-6 py-8 space-y-6">
      {loading && (
        <div className="flex justify-center py-10">
          <Spinner size="lg" />
        </div>
      )}

      <Card>
        <CardHeader className="flex items-center justify-between">
          <h3 className="text-lg font-semibold">隧道分组</h3>
          <Button color="primary" size="sm" onPress={openCreateTunnelGroup}>
            新建隧道分组
          </Button>
        </CardHeader>
        <CardBody>
          <Table aria-label="隧道分组列表">
            <TableHeader>
              <TableColumn>名称</TableColumn>
              <TableColumn>隧道</TableColumn>
              <TableColumn>状态</TableColumn>
              <TableColumn>创建时间</TableColumn>
              <TableColumn>操作</TableColumn>
            </TableHeader>
            <TableBody emptyContent="暂无隧道分组" items={tunnelGroups}>
              {(item) => (
                <TableRow key={item.id}>
                  <TableCell>{item.name}</TableCell>
                  <TableCell>
                    {item.tunnelNames.length > 0
                      ? item.tunnelNames.join("、")
                      : "-"}
                  </TableCell>
                  <TableCell>
                    <Chip
                      color={item.status === 1 ? "success" : "danger"}
                      size="sm"
                    >
                      {item.status === 1 ? "启用" : "停用"}
                    </Chip>
                  </TableCell>
                  <TableCell>{formatDate(item.createdTime)}</TableCell>
                  <TableCell>
                    <div className="flex gap-2">
                      <Button
                        size="sm"
                        variant="flat"
                        onPress={() => openAssignTunnels(item)}
                      >
                        分配隧道
                      </Button>
                      <Button
                        size="sm"
                        variant="light"
                        onPress={() => openEditTunnelGroup(item)}
                      >
                        编辑
                      </Button>
                      <Button
                        color="danger"
                        size="sm"
                        variant="light"
                        onPress={() => handleDeleteTunnelGroup(item.id)}
                      >
                        删除
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardBody>
      </Card>

      <Card>
        <CardHeader className="flex items-center justify-between">
          <h3 className="text-lg font-semibold">用户分组</h3>
          <Button color="primary" size="sm" onPress={openCreateUserGroup}>
            新建用户分组
          </Button>
        </CardHeader>
        <CardBody>
          <Table aria-label="用户分组列表">
            <TableHeader>
              <TableColumn>名称</TableColumn>
              <TableColumn>用户</TableColumn>
              <TableColumn>状态</TableColumn>
              <TableColumn>创建时间</TableColumn>
              <TableColumn>操作</TableColumn>
            </TableHeader>
            <TableBody emptyContent="暂无用户分组" items={userGroups}>
              {(item) => (
                <TableRow key={item.id}>
                  <TableCell>{item.name}</TableCell>
                  <TableCell>
                    {item.userNames.length > 0
                      ? item.userNames.join("、")
                      : "-"}
                  </TableCell>
                  <TableCell>
                    <Chip
                      color={item.status === 1 ? "success" : "danger"}
                      size="sm"
                    >
                      {item.status === 1 ? "启用" : "停用"}
                    </Chip>
                  </TableCell>
                  <TableCell>{formatDate(item.createdTime)}</TableCell>
                  <TableCell>
                    <div className="flex gap-2">
                      <Button
                        size="sm"
                        variant="flat"
                        onPress={() => openAssignUsers(item)}
                      >
                        分配用户
                      </Button>
                      <Button
                        size="sm"
                        variant="light"
                        onPress={() => openEditUserGroup(item)}
                      >
                        编辑
                      </Button>
                      <Button
                        color="danger"
                        size="sm"
                        variant="light"
                        onPress={() => handleDeleteUserGroup(item.id)}
                      >
                        删除
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardBody>
      </Card>

      <Card>
        <CardHeader>
          <h3 className="text-lg font-semibold">权限分配</h3>
        </CardHeader>
        <CardBody className="space-y-4">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-3 md:items-end">
            <Select
              items={userGroups}
              label="用户分组"
              selectedKeys={
                selectedUserGroupId ? [String(selectedUserGroupId)] : []
              }
              onSelectionChange={(keys) => {
                const key = Array.from(keys as Set<React.Key>)[0];

                setSelectedUserGroupId(key ? Number(key) : null);
              }}
            >
              {(item) => <SelectItem key={item.id}>{item.name}</SelectItem>}
            </Select>
            <Select
              items={tunnelGroups}
              label="隧道分组"
              selectedKeys={
                selectedTunnelGroupId ? [String(selectedTunnelGroupId)] : []
              }
              onSelectionChange={(keys) => {
                const key = Array.from(keys as Set<React.Key>)[0];

                setSelectedTunnelGroupId(key ? Number(key) : null);
              }}
            >
              {(item) => <SelectItem key={item.id}>{item.name}</SelectItem>}
            </Select>
            <Button
              className="md:self-end"
              color="primary"
              isLoading={savingPermission}
              onPress={handleAssignPermission}
            >
              分配权限
            </Button>
          </div>

          <Table aria-label="分组权限列表">
            <TableHeader>
              <TableColumn>ID</TableColumn>
              <TableColumn>用户分组</TableColumn>
              <TableColumn>隧道分组</TableColumn>
              <TableColumn>创建时间</TableColumn>
              <TableColumn>操作</TableColumn>
            </TableHeader>
            <TableBody emptyContent="暂无权限分配记录" items={permissions}>
              {(item) => (
                <TableRow key={item.id}>
                  <TableCell>{item.id}</TableCell>
                  <TableCell>
                    {item.userGroupName || item.userGroupId}
                  </TableCell>
                  <TableCell>
                    {item.tunnelGroupName || item.tunnelGroupId}
                  </TableCell>
                  <TableCell>{formatDate(item.createdTime)}</TableCell>
                  <TableCell>
                    <Button
                      color="danger"
                      size="sm"
                      variant="light"
                      onPress={() => handleRemovePermission(item.id)}
                    >
                      回收
                    </Button>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardBody>
      </Card>

      <Modal
        isOpen={tunnelGroupModalOpen}
        onOpenChange={onTunnelGroupModalChange}
      >
        <ModalContent>
          <ModalHeader>
            {editingTunnelGroup ? "编辑隧道分组" : "新建隧道分组"}
          </ModalHeader>
          <ModalBody className="space-y-3">
            <Input
              label="分组名称"
              value={groupName}
              onChange={(e) => setGroupName(e.target.value)}
            />
            <Select
              label="状态"
              selectedKeys={[groupStatus]}
              onSelectionChange={(keys) => {
                const key = Array.from(keys as Set<React.Key>)[0];

                if (key) {
                  setGroupStatus(String(key));
                }
              }}
            >
              <SelectItem key="1">启用</SelectItem>
              <SelectItem key="0">停用</SelectItem>
            </Select>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onTunnelGroupModalClose}>
              取消
            </Button>
            <Button
              color="primary"
              isLoading={savingGroup}
              onPress={saveTunnelGroup}
            >
              保存
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      <Modal isOpen={userGroupModalOpen} onOpenChange={onUserGroupModalChange}>
        <ModalContent>
          <ModalHeader>
            {editingUserGroup ? "编辑用户分组" : "新建用户分组"}
          </ModalHeader>
          <ModalBody className="space-y-3">
            <Input
              label="分组名称"
              value={groupName}
              onChange={(e) => setGroupName(e.target.value)}
            />
            <Select
              label="状态"
              selectedKeys={[groupStatus]}
              onSelectionChange={(keys) => {
                const key = Array.from(keys as Set<React.Key>)[0];

                if (key) {
                  setGroupStatus(String(key));
                }
              }}
            >
              <SelectItem key="1">启用</SelectItem>
              <SelectItem key="0">停用</SelectItem>
            </Select>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onUserGroupModalClose}>
              取消
            </Button>
            <Button
              color="primary"
              isLoading={savingGroup}
              onPress={saveUserGroup}
            >
              保存
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      <Modal
        isOpen={tunnelAssignModalOpen}
        onOpenChange={onTunnelAssignModalChange}
      >
        <ModalContent>
          <ModalHeader>分配隧道 - {assignTunnelGroup?.name}</ModalHeader>
          <ModalBody>
            <Select
              items={tunnels}
              label="选择隧道"
              selectedKeys={selectedTunnelKeys}
              selectionMode="multiple"
              onSelectionChange={(keys) => {
                setSelectedTunnelKeys(
                  new Set(Array.from(keys as Set<React.Key>).map(String)),
                );
              }}
            >
              {(item) => <SelectItem key={item.id}>{item.name}</SelectItem>}
            </Select>
            <p className="text-xs text-default-500">
              当前已选：
              {Array.from(selectedTunnelKeys)
                .map((id) => tunnelNameMap.get(Number(id)) || id)
                .join("、") || "无"}
            </p>
            <p className="text-xs text-default-500">
              不选择任何隧道并保存将清空该分组成员。
            </p>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onTunnelAssignModalClose}>
              取消
            </Button>
            <Button
              color="primary"
              isLoading={savingAssign}
              onPress={saveAssignTunnels}
            >
              保存
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      <Modal
        isOpen={userAssignModalOpen}
        onOpenChange={onUserAssignModalChange}
      >
        <ModalContent>
          <ModalHeader>分配用户 - {assignUserGroup?.name}</ModalHeader>
          <ModalBody>
            <Select
              items={users}
              label="选择用户"
              selectedKeys={selectedUserKeys}
              selectionMode="multiple"
              onSelectionChange={(keys) => {
                setSelectedUserKeys(
                  new Set(Array.from(keys as Set<React.Key>).map(String)),
                );
              }}
            >
              {(item) => <SelectItem key={item.id}>{item.user}</SelectItem>}
            </Select>
            <p className="text-xs text-default-500">
              当前已选：
              {Array.from(selectedUserKeys)
                .map((id) => userNameMap.get(Number(id)) || id)
                .join("、") || "无"}
            </p>
            <p className="text-xs text-default-500">
              不选择任何用户并保存将清空该分组成员。
            </p>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onUserAssignModalClose}>
              取消
            </Button>
            <Button
              color="primary"
              isLoading={savingAssign}
              onPress={saveAssignUsers}
            >
              保存
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
