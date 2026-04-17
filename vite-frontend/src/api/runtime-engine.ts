import axios, { type AxiosResponse } from "axios";

import {
  extractApiErrorMessage,
  isUnauthorizedError,
} from "@/api/error-message";
import type {
  RuntimeEngineSettingsApiData,
  RuntimeEngineUpdatePayload,
} from "@/api/types";
import Network from "@/api/network";
import { clearSession, getToken } from "@/utils/session";

interface ApiResponse<T = unknown> {
  code: number;
  msg: string;
  data: T;
}

const handleTokenExpired = () => {
  clearSession();

  if (window.location.pathname !== "/") {
    window.location.href = "/";
  }
};

const put = async <T>(
  path: string,
  data: unknown,
): Promise<ApiResponse<T>> => {
  if (!axios.defaults.baseURL) {
    return {
      code: -1,
      msg: " - 请先设置面板地址",
      data: null as T,
    };
  }

  try {
    const response: AxiosResponse<ApiResponse<T>> = await axios.put(path, data, {
      timeout: 30_000,
      headers: {
        Authorization: getToken(),
        "Content-Type": "application/json",
      },
    });

    if (response.data?.code === 401) {
      handleTokenExpired();
    }

    return response.data;
  } catch (error: unknown) {
    if (isUnauthorizedError(error)) {
      handleTokenExpired();

      return {
        code: 401,
        msg: "未登录或token已过期",
        data: null as T,
      };
    }

    return {
      code: -1,
      msg: extractApiErrorMessage(error),
      data: null as T,
    };
  }
};

export const getRuntimeEngineSettings = () =>
  Network.get<RuntimeEngineSettingsApiData>("/system/runtime");

export const updateRuntimeEngineSettings = (data: RuntimeEngineUpdatePayload) =>
  put<RuntimeEngineSettingsApiData>("/system/runtime", data);

export const getRuntimeEngineProgress = () =>
  Network.get<RuntimeEngineSettingsApiData>("/system/runtime/progress");
