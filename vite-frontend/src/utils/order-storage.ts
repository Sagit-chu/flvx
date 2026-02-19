const parseOrderIds = (rawValue: string | null): number[] | null => {
  if (!rawValue) {
    return null;
  }

  try {
    const parsed = JSON.parse(rawValue);

    if (!Array.isArray(parsed)) {
      return null;
    }

    return parsed
      .map((id) => Number(id))
      .filter((id) => Number.isInteger(id) && id >= 0);
  } catch {
    return null;
  }
};

export const loadStoredOrder = (
  storageKey: string,
  sourceIds: number[],
): number[] => {
  const parsed = parseOrderIds(localStorage.getItem(storageKey));

  if (!parsed || parsed.length === 0) {
    return sourceIds;
  }

  const idSet = new Set(sourceIds);
  const validIds: number[] = [];

  parsed.forEach((id) => {
    if (idSet.has(id) && !validIds.includes(id)) {
      validIds.push(id);
    }
  });

  if (validIds.length === 0) {
    return sourceIds;
  }

  sourceIds.forEach((id) => {
    if (!validIds.includes(id)) {
      validIds.push(id);
    }
  });

  return validIds;
};

export const saveOrder = (storageKey: string, ids: number[]): void => {
  try {
    localStorage.setItem(storageKey, JSON.stringify(ids));
  } catch {}
};
