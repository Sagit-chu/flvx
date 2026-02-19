export const SESSION_STORAGE_KEYS = {
  token: "token",
  roleId: "role_id",
  name: "name",
  admin: "admin",
} as const;

export interface SessionData {
  token: string | null;
  roleId: number | null;
  name: string | null;
  isAdmin: boolean;
}

export interface LoginSessionPayload {
  token: string;
  role_id: number;
  name: string;
}

const SESSION_EVENT_NAME = "sessionUpdated";

const parseRoleId = (value: string | null): number | null => {
  if (value === null) {
    return null;
  }

  const roleId = Number.parseInt(value, 10);

  return Number.isNaN(roleId) ? null : roleId;
};

export const getToken = (): string | null => {
  return localStorage.getItem(SESSION_STORAGE_KEYS.token);
};

export const getRoleId = (): number | null => {
  return parseRoleId(localStorage.getItem(SESSION_STORAGE_KEYS.roleId));
};

export const getSessionName = (): string | null => {
  return localStorage.getItem(SESSION_STORAGE_KEYS.name);
};

export const getAdminFlag = (): boolean => {
  const adminValue = localStorage.getItem(SESSION_STORAGE_KEYS.admin);

  if (adminValue !== null) {
    return adminValue === "true";
  }

  const roleId = getRoleId();
  const isAdmin = roleId === 0;

  if (roleId !== null) {
    localStorage.setItem(SESSION_STORAGE_KEYS.admin, String(isAdmin));
  }

  return isAdmin;
};

export const readSession = (): SessionData => {
  return {
    token: getToken(),
    roleId: getRoleId(),
    name: getSessionName(),
    isAdmin: getAdminFlag(),
  };
};

export const writeLoginSession = (payload: LoginSessionPayload): void => {
  localStorage.setItem(SESSION_STORAGE_KEYS.token, payload.token);
  localStorage.setItem(SESSION_STORAGE_KEYS.roleId, String(payload.role_id));
  localStorage.setItem(SESSION_STORAGE_KEYS.name, payload.name);
  localStorage.setItem(
    SESSION_STORAGE_KEYS.admin,
    String(payload.role_id === 0),
  );
  window.dispatchEvent(new Event(SESSION_EVENT_NAME));
};

export const clearSession = (): void => {
  localStorage.removeItem(SESSION_STORAGE_KEYS.token);
  localStorage.removeItem(SESSION_STORAGE_KEYS.roleId);
  localStorage.removeItem(SESSION_STORAGE_KEYS.name);
  localStorage.removeItem(SESSION_STORAGE_KEYS.admin);
  window.dispatchEvent(new Event(SESSION_EVENT_NAME));
};

export const SESSION_UPDATED_EVENT = SESSION_EVENT_NAME;
