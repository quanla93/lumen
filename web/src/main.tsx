import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { I18nProvider } from "./i18n/I18nProvider";
import { ConfirmProvider } from "./components/ConfirmDialog";
import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import "./index.css";
import "uplot/dist/uPlot.min.css";

// Apply persisted theme (or system preference) BEFORE React mounts so there's
// no flash of the wrong palette on first paint.
const stored = localStorage.getItem("lumen.theme");
const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
if (stored === "dark" || (stored !== "light" && prefersDark)) {
  document.documentElement.classList.add("dark");
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <I18nProvider>
      <ConfirmProvider>
        <App />
      </ConfirmProvider>
    </I18nProvider>
  </StrictMode>,
);

// Register the PWA service worker. Wrapped so a dev environment
// (HTTP, file://) where SW isn't supported just no-ops instead of
// throwing. Errors are intentionally swallowed — the dashboard works
// without a SW, the only thing lost is the "install to homescreen"
// affordance and the offline app-shell cache.
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js").catch(() => {
      /* SW disabled (file://, HTTP in some browsers) — graceful no-op */
    });
  });
}
