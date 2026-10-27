"use client";

import { useRouter } from "next/navigation";
import Link from "next/link";
import { apiFetch } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Lot } from "@/lib/types";
import LotForm, { LotFormValues } from "@/components/LotForm";

export default function NewLotPage() {
  const { token, user, loading } = useAuth();
  const router = useRouter();

  if (!loading && !user) {
    return (
      <p className="text-gray-700">
        Чтобы выставить лот, нужно{" "}
        <Link href="/login" className="text-brand-600 hover:underline">
          войти
        </Link>
        .
      </p>
    );
  }

  async function handleSubmit(values: LotFormValues) {
    const lot = await apiFetch<Lot>("/lots", { method: "POST", body: JSON.stringify(values) }, token);
    router.push(`/lots/${lot.id}`);
  }

  return (
    <div>
      <h1 className="mb-4 text-2xl font-bold text-gray-900">Выставить лот на аукцион</h1>
      <p className="mb-4 text-sm text-gray-600">
        Лот сохранится как черновик — отправить его на модерацию можно будет на странице лота.
      </p>
      <LotForm onSubmit={handleSubmit} submitLabel="Сохранить черновик" />
    </div>
  );
}
