import CatalogBrowser from "@/components/CatalogBrowser";

export default function StorePage() {
  return (
    <CatalogBrowser
      listingType="fixed_price"
      title="Машины в продаже"
      emptyText="Пока нет машин в продаже."
    />
  );
}
