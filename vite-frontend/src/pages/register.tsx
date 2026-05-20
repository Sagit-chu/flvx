import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import toast from "react-hot-toast";
import { Turnstile } from "@marsidev/react-turnstile";
import { motion } from "framer-motion";

import { Button } from "@/shadcn-bridge/heroui/button";
import { Card, CardBody, CardHeader } from "@/shadcn-bridge/heroui/card";
import { Input } from "@/shadcn-bridge/heroui/input";
import {
  getPublicCommerceSettings,
  getPublicConfigByName,
  registerUser,
} from "@/api";
import { BrandLogo } from "@/components/brand-logo";
import { VersionFooter } from "@/components/version-footer";
import { siteConfig } from "@/config/site";
import { writeLoginSession } from "@/utils/session";

interface RegisterForm {
  username: string;
  password: string;
  confirmPassword: string;
  inviteCode: string;
  captchaId: string;
}

export default function RegisterPage() {
  const navigate = useNavigate();
  const [form, setForm] = useState<RegisterForm>({
    username: "",
    password: "",
    confirmPassword: "",
    inviteCode: "",
    captchaId: "",
  });
  const [registrationEnabled, setRegistrationEnabled] = useState(false);
  const [inviteRequired, setInviteRequired] = useState(false);
  const [captchaEnabled, setCaptchaEnabled] = useState(false);
  const [siteKey, setSiteKey] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let cancelled = false;

    (async () => {
      const settings = await getPublicCommerceSettings();

      if (cancelled) return;
      if (settings.code === 0 && settings.data) {
        setRegistrationEnabled(Boolean(settings.data.registrationEnabled));
        setInviteRequired(Boolean(settings.data.inviteRequired));
        setCaptchaEnabled(Boolean(settings.data.captchaEnabled));
      }
      const keyResp = await getPublicConfigByName("cloudflare_site_key");

      if (!cancelled && keyResp.code === 0 && keyResp.data?.value) {
        setSiteKey(keyResp.data.value);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  const updateForm = (key: keyof RegisterForm, value: string) => {
    setForm((prev) => ({ ...prev, [key]: value }));
  };

  const submit = async (captchaToken?: string) => {
    if (!registrationEnabled) {
      toast.error("当前未开放注册");

      return;
    }
    if (!form.username.trim() || !form.password) {
      toast.error("请填写用户名和密码");

      return;
    }
    if (form.password.length < 6) {
      toast.error("密码长度至少6位");

      return;
    }
    if (form.password !== form.confirmPassword) {
      toast.error("两次输入密码不一致");

      return;
    }
    if (inviteRequired && !form.inviteCode.trim()) {
      toast.error("请输入邀请码");

      return;
    }

    setLoading(true);
    try {
      const res = await registerUser({
        username: form.username.trim(),
        password: form.password,
        inviteCode: form.inviteCode.trim(),
        captchaId: captchaToken || form.captchaId,
      });

      if (res.code !== 0) {
        toast.error(res.msg || "注册失败");

        return;
      }
      writeLoginSession(res.data);
      toast.success("注册成功");
      navigate("/dashboard", { replace: true });
    } catch {
      toast.error("网络错误，请稍后重试");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="relative flex min-h-screen flex-col overflow-hidden bg-mesh-gradient">
      <section className="relative z-10 flex flex-1 flex-col items-center justify-center p-4">
        <motion.div
          animate={{ opacity: 1, y: 0 }}
          className="w-full max-w-[420px] px-4 sm:px-0"
          initial={{ opacity: 0, y: 24 }}
          transition={{ duration: 0.35, ease: [0.25, 0.46, 0.45, 0.94] }}
        >
          <Card className="w-full p-2 sm:p-4">
            <CardHeader className="flex-col items-center px-6 pb-0 pt-6">
              <BrandLogo className="mb-4 h-14 w-14 rounded-2xl" size={56} />
              <h1 className="text-2xl font-bold tracking-tight text-foreground">
                创建账号
              </h1>
              <p className="mt-2 text-sm font-medium text-default-500">
                注册后进入用户面板购买套餐
              </p>
            </CardHeader>
            <CardBody className="mt-2 px-6 py-6">
              <div className="flex flex-col gap-5">
                {!registrationEnabled && (
                  <div className="rounded-xl border border-amber-200 bg-amber-50/70 px-4 py-3 text-sm text-amber-700">
                    当前未开放用户注册，请联系管理员。
                  </div>
                )}
                <Input
                  isDisabled={loading || !registrationEnabled}
                  label="用户名"
                  placeholder="请输入用户名"
                  value={form.username}
                  variant="bordered"
                  onChange={(e) => updateForm("username", e.target.value)}
                />
                <Input
                  isDisabled={loading || !registrationEnabled}
                  label="密码"
                  placeholder="至少6位"
                  type="password"
                  value={form.password}
                  variant="bordered"
                  onChange={(e) => updateForm("password", e.target.value)}
                />
                <Input
                  isDisabled={loading || !registrationEnabled}
                  label="确认密码"
                  placeholder="再次输入密码"
                  type="password"
                  value={form.confirmPassword}
                  variant="bordered"
                  onChange={(e) =>
                    updateForm("confirmPassword", e.target.value)
                  }
                />
                {inviteRequired && (
                  <Input
                    isDisabled={loading || !registrationEnabled}
                    label="邀请码"
                    placeholder="请输入邀请码"
                    value={form.inviteCode}
                    variant="bordered"
                    onChange={(e) => updateForm("inviteCode", e.target.value)}
                  />
                )}
                {captchaEnabled && siteKey && registrationEnabled && (
                  <div className="native-muted-panel flex justify-center p-3">
                    <Turnstile
                      siteKey={siteKey}
                      onSuccess={(token) => updateForm("captchaId", token)}
                    />
                  </div>
                )}
                <Button
                  className="mt-2 h-12 rounded-xl bg-primary text-base font-bold text-white shadow-[0_8px_16px_rgba(0,122,255,0.3)]"
                  isDisabled={!registrationEnabled}
                  isLoading={loading}
                  onPress={() => void submit()}
                >
                  注册
                </Button>
                <Button variant="light" onPress={() => navigate("/")}>
                  返回登录
                </Button>
              </div>
            </CardBody>
          </Card>
        </motion.div>
        <VersionFooter
          containerClassName="fixed inset-x-0 bottom-4 py-4 text-center"
          poweredClassName="text-xs text-gray-400 dark:text-gray-500"
          version={siteConfig.version}
          versionClassName="text-xs text-gray-400 dark:text-gray-500 mt-1"
        />
      </section>
    </div>
  );
}
