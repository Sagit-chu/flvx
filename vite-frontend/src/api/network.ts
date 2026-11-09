import axios, { AxiosResponse } from "axios";

import {
  extractApiErrorMessage,
  isUnauthorizedError,
} from "@/api/error-message";
import { getPanelAddresses, isWebViewFunc } from "@/utils/panel";
import { clearSession, getToken } from "@/utils/session";

interface PanelAddress {
  name: string;
  address: string;
  inx: boolean;
}

const setPanelAddressesFunc = (newAddress: PanelAddress[]) => {
  newAddress.forEach((item) => {
    if (item.inx) {
      baseURL = `${item.address}/api/v1/`;
      axios.defaults.baseURL = baseURL;
    }
  });
};

function getWebViewPanelAddress() {
  (window as any).setAddresses = setPanelAddressesFunc;
  getPanelAddresses("setAddresses");
}

let baseURL: string = "";

export const reinitializeBaseURL = () => {
  if (isWebViewFunc()) {
    getWebViewPanelAddress();
  } else {
    baseURL = import.meta.env.VITE_API_BASE
      ? `${import.meta.env.VITE_API_BASE}/api/v1/`
      : "/api/v1/";
    axios.defaults.baseURL = baseURL;
  }
};

reinitializeBaseURL();

interface ApiResponse<T = unknown> {
  code: number;
  msg: string;
  data: T;
  transportError?: boolean;
}

interface RequestOptions {
  timeout?: number;
}

// 处理token失效的逻辑
function handleTokenExpired() {
  clearSession();

  // 跳转到登录页面
  if (window.location.pathname !== "/") {
    window.location.href = "/";
  }
}

// 检查响应是否为token失效
function isTokenExpired(response: ApiResponse<unknown>) {
  return (
    response &&
    response.code === 401 &&
    (response.msg === "未登录或token已过期" ||
      response.msg === "无效的token或token已过期" ||
      response.msg === "无法获取用户权限信息")
  );
}

const Network = {
  get: function <T = unknown>(
    path: string = "",
    data: unknown = {},
    options: RequestOptions = {},
  ): Promise<ApiResponse<T>> {
    return new Promise(function (resolve) {
      // 如果baseURL是默认值且是WebView环境，说明没有设置面板地址
      if (baseURL === "") {
        resolve({
          code: -1,
          msg: " - 请先设置面板地址",
          data: null as T,
          transportError: true,
        });

        return;
      }

      axios
        .get(path, {
          params: data,
          timeout: options.timeout ?? 30000,
          headers: {
            Authorization: getToken(),
          },
        })
        .then(function (response: AxiosResponse<ApiResponse<T>>) {
          // 检查是否token失效
          if (isTokenExpired(response.data)) {
            handleTokenExpired();
          }

          resolve(response.data);
        })
        .catch(function (error: unknown) {
          const errorMessage = extractApiErrorMessage(error);

          // 检查是否是401错误（token失效）
          if (isUnauthorizedError(error)) {
            handleTokenExpired();

            resolve({
              code: 401,
              msg: "未登录或token已过期",
              data: null as T,
            });

            return;
          }

          resolve({
            code: -1,
            transportError: true,
            msg: errorMessage,
            data: null as T,
          });
        });
    });
  },

  post: function <T = unknown>(
    path: string = "",
    data: unknown = {},
    options: RequestOptions = {},
  ): Promise<ApiResponse<T>> {
    return new Promise(function (resolve) {
      // 如果baseURL是默认值且是WebView环境，说明没有设置面板地址
      if (baseURL === "") {
        resolve({
          code: -1,
          msg: " - 请先设置面板地址",
          data: null as T,
          transportError: true,
        });

        return;
      }

      axios
        .post(path, data, {
          timeout: options.timeout ?? 30000,
          headers: {
            Authorization: getToken(),
            "Content-Type": "application/json",
          },
        })
        .then(function (response: AxiosResponse<ApiResponse<T>>) {
          // 检查是否token失效
          if (isTokenExpired(response.data)) {
            handleTokenExpired();
          }

          resolve(response.data);
        })
        .catch(function (error: unknown) {
          const errorMessage = extractApiErrorMessage(error);

          // 检查是否是401错误（token失效）
          if (isUnauthorizedError(error)) {
            handleTokenExpired();

            resolve({
              code: 401,
              msg: "未登录或token已过期",
              data: null as T,
            });

            return;
          }

          resolve({
            code: -1,
            transportError: true,
            msg: errorMessage,
            data: null as T,
          });
        });
    });
  },
};

export default Network;
