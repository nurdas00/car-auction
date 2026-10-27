// Единая точка обращения к backend API. NEXT_PUBLIC_API_URL должен указывать
// на адрес, доступный из браузера (не из docker-сети) — см. docker-compose.yml.
const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export function apiUrl(path: string): string {
  return `${API_BASE}/api${path}`;
}

export function wsUrl(path: string, token: string): string {
  const wsBase = API_BASE.replace(/^http/, "ws");
  return `${wsBase}/api${path}?token=${encodeURIComponent(token)}`;
}

/** Backend отдаёт медиа как относительный путь ("/uploads/xxx.jpg") — достраиваем до абсолютного URL. */
export function mediaUrl(path: string): string {
  if (/^https?:\/\//.test(path)) return path; // уже абсолютный (например, вставленная вручную ссылка)
  return `${API_BASE}${path}`;
}

export type UploadResult = { url: string; kind: "image" | "video" };

/** Загружает один файл на backend и возвращает относительный URL (см. mediaUrl). */
export async function uploadFile(file: File, token: string | null): Promise<UploadResult> {
  const form = new FormData();
  form.append("file", file);
  const res = await fetch(apiUrl("/uploads"), {
    method: "POST",
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
    body: form,
  });
  const data = await res.json();
  if (!res.ok) {
    throw new ApiError(res.status, data?.error ?? res.statusText);
  }
  return data as UploadResult;
}

export async function apiFetch<T>(
  path: string,
  options: RequestInit = {},
  token?: string | null
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(options.headers as Record<string, string> | undefined),
  };
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const res = await fetch(apiUrl(path), { ...options, headers });

  if (res.status === 204) {
    return undefined as T;
  }

  const text = await res.text();
  const data = text ? JSON.parse(text) : null;

  if (!res.ok) {
    const message = data && typeof data.error === "string" ? data.error : res.statusText;
    throw new ApiError(res.status, message);
  }

  return data as T;
}
