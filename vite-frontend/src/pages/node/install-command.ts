export const tryCopyInstallCommand = async (
  command: string,
): Promise<boolean> => {
  try {
    await navigator.clipboard.writeText(command);

    return true;
  } catch {
    return false;
  }
};
