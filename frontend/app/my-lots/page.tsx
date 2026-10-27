"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { apiFetch } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Lot, lotStatusLabel } from "@/lib/types";
import { formatKGS } from "@/lib/format";

export default function MyLotsPage() {
  const { token, user, loading } = useAuth();
  const [lots, setLots] = useState<Lot[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (loading || !token) return;
    apiFetch<Lot[] | null>("/lots/mine", {}, token)
      .then((data) => setLots(data ?? []))
      .catch((e) => setError(e instanceof Error ? e.message : "Не удалось загрузить лоты"));
  }, [token, loading]);

  if (!loading && !user) {
    return (
      <p className="text-gray-700">
        Чтобы увидеть свои лоты, нужно{" "}
        <Link href="/login" className="text-brand-600 hover:underline">
          войти
        </Link>
        .
      </p>
    );
  }

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">Мои лоты</h1>
        <Link href="/lots/new" className="rounded bg-brand-600 px-4 py-2 text-sm text-white hover:bg-brand-700">
          + Новый лот
        </Link>
      </div>

      {error && <p className="text-red-600">{error}</p>}
      {lots === null && !error && <p className="text-gray-500">Загрузка...</p>}
      {lots && lots.length === 0 && <p className="text-gray-500">У вас пока нет лотов.</p>}

      <ul className="space-y-2">
        {lots?.map((lot) => (
          <li key={lot.id}>
            <Link
              href={`/lots/${lot.id}`}
              className="flex items-center justify-between rounded-lg border border-gray-200 bg-white p-4 shadow-sm hover:shadow-md"
            >
              <div>
                <p className="font-medium text-gray-900">
                  {lot.brand} {lot.model}, {lot.year}
                </p>
                <p className="text-sm text-gray-500">{formatKGS(lot.current_price_kgs ?? lot.start_price_kgs)}</p>
              </div>
              <span className="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600">
                {lotStatusLabel(lot)}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
