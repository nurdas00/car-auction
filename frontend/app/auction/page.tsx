import CatalogBrowser from "@/components/CatalogBrowser";

export default function AuctionPage() {
  return (
    <CatalogBrowser
      listingType="auction"
      title="Активные торги"
      emptyText="Активных лотов не найдено."
    />
  );
}
