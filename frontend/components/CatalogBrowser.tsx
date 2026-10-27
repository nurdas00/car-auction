"use client";

import { useEffect, useState, FormEvent } from "react";
import { apiFetch } from "@/lib/api";
import { Lot, ListingType } from "@/lib/types";
import LotCard from "@/components/LotCard";

export default function CatalogBrowser({
  listingType,
  title,
  emptyText,
}: {
  listingType: ListingType;
  title: string;
  emptyText: string;
}) {
  const [lots, setLots] = useState<Lot[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const [brand, setBrand] = useState("");
  const [city, setCity] = useState("");
  const [priceMin, setPriceMin] = useState("");
  const [priceMax, setPriceMax] = useState("");

  async function load(params: Record<string, string>) {
    setError(null);
    const allParams: Record<string, string> = { type: listingType, ...params };
    const query = new URLSearchParams(
      Object.entries(allParams).filter(([, v]) => v !== "")
    ).toString();
    try {
      const data = await apiFetch<Lot[] | null>(`/lots${query ? `?${query}` : ""}`);
      setLots(data ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось загрузить список");
    }
  }

  useEffect(() => {
    load({});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [listingType]);

  function handleFilter(e: FormEvent) {
    e.preventDefault();
    load({ brand, city, price_min_kgs: priceMin, price_max_kgs: priceMax });
  }

  return (
    <div>
      <h1 className="mb-4 text-2xl font-bold text-gray-900">{title}</h1>

      <form onSubmit={handleFilter} className="mb-6 flex flex-wrap gap-2 rounded-lg bg-white p-3 shadow-sm">
        <input
          placeholder="Марка"
          value={brand}
          onChange={(e) => setBrand(e.target.value)}
          className="w-32 rounded border border-gray-300 px-2 py-1 text-sm"
        />
        <input
          placeholder="Город"
          value={city}
          onChange={(e) => setCity(e.target.value)}
          className="w-32 rounded border border-gray-300 px-2 py-1 text-sm"
        />
        <input
          placeholder="Цена от"
          type="number"
          value={priceMin}
          onChange={(e) => setPriceMin(e.target.value)}
          className="w-28 rounded border border-gray-300 px-2 py-1 text-sm"
        />
        <input
          placeholder="Цена до"
          type="number"
          value={priceMax}
          onChange={(e) => setPriceMax(e.target.value)}
          className="w-28 rounded border border-gray-300 px-2 py-1 text-sm"
        />
        <button type="submit" className="rounded bg-brand-600 px-4 py-1 text-sm text-white hover:bg-brand-700">
          Найти
        </button>
      </form>

      {error && <p className="mb-4 text-sm text-red-600">{error}</p>}

      {lots === null && !error && <p className="text-gray-500">Загрузка...</p>}
      {lots && lots.length === 0 && <p className="text-gray-500">{emptyText}</p>}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {lots?.map((lot) => <LotCard key={lot.id} lot={lot} />)}
      </div>
    </div>
  );
}
