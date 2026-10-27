"use client";

import Link from "next/link";
import { Lot, lotStatusLabel } from "@/lib/types";
import { formatKGS } from "@/lib/format";
import { mediaUrl } from "@/lib/api";
import Countdown from "./Countdown";

export default function LotCard({ lot }: { lot: Lot }) {
  const price = lot.current_price_kgs ?? lot.start_price_kgs;
  const isFixedPrice = lot.listing_type === "fixed_price";
  const thumbnail = lot.photos?.[0];

  return (
    <Link
      href={`/lots/${lot.id}`}
      className="block overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm transition hover:shadow-md"
    >
      <div className="aspect-[4/3] w-full bg-gray-100">
        {thumbnail ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={mediaUrl(thumbnail)} alt={`${lot.brand} ${lot.model}`} className="h-full w-full object-cover" />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-sm text-gray-400">Нет фото</div>
        )}
      </div>
      <div className="p-4">
        <div className="flex items-start justify-between gap-2">
          <h3 className="font-semibold text-gray-900">
            {lot.brand} {lot.model}, {lot.year}
          </h3>
          <span className="shrink-0 rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600">
            {lotStatusLabel(lot)}
          </span>
        </div>
        <p className="mt-1 text-sm text-gray-500">
          {lot.city} · {lot.mileage_km.toLocaleString("ru-RU")} км
        </p>
        <div className="mt-3 flex items-center justify-between">
          <span className="text-lg font-bold text-brand-700">{formatKGS(price)}</span>
          {!isFixedPrice && lot.status === "active" && lot.end_at && <Countdown endAt={lot.end_at} compact />}
        </div>
      </div>
    </Link>
  );
}
