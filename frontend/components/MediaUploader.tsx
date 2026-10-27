"use client";

import { useRef, useState } from "react";
import { uploadFile, mediaUrl } from "@/lib/api";
import { useAuth } from "@/lib/auth";

export default function MediaUploader({
  kind,
  urls,
  onChange,
}: {
  kind: "photo" | "video";
  urls: string[];
  onChange: (urls: string[]) => void;
}) {
  const { token } = useAuth();
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  async function handleFiles(files: FileList | null) {
    if (!files || files.length === 0) return;
    setError(null);
    setUploading(true);
    try {
      const uploaded: string[] = [];
      for (const file of Array.from(files)) {
        const result = await uploadFile(file, token);
        uploaded.push(result.url);
      }
      onChange([...urls, ...uploaded]);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось загрузить файл");
    } finally {
      setUploading(false);
      if (inputRef.current) inputRef.current.value = "";
    }
  }

  function remove(url: string) {
    onChange(urls.filter((u) => u !== url));
  }

  return (
    <div>
      <div className="flex flex-wrap gap-2">
        {urls.map((url) => (
          <div key={url} className="relative h-20 w-20 overflow-hidden rounded border border-gray-200">
            {kind === "photo" ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img src={mediaUrl(url)} alt="" className="h-full w-full object-cover" />
            ) : (
              <video src={mediaUrl(url)} className="h-full w-full object-cover" muted />
            )}
            <button
              type="button"
              onClick={() => remove(url)}
              className="absolute right-0 top-0 flex h-5 w-5 items-center justify-center bg-black/60 text-xs text-white"
              aria-label="Удалить"
            >
              ×
            </button>
          </div>
        ))}
        <button
          type="button"
          onClick={() => inputRef.current?.click()}
          disabled={uploading}
          className="flex h-20 w-20 items-center justify-center rounded border border-dashed border-gray-300 text-xs text-gray-500 hover:border-brand-400 disabled:opacity-50"
        >
          {uploading ? "..." : "+ Добавить"}
        </button>
      </div>
      <input
        ref={inputRef}
        type="file"
        accept={kind === "photo" ? "image/*" : "video/*"}
        multiple={kind === "photo"}
        className="hidden"
        onChange={(e) => handleFiles(e.target.files)}
      />
      {error && <p className="mt-1 text-xs text-red-600">{error}</p>}
    </div>
  );
}
