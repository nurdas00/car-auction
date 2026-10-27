"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { apiFetch } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Lot } from "@/lib/types";
import LotCard from "@/components/LotCard";

export default function FavoritesPage() {
  const { token, user, loading } = useAuth();
  const [lots, setLots] = useState<Lot[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (loading || !token) return;
    apiFetch<Lot[] | null>("/favorites", {}, token)
      .then((data) => setLots(data ?? []))
      .catch((e) => setError(e instanceof Error ? e.message : "Не удалось загрузить избранное"));
  }, [token, loading]);

  if (!loading && !user) {
    return (
      <p className="text-gray-700">
        Чтобы увидеть избранное, нужно{" "}
        <Link href="/login" className="text-brand-600 hover:underline">
          войти
        </Link>
        .
      </p>
    );
  }

  return (
    <div>
      <h1 className="mb-4 text-2xl font-bold text-gray-900">Избранное</h1>
      {error && <p className="text-red-600">{error}</p>}
      {lots === null && !error && <p className="text-gray-500">Загрузка...</p>}
      {lots && lots.length === 0 && <p className="text-gray-500">Вы ещё ничего не добавили в избранное.</p>}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {lots?.map((lot) => <LotCard key={lot.id} lot={lot} />)}
      </div>
    </div>
  );
}
