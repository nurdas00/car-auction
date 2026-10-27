"use client";

import { createContext, useContext, useEffect, useState, ReactNode } from "react";
import { User } from "./types";

type AuthContextValue = {
  token: string | null;
  user: User | null;
  loading: boolean;
  login: (token: string, user: User) => void;
  logout: () => void;
};

const AuthContext = createContext<AuthContextValue>({
  token: null,
  user: null,
  loading: true,
  login: () => {},
  logout: () => {},
});

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(null);
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    try {
      const t = localStorage.getItem("token");
      const u = localStorage.getItem("user");
      if (t) setToken(t);
      if (u) setUser(JSON.parse(u));
    } catch {
      // приватный режим / localStorage недоступен — работаем без сохранённой сессии
    }
    setLoading(false);
  }, []);

  function login(t: string, u: User) {
    try {
      localStorage.setItem("token", t);
      localStorage.setItem("user", JSON.stringify(u));
    } catch {
      // см. выше
    }
    setToken(t);
    setUser(u);
  }

  function logout() {
    try {
      localStorage.removeItem("token");
      localStorage.removeItem("user");
    } catch {
      // см. выше
    }
    setToken(null);
    setUser(null);
  }

  return (
    <AuthContext.Provider value={{ token, user, loading, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  return useContext(AuthContext);
}
