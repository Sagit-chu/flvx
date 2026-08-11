import { getConfigByName, getConfigs, getPublicConfigByName } from "@/api";
import { isLoggedIn } from "@/utils/auth";

export type SiteConfig = typeof siteConfig;

// 缓存相关常量
const CACHE_PREFIX = "vite_config_";
const VERSION = import.meta.env.VITE_APP_VERSION || "dev";
const APP_VERSION = "1.0.3";
const DEFAULT_FAVICON = "/favicon.ico";
const FAVICON_LINK_ID = "app-favicon";
const PUBLIC_BRAND_CONFIG_KEYS = [
  "app_name",
  "app_logo",
  "app_favicon",
  "app_bg_image",
] as const;
const SENSITIVE_CONFIG_KEYS = new Set([
  "jwt_secret",
  "license_key",
  "cloudflare_secret_key",
]);
const GITHUB_REPO =
  import.meta.env.VITE_GITHUB_REPO || "https://github.com/Sagit-chu/flux-panel";

const shouldPersistConfigKey = (key: string) => {
  return !SENSITIVE_CONFIG_KEYS.has(key.trim().toLowerCase());
};

const purgeSensitiveConfigCache = () => {
  SENSITIVE_CONFIG_KEYS.forEach((key) => {
    localStorage.removeItem(CACHE_PREFIX + key);
  });
};

const readCachedConfigs = (keys: readonly string[]) => {
  const cachedConfigs: Record<string, string> = {};
  let hasCachedData = false;

  keys.forEach((key) => {
    const cachedValue = configCache.get(key);

    if (cachedValue !== null) {
      cachedConfigs[key] = cachedValue;
      hasCachedData = true;
    }
  });

  return { cachedConfigs, hasCachedData };
};

const readAllCachedSafeConfigs = () => {
  const cachedConfigs: Record<string, string> = {};

  Object.keys(localStorage).forEach((storageKey) => {
    if (!storageKey.startsWith(CACHE_PREFIX)) {
      return;
    }

    const key = storageKey.slice(CACHE_PREFIX.length).trim().toLowerCase();

    if (!key || SENSITIVE_CONFIG_KEYS.has(key)) {
      return;
    }

    const value = localStorage.getItem(storageKey);

    if (value !== null) {
      cachedConfigs[key] = value;
    }
  });

  return cachedConfigs;
};

const fetchPublicBrandConfigs = async (): Promise<Record<string, string>> => {
  const publicConfigMap: Record<string, string> = {};

  await Promise.all(
    PUBLIC_BRAND_CONFIG_KEYS.map(async (key) => {
      try {
        const response = await getPublicConfigByName(key);

        if (
          response.code === 0 &&
          response.data &&
          typeof response.data.value === "string"
        ) {
          const value = response.data.value;

          publicConfigMap[key] = value;
          configCache.set(key, value);
        }
      } catch {
        // ignore single key fetch error
      }
    }),
  );

  return publicConfigMap;
};

const getInitialConfig = () => {
  if (typeof window === "undefined") {
    return {
      name: "FLVX",
      version: VERSION,
      app_version: APP_VERSION,
      github_repo: GITHUB_REPO,
      app_logo: "",
      app_favicon: "",
      app_bg_image: "",
      is_commercial: false,
      hide_footer_brand: false,
    };
  }

  purgeSensitiveConfigCache();

  const cachedAppName = localStorage.getItem(CACHE_PREFIX + "app_name");
  const cachedAppLogo = localStorage.getItem(CACHE_PREFIX + "app_logo") || "";
  const cachedAppFavicon =
    localStorage.getItem(CACHE_PREFIX + "app_favicon") || "";
  const cachedAppBgImage =
    localStorage.getItem(CACHE_PREFIX + "app_bg_image") || "";
  const isCommercial =
    localStorage.getItem(CACHE_PREFIX + "is_commercial") === "true";
  const hideFooterBrand =
    localStorage.getItem(CACHE_PREFIX + "hide_footer_brand") === "true";

  if (cachedAppName) {
    return {
      name: cachedAppName,
      version: VERSION,
      app_version: APP_VERSION,
      github_repo: GITHUB_REPO,
      app_logo: cachedAppLogo,
      app_favicon: cachedAppFavicon,
      app_bg_image: cachedAppBgImage,
      is_commercial: isCommercial,
      hide_footer_brand: hideFooterBrand,
    };
  }

  return {
    name: "FLVX",
    version: VERSION,
    app_version: APP_VERSION,
    github_repo: GITHUB_REPO,
    app_logo: cachedAppLogo,
    app_favicon: cachedAppFavicon,
    app_bg_image: cachedAppBgImage,
    is_commercial: isCommercial,
    hide_footer_brand: hideFooterBrand,
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
    const normalizedKey = key.trim().toLowerCase();

    if (!shouldPersistConfigKey(normalizedKey)) {
      configCache.remove(normalizedKey);

      return;
    }

    const cacheKey = CACHE_PREFIX + normalizedKey;

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

  if (
    response.code === 0 &&
    response.data &&
    typeof response.data.value === "string"
  ) {
    const value = response.data.value;

    if (shouldPersistConfigKey(key)) {
      configCache.set(key, value);
    }

    return value;
  }

  return null;
};

// 获取所有配置（优先从缓存）
export const getCachedConfigs = async (): Promise<Record<string, string>> => {
  const {
    cachedConfigs: publicCachedConfigs,
    hasCachedData: hasPublicCachedData,
  } = readCachedConfigs(PUBLIC_BRAND_CONFIG_KEYS);

  if (!isLoggedIn()) {
    const publicConfigs = await fetchPublicBrandConfigs();

    if (Object.keys(publicConfigs).length > 0) {
      return { ...publicCachedConfigs, ...publicConfigs };
    }

    return publicCachedConfigs;
  }

  const cachedConfigs = readAllCachedSafeConfigs();
  const hasCachedData = Object.keys(cachedConfigs).length > 0;

  // 从API获取最新配置
  try {
    const response = await getConfigs();

    if (response.code === 0 && response.data) {
      const configs = response.data;

      // 仅将安全配置存入缓存，敏感项会从 localStorage 中移除
      Object.entries(configs).forEach(([key, value]) => {
        const normalizedKey = key.trim().toLowerCase();

        if (SENSITIVE_CONFIG_KEYS.has(normalizedKey)) {
          configCache.remove(normalizedKey);

          return;
        }

        if (shouldPersistConfigKey(normalizedKey)) {
          configCache.set(normalizedKey, value as string);
        }
      });

      return configs;
    }

    if (hasCachedData) {
      return cachedConfigs;
    }

    const publicConfigs = await fetchPublicBrandConfigs();

    return { ...publicCachedConfigs, ...publicConfigs };
  } catch {
    // API失败时返回缓存的数据
    if (hasCachedData) {
      return cachedConfigs;
    }

    const publicConfigs = await fetchPublicBrandConfigs();

    return hasPublicCachedData
      ? { ...publicCachedConfigs, ...publicConfigs }
      : publicConfigs;
  }
};

const updateDocumentFavicon = (faviconUrl: string) => {
  if (typeof document === "undefined") {
    return;
  }

  const normalized = faviconUrl.trim() || DEFAULT_FAVICON;

  let iconLink = document.head.querySelector<HTMLLinkElement>(
    `link#${FAVICON_LINK_ID}`,
  );

  if (!iconLink) {
    iconLink = document.createElement("link");
    iconLink.id = FAVICON_LINK_ID;
    iconLink.rel = "icon";
    document.head.appendChild(iconLink);
  }

  iconLink.rel = "icon";
  iconLink.href = normalized;
  if (normalized.startsWith("data:image/png")) {
    iconLink.type = "image/png";
  } else {
    iconLink.removeAttribute("type");
  }

  let shortcutIconLink = document.head.querySelector<HTMLLinkElement>(
    'link[rel="shortcut icon"]',
  );

  if (!shortcutIconLink) {
    shortcutIconLink = document.createElement("link");
    shortcutIconLink.rel = "shortcut icon";
    document.head.appendChild(shortcutIconLink);
  }

  shortcutIconLink.href = normalized;
  if (normalized.startsWith("data:image/png")) {
    shortcutIconLink.type = "image/png";
  } else {
    shortcutIconLink.removeAttribute("type");
  }

  const duplicatedIcons = Array.from(
    document.head.querySelectorAll<HTMLLinkElement>('link[rel="icon"]'),
  ).filter((link) => link !== iconLink);

  duplicatedIcons.forEach((link) => link.remove());
};

// 动态更新网站配置
export const updateSiteConfig = async (configMap?: Record<string, string>) => {
  const resolvedConfigMap = configMap ?? (await getCachedConfigs());

  Object.entries(resolvedConfigMap).forEach(([key, value]) => {
    const normalizedKey = key.trim().toLowerCase();

    if (SENSITIVE_CONFIG_KEYS.has(normalizedKey)) {
      configCache.remove(normalizedKey);

      return;
    }

    if (shouldPersistConfigKey(normalizedKey)) {
      configCache.set(normalizedKey, String(value));
    }
  });

  const hasAppName = Object.prototype.hasOwnProperty.call(
    resolvedConfigMap,
    "app_name",
  );
  const hasAppLogo = Object.prototype.hasOwnProperty.call(
    resolvedConfigMap,
    "app_logo",
  );
  const hasAppFavicon = Object.prototype.hasOwnProperty.call(
    resolvedConfigMap,
    "app_favicon",
  );
  const hasAppBgImage = Object.prototype.hasOwnProperty.call(
    resolvedConfigMap,
    "app_bg_image",
  );

  const appName = hasAppName
    ? String(resolvedConfigMap.app_name || "").trim()
    : siteConfig.name;
  const appLogo = hasAppLogo
    ? String(resolvedConfigMap.app_logo || "").trim()
    : (siteConfig.app_logo || "").trim();
  const appFavicon = hasAppFavicon
    ? String(resolvedConfigMap.app_favicon || "").trim()
    : (siteConfig.app_favicon || "").trim();
  const appBgImage = hasAppBgImage
    ? String(resolvedConfigMap.app_bg_image || "").trim()
    : (siteConfig.app_bg_image || "").trim();

  if (appName && appName !== siteConfig.name) {
    siteConfig.name = appName;
  }

  siteConfig.app_logo = appLogo;
  siteConfig.app_favicon = appFavicon;
  siteConfig.app_bg_image = appBgImage;
  if (
    Object.prototype.hasOwnProperty.call(resolvedConfigMap, "is_commercial")
  ) {
    siteConfig.is_commercial = resolvedConfigMap.is_commercial === "true";
  }
  if (
    Object.prototype.hasOwnProperty.call(resolvedConfigMap, "hide_footer_brand")
  ) {
    siteConfig.hide_footer_brand =
      resolvedConfigMap.hide_footer_brand === "true";
  }

  if (typeof document !== "undefined") {
    document.title = siteConfig.name;
    window.dispatchEvent(new Event("site-config-updated"));
  }
  updateDocumentFavicon(siteConfig.app_favicon);
};

// 清除配置缓存的工具函数（用于需要强制重拉配置的场景）
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
  if (typeof document !== "undefined") {
    document.title = siteConfig.name;
  }
  updateDocumentFavicon(siteConfig.app_favicon);

  // 延迟执行，避免阻塞初始渲染
  setTimeout(() => {
    void updateSiteConfig();
  }, 50);
}
