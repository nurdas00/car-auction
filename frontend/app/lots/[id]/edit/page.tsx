"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { apiFetch } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Lot } from "@/lib/types";
import LotForm, { LotFormValues } from "@/components/LotForm";

export default function EditLotPage() {
  const { id } = useParams<{ id: string }>();
  const { token, loading: authLoading } = useAuth();
  const router = useRouter();
  const [lot, setLot] = useState<Lot | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (authLoading || !token) return;
    apiFetch<Lot>(`/lots/${id}`, {}, token)
      .then(setLot)
      .catch((e) => setError(e instanceof Error ? e.message : "Не удалось загрузить лот"));
  }, [id, token, authLoading]);

  async function handleSubmit(values: LotFormValues) {
    await apiFetch(`/lots/${id}`, { method: "PATCH", body: JSON.stringify(values) }, token);
    router.push(`/lots/${id}`);
  }

  if (error) return <p className="text-red-600">{error}</p>;
  if (!lot) return <p className="text-gray-500">Загрузка...</p>;

  if (lot.status !== "draft" && lot.status !== "rejected") {
    return <p className="text-gray-700">Редактировать можно только черновик или отклонённый лот.</p>;
  }

  return (
    <div>
      <h1 className="mb-4 text-2xl font-bold text-gray-900">Редактирование лота</h1>
      <LotForm initial={lot} lockListingType onSubmit={handleSubmit} submitLabel="Сохранить изменения" />
    </div>
  );
}
