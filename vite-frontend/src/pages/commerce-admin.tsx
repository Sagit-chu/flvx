import type { Dispatch, ReactNode, SetStateAction } from "react";
import type {
  SpeedLimitApiItem,
  TunnelApiItem,
  TunnelGroupApiItem,
} from "@/api/types";

import { useEffect, useState } from "react";
import toast from "react-hot-toast";

import {
  confirmAdminCommerceOrder,
  adjustAdminWallet,
  closeAdminTicket,
  deleteAdminPlan,
  deleteAdminCoupon,
  deleteInviteCode,
  getAdminAuditLogs,
  getAdminCommerceOrders,
  getAdminCommerceSettings,
  getAdminCoupons,
  getAdminCommerceReportSummary,
  getAdminCommerceRisks,
  getAdminResourceJobs,
  getAdminTicketMessages,
  getAdminPlans,
  getAdminPayments,
  getAdminRefunds,
  getAdminTickets,
  getAdminWalletLedger,
  getInviteCodes,
  getSpeedLimitList,
  getTunnelGroupList,
  getTunnelList,
  handleAdminRefund,
  replyAdminTicket,
  retryAdminResourceJob,
  saveAdminCoupon,
  saveAdminPlan,
  saveInviteCode,
  syncAdminUserResources,
  updateAdminCommerceSettings,
  updateAdminTicket,
  type CommerceAuditApiItem,
  type CommerceListFilter,
  type CommerceOrderApiItem,
  type CommercePaymentApiItem,
  type CommerceReportSummaryApiData,
  type CommerceResourceJobApiItem,
  type CommerceRiskApiItem,
  type CouponApiItem,
  type InviteCodeApiItem,
  type PaginatedApiData,
  type PlanApiItem,
  type RefundRequestApiItem,
  type SupportTicketApiItem,
  type SupportTicketMessageApiItem,
  type WalletLedgerApiItem,
} from "@/api";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Input } from "@/shadcn-bridge/heroui/input";

export type CommerceAdminSection =
  | "settings"
  | "register-settings"
  | "payment-settings"
  | "legal-settings"
  | "plans"
  | "invites"
  | "orders"
  | "reports"
  | "risk"
  | "resource-jobs"
  | "refunds"
  | "tickets"
  | "coupons"
  | "wallet"
  | "payments"
  | "audit";

const sectionMeta: Record<
  CommerceAdminSection,
  { title: string; description: string }
> = {
  settings: {
    title: "运营配置",
    description: "管理注册、支付和合规条款配置。",
  },
  "register-settings": {
    title: "注册设置",
    description: "控制用户注册入口和邀请码注册要求。",
  },
  "payment-settings": {
    title: "支付配置",
    description: "配置 e支付网关、商户信息和支付回调地址。",
  },
  "legal-settings": {
    title: "合规条款",
    description: "维护用户可见的服务条款、隐私政策和退款规则。",
  },
  plans: {
    title: "套餐管理",
    description: "配置套餐价格、周期、流量、隧道和重置流量价格。",
  },
  invites: {
    title: "邀请码",
    description: "生成和停用用户注册邀请码。",
  },
  orders: {
    title: "订单管理",
    description: "查看套餐、升级、重置流量和余额充值订单。",
  },
  reports: {
    title: "财务报表",
    description: "查看收入、退款、订阅和待处理业务指标。",
  },
  risk: {
    title: "风控列表",
    description: "查看异常订单、资源和支付风险信号。",
  },
  "resource-jobs": {
    title: "资源任务",
    description: "查看资源发放和套餐同步重试任务。",
  },
  refunds: {
    title: "退款审核",
    description: "人工审核退款申请并处理资源状态。",
  },
  tickets: {
    title: "工单管理",
    description: "处理用户工单、内部备注和管理员回复。",
  },
  coupons: {
    title: "优惠码",
    description: "管理优惠码、使用限制、套餐和分类绑定。",
  },
  wallet: {
    title: "余额管理",
    description: "查看余额流水并执行人工调账。",
  },
  payments: {
    title: "支付流水",
    description: "查看 e支付和余额支付记录。",
  },
  audit: {
    title: "审计日志",
    description: "查看商业化操作留痕。",
  },
};

const emptyPlan: PlanApiItem = {
  id: 0,
  name: "",
  description: "",
  category: "默认",
  priceCents: 0,
  resetFlowPriceCents: 0,
  currency: "CNY",
  durationDays: 30,
  flow: 100,
  dailyQuotaGB: 0,
  monthlyQuotaGB: 0,
  num: 5,
  maxConn: 0,
  speedId: null,
  sort: 0,
  status: 1,
  tunnelIds: [],
  tunnelGroupIds: [],
};

const statusText: Record<string, string> = {
  pending: "待支付",
  provisioning: "发放中",
  active: "已开通",
  failed: "发放失败",
  cancelled: "已取消",
  refunded: "已退款",
};
const orderTypeText: Record<string, string> = {
  new: "购买套餐",
  upgrade: "升级套餐",
  renew: "续费套餐",
  reset_flow: "重置流量",
  wallet_recharge: "余额充值",
};
const refundStatusText: Record<string, string> = {
  none: "无退款",
  pending: "待审核",
  approved: "已通过",
  rejected: "已拒绝",
};
const resourceJobStatusText: Record<string, string> = {
  pending: "待重试",
  running: "执行中",
  failed: "失败待重试",
  done: "已完成",
};

const money = (cents: number) => `¥${(cents / 100).toFixed(2)}`;
const formatTimeValue = (value?: number) => {
  if (!value) return "不过期";

  return new Date(value).toLocaleString();
};
const formatDateTimeInput = (value?: number) => {
  if (!value) return "";
  const date = new Date(value);
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000);

  return local.toISOString().slice(0, 16);
};
const parseDateTimeInput = (value: string) =>
  value ? new Date(value).getTime() : 0;
const paginatedSections: CommerceAdminSection[] = [
  "orders",
  "invites",
  "refunds",
  "tickets",
  "coupons",
  "payments",
  "wallet",
  "risk",
  "resource-jobs",
  "audit",
];
const extractPaginatedItems = <T,>(data?: PaginatedApiData<T> | null) =>
  data?.items || [];

export default function CommerceAdminPage({
  section = "settings",
}: {
  section?: CommerceAdminSection;
}) {
  const activeTab = section;
  const showRegistrationSettings =
    activeTab === "settings" || activeTab === "register-settings";
  const showPaymentSettings =
    activeTab === "settings" || activeTab === "payment-settings";
  const showLegalSettings =
    activeTab === "settings" || activeTab === "legal-settings";
  const [settings, setSettings] = useState<Record<string, string>>({});
  const [plans, setPlans] = useState<PlanApiItem[]>([]);
  const [planForm, setPlanForm] = useState<PlanApiItem>(emptyPlan);
  const [invites, setInvites] = useState<InviteCodeApiItem[]>([]);
  const [inviteForm, setInviteForm] = useState<Partial<InviteCodeApiItem>>({
    maxUses: 1,
    status: 1,
  });
  const [orders, setOrders] = useState<CommerceOrderApiItem[]>([]);
  const [payments, setPayments] = useState<CommercePaymentApiItem[]>([]);
  const [auditLogs, setAuditLogs] = useState<CommerceAuditApiItem[]>([]);
  const [refunds, setRefunds] = useState<RefundRequestApiItem[]>([]);
  const [tickets, setTickets] = useState<SupportTicketApiItem[]>([]);
  const [ticketMessages, setTicketMessages] = useState<
    SupportTicketMessageApiItem[]
  >([]);
  const [selectedTicket, setSelectedTicket] =
    useState<SupportTicketApiItem | null>(null);
  const [coupons, setCoupons] = useState<CouponApiItem[]>([]);
  const [walletLedger, setWalletLedger] = useState<WalletLedgerApiItem[]>([]);
  const [report, setReport] = useState<CommerceReportSummaryApiData | null>(
    null,
  );
  const [risks, setRisks] = useState<CommerceRiskApiItem[]>([]);
  const [resourceJobs, setResourceJobs] = useState<
    CommerceResourceJobApiItem[]
  >([]);
  const [filter, setFilter] = useState<CommerceListFilter>({});
  const [pagination, setPagination] = useState({
    page: 1,
    pageSize: 50,
    total: 0,
  });
  const [walletForm, setWalletForm] = useState({
    userId: 0,
    amountCents: 0,
    note: "",
  });
  const [resourceSyncUserId, setResourceSyncUserId] = useState(0);
  const [couponForm, setCouponForm] = useState<Partial<CouponApiItem>>({
    discountType: "fixed",
    discountValue: 0,
    status: 1,
  });
  const [ticketReply, setTicketReply] = useState<Record<number, string>>({});
  const [refundReview, setRefundReview] = useState<{
    item: RefundRequestApiItem;
    decision: "approved" | "rejected";
  } | null>(null);
  const [refundNote, setRefundNote] = useState("");
  const [tunnels, setTunnels] = useState<TunnelApiItem[]>([]);
  const [groups, setGroups] = useState<TunnelGroupApiItem[]>([]);
  const [speedLimits, setSpeedLimits] = useState<SpeedLimitApiItem[]>([]);

  const applyPagination = <T,>(
    data: PaginatedApiData<T> | null | undefined,
  ) => {
    if (!data) return;
    setPagination({
      page: data.page || 1,
      pageSize: data.pageSize || 50,
      total: data.total || 0,
    });
  };

  const load = async (page = pagination.page) => {
    const listFilter = {
      ...filter,
      page,
      pageSize: pagination.pageSize,
    };

    if (
      [
        "settings",
        "register-settings",
        "payment-settings",
        "legal-settings",
      ].includes(activeTab)
    ) {
      const settingsResp = await getAdminCommerceSettings();

      if (settingsResp.code === 0) setSettings(settingsResp.data || {});

      return;
    }
    if (activeTab === "plans") {
      const [plansResp, tunnelsResp, groupsResp, speedResp] = await Promise.all(
        [
          getAdminPlans(),
          getTunnelList(),
          getTunnelGroupList(),
          getSpeedLimitList(),
        ],
      );

      if (plansResp.code === 0) setPlans(plansResp.data || []);
      if (tunnelsResp.code === 0) setTunnels(tunnelsResp.data || []);
      if (groupsResp.code === 0) setGroups(groupsResp.data || []);
      if (speedResp.code === 0) setSpeedLimits(speedResp.data || []);

      return;
    }
    if (activeTab === "invites") {
      const invitesResp = await getInviteCodes(listFilter);

      if (invitesResp.code === 0) {
        setInvites(extractPaginatedItems(invitesResp.data));
        applyPagination(invitesResp.data);
      }

      return;
    }
    if (activeTab === "orders") {
      const ordersResp = await getAdminCommerceOrders(listFilter);

      if (ordersResp.code === 0) {
        setOrders(extractPaginatedItems(ordersResp.data));
        applyPagination(ordersResp.data);
      }

      return;
    }
    if (activeTab === "payments") {
      const paymentsResp = await getAdminPayments(listFilter);

      if (paymentsResp.code === 0) {
        setPayments(extractPaginatedItems(paymentsResp.data));
        applyPagination(paymentsResp.data);
      }

      return;
    }
    if (activeTab === "audit") {
      const auditResp = await getAdminAuditLogs(listFilter);

      if (auditResp.code === 0) {
        setAuditLogs(extractPaginatedItems(auditResp.data));
        applyPagination(auditResp.data);
      }

      return;
    }
    if (activeTab === "refunds") {
      const refundResp = await getAdminRefunds(listFilter);

      if (refundResp.code === 0) {
        setRefunds(extractPaginatedItems(refundResp.data));
        applyPagination(refundResp.data);
      }

      return;
    }
    if (activeTab === "tickets") {
      const ticketResp = await getAdminTickets(listFilter);

      if (ticketResp.code === 0) {
        setTickets(extractPaginatedItems(ticketResp.data));
        applyPagination(ticketResp.data);
      }

      return;
    }
    if (activeTab === "coupons") {
      const [couponResp, plansResp] = await Promise.all([
        getAdminCoupons(listFilter),
        getAdminPlans(),
      ]);

      if (couponResp.code === 0) {
        setCoupons(extractPaginatedItems(couponResp.data));
        applyPagination(couponResp.data);
      }
      if (plansResp.code === 0) setPlans(plansResp.data || []);

      return;
    }
    if (activeTab === "wallet") {
      const walletResp = await getAdminWalletLedger(listFilter);

      if (walletResp.code === 0) {
        setWalletLedger(extractPaginatedItems(walletResp.data));
        applyPagination(walletResp.data);
      }

      return;
    }
    if (activeTab === "reports") {
      const reportResp = await getAdminCommerceReportSummary();

      if (reportResp.code === 0) setReport(reportResp.data || null);

      return;
    }
    if (activeTab === "risk") {
      const riskResp = await getAdminCommerceRisks(listFilter);

      if (riskResp.code === 0) {
        setRisks(extractPaginatedItems(riskResp.data));
        applyPagination(riskResp.data);
      }

      return;
    }
    if (activeTab === "resource-jobs") {
      const jobResp = await getAdminResourceJobs(listFilter);

      if (jobResp.code === 0) {
        setResourceJobs(extractPaginatedItems(jobResp.data));
        applyPagination(jobResp.data);
      }

      return;
    }
  };

  useEffect(() => {
    setPagination((prev) => ({ ...prev, page: 1, total: 0 }));
    void load(1);
  }, [activeTab]);

  const saveSettings = async () => {
    const payload = { ...settings };

    if (payload.epay_key === "") {
      delete payload.epay_key;
    }
    if (payload.usdt_secret_key === "") {
      delete payload.usdt_secret_key;
    }
    delete payload.epay_key_configured;
    delete payload.usdt_secret_key_configured;
    const res = await updateAdminCommerceSettings(payload);

    if (res.code === 0) {
      toast.success("配置已保存");
      await load();
    } else {
      toast.error(res.msg || "保存失败");
    }
  };

  const savePlan = async () => {
    if (!planForm.name.trim()) {
      toast.error("请输入套餐名称");

      return;
    }
    const res = await saveAdminPlan(planForm);

    if (res.code === 0) {
      toast.success("套餐已保存");
      setPlanForm(emptyPlan);
      await load();
    } else {
      toast.error(res.msg || "保存失败");
    }
  };

  const saveInvite = async () => {
    const res = await saveInviteCode(inviteForm);

    if (res.code === 0) {
      toast.success("邀请码已保存");
      setInviteForm({ maxUses: 1, status: 1 });
      await load();
    } else {
      toast.error(res.msg || "保存失败");
    }
  };

  const saveCoupon = async () => {
    if (
      (couponForm.discountType || "fixed") === "percent" &&
      Number(couponForm.discountValue || 0) > 100
    ) {
      toast.error("百分比优惠不能超过100%");

      return;
    }
    const res = await saveAdminCoupon(couponForm);

    if (res.code === 0) {
      toast.success("优惠码已保存");
      setCouponForm({ discountType: "fixed", discountValue: 0, status: 1 });
      await load();
    } else {
      toast.error(res.msg || "保存失败");
    }
  };

  const adjustWallet = async () => {
    const res = await adjustAdminWallet(walletForm);

    if (res.code === 0) {
      toast.success("余额已调整");
      setWalletForm({ userId: 0, amountCents: 0, note: "" });
      await load();
    } else {
      toast.error(res.msg || "调账失败");
    }
  };

  const syncUserResources = async (userId = resourceSyncUserId) => {
    if (!userId || userId <= 0) {
      toast.error("请输入用户ID");

      return;
    }
    const res = await syncAdminUserResources(userId);

    if (res.code === 0) {
      toast.success("用户套餐资源已同步");
      setResourceSyncUserId(0);
      await load();
    } else {
      toast.error(res.msg || "资源同步失败");
    }
  };

  const openTicket = async (ticket: SupportTicketApiItem) => {
    const res = await getAdminTicketMessages(ticket.id);

    if (res.code !== 0) {
      toast.error(res.msg || "获取工单详情失败");

      return;
    }
    setSelectedTicket(res.data?.ticket || ticket);
    setTicketMessages(res.data?.messages || []);
  };

  const saveTicketMeta = async () => {
    if (!selectedTicket) return;
    const res = await updateAdminTicket({
      id: selectedTicket.id,
      category: selectedTicket.category,
      priority: selectedTicket.priority,
      internalNote: selectedTicket.internalNote,
    });

    if (res.code !== 0) {
      toast.error(res.msg || "保存工单失败");

      return;
    }
    toast.success("工单已保存");
    await openTicket(selectedTicket);
    await load();
  };

  const toggleArrayValue = (
    key: "tunnelIds" | "tunnelGroupIds",
    id: number,
  ) => {
    setPlanForm((prev) => {
      const exists = prev[key].includes(id);
      const next = exists
        ? prev[key].filter((item) => item !== id)
        : [...prev[key], id];

      return { ...prev, [key]: next };
    });
  };

  const resolvedPlanTunnels = () => {
    const selectedTunnelIds = new Set<number>(planForm.tunnelIds || []);

    groups
      .filter((group) => (planForm.tunnelGroupIds || []).includes(group.id))
      .forEach((group) => {
        (group.tunnelIds || []).forEach((id) => selectedTunnelIds.add(id));
      });

    return tunnels.filter((tunnel) => selectedTunnelIds.has(tunnel.id));
  };

  const openRefundReview = (
    refund: RefundRequestApiItem,
    decision: "approved" | "rejected",
  ) => {
    setRefundReview({ item: refund, decision });
    setRefundNote(
      decision === "approved" ? "已人工审核通过" : "不符合退款规则",
    );
  };

  const submitRefundReview = async () => {
    if (!refundReview) return;
    const note = refundNote.trim();

    if (!note) {
      toast.error("请输入审核备注");

      return;
    }
    await handleAdminRefund(refundReview.item.id, refundReview.decision, note);
    setRefundReview(null);
    setRefundNote("");
    await load();
  };

  return (
    <div className="h-full overflow-auto p-4 sm:p-6">
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-3xl font-bold text-foreground">
            {sectionMeta[activeTab].title}
          </h1>
          <p className="mt-2 text-sm text-default-500">
            {sectionMeta[activeTab].description}
          </p>
        </div>
      </div>

      {paginatedSections.includes(activeTab) && (
        <section className="mb-5 grid gap-3 rounded-2xl border border-white/70 bg-white/60 p-4 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50 md:grid-cols-5">
          <Input
            label="关键词"
            value={filter.keyword || ""}
            onChange={(e) =>
              setFilter((prev) => ({ ...prev, keyword: e.target.value }))
            }
          />
          <Input
            label="订单号"
            value={filter.orderNo || ""}
            onChange={(e) =>
              setFilter((prev) => ({ ...prev, orderNo: e.target.value }))
            }
          />
          <Input
            label="用户ID"
            type="number"
            value={String(filter.userId || 0)}
            onChange={(e) =>
              setFilter((prev) => ({
                ...prev,
                userId: Number(e.target.value) || 0,
              }))
            }
          />
          <Input
            label="状态"
            value={filter.status || ""}
            onChange={(e) =>
              setFilter((prev) => ({ ...prev, status: e.target.value }))
            }
          />
          <Button
            className="self-end bg-primary text-white"
            onPress={() => {
              setPagination((prev) => ({ ...prev, page: 1 }));
              void load(1);
            }}
          >
            筛选
          </Button>
        </section>
      )}
      {paginatedSections.includes(activeTab) && (
        <section className="mb-5 flex flex-col gap-3 rounded-2xl border border-white/70 bg-white/60 p-4 text-sm text-default-600 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50 sm:flex-row sm:items-center sm:justify-between">
          <span>
            第 {pagination.page} 页 / 共 {pagination.total} 条，每页{" "}
            {pagination.pageSize} 条
          </span>
          <div className="flex gap-2">
            <Button
              isDisabled={pagination.page <= 1}
              size="sm"
              variant="flat"
              onPress={() => {
                const page = Math.max(1, pagination.page - 1);

                setPagination((prev) => ({ ...prev, page }));
                void load(page);
              }}
            >
              上一页
            </Button>
            <Button
              isDisabled={
                pagination.page * pagination.pageSize >= pagination.total
              }
              size="sm"
              variant="flat"
              onPress={() => {
                const page = pagination.page + 1;

                setPagination((prev) => ({ ...prev, page }));
                void load(page);
              }}
            >
              下一页
            </Button>
          </div>
        </section>
      )}

      {[
        "settings",
        "register-settings",
        "payment-settings",
        "legal-settings",
      ].includes(activeTab) && (
        <section className="grid gap-5">
          {showRegistrationSettings && (
            <SettingsGroup
              description="控制前台注册入口，以及是否必须使用邀请码。"
              title="注册入口"
            >
              <div className="grid gap-4 lg:grid-cols-2">
                <Field label="开放用户注册">
                  <select
                    className="h-10 rounded-xl border border-default-200 bg-white/70 px-3 dark:bg-zinc-900"
                    value={settings.registration_enabled || "false"}
                    onChange={(e) =>
                      setSettings((prev) => ({
                        ...prev,
                        registration_enabled: e.target.value,
                      }))
                    }
                  >
                    <option value="true">开启</option>
                    <option value="false">关闭</option>
                  </select>
                </Field>
                <Field label="注册必须邀请码">
                  <select
                    className="h-10 rounded-xl border border-default-200 bg-white/70 px-3 dark:bg-zinc-900"
                    value={settings.invite_registration_required || "false"}
                    onChange={(e) =>
                      setSettings((prev) => ({
                        ...prev,
                        invite_registration_required: e.target.value,
                      }))
                    }
                  >
                    <option value="true">开启</option>
                    <option value="false">关闭</option>
                  </select>
                </Field>
              </div>
            </SettingsGroup>
          )}
          {showPaymentSettings && (
            <>
              <SettingsGroup
                description="用于支付宝、微信等人民币收款，按 e支付 V1 接口发起订单和验签回调。"
                title="e支付"
              >
                <div className="grid gap-4 lg:grid-cols-2">
                  <Field label="启用 e支付">
                    <select
                      className="h-10 rounded-xl border border-default-200 bg-white/70 px-3 dark:bg-zinc-900"
                      value={settings.epay_enabled || "false"}
                      onChange={(e) =>
                        setSettings((prev) => ({
                          ...prev,
                          epay_enabled: e.target.value,
                        }))
                      }
                    >
                      <option value="true">开启</option>
                      <option value="false">关闭</option>
                    </select>
                  </Field>
                  <TextSetting
                    label="站点名称"
                    name="epay_sitename"
                    setSettings={setSettings}
                    settings={settings}
                  />
                  <TextSetting
                    label="e支付网关"
                    name="epay_gateway"
                    placeholder="https://max.xinyuqicheng.cn/plugin/EpayApi/GatewayV1"
                    setSettings={setSettings}
                    settings={settings}
                  />
                  <TextSetting
                    label="支付提交地址"
                    name="epay_submit_url"
                    placeholder="https://max.xinyuqicheng.cn/plugin/EpayApi/GatewayV1/submit.php"
                    setSettings={setSettings}
                    settings={settings}
                  />
                  <TextSetting
                    label="商户ID"
                    name="epay_pid"
                    setSettings={setSettings}
                    settings={settings}
                  />
                  <TextSetting
                    description={
                      settings.epay_key_configured === "true"
                        ? "已配置，留空不会修改现有密钥"
                        : "未配置"
                    }
                    label="商户密钥"
                    name="epay_key"
                    placeholder={
                      settings.epay_key_configured === "true"
                        ? "留空不修改"
                        : undefined
                    }
                    setSettings={setSettings}
                    settings={settings}
                    type="password"
                  />
                  <TextSetting
                    label="异步通知地址"
                    name="epay_notify_url"
                    placeholder="https://域名/api/v1/payment/epay/notify"
                    setSettings={setSettings}
                    settings={settings}
                  />
                  <TextSetting
                    label="同步跳转地址"
                    name="epay_return_url"
                    placeholder="https://域名/plans"
                    setSettings={setSettings}
                    settings={settings}
                  />
                </div>
              </SettingsGroup>

              <SettingsGroup
                description="用于 USDT 收款，按 U支付/EPUSDT 接口创建交易并接收异步通知。"
                title="U支付"
              >
                <div className="grid gap-4 lg:grid-cols-2">
                  <Field label="启用 U支付">
                    <select
                      className="h-10 rounded-xl border border-default-200 bg-white/70 px-3 dark:bg-zinc-900"
                      value={settings.usdt_enabled || "false"}
                      onChange={(e) =>
                        setSettings((prev) => ({
                          ...prev,
                          usdt_enabled: e.target.value,
                        }))
                      }
                    >
                      <option value="true">开启</option>
                      <option value="false">关闭</option>
                    </select>
                  </Field>
                  <TextSetting
                    label="U支付网关"
                    name="usdt_api_base"
                    placeholder="https://pay.example.com"
                    setSettings={setSettings}
                    settings={settings}
                  />
                  <TextSetting
                    label="U支付商户PID"
                    name="usdt_pid"
                    setSettings={setSettings}
                    settings={settings}
                  />
                  <TextSetting
                    description={
                      settings.usdt_secret_key_configured === "true"
                        ? "已配置，留空不会修改现有密钥"
                        : "未配置"
                    }
                    label="U支付商户密钥"
                    name="usdt_secret_key"
                    placeholder={
                      settings.usdt_secret_key_configured === "true"
                        ? "留空不修改"
                        : undefined
                    }
                    setSettings={setSettings}
                    settings={settings}
                    type="password"
                  />
                  <TextSetting
                    label="U支付异步通知地址"
                    name="usdt_notify_url"
                    placeholder="https://域名/api/v1/payment/usdt/notify"
                    setSettings={setSettings}
                    settings={settings}
                  />
                  <TextSetting
                    label="U支付同步跳转地址"
                    name="usdt_return_url"
                    placeholder="https://域名/plans"
                    setSettings={setSettings}
                    settings={settings}
                  />
                  <TextSetting
                    label="U支付法币"
                    name="usdt_currency"
                    placeholder="cny"
                    setSettings={setSettings}
                    settings={settings}
                  />
                  <TextSetting
                    label="U支付币种"
                    name="usdt_token"
                    placeholder="usdt"
                    setSettings={setSettings}
                    settings={settings}
                  />
                  <TextSetting
                    label="U支付网络"
                    name="usdt_network"
                    placeholder="tron"
                    setSettings={setSettings}
                    settings={settings}
                  />
                </div>
              </SettingsGroup>

              <SettingsGroup
                description="控制用户是否可在前台付费重置当前套餐流量。具体价格仍按套餐配置读取。"
                title="流量重置"
              >
                <div className="grid gap-4 lg:grid-cols-2">
                  <Field label="开放付费重置流量">
                    <select
                      className="h-10 rounded-xl border border-default-200 bg-white/70 px-3 dark:bg-zinc-900"
                      value={settings.reset_flow_enabled || "false"}
                      onChange={(e) =>
                        setSettings((prev) => ({
                          ...prev,
                          reset_flow_enabled: e.target.value,
                        }))
                      }
                    >
                      <option value="true">开启</option>
                      <option value="false">关闭</option>
                    </select>
                  </Field>
                  <TextSetting
                    label="重置流量商品名"
                    name="reset_flow_name"
                    placeholder="重置套餐流量"
                    setSettings={setSettings}
                    settings={settings}
                  />
                </div>
              </SettingsGroup>
            </>
          )}
          {showLegalSettings && (
            <SettingsGroup
              description="维护用户前台可见的条款内容。"
              title="合规条款"
            >
              <div className="grid gap-4">
                <TextAreaSetting
                  label="服务条款"
                  name="legal_terms"
                  setSettings={setSettings}
                  settings={settings}
                />
                <TextAreaSetting
                  label="隐私政策"
                  name="legal_privacy"
                  setSettings={setSettings}
                  settings={settings}
                />
                <TextAreaSetting
                  label="退款政策"
                  name="legal_refund_policy"
                  setSettings={setSettings}
                  settings={settings}
                />
                <TextAreaSetting
                  label="可接受使用政策"
                  name="legal_acceptable_use"
                  setSettings={setSettings}
                  settings={settings}
                />
              </div>
            </SettingsGroup>
          )}
          <div className="rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50">
            <Button className="bg-primary text-white" onPress={saveSettings}>
              保存配置
            </Button>
          </div>
        </section>
      )}

      {activeTab === "plans" && (
        <div className="grid gap-5 xl:grid-cols-[420px_1fr]">
          <section className="rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50">
            <h2 className="mb-4 text-lg font-semibold">
              {planForm.id ? "编辑套餐" : "新建套餐"}
            </h2>
            <div className="grid gap-3">
              <Input
                label="套餐名称"
                value={planForm.name}
                onChange={(e) =>
                  setPlanForm((prev) => ({ ...prev, name: e.target.value }))
                }
              />
              <Input
                label="套餐说明"
                value={planForm.description}
                onChange={(e) =>
                  setPlanForm((prev) => ({
                    ...prev,
                    description: e.target.value,
                  }))
                }
              />
              <Input
                label="套餐分类"
                placeholder="例如：港日线路、国内线路、高防线路"
                value={planForm.category || "默认"}
                onChange={(e) =>
                  setPlanForm((prev) => ({
                    ...prev,
                    category: e.target.value || "默认",
                  }))
                }
              />
              <MoneyInput
                label="售价(元)"
                name="priceCents"
                setPlanForm={setPlanForm}
                value={planForm.priceCents}
              />
              <MoneyInput
                label="重置流量价格(元)"
                name="resetFlowPriceCents"
                setPlanForm={setPlanForm}
                value={planForm.resetFlowPriceCents}
              />
              <NumberInput
                label="周期(天)"
                name="durationDays"
                setPlanForm={setPlanForm}
                value={planForm.durationDays}
              />
              <NumberInput
                label="总流量(GB)"
                name="flow"
                setPlanForm={setPlanForm}
                value={planForm.flow}
              />
              <NumberInput
                label="规则数量"
                name="num"
                setPlanForm={setPlanForm}
                value={planForm.num}
              />
              <NumberInput
                label="最大连接数"
                name="maxConn"
                setPlanForm={setPlanForm}
                value={planForm.maxConn}
              />
              <Field label="限速规则">
                <select
                  className="h-10 rounded-xl border border-default-200 bg-white/70 px-3 dark:bg-zinc-900"
                  value={planForm.speedId || 0}
                  onChange={(e) =>
                    setPlanForm((prev) => ({
                      ...prev,
                      speedId: Number(e.target.value) || null,
                    }))
                  }
                >
                  <option value={0}>不限速</option>
                  {speedLimits.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.name}
                    </option>
                  ))}
                </select>
              </Field>
              <div>
                <div className="mb-2 text-sm font-medium">隧道分组</div>
                <div className="grid max-h-36 gap-2 overflow-auto rounded-xl border border-default-200 p-3">
                  {groups.map((group) => (
                    <label
                      key={group.id}
                      className="flex items-center gap-2 text-sm"
                    >
                      <input
                        checked={planForm.tunnelGroupIds.includes(group.id)}
                        type="checkbox"
                        onChange={() =>
                          toggleArrayValue("tunnelGroupIds", group.id)
                        }
                      />
                      {group.name}
                    </label>
                  ))}
                </div>
              </div>
              <div>
                <div className="mb-2 text-sm font-medium">指定隧道</div>
                <div className="grid max-h-36 gap-2 overflow-auto rounded-xl border border-default-200 p-3">
                  {tunnels.map((tunnel) => (
                    <label
                      key={tunnel.id}
                      className="flex items-center gap-2 text-sm"
                    >
                      <input
                        checked={planForm.tunnelIds.includes(tunnel.id)}
                        type="checkbox"
                        onChange={() =>
                          toggleArrayValue("tunnelIds", tunnel.id)
                        }
                      />
                      {tunnel.name}
                      <span className="text-xs text-default-400">
                        {Number(tunnel.trafficRatio || 1)} 倍
                      </span>
                    </label>
                  ))}
                </div>
              </div>
              <div className="rounded-xl border border-primary/20 bg-primary/5 p-3 text-sm text-default-600">
                <div className="font-medium text-foreground">最终开通隧道</div>
                <div className="mt-2">
                  {resolvedPlanTunnels().length > 0
                    ? resolvedPlanTunnels()
                        .map(
                          (tunnel) =>
                            `${tunnel.name} ${Number(tunnel.trafficRatio || 1)} 倍`,
                        )
                        .join("、")
                    : "未选择隧道，保存后不会向用户发放隧道资源"}
                </div>
              </div>
              <div className="flex gap-2">
                <Button className="bg-primary text-white" onPress={savePlan}>
                  保存套餐
                </Button>
                <Button variant="flat" onPress={() => setPlanForm(emptyPlan)}>
                  清空
                </Button>
              </div>
            </div>
          </section>
          <section className="rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50">
            <h2 className="mb-4 text-lg font-semibold">套餐列表</h2>
            <div className="overflow-x-auto">
              <table className="w-full min-w-[760px] text-sm">
                <thead>
                  <tr className="border-b text-left text-default-500">
                    <th className="py-2">名称</th>
                    <th className="py-2">分类</th>
                    <th className="py-2">售价</th>
                    <th className="py-2">重置流量</th>
                    <th className="py-2">周期</th>
                    <th className="py-2">资源</th>
                    <th className="py-2">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {plans.map((plan) => (
                    <tr key={plan.id} className="border-b border-default-100">
                      <td className="py-3">{plan.name}</td>
                      <td className="py-3">{plan.category || "默认"}</td>
                      <td className="py-3">{money(plan.priceCents)}</td>
                      <td className="py-3">
                        {plan.resetFlowPriceCents > 0
                          ? money(plan.resetFlowPriceCents)
                          : "未开放"}
                      </td>
                      <td className="py-3">{plan.durationDays} 天</td>
                      <td className="py-3">
                        {plan.tunnelGroupIds.length} 组 /{" "}
                        {plan.tunnelIds.length} 条
                      </td>
                      <td className="py-3">
                        <div className="flex gap-2">
                          <Button
                            size="sm"
                            variant="flat"
                            onPress={() => setPlanForm(plan)}
                          >
                            编辑
                          </Button>
                          <Button
                            color="danger"
                            size="sm"
                            variant="flat"
                            onPress={async () => {
                              await deleteAdminPlan(plan.id);
                              await load();
                            }}
                          >
                            下架
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        </div>
      )}

      {activeTab === "invites" && (
        <section className="rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50">
          <div className="mb-4 grid gap-3 md:grid-cols-2 xl:grid-cols-5">
            <Input
              label="邀请码"
              placeholder="留空自动生成"
              value={inviteForm.code || ""}
              onChange={(e) =>
                setInviteForm((prev) => ({ ...prev, code: e.target.value }))
              }
            />
            <Input
              label="可用次数"
              type="number"
              value={String(inviteForm.maxUses || 1)}
              onChange={(e) =>
                setInviteForm((prev) => ({
                  ...prev,
                  maxUses: Number(e.target.value) || 1,
                }))
              }
            />
            <Input
              label="过期时间"
              type="datetime-local"
              value={formatDateTimeInput(inviteForm.expTime)}
              onChange={(e) =>
                setInviteForm((prev) => ({
                  ...prev,
                  expTime: parseDateTimeInput(e.target.value),
                }))
              }
            />
            <Field label="状态">
              <select
                className="h-10 rounded-xl border border-default-200 bg-white/70 px-3 dark:bg-zinc-900"
                value={inviteForm.status ?? 1}
                onChange={(e) =>
                  setInviteForm((prev) => ({
                    ...prev,
                    status: Number(e.target.value),
                  }))
                }
              >
                <option value={1}>启用</option>
                <option value={0}>停用</option>
              </select>
            </Field>
            <Button
              className="self-end bg-primary text-white"
              onPress={saveInvite}
            >
              保存邀请码
            </Button>
          </div>
          <table className="w-full min-w-[720px] text-sm">
            <thead>
              <tr className="border-b text-left text-default-500">
                <th className="py-2">邀请码</th>
                <th className="py-2">使用</th>
                <th className="py-2">状态</th>
                <th className="py-2">操作</th>
              </tr>
            </thead>
            <tbody>
              {invites.map((invite) => (
                <tr key={invite.id} className="border-b border-default-100">
                  <td className="py-3">{invite.code}</td>
                  <td className="py-3">
                    {invite.usedCount}/{invite.maxUses}
                  </td>
                  <td className="py-3">
                    {invite.status === 1 ? "启用" : "停用"}
                  </td>
                  <td className="py-3">
                    <div className="flex gap-2">
                      <Button
                        size="sm"
                        variant="flat"
                        onPress={() =>
                          setInviteForm({
                            id: invite.id,
                            code: invite.code,
                            maxUses: invite.maxUses,
                            expTime: invite.expTime,
                            status: invite.status,
                          })
                        }
                      >
                        编辑
                      </Button>
                      <Button
                        color={invite.status === 1 ? "danger" : "primary"}
                        size="sm"
                        variant="flat"
                        onPress={async () => {
                          if (invite.status === 1) {
                            await deleteInviteCode(invite.id);
                          } else {
                            await saveInviteCode({ ...invite, status: 1 });
                          }
                          await load();
                        }}
                      >
                        {invite.status === 1 ? "停用" : "启用"}
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}

      {activeTab === "orders" && (
        <section className="rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50">
          <table className="w-full min-w-[900px] text-sm">
            <thead>
              <tr className="border-b text-left text-default-500">
                <th className="py-2">订单号</th>
                <th className="py-2">用户</th>
                <th className="py-2">套餐</th>
                <th className="py-2">类型</th>
                <th className="py-2">金额</th>
                <th className="py-2">状态</th>
                <th className="py-2">操作</th>
              </tr>
            </thead>
            <tbody>
              {orders.map((order) => (
                <tr key={order.id} className="border-b border-default-100">
                  <td className="py-3">{order.orderNo}</td>
                  <td className="py-3">{order.userId}</td>
                  <td className="py-3">{order.planName || order.planId}</td>
                  <td className="py-3">
                    {orderTypeText[order.orderType || "new"] ||
                      order.orderType ||
                      "购买套餐"}
                  </td>
                  <td className="py-3">{money(order.amountCents)}</td>
                  <td className="py-3">
                    {statusText[order.status] || order.status}
                  </td>
                  <td className="py-3">
                    <Button
                      isDisabled={
                        !["pending", "failed", "paid"].includes(order.status)
                      }
                      size="sm"
                      variant="flat"
                      onPress={async () => {
                        const res = await confirmAdminCommerceOrder(order.id);

                        if (res.code === 0) {
                          toast.success("订单已确认并发放");
                        } else {
                          toast.error(res.msg || "确认失败");
                        }
                        await load();
                      }}
                    >
                      手动确认
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}

      {activeTab === "reports" && (
        <section className="grid gap-4 md:grid-cols-3">
          {[
            ["累计实收", money(report?.paidTotalCents || 0)],
            ["近30天实收", money(report?.paidMonthCents || 0)],
            ["累计退款", money(report?.refundTotalCents || 0)],
            ["余额沉淀", money(report?.walletBalanceCents || 0)],
            ["今日订单", String(report?.ordersToday || 0)],
            ["待支付订单", String(report?.pendingOrders || 0)],
            ["有效订阅", String(report?.activeSubscriptions || 0)],
            ["待处理工单", String(report?.openTickets || 0)],
            ["待审核退款", String(report?.pendingRefunds || 0)],
          ].map(([label, value]) => (
            <div
              key={label}
              className="rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50"
            >
              <div className="text-sm text-default-500">{label}</div>
              <div className="mt-2 text-2xl font-bold text-foreground">
                {value}
              </div>
            </div>
          ))}
        </section>
      )}

      {activeTab === "risk" && (
        <div className="grid gap-5">
          <section className="grid gap-3 rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50 md:grid-cols-[1fr_auto]">
            <Input
              label="用户资源重同步"
              placeholder="输入用户ID，用于退款失败、套餐异常后的人工补偿同步"
              type="number"
              value={String(resourceSyncUserId || 0)}
              onChange={(e) =>
                setResourceSyncUserId(Number(e.target.value) || 0)
              }
            />
            <Button
              className="self-end bg-primary text-white"
              onPress={() => void syncUserResources()}
            >
              同步资源
            </Button>
          </section>
          <section className="rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50">
            <DataTable
              disablePagination
              headers={["用户", "类型", "等级", "说明", "计数", "操作"]}
              rows={risks.map((risk) => [
                String(risk.userId),
                risk.type,
                risk.level === "high" ? "高" : "中",
                risk.summary,
                String(risk.count),
                <Button
                  key={`sync-${risk.userId}-${risk.type}`}
                  size="sm"
                  variant="flat"
                  onPress={() => void syncUserResources(risk.userId)}
                >
                  同步资源
                </Button>,
              ])}
            />
          </section>
        </div>
      )}

      {activeTab === "resource-jobs" && (
        <section className="rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50">
          <DataTable
            disablePagination
            headers={[
              "任务",
              "用户",
              "订单",
              "状态",
              "次数",
              "下次执行",
              "错误",
              "操作",
            ]}
            rows={resourceJobs.map((job) => [
              job.jobType,
              String(job.userId || "-"),
              String(job.orderId || "-"),
              resourceJobStatusText[job.status] || job.status,
              `${job.attempts}/${job.maxAttempts}`,
              job.nextRunAt ? new Date(job.nextRunAt).toLocaleString() : "-",
              job.lastError || "-",
              job.status !== "done" ? (
                <Button
                  key={`retry-${job.id}`}
                  size="sm"
                  variant="flat"
                  onPress={async () => {
                    const res = await retryAdminResourceJob(job.id);

                    if (res.code === 0) {
                      toast.success("资源任务已重试");
                      await load();
                    } else {
                      toast.error(res.msg || "重试失败");
                    }
                  }}
                >
                  立即重试
                </Button>
              ) : (
                "-"
              ),
            ])}
          />
        </section>
      )}

      {activeTab === "refunds" && (
        <section className="rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50">
          <DataTable
            disablePagination
            headers={["订单号", "用户", "金额", "状态", "原因", "操作"]}
            rows={refunds.map((refund) => [
              refund.orderNo,
              String(refund.userId),
              money(refund.amountCents),
              refundStatusText[refund.status] || refund.status,
              refund.reason || "-",
              refund.status === "pending" ? (
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="flat"
                    onPress={() => openRefundReview(refund, "approved")}
                  >
                    通过
                  </Button>
                  <Button
                    color="danger"
                    size="sm"
                    variant="flat"
                    onPress={() => openRefundReview(refund, "rejected")}
                  >
                    拒绝
                  </Button>
                </div>
              ) : (
                "-"
              ),
            ])}
          />
        </section>
      )}

      {activeTab === "tickets" && (
        <section className="rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50">
          <div className="grid gap-3">
            {tickets.map((ticket) => (
              <div
                key={ticket.id}
                className="rounded-xl border border-default-100 bg-white/55 p-4 dark:bg-zinc-900/40"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div>
                    <div className="font-semibold">{ticket.title}</div>
                    <div className="text-xs text-default-500">
                      用户 {ticket.userId} / {ticket.category || "general"} /{" "}
                      {ticket.status === "open" ? "处理中" : "已关闭"}
                    </div>
                  </div>
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      variant="flat"
                      onPress={() => void openTicket(ticket)}
                    >
                      详情
                    </Button>
                    <Button
                      isDisabled={ticket.status === "closed"}
                      size="sm"
                      variant="flat"
                      onPress={async () => {
                        await closeAdminTicket(ticket.id);
                        await load();
                      }}
                    >
                      关闭
                    </Button>
                  </div>
                </div>
                <div className="mt-3 flex gap-2">
                  <input
                    className="h-10 flex-1 rounded-xl border border-default-200 bg-white/70 px-3 text-sm outline-none dark:bg-zinc-900"
                    placeholder="管理员回复"
                    value={ticketReply[ticket.id] || ""}
                    onChange={(event) =>
                      setTicketReply((prev) => ({
                        ...prev,
                        [ticket.id]: event.target.value,
                      }))
                    }
                  />
                  <Button
                    className="bg-primary text-white"
                    onPress={async () => {
                      const content = (ticketReply[ticket.id] || "").trim();

                      if (!content) {
                        toast.error("请输入回复内容");

                        return;
                      }
                      await replyAdminTicket(ticket.id, content);
                      setTicketReply((prev) => ({ ...prev, [ticket.id]: "" }));
                      await load();
                    }}
                  >
                    回复
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {activeTab === "coupons" && (
        <section className="rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50">
          <div className="mb-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <Input
              label="优惠码"
              value={couponForm.code || ""}
              onChange={(e) =>
                setCouponForm((prev) => ({ ...prev, code: e.target.value }))
              }
            />
            <Input
              label="名称"
              value={couponForm.name || ""}
              onChange={(e) =>
                setCouponForm((prev) => ({ ...prev, name: e.target.value }))
              }
            />
            <Field label="类型">
              <select
                className="h-10 rounded-xl border border-default-200 bg-white/70 px-3 dark:bg-zinc-900"
                value={couponForm.discountType || "fixed"}
                onChange={(e) =>
                  setCouponForm((prev) => ({
                    ...prev,
                    discountType: e.target.value as "fixed" | "percent",
                  }))
                }
              >
                <option value="fixed">固定减免(分)</option>
                <option value="percent">百分比</option>
              </select>
            </Field>
            <Input
              label="减免值"
              type="number"
              value={String(couponForm.discountValue || 0)}
              onChange={(e) =>
                setCouponForm((prev) => ({
                  ...prev,
                  discountValue: Number(e.target.value) || 0,
                }))
              }
            />
            <Field label="绑定套餐">
              <select
                className="h-10 rounded-xl border border-default-200 bg-white/70 px-3 dark:bg-zinc-900"
                value={couponForm.planId || 0}
                onChange={(e) =>
                  setCouponForm((prev) => ({
                    ...prev,
                    planId: Number(e.target.value) || 0,
                  }))
                }
              >
                <option value={0}>全部套餐</option>
                {plans.map((plan) => (
                  <option key={plan.id} value={plan.id}>
                    {plan.name}
                  </option>
                ))}
              </select>
            </Field>
            <Input
              label="绑定分类"
              value={couponForm.category || ""}
              onChange={(e) =>
                setCouponForm((prev) => ({ ...prev, category: e.target.value }))
              }
            />
            <Input
              label="最低消费(分)"
              type="number"
              value={String(couponForm.minAmountCents || 0)}
              onChange={(e) =>
                setCouponForm((prev) => ({
                  ...prev,
                  minAmountCents: Number(e.target.value) || 0,
                }))
              }
            />
            <Input
              label="每用户限用"
              type="number"
              value={String(couponForm.perUserLimit || 0)}
              onChange={(e) =>
                setCouponForm((prev) => ({
                  ...prev,
                  perUserLimit: Number(e.target.value) || 0,
                }))
              }
            />
            <Input
              label="总次数"
              type="number"
              value={String(couponForm.maxUses || 0)}
              onChange={(e) =>
                setCouponForm((prev) => ({
                  ...prev,
                  maxUses: Number(e.target.value) || 0,
                }))
              }
            />
            <Input
              label="过期时间"
              type="datetime-local"
              value={formatDateTimeInput(couponForm.expTime)}
              onChange={(e) =>
                setCouponForm((prev) => ({
                  ...prev,
                  expTime: parseDateTimeInput(e.target.value),
                }))
              }
            />
            <Field label="状态">
              <select
                className="h-10 rounded-xl border border-default-200 bg-white/70 px-3 dark:bg-zinc-900"
                value={couponForm.status ?? 1}
                onChange={(e) =>
                  setCouponForm((prev) => ({
                    ...prev,
                    status: Number(e.target.value),
                  }))
                }
              >
                <option value={1}>启用</option>
                <option value={0}>停用</option>
              </select>
            </Field>
            <Button
              className="self-end bg-primary text-white"
              onPress={saveCoupon}
            >
              保存优惠码
            </Button>
          </div>
          <DataTable
            disablePagination
            headers={[
              "优惠码",
              "名称",
              "类型",
              "减免",
              "限制",
              "过期时间",
              "使用",
              "状态",
              "操作",
            ]}
            rows={coupons.map((coupon) => [
              coupon.code,
              coupon.name || "-",
              coupon.discountType === "percent" ? "百分比" : "固定减免",
              coupon.discountType === "percent"
                ? `${coupon.discountValue}%`
                : money(coupon.discountValue),
              `${coupon.category || "全部分类"} / ${coupon.minAmountCents ? money(coupon.minAmountCents) : "无门槛"} / 每用户${coupon.perUserLimit || "不限"}`,
              formatTimeValue(coupon.expTime),
              `${coupon.usedCount}/${coupon.maxUses || "不限"}`,
              coupon.status === 1 ? "启用" : "停用",
              <div key={`coupon-actions-${coupon.id}`} className="flex gap-2">
                <Button
                  size="sm"
                  variant="flat"
                  onPress={() => setCouponForm(coupon)}
                >
                  编辑
                </Button>
                <Button
                  color={coupon.status === 1 ? "danger" : "primary"}
                  size="sm"
                  variant="flat"
                  onPress={async () => {
                    if (coupon.status === 1) {
                      await deleteAdminCoupon(coupon.id);
                    } else {
                      await saveAdminCoupon({ ...coupon, status: 1 });
                    }
                    await load();
                  }}
                >
                  {coupon.status === 1 ? "停用" : "启用"}
                </Button>
              </div>,
            ])}
          />
        </section>
      )}

      {activeTab === "payments" && (
        <section className="rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50">
          <DataTable
            disablePagination
            headers={["订单号", "渠道", "渠道流水", "金额", "状态", "时间"]}
            rows={payments.map((payment) => [
              payment.orderNo,
              payment.provider,
              payment.providerTradeNo,
              money(payment.amountCents),
              payment.status,
              new Date(payment.createdTime).toLocaleString(),
            ])}
          />
        </section>
      )}

      {activeTab === "wallet" && (
        <section className="rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50">
          <div className="mb-4 grid gap-3 lg:grid-cols-4">
            <Input
              label="用户ID"
              type="number"
              value={String(walletForm.userId || 0)}
              onChange={(e) =>
                setWalletForm((prev) => ({
                  ...prev,
                  userId: Number(e.target.value) || 0,
                }))
              }
            />
            <Input
              label="调账金额(分)"
              type="number"
              value={String(walletForm.amountCents || 0)}
              onChange={(e) =>
                setWalletForm((prev) => ({
                  ...prev,
                  amountCents: Number(e.target.value) || 0,
                }))
              }
            />
            <Input
              label="备注"
              value={walletForm.note}
              onChange={(e) =>
                setWalletForm((prev) => ({ ...prev, note: e.target.value }))
              }
            />
            <Button
              className="self-end bg-primary text-white"
              onPress={adjustWallet}
            >
              人工调账
            </Button>
          </div>
          <DataTable
            disablePagination
            headers={["用户", "金额", "余额", "类型", "备注", "时间"]}
            rows={walletLedger.map((item) => [
              String(item.userId),
              money(item.amountCents),
              money(item.balanceAfterCents),
              item.type,
              item.note || "-",
              new Date(item.createdTime).toLocaleString(),
            ])}
          />
        </section>
      )}

      {activeTab === "audit" && (
        <section className="rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50">
          <DataTable
            disablePagination
            headers={["动作", "对象", "摘要", "时间"]}
            rows={auditLogs.map((log) => [
              log.action,
              `${log.targetType} #${log.targetId}`,
              log.summary || "-",
              new Date(log.createdTime).toLocaleString(),
            ])}
          />
        </section>
      )}

      {refundReview && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4 backdrop-blur-sm">
          <div className="w-full max-w-lg rounded-2xl border border-white/70 bg-white p-6 shadow-xl dark:border-white/10 dark:bg-zinc-900">
            <div className="mb-4">
              <h2 className="text-xl font-bold">
                {refundReview.decision === "approved" ? "通过退款" : "拒绝退款"}
              </h2>
              <p className="mt-1 text-sm text-default-500">
                订单 {refundReview.item.orderNo} /{" "}
                {money(refundReview.item.amountCents)}
              </p>
            </div>
            <label className="grid gap-2 text-sm font-medium text-default-700">
              审核备注
              <textarea
                className="min-h-24 rounded-xl border border-default-200 bg-white/70 px-3 py-2 text-sm outline-none dark:bg-zinc-900"
                value={refundNote}
                onChange={(event) => setRefundNote(event.target.value)}
              />
            </label>
            <div className="mt-5 flex justify-end gap-2">
              <Button
                variant="flat"
                onPress={() => {
                  setRefundReview(null);
                  setRefundNote("");
                }}
              >
                取消
              </Button>
              <Button
                className="bg-primary text-white"
                onPress={submitRefundReview}
              >
                确认提交
              </Button>
            </div>
          </div>
        </div>
      )}

      {selectedTicket && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4 backdrop-blur-sm">
          <div className="max-h-[90vh] w-full max-w-3xl overflow-auto rounded-2xl border border-white/70 bg-white p-6 shadow-xl dark:border-white/10 dark:bg-zinc-900">
            <div className="mb-5 flex items-start justify-between gap-4">
              <div>
                <h2 className="text-xl font-bold">{selectedTicket.title}</h2>
                <p className="mt-1 text-sm text-default-500">
                  用户 {selectedTicket.userId} /{" "}
                  {selectedTicket.status === "open" ? "处理中" : "已关闭"}
                </p>
              </div>
              <Button
                variant="light"
                onPress={() => {
                  setSelectedTicket(null);
                  setTicketMessages([]);
                }}
              >
                关闭
              </Button>
            </div>
            <div className="mb-4 grid gap-3 md:grid-cols-3">
              <Input
                label="分类"
                value={selectedTicket.category || ""}
                onChange={(e) =>
                  setSelectedTicket((prev) =>
                    prev ? { ...prev, category: e.target.value } : prev,
                  )
                }
              />
              <Input
                label="优先级"
                value={selectedTicket.priority || "normal"}
                onChange={(e) =>
                  setSelectedTicket((prev) =>
                    prev ? { ...prev, priority: e.target.value } : prev,
                  )
                }
              />
              <Button
                className="self-end bg-primary text-white"
                onPress={saveTicketMeta}
              >
                保存工单
              </Button>
              <label className="grid gap-2 text-sm font-medium text-default-700 md:col-span-3">
                内部备注
                <textarea
                  className="min-h-20 rounded-xl border border-default-200 bg-white/70 px-3 py-2 text-sm outline-none dark:bg-zinc-900"
                  value={selectedTicket.internalNote || ""}
                  onChange={(e) =>
                    setSelectedTicket((prev) =>
                      prev ? { ...prev, internalNote: e.target.value } : prev,
                    )
                  }
                />
              </label>
            </div>
            <div className="grid gap-3">
              {ticketMessages.map((message) => (
                <div
                  key={message.id}
                  className="rounded-xl border border-default-100 bg-white/55 p-3 text-sm dark:bg-zinc-900/40"
                >
                  <div className="mb-1 text-xs text-default-400">
                    {message.isAdmin ? "管理员" : `用户 ${message.userId}`} /{" "}
                    {new Date(message.createdTime).toLocaleString()}
                  </div>
                  <div>{message.content}</div>
                  {message.attachmentUrl && (
                    <a
                      className="mt-2 block text-primary"
                      href={message.attachmentUrl}
                      rel="noreferrer"
                      target="_blank"
                    >
                      查看附件
                    </a>
                  )}
                </div>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="grid gap-2 text-sm font-medium text-default-700">
      {label}
      {children}
    </label>
  );
}

function SettingsGroup({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <section className="rounded-2xl border border-white/70 bg-white/60 p-5 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-900/50">
      <div className="mb-4">
        <h2 className="text-base font-semibold text-default-900">{title}</h2>
        <p className="mt-1 text-sm text-default-500">{description}</p>
      </div>
      {children}
    </section>
  );
}

function TextSetting({
  label,
  name,
  placeholder,
  description,
  settings,
  setSettings,
  type = "text",
}: {
  label: string;
  name: string;
  placeholder?: string;
  description?: string;
  settings: Record<string, string>;
  setSettings: Dispatch<SetStateAction<Record<string, string>>>;
  type?: string;
}) {
  return (
    <Input
      description={description}
      label={label}
      placeholder={placeholder}
      type={type}
      value={settings[name] || ""}
      onChange={(e) =>
        setSettings((prev) => ({ ...prev, [name]: e.target.value }))
      }
    />
  );
}

function TextAreaSetting({
  label,
  name,
  settings,
  setSettings,
}: {
  label: string;
  name: string;
  settings: Record<string, string>;
  setSettings: Dispatch<SetStateAction<Record<string, string>>>;
}) {
  return (
    <label className="grid gap-2 text-sm font-medium text-default-700 lg:col-span-2">
      {label}
      <textarea
        className="min-h-24 rounded-xl border border-default-200 bg-white/70 px-3 py-2 text-sm outline-none dark:bg-zinc-900"
        value={settings[name] || ""}
        onChange={(e) =>
          setSettings((prev) => ({ ...prev, [name]: e.target.value }))
        }
      />
    </label>
  );
}

function DataTable({
  headers,
  rows,
  disablePagination = false,
}: {
  headers: string[];
  rows: Array<Array<ReactNode>>;
  disablePagination?: boolean;
}) {
  const [page, setPage] = useState(1);
  const pageSize = 20;
  const totalPages = Math.max(1, Math.ceil(rows.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const visibleRows = disablePagination
    ? rows
    : rows.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  const exportCsv = () => {
    const csv = [headers, ...rows]
      .map((row) =>
        row
          .map((cell) => {
            const value =
              typeof cell === "string" || typeof cell === "number"
                ? String(cell)
                : "";

            return `"${value.replace(/"/g, '""')}"`;
          })
          .join(","),
      )
      .join("\n");
    const blob = new Blob([`\ufeff${csv}`], {
      type: "text/csv;charset=utf-8",
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");

    link.href = url;
    link.download = `export-${Date.now()}.csv`;
    link.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div>
      {rows.length > 0 && (
        <div className="mb-3 flex justify-end">
          <Button size="sm" variant="flat" onPress={exportCsv}>
            导出 CSV
          </Button>
        </div>
      )}
      <div className="overflow-x-auto">
        <table className="w-full min-w-[760px] text-sm">
          <thead>
            <tr className="border-b text-left text-default-500">
              {headers.map((header) => (
                <th key={header} className="py-2">
                  {header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {visibleRows.map((row, index) => (
              <tr key={index} className="border-b border-default-100">
                {row.map((cell, cellIndex) => (
                  <td key={cellIndex} className="py-3 pr-4">
                    {cell}
                  </td>
                ))}
              </tr>
            ))}
            {rows.length === 0 && (
              <tr>
                <td
                  className="py-6 text-center text-default-400"
                  colSpan={headers.length}
                >
                  暂无数据
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      {!disablePagination && rows.length > pageSize && (
        <div className="mt-4 flex items-center justify-between text-sm text-default-500">
          <span>
            第 {currentPage} / {totalPages} 页，共 {rows.length} 条
          </span>
          <div className="flex gap-2">
            <Button
              isDisabled={currentPage <= 1}
              size="sm"
              variant="flat"
              onPress={() => setPage((prev) => Math.max(1, prev - 1))}
            >
              上一页
            </Button>
            <Button
              isDisabled={currentPage >= totalPages}
              size="sm"
              variant="flat"
              onPress={() => setPage((prev) => Math.min(totalPages, prev + 1))}
            >
              下一页
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function NumberInput({
  label,
  name,
  value,
  setPlanForm,
}: {
  label: string;
  name: keyof Pick<
    PlanApiItem,
    | "priceCents"
    | "resetFlowPriceCents"
    | "durationDays"
    | "flow"
    | "num"
    | "maxConn"
  >;
  value: number;
  setPlanForm: Dispatch<SetStateAction<PlanApiItem>>;
}) {
  return (
    <Input
      label={label}
      type="number"
      value={String(value)}
      onChange={(e) =>
        setPlanForm((prev) => ({
          ...prev,
          [name]: Number(e.target.value) || 0,
        }))
      }
    />
  );
}

function MoneyInput({
  label,
  name,
  value,
  setPlanForm,
}: {
  label: string;
  name: keyof Pick<PlanApiItem, "priceCents" | "resetFlowPriceCents">;
  value: number;
  setPlanForm: Dispatch<SetStateAction<PlanApiItem>>;
}) {
  return (
    <Input
      label={label}
      min={0}
      step="0.01"
      type="number"
      value={String(Number(value || 0) / 100)}
      onChange={(e) =>
        setPlanForm((prev) => ({
          ...prev,
          [name]: Math.round((Number(e.target.value) || 0) * 100),
        }))
      }
    />
  );
}
