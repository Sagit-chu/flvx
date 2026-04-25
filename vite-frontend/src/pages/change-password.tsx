import { useState } from "react";
import { useNavigate } from "react-router-dom";
import toast from "react-hot-toast";

import { Button } from "@/shadcn-bridge/heroui/button";
import { Input } from "@/shadcn-bridge/heroui/input";
import { Card, CardBody, CardHeader } from "@/shadcn-bridge/heroui/card";
import { title } from "@/components/primitives";
import { updatePassword } from "@/api";
import DefaultLayout from "@/layouts/default";
import { safeLogout } from "@/utils/logout";

interface PasswordForm {
  newUsername: string;
  currentPassword: string;
  newPassword: string;
  confirmPassword: string;
}

export default function ChangePasswordPage() {
  const [form, setForm] = useState<PasswordForm>({
    newUsername: "",
    currentPassword: "",
    newPassword: "",
    confirmPassword: "",
  });
  const [loading, setLoading] = useState(false);
  const [errors, setErrors] = useState<Partial<PasswordForm>>({});
  const navigate = useNavigate();

  const validateForm = (): boolean => {
    const newErrors: Partial<PasswordForm> = {};

    if (!form.newUsername.trim()) {
      newErrors.newUsername = "请输入新用户名";
    } else if (form.newUsername.length < 3) {
      newErrors.newUsername = "用户名长度至少3位";
    } else if (form.newUsername.length > 20) {
      newErrors.newUsername = "用户名长度不能超过20位";
    }

    if (!form.currentPassword.trim()) {
      newErrors.currentPassword = "请输入当前密码";
    }

    if (!form.newPassword.trim()) {
      newErrors.newPassword = "请输入新密码";
    } else if (form.newPassword.length < 6) {
      newErrors.newPassword = "新密码长度不能少于6位";
    } else if (form.newPassword.length > 20) {
      newErrors.newPassword = "新密码长度不能超过20位";
    }

    if (!form.confirmPassword.trim()) {
      newErrors.confirmPassword = "请再次输入新密码";
    } else if (form.confirmPassword !== form.newPassword) {
      newErrors.confirmPassword = "两次输入密码不一致";
    }

    setErrors(newErrors);

    return Object.keys(newErrors).length === 0;
  };

  const handleInputChange = (field: keyof PasswordForm, value: string) => {
    setForm((prev) => ({ ...prev, [field]: value }));
    if (errors[field]) {
      setErrors((prev) => ({ ...prev, [field]: undefined }));
    }
  };

  const handleSubmit = async () => {
    if (!validateForm()) return;

    setLoading(true);
    try {
      const response = await updatePassword(form);

      if (response.code === 0) {
        toast.success(response.msg || "账号密码修改成功");

        // 使用 toast 确认对话框的替代方案
        setTimeout(() => {
          toast.success("即将跳转到登陆页面，请重新登录");
          setTimeout(() => {
            logout();
          }, 1000);
        }, 1000);
      } else {
        toast.error(response.msg || "账号密码修改失败");
      }
    } catch {
      toast.error("修改账号密码时发生错误");
    } finally {
      setLoading(false);
    }
  };

  const logout = () => {
    safeLogout();
    navigate("/");
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !loading) {
      handleSubmit();
    }
  };

  return (
    <DefaultLayout>
      <section className="flex flex-col items-center justify-center gap-4 py-8 md:py-10 min-h-[calc(100dvh-200px)]">
        <div className="w-full max-w-lg">
          <Card className="w-full">
            <CardHeader className="pb-0 pt-6 px-6 flex-col items-center">
              <div className="w-12 h-12 bg-warning-100 rounded-full flex items-center justify-center mb-3">
                <svg
                  aria-hidden="true"
                  className="w-6 h-6 text-warning-600"
                  fill="currentColor"
                  viewBox="0 0 20 20"
                >
                  <path
                    clipRule="evenodd"
                    d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z"
                    fillRule="evenodd"
                  />
                </svg>
              </div>

              <h1 className={title({ size: "sm" })}>安全提醒</h1>
              <p className="text-small text-default-500 mt-2 text-center">
                检测到您使用的是默认账号密码，为了您的账户安全，请立即修改
              </p>
            </CardHeader>

            <CardBody className="px-6 py-6">
              <div className="flex flex-col gap-4">
                <Input
                  errorMessage={errors.newUsername}
                  isDisabled={loading}
                  isInvalid={!!errors.newUsername}
                  label="新用户名"
                  placeholder="请输入新用户名（至少3位）"
                  value={form.newUsername}
                  variant="bordered"
                  onChange={(e) =>
                    handleInputChange("newUsername", e.target.value)
                  }
                  onKeyDown={handleKeyPress}
                />

                <Input
                  errorMessage={errors.currentPassword}
                  isDisabled={loading}
                  isInvalid={!!errors.currentPassword}
                  label="当前密码"
                  placeholder="请输入当前密码"
                  type="password"
                  value={form.currentPassword}
                  variant="bordered"
                  onChange={(e) =>
                    handleInputChange("currentPassword", e.target.value)
                  }
                  onKeyDown={handleKeyPress}
                />

                <Input
                  errorMessage={errors.newPassword}
                  isDisabled={loading}
                  isInvalid={!!errors.newPassword}
                  label="新密码"
                  placeholder="请输入新密码（至少6位）"
                  type="password"
                  value={form.newPassword}
                  variant="bordered"
                  onChange={(e) =>
                    handleInputChange("newPassword", e.target.value)
                  }
                  onKeyDown={handleKeyPress}
                />

                <Input
                  errorMessage={errors.confirmPassword}
                  isDisabled={loading}
                  isInvalid={!!errors.confirmPassword}
                  label="确认新密码"
                  placeholder="请再次输入新密码"
                  type="password"
                  value={form.confirmPassword}
                  variant="bordered"
                  onChange={(e) =>
                    handleInputChange("confirmPassword", e.target.value)
                  }
                  onKeyDown={handleKeyPress}
                />

                <Button
                  className="mt-2"
                  color="warning"
                  disabled={loading}
                  isLoading={loading}
                  size="lg"
                  onPress={handleSubmit}
                >
                  {loading ? "修改中..." : "立即修改账号密码"}
                </Button>

                <div className="bg-warning-50 border border-warning-200 text-warning-700 px-3 py-2 rounded-lg text-sm text-center">
                  ⚠️ 注意：修改账号密码后需要重新登录
                </div>
              </div>
            </CardBody>
          </Card>
        </div>
      </section>
    </DefaultLayout>
  );
}
