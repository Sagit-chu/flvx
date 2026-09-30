import { useEffect, useState } from "react";
import toast from "react-hot-toast";

import { Card, CardBody } from "@/shadcn-bridge/heroui/card";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Input } from "@/shadcn-bridge/heroui/input";
import {
  beginPasskeyRegistration,
  deletePasskey,
  finishPasskeyRegistration,
  getPasskeyStatus,
  listPasskeys,
  type PasskeyItem,
} from "@/api";
import { createPasskey } from "@/utils/passkey";

export function PasskeyManager() {
  const [enabled, setEnabled] = useState(false);
  const [items, setItems] = useState<PasskeyItem[]>([]);
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);

  const refresh = () =>
    listPasskeys()
      .then((res) => {
        if (res.code === 0) setItems(res.data || []);
      })
      .catch(() => setItems([]));

  useEffect(() => {
    if (
      !window.isSecureContext ||
      typeof window.PublicKeyCredential === "undefined"
    )
      return;
    getPasskeyStatus()
      .then((res) => {
        if (res.code === 0 && res.data.enabled) {
          setEnabled(true);
          void refresh();
        }
      })
      .catch(() => setEnabled(false));
  }, []);

  if (!enabled) return null;

  const register = async () => {
    if (!password) {
      toast.error("请输入当前密码");

      return;
    }
    setBusy(true);
    try {
      const begin = await beginPasskeyRegistration(password);

      if (begin.code !== 0) {
        toast.error(begin.msg || "无法绑定通行证密钥");

        return;
      }
      const credential = await createPasskey(begin.data.options);
      const finish = await finishPasskeyRegistration(
        begin.data.sessionId,
        credential,
        name.trim(),
      );

      if (finish.code !== 0) {
        toast.error(finish.msg || "绑定失败");

        return;
      }
      toast.success("通行证密钥已绑定");
      setPassword("");
      setName("");
      await refresh();
    } catch {
      toast.error("绑定已取消或失败");
    } finally {
      setBusy(false);
    }
  };

  const remove = async (id: string) => {
    if (!password) {
      toast.error("请输入当前密码以删除密钥");

      return;
    }
    setBusy(true);
    try {
      const result = await deletePasskey(id, password);

      if (result.code !== 0) {
        toast.error(result.msg || "删除失败");

        return;
      }
      toast.success("通行证密钥已删除");
      setPassword("");
      await refresh();
    } catch {
      toast.error("删除失败");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardBody className="p-4 space-y-4">
        <div>
          <h3 className="text-base font-medium">通行证密钥</h3>
          <p className="text-sm text-default-500">
            绑定后可使用设备解锁登录，无需 Cloudflare
            验证。绑定和删除均需输入当前密码。
          </p>
        </div>
        <Input
          label="当前密码"
          type="password"
          value={password}
          variant="bordered"
          onChange={(e) => setPassword(e.target.value)}
        />
        <Input
          label="密钥名称（可选）"
          value={name}
          variant="bordered"
          onChange={(e) => setName(e.target.value)}
        />
        <Button disabled={busy} onPress={register}>
          绑定通行证密钥
        </Button>
        <div className="space-y-2">
          {items.map((item) => (
            <div
              key={item.id}
              className="flex items-center justify-between gap-3 rounded-lg border border-default-200 p-3"
            >
              <div>
                <div className="text-sm font-medium">{item.name}</div>
                <div className="text-xs text-default-500">
                  创建于 {new Date(item.createdAt).toLocaleDateString()}{" "}
                  {item.lastUsedAt
                    ? ` · 最近使用 ${new Date(item.lastUsedAt).toLocaleDateString()}`
                    : ""}
                </div>
              </div>
              <Button
                color="danger"
                disabled={busy}
                size="sm"
                variant="light"
                onPress={() => void remove(item.id)}
              >
                删除
              </Button>
            </div>
          ))}
        </div>
      </CardBody>
    </Card>
  );
}
