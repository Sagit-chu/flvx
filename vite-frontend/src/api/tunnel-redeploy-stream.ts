import type { BatchOperationResult } from "@/api/types";

import axios from "axios";

import { clearSession, getToken } from "@/utils/session";

const STREAM_IDLE_TIMEOUT_MS = 2 * 60 * 1000;
const MAX_EVENT_LENGTH = 1024 * 1024;
const UNCERTAIN_RESULT =
  "下发进度连接中断，部分配置可能已执行，结果尚未确认。请核对节点与隧道状态后再决定是否重新下发，系统不会自动重试。";

export interface TunnelRedeployProgress {
  total: number;
  completed: number;
  success: number;
  failed: number;
}

export interface TunnelRedeployProcessing {
  id: number;
  name: string;
  progress: TunnelRedeployProgress;
}

export interface TunnelRedeployItem {
  result: {
    id: number;
    name: string;
    success: boolean;
    reason?: string;
  };
  progress: TunnelRedeployProgress;
}

interface TunnelRedeployCallbacks {
  onStart?: (progress: TunnelRedeployProgress) => void;
  onProcessing?: (payload: TunnelRedeployProcessing) => void;
  onItem?: (payload: TunnelRedeployItem) => void;
}

type RawObject = Record<string, unknown>;

class RequestRejectedError extends Error {}

const object = (value: unknown): RawObject => {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(UNCERTAIN_RESULT);
  }

  return value as RawObject;
};

const readProgress = (value: unknown): TunnelRedeployProgress => {
  const candidate = object(value);
  const { total, completed, success, failed } = candidate;

  if (
    ![total, completed, success, failed].every(
      (count) =>
        typeof count === "number" && Number.isSafeInteger(count) && count >= 0,
    ) ||
    (completed as number) > (total as number) ||
    (success as number) + (failed as number) !== completed
  ) {
    throw new Error(UNCERTAIN_RESULT);
  }

  return { total, completed, success, failed } as TunnelRedeployProgress;
};

const expireSession = () => {
  clearSession();
  if (window.location.pathname !== "/") window.location.href = "/";
};

/** A mutation stream must never be retried or replayed through the legacy API. */
export const batchRedeployTunnelsStream = async (
  ids: number[],
  callbacks: TunnelRedeployCallbacks,
  signal?: AbortSignal,
): Promise<BatchOperationResult> => {
  if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
  if (!ids.length || ids.some((id) => !Number.isSafeInteger(id) || id <= 0)) {
    throw new Error("请选择有效的隧道");
  }
  if (typeof fetch !== "function" || typeof TextDecoder === "undefined") {
    throw new Error("当前浏览器不支持实时下发进度，请使用支持流式响应的浏览器");
  }

  const requestedIds = new Set(ids);
  const controller = new AbortController();
  const abort = () => controller.abort();
  let timer: ReturnType<typeof setTimeout> | undefined;
  let reader: ReadableStreamDefaultReader<Uint8Array> | undefined;
  const resetTimeout = () => {
    clearTimeout(timer);
    timer = setTimeout(abort, STREAM_IDLE_TIMEOUT_MS);
  };

  signal?.addEventListener("abort", abort, { once: true });
  resetTimeout();

  try {
    const base = axios.defaults.baseURL || "/api/v1/";
    const response = await fetch(
      `${base.endsWith("/") ? base : `${base}/`}tunnel/batch-redeploy/stream`,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Accept: "application/x-ndjson, application/json",
          Authorization: getToken() || "",
        },
        body: JSON.stringify({ ids: Array.from(requestedIds) }),
        signal: controller.signal,
      },
    );

    if (response.status === 401) {
      expireSession();
      throw new RequestRejectedError("登录已过期，请重新登录");
    }
    if (response.status === 404 || response.status === 405) {
      throw new RequestRejectedError(
        "后端尚不支持实时下发进度，请升级后端后再操作；未自动改用旧接口下发",
      );
    }
    if (
      !response.headers.get("content-type")?.includes("application/x-ndjson")
    ) {
      let payload: RawObject | undefined;

      try {
        payload = object(await response.json());
      } catch {
        throw new Error(UNCERTAIN_RESULT);
      }
      if (payload.code === 401) {
        expireSession();
        throw new RequestRejectedError("登录已过期，请重新登录");
      }
      if (typeof payload.code === "number" && payload.code !== 0) {
        throw new RequestRejectedError(
          typeof payload.msg === "string" ? payload.msg : "下发请求失败",
        );
      }
      throw new Error(UNCERTAIN_RESULT);
    }
    if (!response.ok || !response.body) throw new Error(UNCERTAIN_RESULT);

    reader = response.body.getReader();
    const decoder = new TextDecoder("utf-8", { fatal: true });
    let buffer = "";
    let progress: TunnelRedeployProgress | undefined;
    let currentId: number | undefined;
    const completedIds = new Set<number>();
    let finalResult: BatchOperationResult | undefined;

    const processLine = (line: string) => {
      if (!line.trim()) return;
      const event = object(JSON.parse(line));
      const data = object(event.data);
      const next = readProgress(event.type === "start" ? data : data.progress);

      if (
        next.total !== requestedIds.size ||
        (progress &&
          (next.success < progress.success || next.failed < progress.failed))
      )
        throw new Error(UNCERTAIN_RESULT);

      if (event.type === "start") {
        if (progress || next.completed !== 0) throw new Error(UNCERTAIN_RESULT);
        progress = next;
        callbacks.onStart?.(next);

        return;
      }
      if (!progress) throw new Error(UNCERTAIN_RESULT);
      if (event.type === "processing") {
        if (
          typeof data.id !== "number" ||
          !requestedIds.has(data.id) ||
          typeof data.name !== "string" ||
          completedIds.has(data.id) ||
          currentId !== undefined ||
          next.completed !== progress.completed
        )
          throw new Error(UNCERTAIN_RESULT);
        currentId = data.id;
        callbacks.onProcessing?.({
          id: data.id,
          name: data.name,
          progress: next,
        });
      } else if (event.type === "item") {
        const result = object(data.result);

        if (
          result.id !== currentId ||
          currentId === undefined ||
          typeof result.name !== "string" ||
          typeof result.success !== "boolean" ||
          (result.reason !== undefined && typeof result.reason !== "string") ||
          next.completed !== progress.completed + 1 ||
          next.success !== progress.success + (result.success ? 1 : 0) ||
          next.failed !== progress.failed + (result.success ? 0 : 1)
        )
          throw new Error(UNCERTAIN_RESULT);
        completedIds.add(currentId);
        currentId = undefined;
        callbacks.onItem?.({
          result: result as unknown as TunnelRedeployItem["result"],
          progress: next,
        });
      } else if (event.type === "done") {
        const result = object(data.result);
        const failures = result.failures ?? [];

        if (
          currentId !== undefined ||
          next.completed !== next.total ||
          next.completed !== completedIds.size ||
          next.completed !== progress.completed ||
          result.successCount !== next.success ||
          result.failCount !== next.failed ||
          !Array.isArray(failures) ||
          failures.length !== next.failed ||
          failures.some((failure) => {
            const item = object(failure);

            return (
              typeof item.id !== "number" ||
              !completedIds.has(item.id) ||
              typeof item.reason !== "string" ||
              (item.name !== undefined && typeof item.name !== "string")
            );
          })
        )
          throw new Error(UNCERTAIN_RESULT);
        finalResult = result as unknown as BatchOperationResult;
      } else {
        throw new Error(UNCERTAIN_RESULT);
      }
      progress = next;
    };

    while (!finalResult) {
      const { value, done } = await reader.read();

      if (done) {
        buffer += decoder.decode();
        if (buffer.trim()) processLine(buffer);
        break;
      }
      resetTimeout();
      buffer += decoder.decode(value, { stream: true });
      let newline: number;

      while ((newline = buffer.indexOf("\n")) >= 0) {
        const line = buffer.slice(0, newline);

        buffer = buffer.slice(newline + 1);
        if (line.length > MAX_EVENT_LENGTH) throw new Error(UNCERTAIN_RESULT);
        processLine(line);
        if (finalResult) break;
      }
      if (buffer.length > MAX_EVENT_LENGTH) throw new Error(UNCERTAIN_RESULT);
    }
    if (!finalResult) throw new Error(UNCERTAIN_RESULT);

    return finalResult;
  } catch (error) {
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    if (error instanceof RequestRejectedError) throw error;
    throw new Error(UNCERTAIN_RESULT);
  } finally {
    clearTimeout(timer);
    controller.abort();
    signal?.removeEventListener("abort", abort);
    if (reader) {
      // Cancel only the response reader, never resend an interrupted mutation.
      await reader.cancel().catch(() => undefined);
      reader.releaseLock();
    }
  }
};
