// ═══════════════════════════════════════════════════════════
// ERROR HANDLING
// Backend'den gelen error code'ları kullanıcıya gösterilebilir
// mesaja çevirir. i18n `errors:` namespace'ini kullanır.
// ═══════════════════════════════════════════════════════════

export function getErrorMessage(errorCode, t) {
  if (!errorCode || typeof errorCode !== "string") {
    return t("errors.generic");
  }

  const key = `errors.${errorCode}`;
  const msg = t(key);

  return msg === key ? t("errors.generic") : msg;
}

export async function extractErrorMessage(res, t) {
  try {
    const data = await res.json();
    return getErrorMessage(data?.error, t);
  } catch {
    return t("errors.generic");
  }
}
