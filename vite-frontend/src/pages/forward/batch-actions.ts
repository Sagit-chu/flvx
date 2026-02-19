import type { BatchOperationResult } from "@/api/types";

import {
  batchChangeTunnel,
  batchDeleteForwards,
  batchPauseForwards,
  batchRedeployForwards,
  batchResumeForwards,
} from "@/api";
import { extractApiErrorMessage } from "@/api/error-message";

export interface ForwardBatchActionOutcome {
  toastVariant: "success" | "error";
  toastMessage: string;
  shouldRefresh: boolean;
  closeDeleteModal?: boolean;
  closeChangeTunnelModal?: boolean;
  resetTargetTunnel?: boolean;
}

const normalizeBatchResult = (value: unknown): BatchOperationResult => {
  const raw = (value ?? {}) as Partial<BatchOperationResult>;

  return {
    successCount: Number(raw.successCount ?? 0),
    failCount: Number(raw.failCount ?? 0),
  };
};

const buildBatchToast = (
  result: BatchOperationResult,
  successText: string,
): Pick<ForwardBatchActionOutcome, "toastVariant" | "toastMessage"> => {
  if (result.failCount === 0) {
    return {
      toastVariant: "success",
      toastMessage: successText,
    };
  }

  return {
    toastVariant: "error",
    toastMessage: `成功 ${result.successCount} 项，失败 ${result.failCount} 项`,
  };
};

export const executeForwardBatchDelete = async (
  ids: number[],
): Promise<ForwardBatchActionOutcome> => {
  try {
    const response = await batchDeleteForwards(ids);

    if (response.code !== 0) {
      return {
        toastVariant: "error",
        toastMessage: response.msg || "删除失败",
        shouldRefresh: false,
      };
    }

    const summary = normalizeBatchResult(response.data);

    return {
      ...buildBatchToast(summary, `成功删除 ${summary.successCount} 项`),
      shouldRefresh: true,
      closeDeleteModal: true,
    };
  } catch (error) {
    return {
      toastVariant: "error",
      toastMessage: extractApiErrorMessage(error, "删除失败"),
      shouldRefresh: false,
    };
  }
};

export const executeForwardBatchToggleService = async (
  ids: number[],
  enable: boolean,
): Promise<ForwardBatchActionOutcome> => {
  const fallback = enable ? "启用失败" : "停用失败";

  try {
    const response = enable
      ? await batchResumeForwards(ids)
      : await batchPauseForwards(ids);

    if (response.code !== 0) {
      return {
        toastVariant: "error",
        toastMessage: response.msg || fallback,
        shouldRefresh: false,
      };
    }

    const summary = normalizeBatchResult(response.data);

    return {
      ...buildBatchToast(
        summary,
        enable
          ? `成功启用 ${summary.successCount} 项`
          : `成功停用 ${summary.successCount} 项`,
      ),
      shouldRefresh: true,
    };
  } catch (error) {
    return {
      toastVariant: "error",
      toastMessage: extractApiErrorMessage(error, fallback),
      shouldRefresh: false,
    };
  }
};

export const executeForwardBatchRedeploy = async (
  ids: number[],
): Promise<ForwardBatchActionOutcome> => {
  try {
    const response = await batchRedeployForwards(ids);

    if (response.code !== 0) {
      return {
        toastVariant: "error",
        toastMessage: response.msg || "下发失败",
        shouldRefresh: false,
      };
    }

    const summary = normalizeBatchResult(response.data);

    return {
      ...buildBatchToast(summary, `成功重新下发 ${summary.successCount} 项`),
      shouldRefresh: true,
    };
  } catch (error) {
    return {
      toastVariant: "error",
      toastMessage: extractApiErrorMessage(error, "下发失败"),
      shouldRefresh: false,
    };
  }
};

export const executeForwardBatchChangeTunnel = async (
  ids: number[],
  targetTunnelId: number,
): Promise<ForwardBatchActionOutcome> => {
  try {
    const response = await batchChangeTunnel({
      forwardIds: ids,
      targetTunnelId,
    });

    if (response.code !== 0) {
      return {
        toastVariant: "error",
        toastMessage: response.msg || "隧道失败",
        shouldRefresh: false,
      };
    }

    const summary = normalizeBatchResult(response.data);

    return {
      ...buildBatchToast(summary, `成功换隧道 ${summary.successCount} 项`),
      shouldRefresh: true,
      closeChangeTunnelModal: true,
      resetTargetTunnel: true,
    };
  } catch (error) {
    return {
      toastVariant: "error",
      toastMessage: extractApiErrorMessage(error, "隧道失败"),
      shouldRefresh: false,
    };
  }
};
