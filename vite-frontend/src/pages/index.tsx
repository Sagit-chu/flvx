import { useState } from "react";
import { useNavigate } from "react-router-dom";
import toast from "react-hot-toast";
import { Turnstile } from "@marsidev/react-turnstile";
import { motion } from "framer-motion";

import { Card, CardBody, CardHeader } from "@/shadcn-bridge/heroui/card";
import { Input } from "@/shadcn-bridge/heroui/input";
import { Button } from "@/shadcn-bridge/heroui/button";
import { siteConfig } from "@/config/site";
import { title } from "@/components/primitives";
import DefaultLayout from "@/layouts/default";
import { login, LoginData, checkCaptcha, getConfigByName } from "@/api";
import { writeLoginSession } from "@/utils/session";
import { useWebViewMode } from "@/hooks/useWebViewMode";

interface LoginForm {
  username: string;
  password: string;
  captchaId: string;
}

export default function IndexPage() {
  const [form, setForm] = useState<LoginForm>({
    username: "",
    password: "",
    captchaId: "",
  });
  const [loading, setLoading] = useState(false);
  const [errors, setErrors] = useState<Partial<LoginForm>>({});
  const [showCaptcha, setShowCaptcha] = useState(false);
  const [siteKey, setSiteKey] = useState("");
  const navigate = useNavigate();
  const isWebView = useWebViewMode();

  // 验证表单
  const validateForm = (): boolean => {
    const newErrors: Partial<LoginForm> = {};

    if (!form.username.trim()) {
      newErrors.username = "请输入用户名";
    }

    if (!form.password.trim()) {
      newErrors.password = "请输入密码";
    } else if (form.password.length < 6) {
      newErrors.password = "密码长度至少6位";
    }

    setErrors(newErrors);

    return Object.keys(newErrors).length === 0;
  };

  // 处理输入变化
  const handleInputChange = (field: keyof LoginForm, value: string) => {
    setForm((prev) => ({ ...prev, [field]: value }));
    // 清除该字段的错误
    if (errors[field]) {
      setErrors((prev) => ({ ...prev, [field]: undefined }));
    }
  };

  // 执行登录请求
  const performLogin = async (captchaToken?: string) => {
    try {
      const finalCaptchaId =
        typeof captchaToken === "string" && captchaToken.trim()
          ? captchaToken
          : form.captchaId;

      const loginData: LoginData = {
        username: form.username.trim(),
        password: form.password,
        captchaId: finalCaptchaId,
      };

      const response = await login(loginData);

      if (response.code !== 0) {
        toast.error(response.msg || "登录失败");
        if (showCaptcha) {
          setForm((prev) => ({ ...prev, captchaId: "" }));
        }

        return;
      }

      // 检查是否需要强制修改密码
      if (response.data.requirePasswordChange) {
        writeLoginSession(response.data);
        toast.success("检测到默认密码，即将跳转到修改密码页面");
        navigate("/change-password");

        return;
      }

      // 保存登录信息
      writeLoginSession(response.data);

      // 登录成功
      toast.success("登录成功");
      navigate("/dashboard");
    } catch {
      toast.error("网络错误，请稍后重试");
    } finally {
      setLoading(false);
    }
  };

  const handleLogin = async () => {
    if (!validateForm()) return;

    setLoading(true);

    try {
      // 先检查是否需要验证码
      const checkResponse = await checkCaptcha();

      if (checkResponse.code !== 0) {
        toast.error("检查验证码状态失败，请重试" + checkResponse.msg);
        setLoading(false);

        return;
      }

      // 根据返回值决定是否显示验证码
      if (checkResponse.data === 0) {
        await performLogin();
      } else {
        const configResp = await getConfigByName("cloudflare_site_key");

        if (configResp.code === 0 && configResp.data && configResp.data.value) {
          setSiteKey(configResp.data.value);
          setShowCaptcha(true);
        } else {
          toast.error("未配置Cloudflare Site Key，请联系管理员");
          setLoading(false);
        }
      }
    } catch (error) {
      toast.error("网络错误，请稍后重试" + error);
      setLoading(false);
    }
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !loading) {
      handleLogin();
    }
  };

  return (
    <DefaultLayout>
      <section className="flex flex-col items-center justify-center gap-4 py-4 sm:py-8 md:py-10 pb-20 min-h-[calc(100dvh-120px)] sm:min-h-[calc(100dvh-200px)]">
        <motion.div
          animate={{ opacity: 1, y: 0 }}
          className="w-full max-w-md px-4 sm:px-0"
          initial={{ opacity: 0, y: 24 }}
          transition={{ duration: 0.35, ease: [0.25, 0.46, 0.45, 0.94] }}
        >
          <Card className="w-full">
            <CardHeader className="pb-0 pt-6 px-6 flex-col items-center">
              <h1 className={title({ size: "sm" })}>登陆</h1>
              <p className="text-small text-default-500 mt-2">
                请输入您的账号信息
              </p>
            </CardHeader>
            <CardBody className="px-6 py-6">
              <div className="flex flex-col gap-4">
                <Input
                  errorMessage={errors.username}
                  isDisabled={loading}
                  isInvalid={!!errors.username}
                  label="用户名"
                  placeholder="请输入用户名"
                  value={form.username}
                  variant="bordered"
                  onChange={(e) =>
                    handleInputChange("username", e.target.value)
                  }
                  onKeyDown={handleKeyPress}
                />

                <Input
                  isDisabled={loading}
                  isInvalid={!!errors.password}
                  label="密码"
                  placeholder="请输入密码"
                  type="password"
                  value={form.password}
                  variant="bordered"
                  onChange={(e) =>
                    handleInputChange("password", e.target.value)
                  }
                  onKeyDown={handleKeyPress}
                />

                <Button
                  className="mt-2"
                  color="primary"
                  disabled={loading}
                  isLoading={loading}
                  size="lg"
                  onPress={handleLogin}
                >
                  {loading ? (showCaptcha ? "验证中..." : "登录中...") : "登录"}
                </Button>
              </div>
            </CardBody>
          </Card>
        </motion.div>

        {/* 版权信息 - 固定在底部，不占据布局空间 */}

        <div className="fixed inset-x-0 bottom-4 text-center py-4">
          <p className="text-xs text-gray-400 dark:text-gray-500">
            Powered by{" "}
            <a
              className="text-gray-500 dark:text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
              href={siteConfig.github_repo}
              rel="noopener noreferrer"
              target="_blank"
            >
              FLVX
            </a>
          </p>
          <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">
            v{isWebView ? siteConfig.app_version : siteConfig.version}
          </p>
        </div>

        {/* 验证码弹层 */}
        {showCaptcha && siteKey && (
          <div className="fixed inset-0 z-50 flex items-center justify-center">
            {/* 背景遮罩层 - 模糊效果，暗黑模式下更深 */}
            <button
              className="absolute inset-0 bg-black/60 dark:bg-black/80 backdrop-blur-sm captcha-backdrop-enter"
              type="button"
              onClick={() => {
                setShowCaptcha(false);
                setLoading(false);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  setShowCaptcha(false);
                  setLoading(false);
                }
              }}
            />
            {/* 验证码容器 */}
            <div className="mb-4 relative z-50 bg-white dark:bg-zinc-900 p-6 rounded-lg shadow-xl">
              <div className="mb-4 text-center text-sm font-medium text-gray-700 dark:text-gray-200">
                请完成安全验证
              </div>
              <div className="flex justify-center">
                <Turnstile
                  options={{
                    theme: (document.documentElement.classList.contains(
                      "dark",
                    ) ||
                    document.documentElement.getAttribute("data-theme") ===
                      "dark" ||
                    window.matchMedia("(prefers-color-scheme: dark)").matches
                      ? "dark"
                      : "light") as "light" | "dark" | "auto",
                  }}
                  siteKey={siteKey}
                  onError={() => {
                    toast.error("验证失败，请刷新重试");
                    setLoading(false);
                  }}
                  onExpire={() => {
                    setForm((prev) => ({ ...prev, captchaId: "" }));
                  }}
                  onSuccess={(token) => {
                    setForm((prev) => ({ ...prev, captchaId: token }));
                    void performLogin(token);
                  }}
                />
              </div>
            </div>
          </div>
        )}
      </section>
    </DefaultLayout>
  );
}
