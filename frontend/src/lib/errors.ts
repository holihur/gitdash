import { ApiError } from "@/lib/api";

type Translator = (key: string, vars?: Record<string, string | number>) => string | undefined;

/** 把 API 错误转成用户可读文案：优先按后端错误码 i18n，否则回退到后端 message。 */
export function apiErrorMsg(
  to: Translator,
  err: unknown,
  fallbackKey?: string,
  fallbackVars?: Record<string, string | number>,
): string {
  if (err instanceof ApiError && err.code) {
    const localized = to(`errors.${err.code}`);
    if (localized) return localized;
  }
  if (err instanceof Error && err.message) return err.message;
  if (fallbackKey) return to(fallbackKey, fallbackVars) ?? fallbackKey;
  return String(err);
}
