"use client";

import { useState, FormEvent } from "react";
import { Lot, ListingType } from "@/lib/types";
import MediaUploader from "./MediaUploader";

export type LotFormValues = {
  listing_type: ListingType;
  brand: string;
  model: string;
  year: number;
  mileage_km: number;
  vin: string;
  condition: string;
  description: string;
  city: string;
  photos: string[];
  videos: string[];
  start_price_kgs: number;
  min_step_kgs: number;
  reserve_price_kgs?: number;
  start_at?: string;
  end_at?: string;
};

function toDatetimeLocal(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  const pad = (n: number) => n.toString().padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export default function LotForm({
  initial,
  lockListingType,
  onSubmit,
  submitLabel,
}: {
  initial?: Partial<Lot>;
  /** При редактировании тип объявления уже не меняется (см. backend Patch). */
  lockListingType?: boolean;
  onSubmit: (values: LotFormValues) => Promise<void>;
  submitLabel: string;
}) {
  const [listingType, setListingType] = useState<ListingType>(initial?.listing_type ?? "auction");
  const isAuction = listingType === "auction";

  const [brand, setBrand] = useState(initial?.brand ?? "");
  const [model, setModel] = useState(initial?.model ?? "");
  const [year, setYear] = useState(String(initial?.year ?? new Date().getFullYear()));
  const [mileageKm, setMileageKm] = useState(String(initial?.mileage_km ?? ""));
  const [vin, setVin] = useState(initial?.vin ?? "");
  const [condition, setCondition] = useState(initial?.condition ?? "good");
  const [description, setDescription] = useState(initial?.description ?? "");
  const [city, setCity] = useState(initial?.city ?? "");
  const [photos, setPhotos] = useState<string[]>(initial?.photos ?? []);
  const [videos, setVideos] = useState<string[]>(initial?.videos ?? []);
  const [startPrice, setStartPrice] = useState(String(initial?.start_price_kgs ?? ""));
  const [minStep, setMinStep] = useState(String(initial?.min_step_kgs ?? "10000"));
  const [reservePrice, setReservePrice] = useState(
    initial?.reserve_price_kgs ? String(initial.reserve_price_kgs) : ""
  );
  const defaultStart = toDatetimeLocal(initial?.start_at) || toDatetimeLocal(new Date().toISOString());
  const defaultEnd =
    toDatetimeLocal(initial?.end_at) ||
    toDatetimeLocal(new Date(Date.now() + 24 * 3600 * 1000).toISOString());
  const [startAt, setStartAt] = useState(defaultStart);
  const [endAt, setEndAt] = useState(defaultEnd);

  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await onSubmit({
        listing_type: listingType,
        brand,
        model,
        year: Number(year),
        mileage_km: Number(mileageKm),
        vin,
        condition,
        description,
        city,
        photos,
        videos,
        start_price_kgs: Number(startPrice),
        min_step_kgs: isAuction ? Number(minStep) : 0,
        reserve_price_kgs: isAuction && reservePrice ? Number(reservePrice) : undefined,
        start_at: isAuction ? new Date(startAt).toISOString() : undefined,
        end_at: isAuction ? new Date(endAt).toISOString() : undefined,
      });
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось сохранить лот");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="max-w-xl space-y-3 rounded-lg bg-white p-4 shadow-sm">
      <Field label="Тип объявления">
        <div className="flex gap-4 text-sm">
          <label className="flex items-center gap-1">
            <input
              type="radio"
              checked={isAuction}
              disabled={lockListingType}
              onChange={() => setListingType("auction")}
            />
            Аукцион (торги)
          </label>
          <label className="flex items-center gap-1">
            <input
              type="radio"
              checked={!isAuction}
              disabled={lockListingType}
              onChange={() => setListingType("fixed_price")}
            />
            Продажа по фиксированной цене
          </label>
        </div>
      </Field>

      <div className="grid grid-cols-2 gap-3">
        <Field label="Марка">
          <input required value={brand} onChange={(e) => setBrand(e.target.value)} className="input" />
        </Field>
        <Field label="Модель">
          <input required value={model} onChange={(e) => setModel(e.target.value)} className="input" />
        </Field>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Год выпуска">
          <input required type="number" value={year} onChange={(e) => setYear(e.target.value)} className="input" />
        </Field>
        <Field label="Пробег, км">
          <input
            required
            type="number"
            value={mileageKm}
            onChange={(e) => setMileageKm(e.target.value)}
            className="input"
          />
        </Field>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="VIN (опционально)">
          <input value={vin} onChange={(e) => setVin(e.target.value)} className="input" />
        </Field>
        <Field label="Состояние">
          <select value={condition} onChange={(e) => setCondition(e.target.value)} className="input">
            <option value="excellent">Отличное</option>
            <option value="good">Хорошее</option>
            <option value="average">Среднее</option>
            <option value="needs_repair">Требует ремонта</option>
          </select>
        </Field>
      </div>
      <Field label="Город">
        <input required value={city} onChange={(e) => setCity(e.target.value)} className="input" />
      </Field>
      <Field label="Описание">
        <textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          rows={4}
          className="input"
        />
      </Field>

      <Field label="Фото">
        <MediaUploader kind="photo" urls={photos} onChange={setPhotos} />
      </Field>
      <Field label="Видео (опционально)">
        <MediaUploader kind="video" urls={videos} onChange={setVideos} />
      </Field>

      <div className="grid grid-cols-2 gap-3">
        <Field label={isAuction ? "Стартовая цена, сом" : "Цена, сом"}>
          <input
            required
            type="number"
            value={startPrice}
            onChange={(e) => setStartPrice(e.target.value)}
            className="input"
          />
        </Field>
        {isAuction && (
          <Field label="Шаг ставки, сом">
            <input
              required
              type="number"
              value={minStep}
              onChange={(e) => setMinStep(e.target.value)}
              className="input"
            />
          </Field>
        )}
      </div>

      {isAuction && (
        <>
          <Field label="Резервная цена, сом (опционально)">
            <input
              type="number"
              value={reservePrice}
              onChange={(e) => setReservePrice(e.target.value)}
              className="input"
            />
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Начало торгов">
              <input
                required
                type="datetime-local"
                value={startAt}
                onChange={(e) => setStartAt(e.target.value)}
                className="input"
              />
            </Field>
            <Field label="Окончание торгов">
              <input
                required
                type="datetime-local"
                value={endAt}
                onChange={(e) => setEndAt(e.target.value)}
                className="input"
              />
            </Field>
          </div>
        </>
      )}

      {error && <p className="text-sm text-red-600">{error}</p>}

      <button
        type="submit"
        disabled={submitting}
        className="w-full rounded bg-brand-600 px-4 py-2 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-50"
      >
        {submitting ? "Сохраняем..." : submitLabel}
      </button>
    </form>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block text-sm text-gray-700">
      {label}
      <div className="mt-1">{children}</div>
    </label>
  );
}
