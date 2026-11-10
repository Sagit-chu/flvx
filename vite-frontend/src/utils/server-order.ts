interface OrderAvailability {
  isAdmin: boolean;
  searchKeyword: string;
  saving: boolean;
  busy: boolean;
}

export function getOrderDisabledReason({
  isAdmin,
  searchKeyword,
  saving,
  busy,
}: OrderAvailability): string {
  if (!isAdmin) return "仅管理员可调整全局顺序。";
  if (searchKeyword.length > 0) return "搜索期间无法排序，请清空搜索后重试。";
  if (saving) return "正在保存顺序，请稍候。";
  if (busy) return "正在加载或更新列表，请稍候。";

  return "";
}
