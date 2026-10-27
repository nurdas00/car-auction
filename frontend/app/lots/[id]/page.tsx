"use client";

import { useEffect, useRef, useState, useCallback, FormEvent } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { apiFetch, wsUrl, ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Bid, Lot, lotStatusLabel } from "@/lib/types";
import { formatKGS, formatDateTime } from "@/lib/format";
import Countdown from "@/components/Countdown";
import MediaGallery from "@/components/MediaGallery";

type WsMessage =
  | { type: "state"; current_price_kgs: number; end_at: string }
  | { type: "bid_accepted"; current_price_kgs: number; end_at: string; extended: boolean }
  | { type: "finished"; status: string; final_price_kgs: number; sold: boolean };

export default function LotDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const { token, user, loading: authLoading } = useAuth();

  const [lot, setLot] = useState<Lot | null>(null);
  const [bids, setBids] = useState<Bid[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [needsLogin, setNeedsLogin] = useState(false);

  const [bidAmount, setBidAmount] = useState("");
  const [bidError, setBidError] = useState<string | null>(null);
  const [placingBid, setPlacingBid] = useState(false);
  const [extendedFlash, setExtendedFlash] = useState(false);
  const [finishedBanner, setFinishedBanner] = useState<string | null>(null);

  const [isFavorite, setIsFavorite] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  const wsRef = useRef<WebSocket | null>(null);

  const loadBids = useCallback(async () => {
    if (!token) return;
    try {
      const data = await apiFetch<Bid[] | null>(`/lots/${id}/bids`, {}, token);
      setBids(data ?? []);
    } catch {
      // история ставок не критична для основного функционала страницы
    }
  }, [id, token]);

  const loadLot = useCallback(async () => {
    if (!token) {
      setNeedsLogin(true);
      return;
    }
    try {
      const data = await apiFetch<Lot>(`/lots/${id}`, {}, token);
      setLot(data);
      setNeedsLogin(false);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        setNeedsLogin(true);
      } else {
        setError(e instanceof Error ? e.message : "Не удалось загрузить лот");
      }
    }
  }, [id, token]);

  const isAuction = lot?.listing_type !== "fixed_price";

  useEffect(() => {
    if (authLoading) return;
    loadLot();
    loadBids();
  }, [authLoading, loadLot, loadBids]);

  useEffect(() => {
    if (!token || !user) return;
    apiFetch<Lot[] | null>("/favorites", {}, token)
      .then((favs) => setIsFavorite((favs ?? []).some((l) => l.id === id)))
      .catch(() => {});
  }, [id, token, user]);

  // Живая трансляция — только для активных торгов аукциона (у витрины нет Room на backend).
  useEffect(() => {
    if (!token || !lot || lot.status !== "active" || lot.listing_type !== "auction") return;

    const ws = new WebSocket(wsUrl(`/ws/lots/${id}`, token));
    wsRef.current = ws;

    ws.onmessage = (event) => {
      const msg: WsMessage = JSON.parse(event.data);
      if (msg.type === "state" || msg.type === "bid_accepted") {
        setLot((prev) =>
          prev ? { ...prev, current_price_kgs: msg.current_price_kgs, end_at: msg.end_at } : prev
        );
        if (msg.type === "bid_accepted") {
          loadBids();
          if (msg.extended) {
            setExtendedFlash(true);
            setTimeout(() => setExtendedFlash(false), 4000);
          }
        }
      } else if (msg.type === "finished") {
        setLot((prev) => (prev ? { ...prev, status: msg.status, current_price_kgs: msg.final_price_kgs } : prev));
        setFinishedBanner(
          msg.sold
            ? `Торги завершены: лот продан за ${formatKGS(msg.final_price_kgs)}.`
            : "Торги завершены без продажи."
        );
        loadBids();
      }
    };

    return () => {
      ws.close();
      wsRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, token, lot?.status === "active", lot?.listing_type]);

  async function handleBid(e: FormEvent) {
    e.preventDefault();
    setBidError(null);
    const amount = Number(bidAmount);
    if (!amount || amount <= 0) {
      setBidError("Введите сумму ставки");
      return;
    }
    setPlacingBid(true);
    try {
      await apiFetch(`/lots/${id}/bids`, { method: "POST", body: JSON.stringify({ amount_kgs: amount }) }, token);
      setBidAmount("");
    } catch (e) {
      setBidError(e instanceof Error ? e.message : "Не удалось сделать ставку");
    } finally {
      setPlacingBid(false);
    }
  }

  async function handleFavorite() {
    setActionError(null);
    try {
      if (isFavorite) {
        await apiFetch(`/lots/${id}/favorite`, { method: "DELETE" }, token);
        setIsFavorite(false);
      } else {
        await apiFetch(`/lots/${id}/favorite`, { method: "POST" }, token);
        setIsFavorite(true);
      }
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Не удалось обновить избранное");
    }
  }

  async function handleSubmitForModeration() {
    setActionError(null);
    try {
      await apiFetch(`/lots/${id}/submit`, { method: "POST" }, token);
      loadLot();
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Не удалось отправить на модерацию");
    }
  }

  async function handleDelete() {
    setActionError(null);
    try {
      await apiFetch(`/lots/${id}`, { method: "DELETE" }, token);
      router.push("/my-lots");
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Не удалось удалить лот");
    }
  }

  async function handleMarkSold() {
    setActionError(null);
    try {
      await apiFetch(`/lots/${id}/mark-sold`, { method: "POST" }, token);
      loadLot();
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Не удалось отметить лот проданным");
    }
  }

  if (needsLogin) {
    return (
      <p className="text-gray-700">
        Чтобы посмотреть лот, нужно{" "}
        <Link href="/login" className="text-brand-600 hover:underline">
          войти
        </Link>
        .
      </p>
    );
  }

  if (error) return <p className="text-red-600">{error}</p>;
  if (!lot) return <p className="text-gray-500">Загрузка...</p>;

  const isOwner = user && user.id === lot.seller_id;
  const price = lot.current_price_kgs ?? lot.start_price_kgs;
  const minNextBid = price + lot.min_step_kgs;

  return (
    <div className="grid gap-6 md:grid-cols-3">
      <div className="md:col-span-2">
        <MediaGallery photos={lot.photos} videos={lot.videos} />

        <h1 className="mt-4 text-2xl font-bold text-gray-900">
          {lot.brand} {lot.model}, {lot.year}
        </h1>
        <p className="mt-1 text-gray-600">
          {lot.city} · {lot.mileage_km.toLocaleString("ru-RU")} км · состояние: {lot.condition || "—"}
        </p>
        <span className="mt-2 inline-block rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600">
          {lotStatusLabel(lot)}
        </span>

        <p className="mt-4 whitespace-pre-wrap text-gray-800">{lot.description || "Без описания."}</p>

        {lot.moderation_note && lot.status === "rejected" && (
          <p className="mt-3 rounded border border-red-200 bg-red-50 p-3 text-sm text-red-700">
            Причина отклонения: {lot.moderation_note}
          </p>
        )}

        {isAuction && (
          <>
            <h2 className="mt-6 text-lg font-semibold text-gray-900">История ставок</h2>
            {bids.length === 0 ? (
              <p className="mt-2 text-sm text-gray-500">Ставок пока не было.</p>
            ) : (
              <ul className="mt-2 divide-y divide-gray-100 rounded-lg bg-white shadow-sm">
                {bids.map((b) => (
                  <li key={b.id} className="flex justify-between px-4 py-2 text-sm">
                    <span className="font-medium text-gray-900">{formatKGS(b.amount_kgs)}</span>
                    <span className="text-gray-500">{formatDateTime(b.created_at)}</span>
                  </li>
                ))}
              </ul>
            )}
          </>
        )}
      </div>

      <div className="md:col-span-1">
        <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
          <p className="text-sm text-gray-500">{isAuction ? "Текущая цена" : "Цена"}</p>
          <p className="text-2xl font-bold text-brand-700">{formatKGS(price)}</p>

          {isAuction && lot.status === "active" && lot.end_at && (
            <div className="mt-2">
              <p className="text-sm text-gray-500">До конца торгов</p>
              <Countdown endAt={lot.end_at} />
              {extendedFlash && (
                <p className="mt-1 text-xs text-amber-600">Торги продлены — поступила ставка в последние секунды.</p>
              )}
            </div>
          )}

          {finishedBanner && (
            <p className="mt-3 rounded border border-brand-100 bg-brand-50 p-2 text-sm text-brand-700">
              {finishedBanner}
            </p>
          )}

          {isAuction && user && !isOwner && lot.status === "active" && (
            <form onSubmit={handleBid} className="mt-4 space-y-2">
              <label className="block text-sm text-gray-700">
                Ваша ставка (минимум {formatKGS(minNextBid)})
              </label>
              <input
                type="number"
                value={bidAmount}
                onChange={(e) => setBidAmount(e.target.value)}
                placeholder={String(minNextBid)}
                className="w-full rounded border border-gray-300 px-3 py-2 text-sm"
              />
              {bidError && <p className="text-sm text-red-600">{bidError}</p>}
              <button
                type="submit"
                disabled={placingBid}
                className="w-full rounded bg-brand-600 px-4 py-2 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-50"
              >
                {placingBid ? "Отправляем..." : "Сделать ставку"}
              </button>
            </form>
          )}

          {isAuction && !user && lot.status === "active" && (
            <p className="mt-4 text-sm text-gray-600">
              <Link href="/login" className="text-brand-600 hover:underline">
                Войдите
              </Link>
              , чтобы сделать ставку.
            </p>
          )}

          {!isAuction && !isOwner && (lot.status === "active" || lot.status === "finished_sold") && (
            <div className="mt-4 rounded border border-gray-200 bg-gray-50 p-3">
              <p className="text-sm font-medium text-gray-900">Контакты продавца</p>
              {lot.seller_contact ? (
                <>
                  <p className="mt-1 text-sm text-gray-700">{lot.seller_contact.name}</p>
                  <p className="text-sm text-gray-700">{lot.seller_contact.phone || "телефон не указан"}</p>
                </>
              ) : (
                <p className="mt-1 text-sm text-gray-500">Войдите, чтобы увидеть контакты.</p>
              )}
            </div>
          )}

          {!isAuction && lot.status === "finished_sold" && (
            <p className="mt-3 rounded border border-gray-200 bg-gray-50 p-2 text-sm text-gray-600">Продано.</p>
          )}

          {user && !isOwner && (
            <button
              onClick={handleFavorite}
              className="mt-4 w-full rounded border border-gray-300 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50"
            >
              {isFavorite ? "Убрать из избранного" : "В избранное"}
            </button>
          )}

          {isOwner && (lot.status === "draft" || lot.status === "rejected") && (
            <div className="mt-4 space-y-2">
              <Link
                href={`/lots/${lot.id}/edit`}
                className="block w-full rounded border border-gray-300 px-4 py-2 text-center text-sm text-gray-700 hover:bg-gray-50"
              >
                Редактировать
              </Link>
              <button
                onClick={handleSubmitForModeration}
                className="w-full rounded bg-brand-600 px-4 py-2 text-sm font-medium text-white hover:bg-brand-700"
              >
                Отправить на модерацию
              </button>
              {lot.status === "draft" && (
                <button
                  onClick={handleDelete}
                  className="w-full rounded border border-red-200 px-4 py-2 text-sm text-red-600 hover:bg-red-50"
                >
                  Удалить черновик
                </button>
              )}
            </div>
          )}

          {isOwner && lot.status === "pending" && (
            <p className="mt-4 text-sm text-gray-600">Лот отправлен на модерацию, ожидайте решения администратора.</p>
          )}

          {isOwner && !isAuction && lot.status === "active" && (
            <button
              onClick={handleMarkSold}
              className="mt-4 w-full rounded bg-brand-600 px-4 py-2 text-sm font-medium text-white hover:bg-brand-700"
            >
              Отметить как проданное
            </button>
          )}

          {actionError && <p className="mt-2 text-sm text-red-600">{actionError}</p>}
        </div>
      </div>
    </div>
  );
}
