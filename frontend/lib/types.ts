export type User = {
  id: string;
  email: string;
  name: string;
  phone: string;
  city: string;
  avatar_url: string;
  role: string;
  verification_level: string;
  is_blocked: boolean;
  created_at: string;
};

export type ListingType = "auction" | "fixed_price";

export type SellerContact = {
  name: string;
  phone: string;
};

export type Lot = {
  id: string;
  seller_id: string;
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
  videos: string[] | null;
  status: string;
  start_price_kgs: number;
  min_step_kgs: number;
  reserve_price_kgs?: number;
  current_price_kgs?: number;
  start_at?: string;
  end_at?: string;
  moderation_note?: string;
  created_at: string;
  updated_at: string;
  // Присутствует только для активной/проданной витрины (listing_type=fixed_price).
  seller_contact?: SellerContact;
};

export type Bid = {
  id: string;
  lot_id: string;
  bidder_id: string;
  amount_kgs: number;
  created_at: string;
};

// Статусы лота — должны совпадать с backend/internal/lot/lot.go.
// "active"/"finished_unsold" читаются по-разному для аукциона и витрины
// (у витрины нет торгов, поэтому "Идут торги"/"Не продан" там не к месту).
const LOT_STATUS_LABELS_AUCTION: Record<string, string> = {
  draft: "Черновик",
  pending: "На модерации",
  active: "Идут торги",
  finished_sold: "Продан",
  finished_unsold: "Не продан",
  rejected: "Отклонён",
  cancelled: "Отменён",
};

const LOT_STATUS_LABELS_FIXED_PRICE: Record<string, string> = {
  ...LOT_STATUS_LABELS_AUCTION,
  active: "В продаже",
  finished_unsold: "Снято с продажи",
};

export function lotStatusLabel(lot: Pick<Lot, "status" | "listing_type">): string {
  const labels = lot.listing_type === "fixed_price" ? LOT_STATUS_LABELS_FIXED_PRICE : LOT_STATUS_LABELS_AUCTION;
  return labels[lot.status] ?? lot.status;
}
