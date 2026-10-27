"use client";

import { useEffect, useState } from "react";
import { formatCountdown } from "@/lib/format";

export default function Countdown({ endAt, compact }: { endAt: string; compact?: boolean }) {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);

  const ms = new Date(endAt).getTime() - now;
  const ending = ms > 0 && ms <= 60_000;

  return (
    <span
      className={
        compact
          ? `text-xs font-mono ${ending ? "text-red-600" : "text-gray-500"}`
          : `font-mono text-xl ${ending ? "text-red-600" : "text-gray-900"}`
      }
    >
      {ms <= 0 ? "торги завершены" : formatCountdown(ms)}
    </span>
  );
}
