"use client";

import { mediaUrl } from "@/lib/api";

export default function MediaGallery({ photos, videos }: { photos: string[]; videos: string[] | null }) {
  const vids = videos ?? [];
  if (photos.length === 0 && vids.length === 0) {
    return (
      <div className="flex aspect-[4/3] w-full items-center justify-center rounded-lg bg-gray-100 text-gray-400">
        Нет фото
      </div>
    );
  }

  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
      {photos.map((url) => (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          key={url}
          src={mediaUrl(url)}
          alt=""
          className="aspect-[4/3] w-full rounded-lg object-cover"
        />
      ))}
      {vids.map((url) => (
        <video key={url} src={mediaUrl(url)} controls className="aspect-[4/3] w-full rounded-lg bg-black object-cover" />
      ))}
    </div>
  );
}
