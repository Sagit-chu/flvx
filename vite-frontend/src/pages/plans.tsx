import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import toast from "react-hot-toast";

import {
  cancelCommerceOrder,
  createCommerceOrder,
  getMyCommerceOrders,
  getMyNotifications,
  getMySubscription,
  getMyTicketMessages,
  getMyTickets,
  getMyWallet,
  getPublicCommerceSettings,
  getPublicPlans,
  payCommerceOrder,
  payCommerceOrderWithProvider,
  payCommerceOrderWithBalance,
  rechargeMyWallet,
  markAllNotificationsRead,
  replyMyTicket,
  resetMySubscriptionFlow,
  requestOrderRefund,
  createMyTicket,
  type CommerceListFilter,
  type CommerceOrderApiItem,
  type NotificationApiItem,
  type PaginatedApiData,
  type PlanApiItem,
  type SupportTicketApiItem,
  type SupportTicketMessageApiItem,
  type UserSubscriptionApiItem,
  type WalletLedgerApiItem,
} from "@/api";
import { Button } from "@/shadcn-bridge/heroui/button";
import { PageHeader, PageShell } from "@/components/app-ui";

const formatMoney = (cents: number) => `¥${(cents / 100).toFixed(2)}`;
const formatNames = (names?: string[], fallbackCount?: number) => {
  if (names && names.length > 0) return names.join("、");
  if (fallbackCount && fallbackCount > 0) return `${fallbackCount} 个`;

  return "未指定";
};
const formatRatio = (value?: number) => {
  const ratio = Number(value || 1);

  return `${Number.isInteger(ratio) ? ratio.toFixed(0) : ratio.toFixed(2)} 倍`;
};
const planScopeKey = (plan: PlanApiItem) => {
  if (plan.scopeKey) return plan.scopeKey;
  const tunnelIds = [...(plan.tunnelIds || [])].sort((a, b) => a - b);

  return tunnelIds.length > 0
    ? `tunnels:${tunnelIds.join(",")}`
    : `plan:${plan.id}`;
};
const planTunnelIds = (plan: PlanApiItem) =>
  new Set(
    (plan.tunnels || []).map((item) => item.id).concat(plan.tunnelIds || []),
  );
const subscriptionTunnelIds = (subscription: UserSubscriptionApiItem) =>
  new Set((subscription.planTunnels || []).map((item) => item.id));

const statusText: Record<string, string> = {
  pending: "待支付",
  paid: "已支付",
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
const userPaginatedSections: PlansSection[] = [
  "orders",
  "wallet",
  "notifications",
  "tickets",
];
const extractPaginatedItems = <T,>(
  data?: PaginatedApiData<T> | T[] | null,
): T[] => {
  if (Array.isArray(data)) return data;

  return data?.items || [];
};

export type PlansSection =
  | "subscription"
  | "store"
  | "coupon"
  | "orders"
  | "wallet"
  | "notifications"
  | "tickets";

const sectionMeta: Record<
  PlansSection,
  { title: string; description: string }
> = {
  subscription: {
    title: "我的套餐",
    description: "查看当前套餐、可用隧道、隧道倍率和重置流量。",
  },
  store: {
    title: "套餐商城",
    description: "购买、续费或升级套餐，开通可用隧道和规则额度。",
  },
  coupon: {
    title: "优惠码",
    description: "购买套餐前填写优惠码，创建订单时自动抵扣。",
  },
  orders: {
    title: "我的订单",
    description: "查看订单详情，继续付款、取消订单或申请退款。",
  },
  wallet: {
    title: "账户余额",
    description: "查看余额、充值记录和余额流水。",
  },
  notifications: {
    title: "站内通知",
    description: "查看套餐、订单、退款和工单相关通知。",
  },
  tickets: {
    title: "售后工单",
    description: "提交售后工单，查看处理状态并补充回复。",
  },
};

export default function PlansPage({ section }: { section?: PlansSection }) {
  const navigate = useNavigate();
  const [plans, setPlans] = useState<PlanApiItem[]>([]);
  const [orders, setOrders] = useState<CommerceOrderApiItem[]>([]);
  const [subscription, setSubscription] = useState<
    | (UserSubscriptionApiItem & {
        subscriptions?: UserSubscriptionApiItem[];
        resetFlowEnabled?: boolean;
      })
    | null
  >(null);
  const [loadingPlanId, setLoadingPlanId] = useState<number | null>(null);
  const [loadingOrderId, setLoadingOrderId] = useState<number | null>(null);
  const [resettingFlow, setResettingFlow] = useState(false);
  const [activeCategory, setActiveCategory] = useState("全部");
  const [couponCode, setCouponCode] = useState("");
  const [notifications, setNotifications] = useState<NotificationApiItem[]>([]);
  const [tickets, setTickets] = useState<SupportTicketApiItem[]>([]);
  const [walletBalance, setWalletBalance] = useState(0);
  const [usdtEnabled, setUsdtEnabled] = useState(false);
  const [walletItems, setWalletItems] = useState<WalletLedgerApiItem[]>([]);
  const [rechargeAmount, setRechargeAmount] = useState(10);
  const [ticketTitle, setTicketTitle] = useState("");
  const [ticketCategory, setTicketCategory] = useState("general");
  const [ticketContent, setTicketContent] = useState("");
  const [ticketAttachment, setTicketAttachment] = useState("");
  const [ticketReply, setTicketReply] = useState("");
  const [selectedTicket, setSelectedTicket] =
    useState<SupportTicketApiItem | null>(null);
  const [ticketMessages, setTicketMessages] = useState<
    SupportTicketMessageApiItem[]
  >([]);
  const [selectedOrder, setSelectedOrder] =
    useState<CommerceOrderApiItem | null>(null);
  const [refundTarget, setRefundTarget] = useState<CommerceOrderApiItem | null>(
    null,
  );
  const [refundReason, setRefundReason] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [listPagination, setListPagination] = useState({
    page: 1,
    pageSize: 20,
    total: 0,
  });

  const activeSection: PlansSection = section || "store";
  const subscriptions = useMemo(
    () => subscription?.subscriptions || (subscription ? [subscription] : []),
    [subscription],
  );
  const resetFlowEnabled = Boolean(subscription?.resetFlowEnabled);
  const canBalancePayOrder = (order: CommerceOrderApiItem) =>
    Boolean(order.canPay) &&
    order.orderType !== "wallet_recharge" &&
    walletBalance >= Number(order.amountCents || 0);
  const categories = useMemo(() => {
    const values = plans
      .map((plan) => String(plan.category || "默认").trim() || "默认")
      .filter((item, index, list) => list.indexOf(item) === index);

    return ["全部", ...values];
  }, [plans]);
  const visiblePlans = useMemo(() => {
    if (activeCategory === "全部") return plans;

    return plans.filter(
      (plan) =>
        (String(plan.category || "默认").trim() || "默认") === activeCategory,
    );
  }, [activeCategory, plans]);

  const applyListPagination = <T,>(
    data: PaginatedApiData<T> | null | undefined,
  ) => {
    if (!data) return;
    setListPagination({
      page: data.page || 1,
      pageSize: data.pageSize || 20,
      total: data.total || 0,
    });
  };

  const load = async (page = listPagination.page) => {
    const listFilter: CommerceListFilter = {
      page,
      pageSize: listPagination.pageSize,
    };
    const [
      planResp,
      orderResp,
      subResp,
      notificationResp,
      ticketResp,
      walletResp,
      settingsResp,
    ] = await Promise.all([
      getPublicPlans(),
      getMyCommerceOrders(listFilter),
      getMySubscription(),
      getMyNotifications(listFilter),
      getMyTickets(listFilter),
      getMyWallet(listFilter),
      getPublicCommerceSettings(),
    ]);

    if (planResp.code === 0) setPlans(planResp.data || []);
    if (orderResp.code === 0) {
      setOrders(extractPaginatedItems(orderResp.data));
      if (activeSection === "orders") applyListPagination(orderResp.data);
    }
    if (subResp.code === 0) setSubscription(subResp.data || null);
    if (notificationResp.code === 0) {
      setNotifications(extractPaginatedItems(notificationResp.data));
      if (activeSection === "notifications")
        applyListPagination(notificationResp.data);
    }
    if (ticketResp.code === 0) {
      setTickets(extractPaginatedItems(ticketResp.data));
      if (activeSection === "tickets") applyListPagination(ticketResp.data);
    }
    if (walletResp.code === 0) {
      setWalletBalance(walletResp.data?.balanceCents || 0);
      setWalletItems(extractPaginatedItems(walletResp.data));
      if (activeSection === "wallet") applyListPagination(walletResp.data);
    }
    if (settingsResp.code === 0) {
      setUsdtEnabled(Boolean(settingsResp.data?.usdtEnabled));
    }
    setLoaded(true);
  };

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    if (!loaded || !userPaginatedSections.includes(activeSection)) return;
    setListPagination((prev) => ({ ...prev, page: 1, total: 0 }));
    void load(1);
  }, [activeSection]);

  useEffect(() => {
    if (section || !loaded) return;
    navigate(subscription ? "/plans/subscription" : "/plans/store", {
      replace: true,
    });
  }, [loaded, navigate, section, subscription]);

  const subscriptionForPlan = (plan: PlanApiItem) => {
    const scopeKey = planScopeKey(plan);

    return subscriptions.find(
      (item) => item.planScopeKey === scopeKey || item.planId === plan.id,
    );
  };

  const actionForPlan = (plan: PlanApiItem) => {
    const current = subscriptionForPlan(plan);

    if (!current) {
      const targetTunnelIds = planTunnelIds(plan);
      const hasOverlap = subscriptions.some((item) => {
        if (item.planId === plan.id || item.planScopeKey === planScopeKey(plan))
          return false;
        const currentTunnelIds = subscriptionTunnelIds(item);

        return [...targetTunnelIds].some((id) => currentTunnelIds.has(id));
      });

      return hasOverlap
        ? {
            action: "new" as const,
            disabled: true,
            label: "已有套餐包含部分相同隧道",
          }
        : { action: "new" as const, disabled: false, label: "立即购买" };
    }
    if (current.planId === plan.id) {
      return { action: "renew" as const, disabled: false, label: "续费套餐" };
    }
    if (plan.priceCents > Number(current.planPriceCents || 0)) {
      return { action: "upgrade" as const, disabled: false, label: "升级套餐" };
    }

    return {
      action: "new" as const,
      disabled: true,
      label: "当前线路不可降级",
    };
  };

  const buyPlan = async (
    plan: PlanApiItem,
    payType: "alipay" | "balance" | "usdt" = "alipay",
  ) => {
    const { action, disabled } = actionForPlan(plan);

    if (disabled) return;

    setLoadingPlanId(plan.id);
    try {
      const res = await createCommerceOrder({
        planId: plan.id,
        type: payType,
        action,
        couponCode: couponCode.trim() || undefined,
      });

      if (res.code !== 0) {
        toast.error(
          res.msg ||
            (payType === "balance"
              ? "余额支付失败"
              : payType === "usdt"
                ? "U支付建单失败"
                : "创建订单失败"),
        );

        return;
      }
      if (!res.data?.payUrl) {
        toast.success(
          payType === "balance" ? "余额支付成功，套餐已开通" : "订单已处理",
        );
        await load();

        return;
      }
      window.location.href = res.data.payUrl;
    } catch {
      toast.error(
        payType === "balance"
          ? "余额支付失败"
          : payType === "usdt"
            ? "U支付建单失败"
            : "创建订单失败",
      );
    } finally {
      setLoadingPlanId(null);
    }
  };

  const refundOrder = async (order: CommerceOrderApiItem) => {
    const reason = refundReason.trim();

    if (!reason) {
      toast.error("请输入退款原因");

      return;
    }
    setLoadingOrderId(order.id);
    try {
      const res = await requestOrderRefund(order.id, reason);

      if (res.code !== 0) {
        toast.error(res.msg || "退款申请失败");

        return;
      }
      toast.success("退款申请已提交");
      setSelectedOrder(null);
      setRefundTarget(null);
      setRefundReason("");
      await load();
    } catch {
      toast.error("退款申请失败");
    } finally {
      setLoadingOrderId(null);
    }
  };

  const submitTicket = async () => {
    const res = await createMyTicket({
      title: ticketTitle.trim(),
      category: ticketCategory,
      content: ticketContent.trim(),
      attachmentUrl: ticketAttachment.trim(),
    });

    if (res.code !== 0) {
      toast.error(res.msg || "提交工单失败");

      return;
    }
    toast.success("工单已提交");
    setTicketTitle("");
    setTicketCategory("general");
    setTicketContent("");
    setTicketAttachment("");
    await load();
  };

  const rechargeWallet = async (payType: "alipay" | "usdt" = "alipay") => {
    const amountCents = Math.round(Number(rechargeAmount || 0) * 100);
    const res = await rechargeMyWallet({ amountCents, type: payType });

    if (res.code !== 0 || !res.data?.payUrl) {
      toast.error(
        res.msg || (payType === "usdt" ? "U支付充值失败" : "创建充值订单失败"),
      );

      return;
    }
    window.location.href = res.data.payUrl;
  };

  const markNotificationsRead = async () => {
    const res = await markAllNotificationsRead();

    if (res.code === 0) {
      await load();
    }
  };

  const openTicket = async (ticket: SupportTicketApiItem) => {
    const res = await getMyTicketMessages(ticket.id);

    if (res.code !== 0) {
      toast.error(res.msg || "获取工单详情失败");

      return;
    }
    setSelectedTicket(res.data?.ticket || ticket);
    setTicketMessages(res.data?.messages || []);
  };

  const submitTicketReply = async () => {
    if (!selectedTicket) return;
    const res = await replyMyTicket(selectedTicket.id, ticketReply.trim());

    if (res.code !== 0) {
      toast.error(res.msg || "回复失败");

      return;
    }
    setTicketReply("");
    await openTicket(selectedTicket);
    await load();
  };

  const resetFlow = async (
    item: UserSubscriptionApiItem,
    payType: "alipay" | "balance" | "usdt" = "alipay",
  ) => {
    setResettingFlow(true);
    try {
      const res = await resetMySubscriptionFlow({
        type: payType,
        subscriptionId: item.id,
      });

      if (res.code !== 0) {
        toast.error(res.msg || "创建重置订单失败");

        return;
      }
      if (!res.data?.payUrl) {
        toast.success("流量已重置");
        await load();

        return;
      }
      window.location.href = res.data.payUrl;
    } catch {
      toast.error("创建重置订单失败");
    } finally {
      setResettingFlow(false);
    }
  };

  const continuePay = async (order: CommerceOrderApiItem) => {
    setLoadingOrderId(order.id);
    try {
      const res =
        order.paymentProvider === "epusdt"
          ? await payCommerceOrderWithProvider(order.id, "usdt")
          : await payCommerceOrder(order.id);

      if (res.code !== 0 || !res.data?.payUrl) {
        toast.error(res.msg || "获取支付链接失败");

        return;
      }
      window.location.href = res.data.payUrl;
    } catch {
      toast.error("获取支付链接失败");
    } finally {
      setLoadingOrderId(null);
    }
  };

  const balancePay = async (order: CommerceOrderApiItem) => {
    setLoadingOrderId(order.id);
    try {
      const res = await payCommerceOrderWithBalance(order.id);

      if (res.code !== 0) {
        toast.error(res.msg || "余额支付失败");

        return;
      }
      toast.success("余额支付成功，订单已发放");
      setSelectedOrder(null);
      await load();
    } catch {
      toast.error("余额支付失败");
    } finally {
      setLoadingOrderId(null);
    }
  };

  const cancelOrder = async (order: CommerceOrderApiItem) => {
    setLoadingOrderId(order.id);
    try {
      const res = await cancelCommerceOrder(order.id);

      if (res.code !== 0) {
        toast.error(res.msg || "取消订单失败");

        return;
      }
      toast.success("订单已取消");
      setSelectedOrder(null);
      await load();
    } catch {
      toast.error("取消订单失败");
    } finally {
      setLoadingOrderId(null);
    }
  };

  return (
    <PageShell className="h-full overflow-auto">
      <PageHeader
        description={sectionMeta[activeSection].description}
        title={sectionMeta[activeSection].title}
      />

      {activeSection === "subscription" && (
        <section className="native-panel p-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h2 className="text-lg font-semibold">我的套餐</h2>
          </div>
          {subscriptions.length > 0 ? (
            <div className="mt-4 grid gap-4">
              {subscriptions.map((item) => (
                <div
                  key={item.id}
                  className="native-muted-panel p-4 text-sm text-default-600"
                >
                  <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <div className="text-base font-semibold text-foreground">
                        {item.planName || `套餐 #${item.planId}`}
                      </div>
                      <div className="mt-1 text-xs text-default-400">
                        {item.planCategory || "默认"}
                      </div>
                    </div>
                    {resetFlowEnabled &&
                    Number(item.resetFlowPriceCents || 0) > 0 ? (
                      <div className="flex flex-wrap gap-2">
                        <Button
                          isLoading={resettingFlow}
                          size="sm"
                          variant="flat"
                          onPress={() => void resetFlow(item)}
                        >
                          {item.resetFlowName || "重置套餐流量"}{" "}
                          {formatMoney(Number(item.resetFlowPriceCents || 0))}
                        </Button>
                        {walletBalance >=
                          Number(item.resetFlowPriceCents || 0) && (
                          <Button
                            isLoading={resettingFlow}
                            size="sm"
                            variant="flat"
                            onPress={() => void resetFlow(item, "balance")}
                          >
                            余额重置
                          </Button>
                        )}
                        {usdtEnabled && (
                          <Button
                            isLoading={resettingFlow}
                            size="sm"
                            variant="flat"
                            onPress={() => void resetFlow(item, "usdt")}
                          >
                            U支付重置
                          </Button>
                        )}
                      </div>
                    ) : null}
                  </div>
                  <div className="grid gap-3 sm:grid-cols-4">
                    <div>
                      订阅状态：
                      {statusText[String(item.status || "active")] ||
                        String(item.status || "active")}
                    </div>
                    <div>
                      生效时间：
                      {new Date(Number(item.startsAt || 0)).toLocaleString()}
                    </div>
                    <div>
                      到期时间：
                      {new Date(Number(item.expiresAt || 0)).toLocaleString()}
                    </div>
                    <div>
                      可用隧道：
                      {Array.isArray(item.planTunnels)
                        ? `${item.planTunnels.length} 条`
                        : "0 条"}
                    </div>
                    <div className="sm:col-span-4">
                      隧道倍率：
                      {Array.isArray(item.planTunnels) &&
                      item.planTunnels.length > 0
                        ? item.planTunnels
                            .map(
                              (tunnel) =>
                                `${tunnel.name || "未命名隧道"} ${formatRatio(tunnel.trafficRatio)}`,
                            )
                            .join("、")
                        : "未指定"}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className="mt-3 text-sm text-default-500">
              当前暂无有效套餐，请在下方选择套餐购买。
            </p>
          )}
        </section>
      )}

      {activeSection === "coupon" && (
        <section className="flex flex-col gap-2 native-panel p-4 sm:flex-row sm:items-end">
          <label className="grid flex-1 gap-1 text-sm font-medium text-default-700">
            优惠码
            <input
              className="h-10 rounded-[var(--radius-control)] border border-input bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
              placeholder="有优惠码可在购买前填写"
              value={couponCode}
              onChange={(event) => setCouponCode(event.target.value)}
            />
          </label>
          <div className="text-xs text-default-500">
            优惠码会在创建订单时自动抵扣，未填写则按套餐原价支付。
          </div>
        </section>
      )}

      {activeSection === "store" && (
        <>
          <section className="mb-4 flex flex-col gap-2 native-panel p-4 sm:flex-row sm:items-end">
            <label className="grid flex-1 gap-1 text-sm font-medium text-default-700">
              优惠码
              <input
                className="h-10 rounded-[var(--radius-control)] border border-input bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
                placeholder="有优惠码可在购买前填写"
                value={couponCode}
                onChange={(event) => setCouponCode(event.target.value)}
              />
            </label>
            <div className="text-xs text-default-500">创建订单时自动抵扣。</div>
          </section>
          <div className="mb-4 flex flex-wrap gap-2">
            {categories.map((category) => (
              <Button
                key={category}
                color={activeCategory === category ? "primary" : "default"}
                size="sm"
                variant={activeCategory === category ? "solid" : "flat"}
                onPress={() => setActiveCategory(category)}
              >
                {category}
              </Button>
            ))}
          </div>

          <section className="grid gap-4 lg:grid-cols-3">
            {visiblePlans.map((plan) => {
              const planAction = actionForPlan(plan);

              return (
                <article
                  key={plan.id}
                  className="flex min-h-[320px] flex-col native-panel p-5"
                >
                  <div className="flex items-start justify-between gap-4">
                    <div>
                      <div className="mb-2 inline-flex rounded-full bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary">
                        {plan.category || "默认"}
                      </div>
                      <h2 className="text-xl font-bold">{plan.name}</h2>
                      <p className="mt-2 text-sm text-default-500">
                        {plan.description || "标准隧道套餐"}
                      </p>
                    </div>
                    <div className="text-right">
                      <div className="text-2xl font-bold text-primary">
                        {formatMoney(plan.priceCents)}
                      </div>
                      <div className="text-xs text-default-500">
                        {plan.durationDays} 天
                      </div>
                    </div>
                  </div>

                  <div className="mb-6 mt-5 grid gap-2 text-sm text-default-600">
                    <div>总流量：{plan.flow} GB</div>
                    <div>
                      重置流量：
                      {plan.resetFlowPriceCents > 0
                        ? formatMoney(plan.resetFlowPriceCents)
                        : "未开放"}
                    </div>
                    <div>规则数量：{plan.num} 条</div>
                    <div>最大连接数：{plan.maxConn || "不限"}</div>
                    <div>
                      隧道分组：
                      {formatNames(
                        plan.tunnelGroupNames,
                        plan.tunnelGroupIds.length,
                      )}
                    </div>
                    <div>
                      指定隧道：
                      {formatNames(plan.tunnelNames, plan.tunnelIds.length)}
                    </div>
                    <div>
                      隧道倍率：
                      {plan.tunnels && plan.tunnels.length > 0
                        ? plan.tunnels
                            .map(
                              (tunnel) =>
                                `${tunnel.name} ${formatRatio(tunnel.trafficRatio)}`,
                            )
                            .join("、")
                        : "未指定"}
                    </div>
                  </div>

                  <div className="mt-auto grid gap-2">
                    <Button
                      className="h-11 rounded-xl bg-primary font-semibold text-white"
                      isDisabled={planAction.disabled}
                      isLoading={loadingPlanId === plan.id}
                      onPress={() => void buyPlan(plan)}
                    >
                      {planAction.label}
                    </Button>
                    {!planAction.disabled && (
                      <Button
                        className="h-10 rounded-xl font-semibold"
                        isLoading={loadingPlanId === plan.id}
                        variant="flat"
                        onPress={() => void buyPlan(plan, "balance")}
                      >
                        余额支付
                      </Button>
                    )}
                    {!planAction.disabled && usdtEnabled && (
                      <Button
                        className="h-10 rounded-xl font-semibold"
                        isLoading={loadingPlanId === plan.id}
                        variant="flat"
                        onPress={() => void buyPlan(plan, "usdt")}
                      >
                        U支付
                      </Button>
                    )}
                  </div>
                </article>
              );
            })}
          </section>
        </>
      )}

      {activeSection === "orders" && (
        <section className="native-panel p-5">
          <h2 className="text-lg font-semibold">我的订单</h2>
          <div className="mt-4 overflow-x-auto">
            <table className="w-full min-w-[900px] text-sm">
              <thead>
                <tr className="border-b border-default-200 text-left text-default-500">
                  <th className="py-2">订单号</th>
                  <th className="py-2">套餐</th>
                  <th className="py-2">类型</th>
                  <th className="py-2">金额</th>
                  <th className="py-2">状态</th>
                  <th className="py-2">创建时间</th>
                  <th className="py-2">操作</th>
                </tr>
              </thead>
              <tbody>
                {orders.map((order) => (
                  <tr key={order.id} className="border-b border-default-100">
                    <td className="py-3">{order.orderNo}</td>
                    <td className="py-3">
                      {order.planName || `套餐 #${order.planId}`}
                    </td>
                    <td className="py-3">
                      {orderTypeText[order.orderType || "new"] ||
                        order.orderType ||
                        "购买套餐"}
                    </td>
                    <td className="py-3">{formatMoney(order.amountCents)}</td>
                    <td className="py-3">
                      {statusText[order.status] || order.status}
                    </td>
                    <td className="py-3">
                      {new Date(order.createdTime).toLocaleString()}
                    </td>
                    <td className="py-3">
                      <div className="flex flex-wrap gap-2">
                        <Button
                          size="sm"
                          variant="flat"
                          onPress={() => setSelectedOrder(order)}
                        >
                          查看
                        </Button>
                        {order.canPay && (
                          <Button
                            className="bg-primary text-white"
                            isLoading={loadingOrderId === order.id}
                            size="sm"
                            onPress={() => void continuePay(order)}
                          >
                            继续付款
                          </Button>
                        )}
                        {canBalancePayOrder(order) && (
                          <Button
                            isLoading={loadingOrderId === order.id}
                            size="sm"
                            variant="flat"
                            onPress={() => void balancePay(order)}
                          >
                            余额支付
                          </Button>
                        )}
                        {order.canCancel && (
                          <Button
                            color="danger"
                            isLoading={loadingOrderId === order.id}
                            size="sm"
                            variant="flat"
                            onPress={() => void cancelOrder(order)}
                          >
                            取消
                          </Button>
                        )}
                        {order.canRefund && (
                          <Button
                            color="warning"
                            isLoading={loadingOrderId === order.id}
                            size="sm"
                            variant="flat"
                            onPress={() => {
                              setRefundTarget(order);
                              setRefundReason("");
                            }}
                          >
                            申请退款
                          </Button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
                {orders.length === 0 && (
                  <tr>
                    <td
                      className="py-6 text-center text-default-400"
                      colSpan={7}
                    >
                      暂无订单
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {activeSection === "wallet" && (
        <section className="native-panel p-5">
          <h2 className="text-lg font-semibold">账户余额</h2>
          <div className="mt-3 text-2xl font-bold text-primary">
            {formatMoney(walletBalance)}
          </div>
          <div className="mt-4 flex flex-col gap-2 sm:flex-row">
            <input
              className="h-10 rounded-[var(--radius-control)] border border-input bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
              min={1}
              type="number"
              value={rechargeAmount}
              onChange={(event) =>
                setRechargeAmount(Number(event.target.value) || 0)
              }
            />
            <Button
              className="bg-primary text-white"
              onPress={() => void rechargeWallet()}
            >
              e支付充值
            </Button>
            {usdtEnabled && (
              <Button
                className="bg-primary text-white"
                variant="flat"
                onPress={() => rechargeWallet("usdt")}
              >
                U支付充值
              </Button>
            )}
          </div>
          <div className="mt-4 grid gap-2">
            {walletItems.map((item) => (
              <div
                key={item.id}
                className="native-muted-panel flex items-center justify-between p-3 text-sm"
              >
                <span>{item.note || "余额变动"}</span>
                <span
                  className={
                    item.amountCents >= 0 ? "text-success" : "text-danger"
                  }
                >
                  {item.amountCents >= 0 ? "+" : ""}
                  {formatMoney(item.amountCents)}
                </span>
              </div>
            ))}
            {walletItems.length === 0 && (
              <div className="text-sm text-default-400">暂无余额流水</div>
            )}
          </div>
        </section>
      )}

      {refundTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4 backdrop-blur-sm">
          <div className="w-full max-w-lg native-panel p-6">
            <div className="mb-4">
              <h2 className="text-xl font-bold">申请退款</h2>
              <p className="mt-1 text-sm text-default-500">
                订单 {refundTarget.orderNo} /{" "}
                {formatMoney(refundTarget.amountCents)}
              </p>
            </div>
            <label className="grid gap-2 text-sm font-medium text-default-700">
              退款原因
              <textarea
                className="min-h-24 rounded-[var(--radius-control)] border border-input bg-surface px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
                value={refundReason}
                onChange={(event) => setRefundReason(event.target.value)}
              />
            </label>
            <div className="mt-5 flex justify-end gap-2">
              <Button
                variant="flat"
                onPress={() => {
                  setRefundTarget(null);
                  setRefundReason("");
                }}
              >
                取消
              </Button>
              <Button
                className="bg-primary text-white"
                isLoading={loadingOrderId === refundTarget.id}
                onPress={() => void refundOrder(refundTarget)}
              >
                提交申请
              </Button>
            </div>
          </div>
        </div>
      )}

      {activeSection === "notifications" && (
        <section className="native-panel p-5">
          <div className="flex items-center justify-between gap-3">
            <h2 className="text-lg font-semibold">站内通知</h2>
            <Button size="sm" variant="flat" onPress={markNotificationsRead}>
              全部已读
            </Button>
          </div>
          <div className="mt-4 grid gap-3">
            {notifications.map((item) => (
              <div key={item.id} className="native-muted-panel p-3 text-sm">
                <div className="font-medium">{item.title}</div>
                <div className="mt-1 text-default-500">{item.content}</div>
                <div className="mt-2 flex items-center justify-between text-xs text-default-400">
                  <span>{new Date(item.createdTime).toLocaleString()}</span>
                  <span>{item.readTime > 0 ? "已读" : "未读"}</span>
                </div>
              </div>
            ))}
            {notifications.length === 0 && (
              <div className="text-sm text-default-400">暂无通知</div>
            )}
          </div>
        </section>
      )}

      {activeSection === "tickets" && (
        <section className="native-panel p-5">
          <h2 className="text-lg font-semibold">售后工单</h2>
          <div className="mt-4 grid gap-3">
            <input
              className="h-10 rounded-[var(--radius-control)] border border-input bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
              placeholder="工单标题"
              value={ticketTitle}
              onChange={(event) => setTicketTitle(event.target.value)}
            />
            <select
              className="h-10 rounded-[var(--radius-control)] border border-input bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
              value={ticketCategory}
              onChange={(event) => setTicketCategory(event.target.value)}
            >
              <option value="general">通用问题</option>
              <option value="billing">账务支付</option>
              <option value="technical">技术故障</option>
              <option value="refund">退款售后</option>
            </select>
            <textarea
              className="min-h-24 rounded-[var(--radius-control)] border border-input bg-surface px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
              placeholder="描述你的问题或退款补充说明"
              value={ticketContent}
              onChange={(event) => setTicketContent(event.target.value)}
            />
            <input
              className="h-10 rounded-[var(--radius-control)] border border-input bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
              placeholder="附件链接，可选"
              value={ticketAttachment}
              onChange={(event) => setTicketAttachment(event.target.value)}
            />
            <Button className="bg-primary text-white" onPress={submitTicket}>
              提交工单
            </Button>
            <div className="mt-2 grid gap-2">
              {tickets.map((ticket) => (
                <div
                  key={ticket.id}
                  className="native-muted-panel flex items-center justify-between p-3 text-sm"
                >
                  <span>{ticket.title}</span>
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-default-500">
                      {ticket.status === "open" ? "处理中" : "已关闭"}
                    </span>
                    <Button
                      size="sm"
                      variant="flat"
                      onPress={() => void openTicket(ticket)}
                    >
                      详情
                    </Button>
                  </div>
                </div>
              ))}
              {tickets.length === 0 && (
                <div className="text-sm text-default-400">暂无工单</div>
              )}
            </div>
          </div>
        </section>
      )}
      {userPaginatedSections.includes(activeSection) && (
        <section className="flex flex-col gap-3 native-panel p-4 text-sm text-default-600 sm:flex-row sm:items-center sm:justify-between">
          <span>
            第 {listPagination.page} 页 / 共 {listPagination.total} 条，每页{" "}
            {listPagination.pageSize} 条
          </span>
          <div className="flex gap-2">
            <Button
              isDisabled={listPagination.page <= 1}
              size="sm"
              variant="flat"
              onPress={() => {
                const page = Math.max(1, listPagination.page - 1);

                setListPagination((prev) => ({ ...prev, page }));
                void load(page);
              }}
            >
              上一页
            </Button>
            <Button
              isDisabled={
                listPagination.page * listPagination.pageSize >=
                listPagination.total
              }
              size="sm"
              variant="flat"
              onPress={() => {
                const page = listPagination.page + 1;

                setListPagination((prev) => ({ ...prev, page }));
                void load(page);
              }}
            >
              下一页
            </Button>
          </div>
        </section>
      )}
      {selectedTicket && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4 backdrop-blur-sm">
          <div className="max-h-[90vh] w-full max-w-2xl overflow-auto native-panel p-6">
            <div className="mb-5 flex items-start justify-between gap-4">
              <div>
                <h2 className="text-xl font-bold">{selectedTicket.title}</h2>
                <p className="mt-1 text-sm text-default-500">
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
            <div className="grid gap-3">
              {ticketMessages.map((message) => (
                <div
                  key={message.id}
                  className="native-muted-panel p-3 text-sm"
                >
                  <div className="mb-1 text-xs text-default-400">
                    {message.isAdmin ? "客服" : "我"} /{" "}
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
            {selectedTicket.status !== "closed" && (
              <div className="mt-4 flex gap-2">
                <input
                  className="h-10 flex-1 rounded-[var(--radius-control)] border border-input bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
                  placeholder="回复内容"
                  value={ticketReply}
                  onChange={(event) => setTicketReply(event.target.value)}
                />
                <Button
                  className="bg-primary text-white"
                  onPress={submitTicketReply}
                >
                  回复
                </Button>
              </div>
            )}
          </div>
        </div>
      )}
      {selectedOrder && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4 backdrop-blur-sm">
          <div className="w-full max-w-lg native-panel p-6">
            <div className="mb-5 flex items-start justify-between gap-4">
              <div>
                <h2 className="text-xl font-bold">订单详情</h2>
                <p className="mt-1 text-sm text-default-500">
                  {selectedOrder.orderNo}
                </p>
              </div>
              <Button variant="light" onPress={() => setSelectedOrder(null)}>
                关闭
              </Button>
            </div>
            <div className="grid gap-3 text-sm text-default-700">
              <div>
                套餐：
                {selectedOrder.planName || `套餐 #${selectedOrder.planId}`}
              </div>
              <div>
                类型：
                {orderTypeText[selectedOrder.orderType || "new"] ||
                  selectedOrder.orderType ||
                  "购买套餐"}
              </div>
              <div>金额：{formatMoney(selectedOrder.amountCents)}</div>
              <div>
                状态：{statusText[selectedOrder.status] || selectedOrder.status}
              </div>
              <div>支付方式：{selectedOrder.paymentProvider || "e支付"}</div>
              <div>
                创建时间：{new Date(selectedOrder.createdTime).toLocaleString()}
              </div>
              {selectedOrder.providerTradeNo && (
                <div>支付流水：{selectedOrder.providerTradeNo}</div>
              )}
            </div>
            <div className="mt-6 flex flex-wrap justify-end gap-2">
              {selectedOrder.canCancel && (
                <Button
                  color="danger"
                  isLoading={loadingOrderId === selectedOrder.id}
                  variant="flat"
                  onPress={() => void cancelOrder(selectedOrder)}
                >
                  取消订单
                </Button>
              )}
              {selectedOrder.canPay && (
                <Button
                  className="bg-primary text-white"
                  isLoading={loadingOrderId === selectedOrder.id}
                  onPress={() => void continuePay(selectedOrder)}
                >
                  继续付款
                </Button>
              )}
              {canBalancePayOrder(selectedOrder) && (
                <Button
                  isLoading={loadingOrderId === selectedOrder.id}
                  variant="flat"
                  onPress={() => void balancePay(selectedOrder)}
                >
                  余额支付
                </Button>
              )}
              {selectedOrder.canRefund && (
                <Button
                  color="warning"
                  isLoading={loadingOrderId === selectedOrder.id}
                  variant="flat"
                  onPress={() => {
                    setRefundTarget(selectedOrder);
                    setRefundReason("");
                  }}
                >
                  申请退款
                </Button>
              )}
            </div>
          </div>
        </div>
      )}
    </PageShell>
  );
}
