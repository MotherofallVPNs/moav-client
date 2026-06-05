import { useEffect, useState } from "react";

// Tracks whether the viewport is phone-width so components can stack instead
// of overflowing. 640px ≈ where the topbar + tables stop fitting.
export function useIsMobile(breakpoint = 640): boolean {
  const [mobile, setMobile] = useState(
    typeof window !== "undefined" ? window.innerWidth < breakpoint : false
  );
  useEffect(() => {
    const onResize = () => setMobile(window.innerWidth < breakpoint);
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, [breakpoint]);
  return mobile;
}
