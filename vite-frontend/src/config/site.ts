import { getConfigByName, getConfigs } from "@/api";

export type SiteConfig = typeof siteConfig;

// 缓存相关常量
const CACHE_PREFIX = "vite_config_";
const VERSION = import.meta.env.VITE_APP_VERSION || "dev";
const APP_VERSION = "1.0.3";
const GITHUB_REPO =
  import.meta.env.VITE_GITHUB_REPO || "https://github.com/Sagit-chu/flux-panel";

const getInitialConfig = () => {
  if (typeof window === "undefined") {
    return {
      name: "FLVX",
      version: VERSION,
      app_version: APP_VERSION,
      github_repo: GITHUB_REPO,
    };
  }

  const cachedAppName = localStorage.getItem(CACHE_PREFIX + "app_name");

  if (cachedAppName) {
    return {
      name: cachedAppName,
      version: VERSION,
      app_version: APP_VERSION,
      github_repo: GITHUB_REPO,
    };
  }

  return {
    name: "FLVX",
    version: VERSION,
    app_version: APP_VERSION,
  };
};

export const siteConfig = getInitialConfig();

// 缓存工具函数
export const configCache = {
  // 获取缓存的配置
  get: (key: string): string | null => {
    const cacheKey = CACHE_PREFIX + key;

    return localStorage.getItem(cacheKey);
  },

  // 设置缓存的配置
  set: (key: string, value: string): void => {
    const cacheKey = CACHE_PREFIX + key;

    localStorage.setItem(cacheKey, value);
  },

  // 删除指定配置的缓存
  remove: (key: string): void => {
    const cacheKey = CACHE_PREFIX + key;

    localStorage.removeItem(cacheKey);
  },

  // 清空所有配置缓存
  clear: (): void => {
    // 获取所有localStorage的key
    const keys = Object.keys(localStorage);

    keys.forEach((key) => {
      if (key.startsWith(CACHE_PREFIX)) {
        localStorage.removeItem(key);
      }
    });
  },
};

// 获取单个配置（优先从缓存）
export const getCachedConfig = async (key: string): Promise<string | null> => {
  const cachedValue = configCache.get(key);

  if (cachedValue !== null) {
    return cachedValue;
  }

  const response = await getConfigByName(key);

  if (response.code === 0 && response.data?.value) {
    const value = response.data.value;

    configCache.set(key, value);

    return value;
  }

  return null;
};

// 获取所有配置（优先从缓存）
export const getCachedConfigs = async (): Promise<Record<string, string>> => {
  // 尝试从缓存获取所有配置
  const configKeys = ["app_name"];
  const cachedConfigs: Record<string, string> = {};
  let hasCachedData = false;

  configKeys.forEach((key) => {
    const cachedValue = configCache.get(key);

    if (cachedValue !== null) {
      cachedConfigs[key] = cachedValue;
      hasCachedData = true;
    }
  });

  // 从API获取最新配置
  try {
    const response = await getConfigs();

    if (response.code === 0 && response.data) {
      const configs = response.data;

      // 将所有配置存入缓存
      Object.entries(configs).forEach(([key, value]) => {
        configCache.set(key, value as string);
      });

      return configs;
    }
  } catch {
    // API失败时返回缓存的数据
    if (hasCachedData) {
      return cachedConfigs;
    }
  }

  return {};
};

// 动态更新网站配置
export const updateSiteConfig = async () => {
  const appName = await getCachedConfig("app_name");

  if (appName && appName !== siteConfig.name) {
    siteConfig.name = appName;
    // 更新页面标题
    document.title = appName;
  }
};

// 清除配置缓存的工具函数
// 缓存清除时机：
// 1. 配置更新时：调用此函数清除所有缓存
// 2. 退出登录时：safeLogout()中的localStorage.clear()会自动清除
export const clearConfigCache = (keys?: string[]) => {
  if (keys && keys.length > 0) {
    // 删除指定的配置缓存
    keys.forEach((key) => configCache.remove(key));
  } else {
    // 清空所有配置缓存
    configCache.clear();
  }
};

// 在页面加载时异步更新配置（如果有更新的话）
if (typeof window !== "undefined") {
  // 延迟执行，避免阻塞初始渲染
  setTimeout(() => {
    updateSiteConfig();
  }, 200);
}
