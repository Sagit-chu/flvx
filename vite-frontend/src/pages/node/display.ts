export const getConnectionStatusMeta = (
  status: "online" | "offline",
): { color: "success" | "danger"; text: string } => {
  if (status === "online") {
    return { color: "success", text: "在线" };
  }

  return { color: "danger", text: "离线" };
};

export const getRemoteSyncErrorMessage = (syncError: string): string => {
  if (syncError === "provider_share_deleted") {
    return "提供方已删除该分享";
  }

  if (syncError === "provider_share_disabled") {
    return "提供方已禁用该分享";
  }

  if (syncError === "provider_share_expired") {
    return "提供方分享已过期";
  }

  return `远程同步失败: ${syncError}`;
};
