"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth";

export default function Nav() {
  const { user, loading, logout } = useAuth();
  const router = useRouter();

  function handleLogout() {
    logout();
    router.push("/");
  }

  return (
    <header className="border-b border-gray-200 bg-white">
      <div className="mx-auto flex max-w-5xl items-center justify-between px-4 py-3">
        <Link href="/" className="text-lg font-bold text-brand-700">
          АвтоТорг
        </Link>
        <nav className="flex items-center gap-4 text-sm">
          <Link href="/" className="text-gray-700 hover:text-brand-600">
            Магазин
          </Link>
          <Link
            href="/auction"
            className="rounded bg-amber-500 px-3 py-1.5 font-medium text-white hover:bg-amber-600"
          >
            Аукцион
          </Link>
          {!loading && user && (
            <>
              <Link href="/lots/new" className="text-gray-700 hover:text-brand-600">
                Выставить лот
              </Link>
              <Link href="/my-lots" className="text-gray-700 hover:text-brand-600">
                Мои лоты
              </Link>
              <Link href="/favorites" className="text-gray-700 hover:text-brand-600">
                Избранное
              </Link>
            </>
          )}
          {!loading && user && (
            <>
              <span className="text-gray-400">{user.name || user.email}</span>
              <button onClick={handleLogout} className="text-gray-700 hover:text-brand-600">
                Выйти
              </button>
            </>
          )}
          {!loading && !user && (
            <>
              <Link href="/login" className="text-gray-700 hover:text-brand-600">
                Войти
              </Link>
              <Link
                href="/register"
                className="rounded bg-brand-600 px-3 py-1.5 text-white hover:bg-brand-700"
              >
                Регистрация
              </Link>
            </>
          )}
        </nav>
      </div>
    </header>
  );
}
